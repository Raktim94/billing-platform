package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	einvoicedomain "billing-platform/internal/modules/einvoice/domain"
	"billing-platform/internal/modules/ewaybill/domain"
)

// fakeRecordRepo is an in-memory, organisation-scoped stand-in for
// domain.Repository — scoping matters here the same way it does in
// logistics/app's equivalent fake: a test that skipped the orgID check
// wouldn't catch a regression to the exact gap
// docs/adr/0006-getbyid-organisation-scoping.md fixed for this module's
// real pg implementation.
type fakeRecordRepo struct {
	mu   sync.Mutex
	byID map[uuid.UUID]*domain.Record
}

func newFakeRecordRepo() *fakeRecordRepo {
	return &fakeRecordRepo{byID: map[uuid.UUID]*domain.Record{}}
}

func (r *fakeRecordRepo) GetBySalesDocumentID(ctx context.Context, orgID, salesDocumentID uuid.UUID) (*domain.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *domain.Record
	for _, rec := range r.byID {
		if rec.OrganisationID != orgID || rec.SalesDocumentID != salesDocumentID {
			continue
		}
		if latest == nil || rec.CreatedAt.After(latest.CreatedAt) {
			latest = rec
		}
	}
	if latest == nil {
		return nil, domain.ErrNotFound
	}
	cp := *latest
	return &cp, nil
}

func (r *fakeRecordRepo) Create(ctx context.Context, rec *domain.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *rec
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = time.Now()
	}
	r.byID[rec.ID] = &cp
	return nil
}

func (r *fakeRecordRepo) UpdateStatus(ctx context.Context, orgID, id uuid.UUID, status domain.Status, f domain.UpdateFields) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byID[id]
	if !ok || rec.OrganisationID != orgID {
		return domain.ErrNotFound
	}
	rec.Status = status
	if f.EWBNumber != nil {
		rec.EWBNumber = f.EWBNumber
	}
	if f.ValidFrom != nil {
		rec.ValidFrom = f.ValidFrom
	}
	if f.ValidUntil != nil {
		rec.ValidUntil = f.ValidUntil
	}
	if f.ErrorMessage != nil {
		rec.ErrorMessage = f.ErrorMessage
	}
	if f.ClosedAt != nil {
		rec.ClosedAt = f.ClosedAt
	}
	if f.ClosedByRole != nil {
		rec.ClosedByRole = f.ClosedByRole
	}
	if f.CancelledAt != nil {
		rec.CancelledAt = f.CancelledAt
	}
	if f.CancelReason != nil {
		rec.CancelReason = f.CancelReason
	}
	return nil
}

func (r *fakeRecordRepo) AppendPartBHistory(ctx context.Context, orgID, id uuid.UUID, entry domain.PartBUpdate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.byID[id]
	if !ok || rec.OrganisationID != orgID {
		return domain.ErrNotFound
	}
	rec.PartBHistory = append(rec.PartBHistory, entry)
	return nil
}

// fakeProvider is a controllable EInvoiceProvider — FailNextGenerateEWayBill
// mirrors the real integration suite's provider test double (tests/
// integration's newTestFreePortalEwaybillService fixture uses the same
// "fail the next call" shape), so this unit test exercises the identical
// failure-path contract without needing Docker.
type fakeProvider struct {
	mu               sync.Mutex
	failNextGenerate error
	generateCalls    int
	cancelCalls      int
}

func (p *fakeProvider) FailNextGenerateEWayBill(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failNextGenerate = err
}

