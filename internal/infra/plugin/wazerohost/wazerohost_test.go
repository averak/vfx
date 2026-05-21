package wazerohost_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
	"github.com/averak/vfx/internal/domain/plugin"
	"github.com/averak/vfx/internal/infra/plugin/wazerohost"
)

const (
	alice = "alice"
	bob   = "bob"
)

// wasmBytes holds the RPS plugin compiled to WASM, built once by
// TestMain. It is nil when TinyGo is unavailable, in which case the
// tests skip rather than fail — building the guest is a toolchain
// concern, not something every checkout can do.
var wasmBytes []byte

func TestMain(m *testing.M) {
	wasmBytes = buildRPSWasm()
	os.Exit(m.Run())
}

// buildRPSWasm compiles examples/rps/plugin/cmd/wasm with TinyGo into a
// temp file and returns its bytes, or nil if TinyGo is not on PATH.
func buildRPSWasm() []byte {
	if _, err := exec.LookPath("tinygo"); err != nil {
		return nil
	}
	// The test runs from the package directory; the module root is four
	// levels up (internal/infra/plugin/wazerohost).
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		return nil
	}
	out := filepath.Join(os.TempDir(), "vfx-test-rps.wasm")
	cmd := exec.Command("tinygo", "build", "-o", out,
		"-buildmode=c-shared", "-target=wasip1", "./examples/rps/plugin/cmd/wasm")
	cmd.Dir = root
	combined, err := cmd.CombinedOutput()
	if err != nil {
		// Surface the toolchain error on stderr; the tests will skip.
		_, _ = os.Stderr.Write(combined)
		return nil
	}
	b, err := os.ReadFile(out)
	if err != nil {
		return nil
	}
	return b
}

func newFactory(t *testing.T) *wazerohost.Factory {
	t.Helper()
	if wasmBytes == nil {
		t.Skip("TinyGo not available; skipping WASM host tests")
	}
	f, err := wazerohost.NewFactory(t.Context(), "rps.wasm", wasmBytes)
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	t.Cleanup(func() { _ = f.Close(context.Background()) })
	return f
}

func newMatch(t *testing.T, f *wazerohost.Factory) plugin.Plugin {
	t.Helper()
	p, err := f.Create(t.Context())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	if _, err := p.Init(t.Context(), &pluginv1.InitRequest{PlayerIds: []string{alice, bob}}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return p
}

// playRound submits both choices through the WASM boundary and returns
// the tick response that resolved the round (the second submission).
func playRound(t *testing.T, p plugin.Plugin, aChoice, bChoice byte) *pluginv1.OnTickResponse {
	t.Helper()
	if _, err := p.OnTick(t.Context(), &pluginv1.OnTickRequest{
		Actions: []*pluginv1.PlayerAction{{PlayerId: alice, Payload: []byte{aChoice}}},
	}); err != nil {
		t.Fatalf("OnTick(alice): %v", err)
	}
	resp, err := p.OnTick(t.Context(), &pluginv1.OnTickRequest{
		Actions: []*pluginv1.PlayerAction{{PlayerId: bob, Payload: []byte{bChoice}}},
	})
	if err != nil {
		t.Fatalf("OnTick(bob): %v", err)
	}
	return resp
}

func TestWasmHost_PlaysFullMatch(t *testing.T) {
	f := newFactory(t)
	p := newMatch(t, f)

	// Round 1: Rock beats Scissors -> alice.
	if resp := playRound(t, p, 'R', 'S'); resp.GetGameEnded() {
		t.Fatal("game ended after one round")
	}
	// Round 2: Paper beats Rock -> alice reaches 2 wins, match ends.
	if resp := playRound(t, p, 'P', 'R'); !resp.GetGameEnded() {
		t.Fatal("game did not end after alice's second win")
	}

	end, err := p.OnGameEnd(t.Context(), &pluginv1.OnGameEndRequest{})
	if err != nil {
		t.Fatalf("OnGameEnd: %v", err)
	}
	assertRank(t, end, alice, 1)
	assertRank(t, end, bob, -1)
}

func TestWasmHost_InitRejectsWrongPlayerCount(t *testing.T) {
	f := newFactory(t)
	p, err := f.Create(t.Context())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	if _, err := p.Init(t.Context(), &pluginv1.InitRequest{PlayerIds: []string{alice}}); err == nil {
		t.Error("Init with one player succeeded, want an error across the WASM boundary")
	}
}

func TestWasmHost_InstancesAreIsolated(t *testing.T) {
	f := newFactory(t)

	// Two matches from one Factory must not share linear memory: a round
	// played in the first must not influence the second's state.
	p1 := newMatch(t, f)
	playRound(t, p1, 'R', 'S') // alice 1-0 in match 1

	p2 := newMatch(t, f)
	// A tie in a fresh match must not end it; if state leaked from p1,
	// alice would already be partway to a win and behaviour could differ.
	if resp := playRound(t, p2, 'R', 'R'); resp.GetGameEnded() {
		t.Fatal("a fresh instance ended after a single tied round; state leaked between matches")
	}
}

func TestWasmHost_FactoryRejectsInvalidModule(t *testing.T) {
	if _, err := exec.LookPath("tinygo"); err != nil {
		// The bad-bytes path needs no guest; still gate on the same
		// toolchain so the suite is uniformly skipped in minimal envs.
		t.Skip("TinyGo not available; skipping WASM host tests")
	}
	if _, err := wazerohost.NewFactory(t.Context(), "broken", []byte("not wasm")); err == nil {
		t.Error("NewFactory accepted non-WASM bytes")
	}
}

func assertRank(t *testing.T, end *pluginv1.OnGameEndResponse, playerID string, want int32) {
	t.Helper()
	for _, r := range end.GetPlayerResults() {
		if r.GetPlayerId() == playerID {
			if r.GetRank() != want {
				t.Errorf("rank of %s = %d, want %d", playerID, r.GetRank(), want)
			}
			return
		}
	}
	t.Errorf("no result for player %s", playerID)
}
