package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/config/internal/domain"
)

func hm(h, m int) int { return h*60 + m }

func classTime(t *testing.T, from, to int) domain.ClassTime {
	t.Helper()
	ct, err := domain.NewClassTime(from, to)
	require.NoError(t, err)
	return ct
}

// block builds a class of the base timetable: group g, subject s, teacher tc.
func block(t *testing.T, id, g, s, tc string, weekday, from, to int) domain.ScheduleBlock {
	t.Helper()
	b, err := domain.NewScheduleBlock(id, "ten", "c1", "a-"+id, g, s, tc, weekday, classTime(t, from, to))
	require.NoError(t, err)
	return b
}

func override(t *testing.T, id, blockID string, on time.Time, kind domain.OverrideKind, teacherID, swapID string) domain.ScheduleOverride {
	t.Helper()
	o, err := domain.NewScheduleOverride(id, "ten", "c1", blockID, on, kind, teacherID, swapID, "motivo",
		domain.Actor{UserID: "u1", Name: "Directora"}, day(2026, time.October, 1))
	require.NoError(t, err)
	return o
}

// monday is 2026-10-05, a Monday.
var monday = day(2026, time.October, 5)

func TestNewClassTime(t *testing.T) {
	tests := []struct {
		name     string
		from, to int
		ok       bool
	}{
		{"a 50-minute class", hm(8, 0), hm(8, 50), true},
		{"until midnight", hm(23, 0), 24 * 60, true},
		{"not a 5-minute step", hm(8, 3), hm(8, 50), false},
		{"end not a 5-minute step", hm(8, 0), hm(8, 52), false},
		{"ends before it starts", hm(9, 0), hm(8, 0), false},
		{"zero length", hm(9, 0), hm(9, 0), false},
		{"negative", -5, hm(8, 0), false},
		{"past midnight", hm(23, 0), 24*60 + 5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewClassTime(tt.from, tt.to)
			if tt.ok {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, domain.CodeScheduleTimeInvalid, domain.RefusalCodeOf(err))
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
		})
	}
}

func TestClassTimeOverlapsIsHalfOpen(t *testing.T) {
	first := classTime(t, hm(8, 0), hm(8, 50))
	assert.False(t, first.Overlaps(classTime(t, hm(8, 50), hm(9, 40))), "back to back classes do not collide")
	assert.True(t, first.Overlaps(classTime(t, hm(8, 45), hm(9, 40))))
	assert.True(t, first.Overlaps(classTime(t, hm(7, 0), hm(9, 0))), "containing")
	assert.True(t, first.Overlaps(classTime(t, hm(8, 10), hm(8, 20))), "contained")
	assert.Equal(t, "08:00–08:50", first.String())
}

func TestScheduleGrid(t *testing.T) {
	g := domain.DefaultScheduleGrid("lvl")
	assert.Equal(t, hm(7, 0), g.Start())
	assert.Equal(t, hm(15, 0), g.End())
	assert.True(t, g.Contains(classTime(t, hm(7, 0), hm(7, 50))))
	assert.True(t, g.Contains(classTime(t, hm(14, 10), hm(15, 0))))
	assert.False(t, g.Contains(classTime(t, hm(6, 55), hm(7, 50))))
	assert.False(t, g.Contains(classTime(t, hm(14, 30), hm(15, 5))))

	for _, bad := range [][2]int{{hm(8, 0), hm(8, 30)}, {hm(8, 3), hm(14, 0)}, {hm(15, 0), hm(7, 0)}, {-5, hm(9, 0)}} {
		_, err := domain.NewScheduleGrid("lvl", bad[0], bad[1])
		require.Error(t, err, "%v", bad)
		assert.Equal(t, domain.CodeScheduleGridInvalid, domain.RefusalCodeOf(err))
	}
	_, err := domain.NewScheduleGrid("", hm(7, 0), hm(15, 0))
	require.Error(t, err)
}

