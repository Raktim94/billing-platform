package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"rechvix/internal/platform/database"
)

// Store is the poller-facing surface (Writer plus claim/mark) — *PGStore
// satisfies it. Kept separate from Writer so sales.Service (which only
// enqueues) depends on the narrower interface.
type Store interface {
	Writer
	ClaimNext(ctx context.Context) (*Event, bool, error)
	MarkDone(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, attempts int, handlerErr error) error
}

// Poller is apps/worker's core loop: claim one event across all
// organisations (via the RLS-bypassing outbox_claim_next() function),
// then process it inside a transaction scoped to THAT event's own
// organisation — so the handler's own reads/writes (e.g.
// einvoice.Service.GenerateForDocument reading the sales document, writing
// an einvoice_records row) go through ordinary tenant-scoped RLS like
// everything else, and the "mark DONE/FAILED" update commits atomically
// with whatever the handler did.
type Poller struct {
	pool     *database.Pool
	store    Store
	handlers map[string]Handler
	logger   *slog.Logger
}

func NewPoller(pool *database.Pool, store Store, logger *slog.Logger) *Poller {
	if logger == nil {
		logger = slog.Default()
	}
	return &Poller{pool: pool, store: store, handlers: map[string]Handler{}, logger: logger}
}

// Register binds eventType to handler. Call before Run/ProcessOnce starts;
// not safe to call concurrently with either.
func (p *Poller) Register(eventType string, handler Handler) {
	p.handlers[eventType] = handler
}

var errNoHandler = errors.New("outbox: no handler registered for event type")

// ProcessOnce claims and processes at most one event. Returns
// (false, nil) when there was nothing to claim — the caller (Run) uses
// this to decide whether to sleep before polling again. A handler error
// is captured (event marked FAILED, scheduled for retry) and NOT
// returned from ProcessOnce — a single bad event must never stop the
// poller loop from continuing to process everything else.
func (p *Poller) ProcessOnce(ctx context.Context) (bool, error) {
	claimed, ok, err := p.store.ClaimNext(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	handler, known := p.handlers[claimed.EventType]

	runErr := p.pool.RunScoped(ctx, claimed.OrganisationID, func(ctx context.Context) error {
		var handleErr error
		if !known {
			handleErr = fmt.Errorf("%w: %q", errNoHandler, claimed.EventType)
		} else {
			handleErr = handler(ctx, *claimed)
		}
		if handleErr != nil {
			p.logger.Warn("outbox: event processing failed", "event_id", claimed.ID, "event_type", claimed.EventType,
				"attempts", claimed.Attempts, "error", handleErr)
			return p.store.MarkFailed(ctx, claimed.ID, claimed.Attempts, handleErr)
		}
		return p.store.MarkDone(ctx, claimed.ID)
	})
	// A failure to even write the FAILED/DONE status update (vs. a
	// handler error, which is already captured above) is a genuine
	// poller-level error worth surfacing to Run's caller/logs.
	return true, runErr
}

// Run polls until ctx is cancelled. pollInterval is how long to sleep
// after an empty claim (nothing to do); a successful claim loops
// immediately to drain the queue without waiting.
func (p *Poller) Run(ctx context.Context, pollInterval time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		processed, err := p.ProcessOnce(ctx)
		if err != nil {
			p.logger.Error("outbox: poller error", "error", err)
		}
		if !processed {
			select {
			case <-ctx.Done():
				return
			case <-time.After(pollInterval):
			}
		}
	}
}
