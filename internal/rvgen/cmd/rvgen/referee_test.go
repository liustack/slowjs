// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRefereeSide(t *testing.T) {
	off := transcriptLines("out a\nresult f0.0 ! TypeError: not a function\nresult sum 12\nstack at f\nmem-end obj=1\n--- info\nout zzz\n")
	on := transcriptLines("out a\nresult f0.0 = s:x\nresult sum 13\n--- status 0\n")
	if len(off) != 3 || len(on) != 3 {
		t.Fatalf("transcriptLines kept %q and %q", off, on)
	}
	// Node words the error its own way: the same side only when cut loose
	node := []string{"out a", "result f0.0 ! TypeError: x is not a function", "result sum 99"}
	if s := side(exact(node), exact(off), exact(on)); s != "neither" {
		t.Errorf("exact: %s, want neither", s)
	}
	if s := side(loose(node), loose(off), loose(on)); s != "off" {
		t.Errorf("loose: %s, want off", s)
	}
	exc := []string{"exception TypeError: cannot define"}
	if s := side(exact([]string{"exception TypeError: 'set' on proxy"}), exact(exc), exact([]string{"result ok"})); s != "off" {
		t.Errorf("an exception by its name: %s, want off", s)
	}
}

func TestRefereeLean(t *testing.T) {
	off := "result     at boom /    at d1 /    at d2 /"
	on := "result     at boom /    at d2 /"
	if l := lean(off, on, "result Error: b/    at boom /    at d1 /    at d2 (x.js:3:1)/"); !strings.HasPrefix(l, "leans off") {
		t.Errorf("node has d1: %s", l)
	}
	if l := lean(off, on, "result Error: b/    at boom /    at d2 (x.js:3:1)/"); !strings.HasPrefix(l, "leans on") {
		t.Errorf("node has no d1: %s", l)
	}
}

// the host of the Node run writes the lines rvdiff writes
func TestRefereeNodeHost(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node")
	}
	dir := t.TempDir()
	host := dir + "/host.cjs"
	script := dir + "/s.js"
	must(os.WriteFile(host, []byte(nodeHost), 0o666))
	must(os.WriteFile(script, []byte("print('a', 1); console.log('b\\nc');\nPromise.resolve().then(() => print('job'));\n(function () { return 'r'; })\n"), 0o666))
	out, err := exec.Command(node, host, script, "").CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	want := "out a 1\nout b\nout c\nout job\nresult r\n"
	if string(out) != want {
		t.Errorf("node host wrote\n%s\nwant\n%s", out, want)
	}
	must(os.WriteFile(script, []byte("var p = new Proxy({}, { defineProperty() { return false; } }); p.x = 1;\n"), 0o666))
	out, _ = exec.Command(node, host, script, "-strict").CombinedOutput()
	if !strings.HasPrefix(string(out), "exception TypeError") {
		t.Errorf("a strict set the trap refuses: node host wrote %q, want an exception", out)
	}
}
