package grpcadapter

import (
	"context"
	"errors"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/pkg/levelarchive"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// Compile-time proof the adapter implements the generated Connect handler.
var _ configv1connect.ConfigServiceHandler = (*ConfigHandler)(nil)

// ConfigHandler adapts the config use cases to the generated
// garcialorca.config.v1 Connect service. It only translates: proto messages ↔
// use-case calls, proto enums ↔ domain enums, timestamps ↔ time.Time, and
// domain error Kinds ↔ Connect codes. All logic lives in usecase/domain.
type ConfigHandler struct {
	catalog *usecase.CatalogService
	health  *usecase.HealthService
	trash   *usecase.LevelDeletionService
	periods *usecase.EvaluationPeriodService
	// schedule is the timetable (T16); see WithSchedule.
	schedule ScheduleService
	// placements is the legacy import's catalog (ADR 0033); see WithPlacements.
	placements PlacementService
}

// WithLevelDeletion wires the level trash (ADR 0021). The composition root
// always wires it.
func (h *ConfigHandler) WithLevelDeletion(trash *usecase.LevelDeletionService) *ConfigHandler {
	h.trash = trash
	return h
}

// NewConfigHandler wires the handler to the catalog + health use cases.
func NewConfigHandler(catalog *usecase.CatalogService, health *usecase.HealthService) *ConfigHandler {
	return &ConfigHandler{catalog: catalog, health: health}
}

// --- levels ----------------------------------------------------------------

func (h *ConfigHandler) CreateLevel(ctx context.Context, req *connect.Request[configv1.CreateLevelRequest]) (*connect.Response[configv1.CreateLevelResponse], error) {
	gran, err := toDomainGranularity(req.Msg.GetEnrollmentGranularity())
	if err != nil {
		return nil, connectErr(err)
	}
	l, err := h.catalog.CreateLevel(ctx, req.Msg.GetName(), req.Msg.GetSepCode(), gran)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateLevelResponse{Level: toProtoLevel(l)}), nil
}

func (h *ConfigHandler) GetLevel(ctx context.Context, req *connect.Request[configv1.GetLevelRequest]) (*connect.Response[configv1.GetLevelResponse], error) {
	l, err := h.catalog.GetLevel(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GetLevelResponse{Level: toProtoLevel(l)}), nil
}

func (h *ConfigHandler) ListLevels(ctx context.Context, _ *connect.Request[configv1.ListLevelsRequest]) (*connect.Response[configv1.ListLevelsResponse], error) {
	levels, err := h.catalog.ListLevels(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.Level, 0, len(levels))
	for _, l := range levels {
		out = append(out, toProtoLevel(l))
	}
	return connect.NewResponse(&configv1.ListLevelsResponse{Levels: out}), nil
}

// SetLevelAttendanceMode changes how a level takes attendance (daily or per
// class) and returns the updated level. An unspecified or unknown mode is
// InvalidArgument; an unknown level is NotFound.
func (h *ConfigHandler) SetLevelAttendanceMode(ctx context.Context, req *connect.Request[configv1.SetLevelAttendanceModeRequest]) (*connect.Response[configv1.SetLevelAttendanceModeResponse], error) {
	l, err := h.catalog.SetLevelAttendanceMode(ctx, req.Msg.GetId(), toDomainAttendanceMode(req.Msg.GetAttendanceMode()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.SetLevelAttendanceModeResponse{Level: toProtoLevel(l)}), nil
}

// SetLevelGradingScale replaces a level's numeric grading scale and returns the
// updated level. A missing or inconsistent scale is InvalidArgument; an unknown
// level is NotFound.
func (h *ConfigHandler) SetLevelGradingScale(ctx context.Context, req *connect.Request[configv1.SetLevelGradingScaleRequest]) (*connect.Response[configv1.SetLevelGradingScaleResponse], error) {
	g := req.Msg.GetGrading()
	if g == nil {
		return nil, connectErr(glerrors.New(glerrors.KindInvalid, "grading scale is required"))
	}
	scale, err := domain.NewGradingScale(g.GetEnabled(), int(g.GetMinHundredths()), int(g.GetMaxHundredths()), int(g.GetPassingHundredths()), int(g.GetDecimals()))
	if err != nil {
		return nil, connectErr(err)
	}
	l, err := h.catalog.SetLevelGradingScale(ctx, req.Msg.GetId(), scale)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.SetLevelGradingScaleResponse{Level: toProtoLevel(l)}), nil
}

