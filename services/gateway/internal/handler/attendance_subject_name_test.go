package handler_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// T5: a per-class save carries the class's NAME to attendance, so the family's
// notification says which class; a daily save carries none.
func TestSavesCarryTheSubjectName(t *testing.T) {
	t.Run("director per-class", func(t *testing.T) {
		att := &fakeAttendance{}
		rec := httptest.NewRecorder()
		newDirectorAttendance(att).SaveDaily(rec, directorReq(http.MethodPost, "/api/v1/attendance",
			`{"groupId":"grp-sec","date":"2026-09-15","subjectId":"sub-mat1","marks":[]}`))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "Matemáticas I", att.gotSave.SubjectName)
	})
	t.Run("one point read names the class, never the whole catalog", func(t *testing.T) {
		config := listTeacher()
		rec := httptest.NewRecorder()
		handler.NewAttendanceHandler(&fakeAttendance{}, config, &fakeTeachingStudents{}, directorUsers).SaveDaily(rec,
			directorReq(http.MethodPost, "/api/v1/attendance", `{"groupId":"grp-sec","date":"2026-09-15","subjectId":"sub-mat1","marks":[]}`))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, 1, config.gotGetSubject)
		assert.Zero(t, config.gotListSubjects)
	})
	t.Run("director daily", func(t *testing.T) {
		att := &fakeAttendance{}
		rec := httptest.NewRecorder()
		newDirectorAttendance(att).SaveDaily(rec, directorReq(http.MethodPost, "/api/v1/attendance",
			`{"groupId":"grp-prim","date":"2026-09-15","marks":[]}`))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Empty(t, att.gotSave.SubjectName)
	})
	t.Run("teacher per-class", func(t *testing.T) {
		att := &fakeTeachingAttendance{}
		rec := httptest.NewRecorder()
		newTeachingAttendance(listTeacher(), att, &fakeTeachingStudents{}).SaveAttendance(rec,
			teacherPOST("/api/v1/teaching/attendance", "usr-t", `{"groupId":"grp-sec","date":"2026-09-15","subjectId":"sub-mat1","marks":[]}`))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "Matemáticas I", att.gotSave.SubjectName)
	})
}

// The label is a courtesy: a catalog that cannot answer never blocks a save,
// it only leaves the name empty (comms then renders the daily copy).
func TestSubjectNameLookupFailureNeverBlocksTheSave(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	config := listTeacher()
	config.getSubjectErr = connect.NewError(connect.CodeUnavailable, errString("config down"))
	att := &fakeTeachingAttendance{}
	rec := httptest.NewRecorder()
	newTeachingAttendance(config, att, &fakeTeachingStudents{}).SaveAttendance(rec,
		teacherPOST("/api/v1/teaching/attendance", "usr-t", `{"groupId":"grp-sec","date":"2026-09-15","subjectId":"sub-mat1","marks":[]}`))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, att.saveCalls)
	assert.Empty(t, att.gotSave.SubjectName)
	assert.Contains(t, logs.String(), "level=WARN", "a degraded label is logged (review #10)")
	assert.Contains(t, logs.String(), "subject_id=sub-mat1")
}
