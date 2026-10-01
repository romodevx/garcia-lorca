package financeclient_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
	"github.com/garcia-lorca/gen/go/garcialorca/finance/v1/financev1connect"
	glerrors "github.com/garcia-lorca/pkg/errors"
	"github.com/garcia-lorca/services/uniforms/internal/adapters/financeclient"
	"github.com/garcia-lorca/services/uniforms/internal/ports"
)

// stubFinance is a finance service that behaves the way the real one does on the
// paths this adapter walks — including refusing a duplicate concept name.
type stubFinance struct {
	financev1connect.UnimplementedFinanceServiceHandler

	mu             sync.Mutex
	concepts       []*financev1.ChargeConcept
	createdCharges []*financev1.CreateChargeRequest
	payments       []*financev1.RegisterPaymentRequest
	cancelled      []string

	listErr   error
	createErr error
	cancelErr error

	createConceptCalls int
}

func (s *stubFinance) ListConcepts(context.Context, *connect.Request[financev1.ListConceptsRequest]) (*connect.Response[financev1.ListConceptsResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listErr != nil {
		return nil, s.listErr
	}
	return connect.NewResponse(&financev1.ListConceptsResponse{Concepts: s.concepts}), nil
}

func (s *stubFinance) CreateConcept(_ context.Context, req *connect.Request[financev1.CreateConceptRequest]) (*connect.Response[financev1.CreateConceptResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createConceptCalls++
	// What the real ledger does: it knows two recurrences and refuses the rest.
	// A stub that took anything is how "one_off" shipped and no counter sale
	// could reach finance on a school without the concept (task 10).
	if r := req.Msg.GetRecurrence(); r != "once" && r != "monthly" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("unknown recurrence "+r))
	}
	for _, c := range s.concepts {
		if c.GetName() == req.Msg.GetName() {
			// What the real ledger does: a name is unique per school.
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("concept already exists"))
		}
	}
	concept := &financev1.ChargeConcept{Id: "concept-1", Name: req.Msg.GetName()}
	s.concepts = append(s.concepts, concept)
	return connect.NewResponse(&financev1.CreateConceptResponse{Concept: concept}), nil
}

