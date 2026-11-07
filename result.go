// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// errCycle is the error of a result of a Go function that contains itself,
// which no conversion could finish. encoding/json refuses such a value too.
var errCycle = errors.New("quickjs: a result that contains itself")

// The results of a Go function, of RegisterFunc or RegisterHostFunc, hand
// the call the references they hold, whether they convert or not:
//
//   - A reference is a place that holds a Value of the VM: a result, an
//     element of a slice or an array, a key or a value of a map, the target
//     of a pointer, and the any that holds one of them. A Value copied, by
//     an assignment or into an any, is not a new reference: only Dup makes
//     one, and nothing in a Value tells a copy from a Dup. So a place is a
//     reference, and a place the results reach twice, through the same
//     slice, map or pointer, under any name of its type, is still one.
//   - One walk of the results, which runs no code of the program, takes a
//     snapshot of them: the references, each once, the slices, arrays, maps
//     and pointers as they are, and the leaves the conversion turns into
//     javascript one by one. It refuses a result that contains itself and
//     a Value of another VM.
//   - The conversion reads only the snapshot. The code of the program it
//     runs, an Error method, a MarshalJSON, a toJSON that calls Go again,
//     can change the results but not the snapshot, so it cannot make the
//     conversion recurse without end or reach a Value the walk did not see.
//   - The conversion Dups the Values it puts in the javascript value, and
//     the call frees the references of the snapshot when it is done: when
//     the results convert, when they do not, when a host function returns
//     an error and when the conversion panics.
//   - A leaf is converted as a whole, and the Values in it, if any, stay
//     the caller's: a struct, which converts through encoding/json, and a
//     value whose type has an Error method, which converts to the string of
//     Error. A String method changes nothing: a slice or a map that has one
//     is walked like another.

// snapKind is what a node of a snapshot is.
type snapKind uint8

const (
	snapNull   snapKind = iota // null: a nil interface, pointer or map
	snapLeaf                   // leaf, which holds no Value and converts as it is
	snapValue                  // v, a Value of the VM or of none
	snapArray                  // the kids [start, start+n), in order
	snapObject                 // the kids [start, start+n), in the order of their keys
	snapFail                   // err: the conversion refuses it
)

// snapNode is a node of a snapshot.
type snapNode struct {
	kind     snapKind
	start, n int32
	v        jsValue
	leaf     reflect.Value
	err      error
}

// snapKid is an element of an array or a property of an object.
type snapKid struct {
	key  string
	node int32
}

// snapshot is the results of a call as the walk found them. Its slices are
// kept on the VM from one call to the next (VM.snap).
type snapshot struct {
	nodes []snapNode
	kids  []snapKid
	// held are the references the results hand over.
	held []jsValue
	// vals holds the values of the arrays being made, one after another.
	vals []jsValue
	// many has the places of refWalk once its few are full.
	many map[refPlace]int32
}

// keepSnapshot is the most of each slice and map of a snapshot that the VM
// keeps for the next call.
const keepSnapshot = 256

// takeSnapshot gives the call the snapshot the VM keeps, or a new one when
// a call that is not done yet has it.
func (m *VM) takeSnapshot() *snapshot {
	s := m.snap
	if s == nil {
		return new(snapshot)
	}
	m.snap = nil
	return s
}

// releaseSnapshot frees the references of s, clears it and keeps it for
// the next call, unless it is large.
func (m *VM) releaseSnapshot(s *snapshot) {
	for _, v := range s.held {
		m.jsFreeValue(v)
	}
	// What the nodes and keys point to is left to the GC.
	clear(s.nodes)
	clear(s.kids)
	if len(s.many) > keepSnapshot {
		s.many = nil
	}
	clear(s.many)
	s.nodes, s.kids, s.held, s.vals = s.nodes[:0], s.kids[:0], s.held[:0], s.vals[:0]
	if cap(s.nodes) <= keepSnapshot && cap(s.kids) <= keepSnapshot && cap(s.held) <= keepSnapshot && cap(s.vals) <= keepSnapshot {
		m.snap = s
	}
}