// UpdateLevel changes a level's attendance mode and/or grading scale in one
// transaction. Every present field is validated before anything is written:
// an unspecified mode or an inconsistent scale is InvalidArgument and changes
// nothing; a request with neither field is InvalidArgument; an unknown level is
// NotFound.
func (h *ConfigHandler) UpdateLevel(ctx context.Context, req *connect.Request[configv1.UpdateLevelRequest]) (*connect.Response[configv1.UpdateLevelResponse], error) {
	var patch ports.LevelPatch
	if req.Msg.AttendanceMode != nil {
		mode := toDomainAttendanceMode(req.Msg.GetAttendanceMode())
		patch.AttendanceMode = &mode
	}
	if g := req.Msg.GetGrading(); g != nil {
		scale, err := domain.NewGradingScale(g.GetEnabled(), int(g.GetMinHundredths()), int(g.GetMaxHundredths()), int(g.GetPassingHundredths()), int(g.GetDecimals()))
		if err != nil {
			return nil, connectErr(err)
		}
		patch.Grading = &scale
	}
	l, err := h.catalog.UpdateLevel(ctx, req.Msg.GetId(), patch)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.UpdateLevelResponse{Level: toProtoLevel(l)}), nil
}

// --- cycles ----------------------------------------------------------------

func (h *ConfigHandler) CreateCycle(ctx context.Context, req *connect.Request[configv1.CreateCycleRequest]) (*connect.Response[configv1.CreateCycleResponse], error) {
	status, err := toDomainCycleStatus(req.Msg.GetStatus())
	if err != nil {
		return nil, connectErr(err)
	}
	c, err := h.catalog.CreateCycle(ctx, req.Msg.GetName(), status, asTime(req.Msg.GetStartsOn()), asTime(req.Msg.GetEndsOn()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateCycleResponse{Cycle: toProtoCycle(c)}), nil
}

func (h *ConfigHandler) GetCycle(ctx context.Context, req *connect.Request[configv1.GetCycleRequest]) (*connect.Response[configv1.GetCycleResponse], error) {
	c, err := h.catalog.GetCycle(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GetCycleResponse{Cycle: toProtoCycle(c)}), nil
}

func (h *ConfigHandler) ListCycles(ctx context.Context, _ *connect.Request[configv1.ListCyclesRequest]) (*connect.Response[configv1.ListCyclesResponse], error) {
	cycles, err := h.catalog.ListCycles(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.Cycle, 0, len(cycles))
	for _, c := range cycles {
		out = append(out, toProtoCycle(c))
	}
	return connect.NewResponse(&configv1.ListCyclesResponse{Cycles: out}), nil
}

// --- grade levels ----------------------------------------------------------

func (h *ConfigHandler) CreateGradeLevel(ctx context.Context, req *connect.Request[configv1.CreateGradeLevelRequest]) (*connect.Response[configv1.CreateGradeLevelResponse], error) {
	g, err := h.catalog.CreateGradeLevel(ctx, req.Msg.GetLevelId(), req.Msg.GetName(), int(req.Msg.GetOrdinal()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateGradeLevelResponse{GradeLevel: toProtoGradeLevel(g)}), nil
}

func (h *ConfigHandler) GetGradeLevel(ctx context.Context, req *connect.Request[configv1.GetGradeLevelRequest]) (*connect.Response[configv1.GetGradeLevelResponse], error) {
	g, err := h.catalog.GetGradeLevel(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GetGradeLevelResponse{GradeLevel: toProtoGradeLevel(g)}), nil
}

func (h *ConfigHandler) ListGradeLevels(ctx context.Context, req *connect.Request[configv1.ListGradeLevelsRequest]) (*connect.Response[configv1.ListGradeLevelsResponse], error) {
	grades, err := h.catalog.ListGradeLevels(ctx, req.Msg.GetLevelId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.GradeLevel, 0, len(grades))
	for _, g := range grades {
		out = append(out, toProtoGradeLevel(g))
	}
	return connect.NewResponse(&configv1.ListGradeLevelsResponse{GradeLevels: out}), nil
}

// --- subjects --------------------------------------------------------------

func (h *ConfigHandler) CreateSubject(ctx context.Context, req *connect.Request[configv1.CreateSubjectRequest]) (*connect.Response[configv1.CreateSubjectResponse], error) {
	s, err := h.catalog.CreateSubject(ctx, req.Msg.GetGradeLevelId(), req.Msg.GetName())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateSubjectResponse{Subject: toProtoSubject(s)}), nil
}

func (h *ConfigHandler) GetSubject(ctx context.Context, req *connect.Request[configv1.GetSubjectRequest]) (*connect.Response[configv1.GetSubjectResponse], error) {
	s, err := h.catalog.GetSubject(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GetSubjectResponse{Subject: toProtoSubject(s)}), nil
}

func (h *ConfigHandler) ListSubjects(ctx context.Context, req *connect.Request[configv1.ListSubjectsRequest]) (*connect.Response[configv1.ListSubjectsResponse], error) {
	subjects, err := h.catalog.ListSubjects(ctx, req.Msg.GetGradeLevelId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.Subject, 0, len(subjects))
	for _, s := range subjects {
		out = append(out, toProtoSubject(s))
	}
	return connect.NewResponse(&configv1.ListSubjectsResponse{Subjects: out}), nil
}

// RenameSubject renames a subject in place (same id, same grade).
func (h *ConfigHandler) RenameSubject(ctx context.Context, req *connect.Request[configv1.RenameSubjectRequest]) (*connect.Response[configv1.RenameSubjectResponse], error) {
	s, err := h.catalog.RenameSubject(ctx, req.Msg.GetId(), req.Msg.GetName())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.RenameSubjectResponse{Subject: toProtoSubject(s)}), nil
}

