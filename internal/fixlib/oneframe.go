// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// fixAlloca puts the room of a small alloca at the end of the TLS frame of
// JS_CallInternal: the function takes a frame of N bytes at its entry, and
// a javascript function frees it and takes N plus the room again right
// before the stack check (see fixAlloca). Every call of a javascript
// function so takes two TLS frames, one after the other.
//
// fixOneFrame makes JS_CallInternal take N plus allocaMax bytes at its
// entry instead, and drops the second frame: the block that frees the
// frame and takes it again becomes a comment. The rest of the function is
// what ran before for an alloca larger than allocaMax, which skips that
// block too: the parameter copies and the pointers into the frame made at
// the entry stay where they are, and the room of a small alloca is still
// bp + N, now inside the frame the entry took. Each call holds one TLS
// stack slot at every point of its run, as before, so the stack check sees
// the same count, and the depth a script reaches is the same.
//
// It changes only the files of linux, where libc keeps one block for each
// slot of the TLS stack and rounds it up to a power of two, and only when
// that power of two for N already holds N plus allocaMax: then a slot of
// JS_CallInternal holds the same block as before, whatever the alloca (2 KB
// for N = 1280 on 64-bit targets). On 32-bit targets (N = 160) a block
// would grow from 256 or 512 bytes to 1 KB, and elsewhere the TLS stack is
// one run of bytes that a larger frame would use up faster, so those files
// are left as they are. Under the libc.membrk and libc.memgrind tags
// Xmalloc_usable_size gives the size asked for, not a power of two, so
// there a slot of JS_CallInternal holds a block of 1280 + 768 bytes from
// the entry on, where it held 1280 bytes before until a call with an
// alloca grew it.
//
// What the block made again is what the rest of the function finds in
// the frame, so the text between the entry and the block must not change
// it: that text is pinned by a fingerprint in oneFrameSites (see
// oneFrameFingerprint), checked in the files fixOneFrame changes.
//
// Other changes match JS_CallInternal as fixAlloca writes it, so
// fixOneFrame comes last before fixNoDefer, and restoreOneFrame, its exact
// inverse, comes first. fixOneFrame checks that restoreOneFrame gives its
// input back.

const (
	oneFrameFunc      = "_JS_CallInternal"
	oneFrameHeadMark  = "// Changed by internal/fixlib: the frame has the room of a small alloca from the entry (see fixOneFrame).\n"
	oneFrameBlockMark = "// Changed by internal/fixlib: the frame has the room already, so it is not taken again (see fixOneFrame).\n"
)

var (
	reOneFrameHead  = regexp.MustCompile(`\n\t` + allocaFrame + ` := (\d+)\n\tvar ` + allocaMalloc + ` uintptr\n\tbp := tls\.Alloc\(` + allocaFrame + `\)\n`)
	reOneFrameBlock = regexp.MustCompile(`\n\tif alloca_size <= ` + strconv.Itoa(allocaMax) + ` \{\n\t\ttls\.Free\(` + allocaFrame + `\)\n\t\t` + allocaFrame + ` = (\d+) \+ int\(alloca_size\)\n\t\tbp = tls\.Alloc\(` + allocaFrame + `\)\n((?:\t\t[^\n]*\n)*?)\t\}\n`)
	// The statements the block runs again on the new frame: the copy of a
	// parameter into it, and a pointer into it.
	reOneFrameRedo = regexp.MustCompile(`^\t\t(\*\(\*\w+\)\(unsafe\.Pointer\(bp \+ \d+\)\) = \w+|\w+ = bp \+ \d+)$`)
	reOneFrameDone = regexp.MustCompile(`\n\t` + regexp.QuoteMeta(oneFrameHeadMark) + `\t` + allocaFrame + ` := (\d+) \+ ` + strconv.Itoa(allocaMax) + `\n`)
)

// oneFrameFile reports whether fixOneFrame changes the generated file name.
func oneFrameFile(name string) bool {
	return strings.HasPrefix(name, "ccgo_linux_")
}

