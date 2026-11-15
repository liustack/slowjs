// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package tifa

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/liustack/slowjs"
)

func newVM(t testing.TB) *slowjs.VM {
	t.Helper()
	vm, err := slowjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vm.Close() })
	if err := vm.SetEvalTimeout(time.Minute); err != nil {
		t.Fatal(err)
	}
	return vm
}

func eval(t testing.TB, vm *slowjs.VM, js string) any {
	t.Helper()
	v, err := vm.Eval(js, slowjs.EvalGlobal)
	if err != nil {
		t.Fatalf("%s: %v", js, err)
	}
	return v
}

// kinds is what a VM has under each name of Hooks: "function", "get set" for an object with
// the functions get and set, or the typeof of anything else.
const kinds = `(names) => names.map((n) => {
  const v = globalThis[n]
  return typeof v === 'function' ? 'function' : v && typeof v.get === 'function' && typeof v.set === 'function' ? 'get set' : typeof v
}).join(',')`

func hookKinds(t *testing.T, vm *slowjs.VM) string {
	t.Helper()
	f, err := vm.EvalValue(kinds, slowjs.EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Free()
	r, err := f.Call(slowjs.UndefinedValue, Hooks())
	if err != nil {
		t.Fatal(err)
	}
	return r.(string)
}

// Register gives a VM URL, Intl, Temporal and every hook of Hooks, which a new VM has none of.
func TestRegister(t *testing.T) {
	want := []string{"__tifa_random_bytes__", "__tifa_murmur2__", "__tifa_escape", "__tifa_copy_props",
		"__tifa_start_tag", "__tifa_end_tag", "__tifa_async_context__"}
	if got := Hooks(); !slices.Equal(got, want) {
		t.Fatalf("Hooks() = %q, want tifa's order %q", got, want)
	}
	vm := newVM(t)
	const globals = `[typeof URL, typeof URLSearchParams, typeof Intl, typeof Temporal].join(',')`
	if got := eval(t, vm, globals); got != "undefined,undefined,undefined,undefined" {
		t.Fatalf("a new VM has %s", got)
	}
	if got := hookKinds(t, vm); got != strings.Repeat("undefined,", len(want)-1)+"undefined" {
		t.Fatalf("a new VM has the hooks %s", got)
	}
	if err := Register(vm); err != nil {
		t.Fatal(err)
	}
	if got := eval(t, vm, globals); got != "function,function,object,object" {
		t.Errorf("after Register: %s", got)
	}
	if got := hookKinds(t, vm); got != "function,function,function,function,function,function,get set" {
		t.Errorf("after Register, the hooks are %s", got)
	}
	if got := eval(t, vm, `new URL('/a?b=1', 'https://x.test').href + ' ' + new Intl.NumberFormat('en-US').format(1234.5)`); got != "https://x.test/a?b=1 1,234.5" {
		t.Errorf("URL and Intl: %v", got)
	}
	if err := Register(vm); err == nil {
		t.Error("a second Register on the same VM did not fail")
	}
	// Hooks returns a copy: changing it changes nothing.
	Hooks()[0] = "x"
	if Hooks()[0] != want[0] {
		t.Error("Hooks returned the package's own list")
	}
}

// RegisterHook defines the one hook it names, and refuses a name that is not one.
func TestRegisterHook(t *testing.T) {
	vm := newVM(t)
	if err := RegisterHook(vm, "__tifa_murmur2__"); err != nil {
		t.Fatal(err)
	}
	if got := hookKinds(t, vm); got != "undefined,function,undefined,undefined,undefined,undefined,undefined" {
		t.Errorf("after RegisterHook of __tifa_murmur2__: %s", got)
	}
	if got := eval(t, vm, `typeof Intl`); got != "undefined" {
		t.Errorf("RegisterHook defined Intl: %v", got)
	}
	if err := RegisterHook(vm, "__tifa_emit_check"); err == nil {
		t.Error("RegisterHook of a name not in Hooks did not fail")
	}
}

// The native test host, engine/test/render.c, defines with tifa=1 the hooks of Hooks and no other
// global named __tifa_*: the training of the profile renders the pages with what tifa gives them.
func TestRenderCHasTheHooks(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "engine", "test", "render.c"))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, n := range regexp.MustCompile(`__tifa_[A-Za-z0-9_]*`).FindAllString(string(src), -1) {
		if !slices.Contains(got, n) {
			got = append(got, n)
		}
	}
	want := Hooks()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("engine/test/render.c names %q, Hooks() %q: make the two agree", got, want)
	}
}

