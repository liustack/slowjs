---
summary: 'The memory allocator of the C engine in engine/: its layers, what MemorySize and the memory limit count, how chunks and the footprint budget work, how memory is given back, the front cache, the pinned TLSF, and how to test it'
read_when:
  - Changing engine/heap.c, engine/heap.h, engine/tlsf.c or the allocator part of engine/quickjs.c
  - Asking what the memory limit, MemorySize or the GC threshold count in the C engine
  - Asking how much memory a VM takes from the host, and when it gives it back
  - Finding a memory bug in the C engine with the sanitizers
---

# The allocator of the C engine

`engine/` is the C tree of the engine to come. Its allocator is not wired into the Go engine yet: the Go engine in `lib/` keeps modernc.org/memory and its own front cache until the switch. The native test host (`engine/test/`) is the only user.

## Layers

```
js_malloc, js_free, js_realloc, js_malloc_usable_size (and the _rt forms)
  front cache: 32 lists of up to 256 freed small blocks       quickjs.c
  small blocks, 16 to 512 bytes: arenas of 4 KiB               quickjs.c, as upstream
  arenas and every block over 504 bytes: TLSF                  tlsf.c
  chunks, counts, limit, budget, resize, release               heap.c
host_memory_grow, host_call (memory release, remap)            the host
```

- The small block layer is upstream's: 31 size classes, an 8 byte header per block, and the reference count 4 bytes before the user data of every block (`JS_BLOCK_REF_COUNT_OFFSET` in `quickjs.h`, checked against `JSMallocBlockHeader`). Heap memory is 8 byte aligned.
- The front cache keeps freed small blocks on a list of their class, so that the next block of that class takes no arena. `JS_RunGC`, `JS_RunYoungGC`, `JS_ReleaseStart` and `JS_FreeRuntime` give it back to the arenas, and so does an allocation the heap fails: the cache goes back to the arenas and the heap is asked once more. A request refused before it reaches the heap (size 0, a size over the maximum) does not do that. `JS_SetFrontCache(rt, max)` gives the cache back and sets the length of a list (0 to 256, 0 for no cache).
- A large block is a TLSF block with an 8 byte header in front. TLSF grows it in place when the next block is free.
- All state is in the runtime. The runtime sits in the first chunk, so there is no global, no lock and no system call. The heap asks the host for `host_memory_grow(vm, size)`: `size` bytes, a multiple of 64 KiB, read and write, page aligned, not zeroed, or 0 when the system has none. For a dedicated chunk (see Chunks) it also uses two kinds of `host_call` (`engine/host.h`), which run no user code and do not call back into the engine: `HOST_CALL_MEMORY_RELEASE` (address, size) gives a chunk back, and `HOST_CALL_MEMORY_REMAP` (address, old size, new size) makes it bigger. Both pass the exact address and size of the chunk the host has.

## What is counted

- `count` (`JS_GetMemorySize`, `malloc_size` of `JS_ComputeMemoryUsage`) is the size of the runtime plus, for each block in use, its TLSF block size and the 8 byte size word before it. An arena counts whole, slots in the front cache included. Free memory and chunk headers do not count. `count` is what the memory limit and the GC threshold use.
- The memory limit (`JS_SetMemoryLimit`) fails an allocation when `count` plus the size asked for (the arena, or the user size plus the 8 byte header) passes it, then counts the real block. A resize is checked as upstream checks it: `count` plus the new size, less the usable size of the old block, against the limit (the old block does not count twice, and its 8 byte word stays in `count`, since the block is still a block). Once the resize is done, `count` changes by the difference of the two usable sizes. Lowering the limit frees nothing. A failure makes the engine throw out of memory.
- The footprint is what the VM took from the host, the sum of its chunks. `JS_SetFootprintLimit` bounds it, 0 for none. It only stops growth: a VM that holds more keeps it, and nothing is taken back by it. It covers the chunks only, not the shadow stack, shared bytecode or the Go side. There is no default.
- TLSF rounds a block up by at most 1/32, where the Go engine's allocator (modernc.org/memory) rounds to a power of two, so `count` is about 13% to 15% lower than the Go engine's `MemorySize` for the same heap. Against glibc's malloc in the C tree it is the same within 1.3%: after 3000 renders of tifa's pages the shop holds 39.84 MB against 39.85, the MUI test app 29.91 against 29.52, the bench app 2.15 against 2.12.

