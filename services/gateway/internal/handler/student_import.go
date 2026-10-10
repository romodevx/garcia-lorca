package handler

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/legacyimport"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

// --- legacy student import (ADR 0033) -----------------------------------------
//
// The office uploads one spreadsheet of the previous system per level.
//
//	preview: parse → plan rows → active cycle → resolve levels →
//	         config.EnsurePlacements(dry) → students.ImportStudents(validate_only)
//	commit : the same preview again → its hash must equal the one the office saw
//	         (409 PREVIEW_STALE otherwise) → EnsurePlacements → ImportStudents
//
// The commit re-sends the file instead of trusting a token: the gateway runs on
// several instances and keeps nothing between requests. Each pupil is its own
// transaction in students, so a commit cut halfway is finished by uploading the
// same file again — what went in reads "ya estaba".
//
// Nothing personal is logged: counts only.

// ImportCatalog is config's side of the import.
type ImportCatalog interface {
	ListCycles(ctx context.Context) ([]client.CycleResult, error)
	ListLevels(ctx context.Context) ([]client.LevelResult, error)
	EnsurePlacements(ctx context.Context, cycleID string, in []client.PlacementInput, dryRun bool) ([]client.PlacementResult, error)
}

// ImportRoster is students' side of the import.
type ImportRoster interface {
	ImportStudents(ctx context.Context, rows []client.ImportRowInput, validateOnly bool, batchID string) ([]client.ImportRowResult, error)
}

// ImportPricing tells which levels have a tuition table in the cycle, so the
// preview can warn that a level without one will not be billed.
type ImportPricing interface {
	ListTuitionPlans(ctx context.Context, cycleID string) ([]client.TuitionPlan, error)
}

// StudentImportDeps wires the handler. Pricing may be nil (no warning then).
//
// WriteBudget is the whole commit route's time, set as ITS write deadline: the
// server-wide write timeout is too short for re-planning plus writing a large
// file, and a commit that outlives the deadline lands while the office sees a
// dropped connection instead of the result.
type StudentImportDeps struct {
	Catalog     ImportCatalog
	Roster      ImportRoster
	Pricing     ImportPricing
	NewID       func() string
	WriteBudget time.Duration
}

// StudentImportHandler serves /api/v1/imports/students/{preview,commit}.
//
// running holds the schools with a commit in flight on this instance: one
// import at a time per school, so two tabs cannot write the same file side by
// side. It is instance-local; the gateway runs a single instance today, and a
// second one would still be safe for the data (CURP uniqueness turns the
// loser's rows into "ya estaba") — only slower.
type StudentImportHandler struct {
	d       StudentImportDeps
	running sync.Map
}

// NewStudentImportHandler builds the handler.
func NewStudentImportHandler(d StudentImportDeps) *StudentImportHandler {
	return &StudentImportHandler{d: d}
}

// Error codes the screen words.
const (
	codeFileNotXLSX       = "FILE_NOT_XLSX"
	codeFileEmpty         = "FILE_EMPTY"
	codeFileTooLarge      = "FILE_TOO_LARGE"
	codeFileMissingColumn = "FILE_MISSING_COLUMN"
	codeNoActiveCycle     = "NO_ACTIVE_CYCLE"
	codeLevelsUnresolved  = "LEVELS_UNRESOLVED"
	codeImportInProgress  = "IMPORT_IN_PROGRESS"
)

// Level resolution states.
const (
	levelMatched    = "matched"    // the file's name is a level the school has
	levelChosen     = "chosen"     // the office sent it to an existing level
	levelCreate     = "create"     // new to the school, or the office asked to create it
	levelUnresolved = "unresolved" // nobody has decided yet
)

// Row states.
const (
	rowNew      = "new"
	rowWarning  = "warning" // imported, with something dropped or guessed
	rowAlready  = "already" // the school has this CURP
	rowSkipped  = "skipped"
	rowPending  = "pending" // its level is unresolved
	statusAlive = "active"
)

// defaultGroupCapacity sizes a group the import creates: never smaller than the
// pupils it receives, never so small the next enrolment finds it full.
const defaultGroupCapacity = 30

// maxGroupCapacity bounds a capacity typed in the preview: a classroom, not a
// number that wraps when it is sent as int32.
const maxGroupCapacity = 500

