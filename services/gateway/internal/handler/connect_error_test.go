package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/garcia-lorca/services/gateway/internal/handler"
)

// TestTheServicesOwnWordsReachThePerson closes a gap that made several careful
// messages invisible.
//
// Every service sanitises its errors at its own boundary (pkg/errors.SafeMessage),
// so what arrives here is already something a person may read. The edge threw all
// of it away and answered the bare status text, which is how a director creating
// a pupil before activating the school year was told "Bad Request" instead of
// "no hay un ciclo escolar activo".
func TestTheServicesOwnWordsReachThePerson(t *testing.T) {
	rec := httptest.NewRecorder()
	handler.WriteConnectError(rec, connect.NewError(connect.CodeInvalidArgument,
		errNoActiveCycle{}))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ciclo escolar activo") {
		t.Errorf("body = %q; the service's own sentence must reach the panel", rec.Body.String())
	}
}

// An error with nothing useful to say still gets the status text, so the body is
// never empty.
func TestAnErrorWithNothingToSayStillAnswers(t *testing.T) {
	rec := httptest.NewRecorder()
	handler.WriteConnectError(rec, connect.NewError(connect.CodeInternal, errBlank{}))

	if rec.Body.Len() == 0 {
		t.Error("an empty body tells a panel nothing at all")
	}
}

type errNoActiveCycle struct{}

func (errNoActiveCycle) Error() string { return "no hay un ciclo escolar activo" }

type errBlank struct{}

func (errBlank) Error() string { return "" }
