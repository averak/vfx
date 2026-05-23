//go:build wasip1

// Package vfxplugin is the guest-side SDK for writing vfx game plugins in Go and compiling them to WebAssembly with TinyGo.
//
// A plugin author implements the Game interface (the same Init / OnTick / OnGameEnd shape the host uses, minus context and Close, since a WASM call is synchronous and an instance maps to a single match) and registers a factory:
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
// The host and guest exchange protobuf-encoded plugin.v1 messages over the module's linear memory.
// The guest exports:
//
//	vfx_alloc(size u32) u32          // reserve a request buffer, return ptr
//	vfx_init(ptr, len u32) u64       // -> packed result frame
//	vfx_on_tick(ptr, len u32) u64    // -> packed result frame
//	vfx_on_game_end(ptr, len u32) u64// -> packed result frame
//
// Each lifecycle export returns packed(ptr<<32 | len) pointing at a result frame: a one-byte status (statusOK / statusErr) followed by a payload.
// On statusOK the payload is the marshalled response message; on statusErr it is a UTF-8 error string.
// The frame is never empty (it always carries at least the status byte), so a zero return is reserved for a host-detected fault.
//
// Calls are strictly sequential (one match, one tick at a time), so a single request buffer and a single response buffer suffice; both are kept alive as package globals so TinyGo's GC cannot reclaim them between the export returning and the host reading.
package vfxplugin

import (
	"unsafe"

	pluginv1 "github.com/averak/vfx/gen/go/plugin/v1"
)

// Game is the contract a plugin implements.
// It is the transport-free core: no context, no Close.
// One Game instance serves one match.
type Game interface {
	Init(*pluginv1.InitRequest) (*pluginv1.InitResponse, error)
	OnTick(*pluginv1.OnTickRequest) (*pluginv1.OnTickResponse, error)
	OnGameEnd(*pluginv1.OnGameEndRequest) (*pluginv1.OnGameEndResponse, error)
}

// vtMessage is the reflection-free codec contract provided by vtprotobuf.
// The guest uses it instead of proto.Marshal/Unmarshal because TinyGo cannot run protobuf-go's reflection-based codec (reflect.NewAt panics at runtime).
type vtMessage interface {
	MarshalVT() ([]byte, error)
}

var (
	factory func() Game
	game    Game

	// reqBuf is the buffer the host writes a request into (via vfx_alloc); respBuf holds the most recent marshalled response.
	// Both are package globals so they survive across the export boundary.
	reqBuf  []byte
	respBuf []byte
)

// Register sets the factory used to construct a fresh Game when the host initialises a match.
// Call it from the plugin's init().
func Register(f func() Game) { factory = f }

//go:wasmexport vfx_alloc
func vfxAlloc(size uint32) uint32 {
	reqBuf = make([]byte, size)
	return bytesPtr(reqBuf)
}

// Result frame status bytes.
// Kept in sync with the host (internal/infra/plugin/wazerohost).
const (
	statusOK  = 0
	statusErr = 1
)

//go:wasmexport vfx_init
func vfxInit(_ uint32, length uint32) uint64 {
	game = factory()
	req := &pluginv1.InitRequest{}
	if err := req.UnmarshalVT(reqBuf[:length]); err != nil {
		return fail("unmarshal InitRequest: " + err.Error())
	}
	resp, err := game.Init(req)
	if err != nil {
		return fail(err.Error())
	}
	return ok(resp)
}

//go:wasmexport vfx_on_tick
func vfxOnTick(_ uint32, length uint32) uint64 {
	req := &pluginv1.OnTickRequest{}
	if err := req.UnmarshalVT(reqBuf[:length]); err != nil {
		return fail("unmarshal OnTickRequest: " + err.Error())
	}
	resp, err := game.OnTick(req)
	if err != nil {
		return fail(err.Error())
	}
	return ok(resp)
}

//go:wasmexport vfx_on_game_end
func vfxOnGameEnd(_ uint32, length uint32) uint64 {
	req := &pluginv1.OnGameEndRequest{}
	if err := req.UnmarshalVT(reqBuf[:length]); err != nil {
		return fail("unmarshal OnGameEndRequest: " + err.Error())
	}
	resp, err := game.OnGameEnd(req)
	if err != nil {
		return fail(err.Error())
	}
	return ok(resp)
}

// ok marshals m into an OK result frame; a marshal failure degrades to an error frame so the host never silently sees a half-written buffer.
func ok(m vtMessage) uint64 {
	out, err := m.MarshalVT()
	if err != nil {
		return fail("marshal response: " + err.Error())
	}
	return frame(statusOK, out)
}

func fail(msg string) uint64 {
	return frame(statusErr, []byte(msg))
}

// frame writes a status byte followed by payload into respBuf and packs its pointer and length.
// respBuf is held as a package global so the guest's GC cannot reclaim it before the host reads it back.
func frame(status byte, payload []byte) uint64 {
	respBuf = make([]byte, len(payload)+1)
	respBuf[0] = status
	copy(respBuf[1:], payload)
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
