package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"billing-platform/internal/modules/notifications/app"
	"billing-platform/internal/modules/notifications/domain"
	httpx "billing-platform/internal/platform/http"
	"billing-platform/internal/platform/permissions"
)

type Handlers struct{ svc *app.Service }

func NewHandlers(svc *app.Service) *Handlers { return &Handlers{svc: svc} }

// Mount registers the authenticated document-sharing routes into the
// same authenticated group every other module mounts into
// (apps/server/main.go). MountPublic registers the UNAUTHENTICATED
// redeem endpoint separately — a share link's whole point is that the
// recipient has no session or API key (brief §21).
func (h *Handlers) Mount(r chi.Router) {
	r.Post("/share-links", h.create)
	r.Delete("/share-links/{id}", h.revoke)
	r.Post("/notifications/send", h.send)
}

func (h *Handlers) MountPublic(r chi.Router) {
	r.Get("/share/{token}", h.redeem)
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
	case errors.As(err, &forbidden):
		httpx.WriteError(w, r, httpx.NewForbidden("FORBIDDEN", "You do not have permission to perform this action."))
	case errors.Is(err, domain.ErrLinkInvalid):
		httpx.WriteError(w, r, httpx.NewNotFound("LINK_INVALID", "This link is invalid, expired, or revoked."))
	default:
		httpx.WriteError(w, r, &httpx.AppError{Status: http.StatusInternalServerError, Code: "INTERNAL_ERROR", Message: "An unexpected error occurred.", Cause: err})
	}
}

type createShareLinkRequest struct {
	DocumentType string    `json:"document_type"`
	DocumentID   uuid.UUID `json:"document_id"`
}

func (h *Handlers) create(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.PrincipalFromContext(r.Context())
	req, err := decodeJSON[createShareLinkRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	token, err := h.svc.CreateShareLink(r.Context(), principal, req.DocumentType, req.DocumentID)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	// Shown exactly once — the URL a caller builds from this token is the
	// only copy; the server never returns it again (brief §21).
	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"token": token})
}

func (h *Handlers) revoke(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.PrincipalFromContext(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_ID", "id must be a UUID."))
		return
	}
	if err := h.svc.RevokeShareLink(r.Context(), principal, id); err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

// redeem is intentionally minimal: it proves the token is valid and
// reports what it grants access to. Actually streaming the shared
// document's PDF bytes to an anonymous, un-authenticated recipient needs
// a dedicated "system-level" cross-module read path (every existing
// print/report method requires a permissions.Principal, which an
// anonymous link recipient by definition doesn't have) — flagged as a
// real, undone follow-up rather than quietly bypassed with a fabricated
// principal.
func (h *Handlers) redeem(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	docType, docID, err := h.svc.RedeemShareLink(r.Context(), token)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"document_type": docType, "document_id": docID})
}

type sendRequest struct {
	Channel      string    `json:"channel"`
	Recipient    string    `json:"recipient"`
	DocumentType string    `json:"document_type"`
	DocumentID   uuid.UUID `json:"document_id"`
	Subject      string    `json:"subject,omitempty"`
	BodyHTML     string    `json:"body_html,omitempty"`
	TemplateName string    `json:"template_name,omitempty"`
	TemplateArgs []string  `json:"template_args,omitempty"`
}

func (h *Handlers) send(w http.ResponseWriter, r *http.Request) {
	principal, _ := httpx.PrincipalFromContext(r.Context())
	req, err := decodeJSON[sendRequest](r)
	if err != nil {
		httpx.WriteError(w, r, httpx.NewBadRequest("INVALID_BODY", "Request body is malformed."))
		return
	}
	err = h.svc.QueueSend(r.Context(), principal, app.SendPayload{
		Channel: domain.Channel(req.Channel), Recipient: req.Recipient,
		DocumentType: req.DocumentType, DocumentID: req.DocumentID,
		Subject: req.Subject, BodyHTML: req.BodyHTML,
		TemplateName: req.TemplateName, TemplateArgs: req.TemplateArgs,
	})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}
