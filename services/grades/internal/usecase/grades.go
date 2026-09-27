package usecase

import (
	"context"
	"sort"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// enrollmentStatusActive is the only enrollment status that may receive grades.
const enrollmentStatusActive = "active"

// RosterRow is one line of a teaching assignment's roster for a term: a student
// (identified by enrollment + student id) and their current grade, if any. The
// student's name is resolved in the web from the students list — grades never
// carries PII.
type RosterRow struct {
	EnrollmentID string
	StudentID    string
	Value        float64
	HasValue     bool
}

// GradesService captures and edits numeric grades and builds the roster for a
// teaching assignment. It orchestrates three seams: the local
// GradeEntryRepository, the cross-service AssignmentReader (config, by RPC) and
// EnrollmentReader (students, by RPC) — all faked in tests. subject_id and
// teacher_id are always DERIVED from the assignment; a client can never set
// them. It depends only on ports + domain.
//
//	Capture: assignment (RPC) ─> enrollment (RPC, in group + active + same cycle)
//	         ─> term (RPC, in cycle) ─> upsert grade_entry (subject/teacher derived)
type GradesService struct {
	entries     ports.GradeEntryRepository
	assignments ports.AssignmentReader
	catalog     ports.CatalogReader
	enrollments ports.EnrollmentReader
	tenantID    string
	newID       func() string
	now         func() time.Time
}

// NewGradesService wires the use case. tenantID is the single-tenant scope for
// this phase; newID mints ids and now stamps entries (both injectable for
// deterministic tests).
func NewGradesService(
	entries ports.GradeEntryRepository,
	assignments ports.AssignmentReader,
	catalog ports.CatalogReader,
	enrollments ports.EnrollmentReader,
	tenantID string,
	newID func() string,
	now func() time.Time,
) *GradesService {
	return &GradesService{
		entries:     entries,
		assignments: assignments,
		catalog:     catalog,
		enrollments: enrollments,
		tenantID:    tenantID,
		newID:       newID,
		now:         now,
	}
}

// CaptureGrade runs the capture knot and upserts one grade entry. In order: the
// assignment must exist (config RPC — yields group/subject/teacher/cycle); the
// enrollment must exist, be in the assignment's group, be active, and be in the
// assignment's cycle (students RPC); the term must exist and belong to that same
// cycle (config RPC). subject_id and teacher_id are DERIVED from the assignment;
// value is validated by the domain constructor. The upsert inserts or updates
// the value on the (enrollment, subject, term) key ("capturar/editar"). Any
// cross-service reference miss is surfaced as KindInvalid (bad input), never a
// NotFound leak.
func (s *GradesService) CaptureGrade(ctx context.Context, assignmentID, enrollmentID, termID string, value float64) (domain.GradeEntry, error) {
	asg, err := s.requireAssignment(ctx, assignmentID)
	if err != nil {
		return domain.GradeEntry{}, err
	}

	enr, err := s.enrollments.ByID(ctx, enrollmentID)
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return domain.GradeEntry{}, glerrors.New(glerrors.KindInvalid, "enrollment not found")
		}
		return domain.GradeEntry{}, err
	}
	if enr.GroupID != asg.GroupID {
		return domain.GradeEntry{}, glerrors.New(glerrors.KindInvalid, "student is not in this assignment's group")
	}
	if enr.Status != enrollmentStatusActive {
		return domain.GradeEntry{}, glerrors.New(glerrors.KindInvalid, "enrollment is not active")
	}
	if enr.CycleID != asg.CycleID {
		return domain.GradeEntry{}, glerrors.New(glerrors.KindInvalid, "enrollment is not in the assignment's cycle")
	}

	if err := s.requireTermOfAssignment(ctx, termID, asg); err != nil {
		return domain.GradeEntry{}, err
	}

	// subject_id and teacher_id are DERIVED from the assignment, never client input.
	entry, err := domain.NewGradeEntry(s.newID(), s.tenantID, enrollmentID, asg.SubjectID, termID, asg.TeacherID, value, s.now())
	if err != nil {
		return domain.GradeEntry{}, err // out-of-range value → KindInvalid propagates
	}
	return s.entries.UpsertGradeEntry(ctx, entry)
}

