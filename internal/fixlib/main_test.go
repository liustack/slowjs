// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func engineFile(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "lib", "ccgo_darwin_arm64.go"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Every inline property read of the interpreter gets a guard, whatever the
// name of its variables, and the result does not depend on the guards
// being there already.
func TestInterpreterGuardsEveryRead(t *testing.T) {
	b := engineFile(t)
	out, err := fixInterpreter(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, b) {
		t.Fatal("fixInterpreter changed a fixed file")
	}
	body, err := funcBody(out, reCallInternal, "_JS_CallInternal")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(reInterpGuard.FindAll(body, -1)); n != 3 {
		t.Fatalf("%d guards, want 3 (get_field, get_field2, get_length)", n)
	}
	for _, name := range []string{"get_field", "get_field2", "get_length"} {
		if !bytes.Contains(body, []byte(") {\n\t\t\t\t\tgoto "+name+"_slow_path\n\t\t\t\t}\n\t\t\t\t_ = ctx")) {
			t.Errorf("no guard in the %s read", name)
		}
	}
}

// A new property lookup in the interpreter that is neither a known inline
// read nor the put_field write stops fixlib.
func TestInterpreterRejectsUnknownLookup(t *testing.T) {
	b := engineFile(t)
	body, err := funcBody(b, reCallInternal, "_JS_CallInternal")
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(body, []byte("\n\tfor {"))
	if i < 0 {
		t.Fatal("no loop in _JS_CallInternal")
	}
	changed := append(append(append([]byte{}, body[:i]...), "\n\t_ = _find_own_property(tls, 0, 0, 0)"...), body[i:]...)
	_, err = fixInterpreter(bytes.Replace(b, body, changed, 1))
	if err == nil || !strings.Contains(err.Error(), "review the new one") {
		t.Fatalf("got %v, want a request to review the new lookup", err)
	}
}

// mutate inserts text before the first occurrence of at inside the
// function whose head starts with head.
func mutate(t *testing.T, b []byte, head, at, text string) []byte {
	t.Helper()
	i := bytes.Index(b, []byte("\n"+head))
	if i < 0 {
		t.Fatalf("no function %s", head)
	}
	j := bytes.Index(b[i:], []byte(at))
	if j < 0 {
		t.Fatalf("no %q in %s", at, head)
	}
	j += i
	return append(append(append([]byte{}, b[:j]...), text...), b[j:]...)
}

// The settle in JS_GetPropertyInternal, and the slot it gets in the
// darwin/arm64 file.
const (
	getPropHead   = "func XJS_GetPropertyInternal("
	getPropSettle = "\t\tif prop == uint32(EJS_ATOM_stack) {\n\t\t\tif _js_backtrace_settle("
	getPropSlot   = "*(*uintptr)(unsafe.Pointer(bp + 56))"
)