// convertResults converts out, the results of a call that can hold a Value
// by their types: the one result, or an array of them all when many is set.
func (m *VM) convertResults(out []reflect.Value, many bool) (r jsValue, err error) {
	if !many {
		v := out[0]
		if v.Kind() == reflect.Interface && !v.IsNil() {
			v = v.Elem()
		}
		switch {
		case v.Type() == rvt:
			// A Value alone is a place of its own, and the reference it
			// hands over is the result.
			x := v.Interface().(Value)
			if m.foreign(x) {
				return r, errForeignValue
			}
			return x.jsValue(), nil
		case !refsIn(v):
			return m.jsValue(v)
		}
	}

	s := m.takeSnapshot()
	defer m.releaseSnapshot(s)
	w := refWalk{m: m, s: s}
	if !many {
		root := w.node(out[0], false)
		if w.err != nil {
			return r, w.err
		}
		return m.build(s, root)
	}
	start := w.reserve(len(out))
	for i, v := range out {
		s.kids[start+i].node = w.node(v, false)
	}
	if w.err != nil {
		return r, w.err
	}
	return m.buildArray(s, int32(start), int32(len(out)))
}

// dropResult frees the references v hands over, a result the call does not
// convert, as that of a host function that returns an error.
func (m *VM) dropResult(v reflect.Value) {
	s := m.takeSnapshot()
	w := refWalk{m: m, s: s}
	w.node(v, false)
	m.releaseSnapshot(s)
}

// refWalk is the walk that takes a snapshot.
type refWalk struct {
	m   *VM
	s   *snapshot
	err error
	// The places and maps the walk is in (node -1), to find a cycle, and
	// the ones it is done with and their nodes, to go through each once:
	// the first ones in few, and all of them in s.many once few is full.
	n        int
	few      [16]refSeen
	overflow bool
}

type refSeen struct {
	at   refPlace
	node int32
}

// refPlace is a place in memory, an addressable value with its type, so
// that an array and its first element are two places, or a map, with no
// type, so that a map is one place under every name of its type.
type refPlace struct {
	p unsafe.Pointer
	t reflect.Type
}

func (w *refWalk) fail(err error) {
	if w.err == nil {
		w.err = err
	}
}

// add adds nd to the snapshot and returns its index.
func (w *refWalk) add(nd snapNode) int32 {
	w.s.nodes = append(w.s.nodes, nd)
	return int32(len(w.s.nodes) - 1)
}

// failed adds a node of err, which the conversion refuses.
func (w *refWalk) failed(err error) int32 {
	return w.add(snapNode{kind: snapFail, err: err})
}

// reserve adds n kids to the snapshot and returns the index of the first.
func (w *refWalk) reserve(n int) int {
	start := len(w.s.kids)
	w.s.kids = slices.Grow(w.s.kids, n)[:start+n]
	return start
}

// enter returns the node of at when the walk has met it, a node of errCycle
// when the walk is still inside at, or else records that the walk is in at
// and returns where.
func (w *refWalk) enter(at refPlace) (slot int, node int32, met bool) {
	if !w.overflow {
		for i := range w.n {
			if w.few[i].at == at {
				return 0, w.metNode(w.few[i].node), true
			}
		}
		if w.n < len(w.few) {
			w.few[w.n] = refSeen{at, -1}
			w.n++
			return w.n - 1, 0, false
		}
		w.overflow = true
		if w.s.many == nil {
			w.s.many = make(map[refPlace]int32, 2*len(w.few))
		}
		for _, seen := range w.few {
			w.s.many[seen.at] = seen.node
		}
	}
	if nd, ok := w.s.many[at]; ok {
		return 0, w.metNode(nd), true
	}
	w.s.many[at] = -1
	return -1, 0, false
}

// metNode is the node of a place the walk meets again.
func (w *refWalk) metNode(node int32) int32 {
	if node < 0 {
		w.fail(errCycle)
		return w.failed(errCycle)
	}
	return node
}

