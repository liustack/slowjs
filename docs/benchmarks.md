---
summary: 'How liustack/slowjs compares with the other JavaScript engines for Go and with three native engines, all as interpreters: the render time of tifa pages and the V8 suite score, what each engine runs with, and how to run the comparison again'
read_when:
  - Saying anything in public about how this engine compares with other Go engines, C QuickJS, Hermes or Node
  - Choosing a JavaScript engine for Go for a job
  - The owner asks for the comparison to be run again (rerun it and update this page)
  - Changing compare/engines
---

# JavaScript engines compared

Run of 2026-11-16: liustack/slowjs v0.31.2 against two other engines for Go and three native engines, on a GCP t2d-standard-4 (AMD EPYC 7B13, four cores without SMT), Linux 7.0, Go 1.27.4. Every timed process ran on one core with the whole machine held for it, GOMAXPROCS=1. The files of the run are in [`compare/engines/results/`](../compare/engines/results/) (`2026-11-16-v0.31.2-gcp-pages` and `-v8`, each as `.md` and `.json`), and [`compare/engines/README.md`](../compare/engines/README.md) says how to run it again. The numbers are from this one machine: do not compare them with numbers from another.

## What is compared

Two tables: the render time of tifa's pages and the V8 suite. The comparison never runs test262 on other engines.

- **The comparison is among interpreters.** An engine with a JIT runs with it off, or is left out. go-quickjs has a JIT that is off by default, and runs that way. The native references are C QuickJS, Hermes V1 and Node with `--jitless`.
- **Every engine runs as it comes**, the way a person would use it. It uses its own Intl and URL where it has them, and a standard polyfill where it has none: FormatJS for Intl, core-js for URL, URLSearchParams, structuredClone and the iterator helpers.
- **The pages run without tifa's Go functions.** The bundles use `__tifa_escape`, `__tifa_start_tag`, `__tifa_end_tag`, `__tifa_copy_props` and `__tifa_murmur2__` only when they exist, and no engine here registers them, ours included (checked: they are `undefined` in our VM). Every engine runs React's own code. tifa's GC rules, pool settings and shared bytecode are left out too.

| Engine | Version | How it runs |
|---|---|---|
| liustack-slowjs | v0.31.2 | the packages `intl`, `temporal` and `url` of the module registered |
| liustack-slowjs-polyfill | v0.31.2 | the same engine without those three packages, on the polyfills C QuickJS and Hermes get. In the page table only |
| go-quickjs-interp | go-quickjs v0.38.0 | JIT off, its own Intl |
| moejs | v0.1.1 | its own (partial) Intl, plus a `FinalizationRegistry` stand-in (`native/moejs-shim.js`) |
| C QuickJS | 2026-06-04 | `qjs`, built as shipped |
| Hermes V1 | `260318099.0.0-stable` | no JIT in this release, run as `hermesc -O` bytecode, as an app ships it |
| Node | v24.22.3 | `--jitless`, its own Intl and URL |

Left out: goja, sobek and otto (no async generators, WeakRef or newer regular expression flags), modernc.org/quickjs (QuickJS translated to Go, far slower) and goant (with its JIT off it scores about 300 on the V8 suite, a setting nobody runs, and it has had no change since 2026-09-21).

## Rendering

CPU time of a render of tifa's pages, as a ratio to liustack/slowjs: geometric mean over the pages of a group, median of two paired runs with the lower and higher run in brackets. Above 1 is slower than liustack/slowjs.

| Engine | bench (2 pages) | MUI (3 pages) | shop (20 pages) |
|---|---|---|---|
| liustack-slowjs | 1.00 | 1.00 | 1.00 |
| liustack-slowjs-polyfill | 0.96 (0.94–0.98) | 1.71 (1.63–1.78) | 1.54 (1.53–1.56) |
| go-quickjs-interp | 2.58 (2.40–2.77) | 3.04 (2.97–3.10) | 3.82 (3.76–3.87) |
| moejs | 1.53 (1.39–1.67) | 1.41 (1.33–1.49) | 1.73 (1.73–1.73) |
| C QuickJS 2026-06-04 | 1.93 (1.69–2.18) | 3.39 (3.11–3.67) | 2.20 (2.09–2.31) |
| Hermes V1, `-O` bytecode | 1.20 (1.02–1.37) | 1.87 (1.86–1.88) | 1.92 (1.85–2.00) |
| Node v24.22.3 `--jitless` | 0.87 (0.81–0.92) | 1.33 (1.22–1.43) | 0.87 (0.83–0.91) |

