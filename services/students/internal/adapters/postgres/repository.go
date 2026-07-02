package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// Compile-time proof the adapter satisfies the port.
var _ ports.StudentRepository = (*Repository)(nil)

const (
	defaultPageSize = 50
	maxPageSize     = 200
	uniqueViolation = "23505"
)

// Repository is the GORM-backed students repository. Every operation runs in a
// transaction bound to a tenant so Row Level Security applies (mirrors iam /
// ADR 0001). In M03 the tenant is fixed at construction (single-tenant).
type Repository struct {
	db       *gorm.DB
	tenantID string
}

// NewRepository builds the repository scoped to tenantID.
func NewRepository(db *gorm.DB, tenantID string) *Repository {
	return &Repository{db: db, tenantID: tenantID}
}

func (r *Repository) withTenant(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT set_config('app.current_tenant', ?, true)", r.tenantID).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "binding RLS tenant", err)
		}
		return fn(tx)
	})
}

// Create inserts a new applicant. A duplicate CURP (per tenant) is KindConflict.
func (r *Repository) Create(ctx context.Context, s *domain.Student) error {
	model := fromDomain(s)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&model).Error; err != nil {
			if isUniqueViolation(err) {
				return glerrors.Wrap(glerrors.KindConflict, "a student with this CURP already exists", err)
			}
			return glerrors.Wrap(glerrors.KindInternal, "creating student", err)
		}
		return nil
	})
}

// Update persists a student's mutable fields (status + editable attributes).
func (r *Repository) Update(ctx context.Context, s *domain.Student) error {
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&studentModel{}).Where("id = ?", s.ID()).Updates(map[string]any{
			"admission_status": s.Status().String(),
			"grade_group":      s.GradeGroup(),
			"guardian_name":    s.GuardianName(),
			"updated_at":       gorm.Expr("now()"),
		})
		if res.Error != nil {
			return glerrors.Wrap(glerrors.KindInternal, "updating student", res.Error)
		}
		if res.RowsAffected == 0 {
			return glerrors.ErrNotFound
		}
		return nil
	})
}

// FindByID returns a student or KindNotFound.
func (r *Repository) FindByID(ctx context.Context, id string) (*domain.Student, error) {
	var out *domain.Student
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var m studentModel
		if err := tx.Where("id = ?", id).First(&m).Error; err != nil {
			return translateFindErr(err)
		}
		student, err := toDomain(m)
		if err != nil {
			return err
		}
		out = student
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// List returns students matching the filter, keyset-paginated by id. The next
// page token is the last returned id ("" when the last page is reached).
func (r *Repository) List(ctx context.Context, f ports.ListFilter) ([]*domain.Student, string, error) {
	size := f.PageSize
	if size <= 0 {
		size = defaultPageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}

	var (
		students []*domain.Student
		next     string
	)
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		q := tx.Model(&studentModel{}).Order("id ASC").Limit(size + 1)
		if f.Level.Valid() {
			q = q.Where("level = ?", f.Level.String())
		}
		if f.Status != domain.AdmissionUnspecified {
			q = q.Where("admission_status = ?", f.Status.String())
		}
		if f.GradeGroup != "" {
			q = q.Where("grade_group = ?", f.GradeGroup)
		}
		if f.PageToken != "" {
			q = q.Where("id > ?", f.PageToken)
		}

		var rows []studentModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing students", err)
		}
		// A (size+1)th row signals there is a next page; drop it and expose its
		// predecessor's id as the cursor.
		if len(rows) > size {
			next = rows[size-1].ID
			rows = rows[:size]
		}
		for _, m := range rows {
			student, err := toDomain(m)
			if err != nil {
				return err
			}
			students = append(students, student)
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	return students, next, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation
}

func translateFindErr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return glerrors.ErrNotFound
	}
	return glerrors.Wrap(glerrors.KindInternal, "query failed", err)
}
