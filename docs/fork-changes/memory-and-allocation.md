---
summary: "The fork changes to how the engine allocates: one block of TLS stack kept per VM outside linux so VMs stop waiting on libc's allocator lock, and a front cache of freed small blocks per runtime that js_malloc takes from without a call, with what each gains and costs in memory, why it is safe and what to do when upstream or libc changes"
read_when:
  - Changing tlsstack_other.go, lib/runtime.go, lib/frontcache.go, fixRuntimeField or fixFrontCache
  - fixlib stops on runtimeSites or frontSites after an upstream release
  - Several VMs render less than one VM times their number, or wait on a lock
  - Asking how much memory a VM holds after a GC, or what a VM keeps outside JS_ComputeMemoryUsage
  - Upgrading modernc.org/libc
---

# Memory and allocation

The engine allocates through libc's allocator, which has one lock for the whole process. These changes cut how often a VM goes to that allocator. The [shape cache](objects-and-shapes.md#kept-shapes) and the [calls without alloca](calls-and-tls-frames.md#calls-allocate-no-go-memory) also allocate less.

## One block of TLS stack per VM outside linux

Release: v0.26.15.

Outside linux, VMs on several goroutines no longer wait for each other on the TLS stack. There libc keeps that stack in segments of about 4 KiB, each malloc'd under the one lock of libc's allocator, and frees a segment once the stack is two segments below it. A render that goes a few calls up and down past the end of a segment so mallocs and frees segments again and again. Each runtime now allocates and frees one block of 255 segments (about 1 MiB) right after it creates its TLS (`warmTLS` in `tlsstack_other.go`), and libc keeps the block as the segment after the first, so every frame up to that depth goes on it and no call mallocs a segment.

Memory: each VM keeps that block until it is closed. libc's allocator maps it as a region of 1 MiB, and only the pages the TLS stack has reached are written, so it holds about as much memory as the deepest call of the VM took, at most 1 MiB, where it used to free those segments. On windows the allocator commits the whole region when it maps it, so each VM takes 1 MiB of the commit limit, though only the pages the stack reached are in memory. After `Close` the allocator keeps the region to hand out again, as it keeps every region it frees: the 4 MiB of regions of one size it freed last keep their written pages in memory, and older ones give their pages back to the system. A single frame larger than the block, far larger than any frame of the engine, would make libc free the block and malloc a segment of that size in its place. A render of tifa's example shop with 10 VMs on darwin/arm64 (Apple M4, `GOMAXPROCS=10`) went from 215 to 281 renders a second to 331 to 390, 1.4 to 1.55 times as many in each pair of runs, and the time the render goroutines spent waiting for a lock from 3.8% to 5.3% of it to about 0.5%. One VM renders as fast as before. Linux is not changed: libc keeps one block per slot there and never frees it.

Test: `tlswarm_other_test.go`.

## A front cache for small blocks

Release: v0.26.16.

