package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/handler"
	"github.com/garcia-lorca/services/gateway/internal/middleware"
)

const (
	delLevel = "aaaaaaaa-0000-0000-0000-0000000000a1"
	delBatch = "bbbbbbbb-0000-0000-0000-0000000000b1"
)

var delNow = time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)

// sagaLog records every archive/restore call, in order, across all fakes.
type sagaLog struct{ calls []string }

func (l *sagaLog) add(s string) { l.calls = append(l.calls, s) }

// fakeLevelConfig is config's side of the saga.
type fakeLevelConfig struct {
	log          *sagaLog
	structure    client.LevelStructure
	previewErr   error
	beginErr     error
	begun        client.BeginLevelDeletionRecord
	archiveErr   error
	restoreErr   error
	summary      client.LevelCounts
	deletion     client.LevelDeletion
	getErr       error
	levels       []client.LevelResult
	marked       bool
	markedPart   bool
	cancelled    bool
	failed       bool
	failPending  []string
	failSkipped  []client.LevelRestoreSkip
	failErr      error
	sweeps       int
	expired      []client.LevelDeletion
	purgedMarked []string
	restoreSkips []client.LevelRestoreSkip
	// restoreStartErr refuses BeginLevelRestore; restoringFrom overrides the
	// status the restore starts from (default: the Get's status).
	restoreStartErr error
	restoringFrom   string
	abandoned       bool
	markErr         error
}

func (f *fakeLevelConfig) BeginLevelRestore(context.Context, string) (client.LevelDeletion, error) {
	f.log.add("restore-start:config")
	if f.restoreStartErr != nil {
		return client.LevelDeletion{}, f.restoreStartErr
	}
	rec := f.deletion
	rec.RestoringFrom = rec.Status
	if f.restoringFrom != "" {
		rec.RestoringFrom = f.restoringFrom
	}
	rec.Status = "restoring"
	return rec, nil
}

func (f *fakeLevelConfig) AbandonLevelRestore(context.Context, string) (client.LevelDeletion, error) {
	f.log.add("restore-abandon:config")
	f.abandoned = true
	return f.deletion, nil
}

func (f *fakeLevelConfig) PreviewLevelDeletion(context.Context, string) (client.LevelStructure, error) {
	return f.structure, f.previewErr
}

func (f *fakeLevelConfig) BeginLevelDeletion(_ context.Context, in client.BeginLevelDeletionRecord) (client.LevelDeletion, error) {
	f.log.add("begin:config")
	f.begun = in
	if f.beginErr != nil {
		return client.LevelDeletion{}, f.beginErr
	}
	return client.LevelDeletion{BatchID: in.BatchID, LevelID: in.LevelID, Status: "deleting",
		DeletedAt: delNow, RestorableUntil: delNow.Add(7 * 24 * time.Hour)}, nil
}

func (f *fakeLevelConfig) ArchiveLevel(_ context.Context, in client.ArchiveLevelRecord) (client.LevelCounts, client.LevelDeletion, error) {
	f.log.add("archive:config")
	if f.archiveErr != nil {
		return nil, client.LevelDeletion{}, f.archiveErr
	}
	f.summary = in.Summary
	total := client.LevelCounts{}
	total.Add(in.Summary)
	total.Add(f.structure.Counts)
	return f.structure.Counts, client.LevelDeletion{
		BatchID: in.BatchID, LevelID: in.LevelID, LevelName: f.structure.LevelName, Counts: total,
		DeletedAt: delNow, RestorableUntil: delNow.Add(7 * 24 * time.Hour), Status: "deleted",
	}, nil
}

func (f *fakeLevelConfig) RestoreLevelBatch(ctx context.Context, _ string) (client.LevelRestoreReport, error) {
	f.log.add("restore:config" + ctxMark(ctx))
	if f.restoreErr != nil {
		return client.LevelRestoreReport{}, f.restoreErr
	}
	return client.LevelRestoreReport{Restored: client.LevelCounts{"levels": 1, "groups": 2}, Skipped: f.restoreSkips}, nil
}

func (f *fakeLevelConfig) MarkLevelDeletionRestored(_ context.Context, _ string, partial bool) (client.LevelDeletion, error) {
	f.log.add("mark:config")
	if f.markErr != nil {
		return client.LevelDeletion{}, f.markErr
	}
	f.marked, f.markedPart = true, partial
	return client.LevelDeletion{Status: "restored"}, nil
}

func (f *fakeLevelConfig) CancelLevelDeletion(context.Context, string) (client.LevelDeletion, error) {
	f.log.add("cancel:config")
	f.cancelled = true
	return client.LevelDeletion{Status: "cancelled"}, nil
}

func (f *fakeLevelConfig) FailLevelDeletion(_ context.Context, _ string, pending []string, skipped []client.LevelRestoreSkip) (client.LevelDeletion, error) {
	f.log.add("fail:config")
	f.failed, f.failPending, f.failSkipped = true, pending, skipped
	return client.LevelDeletion{Status: "failed_partial"}, f.failErr
}

func (f *fakeLevelConfig) SweepLevelDeletions(context.Context) ([]client.LevelDeletion, int64, error) {
	f.sweeps++
	return f.expired, 0, nil
}

func (f *fakeLevelConfig) MarkLevelDeletionPurged(_ context.Context, batchID string) (client.LevelDeletion, error) {
	f.purgedMarked = append(f.purgedMarked, batchID)
	return client.LevelDeletion{BatchID: batchID, Status: "purged"}, nil
}

