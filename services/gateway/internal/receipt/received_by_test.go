package receipt_test

import (
	"context"
	"errors"
	"testing"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

type fakeUsers map[string]client.User

func (f fakeUsers) GetUser(_ context.Context, id string) (client.User, error) {
	u, ok := f[id]
	if !ok {
		return client.User{}, errors.New("no such user")
	}
	return u, nil
}

func counterSale(createdBy string) fakeSources {
	return fakeSources{order: client.UniformOrder{
		ID: "ord-1", Folio: "U-2026-000001", Source: "counter", TotalCents: 32000,
		CreatedAt: "2026-10-17T16:00:00Z", CreatedByUserID: createdBy,
		Lines: []client.UniformOrderLine{{ItemName: "Playera", SizeLabel: "8", Qty: 1, UnitPriceCents: 32000}},
	}}
}

// ISSUE-T14-019: the sale receipt says who collected, as the payment's does.
func TestASaleReceiptSaysWhoCollected(t *testing.T) {
	cases := []struct {
		name  string
		by    string
		users fakeUsers
		want  string
	}{
		{name: "the seller's display name", by: "u-1", users: fakeUsers{"u-1": {DisplayName: "Laura Caja", Email: "laura@escuela.test"}}, want: "Laura Caja"},
		{name: "the email when there is no name", by: "u-1", users: fakeUsers{"u-1": {Email: "laura@escuela.test"}}, want: "laura@escuela.test"},
		{name: "nothing when the directory cannot answer", by: "u-9", users: fakeUsers{}, want: ""},
		{name: "nothing for a sale with no seller recorded", by: "", users: fakeUsers{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := counterSale(tc.by)
			b := receipt.NewBuilder(f, f, f, f, f, receipt.WithUserNamer(tc.users))
			got, err := b.Sale(context.Background(), "ord-1")
			if err != nil {
				t.Fatalf("Sale: %v", err)
			}
			if got.ReceivedBy != tc.want {
				t.Errorf("received by = %q, want %q", got.ReceivedBy, tc.want)
			}
		})
	}
}
