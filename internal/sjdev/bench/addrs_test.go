// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package bench

import (
	"fmt"
	"strings"
	"testing"
)

func TestBodyStartSkipsTheFrameSetUpAndTheNOPs(t *testing.T) {
	// 0x1c bytes of frame set-up, PCALIGN padding to 64, five 8-byte NOPs, then
	// 8 bytes of NOP from xform and the body.
	code := make([]byte, 0, 200)
	code = append(code, make([]byte, 0x1c)...)
	for i := range code {
		code[i] = 0x48
	}
	for len(code) < 64 {
		code = append(code, 0x90)
	}
	for range 6 {
		code = append(code, nops[8]...)
	}
	code = append(code, 0x48, 0x89, 0xe5)
	if got := bodyStart(0x6f7e00, code); got != 64+48 {
		t.Errorf("body at %d, want %d", got, 64+48)
	}
	if got := bodyStart(0x1000, append(append([]byte{}, nops[3]...), 0x55)); got != 3 {
		t.Errorf("body at %d, want 3", got)
	}
}

func TestOpcodeNames(t *testing.T) {
	h := "FMT(none)\nDEF(invalid, 1, 0, 0, none)\nDEF(       push_i32, 5, 0, 1, i32)\n#if SHORT_OPCODES\nDEF(get_loc0, 1, 0, 1, none_loc)\ndef(temp, 1, 0, 0, none)\n"
	names, err := OpcodeNames(strings.NewReader(h))
	if err != nil || fmt.Sprint(names) != "[invalid push_i32 get_loc0]" {
		t.Errorf("%v %v", names, err)
	}
}
