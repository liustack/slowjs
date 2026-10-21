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

// The text fixFuncShape adds.
const (
	funcShapeHook = "\t// Changed by internal/fixlib: put a new function straight onto the shape with length and name (see fixFuncShape).\n\tif funcShape(tls, ctx1, func_obj, name, len1) {\n\t\treturn\n\t}\n"

	// The mark of the shape of arrays in JS_MarkContext, after which
	// funcShapesMark marks the shapes held for the context, and the free of
	// it in JS_FreeContext, before which funcShapesFree lets them go.
	funcShapeMarkAt   = "\tif (*TJSContext)(unsafe.Pointer(ctx)).Farray_shape != 0 {\n\t\t(*(*func(*libc.TLS, uintptr, uintptr))(unsafe.Pointer(&struct{ uintptr }{__ccgo_fp_mark_func})))(tls, rt, (*TJSContext)(unsafe.Pointer(ctx)).Farray_shape)\n\t}\n"
	funcShapeMarkHook = "\t// Changed by internal/fixlib: mark the shapes of new functions held for the context (see fixFuncShape).\n\tfuncShapesMark(tls, rt, ctx, __ccgo_fp_mark_func)\n"
	funcShapeFreeAt   = "\t_js_free_shape_null(tls, (*TJSContext)(unsafe.Pointer(ctx1)).Frt, (*TJSContext)(unsafe.Pointer(ctx1)).Farray_shape)\n"
	funcShapeFreeHook = "\t// Changed by internal/fixlib: let go of the shapes of new functions held for the context (see fixFuncShape).\n\tfuncShapesFree(tls, ctx1)\n"
)

var (
	// The functions whose work lib/funcshape.go repeats or relies on:
	// what js_function_set_properties does, how add_property finds the
	// next shape and moves an object onto it, how a shape gets a key, that
	// a shared shape is copied before it changes, how an object gets its
	// property array, and the string of an atom.
	funcShapeFuncs = []string{"_js_function_set_properties", "_add_property", "_find_hashed_shape_prop", "_add_shape_property", "_js_shape_prepare_update", "_js_clone_shape", "_JS_NewObjectFromShape", "XJS_AtomToString", "_JS_MarkContext", "XJS_FreeContext"}

	reFuncShapeHead = regexp.MustCompile(`\nfunc _js_function_set_properties\(tls \*libc\.TLS, ctx1 uintptr, func_obj TJSValue, name TJSAtom, len1 int32\) \{\n`)
)

// fixFuncShape makes js_function_set_properties call funcShape of
// lib/funcshape.go first, which puts a new function with the empty shape of
// its prototype straight onto the shape with length and name. funcShape
// holds the shapes for a context, as the context holds the shape of its
// arrays: JS_MarkContext calls funcShapesMark after it marks that shape,
// and JS_FreeContext calls funcShapesFree before it frees it.
//
// funcShape does what js_function_set_properties does through
// JS_DefinePropertyValue, objlitDefine and add_property, and relies on a
// shared shape never being changed in place, and holds shapes for a context
// as JS_MarkContext and JS_FreeContext hold the shape of arrays. So the text
// of funcShapeFuncs
// and of JSShape is pinned in funcShapeSites (objlitDefine and the functions
// it stands for are pinned in defineSites): when a release changes one of
// them, fixFuncShape stops, and the new code must be read against
// lib/funcshape.go before the fingerprints are updated (go run
// ./internal/fixlib -funcshape prints them).
//
// Other changes match these functions as ccgo writes them, so fixFuncShape
// comes after them, and restoreFuncShape, its exact inverse, comes first.
func fixFuncShape(b []byte, file string) ([]byte, error) {
	bare, err := restoreFuncShape(b)
	if err != nil {
		return nil, err
	}
	got, err := funcShapeFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if want := funcShapeSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/funcshape.go, then update funcShapeSites (go run ./internal/fixlib -funcshape)", file, strings.Join(funcShapeFuncs, ", ")+" or JSShape", got, want)
	}
	if len(findAll(reFuncShapeHead, bare)) != 1 {
		return nil, fmt.Errorf("_js_function_set_properties: want one definition with the parameters funcShape takes")
	}
	out, err := insertAfterDecls(bare, "_js_function_set_properties", funcShapeHook)
	if err != nil {
		return nil, err
	}
	for _, h := range []struct{ name, at, text string }{
		{"_JS_MarkContext", funcShapeMarkAt, funcShapeMarkAt + funcShapeMarkHook},
		{"XJS_FreeContext", funcShapeFreeAt, funcShapeFreeHook + funcShapeFreeAt},
	} {
		body, err := funcBody(out, funcHead(h.name), h.name)
		if err != nil {
			return nil, err
		}
		if n := bytes.Count(body, []byte(h.at)); n != 1 {
			return nil, fmt.Errorf("%s: %d places for the shapes of new functions, want 1: %q", h.name, n, h.at)
		}
		out = bytes.Replace(out, body, bytes.Replace(body, []byte(h.at), []byte(h.text), 1), 1)
	}
	if back, err := restoreFuncShape(out); err != nil || !bytes.Equal(back, bare) {
		return nil, fmt.Errorf("fixFuncShape: restoreFuncShape does not give the input back (%v)", err)
	}
	if !bytes.Equal(bare, b) && !bytes.Equal(out, b) {
		return nil, fmt.Errorf("fixFuncShape: the file has a part of the change")
	}
	return out, nil
}

// restoreFuncShape undoes fixFuncShape.
func restoreFuncShape(b []byte) ([]byte, error) {
	for _, hook := range []string{funcShapeHook, funcShapeMarkHook, funcShapeFreeHook} {
		if n := bytes.Count(b, []byte(hook)); n > 1 {
			return nil, fmt.Errorf("restoreFuncShape: %d copies of %q", n, hook)
		}
		b = bytes.Replace(b, []byte(hook), nil, 1)
	}
	for _, name := range []string{"funcShape(", "funcShapesMark(", "funcShapesFree("} {
		if bytes.Contains(b, []byte(name)) {
			return nil, fmt.Errorf("restoreFuncShape: a call of %s is left", name[:len(name)-1])
		}
	}
	return b, nil
}

// funcShapeFingerprint hashes funcShapeFuncs and JSShape, without the late
// changes fixFuncShape comes among (the shape cache changes add_property),
// so that -funcshape, which reports on files without them, gives the same,
// with each C string written out in place of its offset. fixOneFrame, which
// comes after them, changes none of these functions.
func funcShapeFingerprint(b []byte) (string, error) {
	b, err := restoreBeforeOneFrame(b)
	if err != nil {
		return "", err
	}
	funcs, err := funcsFingerprint(b, funcShapeFuncs, func(text []byte) []byte { return text })
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
