# Changes to the engine comparison

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
  Docs say what each row measures (docs/benchmarks.md, "slowjs as tifa runs
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
