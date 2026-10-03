package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

var director = domain.Actor{UserID: "u1", Name: "Directora"}

// absence builds an active absence of teacher tc; span nil is a full day.
func absence(t *testing.T, id, tc string, from, to time.Time, span *domain.ClassTime) domain.TeacherAbsence {
	t.Helper()
	a, err := domain.NewTeacherAbsence(domain.AbsenceFields{
		ID: id, TenantID: "ten", CycleID: "c1", TeacherID: tc, Kind: domain.AbsenceSick,
		StartsOn: from, EndsOn: to, Span: span, Note: "",
	}, director, day(2026, time.October, 1))
	require.NoError(t, err)
	return a
}

func span(t *testing.T, from, to int) *domain.ClassTime {
	ct := classTime(t, from, to)
	return &ct
}

func TestNewTeacherAbsence(t *testing.T) {
	long := make([]rune, domain.MaxAbsenceNoteLen+1)
	for i := range long {
		long[i] = 'a'
	}
	tests := []struct {
		name     string
		mutate   func(*domain.AbsenceFields)
		wantCode domain.RefusalCode
		wantKind glerrors.Kind
	}{
		{"a full day", func(*domain.AbsenceFields) {}, "", glerrors.KindUnknown},
		{"a partial day", func(f *domain.AbsenceFields) { ct, _ := domain.NewClassTime(hm(8, 0), hm(10, 0)); f.Span = &ct }, "", glerrors.KindUnknown},
		{"several days", func(f *domain.AbsenceFields) { f.EndsOn = monday.AddDate(0, 0, 4) }, "", glerrors.KindUnknown},
		{"unknown kind", func(f *domain.AbsenceFields) { f.Kind = "holiday" }, domain.CodeAbsenceKindInvalid, glerrors.KindInvalid},
		{"ends before it starts", func(f *domain.AbsenceFields) { f.EndsOn = monday.AddDate(0, 0, -1) }, domain.CodeAbsenceDatesInvalid, glerrors.KindInvalid},
		{"no start date", func(f *domain.AbsenceFields) { f.StartsOn = time.Time{} }, domain.CodeAbsenceDatesInvalid, glerrors.KindInvalid},
		{"longer than a year", func(f *domain.AbsenceFields) { f.EndsOn = monday.AddDate(1, 0, 1) }, domain.CodeAbsenceDatesInvalid, glerrors.KindInvalid},
		{"note too long", func(f *domain.AbsenceFields) { f.Note = string(long) }, domain.CodeAbsenceNoteTooLong, glerrors.KindInvalid},
		{"no teacher", func(f *domain.AbsenceFields) { f.TeacherID = " " }, "", glerrors.KindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := domain.AbsenceFields{ID: "a1", TenantID: "ten", CycleID: "c1", TeacherID: "ana", Kind: domain.AbsenceSick, StartsOn: monday, EndsOn: monday}
			tt.mutate(&f)
			a, err := domain.NewTeacherAbsence(f, director, day(2026, time.October, 1))
			if tt.wantKind == glerrors.KindUnknown {
				require.NoError(t, err)
				assert.True(t, a.Active())
				return
			}
			require.Error(t, err)
			assert.Equal(t, tt.wantKind, glerrors.KindOf(err))
			if tt.wantCode != "" {
				assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(err))
			}
		})
	}
}

func TestNewTeacherAbsenceTrimsTheNoteAndKeepsOnlyTheDate(t *testing.T) {
	a, err := domain.NewTeacherAbsence(domain.AbsenceFields{
		ID: "a1", TenantID: "ten", CycleID: "c1", TeacherID: "ana", Kind: domain.AbsenceVacation,
		StartsOn: monday.Add(15 * time.Hour), EndsOn: monday.Add(20 * time.Hour), Note: "  cita  ",
	}, director, day(2026, time.October, 1))
	require.NoError(t, err)
	assert.Equal(t, "cita", a.Note())
	assert.Equal(t, monday, a.StartsOn())
	assert.Equal(t, monday, a.EndsOn())
	assert.True(t, a.FullDay())
	assert.Equal(t, "Vacaciones", a.Kind().Label())
}

