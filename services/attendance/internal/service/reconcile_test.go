package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/attendance/internal/service"
)

// subjectMath is a per-class subject: a real UUID, as the boundary requires.
const subjectMath = "5b7a1c2e-0000-4000-8000-00000000000a"

// --- DiffList (pure) ---------------------------------------------------------

// DiffList is the whole correction rule in one pure function: new exceptions are
// inserted, changed ones are updated IN PLACE (the stored id survives), cleared
// ones are deleted, and untouched ones produce nothing at all.
func TestDiffList(t *testing.T) {
	date := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	stored := func(id, enr, status, note string) service.AttendanceRecord {
		return service.AttendanceRecord{ID: id, EnrollmentID: enr, Date: date, Status: status, Note: note}
	}
	want := func(id, enr, status, note string) service.AttendanceRecord {
		return service.AttendanceRecord{ID: id, EnrollmentID: enr, Date: date, Status: status, Note: note}
	}
	tests := []struct {
		name    string
		current map[string]service.AttendanceRecord
		desired []service.AttendanceRecord
		want    service.ListChanges
	}{
		{
			name:    "first save inserts every exception",
			current: map[string]service.AttendanceRecord{},
			desired: []service.AttendanceRecord{want("new-1", "a", "absent", ""), want("new-2", "b", "late", "8:15")},
			want:    service.ListChanges{Inserts: []service.AttendanceRecord{want("new-1", "a", "absent", ""), want("new-2", "b", "late", "8:15")}},
		},
		{
			name:    "identical re-save changes nothing",
			current: map[string]service.AttendanceRecord{"a": stored("old-a", "a", "absent", "gripa")},
			desired: []service.AttendanceRecord{want("new-1", "a", "absent", "gripa")},
			want:    service.ListChanges{},
		},
		{
			name:    "status change updates in place and notifies",
			current: map[string]service.AttendanceRecord{"a": stored("old-a", "a", "absent", "")},
			desired: []service.AttendanceRecord{want("new-1", "a", "excused", "")},
			want: service.ListChanges{Updates: []service.RecordUpdate{
				{Record: want("old-a", "a", "excused", ""), StatusChanged: true, PreviousStatus: "absent"},
			}},
		},
		{
			name:    "note-only change updates in place silently",
			current: map[string]service.AttendanceRecord{"a": stored("old-a", "a", "late", "")},
			desired: []service.AttendanceRecord{want("new-1", "a", "late", "tráfico")},
			want: service.ListChanges{Updates: []service.RecordUpdate{
				{Record: want("old-a", "a", "late", "tráfico"), StatusChanged: false, PreviousStatus: "late"},
			}},
		},
		{
			name: "cleared exceptions are deleted, in enrollment order",
			current: map[string]service.AttendanceRecord{
				"c": stored("old-c", "c", "late", ""),
				"a": stored("old-a", "a", "absent", ""),
			},
			desired: nil,
			want:    service.ListChanges{Deletes: []service.AttendanceRecord{stored("old-a", "a", "absent", ""), stored("old-c", "c", "late", "")}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, service.DiffList(tt.current, tt.desired))
		})
	}
}

// --- corrections ---------------------------------------------------------------

