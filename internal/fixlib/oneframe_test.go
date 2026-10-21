// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// oneFrameInput returns the generated file name of lib/ as fixOneFrame gets
// it: with every change before it made, and the defers as ccgo writes them.
func oneFrameInput(t *testing.T, name string) []byte {
	t.Helper()
	b, err := fixInline(inlineInput(t, name), name)
	if err == nil {
		b, err = fixLast(b, name)
	}
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// oneFrameFiles are the files fixOneFrame changes: linux on 64-bit
// targets, where the frame of JS_CallInternal is 1280 bytes.
var oneFrameFiles = map[string]bool{
	"ccgo_linux_amd64.go":   true,
	"ccgo_linux_arm64.go":   true,
	"ccgo_linux_loong64.go": true,
	"ccgo_linux_ppc64le.go": true,
	"ccgo_linux_riscv64.go": true,
	"ccgo_linux_s390x.go":   true,
}

// In the files of linux on 64-bit targets JS_CallInternal takes one TLS
// frame, of 1280 bytes and the room of the largest small alloca, and no
// other file is changed. readEngine gives the function back as fixAlloca
// writes it, and fixOneFrame makes the file again.
func TestOneFrameEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		name := filepath.Base(fn)
		raw, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		body, err := funcBody(raw, funcHead(oneFrameFunc), oneFrameFunc)
		if err != nil {
			t.Fatal(err)
		}
		code := uncommented(body)
		allocs := strings.Count(code, "tls.Alloc(")
		changed := strings.Contains(string(body), "(see fixOneFrame)")
		switch {
		case oneFrameFiles[name] && (!changed || allocs != 1 || !strings.Contains(code, "\tframe_size := 1280 + 768\n")):
			t.Errorf("%s: %s takes %d TLS frames (changed: %v), want one of 1280 + 768 bytes", name, oneFrameFunc, allocs, changed)
		case !oneFrameFiles[name] && (changed || allocs != 2):
			t.Errorf("%s: %s takes %d TLS frames (changed: %v), want 2 and no change", name, oneFrameFunc, allocs, changed)
		}
		b := oneFrameInput(t, name)
		if bytes.Contains(b, []byte("fixOneFrame")) {
			t.Fatalf("%s: readEngine leaves a change of fixOneFrame", name)
		}
		out, err := fixOneFrame(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if out, err = dropDefers(out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(out, raw) {
			t.Errorf("%s: fixOneFrame does not make the file again", name)
		}
	}
}

// uncommented returns the lines of b that are not comments.
func uncommented(b []byte) string {
	var out strings.Builder
	for _, line := range strings.SplitAfter(string(b), "\n") {
		if !strings.HasPrefix(strings.TrimLeft(line, "\t"), "//") {
			out.WriteString(line)
		}
	}
	return out.String()
}

// fixOneFrame stops where the block that takes the frame again does more
// than the entry did, or the frames are not as fixAlloca writes them.
func TestOneFrameRejects(t *testing.T) {
	name := "ccgo_linux_amd64.go"
	b := oneFrameInput(t, name)
	block := "\t\tbp = tls.Alloc(frame_size)\n\t\t*(*TJSValue)(unsafe.Pointer(bp + 80)) = _func_obj\n\t\tsf = bp + 96\n\t}\n"
	for _, c := range []struct{ name, old, new, want string }{{
		name: "another statement in the block",
		old:  block,
		new:  "\t\tbp = tls.Alloc(frame_size)\n\t\t*(*TJSValue)(unsafe.Pointer(bp + 80)) = _func_obj\n\t\tsf = bp + 96\n\t\tpc = 0\n\t}\n",
		want: `runs "pc = 0"`,
	}, {
		name: "a copy the entry does not make",
		old:  block,
		new:  "\t\tbp = tls.Alloc(frame_size)\n\t\t*(*TJSValue)(unsafe.Pointer(bp + 80)) = _func_obj\n\t\tsf = bp + 112\n\t}\n",
		want: "which the entry does not run once",
	}, {
		name: "another size",
		old:  "\t\tframe_size = 1280 + int(alloca_size)\n",
		new:  "\t\tframe_size = 1296 + int(alloca_size)\n",
		want: "taken again with 1296 bytes",
	}, {
		name: "another tls.Alloc",
		old:  "\trt = (*TJSContext)(unsafe.Pointer(caller_ctx)).Frt\n",
		new:  "\trt = (*TJSContext)(unsafe.Pointer(caller_ctx)).Frt\n\ttls.Free(tls.Alloc(16))\n",
		want: "3 tls.Alloc",
	}, {
		name: "no block",
		old:  "\t\tframe_size = 1280 + int(alloca_size)\n",
		new:  "\t\tframe_size = 1280 + int(alloca_size) + 0\n",
		want: "0 blocks",
	}, {
		// The rest of the function would find another value in the
		// frame than the block put there.
		name: "a parameter copy written again before the block",
		old:  "\tsf = bp + 96\n\tif pollDue(",
		new:  "\tsf = bp + 96\n\t*(*TJSValue)(unsafe.Pointer(bp + 80)) = this_obj1\n\tif pollDue(",
		want: "oneFrameSites",
	}, {
		name: "a pointer into the frame set again before the block",
		old:  "\tsf = bp + 96\n\tif pollDue(",
		new:  "\tsf = bp + 96\n\tsf = rt + 8\n\tif pollDue(",
		want: "oneFrameSites",
	}} {
		t.Run(c.name, func(t *testing.T) {
			in := editFunc(t, b, oneFrameFunc, c.old, c.new)
			_, err := fixOneFrame(in, name)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want %q", err, c.want)
			}
		})
	}
	// A file it does not change, with the change.
	out, err := fixOneFrame(bytes.Clone(b), name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixOneFrame(out, "ccgo_darwin_arm64.go"); err == nil {
		t.Error("a change in a file of darwin taken")
	}
	if _, err := restoreOneFrame(out, "ccgo_darwin_arm64.go"); err == nil || !strings.Contains(err.Error(), "only the files of linux") {
		t.Errorf("a change in a file of darwin undone: %v", err)
	}
	if _, err := fixOneFrame(out, name); err == nil || !strings.Contains(err.Error(), "already") {
		t.Errorf("a second run: %v", err)
	}
}

// restoreOneFrame stops at a change that is not in the form fixOneFrame
// writes.
func TestRestoreOneFrameRejects(t *testing.T) {
	name := "ccgo_linux_amd64.go"
	out, err := fixOneFrame(oneFrameInput(t, name), name)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, old, new, want string }{{
		name: "the head changed",
		old:  "\tframe_size := 1280 + 768\n",
		new:  "\tframe_size := 1280 + 512\n",
		want: "the head",
	}, {
		name: "the comment cut short",
		old:  "\t//}\n",
		new:  "\t}\n",
		want: "ends before its closing brace",
	}} {
		t.Run(c.name, func(t *testing.T) {
			in := editFunc(t, out, oneFrameFunc, c.old, c.new)
			if _, err := restoreOneFrame(in, name); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want %q", err, c.want)
			}
		})
	}
}

// -oneframe prints the fingerprints even when oneFrameSites does not match
// the files, which is when it is needed, and without it a wrong entry stops
// fixlib. The file is one fixOneFrame changes, where the table is checked.
func TestOneFrameReportIgnoresSites(t *testing.T) {
	hw, err := handWritten(filepath.Join("..", "..", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	name := "ccgo_linux_amd64.go"
	fn := filepath.Join("..", "..", "lib", name)
	want := oneFrameSites[name]
	oneFrameSites[name] = "0"
	defer func() { oneFrameSites[name] = want }()
	if _, err := reportedFingerprint(fn, hw, oneFrameFingerprint); err == nil || !strings.Contains(err.Error(), "oneFrameSites") {
		t.Errorf("a wrong entry of oneFrameSites taken: %v", err)
	}
	*oneFrame = true
	got, err := reportedFingerprint(fn, hw, oneFrameFingerprint)
	*oneFrame = false
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("-oneframe gives %s, the table has %s", got, want)
	}
}
