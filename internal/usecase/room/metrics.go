package room

import "time"

// Metrics is the telemetry the room orchestrator emits.
// It is an interface so the usecase stays free of the concrete Prometheus registry: bootstrap supplies an adapter, tests use the no-op default.
type Metrics interface {
	// IncActiveMatches and DecActiveMatches track the number of matches currently running in this daemon.
	IncActiveMatches()
	DecActiveMatches()

	// ObserveTick records the wall-clock duration of one plugin OnTick.
	ObserveTick(d time.Duration)
}

type noopMetrics struct{}

func (noopMetrics) IncActiveMatches()         {}
func (noopMetrics) DecActiveMatches()         {}
func (noopMetrics) ObserveTick(time.Duration) {}
