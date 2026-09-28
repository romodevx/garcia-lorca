package client

import (
	"context"
	"time"

	connect "connectrpc.com/connect"

	attendancev1 "github.com/garcia-lorca/gen/go/garcialorca/attendance/v1"
	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	gradesv1 "github.com/garcia-lorca/gen/go/garcialorca/grades/v1"
	iamv1 "github.com/garcia-lorca/gen/go/garcialorca/iam/v1"
	studentsv1 "github.com/garcia-lorca/gen/go/garcialorca/students/v1"
	uniformsv1 "github.com/garcia-lorca/gen/go/garcialorca/uniforms/v1"
)

// --- level deletion (ADR 0021) -------------------------------------------------
//
// Every data service speaks the same four RPCs over its own generated types. The
// gateway's view is one set of plain types: a scope, counts by kind, and a
// restore report.

// LevelScope is everything one level deletion reaches.
type LevelScope struct {
	LevelID       string
	GradeLevelIDs []string
	GroupIDs      []string
	SubjectIDs    []string
	TermIDs       []string
	EnrollmentIDs []string
	StudentIDs    []string
}

// LevelCounts is rows per kind.
type LevelCounts map[string]int64

// Add merges other into c.
func (c LevelCounts) Add(other LevelCounts) {
	for k, v := range other {
		c[k] += v
	}
}

// LevelRestoreSkip is rows of one kind a restore could not put back.
type LevelRestoreSkip struct {
	Kind   string
	Count  int64
	Reason string
}

// LevelRestoreReport is what one service put back and what it skipped.
type LevelRestoreReport struct {
	Restored LevelCounts
	Skipped  []LevelRestoreSkip
}

// LevelStructure is config's preview: the level and its structure's ids.
type LevelStructure struct {
	LevelID       string
	LevelName     string
	GradeLevelIDs []string
	GroupIDs      []string
	SubjectIDs    []string
	TermIDs       []string
	Counts        LevelCounts
}

// StudentsLevelPreview is students' preview: counts plus the people's ids the
// other services need.
type StudentsLevelPreview struct {
	Counts        LevelCounts
	StudentIDs    []string
	EnrollmentIDs []string
}

// LevelDeletion is one batch in the trash.
type LevelDeletion struct {
	BatchID         string
	LevelID         string
	LevelName       string
	Counts          LevelCounts
	DeletedBy       string
	DeletedByName   string
	DeletedAt       time.Time
	RestorableUntil time.Time
	Status          string
}

// ArchiveLevelRecord is config's archive step input.
type ArchiveLevelRecord struct {
	BatchID       string
	LevelID       string
	DeletedBy     string
	DeletedByName string
	Summary       LevelCounts
}

type kindCount interface {
	GetKind() string
	GetCount() int64
}

type restoreSkip interface {
	kindCount
	GetReason() string
}

func countsOf[T kindCount](in []T) LevelCounts {
	out := LevelCounts{}
	for _, kc := range in {
		out[kc.GetKind()] += kc.GetCount()
	}
	return out
}

func reportOf[C kindCount, S restoreSkip](restored []C, skipped []S) LevelRestoreReport {
	rep := LevelRestoreReport{Restored: countsOf(restored), Skipped: make([]LevelRestoreSkip, 0, len(skipped))}
	for _, s := range skipped {
		rep.Skipped = append(rep.Skipped, LevelRestoreSkip{Kind: s.GetKind(), Count: s.GetCount(), Reason: s.GetReason()})
	}
	return rep
}

// --- config ---------------------------------------------------------------------

// PreviewLevelDeletion resolves the level's structure across every cycle.
func (c *ConfigClient) PreviewLevelDeletion(ctx context.Context, levelID string) (LevelStructure, error) {
	res, err := c.rpc.PreviewLevelDeletion(ctx, connect.NewRequest(&configv1.PreviewLevelDeletionRequest{LevelId: levelID}))
	if err != nil {
		return LevelStructure{}, err
	}
	m := res.Msg
	return LevelStructure{
		LevelID: m.GetLevelId(), LevelName: m.GetLevelName(), Counts: countsOf(m.GetCounts()),
		GradeLevelIDs: m.GetGradeLevelIds(), GroupIDs: m.GetGroupIds(), SubjectIDs: m.GetSubjectIds(), TermIDs: m.GetTermIds(),
	}, nil
}

