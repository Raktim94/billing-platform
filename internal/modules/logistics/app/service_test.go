package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"billing-platform/internal/modules/logistics/domain"
	"billing-platform/internal/platform/audit"
	"billing-platform/internal/platform/database"
	"billing-platform/internal/platform/permissions"
)

// fakeRunner runs fn directly with no real transaction — same shape as
// internal/modules/pricing/app's fakeRunner, duplicated locally per that
// file's own comment (no shared-test-helper import across module
// packages without an exported, production-reachable test double).
type fakeRunner struct{}

func (fakeRunner) RunScoped(ctx context.Context, orgID uuid.UUID, fn database.TxFunc) error {
	return fn(ctx)
}
func (fakeRunner) Run(ctx context.Context, fn database.TxFunc) error               { return fn(ctx) }
func (fakeRunner) SetOrganisationScope(ctx context.Context, orgID uuid.UUID) error { return nil }

type allowAllStore struct{}

func (allowAllStore) Grants(ctx context.Context, userID uuid.UUID) ([]permissions.Grant, error) {
	return []permissions.Grant{{PermissionCode: "logistics.manage"}, {PermissionCode: "logistics.view"}}, nil
}

type noopAudit struct{}

func (noopAudit) Record(ctx context.Context, e audit.Entry) error { return nil }

// fakeVehicleRepo is an in-memory, organisation-scoped stand-in — the
// scoping check inside Deactivate/GetByID matters: a test that skipped it
// wouldn't catch a regression to the exact defense-in-depth gap
// docs/adr/0006-getbyid-organisation-scoping.md fixed for this module.
type fakeVehicleRepo struct {
	mu   sync.Mutex
	byID map[uuid.UUID]*domain.Vehicle
}

func newFakeVehicleRepo() *fakeVehicleRepo {
	return &fakeVehicleRepo{byID: map[uuid.UUID]*domain.Vehicle{}}
}

func (r *fakeVehicleRepo) Create(ctx context.Context, v *domain.Vehicle) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *v
	r.byID[v.ID] = &cp
	return nil
}

func (r *fakeVehicleRepo) GetByID(ctx context.Context, orgID, id uuid.UUID) (*domain.Vehicle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.byID[id]
	if !ok || v.OrganisationID != orgID {
		return nil, domain.ErrNotFound
	}
	return v, nil
}

func (r *fakeVehicleRepo) ListByOrganisation(ctx context.Context, orgID uuid.UUID, activeOnly bool) ([]*domain.Vehicle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Vehicle
	for _, v := range r.byID {
		if v.OrganisationID != orgID {
			continue
		}
		if activeOnly && !v.Active {
			continue
		}
		out = append(out, v)
	}
	return out, nil
}

func (r *fakeVehicleRepo) Deactivate(ctx context.Context, orgID, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.byID[id]
	if !ok || v.OrganisationID != orgID {
		return domain.ErrNotFound
	}
	v.Active = false
	return nil
}

type fakeTransporterRepo struct{}

func (fakeTransporterRepo) Create(ctx context.Context, t *domain.Transporter) error { return nil }
func (fakeTransporterRepo) GetByID(ctx context.Context, orgID, id uuid.UUID) (*domain.Transporter, error) {
	return nil, domain.ErrNotFound
}
func (fakeTransporterRepo) ListByOrganisation(ctx context.Context, orgID uuid.UUID, activeOnly bool) ([]*domain.Transporter, error) {
	return nil, nil
}
func (fakeTransporterRepo) Deactivate(ctx context.Context, orgID, id uuid.UUID) error { return nil }

type fakePreferenceRepo struct {
	recorded []recordedUsage
}

type recordedUsage struct {
	orgID, customerID        uuid.UUID
	vehicleID, transporterID *uuid.UUID
}

func (r *fakePreferenceRepo) RecordUsage(ctx context.Context, orgID, customerPartyID uuid.UUID, vehicleID, transporterID *uuid.UUID) error {
	r.recorded = append(r.recorded, recordedUsage{orgID, customerPartyID, vehicleID, transporterID})
	return nil
}
func (r *fakePreferenceRepo) MostRecent(ctx context.Context, orgID, customerPartyID uuid.UUID) (*domain.Preference, error) {
	return nil, domain.ErrNotFound
}

func newTestService(vehicles domain.VehicleRepository) *Service {
	checker := permissions.NewChecker(allowAllStore{}, fakeRunner{})
	return NewService(fakeRunner{}, vehicles, fakeTransporterRepo{}, &fakePreferenceRepo{}, checker, noopAudit{})
}

func TestCreateVehicle_ScopesToOrganisationAndReturnsActive(t *testing.T) {
	vehicles := newFakeVehicleRepo()
	svc := newTestService(vehicles)
	principal := permissions.Principal{UserID: uuid.New(), OrganisationID: uuid.New()}

	v, err := svc.CreateVehicle(context.Background(), principal, CreateVehicleParams{RegistrationNumber: "KA01AB1234"})
	if err != nil {
		t.Fatalf("CreateVehicle: %v", err)
	}
	if v.OrganisationID != principal.OrganisationID {
		t.Fatalf("OrganisationID = %s, want %s", v.OrganisationID, principal.OrganisationID)
	}
	if !v.Active {
		t.Fatal("a newly created vehicle must start Active")
	}
	if v.RegistrationNumber != "KA01AB1234" {
		t.Fatalf("RegistrationNumber = %q, want KA01AB1234", v.RegistrationNumber)
	}
}

