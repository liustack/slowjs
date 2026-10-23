// Copyright (c) 2026 The liustack/quickjs Authors.
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

var youngF = flag.Bool("young", false, "print the fingerprints of the functions fixYoungGC changes or relies on, after the earlier final rewrites, in the form of youngSites, to update it after a review")

// The text fixYoungGC adds.
const (
	youngAddGC    = "\t// Added by internal/fixlib: a new GC object is young (lib/younggc.go).\n\tv1 = youngHead(rt)\n"
	youngInitHook = "\t// Added by internal/fixlib: set up the young list (lib/younggc.go).\n\tyoungInit(rt)\n"
	youngSplice   = "\t// Added by internal/fixlib: the young objects become old first (lib/younggc.go).\n\tyoungSplice(rt)\n"
	// JS_DumpMemoryUsage takes no runtime as well.
	youngSpliceIf = "\t// Added by internal/fixlib: the young objects become old first (lib/younggc.go).\n\tif rt != 0 {\n\t\tyoungSplice(rt)\n\t}\n"
	youngDeferDoc = "\t\t\t// Added by internal/fixlib: an object only the garbage of a young collection held (lib/younggc.go).\n"
)

var (
	// The list of every GC object, gc_obj_list, read where _gc_scan starts.
	reYoungOldList = regexp.MustCompile(`\n\tel = \(\*Tlist_head\)\(unsafe\.Pointer\(rt \+ (\d+)\)\)\.Fnext\n`)
	// A function and its text up to the next one.
	reYoungFunc = regexp.MustCompile(`\nfunc ([A-Za-z_0-9]+)\(`)
	// The object __JS_FreeValueRT frees when its count gets to 0.
	reYoungFreeObj = regexp.MustCompile(`\n\t\t(p\d+) = [^\n]*\n\t\tif TJSGCPhaseEnum\(\*\(\*uint8\)\(unsafe\.Pointer\(rt1 \+ \d+\)\)&0xff>>0\) != int32\(EJS_GC_PHASE_REMOVE_CYCLES\) \{\n`)
	reYoungDefer   = regexp.MustCompile(` else \{\n` + regexp.QuoteMeta(youngDeferDoc) + `\t\t\tyoungDefer\(tls, (rt1?), ([a-z0-9]+)\)\n\t\t\}`)

	// The functions lib/younggc.go repeats or relies on, and the ones
	// fixYoungGC changes, as they are after the earlier final changes: the
	// collector and its weak pass, what marks children, and what frees an
	// object at a count of 0, in or out of a collection.
	youngFuncs = []string{"_add_gc_object", "_JS_RunGCInternal", "x__JS_FreeValueRT", "_async_func_free", "_mark_children", "XJS_MarkValue", "_js_map_mark", "_gc_decref", "_gc_decref_child", "_gc_scan", "_gc_scan_incref_child", "_gc_scan_incref_child2", "_gc_free_cycles", "_free_zero_refcount", "_gc_remove_weak_objects", "_free_gc_object", "_free_object", "_free_function_bytecode", "_free_var_ref", "_js_weakref_free", "_js_weakref_is_live", "XJS_RunGC"}

	// youngListReaders are the functions that use gc_obj_list. Each of them
	// either belongs to the collector, sees the young list moved into
	// gc_obj_list first, or is never called: a function that starts to use
	// gc_obj_list must be read against lib/younggc.go first.
	youngListReaders = []string{"XJS_ComputeMemoryUsage", "XJS_DumpMemoryUsage", "XJS_NewRuntime2", "_JS_DumpShapes", "_add_gc_object", "_gc_decref", "_gc_scan", "_gc_scan_incref_child"}

	// youngSpliced are the functions that move the young list into
	// gc_obj_list at their start, with the text that does it.
	youngSpliced = []struct{ name, text string }{
		{"_JS_RunGCInternal", youngSplice},
		{"XJS_ComputeMemoryUsage", youngSplice},
		{"XJS_DumpMemoryUsage", youngSpliceIf},
	}
)

// youngOldList returns the offset of gc_obj_list in JSRuntime in b.
func youngOldList(b []byte) (string, error) {
	body, err := funcBody(b, funcHead("_gc_scan"), "_gc_scan")
	if err != nil {
		return "", err
	}
	m := reYoungOldList.FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("fixYoungGC: _gc_scan does not start at gc_obj_list")
	}
	return string(m[1]), nil
}

