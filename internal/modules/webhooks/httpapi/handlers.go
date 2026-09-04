package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"rechvix/internal/modules/webhooks/app"
	httpx "rechvix/internal/platform/http"
	"rechvix/internal/platform/permissions"
)

type Handlers struct{ svc *app.Service }

func NewHandlers(svc *app.Service) *Handlers { return &Handlers{svc: svc} }

func decodeJSON[T any](r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&v)
	return v, err
}

// writeServiceError mirrors every other module's httpapi package (e.g.
// internal/modules/contacts/httpapi/handlers.go) — httpx.WriteError only
// special-cases *AppError, so a plain permissions.ErrForbidden needs
// translating to 403 here, and anything else must fall through to a
// generic 500 with the real cause only logged server-side (brief §57),
// never echoed back as the client-facing message.
func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var forbidden *permissions.ErrForbidden
	switch {
	case errors.As(err, &forbidden):
		httpx.WriteError(w, r, httpx.NewForbidden("FORBIDDEN", "You do not have permission to perform this action."))
	case errors.Is(err, app.ErrValidation):
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_REQUEST", err.Error()))
	default:
		httpx.WriteError(w, r, &httpx.AppError{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "An unexpected error occurred.", Cause: err})
	}
}

func (h *Handlers) Mount(r chi.Router) {
	r.Post("/webhooks/endpoints", h.register)
	r.Get("/webhooks/endpoints", h.list)
	r.Delete("/webhooks/endpoints/{id}", h.deactivate)
}

type registerRequest struct {
	URL              string   `json:"url"`
	SubscribedEvents []string `json:"subscribed_events"`
}

func (h *Handlers) register(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.PrincipalFromContext(r.Context())
	req, err := decodeJSON[registerRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	ep, secret, err := h.svc.RegisterEndpoint(r.Context(), principal, app.RegisterEndpointParams{
		URL: req.URL, SubscribedEvents: req.SubscribedEvents,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":                ep.ID,
		"url":               ep.URL,
		"subscribed_events": ep.SubscribedEvents,
		// Shown exactly once, same principle as an API key's raw value.
		"signing_secret": secret,
	})
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.PrincipalFromContext(r.Context())
	endpoints, err := h.svc.ListEndpoints(r.Context(), principal)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	type dto struct {
		ID               string   `json:"id"`
		URL              string   `json:"url"`
		SubscribedEvents []string `json:"subscribed_events"`
		IsActive         bool     `json:"is_active"`
	}
	out := make([]dto, 0, len(endpoints))
	for _, e := range endpoints {
		out = append(out, dto{ID: e.ID.String(), URL: e.URL, SubscribedEvents: e.SubscribedEvents, IsActive: e.IsActive})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"endpoints": out})
}

func (h *Handlers) deactivate(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.PrincipalFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_ID", "id must be a UUID."))
		return
	}
	if err := h.svc.DeactivateEndpoint(r.Context(), principal, id); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "deactivated"})
}
