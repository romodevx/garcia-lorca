//go:build integration

package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	pgadapter "github.com/garcia-lorca/services/students/internal/adapters/postgres"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

const withdrawalTenant = "55555555-5555-5555-5555-555555555551"

func TestWithdrawalAndOutboxAreCommittedAndRelayed(t *testing.T) {
	ctx := context.Background()
	const studentID = "aaaaaaaa-0000-0000-0000-000000002001"
	const enrollmentID = "bbbbbbbb-0000-0000-0000-000000002001"
	const cycleID = "cccccccc-0000-0000-0000-000000002001"
	repo := appRepo(t, withdrawalTenant)
	seedStudent(t, withdrawalTenant, studentID, "WITA050312MDFRNN01")
	markStudentEnrolled(t, repo, studentID, cycleID)
	enrollment, err := domain.NewEnrollment(enrollmentID, withdrawalTenant, studentID, cycleID,
		"dddddddd-0000-0000-0000-000000002001", "", domain.EnrollmentStatusActive, time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, repo.CreateEnrollment(ctx, enrollment))
	withdrawn, err := enrollment.Withdraw(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "  se mudó  ", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, repo.WithdrawEnrollment(ctx, withdrawn))

	stored, err := repo.GetEnrollment(ctx, enrollmentID)
	require.NoError(t, err)
	assert.Equal(t, "se mudó", stored.WithdrawalReason())
	assert.Equal(t, withdrawn.WithdrawnOn(), stored.WithdrawnOn())
	student, err := repo.FindByID(ctx, studentID)
	require.NoError(t, err)
	assert.Equal(t, domain.AdmissionWithdrawn, student.Status())

	var sent []ports.OutboxEvent
	n, err := repo.DrainOutbox(ctx, 10, func(_ context.Context, event ports.OutboxEvent) error {
		// If publication still holds the claim transaction's row lock, the
		// independent connection cannot lock the same event here.
		var unlockedID string
		if err := superDB.Raw(`SELECT id FROM students_outbox WHERE tenant_id = ? AND event_type = ? AND published_at IS NULL ORDER BY created_at DESC LIMIT 1 FOR UPDATE NOWAIT`, withdrawalTenant, event.Type).Scan(&unlockedID).Error; err != nil {
			return err
		}
		if unlockedID == "" {
			return errors.New("claimed event is not visible outside its transaction")
		}
		sent = append(sent, event)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	require.Len(t, sent, 2)
	events := map[string]ports.OutboxEvent{}
	for _, event := range sent {
		events[event.Type] = event
	}
	assert.Equal(t, withdrawalTenant, events["StudentWithdrawn"].TenantID)
	var event studentsv1.StudentWithdrawn
	require.NoError(t, proto.Unmarshal(events["StudentWithdrawn"].Payload, &event))
	assert.Equal(t, studentID, event.GetStudentId())
	assert.Equal(t, cycleID, event.GetCycleId())
	assert.Equal(t, enrollmentID, event.GetEnrollmentId())
	assert.Equal(t, "2026-10-01", event.GetWithdrawnOn())
	var statusEvent studentsv1.AdmissionStatusChanged
	require.NoError(t, proto.Unmarshal(events["AdmissionStatusChanged"].Payload, &statusEvent))
	assert.Equal(t, studentID, statusEvent.GetStudentId())
	assert.Equal(t, studentsv1.AdmissionStatus_ADMISSION_STATUS_ENROLLED, statusEvent.GetPreviousStatus())
	assert.Equal(t, studentsv1.AdmissionStatus_ADMISSION_STATUS_WITHDRAWN, statusEvent.GetNewStatus())
}

func TestWithdrawalKeepsAdmissionStatusWhenAnotherEnrollmentIsActive(t *testing.T) {
	ctx := context.Background()
	const studentID = "aaaaaaaa-0000-0000-0000-000000002004"
	const enrollmentID = "bbbbbbbb-0000-0000-0000-000000002004"
	repo := appRepo(t, withdrawalTenant)
	seedStudent(t, withdrawalTenant, studentID, "WITA050312MDFRNN04")
	markStudentEnrolled(t, repo, studentID, "cccccccc-0000-0000-0000-000000002004")
	first, err := domain.NewEnrollment(enrollmentID, withdrawalTenant, studentID,
		"cccccccc-0000-0000-0000-000000002004", "dddddddd-0000-0000-0000-000000002004", "", domain.EnrollmentStatusActive, time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	second, err := domain.NewEnrollment("bbbbbbbb-0000-0000-0000-000000002005", withdrawalTenant, studentID,
		"cccccccc-0000-0000-0000-000000002005", "dddddddd-0000-0000-0000-000000002005", "", domain.EnrollmentStatusActive, time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, repo.CreateEnrollment(ctx, first))
	require.NoError(t, repo.CreateEnrollment(ctx, second))
	withdrawn, err := first.Withdraw(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "reason", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, repo.WithdrawEnrollment(ctx, withdrawn))

	student, err := repo.FindByID(ctx, studentID)
	require.NoError(t, err)
	assert.Equal(t, domain.AdmissionEnrolled, student.Status())
}

func markStudentEnrolled(t *testing.T, repo *pgadapter.Repository, studentID, cycleID string) {
	t.Helper()
	student, err := repo.FindByID(context.Background(), studentID)
	require.NoError(t, err)
	_, err = student.Accept()
	require.NoError(t, err)
	_, _, err = student.Enroll(cycleID)
	require.NoError(t, err)
	require.NoError(t, repo.Update(context.Background(), student))
}

func TestWithdrawalOutboxRetriesAndIsTenantIsolated(t *testing.T) {
	ctx := context.Background()
	const studentID = "aaaaaaaa-0000-0000-0000-000000002002"
	const enrollmentID = "bbbbbbbb-0000-0000-0000-000000002002"
	repo := appRepo(t, withdrawalTenant)
	seedStudent(t, withdrawalTenant, studentID, "WITA050312MDFRNN02")
	markStudentEnrolled(t, repo, studentID, "cccccccc-0000-0000-0000-000000002002")
	enrollment, err := domain.NewEnrollment(enrollmentID, withdrawalTenant, studentID,
		"cccccccc-0000-0000-0000-000000002002", "dddddddd-0000-0000-0000-000000002002", "", domain.EnrollmentStatusActive, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, repo.CreateEnrollment(ctx, enrollment))
	withdrawn, err := enrollment.Withdraw(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "reason", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, repo.WithdrawEnrollment(ctx, withdrawn))
	n, err := repo.DrainOutbox(ctx, 10, func(context.Context, ports.OutboxEvent) error { return errors.New("broker down") })
	require.NoError(t, err)
	assert.Zero(t, n)

	var attempts int
	var outboxID string
	require.NoError(t, superDB.Raw("SELECT id FROM students_outbox WHERE tenant_id = ? AND published_at IS NULL ORDER BY created_at DESC LIMIT 1", withdrawalTenant).Scan(&outboxID).Error)
	require.NoError(t, superDB.Raw("SELECT attempts FROM students_outbox WHERE id = ?", outboxID).Scan(&attempts).Error)
	assert.Equal(t, 1, attempts)
	var next time.Time
	require.NoError(t, superDB.Raw("SELECT next_attempt_at FROM students_outbox WHERE id = ?", outboxID).Scan(&next).Error)
	assert.True(t, next.After(time.Now()))
	require.NoError(t, superDB.Exec("UPDATE students_outbox SET next_attempt_at = now() - interval '1 second' WHERE id = ?", outboxID).Error)
	n, err = repo.DrainOutbox(ctx, 10, func(context.Context, ports.OutboxEvent) error { return nil })
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	require.NoError(t, superDB.Raw("SELECT attempts FROM students_outbox WHERE id = ?", outboxID).Scan(&attempts).Error)
	assert.Equal(t, 2, attempts)

	const otherTenant = "55555555-5555-5555-5555-555555555552"
	other := appRepo(t, otherTenant)
	otherStudentID := "aaaaaaaa-0000-0000-0000-000000002003"
	seedStudent(t, otherTenant, otherStudentID, "WITA050312MDFRNN03")
	markStudentEnrolled(t, other, otherStudentID, "cccccccc-0000-0000-0000-000000002003")
	otherEnrollment, err := domain.NewEnrollment("bbbbbbbb-0000-0000-0000-000000002003", otherTenant, otherStudentID,
		"cccccccc-0000-0000-0000-000000002003", "dddddddd-0000-0000-0000-000000002003", "", domain.EnrollmentStatusActive, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, other.CreateEnrollment(ctx, otherEnrollment))
	otherWithdrawal, err := otherEnrollment.Withdraw(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "reason", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NoError(t, other.WithdrawEnrollment(ctx, otherWithdrawal))
	n, err = repo.DrainOutbox(ctx, 10, func(context.Context, ports.OutboxEvent) error {
		t.Fatal("tenant A drained tenant B's event")
		return nil
	})
	require.NoError(t, err)
	assert.Zero(t, n)
}
