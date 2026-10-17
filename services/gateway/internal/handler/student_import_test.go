package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/legacyimport"
)

// --- fakes -----------------------------------------------------------------

type fakeImportCatalog struct {
	cycles     []client.CycleResult
	levels     []client.LevelResult
	placements []placementCall
	// commitError makes the real (not dry) call answer this code for every
	// placement, as config does when a level started being deleted meanwhile.
	commitError string
}

type placementCall struct {
	cycleID string
	in      []client.PlacementInput
	dry     bool
}

func (f *fakeImportCatalog) ListCycles(context.Context) ([]client.CycleResult, error) {
	return f.cycles, nil
}

func (f *fakeImportCatalog) ListLevels(context.Context) ([]client.LevelResult, error) {
	return f.levels, nil
}

// EnsurePlacements answers like config: an existing level keeps its id, a new
// one gets an id only when not dry; every group is "created".
func (f *fakeImportCatalog) EnsurePlacements(_ context.Context, cycleID string, in []client.PlacementInput, dry bool) ([]client.PlacementResult, error) {
	f.placements = append(f.placements, placementCall{cycleID, in, dry})
	out := make([]client.PlacementResult, 0, len(in))
	for _, p := range in {
		r := client.PlacementResult{LevelID: p.LevelID, LevelName: p.NewLevelName, GroupName: p.Group,
			GradeName: "1°", Capacity: p.Capacity, GroupCreated: true, GradeCreated: true}
		if p.LevelID == "" {
			r.LevelCreated = true
		}
		if !dry && f.commitError != "" {
			r.Error, r.GroupCreated, r.GradeCreated, r.LevelCreated = f.commitError, false, false, false
			out = append(out, r)
			continue
		}
		if !dry {
			if r.LevelID == "" {
				r.LevelID = "lv-new"
			}
			r.GroupID = "grp-" + p.Group
		}
		out = append(out, r)
	}
	return out, nil
}

type fakeImportRoster struct {
	calls []rosterCall
}

type rosterCall struct {
	rows     []client.ImportRowInput
	validate bool
	batch    string
}

func (f *fakeImportRoster) ImportStudents(_ context.Context, rows []client.ImportRowInput, validate bool, batch string) ([]client.ImportRowResult, error) {
	f.calls = append(f.calls, rosterCall{rows, validate, batch})
	out := make([]client.ImportRowResult, 0, len(rows))
	for _, r := range rows {
		res := client.ImportRowResult{RowNumber: r.RowNumber, Outcome: client.ImportImported}
		if r.CURP == "KNOWN000000000000A" {
			res.Outcome = client.ImportAlreadyExists
		}
		if !validate {
			res.EnrollmentNumber = "2026-00001"
		}
		out = append(out, res)
	}
	return out, nil
}

type fakeImportPricing struct {
	plans []client.TuitionPlan
	err   error
}

func (f *fakeImportPricing) ListTuitionPlans(context.Context, string) ([]client.TuitionPlan, error) {
	return f.plans, f.err
}

// --- fixture ----------------------------------------------------------------

var importHeader = []any{"Matrícula Interna", "Nombre", "Escolaridad", "Grado / Nivel", "Grupo", "Estado",
	"Cumpleaños", "CURP", "Email", "Telefono de Casa", "Número de Celular", "Dirección", "Estado", "Ciudad/Municipio", "Código Postal"}

func importFile(t *testing.T, rows ...[]any) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	sheet := f.GetSheetName(0)
	all := append([][]any{importHeader}, rows...)
	for i, r := range all {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		_ = f.SetSheetRow(sheet, cell, &r)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("write workbook: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func pupil(level, group, status, curp string) []any {
	return []any{"231076", "ALCAZAR MELENDEZ, IRLANDA", level, "1", group, status, "19/02/2021", curp,
		"juan@x.mx", "3314626871", "3311105850", "LERDO 35", "Jalisco", "Tlajomulco", "45640"}
}

type importFixture struct {
	h       *StudentImportHandler
	catalog *fakeImportCatalog
	roster  *fakeImportRoster
	pricing *fakeImportPricing
}

func newImportFixture() importFixture {
	catalog := &fakeImportCatalog{
		cycles: []client.CycleResult{{ID: "cyc-1", Name: "2026-2027", Status: "active"}},
		levels: []client.LevelResult{{ID: "lv-pri", Name: "Primaria"}, {ID: "lv-bach", Name: "Bachillerato"}},
	}
	roster := &fakeImportRoster{}
	pricing := &fakeImportPricing{plans: []client.TuitionPlan{{LevelID: "lv-pri", Active: true}}}
	h := NewStudentImportHandler(StudentImportDeps{
		Catalog: catalog, Roster: roster, Pricing: pricing,
		NewID:       func() string { return "batch-1" },
		WriteBudget: 2 * time.Minute,
	})
	return importFixture{h, catalog, roster, pricing}
}

func post(t *testing.T, fn http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	fn(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))
	return rec
}