func (f *fakeLevelConfig) PurgeLevelBatch(_ context.Context, batchID string) (int64, error) {
	f.log.add("purge:config:" + batchID)
	return 1, nil
}

func (f *fakeLevelConfig) ListLevelDeletions(context.Context) ([]client.LevelDeletion, error) {
	return []client.LevelDeletion{f.deletion}, nil
}

func (f *fakeLevelConfig) GetLevelDeletion(context.Context, string) (client.LevelDeletion, error) {
	return f.deletion, f.getErr
}

func (f *fakeLevelConfig) ListLevels(context.Context) ([]client.LevelResult, error) {
	return f.levels, nil
}

// ctxMark tags a call made on a dead context, so a test can prove the
// compensation never runs on the expired saga context.
func ctxMark(ctx context.Context) string {
	if ctx.Err() != nil {
		return "(dead ctx)"
	}
	return ""
}

// fakeLevelPeople is students' side.
type fakeLevelPeople struct {
	log        *sagaLog
	preview    client.StudentsLevelPreview
	archiveErr error
	restoreErr error
	skipped    []client.LevelRestoreSkip
	unrestored []string
	scope      client.LevelScope
	previewErr error
}

func (f *fakeLevelPeople) PreviewLevelDeletion(_ context.Context, s client.LevelScope) (client.StudentsLevelPreview, error) {
	f.scope = s
	return f.preview, f.previewErr
}

func (f *fakeLevelPeople) ArchiveLevel(context.Context, string, client.LevelScope) (client.LevelCounts, error) {
	f.log.add("archive:students")
	return f.preview.Counts, f.archiveErr
}

func (f *fakeLevelPeople) RestoreLevelBatch(ctx context.Context, _ string) (client.LevelRestoreReport, error) {
	f.log.add("restore:students" + ctxMark(ctx))
	return client.LevelRestoreReport{Restored: client.LevelCounts{"students": 1}, Skipped: f.skipped, Unrestored: f.unrestored}, f.restoreErr
}

func (f *fakeLevelPeople) PurgeLevelBatch(_ context.Context, batchID string) (int64, error) {
	f.log.add("purge:students:" + batchID)
	return 1, nil
}

// fakeLevelData is one of grades/attendance/finance/uniforms.
type fakeLevelData struct {
	name       string
	log        *sagaLog
	counts     client.LevelCounts
	archiveErr error
	blockUntil bool // ArchiveLevel waits for its context to end (a hung service)
	restoreErr error
	skipped    []client.LevelRestoreSkip
	skipIDs    []string
	purgeErr   error
	scope      client.LevelScope
	onPreview  func() // observes when the preview runs (e.g. under the freeze)
}

func (f *fakeLevelData) PreviewLevelDeletion(_ context.Context, s client.LevelScope) (client.LevelCounts, error) {
	f.scope = s
	if f.onPreview != nil {
		f.onPreview()
	}
	return f.counts, nil
}

