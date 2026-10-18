// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// The 46 cases of testdata/objlit.js print what each definition leaves:
// keys in order, values and attributes, or the error. testdata/objlit.txt is
// the transcript of the engine before the fast path of objlit.go, so a
// change to how a new property is defined must leave every line as it was.
func TestObjlitSemantics(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("testdata", "objlit.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "objlit.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := eval(t, "objlit.js", string(js))
	if got == strings.TrimSuffix(string(want), "\n") {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			t.Errorf("line %d:\n got %s\nwant %s", i+1, g, w)
		}
	}
}

// A new data property of an ordinary object skips the generic path of
// JS_DefinePropertyValue, JS_DefineProperty and JS_CreateProperty. Their
// TLS frames are how the test tells: the fast path writes nothing to the
// TLS stack in these cases, the generic path always does. The cases that
// the fast path leaves to the generic path show that the test sees it.
func TestObjlitFastPath(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	name, err := libc.CString("objlitProp")
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(tls, name)
	prop := XJS_NewAtom(tls, ctx, name)
	defer XJS_FreeAtom(tls, ctx, prop)

	for _, c := range []struct {
		name, js string
		fast     bool
		want     int32
	}{
		// The literal shares the shape of o2, and keep has the shape to
		// move to, so neither path allocates.
		{"shared shape", "var keep = {a: 1, objlitProp: 2}, o2 = {a: 1}; ({a: 1})", true, 1},
		{"own shape", "({x: 1, y: 2})", true, 1},
		{"instance", "class A { f = 1 }; new A()", true, 1},
		{"existing property", "({objlitProp: 1})", false, 1},
		{"array", "[1, 2]", false, 1},
		{"not extensible", "Object.preventExtensions({})", false, 0},
		{"proxy", "new Proxy({}, {})", false, 1},
		{"global object", "globalThis", false, 1},
	} {
		obj := eval0(t, tls, ctx, c.js)
		if tag(obj) != EJS_TAG_OBJECT {
			t.Fatalf("%s: %s is no object: %s", c.name, c.js, toString(t, tls, ctx, obj))
		}
		val := eval0(t, tls, ctx, "42")
		var r int32
		touched := touchesTLS(t, tls, func() {
			r = XJS_DefinePropertyValue(tls, ctx, obj, prop, val, MJS_PROP_C_W_E)
		})
		if touched == c.fast {
			t.Errorf("%s: fast path taken = %v, want %v", c.name, !touched, c.fast)
		}
		if r != c.want {
			t.Errorf("%s: JS_DefinePropertyValue = %d, want %d", c.name, r, c.want)
		}
		if c.want == 1 {
			if got := XJS_GetPropertyInternal(tls, ctx, obj, prop, obj, 0); tag(got) != EJS_TAG_INT || jsvToInt32(got) != 42 {
				t.Errorf("%s: the property reads %s", c.name, toString(t, tls, ctx, got))
			}
		}
		XFreeValue(tls, ctx, obj)
	}
}

// touchesTLS reports whether f writes to the TLS stack above its top.
func touchesTLS(t *testing.T, tls *libc.TLS, f func()) bool {
	const n, fill = 1024, 0xa5
	p := tls.Alloc(n)
	b := unsafe.Slice((*byte)(unsafe.Pointer(p)), n)
	for i := range b {
		b[i] = fill
	}
	tls.Free(n)
	f()
	if q := tls.Alloc(n); q != p {
		t.Fatalf("the TLS stack top moved from %#x to %#x", p, q)
	}
	defer tls.Free(n)
	for _, c := range b {
		if c != fill {
			return true
		}
	}
	return false
}

// objlitDefine leaves every case but a new data property of an ordinary
// extensible object to the generic path: it does nothing and keeps its
// hands off the reference of the value.
func TestObjlitDefineRefuses(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ctx := XJS_NewContext(tls, rt)
	defer XJS_FreeContext(tls, ctx)

	name, err := libc.CString("objlitProp")
	if err != nil {
		t.Fatal(err)
	}
	defer libc.Xfree(tls, name)
	prop := XJS_NewAtom(tls, ctx, name)
	defer XJS_FreeAtom(tls, ctx, prop)

	val := eval0(t, tls, ctx, "({})")
	defer XFreeValue(tls, ctx, val)
	rc := func() int32 { return (*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, jsvPtr(val)))).Fref_count }
	before := rc()
	for _, c := range []struct {
		name, js string
		flags    int32
	}{
		{"existing property", "({objlitProp: 1})", MJS_PROP_C_W_E},
		{"getter", "({})", MJS_PROP_C_W_E | MJS_PROP_HAS_GET},
		{"setter", "({})", MJS_PROP_C_W_E | MJS_PROP_HAS_SET},
		{"not extensible", "Object.preventExtensions({})", MJS_PROP_C_W_E},
		{"frozen", "Object.freeze({})", MJS_PROP_C_W_E},
		{"fast array", "[1, 2]", MJS_PROP_C_W_E},
		{"array", "var a = [1, 2]; a.x = 1; delete a[0]; a", MJS_PROP_C_W_E},
		{"proxy", "new Proxy({}, {})", MJS_PROP_C_W_E},
		{"string object", "new String('ab')", MJS_PROP_C_W_E},
		{"typed array", "new Uint8Array(2)", MJS_PROP_C_W_E},
		{"arguments", "(function () { return arguments })(1)", MJS_PROP_C_W_E},
		{"global object", "globalThis", MJS_PROP_C_W_E},
		{"number", "1", MJS_PROP_C_W_E},
		{"string", "'s'", MJS_PROP_C_W_E},
	} {
		obj := eval0(t, tls, ctx, c.js)
		if r, ok := objlitDefine(tls, ctx, obj, prop, val, c.flags); ok {
			t.Errorf("%s: objlitDefine = %d, %v, want the generic path", c.name, r, ok)
		}
		if got := rc(); got != before {
			t.Errorf("%s: the value has %d references, want %d", c.name, got, before)
		}
		XFreeValue(tls, ctx, obj)
	}

	// A new property takes the reference it is given and the attributes
	// of flags.
	obj := eval0(t, tls, ctx, "globalThis.objlitObj = {a: 1}")
	defer XFreeValue(tls, ctx, obj)
	if r, ok := objlitDefine(tls, ctx, obj, prop, XDupValue(tls, ctx, val), MJS_PROP_WRITABLE|MJS_PROP_HAS_VALUE|MJS_PROP_HAS_ENUMERABLE); r != 1 || !ok {
		t.Fatalf("a new property: objlitDefine = %d, %v, want 1, true", r, ok)
	}
	if got := rc(); got != before+1 {
		t.Errorf("the value has %d references, want %d", got, before+1)
	}
	const want = `a,objlitProp {"value":{},"writable":true,"enumerable":false,"configurable":false}`
	if got := eval1(t, tls, ctx, "Reflect.ownKeys(objlitObj).join() + ' ' + JSON.stringify(Object.getOwnPropertyDescriptor(objlitObj, 'objlitProp'))"); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
