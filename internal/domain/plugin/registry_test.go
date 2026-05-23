package plugin_test

import (
	"context"
	"sort"
	"testing"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
	"github.com/averak/vfx/internal/domain/plugin"
)

// stubFactory is a no-op Factory used to exercise the Registry; it never
// produces a usable Plugin because the Registry never calls Create.
type stubFactory struct{ name string }

func (f stubFactory) Name() string { return f.name }

func (stubFactory) Create(context.Context) (plugin.Plugin, error) { return stubPlugin{}, nil }

const rpsName = "rps"

type stubPlugin struct{}

func (stubPlugin) Init(context.Context, *pluginv1.InitRequest) (*pluginv1.InitResponse, error) {
	return &pluginv1.InitResponse{}, nil
}

func (stubPlugin) OnTick(context.Context, *pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error) {
	return &pluginv1.OnTickResponse{}, nil
}

func (stubPlugin) OnGameEnd(context.Context, *pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error) {
	return &pluginv1.OnGameEndResponse{}, nil
}

func (stubPlugin) Close() error { return nil }

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := plugin.NewRegistry()
	if err := r.Register(stubFactory{name: rpsName}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.Lookup(rpsName)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.Name() != rpsName {
		t.Errorf("Lookup returned %q, want rps", got.Name())
	}
}

func TestRegistry_DuplicateRegisterRejected(t *testing.T) {
	r := plugin.NewRegistry()
	if err := r.Register(stubFactory{name: rpsName}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register(stubFactory{name: rpsName}); err == nil {
		t.Error("registering the same name twice should fail")
	}
}

func TestRegistry_LookupMissing(t *testing.T) {
	r := plugin.NewRegistry()
	if _, err := r.Lookup("absent"); err == nil {
		t.Error("looking up an unregistered plugin should fail")
	}
}

func TestRegistry_Names(t *testing.T) {
	r := plugin.NewRegistry()
	if names := r.Names(); len(names) != 0 {
		t.Errorf("fresh registry has names: %v", names)
	}
	for _, n := range []string{rpsName, "pong"} {
		if err := r.Register(stubFactory{name: n}); err != nil {
			t.Fatalf("Register(%q): %v", n, err)
		}
	}
	names := r.Names()
	sort.Strings(names)
	if len(names) != 2 || names[0] != "pong" || names[1] != rpsName {
		t.Errorf("Names() = %v, want [pong rps]", names)
	}
}
