// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package hwm

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	status := "Name:\tx\nVmPeak:\t  900 kB\nVmHWM:\t  1234 kB\nVmRSS:\t 1000 kB\n"
	kb, err := Parse(status)
	if err != nil || kb != 1234 {
		t.Fatalf("Parse = %d, %v, want 1234", kb, err)
	}
	if _, err := Parse("VmRSS:\t5 kB\n"); err == nil {
		t.Fatal("a status without VmHWM parsed")
	}
	if _, err := Parse("VmHWM:\t5 MB\n"); err == nil {
		t.Fatal("a unit other than kB parsed")
	}
}

func TestFindTakesTheLastLine(t *testing.T) {
	out := "eval: 3 runs\nvmhwm-kb: 100\nvmhwm-kb: 2048\n"
	if kb, ok := Find(out); !ok || kb != 2048 {
		t.Fatalf("Find = %d, %v", kb, ok)
	}
	if _, ok := Find("nothing\nvmhwm-kb: x\n"); ok {
		t.Fatal("found a number in a bad line")
	}
}

func TestPrintAndRead(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs /proc/self/status")
	}
	var b bytes.Buffer
	Print(&b)
	kb, ok := Find(b.String())
	if !ok || kb <= 0 {
		t.Fatalf("Print wrote %q", b.String())
	}
	if !strings.HasPrefix(b.String(), Prefix) {
		t.Fatalf("line %q does not start with %q", b.String(), Prefix)
	}
}
