package postgres

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/finance/internal/domain"
	"github.com/garcia-lorca/services/finance/internal/ports"
)

// T18 (migration 0026): scholarships and who holds them. Same Repository, same
// tenant binding, its own port.

var _ ports.ScholarshipRepository = (*Repository)(nil)

const errScholarshipNameTaken = "ya existe una beca con ese nombre"

func fromDomainScholarship(s domain.Scholarship) (scholarshipModel, error) {
	targets, err := json.Marshal(s.Targets())
	if err != nil {
		return scholarshipModel{}, glerrors.Wrap(glerrors.KindInternal, "encoding scholarship targets", err)
	}
	return scholarshipModel{
		ID: s.ID(), TenantID: s.TenantID(), Name: s.Name(), Kind: s.Kind(), Value: s.Value(),
		Targets: targets, Active: s.Active(),
	}, nil
}

func toDomainScholarship(m scholarshipModel) (domain.Scholarship, error) {
	var targets []string
	if err := json.Unmarshal(m.Targets, &targets); err != nil {
		return domain.Scholarship{}, glerrors.Wrap(glerrors.KindInternal, "decoding scholarship targets", err)
	}
	return domain.NewScholarship(domain.ScholarshipSpec{
		ID: m.ID, TenantID: m.TenantID, Name: m.Name, Kind: m.Kind, Value: m.Value, Targets: targets, Active: m.Active,
	})
}

func toDomainScholarships(rows []scholarshipModel) ([]domain.Scholarship, error) {
	out := make([]domain.Scholarship, 0, len(rows))
	for _, m := range rows {
		s, err := toDomainScholarship(m)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func scholarshipWriteErr(err error, what string) error {
	if isUniqueViolation(err) {
		return glerrors.Wrap(glerrors.KindConflict, errScholarshipNameTaken, err)
	}
	return glerrors.Wrap(glerrors.KindInternal, what, err)
}

// CreateScholarship stores a new scholarship.
func (r *Repository) CreateScholarship(ctx context.Context, s domain.Scholarship) (domain.Scholarship, error) {
	m, err := fromDomainScholarship(s)
	if err != nil {
		return domain.Scholarship{}, err
	}
	err = r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return scholarshipWriteErr(err, "creating a scholarship")
		}
		return nil
	})
	if err != nil {
		return domain.Scholarship{}, err
	}
	return s, nil
}

// UpdateScholarship replaces a scholarship's fields.
func (r *Repository) UpdateScholarship(ctx context.Context, s domain.Scholarship) (domain.Scholarship, error) {
	if !uuidShaped.MatchString(s.ID()) {
		return domain.Scholarship{}, glerrors.ErrNotFound
	}
	m, err := fromDomainScholarship(s)
	if err != nil {
		return domain.Scholarship{}, err
	}
	err = r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&scholarshipModel{}).Where("id = ?", s.ID()).Updates(map[string]any{
			"name": m.Name, "kind": m.Kind, "value": m.Value, "targets": m.Targets, "active": m.Active,
			"updated_at": gorm.Expr("now()"),
		})
		if res.Error != nil {
			return scholarshipWriteErr(res.Error, "updating a scholarship")
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
	if err != nil {
		return domain.Scholarship{}, err
	}
	return s, nil
}

// GetScholarship reads one scholarship.
func (r *Repository) GetScholarship(ctx context.Context, id string) (domain.Scholarship, error) {
	if !uuidShaped.MatchString(id) {
		return domain.Scholarship{}, glerrors.ErrNotFound
	}
	var out domain.Scholarship
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m scholarshipModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		s, err := toDomainScholarship(m)
		out = s
		return err
	})
	return out, err
}

// ListScholarships lists them by name.
func (r *Repository) ListScholarships(ctx context.Context, includeInactive bool) ([]domain.Scholarship, error) {
	var out []domain.Scholarship
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&scholarshipModel{}).Order("lower(name) ASC, id ASC").Limit(maxListLimit)
		if !includeInactive {
			q = q.Where("active")
		}
		var rows []scholarshipModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing scholarships", err)
		}
		list, err := toDomainScholarships(rows)
		out = list
		return err
	})
	return out, err
}

// CountHolders is, per scholarship, how many pupils hold it now.
func (r *Repository) CountHolders(ctx context.Context) (map[string]int, error) {
	type row struct {
		ScholarshipID string
		N             int
	}
	var rows []row
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Model(&scholarshipHoldingModel{}).
			Select("scholarship_id, count(*) AS n").
			Where("ends_on IS NULL").
			Group("scholarship_id").Scan(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "counting scholarship holders", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(rows))
	for _, x := range rows {
		out[x.ScholarshipID] = x.N
	}
	return out, nil
}

func toDomainHolding(m scholarshipHoldingModel) domain.ScholarshipHolding {
	h := domain.ScholarshipHolding{
		ID: m.ID, TenantID: m.TenantID, ScholarshipID: m.ScholarshipID, StudentID: m.StudentID, StartsOn: m.StartsOn,
	}
	if m.EndsOn != nil {
		h.EndsOn = *m.EndsOn
	}
	return h
}

// ListHolders lists the open holdings of one scholarship, oldest first.
func (r *Repository) ListHolders(ctx context.Context, scholarshipID string) ([]domain.ScholarshipHolding, error) {
	if !uuidShaped.MatchString(scholarshipID) {
		return nil, nil
	}
	var out []domain.ScholarshipHolding
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var rows []scholarshipHoldingModel
		if err := tx.Where("scholarship_id = ? AND ends_on IS NULL", scholarshipID).
			Order("starts_on ASC, id ASC").Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing scholarship holders", err)
		}
		for _, m := range rows {
			out = append(out, toDomainHolding(m))
		}
		return nil
	})
	return out, err
}

