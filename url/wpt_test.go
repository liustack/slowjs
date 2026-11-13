// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package url

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/liustack/quickjs"
)

// newVM returns a VM with URL and URLSearchParams.
func newVM(t testing.TB) *quickjs.VM {
	t.Helper()
	vm, err := quickjs.NewVM()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vm.Close() })
	if err := Register(vm); err != nil {
		t.Fatal(err)
	}
	return vm
}

func readFile(t testing.TB, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// knownList reads a list of known failures: one line per test, as the quoted
// "<file> <name>", two spaces and "# " and the reason.
func knownList(t testing.TB, path string) map[string]bool {
	m := map[string]bool{}
	for _, l := range strings.Split(readFile(t, path), "\n") {
		if l == "" || l[0] == '#' {
			continue
		}
		if i := strings.Index(l, `"  # `); i >= 0 {
			l = l[:i+1]
		}
		m[l] = true
	}
	return m
}

// TestWPT runs the URL and URLSearchParams tests of web-platform-tests in a
// VM, on the files as they are and the few functions of testharness.js they
// use (testdata/harness.js).
func TestWPT(t *testing.T) {
	files, err := filepath.Glob("testdata/wpt/*.any.js")
	if err != nil || len(files) == 0 {
		t.Fatalf("no test files: %v", err)
	}
	known := knownList(t, "testdata/known-failures.txt")
	used := map[string]bool{}
	data := map[string]string{}
	for _, n := range []string{"urltestdata.json", "urltestdata-javascript-only.json", "setters_tests.json", "IdnaTestV2.json", "IdnaTestV2-removed.json", "toascii.json"} {
		data["resources/"+n] = readFile(t, "../internal/whatwgurl/testdata/"+n)
	}
	harness := readFile(t, "testdata/harness.js")
	total := 0
	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			vm := newVM(t)
			if _, err := vm.Eval(harness, quickjs.EvalGlobal); err != nil {
				t.Fatal(err)
			}
			for k, v := range data {
				if _, err := vm.Eval("__files["+strconv.Quote(k)+"] = "+strconv.Quote(v), quickjs.EvalGlobal); err != nil {
					t.Fatal(err)
				}
			}
			vm.SetEvalTimeout(0)
			if _, err := vm.Eval(readFile(t, file), quickjs.EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if _, err := vm.ExecutePendingJobs(); err != nil {
				t.Fatal(err)
			}
			out, err := vm.Eval("JSON.stringify(__results)", quickjs.EvalGlobal)
			if err != nil {
				t.Fatal(err)
			}
			var results []struct {
				Name    string
				Ok      bool
				Message string
			}
			if err := json.Unmarshal([]byte(out.(string)), &results); err != nil {
				t.Fatal(err)
			}
			if len(results) == 0 {
				t.Fatal("no test ran")
			}
			for _, r := range results {
				total++
				key := strconv.Quote(name + " " + r.Name)
				if known[key] {
					used[key] = true
					if r.Ok {
						t.Errorf("%s passes but is in known-failures.txt", key)
					}
					continue
				}
				if !r.Ok {
					t.Errorf("%s: %s", key, r.Message)
				}
			}
		})
	}
	for k := range known {
		if !used[k] && !t.Failed() {
			t.Errorf("%s is in known-failures.txt but did not run", k)
		}
	}
	t.Logf("%d tests", total)
}