func (p *fakeProvider) Authenticate(ctx context.Context) error { return nil }
func (p *fakeProvider) GenerateIRN(ctx context.Context, req einvoicedomain.IRNRequest) (einvoicedomain.IRNResponse, error) {
	return einvoicedomain.IRNResponse{}, nil
}
func (p *fakeProvider) GetIRN(ctx context.Context, irn string) (einvoicedomain.IRNResponse, error) {
	return einvoicedomain.IRNResponse{}, nil
}
func (p *fakeProvider) GetIRNByDocument(ctx context.Context, docType, docNo string, docDate time.Time) (einvoicedomain.IRNResponse, error) {
	return einvoicedomain.IRNResponse{}, nil
}
func (p *fakeProvider) CancelIRN(ctx context.Context, irn, reason string) error { return nil }
func (p *fakeProvider) GenerateEWayBillByIRN(ctx context.Context, irn string, transport einvoicedomain.TransportDetails) (einvoicedomain.EWBResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.generateCalls++
	if p.failNextGenerate != nil {
		err := p.failNextGenerate
		p.failNextGenerate = nil
		return einvoicedomain.EWBResponse{}, err
	}
	return einvoicedomain.EWBResponse{
		EWBNumber: "331000000001", ValidFrom: time.Now(), ValidUntil: time.Now().Add(72 * time.Hour),
	}, nil
}
func (p *fakeProvider) CancelEWayBill(ctx context.Context, ewbNo, reason string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelCalls++
	return nil
}
func (p *fakeProvider) GetEWayBillByIRN(ctx context.Context, irn string) (einvoicedomain.EWBResponse, error) {
	return einvoicedomain.EWBResponse{}, nil
}
func (p *fakeProvider) GetGSTIN(ctx context.Context, gstin string) (einvoicedomain.GSTINInfo, error) {
	return einvoicedomain.GSTINInfo{}, nil
}
func (p *fakeProvider) HealthCheck(ctx context.Context) error { return nil }

var _ einvoicedomain.EInvoiceProvider = (*fakeProvider)(nil)

func TestGenerateForDocument_Succeeds_SetsEWBNumberAndStatus(t *testing.T) {
	records := newFakeRecordRepo()
	provider := &fakeProvider{}
	svc := NewService(records, provider, nil)
	orgID, docID := uuid.New(), uuid.New()

	rec, err := svc.GenerateForDocument(context.Background(), orgID, GenerateParams{
		SalesDocumentID: docID, IRN: "irn-1", VehicleNumber: "KA01AB1234", DistanceKM: decimal.NewFromInt(50),
	})
	if err != nil {
		t.Fatalf("GenerateForDocument: %v", err)
	}
	if rec.Status != domain.StatusGenerated {
		t.Fatalf("Status = %s, want GENERATED", rec.Status)
	}
	if rec.EWBNumber == nil || *rec.EWBNumber != "331000000001" {
		t.Fatalf("EWBNumber = %v, want 331000000001", rec.EWBNumber)
	}
	if provider.generateCalls != 1 {
		t.Fatalf("provider called %d times, want exactly 1", provider.generateCalls)
	}
}

// TestGenerateForDocument_Idempotent_TerminalRecordNotRegenerated is a
// regression test for the exact idempotency guarantee
// docs/TODO.md Stage 8 claims ("a reprocessed outbox event is a safe
// no-op... double-processing calls the provider exactly once") — calling
// GenerateForDocument twice for the same sales document must not call
// the government provider a second time once the first call succeeded.
func TestGenerateForDocument_Idempotent_TerminalRecordNotRegenerated(t *testing.T) {
	records := newFakeRecordRepo()
	provider := &fakeProvider{}
	svc := NewService(records, provider, nil)
	orgID, docID := uuid.New(), uuid.New()
	params := GenerateParams{SalesDocumentID: docID, IRN: "irn-1", VehicleNumber: "KA01AB1234", DistanceKM: decimal.NewFromInt(50)}

	first, err := svc.GenerateForDocument(context.Background(), orgID, params)
	if err != nil {
		t.Fatalf("first GenerateForDocument: %v", err)
	}
	second, err := svc.GenerateForDocument(context.Background(), orgID, params)
	if err != nil {
		t.Fatalf("second GenerateForDocument: %v", err)
	}
	if provider.generateCalls != 1 {
		t.Fatalf("provider called %d times across two GenerateForDocument calls, want exactly 1 (idempotent)", provider.generateCalls)
	}
	if second.ID != first.ID || second.EWBNumber == nil || *second.EWBNumber != *first.EWBNumber {
		t.Fatalf("second call returned a different record than the first: first=%+v second=%+v", first, second)
	}
}

