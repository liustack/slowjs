// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
)

// The shapes of results with no Value in them, and how javascript sees each,
// from each of the ways a Go function can return it. A way beside a Value
// takes the snapshot of the results (result.go), the others need not.

type (
	shapeInts     []int
	shapeIntMap   map[string]int
	shapeAnyMap   map[string]any
	shapeErrStr   string
	shapeErrMap   map[string]Value
	shapeErrSlice []Value
	shapeErrInts  []int
	shapeStruct   struct {
		A int
		B []int
		C map[string]int
		D *int
		E any
	}
)

func (e shapeErrStr) Error() string   { return "errStr " + string(e) }
func (e shapeErrMap) Error() string   { return fmt.Sprintf("errMap %d", len(e)) }
func (e shapeErrSlice) Error() string { return fmt.Sprintf("errSlice %d", len(e)) }
func (e shapeErrInts) Error() string  { return fmt.Sprintf("errInts %v", []int(e)) }

type shapeErrPtr struct{ n int }

func (e *shapeErrPtr) Error() string {
	if e == nil {
		return "errPtr nil"
	}
	return fmt.Sprintf("errPtr %d", e.n)
}

// anyOf is v in an any, typed any, so that a nil any is one too.
func anyOf(v any) reflect.Value { return reflect.ValueOf(&v).Elem() }

func resultShapes() map[string]func() reflect.Value {
	one := 1
	var nilInts []int
	return map[string]func() reflect.Value{
		"nil map of ints":          func() reflect.Value { return reflect.ValueOf(map[string]int(nil)) },
		"empty map of ints":        func() reflect.Value { return reflect.ValueOf(map[string]int{}) },
		"map of ints":              func() reflect.Value { return reflect.ValueOf(map[string]int{"b": 2, "a": 1}) },
		"nil map of any":           func() reflect.Value { return reflect.ValueOf(map[string]any(nil)) },
		"empty map of any":         func() reflect.Value { return reflect.ValueOf(map[string]any{}) },
		"nil map of Values":        func() reflect.Value { return reflect.ValueOf(map[string]Value(nil)) },
		"empty map of Values":      func() reflect.Value { return reflect.ValueOf(map[string]Value{}) },
		"nil named map":            func() reflect.Value { return reflect.ValueOf(shapeIntMap(nil)) },
		"nil named map of any":     func() reflect.Value { return reflect.ValueOf(shapeAnyMap(nil)) },
		"empty float key map":      func() reflect.Value { return reflect.ValueOf(map[float64]int{}) },
		"float key map":            func() reflect.Value { return reflect.ValueOf(map[float64]int{1.5: 1}) },
		"empty bool key map":       func() reflect.Value { return reflect.ValueOf(map[bool]Value{}) },
		"nil bool key map":         func() reflect.Value { return reflect.ValueOf(map[bool]Value(nil)) },
		"empty struct key map":     func() reflect.Value { return reflect.ValueOf(map[struct{}]any{}) },
		"nil slice of ints":        func() reflect.Value { return reflect.ValueOf([]int(nil)) },
		"empty slice of ints":      func() reflect.Value { return reflect.ValueOf([]int{}) },
		"nil slice of any":         func() reflect.Value { return reflect.ValueOf([]any(nil)) },
		"empty slice of any":       func() reflect.Value { return reflect.ValueOf([]any{}) },
		"nil slice of Values":      func() reflect.Value { return reflect.ValueOf([]Value(nil)) },
		"empty slice of Values":    func() reflect.Value { return reflect.ValueOf([]Value{}) },
		"nil named slice":          func() reflect.Value { return reflect.ValueOf(shapeInts(nil)) },
		"nil pointer to int":       func() reflect.Value { return reflect.ValueOf((*int)(nil)) },
		"pointer to int":           func() reflect.Value { return reflect.ValueOf(&one) },
		"nil pointer to struct":    func() reflect.Value { return reflect.ValueOf((*shapeStruct)(nil)) },
		"nil pointer to Value":     func() reflect.Value { return reflect.ValueOf((*Value)(nil)) },
		"pointer to nil slice":     func() reflect.Value { return reflect.ValueOf(&nilInts) },
		"pointer to pointer":       func() reflect.Value { p := &one; return reflect.ValueOf(&p) },
		"nil any":                  func() reflect.Value { return anyOf(nil) },
		"array of ints":            func() reflect.Value { return reflect.ValueOf([2]int{1, 2}) },
		"error string":             func() reflect.Value { return reflect.ValueOf(shapeErrStr("x")) },
		"nil error map":            func() reflect.Value { return reflect.ValueOf(shapeErrMap(nil)) },
		"empty error map":          func() reflect.Value { return reflect.ValueOf(shapeErrMap{}) },
		"nil error slice":          func() reflect.Value { return reflect.ValueOf(shapeErrSlice(nil)) },
		"nil error ints":           func() reflect.Value { return reflect.ValueOf(shapeErrInts(nil)) },
		"error ints":               func() reflect.Value { return reflect.ValueOf(shapeErrInts{1, 2}) },
		"nil error pointer":        func() reflect.Value { return reflect.ValueOf((*shapeErrPtr)(nil)) },
		"error pointer":            func() reflect.Value { return reflect.ValueOf(&shapeErrPtr{3}) },
		"nil error pointer in any": func() reflect.Value { return anyOf((*shapeErrPtr)(nil)) },
		"nil error map in any":     func() reflect.Value { return anyOf(shapeErrMap(nil)) },
		"nil error":                func() reflect.Value { var e error; return reflect.ValueOf(&e).Elem() },
		"struct":                   func() reflect.Value { return reflect.ValueOf(shapeStruct{}) },
		"struct with values": func() reflect.Value {
			return reflect.ValueOf(shapeStruct{1, []int{2}, map[string]int{"c": 3}, &one, "e"})
		},
		"nil map in any":     func() reflect.Value { return anyOf(map[string]int(nil)) },
		"nil slice in any":   func() reflect.Value { return anyOf([]int(nil)) },
		"nil pointer in any": func() reflect.Value { return anyOf((*int)(nil)) },
		"nil maps nested": func() reflect.Value {
			return reflect.ValueOf([]any{map[string]int(nil), map[string]any(nil), []map[string]int{nil}, map[string]any{"m": map[string]any(nil)}})
		},
		"nil slices nested": func() reflect.Value {
			return reflect.ValueOf(map[string]any{"a": []int(nil), "b": []any(nil), "c": map[string][]int{"d": nil}})
		},
		"nil pointers nested": func() reflect.Value {
			return reflect.ValueOf([]any{(*int)(nil), (*shapeStruct)(nil), []*int{nil}})
		},
		"errors nested": func() reflect.Value {
			return reflect.ValueOf([]any{shapeErrMap(nil), (*shapeErrPtr)(nil), shapeErrInts{4}, map[string]any{"e": shapeErrSlice(nil)}})
		},
		"empty maps nested": func() reflect.Value {
			return reflect.ValueOf([]any{map[float64]int{}, map[bool]Value{}, map[string]map[float64]int{"f": {}}})
		},
		"bad key nested": func() reflect.Value {
			return reflect.ValueOf([]any{1, map[float64]int{2: 2}})
		},
	}
}