func (s *stubFinance) CreateCharge(_ context.Context, req *connect.Request[financev1.CreateChargeRequest]) (*connect.Response[financev1.CreateChargeResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.createErr != nil {
		return nil, s.createErr
	}
	s.createdCharges = append(s.createdCharges, req.Msg)
	return connect.NewResponse(&financev1.CreateChargeResponse{Charge: &financev1.Charge{
		Id: "chg-1", AmountCents: req.Msg.GetAmountCentsOverride(),
		RemainingCents: req.Msg.GetAmountCentsOverride(), Status: "pending",
		ExternalRef: req.Msg.GetExternalRef(),
	}}), nil
}

func (s *stubFinance) RegisterPayment(_ context.Context, req *connect.Request[financev1.RegisterPaymentRequest]) (*connect.Response[financev1.RegisterPaymentResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.payments = append(s.payments, req.Msg)
	return connect.NewResponse(&financev1.RegisterPaymentResponse{}), nil
}

func (s *stubFinance) CancelChargeIfUnpaid(_ context.Context, req *connect.Request[financev1.CancelChargeIfUnpaidRequest]) (*connect.Response[financev1.CancelChargeIfUnpaidResponse], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancelErr != nil {
		return nil, s.cancelErr
	}
	s.cancelled = append(s.cancelled, req.Msg.GetChargeId())
	return connect.NewResponse(&financev1.CancelChargeIfUnpaidResponse{}), nil
}

func clientFor(t *testing.T, stub *stubFinance) *financeclient.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(financev1connect.NewFinanceServiceHandler(stub))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return financeclient.New(srv.URL, srv.Client())
}

// TestEnsureConceptCreatesItOnce: one catalogue entry, not one per order. Three
// hundred rows a year would make the cashier's dropdown useless within a term.
func TestEnsureConceptCreatesItOnce(t *testing.T) {
	stub := &stubFinance{}
	c := clientFor(t, stub)
	ctx := context.Background()

	first, err := c.EnsureConcept(ctx)
	if err != nil {
		t.Fatalf("EnsureConcept: %v", err)
	}
	second, err := c.EnsureConcept(ctx)
	if err != nil {
		t.Fatalf("second EnsureConcept: %v", err)
	}
	if first != second {
		t.Errorf("two ids for one concept: %q and %q", first, second)
	}
	if stub.createConceptCalls != 1 {
		t.Errorf("created the concept %d times, want 1", stub.createConceptCalls)
	}
}

// TestEnsureConceptFindsAnExistingOne: a school that already has the entry —
// from a previous release, or created by hand — must not get a second one.
func TestEnsureConceptFindsAnExistingOne(t *testing.T) {
	stub := &stubFinance{concepts: []*financev1.ChargeConcept{
		{Id: "concept-old", Name: "Uniformes"},
	}}
	c := clientFor(t, stub)

	got, err := c.EnsureConcept(context.Background())
	if err != nil {
		t.Fatalf("EnsureConcept: %v", err)
	}
	if got != "concept-old" {
		t.Errorf("id = %q, want the existing concept-old", got)
	}
	if stub.createConceptCalls != 0 {
		t.Error("it created a duplicate instead of finding the existing entry")
	}
}

// TestEnsureConceptFindsARetiredOne is the subtle case. A school that
// deactivated the entry still has issued charges pointing at it; minting a second
// with the same name would split the history in two, and the older half would
// quietly stop appearing beside the newer.
func TestEnsureConceptFindsARetiredOne(t *testing.T) {
	stub := &stubFinance{concepts: []*financev1.ChargeConcept{
		{Id: "concept-retired", Name: "Uniformes", Active: false},
	}}
	c := clientFor(t, stub)

	got, err := c.EnsureConcept(context.Background())
	if err != nil {
		t.Fatalf("EnsureConcept: %v", err)
	}
	if got != "concept-retired" {
		t.Errorf("id = %q; a retired entry is still the one issued charges point at", got)
	}
}

// TestEnsureConceptSurvivesLosingTheRace: two instances start together, both see
// no concept, both create. The loser is told AlreadyExists and must read back
// what the winner made rather than failing the sale behind it.
func TestEnsureConceptSurvivesLosingTheRace(t *testing.T) {
	stub := &stubFinance{}
	winner := clientFor(t, stub)
	if _, err := winner.EnsureConcept(context.Background()); err != nil {
		t.Fatalf("the winner: %v", err)
	}

	// A second instance with its own empty cache, arriving after the winner.
	loser := financeclient.New(serverURL(t, stub), http.DefaultClient)
	got, err := loser.EnsureConcept(context.Background())
	if err != nil {
		t.Fatalf("the loser must recover, not fail: %v", err)
	}
	if got == "" {
		t.Error("the loser came back with no concept id")
	}
}

// TestCreateChargeCarriesTheExternalRef: it does two jobs at once — it exempts
// the charge from the ledger's "one concept per cycle" rule, without which a
// family could never buy a uniform twice in a school year, and it deduplicates a
// retry whose response was lost.
func TestCreateChargeCarriesTheExternalRef(t *testing.T) {
	stub := &stubFinance{}
	c := clientFor(t, stub)

	got, err := c.CreateCharge(context.Background(), ports.CreateChargeInput{
		StudentID: "stu-1", CycleID: "cyc-1", ConceptID: "concept-1",
		Description: "Uniformes · U-2026-000123", AmountCents: 64000,
		DueDate: "2026-09-15", ExternalRef: "uniforms:ful-1",
	})
	if err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}
	if got.ID != "chg-1" || got.RemainingCents != 64000 {
		t.Errorf("charge = %+v", got)
	}
	if len(stub.createdCharges) != 1 || stub.createdCharges[0].GetExternalRef() != "uniforms:ful-1" {
		t.Errorf("the external ref did not reach finance: %+v", stub.createdCharges)
	}
	if stub.createdCharges[0].GetAmountCentsOverride() != 64000 {
		t.Errorf("amount = %d, want 64000", stub.createdCharges[0].GetAmountCentsOverride())
	}
}