func TestGenerateForDocument_ProviderFailure_MarksFailedRetryable(t *testing.T) {
	records := newFakeRecordRepo()
	provider := &fakeProvider{}
	provider.FailNextGenerateEWayBill(errors.New("simulated NIC outage"))
	svc := NewService(records, provider, nil)
	orgID, docID := uuid.New(), uuid.New()

	_, err := svc.GenerateForDocument(context.Background(), orgID, GenerateParams{
		SalesDocumentID: docID, IRN: "irn-1", VehicleNumber: "KA01AB1234", DistanceKM: decimal.NewFromInt(50),
	})
	if err == nil {
		t.Fatal("expected an error when the provider fails, got nil")
	}
	rec, getErr := records.GetBySalesDocumentID(context.Background(), orgID, docID)
	if getErr != nil {
		t.Fatalf("GetBySalesDocumentID after failure: %v", getErr)
	}
	if rec.Status != domain.StatusFailedRetryable {
		t.Fatalf("Status = %s, want FAILED_RETRYABLE", rec.Status)
	}
	if rec.ErrorMessage == nil || *rec.ErrorMessage == "" {
		t.Fatal("expected ErrorMessage to be recorded on failure")
	}

	// A retry after the outage clears must succeed and produce a real
	// EWB number — a FAILED_RETRYABLE record is not terminal, so the
	// idempotency short-circuit above must not block a genuine retry.
	rec2, err := svc.GenerateForDocument(context.Background(), orgID, GenerateParams{
		SalesDocumentID: docID, IRN: "irn-1", VehicleNumber: "KA01AB1234", DistanceKM: decimal.NewFromInt(50),
	})
	if err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
	if rec2.Status != domain.StatusGenerated {
		t.Fatalf("Status after retry = %s, want GENERATED", rec2.Status)
	}
	if provider.generateCalls != 2 {
		t.Fatalf("provider called %d times (1 failure + 1 retry), want 2", provider.generateCalls)
	}
}

func TestClose_RejectsInvalidRole(t *testing.T) {
	svc := NewService(newFakeRecordRepo(), &fakeProvider{}, nil)
	err := svc.Close(context.Background(), uuid.New(), uuid.New(), domain.ClosedByRole("WAREHOUSE_MANAGER"))
	if !errors.Is(err, domain.ErrInvalidCloseRole) {
		t.Fatalf("got %v, want ErrInvalidCloseRole", err)
	}
}

func TestClose_AcceptsEachDocumentedRole(t *testing.T) {
	for _, role := range []domain.ClosedByRole{domain.ClosedBySupplier, domain.ClosedByRecipient, domain.ClosedByTransporter, domain.ClosedByDriver} {
		records := newFakeRecordRepo()
		provider := &fakeProvider{}
		svc := NewService(records, provider, nil)
		orgID, docID := uuid.New(), uuid.New()
		rec, err := svc.GenerateForDocument(context.Background(), orgID, GenerateParams{
			SalesDocumentID: docID, IRN: "irn-1", VehicleNumber: "KA01AB1234", DistanceKM: decimal.NewFromInt(50),
		})
		if err != nil {
			t.Fatalf("GenerateForDocument: %v", err)
		}
		if err := svc.Close(context.Background(), orgID, rec.ID, role); err != nil {
			t.Fatalf("Close with role %s: %v", role, err)
		}
	}
}