Each runtime has a front cache in front of the small block allocator. A freed block of 16 to 512 bytes went back to its arena: the free list of the arena, its count of used blocks and the lists of arenas changed on every free, and again on the next allocation of that size. Now `__js_free` puts the block on a list of its size, up to 256 blocks a size (1.6 MiB at most), and `__js_malloc` takes it from there (`lib/frontcache.go`). A cached block keeps its header, so to the arena and to `js_malloc_usable_size` it is a block in use. `JS_RunGC` gives every cached block back to its arena, so after a GC the memory a runtime holds is what it was without the cache, give or take how the blocks fell in the arenas: one VM of the shop holds 48 KiB more after a GC (7.37 MiB rather than 7.33). With a memory limit set, an allocation that fails gives the cache back and is tried once more, so cached blocks never make an allocation of another size fail. This holds for the blocks of this cache only: the [shape cache](objects-and-shapes.md#kept-shapes) does not give its shapes back. A cached block also keeps its arena of 4 KiB from being freed until the next GC: with every list full and each block in an arena of its own, that is about 31 MiB, which the GC threshold, as it counts those arenas, and the retry past the memory limit get back. `JS_FreeRuntime` gives the cache back first and turns it off. The cache lives in a field that `fixlib` adds at the end of `JSRuntime` (`Ffork`, the state of `lib/runtime.go`), which `JS_NewRuntime2` allocates and clears with the C struct, so each runtime has its own, used only by the goroutine that runs it, and the fields of the C struct keep their offsets (`internal/fixlib/runtimefield.go`). Alone it makes a render 0.9% faster with PGO and leaves it about as it was without. The [shape cache](objects-and-shapes.md#kept-shapes) allocates and frees more: with it, the front cache and the interrupt countdown take another 0.9% off a render without PGO. Generated Go only, no C hunk (`internal/fixlib/frontcache.go`).

Tests: `lib/frontcache_test.go`, `alloc_test.go` (8 VMs on 8 goroutines, the same results, and memory after a GC that does not grow), `internal/fixlib/frontcache_test.go` and `internal/fixlib/runtimefield_test.go`.

### When upstream changes it

What to do when `go run ./internal/fixlib` stops on this change after an upstream release. [upgrading.md](../upgrading.md) has the whole procedure and the order in which to update the lists `fixlib` checks against.

If it reports that `JSRuntime` or `JS_NewRuntime2` changed (`internal/fixlib/runtimesites.go`), check that `malloc_ctx` is still the first field and that `JS_NewRuntime2` still allocates and clears the runtime with `sizeof(JSRuntime)`, then run `-runtime`.

If it reports that the functions of `frontFuncs` or `JSMallocBlockHeader` changed (`internal/fixlib/frontsites.go`), read them against `lib/frontcache.go`: `__js_malloc` must still size a block as `frontTake` does, `js_malloc` must still get its block from `js_malloc_rt`, a large block must still have `MFREE_NIL` in its header (`js_malloc_large` writes it), `__js_realloc` must still move a small block through `__js_malloc` and `__js_free`, and `js_def_malloc` and `js_def_realloc` must still fail only past the memory limit or when libc fails. Then run `-front`.

## js_malloc takes cached blocks itself

Release: v0.27.1.

An allocation the front cache could serve still went through four calls (`js_malloc`, `js_malloc_rt`, `__js_malloc` and the cache), about 78 instructions, and a free through two, about 43. Go inlines only small functions, and the cache was too large for it. Now the cache is two small functions, `frontTake` and `frontGive` in `lib/frontcache.go`, which Go inlines into `js_malloc`, `js_malloc_rt`, `__js_malloc` and `__js_free`. So `js_malloc` takes a cached block without a call, and `__js_free` keeps one with fewer checks. A size is turned into its list with a table of 64 entries, and there are 32 lists so that an index needs no bounds check. While the cache is off or gives its blocks back, every list counts as full, so `frontGive` tests no flag. The blocks the cache holds and when it gives them back do not change, and neither does the memory a VM holds.

On linux/amd64 (AMD EPYC), counted with callgrind over 4 renders after warmup, a render of the 6 workbench pages of tifa's shop takes 2.1% to 2.3% fewer instructions, and the pages of tifa's MUI test app 1.3% fewer. What the allocator takes of a render of the studio list goes from 16.0M instructions to 11.6M. Timed without PGO on one thread of an idle core pair, 6 rounds of main and the branch taking turns in 3 code layouts, the 6 shop pages render in 0.985 of the time at the median (0.971 to 0.992, below 1 in every round) and the 4 MUI pages in 0.983 (0.966 to 0.992). The memory of a VM after loading, after the renders and after a GC is the same to the KiB. Generated Go only, no C hunk (`internal/fixlib/frontcache.go`).

Tests: `lib/frontcache_test.go` (`TestFrontTake` for every size up to 600 bytes and through a context, `TestFrontCacheOff`) and `internal/fixlib/frontcache_test.go`.

### When upstream changes it

Same as [A front cache for small blocks](#a-front-cache-for-small-blocks): the fingerprints of `frontFuncs` cover `js_malloc` too.
