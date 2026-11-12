---
summary: 'How much faster each release of the C engine renders pages, how to measure an engine change in tifa and across code positions, and the measurements of each change for speed (hot loops, appends, objects, calls, React helpers in C, JSON, locals, the optimizer pass, the cost of the generated code against native C, stability under load), with what was tried and not done'
read_when:
  - Making a change to the engine for speed, or reviewing one
  - Measuring a branch of liustack/quickjs on tifa's pages before a release
  - A speed result of a few percent, which may only be where the code landed
  - About to try an engine idea that may already have been tried
  - Planning the next round of work on render speed
  - Asking what RegisterEscapeText gains in tifa's pages, or what a cache in front of it would give
  - Asking what RegisterCopyProps gains in tifa's pages, or whether a check in JavaScript in front of a spread would do
  - Asking what RegisterStartTag and RegisterEndTag gain on the start and end tags of tifa's pages
  - Quoting how fast a release is
  - Checking that the C engine stays flat, scales over VMs and stops soon after Interrupt
  - Asking how much slower the generated engine is than native C, or what a render spends its time on
  - About to change how the polls are counted, or to try PGO on the C engine
---

# Performance

English · [简体中文](performance.zh-CN.md)

liustack/quickjs is made for tifa, which renders React pages to HTML in the engine, so render speed is measured on the pages of tifa's example shop: one warm VM rendering the order list, an order and the stats of each of its two shops (`BenchmarkShop` in tifa's `test/ssr`). Every change for speed leaves the HTML the same byte for byte. The doc of each change in [changes](changes/README.md) has its full measurement, and tifa's [docs/performance.md](https://github.com/liustack/tifa/blob/main/docs/performance.md) has what the numbers mean for an app. How the engine compares with the other pure Go engines, on tifa's pages and on general workloads, is in [benchmarks.md](benchmarks.md).

The engine changes up to v0.26.15 alone made renders 1.63 to 1.66 times as fast as on the engine liustack/quickjs started from (modernc.org/quickjs v0.25.0 with libquickjs v0.13.1), and 2.07 times as fast with the rewrites of tifa's Vite plugin and PGO (measured on 2026-10-20). The numbers of the engine before the C engine (up to v0.28) are in [history.md](history.md).

## How the numbers were measured

Unless a row says otherwise:

- Machine: a Google Cloud VM with 8 vCPUs of an AMD EPYC 7B13 (Zen 3), Linux, one core pinned with its hyperthread sibling idle. Some rows are on an Apple M4 (darwin/arm64), which is noisier.
- tifa is built without PGO (profile-guided optimization). It stopped using it with the C engine, where the profile made no difference. "With PGO" appears only in [history.md](history.md).

## What each release gained

Releases from v0.29.0 run the C engine. Instructions are counted with callgrind on the pages of tifa's example apps, and every row has the same HTML as before. Times are ratios to the build before the change, on the pages named, by the rule of [measuring.md](measuring.md). A ratio inside the spread of the code positions is no change. The rows of releases up to v0.28 (the engine translated by ccgo) are in [history.md](history.md#what-each-release-gained-up-to-v028).

