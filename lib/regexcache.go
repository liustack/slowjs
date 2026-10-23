// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"unsafe"

	"modernc.org/libc"
)

// The regexp cache of a runtime keeps the bytecode of the regexps it
// compiled, by pattern and flags, so that compiling the same pattern with
// the same flags again takes the bytecode it made before instead of
// running lre_compile. internal/fixlib wires it in (see fixRegexCache):
//
//   - js_compile_regexp, which compiles every regexp (a literal when the
//     parser meets it, new RegExp, RegExp.prototype.compile, and the
//     regexps that split and the other built-ins make from a regexp with
//     new flags), calls regexpCacheGet first, and returns the bytecode it
//     gives.
//   - js_compile_regexp calls regexpCachePut with the bytecode it made.
//   - JS_FreeRuntime calls regexpCacheOff first, which lets every entry go.
//
// The bytecode of a regexp is an 8-bit string that the engine never
// changes once lre_compile wrote it: lre_exec and the other readers take
// it as const, and lastIndex is a property of each RegExp object, not of
// its bytecode. The engine already shares it between objects: every
// object a regexp literal makes holds the bytecode of the literal, and
// new RegExp(re) holds the bytecode of re. So objects made from the cache
// share it too.
//
// Only a pattern that is a string, not a rope, with flags that are
// undefined or a string, not a rope, is looked up and kept: converting
// anything else to a string can run javascript (toString and the like)
// or allocate, and js_compile_regexp does both before it compiles. For
// such a pattern and such flags js_compile_regexp only reads them, so a
// hit skips work nothing sees. What a hit can change: compiling can fail
// where the cached bytecode does not, when there is no memory left or the
// pattern nests deeper than the stack has room for at that call. A
// compile that fails is not kept, so every pattern and flags that throw
// a SyntaxError throw it every time.
//
// An entry holds one reference to the pattern, to the flags and to the
// bytecode, and is kept in C memory from js_malloc_rt, as is the table of
// its hash chains: the cache counts against the memory limit of the
// runtime. The cache keeps at most RegExpCacheEntries entries and
// RegExpCacheBytes bytes (the usable size of the entry and of the three
// strings), or what JS_SetRegExpCache set, and lets go of the entry used
// least recently first. The table of hash chains is not in those bytes: it
// has one pointer per entry that fits, up to regexpMaxBuckets, and counts
// against the memory limit only. It does not let go of its entries when an
// allocation fails: freeing a string can free an atom, and the allocation
// that fails can be one in the middle of growing the atom table.
//
// The state of the cache is in the state of the runtime (see runtime.go),
// so each runtime has its own, used only by the goroutine that runs it. A
// new runtime starts with the cache on, at the default limits.

// RegExpCacheEntries and RegExpCacheBytes are the default limits of the
// regexp cache of a runtime. The largest set measured, the post of the
// storefront of tifa's example shop that highlights CSS (react-markdown
// makes a new highlighter for each render, which compiles the regexps of
// the CSS grammar again), compiles 392 regexps per render, 162 of them
// different, which take 714 KiB with their patterns: they fit with room to
// spare. The other pages of the shop and of an app built on MUI compile at
// most 83 per render, of at most 11 patterns, in under 15 KiB.
const (
	RegExpCacheEntries = 256
	RegExpCacheBytes   = 1 << 20
)

// TregexpCache is the regexp cache of a runtime. Its zero value is a cache
// with the default limits and no entries.
type TregexpCache struct {
	table    uintptr // the first entry of each hash chain, from js_malloc_rt, 0 until the first entry is kept
	buckets  uint32  // the number of hash chains of table, a power of 2
	count    int32   // the number of entries
	newest   uintptr // the entry used last
	oldest   uintptr // the entry used least recently
	bytes    uintptr // the bytes the entries count
	set      int32   // nonzero once JS_SetRegExpCache set maxCount and maxBytes
	maxCount int32   // the number of entries kept at most, 0 when the cache is off
	maxBytes uintptr // the bytes kept at most
}

// tregexpEntry is an entry of the regexp cache, in C memory.
type tregexpEntry struct {
	chain   uintptr // the next entry of the same hash chain
	newer   uintptr // the entry used next after this one
	older   uintptr // the entry used before this one
	pattern uintptr // the JSString of the pattern
	flags   uintptr // the JSString of the flags, or 0 for undefined
	bc      uintptr // the JSString of the bytecode
	size    uintptr // the bytes this entry counts
	hash    uint32
}

// regexpMaxBuckets is the most hash chains a regexp cache has: its table
// takes at most 512 KiB on 64-bit targets, whatever its limits.
const regexpMaxBuckets = 1 << 16

