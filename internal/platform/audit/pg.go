package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"billing-platform/internal/platform/database"
)

var timeNow = time.Now

type PGRecorder struct {
	pool *database.Pool
}

func NewPGRecorder(pool *database.Pool) *PGRecorder {
	return &PGRecorder{pool: pool}
}

func (r *PGRecorder) Record(ctx context.Context, entry Entry) error {
	before, err := marshalState(entry.BeforeState)
	if err != nil {
		return fmt.Errorf("audit: marshaling before_state: %w", err)
	}
	after, err := marshalState(entry.AfterState)
	if err != nil {
		return fmt.Errorf("audit: marshaling after_state: %w", err)
	}

	const q = `
		INSERT INTO audit_log
			(id, organisation_id, actor_user_id, actor_type, action, entity_type,
			 entity_id, before_state, after_state, ip, user_agent, request_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("audit: generating id: %w", err)
	}
	at := entry.At
	if at.IsZero() {
		at = timeNow()
	}

	_, err = r.pool.Q(ctx).Exec(ctx, q,
		id,
		entry.OrganisationID, entry.ActorUserID, string(entry.ActorType), entry.Action, entry.EntityType,
		entry.EntityID, before, after, nullIfEmpty(entry.IP), nullIfEmpty(entry.UserAgent), nullIfEmpty(entry.RequestID), at,
	)
	if err != nil {
		return fmt.Errorf("audit: inserting audit_log row: %w", err)
	}
	return nil
}

func marshalState(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
