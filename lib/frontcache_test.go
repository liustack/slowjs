// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package libquickjs // import "github.com/liustack/quickjs/lib"

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// The state of the fork is the last field of JSRuntime, right after the
// fields of the C struct, and JS_NewRuntime2 allocates and clears the C
// struct with it.
func TestRuntimeField(t *testing.T) {
	var rt TJSRuntime
	if off := unsafe.Offsetof(rt.Fmalloc_ctx); off != 0 {
		t.Fatalf("malloc_ctx at %d: the allocator's state must be at the start of JSRuntime", off)
	}
	if end := unsafe.Offsetof(rt.Ffork) + forkRuntimeSize; end != unsafe.Sizeof(rt) {
		t.Fatalf("the state of the fork ends at %d of %d", end, unsafe.Sizeof(rt))
	}
	b, err := os.ReadFile(fmt.Sprintf("ccgo_%s_%s.go", runtime.GOOS, runtime.GOARCH))
	if os.IsNotExist(err) && runtime.GOOS == "windows" {
		b, err = os.ReadFile("ccgo_windows.go")
	}
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`libc\.X(?:memset|__builtin___memset_chk)\(tls, rt, 0, uint(?:32|64)\((\d+)\+forkRuntimeSize\)`).FindSubmatch(b)
	if m == nil {
		t.Fatal("JS_NewRuntime2 does not clear the runtime with the state of the fork")
	}
	if n, _ := strconv.Atoi(string(m[1])); uintptr(n) != unsafe.Offsetof(rt.Ffork) {
		t.Fatalf("JS_NewRuntime2 allocates %d bytes for the C struct, which ends at %d", n, unsafe.Offsetof(rt.Ffork))
	}
}

// frontSizes are sizes of small blocks, one for each block size of the
// allocator, and a few in between.
var frontSizes = []Tsize_t{1, 8, 9, 16, 24, 100, 120, 121, 200, 248, 249, 300, 504}

// A freed small block goes into the front cache of its size, the next
// allocation of that size takes it back, and the cache keeps at most
// frontMax blocks of a size.
func TestFrontCacheReuse(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	fc := frontCacheOf(rt)

	for _, size := range frontSizes {
		p := Xjs_malloc_rt(tls, rt, size)
		if p == 0 {
			t.Fatal("out of memory")
		}
		idx := (*TJSMallocBlockHeader)(unsafe.Pointer(p - frontHeader)).Fblock_size_idx
		n := fc.count[idx]
		Xjs_free_rt(tls, rt, p)
		if fc.count[idx] != n+1 || fc.head[idx] != p-frontHeader {
			t.Errorf("size %d: %d blocks cached, head %#x, want %d, %#x", size, fc.count[idx], fc.head[idx], n+1, p-frontHeader)
		}
		if q := Xjs_malloc_rt(tls, rt, size); q != p || fc.count[idx] != n {
			t.Errorf("size %d: got %#x and %d blocks cached, want %#x and %d", size, q, fc.count[idx], p, n)
		}
		if got := Xjs_malloc_usable_size_rt(tls, rt, p); got < size {
			t.Errorf("size %d: %d usable", size, got)
		}
		Xjs_free_rt(tls, rt, p)
	}

	// A block too large for the small block allocator, and a block of size
	// 0, stay out of the cache.
	before := *fc
	for _, size := range []Tsize_t{0, 505, 4096} {
		Xjs_free_rt(tls, rt, Xjs_malloc_rt(tls, rt, size))
	}
	if *fc != before {
		t.Error("a large or empty block went into the cache")
	}

	// A realloc to size 0 frees the block, into the cache, and is no
	// failure that gives the cache back.
	p := Xjs_malloc_rt(tls, rt, 40)
	idx := (*TJSMallocBlockHeader)(unsafe.Pointer(p - frontHeader)).Fblock_size_idx
	cached := *fc
	if r := Xjs_realloc_rt(tls, rt, p, 0); r != 0 || fc.count[idx] != cached.count[idx]+1 || fc.head[idx] != p-frontHeader {
		t.Errorf("realloc to 0: %#x, %d blocks of the size cached, want 0, %d", r, fc.count[idx], cached.count[idx]+1)
	}

	var ps []uintptr
	for range frontMax + 10 {
		ps = append(ps, Xjs_malloc_rt(tls, rt, 40))
	}
	for _, p := range ps {
		Xjs_free_rt(tls, rt, p)
	}
	idx = (*TJSMallocBlockHeader)(unsafe.Pointer(ps[0] - frontHeader)).Fblock_size_idx
	if fc.count[idx] != frontMax {
		t.Errorf("%d blocks freed: %d cached, want %d", len(ps), fc.count[idx], frontMax)
	}
}

