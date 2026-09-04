package app

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"rechvix/internal/modules/catalogue/domain"
	"rechvix/internal/platform/importer"
	"rechvix/internal/platform/permissions"
)

// ImportProducts bulk-creates products from parsed spreadsheet rows
// (brief §53). Expected columns (case-sensitive header match): name,
// hsn_sac_code (optional), base_uom_code (must already exist for this
// organisation — create units first). Every row gets an outcome in the
// returned Report — a malformed row is recorded as an error, never
// silently skipped. Duplicate detection is by exact, case-insensitive
// product name within the organisation.
//
// dryRun=true validates and reports without writing anything — the
// caller can show the report to a user before committing.
func (s *Service) ImportProducts(ctx context.Context, principal permissions.Principal, rows []importer.Row, dryRun bool) (importer.Report, error) {
	if err := s.manage(ctx, principal); err != nil {
		return importer.Report{}, err
	}
	b := importer.NewBuilder(dryRun)

	var existingNames map[string]bool
	err := s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		existing, err := s.products.ListByOrganisation(ctx, principal.OrganisationID)
		if err != nil {
			return err
		}
		existingNames = make(map[string]bool, len(existing))
		for _, p := range existing {
			existingNames[strings.ToLower(strings.TrimSpace(p.Name))] = true
		}

		units, err := s.units.ListByOrganisation(ctx, principal.OrganisationID)
		if err != nil {
			return err
		}
		unitByCode := make(map[string]uuid.UUID, len(units))
		for _, u := range units {
			unitByCode[strings.ToUpper(u.Code)] = u.ID
		}

		for _, row := range rows {
			name := strings.TrimSpace(row.Fields["name"])
			hsnSac := strings.TrimSpace(row.Fields["hsn_sac_code"])
			uomCode := strings.ToUpper(strings.TrimSpace(row.Fields["base_uom_code"]))

			if name == "" {
				b.Error(row.Number, "name is required")
				continue
			}
			uomID, ok := unitByCode[uomCode]
			if !ok {
				b.Error(row.Number, "base_uom_code %q does not match any existing unit of measure for this organisation", uomCode)
				continue
			}
			key := strings.ToLower(name)
			if existingNames[key] {
				b.Duplicate(row.Number, "a product named %q already exists", name)
				continue
			}

			if dryRun {
				b.Valid(row.Number)
				existingNames[key] = true // a later row in the same file with the same name is still a duplicate
				continue
			}

			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			now := s.now()
			p := &domain.Product{ID: id, OrganisationID: principal.OrganisationID, BaseUOMID: uomID,
				Name: name, HSNSACCode: hsnSac, Status: domain.StatusActive, CreatedAt: now, UpdatedAt: now}
			if err := s.products.Create(ctx, p); err != nil {
				return err
			}
			existingNames[key] = true
			b.Committed(row.Number)
		}
		return nil
	})
	if err != nil {
		return importer.Report{}, err
	}
	return b.Report(), nil
}