// nextPow2 returns the smallest power of two not below n.
func nextPow2(n int) int {
	p := 1
	for p < n {
		p *= 2
	}
	return p
}

// fixOneFrame makes the change in b, the generated file name.
func fixOneFrame(b []byte, name string) ([]byte, error) {
	if !oneFrameFile(name) {
		if bytes.Contains(b, []byte("(see fixOneFrame)")) {
			return nil, fmt.Errorf("fixOneFrame: a change in a file it does not change")
		}
		return b, nil
	}
	body, err := funcBody(b, funcHead(oneFrameFunc), oneFrameFunc)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte("fixOneFrame")) {
		return nil, fmt.Errorf("%s: changed by fixOneFrame already", oneFrameFunc)
	}
	heads := reOneFrameHead.FindAllSubmatchIndex(body, -1)
	if len(heads) != 1 {
		return nil, fmt.Errorf("%s: %d frames taken as fixAlloca writes them, want 1", oneFrameFunc, len(heads))
	}
	n, _ := strconv.Atoi(string(body[heads[0][2]:heads[0][3]]))
	if nextPow2(n) < n+allocaMax {
		return b, nil // the block of a slot would grow
	}
	blocks := reOneFrameBlock.FindAllSubmatchIndex(body, -1)
	if len(blocks) != 1 {
		return nil, fmt.Errorf("%s: %d blocks that take the frame again, want 1", oneFrameFunc, len(blocks))
	}
	blk := blocks[0]
	if m, _ := strconv.Atoi(string(body[blk[2]:blk[3]])); m != n {
		return nil, fmt.Errorf("%s: the frame is taken again with %d bytes and the room, at the entry with %d", oneFrameFunc, m, n)
	}
	if c := bytes.Count(body, []byte("tls.Alloc(")); c != 2 {
		return nil, fmt.Errorf("%s: %d tls.Alloc, want the entry and the block", oneFrameFunc, c)
	}
	// Each statement the block runs again must be one the entry ran, before
	// the block: nothing else is in the frame yet.
	before := body[:blk[0]]
	for _, line := range strings.Split(strings.TrimSuffix(string(body[blk[4]:blk[5]]), "\n"), "\n") {
		if line == "" {
			continue
		}
		if !reOneFrameRedo.MatchString(line) {
			return nil, fmt.Errorf("%s: the block that takes the frame again runs %q", oneFrameFunc, strings.TrimSpace(line))
		}
		if bytes.Count(before, []byte("\n"+line[1:]+"\n")) != 1 {
			return nil, fmt.Errorf("%s: the block that takes the frame again runs %q, which the entry does not run once", oneFrameFunc, strings.TrimSpace(line))
		}
	}
	// Nothing between the entry and the block may write again what the
	// block made: the text there is the one reviewed. It is read without
	// the changes made after fixInline, as the report prints it.
	base, err := restoreLast(b)
	if err == nil {
		base, err = restoreInline(base)
	}
	if err != nil {
		return nil, fmt.Errorf("fixOneFrame: %v", err)
	}
	got, err := oneFrameFingerprint(base)
	if err != nil {
		return nil, err
	}
	if want := oneFrameSites[name]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: the text between the entry and the block that takes the frame again has the fingerprint %s, oneFrameSites has %q: read it, nothing there may write again the parameter copies and the pointers into the frame the block makes, then put the new fingerprints in (go run ./internal/fixlib -oneframe prints them)", oneFrameFunc, got, want)
	}
	// The head, then the block as a comment.
	var out bytes.Buffer
	out.Write(body[:heads[0][0]])
	fmt.Fprintf(&out, "\n\t%s\t%s := %d + %d\n", oneFrameHeadMark, allocaFrame, n, allocaMax)
	out.Write(body[heads[0][0]+len("\n\t"+allocaFrame+" := "+strconv.Itoa(n)+"\n") : blk[0]])
	out.WriteString("\n\t" + oneFrameBlockMark)
	for _, line := range strings.SplitAfter(string(body[blk[0]+1:blk[1]]), "\n") {
		if line != "" {
			out.WriteString("\t//" + line[1:])
		}
	}
	out.Write(body[blk[1]:])
	fixed := bytes.Replace(b, body, out.Bytes(), 1)
	back, err := restoreOneFrame(fixed, name)
	if err != nil {
		return nil, fmt.Errorf("fixOneFrame: %v", err)
	}
	if !bytes.Equal(back, b) {
		return nil, fmt.Errorf("fixOneFrame: restoreOneFrame does not give the input back")
	}
	return fixed, nil
}

