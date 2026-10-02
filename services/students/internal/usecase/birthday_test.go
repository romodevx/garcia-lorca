package usecase_test

import (
	"context"
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/ports"
)

// T11: the birth date can be captured at intake, judged by the profile's rule.
func TestCreateApplicantKeepsTheBirthDate(t *testing.T) {
	svc, repo, _ := newService(t)
	in := validInput()
	in.BirthDate = "2016-02-29"
	s, err := svc.CreateApplicant(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateApplicant: %v", err)
	}
	want := time.Date(2016, time.February, 29, 0, 0, 0, 0, time.UTC)
	if !s.Profile().BirthDate().Equal(want) {
		t.Errorf("birth date = %v, want %v", s.Profile().BirthDate(), want)
	}
	if !repo.created[0].Profile().BirthDate().Equal(want) {
		t.Errorf("stored birth date = %v", repo.created[0].Profile().BirthDate())
	}
}

func TestCreateApplicantRejectsABadBirthDate(t *testing.T) {
	today := time.Date(2026, time.October, 1, 12, 0, 0, 0, time.UTC)
	for _, raw := range []string{"2016-13-01", "01/02/2016", "2027-01-01"} {
		svc, repo, _ := newService(t)
		svc = svc.WithClock(func() time.Time { return today })
		in := validInput()
		in.BirthDate = raw
		if _, err := svc.CreateApplicant(context.Background(), in); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("%q: want KindInvalid, got %v", raw, err)
		}
		if len(repo.created) != 0 {
			t.Errorf("%q: a refused applicant was stored", raw)
		}
	}
}

func TestCreateApplicantWithoutBirthDateStillWorks(t *testing.T) {
	svc, _, _ := newService(t)
	s, err := svc.CreateApplicant(context.Background(), validInput())
	if err != nil {
		t.Fatalf("CreateApplicant: %v", err)
	}
	if !s.Profile().BirthDate().IsZero() {
		t.Errorf("birth date = %v, want none", s.Profile().BirthDate())
	}
}

// The birthday filter is a calendar day; one that cannot exist is bad input,
// answered before the query.
func TestListStudentsValidatesTheBirthdayFilter(t *testing.T) {
	cases := []struct {
		name  string
		month int
		day   int
		ok    bool
	}{
		{"first of october", 10, 1, true},
		{"leap day", 2, 29, true},
		{"december 31", 12, 31, true},
		{"month zero", 0, 1, false},
		{"month thirteen", 13, 1, false},
		{"day zero", 5, 0, false},
		{"february 30", 2, 30, false},
		{"april 31", 4, 31, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newService(t)
			f := ports.ListFilter{Birthday: &ports.BirthdayFilter{Month: tc.month, Day: tc.day}}
			_, _, err := svc.ListStudents(context.Background(), f)
			if tc.ok && err != nil {
				t.Errorf("want ok, got %v", err)
			}
			if !tc.ok && glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("want KindInvalid, got %v", err)
			}
		})
	}
}
