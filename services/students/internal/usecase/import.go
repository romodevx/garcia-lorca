package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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
	// Action is the school's choice in the preview (PLAN-import-avisos D2).
	Action ImportAction
	// Issues are what the gateway changed or dropped while reading the file;
	// each is stored with the pupil.
	Issues []ImportIssueInput
	// Original is the spreadsheet's raw text, reported by the issues students
	// raises instead of the value received when it is set.
	Original ImportOriginal
}

// ImportAction is what the school chose for a row.
type ImportAction uint8

// The actions. The zero value imports.
const (
	ImportActionImport ImportAction = iota
	// ImportActionDraft stores a row whose CURP is missing or invalid as a
	// draft pupil, with no enrollment. A row with a valid CURP imports.
	ImportActionDraft
)

// ImportIssueInput is one issue the gateway raised on a row.
type ImportIssueInput struct {
	Field         string
	Code          string
	OriginalValue string
	StoredValue   string
}

// ImportOriginal is the spreadsheet's raw text per field; "" is unknown.
type ImportOriginal struct {
	CURP          string
	BirthDate     string
	PhoneHome     string
	PhoneMobile   string
	Email         string
	PostalCode    string
	State         string
	BankReference string
	Address       string
}

// of returns the raw text of a field, or `received` when the gateway sent none.
func (o ImportOriginal) of(field domain.IssueField, received string) string {
	raw := map[domain.IssueField]string{
		domain.FieldCURP: o.CURP, domain.FieldBirthDate: o.BirthDate, domain.FieldPhoneHome: o.PhoneHome,
		domain.FieldPhoneMobile: o.PhoneMobile, domain.FieldEmail: o.Email, domain.FieldPostalCode: o.PostalCode,
		domain.FieldState: o.State, domain.FieldBankReference: o.BankReference, domain.FieldAddress: o.Address,
	}[field]
	if raw == "" {
		return received
	}
	return raw
}

// ImportResult is the outcome of one row.
type ImportResult struct {
	RowNumber        int
	Outcome          ImportOutcome
	StudentID        string
	EnrollmentNumber string
	Warnings         []string
	Reason           string
	// MovedToCycleStart says the pupil was already here from an earlier import
	// dated after the cycle's first day, and their enrollment was moved back to
	// it (or, validating, would be).
	MovedToCycleStart bool
	// Draft says the row was (or, validating, would be) stored as a draft.
	Draft bool
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
	// zone is the school's time zone: a matrícula is numbered under the
	// school's calendar year (T5b). UTC until WithSchoolZone.
	zone *time.Location
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
	return &ImportService{repo: repo, catalog: catalog, levels: levels, tenant: tenant, newID: newID, now: now, zone: time.UTC}
}

// WithSchoolZone returns the import numbering matrículas in the school's
// calendar year of `zone`, resolved once at boot.
func (s *ImportService) WithSchoolZone(zone *time.Location) *ImportService {
	cp := *s
	cp.zone = zone
	return &cp
}

// placement is a group resolved against config: where the pupil goes, the
// cycle it derives, the level the pupil is filed under, and the instant their
// enrollment starts. A group config does not know is ok=false.
type placement struct {
	group ports.GroupRef
	level domain.Level
	entry time.Time
	ok    bool
}

