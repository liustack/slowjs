// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hostmem

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"testing"
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