// DeleteSubject removes a subject no teacher is assigned to.
func (h *ConfigHandler) DeleteSubject(ctx context.Context, req *connect.Request[configv1.DeleteSubjectRequest]) (*connect.Response[configv1.DeleteSubjectResponse], error) {
	if err := h.catalog.DeleteSubject(ctx, req.Msg.GetId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.DeleteSubjectResponse{}), nil
}

// --- terms -----------------------------------------------------------------

// SetTermMonths replaces the calendar months a term covers.
func (h *ConfigHandler) SetTermMonths(ctx context.Context, req *connect.Request[configv1.SetTermMonthsRequest]) (*connect.Response[configv1.SetTermMonthsResponse], error) {
	t, err := h.catalog.SetTermMonths(ctx, req.Msg.GetTermId(), req.Msg.GetMonths())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.SetTermMonthsResponse{Term: toProtoTerm(t)}), nil
}

// CopyTermMonths mirrors one level's term months onto other levels of a cycle.
func (h *ConfigHandler) CopyTermMonths(ctx context.Context, req *connect.Request[configv1.CopyTermMonthsRequest]) (*connect.Response[configv1.CopyTermMonthsResponse], error) {
	terms, err := h.catalog.CopyTermMonths(ctx, req.Msg.GetCycleId(), req.Msg.GetFromLevelId(), req.Msg.GetToLevelIds())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.Term, 0, len(terms))
	for _, t := range terms {
		out = append(out, toProtoTerm(t))
	}
	return connect.NewResponse(&configv1.CopyTermMonthsResponse{Terms: out}), nil
}

func (h *ConfigHandler) CreateTerm(ctx context.Context, req *connect.Request[configv1.CreateTermRequest]) (*connect.Response[configv1.CreateTermResponse], error) {
	t, err := h.catalog.CreateTerm(ctx, req.Msg.GetCycleId(), req.Msg.GetLevelId(), req.Msg.GetName(), int(req.Msg.GetOrdinal()), asTime(req.Msg.GetStartsOn()), asTime(req.Msg.GetEndsOn()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateTermResponse{Term: toProtoTerm(t)}), nil
}

func (h *ConfigHandler) GetTerm(ctx context.Context, req *connect.Request[configv1.GetTermRequest]) (*connect.Response[configv1.GetTermResponse], error) {
	t, err := h.catalog.GetTerm(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GetTermResponse{Term: toProtoTerm(t)}), nil
}

func (h *ConfigHandler) ListTerms(ctx context.Context, req *connect.Request[configv1.ListTermsRequest]) (*connect.Response[configv1.ListTermsResponse], error) {
	terms, err := h.catalog.ListTerms(ctx, req.Msg.GetCycleId(), req.Msg.GetLevelId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.Term, 0, len(terms))
	for _, t := range terms {
		out = append(out, toProtoTerm(t))
	}
	return connect.NewResponse(&configv1.ListTermsResponse{Terms: out}), nil
}

// --- groups ----------------------------------------------------------------

func (h *ConfigHandler) CreateGroup(ctx context.Context, req *connect.Request[configv1.CreateGroupRequest]) (*connect.Response[configv1.CreateGroupResponse], error) {
	g, err := h.catalog.CreateGroup(ctx, req.Msg.GetCycleId(), req.Msg.GetGradeLevelId(), req.Msg.GetName(), int(req.Msg.GetCapacity()))
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateGroupResponse{Group: toProtoGroup(g)}), nil
}