// pendingRow is a row that passed the row-local checks. A draft has no CURP;
// curpCode says why (curp_missing or curp_invalid). refInvalid is a reference
// the row brought that is not one.
type pendingRow struct {
	index      int
	curp       domain.CURP
	ref        domain.BankReference
	draft      bool
	curpCode   string
	refInvalid bool
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
	if err := checkGatewayIssues(rows); err != nil {
		return nil, err
	}

	results := make([]ImportResult, len(rows))
	pending := s.checkRows(rows, results)

	existing, err := s.repo.StudentsByCURP(ctx, curpsOf(pending))
	if err != nil {
		return nil, err
	}
	pending = markExisting(rows, pending, existing, results)
	if pending, err = s.markExistingDrafts(ctx, rows, pending, results); err != nil {
		return nil, err
	}

	placements, err := s.resolveGroups(ctx, rows, pending, results)
	if err != nil {
		return nil, err
	}
	if err := s.moveToCycleStart(ctx, rows, placements, validateOnly, batchID, results); err != nil {
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

// checkGatewayIssues refuses, before anything is stored, a request carrying
// an issue students could not store — the gateway's bug, never the school's
// row. It applies every rule NewImportIssue will (field, code, sizes after the
// same clipping attachIssues does), so the commit can never stop half way on
// one.
func checkGatewayIssues(rows []ImportRow) error {
	for _, r := range rows {
		for _, in := range r.Issues {
			_, err := domain.NewImportIssue(domain.ImportIssueParams{
				ID: "check", StudentID: "check", Field: in.Field, Code: in.Code,
				OriginalValue: domain.ClipIssueValue(in.OriginalValue), StoredValue: domain.ClipIssueValue(in.StoredValue),
				CreatedAt: time.Unix(0, 0),
			})
			if err != nil {
				return fmt.Errorf("row %d: %w", r.RowNumber, err)
			}
		}
	}
	return nil
}

// checkRows applies the checks that need no I/O: the CURP, its repetition in
// the request, and the names. The bank reference is judged here too: an
// invalid one only costs a warning. A row without a valid CURP is rejected,
// unless the school chose to keep it as a draft.
func (s *ImportService) checkRows(rows []ImportRow, results []ImportResult) []pendingRow {
	pending := make([]pendingRow, 0, len(rows))
	seen := make(map[domain.CURP]bool, len(rows))
	for i, r := range rows {
		results[i] = ImportResult{RowNumber: r.RowNumber}
		p := pendingRow{index: i}
		curp, err := domain.NewCURP(r.CURP)
		switch {
		case err != nil && r.Action != ImportActionDraft:
			reject(&results[i], ReasonCURPInvalid)
			continue
		case err != nil:
			p.draft, p.curpCode = true, curpIssueCode(r.CURP)
		case seen[curp]:
			reject(&results[i], ReasonCURPDuplicated)
			continue
		default:
			seen[curp], p.curp = true, curp
		}
		ref, err := domain.NewBankReference(r.BankReference)
		if err != nil {
			results[i].Warnings = append(results[i].Warnings, WarnBankReferenceInvalid)
			p.refInvalid = true
		}
		p.ref = ref
		pending = append(pending, p)
	}
	return pending
}

// curpIssueCode is why a draft has no CURP.
func curpIssueCode(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return domain.IssueCURPMissing
	}
	return domain.IssueCURPInvalid
}

// markExisting settles the rows whose CURP the school already has (O4): they
// are skipped and nothing is touched. The pupil already placed in the row's
// own group is ALREADY_IMPORTED — the same file uploaded twice.
func markExisting(rows []ImportRow, pending []pendingRow, existing map[string]ports.ExistingPupil, results []ImportResult) []pendingRow {
	left := pending[:0]
	for _, p := range pending {
		if p.draft {
			left = append(left, p)
			continue
		}
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

// draftKey recognizes a draft without a CURP: its names (case and spacing
// folded), birth date and intended group. The bank reference is compared
// apart, only when both sides have one.
type draftKey struct {
	name, born, group string
}

func newDraftKey(first, last, maternal string, born time.Time, group string) draftKey {
	name := strings.ToUpper(strings.Join(strings.Fields(last+" "+maternal+" "+first), " "))
	day := ""
	if !born.IsZero() {
		day = born.Format(time.DateOnly)
	}
	return draftKey{name: name, born: day, group: group}
}

// markExistingDrafts settles the draft rows a previous upload already stored
// (same names, birth date, intended group and — when both have one — bank
// reference) as ALREADY_IMPORTED: re-uploading a file never duplicates its
// drafts. Read once per request, and only when the request has a draft row.
func (s *ImportService) markExistingDrafts(ctx context.Context, rows []ImportRow, pending []pendingRow, results []ImportResult) ([]pendingRow, error) {
	if !slices.ContainsFunc(pending, func(p pendingRow) bool { return p.draft }) {
		return pending, nil
	}
	drafts, err := s.repo.OpenDrafts(ctx)
	if err != nil {
		return nil, err
	}
	byKey := make(map[draftKey][]ports.ExistingDraft, len(drafts))
	for _, d := range drafts {
		k := newDraftKey(d.FirstName, d.LastName, d.MaternalLastName, d.BirthDate, d.IntendedGroupID)
		byKey[k] = append(byKey[k], d)
	}
	left := pending[:0]
	for _, p := range pending {
		match, ok := s.draftOf(rows[p.index], p, byKey)
		if !ok {
			left = append(left, p)
			continue
		}
		results[p.index].Outcome = ImportAlreadyImported
		results[p.index].StudentID = match.StudentID
		results[p.index].Draft = true
		results[p.index].Warnings = nil
	}
	return left, nil
}

// draftOf finds the stored draft a draft row is, if any.
func (s *ImportService) draftOf(r ImportRow, p pendingRow, byKey map[draftKey][]ports.ExistingDraft) (ports.ExistingDraft, bool) {
	if !p.draft {
		return ports.ExistingDraft{}, false
	}
	profile, _ := domain.NewProfileLenient(r.Profile, s.now())
	k := newDraftKey(r.FirstName, r.LastName, r.MaternalLastName, profile.BirthDate(), r.GroupID)
	for _, d := range byKey[k] {
		if p.ref.IsZero() || d.BankReference == "" || d.BankReference == p.ref.String() {
			return d, true
		}
	}
	return ports.ExistingDraft{}, false
}

// resolveGroups asks config about each distinct group ONCE (an import of 570
// pupils spans ~40 groups), and about each distinct cycle once. It derives the
// cycle and the level from the group, as Enroll does: the pupil's level IS the
// group's level, so the same-level rule holds by construction. The groups of
// the pupils the school already has are resolved too: a re-import repairs
// their enrollment's first day (moveToCycleStart).
func (s *ImportService) resolveGroups(ctx context.Context, rows []ImportRow, pending []pendingRow, results []ImportResult) (map[string]placement, error) {
	ids := make([]string, 0, len(rows))
	for _, p := range pending {
		ids = append(ids, rows[p.index].GroupID)
	}
	for i, r := range results {
		if isExisting(r) {
			ids = append(ids, rows[i].GroupID)
		}
	}
	out := map[string]placement{}
	entries := map[string]cycleEntry{}
	for _, id := range ids {
		if _, done := out[id]; done || id == "" {
			continue
		}
		resolved, err := s.placementOf(ctx, id, entries)
		if err != nil {
			return nil, err
		}
		out[id] = resolved
	}
	return out, nil
}

// cycleEntry is when an import enrollment in a cycle starts, or ok=false for a
// cycle config does not know.
type cycleEntry struct {
	at time.Time
	ok bool
}

// placementOf resolves one group. A group (or grade, or level, or cycle) config
// does not know is ok=false — a row's problem; anything else stops the import.
// entries caches the cycles already asked about.
func (s *ImportService) placementOf(ctx context.Context, groupID string, entries map[string]cycleEntry) (placement, error) {
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
	entry, err := s.entryOf(ctx, grp.CycleID, entries)
	if err != nil || !entry.ok {
		return placement{}, err
	}
	return placement{group: grp, level: level, entry: entry.at, ok: true}, nil
}

// entryOf is when an imported pupil's enrollment in the cycle starts: the
// cycle's first day at 00:00 on the school's calendar (owner decision
// 2026-10-17) — they were in school since then and are billed from it. An
// import run before the cycle opens is dated now instead of in the future;
// billing and attendance read that as the whole cycle all the same.
func (s *ImportService) entryOf(ctx context.Context, cycleID string, entries map[string]cycleEntry) (cycleEntry, error) {
	if known, ok := entries[cycleID]; ok {
		return known, nil
	}
	cycle, err := s.catalog.CycleByID(ctx, cycleID)
	if glerrors.KindOf(err) == glerrors.KindNotFound {
		entries[cycleID] = cycleEntry{}
		return cycleEntry{}, nil
	}
	if err != nil {
		return cycleEntry{}, err
	}
	d := cycle.StartsOn.UTC()
	at := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, s.zone)
	if now := s.now(); at.After(now) {
		at = now
	}
	entries[cycleID] = cycleEntry{at: at, ok: true}
	return entries[cycleID], nil
}

// moveToCycleStart repairs the pupils an earlier import dated on its own day
// (before the owner's decision of 2026-10-17): for every pupil the school
// already has whose row is placed, the store moves their import enrollment in
// that cycle back to its first day. One statement per cycle; validating only
// reports who would move. The store decides who moves, so a second upload
// moves nobody.
//
// A real move is logged with the batch and the pupils' ids — ids only, no
// name or CURP — so who was re-dated, and by which upload, is on record.
func (s *ImportService) moveToCycleStart(ctx context.Context, rows []ImportRow, placements map[string]placement, validateOnly bool, batchID string, results []ImportResult) error {
	var cycles []string
	moves := map[string]*ports.CycleStartMove{}
	for i, r := range results {
		where := placements[rows[i].GroupID]
		if !isExisting(r) || r.Draft || !where.ok {
			continue
		}
		move, ok := moves[where.group.CycleID]
		if !ok {
			move = &ports.CycleStartMove{CycleID: where.group.CycleID, StartsAt: where.entry, DryRun: validateOnly}
			moves[where.group.CycleID] = move
			cycles = append(cycles, where.group.CycleID)
		}
		move.StudentIDs = append(move.StudentIDs, r.StudentID)
	}
	moved := map[string]bool{}
	for _, cycleID := range cycles {
		ids, err := s.repo.MoveImportedToCycleStart(ctx, *moves[cycleID])
		if err != nil {
			return fmt.Errorf("moving earlier imports to the cycle start: %w", err)
		}
		for _, id := range ids {
			moved[id] = true
		}
		if !validateOnly && len(ids) > 0 {
			slog.InfoContext(ctx, "import: enrollments moved to the cycle start",
				"batch", batchID, "cycle", cycleID, "starts_at", moves[cycleID].StartsAt, "student_ids", ids)
		}
	}
	for i := range results {
		if isExisting(results[i]) && moved[results[i].StudentID] {
			results[i].MovedToCycleStart = true
		}
	}
	return nil
}

// isExisting is a row whose CURP the school already has (O4).
func isExisting(r ImportResult) bool {
	return r.Outcome == ImportAlreadyExists || r.Outcome == ImportAlreadyImported
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
		result.Draft = p.draft
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
	profile, drops := domain.NewProfileLenientReport(r.Profile, s.now())
	cmd, err := s.command(r, p, where, profile, batchID)
	if err != nil {
		return err
	}
	if err := s.attachIssues(&cmd, r, p, drops, batchID); err != nil {
		return err
	}

	stored, err := s.repo.ImportStudent(ctx, cmd)
	if errors.Is(err, domain.ErrBankReferenceTaken) {
		cmd.Student.ChangeBankReference("")
		if cmd.ReferenceTakenIssue != nil {
			cmd.Issues = append(cmd.Issues, *cmd.ReferenceTakenIssue)
			cmd.ReferenceTakenIssue = nil
		}
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
	result.Warnings = append(result.Warnings, domain.WarningCodes(drops)...)
	result.Outcome = ImportImported
	result.StudentID = cmd.Student.ID()
	result.EnrollmentNumber = stored.EnrollmentNumber
	result.Draft = p.draft
	return nil
}

// attachIssues turns everything the row could not keep as written into the
// issues stored with the pupil (PLAN-import-avisos D1), in order: the draft's
// missing CURP (with the group it was meant for), the gateway's own, an
// invalid bank reference, the profile's drops. A reference the store may find
// taken is prepared too, stored only if it is.
func (s *ImportService) attachIssues(cmd *ports.ImportCommand, r ImportRow, p pendingRow, drops []domain.ProfileDrop, batchID string) error {
	studentID := cmd.Student.ID()
	issue := func(in domain.ImportIssueParams) (domain.ImportIssue, error) {
		in.ID, in.TenantID, in.StudentID, in.BatchID, in.CreatedAt = s.newID(), s.tenant, studentID, batchID, s.now()
		in.OriginalValue, in.StoredValue = domain.ClipIssueValue(in.OriginalValue), domain.ClipIssueValue(in.StoredValue)
		return domain.NewImportIssue(in)
	}
	var params []domain.ImportIssueParams
	if p.draft {
		params = append(params, domain.ImportIssueParams{Field: string(domain.FieldCURP), Code: p.curpCode,
			OriginalValue: r.Original.of(domain.FieldCURP, r.CURP), IntendedGroupID: r.GroupID})
	}
	for _, in := range r.Issues {
		params = append(params, domain.ImportIssueParams{Field: in.Field, Code: in.Code, OriginalValue: in.OriginalValue, StoredValue: in.StoredValue})
	}
	if p.refInvalid {
		params = append(params, domain.ImportIssueParams{Field: string(domain.FieldBankReference), Code: WarnBankReferenceInvalid,
			OriginalValue: r.Original.of(domain.FieldBankReference, r.BankReference)})
	}
	for _, d := range drops {
		params = append(params, domain.ImportIssueParams{Field: string(d.Field), Code: d.Code,
			OriginalValue: r.Original.of(d.Field, d.Original), StoredValue: d.Stored})
	}
	for _, in := range params {
		built, err := issue(in)
		if err != nil {
			return fmt.Errorf("row %d: %w", r.RowNumber, err)
		}
		cmd.Issues = append(cmd.Issues, built)
	}
	if p.ref.IsZero() {
		return nil
	}
	taken, err := issue(domain.ImportIssueParams{Field: string(domain.FieldBankReference), Code: WarnBankReferenceTaken,
		OriginalValue: r.Original.of(domain.FieldBankReference, p.ref.String())})
	if err != nil {
		return fmt.Errorf("row %d: %w", r.RowNumber, err)
	}
	cmd.ReferenceTakenIssue = &taken
	return nil
}

// command builds the pupil (enrolled, filed under the group's level, named in
// it) and their import enrollment, dated from the cycle's first day (entryOf;
// replaces ADR 0033's "dated the day of the import", owner decision 2026-10-17).
//
// A draft is built as one: filed under the group's level, NOT placed in it,
// and without an enrollment — the group rides on its curp issue.
func (s *ImportService) command(r ImportRow, p pendingRow, where placement, profile domain.Profile, batchID string) (ports.ImportCommand, error) {
	if p.draft {
		return s.draftCommand(r, p, where, profile, batchID)
	}
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
		domain.EnrollmentStatusActive, where.entry)
	if err != nil {
		return ports.ImportCommand{}, err
	}
	return ports.ImportCommand{
		Student:    student,
		Enrollment: enrollment.WithOrigin(domain.EnrollmentOriginImport),
		BatchID:    batchID,
		NumberYear: s.now().In(s.zone).Year(),
	}, nil
}

func (s *ImportService) draftCommand(r ImportRow, p pendingRow, where placement, profile domain.Profile, batchID string) (ports.ImportCommand, error) {
	student, err := domain.NewImportedDraft(domain.NewApplicantParams{
		ID:               s.newID(),
		TenantID:         s.tenant,
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
	return ports.ImportCommand{Student: student, BatchID: batchID, NumberYear: s.now().In(s.zone).Year()}, nil
}

// settleRace answers a row whose CURP another writer stored after this
// import's read: the pupil that won is read back and reported.
func (s *ImportService) settleRace(ctx context.Context, curp domain.CURP, groupID string, result *ImportResult) error {
	existing, err := s.repo.StudentsByCURP(ctx, []string{curp.String()})
	if err != nil {
		return err
	}
	pupil, ok := existing[curp.String()]
	if !ok {
		return fmt.Errorf("import: CURP %s reported taken but not found", curp.String()[:4]+"…")
	}
	settleExisting(result, pupil, groupID)
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
		if !p.draft {
			out = append(out, p.curp.String())
		}
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
