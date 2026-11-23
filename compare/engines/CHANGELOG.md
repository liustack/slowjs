# Changes to the engine comparison

## 2026-11-16: the run of v0.31.3

- The bundles and inputs of the pages are made again from tifa 63a2099e (v0.0.10), so the page table of this run is not to be set beside the one of v0.31.2: the shop bundle has i18next 27.1.2 and react-i18next 18.0.1 (were 26.5.3 and 17.1.0), and the inputs are new.
- Node moved from v24.22.3 to v26.13.1 and Hermes V1 from 260318099.0.0 to 260318099.0.7, the newest of each. go-quickjs v0.38.0, moejs v0.1.1 and C QuickJS 2026-06-04 are still the newest.
- The run is on server C after its restart on another host of the same type, so its numbers are not comparable with those of the run of v0.31.2. The V8 suite has ten rounds in one session.

## 2026-11-16: the run of v0.31.2

- The run moved to another machine of the same type (server C, GCP t2d-standard-4), so its numbers are not comparable with those of the run of v0.31.1. moejs is v0.1.1 (`go.mod`, `go.sum`), still with the `FinalizationRegistry` stand-in.
- The page table is paired: each engine runs in two slots, and in each slot liustack-slowjs runs in the same hold of the machine, before the engine in one slot and after it in the other. The table shows the median of the two slots' ratios with the lower and higher slot in brackets. The V8 suite has ten rounds in two sessions of five, and its ratios are medians of the ratio within each round.

## 2026-11-16: the run of v0.31.1

- moejs is timed in the same session as the other six page engines now, not in a session of its own, so its ratios come from the same session as the others'.
- go-quickjs moved from v0.37.3 to v0.38.0 (`go.mod`, `go.sum`). moejs stays at v0.1.0, its newest release.
- The V8 round has six sides: the three Go engines, C QuickJS, Hermes and Node `--jitless`, each once per round in a turning order. `liustack-slowjs-polyfill` is not in it.

## 2026-11-15: liustack-slowjs-polyfill, Node in the V8 suite

- The page table has a second row for liustack/slowjs, `liustack-slowjs-polyfill`: the bare engine, with none of the packages `intl`, `temporal` and `url` and with the polyfills an engine without those features gets. It has its own adapter (`eng_liustackpolyfill`), binary (`liustackpolyfill`), `tifa/run.sh` runs (`TIFA_ONLY=liustack-slowjs-polyfill`, kinds `bench`, `pf` and `mui-pf`) and column in `cmd/pages`. Ratios stay against `liustack-slowjs`. `cmd/compare -build-only` builds it, and the V8 suite leaves it out unless `-engines` names it.
- The V8 suite has a row for Node `--jitless` (`native/node-v8.cjs`, run as `node --jitless native/node-v8.cjs <dir>` on one core, like `qjs-v8.js` and `hermes-v8.js`). It is part of the round that README.md describes.

## 2026-11-15: rows named liustack-slowjs

- The module is `github.com/liustack/slowjs` again, so its row is
  `liustack-slowjs`, in the results, the reports, `TIFA_ONLY`, `-engines`
  and the file names of `tifa/run.sh`. Results files made while the module was
  `github.com/liustack/quickjs` (v0.30.0 to v0.30.7) name the row
  `liustack-quickjs`, those of 2026-10-24 to 2026-10-29 name it `slowjs`.
  `-report` reads all three. Saved results files are not rewritten, so `-add`
  and `-merge` into one of them would give the engine two rows: start a new run
  instead. `cmd/pages` reads the raw files of `tifa/run.sh` under the new name
  only, so make the page tables of a raw run of v0.30.7 or older with the tools
  of that release.

## 2026-11-14: moejs renders the pages, test262 removed

- moejs renders all 25 pages now. The bundles read `FinalizationRegistry` at load, which moejs v0.1.0 lacks, and the new `native/moejs-shim.js` gives it a registry that never calls back (no call of the callback is promised, so that is a valid one). The HTML is byte-identical to ours on all 25 pages. moejs was timed in a session of its own, with liustack-quickjs run again in the same session, and its ratios are against that run ([2026-11-14-v0.30.7-gcp-pages-moejs](results/2026-11-14-v0.30.7-gcp-pages-moejs.md)).
- test262 is removed: the `t262` package, the `jsbench` subcommand, the driver phase and its flags, the merge of test262 results and the report section are deleted, with the driver tests adjusted. `-merge` now only takes `-merge-engines`.

## 2026-11-14: two tables, interpreters only, new bundles

