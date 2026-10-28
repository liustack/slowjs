// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package hwm reads the most memory the calling process held, from the
// process itself, for the programs that sjdev times.
//
// A parent that reads the peak of a child with wait4 gets a number that is not
// below the parent's own peak, because the child starts as a copy of it. So a
// program under test prints its own peak just before it exits, as a line
// "vmhwm-kb: N", and `sjdev bench run` reads that line.
package hwm

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Prefix starts the line a program prints for sjdev.
const Prefix = "vmhwm-kb:"

// Parse returns the VmHWM line of the text of /proc/PID/status, in KiB.
func Parse(status string) (int64, error) {
	for _, line := range strings.Split(status, "\n") {
		rest, ok := strings.CutPrefix(line, "VmHWM:")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) != 2 || f[1] != "kB" {
			return 0, fmt.Errorf("hwm: unexpected VmHWM line %q", line)
		}
		return strconv.ParseInt(f[0], 10, 64)
	}
	return 0, fmt.Errorf("hwm: no VmHWM line")
}

// Read returns the peak resident memory of this process in KiB. It fails where
// there is no /proc/self/status.
func Read() (int64, error) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, err
	}
	return Parse(string(b))
}

// Print writes the "vmhwm-kb: N" line to w, or nothing where the peak cannot
// be read.
func Print(w io.Writer) {
	if kb, err := Read(); err == nil {
		fmt.Fprintf(w, "%s %d\n", Prefix, kb)
	}
}

// Find returns the number of the last "vmhwm-kb: N" line of out, and whether
// there was one.
func Find(out string) (int64, bool) {
	var kb int64
	found := false
	for _, line := range strings.Split(out, "\n") {
		rest, ok := strings.CutPrefix(line, Prefix)
		if !ok {
			continue
		}
		if n, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64); err == nil {
			kb, found = n, true
		}
	}
	return kb, found
}
