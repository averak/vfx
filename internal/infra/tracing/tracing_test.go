package tracing_test

import (
	"testing"

	"github.com/averak/vfx/internal/infra/tracing"
)

// With no OTEL endpoint configured, Setup stays off: it returns a no-op shutdown and no error, never failing for lack of a collector.
func TestSetup_NoOpWhenDisabled(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")

	shutdown, err := tracing.Setup(t.Context(), "vfx-test")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Setup returned a nil shutdown")
	}
	if err := shutdown(t.Context()); err != nil {
		t.Errorf("no-op shutdown returned %v", err)
	}
}
