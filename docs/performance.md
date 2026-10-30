---
summary: 'How much faster each release of slowjs renders pages, how much faster slowjs is than the engine it started from, how to measure an engine change in tifa and across code positions, where the C engine puts its hot loops and what its latest changes did to its speed, what the C engine's string appends gained, what ordinary objects of 48 bytes gained and cost, what object literals made at their final shape gained and cost, what React DOM's HTML escape written in C gained, what the loop that copies the props of a React element written in C gained, what calls that set and free their values in fewer turns, for in loops that hold their shape and hasOwnProperty.call answered in the interpreter gained, what a new VM and the load of a bundle cost, how fast and how large tifa's pages are on the C engine against the ccgo engine and what tifa's wiring gained, what the C engine had to pass before the switch and how it did, what the native C patch costs, what the generated code costs against native C and where a render's time goes on the C engine, which engine ideas were tried and dropped and why, and where to continue, and how the C engine holds up under load (a soak, several VMs, Go GC pauses, Interrupt and the kill)'
read_when:
  - Making a change to the engine for speed, or reviewing one
  - Measuring a branch of slowjs on tifa's pages before a release
  - A speed result of a few percent, which may only be where the code landed
  - About to try an engine idea that may already have been tried
  - Planning the next round of work on render speed
  - Asking what RegisterEscapeText gains in tifa's pages, or what a cache in front of it would give
  - Asking what RegisterCopyProps gains in tifa's pages, or whether a check in JavaScript in front of a spread would do
  - Quoting how fast a release is
  - Checking that the C engine stays flat, scales over VMs, stops soon after Interrupt and holds the GC up for no longer than the ccgo engine
  - Checking what the C engine had to pass before it replaced the ccgo engine, and what it measured then
  - Asking how much slower the generated engine is than native C, or what a render spends its time on
  - About to change how the polls are counted, or to try PGO on the C engine
---

# Performance

slowjs is made for tifa, which renders React pages to HTML in the engine, so render speed is measured on the pages of tifa's example shop: one warm VM rendering the order list, an order and the stats of each of its two shops (`BenchmarkShop` in tifa's `test/ssr`). Every change for speed leaves the HTML the same byte for byte. The doc of each change in [changes](changes/README.md) has its full measurement, and tifa's [docs/performance.md](https://github.com/liustack/tifa/blob/main/docs/performance.md) has what the numbers mean for an app. How the engine compares with the other pure Go engines, on tifa's pages and on general workloads, is in [benchmarks.md](benchmarks.md).

## How the numbers were measured

Unless a row says otherwise:

