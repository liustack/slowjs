// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// The text fixShapeCache adds.
const (
	shapeKeepHook = "\t// Changed by internal/fixlib: keep the shape in the shape cache (see fixShapeCache).\n\tif shapeKeep(tls, rt1, sh) {\n\t\treturn\n\t}\n"
	shapeOffHook  = "\t// Changed by internal/fixlib: let go of the shapes the shape cache keeps (see fixShapeCache).\n\tshapeCacheOff(tls, rt1)\n"

	shapeFoundOld = "\t\t\t(*TJSObject)(unsafe.Pointer(p)).Fshape = _js_dup_shape(tls, new_sh)\n"
	shapeFoundNew = "\t\t\t// Changed by internal/fixlib: a shape found again earns the shape cache a copy (see fixShapeCache).\n\t\t\tshapeFound(tls, ctx)\n" + shapeFoundOld

	shapeCloneOld = "\t\t\tif (*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, sh))).Fref_count != int32(1) {\n\t\t\t\t/* if the shape is shared, clone it */\n"
	shapeCloneNew = "\t\t\t// Changed by internal/fixlib: copy a shape the shape cache can keep (see fixShapeCache).\n\t\t\tif (*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, sh))).Fref_count != int32(1) || shapeKeepable(tls, ctx, sh) {\n\t\t\t\t/* if the shape is shared, clone it */\n"
)

var (
	reShapeStruct = regexp.MustCompile(`\ntype TJSShape = struct \{\n`)
	// gc_phase, the field right after tmp_obj_list, which ccgo names by its
	// offset, and the first statement of gc_free_cycles, which sets it.
	reGCPhaseField = regexp.MustCompile(`\n\tFtmp_obj_list +Tlist_head\n\tF__ccgo(\d+) +uint8\n`)
	reGCPhaseSet   = regexp.MustCompile(`^\{\n\tvar [^\n]+\n\t_[^\n]+\n\tlibc\.SetBitFieldPtr8Int32\(rt\+(\d+), int32\(EJS_GC_PHASE_REMOVE_CYCLES\), 0, 0xff\)\n`)

	// The functions whose work lib/shapecache.go relies on: how shapes are
	// made, found, copied, grown, changed, counted and freed, how objects
	// take and leave them, how the cycle collector frees them, and how an
	// atom tells a symbol.
	shapeFuncs = []string{"_add_property", "_add_shape_property", "_find_hashed_shape_prop", "_js_clone_shape", "_js_new_shape2", "_resize_shape_hash", "_js_shape_prepare_update", "_resize_properties", "_compact_properties", "_JS_NewObjectFromShape", "_free_object", "_js_free_shape", "_js_free_shape0", "_js_rc", "_gc_free_cycles", "___JS_NewAtom", "_JS_ResizeAtomHash", "_JS_FreeAtomStruct", "_js_get_atom_index"}
)

// fixShapeCache wires the shape cache of lib/shapecache.go, in the field
// that fixRuntimeField adds to JSRuntime, into the engine: add_property
// calls shapeFound when it finds the next shape in the shape hash table,
// and asks shapeKeepable whether to copy a shape that only its object uses
// rather than grow it, js_free_shape0 calls shapeKeep first, and
// JS_FreeRuntime calls shapeCacheOff first.
//
// lib/shapecache.go keeps shapes alive past the count the engine gives
// them, grows the shape hash table as js_new_shape2 does, reads gc_phase
// at the offset right after tmp_obj_list, and tells a symbol by the
// hash_next that __JS_NewAtom gives it and that JS_ResizeAtomHash,
// JS_FreeAtomStruct and js_get_atom_index read and write. So the text of
// shapeFuncs and of JSShape is pinned in shapeSites: when a release changes
// one of them, fixShapeCache stops, and the new code must be read against
// lib/shapecache.go before the fingerprints are updated (go run
// ./internal/fixlib -shape prints them). fixShapeCache also checks that
// gc_phase is right after tmp_obj_list, where gcPhaseOffset says.
//
// Other changes match these functions as ccgo writes them, so
// fixShapeCache comes after them, and restoreShapeCache, its exact inverse,
// comes first.
func fixShapeCache(b []byte, file string) ([]byte, error) {
	bare, err := restoreShapeCache(b)
	if err != nil {
		return nil, err
	}
	got, err := shapeFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if want := shapeSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/shapecache.go, then update shapeSites (go run ./internal/fixlib -shape)", file, strings.Join(shapeFuncs, ", ")+" or JSShape", got, want)
	}
	if err := checkGCPhase(bare); err != nil {
		return nil, err
	}
	out, err := addShapeCache(bare)
	if err != nil {
		return nil, err
	}
	if back, err := restoreShapeCache(out); err != nil || !bytes.Equal(back, bare) {
		return nil, fmt.Errorf("fixShapeCache: restoreShapeCache does not give the input back (%v)", err)
	}
	if !bytes.Equal(bare, b) && !bytes.Equal(out, b) {
		return nil, fmt.Errorf("fixShapeCache: the file has a part of the change")
	}
	return out, nil
}

