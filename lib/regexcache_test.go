// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// reVM is a runtime and a context for the tests of the regexp cache.
type reVM struct {
	t   testing.TB
	tls *libc.TLS
	rt  uintptr
	ctx uintptr
}

// newReVM makes a runtime whose regexp cache keeps n entries and bytes
// bytes, or the default when n is -1.
func newReVM(t testing.TB, n int32, bytes Tsize_t) *reVM {
	tls := libc.NewTLS()
	rt := XJS_NewRuntime(tls)
	ctx := XJS_NewContext(tls, rt)
	if n >= 0 {
		XJS_SetRegExpCache(tls, rt, n, bytes)
	}
	return &reVM{t, tls, rt, ctx}
}

func (v *reVM) close() {
	XJS_FreeContext(v.tls, v.ctx)
	XJS_FreeRuntime(v.tls, v.rt)
	v.tls.Close()
}

func (v *reVM) run(js string) string { return eval1(v.t, v.tls, v.ctx, js) }

// bytecode returns the bytecode of the regexp js evaluates to, which the
// global object keeps alive as keep.
func (v *reVM) bytecode(js string) uintptr {
	v.t.Helper()
	val := eval0(v.t, v.tls, v.ctx, "globalThis.keep = "+js)
	defer XFreeValue(v.tls, v.ctx, val)
	re := _js_get_regexp(v.tls, v.ctx, val, 0)
	if re == 0 {
		v.t.Fatalf("%s: %s is not a regexp", js, toString(v.t, v.tls, v.ctx, XDupValue(v.tls, v.ctx, val)))
	}
	return (*TJSRegExp)(unsafe.Pointer(re)).Fbytecode
}

// entries returns the patterns and flags the cache keeps, newest first, and
// checks the lists and the counts.
func (v *reVM) entries() (r []string) {
	v.t.Helper()
	rc := regexpCacheOf(v.rt)
	var bytes uintptr
	var prev uintptr
	for e := rc.newest; e != 0; e = regexpEntryOf(e).older {
		en := regexpEntryOf(e)
		if en.newer != prev {
			v.t.Fatalf("entry %d: newer is not the entry before", len(r))
		}
		prev = e
		found := false
		for c := *rc.chain(en.hash); c != 0; c = regexpEntryOf(c).chain {
			found = found || c == e
		}
		if !found {
			v.t.Fatalf("entry %d is not in its hash chain", len(r))
		}
		f := "undefined"
		if en.flags != 0 {
			f = v.str(en.flags)
		}
		r = append(r, v.str(en.pattern)+" "+f)
		bytes += en.size
	}
	if rc.oldest != prev {
		v.t.Fatal("oldest is not the last entry")
	}
	if len(r) != int(rc.count) || bytes != rc.bytes {
		v.t.Fatalf("%d entries of %d bytes, count %d of %d bytes", len(r), bytes, rc.count, rc.bytes)
	}
	return r
}

// str returns the JSString p as Go text.
func (v *reVM) str(p uintptr) string {
	(*TJSMallocBlockHeader)(unsafe.Pointer(_js_rc(v.tls, p))).Fref_count++
	return toString(v.t, v.tls, v.ctx, jsvMkPtr(EJS_TAG_STRING, p))
}

// mallocSize is the memory the runtime has from its allocator.
func (v *reVM) mallocSize() Tsize_t {
	return (*TJSRuntime)(unsafe.Pointer(v.rt)).Fmalloc_ctx.Fmalloc_state.Fmalloc_size
}

