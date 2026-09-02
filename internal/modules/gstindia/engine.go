// Package gstindia implements India's GST as a TaxEngine plugin
// (docs/architecture.md §8, brief §7). It is the ONLY package that knows
// about CGST/SGST/IGST/UTGST/CESS, place-of-supply, or GST state codes —
// internal/modules/taxation stays fully generic.
package gstindia

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	gstdomain "billing-platform/internal/modules/gstindia/domain"
	taxdomain "billing-platform/internal/modules/taxation/domain"
	"billing-platform/internal/platform/money"
)

type Engine struct {
	rates  gstdomain.TaxRateRepository
	states gstdomain.StateRepository
}

func NewEngine(rates gstdomain.TaxRateRepository, states gstdomain.StateRepository) *Engine {
	return &Engine{rates: rates, states: states}
}

var _ taxdomain.TaxEngine = (*Engine)(nil)

var hundred = decimal.NewFromInt(100)

// Calculate implements taxdomain.TaxEngine. For each line: resolve the
// tax_rate_master row valid on in.DocumentDate for that HSN/SAC (a hard
// error if none exists — never a silent 0%, brief Rule 2); determine
// intra-state (CGST+SGST, or CGST+UTGST if the place of supply is a Union
// Territory) vs. inter-state (IGST) from supplier state vs. place of
// supply; back out the taxable value for INCLUSIVE pricing or use the
// input directly for EXCLUSIVE; round each tax component independently
// (the ONE documented rounding point, docs/architecture.md §5) and derive
// the taxable amount as gross minus the summed rounded tax for INCLUSIVE
// lines, guaranteeing taxable+tax reconciles exactly to the entered gross
// (brief's ₹90 fixture: grand total stays exactly ₹90 after rounding).
//
// Rounding components independently (rather than rounding one combined
// rate) means an intra-state split (two rounded halves) and an
// inter-state whole (one rounded figure) CAN differ by up to one currency
// minor unit for the identical nominal rate and gross amount — this is
// expected statutory invoice-rounding behavior (each displayed tax line
// on a real GST invoice is independently rounded), not a bug; see
// TestEngine_IntraVsInterState_UnroundedAmountsMatchExactly for the
// invariant that actually holds (equal before rounding, may differ by
// <=1 minor unit after).
func (e *Engine) Calculate(ctx context.Context, in taxdomain.TaxCalculationInput) (taxdomain.TaxCalculationResult, error) {
	if len(in.Lines) == 0 {
		return taxdomain.TaxCalculationResult{}, taxdomain.ErrNoLines
	}
	placeState, err := e.states.GetByCode(ctx, in.SupplyPlace.StateCode)
	if err != nil {
		return taxdomain.TaxCalculationResult{}, fmt.Errorf("gstindia: resolving place of supply state %q: %w", in.SupplyPlace.StateCode, err)
	}
	intraState := in.SupplierStateCode == in.SupplyPlace.StateCode

	var result taxdomain.TaxCalculationResult
	totalTaxable := decimal.Zero
	totalTax := decimal.Zero

	for _, line := range in.Lines {
		lr, err := e.calculateLine(ctx, in.OrganisationID, in.DocumentDate, line, intraState, placeState.IsUnionTerritory)
		if err != nil {
			return taxdomain.TaxCalculationResult{}, err
		}
		result.Lines = append(result.Lines, lr)
		totalTaxable = totalTaxable.Add(lr.TaxableAmount.Decimal())
		totalTax = totalTax.Add(lr.TotalTax.Decimal())
	}

	totalTaxableMoney, err := money.New(totalTaxable, in.CurrencyCode)
	if err != nil {
		return taxdomain.TaxCalculationResult{}, err
	}
	totalTaxMoney, err := money.New(totalTax, in.CurrencyCode)
	if err != nil {
		return taxdomain.TaxCalculationResult{}, err
	}
	grandTotal, err := totalTaxableMoney.Add(totalTaxMoney)
	if err != nil {
		return taxdomain.TaxCalculationResult{}, err
	}
	result.TotalTaxableAmount = totalTaxableMoney
	result.TotalTaxAmount = totalTaxMoney
	result.GrandTotal = grandTotal
	return result, nil
}