// checkGCPhase checks that gc_phase is the byte right after tmp_obj_list:
// the field there is named by its offset, which gc_free_cycles writes.
func checkGCPhase(b []byte) error {
	st, err := structBody(b, reRuntimeStruct, "TJSRuntime")
	if err != nil {
		return err
	}
	field := reGCPhaseField.FindSubmatch(st)
	if field == nil {
		return fmt.Errorf("TJSRuntime: no byte right after tmp_obj_list")
	}
	body, err := funcBody(b, funcHead("_gc_free_cycles"), "_gc_free_cycles")
	if err != nil {
		return err
	}
	head := funcHead("_gc_free_cycles").FindIndex(body)[1] - 2
	set := reGCPhaseSet.FindSubmatch(body[head:])
	if set == nil || !bytes.Equal(set[1], field[1]) {
		return fmt.Errorf("_gc_free_cycles: gc_phase is not the byte right after tmp_obj_list, at %s", field[1])
	}
	return nil
}

// addShapeCache makes the change in b, which has none of it.
func addShapeCache(b []byte) ([]byte, error) {
	var err error
	for _, h := range []struct{ name, hook string }{
		{"_js_free_shape0", shapeKeepHook},
		{"XJS_FreeRuntime", shapeOffHook},
	} {
		if b, err = insertAfterDecls(b, h.name, h.hook); err != nil {
			return nil, err
		}
	}
	body, err := funcBody(b, funcHead("_add_property"), "_add_property")
	if err != nil {
		return nil, err
	}
	if n := bytes.Count(body, []byte(shapeCloneOld)); n != 1 {
		return nil, fmt.Errorf("_add_property: %d tests of a shared shape, want 1", n)
	}
	if n := bytes.Count(body, []byte(shapeFoundOld)); n != 1 {
		return nil, fmt.Errorf("_add_property: %d objects that take a found shape, want 1", n)
	}
	changed := bytes.Replace(body, []byte(shapeCloneOld), []byte(shapeCloneNew), 1)
	changed = bytes.Replace(changed, []byte(shapeFoundOld), []byte(shapeFoundNew), 1)
	return bytes.Replace(b, body, changed, 1), nil
}

// restoreShapeCache undoes fixShapeCache.
func restoreShapeCache(b []byte) ([]byte, error) {
	for _, r := range []struct{ new, old string }{
		{shapeKeepHook, ""},
		{shapeOffHook, ""},
		{shapeCloneNew, shapeCloneOld},
		{shapeFoundNew, shapeFoundOld},
	} {
		if n := bytes.Count(b, []byte(r.new)); n > 1 {
			return nil, fmt.Errorf("restoreShapeCache: %d copies of %q", n, r.new)
		}
		b = bytes.Replace(b, []byte(r.new), []byte(r.old), 1)
	}
	for _, name := range []string{"shapeKeep(", "shapeKeepable(", "shapeFound(", "shapeCacheOff("} {
		if bytes.Contains(b, []byte(name)) {
			return nil, fmt.Errorf("restoreShapeCache: a call of %s is left", name[:len(name)-1])
		}
	}
	return b, nil
}

// shapeFingerprint hashes shapeFuncs and JSShape, as ccgo writes them,
// with each C string written out in place of its offset.
func shapeFingerprint(b []byte) (string, error) {
	funcs, err := funcsFingerprint(b, shapeFuncs, func(text []byte) []byte { return text })
	if err != nil {
		return "", err
	}
	st, err := structBody(b, reShapeStruct, "TJSShape")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(funcs))
	h.Write(st)
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}
