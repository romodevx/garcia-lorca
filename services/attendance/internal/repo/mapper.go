package repo

import (
	"github.com/garcia-lorca/services/attendance/internal/service"
)

// mapper.go translates GORM rows ↔ the service's records. It is the seam that
// keeps GORM out of the application layer. The one bridge that needs care is the
// subject: the service speaks "" for a daily list, the column NULL.

// toRecord rebuilds a service record from its row. The row's audit timestamps are
// storage detail and stay here: the only instant attendance cares about is the
// school day.
func toRecord(m attendanceModel) service.AttendanceRecord {
	return service.AttendanceRecord{
		ID:           m.ID,
		EnrollmentID: m.EnrollmentID,
		Date:         m.Date,
		SubjectID:    fromNullable(m.SubjectID),
		Status:       m.Status,
		Note:         m.Note,
		Revision:     m.Revision,
	}
}

// fromRecord projects a service record into its GORM row for write, stamping the
// repository's tenant (the RLS policy re-checks it via WITH CHECK, so a row can
// never be written into another tenant). CreatedAt/UpdatedAt are left to GORM's
// write-time tracking.
func fromRecord(rec service.AttendanceRecord, tenantID string) attendanceModel {
	return attendanceModel{
		ID:           rec.ID,
		TenantID:     tenantID,
		EnrollmentID: rec.EnrollmentID,
		Date:         rec.Date,
		SubjectID:    nullable(rec.SubjectID),
		Status:       rec.Status,
		Note:         rec.Note,
	}
}

// toSession rebuilds a list's session from its row.
func toSession(m sessionModel) service.Session {
	return service.Session{TakenAt: m.TakenAt, TakenByName: m.TakenByName, UpdatedAt: m.UpdatedAt}
}

// nullable maps the service's "" (daily list) to a NULL column.
func nullable(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// fromNullable maps a NULL column back to "".
func fromNullable(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// toTakenList projects a session row onto the taken-list view: a NULL subject
// (the daily list) becomes "".
func toTakenList(m sessionModel) service.TakenList {
	out := service.TakenList{GroupID: m.GroupID, Date: m.Date.UTC()}
	if m.SubjectID != nil {
		out.SubjectID = *m.SubjectID
	}
	return out
}