func (h *ConfigHandler) GetGroup(ctx context.Context, req *connect.Request[configv1.GetGroupRequest]) (*connect.Response[configv1.GetGroupResponse], error) {
	g, err := h.catalog.GetGroup(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GetGroupResponse{Group: toProtoGroup(g)}), nil
}

func (h *ConfigHandler) ListGroups(ctx context.Context, req *connect.Request[configv1.ListGroupsRequest]) (*connect.Response[configv1.ListGroupsResponse], error) {
	groups, err := h.catalog.ListGroups(ctx, req.Msg.GetCycleId(), req.Msg.GetGradeLevelId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.Group, 0, len(groups))
	for _, g := range groups {
		out = append(out, toProtoGroup(g))
	}
	return connect.NewResponse(&configv1.ListGroupsResponse{Groups: out}), nil
}

// --- teachers --------------------------------------------------------------

func (h *ConfigHandler) CreateTeacher(ctx context.Context, req *connect.Request[configv1.CreateTeacherRequest]) (*connect.Response[configv1.CreateTeacherResponse], error) {
	te, err := h.catalog.CreateTeacher(ctx, req.Msg.GetFirstName(), req.Msg.GetLastName(), req.Msg.GetEmail(), req.Msg.GetUserId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateTeacherResponse{Teacher: toProtoTeacher(te)}), nil
}

func (h *ConfigHandler) GetTeacher(ctx context.Context, req *connect.Request[configv1.GetTeacherRequest]) (*connect.Response[configv1.GetTeacherResponse], error) {
	te, err := h.catalog.GetTeacher(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GetTeacherResponse{Teacher: toProtoTeacher(te)}), nil
}

func (h *ConfigHandler) ListTeachers(ctx context.Context, _ *connect.Request[configv1.ListTeachersRequest]) (*connect.Response[configv1.ListTeachersResponse], error) {
	teachers, err := h.catalog.ListTeachers(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.Teacher, 0, len(teachers))
	for _, te := range teachers {
		out = append(out, toProtoTeacher(te))
	}
	return connect.NewResponse(&configv1.ListTeachersResponse{Teachers: out}), nil
}

func (h *ConfigHandler) SetTeacherUser(ctx context.Context, req *connect.Request[configv1.SetTeacherUserRequest]) (*connect.Response[configv1.SetTeacherUserResponse], error) {
	te, err := h.catalog.SetTeacherUser(ctx, req.Msg.GetTeacherId(), req.Msg.GetUserId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.SetTeacherUserResponse{Teacher: toProtoTeacher(te)}), nil
}

// --- teaching assignments (the knot) ---------------------------------------

func (h *ConfigHandler) CreateTeachingAssignment(ctx context.Context, req *connect.Request[configv1.CreateTeachingAssignmentRequest]) (*connect.Response[configv1.CreateTeachingAssignmentResponse], error) {
	// The request carries NO cycle_id: the use case derives it from the group.
	a, err := h.catalog.CreateTeachingAssignment(ctx, req.Msg.GetGroupId(), req.Msg.GetSubjectId(), req.Msg.GetTeacherId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateTeachingAssignmentResponse{TeachingAssignment: toProtoTeachingAssignment(a)}), nil
}

func (h *ConfigHandler) GetTeachingAssignment(ctx context.Context, req *connect.Request[configv1.GetTeachingAssignmentRequest]) (*connect.Response[configv1.GetTeachingAssignmentResponse], error) {
	a, err := h.catalog.GetTeachingAssignment(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.GetTeachingAssignmentResponse{TeachingAssignment: toProtoTeachingAssignment(a)}), nil
}

func (h *ConfigHandler) ListTeachingAssignments(ctx context.Context, req *connect.Request[configv1.ListTeachingAssignmentsRequest]) (*connect.Response[configv1.ListTeachingAssignmentsResponse], error) {
	assignments, err := h.catalog.ListTeachingAssignments(ctx, req.Msg.GetCycleId(), req.Msg.GetGroupId(), req.Msg.GetTeacherId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.TeachingAssignment, 0, len(assignments))
	for _, a := range assignments {
		out = append(out, toProtoTeachingAssignment(a))
	}
	return connect.NewResponse(&configv1.ListTeachingAssignmentsResponse{TeachingAssignments: out}), nil
}

