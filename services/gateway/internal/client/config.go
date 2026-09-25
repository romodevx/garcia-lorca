package client

import (
	"context"
	"net/http"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/config/v1/configv1connect"
)

// ConfigClient is the gateway's typed client for the config (academic catalog)
// service over the generated Connect client. Handlers depend on its small
// methods and the plain structs below, never on the generated proto types.
// Enums are flattened to canonical lowercase strings and timestamps to
// time.Time so the JSON layer stays framework-free.
type ConfigClient struct {
	rpc configv1connect.ConfigServiceClient
}

// NewConfigClient builds a ConfigClient targeting the config base URL over
// httpClient.
func NewConfigClient(baseURL string, httpClient connect.HTTPClient) *ConfigClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &ConfigClient{rpc: configv1connect.NewConfigServiceClient(httpClient, baseURL)}
}

// --- edge structs ---------------------------------------------------------

// CreateCycleInput is the edge's view of a new-cycle request. Status is a
// canonical lowercase name ("draft"/"active"/"closed"); an unknown value maps
// to the unspecified wire enum, which the config service rejects as invalid.
type CreateCycleInput struct {
	Name     string
	Status   string
	StartsOn time.Time
	EndsOn   time.Time
}

// CycleResult is the edge's view of a cycle.
type CycleResult struct {
	ID       string
	Name     string
	Status   string
	StartsOn time.Time
	EndsOn   time.Time
}

// CreateLevelInput is the edge's view of a new-level request. Granularity is a
// canonical lowercase name ("cycle"/"term").
type CreateLevelInput struct {
	Name        string
	SEPCode     string
	Granularity string
}

// LevelResult is the edge's view of a catalog level.
type LevelResult struct {
	ID          string
	Name        string
	SEPCode     string
	Granularity string
}

// CreateGradeLevelInput is the edge's view of a new grade-within-a-level.
type CreateGradeLevelInput struct {
	LevelID string
	Name    string
	Ordinal int
}

// GradeLevelResult is the edge's view of a grade_level.
type GradeLevelResult struct {
	ID      string
	LevelID string
	Name    string
	Ordinal int
}

// CreateSubjectInput is the edge's view of a new subject at a grade.
type CreateSubjectInput struct {
	GradeLevelID string
	Name         string
}

// SubjectResult is the edge's view of a subject.
type SubjectResult struct {
	ID           string
	GradeLevelID string
	Name         string
}

// CreateTermInput is the edge's view of a new period within a cycle for a level.
type CreateTermInput struct {
	CycleID  string
	LevelID  string
	Name     string
	Ordinal  int
	StartsOn time.Time
	EndsOn   time.Time
}

// TermResult is the edge's view of a term.
type TermResult struct {
	ID       string
	CycleID  string
	LevelID  string
	Name     string
	Ordinal  int
	StartsOn time.Time
	EndsOn   time.Time
}

// CreateGroupInput is the edge's view of a new class group within a cycle for a
// grade.
type CreateGroupInput struct {
	CycleID      string
	GradeLevelID string
	Name         string
	Capacity     int
}

// GroupResult is the edge's view of a group.
type GroupResult struct {
	ID           string
	CycleID      string
	GradeLevelID string
	Name         string
	Capacity     int
}

// Teacher is the edge's view of a catalog teacher (a person record, not a login
// account). UserID is reserved for a future IAM link and may be empty.
type Teacher struct {
	ID        string
	FirstName string
	LastName  string
	Email     string
	UserID    string
}

// TeachingAssignment is the edge's view of the knot binding a teacher to a
// (group, subject) within a cycle. CycleID is DERIVED server-side from the
// group at create time — it is never sent by the client.
type TeachingAssignment struct {
	ID        string
	CycleID   string
	GroupID   string
	SubjectID string
	TeacherID string
}

// --- cycles ---------------------------------------------------------------

// CreateCycle adds a school-year cycle via the config service.
func (c *ConfigClient) CreateCycle(ctx context.Context, in CreateCycleInput) (CycleResult, error) {
	res, err := c.rpc.CreateCycle(ctx, connect.NewRequest(&configv1.CreateCycleRequest{
		Name:     in.Name,
		Status:   cycleStatusFromName(in.Status),
		StartsOn: tsFromTime(in.StartsOn),
		EndsOn:   tsFromTime(in.EndsOn),
	}))
	if err != nil {
		return CycleResult{}, err
	}
	return toCycleResult(res.Msg.GetCycle()), nil
}

