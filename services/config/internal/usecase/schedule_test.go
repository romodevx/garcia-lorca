package usecase_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/ports"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// fakeSchedule is an in-memory ScheduleRepository + ScheduleTx: one cycle
// (2026-08-24 … 2027-07-09), one level with grades 3 and 4, groups 3A, 3B, 4A,
// four teachers and their assignments.
type fakeSchedule struct {
	cat       ports.ScheduleCatalog
	blocks    map[string]domain.ScheduleBlock
	overrides map[string]domain.ScheduleOverride
	events    []domain.ScheduleEvent
	grids     map[string]domain.ScheduleGrid
	txs       int
}

const (
	sCycle = "cycle-1"
	sLevel = "lvl-1"
	g3, g4 = "grade-3", "grade-4"
	gr3A   = "group-3A"
	gr3B   = "group-3B"
	gr4A   = "group-4A"
	// teachers
	tAna, tLuis, tEva, tZoe = "t-ana", "t-luis", "t-eva", "t-zoe"
	// subjects
	sMat3, sMus3, sEsp4, sMus4 = "s-mat3", "s-mus3", "s-esp4", "s-mus4"
	// assignments
	aMat3A = "a-mat3A" // Ana, Matemáticas 3A
	aMus3A = "a-mus3A" // Luis, Música 3A
	aMat3B = "a-mat3B" // Ana, Matemáticas 3B
	aEsp4A = "a-esp4A" // Eva, Español 4A
	aMus4A = "a-mus4A" // Zoe, Música 4A
)

func newFakeSchedule(t *testing.T) *fakeSchedule {
	t.Helper()
	must := func(err error) { require.NoError(t, err) }
	cycle, err := domain.NewCycle(sCycle, "ten", "2026-2027", domain.CycleStatusActive, d(2026, time.August, 24), d(2027, time.July, 9))
	must(err)
	level, err := domain.NewLevel(sLevel, "ten", "Primaria", "", domain.GranularityCycle)
	must(err)
	gr := func(id, name string, ord int) domain.GradeLevel {
		g, err := domain.NewGradeLevel(id, "ten", sLevel, name, ord)
		must(err)
		return g
	}
	grp := func(id, grade, name string) domain.Group {
		g, err := domain.NewGroup(id, "ten", sCycle, grade, name, 30)
		must(err)
		return g
	}
	subj := func(id, grade, name string) domain.Subject {
		s, err := domain.NewSubject(id, "ten", grade, name)
		must(err)
		return s
	}
	teacher := func(id, first, last string) domain.Teacher {
		tc, err := domain.NewTeacher(id, "ten", first, last, id+"@x.mx", "")
		must(err)
		return tc
	}
	asg := func(id, group, subject, tc string) domain.TeachingAssignment {
		a, err := domain.NewTeachingAssignment(id, "ten", sCycle, group, subject, tc)
		must(err)
		return a
	}
	return &fakeSchedule{
		cat: ports.ScheduleCatalog{
			Cycle:    cycle,
			Levels:   []domain.Level{level},
			Grades:   []domain.GradeLevel{gr(g3, "3.º", 3), gr(g4, "4.º", 4)},
			Groups:   []domain.Group{grp(gr3A, g3, "A"), grp(gr3B, g3, "B"), grp(gr4A, g4, "A")},
			Subjects: []domain.Subject{subj(sMat3, g3, "Matemáticas"), subj(sMus3, g3, "Música"), subj(sEsp4, g4, "Español"), subj(sMus4, g4, "MÚSICA")},
			Teachers: []domain.Teacher{teacher(tAna, "Ana", "Ruiz"), teacher(tLuis, "Luis", "Pérez"), teacher(tEva, "Eva", "Soto"), teacher(tZoe, "Zoe", "Alba")},
			Assignments: []domain.TeachingAssignment{
				asg(aMat3A, gr3A, sMat3, tAna), asg(aMus3A, gr3A, sMus3, tLuis), asg(aMat3B, gr3B, sMat3, tAna),
				asg(aEsp4A, gr4A, sEsp4, tEva), asg(aMus4A, gr4A, sMus4, tZoe),
			},
		},
		blocks:    map[string]domain.ScheduleBlock{},
		overrides: map[string]domain.ScheduleOverride{},
		grids:     map[string]domain.ScheduleGrid{},
	}
}