func TestCancel_RequiresGeneratedStatus(t *testing.T) {
	records := newFakeRecordRepo()
	svc := NewService(records, &fakeProvider{}, nil)
	orgID, docID, id := uuid.New(), uuid.New(), uuid.New()
	// A DRAFT record with no EWB number — Cancel must refuse it.
	if err := records.Create(context.Background(), &domain.Record{ID: id, OrganisationID: orgID, SalesDocumentID: docID, Status: domain.StatusDraft}); err != nil {
		t.Fatalf("seeding a DRAFT record: %v", err)
	}
	err := svc.Cancel(context.Background(), orgID, docID, "customer refused delivery")
	if !errors.Is(err, domain.ErrNotGenerated) {
		t.Fatalf("Cancel on a non-GENERATED record: got %v, want ErrNotGenerated", err)
	}
}

func TestCancel_GeneratedRecord_CallsProviderAndMarksCancelled(t *testing.T) {
	records := newFakeRecordRepo()
	provider := &fakeProvider{}
	svc := NewService(records, provider, nil)
	orgID, docID := uuid.New(), uuid.New()
	rec, err := svc.GenerateForDocument(context.Background(), orgID, GenerateParams{
		SalesDocumentID: docID, IRN: "irn-1", VehicleNumber: "KA01AB1234", DistanceKM: decimal.NewFromInt(50),
	})
	if err != nil {
		t.Fatalf("GenerateForDocument: %v", err)
	}
	if err := svc.Cancel(context.Background(), orgID, docID, "wrong address"); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if provider.cancelCalls != 1 {
		t.Fatalf("provider CancelEWayBill called %d times, want 1", provider.cancelCalls)
	}
	got, err := records.GetBySalesDocumentID(context.Background(), orgID, docID)
	if err != nil {
		t.Fatalf("GetBySalesDocumentID after cancel: %v", err)
	}
	if got.ID != rec.ID || got.Status != domain.StatusCancelled {
		t.Fatalf("Status after Cancel = %s, want CANCELLED", got.Status)
	}
}

func TestGetRecordForDocument_CrossOrganisationReturnsNotFound(t *testing.T) {
	records := newFakeRecordRepo()
	provider := &fakeProvider{}
	svc := NewService(records, provider, nil)
	orgA, orgB, docID := uuid.New(), uuid.New(), uuid.New()
	if _, err := svc.GenerateForDocument(context.Background(), orgA, GenerateParams{
		SalesDocumentID: docID, IRN: "irn-1", VehicleNumber: "KA01AB1234", DistanceKM: decimal.NewFromInt(50),
	}); err != nil {
		t.Fatalf("GenerateForDocument as org A: %v", err)
	}
	if _, err := svc.GetRecordForDocument(context.Background(), orgB, docID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("org B reading org A's e-Way Bill record: got %v, want ErrNotFound", err)
	}
}

func TestUpdatePartB_AppendsToHistory(t *testing.T) {
	records := newFakeRecordRepo()
	svc := NewService(records, &fakeProvider{}, nil)
	orgID, docID := uuid.New(), uuid.New()
	rec, err := svc.GenerateForDocument(context.Background(), orgID, GenerateParams{
		SalesDocumentID: docID, IRN: "irn-1", VehicleNumber: "KA01AB1234", DistanceKM: decimal.NewFromInt(50),
	})
	if err != nil {
		t.Fatalf("GenerateForDocument: %v", err)
	}
	if err := svc.UpdatePartB(context.Background(), orgID, rec.ID, domain.PartBUpdate{VehicleNumber: "KA01AB9999", Reason: "vehicle breakdown"}); err != nil {
		t.Fatalf("UpdatePartB: %v", err)
	}
	got, err := records.GetBySalesDocumentID(context.Background(), orgID, docID)
	if err != nil {
		t.Fatalf("GetBySalesDocumentID: %v", err)
	}
	if len(got.PartBHistory) != 1 || got.PartBHistory[0].VehicleNumber != "KA01AB9999" {
		t.Fatalf("PartBHistory = %+v, want one entry for KA01AB9999", got.PartBHistory)
	}
}
