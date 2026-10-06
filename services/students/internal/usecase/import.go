package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// MaxImportRows is the most pupils one import request may carry.
const MaxImportRows = 1000

// Reason codes of a REJECTED row, and the warning codes the import adds to the
// profile's. They are a contract (students.v1 ImportRowResult): the client
// words them for the school.
const (
	ReasonCURPInvalid    = "curp_invalid"
	ReasonCURPDuplicated = "curp_duplicated"
	ReasonNameRequired   = "name_required"
	ReasonGroupNotFound  = "group_not_found"

	WarnBankReferenceInvalid = "bank_reference_invalid"
	WarnBankReferenceTaken   = "bank_reference_taken"
)

// ImportOutcome is what happened to one row.
type ImportOutcome uint8

// The outcomes of a row.
const (
	ImportOutcomeUnspecified ImportOutcome = iota
	// ImportImported: stored (or, validating, would be).
	ImportImported
	// ImportAlreadyExists: the school has this CURP; nothing is touched (O4).
	ImportAlreadyExists
	// ImportAlreadyImported: ...and the pupil is already placed in this group.
	ImportAlreadyImported
	// ImportRejected: see Reason.
	ImportRejected
)

// ImportRow is one pupil of the previous system's spreadsheet, already parsed
// by the gateway. Domain rules are applied here.
type ImportRow struct {
	RowNumber        int
	CURP             string
	FirstName        string
	LastName         string // the paternal surname
	MaternalLastName string
	// GroupID is the config group; empty only while validating, for a group
	// the import has not created yet.
	GroupID       string
	BankReference string
	Profile       domain.ProfileInput
}

// ImportResult is the outcome of one row.
type ImportResult struct {
	RowNumber        int
	Outcome          ImportOutcome
	StudentID        string
	EnrollmentNumber string
	Warnings         []string
	Reason           string
}

// ImportService brings the pupils of the previous system in (ADR 0033,
// PLAN-student-import.md). They were enrolled and paid there, so they enter
// ENROLLED with an enrollment of origin "import" — which is what keeps finance
// from billing an inscription the school already collected.
//
// It has no event publisher, by construction: an import announces nothing (no
// StudentEnrolled, no AdmissionStatusChanged).
//
//	validate rows ─> resolve each distinct group once (config) ─> one read of
//	the CURPs the school has ─> per row: ONE transaction (pupil + enrollment)
type ImportService struct {
	repo    ports.ImportRepository
	catalog ports.CatalogValidator
	levels  ports.LevelCatalog
	tenant  string
	newID   func() string
	now     func() time.Time
}

// NewImportService wires the import. `now` stamps the enrollments and judges
// birth dates (injectable for deterministic tests).
func NewImportService(
	repo ports.ImportRepository,
	catalog ports.CatalogValidator,
	levels ports.LevelCatalog,
	tenant string,
	newID func() string,
	now func() time.Time,
) *ImportService {
	return &ImportService{repo: repo, catalog: catalog, levels: levels, tenant: tenant, newID: newID, now: now}
}

// placement is a group resolved against config: where the pupil goes, the
// cycle it derives, and the level the pupil is filed under. A group config does
// not know is ok=false.
type placement struct {
	group ports.GroupRef
	level domain.Level
	ok    bool
}

// pendingRow is a row that passed the row-local checks.
type pendingRow struct {
	index int
	curp  domain.CURP
	ref   domain.BankReference
}

// ImportStudents imports (or, with validateOnly, only judges) up to
// MaxImportRows pupils and answers one result per row, in request order. A
// bad ROW is a result; only a bad REQUEST (too many rows, a commit without a
// batch id) or an unreachable dependency is an error.
//
// Rows already committed stay committed if a later row hits an infrastructure
// error: each pupil is its own transaction, and running the same import again
// answers ALREADY_IMPORTED for them.
func (s *ImportService) ImportStudents(ctx context.Context, rows []ImportRow, validateOnly bool, batchID string) ([]ImportResult, error) {
	if len(rows) > MaxImportRows {
		return nil, glerrors.New(glerrors.KindInvalid, fmt.Sprintf("an import carries at most %d rows", MaxImportRows))
	}
	if !validateOnly {
		if err := uuid.Validate(batchID); err != nil {
			return nil, glerrors.New(glerrors.KindInvalid, "an import needs its batch id")
		}
	}

	results := make([]ImportResult, len(rows))
	pending := s.checkRows(rows, results)

	existing, err := s.repo.StudentsByCURP(ctx, curpsOf(pending))
	if err != nil {
		return nil, err
	}
	pending = markExisting(rows, pending, existing, results)

	placements, err := s.resolveGroups(ctx, rows, pending)
	if err != nil {
		return nil, err
	}

	if validateOnly {
		return results, s.judge(ctx, rows, pending, placements, results)
	}
	for _, p := range pending {
		if err := s.importOne(ctx, rows[p.index], p, placements, batchID, &results[p.index]); err != nil {
			return nil, err
		}
	}
	return results, nil
}