// --- entity mapping (domain → proto) ---------------------------------------

func toProtoLevel(l domain.Level) *configv1.Level {
	return &configv1.Level{
		Id:                    l.ID(),
		TenantId:              l.TenantID(),
		Name:                  l.Name(),
		SepCode:               l.SEPCode(),
		EnrollmentGranularity: toProtoGranularity(l.Granularity()),
		AttendanceMode:        toProtoAttendanceMode(l.AttendanceMode()),
		Grading:               toProtoGradingScale(l.Grading()),
		Status:                toProtoLevelStatus(l.Deleting()),
	}
}

// toProtoLevelStatus names the level's lifecycle on the wire (ADR 0021).
func toProtoLevelStatus(deleting bool) configv1.LevelStatus {
	if deleting {
		return configv1.LevelStatus_LEVEL_STATUS_DELETING
	}
	return configv1.LevelStatus_LEVEL_STATUS_ACTIVE
}

// toProtoGradingScale maps the domain scale to the wire message (hundredths).
func toProtoGradingScale(g domain.GradingScale) *configv1.GradingScale {
	return &configv1.GradingScale{
		Enabled:           g.Enabled(),
		MinHundredths:     int32(g.MinHundredths()),
		MaxHundredths:     int32(g.MaxHundredths()),
		Decimals:          int32(g.Decimals()),
		PassingHundredths: int32(g.PassingHundredths()),
	}
}

func toProtoCycle(c domain.Cycle) *configv1.Cycle {
	return &configv1.Cycle{
		Id:       c.ID(),
		TenantId: c.TenantID(),
		Name:     c.Name(),
		Status:   toProtoCycleStatus(c.Status()),
		StartsOn: timestamppb.New(c.StartsOn()),
		EndsOn:   timestamppb.New(c.EndsOn()),
	}
}

func toProtoGradeLevel(g domain.GradeLevel) *configv1.GradeLevel {
	return &configv1.GradeLevel{
		Id:       g.ID(),
		TenantId: g.TenantID(),
		LevelId:  g.LevelID(),
		Name:     g.Name(),
		Ordinal:  int32(g.Ordinal()),
	}
}

func toProtoSubject(s domain.Subject) *configv1.Subject {
	return &configv1.Subject{
		Id:           s.ID(),
		TenantId:     s.TenantID(),
		GradeLevelId: s.GradeLevelID(),
		Name:         s.Name(),
	}
}

func toProtoTerm(t domain.Term) *configv1.Term {
	return &configv1.Term{
		Id:       t.ID(),
		TenantId: t.TenantID(),
		CycleId:  t.CycleID(),
		LevelId:  t.LevelID(),
		Name:     t.Name(),
		Ordinal:  int32(t.Ordinal()),
		StartsOn: timestamppb.New(t.StartsOn()),
		EndsOn:   timestamppb.New(t.EndsOn()),
		Months:   formatMonths(t.Months()),
	}
}

// formatMonths renders a term's months as "YYYY-MM", keeping their order.
func formatMonths(months []time.Time) []string {
	out := make([]string, 0, len(months))
	for _, m := range months {
		out = append(out, domain.FormatMonth(m))
	}
	return out
}

func toProtoGroup(g domain.Group) *configv1.Group {
	return &configv1.Group{
		Id:           g.ID(),
		TenantId:     g.TenantID(),
		CycleId:      g.CycleID(),
		GradeLevelId: g.GradeLevelID(),
		Name:         g.Name(),
		Capacity:     int32(g.Capacity()),
	}
}

func toProtoTeacher(t domain.Teacher) *configv1.Teacher {
	return &configv1.Teacher{
		Id:        t.ID(),
		TenantId:  t.TenantID(),
		FirstName: t.FirstName(),
		LastName:  t.LastName(),
		Email:     t.Email(),
		UserId:    t.UserID(), // "" when the teacher has no IAM link yet.
	}
}

