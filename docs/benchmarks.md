---
summary: 'How liustack/quickjs compares with the other pure Go JavaScript engines that run current JavaScript: what each supports, how fast each is on server-side rendering and on general workloads, where liustack/quickjs is far behind, what limits each has, and how to run the comparison again'
read_when:
  - Saying anything in public about how this engine compares with other Go engines
  - Choosing a pure Go JavaScript engine for a job
  - Looking for the workloads where this engine is slowest, to make it faster
  - Releasing a new version (rerun the comparison and update this page)
  - Changing compare/engines
---

# Pure Go JavaScript engines compared

Run of 2026-10-23: liustack/quickjs v0.27.2 against four other engines that
build with `CGO_ENABLED=0`, two of them twice (with and without their JIT).
GCP c2d-standard-8 (AMD EPYC 7B13), Linux 7.3, Go 1.27.2, every engine
process on one CPU with its hyperthread sibling idle, GOMAXPROCS=1. The full
tables are in
[`compare/engines/results/2026-10-23-v0.27.2-gcp.md`](../compare/engines/results/2026-10-23-v0.27.2-gcp.md),
the raw numbers in the `.json` next to it. The liustack/quickjs numbers are
from the engine code of the v0.27.2 tag. The earlier runs are in
[History](#history).

## In short

- **Support.** go-quickjs and goant pass 99.3% of test262 (language,
  built-ins and intl402), Temporal and Intl included. liustack/quickjs
  passes 83.1%: its Intl now has every constructor, but the comparison
  runs it without Temporal (package `temporal` is not registered), and
  Temporal is 13426 of the runs. moejs passes 79.6% and has no Intl. On
  ES2023 and older, every engine passes 99.1% to 99.3%.
- **Server-side rendering.** On the server bundles of real tifa apps,
  liustack/quickjs needs the least CPU of the pure Go engines (see
  [tifa pages](#tifa-pages)). On the smaller React page of the speed table,
  moejs needs 7% less CPU than us and go-quickjs 5% more.
- **General speed.** liustack/quickjs is not the fastest pure Go engine.
  go-quickjs without its JIT (its default) scores 1.8 times as high on the
  V8 suite, parses and writes JSON in 56% to 62% of the time, runs regular
  expressions 3.5 times faster and is faster on 55 of 63 QuickJS micro
  benchmarks. moejs writes JSON 3.7 times faster and builds strings 1.3
  times faster.
- **Far behind.** Empty loops and number arithmetic take 2.0 to 2.9 times
  the median of the other pure Go engines, V8's Crypto 2.1 times, property
  reads, array reads and strings to integers about 2 times (see
  [Where liustack/quickjs is far behind](#where-liustackquickjs-is-far-behind)).
  `Date.now()`, at 6.9 times in the v0.26.17 run, is now 1.07 times: since
  v0.27.0 the engine reads Go's clocks.
- **Where liustack/quickjs leads.** The tifa pages, little memory per VM
  (1.2 MiB, modernc 1.1 MiB, moejs 1.4 MiB, go-quickjs 2.1 MiB), and a
  memory limit that counts live memory and stops a script exactly at it,
  which only modernc shares. It is about twice as fast as
  modernc.org/quickjs, the C-to-Go translation it started from.

## Engines

| Engine | Version | What it is | How the comparison uses it |
|---|---|---|---|
| liustack-quickjs | v0.27.2 | Bellard's QuickJS 2026-06-04 translated from C to Go (ccgo), with hand-written fast paths | bytecode compiled once (`VM.Compile`), run with `EvalBytecode`, Intl from package `intl` |
| modernc-quickjs | v0.25.0 | the same translation, upstream of the fork | as above |
| go-quickjs-interp | v0.28.0 | QuickJS-NG rewritten in Go | `quickjs.Compile` once, `RunProgram`, JIT off, the default |
| go-quickjs-jit | v0.28.0 | the same with `-tags quickjs_jit` and `WithJIT()` | as above, the JIT is marked experimental |
| goant-interp | v0.2.1 | a JavaScript engine written in Go | `Runtime.Compile` and `RunProgram` per runtime, JIT off, the default |
| goant-jit | v0.2.1 | the same with `WithJIT(true)` | as above |
| moejs | v0.1.0-alpha.9 | a register bytecode engine written in Go, for plugin sandboxes | `moejs.CompileScript` once, `RunScript` on each runtime, runtimes with their own mutable builtins (`MutableIntrinsics`), since test262, the polyfills and the tifa bundles write to them |

goja, sobek and otto are left out: they lack async generators, WeakRef,
newer regular expression flags and more (otto implements ES5 only). jsc2go
and fastschema/qjs were in the first version of the comparison and are no
longer run (`compare/engines/CHANGELOG.md` has their last numbers).

## Support

| Engine | test262 | ES2023 and older | language | built-ins | intl402 | Temporal | Feature probes (77) | Intl probes (13) |
|---|---|---|---|---|---|---|---|---|
| goant-interp, goant-jit | 99.3% | 99.3% | 98.7% | 99.9% | 99.6% | yes | 73 | 10 |
| go-quickjs-interp, go-quickjs-jit | 99.3% | 99.3% | 98.6% | 99.8% | 100.0% | yes | 77 | 13 |
| liustack-quickjs | 83.1% | 99.2% | 97.3% | 76.5% | 36.3% | not registered | 75 | 13 |
| modernc-quickjs | 81.1% | 99.2% | 97.4% | 76.4% | 8.3% | no | 64 | 3 |
| moejs | 79.6% | 99.1% | 97.3% | 74.4% | 0.7% | no | 61 | 2 |

test262 at `d53ebc8b`, 96540 runs per engine (sloppy and strict), module
tests and tests that need host hooks left out for every engine. "built-ins"
counts Temporal, which is 13268 of the runs. Feature probes and Intl probes:
small scripts with a known answer, Intl compared with Node 24. Between two
runs of the same code a few tests change (modernc-quickjs 4 fewer,
goant-interp 9 fewer in the v0.26.17 run).

liustack/quickjs has had Temporal since v0.27.0, in package `temporal`, but
the comparison registers only package `intl`, so Temporal and the intl402
tests of Temporal (4058 of the 6742 intl402 runs) fail. Apart from them,
intl402 passes 2448 of 2684 (91.2%). Since v0.27.0 its Intl has every constructor,
Collator (and so a locale-aware `localeCompare`), Locale, DurationFormat,
ListFormat, RelativeTimeFormat, Segmenter and DisplayNames included, and
it passes all 13 Intl probes. What it still lacks, by test262 feature tag:
explicit resource management (`using`, 128 of 820 pass), `Array.fromAsync`,
import attributes for JSON modules, ShadowRealm, `Atomics.waitAsync` and the
newest proposals (`import defer`, iterator chunking and joining, immutable
ArrayBuffer). moejs fails the probes for Intl, `FinalizationRegistry`,
iterator helpers, `RegExp.escape`, JSON modules and proper tail calls.

## Speed

CPU time per operation (user and system, all threads), in milliseconds,
median of 5 rounds. Lower is better, except the V8 suite score. In brackets:
against liustack-quickjs.

| Engine | React SSR render | V8 suite score | JSON.parse | JSON.stringify | String building | RegExp | QuickJS micro benchmarks (geomean time) |
|---|---|---|---|---|---|---|---|
| liustack-quickjs | 15.1 | 835 | 3.75 | 5.08 | 6.37 | 47.3 | 1.00 |
| modernc-quickjs | 31.6 (2.09) | 376 (0.45) | 6.31 (1.68) | 9.31 (1.83) | 12.6 (1.98) | 84.4 (1.78) | 2.07 |
| go-quickjs-interp | 15.8 (1.05) | 1522 (1.82) | 2.09 (0.56) | 3.15 (0.62) | 8.04 (1.26) | 13.7 (0.29) | 0.61 |
| go-quickjs-jit | 18.6 (1.23) | 3718 (4.45) | 2.20 (0.59) | 3.30 (0.65) | 7.64 (1.20) | 13.4 (0.28) | 0.32 |
| goant-interp | 34.5 (2.28) | 347 (0.42) | 1.42 (0.38) | 2.68 (0.53) | 12.5 (1.96) | 60.6 (1.28) | 3.29 |
| goant-jit | 27.9 (1.85) | 1144 (1.37) | 1.42 (0.38) | 2.71 (0.53) | 10.3 (1.62) | 61.4 (1.30) | 1.13 |
| moejs | 14.0 (0.93) | 703 (0.84) | 1.90 (0.51) | 1.39 (0.27) | 4.82 (0.76) | 39.6 (0.84) | 0.84 |

Wall time tells the same story with one difference: go-quickjs's median
render takes less wall time than its CPU time, because its garbage
collector also works between operations. On the React page its median is
13.9 ms against our 15.0 ms. CPU time is what a busy server pays.

Cold start and memory, with the React bundle (117 KB):

| Engine | New VM, ms | New VM + load from source | New VM + load compiled program | First render | RSS per VM, MiB | Go heap per VM, MiB |
|---|---|---|---|---|---|---|
| liustack-quickjs | 0.209 | 13.9 | 1.68 | 15.5 | 1.2 | 0 |
| modernc-quickjs | 0.279 | 17.5 | 2.28 | 31.7 | 1.1 | 0 |
| go-quickjs-interp | 0.231 | 13.0 | 1.23 | 22.3 | 2.1 | 5.5 |
| go-quickjs-jit | 0.274 | 13.2 | 1.30 | 82.9 | 18.9 | 17.8 |
| goant-interp | 0.629 | 17.3 | n/a | 39.9 | 7.6 | 6.9 |
| goant-jit | 0.580 | 16.2 | n/a | 42.2 | 8.9 | 7.2 |
| moejs | 0.104 | 15.3 | 0.565 | 12.1 | 1.4 | 1.1 |

Memory: what the process grows by per VM with 100 VMs alive, each with the
bundle loaded and one page rendered. goant's Program serves only the runtime
that compiled it (`goant.go` 205 to 208), so it loads from source. moejs
makes each runtime with its own mutable builtins, which its guide says costs
about 25 times as much as its default of frozen builtins shared by every
runtime.

## tifa pages

For v0.26.16 and v0.26.17, HTML equality was checked at run time, but the
normalized digests were not archived. Their Markdown keeps the verdicts.
From the v0.27.1 run on, `pages.py --output` archives per-page SHA256
digests after masking timestamps and the same-as-liustack verdict, including
the separate polyfill and deployed comparisons. A missing HTML file has a
null verdict, not a match. The JSON also keeps raw process results, rest
runs, single-page probes and every try from the metadata files.

Server bundles of three tifa apps (`tifa/bundles`), rendered the way tifa
renders them, with the data each app's server sends for each page. Times
are CPU per render (the mean of 200 timed renders after 50 warm ones for
the bench pages, 100 after 20 for the shop, page by page in one process),
as a ratio to liustack-quickjs: geometric mean over the pages, range in
brackets. The C QuickJS and Node rows are references, not pure Go engines.
C QuickJS (`qjs`, built with its own Makefile) is timed in the script with
wall time on a CPU of its own. Every page's HTML is compared with
liustack-quickjs's, with the router's render timestamps (`u:` and 13
digits) masked. Every page per engine:
[`compare/engines/results/2026-10-23-v0.27.2-gcp-pages.md`](../compare/engines/results/2026-10-23-v0.27.2-gcp-pages.md).

On these pages liustack/quickjs is the fastest pure Go JS engine for SSR:
every other pure Go engine needs more CPU per page, or does not render it.

- Pages without Intl: go-quickjs-interp needs 24% more, moejs 26% more,
  go-quickjs-jit 34% more, the others 2.3 to 3.1 times as much.
- The shop, as each engine would deploy it: go-quickjs-interp needs 28%
  more on the 18 pages it renders, go-quickjs-jit 51% more on the same 18,
  the others 2.4 to 3.7 times as much. moejs cannot load the shop bundle
  (its functions are limited to 256 registers).

### Which pages need Intl

Deleting `Intl` and replacing `localeCompare` and the `toLocale*` methods
with stubs that throw leaves only the bench app's two pages unchanged:

| App | Pages | Same HTML without Intl | What reaches Intl first |
|---|---|---|---|
| bench | table, spiral | yes, both | nothing |
| mui | home, order, orders | no | the app's own currency `Intl.NumberFormat('en-US', {style: 'currency'})` and `Intl.DateTimeFormat`, home also `Number.prototype.toLocaleString('en-US')`, MUI's TablePagination checks for Intl |
| shop | all 20 | the bundle does not load | `new Intl.NumberFormat('zh-CN', {style: 'currency', currency: 'CNY'})` at the top level of the bundle, then i18next (`Intl.getCanonicalLocales`, `Intl.PluralRules`) and the pages' `Intl.DateTimeFormat` with a time zone |

So only liustack-quickjs, go-quickjs and Node can render the shop and MUI
pages as they are. To compare everyone on the shop, it was rendered twice
more: with the FormatJS polyfills (pinned in `tifa/polyfill`, zh-CN and
en-US data and time zone data, 2.4 MB) loaded before the bundle, and as each
engine would deploy it.

### Pages without Intl

| Engine | Load ms | Pages that render | Same page as liustack-quickjs | Time per render vs liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs | 65 | 2 of 2 | 2 of 2 | 1.00 (1.00–1.00) |
| modernc-quickjs | 79 | 2 of 2 | 2 of 2 | 2.33 (2.32–2.34) |
| go-quickjs-interp | 71 | 2 of 2 | 2 of 2 | 1.24 (1.23–1.25) |
| go-quickjs-jit | 73 | 2 of 2 | 2 of 2 | 1.34 (1.32–1.36) |
| goant-interp | 83 | 2 of 2 | 2 of 2 | 3.13 (2.91–3.38) |
| goant-jit | 73 | 2 of 2 | 2 of 2 | 2.67 (2.36–3.01) |
| moejs | 90 | 2 of 2 | 2 of 2 | 1.26 (1.23–1.29) |
| C QuickJS 2026-06-04 (reference) | 47 | 2 of 2 | 2 of 2 | 0.68 (0.64–0.71) |
| C QuickJS master 7d1c13c5d8 (reference) | 39 | 2 of 2 | 2 of 2 | 0.66 (0.63–0.70) |
| QuickJS-NG v0.17.0 (reference) | 36 | 2 of 2 | 2 of 2 | 0.89 (0.85–0.94) |
| Node v24.11.1 (reference) | 44 | 2 of 2 | 2 of 2 | 0.11 (0.09–0.13) |
| Node v24.11.1 --jitless (reference) | 73 | 2 of 2 | 2 of 2 | 0.36 (0.36–0.36) |

| Page | liustack-quickjs | modernc-quickjs | go-quickjs-interp | go-quickjs-jit | goant-interp | goant-jit | moejs | quickjs-2026-06-04 | quickjs-master | quickjs-ng | node-jit | node-jitless |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spiral | 31.2 (1.00) | 72.3 (2.32) | 38.9 (1.25) | 41.3 (1.32) | 105 (3.38) | 94 (3.01) | 40.3 (1.29) | 20.1 (0.64) | 19.6 (0.63) | 26.7 (0.85) | 4.04 (0.13) | 11.2 (0.36) |
| table | 27.7 (1.00) | 65 (2.34) | 34.2 (1.23) | 37.7 (1.36) | 80.7 (2.91) | 65.5 (2.36) | 34 (1.23) | 19.8 (0.71) | 19.5 (0.70) | 26 (0.94) | 2.38 (0.09) | 9.88 (0.36) |

The C QuickJS rows show what the translation from C to Go costs: on these
pages liustack-quickjs takes 1.5 times the time of the C it is translated
from, on the polyfilled shop 1.4 times.

### Shop on the FormatJS polyfill, every engine

Native `Intl` deleted first in every engine, including the three that have
one, so all run the same JavaScript. Load ms: the polyfill, then the bundle.

| Engine | Load ms | Pages that render | Same page as liustack-quickjs | Time per render vs liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs | 655 | 20 of 20 | 20 of 20 | 1.00 (1.00–1.00) |
| modernc-quickjs | 938 | 20 of 20 | 20 of 20 | 1.85 (1.75–2.00) |
| go-quickjs-interp | 793 | 18 of 20 | 18 of 18 | 1.23 (1.14–1.33). store-en-US-post, store-zh-CN-post fail, a failure leaves the runtime closed, the other pages measured in a run without the failing ones |
| go-quickjs-jit | 796 | 2 of 20 | 2 of 2 | 1.30 (1.27–1.33). every page but store-en-US-products, store-zh-CN-products fails, a failure leaves the runtime closed, the other pages measured in a run without the failing ones |
| goant-interp | 1085 | 20 of 20 | 20 of 20 | 2.85 (2.62–3.21) |
| goant-jit | 1119 | 20 of 20 | 20 of 20 | 2.70 (2.16–3.04) |
| moejs | load: SyntaxError: function needs more than 256 registers is not supported yet (see TODO.md) | 0 of 20 | | |
| C QuickJS 2026-06-04 (reference) | 386 | 20 of 20 | 20 of 20 | 0.70 (0.65–0.79) |
| C QuickJS master 7d1c13c5d8 (reference) | 379 | 20 of 20 | 20 of 20 | 0.69 (0.63–0.78) |
| QuickJS-NG v0.17.0 (reference) | 368 | 20 of 20 | 18 of 20 | 0.87 (0.83–0.94) |
| Node v24.11.1 (reference) | 260 | 20 of 20 | 20 of 20 | 0.16 (0.13–0.21) |
| Node v24.11.1 --jitless (reference) | 273 | 20 of 20 | 20 of 20 | 0.41 (0.37–0.44) |

QuickJS-NG's two different pages (`wb-order-studio`, `wb-order-weekend`)
have one more React text separator (`<!-- -->`) after "选择快递公司" in the
select value. moejs compiles the polyfill and the other two bundles, and
stops at the shop bundle's outer function, at line 1, column 2.

### Shop as deployed

Each engine with its own Intl if it has one, else with the polyfill.
Against liustack-quickjs with its own Intl.

| Engine | Load ms | Pages that render | Same page as liustack-quickjs | Time per render vs liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs (own Intl) | 302 | 20 of 20 | 20 of 20 | 1.00 (1.00–1.00) |
| modernc-quickjs (polyfill) | 938 | 20 of 20 | 20 of 20 | 2.43 (1.98–2.78) |
| go-quickjs-interp (own Intl) | 542 | 18 of 20 | 18 of 18 | 1.28 (1.18–1.39). store-en-US-post, store-zh-CN-post fail, a failure leaves the runtime closed, the other pages measured in a run without the failing ones |
| go-quickjs-jit (own Intl) | 556 | 18 of 20 | 18 of 18 | 1.51 (1.35–1.81). store-en-US-post, store-zh-CN-post fail, a failure leaves the runtime closed, the other pages measured in a run without the failing ones |
| goant-interp (polyfill) | 1085 | 20 of 20 | 20 of 20 | 3.74 (2.70–4.39) |
| goant-jit (polyfill) | 1119 | 20 of 20 | 20 of 20 | 3.55 (2.30–4.20) |
| moejs (polyfill) | load: SyntaxError: function needs more than 256 registers is not supported yet (see TODO.md) | 0 of 20 | | |
| C QuickJS 2026-06-04 (reference) (polyfill) | 386 | 20 of 20 | 20 of 20 | 0.92 (0.68–1.06) |
| C QuickJS master 7d1c13c5d8 (reference) (polyfill) | 379 | 20 of 20 | 20 of 20 | 0.91 (0.65–1.06) |
| QuickJS-NG v0.17.0 (reference) (polyfill) | 368 | 20 of 20 | 18 of 20 | 1.14 (0.88–1.32) |
| Node v24.11.1 (reference) (own Intl) | 127 | 20 of 20 | 20 of 20 | 0.16 (0.13–0.24) |
| Node v24.11.1 --jitless (reference) (own Intl) | 150 | 20 of 20 | 20 of 20 | 0.41 (0.36–0.44) |

### What the polyfill costs

On liustack-quickjs, from the same runs:

- Loading it: 371 ms per VM before the bundle. The bundle then loads in
  284 ms, 302 ms without the polyfill. In the v0.27.1 run it took 411 ms
  after the polyfill, and 294 ms in the v0.26.17 run: why that run was
  slower was not looked into.
- Memory per VM (20 VMs, each with the bundle loaded, the polyfill too
  where named, and the zh-CN cart page rendered):
  34.7 MiB RSS with the polyfill against 12.8 MiB with its own Intl.
  go-quickjs-interp: 116.1 MiB against 48.0 MiB (Go heap 82.6 against
  40.4 MiB).
- Share of a render: summed over the 20 pages, 217 of 939 ms of a render is
  spent inside the polyfill (23%, 1354 calls), against 5.6 of 710 ms inside
  its own Intl (1%) for the same calls.

go-quickjs-interp fails on the two blog post pages, with its own Intl and on
the polyfill alike. go-quickjs-jit fails on the same two with its own Intl,
and on 18 of the 20 pages on the polyfill (see
[Problems found](#problems-found-in-other-engines)).

## Where liustack/quickjs is far behind

Every QuickJS micro benchmark, V8 suite benchmark and workload of the speed
table where liustack-quickjs needs at least twice the time of the median of
the six other pure Go builds (for the V8 suite: half the score). The
suspected causes come from our CPU profiles, and are what to look at first.

| Item | Ours vs the median | Fastest | Suspected cause |
|---|---|---|---|
| `empty_loop` | 2.9 | go-quickjs-jit | dispatching each bytecode through the translated interpreter loop, with the operand stack in C memory |
| `int_arith` | 2.6 | go-quickjs-jit | the same dispatch, and a tag check and a 16-byte value per operand where others keep int32 or NaN-boxed numbers |
| `empty_down_loop` | 2.6 | go-quickjs-jit | as `empty_loop` |
| `empty_down_loop2` | 2.4 | go-quickjs-jit | as `empty_loop` |
| `float_arith` | 2.4 | go-quickjs-jit | as `int_arith` |
| `array_for` | 2.3 | go-quickjs-jit | each element read goes through the generic `get_array_el` path |
| `prop_read` | 2.1 | go-quickjs-jit | every property read hashes into the object's shape (`find_own_property`), with no cache at the access site |
| V8 Crypto | 2.1 (score) | go-quickjs-jit | integer arithmetic in loops and array element reads, as above |
| `string_to_int` | 2.1 | go-quickjs-jit | `js_atod` and its multiprecision code for every conversion, where go-quickjs has a path for plain integers |
| `array_read` | 2.0 | go-quickjs-jit | as `array_for` |
| `empty_do_loop` | 2.0 | go-quickjs-jit | as `empty_loop` |

The medians are of modernc-quickjs, go-quickjs-interp, go-quickjs-jit,
goant-interp, goant-jit and moejs. `python3 behind.py
results/<run>.json` in `compare/engines` makes the list again.

Just under the line: `string_to_float` (1.99, fastest moejs), JSON.parse
(1.88), V8 NavierStokes (1.75), JSON.stringify (1.74) and
`typed_array_read` (1.71). `string_to_int` and `array_read` were at 1.92
and 1.86 in the v0.27.1 run: they took 11% and 10% more time in this run,
which was not looked into (see the note on single tests below).
`date_now` was on the list at 6.9 in the v0.26.17
run and is 1.07 now: since v0.27.0 the engine reads Go's clocks instead of
calling libc's `gettimeofday`, which modernc.org/libc made as a real
system call. Single micro benchmarks of tiny loops move
between two builds of the same code, as the code lands at other addresses:
modernc-quickjs, the same code in both runs of History, took 0.8 to 2.5
times its earlier time on single tests (2.5 on `global_read`), though 1.05
times on their geometric mean. Read one test's ratio as rough.

## Where the others are faster, and why

CPU profiles on the same machine (`JSBENCH_CPUPROFILE`, `-pgo=off`) of each
case where an engine needs at least 1.3 times less CPU than
liustack-quickjs. The profiles are from the v0.26.16 run (go-quickjs
v0.27.0), the ratios from the v0.26.17 run, and the go-quickjs file and line
numbers from v0.28.0. moejs was not profiled: its mechanisms below are from its
guide (`docs/guide.md`, "Implementation"). No text summaries of those
profiles are archived, so the descriptions below are qualitative observations
from that run, not measured CPU shares for v0.26.17.

- **Property reads and writes, and calls** (V8 Richards and DeltaBlue,
  micro benchmarks `prop_read`, `prop_update`, `func_call`). In our
  Richards and DeltaBlue profiles, property lookup, field operations and
  setting up JS call frames were prominent. Every property access hashes
  into the object's shape. go-quickjs caches the shape and slot at each access site
  (`internal/vm/vm_shape.go` 807 and 1019, read in `tree_build.go` 2214),
  runs hot functions as a tree of Go closures with no operand stack
  (`internal/vm/vm_tree.go`, `zcall_tree.go` 86), updates a property in
  place in one node (`ztree_update.go` 601) and calls small pure functions
  without a frame (`vm_leaf_pure.go` 179). moejs has an inline cache per
  instruction.
- **Numbers in loops** (V8 Crypto and NavierStokes, `empty_loop`,
  `int_arith`, `float_arith`). Ours is bytecode dispatch and the arithmetic
  opcodes, with `get_array_el` also prominent in NavierStokes. go-quickjs keeps
  numbers as NaN-boxed floats (`internal/vm/value.go` 81), has nodes for
  int32 arithmetic (`tree_build.go` 1639, `tree_operand.go` 9), for array
  element reads (`vm_tree.go` 267) and for a whole loop (`ztree_flow.go`
  1140). In the go-quickjs-jit and goant-jit profiles, generated machine
  code dominated.
- **`Date.now()`** (`date_now`, 3.7 to 9 times faster in the engines written
  in Go, modernc has the same cost).
  The v0.26.16 profile was dominated by `clock_gettime`: `date_now` calls libc
  `gettimeofday`, which modernc.org/libc v1.77.1 makes as a real system
  call instead of going through the vDSO. go-quickjs calls `time.Now`
  (`internal/vm/builtins_date.go` 116), which uses the vDSO. Fixed in
  v0.27.0, where the engine reads Go's clocks: in the v0.27.2 run
  `date_now` takes 1.07 times the median of the others.
- **Copying arrays** (`array_slice`, go-quickjs 2.9 times faster, moejs 12.6
  times). Ours
  defines each element as a property (`JS_DefineProperty`), with
  `TLS.Alloc` also prominent in that profile. go-quickjs reads the dense
  elements of the source and appends them to the new array's storage (`internal/vm/builtins.go` 1228).
- **Destructuring arrays** (`local_destruct`, go-quickjs 1.9 times faster, moejs
  3.3 times).
  Ours runs the iterator protocol. go-quickjs skips it for arrays that still
  iterate the way `Array.prototype` does (`internal/vm/iterate.go` 608).
- **Strings to numbers** (`string_to_int`, `string_to_float`). Ours goes
  through `js_atod` and its multiprecision code. go-quickjs has a path for
  plain integers (`internal/jsnum`), goant calls Go's `strconv`.
- **JSON** (goant: parse 2.8 times, stringify 2.1 times faster, moejs:
  parse 2.1 times, stringify 4.1 times). goant parses and writes with code
  of its own over the string (`internal/engine/builtin_json.go` 571, 870,
  190) and caches the shape changes of the objects it builds (`shapes.go`
  404). moejs's JSON is native Go and builds objects on known shapes. Ours
  showed time in `TLS.Alloc`, `json_next_token` and
  `string_buffer_putc` in that profile. go-quickjs's JSON is profiled separately.
- **String building** (moejs 1.4 times faster). Not profiled on moejs.
- **Regular expressions** (go-quickjs 3.6 times faster, moejs 1.2 times).
  Our profile showed the backtracking matcher and its timeout check.
  moejs runs the patterns it can translate exactly on Go's RE2. go-quickjs's regular
  expressions are profiled separately.
- **Loading a compiled program** (go-quickjs 1.40 times faster, moejs 3.0
  times). We read the bytecode into every new VM, then run the bundle's
  top level. go-quickjs shares one compiled program between runtimes and only prepares it
  (`internal/vm/eval.go` 18) before running the top level. A moejs `Script`
  is immutable and runs on any runtime.
- **Across the board**, `TLS.Alloc` (the C stack frames the C-to-Go
  translation keeps on a stack of its own) appeared across the v0.26.16
  profiles.

## Limits

| Engine | Memory limit per VM | Stopping a script | Recursion depth (probe) | JIT platforms | Other |
|---|---|---|---|---|---|
| liustack-quickjs | exact: the allocator refuses past the limit and the script gets an uncatchable out of memory (`VM.SetMemoryLimit`, `quickjs.go`) | `Interrupt`, uncatchable | 9993 | no JIT | one VM per goroutine at a time, VMs independent |
| modernc-quickjs | exact, same allocator | interrupt handler | crashes the process (Go stack overflow) instead of a RangeError | no JIT | `tail-call` probe also crashes the process |
| go-quickjs | approximate: the heap is walked once the process has allocated a quarter of the limit since the last walk (`docs/known-issues.md` KI-02) | context or abort | 8189 | `quickjs_jit` tag, experimental, linux/amd64, windows/amd64, darwin/arm64 (`internal/jit` build tags) | without a deadline, a regular expression stops after a step budget with `SyntaxError: regular expression is too complex` (KI-17) |
| goant | partial: arrays went past it, a string grew past it without an error, objects stopped (probe). Compiled code is outside it (`goant.go` 576 to 585) | `Interrupt` | 8189 (12044 with JIT) | amd64, arm64 (`internal/jitasm`) | the inline cache counter is process-wide, so a collection or a prototype change in one runtime makes every runtime's caches refill (`internal/engine/shapes.go` 28 to 48). `setTimeout`, `setImmediate`, `structuredClone` and `process` are present by default |
| moejs | counts what a runtime allocates between resets, not what is live, and stops the script uncatchably past it (`Options.MemoryLimit`, `docs/guide.md` "Memory limit"). It stopped all three probes | `Interrupt`, uncatchable | 508: the call depth is fixed at 512 (`engine/function.go` 332) | no JIT | a function can use at most 256 registers (`compiler/compiler.go` 452, 483, 1043), which the tifa shop bundle goes past. Builtins are frozen and shared by every runtime unless `MutableIntrinsics` is set. No timers unless the host adds its `eventloop` package |

File and line numbers are for the versions in the Engines table.

## Which engine for what

- **liustack-quickjs:** server-side rendering, many small VMs in one process
  with hard memory limits, a fresh VM per request, and pages that need
  Intl or Temporal (package `temporal`, which this comparison does not
  measure).
- **go-quickjs-interp:** the best all-round choice when CPU-heavy scripts
  matter and full Intl or Temporal is needed. Its memory limit is
  approximate, and two of the tifa shop pages make it fail (below).
- **go-quickjs-jit:** long-running numeric code on its three platforms. The
  first render of a page takes 3.8 times as long as the interpreter's, a VM
  takes 9 times the memory, and with the FormatJS polyfills it fails on 18
  of the 20 shop pages (below).
- **goant-interp, goant-jit:** the most complete test262 and Intl after
  go-quickjs, and fast JSON. Slow elsewhere without the JIT.
- **moejs:** small plugin scripts: the cheapest new runtime and compiled
  program, fast JSON and strings, little memory. Its fixed call depth, its
  register limit and the lack of Intl rule out large bundles like the tifa
  shop.
- **modernc-quickjs:** the upstream of liustack/quickjs. Slower on every
  workload.

## Problems found in other engines

Each with a way to reproduce it from `compare/engines`. Reported as found,
on the versions above.

- **go-quickjs with its JIT, on the FormatJS polyfills: 18 of the 20 shop
  pages fail.** `quickjs: internal error: runtime error: invalid memory
  address or nil pointer dereference`, each page in a fresh process, every
  time. Only the two product list pages render. With the same scripts the
  interpreter build renders every page but the two post pages, and so does
  the JIT build with its own Intl instead of the polyfills. With v0.27.0 the JIT build
  already failed while loading the polyfills, in generated code. With
  v0.28.0 the polyfills load and the panic comes from the tree interpreter,
  as in the next item. Reproduce (after `tifa/polyfill/build.sh`):
  `JSBENCH_FULL_ERRORS=1 ./bin/goquickjsjit tifa-ssr -bundle tifa/bundles/shop.js -inputs tifa/in/shop/wb-stats-studio.json -prelude tifa/prelude/nointl.js,tifa/polyfill/intl-polyfill.js -warm 0 -n 1`.
- **go-quickjs: the shop's two blog post pages fail.** `quickjs: internal
  error: runtime error: invalid memory address or nil pointer dereference`
  from a panic in the tree interpreter (`internal/vm/vm_tree.go` 168,
  rethrown through `ztree_zexception.go` 290, so the stack no longer shows
  where it began), on a fresh runtime, every time, with its own Intl and on
  the polyfill, with and without its JIT, in v0.27.0 and v0.28.0. The
  runtime is closed afterwards: every later page fails with
  `quickjs: runtime is closed`. Reproduce:
  `JSBENCH_FULL_ERRORS=1 ./bin/goquickjs tifa-ssr -bundle tifa/bundles/shop.js -inputs tifa/in/shop/store-en-US-post.json -warm 0 -n 1`.
- **goant: `string_build_large2` exceeds the 4 GiB cap.** The QuickJS micro
  benchmark that prepends to a string in a loop goes over the 4 GiB cap
  in all five rounds of the [archived v0.26.17 run](../compare/engines/results/2026-10-21-v0.26.17-gcp.json),
  and again in the v0.27.1 and v0.27.2 runs. Those runs do not
  measure its uncapped peak memory or successful call time. goant keeps
  strings flat, so each prepend copies. Reproduce: `./bin/goant upstream -dir ../testdata -only string_build_large2`.

## Rerun

Everything lives in `compare/engines`, a Go module of its own, so the other
engines never enter this module's `go.mod`.

```sh
cd compare/engines
go run ./cmd/compare                        # every workload, 5 rounds
go run ./cmd/compare -cpu 1 -sibling 5      # Linux: pin to CPU 1, rerun when CPU 5 was busy
go run ./cmd/compare -phases react,v8       # some workloads
go run ./cmd/compare -engines liustack-quickjs,go-quickjs-interp
go run ./cmd/compare -report results/<run>.json   # Markdown again from the JSON
```

`make compare-engines ARGS="..."` at the root does the same. Each run writes
`results/<date>-<version>-<host>.json` and `.md`, and keeps the raw output of
every process in `results/raw/<run>/` (not checked in). A new run never
overwrites an old one.

On a shared machine a run can be cut into short pieces with `-name` and
`-add`: the first piece makes the run, each later one adds to it. Timed
workloads get `-rounds` more rounds each time, and test262 can run a few
engines at a time, or on another CPU at low priority and be merged in:

```sh
B="go run ./cmd/compare -cpu 1 -sibling 5 -name 2026-10-21-v0.26.17-gcp"
$B -phases features,concurrent
$B -add -phases react -rounds 5
$B -add -phases v8 -rounds 1          # five times
go run ./cmd/compare -cpu-test262 0 -nice-test262 19 -phases t262 -name 2026-10-21-v0.26.17-gcp-t262
go run ./cmd/compare -name 2026-10-21-v0.26.17-gcp -merge results/2026-10-21-v0.26.17-gcp-t262.json
```

An engine added after a run was made is run on its own and merged in with
`-merge-engines`, which copies all its results and records the run and the
commit its binaries came from:

```sh
go run ./cmd/compare -cpu 1 -sibling 5 -engines moejs -name 2026-10-21-v0.26.16-gcp-moejs
go run ./cmd/compare -name 2026-10-21-v0.26.16-gcp -merge results/2026-10-21-v0.26.16-gcp-moejs.json -merge-engines
```

To measure on another machine, build there or cross-build and copy `bin/`
with it:

```sh
go run ./cmd/compare -build-only -goos linux -goarch amd64 -bindir bin-linux-amd64
GOOS=linux GOARCH=amd64 go build -o bin-linux-amd64/compare ./cmd/compare
# on the machine, in compare/engines with bin/ = bin-linux-amd64:
./bin/compare -nobuild -cpu 1 -sibling 5
```

The driver needs `compare/testdata` and `internal/arewefastyet/v8-v7` next to
it, as in the repository. test262 is downloaded at the pinned commit unless
`-test262` names a checkout.

`native/build.sh` fetches these inputs only when building the reference
rows. They are not shipped in this repository. The script pins URLs and
SHA256 hashes. These are the primary licenses, with bundled third-party
terms retained in each downloaded distribution:

| Download | Version | License |
| --- | --- | --- |
| C QuickJS release | 2026-06-04 | MIT, see the archive's `LICENSE` |
| C QuickJS master snapshot | 7d1c13c5d8 | [MIT](https://github.com/bellard/quickjs/blob/7d1c13c5d8/LICENSE) |
| QuickJS-NG | v0.17.0 | [MIT](https://github.com/quickjs-ng/quickjs/blob/v0.17.0/LICENSE) |
| Node, Linux x64 binary | v24.11.1 | [MIT and third-party notices](https://github.com/nodejs/node/blob/v24.11.1/LICENSE), including V8 (BSD-3-Clause), OpenSSL (Apache-2.0) and ICU (Unicode) |
| CMake, Linux x86_64 binary | 3.31.6 | [BSD-3-Clause](https://github.com/Kitware/CMake/blob/v3.31.6/Copyright.txt), with third-party terms under `Licenses/` in the distribution |

The tifa pages and the native reference rows have a script of their own
(Linux, CPU 1 and its sibling CPU 5):

```sh
cd compare/engines
tifa/polyfill/build.sh                      # Node and npm: the FormatJS bundle
native/build.sh "$HOME/jsnative"            # C QuickJS, QuickJS-NG and Node
run="<date>-<version>-<host>"
page_dir=$(mktemp -d "/tmp/$run-pages.XXXXXX")
tifa/run.sh "$page_dir" bin-linux-amd64 "$HOME/jsnative"
python3 tifa/pages.py "$page_dir" --output "results/$run-pages"
```

When a page leaves an engine unusable (go-quickjs closes its runtime after
a page fails), run the shop again without the failing pages into
`<engine>-<kind>-rest.json` in the same directory: `pages.py` takes the
missing pages from there. To find the failing pages when most fail, render
each page alone (`-inputs <one page> -warm 0 -n 1`) first.

At each release, follow "Engine comparison" in
[releasing.md](releasing.md#compare-with-the-other-engines-again).

## How it measures

- Each engine is linked into a binary of its own (`jsbench`, build tag
  `eng_<name>`), built with `CGO_ENABLED=0 -pgo=off -trimpath`, so no engine
  shares a process, a profile or a garbage collector with another.
- Each engine is used the way its README recommends: scripts compiled once
  and run as programs where the API allows, Intl registered where the
  engine needs it, modules through each engine's own loader. The results
  page lists, per engine, exactly how.
- Timed workloads run in rounds. In each round every engine runs every
  workload in a new process, and the engine order changes from round to
  round. Tables show the median of the round medians and, in brackets, the
  range of the round medians.
- With `-cpu`, the Go driver pins timed processes under `taskset -c <cpu>`
  and sets GOMAXPROCS=1. `tifa/run.sh` pins both its functional checks
  (normal, stub and proxy) and its timed processes to CPU 1, through the
  same capped entry with TZ=UTC and GOMAXPROCS=1. When the hyperthread
  sibling of that CPU was busy during a run for at least 3 clock ticks (30 ms) and more than 5% of the run, or
  the engine process got less than 90% of the run as CPU time (another
  process ran on its CPU), the run is tried again, up to three tries in
  all. The results count the repeats and keep the last try.
- test262 does not need a quiet CPU: it can run on another CPU at low
  priority (`-cpu-test262 0 -nice-test262 19`) as a run of its own, and
  `-merge` copies its results into the main run.
- Every engine process runs under a memory cap (4 GiB, 2 GiB for test262):
  on Linux a cgroup (`systemd-run --user --scope -p MemoryMax`), which also
  counts memory outside the Go heap, plus a check of the resident size.
- Pinned inputs: test262 at `d53ebc8b`, the V8 benchmark suite version 7
  from mozilla/arewefastyet at `d2ddc6ce`, React 19.3.1 bundled with
  esbuild 0.28.2, the tifa bundles in `tifa/bundles` (`SHA256SUMS`), the
  FormatJS packages in `tifa/polyfill/package-lock.json`.

### Workloads

| Workload | What it does |
|---|---|
| React SSR render | `renderToString` of a 717-element catalog page (React 19.3.1, `react-dom/server` legacy browser build, one IIFE bundle of 117 KB, no host functions). The first render of every engine is compared with Node's output byte for byte. |
| V8 suite | The 8 benchmarks of the V8 suite version 7, scores as the suite computes them. Licenses: see `internal/arewefastyet/v8-v7`. |
| JSON.parse, JSON.stringify | A document of 1000 items (155 KiB of UTF-8), parsed or written once per call. |
| String building | 20000 appends with `+=`, then 5000 pieces joined with `Array.prototype.join`. |
| RegExp | Five common regular expressions (`match` with `g`, an `exec` loop, `replace`, `split`) over a 2000-line access log. |
| QuickJS micro benchmarks | `compare/testdata`, QuickJS's own micro benchmarks, one process per test, as time per iteration against liustack-quickjs. |
| Cold start | A new VM, a new VM with the bundle loaded from source, from a compiled program (checked to render the same page), and the first render. |
| Memory | Growth of the process per VM with 100 VMs alive, each with the bundle loaded and one page rendered. |
| Features | 77 probes from ES2015 to ES2025, Intl (compared with Node 24's answers) and limits. |
| test262 | `test/language`, `test/built-ins`, `test/intl402`, sloppy and strict, a fresh VM per run. Module tests and tests that need host hooks (`$262.createRealm` and the like) are left out for every engine. |
| tifa pages | Server bundles of three tifa apps (`tifa/bundles`), rendered the way tifa renders them: `startRender(json)`, then the promise jobs, then `renderResult`. Inputs are the data the apps' servers send for each page. |

## History

| Run | liustack/quickjs | Machine | Notes |
|---|---|---|---|
| [2026-10-23-v0.27.2-gcp](../compare/engines/results/2026-10-23-v0.27.2-gcp.md) | v0.27.2 | GCP c2d-standard-8, AMD EPYC 7B13, CPU 1 | every engine again at the same versions ([pages](../compare/engines/results/2026-10-23-v0.27.2-gcp-pages.md)). test262 run on CPU 0 at low priority and merged in, the same 80215 runs pass. React page 15.1 ms of CPU rather than 14.9, V8 score 835 rather than 854, JSON and regexps 1% to 3% faster. `string_to_int` and `array_read` cross the far-behind line, `string_to_float` drops under it |
| [2026-10-22-v0.27.1-gcp](../compare/engines/results/2026-10-22-v0.27.1-gcp.md) | v0.27.1 | GCP c2d-standard-8, AMD EPYC 7B13, CPU 1 | every engine again at the same versions ([pages](../compare/engines/results/2026-10-22-v0.27.1-gcp-pages.md)). test262 run on CPU 0 at low priority and merged in. React page 14.9 ms of CPU rather than 16.3, V8 score 854 rather than 800, `Date.now()` no longer far behind, all 13 Intl probes pass |
| [2026-10-21-v0.26.17-gcp](../compare/engines/results/2026-10-21-v0.26.17-gcp.md) | v0.26.17 | GCP c2d-standard-8, AMD EPYC 7B13, CPU 1 | every engine again, go-quickjs at v0.28.0 ([pages](../compare/engines/results/2026-10-21-v0.26.17-gcp-pages.md)) |
| [2026-10-21-v0.26.16-gcp](../compare/engines/results/2026-10-21-v0.26.16-gcp.md) | v0.26.16 | GCP c2d-standard-8, AMD EPYC 7B13, CPU 1 | first run, with the tifa pages ([pages](../compare/engines/results/2026-10-21-v0.26.16-gcp-pages.md)). moejs added the same day. jsc2go and fastschema/qjs taken out after the run |
