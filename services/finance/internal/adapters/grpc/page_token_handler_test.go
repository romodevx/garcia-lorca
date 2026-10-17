package grpcadapter_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	connect "connectrpc.com/connect"

	financev1 "github.com/garcia-lorca/gen/go/garcialorca/finance/v1"
)

// seedThreeCharges issues three charges to the test pupil, one per concept (a
// pupil owes a concept once per cycle).
func seedThreeCharges(t *testing.T, f *fixture) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	for i := 0; i < 3; i++ {
		concept := f.seedConcept(t, fmt.Sprintf("Concepto %d", i), 100000)
		ids[f.seedCharge(t, concept.GetId(), 100000).GetId()] = true
	}
	return ids
}

// T14: the wire walk. A token handed back is the way to the next page, and the
// last page hands back none.
func TestListChargesPagesWithAnOpaqueToken(t *testing.T) {
	f := newFixture(t)
	want := seedThreeCharges(t, f)
	ctx := context.Background()

	first, err := f.h.ListCharges(ctx, connect.NewRequest(&financev1.ListChargesRequest{
		CycleId: testCycle, PageSize: 2,
	}))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Msg.GetCharges()) != 2 || first.Msg.GetNextPageToken() == "" {
		t.Fatalf("first page = %d charges, token %q; want 2 and a token",
			len(first.Msg.GetCharges()), first.Msg.GetNextPageToken())
	}
	second, err := f.h.ListCharges(ctx, connect.NewRequest(&financev1.ListChargesRequest{
		CycleId: testCycle, PageSize: 2, PageToken: first.Msg.GetNextPageToken(),
	}))
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Msg.GetCharges()) != 1 || second.Msg.GetNextPageToken() != "" {
		t.Fatalf("second page = %d charges, token %q; want 1 and no token",
			len(second.Msg.GetCharges()), second.Msg.GetNextPageToken())
	}
	seen := map[string]bool{}
	for _, c := range append(first.Msg.GetCharges(), second.Msg.GetCharges()...) {
		seen[c.GetId()] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("walked %v, want %v", seen, want)
	}
}

func TestListOverduePagesWithAnOpaqueToken(t *testing.T) {
	f := newFixture(t)
	seedThreeCharges(t, f) // due 2026-09-05, before the fixed clock
	ctx := context.Background()

	first, err := f.h.ListOverdue(ctx, connect.NewRequest(&financev1.ListOverdueRequest{
		CycleId: testCycle, PageSize: 2,
	}))
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Msg.GetCharges()) != 2 || first.Msg.GetNextPageToken() == "" {
		t.Fatalf("first page = %d, token %q; want 2 and a token", len(first.Msg.GetCharges()), first.Msg.GetNextPageToken())
	}
	last, err := f.h.ListOverdue(ctx, connect.NewRequest(&financev1.ListOverdueRequest{
		CycleId: testCycle, PageSize: 2, PageToken: first.Msg.GetNextPageToken(),
	}))
	if err != nil {
		t.Fatalf("last page: %v", err)
	}
	if len(last.Msg.GetCharges()) != 1 || last.Msg.GetNextPageToken() != "" {
		t.Fatalf("last page = %d, token %q; want 1 and no token", len(last.Msg.GetCharges()), last.Msg.GetNextPageToken())
	}
}

// A token the service did not mint is the caller's mistake, never a 500 and
// never a silent first page.
func TestPagedReadsRejectAForeignToken(t *testing.T) {
	enc := base64.RawURLEncoding.EncodeToString
	tokens := map[string]string{
		"not base64":        "%%%",
		"no separator":      enc([]byte("2026-09-05")),
		"impossible date":   enc([]byte("2026-13-40|id-1")),
		"missing id":        enc([]byte("2026-09-05|")),
		"padded std base64": base64.StdEncoding.EncodeToString([]byte("2026-09-05|id-1")) + "=",
	}
	for name, token := range tokens {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			_, err := f.h.ListCharges(ctx, connect.NewRequest(&financev1.ListChargesRequest{
				CycleId: testCycle, PageToken: token,
			}))
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("ListCharges: code = %v, want InvalidArgument", connect.CodeOf(err))
			}
			_, err = f.h.ListOverdue(ctx, connect.NewRequest(&financev1.ListOverdueRequest{
				CycleId: testCycle, PageToken: token,
			}))
			if connect.CodeOf(err) != connect.CodeInvalidArgument {
				t.Errorf("ListOverdue: code = %v, want InvalidArgument", connect.CodeOf(err))
			}
		})
	}
}