// A change in how any function uses a property slot stops fixlib, whatever
// the names of its variables and parameters are.
func TestAuditSlotsRejectsChanges(t *testing.T) {
	b := engineFile(t)
	if err := auditSlots(b); err != nil {
		t.Fatal(err)
	}
	type auditCase struct {
		name string
		b    []byte
		want []string
	}
	cases := []auditCase{
		{
			// A new helper that reads the value of the slot it is given,
			// whatever its parameter is called.
			"new helper",
			append(mutate(t, b, getPropHead, getPropSettle, "\t\t_ = _fresh_slot_value(tls, "+getPropSlot+")\n"),
				"\nfunc _fresh_slot_value(tls *libc.TLS, slot uintptr) TJSValue {\n\treturn *(*TJSValue)(unsafe.Pointer(slot))\n}\n"...),
			[]string{"_fresh_slot_value uses property slots and is not listed: slot read value=1", "XJS_GetPropertyInternal: uses changed"},
		},
		{
			// A read before the settle in a function that has one.
			"read before the guard",
			mutate(t, b, getPropHead, getPropSettle, "\t\tif prop == uint32(EJS_ATOM_stack) {\n\t\t\treturn XDupValue(tls, ctx3, *(*TJSValue)(unsafe.Pointer("+getPropSlot+")))\n\t\t}\n"),
			[]string{"XJS_GetPropertyInternal: uses changed", "early slot read value=1"},
		},
		{
			// The same read through a variable and pointer arithmetic.
			"read through a variable",
			mutate(t, b, getPropHead, getPropSettle, "\t\tq := "+getPropSlot+" + 0\n\t\t_ = (*TJSValue)(unsafe.Pointer(q)).Ftag\n"),
			[]string{"XJS_GetPropertyInternal: uses changed", "early slot read value=1"},
		},
		{
			// A slot pointer kept somewhere the audit cannot follow.
			"stored in a structure",
			mutate(t, b, getPropHead, getPropSettle, "\t\t(*TJSObject)(unsafe.Pointer(p2)).Ffirst_weak_ref = "+getPropSlot+"\n"),
			[]string{"XJS_GetPropertyInternal: uses changed", "early slot stored in (*TJSObject)(unsafe.Pointer(p2)).Ffirst_weak_ref=1"},
		},
		{
			// A new function that reads the prop array of an object.
			"new reader",
			append(append([]byte{}, b...), "\nfunc _new_reader(tls *libc.TLS, obj uintptr) TJSValue {\n\treturn *(*TJSValue)(unsafe.Pointer((*TJSObject)(unsafe.Pointer(obj)).Fprop + 16))\n}\n"...),
			[]string{"_new_reader uses property slots and is not listed: slot read value=1"},
		},
	}
	cases = append(cases, auditCase{
		// The slot pointer read from its cell through arithmetic on the
		// cell's address, in the form ccgo writes it.
		"cell offset",
		mutate(t, b, getPropHead, getPropSettle, "\t\t_ = (*TJSValue)(unsafe.Pointer(**(**uintptr)(__ccgo_up(bp + 56 + 0)))).Ftag\n"),
		[]string{"XJS_GetPropertyInternal: uses changed", "cell offset=1"},
	})
	// The same read through a variable declared each way Go allows, in a
	// listed function and in a new one.
	for _, decl := range []string{"var q = ", "var q uintptr = ", "q := "} {
		cases = append(cases, auditCase{
			"declared with " + decl + "in a listed function",
			mutate(t, b, getPropHead, getPropSettle, "\t\t"+decl+getPropSlot+"\n\t\t_ = (*TJSValue)(unsafe.Pointer(q)).Ftag\n"),
			[]string{"XJS_GetPropertyInternal: uses changed", "early slot read value=1"},
		}, auditCase{
			"declared with " + decl + "in a new function",
			append(append([]byte{}, b...), "\nfunc _new_reader(tls *libc.TLS, obj uintptr) TJSValue {\n\t"+decl+"(*TJSObject)(unsafe.Pointer(obj)).Fprop\n\treturn *(*TJSValue)(unsafe.Pointer(q))\n}\n"...),
			[]string{"_new_reader uses property slots and is not listed: slot read value=1"},
		})
	}
	for _, c := range cases {
		err := auditSlots(c.b)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: error does not say %q:\n%v", c.name, w, err)
			}
		}
	}
}

// getTimezoneOffset answers 0 in every generated file, and fixTimezone
// leaves a fixed file as it is.
func TestTimezoneOffsetIsUTC(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "lib", "ccgo_*.go"))
	if err != nil || len(files) == 0 {
		t.Fatal(files, err)
	}
	for _, fn := range files {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		out, err := fixTimezone(b)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		body, err := funcBody(out, reTimezoneOffset, "_getTimezoneOffset")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(string(body), "\treturn 0\n}\n") || bytes.Contains(out, []byte("libc.Xlocaltime")) {
			t.Fatalf("%s: getTimezoneOffset still reads the zone:\n%s", fn, body)
		}
		again, err := fixTimezone(out)
		if err != nil || !bytes.Equal(again, out) {
			t.Fatalf("%s: fixTimezone changed a fixed file: %v", fn, err)
		}
	}
}

