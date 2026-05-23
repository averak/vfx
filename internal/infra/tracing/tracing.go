// Package tracing wires OpenTelemetry span export for the vfx processes.
//
// Tracing is opt-in: Setup installs a global tracer provider only when an OTLP endpoint is configured via the standard OTEL_* environment variables.
// With no endpoint the global provider stays the SDK's no-op, so a single-node or VPS deployment that never runs a collector pays nothing and emits no connection errors.
// Span instrumentation in the rest of the codebase always talks to otel.Tracer(...), so it is correct whether or not Setup turned anything on.
package tracing

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// ShutdownFunc flushes and tears down the tracer provider.
// Call it before the process exits so buffered spans are exported.
type ShutdownFunc func(context.Context) error

// Setup installs a global OTLP/HTTP tracer provider for serviceName and returns a shutdown function.
// When neither OTEL_EXPORTER_OTLP_ENDPOINT nor OTEL_EXPORTER_OTLP_TRACES_ENDPOINT is set, tracing stays off and the returned shutdown is a no-op.
// Endpoint, headers, and TLS follow the standard OTEL_EXPORTER_OTLP_* variables read by the SDK.
func Setup(ctx context.Context, serviceName string) (ShutdownFunc, error) {
	if !enabled() {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("tracing: otlp exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("tracing: resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}

func enabled() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
}
