// Copyright (c) 2026 The slowjs Authors.
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

// Every TLS frame of the files of linux but those of fastFrameSkip is taken
// and freed through the fast path, the other files keep TLS.Alloc and
// TLS.Free, and fixFastFrames makes each file again from what
// restoreFastFrames gives.
func TestFastFramesEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		name := filepath.Base(fn)
		raw, err := os.ReadFile(fn)
		if err != nil {
			t.Fatal(err)
		}
		b, err := restoreFastFrames(raw, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		takes, drops := bytes.Count(raw, []byte("tlsTake(tls, ")), bytes.Count(raw, []byte("if tlsDrop(tls, "))
		if !fastFrameFile(name) {
			if takes != 0 || drops != 0 || !bytes.Equal(b, raw) {
				t.Errorf("%s: changed by fixFastFrames", name)
			}
			continue
		}
		// The functions of fastFrameSkip are as they were, and the counts
		// below are of the rest of the file.
		rest, restRaw := b, raw
		for _, fn := range fastFrameSkip {
			f, err := funcBody(b, funcHead(fn), fn)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if !bytes.Contains(raw, f) || len(reTLSCall.FindAll(f, -1)) == 0 {
				t.Errorf("%s: %s changed by fixFastFrames, or without a TLS frame", name, fn)
			}
			rest, restRaw = bytes.Replace(rest, f, nil, 1), bytes.Replace(restRaw, f, nil, 1)
		}
		allocs, frees := len(reTLSAllocTo.FindAll(rest, -1))+len(reTLSAllocAll.FindAll(rest, -1)), len(reTLSFree.FindAll(rest, -1))
		if takes != allocs || drops != frees || allocs < 600 || frees < 2000 {
			t.Errorf("%s: %d frames taken and %d freed through the fast path, of %d and %d", name, takes, drops, allocs, frees)
		}
		// What is left of TLS.Alloc is the call when the next slot is too
		// small, and of TLS.Free none.
		if n := len(reTLSCall.FindAll(restRaw, -1)); n != allocs {
			t.Errorf("%s: %d calls of tls.Alloc and tls.Free, want the %d of the slow path", name, n, allocs)
		}
		out, err := fixFastFrames(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(out, raw) {
			t.Errorf("%s: fixFastFrames does not make the file again", name)
		}
	}
}

// fixFastFrames stops at a tls.Alloc or tls.Free that is not a statement
// of a form it knows, and at a file that has its change already, and
// restoreFastFrames at the fast path outside linux or in a form
// fixFastFrames does not write.
func TestFastFramesRejects(t *testing.T) {
	name := "ccgo_linux_amd64.go"
	b, err := readNoDefer(filepath.Join("..", "..", "lib", name))
	if err != nil {
		t.Fatal(err)
	}
	head := "\nfunc _js_realloc_array(tls *libc.TLS, ctx uintptr, parray uintptr, elem_size int32, psize uintptr, req_size int32) (r int32) {\n\tbp := tls.Alloc(16)\n"
	if bytes.Count(b, []byte(head)) != 1 {
		t.Fatalf("%q not found once", head)
	}
	for _, c := range []struct{ name, new, want string }{{
		name: "an alloc in an expression",
		new:  "\nfunc _js_realloc_array(tls *libc.TLS, ctx uintptr, parray uintptr, elem_size int32, psize uintptr, req_size int32) (r int32) {\n\tbp := tls.Alloc(16) + 0\n",
		want: "statements of a form it knows",
	}, {
		name: "a free in an expression",
		new:  head + "\t_ = func() int { tls.Free(16); return 0 }()\n",
		want: "statements of a form it knows",
	}, {
		name: "a size with a call",
		new:  "\nfunc _js_realloc_array(tls *libc.TLS, ctx uintptr, parray uintptr, elem_size int32, psize uintptr, req_size int32) (r int32) {\n\tbp := tls.Alloc(int(unsafe.Sizeof(x)))\n",
		want: "statements of a form it knows",
	}, {
		name: "the change made already",
		new:  head + "\ttlsTake(tls, 16)\n",
		want: "restoreFastFrames goes first",
	}} {
		t.Run(c.name, func(t *testing.T) {
			in := bytes.Replace(bytes.Clone(b), []byte(head), []byte(c.new), 1)
			_, err := fixFastFrames(in, name)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want an error with %q", err, c.want)
			}
		})
	}
	fixed, err := fixFastFrames(bytes.Clone(b), name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoreFastFrames(fixed, "ccgo_darwin_arm64.go"); err == nil || !strings.Contains(err.Error(), "only the files of linux") {
		t.Errorf("restoreFastFrames outside linux: %v", err)
	}
	odd := bytes.Replace(fixed, []byte("\tif tlsDrop(tls, 16) {\n\t\ttlsSignals(tls)\n\t}\n"), []byte("\tif tlsDrop(tls, 16) {\n\t}\n"), 1)
	if _, err := restoreFastFrames(odd, name); err == nil || !strings.Contains(err.Error(), "in a form fixFastFrames does not write") {
		t.Errorf("restoreFastFrames of a drop without its check: %v", err)
	}
	odd = bytes.Replace(fixed, []byte("\tbp := tlsTake(tls, 16)\n\tif bp == 0 {\n\t\tbp = tls.Alloc(16)\n"), []byte("\tbp := tlsTake(tls, 16)\n\tif bp == 0 {\n\t\tbp = tls.Alloc(32)\n"), 1)
	if _, err := restoreFastFrames(odd, name); err == nil || !strings.Contains(err.Error(), "not as fixFastFrames writes it") {
		t.Errorf("restoreFastFrames of a slow path of another size: %v", err)
	}
}

// The text of libc that lib/fastframes_linux.go relies on is the one
// reviewed, and a change to any of it changes the fingerprint.
func TestFastFrameSites(t *testing.T) {
	got, err := fastFrameFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if got != fastFrameSites {
		t.Fatalf("the text of libc has the fingerprint %s, fastFrameSites has %s", got, fastFrameSites)
	}
	src := "package libc\n\n"
	for _, head := range fastFrameText {
		src += head + "\n\tx\n}\n\n"
	}
	fp, err := libcTextFingerprint([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	for _, head := range fastFrameText {
		other, err := libcTextFingerprint([]byte(strings.Replace(src, head+"\n\tx\n", head+"\n\ty\n", 1)))
		if err != nil || other == fp {
			t.Errorf("%q changed: fingerprint %s (%v), unchanged %s", head, other, err, fp)
		}
		if _, err := libcTextFingerprint([]byte(strings.Replace(src, head, head[:len(head)-2]+"x {", 1))); err == nil {
			t.Errorf("%q missing: no error", head)
		}
	}
}