// ListCycles lists every cycle in the tenant.
func (c *ConfigClient) ListCycles(ctx context.Context) ([]CycleResult, error) {
	res, err := c.rpc.ListCycles(ctx, connect.NewRequest(&configv1.ListCyclesRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]CycleResult, 0, len(res.Msg.GetCycles()))
	for _, cyc := range res.Msg.GetCycles() {
		out = append(out, toCycleResult(cyc))
	}
	return out, nil
}

// ActivateCycle makes a cycle the school's live year, standing down whichever
// other was active.
func (c *ConfigClient) ActivateCycle(ctx context.Context, id string) (CycleResult, error) {
	res, err := c.rpc.ActivateCycle(ctx, connect.NewRequest(&configv1.ActivateCycleRequest{Id: id}))
	if err != nil {
		return CycleResult{}, err
	}
	return toCycleResult(res.Msg.GetCycle()), nil
}

// --- levels ---------------------------------------------------------------

// CreateLevel adds a catalog level via the config service.
func (c *ConfigClient) CreateLevel(ctx context.Context, in CreateLevelInput) (LevelResult, error) {
	res, err := c.rpc.CreateLevel(ctx, connect.NewRequest(&configv1.CreateLevelRequest{
		Name:                  in.Name,
		SepCode:               in.SEPCode,
		EnrollmentGranularity: granularityFromName(in.Granularity),
	}))
	if err != nil {
		return LevelResult{}, err
	}
	return toLevelResult(res.Msg.GetLevel()), nil
}

// ListLevels lists every level in the tenant catalog.
func (c *ConfigClient) ListLevels(ctx context.Context) ([]LevelResult, error) {
	res, err := c.rpc.ListLevels(ctx, connect.NewRequest(&configv1.ListLevelsRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]LevelResult, 0, len(res.Msg.GetLevels()))
	for _, l := range res.Msg.GetLevels() {
		out = append(out, toLevelResult(l))
	}
	return out, nil
}

// --- grade_levels ---------------------------------------------------------

// CreateGradeLevel adds a grade within a level.
func (c *ConfigClient) CreateGradeLevel(ctx context.Context, in CreateGradeLevelInput) (GradeLevelResult, error) {
	res, err := c.rpc.CreateGradeLevel(ctx, connect.NewRequest(&configv1.CreateGradeLevelRequest{
		LevelId: in.LevelID,
		Name:    in.Name,
		Ordinal: int32(in.Ordinal),
	}))
	if err != nil {
		return GradeLevelResult{}, err
	}
	return toGradeLevelResult(res.Msg.GetGradeLevel()), nil
}

// ListGradeLevels lists the grades of a level (scoped by levelID).
func (c *ConfigClient) ListGradeLevels(ctx context.Context, levelID string) ([]GradeLevelResult, error) {
	res, err := c.rpc.ListGradeLevels(ctx, connect.NewRequest(&configv1.ListGradeLevelsRequest{LevelId: levelID}))
	if err != nil {
		return nil, err
	}
	out := make([]GradeLevelResult, 0, len(res.Msg.GetGradeLevels()))
	for _, g := range res.Msg.GetGradeLevels() {
		out = append(out, toGradeLevelResult(g))
	}
	return out, nil
}

// --- subjects -------------------------------------------------------------

// CreateSubject adds a subject taught at a grade.
func (c *ConfigClient) CreateSubject(ctx context.Context, in CreateSubjectInput) (SubjectResult, error) {
	res, err := c.rpc.CreateSubject(ctx, connect.NewRequest(&configv1.CreateSubjectRequest{
		GradeLevelId: in.GradeLevelID,
		Name:         in.Name,
	}))
	if err != nil {
		return SubjectResult{}, err
	}
	return toSubjectResult(res.Msg.GetSubject()), nil
}

// ListSubjects lists the subjects of a grade (scoped by gradeLevelID).
func (c *ConfigClient) ListSubjects(ctx context.Context, gradeLevelID string) ([]SubjectResult, error) {
	res, err := c.rpc.ListSubjects(ctx, connect.NewRequest(&configv1.ListSubjectsRequest{GradeLevelId: gradeLevelID}))
	if err != nil {
		return nil, err
	}
	out := make([]SubjectResult, 0, len(res.Msg.GetSubjects()))
	for _, s := range res.Msg.GetSubjects() {
		out = append(out, toSubjectResult(s))
	}
	return out, nil
}