- **The second row** is liustack/slowjs without its built-in Intl, URL and Temporal, on the same polyfills as C QuickJS and Hermes. It shows how much of the lead on the MUI and shop pages comes from the interpreter and how much from Intl written in Go. On the same JavaScript, C QuickJS takes 2.0 times as long on the bench pages, 2.0 times on MUI and 1.4 times on the shop, and Hermes takes 1.25, 1.09 and 1.25 times as long. The rest of the lead on those two columns is the Go Intl.
- **Where polyfills ran.** C QuickJS, Hermes and the second row have no Intl and got FormatJS on the MUI and shop pages. URL, URLSearchParams and structuredClone came from core-js for go-quickjs, C QuickJS, Hermes and the second row, URL, URLSearchParams and the iterator helpers for moejs, and `Iterator.concat` for Hermes and Node. liustack/slowjs and Node need no polyfill for Intl and URL. The bench pages need no Intl, so that column is the same JavaScript on every engine: the claim that this engine is faster than C QuickJS on the same code rests on that column and on the V8 suite. core-js replaces only `Function.prototype.toString` and `Iterator.prototype.drop` and `take` (and `flatMap` on C QuickJS) in C QuickJS and Hermes, and a bench render calls none of them (counted over 3 renders of each page), nor `new URL`, so that column measures the engines.
- **moejs** needs one stand-in to load the bundles: they read `FinalizationRegistry` at load, which moejs v0.1.1 lacks, and `native/moejs-shim.js` gives it a registry that never calls back. With that and core-js, all 25 pages render.
- **Hermes** has no async generators, so for Hermes only esbuild lowered the bundles' async generators and `for await` to ES2017 async functions. A small file of stand-ins (`native/hermes-shim.js`) gives Hermes the builtins the bundles only read at load and it lacks (the resizable ArrayBuffer getters, the RegExp flag getters, `SharedArrayBuffer`, `Float16Array`). Its shell has no `performance.now`, so its time is the wall time of the whole timed loop divided by the number of renders.
- **HTML.** go-quickjs, moejs and Node give the same HTML as ours on all 25 pages. C QuickJS differs on 9 pages, the second row on the same 9 (its HTML is the same as C QuickJS's on all 25), and Hermes on all 25, always inside text that Intl formats or, for Hermes, in the inline script of the router. C QuickJS on FormatJS writes dates as `October 13, 14:32` where Go's Intl writes `October 13 at 14:32`, and a narrow no-break space before `AM` and `PM`. Hermes prints the router's dehydration function as `function($R){return ...}` where the others print the arrow function as written (outside Intl output), and the date text of the MUI and workbench pages differs as in C QuickJS. Every other byte is the same: the pages are listed in the `-pages.md` file with the first place each differs.

## The V8 suite

Score of each engine on the V8 suite version 7, median of 10 rounds in two sessions of 5, in which every side ran once per round and the order turned from round to round. Higher is better. In brackets: the lowest and highest round.

| Engine | Richards | DeltaBlue | Crypto | RayTrace | EarleyBoyer | RegExp | Splay | NavierStokes | Score |
|---|---|---|---|---|---|---|---|---|---|
| liustack-slowjs | 1080 | 950 | 1284 | 2288 | 2216 | 905 | 3014 | 2118 | 1546 (1459–1721) |
| go-quickjs-interp | 926 | 774 | 1149 | 1230 | 1396 | 1836 | 2125 | 1938 | 1303 (1237–1518) |
| moejs | 1182 | 888 | 1265 | 1298 | 1682 | 908 | 2264 | 2755 | 1405 (1298–1691) |
| C QuickJS 2026-06-04 | 804 | 698 | 890 | 1551 | 1750 | 347 | 2236 | 1753 | 1056 (1018–1126) |
| Hermes V1, `-O` bytecode | 1267 | 1377 | 1685 | 3937 | 3496 | 446 | 1252 | 3342 | 1696 (1354–1960) |
| Node v24.22.3 `--jitless` | 1238 | 1364 | 1025 | 2682 | 4750 | 2509 | 2328 | 1338 | 1875 (1248–2055) |

The ratios below are medians of the ratio of two engines' scores within the same round. liustack/slowjs scores 1.49 times C QuickJS and is ahead of it on every benchmark. Among the engines for Go it scores 1.18 times go-quickjs and 1.11 times moejs, though in some rounds moejs and it cannot be told apart (the ratio runs from 0.96 to 1.17). It is not the fastest engine on this suite: Hermes scores 1.07 times as much and Node `--jitless` 1.15. Against moejs it leads on RayTrace, EarleyBoyer and Splay, is about level on Crypto, DeltaBlue and RegExp, and trails on NavierStokes (moejs 1.3 times) and Richards (1.1). go-quickjs leads on RegExp only (2.0 times). liustack/slowjs leads on Splay (1.2 to 2.4 times every other row).

## The pages

The bundles are the server bundles of three tifa apps built from tifa main `ba584f71` (Node 24.13.0, pnpm 10.33.0, Go 1.27.2): the bench app (`test/apps/bench`, 2 pages), the MUI app (`test/apps/mui`, 3 pages) and the example shop (`examples/shop`, 20 pages: 6 of the workbench and 14 of the store in en-US and zh-CN). They are in `compare/engines/tifa/bundles`, with these SHA256:

```
8df28d3d550cfeb198f00e7d165b6a4e8aa6f53cc3049a53006785cea14beebe  bench.js
5d3cbbb1d6b24d42b3b7bfd3f8e6df757d843fb9b4a51588fdb3159de9733770  mui.js
b0d5bc4bf6c795e6fe50de3beb10b4a55c7ece542c65dd01c71dcba3a8f6e439  shop.js
```

Each page is rendered as tifa renders it: `startRender(json)` with the input each app's server sends for that page (`tifa/in`), the promise jobs, then the result. A page's time is the CPU time per render (the mean of 200 timed renders after 50 warm ones for the bench pages, 100 after 20 for MUI and the shop); C QuickJS and Node time the renders in the script by wall time. Every engine's HTML is written out and compared with ours after the router's render timestamps are masked.

## Run it again

See [`compare/engines/README.md`](../compare/engines/README.md): the commands, the machine it needs, how the bundles are made again and how it measures. Before a release that runs it again is tagged, follow "The comparison with the other engines" in [releasing.md](releasing.md#the-comparison-with-the-other-engines).


test262 is run on liustack/slowjs only, by the repository's own check ([testing.md](testing.md#test262)): 83426 tests run and 33 known errors, the same on every run.
