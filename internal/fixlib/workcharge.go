// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
)

// fixWorkCharge makes the parser and the string builder count their work
// toward the interrupt counter of the interpreter, the Go form of
// js_charge_interrupts in lib/internal/patch/quickjs.diff (see
// lib/charge.go):
//
//   - _js_parse_init counts a step for each byte of the source it is given,
//     for eval, new Function, scripts, modules and JSON.parse.
//   - _string_buffer_init2 counts a step for each 8 characters it makes
//     room for, and _string_buffer_realloc for each 8 it adds room for.
//
// The interpreter calls the interrupt handler once in
// JS_INTERRUPT_COUNTER_INIT polls, and a call of a built-in was one poll
// however much it parsed or built, so a loop of such calls ran on for
// hundreds of milliseconds past its timeout. _js_charge_interrupts counts on
// the counter that ___js_poll_interrupts resets, so the text of those
// functions is pinned in workChargeSites: when a release changes one of
// them, fixWorkCharge stops, and the new code must be read against
// lib/charge.go before the fingerprints are updated (go run ./internal/fixlib
// -charge prints them). The fingerprints are of the text without the
// change, after every other change before it.
func fixWorkCharge(b []byte, file string) ([]byte, error) {
	got, err := workChargeFingerprint(b)
	if err != nil {
		return nil, err
	}
	if want := workChargeSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/charge.go, then update workChargeSites (go run ./internal/fixlib -charge)", file, "js_parse_init, the string buffer or __js_poll_interrupts", got, want)
	}
	for _, c := range workCharges {
		body, err := funcBody(b, funcHead(c.name), c.name)
		if err != nil {
			return nil, err
		}
		fixed, err := insertAt(c.name, body, c.text, func(body []byte) (int, error) {
			if c.after {
				return after(body, c.anchor, "", c.name+": cannot find where the work is counted")
			}
			return before(body, c.anchor, "", c.name+": cannot find where the work is counted")
		})
		if err != nil {
			return nil, err
		}
		b = bytes.Replace(b, body, fixed, 1)
	}
	if n := bytes.Count(b, []byte("_js_charge_interrupts(")); n != len(workCharges) {
		return nil, fmt.Errorf("want %d calls of _js_charge_interrupts, found %d", len(workCharges), n)
	}
	return b, nil
}

// workCharges are the counts fixWorkCharge inserts: text goes before
// anchor, or after it.
var workCharges = []struct {
	name, anchor, text string
	after              bool
}{
	{
		name:   "_string_buffer_init2",
		anchor: "\t(*TStringBuffer)(unsafe.Pointer(s)).Ferror_status = 0\n",
		text: "\t// Changed by internal/fixlib: count the characters toward the interrupt counter (see fixWorkCharge).\n" +
			"\t_js_charge_interrupts(ctx, Tsize_t(size>>3))\n",
		after: true,
	},
	{
		name:   "_string_buffer_realloc",
		anchor: "\t(*TStringBuffer)(unsafe.Pointer(s)).Fsize = new_size\n\t(*TStringBuffer)(unsafe.Pointer(s)).Fstr = new_str\n",
		text: "\t// Changed by internal/fixlib: count the characters added toward the interrupt counter (see fixWorkCharge).\n" +
			"\t_js_charge_interrupts((*TStringBuffer)(unsafe.Pointer(s)).Fctx, Tsize_t((new_size-(*TStringBuffer)(unsafe.Pointer(s)).Fsize)>>3))\n",
	},
	{
		name:   "_js_parse_init",
		anchor: "\t(*TJSParseState)(unsafe.Pointer(s)).Fget_line_col_cache.Fcol_num = 0\n",
		text: "\t// Changed by internal/fixlib: count the source toward the interrupt counter (see fixWorkCharge).\n" +
			"\t_js_charge_interrupts(ctx, input_len)\n",
		after: true,
	},
}

// workChargeFuncs are the functions fixWorkCharge changes, and the one that
// resets the counter it counts on.
var workChargeFuncs = []string{"_string_buffer_init2", "_string_buffer_realloc", "_js_parse_init", "___js_poll_interrupts"}

// unfixWorkCharge gives text without the counts fixWorkCharge inserts.
func unfixWorkCharge(text []byte) []byte {
	for _, c := range workCharges {
		text = bytes.ReplaceAll(text, []byte(c.text), nil)
	}
	return text
}

// workChargeFingerprint hashes workChargeFuncs, without the change of
// fixWorkCharge, with each C string written out in place of its offset.
func workChargeFingerprint(b []byte) (string, error) {
	return funcsFingerprint(b, workChargeFuncs, unfixWorkCharge)
}