// maxImportBody bounds the JSON request: a 2 MB file is ~2.7 MB in base64.
const maxImportBody = 4 << 20

// importChunk is how many pupils one call to students carries on a commit.
const importChunk = 50

// reasonNotAnswered marks a row students never answered (the commit ran out of
// time first); uploading the same file again settles it.
const reasonNotAnswered = "not_answered"

// commitAnswerMargin is kept back from the route's deadline for writing the
// answer after the detached commit returns.
const commitAnswerMargin = 3 * time.Second

type levelChoice struct {
	LevelID string `json:"levelId,omitempty"`
	Create  bool   `json:"create,omitempty"`
}

type importRequest struct {
	FileName   string                 `json:"fileName"`
	FileBase64 string                 `json:"fileBase64"`
	Levels     map[string]levelChoice `json:"levels,omitempty"`
	// Capacities sizes the groups the import creates, keyed "level|ordinal|group".
	Capacities map[string]int `json:"capacities,omitempty"`
	PlanHash   string         `json:"planHash,omitempty"`
}

type importCycle struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type importLevel struct {
	FileName  string `json:"fileName"`
	Status    string `json:"status"`
	LevelID   string `json:"levelId,omitempty"`
	LevelName string `json:"levelName,omitempty"`
	Students  int    `json:"students"`
	NoTuition bool   `json:"noTuition"`
}

type importPlacement struct {
	Key          string `json:"key"`
	Level        string `json:"level"`
	Grade        string `json:"grade"`
	Group        string `json:"group"`
	Students     int    `json:"students"`
	Capacity     int    `json:"capacity"`
	LevelCreated bool   `json:"levelCreated"`
	GradeCreated bool   `json:"gradeCreated"`
	GroupCreated bool   `json:"groupCreated"`
	Error        string `json:"error,omitempty"`
}