// A read of libc's local time anywhere else, or a getTimezoneOffset that
// reads the zone some new way, stops fixlib.
func TestTimezoneRejectsLocalTime(t *testing.T) {
	b, err := fixTimezone(engineFile(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, call := range []string{"Xlocaltime_r(tls, 0, 0)", "Xstrftime(tls, 0, 0, 0, 0)", "Xctime_r(tls, 0, 0)", "Xctime(tls, 0)"} {
		elsewhere := bytes.Replace(b, []byte("\nfunc _js_regexp_finalizer("), []byte("\nfunc _other(tls *libc.TLS) {\n\tlibc."+call+"\n}\n\nfunc _js_regexp_finalizer("), 1)
		if _, err := fixTimezone(elsewhere); err == nil {
			t.Errorf("a new libc.%s call went through", call)
		}
	}

	body, err := funcBody(b, reTimezoneOffset, "_getTimezoneOffset")
	if err != nil {
		t.Fatal(err)
	}
	changed := bytes.Replace(b, body, []byte("\nfunc _getTimezoneOffset(tls *libc.TLS, time Tint64_t) (r int32) {\n\treturn int32(libc.Xtime(tls, 0))\n}\n"), 1)
	if _, err := fixTimezone(changed); err == nil {
		t.Error("a getTimezoneOffset of a new shape went through")
	}
}

// fixToCString leaves a fixed file as it is, and stops when upstream frees
// val at the fail label or adds a new way to fail: the free it adds would
// then free val twice, or miss a path.
func TestToCStringRejects(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		out, err := fixToCString(b)
		if err != nil || !bytes.Equal(out, b) {
			t.Fatalf("%s: fixToCString changed a fixed file: %v", fn, err)
		}
		// The label as upstream writes it, before fixUp.
		raw := bytes.ReplaceAll(b, []byte("*(*Tsize_t)(unsafe.Pointer(plen)) = uint"), []byte("**(**Tsize_t)(__ccgo_up(plen)) = uint"))
		if _, err := fixToCString(raw); err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
	}
	head := "func XJS_ToCStringLen2("
	for _, name := range []string{"ccgo_darwin_arm64.go", "ccgo_linux_386.go"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "lib", name))
		if err != nil {
			t.Fatal(err)
		}
		val := "val"
		if name == "ccgo_darwin_arm64.go" {
			val = "*(*TJSValue)(unsafe.Pointer(bp + 32))"
		}
		for _, c := range []struct{ name, at, text string }{
			{"free at the label", "\tif plen != 0 {\n\t\t*(*Tsize_t)(unsafe.Pointer(plen)) = uint", "\tXFreeValue(tls, ctx2, " + val + ")\n"},
			{"a new way to fail", "\tlen1 = ", "\tif plen == 0 {\n\t\tgoto fail\n\t}\n"},
		} {
			if _, err := fixToCString(mutate(t, b, head, c.at, c.text)); err == nil {
				t.Errorf("%s: %s went through", name, c.name)
			}
		}
	}
}

// Every generated file has the _build_backtrace that lib/backtrace.go was
// checked against, and a change to it, or to the functions whose work
// lib/backtrace.go repeats, stops fixlib.
func TestBacktraceSites(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkBacktraceSites(b, filepath.Base(fn)); err != nil {
			t.Fatal(err)
		}
	}
	b := engineFile(t)
	for _, c := range []struct{ head, old, new string }{
		{"func _build_backtrace(", "if (*TJSStackFrame)(unsafe.Pointer(sf)).Fjs_mode&", "if false && (*TJSStackFrame)(unsafe.Pointer(sf)).Fjs_mode&"},
		{"func _get_prop_string(", "p = (*TJSShape)(unsafe.Pointer((*TJSObject)(unsafe.Pointer(p)).Fshape)).Fproto\n", "p = 0\n"},
		{"func _JS_GetFunctionBytecode(", "\treturn (*(*struct {", "\treturn 8 + (*(*struct {"},
	} {
		i := bytes.Index(b, []byte("\n"+c.head))
		j := bytes.Index(b[i:], []byte(c.old))
		if i < 0 || j < 0 {
			t.Fatalf("no %q in %s", c.old, c.head)
		}
		changed := append(append(append([]byte{}, b[:i+j]...), c.new...), b[i+j+len(c.old):]...)
		if err := checkBacktraceSites(changed, "ccgo_darwin_arm64.go"); err == nil {
			t.Errorf("a change to %s went through", c.head)
		}
	}
}