func (f *fakeLevelData) ArchiveLevel(ctx context.Context, _ string, s client.LevelScope) (client.LevelCounts, error) {
	f.log.add("archive:" + f.name)
	f.scope = s
	if f.blockUntil {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.counts, f.archiveErr
}

func (f *fakeLevelData) RestoreLevelBatch(ctx context.Context, _ string, skip []string) (client.LevelRestoreReport, error) {
	f.log.add("restore:" + f.name + ctxMark(ctx))
	f.skipIDs = skip
	return client.LevelRestoreReport{Restored: client.LevelCounts{}, Skipped: f.skipped}, f.restoreErr
}

func (f *fakeLevelData) PurgeLevelBatch(_ context.Context, batchID string) (int64, error) {
	f.log.add("purge:" + f.name + ":" + batchID)
	return 1, f.purgeErr
}

type fakePasswords struct {
	err   error
	token string
	calls int
}

func (f *fakePasswords) VerifyPassword(_ context.Context, token, _ string) error {
	f.calls++
	f.token = token
	return f.err
}

type deleterNamer struct{}

func (deleterNamer) GetUser(context.Context, string) (client.User, error) {
	return client.User{DisplayName: "Laura Díaz"}, nil
}

type levelDeletionRig struct {
	log         *sagaLog
	cfg         *fakeLevelConfig
	people      *fakeLevelPeople
	grades      *fakeLevelData
	attendance  *fakeLevelData
	finance     *fakeLevelData
	uniforms    *fakeLevelData
	passwords   *fakePasswords
	now         time.Time
	sagaTimeout time.Duration
	h           *handler.LevelDeletionHandler
}

// rebuild re-wires the handler after a test changed the rig's knobs.
func (rig *levelDeletionRig) rebuild() {
	rig.h = handler.NewLevelDeletionHandler(handler.LevelDeletionDeps{
		Config: rig.cfg, Students: rig.people,
		Grades: rig.grades, Attendance: rig.attendance, Finance: rig.finance, Uniforms: rig.uniforms,
		Passwords: rig.passwords, Users: deleterNamer{},
		Tokens:      handler.NewDeletionTokens([]byte("0123456789abcdef0123456789abcdef"), func() time.Time { return rig.now }),
		Now:         func() time.Time { return rig.now },
		NewID:       func() string { return delBatch },
		Log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		SagaTimeout: rig.sagaTimeout,
	})
}

func newLevelDeletionRig(withData bool) *levelDeletionRig {
	log := &sagaLog{}
	rig := &levelDeletionRig{log: log, now: delNow}
	structure := client.LevelStructure{
		LevelID: delLevel, LevelName: "Primaria",
		Counts: client.LevelCounts{"levels": 1, "terms": 1, "gradeLevels": 0, "groups": 0},
	}
	people := client.StudentsLevelPreview{Counts: client.LevelCounts{"students": 0, "enrollments": 0}}
	if withData {
		structure.GradeLevelIDs = []string{"gl-1"}
		structure.GroupIDs = []string{"gr-1"}
		structure.SubjectIDs = []string{"su-1"}
		structure.TermIDs = []string{"te-1"}
		structure.Counts = client.LevelCounts{"levels": 1, "gradeLevels": 1, "groups": 1, "subjects": 1, "terms": 1, "termMonths": 3, "teachingAssignments": 1, "schoolEvents": 2}
		people = client.StudentsLevelPreview{
			Counts:        client.LevelCounts{"students": 2, "enrollments": 2, "enrollmentTransfers": 0},
			StudentIDs:    []string{"st-1", "st-2"},
			EnrollmentIDs: []string{"en-1", "en-2"},
		}
	}
	rig.cfg = &fakeLevelConfig{log: log, structure: structure, deletion: client.LevelDeletion{
		BatchID: delBatch, LevelID: delLevel, LevelName: "Primaria", Status: "deleted", DeletedByName: "Laura Díaz",
		DeletedAt: delNow, RestorableUntil: delNow.Add(7 * 24 * time.Hour), Counts: client.LevelCounts{"groups": 1},
	}}
	rig.people = &fakeLevelPeople{log: log, preview: people}
	rig.grades = &fakeLevelData{name: "grades", log: log, counts: client.LevelCounts{"monthlyGrades": 40, "reportCards": 4}}
	rig.attendance = &fakeLevelData{name: "attendance", log: log, counts: client.LevelCounts{"attendanceMarks": 100}}
	rig.finance = &fakeLevelData{name: "finance", log: log, counts: client.LevelCounts{"tuitionPlans": 1, "charges": 20, "payments": 12}}
	rig.uniforms = &fakeLevelData{name: "uniforms", log: log, counts: client.LevelCounts{"uniformItems": 3}}
	rig.passwords = &fakePasswords{}
	rig.rebuild()
	return rig
}

func ownerRequest(method, target, body string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	r.Header.Set("Authorization", "Bearer owner-token")
	return r.WithContext(middleware.WithPrincipal(r.Context(), middleware.Principal{
		UserID: "cccccccc-0000-0000-0000-0000000000c1", TenantID: "t1", Email: "director@x.mx",
		Permissions: []string{"levels:delete"},
	}))
}

type previewBody struct {
	LevelID   string           `json:"levelId"`
	LevelName string           `json:"levelName"`
	Empty     bool             `json:"empty"`
	Counts    map[string]int64 `json:"counts"`
	Keeps     map[string]int64 `json:"keeps"`
	Token     string           `json:"confirmToken"`
	ExpiresAt time.Time        `json:"expiresAt"`
}

func (rig *levelDeletionRig) preview(t *testing.T) previewBody {
	t.Helper()
	r := ownerRequest(http.MethodGet, "/api/v1/config/levels/"+delLevel+"/deletion-preview", "")
	r.SetPathValue("id", delLevel)
	w := httptest.NewRecorder()
	rig.h.Preview(w, r)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body previewBody
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func (rig *levelDeletionRig) delete(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := ownerRequest(http.MethodPost, "/api/v1/config/levels/"+delLevel+"/delete", body)
	r.SetPathValue("id", delLevel)
	w := httptest.NewRecorder()
	rig.h.Delete(w, r)
	return w
}

func deleteBody(token, name, password string) string {
	b, _ := json.Marshal(map[string]string{"confirmToken": token, "confirmName": name, "password": password})
	return string(b)
}

func TestLevelDeletionPreviewOfAnEmptyLevel(t *testing.T) {
	rig := newLevelDeletionRig(false)
	body := rig.preview(t)

	assert.True(t, body.Empty, "no grades, no pupils, no enrollments")
	assert.Equal(t, "Primaria", body.LevelName)
	assert.Equal(t, int64(1), body.Counts["terms"])
	assert.NotEmpty(t, body.Token)
	assert.Equal(t, delNow.Add(10*time.Minute), body.ExpiresAt.UTC())
	assert.Empty(t, rig.log.calls, "a preview archives nothing")
}

func TestLevelDeletionPreviewWithDataResolvesTheScopeAcrossServices(t *testing.T) {
	rig := newLevelDeletionRig(true)
	body := rig.preview(t)

	assert.False(t, body.Empty)
	for kind, want := range map[string]int64{
		"gradeLevels": 1, "groups": 1, "subjects": 1, "terms": 1, "teachingAssignments": 1, "students": 2,
		"enrollments": 2, "attendanceMarks": 100, "monthlyGrades": 40, "reportCards": 4, "tuitionPlans": 1,
		"uniformItems": 3, "schoolEvents": 2,
	} {
		assert.Equal(t, want, body.Counts[kind], kind)
	}
	assert.NotContains(t, body.Counts, "charges", "money is kept, never counted as deleted")
	assert.Equal(t, map[string]int64{"charges": 20, "payments": 12}, body.Keeps)
	assert.Equal(t, []string{"gr-1"}, rig.people.scope.GroupIDs, "students gets config's groups")
	assert.Equal(t, []string{"en-1", "en-2"}, rig.grades.scope.EnrollmentIDs, "grades gets students' enrollments")
	assert.Equal(t, []string{"te-1"}, rig.grades.scope.TermIDs)
	assert.Equal(t, []string{"st-1", "st-2"}, rig.finance.scope.StudentIDs, "finance counts the kept money of these pupils")
}

func TestLevelDeletionPreviewOfAnUnknownLevelIs404(t *testing.T) {
	rig := newLevelDeletionRig(true)
	rig.cfg.previewErr = connect.NewError(connect.CodeNotFound, errors.New("not found"))
	r := ownerRequest(http.MethodGet, "/x", "")
	r.SetPathValue("id", delLevel)
	w := httptest.NewRecorder()
	rig.h.Preview(w, r)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestLevelDeletionTripleValidation(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(rig *levelDeletionRig)
		token      func(valid string) string
		confirm    string
		passwordOK bool
		wantStatus int
		wantCode   string
		// froze: the refusal came from the counts, compared under the freeze
		// (taken, then let go); every other refusal takes no lock at all.
		froze bool
	}{
		{"stale: the data changed since the preview", func(rig *levelDeletionRig) { rig.grades.counts["monthlyGrades"]++ },
			nil, "Primaria", true, http.StatusConflict, "PREVIEW_STALE", true},
		{"stale: the preview expired", func(rig *levelDeletionRig) { rig.now = rig.now.Add(11 * time.Minute) },
			nil, "Primaria", true, http.StatusConflict, "PREVIEW_STALE", false},
		{"stale: no token", nil, func(string) string { return "" }, "Primaria", true, http.StatusConflict, "PREVIEW_STALE", false},
		{"the typed name does not match", nil, nil, "Secundaria", true, http.StatusBadRequest, "NAME_MISMATCH", false},
		{"wrong password", nil, nil, "Primaria", false, http.StatusUnauthorized, "REAUTH_FAILED", false},
		{"name is trimmed and case-insensitive", nil, nil, "  pRIMARIA ", true, http.StatusOK, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newLevelDeletionRig(true)
			token := rig.preview(t).Token
			if tc.mutate != nil {
				tc.mutate(rig)
			}
			if tc.token != nil {
				token = tc.token(token)
			}
			if !tc.passwordOK {
				rig.passwords.err = connect.NewError(connect.CodeUnauthenticated, errors.New("invalid credentials"))
			}
			w := rig.delete(t, deleteBody(token, tc.confirm, "pw"))
			assert.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			assert.Equal(t, tc.wantCode, w.Header().Get("X-Error-Code"))
			switch {
			case tc.froze:
				assert.Equal(t, []string{"begin:config", "cancel:config"}, rig.log.calls,
					"the counts are compared under the freeze; a stale preview lets it go and archives nothing")
			case tc.wantStatus != http.StatusOK:
				assert.Empty(t, rig.log.calls, "a refused deletion takes no lock and archives nothing")
			}
		})
	}
}

func TestLevelDeletionPasswordIsCheckedWithTheCallersOwnToken(t *testing.T) {
	rig := newLevelDeletionRig(true)
	w := rig.delete(t, deleteBody(rig.preview(t).Token, "Primaria", "pw"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "owner-token", rig.passwords.token, "iam derives the subject from the verified token")
}

func TestLevelDeletionRunsTheSagaInOrder(t *testing.T) {
	rig := newLevelDeletionRig(true)
	token := rig.preview(t).Token
	w := rig.delete(t, deleteBody(token, "Primaria", "pw"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, handler.TokenHash(token), rig.cfg.begun.TokenHash, "config marks the token used")

	assert.Equal(t, []string{
		"begin:config",
		"archive:grades", "archive:attendance", "archive:finance", "archive:uniforms", "archive:students", "archive:config",
	}, rig.log.calls, "the lock first; then dependents first, the structure (and the level) last")
	assert.Equal(t, int64(40), rig.cfg.summary["monthlyGrades"], "config records what the others archived")
	assert.Equal(t, int64(2), rig.cfg.summary["students"])
	assert.NotContains(t, rig.cfg.summary, "charges", "kept money is not a deletion count")
	assert.Equal(t, "Laura Díaz", rig.cfg.begun.DeletedByName)
	assert.Equal(t, "cccccccc-0000-0000-0000-0000000000c1", rig.cfg.begun.DeletedBy)
	assert.Equal(t, delLevel, rig.cfg.begun.LevelID)

	var body struct {
		BatchID         string           `json:"batchId"`
		DeletedAt       time.Time        `json:"deletedAt"`
		RestorableUntil time.Time        `json:"restorableUntil"`
		Counts          map[string]int64 `json:"counts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, delBatch, body.BatchID)
	assert.Equal(t, delNow.Add(7*24*time.Hour), body.RestorableUntil.UTC())
	assert.Equal(t, int64(100), body.Counts["attendanceMarks"])
}

func TestLevelDeletionCompensatesAMidSagaFailure(t *testing.T) {
	cases := []struct {
		name  string
		fail  func(rig *levelDeletionRig)
		wants []string
	}{
		{"students fails", func(rig *levelDeletionRig) { rig.people.archiveErr = errors.New("boom") }, []string{
			"begin:config",
			"archive:grades", "archive:attendance", "archive:finance", "archive:uniforms", "archive:students",
			"restore:students", "restore:uniforms", "restore:finance", "restore:attendance", "restore:grades",
			"cancel:config",
		}},
		{"config fails last", func(rig *levelDeletionRig) {
			rig.cfg.archiveErr = errors.New("boom")
			rig.cfg.restoreErr = connect.NewError(connect.CodeNotFound, errors.New("no such batch"))
		}, []string{
			"begin:config",
			"archive:grades", "archive:attendance", "archive:finance", "archive:uniforms", "archive:students", "archive:config",
			"restore:config", "restore:students", "restore:uniforms", "restore:finance", "restore:attendance", "restore:grades",
			"cancel:config",
		}},
		{"the first step fails", func(rig *levelDeletionRig) { rig.grades.archiveErr = errors.New("boom") }, []string{
			"begin:config", "archive:grades", "restore:grades", "cancel:config",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newLevelDeletionRig(true)
			token := rig.preview(t).Token
			tc.fail(rig)
			w := rig.delete(t, deleteBody(token, "Primaria", "pw"))
			assert.Equal(t, http.StatusInternalServerError, w.Code)
			assert.Contains(t, w.Body.String(), "no se borró nada")
			assert.Equal(t, tc.wants, rig.log.calls, "every step that ran (or may have) is put back, newest first, then the lock is released")
			assert.False(t, rig.cfg.failed)
		})
	}
}

// Review finding 1: when the compensation fails too, the batch is recorded as
// failed_partial (the trash lists it, Restaurar recovers it), the answer names
// the folio, and it never claims nothing was deleted.
func TestLevelDeletionRecordsAFailedCompensation(t *testing.T) {
	rig := newLevelDeletionRig(true)
	token := rig.preview(t).Token
	rig.people.archiveErr = errors.New("boom")
	rig.finance.restoreErr = errors.New("still down")
	rig.grades.skipped = []client.LevelRestoreSkip{{Kind: "monthlyGrades", Count: 2, Reason: "ya existe otra captura"}}
	w := rig.delete(t, deleteBody(token, "Primaria", "pw"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "DELETION_INCOMPLETE", w.Header().Get("X-Error-Code"))
	assert.Contains(t, w.Body.String(), delBatch, "the folio")
	assert.NotContains(t, w.Body.String(), "no se borró nada")
	assert.True(t, rig.cfg.failed)
	assert.False(t, rig.cfg.cancelled)
	assert.ElementsMatch(t, []string{"finance", "grades"}, rig.cfg.failPending,
		"the step that could not restore, and the one whose skipped rows stay archived")
	assert.Equal(t, rig.grades.skipped, rig.cfg.failSkipped, "the skipped rows are recorded with the batch")
}

// Review finding 1: a saga that ran out of time is compensated on a fresh,
// detached context — never the expired one.
func TestLevelDeletionCompensatesOnADetachedContext(t *testing.T) {
	rig := newLevelDeletionRig(true)
	token := rig.preview(t).Token
	rig.uniforms.blockUntil = true
	rig.sagaTimeout = 20 * time.Millisecond
	rig.rebuild()
	w := rig.delete(t, deleteBody(token, "Primaria", "pw"))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "no se borró nada")
	for _, c := range rig.log.calls {
		assert.NotContains(t, c, "dead ctx", "compensation must not run on the expired saga context")
	}
	assert.Contains(t, rig.log.calls, "restore:uniforms", "the step that timed out is undone too")
	assert.True(t, rig.cfg.cancelled)
}

// Review finding 5: a double click. The second request finds the level
// being deleted and never starts a second saga.
func TestLevelDeletionRefusesASecondDeletionWhileOneRuns(t *testing.T) {
	t.Run("seen in the preview", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		token := rig.preview(t).Token
		rig.cfg.structure.Deleting = true
		w := rig.delete(t, deleteBody(token, "Primaria", "pw"))
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, "DELETION_IN_PROGRESS", w.Header().Get("X-Error-Code"))
		assert.Contains(t, w.Body.String(), "Papelera")
		assert.Empty(t, rig.log.calls, "no lock taken, nothing archived")
		assert.Zero(t, rig.passwords.calls, "not even the password is asked")
	})
	t.Run("lost the race for the lock", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		token := rig.preview(t).Token
		rig.cfg.beginErr = codedConnectErr(connect.CodeAlreadyExists, "DELETION_IN_PROGRESS", "Ya se está eliminando este nivel")
		w := rig.delete(t, deleteBody(token, "Primaria", "pw"))
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, "DELETION_IN_PROGRESS", w.Header().Get("X-Error-Code"))
		assert.Equal(t, []string{"begin:config"}, rig.log.calls, "no step ran")
	})
	t.Run("the token was already used", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		token := rig.preview(t).Token
		rig.cfg.beginErr = codedConnectErr(connect.CodeAlreadyExists, "PREVIEW_STALE", "Los datos cambiaron")
		w := rig.delete(t, deleteBody(token, "Primaria", "pw"))
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, "PREVIEW_STALE", w.Header().Get("X-Error-Code"))
		assert.Equal(t, []string{"begin:config"}, rig.log.calls)
	})
}

func codedConnectErr(code connect.Code, errorCode, msg string) error {
	ce := connect.NewError(code, errors.New(msg))
	ce.Meta().Set("X-Error-Code", errorCode)
	return ce
}

func (rig *levelDeletionRig) restore(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	r := ownerRequest(http.MethodPost, "/api/v1/config/level-trash/"+delBatch+"/restore", "")
	r.SetPathValue("batchId", delBatch)
	w := httptest.NewRecorder()
	rig.h.Restore(w, r)
	return w
}

func TestLevelRestore(t *testing.T) {
	t.Run("restores config first, then the rest, and closes the batch", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.people.skipped = []client.LevelRestoreSkip{{Kind: "enrollments", Count: 1, Reason: "el alumno ya está inscrito en otro grupo de ese ciclo"}}
		w := rig.restore(t)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, []string{
			"restore-start:config",
			"restore:config", "restore:students", "restore:uniforms", "restore:finance", "restore:attendance", "restore:grades", "mark:config",
		}, rig.log.calls, "the batch is marked restoring before the first step")
		assert.True(t, rig.cfg.markedPart, "a skip keeps the batch in the trash as restored_partial")
		assert.Positive(t, rig.cfg.sweeps, "the sweep runs lazily first")
		var body struct {
			Restored map[string]int64 `json:"restored"`
			Skipped  []struct {
				Kind   string `json:"kind"`
				Count  int64  `json:"count"`
				Reason string `json:"reason"`
			} `json:"skipped"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, int64(2), body.Restored["groups"])
		assert.Equal(t, int64(1), body.Restored["students"])
		require.Len(t, body.Skipped, 1)
		assert.Equal(t, "enrollments", body.Skipped[0].Kind)
	})

	cases := []struct {
		name       string
		mutate     func(rig *levelDeletionRig)
		wantStatus int
		wantCode   string
	}{
		{"already purged", func(rig *levelDeletionRig) { rig.cfg.deletion.Status = "purged" }, http.StatusGone, "TRASH_EXPIRED"},
		{"already restored", func(rig *levelDeletionRig) { rig.cfg.deletion.Status = "restored" }, http.StatusNotFound, ""},
		{"unknown batch", func(rig *levelDeletionRig) {
			rig.cfg.getErr = connect.NewError(connect.CodeNotFound, errors.New("nope"))
		}, http.StatusNotFound, ""},
		{"a level with the same name now exists", func(rig *levelDeletionRig) {
			rig.cfg.levels = []client.LevelResult{{ID: "other", Name: " primaria "}}
		}, http.StatusConflict, "LEVEL_NAME_TAKEN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newLevelDeletionRig(true)
			tc.mutate(rig)
			w := rig.restore(t)
			assert.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			assert.Equal(t, tc.wantCode, w.Header().Get("X-Error-Code"))
			assert.Empty(t, rig.log.calls, "nothing is touched")
		})
	}

	t.Run("past seven days on the database's clock", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.cfg.restoreErr = codedConnectErr(connect.CodePermissionDenied, "TRASH_EXPIRED", "Pasaron más de 7 días")
		w := rig.restore(t)
		assert.Equal(t, http.StatusGone, w.Code)
		assert.Equal(t, "TRASH_EXPIRED", w.Header().Get("X-Error-Code"))
		assert.Equal(t, []string{"restore-start:config", "restore:config", "restore-abandon:config"}, rig.log.calls)
		assert.False(t, rig.cfg.marked)
	})

	t.Run("its own level, back from a partial restore, is not a name clash", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.cfg.levels = []client.LevelResult{{ID: delLevel, Name: "Primaria"}}
		w := rig.restore(t)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.True(t, rig.cfg.marked)
		assert.False(t, rig.cfg.markedPart, "nothing skipped: fully restored")
	})

	t.Run("enrollments students could not restore keep their grades and marks archived", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.people.unrestored = []string{"en-2"}
		w := rig.restore(t)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, []string{"en-2"}, rig.grades.skipIDs)
		assert.Equal(t, []string{"en-2"}, rig.attendance.skipIDs)
	})

	t.Run("a missing archive is never a silent success", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.finance.restoreErr = connect.NewError(connect.CodeNotFound, errors.New("no archive"))
		w := rig.restore(t)
		assert.Equal(t, http.StatusInternalServerError, w.Code, "config and students came back; finance's part is gone")
		assert.Equal(t, "RESTORE_INCOMPLETE", w.Header().Get("X-Error-Code"))
		assert.Contains(t, w.Body.String(), "finance")
		assert.False(t, rig.cfg.marked)
		assert.True(t, rig.cfg.abandoned, "the batch falls back from restoring")
	})

	t.Run("a failed_partial batch restores only its pending steps; a step that never archived is fine", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.cfg.deletion.Status = "failed_partial"
		rig.cfg.deletion.PendingSteps = []string{"students", "grades", "attendance"}
		rig.grades.restoreErr = connect.NewError(connect.CodeNotFound, errors.New("never archived"))
		w := rig.restore(t)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, []string{"restore-start:config", "restore:students", "restore:attendance", "restore:grades", "mark:config"}, rig.log.calls)
	})

	t.Run("nothing found at all is an expired batch", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.cfg.restoreErr = connect.NewError(connect.CodeNotFound, errors.New("no archive"))
		w := rig.restore(t)
		assert.Equal(t, http.StatusGone, w.Code)
		assert.Equal(t, "TRASH_EXPIRED", w.Header().Get("X-Error-Code"))
		assert.False(t, rig.cfg.marked)
	})

	t.Run("a service failing midway leaves the batch in the trash to retry", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.finance.restoreErr = errors.New("down")
		w := rig.restore(t)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.False(t, rig.cfg.marked, "not closed: a retry finishes it")
		assert.True(t, rig.cfg.abandoned, "back where it was, so the next restore may start")
	})
}