// --- terms ----------------------------------------------------------------

// CreateTerm adds a period within a cycle for a level.
func (c *ConfigClient) CreateTerm(ctx context.Context, in CreateTermInput) (TermResult, error) {
	res, err := c.rpc.CreateTerm(ctx, connect.NewRequest(&configv1.CreateTermRequest{
		CycleId:  in.CycleID,
		LevelId:  in.LevelID,
		Name:     in.Name,
		Ordinal:  int32(in.Ordinal),
		StartsOn: tsFromTime(in.StartsOn),
		EndsOn:   tsFromTime(in.EndsOn),
	}))
	if err != nil {
		return TermResult{}, err
	}
	return toTermResult(res.Msg.GetTerm()), nil
}

// ListTerms lists the terms of a cycle, optionally filtered by level.
func (c *ConfigClient) ListTerms(ctx context.Context, cycleID, levelID string) ([]TermResult, error) {
	res, err := c.rpc.ListTerms(ctx, connect.NewRequest(&configv1.ListTermsRequest{CycleId: cycleID, LevelId: levelID}))
	if err != nil {
		return nil, err
	}
	out := make([]TermResult, 0, len(res.Msg.GetTerms()))
	for _, tm := range res.Msg.GetTerms() {
		out = append(out, toTermResult(tm))
	}
	return out, nil
}

// --- groups ---------------------------------------------------------------

// CreateGroup adds a class group within a cycle for a grade.
func (c *ConfigClient) CreateGroup(ctx context.Context, in CreateGroupInput) (GroupResult, error) {
	res, err := c.rpc.CreateGroup(ctx, connect.NewRequest(&configv1.CreateGroupRequest{
		CycleId:      in.CycleID,
		GradeLevelId: in.GradeLevelID,
		Name:         in.Name,
		Capacity:     int32(in.Capacity),
	}))
	if err != nil {
		return GroupResult{}, err
	}
	return toGroupResult(res.Msg.GetGroup()), nil
}

// ListGroups lists the groups of a cycle, optionally filtered by grade.
func (c *ConfigClient) ListGroups(ctx context.Context, cycleID, gradeLevelID string) ([]GroupResult, error) {
	res, err := c.rpc.ListGroups(ctx, connect.NewRequest(&configv1.ListGroupsRequest{CycleId: cycleID, GradeLevelId: gradeLevelID}))
	if err != nil {
		return nil, err
	}
	out := make([]GroupResult, 0, len(res.Msg.GetGroups()))
	for _, g := range res.Msg.GetGroups() {
		out = append(out, toGroupResult(g))
	}
	return out, nil
}

// --- teachers -------------------------------------------------------------

// CreateTeacher adds a teacher catalog record. userID may be empty (reserved
// for a future IAM link).
func (c *ConfigClient) CreateTeacher(ctx context.Context, first, last, email, userID string) (Teacher, error) {
	res, err := c.rpc.CreateTeacher(ctx, connect.NewRequest(&configv1.CreateTeacherRequest{
		FirstName: first,
		LastName:  last,
		Email:     email,
		UserId:    userID,
	}))
	if err != nil {
		return Teacher{}, err
	}
	return toTeacher(res.Msg.GetTeacher()), nil
}

// SetTeacherUser links a teacher catalog record to an iam user, or unlinks it
// when userID is empty. The config service just stores the id; the gateway is
// responsible for having validated the user exists (via iam GetUser) first.
func (c *ConfigClient) SetTeacherUser(ctx context.Context, teacherID, userID string) (Teacher, error) {
	res, err := c.rpc.SetTeacherUser(ctx, connect.NewRequest(&configv1.SetTeacherUserRequest{
		TeacherId: teacherID,
		UserId:    userID,
	}))
	if err != nil {
		return Teacher{}, err
	}
	return toTeacher(res.Msg.GetTeacher()), nil
}

