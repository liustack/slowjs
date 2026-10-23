---
summary: 'How much faster each release of the fork renders pages, how much faster the fork is than the QuickJS it started from, how to measure an engine change in tifa, what the native C patch costs, which engine ideas were tried and dropped and why, and where to continue'
read_when:
  - Making a change to the engine for speed, or reviewing one
  - Measuring a branch of the fork on tifa's pages before a release
  - About to try an engine idea that may already have been tried
  - Planning the next round of work on render speed
  - Quoting how fast a release is
---

# Performance

The fork is made for tifa, which renders React pages to HTML in the engine, so render speed is measured on the pages of tifa's example shop: one warm VM rendering the order list, an order and the stats of each of its two shops (`BenchmarkShop` in tifa's `test/ssr`). Every change for speed leaves the HTML the same byte for byte. The doc of each change in [fork-changes](fork-changes/README.md) has its full measurement, and tifa's [docs/performance.md](https://github.com/liustack/tifa/blob/main/docs/performance.md) has what the numbers mean for an app. How the engine compares with the other pure Go engines, on tifa's pages and on general workloads, is in [benchmarks.md](benchmarks.md).

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
| v0.26.17 | [for in skips prototypes with no enumerable property](fork-changes/objects-and-shapes.md#for-in-skips-prototypes-with-no-enumerable-property) | The shop barely changes (0.996). The spiral and table pages of tifa's bench app, which run many for in loops, 0.983 and 0.971 of the time, without PGO |
| v0.26.17 | [String keys that are already atoms](fork-changes/objects-and-shapes.md#string-keys-that-are-already-atoms) | 1.3% less without PGO, measured before the for in, bitwise and comparison changes. Reading with a string key takes 13% less time in a micro benchmark, writing 30% less |
| v0.26.17 | [JSON copies runs of plain characters](fork-changes/json.md#json-copies-runs-of-plain-characters) | 0.2% to 1.7% less, inside the spread of a round: JSON is about 1% of a render. Quoting the data of a route as one string takes 41% less time, `JSON.parse` of the render data 9.5% less |
| v0.26.17 | [Bitwise operators on floats skip the slow path](fork-changes/operators.md#bitwise-operators-on-floats-skip-the-slow-path) | The shop has nothing to gain. The pages of tifa's MUI app take 0.952 to 0.960 of the time, without PGO |
| v0.26.17 | [The usual comparisons answered in the interpreter](fork-changes/operators.md#the-usual-comparisons-answered-in-the-interpreter) | 0.982 of the time without PGO. The bench pages 0.940 and 0.942, the MUI pages 0.982 to 0.985 |
| v0.26.17 | [One TLS frame per call on linux](fork-changes/calls-and-tls-frames.md#one-tls-frame-per-call-on-linux), with 22 more functions without a TLS frame and 12 more with frames per statement | About a fifth fewer TLS frames per render. 1.4% less than v0.26.16 without PGO, and 2.2% and 3.1% less on tifa's bench and mui test apps |
| v0.26.17 | [Alternatives that cannot match are not pushed](fork-changes/regexp.md#alternatives-that-cannot-match-are-not-pushed) | About 1% to 2.5% less without PGO, and 3% to 5% less on the MUI pages of tifa's test apps. A `replace` with react-spring's regexp of 148 color names takes 5.9 µs rather than 44.5 |
| v0.26.17 | [New functions get length and name at once](fork-changes/objects-and-shapes.md#new-functions-get-length-and-name-at-once) | 0.982 of the time without PGO, on top of the regexp change. 3,000 new functions make 2,000 shapes rather than 5,001 |
| v0.26.17 | [Intl calls Go through Builtins](fork-changes/time-and-intl.md#intl-calls-go-through-builtins) | Not measured on a render. `new Intl.NumberFormat('en')` takes 2.7 µs rather than 16.5, a `DateTimeFormat` with a time zone 8.1 µs to make rather than 32.0, and a VM that has used `Intl` holds about 15 KB less |
| v0.27.0 | [Compiled regexps kept per VM](fork-changes/regexp.md#compiled-regexps-kept-per-vm) | The journal post of tifa's example shop that highlights CSS, which compiles 392 regexps per render, renders in 0.893 of the time without PGO (0.841 to 0.903 over 6 rounds in 3 code layouts). The other pages render in the same time. A VM of the shop holds 12 KiB more |
| v0.27.0 | [The engine reads Go's clocks](fork-changes/time-and-intl.md#the-engine-reads-gos-clocks) | The shop does not change (0.994 to 1.009 over 6 rounds, median 1.000), without PGO. `Date.now()` takes 122 ns rather than 689, `new Date()` 261 rather than 870 and `performance.now()` 107 rather than 686 |
| v0.27.0 | [Likely and unlikely tests branch on their condition](fork-changes/interpreter-loop.md#likely-and-unlikely-tests-branch-on-their-condition) | 0.983 of the time without PGO (0.976 to 0.984), the bench pages of tifa 0.976 to 0.987. 1.6% fewer instructions |
| v0.27.0 | [TLS frames taken without a call on linux](fork-changes/calls-and-tls-frames.md#tls-frames-taken-without-a-call-on-linux) | 0.998 of the time on top of the row above (0.995 to 1.004), the bench pages 0.990. 1.9% fewer instructions. With the row above, 0.979 of the time of v0.26.17 and the bench pages 0.967 |
| v0.27.0 | [Compiled Intl formatting plans](fork-changes/time-and-intl.md#compiled-intl-formatting-plans) | NumberFormat takes 41% to 42% less time than main and DateTimeFormat 28% to 37% less on the fixed inputs. [Ten shop pages](#intl-formatting) render in 0.9998 to 1.0081 of main's time without PGO |
| v0.27.1 | [Methods of strings, numbers and arrays found in the interpreter](fork-changes/objects-and-shapes.md#methods-of-strings-numbers-and-arrays-found-in-the-interpreter) | 0.996 of the time without PGO (0.992 to 0.998 over 6 rounds in 3 code layouts), and 0.975 (0.966 to 0.986) on the MUI pages of tifa's test app, whose order list reads string methods about 320000 times a render. 1.0% fewer instructions on the MUI pages, under 0.1% on the shop |
| v0.27.1 | [js_malloc takes cached blocks itself](fork-changes/memory-and-allocation.md#js_malloc-takes-cached-blocks-itself) | 0.985 of the time without PGO (0.971 to 0.992 over 6 rounds in 3 code layouts), the MUI pages of tifa's test apps 0.983. 2.1% to 2.3% fewer instructions on the shop, 1.3% on the MUI pages. Memory the same |
| v0.27.2 | [WeakMap entries freed in one full GC](fork-changes/weak-collections.md#what-it-costs) | A fix, not a speedup. Render time does not change. Without PGO, a full GC after each render takes 0.989 of the time on the shop and 1.046 on the MUI pages (1.0% and 0.3% more instructions). Large weak maps collect slower: 2.9 times as long for a chain of 100,000 live entries |
| v0.27.2 | [Strings of Builtins converted in Go](fork-changes/go-api.md#strings-of-builtins-converted-in-go) | The shop does not change that can be told: 1.004 of the time without PGO (0.997 to 1.018 over 6 rounds in 3 code layouts), since its Builtins take few strings. A Builtin that reads a 16-character string and returns a new one takes 134 ns rather than 230, one that returns its argument 81 ns rather than 97, against 60 ns for a call of a javascript function. With [Args.Latin1 and Args.UTF16](fork-changes/go-api.md#argslatin1-argsutf16-newstringlatin1-and-newstringutf16) React's escape of a 16-character string takes 99 ns in Go against 200 ns in javascript |
| Unreleased | [Searches skip to where the string is](fork-changes/strings.md#searches-skip-to-where-the-string-is) | 0.995 of the time without PGO (0.975 to 1.032 over 6 rounds in 6 code layouts), the MUI pages of tifa's test app 1.003, both inside the spread of the layouts. `String.prototype.includes` went from 1.06% of a shop profile to 0.27%, and a search through 100 KB of HTML takes 81 µs rather than 320 |
| Unreleased | [Young collection, VM.RunYoungGC](fork-changes/young-collection.md) | With tifa running a young collection after each render and a full one now and then, the P99 of a render of the shop is 0.748 of what it was with one VM and the pages in random order, and 0.796 with two VMs. The MUI pages of tifa's test app 0.940 and 0.919. The P50 and CPU stay within 2.1%, and peak RSS is 0.82 to 0.88 of what it was, without PGO |

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

## Intl formatting

v0.27.0, against main `641e996`, with tifa shop `fb31f616`. The shared go-intl formatters use [compiled plans and direct string output](fork-changes/time-and-intl.md#compiled-intl-formatting-plans). The fixed-input NumberFormat cases take 41% to 42% less time than main and DateTimeFormat 28% to 37% less. The [full call table](fork-changes/time-and-intl.md#focused-timing-and-memory-costs) includes construction, uncached resolution, allocation and memory costs.

The ten shop pages were measured on an AMD EPYC 7B13, Linux, Go 1.27.2, `CGO_ENABLED=0`, with `-pgo=off`. Both builds use the same SSR bundle and captured navigation inputs. The driver follows BenchmarkShop: one VM, 256 warm renders, then 200 measured renders per page in each of six alternating main and branch rounds. It includes the six workbench pages plus the storefront list and product page in both languages. HTTP and data fetching are outside the timer. HTML is equal after masking only the router's `u:` timestamp.

Each engine runs at nice 19 on CPU 1, with sibling 5 idle, in a 4 GiB systemd scope with no swap. A run starts only after both CPUs stay below 1% busy for three seconds. Samples are rejected and repeated if sibling 5 averages at least 1% busy or reaches 5% in a one-second sample, or another application runs on CPU 1. Rounds 1 to 5 run all ten subbenchmarks in one process per build. Round 6 invokes each page separately and alternates the two builds for that page, so activity outside that page cannot discard its sample. Each subbenchmark still creates its own VM pool and uses the same warmup and sample counts. The table uses six valid samples per build for every page. Values are the median of each round's P50, with the minimum and maximum. Ratios below 1 favor the branch. The last column pairs the builds within each round.

| Page | Main P50, ms | Branch P50, ms | Branch / main | Paired ratio median and range |
| --- | ---: | ---: | ---: | ---: |
| `/projects/studio` | 29.165 (28.480 to 29.400) | 29.400 (28.500 to 29.980) | 1.0081 | 1.008 (0.981 to 1.021) |
| `/projects/weekend` | 28.125 (27.770 to 28.330) | 28.120 (27.880 to 28.940) | 0.9998 | 1.006 (0.989 to 1.022) |
| `/projects/studio/orders/SH-1042` | 10.455 (10.360 to 10.570) | 10.515 (10.440 to 10.560) | 1.0057 | 1.002 (0.994 to 1.017) |
| `/projects/weekend/orders/WK-0201` | 10.210 (10.150 to 10.240) | 10.270 (10.200 to 10.590) | 1.0059 | 1.009 (0.998 to 1.036) |
| `/projects/studio/stats` | 8.477 (8.407 to 8.528) | 8.492 (8.455 to 8.525) | 1.0017 | 1.003 (0.991 to 1.012) |
| `/projects/weekend/stats` | 8.465 (8.398 to 8.499) | 8.524 (8.406 to 8.635) | 1.0069 | 1.009 (0.989 to 1.025) |
| `/store/en-US/products` | 69.395 (68.710 to 70.150) | 69.450 (69.020 to 70.080) | 1.0008 | 1.002 (0.993 to 1.009) |
| `/store/en-US/products/pour-over-kettle` | 41.040 (40.350 to 41.530) | 41.030 (40.650 to 41.950) | 0.9998 | 0.999 (0.981 to 1.031) |
| `/store/zh-CN/products` | 69.315 (68.720 to 70.230) | 69.815 (69.680 to 70.020) | 1.0072 | 1.008 (0.994 to 1.016) |
| `/store/zh-CN/products/pour-over-kettle` | 41.155 (40.670 to 41.690) | 41.475 (41.080 to 41.570) | 1.0078 | 1.005 (0.997 to 1.016) |

The page ratios range from 0.9998 to 1.0081. The call-level gain does not establish a whole-page speedup. Page differences should be read with the paired sample spread, not rounded into a claim that every page is faster.

## Native C patch cost

Measured on 2026-10-22, using the pinned Bellard source `3d5e064e9dd67c70f7962836505a7fa067bf0a4e` and the fork at `620658c`. The old C patch unconditionally set `DIRECT_DISPATCH` to zero. That translation setting predates the fork's exception changes. Native C therefore used a central switch instead of upstream's computed goto dispatch.

The machine was the AMD EPYC 7B13 VM described above. Both compilers used the upstream Makefile at `-O2`, gcc 15.3 and clang 22.1.8. The V8-v7 harness was `tests/bench-v8/combined.js` from Bellard's 2026-06-04 extras, retaining strict mode, as in [goc's comparison](https://github.com/LoyieKing/goc/blob/8482979acbc8f10f76d9715d88c1beed55df230b/docs/benchmark/README.md). Native invocations passed `--stack-size 16M` in every variant.

Eight alternating rounds gave the following results. Scores are medians. Ratios are the median and range of the eight paired ratios, not the quotient of the displayed score medians. Higher scores are faster.

| C source | gcc score | gcc / unpatched | clang score | clang / unpatched |
| --- | ---: | ---: | ---: | ---: |
| Unpatched | 1313.0 | 1.0000 | 1295.5 | 1.0000 |
| Old complete patch | 1172.5 | 0.8886 (0.8591 to 0.9077) | 1161.5 | 0.8941 (0.8423 to 0.9072) |
| Only disable computed goto | 1204.5 | 0.9102 (0.9018 to 0.9307) | 1136.0 | 0.8780 (0.8609 to 0.8983) |
| Complete patch with native computed goto | 1312.0 | 0.9932 (0.9871 to 1.0162) | 1303.5 | 1.0046 (0.9924 to 1.0171) |

Keeping native computed goto raises the score by 12.3% with gcc and 12.6% with clang relative to the old patch, using paired medians. The remaining difference from unpatched C is small compared with the spread of the rounds. This confirms a substantial regression, but not a universal 7% cost. The goc result used an Intel Xeon VM, gcc 14.2 and clang 19. Near equality of the total does not imply equality of each suite. After restoring native dispatch, RegExp remains at 0.935 of unpatched gcc's score and 1.106 of unpatched clang's score. Crypto and NavierStokes account for the largest losses with the old dispatch.

Each timing invocation used one of CPUs 1, 2 or 3, with its sibling 5, 6 or 7 idle. It started after 30 samples taken once a second in which both CPUs averaged below 1.5% busy and stayed below 10% in every sample. A sample was rejected if the sibling reached either threshold or process CPU time was below 92% of wall time. Builds, tests and Callgrind collection used CPUs 0 and 4. JavaScript ran at nice 19 in a 4 GiB systemd scope without swap.

Callgrind used fixed work instead of the adaptive score harness. Each benchmark ran setup, a fixed number of operations, and teardown. The repeat counts were Richards 4, DeltaBlue 4, Crypto 1, RayTrace 2, EarleyBoyer 1, RegExp 1, Splay 2 and NavierStokes 1. These weights differ from the geometric V8 score, so instruction percentages are not timing percentages. V8 collection covers the whole fixed workload process, including startup and script compilation. The scored benchmark timers exclude startup compilation.

| C source | gcc instructions | clang instructions |
| --- | ---: | ---: |
| Unpatched | 12,138,113,565 | 12,050,493,307 |
| Old complete patch | 12,733,447,857 | 13,940,102,751 |
| Only disable computed goto | 12,696,016,426 | 13,983,292,132 |
| Complete patch with native computed goto | 12,191,769,505 | 12,204,400,477 |

Retaining all behavior changes while restoring native dispatch reduces the excess over unpatched C from 4.90% to 0.44% for gcc, and from 15.68% to 1.28% for clang. The switch adds range checking and address calculation around a central indirect jump. Hardware branch prediction and cache effects were not measured.

After restoring native dispatch, six further alternating rounds removed one feature group at a time. The score ratios below compare each diagnostic binary with the complete patch using computed goto. Higher is faster. Each range crosses 1.0000, and none of these groups reproduces the large dispatch regression. The instruction columns use the same fixed workload, relative to that complete patch.

| Removed feature group | gcc score ratio | clang score ratio | gcc instruction change | clang instruction change |
| --- | ---: | ---: | ---: | ---: |
| Lazy error stacks | 1.0035 (0.9931 to 1.0085) | 1.0077 (0.9954 to 1.0156) | -0.310% | -0.221% |
| Pending exception protection | 1.0112 (0.9923 to 1.0247) | 0.9916 (0.9877 to 1.0164) | -0.250% | -1.105% |
| Compiler allocation failure hunks | 1.0062 (0.9954 to 1.0193) | 0.9992 (0.9870 to 1.0156) | +0.341% | -0.011% |

These diagnostic variants change behavior and cannot ship. The compiler group includes adjacent failure handling in combined hunks. Ordinary interrupt polling and the OOM flag remain when pending protection is removed.

Feature differences are not additive. For example, an unchanged regexp execution function has one fewer alignment NOP in a loop in the complete patch, accounting for 17,745,900 fewer instructions there. Removing compiler allocation failure guards also changes gcc's inlining of `js_malloc` into object allocation. These code generation effects must be distinguished from executing the guards themselves.

The retained fix disables computed goto only for `__CCGO__` and `__EMSCRIPTEN__`. Native preprocessing is identical to the measured native computed goto variant. ccgo still takes the switch branch, and the committed Go engine is unchanged. Its incremental behavior costs need their own measurements rather than the native C percentage. Native C limits stack bytes, not recursive call counts. A separate 1 MiB recursion probe reaches 1423 calls with the old gcc patch and 1393 with native computed goto, and 1926 against 1073 with clang. Both throw the same RangeError and message. Go's TLS slot checks and depth contract do not change.

### What the behavior changes cost the Go engine

The dispatch fix does not reach Go: ccgo always generated a switch, and the generated Go is unchanged. The behavior changes themselves are in Go through `fixlib`. To price them, diagnostic builds made `_js_uncatchable_pending` return false, turned off the lazy backtrace helpers so stacks are built at once, or both. Interrupt polling and the OOM flag stayed. These builds break behavior and cannot ship.

All hosts were static `CGO_ENABLED=0` builds with `-pgo=off`, Go 1.27.2 and tifa `b1cb68f6`. V8 counts cover the whole fixed workload process above. SSR counts cover two renders of each page after two warmups, in one VM for the six shop workbench pages or the four MUI pages, counted with Callgrind client requests around the render loop only. Counting the base again at the end moved the shop by 0.008% and MUI by 0.006%.

| Go variant | V8 instructions | Shop instructions | MUI instructions |
| --- | ---: | ---: | ---: |
| Base | 19,613,250,952 | 1,286,286,893 | 7,247,686,905 |
| Pending check removed | +1.11% | +1.00% | +1.52% |
| Lazy backtrace helpers off | +0.04% | +0.10% | -0.05% |
| Both | +1.09% | +1.04% | +1.48% |

Six alternating timing rounds, 32 warmups and 80 renders per page, one build of each. V8 is a score ratio, higher is faster. SSR is the sum of page P50 times, lower is faster. Ranges are the smallest and largest paired ratio.

| Go variant | V8 / base | Shop / base | MUI / base |
| --- | ---: | ---: | ---: |
| Pending check removed | 1.0101 (0.9870 to 1.0531) | 1.0165 (0.9977 to 1.0259) | 1.0043 (0.9919 to 1.0071) |
| Lazy backtrace helpers off | 1.0059 (1.0000 to 1.0457) | 1.0018 (0.9855 to 1.0110) | 0.9882 (0.9729 to 0.9913) |
| Both | 0.9513 (0.9468 to 0.9852) | 1.0171 (0.9934 to 1.0223) | 1.0195 (1.0088 to 1.0277) |

No row shows a cost that removing the feature wins back on both page sets. Removing the pending check adds instructions because it changes the code Go generates: in the V8 binary an alignment NOP lands on the dispatch path and runs 152,120,292 times. The check's own instructions are 0.19% of V8, 0.30% of the shop and 0.35% of MUI. That is the most removing its call could save, below what these timings can resolve. The hot SSR profiles reach only the backtrace attribute and settle helpers, never stack capture or formatting, so pages that make many errors were not measured.

## What was tried and not done

Each item says what it would give and why it was dropped. Check here before trying one of them again. The numbers come from tifa's [docs/performance.md](https://github.com/liustack/tifa/blob/main/docs/performance.md#what-was-tried-and-not-done) and [docs/studies/engine-hotspots.md](https://github.com/liustack/tifa/blob/main/docs/studies/engine-hotspots.md), and from the docs of the changes.

- **Removing the TLS frame of 135 more small functions** (a TLS frame is the part of libc's TLS stack where the Go translation keeps a function's C locals whose address is taken): about 0.24% of render time, too small to measure, for 135 more entries to maintain.
- **Letting more than 768 bytes of a call's room live in its TLS frame**: 1024 bytes covers 0.02% more calls, but doubles the blocks Linux keeps per call depth from 2 KB to 4 KB.
- **Keeping only 256 KiB of TLS stack per VM outside linux**: not enough. 10 VMs on the Apple M4 rendered 315 pages a second with 4.2% lock wait, against 437.8 to 481.6 with 1 MiB, since renders reach deeper into the TLS stack.
- **Moving rarely used opcodes out of the interpreter loop**: at most 8% alone on amd64, an upper bound measured by replacing the 56 handlers of opcodes the shop never runs with empty stubs, but only 1.4% on top of the rewrite that keeps 15 values in their stack slots, and 0.5% on arm64. It would need the loop's locals moved into a struct.
- **Moving small blocks in realloc without going through malloc and free**: 0.23% fewer instructions on the studio list on top of js_malloc taking cached blocks, too small for one more fixlib rule.
- **Keeping 1024 blocks a size in the front cache instead of 256**: misses on the studio list go from 19.7k to 18.6k a render. They come from the heap growing during a render, not from full lists.
- **Dispatching opcodes through closures or a table of functions**, as some Go interpreters do: a rewrite of the interpreter, against the fork's rule of making only mechanical changes. Not tried.
- **A fast path for `===` in the interpreter loop**: 1.0% to 1.3% slower, measured with v0.26.14 and PGO. The interpreter function is so large that Go inlines only the smallest functions into it, so a new helper there is one more real call.
- **The interrupt countdown in a small function of its own**: no change (0.998 to 1.001 of the time) with v0.26.14, for the same reason. v0.26.16 writes the countdown inline instead, through `pollDue`, which Go does inline, for 0.6% with PGO.
- **Writing out `JS_FreeAtom` and `is_strict_mode` in the interpreter**, as v0.26.16 does for `set_value`: nothing that could be told from noise, so they stay calls.
- **Writing the bit fields of a new object in 2 stores rather than 9**: no change (0.991 to 1.010). The time is the cache miss of the first write to the new object, and fewer stores only move it. Tried again after v0.26.17, as one store per run of bit fields in `JS_NewObjectFromShape`, `JS_NewArray` and the like: 0.3% fewer instructions, but the bench pages of tifa took 1.010 of the time (1.005 to 1.030 over 8 rounds, three builds at different addresses).
- **Taking the room of large calls from a stack per runtime**: a call whose arguments, locals and stack need more than 768 bytes mallocs them, about 6000 times in a render of tifa's bench pages, through libc's allocator and its lock. Cutting them from a block of 64 KiB per runtime instead, as calls return in reverse order, saves as many instructions as the cut itself costs, the lock is not contended with one VM, and the bench pages took 1.009 of the time (0.998 to 1.020 over 8 rounds, six builds at different addresses), the shop's 1.003.
- **Taking the TLS frame of `JS_CallInternal` without a call**: Go does not inline the fast path into the interpreter, and the frees inlined at its returns made renders of tifa's bench pages take 1.6% more time. [TLS frames taken without a call on linux](fork-changes/calls-and-tls-frames.md#tls-frames-taken-without-a-call-on-linux) leaves it out.
- **A fast path for object spread** (`JS_CopyDataProperties`): one spread took 240 ns rather than 415 in a micro benchmark on the Apple M4, but the shop did not change (0.997 to 1.005).
- **Testing the uncatchable flag before calling `_js_uncatchable_pending`** at the entry of `JS_CallInternal`: the helper is too large to inline into the interpreter, so every JavaScript call pays a real call to it. Testing the runtime's flag inline first skips that call on the normal path, and keeps the full check, which is still needed because the flag stays set after the exception is taken. 0.06% fewer instructions on V8, 0.09% on the shop and 0.41% on MUI. Over six rounds on three code layouts, V8 took 0.9988 of the base score (0.9883 to 1.0254), the shop 0.9927 of the time (0.9859 to 1.0014) and MUI 0.9850 (0.9763 to 1.0265). The sign follows the layout: MUI took 1.027 and 1.012 of the time in the two rounds of one layout, and 0.976 to 0.988 in the four rounds of the other two. The gain cannot be told from layout noise.
- **Spreading a dense array without its iterator**: `[...a]` of three elements took 165 ns rather than 615, and a call with spread arguments 394 ns rather than 869, on v0.26.17 in a micro benchmark on linux/amd64. The shop's six pages (0.997) and three MUI pages (1.007) did not change beyond the spread that the layout of the code gives, since pages rarely spread arrays. It would have added about 870 lines of fixlib rules.
- **Keeping the in-between shapes of every object literal**, before v0.26.16: about 2% more on top of the new property fast path, but the two list pages ran out of VM memory, because the kept shapes held their prototypes and everything those reach. v0.26.16 keeps only shapes whose prototype is `Object.prototype`, at most 32 keys and no unregistered symbol key, in a ring of 2048 (see [Kept shapes](fork-changes/objects-and-shapes.md#kept-shapes)).
- **Building a shape in place when no other object has its keys**: in the first form of the shape cache, copying every shape made objects of keys no other object has 22% slower to build. v0.26.16 earns one copy for each shape found, up to 64, which leaves those objects about 6% slower.
- **Literal templates in the bytecode**, as V8 copies a prebuilt object: needs compiler and bytecode format changes, and quickjs-ng saw no clear difference when it tried.
- **Allocating the property array of a literal at its final size**: the opcode that makes the object does not know the count, and growing arrays differently costs memory for every small object. It pays only together with kept shapes.
- **Inline caches for property reads**: quickjs-ng added and then removed them. Across 17 real tools they ranged from 11% slower to 54% faster, 4% faster on average, and used more memory. Tried after v0.27.0 as a table of 4096 entries keyed by the bytecode position, each holding a shape and the index of an own data property, checked against the shape itself (the index under its count, the same key, plain flags), so it needs no invalidation. Most reads on tifa's pages are own properties of the object, which `find_own_property` already finds in one hash step, and a hit on a prototype cannot skip the check of the shapes on the way. 0.3% fewer instructions, but the shop took 1.006 of the time and the MUI pages 1.007 (6 rounds, 3 layouts), for 96 KiB more per VM.
- **A table of shape steps for new keys**: `add_property` looks for the shape an object goes to in the shape hash table, and that lookup misses the cache often (2.4% of the instructions of a render of the shop, 7.7% of its cache misses). A table of 1024 steps per runtime, from a shape and a key to the next shape, holding both, cut 0.8% of the instructions on the shop and the MUI pages, but the shop took 1.008 of the time and the MUI pages 1.019 (6 rounds, 3 layouts): the shapes it holds alive (2718 to 3504 on an MUI page) add 28 KB to 142 KB per VM and change when the cycle collector runs.
- **One libc allocator per VM**, to avoid its global lock: with 8 VMs on Linux the lock costs 0.4% to 0.9% of the CPU, and since v0.26.15 10 VMs on an Apple M4 wait on locks for about 0.7% of their time.
- **Waiting for Go 1.28**: a development build made the unchanged engine 5% faster on amd64 and 2% on arm64, but the rewritten engine 5% to 6.5% slower on amd64, for a reason not found. The rewrites still win there by 4.2% on amd64 and 15.5% on arm64.
- **Dropping function source from the bytecode** (`JS_SetStripInfo`) to save VM memory: seroval, which tifa's pages use, reads the source of its own helpers with `toString` and writes it into pages, so the browser would break.
- **Removing the store of the bytecode position on arm64**: the up to 11.6% of the interpreter's own time it seemed to take is where the sampler puts the wait for the next opcode. The store is Go saving the local `pc` for the calls that read it back, on the path every opcode takes. Only moving the instructions put the samples on the opcode load instead (8.8% down to 0.4%), with the same instruction count, cycles and render time.

## Where to continue

With v0.26.15 the interpreter function alone took about 36% of a render. In rough order of what it may give:

1. **Property access.** On v0.27.0 the code that reads, writes and adds properties runs about 17% of the instructions of a render of the shop and has about 25% of its cache misses: `find_own_property` 2.1% of the instructions and 9.7% of the misses, `find_hashed_shape_prop` (the next shape of a new key) 2.4% and 7.7%. Most reads are own properties of monomorphic objects that the interpreter already finds in one hash step, and writes to existing properties already take a fast path. [Methods of strings, numbers and arrays](fork-changes/objects-and-shapes.md#methods-of-strings-numbers-and-arrays-found-in-the-interpreter) took the largest slow path out. Inline caches and a table of shape steps were tried and are slower (see above). What is left is mostly memory latency of the shapes themselves.
2. **Making and freeing objects.** On v0.27.0 about 22% of the instructions of a render of the studio list touch allocation: the engine's malloc, free and realloc 8.4%, freeing objects 4.6%, making them 4.3%, reference counts at least 2.9% and growing property arrays and shapes 1.5%. With js_malloc taking cached blocks itself the allocator is down to 6.3%. What is left is spread thin: inlining `free_property`, `js_free_shape` or `js_trigger_gc` would give at most 0.4% each.
3. **TLS frames.** About 1.5% of a render is frames the stack check counts, in `JS_CallInternal`, `js_call_c_function` and the like, which must stay. v0.26.16 takes the frames of 8 more functions per statement, and v0.26.17 takes out about a fifth of the frames of a render. tifa's engine study puts what is left to win there at most 1% to 1.2%, with a general transform in fixlib for every function whose frame address stays inside it. After v0.26.17 frames on linux are taken without a call, except in `JS_CallInternal`, which still makes about 132000 calls of `TLS.Alloc` in a render of tifa's bench pages.
4. **Regular expressions.** Testing the first character of each alternative before trying it, for react-spring's test of 148 color names, is done in v0.26.17 (see [Alternatives that cannot match are not pushed](fork-changes/regexp.md#alternatives-that-cannot-match-are-not-pushed)).
5. **Spreading a plain array without the iterator**: about 1.2% of the order list (TanStack Table calls through `fn(this, ...args)`).
6. **Less memory per VM.** All VMs could share one read-only copy of the function source and line tables from the bytecode, so `toString` still works. Estimated at about 380 KB per VM. Needs changes to the C patch, to fixlib and to the Go API.
7. **Kept shapes as a C change.** When the Go code of the engine can be generated again from C, kept shapes with a reference from child to parent shape might keep more shapes for less memory than the ring of v0.26.16. A candidate for upstream.
8. **Go 1.28 on amd64.** When it ships, measure the rewritten engine again. If it is still slower than with Go 1.27, make a small reproduction for the Go team.
