// Package metrics holds the Prometheus collectors vfx exposes.
//
// Collectors are registered against a private Registry rather than the
// process default so tests can construct an isolated set, and so the
// /metrics endpoint never accidentally exposes Go runtime metrics from
// a transitive dependency we did not vet.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry is the vfx-owned Prometheus registry. All vfx collectors
// register here; nothing should touch prometheus.DefaultRegisterer.
type Registry struct {
	*prometheus.Registry

	// Gateway counters.
	LoginAttempts    *prometheus.CounterVec
	TicketsCreated   prometheus.Counter
	TicketsActive    prometheus.Gauge
	MatchesAllocated prometheus.Counter

	// Room counters.
	RoomMatchesActive prometheus.Gauge
	RoomTickDuration  prometheus.Histogram
	RoomFrameSent     *prometheus.CounterVec
}

// NewRegistry builds a fresh registry pre-loaded with the standard Go
// and process collectors plus the vfx-specific ones.
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	r := &Registry{Registry: reg}

	r.LoginAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vfx_gateway_login_attempts_total",
			Help: "Login attempts at the gateway, labelled by credential kind and outcome.",
		},
		[]string{"credential", "outcome"},
	)
	r.TicketsCreated = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vfx_gateway_tickets_created_total",
		Help: "Total matchmaking tickets accepted by the gateway.",
	})
	r.TicketsActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vfx_gateway_tickets_active",
		Help: "Tickets currently waiting in the matchmaker queue.",
	})
	r.MatchesAllocated = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vfx_matchmaker_matches_allocated_total",
		Help: "Successful match allocations by the matchmaker worker.",
	})

	r.RoomMatchesActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vfx_room_matches_active",
		Help: "Matches currently running inside this room daemon.",
	})
	r.RoomTickDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "vfx_room_tick_duration_seconds",
		Help:    "Wall-clock duration of plugin OnTick invocations.",
		Buckets: prometheus.ExponentialBucketsRange(0.00005, 0.5, 12),
	})
	r.RoomFrameSent = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vfx_room_frames_sent_total",
			Help: "Frames sent from the room to a player, labelled by kind.",
		},
		[]string{"kind"},
	)

	reg.MustRegister(
		r.LoginAttempts,
		r.TicketsCreated,
		r.TicketsActive,
		r.MatchesAllocated,
		r.RoomMatchesActive,
		r.RoomTickDuration,
		r.RoomFrameSent,
	)
	return r
}

// Handler returns an http.Handler that exports the registry in the
// Prometheus text format.
func (r *Registry) Handler() http.Handler {
	return promhttp.HandlerFor(r.Registry, promhttp.HandlerOpts{
		Registry: r.Registry,
	})
}