// ListTeachers lists every teacher in the tenant catalog.
func (c *ConfigClient) ListTeachers(ctx context.Context) ([]Teacher, error) {
	res, err := c.rpc.ListTeachers(ctx, connect.NewRequest(&configv1.ListTeachersRequest{}))
	if err != nil {
		return nil, err
	}
	out := make([]Teacher, 0, len(res.Msg.GetTeachers()))
	for _, t := range res.Msg.GetTeachers() {
		out = append(out, toTeacher(t))
	}
	return out, nil
}

// --- school calendar (M16) -------------------------------------------------

// NonWorkingPeriod is a day, or a run of days, the school does not operate.
// Dates are plain YYYY-MM-DD: a holiday is a calendar day, and a timestamp
// would invite a timezone to move it.
type NonWorkingPeriod struct {
	ID       string `json:"id"`
	CycleID  string `json:"cycleId"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	StartsOn string `json:"startsOn"`
	EndsOn   string `json:"endsOn"`
}

// CreateNonWorkingPeriod adds a holiday, vacation or suspension to a cycle.
func (c *ConfigClient) CreateNonWorkingPeriod(ctx context.Context, cycleID, label, kind, startsOn, endsOn string) (NonWorkingPeriod, error) {
	res, err := c.rpc.CreateNonWorkingPeriod(ctx, connect.NewRequest(&configv1.CreateNonWorkingPeriodRequest{
		CycleId: cycleID, Label: label, Kind: kind, StartsOn: startsOn, EndsOn: endsOn,
	}))
	if err != nil {
		return NonWorkingPeriod{}, err
	}
	return toNonWorkingPeriod(res.Msg.GetPeriod()), nil
}

// ListNonWorkingPeriods lists a cycle's closed days, earliest first.
func (c *ConfigClient) ListNonWorkingPeriods(ctx context.Context, cycleID string) ([]NonWorkingPeriod, error) {
	res, err := c.rpc.ListNonWorkingPeriods(ctx, connect.NewRequest(&configv1.ListNonWorkingPeriodsRequest{
		CycleId: cycleID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]NonWorkingPeriod, 0, len(res.Msg.GetPeriods()))
	for _, p := range res.Msg.GetPeriods() {
		out = append(out, toNonWorkingPeriod(p))
	}
	return out, nil
}

// DeleteNonWorkingPeriod removes one period.
func (c *ConfigClient) DeleteNonWorkingPeriod(ctx context.Context, id string) error {
	_, err := c.rpc.DeleteNonWorkingPeriod(ctx, connect.NewRequest(&configv1.DeleteNonWorkingPeriodRequest{Id: id}))
	return err
}

func toNonWorkingPeriod(p *configv1.NonWorkingPeriod) NonWorkingPeriod {
	return NonWorkingPeriod{
		ID:       p.GetId(),
		CycleID:  p.GetCycleId(),
		Label:    p.GetLabel(),
		Kind:     p.GetKind(),
		StartsOn: p.GetStartsOn(),
		EndsOn:   p.GetEndsOn(),
	}
}

// --- school agenda (M16) ---------------------------------------------------

// SchoolEvent is something the school DOES, as opposed to a day it is shut. The
// distinction is not cosmetic: a closed day moves a due date and an event never
// may.
type SchoolEvent struct {
	ID          string `json:"id"`
	CycleID     string `json:"cycleId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	StartsOn    string `json:"startsOn"`
	EndsOn      string `json:"endsOn"`
	// At most one is set; neither means the whole school.
	LevelID string `json:"levelId,omitempty"`
	GroupID string `json:"groupId,omitempty"`
}

// CreateSchoolEvent records something the school does.
func (c *ConfigClient) CreateSchoolEvent(ctx context.Context, in SchoolEvent) (SchoolEvent, error) {
	res, err := c.rpc.CreateSchoolEvent(ctx, connect.NewRequest(&configv1.CreateSchoolEventRequest{
		CycleId: in.CycleID, Title: in.Title, Description: in.Description,
		StartsOn: in.StartsOn, EndsOn: in.EndsOn,
		LevelId: in.LevelID, GroupId: in.GroupID,
	}))
	if err != nil {
		return SchoolEvent{}, err
	}
	return toSchoolEvent(res.Msg.GetEvent()), nil
}

// SchoolProfile is the school's details as a receipt prints them (task 12).
type SchoolProfile struct {
	Name      string `json:"name"`
	LegalName string `json:"legalName"`
	RFC       string `json:"rfc"`
	CCT       string `json:"cct"`
	Address   string `json:"address"`
	Phone     string `json:"phone"`
	Legend    string `json:"legend"`
}

