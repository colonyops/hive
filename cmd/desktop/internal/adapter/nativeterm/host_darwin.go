//go:build darwin && ghostty && !server

package nativeterm

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -I${SRCDIR}/../../../build/ghostty/include
#cgo LDFLAGS: -L${SRCDIR}/../../../build/ghostty/lib -lghostty-internal -lc++
#cgo LDFLAGS: -framework Cocoa -framework WebKit -framework Carbon -framework Metal -framework MetalKit
#cgo LDFLAGS: -framework QuartzCore -framework CoreText -framework CoreGraphics -framework CoreVideo
#cgo LDFLAGS: -framework IOSurface -framework UniformTypeIdentifiers -framework CoreServices
#include <stdlib.h>
#include "host_darwin.h"
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

// Available reports whether this build draws native terminals.
func Available() bool { return true }

// Host owns the native surfaces of one window.
type Host struct {
	window  unsafe.Pointer
	onEvent func(Event)

	mu      sync.Mutex
	handles map[string]uintptr
}

var registry = struct {
	sync.Mutex
	next  uintptr
	byKey map[uintptr]surfaceRef
}{byKey: map[uintptr]surfaceRef{}}

type surfaceRef struct {
	host *Host
	id   string
}

// New loads libghostty and returns a host for the NSWindow at window. The
// user's own Ghostty config loads first, then the file at configPath (empty
// for none). onEvent runs on the main thread and must not block.
func New(window unsafe.Pointer, configPath string, onEvent func(Event)) (*Host, error) {
	if window == nil {
		return nil, errors.New("nativeterm: no native window")
	}
	var cpath *C.char
	if configPath != "" {
		cpath = C.CString(configPath)
		defer C.free(unsafe.Pointer(cpath))
	}
	if !C.hive_nt_init(cpath, true) {
		return nil, errors.New("nativeterm: libghostty failed to start")
	}
	return &Host{window: window, onEvent: onEvent, handles: map[string]uintptr{}}, nil
}

// Open starts a hidden surface for id; SetFrame shows it.
func (h *Host) Open(id string, spec Spec) error {
	h.mu.Lock()
	if _, ok := h.handles[id]; ok {
		h.mu.Unlock()
		return nil
	}
	registry.Lock()
	registry.next++
	handle := registry.next
	registry.byKey[handle] = surfaceRef{host: h, id: id}
	registry.Unlock()
	h.handles[id] = handle
	h.mu.Unlock()

	cmd := C.CString(spec.Command)
	defer C.free(unsafe.Pointer(cmd))
	var cwd *C.char
	if spec.Cwd != "" {
		cwd = C.CString(spec.Cwd)
		defer C.free(unsafe.Pointer(cwd))
	}
	keys := make([]*C.char, 0, len(spec.Env))
	values := make([]*C.char, 0, len(spec.Env))
	for k, v := range spec.Env {
		keys = append(keys, C.CString(k))
		values = append(values, C.CString(v))
	}
	defer func() {
		for i := range keys {
			C.free(unsafe.Pointer(keys[i]))
			C.free(unsafe.Pointer(values[i]))
		}
	}()
	var keyPtr, valuePtr **C.char
	if len(keys) > 0 {
		keyPtr, valuePtr = &keys[0], &values[0]
	}
	C.hive_nt_create(C.uintptr_t(handle), h.window, cmd, cwd, C.double(spec.FontSize), keyPtr, valuePtr, C.int(len(keys)))
	return nil
}

// ReloadConfig reloads the user's Ghostty config with the file at configPath
// on top, into every open surface.
func (h *Host) ReloadConfig(configPath string) {
	p := C.CString(configPath)
	defer C.free(unsafe.Pointer(p))
	C.hive_nt_reload_config(p)
}

// SetFrame moves, sizes, shows or hides id's surface.
func (h *Host) SetFrame(id string, r Rect) {
	if handle, ok := h.handle(id); ok {
		C.hive_nt_set_frame(C.uintptr_t(handle), C.double(r.X), C.double(r.Y), C.double(r.W), C.double(r.H), C.bool(r.Visible))
	}
}

// Focus gives id's surface the keyboard.
func (h *Host) Focus(id string) {
	if handle, ok := h.handle(id); ok {
		C.hive_nt_focus(C.uintptr_t(handle))
	}
}

// Close frees id's surface. Its command gets SIGHUP; a herdr-core program
// behind an attach bridge keeps running.
func (h *Host) Close(id string) {
	h.mu.Lock()
	handle, ok := h.handles[id]
	delete(h.handles, id)
	h.mu.Unlock()
	if !ok {
		return
	}
	registry.Lock()
	delete(registry.byKey, handle)
	registry.Unlock()
	C.hive_nt_destroy(C.uintptr_t(handle))
}

func (h *Host) handle(id string) (uintptr, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	handle, ok := h.handles[id]
	return handle, ok
}

//export hiveNativeTermEvent
func hiveNativeTermEvent(handle C.uintptr_t, kind C.int, data *C.char) {
	registry.Lock()
	ref, ok := registry.byKey[uintptr(handle)]
	registry.Unlock()
	if !ok || ref.host.onEvent == nil {
		return
	}
	ref.host.onEvent(Event{ID: ref.id, Kind: EventKind(kind), Data: C.GoString(data)})
}

// SetOccluders sets the page elements floating over the terminals: every
// surface cuts them out, so the page shows through, and passes their clicks
// to the page.
func SetOccluders(rects []Rect) {
	flat := make([]C.double, 0, 4*len(rects))
	for _, r := range rects {
		flat = append(flat, C.double(r.X), C.double(r.Y), C.double(r.W), C.double(r.H))
	}
	var ptr *C.double
	if len(flat) > 0 {
		ptr = &flat[0]
	}
	C.hive_nt_set_occluders(ptr, C.int(len(rects)))
}
