package domain

import "github.com/shopspring/decimal"

// CostingStrategy computes the new weighted-average cost per base unit
// after a receipt. docs/architecture.md §6: this interface exists so a
// future FIFO strategy is a second implementation plus a per-product/
// per-warehouse config flag, not a rewrite of the movement ledger.
type CostingStrategy interface {
	// OnReceipt returns the new average cost given the balance before the
	// receipt (currentQty, currentAvgCost) and the receipt itself
	// (receiptQty, receiptUnitCost). Called only for movement types where
	// IsReceipt is true.
	OnReceipt(currentQty, currentAvgCost, receiptQty, receiptUnitCost decimal.Decimal) decimal.Decimal
}

// WeightedAverageCostingStrategy is the only CostingStrategy implemented
// in v1 (brief §12: "Support at minimum: weighted average cost").
type WeightedAverageCostingStrategy struct{}

// OnReceipt implements the standard moving-average formula:
//
//	new_avg = (currentQty*currentAvgCost + receiptQty*receiptUnitCost) / (currentQty+receiptQty)
//
// If the resulting total quantity is zero (only possible if both inputs
// are zero, since receiptQty > 0 is enforced by the DB CHECK constraint
// on stock_movements.quantity before this is ever called), returns zero
// rather than dividing by zero.
func (WeightedAverageCostingStrategy) OnReceipt(currentQty, currentAvgCost, receiptQty, receiptUnitCost decimal.Decimal) decimal.Decimal {
	totalQty := currentQty.Add(receiptQty)
	if totalQty.IsZero() {
		return decimal.Zero
	}
	totalValue := currentQty.Mul(currentAvgCost).Add(receiptQty.Mul(receiptUnitCost))
	return totalValue.Div(totalQty)
}
