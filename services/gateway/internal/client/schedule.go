package client

import (
	"context"
	"time"

	connect "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	configv1 "github.com/garcia-lorca/gen/go/garcialorca/config/v1"
)

// The school timetable (T16) as the edge sees it. Times are minutes since
// midnight; weekdays are ISO (1 = Monday); dates are midnight UTC.

// ScheduleActor is who makes a timetable change.
type ScheduleActor struct {
	UserID string
	Name   string
}

// ScheduleGrid is the hours the calendar shows for a level.
type ScheduleGrid struct {
	LevelID   string
	GridStart int
	GridEnd   int
}

// ScheduleBlock is one class of the base timetable.
type ScheduleBlock struct {
	ID           string
	CycleID      string
	AssignmentID string
	GroupID      string
	SubjectID    string
	TeacherID    string
	Weekday      int
	StartsAt     int
	EndsAt       int
}

// ScheduleBlockInput places a teaching assignment on a weekday and time.
type ScheduleBlockInput struct {
	AssignmentID string
	Weekday      int
	StartsAt     int
	EndsAt       int
	Reason       string
}

// ScheduleOverride changes one class on one date.
type ScheduleOverride struct {
	ID          string
	CycleID     string
	BlockID     string
	Date        time.Time
	Kind        string
	TeacherID   string
	SwapBlockID string
	Reason      string
	CreatedBy   ScheduleActor
	CreatedAt   time.Time
}

// ScheduleOverrideInput is a new date-specific change.
type ScheduleOverrideInput struct {
	BlockID     string
	Date        time.Time
	Kind        string
	TeacherID   string
	SwapBlockID string
	Reason      string
}

// ScheduledClass is a class as it happens on a date; Override is nil when the
// class runs as planned.
type ScheduledClass struct {
	Block     ScheduleBlock
	TeacherID string
	StartsAt  int
	EndsAt    int
	Status    string
	Override  *ScheduleOverride
	// Absence is why an "uncovered" class has nobody (T19).
	Absence *TeacherAbsence
}

// ScheduleDay is the effective timetable of a date; NonWorking is the
// school-calendar label closing it ("" on a school day).
type ScheduleDay struct {
	Date       time.Time
	Weekday    int
	NonWorking string
	Classes    []ScheduledClass
}

// ScheduleBoard is a cycle's base timetable with the catalog it is drawn with.
type ScheduleBoard struct {
	Cycle       CycleResult
	Cycles      []CycleResult
	Started     bool
	Grids       []ScheduleGrid
	Levels      []LevelResult
	Grades      []GradeLevelResult
	Groups      []GroupResult
	Subjects    []SubjectResult
	Teachers    []Teacher
	Assignments []TeachingAssignment
	Blocks      []ScheduleBlock
}

// AvailabilityQuery asks who is free for a class; Date zero means the base
// timetable of Weekday.
type AvailabilityQuery struct {
	CycleID        string
	GroupID        string
	SubjectID      string
	Weekday        int
	Date           time.Time
	StartsAt       int
	EndsAt         int
	ExcludeBlockID string
}

// AvailableTeacher is a free teacher and how well they fit.
type AvailableTeacher struct {
	Teacher Teacher
	Match   string
}

// ScheduleEvent is one entry of the timetable's history.
type ScheduleEvent struct {
	ID         string
	At         time.Time
	By         ScheduleActor
	Action     string
	BlockID    string
	OverrideID string
	AbsenceID  string
	Date       time.Time // zero unless about a date-specific change or an absence
	Reason     string
	Summary    string
}

// GetScheduleBoard reads a cycle's timetable ("" = the active cycle),
// narrowed to teacherID's classes when set.
func (c *ConfigClient) GetScheduleBoard(ctx context.Context, cycleID, teacherID string) (ScheduleBoard, error) {
	res, err := c.rpc.GetScheduleBoard(ctx, connect.NewRequest(&configv1.GetScheduleBoardRequest{CycleId: cycleID, TeacherId: teacherID}))
	if err != nil {
		return ScheduleBoard{}, err
	}
	m := res.Msg
	b := ScheduleBoard{Cycle: toCycleResult(m.GetCycle()), Started: m.GetStarted()}
	for _, x := range m.GetCycles() {
		b.Cycles = append(b.Cycles, toCycleResult(x))
	}
	for _, g := range m.GetGrids() {
		b.Grids = append(b.Grids, toScheduleGrid(g))
	}
	for _, l := range m.GetLevels() {
		b.Levels = append(b.Levels, toLevelResult(l))
	}
	for _, g := range m.GetGradeLevels() {
		b.Grades = append(b.Grades, toGradeLevelResult(g))
	}
	for _, g := range m.GetGroups() {
		b.Groups = append(b.Groups, toGroupResult(g))
	}
	for _, s := range m.GetSubjects() {
		b.Subjects = append(b.Subjects, toSubjectResult(s))
	}
	for _, t := range m.GetTeachers() {
		b.Teachers = append(b.Teachers, toTeacher(t))
	}
	for _, a := range m.GetAssignments() {
		b.Assignments = append(b.Assignments, toTeachingAssignment(a))
	}
	for _, x := range m.GetBlocks() {
		b.Blocks = append(b.Blocks, toScheduleBlock(x))
	}
	return b, nil
}

