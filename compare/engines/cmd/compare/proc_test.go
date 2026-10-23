// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestMain lets the test binary act as a process that keeps allocating, so
// that the memory guard has something to stop.
func TestMain(m *testing.M) {
	if os.Getenv("COMPARE_GROW") == "1" {
		var keep [][]byte
		for i := 0; i < 64; i++ { // up to 1 GiB
			b := make([]byte, 16<<20)
			for j := range b {
				b[j] = 1
			}
			keep = append(keep, b)
			time.Sleep(20 * time.Millisecond)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestGuardStopsAProcessOverTheCap(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "COMPARE_GROW=1")
	_, why, err := outputTimeout(cmd, 64<<20, time.Minute)
	if why != "memory" {
		t.Fatalf("why = %q, err = %v; want memory", why, err)
	}
}

func TestGuardTimeout(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	_, why, _ := outputTimeout(cmd, 1<<30, 200*time.Millisecond)
	if why != "timeout" {
		t.Fatalf("why = %q; want timeout", why)
	}
}

func TestGuardNormalExit(t *testing.T) {
	out, why, err := outputTimeout(exec.Command("echo", "hi"), 1<<30, time.Minute)
	if why != "" || err != nil || string(out) != "hi\n" {
		t.Fatalf("out %q, why %q, err %v", out, why, err)
	}
}
