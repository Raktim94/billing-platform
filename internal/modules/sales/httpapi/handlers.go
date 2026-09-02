// Package httpapi is the sales module's HTTP transport layer. Mirrors
// internal/modules/purchases/httpapi's shape.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"billing-platform/internal/modules/sales/app"
	"billing-platform/internal/modules/sales/domain"
	"billing-platform/internal/modules/sales/printing"
	taxdomain "billing-platform/internal/modules/taxation/domain"
	httpx "billing-platform/internal/platform/http"
	"billing-platform/internal/platform/permissions"
)

type Handlers struct{ svc *app.Service }

func NewHandlers(svc *app.Service) *Handlers { return &Handlers{svc: svc} }

func (h *Handlers) Mount(r chi.Router) {
	r.Get("/sales/documents", h.listDocuments)
	r.Post("/sales/documents", h.createDocument)
	r.Get("/sales/documents/{id}", h.getDocument)
	r.Post("/sales/documents/{id}/lines", h.addLine)
	r.Post("/sales/documents/{id}/finalize", h.finalizeDocument)
	r.Post("/sales/documents/{id}/convert", h.convertDocument)
	r.Get("/sales/documents/{id}/print", h.printDocument)
	r.Get("/sales/billing-lookup", h.billingLookup)
}

func decodeJSON[T any](r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&v)
	return v, err
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var forbidden *permissions.ErrForbidden
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, r, httpx.NewNotFound("NOT_FOUND", "The requested resource was not found."))
	case errors.Is(err, domain.ErrInvalidDocumentType):
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_DOCUMENT_TYPE", "That is not a recognized sales document type."))
	case errors.Is(err, domain.ErrDocumentNotDraft):
		httpx.WriteError(w, r, httpx.NewConflict("DOCUMENT_NOT_DRAFT", "This document is not in DRAFT status and cannot be modified or finalized again."))
	case errors.Is(err, domain.ErrDocumentNotFinalized):
		httpx.WriteError(w, r, httpx.NewConflict("DOCUMENT_NOT_FINALIZED", "Only a FINALIZED document can be converted."))
	case errors.Is(err, domain.ErrEmptyDocument):
		httpx.WriteError(w, r, httpx.NewConflict("EMPTY_DOCUMENT", "A document needs at least one line before it can be finalized."))
	case errors.Is(err, domain.ErrDuplicateNumber):
		httpx.WriteError(w, r, httpx.NewConflict("DUPLICATE_NUMBER", "That document number is already in use."))
	case errors.As(err, &forbidden):
		httpx.WriteError(w, r, httpx.NewForbidden("FORBIDDEN", "You do not have permission to perform this action."))
	default:
		httpx.WriteError(w, r, &httpx.AppError{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "An unexpected error occurred.", Cause: err})
	}
}

func principal(r *http.Request) permissions.Principal {
	p, _ := httpx.PrincipalFromContext(r.Context())
	return p
}

type createDocumentRequest struct {
	LegalEntityID             uuid.UUID  `json:"legal_entity_id"`
	BranchID                  uuid.UUID  `json:"branch_id"`
	WarehouseID               uuid.UUID  `json:"warehouse_id"`
	CustomerPartyID           uuid.UUID  `json:"customer_party_id"`
	DocumentType              string     `json:"document_type"`
	ReferenceDocumentID       *uuid.UUID `json:"reference_document_id"`
	IssueDate                 *time.Time `json:"issue_date"`
	DueDate                   *time.Time `json:"due_date"`
	SupplyDate                *time.Time `json:"supply_date"`
	BillingAddressID          *uuid.UUID `json:"billing_address_id"`
	ShippingAddressID         *uuid.UUID `json:"shipping_address_id"`
	CustomerTaxRegistrationID *uuid.UUID `json:"customer_tax_registration_id"`
	PlaceOfSupplyStateCode    string     `json:"place_of_supply_state_code"`
	SalespersonUserID         *uuid.UUID `json:"salesperson_user_id"`
	PriceListID               *uuid.UUID `json:"price_list_id"`
	CurrencyCode              string     `json:"currency_code"`
	BaseCurrencyCode          string     `json:"base_currency_code"`
	ExchangeRate              string     `json:"exchange_rate"`
	PricingMode               string     `json:"pricing_mode"`
	CustomerReference         string     `json:"customer_reference"`
	Transporter               string     `json:"transporter"`
	VehicleNumber             string     `json:"vehicle_number"`
	ShippingTerms             string     `json:"shipping_terms"`
	Notes                     string     `json:"notes"`
	TermsAndConditions        string     `json:"terms_and_conditions"`
	PaymentTermsDays          int        `json:"payment_terms_days"`
}