- The comparison is two tables: the render time of tifa's pages and the V8 suite.
  The feature probes, the QuickJS micro benchmarks, cold start, memory,
  concurrency, the far-behind list (`cmd/behind`), the liustack-tifa row
  (`eng_liustacktifa*.go`) and every table of the engine's own Intl against the
  polyfill are gone from the run. `cmd/compare` runs `v8` by default, and the
  comparison never runs test262 on other engines.
- The comparison is among interpreters. go-quickjs runs with its JIT off (its
  default, built without the `quickjs_jit` tag). The rows with a JIT,
  go-quickjs-jit, goant-jit and Node with its JIT, are out, and so are the
  variants that built them. The Go engines are liustack/quickjs, go-quickjs and
  moejs. modernc.org/quickjs and goant are out of the comparison (their adapters
  and requirements are removed). The native references are C QuickJS 2026-06-04,
  Hermes V1 (release `260318099.0.0-stable`, built by `native/build.sh`, pinned by
  commit and SHA256, no JIT in that release, run as `hermesc -O` bytecode, as it
  ships) and Node `--jitless`. QuickJS-NG and the C QuickJS master snapshot are
  out.
- New bundles. `tifa/bundles` and `tifa/in` are built again from tifa main
  `ba584f71` (bench, MUI and shop, 25 pages): the old ones were from 2026-10-22.
  The bundles use tifa's Go functions only when they exist, and no engine of the
  comparison registers them, so every engine runs React's own code.
  `cmd/modulescript` and `cmd/notices` accept the helper functions tifa now
  writes after `export{};`.
- Each engine runs as it comes. It has its own Intl and URL where it has them
  (liustack-quickjs with the packages `intl`, `temporal` and `url`), and gets a
  standard polyfill where it has none: FormatJS for Intl, core-js 3.51.0
  (`tifa/polyfill/corejs-polyfill.js`) for URL, URLSearchParams, structuredClone
  and the iterator helpers, which also replaces the Iterator.concat stand-in of
  `native/node-ssr.cjs`. `tifa/run.sh` has the lists. It exits with status 1 when
  a run renders no page.
- Hermes V1 runs the pages through `native/hermes-run.sh`, `hermes-ssr.js` and
  `hermes-shim.js`. Hermes has no async generators, so the bundles are lowered for
  Hermes only: esbuild turns async generators and `for await` into ES2017 async
  functions. The Hermes shell has no `performance.now`, so a page's time is the
  wall time of the timed loop divided by the number of renders.
- `cmd/pages` makes one render table (ratio to liustack-quickjs per page group),
  the per-page times, and the list of pages whose HTML differs from ours with the
  first place they differ. `results/` holds only the files of the last run.

## 2026-11-14: newer reference engines

- The native reference rows move to C QuickJS master `e585640ffa` (it was
  `7d1c13c5d8`), QuickJS-NG v0.18.0 (it was v0.17.0) and Node v24.22.3 (it was
  v24.11.1, the same major version). C QuickJS 2026-06-04, the release this engine
  is built from, stays. `native/build.sh` pins the new archives by SHA256.
  Numbers in earlier result files are those of the old versions.
- Node v24.22.3 still has no `Iterator.concat`, so `native/node-ssr.cjs` keeps its
  stand-in.

## 2026-11-14: newer moejs and go-quickjs

- moejs is v0.1.0 (it was v0.1.0-alpha.11, released 2026-11-09) and go-quickjs is
  v0.37.3 (it was v0.34.1), both from `go get <module>@latest`. go-quickjs now
  pulls go-intl v0.9.0 (it was v0.8.0). goant (v0.2.1) and modernc.org/quickjs
  (v0.27.0) were already the newest. The adapters needed no change, and the
  feature probes give the same answers as in the run of 2026-11-06.
  Numbers in earlier result files are those of the old versions.
- The module line for liustack/quickjs reads v0.30.7. `compare/engines` reaches
  the module through its `replace`, so the run measures the checkout it is in.

## 2026-11-14: Hermes V1 on the V8 suite

- Hermes V1 has rows on the V8 suite, in a session of its own on server C (GCP
  t2d-standard-4, AMD EPYC 7B13, CPU 0, the machine held for each round),
  with liustack/quickjs v0.30.6 and C QuickJS 2026-06-04 run again in the
  same session (2026-11-14-v0.30.6-gcp-v8-hermes).
  Its scores are not compared with those of the session on the other machine.
- Rows: the release `260318099.0.0-stable` (`fd22091391c3`) from source and
  as `hermesc -O` bytecode, and `static_h` (`44f5f48c3edf`) as `-O` bytecode,
  without and with `-Xjit`. The release has no x86-64 JIT. Built Release
  with clang 22.1.8 and Intl off.
