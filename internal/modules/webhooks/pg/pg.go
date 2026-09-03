package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"billing-platform/internal/modules/webhooks/domain"
	"billing-platform/internal/platform/database"
)

type EndpointRepo struct{ pool *database.Pool }

func NewEndpointRepo(pool *database.Pool) *EndpointRepo { return &EndpointRepo{pool: pool} }

func (r *EndpointRepo) Create(ctx context.Context, e *domain.Endpoint) error {
	const q = `
		INSERT INTO webhook_endpoints (id, organisation_id, url, signing_secret, subscribed_events, is_active, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := r.pool.Q(ctx).Exec(ctx, q, e.ID, e.OrganisationID, e.URL, e.SigningSecret, e.SubscribedEvents,
		e.IsActive, e.CreatedAt, e.CreatedBy)
	if err != nil {
		return fmt.Errorf("webhooks: inserting endpoint: %w", err)
	}
	return nil
}

func (r *EndpointRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Endpoint, error) {
	const q = `
		SELECT id, organisation_id, url, signing_secret, subscribed_events, is_active, created_at, created_by
		FROM webhook_endpoints WHERE id = $1`
	row := r.pool.Q(ctx).QueryRow(ctx, q, id)
	e, err := scanEndpoint(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return e, nil
}

func (r *EndpointRepo) ListSubscribed(ctx context.Context, organisationID uuid.UUID, eventType string) ([]*domain.Endpoint, error) {
	const q = `
		SELECT id, organisation_id, url, signing_secret, subscribed_events, is_active, created_at, created_by
		FROM webhook_endpoints
		WHERE organisation_id = $1 AND is_active = true AND $2 = ANY(subscribed_events)`
	return r.listQuery(ctx, q, organisationID, eventType)
}

func (r *EndpointRepo) ListForOrganisation(ctx context.Context, organisationID uuid.UUID) ([]*domain.Endpoint, error) {
	const q = `
		SELECT id, organisation_id, url, signing_secret, subscribed_events, is_active, created_at, created_by
		FROM webhook_endpoints WHERE organisation_id = $1 ORDER BY created_at DESC`
	return r.listQuery(ctx, q, organisationID)
}

func (r *EndpointRepo) listQuery(ctx context.Context, q string, args ...any) ([]*domain.Endpoint, error) {
	rows, err := r.pool.Q(ctx).Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("webhooks: listing endpoints: %w", err)
	}
	defer rows.Close()
	var out []*domain.Endpoint
	for rows.Next() {
		e, err := scanEndpoint(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *EndpointRepo) Deactivate(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Q(ctx).Exec(ctx, `UPDATE webhook_endpoints SET is_active = false WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("webhooks: deactivating endpoint: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEndpoint(row rowScanner) (*domain.Endpoint, error) {
	var e domain.Endpoint
	if err := row.Scan(&e.ID, &e.OrganisationID, &e.URL, &e.SigningSecret, &e.SubscribedEvents,
		&e.IsActive, &e.CreatedAt, &e.CreatedBy); err != nil {
		return nil, fmt.Errorf("webhooks: scanning endpoint row: %w", err)
	}
	return &e, nil
}

type DeliveryLogRepo struct{ pool *database.Pool }

func NewDeliveryLogRepo(pool *database.Pool) *DeliveryLogRepo { return &DeliveryLogRepo{pool: pool} }

func (r *DeliveryLogRepo) Create(ctx context.Context, d *domain.DeliveryLog) error {
	const q = `
		INSERT INTO webhook_deliveries (id, organisation_id, webhook_endpoint_id, event_type, event_id,
			http_status, succeeded, error_message, attempted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := r.pool.Q(ctx).Exec(ctx, q, d.ID, d.OrganisationID, d.WebhookEndpointID, d.EventType, d.EventID,
		d.HTTPStatus, d.Succeeded, d.ErrorMessage, d.AttemptedAt)
	if err != nil {
		return fmt.Errorf("webhooks: inserting delivery log: %w", err)
	}
	return nil
}
