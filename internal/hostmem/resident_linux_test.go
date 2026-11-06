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
