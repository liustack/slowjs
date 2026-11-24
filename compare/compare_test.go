// Copyright (c) 2026 The liustack/beastjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE at the
// root of this repository.

// Package compare times QuickJS's micro benchmarks on this engine, on goja
// and on fastschema/qjs (QuickJS on wazero). make benchmark runs them and
// benchreport.go makes the table.
package compare

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/fastschema/qjs"
	"github.com/liustack/beastjs"
)

//go:embed testdata/*.js
var scripts embed.FS

// skipped are the scripts one of the engines fails, which the three leave
// out alike.
var skipped = map[string]bool{
	"date_parse.js":             true,
	"global_destruct_strict.js": true,
	"global_write_strict.js":    true,
	"map_delete.js":             true,
	"weak_map_delete.js":        true,
	"weak_map_set.js":           true,
}

// names are the scripts every engine runs.
var names []string

func TestMain(m *testing.M) {
	entries, err := fs.ReadDir(scripts, "testdata")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for _, e := range entries {
		if !skipped[e.Name()] {
			names = append(names, e.Name())
		}
	}
	os.Exit(m.Run())
}

// script is the benchmark name, set to run its loop n times.
func script(b *testing.B, name string, n int) string {
	src, err := scripts.ReadFile("testdata/" + name)
	if err != nil {
		b.Fatal(err)
	}

	js := strings.Replace(string(src), "$N", strconv.Itoa(n), 1)
	// Start each engine from as little garbage of the one before as can be.
	debug.FreeOSMemory()
	return js
}

func BenchmarkLIUSTACK(b *testing.B) {
	for _, name := range names {
		b.Run(name, func(b *testing.B) {
			js := script(b, name, b.N)
			vm, err := beastjs.NewVM()
			if err != nil {
				b.Fatal(err)
			}

			defer vm.Close()

			b.ReportAllocs()
			b.ResetTimer()
			if out, err := vm.Eval(js, beastjs.EvalGlobal); err != nil {
				b.Fatalf("%v (%v)", err, out)
			}
		})
	}
}

func BenchmarkQJS(b *testing.B) {
	for _, name := range names {
		b.Run(name, func(b *testing.B) {
			js := script(b, name, b.N)
			vm, err := qjs.New()
			if err != nil {
				b.Fatal(err)
			}

			defer vm.Close()

			b.ReportAllocs()
			b.ResetTimer()
			out, err := vm.Eval(name, qjs.Code(js))
			if err != nil {
				b.Fatalf("%v (%v)", err, out)
			}
			if out != nil {
				out.Free()
			}
		})
	}
}

func BenchmarkGOJA(b *testing.B) {
	for _, name := range names {
		b.Run(name, func(b *testing.B) {
			js := script(b, name, b.N)
			vm := goja.New()
			b.ReportAllocs()
			b.ResetTimer()
			if out, err := vm.RunString(js); err != nil {
				b.Fatalf("%v (%v)", err, out)
			}
		})
	}
}