// Roster lists one row per ACTIVE enrollment in the assignment's group, joined
// with the current grade value for the term (absent → HasValue false). Rows are
// returned in deterministic order by enrollment id.
func (s *GradesService) Roster(ctx context.Context, assignmentID, termID string) ([]RosterRow, error) {
	asg, err := s.requireAssignment(ctx, assignmentID)
	if err != nil {
		return nil, err
	}
	if err := s.requireTermOfAssignment(ctx, termID, asg); err != nil {
		return nil, err
	}

	enrollments, err := s.enrollments.ByGroup(ctx, asg.GroupID)
	if err != nil {
		return nil, err
	}

	// Collect the active enrollments, then resolve every grade in ONE batched,
	// tenant-bound read (no per-student query — the roster is O(1) round-trips).
	activeIDs := make([]string, 0, len(enrollments))
	studentByEnrollment := make(map[string]string, len(enrollments))
	for _, enr := range enrollments {
		if enr.Status != enrollmentStatusActive {
			continue
		}
		activeIDs = append(activeIDs, enr.ID)
		studentByEnrollment[enr.ID] = enr.StudentID
	}

	grades, err := s.entries.GetForKeys(ctx, activeIDs, asg.SubjectID, termID)
	if err != nil {
		return nil, err
	}

	rows := make([]RosterRow, 0, len(activeIDs))
	for _, enrollmentID := range activeIDs {
		row := RosterRow{EnrollmentID: enrollmentID, StudentID: studentByEnrollment[enrollmentID]}
		if entry, ok := grades[enrollmentID]; ok {
			row.Value = entry.Value()
			row.HasValue = true
		}
		rows = append(rows, row)
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].EnrollmentID < rows[j].EnrollmentID })
	return rows, nil
}

// ListGradeEntries returns the entries for an enrollment in a term (passthrough
// to the repository).
func (s *GradesService) ListGradeEntries(ctx context.Context, enrollmentID, termID string) ([]domain.GradeEntry, error) {
	return s.entries.ListGradeEntries(ctx, enrollmentID, termID)
}

// requireAssignment loads an assignment, mapping a NotFound to KindInvalid (a
// bad client reference), and propagating any other error.
func (s *GradesService) requireAssignment(ctx context.Context, assignmentID string) (ports.AssignmentRef, error) {
	asg, err := s.assignments.ByID(ctx, assignmentID)
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return ports.AssignmentRef{}, glerrors.New(glerrors.KindInvalid, "assignment not found")
		}
		return ports.AssignmentRef{}, err
	}
	return asg, nil
}

// requireTermOfAssignment loads a term and asserts it belongs to the
// assignment's cycle AND to its group's level — a term is per cycle and per
// level, so a Secundaria term must never grade a Primaria group. A missing term
// and either mismatch are KindInvalid.
func (s *GradesService) requireTermOfAssignment(ctx context.Context, termID string, asg ports.AssignmentRef) error {
	term, err := s.assignments.TermByID(ctx, termID)
	if err != nil {
		if glerrors.KindOf(err) == glerrors.KindNotFound {
			return glerrors.New(glerrors.KindInvalid, "term not found")
		}
		return err
	}
	if term.CycleID != asg.CycleID {
		return glerrors.New(glerrors.KindInvalid, "term does not belong to the assignment's cycle")
	}
	levelID, err := s.catalog.LevelIDOfGroup(ctx, asg.GroupID)
	if err != nil {
		return err
	}
	if term.LevelID != levelID {
		return glerrors.New(glerrors.KindInvalid, "El periodo no corresponde al nivel del grupo")
	}
	return nil
}
