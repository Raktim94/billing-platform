// Package httpapi is logistics' HTTP transport layer — vehicle/
// transporter master data (docs/architecture.md §9b "New master data").
// Mirrors organisation/httpapi's shape; logistics.Service is already
// fully self-scoping (permission check + RunScoped happen inside the app
// layer), so these handlers are thin decode/encode wrappers only.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"rechvix/internal/modules/logistics/app"
	"rechvix/internal/modules/logistics/domain"
	httpx "rechvix/internal/platform/http"
	"rechvix/internal/platform/permissions"
)

type Handlers struct{ svc *app.Service }

func NewHandlers(svc *app.Service) *Handlers { return &Handlers{svc: svc} }

func (h *Handlers) Mount(r chi.Router) {
	r.Get("/logistics/vehicles", h.listVehicles)
	r.Post("/logistics/vehicles", h.createVehicle)
	r.Post("/logistics/vehicles/{id}/deactivate", h.deactivateVehicle)
	r.Get("/logistics/transporters", h.listTransporters)
	r.Post("/logistics/transporters", h.createTransporter)
	r.Post("/logistics/transporters/{id}/deactivate", h.deactivateTransporter)
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

func (h *Handlers) listVehicles(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active_only") != "false"
	list, err := h.svc.ListVehicles(r.Context(), principal(r), activeOnly)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"vehicles": list})
}

type createVehicleRequest struct {
	RegistrationNumber   string `json:"registration_number"`
	Nickname             string `json:"nickname"`
	VehicleType          string `json:"vehicle_type"`
	DefaultTransportMode string `json:"default_transport_mode"`
}

func (h *Handlers) createVehicle(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createVehicleRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	v, err := h.svc.CreateVehicle(r.Context(), principal(r), app.CreateVehicleParams{
		RegistrationNumber: req.RegistrationNumber, Nickname: req.Nickname,
		VehicleType: req.VehicleType, DefaultTransportMode: req.DefaultTransportMode,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, v)
}

func (h *Handlers) deactivateVehicle(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_ID", "id must be a UUID."))
		return
	}
	if err := h.svc.DeactivateVehicle(r.Context(), principal(r), id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) listTransporters(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active_only") != "false"
	list, err := h.svc.ListTransporters(r.Context(), principal(r), activeOnly)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"transporters": list})
}

type createTransporterRequest struct {
	Name                 string `json:"name"`
	TransporterID        string `json:"transporter_id"`
	GSTIN                string `json:"gstin"`
	Phone                string `json:"phone"`
	Address              string `json:"address"`
	DefaultTransportMode string `json:"default_transport_mode"`
}

func (h *Handlers) createTransporter(w http.ResponseWriter, r *http.Request) {
	req, err := decodeJSON[createTransporterRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	t, err := h.svc.CreateTransporter(r.Context(), principal(r), app.CreateTransporterParams{
		Name: req.Name, TransporterID: req.TransporterID, GSTIN: req.GSTIN,
		Phone: req.Phone, Address: req.Address, DefaultTransportMode: req.DefaultTransportMode,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, t)
}

func (h *Handlers) deactivateTransporter(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_ID", "id must be a UUID."))
		return
	}
	if err := h.svc.DeactivateTransporter(r.Context(), principal(r), id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
