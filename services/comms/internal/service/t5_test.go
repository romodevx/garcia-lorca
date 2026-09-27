package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/comms/internal/service"
)

const subjectMathID = "5b7a1c2e-0000-4000-8000-00000000000a"

// fakeNames is the students directory as comms sees it: a pupil's first name.
type fakeNames struct {
	names map[string]string
	err   error
	calls int
}

func (f *fakeNames) FirstName(_ context.Context, id string) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	name, ok := f.names[id]
	if !ok {
		return "", glerrors.ErrNotFound
	}
	return name, nil
}

func newNamedService(t *testing.T, repo *fakeRepo, sender service.Sender, names service.StudentNameReader) *service.CommsService {
	t.Helper()
	svc, _ := newService(t, repo, newFakeGuardians(), sender, service.WithStudentNames(names))
	return svc
}

func classAbsence(t *testing.T, recordID, subjectID, subjectName string, version int64) pubsub.Message {
	t.Helper()
	return protoMessage(t, "AbsenceRecorded", &attendancev1.AbsenceRecorded{
		RecordId: recordID, EnrollmentId: "cccccccc-0000-0000-0000-000000000003", StudentId: studentID,
		TenantId: tenantID, Date: "2026-09-15", SubjectId: subjectID, SubjectName: subjectName, MarkVersion: version,
	}, tenantID)
}

func classMarked(t *testing.T, recordID, status, subjectID, subjectName string, version int64) pubsub.Message {
	t.Helper()
	return protoMessage(t, "AttendanceMarked", &attendancev1.AttendanceMarked{
		RecordId: recordID, EnrollmentId: "cccccccc-0000-0000-0000-000000000003", StudentId: studentID,
		TenantId: tenantID, Date: "2026-09-15", Status: status, SubjectId: subjectID, SubjectName: subjectName, MarkVersion: version,
	}, tenantID)
}

// T5 copy: the pupil's first name, the class for a per-class list, a Spanish
// date, and a correction back to present is a notification of its own.
func TestHandleEventRendersNamedClassAwareCopy(t *testing.T) {
	const rec = "eeeeeeee-5000-0000-0000-000000000001"
	tests := []struct {
		name      string
		msg       pubsub.Message
		wantType  string
		wantTitle string
		wantBody  string
	}{
		{"daily absence", classAbsence(t, rec, "", "", 1), "absence", "Ausencia registrada", "Ana faltó el 15 de septiembre."},
		{"per-class absence", classAbsence(t, rec, subjectMathID, "Matemáticas", 1), "absence", "Ausencia registrada", "Ana faltó a Matemáticas el 15 de septiembre."},
		{"per-class absence without a label reads as daily", classAbsence(t, rec, subjectMathID, "", 1), "absence", "Ausencia registrada", "Ana faltó el 15 de septiembre."},
		{"daily late", classMarked(t, rec, "late", "", "", 1), "attendance", "Llegada tarde", "Ana llegó tarde el 15 de septiembre."},
		{"per-class late", classMarked(t, rec, "late", subjectMathID, "Matemáticas", 1), "attendance", "Llegada tarde", "Ana llegó tarde a Matemáticas el 15 de septiembre."},
		{"daily excused", classMarked(t, rec, "excused", "", "", 2), "attendance", "Ausencia justificada", "Ana tiene falta justificada el 15 de septiembre."},
		{"per-class excused", classMarked(t, rec, "excused", subjectMathID, "Matemáticas", 2), "attendance", "Ausencia justificada", "Ana tiene falta justificada en Matemáticas el 15 de septiembre."},
		{"daily correction", classMarked(t, rec, "present", "", "", 2), "attendance", "Corrección de asistencia", "Corrección: Ana sí asistió el 15 de septiembre."},
		{"per-class correction", classMarked(t, rec, "present", subjectMathID, "Matemáticas", 2), "attendance", "Corrección de asistencia", "Corrección: Ana sí asistió el 15 de septiembre (Matemáticas)."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newNamedService(t, repo, nil, &fakeNames{names: map[string]string{studentID: "Ana"}})
			require.NoError(t, svc.HandleEvent(context.Background(), tt.msg))
			require.Len(t, repo.notifications, 1)
			got := repo.notifications[0]
			assert.Equal(t, tt.wantType, got.Type)
			assert.Equal(t, tt.wantTitle, got.Title)
			assert.Equal(t, tt.wantBody, got.Body)
		})
	}
}