- `native/hermes-v8.js` runs the suite on the `hermes` shell with the steps of
  `qjs-v8.js`. The shell reads files only through `hermescli`, which needs
  `-Xhermes-internal-test-methods`.
- Hermes is not run on the other workloads, and the other pure Go engines
  were not run in this session.

## 2026-11-14: the V8 suite at v0.30.6, on another machine

- The machine of the earlier runs (GCP c2d-standard-8) is gone. This run is
  on a GCP t2d-standard-4 (AMD EPYC 7B13, 4 cores without SMT), pinned to
  CPU 0 with the whole machine held quiet. Scores of this machine are not
  compared with scores of the earlier one, so the session ran liustack/quickjs
  v0.30.1 again next to v0.30.6 and C QuickJS 2026-06-04
  (2026-11-14-v0.30.6-gcp-v8).
- Only the V8 suite was run, and only these three sides. The other pure Go
  engines were not run, so the tables of the run of v0.30.1 stay as they are.
- The Go sides ran `cmd/jsbench v8` of their own tree under `taskset`, not
  through `cmd/compare -phases v8`, with no memory limit. The steps of the
  suite are the same, and `jsbench`, the adapter and the files of the suite
  are the same in both trees.

## 2026-11-06: newer go-quickjs and modernc, a core pair under a lock

- go-quickjs is v0.34.1 (it was v0.28.0) and modernc.org/quickjs is v0.27.0
  (it was v0.25.0), both from `go get <module>@latest`. goant (v0.2.1) and
  moejs (v0.1.0-alpha.11) were already the newest. The adapters needed no
  change. Numbers in earlier result files are those of the old versions.
- The run of 2026-11-06 was made on the GCP machine shared with other
  sessions: timed workloads and the tifa pages ran in short pieces on CPU 0
  and its sibling 4, one piece at a time under a lock (`-add` for the
  driver, `TIFA_ONLY`, `TIFA_CPU=0` and `TIFA_SIBLING=4` for `tifa/run.sh`),
  and test262 ran on CPU 2 at low priority and was merged in. The method of
  the measurements did not change.
- go-quickjs-jit rendered the bench `table` page differently between two
  renders in the page run (`output changed between renders`), which leaves
  the page without a time. The page was run alone into
  `go-quickjs-jit-bench-rest.json`, which `cmd/pages` reads, and the run
  that passed is the one in the tables.
- C QuickJS 2026-06-04 has a row on the V8 suite: `native/qjs-v8.js` runs the
  suite on `qjs` with the steps of `runV8`. It was run in a session of its own,
  taking turns with liustack-quickjs (2026-11-06-v0.30.1-gcp-v8-c).

## 2026-11-05: rows named liustack-quickjs again

- The module is `github.com/liustack/quickjs` again, and its rows take back
  the names they had before the module was `github.com/liustack/slowjs`:
  `slowjs` is now `liustack-quickjs` and `slowjs-tifa` is `liustack-tifa`, in
  the results, the reports, `TIFA_ONLY`, `-engines` and the file names
  `tifa/run.sh` writes. Nothing is measured differently.
- The results files made while the module was `github.com/liustack/slowjs`
  (the v0.28.6 PGO page runs of 2026-10-24 and the runs of 2026-10-29) keep
  the names `slowjs` and `slowjs-tifa`. `-report` and `cmd/behind` read both
  names. Adding to one of those runs with `-add` or `-merge` would give the
  module two rows, so start a new run instead.

## 2026-10-29: the C engine, and the helper tools in Go

- The slowjs rows run the C engine (v0.29.0). The `slowjs-tifa-nogoasm` row is gone: there is one interpreter now, so it would repeat `slowjs-tifa`. Earlier results files keep it.
- The helper tools are Go programs run from `compare/engines`: `cmd/behind` (the far-behind list), `cmd/pages` (page tables and archive), `cmd/modulescript` (a tifa module bundle as a script) and `cmd/notices` (the third-party notices). They replace the Python files with the same names and write the same output for the same input. The method of the measurements did not change.

## 2026-10-28: moejs moved

- moejs is now `github.com/MoeForge/moejs`, v0.1.0-alpha.11 (it was `github.com/Calcium-Ion/moejs`, v0.1.0-alpha.9). Numbers in earlier result files are those of alpha.9. The C QuickJS rows keep their Makefile's build, which has no `-DNDEBUG` (see `docs/use/benchmarks.md`).

