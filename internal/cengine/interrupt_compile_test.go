// Copyright (c) 2026 The liustack/slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package cengine

import (
	"errors"
	"fmt"
	"testing"
)

// compileStopPolls is how many polls may see the request to stop before the
// parse and the compile of a source have stopped: two steps, as for the
// parse of a long token (parseStopPolls of the root package).
const compileStopPolls = 16

// compileCases are sources of 10 to 50 KB of short tokens, each made by its
// setup, with a number that changes at each call so that nothing is kept
// between two calls. Each makes one part of the parse and the compile long:
// the parse and the code of one long expression, of many statements at the
// top level and in a function (whose code is converted to register code when
// it runs), the jumps the labels resolve, the names of many locals, an object
// literal of many keys, the closures of a local that is written, whose paths
// capture_by_value follows, and four loops whose work grows as a square: the
// jumps resolve_labels makes shorter, each of which moves the code after it,
// the vars of an eval, each looked for among the variables of the function
// around it, the variables of the functions around a direct eval, which the
// compile collects for it and looks for at each level, and the parameters of
// a function, each looked for among those before it. The polls of the
// freeing of the code a stopped compile made are not counted
// (JS_GetInterruptFlag in engine/quickjs.h).
var compileCases = []struct{ name, setup, source string }{
	{"an array literal of 24,576 numbers", `var S = '[' + '1,'.repeat(3 << 13)`, `S + n++ + ']'`},
	{"8,192 statements at the top level", `var a = 0, S = 'a=a+1;'.repeat(1 << 13)`, `S + n++`},
	{"8,192 statements in a function", `var a = 0, S = '(function(){' + 'a=a+1;'.repeat(1 << 13)`, `S + n++ + '})'`},
	{"8,192 statements in a function that runs", `var a = 0, S = '(function(){' + 'a=a+1;'.repeat(1 << 13)`, `S + n++ + '})()'`},
	{"4,096 ifs in a function that runs", `var a = 0, S = '(function(){' + 'if(a)a++;else a--;'.repeat(1 << 12)`, `S + n++ + '})()'`},
	{"2,048 locals in a function that runs", `var S = '(function(){' + Array.from({length: 1 << 11}, (_, i) => 'let x' + i + '=' + i + ';').join('')`, `S + 'return ' + n++ + '})()'`},
	{"an object literal of 4,096 keys", `var S = '({' + Array.from({length: 1 << 12}, (_, i) => 'k' + i + ':1,').join('')`, `S + 'n:' + n++ + '})'`},
	{"2,048 closures of a local it writes, in a function that runs", `var S = '(function(){let x = 0;' + 'x++;(() => x);'.repeat(1 << 11)`, `S + 'return ' + n++ + '})()'`},
	{"512 jumps the labels shorten", `var a = 0, S = '(function(){' + ('if(a){' + 'a=a+1;'.repeat(8) + '}').repeat(1 << 9)`, `S + n++ + '})'`},
	{"an eval of 1,024 vars in a function of 1,024 locals", `var L = Array.from({length: 1 << 10}, (_, i) => 'x' + i).join(','), V = 'var ' + Array.from({length: 1 << 10}, (_, i) => 'y' + i).join(','), f = (0, eval)('(function(V){var ' + L + '; eval(V)})')`, `'f(V + ",z' + n++ + '")'`},
	{"an eval in five functions around 2,048 locals", `var S = '(function(){var ' + Array.from({length: 1 << 11}, (_, i) => 'x' + i).join(',') + ';return function(){return function(){return function(){return function(){return function(){return eval("x0")}}}}}})//'`, `S + n++`},
	{"a function of 8,192 parameters", `var S = '(function(' + Array.from({length: 1 << 13}, (_, i) => 'p' + i).join(',') + '){})//'`, `S + n++`},
}

// TestInterruptStopsCompileAtEveryPoll evaluates each source of
// compileCases once to count its polls, then stops it at every one of them:
// each stop must come within compileStopPolls, with the error interrupted.
// The parse read the flag only in the scans of long tokens, where their
// pointer met a multiple of 65,536 bytes, and the passes of the compile
// read none, so a stop waited for the whole compile: up to 652 polls saw it
// for the 2,048 locals on linux/amd64, and the eval timeout of the Go API
// killed the VM in the compile of a script of 3 MB (TestCompileStops of the
// root package).
func TestInterruptStopsCompileAtEveryPoll(t *testing.T) {
	needPolls(t)
	for _, c := range compileCases {
		t.Run(c.name, func(t *testing.T) {
			v := newVM(t, Options{Seed: 1})
			defer closeVM(t, v)
			h := v.Host()
			if _, err := v.Eval(c.setup+`; var n = 0; 0`, "setup.js"); err != nil {
				t.Fatal(err)
			}
			v.SetGCThreshold(^uintptr(0))
			call := `(0, eval)(` + c.source + `)`
			v.RunGC()
			before := h.Polls()
			if _, err := v.Eval(call+`; 0`, "call.js"); err != nil {
				t.Fatal(err)
			}
			span := h.Polls() - before
			var worst int64
			var worstAt string
			for at := int64(1); at <= span; at++ {
				v.RunGC()
				h.InterruptAtPoll(h.Polls() + at)
				_, err := v.Eval(`for (;;) `+call, "call.js")
				h.InterruptAtPoll(0)
				what := fmt.Sprintf("the stop at poll %d of %d", at, span)
				var je *JSError
				if !errors.As(err, &je) || je.Message != "InternalError: interrupted" || !je.Uncatchable {
					t.Fatalf("%s: Eval gave %v, want the error interrupted", what, err)
				}
				if n := h.StopPolls(); n > worst {
					worst, worstAt = n, what
				}
			}
			t.Logf("one call took %d polls, at most %d saw the request (%s)", span, worst, worstAt)
			if worst > compileStopPolls {
				t.Errorf("%s: %d polls saw the request, want at most %d", worstAt, worst, compileStopPolls)
			}
		})
	}
}
