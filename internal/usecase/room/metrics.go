package room

import "time"

// Metrics is a port so the usecase stays free of the concrete Prometheus registry.
type Metrics interface {
	IncActiveMatches()
	DecActiveMatches()

	// ObserveTick records the duration of one plugin OnTick.
	ObserveTick(d time.Duration)
}

type noopMetrics struct{}

func (noopMetrics) IncActiveMatches()         {}
func (noopMetrics) DecActiveMatches()         {}
func (noopMetrics) ObserveTick(time.Duration) {}
