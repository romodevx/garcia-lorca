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
	archiveErr   error
	restoreErr   error
	summary      client.LevelCounts
	archivedBy   string
	archivedName string
	deletion     client.LevelDeletion
	getErr       error
	levels       []client.LevelResult
	marked       bool
}

func (f *fakeLevelConfig) PreviewLevelDeletion(context.Context, string) (client.LevelStructure, error) {
	return f.structure, f.previewErr
}

func (f *fakeLevelConfig) ArchiveLevel(_ context.Context, in client.ArchiveLevelRecord) (client.LevelCounts, client.LevelDeletion, error) {
	f.log.add("archive:config")
	if f.archiveErr != nil {
		return nil, client.LevelDeletion{}, f.archiveErr
	}
	f.summary, f.archivedBy, f.archivedName = in.Summary, in.DeletedBy, in.DeletedByName
	total := client.LevelCounts{}
	total.Add(in.Summary)
	total.Add(f.structure.Counts)
	return f.structure.Counts, client.LevelDeletion{
		BatchID: in.BatchID, LevelID: in.LevelID, LevelName: f.structure.LevelName, Counts: total,
		DeletedAt: delNow, RestorableUntil: delNow.Add(7 * 24 * time.Hour), Status: "deleted",
	}, nil
}

func (f *fakeLevelConfig) RestoreLevelBatch(context.Context, string) (client.LevelRestoreReport, error) {
	f.log.add("restore:config")
	if f.restoreErr != nil {
		return client.LevelRestoreReport{}, f.restoreErr
	}
	return client.LevelRestoreReport{Restored: client.LevelCounts{"levels": 1, "groups": 2}}, nil
}

