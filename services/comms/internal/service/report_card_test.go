package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	"github.com/garcia-lorca/pkg/pubsub"
	"github.com/garcia-lorca/services/comms/internal/service"
)

const reportCardID = "7c7c7c7c-0000-0000-0000-000000000001"

func gradesPublished(t *testing.T, cardID, termName string, version int32, tenant string) pubsub.Message {
	t.Helper()
	return protoMessage(t, "GradesPublished", &gradesv1.GradesPublished{
		StudentId: studentID, CycleId: "c0000000-0000-0000-0000-000000000001", TenantId: tenant,
		ReportCardId: cardID, TermId: "70000000-0000-0000-0000-000000000001", TermName: termName,
		Version: version, EnrollmentId: "cccccccc-0000-0000-0000-000000000009",
	}, tenant)
}

// T7: an issued report card tells the pupil's guardians it is available (v1)
// or that it was updated (v>1), naming the pupil and the period, and the push
// opens the report card.
func TestHandleEventRendersReportCards(t *testing.T) {
	tests := []struct {
		name      string
		version   int32
		termName  string
		wantTitle string
		wantBody  string
	}{
		{"first issue", 1, "1.er trimestre", "Boleta del 1.er trimestre disponible", "La boleta del 1.er trimestre de Ana ya está disponible."},
		{"a correction", 2, "1.er trimestre", "Boleta actualizada", "Boleta actualizada: 1.er trimestre de Ana."},
		{"a term without a name", 1, "", "Boleta disponible", "La boleta de Ana ya está disponible."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			_, err := repo.UpsertDevice(context.Background(), service.Device{ID: "d1", UserID: userID, FCMToken: "tok-1", Platform: "ios"})
			require.NoError(t, err)
			sender := &spySender{}
			svc := newNamedService(t, repo, sender, &fakeNames{names: map[string]string{studentID: "Ana"}})

			require.NoError(t, svc.HandleEvent(context.Background(), gradesPublished(t, reportCardID, tt.termName, tt.version, tenantID)))
			require.Len(t, repo.notifications, 1)
			n := repo.notifications[0]
			assert.Equal(t, "report_card", n.Type)
			assert.Equal(t, studentID, n.StudentID)
			assert.Equal(t, tt.wantTitle, n.Title)
			assert.Equal(t, tt.wantBody, n.Body)
			assert.Equal(t, 1, sender.calls)
			assert.Equal(t, map[string]string{"type": "report_card", "studentId": studentID, "reportCardId": reportCardID}, sender.data)
		})
	}
}

// Redelivery of the same card and version is deduplicated; a new version of
// the same period is a new notification (the ledger key carries the version).
func TestReportCardLedgerKeyCarriesTheVersion(t *testing.T) {
	repo := newFakeRepo()
	svc := newNamedService(t, repo, nil, &fakeNames{names: map[string]string{studentID: "Ana"}})
	ctx := context.Background()

	require.NoError(t, svc.HandleEvent(ctx, gradesPublished(t, reportCardID, "1.er trimestre", 1, tenantID)))
	require.NoError(t, svc.HandleEvent(ctx, gradesPublished(t, reportCardID, "1.er trimestre", 1, tenantID)))
	assert.Len(t, repo.notifications, 1, "a redelivery writes nothing")
	assert.True(t, repo.claimed["GradesPublished:"+reportCardID+":1"])

	const v2ID = "7c7c7c7c-0000-0000-0000-000000000002"
	require.NoError(t, svc.HandleEvent(ctx, gradesPublished(t, v2ID, "1.er trimestre", 2, tenantID)))
	assert.Len(t, repo.notifications, 2)
	assert.True(t, repo.claimed["GradesPublished:"+v2ID+":2"])
}

// A report card event that is wrong is acked and dropped, never stored.
func TestReportCardUndeliverableEventsAreDropped(t *testing.T) {
	tests := map[string]pubsub.Message{
		"another tenant":  gradesPublished(t, reportCardID, "1.er trimestre", 1, otherTenantID),
		"no report card":  gradesPublished(t, "", "1.er trimestre", 1, tenantID),
		"no version":      gradesPublished(t, reportCardID, "1.er trimestre", 0, tenantID),
		"garbage payload": {Topic: "grades-events", Data: []byte{0xff, 0xff}, Attributes: map[string]string{"type": "GradesPublished"}},
	}
	for name, msg := range tests {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newNamedService(t, repo, nil, &fakeNames{names: map[string]string{studentID: "Ana"}})
			require.NoError(t, svc.HandleEvent(context.Background(), msg))
			assert.Empty(t, repo.notifications)
		})
	}
}