// TestRegisterPaymentPassesTheIdempotencyKey: a retried call returns the original
// payment rather than collecting from the family a second time.
func TestRegisterPaymentPassesTheIdempotencyKey(t *testing.T) {
	stub := &stubFinance{}
	c := clientFor(t, stub)

	if err := c.RegisterPayment(context.Background(), "chg-1", 64000,
		[]ports.Tender{{Method: "cash", AmountCents: 64000}}, "ful-1"); err != nil {
		t.Fatalf("RegisterPayment: %v", err)
	}
	if len(stub.payments) != 1 || stub.payments[0].GetIdempotencyKey() != "ful-1" {
		t.Errorf("the idempotency key did not reach finance: %+v", stub.payments)
	}
	if stub.payments[0].GetMethod() != "cash" {
		t.Errorf("a single tender must also fill method for an older finance: %+v", stub.payments[0])
	}
}

// T8: a combined payment reaches finance tender by tender.
func TestRegisterPaymentCarriesTheTenders(t *testing.T) {
	stub := &stubFinance{}
	c := clientFor(t, stub)

	if err := c.RegisterPayment(context.Background(), "chg-1", 64000, []ports.Tender{
		{Method: "credit_note", AmountCents: 20000, Reference: "NC-12"},
		{Method: "card", AmountCents: 44000, CardType: "debit"},
	}, "ful-2"); err != nil {
		t.Fatalf("RegisterPayment: %v", err)
	}
	got := stub.payments[0].GetTenders()
	if len(got) != 2 || got[0].GetMethod() != "credit_note" || got[0].GetReference() != "NC-12" ||
		got[1].GetCardType() != "debit" || got[1].GetAmountCents() != 44000 {
		t.Errorf("tenders = %+v", got)
	}
}

// TestCancelIfUnpaidRefusalIsAConflictNotAFailure is the distinction the expiry
// sweep is built on. A refusal means the cashier got there first, so the goods
// stay where they are — an ANSWER, not an error, and the shop must be able to
// tell them apart.
func TestCancelIfUnpaidRefusalIsAConflictNotAFailure(t *testing.T) {
	stub := &stubFinance{cancelErr: connect.NewError(connect.CodeAlreadyExists,
		errors.New("charge has payments and cannot be cancelled"))}
	c := clientFor(t, stub)

	err := c.CancelChargeIfUnpaid(context.Background(), "chg-1")
	if glerrors.KindOf(err) != glerrors.KindConflict {
		t.Errorf("want KindConflict, got %v", err)
	}
}

func TestCancelIfUnpaidSucceedsOnACleanCharge(t *testing.T) {
	stub := &stubFinance{}
	c := clientFor(t, stub)
	if err := c.CancelChargeIfUnpaid(context.Background(), "chg-1"); err != nil {
		t.Fatalf("CancelChargeIfUnpaid: %v", err)
	}
	if len(stub.cancelled) != 1 || stub.cancelled[0] != "chg-1" {
		t.Errorf("cancelled = %v", stub.cancelled)
	}
}

func TestFinanceErrorsKeepTheirMeaning(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want glerrors.Kind
	}{
		{"not found", connect.NewError(connect.CodeNotFound, errors.New("no such charge")), glerrors.KindNotFound},
		{"invalid", connect.NewError(connect.CodeInvalidArgument, errors.New("bad amount")), glerrors.KindInvalid},
		{"conflict", connect.NewError(connect.CodeAlreadyExists, errors.New("dup")), glerrors.KindConflict},
		{"unavailable becomes internal", connect.NewError(connect.CodeUnavailable, errors.New("down")), glerrors.KindInternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := clientFor(t, &stubFinance{createErr: tc.err})
			_, err := c.CreateCharge(context.Background(), ports.CreateChargeInput{})
			if glerrors.KindOf(err) != tc.want {
				t.Errorf("want %v, got %v", tc.want, err)
			}
		})
	}
}

// serverURL spins a second server over the same stub, so two clients can share
// one finance while keeping their own caches.
func serverURL(t *testing.T, stub *stubFinance) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(financev1connect.NewFinanceServiceHandler(stub))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}
