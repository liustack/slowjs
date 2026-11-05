---
summary: 'The speed numbers of the engine liustack/quickjs ran before the C engine (QuickJS translated to Go by ccgo, up to v0.28): what each release gained, the cost of the patch on native C, the Intl formatting plans, the comparison with the C engine at the switch, the acceptance run, and what was tried and dropped on that engine'
read_when:
  - Asking what the ccgo engine of v0.28 and earlier gained in each release
  - Asking how fast the C engine was against the ccgo engine when it replaced it, or what it had to pass first
  - Asking why computed goto stays on in the C tree
  - Reading a "with PGO" or "lib" column in an old measurement
  - About to try an idea that was already tried on the ccgo engine (the TLS stack, the libc allocator, the Go interpreter)
---

# History of the numbers before the C engine

These are the measurements of the engine that ran up to v0.28, QuickJS translated to Go by ccgo, and of the switch to the C engine in v0.29.0. That engine is gone, so nothing here describes how liustack/quickjs runs now. It is kept because the numbers are the record of why some things are the way they are, and a reader sometimes needs to look one up. The speed of the C engine, and how to measure a change, are in [performance.md](performance.md).

In these sections `lib` is the ccgo engine (the `lib/` folder it lived in), "with PGO" means tifa was built with an engine profile collected on that engine, and counts such as the 58 known test262 errors are the counts of that day (33 now).

## What each release gained, up to v0.28