// ArchiveLevel is the saga's last step: the structure into the trash and the
// batch into the ledger.
func (c *ConfigClient) ArchiveLevel(ctx context.Context, in ArchiveLevelRecord) (LevelCounts, LevelDeletion, error) {
	summary := make([]*configv1.LevelArchiveCount, 0, len(in.Summary))
	for k, v := range in.Summary {
		summary = append(summary, &configv1.LevelArchiveCount{Kind: k, Count: v})
	}
	res, err := c.rpc.ArchiveLevel(ctx, connect.NewRequest(&configv1.ArchiveLevelRequest{
		BatchId: in.BatchID, LevelId: in.LevelID, DeletedBy: in.DeletedBy, DeletedByName: in.DeletedByName, Summary: summary,
	}))
	if err != nil {
		return nil, LevelDeletion{}, err
	}
	return countsOf(res.Msg.GetCounts()), levelDeletionOf(res.Msg.GetDeletion()), nil
}

// RestoreLevelBatch puts the structure back.
func (c *ConfigClient) RestoreLevelBatch(ctx context.Context, batchID string) (LevelRestoreReport, error) {
	res, err := c.rpc.RestoreLevelBatch(ctx, connect.NewRequest(&configv1.RestoreLevelBatchRequest{BatchId: batchID}))
	if err != nil {
		return LevelRestoreReport{}, err
	}
	return reportOf(res.Msg.GetRestored(), res.Msg.GetSkipped()), nil
}

// MarkLevelDeletionRestored closes a batch every service restored.
func (c *ConfigClient) MarkLevelDeletionRestored(ctx context.Context, batchID string) (LevelDeletion, error) {
	res, err := c.rpc.MarkLevelDeletionRestored(ctx, connect.NewRequest(&configv1.MarkLevelDeletionRestoredRequest{BatchId: batchID}))
	if err != nil {
		return LevelDeletion{}, err
	}
	return levelDeletionOf(res.Msg.GetDeletion()), nil
}

// ListLevelDeletions is the trash.
func (c *ConfigClient) ListLevelDeletions(ctx context.Context) ([]LevelDeletion, error) {
	res, err := c.rpc.ListLevelDeletions(ctx, connect.NewRequest(&configv1.ListLevelDeletionsRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]LevelDeletion, 0, len(res.Msg.GetDeletions()))
	for _, d := range res.Msg.GetDeletions() {
		out = append(out, levelDeletionOf(d))
	}
	return out, nil
}

// GetLevelDeletion fetches one batch.
func (c *ConfigClient) GetLevelDeletion(ctx context.Context, batchID string) (LevelDeletion, error) {
	res, err := c.rpc.GetLevelDeletion(ctx, connect.NewRequest(&configv1.GetLevelDeletionRequest{BatchId: batchID}))
	if err != nil {
		return LevelDeletion{}, err
	}
	return levelDeletionOf(res.Msg.GetDeletion()), nil
}

func levelDeletionOf(d *configv1.LevelDeletion) LevelDeletion {
	if d == nil {
		return LevelDeletion{Counts: LevelCounts{}}
	}
	return LevelDeletion{
		BatchID: d.GetBatchId(), LevelID: d.GetLevelId(), LevelName: d.GetLevelName(), Counts: countsOf(d.GetCounts()),
		DeletedBy: d.GetDeletedBy(), DeletedByName: d.GetDeletedByName(),
		DeletedAt: d.GetDeletedAt().AsTime(), RestorableUntil: d.GetRestorableUntil().AsTime(), Status: d.GetStatus(),
	}
}

// --- students -------------------------------------------------------------------

func studentsScope(s LevelScope) *studentsv1.LevelScope {
	return &studentsv1.LevelScope{LevelId: s.LevelID, GradeLevelIds: s.GradeLevelIDs, GroupIds: s.GroupIDs,
		SubjectIds: s.SubjectIDs, TermIds: s.TermIDs, EnrollmentIds: s.EnrollmentIDs, StudentIds: s.StudentIDs}
}

// PreviewLevelDeletion counts the level's pupils and enrollments and returns
// their ids.
func (c *StudentsClient) PreviewLevelDeletion(ctx context.Context, scope LevelScope) (StudentsLevelPreview, error) {
	res, err := c.rpc.PreviewLevelDeletion(ctx, connect.NewRequest(&studentsv1.PreviewLevelDeletionRequest{Scope: studentsScope(scope)}))
	if err != nil {
		return StudentsLevelPreview{}, err
	}
	return StudentsLevelPreview{Counts: countsOf(res.Msg.GetCounts()), StudentIDs: res.Msg.GetStudentIds(), EnrollmentIDs: res.Msg.GetEnrollmentIds()}, nil
}