// JS_RunGC gives every cached block back to its arena, so an arena with no
// block in use is freed, and memory is back where it was. The blocks go to
// their arenas even when their lists in the cache have room for them.
func TestFrontCacheGC(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	XJS_RunGC(tls, rt)
	base := (*TJSRuntime)(unsafe.Pointer(rt)).Fmalloc_ctx.Fmalloc_state.Fmalloc_size

	for _, c := range []struct {
		name   string
		blocks int
	}{
		{"full lists", 4000},
		{"lists with room", 3 * len(frontSizes)},
	} {
		var ps []uintptr
		for i := range c.blocks {
			ps = append(ps, Xjs_malloc_rt(tls, rt, frontSizes[i%len(frontSizes)]))
		}
		for _, p := range ps {
			Xjs_free_rt(tls, rt, p)
		}
		XJS_RunGC(tls, rt)
		fc := frontCacheOf(rt)
		for i, n := range fc.count {
			if n != 0 || fc.head[i] != 0 {
				t.Errorf("%s: size index %d: %d blocks cached after JS_RunGC", c.name, i, n)
			}
		}
		if got := (*TJSRuntime)(unsafe.Pointer(rt)).Fmalloc_ctx.Fmalloc_state.Fmalloc_size; got != base {
			t.Errorf("%s: after JS_RunGC: %d bytes allocated, want %d", c.name, got, base)
		}
	}
}

// Running past the memory limit with blocks in the cache gives the cache
// back and tries again, so the cached blocks do not count against the
// memory limit for a block of another size. Without a limit, a failure of
// the allocator is no reason to give the cache back.
func TestFrontCacheOutOfMemory(t *testing.T) {
	tls := libc.NewTLS()
	defer tls.Close()
	rt := XJS_NewRuntime(tls)
	defer XJS_FreeRuntime(tls, rt)
	ms := &(*TJSRuntime)(unsafe.Pointer(rt)).Fmalloc_ctx.Fmalloc_state

	for _, c := range []struct {
		name string
		f    func() uintptr
	}{
		{"malloc", func() uintptr { return Xjs_malloc_rt(tls, rt, 64<<10) }},
		{"realloc", func() uintptr {
			p := Xjs_malloc_rt(tls, rt, 100)
			if p == 0 {
				return 0
			}
			return Xjs_realloc_rt(tls, rt, p, 64<<10)
		}},
	} {
		// frontMax blocks of 504 bytes take about 37 arenas of 4 KiB, and
		// stay in the cache once freed.
		XJS_SetMemoryLimit(tls, rt, ^Tsize_t(0))
		var ps []uintptr
		for range frontMax {
			ps = append(ps, Xjs_malloc_rt(tls, rt, 504))
		}
		XJS_SetMemoryLimit(tls, rt, ms.Fmalloc_size+4096)
		for _, p := range ps {
			Xjs_free_rt(tls, rt, p)
		}
		p := c.f()
		if p == 0 {
			t.Fatalf("%s: out of memory with %d bytes allocated, limit %d", c.name, ms.Fmalloc_size, ms.Fmalloc_limit)
		}
		Xjs_free_rt(tls, rt, p)
	}
	XJS_SetMemoryLimit(tls, rt, ^Tsize_t(0))
	p := Xjs_malloc_rt(tls, rt, 504)
	idx := (*TJSMallocBlockHeader)(unsafe.Pointer(p - frontHeader)).Fblock_size_idx
	Xjs_free_rt(tls, rt, p)
	if frontRetry(tls, rt) || frontCacheOf(rt).count[idx] == 0 {
		t.Error("without a memory limit, frontRetry gave the cache back")
	}
}

// Each runtime has a cache of its own, so runtimes on several goroutines
// allocate and free without sharing one.
func TestFrontCacheRuntimes(t *testing.T) {
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for g := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tls := libc.NewTLS()
			defer tls.Close()
			rt := XJS_NewRuntime(tls)
			defer XJS_FreeRuntime(tls, rt)
			fc := frontCacheOf(rt)
			for i := range 20000 {
				size := frontSizes[(i+g)%len(frontSizes)]
				p := Xjs_malloc_rt(tls, rt, size)
				if p == 0 {
					errs <- fmt.Errorf("runtime %d: out of memory", g)
					return
				}
				*(*byte)(unsafe.Pointer(p)) = byte(g)
				idx := (*TJSMallocBlockHeader)(unsafe.Pointer(p - frontHeader)).Fblock_size_idx
				k := fc.count[idx]
				Xjs_free_rt(tls, rt, p)
				if fc.count[idx] != k+1 && k < frontMax {
					errs <- fmt.Errorf("runtime %d: a freed block is not in its own cache", g)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
