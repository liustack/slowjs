// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"strings"
	"testing"
)

// panicError is an error whose Error method panics, so that converting it
// panics in the middle of a result.
type panicError struct{}

func (panicError) Error() string { panic("boom") }

// loop is a type that holds itself and can hold no Value.
type loop []loop

// resultCases are results of Go functions and what the call does with them.
// ref gives one more reference to the same object each time it is called,
// and each case puts every reference it gets in one place of the result: a
// variable, an element of a slice or array, a value in a map or the target
// of a pointer. A place the result reaches twice is still one reference.
var resultCases = []struct {
	name string
	f    func(ref func() Value) (any, error)
	want string // "ok", "throws" (a TypeError) or "panics"
}{
	{"value", func(ref func() Value) (any, error) { return ref(), nil }, "ok"},
	{"value and its Dup", func(ref func() Value) (any, error) { v := ref(); return []any{v, v.Dup()}, nil }, "ok"},
	{"slice twice", func(ref func() Value) (any, error) { s := []Value{ref()}; return []any{s, s}, nil }, "ok"},
	{"overlapping slices", func(ref func() Value) (any, error) {
		s := []any{ref(), ref(), ref()}
		return []any{s[:2], s[1:]}, nil
	}, "ok"},
	{"pointer twice", func(ref func() Value) (any, error) { v := ref(); return []any{&v, &v}, nil }, "ok"},
	{"slice and a pointer into it", func(ref func() Value) (any, error) { s := []Value{ref()}; return []any{s, &s[0]}, nil }, "ok"},
	{"map twice", func(ref func() Value) (any, error) { mp := map[string]Value{"a": ref()}; return []any{mp, mp}, nil }, "ok"},
	{"map of one slice twice", func(ref func() Value) (any, error) {
		s := []Value{ref()}
		return map[string]any{"a": s, "b": s}, nil
	}, "ok"},

	{"many places twice", func(ref func() Value) (any, error) {
		s := make([]Value, 40)
		for i := range s {
			s[i] = ref()
		}
		return []any{s, s}, nil
	}, "ok"},

	{"array", func(ref func() Value) (any, error) { return [1]Value{ref()}, nil }, "throws"},
	{"array in a slice", func(ref func() Value) (any, error) { return []any{[1]Value{ref()}, ref()}, nil }, "throws"},
	{"map of arrays", func(ref func() Value) (any, error) { return map[string][1]Value{"a": {ref()}}, nil }, "throws"},
	{"pointer to a pointer", func(ref func() Value) (any, error) { v := ref(); p := &v; return &p, nil }, "throws"},
	{"Value as a map key", func(ref func() Value) (any, error) { return map[Value]int{ref(): 1}, nil }, "throws"},
	{"slice that holds itself", func(ref func() Value) (any, error) {
		s := []any{ref(), nil, ref()}
		s[1] = s
		return s, nil
	}, "throws"},
	{"map that holds itself", func(ref func() Value) (any, error) {
		mp := map[string]any{"a": ref(), "c": ref()}
		mp["b"] = mp
		return mp, nil
	}, "throws"},
	{"pointer to itself", func(ref func() Value) (any, error) {
		var x any
		x = &x
		return []any{ref(), x, ref()}, nil
	}, "throws"},
	{"cycle after many places", func(ref func() Value) (any, error) {
		s := make([]any, 41)
		for i := range 40 {
			s[i] = ref()
		}
		s[40] = s
		return s, nil
	}, "throws"},
	{"cycle without a Value", func(ref func() Value) (any, error) {
		s := []any{nil}
		s[0] = s
		return s, nil
	}, "throws"},
	{"type that holds itself", func(ref func() Value) (any, error) {
		l := loop{nil}
		l[0] = l
		return l, nil
	}, "throws"},

	{"error with a value", func(ref func() Value) (any, error) { return ref(), errors.New("no") }, "throws"},
	{"error with a slice twice", func(ref func() Value) (any, error) {
		s := []Value{ref()}
		return []any{s, s}, errors.New("no")
	}, "throws"},
	{"error with an array", func(ref func() Value) (any, error) { return [1]Value{ref()}, errors.New("no") }, "throws"},
	{"error with a map of arrays", func(ref func() Value) (any, error) {
		return map[string][1]Value{"a": {ref()}}, errors.New("no")
	}, "throws"},
	{"error with a pointer to a pointer", func(ref func() Value) (any, error) {
		v := ref()
		p := &v
		return &p, errors.New("no")
	}, "throws"},
	{"error with a cycle", func(ref func() Value) (any, error) {
		s := []any{ref(), nil}
		s[1] = s
		return s, errors.New("no")
	}, "throws"},

	{"panic in a slice", func(ref func() Value) (any, error) { return []any{ref(), panicError{}, ref()}, nil }, "panics"},
	{"panic in a nested slice", func(ref func() Value) (any, error) {
		return []any{[]any{ref(), panicError{}, ref()}, ref()}, nil
	}, "panics"},
	{"panic in a map", func(ref func() Value) (any, error) {
		return map[string]any{"a": ref(), "b": panicError{}, "c": ref()}, nil
	}, "panics"},
	{"panic in the error", func(ref func() Value) (any, error) { return []any{ref()}, panicError{} }, "panics"},
}

