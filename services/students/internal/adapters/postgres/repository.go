package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/garcia-lorca/pkg/authctx"
	"github.com/garcia-lorca/pkg/authctx/gormtx"
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

// withTenant runs fn inside a transaction that has bound app.current_tenant,
// delegating to the shared pkg/authctx RLS primitive so tenant binding lives in
// exactly one place across every service.
func (r *Repository) withTenant(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return authctx.WithTenantContext(ctx, gormtx.NewRunner(r.db), r.tenantID, func(tx authctx.Tx) error {
		return fn(gormtx.Gorm(tx))
	})
}

// Create inserts a new applicant. A duplicate CURP (per tenant) is KindConflict.
func (r *Repository) Create(ctx context.Context, s *domain.Student) error {
	model := fromDomain(s)
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		number, err := r.nextEnrollmentNumber(tx)
		if err != nil {
			return err
		}
		model.EnrollmentNumber = number
		if err := tx.Create(&model).Error; err != nil {
			return translateStudentWriteErr(err, "creating student")
		}
		s.AssignEnrollmentNumber(number)
		return nil
	})
}

// maxEnrollmentSeq is the last number a matrícula's five digits can carry.
const maxEnrollmentSeq = 99999

// nextEnrollmentNumber allocates the school's next matrícula for the year of
// this transaction (task 13, migration 0013), under the counter row's own lock.
// Per school: a SEQUENCE is per database and ignores RLS, so two schools would
// share one run and read each other's size in the gaps.
func (r *Repository) nextEnrollmentNumber(tx *gorm.DB) (string, error) {
	var got struct {
		Year int
		Seq  int
	}
	err := tx.Raw(`INSERT INTO student_number_counters (tenant_id, year, next_seq)
	               VALUES (?, EXTRACT(YEAR FROM now() AT TIME ZONE 'UTC')::int, 2)
	               ON CONFLICT (tenant_id, year)
	               DO UPDATE SET next_seq = student_number_counters.next_seq + 1
	               RETURNING year, next_seq - 1 AS seq`, r.tenantID).Scan(&got).Error
	if err != nil {
		return "", glerrors.Wrap(glerrors.KindInternal, "allocating a matrícula", err)
	}
	if got.Seq > maxEnrollmentSeq {
		// Five digits: the year has no matrículas left. Said as such, not as a
		// CHECK violation reported as "creating student".
		return "", glerrors.New(glerrors.KindInternal, "no matrícula numbers left for this year")
	}
	return domain.FormatEnrollmentNumber(got.Year, got.Seq), nil
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

// UpdateDetails persists a correction of the file: names, CURP and profile. A
// CURP another pupil holds is KindConflict (the (tenant, curp) UNIQUE).
func (r *Repository) UpdateDetails(ctx context.Context, s *domain.Student) error {
	cols, err := profileColumns(s.Profile())
	if err != nil {
		return err
	}
	cols["first_name"] = s.FirstName()
	cols["last_name"] = s.LastName()
	cols["maternal_last_name"] = nullableText(s.MaternalLastName())
	cols["curp"] = s.CURP().String()
	cols["bank_reference"] = nullableText(s.BankReference().String())
	cols["updated_at"] = gorm.Expr("now()")
	return r.withTenant(ctx, func(tx *gorm.DB) error {
		res := tx.Model(&studentModel{}).Where("id = ?", s.ID()).Updates(cols)
		if res.Error != nil {
			return translateStudentWriteErr(res.Error, "updating student details")
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

// surnameOrder is how every list reads (T3): paternal surname, maternal
// surname, first name — in es_primary, where accents and case do not move a
// pupil — and id as the tie-break that makes the order total. It must match the
// expressions of idx_students_surname_order (migration 0017).
const surnameOrder = "last_name COLLATE es_primary, COALESCE(maternal_last_name, '') COLLATE es_primary, first_name COLLATE es_primary, id"

// afterKey keeps the keyset past a sort key carried by the page token, so a
// walk neither stops nor skips when the cursor pupil is renamed or leaves the
// list between two page reads.
const afterKey = "(" + surnameOrder + ") > (?, ?, ?, ?::uuid)"

// afterLegacyID serves a token issued before T3 (a bare id): the cursor pupil's
// key is read back in the same statement.
const afterLegacyID = "(" + surnameOrder + ") > (SELECT last_name, COALESCE(maternal_last_name, ''), first_name, id FROM students WHERE id = ?)"

// cursorPrefix marks a page token that carries the sort key (T3).
const cursorPrefix = "k1."

// pageCursor is the sort key of the last pupil a page returned.
type pageCursor struct {
	Last     string `json:"l"`
	Maternal string `json:"m"`
	First    string `json:"f"`
	ID       string `json:"i"`
}

// encodeCursor turns the last row of a page into its opaque next-page token.
func encodeCursor(m studentModel) string {
	raw, _ := json.Marshal(pageCursor{Last: m.LastName, Maternal: derefOrEmpty(m.MaternalLastName), First: m.FirstName, ID: m.ID})
	return cursorPrefix + base64.RawURLEncoding.EncodeToString(raw)
}

// afterToken narrows q to the pupils after the token's cursor. A token that is
// not ours is the caller's mistake (KindInvalid).
func afterToken(q *gorm.DB, token string) (*gorm.DB, error) {
	encoded, ok := strings.CutPrefix(token, cursorPrefix)
	if !ok {
		return q.Where(afterLegacyID, token), nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, glerrors.Wrap(glerrors.KindInvalid, "malformed page token", err)
	}
	var c pageCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, glerrors.Wrap(glerrors.KindInvalid, "malformed page token", err)
	}
	return q.Where(afterKey, c.Last, c.Maternal, c.First, c.ID), nil
}

// List returns students matching the filter in surnameOrder, keyset-paginated.
// The next page token is opaque ("" when the last page is reached).
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
		q := tx.Model(&studentModel{}).Order(surnameOrder).Limit(size + 1)
		if f.LevelID != "" {
			q = q.Where("level_id = ?", f.LevelID)
		}
		if f.Status != domain.AdmissionUnspecified {
			q = q.Where("admission_status = ?", f.Status.String())
		}
		if f.GradeGroup != "" {
			q = q.Where("grade_group = ?", f.GradeGroup)
		}
		if len(f.IDs) > 0 {
			q = q.Where("id IN ?", f.IDs)
		}
		if f.Birthday != nil {
			q = whereBornOn(q, *f.Birthday)
		}
		if f.PageToken != "" {
			var err error
			if q, err = afterToken(q, f.PageToken); err != nil {
				return err
			}
		}

		var rows []studentModel
		if err := q.Find(&rows).Error; err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "listing students", err)
		}
		// A (size+1)th row signals there is a next page; drop it and expose its
		// predecessor's id as the cursor.
		if len(rows) > size {
			next = encodeCursor(rows[size-1])
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

// whereBornOn narrows q to pupils born on the filter's calendar day, of any
// year — plus February 29 when asked (the greeting of a leap-day pupil in a year
// without one). A pupil with no birth date matches nothing: NULL compares false.
//
// No index: it runs once a day over one school's pupils, already narrowed by
// tenant and status. An expression index on (month, day) is the move if a
// school ever grows big enough for this scan to show.
func whereBornOn(q *gorm.DB, b ports.BirthdayFilter) *gorm.DB {
	const sameDay = "(EXTRACT(MONTH FROM birth_date) = ? AND EXTRACT(DAY FROM birth_date) = ?)"
	if b.IncludeFeb29 {
		return q.Where("("+sameDay+" OR "+sameDay+")", b.Month, b.Day, february, leapDay)
	}
	return q.Where(sameDay, b.Month, b.Day)
}

// february and leapDay are the leap day, February 29.
const (
	february = 2
	leapDay  = 29
)

// The UNIQUE constraints of `students`. A unique violation is told apart by
// its constraint name: each one means something different to the school.
const (
	// enrollmentNumberUniqueConstraint is UNIQUE (tenant_id, enrollment_number)
	// (migration 0013).
	enrollmentNumberUniqueConstraint = "students_enrollment_number_unique"
	// curpUniqueConstraint is the inline UNIQUE (tenant_id, curp) (0001).
	curpUniqueConstraint = "students_tenant_id_curp_key"
	// bankReferenceUniqueIndex is the partial unique index of 0019.
	bankReferenceUniqueIndex = "uq_students_bank_reference"
)

// translateStudentWriteErr maps a failed insert/update of `students` to a
// domain error by the constraint it broke. The domain sentinels stay in the
// chain (errors.Is) and the driver error follows them for the logs.
func translateStudentWriteErr(err error, doing string) error {
	switch {
	case violates(err, bankReferenceUniqueIndex):
		return fmt.Errorf("%w: %w", domain.ErrBankReferenceTaken, err)
	case violates(err, curpUniqueConstraint):
		return fmt.Errorf("%w: %w", domain.ErrCURPTaken, err)
	case violates(err, enrollmentNumberUniqueConstraint):
		// The counter is behind the stored numbers: a fault to fix, never
		// "that CURP already exists".
		return glerrors.Wrap(glerrors.KindInternal, "the matrícula counter is behind its numbers", err)
	case isUniqueViolation(err):
		return glerrors.Wrap(glerrors.KindConflict, "a student with these values already exists", err)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == checkViolation {
		return glerrors.Wrap(glerrors.KindInvalid, "a value is not allowed", err)
	}
	return glerrors.Wrap(glerrors.KindInternal, doing, err)
}

// violates reports whether err is a unique violation of the named constraint.
func violates(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == constraint
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