// regexpBuckets returns the number of hash chains of a cache of maxCount
// entries in maxBytes bytes: the power of 2, from 16 up to
// regexpMaxBuckets, at or above the number of entries that fit. A
// larger cache has longer chains instead of a table that could take more
// memory than its entries, or whose size in bytes would not fit in a
// size_t on 32-bit targets.
func regexpBuckets(maxCount int32, maxBytes uintptr) uint32 {
	fit := uint64(maxBytes / unsafe.Sizeof(tregexpEntry{}))
	if uint64(maxCount) < fit {
		fit = uint64(maxCount)
	}
	n := uint32(16)
	for uint64(n) < fit && n < regexpMaxBuckets {
		n *= 2
	}
	return n
}

// regexpCacheOf returns the regexp cache of the runtime at rt.
func regexpCacheOf(rt uintptr) *TregexpCache {
	return &forkOf(rt).regexp
}

// limits returns the number of entries and the bytes rc keeps at most.
func (rc *TregexpCache) limits() (int32, uintptr) {
	if rc.set == 0 {
		return RegExpCacheEntries, RegExpCacheBytes
	}
	return rc.maxCount, rc.maxBytes
}

func regexpEntryOf(p uintptr) *tregexpEntry { return (*tregexpEntry)(unsafe.Pointer(p)) }

// chain returns the address of the first entry of the hash chain of h.
func (rc *TregexpCache) chain(h uint32) *uintptr {
	return (*uintptr)(unsafe.Pointer(rc.table + uintptr(h&(rc.buckets-1))*unsafe.Sizeof(uintptr(0))))
}

// XJS_SetRegExpCache sets the regexp cache of the runtime rt to keep at
// most n entries and at most bytes bytes, and lets go of the entries it
// kept before. An n or bytes of 0 turns the cache off.
func XJS_SetRegExpCache(tls *libc.TLS, rt uintptr, n int32, bytes Tsize_t) {
	regexpCacheOff(tls, rt)
	rc := regexpCacheOf(rt)
	rc.set, rc.maxCount, rc.maxBytes = 1, 0, 0
	if n > 0 && bytes > 0 {
		rc.maxCount, rc.maxBytes = n, uintptr(bytes)
	}
}

// regexpCacheOff lets go of every entry of the regexp cache of the runtime
// at rt and of its table. The limits stay.
func regexpCacheOff(tls *libc.TLS, rt uintptr) {
	rc := regexpCacheOf(rt)
	e := rc.newest
	table := rc.table
	rc.table, rc.buckets, rc.count, rc.newest, rc.oldest, rc.bytes = 0, 0, 0, 0, 0, 0
	for e != 0 {
		next := regexpEntryOf(e).older
		regexpEntryFree(tls, rt, e)
		e = next
	}
	if table != 0 {
		Xjs_free_rt(tls, rt, table)
	}
}

// regexpEntryFree lets go of the references of the entry e and frees it.
func regexpEntryFree(tls *libc.TLS, rt, e uintptr) {
	en := regexpEntryOf(e)
	_js_free_string(tls, rt, en.pattern)
	if en.flags != 0 {
		_js_free_string(tls, rt, en.flags)
	}
	_js_free_string(tls, rt, en.bc)
	Xjs_free_rt(tls, rt, e)
}

// regexpKey returns the strings of pattern and flags (0 for undefined
// flags), and whether the cache can look them up.
func regexpKey(pattern, flags TJSValue) (p, f uintptr, ok bool) {
	if jsvTag(pattern) != EJS_TAG_STRING {
		return 0, 0, false
	}
	switch jsvTag(flags) {
	case EJS_TAG_UNDEFINED:
		return jsvPtr(pattern), 0, true
	case EJS_TAG_STRING:
		return jsvPtr(pattern), jsvPtr(flags), true
	}
	return 0, 0, false
}

// regexpHash hashes the pattern p and the flags f.
func regexpHash(tls *libc.TLS, p, f uintptr) uint32 {
	h := _hash_string(tls, p, 0)
	if f != 0 {
		h = _hash_string(tls, f, h*263+1)
	}
	return h
}

// sameString reports whether the strings a and b, either of which can be
// 0, hold the same code units.
func sameString(tls *libc.TLS, ctx, a, b uintptr) bool {
	if a == b {
		return true
	}
	if a == 0 || b == 0 {
		return false
	}
	return _js_string_compare(tls, ctx, a, b) == 0
}