## Chunks

- The first chunk is 64 KiB and holds the runtime, the TLSF control and the first pool. Later chunks are a quarter of the footprint, 64 KiB to 8 MiB, a multiple of 64 KiB. Near the budget a shared chunk shrinks to what the request needs (with the room TLSF needs to find it, `need + need/32 + 112 + 64`, rounded up to 64 KiB), and is refused when even that does not fit.
- A request of more than half of the next chunk (so over 4 MiB for good) gets a dedicated chunk. A dedicated chunk is flagged in its header (`kind`), holds one block, and is not under TLSF: the 32 byte header, the size word of the block, the block (rounded up to 8 bytes), an end marker, and room up to the next 64 KiB, which the block can grow into and nobody else gets. The block counts by its size, not by the room. `heap_check` and `heap_contains` know both kinds.
- When the block of a dedicated chunk is freed, or `heap_realloc` moves it, the chunk goes back to the host in that call (`HOST_CALL_MEMORY_RELEASE`). The heap takes the chunk out of its lists and counts first, then asks, and never reads the chunk again. There is no cache of freed dedicated chunks. A resize gets the new block and copies before it gives the old chunk back.
- A block that fits its chunk and uses at least half of it stays in it. When it outgrows the chunk and the host says it can remap (`JS_SetHostRemap(rt, 1)`, right after `JS_NewRuntime`, linux), the heap checks the budget for the growth alone and asks the host to remap the chunk (`HOST_CALL_MEMORY_REMAP`, `mremap` with `MREMAP_MAYMOVE`), which keeps the content and may change the address. The host refusing is out of memory: the resize fails and nothing changes, there is no copy. Without remap, or when the block shrinks under half of its chunk, the heap takes a new block, copies and gives the old chunk back. A block that is growing gets an eighth of its size as room in its new chunk, so a buffer that grows a little at a time does not get a new chunk, a copy or a system call at each step (the budget may take that room away).
- A resize that cannot grow in place or move inside the chunks needs a new chunk while the old block still sits in its chunk, so the budget can refuse a resize that the count allows. A refused resize leaves the old block, its address, its content and its usable size as they were, and the heap's `count` too. At the runtime, a failed request first gives the front cache back, which can empty arenas and lower `count`, so only the heap alone promises the count of a refused call.
- The largest request is 2^32 - 8 bytes: bigger ones fail before any arithmetic on the size.

## Giving memory back

Dedicated chunks are given back at once (see Chunks). Shared chunks are not: nothing of them is given back unless the host asks. `JS_ReleaseStart(rt, keep)` plans to offer all the free memory but `keep` bytes. `JS_ReleaseNextChunk` then returns whole free chunks (not the first), largest first, taken out of the heap and out of the footprint, until its length is 0. `JS_ReleaseNextRange` returns the inside of free blocks of 4 KiB or more, which the host may drop the pages of: TLSF never reads it, it only writes there when it splits or merges a block. The host does not use the runtime in between, unmaps the chunks and rounds each range inward to its page size. A range may be offered again by a later release. `release_candidate` of `JS_ComputeMemoryUsage` totals what was offered. `keep` is a target and not an exact amount: chunks are counted by their whole size, ranges by what is left of a free block without its TLSF words, and a range is at least 4 KiB, so one big free block can take the total past the target.

A buffer that grows by realloc past half a chunk used to leave a chain of empty chunks behind it, mapped until a release: a string of 1 GiB built in 1 MiB steps (`staging/sm/String/replace-math.js`) took 6.7 GB. With dedicated chunks it takes 1.05 GB in 1.2 seconds with remap (the old C tree on glibc: 1.09 GB, 1.2 seconds) and 2.0 GB in 4.5 seconds without it, where the old chunk waits for the copy to end.

## TLSF