What changed in how the comparison measures, so that numbers from different
runs can be read side by side. A run's file in `results/` says which
version of the comparison made it (the commit of this repository).

## 2026-10-21: first version

- Engines: liustack/quickjs, modernc.org/quickjs, go-quickjs (and its JIT),
  jsc2go (with and without its JIT), goant (with and without its JIT),
  fastschema/qjs. Only engines that build with `CGO_ENABLED=0` and run
  current JavaScript: goja, sobek and otto are left out, as they lack
  async generators, WeakRef, newer regular expression flags and more
  (otto implements ES5 only).
- Engines with a JIT run twice, named for the setting: go-quickjs-interp
  and go-quickjs-jit, jsc2go-jit and jsc2go-interp, goant-interp and
  goant-jit. Both run every workload, test262 included.
- Each engine is linked into a binary of its own (`jsbench`, build tag
  `eng_<name>`), built with `-pgo=off -trimpath`.
- Workloads: React 19 `renderToString` of a 717-element page, the V8
  benchmark suite version 7, JSON.parse, JSON.stringify, string building,
  regular expressions, the QuickJS micro benchmarks in `compare/testdata`,
  cold start, memory per VM (100 VMs alive), feature probes, test262.
- The QuickJS micro benchmarks run one process per test, so that a test
  that goes over the memory cap loses only itself.
- Timed workloads: 5 rounds, engines in a different order each round,
  GOMAXPROCS=1. On Linux pinned to one CPU, and a run is repeated when its
  hyperthread sibling was busy for at least 3 clock ticks and more than 5%
  of the run, or when the engine process got less than 90% of its wall time
  as CPU time (another process ran on its CPU). Process CPU time is
  reported next to wall time.
- test262 at `d53ebc8b`: language, built-ins and intl402, without module
  tests and tests that need host hooks. Every run in a fresh VM, one
  process per engine at a time. It may run on another CPU at low priority,
  as a run of its own merged into the main run.
- Every engine process runs under a memory cap: 2 GiB for test262, 4 GiB
  for the rest. On Linux the cap is a cgroup (systemd-run), which also
  counts memory outside the Go heap. A test262 test that goes over the cap
  when run alone counts as out of memory.
- Engines with programs that run on any VM (liustack, modernc, go-quickjs,
  jsc2go, fastschema) load cold start and memory from one compiled
  program, and cold start checks that such a VM renders the same page.
  jsc2go started doing so during the 2026-10-21 run: its cold start and
  memory rows there come from a rerun with that adapter.
- fastschema runtimes are closed before their context is cancelled.
  Cancelled first, `Runtime.Close` panics and leaves memory behind. The
  test262 run of 2026-10-21 predates this, so fastschema-qjs has more
  memory restarts there than it would now.
- tifa pages (`tifa/run.sh`): the server bundles of three tifa apps,
  rendered on every engine. Pages without Intl run as they are, the shop
  pages run on pinned FormatJS polyfills (native Intl removed) and, on the
  engines with Intl of their own, on that too. C QuickJS 2026-06-04 and
  master, QuickJS-NG v0.17.0 and Node v24.11.1 (with and without its JIT)
  render the same pages as reference rows, not as competitors.

## 2026-10-21: jsc2go and fastschema/qjs dropped

- No longer compared: fastschema/qjs because it was the slowest on most workloads and its tifa renders never finished, which cost hours of machine time, and jsc2go because it is of little interest here (no Intl, no memory limit). The removed rows came from the 2026-10-21 run on v0.26.16. Their raw results are not retained here, so no numeric comparison is claimed for them.

## 2026-10-21: moejs added

- moejs (github.com/Calcium-Ion/moejs, v0.1.0-alpha.9) joins the comparison. It has no Intl, so it renders the tifa shop on the FormatJS polyfill. Its call depth is fixed at 512: a page or test that fails on depth counts as a limit of moejs, and the limit is not raised. Runtimes are made with mutable builtins, since test262, the polyfill and the tifa bundles write to them.

## 2026-10-21: go-quickjs v0.28.0

- go-quickjs moves from v0.27.0 to v0.28.0 (no API changes, by its release notes a new JIT and a faster interpreter), for the run on liustack/quickjs v0.26.17. go-intl stays at v0.6.0, the version go-quickjs requires.
- On the tifa shop, when most pages of an engine fail, each page is first rendered alone in a process of its own to find the failing ones, then the others are measured in a run without them, as before.

## 2026-10-22: archive page equality and align the runner

- Page exports now keep normalized HTML SHA256 digests and per-page equality
  verdicts, alongside raw results, rest runs, probes and tries. The two
  existing runs keep their original data and runtime equality verdicts.
