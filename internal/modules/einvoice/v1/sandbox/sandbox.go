// Package sandbox is the real, network-calling EInvoiceProvider adapter
// targeting NIC's e-Invoice sandbox (einv-apisandbox.nic.in,
// docs/research.md). It is wired into apps/worker as an available provider
// choice but is deliberately NEVER exercised by go test — brief Rule 17
// ("never use production API credentials during automated tests") extends
// here to the sandbox too, since even sandbox calls require a real
// registered sandbox account and network access this codebase's test
// suite must not depend on. Treat this package as reviewed-by-reading,
// not proven-by-testing, until an operator with real sandbox credentials
// exercises it manually (docs/operations/, a follow-up doc).
//
// The exact request/response JSON shapes below follow the NIC e-Invoice
// API's publicly documented schema at the time of writing
// (docs/research.md's "e-Invoice JSON schema v1.1 (INV-01)" note) — NIC
// has changed this schema with limited notice before (the 2026-08-01
// Ship-to-GSTIN change), so this adapter is versioned (package path
// v1/sandbox) specifically so a future schema break becomes a new v2
// package, not a silent edit here that could break the pinned v1 contract.
package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"billing-platform/internal/modules/einvoice/domain"
)

const DefaultBaseURL = "https://einv-apisandbox.nic.in"

type Credentials struct {
	ClientID     string
	ClientSecret string
	GSTIN        string
	Username     string
	Password     string
}

type Provider struct {
	baseURL     string
	httpClient  *http.Client
	credentials Credentials
	authToken   string
}

func New(baseURL string, creds Credentials, httpClient *http.Client) *Provider {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Provider{baseURL: baseURL, credentials: creds, httpClient: httpClient}
}

var _ domain.EInvoiceProvider = (*Provider)(nil)

func (p *Provider) Authenticate(ctx context.Context) error {
	var resp struct {
		Status    int    `json:"Status"`
		AuthToken string `json:"AuthToken"`
	}
	body := map[string]string{
		"UserName": p.credentials.Username,
		"Password": p.credentials.Password,
		"ClientID": p.credentials.ClientID,
	}
	if err := p.call(ctx, http.MethodPost, "/eivital/v1.04/auth", body, &resp); err != nil {
		return fmt.Errorf("einvoice sandbox: authenticate: %w", err)
	}
	if resp.AuthToken == "" {
		return fmt.Errorf("einvoice sandbox: authenticate: empty AuthToken in response")
	}
	p.authToken = resp.AuthToken
	return nil
}

func (p *Provider) GenerateIRN(ctx context.Context, req domain.IRNRequest) (domain.IRNResponse, error) {
	var resp struct {
		Irn           string `json:"Irn"`
		AckNo         string `json:"AckNo"`
		AckDt         string `json:"AckDt"`
		SignedInvoice string `json:"SignedInvoice"`
		SignedQRCode  string `json:"SignedQRCode"`
		Status        string `json:"Status"`
	}
	if err := p.call(ctx, http.MethodPost, "/eicore/v1.03/Invoice", buildIRNPayload(req), &resp); err != nil {
		return domain.IRNResponse{}, fmt.Errorf("einvoice sandbox: generate IRN: %w", err)
	}
	ackDate, _ := time.Parse("02-01-2006 15:04:05", resp.AckDt)
	return domain.IRNResponse{
		IRN: resp.Irn, AckNumber: resp.AckNo, AckDate: ackDate,
		SignedInvoice: resp.SignedInvoice, SignedQRCode: resp.SignedQRCode, Status: resp.Status,
	}, nil
}

func (p *Provider) GetIRN(ctx context.Context, irn string) (domain.IRNResponse, error) {
	var resp struct {
		Irn    string `json:"Irn"`
		Status string `json:"Status"`
	}
	if err := p.call(ctx, http.MethodGet, "/eicore/v1.03/Invoice/irn/"+irn, nil, &resp); err != nil {
		return domain.IRNResponse{}, fmt.Errorf("einvoice sandbox: get IRN: %w", err)
	}
	return domain.IRNResponse{IRN: resp.Irn, Status: resp.Status}, nil
}

func (p *Provider) GetIRNByDocument(ctx context.Context, docType, docNo string, docDate time.Time) (domain.IRNResponse, error) {
	path := fmt.Sprintf("/eicore/v1.03/Invoice/%s/%s/%s", docType, docNo, docDate.Format("02-01-2006"))
	var resp struct {
		Irn    string `json:"Irn"`
		Status string `json:"Status"`
	}
	if err := p.call(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return domain.IRNResponse{}, fmt.Errorf("einvoice sandbox: get IRN by document: %w", err)
	}
	return domain.IRNResponse{IRN: resp.Irn, Status: resp.Status}, nil
}

