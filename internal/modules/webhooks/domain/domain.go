// Package domain holds the webhooks module's entity types, repository
// interfaces, and sentinel errors (docs/architecture.md §2).
package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrNotFound = errors.New("webhooks: not found")

// EventCatalog mirrors the CHECK constraint in
// migrations/0026_webhooks.up.sql — kept in Go too so RegisterEndpoint
// can reject an unknown event type with a clear application-level error
// instead of a raw Postgres CHECK-violation message.
var EventCatalog = map[string]bool{
	"invoice.created": true, "invoice.finalized": true, "invoice.cancelled": true,
	"payment.created": true, "payment.received": true,
	"stock.changed": true, "stock.low": true,
	"customer.created":   true,
	"einvoice.generated": true,
	"einvoice.failed":    true,
	"ewaybill.generated": true,
	"ewaybill.failed":    true,
}

type Endpoint struct {
	ID               uuid.UUID
	OrganisationID   uuid.UUID
	URL              string
	SigningSecret    string
	SubscribedEvents []string
	IsActive         bool
	CreatedAt        time.Time
	CreatedBy        uuid.UUID
}

type DeliveryLog struct {
	ID                uuid.UUID
	OrganisationID    uuid.UUID
	WebhookEndpointID uuid.UUID
	EventType         string
	EventID           uuid.UUID
	HTTPStatus        *int
	Succeeded         bool
	ErrorMessage      *string
	AttemptedAt       time.Time
}

type EndpointRepository interface {
	Create(ctx context.Context, e *Endpoint) error
	GetByID(ctx context.Context, id uuid.UUID) (*Endpoint, error)
	// ListSubscribed returns active endpoints for organisationID subscribed
	// to eventType — the fan-out query the outbox source-event handler
	// runs to decide how many "webhook.delivery" events to enqueue.
	ListSubscribed(ctx context.Context, organisationID uuid.UUID, eventType string) ([]*Endpoint, error)
	ListForOrganisation(ctx context.Context, organisationID uuid.UUID) ([]*Endpoint, error)
	Deactivate(ctx context.Context, id uuid.UUID) error
}

type DeliveryLogRepository interface {
	Create(ctx context.Context, d *DeliveryLog) error
}