- Machine: a Google Cloud VM with 8 vCPUs of an AMD EPYC 7B13 (Zen 3), Linux, one core pinned with its hyperthread sibling idle. Some rows are on an Apple M4 (darwin/arm64), which is noisier.
- The build before and the build after a change, alternated round after round in one session. Only ratios from the same rounds compare.
- With PGO means tifa built with an engine profile collected on that same engine. A profile collected on an older engine matches fewer call sites (see [Measuring in tifa](#measuring-in-tifa)).

## What each release gained

The rows up to v0.28 were measured on the engine of those releases, QuickJS translated to Go by ccgo, which the C engine replaced. The docs they linked for the Go form of a change are gone, and their names stay as text. The speed of the C engine is in [The C engine](#the-c-engine) below.

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

### Against the engine slowjs started from

Measured in tifa on 2026-10-20 against `modernc.org/quickjs` v0.25.0 with libquickjs v0.13.1, on the same SSR bundle and host program (tifa's [docs/studies/moejs.md](https://github.com/liustack/tifa/blob/main/docs/studies/moejs.md)): the engine changes up to v0.26.15 alone make renders 1.63 to 1.66 times as fast. With the rewrites of tifa's Vite plugin and PGO, pages render 2.07 times as fast as on the original engine with neither.

## Measuring in tifa

To measure a branch of slowjs before it is released, point tifa at the checkout with a `replace`, build the example app, and run the benchmarks of tifa's [docs/testing.md](https://github.com/liustack/tifa/blob/main/docs/testing.md#performance-comparisons):

```sh
# in tifa
go mod edit -replace github.com/liustack/slowjs=../quickjs
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

### Code positions

Where the interpreter's code lands in the binary changes its speed by 5% to 14% on its own, so one build against one build can show a change that is only a move. Measure each side at several positions.

Go puts amd64 functions on 32 bytes, so moving the whole program by less than 32 bytes moves nothing, and the effect repeats every 64 bytes. A position is two numbers:

- The whole program moved by 0 or 32 bytes.
- The start of the interpreter body moved by 0, 8, 16 or 24 bytes of NOP, put right after the `PCALIGN` at its start.

All 8 combinations are the full set. For a quick check use 4: 0 or 32 for the first, 0 or 16 for the second. Build every position with the same flags (`-pgo=off` or the same profile), and put a measurement of the same engine at all positions next to the one of the change.

Making the positions:

- The tool in [testing.md](testing.md#developer-tools-generate-check-time) makes the positions of the C engine, runs the builds in turn and applies the rules below (`bench build`, `bench run`, `bench report`), and counts instructions (`bench cg`). By hand: C engine (`internal/cengine`): copy the generated `*_amd64.s` files, run `go run ./internal/goasm/cmd/xform DIR shift:32 front:JS_CallInternal_block:16` on the copy (use `shift:0` and `front:...:0` for no move), and build with the copy. The bytes of `shift` sit after a `RET` and never run. The bytes of `front` sit after the `PCALIGN` at the start of the interpreter body and run once per call.

Reading the result:

- For each position, run the two builds in turn and take the median of the paired ratios. Report that median per position.
- Take the range of the baseline's own positions: the same engine, new against itself at each position.
- A change is real only when the median passes both 2% and half of that range, and most positions agree on the sign.
- Under 3%, do not argue from timings. Count instructions with callgrind, which hardly move with the position.

The 8 and 24 bytes of `front` move the body and the constant pool it keeps after its code together, so the pool leaves its 16 bytes. The interpreter as built reads its pool with scalar instructions only (`movsd`, `ucomisd`), which need no alignment, so this does no harm today. A build in which it reads the pool with an instruction that needs 16 bytes, as a profile-guided one did in 19 places, faults at those positions. Make those two places with the generator instead: `-align64 JS_CallInternal:48` and `-align64 JS_CallInternal:0` put the body there with its pool on 16 bytes.

### The C engine's hot loops on fixed places

The generator puts the body of the C engine's interpreter at 40 bytes into a line of 64 bytes (0 until a JSValue was copied in two moves of 8 bytes, see [below](#the-c-engine-copies-a-jsvalue-in-two-moves-of-8-bytes)) and the body of its regexp matcher at 16 (`-align64`, see [c-engine.md](changes/c-engine.md#where-the-interpreter-and-the-regexp-matcher-sit)). Before, each body sat after a `PCALIGN $16` and moved within its lines with any change to code linked before it.

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

Measured on 2026-10-22, using the pinned Bellard source `3d5e064e9dd67c70f7962836505a7fa067bf0a4e` and slowjs at `620658c`. The old C patch unconditionally set `DIRECT_DISPATCH` to zero. That translation setting predates slowjs's exception changes. Native C therefore used a central switch instead of upstream's computed goto dispatch.

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

## The C engine

The C engine of `internal/cengine` is the only engine since the switch. Its numbers come from the `jsrun` binary on the Google Cloud VM above, one core pinned with its hyperthread sibling idle, at the 8 code positions of [Code positions](#code-positions), and from callgrind.

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

An object literal of plain keys [gets all its keys at once](changes/objects-and-shapes.md#object-literals-made-at-their-final-shape). Against main (dfd2caee, with the ordinary objects of 48 bytes), without PGO, over 4 rounds at 8 positions on one core of an EPYC 7B13 with its sibling idle, with the same HTML. Time is the median over positions of the paired ratios, and instructions per render are from callgrind at position 0.0.

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

An object literal [keeps its properties in the block of the object](changes/objects-and-shapes.md#object-literals-keep-their-properties-in-the-block-of-the-object). It saves 0.1% to 0.8% of the instructions of a render and leaves the time of every page inside what the rule counts as no change. Against main (a9d61185), without PGO, over 2 rounds at 8 positions on one core of an EPYC 7B13 with its sibling idle, with the same HTML. Time is the median over positions of the paired ratios and range the base's own range over positions. Instructions per render are from callgrind at position 0.0 (the difference of two runs with different numbers of renders).

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

Two changes, measured one on top of the other: [a call sets and frees its values in fewer loop turns](changes/c-engine.md#how-a-call-sets-and-frees-its-values), then [a for in loop holds the shape of its object](changes/objects-and-shapes.md#a-for-in-loop-holds-the-shape-of-its-object) together with [hasOwnProperty.call answered in the interpreter](changes/objects-and-shapes.md#hasownpropertycall-answered-in-the-interpreter). Against main (9fd4bdb0, with object literals made at their final shape), without PGO, over 4 rounds at 8 positions on one core of an EPYC 7B13 with its sibling idle, with the same HTML. Time is the median over positions of the paired ratios, and positions gives the lowest and the highest of them. Instructions per render are from callgrind at position 0.0: the difference of 12 and 4 renders of each page for the shop, of 3 and 1 for the MUI pages and the scripts. The bench pages are counted on `r_bench_table` and `r_bench_spiral`, which render the same pages.

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

### React DOM's text escape in C

[`VM.RegisterEscapeText`](changes/go-api.md#registerescapetext) gives a host React DOM's `escapeTextForBrowser` as a C function of the engine. tifa registers it as `__tifa_escape`, and its Vite plugin makes React's string renderer call it where the renderer called its JavaScript function, which tifa had given a cache of escapes and five `replaceAll` calls in place of React's loop. The step before is slowjs main (4cdd3a27) with tifa main (363b6b1). The step after is this branch with the tifa branch that uses it. The HTML of the 16 pages (the six workbench pages, the five storefront pages, the three MUI pages and the two bench pages) is the same byte for byte, with the render time stamp masked.

On linux/amd64 (one core of an EPYC 7B13, its sibling idle), without PGO, 3 rounds at the 8 code positions. Time is the sum of the p50 of the pages of an app, as the median over positions of the paired ratios. Instructions are callgrind's, per render of every page, at position 0.0 (the difference of a run of 5 renders and one of 2):

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

[`VM.RegisterCopyProps`](changes/go-api.md#registercopyprops) gives a host the loop with which React's `createElement` and `cloneElement` copy the keys of their config into the props of an element, as a C function of the engine. tifa registers it as `__tifa_copy_props`, and its Vite plugin makes both functions ask it before their loop. The step before is slowjs main (a5feface) with tifa main (4b511fc). The step after is this branch with the tifa branch that uses it. The HTML of the 16 pages (the six workbench pages, the five storefront pages, the three MUI pages and the two bench pages) is the same byte for byte, with the render time stamp masked.

How often it is asked, from one render of each page in tifa with a wrapper that counted its answers: the six workbench pages make 980 elements with `createElement` and 518 with `cloneElement`, the five storefront pages 2,507 and 328, the three MUI pages 3,266 and 114, and each bench page 1. The function copies all of them but one config of 39 keys (MUI's input, on the order list and on an order page), which is over the 32 keys it takes and goes through the loop.

On linux/amd64 (one core of an EPYC 7B13, its sibling idle), without PGO, 3 rounds at the 8 code positions. Time is the sum of the p50 of the pages of an app, as the median over positions of the paired ratios. Instructions are callgrind's, per render of every page, at position 0.0 (the difference of a run of 5 renders and one of 2). "Same engine, old bundle" is a control: the engine of this branch with the bundle tifa main builds, which never asks for the function:

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

### Upstream commits and the scripts where the C engine was slower than C

[upstream-commits.md](changes/upstream-commits.md) lists the commits of upstream QuickJS taken into the C tree and the ones tried and taken back, with their speed. Against main (`c4dcea91`) before the batch, all 37 scripts at 8 positions, 2 rounds: geometric mean 0.917, with `q_array_slice` at 0.085 and `c_array_methods` at 0.530 and the other 35 scripts together at 0.997. `r_react_ssr` took 0.977, within its noise.

Where the C engine was slower than the C QuickJS builds, the same C of `engine/` built with clang -O2 for the machine (the objects of `engine/test` and a runner like `jsrun`) shows where the time goes. Time as a ratio to the C engine as Go assembly (fin2, before the search change below), median of 5 rounds, the assembly at its 8 positions:

| Script | C engine, ms | same C, clang | pinned 3d5e064, clang | pinned, gcc | latest 19d6bd8, clang | latest, gcc |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `c_array_methods` | 395.3 | 0.744 | 0.813 | 0.815 | 0.719 | 0.701 |
| `c_string_methods` | 358.3 | 0.785 | 0.796 | 0.880 | 0.809 | 0.874 |
| `q_regexp_ascii` | 376.0 | 0.829 | 0.765 | 0.777 | 0.746 | 0.774 |
| `r_react_ssr` | 386.6 | 0.940 | 1.073 | 1.062 | 1.031 | 1.049 |

slowjs's C changes cost nothing on the first two (the same C is as fast as the pinned QuickJS or faster), so what is left is the cost of the assembly: 20% to 30% on these three scripts, 6% on `r_react_ssr`. On `q_regexp_ascii` the same C is 8% slower than the pinned QuickJS (0.829 against 0.765 to 0.777). The cause was not looked for: the native host also calls `host_poll` (2% of the samples). perf (4999 Hz, samples of the same 20 runs, the assembly against the same C):

- `c_array_methods`: `js_array_includes` 6020 samples against 1826, `js_array_indexOf` 3915 against 957, `JS_CallInternal` 10743 against 8749, the rest alike (`js_strict_eq2` 13630 against 13231). The loops called `js_strict_eq2` for each element, and a loop with a call that may poll keeps its poll counter in memory, which is a read, decrement and write of one word per turn: 36% of the samples of `js_array_includes` sit on that decrement. Fixed by [deciding most elements without a call](changes/upstream-commits.md#indexof-and-includes-decide-most-elements-without-a-call): the script took 0.583 of the time.
- `c_string_methods`: 38900 samples against 30440, spread over many small functions: `JS_CallInternal` +940, `js_call_c_function` 1964 against 1210, `JS_ToStringInternal` 1726 against 1122, `JS_ConcatString` 1182 against 658, `js_string_includes` 967 against 523. No loop with one hot instruction. It is the cost of each call of the generated code (arguments and results through memory, a stack check, the poll counter).
- `q_regexp_ascii`: 39700 samples against 33500. The matcher `lre_exec_backtrack` is alike (10390 against 10002) and so is `JS_CallInternal`. The extra samples are in what surrounds it: `js_regexp_exec` 3582 against 2374, `lre_find` 1540 against 615, `lre_exec` 2367 against 1660, `free_object` 2440 against 1650. Calls again.

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

### Where a render's time goes on the C engine

From callgrind on the generated engine (amd64), with cycles estimated as instructions plus 10 for each L1 miss plus 40 for each last level miss (last level 512 KB, as on Zen 3's L2). Each page is a run of renders less a run of fewer, so loading and warm-up cancel out. The lines of `JS_CallInternal` were split by a native build with debug lines on the same pages. Measured on 2026-10-29 on slowjs `6c757279`, before the objects of 40 bytes and the single scan of shared bytecode for polls.

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
- Against Node with `--jitless`, slowjs took 1.82 times as long on the shop workbench, 1.67 on the store and 1.52 on MUI. The JavaScript functions with the highest ratio are small ones (3 to 5 times, since the cost is the fixed price of a call), React's `for in` with `hasOwnProperty.call` (2.0 to 2.6 times, done since: [Calls, for in and hasOwnProperty.call](#calls-for-in-and-hasownpropertycall) and [The copy of the props of a React element in C](#the-copy-of-the-props-of-a-react-element-in-c)), and functions where a reference count reaching 0 frees a whole chain at the assignment, where Node leaves it to its collector (`resetHooksState` 3.5 to 4.9 times).

### Stability under load

Measured on the Google Cloud VM above (Go 1.27.2), with the 18 pages of tifa's example shop (its 6 workbench pages and 6 storefront pages in each language) rendered through tifa's pool, the C engine against the engine of `lib/`, built from the same tree. The cores are pinned: `1,3` are two physical cores, `1,3,5,7` are those two with their hyperthreads. The pool keeps tifa's rules: any error retires the VM, a young GC runs after each render and a full one every 256.

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

What decides the time is of two kinds. A loop that does not read the flag runs to its end or to the kill: `fill`, `join`, `normalize` and the global `replace` that matches in C were such, and read it now every 4096, 1024, 4096 characters and each match. After an error the call also frees what it made, and that has no flag to read: the 300,000 objects of a `JSON.parse` take 36 ms to free (the engine of `lib/`: 58 ms), and so would the 33 million strings of a `split` of 64 Mi characters and the 16 million arrays that a generic `replace` of 32 Mi characters holds until it ends (not timed apart). A script that builds more than about 400,000 objects in one call can be killed while it frees them. The stop of a `JSON.parse` loop follows the size of the tree: 1.8 ms for 10,000 objects, 5.5 ms for 30,000, 20 ms for 100,000, and the kill at 600,000 (lib: 12.5 ms, 38 ms, 145 ms, 1.4 s). The VM is gone either way, and the pool replaces it. `JSON.stringify` reads the flag at every value, and its 2.5 to 4 ms are the free of the half-written string (1.7 ms for 25 MB) and the allocation of the error. Multiplying or powering BigInts of hundreds of thousands of bits reads no flag in libbf: it runs to the kill.

`split`, `isWellFormed`, `toWellFormed`, `padStart`, `padEnd`, `trim`, the attribute of `anchor` and the other HTML methods, `escape`, `unescape`, `encodeURI`, `decodeURI`, `Array.from`, the typed array constructors, making a rope flat and `<` on two ropes read the flag now too (see [c-engine.md](changes/c-engine.md#polls-and-interrupts)). The `split` row of the table above is the scenario `string_repeat_split` (4 Mi characters in 2 Mi pieces) on the 20 stops of `interrupt`: the median was 67.8 ms and the highest 243 ms, and it is 23.3 ms and 39.7 ms now (core 5 of an EPYC 7B13, no VM killed in either run). What is left is the free of the pieces made so far. A stop of `array_join` and `normalize` did not change (4.3 and 4.1 ms, then 3.5 and 4.1 ms). The new loops cost the pages no more than the noise of the Go runtime in a callgrind run: per render, `r_react_ssr` runs 0.06% more instructions (2,511.2 M, then 2,512.8 M), the six workbench pages of the shop 0.18% more (978.1 M, then 979.9 M, of which the Go collector's work in the two runs is more than the difference), the four MUI pages 0.06% more (751.2 M, then 751.6 M) and the bench pages 0.09% more (309.9 M, then 310.2 M), against main (4cdd3a27), without PGO.

**Timeouts and kills in the pool.** 4 VMs, 3 callers rendering all the time, 300 ms timeout. A fourth caller sends a request every second that loops for ever (`loop`), one that sleeps 1.5 s in a Go function it called (`sleep`), or both in turn. Two runs of 20 seconds each, cores 1,3,5,7, measured on v0.29.0, when the first poll after the sleep was past the grace and killed:

| | C engine good renders/run | p50 | p99 | bad requests | killed | lib good renders/run | p50 | p99 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| No bad requests | 2164 | 22.2 ms | 100.2 ms | | | 1752 | 27.2 ms | 132.0 ms |
| `loop` | 1928 | 25.1 ms | 116.9 ms | 50, all failed | 0 | 1609 | 30.1 ms | 140.2 ms |
| `sleep` | 2040 | 23.2 ms | 103.7 ms | 34, all failed | 34 | 1688 | 28.4 ms | 127.8 ms |
| both | 1947 | 24.3 ms | 112.8 ms | 49, all failed | 24 | 1633 | 28.7 ms | 130.5 ms |

Every bad request failed with a deadline error, every one got a new VM, no good render failed, and all 4 VMs were back in the pool at the end. Killed VMs were replaced like timed out ones. Since a kill also needs 1000 polls of work after the request (`gohost.KillPolls`), the sleep kills nothing: the script reads the flag as soon as the Go function returns and fails with the same deadline error, and the VM is replaced as a timed out one. The killed column of `sleep` and `both` would be 0. The table was not measured again. The good renders' p99 grew 3% to 17% on the C engine and by at most 6% on lib, the cost of loading a replacement VM while the others render. The C engine's p99 stays under lib's in every row.

### tifa's pages on the C engine

Measured with tifa's `test/ssr` on the Google Cloud VM above, one core pinned with its hyperthread sibling idle: the six workbench pages of `examples/shop` (the pages of `BenchmarkShop`), the three pages of `test/apps/mui` and the two of `test/apps/bench`. Each page renders in a warm pool of one VM with `GOMAXPROCS=1`, 64 renders to warm up, then the P50 of 100 renders (60 on the MUI pages). 3 rounds at the 8 [code positions](#code-positions). The engine of `lib/` is built without PGO and with a profile of 500 renders of the shop. A ratio is the median over positions of the paired ratios, and the C engine was faster at all 8 positions on every page.

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

### Acceptance before the switch

Run on 2026-10-29 at slowjs `75052b72` and tifa `41dcf45` (branch `phase5/accept`: tifa upgraded to that slowjs, with its engine profile collected again), on the Google Cloud VM above. The C engine is built without PGO. Where it is compared with the engine of `lib/`, that one is built with the tifa engine profile.

| Gate | Result |
| --- | --- |
| test262 as before | Native C tree: the same 58 known errors, line for line (58 of 83,426). Go host: 80,215 of 96,540 runs, the number of the v0.27.2 row of [benchmarks.md](benchmarks.md), the same in every area, in ES2023 and older, and under every feature tag. 0 crashes, timeouts and out of memory |
| tifa's test pages, same HTML | shop 160 pages, pages 33, tailwind 2, mui 60, bench 4, extreme 30: all the same, except `/json/2001` of extreme, which the C engine answers with a 500 (its JSON nests at most 2,000 levels, as tifa's docs say). `api` has no pages. tifa's `go test ./...` passes on the default build, and on the C engine all but one test of `internal/ssr` (below). The end-to-end tests pass on both |
| No page slower than on `lib/` with PGO | All 11 pages faster at all 8 code positions: 0.834 to 0.860 of the time |
| GC pause P99 | Not above the stored `lib/` numbers: 1 VM 131 to 164 µs against 164, 3 VMs 197 µs against 197 |
| Soak | 150 minutes, 211,076 renders, 0 errors, no VM replaced. The memory of the VMs and what they took from the system did not move. The process grew 3.8 MB between minutes 30 to 60 and minutes 120 to 150 |

**Same HTML.** Every page of a crawl of the built program, the default build against the C engine, byte for byte after masking the times the loaders write into a page (RFC 3339 text and the `u:` milliseconds of the router's state). The default build against itself, crawled twice, masks exactly as many (shop 5,169), so the masked bytes are not the engine's. Extreme was crawled with `--ssr-timeout 60s`, as tifa's end-to-end test does, because `/console/100` takes 1.4 to 3.5 s on `lib/` and a default 3 s timeout turned it into a 500 in one crawl, depending on the load. The shop uses `TestShopCrawl` of tifa, the other apps a small crawler of the same kind.

**One failing test on the C engine.** `TestFullGCGivesBackWhatTheThresholdNoLongerNeeds` in tifa's `internal/ssr` fails the same way each time and on darwin/arm64 too: after 1,000 renders full GCs gave back 68,688 bytes where it wants at least half the live size, 112,008. The VM there has a live size of 224,016 bytes and a footprint of one 262,144-byte chunk. It skips on `lib/`, which takes its memory from libc, and the pin tifa had before had no engine to run it on. The test next to it passes.

**Speed.** Median over 4 rounds of the paired ratio C engine / `lib/` with PGO, at the 8 [code positions](#code-positions), `GOMAXPROCS=1` on one core with its sibling idle. The `range` column of the tool (the positions of `lib/` against each other) is 1.4% to 4.2% on the pages. No process had to be run again. Another test was running on the two cores next to it the whole time.

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

**GC pauses.** The method of the [stability batch](#stability-under-load): a goroutine allocates 512 MB a second, so the Go GC runs 65 to 86 times a second, while the pool renders the 18 pages for 60 seconds after 10 of warm-up. 1 VM on cores 1,2 (two physical cores) and 3 VMs on cores 1,5,2,6, 5 rounds each. The `lib/` numbers are the single runs of the stability batch.

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

## What was tried and not done

Each item says what it would give and why it was dropped. Check here before trying one of them again. The numbers come from tifa's [docs/performance.md](https://github.com/liustack/tifa/blob/main/docs/performance.md#what-was-tried-and-not-done) and [docs/studies/engine-hotspots.md](https://github.com/liustack/tifa/blob/main/docs/studies/engine-hotspots.md), and from the docs of the changes.

- **Removing the TLS frame of 135 more small functions** (a TLS frame is the part of libc's TLS stack where the Go translation keeps a function's C locals whose address is taken): about 0.24% of render time, too small to measure, for 135 more entries to maintain.
- **Letting more than 768 bytes of a call's room live in its TLS frame**: 1024 bytes covers 0.02% more calls, but doubles the blocks Linux keeps per call depth from 2 KB to 4 KB.
- **Keeping only 256 KiB of TLS stack per VM outside linux**: not enough. 10 VMs on the Apple M4 rendered 315 pages a second with 4.2% lock wait, against 437.8 to 481.6 with 1 MiB, since renders reach deeper into the TLS stack.
- **Moving rarely used opcodes out of the interpreter loop**: at most 8% alone on amd64, an upper bound measured by replacing the 56 handlers of opcodes the shop never runs with empty stubs, but only 1.4% on top of the rewrite that keeps 15 values in their stack slots, and 0.5% on arm64. It would need the loop's locals moved into a struct.
- **Moving small blocks in realloc without going through malloc and free**: 0.23% fewer instructions on the studio list on top of js_malloc taking cached blocks, too small for one more fixlib rule.
- **Keeping 1024 blocks a size in the front cache instead of 256**: misses on the studio list go from 19.7k to 18.6k a render. They come from the heap growing during a render, not from full lists.
- **Dispatching opcodes through closures or a table of functions**, as some Go interpreters do: a rewrite of the interpreter, against the rule of slowjs to make only mechanical changes. Not tried.
- **A check in JavaScript in front of one spread for the props of `createElement` and `cloneElement`**, measured on v0.29.1 before `RegisterCopyProps`: the plugin tested the config (the prototype, the symbols, `__proto__`, `__self` and `__source`, a getter behind `key`) and then copied it with the rest of a destructuring. The test took 310 to 380 ns a call, more than the loop takes for a config of none, one or two keys, so `createElement` took 1.57 times as long for no keys, 1.07 for one key and 1.13 to 1.40 for a `key` and one or two more. The shop took 0.99 of the time (1.6% to 2.5% fewer instructions) and no page passed the rule. It also could not rule out a proxy, a getter that deletes a later key and a setter added to `Object.prototype` after React loaded. The same checks in C cost less than the empty loop, and the function gives 2.1% to 4.1% fewer instructions.
- **A fast path for `===` in the interpreter loop**: 1.0% to 1.3% slower, measured with v0.26.14 and PGO. The interpreter function is so large that Go inlines only the smallest functions into it, so a new helper there is one more real call.
- **The interrupt countdown in a small function of its own**: no change (0.998 to 1.001 of the time) with v0.26.14, for the same reason. v0.26.16 writes the countdown inline instead, through `pollDue`, which Go does inline, for 0.6% with PGO.
- **Writing out `JS_FreeAtom` and `is_strict_mode` in the interpreter**, as v0.26.16 does for `set_value`: nothing that could be told from noise, so they stay calls.
- **Writing the bit fields of a new object in 2 stores rather than 9**: no change (0.991 to 1.010). The time is the cache miss of the first write to the new object, and fewer stores only move it. Tried again after v0.26.17, as one store per run of bit fields in `JS_NewObjectFromShape`, `JS_NewArray` and the like: 0.3% fewer instructions, but the bench pages of tifa took 1.010 of the time (1.005 to 1.030 over 8 rounds, three builds at different addresses).
- **Taking the room of large calls from a stack per runtime**: a call whose arguments, locals and stack need more than 768 bytes mallocs them, about 6000 times in a render of tifa's bench pages, through libc's allocator and its lock. Cutting them from a block of 64 KiB per runtime instead, as calls return in reverse order, saves as many instructions as the cut itself costs, the lock is not contended with one VM, and the bench pages took 1.009 of the time (0.998 to 1.020 over 8 rounds, six builds at different addresses), the shop's 1.003.
- **Taking the TLS frame of `JS_CallInternal` without a call**: Go does not inline the fast path into the interpreter, and the frees inlined at its returns made renders of tifa's bench pages take 1.6% more time. TLS frames taken without a call on linux leaves it out.
- **A fast path for object spread** (`JS_CopyDataProperties`): one spread took 240 ns rather than 415 in a micro benchmark on the Apple M4, but the shop did not change (0.997 to 1.005). It is done since together with the listing of keys, which it shares with `for in` and `Object.keys` (see [Own keys listed from the shape](changes/objects-and-shapes.md#own-keys-listed-from-the-shape)).
- **Testing the uncatchable flag before calling `_js_uncatchable_pending`** at the entry of `JS_CallInternal`: the helper is too large to inline into the interpreter, so every JavaScript call pays a real call to it. Testing the runtime's flag inline first skips that call on the normal path, and keeps the full check, which is still needed because the flag stays set after the exception is taken. 0.06% fewer instructions on V8, 0.09% on the shop and 0.41% on MUI. Over six rounds on three code layouts, V8 took 0.9988 of the base score (0.9883 to 1.0254), the shop 0.9927 of the time (0.9859 to 1.0014) and MUI 0.9850 (0.9763 to 1.0265). The sign follows the layout: MUI took 1.027 and 1.012 of the time in the two rounds of one layout, and 0.976 to 0.988 in the four rounds of the other two. The gain cannot be told from layout noise.
- **Reading the count of locals once, alone**: at the start of a call the interpreter read `b->var_count` again after each local it set. Reading it once gave React 1.0% fewer instructions but a loop of arithmetic 3.9% more, from a register spilled in one handler, and the time did not change. It is done since together with setting four locals a turn and freeing two values a turn (see [Calls, for in and hasOwnProperty.call](#calls-for-in-and-hasownpropertycall)).
- **Spreading a dense array without its iterator**: `[...a]` of three elements took 165 ns rather than 615, and a call with spread arguments 394 ns rather than 869, on v0.26.17 in a micro benchmark on linux/amd64. The shop's six pages (0.997) and three MUI pages (1.007) did not change beyond the spread that the layout of the code gives, since pages rarely spread arrays. It would have added about 870 lines of fixlib rules.
- **Keeping the in-between shapes of every object literal**, before v0.26.16: about 2% more on top of the new property fast path, but the two list pages ran out of VM memory, because the kept shapes held their prototypes and everything those reach. v0.26.16 keeps only shapes whose prototype is `Object.prototype`, at most 32 keys and no unregistered symbol key, in a ring of 2048 (see [Kept shapes](changes/objects-and-shapes.md#kept-shapes)).
- **Building a shape in place when no other object has its keys**: in the first form of the shape cache, copying every shape made objects of keys no other object has 22% slower to build. v0.26.16 earns one copy for each shape found, up to 64, which leaves those objects about 6% slower.
- **Literal templates in the bytecode**, as V8 copies a prebuilt object: needs compiler and bytecode format changes, and quickjs-ng saw no clear difference when it tried. It is done since, as one opcode for each run of plain keys whose final shape the function keeps (see [Object literals made at their final shape](changes/objects-and-shapes.md#object-literals-made-at-their-final-shape)).
- **Allocating the property array of a literal at its final size**: the opcode that makes the object does not know the count, and growing arrays differently costs memory for every small object. It pays only together with kept shapes. Done since with that opcode, which knows the count of its keys.
- **Inline caches for property reads**: quickjs-ng added and then removed them. Across 17 real tools they ranged from 11% slower to 54% faster, 4% faster on average, and used more memory. Tried after v0.27.0 as a table of 4096 entries keyed by the bytecode position, each holding a shape and the index of an own data property, checked against the shape itself (the index under its count, the same key, plain flags), so it needs no invalidation. Most reads on tifa's pages are own properties of the object, which `find_own_property` already finds in one hash step, and a hit on a prototype cannot skip the check of the shapes on the way. 0.3% fewer instructions, but the shop took 1.006 of the time and the MUI pages 1.007 (6 rounds, 3 layouts), for 96 KiB more per VM.
- **A table of shape steps for new keys**: `add_property` looks for the shape an object goes to in the shape hash table, and that lookup misses the cache often (2.4% of the instructions of a render of the shop, 7.7% of its cache misses). A table of 1024 steps per runtime, from a shape and a key to the next shape, holding both, cut 0.8% of the instructions on the shop and the MUI pages, but the shop took 1.008 of the time and the MUI pages 1.019 (6 rounds, 3 layouts): the shapes it holds alive (2718 to 3504 on an MUI page) add 28 KB to 142 KB per VM and change when the cycle collector runs.
- **Fast arrays in `every`, `some`, `forEach`, `map` and `filter`**, without asking `JS_TryGetPropertyInt64` for each element: `js_array_every`, which runs all five, spends 0.67 s of its own outside the callbacks in a profile of 85.6 s of the ten store pages of tifa's example shop, `JS_TryGetPropertyInt64` 0.19 s of it. A fast path would save at most about 0.5% of a store render, which the layout of the code hides. `find`, `findIndex` and `includes` already read fast arrays directly.
- **One libc allocator per VM**, to avoid its global lock: with 8 VMs on Linux the lock costs 0.4% to 0.9% of the CPU, and since v0.26.15 10 VMs on an Apple M4 wait on locks for about 0.7% of their time.
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
- **Clang PGO for the C engine**, measured on 2026-10-29. Instrumented builds trained on `r_react_ssr`, the 400 KB Markdown, 18 shop pages, 3 MUI pages and 2 bench pages. On the native C tree, 8 positions, 3 rounds: `r_react_ssr` took 1.096 of the time and the bench pages 1.083 (8 of 8 positions), the Markdown 0.962, the shop pages 0.987 and MUI 1.017 (both under the 2% the rule counts). A profile of only 4 shop pages did not make those 4 faster (1.000). On the generated engine (tifa's pages, 4 positions, 6 rounds) every page group was slower, the shop 1.058, the store 1.051, MUI 1.064 and the bench pages 1.150, the same sign at all 4 positions. The bench table page ran 32.5% more instructions: `JS_FreeValue` was no longer inlined (19.7% of its instructions as a call) and `JS_CallInternal` ran 22% to 23% more instructions with a frame of 640 bytes in place of 496. With the four inline thresholds put back to 400 it was still 29.4% more, so the thresholds are not all of the cause, and the rest was not found. The generator takes a profile without changes (a wrapped clang that adds `-fprofile-instr-use` when it makes the IR), and it makes the `.s` 22% larger (8.0 MB to 9.77 MB). It also found that the constant pools of the generated code are aligned within the function, not at their absolute address, so an aligned SSE read from a pool faults when the body of `JS_CallInternal` sits at 40 bytes of its line (the PGO build had 19 such reads). `JS_CallInternal` of main has none.
- **Cheaper polls**, after the cost was measured (see [What the generated code costs against native C](#what-the-generated-code-costs-against-native-c)). The call of `host_poll` is not the cost: with the interval at 1,000, 10,000 and 100,000 polls a script took the same time, and `host_poll` and the Go runtime are 0.2% to 1.1% of the added instructions (0.06% to 0.16% of a tifa render). The cost is in the counting: the registers it holds, the count that is written back before and read after each call, and bigger code. Taking the counts out of the dispatch saved only 6% of the instructions, and taking away as well the register the function held for the count and the sync around its calls still left a `jsloop` at 1.41 times the instructions of an engine without polls. What was done is in [Polls and interrupts](changes/c-engine.md#polls-and-interrupts): counts only at jumps back and at the entry of a function, loops that run in chunks and count once a chunk, a count in a register or in memory for each loop, and no more `llc` options for tail duplication. What was not done:
  - **A fixed register for the count in all the code (H2).** It can be made correct under Go, but x86 would need LLVM 23, and OCaml found the count in a virtual register mostly spilled and slower, and DTVM went back to ordinary registers in its optimizing mode.
  - **Cutting the limit of a loop into chunks as HotSpot does (M1).** It works only for loops whose trip count the compiler can work out. Of the scripts it would speed up, `strsearch` 5% to 10% and React about 0, and a mistake in the arithmetic of the limit is an out of range access.
  - **Giving the small functions whose only call is `host_poll` their `NOSPLIT` back (L3).** With the counts of all the C functions taken out the ceiling is `json` 15%, `strsearch` 11%, React 4% and `r_react_ssr` 0%, so under 0.5% on a page.
  - **The `llc` options for tail duplication for the interpreter alone (M2).** Without them only `JS_CallInternal` changes in the whole engine, so giving them to it alone gives the same bytes. They are taken out since, which cost `jsloop` 20% before the counts moved to the jumps back and made it faster after.
  - **Counting nothing in short loops of the C (the shape hash chains, `lre_find`, the property loop of `free_object`).** It needs the C to promise the bound, which the generator cannot check, the same weak spot that put an earlier idea out.
  - **Skipping the targets that cannot poll in a call through a pointer (L4)**, from the type of the pointer: types only tell the number and kind of the arguments, so a comparison function that may call JavaScript shares its type with the ones that cannot.
  - `taindexof` 25% slower with polls turned out to be where the code landed: its inner loop of 7 instructions across a line of 64 bytes took 22% to 25% longer, with and without polls.

## Where to continue

What the measurements above leave for the C engine, none of it large. A change under about 3% needs the positions of [Code positions](#code-positions), and callgrind below that.

- **The interpreter function** is 52.7% of the instructions of tifa's pages (15 pages, 4,147 million instructions a render on average) and the next functions are far behind: `add_property` 4.9%, `free_object` 2.8%, `js_call_c_function` 2.2%, `JS_NewObjectFromShape` 1.7%, `JS_GetPropertyInternal` 1.7%, `lre_find` 1.6%.
- **The entry of a JavaScript call** is about 95 of the 190 native instructions a call takes, and 73,000 calls make a workbench render. The loop that sets the locals to `undefined` is 2.1% of the interpreter's native instructions. Smaller entries have been tried ([Calls, for in and hasOwnProperty.call](#calls-for-in-and-hasownpropertycall), and fewer arguments above).
- **Adding a property** (`add_property`, 4.9% of the instructions and 8.6% of the last level misses) is the largest piece of property access that is left. Object literals now skip it for plain keys ([Object literals made at their final shape](#object-literals-made-at-their-final-shape)).
- **The translation** costs 11% to 13% and the polls 2% to 7%. What is left of the causes is the design (locals on the shadow stack) or tried above.
- **Memory** per VM: the free pages of the render's temporary space, the pages the shadow stack touched and the holes between free blocks make up the gap between a VM's resident memory and the engine's count of its blocks (4.2 MiB on the shop and 2.9 on MUI per VM, from 8 VMs against 1 after the renders). `ReleaseMemory` gives back the free pages when the pool lets a VM wait.