`engine/tlsf.c` and `engine/tlsf.h` are Matthew Conte's TLSF 3.1 (BSD-3-Clause, see `NOTICE.md`) at commit `deff9ab509341f264addbd3c8ada533678591905` (`tlsf.c` SHA-256 `2a0f8cfc9cfe6114ccdc6cf22339059440b16f1149b5107bead4ae4c3a0d50e2`, `tlsf.h` `f7f73c48810ba60203095667c226e5a600a6ea0f69afba48efff6efbaa628d4f`, both with CRLF line ends), converted to LF, and changed in these ways:

- A request or pool TLSF cannot hold fails (issues 9, 22 and 35 of the project). `engine/test/tlsf-edges.sh` runs the edge cases on the pinned file, where four fail or crash, and on `engine/tlsf.c`, where all pass.
- No `stdio.h`, `stdlib.h`, debug self test or `tlsf_memalign`.
- Four functions for the heap: `tlsf_realloc_inplace` (resize without moving), `tlsf_next_free` (walk the free blocks, largest class first), `tlsf_free_inside` (the part of a free block TLSF never reads) and `tlsf_pool_is_free`.

## Testing

All of it runs on the native test host (see [testing.md](testing.md#the-c-tree-and-its-native-test-host)).

```sh
engine/test/tlsf-edges.sh                    # TLSF edge cases, pinned file and engine/tlsf.c
engine/test/build.sh plain                   # also asan-heap, ubsan
engine/test/heap.sh -m plain                 # scenarios and random sequences, heap and runtime
engine/test/build.sh fuzz && engine/test/heap.sh -m plain -f 300   # the libFuzzer target
engine/test/build.sh asan                    # every block from the system malloc, with LeakSanitizer
engine/test/build.sh asan-heap               # the real allocator, free memory poisoned
engine/test/heap.sh -m asan-heap             # also poison-test: what the poison covers
engine/test/host-test.sh tsan                # the test host, make and free runtimes on two threads
```

- `heap-test.c` has a fake host that maps chunks, can refuse the nth request, and fills every range given back with 0xdd. Every block holds a pattern, checked when it is freed or resized. Every few thousand steps `heap_check` and `JS_CheckMemory` (compiled in with `JS_HEAP_CHECK`) walk TLSF, the arenas and the front cache, and the test compares the counts with the sum over the blocks it holds.
- In `asan-heap` the engine poisons the user bytes of free small slots, free TLSF payloads, the rest of an arena and the cached blocks (all but the next pointer), so a use after free inside our own memory is found. `heap.c` and `tlsf.c` are built without AddressSanitizer there, since the poison is theirs to put and take. `poison-test.c` (run by `heap.sh -m asan-heap`) asks AddressSanitizer which bytes are poisoned. What the poison does not cover: the 8 byte header of the slot after a live small block stays readable, since the allocator reads and writes headers, so an overrun of up to 8 bytes there is not found. Neither is an overrun into another live block, nor the next pointer of a cached block (for a block of 8 bytes, its whole user area). `asan` does find those, with red zones around every block, but it has no arenas and no front cache. Run both.
- `host-test.c` tests the host alone against stubs of the engine: a runtime that does not start leaves no VM and no chunk, and two threads that make and free runtimes do not race (`engine/test/host-test.sh tsan` builds it with ThreadSanitizer).
- Dedicated chunks: `heap-test` has a scenario for them (made for one block, given back in the call that frees it, resized in the chunk and out of it, refused by the host, the budget and the limit with the old chunk intact) and one for remap (the host refusing is out of memory and nothing is copied, the budget is checked for the growth alone, a buffer that grows by 64 KiB gets few chunks). Its random runs give dedicated chunks back and remap them (odd seeds, where the fake host remaps), and fail when they do not. `api-test` does the same on the real native host: the chunks it maps are counted right after a malloc, a resize and a free.
- A runtime must end with `JS_FreeRuntime` returning 0. It returns the number of blocks of the heap still in use, an arena counting as one block however many of its slots are in use, so it is not a count of leaked objects. The test host exits with status 87 otherwise.
- `engine/test/render.c` renders tifa's pages on the C tree and, with `soak=N`, prints the count, blocks and footprint every 100 renders.
