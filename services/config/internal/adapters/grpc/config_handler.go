package grpcadapter

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
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

// --- terms -----------------------------------------------------------------

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
	}
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
func connectErr(err error) *connect.Error {
	switch glerrors.KindOf(err) {
	case glerrors.KindInvalid:
		return connect.NewError(connect.CodeInvalidArgument, err)
	case glerrors.KindUnauthorized:
		return connect.NewError(connect.CodeUnauthenticated, err)
	case glerrors.KindForbidden:
		return connect.NewError(connect.CodePermissionDenied, err)
	case glerrors.KindNotFound:
		return connect.NewError(connect.CodeNotFound, err)
	case glerrors.KindConflict:
		return connect.NewError(connect.CodeAlreadyExists, err)
	default:
		return connect.NewError(connect.CodeInternal, glerrors.New(glerrors.KindInternal, "internal error"))
	}
}
