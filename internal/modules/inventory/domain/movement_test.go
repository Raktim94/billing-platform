package domain

import "testing"

// TestAllMovementTypesHaveDirection guards the "two places must agree"
// risk called out in migrations/0012_inventory.up.sql's stock_movements
// comment: every constant in AllMovementTypes must have an entry in the
// directions map, or a new movement type could reach the ledger (it's
// valid per the DB CHECK constraint) with no defined effect on
// stock_balances.
func TestAllMovementTypesHaveDirection(t *testing.T) {
	for _, mt := range AllMovementTypes {
		if _, ok := DirectionOf(mt); !ok {
			t.Errorf("movement type %q has no direction mapping", mt)
		}
	}
	if len(directions) != len(AllMovementTypes) {
		t.Errorf("directions map has %d entries, AllMovementTypes has %d — one was added without the other",
			len(directions), len(AllMovementTypes))
	}
}

func TestDirectionOf_UnknownType(t *testing.T) {
	if _, ok := DirectionOf("NOT_A_REAL_TYPE"); ok {
		t.Fatal("expected DirectionOf to return false for an unrecognized movement type")
	}
}

func TestIsReceipt(t *testing.T) {
	cases := map[MovementType]bool{
		MovementOpening:         true,
		MovementPurchaseReceipt: true,
		MovementSale:            false,
		MovementDamage:          false,
		MovementTransferIn:      false, // TRANSFER_IN moves existing cost basis between warehouses, it doesn't establish a new one
	}
	for mt, want := range cases {
		if got := IsReceipt(mt); got != want {
			t.Errorf("IsReceipt(%s) = %v, want %v", mt, got, want)
		}
	}
}

func TestStockBalance_Available(t *testing.T) {
	b := StockBalance{QuantityOnHand: mustDecimal(t, "100"), QuantityReserved: mustDecimal(t, "30")}
	got := b.Available()
	if !got.Equal(mustDecimal(t, "70")) {
		t.Fatalf("Available() = %s, want 70", got)
	}
}