func decodePreview(t *testing.T, rec *httptest.ResponseRecorder) importResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var out importResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// --- preview ----------------------------------------------------------------

func TestPreviewWritesNothingAndReportsEveryRow(t *testing.T) {
	f := newImportFixture()
	file := importFile(t,
		pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"),
		pupil("primaria", "A", "Activo", "KNOWN000000000000A"),
		pupil("Primaria", "B", "Baja", "AAMI210219MJCLLRA0"))

	got := decodePreview(t, post(t, f.h.Preview, importRequest{FileName: "Primaria.xlsx", FileBase64: file}))

	if len(f.catalog.placements) != 1 || !f.catalog.placements[0].dry {
		t.Fatalf("placements calls = %+v, want one dry run", f.catalog.placements)
	}
	if len(f.roster.calls) != 1 || !f.roster.calls[0].validate {
		t.Fatalf("roster calls = %+v, want one validate_only", f.roster.calls)
	}
	if got.Summary != (importSummary{Total: 3, New: 1, Already: 1, Skipped: 1}) {
		t.Errorf("summary = %+v", got.Summary)
	}
	if got.Cycle.ID != "cyc-1" || got.PlanHash == "" {
		t.Errorf("cycle %+v hash %q", got.Cycle, got.PlanHash)
	}
	if got.Levels[0].Status != levelMatched || got.Levels[0].LevelID != "lv-pri" || got.Levels[0].NoTuition {
		t.Errorf("level = %+v", got.Levels[0])
	}
	if got.Rows[2].Status != rowSkipped || got.Rows[2].Reason != "status_withdrawn" {
		t.Errorf("baja row = %+v", got.Rows[2])
	}
	if got.Rows[0].Destination != "Primaria · 1° · A" {
		t.Errorf("destination = %q", got.Rows[0].Destination)
	}
}

// A level the school does not have is created on its own, like its grades and
// groups: the office is never asked where its pupils go.
func TestPreviewCreatesALevelTheSchoolDoesNotHave(t *testing.T) {
	f := newImportFixture()
	file := importFile(t, pupil("Preparatoria", "A", "Activo", "AAMI210219MJCLLRA0"))

	got := decodePreview(t, post(t, f.h.Preview, importRequest{FileBase64: file}))

	if got.Levels[0].Status != levelCreate || got.Levels[0].LevelName != "PREPARATORIA" || len(got.Unresolved) != 0 {
		t.Fatalf("levels = %+v unresolved %v, want PREPARATORIA created, in capitals like a typed level (ISSUE-T14-008)", got.Levels, got.Unresolved)
	}
	if got.Rows[0].Status != rowNew {
		t.Errorf("row = %+v, want new", got.Rows[0])
	}
	in := f.catalog.placements[0].in
	if len(in) != 1 || in[0].LevelID != "" || in[0].NewLevelName != "Preparatoria" {
		t.Errorf("placements = %+v, want Preparatoria asked to be created", in)
	}
}

func TestPreviewFollowsTheOfficesLevelChoices(t *testing.T) {
	f := newImportFixture()
	file := importFile(t, pupil("Preparatoria", "A", "Activo", "AAMI210219MJCLLRA0"),
		pupil("Maternal", "A", "Activo", "AAMI210219MJCLLRA1"))

	got := decodePreview(t, post(t, f.h.Preview, importRequest{FileBase64: file, Levels: map[string]levelChoice{
		"Preparatoria": {LevelID: "lv-bach"}, "Maternal": {Create: true},
	}}))

	byName := map[string]importLevel{}
	for _, l := range got.Levels {
		byName[l.FileName] = l
	}
	if byName["Preparatoria"].Status != levelChosen || byName["Preparatoria"].LevelName != "Bachillerato" {
		t.Errorf("Preparatoria = %+v", byName["Preparatoria"])
	}
	if byName["Maternal"].Status != levelCreate || !byName["Maternal"].NoTuition {
		t.Errorf("Maternal = %+v, want created and flagged without tuition", byName["Maternal"])
	}
	in := f.catalog.placements[0].in
	if in[0].LevelID != "lv-bach" && in[1].LevelID != "lv-bach" {
		t.Errorf("placements = %+v, want Preparatoria sent to lv-bach", in)
	}
	for _, p := range in {
		if p.Capacity != defaultGroupCapacity {
			t.Errorf("capacity = %d, want the default %d for a small group", p.Capacity, defaultGroupCapacity)
		}
	}
}