func TestNewScheduleBlockWeekdays(t *testing.T) {
	ct := classTime(t, hm(8, 0), hm(8, 50))
	for _, wd := range []int{0, 6, 7} {
		_, err := domain.NewScheduleBlock("b", "ten", "c1", "a", "g", "s", "tc", wd, ct)
		require.Error(t, err)
		assert.Equal(t, domain.CodeScheduleWeekdayInvalid, domain.RefusalCodeOf(err))
	}
	b, err := domain.NewScheduleBlock("b", "ten", "c1", "a", "g", "s", "tc", 5, ct)
	require.NoError(t, err)
	assert.Equal(t, 5, b.Weekday())
	_, err = domain.NewScheduleBlock("b", "ten", "c1", "", "g", "s", "tc", 1, ct)
	require.Error(t, err, "a class always comes from an assignment")
}

func TestWeekdayOf(t *testing.T) {
	assert.Equal(t, 1, domain.WeekdayOf(monday))
	assert.Equal(t, 5, domain.WeekdayOf(monday.AddDate(0, 0, 4)))
	assert.Equal(t, 6, domain.WeekdayOf(monday.AddDate(0, 0, 5)))
	assert.Equal(t, 7, domain.WeekdayOf(monday.AddDate(0, 0, 6)))
}

func TestNewScheduleOverrideValidates(t *testing.T) {
	by := domain.Actor{UserID: "u1", Name: "Directora"}
	tests := []struct {
		name     string
		kind     domain.OverrideKind
		teacher  string
		swap     string
		reason   string
		wantCode domain.RefusalCode
	}{
		{"substitute needs a teacher", domain.OverrideSubstitute, "", "", "tarde", domain.CodeScheduleSubstituteInvalid},
		{"swap needs another class", domain.OverrideSwap, "", "", "tarde", domain.CodeScheduleSwapInvalid},
		{"swap with itself", domain.OverrideSwap, "", "b1", "tarde", domain.CodeScheduleSwapInvalid},
		{"reason required", domain.OverrideCancel, "", "", "   ", domain.CodeScheduleReasonRequired},
		{"unknown kind", domain.OverrideKind("move"), "", "", "x", domain.CodeScheduleOverrideKindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewScheduleOverride("o", "ten", "c1", "b1", monday, tt.kind, tt.teacher, tt.swap, tt.reason, by, monday)
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(err))
		})
	}
	long := make([]byte, domain.MaxScheduleReasonLen+1)
	for i := range long {
		long[i] = 'a'
	}
	_, err := domain.NewScheduleOverride("o", "ten", "c1", "b1", monday, domain.OverrideCancel, "", "", string(long), by, monday)
	require.Error(t, err)

	o, err := domain.NewScheduleOverride("o", "ten", "c1", "b1", monday, domain.OverrideCancel, "tc9", "b2", " Junta ", by, monday)
	require.NoError(t, err)
	assert.Equal(t, "", o.TeacherID(), "a cancel carries no teacher")
	assert.Equal(t, "", o.SwapBlockID(), "nor another class")
	assert.Equal(t, "Junta", o.Reason())
	assert.True(t, o.Touches("b1"))
	assert.False(t, o.Touches("b2"))

	sw := override(t, "o2", "b1", monday, domain.OverrideSwap, "", "b2")
	assert.True(t, sw.Touches("b2"), "a swap touches both classes")
}

