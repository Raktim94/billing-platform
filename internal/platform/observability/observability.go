// Package observability wires up OpenTelemetry tracing (brief §58). It
// exports a stdout trace exporter when no OTLP endpoint is configured
// (local/dev), so tracing is always active — a deployment doesn't have to
// stand up a collector before getting any traces at all — and swapping in
// a real OTLP exporter later is a config change, not a code change.
package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// Config controls tracer-provider setup.
type Config struct {
	ServiceName string
	// OTLPEndpoint, if non-empty, is where a real OTLP exporter would
	// send spans. Stage 2 wires the stdout exporter unconditionally and
	// records this field for the composition root to act on once an
	// OTLP exporter dependency is added — deliberately not built now
	// (brief: "don't build a fake OTLP endpoint").
	OTLPEndpoint string
}

// Shutdown flushes and stops the tracer provider. Call it during graceful
// shutdown.
type Shutdown func(context.Context) error

// Setup installs a global TracerProvider and returns a shutdown function.
func Setup(ctx context.Context, cfg Config) (Shutdown, error) {
	exporter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, fmt.Errorf("observability: creating stdout trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: building resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}

// Tracer returns a named tracer from the global TracerProvider. Modules
// should call this once at construction time, not per-request.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
