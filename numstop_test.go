// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package quickjs // import "github.com/liustack/quickjs"

import (
	"errors"
	"testing"
	"time"
)

// The eval timeout stops the conversion of a long string to a number or a
// BigInt within a few polls: the loops over its characters (the spaces
// around it, the scan of its digits, their copy, the digits that js_atod
// reads, the leading zeros of a BigInt) read the interrupt flag every 65,536
// characters, and so do the number and the spaces of the text of JSON.parse
// and the number of a script. Before, +'1'.repeat(64 << 20) ran for a few
// hundred milliseconds without reading it, past the grace of the kill.
func TestNumberConversionStops(t *testing.T) {
	// flat joins its parts in one flat string: a rope would be made flat by
	// each call, in steps that read the flag
	const n = `function flat(...a) { return a.join('') }; var n = 1 << 26`
	for _, c := range []struct{ name, setup, call string }{
		{"unary plus", n + `, s = '1'.repeat(n)`, `+s`},
		{"Number of zeros", n + `, s = '0'.repeat(n)`, `Number(s)`},
		{"Number of spaces", n + `, s = flat(' '.repeat(n), '1')`, `Number(s)`},
		{"Number of wide spaces", n + `, s = flat('\u3000'.repeat(n >> 2), '1')`, `Number(s)`},
		{"Number of a long exponent", n + `, s = flat('1e', '0'.repeat(n))`, `Number(s)`},
		{"Number of a long fraction", n + `, s = flat('0.', '1'.repeat(n))`, `Number(s)`},
		{"Number of hex digits", n + `, s = flat('0x', '0'.repeat(n), 'f')`, `Number(s)`},
		{"subtraction", n + `, s = '1'.repeat(n)`, `s - 0`},
		{"isNaN", n + `, s = '1'.repeat(n)`, `isNaN(s)`},
		{"parseInt", n + `, s = '1'.repeat(n)`, `parseInt(s)`},
		{"parseInt of spaces", n + `, s = flat(' '.repeat(n), '1')`, `parseInt(s, 16)`},
		{"parseFloat", n + `, s = '1'.repeat(n)`, `parseFloat(s)`},
		{"BigInt of zeros", n + `, s = '0'.repeat(n)`, `BigInt(s)`},
		{"BigInt of spaces", n + `, s = flat(' '.repeat(n), '1')`, `BigInt(s)`},
		{"BigInt of hex zeros", n + `, s = flat('0x', '0'.repeat(n), '1')`, `BigInt(s)`},
		{"BigInt compared with zeros", n + `, s = '0'.repeat(n)`, `1n == s`},
		{"BigInt less than spaces", n + `, s = flat(' '.repeat(n), '1')`, `1n < s`},
		{"JSON.parse of digits", n + `, s = '1'.repeat(n)`, `JSON.parse(s)`},
		{"JSON.parse of a fraction", n + `, s = flat('0.', '1'.repeat(n))`, `JSON.parse(s)`},
		{"JSON.parse of spaces", n + `, s = flat(' '.repeat(n), '1')`, `JSON.parse(s)`},
		{"JSON.parse of newlines", n + `, s = flat('\r\n'.repeat(n >> 1), '1')`, `JSON.parse(s)`},
		{"eval of a number", n + `, s = '1'.repeat(n)`, `(0, eval)(s)`},
		{"eval of a BigInt", n + `, s = flat('0x', '0'.repeat(n), '1n')`, `(0, eval)(s)`},
		{"eval of a legacy octal", n + `, s = flat('0', '7'.repeat(n))`, `(0, eval)(s)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := newTestVM(t)
			if _, err := m.Eval(c.setup+`; 0`, EvalGlobal); err != nil {
				t.Fatal(err)
			}
			if err := m.SetEvalTimeout(20 * time.Millisecond); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			_, err := m.Eval(`for (;;) `+c.call, EvalGlobal)
			took := time.Since(start)
			if errors.Is(err, ErrKilled) {
				t.Fatalf("killed after %v, want the interrupt", took)
			}
			var e *Error
			if !errors.As(err, &e) || !e.Interrupted || !e.Uncatchable {
				t.Fatalf("got %v after %v, want interrupted", err, took)
			}
			if n := m.h.StopPolls(); n > numberStopPolls {
				t.Errorf("%d polls saw the request to stop, want at most %d (stopped %v after the call started)", n, numberStopPolls, took)
			}
			t.Logf("stopped after %v, %d polls saw the request", took, m.h.StopPolls())
			if err := m.SetEvalTimeout(0); err != nil {
				t.Fatal(err)
			}
			if v, err := m.Eval(`1+1`, EvalGlobal); err != nil || v != 2 {
				t.Fatalf("the VM after the stop: 1+1 got %v, %v", v, err)
			}
		})
	}
}

// numberStopPolls is the most polls TestNumberConversionStops lets see the
// request: the loops read the flag every 65,536 characters, a step of a
// fraction of a poll, and the error and the free of the strings take a few
// more.
const numberStopPolls = 8

// A conversion that reads the flag on its way gives what it gave: strings
// of 200,000 characters and more, which cross the places where the loops
// read it.
func TestNumberConversionLongResults(t *testing.T) {
	m := newTestVM(t)
	const z = `'0'.repeat(200000)`
	const sp = `' '.repeat(200000)`
	for _, c := range []struct{ expr, want string }{
		{`Number(` + z + ` + '1.5')`, `1.5`},
		{`Number(` + sp + ` + '-2.25' + ` + sp + `)`, `-2.25`},
		{`Number('1e' + ` + z + ` + '2')`, `100`},
		{`Number('0.' + ` + z + ` + '1e200001')`, `1`},
		{`Number('0x' + ` + z + ` + 'ff')`, `255`},
		{`Number('0b' + ` + z + ` + '101')`, `5`},
		{`Number('1'.repeat(200000)) === Infinity`, `true`},
		{`Number('1' + ` + z + ` + 'x')`, `NaN`},
		{`Number(` + sp + `)`, `0`},
		{`Number('\u3000\u2028'.repeat(100000) + '8' + '\u3000'.repeat(100000))`, `8`},
		{`Number('\u3000'.repeat(100000))`, `0`},
		{`parseInt(` + sp + ` + '42abc')`, `42`},
		{`parseInt(` + z + ` + '7f', 16)`, `127`},
		{`parseFloat(` + sp + ` + '3.5e1x')`, `35`},
		{`parseFloat('0.' + ` + z + ` + '5') === 5e-200001`, `true`},
		{`String(BigInt(` + z + ` + '123456789012345678901234567890'))`, `123456789012345678901234567890`},
		{`String(BigInt(` + sp + ` + '0x' + ` + z + ` + 'ff' + ` + sp + `))`, `255`},
		{`String(BigInt('0o' + ` + z + ` + '777'))`, `511`},
		{`BigInt(` + sp + `) === 0n`, `true`},
		{`5n == ` + z + ` + '5'`, `true`},
		{`5n < ` + sp + ` + '6'`, `true`},
		{`JSON.parse(` + sp + ` + '-12.5e-1' + ` + sp + `)`, `-1.25`},
		{`JSON.parse('[' + ` + sp + ` + '1,' + ` + sp + ` + '2]').join()`, `1,2`},
		{`JSON.parse('0.' + ` + z + ` + '1e200001')`, `1`},
		{`JSON.parse('\r\n\r'.repeat(100000) + '[7,\r\n8]' + '\n\r'.repeat(100000)).join()`, `7,8`},
		{`JSON.parse('-0.' + '9'.repeat(70000))`, `-1`},
		{`(0, eval)('0' + '7'.repeat(10))`, `1073741823`},
		{`(0, eval)('1'.repeat(200000)) === Infinity`, `true`},
	} {
		v, err := m.Eval(`String(`+c.expr+`)`, EvalGlobal)
		if err != nil {
			t.Errorf("%s: %v", c.expr, err)
			continue
		}
		if v != c.want {
			t.Errorf("%s = %v, want %s", c.expr, v, c.want)
		}
	}
}
