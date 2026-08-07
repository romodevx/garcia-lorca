package repo

import "time"

// notificationModel is the GORM row for `notifications`: one message materialised
// for a student. StudentID is a cross-service UUID (students.students) validated
// upstream — a plain column, never a FK. ReadAt is a POINTER because NULL is
// meaningful here: it is the unread state the badge counts, not a zero instant.
type notificationModel struct {
	ID        string     `gorm:"column:id;primaryKey"`
	TenantID  string     `gorm:"column:tenant_id"`
	StudentID string     `gorm:"column:student_id"`
	Type      string     `gorm:"column:type"`
	Title     string     `gorm:"column:title"`
	Body      string     `gorm:"column:body"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	ReadAt    *time.Time `gorm:"column:read_at"`
}

func (notificationModel) TableName() string { return "notifications" }

// deviceModel is the GORM row for `devices`: one push target. UserID is a
// cross-service UUID (iam.users) — a plain column, never a FK. The token, not the
// id, is the natural key: (tenant_id, fcm_token) is UNIQUE, so a client
// re-registering the same token refreshes this row.
type deviceModel struct {
	ID        string    `gorm:"column:id;primaryKey"`
	TenantID  string    `gorm:"column:tenant_id"`
	UserID    string    `gorm:"column:user_id"`
	FCMToken  string    `gorm:"column:fcm_token"`
	Platform  string    `gorm:"column:platform"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (deviceModel) TableName() string { return "devices" }

// processedEventModel is the GORM row for `processed_events`, the idempotency
// ledger: (tenant_id, event_key) is the composite PRIMARY KEY, so a redelivered
// event's insert conflicts and the redelivery becomes a no-op.
//
// ProcessedAt carries a `default` so GORM omits it from the INSERT while it is
// zero and the column's own DEFAULT now() stamps it: the instant an event was
// handled is the database's to record, not a caller's to assert.
type processedEventModel struct {
	TenantID    string    `gorm:"column:tenant_id;primaryKey"`
	EventKey    string    `gorm:"column:event_key;primaryKey"`
	ProcessedAt time.Time `gorm:"column:processed_at;default:now()"`
}

func (processedEventModel) TableName() string { return "processed_events" }