func (p *Provider) CancelIRN(ctx context.Context, irn, reason string) error {
	body := map[string]string{"Irn": irn, "CnlRsn": "1", "CnlRem": reason}
	return p.call(ctx, http.MethodPost, "/eicore/v1.03/Invoice/Cancel", body, nil)
}

func (p *Provider) GenerateEWayBillByIRN(ctx context.Context, irn string, transport domain.TransportDetails) (domain.EWBResponse, error) {
	var resp struct {
		EwbNo        int64  `json:"EwbNo"`
		EwbDt        string `json:"EwbDt"`
		EwbValidTill string `json:"EwbValidTill"`
	}
	body := map[string]any{
		"Irn": irn, "TransId": transport.TransporterID, "TransName": transport.TransporterName,
		"TransMode": transport.TransportMode, "VehNo": transport.VehicleNumber,
		"Distance": transport.DistanceKM.String(), "ShipToGstin": transport.ShipToGSTIN,
	}
	if err := p.call(ctx, http.MethodPost, "/ewaybillapi/v1.03/Ewayapi", body, &resp); err != nil {
		return domain.EWBResponse{}, fmt.Errorf("einvoice sandbox: generate EWB by IRN: %w", err)
	}
	validFrom, _ := time.Parse("02-01-2006 15:04:05", resp.EwbDt)
	validUntil, _ := time.Parse("02-01-2006 15:04:05", resp.EwbValidTill)
	return domain.EWBResponse{EWBNumber: fmt.Sprintf("%d", resp.EwbNo), ValidFrom: validFrom, ValidUntil: validUntil}, nil
}

func (p *Provider) CancelEWayBill(ctx context.Context, ewbNo, reason string) error {
	body := map[string]string{"EwbNo": ewbNo, "CancelRsnCode": "1", "CancelRmrk": reason}
	return p.call(ctx, http.MethodPost, "/ewaybillapi/v1.03/ewayapi/Cancel", body, nil)
}

func (p *Provider) GetEWayBillByIRN(ctx context.Context, irn string) (domain.EWBResponse, error) {
	var resp struct {
		EwbNo int64 `json:"EwbNo"`
	}
	if err := p.call(ctx, http.MethodGet, "/ewaybillapi/v1.03/ewayapi/irn/"+irn, nil, &resp); err != nil {
		return domain.EWBResponse{}, fmt.Errorf("einvoice sandbox: get EWB by IRN: %w", err)
	}
	return domain.EWBResponse{EWBNumber: fmt.Sprintf("%d", resp.EwbNo)}, nil
}

func (p *Provider) GetGSTIN(ctx context.Context, gstin string) (domain.GSTINInfo, error) {
	var resp struct {
		Gstin     string `json:"Gstin"`
		LegalName string `json:"LegalName"`
		TradeName string `json:"TradeName"`
		Status    string `json:"Status"`
	}
	if err := p.call(ctx, http.MethodGet, "/eivital/v1.04/Master/gstin/"+gstin, nil, &resp); err != nil {
		return domain.GSTINInfo{}, fmt.Errorf("einvoice sandbox: get GSTIN: %w", err)
	}
	return domain.GSTINInfo{GSTIN: resp.Gstin, LegalName: resp.LegalName, TradeName: resp.TradeName, Status: resp.Status}, nil
}

func (p *Provider) HealthCheck(ctx context.Context) error {
	return p.call(ctx, http.MethodGet, "/eivital/v1.04/health", nil, nil)
}

func buildIRNPayload(req domain.IRNRequest) map[string]any {
	lines := make([]map[string]any, 0, len(req.Lines))
	for i, l := range req.Lines {
		lines = append(lines, map[string]any{
			"SlNo": fmt.Sprintf("%d", i+1), "HsnCd": l.HSNSACCode, "PrdDesc": l.Description,
			"Qty": l.Quantity.String(), "UnitPrice": l.UnitPrice.String(),
			"TotAmt": l.TaxableValue.String(), "GstRt": l.GSTRate.String(), "TotItemVal": l.TaxAmount.Add(l.TaxableValue).String(),
		})
	}
	return map[string]any{
		"Version":    "1.1",
		"SellerDtls": map[string]string{"Gstin": req.SupplierGSTIN, "Stcd": req.SupplierState},
		"BuyerDtls":  map[string]string{"Gstin": req.BuyerGSTIN, "Stcd": req.BuyerState},
		"DocDtls": map[string]string{
			"Typ": req.DocumentType, "No": req.DocumentNumber, "Dt": req.DocumentDate.Format("02-01-2006"),
		},
		"ItemList": lines,
		"ValDtls": map[string]string{
			"AssVal": req.TaxableValue.String(), "TotInvVal": req.GrandTotal.String(), "TotTax": req.TotalTax.String(),
		},
	}
}

func (p *Provider) call(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.authToken)
	}
	req.Header.Set("client_id", p.credentials.ClientID)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		// Never echo the Authorization header or credentials into this
		// error — only the response body, which is the provider's own
		// (non-secret) error payload.
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}