func toProtoTeachingAssignment(a domain.TeachingAssignment) *configv1.TeachingAssignment {
	return &configv1.TeachingAssignment{
		Id:        a.ID(),
		TenantId:  a.TenantID(),
		CycleId:   a.CycleID(), // server-derived from the group.
		GroupId:   a.GroupID(),
		SubjectId: a.SubjectID(),
		TeacherId: a.TeacherID(),
	}
}

// --- enum mapping ----------------------------------------------------------

// toDomainGranularity maps a proto EnrollmentGranularity to the domain enum,
// rejecting UNSPECIFIED (and any unknown value) as KindInvalid so the boundary
// never forwards an unset enum inward.
func toDomainGranularity(g configv1.EnrollmentGranularity) (domain.EnrollmentGranularity, error) {
	switch g {
	case configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_CYCLE:
		return domain.GranularityCycle, nil
	case configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_TERM:
		return domain.GranularityTerm, nil
	default:
		return domain.GranularityUnspecified, glerrors.New(glerrors.KindInvalid, "enrollment_granularity is required")
	}
}

// toProtoGranularity maps the domain granularity to its proto enum.
func toProtoGranularity(g domain.EnrollmentGranularity) configv1.EnrollmentGranularity {
	switch g {
	case domain.GranularityCycle:
		return configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_CYCLE
	case domain.GranularityTerm:
		return configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_TERM
	default:
		return configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_UNSPECIFIED
	}
}

// toProtoAttendanceMode maps the domain attendance mode to the wire enum.
func toProtoAttendanceMode(m domain.AttendanceMode) configv1.AttendanceMode {
	switch m {
	case domain.AttendanceDaily:
		return configv1.AttendanceMode_ATTENDANCE_MODE_DAILY
	case domain.AttendancePerClass:
		return configv1.AttendanceMode_ATTENDANCE_MODE_PER_CLASS
	default:
		return configv1.AttendanceMode_ATTENDANCE_MODE_UNSPECIFIED
	}
}

// toDomainAttendanceMode maps the wire enum to the domain mode. UNSPECIFIED and
// any unknown value map to AttendanceModeUnspecified, which the use case rejects
// as invalid.
func toDomainAttendanceMode(m configv1.AttendanceMode) domain.AttendanceMode {
	switch m {
	case configv1.AttendanceMode_ATTENDANCE_MODE_DAILY:
		return domain.AttendanceDaily
	case configv1.AttendanceMode_ATTENDANCE_MODE_PER_CLASS:
		return domain.AttendancePerClass
	default:
		return domain.AttendanceModeUnspecified
	}
}

// toDomainCycleStatus maps a proto CycleStatus to the domain enum. UNSPECIFIED
// passes through as CycleStatusUnspecified — the use case defaults it to draft
// (matching the proto contract and the DDL DEFAULT 'draft'). Genuinely unknown /
// out-of-range enum values are rejected as KindInvalid so the boundary never
// forwards a garbage enum inward.
func toDomainCycleStatus(s configv1.CycleStatus) (domain.CycleStatus, error) {
	switch s {
	case configv1.CycleStatus_CYCLE_STATUS_UNSPECIFIED:
		return domain.CycleStatusUnspecified, nil
	case configv1.CycleStatus_CYCLE_STATUS_DRAFT:
		return domain.CycleStatusDraft, nil
	case configv1.CycleStatus_CYCLE_STATUS_ACTIVE:
		return domain.CycleStatusActive, nil
	case configv1.CycleStatus_CYCLE_STATUS_CLOSED:
		return domain.CycleStatusClosed, nil
	default:
		return domain.CycleStatusUnspecified, glerrors.New(glerrors.KindInvalid, "status is invalid")
	}
}

// toProtoCycleStatus maps the domain cycle status to its proto enum.
func toProtoCycleStatus(s domain.CycleStatus) configv1.CycleStatus {
	switch s {
	case domain.CycleStatusDraft:
		return configv1.CycleStatus_CYCLE_STATUS_DRAFT
	case domain.CycleStatusActive:
		return configv1.CycleStatus_CYCLE_STATUS_ACTIVE
	case domain.CycleStatusClosed:
		return configv1.CycleStatus_CYCLE_STATUS_CLOSED
	default:
		return configv1.CycleStatus_CYCLE_STATUS_UNSPECIFIED
	}
}

// asTime converts a proto timestamp to time.Time, treating an unset (nil)
// timestamp as the zero time (so callers never deref a nil timestamp).
func asTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