// testdata/regexcache.js makes regexps from the same patterns and flags
// again and again, in every way that compiles one. testdata/regexcache.txt
// is what Node 24 gives, and the engine gives the same with the cache at its
// defaults, at a few entries and off.
func TestRegexCacheSemantics(t *testing.T) {
	js, err := os.ReadFile(filepath.Join("testdata", "regexcache.js"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "regexcache.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		n     int32
		bytes Tsize_t
	}{{"default", -1, 0}, {"small", 4, 1 << 20}, {"off", 0, 0}} {
		v := newReVM(t, c.n, c.bytes)
		got := v.run(string(js))
		if c.n != 0 && regexpCacheOf(v.rt).count == 0 {
			t.Errorf("%s: no regexp kept", c.name)
		}
		v.close()
		if got == strings.TrimSuffix(string(want), "\n") {
			continue
		}
		gl, wl := strings.Split(got, "\n"), strings.Split(strings.TrimSuffix(string(want), "\n"), "\n")
		errs := 0
		for i := 0; (i < len(gl) || i < len(wl)) && errs < 10; i++ {
			var g, w string
			if i < len(gl) {
				g = gl[i]
			}
			if i < len(wl) {
				w = wl[i]
			}
			if g != w {
				t.Errorf("%s: line %d:\n got %s\nwant %s", c.name, i+1, g, w)
				errs++
			}
		}
	}
}

// A regexp made again from the same pattern and flags, whichever way,
// shares the bytecode the first one made. Other flags, or a pattern of
// other characters, compile again.
func TestRegexCacheShares(t *testing.T) {
	v := newReVM(t, -1, 0)
	defer v.close()
	lit := v.bytecode("/a+b/g")
	for _, js := range []string{
		"new RegExp('a+b', 'g')",
		"RegExp('a+b', 'g')",
		"new RegExp(['a', '+b'].join(''), 'g')",
		"new RegExp(new RegExp('a+b'), 'g')",
		"/x/.compile('a+b', 'g')",
		"eval('/a+b/g')",
	} {
		if got := v.bytecode(js); got != lit {
			t.Errorf("%s: bytecode %#x, the literal has %#x", js, got, lit)
		}
	}
	for _, js := range []string{"new RegExp('a+b', 'gi')", "new RegExp('a+b')", "new RegExp('a+b', '')", "new RegExp('a+c', 'g')"} {
		if got := v.bytecode(js); got == lit {
			t.Errorf("%s: the bytecode of /a+b/g", js)
		}
	}
	// Undefined flags and "" are separate entries, with the same bytecode.
	if a, b := v.bytecode("new RegExp('q')"), v.bytecode("new RegExp('q', '')"); a == b {
		t.Error("undefined flags and '' share an entry")
	}
	if a, b := v.bytecode("new RegExp('q')"), v.bytecode("new RegExp('q', undefined)"); a != b {
		t.Error("undefined flags compile again")
	}
	// A pattern of 16-bit characters matches one of 8-bit characters with
	// the same code units.
	if a, b := v.bytecode("new RegExp('aé')"), v.bytecode("new RegExp('aé中'.slice(0, 2))"); a != b {
		t.Error("the same pattern in 16-bit characters compiles again")
	}
	// The objects share the bytecode, not lastIndex.
	if got := v.run("var r1 = new RegExp('o', 'g'), r2 = new RegExp('o', 'g'); r1.exec('foo'); [r1.lastIndex, r2.lastIndex, r2.exec('foo').index, r1.exec('foo').index].join()"); got != "2,0,1,2" {
		t.Errorf("lastIndex: %s", got)
	}
}

// Only a pattern that is a string with flags that are undefined or a string
// is kept: anything else is converted again each time, which can run
// javascript. A compile that fails keeps nothing.
func TestRegexCacheKeeps(t *testing.T) {
	v := newReVM(t, -1, 0)
	defer v.close()
	v.run("var n = 0, f = {toString() { n++; return 'g' }}, p = {toString() { n++; return 'x' }}; 0")
	for range 3 {
		v.run("new RegExp('a', f); new RegExp(p, 'g'); new RegExp(new String('b')); new RegExp('c', new String('i')); 0")
	}
	if got := v.run("n"); got != "6" {
		t.Errorf("toString ran %s times, want 6", got)
	}
	// The constructor converts a pattern that is not a regexp to a string
	// before it compiles: that string is kept. Flags it hands on as they are.
	got := strings.Join(v.entries(), ",")
	if want := "b undefined,x g"; got != want {
		t.Errorf("entries %q, want %q", got, want)
	}
	before := regexpCacheOf(v.rt).count
	for _, js := range []string{"new RegExp('(')", "new RegExp('a', 'gg')", "new RegExp('a', 'uv')", "new RegExp('a', 'q')", "new RegExp('\\\\p{Nope}', 'u')"} {
		for range 2 {
			if got := v.run("try { " + js + "; 'made' } catch (e) { e.name }"); got != "SyntaxError" {
				t.Errorf("%s: %s, want SyntaxError", js, got)
			}
		}
	}
	if got := regexpCacheOf(v.rt).count; got != before {
		t.Errorf("%d entries after compiles that fail, want %d", got, before)
	}
}

// The cache keeps the entries used last, at most n of them, and at most its
// bytes.
func TestRegexCacheLRU(t *testing.T) {
	v := newReVM(t, 3, 1<<20)
	defer v.close()
	v.run("new RegExp('a'); new RegExp('b'); new RegExp('c'); new RegExp('a'); new RegExp('d'); 0")
	if got, want := strings.Join(v.entries(), ","), "d undefined,a undefined,c undefined"; got != want {
		t.Errorf("entries %q, want %q", got, want)
	}
	// A hit moves the entry to the front.
	v.run("new RegExp('c'); 0")
	if got, want := strings.Join(v.entries(), ","), "c undefined,d undefined,a undefined"; got != want {
		t.Errorf("entries %q, want %q", got, want)
	}
	// The bytes: entries of about 1 KiB each, in a cache of 3 KiB.
	XJS_SetRegExpCache(v.tls, v.rt, 100, 3<<10)
	v.run("new RegExp('x'.repeat(200) + 'a'); 0")
	one := regexpCacheOf(v.rt).bytes
	if one < 700 || one > 1500 {
		t.Fatalf("an entry of %d bytes", one)
	}
	v.run("for (var i = 0; i < 10; i++) new RegExp('x'.repeat(200) + i); 0")
	got := v.entries()
	if len(got) != int(3<<10/one) || !strings.HasSuffix(got[0], "9 undefined") {
		t.Errorf("%d entries of %d bytes, newest %q", len(got), one, got[0][len(got[0])-12:])
	}
	if rc := regexpCacheOf(v.rt); rc.bytes > 3<<10 {
		t.Errorf("%d bytes kept in a cache of %d", rc.bytes, 3<<10)
	}
	// A regexp larger than the cache is not kept.
	v.run("new RegExp('y'.repeat(2000)); 0")
	if got := v.entries(); strings.HasPrefix(got[0], "yyy") {
		t.Error("a regexp larger than the cache was kept")
	}
	// Off, it keeps nothing, and the memory of the entries is free again.
	XJS_SetRegExpCache(v.tls, v.rt, 0, 0)
	if rc := regexpCacheOf(v.rt); rc.table != 0 || rc.count != 0 || rc.newest != 0 {
		t.Errorf("not empty after off: %+v", *rc)
	}
	v.run("new RegExp('a'); new RegExp('a'); 0")
	if n := regexpCacheOf(v.rt).count; n != 0 {
		t.Errorf("%d entries with the cache off", n)
	}
}

// A cache of more entries than fit in its bytes, or than a table could
// chain on a 32-bit target, gets a table for the entries that fit, at most
// regexpMaxBuckets chains, and works.
func TestRegexCacheBuckets(t *testing.T) {
	size := unsafe.Sizeof(tregexpEntry{})
	// fits is the number of chains for the entries that fit in bytes, which
	// depends on the size of an entry on the target.
	fits := func(bytes uintptr) uint32 {
		n := uint32(16)
		for uintptr(n) < bytes/size {
			n *= 2
		}
		return n
	}
	for _, c := range []struct {
		n     int32
		bytes uintptr
		want  uint32
	}{
		{0, 1 << 20, 16},
		{3, 1 << 20, 16},
		{256, 1 << 20, 256},
		{257, 1 << 20, 512},
		{1 << 30, 4 << 10, fits(4 << 10)},
		{1<<31 - 1, 1 << 20, fits(1 << 20)},
		{1<<31 - 1, ^uintptr(0), regexpMaxBuckets},
	} {
		if got := regexpBuckets(c.n, c.bytes); got != c.want {
			t.Errorf("regexpBuckets(%d, %d) = %d, want %d", c.n, c.bytes, got, c.want)
		}
	}
	for _, c := range []struct {
		n     int32
		bytes Tsize_t
	}{{1<<31 - 1, 4 << 10}, {1 << 30, 1 << 20}, {1<<31 - 1, ^Tsize_t(0)}} {
		v := newReVM(t, c.n, c.bytes)
		before := v.mallocSize()
		if got := v.run("for (var i = 0; i < 100; i++) new RegExp('(a|b)+c' + i); [new RegExp('(a|b)+c7').test('xabc7'), new RegExp('(a|b)+c99').exec('ac99')[0]].join()"); got != "true,ac99" {
			t.Errorf("cache of %d in %d bytes: %s", c.n, c.bytes, got)
		}
		rc := regexpCacheOf(v.rt)
		if rc.buckets > regexpMaxBuckets || uintptr(rc.buckets) > uintptr(c.bytes)/size && rc.buckets > 16 {
			t.Errorf("cache of %d in %d bytes: %d chains", c.n, c.bytes, rc.buckets)
		}
		// At most 512 KiB of table and the 100 regexps, not a table for
		// every entry n allows.
		if grown := v.mallocSize() - before; grown > 2<<20 {
			t.Errorf("cache of %d in %d bytes: %d bytes more", c.n, c.bytes, grown)
		}
		if uintptr(rc.bytes) > uintptr(c.bytes) {
			t.Errorf("cache of %d in %d bytes keeps %d", c.n, c.bytes, rc.bytes)
		}
		v.entries()
		v.close()
	}
}

// A regexp whose entry the cache let go keeps its bytecode: the entry
// held one reference and the object holds its own.
func TestRegexCacheEvicted(t *testing.T) {
	v := newReVM(t, 4, 1<<20)
	defer v.close()
	v.run("globalThis.re = new RegExp('a'); for (var i = 0; i < 5; i++) new RegExp('b' + i); 0")
	for _, e := range v.entries() {
		if e == "a undefined" {
			t.Fatal("the entry of a is still kept")
		}
	}
	XJS_RunGC(v.tls, v.rt)
	if got := v.run("[re.exec('xa').index, re.test('b'), String(re)].join()"); got != "1,false,/a/" {
		t.Errorf("after the entry went: %s", got)
	}
	// Compiling a again makes a new entry, which the old object does not
	// share.
	v.run("globalThis.re2 = new RegExp('a'); 0")
	if got := v.run("[re.exec('a').index, re2.exec('ba').index].join()"); got != "0,1" {
		t.Errorf("with a new entry: %s", got)
	}
}

// The entries, their table and the strings they hold are the memory the
// cache takes: turning it off gives all of it back, and JS_FreeRuntime frees
// a runtime with entries.
func TestRegexCacheMemory(t *testing.T) {
	v := newReVM(t, 0, 0)
	defer v.close()
	js := "for (var i = 0; i < 300; i++) new RegExp('(a|b)+c' + i, 'g'); 0"
	v.run(js)
	XJS_RunGC(v.tls, v.rt)
	off := v.mallocSize()
	XJS_SetRegExpCache(v.tls, v.rt, 256, 512<<10)
	v.run(js)
	XJS_RunGC(v.tls, v.rt)
	on := v.mallocSize()
	rc := regexpCacheOf(v.rt)
	if rc.count != 256 || on <= off {
		t.Fatalf("%d entries, %d bytes with the cache, %d without", rc.count, on, off)
	}
	// The cache counts the usable size of each block: the allocator rounds
	// up and adds a header, which the count of malloc_size has too.
	if kept := on - off; uintptr(kept) < rc.bytes/2 || uintptr(kept) > rc.bytes*2+16<<10 {
		t.Errorf("the cache takes %d bytes and counts %d", kept, rc.bytes)
	}
	XJS_SetRegExpCache(v.tls, v.rt, 0, 0)
	XJS_RunGC(v.tls, v.rt)
	if got := v.mallocSize(); got != off {
		t.Errorf("%d bytes after the cache let go, %d before it", got, off)
	}
	// JS_FreeRuntime lets go of what a runtime keeps (its assertion on
	// leaked strings would fire otherwise).
	w := newReVM(t, -1, 0)
	w.run(js)
	w.close()
}

// Entries count against the memory limit, and a runtime that runs out of
// memory while it compiles keeps what it kept and nothing half made: the
// compile fails or makes the regexp, and the cache stays whole.
func TestRegexCacheOutOfMemory(t *testing.T) {
	ooms := 0
	for limit := Tsize_t(0); limit < 32*8<<10; limit += 8 << 10 {
		v := newReVM(t, -1, 0)
		v.run("new RegExp('k+', 'g'); 0")
		XJS_RunGC(v.tls, v.rt)
		XJS_SetMemoryLimit(v.tls, v.rt, v.mallocSize()+limit)
		got := v.run("var a = []; for (var i = 0; i < 200; i++) a.push(new RegExp('(x|y)+z' + i, 'g'), new RegExp('k+', 'g')); a.length")
		XJS_SetMemoryLimit(v.tls, v.rt, ^Tsize_t(0))
		// The engine throws null when it cannot make the error either.
		if got != "400" && !strings.Contains(got, "out of memory") && got != "<exception=null>" {
			t.Errorf("limit +%d: %s", limit, got)
		}
		if got != "400" {
			ooms++
		}
		v.entries()
		if got := v.run("[new RegExp('(x|y)+z3', 'g').test('xz3'), new RegExp('k+', 'g').exec('akk')[0]].join()"); got != "true,kk" {
			t.Errorf("limit +%d: after: %s", limit, got)
		}
		v.close()
	}
	if ooms == 0 || ooms == 32 {
		t.Errorf("%d of 32 limits ran out of memory", ooms)
	}
}

// Each runtime has a cache of its own: runtimes on several goroutines make
// and use the same regexps at once.
func TestRegexCacheRuntimes(t *testing.T) {
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for g := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := newReVM(t, 32, 1<<20)
			defer v.close()
			js := fmt.Sprintf("(() => { let s = 0; for (let i = 0; i < 2000; i++) { if (new RegExp('g%d-' + (i %% 50) + '$').test('g%d-' + (i %% 50))) s++ } return s })()", g, g)
			for range 3 {
				if got := v.run(js); got != "2000" {
					errs <- fmt.Errorf("runtime %d: %s", g, got)
					return
				}
			}
			if c := regexpCacheOf(v.rt).count; c != 32 {
				errs <- fmt.Errorf("runtime %d: %d entries", g, c)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
