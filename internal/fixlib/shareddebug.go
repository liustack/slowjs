// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var sharedDebugF = flag.Bool("shareddebug", false, "print the fingerprints of the functions fixSharedDebug changes or relies on, after the earlier final rewrites, in the form of sharedDebugSites, to update it after a review")

// sharedDebugHook is what fixSharedDebug adds to JS_ReadObject, after the
// line that sets allow_reference from the flags.
const (
	sharedDebugFlagLine = "\tlibc.SetBitFieldPtr32Int32(s+52, libc.BoolInt32(flags&(libc.Int32FromInt32(1)<<libc.Int32FromInt32(3)) != libc.Int32FromInt32(0)), 24, 0xff000000)\n"
	sharedDebugHook     = "\t// Added by internal/fixlib: read the debug information shared or not (lib/shareddebug.go).\n\treadSharedDebug(ctx, flags)\n"
)

// sharedDebugEdits are the changes of fixSharedDebug, each of the text old,
// once in the function fn, into new. Each new is undone into its old.
var sharedDebugEdits = []struct{ fn, old, new string }{
	{"XJS_ReadObject", sharedDebugFlagLine, sharedDebugFlagLine + sharedDebugHook},
	{"_JS_ReadFunctionTag", ".Fdebug.Fpc2line_buf = Xjs_mallocz(tls, ctx1, ", ".Fdebug.Fpc2line_buf = debugAlloc(tls, s, b, "},
	{"_JS_ReadFunctionTag", "if _bc_get_buf(tls, s, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fpc2line_buf, ", "if debugGetBuf(tls, s, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fpc2line_buf, "},
	{"_JS_ReadFunctionTag", ".Fdebug.Fsource = Xjs_mallocz(tls, ctx1, ", ".Fdebug.Fsource = debugAlloc(tls, s, b, "},
	{"_JS_ReadFunctionTag", "if _bc_get_buf(tls, s, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fsource, ", "if debugGetBuf(tls, s, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fsource, "},
	{"_free_function_bytecode", "Xjs_free_rt(tls, rt1, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fpc2line_buf)", "debugFree(tls, rt1, b, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fpc2line_buf)"},
	{"_free_function_bytecode", "Xjs_free_rt(tls, rt1, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fsource)", "debugFree(tls, rt1, b, (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fsource)"},
	{"_compute_bytecode_size", "if (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fsource != 0 {", "if (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fsource != 0 && !debugShared(b) {"},
	{"_compute_bytecode_size", "if (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fpc2line_len != 0 {", "if (*TJSFunctionBytecode)(unsafe.Pointer(b)).Fdebug.Fpc2line_len != 0 && !debugShared(b) {"},
}

// sharedDebugCalls are the functions of lib/shareddebug.go that the edits
// call. None of them may be called anywhere else.
var sharedDebugCalls = []string{"readSharedDebug(", "debugAlloc(", "debugGetBuf(", "debugFree(", "debugShared("}

var (
	// A use of the source text or line table of a function: the fields by
	// name, or their addresses as ccgo writes them.
	reSharedDebugUse = regexp.MustCompile(`\.Fdebug\.(?:Fsource|Fpc2line_buf)\b|\+96\+(?:16|24)\b`)
	// A read or a write of the fifth bit of the second byte of flags of
	// JSFunctionBytecode, which shared_debug takes.
	reSharedDebugBit = regexp.MustCompile(`\+ ?18\)\)&0x20>>5|\+18, [^\n]*, 5, 0x20\)`)

	// sharedDebugUsers are the functions that use the source text or line
	// table of a function. Each of them only reads them, or is changed by
	// fixSharedDebug: a function that starts to use them must be read against
	// lib/shareddebug.go first, as it may free, move or write them.
	sharedDebugUsers = []string{"_JS_ReadFunctionTag", "_JS_WriteFunctionTag", "_compute_bytecode_size", "_find_line_num", "_free_function_bytecode", "_js_create_function", "_js_function_toString"}

	// sharedDebugFuncs are the functions fixSharedDebug changes or relies
	// on: the users above, JS_ReadObject, and the reader functions
	// lib/shareddebug.go calls or follows.
	sharedDebugFuncs = append([]string{"XJS_ReadObject", "_bc_get_buf", "_bc_read_error_end"}, sharedDebugUsers...)
)

// sharedDebugFingerprint is the fingerprint of sharedDebugFuncs in b, which
// has none of the change.
func sharedDebugFingerprint(b []byte) (string, error) {
	h := sha256.New()
	for _, name := range sharedDebugFuncs {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return "", err
		}
		h.Write(body)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}