// A caller of _js_backtrace_own_flags that reads the value of the
// descriptor, or hands it to another function, stops fixlib: it would get
// undefined for a pending stack.
func TestOwnFlagsRejectsValueRead(t *testing.T) {
	b := engineFile(t)
	if _, err := fixBacktrace(b); err != nil {
		t.Fatal(err)
	}
	head := "func _JS_CopyDataProperties("
	at := "\t\t\t_js_free_desc(tls, ctx2, bp+48)\n"
	for _, text := range []string{
		"\t\t\t_ = (*(*TJSPropertyDescriptor)(unsafe.Pointer(bp + 48))).Fvalue\n",
		"\t\t\t_ = *(*TJSValue)(unsafe.Pointer(bp + 48 + 8))\n",
		"\t\t\t_js_other(tls, bp+48)\n",
	} {
		if _, err := fixBacktrace(mutate(t, b, head, at, text)); err == nil {
			t.Errorf("%q went through", text)
		}
	}
}

// descUses reads both forms of a field access, before and after fixUp.
func TestDescUsesBothForms(t *testing.T) {
	body := "\nfunc f(tls *libc.TLS) {\n\tbp := tls.Alloc(56)\n\tdefer tls.Free(56)\n" +
		"\t_js_backtrace_own_flags(tls, 0, bp, 0, 0)\n" +
		"\t_ = (**(**TJSPropertyDescriptor)(__ccgo_up(bp))).Fflags\n" +
		"\t_ = (*(*TJSPropertyDescriptor)(unsafe.Pointer(bp))).Fflags\n" +
		"\t_ = (**(**TJSPropertyDescriptor)(__ccgo_up(bp))).Fvalue\n" +
		"\t_ = (*(*TJSPropertyDescriptor)(unsafe.Pointer(bp))).Fvalue\n" +
		"\t_ = *(*uintptr)(unsafe.Pointer(bp + 40))\n" +
		"\t_ = *(*uintptr)(unsafe.Pointer(bp + 8))\n" +
		"\t_js_free_desc(tls, 0, bp)\n}\n"
	uses, err := descUses("f", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"call _js_backtrace_own_flags": 1, "call _js_free_desc": 1, "value": 2, "raw": 1}
	if fmt.Sprint(uses) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", uses, want)
	}
}

// A change of fixBacktrace counts as made only as the whole text fixlib
// writes, where fixlib writes it.
func TestBacktraceRejectsTextElsewhere(t *testing.T) {
	b := engineFile(t)
	for _, c := range []struct {
		head, first string
		n           int
	}{
		{"_build_backtrace", "if filename == 0 && _js_backtrace_build(", 3},
		{"_get_prop_string", "if pending, ok := _js_backtrace_prop_string(", 3},
		{"XJS_GetPropertyInternal", "if prop == uint32(EJS_ATOM_stack) {", 5},
		{"_JS_GetOwnPropertyInternal", "if prop == uint32(EJS_ATOM_stack) && desc != 0 {", 5},
		{"XJS_DefineProperty", "if prop == uint32(EJS_ATOM_stack) && _js_backtrace_settle(", 3},
		{"_js_print_error", "if _js_backtrace_raw_stack(tls, s, p) {", 3},
		{"_js_print_object", "if _js_backtrace_print_pending(", 3},
		{"XJS_ComputeMemoryUsage", "_js_backtrace_memory_usage(", 1},
	} {
		if _, err := fixBacktrace(moveLines(t, b, c.head, c.first, c.n)); err == nil {
			t.Errorf("%s: %q elsewhere went through", c.head, c.first)
		}
	}
}
