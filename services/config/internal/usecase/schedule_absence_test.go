package usecase_test

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
	"github.com/garcia-lorca/services/config/internal/usecase"
)

// --- the fake's absences and calendar (T19) ------------------------------------

func (f *fakeSchedule) GetTeacherAbsence(_ context.Context, id string) (domain.TeacherAbsence, error) {
	return f.GetAbsence(id)
}

func (f *fakeSchedule) ListTeacherAbsences(_ context.Context, _ string, from, to time.Time) ([]domain.TeacherAbsence, error) {
	var out []domain.TeacherAbsence
	for _, a := range f.absences {
		if !a.EndsOn().Before(from) && !a.StartsOn().After(to) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out, nil
}

func (f *fakeSchedule) ListNonWorkingPeriods(context.Context, string) ([]domain.NonWorkingPeriod, error) {
	return f.periods, nil
}

func (f *fakeSchedule) GetAbsence(id string) (domain.TeacherAbsence, error) {
	a, ok := f.absences[id]
	if !ok {
		return domain.TeacherAbsence{}, notFound("absence")
	}
	return a, nil
}

func (f *fakeSchedule) ActiveAbsences(_ string, from, to time.Time) ([]domain.TeacherAbsence, error) {
	all, _ := f.ListTeacherAbsences(context.Background(), sCycle, from, to)
	var out []domain.TeacherAbsence
	for _, a := range all {
		if a.Active() {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeSchedule) InsertAbsence(a domain.TeacherAbsence) error {
	f.absences[a.ID()] = a
	return nil
}
func (f *fakeSchedule) UpdateAbsence(a domain.TeacherAbsence) error {
	f.absences[a.ID()] = a
	return nil
}

func (f *fakeSchedule) closeDays(t *testing.T, label string, from, to time.Time) {
	t.Helper()
	p, err := domain.NewNonWorkingPeriod("p-"+label, "ten", sCycle, label, domain.KindHoliday, from, to)
	require.NoError(t, err)
	f.periods = append(f.periods, p)
}

// --- rig ----------------------------------------------------------------------------

var wed7 = d(2026, time.October, 7)

func fullDay(teacher string, from, to time.Time) usecase.AbsenceInput {
	return usecase.AbsenceInput{CycleID: sCycle, TeacherID: teacher, Kind: domain.AbsenceSick, StartsOn: from, EndsOn: to, AllDay: true, Note: "gripe"}
}

func partial(teacher string, on time.Time, from, to int) usecase.AbsenceInput {
	return usecase.AbsenceInput{CycleID: sCycle, TeacherID: teacher, Kind: domain.AbsencePersonal, StartsOn: on, EndsOn: on, Start: from, End: to}
}

// anaWeek: Ana gives Matemáticas 3A Monday 08:00 and 3B Wednesday 09:00; Luis
// gives Música 3A Monday 09:00.
func anaWeek(t *testing.T) (*fakeSchedule, domain.ScheduleBlock, domain.ScheduleBlock, domain.ScheduleBlock) {
	t.Helper()
	f := newFakeSchedule(t)
	pre := newScheduleService(f, beforeCycle())
	mon := addBlock(t, pre, aMat3A, 1, at(8, 0), at(8, 50))
	wed := addBlock(t, pre, aMat3B, 3, at(9, 0), at(9, 50))
	music := addBlock(t, pre, aMus3A, 1, at(9, 0), at(9, 50))
	return f, mon, wed, music
}

// --- registering ---------------------------------------------------------------------

func TestCreateAbsenceCountsTheClassesLeftUncovered(t *testing.T) {
	f, mon, _, _ := anaWeek(t)
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	// Monday's class already has a substitute: only Wednesday's stays uncovered.
	_, err := s.CreateOverride(ctx, usecase.OverrideInput{BlockID: mon.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: tZoe, Reason: "cita"}, director)
	require.NoError(t, err)

	got, err := s.CreateAbsence(ctx, fullDay(tAna, mon5, wed7), director)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Uncovered)
	assert.Equal(t, tAna, got.Absence.TeacherID())
	assert.Equal(t, director, got.Absence.CreatedBy())
	assert.True(t, got.Absence.FullDay())

	require.NotEmpty(t, f.events)
	e := f.events[0]
	assert.Equal(t, domain.ActionAbsenceCreated, e.Action)
	assert.Equal(t, got.Absence.ID(), e.AbsenceID)
	assert.Equal(t, mon5, e.Date)
	assert.Equal(t, "Ausencia registrada: Ana Ruiz, del 5 de octubre de 2026 al 7 de octubre de 2026, todo el día", e.Summary)
	assert.NotContains(t, e.Summary, "gripe", "the history is schedule:read: never the note")
	assert.Empty(t, e.Reason)
}

func TestCreateAbsenceSkipsNonWorkingDaysInTheCount(t *testing.T) {
	f, _, _, _ := anaWeek(t)
	f.closeDays(t, "Puente", wed7, wed7)
	s := newScheduleService(f, duringCycle())
	got, err := s.CreateAbsence(context.Background(), fullDay(tAna, mon5, wed7), director)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Uncovered, "Wednesday has no classes")
}

func TestCreateAbsenceRules(t *testing.T) {
	f, _, _, _ := anaWeek(t)
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	_, err := s.CreateAbsence(ctx, partial(tAna, mon5, at(8, 0), at(10, 0)), director)
	require.NoError(t, err)

	tests := []struct {
		name     string
		in       usecase.AbsenceInput
		wantCode domain.RefusalCode
		wantKind glerrors.Kind
	}{
		{"overlaps her appointment", fullDay(tAna, mon5, mon5), domain.CodeAbsenceOverlap, glerrors.KindConflict},
		{"overlapping partial", partial(tAna, mon5, at(9, 30), at(11, 0)), domain.CodeAbsenceOverlap, glerrors.KindConflict},
		{"outside the cycle", fullDay(tAna, d(2027, time.July, 8), d(2027, time.July, 12)), domain.CodeScheduleDateOutsideCycle, glerrors.KindInvalid},
		{"unknown teacher", fullDay("t-ghost", mon5, mon5), domain.CodeScheduleSubstituteInvalid, glerrors.KindInvalid},
		{"bad time", partial(tLuis, mon5, at(8, 3), at(9, 0)), domain.CodeScheduleTimeInvalid, glerrors.KindInvalid},
		{"bad kind", usecase.AbsenceInput{CycleID: sCycle, TeacherID: tLuis, Kind: "x", StartsOn: mon5, EndsOn: mon5, AllDay: true}, domain.CodeAbsenceKindInvalid, glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateAbsence(ctx, tt.in, director)
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, codeOf(err))
			assert.Equal(t, tt.wantKind, glerrors.KindOf(err))
		})
	}

	// Right after her appointment, the same day: fine.
	_, err = s.CreateAbsence(ctx, partial(tAna, mon5, at(10, 0), at(11, 0)), director)
	require.NoError(t, err)
	assert.Contains(t, glerrors.SafeMessage(func() error {
		_, e := s.CreateAbsence(ctx, fullDay(tAna, mon5, mon5), director)
		return e
	}()), "Ana Ruiz ya tiene una ausencia el 5 de octubre de 2026, de 08:00 a 10:00")
}