// connectErr maps a domain error Kind to the Connect status code. Internal
// failures return a generic message (no internal detail leaks to the client);
// the other Kinds carry already-generic domain messages.
// Every code goes through glerrors.SafeMessage, not just the internal one. The
// kind decides the STATUS; it does not decide whether the wrapped cause is the
// caller's business, and it never is. Error() concatenates the cause, so handing
// the error straight to connect.NewError shipped Postgres constraint names and
// SQLSTATEs — and on some constraints the conflicting value — to whoever called.
func connectErr(err error) *connect.Error {
	ce := connectErrForKind(err)
	if code := domain.RefusalCodeOf(err); code != "" {
		ce.Meta().Set(ErrorCodeMeta, string(code))
	} else if code := levelarchive.CodeOf(err); code != "" {
		ce.Meta().Set(ErrorCodeMeta, code)
	}
	return ce
}

// ErrorCodeMeta is the error-metadata key a refusal's machine code rides in; the
// gateway forwards it as the X-Error-Code response header.
const ErrorCodeMeta = "X-Error-Code"

func connectErrForKind(err error) *connect.Error {
	safe := errors.New(glerrors.SafeMessage(err))

	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, safe)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, safe)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, safe)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, safe)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, safe)
	default:
		return connect.NewError(connect.CodeInternal, glerrors.New(glerrors.KindInternal, "internal error"))
	}
}

// ActivateCycle makes a cycle the school's live year.
func (h *ConfigHandler) ActivateCycle(ctx context.Context, req *connect.Request[configv1.ActivateCycleRequest]) (*connect.Response[configv1.ActivateCycleResponse], error) {
	cycle, err := h.catalog.ActivateCycle(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.ActivateCycleResponse{Cycle: toProtoCycle(cycle)}), nil
}

// --- school calendar (M16) -------------------------------------------------

// dateLayout is how a calendar day travels on the wire. A plain YYYY-MM-DD, not
// a timestamp: a holiday is a calendar day everywhere in Mexico, and a timestamp
// would invite a timezone to move it a day either way.
const dateLayout = "2006-01-02"

func (h *ConfigHandler) CreateNonWorkingPeriod(ctx context.Context, req *connect.Request[configv1.CreateNonWorkingPeriodRequest]) (*connect.Response[configv1.CreateNonWorkingPeriodResponse], error) {
	from, err := parseDate(req.Msg.GetStartsOn())
	if err != nil {
		return nil, connectErr(err)
	}
	to, err := parseDate(req.Msg.GetEndsOn())
	if err != nil {
		return nil, connectErr(err)
	}
	p, err := h.catalog.CreateNonWorkingPeriod(ctx, req.Msg.GetCycleId(), req.Msg.GetLabel(), req.Msg.GetKind(), from, to)
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateNonWorkingPeriodResponse{Period: toProtoNonWorkingPeriod(p)}), nil
}

func (h *ConfigHandler) ListNonWorkingPeriods(ctx context.Context, req *connect.Request[configv1.ListNonWorkingPeriodsRequest]) (*connect.Response[configv1.ListNonWorkingPeriodsResponse], error) {
	periods, err := h.catalog.ListNonWorkingPeriods(ctx, req.Msg.GetCycleId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.NonWorkingPeriod, 0, len(periods))
	for _, p := range periods {
		out = append(out, toProtoNonWorkingPeriod(p))
	}
	return connect.NewResponse(&configv1.ListNonWorkingPeriodsResponse{Periods: out}), nil
}

func (h *ConfigHandler) DeleteNonWorkingPeriod(ctx context.Context, req *connect.Request[configv1.DeleteNonWorkingPeriodRequest]) (*connect.Response[configv1.DeleteNonWorkingPeriodResponse], error) {
	if err := h.catalog.DeleteNonWorkingPeriod(ctx, req.Msg.GetId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.DeleteNonWorkingPeriodResponse{}), nil
}

// parseDate reads a wire date. A malformed one is the caller's mistake, so it
// comes back KindInvalid rather than as a zero date that would silently mean
// year one.
func parseDate(s string) (time.Time, error) {
	d, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, glerrors.New(glerrors.KindInvalid, "date must be YYYY-MM-DD")
	}
	return d, nil
}

func toProtoNonWorkingPeriod(p domain.NonWorkingPeriod) *configv1.NonWorkingPeriod {
	return &configv1.NonWorkingPeriod{
		Id:       p.ID(),
		CycleId:  p.CycleID(),
		Label:    p.Label(),
		Kind:     p.Kind(),
		StartsOn: p.From().Format(dateLayout),
		EndsOn:   p.To().Format(dateLayout),
	}
}

