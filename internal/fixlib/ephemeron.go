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
	"strings"
)

var ephemeronF = flag.Bool("ephemeron", false, "print collector fingerprints after the earlier final rewrites for review")

var ephemeronHooks = []struct{ name, text string }{
	{"_JS_RunGCInternal", "\t// Added by internal/fixlib: conditional WeakMap edges (lib/ephemeron.go).\n\tif gcEphemeron(tls, rt, remove_weak_objects) {\n\t\treturn\n\t}\n"},
	{"_js_map_mark", "\t// Added by internal/fixlib: WeakMap values are scanned from reachable keys.\n\tif gcEphemeronMap(tls, rt, _val, __ccgo_fp_mark_func) {\n\t\treturn\n\t}\n"},
}

// Pin the trial deletion, restoration, sweep and weak record ownership as
// well as the two entry points. All hooks are removed before earlier rules.
var ephemeronFunctions = []string{"XJS_MarkValue", "_mark_children", "_JS_RunGCInternal", "_js_map_mark", "_gc_decref", "_gc_scan", "_gc_scan_incref_child", "_gc_scan_incref_child2", "_gc_free_cycles", "_gc_remove_weak_objects", "_map_delete_record_internal", "_js_weakref_free", "_js_weakref_is_live"}

func ephemeronFingerprint(b []byte) (string, error) {
	h := sha256.New()
	for _, name := range ephemeronFunctions {
		body, err := funcBody(b, funcHead(name), name)
		if err != nil {
			return "", err
		}
		h.Write(body)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func ephemeronSymbolHook(b []byte) (string, error) {
	body, err := funcBody(b, funcHead("XJS_MarkValue"), "XJS_MarkValue")
	if err != nil {
		return "", err
	}
	param := "val"
	if bytes.Contains(body, []byte("_val TJSValue")) {
		param = "_val"
	}
	return "\t\tcase int32(EJS_TAG_SYMBOL):\n\t\t\tgcEphemeronSymbol(tls, rt, jsvPtr(" + param + "), __ccgo_fp_mark_func)\n", nil
}
func ephemeronHookText(name, text string, b []byte) string {
	if name == "_js_map_mark" && bytes.Contains(b, []byte("func _js_map_mark(tls *libc.TLS, rt uintptr, val TJSValue,")) {
		return strings.ReplaceAll(text, "_val,", "val,")
	}
	return text
}

func restoreEphemeron(b []byte, _ string) ([]byte, error) {
	symbol, err := ephemeronSymbolHook(b)
	if err != nil {
		return nil, err
	}
	if n := bytes.Count(b, []byte(symbol)); n > 1 {
		return nil, fmt.Errorf("ephemeron: duplicate symbol hook")
	} else if n == 1 {
		b = bytes.Replace(b, []byte(symbol), nil, 1)
	}
	if bytes.Contains(b, []byte("gcEphemeronSymbol(")) {
		return nil, fmt.Errorf("ephemeron: changed symbol hook")
	}

	for _, hook := range ephemeronHooks {
		hook.text = ephemeronHookText(hook.name, hook.text, b)
		n := bytes.Count(b, []byte(hook.text))
		if n > 1 {
			return nil, fmt.Errorf("ephemeron: duplicate hook %s", hook.name)
		}
		if n == 1 {
			loc := findAll(funcHead(hook.name), b)
			if len(loc) != 1 || !bytes.HasPrefix(b[loc[0][1]:], []byte(hook.text)) {
				return nil, fmt.Errorf("ephemeron: misplaced hook %s", hook.name)
			}
			b = bytes.Replace(b, []byte(hook.text), nil, 1)
		}
	}
	if bytes.Contains(b, []byte("gcEphemeron(")) || bytes.Contains(b, []byte("gcEphemeronMap(")) {
		return nil, fmt.Errorf("ephemeron: changed hook")
	}
	return b, nil
}

func fixEphemeron(b []byte, name string) ([]byte, error) {
	bare, err := restoreEphemeron(b, name)
	if err != nil {
		return nil, err
	}
	got, err := ephemeronFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if got != ephemeronSites[name] {
		return nil, fmt.Errorf("%s: ephemeron collector fingerprint %s, want %s: review lib/ephemeron.go and docs/fork-changes/weak-collections.md", name, got, ephemeronSites[name])
	}
	out := bytes.Clone(bare)
	for _, hook := range ephemeronHooks {
		hook.text = ephemeronHookText(hook.name, hook.text, out)
		loc := findAll(funcHead(hook.name), out)
		if len(loc) != 1 {
			return nil, fmt.Errorf("ephemeron: want one %s", hook.name)
		}
		at := loc[0][1]
		out = append(append(append([]byte{}, out[:at]...), hook.text...), out[at:]...)
	}
	symbol, err := ephemeronSymbolHook(out)
	if err != nil {
		return nil, err
	}
	body, err := funcBody(out, funcHead("XJS_MarkValue"), "XJS_MarkValue")
	if err != nil {
		return nil, err
	}
	if strings.Count(string(body), "\t\tdefault:\n") != 1 {
		return nil, fmt.Errorf("ephemeron: symbol switch changed")
	}
	out = bytes.Replace(out, body, bytes.Replace(body, []byte("\t\tdefault:\n"), []byte(symbol+"\t\tdefault:\n"), 1), 1)
	if !bytes.Equal(bare, b) && !bytes.Equal(out, b) {
		return nil, fmt.Errorf("ephemeron: incomplete rewrite")
	}
	back, err := restoreEphemeron(out, name)
	if err != nil || !bytes.Equal(back, bare) {
		return nil, fmt.Errorf("ephemeron: inverse did not restore input: %v", err)
	}
	return out, nil
}

func ephemeronReport(read func(string) ([]byte, error)) error {
	files, err := filepath.Glob("lib/ccgo_*.go")
	if err != nil {
		return err
	}
	fmt.Println("var ephemeronSites = map[string]string{")
	for _, fn := range files {
		b, err := read(fn)
		if err != nil {
			return err
		}
		name := filepath.Base(fn)
		for _, fix := range []func([]byte, string) ([]byte, error){fixExpect, fixFastFrames} {
			b, err = fix(b, name)
			if err != nil {
				return err
			}
		}
		sum, err := ephemeronFingerprint(b)
		if err != nil {
			return err
		}
		fmt.Printf("\t%q: %q,\n", name, sum)
	}
	fmt.Println("}")
	return nil
}
