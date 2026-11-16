<!-- Copyright (c) 2026 The liustack/slowjs Authors. -->
<!-- SPDX-License-Identifier: MIT -->

# compare/engines

The comparison of liustack/slowjs with other JavaScript engines for Go and with three native engines. A Go module of its own, so the other engines never enter the `go.mod` of the root. The results and what they say are in [docs/benchmarks.md](../../docs/benchmarks.md).

## What is compared

Two tables:

1. **Rendering.** CPU time of a render of tifa's pages: the bench app (2 pages), the MUI app (3) and the shop (20), as a ratio to liustack/slowjs.
2. **The V8 suite.** The score of each engine, and of each of its 8 benchmarks.

The comparison never runs test262 on other engines.

The V8 suite has a row for Node `--jitless` as well. `native/node-v8.cjs` takes the steps of `qjs-v8.js` and of `jsbench v8`: the same nine files of `internal/arewefastyet/v8-v7` run as global scripts in one context, then `BenchmarkSuite.RunSuites`, so the number of runs, the time rule (`base.js` times with `new Date()`) and the score are the same for every engine, and it prints the same JSON. What cannot be the same: Node runs V8's interpreter in a process with a garbage collector and helper threads of V8's own, which the other engines do not have. It is held to one core like the others.

The page table shows liustack/slowjs twice, because the Go engines differ in what is built in. **liustack-slowjs** runs as it ships: its own Intl, Temporal, URL and URLSearchParams, from the Go packages `intl/`, `temporal/` and `url/`. **liustack-slowjs-polyfill** is the same engine with none of those packages linked or registered, and with the polyfills an engine without the features gets (FormatJS for Intl, core-js for URL, URLSearchParams, structuredClone and the iterator helpers). On pages that format dates and numbers, Hermes and C QuickJS run FormatJS in JavaScript while liustack-slowjs runs Intl written in Go, so the first row mixes the speed of the interpreter with the Intl being built in. The second row has the interpreter alone. Ratios stay a multiple of liustack-slowjs, the first row. The second row is for the pages only: `cmd/compare` builds its binary but leaves it out of the V8 suite, where it would be the same engine twice, unless `-engines liustack-slowjs-polyfill` names it.

The rule of the engine set: **the comparison is among interpreters.** An engine with a JIT runs with it off, or is left out. The Go engines are liustack/slowjs (two rows, see above)
, go-quickjs (its JIT is off by default) and moejs. The native references are C QuickJS 2026-06-04, Hermes V1 (the release, which has no x86-64 JIT, as it ships: bytecode compiled with `hermesc -O`) and Node with `--jitless`.

Each engine runs the way a person would use it. It has its own Intl and URL where it has them, and a standard polyfill where it has none: FormatJS for Intl, core-js for URL, URLSearchParams, structuredClone and the iterator helpers. A polyfill is loaded only into an engine that lacks the feature (`tifa/run.sh` has the list). The comparison's bundles run without tifa's Go functions (`__tifa_escape`, `__tifa_start_tag`, `__tifa_end_tag`, `__tifa_copy_props`, `__tifa_murmur2__`, `__tifa_random_bytes__`), tifa's GC rules and pool settings, and shared bytecode: no engine registers them, ours included, so every engine runs React's own code.

## Layout

| Path | What |
| --- | --- |
| `cmd/compare` | The driver: builds one `jsbench` per engine and runs the V8 suite (`-phases v8`, the default) |
| `cmd/jsbench` | The program linked with one engine: `v8`, `tifa-ssr` and the older workloads |
| `engine/` | One adapter per engine, behind build tags (`eng_<name>`). liustack-slowjs and liustack-slowjs-polyfill share `eng_liustack.go` and differ in `eng_liustack_own.go` and `eng_liustack_bare.go` |
| `tifa/` | The three server bundles (`bundles/`), the 25 page inputs (`in/`), the polyfills (`polyfill/`, `prelude/`), `run.sh` and `pagerun.sh` |
| `native/` | `build.sh` (C QuickJS, Hermes, Node), the drivers of the native engines (`ssr-driver.js`, `node-ssr.cjs`, `node-v8.cjs`, `hermes-ssr.js`, `hermes-run.sh`, `qjs-v8.js`, `hermes-v8.js`) |
| `cmd/pages` | The page tables and the archive of a run of `tifa/run.sh` |
| `cmd/modulescript`, `cmd/notices` | Turns tifa's module bundle into a script. Makes `tifa/THIRD_PARTY_NOTICES.md` |
| `results/` | The files of the last run |