The rows up to v0.28 were measured on the engine of those releases, QuickJS translated to Go by ccgo, which the C engine replaced. The docs they linked for the Go form of a change are gone, and their names stay as text. The speed of the C engine is in [The C engine](performance.md#the-c-engine) below.

| Release | Change | Effect on a render of the shop |
| --- | --- | --- |
| v0.26.1 | [Error stacks formatted when first read](changes/errors-and-stacks.md#error-stacks-formatted-when-first-read) | With the development build of React, a 100-item component took 11.2 ms at the median instead of 42.1 ms (darwin/arm64). Production builds stayed within 1% |
| v0.26.12 | Calls allocate no Go memory | About 11% less time on linux/amd64 and 14% less on darwin/arm64. Loading the bytecode into a VM about 11% less on linux. It also took libc's allocator lock out of every call, which held several VMs back |
| v0.26.13 | Less stack work in the interpreter loop | About 12% less on linux/amd64 and 17% less on darwin/arm64. The Go stack frame of `JS_CallInternal` went from 2608 to 896 bytes on amd64 and from 7744 to 944 on arm64 |
| v0.26.13 | [New properties of plain objects skip the generic path](changes/objects-and-shapes.md#new-properties-of-plain-objects-skip-the-generic-path) | About 6% less. A property of an object literal takes about 55 ns rather than 95 |
| v0.26.15 | Frames freed without defer | About 10% less on linux/amd64, with or without PGO |
| v0.26.15 | [Skip to where a regexp can match](changes/regexp.md#skip-to-where-a-regexp-can-match) | About 3% less |
| v0.26.15 | Cold paths of === and key names | 0.7% less with PGO, 2% without |
| v0.26.15 | One block of TLS stack per VM outside linux | 10 VMs on an Apple M4 render 1.4 to 1.55 times as many pages a second, and wait on a lock about 0.5% of their time instead of 3.8% to 5.3%. One VM and linux do not change |
| v0.26.16 | TLS frames taken per statement | 1.6% less with PGO and 1.1% without on linux/amd64, 2.6% less on darwin/arm64 with PGO |
| v0.26.16 | set_value written out in the interpreter | 1.4% less, with and without PGO. With the frames per statement, 2.8% less than v0.26.15 with PGO and 2.9% less without |
| v0.26.16 | The interrupt countdown inside the interpreter | 0.6% less with PGO, about the same without |
| v0.26.16 | A front cache for small blocks | 0.9% less with PGO alone, about the same without |
| v0.26.16 | [Kept shapes](changes/objects-and-shapes.md#kept-shapes) | 3.6% less alone, without PGO. With the front cache and the interrupt countdown, 4.5% less than without the three. Two VMs on two hyperthreads of one core render 2.9% more pages a second |
| v0.26.17 | [for in skips prototypes with no enumerable property](changes/objects-and-shapes.md#for-in-skips-prototypes-with-no-enumerable-property) | The shop barely changes (0.996). The spiral and table pages of tifa's bench app, which run many for in loops, 0.983 and 0.971 of the time, without PGO |
| v0.26.17 | [String keys that are already atoms](changes/objects-and-shapes.md#string-keys-that-are-already-atoms) | 1.3% less without PGO, measured before the for in, bitwise and comparison changes. Reading with a string key takes 13% less time in a micro benchmark, writing 30% less |
| v0.26.17 | [JSON copies runs of plain characters](changes/json.md#json-copies-runs-of-plain-characters) | 0.2% to 1.7% less, inside the spread of a round: JSON is about 1% of a render. Quoting the data of a route as one string takes 41% less time, `JSON.parse` of the render data 9.5% less |
| v0.26.17 | [Bitwise operators on floats skip the slow path](changes/operators.md#bitwise-operators-on-floats-skip-the-slow-path) | The shop has nothing to gain. The pages of tifa's MUI app take 0.952 to 0.960 of the time, without PGO |
| v0.26.17 | [The usual comparisons answered in the interpreter](changes/operators.md#the-usual-comparisons-answered-in-the-interpreter) | 0.982 of the time without PGO. The bench pages 0.940 and 0.942, the MUI pages 0.982 to 0.985 |
| v0.26.17 | One TLS frame per call on linux, with 22 more functions without a TLS frame and 12 more with frames per statement | About a fifth fewer TLS frames per render. 1.4% less than v0.26.16 without PGO, and 2.2% and 3.1% less on tifa's bench and mui test apps |
| v0.26.17 | [Alternatives that cannot match are not pushed](changes/regexp.md#alternatives-that-cannot-match-are-not-pushed) | About 1% to 2.5% less without PGO, and 3% to 5% less on the MUI pages of tifa's test apps. A `replace` with react-spring's regexp of 148 color names takes 5.9 µs rather than 44.5 |
| v0.26.17 | [New functions get length and name at once](changes/objects-and-shapes.md#new-functions-get-length-and-name-at-once) | 0.982 of the time without PGO, on top of the regexp change. 3,000 new functions make 2,000 shapes rather than 5,001 |
| v0.26.17 | [Intl calls Go through Builtins](changes/time-and-intl.md#intl-calls-go-through-builtins) | Not measured on a render. `new Intl.NumberFormat('en')` takes 2.7 µs rather than 16.5, a `DateTimeFormat` with a time zone 8.1 µs to make rather than 32.0, and a VM that has used `Intl` holds about 15 KB less |
| v0.27.0 | [Compiled regexps kept per VM](changes/regexp.md#compiled-regexps-kept-per-vm) | The journal post of tifa's example shop that highlights CSS, which compiles 392 regexps per render, renders in 0.893 of the time without PGO (0.841 to 0.903 over 6 rounds in 3 code layouts). The other pages render in the same time. A VM of the shop holds 12 KiB more |
| v0.27.0 | [The engine reads Go's clocks](changes/time-and-intl.md#the-engine-reads-gos-clocks) | The shop does not change (0.994 to 1.009 over 6 rounds, median 1.000), without PGO. `Date.now()` takes 122 ns rather than 689, `new Date()` 261 rather than 870 and `performance.now()` 107 rather than 686 |
| v0.27.0 | Likely and unlikely tests branch on their condition | 0.983 of the time without PGO (0.976 to 0.984), the bench pages of tifa 0.976 to 0.987. 1.6% fewer instructions |
| v0.27.0 | TLS frames taken without a call on linux | 0.998 of the time on top of the row above (0.995 to 1.004), the bench pages 0.990. 1.9% fewer instructions. With the row above, 0.979 of the time of v0.26.17 and the bench pages 0.967 |
| v0.27.0 | [Compiled Intl formatting plans](changes/time-and-intl.md#compiled-intl-formatting-plans) | NumberFormat takes 41% to 42% less time than main and DateTimeFormat 28% to 37% less on the fixed inputs. [Ten shop pages](#intl-formatting) render in 0.9998 to 1.0081 of main's time without PGO |
| v0.27.1 | [Methods of strings, numbers and arrays found in the interpreter](changes/objects-and-shapes.md#methods-of-strings-numbers-and-arrays-found-in-the-interpreter) | 0.996 of the time without PGO (0.992 to 0.998 over 6 rounds in 3 code layouts), and 0.975 (0.966 to 0.986) on the MUI pages of tifa's test app, whose order list reads string methods about 320000 times a render. 1.0% fewer instructions on the MUI pages, under 0.1% on the shop |
| v0.27.1 | js_malloc takes cached blocks itself | 0.985 of the time without PGO (0.971 to 0.992 over 6 rounds in 3 code layouts), the MUI pages of tifa's test apps 0.983. 2.1% to 2.3% fewer instructions on the shop, 1.3% on the MUI pages. Memory the same |
| v0.27.2 | [WeakMap entries freed in one full GC](changes/weak-collections.md#what-it-costs) | A fix, not a speedup. Render time does not change. Without PGO, a full GC after each render takes 0.989 of the time on the shop and 1.046 on the MUI pages (1.0% and 0.3% more instructions). Large weak maps collect slower: 2.9 times as long for a chain of 100,000 live entries |
| v0.27.2 | [Strings of Builtins converted in Go](changes/go-api.md#strings-of-builtins-converted-in-go) | The shop does not change that can be told: 1.004 of the time without PGO (0.997 to 1.018 over 6 rounds in 3 code layouts), since its Builtins take few strings. A Builtin that reads a 16-character string and returns a new one takes 134 ns rather than 230, one that returns its argument 81 ns rather than 97, against 60 ns for a call of a javascript function. With [Args.Latin1 and Args.UTF16](changes/go-api.md#argslatin1-argsutf16-newstringlatin1-and-newstringutf16) React's escape of a 16-character string takes 99 ns in Go against 200 ns in javascript |
| v0.27.3 | [Searches skip to where the string is](changes/strings.md#searches-skip-to-where-the-string-is) | 0.995 of the time without PGO (0.975 to 1.032 over 6 rounds in 6 code layouts), the MUI pages of tifa's test app 1.003, both inside the spread of the layouts. `String.prototype.includes` went from 1.06% of a shop profile to 0.27%, and a search through 100 KB of HTML takes 81 µs rather than 320 |
| v0.27.3 | [Young collection, VM.RunYoungGC](changes/young-collection.md) | With tifa running a young collection after each render and a full one now and then, the P99 of a render of the shop is 0.748 of what it was with one VM and the pages in random order, and 0.796 with two VMs. The MUI pages of tifa's test app 0.940 and 0.919. The P50 and CPU stay within 2.1%, and peak RSS is 0.82 to 0.88 of what it was, without PGO |
| v0.28.1 | [One copy of source text and line tables for every VM](changes/memory-and-allocation.md#one-copy-of-source-text-and-line-tables-for-every-vm) | Memory, not speed: with `EvalSharedBytecode` a VM of the shop holds 2.36 MiB less after loading, one of tifa's MUI test app 1.22 MiB less. Render time stays the same: 0.994 of the time without PGO on the shop (0.979 to 1.015 over 23 pairs of runs in 3 code layouts), 0.994 on the MUI pages (0.981 to 1.005) |
| v0.28.5 | [Own keys listed from the shape](changes/objects-and-shapes.md#own-keys-listed-from-the-shape) | 0.990 of the time without PGO on the bench pages (0.958 to 1.020 over 6 rounds in 3 code layouts), 0.994 on the store pages and 0.987 on the MUI pages of tifa's test app: 2.2% to 2.4% less in one layout, about the same in the other two. `Object.keys` of six keys takes 134 ns rather than 542 and a spread of six keys 74 ns rather than 250 (Apple M4). A VM holds 8 KiB more |
| v0.28.5 | The goasm interpreter, opt-in on linux/amd64 | With `-tags goasm`, 0.940 of the time on the shop without PGO (0.922 to 0.964 over 18 pairs of runs in 3 code layouts), 0.850 on the MUI pages of tifa's test app (0.834 to 0.875). Without the tag nothing changes |
| v0.28.6 | The goasm interpreter, the default on linux/amd64 | With several VMs in tifa's pool, without PGO: 1.046 times the shop pages a second of the Go interpreter with 4 VMs on 4 cores (1.026 to 1.082 over 3 rounds in 3 code layouts) and 1.085 with 8 VMs on 8 hyperthreads (1.060 to 1.118), 1.171 times the MUI pages (1.148 to 1.194) and 1.211 (1.205 to 1.219). After ten minutes of 8 VMs, 0.93 of the RSS on the shop and 0.88 on the MUI pages |

Whole releases, measured in tifa:

- v0.26.13 renders in 0.823 of the time of v0.26.12 (0.821 to 0.825 over 8 rounds).
- v0.26.15 renders in 0.859 of the time of v0.26.14 with a profile collected again for each (0.857 to 0.861 over 6 rounds), and in 0.843 without PGO. Whole requests with 6 VMs on 3 physical cores serve 1.17 times as many. On the Apple M4, without PGO, one VM renders 108 pages a second instead of 84 to 86, and 10 VMs 532 to 549 instead of 288 to 305, 1.80 to 1.88 times as many.
- v0.26.16 has not been measured as a whole release in tifa yet. Its changes were measured one by one in the rows above.

What it costs in memory: v0.26.16 makes one VM of the shop hold 722 KiB more after a GC (8.03 MiB rather than 7.33), 647 KiB of it the kept shapes. Outside linux each VM keeps up to 1 MiB of TLS stack since v0.26.15.


### Against the engine liustack/quickjs started from

Measured in tifa on 2026-10-20 against `modernc.org/quickjs` v0.25.0 with libquickjs v0.13.1, on the same SSR bundle and host program (tifa's [docs/studies/moejs.md](https://github.com/liustack/tifa/blob/main/docs/studies/moejs.md)): the engine changes up to v0.26.15 alone make renders 1.63 to 1.66 times as fast. With the rewrites of tifa's Vite plugin and PGO, pages render 2.07 times as fast as on the original engine with neither.

## Intl formatting

v0.27.0, against main `641e996`, with tifa shop `fb31f616`. The shared go-intl formatters use [compiled plans and direct string output](changes/time-and-intl.md#compiled-intl-formatting-plans). The fixed-input NumberFormat cases take 41% to 42% less time than main and DateTimeFormat 28% to 37% less. The [full call table](changes/time-and-intl.md#focused-timing-and-memory-costs) includes construction, uncached resolution, allocation and memory costs.

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

This section measured the patch of the ccgo engine (`quickjs.diff`, now deleted) on native C builds. It stays as the record of why computed goto stayed on in the C tree.

Measured on 2026-10-22, using the pinned Bellard source `3d5e064e9dd67c70f7962836505a7fa067bf0a4e` and liustack/quickjs at `620658c`. The old C patch unconditionally set `DIRECT_DISPATCH` to zero. That translation setting predates the exception changes of liustack/quickjs. Native C therefore used a central switch instead of upstream's computed goto dispatch.

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


## tifa's pages on the C engine, against the old engine

Measured with tifa's `test/ssr` on the Google Cloud VM above, one core pinned with its hyperthread sibling idle: the six workbench pages of `examples/shop` (the pages of `BenchmarkShop`), the three pages of `test/apps/mui` and the two of `test/apps/bench`. Each page renders in a warm pool of one VM with `GOMAXPROCS=1`, 64 renders to warm up, then the P50 of 100 renders (60 on the MUI pages). 3 rounds at the 8 [code positions](performance.md#code-positions). The engine of `lib/` is built without PGO and with a profile of 500 renders of the shop. A ratio is the median over positions of the paired ratios, and the C engine was faster at all 8 positions on every page.

| Page | lib (ms) | lib with PGO (ms) | C engine (ms) | C / lib | C / lib with PGO |
| --- | ---: | ---: | ---: | ---: | ---: |
| shop: studio | 71.16 | 70.71 | 59.61 | 0.837 | 0.848 |
| shop: studio orders | 12.34 | 12.23 | 10.47 | 0.846 | 0.861 |
| shop: studio stats | 14.18 | 14.04 | 12.04 | 0.842 | 0.854 |
| shop: weekend | 42.12 | 41.58 | 35.40 | 0.839 | 0.849 |
| shop: weekend orders | 11.75 | 11.62 | 9.97 | 0.847 | 0.857 |
| shop: weekend stats | 12.07 | 11.98 | 10.24 | 0.845 | 0.855 |
| MUI: `/` | 44.10 | 43.38 | 37.57 | 0.848 | 0.867 |
| MUI: `/orders` | 94.93 | 93.05 | 81.54 | 0.861 | 0.872 |
| MUI: `/orders/ORD-1240` | 41.58 | 40.99 | 35.61 | 0.852 | 0.867 |
| bench: `/spiral` | 25.45 | 24.92 | 20.82 | 0.819 | 0.836 |
| bench: `/table` | 22.20 | 21.96 | 18.31 | 0.824 | 0.836 |

Memory, from the resident memory of the process (VmRSS, after a Go GC that gives the Go heap back) with a pool of 8 VMs less that with 1, divided by 7, after loading and after every VM rendered every page 32 times:

| Per VM | lib, loaded | lib, rendered | C, loaded | C, rendered |
| --- | ---: | ---: | ---: | ---: |
| shop | 8.7 MiB | 11.6 MiB | 8.4 MiB | 12.3 MiB |
| MUI | 4.1 MiB | 7.5 MiB | 3.9 MiB | 7.7 MiB |

The process with one VM is smaller on the C engine: 34.0 MiB against 42.4 MiB after the shop's renders, 26.2 MiB against 31.8 MiB after MUI's. A VM of the shop holds 7.8 MiB of allocations after loading and 8.3 MiB after the renders (9.0 and 9.5 MiB on lib), in a footprint of 10.2 and 12.7 MiB.

What tifa's wiring changed, against the C engine before it, by callgrind's instructions per render (the difference of a run of 12 renders a page and one of 2, so that loading cancels out) and by time at the 8 positions (with the engine of main at 09e731a3 on both sides):

- **Strings that javascript gives Go are read where the engine keeps them**: a render's result, the JSON of an object and the arguments of Go functions. `JS_ToCStringLen2` made a UTF-8 copy in the VM's memory of any string that is not ASCII (3 bytes a unit for a 16-bit string), which failed with out of memory when the VM had no room. A rope is joined first. A 16-bit string goes to UTF-8 in one pass, four units at a time where they are ASCII or all from U+0800, into a buffer that is reused, then one copy. Counting the bytes first, a second pass, made it slower than the C: 1.05% more instructions on the shop, whose pages are Chinese. The one pass takes 1.03 million instructions a render of the shop where the C took 1.34 million: 0.22% fewer on the shop, 0.03% on MUI and the bench pages.
- **The hash of emotion** (tifa's Builtin for `@emotion/hash`) reads 8-bit and 16-bit strings where the engine keeps them and multiplies in 32 bits, as the JavaScript's halves amount to: 5.1 million instructions a render of an MUI page down to 1.1 million, 1.86% of the render.
- **tifa takes `startRender` and its readers once at Load**, where each render compiled three small scripts: 0.07% to 0.14% fewer instructions.
- **tifa gives memory back** with `ReleaseMemory`: all the free memory after Load (2.1 MB on the shop, 1.0 MB on MUI), and after a full GC what is beyond what the renders until the next GC fill. Loaded VMs take 0.2 to 0.4 MiB less. After renders the free memory is taken again, and the full GC gives back only what a big render took once the GC threshold has come back down.

Together: the shop 0.37% fewer instructions and 0.994 of the time (7 of 8 positions faster), MUI 1.96% and 0.984 (8 of 8), the bench pages 0.12% and 0.994 (7 of 8). Each is under the 2% the rule counts as time, and none is slower.

Measured and left (shares of a render on the C engine, from callgrind):

- **The path of a Builtin call** (`hostCall`, `callBuiltin`, reading the arguments and making the result) is 0.34% of a shop render with the Builtin's own work, and about 0.2% of an MUI render beside the hash.
- **Joining ropes in Go** instead of `JS_ToString`: the join takes 0.33% of a shop render and 0.23% of MUI. Walking the pieces in Go still reads every character, so it would save part of that.
- **AsyncContext as C**: its Builtins are under 0.05% of a render.
- **The polls**: `host_poll` is 0.06% to 0.16% of a render.

## Acceptance before the switch

Run on 2026-10-29 at liustack/quickjs `75052b72` and tifa `41dcf45` (branch `phase5/accept`: tifa upgraded to that commit of liustack/quickjs, with its engine profile collected again), on the Google Cloud VM above. The C engine is built without PGO. Where it is compared with the engine of `lib/`, that one is built with the tifa engine profile.

| Gate | Result |
| --- | --- |
| test262 as before | Native C tree: the same 58 known errors, line for line (58 of 83,426). Go host: 80,215 of 96,540 runs, the number of the v0.27.2 row of [benchmarks.md](benchmarks.md), the same in every area, in ES2023 and older, and under every feature tag. 0 crashes, timeouts and out of memory |
| tifa's test pages, same HTML | shop 160 pages, pages 33, tailwind 2, mui 60, bench 4, extreme 30: all the same, except `/json/2001` of extreme, which the C engine answers with a 500 (its JSON nests at most 2,000 levels, as tifa's docs say). `api` has no pages. tifa's `go test ./...` passes on the default build, and on the C engine all but one test of `internal/ssr` (below). The end-to-end tests pass on both |
| No page slower than on `lib/` with PGO | All 11 pages faster at all 8 code positions: 0.834 to 0.860 of the time |
| GC pause P99 | Not above the stored `lib/` numbers: 1 VM 131 to 164 µs against 164, 3 VMs 197 µs against 197 |
| Soak | 150 minutes, 211,076 renders, 0 errors, no VM replaced. The memory of the VMs and what they took from the system did not move. The process grew 3.8 MB between minutes 30 to 60 and minutes 120 to 150 |

**Same HTML.** Every page of a crawl of the built program, the default build against the C engine, byte for byte after masking the times the loaders write into a page (RFC 3339 text and the `u:` milliseconds of the router's state). The default build against itself, crawled twice, masks exactly as many (shop 5,169), so the masked bytes are not the engine's. Extreme was crawled with `--ssr-timeout 60s`, as tifa's end-to-end test does, because `/console/100` takes 1.4 to 3.5 s on `lib/` and a default 3 s timeout turned it into a 500 in one crawl, depending on the load. The shop uses `TestShopCrawl` of tifa, the other apps a small crawler of the same kind.

**One failing test on the C engine.** `TestFullGCGivesBackWhatTheThresholdNoLongerNeeds` in tifa's `internal/ssr` fails the same way each time and on darwin/arm64 too: after 1,000 renders full GCs gave back 68,688 bytes where it wants at least half the live size, 112,008. The VM there has a live size of 224,016 bytes and a footprint of one 262,144-byte chunk. It skips on `lib/`, which takes its memory from libc, and the pin tifa had before had no engine to run it on. The test next to it passes.

**Speed.** Median over 4 rounds of the paired ratio C engine / `lib/` with PGO, at the 8 [code positions](performance.md#code-positions), `GOMAXPROCS=1` on one core with its sibling idle. The `range` column of the tool (the positions of `lib/` against each other) is 1.4% to 4.2% on the pages. No process had to be run again. Another test was running on the two cores next to it the whole time.

| Page | `lib/` with PGO (ms) | C engine (ms) | C / lib |
| --- | ---: | ---: | ---: |
| shop: studio | 70.17 | 58.53 | 0.834 |
| shop: studio orders | 29.22 | 24.66 | 0.843 |
| shop: studio stats | 32.48 | 27.10 | 0.834 |
| shop: weekend | 41.46 | 34.67 | 0.838 |
| shop: weekend orders | 25.77 | 21.73 | 0.843 |
| shop: weekend stats | 25.15 | 21.01 | 0.835 |
| MUI: `/` | 42.22 | 36.21 | 0.856 |
| MUI: `/orders` | 90.84 | 78.19 | 0.860 |
| MUI: `/orders/ORD-1240` | 40.00 | 34.13 | 0.853 |
| bench: `/spiral` | 24.15 | 20.37 | 0.844 |
| bench: `/table` | 21.18 | 17.84 | 0.849 |

The shop's order and stats pages are two to three times what they were in the table above, because the shop got bigger pages after that measurement.

**GC pauses.** The method of the [stability batch](performance.md#stability-under-load): a goroutine allocates 512 MB a second, so the Go GC runs 65 to 86 times a second, while the pool renders the 18 pages for 60 seconds after 10 of warm-up. 1 VM on cores 1,2 (two physical cores) and 3 VMs on cores 1,5,2,6, 5 rounds each. The `lib/` numbers are the single runs of the stability batch.

| Pause of the whole stop | C engine P50 | P99 | largest | `lib/` P99 | largest |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1 VM | 32.8 µs | 131.1 (3 rounds), 163.8 (2) | 2,097 to 3,146 µs | 163.8 | 4,194 |
| 3 VMs | 57.3 µs | 196.6 (5) | 459 to 917 µs | 196.6 | 2,097 |

The histogram of the runtime has bucket edges 131.1 and 163.8 next to each other, so the 1 VM P99 moves by a bucket between rounds. The 6 pages of `TestShopGCPause` give 114.7 to 131.1 µs for 1 VM and 196.6 for 3 VMs (3 rounds each). The pages of the shop are heavier than in the batch, so the renders a second are lower (18 against 45 for 1 VM).

**Soak.** 2 VMs, 2 callers, the 18 pages in turn with no pause, and every 5 minutes 3 seconds in which no one renders, so that the pool gives the free memory of its VMs back (60 times, 30 for each VM). Memory read every minute.

- Renders: 211,076, 0 errors, 2 VMs created, none closed or replaced. Render P50 91.3 ms, P99 165.0, P99.9 167.6, largest 246.6. Each 30 minutes the same to within 2 ms.
- Per VM: the allocations stayed between 12.18 and 12.36 MB, the footprint at 20,905,984 bytes in 21 chunks the whole time.
- Process: VmRSS 75.0 MB at the start, 77.3 MB on average over minutes 30 to 60 and 81.2 MB over minutes 120 to 150 (+2.5 MB an hour by a fit over minutes 30 to 150, +1.9 over the last hour). Part of it is Go's: the memory the Go runtime holds (`Sys` less what it released) went from 22.2 MB to 26.4 MB while `Sys` stayed at 30.5 MB from minute 15. What is left (the chunks of the VMs, the shadow stacks, the binary) was 41.5 MB from minute 60 to the end.
- The shadow stacks are 3 mappings of 32 MiB, resident 2,156 KB together.

**Only reported.**

- Per VM, from 8 VMs against 1 as tifa's `TestShopVMResident` does, 300 renders each, 3 runs: after loading 8.3 to 8.6 MiB, after rendering 12.8 to 12.9, and after the pool let it wait idle for 1.5 s (so with `ReleaseMemory`) 9.7. Without it the VM keeps 3 MiB more.
- Binary of the shop program (tifa with pages): 37,433,922 bytes on `lib/`, 37,615,732 with the C engine (+0.5%). `.text` 7.92 MB to 8.37 MB, `.rodata` 20.80 to 20.87, `.gopclntab` 5.59 MB to 5.44 MB (smaller by 152 KB). The MUI program is 29,193,071 and 29,374,881 bytes, `.gopclntab` 5.40 and 5.24 MB.


## What was tried and not done on the old engine

The same kind of list as in [performance.md](performance.md#what-was-tried-and-not-done), for the engine translated by ccgo. Most of these are about its TLS stack (the part of libc's stack where the translation kept the locals of a C function whose address is taken), its libc allocator and its Go interpreter loop, none of which exist in the C engine.

- **Removing the TLS frame of 135 more small functions** (a TLS frame is the part of libc's TLS stack where the Go translation keeps a function's C locals whose address is taken): about 0.24% of render time, too small to measure, for 135 more entries to maintain.
- **Letting more than 768 bytes of a call's room live in its TLS frame**: 1024 bytes covers 0.02% more calls, but doubles the blocks Linux keeps per call depth from 2 KB to 4 KB.
- **Keeping only 256 KiB of TLS stack per VM outside linux**: not enough. 10 VMs on the Apple M4 rendered 315 pages a second with 4.2% lock wait, against 437.8 to 481.6 with 1 MiB, since renders reach deeper into the TLS stack.
- **Moving rarely used opcodes out of the interpreter loop**: at most 8% alone on amd64, an upper bound measured by replacing the 56 handlers of opcodes the shop never runs with empty stubs, but only 1.4% on top of the rewrite that keeps 15 values in their stack slots, and 0.5% on arm64. It would need the loop's locals moved into a struct.
- **Moving small blocks in realloc without going through malloc and free**: 0.23% fewer instructions on the studio list on top of js_malloc taking cached blocks, too small for one more fixlib rule.
- **Keeping 1024 blocks a size in the front cache instead of 256**: misses on the studio list go from 19.7k to 18.6k a render. They come from the heap growing during a render, not from full lists.
- **A fast path for `===` in the interpreter loop**: 1.0% to 1.3% slower, measured with v0.26.14 and PGO. The interpreter function is so large that Go inlines only the smallest functions into it, so a new helper there is one more real call.
- **The interrupt countdown in a small function of its own**: no change (0.998 to 1.001 of the time) with v0.26.14, for the same reason. v0.26.16 writes the countdown inline instead, through `pollDue`, which Go does inline, for 0.6% with PGO.
- **Writing out `JS_FreeAtom` and `is_strict_mode` in the interpreter**, as v0.26.16 does for `set_value`: nothing that could be told from noise, so they stay calls.
- **Writing the bit fields of a new object in 2 stores rather than 9**: no change (0.991 to 1.010). The time is the cache miss of the first write to the new object, and fewer stores only move it. Tried again after v0.26.17, as one store per run of bit fields in `JS_NewObjectFromShape`, `JS_NewArray` and the like: 0.3% fewer instructions, but the bench pages of tifa took 1.010 of the time (1.005 to 1.030 over 8 rounds, three builds at different addresses).
- **Taking the room of large calls from a stack per runtime**: a call whose arguments, locals and stack need more than 768 bytes mallocs them, about 6000 times in a render of tifa's bench pages, through libc's allocator and its lock. Cutting them from a block of 64 KiB per runtime instead, as calls return in reverse order, saves as many instructions as the cut itself costs, the lock is not contended with one VM, and the bench pages took 1.009 of the time (0.998 to 1.020 over 8 rounds, six builds at different addresses), the shop's 1.003.
- **Taking the TLS frame of `JS_CallInternal` without a call**: Go does not inline the fast path into the interpreter, and the frees inlined at its returns made renders of tifa's bench pages take 1.6% more time. TLS frames taken without a call on linux leaves it out.
- **One libc allocator per VM**, to avoid its global lock: with 8 VMs on Linux the lock costs 0.4% to 0.9% of the CPU, and since v0.26.15 10 VMs on an Apple M4 wait on locks for about 0.7% of their time.