// restoreOneFrame undoes fixOneFrame in b, the generated file name. A file
// without its change comes back as it is.
func restoreOneFrame(b []byte, name string) ([]byte, error) {
	if !bytes.Contains(b, []byte("(see fixOneFrame)")) {
		return b, nil
	}
	if !oneFrameFile(name) {
		return nil, fmt.Errorf("restoreOneFrame: %s has a change of fixOneFrame, which changes only the files of linux", name)
	}
	body, err := funcBody(b, funcHead(oneFrameFunc), oneFrameFunc)
	if err != nil {
		return nil, err
	}
	if n, m := bytes.Count(b, []byte("(see fixOneFrame)")), bytes.Count(body, []byte("(see fixOneFrame)")); n != 2 || m != 2 {
		return nil, fmt.Errorf("restoreOneFrame: %d changes of fixOneFrame, %d of them in %s, want 2 and 2", n, m, oneFrameFunc)
	}
	head := reOneFrameDone.FindSubmatchIndex(body)
	if head == nil {
		return nil, fmt.Errorf("restoreOneFrame: the head of %s is not as fixOneFrame writes it", oneFrameFunc)
	}
	var out bytes.Buffer
	out.Write(body[:head[0]])
	fmt.Fprintf(&out, "\n\t%s := %s\n", allocaFrame, body[head[2]:head[3]])
	rest := body[head[1]:]
	i := bytes.Index(rest, []byte("\n\t"+oneFrameBlockMark))
	if i < 0 {
		return nil, fmt.Errorf("restoreOneFrame: the block of %s is not marked where fixOneFrame puts it", oneFrameFunc)
	}
	out.Write(rest[:i+1])
	rest = rest[i+2+len(oneFrameBlockMark):]
	for {
		eol := bytes.IndexByte(rest, '\n')
		if eol < 0 || !bytes.HasPrefix(rest, []byte("\t//")) {
			return nil, fmt.Errorf("restoreOneFrame: the block of %s ends before its closing brace", oneFrameFunc)
		}
		line := rest[3 : eol+1]
		out.WriteString("\t")
		out.Write(line)
		rest = rest[eol+1:]
		if string(line) == "}\n" {
			break
		}
	}
	out.Write(rest)
	if bytes.Contains(out.Bytes(), []byte("fixOneFrame")) {
		return nil, fmt.Errorf("restoreOneFrame: a change of fixOneFrame left after undoing it")
	}
	return bytes.Replace(b, body, out.Bytes(), 1), nil
}

// oneFrameFingerprint returns the fingerprint of the text of
// JS_CallInternal between its entry, which takes the frame, and the block
// that takes it again, in b as fixAlloca writes them.
func oneFrameFingerprint(b []byte) (string, error) {
	body, err := funcBody(b, funcHead(oneFrameFunc), oneFrameFunc)
	if err != nil {
		return "", err
	}
	heads := reOneFrameHead.FindAllIndex(body, -1)
	blocks := reOneFrameBlock.FindAllIndex(body, -1)
	if len(heads) != 1 || len(blocks) != 1 || blocks[0][0] < heads[0][1] {
		return "", fmt.Errorf("%s: %d frames taken as fixAlloca writes them and %d blocks that take the frame again after them, want 1 and 1", oneFrameFunc, len(heads), len(blocks))
	}
	h := sha256.Sum256(body[heads[0][1]:blocks[0][0]])
	return fmt.Sprintf("%x", h)[:16], nil
}
