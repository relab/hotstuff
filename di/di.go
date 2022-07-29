package di

import (
	"fmt"
	"reflect"
)

// Initializer is a helper that assigns values to pointers.
type Initializer struct {
	ran      bool
	pointers []reflect.Value
	values   map[reflect.Type]reflect.Value
}

// New returns a new Initializer.
func New() *Initializer {
	return &Initializer{
		values: make(map[reflect.Type]reflect.Value),
	}
}

// Resolve assigns a registered value to each registered pointer.
func (init *Initializer) Resolve() {
	if init.ran {
		panic("initializer can only run once")
	}

	for _, field := range init.pointers {
		if field.Kind() != reflect.Pointer {
			panic(fmt.Sprintf("expected pointer type, but got %v", field.Type()))
		}

		t := field.Elem().Type()
		v, ok := init.values[t]
		if !ok {
			panic(fmt.Sprintf("missing value for type %v", t))
		}

		field.Elem().Set(reflect.ValueOf(v))
	}

	init.ran = true
}

// Needer is a type that needs to be initialized by the initializer.
type Needer interface {
	// Need registers pointers to any dependencies.
	Need(init *Initializer)
}

// Provide registers a value with the given initializer.
// The value may be used to initialize any pointers to a matching type.
func Provide[T any](init *Initializer, value T) {
	rv := reflect.ValueOf(value)
	init.values[rv.Type()] = rv
	if needer, needy := any(value).(Needer); needy {
		needer.Need(init)
	}
}

// Register allows the needer to register its pointers with the initializer.
func Register(init *Initializer, needer Needer) {
	needer.Need(init)
}

// Init registers a pointer with the given initializer.
// When Resolve() runs, it will give the pointer a value of a matching type.
func Init[T any](init *Initializer, ptr *T) {
	if ptr == nil {
		panic("pointer is nil")
	}
	init.pointers = append(init.pointers, reflect.ValueOf(ptr))
}
