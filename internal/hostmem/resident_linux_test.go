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
	"unsafe"

	"golang.org/x/sys/unix"
)

// resident counts the pages of the mapping [addr, addr+size) that take
// memory: its Rss in /proc/self/smaps, which counts a huge page as all its
// pages. mincore is not used here: it counts every page with an entry in the
// page table, and a page that was read but never written has one that points
// to the shared zero page of the kernel, which takes no memory. The range must
// be one whole mapping, as a shadow stack is.
func resident(t *testing.T, addr, size uintptr) int {
	t.Helper()
	return int(uintptr(smapsKB(t, addr, size, "Rss")<<10) / pageSize)
}

// written counts the pages of [addr, addr+size) in memory, by mincore, for a
// range whose pages are written or not touched at all, as a block's are in
// the tests: mincore counts a page read and never written too, which the
// range has none of. smaps does not serve for a block, as the kernel may
// join it with the mapping next to it into one, which it did with one of
// the race detector's.
func written(t *testing.T, addr, size uintptr) int {
	t.Helper()
	vec := make([]byte, (size+pageSize-1)/pageSize)
	if _, _, e := unix.Syscall(unix.SYS_MINCORE, addr, size, uintptr(unsafe.Pointer(&vec[0]))); e != 0 {
		t.Fatalf("mincore of %d bytes at %#x: %v", size, addr, e)
	}
	n := 0
	for _, v := range vec {
		n += int(v & 1)
	}
	return n
}

// smapsKB returns the field of /proc/self/smaps, in kB, for the mapping
// [addr, addr+size).
func smapsKB(t *testing.T, addr, size uintptr, field string) uint64 {
	t.Helper()
	f, err := os.Open("/proc/self/smaps")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	head := fmt.Sprintf("%08x-%08x ", addr, addr+size)
	in := false
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if !in {
			in = strings.HasPrefix(line, head)
			continue
		}
		v, ok := strings.CutPrefix(line, field+":")
		if !ok {
			// the lines of a mapping end where the range of the next starts
			if first, _, _ := strings.Cut(line, " "); !strings.HasSuffix(first, ":") {
				break
			}
			continue
		}
		kb, err := strconv.ParseUint(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
		if err != nil {
			t.Fatalf("the %s of the mapping at %#x: %v", field, addr, err)
		}
		return kb
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatalf("no %s for a mapping of %d bytes at %#x in /proc/self/smaps\n%s", field, size, addr, mappingDetail(addr))
	return 0
}