// A correction never re-notifies what did not change: re-saving the same list
// publishes nothing; changing one status publishes that one (under the STORED
// id); clearing an exception publishes it as "present"; a note-only edit is
// silent. Notes round-trip through the roster.
func TestSaveDailyAttendanceCorrectionsPublishOnlyTheDiff(t *testing.T) {
	repo := newFakeRepo()
	pub := &spyPublisher{}
	svc := newService(t, repo, pub, "rec-1", "rec-2", "rec-3", "rec-4", "rec-5", "rec-6", "rec-7", "rec-8")
	ctx := context.Background()

	first := []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "absent", Note: "gripa"},
		{EnrollmentID: enrollmentBeto, Status: "late"},
	}
	_, _, err := save(svc, ctx, groupID, schoolDay, first)
	require.NoError(t, err)
	require.Len(t, pub.marked, 2)
	require.Len(t, pub.absences, 1)

	// Same list again: nothing changed, nothing published, ids untouched.
	pub.marked, pub.absences = nil, nil
	_, _, err = save(svc, ctx, groupID, schoolDay, first)
	require.NoError(t, err)
	assert.Empty(t, pub.marked, "an unchanged re-save must not re-notify")
	assert.Empty(t, pub.absences)

	// Ana absent→excused (status change), Beto cleared, Cruz note-less late (new).
	_, _, err = save(svc, ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "excused", Note: "gripa"},
		{EnrollmentID: enrollmentCruz, Status: "late", Note: "8:20"},
	})
	require.NoError(t, err)
	byEnrollment := map[string]service.MarkedEvent{}
	for _, e := range pub.marked {
		byEnrollment[e.EnrollmentID] = e
	}
	require.Len(t, pub.marked, 3)
	assert.Equal(t, "excused", byEnrollment[enrollmentAna].Status)
	assert.Equal(t, "rec-1", byEnrollment[enrollmentAna].RecordID, "an updated mark keeps its stored id")
	assert.Equal(t, "present", byEnrollment[enrollmentBeto].Status, "a cleared exception is announced as present")
	assert.Equal(t, "rec-2", byEnrollment[enrollmentBeto].RecordID)
	assert.Equal(t, "late", byEnrollment[enrollmentCruz].Status)
	assert.Empty(t, pub.absences, "no mark became absent")

	// Note-only edit: stored, but silent.
	pub.marked = nil
	_, _, err = save(svc, ctx, groupID, schoolDay, []service.Mark{
		{EnrollmentID: enrollmentAna, Status: "excused", Note: "receta médica"},
		{EnrollmentID: enrollmentCruz, Status: "late", Note: "8:20"},
	})
	require.NoError(t, err)
	assert.Empty(t, pub.marked, "a note-only correction must not notify")

	rows, err := roster(svc, ctx, groupID, schoolDay)
	require.NoError(t, err)
	notes := map[string]string{}
	for _, r := range rows {
		notes[r.EnrollmentID] = r.Status + "/" + r.Note
	}
	assert.Equal(t, "excused/receta médica", notes[enrollmentAna])
	assert.Equal(t, "present/", notes[enrollmentBeto])
	assert.Equal(t, "late/8:20", notes[enrollmentCruz])
}

// A status change INTO absent is an absence the parents must hear about.
func TestSaveDailyAttendanceChangeToAbsentPublishesAbsence(t *testing.T) {
	repo := newFakeRepo()
	pub := &spyPublisher{}
	svc := newService(t, repo, pub, "rec-1", "rec-2")
	ctx := context.Background()

	_, _, err := save(svc, ctx, groupID, schoolDay, []service.Mark{{EnrollmentID: enrollmentAna, Status: "late"}})
	require.NoError(t, err)
	pub.marked, pub.absences = nil, nil

	_, _, err = save(svc, ctx, groupID, schoolDay, []service.Mark{{EnrollmentID: enrollmentAna, Status: "absent"}})
	require.NoError(t, err)
	require.Len(t, pub.absences, 1)
	assert.Equal(t, "rec-1", pub.absences[0].RecordID)
}

// Every stored change of a mark is a NEW version of it, so a consumer that
// dedupes on (record id, version) hears each correction — late→excused, and a
// second absence after absent→excused→absent — while the record id stays the
// stored one. A cleared mark is announced one version past its last stored
// revision; the absence companion carries the same version as its marked event.
func TestSaveDailyAttendanceEveryStatusChangeIsANewMarkVersion(t *testing.T) {
	repo := newFakeRepo()
	pub := &spyPublisher{}
	svc := newService(t, repo, pub, "rec-1", "rec-2", "rec-3", "rec-4", "rec-5", "rec-6")
	ctx := context.Background()
	saveAna := func(status string) {
		t.Helper()
		pub.marked, pub.absences = nil, nil
		marks := []service.Mark{}
		if status != "" {
			marks = append(marks, service.Mark{EnrollmentID: enrollmentAna, Status: status})
		}
		_, _, err := save(svc, ctx, groupID, schoolDay, marks)
		require.NoError(t, err)
	}
	type version struct {
		Status  string
		Version int64
	}
	marked := func() []version {
		out := make([]version, 0, len(pub.marked))
		for _, e := range pub.marked {
			assert.Equal(t, "rec-1", e.RecordID, "the stored id survives every correction")
			out = append(out, version{e.Status, e.MarkVersion})
		}
		return out
	}

	saveAna("absent")
	assert.Equal(t, []version{{"absent", 1}}, marked())
	require.Len(t, pub.absences, 1)
	assert.Equal(t, int64(1), pub.absences[0].MarkVersion)

	saveAna("excused")
	assert.Equal(t, []version{{"excused", 2}}, marked())

	saveAna("absent")
	assert.Equal(t, []version{{"absent", 3}}, marked())
	require.Len(t, pub.absences, 1)
	assert.Equal(t, int64(3), pub.absences[0].MarkVersion, "the second absence is a new version, not a duplicate")

	saveAna("absent")
	assert.Empty(t, pub.marked, "an unchanged re-save publishes nothing")

	saveAna("")
	assert.Equal(t, []version{{"present", 4}}, marked(), "a cleared mark is one version past its last revision")
}

// --- per-class lists -------------------------------------------------------------

