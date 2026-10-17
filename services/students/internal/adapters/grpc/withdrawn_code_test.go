package grpcadapter

import (
	"fmt"
	"testing"

	"github.com/garcia-lorca/services/students/internal/domain"
)

// T14-009 gate: a refused correction of a withdrawn pupil is a 409 like a
// duplicate CURP, so the screen tells them apart by the machine code the
// gateway forwards as X-Error-Code.
func TestAWithdrawnRefusalCarriesItsOwnCode(t *testing.T) {
	err := connectErr(fmt.Errorf("saving: %w", domain.ErrWithdrawnReadOnly))
	if got := err.Meta().Get(ErrorCodeMeta); got != domain.CodeStudentWithdrawn {
		t.Errorf("X-Error-Code = %q, want %q", got, domain.CodeStudentWithdrawn)
	}
	if got := err.Message(); got != "ese alumno está dado de baja: su ficha ya no se corrige" {
		t.Errorf("message = %q", got)
	}
}