// GetScheduleDay reads the effective timetable of a date.
func (c *ConfigClient) GetScheduleDay(ctx context.Context, cycleID string, date time.Time, teacherID string) (ScheduleDay, error) {
	res, err := c.rpc.GetScheduleDay(ctx, connect.NewRequest(&configv1.GetScheduleDayRequest{
		CycleId: cycleID, Date: timestamppb.New(date), TeacherId: teacherID,
	}))
	if err != nil {
		return ScheduleDay{}, err
	}
	d := ScheduleDay{Date: res.Msg.GetDate().AsTime().UTC(), Weekday: int(res.Msg.GetWeekday()), NonWorking: res.Msg.GetNonWorking()}
	for _, pc := range res.Msg.GetClasses() {
		d.Classes = append(d.Classes, toScheduledClass(pc))
	}
	return d, nil
}

func toScheduledClass(pc *configv1.ScheduledClass) ScheduledClass {
	cl := ScheduledClass{
		Block: toScheduleBlock(pc.GetBlock()), TeacherID: pc.GetTeacherId(), Status: pc.GetStatus(),
		StartsAt: int(pc.GetStartsAt()), EndsAt: int(pc.GetEndsAt()),
	}
	if pc.GetOverride() != nil {
		o := toScheduleOverride(pc.GetOverride())
		cl.Override = &o
	}
	if pc.GetAbsence() != nil {
		a := toTeacherAbsence(pc.GetAbsence())
		cl.Absence = &a
	}
	return cl
}

// CreateScheduleBlock adds a class to the base timetable.
func (c *ConfigClient) CreateScheduleBlock(ctx context.Context, in ScheduleBlockInput, by ScheduleActor) (ScheduleBlock, error) {
	res, err := c.rpc.CreateScheduleBlock(ctx, connect.NewRequest(&configv1.CreateScheduleBlockRequest{
		Block: toProtoBlockInput(in), Actor: toProtoScheduleActor(by),
	}))
	if err != nil {
		return ScheduleBlock{}, err
	}
	return toScheduleBlock(res.Msg.GetBlock()), nil
}

// UpdateScheduleBlock moves or changes a class.
func (c *ConfigClient) UpdateScheduleBlock(ctx context.Context, id string, in ScheduleBlockInput, by ScheduleActor) (ScheduleBlock, error) {
	res, err := c.rpc.UpdateScheduleBlock(ctx, connect.NewRequest(&configv1.UpdateScheduleBlockRequest{
		Id: id, Block: toProtoBlockInput(in), Actor: toProtoScheduleActor(by),
	}))
	if err != nil {
		return ScheduleBlock{}, err
	}
	return toScheduleBlock(res.Msg.GetBlock()), nil
}

// DeleteScheduleBlock removes a class.
func (c *ConfigClient) DeleteScheduleBlock(ctx context.Context, id, reason string, by ScheduleActor) error {
	_, err := c.rpc.DeleteScheduleBlock(ctx, connect.NewRequest(&configv1.DeleteScheduleBlockRequest{
		Id: id, Reason: reason, Actor: toProtoScheduleActor(by),
	}))
	return err
}

// CreateScheduleOverride records a date-specific change.
func (c *ConfigClient) CreateScheduleOverride(ctx context.Context, in ScheduleOverrideInput, by ScheduleActor) (ScheduleOverride, error) {
	res, err := c.rpc.CreateScheduleOverride(ctx, connect.NewRequest(&configv1.CreateScheduleOverrideRequest{
		BlockId: in.BlockID, Date: timestamppb.New(in.Date), Kind: in.Kind, TeacherId: in.TeacherID,
		SwapBlockId: in.SwapBlockID, Reason: in.Reason, Actor: toProtoScheduleActor(by),
	}))
	if err != nil {
		return ScheduleOverride{}, err
	}
	return toScheduleOverride(res.Msg.GetOverride()), nil
}

// DeleteScheduleOverride undoes a date-specific change.
func (c *ConfigClient) DeleteScheduleOverride(ctx context.Context, id, reason string, by ScheduleActor) error {
	_, err := c.rpc.DeleteScheduleOverride(ctx, connect.NewRequest(&configv1.DeleteScheduleOverrideRequest{
		Id: id, Reason: reason, Actor: toProtoScheduleActor(by),
	}))
	return err
}

