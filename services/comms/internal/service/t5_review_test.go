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

func correctedMark(t *testing.T, recordID, status, previous, subjectName string, version int64) pubsub.Message {
	t.Helper()
	subjectID := ""
	if subjectName != "" {
		subjectID = subjectMathID
	}
	return protoMessage(t, "AttendanceMarked", &attendancev1.AttendanceMarked{
		RecordId: recordID, EnrollmentId: "cccccccc-0000-0000-0000-000000000003", StudentId: studentID,
		TenantId: tenantID, Date: "2026-09-15", Status: status, SubjectId: subjectID, SubjectName: subjectName,
		MarkVersion: version, PreviousStatus: previous,
	}, tenantID)
}

func correctedAbsence(t *testing.T, recordID, previous string, version int64) pubsub.Message {
	t.Helper()
	return protoMessage(t, "AbsenceRecorded", &attendancev1.AbsenceRecorded{
		RecordId: recordID, EnrollmentId: "cccccccc-0000-0000-0000-000000000003", StudentId: studentID,
		TenantId: tenantID, Date: "2026-09-15", MarkVersion: version, PreviousStatus: previous,
	}, tenantID)
}

// Review #9: a status change between two exceptions is a CORRECTION of what the
// family was already told, and says what it replaced.
func TestExceptionToExceptionChangeRendersAsCorrection(t *testing.T) {
	const rec = "eeeeeeee-5100-0000-0000-000000000001"
	tests := []struct {
		name      string
		msg       pubsub.Message
		wantTitle string
		wantBody  string
	}{
		{"absent → late", correctedMark(t, rec, "late", "absent", "", 2), "Corrección de asistencia", "Corrección: Ana llegó con retardo el 15 de septiembre (antes: falta)."},
		{"absent → excused", correctedMark(t, rec, "excused", "absent", "", 2), "Corrección de asistencia", "Corrección: Ana tiene falta justificada el 15 de septiembre (antes: falta)."},
		{"late → excused", correctedMark(t, rec, "excused", "late", "", 3), "Corrección de asistencia", "Corrección: Ana tiene falta justificada el 15 de septiembre (antes: retardo)."},
		{"per-class absent → late", correctedMark(t, rec, "late", "absent", "Matemáticas", 2), "Corrección de asistencia", "Corrección: Ana llegó con retardo a Matemáticas el 15 de septiembre (antes: falta)."},
		{"late → absent", correctedAbsence(t, rec, "late", 2), "Corrección de asistencia", "Corrección: Ana faltó el 15 de septiembre (antes: retardo)."},
		{"excused → absent", correctedAbsence(t, rec, "excused", 2), "Corrección de asistencia", "Corrección: Ana faltó el 15 de septiembre (antes: falta justificada)."},
		{"a new exception is not a correction", correctedMark(t, rec, "late", "", "", 1), "Retardo registrado", "Ana llegó con retardo el 15 de septiembre."},
		{"an unknown previous status is not a correction", correctedMark(t, rec, "late", "weird", "", 2), "Retardo registrado", "Ana llegó con retardo el 15 de septiembre."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newNamedService(t, repo, nil, &fakeNames{names: map[string]string{studentID: "Ana"}})
			require.NoError(t, svc.HandleEvent(context.Background(), tt.msg))
			require.Len(t, repo.notifications, 1)
			assert.Equal(t, tt.wantTitle, repo.notifications[0].Title)
			assert.Equal(t, tt.wantBody, repo.notifications[0].Body)
		})
	}
}

// Review #6: only a TRANSIENT directory failure is worth a redelivery. Anything
// else (a malformed answer, an auth error, an internal bug) would loop forever,
// so the notification is stored with the neutral name instead.
func TestStudentNameLookupRetriesOnlyTransientFailures(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantRetry   bool
		wantStored  int
		wantLogLine string
	}{
		{"unavailable redelivers", fmt.Errorf("students: %w", service.ErrStudentsTransient), true, 0, ""},
		{"a permanent failure falls back", glerrors.New(glerrors.KindInternal, "permission denied"), false, 1, "fell back"},
		{"an unclassified failure falls back", errors.New("boom"), false, 1, "fell back"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc, logs := newService(t, repo, newFakeGuardians(), nil, service.WithStudentNames(&fakeNames{err: tt.err}))
			err := svc.HandleEvent(context.Background(), classAbsence(t, "eeeeeeee-5100-0000-0000-000000000002", "", "", 1))
			if tt.wantRetry {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, "Tu hijo(a) faltó el 15 de septiembre.", repo.notifications[0].Body)
				assert.Contains(t, logs.String(), tt.wantLogLine)
			}
			assert.Len(t, repo.notifications, tt.wantStored)
		})
	}
}

// Review #6: a student id that is not a uuid can never be looked up nor
// listed — the event is undeliverable, acked, and nothing is stored.
func TestMalformedStudentIDIsUndeliverable(t *testing.T) {
	repo := newFakeRepo()
	names := &fakeNames{names: map[string]string{}}
	svc := newNamedService(t, repo, nil, names)
	msg := protoMessage(t, "AbsenceRecorded", &attendancev1.AbsenceRecorded{
		RecordId: "eeeeeeee-5100-0000-0000-000000000003", StudentId: "not-a-uuid", TenantId: tenantID, Date: "2026-09-15", MarkVersion: 1,
	}, tenantID)
	require.NoError(t, svc.HandleEvent(context.Background(), msg))
	assert.Empty(t, repo.notifications)
	assert.Zero(t, names.calls)
}