// HeldBy is, per pupil, the scholarships they hold now; nil studentIDs means
// every pupil. One query for the holdings, one for their scholarships.
func (r *Repository) HeldBy(ctx context.Context, studentIDs []string) (map[string][]domain.HeldScholarship, error) {
	out := map[string][]domain.HeldScholarship{}
	var ids []string
	if studentIDs != nil {
		if ids = uuidsOnly(studentIDs); len(ids) == 0 {
			return out, nil
		}
	}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Where("ends_on IS NULL")
		if ids != nil {
			q = q.Where("student_id IN ?", ids)
		}
		var holdings []scholarshipHoldingModel
		if err := q.Find(&holdings).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading scholarship holdings", err)
		}
		if len(holdings) == 0 {
			return nil
		}
		wanted := make([]string, 0, len(holdings))
		for _, h := range holdings {
			wanted = append(wanted, h.ScholarshipID)
		}
		var rows []scholarshipModel
		if err := tx.Where("id IN ?", wanted).Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "reading held scholarships", err)
		}
		list, err := toDomainScholarships(rows)
		if err != nil {
			return err
		}
		byID := make(map[string]domain.Scholarship, len(list))
		for _, s := range list {
			byID[s.ID()] = s
		}
		for _, h := range holdings {
			if s, ok := byID[h.ScholarshipID]; ok {
				out[h.StudentID] = append(out[h.StudentID], domain.HeldScholarship{Scholarship: s, Since: h.StartsOn})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Assign stores new open holdings; a pupil already holding the scholarship is
// skipped by the open-holding unique index.
func (r *Repository) Assign(ctx context.Context, hs []domain.ScholarshipHolding) ([]domain.ScholarshipHolding, error) {
	var out []domain.ScholarshipHolding
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		for _, h := range hs {
			m := scholarshipHoldingModel{
				ID: h.ID, TenantID: r.tenantID, ScholarshipID: h.ScholarshipID, StudentID: h.StudentID, StartsOn: h.StartsOn,
			}
			// TargetWhere as a raw literal, as CreateEnrollments: a bound
			// parameter cannot prove the partial index applies.
			res := tx.Clauses(clause.OnConflict{
				Columns:     []clause.Column{{Name: "tenant_id"}, {Name: "scholarship_id"}, {Name: "student_id"}},
				TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{SQL: "ends_on IS NULL"}}},
				DoNothing:   true,
			}).Create(&m)
			if res.Error != nil {
				if isForeignKeyViolation(res.Error) {
					return glerrors.Wrap(glerrors.KindNotFound, "la beca no existe", res.Error)
				}
				return glerrors.Wrap(glerrors.KindInternal, "assigning a scholarship", res.Error)
			}
			if res.RowsAffected == 1 {
				out = append(out, h)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Unassign ends the pupils' open holdings of a scholarship on `on`.
func (r *Repository) Unassign(ctx context.Context, scholarshipID string, studentIDs []string, on time.Time) (int, error) {
	if !uuidShaped.MatchString(scholarshipID) {
		return 0, nil
	}
	ids := uuidsOnly(studentIDs)
	if len(ids) == 0 {
		return 0, nil
	}
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&scholarshipHoldingModel{}).
			Where("scholarship_id = ? AND student_id IN ? AND ends_on IS NULL", scholarshipID, ids).
			Updates(map[string]any{"ends_on": on, "updated_at": gorm.Expr("now()")})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "ending scholarship holdings", res.Error)
		}
		n = res.RowsAffected
		return nil
	})
	return int(n), err
}

// FreezeScholarships writes the scholarship part of charges settled under it,
// each under its row lock: only on a charge not cancelled with no part written
// yet, re-deriving its status from what was collected. A charge that changed
// since it was priced (cancelled, or written by a deposit) is left as it is.
func (r *Repository) FreezeScholarships(ctx context.Context, charges []domain.Charge) (int, error) {
	written := 0
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		for _, c := range charges {
			var row chargeModel
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", c.ID()).First(&row).Error; err != nil {
				return translateFindErr(err)
			}
			if row.Status == domain.StatusCancelled || row.ScholarshipCents > 0 {
				continue
			}
			paid, err := paidFor(tx, c.ID())
			if err != nil {
				return err
			}
			stored, err := toDomainCharge(row, paid)
			if err != nil {
				return err
			}
			// Re-checked under the lock against what is collected NOW: the
			// part never refunds (capped at what is unpaid), and the charge
			// must still be settled with it — a payment voided meanwhile, or a
			// discount written meanwhile, leaves the row to the rule.
			base := stored.Amount().Int64() - stored.Discount().Int64()
			part := min(c.Scholarship().Int64(), base-paid)
			if part <= 0 || paid < base-part {
				continue
			}
			frozen, err := stored.WithScholarship(c.ScholarshipID(), part, paid)
			if err != nil {
				return err
			}
			if err := tx.Model(&chargeModel{}).Where("id = ?", c.ID()).Updates(map[string]any{
				"scholarship_cents": frozen.Scholarship().Int64(),
				"scholarship_id":    optionalString(frozen.ScholarshipID()),
				"status":            frozen.Status(),
				"updated_at":        gorm.Expr("now()"),
			}).Error; err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "writing a settled scholarship", err)
			}
			written++
		}
		return nil
	})
	return written, err
}
