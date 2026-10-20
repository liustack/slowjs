// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// inlineInput returns the generated file name of lib/ as fixInline gets it:
// with every change before it made, and the defers as ccgo writes them.
func inlineInput(t *testing.T, name string) []byte {
	t.Helper()
	b, hw := lazyInput(t, name)
	b, err := fixLazyFrames(b, hw, name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// editFunc replaces old, which must occur once in the function name of b,
// with new.
func editFunc(t *testing.T, b []byte, name, old, new string) []byte {
	t.Helper()
	body, err := funcBody(b, funcHead(name), name)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(body, []byte(old)); n != 1 {
		t.Fatalf("%s: %q found %d times", name, old, n)
	}
	fixed := bytes.Replace(body, []byte(old), []byte(new), 1)
	return bytes.Replace(bytes.Clone(b), body, fixed, 1)
}

// Every generated file has the calls of inlineCallees in JS_CallInternal
// written out, readEngine gives them back as calls, and fixInline writes
// them out again.
func TestInlineFixedFile(t *testing.T) {
	want := 0
	for _, n := range inlineCallees {
		want += n
	}
	for _, fn := range libFiles(t) {
		name := filepath.Base(fn)
		raw, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		if n := bytes.Count(raw, []byte("written out (see fixInline)")); n != want {
			t.Errorf("%s: %d calls written out, want %d", name, n, want)
		}
		b := inlineInput(t, name)
		if bytes.Contains(b, []byte("fixInline")) {
			t.Fatalf("%s: readEngine leaves a change of fixInline", name)
		}
		out, err := fixInline(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		// fixLast and fixOneFrame come between it and fixNoDefer.
		if out, err = fixLast(out, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out, err = fixOneFrame(out, name); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out, err = dropDefers(out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(out, raw) {
			t.Errorf("%s: fixInline does not make the file again", name)
		}
		back, err := restoreDefer(out)
		if err == nil {
			back, err = restoreOneFrame(back, name)
		}
		if err == nil {
			back, err = restoreLast(back)
		}
		if err == nil {
			back, err = restoreInline(back)
		}
		if err != nil || !bytes.Equal(back, b) {
			t.Errorf("%s: restoreInline does not give the input back (%v)", name, err)
		}
	}
}

// What a call becomes: a block that evaluates the arguments in order, then
// runs the callee's body.
func TestInlineShapes(t *testing.T) {
	b := inlineInput(t, lazyPinFile)
	out, err := inlineCalls(bytes.Clone(b), inlineCallees)
	if err != nil {
		t.Fatal(err)
	}
	body, err := funcBody(out, funcHead(inlineFunc), inlineFunc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\t\t{\n\t\t\t// Changed by internal/fixlib: _set_value written out (see fixInline), with ctx10 as inl1.\n\t\t\tvar inl2 uintptr = var_buf+uintptr(idx3)*16\n\t\t\tvar inl3 TJSValue = *(*TJSValue)(unsafe.Pointer(sp + uintptr(-libc.Int32FromInt32(1))*16))\n\t\t\tvar cell TJSValue\n\t\t\tvar old_val, v1 TJSValue\n",
		"\t\t\told_val = *(*TJSValue)(unsafe.Pointer(inl2))\n\t\t\t*(*TJSValue)(unsafe.Pointer(inl2)) = inl3\n",
		"\t\t\tcell = v1\n",
		"\t\t\t\tv2 = uintptr(cell.Fu.Fuint641) - libc.UintptrFromInt32(1)*4\n",
		"\t\t\t\tx__JS_FreeValue(tls, ctx10, v1)\n",
		"// Changed by internal/fixlib: _set_value written out (see fixInline), with ctx10 as inl1, var_buf as inl2, v121 as inl3.\n\t\t\tvar cell TJSValue\n",
		"\t\t\t*(*TJSValue)(unsafe.Pointer(var_buf)) = v121\n",
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Errorf("no %q", want)
		}
	}
	// The labels of each copy are its own, and the marks of other changes
	// stay with the callee.
	if n := len(regexp.MustCompile(`\tgoto (inl\d+__3)\n\t+(inl\d+__3):\n`).FindAll(body, -1)); n != inlineCallees["_set_value"] {
		t.Errorf("%d labels of set_value renamed", n)
	}
	if bytes.Contains(body, []byte(framesInGo)) {
		t.Error("the mark of fixFrames copied")
	}
	// A variable given as an argument is read where the parameter is.
	if bytes.Contains(body, []byte("var inl1 ")) {
		t.Error("a variable given to a parameter")
	}
	// The frame of set_value, one JSValue, is a Go variable in each copy.
	if bytes.Contains(body, []byte("localbp")) {
		t.Error("the frame of set_value copied")
	}
	for name := range inlineCallees {
		if n, m := bytes.Count(body, []byte(name+"(tls")), bytes.Count(body, []byte(": "+name+"(tls")); n != m {
			t.Errorf("%d calls of %s left", n-m, name)
		}
	}
}

// fixInline stops where it cannot write out a call as it is.
func TestInlineRejects(t *testing.T) {
	b := inlineInput(t, lazyPinFile)
	for _, c := range []struct {
		name, fn, old, new, want string
	}{{
		name: "a return in a callee without a result",
		fn:   "_set_value",
		old:  "\tv1 = old_val\n",
		new:  "\tv1 = old_val\n\tif pval == 0 {\n\t\treturn\n\t}\n",
		want: "_set_value: a return",
	}, {
		name: "another number of calls",
		fn:   inlineFunc,
		old:  "\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		new:  "\t\t_set_value(tls, ctx10, var_buf, v121)\n\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		want: "47 calls of _set_value, inlineCallees has 46",
	}, {
		name: "a call written another way",
		fn:   inlineFunc,
		old:  "\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		new:  "\t\t_set_value(tls, ctx10, var_buf,v121)\n",
		want: "not written as _set_value(tls, ctx10, var_buf, v121)",
	}, {
		name: "a call that is not a statement",
		fn:   inlineFunc,
		old:  "\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		new:  "\t\tfor _set_value(tls, ctx10, var_buf, v121); false; {\n\t\t}\n",
		want: "_set_value called in a *ast.ForStmt",
	}, {
		name: "a name of the callee taken by a local",
		fn:   inlineFunc,
		old:  "\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		new:  "\t\tvar x__JS_FreeValue int\n\t\t_ = x__JS_FreeValue\n\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		want: "x__JS_FreeValue, which _set_value uses, is a local of _JS_CallInternal",
	}, {
		name: "a local named like the copies",
		fn:   inlineFunc,
		old:  "\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		new:  "\t\tvar inl1 int\n\t\t_ = inl1\n\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		want: "_JS_CallInternal already has a name inl1",
	}, {
		name: "a callee with a result",
		fn:   "_set_value",
		old:  "new_val TJSValue) {",
		new:  "new_val TJSValue) (r int32) {",
		want: "_set_value: a result",
	}, {
		name: "a frame used another way",
		fn:   "_set_value",
		old:  "\t\tv2 = *(*uintptr)(unsafe.Pointer(&localbp))",
		new:  "\t\tv2 = uintptr(unsafe.Pointer(&localbp))",
		want: "_set_value: localbp used other than as one JSValue",
	}, {
		name: "a frame of another size",
		fn:   "_set_value",
		old:  "\tvar localbp [2]uint64\n",
		new:  "\tvar localbp [3]uint64\n",
		want: "_set_value: localbp used other than as one JSValue",
	}, {
		name: "a frame of a value written another way",
		fn:   "_set_value",
		old:  "\t*(*TJSValue)(unsafe.Pointer(&localbp)) = v1\n",
		new:  "\t*(*uintptr)(unsafe.Pointer(&localbp)) = 0\n",
		want: "_set_value: localbp used other than as one JSValue",
	}, {
		name: "a local named like the cell",
		fn:   "_set_value",
		old:  "\tvar old_val, v1 TJSValue\n",
		new:  "\tvar old_val, v1, cell TJSValue\n",
		want: "_set_value: a local named cell",
	}} {
		t.Run(c.name, func(t *testing.T) {
			m := editFunc(t, b, c.fn, c.old, c.new)
			_, err := inlineCalls(m, inlineCallees)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want %q", err, c.want)
			}
		})
	}
}

// A variable given as an argument is read in place of the parameter only
// when the callee does not set the parameter or take its address, has no
// local of that name, and the variable is declared once, with the type of
// the parameter, and its address is not taken (or given to _ only).
func TestInlineArgs(t *testing.T) {
	b := inlineInput(t, lazyPinFile)
	for _, c := range []struct {
		name, fn, old, new, want string
	}{{
		name: "a parameter set by the callee",
		fn:   "_set_value",
		old:  "\tv1 = old_val\n",
		new:  "\tv1 = old_val\n\tctx1 += 0\n",
		want: "\t\t\tinl1 += 0\n",
	}, {
		name: "a parameter whose address the callee takes",
		fn:   "_set_value",
		old:  "\tv1 = old_val\n",
		new:  "\tv1 = old_val\n\t_ = &new_val\n",
		want: "\t\t\t_ = &inl3\n",
	}, {
		name: "a local of the callee named like the variable",
		fn:   "_set_value",
		old:  "\tvar old_val, v1 TJSValue\n",
		new:  "\tvar old_val, v1, v121 TJSValue\n",
		want: "\t\tvar inl3 TJSValue = v121\n",
	}, {
		name: "a variable whose address goes nowhere",
		fn:   inlineFunc,
		old:  "\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		new:  "\t\t_, _ = &ctx10, &var_buf\n\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		want: "with ctx10 as inl1, var_buf as inl2, v121 as inl3.\n\t\t\tvar cell TJSValue\n",
	}, {
		name: "a variable whose address is taken",
		fn:   inlineFunc,
		old:  "\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		new:  "\t\t_ = uintptr(unsafe.Pointer(&var_buf))\n\t\t_set_value(tls, ctx10, var_buf, v121)\n",
		want: "with ctx10 as inl1, v121 as inl3.\n\t\t\tvar inl2 uintptr = var_buf\n",
	}} {
		t.Run(c.name, func(t *testing.T) {
			m := editFunc(t, b, c.fn, c.old, c.new)
			out, err := inlineCalls(m, inlineCallees)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(out, []byte(c.want)) {
				t.Fatalf("no %q", c.want)
			}
			back, err := restoreInline(out)
			if err != nil || !bytes.Equal(back, m) {
				t.Fatalf("restoreInline does not give the input back (%v)", err)
			}
		})
	}
}

// The frame of set_value becomes a Go JSValue only where a JSValue is
// 8 bytes of a union then a tag.
func TestInlineFrameLayout(t *testing.T) {
	b := inlineInput(t, lazyPinFile)
	m := bytes.Replace(b, []byte("\tFuint641       Tuint64_t\n"), []byte("\tFuint641       Tuint32_t\n"), 1)
	if bytes.Equal(m, b) {
		t.Fatal("no union to change")
	}
	if _, err := inlineCalls(m, inlineCallees); err == nil || !strings.Contains(err.Error(), "JSValue is not a union of 8 bytes and a tag of 8 bytes") {
		t.Fatalf("got %v", err)
	}
}

// A change to a callee stops fixInline until inlineSites is updated after
// a review.
func TestInlineSites(t *testing.T) {
	b := inlineInput(t, lazyPinFile)
	if _, err := fixInline(bytes.Clone(b), lazyPinFile); err != nil {
		t.Fatal(err)
	}
	m := editFunc(t, b, "_set_value", "\tv1 = old_val\n", "\tv1 = old_val\n\t_ = 0\n")
	if _, err := fixInline(m, lazyPinFile); err == nil || !strings.Contains(err.Error(), "update inlineSites") {
		t.Fatalf("got %v", err)
	}
}

// restoreInline stops at a change that is not in the form fixInline writes.
func TestRestoreInlineRejects(t *testing.T) {
	head := "package p\n\nfunc _JS_CallInternal(tls *libc.TLS, ctx10 uintptr) (r int32) {\n"
	for _, c := range []struct {
		name, body, want string
	}{
		{"a block without its arguments", "\t{\n\t\t" + inlineMark("_set_value") + "\t\tvar old_val TJSValue\n\t}\n\treturn 0\n", "a block of _set_value without its arguments"},
		{"an argument out of order", "\t{\n\t\t" + inlineMark("_set_value") + "\t\tvar inl2 uintptr = ctx10\n\t}\n\treturn 0\n", "a block of _set_value without its arguments"},
		{"an argument missing", "\t{\n\t\t" + inlineMark("_set_value", "ctx10 as inl3") + "\t\tvar inl1 uintptr = ctx10\n\t}\n\treturn 0\n", "a block of _set_value without its argument inl2"},
	} {
		_, err := restoreInline([]byte(head + c.body + "}\n"))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}
