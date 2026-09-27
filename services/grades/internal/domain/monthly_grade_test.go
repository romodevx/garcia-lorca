package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/grades/internal/domain"
)

func defaultScale(t *testing.T) domain.GradingScale {
	t.Helper()
	s, err := domain.NewGradingScale(500, 1000, 600, 1)
	require.NoError(t, err)
	return s
}

func TestNewGradingScaleRejectsInconsistentScales(t *testing.T) {
	tests := []struct {
		name                       string
		min, max, passing, decimal int
	}{
		{"min equals passing", 600, 1000, 600, 1},
		{"passing above max", 500, 1000, 1100, 1},
		{"negative min", -1, 1000, 600, 1},
		{"above one hundred", 500, 10001, 600, 1},
		{"three decimals", 500, 1000, 600, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewGradingScale(tt.min, tt.max, tt.passing, tt.decimal)
			assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
		})
	}
}

// ParseValue judges the text the teacher typed EXACTLY (decimal arithmetic, no
// float): 9.95 carries two decimals even though a float64 would print it as
// 9.949999…, and 10.0 is ten.
func TestGradingScaleParseValue(t *testing.T) {
	const (
		notNumber = "La calificación debe ser un número"
		outRange  = "La calificación debe estar entre 5 y 10"
		oneDec    = "Solo se permite 1 decimal"
	)
	tests := []struct {
		text     string
		want     int
		wantMsg  string
		wantCode domain.RefusalCode
	}{
		{"10", 1000, "", ""},
		{"10.0", 1000, "", ""},
		{"5", 500, "", ""},
		{"9.5", 950, "", ""},
		{"6.0", 600, "", ""},
		{"9.50000000000000", 950, "", ""}, // 16 chars: still a number
		{"9.95", 0, oneDec, domain.CodeGradeDecimals},
		{"7.25", 0, oneDec, domain.CodeGradeDecimals},
		{"4.9", 0, outRange, domain.CodeGradeOutOfRange},
		{"10.1", 0, outRange, domain.CodeGradeOutOfRange},
		{"10.05", 0, outRange, domain.CodeGradeOutOfRange},
		{"-7", 0, outRange, domain.CodeGradeOutOfRange},
		{"", 0, notNumber, domain.CodeGradeNotNumber},
		{"abc", 0, notNumber, domain.CodeGradeNotNumber},
		{"NaN", 0, notNumber, domain.CodeGradeNotNumber},
		{"Inf", 0, notNumber, domain.CodeGradeNotNumber},
		{"1/2", 0, notNumber, domain.CodeGradeNotNumber},
		// Exponent, hex and binary forms never reach big.Rat: "1e1000000000"
		// would make it build a billion-digit number.
		{"1e1", 0, notNumber, domain.CodeGradeNotNumber},
		{"1e1000000000", 0, notNumber, domain.CodeGradeNotNumber},
		{"0x1p3", 0, notNumber, domain.CodeGradeNotNumber},
		{"0b1010", 0, notNumber, domain.CodeGradeNotNumber},
		{"+9", 0, notNumber, domain.CodeGradeNotNumber},
		{"9.500000000000000", 0, notNumber, domain.CodeGradeNotNumber}, // 17 chars
	}
	scale := defaultScale(t)
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got, err := scale.ParseValue(tt.text)
			if tt.wantMsg != "" {
				require.Error(t, err)
				assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
				assert.Equal(t, tt.wantMsg, glerrors.SafeMessage(err))
				assert.Equal(t, tt.wantCode, domain.RefusalCodeOf(err))
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.Hundredths())
		})
	}
}

// A refusal carries a machine code without hiding its kind or its sentence.
func TestRefusalKeepsKindAndMessage(t *testing.T) {
	err := domain.Refusal(domain.CodeMonthOutsideCycle, "Ese mes no pertenece al ciclo")
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err))
	assert.Equal(t, "Ese mes no pertenece al ciclo", glerrors.SafeMessage(err))
	assert.Equal(t, domain.CodeMonthOutsideCycle, domain.RefusalCodeOf(err))
	assert.Equal(t, domain.RefusalCode(""), domain.RefusalCodeOf(glerrors.New(glerrors.KindInvalid, "x")))
}

func TestGradingScaleDecimalsWording(t *testing.T) {
	tests := []struct {
		decimals int
		text     string
		wantMsg  string
	}{
		{0, "7.5", "Solo se permiten calificaciones enteras"},
		{2, "7.255", "Solo se permiten 2 decimales"},
	}
	for _, tt := range tests {
		s, err := domain.NewGradingScale(0, 1000, 600, tt.decimals)
		require.NoError(t, err)
		_, err = s.ParseValue(tt.text)
		assert.Equal(t, tt.wantMsg, glerrors.SafeMessage(err))
	}
	s, err := domain.NewGradingScale(550, 975, 600, 2)
	require.NoError(t, err)
	_, err = s.ParseValue("9.8")
	assert.Equal(t, "La calificación debe estar entre 5.5 y 9.75", glerrors.SafeMessage(err))
}