// checkRows applies the checks that need no I/O: the CURP, its repetition in
// the request, and the names. The bank reference is judged here too: an
// invalid one only costs a warning.
func (s *ImportService) checkRows(rows []ImportRow, results []ImportResult) []pendingRow {
	pending := make([]pendingRow, 0, len(rows))
	seen := make(map[domain.CURP]bool, len(rows))
	for i, r := range rows {
		results[i] = ImportResult{RowNumber: r.RowNumber}
		curp, err := domain.NewCURP(r.CURP)
		if err != nil {
			reject(&results[i], ReasonCURPInvalid)
			continue
		}
		if seen[curp] {
			reject(&results[i], ReasonCURPDuplicated)
			continue
		}
		seen[curp] = true
		ref, err := domain.NewBankReference(r.BankReference)
		if err != nil {
			results[i].Warnings = append(results[i].Warnings, WarnBankReferenceInvalid)
		}
		pending = append(pending, pendingRow{index: i, curp: curp, ref: ref})
	}
	return pending
}

// markExisting settles the rows whose CURP the school already has (O4): they
// are skipped and nothing is touched. The pupil already placed in the row's
// own group is ALREADY_IMPORTED — the same file uploaded twice.
func markExisting(rows []ImportRow, pending []pendingRow, existing map[string]ports.ExistingPupil, results []ImportResult) []pendingRow {
	left := pending[:0]
	for _, p := range pending {
		pupil, ok := existing[p.curp.String()]
		if !ok {
			left = append(left, p)
			continue
		}
		settleExisting(&results[p.index], pupil, rows[p.index].GroupID)
	}
	return left
}

func settleExisting(result *ImportResult, pupil ports.ExistingPupil, groupID string) {
	result.Outcome = ImportAlreadyExists
	result.StudentID = pupil.StudentID
	result.Warnings = nil
	for _, g := range pupil.ActiveGroupIDs {
		if groupID != "" && g == groupID {
			result.Outcome = ImportAlreadyImported
			return
		}
	}
}

// resolveGroups asks config about each distinct group ONCE (an import of 570
// pupils spans ~40 groups). It derives the cycle and the level from the group,
// as Enroll does: the pupil's level IS the group's level, so the same-level
// rule holds by construction.
func (s *ImportService) resolveGroups(ctx context.Context, rows []ImportRow, pending []pendingRow) (map[string]placement, error) {
	out := map[string]placement{}
	for _, p := range pending {
		id := rows[p.index].GroupID
		if _, done := out[id]; done || id == "" {
			continue
		}
		resolved, err := s.placementOf(ctx, id)
		if err != nil {
			return nil, err
		}
		out[id] = resolved
	}
	return out, nil
}

// placementOf resolves one group. A group (or grade, or level) config does not
// know is ok=false — a row's problem; anything else stops the import.
func (s *ImportService) placementOf(ctx context.Context, groupID string) (placement, error) {
	if uuid.Validate(groupID) != nil {
		return placement{}, nil
	}
	grp, err := groupOf(ctx, s.catalog, groupID)
	if glerrors.KindOf(err) == glerrors.KindInvalid {
		return placement{}, nil
	}
	if err != nil {
		return placement{}, err
	}
	levelID, err := levelOfGrade(ctx, s.catalog, grp.GradeLevelID)
	if glerrors.KindOf(err) == glerrors.KindInvalid {
		return placement{}, nil
	}
	if err != nil {
		return placement{}, err
	}
	level, err := s.levels.LevelByID(ctx, levelID)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		return placement{}, nil
	}
	if err != nil {
		return placement{}, err
	}
	return placement{group: grp, level: level, ok: true}, nil
}

// judge finishes a validate_only run: every check a commit would make, and
// nothing written. A reference another pupil holds — or an earlier row of
// this same request claims — is the warning the commit would give.
func (s *ImportService) judge(ctx context.Context, rows []ImportRow, pending []pendingRow, placements map[string]placement, results []ImportResult) error {
	taken, err := s.repo.BankReferencesTaken(ctx, refsOf(pending))
	if err != nil {
		return err
	}
	claimed := map[domain.BankReference]bool{}
	for _, p := range pending {
		r, result := rows[p.index], &results[p.index]
		if !s.placeable(r, placements, true, result) {
			continue
		}
		if !p.ref.IsZero() {
			if taken[p.ref.String()] || claimed[p.ref] {
				result.Warnings = append(result.Warnings, WarnBankReferenceTaken)
			}
			claimed[p.ref] = true
		}
		_, warnings := domain.NewProfileLenient(r.Profile, s.now())
		result.Warnings = append(result.Warnings, warnings...)
		result.Outcome = ImportImported
	}
	return nil
}