// A per-class list is its own key: it is reconciled and read apart from the
// daily list of the same group·day, and its events carry the subject.
func TestPerClassListIsKeyedBySubject(t *testing.T) {
	repo := newFakeRepo()
	pub := &spyPublisher{}
	svc := newService(t, repo, pub, "rec-1", "rec-2")
	ctx := context.Background()

	_, err := svc.SaveDailyAttendance(ctx, service.SaveRequest{
		GroupID: groupID, Date: schoolDay, SubjectID: subjectMath,
		Marks:   []service.Mark{{EnrollmentID: enrollmentAna, Status: "absent"}},
		TakenBy: testTaker.UserID, TakenByName: testTaker.Name,
	})
	require.NoError(t, err)
	assert.Equal(t, subjectMath, repo.lastKey.SubjectID)
	require.Len(t, pub.marked, 1)
	assert.Equal(t, subjectMath, pub.marked[0].SubjectID)
	require.Len(t, pub.absences, 1)
	assert.Equal(t, subjectMath, pub.absences[0].SubjectID)

	perClass, err := svc.GetDailyRoster(ctx, groupID, schoolDay, subjectMath)
	require.NoError(t, err)
	assert.Equal(t, subjectMath, repo.lastMarksSubject)
	daily, err := svc.GetDailyRoster(ctx, groupID, schoolDay, "")
	require.NoError(t, err)

	status := func(rows []service.RosterRow) string {
		for _, r := range rows {
			if r.EnrollmentID == enrollmentAna {
				return r.Status
			}
		}
		return ""
	}
	assert.Equal(t, "absent", status(perClass.Rows))
	assert.Equal(t, "present", status(daily.Rows), "the daily list does not see a per-class mark")
	assert.NotNil(t, perClass.Session)
	assert.Nil(t, daily.Session, "the daily list was never taken")
}

// --- sessions ---------------------------------------------------------------------

// A list nobody saved has no session; saving it — even with everybody present —
// records who took it, and a later correction keeps the first taker.
func TestSaveDailyAttendanceRecordsTheSession(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo, nil)
	ctx := context.Background()

	before, err := svc.GetDailyRoster(ctx, groupID, schoolDay, "")
	require.NoError(t, err)
	assert.Nil(t, before.Session, "an untaken list has no session")

	_, _, err = save(svc, ctx, groupID, schoolDay, nil) // everybody present
	require.NoError(t, err)
	assert.Equal(t, testTaker, repo.lastTaker)
	assert.Equal(t, service.ListKey{GroupID: groupID, Date: day(t, schoolDay)}, repo.lastKey)

	after, err := svc.GetDailyRoster(ctx, groupID, schoolDay, "")
	require.NoError(t, err)
	require.NotNil(t, after.Session, "an all-present list that was taken has a session")
	assert.Equal(t, "Gabriela Rojas", after.Session.TakenByName)
	assert.Equal(t, sessionTakenAt, after.Session.TakenAt)
}

// --- boundary validation ---------------------------------------------------------

func TestSaveDailyAttendanceRejectsBadListInput(t *testing.T) {
	tests := []struct {
		name string
		req  service.SaveRequest
	}{
		{"malformed subject", service.SaveRequest{SubjectID: "math", TakenBy: testTaker.UserID, TakenByName: "x"}},
		{"missing taker", service.SaveRequest{TakenByName: "x"}},
		{"malformed taker", service.SaveRequest{TakenBy: "usr-1", TakenByName: "x"}},
		{"blank taker name", service.SaveRequest{TakenBy: testTaker.UserID, TakenByName: "   "}},
		{"overlong taker name", service.SaveRequest{TakenBy: testTaker.UserID, TakenByName: string(make([]byte, 201))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newService(t, repo, nil)
			tt.req.GroupID, tt.req.Date = groupID, schoolDay
			_, err := svc.SaveDailyAttendance(context.Background(), tt.req)
			wantInvalid(t, err)
			assert.Zero(t, repo.reconcileCalls, "a rejected save must not write")
		})
	}
}

func TestGetDailyRosterRejectsMalformedSubject(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo, nil)
	_, err := svc.GetDailyRoster(context.Background(), groupID, schoolDay, "' OR 1=1")
	wantInvalid(t, err)
	assert.Zero(t, repo.marksForDateCalls)
}

// The taker's name is stored trimmed.
func TestSaveDailyAttendanceTrimsTakerName(t *testing.T) {
	repo := newFakeRepo()
	svc := newService(t, repo, nil)
	_, err := svc.SaveDailyAttendance(context.Background(), service.SaveRequest{
		GroupID: groupID, Date: schoolDay, TakenBy: testTaker.UserID, TakenByName: "  Gabriela Rojas ",
	})
	require.NoError(t, err)
	assert.Equal(t, "Gabriela Rojas", repo.lastTaker.Name)
}
