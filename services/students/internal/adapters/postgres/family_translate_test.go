package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	glerrors "github.com/garcia-lorca/pkg/errors"
)

// A deadlock or serialization failure between two family writes is a lost
// race the caller may retry (409), never a 500.
func TestTranslateFamilyWriteErrMapsLostRacesToConflict(t *testing.T) {
	for _, code := range []string{"40P01", "40001"} {
		err := translateFamilyWriteErr(&pgconn.PgError{Code: code}, "adding pupils")
		if glerrors.KindOf(err) != glerrors.KindConflict {
			t.Errorf("SQLSTATE %s: kind = %v, want KindConflict", code, glerrors.KindOf(err))
		}
	}
}

func TestCapRows(t *testing.T) {
	rows := []int{1, 2, 3}
	if kept, hit := capRows(rows, 3); len(kept) != 3 || hit {
		t.Errorf("at the cap: %v, %v", kept, hit)
	}
	if kept, hit := capRows(append(rows, 4), 3); len(kept) != 3 || !hit {
		t.Errorf("past the cap: %v, %v", kept, hit)
	}
}