func TestCreateAbsenceFindsTheCycleOfItsDates(t *testing.T) {
	f, _, _, _ := anaWeek(t)
	s := newScheduleService(f, duringCycle())
	in := fullDay(tAna, mon5, mon5)
	in.CycleID = ""
	got, err := s.CreateAbsence(context.Background(), in, director)
	require.NoError(t, err)
	assert.Equal(t, sCycle, got.Absence.CycleID())

	in = fullDay(tAna, d(2028, time.January, 10), d(2028, time.January, 10))
	in.CycleID = ""
	_, err = s.CreateAbsence(context.Background(), in, director)
	assert.Equal(t, domain.CodeScheduleDateOutsideCycle, codeOf(err))
}

func TestUpdateAndCancelAbsenceKeepTheRecordAndTheHistory(t *testing.T) {
	f, _, _, _ := anaWeek(t)
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	created, err := s.CreateAbsence(ctx, fullDay(tAna, mon5, mon5), director)
	require.NoError(t, err)
	assert.Equal(t, 1, created.Uncovered)

	edited, err := s.UpdateAbsence(ctx, created.Absence.ID(), fullDay(tAna, mon5, wed7), director)
	require.NoError(t, err)
	assert.Equal(t, 2, edited.Uncovered)
	assert.Equal(t, wed7, edited.Absence.EndsOn())
	assert.Equal(t, domain.ActionAbsenceUpdated, f.events[0].Action)
	assert.Equal(t, "Ausencia cambiada: Ana Ruiz, del 5 de octubre de 2026 al 7 de octubre de 2026, todo el día (antes el 5 de octubre de 2026, todo el día)", f.events[0].Summary)

	in := fullDay(tLuis, mon5, mon5)
	_, err = s.UpdateAbsence(ctx, created.Absence.ID(), in, director)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "the teacher of an absence does not change")

	cancelled, err := s.CancelAbsence(ctx, created.Absence.ID(), director)
	require.NoError(t, err)
	assert.False(t, cancelled.Active())
	assert.Equal(t, domain.ActionAbsenceCanceled, f.events[0].Action)
	assert.Contains(t, f.events[0].Summary, "Ausencia cancelada: Ana Ruiz")
	_, stillThere := f.absences[created.Absence.ID()]
	assert.True(t, stillThere, "never deleted")

	_, err = s.CancelAbsence(ctx, created.Absence.ID(), director)
	assert.Equal(t, domain.CodeAbsenceCancelled, codeOf(err))
	_, err = s.UpdateAbsence(ctx, created.Absence.ID(), fullDay(tAna, mon5, mon5), director)
	assert.Equal(t, domain.CodeAbsenceCancelled, codeOf(err))

	day, err := s.Day(ctx, sCycle, mon5, "")
	require.NoError(t, err)
	for _, c := range day.Classes {
		assert.NotEqual(t, domain.ClassUncovered, c.Status, "a cancelled absence uncovers nothing")
	}
}