// GetSchoolProfile returns the school's details; found is false until somebody
// fills them in.
func (c *ConfigClient) GetSchoolProfile(ctx context.Context) (SchoolProfile, bool, error) {
	res, err := c.rpc.GetSchoolProfile(ctx, connect.NewRequest(&configv1.GetSchoolProfileRequest{}))
	if err != nil {
		return SchoolProfile{}, false, err
	}
	return toSchoolProfile(res.Msg.GetProfile()), res.Msg.GetFound(), nil
}

// SetSchoolProfile replaces the school's details and returns them normalised.
func (c *ConfigClient) SetSchoolProfile(ctx context.Context, in SchoolProfile) (SchoolProfile, error) {
	res, err := c.rpc.SetSchoolProfile(ctx, connect.NewRequest(&configv1.SetSchoolProfileRequest{
		Profile: &configv1.SchoolProfile{
			Name: in.Name, LegalName: in.LegalName, Rfc: in.RFC, Cct: in.CCT,
			Address: in.Address, Phone: in.Phone, Legend: in.Legend,
		},
	}))
	if err != nil {
		return SchoolProfile{}, err
	}
	return toSchoolProfile(res.Msg.GetProfile()), nil
}

func toSchoolProfile(p *configv1.SchoolProfile) SchoolProfile {
	return SchoolProfile{
		Name: p.GetName(), LegalName: p.GetLegalName(), RFC: p.GetRfc(), CCT: p.GetCct(),
		Address: p.GetAddress(), Phone: p.GetPhone(), Legend: p.GetLegend(),
	}
}