func TestGradeValueString(t *testing.T) {
	scale := defaultScale(t)
	for text, want := range map[string]string{"10.0": "10", "9.5": "9.5", "6": "6"} {
		v, err := scale.ParseValue(text)
		require.NoError(t, err)
		assert.Equal(t, want, v.String())
	}
	assert.Equal(t, "9.25", domain.GradeValueFromHundredths(925).String())
}

func TestParseMonth(t *testing.T) {
	m, err := domain.ParseMonth("2026-09")
	require.NoError(t, err)
	assert.Equal(t, "2026-09", m.String())
	assert.Equal(t, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), m.FirstDay())

	for _, bad := range []string{"", "2026-9", "2026-13", "2026-09-01", "sept"} {
		_, err := domain.ParseMonth(bad)
		assert.Equal(t, "El mes debe tener el formato AAAA-MM", glerrors.SafeMessage(err), bad)
	}
}

func TestCycleMonths(t *testing.T) {
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 7, 15, 0, 0, 0, 0, time.UTC)
	months := domain.CycleMonths(start, end)
	require.Len(t, months, 12)
	assert.Equal(t, "2026-08", months[0].String())
	assert.Equal(t, "2027-07", months[11].String())
	assert.Empty(t, domain.CycleMonths(time.Time{}, time.Time{}), "a cycle without dates has no months")
	assert.Empty(t, domain.CycleMonths(end, start), "an inverted range has no months")
}

func TestClampMonth(t *testing.T) {
	months := domain.CycleMonths(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 7, 1, 0, 0, 0, 0, time.UTC))
	tests := []struct {
		now  time.Time
		want string
	}{
		{time.Date(2026, 10, 31, 23, 0, 0, 0, time.UTC), "2026-10"},
		{time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), "2026-08"},
		{time.Date(2027, 9, 1, 0, 0, 0, 0, time.UTC), "2027-07"},
	}
	for _, tt := range tests {
		got, ok := domain.ClampMonth(months, tt.now)
		require.True(t, ok)
		assert.Equal(t, tt.want, got.String())
	}
	_, ok := domain.ClampMonth(nil, time.Now())
	assert.False(t, ok)
}

func TestNewMonthlyGrade(t *testing.T) {
	month, err := domain.ParseMonth("2026-09")
	require.NoError(t, err)
	value, err := defaultScale(t).ParseValue("9.5")
	require.NoError(t, err)
	at := time.Date(2026, 9, 3, 10, 0, 0, 0, time.UTC)

	g, err := domain.NewMonthlyGrade(domain.MonthlyGradeFields{
		ID: "mg-1", TenantID: "t1", EnrollmentID: "enr-1", SubjectID: "subj-1",
		TeacherID: "teacher-1", CapturedBy: "user-1", Month: month, Value: value, UpdatedAt: at,
	})
	require.NoError(t, err)
	assert.Equal(t, "enr-1", g.EnrollmentID())
	assert.Equal(t, "subj-1", g.SubjectID())
	assert.Equal(t, "teacher-1", g.TeacherID())
	assert.Equal(t, "user-1", g.CapturedBy())
	assert.Equal(t, "2026-09", g.Month().String())
	assert.Equal(t, 950, g.Value().Hundredths())
	assert.Equal(t, at, g.UpdatedAt())

	for _, blank := range []string{"ID", "TenantID", "EnrollmentID", "SubjectID", "TeacherID", "CapturedBy"} {
		f := domain.MonthlyGradeFields{ID: "mg-1", TenantID: "t1", EnrollmentID: "enr-1", SubjectID: "subj-1",
			TeacherID: "teacher-1", CapturedBy: "user-1", Month: month, Value: value}
		switch blank {
		case "ID":
			f.ID = ""
		case "TenantID":
			f.TenantID = ""
		case "EnrollmentID":
			f.EnrollmentID = ""
		case "SubjectID":
			f.SubjectID = ""
		case "TeacherID":
			f.TeacherID = ""
		case "CapturedBy":
			f.CapturedBy = " "
		}
		_, err := domain.NewMonthlyGrade(f)
		assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), blank)
	}
	_, err = domain.NewMonthlyGrade(domain.MonthlyGradeFields{ID: "mg-1", TenantID: "t1", EnrollmentID: "enr-1",
		SubjectID: "subj-1", TeacherID: "teacher-1", CapturedBy: "user-1", Value: value})
	assert.Equal(t, glerrors.KindInvalid, glerrors.KindOf(err), "a zero month is not a month")
}

// Admits judges a stored value against a (possibly new) scale: range and
// decimals, exactly as ParseValue would have.
func TestGradingScaleAdmits(t *testing.T) {
	s := defaultScale(t)
	for h, want := range map[int]bool{500: true, 1000: true, 950: true, 495: false, 1010: false, 925: false} {
		assert.Equal(t, want, s.Admits(domain.GradeValueFromHundredths(h)), "hundredths %d", h)
	}
}