| Release | Change | Effect on a render |
| --- | --- | --- |
| v0.29.0 | The C engine replaces the engine translated by ccgo ([numbers](history.md#tifas-pages-on-the-c-engine-against-the-old-engine)) | The 11 pages of tifa's shop, MUI app and bench app render in 0.834 to 0.860 of the time of the old engine built with PGO, faster at every code position. The pause of the Go GC stays as short |
| v0.29.1 | [Ordinary objects take 48 bytes](#ordinary-objects-of-48-bytes) | Memory, not speed: the highest `MemorySize` of a long Markdown page is 3.8% lower, and the shop holds 1.8% less at the end of a render |
| v0.29.1 | [Object literals made at their final shape](#object-literals-made-at-their-final-shape) | The shop's workbench pages 0.956 of the time, the MUI pages 0.969, the bench table page 0.942. React alone 0.991 |
| v0.29.1 | [Calls set four locals at a time, for in holds a shape, hasOwnProperty.call in the interpreter](#calls-for-in-and-hasownpropertycall) | The bench table and spiral pages 0.958 and 0.967. With the for in and hasOwnProperty.call change, the shop 0.984, the MUI pages 0.981, React 0.988 (2.5% to 3.1% fewer instructions) |
| v0.29.1 | [Shared bytecode scanned once](#what-a-new-vm-and-loading-a-bundle-cost) | `EvalSharedBytecode` loads a bundle in 0.79 to 0.86 of the time after the first load |
| v0.29.2 | [RegisterEscapeText](#react-doms-text-escape-in-c) | 0.7% to 4.5% fewer instructions. The shop 0.974 of the time, the bench pages 0.938 |
| v0.29.3 | [RegisterCopyProps](#the-copy-of-the-props-of-a-react-element-in-c) | 2.1% to 4.1% fewer instructions. The shop 0.988 and 0.989, the MUI pages 0.980 |
| v0.29.4 | [The interpreter calls a C function itself](#c-functions-called-from-the-interpreter) | 0.74% (shop) to 2.3% (bench pages) fewer instructions. The pages 0.98 to 0.99 of the time, inside the noise |
| v0.29.4 | [Object literals keep their properties in the object's block](#object-literals-with-their-properties-in-the-block-of-the-object) | 1.0% to 2.4% less memory after a render, 0.1% to 0.8% fewer instructions. No page faster or slower |
| v0.29.5 | [The end of a call, and eight locals to a turn](#the-end-of-a-call-and-the-start-of-one) | 0.7% to 2.1% fewer instructions for the end of a call, 0.2% to 0.5% for the locals |
| v0.29.5 | [Opcodes that do a few instructions at once](#opcodes-that-do-a-few-instructions-at-once) | 1.59% to 3.41% fewer instructions. The bench pages 3.4% less time |
| v0.29.5 | [Object literals that share their shape](#object-literals-that-share-their-shape) | 12% to 15% fewer shapes in a VM, 0.7% to 1.7% less memory |
| v0.29.5 | [23 fixes of upstream QuickJS](#upstream-commits-and-the-scripts-where-the-c-engine-was-slower-than-c) | Fixes. Over 37 scripts the geometric mean is 0.917 of the time before the batch, nearly all of it `q_array_slice` (0.085). `r_react_ssr` 0.977, inside its noise |
| v0.29.6 | [Results of calls between generated functions in registers](#what-the-generated-code-costs-against-native-c) | 0.37% to 0.65% fewer instructions. The shop 0.978 of the time, MUI 0.984 |
| v0.29.6 | [Checks of the temporal dead zone that cannot fail dropped](#checks-of-the-temporal-dead-zone) | 0.06% to 0.77% fewer instructions. The shop 0.995, MUI 0.997 |
| v0.29.7 | [The optimizer runs again over the linked module (amd64)](#the-optimizer-over-the-linked-module) | 0.6% to 1.2% fewer instructions, 3.5% on `r_react_ssr`. The time stays inside the spread |
| v0.29.7 | [Locals that share a slot](#locals-that-share-a-slot) | 0.82% to 1.81% fewer instructions. The time stays inside the spread |
| v0.29.7 | [JSON.parse reads keys and plain strings from the text](#jsonparse-reads-keys-and-plain-strings-from-the-text) | A parse takes 31% to 60% fewer instructions. The bench table page 3.91% fewer, the shop 0.31% |
| v0.29.8 | Fixes taken from upstream (iterators, sparse array truncation) | Not a speed change |
| v0.29.9 | Ports to linux/arm64, darwin/amd64 and windows/amd64 | Not a speed change |
| v0.29.10 | [The optimizer runs again over the linked module (arm64)](#on-arm64) | 0.56% to 0.99% fewer instructions on linux/arm64. No slowdown on an M4 |
| v0.29.10 | [Shapes of built-in classes, shapes found by their parent, objects that die alone](#shapes-of-built-in-classes-shapes-found-by-their-parent-objects-that-die-alone) | 1.84% to 2.13% fewer instructions on the shop and MUI pages, 0.49% on the bench pages. The bench pages 0.73% less time |
| v0.29.10 | [RegisterStartTag and RegisterEndTag](#react-doms-start-and-end-tags-in-c) | The workbench pages of the shop take 7.3% less time, the storefront pages 7.0%, the MUI pages 3.8% and the bench pages 33%. A VM holds about 26 KB more |
| v0.29.11 | [A stack check in every function that calls](#every-function-that-calls-has-a-stack-check) | A cost, for a safe fault: up to 0.6% more time (0.27% to 0.36% more instructions) |
| v0.30.0, v0.30.1 | Interrupt reaches the tag functions, long ropes, `repeat` and regexp compiles | Fixes. No speed claim |

## Measuring in tifa

Judge every timing by the protocol of [measuring.md](measuring.md). This section is how to build and run tifa against a checkout.

To measure a branch of liustack/quickjs before it is released, point tifa at the checkout with a `replace`, build the example app, and run the benchmarks of tifa's [docs/testing.md](https://github.com/liustack/tifa/blob/main/docs/testing.md#performance-comparisons):

```sh
# in tifa
go mod edit -replace github.com/liustack/quickjs=../quickjs
(cd examples/shop && pnpm install --frozen-lockfile && go tool tifa build)
TIFA_SSR_SHOP=1 go test -run '^$' -bench 'Shop/^render' -benchtime 2000x ./test/ssr
TIFA_SSR_SHOP=1 go test -run ShopVMMemory -v ./test/ssr
TIFA_SSR_SHOP=1 go test -run '^$' -bench ShopParallel -benchtime 15s ./test/ssr
```

- Build the test binary once for the old engine and once for the new one (`go test -c -o old.test`, `-o new.test`), and run them in turn on the same pinned core, round after round. Read the ratio of the sums of the six P50 within each round.
- `go test` builds unless given `-pgo=<file>`, and tifa builds too, so these compare the engines as tifa runs them.
- Compare the HTML of every page between the two builds. The router writes the time of the render into each match (`u:` followed by 13 digits), so mask that first.
- A benchmark with one VM cannot see contention between VMs. For a change near memory allocation, also run `BenchmarkShopParallel` and read its `speedup` and `lock-wait-%`.
- Note the load of the machine during each round, and run again when other work used more than about 10% of the CPU.
- Remove the `replace` before committing in tifa.

After a release, upgrade tifa (see [releasing.md](releasing.md#after-the-release)).


### Code positions

Where the interpreter's code lands in the binary changes its speed by 5% to 14% on its own, so one build against one build can show a change that is only a move. Measure each side at several positions (layouts), by the protocol of [measuring.md](measuring.md): it says which layouts (the `front` and `shift` of a position are defined there), in what order, how to read the result and what it can and cannot see.

Until that protocol, `shift` padded the first function of `cutils_amd64.s`, which is never called, so the linker dropped it with its NOPs: in the C engine of v0.29.8 a shift of 32 or 1664 bytes left every function at the same address and moved only some read-only data. The results below that name "the whole-program move" measured that.

Build every position with the same flags (`-pgo=off` or the same profile), and put a measurement of the same engine at all positions next to the one of the change.

Making the positions:

- The tool in [testing.md](testing.md#time-one-build-against-another) makes the positions of the C engine, runs the builds in turn and judges the result (`bench build`, `bench run`, `bench judge`), and counts instructions (`bench cg`). By hand: C engine (`internal/cengine`): copy the generated `*_amd64.s` files, run `go run ./internal/goasm/cmd/xform DIR shift:1664:dbuf_init2 front:JS_CallInternal_block:16` on the copy (use `shift:0` and `front:...:0` for no move, and the name of the first engine function of your binary, which `go tool nm -n` shows), and build with the copy. The bytes of `shift` sit after a `RET` and never run. The bytes of `front` sit after the `PCALIGN` at the start of the interpreter body and run once per call.

A `front` that is not a multiple of 16 (8, 24, 40, 56) moves the body and the constant pool it keeps after its code together, so the pool leaves its 16 bytes. The interpreter as built reads its pool with scalar instructions only (`movsd`, `ucomisd`), which need no alignment, so this does no harm today. A build in which it reads the pool with an instruction that needs 16 bytes, as a profile-guided one did in 19 places, faults at those positions. Make those two places with the generator instead: `-align64 JS_CallInternal:48` and `-align64 JS_CallInternal:0` put the body there with its pool on 16 bytes.

## The C engine

The C engine of `internal/cengine` is the only engine since the switch. Its numbers come from the `jsrun` binary on the Google Cloud VM above, one core pinned with its hyperthread sibling idle, at the 8 code positions of [Code positions](#code-positions), and from callgrind. When the C engine replaced the ccgo engine in v0.29.0, every page of tifa rendered in 0.834 to 0.860 of the time, with the same HTML, and the pause of the Go GC was not longer than before ([history.md](history.md#acceptance-before-the-switch)).

### The C engine's hot loops on fixed places

The generator puts the body of the C engine's interpreter at 40 bytes into a line of 64 bytes (0 until a JSValue was copied in two moves of 8 bytes, see [below](#the-c-engine-copies-a-jsvalue-in-two-moves-of-8-bytes)) and the body of its regexp matcher at 16 (`-align64`, see [c-engine.md](changes/c-engine.md#where-the-interpreter-and-the-regexp-matcher-sit)). Before, each body sat after a `PCALIGN $16` and moved within its lines with any change to code linked before it. The 40 was measured for the old loop, `JS_CallInternal`. The register loop `JS_RegvmRun` has no fixed place, and only the matcher is named (see [Where to continue](#where-to-continue)).

With the bodies fixed, the whole-program move of a position no longer moves the interpreter body within its lines: it moves the code and data around it, and the front NOP still moves the body. So the 8 positions now put the body 0, 8, 16 and 24 bytes past its place, each with two places of the rest. Keep measuring all 8, or the 4 of the quick check: the move of the rest alone still changes `q_array_read` by up to 8%, `q_int_arith` by up to 6% and `q_prop_read` and `c_string_concat` by up to 5%. Even with the interpreter at the same address in two builds, `q_regexp_ascii` took 389 ms in one and 399 ms in the other.

Measured on 12 scripts of the C engine at all 8 positions, 5 rounds:

- Against the bodies after a `PCALIGN $16`: 0.983 of the time as a geometric mean, `q_prop_read` 0.966, the render scripts 0.995 to 1.000, and the rest within the noise of the positions. `q_array_read` took 0.918 here, but 1.008 in a later run of all 37 scripts against main at 4 positions, where its positions alone ranged from 0.894 to 1.073: it follows where code and data land too closely to read a change of a few percent from it.
- The position ranges are not narrower as a whole: 5 scripts narrower (`q_int_arith` from 1.157 to 1.091, `v8_richards` from 1.056 to 1.030), 5 wider (`q_array_read` from 1.109 to 1.151, `c_regexp_mix` from 1.014 to 1.038). The largest effect of the whole-program move alone drops from 3.9% to 2.9% on average over the scripts.
- The place matters. With the function on 64 bytes and the body after a `PCALIGN $16`, the interpreter body sits at 48, and 8 scripts took 1.034 of the time they take with it at 0 (`q_int_arith` 1.091), at 16 1.007, at 32 1.013 (2 positions each). The matcher at 0 makes `q_regexp_ascii` take 1.072 of the time it takes at 16.

### The C engine after the property-layer cut and the polls

Measured by the rule above on the C engine (`jsrun`, one core of an EPYC 7B13, its other hyperthread idle):

- **Cutting the call cycle of the property layer** made the loops that turn numbers into strings faster, against the commit before it at 8 positions: `m_num_to_str` 0.875 of the time, `m_concat_short` 0.912, `m_concat_num` 0.957, `c_string_concat` 0.939, `c_string_methods` 0.945, `r_react_ssr` 0.869. Only `c_json_stringify` got slower (1.038, 8 of 8 positions, 0.77% more instructions): with the recursion gone LLVM inlined in another order, and `string_buffer_concat` went from a cost of 70, inlined, to 540 against the threshold of 400, a call. `JSON.stringify` paid it for every piece it writes, 3.25 million calls and 58.5 million instructions. An always inlined copy for `string_buffer_concat_value` gives it back: 0.978 of the time of main at 4 positions and 0.974 in the run of all 37 scripts below, with 0.6% fewer instructions than before the cut.
- **The polls** left `q_int_arith` as it was: 0.984 of the time of the engine before them at 8 positions, within its own range of 1.10 (a process runs it in either about 507 or about 633 ms). It now runs 219 instructions a loop where it ran 353, in the same time. The loop is not bound by instructions. Each turn the value stack has two loads of 16 bytes of a value that two stores of 8 bytes wrote, and on Zen 3 that cannot be forwarded (24 cycles, against 1 for a load of 8 bytes of a store of 8), but [removing them](#the-c-engine-copies-a-jsvalue-in-two-moves-of-8-bytes) left the time as it was. Changing the base register between the store and the load costs nothing more, nor do a load and a store 4 KiB apart. What binds the 77 cycles a turn is not found. `q_array_read` took 1.082 of the time in that run with 36% fewer instructions, but its time follows its layout closely (see above).
- **All 37 scripts** with the inlined copy and the loops on fixed places, against main at 4 positions: 0.997 of the time as a geometric mean. Faster: `q_prop_read` 0.942, `q_string_build1` 0.952, `c_string_concat` 0.969, `c_json_stringify` 0.974. `q_string_build2` took 1.027 with the same instructions (0.999996), so that is layout. The render scripts are 0.989 to 1.002.

Not done:

- **Moving the heap path of `js_dtoa2` out of line**, which LLVM no longer inlines either (cost 1370 against 230): 1.5 million fewer instructions on `c_json_stringify`, too few to keep.

### The C engine copies a JSValue in two moves of 8 bytes

`quickjs.c` is built without the SLP vectorizer and with `+slow-unaligned-mem-16` (see [c-engine.md](changes/c-engine.md#how-a-jsvalue-is-copied)), so a JSValue is copied with two loads and two stores of 8 bytes in place of one SSE move of 16. The interpreter writes a JSValue as two stores of 8 bytes, and on Zen 3 (EPYC 7B13) a load of 16 bytes over them cannot be forwarded. It waits for both stores to reach the cache. Counted with a memory trace, the loads like that fell from 433,938 to 4,541 in short runs of `r_react_ssr`, and the 16-byte stores read back in parts from 813,223 to 14,349.

Measured by the rule above, against main (5db67b0f). The interpreter body is now fixed at 40 bytes into its line, main's at 0, so each side is at its own place and the positions left are the whole-program move of 0 and 32 bytes. Time is the median of the paired ratios over the 2 positions, 4 rounds, all 37 scripts:

- **Geometric mean 0.954** (0.953 and 0.955 at the two positions). 22 scripts are faster by more than the rule asks, 13 are within it, 2 are slower.
- **The render scripts**, 10 rounds: `r_react_ssr` 0.979, `r_bench_table` 0.977, `r_bench_spiral` 0.976, `r_shiki_regexp` 0.986, the same at both positions.
- **Most gained**: `q_prop_write` 0.523 (`obj.a = j` four times a loop: each `put_field` copies its object and value), `v8_deltablue` 0.860, `v8_richards` 0.853, `q_func_closure_call` 0.880, `q_func_call` 0.936, `q_prop_create` 0.940, `v8_crypto` 0.941, `v8_earley_boyer` 0.951, `q_array_push` 0.957, `v8_navier_stokes` 0.963, `q_regexp_ascii` 0.970. `q_prop_update` took 0.834 with the body at 16 and 0.995 at 40: its time follows the place of the body.
- **Slower**: `q_string_build2` 1.061 (`r += "x"` on an argument, 0.93% more instructions, and 1.04 to 1.35 at the other places of the body), and `c_closures` 1.021 with 0.5% more instructions, under the 3% below which the rule counts instructions.

Instructions, `JS_CallInternal` holding all but 0.1% of the change: `r_react_ssr` +0.71%, `r_bench_table` +0.62%, `r_bench_spiral` +0.76%, `r_shiki_regexp` +0.47%, `q_func_call` +1.4%, `v8_richards` +1.6%, `v8_deltablue` +1.6%, `q_int_arith` +1.8%, `q_prop_write` +5.3%, `c_json_parse` +0.03%. A copy is four instructions in place of two, and the place of the body adds 40 bytes of no-ops, once per call.

The place of the body: the 8 places on 14 scripts, the engine of this change against main with its body at 0. The first column is the geometric mean of the 4 render scripts, the second of all 14.

| Body at | Render | All 14 | `q_string_build2` |
| ---: | ---: | ---: | ---: |
| 0 | 1.003 | 0.934 | 1.243 |
| 8 | 0.993 | 0.947 | 1.354 |
| 16 | 0.983 | 0.919 | 1.040 |
| 24 | 1.002 | 0.940 | 1.153 |
| 32 | 0.986 | 0.932 | 1.148 |
| 40 | 0.977 | 0.924 | 1.085 |
| 48 | 0.990 | 0.921 | 1.063 |
| 56 | 0.992 | 0.930 | 1.165 |

At 0, where main has it, `r_react_ssr` took 1.015 of main's time. 16 and 40 were run on all 37 scripts: 16 gave 0.957 and three slower scripts, 40 gave 0.954 and two, with the render scripts 0.967 to 1.004 and 0.977 to 0.985 (4 rounds, 2 positions). 40 is the default.

At the 8 positions of the rule, which move the body from 40 to 48, 56 and 0 past its place, over 8 scripts and 4 rounds: geometric mean 0.959, `v8_richards` 0.861, `q_func_call` 0.936, `c_string_concat` 0.974, `r_bench_spiral` 0.978 (8 of 8), `r_bench_table` 0.978, `r_react_ssr` 0.988 (6 of 8), `r_shiki_regexp` 0.992, and `q_int_arith` 0.974 within its own range of 1.14. The two positions that put the body at 0 are the two where `r_react_ssr` was slower than main, 1.037 and 1.029: with this code 0 is a bad place for the interpreter.

Findings on the way:

- **`q_int_arith` is not bound by the forwards.** A trace of the generated engine finds 525 loads of the kind in 69.6 million instructions where main has 301,167 in 68.4 million (`put_loc2` once a turn), and its time is the same within its range of 1.05 to 1.11. What binds its 77 cycles a turn is not found. [The polls](#the-c-engine-after-the-property-layer-cut-and-the-polls) blamed these forwards for it, which this shows wrong.
- **The combinations that were not kept**, 8 positions (or 4) on the scripts above, the geometric mean of 37 scripts at 4 positions against main: the feature on all files, the libc's included, 0.952, with `r_shiki_regexp` 1.033 (8 of 8) and 29 SSE moves left in `JS_CallInternal`. The feature on all files but the libc, 0.963, `r_shiki_regexp` 1.029. The feature on `quickjs.c` alone, `r_shiki_regexp` 1.028. With the vectorizer off, on `quickjs.c` alone or on every file but the libc, `r_shiki_regexp` is 0.992 to 1.004. The vectorizer off without the feature leaves 201 SSE moves and was timed on `q_int_arith` only (608 ms against 601).
- **Cycles were not counted.** The trace counts loads, and the VM does not expose the PMU (`perf stat` has no cycles), so what the 1.25 million loads of the render cost in cycles is not known. The time of `q_prop_write` says what they cost where nothing else runs: half of it.
- **What still fails**: in `c_string_concat` 112,740 loads of 4 bytes in `memcpy` over bytes the string code wrote one at a time, and in a render 12,315 loads of 4 bytes of `JS_NewObjectFromShape` over two stores of 2 bytes. Neither holds a JSValue.
- **Spread of `q_int_arith`**: a process runs it at about 510 to 550 ms or at about 590 to 640, and the VMs of one process (each run is a new VM) range 512 to 660 ms. With address randomization off none of 16 processes ran fast, against 4 of 16 with it on, for main, so the addresses the kernel hands out count. A build with the change has the fast mode too (1 of 12 processes), so it is not the change. The cause is not found.


### Strings built by appending

A string appended to a variable is [copied with room](changes/c-engine.md#strings-built-by-appending) instead of at each append. Against the engine before it, over 3 rounds at 8 positions of all 37 scripts. Time is the median over positions of the paired ratios, and range is the base's own range over positions:

| Script | Instructions | Time | Range |
| --- | ---: | ---: | ---: |
| `r_react_ssr` | -8.6% | 0.933 (0.910 to 0.944) | 3.0% |
| `c_string_concat` | -17.5% | 0.882 (0.839 to 0.900) | 3.1% |
| `q_string_build2` (`+=` on an argument) | -35.7% | 0.698 (0.658 to 0.839) | 3.4% |
| `q_string_build1` (`+=` on a local) | -1.1% | 0.948, within its range | 18.7% |
| `c_escape_html` | +0.17% | 1.010, within its range | 8.0% |
| `v8_splay` | +0.16% | 1.000 | 3.6% |
| `r_bench_table`, `r_bench_spiral` | +0.004%, +0.017% | 1.000, 1.000 | 2.2%, 2.6% |

The other 29 scripts run within 0.08% of the instructions (`c_string_methods` +0.076%, the rest within 0.01%), and no time passes the rule. `c_json_stringify` took 0.972 at all 8 positions with the same instructions, which is the layout, not the change. A string `+` pays about 2 instructions for the checks (`v8_splay`, `c_escape_html`).

Memory of `r_react_ssr`: the most bytes in use at once went from 2.39 MB to 2.46 MB (+2.9%, the room of the page string). That made the heap grow by one more chunk, a quarter of what it had: the footprint went from 3.01 MB to 3.80 MB, and the most memory the process held from 8.05 to 8.31 MB to 8.38 to 8.50 MB. `c_string_concat` uses 35% less at its most.

### Ordinary objects of 48 bytes

An ordinary object takes a [block of 48 bytes, not 72](changes/objects-and-shapes.md#ordinary-objects-take-48-bytes-not-72). It makes no page faster or slower: the engine runs between 0.07% fewer and 0.27% more instructions on the pages below, and no time passes the rule. Against main (01c3da62) at 8 positions, 5 rounds (the shop 3), one core of an EPYC 7B13 with its sibling idle. Time is the median over positions of the paired ratios, range the base's own range over positions, and instructions per render are from callgrind at position 0.0 (the difference of a run of 12 renders and one of 2).

| Script or page | Time | Range | Instructions |
| --- | ---: | ---: | ---: |
| `r_react_ssr` | 1.001 | 1.1% | +0.01% |
| bench app, spiral | 0.993 | 6.0% | +0.15% |
| bench app, table | 1.000 | 6.3% | +0.20% |
| MUI app, home | 1.003 | 2.6% | +0.22% |
| MUI app, order | 1.004 | 2.7% | not counted |
| MUI app, orders | 1.002 | 2.5% | +0.27% |
| shop, six workbench pages (sum) | 0.998 | 2.4% | studio list +0.04% |
| shop, store products page | not timed | | -0.07% |

No page passes the rule for faster or slower. The shop was timed with 3 rounds of 60 renders a page (every page 0.997 to 0.999). The instruction counts of the shop and MUI pages come from a build with the render time limit raised, since valgrind runs about 50 times slower than the limit allows.

Memory, on the C tree natively (Apple M4), against main with the same HTML:

| What | Before | After | Change |
| --- | ---: | ---: | ---: |
| Highest `MemorySize`, 11 Markdown pages of 300 to 500 KB, mean of the ratios | | | -3.8% (-11.8% to +2.7%) |
| Highest `MemorySize`, the page of 400 KB | 180.8 MB | 181.1 MB | +0.2% |
| End of that render, collector turned off | 394.3 MB | 379.9 MB | -3.7% |
| Highest `MemorySize`, the 20 pages of tifa's shop, sum | 253.5 MB | 240.9 MB | -5.0% |
| End of the render of the 20 pages, collector turned off, sum | 244.3 MB | 240.0 MB | -1.8% |

The highest count follows the threshold of the collector (1.5 times the count after the last collection), so one page can peak just before or after a collection: the average of a run of sizes, or a render with the collector off, shows what the objects saved. The count at the end of a render with the collector on moves with the same threshold (the page of 400 KB ends at 174.8 MB, then 177.2 MB, with the collector at another point of its cycle).

### What a new VM and loading a bundle cost

On linux/amd64 (AMD EPYC 7B13, one core, its sibling idle), a new VM takes 242 µs. 15 µs of it is the host: the 32 MiB shadow stack, reserved by `mmap`. The rest is `JS_NewRuntime` and `JS_NewContext` (224 µs), which build the built-in objects of a VM. Closing a VM runs a full collection (`JS_FreeRuntime`), which is 30% of the instructions of a new VM and its close together.

Instructions of the engine in a new VM (callgrind, 764 K, with 30 K more in Go), by what it does:

| What | Instructions |
| --- | ---: |
| `JS_NewContext` (every built-in object and function) | 671 K |
| of which `JS_SetPropertyFunctionList`, defining the functions and properties of the built-ins | 486 K |
| `JS_NewAtomLen`, atoms made from C strings (357 calls a VM) | 274 K |
| `JS_NewRuntime` (the atom table, the classes) | 110 K |
| `snprintf`, for the names of getters and setters (43 calls) | 24 K |
| `JS_SetShapeCache` | 25 K |

The built-ins are 164 objects and 515 atoms, 174 KB, and make up 2% of a shop VM. Loading a bundle is the rest. For a new VM and `EvalSharedBytecode` of the React bundle (11.05 M instructions with the close), reading the bytecode is 4.7 M, of which the scan of the polls was 2.5 M (see [shared bytecode](changes/memory-and-allocation.md#shared-bytecode-is-scanned-for-the-polls-on-its-first-load-only)), running the top level 3.5 M and the close 2.0 M.

A VM of tifa's shop holds 8.47 MB after loading (`TestShopVMMemory`): 1.48 MB the functions (7,149 of them), 0.72 MB of that their bytecode, 0.82 MB atoms, 0.80 MB objects, 0.73 MB properties, 0.65 MB shapes, 0.31 MB strings and the rest the 8-byte headers and rounding of 2,600 blocks. The RSS of a VM in tifa's pool is 8.6 MiB after loading, 12.8 after rendering and 9.6 once the pool has given its free memory back, close to what the engine counts. The 174 KB of built-ins is the only part that every VM has whatever it loads.

### Object literals made at their final shape

An object literal of plain keys [gets all its keys at once](changes/objects-and-shapes.md#object-literals-made-at-their-final-shape). Against main (dfd2caee, with the ordinary objects of 48 bytes), over 4 rounds at 8 positions on one core of an EPYC 7B13 with its sibling idle, with the same HTML. Time is the median over positions of the paired ratios, and instructions per render are from callgrind at position 0.0.

| Pages | Time | Positions | Base range | Instructions |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, six workbench pages (sum) | 0.956 | 0.935 to 0.963 | 1.8% | not counted |
| tifa's MUI test app, three pages (sum) | 0.969 | 0.948 to 0.976 | 2.6% | orders -3.8% |
| tifa's bench app, table | 0.942 | 0.914 to 0.950 | 3.6% | not counted |
| tifa's bench app, spiral | 0.989, no change by the rule | 0.968 to 1.005 | 2.2% | -3.5% |
| `r_react_ssr` | 0.991, no change by the rule | 0.954 to 1.023 | 4.8% | -3.1% |

Every position of the shop and MUI pages was faster, and so was every position of the bench table. The same comparison against the engine before the ordinary objects of 48 bytes gave 0.958, 0.971 and 0.949 for the three apps. The instructions per render are the difference of a run of 4 renders and one of 1 for the MUI order list (440.3 M, then 423.6 M) and the spiral page of the bench app (185.9 M, then 179.4 M), and of 3 and 1 for `r_react_ssr`. Callgrind stopped on most runs of the shop test binary (an assertion of callgrind on a signal, at the exit of the Go program, on both engines), so the shop was not counted: its time passes the rule.

On the Apple M4, in the native C tree, the first render of a markdown page of 400 KB made 1.39 million objects with `OP_object` and 3.76 million keys with `OP_define_field`, 80 ms in all. With the change, 1.05 million of those objects are `OP_object_lit`, 341 thousand are still `OP_object` and 66 thousand keys are still defined one by one. A timer around `OP_object_lit` reads 166 ms, but nearly all of it is the garbage collection: a collection starts at the creation of an object, and now mostly inside `OP_object_lit` (164 ms of it in a build that times the collections apart). Leaving the collection out, the three opcodes take about 20 ms. A build that times only the collections, with no timer in any opcode, gives the same: over 6 renders each the page takes 1583 ms rather than 1639 (every render faster), the 9 collections take 250 ms rather than 243, and the rest 1334 ms rather than 1396. Over 20 timed renders of each of the 20 pages of the shop, the three opcodes take 125 ms rather than 250.

Memory, on the C tree natively (Apple M4), against main with the same HTML:

| What | Before | After | Change |
| --- | ---: | ---: | ---: |
| Highest `MemorySize`, 11 Markdown pages of 300 to 500 KB, mean of the ratios | | | -1.6% (-3.8% to -0.6%) |
| `MemorySize` at the end of the render, the 9 of those pages with no collection near the end, mean of the ratios | | | -1.1% |
| Highest `MemorySize`, the 20 pages of tifa's shop, sum | 216.1 MiB | 219.5 MiB | +1.6% |
| End of the first render of the 20 pages, sum | 206.6 MiB | 210.0 MiB | +1.6% |

A VM of the shop holds about 0.17 MiB more after its first render, for the keys of its 5088 literals and their shapes, which now live as long as their functions. A long Markdown page takes less, as its objects no longer have spare slots. The highest memory of a process of `r_react_ssr` goes from 8.0 to 8.3 MiB (median of 32 processes).

### Object literals with their properties in the block of the object

An object literal [keeps its properties in the block of the object](changes/objects-and-shapes.md#object-literals-keep-their-properties-in-the-block-of-the-object). It saves 0.1% to 0.8% of the instructions of a render and leaves the time of every page inside what the rule counts as no change. Against main (a9d61185), over 2 rounds at 8 positions on one core of an EPYC 7B13 with its sibling idle, with the same HTML. Time is the median over positions of the paired ratios and range the base's own range over positions. Instructions per render are from callgrind at position 0.0 (the difference of two runs with different numbers of renders).

| Pages | Time | Range | Instructions |
| --- | ---: | ---: | ---: |
| tifa's shop, six workbench pages (sum) | 0.991 | 2.4% | -0.14% |
| tifa's MUI test app, three pages (sum) | 0.998 | 2.2% | -0.19% |
| tifa's bench app, table and spiral (sum) | 0.985 | 2.6% | -0.82% |
| `r_react_ssr` | 0.995 | 4.0% | -0.43% |

No group of pages is slower. The bench app and React render 1.5% and 0.5% faster, under the 2% the rule asks. The version before the flag `inline_grew` (the first version of this change) ran 0.26%, 0.21%, 0.83% and 0.44% fewer instructions, and took 0.997 (a range of 12%, as other work shared the machine), 0.981, 0.989 and 0.978 of the time.

Memory of one VM of tifa's pages (the engine's `MallocSize`, from a VM made by `ssr.Load`), against main, with the same HTML:

| What | shop | MUI | bench | store pages |
| --- | ---: | ---: | ---: | ---: |
| After loading | -0.2% | -0.2% | -0.3% | -0.2% |
| After 96 renders and a full collection | -1.5% | -2.4% | -2.0% | -1.0% |
| Mean at the end of a render, collector on | -0.7% | -0.8% | -0.7% | -1.8% |

With the collector off, so that nothing the pages leave is collected, the memory one render adds goes from -2.8% to +4.5% from page to page (mean +0.7%). A reason that fits, not shown: objects of 48 to 130 bytes now fall in as many size classes as there are numbers of properties, and a free block of the front cache that one class leaves is not used by another. The memory the process holds at the most for `r_react_ssr` is 8.5 MiB, then 8.4.

### Calls, for in and hasOwnProperty.call

Two changes, measured one on top of the other: [a call sets and frees its values in fewer loop turns](changes/c-engine.md#how-a-call-sets-and-frees-its-values), then [a for in loop holds the shape of its object](changes/objects-and-shapes.md#a-for-in-loop-holds-the-shape-of-its-object) together with [hasOwnProperty.call answered in the interpreter](changes/objects-and-shapes.md#hasownpropertycall-answered-in-the-interpreter). Against main (9fd4bdb0, with object literals made at their final shape), over 4 rounds at 8 positions on one core of an EPYC 7B13 with its sibling idle, with the same HTML. Time is the median over positions of the paired ratios, and positions gives the lowest and the highest of them. Instructions per render are from callgrind at position 0.0: the difference of 12 and 4 renders of each page for the shop, of 3 and 1 for the MUI pages and the scripts. The bench pages are counted on `r_bench_table` and `r_bench_spiral`, which render the same pages.

The calls, against main:

| Pages | Time | Positions | Base range | Instructions |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, six workbench pages (sum) | 0.996, no change by the rule | 0.991 to 1.001 | 2.0% | -2.5% |
| tifa's MUI test app, three pages (sum) | 0.999, no change by the rule | 0.991 to 1.003 | 1.7% | -1.8% |
| tifa's bench app, table | 0.958 | 0.939 to 0.989 | 2.5% | -4.8% |
| tifa's bench app, spiral | 0.967 | 0.958 to 0.997 | 2.4% | -3.2% |
| `r_react_ssr` | 0.984, no change by the rule | 0.975 to 1.005 | 2.7% | -3.1% |

The for in loops and hasOwnProperty.call, against the calls:

| Pages | Time | Positions | Base range | Instructions |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, six workbench pages (sum) | 0.984, no change by the rule | 0.975 to 0.996 | 2.0% | -2.5% |
| tifa's MUI test app, three pages (sum) | 0.981, no change by the rule | 0.971 to 0.995 | 1.9% | -3.4% |
| tifa's bench app, table | 0.996, no change by the rule | 0.968 to 1.011 | 2.4% | -1.1% |
| tifa's bench app, spiral | 0.977, no change by the rule | 0.951 to 0.987 | 2.0% | -2.8% |
| `r_react_ssr` | 0.988, no change by the rule | 0.956 to 1.012 | 3.9% | -1.8% |

Every position of the shop and the MUI pages was faster with the second change. Both changes against main: the shop 0.982 (0.967 to 0.983, 5.0% fewer instructions), the MUI pages 0.976 (0.966 to 0.995), the bench table 0.955 (0.948 to 0.966), the bench spiral 0.950 (0.938 to 0.956) and `r_react_ssr` 0.969 (0.956 to 0.991), each at every position. No page is slower. The one script found slower is `q_func_call`, a loop of calls to a function with no locals: 3.0% more instructions with the first change, for the tests of the two and the one locals left over.

The memory of a VM does not change: `MemorySize` and the footprint at the end of `r_react_ssr` and `r_bench_table` are the same bytes before and after (Apple M4). On linux/amd64 the heap of the engine grows the same 13 times to the same 12 chunks. The most memory of a `jsrun` process of `r_react_ssr`, which the Go runtime and the code pages add to, is 8.3 MiB at the median of 32 processes before the changes and 8.4 MiB after.

### C functions called from the interpreter

[The interpreter calls a plain C function itself](changes/c-engine.md#how-the-interpreter-calls-a-c-function), without the entry of `JS_CallInternal`, and counts the frame it skips as taken on the stack. Against main (a9d61185), over 4 rounds at 8 positions on one core of an EPYC 7B13 with its sibling idle, with the same HTML on the 16 pages of the shop, the MUI app, the bench app and the store. Time is the median over positions of the paired ratios, and positions gives the lowest and the highest of them. Instructions per render are from callgrind at position 0.0: the difference of 5 and 2 renders of each page for the tifa pages, of 3 and 1 runs for `r_react_ssr`.

| Pages | Time | Positions | Base range | Instructions |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, six workbench pages (sum) | 0.984, no change by the rule | 0.971 to 1.003 | 2.0% | -0.74% |
| tifa's MUI test app, three pages (sum) | 0.983, no change by the rule | 0.965 to 0.988 | 2.7% | -0.34% |
| tifa's bench app, two pages (sum) | 0.987, no change by the rule | 0.961 to 1.010 | 2.9% | -2.32% |
| `r_react_ssr` | 0.980, no change by the rule | 0.950 to 1.015 | 3.2% | -0.87% |

No page is slower. Every position of the MUI pages was faster. The memory of a `jsrun` process of `r_react_ssr` is 8.4 MiB at the median of 32 processes before and 8.2 MiB after. Without counting the skipped frame as taken the change ran fewer instructions (-0.84%, -0.60%, -2.66% and -1.11%), but let a recursion with big frames through a C function go deeper than before.

### The end of a call and the start of one

The first change is [the end of a call skipping the values with no reference count](changes/c-engine.md#the-end-of-a-call-skips-the-values-with-no-reference-count). Against main (14271f3f, v0.29.3), over 5 rounds at the 8 positions on one core of an EPYC 7B13 with its sibling idle, with the same HTML. Time is the median over positions of the paired ratios of the P50 of a page (100 renders after 16 warm ones), and positions gives the lowest and the highest of them. Instructions are from callgrind at position 0.0: the difference of 4 rounds and 1 of the pages of an app in one VM, of 3 runs and 1 of the scripts. The pages are those of tifa's bench app, the only ones whose instructions fall by 2% or more.

| Pages | Time | Positions | Base range | Instructions |
| --- | ---: | ---: | ---: | ---: |
| tifa's bench app, table | 0.962 | 0.939 to 0.983 | 3.9% | -2.04% (`r_bench_table`) |
| tifa's bench app, spiral | 0.973, under 3% so see callgrind | 0.960 to 0.999 | 4.8% | -1.29% (`r_bench_spiral`) |
| tifa's bench app, both (sum) | 0.968 | 0.948 to 0.992 | 3.5% | -2.10% |

Every page was faster at all 8 positions. The other pages fall by less than 2% and are not timed: the six workbench pages of the shop -1.07%, its five storefront pages -0.71%, the three MUI pages -0.76% and `r_react_ssr` -1.29%. Memory does not change: the end of a call frees the same values.

The second change is [a call setting its locals eight to a turn](changes/c-engine.md#a-call-sets-its-locals-eight-to-a-turn). Against the first, counted the same way at position 0.0, it runs 0.35% fewer instructions on the shop's workbench pages, 0.16% on its storefront pages, 0.18% on the MUI pages, 0.54% on the bench pages, 0.32% on `r_react_ssr`, 0.52% on `r_bench_table` and 0.31% on `r_bench_spiral`. None is over the 2% for which the rule asks for a timing. The locals of a call of a function with 200 locals cost 6.3 instructions each, from 11.6 before the first change and 7.3 after it.

### React DOM's text escape in C

[`VM.RegisterEscapeText`](changes/go-api.md#registerescapetext) gives a host React DOM's `escapeTextForBrowser` as a C function of the engine. tifa registers it as `__tifa_escape`, and its Vite plugin makes React's string renderer call it where the renderer called its JavaScript function, which tifa had given a cache of escapes and five `replaceAll` calls in place of React's loop. The step before is liustack/quickjs main (4cdd3a27) with tifa main (363b6b1). The step after is this branch with the tifa branch that uses it. The HTML of the 16 pages (the six workbench pages, the five storefront pages, the three MUI pages and the two bench pages) is the same byte for byte, with the render time stamp masked.

On linux/amd64 (one core of an EPYC 7B13, its sibling idle), 3 rounds at the 8 code positions. Time is the sum of the p50 of the pages of an app, as the median over positions of the paired ratios. Instructions are callgrind's, per render of every page, at position 0.0 (the difference of a run of 5 renders and one of 2):

| Pages | Time | Positions | Base range | Instructions |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, six workbench pages (sum) | 0.974 | 0.970 to 0.981 | 2.5% | -2.47% |
| tifa's shop, five storefront pages (sum) | 0.975 | 0.957 to 0.979 | 2.5% | -2.41% |
| tifa's MUI test app, three pages (sum) | not timed | | | -0.71% |
| tifa's bench app, table and spiral (sum) | 0.938 | 0.932 to 0.947 | 1.8% | -4.50% |
| `r_react_ssr`, which does not register the function | 1.000 | 0.991 to 1.006 | 4.1% | not counted |

`r_react_ssr` took 360.2 ms on main and 361.4 ms on the branch (5 rounds of 15 runs), and 14 of its 80 processes ran with other work on the machine after all their tries. After the branch was rebased on main at 767d3fd4 (the fixes of the GC threshold, of spread copies and of long loops), the instructions of the shop's six workbench pages were counted again, with both tifa changes (this one and the attribute rewrite) against tifa main on 767d3fd4: 979.8 M to 948.5 M, 3.19% fewer, where the same pair on 4cdd3a27 gave 3.25% fewer. The shop, store and bench pages were faster at all 8 positions. Under 3%, the shop and store timings are read with the instruction counts beside them, and both agree. The MUI pages use few characters that need escaping, and their saving is under the 2% that is worth timing. On the Apple M4 (darwin/arm64, one binary, 5 rounds in turn) the shop's workbench pages take 0.967 of the time, the storefront pages 0.969, the MUI pages 0.993 and the bench pages 0.950.

One call, from `BenchmarkEscapeText` (ns a call on the same machine, net of a loop that calls the empty function, which takes 38 ns). The first column is React's own function, the second tifa's cache and `replaceAll` for a text the cache holds (the best case for it), the third the C function:

| Text | React | tifa cache | C |
| --- | ---: | ---: | ---: |
| 9 characters, nothing to escape | 192 | 119 | 4 |
| 30 characters, a class name with 2 to escape | 3,590 | 340 | 70 |
| 56 characters, nothing to escape | 263 | 195 | 22 |
| 60 characters, 6 to escape | 5,267 | 402 | 141 |
| the number 1234.5 | 259 | 309 | 179 |
| 512 characters, nothing to escape | 957 | 850 | 217 |
| 512 characters, 171 to escape | 70,101 | 899 | 2,061 |
| 4096 characters, nothing to escape | 6,323 | 6,028 | 1,673 |

The call of the C function costs about what a call of an empty JavaScript function costs (4 ns more for a text of 9 characters), which is why a short text is a gain at once. For the texts a page escapes most, short ones, a lookup in the cache takes longer than escaping the text in C, so tifa goes without a cache in front of the function. A long text with many characters to escape that the cache holds is the one row where the cache wins, and the C loop for such a text costs about 4 ns a character. A Go `Builtin` could not do this: a call costs 22 ns more than a C function, and the copies of the string in and out cost more than the search and the cache saved in tifa (1.3% to 2.0% slower on the shop).

Memory does not change that can be told from the heap's own spread: the live heap after 768 renders (Apple M4, after a full GC) is 0.6% lower on the shop's pages, 0.4% on the storefront, 0.5% on the MUI pages and 0.2% higher on the bench pages. The footprint moves by a chunk with the moment of the collection in both builds.

### The copy of the props of a React element in C

[`VM.RegisterCopyProps`](changes/go-api.md#registercopyprops) gives a host the loop with which React's `createElement` and `cloneElement` copy the keys of their config into the props of an element, as a C function of the engine. tifa registers it as `__tifa_copy_props`, and its Vite plugin makes both functions ask it before their loop. The step before is liustack/quickjs main (a5feface) with tifa main (4b511fc). The step after is this branch with the tifa branch that uses it. The HTML of the 16 pages (the six workbench pages, the five storefront pages, the three MUI pages and the two bench pages) is the same byte for byte, with the render time stamp masked.

How often it is asked, from one render of each page in tifa with a wrapper that counted its answers: the six workbench pages make 980 elements with `createElement` and 518 with `cloneElement`, the five storefront pages 2,507 and 328, the three MUI pages 3,266 and 114, and each bench page 1. The function copies all of them but one config of 39 keys (MUI's input, on the order list and on an order page), which is over the 32 keys it takes and goes through the loop.

On linux/amd64 (one core of an EPYC 7B13, its sibling idle), 3 rounds at the 8 code positions. Time is the sum of the p50 of the pages of an app, as the median over positions of the paired ratios. Instructions are callgrind's, per render of every page, at position 0.0 (the difference of a run of 5 renders and one of 2). "Same engine, old bundle" is a control: the engine of this branch with the bundle tifa main builds, which never asks for the function:

| Pages | Time | Positions | Base range | Same engine, old bundle | Instructions |
| --- | ---: | ---: | ---: | ---: | ---: |
| tifa's shop, six workbench pages (sum) | 0.988 | 0.971 to 1.010 | 2.9% | 1.004 | -2.85% (948.0 M to 921.0 M) |
| tifa's shop, five storefront pages (sum) | 0.989 | 0.982 to 1.003 | 3.9% | 1.011 | -2.06% (1,897.8 M to 1,858.7 M) |
| tifa's MUI test app, three pages (sum) | 0.980 | 0.966 to 0.993 | 2.7% | 1.007 | -4.13% (746.4 M to 715.6 M) |
| tifa's bench app, table and spiral (sum) | 1.014 | 0.999 to 1.035 | 2.1% | 1.022 | +0.04% (293.39 M to 293.50 M) |
| `r_react_ssr`, which does not register the function | 1.003 | 0.994 to 1.007 | 2.5% | | not counted |

By the rule for code positions none of the four times changes: each is under 2% or inside half the spread of the base's own positions (the MUI pages, at 2.0%, are on the line). The instruction counts do not depend on where the code lands, and they say the shop, the storefront and the MUI pages run 2.1% to 4.1% less, and the bench pages, which make one element each and cannot gain, the same. The control shows why the times gain less than the instructions: the engine of the branch with the old bundle takes 0.4% to 2.2% more time than main on the same instructions, which is where the code of the engine landed once it has one more function (the positions of the rule move the program and the interpreter, not the functions between). It also is why the bench pages show +1.4%. `r_react_ssr`, a React render that never asks for the function, was timed on the first build of the branch (the field of the context that holds the atoms of the function was in the middle of `JSContext` then, and is last now).

One call, from `BenchmarkCopyProps` (ns for one `createElement` or `cloneElement` with a new config each call, as JSX makes it, the literal of the config and the loop included, 5 counts of 300,000 calls). No config is slower with the function than with React's loop, and the empty one takes 4% to 5% less. The code tifa's plugin writes for a VM that has no function runs within 4% of React's own:

| Config | createElement React | with the function | cloneElement React | with the function |
| --- | ---: | ---: | ---: | ---: |
| none | 580 | 556 | 803 | 760 |
| 1 key | 869 | 608 | 1,085 | 822 |
| `key` and 1 key | 1,044 | 717 | 1,298 | 972 |
| `key` and 2 keys | 1,226 | 745 | 1,505 | 1,007 |
| 3 keys | 1,249 | 639 | 1,524 | 898 |
| 5 keys | 1,638 | 663 | 1,934 | 953 |
| 10 keys | 2,545 | 716 | 2,967 | 1,108 |

React's loop takes about 200 ns for each key in these runs ((2,545 - 580) / 10 for `createElement`), and the function about 16 ns for each key it adds ((716 - 556) / 10). The call with its checks of the config, `Object.prototype`, `hasOwnProperty` and `call` costs less than the setup of the empty `for in` it replaces. On the Apple M4 the ratios are the same: 0.96 for no keys, 0.75 for one key, 0.50 for 3 keys and 0.24 for 10 keys, against React's `createElement`.

A check in JavaScript in front of one spread was measured first, as the step before this one (see [What was tried and not done](#what-was-tried-and-not-done)).

### React DOM's start and end tags in C

[`VM.RegisterStartTag` and `VM.RegisterEndTag`](changes/go-api.md#registerstarttag-and-registerendtag) give a host React DOM's `pushStartGenericElement` (with `pushAttribute`, `pushStyleAttribute` and the escape they call) and `endChunkForTag` as C functions of the engine. They are measured on the calls a render makes, recorded and played back, since tifa's wiring is not part of this change.

The calls were recorded from one render of the pages of tifa's example apps on the native test host, with a wrapper around `pushStartGenericElement` in the bundle that saved its arguments: 9,682 calls on 16 pages of the shop (14 storefront pages and 2 workbench pages), 1,227 on the three MUI pages and 5,401 on the two bench pages. A Go program plays them back in their order on one warm VM, three ways: React's functions as the bundle has them (`js`), the C start tag with the C end tag (`c`), and a loop that only makes the arguments and calls an empty function (`loop`). Each tag is passed as the string of an atom, the same string at each call, as JSX gives it. Every call gives the same text as React's functions, and none declines.

Instructions are callgrind's on linux/amd64, of the functions of the engine only (the Go runtime left out), with no collection during the counted passes, for one element, a start tag and its end tag:

| Pages | loop | js | c | js net of loop / c net of loop |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, storefront and workbench | 991 | 19,265 | 4,326 | 5.48 |
| tifa's MUI test app | 994 | 17,884 | 3,887 | 5.84 |
| tifa's bench app | 993 | 15,118 | 3,402 | 5.86 |

Time on the Apple M4 (darwin/arm64, ns for one element, the median of 21 rounds):

| Pages | loop | js | c | js net of loop / c net of loop |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, storefront and workbench | 33.7 | 978.0 | 200.7 | 5.65 |
| tifa's MUI test app | 33.0 | 890.7 | 175.1 | 6.03 |
| tifa's bench app | 32.8 | 720.2 | 146.8 | 6.03 |

The C functions take about a fifth of the instructions and of the time of React's. Of the instructions of `c`, the escape of the text and the attributes takes about 800, and the checks that the target, `hasOwnProperty`, `call` and `Array.prototype.push` are the built-in ones about 100. The tables of names and end tags grow from 16 entries at the first call and never let two names take each other's place: a table with one place for each name (512 names and 256 end tags, made with the first function) takes 32 KB a VM, and on tifa's shop pages it makes again about 32 names and 6 end tags a render that land on the place of another. With the tables, a VM of tifa's shop holds 1.6 KB more than one without the functions once the bundle is loaded (the shop's `TestShopVMMemory` in tifa). After 64 renders it holds 26 KB more in tifa's pool (a young collection after each render) and 10 KB more after a full collection, with the same live size: arenas of 4 KB in which the start tags of a render, which live until it is written out, sat next to long-lived objects the render made. When the start tag kept the buffer it wrote in as its string, at least 160 bytes for any tag and all of them in one class, these were 107 KB and 70 KB. The growing tables took 0.6% to 1.6% more instructions than the fixed table in this playback, which has no names that take the same place. Writing into the buffer the cache keeps and copying the text into a string of its length takes 1.5% to 2.6% fewer instructions than a buffer of its own for each call, kept as the string (`c` was 4,443, 3,969 and 3,453), and on the M4, run in turns with that build, 1% to 3.5% less time.

A profile of the render on linux/amd64 puts the share of the time spent in these React functions at 10.4% on the workbench pages, 9.7% on the storefront pages, 4.3% on the MUI pages and 29.4% on the bench pages. tifa measured what it gains once its React DOM calls the functions, on v0.29.10's engine: the same tifa on the same engine with and without the calls, on the machine of [How the numbers were measured](#how-the-numbers-were-measured), with `measure bench run` on cpu 0 with 4 idle, GOMAXPROCS=1, 2 rounds of 3 reps and 16 layouts (18 units of each kind, none run again), and judged with a delta of 1%. The time is the p50 of a page summed over the pages of a kind, with 95% intervals:

| Pages | At the formal layout | Mean of the layouts | Page that gains least | Instructions (callgrind) |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, workbench | -7.30% [-8.11, -6.48] | -7.35% [-7.75, -6.95] | -4.56% | -10.10% |
| tifa's shop, storefront | -6.98% [-7.87, -6.09] | -6.69% [-7.16, -6.22] | -1.68% | -9.55% |
| tifa's MUI test app | -3.76% [-4.57, -2.95] | -3.71% [-4.11, -3.32] | -3.47% | -5.67% |
| tifa's bench app | -33.02% [-33.60, -32.44] | -32.33% [-32.66, -32.00] | -20.08% | -31.88% |

The instructions compare v0.29.9 without the calls to v0.29.10 with them, one run of each. Against v0.29.9, the same tifa on v0.29.10 without the calls changes by no more than 1% on each kind, except the bench pages at the formal layout, +1.61% [+0.49, +2.75], which the mean of the layouts does not show (+0.20% [-0.62, +1.02]). The HTML of the pages is the same byte for byte.

### Object literals that share their shape

[Object literals of the same keys share one shape](changes/objects-and-shapes.md#object-literals-of-the-same-keys-share-one-shape). Against v0.29.4 (c2e07b7d), with the same HTML on the 16 pages. Memory is the engine's `MallocSize` of one VM made by `ssr.Load`, after one render of each page of the app and after 120, with a full collection. Instructions per render of every page of an app are from callgrind at position 0.0 (the difference of 3 renders and 1).

| Pages | Shapes | Memory after 1 render | After 120 | Instructions |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, six workbench pages | 4203 to 3615 | -0.66% | -0.54% | -0.13% |
| the shop's store pages | 5210 to 4403 | -1.67% | -2.10% | -0.03% |
| tifa's MUI test app, three pages | 3896 to 3425 | -0.69% | +0.70% | +0.01% |
| tifa's bench app, two pages | 940 to 928 | 0 | 0 | +0.02% |
| `r_react_ssr` | | | | +0.03% |

The MUI pages after 120 renders hold 41 KB more, where after one render they hold 41 KB less: the shapes are as many as after one render, and the difference is in what the collector and the allocator keep at that moment. The instructions do not change, so the time was not taken.

### Opcodes that do a few instructions at once

[A read of a local and its get_field, the case of a switch on a string](changes/c-engine.md#a-read-of-a-local-and-its-get_field-and-the-case-of-a-switch-on-a-string) and [typeof tests](changes/c-engine.md#typeof-as-one-opcode) take one opcode. Each change against the one before it, starting from the shapes above, with the same HTML on the 16 pages. Instructions per render of every page of an app are from callgrind at position 0.0 (the difference of 3 renders and 1, of 3 and 1 runs for `r_react_ssr`).

| Pages | Fused reads and cases | typeof | A table for the compiler | All three |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, six workbench pages | -1.85% | -0.63% | | -2.81% |
| the shop's store pages | -1.59% | -0.83% | | -2.41% |
| tifa's MUI test app, three pages | -1.66% | -0.75% | | -2.28% |
| tifa's bench app, two pages | -3.41% | -1.42% | -0.03% | -4.82% |
| `r_react_ssr` | +2.07% | -0.08% | -0.47% | +1.46% |

A run of `r_react_ssr` compiles its bundle and then renders once, and the compile is most of its instructions. The compiler pays for the search of the runs: `fuse_code` took 1.45 M instructions of a run in its first form, and 0.98 M once it looks the opcodes up in a table (the third column). Each VM of tifa reads bytecode compiled once, so its renders do not pay for it. The pages render with fewer instructions on every app. Timed at 8 code positions, 3 rounds, the fused reads and cases with the shapes above against v0.29.4: the bench pages, the one app over 2% at a step, take 0.966 of the time (16.49 ms to 15.97 ms for the spiral page, 15.25 ms to 14.76 ms for the table page), and `r_react_ssr` 1.018 with the first form of `fuse_code`, which is under 3%, so callgrind decides. All three against v0.29.4, timed the same way: the bench pages take 0.943 of the time, the six workbench pages 0.976 and the store pages 0.980. Those two are under 3%, so callgrind decides for them, and the MUI pages were not timed.

The memory of a VM does not change: the fused opcodes take the place of the first instruction of their run, and a `typeof` test takes as many bytes as before or fewer.

On main after it took the reads of the interrupt flag in long string loops (bcd2eb1a), against that main, callgrind as above with the collector of Go off (`GOGC=off`): all three take 3.03% of the instructions off a render of the six workbench pages and 2.53% off the MUI pages, with the same HTML on the shop, store and MUI pages.

The chain of cases (`dup_case_atom` with the `if_true` jumps and a chain of cases in one dispatch, bytecode version 8), against main at 4551c4ab, with the same HTML on the pages of tifa's shop, MUI and bench apps, built by tifa's build now:

| Pages | Dispatches | Instructions | Time (16 positions, 95% interval) |
| --- | ---: | ---: | --- |
| React DOM, a render of the React page of `r_react_ssr` | -20.3% | -5.67% | -9.49% (-11.20% to -7.74%) |
| tifa's shop, six pages | -1.20% | -0.43% | +0.36% (-0.23% to +0.95%), 4 rounds |
| tifa's MUI test app, three pages | -1.10% | -0.43% | +0.51% (-0.16% to +1.19%), 4 rounds |
| tifa's bench app, two pages | -1.02% | -0.64% | +0.18% (-0.97% to +1.34%) |

Dispatches are counted per render, on the C tree for React DOM and on the generated engine for the apps. Instructions are callgrind on the generated engine on linux/amd64, around the renders alone (54.70 M to 51.60 M a render of the React page, 7.3% fewer in `JS_CallInternal`). Time is the mean of 16 code positions, 2 rounds (4 for the shop and MUI, whose first 2 rounds gave +0.25%, -0.53% to +1.04%, and +0.97%, +0.35% to +1.60%), on cpu 0 of the GCP machine with cpu 4 idle. The bundles of tifa's build guard the long switches of React DOM, so its pages run few chains of cases. React DOM as npm ships it runs 53K of its 83K string cases a render as cases that share a body.

### Checks of the temporal dead zone

[The checks that cannot fail](changes/c-engine.md#checks-of-the-temporal-dead-zone-that-cannot-fail) are dropped by the compiler, against main at 74e4ddd7. Instructions per render from callgrind at position 0.0 (the difference of 3 renders and 1), with the same HTML on the 16 pages:

| Pages | Instructions a render | Change |
| --- | ---: | ---: |
| tifa's shop, six workbench pages | 857.5 M to 852.3 M | -0.61% |
| the shop's store pages | 1,639.0 M to 1,633.4 M | -0.34% |
| tifa's MUI test app, three pages | 678.4 M to 673.1 M | -0.77% |
| tifa's bench app, two pages | 257.4 M to 257.2 M | -0.11% |
| a render of `r_react_ssr`, compiled once | 54.92 M to 54.88 M | -0.06% |
| a run of `r_react_ssr`, which compiles its bundle | 108.19 M to 108.87 M | +0.62% |

A round of the workbench pages dispatches 1.8% fewer opcodes: 12.6K `get_loc_check` where it ran 117.9K and 28.6K `set_loc_uninitialized` where it ran 68.4K. The store pages run 106K `get_loc_check` where they ran 1.36 M, but a check is only a few instructions more than the read without it. The pass costs 0.66 M instructions of the compile of the bundle of `r_react_ssr`, which a VM of tifa does not pay, since it reads bytecode compiled once. An upper bound, a build that drops every check of a local wherever it is and the `set_loc_uninitialized` of every local no closure captures (wrong, for measuring only), ran 0.74% fewer instructions on the workbench pages: the rule gets 0.61 of those 0.74 points. Timed on linux/amd64 at 4 code positions, 3 rounds, against the same main: the workbench pages take 0.995 of the time, MUI 0.997 and the bench pages 0.990, all under 3%, so callgrind decides. The store pages were not timed.

### The optimizer over the linked module

On amd64 and arm64 the generator links the C files into one module and runs `opt -O2` over it (inline threshold 400, the SLP vectorizer off) before the shadow-stack pass ([README](../internal/goasm/gen/README.md#the-whole-engine)). clang had optimized each file alone, so the calls from one file to another (`memcpy`, `memset` and `memcmp` of the small libc, `tlsf_*` and `heap_*` into the allocator callers, `mark_children` in the collector) were never inlined. In native C the same second run saved 1.6% to 1.7% of the instructions of a page (0.8% on MUI). Against main (c5efd5d8), with the same HTML on the 16 pages, in the generated engine:

| Pages | Instructions a render | Change |
| --- | ---: | ---: |
| tifa's shop, six workbench pages | 850.1 M to 841.5 M | -1.01% |
| the shop's store pages | 1,631.4 M to 1,611.3 M | -1.23% |
| tifa's MUI test app, three pages | 672.3 M to 666.1 M | -0.93% |
| tifa's bench app, two pages | 255.9 M to 254.3 M | -0.61% |
| a run of `r_react_ssr`, which compiles its bundle | 108.87 M to 105.07 M | -3.49% |

Instructions are callgrind's at position 0.0 (the difference of 3 renders and 1, Go's collector off). The time is the sum of the P50 of the pages of an app (60 renders after 32 warm ones, `r_react_ssr` the median of 20 runs after 5), 3 rounds at the 8 [code positions](#code-positions) on one core with its sibling idle. The ratio is the median over positions of the paired ratios, and the positions give the lowest and the highest:

| Pages | Time, base to new | Ratio | Positions | By the rule of the positions |
| --- | ---: | ---: | ---: | --- |
| shop, six workbench pages | 157.7 ms to 157.5 ms | 1.001 | 0.991 to 1.017 | no change |
| store pages | 284.0 ms to 282.6 ms | 0.996 | 0.989 to 1.011 | no change |
| MUI, three pages | 130.4 ms to 130.9 ms | 1.009 | 0.995 to 1.016 | no change |
| bench, two pages | 28.24 ms to 27.57 ms | 0.982 | 0.969 to 1.006 | no change (the spiral page 0.968, faster) |
| `r_react_ssr` | 13.56 ms to 13.33 ms | 0.983 | 0.980 to 0.986 | no change, all 8 positions agree |

A second set of 3 rounds of the shop and MUI pages gave 1.000 (positions 0.987 to 1.018) and 1.004 (0.992 to 1.020). In both sets the ratio is highest at the two positions with the body of the interpreter moved by 24 bytes (`0.24` and `32.24`, 1.015 to 1.020 for MUI and 1.017 to 1.018 for the shop). The store pages were not timed again.

The instructions fall by 0.6% to 1.2% on the pages of tifa and the time does not follow, as in native C, where only the React script got faster. On the shop and the store pages the saving is about a third smaller than in native C (MUI is not smaller: 0.93% against 0.77%). The poll pass counts more ticks in the module the second run makes (2,985 against 2,280, with 1,198 bounded loop edges against 749), which takes part of the saving back. Where the new ticks come from was not traced.

What it costs: the generated assembly of amd64 is 13.3% larger (9.34 MB against 8.25 MB, 380,323 lines against 337,285, 25 files against 23), and the `.text` of llc's object 18.9% (1,942,196 bytes against 1,633,929). `JS_CallInternal` is 77,133 bytes against 77,165 and its frame does not change (496 bytes of Go frame, 392 of llc's, 1472 of shadow stack). `lre_exec_backtrack` is 1.5 KB larger (10,819 bytes to 12,365). 17 static functions are gone, inlined into every caller (`BC_add_object_ref`, `cr_add_interval`, `round_to_d`, `find_line_num` and others), and the static `mark_children` is three functions, `mark_children.specialized.2`, `.5` and `.7`, one for each function it marks with. With the SLP vectorizer on, the second run puts 52 lines of vector code into the IR of `JS_CallInternal` again (none without), the 16-byte moves of a JSValue that [EngineValueFlags](changes/c-engine.md#how-a-jsvalue-is-copied) keep out. An inline threshold of 1000 makes the interpreter bigger and its frame 32 bytes taller in native C: not used. The section below has arm64.

#### On arm64

The same run is on for arm64 (`PostLinkOptFor`). The post-link check of the whole engine passes (`-verify-all`, 1335 functions and 1382 pointers of the read-only data), `JS_CallInternal_block` keeps its frame of 504 bytes, `make check-gen` passes for both architectures, and the tests of the root package and of `./internal/...` pass on darwin/arm64 and on linux/arm64.

Instructions are from callgrind on linux/arm64 (Docker on a Mac with Apple silicon, a native arm64 kernel, valgrind 3.24), the counts of one render of each page of an app in a window the benchmark marks, 3 renders after 8 warm-up renders of each page, Go's collector in the count. Against main (84761bc7):

| Pages | Instructions a render, main to new | Change |
| --- | ---: | ---: |
| tifa's shop, six workbench pages | 714.03 M to 708.81 M | -0.73% |
| the shop's store pages | 1,360.00 M to 1,346.60 M | -0.99% |
| tifa's MUI test app, three pages | 588.19 M to 584.32 M | -0.66% |
| tifa's bench app, two pages | 202.97 M to 201.84 M | -0.56% |

The interpreter's own code does not change (-0.06% to +0.02%), as on amd64: the saving is in the calls between files. The HTML of the 16 pages of the shop, store, MUI and bench apps, rendered once by main and once by the branch, is the same byte for byte once the time stamps are masked, on darwin/arm64 and on linux/arm64. Two runs of one binary differ in these stamps alone: each page holds 3 or 4 values `u:` followed by the 13 digits of `Date.now()` (the update time that TanStack Router writes into its dehydrated state), 9 to 20 bytes in all, and the pattern `u:1[0-9]{12}` is replaced by `u:T` before the comparison.

Time was measured on a Mac with an M4 (darwin/arm64) with no control of the code position, which `go tool measure` has on amd64 only. 8 rounds, in each one every page ran 8 warm renders and then 20 timed ones, base and new in turn, the order swapped every round, `GOMAXPROCS=1`, the P50 of a page summed over the pages of an app. The ratio is new over base, the median of the 8 rounds, with the lowest and the highest:

| Pages | Time, main to new (median of rounds) | Ratio | Rounds |
| --- | ---: | ---: | --- |
| shop | 60.58 ms to 60.13 ms | 0.989 | 0.929 to 1.002 |
| store pages | 110.01 ms to 108.57 ms | 0.983 | 0.930 to 1.048 |
| MUI | 53.84 ms to 53.19 ms | 0.988 | 0.979 to 1.174 |
| bench | 10.53 ms to 10.47 ms | 0.982 | 0.950 to 1.003 |
| all four | 234.62 ms to 232.20 ms | 0.987 | 0.968 to 1.015 |

Base alone moves 8% to 16% from round to round on this machine, so the table shows only that no slowdown was seen: new was faster in 7 of 8 rounds of the sum. It does not show a gain of 1%.

What it costs: the generated assembly of arm64 is 12.4% larger (9,225,487 bytes against 8,206,329, 477,900 lines against 421,744, 24 files against 23).

### Locals that share a slot

[Locals that share a slot of the frame](changes/c-engine.md#locals-that-share-a-slot), against main with the optimizer over the linked module (619e1bc8). Instructions are from callgrind on the generated build on linux/amd64 at position 0.0, Go side included: for the pages of an app, the difference of 5 rounds and 2 in one VM, a third, and for `r_react_ssr`, the difference of 3 runs and 1 in one process, halved. A render of `r_react_ssr` alone is the difference of a run with 38 renders and one with a single render, over 37. The HTML of the 14 pages of the shop, store and MUI is the same.

| Pages | main | Shared slots |
| --- | ---: | ---: |
| tifa's shop, six workbench pages | 833.6 M | -1.43% |
| the shop's store pages | 1,588.1 M | -1.02% |
| tifa's MUI test app, three pages | 662.3 M | -0.82% |
| a render of `r_react_ssr` | 54.69 M | -1.81% |
| a run of `r_react_ssr` that compiles its bundle and renders once | 185.59 M | +2.45% |

Time on the generated build, 8 code positions, 3 rounds, one core with its sibling idle. Time is the median over positions of the paired ratios of the P50 of a page (the sum of the pages of an app), positions gives the lowest and the highest, and base range is how far main's own positions are apart. The render of `r_react_ssr` alone is `(a run with 38 renders - a run with 1) / 37` for each side, round and position.

| What | Time | Positions | Base range |
| --- | ---: | ---: | ---: |
| tifa's shop, six workbench pages | 1.002 | 0.996 to 1.010 | 2.0% |
| the shop's store pages | 1.000 | 0.988 to 1.009 | 2.3% |
| tifa's MUI test app, three pages | 0.995 | 0.983 to 1.007 | 2.7% |
| a render of `r_react_ssr` alone | 0.994 | 0.981 to 1.004 | 6.7% |
| a run of `r_react_ssr` that compiles its bundle and renders once | 1.026 | 1.021 to 1.031 | 2.3% |

No page passes the rule for faster or slower, except the run that compiles: the pass costs 4.5 M instructions there, 37 M on the bundle of tifa's shop (3.9% of `js_create_function`, native tree). A VM of tifa reads bytecode compiled once and does not pay it. The 2021 functions of the bundle of the shop have 9,983 slots where they had 12,191 locals. The peak memory of a run of `r_react_ssr` is 8.3 MiB at the median of 24 processes, where main's is 8.7. 3 of the 240 processes ran again because other work used the sibling core.

Against the engine before the optimizer over the linked module (c5efd5d8), with 2 rounds (3 for `r_react_ssr`) at the same positions, the pages were faster than they are against its successor: the shop 0.989 (0.980 to 1.000), the store pages 0.989 (0.979 to 0.998), the MUI pages 0.991 (0.983 to 1.004), `r_react_ssr` alone 1.002 (0.996 to 1.019), and a run that compiles 1.049. Time between the two baselines differs by about 1%, which is inside the spread of the code positions, so read the instruction counts above.

### JSON.parse reads keys and plain strings from the text

[Keys and plain strings read from the text](changes/c-engine.md#jsonparse-reads-keys-and-plain-strings-from-the-text), against main at c5efd5d8 (v0.29.6). Instructions of one `JSON.parse` (the difference of 3 parses and 1), in `jsrun` built from the C (native) and in the generated engine of a Go test binary (`js_json_parse` and what it calls), a byte of the text:

| Text | Bytes | Native | Change | Generated | Change |
| --- | ---: | ---: | ---: | ---: | ---: |
| the rows of the table page of tifa's bench app, 1000 rows | 92,001 | 62.93 to 24.88 | -60.5% | 74.97 to 27.14 | -63.8% |
| the orders the shop's workbench loads | 10,682 | 98.95 to 68.70 | -30.6% | 124.83 to 85.29 | -31.7% |
| the data of tifa's large store page (`/mall/en-US`) | 1,085,235 | 84.47 to 39.21 | -53.6% | 104.37 to 47.32 | -54.7% |
| `package-lock.json` of npm/cli | 437,638 | 60.75 to 31.19 | -48.7% | 74.47 to 37.49 | -49.7% |
| the repositories of the golang organization from the GitHub API | 322,688 | 57.69 to 28.75 | -50.2% | 71.35 to 34.59 | -51.5% |

Before the change, in native callgrind, the 1000 rows spent 12.0 instructions a byte making the text a C string, 30.2 on tokens and strings, 5.7 on key atoms and 5.6 on defining properties. The orders of the shop spent 51.7 on tokens and strings (their strings hold characters of more than one byte, which still take the buffer) and 13.7 on key atoms. The lock file is not a table: 1,202 of its keys are paths of packages that appear once each, which miss the cache, and its packages have 143 different sets of keys. The repositories are 61 objects of 83 keys with objects for the owner, the license and the permissions in them. Both gain about half.

A render, from callgrind at position 0.0 (the difference of 2 renders and 1, with the collector of Go off), with the same HTML on the 16 pages of tifa's apps:

| Pages | Instructions a render | Change |
| --- | ---: | ---: |
| the table page of tifa's bench app | 117.50 M to 112.91 M | -3.91% |
| the spiral page of tifa's bench app (the difference of 4 renders and 1, two runs) | 132.78 M to 132.79 M | 0.00% |
| tifa's shop, six workbench pages | 833.89 M to 831.29 M | -0.31% |
| the shop's store pages | 1,601.73 M to 1,598.73 M | -0.19% |
| `/table/1000` of tifa's large pages, with a VM of 1 GiB | 2,258.61 M to 2,248.90 M | -0.43% |

Each of the six workbench pages runs fewer instructions, from -0.03% (an order) to -0.51% (the stats of a project).

Timed on the generated engine at 8 code positions, 3 rounds, the median of the ratios and their range over the positions (and the spread of main itself over the positions):

| Work | Time of main | Ratio | Range over positions | Spread of main |
| --- | ---: | ---: | ---: | ---: |
| `JSON.parse` of the 1000 rows | 0.643 ms | 0.357 | 0.351 to 0.366 | 1.4% |
| `JSON.parse` of the orders of the shop | 0.137 ms | 0.685 | 0.676 to 0.696 | 1.5% |
| `JSON.parse` of the data of `/mall/en-US` | 14.09 ms | 0.517 | 0.511 to 0.524 | 2.2% |
| `JSON.parse` of the lock file of npm/cli | 3.56 ms | 0.629 | 0.618 to 0.636 | 1.9% |
| `JSON.parse` of the GitHub repositories | 2.38 ms | 0.509 | 0.502 to 0.516 | 1.4% |
| the table page of the bench app | 12.43 ms | 0.968 | 0.954 to 0.977 | 2.0% |
| the spiral page of the bench app | 14.78 ms | 0.994 | 0.974 to 1.024 | 2.3% |
| `/inject/1` of tifa's large pages | 16.28 ms | 0.831 | 0.817 to 0.866 | 2.2% |
| `/mall/en-US` | 191.2 ms | 0.951 | 0.938 to 0.965 | 1.4% |
| `/table/1000` | 401.7 ms | 1.002 | 0.985 to 1.010 | 4.6% |
| the six workbench pages | 154.9 ms | 0.986 | 0.978 to 1.006 | 1.3% |
| the five store pages | 282.0 ms | 1.003 | 0.988 to 1.013 | 4.6% |

The workbench and store pages, the spiral page and `/table/1000` are under 3%, inside the spread of main, so callgrind decides for them: they run fewer instructions, and the spiral page as many. Its difference of 2 renders and 1 moved by up to 0.19% from run to run, on both sides, so it is measured over 3 renders.

On main after the optimizer over the linked module and the locals that share a slot (ffdc0bdb), against that main, callgrind as above: the table page of the bench app runs 3.94% fewer instructions (113.07 M to 108.62 M), the six workbench pages 0.49% fewer (814.00 M to 810.01 M, each of them fewer) and `/inject/1` 9.60% fewer (254.08 M to 229.69 M), with the same HTML on the 16 pages. The JSON rows, the bench pages and the store pages were timed while a check of the engine ran on two other cores of the machine. A first run of the workbench pages then gave 0.997 for the six, with the page of a project at 1.007 (7 of 8 positions), and the run in the table, with those cores idle, gives 0.991 to 1.016 for that page and 0.997 at the median.

### Upstream commits and the scripts where the C engine was slower than C

[upstream-commits.md](changes/upstream-commits.md) lists the commits of upstream QuickJS taken into the C tree and the ones tried and taken back, with their speed. Against main (`c4dcea91`) before the batch, all 37 scripts at 8 positions, 2 rounds: geometric mean 0.917, with `q_array_slice` at 0.085 and `c_array_methods` at 0.530 and the other 35 scripts together at 0.997. `r_react_ssr` took 0.977, within its noise.

Where the C engine was slower than the C QuickJS builds, the same C of `engine/` built with clang -O2 for the machine (the objects of `engine/test` and a runner like `jsrun`) shows where the time goes. Time as a ratio to the C engine as Go assembly (fin2, before the search change below), median of 5 rounds, the assembly at its 8 positions:

| Script | C engine, ms | same C, clang | pinned 3d5e064, clang | pinned, gcc | latest 19d6bd8, clang | latest, gcc |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `c_array_methods` | 395.3 | 0.744 | 0.813 | 0.815 | 0.719 | 0.701 |
| `c_string_methods` | 358.3 | 0.785 | 0.796 | 0.880 | 0.809 | 0.874 |
| `q_regexp_ascii` | 376.0 | 0.829 | 0.765 | 0.777 | 0.746 | 0.774 |
| `r_react_ssr` | 386.6 | 0.940 | 1.073 | 1.062 | 1.031 | 1.049 |

The C changes of liustack/quickjs cost nothing on the first two (the same C is as fast as the pinned QuickJS or faster), so what is left is the cost of the assembly: 20% to 30% on these three scripts, 6% on `r_react_ssr`. On `q_regexp_ascii` the same C is 8% slower than the pinned QuickJS (0.829 against 0.765 to 0.777). The cause was not looked for: the native host also calls `host_poll` (2% of the samples). perf (4999 Hz, samples of the same 20 runs, the assembly against the same C):

- `c_array_methods`: `js_array_includes` 6020 samples against 1826, `js_array_indexOf` 3915 against 957, `JS_CallInternal` 10743 against 8749, the rest alike (`js_strict_eq2` 13630 against 13231). The loops called `js_strict_eq2` for each element, and a loop with a call that may poll keeps its poll counter in memory, which is a read, decrement and write of one word per turn: 36% of the samples of `js_array_includes` sit on that decrement. Fixed by [deciding most elements without a call](changes/upstream-commits.md#indexof-and-includes-decide-most-elements-without-a-call): the script took 0.583 of the time.
- `c_string_methods`: 38900 samples against 30440, spread over many small functions: `JS_CallInternal` +940, `js_call_c_function` 1964 against 1210, `JS_ToStringInternal` 1726 against 1122, `JS_ConcatString` 1182 against 658, `js_string_includes` 967 against 523. No loop with one hot instruction. It is the cost of each call of the generated code (arguments and results through memory, a stack check, the poll counter).
- `q_regexp_ascii`: 39700 samples against 33500. The matcher `lre_exec_backtrack` is alike (10390 against 10002) and so is `JS_CallInternal`. The extra samples are in what surrounds it: `js_regexp_exec` 3582 against 2374, `lre_find` 1540 against 615, `lre_exec` 2367 against 1660, `free_object` 2440 against 1650. Calls again.

### Shapes of built-in classes, shapes found by their parent, objects that die alone

Three changes, measured together and each against the one before it: [objects of the built-in classes find the empty shape of their prototype](changes/objects-and-shapes.md#objects-of-the-built-in-classes-find-the-empty-shape-of-their-prototype), [a shape is found by the shape it grew from](changes/objects-and-shapes.md#a-shape-is-found-by-the-shape-it-grew-from) and [an object that dies alone is freed at once](changes/objects-and-shapes.md#an-object-that-dies-alone-is-freed-at-once). Instructions per render of every page of an app are from callgrind at position 0.0, with the same HTML on the 16 pages. The first column is the native C tree, the other two the generated engine.

| Pages | Built-in class shapes | Shape by its parent | Object freed at once | All three |
| --- | ---: | ---: | ---: | ---: |
| tifa's shop, six workbench pages | -0.61% | -0.82% | -0.65% | -1.93% |
| the shop's store pages | -0.69% | -0.88% | -0.71% | -1.84% |
| tifa's MUI test app, three pages | -0.19% | -1.38% | -0.64% | -2.13% |
| tifa's bench app, two pages | | -0.03% | -0.45% | -0.49% |

The interpreter's own instructions do not change (within 0.04%, the spread of two runs of one binary). All three against main, timed on linux/amd64 by the protocol of [measuring.md](measuring.md) (16 layouts, delta 1%), the mean of the layouts: the bench pages take 0.73% less time, faster by the rule, the MUI pages 0.57% less, the six workbench pages 0.26% less and the store pages 0.13% more, none of them a regression beyond delta. The timing was of the first form of the three, before the review moved an object that dies alone onto the list of objects to free before it is freed. That costs 0.06 to 0.18 points of the instructions above, under the 0.2 from which the time is taken again.

### Every function that calls has a stack check

[Functions without a stack check](changes/c-engine.md#functions-without-a-stack-check) is a change that costs speed, made so that a fault in the engine does not depend on room read from one release of the Go runtime. Against main at 053d158e, on linux/amd64, with the same list of functions without a stack check as the change. Instructions are from callgrind (`BenchmarkAppCG`), counted as what the functions that lost NOSPLIT run more, since the whole program moves by up to 0.4% between two builds of the same engine.

| Pages | Instructions | Mean of the 16 layouts | Formal layout |
| --- | ---: | --- | --- |
| tifa's shop, six workbench pages | +0.27% | +0.47% [+0.00%, +0.94%] | +0.61% [-0.36%, +1.60%] |
| the shop's store pages | +0.32% | +0.04% [-0.51%, +0.59%] | +1.09% [-0.11%, +2.31%] |
| tifa's MUI test app, three pages | +0.36% | +0.63% [+0.22%, +1.04%] | +1.07% [+0.35%, +1.79%] |
| tifa's bench pages | +0.29% | +0.17% [-0.34%, +0.68%] | +0.81% [+0.24%, +1.38%] |

Time is the sum of the pages, as [measuring.md](measuring.md) says: 16 layouts, 2 rounds, delta 1%, 95% intervals, cpu 0 with its sibling idle. The mean of the layouts is slower by less than delta on the shop, slower on MUI, and shows no regression beyond delta on the store and the bench pages. No app is shown to be past 1%.

### What the generated code costs against native C

Measured on 2026-10-30 on linux/amd64 only (the machine above, timing on one core with its sibling idle). Four builds of the same C with the same JavaScript and the same steps in the host (load the bundle, start a render from a JSON, run the jobs, read the result): the pages of the shop, the store, MUI and the bench app from fixed bundles and inputs, with a fake Intl and a fixed clock, so the Go Builtins of tifa are not in it. All 26 pages give the same HTML in all four builds.

- **N0**: clang -O2 with the portable flags of `engine/test/flags.sh`.
- **N1**: N0 with every code option the generator gives clang (frame pointer kept, no red zone, no tail calls, no SLP vectorizer and `+slow-unaligned-mem-16` for `quickjs.c`, and the others).
- **G0**: the generated assembly with the polls left out (`-poll=false`).
- **G1**: the generated assembly as it ships.

Median of 5 rounds at 8 code positions each (the round and the order of the builds drawn at random), as a ratio of time:

| Workload | N1 / N0 | G0 / N1 | G1 / G0 |
| --- | ---: | ---: | ---: |
| shop workbench, 6 pages | 0.997 | 1.111 | 1.042 |
| store, 14 pages | 0.999 | 1.128 | 1.048 |
| MUI, 3 pages | 0.997 | 1.110 | 1.024 |
| bench, 2 pages | 0.967 | 1.121 | 1.068 |
| `r_react_ssr` | 0.976 | 1.109 | 1.021 |
| Markdown of 400 KB | 0.999 | 1.108 | 1.038 |

- **The code options cost nothing.** N1 has about 2% more instructions than N0 and takes the same time. Only the bench pages are faster (the SLP option, which breaks up the copy of a JSValue).
- **The translation costs 11% to 13%** (G0 against N1, the same sign at all 8 positions), and the polls 2% to 7% on top (4% to 5% on the shop pages, 6.8% on the bench pages). N1 has one cost that generated code does not have: its check of the shadow stack is a call. With that check inline N1 is 0.7% to 3% faster, so the 11% to 13% is about one point low.
- **Instructions and memory.** G0 runs 13% to 19% more instructions than N1 and 19% to 25% more loads and stores. Its misses in the instruction cache are 2.2 to 3.7 times as many, its data cache misses only 6% to 9% more. The polls add 3.8% to 7.5% instructions and 11% to 26% more instruction cache misses. `JS_CallInternal` is 48,413 bytes natively and 86,767 as generated (79% more), which is where the instruction cache misses come from.
- **Where the extra instructions come from**, with the shadow stack check of N1 inline (the shop workbench pages, other workloads within a few points): arguments and results that go through the Go stack 25% to 43%, the dispatch through a table of relative offsets (3 more instructions than an absolute table) 22% to 41%, and the Go stack check, the no-ops after the prologue and the jump to the one shared `RET` together 17% to 27%. Reads and writes through the shadow stack, for locals whose address is taken, are another 18% to 22%. Saving and reloading registers around calls comes out even, since `preserve_none` puts it in the callee and the native code in the pushes and pops. Everything else (moves, arithmetic) is fewer instructions in the generated code, for a reason not looked for.
- **By function.** `JS_CallInternal` is 52% of the difference on the shop pages and 74% on `r_react_ssr`. The smaller the function, the higher the ratio, since a stack check, the reads of the arguments and the store of the result are a fixed price for each call: `js_call_c_function` 1.45 times the instructions, `JS_GetPropertyValue` 1.32, `add_property` 1.17, `free_object` 1.06. Strings and regexps are about even.
- **Not measured.** How much time each cause takes: `perf` has no cycles on the VM, and a fit of time against instructions and instruction cache misses was off by up to 35%. Time grew 0.65 times what instructions did, so the extra instructions are cheaper than the rest (loads that hit the cache, no-ops).

What the generator could still give back is small or already tried. The absolute dispatch table (22% to 41% of the extra instructions) did not change the time. Dropping the no-ops and the shared `RET` is estimated under 1% of the time. Registers for the arguments of calls into the functions without a stack check would give 0.3% to 0.9% (see [What was tried and not done](#what-was-tried-and-not-done)). The shadow stack reads are the design.

**Results from registers.** Since the release after v0.29.5, a call to another generated function takes the result from the registers the callee returns it in (RAX, RDX or X0 on amd64, x0, x1 or d0 on arm64) and no longer loads it back from the Go stack. The callee still stores it to its Go result slot, and after that runs only Go's epilogue, which touches the frame pointer, the return address and SP, so the registers still hold it after the `RET`. 6,982 call sites on amd64 and 7,087 on arm64 skip the load. A call through a pointer, to Go code or to a function that failed to generate keeps it. The post-link check holds each call that skips the load to a callee that is a lowered unified function, and that callee to its return sequence. Measured on 2026-10-31 against v0.29.5, instructions by callgrind on amd64, time at 4 code positions and 3 rounds on each machine (linux/amd64 on the machine above, darwin/arm64 on an Apple M4), the same HTML on both:

| Workload | Instructions (amd64) | Time, amd64 | Time, arm64 |
| --- | ---: | ---: | ---: |
| shop workbench, 6 pages | -0.37% | 0.978 | 0.995 |
| store, 5 pages | -0.62% | 0.980 | 0.990 |
| MUI, 3 pages | -0.65% | 0.984 | 0.996 |
| bench, 2 pages | -0.53% | 0.995 | 0.997 |
| `r_react_ssr` | -0.42% | 0.973 | 0.997 |

All changes in time are under 3%, so the instructions are the measure. All 4 positions agree on amd64 for every workload. The time on amd64 fell more than the instructions did, by a cause not measured. The generated assembly is 1.9% smaller on amd64 and 2.1% on arm64.

### Where a render's time goes on the C engine

From callgrind on the generated engine (amd64), with cycles estimated as instructions plus 10 for each L1 miss plus 40 for each last level miss (last level 512 KB, as on Zen 3's L2). Each page is a run of renders less a run of fewer, so loading and warm-up cancel out. The lines of `JS_CallInternal` were split by a native build with debug lines on the same pages. Measured on 2026-10-29 on liustack/quickjs `6c757279`, before the objects of 40 bytes and the single scan of shared bytecode for polls.

| Share of a render | shop workbench | store | MUI | bench | `r_react_ssr` | Markdown 400 KB |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Dispatch and the opcodes themselves | 19.9% | 19.4% | 24.9% | 20.4% | 31.6% | 18.7% |
| Reading and writing properties, adding them | 20.4% | 22.8% | 24.2% | 15.2% | 12.7% | 19.0% |
| Calls and returns (frames, arguments, C functions) | 18.8% | 17.4% | 15.2% | 21.8% | 17.8% | 18.9% |
| Making and freeing objects, reference counts | 16.8% | 15.6% | 15.8% | 14.8% | 14.7% | 17.1% |
| Strings | 6.1% | 7.0% | 7.1% | 8.3% | 7.7% | 3.2% |
| Regexps | 3.1% | 3.5% | 1.7% | 4.2% | 1.7% | 1.9% |
| Garbage collection | 1.5% | 1.6% | 0.5% | 0.3% | 0.6% | 8.5% |
| C built-ins (arrays, JSON, Map) | 3.0% | 2.8% | 1.6% | 3.7% | 2.1% | 2.9% |
| Go host and Builtins | 2.1% | 1.7% | 1.9% | 1.1% | 0.3% | 0.2% |
| Polls and stack checks (instructions only) | 8.1% | 8.2% | 7.0% | 10.1% | 8.5% | 8.1% |
| Parsing | 0.0% | 0.0% | 0.0% | 0.1% | 2.2% | 1.5% |

- No class is above 25%, and properties, calls and objects together are 50% to 55%. The dispatch alone (4 instructions per opcode) is 10% to 12% of `JS_CallInternal` and 5% to 7% of a render.
- A workbench render runs about 73,000 JavaScript calls and 1.28 million opcodes: 81 native instructions per opcode counting what it calls, and about 190 per call in the entry, the exit and the call opcodes, of which about 95 are the entry. The largest piece of the entry is the loop that sets the locals to `undefined` (2.1% of the native instructions of `JS_CallInternal`).
- The polls, measured by building the engine without them: the shop, store and MUI pages took 0.953 to 0.955 of the time and the bench pages 0.930, at 8 of 8 positions. 4.9% to 8.3% fewer instructions. `JS_CallInternal` holds 41% of the polls' instructions on a store page and 64% on a bench page, and the rest is spread over about 40 functions. The stack check is 4 instructions at the start of 729 of 1320 functions, 1.2% to 1.8% of the instructions of a page.
- Collection matters only on the long Markdown: 8.5% of the time and 40% of its last level misses. On tifa's pages it is under 2%.
- Against Node with `--jitless`, liustack/quickjs took 1.82 times as long on the shop workbench, 1.67 on the store and 1.52 on MUI. The JavaScript functions with the highest ratio are small ones (3 to 5 times, since the cost is the fixed price of a call), React's `for in` with `hasOwnProperty.call` (2.0 to 2.6 times, done since: [Calls, for in and hasOwnProperty.call](#calls-for-in-and-hasownpropertycall) and [The copy of the props of a React element in C](#the-copy-of-the-props-of-a-react-element-in-c)), and functions where a reference count reaching 0 frees a whole chain at the assignment, where Node leaves it to its collector (`resetHooksState` 3.5 to 4.9 times).

### Stability under load

Measured on the Google Cloud VM above (Go 1.27.2), with the 18 pages of tifa's example shop (its 6 workbench pages and 6 storefront pages in each language) rendered through tifa's pool, the C engine against the ccgo engine (`lib`, [history.md](history.md)), built from the same tree. The cores are pinned: `1,3` are two physical cores, `1,3,5,7` are those two with their hyperthreads. The pool keeps tifa's rules: any error retires the VM, a young GC runs after each render and a full one every 256.

**Soak.** 35 minutes, 2 VMs, 2 callers that render in turn without a pause, memory read every minute (after a full GC for the VMs). The C engine made 106,194 renders and the engine of `lib/` 91,131, with 0 errors and no VM replaced in either.

| After the first 5 minutes | C engine | lib |
| --- | --- | --- |
| Memory of the VMs (`MemoryUsage.MallocSize`, summed) | 21.79 MB at minute 5, 21.85 MB at minute 35 (+0.3%) | 24.71 MB, 24.72 MB (+0.06%) |
| What the VMs took from the system (`Footprint`) | 34 MB, not a byte more | not counted |
| Process RSS | 64.1 MB, then 65.1 MB (+1.0 MB, still creeping) | 68.8 MB, then 69.8 MB (flat from minute 20) |

The rise of the RSS is the same size in both engines (+957 KB and +925 KB between the first 5 and the last 5 samples), and it is not the VMs. The Go runtime asked the system for no more memory (`Sys` did not change). The stacks of goroutines grew by 550 KB (C engine) and 400 KB (lib), taking pages the heap had held. The rest, 400 to 500 KB, cannot be told apart in these data: pages that were mapped and are first touched, pages of the Go heap used again, and memory outside Go. They would need `smaps` and `HeapReleased` in each sample. 35 minutes cannot say whether the C engine's creep stops.

**Several VMs.** Renders a second and the latency of `Render` (median of 3 alternating rounds of 15 seconds, one caller for each VM):

| Cores | VMs | C engine renders/s | p50 | p99 | lib renders/s | p50 | p99 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1,3 | 1 | 49.5 | 19.7 ms | 63.1 ms | 40.9 | 21.5 ms | 80.2 ms |
| 1,3 | 2 | 98.8 (2.00x) | 17.8 ms | 63.6 ms | 80.2 (1.96x) | 21.4 ms | 82.5 ms |
| 1,3,5,7 | 1 | 49.5 | 19.6 ms | 63.2 ms | 41.1 | 21.5 ms | 80.3 ms |
| 1,3,5,7 | 2 | 94.8 (1.92x) | 18.0 ms | 69.3 ms | 77.6 (1.89x) | 21.5 ms | 90.3 ms |
| 1,3,5,7 | 4 | 117.7 (2.38x) | 28.5 ms | 118.8 ms | 96.8 (2.36x) | 34.1 ms | 147.4 ms |

Two VMs on two physical cores give twice one VM. Four VMs on two cores and their hyperthreads give 2.4 times, which is what the hardware has. Time spent waiting for a lock was at most 0.002% of the render time on the C engine (0.05% on lib), so no lock or shared state holds the VMs back. The C engine renders 20% to 24% more a second and its p99 is 21% to 22% lower at the same load.

**Go GC pauses.** The same pool renders while a goroutine allocates 512 MB a second (75 to 98 GCs a second, with a graph of 100,000 nodes on the heap). Stop-the-world pauses from `/sched/pauses/total/gc:seconds`, and the time the runtime waited for the Ps to stop (`/sched/pauses/stopping/gc:seconds`), which is the part a VM that does not reach a poll would add. The buckets are about 6% apart, so these are upper bounds. One run of 60 seconds each:

| | C engine, 1 VM | lib, 1 VM | C engine, 3 VMs | lib, 3 VMs |
| --- | ---: | ---: | ---: | ---: |
| Pauses in total: P50 / P99 / max | 33 / 131 / 3146 µs | 33 / 164 / 4194 µs | 57 / 197 / 524 µs | 57 / 197 / 2097 µs |
| Waiting to stop: P50 / P99 / max | 25 / 131 / 655 µs | 25 / 131 / 197 µs | 41 / 197 / 459 µs | 41 / 197 / 328 µs |
| Renders a second, p99 of a render | 45.2, 68.8 ms | 38.4, 84.4 ms | 92.1, 115.0 ms | 75.4, 143.8 ms |

The P50 and P99 are the same in both engines. The maximum of the C engine is the longest wait for a poll in these runs: 0.66 ms with 1 VM and 0.46 ms with 3. Those are single samples out of 5,600 and 4,500 GCs.

A script that stays inside one operation of the engine, with the GC forced every 5 ms for 10 seconds (the scripts of `internal/cengine/cmd/scenarios`, run by `gcpause`), 1 VM, cores 3 and 7. Time the runtime waited to stop, P50 / P99 / max in µs:

| Script | C engine | lib |
| --- | --- | --- |
| `for` loop, `fib(27)` | 25 / 41 / 82 and 25 / 41 / 1311 | 328 / 328 / 655 and 25 / 41 / 197 |
| regexp that backtracks, regexp scan of 64 MiB | 57 / 82 / 98 and 49 / 66 / 98 | 25 / 41 / 49 and 25 / 41 / 229 |
| `replace` with a global regexp of 8 Mi characters | 25 / 131 / 164 | 25 / 49 / 5243 |
| `JSON.parse`, `JSON.stringify` of 300,000 objects | 25 / 41 / 786 and 25 / 49 / 197 | 25 / 41 / 2621 and 25 / 41 / 917 |
| `indexOf` over 128 MiB | 164 / 229 / 393 | 25 / 14680 / 14680 |
| `split` of 4 Mi characters, `join` of 2 Mi strings | 25 / 98 / 197 and 25 / 164 / 262 | 25 / 16777 / 20971 and 25 / 524 / 917 |
| typed array sort, `sort()` of 500,000 numbers, `fill` of 4 Mi | 29 / 164 / 197, 25 / 82 / 197, 29 / 82 / 197 | does not stop, see below |
| `normalize` of 4 Mi characters | 66 / 98 / 393 | 25 / 41 / 57 |
| multiplication of 475,000-bit BigInts | 33 / 49 / 98 | does not stop |

The longest wait of the C engine, 1.3 ms in `fib`, came once in 1,857 GCs, and the 786 µs of `JSON.parse` once in 1,857. The rest is under 0.4 ms. The engine of `lib/` waits up to 21 ms in two string loops. (`lib` rows marked "does not stop": the script did not end a minute after `Interrupt`, so the run has no figure.)

**Interrupt.** `interrupt` (`internal/cengine/cmd/interrupt`) stops the same scripts at random points of their run and times the return of `Eval`. Median in ms, 6 stops each, on the first sizes of the scripts (32 Mi characters to `replace`, 64 Mi to `split`, 16 Mi to `fill` and `normalize`, 2 Mi strings to `join`). A stop at 50 ms is the kill of the C engine, which comes 50 ms after `Interrupt` for a script that reads no flag. The engine of `lib/` kills nothing, so it keeps running until it reads the flag:

| Script | C engine before | C engine after | lib |
| --- | ---: | ---: | ---: |
| `replace` with a global regexp (generic path) | 50.1 (killed 6 of 6) | 50 (killed) | 231 |
| `JSON.parse` of 300,000 objects | 22 | 22 | 265 |
| `JSON.stringify` of 300,000 objects | 4.1 | 4.1 | 125 |
| `split` | 50.5 (killed) | 50 (killed) | 2406 |
| `fill` | 50.1 (killed) | 38 | does not stop |
| `join` | 47.4 | 3.2 | 56 |
| `normalize` | 50.1 (killed) | 13.3 | 1709 |
| `sort()` of 500,000 numbers, sort of 4 Mi doubles | 13.3, 3.8 | 13.3, 3.8 | does not stop |

What decides the time is of two kinds. A loop that does not read the flag runs to its end or to the kill: `fill`, `join`, `normalize` and the global `replace` that matches in C were such, and read it now every 4096, 1024, 4096 characters and each match. After an error the call also frees what it made, and that has no flag to read: the 300,000 objects of a `JSON.parse` take 36 ms to free (the engine of `lib/`: 58 ms), and so would the 33 million strings of a `split` of 64 Mi characters and the 16 million arrays that a generic `replace` of 32 Mi characters holds until it ends (not timed apart). A script that builds more than about 400,000 objects in one call can be killed while it frees them. The stop of a `JSON.parse` loop follows the size of the tree: 1.8 ms for 10,000 objects, 5.5 ms for 30,000, 20 ms for 100,000, and the kill at 600,000 (lib: 12.5 ms, 38 ms, 145 ms, 1.4 s). The VM is gone either way, and the pool replaces it. `JSON.stringify` reads the flag at every value, and its 2.5 to 4 ms are the free of the half-written string (1.7 ms for 25 MB) and the allocation of the error. Multiplying, dividing or powering BigInts of hundreds of thousands of bits, and turning them to and from strings, read no flag before and ran to the kill. They read it now every 32,768 limbs or digits ([uncatchable.md](changes/uncatchable.md#long-operations-on-bigints-read-the-interrupt-flag)): on 20 stops of `interrupt`, the scenario `bigint_mul` stopped at a median of 50.03 ms with 17 VMs killed, and stops at 0.14 ms with none killed now (core 0 of an EPYC 7B13).

`split`, `isWellFormed`, `toWellFormed`, `padStart`, `padEnd`, `trim`, the attribute of `anchor` and the other HTML methods, `escape`, `unescape`, `encodeURI`, `decodeURI`, `Array.from`, the typed array constructors, making a rope flat and `<` on two ropes read the flag now too (see [c-engine.md](changes/c-engine.md#polls-and-interrupts)). The `split` row of the table above is the scenario `string_repeat_split` (4 Mi characters in 2 Mi pieces) on the 20 stops of `interrupt`: the median was 67.8 ms and the highest 243 ms, and it is 23.3 ms and 39.7 ms now (core 5 of an EPYC 7B13, no VM killed in either run). What is left is the free of the pieces made so far. A stop of `array_join` and `normalize` did not change (4.3 and 4.1 ms, then 3.5 and 4.1 ms). The new loops cost the pages no more than the noise of the Go runtime in a callgrind run: per render, `r_react_ssr` runs 0.06% more instructions (2,511.2 M, then 2,512.8 M), the six workbench pages of the shop 0.18% more (978.1 M, then 979.9 M, of which the Go collector's work in the two runs is more than the difference), the four MUI pages 0.06% more (751.2 M, then 751.6 M) and the bench pages 0.09% more (309.9 M, then 310.2 M), against main (4cdd3a27).

**Timeouts and kills in the pool.** 4 VMs, 3 callers rendering all the time, 300 ms timeout. A fourth caller sends a request every second that loops for ever (`loop`), one that sleeps 1.5 s in a Go function it called (`sleep`), or both in turn. Two runs of 20 seconds each, cores 1,3,5,7, measured on v0.29.0, when the first poll after the sleep was past the grace and killed:

| | C engine good renders/run | p50 | p99 | bad requests | killed | lib good renders/run | p50 | p99 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| No bad requests | 2164 | 22.2 ms | 100.2 ms | | | 1752 | 27.2 ms | 132.0 ms |
| `loop` | 1928 | 25.1 ms | 116.9 ms | 50, all failed | 0 | 1609 | 30.1 ms | 140.2 ms |
| `sleep` | 2040 | 23.2 ms | 103.7 ms | 34, all failed | 34 | 1688 | 28.4 ms | 127.8 ms |
| both | 1947 | 24.3 ms | 112.8 ms | 49, all failed | 24 | 1633 | 28.7 ms | 130.5 ms |

Every bad request failed with a deadline error, every one got a new VM, no good render failed, and all 4 VMs were back in the pool at the end. Killed VMs were replaced like timed out ones. Since a kill also needs 1000 polls of work after the request (`gohost.KillPolls`), the sleep kills nothing: the script reads the flag as soon as the Go function returns and fails with the same deadline error, and the VM is replaced as a timed out one. The killed column of `sleep` and `both` would be 0. The table was not measured again. The good renders' p99 grew 3% to 17% on the C engine and by at most 6% on lib, the cost of loading a replacement VM while the others render. The C engine's p99 stays under lib's in every row.

## What was tried and not done

Each item says what it would give and why it was dropped. Check here before trying one of them again. The numbers come from tifa's [docs/performance.md](https://github.com/liustack/tifa/blob/main/docs/performance.md#what-was-tried-and-not-done) and [docs/studies/engine-hotspots.md](https://github.com/liustack/tifa/blob/main/docs/studies/engine-hotspots.md), and from the docs of the changes.

- **Dispatching opcodes through closures or a table of functions**, as some Go interpreters do: a rewrite of the interpreter, against the rule of liustack/quickjs to make only mechanical changes. Not tried.
- **A check in JavaScript in front of one spread for the props of `createElement` and `cloneElement`**, measured on v0.29.1 before `RegisterCopyProps`: the plugin tested the config (the prototype, the symbols, `__proto__`, `__self` and `__source`, a getter behind `key`) and then copied it with the rest of a destructuring. The test took 310 to 380 ns a call, more than the loop takes for a config of none, one or two keys, so `createElement` took 1.57 times as long for no keys, 1.07 for one key and 1.13 to 1.40 for a `key` and one or two more. The shop took 0.99 of the time (1.6% to 2.5% fewer instructions) and no page passed the rule. It also could not rule out a proxy, a getter that deletes a later key and a setter added to `Object.prototype` after React loaded. The same checks in C cost less than the empty loop, and the function gives 2.1% to 4.1% fewer instructions.
- **A fast path for object spread** (`JS_CopyDataProperties`): one spread took 240 ns rather than 415 in a micro benchmark on the Apple M4, but the shop did not change (0.997 to 1.005). It is done since together with the listing of keys, which it shares with `for in` and `Object.keys` (see [Own keys listed from the shape](changes/objects-and-shapes.md#own-keys-listed-from-the-shape)).
- **Testing the uncatchable flag before calling `_js_uncatchable_pending`** at the entry of `JS_CallInternal`: the helper is too large to inline into the interpreter, so every JavaScript call pays a real call to it. Testing the runtime's flag inline first skips that call on the normal path, and keeps the full check, which is still needed because the flag stays set after the exception is taken. 0.06% fewer instructions on V8, 0.09% on the shop and 0.41% on MUI. Over six rounds on three code layouts, V8 took 0.9988 of the base score (0.9883 to 1.0254), the shop 0.9927 of the time (0.9859 to 1.0014) and MUI 0.9850 (0.9763 to 1.0265). The sign follows the layout: MUI took 1.027 and 1.012 of the time in the two rounds of one layout, and 0.976 to 0.988 in the four rounds of the other two. The gain cannot be told from layout noise.
- **Reading the count of locals once, alone**: at the start of a call the interpreter read `b->var_count` again after each local it set. Reading it once gave React 1.0% fewer instructions but a loop of arithmetic 3.9% more, from a register spilled in one handler, and the time did not change. It is done since together with setting four locals a turn and freeing two values a turn (see [Calls, for in and hasOwnProperty.call](#calls-for-in-and-hasownpropertycall)).
- **Spreading a dense array without its iterator**: `[...a]` of three elements took 165 ns rather than 615, and a call with spread arguments 394 ns rather than 869, on v0.26.17 in a micro benchmark on linux/amd64. The shop's six pages (0.997) and three MUI pages (1.007) did not change beyond the spread that the layout of the code gives, since pages rarely spread arrays. It would have added about 870 lines of fixlib rules.
- **Keeping the in-between shapes of every object literal**, before v0.26.16: about 2% more on top of the new property fast path, but the two list pages ran out of VM memory, because the kept shapes held their prototypes and everything those reach. v0.26.16 keeps only shapes whose prototype is `Object.prototype`, at most 32 keys and no unregistered symbol key, in a ring of 2048 (see [Kept shapes](changes/objects-and-shapes.md#kept-shapes)).
- **Building a shape in place when no other object has its keys**: in the first form of the shape cache, copying every shape made objects of keys no other object has 22% slower to build. v0.26.16 earns one copy for each shape found, up to 64, which leaves those objects about 6% slower.
- **Literal templates in the bytecode**, as V8 copies a prebuilt object: needs compiler and bytecode format changes, and quickjs-ng saw no clear difference when it tried. It is done since, as one opcode for each run of plain keys whose final shape the function keeps (see [Object literals made at their final shape](changes/objects-and-shapes.md#object-literals-made-at-their-final-shape)).
- **Allocating the property array of a literal at its final size**: the opcode that makes the object does not know the count, and growing arrays differently costs memory for every small object. It pays only together with kept shapes. Done since with that opcode, which knows the count of its keys.
- **Inline caches for property reads**: quickjs-ng added and then removed them. Across 17 real tools they ranged from 11% slower to 54% faster, 4% faster on average, and used more memory. Tried after v0.27.0 as a table of 4096 entries keyed by the bytecode position, each holding a shape and the index of an own data property, checked against the shape itself (the index under its count, the same key, plain flags), so it needs no invalidation. Most reads on tifa's pages are own properties of the object, which `find_own_property` already finds in one hash step, and a hit on a prototype cannot skip the check of the shapes on the way. 0.3% fewer instructions, but the shop took 1.006 of the time and the MUI pages 1.007 (6 rounds, 3 layouts), for 96 KiB more per VM. Tried again on 2026-10-30 on the C engine as a table per function, one entry of 16 bytes for each byte of its bytecode, holding a reference to the shape: 71% to 98% of the places that read a property by name see one shape only, but a hit costs about as many instructions as the hash lookup it skips. Against the engine of v0.29.4, callgrind gave -0.29% to +0.93% of the instructions and, with the cache misses weighed in, 1.2% to 2.7% more cycles: the misses of the three reads it serves fell by 70% to 90%, but they were only 0.52% to 0.64% of the cycles, and the reads of the table missed more. A compact table, one entry for each place that reads, could win back at most about 0.4% less its own misses, for a new bytecode format and a shape held by the cache that its only object could no longer change in place. Not done.
- **A table of shape steps for new keys**: `add_property` looks for the shape an object goes to in the shape hash table, and that lookup misses the cache often (2.4% of the instructions of a render of the shop, 7.7% of its cache misses). A table of 1024 steps per runtime, from a shape and a key to the next shape, holding both, cut 0.8% of the instructions on the shop and the MUI pages, but the shop took 1.008 of the time and the MUI pages 1.019 (6 rounds, 3 layouts): the shapes it holds alive (2718 to 3504 on an MUI page) add 28 KB to 142 KB per VM and change when the cycle collector runs.
- **Fast arrays in `every`, `some`, `forEach`, `map` and `filter`**, without asking `JS_TryGetPropertyInt64` for each element: `js_array_every`, which runs all five, spends 0.67 s of its own outside the callbacks in a profile of 85.6 s of the ten store pages of tifa's example shop, `JS_TryGetPropertyInt64` 0.19 s of it. A fast path would save at most about 0.5% of a store render, which the layout of the code hides. `find`, `findIndex` and `includes` already read fast arrays directly.
- **`+slow-unaligned-mem-16` on every file, or with the SLP vectorizer on** (C engine): the libc gets 8-byte moves in its `memcpy`, and with the vectorizer on `r_shiki_regexp` took 1.028 to 1.033 of the time and 29 SSE moves stayed in `JS_CallInternal`. Only `quickjs.c` gets it, with the vectorizer off ([numbers](#the-c-engine-copies-a-jsvalue-in-two-moves-of-8-bytes)).
- **Waiting for Go 1.28**: a development build made the unchanged engine 5% faster on amd64 and 2% on arm64, but the rewritten engine 5% to 6.5% slower on amd64, for a reason not found. The rewrites still win there by 4.2% on amd64 and 15.5% on arm64.
- **Dropping function source from the bytecode** (`JS_SetStripInfo`) to save VM memory: seroval, which tifa's pages use, reads the source of its own helpers with `toString` and writes it into pages, so the browser would break.
- **Removing the store of the bytecode position on arm64**: the up to 11.6% of the interpreter's own time it seemed to take is where the sampler puts the wait for the next opcode. The store is Go saving the local `pc` for the calls that read it back, on the path every opcode takes. Only moving the instructions put the samples on the opcode load instead (8.8% down to 0.4%), with the same instruction count, cycles and render time.
- **Ropes sooner in the C engine**, for React's page string: making a rope of any result over 512 characters (`JS_STRING_ROPE_SHORT2_LEN` of 512 instead of 8192) took `q_string_build1` from 0.18 s to 0.32 s natively, since `+=` on a local then leaves its in-place append for ropes, and gained React no more than copying with room (see [Strings built by appending](#strings-built-by-appending)).
- **Skipping leaf objects in the cycle collector**, as CPython skips tuples that hold only atoms: a plain object, array or closure variable that holds no GC value (only numbers, strings, booleans and the like) moves to a list the collector never walks, and back when a GC value is stored into it. On a render of a 400 KB Markdown page only 7.6% of what the collector visits is such an object that lives on (304k of 4.0M visits), since 61% of what it visits is garbage made since the last collection. The prototype cut the collector's work by 6.6% of its instructions and took that render to 0.978 of the time (0.964 to 0.991 over 8 positions, 5 rounds, the base's own range 2.9%), but the checks on stores and the frees it defers cost as many instructions as the collector saved (-0.00% a render), and `r_react_ssr` paid 0.11% more. tifa's pages, which rarely collect during a render, did not change (shop 1.004, MUI 0.999, bench 1.002). Every store of a GC value into an object, an array or a closure variable needs the check, about 150 places in the C, and a missed one keeps garbage until the runtime is freed, which the block count of `JS_FreeRuntime` cannot see. A build that checks the list at every collection found a missed store in the first scripts of `compare.sh` that it ran (a global variable defined after a function used it). A leaf also hides its edge to its prototype, so a context freed while its runtime lives would stay until the runtime is freed. CPython dropped the same scheme for dicts in 3.14 ([gh-127010](https://github.com/python/cpython/issues/127010)): the check on every store cost more than the collector saved. quickjs-ng has nothing like it.
- **Passing arguments and results in registers to the functions without a stack check** (the NOSPLIT ones, since Go's morestack loses registers), measured with callgrind on the C engine, not built. One call between generated functions comes every 171 to 283 instructions, and 23% to 27% of them go to a NOSPLIT callee. The stores and loads that move their arguments and results through the Go stack are 0.72% of the instructions of `r_react_ssr` and 1.25% to 1.38% of tifa's pages (arm64, from the same call counts: 0.75% to 1.49%). That is about 0.3% to 0.9% of the time, by 0.65 of the instructions or by the per-call saving of the phase 0 micro benchmark, under the 2% to 3% that the code position moves a render. The moves of all calls are 5.6% to 8.3%, but the rest goes to functions with a stack check (the argument block of `JS_CallInternal` 1.2% to 2.4%, functions on a cycle of calls 1.3% to 2.3%, calls through a pointer 0.5% to 1.0%). The 792 bytes the linker gives a chain of NOSPLIT calls are used (the tallest chain takes 752): frames smaller by 7.6 KB in all add no NOSPLIT function, and breaking each cycle of calls at one function adds 41 of them (the linker accepts it) but moves the share of moves to NOSPLIT callees by +0.2% to +0.3% on the shop and MUI pages and -0.14% on the bench pages. It would take a new kind of entry in the generator and in both post-link checks for 4 to 6 days, for a gain under the noise of the code position.
- **An absolute dispatch table** (C engine), filled once per VM with the addresses of the opcode handlers, so that a dispatch is one indirect jump instead of the 3 more instructions of the relative table llc emits for position-independent code: prototyped on 2026-10-27. The time did not change, `r_bench_table` got slower, and the table sat in writable memory, where a stray write could change where the interpreter jumps.
- **Dropping the no-ops after the prologue and the jump to the shared `RET`** (C engine): 1.6% to 2.6% of the instructions but under 1% of the time by estimate, since the no-ops are cheap. The no-ops are the `PCALIGN` that puts each body on 16 bytes, which keeps the alignment of its constant pools (`FuncPlace`) and its place for the code position measurements, and a Go epilogue at every return makes the code bigger.
- **Fewer arguments for `JS_CallInternal`** (C engine), prototyped on 2026-10-30. Its seven arguments (the context, the function, `this` and `new.target` as values, the count, the array of arguments and the flags) take 10 words, 11 with the shadow pointer, one more than the registers of an entry on amd64, so every call writes them into a block and passes its address. The prototype passed the address of the function in the caller's stack, where the interpreter keeps it under the arguments and above `this` of a method call, and read `this` and `new.target` below it when a flag said so: 6 words and no block, 10 to 14 fewer instructions a call. The engine then ran 0.77% fewer instructions a render of tifa's shop, 0.71% of its MUI pages, 0.93% of its bench pages and 0.85% of `r_react_ssr` (callgrind), under the 1% that would be worth timing at the code positions. The 6 words and the result still go through the Go stack, since the interpreter keeps its stack check, so packing the count and the flags into one word would save only about 2 more instructions a call. The prototype also made the shadow frame of the interpreter 96 bytes larger on amd64: the calls that LLVM inlines into it (`js_call`, `JS_CallFree`) each kept their own pair of the function and `this`, where they had all shared one block, and a recursion with 250 locals ended at 5978 levels instead of 6082 at the depth limit of 10000. The arguments of `js_call_c_function` (9 words, called through a pointer) were not tried: if none of them were moved at all, a render would run 0.27% (MUI) to 0.79% (bench) fewer instructions, 0.40% for `r_react_ssr`.
- **Making a call level take less Go stack**, researched on 2026-10-27. The design bound is 5,600 bytes for a call level (the longest chain of frames between two counts of the depth is 38), but real recursions use 416 to 2,224 bytes a level (a plain recursion 480). In `body` mode 71% of the bound is the same in every frame (the return address and frame pointer 608, the area for outgoing arguments 1,896, the area where registers are saved 1,480) and 29% is the locals LLVM keeps (1,616), and `preserve_none` moves the saves into the locals without making the total smaller. LLVM at `-O2` already reuses stack slots, and shrink-wrapping changes when a frame is built, not its size. Not done: passing the arguments of a call in the shadow frame with the shadow pointer in the closure register (the bound falls to 2,184, but it relies on how Go uses that register, which is no mechanism to depend on), packing the arguments of more than two words into a block (17% less, 2.5% to 2.9% slower), and counting the worst bytes of each stretch instead of 1 (it saves only memory and adds counts). Running the engine on a stack of its own, as cgo, wazero and purego do, would bring the Go stack a level takes to 0, but a fault in the C could then no longer be turned into a panic that ends only the VM, and it ends the process. That is not acceptable for a server, and the engine keeps its frames on the goroutine stack.
- **Clang PGO for the C engine**, measured on 2026-10-29. Instrumented builds trained on `r_react_ssr`, the 400 KB Markdown, 18 shop pages, 3 MUI pages and 2 bench pages. On the native C tree, 8 positions, 3 rounds: `r_react_ssr` took 1.096 of the time and the bench pages 1.083 (8 of 8 positions), the Markdown 0.962, the shop pages 0.987 and MUI 1.017 (both under the 2% the rule counts). A profile of only 4 shop pages did not make those 4 faster (1.000). On the generated engine (tifa's pages, 4 positions, 6 rounds) every page group was slower, the shop 1.058, the store 1.051, MUI 1.064 and the bench pages 1.150, the same sign at all 4 positions. The bench table page ran 32.5% more instructions: `JS_FreeValue` was no longer inlined (19.7% of its instructions as a call) and `JS_CallInternal` ran 22% to 23% more instructions with a frame of 640 bytes in place of 496. With the four inline thresholds put back to 400 it was still 29.4% more, so the thresholds are not all of the cause, and the rest was not found. The generator takes a profile without changes (a wrapped clang that adds `-fprofile-instr-use` when it makes the IR), and it makes the `.s` 22% larger (8.0 MB to 9.77 MB). It also found that the constant pools of the generated code are aligned within the function, not at their absolute address, so an aligned SSE read from a pool faults when the body of `JS_CallInternal` sits at 40 bytes of its line (the PGO build had 19 such reads). `JS_CallInternal` of main has none.
- **Values of 8 bytes** (NaN boxing, `CONFIG_NAN_BOXING64`), measured on 2026-10-30 on the native C tree built with the options of the generator, the same commit with and without the flag. Reads fell by 8% to 11%, writes by 12% to 17% and misses of the data cache by 13% to 20%, but the instructions rose on every load: the shop's workbench 2.6%, the store 2.8%, MUI 4.8% and `r_react_ssr` 6.9%. The tags, the pointer mask and constants such as `undefined` are 64-bit numbers that x86-64 compares and masks only from a register, so `movabs` went from 580 to 9,310 in `quickjs.o`, `JS_CallInternal` grew by 16%, and the instruction cache missed 8% to 12% more. `JS_FreeValue` was no longer inlined, which is 70% of what `r_react_ssr` added, and forcing it inline made the four loads 2% to 11% slower. The time did not change (median of 8 positions 0.983 to 1.008), under the 3% at every position that the second step, a property of 8 bytes, needed to go on. Worth looking at again only if the memory of the VMs, not the time of a render, becomes the limit.
- **Cheaper polls**, after the cost was measured (see [What the generated code costs against native C](#what-the-generated-code-costs-against-native-c)). The call of `host_poll` is not the cost: with the interval at 1,000, 10,000 and 100,000 polls a script took the same time, and `host_poll` and the Go runtime are 0.2% to 1.1% of the added instructions (0.06% to 0.16% of a tifa render). The cost is in the counting: the registers it holds, the count that is written back before and read after each call, and bigger code. Taking the counts out of the dispatch saved only 6% of the instructions, and taking away as well the register the function held for the count and the sync around its calls still left a `jsloop` at 1.41 times the instructions of an engine without polls. What was done is in [Polls and interrupts](changes/c-engine.md#polls-and-interrupts): counts only at jumps back and at the entry of a function, loops that run in chunks and count once a chunk, a count in a register or in memory for each loop, and no more `llc` options for tail duplication. What was not done:
  - **A fixed register for the count in all the code (H2).** It can be made correct under Go, but x86 would need LLVM 23, and OCaml found the count in a virtual register mostly spilled and slower, and DTVM went back to ordinary registers in its optimizing mode.
  - **Cutting the limit of a loop into chunks as HotSpot does (M1).** It works only for loops whose trip count the compiler can work out. Of the scripts it would speed up, `strsearch` 5% to 10% and React about 0, and a mistake in the arithmetic of the limit is an out of range access.
  - **Giving the small functions whose only call is `host_poll` their `NOSPLIT` back (L3).** With the counts of all the C functions taken out the ceiling is `json` 15%, `strsearch` 11%, React 4% and `r_react_ssr` 0%, so under 0.5% on a page.
  - **The `llc` options for tail duplication for the interpreter alone (M2).** Without them only `JS_CallInternal` changes in the whole engine, so giving them to it alone gives the same bytes. They are taken out since, which cost `jsloop` 20% before the counts moved to the jumps back and made it faster after.
  - **Counting nothing in short loops of the C (the shape hash chains, `lre_find`, the property loop of `free_object`).** It needs the C to promise the bound, which the generator cannot check, the same weak spot that put an earlier idea out.
  - **Skipping the targets that cannot poll in a call through a pointer (L4)**, from the type of the pointer: types only tell the number and kind of the arguments, so a comparison function that may call JavaScript shares its type with the ones that cannot.
  - `taindexof` 25% slower with polls turned out to be where the code landed: its inner loop of 7 instructions across a line of 64 bytes took 22% to 25% longer, with and without polls.
- **A comparison and the conditional jump after it as one opcode**, measured on 2026-10-31 on top of the dropped checks of the temporal dead zone. `fuse_code` turned `strict_eq`, `lt`, `is_null` and `is_undefined` followed by `if_false`, `if_true`, `if_false8` or `if_true8` into `strict_eq_if`, `lt_if`, `is_null_if` and `is_undefined_if`, which compare and jump in one dispatch, unless a jump lands on the jump or the comparison is inside a case of a `switch` already joined into `dup_case_atom`. These four are 127K of the 182K comparisons followed by a jump in a round of the six workbench pages (`===` 48K, `=== null` 45K, `=== undefined` 21K, `<` 14K, then `typeof` 18K, `!=` 9K and `!==` 8K), and the dispatches fell by 3.3% there, 2.9% on the store pages, 1.5% on MUI and 3.3% on the bench pages. But a dispatch saved is about 17 instructions, so the workbench pages ran only 0.26% fewer instructions a render, the store pages 0.34%, MUI 0.22%, the bench pages 0.28% and `r_react_ssr` 0.86% (callgrind, the same HTML on the 16 pages), against the 0.7% on the workbench that it had to reach. Joining every comparison followed by a jump (`!==` by turning the jump around, and `typeof`, `==`, `!=` and `>=` in the last free opcode numbers) would add about 0.1%. It would also take the last free opcode numbers below 256.
- **A `switch` on strings dispatched by a lookup of the atom in a table**, as Hermes does for 10 cases or more, estimated on 2026-10-31. A round of the six workbench pages compares 38K string cases: 26.5K of them as `dup_case_atom`, about 25 instructions each with its dispatch, and 11.7K of them still as `dup` `push_atom_value` `strict_eq` and a jump, the shape of cases that share a body, which jump with `if_true`. That is about 0.2% of the instructions of a render, under the 0.3% where it is worth it, since tifa's Vite plugin already puts a `Set.has` test (`guardSwitches`) in front of the long switches of React DOM. React DOM without the plugin compares 83K cases a render (30K as `dup_case_atom`, 53K in the `if_true` shape), about 9% of the instructions of a render of `r_react_ssr`: the table would pay for libraries the plugin does not cover, or once `guardSwitches` is removed. `dup_case_atom` now takes the `if_true` shape too and runs a chain of cases in one dispatch ([Opcodes that do a few instructions at once](#opcodes-that-do-a-few-instructions-at-once)), which leaves a table less to gain.
- **A comparison with a string literal as one opcode** (`strict_eq_atom`, `strict_neq_atom`, `strict_eq_loc`, `strict_neq_loc`), measured on 2026-10-31 on top of v0.29.6 (c5efd5d8). `x === "lit"` and `x !== "lit"`, with the literal on either side, compared the atom of the string in place, a local too, and the case of a `switch` on a string used it. A round of the six workbench pages ran 26.6K `push_atom_value` `strict_eq` and 19.5K `push_atom_value` `strict_neq`, 26.5K of them right after a `get_loc`. The instructions of a render fell on `r_react_ssr` (2.29%) and the workbench pages (0.10%) and rose on the store pages (0.19%) and the MUI pages (0.15%), callgrind on linux/amd64 at position 0.0 with the same HTML, where LLVM laid the interpreter out again for four more cases. The time rose all the same: on the generated build at 8 code positions a render of `r_react_ssr` took 1.026 of the time (1.009 to 1.047, 3 rounds), a run of it that compiles the bundle and renders once 1.012, and the six workbench pages 1.022 (one round). Callgrind's simulation of the caches and branches on a render of `r_react_ssr` found 8.4% fewer indirect jumps (the dispatches) with as many missed, 4.4% more missed conditional branches and 4.1% more misses of the instruction cache: at 15 to 20 cycles a missed branch, about the 2.6% lost. The opcodes took 4 of the 5 free numbers below 256 and the bytecode version. Not taken.
- **`return x` of a local or an argument as one opcode that moves the value out** (`return_loc`), measured on 2026-10-31 on top of the comparison with a string literal in one opcode above. It was made only where nothing could read the variable after the return (no capture, no direct `eval`, no `with`, no mapped `arguments`, not a generator or an async function, not inside a `finally`), and saved one reference taken and one freed: a round of the six workbench pages returns an argument 14.6K times this way and a local 6.5K times. A render ran more instructions on every app all the same: 0.13% on the workbench pages, 0.10% on the store, 0.73% on MUI and 0.55% on a render of `r_react_ssr`, callgrind on linux/amd64. The opcode took the last free number below 256, and the cost is in `JS_CallInternal` as LLVM lays it out with one more case, more than the few instructions a return saves.
- **Running the conditional jump after a comparison in the same dispatch**, measured on 2026-11-10. A comparison or a test that gives a boolean (`<`, `<=`, `>`, `>=` of integers, the fast equalities, the tests of `undefined`, `null` and `typeof`, `lnot`) looked at the next instruction and branched there when it was `if_false8`, `if_true8`, `if_false`, `if_true` or the `dup`, jump and `drop` of `&&` and `||`, poll included, with no new opcode. It took 7.1% to 9.8% of the dispatches off a render of tifa's shop, MUI and bench pages and 11.6% off React DOM's, but no instructions: callgrind on the generated engine gave 0.42% to 0.58% more on the three apps, and the native C tree 0.4% more on React. Counted by the line of the C (native, clang `-g`), each dispatch it took away saved about 5 instructions in the jump cases (if_false8, if_false, if_true8, if_true and dup: 0.95 M fewer a render of the bench pages of an older build of tifa), the comparisons gained 0.40 M on React for the look at the next instruction, and cases it did not touch got dearer: `call_method` 0.33 M more on the bench pages and 0.10 M on React, `call` and `js_call_c_direct_ok` 0.07 M each. A dispatch is a few instructions in this interpreter, and a change to its code moves the registers and the layout LLVM gives the whole of `JS_CallInternal`, which costs as much as the dispatches saved.
- **`!0` and `!1` folded into `true` and `false` by the compiler**, measured on top of the jumps above: React DOM's bundle takes 1.0% fewer dispatches and 0.4% fewer instructions natively, but the dispatches of tifa's pages do not change at all, so it gains nothing there.
- **Reading a local without a reference when the next instruction only reads it** (borrowing, as the fused reads of `get_field` do), estimated on 2026-11-10 and not built. After the cases of a `switch` were chained, a read of a local, an argument or a closure variable followed by an instruction that only reads it (`typeof_is`, `get_array_el`, `get_length`, `lt`, `is_null`, `strict_eq` and a few more) is 3.6% of the dispatches of a render of the bench pages of an older build of tifa and 5.0% of React DOM's, spread over many pairs: the largest pair is under 0.8% of the dispatches, and the largest pairs of the two bundles are not the same. Each pair needs an opcode of its own and only 5 numbers are free, and a look at the next instruction in every read would cost every read. With a dispatch at about 5 instructions and the dup and free at about 9, it would save about 1% at best, under the noise of a render.

## Where to continue

What the measurements above leave for the C engine, none of it large. A change under about 3% needs the positions of [Code positions](#code-positions), and callgrind below that.

- **The interpreter function** is 52.7% of the instructions of tifa's pages (15 pages, 4,147 million instructions a render on average) and the next functions are far behind: `add_property` 4.9%, `free_object` 2.8%, `js_call_c_function` 2.2%, `JS_NewObjectFromShape` 1.7%, `JS_GetPropertyInternal` 1.7%, `lre_find` 1.6%.
- **The entry of a JavaScript call** is about 95 of the 190 native instructions a call takes, and 73,000 calls make a workbench render. The loop that sets the locals to `undefined` is 2.1% of the interpreter's native instructions. Smaller entries have been tried ([Calls, for in and hasOwnProperty.call](#calls-for-in-and-hasownpropertycall), and fewer arguments above).
- **Adding a property** (`add_property`, 4.9% of the instructions and 8.6% of the last level misses) is the largest piece of property access that is left. Object literals now skip it for plain keys ([Object literals made at their final shape](#object-literals-made-at-their-final-shape)).
- **The translation** costs 11% to 13% and the polls 2% to 7%. What is left of the causes is the design (locals on the shadow stack) or tried above.
- **The place of the register loop** `JS_RegvmRun` in its line of 64 bytes is not measured yet. Until it is, its body sits after a `PCALIGN $16` like any other function and moves with the code linked before it.
- **Memory** per VM: the free pages of the render's temporary space, the pages the shadow stack touched and the holes between free blocks make up the gap between a VM's resident memory and the engine's count of its blocks (4.2 MiB on the shop and 2.9 on MUI per VM, from 8 VMs against 1 after the renders). `ReleaseMemory` gives back the free pages when the pool lets a VM wait.