// ListSchoolEvents returns a cycle's agenda, earliest first.
func (c *ConfigClient) ListSchoolEvents(ctx context.Context, cycleID string) ([]SchoolEvent, error) {
	res, err := c.rpc.ListSchoolEvents(ctx, connect.NewRequest(&configv1.ListSchoolEventsRequest{
		CycleId: cycleID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]SchoolEvent, 0, len(res.Msg.GetEvents()))
	for _, e := range res.Msg.GetEvents() {
		out = append(out, toSchoolEvent(e))
	}
	return out, nil
}

// DeleteSchoolEvent removes one event.
func (c *ConfigClient) DeleteSchoolEvent(ctx context.Context, id string) error {
	_, err := c.rpc.DeleteSchoolEvent(ctx, connect.NewRequest(&configv1.DeleteSchoolEventRequest{Id: id}))
	return err
}

func toSchoolEvent(e *configv1.SchoolEvent) SchoolEvent {
	return SchoolEvent{
		ID: e.GetId(), CycleID: e.GetCycleId(), Title: e.GetTitle(),
		Description: e.GetDescription(), StartsOn: e.GetStartsOn(), EndsOn: e.GetEndsOn(),
		LevelID: e.GetLevelId(), GroupID: e.GetGroupId(),
	}
}

// --- teaching_assignments -------------------------------------------------

// CreateTeachingAssignment binds a teacher to a (group, subject). The config
// service derives the cycle from the group; the returned assignment carries it.
func (c *ConfigClient) CreateTeachingAssignment(ctx context.Context, groupID, subjectID, teacherID string) (TeachingAssignment, error) {
	res, err := c.rpc.CreateTeachingAssignment(ctx, connect.NewRequest(&configv1.CreateTeachingAssignmentRequest{
		GroupId:   groupID,
		SubjectId: subjectID,
		TeacherId: teacherID,
	}))
	if err != nil {
		return TeachingAssignment{}, err
	}
	return toTeachingAssignment(res.Msg.GetTeachingAssignment()), nil
}

// ListTeachingAssignments lists assignments, filtered by any of cycleID,
// groupID, teacherID ("" = no filter on that dimension).
func (c *ConfigClient) ListTeachingAssignments(ctx context.Context, cycleID, groupID, teacherID string) ([]TeachingAssignment, error) {
	res, err := c.rpc.ListTeachingAssignments(ctx, connect.NewRequest(&configv1.ListTeachingAssignmentsRequest{
		CycleId:   cycleID,
		GroupId:   groupID,
		TeacherId: teacherID,
	}))
	if err != nil {
		return nil, err
	}
	out := make([]TeachingAssignment, 0, len(res.Msg.GetTeachingAssignments()))
	for _, a := range res.Msg.GetTeachingAssignments() {
		out = append(out, toTeachingAssignment(a))
	}
	return out, nil
}

// --- proto ↔ edge mappers -------------------------------------------------

func toCycleResult(c *configv1.Cycle) CycleResult {
	return CycleResult{
		ID:       c.GetId(),
		Name:     c.GetName(),
		Status:   cycleStatusName(c.GetStatus()),
		StartsOn: timeFromTS(c.GetStartsOn()),
		EndsOn:   timeFromTS(c.GetEndsOn()),
	}
}

func toLevelResult(l *configv1.Level) LevelResult {
	return LevelResult{
		ID:          l.GetId(),
		Name:        l.GetName(),
		SEPCode:     l.GetSepCode(),
		Granularity: granularityName(l.GetEnrollmentGranularity()),
	}
}

func toGradeLevelResult(g *configv1.GradeLevel) GradeLevelResult {
	return GradeLevelResult{
		ID:      g.GetId(),
		LevelID: g.GetLevelId(),
		Name:    g.GetName(),
		Ordinal: int(g.GetOrdinal()),
	}
}

func toSubjectResult(s *configv1.Subject) SubjectResult {
	return SubjectResult{
		ID:           s.GetId(),
		GradeLevelID: s.GetGradeLevelId(),
		Name:         s.GetName(),
	}
}

func toTermResult(t *configv1.Term) TermResult {
	return TermResult{
		ID:       t.GetId(),
		CycleID:  t.GetCycleId(),
		LevelID:  t.GetLevelId(),
		Name:     t.GetName(),
		Ordinal:  int(t.GetOrdinal()),
		StartsOn: timeFromTS(t.GetStartsOn()),
		EndsOn:   timeFromTS(t.GetEndsOn()),
	}
}

func toGroupResult(g *configv1.Group) GroupResult {
	return GroupResult{
		ID:           g.GetId(),
		CycleID:      g.GetCycleId(),
		GradeLevelID: g.GetGradeLevelId(),
		Name:         g.GetName(),
		Capacity:     int(g.GetCapacity()),
	}
}

func toTeacher(t *configv1.Teacher) Teacher {
	return Teacher{
		ID:        t.GetId(),
		FirstName: t.GetFirstName(),
		LastName:  t.GetLastName(),
		Email:     t.GetEmail(),
		UserID:    t.GetUserId(),
	}
}

func toTeachingAssignment(a *configv1.TeachingAssignment) TeachingAssignment {
	return TeachingAssignment{
		ID:        a.GetId(),
		CycleID:   a.GetCycleId(),
		GroupID:   a.GetGroupId(),
		SubjectID: a.GetSubjectId(),
		TeacherID: a.GetTeacherId(),
	}
}

// --- enum ↔ string --------------------------------------------------------

var cycleStatusNames = map[configv1.CycleStatus]string{
	configv1.CycleStatus_CYCLE_STATUS_DRAFT:  "draft",
	configv1.CycleStatus_CYCLE_STATUS_ACTIVE: "active",
	configv1.CycleStatus_CYCLE_STATUS_CLOSED: "closed",
}

func cycleStatusName(s configv1.CycleStatus) string { return cycleStatusNames[s] }

func cycleStatusFromName(name string) configv1.CycleStatus {
	for status, n := range cycleStatusNames {
		if n == name {
			return status
		}
	}
	return configv1.CycleStatus_CYCLE_STATUS_UNSPECIFIED
}

var granularityNames = map[configv1.EnrollmentGranularity]string{
	configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_CYCLE: "cycle",
	configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_TERM:  "term",
}

func granularityName(g configv1.EnrollmentGranularity) string { return granularityNames[g] }

func granularityFromName(name string) configv1.EnrollmentGranularity {
	for gran, n := range granularityNames {
		if n == name {
			return gran
		}
	}
	return configv1.EnrollmentGranularity_ENROLLMENT_GRANULARITY_UNSPECIFIED
}

// --- time ↔ timestamp -----------------------------------------------------

// tsFromTime converts a wall-clock time to a proto timestamp, mapping the zero
// time to nil so an unset date is not sent as the Unix epoch.
func tsFromTime(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// timeFromTS converts a proto timestamp to a wall-clock time, mapping nil to the
// zero time.
func timeFromTS(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}
