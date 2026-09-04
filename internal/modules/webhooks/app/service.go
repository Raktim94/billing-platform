// Package app is the webhooks module's application/use-case layer.
//
// Delivery reuses the Stage 8 transactional outbox in two hops, so retries
// are correctly scoped per-endpoint rather than per-source-event:
//
//  1. A source event happens elsewhere (sales.FinalizeDocument enqueues
//     "invoice.finalized"; einvoice enqueues "einvoice.generated"/
//     "einvoice.failed") — see docs/adr/0005-webhook-delivery-fanout.md.
//  2. HandlerForSourceEvent, registered against each of those event
//     types, fans out: for every active endpoint subscribed to that event
//     type, it enqueues ONE "webhook.delivery" outbox event
//     (idempotency_key = sourceEvent.ID + endpoint.ID). If endpoint A is
//     down and endpoint B isn't, A's retries never block or duplicate B's.
//  3. DeliverHandler, registered against "webhook.delivery", does the
//     actual signed HTTP POST for one (event, endpoint) pair.
package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"rechvix/internal/modules/webhooks/domain"
	"rechvix/internal/platform/audit"
	"rechvix/internal/platform/crypto"
	"rechvix/internal/platform/database"
	"rechvix/internal/platform/outbox"
	"rechvix/internal/platform/permissions"
)

// maxDeliveryAttempts bounds retries before a delivery is left FAILED
// permanently (outbox.Permanent) — the delivery attempts themselves are
// forever visible/queryable (webhook_deliveries + the terminal outbox
// row), satisfying brief §38's "dead-letter visibility" without a
// separate dead-letter table.
const maxDeliveryAttempts = 8

const EventTypeDelivery = "webhook.delivery"

// ErrValidation wraps every RegisterEndpoint input-validation failure, so
// the HTTP layer can map them to 400 via errors.Is without a growing
// switch of individual sentinels for what are all just "bad request body"
// cases.
var ErrValidation = errors.New("webhooks: invalid request")

type deliveryPayload struct {
	EndpointID uuid.UUID       `json:"endpoint_id"`
	EventType  string          `json:"event_type"`
	EventID    uuid.UUID       `json:"event_id"`
	Business   json.RawMessage `json:"business_payload"`
}

type Service struct {
	pool       database.Runner
	endpoints  domain.EndpointRepository
	deliveries domain.DeliveryLogRepository
	outbox     outbox.Writer
	perms      *permissions.Checker
	audit      audit.Recorder
	client     *http.Client
	now        func() time.Time
}

func NewService(
	pool database.Runner,
	endpoints domain.EndpointRepository,
	deliveries domain.DeliveryLogRepository,
	outboxWriter outbox.Writer,
	perms *permissions.Checker,
	recorder audit.Recorder,
) *Service {
	return &Service{
		pool: pool, endpoints: endpoints, deliveries: deliveries, outbox: outboxWriter, perms: perms, audit: recorder,
		client: &http.Client{Timeout: 10 * time.Second}, now: time.Now,
	}
}

// --- Endpoint management ---

type RegisterEndpointParams struct {
	URL              string
	SubscribedEvents []string
}

func (s *Service) RegisterEndpoint(ctx context.Context, principal permissions.Principal, p RegisterEndpointParams) (*domain.Endpoint, string, error) {
	if err := s.perms.Require(ctx, principal, "webhooks.manage", permissions.Scope{}); err != nil {
		return nil, "", err
	}
	if p.URL == "" {
		return nil, "", fmt.Errorf("%w: url is required", ErrValidation)
	}
	if len(p.SubscribedEvents) == 0 {
		return nil, "", fmt.Errorf("%w: at least one subscribed event is required", ErrValidation)
	}
	for _, e := range p.SubscribedEvents {
		if !domain.EventCatalog[e] {
			return nil, "", fmt.Errorf("%w: unrecognized event type %q", ErrValidation, e)
		}
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, "", fmt.Errorf("webhooks: generating endpoint id: %w", err)
	}
	_, secret, err := crypto.RandomToken(32)
	if err != nil {
		return nil, "", fmt.Errorf("webhooks: generating signing secret: %w", err)
	}
	ep := &domain.Endpoint{
		ID: id, OrganisationID: principal.OrganisationID, URL: p.URL, SigningSecret: secret,
		SubscribedEvents: p.SubscribedEvents, IsActive: true, CreatedAt: s.now(), CreatedBy: principal.UserID,
	}
	err = s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		return s.endpoints.Create(ctx, ep)
	})
	if err != nil {
		return nil, "", err
	}
	// The signing secret is returned exactly once here, same "shown once"
	// principle as an API key's raw value — every subsequent read of this
	// endpoint (ListEndpoints) omits it.
	return ep, secret, nil
}

