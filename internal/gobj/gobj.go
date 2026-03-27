// Package gobj provides GObject type registration and instance management for puregotk.
package gobj

import (
	"sync"
	"unsafe"

	"codeberg.org/puregotk/puregotk/v4/gobject"
)

// TypeDef defines a GObject subtype to register.
type TypeDef struct {
	ParentGLibType func() gobject.Type
	ClassName      string
	ClassInit      func(tc *gobject.TypeClass, reg *InstanceRegistry)
}

// InstanceRegistry maps GObject pointers to Go struct instances.
type InstanceRegistry struct {
	mu        sync.RWMutex
	instances map[uintptr]unsafe.Pointer
}

// Pin associates a Go struct pointer with a GObject.
func (r *InstanceRegistry) Pin(obj *gobject.Object, ptr unsafe.Pointer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.instances[obj.GoPointer()] = ptr
}

// Get retrieves the Go struct pointer for a GObject pointer.
func (r *InstanceRegistry) Get(goPtr uintptr) unsafe.Pointer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.instances[goPtr]
}

// RegisterType registers a new GObject subtype and returns its type ID and registry.
func RegisterType(def TypeDef) (gobject.Type, *InstanceRegistry) {
	reg := &InstanceRegistry{
		instances: make(map[uintptr]unsafe.Pointer),
	}

	classInit := gobject.ClassInitFunc(func(tc *gobject.TypeClass, classData uintptr) {
		def.ClassInit(tc, reg)
	})

	gType := gobject.TypeRegisterStaticSimple(
		def.ParentGLibType(),
		def.ClassName,
		uint32(unsafe.Sizeof(gobject.ObjectClass{})),
		&classInit,
		uint32(unsafe.Sizeof(gobject.Object{})),
		nil,
		0,
	)

	return gType, reg
}