func TestPreviewWithoutAnActiveCycleAsksForOne(t *testing.T) {
	f := newImportFixture()
	f.catalog.cycles = []client.CycleResult{{ID: "c0", Status: "draft"}}

	rec := post(t, f.h.Preview, importRequest{FileBase64: importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))})

	if rec.Code != http.StatusConflict || rec.Header().Get(errorCodeHeader) != codeNoActiveCycle {
		t.Errorf("status %d code %q", rec.Code, rec.Header().Get(errorCodeHeader))
	}
}

func TestPreviewNamesWhatIsWrongWithTheFile(t *testing.T) {
	f := newImportFixture()
	cases := map[string]struct {
		file   string
		status int
		code   string
	}{
		"not base64": {"%%%", http.StatusBadRequest, codeFileNotXLSX},
		"not xlsx":   {base64.StdEncoding.EncodeToString([]byte("a,b")), http.StatusBadRequest, codeFileNotXLSX},
		"empty":      {importFile(t), http.StatusBadRequest, codeFileEmpty},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := post(t, f.h.Preview, importRequest{FileBase64: tc.file})
			if rec.Code != tc.status || rec.Header().Get(errorCodeHeader) != tc.code {
				t.Errorf("status %d code %q, want %d %q", rec.Code, rec.Header().Get(errorCodeHeader), tc.status, tc.code)
			}
		})
	}
}

// --- commit -----------------------------------------------------------------

func TestCommitRefusesAStalePreview(t *testing.T) {
	f := newImportFixture()
	file := importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))

	rec := post(t, f.h.Commit, importRequest{FileBase64: file, PlanHash: "not-the-hash"})

	if rec.Code != http.StatusConflict || rec.Header().Get(errorCodeHeader) != codePreviewStale {
		t.Fatalf("status %d code %q", rec.Code, rec.Header().Get(errorCodeHeader))
	}
	for _, c := range f.catalog.placements {
		if !c.dry {
			t.Error("a stale commit wrote the catalog")
		}
	}
	for _, c := range f.roster.calls {
		if !c.validate {
			t.Error("a stale commit wrote pupils")
		}
	}
}

func TestCommitCreatesALevelTheSchoolDoesNotHave(t *testing.T) {
	f := newImportFixture()
	req := importRequest{FileBase64: importFile(t, pupil("Preparatoria", "A", "Activo", "AAMI210219MJCLLRA0"))}
	req.PlanHash = decodePreview(t, post(t, f.h.Preview, req)).PlanHash

	rec := post(t, f.h.Commit, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d code %q, want 200", rec.Code, rec.Header().Get(errorCodeHeader))
	}
	last := f.catalog.placements[len(f.catalog.placements)-1]
	if last.dry || len(last.in) != 1 || last.in[0].NewLevelName != "Preparatoria" {
		t.Errorf("commit placements = %+v, want a real call creating Preparatoria", last)
	}
}

func TestCommitImportsWhatThePreviewShowed(t *testing.T) {
	f := newImportFixture()
	file := importFile(t,
		pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"),
		pupil("Primaria", "B", "Baja", "AAMI210219MJCLLRA0"))
	req := importRequest{FileBase64: file}
	req.PlanHash = decodePreview(t, post(t, f.h.Preview, req)).PlanHash

	got := decodePreview(t, post(t, f.h.Commit, req))

	last := f.catalog.placements[len(f.catalog.placements)-1]
	if last.dry || len(last.in) != 1 {
		t.Fatalf("commit placements = %+v, want one real call for the one placement used", last)
	}
	write := f.roster.calls[len(f.roster.calls)-1]
	if write.validate || write.batch != "batch-1" || len(write.rows) != 1 || write.rows[0].GroupID != "grp-A" {
		t.Fatalf("commit roster call = %+v", write)
	}
	r := write.rows[0]
	if r.LastName != "ALCAZAR" || r.MaternalLastName != "MELENDEZ" || r.BankReference != "231076" ||
		r.Profile.Sex != "F" || r.Profile.BirthDate != "2021-02-19" || len(r.Profile.Phones) != 2 {
		t.Errorf("row sent = %+v", r)
	}
	if got.Summary.New != 1 || got.Summary.Skipped != 1 || got.Rows[0].EnrollmentNumber != "2026-00001" || got.BatchID != "batch-1" {
		t.Errorf("result = %+v", got)
	}
}