// placeable applies the names and group checks, rejecting the row on failure.
// While validating, a row without a group is fine: its group does not exist
// yet.
func (s *ImportService) placeable(r ImportRow, placements map[string]placement, validateOnly bool, result *ImportResult) bool {
	if strings.TrimSpace(r.FirstName) == "" || strings.TrimSpace(r.LastName) == "" {
		reject(result, ReasonNameRequired)
		return false
	}
	if r.GroupID == "" && validateOnly {
		return true
	}
	if !placements[r.GroupID].ok {
		reject(result, ReasonGroupNotFound)
		return false
	}
	return true
}

// importOne stores one pupil in its own transaction. A CURP another writer
// stored meanwhile is ALREADY_EXISTS; a reference taken between the store's
// check and its insert costs the reference, never the pupil.
func (s *ImportService) importOne(ctx context.Context, r ImportRow, p pendingRow, placements map[string]placement, batchID string, result *ImportResult) error {
	if !s.placeable(r, placements, false, result) {
		return nil
	}
	where := placements[r.GroupID]
	profile, warnings := domain.NewProfileLenient(r.Profile, s.now())
	cmd, err := s.command(r, p, where, profile, batchID)
	if err != nil {
		return err
	}

	stored, err := s.repo.ImportStudent(ctx, cmd)
	if errors.Is(err, domain.ErrBankReferenceTaken) {
		cmd.Student.ChangeBankReference("")
		stored, err = s.repo.ImportStudent(ctx, cmd)
		stored.BankReferenceDropped = true
	}
	if errors.Is(err, domain.ErrCURPTaken) {
		return s.settleRace(ctx, p.curp, r.GroupID, result)
	}
	if err != nil {
		return fmt.Errorf("importing row %d: %w", r.RowNumber, err)
	}

	if stored.BankReferenceDropped {
		result.Warnings = append(result.Warnings, WarnBankReferenceTaken)
	}
	result.Warnings = append(result.Warnings, warnings...)
	result.Outcome = ImportImported
	result.StudentID = cmd.Student.ID()
	result.EnrollmentNumber = stored.EnrollmentNumber
	return nil
}

// command builds the pupil (enrolled, filed under the group's level, named in
// it) and their import enrollment, dated now — never back to the start of the
// cycle, so tuition is billed from the month of the import (ADR 0033).
func (s *ImportService) command(r ImportRow, p pendingRow, where placement, profile domain.Profile, batchID string) (ports.ImportCommand, error) {
	student, err := domain.NewImported(domain.NewApplicantParams{
		ID:               s.newID(),
		TenantID:         s.tenant,
		CURP:             p.curp,
		FirstName:        r.FirstName,
		LastName:         r.LastName,
		MaternalLastName: r.MaternalLastName,
		Level:            where.level,
		Profile:          profile,
		BankReference:    p.ref,
	})
	if err != nil {
		return ports.ImportCommand{}, err
	}
	student.PlaceIn(where.group.Name)
	enrollment, err := domain.NewEnrollment(s.newID(), s.tenant, student.ID(), where.group.CycleID, where.group.ID, "",
		domain.EnrollmentStatusActive, s.now())
	if err != nil {
		return ports.ImportCommand{}, err
	}
	return ports.ImportCommand{
		Student:    student,
		Enrollment: enrollment.WithOrigin(domain.EnrollmentOriginImport),
		BatchID:    batchID,
	}, nil
}

// settleRace answers a row whose CURP another writer stored after this
// import's read: the pupil that won is read back and reported.
func (s *ImportService) settleRace(ctx context.Context, curp domain.CURP, groupID string, result *ImportResult) error {
	existing, err := s.repo.StudentsByCURP(ctx, []string{curp.String()})
	if err != nil {
		return err
	}
	settleExisting(result, existing[curp.String()], groupID)
	return nil
}

func reject(result *ImportResult, reason string) {
	result.Outcome = ImportRejected
	result.Reason = reason
	result.Warnings = nil
}

func curpsOf(pending []pendingRow) []string {
	out := make([]string, 0, len(pending))
	for _, p := range pending {
		out = append(out, p.curp.String())
	}
	return out
}

func refsOf(pending []pendingRow) []string {
	out := make([]string, 0, len(pending))
	for _, p := range pending {
		if !p.ref.IsZero() {
			out = append(out, p.ref.String())
		}
	}
	return out
}