func TestLevelTrashLists(t *testing.T) {
	rig := newLevelDeletionRig(true)
	w := httptest.NewRecorder()
	rig.h.Trash(w, ownerRequest(http.MethodGet, "/api/v1/config/level-trash", ""))
	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Items []struct {
			BatchID       string           `json:"batchId"`
			LevelName     string           `json:"levelName"`
			DeletedByName string           `json:"deletedByName"`
			Status        string           `json:"status"`
			Counts        map[string]int64 `json:"counts"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Items, 1)
	assert.Positive(t, rig.cfg.sweeps, "the sweep runs lazily first")
	assert.Equal(t, delBatch, body.Items[0].BatchID)
	assert.Equal(t, "Laura Díaz", body.Items[0].DeletedByName)
	assert.Equal(t, "deleted", body.Items[0].Status)
	assert.Equal(t, int64(1), body.Items[0].Counts["groups"])
}

// The level deletion answers every refusal in text/plain with X-Error-Code, as
// the contract says; the contract must not promise the JSON envelope.
func TestLevelDeletionContractDocumentsPlainTextErrors(t *testing.T) {
	doc := loadOpenAPI(t)
	operations := []struct{ path, method string }{
		{"/api/v1/config/levels/{id}/deletion-preview", "get"},
		{"/api/v1/config/levels/{id}/delete", "post"},
		{"/api/v1/config/level-trash", "get"},
		{"/api/v1/config/level-trash/{batchId}/restore", "post"},
	}
	for _, op := range operations {
		node, ok := doc.Paths[op.path][op.method]
		require.True(t, ok, "%s %s is documented", op.method, op.path)
		var operation openAPIOperation
		require.NoError(t, node.Decode(&operation))
		for status, response := range operation.Responses {
			if strings.HasPrefix(status, "2") {
				continue
			}
			content := doc.resolve(t, response).Content
			assert.Contains(t, content, "text/plain", "%s %s %s", op.method, op.path, status)
			assert.NotContains(t, content, "application/json", "%s %s %s", op.method, op.path, status)
		}
	}
	for _, code := range []string{"PREVIEW_STALE", "NAME_MISMATCH", "REAUTH_FAILED"} {
		raw, err := os.ReadFile(filepath.Clean(openAPIPath))
		require.NoError(t, err)
		assert.Contains(t, string(raw), code)
	}
}

func TestLevelTrashListsIncompleteDeletions(t *testing.T) {
	for _, status := range []string{"failed_partial", "restored_partial", "restoring"} {
		t.Run(status, func(t *testing.T) {
			rig := newLevelDeletionRig(true)
			rig.cfg.deletion.Status = status
			w := httptest.NewRecorder()
			rig.h.Trash(w, ownerRequest(http.MethodGet, "/api/v1/config/level-trash", ""))
			require.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Body.String(), `"status":"`+status+`"`)
		})
	}
}

// Review finding 8: the gateway (always warm) sweeps on config's clock:
// every service purges each expired batch, then config closes it. A batch a
// service could not purge stays open for the next run.
func TestLevelTrashSweep(t *testing.T) {
	rig := newLevelDeletionRig(true)
	rig.cfg.expired = []client.LevelDeletion{{BatchID: "old-1"}, {BatchID: "old-2"}}
	rig.uniforms.purgeErr = nil
	n, err := rig.h.Sweep(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	assert.Equal(t, []string{"old-1", "old-2"}, rig.cfg.purgedMarked)
	for _, svc := range []string{"grades", "attendance", "finance", "uniforms", "students", "config"} {
		assert.Contains(t, rig.log.calls, "purge:"+svc+":old-1")
	}

	rig = newLevelDeletionRig(true)
	rig.cfg.expired = []client.LevelDeletion{{BatchID: "old-1"}}
	rig.finance.purgeErr = errors.New("down")
	n, err = rig.h.Sweep(context.Background())
	assert.Error(t, err)
	assert.Zero(t, n)
	assert.Empty(t, rig.cfg.purgedMarked, "not closed: the next run retries it")
}

func TestLevelDeletionPreviewOfALevelBeingDeletedIs409(t *testing.T) {
	rig := newLevelDeletionRig(true)
	rig.cfg.structure.Deleting = true
	r := ownerRequest(http.MethodGet, "/x", "")
	r.SetPathValue("id", delLevel)
	w := httptest.NewRecorder()
	rig.h.Preview(w, r)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, "DELETION_IN_PROGRESS", w.Header().Get("X-Error-Code"))
}

// Confirmation review (gap 4): a restore marks its batch restoring in config
// first (compare-and-set), so the sweep cannot purge it mid-way; the steps it
// runs follow the status the restore started from; a failure puts the batch
// back where it was.
func TestLevelRestoreRunsUnderTheRestoringMark(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(rig *levelDeletionRig)
		wantStatus int
		wantCode   string
		wantLog    []string
	}{
		{"another restore runs", func(rig *levelDeletionRig) {
			rig.cfg.restoreStartErr = codedConnectErr(connect.CodeAlreadyExists, "RESTORE_IN_PROGRESS", "Ya se está restaurando")
		}, http.StatusConflict, "RESTORE_IN_PROGRESS", []string{"restore-start:config"}},
		{"the window closed as it started", func(rig *levelDeletionRig) {
			rig.cfg.restoreStartErr = codedConnectErr(connect.CodePermissionDenied, "TRASH_EXPIRED", "Pasaron más de 7 días")
		}, http.StatusGone, "TRASH_EXPIRED", []string{"restore-start:config"}},
		{"the trash already shows it restoring", func(rig *levelDeletionRig) {
			rig.cfg.deletion.Status = "restoring"
		}, http.StatusConflict, "RESTORE_IN_PROGRESS", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newLevelDeletionRig(true)
			tc.mutate(rig)
			w := rig.restore(t)
			assert.Equal(t, tc.wantStatus, w.Code, w.Body.String())
			assert.Equal(t, tc.wantCode, w.Header().Get("X-Error-Code"))
			assert.Equal(t, tc.wantLog, rig.log.calls, "no step runs")
			assert.False(t, rig.cfg.abandoned, "a restore that never started has nothing to put back")
		})
	}

	t.Run("the steps follow the status the restore started from", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.cfg.deletion.PendingSteps = []string{"grades"}
		rig.cfg.restoringFrom = "failed_partial" // it failed half-way between the read and the start
		w := rig.restore(t)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, []string{"restore-start:config", "restore:grades", "mark:config"}, rig.log.calls)
	})

	t.Run("closing the batch fails: it falls back and the error is answered", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.cfg.markErr = errors.New("config down")
		w := rig.restore(t)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.True(t, rig.cfg.abandoned)
	})
}

// Confirmation review (gap 4): the sweep never purges a batch config answers as
// running, even if a config that predates the restoring mark listed it.
func TestLevelTrashSweepSkipsRunningBatches(t *testing.T) {
	rig := newLevelDeletionRig(true)
	rig.cfg.expired = []client.LevelDeletion{
		{BatchID: "restoring-1", Status: "restoring"}, {BatchID: "deleting-1", Status: "deleting"}, {BatchID: "old-1", Status: "deleted"},
	}
	n, err := rig.h.Sweep(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	assert.Equal(t, []string{"old-1"}, rig.cfg.purgedMarked)
	for _, c := range rig.log.calls {
		assert.NotContains(t, c, "restoring-1")
		assert.NotContains(t, c, "deleting-1")
	}
}

// Confirmation review (gap 5): the freeze comes FIRST — the scope is resolved
// and its counts compared with the token's while nothing of the level can be
// written, so a grade captured between the check and the lock cannot slip in.
func TestLevelDeletionResolvesTheScopeUnderTheFreeze(t *testing.T) {
	t.Run("the scope is resolved under the freeze", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		token := rig.preview(t).Token
		var frozenAtPreview []bool
		rig.grades.onPreview = func() { frozenAtPreview = append(frozenAtPreview, rig.cfg.begun.BatchID != "") }
		w := rig.delete(t, deleteBody(token, "Primaria", "pw"))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NotEmpty(t, frozenAtPreview)
		for _, frozen := range frozenAtPreview {
			assert.True(t, frozen, "every count the token is checked against was read under the freeze")
		}
	})

	t.Run("a count that moved before the freeze lets it go: PREVIEW_STALE", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		token := rig.preview(t).Token
		rig.grades.onPreview = func() {
			if rig.cfg.begun.BatchID != "" {
				rig.grades.counts["monthlyGrades"] = 41 // captured just before the lock
			}
		}
		w := rig.delete(t, deleteBody(token, "Primaria", "pw"))
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, "PREVIEW_STALE", w.Header().Get("X-Error-Code"))
		assert.Equal(t, []string{"begin:config", "cancel:config"}, rig.log.calls, "released, nothing archived")
	})

	t.Run("resolving under the freeze fails: it is let go", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		token := rig.preview(t).Token
		rig.people.previewErr = errors.New("students down")
		w := rig.delete(t, deleteBody(token, "Primaria", "pw"))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, []string{"begin:config", "cancel:config"}, rig.log.calls)
	})
}

// QA (deferred): the preview counts every kind a director would recognise,
// not only the headline ones.
func TestLevelDeletionPreviewCountsEveryKindADirectorReads(t *testing.T) {
	rig := newLevelDeletionRig(true)
	rig.grades.counts = client.LevelCounts{"monthlyGrades": 40, "reportCards": 4, "gradeEntries": 7, "periodClosures": 2,
		"reportCardLines": 90, "gradesOutbox": 1}
	rig.attendance.counts = client.LevelCounts{"attendanceMarks": 100, "attendanceSessions": 12}
	rig.people.preview.Counts["enrollmentTransfers"] = 1
	body := rig.preview(t)
	for kind, want := range map[string]int64{"gradeEntries": 7, "periodClosures": 2, "attendanceSessions": 12, "enrollmentTransfers": 1} {
		assert.Equal(t, want, body.Counts[kind], kind)
	}
	for _, internal := range []string{"reportCardLines", "gradesOutbox", "termMonths", "levels"} {
		assert.NotContains(t, body.Counts, internal, "%s means nothing to a director", internal)
	}
}
