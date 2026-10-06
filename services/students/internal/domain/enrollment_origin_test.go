package domain_test

import (
	"testing"
	"time"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestAnEnrollmentIsManualUnlessSaidOtherwise(t *testing.T) {
	e, err := domain.NewEnrollment("e1", "t1", "s1", "c1", "g1", "", domain.EnrollmentStatusActive, time.Now())
	if err != nil {
		t.Fatalf("NewEnrollment: %v", err)
	}
	if e.Origin() != domain.EnrollmentOriginManual {
		t.Fatalf("origin = %q; want manual", e.Origin())
	}
	imported := e.WithOrigin(domain.EnrollmentOriginImport)
	if imported.Origin() != domain.EnrollmentOriginImport || e.Origin() != domain.EnrollmentOriginManual {
		t.Fatalf("WithOrigin must return a copy: got %q, original %q", imported.Origin(), e.Origin())
	}
}

func TestParseEnrollmentOrigin(t *testing.T) {
	cases := []struct {
		raw     string
		want    domain.EnrollmentOrigin
		invalid bool
	}{
		{raw: "manual", want: domain.EnrollmentOriginManual},
		{raw: "import", want: domain.EnrollmentOriginImport},
		{raw: "", want: domain.EnrollmentOriginManual},
		{raw: "excel", invalid: true},
	}
	for _, tc := range cases {
		got, err := domain.ParseEnrollmentOrigin(tc.raw)
		if tc.invalid {
			if glerrors.KindOf(err) != glerrors.KindInvalid {
				t.Errorf("ParseEnrollmentOrigin(%q) err = %v; want KindInvalid", tc.raw, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("ParseEnrollmentOrigin(%q) = %q, %v; want %q", tc.raw, got, err, tc.want)
		}
	}
}