// ArchiveLevel archives the enrollments and clears the pupils' level.
func (c *StudentsClient) ArchiveLevel(ctx context.Context, batchID string, scope LevelScope) (LevelCounts, error) {
	res, err := c.rpc.ArchiveLevel(ctx, connect.NewRequest(&studentsv1.ArchiveLevelRequest{BatchId: batchID, Scope: studentsScope(scope)}))
	if err != nil {
		return nil, err
	}
	return countsOf(res.Msg.GetCounts()), nil
}

// RestoreLevelBatch puts the batch back.
func (c *StudentsClient) RestoreLevelBatch(ctx context.Context, batchID string) (LevelRestoreReport, error) {
	res, err := c.rpc.RestoreLevelBatch(ctx, connect.NewRequest(&studentsv1.RestoreLevelBatchRequest{BatchId: batchID}))
	if err != nil {
		return LevelRestoreReport{}, err
	}
	return reportOf(res.Msg.GetRestored(), res.Msg.GetSkipped()), nil
}

// --- attendance -----------------------------------------------------------------

func attendanceScope(s LevelScope) *attendancev1.LevelScope {
	return &attendancev1.LevelScope{LevelId: s.LevelID, GradeLevelIds: s.GradeLevelIDs, GroupIds: s.GroupIDs,
		SubjectIds: s.SubjectIDs, TermIds: s.TermIDs, EnrollmentIds: s.EnrollmentIDs, StudentIds: s.StudentIDs}
}

// PreviewLevelDeletion counts the level's attendance.
func (c *AttendanceClient) PreviewLevelDeletion(ctx context.Context, scope LevelScope) (LevelCounts, error) {
	res, err := c.rpc.PreviewLevelDeletion(ctx, connect.NewRequest(&attendancev1.PreviewLevelDeletionRequest{Scope: attendanceScope(scope)}))
	if err != nil {
		return nil, err
	}
	return countsOf(res.Msg.GetCounts()), nil
}

// ArchiveLevel archives the level's attendance.
func (c *AttendanceClient) ArchiveLevel(ctx context.Context, batchID string, scope LevelScope) (LevelCounts, error) {
	res, err := c.rpc.ArchiveLevel(ctx, connect.NewRequest(&attendancev1.ArchiveLevelRequest{BatchId: batchID, Scope: attendanceScope(scope)}))
	if err != nil {
		return nil, err
	}
	return countsOf(res.Msg.GetCounts()), nil
}

// RestoreLevelBatch puts the batch back.
func (c *AttendanceClient) RestoreLevelBatch(ctx context.Context, batchID string) (LevelRestoreReport, error) {
	res, err := c.rpc.RestoreLevelBatch(ctx, connect.NewRequest(&attendancev1.RestoreLevelBatchRequest{BatchId: batchID}))
	if err != nil {
		return LevelRestoreReport{}, err
	}
	return reportOf(res.Msg.GetRestored(), res.Msg.GetSkipped()), nil
}

// --- grades ---------------------------------------------------------------------

func gradesScope(s LevelScope) *gradesv1.LevelScope {
	return &gradesv1.LevelScope{LevelId: s.LevelID, GradeLevelIds: s.GradeLevelIDs, GroupIds: s.GroupIDs,
		SubjectIds: s.SubjectIDs, TermIds: s.TermIDs, EnrollmentIds: s.EnrollmentIDs, StudentIds: s.StudentIDs}
}

// PreviewLevelDeletion counts the level's grades.
func (c *GradesClient) PreviewLevelDeletion(ctx context.Context, scope LevelScope) (LevelCounts, error) {
	res, err := c.rpc.PreviewLevelDeletion(ctx, connect.NewRequest(&gradesv1.PreviewLevelDeletionRequest{Scope: gradesScope(scope)}))
	if err != nil {
		return nil, err
	}
	return countsOf(res.Msg.GetCounts()), nil
}

// ArchiveLevel archives the level's grades.
func (c *GradesClient) ArchiveLevel(ctx context.Context, batchID string, scope LevelScope) (LevelCounts, error) {
	res, err := c.rpc.ArchiveLevel(ctx, connect.NewRequest(&gradesv1.ArchiveLevelRequest{BatchId: batchID, Scope: gradesScope(scope)}))
	if err != nil {
		return nil, err
	}
	return countsOf(res.Msg.GetCounts()), nil
}

// RestoreLevelBatch puts the batch back.
func (c *GradesClient) RestoreLevelBatch(ctx context.Context, batchID string) (LevelRestoreReport, error) {
	res, err := c.rpc.RestoreLevelBatch(ctx, connect.NewRequest(&gradesv1.RestoreLevelBatchRequest{BatchId: batchID}))
	if err != nil {
		return LevelRestoreReport{}, err
	}
	return reportOf(res.Msg.GetRestored(), res.Msg.GetSkipped()), nil
}