## Run it again

On a Linux machine with four cores that nothing else uses, with Go, clang, and Node and npm (the Node of `native/build.sh` will do). The run measures the checkout it is in, so check out the commit to measure first: for a release, the commit of its branch that holds the final generated engine, before the tag ([releasing.md](../../docs/releasing.md#the-comparison-with-the-other-engines)).

```sh
cd compare/engines
sh native/build.sh $HOME/jsnative                 # C QuickJS, Hermes, Node
export PATH=$HOME/jsnative/node-v26.13.1-linux-x64/bin:$PATH
tifa/polyfill/build.sh                            # the two polyfill files
go build -pgo=off -trimpath -o bin/compare ./cmd/compare
./bin/compare -build-only -bindir bin             # one jsbench per engine
```

The tifa pages, one engine per call, each call on its own core held by `~/bin/quiet`-style locking (here core 0), then the tables:

```sh
run=<date>-<version>-<host>
page_dir=$HOME/pages-$run
for e in liustack-slowjs liustack-slowjs-polyfill go-quickjs-interp quickjs-2026-06-04 node-jitless hermes moejs; do
  TIFA_ONLY=$e TIFA_CPU=0 TIFA_SIBLING=3 tifa/run.sh $page_dir bin $HOME/jsnative
done
go run ./cmd/pages $page_dir --output results/$run-pages
```

The V8 suite, five rounds, each side once per round in a turning order. The Go sides run `bin/<name> v8 -dir ../../internal/arewefastyet/v8-v7` under `GOMAXPROCS=1 taskset -c 0`, C QuickJS runs `qjs --std native/qjs-v8.js <dir>`, Node runs `node --jitless native/node-v8.cjs <dir>` (the Node of `native/build.sh`), Hermes runs `hermes -Xhermes-internal-test-methods native/hermes-v8.js -- hbc <dir>`, the files compiled ahead by `hermesc -O`.

A run is cut into pieces on a shared machine: `-name` makes a run and `-add` adds to it, `TIFA_ONLY` picks the engines of the page runs.

## Make the bundles again

`tifa/bundles/*.js` and `tifa/in/*/*.json` come from a throwaway clone of tifa (never build in a working copy of tifa): the commit, Node, pnpm and Go versions, and the SHA256 of each bundle are in [docs/benchmarks.md](../../docs/benchmarks.md#the-pages). In the clone, `go tool tifa build` in `test/apps/bench`, `test/apps/mui` and `examples/shop` writes each app's `.tifa/dist/ssr.js`. `go run ./cmd/modulescript <ssr.js> tifa/bundles/<app>.js` turns it into a script. The inputs are what each app's server sends to `startRender` for a page: the recipe of [engine/pgo/README.md](../../engine/pgo/README.md) records them (a file `zz_dump.go` in `internal/ssr`, `TIFA_DUMP_INPUT` set, one request per page, `SHOP_OPEN=1` for the shop's workbench). `go run ./cmd/notices <tifa clone> tifa` writes the third-party notices again, and `shasum -a 256` the `SHA256SUMS`.

## How it measures

- Each engine is linked into a binary of its own (`jsbench`, build tag `eng_<name>`), built with `CGO_ENABLED=0 -pgo=off -trimpath`, so no engine shares a process, a profile or a garbage collector with another.
- Every timed process runs on one core (`taskset -c`), with GOMAXPROCS=1 and TZ=UTC, under a 4 GiB memory cap, with the machine held for it. `pagerun.sh` runs a page set again, up to three tries, when the process got less than 90% of the run as CPU time.
- A page's time is CPU time per render (the mean of 200 timed renders after 50 warm ones for the bench pages, 100 after 20 for MUI and the shop) for the Go engines. C QuickJS and Node time the renders in the script by wall time on a core of their own. The Hermes shell has no clock finer than a millisecond, so its time is the wall time of the whole timed loop divided by the number of renders.
- The HTML of every page is written out and compared with ours after the router's render timestamps (`u:` and 13 digits) are masked. `cmd/pages` lists each page that differs with the first place it differs, so that a difference inside Intl output can be told from one outside it.
