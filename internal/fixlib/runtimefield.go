// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
)

// The text fixRuntimeField adds.
const (
	forkField = "\t// Added by internal/fixlib: the state of lib/runtime.go (see fixRuntimeField).\n\tFfork TforkRuntime\n"
	forkSize  = "+forkRuntimeSize)"
)

var (
	reRuntimeStruct = regexp.MustCompile(`\ntype TJSRuntime = struct \{\n`)
	// The start of JSRuntime: malloc_ctx first, after the field of size 0
	// that aligns the struct on 32-bit targets.
	reFirstField = regexp.MustCompile(`^\{\n(?:\tF__ccgo_align +\[0\]uint(?:32|64)\n)?\tFmalloc_ctx +TJSMallocContext\n`)
	// The two uses of sizeof(JSRuntime) in JS_NewRuntime2: the allocation
	// and the memset, without the change.
	reRuntimeAlloc = regexp.MustCompile(`\(tls, bp, uint(?:32|64)\((\d+)\)\)\n`)
	reRuntimeClear = regexp.MustCompile(`\n\tlibc\.X(?:memset|__builtin___memset_chk)\(tls, rt, 0, uint(?:32|64)\((\d+)\)(?:, \^t__predefined_size_t\(0\))?\)\n`)
)

// fixRuntimeField adds the field Ffork, of the type TforkRuntime of
// lib/runtime.go, at the end of JSRuntime, and makes JS_NewRuntime2
// allocate and clear it with the C struct: the size of the C struct N in
// both becomes N+forkRuntimeSize. The fields of the C struct keep their
// offsets, which the generated code writes as numbers.
//
// lib/runtime.go takes the runtime at the address of its malloc_ctx, which
// the allocator passes around, so malloc_ctx must stay the first field. The
// text of JSRuntime and of JS_NewRuntime2, without the change, is pinned in
// runtimeSites: when a release changes either, fixRuntimeField stops, and
// the new code must be read against lib/runtime.go before the fingerprints
// are updated (go run ./internal/fixlib -runtime prints them).
//
// Other changes match JSRuntime and JS_NewRuntime2 as ccgo writes them, so
// fixRuntimeField comes after them, and restoreRuntimeField, its exact
// inverse, comes first.
func fixRuntimeField(b []byte, file string) ([]byte, error) {
	bare, err := restoreRuntimeField(b)
	if err != nil {
		return nil, err
	}
	got, err := runtimeFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if want := runtimeSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: JSRuntime or XJS_NewRuntime2 changed (fingerprint %s, want %s): read them against lib/runtime.go, then update runtimeSites (go run ./internal/fixlib -runtime)", file, got, want)
	}
	if !bytes.Equal(bare, b) {
		out, err := addRuntimeField(bare)
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(out, b) {
			return nil, fmt.Errorf("fixRuntimeField: the file has a part of the change")
		}
		return b, nil
	}
	out, err := addRuntimeField(b)
	if err != nil {
		return nil, err
	}
	if back, err := restoreRuntimeField(out); err != nil || !bytes.Equal(back, b) {
		return nil, fmt.Errorf("fixRuntimeField: restoreRuntimeField does not give the input back (%v)", err)
	}
	return out, nil
}

