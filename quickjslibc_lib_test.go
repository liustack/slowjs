// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build !slowjs_cengine

package slowjs // import "github.com/liustack/slowjs"

import (
	"bufio"
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The part of quickjs-libc.c that the Go API reaches works as before
// internal/fixlib dropped the rest of it: the bytecode of a module, which
// sets import.meta and waits for the module's promise.
func TestQuickJSLibcKept(t *testing.T) {
	m, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

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