// TestListVehicles_CrossOrganisationIsolation is a regression test for
// the exact class of bug docs/adr/0006 fixed in this module's real pg
// implementation (VehicleRepo.GetByID/Deactivate previously had no
// organisation_id filter) — this fake enforces the same scoping so a
// future regression to an unscoped fake would be caught here too, not
// just in the (Docker-only) integration suite.
func TestListVehicles_CrossOrganisationIsolation(t *testing.T) {
	vehicles := newFakeVehicleRepo()
	svc := newTestService(vehicles)
	orgA := permissions.Principal{UserID: uuid.New(), OrganisationID: uuid.New()}
	orgB := permissions.Principal{UserID: uuid.New(), OrganisationID: uuid.New()}

	if _, err := svc.CreateVehicle(context.Background(), orgA, CreateVehicleParams{RegistrationNumber: "KA01AB1234"}); err != nil {
		t.Fatalf("CreateVehicle as org A: %v", err)
	}

	listB, err := svc.ListVehicles(context.Background(), orgB, false)
	if err != nil {
		t.Fatalf("ListVehicles as org B: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("org B saw %d of org A's vehicles, want 0", len(listB))
	}

	listA, err := svc.ListVehicles(context.Background(), orgA, false)
	if err != nil {
		t.Fatalf("ListVehicles as org A: %v", err)
	}
	if len(listA) != 1 {
		t.Fatalf("org A saw %d vehicles, want 1", len(listA))
	}
}

func TestDeactivateVehicle_CrossOrganisationRejected(t *testing.T) {
	vehicles := newFakeVehicleRepo()
	svc := newTestService(vehicles)
	orgA := permissions.Principal{UserID: uuid.New(), OrganisationID: uuid.New()}
	orgB := permissions.Principal{UserID: uuid.New(), OrganisationID: uuid.New()}

	v, err := svc.CreateVehicle(context.Background(), orgA, CreateVehicleParams{RegistrationNumber: "KA01AB1234"})
	if err != nil {
		t.Fatalf("CreateVehicle as org A: %v", err)
	}

	if err := svc.DeactivateVehicle(context.Background(), orgB, v.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("org B deactivating org A's vehicle: got %v, want ErrNotFound", err)
	}

	listA, err := svc.ListVehicles(context.Background(), orgA, true)
	if err != nil {
		t.Fatalf("ListVehicles as org A: %v", err)
	}
	if len(listA) != 1 {
		t.Fatal("org A's vehicle must still be active — org B's deactivate attempt must not have affected it")
	}

	if err := svc.DeactivateVehicle(context.Background(), orgA, v.ID); err != nil {
		t.Fatalf("DeactivateVehicle as org A (the actual owner): %v", err)
	}
	listA, err = svc.ListVehicles(context.Background(), orgA, true)
	if err != nil {
		t.Fatalf("ListVehicles as org A after deactivate: %v", err)
	}
	if len(listA) != 0 {
		t.Fatal("vehicle should no longer appear in the active-only list after its owning org deactivates it")
	}
}

func TestSuggestForCustomerForOtherModule_NoHistory_ReturnsNilsNotError(t *testing.T) {
	svc := newTestService(newFakeVehicleRepo())
	vehicleID, transporterID, err := svc.SuggestForCustomerForOtherModule(context.Background(), uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("a first-time customer with no preference history must not error, got %v", err)
	}
	if vehicleID != nil || transporterID != nil {
		t.Fatalf("expected nil/nil for a customer with no history, got vehicleID=%v transporterID=%v", vehicleID, transporterID)
	}
}

func TestRecordTransportUsageForOtherModule_SkipsWhenBothNil(t *testing.T) {
	prefs := &fakePreferenceRepo{}
	checker := permissions.NewChecker(allowAllStore{}, fakeRunner{})
	svc := NewService(fakeRunner{}, newFakeVehicleRepo(), fakeTransporterRepo{}, prefs, checker, noopAudit{})

	if err := svc.RecordTransportUsageForOtherModule(context.Background(), uuid.New(), uuid.New(), nil, nil); err != nil {
		t.Fatalf("expected no error when both vehicleID and transporterID are nil, got %v", err)
	}
	if len(prefs.recorded) != 0 {
		t.Fatal("must not write a preference row when there is nothing to record")
	}
}

func TestRecordTransportUsageForOtherModule_RecordsWhenEitherSet(t *testing.T) {
	prefs := &fakePreferenceRepo{}
	checker := permissions.NewChecker(allowAllStore{}, fakeRunner{})
	svc := NewService(fakeRunner{}, newFakeVehicleRepo(), fakeTransporterRepo{}, prefs, checker, noopAudit{})

	orgID, customerID, vehicleID := uuid.New(), uuid.New(), uuid.New()
	if err := svc.RecordTransportUsageForOtherModule(context.Background(), orgID, customerID, &vehicleID, nil); err != nil {
		t.Fatalf("RecordTransportUsageForOtherModule: %v", err)
	}
	if len(prefs.recorded) != 1 {
		t.Fatalf("got %d recorded usages, want 1", len(prefs.recorded))
	}
	got := prefs.recorded[0]
	if got.orgID != orgID || got.customerID != customerID || got.vehicleID == nil || *got.vehicleID != vehicleID {
		t.Fatalf("recorded usage = %+v, want org=%s customer=%s vehicle=%s", got, orgID, customerID, vehicleID)
	}
}
