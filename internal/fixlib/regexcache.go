// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

var (
	// The head of js_compile_regexp: the names of its context, pattern and
	// flags.
	reCompileRegexpHead = regexp.MustCompile(`\nfunc _js_compile_regexp\(tls \*libc\.TLS, (ctx\d*) uintptr, (_?pattern) TJSValue, (_?flags) TJSValue\) \(r TJSValue\) \{\n`)

	// The functions whose work lib/regexcache.go relies on: what
	// js_compile_regexp reads of its pattern and flags and what it returns,
	// and how the cache hashes, compares, counts and frees strings.
	regexCacheFuncs = []string{"_js_compile_regexp", "_hash_string", "_hash_string8", "_hash_string16", "_js_string_compare", "_js_string_memcmp", "_js_free_string", "_js_rc", "Xjs_malloc_usable_size_rt"}
)

const (
	regexCacheGetMark = "\t// Changed by internal/fixlib: take the bytecode the regexp cache keeps (see fixRegexCache).\n"
	regexCachePutMark = "\t// Changed by internal/fixlib: keep the bytecode in the regexp cache (see fixRegexCache).\n"
	regexCacheOffHook = "\t// Changed by internal/fixlib: let go of the bytecode the regexp cache keeps (see fixRegexCache).\n\tregexpCacheOff(tls, rt1)\n"
)

// regexCacheGet is the text fixRegexCache puts at the start of
// js_compile_regexp, whose context, pattern and flags are named ctx,
// pattern and flags.
func regexCacheGet(ctx, pattern, flags string) string {
	return regexCacheGetMark + fmt.Sprintf("\tif bc, ok := regexpCacheGet(tls, %s, %s, %s); ok {\n\t\treturn bc\n\t}\n", ctx, pattern, flags)
}

// regexCachePut is the text fixRegexCache puts before the return of the
// bytecode js_compile_regexp made.
func regexCachePut(ctx, pattern, flags string) string {
	return regexCachePutMark + fmt.Sprintf("\tregexpCachePut(tls, %s, %s, %s, ret)\n", ctx, pattern, flags)
}

// fixRegexCache wires the regexp cache of lib/regexcache.go, in the field
// that fixRuntimeField adds to JSRuntime, into the engine:
// js_compile_regexp asks regexpCacheGet first and returns the bytecode it
// gives, and calls regexpCachePut with the bytecode it made before it
// returns it, and JS_FreeRuntime calls regexpCacheOff first.
//
// lib/regexcache.go skips js_compile_regexp for a pattern and flags that
// are strings, on the grounds that for them it only reads them, and
// hashes, compares, counts and frees strings through the engine's own
// functions. So the text of regexCacheFuncs is pinned in regexCacheSites:
// when a release changes one of them, fixRegexCache stops, and the new
// code must be read against lib/regexcache.go before the fingerprints are
// updated (go run ./internal/fixlib -recache prints them).
//
// Other changes match JS_FreeRuntime as ccgo writes it, so fixRegexCache
// comes after them, and restoreRegexCache, its exact inverse, comes first.
func fixRegexCache(b []byte, file string) ([]byte, error) {
	bare, err := restoreRegexCache(b)
	if err != nil {
		return nil, err
	}
	got, err := regexCacheFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if want := regexCacheSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/regexcache.go, then update regexCacheSites (go run ./internal/fixlib -recache)", file, strings.Join(regexCacheFuncs, ", "), got, want)
	}
	out, err := addRegexCache(bare)
	if err != nil {
		return nil, err
	}
	if back, err := restoreRegexCache(out); err != nil || !bytes.Equal(back, bare) {
		return nil, fmt.Errorf("fixRegexCache: restoreRegexCache does not give the input back (%v)", err)
	}
	if !bytes.Equal(bare, b) && !bytes.Equal(out, b) {
		return nil, fmt.Errorf("fixRegexCache: the file has a part of the change")
	}
	return out, nil
}