// Carry-over (a): one list per class used to render identical copies. Two
// absences on the same day in two classes now read apart.
func TestPerClassAbsencesOfOneDayAreDistinct(t *testing.T) {
	repo := newFakeRepo()
	svc := newNamedService(t, repo, nil, &fakeNames{names: map[string]string{studentID: "Ana"}})
	require.NoError(t, svc.HandleEvent(context.Background(), classAbsence(t, "eeeeeeee-5000-0000-0000-000000000002", subjectMathID, "Matemáticas", 1)))
	require.NoError(t, svc.HandleEvent(context.Background(), classAbsence(t, "eeeeeeee-5000-0000-0000-000000000003", "5b7a1c2e-0000-4000-8000-00000000000b", "Historia", 1)))
	require.Len(t, repo.notifications, 2)
	assert.NotEqual(t, repo.notifications[0].Body, repo.notifications[1].Body)
}

// The absence → present correction is a new ledger key (version), and its
// redelivery is still deduped.
func TestCorrectionToPresentIsIdempotent(t *testing.T) {
	const rec = "eeeeeeee-5000-0000-0000-000000000004"
	repo := newFakeRepo()
	svc := newNamedService(t, repo, nil, &fakeNames{names: map[string]string{studentID: "Ana"}})
	for _, msg := range []pubsub.Message{
		classAbsence(t, rec, "", "", 1),
		classMarked(t, rec, "present", "", "", 2),
		classMarked(t, rec, "present", "", "", 2),
	} {
		require.NoError(t, svc.HandleEvent(context.Background(), msg))
	}
	require.Len(t, repo.notifications, 2)
	assert.True(t, repo.claimed["AttendanceMarked:"+rec+":2"])
}

// A pupil the directory does not know (deleted) falls back to a neutral name; a
// directory OUTAGE stores nothing and asks for redelivery, so the family never
// gets a permanently anonymous message because students blinked.
func TestStudentNameLookup(t *testing.T) {
	t.Run("not found falls back", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newNamedService(t, repo, nil, &fakeNames{names: map[string]string{}})
		require.NoError(t, svc.HandleEvent(context.Background(), classAbsence(t, "eeeeeeee-5000-0000-0000-000000000005", "", "", 1)))
		require.Len(t, repo.notifications, 1)
		assert.Equal(t, "Tu hijo(a) faltó el 15 de septiembre.", repo.notifications[0].Body)
	})
	t.Run("outage redelivers", func(t *testing.T) {
		repo := newFakeRepo()
		svc := newNamedService(t, repo, nil, &fakeNames{err: fmt.Errorf("students down: %w", service.ErrStudentsTransient)})
		err := svc.HandleEvent(context.Background(), classAbsence(t, "eeeeeeee-5000-0000-0000-000000000006", "", "", 1))
		require.Error(t, err)
		assert.Zero(t, repo.recordCalls, "nothing is claimed before the copy can be rendered")
	})
	t.Run("a skipped event never asks the directory", func(t *testing.T) {
		names := &fakeNames{names: map[string]string{studentID: "Ana"}}
		svc := newNamedService(t, newFakeRepo(), nil, names)
		require.NoError(t, svc.HandleEvent(context.Background(), classMarked(t, "eeeeeeee-5000-0000-0000-000000000007", "absent", "", "", 1)))
		assert.Zero(t, names.calls)
	})
	t.Run("a payment never asks the directory", func(t *testing.T) {
		names := &fakeNames{names: map[string]string{studentID: "Ana"}}
		svc := newNamedService(t, newFakeRepo(), nil, names)
		require.NoError(t, svc.HandleEvent(context.Background(), paymentMessage(t, "ffffffff-5000-0000-0000-000000000001", studentID, tenantID, 100)))
		assert.Zero(t, names.calls)
	})
}

