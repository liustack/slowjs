// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// TestStackNoHugePages reads the flags the kernel keeps for the mapping of
// the shadow stack: nh says no transparent huge pages.
func TestStackNoHugePages(t *testing.T) {
	if _, err := os.Stat("/sys/kernel/mm/transparent_hugepage"); err != nil {
		t.Skipf("a kernel without transparent huge pages: %v", err)
	}
	m := newMemory(t, DefaultStackSize)
	base, size := m.Stack()
	flags := vmFlags(t, base)
	t.Logf("the mapping of the stack: %s", flags)
	if !strings.Contains(" "+flags+" ", " nh ") {
		t.Fatalf("the %d bytes of the stack at %#x have the flags %q, want nh", size, base, flags)
	}
}

// vmFlags returns the VmFlags line of /proc/self/smaps for the mapping that
// starts at addr.
func vmFlags(t *testing.T, addr uintptr) string {
	t.Helper()
	f, err := os.Open("/proc/self/smaps")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	head := fmt.Sprintf("%x-", addr)
	in := false
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		switch {
		case strings.HasPrefix(line, head):
			in = true
		case in && strings.HasPrefix(line, "VmFlags:"):
			return strings.TrimSpace(strings.TrimPrefix(line, "VmFlags:"))
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("no mapping at %#x in /proc/self/smaps", addr)
	return ""
}

// TestStackReadPages reads the first 8 MiB of a shadow stack after a write
// of its first byte. A page read but never written maps the shared zero page
// of the kernel and takes no memory, so only the page written counts. On the
// linux/arm64 runners of CI, something other than the test read 8 MiB of one
// stack of TestManyStacks, and mincore counted those 2048 pages as resident.
func TestStackReadPages(t *testing.T) {
	m := newMemory(t, DefaultStackSize)
	base, size := m.Stack()
	bytesAt(base, 1)[0] = 1
	b := bytesAt(base, 8<<20)
	for i := 0; i < len(b); i += int(pageSize) {
		sink += b[i]
	}
	if n := resident(t, base, size); n != 1 {
		t.Fatalf("%d pages resident after a write of one byte and reads of 8 MiB, want 1\n%s", n, mappingDetail(base))
	}
}

// TestResidentHugePage: resident counts a transparent huge page as all its
// pages, so a stack that got one at its first touch fails TestManyStacks.
func TestResidentHugePage(t *testing.T) {
	b, err := os.ReadFile("/sys/kernel/mm/transparent_hugepage/hpage_pmd_size")
	if err != nil {
		t.Skipf("a kernel without transparent huge pages: %v", err)
	}
	huge, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	n := 2 * uintptr(huge)
	p, err := unix.MmapPtr(-1, 0, nil, n, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.MunmapPtr(p, n) })
	// The aligned huge page inside becomes a mapping of its own.
	addr := alignUp(uintptr(p), uintptr(huge))
	if err := unix.Madvise(bytesAt(addr, uintptr(huge)), unix.MADV_HUGEPAGE); err != nil {
		t.Skipf("MADV_HUGEPAGE: %v", err)
	}
	bytesAt(addr, 1)[0] = 1
	if smapsKB(t, addr, uintptr(huge), "AnonHugePages") != huge>>10 {
		t.Skipf("the kernel gave no huge page\n%s\n%s", mappingDetail(addr), hugePageSettings())
	}
	if got, want := resident(t, addr, uintptr(huge)), int(uintptr(huge)/pageSize); got != want {
		t.Fatalf("%d pages resident in a huge page of %d bytes, want %d\n%s", got, huge, want, mappingDetail(addr))
	}
}