// sharedDebugUsersOf returns the functions of b that use the source text or
// line table of a function, sorted.
func sharedDebugUsersOf(b []byte) []string {
	var names []string
	reFunc := regexp.MustCompile(`\nfunc ([A-Za-z_0-9]+)\(`)
	for _, loc := range reFunc.FindAllSubmatchIndex(b, -1) {
		text := b[loc[0]+1:]
		if end := bytes.Index(text, []byte("\n}\n")); end >= 0 {
			text = text[:end+2]
		}
		if reSharedDebugUse.Match(text) {
			names = append(names, string(b[loc[2]:loc[3]]))
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// fixSharedDebug lets JS_ReadObject read the source text and line table of
// each function as pointers into the buffer it reads, with the flag
// ReadObjSharedDebug of lib/shareddebug.go: JS_ReadObject passes its flags
// to readSharedDebug, JS_ReadFunctionTag takes the two from debugAlloc and
// debugGetBuf in place of js_mallocz and bc_get_buf, free_function_bytecode
// frees them with debugFree, and compute_bytecode_size does not count them
// when debugShared says they are shared.
//
// The text of sharedDebugFuncs is pinned in sharedDebugSites, every function
// that uses the two fields must be one of sharedDebugUsers, and no generated
// code may use the bit shared_debug takes: when a release changes any of
// that, fixSharedDebug stops, and the new code must be read against
// lib/shareddebug.go before the lists are updated (go run ./internal/fixlib
// -shareddebug prints the fingerprints).
//
// Its exact inverse runs before those of the earlier final changes.
func fixSharedDebug(b []byte, name string) ([]byte, error) {
	bare, err := restoreSharedDebug(b, name)
	if err != nil {
		return nil, err
	}
	got, err := sharedDebugFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if want := sharedDebugSites[name]; got != want {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/shareddebug.go and docs/fork-changes/memory-and-allocation.md, then update sharedDebugSites (go run ./internal/fixlib -shareddebug)", name, strings.Join(sharedDebugFuncs, ", "), got, want)
	}
	if users := sharedDebugUsersOf(bare); !slices.Equal(users, sharedDebugUsers) {
		return nil, fmt.Errorf("%s: the functions that use the source text or line table of a function are %s, want %s: read the new ones against lib/shareddebug.go, a shared one must not be freed or written", name, strings.Join(users, ", "), strings.Join(sharedDebugUsers, ", "))
	}
	if m := reSharedDebugBit.Find(bare); m != nil {
		return nil, fmt.Errorf("%s: %q uses the bit of JSFunctionBytecode that shared_debug takes (see lib/shareddebug.go)", name, m)
	}
	out := bare
	for _, e := range sharedDebugEdits {
		body, err := funcBody(out, funcHead(e.fn), e.fn)
		if err != nil {
			return nil, err
		}
		if n := bytes.Count(body, []byte(e.old)); n != 1 {
			return nil, fmt.Errorf("fixSharedDebug: %s: %q %d times, want once", e.fn, firstLine(e.old), n)
		}
		out = bytes.Replace(out, body, bytes.Replace(body, []byte(e.old), []byte(e.new), 1), 1)
	}
	if back, err := restoreSharedDebug(out, name); err != nil || !bytes.Equal(back, bare) {
		return nil, fmt.Errorf("fixSharedDebug: restoreSharedDebug does not give the input back (%v)", err)
	}
	if !bytes.Equal(bare, b) && !bytes.Equal(out, b) {
		return nil, fmt.Errorf("fixSharedDebug: the file has a part of the change")
	}
	return out, nil
}

// restoreSharedDebug undoes fixSharedDebug.
func restoreSharedDebug(b []byte, _ string) ([]byte, error) {
	for i := len(sharedDebugEdits) - 1; i >= 0; i-- {
		e := sharedDebugEdits[i]
		switch n := bytes.Count(b, []byte(e.new)); n {
		case 0:
		case 1:
			body, err := funcBody(b, funcHead(e.fn), e.fn)
			if err != nil {
				return nil, err
			}
			if !bytes.Contains(body, []byte(e.new)) {
				return nil, fmt.Errorf("restoreSharedDebug: %q is outside %s", firstLine(e.new), e.fn)
			}
			b = bytes.Replace(b, body, bytes.Replace(body, []byte(e.new), []byte(e.old), 1), 1)
		default:
			return nil, fmt.Errorf("restoreSharedDebug: %d copies of %q", n, firstLine(e.new))
		}
	}
	for _, call := range sharedDebugCalls {
		if bytes.Contains(b, []byte(call)) {
			return nil, fmt.Errorf("restoreSharedDebug: %s is called where fixSharedDebug does not put it", strings.TrimSuffix(call, "("))
		}
	}
	return b, nil
}

func sharedDebugReport(read func(string) ([]byte, error)) error {
	files, err := filepath.Glob("lib/ccgo_*.go")
	if err != nil {
		return err
	}
	fmt.Println("var sharedDebugSites = map[string]string{")
	for _, fn := range files {
		b, err := read(fn)
		if err != nil {
			return err
		}
		name := filepath.Base(fn)
		// The final changes before fixSharedDebug.
		for _, fix := range []func([]byte, string) ([]byte, error){fixExpect, fixFastFrames, fixEphemeron, fixYoungGC} {
			if b, err = fix(b, name); err != nil {
				return err
			}
		}
		sum, err := sharedDebugFingerprint(b)
		if err != nil {
			return err
		}
		fmt.Printf("\t%q: %q,\n", name, sum)
	}
	fmt.Println("}")
	return nil
}
