package plugin

import (
	"fmt"
	"sync"
)

// Registry holds the set of plugin factories baked into a vfx room
// binary. Plugins are registered at process start (typically via an
// init function in the plugin package) and looked up by name from
// configuration.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register associates a Factory with its name. It is safe to call from
// init functions in package-level main.go side-effect imports.
func (r *Registry) Register(f Factory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := f.Name()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("plugin: %q already registered", name)
	}
	r.factories[name] = f
	return nil
}

// Lookup returns the registered Factory for name, or an error if no
// such plugin is known.
func (r *Registry) Lookup(name string) (Factory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("plugin: %q not registered", name)
	}
	return f, nil
}

// Names returns every plugin name in registration order. Useful for
// diagnostics ("which plugins are available in this binary?").
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for n := range r.factories {
		out = append(out, n)
	}
	return out
}
