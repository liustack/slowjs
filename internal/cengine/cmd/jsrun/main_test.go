// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/liustack/slowjs/internal/sjdev/hwm"
)

// TestMain lets the tests run this binary as jsrun: with JSRUN_AS_MAIN set it
// runs main with the arguments it was given.
func TestMain(m *testing.M) {
	if os.Getenv("JSRUN_AS_MAIN") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

// TestOutputSaysWhatIsTimed: the output names what the time of a run
// includes, so that a figure is not read as the engine call alone.
func TestOutputSaysWhatIsTimed(t *testing.T) {
	file := filepath.Join(t.TempDir(), "job.js")
	if err := os.WriteFile(file, []byte(`Promise.resolve(1).then(() => { globalThis.x = 41 + 1 }); "done"`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-n", "2", file)
	cmd.Env = append(os.Environ(), "JSRUN_AS_MAIN=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jsrun: %v\n%s", err, out)
	}
	var timed string
	for _, line := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(line, "timed: "); ok {
			timed = rest
		}
	}
	if timed == "" {
		t.Fatalf("no line says what is timed:\n%s", out)
	}
	for _, want := range []string{"source", "jobs", "string", "not timed"} {
		if !strings.Contains(timed, want) {
			t.Errorf("the line about the timing does not mention %q: %s", want, timed)
		}
	}
	if !strings.Contains(string(out), "result: done") || !strings.Contains(string(out), "eval: 2 runs") {
		t.Errorf("the usual output is missing:\n%s", out)
	}
}

// TestOutputEndsWithItsOwnPeakMemory: sjdev bench run reads the peak memory from
// the line the process prints itself.
func TestOutputEndsWithItsOwnPeakMemory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs /proc/self/status")
	}
	file := filepath.Join(t.TempDir(), "job.js")
	if err := os.WriteFile(file, []byte(`"done"`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], file)
	cmd.Env = append(os.Environ(), "JSRUN_AS_MAIN=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jsrun: %v\n%s", err, out)
	}
	if kb, ok := hwm.Find(string(out)); !ok || kb <= 0 {
		t.Errorf("no vmhwm-kb line:\n%s", out)
	}
}
