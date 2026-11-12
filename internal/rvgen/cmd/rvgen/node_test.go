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

func TestNodeLines(t *testing.T) {
	q := transcriptLines("out a\nresult f0.0 ! TypeError: not a function\nstack at f\nmem-end obj=1\n--- info\nout zzz\n")
	if len(q) != 2 {
		t.Fatalf("transcriptLines kept %q", q)
	}
	// Node words the error its own way: an exception is compared by its name
	if !same(exact([]string{"exception TypeError: 'set' on proxy"}), exact([]string{"exception TypeError: cannot define"})) {
		t.Errorf("an exception is not compared by its name")
	}
	if same(exact([]string{"result 1"}), exact([]string{"result 2"})) {
		t.Errorf("results that differ are the same")
	}
}

// the host of the Node run writes the lines rvdiff writes
func TestNodeHost(t *testing.T) {
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
