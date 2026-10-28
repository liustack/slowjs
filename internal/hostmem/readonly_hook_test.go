// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

//go:build testhooks

package hostmem

import (
	"errors"
	"testing"
)

// TestReadOnlyCalls: MapReadOnly maps, then protects the pages it wrote,
// and gives them back when the OS refuses to protect them. UnmapReadOnly
// unmaps the same pages.
func TestReadOnlyCalls(t *testing.T) {
	n := int(pageSize) + 1
	size := ReadOnlySize(n)
	calls := failing(t, func(c call, before []call) error {
		if c.name == "protect" {
			return errInjected
		}
		return nil
	})
	if _, err := MapReadOnly(make([]byte, n)); !errors.Is(err, errInjected) {
		t.Fatalf("MapReadOnly with protect refused: %v, want the error of protect", err)
	}
	if len(*calls) != 3 || (*calls)[0].name != "map" || (*calls)[1].name != "protect" || (*calls)[2].name != "unmap" {
		t.Fatalf("MapReadOnly with protect refused made %+v, want a map, a protect and an unmap", *calls)
	}
	p := (*calls)[2].addr
	if (*calls)[1].addr != p || (*calls)[1].size != size {
		t.Fatalf("protect of %+v, want the %d bytes at %#x", (*calls)[1], size, p)
	}

	*calls = nil
	fault = func(name string, addr, size uintptr) error {
		*calls = append(*calls, call{name, addr, size})
		return nil
	}
	p, err := MapReadOnly(make([]byte, n))
	if err != nil {
		t.Fatal(err)
	}
	if err := UnmapReadOnly(p, n); err != nil {
		t.Fatal(err)
	}
	if c := (*calls)[len(*calls)-1]; c.name != "unmap" || c.addr != p {
		t.Fatalf("UnmapReadOnly made %+v, want an unmap at %#x", c, p)
	}
}
