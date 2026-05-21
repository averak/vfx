// Package wazerohost loads WebAssembly game plugins with wazero and
// adapts them to the host plugin.Plugin / plugin.Factory contract.
//
// One CompiledModule is shared across the daemon; each match gets a
// fresh module instance so its linear memory (and therefore its game
// state) is isolated. Calls cross the boundary as protobuf-encoded
// plugin.v1 messages following the ABI documented in the guest SDK
// (sdk/plugin/go): vfx_alloc reserves a request buffer, and each
// lifecycle export returns a packed (ptr<<32 | len) pointing at the
// response.
package wazerohost

import (
	"context"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
	"github.com/averak/vfx/internal/domain/plugin"
)

// statusOK is the result-frame status byte for a successful call; any
// other value marks an error frame whose payload is a UTF-8 message.
// Kept in sync with the guest SDK (sdk/plugin/go), which returns
// packed(ptr<<32 | len) pointing at the frame.
const statusOK = 0

// Factory compiles a WASM module once and instantiates a fresh copy per
// match.
type Factory struct {
	name     string
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

var _ plugin.Factory = (*Factory)(nil)

// NewFactory compiles the given WASM bytes. name is the identifier the
// room matches against VFX_ROOM_PLUGIN_PATH (for a file-loaded plugin
// the caller typically passes the path). The returned Factory owns a
// wazero runtime; call Close to release it at shutdown.
func NewFactory(ctx context.Context, name string, wasm []byte) (*Factory, error) {
	rt := wazero.NewRuntime(ctx)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx) //nolint:errcheck // cleanup on failure path.
		return nil, fmt.Errorf("wazerohost: instantiate wasi: %w", err)
	}
	compiled, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		_ = rt.Close(ctx) //nolint:errcheck // cleanup on failure path.
		return nil, fmt.Errorf("wazerohost: compile module: %w", err)
	}
	return &Factory{name: name, runtime: rt, compiled: compiled}, nil
}

// Name returns the plugin identifier.
func (f *Factory) Name() string { return f.name }

// Close releases the wazero runtime and every module instance it owns.
func (f *Factory) Close(ctx context.Context) error { return f.runtime.Close(ctx) }

// Create instantiates a fresh module for one match. The instance runs
// its _initialize (package init) so the guest registers its game
// factory before any export is called.
func (f *Factory) Create(ctx context.Context) (plugin.Plugin, error) {
	// TinyGo's -buildmode=c-shared produces a WASI reactor whose runtime
	// is brought up by _initialize, not _start. wazero defaults to
	// calling _start, so we point it at _initialize explicitly; without
	// this the guest's wasmexport guard panics ("runtime not started").
	mod, err := f.runtime.InstantiateModule(ctx, f.compiled,
		wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
	if err != nil {
		return nil, fmt.Errorf("wazerohost: instantiate module: %w", err)
	}

	p := &wasmPlugin{
		mod:       mod,
		alloc:     mod.ExportedFunction("vfx_alloc"),
		init:      mod.ExportedFunction("vfx_init"),
		onTick:    mod.ExportedFunction("vfx_on_tick"),
		onGameEnd: mod.ExportedFunction("vfx_on_game_end"),
		memory:    mod.Memory(),
	}
	if p.alloc == nil || p.init == nil || p.onTick == nil || p.onGameEnd == nil || p.memory == nil {
		_ = mod.Close(ctx) //nolint:errcheck // cleanup on failure path.
		return nil, errors.New("wazerohost: module is missing required vfx exports")
	}
	return p, nil
}

// wasmPlugin is one instantiated WASM module = one match.
type wasmPlugin struct {
	mod       api.Module
	alloc     api.Function
	init      api.Function
	onTick    api.Function
	onGameEnd api.Function
	memory    api.Memory
}

var _ plugin.Plugin = (*wasmPlugin)(nil)

func (p *wasmPlugin) Init(ctx context.Context, req *pluginv1.InitRequest) (*pluginv1.InitResponse, error) {
	resp := &pluginv1.InitResponse{}
	if err := p.call(ctx, p.init, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *wasmPlugin) OnTick(ctx context.Context, req *pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error) {
	resp := &pluginv1.OnTickResponse{}
	if err := p.call(ctx, p.onTick, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (p *wasmPlugin) OnGameEnd(ctx context.Context, req *pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error) {
	resp := &pluginv1.OnGameEndResponse{}
	if err := p.call(ctx, p.onGameEnd, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// Close tears down the module instance, freeing its linear memory.
func (p *wasmPlugin) Close() error {
	return p.mod.Close(context.Background())
}

// call marshals req, hands it to the guest through a freshly allocated
// buffer, invokes fn, and unmarshals the packed response back into out.
func (p *wasmPlugin) call(ctx context.Context, fn api.Function, req, out proto.Message) error {
	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("wazerohost: marshal request: %w", err)
	}

	allocRes, err := p.alloc.Call(ctx, uint64(len(data)))
	if err != nil {
		return fmt.Errorf("wazerohost: alloc: %w", err)
	}
	// WASM linear memory is 32-bit addressed; pointers and lengths are
	// inherently uint32, so these truncations are exact by construction.
	ptr := uint32(allocRes[0]) //nolint:gosec // 32-bit wasm address.

	if len(data) > 0 && !p.memory.Write(ptr, data) {
		return errors.New("wazerohost: failed writing request into guest memory")
	}

	res, err := fn.Call(ctx, uint64(ptr), uint64(len(data)))
	if err != nil {
		return fmt.Errorf("wazerohost: call export: %w", err)
	}
	packed := res[0]
	respPtr := uint32(packed >> 32)
	respLen := uint32(packed) //nolint:gosec // 32-bit wasm length.
	if respLen == 0 {
		// The guest always returns at least a status byte; a zero-length
		// frame means it trapped or never wrote one.
		return errors.New("wazerohost: guest returned an empty frame")
	}

	frame, ok := p.memory.Read(respPtr, respLen)
	if !ok {
		return errors.New("wazerohost: failed reading response from guest memory")
	}
	// frame = [status byte][payload]. statusErr carries a UTF-8 message
	// instead of a marshalled response.
	status, payload := frame[0], frame[1:]
	if status != statusOK {
		return fmt.Errorf("wazerohost: plugin returned an error: %s", payload)
	}
	if err := proto.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("wazerohost: unmarshal response: %w", err)
	}
	return nil
}