// regexpCacheGet is called first by js_compile_regexp(ctx, pattern,
// flags). When the cache holds bytecode for pattern and flags, it returns
// a new reference to it, and ok.
func regexpCacheGet(tls *libc.TLS, ctx uintptr, pattern, flags TJSValue) (bc TJSValue, ok bool) {
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	rc := regexpCacheOf(rt)
	if rc.count == 0 {
		return bc, false
	}
	p, f, ok := regexpKey(pattern, flags)
	if !ok {
		return bc, false
	}
	h := regexpHash(tls, p, f)
	// The hash and the compare read the whole pattern, which the
	// interpreter counts as one instruction: count a step for each 32
	// bytes too.
	_js_charge_interrupts(ctx, Tsize_t(stringBytes(p)>>5))
	for e := *rc.chain(h); e != 0; e = regexpEntryOf(e).chain {
		en := regexpEntryOf(e)
		if en.hash != h || !sameString(tls, ctx, en.pattern, p) || !sameString(tls, ctx, en.flags, f) {
			continue
		}
		if rc.newest != e {
			rc.unlink(e)
			rc.pushNewest(e)
		}
		(*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, en.bc))).Fref_count++
		return jsvMkPtr(EJS_TAG_STRING, en.bc), true
	}
	return bc, false
}

// unlink takes the entry e out of the list by use. It stays in its hash
// chain.
func (rc *TregexpCache) unlink(e uintptr) {
	en := regexpEntryOf(e)
	if en.newer != 0 {
		regexpEntryOf(en.newer).older = en.older
	} else {
		rc.newest = en.older
	}
	if en.older != 0 {
		regexpEntryOf(en.older).newer = en.newer
	} else {
		rc.oldest = en.newer
	}
	en.newer, en.older = 0, 0
}

// pushNewest puts the entry e at the newest end of the list by use.
func (rc *TregexpCache) pushNewest(e uintptr) {
	en := regexpEntryOf(e)
	en.older = rc.newest
	if rc.newest != 0 {
		regexpEntryOf(rc.newest).newer = e
	} else {
		rc.oldest = e
	}
	rc.newest = e
}

// dropOldest lets go of the entry used least recently.
func (rc *TregexpCache) dropOldest(tls *libc.TLS, rt uintptr) {
	e := rc.oldest
	en := regexpEntryOf(e)
	for at := rc.chain(en.hash); ; at = &regexpEntryOf(*at).chain {
		if *at == e {
			*at = en.chain
			break
		}
	}
	rc.unlink(e)
	rc.count--
	rc.bytes -= en.size
	regexpEntryFree(tls, rt, e)
}

// regexpCachePut is called by js_compile_regexp(ctx, pattern, flags) with
// the bytecode bc it made, before it returns it. It keeps a reference to
// pattern, flags and bc in the cache, if the cache can look them up and
// has room for them. When there is no memory for the entry it keeps
// nothing, and leaves no exception.
func regexpCachePut(tls *libc.TLS, ctx uintptr, pattern, flags, bc TJSValue) {
	if jsvTag(bc) != EJS_TAG_STRING {
		return
	}
	p, f, ok := regexpKey(pattern, flags)
	if !ok {
		return
	}
	rt := (*TJSContext)(unsafe.Pointer(ctx)).Frt
	rc := regexpCacheOf(rt)
	maxCount, maxBytes := rc.limits()
	if maxCount <= 0 {
		return
	}
	b := jsvPtr(bc)
	size := uintptr(unsafe.Sizeof(tregexpEntry{})) + uintptr(Xjs_malloc_usable_size_rt(tls, rt, b)) + uintptr(Xjs_malloc_usable_size_rt(tls, rt, p))
	if f != 0 {
		size += uintptr(Xjs_malloc_usable_size_rt(tls, rt, f))
	}
	if size > maxBytes {
		return
	}
	if rc.table == 0 {
		n := regexpBuckets(maxCount, maxBytes)
		table := Xjs_mallocz_rt(tls, rt, Tsize_t(n)*Tsize_t(unsafe.Sizeof(uintptr(0))))
		if table == 0 {
			return
		}
		rc.table, rc.buckets = table, n
	}
	// rc.bytes never passes maxBytes, so maxBytes-rc.bytes cannot wrap,
	// where rc.bytes+size could on 32-bit targets.
	for rc.count > 0 && (rc.count >= maxCount || size > maxBytes-rc.bytes) {
		rc.dropOldest(tls, rt)
	}
	e := Xjs_mallocz_rt(tls, rt, Tsize_t(unsafe.Sizeof(tregexpEntry{})))
	if e == 0 {
		return
	}
	en := regexpEntryOf(e)
	en.pattern, en.flags, en.bc, en.size = p, f, b, size
	en.hash = regexpHash(tls, p, f)
	for _, s := range []uintptr{p, f, b} {
		if s != 0 {
			(*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(tls, s))).Fref_count++
		}
	}
	at := rc.chain(en.hash)
	en.chain = *at
	*at = e
	rc.pushNewest(e)
	rc.count++
	rc.bytes += size
}