func TestEffectiveDay(t *testing.T) {
	math := block(t, "math", "3A", "mat", "ana", 1, hm(8, 0), hm(8, 50))
	music := block(t, "music", "3A", "mus", "luis", 1, hm(8, 50), hm(9, 40))
	spanish := block(t, "spa", "4A", "esp", "eva", 1, hm(8, 0), hm(8, 50))
	tuesday := block(t, "tue", "3A", "mat", "ana", 2, hm(8, 0), hm(8, 50))
	blocks := []domain.ScheduleBlock{music, math, spanish, tuesday}

	t.Run("base only, that weekday, by time then group", func(t *testing.T) {
		day := domain.EffectiveDay(blocks, nil, monday)
		require.Len(t, day, 3)
		assert.Equal(t, []string{"math", "spa", "music"}, blockIDs(day))
		for _, c := range day {
			assert.Equal(t, domain.ClassRegular, c.Status)
		}
	})

	t.Run("weekend has no classes", func(t *testing.T) {
		assert.Empty(t, domain.EffectiveDay(blocks, nil, monday.AddDate(0, 0, 5)))
	})

	t.Run("substitute", func(t *testing.T) {
		o := override(t, "o1", "music", monday, domain.OverrideSubstitute, "eva", "")
		day := domain.EffectiveDay(blocks, []domain.ScheduleOverride{o}, monday)
		c := classOf(t, day, "music")
		assert.Equal(t, "eva", c.TeacherID)
		assert.Equal(t, "luis", c.Block.TeacherID())
		assert.Equal(t, domain.ClassSubstitute, c.Status)
		assert.Equal(t, "o1", c.Override.ID())
	})

	t.Run("an override of another date changes nothing", func(t *testing.T) {
		o := override(t, "o1", "music", monday.AddDate(0, 0, 7), domain.OverrideSubstitute, "eva", "")
		day := domain.EffectiveDay(blocks, []domain.ScheduleOverride{o}, monday)
		assert.Equal(t, "luis", classOf(t, day, "music").TeacherID)
	})

	t.Run("swap trades the times of both classes", func(t *testing.T) {
		o := override(t, "o1", "math", monday, domain.OverrideSwap, "", "music")
		day := domain.EffectiveDay(blocks, []domain.ScheduleOverride{o}, monday)
		m, mu := classOf(t, day, "math"), classOf(t, day, "music")
		assert.Equal(t, hm(8, 50), m.Time.Start())
		assert.Equal(t, hm(8, 0), mu.Time.Start())
		assert.Equal(t, domain.ClassSwapped, m.Status)
		assert.Equal(t, domain.ClassSwapped, mu.Status)
		assert.Equal(t, []string{"music", "spa", "math"}, blockIDs(day), "sorted by the times of the day")
	})

	t.Run("cancel keeps the class, struck", func(t *testing.T) {
		o := override(t, "o1", "spa", monday, domain.OverrideCancel, "", "")
		day := domain.EffectiveDay(blocks, []domain.ScheduleOverride{o}, monday)
		assert.Equal(t, domain.ClassCancelled, classOf(t, day, "spa").Status)
	})
}

func TestFindClash(t *testing.T) {
	math := block(t, "math", "3A", "mat", "ana", 1, hm(8, 0), hm(8, 50))
	music := block(t, "music", "3A", "mus", "luis", 1, hm(8, 50), hm(9, 40))
	spanish := block(t, "spa", "4A", "esp", "eva", 1, hm(8, 0), hm(8, 50))
	day := domain.EffectiveDay([]domain.ScheduleBlock{math, music, spanish}, nil, monday)

	t.Run("teacher in two groups at once", func(t *testing.T) {
		cand := domain.RegularClass(block(t, "new", "4B", "mat", "ana", 1, hm(8, 30), hm(9, 0)))
		other, kind, found := domain.FindClash(day, cand)
		require.True(t, found)
		assert.Equal(t, domain.ClashTeacher, kind)
		assert.Equal(t, "math", other.Block.ID())
	})
	t.Run("group with two classes at once", func(t *testing.T) {
		cand := domain.RegularClass(block(t, "new", "3A", "ing", "zoe", 1, hm(9, 0), hm(9, 30)))
		other, kind, found := domain.FindClash(day, cand)
		require.True(t, found)
		assert.Equal(t, domain.ClashGroup, kind)
		assert.Equal(t, "music", other.Block.ID())
	})
	t.Run("back to back is fine", func(t *testing.T) {
		cand := domain.RegularClass(block(t, "new", "4A", "ing", "ana", 1, hm(8, 50), hm(9, 40)))
		_, _, found := domain.FindClash(day, cand)
		assert.False(t, found)
	})
	t.Run("the class itself is not a clash", func(t *testing.T) {
		moved := domain.RegularClass(block(t, "math", "3A", "mat", "ana", 1, hm(8, 10), hm(8, 50)))
		_, _, found := domain.FindClash(day, moved)
		assert.False(t, found)
	})
	t.Run("a cancelled class frees the slot", func(t *testing.T) {
		o := override(t, "o1", "spa", monday, domain.OverrideCancel, "", "")
		withCancel := domain.EffectiveDay([]domain.ScheduleBlock{math, music, spanish}, []domain.ScheduleOverride{o}, monday)
		cand := domain.RegularClass(block(t, "new", "5A", "ing", "eva", 1, hm(8, 0), hm(8, 50)))
		_, _, found := domain.FindClash(withCancel, cand)
		assert.False(t, found)
	})
	t.Run("a substitute is busy at the class they cover", func(t *testing.T) {
		o := override(t, "o1", "music", monday, domain.OverrideSubstitute, "eva", "")
		covered := domain.EffectiveDay([]domain.ScheduleBlock{math, music, spanish}, []domain.ScheduleOverride{o}, monday)
		cand := domain.RegularClass(block(t, "new", "5A", "ing", "eva", 1, hm(9, 0), hm(9, 30)))
		other, kind, found := domain.FindClash(covered, cand)
		require.True(t, found)
		assert.Equal(t, domain.ClashTeacher, kind)
		assert.Equal(t, "music", other.Block.ID())
	})
}

