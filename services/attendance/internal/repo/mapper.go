package repo

import (
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// mapper.go translates GORM rows ↔ the service's records. It is the seam that
// keeps GORM out of the application layer.

// toRecord rebuilds a service record from its row. The row's audit timestamps are
// storage detail and stay here: the only instant attendance cares about is the
// school day.
func toRecord(m attendanceModel) service.AttendanceRecord {
	return service.AttendanceRecord{
		ID:           m.ID,
		EnrollmentID: m.EnrollmentID,
		Date:         m.Date,
		Status:       m.Status,
		Note:         m.Note,
	}
}

// fromRecord projects a service record into its GORM row for insert, stamping the
// repository's tenant (the RLS policy re-checks it via WITH CHECK, so a row can
// never be written into another tenant). CreatedAt/UpdatedAt are left to GORM's
// write-time tracking.
func fromRecord(rec service.AttendanceRecord, tenantID string) attendanceModel {
	return attendanceModel{
		ID:           rec.ID,
		TenantID:     tenantID,
		EnrollmentID: rec.EnrollmentID,
		Date:         rec.Date,
		Status:       rec.Status,
		Note:         rec.Note,
	}
}
