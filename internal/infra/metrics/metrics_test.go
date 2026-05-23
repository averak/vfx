package metrics_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/averak/vfx/internal/infra/metrics"
)

// NewRegistry uses MustRegister, which panics on a duplicate or invalid collector.
// Building two independent registries proves the collectors are per-instance (not global), so isolated test setups never collide.
func TestNewRegistry_BuildsIsolatedInstances(t *testing.T) {
	_ = metrics.NewRegistry()
	reg := metrics.NewRegistry()
	if reg.RPCRequests == nil || reg.RPCDuration == nil || reg.MatchesAllocated == nil ||
		reg.QueueDepth == nil || reg.RoomMatchesActive == nil || reg.RoomTickDuration == nil {
		t.Fatal("a collector was left nil")
	}
}

func TestRegistry_HandlerExposesVfxMetric(t *testing.T) {
	reg := metrics.NewRegistry()
	reg.MatchesAllocated.Inc()

	srv := httptest.NewServer(reg.Handler())
	t.Cleanup(srv.Close)

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "vfx_matchmaker_matches_allocated_total 1") {
		t.Errorf("incremented counter not exposed; body:\n%s", body)
	}
}