func notFound(what string) error { return glerrors.New(glerrors.KindNotFound, what+" not found") }

func (f *fakeSchedule) GetCycle(_ context.Context, id string) (domain.Cycle, error) {
	if id != f.cat.Cycle.ID() {
		return domain.Cycle{}, notFound("cycle")
	}
	return f.cat.Cycle, nil
}
func (f *fakeSchedule) ListCycles(context.Context) ([]domain.Cycle, error) {
	return []domain.Cycle{f.cat.Cycle}, nil
}
func (f *fakeSchedule) GetLevel(_ context.Context, id string) (domain.Level, error) {
	for _, l := range f.cat.Levels {
		if l.ID() == id {
			return l, nil
		}
	}
	return domain.Level{}, notFound("level")
}
func (f *fakeSchedule) GetTeachingAssignment(_ context.Context, id string) (domain.TeachingAssignment, error) {
	for _, a := range f.cat.Assignments {
		if a.ID() == id {
			return a, nil
		}
	}
	return domain.TeachingAssignment{}, notFound("assignment")
}
func (f *fakeSchedule) GetScheduleBlock(_ context.Context, id string) (domain.ScheduleBlock, error) {
	return f.GetBlock(id)
}
func (f *fakeSchedule) GetScheduleOverride(_ context.Context, id string) (domain.ScheduleOverride, error) {
	return f.GetOverride(id)
}
func (f *fakeSchedule) LoadScheduleCatalog(_ context.Context, cycleID string) (ports.ScheduleCatalog, error) {
	if cycleID != f.cat.Cycle.ID() {
		return ports.ScheduleCatalog{}, notFound("cycle")
	}
	cat := f.cat
	cat.Grids = nil
	for _, g := range f.grids {
		cat.Grids = append(cat.Grids, g)
	}
	return cat, nil
}
func (f *fakeSchedule) ListScheduleBlocks(_ context.Context, _ string) ([]domain.ScheduleBlock, error) {
	return f.Blocks(sCycle)
}
func (f *fakeSchedule) ListScheduleOverrides(_ context.Context, _ string, from, to time.Time) ([]domain.ScheduleOverride, error) {
	var out []domain.ScheduleOverride
	for _, o := range f.overrides {
		if !o.Date().Before(from) && !o.Date().After(to) {
			out = append(out, o)
		}
	}
	return out, nil
}
func (f *fakeSchedule) ListScheduleEvents(_ context.Context, _ string, limit int) ([]domain.ScheduleEvent, error) {
	if limit > len(f.events) {
		limit = len(f.events)
	}
	return f.events[:limit], nil
}
func (f *fakeSchedule) SaveScheduleGrid(_ context.Context, g domain.ScheduleGrid) error {
	f.grids[g.LevelID()] = g
	return nil
}
func (f *fakeSchedule) RunScheduleTx(_ context.Context, _ string, fn func(ports.ScheduleTx) error) error {
	f.txs++
	return fn(f)
}

// --- ScheduleTx --------------------------------------------------------------

