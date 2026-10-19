// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lazyInput returns the generated file name of lib/ as fixLazyFrames gets
// it: with every other change made, the defers as ccgo writes them and the
// change of fixLazyFrames undone.
func lazyInput(t *testing.T, name string) ([]byte, []*ast.File) {
	t.Helper()
	hw, err := handWritten(filepath.Join("..", "..", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := readEngine(filepath.Join("..", "..", "lib", name))
	if err != nil {
		t.Fatal(err)
	}
	return b, hw
}

// Every generated file has the functions of lazySites changed, readEngine
// gives them back as ccgo wrote them, and fixLazyFrames makes them again.
func TestLazyFramesFixedFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		name := filepath.Base(fn)
		raw, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		// _JS_ToObject and js_array_finalizer have no TLS frame on 32-bit
		// targets.
		want := len(lazySites)
		if strings.HasSuffix(name, "_386.go") || strings.HasSuffix(name, "_arm.go") {
			want -= 2
		}
		if n := bytes.Count(raw, []byte("each statement that needs a TLS frame takes one of its own")); n != want {
			t.Errorf("%s: %d functions changed by fixLazyFrames, want %d", name, n, want)
		}
		b, hw := lazyInput(t, name)
		if bytes.Contains(b, []byte("fixLazyFrames")) || bytes.Contains(b, []byte("_lazy(")) {
			t.Fatalf("%s: readEngine leaves a change of fixLazyFrames", name)
		}
		out, err := fixLazyFrames(bytes.Clone(b), hw, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out, err = dropDefers(out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(out, raw) {
			t.Errorf("%s: fixLazyFrames does not make the file again", name)
		}
		back, err := restoreLazy(out)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if back, err = restoreDefer(back); err != nil || !bytes.Equal(back, b) {
			t.Errorf("%s: restoreLazy does not give the input back (%v)", name, err)
		}
	}
}

// The functions of lazySites have the counts lazySites has, every TLS frame
// is freed in the list it is taken in, and the copies of outParams follow
// their originals.
func TestLazyFramesCounts(t *testing.T) {
	b, hw := lazyInput(t, lazyPinFile)
	out, err := fixLazyFrames(bytes.Clone(b), hw, lazyPinFile)
	if err != nil {
		t.Fatal(err)
	}
	for name, site := range lazySites {
		body, err := funcBody(out, funcHead(name), name)
		if err != nil {
			t.Fatal(err)
		}
		if n, ok := lazyHead(body[bytes.IndexByte(body, '{'):]); !ok || n != site.frame {
			t.Errorf("%s: head mark of %d bytes (%v), want %d", name, n, ok, site.frame)
		}
		allocs := bytes.Count(body, []byte("tls.Alloc("))
		frees := bytes.Count(body, []byte("tls.Free("))
		if allocs != site.wraps || frees != site.wraps {
			t.Errorf("%s: %d allocs, %d frees, want %d", name, allocs, frees, site.wraps)
		}
		if bytes.Contains(body, []byte("defer ")) {
			t.Errorf("%s: a defer left", name)
		}
	}
	for orig, op := range outParams {
		i := bytes.Index(out, []byte("\nfunc "+orig+"("))
		j := bytes.Index(out, []byte("\nfunc "+op.clone+"("))
		if i < 0 || j < i {
			t.Errorf("%s does not follow %s", op.clone, orig)
		}
	}
}

// fixLazyFrames stops at a use of the frame it cannot show is safe.
func TestLazyFramesRejects(t *testing.T) {
	b, hw := lazyInput(t, lazyPinFile)
	for _, c := range []struct {
		name, site, old, new, want string
	}{{
		name: "address to an unknown callee",
		site: "_JS_GetOwnPropertyInternal",
		old:  "retry:\n\t;\n\tprs = _find_own_property(tls, bp+16, p1, prop)\n",
		new:  "retry:\n\t;\n\tprs = _find_own_property1(tls, bp+16, p1, prop)\n",
		want: "the frame address passed to _find_own_property1",
	}, {
		name: "stack check in a loop condition",
		site: "_js_eq_slow",
		old:  "\t\t\t\t\t\t*(*TJSValue)(unsafe.Pointer(bp + 16)) = _JS_ToNumericFree(tls, ctx3, *(*TJSValue)(unsafe.Pointer(bp + 16)))\n",
		new:  "\t\t\t\t\t\tfor _JS_ToNumericFree(tls, ctx3, *(*TJSValue)(unsafe.Pointer(bp + 16))).Ftag != 0 {\n\t\t\t\t\t\t}\n",
		want: "in a *ast.ForStmt",
	}, {
		name: "address kept in a local",
		site: "_JS_ToStringInternal",
		old:  "\t\tlen1 = Xi32toa(tls, bp+48, ",
		new:  "\t\tp1 = bp + 48\n\t\tlen1 = Xi32toa(tls, p1, ",
		want: "the frame address in p1 = bp + 48",
	}, {
		name: "va_list to an unknown callee",
		site: "XJS_GetPropertyInternal",
		old:  "\t\t\treturn ___JS_ThrowTypeErrorAtom(tls, ctx3, prop, __ccgo_ts+1560, libc.VaList(bp+128, __ccgo_ts+346))\n",
		new:  "\t\t\treturn XJS_ThrowSyntaxError(tls, ctx3, __ccgo_ts+1560, libc.VaList(bp+128, __ccgo_ts+346))\n",
		want: "the result of libc.VaList passed to XJS_ThrowSyntaxError",
	}, {
		name: "stack check in an else if",
		site: "_JS_ToObject",
		old:  "\ttag = int32(val1.Ftag)\n\tswitch tag {\n",
		new:  "\ttag = int32(val1.Ftag)\n\tif tag == 0 {\n\t} else if XJS_ToString(tls, ctx3, val1).Ftag == 0 {\n\t}\n\tswitch tag {\n",
		want: "in the condition of an else if",
	}, {
		name: "another frame size",
		site: "_JS_NewAtomStr",
		old:  "func _JS_NewAtomStr(tls *libc.TLS, ctx uintptr, p uintptr) (r TJSAtom) {\n\tbp := tls.Alloc(16)\n\tdefer tls.Free(16)\n",
		new:  "func _JS_NewAtomStr(tls *libc.TLS, ctx uintptr, p uintptr) (r TJSAtom) {\n\tbp := tls.Alloc(32)\n\tdefer tls.Free(32)\n",
		want: "a frame of 32 bytes",
	}} {
		t.Run(c.name, func(t *testing.T) {
			if bytes.Count(b, []byte(c.old)) != 1 {
				t.Fatalf("%q not found once", c.old)
			}
			m := bytes.Replace(bytes.Clone(b), []byte(c.old), []byte(c.new), 1)
			_, err := lazyFrames(m, hw, map[string]lazySite{c.site: lazySites[c.site]}, true)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
}

// outClone takes only an out parameter that is loaded or stored through.
func TestLazyOutCloneRejects(t *testing.T) {
	b, _ := lazyInput(t, lazyPinFile)
	body, err := funcBody(b, funcHead("_is_num_string"), "_is_num_string")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outClone("_is_num_string", body); err != nil {
		t.Fatal(err)
	}
	m := bytes.Replace(bytes.Clone(body), []byte("\t\t*(*Tuint32_t)(unsafe.Pointer(pval)) = n\n"), []byte("\t\t*(*Tuint32_t)(unsafe.Pointer(pval)) = n\n\t\tglobal = pval\n"), 1)
	if bytes.Equal(m, body) {
		t.Fatal("store not found")
	}
	if _, err := outClone("_is_num_string", m); err == nil || !strings.Contains(err.Error(), "pval used in") {
		t.Fatalf("got %v", err)
	}
}

// restoreLazy stops at a statement fixLazyFrames marked that is not as it
// writes it.
func TestRestoreLazyRejects(t *testing.T) {
	head := "package p\n\nfunc f(tls *libc.TLS, x int32) (r int32) {\n\t" + lazyHeadMark(16) + "\tvar localbp [2]uint64\n\tvar bp uintptr\n"
	for _, c := range []struct {
		name, body, want string
	}{
		{"no free", "\t" + lazyStmtMark + "\ttls.Alloc(16)\n\tx = g(tls)\n\treturn x\n", "f: a statement marked by fixLazyFrames without its tls.Free(16)"},
		{"no alloc", "\t" + lazyStmtMark + "\tx = g(tls, bp)\n\ttls.Free(16)\n\treturn x\n", "f: a statement marked by fixLazyFrames without its tls.Alloc(16)"},
		{"return without its value", "\t" + lazyReturnMark + "\ttls.Alloc(16)\n\tg(tls)\n\ttls.Free(16)\n\treturn\n", "f: a return marked by fixLazyFrames without r = X"},
		{"if without lazyc", "\t" + lazyIfMark + "\ttls.Alloc(16)\n\tc := g(tls) != 0\n\ttls.Free(16)\n\tif c {\n\t}\n\treturn x\n", "f: an if marked by fixLazyFrames without lazyc = X"},
	} {
		_, err := restoreLazy([]byte(head + c.body + "}\n"))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

// A change to a callee that gets an address in the frame, or a new class
// with a get_own_property method, stops fixLazyFrames until lazyAddrSites
// is updated after a review.
func TestLazyAddrSites(t *testing.T) {
	b, hw := lazyInput(t, lazyPinFile)
	if _, err := fixLazyFrames(bytes.Clone(b), hw, lazyPinFile); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, old, new string }{{
		name: "a callee changed",
		old:  "\tlen1 = libc.Uint64FromInt64(int64(bp+uintptr(10)) - int64(q))\n",
		new:  "\tlen1 = libc.Uint64FromInt64(int64(bp+uintptr(10)) - int64(q))\n\tsaved = buf\n",
	}, {
		name: "the va_list passed on",
		old:  "\treturn _JS_ThrowError2(tls, ctx, error_num, fmt, ap, add_backtrace)\n",
		new:  "\tsaved = ap\n\treturn _JS_ThrowError2(tls, ctx, error_num, fmt, ap, add_backtrace)\n",
	}, {
		name: "a new get_own_property",
		old:  "\tp := unsafe.Pointer(&_js_arguments_exotic_methods)\n",
		new:  "\tp := unsafe.Pointer(&_js_arguments_exotic_methods)\n\t*(*uintptr)(unsafe.Add(p, 0)) = __ccgo_fp(_js_string_get_own_property)\n",
	}} {
		if bytes.Count(b, []byte(c.old)) != 1 {
			t.Fatalf("%s: %q not found once", c.name, c.old)
		}
		m := bytes.Replace(bytes.Clone(b), []byte(c.old), []byte(c.new), 1)
		_, err := fixLazyFrames(m, hw, lazyPinFile)
		if err == nil || !strings.Contains(err.Error(), "update lazyAddrSites") {
			t.Errorf("%s: got %v", c.name, err)
		}
	}
}