// __tifa_murmur2__ gives the hash of @emotion/hash for every string, and leaves anything but a
// string, and a string while String.prototype.charCodeAt or Number.prototype.toString is
// replaced, to javascript. The checks are those of engine/test/render-test/murmur2.js, which
// holds the function of @emotion/hash.
func TestMurmur2IsTheHashOfEmotion(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "engine", "test", "render-test", "murmur2.js"))
	if err != nil {
		t.Fatal(err)
	}
	vm := newVM(t)
	if err := RegisterHook(vm, "__tifa_murmur2__"); err != nil {
		t.Fatal(err)
	}
	eval(t, vm, string(src))
	if got := eval(t, vm, `sameHashes()`); got != "2011 0" {
		t.Errorf("strings and those hashed otherwise than @emotion/hash does: %v", got)
	}
	const want = `[null,null,null,null,null,null,null,null,true,null,null,true,false]`
	if got := eval(t, vm, `restToJavaScript()`); got != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// BenchmarkMurmur2 hashes a style of the length MUI serializes, in Go.
func BenchmarkMurmur2(b *testing.B) {
	style := strings.Repeat("display:flex;align-items:center;padding:8px 16px;", 6)
	for b.Loop() {
		murmur2(style)
	}
}

// __tifa_random_bytes__(n) gives n new bytes in base64 for n from 0 to 65536, and throws for
// anything else.
func TestRandomBytes(t *testing.T) {
	vm := newVM(t)
	if err := RegisterHook(vm, "__tifa_random_bytes__"); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 1, 16, 65536} {
		s, ok := eval(t, vm, `__tifa_random_bytes__(`+strconv.Itoa(n)+`)`).(string)
		if !ok {
			t.Fatalf("n = %d: not a string", n)
		}
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil || len(b) != n {
			t.Errorf("n = %d: %d bytes, %v", n, len(b), err)
		}
	}
	if eval(t, vm, `__tifa_random_bytes__(32) === __tifa_random_bytes__(32)`) != false {
		t.Error("two calls gave the same 32 bytes")
	}
	for _, arg := range []string{"", "-1", "1.5", "65537", "'8'", "8, 8", "NaN"} {
		got := eval(t, vm, `(() => { try { __tifa_random_bytes__(`+arg+`); return 'no error' } catch (e) { return e instanceof TypeError } })()`)
		if got != true {
			t.Errorf("__tifa_random_bytes__(%s): %v, want a TypeError", arg, got)
		}
	}
}

// __tifa_async_context__ reads and sets the async context of the VM, an object, which a promise job keeps
// from where it was queued, and a script can delete it, as tifa's host script does.
func TestAsyncContext(t *testing.T) {
	vm := newVM(t)
	if err := RegisterHook(vm, "__tifa_async_context__"); err != nil {
		t.Fatal(err)
	}
	eval(t, vm, `
const c = __tifa_async_context__
globalThis.seen = []
c.set({ name: 'outer' })
Promise.resolve().then(() => seen.push(c.get().name))
c.set({ name: 'inner' })
seen.push(c.get().name)
globalThis.deleted = Reflect.deleteProperty(globalThis, '__tifa_async_context__') && !('__tifa_async_context__' in globalThis)
`)
	if _, err := vm.ExecutePendingJobs(); err != nil {
		t.Fatal(err)
	}
	if got := eval(t, vm, `JSON.stringify(seen) + ' ' + deleted`); got != `["inner","outer"] true` {
		t.Errorf("got %v", got)
	}
}

// The functions of the engine that Register defines do the work of React's: one call of each, as
// tifa's tests make them.
func TestEngineHooks(t *testing.T) {
	vm := newVM(t)
	if err := Register(vm); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ js, want string }{
		{`__tifa_escape('a & b < c > d " e \' f')`, `a &amp; b &lt; c &gt; d &quot; e &#x27; f`},
		{`(() => { const props = {}; const r = __tifa_copy_props({ a: 1, key: 'k', b: [2] }, props, Object.prototype.hasOwnProperty, false); return r + ' ' + JSON.stringify(props) })()`, `true {"a":1,"b":[2]}`},
		{`(() => { const target = [], decline = {}
  return JSON.stringify([
    __tifa_start_tag(target, { className: 'a', children: 'x<' }, 'div', { viewTransition: null }, Object.prototype.hasOwnProperty, decline),
    __tifa_start_tag(target, { dangerouslySetInnerHTML: { __html: '' } }, 'div', { viewTransition: null }, Object.prototype.hasOwnProperty, decline) === decline,
    target, __tifa_end_tag('div')]) })()`, `[null,true,["<div class=\"a\">x&lt;"],"</div>"]`},
	}
	for _, c := range cases {
		if got := eval(t, vm, c.js); got != c.want {
			t.Errorf("%s = %v, want %s", c.js, got, c.want)
		}
	}
}