// addRuntimeField makes the change in b, which has none of it.
func addRuntimeField(b []byte) ([]byte, error) {
	// The field goes after its comment, which gofmt does not align with
	// the fields before it.
	st, err := structBody(b, reRuntimeStruct, "TJSRuntime")
	if err != nil {
		return nil, err
	}
	if !reFirstField.Match(st[bytes.IndexByte(st, '{'):]) {
		return nil, fmt.Errorf("TJSRuntime: malloc_ctx is not the first field")
	}
	fixed := append(bytes.Clone(st[:len(st)-2]), forkField+"}\n"...)
	b = bytes.Replace(b, st, fixed, 1)

	body, err := funcBody(b, funcHead("XJS_NewRuntime2"), "XJS_NewRuntime2")
	if err != nil {
		return nil, err
	}
	var sizes []string
	fixed = body
	for _, u := range []struct {
		re   *regexp.Regexp
		what string
	}{{reRuntimeAlloc, "allocation"}, {reRuntimeClear, "memset"}} {
		loc := u.re.FindAllSubmatchIndex(fixed, -1)
		if len(loc) != 1 {
			return nil, fmt.Errorf("XJS_NewRuntime2: want 1 %s of the runtime, found %d", u.what, len(loc))
		}
		end := loc[0][3]
		sizes = append(sizes, string(fixed[loc[0][2]:end]))
		fixed = append(append(fixed[:end:end], forkSize[:len(forkSize)-1]...), fixed[end:]...)
	}
	if sizes[0] != sizes[1] {
		return nil, fmt.Errorf("XJS_NewRuntime2: allocates %s bytes and clears %s", sizes[0], sizes[1])
	}
	return bytes.Replace(b, body, fixed, 1), nil
}

// restoreRuntimeField undoes fixRuntimeField.
func restoreRuntimeField(b []byte) ([]byte, error) {
	n, m := bytes.Count(b, []byte(forkField)), bytes.Count(b, []byte(forkSize))
	if n == 0 && m == 0 {
		return b, nil
	}
	if n != 1 || m != 2 {
		return nil, fmt.Errorf("restoreRuntimeField: %d fields Ffork and %d sizes with it, want 1 and 2", n, m)
	}
	b = bytes.Replace(b, []byte(forkField), nil, 1)
	return bytes.ReplaceAll(b, []byte(forkSize), []byte(")")), nil
}

// runtimeFingerprint hashes JSRuntime and XJS_NewRuntime2, without the
// change of fixRuntimeField.
func runtimeFingerprint(b []byte) (string, error) {
	bare, err := restoreRuntimeField(b)
	if err != nil {
		return "", err
	}
	st, err := structBody(bare, reRuntimeStruct, "TJSRuntime")
	if err != nil {
		return "", err
	}
	f, err := funcBody(bare, funcHead("XJS_NewRuntime2"), "XJS_NewRuntime2")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write(st)
	h.Write(f)
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}

// structBody returns the text of the struct type re finds in b, from the
// newline before it to the newline after its closing brace.
func structBody(b []byte, re *regexp.Regexp, name string) ([]byte, error) {
	loc := findAll(re, b)
	if len(loc) != 1 {
		return nil, fmt.Errorf("want 1 type %s, found %d", name, len(loc))
	}
	start := loc[0][0]
	end := bytes.Index(b[start:], []byte("\n}\n"))
	if end < 0 {
		return nil, fmt.Errorf("end of %s not found", name)
	}
	return b[start : start+end+3], nil
}

// insertAfterDecls inserts text into the function name of b right after
// the line that ends its declarations, which must come before any other
// statement but the TLS frame and comments.
func insertAfterDecls(b []byte, name, text string) ([]byte, error) {
	body, err := funcBody(b, funcHead(name), name)
	if err != nil {
		return nil, err
	}
	loc := reBlankUse.FindIndex(body)
	if loc == nil {
		return nil, fmt.Errorf("%s: cannot find the end of the declarations", name)
	}
	head := funcHead(name).FindIndex(body)[1]
	for _, line := range bytes.Split(bytes.TrimSuffix(body[head:loc[0]+1], []byte("\n")), []byte("\n")) {
		if len(line) != 0 && !bytes.HasPrefix(line, []byte("\tvar ")) && !bytes.HasPrefix(line, []byte("\tbp := tls.Alloc(")) && !bytes.HasPrefix(line, []byte("\tdefer tls.Free(")) && !bytes.HasPrefix(line, []byte("\t// ")) {
			return nil, fmt.Errorf("%s: a statement before the end of the declarations: %s", name, line)
		}
	}
	return bytes.Replace(b, body, splice(body, loc[1], text), 1), nil
}

// The line that ends the declarations of a function.
var reBlankUse = regexp.MustCompile(`\n\t_(?:, _)* = [^\n]+\n`)