func TestAbsenceKindLabels(t *testing.T) {
	assert.Equal(t, "Incapacidad", domain.AbsenceSick.Label())
	assert.Equal(t, "Vacaciones", domain.AbsenceVacation.Label())
	assert.Equal(t, "Asunto personal", domain.AbsencePersonal.Label())
	assert.Equal(t, "Otro", domain.AbsenceOther.Label())
}

// Covers is THE rule "is teacher T absent at date D, time range R".
func TestTeacherAbsenceCovers(t *testing.T) {
	wed := monday.AddDate(0, 0, 2)
	tests := []struct {
		name    string
		absence func(t *testing.T) domain.TeacherAbsence
		date    time.Time
		from    int
		to      int
		want    bool
	}{
		{"full day covers any class that day", func(t *testing.T) domain.TeacherAbsence { return absence(t, "a", "ana", monday, monday, nil) }, monday, hm(8, 0), hm(8, 50), true},
		{"full day does not cover the next day", func(t *testing.T) domain.TeacherAbsence { return absence(t, "a", "ana", monday, monday, nil) }, monday.AddDate(0, 0, 1), hm(8, 0), hm(8, 50), false},
		{"full day does not cover the day before", func(t *testing.T) domain.TeacherAbsence { return absence(t, "a", "ana", monday, monday, nil) }, monday.AddDate(0, 0, -1), hm(8, 0), hm(8, 50), false},
		{"multi-day covers the middle", func(t *testing.T) domain.TeacherAbsence {
			return absence(t, "a", "ana", monday, monday.AddDate(0, 0, 4), nil)
		}, wed, hm(12, 0), hm(12, 50), true},
		{"multi-day covers the last day (inclusive)", func(t *testing.T) domain.TeacherAbsence { return absence(t, "a", "ana", monday, wed, nil) }, wed, hm(8, 0), hm(8, 50), true},
		{"partial covers an overlapping class", func(t *testing.T) domain.TeacherAbsence {
			return absence(t, "a", "ana", monday, monday, span(t, hm(8, 0), hm(10, 0)))
		}, monday, hm(9, 30), hm(10, 20), true},
		{"partial does not cover a class right after (half-open)", func(t *testing.T) domain.TeacherAbsence {
			return absence(t, "a", "ana", monday, monday, span(t, hm(8, 0), hm(10, 0)))
		}, monday, hm(10, 0), hm(10, 50), false},
		{"partial does not cover a class right before", func(t *testing.T) domain.TeacherAbsence {
			return absence(t, "a", "ana", monday, monday, span(t, hm(8, 0), hm(10, 0)))
		}, monday, hm(7, 10), hm(8, 0), false},
		{"partial applies to every day of the span", func(t *testing.T) domain.TeacherAbsence {
			return absence(t, "a", "ana", monday, wed, span(t, hm(8, 0), hm(10, 0)))
		}, wed, hm(8, 0), hm(8, 50), true},
		{"cancelled covers nothing", func(t *testing.T) domain.TeacherAbsence {
			a := absence(t, "a", "ana", monday, monday, nil)
			c, err := a.Cancel(director, day(2026, time.October, 2))
			require.NoError(t, err)
			return c
		}, monday, hm(8, 0), hm(8, 50), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.absence(t).Covers(tt.date, classTime(t, tt.from, tt.to)))
		})
	}
}

func TestAbsentAtPicksTheTeachersActiveAbsence(t *testing.T) {
	cancelled, err := absence(t, "x", "ana", monday, monday, nil).Cancel(director, day(2026, time.October, 2))
	require.NoError(t, err)
	set := []domain.TeacherAbsence{
		cancelled,
		absence(t, "luis", "luis", monday, monday, nil),
		absence(t, "ana-am", "ana", monday, monday, span(t, hm(8, 0), hm(10, 0))),
	}
	got, ok := domain.AbsentAt(set, "ana", monday, classTime(t, hm(9, 0), hm(9, 50)))
	require.True(t, ok)
	assert.Equal(t, "ana-am", got.ID())

	_, ok = domain.AbsentAt(set, "ana", monday, classTime(t, hm(11, 0), hm(11, 50)))
	assert.False(t, ok, "the cancelled full day does not count")
}

