package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestUnitConversion_Convert(t *testing.T) {
	// 1 BOX = 25 PCS
	c := UnitConversion{Factor: decimal.NewFromInt(25)}

	got := c.Convert(decimal.NewFromInt(3)) // 3 BOX
	want := decimal.NewFromInt(75)          // -> 75 PCS
	if !got.Equal(want) {
		t.Fatalf("Convert(3) = %s, want %s", got, want)
	}
}

func TestUnitConversion_Convert_FractionalQuantity(t *testing.T) {
	// 1 KG = 1000 G
	c := UnitConversion{Factor: decimal.NewFromInt(1000)}

	got := c.Convert(decimal.RequireFromString("2.5")) // 2.5 KG
	want := decimal.NewFromInt(2500)                   // -> 2500 G
	if !got.Equal(want) {
		t.Fatalf("Convert(2.5) = %s, want %s", got, want)
	}
}

func TestUnitConversion_Invert_RoundTrips(t *testing.T) {
	// 1 BOX = 25 PCS; inverting and converting back should recover the
	// original quantity within decimal precision.
	c := UnitConversion{Factor: decimal.NewFromInt(25)}
	inv := c.Invert()

	boxes := decimal.NewFromInt(4)
	pcs := c.Convert(boxes)
	backToBoxes := inv.Convert(pcs)

	if !backToBoxes.Equal(boxes) {
		t.Fatalf("round trip: got %s, want %s", backToBoxes, boxes)
	}
}

func TestUnitConversion_Invert_SwapsUnits(t *testing.T) {
	fromID, toID := uuid.New(), uuid.New()
	c := UnitConversion{FromUnitID: fromID, ToUnitID: toID, Factor: decimal.NewFromInt(25)}
	inv := c.Invert()
	if inv.FromUnitID != toID || inv.ToUnitID != fromID {
		t.Fatalf("Invert() did not swap From/To unit ids: %+v", inv)
	}
}
