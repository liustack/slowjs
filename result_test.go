// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

// panicError is an error whose Error method panics, so that converting it
// panics in the middle of a result.
type panicError struct{}

func (panicError) Error() string { panic("boom") }

// loop is a type that holds itself and can hold no Value.
type loop []loop

// namedValueMap, namedAnyMap and namedValues name a map or a slice that a
// result also holds under its own type.
type (
	namedValueMap map[string]Value
	namedAnyMap   map[string]any
	namedValues   []Value
)

// mutator is an error, and mutJSON a struct with a MarshalJSON method, that
// run f when the call converts them, to change the result around them.
type mutator struct{ f func() }

func (e mutator) Error() string { e.f(); return "mutator" }

type mutJSON struct{ f func() }

func (j mutJSON) MarshalJSON() ([]byte, error) { j.f(); return []byte(`"json"`), nil }

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

	{"map under two names", func(ref func() Value) (any, error) {
		mp := map[string]Value{"v": ref()}
		return []any{mp, namedValueMap(mp)}, nil
	}, "ok"},
	{"map of any under two names", func(ref func() Value) (any, error) {
		mp := map[string]any{"v": ref()}
		return []any{mp, namedAnyMap(mp)}, nil
	}, "ok"},
	{"slice under two names", func(ref func() Value) (any, error) {
		s := []Value{ref()}
		return []any{s, namedValues(s)}, nil
	}, "ok"},
	{"cycle made while the result converts", func(ref func() Value) (any, error) {
		s := []any{nil, nil, ref()}
		s[0] = mutator{func() { s[1] = s }}
		return s, nil
	}, "ok"},
	{"Value taken out while the result converts", func(ref func() Value) (any, error) {
		s := []any{nil, ref()}
		s[0] = mutator{func() { s[1] = nil }}
		return s, nil
	}, "ok"},
	{"cycle made by a MarshalJSON", func(ref func() Value) (any, error) {
		inner := []any{nil, ref()}
		mp := map[string]any{"b": inner}
		mp["a"] = mutJSON{func() { inner[0] = mp }}
		return mp, nil
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

// A Value of another VM that user code puts into a result while the call
// converts it is not read: the conversion reads the result as it was when
// the call took its references, so the other VM's Value keeps its count.
func TestResultChangedDuringConversion(t *testing.T) {
	m := newTestVM(t)
	other := newTestVM(t)
	theirs, err := other.NewObjectValue()
	if err != nil {
		t.Fatal(err)
	}
	defer theirs.Free()
	if err := m.RegisterHostFunc("f", func([]any) (any, error) {
		s := []any{nil, nil}
		s[0] = mutator{func() { s[1] = theirs }}
		return s, nil
	}); err != nil {
		t.Fatal(err)
	}
	if r, err := m.Eval(`JSON.stringify(f())`, EvalGlobal); err != nil || r != `["mutator",null]` {
		t.Fatalf("got %v, %v", r, err)
	}
	if n := *refCount(theirs.v); n != 1 {
		t.Errorf("the Value of the other VM has %d references, want 1", n)
	}
}

// The conversion of a result reads the message of an exception it meets,
// here of the JSON of an empty *Object, through a toString of the script,
// which can call Go and move a Value in the result. The walk took each
// reference before any of that ran, so each is freed once.
func TestResultExceptionMovesValue(t *testing.T) {
	m := newTestVM(t)
	obj, err := m.NewObjectValue()
	if err != nil {
		t.Fatal(err)
	}
	_ = obj.Dup()
	var s []any
	moved := 0
	if err := m.RegisterFunc("move", func() { s[2], s[0] = s[0], nil; moved++ }, false); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterFunc("f", func() []any { s = []any{obj.Dup(), &Object{}, nil}; return s }, false); err != nil {
		t.Fatal(err)
	}
	r, err := m.Eval(`
		SyntaxError.prototype.toString = function () { move(); return "moved"; };
		try { f(); "returned" } catch (e) { String(e) }
	`, EvalGlobal)
	if err != nil || r == "returned" {
		t.Fatalf("got %v, %v, want an exception", r, err)
	}
	if moved == 0 {
		t.Fatal("the toString of the exception did not run")
	}
	wantTestReferences(t, m, obj)
}

// A leaf of a result is what the walk found: a string that an Error method
// puts in its place afterwards is not converted.
func TestResultLeafKeptByWalk(t *testing.T) {
	m := newTestVM(t)
	if err := m.RegisterFunc("f", func() []any {
		s := []any{nil, "before"}
		s[0] = mutator{func() { s[1] = "after" }}
		return s
	}, false); err != nil {
		t.Fatal(err)
	}
	if r, err := m.Eval(`JSON.stringify(f())`, EvalGlobal); err != nil || r != `["mutator","before"]` {
		t.Fatalf("got %v, %v", r, err)
	}
}

// errValues is a slice of Values that is an error, so that the conversion of
// a result turns it into the string of its Error method.
type errValues []Value

func (errValues) Error() string { return "errValues" }

// A result whose type has an Error method converts to that string, as a
// whole: the Values in it stay the caller's, as those in a struct do.
func TestResultErrorKeepsItsValues(t *testing.T) {
	for _, how := range []string{"RegisterFunc", "RegisterHostFunc", "RegisterHostFunc with an error"} {
		t.Run(how, func(t *testing.T) {
			m := newTestVM(t)
			obj, err := m.NewObjectValue()
			if err != nil {
				t.Fatal(err)
			}
			_ = obj.Dup()
			mine := obj.Dup()
			switch how {
			case "RegisterFunc":
				err = m.RegisterFunc("f", func() any { return []any{errValues{mine}} }, false)
			case "RegisterHostFunc":
				err = m.RegisterHostFunc("f", func([]any) (any, error) { return []any{errValues{mine}}, nil })
			default:
				err = m.RegisterHostFunc("f", func([]any) (any, error) { return []any{errValues{mine}}, errors.New("no") })
			}
			if err != nil {
				t.Fatal(err)
			}
			want := `["errValues"]`
			if strings.HasSuffix(how, "error") {
				want = "no"
			}
			if r, err := m.Eval(`try { JSON.stringify(f()) } catch (e) { e.message }`, EvalGlobal); err != nil || r != want {
				t.Fatalf("got %v, %v, want %s", r, err, want)
			}
			m.RunGC()
			if n := *refCount(obj.v); n != 3 {
				t.Errorf("the object has %d references, want the test's 3", n)
			}
			mine.Free()
			wantTestReferences(t, m, obj)
		})
	}
}

// holdsRefs is true for the types whose values can hold a Value or
// themselves, and only for those: a type that is twice in another, side by
// side, is not one that holds itself.
func TestHoldsRefs(t *testing.T) {
	for _, c := range []struct {
		t    reflect.Type
		want bool
	}{
		{reflect.TypeFor[int](), false},
		{reflect.TypeFor[map[*int]*int](), false},
		{reflect.TypeFor[map[string][]*int](), false},
		{reflect.TypeFor[[2][]*int](), false},
		{reflect.TypeFor[struct{ V Value }](), false},
		{reflect.TypeFor[errValues](), false},
		{reflect.TypeFor[*big.Int](), false},
		{reflect.TypeFor[Value](), true},
		{reflect.TypeFor[[]any](), true},
		{reflect.TypeFor[map[*int][]Value](), true},
		{reflect.TypeFor[[1]Value](), true},
		{reflect.TypeFor[loop](), true},
		{reflect.TypeFor[*error](), true},
	} {
		if got := holdsRefs(c.t); got != c.want {
			t.Errorf("holdsRefs(%v) = %v, want %v", c.t, got, c.want)
		}
	}
}

// The walk of a result allocates nothing once the VM has room for it: a
// call that returns 40 Values allocates as much as one that returns 2.
func TestResultWalkAllocs(t *testing.T) {
	m := newTestVM(t)
	obj, err := m.NewObjectValue()
	if err != nil {
		t.Fatal(err)
	}
	defer obj.Free()
	// 250 Values make 251 nodes, 250 kids, references and places, close
	// to keepSnapshot.
	few, many, most := make([]Value, 2), make([]Value, 40), make([]Value, 250)
	fill := func(s []Value) []Value {
		for i := range s {
			s[i] = obj.Dup()
		}
		return s
	}
	if err := m.RegisterFunc("few", func() []Value { return fill(few) }, false); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterFunc("many", func() []Value { return fill(many) }, false); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterFunc("most", func() []Value { return fill(most) }, false); err != nil {
		t.Fatal(err)
	}
	allocs := func(name string) float64 {
		fn, err := m.EvalValue(`(function () { `+name+`() })`, EvalGlobal)
		if err != nil {
			t.Fatal(err)
		}
		defer fn.Free()
		return testing.AllocsPerRun(50, func() {
			if _, err := fn.Call(UndefinedValue); err != nil {
				t.Fatal(err)
			}
		})
	}
	a := allocs("few")
	if b := allocs("many"); a != b {
		t.Errorf("a call that returns 2 Values allocates %v times, one that returns 40 %v", a, b)
	}
	if b := allocs("most"); a != b {
		t.Errorf("a call that returns 2 Values allocates %v times, one that returns 250 %v", a, b)
	}
}

// namedAnySlice names a []any.
type namedAnySlice []any

// resultGen makes random results: slices, maps and pointers, some under
// names of their own, nested, shared and, when asked, holding themselves.
// Every reference that ref gives goes into one place.
type resultGen struct {
	r      *rand.Rand
	ref    func() Value
	shared []any
	// spare are the slices, each with a last element left nil, and maps
	// where a cycle can go.
	spare []func(root any)
}

func (g *resultGen) value(depth int) any {
	n := g.r.IntN(10)
	switch {
	case depth >= 4 || n < 3:
		switch g.r.IntN(5) {
		case 0, 1:
			return g.ref()
		case 2:
			return "s"
		case 3:
			return 1
		}
		return nil
	case n < 4 && len(g.shared) > 0:
		// A part again, at times under the other name of its type.
		x := g.shared[g.r.IntN(len(g.shared))]
		if g.r.IntN(2) == 0 {
			switch y := x.(type) {
			case []any:
				return namedAnySlice(y)
			case namedAnySlice:
				return []any(y)
			case map[string]any:
				return namedAnyMap(y)
			case namedAnyMap:
				return map[string]any(y)
			}
		}
		return x
	case n < 6:
		s := make([]any, g.r.IntN(4)+1)
		for i := range len(s) - 1 {
			s[i] = g.value(depth + 1)
		}
		g.spare = append(g.spare, func(root any) { s[len(s)-1] = root })
		var x any = s
		if g.r.IntN(2) == 0 {
			x = namedAnySlice(s)
		}
		g.shared = append(g.shared, x)
		return x
	case n < 8:
		mp := map[string]any{}
		for i := range g.r.IntN(4) {
			mp[fmt.Sprintf("k%d", i)] = g.value(depth + 1)
		}
		g.spare = append(g.spare, func(root any) { mp["cycle"] = root })
		var x any = mp
		if g.r.IntN(2) == 0 {
			x = namedAnyMap(mp)
		}
		g.shared = append(g.shared, x)
		return x
	case n < 9:
		switch g.r.IntN(3) {
		case 0:
			s := make([]Value, g.r.IntN(3))
			for i := range s {
				s[i] = g.ref()
			}
			g.shared = append(g.shared, s)
			return s
		case 1:
			v := g.ref()
			g.shared = append(g.shared, &v)
			return &v
		}
	}
	x := g.value(depth + 1)
	p := &x
	g.shared = append(g.shared, p)
	return p
}

// Random results, with shared parts, names of their own and cycles, convert
// as encoding/json marshals them, or throw for a cycle, and the call takes
// exactly the references they hold.
func TestResultReferencesRandom(t *testing.T) {
	m := newTestVM(t)
	obj, err := m.NewObjectValue()
	if err != nil {
		t.Fatal(err)
	}
	_ = obj.Dup()
	var result any
	if err := m.RegisterHostFunc("host", func([]any) (any, error) { return result, nil }); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterFunc("fn", func() any { return result }, false); err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewPCG(1, 2))
	for i := range 2000 {
		g := resultGen{r: r, ref: func() Value { return obj.Dup() }}
		result = g.value(0)
		cycle := r.IntN(4) == 0 && len(g.spare) > 0
		var want string
		if cycle {
			g.spare[r.IntN(len(g.spare))](result)
			want = "contains itself"
		} else {
			b, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			want = string(b)
		}
		f := "host"
		if i%2 == 1 {
			f = "fn"
		}
		got, err := m.Eval(`try { JSON.stringify(`+f+`()) } catch (e) { e.message }`, EvalGlobal)
		s, _ := got.(string)
		if err != nil || (cycle && !strings.Contains(s, want)) || (!cycle && s != want) {
			t.Fatalf("case %d: got %v, %v, want %s", i, got, err, want)
		}
		result = nil
		m.RunGC()
		if n := *refCount(obj.v); n != 2 {
			t.Fatalf("case %d (%s): the object has %d references, want the test's 2", i, want, n)
		}
	}
	wantTestReferences(t, m, obj)
}
