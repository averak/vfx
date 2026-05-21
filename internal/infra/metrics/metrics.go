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

	// RPC-level metrics, recorded generically by the metrics
	// interceptor for every Connect call.
	RPCRequests *prometheus.CounterVec   // labels: method, code
	RPCDuration *prometheus.HistogramVec // labels: method

	// Matchmaking metrics, recorded by the matchmaker worker.
	MatchesAllocated prometheus.Counter
	QueueDepth       *prometheus.GaugeVec // labels: game_mode

	// Room metrics, recorded by the room daemon (exposed once the room
	// grows an HTTP probe sidecar).
	RoomMatchesActive prometheus.Gauge
	RoomTickDuration  prometheus.Histogram
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

	r.RPCRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "vfx_rpc_requests_total",
			Help: "Connect RPC calls, labelled by fully-qualified method and result code.",
		},
		[]string{"method", "code"},
	)
	r.RPCDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "vfx_rpc_request_duration_seconds",
			Help:    "Connect RPC handler latency, labelled by method.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)
	r.MatchesAllocated = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vfx_matchmaker_matches_allocated_total",
		Help: "Successful match allocations by the matchmaker worker.",
	})
	r.QueueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "vfx_matchmaker_queue_depth",
			Help: "Tickets currently waiting in the matchmaker queue, by game mode.",
		},
		[]string{"game_mode"},
	)

	r.RoomMatchesActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vfx_room_matches_active",
		Help: "Matches currently running inside this room daemon.",
	})
	r.RoomTickDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "vfx_room_tick_duration_seconds",
		Help:    "Wall-clock duration of plugin OnTick invocations.",
		Buckets: prometheus.ExponentialBucketsRange(0.00005, 0.5, 12),
	})

	reg.MustRegister(
		r.RPCRequests,
		r.RPCDuration,
		r.MatchesAllocated,
		r.QueueDepth,
		r.RoomMatchesActive,
		r.RoomTickDuration,
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
