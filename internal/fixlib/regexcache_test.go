// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// Every target has the regexp cache wired in: js_compile_regexp asks the
// cache before it compiles and keeps what it compiled, and JS_FreeRuntime
// lets the cache go before the shape cache and the front cache.
func TestRegexCacheEveryFile(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readNoDefer(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		ctx, pattern, flags, err := compileRegexpNames(b)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		body, err := funcBody(b, reCompileRegexpHead, "_js_compile_regexp")
		if err != nil {
			t.Fatal(err)
		}
		get := strings.Replace(regexCacheGet(ctx, pattern, flags), "\t\treturn bc\n", "\t\tr = bc\n\t\ttls.Free(96)\n\t\treturn\n", 1)
		put := regexCachePut(ctx, pattern, flags)
		for _, text := range []string{get, put} {
			if n := bytes.Count(body, []byte(text)); n != 1 {
				t.Errorf("%s: %d copies of %q in js_compile_regexp", name, n, text)
			}
		}
		// The cache is asked before the flags are read, and given what the
		// last statement returns.
		if i, j := bytes.Index(body, []byte(get)), bytes.Index(body, []byte("re_flags = 0\n")); i < 0 || j < i {
			t.Errorf("%s: js_compile_regexp reads the flags before it asks the cache", name)
		}
		if !bytes.HasSuffix(body, []byte(put+"\tr = ret\n\ttls.Free(96)\n\treturn\n}\n")) {
			t.Errorf("%s: js_compile_regexp does not keep the bytecode right before it returns it", name)
		}
		if n := bytes.Count(b, []byte(regexCacheOffHook)); n != 1 {
			t.Errorf("%s: %d copies of the hook of JS_FreeRuntime", name, n)
		}
		if i, j := bytes.Index(b, []byte(regexCacheOffHook)), bytes.Index(b, []byte(shapeOffHook)); i < 0 || j < i {
			t.Errorf("%s: JS_FreeRuntime lets the shape cache go first", name)
		}
	}
}

// fixRegexCache leaves a fixed file as it is, and puts the change back into
// a file without it, on every target. restoreRegexCache gives back the text
// ccgo writes.
func TestRegexCacheIdempotent(t *testing.T) {
	for _, fn := range libFiles(t) {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(fn)
		for _, s := range []string{"regexpCacheGet(", "regexpCachePut(", "regexpCacheOff("} {
			if bytes.Contains(b, []byte(s)) {
				t.Fatalf("%s: readEngine leaves a call of %s in", name, s)
			}
		}
		fixed, err := fixRegexCache(bytes.Clone(b), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if bytes.Equal(fixed, b) {
			t.Fatalf("%s: fixRegexCache changed nothing", name)
		}
		again, err := fixRegexCache(bytes.Clone(fixed), name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(again, fixed) {
			t.Errorf("%s: a fixed file changed", name)
		}
		back, err := restoreRegexCache(fixed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(back, b) {
			t.Errorf("%s: restoreRegexCache does not give the text back", name)
		}
	}
}

// A change to what lib/regexcache.go relies on, or a part of the change
// missing, stops fixRegexCache.
func TestRegexCacheRejects(t *testing.T) {
	b := string(engineFile(t))
	fixed, err := fixRegexCache([]byte(b), "ccgo_darwin_arm64.go")
	if err != nil {
		t.Fatal(err)
	}
	put := regexCachePut("ctx1", "pattern", "flags")
	for _, c := range []struct {
		name, in, old, new, err string
	}{
		{"another hash", b, "h = _hash_string8(tls, str+12,", "h = _hash_string8(tls, str+13,", "changed (fingerprint"},
		{"another free", b, "_JS_FreeAtomStruct(tls, rt, str)", "Xjs_free_rt(tls, rt, str)", "changed (fingerprint"},
		{"no get", string(fixed), regexCacheGetMark, "", "a part of regexpCacheGet is left"},
		{"no put", string(fixed), put, "", "a part of the change"},
		{"no off", string(fixed), regexCacheOffHook, "", "a part of the change"},
		{"a put left", string(fixed), put, "\tregexpCachePut(tls, ctx1, pattern, flags, ret)\n", "a part of regexpCachePut is left"},
	} {
		if !strings.Contains(c.in, c.old) {
			t.Fatalf("%s: no %q", c.name, c.old)
		}
		_, err := fixRegexCache([]byte(strings.Replace(c.in, c.old, c.new, 1)), "ccgo_darwin_arm64.go")
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: %v, want %q", c.name, err, c.err)
		}
	}
	// Another flag js_compile_regexp takes changes its fingerprint.
	body, err := funcBody([]byte(b), reCompileRegexpHead, "_js_compile_regexp")
	if err != nil {
		t.Fatal(err)
	}
	flag := bytes.Replace(body, []byte("case int32('y'):"), []byte("case int32('z'):"), 1)
	if bytes.Equal(flag, body) {
		t.Fatal("no flag y")
	}
	if _, err := fixRegexCache(bytes.Replace([]byte(b), body, flag, 1), "ccgo_darwin_arm64.go"); err == nil || !strings.Contains(err.Error(), "changed (fingerprint") {
		t.Errorf("another flag: %v", err)
	}
	// A second return of the bytecode stops the change, whatever the
	// fingerprint.
	two := bytes.Replace(body, []byte("\tre_flags = 0\n"), []byte("\tre_flags = 0\n\tif ret.Ftag == 1 {\n\t\treturn ret\n\t}\n"), 1)
	if bytes.Equal(two, body) {
		t.Fatal("no return of the bytecode")
	}
	if _, err := addRegexCache(bytes.Replace([]byte(b), body, two, 1)); err == nil || !strings.Contains(err.Error(), "returns of ret") {
		t.Errorf("two returns: %v", err)
	}
}
