// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The parts of quickjs-libc.c that the Go API reaches work as before
// internal/fixlib dropped the rest of it: the helpers of StdAddHelpers, the
// module loader of SetDefaultModuleLoader with a script and a JSON module,
// and the bytecode of a module, which sets import.meta and waits for the
// module's promise.
func TestQuickJSLibcKept(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "script.js")
	if err := os.WriteFile(script, []byte("globalThis.loaded = 'script'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(dir, "data.json")
	if err := os.WriteFile(data, []byte(`{"answer": 42}`), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	if err := m.StdAddHelpers(); err != nil {
		t.Fatal(err)
	}
	for js, want := range map[string]string{
		"[typeof console.log, typeof print, typeof performance.now, typeof __loadScript].join()": "function,function,function,function",
		"performance.now() > 0":                               "true",
		"__loadScript(" + strconv.Quote(script) + "); loaded": "script",
		"[typeof std, typeof os, typeof Worker].join()":       "undefined,undefined,undefined",
	} {
		if got := evalString(t, m, js); got != want {
			t.Errorf("%s: %s, want %s", js, got, want)
		}
	}

	m.SetDefaultModuleLoader()
	module := "import * as Power from './testdata/power.js'\n" +
		"import data from " + strconv.Quote(data) + " with { type: 'json' }\n" +
		"globalThis.imported = [Power.cube(3), data.answer].join()\n"
	if _, err := m.Eval(module, EvalModule); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, m, "imported"); got != "27,42" {
		t.Errorf("imported: %s, want 27,42", got)
	}
	if _, err := m.Eval("import * as std from 'qjs:std'", EvalModule); err == nil {
		t.Error("the module qjs:std loaded")
	}

	// The module is compiled in m and run in a VM of its own: import.meta
	// finds the module by its name, which is the same for every module
	// compiled from a string.
	code, err := m.Compile("await Promise.resolve()\nglobalThis.awaited = [import.meta.main, typeof import.meta.url].join()\n", EvalModule)
	if err != nil {
		t.Fatal(err)
	}
	m2, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	if _, err := m2.EvalBytecode(code); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, m2, "awaited"); got != "true,string" {
		t.Errorf("awaited: %s, want true,string", got)
	}
}

// A program that only evaluates javascript links none of the std, os and
// Worker modules of quickjs-libc.c, nor the parts of libc only they call:
// internal/fixlib drops them, as the Go API cannot register them (see
// docs/fork-changes/trimmed.md).
func TestQuickJSLibcNotLinked(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go command")
	}
	bin := filepath.Join(t.TempDir(), "evalonly")
	if goos == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command(gobin, "build", "-o", bin, "./testdata/evalonly").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	if out, err := exec.Command(bin).Output(); err != nil || string(out) != "2,4,6\n" {
		t.Fatalf("evalonly: %q, %v", out, err)
	}
	out, err := exec.Command(gobin, "tool", "nm", bin).Output()
	if err != nil {
		t.Fatalf("go tool nm: %v", err)
	}

	const (
		libPrefix  = "github.com/liustack/slowjs/lib."
		libcPrefix = "modernc.org/libc."
	)
	dropped := map[string]bool{
		"Xjs_init_module_std": true, "Xjs_init_module_os": true, "Xjs_std_init_handlers": true,
		"_js_std_funcs": true, "_js_os_funcs": true, "_js_std_init": true, "_js_os_init": true,
		"_js_std_popen": true, "_js_std_setenv": true, "_js_std_urlGet": true, "_my_execvpe": true,
	}
	// The functions of libc the dropped code called, which a program
	// linked before. fork, execve and popen are not among them: the
	// compiler inlines them into their callers (_js_os_exec, _js_std_popen)
	// even when they are linked, so nm never shows them.
	libc := map[string]bool{
		"Xpclose": true, "Xsetenv": true, "Xunsetenv": true, "Xwaitpid": true, "Xpipe": true,
		"Xkill": true, "Xtcsetattr": true, "Xsymlink": true, "Xtmpfile": true,
	}
	var found []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		sym := f[len(f)-1]
		if name, ok := strings.CutPrefix(sym, libPrefix); ok {
			if dropped[name] ||
				strings.HasPrefix(name, "_js_os_") && name != "_js_os_now" ||
				strings.HasPrefix(name, "_js_std_file_") ||
				strings.HasPrefix(name, "_js_worker_") {
				found = append(found, sym)
			}
		}
		if name, ok := strings.CutPrefix(sym, libcPrefix); ok && libc[name] {
			found = append(found, sym)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("linked: %s", strings.Join(found, " "))
	}
}