// leave records the node of at, which enter put at slot.
func (w *refWalk) leave(at refPlace, slot int, node int32) {
	if slot >= 0 && !w.overflow {
		w.few[slot].node = node
		return
	}
	w.s.many[at] = node
}

// refsIn reports whether v can hold a Value or itself: it is not nil, and
// its type, or the type in it for an interface, holdsRefs.
func refsIn(v reflect.Value) bool {
	kind := v.Kind()
	switch kind {
	case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return false
		}
	}
	if kind == reflect.Interface {
		return holdsRefs(v.Elem().Type())
	}
	return holdsRefs(v.Type())
}

// node adds the node of v to the snapshot and returns its index. inline is
// set for a copy of a key or a value of a map, and for the elements of an
// array in one: they are not places of their own, but parts of the entry of
// their map.
func (w *refWalk) node(v reflect.Value, inline bool) int32 {
	kind := v.Kind()
	// A leaf comes first, as in jsValue, so that a nil value whose type has
	// an Error method converts to the string of Error.
	if kind == reflect.Interface {
		if v.IsNil() {
			return w.add(snapNode{kind: snapNull})
		}
		if x := v.Elem(); !holdsRefs(x.Type()) {
			// What an any holds cannot change: the leaf is the conversion's.
			return w.add(snapNode{kind: snapLeaf, leaf: x})
		}
	} else if !holdsRefs(v.Type()) {
		// A leaf of a type of its own is a result, which the program cannot
		// reach: in a container the walk goes through, every place whose
		// type holds no Value is an any, or a key the walk copies.
		return w.add(snapNode{kind: snapLeaf, leaf: v})
	}
	if (kind == reflect.Pointer || kind == reflect.Map) && v.IsNil() {
		return w.add(snapNode{kind: snapNull})
	}
	var at refPlace
	switch {
	case kind == reflect.Map:
		at = refPlace{p: v.UnsafePointer()}
	case !inline && v.CanAddr():
		at = refPlace{unsafe.Pointer(v.UnsafeAddr()), v.Type()}
	}
	if at.p == nil {
		return w.content(v, kind, inline)
	}
	slot, nd, met := w.enter(at)
	if met {
		return nd
	}
	nd = w.content(v, kind, inline)
	w.leave(at, slot, nd)
	return nd
}

// content adds the node of v, of a type that holdsRefs.
func (w *refWalk) content(v reflect.Value, kind reflect.Kind, inline bool) int32 {
	switch kind {
	case reflect.Interface:
		return w.node(v.Elem(), false)
	case reflect.Struct: // a Value, holdsRefs says
		var x Value
		if v.CanAddr() {
			x = *(*Value)(unsafe.Pointer(v.UnsafeAddr()))
		} else {
			x = v.Interface().(Value)
		}
		switch {
		case x.vm == w.m:
			if hasRefCount(x.v) {
				w.s.held = append(w.s.held, x.v)
			}
		case x.vm != nil:
			w.fail(errForeignValue)
			return w.failed(errForeignValue)
		}
		return w.add(snapNode{kind: snapValue, v: x.jsValue()})
	case reflect.Pointer:
		if v.IsNil() {
			return w.add(snapNode{kind: snapNull})
		}
		elem := v.Elem()
		if elem.Kind() == reflect.Pointer {
			w.node(elem, false)
			return w.failed(fmt.Errorf("type not supported: %v", v.Type()))
		}
		return w.node(elem, false)
	case reflect.Slice:
		if v.Type() == anySliceType {
			return w.anySlice(v)
		}
		n := v.Len()
		start := w.reserve(n)
		for i := range n {
			w.s.kids[start+i].node = w.node(v.Index(i), false)
		}
		return w.add(snapNode{kind: snapArray, start: int32(start), n: int32(n)})
	case reflect.Array:
		// An array is not converted, but the references in it are taken.
		for i := range v.Len() {
			w.node(v.Index(i), inline)
		}
		return w.failed(fmt.Errorf("type not supported: %v", v.Type()))
	case reflect.Map:
		if v.Type() == anyMapType {
			return w.anyMap(v)
		}
		return w.genericMap(v)
	}
	panic(fmt.Sprintf("quickjs: internal error: the walk of a result met %v", v.Type()))
}