func (f *fakeLevelConfig) MarkLevelDeletionRestored(context.Context, string) (client.LevelDeletion, error) {
	f.log.add("mark:config")
	f.marked = true
	return client.LevelDeletion{Status: "restored"}, nil
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

// fakeLevelPeople is students' side.
type fakeLevelPeople struct {
	log        *sagaLog
	preview    client.StudentsLevelPreview
	archiveErr error
	restoreErr error
	skipped    []client.LevelRestoreSkip
	scope      client.LevelScope
}

func (f *fakeLevelPeople) PreviewLevelDeletion(_ context.Context, s client.LevelScope) (client.StudentsLevelPreview, error) {
	f.scope = s
	return f.preview, nil
}

func (f *fakeLevelPeople) ArchiveLevel(context.Context, string, client.LevelScope) (client.LevelCounts, error) {
	f.log.add("archive:students")
	return f.preview.Counts, f.archiveErr
}

func (f *fakeLevelPeople) RestoreLevelBatch(context.Context, string) (client.LevelRestoreReport, error) {
	f.log.add("restore:students")
	return client.LevelRestoreReport{Restored: client.LevelCounts{"students": 1}, Skipped: f.skipped}, f.restoreErr
}

// fakeLevelData is one of grades/attendance/finance/uniforms.
type fakeLevelData struct {
	name       string
	log        *sagaLog
	counts     client.LevelCounts
	archiveErr error
	restoreErr error
	scope      client.LevelScope
}

func (f *fakeLevelData) PreviewLevelDeletion(_ context.Context, s client.LevelScope) (client.LevelCounts, error) {
	f.scope = s
	return f.counts, nil
}

func (f *fakeLevelData) ArchiveLevel(_ context.Context, _ string, s client.LevelScope) (client.LevelCounts, error) {
	f.log.add("archive:" + f.name)
	f.scope = s
	return f.counts, f.archiveErr
}

func (f *fakeLevelData) RestoreLevelBatch(context.Context, string) (client.LevelRestoreReport, error) {
	f.log.add("restore:" + f.name)
	return client.LevelRestoreReport{Restored: client.LevelCounts{}}, f.restoreErr
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
	log        *sagaLog
	cfg        *fakeLevelConfig
	people     *fakeLevelPeople
	grades     *fakeLevelData
	attendance *fakeLevelData
	finance    *fakeLevelData
	uniforms   *fakeLevelData
	passwords  *fakePasswords
	now        time.Time
	h          *handler.LevelDeletionHandler
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
	rig.h = handler.NewLevelDeletionHandler(handler.LevelDeletionDeps{
		Config: rig.cfg, Students: rig.people,
		Grades: rig.grades, Attendance: rig.attendance, Finance: rig.finance, Uniforms: rig.uniforms,
		Passwords: rig.passwords, Users: deleterNamer{},
		Tokens: handler.NewDeletionTokens([]byte("0123456789abcdef0123456789abcdef"), func() time.Time { return rig.now }),
		Now:    func() time.Time { return rig.now },
		NewID:  func() string { return delBatch },
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
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
	}{
		{"stale: the data changed since the preview", func(rig *levelDeletionRig) { rig.grades.counts["monthlyGrades"]++ },
			nil, "Primaria", true, http.StatusConflict, "PREVIEW_STALE"},
		{"stale: the preview expired", func(rig *levelDeletionRig) { rig.now = rig.now.Add(11 * time.Minute) },
			nil, "Primaria", true, http.StatusConflict, "PREVIEW_STALE"},
		{"stale: no token", nil, func(string) string { return "" }, "Primaria", true, http.StatusConflict, "PREVIEW_STALE"},
		{"the typed name does not match", nil, nil, "Secundaria", true, http.StatusBadRequest, "NAME_MISMATCH"},
		{"wrong password", nil, nil, "Primaria", false, http.StatusUnauthorized, "REAUTH_FAILED"},
		{"name is trimmed and case-insensitive", nil, nil, "  pRIMARIA ", true, http.StatusOK, ""},
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
			if tc.wantStatus != http.StatusOK {
				assert.Empty(t, rig.log.calls, "a refused deletion archives nothing")
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
	w := rig.delete(t, deleteBody(rig.preview(t).Token, "Primaria", "pw"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, []string{
		"archive:grades", "archive:attendance", "archive:finance", "archive:uniforms", "archive:students", "archive:config",
	}, rig.log.calls, "dependents first, the structure (and the level) last")
	assert.Equal(t, int64(40), rig.cfg.summary["monthlyGrades"], "config records what the others archived")
	assert.Equal(t, int64(2), rig.cfg.summary["students"])
	assert.NotContains(t, rig.cfg.summary, "charges", "kept money is not a deletion count")
	assert.Equal(t, "Laura Díaz", rig.cfg.archivedName)
	assert.Equal(t, "cccccccc-0000-0000-0000-0000000000c1", rig.cfg.archivedBy)

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
			"archive:grades", "archive:attendance", "archive:finance", "archive:uniforms", "archive:students",
			"restore:students", "restore:uniforms", "restore:finance", "restore:attendance", "restore:grades",
		}},
		{"config fails last", func(rig *levelDeletionRig) {
			rig.cfg.archiveErr = errors.New("boom")
			rig.cfg.restoreErr = connect.NewError(connect.CodeNotFound, errors.New("no such batch"))
		}, []string{
			"archive:grades", "archive:attendance", "archive:finance", "archive:uniforms", "archive:students", "archive:config",
			"restore:config", "mark:config", "restore:students", "restore:uniforms", "restore:finance", "restore:attendance", "restore:grades",
		}},
		{"the first step fails", func(rig *levelDeletionRig) { rig.grades.archiveErr = errors.New("boom") }, []string{
			"archive:grades", "restore:grades",
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
			assert.Equal(t, tc.wants, rig.log.calls, "every step that ran (or may have) is put back, newest first")
		})
	}
}

func TestLevelDeletionSaysSoWhenTheCompensationFailsToo(t *testing.T) {
	rig := newLevelDeletionRig(true)
	token := rig.preview(t).Token
	rig.people.archiveErr = errors.New("boom")
	rig.finance.restoreErr = errors.New("still down")
	w := rig.delete(t, deleteBody(token, "Primaria", "pw"))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), delBatch, "support gets the batch to finish the restore by hand")
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
			"restore:config", "restore:students", "restore:uniforms", "restore:finance", "restore:attendance", "restore:grades", "mark:config",
		}, rig.log.calls)
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
		{"past seven days", func(rig *levelDeletionRig) { rig.now = rig.cfg.deletion.RestorableUntil.Add(time.Second) }, http.StatusGone, "TRASH_EXPIRED"},
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

	t.Run("a service failing midway leaves the batch in the trash to retry", func(t *testing.T) {
		rig := newLevelDeletionRig(true)
		rig.finance.restoreErr = errors.New("down")
		w := rig.restore(t)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.False(t, rig.cfg.marked, "not closed: a retry finishes it")
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