// --- review fixes ------------------------------------------------------------

// deadlineRecorder lets the test see the write deadline the handler asks for
// (http.ResponseController looks for this method).
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error { d.deadline = t; return nil }

// The commit re-plans before writing, so the server-wide 30 s write timeout is
// not its budget: the route sets its own deadline first, and the detached
// commit stays inside it.
func TestCommitSetsItsOwnWriteDeadline(t *testing.T) {
	f := newImportFixture()
	req := importRequest{FileBase64: importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))}
	req.PlanHash = decodePreview(t, post(t, f.h.Preview, req)).PlanHash
	raw, _ := json.Marshal(req)
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	before := time.Now()

	f.h.Commit(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.deadline.Before(before.Add(2*time.Minute-time.Second)) || rec.deadline.After(time.Now().Add(2*time.Minute)) {
		t.Errorf("write deadline = %s from now, want the route's 2m budget", rec.deadline.Sub(before))
	}
}

// What the office approved is the FILE, not only its layout: a different file
// with the same rows in the same places must not ride the old hash.
func TestCommitRefusesAnotherFileWithTheSameLayout(t *testing.T) {
	f := newImportFixture()
	a := importRequest{FileBase64: importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))}
	hash := decodePreview(t, post(t, f.h.Preview, a)).PlanHash
	other := pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0")
	other[1] = "OTRO APELLIDO, OTRO NOMBRE"

	rec := post(t, f.h.Commit, importRequest{FileBase64: importFile(t, other), PlanHash: hash})

	if rec.Code != http.StatusConflict || rec.Header().Get(errorCodeHeader) != codePreviewStale {
		t.Errorf("status %d code %q, want 409 PREVIEW_STALE", rec.Code, rec.Header().Get(errorCodeHeader))
	}
}

// The tuition warning is advice: finance going quiet between preview and
// commit must not make the plan look changed.
func TestCommitIgnoresTuitionAdviceThatDrifted(t *testing.T) {
	f := newImportFixture()
	req := importRequest{FileBase64: importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))}
	req.PlanHash = decodePreview(t, post(t, f.h.Preview, req)).PlanHash
	f.pricing.err = errors.New("finance timed out")

	rec := post(t, f.h.Commit, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d (%s), want the commit to go through", rec.Code, rec.Header().Get(errorCodeHeader))
	}
}

// A placement that fails for real (the level started being deleted after the
// preview) is reported as such, and its rows say why.
func TestCommitReportsAPlacementThatFailedForReal(t *testing.T) {
	f := newImportFixture()
	req := importRequest{FileBase64: importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))}
	req.PlanHash = decodePreview(t, post(t, f.h.Preview, req)).PlanHash
	f.catalog.commitError = "level_being_deleted"

	got := decodePreview(t, post(t, f.h.Commit, req))

	if got.Placements[0].Error != "level_being_deleted" || got.Placements[0].GroupCreated {
		t.Errorf("placement = %+v, want the real error and nothing created", got.Placements[0])
	}
	if got.Rows[0].Status != rowSkipped || got.Rows[0].Reason != "level_being_deleted" {
		t.Errorf("row = %+v", got.Rows[0])
	}
	for _, c := range f.roster.calls {
		if !c.validate {
			t.Error("rows of a failed placement were written")
		}
	}
}

// blockingRoster holds the real (not validating) call until released, so a
// second commit can arrive while the first is writing.
type blockingRoster struct {
	fakeImportRoster
	entered, release chan struct{}
}

func (b *blockingRoster) ImportStudents(ctx context.Context, rows []client.ImportRowInput, validate bool, batch string) ([]client.ImportRowResult, error) {
	if !validate {
		b.entered <- struct{}{}
		<-b.release
	}
	return b.fakeImportRoster.ImportStudents(ctx, rows, validate, batch)
}

// Two tabs pressing Importar at once must not run two commits side by side
// against the same school: the second is told one is already running.
func TestCommitRunsOneImportAtATime(t *testing.T) {
	f := newImportFixture()
	roster := &blockingRoster{entered: make(chan struct{}, 2), release: make(chan struct{})}
	f.h.d.Roster = roster
	req := importRequest{FileBase64: importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))}
	req.PlanHash = decodePreview(t, post(t, f.h.Preview, req)).PlanHash

	done := make(chan *httptest.ResponseRecorder)
	go func() { done <- post(t, f.h.Commit, req) }()
	<-roster.entered

	second := post(t, f.h.Commit, req)
	close(roster.release)
	first := <-done

	if second.Code != http.StatusConflict || second.Header().Get(errorCodeHeader) != codeImportInProgress {
		t.Errorf("second commit: status %d code %q, want 409 %s", second.Code, second.Header().Get(errorCodeHeader), codeImportInProgress)
	}
	if first.Code != http.StatusOK {
		t.Errorf("first commit: status %d", first.Code)
	}
	if third := post(t, f.h.Commit, req); third.Code != http.StatusOK {
		t.Errorf("after the first ended, a commit must run again: status %d", third.Code)
	}
}