func TestTeacherAbsenceOverlaps(t *testing.T) {
	tests := []struct {
		name string
		a, b func(t *testing.T) domain.TeacherAbsence
		want bool
	}{
		{"same day, both full", func(t *testing.T) domain.TeacherAbsence { return absence(t, "a", "ana", monday, monday, nil) },
			func(t *testing.T) domain.TeacherAbsence { return absence(t, "b", "ana", monday, monday, nil) }, true},
		{"different teachers", func(t *testing.T) domain.TeacherAbsence { return absence(t, "a", "ana", monday, monday, nil) },
			func(t *testing.T) domain.TeacherAbsence { return absence(t, "b", "luis", monday, monday, nil) }, false},
		{"dates apart", func(t *testing.T) domain.TeacherAbsence { return absence(t, "a", "ana", monday, monday, nil) },
			func(t *testing.T) domain.TeacherAbsence {
				return absence(t, "b", "ana", monday.AddDate(0, 0, 1), monday.AddDate(0, 0, 1), nil)
			}, false},
		{"full day against a partial the same day", func(t *testing.T) domain.TeacherAbsence { return absence(t, "a", "ana", monday, monday, nil) },
			func(t *testing.T) domain.TeacherAbsence {
				return absence(t, "b", "ana", monday, monday, span(t, hm(8, 0), hm(9, 0)))
			}, true},
		{"two partials that do not touch", func(t *testing.T) domain.TeacherAbsence {
			return absence(t, "a", "ana", monday, monday, span(t, hm(8, 0), hm(9, 0)))
		},
			func(t *testing.T) domain.TeacherAbsence {
				return absence(t, "b", "ana", monday, monday, span(t, hm(9, 0), hm(10, 0)))
			}, false},
		{"two partials that overlap", func(t *testing.T) domain.TeacherAbsence {
			return absence(t, "a", "ana", monday, monday, span(t, hm(8, 0), hm(9, 30)))
		},
			func(t *testing.T) domain.TeacherAbsence {
				return absence(t, "b", "ana", monday, monday, span(t, hm(9, 0), hm(10, 0)))
			}, true},
		{"a span that contains the other", func(t *testing.T) domain.TeacherAbsence {
			return absence(t, "a", "ana", monday, monday.AddDate(0, 0, 4), nil)
		},
			func(t *testing.T) domain.TeacherAbsence {
				return absence(t, "b", "ana", monday.AddDate(0, 0, 2), monday.AddDate(0, 0, 2), nil)
			}, true},
		{"the same absence never overlaps itself", func(t *testing.T) domain.TeacherAbsence { return absence(t, "a", "ana", monday, monday, nil) },
			func(t *testing.T) domain.TeacherAbsence { return absence(t, "a", "ana", monday, monday, nil) }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := tt.a(t), tt.b(t)
			assert.Equal(t, tt.want, a.Overlaps(b))
			assert.Equal(t, tt.want, b.Overlaps(a), "symmetric")
		})
	}
}

func TestTeacherAbsenceEditAndCancel(t *testing.T) {
	a := absence(t, "a", "ana", monday, monday, nil)
	edited, err := a.Edit(domain.AbsenceChanges{Kind: domain.AbsencePersonal, StartsOn: monday, EndsOn: monday.AddDate(0, 0, 1),
		Span: span(t, hm(8, 0), hm(10, 0)), Note: "trámite"}, day(2026, time.October, 2))
	require.NoError(t, err)
	assert.Equal(t, domain.AbsencePersonal, edited.Kind())
	assert.False(t, edited.FullDay())
	assert.Equal(t, "trámite", edited.Note())
	assert.Equal(t, a.CreatedBy(), edited.CreatedBy(), "who registered it does not change")

	_, err = a.Edit(domain.AbsenceChanges{Kind: "x", StartsOn: monday, EndsOn: monday}, day(2026, time.October, 2))
	assert.Equal(t, domain.CodeAbsenceKindInvalid, domain.RefusalCodeOf(err))

	cancelled, err := a.Cancel(director, day(2026, time.October, 3))
	require.NoError(t, err)
	assert.False(t, cancelled.Active())
	assert.Equal(t, director, cancelled.CancelledBy())

	_, err = cancelled.Cancel(director, day(2026, time.October, 3))
	assert.Equal(t, domain.CodeAbsenceCancelled, domain.RefusalCodeOf(err))
	_, err = cancelled.Edit(domain.AbsenceChanges{Kind: domain.AbsenceSick, StartsOn: monday, EndsOn: monday}, day(2026, time.October, 3))
	assert.Equal(t, domain.CodeAbsenceCancelled, domain.RefusalCodeOf(err))
}