// --- finance --------------------------------------------------------------------

func financeScope(s LevelScope) *financev1.LevelScope {
	return &financev1.LevelScope{LevelId: s.LevelID, GradeLevelIds: s.GradeLevelIDs, GroupIds: s.GroupIDs,
		SubjectIds: s.SubjectIDs, TermIds: s.TermIDs, EnrollmentIds: s.EnrollmentIDs, StudentIds: s.StudentIDs}
}

// PreviewLevelDeletion counts the level's tuition plans and the money kept.
func (c *FinanceClient) PreviewLevelDeletion(ctx context.Context, scope LevelScope) (LevelCounts, error) {
	res, err := c.rpc.PreviewLevelDeletion(ctx, connect.NewRequest(&financev1.PreviewLevelDeletionRequest{Scope: financeScope(scope)}))
	if err != nil {
		return nil, err
	}
	return countsOf(res.Msg.GetCounts()), nil
}

// ArchiveLevel archives the level's tuition plans (never money).
func (c *FinanceClient) ArchiveLevel(ctx context.Context, batchID string, scope LevelScope) (LevelCounts, error) {
	res, err := c.rpc.ArchiveLevel(ctx, connect.NewRequest(&financev1.ArchiveLevelRequest{BatchId: batchID, Scope: financeScope(scope)}))
	if err != nil {
		return nil, err
	}
	return countsOf(res.Msg.GetCounts()), nil
}

// RestoreLevelBatch puts the batch back.
func (c *FinanceClient) RestoreLevelBatch(ctx context.Context, batchID string) (LevelRestoreReport, error) {
	res, err := c.rpc.RestoreLevelBatch(ctx, connect.NewRequest(&financev1.RestoreLevelBatchRequest{BatchId: batchID}))
	if err != nil {
		return LevelRestoreReport{}, err
	}
	return reportOf(res.Msg.GetRestored(), res.Msg.GetSkipped()), nil
}

// --- uniforms -------------------------------------------------------------------

func uniformsScope(s LevelScope) *uniformsv1.LevelScope {
	return &uniformsv1.LevelScope{LevelId: s.LevelID, GradeLevelIds: s.GradeLevelIDs, GroupIds: s.GroupIDs,
		SubjectIds: s.SubjectIDs, TermIds: s.TermIDs, EnrollmentIds: s.EnrollmentIDs, StudentIds: s.StudentIDs}
}

// PreviewLevelDeletion counts the garments scoped to the level.
func (c *UniformsClient) PreviewLevelDeletion(ctx context.Context, scope LevelScope) (LevelCounts, error) {
	res, err := c.rpc.PreviewLevelDeletion(ctx, connect.NewRequest(&uniformsv1.PreviewLevelDeletionRequest{Scope: uniformsScope(scope)}))
	if err != nil {
		return nil, err
	}
	return countsOf(res.Msg.GetCounts()), nil
}

// ArchiveLevel leaves the level's garments unresolved (never deleted).
func (c *UniformsClient) ArchiveLevel(ctx context.Context, batchID string, scope LevelScope) (LevelCounts, error) {
	res, err := c.rpc.ArchiveLevel(ctx, connect.NewRequest(&uniformsv1.ArchiveLevelRequest{BatchId: batchID, Scope: uniformsScope(scope)}))
	if err != nil {
		return nil, err
	}
	return countsOf(res.Msg.GetCounts()), nil
}

// RestoreLevelBatch puts the batch back.
func (c *UniformsClient) RestoreLevelBatch(ctx context.Context, batchID string) (LevelRestoreReport, error) {
	res, err := c.rpc.RestoreLevelBatch(ctx, connect.NewRequest(&uniformsv1.RestoreLevelBatchRequest{BatchId: batchID}))
	if err != nil {
		return LevelRestoreReport{}, err
	}
	return reportOf(res.Msg.GetRestored(), res.Msg.GetSkipped()), nil
}

// --- iam ------------------------------------------------------------------------

// VerifyPassword re-checks the caller's password the way sign-in does. A wrong
// password is a Connect Unauthenticated error.
func (c *AuthClient) VerifyPassword(ctx context.Context, accessToken, password string) error {
	_, err := c.rpc.VerifyPassword(ctx, connect.NewRequest(&iamv1.VerifyPasswordRequest{AccessToken: accessToken, Password: password}))
	return err
}