func TestAbsencesListsByDatesAndTeacher(t *testing.T) {
	f, _, _, _ := anaWeek(t)
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	ana, err := s.CreateAbsence(ctx, fullDay(tAna, mon5, mon5), director)
	require.NoError(t, err)
	_, err = s.CreateAbsence(ctx, fullDay(tLuis, wed7, wed7), director)
	require.NoError(t, err)
	_, err = s.CancelAbsence(ctx, ana.Absence.ID(), director)
	require.NoError(t, err)

	got, err := s.Absences(ctx, usecase.AbsenceQuery{CycleID: sCycle, From: mon5, To: wed7})
	require.NoError(t, err)
	assert.Len(t, got, 1, "cancelled ones only when asked")
	got, err = s.Absences(ctx, usecase.AbsenceQuery{CycleID: sCycle, From: mon5, To: wed7, IncludeCancelled: true})
	require.NoError(t, err)
	assert.Len(t, got, 2)
	got, err = s.Absences(ctx, usecase.AbsenceQuery{CycleID: sCycle, TeacherID: tLuis, From: mon5, To: wed7, IncludeCancelled: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, tLuis, got[0].TeacherID())
}

// --- the effective timetable ---------------------------------------------------------

func TestDayShowsTheAbsentTeachersClassesUncovered(t *testing.T) {
	f, mon, _, music := anaWeek(t)
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	_, err := s.CreateAbsence(ctx, partial(tAna, mon5, at(7, 0), at(8, 30)), director)
	require.NoError(t, err)

	day, err := s.Day(ctx, sCycle, mon5, "")
	require.NoError(t, err)
	status := map[string]domain.ClassStatus{}
	for _, c := range day.Classes {
		status[c.Block.ID()] = c.Status
	}
	assert.Equal(t, domain.ClassUncovered, status[mon.ID()])
	assert.Equal(t, domain.ClassRegular, status[music.ID()])

	// A substitute covers it.
	_, err = s.CreateOverride(ctx, usecase.OverrideInput{BlockID: mon.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: tZoe, Reason: "cita"}, director)
	require.NoError(t, err)
	day, err = s.Day(ctx, sCycle, mon5, "")
	require.NoError(t, err)
	for _, c := range day.Classes {
		assert.NotEqual(t, domain.ClassUncovered, c.Status)
	}
}

func TestDayOnANonWorkingDayHasNoClasses(t *testing.T) {
	f, _, _, _ := anaWeek(t)
	f.closeDays(t, "Día del maestro", mon5, mon5)
	s := newScheduleService(f, duringCycle())
	day, err := s.Day(context.Background(), sCycle, mon5, "")
	require.NoError(t, err)
	assert.Empty(t, day.Classes)
	assert.Equal(t, "Día del maestro", day.NonWorking)
}

func TestUncoveredListsEachDateSkippingClosedDays(t *testing.T) {
	f, mon, wed, _ := anaWeek(t)
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	_, err := s.CreateAbsence(ctx, fullDay(tAna, mon5, d(2026, time.October, 16)), director)
	require.NoError(t, err)
	f.closeDays(t, "Puente", d(2026, time.October, 12), d(2026, time.October, 12))

	got, err := s.Uncovered(ctx, sCycle, mon5, d(2026, time.October, 16))
	require.NoError(t, err)
	var lines []string
	for _, u := range got {
		lines = append(lines, u.Date.Format("01-02")+" "+u.Class.Block.ID())
		require.NotNil(t, u.Class.Absence)
	}
	assert.Equal(t, []string{"10-05 " + mon.ID(), "10-07 " + wed.ID(), "10-14 " + wed.ID()}, lines, "Monday the 12th is closed")

	_, err = s.Uncovered(ctx, sCycle, mon5, mon5.AddDate(0, 0, usecase.MaxUncoveredDays))
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "a bounded range")
	_, err = s.Uncovered(ctx, sCycle, wed7, mon5)
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
}

