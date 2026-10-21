// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// quickjsLibcKept is what stays of quickjs-libc.c on every target: the
// functions of quickjsLibcRoots and what they reach.
var quickjsLibcKept = []string{
	"Xjs_load_file", "_js_loadScript", "_js_module_loader_so",
	"Xjs_module_set_import_meta", "_json_module_init", "_create_json_module",
	"Xjs_module_test_json", "Xjs_module_loader", "_js_print_value_write",
	"_get_time_ns", "_js_os_now", "_js_print", "_js_console_log",
	"Xjs_std_add_helpers", "_js_std_dump_error1", "Xjs_std_dump_error",
}

// fixQuickJSLibc leaves a fixed file as it is, on every target, and keeps
// quickjsLibcKept.
func TestQuickJSLibcIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		got, err := fixQuickJSLibc(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s: a fixed file changed", name)
		}
		_, site, kept, dropped, err := trimQuickJSLibc(b)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if site.names != "" || len(dropped) != 0 {
			t.Errorf("%s: %s left to drop", name, strings.Join(dropped, " "))
		}
		if !slices.Equal(kept, quickjsLibcKept) {
			t.Errorf("%s: keeps %s, want %s", name, strings.Join(kept, " "), strings.Join(quickjsLibcKept, " "))
		}
	}
}

// Taken from upstream, a file loses the std, os and Worker modules, and
// what stays is what the committed file has. It needs libquickjs v0.13.1 in
// the module cache (go mod download modernc.org/libquickjs@v0.13.1).
func TestQuickJSLibcUpstream(t *testing.T) {
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Skip("go env GOMODCACHE:", err)
	}
	dir := filepath.Join(strings.TrimSpace(string(out)), "modernc.org", "libquickjs@v0.13.1")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("libquickjs v0.13.1 is not in the module cache")
	}
	hw, err := handWritten(filepath.Join("..", "..", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ccgo_darwin_arm64.go", "ccgo_linux_386.go", "ccgo_windows.go"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if b, err = fixBeforeUp(b, hw, name); err == nil {
			b, err = fixUp(b)
		}
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		_, _, _, dropped, err := trimQuickJSLibc(b)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, want := range []string{"Xjs_init_module_std", "Xjs_init_module_os", "_js_std_funcs", "_js_os_funcs", "_js_std_popen", "_js_std_file_puts", "_js_worker_ctor", "init(_js_std_file_class)", "init(_js_worker_class)", "Xjs_std_loop"} {
			if !slices.Contains(dropped, want) {
				t.Errorf("%s: %s stays", name, want)
			}
		}
		if name != "ccgo_windows.go" && !slices.Contains(dropped, "_js_os_exec") {
			t.Errorf("%s: _js_os_exec stays", name)
		}
		got, err := fixQuickJSLibc(b, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		fixed, err := readEngine(filepath.Join("..", "..", "lib", name))
		if err != nil {
			t.Fatal(err)
		}
		gs, ge, err := quickjsLibcPart(got)
		if err != nil {
			t.Fatal(err)
		}
		fs, fe, err := quickjsLibcPart(fixed)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got[gs:ge], fixed[fs:fe]) {
			t.Errorf("%s: what stays of quickjs-libc.c differs from the committed file", name)
		}
	}
}

// A function upstream adds to quickjs-libc.c, a change to what stays, a
// dropped function used elsewhere, a root that is gone or a mark out of
// place stops fixQuickJSLibc.
func TestQuickJSLibcRejects(t *testing.T) {
	const name = "ccgo_darwin_arm64.go"
	b := string(engineFile(t))
	upstream := strings.Replace(b, quickjsLibcMark, "\n", 1) // as if never trimmed
	newFunc := "func _js_os_new(tls *libc.TLS) {\n}\n\n"
	dump := "func Xjs_std_dump_error(tls *libc.TLS, "
	// called puts a call of _js_os_new at the start of the function head
	// starts.
	called := func(in, head string) string {
		at := strings.Index(in, head)
		if at < 0 {
			t.Fatalf("no %q", head)
		}
		open := strings.Index(in[at:], "{\n") + at + 2
		return in[:open] + "\t_js_os_new(tls)\n" + in[open:]
	}
	withNew := strings.Replace(b, dump, newFunc+dump, 1)
	// initOf is an init function as ccgo writes one, which fills v.
	initOf := func(v string) string {
		return "func init() {\n\tp := unsafe.Pointer(&" + v + ")\n\t*(*uintptr)(unsafe.Add(p, 0)) = 0\n}\n\n"
	}
	for _, c := range []struct {
		name, in, err string
	}{
		{"a new function", strings.Replace(upstream, dump, newFunc+dump, 1), "the functions and variables of quickjs-libc.c changed"},
		{"a new function in a fixed file", withNew, "has the mark of fixQuickJSLibc and functions or variables to drop: _js_os_new"},
		{"a new function used elsewhere", called(strings.Replace(upstream, dump, newFunc+dump, 1), "\nfunc XJS_NewRuntime(tls *libc.TLS)"), "_js_os_new used outside what stays"},
		{"a new function a root calls", called(withNew, "\nfunc Xjs_std_add_helpers(tls *libc.TLS, "), "what stays of quickjs-libc.c changed"},
		{"a change to what stays", called(b, "\nfunc _js_console_log(tls *libc.TLS, "), "what stays of quickjs-libc.c changed"},
		{"a root that is gone", strings.Replace(b, dump, "func Xjs_std_dump_error2(tls *libc.TLS, ", 1), "has no function Xjs_std_dump_error of quickjsLibcRoots"},
		{"an init of an engine variable", strings.Replace(b, dump, initOf("_js_engine_table")+dump, 1), "an init function fills _js_engine_table, which is not a variable of quickjs-libc.c"},
		{"an init of a function", strings.Replace(b, dump, initOf("_js_console_log")+dump, 1), "an init function fills _js_console_log, which is not a variable of quickjs-libc.c"},
		{"no mark", upstream, "nothing to drop but no mark"},
		{"the mark elsewhere", strings.Replace(b, dump, strings.TrimPrefix(quickjsLibcMark, "\n")+dump, 1), "is not at its start"},
	} {
		_, err := fixQuickJSLibc([]byte(c.in), name)
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
}

// The test262 runner still calls the three functions of quickjs-libc.c it
// uses in lib/, which fixQuickJSLibc keeps.
func TestQuickJSLibcRunner(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "lib", "internal", "run-test262", "ccgo_*.go"))
	if err != nil || len(files) == 0 {
		t.Fatal(files, err)
	}
	for _, fn := range files {
		b, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"Xjs_load_file", "Xjs_module_test_json", "Xjs_std_dump_error"} {
			if !bytes.Contains(b, []byte("libquickjs."+f+"(")) {
				t.Errorf("%s: no call of %s in lib", filepath.Base(fn), f)
			}
		}
	}
}
