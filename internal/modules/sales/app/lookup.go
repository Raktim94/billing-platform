package app

import (
	"context"

	"github.com/google/uuid"

	"rechvix/internal/platform/money"
	"rechvix/internal/platform/permissions"
)

// BillingLookupResult is one product's billing-counter search result
// (brief §24/§25: "Sales screen product search must feel instantaneous"
// and show stock/price/tax visibility in one call) — product+variant
// identity, current stock at the given warehouse, and the resolved price
// from the given price list, if either is supplied. This is the API-side
// groundwork the brief asks for; there is no UI yet to actually measure
// the <150ms target against, so this method's job is to make that target
// reachable later (one call, backed by the trigram index Stage 3 already
// built) rather than to prove the number itself.
type BillingLookupResult struct {
	ProductID         uuid.UUID
	ProductName       string
	HSNSACCode        string
	ProductVariantID  uuid.UUID
	SKUCode           string
	QuantityOnHand    string
	QuantityAvailable string
	UnitPrice         *money.Money
}

// BillingLookup searches products by name (reusing catalogue's Stage 3
// trigram index) and, for each match's first variant, attaches current
// stock at warehouseID (if provided) and a resolved price from
// priceListID (if provided, via pricing.Service.ResolvePrice). Requires
// sales.view — a billing operator's own permission, not catalogue.view/
// inventory.view/pricing.view separately, since this is explicitly the
// combined billing-screen lookup brief §24/§25 describes as one call.
func (s *Service) BillingLookup(ctx context.Context, principal permissions.Principal, query string, warehouseID *uuid.UUID, priceListID *uuid.UUID, limit int) ([]BillingLookupResult, error) {
	if err := s.view(ctx, principal); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	products, err := s.catalogue.SearchProducts(ctx, principal, query, limit)
	if err != nil {
		return nil, err
	}
	out := make([]BillingLookupResult, 0, len(products))
	for _, p := range products {
		variants, err := s.catalogue.ListVariantsByProduct(ctx, principal, p.ID)
		if err != nil || len(variants) == 0 {
			continue
		}
		v := variants[0]
		result := BillingLookupResult{
			ProductID: p.ID, ProductName: p.Name, HSNSACCode: p.HSNSACCode,
			ProductVariantID: v.ID, SKUCode: v.SKUCode,
		}
		if warehouseID != nil {
			bal, err := s.inventory.GetBalance(ctx, principal, *warehouseID, v.ID)
			if err == nil {
				result.QuantityOnHand = bal.QuantityOnHand.String()
				result.QuantityAvailable = bal.QuantityOnHand.Sub(bal.QuantityReserved).String()
			}
		}
		if priceListID != nil && s.pricing != nil {
			item, err := s.pricing.ResolvePrice(ctx, principal, *priceListID, v.ID, p.BaseUOMID)
			if err == nil && item != nil {
				price := item.Price
				result.UnitPrice = &price
			}
		}
		out = append(out, result)
	}
	return out, nil
}
