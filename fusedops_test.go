// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package slowjs // import "github.com/liustack/slowjs"

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liustack/slowjs/internal/cengine"
)

// fusedOps turns on or off the opcodes the compiler joins from a few
// instructions, for the functions m compiles from then on.
func fusedOps(m *VM, on bool) {
	m.outer(func() { cengine.JS_SetFusedOps(m.sp, m.ctx, uintptr(map[bool]int{false: 0, true: 1}[on])) })
}

// The scripts of engine/test/compare on the opcodes the compiler joins print
// their file in the Go build of the engine too, with the opcodes and without.
func TestFusedOps(t *testing.T) {
	for _, name := range []string{"fused_ops", "typeof_is"} {
		src, err := os.ReadFile(filepath.Join("engine", "test", "compare", name+".js"))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("engine", "test", "compare", name+".txt"))
		if err != nil {
			t.Fatal(err)
		}
		for _, on := range []bool{true, false} {
			m := newTestVM(t)
			fusedOps(m, on)
			if err := m.RegisterFunc("gc", func() { m.RunGC() }, false); err != nil {
				t.Fatal(err)
			}
			if _, err := m.Eval(string(src), EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if _, err := m.ExecutePendingJobs(); err != nil {
				t.Fatal(err)
			}
			got, err := m.Eval(`out.join("\n")`, EvalGlobal)
			if err != nil {
				t.Fatal(err)
			}
			gl, wl := strings.Split(got.(string), "\n"), strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
			for i := range max(len(gl), len(wl)) {
				var g, w string
				if i < len(gl) {
					g = gl[i]
				}
				if i < len(wl) {
					w = wl[i]
				}
				if g != w {
					t.Errorf("%s fused %v line %d:\ngot  %s\nwant %s", name, on, i+1, g, w)
				}
			}
		}
	}
}