// resultShapeOutputs returns how javascript sees each shape, by the name of
// the shape and of the way it was returned: alone, in an any, as the first
// of two results beside a Value, in a []any beside a Value, and in a
// map[string]any beside a Value.
func resultShapeOutputs(t *testing.T) map[string]string {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	valueType := reflect.TypeFor[Value]()
	anyType := reflect.TypeFor[any]()
	obj := func() reflect.Value {
		v, err := m.NewObjectValue()
		if err != nil {
			panic(err)
		}
		return reflect.ValueOf(v)
	}
	shapes := resultShapes()
	names := make([]string, 0, len(shapes))
	for name := range shapes {
		names = append(names, name)
	}
	sort.Strings(names)
	ways := []string{"alone", "any", "results", "slice", "map"}
	for i, name := range names {
		mk := shapes[name]
		typ := mk().Type()
		fns := []reflect.Value{
			reflect.MakeFunc(reflect.FuncOf(nil, []reflect.Type{typ}, false), func([]reflect.Value) []reflect.Value {
				return []reflect.Value{mk()}
			}),
			reflect.MakeFunc(reflect.FuncOf(nil, []reflect.Type{anyType}, false), func([]reflect.Value) []reflect.Value {
				return []reflect.Value{anyOf(mk().Interface())}
			}),
			reflect.MakeFunc(reflect.FuncOf(nil, []reflect.Type{typ, valueType}, false), func([]reflect.Value) []reflect.Value {
				return []reflect.Value{mk(), obj()}
			}),
			reflect.MakeFunc(reflect.FuncOf(nil, []reflect.Type{anyType}, false), func([]reflect.Value) []reflect.Value {
				return []reflect.Value{anyOf([]any{mk().Interface(), obj().Interface()})}
			}),
			reflect.MakeFunc(reflect.FuncOf(nil, []reflect.Type{anyType}, false), func([]reflect.Value) []reflect.Value {
				return []reflect.Value{anyOf(map[string]any{"x": mk().Interface(), "y": obj().Interface()})}
			}),
		}
		for j, fn := range fns {
			fname := fmt.Sprintf("shape%d_%d", i, j)
			if err := m.RegisterFunc(fname, fn.Interface(), false); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := m.Eval(`
		function showShape(f, way) {
			let r;
			try {
				r = f();
			} catch (e) {
				return "throw " + e.name + ": " + String(e.message).replace(/ \(.*$/s, "").replace(/^callback shape\d+_\d+:/, "callback:");
			}
			if (way === 2 || way === 3) {
				r = r[0];
			} else if (way === 4) {
				r = r.x;
			}
			if (r === undefined) {
				return "undefined";
			}
			return typeof r + " " + JSON.stringify(r);
		}
	`, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for i, name := range names {
		for j, way := range ways {
			r, err := m.Eval(fmt.Sprintf("showShape(shape%d_%d, %d)", i, j, j), EvalGlobal)
			if err != nil {
				t.Fatalf("%s %s: %v", name, way, err)
			}
			out[name+"/"+way] = r.(string)
		}
	}
	return out
}

// TestResultShapesMatchMain requires each shape to convert, in every way, as
// it did before the results had a snapshot: mainResultShapes is what
// resultShapeOutputs gave on main at 359e4268.
func TestResultShapesMatchMain(t *testing.T) {
	got := resultShapeOutputs(t)
	for k, want := range mainResultShapes {
		if g, ok := got[k]; !ok {
			t.Errorf("%s: missing", k)
		} else if g != want {
			t.Errorf("%s: %s, want %s", k, g, want)
		}
	}
	if len(got) != len(mainResultShapes) {
		t.Errorf("%d outputs, want %d", len(got), len(mainResultShapes))
	}
}

var mainResultShapes = map[string]string{
	"array of ints/alone":              "throw TypeError: cannot convert value: internal error: array",
	"array of ints/any":                "throw TypeError: cannot convert value: internal error: array",
	"array of ints/map":                "throw TypeError: cannot convert value: internal error: array",
	"array of ints/results":            "throw TypeError: callback: internal error: array",
	"array of ints/slice":              "throw TypeError: cannot convert value: internal error: array",
	"bad key nested/alone":             "throw TypeError: cannot convert value: unsupported map key type: float64",
	"bad key nested/any":               "throw TypeError: cannot convert value: unsupported map key type: float64",
	"bad key nested/map":               "throw TypeError: cannot convert value: unsupported map key type: float64",
	"bad key nested/results":           "throw TypeError: callback: unsupported map key type: float64",
	"bad key nested/slice":             "throw TypeError: cannot convert value: unsupported map key type: float64",
	"empty bool key map/alone":         "object {}",
	"empty bool key map/any":           "object {}",
	"empty bool key map/map":           "object {}",
	"empty bool key map/results":       "object {}",
	"empty bool key map/slice":         "object {}",
	"empty error map/alone":            "string \"errMap 0\"",
	"empty error map/any":              "string \"errMap 0\"",
	"empty error map/map":              "string \"errMap 0\"",
	"empty error map/results":          "string \"errMap 0\"",
	"empty error map/slice":            "string \"errMap 0\"",
	"empty float key map/alone":        "object {}",
	"empty float key map/any":          "object {}",
	"empty float key map/map":          "object {}",
	"empty float key map/results":      "object {}",
	"empty float key map/slice":        "object {}",
	"empty map of Values/alone":        "object {}",
	"empty map of Values/any":          "object {}",
	"empty map of Values/map":          "object {}",
	"empty map of Values/results":      "object {}",
	"empty map of Values/slice":        "object {}",
	"empty map of any/alone":           "object {}",
	"empty map of any/any":             "object {}",
	"empty map of any/map":             "object {}",
	"empty map of any/results":         "object {}",
	"empty map of any/slice":           "object {}",
	"empty map of ints/alone":          "object {}",
	"empty map of ints/any":            "object {}",
	"empty map of ints/map":            "object {}",
	"empty map of ints/results":        "object {}",
	"empty map of ints/slice":          "object {}",
	"empty maps nested/alone":          "object [{},{},{\"f\":{}}]",
	"empty maps nested/any":            "object [{},{},{\"f\":{}}]",
	"empty maps nested/map":            "object [{},{},{\"f\":{}}]",
	"empty maps nested/results":        "object [{},{},{\"f\":{}}]",
	"empty maps nested/slice":          "object [{},{},{\"f\":{}}]",
	"empty slice of Values/alone":      "object []",
	"empty slice of Values/any":        "object []",
	"empty slice of Values/map":        "object []",
	"empty slice of Values/results":    "object []",
	"empty slice of Values/slice":      "object []",
	"empty slice of any/alone":         "object []",
	"empty slice of any/any":           "object []",
	"empty slice of any/map":           "object []",
	"empty slice of any/results":       "object []",
	"empty slice of any/slice":         "object []",
	"empty slice of ints/alone":        "object []",
	"empty slice of ints/any":          "object []",
	"empty slice of ints/map":          "object []",
	"empty slice of ints/results":      "object []",
	"empty slice of ints/slice":        "object []",
	"empty struct key map/alone":       "object {}",
	"empty struct key map/any":         "object {}",
	"empty struct key map/map":         "object {}",
	"empty struct key map/results":     "object {}",
	"empty struct key map/slice":       "object {}",
	"error ints/alone":                 "string \"errInts [1 2]\"",
	"error ints/any":                   "string \"errInts [1 2]\"",
	"error ints/map":                   "string \"errInts [1 2]\"",
	"error ints/results":               "string \"errInts [1 2]\"",
	"error ints/slice":                 "string \"errInts [1 2]\"",
	"error pointer/alone":              "string \"errPtr 3\"",
	"error pointer/any":                "string \"errPtr 3\"",
	"error pointer/map":                "string \"errPtr 3\"",
	"error pointer/results":            "string \"errPtr 3\"",
	"error pointer/slice":              "string \"errPtr 3\"",
	"error string/alone":               "string \"errStr x\"",
	"error string/any":                 "string \"errStr x\"",
	"error string/map":                 "string \"errStr x\"",
	"error string/results":             "string \"errStr x\"",
	"error string/slice":               "string \"errStr x\"",
	"errors nested/alone":              "object [\"errMap 0\",\"errPtr nil\",\"errInts [4]\",{\"e\":\"errSlice 0\"}]",
	"errors nested/any":                "object [\"errMap 0\",\"errPtr nil\",\"errInts [4]\",{\"e\":\"errSlice 0\"}]",
	"errors nested/map":                "object [\"errMap 0\",\"errPtr nil\",\"errInts [4]\",{\"e\":\"errSlice 0\"}]",
	"errors nested/results":            "object [\"errMap 0\",\"errPtr nil\",\"errInts [4]\",{\"e\":\"errSlice 0\"}]",
	"errors nested/slice":              "object [\"errMap 0\",\"errPtr nil\",\"errInts [4]\",{\"e\":\"errSlice 0\"}]",
	"float key map/alone":              "throw TypeError: cannot convert value: unsupported map key type: float64",
	"float key map/any":                "throw TypeError: cannot convert value: unsupported map key type: float64",
	"float key map/map":                "throw TypeError: cannot convert value: unsupported map key type: float64",
	"float key map/results":            "throw TypeError: callback: unsupported map key type: float64",
	"float key map/slice":              "throw TypeError: cannot convert value: unsupported map key type: float64",
	"map of ints/alone":                "object {\"a\":1,\"b\":2}",
	"map of ints/any":                  "object {\"a\":1,\"b\":2}",
	"map of ints/map":                  "object {\"a\":1,\"b\":2}",
	"map of ints/results":              "object {\"a\":1,\"b\":2}",
	"map of ints/slice":                "object {\"a\":1,\"b\":2}",
	"nil any/alone":                    "object null",
	"nil any/any":                      "object null",
	"nil any/map":                      "object null",
	"nil any/results":                  "object null",
	"nil any/slice":                    "object null",
	"nil bool key map/alone":           "object null",
	"nil bool key map/any":             "object null",
	"nil bool key map/map":             "object null",
	"nil bool key map/results":         "object null",
	"nil bool key map/slice":           "object null",
	"nil error ints/alone":             "string \"errInts []\"",
	"nil error ints/any":               "string \"errInts []\"",
	"nil error ints/map":               "string \"errInts []\"",
	"nil error ints/results":           "string \"errInts []\"",
	"nil error ints/slice":             "string \"errInts []\"",
	"nil error map in any/alone":       "string \"errMap 0\"",
	"nil error map in any/any":         "string \"errMap 0\"",
	"nil error map in any/map":         "string \"errMap 0\"",
	"nil error map in any/results":     "string \"errMap 0\"",
	"nil error map in any/slice":       "string \"errMap 0\"",
	"nil error map/alone":              "string \"errMap 0\"",
	"nil error map/any":                "string \"errMap 0\"",
	"nil error map/map":                "string \"errMap 0\"",
	"nil error map/results":            "string \"errMap 0\"",
	"nil error map/slice":              "string \"errMap 0\"",
	"nil error pointer in any/alone":   "string \"errPtr nil\"",
	"nil error pointer in any/any":     "string \"errPtr nil\"",
	"nil error pointer in any/map":     "string \"errPtr nil\"",
	"nil error pointer in any/results": "string \"errPtr nil\"",
	"nil error pointer in any/slice":   "string \"errPtr nil\"",
	"nil error pointer/alone":          "string \"errPtr nil\"",
	"nil error pointer/any":            "string \"errPtr nil\"",
	"nil error pointer/map":            "string \"errPtr nil\"",
	"nil error pointer/results":        "string \"errPtr nil\"",
	"nil error pointer/slice":          "string \"errPtr nil\"",
	"nil error slice/alone":            "string \"errSlice 0\"",
	"nil error slice/any":              "string \"errSlice 0\"",
	"nil error slice/map":              "string \"errSlice 0\"",
	"nil error slice/results":          "string \"errSlice 0\"",
	"nil error slice/slice":            "string \"errSlice 0\"",
	"nil error/alone":                  "object null",
	"nil error/any":                    "object null",
	"nil error/map":                    "object null",
	"nil error/results":                "object null",
	"nil error/slice":                  "object null",
	"nil map in any/alone":             "object null",
	"nil map in any/any":               "object null",
	"nil map in any/map":               "object null",
	"nil map in any/results":           "object null",
	"nil map in any/slice":             "object null",
	"nil map of Values/alone":          "object null",
	"nil map of Values/any":            "object null",
	"nil map of Values/map":            "object null",
	"nil map of Values/results":        "object null",
	"nil map of Values/slice":          "object null",
	"nil map of any/alone":             "object null",
	"nil map of any/any":               "object null",
	"nil map of any/map":               "object null",
	"nil map of any/results":           "object null",
	"nil map of any/slice":             "object null",
	"nil map of ints/alone":            "object null",
	"nil map of ints/any":              "object null",
	"nil map of ints/map":              "object null",
	"nil map of ints/results":          "object null",
	"nil map of ints/slice":            "object null",
	"nil maps nested/alone":            "object [null,null,[null],{\"m\":null}]",
	"nil maps nested/any":              "object [null,null,[null],{\"m\":null}]",
	"nil maps nested/map":              "object [null,null,[null],{\"m\":null}]",
	"nil maps nested/results":          "object [null,null,[null],{\"m\":null}]",
	"nil maps nested/slice":            "object [null,null,[null],{\"m\":null}]",
	"nil named map of any/alone":       "object null",
	"nil named map of any/any":         "object null",
	"nil named map of any/map":         "object null",
	"nil named map of any/results":     "object null",
	"nil named map of any/slice":       "object null",
	"nil named map/alone":              "object null",
	"nil named map/any":                "object null",
	"nil named map/map":                "object null",
	"nil named map/results":            "object null",
	"nil named map/slice":              "object null",
	"nil named slice/alone":            "object []",
	"nil named slice/any":              "object []",
	"nil named slice/map":              "object []",
	"nil named slice/results":          "object []",
	"nil named slice/slice":            "object []",
	"nil pointer in any/alone":         "object null",
	"nil pointer in any/any":           "object null",
	"nil pointer in any/map":           "object null",
	"nil pointer in any/results":       "object null",
	"nil pointer in any/slice":         "object null",
	"nil pointer to Value/alone":       "object null",
	"nil pointer to Value/any":         "object null",
	"nil pointer to Value/map":         "object null",
	"nil pointer to Value/results":     "object null",
	"nil pointer to Value/slice":       "object null",
	"nil pointer to int/alone":         "object null",
	"nil pointer to int/any":           "object null",
	"nil pointer to int/map":           "object null",
	"nil pointer to int/results":       "object null",
	"nil pointer to int/slice":         "object null",
	"nil pointer to struct/alone":      "object null",
	"nil pointer to struct/any":        "object null",
	"nil pointer to struct/map":        "object null",
	"nil pointer to struct/results":    "object null",
	"nil pointer to struct/slice":      "object null",
	"nil pointers nested/alone":        "object [null,null,[null]]",
	"nil pointers nested/any":          "object [null,null,[null]]",
	"nil pointers nested/map":          "object [null,null,[null]]",
	"nil pointers nested/results":      "object [null,null,[null]]",
	"nil pointers nested/slice":        "object [null,null,[null]]",
	"nil slice in any/alone":           "object []",
	"nil slice in any/any":             "object []",
	"nil slice in any/map":             "object []",
	"nil slice in any/results":         "object []",
	"nil slice in any/slice":           "object []",
	"nil slice of Values/alone":        "object []",
	"nil slice of Values/any":          "object []",
	"nil slice of Values/map":          "object []",
	"nil slice of Values/results":      "object []",
	"nil slice of Values/slice":        "object []",
	"nil slice of any/alone":           "object []",
	"nil slice of any/any":             "object []",
	"nil slice of any/map":             "object []",
	"nil slice of any/results":         "object []",
	"nil slice of any/slice":           "object []",
	"nil slice of ints/alone":          "object []",
	"nil slice of ints/any":            "object []",
	"nil slice of ints/map":            "object []",
	"nil slice of ints/results":        "object []",
	"nil slice of ints/slice":          "object []",
	"nil slices nested/alone":          "object {\"a\":[],\"b\":[],\"c\":{\"d\":[]}}",
	"nil slices nested/any":            "object {\"a\":[],\"b\":[],\"c\":{\"d\":[]}}",
	"nil slices nested/map":            "object {\"a\":[],\"b\":[],\"c\":{\"d\":[]}}",
	"nil slices nested/results":        "object {\"a\":[],\"b\":[],\"c\":{\"d\":[]}}",
	"nil slices nested/slice":          "object {\"a\":[],\"b\":[],\"c\":{\"d\":[]}}",
	"pointer to int/alone":             "number 1",
	"pointer to int/any":               "number 1",
	"pointer to int/map":               "number 1",
	"pointer to int/results":           "number 1",
	"pointer to int/slice":             "number 1",
	"pointer to nil slice/alone":       "object []",
	"pointer to nil slice/any":         "object []",
	"pointer to nil slice/map":         "object []",
	"pointer to nil slice/results":     "object []",
	"pointer to nil slice/slice":       "object []",
	"pointer to pointer/alone":         "throw TypeError: cannot convert value: type not supported: **int",
	"pointer to pointer/any":           "throw TypeError: cannot convert value: type not supported: **int",
	"pointer to pointer/map":           "throw TypeError: cannot convert value: type not supported: **int",
	"pointer to pointer/results":       "throw TypeError: callback: type not supported: **int",
	"pointer to pointer/slice":         "throw TypeError: cannot convert value: type not supported: **int",
	"struct with values/alone":         "object {\"A\":1,\"B\":[2],\"C\":{\"c\":3},\"D\":1,\"E\":\"e\"}",
	"struct with values/any":           "object {\"A\":1,\"B\":[2],\"C\":{\"c\":3},\"D\":1,\"E\":\"e\"}",
	"struct with values/map":           "object {\"A\":1,\"B\":[2],\"C\":{\"c\":3},\"D\":1,\"E\":\"e\"}",
	"struct with values/results":       "object {\"A\":1,\"B\":[2],\"C\":{\"c\":3},\"D\":1,\"E\":\"e\"}",
	"struct with values/slice":         "object {\"A\":1,\"B\":[2],\"C\":{\"c\":3},\"D\":1,\"E\":\"e\"}",
	"struct/alone":                     "object {\"A\":0,\"B\":null,\"C\":null,\"D\":null,\"E\":null}",
	"struct/any":                       "object {\"A\":0,\"B\":null,\"C\":null,\"D\":null,\"E\":null}",
	"struct/map":                       "object {\"A\":0,\"B\":null,\"C\":null,\"D\":null,\"E\":null}",
	"struct/results":                   "object {\"A\":0,\"B\":null,\"C\":null,\"D\":null,\"E\":null}",
	"struct/slice":                     "object {\"A\":0,\"B\":null,\"C\":null,\"D\":null,\"E\":null}",
}