// ListAvailableTeachers lists who is free for a class, best fits first.
func (c *ConfigClient) ListAvailableTeachers(ctx context.Context, q AvailabilityQuery) ([]AvailableTeacher, error) {
	req := &configv1.ListAvailableTeachersRequest{
		CycleId: q.CycleID, GroupId: q.GroupID, SubjectId: q.SubjectID, ExcludeBlockId: q.ExcludeBlockID,
		Weekday: int32(q.Weekday), StartsAt: int32(q.StartsAt), EndsAt: int32(q.EndsAt), //nolint:gosec // checked at the edge
	}
	if !q.Date.IsZero() {
		req.Date = timestamppb.New(q.Date)
	}
	res, err := c.rpc.ListAvailableTeachers(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	out := make([]AvailableTeacher, 0, len(res.Msg.GetTeachers()))
	for _, a := range res.Msg.GetTeachers() {
		out = append(out, AvailableTeacher{Teacher: toTeacher(a.GetTeacher()), Match: a.GetMatch()})
	}
	return out, nil
}

// SetScheduleGrid saves a level's grid.
func (c *ConfigClient) SetScheduleGrid(ctx context.Context, levelID string, start, end int) (ScheduleGrid, error) {
	res, err := c.rpc.SetScheduleGrid(ctx, connect.NewRequest(&configv1.SetScheduleGridRequest{
		LevelId: levelID, GridStart: int32(start), GridEnd: int32(end), //nolint:gosec // checked at the edge
	}))
	if err != nil {
		return ScheduleGrid{}, err
	}
	return toScheduleGrid(res.Msg.GetGrid()), nil
}

// ListScheduleEvents reads the timetable's history.
func (c *ConfigClient) ListScheduleEvents(ctx context.Context, cycleID string, limit int) ([]ScheduleEvent, error) {
	res, err := c.rpc.ListScheduleEvents(ctx, connect.NewRequest(&configv1.ListScheduleEventsRequest{
		CycleId: cycleID, Limit: int32(limit), //nolint:gosec // 1..500, checked at the edge
	}))
	if err != nil {
		return nil, err
	}
	out := make([]ScheduleEvent, 0, len(res.Msg.GetEvents()))
	for _, e := range res.Msg.GetEvents() {
		ev := ScheduleEvent{
			ID: e.GetId(), At: e.GetAt().AsTime().UTC(), By: ScheduleActor{UserID: e.GetBy().GetUserId(), Name: e.GetBy().GetName()},
			Action: e.GetAction(), BlockID: e.GetBlockId(), OverrideID: e.GetOverrideId(), AbsenceID: e.GetAbsenceId(), Reason: e.GetReason(), Summary: e.GetSummary(),
		}
		if e.GetDate() != nil {
			ev.Date = e.GetDate().AsTime().UTC()
		}
		out = append(out, ev)
	}
	return out, nil
}

func toProtoBlockInput(in ScheduleBlockInput) *configv1.ScheduleBlockInput {
	return &configv1.ScheduleBlockInput{
		AssignmentId: in.AssignmentID, Reason: in.Reason,
		Weekday: int32(in.Weekday), StartsAt: int32(in.StartsAt), EndsAt: int32(in.EndsAt), //nolint:gosec // checked at the edge
	}
}

func toProtoScheduleActor(a ScheduleActor) *configv1.ScheduleActor {
	return &configv1.ScheduleActor{UserId: a.UserID, Name: a.Name}
}

func toScheduleGrid(g *configv1.ScheduleGrid) ScheduleGrid {
	return ScheduleGrid{LevelID: g.GetLevelId(), GridStart: int(g.GetGridStart()), GridEnd: int(g.GetGridEnd())}
}

func toScheduleBlock(b *configv1.ScheduleBlock) ScheduleBlock {
	return ScheduleBlock{
		ID: b.GetId(), CycleID: b.GetCycleId(), AssignmentID: b.GetAssignmentId(), GroupID: b.GetGroupId(),
		SubjectID: b.GetSubjectId(), TeacherID: b.GetTeacherId(), Weekday: int(b.GetWeekday()),
		StartsAt: int(b.GetStartsAt()), EndsAt: int(b.GetEndsAt()),
	}
}

func toScheduleOverride(o *configv1.ScheduleOverride) ScheduleOverride {
	return ScheduleOverride{
		ID: o.GetId(), CycleID: o.GetCycleId(), BlockID: o.GetBlockId(), Date: o.GetDate().AsTime().UTC(), Kind: o.GetKind(),
		TeacherID: o.GetTeacherId(), SwapBlockID: o.GetSwapBlockId(), Reason: o.GetReason(),
		CreatedBy: ScheduleActor{UserID: o.GetCreatedBy().GetUserId(), Name: o.GetCreatedBy().GetName()},
		CreatedAt: o.GetCreatedAt().AsTime().UTC(),
	}
}
