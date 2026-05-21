//go:build wasip1

// Package vfxplugin is the guest-side SDK for writing vfx game plugins
// in Go and compiling them to WebAssembly with TinyGo.
//
// A plugin author implements the Game interface (the same Init / OnTick
// / OnGameEnd shape the host uses, minus context and Close, since a
// WASM call is synchronous and an instance maps to a single match) and
// registers a factory:
//
//	//go:build wasip1
//	package main
//
//	func init() {
//	    vfxplugin.Register(func() vfxplugin.Game { return mygame.New() })
//	}
//
//	func main() {}
//
// Build it with:
//
//	tinygo build -o plugin.wasm -buildmode=c-shared -target=wasip1 ./...
//
// # ABI
//
// The host and guest exchange protobuf-encoded plugin.v1 messages over
// the module's linear memory. The guest exports:
//
//	vfx_alloc(size u32) u32          // reserve a request buffer, return ptr
//	vfx_init(ptr, len u32) u64       // -> packed(ptr<<32 | len) of InitResponse
//	vfx_on_tick(ptr, len u32) u64    // -> packed OnTickResponse
//	vfx_on_game_end(ptr, len u32) u64// -> packed OnGameEndResponse
//
// Calls are strictly sequential (one match, one tick at a time), so a
// single request buffer and a single response buffer — both kept alive
// as package globals so TinyGo's GC cannot reclaim them between the
// export returning and the host reading — are sufficient.
package vfxplugin

import (
	"unsafe"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
)

// Game is the contract a plugin implements. It is the transport-free
// core: no context, no Close. One Game instance serves one match.
type Game interface {
	Init(*pluginv1.InitRequest) (*pluginv1.InitResponse, error)
	OnTick(*pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error)
	OnGameEnd(*pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error)
}

// vtMessage is the reflection-free codec contract provided by
// vtprotobuf. The guest uses it instead of proto.Marshal/Unmarshal
// because TinyGo cannot run protobuf-go's reflection-based codec
// (reflect.NewAt panics at runtime).
type vtMessage interface {
	MarshalVT() ([]byte, error)
}

var (
	factory func() Game
	game    Game

	// reqBuf is the buffer the host writes a request into (via vfx_alloc).
	// respBuf holds the most recent marshalled response. Both are package
	// globals so they survive across the export boundary.
	reqBuf  []byte
	respBuf []byte
)

// Register sets the factory used to construct a fresh Game when the
// host initialises a match. Call it from the plugin's init().
func Register(f func() Game) { factory = f }

//go:wasmexport vfx_alloc
func vfxAlloc(size uint32) uint32 {
	reqBuf = make([]byte, size)
	return bytesPtr(reqBuf)
}

//go:wasmexport vfx_init
func vfxInit(_ uint32, length uint32) uint64 {
	game = factory()
	req := &pluginv1.InitRequest{}
	if err := req.UnmarshalVT(reqBuf[:length]); err != nil {
		return 0
	}
	resp, err := game.Init(req)
	if err != nil {
		return 0
	}
	return marshalResp(resp)
}

//go:wasmexport vfx_on_tick
func vfxOnTick(_ uint32, length uint32) uint64 {
	req := &pluginv1.OnTickRequest{}
	if err := req.UnmarshalVT(reqBuf[:length]); err != nil {
		return 0
	}
	resp, err := game.OnTick(req)
	if err != nil {
		return 0
	}
	return marshalResp(resp)
}

//go:wasmexport vfx_on_game_end
func vfxOnGameEnd(_ uint32, length uint32) uint64 {
	req := &pluginv1.OnGameEndRequest{}
	if err := req.UnmarshalVT(reqBuf[:length]); err != nil {
		return 0
	}
	resp, err := game.OnGameEnd(req)
	if err != nil {
		return 0
	}
	return marshalResp(resp)
}

func marshalResp(m vtMessage) uint64 {
	out, err := m.MarshalVT()
	if err != nil {
		return 0
	}
	respBuf = out
	return pack(bytesPtr(respBuf), uint32(len(respBuf)))
}

func bytesPtr(b []byte) uint32 {
	if len(b) == 0 {
		return 0
	}
	return uint32(uintptr(unsafe.Pointer(unsafe.SliceData(b))))
}

func pack(ptr, length uint32) uint64 {
	return uint64(ptr)<<32 | uint64(length)
}
