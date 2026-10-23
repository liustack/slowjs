// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"strings"
	"testing"
	"unsafe"
)

// _js_charge_interrupts counts the steps it is given, and stops the counter
// at 0, however many, so that the counter cannot wrap to a large positive
// count.
func TestChargeInterrupts(t *testing.T) {
	v := newReVM(t, -1, 0)
	defer v.close()
	counter := &(*TJSContext)(unsafe.Pointer(v.ctx)).Finterrupt_counter
	for _, c := range []struct {
		from int32
		cost Tsize_t
		want int32
	}{
		{100, 0, 100},
		{100, 40, 60},
		{100, 99, 1},
		{100, 100, 0},
		{100, 1<<31 - 1, 0},
		{0, 0, 0},
	} {
		*counter = c.from
		_js_charge_interrupts(v.ctx, c.cost)
		if *counter != c.want {
			t.Errorf("%d less %d steps: counter %d, want %d", c.from, c.cost, *counter, c.want)
		}
	}
}

// The parser counts a step for each byte of the source it is given, and
// the string buffer one for each 8 characters it makes room for, so a
// built-in that parses or builds a long string counts in proportion to its
// length.
func TestChargeParseAndBuild(t *testing.T) {
	v := newReVM(t, -1, 0)
	defer v.close()
	counter := &(*TJSContext)(unsafe.Pointer(v.ctx)).Finterrupt_counter
	v.run("var src = '0//" + strings.Repeat("x", 32000) + "'; var json = JSON.stringify('" + strings.Repeat("x", 32000) + "'); 0")
	for _, c := range []struct {
		name     string
		js       string
		min, max int32
	}{
		{"eval", "eval(src); 0", 32000, 33000},
		// new Function builds its source in a string buffer, then parses
		// it, and JSON.parse parses its text, then builds the string.
		{"new Function", "new Function(src); 0", 36000, 44000},
		{"JSON.parse", "JSON.parse(json); 0", 36000, 44000},
		{"repeat", "'x'.repeat(32000); 0", 4000, 4100},
		// A buffer grows by half each time, and counts each room it adds:
		// three times the final length at most.
		{"JSON.stringify", "JSON.stringify(json); 0", 4000, 12000},
	} {
		*counter = 1 << 30
		v.run(c.js)
		if got := 1<<30 - *counter; got < c.min || got > c.max {
			t.Errorf("%s: %d steps, want %d to %d", c.name, got, c.min, c.max)
		}
	}
}