// --- substitutes and changes ---------------------------------------------------------

func TestAvailableTeachersLeaveOutTheAbsent(t *testing.T) {
	f, _, _, music := anaWeek(t)
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	_, err := s.CreateAbsence(ctx, partial(tZoe, mon5, at(9, 30), at(10, 0)), director)
	require.NoError(t, err)
	_, err = s.CreateAbsence(ctx, partial(tEva, mon5, at(11, 0), at(12, 0)), director)
	require.NoError(t, err)

	got, err := s.AvailableTeachers(ctx, usecase.AvailabilityQuery{
		CycleID: sCycle, GroupID: gr3A, SubjectID: sMus3, Date: mon5, Start: at(9, 0), End: at(9, 50), ExcludeBlockID: music.ID(),
	})
	require.NoError(t, err)
	var ids []string
	for _, a := range got {
		ids = append(ids, a.Teacher.ID())
	}
	// Zoe teaches Música but is away during the class; Eva is away only later;
	// Ana teaches in the grade and is free at 09:00.
	assert.Equal(t, []string{tAna, tEva}, ids)
}

func TestCreateOverrideRefusesAnAbsentSubstitute(t *testing.T) {
	f, _, _, music := anaWeek(t)
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	_, err := s.CreateAbsence(ctx, fullDay(tZoe, mon5, mon5), director)
	require.NoError(t, err)

	_, err = s.CreateOverride(ctx, usecase.OverrideInput{BlockID: music.ID(), Date: mon5, Kind: domain.OverrideSubstitute, TeacherID: tZoe, Reason: "junta"}, director)
	require.Error(t, err)
	assert.Equal(t, domain.CodeScheduleTeacherAbsent, codeOf(err))
	assert.Equal(t, "Zoe Alba está ausente el 5 de octubre de 2026 (todo el día)", glerrors.SafeMessage(err))
}

func TestCreateOverrideRefusesASwapThatPutsAnAbsentTeacherInClass(t *testing.T) {
	f, mon, _, music := anaWeek(t)
	s := newScheduleService(f, duringCycle())
	ctx := context.Background()
	// Ana is away 09:00–10:00: swapping her 08:00 class into Luis's 09:00 slot puts her in class while away.
	_, err := s.CreateAbsence(ctx, partial(tAna, mon5, at(9, 0), at(10, 0)), director)
	require.NoError(t, err)
	_, err = s.CreateOverride(ctx, usecase.OverrideInput{BlockID: mon.ID(), Date: mon5, Kind: domain.OverrideSwap, SwapBlockID: music.ID(), Reason: "ajuste"}, director)
	assert.Equal(t, domain.CodeScheduleTeacherAbsent, codeOf(err))

	// Moving her class OUT of her absence is what a swap is for.
	f2, mon2, _, music2 := anaWeek(t)
	s2 := newScheduleService(f2, duringCycle())
	_, err = s2.CreateAbsence(ctx, partial(tAna, mon5, at(8, 0), at(8, 50)), director)
	require.NoError(t, err)
	_, err = s2.CreateOverride(ctx, usecase.OverrideInput{BlockID: mon2.ID(), Date: mon5, Kind: domain.OverrideSwap, SwapBlockID: music2.ID(), Reason: "cita"}, director)
	require.NoError(t, err)
}

func TestCreateOverrideRefusesANonWorkingDay(t *testing.T) {
	f, _, _, music := anaWeek(t)
	f.closeDays(t, "Independencia", mon5, mon5)
	s := newScheduleService(f, duringCycle())
	_, err := s.CreateOverride(context.Background(), usecase.OverrideInput{BlockID: music.ID(), Date: mon5, Kind: domain.OverrideCancel, Reason: "x"}, director)
	assert.Equal(t, domain.CodeScheduleDateNonWorking, codeOf(err))
	assert.Equal(t, "El 5 de octubre de 2026 no hay clases (Independencia)", glerrors.SafeMessage(err))
}