func TestFirstClashInDay(t *testing.T) {
	math := block(t, "math", "3A", "mat", "ana", 1, hm(8, 0), hm(8, 50))
	spanish := block(t, "spa", "4A", "esp", "eva", 1, hm(8, 0), hm(8, 50))
	o := override(t, "o1", "spa", monday, domain.OverrideSubstitute, "ana", "")
	day := domain.EffectiveDay([]domain.ScheduleBlock{math, spanish}, []domain.ScheduleOverride{o}, monday)
	a, b, kind, found := domain.FirstClashInDay(day)
	require.True(t, found)
	assert.Equal(t, domain.ClashTeacher, kind)
	assert.ElementsMatch(t, []string{"math", "spa"}, []string{a.Block.ID(), b.Block.ID()})

	_, _, _, found = domain.FirstClashInDay(domain.EffectiveDay([]domain.ScheduleBlock{math, spanish}, nil, monday))
	assert.False(t, found)
}

type fakeNames struct{}

func (fakeNames) Teacher(id string) string {
	return map[string]string{"ana": "Ana Ruiz", "luis": "Luis Pérez", "eva": "Eva Soto"}[id]
}
func (fakeNames) Subject(id string) string {
	return map[string]string{"mat": "Matemáticas", "mus": "Música", "esp": "Español"}[id]
}
func (fakeNames) Group(id string) string {
	return map[string]string{"3A": "3.º A", "4A": "4.º A"}[id]
}

func TestClashErrorNamesTheConflictingClass(t *testing.T) {
	math := domain.RegularClass(block(t, "math", "3A", "mat", "ana", 1, hm(8, 0), hm(8, 50)))
	err := domain.ClashError(domain.ClashTeacher, math, fakeNames{})
	assert.Equal(t, domain.CodeScheduleTeacherBusy, domain.RefusalCodeOf(err))
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Equal(t, "Ana Ruiz ya da Matemáticas a 3.º A el lunes de 08:00 a 08:50", glerrors.SafeMessage(err))

	err = domain.ClashError(domain.ClashGroup, math, fakeNames{})
	assert.Equal(t, domain.CodeScheduleGroupBusy, domain.RefusalCodeOf(err))
	assert.Equal(t, "3.º A ya tiene Matemáticas con Ana Ruiz el lunes de 08:00 a 08:50", glerrors.SafeMessage(err))
}

func TestOverrideConflictError(t *testing.T) {
	math := domain.RegularClass(block(t, "math", "3A", "mat", "ana", 1, hm(8, 0), hm(8, 50)))
	err := domain.OverrideConflictError(monday, domain.ClashTeacher, math, fakeNames{})
	assert.Equal(t, domain.CodeScheduleOverrideConflict, domain.RefusalCodeOf(err))
	assert.Equal(t, glerrors.KindConflict, glerrors.KindOf(err))
	assert.Contains(t, glerrors.SafeMessage(err), "5 de octubre de 2026")
	assert.Contains(t, glerrors.SafeMessage(err), "Ana Ruiz ya da Matemáticas a 3.º A")
}

