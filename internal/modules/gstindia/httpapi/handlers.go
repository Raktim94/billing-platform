// Package httpapi is the gstindia module's HTTP transport layer — admin
// CRUD for tax_rate_master only. Mirrors internal/modules/catalogue/httpapi's
// shape.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"billing-platform/internal/modules/gstindia/app"
	"billing-platform/internal/modules/gstindia/domain"
	httpx "billing-platform/internal/platform/http"
	"billing-platform/internal/platform/permissions"
)

type Handlers struct{ svc *app.Service }

func NewHandlers(svc *app.Service) *Handlers { return &Handlers{svc: svc} }

func (h *Handlers) Mount(r chi.Router) {
	r.Post("/gst/tax-rates", h.createRate)
	r.Get("/gst/tax-rates/{hsn}", h.listRatesByHSN)
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

type createRateRequest struct {
	HSNSACCode     string  `json:"hsn_sac_code"`
	Classification string  `json:"classification"`
	GSTRate        string  `json:"gst_rate"`
	CessRate       string  `json:"cess_rate"`
	ValidFrom      string  `json:"valid_from"` // YYYY-MM-DD
	ValidTo        *string `json:"valid_to"`
}

func (h *Handlers) createRate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createRateRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	gstRate, err := decimal.NewFromString(req.GSTRate)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_GST_RATE", "gst_rate must be a decimal string."))
		return
	}
	cessRate := decimal.Zero
	if req.CessRate != "" {
		cessRate, err = decimal.NewFromString(req.CessRate)
		if err != nil {
			httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_CESS_RATE", "cess_rate must be a decimal string."))
			return
		}
	}
	validFrom, err := time.Parse("2006-01-02", req.ValidFrom)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_VALID_FROM", "valid_from must be YYYY-MM-DD."))
		return
	}
	var validTo *time.Time
	if req.ValidTo != nil && *req.ValidTo != "" {
		t, err := time.Parse("2006-01-02", *req.ValidTo)
		if err != nil {
			httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_VALID_TO", "valid_to must be YYYY-MM-DD."))
			return
		}
		validTo = &t
	}
	classification := domain.RateClassification(req.Classification)
	if classification == "" {
		classification = domain.ClassificationTaxable
	}
	rate, err := h.svc.CreateRate(r.Context(), principal(r), app.CreateRateParams{
		HSNSACCode: req.HSNSACCode, Classification: classification,
		GSTRate: gstRate, CessRate: cessRate, ValidFrom: validFrom, ValidTo: validTo,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, rate)
}

func (h *Handlers) listRatesByHSN(w http.ResponseWriter, r *http.Request) {
	hsn := chi.URLParam(r, "hsn")
	list, err := h.svc.ListRatesByHSN(r.Context(), principal(r), hsn)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tax_rates": list})
}