- tifa paths resolve from the caller's directory. Functional checks now use
  the same capped, pinned entry and UTC environment as timing. A disturbed
  process gets at most three tries in total.
- Generated reports omit trailing whitespace and empty lines at the end.
  Bundle bytes stay unchanged because their whitespace occurs in strings.
- Bundle package versions and license texts are in `tifa/THIRD_PARTY_NOTICES.md`.
  Unsupported precise profile and historical claims were removed from the docs.

## 2026-10-24: slowjs as tifa runs it, and the MUI pages

- New row `liustack-tifa` in the tifa page runs: slowjs set up the way tifa
  sets up a VM of its pool (packages `intl` and `temporal`, no polyfill,
  `__tifa_murmur2__` and `__tifa_random_bytes__` as Go functions, a memory
  limit of 128 MiB, a stack of 10000 slots, a time budget of 3 s, tifa's GC
  rules, a shared compiled bundle). The Go is copied from `internal/ssr` of
  tifa, not imported. `liustack-quickjs` keeps its rows, with the same
  JavaScript on the polyfill as every engine. The tifa row has bench, shop and
  MUI runs on its own Intl and no polyfill run, and the C QuickJS and Node
  rows have no counterpart: they cannot take Go functions or a Go `Intl`.
  Docs say what each row measures (docs/use/benchmarks.md, "slowjs as tifa runs
  it").
- A bundle with `@emotion/hash` (MUI) gets the one-line change the tifa plugin
  makes to `murmur2`, in the adapter, since the pinned bundles were built
  before the plugin had it. Its results file counts the rewrites and the calls
  of the Go functions.
- The bundle's work after a render (young GC, full GC by the pool's rules) is
  in the CPU time per render of this row and not in its wall time, as in tifa.
- The MUI bundle is timed for the first time, on the polyfill in every engine
  and on own Intl where there is one (`mui-pf`, `mui-native`).
- `pages.py` compares the tifa row's HTML with the polyfill row's and counts
  pages that differ only in U+202F (the polyfill writes it before AM and PM,
  Go's Intl and Node write a plain space), and gives each bundle as a ratio to
  C QuickJS 2026-06-04.
- `tifa/run.sh` takes `TIFA_ONLY` (run some engines) and `pagerun.sh` takes
  `TIFA_CPU` and `TIFA_SIBLING` (another CPU pair than 1 and 5).
- `cmd/compare` builds the tifa row with the other engines and leaves it out of
  the other workloads unless `-engines` names it.
- Runs of 2026-10-24 (`results/2026-10-24-main-tifa-row-gcp-pages`) cover the
  tifa pages for five rows only. The other engines were not run again.

## 2026-10-24: today's tifa bundles, and slowjs without the Go assembly

- `tifa/run.sh` takes `TIFA_BUNDLES=<dir>`, a folder laid out as `tifa/`
  (`bundles/<app>.js`, `in/<app>/*.json`), to run bundles that tifa builds
  now in place of the pinned ones. The pinned bundles and their runs stay
  as they are.
- tifa builds its SSR bundle as an ES module since tifa 6e062d6. The
  `liustack-tifa` rows compile `bundles/<app>.mjs` as a module, as tifa
  does, and give the VM `__tifa_async_context__` as tifa does. Every other
  row runs `bundles/<app>.js`, which `tifa/module-script.py` makes from the
  module: the final `export{};` dropped and the rest in a strict function.
- New row `liustack-tifa-nogoasm`: the `liustack-tifa` row built with
  `slowjs_nogoasm`, as tifa builds an app with `build.goasm: false`, so the
  interpreter in Go on linux/amd64 rather than the one in Go assembly.
- The Node rows make the getters Node has on `globalThis` (`crypto`,
  `navigator`) plain values before the bundle runs, since the host patches
  of the new bundles assign them, and add `Iterator.concat`, which
  TanStack Router calls and Node v24.11.1 lacks.

## 2026-10-24: rows named slowjs

- The slowjs rows take the module's name: `liustack-quickjs` is now
  `slowjs`, `liustack-tifa` is `slowjs-tifa` and `liustack-tifa-nogoasm` is
  `slowjs-tifa-nogoasm`, in the results, the reports, `TIFA_ONLY`, `-engines`
  and the file names `tifa/run.sh` writes. Nothing is measured differently.
- Results files made before this keep the old names. `-report` and
  `behind.py` read both. Adding to an old run with `-add` or `-merge` would
  give slowjs two rows, so start a new run instead.
