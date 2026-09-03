// Package mock implements domain.EInvoiceProvider with canned, deterministic
// responses and no network calls whatsoever. This is the ONLY provider
// implementation the automated test suite is allowed to exercise (brief
// Rule 17: never use production — or even sandbox — API credentials during
// automated tests). See v1/sandbox for the real NIC-sandbox-calling
// adapter, which this package's tests never touch.
package mock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"billing-platform/internal/modules/einvoice/domain"
)

// Provider is safe for concurrent use. FailNextGenerateIRN lets a test
// simulate exactly one government-API outage (Scenario L's building
// block) without needing a real network fault.
type Provider struct {
	mu                  sync.Mutex
	failNextGenerateIRN error
	generateIRNCalls    int
}

func New() *Provider {
	return &Provider{}
}

var _ domain.EInvoiceProvider = (*Provider)(nil)

// FailNextGenerateIRN makes the NEXT GenerateIRN call return err instead of
// a canned success — and only that one call; the one after it succeeds
// normally. Used to test the FAILED_RETRYABLE → retry → GENERATED path and
// the "government API outage doesn't corrupt the sale" scenario.
func (p *Provider) FailNextGenerateIRN(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failNextGenerateIRN = err
}

// GenerateIRNCallCount reports how many times GenerateIRN has actually run
// (including failed attempts) — the idempotency test uses this to prove a
// second processing of the same already-GENERATED document does NOT call
// the provider again.
func (p *Provider) GenerateIRNCallCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.generateIRNCalls
}

func (p *Provider) Authenticate(ctx context.Context) error { return nil }

func (p *Provider) GenerateIRN(ctx context.Context, req domain.IRNRequest) (domain.IRNResponse, error) {
	p.mu.Lock()
	p.generateIRNCalls++
	failErr := p.failNextGenerateIRN
	p.failNextGenerateIRN = nil
	p.mu.Unlock()

	if failErr != nil {
		return domain.IRNResponse{}, failErr
	}

	// Deterministic, fixture-shaped canned response — a real IRN is a
	// 64-character hex hash of the invoice's key fields; this mirrors
	// that shape closely enough for schema/persistence round-trip tests
	// without claiming to BE NIC's actual hashing algorithm.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s", req.SupplierGSTIN, req.DocumentNumber, req.DocumentDate.Format("2006-01-02"), req.GrandTotal.String())))
	irn := hex.EncodeToString(sum[:])
	return domain.IRNResponse{
		IRN:           irn,
		AckNumber:     "MOCKACK" + irn[:12],
		AckDate:       time.Now().UTC(),
		SignedInvoice: "mock-signed-invoice-" + irn[:8],
		SignedQRCode:  "mock-qr-payload-" + irn[:8],
		Status:        "ACT",
	}, nil
}

func (p *Provider) GetIRN(ctx context.Context, irn string) (domain.IRNResponse, error) {
	return domain.IRNResponse{IRN: irn, Status: "ACT"}, nil
}

func (p *Provider) GetIRNByDocument(ctx context.Context, docType, docNo string, docDate time.Time) (domain.IRNResponse, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("lookup|%s|%s|%s", docType, docNo, docDate.Format("2006-01-02"))))
	return domain.IRNResponse{IRN: hex.EncodeToString(sum[:]), Status: "ACT"}, nil
}

func (p *Provider) CancelIRN(ctx context.Context, irn, reason string) error { return nil }

func (p *Provider) GenerateEWayBillByIRN(ctx context.Context, irn string, transport domain.TransportDetails) (domain.EWBResponse, error) {
	sum := sha256.Sum256([]byte("ewb|" + irn))
	now := time.Now().UTC()
	return domain.EWBResponse{
		EWBNumber:  hex.EncodeToString(sum[:6]),
		ValidFrom:  now,
		ValidUntil: now.Add(24 * time.Hour),
	}, nil
}

func (p *Provider) CancelEWayBill(ctx context.Context, ewbNo, reason string) error { return nil }

func (p *Provider) GetEWayBillByIRN(ctx context.Context, irn string) (domain.EWBResponse, error) {
	sum := sha256.Sum256([]byte("ewb|" + irn))
	now := time.Now().UTC()
	return domain.EWBResponse{EWBNumber: hex.EncodeToString(sum[:6]), ValidFrom: now, ValidUntil: now.Add(24 * time.Hour)}, nil
}

func (p *Provider) GetGSTIN(ctx context.Context, gstin string) (domain.GSTINInfo, error) {
	return domain.GSTINInfo{GSTIN: gstin, LegalName: "Mock Registered Entity", Status: "Active"}, nil
}

func (p *Provider) HealthCheck(ctx context.Context) error { return nil }