func TestRankAvailableTeachers(t *testing.T) {
	mk := func(id, first, last string) domain.Teacher {
		tc, err := domain.NewTeacher(id, "ten", first, last, id+"@x.mx", "")
		require.NoError(t, err)
		return tc
	}
	ana, luis, eva, zoe, bob := mk("ana", "Ana", "Ruiz"), mk("luis", "Luis", "Pérez"), mk("eva", "Eva", "Soto"), mk("zoe", "Zoe", "Alba"), mk("bob", "Bob", "Zúñiga")
	asg := func(id, group, subject, teacher string) domain.TeachingAssignment {
		a, err := domain.NewTeachingAssignment(id, "ten", "c1", group, subject, teacher)
		require.NoError(t, err)
		return a
	}
	assignments := []domain.TeachingAssignment{
		asg("a1", "3B", "mus-3", "eva"),   // teaches Música in another grade
		asg("a2", "3A", "mat-3", "zoe"),   // teaches in the same grade (3A)
		asg("a3", "5A", "esp-5", "bob"),   // neither
		asg("a4", "3A", "mus-3a", "luis"), // the class's own teacher
	}
	math := block(t, "math", "3A", "mat-3", "ana", 1, hm(8, 0), hm(8, 50))
	busy := block(t, "busy", "5A", "esp-5", "bob", 1, hm(9, 0), hm(9, 50))
	day := domain.EffectiveDay([]domain.ScheduleBlock{math, busy}, nil, monday)

	sameSubject := func(subjectID string) bool { return subjectID == "mus-3" || subjectID == "mus-3a" }
	sameGrade := func(groupID string) bool { return groupID == "3A" || groupID == "3B" }
	slot := classTime(t, hm(9, 0), hm(9, 50))

	got := domain.AvailableTeachers([]domain.Teacher{ana, luis, eva, zoe, bob}, day, slot, "", assignments, sameSubject, sameGrade)
	ids := make([]string, 0, len(got))
	for _, a := range got {
		ids = append(ids, a.Teacher.ID()+":"+string(a.Match))
	}
	// bob teaches at 9:00 → out. eva and luis teach Música (by name); zoe and
	// eva teach in grade 3; ana nothing. Within a tier, by last name.
	assert.Equal(t, []string{"luis:subject", "eva:subject", "zoe:grade", "ana:other"}, ids)

	// excluding the busy class frees its teacher
	got = domain.AvailableTeachers([]domain.Teacher{bob}, day, slot, "busy", assignments, sameSubject, sameGrade)
	require.Len(t, got, 1)
}

func TestScheduleSummaries(t *testing.T) {
	math := block(t, "math", "3A", "mat", "ana", 1, hm(8, 0), hm(8, 50))
	music := block(t, "music", "3A", "mus", "luis", 1, hm(8, 50), hm(9, 40))
	n := fakeNames{}
	assert.Equal(t, "Clase nueva: Matemáticas, 3.º A, lunes 08:00–08:50, Ana Ruiz", domain.BlockSummary("Clase nueva", math, n))
	moved := block(t, "math", "3A", "mat", "ana", 2, hm(9, 0), hm(9, 50))
	assert.Equal(t, "Clase cambiada: Matemáticas, 3.º A, lunes 08:00–08:50, Ana Ruiz → martes 09:00–09:50, Ana Ruiz",
		domain.BlockChangeSummary(math, moved, n))

	sub := override(t, "o1", "music", monday, domain.OverrideSubstitute, "eva", "")
	assert.Equal(t, "Suplencia el 5 de octubre de 2026: Música, 3.º A, 08:50–09:40 — Eva Soto en lugar de Luis Pérez",
		domain.OverrideSummary(sub, music, nil, n))
	sw := override(t, "o2", "math", monday, domain.OverrideSwap, "", "music")
	assert.Equal(t, "Intercambio el 5 de octubre de 2026, 3.º A: Matemáticas (08:00–08:50) ↔ Música (08:50–09:40)",
		domain.OverrideSummary(sw, math, &music, n))
	cancel := override(t, "o3", "math", monday, domain.OverrideCancel, "", "")
	assert.Equal(t, "Clase cancelada el 5 de octubre de 2026: Matemáticas, 3.º A, 08:00–08:50", domain.OverrideSummary(cancel, math, nil, n))
}

func blockIDs(day []domain.ScheduledClass) []string {
	out := make([]string, 0, len(day))
	for _, c := range day {
		out = append(out, c.Block.ID())
	}
	return out
}

func classOf(t *testing.T, day []domain.ScheduledClass, blockID string) domain.ScheduledClass {
	t.Helper()
	for _, c := range day {
		if c.Block.ID() == blockID {
			return c
		}
	}
	t.Fatalf("class %s not in the day", blockID)
	return domain.ScheduledClass{}
}
