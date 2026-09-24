package domain_test

import (
	"strings"
	"testing"

	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/students/internal/domain"
)

func TestNewMedicalRecordAcceptsTheEightBloodTypes(t *testing.T) {
	for _, bt := range []string{"A+", "A-", "B+", "B-", "AB+", "AB-", "O+", "O-", ""} {
		rec, err := domain.NewMedicalRecord(bt, " Penicilina ", " Asma leve ")
		if err != nil {
			t.Fatalf("NewMedicalRecord(%q): %v", bt, err)
		}
		if string(rec.BloodType()) != bt || rec.Allergies() != "Penicilina" || rec.Conditions() != "Asma leve" {
			t.Errorf("record = %q %q %q", rec.BloodType(), rec.Allergies(), rec.Conditions())
		}
	}
}

func TestNewMedicalRecordRejectsWhatIsNotOnTheList(t *testing.T) {
	for _, bt := range []string{"A", "0+", "AB", "ab+"} {
		if _, err := domain.NewMedicalRecord(bt, "", ""); glerrors.KindOf(err) != glerrors.KindInvalid {
			t.Errorf("blood type %q: want KindInvalid, got %v", bt, err)
		}
	}
	long := strings.Repeat("x", 2001)
	if _, err := domain.NewMedicalRecord("O+", long, ""); glerrors.KindOf(err) != glerrors.KindInvalid {
		t.Errorf("an allergy note over 2000 characters must be refused, got %v", err)
	}
}