func (s *Service) ListEndpoints(ctx context.Context, principal permissions.Principal) ([]*domain.Endpoint, error) {
	if err := s.perms.Require(ctx, principal, "webhooks.manage", permissions.Scope{}); err != nil {
		return nil, err
	}
	var out []*domain.Endpoint
	err := s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		var err error
		out, err = s.endpoints.ListForOrganisation(ctx, principal.OrganisationID)
		return err
	})
	return out, err
}

func (s *Service) DeactivateEndpoint(ctx context.Context, principal permissions.Principal, id uuid.UUID) error {
	if err := s.perms.Require(ctx, principal, "webhooks.manage", permissions.Scope{}); err != nil {
		return err
	}
	return s.pool.RunScoped(ctx, principal.OrganisationID, func(ctx context.Context) error {
		return s.endpoints.Deactivate(ctx, id)
	})
}

// --- Fan-out (hop 1 -> 2) ---

// HandlerForSourceEvent returns an outbox.Handler that fans a source
// domain event out to every active, subscribed endpoint. Register it
// against each event_type in domain.EventCatalog that a producer module
// actually enqueues (apps/worker wires this per event type it knows
// about — see that file for exactly which ones are wired vs. stubbed).
func (s *Service) HandlerForSourceEvent(eventType string) outbox.Handler {
	return func(ctx context.Context, event outbox.Event) error {
		endpoints, err := s.endpoints.ListSubscribed(ctx, event.OrganisationID, eventType)
		if err != nil {
			return fmt.Errorf("webhooks: listing subscribed endpoints: %w", err)
		}
		for _, ep := range endpoints {
			payload := deliveryPayload{EndpointID: ep.ID, EventType: eventType, EventID: event.ID, Business: event.Payload}
			idempotencyKey := "webhook:deliver:" + event.ID.String() + ":" + ep.ID.String()
			if err := s.outbox.Enqueue(ctx, event.OrganisationID, EventTypeDelivery, idempotencyKey, payload); err != nil {
				return fmt.Errorf("webhooks: enqueuing delivery for endpoint %s: %w", ep.ID, err)
			}
		}
		return nil
	}
}

// --- Delivery (hop 2 -> HTTP) ---

// DeliverHandler performs the actual signed HTTP POST for one
// (event, endpoint) pair. Registered against EventTypeDelivery.
func (s *Service) DeliverHandler() outbox.Handler {
	return func(ctx context.Context, event outbox.Event) error {
		var p deliveryPayload
		if err := json.Unmarshal(event.Payload, &p); err != nil {
			return outbox.Permanent(fmt.Errorf("webhooks: malformed delivery payload: %w", err))
		}
		ep, err := s.endpoints.GetByID(ctx, p.EndpointID)
		if err != nil {
			// The endpoint was deleted/deactivated between enqueue and
			// delivery — nothing left to deliver to, and retrying can
			// never fix that.
			return outbox.Permanent(fmt.Errorf("webhooks: loading endpoint: %w", err))
		}

		attemptedAt := s.now()
		status, deliverErr := s.post(ctx, *ep, p, event.ID, attemptedAt)

		logID, idErr := uuid.NewV7()
		if idErr == nil {
			var httpStatus *int
			var errMsg *string
			if status != 0 {
				httpStatus = &status
			}
			if deliverErr != nil {
				m := deliverErr.Error()
				errMsg = &m
			}
			_ = s.deliveries.Create(ctx, &domain.DeliveryLog{
				ID: logID, OrganisationID: event.OrganisationID, WebhookEndpointID: ep.ID,
				EventType: p.EventType, EventID: p.EventID, HTTPStatus: httpStatus,
				Succeeded: deliverErr == nil, ErrorMessage: errMsg, AttemptedAt: attemptedAt,
			})
		}

		if deliverErr == nil {
			return nil
		}
		if event.Attempts >= maxDeliveryAttempts {
			return outbox.Permanent(fmt.Errorf("webhooks: giving up after %d attempts: %w", event.Attempts, deliverErr))
		}
		return deliverErr
	}
}

func (s *Service) post(ctx context.Context, ep domain.Endpoint, p deliveryPayload, eventID uuid.UUID, at time.Time) (int, error) {
	body, err := json.Marshal(map[string]any{
		"event_id":   eventID,
		"event_type": p.EventType,
		"timestamp":  at.Unix(),
		"data":       p.Business,
	})
	if err != nil {
		return 0, fmt.Errorf("marshaling delivery body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.URL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Event-Id", eventID.String())
	req.Header.Set("X-Webhook-Timestamp", fmt.Sprintf("%d", at.Unix()))
	req.Header.Set("X-Webhook-Signature", Sign(ep.SigningSecret, at.Unix(), eventID.String(), body))

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("delivering webhook: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("receiver returned status %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// Sign computes the HMAC-SHA256 signature a receiver must recompute to
// verify authenticity — over timestamp + "." + eventID + "." + body, so a
// receiver can also enforce a reasonable timestamp window to reject
// replayed deliveries (brief §38's "replay protection"), not just verify
// the body wasn't tampered with.
func Sign(secret string, timestamp int64, eventID string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.%s.", timestamp, eventID)
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