// A capacity typed in the preview is bounded: never below the pupils the group
// receives, never past what a classroom holds (and int32 on the wire).
func TestCapacityIsBounded(t *testing.T) {
	key := legacyimport.PlacementKey{Level: "Primaria", Ordinal: 1, Group: "A"}
	k := placementKey(key)
	for name, tc := range map[string]struct{ asked, pupils, want int }{
		"default":            {0, 10, defaultGroupCapacity},
		"asked within range": {40, 10, 40},
		"below the pupils":   {5, 10, defaultGroupCapacity},
		"absurd":             {1 << 33, 10, maxGroupCapacity},
	} {
		asked := map[string]int{}
		if tc.asked != 0 {
			asked[k] = tc.asked
		}
		if got := capacityFor(asked, key, tc.pupils); got != tc.want {
			t.Errorf("%s: capacity = %d, want %d", name, got, tc.want)
		}
	}
}

// slowRoster takes `per` for every real (not validating) call, like students
// far from its database, and records how many rows each call carried.
type slowRoster struct {
	fakeImportRoster
	per    time.Duration
	chunks []int
}

func (s *slowRoster) ImportStudents(ctx context.Context, rows []client.ImportRowInput, validate bool, batch string) ([]client.ImportRowResult, error) {
	if !validate {
		s.chunks = append(s.chunks, len(rows))
		select {
		case <-time.After(s.per):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.fakeImportRoster.ImportStudents(ctx, rows, validate, batch)
}

// A file too big for the time left is written in chunks and stops cleanly:
// the office gets what went in and which rows are still to do — re-uploading
// the same file finishes them — instead of a 500 that loses the whole answer.
func TestCommitStopsBetweenChunksWhenTimeRunsOut(t *testing.T) {
	f := newImportFixture()
	roster := &slowRoster{per: 300 * time.Millisecond}
	f.h.d.Roster = roster
	f.h.d.WriteBudget = commitAnswerMargin + 700*time.Millisecond
	var rows [][]any
	for i := 0; i < 3*importChunk; i++ {
		curp := fmt.Sprintf("AAMI21%04dMJCLLRA0", i%10000)
		rows = append(rows, pupil("Primaria", "A", "Activo", curp[:6]+fmt.Sprintf("%02d%02d", 1+i%12, 1+i%28)+curp[10:]))
	}
	req := importRequest{FileBase64: importFile(t, rows...)}
	req.PlanHash = decodePreview(t, post(t, f.h.Preview, req)).PlanHash

	got := decodePreview(t, post(t, f.h.Commit, req))

	if len(roster.chunks) == 0 || roster.chunks[0] != importChunk {
		t.Fatalf("chunks = %v, want chunks of %d", roster.chunks, importChunk)
	}
	if len(roster.chunks) >= 3 {
		t.Fatalf("chunks = %v: it kept sending after the time ran out", roster.chunks)
	}
	if got.Summary.New == 0 || got.Summary.New >= 3*importChunk {
		t.Errorf("summary = %+v, want some but not all imported", got.Summary)
	}
	var unanswered int
	for _, r := range got.Rows {
		if r.Reason == reasonNotAnswered {
			unanswered++
		}
	}
	if unanswered+got.Summary.New != 3*importChunk {
		t.Errorf("imported %d + not answered %d, want every row accounted for (%d)", got.Summary.New, unanswered, 3*importChunk)
	}
}

// The preview re-reads the whole file and asks config and students about every
// group and row; far from the database that outlives the server's 30 s write
// timeout, so it gets the same route deadline as the commit.
func TestPreviewSetsItsOwnWriteDeadline(t *testing.T) {
	f := newImportFixture()
	raw, _ := json.Marshal(importRequest{FileBase64: importFile(t, pupil("Primaria", "A", "Activo", "AAMI210219MJCLLRA0"))})
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	before := time.Now()

	f.h.Preview(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw)))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.deadline.Before(before.Add(2*time.Minute - time.Second)) {
		t.Errorf("write deadline = %s from now, want the route's 2m budget", rec.deadline.Sub(before))
	}
}
