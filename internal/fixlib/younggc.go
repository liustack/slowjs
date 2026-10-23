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
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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

// youngReaders audits every use of the numeric offset, regardless of the
// name of its base. ccgo erases C pointer types to uintptr, so an unknown
// base must be reviewed too. Only proven TLS storage or a pointer parameter
// to a different struct can be excluded. A cast at the read site alone is
// not evidence: gc_obj_list itself is read through a Tlist_head cast.
func youngReaders(b []byte, off string) ([]string, error) {
	offset, err := strconv.ParseInt(off, 10, 64)
	if err != nil {
		return nil, err
	}
	// ccgo writes its struct definitions in this form. Aliases and unknown
	// types are deliberately not proof of a different struct.
	otherStruct := map[string]bool{}
	reStruct := regexp.MustCompile(`(?m)^type (\w+) = struct \{(?s:.*?)\n\}`)
	for _, m := range reStruct.FindAllSubmatch(b, -1) {
		otherStruct[string(m[1])] = !bytes.Contains(m[0], []byte("Fgc_obj_list"))
	}
	// Initializers can have the same name. Audit each declaration rather
	// than indexing functions by name.
	globals := map[string]bool{}
	reArray := regexp.MustCompile(`(?m)^var (\w+) = \[\d+\](\w+)\{`)
	for _, m := range reArray.FindAllSubmatch(b, -1) {
		globals[string(m[1])] = otherStruct[string(m[2])]
	}
	var names []string
	reFunc := regexp.MustCompile(`\nfunc ([A-Za-z_0-9]+)\(`)
	for _, loc := range reFunc.FindAllSubmatchIndex(b, -1) {
		text := b[loc[0]+1:]
		if end := bytes.Index(text, []byte("\n}\n")); end >= 0 {
			text = text[:end+2]
		}
		f := &slotFunc{name: string(b[loc[2]:loc[3]]), text: text}

		if !bytes.Contains(f.text, []byte(off)) && !bytes.Contains(f.text, []byte(fmt.Sprintf("0x%x", offset))) && !bytes.Contains(f.text, []byte("Fgc_obj_list")) {
			continue
		}
		if err := f.parse(); err != nil {
			return nil, err
		}
		nonRuntime := youngNonRuntimeBases(f.decl, otherStruct, globals)
		reader := false
		isOffset := func(e ast.Expr) bool {
			n, ok := ast.Unparen(e).(*ast.BasicLit)
			if !ok || n.Kind != token.INT {
				return false
			}
			v, err := strconv.ParseInt(n.Value, 0, 64)
			return err == nil && v == offset
		}
		ast.Inspect(f.decl.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				if n.Sel.Name == "Fgc_obj_list" {
					reader = true
				}
			case *ast.BinaryExpr:
				if n.Op == token.ADD && ((isOffset(n.Y) && !nonRuntime(n.X)) || (isOffset(n.X) && !nonRuntime(n.Y))) {
					reader = true
				}
			case *ast.CallExpr:
				if fun, ok := n.Fun.(*ast.SelectorExpr); ok && youngIdent(fun.X, "unsafe") && fun.Sel.Name == "Add" && len(n.Args) == 2 && isOffset(n.Args[1]) && !nonRuntime(n.Args[0]) {
					reader = true
				}
			}
			return true
		})
		if reader {
			names = append(names, f.name)
		}
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

func youngIdent(e ast.Expr, name string) bool {
	id, ok := ast.Unparen(e).(*ast.Ident)
	return ok && id.Name == name
}

// Prove origins only when every definition has a non-runtime origin and
// the variable has no address escape. This is not a general type inference
// pass, and an unknown origin never exempts a read.
func youngNonRuntimeBases(f *ast.FuncDecl, otherStruct, globals map[string]bool) func(ast.Expr) bool {
	writes := map[string]int{}
	locals := map[string]bool{}
	defs := map[string][]ast.Expr{}
	typed := map[string]bool{}
	tls := map[string]bool{}
	for _, field := range f.Type.Params.List {
		ptr, ok := field.Type.(*ast.StarExpr)
		for _, id := range field.Names {
			writes[id.Name]++
			locals[id.Name] = true
			if !ok {
				continue
			}
			if typ, ok := ptr.X.(*ast.Ident); ok {
				typed[id.Name] = otherStruct[typ.Name]
			}
			if typ, ok := ptr.X.(*ast.SelectorExpr); ok {
				tls[id.Name] = youngIdent(typ.X, "libc") && typ.Sel.Name == "TLS"
			}
		}
	}
	ast.Inspect(f.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
					writes[id.Name]++
					locals[id.Name] = true
					if len(n.Lhs) == len(n.Rhs) && (n.Tok == token.ASSIGN || n.Tok == token.DEFINE) {
						defs[id.Name] = append(defs[id.Name], n.Rhs[i])
					}
				}
			}
		case *ast.ValueSpec:
			for i, id := range n.Names {
				locals[id.Name] = true
				if len(n.Values) == len(n.Names) {
					writes[id.Name]++
					defs[id.Name] = append(defs[id.Name], n.Values[i])
				}
			}
		case *ast.RangeStmt:
			for _, e := range []ast.Expr{n.Key, n.Value} {
				if id, ok := e.(*ast.Ident); ok {
					locals[id.Name] = true
					writes[id.Name] += 2
				}
			}
		case *ast.IncDecStmt:
			if id, ok := n.X.(*ast.Ident); ok {
				writes[id.Name] += 2
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				if id, ok := n.X.(*ast.Ident); ok {
					writes[id.Name] += 2
				}
			}
		}
		return true
	})
	visiting := map[string]bool{}
	var proven func(ast.Expr) bool
	proven = func(e ast.Expr) bool {
		switch e := ast.Unparen(e).(type) {
		case *ast.UnaryExpr:
			if e.Op == token.AND {
				if id, ok := e.X.(*ast.Ident); ok {
					return globals[id.Name] && !locals[id.Name]
				}
			}
		case *ast.Ident:
			if visiting[e.Name] {
				return false
			}
			if typed[e.Name] && writes[e.Name] == 1 {
				return true
			}
			origins := defs[e.Name]
			if len(origins) == 0 || writes[e.Name] != len(origins) {
				return false
			}
			visiting[e.Name] = true
			defer delete(visiting, e.Name)
			for _, origin := range origins {
				if !proven(origin) {
					return false
				}
			}
			return true

		case *ast.CallExpr:
			if fun, ok := e.Fun.(*ast.SelectorExpr); ok {
				if id, ok := fun.X.(*ast.Ident); ok && tls[id.Name] && writes[id.Name] == 1 && fun.Sel.Name == "Alloc" {
					return true
				}
				if youngIdent(fun.X, "unsafe") && fun.Sel.Name == "Pointer" && len(e.Args) == 1 {
					return proven(e.Args[0])
				}
			}
			if youngIdent(e.Fun, "uintptr") && len(e.Args) == 1 {
				return proven(e.Args[0])
			}
		}
		return false
	}
	return proven
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
	readers, err := youngReaders(bare, off)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(readers, youngListReaders) {
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
