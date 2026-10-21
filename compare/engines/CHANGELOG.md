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
