package plugin

import (
	"fmt"
	"sync"
)

// Registry holds the plugin factories baked into a room binary, registered at startup and looked up by name.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

// Register is safe to call from init functions in side-effect imports.
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

func (r *Registry) Lookup(name string) (Factory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[name]
	if !ok {
		return nil, fmt.Errorf("plugin: %q not registered", name)
	}
	return f, nil
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for n := range r.factories {
		out = append(out, n)
	}
	return out
}
