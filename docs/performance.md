---
summary: 'How much faster each release of the fork renders pages, how much faster the fork is than the QuickJS it started from, how to measure an engine change in tifa, which engine ideas were tried and dropped and why, and where to continue'
read_when:
  - Making a change to the engine for speed, or reviewing one
  - Measuring a branch of the fork on tifa's pages before a release
  - About to try an engine idea that may already have been tried
  - Planning the next round of work on render speed
  - Quoting how fast a release is
---

# Performance

The fork is made for tifa, which renders React pages to HTML in the engine, so render speed is measured on the pages of tifa's example shop: one warm VM rendering the order list, an order and the stats of each of its two shops (`BenchmarkShop` in tifa's `test/ssr`). Every change for speed leaves the HTML the same byte for byte. The doc of each change in [fork-changes](fork-changes/README.md) has its full measurement, and tifa's [docs/performance.md](https://github.com/liustack/tifa/blob/main/docs/performance.md) has what the numbers mean for an app. [benchmarks.md](benchmarks.md) compares the engine with other JavaScript engines for Go.

## How the numbers were measured

Unless a row says otherwise:

- Machine: a Google Cloud VM with 8 vCPUs of an AMD EPYC 7B13 (Zen 3), Linux, one core pinned with its hyperthread sibling idle. Some rows are on an Apple M4 (darwin/arm64), which is noisier.
- The build before and the build after a change, alternated round after round in one session. Only ratios from the same rounds compare.
- With PGO means tifa built with an engine profile collected on that same engine. A profile collected on an older engine matches fewer call sites (see [Measuring in tifa](#measuring-in-tifa)).

## What each release gained

| Release | Change | Effect on a render of the shop |
| --- | --- | --- |
| v0.26.1 | [Error stacks formatted when first read](fork-changes/errors-and-stacks.md#error-stacks-formatted-when-first-read) | With the development build of React, a 100-item component took 11.2 ms at the median instead of 42.1 ms (darwin/arm64). Production builds stayed within 1% |
| v0.26.12 | [Calls allocate no Go memory](fork-changes/calls-and-tls-frames.md#calls-allocate-no-go-memory) | About 11% less time on linux/amd64 and 14% less on darwin/arm64. Loading the bytecode into a VM about 11% less on linux. It also took libc's allocator lock out of every call, which held several VMs back |
| v0.26.13 | [Less stack work in the interpreter loop](fork-changes/interpreter-loop.md#less-stack-work-in-the-interpreter-loop) | About 12% less on linux/amd64 and 17% less on darwin/arm64. The Go stack frame of `JS_CallInternal` went from 2608 to 896 bytes on amd64 and from 7744 to 944 on arm64 |
| v0.26.13 | [New properties of plain objects skip the generic path](fork-changes/objects-and-shapes.md#new-properties-of-plain-objects-skip-the-generic-path) | About 6% less. A property of an object literal takes about 55 ns rather than 95 |
| v0.26.15 | [Frames freed without defer](fork-changes/calls-and-tls-frames.md#frames-freed-without-defer) | About 10% less on linux/amd64, with or without PGO |
| v0.26.15 | [Skip to where a regexp can match](fork-changes/regexp.md#skip-to-where-a-regexp-can-match) | About 3% less |
| v0.26.15 | [Cold paths of === and key names](fork-changes/calls-and-tls-frames.md#cold-paths-of--and-key-names) | 0.7% less with PGO, 2% without |
| v0.26.15 | [One block of TLS stack per VM outside linux](fork-changes/memory-and-allocation.md#one-block-of-tls-stack-per-vm-outside-linux) | 10 VMs on an Apple M4 render 1.4 to 1.55 times as many pages a second, and wait on a lock about 0.5% of their time instead of 3.8% to 5.3%. One VM and linux do not change |
| v0.26.16 | [TLS frames taken per statement](fork-changes/calls-and-tls-frames.md#tls-frames-taken-per-statement) | 1.6% less with PGO and 1.1% without on linux/amd64, 2.6% less on darwin/arm64 with PGO |
| v0.26.16 | [set_value written out in the interpreter](fork-changes/interpreter-loop.md#set_value-written-out-in-the-interpreter) | 1.4% less, with and without PGO. With the frames per statement, 2.8% less than v0.26.15 with PGO and 2.9% less without |
| v0.26.16 | [The interrupt countdown inside the interpreter](fork-changes/interpreter-loop.md#the-interrupt-countdown-inside-the-interpreter) | 0.6% less with PGO, about the same without |
| v0.26.16 | [A front cache for small blocks](fork-changes/memory-and-allocation.md#a-front-cache-for-small-blocks) | 0.9% less with PGO alone, about the same without |
| v0.26.16 | [Kept shapes](fork-changes/objects-and-shapes.md#kept-shapes) | 3.6% less alone, without PGO. With the front cache and the interrupt countdown, 4.5% less than without the three. Two VMs on two hyperthreads of one core render 2.9% more pages a second |
| Unreleased | [for in skips prototypes with no enumerable property](fork-changes/objects-and-shapes.md#for-in-skips-prototypes-with-no-enumerable-property) | The shop barely changes (0.996). The spiral and table pages of tifa's bench app, which run many for in loops, 0.983 and 0.971 of the time, without PGO |
| Unreleased | [String keys that are already atoms](fork-changes/objects-and-shapes.md#string-keys-that-are-already-atoms) | 1.3% less without PGO, measured before the for in, bitwise and comparison changes. Reading with a string key takes 13% less time in a micro benchmark, writing 30% less |
| Unreleased | [JSON copies runs of plain characters](fork-changes/json.md#json-copies-runs-of-plain-characters) | 0.2% to 1.7% less, inside the spread of a round: JSON is about 1% of a render. Quoting the data of a route as one string takes 41% less time, `JSON.parse` of the render data 9.5% less |
| Unreleased | [Bitwise operators on floats skip the slow path](fork-changes/operators.md#bitwise-operators-on-floats-skip-the-slow-path) | The shop has nothing to gain. The pages of tifa's MUI app take 0.952 to 0.960 of the time, without PGO |
| Unreleased | [The usual comparisons answered in the interpreter](fork-changes/operators.md#the-usual-comparisons-answered-in-the-interpreter) | 0.982 of the time without PGO. The bench pages 0.940 and 0.942, the MUI pages 0.982 to 0.985 |
| Unreleased | [One TLS frame per call on linux](fork-changes/calls-and-tls-frames.md#one-tls-frame-per-call-on-linux), with 22 more functions without a TLS frame and 12 more with frames per statement | About a fifth fewer TLS frames per render. 1.4% less than v0.26.16 without PGO, and 2.2% and 3.1% less on tifa's bench and mui test apps |

Whole releases, measured in tifa:

- v0.26.13 renders in 0.823 of the time of v0.26.12 (0.821 to 0.825 over 8 rounds).
- v0.26.15 renders in 0.859 of the time of v0.26.14 with a profile collected again for each (0.857 to 0.861 over 6 rounds), and in 0.843 without PGO. Whole requests with 6 VMs on 3 physical cores serve 1.17 times as many. On the Apple M4, without PGO, one VM renders 108 pages a second instead of 84 to 86, and 10 VMs 532 to 549 instead of 288 to 305, 1.80 to 1.88 times as many.
- v0.26.16 has not been measured as a whole release in tifa yet. Its changes were measured one by one in the rows above.

What it costs in memory: v0.26.16 makes one VM of the shop hold 722 KiB more after a GC (8.03 MiB rather than 7.33), 647 KiB of it the kept shapes. Outside linux each VM keeps up to 1 MiB of TLS stack since v0.26.15.

### Against the QuickJS the fork started from

Measured in tifa on 2026-10-20 against `modernc.org/quickjs` v0.25.0 with libquickjs v0.13.1, on the same SSR bundle and host program (tifa's [docs/studies/moejs.md](https://github.com/liustack/tifa/blob/main/docs/studies/moejs.md)): the engine changes up to v0.26.15 alone make renders 1.63 to 1.66 times as fast. With the rewrites of tifa's Vite plugin and PGO, pages render 2.07 times as fast as on the original engine with neither.

## Measuring in tifa

To measure a branch of the fork before it is released, point tifa at the checkout with a `replace`, build the example app, and run the benchmarks of tifa's [docs/testing.md](https://github.com/liustack/tifa/blob/main/docs/testing.md#performance-comparisons):

```sh
# in tifa
go mod edit -replace github.com/liustack/quickjs=../quickjs
(cd examples/shop && pnpm install --frozen-lockfile && go tool tifa build)
TIFA_SSR_SHOP=1 go test -run '^$' -bench 'Shop/^render' -benchtime 2000x ./test/ssr
TIFA_SSR_SHOP=1 go test -run ShopVMMemory -v ./test/ssr
TIFA_SSR_SHOP=1 go test -run '^$' -bench ShopParallel -benchtime 15s ./test/ssr
```

- Build the test binary once for the old engine and once for the new one (`go test -c -o old.test`, `-o new.test`), and run them in turn on the same pinned core, round after round. Read the ratio of the sums of the six P50 within each round.
- `go test` builds without PGO unless given `-pgo=<file>`, so these compare the engines without PGO. For numbers with PGO, collect a profile on each engine as tifa's docs/testing.md says under "The engine profile", and build each with its own profile.
- Compare the HTML of every page between the two builds. The router writes the time of the render into each match (`u:` followed by 13 digits), so mask that first.
- A benchmark with one VM cannot see contention between VMs. For a change near memory allocation or the TLS stack, also run `BenchmarkShopParallel` and read its `speedup` and `lock-wait-%`.
- Note the load of the machine during each round, and run again when other work used more than about 10% of the CPU.
- Remove the `replace` before committing in tifa: `TestEngineProfileMatchesTheDependencies` fails for a profile made with a `replace`.

After a release, upgrade tifa and collect its engine profile again (see [releasing.md](releasing.md#after-the-release)).

## What was tried and not done

Each item says what it would give and why it was dropped. Check here before trying one of them again. The numbers come from tifa's [docs/performance.md](https://github.com/liustack/tifa/blob/main/docs/performance.md#what-was-tried-and-not-done) and [docs/studies/engine-hotspots.md](https://github.com/liustack/tifa/blob/main/docs/studies/engine-hotspots.md), and from the docs of the changes.

- **Removing the TLS frame of 135 more small functions** (a TLS frame is the part of libc's TLS stack where the Go translation keeps a function's C locals whose address is taken): about 0.24% of render time, too small to measure, for 135 more entries to maintain.
- **Letting more than 768 bytes of a call's room live in its TLS frame**: 1024 bytes covers 0.02% more calls, but doubles the blocks Linux keeps per call depth from 2 KB to 4 KB.
- **Keeping only 256 KiB of TLS stack per VM outside linux**: not enough. 10 VMs on the Apple M4 rendered 315 pages a second with 4.2% lock wait, against 437.8 to 481.6 with 1 MiB, since renders reach deeper into the TLS stack.
- **Moving rarely used opcodes out of the interpreter loop**: at most 8% alone on amd64, an upper bound measured by replacing the 56 handlers of opcodes the shop never runs with empty stubs, but only 1.4% on top of the rewrite that keeps 15 values in their stack slots, and 0.5% on arm64. It would need the loop's locals moved into a struct.
- **Dispatching opcodes through closures or a table of functions**, as some Go interpreters do: a rewrite of the interpreter, against the fork's rule of making only mechanical changes. Not tried.
- **A fast path for `===` in the interpreter loop**: 1.0% to 1.3% slower, measured with v0.26.14 and PGO. The interpreter function is so large that Go inlines only the smallest functions into it, so a new helper there is one more real call.
- **The interrupt countdown in a small function of its own**: no change (0.998 to 1.001 of the time) with v0.26.14, for the same reason. v0.26.16 writes the countdown inline instead, through `pollDue`, which Go does inline, for 0.6% with PGO.
- **Writing out `JS_FreeAtom` and `is_strict_mode` in the interpreter**, as v0.26.16 does for `set_value`: nothing that could be told from noise, so they stay calls.
- **Writing the bit fields of a new object in 2 stores rather than 9**: no change (0.991 to 1.010). The time is the cache miss of the first write to the new object, and fewer stores only move it.
- **A fast path for object spread** (`JS_CopyDataProperties`): one spread took 240 ns rather than 415 in a micro benchmark on the Apple M4, but the shop did not change (0.997 to 1.005).
- **Keeping the in-between shapes of every object literal**, before v0.26.16: about 2% more on top of the new property fast path, but the two list pages ran out of VM memory, because the kept shapes held their prototypes and everything those reach. v0.26.16 keeps only shapes whose prototype is `Object.prototype`, at most 32 keys and no unregistered symbol key, in a ring of 2048 (see [Kept shapes](fork-changes/objects-and-shapes.md#kept-shapes)).
- **Building a shape in place when no other object has its keys**: in the first form of the shape cache, copying every shape made objects of keys no other object has 22% slower to build. v0.26.16 earns one copy for each shape found, up to 64, which leaves those objects about 6% slower.
- **Literal templates in the bytecode**, as V8 copies a prebuilt object: needs compiler and bytecode format changes, and quickjs-ng saw no clear difference when it tried.
- **Allocating the property array of a literal at its final size**: the opcode that makes the object does not know the count, and growing arrays differently costs memory for every small object. It pays only together with kept shapes.
- **Inline caches for property reads**: quickjs-ng added and then removed them. Across 17 real tools they ranged from 11% slower to 54% faster, 4% faster on average, and used more memory. Not tried.
- **One libc allocator per VM**, to avoid its global lock: with 8 VMs on Linux the lock costs 0.4% to 0.9% of the CPU, and since v0.26.15 10 VMs on an Apple M4 wait on locks for about 0.7% of their time.
- **Waiting for Go 1.28**: a development build made the unchanged engine 5% faster on amd64 and 2% on arm64, but the rewritten engine 5% to 6.5% slower on amd64, for a reason not found. The rewrites still win there by 4.2% on amd64 and 15.5% on arm64.
- **Dropping function source from the bytecode** (`JS_SetStripInfo`) to save VM memory: seroval, which tifa's pages use, reads the source of its own helpers with `toString` and writes it into pages, so the browser would break.

## Where to continue

With v0.26.15 the interpreter function alone took about 36% of a render. In rough order of what it may give:

1. **Property access.** Property reads took about 16% of a render with v0.26.15, `find_own_property` alone 5.5%, mostly waiting on cache misses. Assignment (`o.x = v`, `Object.assign`) still takes the slow path, about 2%, and a fast path for it needs prototype setters and read-only properties checked: estimated at 0.3% to 0.5%.
2. **Making and freeing objects.** The engine's malloc and free, making and freeing objects and reference counts took about 13% of a render. v0.26.16's front cache and kept shapes go after part of it.
3. **TLS frames.** About 1.5% of a render is frames the stack check counts, in `JS_CallInternal`, `js_call_c_function` and the like, which must stay. v0.26.16 takes the frames of 8 more functions per statement, and the changes after it (not released yet) take out about a fifth of the frames of a render. tifa's engine study puts what is left to win there at most 1% to 1.2%, with a general transform in fixlib for every function whose frame address stays inside it.
4. **Regular expressions.** Testing the first character of each alternative before trying it would make react-spring's test of 148 color names cheaper: about 2% of the order list.
5. **Spreading a plain array without the iterator**: about 1.2% of the order list (TanStack Table calls through `fn(this, ...args)`).
6. **Less memory per VM.** All VMs could share one read-only copy of the function source and line tables from the bytecode, so `toString` still works. Estimated at about 380 KB per VM. Needs changes to the C patch, to fixlib and to the Go API.
7. **Kept shapes as a C change.** When the Go code of the engine can be generated again from C, kept shapes with a reference from child to parent shape might keep more shapes for less memory than the ring of v0.26.16. A candidate for upstream.
8. **Go 1.28 on amd64.** When it ships, measure the rewritten engine again. If it is still slower than with Go 1.27, make a small reproduction for the Go team.
9. **arm64.** After the stack rewrites, storing the bytecode position took up to about 11.6% of the interpreter's own time on arm64, for a reason not found. It is an upper bound: the store sits on the chain that reads the next opcode, so the sampling may count time spent waiting there against it.