func (f *fakeSchedule) Blocks(string) ([]domain.ScheduleBlock, error) {
	out := make([]domain.ScheduleBlock, 0, len(f.blocks))
	for _, b := range f.blocks {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}
func (f *fakeSchedule) OverridesFrom(_ string, from time.Time) ([]domain.ScheduleOverride, error) {
	return f.ListScheduleOverrides(context.Background(), sCycle, from, d(9999, time.December, 31))
}
func (f *fakeSchedule) GetBlock(id string) (domain.ScheduleBlock, error) {
	b, ok := f.blocks[id]
	if !ok {
		return domain.ScheduleBlock{}, notFound("block")
	}
	return b, nil
}
func (f *fakeSchedule) GetOverride(id string) (domain.ScheduleOverride, error) {
	o, ok := f.overrides[id]
	if !ok {
		return domain.ScheduleOverride{}, notFound("override")
	}
	return o, nil
}
func (f *fakeSchedule) InsertBlock(b domain.ScheduleBlock) error { f.blocks[b.ID()] = b; return nil }
func (f *fakeSchedule) UpdateBlock(b domain.ScheduleBlock) error { f.blocks[b.ID()] = b; return nil }
func (f *fakeSchedule) DeleteBlock(id string) error {
	delete(f.blocks, id)
	for oid, o := range f.overrides {
		if o.Touches(id) {
			delete(f.overrides, oid)
		}
	}
	return nil
}
func (f *fakeSchedule) InsertOverride(o domain.ScheduleOverride) error {
	f.overrides[o.ID()] = o
	return nil
}
func (f *fakeSchedule) DeleteOverride(id string) error { delete(f.overrides, id); return nil }
func (f *fakeSchedule) AppendEvent(e domain.ScheduleEvent) error {
	f.events = append([]domain.ScheduleEvent{e}, f.events...)
	return nil
}

// --- rig ----------------------------------------------------------------------

var director = domain.Actor{UserID: "u-dir", Name: "Directora Ruiz"}

// mexico is the school's zone; "now" is 2026-08-10 12:00 there unless a test
// moves it — before the cycle starts on 2026-08-24.
func newScheduleService(f *fakeSchedule, now time.Time) *usecase.ScheduleService {
	loc, _ := time.LoadLocation("America/Mexico_City")
	n := 0
	return usecase.NewScheduleService(f, "ten", func() string { n++; return fmt.Sprintf("id-%02d", n) }, func() time.Time { return now }, loc)
}

func beforeCycle() time.Time { return time.Date(2026, time.August, 10, 18, 0, 0, 0, time.UTC) }

// duringCycle is Monday 2026-10-05 at 19:00 in Mexico City (01:00 UTC on
// Tuesday): the school's date is still Monday the 5th.
func duringCycle() time.Time { return time.Date(2026, time.October, 6, 1, 0, 0, 0, time.UTC) }

var mon5 = d(2026, time.October, 5)

func at(h, m int) int { return h*60 + m }

func addBlock(t *testing.T, s *usecase.ScheduleService, assignment string, weekday, from, to int) domain.ScheduleBlock {
	t.Helper()
	b, err := s.CreateBlock(context.Background(), usecase.BlockInput{AssignmentID: assignment, Weekday: weekday, Start: from, End: to}, director)
	require.NoError(t, err)
	return b
}

func codeOf(err error) domain.RefusalCode { return domain.RefusalCodeOf(err) }

// --- tests --------------------------------------------------------------------

func TestCreateBlockFromAnAssignment(t *testing.T) {
	f := newFakeSchedule(t)
	s := newScheduleService(f, beforeCycle())
	b := addBlock(t, s, aMat3A, 1, at(8, 0), at(8, 50))
	assert.Equal(t, gr3A, b.GroupID())
	assert.Equal(t, sMat3, b.SubjectID())
	assert.Equal(t, tAna, b.TeacherID())
	assert.Equal(t, sCycle, b.CycleID())
	assert.Empty(t, f.events, "before the cycle starts the base is edited freely, without history")

	_, err := s.CreateBlock(context.Background(), usecase.BlockInput{AssignmentID: "nope", Weekday: 1, Start: at(9, 0), End: at(9, 50)}, director)
	assert.Equal(t, domain.CodeScheduleAssignmentInvalid, codeOf(err))
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
}

func TestCreateBlockRefusesDoubleBooking(t *testing.T) {
	f := newFakeSchedule(t)
	s := newScheduleService(f, beforeCycle())
	addBlock(t, s, aMat3A, 1, at(8, 0), at(8, 50))

	_, err := s.CreateBlock(context.Background(), usecase.BlockInput{AssignmentID: aMat3B, Weekday: 1, Start: at(8, 30), End: at(9, 20)}, director)
	require.Error(t, err)
	assert.Equal(t, domain.CodeScheduleTeacherBusy, codeOf(err))
	assert.Equal(t, "Ana Ruiz ya da Matemáticas a 3.º A el lunes de 08:00 a 08:50", glerrors.SafeMessage(err))

	_, err = s.CreateBlock(context.Background(), usecase.BlockInput{AssignmentID: aMus3A, Weekday: 1, Start: at(8, 45), End: at(9, 30)}, director)
	assert.Equal(t, domain.CodeScheduleGroupBusy, codeOf(err))

	// back to back, another weekday: fine
	addBlock(t, s, aMus3A, 1, at(8, 50), at(9, 40))
	addBlock(t, s, aMat3B, 2, at(8, 0), at(8, 50))
	assert.Len(t, f.blocks, 3)
}

func TestCreateBlockStaysInsideTheLevelGrid(t *testing.T) {
	f := newFakeSchedule(t)
	s := newScheduleService(f, beforeCycle())
	_, err := s.CreateBlock(context.Background(), usecase.BlockInput{AssignmentID: aMat3A, Weekday: 1, Start: at(6, 30), End: at(7, 20)}, director)
	assert.Equal(t, domain.CodeScheduleOutsideGrid, codeOf(err))

	_, err = s.SetGrid(context.Background(), sLevel, at(6, 30), at(14, 0))
	require.NoError(t, err)
	addBlock(t, s, aMat3A, 1, at(6, 30), at(7, 20))

	_, err = s.CreateBlock(context.Background(), usecase.BlockInput{AssignmentID: aMat3A, Weekday: 1, Start: at(8, 3), End: at(9, 0)}, director)
	assert.Equal(t, domain.CodeScheduleTimeInvalid, codeOf(err))
	_, err = s.CreateBlock(context.Background(), usecase.BlockInput{AssignmentID: aMat3A, Weekday: 6, Start: at(8, 0), End: at(9, 0)}, director)
	assert.Equal(t, domain.CodeScheduleWeekdayInvalid, codeOf(err))
}

func TestBaseEditsAfterTheCycleStartedNeedAReasonAndAreRecorded(t *testing.T) {
	f := newFakeSchedule(t)
	b := addBlock(t, newScheduleService(f, beforeCycle()), aMat3A, 1, at(8, 0), at(8, 50))
	s := newScheduleService(f, duringCycle())

	_, err := s.CreateBlock(context.Background(), usecase.BlockInput{AssignmentID: aMus3A, Weekday: 1, Start: at(9, 0), End: at(9, 50)}, director)
	assert.Equal(t, domain.CodeScheduleReasonRequired, codeOf(err))

	_, err = s.CreateBlock(context.Background(), usecase.BlockInput{AssignmentID: aMus3A, Weekday: 1, Start: at(9, 0), End: at(9, 50), Reason: "Nueva materia"}, director)
	require.NoError(t, err)
	require.Len(t, f.events, 1)
	e := f.events[0]
	assert.Equal(t, domain.ActionBlockCreated, e.Action)
	assert.Equal(t, director, e.By)
	assert.Equal(t, "Nueva materia", e.Reason)
	assert.Equal(t, "Clase nueva: Música, 3.º A, lunes 09:00–09:50, Luis Pérez", e.Summary)

	_, err = s.UpdateBlock(context.Background(), b.ID(), usecase.BlockInput{AssignmentID: aMat3A, Weekday: 2, Start: at(8, 0), End: at(8, 50), Reason: "Ajuste"}, director)
	require.NoError(t, err)
	assert.Equal(t, domain.ActionBlockUpdated, f.events[0].Action)
	assert.Contains(t, f.events[0].Summary, "→ martes 08:00–08:50")

	require.Equal(t, domain.CodeScheduleReasonRequired, codeOf(s.DeleteBlock(context.Background(), b.ID(), "", director)))
	require.NoError(t, s.DeleteBlock(context.Background(), b.ID(), "Ya no se imparte", director))
	assert.Equal(t, domain.ActionBlockDeleted, f.events[0].Action)
	assert.NotContains(t, f.blocks, b.ID())
}

func TestUpdateBlockChecksOthersButNotItself(t *testing.T) {
	f := newFakeSchedule(t)
	s := newScheduleService(f, beforeCycle())
	b := addBlock(t, s, aMat3A, 1, at(8, 0), at(8, 50))
	addBlock(t, s, aMus3A, 1, at(9, 0), at(9, 50))

	moved, err := s.UpdateBlock(context.Background(), b.ID(), usecase.BlockInput{AssignmentID: aMat3A, Weekday: 1, Start: at(8, 10), End: at(9, 0)}, director)
	require.NoError(t, err)
	assert.Equal(t, at(8, 10), moved.Time().Start())

	_, err = s.UpdateBlock(context.Background(), b.ID(), usecase.BlockInput{AssignmentID: aMat3A, Weekday: 1, Start: at(8, 30), End: at(9, 20)}, director)
	assert.Equal(t, domain.CodeScheduleGroupBusy, codeOf(err))

	_, err = s.UpdateBlock(context.Background(), "nope", usecase.BlockInput{AssignmentID: aMat3A, Weekday: 1, Start: at(8, 30), End: at(9, 20)}, director)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestSubstituteForOneDate(t *testing.T) {
	f := newFakeSchedule(t)
	pre := newScheduleService(f, beforeCycle())
	music := addBlock(t, pre, aMus3A, 1, at(8, 50), at(9, 40))
	spanish := addBlock(t, pre, aEsp4A, 1, at(9, 0), at(9, 50))
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()

	// Eva teaches 4A at 9:00 that Monday: she cannot cover music at 8:50.
	_, err := s.CreateOverride(ctx, usecase.OverrideInput{BlockID: music.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: tEva, Reason: "Llegó tarde"}, director)
	require.Error(t, err)
	assert.Equal(t, domain.CodeScheduleTeacherBusy, codeOf(err))
	assert.Equal(t, "Eva Soto ya da Español a 4.º A el lunes de 09:00 a 09:50", glerrors.SafeMessage(err))

	o, err := s.CreateOverride(ctx, usecase.OverrideInput{BlockID: music.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: tZoe, Reason: "Llegó tarde"}, director)
	require.NoError(t, err)
	assert.Equal(t, director, o.By())
	require.Len(t, f.events, 1)
	assert.Equal(t, domain.ActionOverrideCreated, f.events[0].Action)
	assert.Equal(t, "Suplencia el 5 de octubre de 2026: Música, 3.º A, 08:50–09:40 — Zoe Alba en lugar de Luis Pérez", f.events[0].Summary)
	assert.Equal(t, mon5, f.events[0].Date)

	// the base is untouched
	assert.Equal(t, tLuis, f.blocks[music.ID()].TeacherID())

	day, err := s.Day(ctx, sCycle, mon5, "")
	require.NoError(t, err)
	require.Len(t, day.Classes, 2)
	assert.Equal(t, tZoe, day.Classes[0].TeacherID)
	assert.Equal(t, domain.ClassSubstitute, day.Classes[0].Status)

	// Zoe now covers 8:50–9:40 that day: she cannot also cover Spanish at 9:00.
	_, err = s.CreateOverride(ctx, usecase.OverrideInput{BlockID: spanish.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: tZoe, Reason: "Junta"}, director)
	assert.Equal(t, domain.CodeScheduleTeacherBusy, codeOf(err))

	// one change per class and date
	_, err = s.CreateOverride(ctx, usecase.OverrideInput{BlockID: music.ID(), Date: mon5, Kind: domain.OverrideCancel, Reason: "otra"}, director)
	assert.Equal(t, domain.CodeScheduleBlockTaken, codeOf(err))

	// the class's own teacher is no substitute
	_, err = s.CreateOverride(ctx, usecase.OverrideInput{BlockID: spanish.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: tEva, Reason: "x"}, director)
	assert.Equal(t, domain.CodeScheduleSubstituteInvalid, codeOf(err))
	_, err = s.CreateOverride(ctx, usecase.OverrideInput{BlockID: spanish.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: "t-ghost", Reason: "x"}, director)
	assert.Equal(t, domain.CodeScheduleSubstituteInvalid, codeOf(err))
}

func TestOverrideDateRules(t *testing.T) {
	f := newFakeSchedule(t)
	music := addBlock(t, newScheduleService(f, beforeCycle()), aMus3A, 1, at(8, 50), at(9, 40))
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	in := usecase.OverrideInput{BlockID: music.ID(), Kind: domain.OverrideCancel, Reason: "Junta"}

	in.Date = mon5.AddDate(0, 0, 1) // a Tuesday
	_, err := s.CreateOverride(ctx, in, director)
	assert.Equal(t, domain.CodeScheduleDateWrongWeekday, codeOf(err))

	in.Date = d(2027, time.August, 2) // a Monday after the cycle
	_, err = s.CreateOverride(ctx, in, director)
	assert.Equal(t, domain.CodeScheduleDateOutsideCycle, codeOf(err))

	in.Date, in.Reason = mon5, " "
	_, err = s.CreateOverride(ctx, in, director)
	assert.Equal(t, domain.CodeScheduleReasonRequired, codeOf(err))

	in.BlockID = "nope"
	in.Reason = "x"
	_, err = s.CreateOverride(ctx, in, director)
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestSwapTwoClassesOfTheGroupForADay(t *testing.T) {
	f := newFakeSchedule(t)
	pre := newScheduleService(f, beforeCycle())
	math := addBlock(t, pre, aMat3A, 1, at(8, 0), at(8, 50))
	music := addBlock(t, pre, aMus3A, 1, at(8, 50), at(9, 40))
	other := addBlock(t, pre, aEsp4A, 1, at(8, 0), at(8, 50))
	// Ana also teaches 3B at 8:50 — swapping would put her in two groups.
	math3B := addBlock(t, pre, aMat3B, 1, at(8, 50), at(9, 40))
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()

	_, err := s.CreateOverride(ctx, usecase.OverrideInput{BlockID: math.ID(), Date: mon5, Kind: domain.OverrideSwap, SwapBlockID: other.ID(), Reason: "x"}, director)
	assert.Equal(t, domain.CodeScheduleSwapInvalid, codeOf(err), "only classes of the same group trade places")

	_, err = s.CreateOverride(ctx, usecase.OverrideInput{BlockID: math.ID(), Date: mon5, Kind: domain.OverrideSwap, SwapBlockID: music.ID(), Reason: "x"}, director)
	assert.Equal(t, domain.CodeScheduleTeacherBusy, codeOf(err))

	require.NoError(t, s.DeleteBlock(ctx, math3B.ID(), "Se fue", director))
	o, err := s.CreateOverride(ctx, usecase.OverrideInput{BlockID: math.ID(), Date: mon5, Kind: domain.OverrideSwap, SwapBlockID: music.ID(), Reason: "Música primero"}, director)
	require.NoError(t, err)
	assert.Equal(t, music.ID(), o.SwapBlockID())

	// the partner is taken that day as well
	_, err = s.CreateOverride(ctx, usecase.OverrideInput{BlockID: music.ID(), Date: mon5, Kind: domain.OverrideCancel, Reason: "x"}, director)
	assert.Equal(t, domain.CodeScheduleBlockTaken, codeOf(err))

	day, err := s.Day(ctx, sCycle, mon5, "")
	require.NoError(t, err)
	byBlock := map[string]domain.ScheduledClass{}
	for _, c := range day.Classes {
		byBlock[c.Block.ID()] = c
	}
	assert.Equal(t, at(8, 50), byBlock[math.ID()].Time.Start())
	assert.Equal(t, at(8, 0), byBlock[music.ID()].Time.Start())
}

func TestUndoingAChangeCannotDoubleBook(t *testing.T) {
	f := newFakeSchedule(t)
	pre := newScheduleService(f, beforeCycle())
	music := addBlock(t, pre, aMus3A, 1, at(8, 50), at(9, 40))
	spanish := addBlock(t, pre, aEsp4A, 1, at(9, 0), at(9, 50))
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()

	// Luis is late: Zoe covers music. Luis then covers Spanish at 9:00 (he is
	// free at 9:00 that day since he is not giving music).
	cover, err := s.CreateOverride(ctx, usecase.OverrideInput{BlockID: music.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: tZoe, Reason: "tarde"}, director)
	require.NoError(t, err)
	_, err = s.CreateOverride(ctx, usecase.OverrideInput{BlockID: spanish.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: tLuis, Reason: "junta"}, director)
	require.NoError(t, err)

	// Undoing Zoe's cover would put Luis in 3A and 4A at once.
	err = s.DeleteOverride(ctx, cover.ID(), "", director)
	assert.Equal(t, domain.CodeScheduleTeacherBusy, codeOf(err))
	assert.Contains(t, f.overrides, cover.ID())
}

func TestDeleteOverrideIsRecorded(t *testing.T) {
	f := newFakeSchedule(t)
	music := addBlock(t, newScheduleService(f, beforeCycle()), aMus3A, 1, at(8, 50), at(9, 40))
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	o, err := s.CreateOverride(ctx, usecase.OverrideInput{BlockID: music.ID(), Date: mon5, Kind: domain.OverrideCancel, Reason: "Junta"}, director)
	require.NoError(t, err)
	require.NoError(t, s.DeleteOverride(ctx, o.ID(), "Siempre sí", director))
	assert.Empty(t, f.overrides)
	require.Len(t, f.events, 2)
	assert.Equal(t, domain.ActionOverrideDeleted, f.events[0].Action)
	assert.Equal(t, "Siempre sí", f.events[0].Reason)
	assert.Contains(t, f.events[0].Summary, "Se deshizo")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(s.DeleteOverride(ctx, o.ID(), "", director)))
}

func TestBaseEditCannotBreakAFutureChange(t *testing.T) {
	f := newFakeSchedule(t)
	pre := newScheduleService(f, beforeCycle())
	music := addBlock(t, pre, aMus3A, 1, at(8, 50), at(9, 40))
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()

	// next Monday (the 12th) Eva covers music 8:50–9:40.
	_, err := s.CreateOverride(ctx, usecase.OverrideInput{BlockID: music.ID(), Date: mon5.AddDate(0, 0, 7), Kind: domain.OverrideSubstitute, TeacherID: tEva, Reason: "curso"}, director)
	require.NoError(t, err)

	// A new base class for Eva on Mondays at 9:00 would collide with that cover.
	_, err = s.CreateBlock(ctx, usecase.BlockInput{AssignmentID: aEsp4A, Weekday: 1, Start: at(9, 0), End: at(9, 50), Reason: "nueva"}, director)
	require.Error(t, err)
	assert.Equal(t, domain.CodeScheduleOverrideConflict, codeOf(err))
	assert.Contains(t, glerrors.SafeMessage(err), "12 de octubre de 2026")

	// Tuesdays are fine.
	_, err = s.CreateBlock(ctx, usecase.BlockInput{AssignmentID: aEsp4A, Weekday: 2, Start: at(9, 0), End: at(9, 50), Reason: "nueva"}, director)
	require.NoError(t, err)

	// A class with future changes cannot move to another weekday.
	_, err = s.UpdateBlock(ctx, music.ID(), usecase.BlockInput{AssignmentID: aMus3A, Weekday: 3, Start: at(8, 50), End: at(9, 40), Reason: "x"}, director)
	assert.Equal(t, domain.CodeScheduleBlockHasOverrides, codeOf(err))
}

func TestAvailableTeachersRanksByFit(t *testing.T) {
	f := newFakeSchedule(t)
	pre := newScheduleService(f, beforeCycle())
	music := addBlock(t, pre, aMus3A, 1, at(8, 50), at(9, 40))
	addBlock(t, pre, aMat3B, 1, at(9, 0), at(9, 50)) // Ana busy at 9:00
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()

	got, err := s.AvailableTeachers(ctx, usecase.AvailabilityQuery{
		CycleID: sCycle, GroupID: gr3A, SubjectID: sMus3, Date: mon5, Start: at(8, 50), End: at(9, 40), ExcludeBlockID: music.ID(),
	})
	require.NoError(t, err)
	ids := []string{}
	for _, a := range got {
		ids = append(ids, a.Teacher.ID()+":"+string(a.Match))
	}
	// Luis (the class's own, excluded block) and Zoe teach Música (any grade,
	// any case); Eva teaches in neither; Ana is busy.
	assert.Equal(t, []string{tZoe + ":subject", tLuis + ":subject", tEva + ":other"}, ids)

	// by weekday (base timetable) with no date
	got, err = s.AvailableTeachers(ctx, usecase.AvailabilityQuery{CycleID: sCycle, GroupID: gr3A, SubjectID: sMus3, Weekday: 2, Start: at(9, 0), End: at(9, 50)})
	require.NoError(t, err)
	assert.Len(t, got, 4)

	_, err = s.AvailableTeachers(ctx, usecase.AvailabilityQuery{CycleID: sCycle, GroupID: "nope", SubjectID: sMus3, Weekday: 2, Start: at(9, 0), End: at(9, 50)})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
}

func TestBoard(t *testing.T) {
	f := newFakeSchedule(t)
	pre := newScheduleService(f, beforeCycle())
	addBlock(t, pre, aMus3A, 1, at(8, 50), at(9, 40))
	addBlock(t, pre, aEsp4A, 1, at(9, 50), at(10, 40))
	ctx := context.Background()

	b, err := pre.Board(ctx, "", "")
	require.NoError(t, err, "no cycle id: the active cycle")
	assert.Equal(t, sCycle, b.Catalog.Cycle.ID())
	assert.False(t, b.Started)
	assert.Len(t, b.Blocks, 2)
	require.Len(t, b.Grids, 1)
	assert.Equal(t, domain.DefaultGridStart, b.Grids[0].Start(), "a level with no grid saved gets the default")

	during := newScheduleService(f, duringCycle())
	b, err = during.Board(ctx, sCycle, tEva)
	require.NoError(t, err)
	assert.True(t, b.Started)
	require.Len(t, b.Blocks, 1, "narrowed to the teacher")
	assert.Equal(t, tEva, b.Blocks[0].TeacherID())

	_, err = during.Board(ctx, "nope", "")
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestDayForATeacherIncludesWhatTheyCover(t *testing.T) {
	f := newFakeSchedule(t)
	pre := newScheduleService(f, beforeCycle())
	music := addBlock(t, pre, aMus3A, 1, at(8, 50), at(9, 40))
	addBlock(t, pre, aEsp4A, 1, at(9, 50), at(10, 40))
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	_, err := s.CreateOverride(ctx, usecase.OverrideInput{BlockID: music.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: tEva, Reason: "tarde"}, director)
	require.NoError(t, err)

	day, err := s.Day(ctx, sCycle, mon5, tEva)
	require.NoError(t, err)
	assert.Len(t, day.Classes, 2)
	day, err = s.Day(ctx, sCycle, mon5, tLuis)
	require.NoError(t, err)
	require.Len(t, day.Classes, 1, "the replaced teacher still sees the class, as substituted")
	assert.Equal(t, domain.ClassSubstitute, day.Classes[0].Status)

	weekend, err := s.Day(ctx, sCycle, mon5.AddDate(0, 0, 5), "")
	require.NoError(t, err)
	assert.Empty(t, weekend.Classes)
	assert.Equal(t, 6, weekend.Weekday)
}

func TestSetGridValidates(t *testing.T) {
	f := newFakeSchedule(t)
	s := newScheduleService(f, beforeCycle())
	_, err := s.SetGrid(context.Background(), sLevel, at(8, 0), at(8, 30))
	assert.Equal(t, domain.CodeScheduleGridInvalid, codeOf(err))
	_, err = s.SetGrid(context.Background(), "nope", at(7, 0), at(15, 0))
	assert.Equal(t, glerrors.KindNotFound, glerrors.KindOf(err))
}

func TestEventsAreBounded(t *testing.T) {
	f := newFakeSchedule(t)
	s := newScheduleService(f, beforeCycle())
	_, err := s.Events(context.Background(), "", 10)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	ev, err := s.Events(context.Background(), sCycle, 0)
	require.NoError(t, err)
	assert.Empty(t, ev)
}

// staleRead hands the use case a class as it was before another editor moved
// it: the read outside the lock is stale.
type staleRead struct {
	*fakeSchedule
	stale domain.ScheduleBlock
}

func (s staleRead) GetScheduleBlock(context.Context, string) (domain.ScheduleBlock, error) {
	return s.stale, nil
}

func TestCreateOverrideRefusesAClassMovedMeanwhile(t *testing.T) {
	f := newFakeSchedule(t)
	pre := newScheduleService(f, beforeCycle())
	music := addBlock(t, pre, aMus3A, 1, at(8, 50), at(9, 40))
	_, err := pre.UpdateBlock(context.Background(), music.ID(), usecase.BlockInput{AssignmentID: aMus3A, Weekday: 1, Start: at(10, 0), End: at(10, 50)}, director)
	require.NoError(t, err)

	loc, _ := time.LoadLocation("America/Mexico_City")
	s := usecase.NewScheduleService(staleRead{fakeSchedule: f, stale: music}, "ten", func() string { return "id-x" }, duringCycle, loc)
	_, err = s.CreateOverride(context.Background(), usecase.OverrideInput{BlockID: music.ID(), Date: mon5, Kind: domain.OverrideCancel, Reason: "x"}, director)
	assert.Equal(t, domain.CodeScheduleChanged, codeOf(err))
}