// MarkUncovered turns the classes of an absent teacher "Sin cubrir" on the
// effective timetable; a substitute or a cancellation covers them.
func TestMarkUncovered(t *testing.T) {
	blocks := []domain.ScheduleBlock{
		block(t, "b1", "gA", "math", "ana", 1, hm(8, 0), hm(8, 50)),
		block(t, "b2", "gB", "math", "ana", 1, hm(9, 0), hm(9, 50)),
		block(t, "b3", "gA", "music", "luis", 1, hm(9, 0), hm(9, 50)),
		block(t, "b4", "gB", "music", "ana", 1, hm(11, 0), hm(11, 50)),
		block(t, "b5", "gA", "art", "ana", 1, hm(12, 0), hm(12, 50)),
	}
	overrides := []domain.ScheduleOverride{
		override(t, "o1", "b2", monday, domain.OverrideSubstitute, "luis2", ""),
		override(t, "o2", "b4", monday, domain.OverrideCancel, "", ""),
	}
	absences := []domain.TeacherAbsence{absence(t, "abs", "ana", monday, monday, span(t, hm(7, 0), hm(12, 0)))}
	day := domain.MarkUncovered(domain.EffectiveDay(blocks, overrides, monday), absences, monday)

	status := map[string]domain.ClassStatus{}
	for _, c := range day {
		status[c.Block.ID()] = c.Status
		if c.Status == domain.ClassUncovered {
			require.NotNil(t, c.Absence)
			assert.Equal(t, "abs", c.Absence.ID())
		}
	}
	assert.Equal(t, domain.ClassUncovered, status["b1"], "Ana's class inside her absence")
	assert.Equal(t, domain.ClassSubstitute, status["b2"], "covered by a substitute")
	assert.Equal(t, domain.ClassRegular, status["b3"], "another teacher")
	assert.Equal(t, domain.ClassCancelled, status["b4"], "cancelled for that date")
	assert.Equal(t, domain.ClassRegular, status["b5"], "after her absence ends")

	// A substitute who is absent leaves the class uncovered too.
	away := []domain.TeacherAbsence{absence(t, "abs2", "luis2", monday, monday, nil)}
	for _, c := range domain.MarkUncovered(domain.EffectiveDay(blocks, overrides, monday), away, monday) {
		if c.Block.ID() == "b2" {
			assert.Equal(t, domain.ClassUncovered, c.Status)
		}
	}
}

func TestAbsenceSummaryNeverNamesTheKindOrTheNote(t *testing.T) {
	a, err := domain.NewTeacherAbsence(domain.AbsenceFields{
		ID: "a1", TenantID: "ten", CycleID: "c1", TeacherID: "ana", Kind: domain.AbsenceSick,
		StartsOn: monday, EndsOn: monday.AddDate(0, 0, 2), Note: "gripe",
	}, director, day(2026, time.October, 1))
	require.NoError(t, err)
	got := domain.AbsenceSummary("Ausencia registrada", a, "Ana Ruiz")
	assert.Equal(t, "Ausencia registrada: Ana Ruiz, del 5 de octubre de 2026 al 7 de octubre de 2026, todo el día", got)
	assert.NotContains(t, got, "gripe")
	assert.NotContains(t, got, "Incapacidad")

	one := absence(t, "a2", "ana", monday, monday, span(t, hm(8, 0), hm(10, 0)))
	assert.Equal(t, "Ausencia cancelada: Ana Ruiz, el 5 de octubre de 2026, de 08:00 a 10:00", domain.AbsenceSummary("Ausencia cancelada", one, "Ana Ruiz"))
}

func TestTeacherAbsentError(t *testing.T) {
	err := domain.TeacherAbsentError("Ana Ruiz", absence(t, "a", "ana", monday, monday, nil), monday)
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, domain.CodeScheduleTeacherAbsent, domain.RefusalCodeOf(err))
	assert.Equal(t, "Ana Ruiz está ausente el 5 de octubre de 2026 (todo el día)", glerrors.SafeMessage(err))
}