// The push carries the data the app routes on: {type, studentId, date}.
func TestAttendancePushCarriesRoutingData(t *testing.T) {
	repo := newFakeRepo()
	repo.devices = []service.Device{{ID: "d1", UserID: userID, FCMToken: "ExponentPushToken[aaa]", Platform: "ios"}}
	sender := &spySender{}
	svc := newNamedService(t, repo, sender, &fakeNames{names: map[string]string{studentID: "Ana"}})
	require.NoError(t, svc.HandleEvent(context.Background(), classMarked(t, "eeeeeeee-5000-0000-0000-000000000008", "late", "", "", 1)))
	require.Equal(t, 1, sender.calls)
	assert.Equal(t, map[string]string{"type": "attendance", "studentId": studentID, "date": "2026-09-15"}, sender.data)
	assert.Equal(t, "Ana llegó tarde el 15 de septiembre.", sender.body)
}

// DeviceNotRegistered: the provider's dead tokens are removed, the others kept,
// and the handler still acks (push is best-effort).
func TestPushDeactivatesUnregisteredDevices(t *testing.T) {
	repo := newFakeRepo()
	repo.devices = []service.Device{
		{ID: "d1", UserID: userID, FCMToken: "ExponentPushToken[dead]", Platform: "ios"},
		{ID: "d2", UserID: userID, FCMToken: "ExponentPushToken[live]", Platform: "android"},
	}
	sender := &spySender{report: service.PushReport{Unregistered: []string{"ExponentPushToken[dead]"}}}
	svc := newNamedService(t, repo, sender, nil)
	require.NoError(t, svc.HandleEvent(context.Background(), classAbsence(t, "eeeeeeee-5000-0000-0000-000000000009", "", "", 1)))
	assert.Equal(t, []string{"ExponentPushToken[dead]"}, repo.deactivated)
	require.Len(t, repo.devices, 1)
	assert.Equal(t, "ExponentPushToken[live]", repo.devices[0].FCMToken)
}

// A failed deactivation is logged and swallowed: the notification is durable.
func TestPushDeactivationFailureIsBestEffort(t *testing.T) {
	repo := newFakeRepo()
	repo.devices = []service.Device{{ID: "d1", UserID: userID, FCMToken: "ExponentPushToken[dead]"}}
	repo.deactivateErr = errors.New("db down")
	sender := &spySender{report: service.PushReport{Unregistered: []string{"ExponentPushToken[dead]"}}}
	svc, logs := newService(t, repo, newFakeGuardians(), sender)
	require.NoError(t, svc.HandleEvent(context.Background(), classAbsence(t, "eeeeeeee-5000-0000-0000-00000000000a", "", "", 1)))
	assert.Contains(t, logs.String(), "could not deactivate")
}

func TestUnregisterDevice(t *testing.T) {
	tests := []struct {
		name    string
		userID  string
		token   string
		wantErr bool
	}{
		{name: "own token", userID: userID, token: "ExponentPushToken[aaa]"},
		{name: "malformed user", userID: "nope", token: "ExponentPushToken[aaa]", wantErr: true},
		{name: "blank token", userID: userID, token: "  ", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc, _ := newService(t, repo, newFakeGuardians(), nil)
			err := svc.UnregisterDevice(context.Background(), tt.userID, tt.token)
			if tt.wantErr {
				assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
				assert.Empty(t, repo.unregistered)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, [][2]string{{tt.userID, tt.token}}, repo.unregistered)
		})
	}
}
