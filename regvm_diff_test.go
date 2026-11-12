// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liustack/quickjs/internal/cengine"
	"github.com/liustack/quickjs/internal/rvgen"
)

// The scripts of engine/test/regvm/corpus that give the same output with
// both loops on the native host today. throws.js is left out: it prints
// the frames of a stack, which the register loop does not list yet.
var regvmDiffCorpus = []string{"arith.js", "calls.js", "depth.js", "refs.js"}

// regvmDiffRun evaluates src in a VM with the register loop on or off and
// returns what it gave and how many functions the register pass converted.
func regvmDiffRun(t *testing.T, src string, on bool) (string, int) {
	m := newTestVM(t)
	regvm(m, on)
	// gc() of the native host (engine/test/jsrun.c), which refs.js calls
	if err := m.RegisterHostFunc("gc", func([]any) (any, error) { m.RunGC(); return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Eval(regvmExpectJS, EvalGlobal); err != nil {
		t.Fatal(err)
	}
	r, err := m.Eval(src, EvalGlobal)
	if err == nil {
		_, err = m.ExecutePendingJobs()
	}
	out := fmt.Sprintf("%v\nerror: %v", r, err)
	if on {
		out += regvmExpect(t, m)
	}
	return out, regvmStat(m, 0)
}

// regvmExpectJS is rvExpect of the native rvdiff (engine/test/regvm/rvdiff.c)
// for the Go VM: it keeps the functions it is given on the global object,
// for regvmExpect to read after the run.
const regvmExpectJS = `var __rvxN = 0, __rvxNames = [];
function rvExpect() {
  for (var i = 0; i < arguments.length; i++) {
    var f = arguments[i];
    globalThis["__rvx" + __rvxN++] = f;
    __rvxNames.push(typeof f === "function" && f.name ? f.name : "?");
  }
}`

// regvmExpect returns a line for each function given to rvExpect that has
// no register code (JS_RegvmFuncStat 0 is not 1), as rvdiff writes one: the
// run with the register loop on then differs from the other.
func regvmExpect(t *testing.T, m *VM) string {
	n, err := m.Eval("__rvxN", EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	names, err := m.Eval(`__rvxNames.join("\n")`, EvalGlobal)
	if err != nil {
		t.Fatal(err)
	}
	count, ok := n.(int)
	if !ok {
		t.Fatalf("__rvxN is %T %v", n, n)
	}
	name := strings.Split(names.(string), "\n")
	var out strings.Builder
	g := m.jsGetGlobalObject()
	defer m.jsFreeValue(g)
	for i := range count {
		a, err := m.jsNewAtom(fmt.Sprintf("__rvx%d", i))
		if err != nil {
			t.Fatal(err)
		}
		v := m.jsGetProperty(g, a)
		m.jsFreeAtom(a)
		var st, op, pos int
		m.outer(func() {
			st = int(int32(cengine.JS_RegvmFuncStat(m.sp, m.ctx, v.u, v.tag, 0)))
			if st == -1 {
				op = int(int32(cengine.JS_RegvmFuncStat(m.sp, m.ctx, v.u, v.tag, 2)))
				pos = int(int32(cengine.JS_RegvmFuncStat(m.sp, m.ctx, v.u, v.tag, 3)))
			}
		})
		m.jsFreeValue(v)
		switch st {
		case 1:
		case -1:
			fmt.Fprintf(&out, "\nexpect %s (argument %d) has no register code: refused at opcode %d, offset %d (JS_RegvmFuncStat 2 and 3)",
				name[i], i+1, op, pos)
		default:
			fmt.Fprintf(&out, "\nexpect %s (argument %d) has no register code: state %d (JS_RegvmFuncStat 0)", name[i], i+1, st)
		}
	}
	return out.String()
}

// TestRegvmDiff runs a fixed part of the differential corpus of the native
// host (engine/test/regvm/corpus, the tests of engine/test/regvm/js, and
// programs of internal/rvgen of fixed seeds) on the generated engine with the register loop off and on, and
// wants the same output. A script in which the engine converts nothing is
// skipped: the generated engine then has no register code for what it uses
// (it is older than the C, or the pass refuses it), and the two runs would
// both be the old loop. A function given to rvExpect that has no register
// code in the run with the loop on is a difference.
func TestRegvmDiff(t *testing.T) {
	type script struct{ name, src string }
	var scripts []script
	for _, f := range regvmDiffCorpus {
		b, err := os.ReadFile("engine/test/regvm/corpus/" + f)
		if err != nil {
			t.Fatal(err)
		}
		scripts = append(scripts, script{f, string(b)})
	}
	// the tests of the packages of the register loop, which end with rvExpect
	pkg, err := filepath.Glob("engine/test/regvm/js/*.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range pkg {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		scripts = append(scripts, script{filepath.Base(f), string(b)})
	}
	for seed := uint64(1); seed <= 12; seed++ {
		o := rvgen.Options{Core: seed%2 == 0}
		scripts = append(scripts, script{fmt.Sprintf("rvgen-%d-core-%v", seed, o.Core), rvgen.Generate(seed, o)})
	}
	for _, s := range scripts {
		t.Run(s.name, func(t *testing.T) {
			off, _ := regvmDiffRun(t, s.src, false)
			on, conv := regvmDiffRun(t, s.src, true)
			if conv == 0 {
				t.Skip("no function converted: the engine has no register code for this script")
			}
			if off == on {
				return
			}
			a, b := strings.Split(off, "\n"), strings.Split(on, "\n")
			for i := 0; i < len(a) || i < len(b); i++ {
				var x, y string
				if i < len(a) {
					x = a[i]
				}
				if i < len(b) {
					y = b[i]
				}
				if x != y {
					t.Fatalf("line %d differs (%d functions converted):\noff: %q\non:  %q", i+1, conv, x, y)
				}
			}
		})
	}
}