// mapKey is how a key of a map of type t becomes the name of a property, or
// nil for a type of key javascript has no name for.
func mapKey(t reflect.Type) func(k reflect.Value) string {
	// Integer keys stringify the same way they do in JSON and the same way
	// obj[1] and obj["1"] are the same property in javascript.
	switch t.Kind() {
	case reflect.String:
		return reflect.Value.String
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(k reflect.Value) string { return strconv.FormatInt(k.Int(), 10) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(k reflect.Value) string { return strconv.FormatUint(k.Uint(), 10) }
	}
	return nil
}

// object adds the node of an object of the n kids from start, which it
// sorts: javascript property order is observable, via Object.keys,
// JSON.stringify or for-in, while Go map iteration order is randomized, and
// so the same map always makes the same object, as encoding/json does.
func (w *refWalk) object(start, n int) int32 {
	slices.SortFunc(w.s.kids[start:start+n], func(a, b snapKid) int { return strings.Compare(a.key, b.key) })
	return w.add(snapNode{kind: snapObject, start: int32(start), n: int32(n)})
}

// genericMap is content of a map, through reflect.
func (w *refWalk) genericMap(v reflect.Value) int32 {
	t := v.Type()
	key := mapKey(t.Key())
	// The key and the value of each entry are copied into one variable each,
	// which does not allocate as MapIter.Key and Value do.
	var k, val reflect.Value
	if key != nil || holdsRefs(t.Key()) {
		k = reflect.New(t.Key()).Elem()
	}
	val = reflect.New(t.Elem()).Elem()
	// The kids are the first ones after start, before those of what the
	// values hold.
	var start, i int
	if key != nil {
		start = w.reserve(v.Len())
	}
	for it := v.MapRange(); it.Next(); {
		if k.IsValid() {
			k.SetIterKey(it)
		}
		val.SetIterValue(it)
		if key == nil {
			// The object is refused, but the references in it are taken.
			if holdsRefs(t.Key()) {
				w.node(k, true)
			}
			w.node(val, true)
			continue
		}
		w.s.kids[start+i] = snapKid{key(k), w.node(val, true)}
		i++
	}
	switch {
	case key != nil:
		return w.object(start, i)
	case v.Len() == 0:
		// No key needs a name, as in jsMap.
		return w.object(len(w.s.kids), 0)
	}
	return w.failed(fmt.Errorf("unsupported map key type: %s", t.Key()))
}

// anySliceType and anyMapType are the types of the most common results that
// hold values of any type, which anySlice and anyMap go through with a range
// of Go, many times faster than one of reflect.
var (
	anySliceType = reflect.TypeFor[[]any]()
	anyMapType   = reflect.TypeFor[map[string]any]()
)

// anySlice is content of v, a []any.
func (w *refWalk) anySlice(v reflect.Value) int32 {
	var s []any
	if v.CanAddr() {
		s = *(*[]any)(unsafe.Pointer(v.UnsafeAddr()))
	} else {
		s = v.Interface().([]any)
	}
	start := w.reserve(len(s))
	for i, x := range s {
		var nd int32
		switch {
		case x == nil:
			nd = w.add(snapNode{kind: snapNull})
		case !holdsRefs(reflect.TypeOf(x)):
			nd = w.add(snapNode{kind: snapLeaf, leaf: reflect.ValueOf(x)})
		default:
			nd = w.node(reflect.ValueOf(&s[i]).Elem(), false)
		}
		w.s.kids[start+i].node = nd
	}
	return w.add(snapNode{kind: snapArray, start: int32(start), n: int32(len(s))})
}

// anyMap is content of v, a map[string]any.
func (w *refWalk) anyMap(v reflect.Value) int32 {
	mp := v.Interface().(map[string]any)
	start, i := w.reserve(len(mp)), 0
	for k, x := range mp {
		var nd int32
		switch {
		case x == nil:
			nd = w.add(snapNode{kind: snapNull})
		case !holdsRefs(reflect.TypeOf(x)):
			nd = w.add(snapNode{kind: snapLeaf, leaf: reflect.ValueOf(x)})
		default:
			nd = w.node(reflect.ValueOf(x), true)
		}
		w.s.kids[start+i] = snapKid{k, nd}
		i++
	}
	return w.object(start, i)
}

// build makes the javascript value of node i of s.
func (m *VM) build(s *snapshot, i int32) (out jsValue, err error) {
	nd := &s.nodes[i]
	switch nd.kind {
	case snapNull:
		return null, nil
	case snapLeaf:
		return m.jsValue(nd.leaf)
	case snapValue:
		return dupValue(nd.v), nil
	case snapArray:
		return m.buildArray(s, nd.start, nd.n)
	case snapObject:
		return m.buildObject(s, nd.start, nd.n)
	}
	return out, nd.err
}

// buildArray makes an array of the n kids of s from start.
func (m *VM) buildArray(s *snapshot, start, n int32) (out jsValue, err error) {
	// The values are made on top of s.vals, and freed when one fails or
	// panics, and once the array has its own references.
	base := len(s.vals)
	defer func() {
		for _, v := range s.vals[base:] {
			m.jsFreeValue(v)
		}
		s.vals = s.vals[:base]
	}()
	for _, kid := range s.kids[start : start+n] {
		jv, err := m.build(s, kid.node)
		if err != nil {
			return out, err
		}
		s.vals = append(s.vals, jv)
	}
	out, err = m.jsArrayOfValues(s.vals[base:])
	if err == nil && isException(out) {
		err = m.errFromException()
	}
	return out, err
}

// buildObject makes an object of the n kids of s from start, in order.
func (m *VM) buildObject(s *snapshot, start, n int32) (out jsValue, err error) {
	obj := m.jsNewObject()
	if isException(obj) {
		return out, m.errFromException()
	}
	// The object is freed when a value fails to convert, or panics.
	done := false
	defer func() {
		if !done {
			m.jsFreeValue(obj)
		}
	}()
	for _, kid := range s.kids[start : start+n] {
		jv, err := m.build(s, kid.node)
		if err != nil {
			return out, err
		}
		// The key is an atom of all its bytes, NULs too, and an own data
		// property as in an object literal: no setter runs, and __proto__ is
		// a key. JS_DefinePropertyValue takes jv, also when it fails.
		atom, err := m.jsNewAtom(validUTF8(kid.key))
		if err == nil && atom == atomNull {
			err = m.errFromException()
		}
		if err != nil {
			m.jsFreeValue(jv)
			return out, err
		}
		rc := m.jsDefinePropertyValue(obj, atom, jv, propCWE|propThrow)
		m.jsFreeAtom(atom)
		if rc < 0 {
			return out, m.errFromException()
		}
	}
	done = true
	return obj, nil
}

// refTypes caches holdsRefs of the types of containers.
var refTypes sync.Map // reflect.Type -> bool

var errorType = reflect.TypeFor[error]()

// holdsRefs reports whether a value of type t can hold a Value, or itself,
// in what the conversion of a result goes through: interfaces, pointers,
// slices, arrays and maps. A struct converts through encoding/json, and a
// type with an Error method through Error, so neither is walked, but for
// Value.
func holdsRefs(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Struct:
		return t == rvt
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
	default:
		return false
	}
	if r, ok := refTypes.Load(t); ok {
		return r.(bool)
	}
	r := holdsRefsOf(t, map[reflect.Type]bool{})
	refTypes.Store(t, r)
	return r
}

// holdsRefsOf is holdsRefs. path has the types the check is inside: a type
// met again there holds itself, and so can make a cycle.
func holdsRefsOf(t reflect.Type, path map[reflect.Type]bool) bool {
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Struct:
		return t == rvt
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
	default:
		return false
	}
	if t.Implements(errorType) {
		return false
	}
	if path[t] {
		return true
	}
	path[t] = true
	defer delete(path, t)
	if t.Kind() == reflect.Map && holdsRefsOf(t.Key(), path) {
		return true
	}
	return holdsRefsOf(t.Elem(), path)
}
