package repo

import (
	"github.com/garcia-lorca/services/comms/internal/service"
)

// mapper.go translates GORM rows ↔ the service's types. It is the seam that keeps
// GORM out of the application layer.

// toNotification rebuilds a service notification from its row, collapsing the
// nullable read_at into the zero time the service reads as "unread". The tenant
// stays here: the service is bound to one tenant and never carries it.
func toNotification(m notificationModel) service.Notification {
	n := service.Notification{
		ID:        m.ID,
		StudentID: m.StudentID,
		Type:      m.Type,
		Title:     m.Title,
		Body:      m.Body,
		CreatedAt: m.CreatedAt,
	}
	if m.ReadAt != nil {
		n.ReadAt = *m.ReadAt
	}
	return n
}

// fromNotification projects a service notification into its GORM row for insert,
// stamping the repository's tenant (the RLS policy re-checks it via WITH CHECK, so
// a row can never be written into another tenant). A zero ReadAt becomes NULL —
// a new notification is unread, and NULL is what the unread filter tests.
func fromNotification(n service.Notification, tenantID string) notificationModel {
	m := notificationModel{
		ID:        n.ID,
		TenantID:  tenantID,
		StudentID: n.StudentID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		CreatedAt: n.CreatedAt,
	}
	if !n.ReadAt.IsZero() {
		readAt := n.ReadAt
		m.ReadAt = &readAt
	}
	return m
}

// toDevice rebuilds a service device from its row. created_at is storage detail:
// the service only ever needs somewhere to push.
func toDevice(m deviceModel) service.Device {
	return service.Device{
		ID:       m.ID,
		UserID:   m.UserID,
		FCMToken: m.FCMToken,
		Platform: m.Platform,
	}
}

// fromDevice projects a service device into its GORM row for upsert, stamping the
// repository's tenant. CreatedAt is left to GORM's write-time tracking.
func fromDevice(d service.Device, tenantID string) deviceModel {
	return deviceModel{
		ID:       d.ID,
		TenantID: tenantID,
		UserID:   d.UserID,
		FCMToken: d.FCMToken,
		Platform: d.Platform,
	}
}
