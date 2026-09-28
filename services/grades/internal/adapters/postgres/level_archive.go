package postgres

import (
	"context"
	"time"

	"gorm.io/gorm"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/pkg/levelarchive/gormarchive"
	"github.com/garcia-lorca/services/grades/internal/ports"
)

// Compile-time proof the adapter satisfies the level-archive port.
var _ ports.LevelArchiveRepository = (*Repository)(nil)

// levelTrash is grades' half of the deletion archive (migration 0007).
var levelTrash = gormarchive.New("grades_deletion_archive")

// The kinds grades reports, one per table it archives.
const (
	kindReportCardLines = "reportCardLines"
	kindReportCards     = "reportCards"
	kindPeriodClosures  = "periodClosures"
	kindMonthlyGrades   = "monthlyGrades"
	kindGradeEntries    = "gradeEntries"
	kindGradesOutbox    = "gradesOutbox"
)

// reportCardsOfLevel is a report card reached by the deletion: of one of the
// level's enrollments, terms or groups, or issued for the level itself.
const reportCardsOfLevel = "enrollment_id IN ? OR term_id IN ? OR level_id = ? OR group_id IN ?"

// levelSteps is grades' deletion, children first: the lines go before the
// report cards their predicate reads.
func levelSteps(s levelarchive.Scope) []gormarchive.Step {
	rc := []any{s.EnrollmentIDs, s.TermIDs, s.LevelID, s.GroupIDs}
	return []gormarchive.Step{
		{Kind: kindReportCardLines, Table: "report_card_lines",
			Where: "report_card_id IN (SELECT id FROM report_cards WHERE " + reportCardsOfLevel + ")", Args: rc},
		{Kind: kindReportCards, Table: "report_cards", Where: reportCardsOfLevel, Args: rc},
		{Kind: kindPeriodClosures, Table: "period_closures", Where: "term_id IN ? OR level_id = ?",
			Args: []any{s.TermIDs, s.LevelID}},
		{Kind: kindMonthlyGrades, Table: "monthly_grades", Where: "enrollment_id IN ? OR subject_id IN ?",
			Args: []any{s.EnrollmentIDs, s.SubjectIDs}},
		{Kind: kindGradeEntries, Table: "grade_entries", Where: "enrollment_id IN ? OR term_id IN ? OR subject_id IN ?",
			Args: []any{s.EnrollmentIDs, s.TermIDs, s.SubjectIDs}},
		// The outbox payload is JSON text; the ids were canonicalized (lowercase)
		// at the boundary, the same spelling the events were written with.
		{Kind: kindGradesOutbox, Table: "grades_outbox",
			Where: "payload->>'enrollmentId' IN ? OR payload->>'termId' IN ?",
			Args:  []any{s.EnrollmentIDs, s.TermIDs}},
	}
}

// levelRestore is the way back, parents first. Only the lines have a parent
// in this service; every other conflict is a unique key taken meanwhile.
var levelRestore = []struct {
	kind    string
	table   string
	parents []gormarchive.Parent
}{
	{kindPeriodClosures, "period_closures", nil},
	{kindReportCards, "report_cards", nil},
	{kindReportCardLines, "report_card_lines", []gormarchive.Parent{{Column: "report_card_id", Table: "report_cards"}}},
	{kindMonthlyGrades, "monthly_grades", nil},
	{kindGradeEntries, "grade_entries", nil},
	{kindGradesOutbox, "grades_outbox", nil},
}

// restoreSkipReason is the one sentence a skipped grades row gets.
const restoreSkipReason = "ya existe otra captura o boleta para ese alumno y periodo"

// PreviewLevelDeletion counts what the deletion would archive, changing nothing.
func (r *Repository) PreviewLevelDeletion(ctx context.Context, scope levelarchive.Scope) (levelarchive.Counts, error) {
	counts := levelarchive.Counts{}
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		for _, s := range levelSteps(scope) {
			n, err := gormarchive.Count(tx, s)
			if err != nil {
				return glerrors.Wrap(glerrors.KindInternal, "counting the level's grades", err)
			}
			counts[s.Kind] = n
		}
		return nil
	})
	return counts, err
}

// ArchiveLevel moves the level's grades into the trash, in one transaction. A
// retried batch answers its recorded counts.
func (r *Repository) ArchiveLevel(ctx context.Context, batchID string, scope levelarchive.Scope) (levelarchive.Counts, error) {
	var counts levelarchive.Counts
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		counts, err = levelTrash.ArchiveBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Counts, error) {
			out := levelarchive.Counts{}
			for i, s := range levelSteps(scope) {
				n, err := levelTrash.Move(tx, batchID, i, s)
				if err != nil {
					return nil, glerrors.Wrap(glerrors.KindInternal, "archiving the level's grades", err)
				}
				out[s.Kind] = n
			}
			return out, nil
		})
		return err
	})
	return counts, err
}

// RestoreLevelBatch puts a batch back, parents first, in one transaction.
func (r *Repository) RestoreLevelBatch(ctx context.Context, batchID string) (levelarchive.Report, error) {
	var rep levelarchive.Report
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		rep, err = levelTrash.RestoreBatch(tx, batchID, func(tx *gorm.DB) (levelarchive.Report, error) {
			out := levelarchive.NewReport()
			for _, s := range levelRestore {
				restored, skipped, err := levelTrash.Reinsert(tx, batchID, s.table, s.parents...)
				if err != nil {
					return out, glerrors.Wrap(glerrors.KindInternal, "restoring the level's grades", err)
				}
				out.Record(s.kind, restored, skipped, restoreSkipReason)
			}
			return out, nil
		})
		return err
	})
	return rep, err
}

// PurgeLevelArchives drops the archives filed before cutoff.
func (r *Repository) PurgeLevelArchives(ctx context.Context, cutoff time.Time) (int64, error) {
	var n int64
	err := r.withTenant(ctx, func(tx *gorm.DB) error {
		var err error
		if n, err = levelTrash.Purge(tx, cutoff); err != nil {
			return glerrors.Wrap(glerrors.KindInternal, "purging the grades trash", err)
		}
		return nil
	})
	return n, err
}