// --- school agenda (M16) ---------------------------------------------------

func (h *ConfigHandler) CreateSchoolEvent(ctx context.Context, req *connect.Request[configv1.CreateSchoolEventRequest]) (*connect.Response[configv1.CreateSchoolEventResponse], error) {
	m := req.Msg
	from, err := parseDate(m.GetStartsOn())
	if err != nil {
		return nil, connectErr(err)
	}
	// A one-day event may send only the start; repeating the date is noise.
	to := from
	if raw := m.GetEndsOn(); raw != "" {
		to, err = parseDate(raw)
		if err != nil {
			return nil, connectErr(err)
		}
	}
	event, err := h.catalog.CreateSchoolEvent(ctx, usecase.SchoolEventInput{
		CycleID: m.GetCycleId(), Title: m.GetTitle(), Description: m.GetDescription(),
		From: from, To: to,
		Audience: domain.EventAudience{LevelID: m.GetLevelId(), GroupID: m.GetGroupId()},
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.CreateSchoolEventResponse{Event: toProtoSchoolEvent(event)}), nil
}

func (h *ConfigHandler) ListSchoolEvents(ctx context.Context, req *connect.Request[configv1.ListSchoolEventsRequest]) (*connect.Response[configv1.ListSchoolEventsResponse], error) {
	events, err := h.catalog.ListSchoolEvents(ctx, req.Msg.GetCycleId())
	if err != nil {
		return nil, connectErr(err)
	}
	out := make([]*configv1.SchoolEvent, 0, len(events))
	for _, e := range events {
		out = append(out, toProtoSchoolEvent(e))
	}
	return connect.NewResponse(&configv1.ListSchoolEventsResponse{Events: out}), nil
}

func (h *ConfigHandler) DeleteSchoolEvent(ctx context.Context, req *connect.Request[configv1.DeleteSchoolEventRequest]) (*connect.Response[configv1.DeleteSchoolEventResponse], error) {
	if err := h.catalog.DeleteSchoolEvent(ctx, req.Msg.GetId()); err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.DeleteSchoolEventResponse{}), nil
}

// --- the school's details (task 12) ----------------------------------------

func (h *ConfigHandler) GetSchoolProfile(ctx context.Context, _ *connect.Request[configv1.GetSchoolProfileRequest]) (*connect.Response[configv1.GetSchoolProfileResponse], error) {
	p, found, err := h.catalog.GetSchoolProfile(ctx)
	if err != nil {
		return nil, connectErr(err)
	}
	res := &configv1.GetSchoolProfileResponse{Found: found}
	if found {
		res.Profile = toProtoSchoolProfile(p)
	}
	return connect.NewResponse(res), nil
}

func (h *ConfigHandler) SetSchoolProfile(ctx context.Context, req *connect.Request[configv1.SetSchoolProfileRequest]) (*connect.Response[configv1.SetSchoolProfileResponse], error) {
	in := req.Msg.GetProfile()
	p, err := h.catalog.SetSchoolProfile(ctx, domain.SchoolProfileInput{
		Name: in.GetName(), LegalName: in.GetLegalName(), RFC: in.GetRfc(), CCT: in.GetCct(),
		Address: in.GetAddress(), Phone: in.GetPhone(), Legend: in.GetLegend(),
	})
	if err != nil {
		return nil, connectErr(err)
	}
	return connect.NewResponse(&configv1.SetSchoolProfileResponse{Profile: toProtoSchoolProfile(p)}), nil
}

func toProtoSchoolProfile(p domain.SchoolProfile) *configv1.SchoolProfile {
	return &configv1.SchoolProfile{
		Name: p.Name(), LegalName: p.LegalName(), Rfc: p.RFC(), Cct: p.CCT(),
		Address: p.Address(), Phone: p.Phone(), Legend: p.Legend(),
	}
}

func toProtoSchoolEvent(e domain.SchoolEvent) *configv1.SchoolEvent {
	return &configv1.SchoolEvent{
		Id:          e.ID(),
		CycleId:     e.CycleID(),
		Title:       e.Title(),
		Description: e.Description(),
		StartsOn:    e.From().Format(dateLayout),
		EndsOn:      e.To().Format(dateLayout),
		LevelId:     e.Audience().LevelID,
		GroupId:     e.Audience().GroupID,
	}
}
