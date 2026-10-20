// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
)

// The text fixFrontCache adds.
const (
	frontPopHook  = "\t// Changed by internal/fixlib: take a block of the front cache first (see fixFrontCache).\n\tif p := frontPop(tls, s, size); p != 0 {\n\t\treturn p\n\t}\n"
	frontPushHook = "\t// Changed by internal/fixlib: keep the block in the front cache (see fixFrontCache).\n\tif frontPush(s, ptr) {\n\t\treturn\n\t}\n"
	frontOffHook  = "\t// Changed by internal/fixlib: give the front cache back and turn it off (see fixFrontCache).\n\tfrontOff(tls, rt1)\n"
	frontGCHook   = "\t// Changed by internal/fixlib: give the front cache back to the arenas (see fixFrontCache).\n\tfrontDrain(tls, rt)\n"
	frontRetryDoc = "\t// Changed by internal/fixlib: past the memory limit, give the front cache back and try again (see fixFrontCache).\n"

	frontMallocOld  = "\treturn ___js_malloc(tls, rt, size)\n"
	frontMallocNew  = frontRetryDoc + "\tif r = ___js_malloc(tls, rt, size); r == 0 && frontRetry(tls, rt) {\n\t\tr = ___js_malloc(tls, rt, size)\n\t}\n\treturn r\n"
	frontReallocOld = "\treturn ___js_realloc(tls, rt, ptr, size)\n"
	// A realloc to size 0 frees the block and returns 0: that is no
	// failure to try again.
	frontReallocNew = frontRetryDoc + "\tif r = ___js_realloc(tls, rt, ptr, size); r == 0 && size != 0 && frontRetry(tls, rt) {\n\t\tr = ___js_realloc(tls, rt, ptr, size)\n\t}\n\treturn r\n"
)

var (
	reBlockHeader = regexp.MustCompile(`\ntype TJSMallocBlockHeader = struct \{\n`)
	reRunGC       = regexp.MustCompile(`^\{\n\t_JS_RunGCInternal\(tls, rt, int32\([EM]TRUE\)\)\n`)

	// The functions whose work lib/frontcache.go repeats or relies on.
	frontFuncs = []string{"___js_malloc", "___js_free", "___js_realloc", "_js_malloc_large", "_get_block_size_index", "_js_def_malloc", "_js_def_realloc", "Xjs_malloc_rt", "Xjs_realloc_rt", "XJS_RunGC"}
)

// fixFrontCache wires the front cache of lib/frontcache.go, in the field
// that fixRuntimeField adds to JSRuntime, into the small block allocator:
// it calls frontPop and frontPush at the start of __js_malloc and
// __js_free, frontDrain at the end of JS_RunGC, frontRetry when
// js_malloc_rt or js_realloc_rt runs out of memory, and frontOff at the
// start of JS_FreeRuntime.
//
// lib/frontcache.go sizes a block as __js_malloc does, reads the header of
// a block as __js_free does, tells a large block by the MFREE_NIL that
// js_malloc_large writes in its header, counts on __js_realloc moving a
// small block through __js_malloc and __js_free, and on js_def_malloc and
// js_def_realloc failing past the memory limit only. So the text of frontFuncs and of JSMallocBlockHeader
// is pinned in frontSites: when a release changes one of them,
// fixFrontCache stops, and the new code must be read against
// lib/frontcache.go before the fingerprints are updated (go run
// ./internal/fixlib -front prints them).
//
// Other changes match these functions as ccgo writes them, so
// fixFrontCache comes after them, and restoreFrontCache, its exact inverse,
// comes first.
func fixFrontCache(b []byte, file string) ([]byte, error) {
	bare, err := restoreFrontCache(b)
	if err != nil {
		return nil, err
	}
	got, err := frontFingerprint(bare)
	if err != nil {
		return nil, err
	}
	if want := frontSites[file]; got != want && !reporting() {
		return nil, fmt.Errorf("%s: %s changed (fingerprint %s, want %s): read them against lib/frontcache.go, then update frontSites (go run ./internal/fixlib -front)", file, strings.Join(frontFuncs, ", ")+" or JSMallocBlockHeader", got, want)
	}
	out, err := addFrontCache(bare)
	if err != nil {
		return nil, err
	}
	if back, err := restoreFrontCache(out); err != nil || !bytes.Equal(back, bare) {
		return nil, fmt.Errorf("fixFrontCache: restoreFrontCache does not give the input back (%v)", err)
	}
	if !bytes.Equal(bare, b) && !bytes.Equal(out, b) {
		return nil, fmt.Errorf("fixFrontCache: the file has a part of the change")
	}
	return out, nil
}