// compileRegexpNames returns the names of the context, the pattern and the
// flags of js_compile_regexp in b.
func compileRegexpNames(b []byte) (ctx, pattern, flags string, err error) {
	m := reCompileRegexpHead.FindAllSubmatch(b, -1)
	if len(m) != 1 {
		return "", "", "", fmt.Errorf("want 1 function _js_compile_regexp, found %d", len(m))
	}
	return string(m[0][1]), string(m[0][2]), string(m[0][3]), nil
}

// addRegexCache makes the change in b, which has none of it.
func addRegexCache(b []byte) ([]byte, error) {
	ctx, pattern, flags, err := compileRegexpNames(b)
	if err != nil {
		return nil, err
	}
	if b, err = insertAfterDecls(b, "XJS_FreeRuntime", regexCacheOffHook); err != nil {
		return nil, err
	}
	if b, err = insertAfterDecls(b, "_js_compile_regexp", regexCacheGet(ctx, pattern, flags)); err != nil {
		return nil, err
	}
	body, err := funcBody(b, reCompileRegexpHead, "_js_compile_regexp")
	if err != nil {
		return nil, err
	}
	// The bytecode made is returned at the end, after the buffer lre_compile
	// wrote it into is freed. Every other return is of an exception.
	end := fmt.Sprintf("\tret = _js_new_string8_len(tls, %[1]s, re_bytecode_buf, *(*int32)(unsafe.Pointer(bp + %[2]s)))\n\tXjs_free(tls, %[1]s, re_bytecode_buf)\n\treturn ret\n}\n", ctx, `\d+`)
	reEnd := regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(end), `\\d\+`, `\d+`) + `$`)
	loc := reEnd.FindIndex(body)
	if loc == nil {
		return nil, fmt.Errorf("_js_compile_regexp: the bytecode is not returned at the end")
	}
	if n := bytes.Count(body, []byte("\treturn ret\n")); n != 1 {
		return nil, fmt.Errorf("_js_compile_regexp: %d returns of ret, want 1", n)
	}
	at := bytes.LastIndex(body, []byte("\treturn ret\n"))
	return bytes.Replace(b, body, splice(body, at, regexCachePut(ctx, pattern, flags)), 1), nil
}

// reRegexCacheHooks are the texts addRegexCache puts into js_compile_regexp,
// with their mark, whatever the names.
var reRegexCacheHooks = regexp.MustCompile(regexp.QuoteMeta(regexCacheGetMark) + `\tif bc, ok := regexpCacheGet\(tls, \w+, \w+, \w+\); ok \{\n\t\treturn bc\n\t\}\n|` + regexp.QuoteMeta(regexCachePutMark) + `\tregexpCachePut\(tls, \w+, \w+, \w+, ret\)\n`)

// restoreRegexCache undoes fixRegexCache.
func restoreRegexCache(b []byte) ([]byte, error) {
	if n := bytes.Count(b, []byte(regexCacheOffHook)); n > 1 {
		return nil, fmt.Errorf("restoreRegexCache: %d copies of %q", n, regexCacheOffHook)
	}
	b = bytes.Replace(b, []byte(regexCacheOffHook), nil, 1)
	for _, mark := range []string{regexCacheGetMark, regexCachePutMark} {
		if n := bytes.Count(b, []byte(mark)); n > 1 {
			return nil, fmt.Errorf("restoreRegexCache: %d copies of %q", n, mark)
		}
	}
	b = reRegexCacheHooks.ReplaceAll(b, nil)
	for _, name := range []string{"regexpCacheGet(", "regexpCachePut(", "regexpCacheOff(", regexCacheGetMark, regexCachePutMark} {
		if bytes.Contains(b, []byte(name)) {
			return nil, fmt.Errorf("restoreRegexCache: a part of %s is left", strings.TrimSpace(strings.TrimSuffix(name, "(")))
		}
	}
	return b, nil
}

// regexCacheFingerprint hashes regexCacheFuncs, as ccgo writes them, with
// each C string written out in place of its offset.
func regexCacheFingerprint(b []byte) (string, error) {
	return funcsFingerprint(b, regexCacheFuncs, func(text []byte) []byte { return text })
}