type importRow struct {
	Row              int      `json:"row"`
	Name             string   `json:"name"`
	CURP             string   `json:"curp"`
	Destination      string   `json:"destination"`
	Status           string   `json:"status"`
	Reason           string   `json:"reason,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	EnrollmentNumber string   `json:"enrollmentNumber,omitempty"`
}

type importSummary struct {
	Total    int `json:"total"`
	New      int `json:"new"`
	Warnings int `json:"warnings"` // a subset of New
	Already  int `json:"already"`
	Skipped  int `json:"skipped"`
	Pending  int `json:"pending"`
}

type importResponse struct {
	Cycle      importCycle       `json:"cycle"`
	Levels     []importLevel     `json:"levels"`
	Placements []importPlacement `json:"placements"`
	Rows       []importRow       `json:"rows"`
	Summary    importSummary     `json:"summary"`
	Unresolved []string          `json:"unresolved"`
	PlanHash   string            `json:"planHash"`
	BatchID    string            `json:"batchId,omitempty"`
}

// importRun is one planning pass: what the screen sees, plus what a commit
// needs to write.
type importRun struct {
	resp       importResponse
	planned    legacyimport.Planned
	fileDigest string                      // sha256 of the uploaded bytes: the plan approves THIS file
	keys       []legacyimport.PlacementKey // in the order sent to config
	inputs     []client.PlacementInput
	placements map[legacyimport.PlacementKey]client.PlacementResult
	levelOf    map[string]importLevel // by the file's level name
}

// Preview: POST /api/v1/imports/students/preview — writes nothing.
func (h *StudentImportHandler) Preview(w http.ResponseWriter, r *http.Request) {
	h.extendWriteDeadline(w, r)
	req, ok := decodeImport(w, r)
	if !ok {
		return
	}
	run, ok := h.plan(w, r.Context(), req)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, run.resp)
}

// Commit: POST /api/v1/imports/students/commit — writes what the preview
// showed, or nothing (409 PREVIEW_STALE, 422 LEVELS_UNRESOLVED).
func (h *StudentImportHandler) Commit(w http.ResponseWriter, r *http.Request) {
	deadline := h.extendWriteDeadline(w, r)
	req, ok := decodeImport(w, r)
	if !ok {
		return
	}
	run, ok := h.plan(w, r.Context(), req)
	if !ok {
		return
	}
	if req.PlanHash == "" || req.PlanHash != run.resp.PlanHash {
		writeCodedError(w, http.StatusConflict, codePreviewStale, "the preview changed; review it again")
		return
	}
	if len(run.resp.Unresolved) > 0 {
		writeCodedError(w, http.StatusUnprocessableEntity, codeLevelsUnresolved,
			"choose where these levels go: "+strings.Join(run.resp.Unresolved, ", "))
		return
	}
	who, _ := middleware.PrincipalFrom(r.Context())
	if _, busy := h.running.LoadOrStore(who.TenantID, struct{}{}); busy {
		writeCodedError(w, http.StatusConflict, codeImportInProgress, "another import is running for this school")
		return
	}
	defer h.running.Delete(who.TenantID)
	// Detached: a closed tab must not stop an import halfway through a level.
	ctx, cancel := context.WithDeadline(context.WithoutCancel(r.Context()), deadline.Add(-commitAnswerMargin))
	defer cancel()
	batch := h.d.NewID()
	resp, err := h.write(ctx, run, batch)
	if err != nil {
		writeConnectError(w, err)
		return
	}
	// Who waived these pupils' inscription, and when: the batch id is on every
	// pupil it created (students.import_batch_id).
	slog.InfoContext(ctx, "student import committed", "batch", batch, "by", who.UserID, "rows", resp.Summary.Total,
		"new", resp.Summary.New, "already", resp.Summary.Already, "skipped", resp.Summary.Skipped)
	writeJSON(w, http.StatusOK, resp)
}

// extendWriteDeadline gives this route its own write deadline (WriteBudget):
// both the preview and the commit read the whole file and ask config and
// students about every group and row, which outlives the server-wide write
// timeout on a large file or a slow database. It returns the deadline set.
func (h *StudentImportHandler) extendWriteDeadline(w http.ResponseWriter, r *http.Request) time.Time {
	deadline := time.Now().Add(h.d.WriteBudget)
	if err := http.NewResponseController(w).SetWriteDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		slog.WarnContext(r.Context(), "student import: cannot extend the write deadline", "err", err)
	}
	return deadline
}

func decodeImport(w http.ResponseWriter, r *http.Request) (importRequest, bool) {
	var req importRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBody)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeCodedError(w, http.StatusRequestEntityTooLarge, codeFileTooLarge, "the file is too large")
			return req, false
		}
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return req, false
	}
	return req, true
}

// plan runs the whole preview. On failure it has already answered.
func (h *StudentImportHandler) plan(w http.ResponseWriter, ctx context.Context, req importRequest) (importRun, bool) {
	rows, digest, ok := parseUpload(w, req.FileBase64)
	if !ok {
		return importRun{}, false
	}
	run := importRun{planned: legacyimport.Plan(rows), fileDigest: digest}
	cycle, err := h.activeCycle(ctx)
	if err != nil {
		writeConnectError(w, err)
		return importRun{}, false
	}
	if cycle.ID == "" {
		writeCodedError(w, http.StatusConflict, codeNoActiveCycle, "there is no active school cycle")
		return importRun{}, false
	}
	run.resp.Cycle = importCycle{ID: cycle.ID, Name: cycle.Name}
	if err := h.resolveLevels(ctx, req, &run); err != nil {
		writeConnectError(w, err)
		return importRun{}, false
	}
	if err := h.placeDry(ctx, req, &run); err != nil {
		writeConnectError(w, err)
		return importRun{}, false
	}
	results, err := h.judgeRows(ctx, &run, nil, true, "")
	if err != nil {
		writeConnectError(w, err)
		return importRun{}, false
	}
	run.resp.Rows, run.resp.Summary = h.rows(&run, results)
	run.resp.PlanHash = planHash(run.resp, run.fileDigest)
	return run, true
}

func parseUpload(w http.ResponseWriter, encoded string) ([]legacyimport.Row, string, bool) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		writeCodedError(w, http.StatusBadRequest, codeFileNotXLSX, "the file is not an .xlsx workbook")
		return nil, "", false
	}
	sum := sha256.Sum256(raw)
	digest := hex.EncodeToString(sum[:])
	rows, err := legacyimport.Parse(strings.NewReader(string(raw)), legacyimport.DefaultLimits)
	var missing legacyimport.MissingColumnError
	switch {
	case err == nil:
		return rows, digest, true
	case errors.As(err, &missing):
		writeCodedError(w, http.StatusBadRequest, codeFileMissingColumn, missing.Column)
	case errors.Is(err, legacyimport.ErrEmpty):
		writeCodedError(w, http.StatusBadRequest, codeFileEmpty, "the sheet has no students")
	case errors.Is(err, legacyimport.ErrTooLarge), errors.Is(err, legacyimport.ErrTooManyRows):
		writeCodedError(w, http.StatusRequestEntityTooLarge, codeFileTooLarge, "the file is too large")
	default:
		writeCodedError(w, http.StatusBadRequest, codeFileNotXLSX, "the file is not an .xlsx workbook")
	}
	return nil, "", false
}

func (h *StudentImportHandler) activeCycle(ctx context.Context) (client.CycleResult, error) {
	cycles, err := h.d.Catalog.ListCycles(ctx)
	if err != nil {
		return client.CycleResult{}, err
	}
	for _, c := range cycles {
		if c.Status == statusAlive {
			return c, nil
		}
	}
	return client.CycleResult{}, nil
}

// resolveLevels decides where each level of the file goes: the office's
// choice, else an existing level of the same folded name, else a new level with
// the file's name — created on its own, like the grades and groups under it.
func (h *StudentImportHandler) resolveLevels(ctx context.Context, req importRequest, run *importRun) error {
	levels, err := h.d.Catalog.ListLevels(ctx)
	if err != nil {
		return err
	}
	byID, byFold := map[string]client.LevelResult{}, map[string]client.LevelResult{}
	for _, l := range levels {
		if l.Deleting {
			continue
		}
		byID[l.ID] = l
		byFold[foldName(l.Name)] = l
	}
	priced := h.pricedLevels(ctx, run.resp.Cycle.ID)
	run.levelOf = map[string]importLevel{}
	counts := map[string]int{}
	for key, n := range run.planned.Placements {
		counts[key.Level] += n
	}
	for _, name := range run.planned.Levels {
		lv := importLevel{FileName: name, Status: levelUnresolved, Students: counts[name]}
		choice := req.Levels[name]
		switch existing, ok := byID[choice.LevelID]; {
		case choice.LevelID != "" && ok:
			lv.Status, lv.LevelID, lv.LevelName = levelChosen, existing.ID, existing.Name
		case choice.Create:
			lv.Status, lv.LevelName = levelCreate, name
		default:
			if match, ok := byFold[foldName(name)]; ok {
				lv.Status, lv.LevelID, lv.LevelName = levelMatched, match.ID, match.Name
			} else {
				lv.Status, lv.LevelName = levelCreate, name
			}
		}
		if lv.Status != levelUnresolved && priced != nil && !priced[lv.LevelID] {
			lv.NoTuition = true
		}
		if lv.Status == levelUnresolved {
			run.resp.Unresolved = append(run.resp.Unresolved, name)
		}
		run.levelOf[name] = lv
		run.resp.Levels = append(run.resp.Levels, lv)
	}
	if run.resp.Unresolved == nil {
		run.resp.Unresolved = []string{}
	}
	return nil
}

// pricedLevels is the set of levels with an active tuition table in the cycle,
// or nil when pricing is not wired or not answering (no warning then).
func (h *StudentImportHandler) pricedLevels(ctx context.Context, cycleID string) map[string]bool {
	if h.d.Pricing == nil {
		return nil
	}
	plans, err := h.d.Pricing.ListTuitionPlans(ctx, cycleID)
	if err != nil {
		slog.WarnContext(ctx, "student import: tuition plans unavailable", "err", err)
		return nil
	}
	out := map[string]bool{}
	for _, p := range plans {
		if p.Active {
			out[p.LevelID] = true
		}
	}
	return out
}

// placeDry asks config what the resolved placements are, creating nothing.
func (h *StudentImportHandler) placeDry(ctx context.Context, req importRequest, run *importRun) error {
	for _, key := range sortedPlacements(run.planned.Placements) {
		lv := run.levelOf[key.Level]
		if lv.Status == levelUnresolved {
			continue
		}
		in := client.PlacementInput{LevelID: lv.LevelID, Ordinal: key.Ordinal, Group: key.Group,
			Capacity: capacityFor(req.Capacities, key, run.planned.Placements[key])}
		if lv.Status == levelCreate {
			in.LevelID, in.NewLevelName = "", key.Level
		}
		run.keys, run.inputs = append(run.keys, key), append(run.inputs, in)
	}
	run.placements = map[legacyimport.PlacementKey]client.PlacementResult{}
	if len(run.inputs) == 0 {
		run.resp.Placements = []importPlacement{}
		return nil
	}
	results, err := h.d.Catalog.EnsurePlacements(ctx, run.resp.Cycle.ID, run.inputs, true)
	if err != nil {
		return err
	}
	return run.recordPlacements(results)
}

func (run *importRun) recordPlacements(results []client.PlacementResult) error {
	if len(results) != len(run.keys) {
		return fmt.Errorf("student import: config answered %d placements for %d", len(results), len(run.keys))
	}
	run.resp.Placements = run.resp.Placements[:0]
	for i, key := range run.keys {
		p := results[i]
		run.placements[key] = p
		run.resp.Placements = append(run.resp.Placements, importPlacement{
			Key: placementKey(key), Level: run.levelOf[key.Level].LevelName, Grade: gradeName(p, key.Ordinal),
			Group: key.Group, Students: run.planned.Placements[key], Capacity: p.Capacity,
			LevelCreated: p.LevelCreated, GradeCreated: p.GradeCreated, GroupCreated: p.GroupCreated, Error: p.Error,
		})
	}
	return nil
}

// judgeRows sends every row that has somewhere to go to students. groups maps
// a placement to its real group id (commit); nil means validating.
func (h *StudentImportHandler) judgeRows(ctx context.Context, run *importRun,
	groups map[legacyimport.PlacementKey]string, validate bool, batch string,
) (map[int]client.ImportRowResult, error) {
	var rows []client.ImportRowInput
	for _, c := range run.planned.Candidates {
		key, ok := run.sendable(c)
		if !ok {
			continue
		}
		in := importRowInput(c)
		in.GroupID = run.placements[key].GroupID
		if groups != nil {
			in.GroupID = groups[key]
		}
		rows = append(rows, in)
	}
	out := map[int]client.ImportRowResult{}
	if len(rows) == 0 {
		return out, nil
	}
	if validate {
		results, err := h.d.Roster.ImportStudents(ctx, rows, true, batch)
		if err != nil {
			return nil, err
		}
		record(out, results)
		return out, nil
	}
	return out, h.writeInChunks(ctx, rows, batch, out)
}

// writeInChunks sends the pupils to students importChunk at a time and stops
// starting chunks when the time left would not fit another one. Every pupil is
// its own transaction in students, so what was written stays written; the rows
// never answered come back as "not_answered", and uploading the same file again
// finishes them ("ya estaba" for the rest). Far from its database a pupil costs
// several round trips: the load test against Cloud SQL from outside the region
// wrote 128 of 1,000 before the old single call hit its deadline and answered a
// 500 that lost even those.
func (h *StudentImportHandler) writeInChunks(ctx context.Context, rows []client.ImportRowInput, batch string, out map[int]client.ImportRowResult) error {
	deadline, bounded := ctx.Deadline()
	var slowest time.Duration
	for start := 0; start < len(rows); start += importChunk {
		if bounded && start > 0 && time.Until(deadline) < 2*slowest {
			slog.WarnContext(ctx, "student import: out of time, stopping between chunks", "written", start, "rows", len(rows))
			return nil
		}
		began := time.Now()
		results, err := h.d.Roster.ImportStudents(ctx, rows[start:min(start+importChunk, len(rows))], false, batch)
		if err != nil {
			if start > 0 && ctx.Err() != nil {
				// The chunk in flight may be partly written; its rows read as
				// unanswered and the next upload settles them.
				return nil
			}
			return err
		}
		record(out, results)
		slowest = max(slowest, time.Since(began))
	}
	return nil
}

func record(out map[int]client.ImportRowResult, results []client.ImportRowResult) {
	for _, r := range results {
		out[r.RowNumber] = r
	}
}

// sendable is a candidate students should judge: not skipped here, its level
// resolved and its placement without error.
func (run *importRun) sendable(c legacyimport.Candidate) (legacyimport.PlacementKey, bool) {
	if c.Skip != "" {
		return legacyimport.PlacementKey{}, false
	}
	key := legacyimport.PlacementKey{Level: run.planned.LevelOf(c), Ordinal: c.Ordinal, Group: c.Row.GroupName}
	if run.levelOf[key.Level].Status == levelUnresolved {
		return key, false
	}
	p, ok := run.placements[key]
	return key, ok && p.Error == ""
}

func importRowInput(c legacyimport.Candidate) client.ImportRowInput {
	var phones []client.StudentPhone
	if c.Row.HomePhone != "" {
		phones = append(phones, client.StudentPhone{Kind: "casa", Number: c.Row.HomePhone})
	}
	if c.Row.MobilePhone != "" {
		phones = append(phones, client.StudentPhone{Kind: "celular", Number: c.Row.MobilePhone})
	}
	return client.ImportRowInput{
		RowNumber: c.Row.Number, CURP: c.Row.CURP, FirstName: c.Name.First, LastName: c.Name.Paternal,
		MaternalLastName: c.Name.Maternal, BankReference: c.BankRef,
		Profile: client.StudentProfile{
			BirthDate: c.BirthDate, Sex: c.Sex, Email: c.Row.Email, Phones: phones,
			Address: client.StudentAddress{Street: c.Row.Street, Municipality: c.Row.Municipality,
				State: c.Row.State, PostalCode: c.Row.PostalCode},
		},
	}
}

// rows builds the per-row answer and its summary from the candidates and
// students' outcomes.
func (h *StudentImportHandler) rows(run *importRun, results map[int]client.ImportRowResult) ([]importRow, importSummary) {
	out := make([]importRow, 0, len(run.planned.Candidates))
	var sum importSummary
	for _, c := range run.planned.Candidates {
		row := importRow{Row: c.Row.Number, Name: c.Row.FullName, CURP: c.Row.CURP, Warnings: c.Warnings}
		key, sendable := run.sendable(c)
		if c.Skip == "" {
			row.Destination = run.destination(key)
		}
		res, judged := results[c.Row.Number]
		switch {
		case c.Skip != "":
			row.Status, row.Reason = rowSkipped, c.Skip
		case run.levelOf[key.Level].Status == levelUnresolved:
			row.Status, row.Reason = rowPending, "level_unresolved"
		case !sendable:
			row.Status, row.Reason = rowSkipped, run.placements[key].Error
		case !judged:
			row.Status, row.Reason = rowSkipped, reasonNotAnswered
		default:
			row.Warnings = append(row.Warnings, res.Warnings...)
			row.EnrollmentNumber = res.EnrollmentNumber
			row.Status, row.Reason = outcomeStatus(res, row.Warnings)
		}
		tally(&sum, row.Status)
		out = append(out, row)
	}
	return out, sum
}

func outcomeStatus(res client.ImportRowResult, warnings []string) (string, string) {
	switch res.Outcome {
	case client.ImportImported:
		if len(warnings) > 0 {
			return rowWarning, ""
		}
		return rowNew, ""
	case client.ImportAlreadyExists, client.ImportAlreadyImported:
		return rowAlready, res.Outcome
	default:
		return rowSkipped, res.Reason
	}
}

func tally(sum *importSummary, status string) {
	sum.Total++
	switch status {
	case rowNew:
		sum.New++
	case rowWarning:
		sum.New++
		sum.Warnings++
	case rowAlready:
		sum.Already++
	case rowPending:
		sum.Pending++
	default:
		sum.Skipped++
	}
}

func (run *importRun) destination(key legacyimport.PlacementKey) string {
	lv := run.levelOf[key.Level]
	name := lv.LevelName
	if name == "" {
		name = key.Level
	}
	return fmt.Sprintf("%s · %s · %s", name, gradeName(run.placements[key], key.Ordinal), key.Group)
}

// write commits the run: the catalog it needs, then the pupils.
func (h *StudentImportHandler) write(ctx context.Context, run importRun, batch string) (importResponse, error) {
	resp := run.resp
	var keys []legacyimport.PlacementKey
	var inputs []client.PlacementInput
	used := map[legacyimport.PlacementKey]bool{}
	for _, c := range run.planned.Candidates {
		if key, ok := run.sendable(c); ok {
			used[key] = true
		}
	}
	for i, key := range run.keys {
		if used[key] {
			keys, inputs = append(keys, key), append(inputs, run.inputs[i])
		}
	}
	groups := map[legacyimport.PlacementKey]string{}
	if len(inputs) > 0 {
		results, err := h.d.Catalog.EnsurePlacements(ctx, resp.Cycle.ID, inputs, false)
		if err != nil {
			return importResponse{}, err
		}
		if len(results) != len(keys) {
			return importResponse{}, fmt.Errorf("student import: config answered %d placements for %d", len(results), len(keys))
		}
		run.adoptRealPlacements(keys, results)
		for i, key := range keys {
			groups[key] = results[i].GroupID
		}
		resp.Placements = run.resp.Placements
	}
	results, err := h.judgeRows(ctx, &run, groups, false, batch)
	if err != nil {
		return importResponse{}, err
	}
	resp.Rows, resp.Summary = h.rows(&run, results)
	resp.BatchID = batch
	return resp, nil
}

// adoptRealPlacements replaces the dry run's answer with what config actually
// did: a level deleted meanwhile is an error the rows must carry, and what
// already existed by then was adopted, not created.
func (run *importRun) adoptRealPlacements(keys []legacyimport.PlacementKey, results []client.PlacementResult) {
	at := make(map[string]int, len(run.resp.Placements))
	for i, p := range run.resp.Placements {
		at[p.Key] = i
	}
	for i, key := range keys {
		p := results[i]
		run.placements[key] = p
		shown := &run.resp.Placements[at[placementKey(key)]]
		shown.Capacity, shown.Error = p.Capacity, p.Error
		shown.LevelCreated, shown.GradeCreated, shown.GroupCreated = p.LevelCreated, p.GradeCreated, p.GroupCreated
	}
}

// planHash fingerprints what the office approves: THIS file (its bytes), the
// cycle, where each level goes, what each placement creates and each row's
// fate. Warnings and the tuition advice are left out — they do not change what
// is written, and finance answering at preview but not at commit must not read
// as a changed plan.
func planHash(r importResponse, fileDigest string) string {
	type rowFate struct {
		Row                 int
		Status, Reason, Dst string
	}
	fates := make([]rowFate, 0, len(r.Rows))
	for _, row := range r.Rows {
		fates = append(fates, rowFate{row.Row, row.Status, row.Reason, row.Destination})
	}
	levels := make([]importLevel, len(r.Levels))
	for i, l := range r.Levels {
		l.NoTuition = false
		levels[i] = l
	}
	// Marshal cannot fail: every field is a string, int, bool or slice of them.
	raw, _ := json.Marshal(struct {
		File       string
		Cycle      string
		Levels     []importLevel
		Placements []importPlacement
		Rows       []rowFate
	}{fileDigest, r.Cycle.ID, levels, r.Placements, fates})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func sortedPlacements(m map[legacyimport.PlacementKey]int) []legacyimport.PlacementKey {
	keys := make([]legacyimport.PlacementKey, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Deterministic order, so the same file always hashes the same.
	slices.SortFunc(keys, func(a, b legacyimport.PlacementKey) int {
		return cmp.Compare(placementKey(a), placementKey(b))
	})
	return keys
}

func placementKey(k legacyimport.PlacementKey) string {
	return fmt.Sprintf("%s|%02d|%s", k.Level, k.Ordinal, k.Group)
}

func capacityFor(requested map[string]int, key legacyimport.PlacementKey, pupils int) int {
	if c, ok := requested[placementKey(key)]; ok && c >= pupils && c > 0 {
		return min(c, maxGroupCapacity)
	}
	return max(pupils, defaultGroupCapacity)
}

func gradeName(p client.PlacementResult, ordinal int) string {
	if p.GradeName != "" {
		return p.GradeName
	}
	return fmt.Sprintf("%d°", ordinal)
}

// foldName compares level names the way config's unique index does: case,
// accents and spacing ignored.
func foldName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(legacyimport.Fold(s))), " ")
}