// addFrontCache makes the change in b, which has none of it.
func addFrontCache(b []byte) ([]byte, error) {
	var err error
	for _, h := range []struct{ name, hook string }{
		{"___js_malloc", frontPopHook},
		{"___js_free", frontPushHook},
		{"XJS_FreeRuntime", frontOffHook},
	} {
		if b, err = insertAfterDecls(b, h.name, h.hook); err != nil {
			return nil, err
		}
	}

	body, err := funcBody(b, funcHead("XJS_RunGC"), "XJS_RunGC")
	if err != nil {
		return nil, err
	}
	head := funcHead("XJS_RunGC").FindIndex(body)[1] - 2
	loc := reRunGC.FindIndex(body[head:])
	if loc == nil || string(body[head+loc[1]:]) != "}\n" {
		return nil, fmt.Errorf("XJS_RunGC: want only the call of _JS_RunGCInternal")
	}
	b = bytes.Replace(b, body, splice(body, head+loc[1], frontGCHook), 1)

	for _, r := range []struct{ name, old, new string }{
		{"Xjs_malloc_rt", frontMallocOld, frontMallocNew},
		{"Xjs_realloc_rt", frontReallocOld, frontReallocNew},
	} {
		if body, err = funcBody(b, funcHead(r.name), r.name); err != nil {
			return nil, err
		}
		head := funcHead(r.name).FindIndex(body)[1]
		if string(body[head:]) != r.old+"}\n" {
			return nil, fmt.Errorf("%s: want only the call of the allocator", r.name)
		}
		b = bytes.Replace(b, body, append(bytes.Clone(body[:head]), r.new+"}\n"...), 1)
	}
	return b, nil
}

// restoreFrontCache undoes fixFrontCache.
func restoreFrontCache(b []byte) ([]byte, error) {
	for _, r := range []struct{ new, old string }{
		{frontPopHook, ""},
		{frontPushHook, ""},
		{frontOffHook, ""},
		{frontGCHook, ""},
		{frontMallocNew, frontMallocOld},
		{frontReallocNew, frontReallocOld},
	} {
		if n := bytes.Count(b, []byte(r.new)); n > 1 {
			return nil, fmt.Errorf("restoreFrontCache: %d copies of %q", n, r.new)
		}
		b = bytes.Replace(b, []byte(r.new), []byte(r.old), 1)
	}
	for _, name := range []string{"frontPop(", "frontPush(", "frontDrain(", "frontRetry(", "frontOff("} {
		if bytes.Contains(b, []byte(name)) {
			return nil, fmt.Errorf("restoreFrontCache: a call of %s is left", name[:len(name)-1])
		}
	}
	return b, nil
}

// frontFingerprint hashes frontFuncs and JSMallocBlockHeader, as ccgo
// writes them, with each C string written out in place of its offset.
func frontFingerprint(b []byte) (string, error) {
	funcs, err := funcsFingerprint(b, frontFuncs, func(text []byte) []byte { return text })
	if err != nil {
		return "", err
	}
	st, err := structBody(b, reBlockHeader, "TJSMallocBlockHeader")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(funcs))
	h.Write(st)
	return fmt.Sprintf("%x", h.Sum(nil))[:16], nil
}
