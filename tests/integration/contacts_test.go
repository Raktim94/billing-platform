//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	contactsapp "billing-platform/internal/modules/contacts/app"
	contactsdomain "billing-platform/internal/modules/contacts/domain"
	contactspg "billing-platform/internal/modules/contacts/pg"
	"billing-platform/internal/platform/audit"
	"billing-platform/internal/platform/permissions"
)

func newTestContactsService(t *testing.T) *contactsapp.Service {
	t.Helper()
	return contactsapp.NewService(
		sharedPool,
		contactspg.NewPartyRepo(sharedPool),
		contactspg.NewAddressRepo(sharedPool),
		contactspg.NewTaxRegistrationRepo(sharedPool),
		permissions.NewChecker(permissions.NewPGStore(sharedPool), sharedPool),
		audit.NewPGRecorder(sharedPool),
	)
}

func TestContacts_Party_Address_TaxRegistration_CRUD(t *testing.T) {
	ctx := context.Background()
	svc := newTestContactsService(t)
	principal := bootstrapOwnerPrincipal(t, ctx)

	party, err := svc.CreateParty(ctx, principal, contactsapp.CreatePartyParams{
		PartyType: contactsdomain.PartyBoth, LegalName: "Integration Test Traders", CurrencyCode: "INR",
	})
	if err != nil {
		t.Fatalf("CreateParty: %v", err)
	}
	if party.PartyType != contactsdomain.PartyBoth {
		t.Fatalf("PartyType = %s, want BOTH", party.PartyType)
	}

	addr, err := svc.AddAddress(ctx, principal, contactsapp.AddAddressParams{
		PartyID: party.ID, AddressType: contactsdomain.AddressBilling, Line1: "1 Test Street", CountryCode: "IN", IsDefault: true,
	})
	if err != nil {
		t.Fatalf("AddAddress: %v", err)
	}
	addrs, err := svc.ListAddresses(ctx, principal, party.ID)
	if err != nil {
		t.Fatalf("ListAddresses: %v", err)
	}
	if len(addrs) != 1 || addrs[0].ID != addr.ID {
		t.Fatalf("ListAddresses = %+v, want exactly the one just added", addrs)
	}

	gstin := "29ABCDE1234F1Z" + uuid.NewString()[:1]
	reg, err := svc.AddTaxRegistration(ctx, principal, contactsapp.AddTaxRegistrationParams{
		PartyID: party.ID, CountryCode: "IN", RegistrationNumber: gstin, StateCode: "29", IsPrimary: true,
	})
	if err != nil {
		t.Fatalf("AddTaxRegistration: %v", err)
	}

	looked, err := svc.LookupByRegistrationNumber(ctx, principal, gstin)
	if err != nil {
		t.Fatalf("LookupByRegistrationNumber: %v", err)
	}
	if looked.ID != reg.ID {
		t.Fatalf("LookupByRegistrationNumber returned %s, want %s", looked.ID, reg.ID)
	}

	// Duplicate registration number for the same party must be rejected,
	// not silently accepted as a second row (migrations/0009_contacts.up.sql
	// UNIQUE(organisation_id, party_id, registration_number)).
	if _, err := svc.AddTaxRegistration(ctx, principal, contactsapp.AddTaxRegistrationParams{
		PartyID: party.ID, CountryCode: "IN", RegistrationNumber: gstin,
	}); !errors.Is(err, contactsdomain.ErrDuplicateRegistration) {
		t.Fatalf("expected ErrDuplicateRegistration on a repeated registration number, got %v", err)
	}
}

func TestContacts_CreateParty_RejectsInvalidPartyType(t *testing.T) {
	ctx := context.Background()
	svc := newTestContactsService(t)
	principal := bootstrapOwnerPrincipal(t, ctx)

	_, err := svc.CreateParty(ctx, principal, contactsapp.CreatePartyParams{
		PartyType: "VENDOR", LegalName: "Should Not Be Created", CurrencyCode: "INR",
	})
	if !errors.Is(err, contactsdomain.ErrInvalidPartyType) {
		t.Fatalf("expected ErrInvalidPartyType, got %v", err)
	}
}

// TestContacts_RLS_BlocksCrossOrganisationPartyRead is the contacts
// module's Scenario G building block: Organisation B must not be able to
// read Organisation A's customer/supplier record.
func TestContacts_RLS_BlocksCrossOrganisationPartyRead(t *testing.T) {
	ctx := context.Background()
	svc := newTestContactsService(t)
	principalA := bootstrapOwnerPrincipal(t, ctx)
	principalB := bootstrapOwnerPrincipal(t, ctx)

	party, err := svc.CreateParty(ctx, principalA, contactsapp.CreatePartyParams{
		PartyType: contactsdomain.PartyCustomer, LegalName: "Org A Private Customer", CurrencyCode: "INR",
	})
	if err != nil {
		t.Fatalf("CreateParty as A: %v", err)
	}

	if _, err := svc.GetParty(ctx, principalB, party.ID); !errors.Is(err, contactsdomain.ErrNotFound) {
		t.Fatalf("GetParty as B for A's party: got err=%v, want ErrNotFound", err)
	}
	if _, err := svc.GetParty(ctx, principalA, party.ID); err != nil {
		t.Fatalf("GetParty as A for its own party: %v", err)
	}
}