// youngReaders returns the functions of b that use gc_obj_list, at offset
// off of JSRuntime.
func youngReaders(b []byte, off string) []string {
	re := regexp.MustCompile(`\brt1? \+ ` + off + `\b|\.Fgc_obj_list\b`)
	locs := reYoungFunc.FindAllSubmatchIndex(b, -1)
	var names []string
	for i, loc := range locs {
		end := len(b)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		if re.Match(b[loc[0]:end]) {
			names = append(names, string(b[loc[2]:loc[3]]))
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

// youngFingerprint is the fingerprint of youngFuncs in b, which has none of
// the change.
func youngFingerprint(b []byte) (string, error) {
	h := sha256.New()
	for _, name := range youngFuncs {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return "", err
		}
		h.Write(body)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}

// fixYoungGC wires the young collection of lib/younggc.go into the engine:
// add_gc_object puts a new GC object on the young list, which JS_NewRuntime2
// sets up, JS_RunGCInternal, JS_ComputeMemoryUsage and JS_DumpMemoryUsage
// move it into gc_obj_list first, and __JS_FreeValueRT and async_func_free
// call youngDefer for an object whose count gets to 0 while the garbage is
// freed.
//
// lib/younggc.go runs the trial deletion, the scan and the sweep of the
// collector on the young list, frees what gets to a count of 0 as these
// functions do, and runs the weak pass of the collector. So the text of
// youngFuncs is pinned in youngSites, and every function that uses
// gc_obj_list must be one of youngListReaders: when a release changes one of
// them, fixYoungGC stops, and the new code must be read against
// lib/younggc.go before the lists are updated (go run ./internal/fixlib
// -young prints the fingerprints).
//
// It is the last final change, and its exact inverse runs first, so that
// the other changes, fixEphemeron among them, see the code without it.
func fixYoungGC(b []byte, name string) ([]byte, error) {
	bare, err := restoreYoungGC(b, name)
	if err != nil {
		return nil, err
	}
	got, err := youngFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if want := youngSites[name]; got != want {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/younggc.go and docs/fork-changes/young-gc.md, then update youngSites (go run ./internal/fixlib -young)", name, strings.Join(youngFuncs, ", "), got, want)
	}
	off, err := youngOldList(bare)
	if err != nil {
		return nil, err
	}
	if readers := youngReaders(bare, off); !slices.Equal(readers, youngListReaders) {
		return nil, fmt.Errorf("%s: the functions that use gc_obj_list are %s, want %s: read the new ones against lib/younggc.go, they may need the young objects too", name, strings.Join(readers, ", "), strings.Join(youngListReaders, ", "))
	}
	out, err := addYoungGC(bare, off)
	if err != nil {
		return nil, err
	}
	if back, err := restoreYoungGC(out, name); err != nil || !bytes.Equal(back, bare) {
		return nil, fmt.Errorf("fixYoungGC: restoreYoungGC does not give the input back (%v)", err)
	}
	if !bytes.Equal(bare, b) && !bytes.Equal(out, b) {
		return nil, fmt.Errorf("fixYoungGC: the file has a part of the change")
	}
	return out, nil
}

// youngOldAdd is the statement of add_gc_object that picks gc_obj_list.
func youngOldAdd(off string) string { return "\tv1 = rt + " + off + "\n" }

// youngOldInit is where JS_NewRuntime2 sets up gc_obj_list.
func youngOldInit(off string) string {
	return youngOldAdd(off) + "\t(*Tlist_head)(unsafe.Pointer(v1)).Fprev = v1\n\t(*Tlist_head)(unsafe.Pointer(v1)).Fnext = v1\n"
}

// youngReplaceIn replaces the one old in the function name of b by new.
func youngReplaceIn(b []byte, name, old, new string) ([]byte, error) {
	body, err := funcBody(b, funcHead(name), name)
	if err != nil {
		return nil, err
	}
	if n := bytes.Count(body, []byte(old)); n != 1 {
		return nil, fmt.Errorf("fixYoungGC: %s: %q %d times, want once", name, firstLine(old), n)
	}
	return bytes.Replace(b, body, bytes.Replace(body, []byte(old), []byte(new), 1), 1), nil
}

// addYoungGC makes the change in b, which has none of it, off being the
// offset of gc_obj_list.
func addYoungGC(b []byte, off string) ([]byte, error) {
	var err error
	if b, err = youngReplaceIn(b, "_add_gc_object", youngOldAdd(off), youngAddGC); err != nil {
		return nil, err
	}
	if b, err = youngReplaceIn(b, "XJS_NewRuntime2", youngOldInit(off), youngOldInit(off)+youngInitHook); err != nil {
		return nil, err
	}
	for _, s := range youngSpliced {
		loc := findAll(funcHead(s.name), b)
		if len(loc) != 1 {
			return nil, fmt.Errorf("fixYoungGC: want one %s", s.name)
		}
		b = splice(b, loc[0][1], s.text)
	}

	body, err := funcBody(b, funcHead("x__JS_FreeValueRT"), "x__JS_FreeValueRT")
	if err != nil {
		return nil, err
	}
	m := reYoungFreeObj.FindAllSubmatch(body, -1)
	if len(m) != 1 {
		return nil, fmt.Errorf("fixYoungGC: x__JS_FreeValueRT: want one object freed at the end of a REMOVE_CYCLES test, found %d", len(m))
	}
	const freeEnd = "\t\t\t}\n\t\t}\n\tcase libc.Uint32FromInt32(EJS_TAG_BIG_INT):\n"
	if b, err = youngReplaceIn(b, "x__JS_FreeValueRT", freeEnd, youngDeferText("rt1", string(m[0][1]), freeEnd)); err != nil {
		return nil, err
	}
	const asyncEnd = "\t\t\t}\n\t\t}\n\t}\n}\n"
	if b, err = youngReplaceIn(b, "_async_func_free", asyncEnd, youngDeferText("rt", "s", asyncEnd)); err != nil {
		return nil, err
	}
	return b, nil
}

// youngDeferText is end, the end of the test of the phase REMOVE_CYCLES of
// a function that frees p of the runtime rt at a count of 0, with the call
// of youngDefer in an else branch.
func youngDeferText(rt, p, end string) string {
	return "\t\t\t}\n\t\t} else {\n" + youngDeferDoc + "\t\t\tyoungDefer(tls, " + rt + ", " + p + ")\n\t\t}\n" + strings.TrimPrefix(end, "\t\t\t}\n\t\t}\n")
}

// restoreYoungGC undoes fixYoungGC.
func restoreYoungGC(b []byte, _ string) ([]byte, error) {
	off, err := youngOldList(b)
	if err != nil {
		return nil, err
	}
	for _, r := range []struct{ new, old string }{
		{youngAddGC, youngOldAdd(off)},
		{youngInitHook, ""},
	} {
		if n := bytes.Count(b, []byte(r.new)); n > 1 {
			return nil, fmt.Errorf("restoreYoungGC: %d copies of %q", n, firstLine(r.new))
		}
		b = bytes.Replace(b, []byte(r.new), []byte(r.old), 1)
	}
	for _, s := range youngSpliced {
		loc := findAll(funcHead(s.name), b)
		if len(loc) == 1 && bytes.HasPrefix(b[loc[0][1]:], []byte(s.text)) {
			b = slices.Delete(slices.Clone(b), loc[0][1], loc[0][1]+len(s.text))
		}
	}
	if n := len(reYoungDefer.FindAllIndex(b, -1)); n > 2 {
		return nil, fmt.Errorf("restoreYoungGC: %d calls of youngDefer, want 2", n)
	}
	b = reYoungDefer.ReplaceAll(b, nil)
	for _, name := range []string{"youngHead(", "youngInit(", "youngSplice(", "youngDefer("} {
		if bytes.Contains(b, []byte(name)) {
			return nil, fmt.Errorf("restoreYoungGC: %s is called where fixYoungGC does not put it", strings.TrimSuffix(name, "("))
		}
	}
	return b, nil
}

func youngReport(read func(string) ([]byte, error)) error {
	files, err := filepath.Glob("lib/ccgo_*.go")
	if err != nil {
		return err
	}
	fmt.Println("var youngSites = map[string]string{")
	for _, fn := range files {
		b, err := read(fn)
		if err != nil {
			return err
		}
		name := filepath.Base(fn)
		// The final changes before fixYoungGC.
		for _, fix := range []func([]byte, string) ([]byte, error){fixExpect, fixFastFrames, fixEphemeron} {
			if b, err = fix(b, name); err != nil {
				return err
			}
		}
		sum, err := youngFingerprint(b)
		if err != nil {
			return err
		}
		fmt.Printf("\t%q: %q,\n", name, sum)
	}
	fmt.Println("}")
	return nil
}