func (e *Engine) calculateLine(
	ctx context.Context,
	orgID uuid.UUID,
	documentDate time.Time,
	line taxdomain.TaxableLine,
	intraState bool,
	placeIsUT bool,
) (taxdomain.TaxLineResult, error) {
	rate, err := e.rates.Resolve(ctx, orgID, "IN", line.HSNSACCode, documentDate)
	if err != nil {
		return taxdomain.TaxLineResult{}, fmt.Errorf("%w (hsn_sac_code=%q)", gstdomain.ErrRateNotConfigured, line.HSNSACCode)
	}

	currency := line.Amount.Currency()
	combinedRate := rate.GSTRate.Add(rate.CessRate)
	gross := line.Amount.Decimal()

	// Full-precision intermediate value (decimal's default 16-digit
	// division precision, far beyond any currency's minor-unit precision)
	// — nothing is rounded until the components below.
	var taxableUnrounded decimal.Decimal
	if line.PricingMode == taxdomain.PricingInclusive {
		divisor := decimal.NewFromInt(1).Add(combinedRate.Div(hundred))
		taxableUnrounded = gross.Div(divisor)
	} else {
		taxableUnrounded = gross
	}

	var components []taxdomain.TaxComponent
	if rate.GSTRate.GreaterThan(decimal.Zero) {
		if intraState {
			half := rate.GSTRate.Div(decimal.NewFromInt(2))
			secondType := "SGST"
			if placeIsUT {
				secondType = "UTGST"
			}
			cgst, err := newComponent("CGST", half, taxableUnrounded, currency)
			if err != nil {
				return taxdomain.TaxLineResult{}, err
			}
			second, err := newComponent(secondType, half, taxableUnrounded, currency)
			if err != nil {
				return taxdomain.TaxLineResult{}, err
			}
			components = append(components, cgst, second)
		} else {
			igst, err := newComponent("IGST", rate.GSTRate, taxableUnrounded, currency)
			if err != nil {
				return taxdomain.TaxLineResult{}, err
			}
			components = append(components, igst)
		}
	}
	if rate.CessRate.GreaterThan(decimal.Zero) {
		// Cess is computed on the same taxable value as GST (the
		// transaction value), not on the GST-inclusive gross — standard
		// Compensation Cess practice.
		cess, err := newComponent("CESS", rate.CessRate, taxableUnrounded, currency)
		if err != nil {
			return taxdomain.TaxLineResult{}, err
		}
		components = append(components, cess)
	}

	// Round each component now — the single documented rounding point.
	lineTax, err := money.Zero(currency)
	if err != nil {
		return taxdomain.TaxLineResult{}, err
	}
	for i, c := range components {
		rounded := c.Amount.Round(money.RoundHalfUp)
		components[i].Amount = rounded
		lineTax, err = lineTax.Add(rounded)
		if err != nil {
			return taxdomain.TaxLineResult{}, err
		}
	}

	var taxableAmount, grossAmount money.Money
	if line.PricingMode == taxdomain.PricingInclusive {
		// Derive taxable as gross minus the already-rounded tax, so
		// taxable+tax reconciles exactly to the entered gross even though
		// each component was rounded independently.
		grossAmount = line.Amount.Round(money.RoundHalfUp)
		taxableAmount, err = grossAmount.Sub(lineTax)
		if err != nil {
			return taxdomain.TaxLineResult{}, err
		}
	} else {
		taxableAmount, err = money.New(taxableUnrounded, currency)
		if err != nil {
			return taxdomain.TaxLineResult{}, err
		}
		taxableAmount = taxableAmount.Round(money.RoundHalfUp)
		grossAmount, err = taxableAmount.Add(lineTax)
		if err != nil {
			return taxdomain.TaxLineResult{}, err
		}
	}

	return taxdomain.TaxLineResult{
		LineRef:        line.LineRef,
		HSNSACCode:     line.HSNSACCode,
		PricingMode:    line.PricingMode,
		GrossAmount:    grossAmount,
		TaxableAmount:  taxableAmount,
		TotalTax:       lineTax,
		Classification: string(rate.Classification),
		Components:     components,
		RateMasterID:   rate.ID,
	}, nil
}

// newComponent computes an UNROUNDED component amount (taxable * rate /
// 100) — rounding happens once, centrally, in calculateLine.
func newComponent(componentType string, rate, taxableUnrounded decimal.Decimal, currency string) (taxdomain.TaxComponent, error) {
	amount := taxableUnrounded.Mul(rate).Div(hundred)
	m, err := money.New(amount, currency)
	if err != nil {
		return taxdomain.TaxComponent{}, err
	}
	return taxdomain.TaxComponent{Type: componentType, Rate: rate, Amount: m}, nil
}
