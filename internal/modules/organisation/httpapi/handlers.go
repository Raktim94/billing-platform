// Package httpapi is the organisation module's HTTP transport layer.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"billing-platform/internal/modules/organisation/app"
	"billing-platform/internal/modules/organisation/domain"
	httpx "billing-platform/internal/platform/http"
	"billing-platform/internal/platform/permissions"
)

type Handlers struct {
	svc *app.Service
}

func NewHandlers(svc *app.Service) *Handlers {
	return &Handlers{svc: svc}
}

// Mount registers organisation's routes under an already-authenticated
// router group — every handler here requires httpx.PrincipalFromContext
// to be populated, so the composition root must mount this behind
// identity's RequireAuth middleware.
func (h *Handlers) Mount(r chi.Router) {
	r.Get("/organisation", h.getOrganisation)
	r.Get("/legal-entities", h.listLegalEntities)
	r.Post("/legal-entities", h.createLegalEntity)
	r.Get("/branches", h.listBranches)
	r.Post("/branches", h.createBranch)
	r.Post("/warehouses", h.createWarehouse)
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

func (h *Handlers) getOrganisation(w http.ResponseWriter, r *http.Request) {
	org, err := h.svc.GetOrganisation(r.Context(), principal(r))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, org)
}

func (h *Handlers) listLegalEntities(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListLegalEntities(r.Context(), principal(r))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"legal_entities": list})
}

type createLegalEntityRequest struct {
	LegalName        string `json:"legal_name"`
	CountryCode      string `json:"country_code"`
	BaseCurrencyCode string `json:"base_currency_code"`
}

func (h *Handlers) createLegalEntity(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createLegalEntityRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	le, err := h.svc.CreateLegalEntity(r.Context(), principal(r), app.CreateLegalEntityParams{
		LegalName: req.LegalName, CountryCode: req.CountryCode, BaseCurrencyCode: req.BaseCurrencyCode,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, le)
}

func (h *Handlers) listBranches(w http.ResponseWriter, r *http.Request) {
	list, err := h.svc.ListBranches(r.Context(), principal(r))
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"branches": list})
}

type createBranchRequest struct {
	LegalEntityID uuid.UUID `json:"legal_entity_id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	Timezone      string    `json:"timezone"`
}

func (h *Handlers) createBranch(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createBranchRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	b, err := h.svc.CreateBranch(r.Context(), principal(r), app.CreateBranchParams{
		LegalEntityID: req.LegalEntityID, Code: req.Code, Name: req.Name, Timezone: req.Timezone,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, b)
}

type createWarehouseRequest struct {
	BranchID uuid.UUID `json:"branch_id"`
	Code     string    `json:"code"`
	Name     string    `json:"name"`
}

func (h *Handlers) createWarehouse(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createWarehouseRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	wh, err := h.svc.CreateWarehouse(r.Context(), principal(r), app.CreateWarehouseParams{
		BranchID: req.BranchID, Code: req.Code, Name: req.Name,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, wh)
}
