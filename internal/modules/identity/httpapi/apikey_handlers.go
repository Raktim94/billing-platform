package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"rechvix/internal/modules/identity/app"
	"rechvix/internal/modules/identity/domain"
	httpx "rechvix/internal/platform/http"
)

// MountAPIKeys registers the API key management routes into an
// already-authenticated route group (mounted alongside every other
// module in apps/server/main.go's RequireAuthOrAPIKey block) — creating
// or listing API keys itself requires a session or an existing key with
// apikeys.manage, checked inside app.Service.
func (h *Handlers) MountAPIKeys(r chi.Router) {
	r.Post("/api-keys", h.createAPIKey)
	r.Get("/api-keys", h.listAPIKeys)
	r.Delete("/api-keys/{id}", h.revokeAPIKey)
}

type createAPIKeyRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at,omitempty"`
	AllowedIP *string  `json:"allowed_ip,omitempty"`
}

func (h *Handlers) createAPIKey(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.PrincipalFromContext(r.Context())
	req, err := decodeJSON[createAPIKeyRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	scopes := make([]domain.APIScope, len(req.Scopes))
	for i, s := range req.Scopes {
		scopes[i] = domain.APIScope(s)
	}
	var expires *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "expires_at must be RFC3339."))
			return
		}
		expires = &t
	}
	created, err := h.svc.CreateAPIKey(r.Context(), principal, app.CreateAPIKeyParams{
		Name: req.Name, Scopes: scopes, ExpiresAt: expires, AllowedIP: req.AllowedIP,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	// The only response that ever contains the raw key — brief §36 "shown
	// only once." Every subsequent GET /api-keys returns key_prefix only.
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":         created.APIKey.ID,
		"name":       created.APIKey.Name,
		"key_prefix": created.APIKey.KeyPrefix,
		"scopes":     created.APIKey.Scopes,
		"key":        created.RawKey,
	})
}

func (h *Handlers) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.PrincipalFromContext(r.Context())
	keys, err := h.svc.ListAPIKeys(r.Context(), principal)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	type keyDTO struct {
		ID         string     `json:"id"`
		Name       string     `json:"name"`
		KeyPrefix  string     `json:"key_prefix"`
		Scopes     []string   `json:"scopes"`
		ExpiresAt  *time.Time `json:"expires_at,omitempty"`
		LastUsedAt *time.Time `json:"last_used_at,omitempty"`
		CreatedAt  time.Time  `json:"created_at"`
	}
	out := make([]keyDTO, 0, len(keys))
	for _, k := range keys {
		scopes := make([]string, len(k.Scopes))
		for i, s := range k.Scopes {
			scopes[i] = string(s)
		}
		out = append(out, keyDTO{
			ID: k.ID.String(), Name: k.Name, KeyPrefix: k.KeyPrefix, Scopes: scopes,
			ExpiresAt: k.ExpiresAt, LastUsedAt: k.LastUsedAt, CreatedAt: k.CreatedAt,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"api_keys": out})
}

func (h *Handlers) revokeAPIKey(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.PrincipalFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_ID", "id must be a UUID."))
		return
	}
	if err := h.svc.RevokeAPIKey(r.Context(), principal, id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}
