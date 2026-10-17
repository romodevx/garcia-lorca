package handler_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// T14: a day the school calendar closes reads back as read-only "no_classes"
// on both surfaces, so the screen locks it before anyone types a mark.
func TestRosterOfADayWithoutClassesIsReadOnlyNoClasses(t *testing.T) {
	t.Run("teacher", func(t *testing.T) {
		h := newTeachingAttendance(listTeacher(), &fakeTeachingAttendance{noClasses: true}, &fakeTeachingStudents{})
		rec := httptest.NewRecorder()
		h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-prim&date=2026-09-13", "usr-t"))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		out := decodeRosterMeta(t, rec)
		assert.False(t, out.Editable)
		require.NotNil(t, out.ReadOnlyReason)
		assert.Equal(t, "no_classes", *out.ReadOnlyReason)
	})
	t.Run("teacher future day keeps future", func(t *testing.T) {
		h := newTeachingAttendance(listTeacher(), &fakeTeachingAttendance{noClasses: true}, &fakeTeachingStudents{})
		rec := httptest.NewRecorder()
		h.Roster(rec, teacherGET("/api/v1/teaching/attendance/roster?groupId=grp-prim&date=2026-09-19", "usr-t"))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		out := decodeRosterMeta(t, rec)
		require.NotNil(t, out.ReadOnlyReason)
		assert.Equal(t, "future", *out.ReadOnlyReason)
	})
	t.Run("director", func(t *testing.T) {
		fake := &fakeAttendance{}
		fake.noClasses = true
		h := handler.NewAttendanceHandler(fake, listTeacher(), &fakeTeachingStudents{}, directorUsers, handler.WithDirectorAttendanceClock(fixedClock))
		rec := httptest.NewRecorder()
		h.GetRoster(rec, directorReq(http.MethodGet, "/api/v1/attendance/roster?groupId=grp-prim&date=2026-09-13", ""))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		out := decodeRosterMeta(t, rec)
		assert.False(t, out.Editable)
		require.NotNil(t, out.ReadOnlyReason)
		assert.Equal(t, "no_classes", *out.ReadOnlyReason)
	})
}

// The attendance service's refusal reaches the person as is: 400, its
// sentence, and X-Error-Code NO_CLASSES (both save routes forward it).
func TestSaveOnADayWithoutClassesForwardsTheRefusal(t *testing.T) {
	refusal := connect.NewError(connect.CodeInvalidArgument, errors.New("Ese día no hay clases."))
	refusal.Meta().Set("X-Error-Code", "NO_CLASSES")
	body := `{"groupId":"grp-prim","date":"2026-09-14","marks":[]}`

	fake := &fakeAttendance{}
	fake.saveErr = refusal
	h := handler.NewAttendanceHandler(fake, listTeacher(), &fakeTeachingStudents{}, directorUsers, handler.WithDirectorAttendanceClock(fixedClock))
	rec := httptest.NewRecorder()
	h.SaveDaily(rec, directorReq(http.MethodPost, "/api/v1/attendance", body))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "Ese día no hay clases.", strings.TrimSpace(rec.Body.String()))
	assert.Equal(t, "NO_CLASSES", rec.Header().Get("X-Error-Code"))
}
