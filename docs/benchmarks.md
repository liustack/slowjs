---
summary: 'How slowjs compares with the other pure Go JavaScript engines that run current JavaScript: what each supports, how fast each is on server-side rendering and on general workloads, where slowjs is far behind, what limits each has, what slowjs does as tifa runs it, how it ranks against C QuickJS and Node on tifa pages, and how to run the comparison again'
read_when:
  - Saying anything in public about how this engine compares with other Go engines, C QuickJS or Node
  - Choosing a pure Go JavaScript engine for a job
  - Looking for the workloads where this engine is slowest, to make it faster
  - Releasing a new version (rerun the comparison and update this page)
  - Changing compare/engines
---

# Pure Go JavaScript engines compared

Run of 2026-10-29: slowjs v0.29.0, the first release of the C engine
(QuickJS compiled to Go assembly), against five other engines that build
with `CGO_ENABLED=0`, two of them twice (with and without their JIT).
GCP c2d-standard-8 (AMD EPYC 7B13), Linux 7.3, Go 1.27.2, every engine
process on one CPU with its hyperthread sibling idle, GOMAXPROCS=1. The full
tables are in
[`compare/engines/results/2026-10-29-v0.29.0-gcp.md`](../compare/engines/results/2026-10-29-v0.29.0-gcp.md),
the raw numbers in the `.json` next to it. The slowjs numbers are from the
engine code of `main` at `0cc6d332`, the commit v0.29.0 is tagged on. Every
table here is from this run. The earlier runs, made with the engine that was
QuickJS translated to Go by ccgo (up to v0.28.6), are in
[History](#history).

The slowjs rows are named `slowjs` and `slowjs-tifa`. The results files of
runs up to 2026-10-24 name them `liustack-quickjs` and `liustack-tifa`, from
the module's name before v0.28.0 (`github.com/liustack/quickjs`), and have
a `slowjs-tifa-nogoasm` row that the C engine does not need.

## In short

- **Support.** go-quickjs and goant pass 99.3% of test262 (language,
  built-ins and intl402), Temporal and Intl included. slowjs
  passes 83.1%: its Intl has every constructor, but the comparison
  runs it without Temporal (package `temporal` is not registered), and
  Temporal is 13426 of the runs. moejs passes 81.3%. On
  ES2023 and older, every engine passes 99.1% to 99.3%. slowjs has no module
  loader any more: it fails the probes for `import()` and static `import`.
- **Server-side rendering.** On the server bundles of real tifa apps,
  slowjs needs the least CPU of the pure Go engines on every page set (see
  [tifa pages](#tifa-pages)). On the React page of the speed table it
  needs 9.8 ms of CPU, moejs 1.4 times and go-quickjs 1.6 times as much.
- **Against C QuickJS.** On the same JavaScript on the same core, slowjs
  takes 0.98 times the time of C QuickJS 2026-06-04 on the bench pages (which
  need no Intl), 0.81 on the MUI pages and 1.06 on the shop (both on the
  FormatJS polyfill, as C QuickJS has no Intl). With its own Intl the shop
  takes 0.82 and the MUI pages 0.52 of the time C needs on the polyfill.
- **Against Node.** Node with its JIT is 4.8 to 6.1 times as fast as slowjs
  on the same JavaScript, and Node with `--jitless` is 1.7 to 1.9 times as
  fast (see [tifa pages](#tifa-pages)).
- **As tifa runs it.** The tifa pages are also run with slowjs set up the
  way tifa sets it up: its own Intl and Temporal, no polyfill, tifa's Go
  functions and tifa's GC rules (see
  [slowjs as tifa runs it](#slowjs-as-tifa-runs-it)). On the Go side that
  is the only row with features the C QuickJS and Node rows cannot have.
  It takes 0.98 of the time of C QuickJS on the bench pages, 0.79 on the
  shop and 0.38 on the MUI pages.
- **General speed.** slowjs is not the fastest pure Go engine.
  go-quickjs without its JIT (its default) scores 1.2 times as high on the
  V8 suite (3.0 times with it), parses JSON in 81% of the time, runs regular
  expressions 2.4 times faster and is faster on 26 of 63 QuickJS micro
  benchmarks. moejs writes JSON 2.0 times faster, goant parses it 1.8 times
  faster.
- **Far behind.** Nothing is at twice the median of the other pure Go
  engines. The farthest are the empty loop (1.58), strings to integers
  (1.52) and number arithmetic (1.43 to 1.50): see
  [Where slowjs is far behind](#where-slowjs-is-far-behind).
- **Where slowjs leads.** The tifa pages, little memory per VM
  (1.3 MiB RSS, modernc 1.1 MiB, moejs 1.4 MiB, go-quickjs 2.0 MiB), and a
  memory limit that counts live memory and stops a script exactly at it,
  which only modernc shares. It is 3.3 times as fast as modernc.org/quickjs,
  the C-to-Go translation it started from, on the React page.
  A new VM costs more than in the others: 0.36 ms against 0.20 ms for
  go-quickjs, and 2.5 ms to load a compiled program against 1.1 ms.

## Engines

| Engine | Version | What it is | How the comparison uses it |
|---|---|---|---|
| slowjs | v0.29.0 (main `0cc6d332`) | Bellard's QuickJS (pinned commit) in C, compiled by LLVM to Go assembly, with the changes of the fork | bytecode compiled once (`VM.Compile`), run with `EvalBytecode`, Intl from package `intl` |
| slowjs-tifa | v0.29.0 (main `0cc6d332`) | the same engine set up as tifa sets up a VM, for the tifa pages only | packages `intl` and `temporal`, tifa's Go functions, tifa's limits and GC rules, a shared compiled bundle (see [slowjs as tifa runs it](#slowjs-as-tifa-runs-it)) |
| modernc-quickjs | v0.25.0 | QuickJS translated from C to Go by ccgo, the engine slowjs started from | as above |
| go-quickjs-interp | v0.28.0 | QuickJS-NG rewritten in Go | `quickjs.Compile` once, `RunProgram`, JIT off, the default |
| go-quickjs-jit | v0.28.0 | the same with `-tags quickjs_jit` and `WithJIT()` | as above, the JIT is marked experimental |
| goant-interp | v0.2.1 | a JavaScript engine written in Go | `Runtime.Compile` and `RunProgram` per runtime, JIT off, the default |
| goant-jit | v0.2.1 | the same with `WithJIT(true)` | as above |
| moejs | v0.1.0-alpha.11 | a register bytecode engine written in Go, for plugin sandboxes | `moejs.CompileScript` once, `RunScript` on each runtime, runtimes with their own mutable builtins (`MutableIntrinsics`), since test262, the polyfills and the tifa bundles write to them |


goja, sobek and otto are left out: they lack async generators, WeakRef,
newer regular expression flags and more (otto implements ES5 only). jsc2go
and fastschema/qjs were in the first version of the comparison and are no
longer run. Their raw results were not kept (`compare/engines/CHANGELOG.md`).

## Support

| Engine | test262 | ES2023 and older | language | built-ins | intl402 | Temporal | Feature probes (77) | Intl probes (13) |
|---|---|---|---|---|---|---|---|---|
| goant-interp, goant-jit | 99.3% | 99.3% | 98.7% | 99.9% | 99.6% | yes | 73 | 10 |
| go-quickjs-interp, go-quickjs-jit | 99.3% | 99.3% | 98.6% | 99.8% | 100.0% | yes | 77 | 13 |
| slowjs | 83.1% | 99.2% | 97.3% | 76.5% | 36.3% | not registered | 73 | 13 |
| moejs | 81.3% | 99.1% | 97.3% | 74.4% | 24.5% | no | 68 | 9 |
| modernc-quickjs | 81.1% | 99.2% | 97.4% | 76.4% | 8.3% | no | 64 | 3 |

test262 at `d53ebc8b`, 96540 runs per engine (sloppy and strict), module
tests and tests that need host hooks left out for every engine. "built-ins"
counts Temporal, which is 13268 of the runs. Feature probes and Intl probes:
small scripts with a known answer, Intl compared with Node 24. Between two
runs of the same code a few tests change (modernc-quickjs 4 fewer,
goant-interp 9 fewer in the v0.26.17 run). slowjs passes the same 80215 runs
as before the C engine replaced the translated one, and its 58 known errors
are the same ones.

slowjs has had Temporal since v0.27.0, in package `temporal`, but
the comparison registers only package `intl`, so Temporal and the intl402
tests of Temporal (4058 of the 6742 intl402 runs) fail. Apart from them,
intl402 passes 2448 of 2684 (91.2%). Its Intl has every constructor,
Collator (and so a locale-aware `localeCompare`), Locale, DurationFormat,
ListFormat, RelativeTimeFormat, Segmenter and DisplayNames included, and
it passes all 13 Intl probes. What it still lacks, by test262 feature tag:
explicit resource management (`using`, 128 of 820 pass), `Array.fromAsync`,
import attributes for JSON modules, ShadowRealm, `Atomics.waitAsync` and the
newest proposals (`import defer`, iterator chunking and joining, immutable
ArrayBuffer). The C engine has no module loader: the probes for `import()`
and static `import` fail, as do those for JSON modules and proper tail
calls. moejs fails the probes for `Intl.Collator`, `Intl.RelativeTimeFormat`,
`Intl.ListFormat`, `Intl.DisplayNames`, `FinalizationRegistry`, iterator
helpers, `RegExp.escape`, JSON modules and proper tail calls.

Code highlighting is where engines split most. shiki 4.5.0 turns the
TextMate grammars of bash, css, go, html, json, typescript and yaml into 552
regexps, many with the `v` flag, lookbehind and named groups (they are in
`engine/test/compare/recorded/shiki`, and were checked against Node). On every fifth of them, 111 regexps run over the code they were
made for, slowjs, go-quickjs v0.28.0 and moejs
v0.1.0-alpha.9 give what Node gives for all 111 (this was not run again for
this page, and moejs is at alpha.11 now). The others throw on or
match differently for some of them: QuickJS at `19d6bd86` 31, QuickJS at
`3d5e064e` (the commit slowjs's C engine comes from) 37, quickjs-ng at
`7a23d8e3` 14, modernc.org/quickjs v0.25.0 37 and goant v0.2.1 31. On
those engines a page that highlights code with shiki fails or comes out
wrong.

## Speed

CPU time per operation (user and system, all threads), in milliseconds,
median of 5 rounds. Lower is better, except the V8 suite score. In brackets:
against slowjs.

| Engine | React SSR render | V8 suite score | JSON.parse | JSON.stringify | String building | RegExp | QuickJS micro benchmarks (geomean time) |
|---|---|---|---|---|---|---|---|
| slowjs | 9.76 | 1246 | 2.64 | 2.66 | 3.14 | 31.9 | 1.00 |
| modernc-quickjs | 32.0 (3.28) | 374 (0.30) | 6.38 (2.42) | 9.43 (3.55) | 12.8 (4.08) | 84.3 (2.64) | 3.72 |
| go-quickjs-interp | 15.3 (1.57) | 1523 (1.22) | 2.13 (0.81) | 3.14 (1.18) | 7.96 (2.54) | 13.5 (0.42) | 1.09 |
| go-quickjs-jit | 18.5 (1.90) | 3749 (3.01) | 2.17 (0.82) | 3.27 (1.23) | 7.63 (2.43) | 13.4 (0.42) | 0.58 |
| goant-interp | 33.9 (3.47) | 349 (0.28) | 1.43 (0.54) | 2.67 (1.00) | 12.5 (3.98) | 61.2 (1.92) | 5.84 |
| goant-jit | 27.4 (2.81) | 1133 (0.91) | 1.42 (0.54) | 2.69 (1.01) | 10.2 (3.25) | 61.8 (1.94) | 1.99 |
| moejs | 13.6 (1.39) | 816 (0.65) | 1.87 (0.71) | 1.31 (0.49) | 4.85 (1.54) | 39.8 (1.25) | 1.48 |

Wall time tells the same story with one difference: go-quickjs's median
render takes less wall time than its CPU time, because its garbage
collector also works between operations. On the React page its median is
13.7 ms against 9.70 ms for slowjs. CPU time is what a busy server pays.

Cold start and memory, with the React bundle (117 KB):

| Engine | New VM, ms | New VM + load from source | New VM + load compiled program | First render | RSS per VM, MiB | Go heap per VM, MiB |
|---|---|---|---|---|---|---|
| slowjs | 0.362 | 13.8 | 2.48 | 9.97 | 1.3 | 0 |
| modernc-quickjs | 0.256 | 17.6 | 2.28 | 32.2 | 1.1 | 0 |
| go-quickjs-interp | 0.201 | 12.6 | 1.14 | 21.8 | 2.0 | 5.5 |
| go-quickjs-jit | 0.266 | 13.1 | 1.28 | 82.1 | 18.7 | 17.8 |
| goant-interp | 0.582 | 17.1 | n/a | 39.8 | 8.3 | 7.1 |
| goant-jit | 0.551 | 16.0 | n/a | 41.7 | 9.1 | 7.2 |
| moejs | 0.090 | 17.0 | 0.605 | 13.4 | 1.4 | 1.0 |

Memory: what the process grows by per VM with 100 VMs alive, each with the
bundle loaded and one page rendered. goant's Program serves only the runtime
that compiled it (`goant.go` 205 to 208), so it loads from source. moejs
makes each runtime with its own mutable builtins, which its guide says costs
about 25 times as much as its default of frozen builtins shared by every
runtime.

## tifa pages

HTML equality is checked at run time and archived: `cmd/pages --output`
writes per-page SHA256 digests after masking timestamps and the verdict of
whether each page is the same as slowjs's (`same_as_liustack` in the JSON),
including the separate polyfill and deployed comparisons. A missing HTML file
has a null verdict, not a match. The JSON also keeps raw process results, rest
runs, single-page probes and every try from the metadata files.

Server bundles of three tifa apps (`tifa/bundles`), rendered the way tifa
renders them, with the data each app's server sends for each page. Times
are CPU per render (the mean of 200 timed renders after 50 warm ones for
the bench pages, 100 after 20 for the shop and MUI, page by page in one
process), as a ratio to slowjs: geometric mean over the pages, range in
brackets. The C QuickJS and Node rows are references, not pure Go engines.
C QuickJS (`qjs`, built with its own Makefile) is timed in the script with
wall time on a CPU of its own. Node is timed in the script as well, after the
same warm renders, with its JIT (the engine has compiled the hot code by
then) and with `--jitless`. Every page's HTML is compared with
slowjs's, with the router's render timestamps (`u:` and 13
digits) masked. Every page per engine:
[`compare/engines/results/2026-10-29-v0.29.0-gcp-pages.md`](../compare/engines/results/2026-10-29-v0.29.0-gcp-pages.md).

On these pages slowjs is the fastest pure Go JS engine for SSR:
every other pure Go engine needs more CPU per page, or does not render it.

- Pages without Intl: moejs needs 88% more, go-quickjs-interp 91% more,
  go-quickjs-jit 96% more, the others 3.6 to 4.5 times as much.
- The shop, as each engine would deploy it: go-quickjs-interp needs 66%
  more on the 18 pages it renders, go-quickjs-jit 91% more on the same 18,
  the others 3.3 to 5.0 times as much. moejs cannot load the shop bundle
  (its functions are limited to 256 registers).
- The MUI pages as deployed: go-quickjs-interp needs 66% more, go-quickjs-jit
  79% more, moejs 2.3 times and the others 6.0 to 6.6 times as much.

Against the references, below 1 is faster than C QuickJS 2026-06-04 on the
same pages (geometric mean over the pages of a bundle, range in brackets).
Bench has no Intl, MUI and the shop need it, so every row but the
slowjs-tifa row and the "own Intl only" row runs them on the polyfill.

| Row | bench (2 pages) | MUI (3 pages) | shop (20 pages) |
|---|---|---|---|
| slowjs, same JavaScript (polyfill) | 0.98 (0.93–1.04) | 0.81 (0.74–0.94) | 1.06 (0.98–1.15) |
| slowjs, own Intl only (no Temporal, no Go functions, no tifa GC rules) | 0.98 (0.93–1.04) | 0.52 (0.39–0.85) | 0.82 (0.71–1.07) |
| **slowjs-tifa, as tifa runs it** | **0.98 (0.93–1.04)** | **0.38 (0.29–0.62)** | **0.79 (0.68–0.98)** |
| C QuickJS 2026-06-04 | 1.00 | 1.00 | 1.00 |
| Node v24.11.1 | 0.16 (0.12–0.21) | 0.16 (0.13–0.20) | 0.22 (0.18–0.29) |
| Node v24.11.1 `--jitless` | 0.52 (0.49–0.56) | 0.48 (0.43–0.57) | 0.58 (0.53–0.62) |

- **Against C.** On the same JavaScript slowjs takes the time of C QuickJS
  on the bench pages (0.98), is 19% faster on MUI and 6% slower on the shop.
  The difference to native code is small.
- **Against Node.** On the same JavaScript, Node with its JIT is 6.1 times as
  fast as slowjs on the bench pages, 5.1 times on MUI and 4.8 times on the
  shop. Node with `--jitless` is 1.9, 1.7 and 1.8 times as fast. With each on
  its own Intl, Node takes 0.21 of the time of slowjs on the shop and MUI
  pages with the JIT and 0.55 and 0.64 without it (the "as deployed" tables
  below).
- C QuickJS master at `7d1c13c5d8` takes 1.02 times the time of slowjs on the
  bench pages, as the 2026-06-04 release does. QuickJS-NG v0.17.0 takes 1.36
  times.

### Which pages need Intl

Deleting `Intl` and replacing `localeCompare` and the `toLocale*` methods
with stubs that throw leaves only the bench app's two pages unchanged:

| App | Pages | Same HTML without Intl | What reaches Intl first |
|---|---|---|---|
| bench | table, spiral | yes, both | nothing |
| mui | home, order, orders | no | the app's own currency `Intl.NumberFormat('en-US', {style: 'currency'})` and `Intl.DateTimeFormat`, home also `Number.prototype.toLocaleString('en-US')`, MUI's TablePagination checks for Intl |
| shop | all 20 | the bundle does not load | `new Intl.NumberFormat('zh-CN', {style: 'currency', currency: 'CNY'})` at the top level of the bundle, then i18next (`Intl.getCanonicalLocales`, `Intl.PluralRules`) and the pages' `Intl.DateTimeFormat` with a time zone |

So only slowjs, go-quickjs and Node can render the shop and MUI
pages as they are. To compare everyone on the shop, it was rendered twice
more: with the FormatJS polyfills (pinned in `tifa/polyfill`, zh-CN and
en-US data and time zone data, 2.4 MB) loaded before the bundle, and as each
engine would deploy it.

### Pages without Intl


| Engine | Load ms | Pages that render | Same page as slowjs | Time per render vs slowjs |
|---|---|---|---|---|
| slowjs | 71 | 2 of 2 | 2 of 2 | 1.00 (1.00–1.00) |
| slowjs-tifa | 62 | 2 of 2 | 2 of 2 | 1.00 (1.00–1.00) |
| modernc-quickjs | 76 | 2 of 2 | 2 of 2 | 3.57 (3.54–3.60) |
| go-quickjs-interp | 72 | 2 of 2 | 2 of 2 | 1.91 (1.90–1.92) |
| go-quickjs-jit | 71 | 2 of 2 | 2 of 2 | 1.96 (1.96–1.96) |
| goant-interp | 76 | 2 of 2 | 2 of 2 | 4.53 (4.17–4.92) |
| goant-jit | 80 | 2 of 2 | 2 of 2 | 4.00 (3.63–4.42) |
| moejs | 93 | 2 of 2 | 2 of 2 | 1.88 (1.80–1.96) |
| C QuickJS 2026-06-04 (reference) | 39 | 2 of 2 | 2 of 2 | 1.02 (0.97–1.08) |
| C QuickJS master 7d1c13c5d8 (reference) | 40 | 2 of 2 | 2 of 2 | 1.02 (0.97–1.07) |
| QuickJS-NG v0.17.0 (reference) | 34 | 2 of 2 | 2 of 2 | 1.36 (1.30–1.43) |
| Node v24.11.1 (reference) | 17 | 2 of 2 | 2 of 2 | 0.16 (0.13–0.20) |
| Node v24.11.1 --jitless (reference) | 16 | 2 of 2 | 2 of 2 | 0.53 (0.53–0.54) |

| Page | slowjs | slowjs-tifa | modernc-quickjs | go-quickjs-interp | go-quickjs-jit | goant-interp | goant-jit | moejs | quickjs-2026-06-04 | quickjs-master | quickjs-ng | node-jit | node-jitless |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spiral | 20.5 (1.00) | 20.5 (1.00) | 72.8 (3.54) | 39.4 (1.92) | 40.2 (1.96) | 101 (4.92) | 90.8 (4.42) | 40.3 (1.96) | 19.8 (0.97) | 20 (0.97) | 26.7 (1.30) | 4.07 (0.20) | 11.1 (0.54) |
| table | 18.4 (1.00) | 18.4 (1.00) | 66.1 (3.60) | 35 (1.90) | 36 (1.96) | 76.6 (4.17) | 66.7 (3.63) | 33 (1.80) | 19.8 (1.08) | 19.6 (1.07) | 26.3 (1.43) | 2.42 (0.13) | 9.69 (0.53) |


The C QuickJS rows show how close the generated Go assembly is to native
code: on these pages slowjs takes 0.98 times the time of the C it is
compiled from, on the polyfilled shop 1.06 times.

### Shop on the FormatJS polyfill, every engine

Native `Intl` deleted first in every engine, including the three that have
one, so all run the same JavaScript. Load ms: the polyfill, then the bundle.


| Engine | Load ms | Pages that render | Same page as slowjs | Time per render vs slowjs |
|---|---|---|---|---|
| slowjs | 572 | 20 of 20 | 20 of 20 | 1.00 (1.00–1.00) |
| slowjs-tifa | not run: this row has no polyfill | | | |
| modernc-quickjs | 956 | 20 of 20 | 20 of 20 | 2.54 (2.41–2.84) |
| go-quickjs-interp | 811 | 18 of 20 | 18 of 18 | 1.67 (1.50–1.84). store-en-US-post, store-zh-CN-post fail, a failure leaves the runtime closed, the other pages measured in a run without the failing ones |
| go-quickjs-jit | 822 | 2 of 20 | 2 of 2 | 1.76 (1.75–1.77). every page but store-en-US-products, store-zh-CN-products fails, a failure leaves the runtime closed, the other pages measured in a run without the failing ones |
| goant-interp | 1020 | 20 of 20 | 20 of 20 | 3.85 (3.47–4.27) |
| goant-jit | 1044 | 20 of 20 | 20 of 20 | 3.69 (3.05–4.12) |
| moejs | load: SyntaxError: function needs more than 256 registers is not supported yet | 0 of 20 | | |
| C QuickJS 2026-06-04 (reference) | 380 | 20 of 20 | 20 of 20 | 0.95 (0.87–1.02) |
| C QuickJS master 7d1c13c5d8 (reference) | 383 | 20 of 20 | 20 of 20 | 0.95 (0.85–1.07) |
| QuickJS-NG v0.17.0 (reference) | 356 | 20 of 20 | 18 of 20 | 1.17 (1.10–1.24) |
| Node v24.11.1 (reference) | 210 | 20 of 20 | 20 of 20 | 0.21 (0.17–0.28) |
| Node v24.11.1 --jitless (reference) | 198 | 20 of 20 | 20 of 20 | 0.55 (0.50–0.59) |


QuickJS-NG's two different pages (`wb-order-studio`, `wb-order-weekend`)
have one more React text separator (`<!-- -->`) after "选择快递公司" in the
select value. moejs compiles the polyfill and the other two bundles, and
stops at the shop bundle's outer function, at line 1, column 2.

### Shop as deployed

Each engine with its own Intl if it has one, else with the polyfill.
Against slowjs with its own Intl.


| Engine | Load ms | Pages that render | Same page as slowjs | Time per render vs slowjs |
|---|---|---|---|---|
| slowjs (own Intl) | 283 | 20 of 20 | 20 of 20 | 1.00 (1.00–1.00) |
| slowjs-tifa (own Intl) | 306 | 20 of 20 | 20 of 20 | 0.97 (0.87–1.03) |
| modernc-quickjs (polyfill) | 956 | 20 of 20 | 20 of 20 | 3.29 (2.77–3.77) |
| go-quickjs-interp (own Intl) | 533 | 18 of 20 | 18 of 18 | 1.66 (1.53–1.81). store-en-US-post, store-zh-CN-post fail, a failure leaves the runtime closed, the other pages measured in a run without the failing ones |
| go-quickjs-jit (own Intl) | 547 | 18 of 20 | 18 of 18 | 1.91 (1.73–2.17). store-en-US-post, store-zh-CN-post fail, a failure leaves the runtime closed, the other pages measured in a run without the failing ones |
| goant-interp (polyfill) | 1020 | 20 of 20 | 20 of 20 | 4.98 (3.68–5.84) |
| goant-jit (polyfill) | 1044 | 20 of 20 | 20 of 20 | 4.77 (3.15–5.66) |
| moejs (polyfill) | load: SyntaxError: function needs more than 256 registers is not supported yet | 0 of 20 | | |
| C QuickJS 2026-06-04 (reference) (polyfill) | 380 | 20 of 20 | 20 of 20 | 1.23 (0.93–1.41) |
| C QuickJS master 7d1c13c5d8 (reference) (polyfill) | 383 | 20 of 20 | 20 of 20 | 1.23 (0.91–1.41) |
| QuickJS-NG v0.17.0 (reference) (polyfill) | 356 | 20 of 20 | 18 of 20 | 1.52 (1.19–1.75) |
| Node v24.11.1 (reference) (own Intl) | 151 | 20 of 20 | 20 of 20 | 0.21 (0.17–0.30) |
| Node v24.11.1 --jitless (reference) (own Intl) | 90 | 20 of 20 | 20 of 20 | 0.55 (0.50–0.58) |


### MUI pages as deployed

The three MUI pages the same way. The pages of the engines with the polyfill
differ from slowjs's in one character: the FormatJS polyfill writes the
narrow no-break space U+202F before `AM` and `PM`, where Go's `Intl` and Node
write a plain space (see below), so only the engines with an `Intl` of their
own give the same page as slowjs.


| Engine | Load ms | Pages that render | Same page as slowjs | Time per render vs slowjs |
|---|---|---|---|---|
| slowjs (own Intl) | 143 | 3 of 3 | 3 of 3 | 1.00 (1.00–1.00) |
| slowjs-tifa (own Intl) | 149 | 3 of 3 | 3 of 3 | 0.73 (0.73–0.74) |
| modernc-quickjs (polyfill) | 742 | 3 of 3 | 0 of 3 | 6.34 (3.63–8.92) |
| go-quickjs-interp (own Intl) | 211 | 3 of 3 | 3 of 3 | 1.66 (1.61–1.70) |
| go-quickjs-jit (own Intl) | 217 | 3 of 3 | 3 of 3 | 1.79 (1.74–1.84) |
| goant-interp (polyfill) | 728 | 3 of 3 | 0 of 3 | 6.64 (4.72–8.10) |
| goant-jit (polyfill) | 712 | 3 of 3 | 0 of 3 | 6.02 (4.21–7.45) |
| moejs (polyfill) | 446 | 3 of 3 | 0 of 3 | 2.27 (1.64–2.74) |
| C QuickJS 2026-06-04 (reference) (polyfill) | 291 | 3 of 3 | 0 of 3 | 1.91 (1.18–2.56) |
| C QuickJS master 7d1c13c5d8 (reference) (polyfill) | 285 | 3 of 3 | 0 of 3 | 1.92 (1.19–2.57) |
| QuickJS-NG v0.17.0 (reference) (polyfill) | 296 | 3 of 3 | 0 of 3 | 2.00 (1.46–2.38) |
| Node v24.11.1 (reference) (own Intl) | 57 | 3 of 3 | 3 of 3 | 0.21 (0.18–0.22) |
| Node v24.11.1 --jitless (reference) (own Intl) | 55 | 3 of 3 | 3 of 3 | 0.64 (0.64–0.65) |
### slowjs as tifa runs it

Two rows of slowjs run the tifa pages, and they measure different things.

- **slowjs, same JavaScript.** The row of every table above. Native
  `Intl` is deleted and the FormatJS polyfill is loaded, in every engine, so
  all engines run the same JavaScript. It answers how fast the engine is at
  running a given bundle.
- **slowjs-tifa, as tifa runs it.** slowjs set up as tifa sets up a VM of
  its pool (`engine/eng_liustacktifa.go`, copied from `internal/ssr` of tifa,
  not imported). It answers how fast a tifa app renders on slowjs, counting
  the Go side as part of what slowjs offers. It has:
  - its own Go `Intl` (package `intl`) and `Temporal` (package `temporal`),
    registered as tifa registers them, and no polyfill
  - the Go functions tifa registers: `__tifa_murmur2__` (the hash of
    `@emotion/hash`, which makes MUI's styles) and `__tifa_random_bytes__`
    (secure random bytes for `crypto`). These are all of them: the other
    `__tifa_*` name in tifa, `__tifa_intl_warnings__`, is JavaScript that
    only `tifa dev` loads
  - tifa's defaults for the VM: a memory limit of 128 MiB, a stack of 10000
    slots, a time budget of 3 s that is set again before each step of a
    render, and the VM clock
  - tifa's GC rules: a full GC and a GC threshold of twice the live size after
    loading, a young GC after each render and a full GC after 256 renders or
    when the young GC left 1.5 times the live size, as `internal/ssr/pool.go`
    has it. This work is in the CPU time of a render and not in its wall time,
    as in tifa, where it runs after the page is sent
  - a bundle compiled once into a `SharedBytecode` that the VM loads
  - a render made of `startRender(input)`, the promise jobs and the read of
    the result, as `ssr.RenderVM` does it

The pinned bundles in `tifa/bundles` were built before the tifa plugin made
`murmur2` ask `__tifa_murmur2__`. The adapter makes the same one-line change
in the text of a bundle that has `@emotion/hash` (only MUI): a result file
counts it (`bundle_rewrites`) and the calls of the Go function
(`murmur2_calls`). No pinned bundle calls `__tifa_random_bytes__`, and the
shop and bench bundles do not hash with `@emotion/hash`, so on those the Go
functions are registered and never called.

The C QuickJS and Node rows have no row like this one and cannot have one.
C QuickJS has no `Intl` and no `Temporal` of its own, so its only way to
render these pages is the polyfill. It has no host to give it Go functions:
`qjs` is the stock program, and a C host with a fast `murmur2`, a GC policy
and its own `Intl` would be a new program, not the engine the row is named
for. Node has a complete `Intl` of its own (its "own Intl" rows, which stay)
but not tifa's functions or rules. So those rows stay references for the same
JavaScript, and the second slowjs row shows what the Go side adds on top.

Run of 2026-10-24 on the GCP machine, CPU 1 and its sibling idle, Go 1.27.2,

Run of 2026-10-29, as above ([results](../compare/engines/results/2026-10-29-v0.29.0-gcp-pages.md)):

- On the bench pages the tifa row takes the time of the plain row (1.00): they
  use neither Intl nor the Go functions, and the young GCs after each render
  cost about what the engine saves elsewhere.
- On the shop it takes 0.97 of the time of the plain own-Intl row (0.87 to
  1.03 by page). The two blog post pages are 0.87 to 0.92, where the young GC
  after each render pays most.
- On MUI it takes 0.73 of the time of the plain own-Intl row. That is the hash
  in Go (`__tifa_murmur2__`) together with the rest of the tifa setup. The two
  were not measured apart.
- Against the same JavaScript on the polyfill, the plain own-Intl rows take 0.77
  (shop) and 0.64 (MUI) of the time. The polyfill costs more than the engine
  does: see [What the polyfill costs](#what-the-polyfill-costs).

HTML of the tifa row against the polyfill row, page by page, with the
render timestamps masked:


| Bundle | Pages | Same HTML | Same but for U+202F | Other difference | Not compared |
|---|---|---|---|---|---|
| bench | 2 | 2 | 0 | 0 | 0 |
| mui | 3 | 0 | 3 | 0 | 0 |
| shop | 20 | 20 | 0 | 0 | 0 |


The three MUI pages differ in one character. The FormatJS polyfill writes the
narrow no-break space U+202F between a time and `AM` or `PM` (`08:00 AM`),
where Go's `Intl` writes a plain space. Node writes a plain space as well (the
Node own-Intl rows have the same MUI pages as the tifa row, 3 of 3), so the
polyfill is the one that differs. With U+202F
changed to a space the pages are byte for byte the same.

### What the polyfill costs

On slowjs, from the same runs:

- Loading it: 291 ms per VM before the bundle. The bundle then loads in
  281 ms, 283 ms without the polyfill.
- Memory per VM (20 VMs, each with the bundle loaded, the polyfill too
  where named, and the zh-CN cart page rendered):
  38.6 MiB RSS with the polyfill against 17.4 MiB with its own Intl.
  go-quickjs-interp: 111.5 MiB against 48.5 MiB (Go heap 80.4 against
  39.9 MiB).
- Share of a render: summed over the 20 pages, 149 of 674 ms of a render is
  spent inside the polyfill (22%, 1354 calls), against 4.8 of 520 ms inside
  its own Intl (1%) for the same calls.

go-quickjs-interp fails on the two blog post pages, with its own Intl and on
the polyfill alike. go-quickjs-jit fails on the same two with its own Intl,
and on 18 of the 20 pages on the polyfill (see
[Problems found](#problems-found-in-other-engines)).

## Where slowjs is far behind

Every QuickJS micro benchmark, V8 suite benchmark and workload of the speed
table where slowjs needs at least twice the time of the median of
the six other pure Go builds (for the V8 suite: half the score). In this run
there is none. The closest, at 1.2 times the median or more:

| Item | slowjs vs the median | Fastest |
|---|---|---|
| `empty_loop` | 1.58 | go-quickjs-jit |
| `string_to_int` | 1.52 | go-quickjs-jit |
| `float_arith` | 1.50 | go-quickjs-jit |
| `int_arith` | 1.43 | go-quickjs-jit |
| `string_to_float` | 1.42 | moejs |
| `array_for` | 1.34 | go-quickjs-jit |
| JSON.parse | 1.32 | goant-jit |
| V8 Crypto | 1.30 (score) | go-quickjs-jit |
| `empty_down_loop` | 1.27 | go-quickjs-jit |
| `empty_down_loop2` | 1.26 | go-quickjs-jit |

The medians are of modernc-quickjs, go-quickjs-interp, go-quickjs-jit,
goant-interp, goant-jit and moejs. `go run ./cmd/behind
results/<run>.json [limit]` in `compare/engines` makes the list again, for
the limit you give (2.0 by default). In the last run before the C engine
(v0.27.2) eleven items were at 2.0 to 2.9, the empty loops and number
arithmetic first. Single micro benchmarks of tiny loops move
between two builds of the same code, as the code lands at other addresses:
modernc-quickjs, the same code in two earlier runs, took 0.8 to 2.5
times its earlier time on single tests, though 1.05 times on their
geometric mean. Read one test's ratio as rough.

## Where the others are faster

In this run, with the ratios to slowjs from the speed table (below 1 is
faster than slowjs):

- **Numbers in loops and the V8 suite.** go-quickjs-interp scores 1.22 times as
  high on the V8 suite (Crypto 1.7, NavierStokes 1.6, RegExp 2.8 times), and
  go-quickjs-jit 3.0 times (NavierStokes 7.2, Crypto 6.9). go-quickjs keeps
  numbers as NaN-boxed floats (`internal/vm/value.go` 81), has nodes for
  int32 arithmetic (`tree_build.go` 1639, `tree_operand.go` 9), for array
  element reads (`vm_tree.go` 267) and for a whole loop (`ztree_flow.go`
  1140). In the go-quickjs-jit and goant-jit profiles of the earlier run,
  generated machine code dominated.
- **Property reads and calls.** go-quickjs caches the shape and slot at each
  access site (`internal/vm/vm_shape.go` 807 and 1019, read in
  `tree_build.go` 2214), runs hot functions as a tree of Go closures with no
  operand stack (`internal/vm/vm_tree.go`, `zcall_tree.go` 86), updates a
  property in place in one node (`ztree_update.go` 601) and calls small pure
  functions without a frame (`vm_leaf_pure.go` 179). moejs has an inline
  cache per instruction (its guide, `docs/guide.md`, "Implementation").
- **JSON** (JSON.parse: goant 0.54, moejs 0.71, go-quickjs 0.81 times the
  CPU time of slowjs. JSON.stringify: moejs 0.49). goant parses and writes with
  code of its own over the string (`internal/engine/builtin_json.go` 571,
  870, 190) and caches the shape changes of the objects it builds
  (`shapes.go` 404). moejs's JSON is native Go and builds objects on known
  shapes.
- **Regular expressions** (go-quickjs 0.42). go-quickjs's regular expressions
  are profiled separately. moejs runs the patterns it can translate exactly
  on Go's RE2 (1.25 times the time of slowjs on this workload).
- **Strings to numbers** (`string_to_int`, `string_to_float`). go-quickjs has
  a path for plain integers (`internal/jsnum`), goant calls Go's `strconv`.
- **Copying arrays, destructuring arrays** (`array_slice`, `local_destruct`).
  go-quickjs reads the dense elements of the source and appends them to the
  new array's storage (`internal/vm/builtins.go` 1228), and skips the
  iterator protocol for arrays that still iterate the way `Array.prototype`
  does (`internal/vm/iterate.go` 608).
- **Loading a compiled program** (go-quickjs 0.46 times the time of slowjs,
  moejs 0.24). go-quickjs shares one compiled program between runtimes and
  only prepares it (`internal/vm/eval.go` 18) before running the top level. A
  moejs `Script` is immutable and runs on any runtime. slowjs reads the
  bytecode into every new VM, then runs the bundle's top level.

The file and line numbers are for go-quickjs v0.28.0. No profile of the C
engine against these engines was taken for this page.

## Limits

| Engine | Memory limit per VM | Stopping a script | Recursion depth (probe) | JIT platforms | Other |
|---|---|---|---|---|---|
| slowjs | exact: the allocator refuses past the limit and the script gets an uncatchable out of memory (`VM.SetMemoryLimit`) | `Interrupt`, uncatchable | 4996: the default is 5000 calls, `SetMaxStackSize` raises it to 10000 at most | no JIT | one VM per goroutine at a time, VMs independent. linux/amd64 and darwin/arm64 only |
| modernc-quickjs | exact, same allocator | interrupt handler | crashes the process (Go stack overflow) instead of a RangeError | no JIT | `tail-call` probe also crashes the process |
| go-quickjs | approximate: the heap is walked once the process has allocated a quarter of the limit since the last walk (`docs/known-issues.md` KI-02) | context or abort | 8189 | `quickjs_jit` tag, experimental, linux/amd64, windows/amd64, darwin/arm64 (`internal/jit` build tags) | without a deadline, a regular expression stops after a step budget with `SyntaxError: regular expression is too complex` (KI-17) |
| goant | partial: arrays went past it, a string grew past it without an error, objects stopped (probe). Compiled code is outside it (`goant.go` 576 to 585) | `Interrupt` | 8189 (12044 with JIT) | amd64, arm64 (`internal/jitasm`) | the inline cache counter is process-wide, so a collection or a prototype change in one runtime makes every runtime's caches refill (`internal/engine/shapes.go` 28 to 48). `setTimeout`, `setImmediate`, `structuredClone` and `process` are present by default |
| moejs | counts what a runtime allocates between resets, not what is live, and stops the script uncatchably past it (`Options.MemoryLimit`, `docs/guide.md` "Memory limit"). It stopped all three probes | `Interrupt`, uncatchable | 508: the call depth is fixed at 512 (`engine/function.go` 332) | no JIT | a function can use at most 256 registers (`compiler/compiler.go` 452, 483, 1043), which the tifa shop bundle goes past. Builtins are frozen and shared by every runtime unless `MutableIntrinsics` is set. No timers unless the host adds its `eventloop` package |

File and line numbers are for the versions in the Engines table.

## Which engine for what

- **slowjs:** server-side rendering, many small VMs in one process
  with hard memory limits, a fresh VM per request, and pages that need
  Intl or Temporal (package `temporal`, which this comparison does not
  measure), on linux/amd64 and darwin/arm64.
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
  register limit and its partial Intl rule out large bundles like the tifa
  shop.
- **modernc-quickjs:** the C-to-Go translation slowjs started from.
  Slower on every workload.

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
  and again in the v0.27.1, v0.27.2 and v0.29.0 runs. Those runs do not
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
go run ./cmd/compare -engines slowjs,go-quickjs-interp
go run ./cmd/compare -engines slowjs -phases t262   # one engine, one workload
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

To time a change to this engine against the engine before it, not against
other engines, use the tool in [testing.md](testing.md#developer-tools-generate-check-time):
it builds both at several code positions and reads the result by the rule of
[Code positions](performance.md#code-positions).

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

The two C QuickJS rows are built by the Makefile that ships with them, which
defines no `NDEBUG`, so their `assert` calls run, while QuickJS-NG's CMake
Release build defines `-DNDEBUG`. A row of C QuickJS built for speed adds
`-DNDEBUG` to `DEFINES` in its Makefile. Say which one a table shows.

The tifa pages and the native reference rows have a script of their own
(Linux, CPU 1 and its sibling CPU 5):

```sh
cd compare/engines
tifa/polyfill/build.sh                      # Node and npm: the FormatJS bundle
native/build.sh "$HOME/jsnative"            # C QuickJS, QuickJS-NG and Node
run="<date>-<version>-<host>"
page_dir=$(mktemp -d "/tmp/$run-pages.XXXXXX")
tifa/run.sh "$page_dir" bin-linux-amd64 "$HOME/jsnative"
go run ./cmd/pages "$page_dir" --output "results/$run-pages"
```

`tifa/run.sh` runs the bench, MUI and shop bundles. `TIFA_ONLY=<engine>,...`
(names as in the result files) runs only those engines, which cuts a run into
pieces on a shared machine, and `TIFA_CPU` and `TIFA_SIBLING` pin the runs to
another CPU and its hyperthread sibling than CPU 1 and 5. The check, timer and
memory runs belong to `slowjs`.

`TIFA_BUNDLES=<dir>` runs another set of bundles and inputs, laid out as
`tifa/` (`bundles/<app>.js`, `in/<app>/*.json`), in place of the pinned ones.
tifa builds its SSR bundle as an ES module: put it there as
`bundles/<app>.mjs`, which the `slowjs-tifa` rows compile as a module, and
as `bundles/<app>.js` made by `go run ./cmd/modulescript <app>.mjs <app>.js`,
which every other row runs as a script.

When a page leaves an engine unusable (go-quickjs closes its runtime after
a page fails), run the shop again without the failing pages into
`<engine>-<kind>-rest.json` in the same directory: `cmd/pages` takes the
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
- The tifa row (`slowjs-tifa`) is built into a binary of its own like the
  others (tag `eng_liustacktifa`), and `cmd/compare` builds it with them but
  does not run it on the other workloads unless `-engines` names it. It runs
  only in `tifa/run.sh`.
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
| QuickJS micro benchmarks | `compare/testdata`, QuickJS's own micro benchmarks, one process per test, as time per iteration against slowjs. |
| Cold start | A new VM, a new VM with the bundle loaded from source, from a compiled program (checked to render the same page), and the first render. |
| Memory | Growth of the process per VM with 100 VMs alive, each with the bundle loaded and one page rendered. |
| Features | 77 probes from ES2015 to ES2025, Intl (compared with Node 24's answers) and limits. |
| test262 | `test/language`, `test/built-ins`, `test/intl402`, sloppy and strict, a fresh VM per run. Module tests and tests that need host hooks (`$262.createRealm` and the like) are left out for every engine. |
| tifa pages | Server bundles of three tifa apps (`tifa/bundles`), rendered the way tifa renders them: `startRender(json)`, then the promise jobs, then `renderResult`. Inputs are the data the apps' servers send for each page. |

## History

| Run | slowjs | Machine | Notes |
|---|---|---|---|
| [2026-10-29-v0.29.0-gcp](../compare/engines/results/2026-10-29-v0.29.0-gcp.md) | v0.29.0 (main `0cc6d332`) | GCP c2d-standard-8, AMD EPYC 7B13, CPU 0 and its sibling 4 | every engine again, the first run of the C engine ([pages](../compare/engines/results/2026-10-29-v0.29.0-gcp-pages.md)). Timed workloads, then the tifa pages, then test262 on both cores, one after the other. go-quickjs failed pages were measured in rest runs. The row `slowjs-tifa-nogoasm` is gone. The version column of the Engines table in the results file reads `v0.26.16 => ../../`, as `go.mod` of the comparison was not moved: the engine is the checkout |
| [2026-10-24-v0.28.6-gcp-pages](../compare/engines/results/2026-10-24-v0.28.6-gcp-pages.md) | v0.28.6 | GCP c2d-standard-8, AMD EPYC 7B13, CPU 3 | tifa pages only, on the bundles tifa c729ad2 builds rather than the pinned ones: slowjs three times (same JavaScript, as tifa runs it, and as tifa runs it without the Go assembly interpreter), C QuickJS 2026-06-04 and master, QuickJS-NG and Node with and without its JIT. Core pair 3-7 under a lock. The other engines were not run again |
| [2026-10-24-main-tifa-row-gcp-pages](../compare/engines/results/2026-10-24-main-tifa-row-gcp-pages.md) | main (c17092a) | GCP c2d-standard-8, AMD EPYC 7B13, CPU 1 | tifa pages only: slowjs twice (same JavaScript, and as tifa runs it), C QuickJS 2026-06-04 and Node with and without its JIT, with the MUI pages timed for the first time. Core pair 1-5 under a lock. The other engines were not run again |
| [2026-10-23-v0.27.2-gcp](../compare/engines/results/2026-10-23-v0.27.2-gcp.md) | v0.27.2 | GCP c2d-standard-8, AMD EPYC 7B13, CPU 1 | every engine again at the same versions ([pages](../compare/engines/results/2026-10-23-v0.27.2-gcp-pages.md)). test262 run on CPU 0 at low priority and merged in, the same 80215 runs pass. React page 15.1 ms of CPU rather than 14.9, V8 score 835 rather than 854, JSON and regexps 1% to 3% faster. `string_to_int` and `array_read` cross the far-behind line, `string_to_float` drops under it |
| [2026-10-22-v0.27.1-gcp](../compare/engines/results/2026-10-22-v0.27.1-gcp.md) | v0.27.1 | GCP c2d-standard-8, AMD EPYC 7B13, CPU 1 | every engine again at the same versions ([pages](../compare/engines/results/2026-10-22-v0.27.1-gcp-pages.md)). test262 run on CPU 0 at low priority and merged in. React page 14.9 ms of CPU rather than 16.3, V8 score 854 rather than 800, `Date.now()` no longer far behind, all 13 Intl probes pass |
| [2026-10-21-v0.26.17-gcp](../compare/engines/results/2026-10-21-v0.26.17-gcp.md) | v0.26.17 | GCP c2d-standard-8, AMD EPYC 7B13, CPU 1 | every engine again, go-quickjs at v0.28.0 ([pages](../compare/engines/results/2026-10-21-v0.26.17-gcp-pages.md)) |
| [2026-10-21-v0.26.16-gcp](../compare/engines/results/2026-10-21-v0.26.16-gcp.md) | v0.26.16 | GCP c2d-standard-8, AMD EPYC 7B13, CPU 1 | first run, with the tifa pages ([pages](../compare/engines/results/2026-10-21-v0.26.16-gcp-pages.md)). moejs added the same day. jsc2go and fastschema/qjs taken out after the run |