// TestResultReferences returns each of resultCases from a host function, and
// from a function of RegisterFunc when it has no error, and checks that the
// call takes exactly the references the result holds: once each, whether the
// result converts, fails to convert, has an error or panics, and that a
// result that holds itself is an error, not a crash.
func TestResultReferences(t *testing.T) {
	for _, c := range resultCases {
		for _, how := range []string{"RegisterHostFunc", "RegisterFunc"} {
			// A function of RegisterFunc returns no error to throw.
			if how == "RegisterFunc" && (strings.HasPrefix(c.name, "error") || c.name == "panic in the error") {
				continue
			}
			t.Run(c.name+"/"+how, func(t *testing.T) {
				m := newTestVM(t)
				obj, err := m.NewObjectValue()
				if err != nil {
					t.Fatal(err)
				}
				// Two references of the test's own, so that a reference freed
				// twice leaves the object alive to count.
				_ = obj.Dup()
				ref := func() Value { return obj.Dup() }
				switch how {
				case "RegisterHostFunc":
					err = m.RegisterHostFunc("f", func([]any) (any, error) { return c.f(ref) })
				default:
					err = m.RegisterFunc("f", func() any { r, _ := c.f(ref); return r }, false)
				}
				if err != nil {
					t.Fatal(err)
				}

				switch c.want {
				case "ok":
					if r, err := m.Eval(`f(); true`, EvalGlobal); err != nil || r != true {
						t.Fatalf("got %v, %v", r, err)
					}
				case "throws":
					r, err := m.Eval(`try { f(); 'returned' } catch (e) { e instanceof TypeError ? e.message : String(e) }`, EvalGlobal)
					s, _ := r.(string)
					if err != nil || s == "returned" || s == "" {
						t.Fatalf("got %v, %v, want a TypeError", r, err)
					}
					if !strings.HasPrefix(c.name, "error") && (strings.Contains(c.name, "itself") || strings.Contains(c.name, "cycle")) {
						if !strings.Contains(s, "contains itself") {
							t.Errorf("got the message %q", s)
						}
					}
				case "panics":
					if p := evalRecover(m, `f()`); p != "boom" {
						t.Fatalf("recovered %v, want boom", p)
					}
				}
				wantTestReferences(t, m, obj)
			})
		}
	}
}

// TestResultsReferences is TestResultReferences for the several results of
// a function of RegisterFunc, which are an array to javascript.
func TestResultsReferences(t *testing.T) {
	for _, c := range []struct {
		name string
		f    func(ref func() Value) any
		ok   bool
	}{
		{"one slice twice", func(ref func() Value) any {
			return func() ([]Value, []Value) { s := []Value{ref()}; return s, s }
		}, true},
		{"value and a pointer to it", func(ref func() Value) any {
			return func() (*Value, any) { v := ref(); return &v, &v }
		}, true},
		{"an array after a value", func(ref func() Value) any {
			return func() (Value, [1]Value) { return ref(), [1]Value{ref()} }
		}, false},
		{"a cycle after a value", func(ref func() Value) any {
			return func() (Value, []any) {
				s := []any{ref(), nil}
				s[1] = s
				return ref(), s
			}
		}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			obj, err := m.NewObjectValue()
			if err != nil {
				t.Fatal(err)
			}
			_ = obj.Dup()
			if err := m.RegisterFunc("f", c.f(func() Value { return obj.Dup() }), false); err != nil {
				t.Fatal(err)
			}
			r, err := m.Eval(`try { f(); true } catch (e) { e instanceof TypeError ? false : String(e) }`, EvalGlobal)
			if err != nil || r != c.ok {
				t.Fatalf("got %v, %v, want %v", r, err, c.ok)
			}
			wantTestReferences(t, m, obj)
		})
	}
}

// wantTestReferences checks, after a GC, that obj has the two references the
// test holds, and frees what it has, so that a wrong count is reported, not
// an abort of the engine on a reference freed twice.
func wantTestReferences(t *testing.T, m *VM, obj Value) {
	t.Helper()
	m.RunGC()
	n := *refCount(obj.v)
	if n != 2 {
		t.Errorf("the object has %d references, want the test's 2", n)
	}
	for range n {
		m.jsFreeValue(obj.v)
	}
}

// A Go function that javascript calls while the results of another are
// converted, from the toJSON of a Value in a struct, frees its own
// references, and the outer call frees its own: the room the two keep them
// in is not shared. A call of inner first leaves room for the next call to
// take.
func TestResultReferencesNested(t *testing.T) {
	m := newTestVM(t)
	outer, err := m.NewObjectValue()
	if err != nil {
		t.Fatal(err)
	}
	inner, err := m.NewObjectValue()
	if err != nil {
		t.Fatal(err)
	}
	_ = outer.Dup()
	_ = inner.Dup()
	if err := m.RegisterHostFunc("inner", func([]any) (any, error) {
		return []any{inner.Dup(), inner.Dup()}, nil
	}); err != nil {
		t.Fatal(err)
	}
	withToJSON, err := m.EvalValue(`({ toJSON() { inner(); inner(); return 1 } })`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer withToJSON.Free()
	type holder struct{ V Value }
	if err := m.RegisterHostFunc("outer", func([]any) (any, error) {
		return []any{outer.Dup(), holder{withToJSON}, outer.Dup()}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if r, err := m.Eval(`inner(); JSON.stringify(outer()[1])`, EvalGlobal); err != nil || r != `{"V":1}` {
		t.Fatalf("got %v, %v", r, err)
	}
	wantTestReferences(t, m, outer)
	wantTestReferences(t, m, inner)
}
