package domain

import (
	"strings"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// EnrollmentTransfer is the dated, reasoned history of moving an enrollment from
// one group to another within the same cycle ("traslado entre grupos con fecha y
// motivo"). It is an immutable record; a new one is written per move.
type EnrollmentTransfer struct {
	id           string
	tenantID     string
	enrollmentID string
	fromGroupID  string
	toGroupID    string
	reason       string
	movedAt      time.Time
}

// NewEnrollmentTransfer builds a validated transfer record. All fields are
// required (a reason is mandatory) and the destination group must differ from the
// origin.
func NewEnrollmentTransfer(id, tenantID, enrollmentID, fromGroupID, toGroupID, reason string, movedAt time.Time) (EnrollmentTransfer, error) {
	if strings.TrimSpace(id) == "" {
		return EnrollmentTransfer{}, glerrors.New(glerrors.KindInvalid, "transfer id is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return EnrollmentTransfer{}, glerrors.New(glerrors.KindInvalid, "tenant id is required")
	}
	if strings.TrimSpace(enrollmentID) == "" {
		return EnrollmentTransfer{}, glerrors.New(glerrors.KindInvalid, "enrollment id is required")
	}
	if strings.TrimSpace(fromGroupID) == "" {
		return EnrollmentTransfer{}, glerrors.New(glerrors.KindInvalid, "from group id is required")
	}
	if strings.TrimSpace(toGroupID) == "" {
		return EnrollmentTransfer{}, glerrors.New(glerrors.KindInvalid, "to group id is required")
	}
	if strings.TrimSpace(reason) == "" {
		return EnrollmentTransfer{}, glerrors.New(glerrors.KindInvalid, "a transfer reason is required")
	}
	if fromGroupID == toGroupID {
		return EnrollmentTransfer{}, glerrors.New(glerrors.KindInvalid, "from and to group must differ")
	}
	return EnrollmentTransfer{
		id:           id,
		tenantID:     tenantID,
		enrollmentID: enrollmentID,
		fromGroupID:  fromGroupID,
		toGroupID:    toGroupID,
		reason:       reason,
		movedAt:      movedAt,
	}, nil
}

// Accessors.
func (t EnrollmentTransfer) ID() string           { return t.id }
func (t EnrollmentTransfer) TenantID() string     { return t.tenantID }
func (t EnrollmentTransfer) EnrollmentID() string { return t.enrollmentID }
func (t EnrollmentTransfer) FromGroupID() string  { return t.fromGroupID }
func (t EnrollmentTransfer) ToGroupID() string    { return t.toGroupID }
func (t EnrollmentTransfer) Reason() string       { return t.reason }
func (t EnrollmentTransfer) MovedAt() time.Time   { return t.movedAt }
