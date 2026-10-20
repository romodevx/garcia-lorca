package receipt_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/garcia-lorca/services/gateway/internal/client"
	"github.com/garcia-lorca/services/gateway/internal/receipt"
)

// C2-015: a returned sale's receipt says so, with the reason, like a voided
// payment's: a reprint must never read as money still in the drawer.
func TestAReturnedSaleReceiptSaysCancelledWithItsReason(t *testing.T) {
	f := fakeSources{
		profile: school, found: true,
		order: client.UniformOrder{
			ID: "ord-r", Folio: "U-2026-000002", Source: "counter", TotalCents: 25000,
			CreatedAt: "2026-10-19T18:00:00Z",
			Fulfillments: []client.UniformFulfillment{{
				ID: "ful-r", Status: "returned",
				Return: &client.UniformReturn{Reason: "Talla equivocada", RefundMode: "void"},
			}},
		},
	}

	got, err := receipt.NewBuilder(f, f, f, f, f).Sale(context.Background(), "ord-r")

	require.NoError(t, err)
	assert.True(t, got.Voided)
	assert.Equal(t, "Devolución: Talla equivocada", got.VoidReason)
}

// Review: a return's decision is stored before the money moves; until the
// sale is actually returned its receipt is an ordinary sale's.
func TestASaleWithOnlyAReturnDecisionIsNotCancelled(t *testing.T) {
	f := fakeSources{
		profile: school, found: true,
		order: client.UniformOrder{
			ID: "ord-i", Folio: "U-2026-000003", Source: "counter", TotalCents: 25000, CreatedAt: "2026-10-19T18:00:00Z",
			Fulfillments: []client.UniformFulfillment{{
				ID: "ful-i", Status: "sold_at_counter",
				Return: &client.UniformReturn{Reason: "Talla", RefundMode: "void"},
			}},
		},
	}

	got, err := receipt.NewBuilder(f, f, f, f, f).Sale(context.Background(), "ord-i")

	require.NoError(t, err)
	assert.False(t, got.Voided)
}
