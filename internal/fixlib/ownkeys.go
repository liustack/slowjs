// Copyright (c) 2026 The slowjs Authors.
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

// A function fixOwnKeys changes: the line that starts it, and the text it
// puts right after that line, before the function takes its TLS frame.
type ownKeysHook struct {
	name, head, text string
}

var (
	ownKeysHooks = []ownKeysHook{
		{
			"_JS_GetOwnPropertyNamesInternal",
			"func _JS_GetOwnPropertyNamesInternal(tls *libc.TLS, ctx uintptr, ptab uintptr, plen uintptr, p uintptr, flags int32) (r int32) {\n",
			"\t// Changed by internal/fixlib: list the keys of an ordinary object in Go (see fixOwnKeys).\n\tif r1, ok := ownKeys(tls, ctx, ptab, plen, p, flags); ok {\n\t\treturn r1\n\t}\n",
		},
		{
			"_JS_CopyDataProperties",
			"func _JS_CopyDataProperties(tls *libc.TLS, ctx2 uintptr, target TJSValue, _source TJSValue, _excluded TJSValue, setprop TBOOL) (r int32) {\n",
			"\t// Changed by internal/fixlib: copy the data properties of an ordinary object in Go (see fixOwnKeys).\n\tif r1, ok := copyData(tls, ctx2, target, _source, _excluded, setprop); ok {\n\t\treturn r1\n\t}\n",
		},
		{
			"_JS_GetOwnPropertyNames2",
			"func _JS_GetOwnPropertyNames2(tls *libc.TLS, ctx2 uintptr, obj1 TJSValue, flags int32, kind int32) (r1 TJSValue) {\n",
			"\t// Changed by internal/fixlib: make the array of the keys, values or entries of an ordinary object in Go (see fixOwnKeys).\n\tif r2, ok := ownKeysArray(tls, ctx2, obj1, flags, kind); ok {\n\t\treturn r2\n\t}\n",
		},
		{
			"_js_shape_hash_unlink",
			"func _js_shape_hash_unlink(tls *libc.TLS, rt uintptr, sh uintptr) {\n",
			"\t// Changed by internal/fixlib: the shape leaves the shape hash table, the key cache drops it (see fixOwnKeys).\n\townKeyCacheDrop(rt, sh)\n",
		},
	}

	// A call of lib/ownkeys.go.
	reOwnKeysCall = regexp.MustCompile(`[^\w.](?:ownKeys|copyData|ownKeysArray|ownKeyCacheDrop)\(`)

	// The functions whose work lib/ownkeys.go repeats or relies on: the
	// three it is called from, how an atom tells its kind and an array
	// index, the shape properties, how a shape leaves the shape hash table
	// before it changes or is freed, how add_property moves an object onto
	// a shape, and how an array gets its elements.
	ownKeysFuncs = []string{
		"_JS_GetOwnPropertyNamesInternal", "_JS_CopyDataProperties", "_JS_GetOwnPropertyNames2",
		"_JS_AtomGetKind", "_JS_AtomIsArrayIndex", "_is_num_string", "_get_shape_prop",
		"_js_shape_hash_unlink", "_js_shape_hash_link", "_add_shape_property", "_js_shape_prepare_update", "_resize_properties", "_compact_properties", "_js_clone_shape", "_js_free_shape0",
		"_add_property", "_expand_fast_array", "_add_fast_array_element",
	}
)

// fixOwnKeys makes three functions that list the own keys of an object ask
// lib/ownkeys.go first, which lists those of an ordinary object from its
// shape in Go: JS_GetOwnPropertyNamesInternal calls ownKeys,
// JS_CopyDataProperties, behind object spread and the rest of a
// destructuring, calls copyData, and JS_GetOwnPropertyNames2, behind
// Object.keys, Object.values and Object.entries, calls ownKeysArray. Each
// does nothing and says so for what it does not handle, and the engine goes
// on as before. lib/ownkeys.go keeps the kinds of the keys of a shape in the
// shape hash table in the key cache of the runtime, in the field that
// fixRuntimeField adds to JSRuntime, and js_shape_hash_unlink calls
// ownKeyCacheDrop first, so that the cache drops a shape when it leaves the
// table.
//
// lib/ownkeys.go repeats what JS_GetOwnPropertyNamesInternal,
// JS_CopyDataProperties and JS_GetOwnPropertyNames2 do for an ordinary
// object, and relies on a shape in the shape hash table never changing in
// place or being freed without leaving it. So the text of ownKeysFuncs and
// of JSShape is pinned in ownKeysSites: when a release changes one of them,
// fixOwnKeys stops, and the new code must be read against lib/ownkeys.go
// before the fingerprints are updated (go run ./internal/fixlib -ownkeys
// prints them). The bit fields it reads are the constants of fixForIn,
// fixAtomKey and fixDefine.
//
// Other changes match these functions as ccgo writes them, so fixOwnKeys
// comes after them, and restoreOwnKeys, its exact inverse, comes first.
func fixOwnKeys(b []byte, file string) ([]byte, error) {
	bare, err := restoreOwnKeys(b)
	if err != nil {
		return nil, err
	}
	got, err := ownKeysFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if want := ownKeysSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/ownkeys.go, then update ownKeysSites (go run ./internal/fixlib -ownkeys)", file, strings.Join(ownKeysFuncs, ", ")+" or JSShape", got, want)
	}
	out := bare
	for _, h := range ownKeysHooks {
		if n := bytes.Count(out, []byte("\n"+h.head)); n != 1 {
			return nil, fmt.Errorf("%s: want one definition with the parameters lib/ownkeys.go takes, found %d", h.name, n)
		}
		out = bytes.Replace(out, []byte("\n"+h.head), []byte("\n"+h.head+h.text), 1)
	}
	if back, err := restoreOwnKeys(out); err != nil || !bytes.Equal(back, bare) {
		return nil, fmt.Errorf("fixOwnKeys: restoreOwnKeys does not give the input back (%v)", err)
	}
	if !bytes.Equal(bare, b) && !bytes.Equal(out, b) {
		return nil, fmt.Errorf("fixOwnKeys: the file has a part of the change")
	}
	return out, nil
}

// restoreOwnKeys undoes fixOwnKeys.
func restoreOwnKeys(b []byte) ([]byte, error) {
	for _, h := range ownKeysHooks {
		if n := bytes.Count(b, []byte(h.text)); n > 1 {
			return nil, fmt.Errorf("restoreOwnKeys: %d copies of %q", n, h.text)
		}
		b = bytes.Replace(b, []byte(h.head+h.text), []byte(h.head), 1)
	}
	if m := reOwnKeysCall.Find(b); m != nil {
		return nil, fmt.Errorf("restoreOwnKeys: a call is left: %s", m)
	}
	return b, nil
}

// ownKeysFingerprint hashes ownKeysFuncs and JSShape, without the late
// changes fixOwnKeys comes among (the shape cache changes add_property and
// js_free_shape0), so that -ownkeys, which reports on files without them,
// gives the same, with each C string written out in place of its offset.
func ownKeysFingerprint(b []byte) (string, error) {
	b, err := restoreBeforeOneFrame(b)
	if err != nil {
		return "", err
	}
	funcs, err := funcsFingerprint(b, ownKeysFuncs, func(text []byte) []byte { return text })
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
