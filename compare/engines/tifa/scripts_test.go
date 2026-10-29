// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// Package tifa has the checks of the shell scripts of this folder, run with
// stand-ins, without running any engine.
package tifa

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// run.sh calls pagerun.sh with paths made from the caller's directory, with
// TZ=UTC and GOMAXPROCS=1, for the nine functional checks and the timed runs.
func TestCallerPathsAndCheckEnvironment(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	scripts := filepath.Join(root, "scripts")
	runSh, err := os.ReadFile("run.sh")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(scripts, "run.sh"), string(runSh), 0o755)
	for _, app := range []string{"bench", "mui", "shop"} {
		writeFile(t, filepath.Join(scripts, "in", app, "one.json"), "{}", 0o644)
	}
	writeFile(t, filepath.Join(scripts, "pagerun.sh"), `printf '%s\t%s\t%s\t%s\n' "$TZ" "$GOMAXPROCS" "$1" "$2" >> "$CALL_LOG"`+"\n", 0o644)
	log := filepath.Join(root, "calls.tsv")
	cmd := exec.Command("bash", filepath.Join(scripts, "run.sh"), "new output", "engine bins", "native bins")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CALL_LOG="+log)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("run.sh: %v\n%s", err, out)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	checks, node := 0, false
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			t.Fatalf("bad call line %q", line)
		}
		tz, procs, stem, bin := f[0], f[1], f[2], f[3]
		if tz != "UTC" || procs != "1" {
			t.Errorf("TZ=%q GOMAXPROCS=%q, want UTC and 1: %s", tz, procs, line)
		}
		if !strings.HasPrefix(stem, filepath.Join(root, "new output")+"/") {
			t.Errorf("output %q is not under the caller's output directory", stem)
		}
		if !strings.HasPrefix(bin, filepath.Join(root, "engine bins")+"/") && !strings.HasPrefix(bin, filepath.Join(root, "native bins")+"/") {
			t.Errorf("command %q is not under the caller's bin directories", bin)
		}
		if strings.Contains(stem, "/check/") {
			checks++
		}
		if strings.HasSuffix(bin, "/bin/node") {
			node = true
		}
	}
	if checks != 9 {
		t.Errorf("%d functional checks, want 9", checks)
	}
	if !node {
		t.Error("no run of Node")
	}
}

// pagerun.sh caps each try at 4 GiB on its CPU, and makes three tries when
// the sibling was busy or the process did not get its CPU.
func TestCapAndThreeDisturbedTries(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(root, "calls")
	for name, content := range map[string]string{
		"awk":         "#!/bin/sh\ncase \"$1\" in -v) echo \"1 0 1\" ;; *) echo 0 ;; esac\n",
		"sleep":       "#!/bin/sh\nexit 0\n",
		"systemd-run": "#!/bin/sh\nprintf \"%s\\n\" \"$*\" >> \"$CALL_LOG\"\necho \"{}\"\n",
	} {
		writeFile(t, filepath.Join(root, name), content, 0o755)
	}
	stem := filepath.Join(root, "output with spaces")
	cmd := exec.Command("bash", "pagerun.sh", stem, "fake-engine")
	cmd.Env = append(os.Environ(), "PATH="+root+":"+os.Getenv("PATH"), "CALL_LOG="+log)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pagerun.sh: %v\n%s", err, out)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(calls) != 3 {
		t.Fatalf("%d tries, want 3", len(calls))
	}
	for _, c := range calls {
		if !strings.Contains(c, "-p MemoryMax=4G -p MemorySwapMax=0 taskset -c 1 fake-engine") {
			t.Errorf("call %q lacks the cap and the CPU", c)
		}
	}
	meta, err := os.ReadFile(stem + ".meta")
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(strings.TrimSuffix(string(meta), "\n"), "\n") {
		var try struct{ Try int }
		if err := json.Unmarshal([]byte(line), &try); err != nil || try.Try != i {
			t.Errorf("meta line %d = %q, want try %d", i, line, i)
		}
	}
}
