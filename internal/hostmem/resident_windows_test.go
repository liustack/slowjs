// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// resident counts the pages of [addr, addr+size) in the working set of the
// process.
func resident(t *testing.T, addr, size uintptr) int {
	t.Helper()
	infos := make([]windows.PSAPI_WORKING_SET_EX_INFORMATION, (size+pageSize-1)/pageSize)
	for i := range infos {
		infos[i].VirtualAddress = windows.Pointer(pointer(addr + uintptr(i)*pageSize))
	}
	cb := uint32(uintptr(len(infos)) * unsafe.Sizeof(infos[0]))
	if err := windows.QueryWorkingSetEx(windows.CurrentProcess(), uintptr(unsafe.Pointer(&infos[0])), cb); err != nil {
		t.Fatalf("QueryWorkingSetEx of %d bytes at %#x: %v", size, addr, err)
	}
	n := 0
	for _, info := range infos {
		if info.VirtualAttributes.Valid() {
			n++
		}
	}
	return n
}

// memFree is the state of an address that nothing has reserved.
const memFree = 0x10000

// anyMapped reports whether a page of [addr, addr+size) is reserved.
func anyMapped(t *testing.T, addr, size uintptr) bool {
	t.Helper()
	var mbi windows.MemoryBasicInformation
	for p := addr; p < addr+size; p = mbi.BaseAddress + mbi.RegionSize {
		if err := windows.VirtualQuery(p, &mbi, unsafe.Sizeof(mbi)); err != nil {
			t.Fatalf("VirtualQuery at %#x: %v", p, err)
		}
		if mbi.State != memFree {
			return true
		}
	}
	return false
}