func (h *Handlers) createDocument(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createDocumentRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	var issueDate time.Time
	if req.IssueDate != nil {
		issueDate = *req.IssueDate
	}
	exchangeRate := decimal.NewFromInt(1)
	if req.ExchangeRate != "" {
		exchangeRate, err = decimal.NewFromString(req.ExchangeRate)
		if err != nil {
			httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_EXCHANGE_RATE", "exchange_rate must be a decimal string."))
			return
		}
	}
	d, err := h.svc.CreateDocument(r.Context(), principal(r), app.CreateDocumentParams{
		LegalEntityID: req.LegalEntityID, BranchID: req.BranchID, WarehouseID: req.WarehouseID,
		CustomerPartyID: req.CustomerPartyID, DocumentType: domain.DocumentType(req.DocumentType),
		ReferenceDocumentID: req.ReferenceDocumentID, IssueDate: issueDate, DueDate: req.DueDate, SupplyDate: req.SupplyDate,
		BillingAddressID: req.BillingAddressID, ShippingAddressID: req.ShippingAddressID,
		CustomerTaxRegistrationID: req.CustomerTaxRegistrationID, PlaceOfSupplyStateCode: req.PlaceOfSupplyStateCode,
		SalespersonUserID: req.SalespersonUserID, PriceListID: req.PriceListID, CurrencyCode: req.CurrencyCode,
		BaseCurrencyCode: req.BaseCurrencyCode, ExchangeRate: exchangeRate, PricingMode: taxdomain.PricingMode(req.PricingMode),
		CustomerReference: req.CustomerReference, Transporter: req.Transporter, VehicleNumber: req.VehicleNumber,
		ShippingTerms: req.ShippingTerms, Notes: req.Notes, TermsAndConditions: req.TermsAndConditions,
		PaymentTermsDays: req.PaymentTermsDays,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, d)
}

func (h *Handlers) getDocument(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_ID", "id must be a UUID."))
		return
	}
	doc, lines, err := h.svc.GetDocument(r.Context(), principal(r), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"document": doc, "lines": lines})
}

func (h *Handlers) listDocuments(w http.ResponseWriter, r *http.Request) {
	var docType *domain.DocumentType
	if q := r.URL.Query().Get("document_type"); q != "" {
		t := domain.DocumentType(q)
		docType = &t
	}
	list, err := h.svc.ListDocuments(r.Context(), principal(r), docType)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"documents": list})
}

type addLineRequest struct {
	ProductVariantID   uuid.UUID `json:"product_variant_id"`
	UnitID             uuid.UUID `json:"unit_id"`
	Quantity           string    `json:"quantity"`
	UnitPrice          string    `json:"unit_price"`
	LineDiscountAmount string    `json:"line_discount_amount"`
	BatchCode          string    `json:"batch_code"`
	SerialCode         string    `json:"serial_code"`
}

func (h *Handlers) addLine(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_ID", "id must be a UUID."))
		return
	}
	req, err := decodeJSON[addLineRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	qty, err := decimal.NewFromString(req.Quantity)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_QUANTITY", "quantity must be a decimal string."))
		return
	}
	price, err := decimal.NewFromString(req.UnitPrice)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_UNIT_PRICE", "unit_price must be a decimal string."))
		return
	}
	discount := decimal.Zero
	if req.LineDiscountAmount != "" {
		discount, err = decimal.NewFromString(req.LineDiscountAmount)
		if err != nil {
			httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_DISCOUNT", "line_discount_amount must be a decimal string."))
			return
		}
	}
	line, err := h.svc.AddLine(r.Context(), principal(r), app.AddLineParams{
		DocumentID: id, ProductVariantID: req.ProductVariantID, UnitID: req.UnitID,
		Quantity: qty, UnitPrice: price, LineDiscountAmount: discount,
		BatchCode: req.BatchCode, SerialCode: req.SerialCode,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, line)
}

func (h *Handlers) finalizeDocument(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_ID", "id must be a UUID."))
		return
	}
	doc, err := h.svc.FinalizeDocument(r.Context(), principal(r), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, doc)
}

type convertDocumentRequest struct {
	TargetType string `json:"target_type"`
}

func (h *Handlers) convertDocument(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_ID", "id must be a UUID."))
		return
	}
	req, err := decodeJSON[convertDocumentRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	target, err := h.svc.ConvertDocument(r.Context(), principal(r), id, domain.DocumentType(req.TargetType))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, target)
}

// printDocument renders a finalized document to PDF. ?template= selects
// the layout (brief §19); defaults to the A4 GST invoice.
func (h *Handlers) printDocument(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_ID", "id must be a UUID."))
		return
	}
	tpl := printing.Template(r.URL.Query().Get("template"))
	if tpl == "" {
		tpl = printing.TemplateA4GSTInvoice
	}
	data, err := h.svc.BuildInvoiceData(r.Context(), principal(r), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	pdfBytes, err := printing.RenderPDF(tpl, *data)
	if err != nil {
		httpx.WriteError(w, r, &httpx.AppError{Status: http.StatusInternalServerError, Code: "RENDER_FAILED", Message: "Could not render the document.", Cause: err})
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdfBytes)
}

// billingLookup is the sales-screen search endpoint (brief §24/§25):
// product search + stock + price in one call.
func (h *Handlers) billingLookup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	var warehouseID, priceListID *uuid.UUID
	if v := r.URL.Query().Get("warehouse_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_WAREHOUSE_ID", "warehouse_id must be a UUID."))
			return
		}
		warehouseID = &id
	}
	if v := r.URL.Query().Get("price_list_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_PRICE_LIST_ID", "price_list_id must be a UUID."))
			return
		}
		priceListID = &id
	}
	results, err := h.svc.BillingLookup(r.Context(), principal(r), q, warehouseID, priceListID, 10)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}
