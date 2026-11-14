// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package gen

import (
	"strings"
	"testing"
)

// TestEmitDataAligned: the data after a function's code goes after a
// PCALIGN $16, which adds nothing where the data is on 16 bytes in the
// object, and a function whose data is not on 16 there is refused.
func TestEmitDataAligned(t *testing.T) {
	var b strings.Builder
	if err := emitData(&b, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}, 0x40); err != nil {
		t.Fatal(err)
	}
	want := "\tPCALIGN $16\n" + dataComment + "\n\tQUAD $0x0807060504030201\n\tBYTE $0x09\n"
	if b.String() != want {
		t.Errorf("emitData wrote\n%s\nwant\n%s", b.String(), want)
	}
	b.Reset()
	if err := emitData(&b, nil, 0x48); err != nil || b.Len() != 0 {
		t.Errorf("no data: %q, %v", b.String(), err)
	}
	for _, at := range []int{0x48, 0x44, 0x41} {
		if err := emitData(&b, []byte{1}, at); err == nil || !strings.Contains(err.Error(), "past a multiple of 16") {
			t.Errorf("data at %#x: %v, want refused", at, err)
		}
	}
}

// TestDataPlaced: a body whose data would start off 16 bytes in the binary is
// refused, since the PCALIGN $16 in front of the data would have padded and
// moved it. A body with no data, or whose data starts on 16, passes.
func TestDataPlaced(t *testing.T) {
	if err := dataPlaced(0x47e800, 0x40, 0x60); err != nil {
		t.Errorf("data on 16: %v", err)
	}
	if err := dataPlaced(0x47e808, 0x44, 0x44); err != nil {
		t.Errorf("no data: %v", err)
	}
	for _, body := range []uint64{0x47e808, 0x47e804, 0x47e801} {
		if err := dataPlaced(body, 0x40, 0x60); err == nil || !strings.Contains(err.Error(), "not on 16 bytes") {
			t.Errorf("data at %#x: %v, want refused", body+0x40, err)
		}
	}
}
