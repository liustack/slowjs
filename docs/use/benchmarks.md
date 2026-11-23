---
summary: 'How liustack/beastjs compares with the other JavaScript engines for Go and with three native engines, all as interpreters: the render time of tifa pages and the V8 suite score, what each engine runs with, and how to run the comparison again'
audience: user
order: 4
read_when:
  - Saying anything in public about how this engine compares with other Go engines, C QuickJS, Hermes or Node
  - Choosing a JavaScript engine for Go for a job
  - The owner asks for the comparison to be run again (rerun it and update this page)
  - Changing compare/engines
---

# JavaScript engines compared

English · [简体中文](benchmarks.zh-CN.md)

Run of 2026-11-24: liustack/beastjs at v0.32.0 (the engine of v0.33.0, under the module path of then) against two other engines for Go and three native engines, on a GCP t2d-standard-4 (AMD EPYC 7B13, four cores without SMT), Linux 7.0, Go 1.27.4. Every timed process ran on one core with the whole machine held for it, GOMAXPROCS=1. The files of the run are in [`compare/engines/results/`](../../compare/engines/results/) (`2026-11-24-v0.32.0-gcp-pages` and `-v8`, each as `.md` and `.json`), and [`compare/engines/README.md`](../../compare/engines/README.md) says how to run it again. The numbers are from this one machine: do not compare them with numbers from another.

## What is compared

Two tables: the render time of tifa's pages and the V8 suite. The comparison never runs test262 on other engines.

- **The comparison is among interpreters.** An engine with a JIT runs with it off, or is left out. go-quickjs has a JIT that is off by default, and runs that way. The native references are C QuickJS, Hermes V1 and Node with `--jitless`.
- **Every engine runs as it comes**, the way a person would use it. It uses its own Intl and URL where it has them, and a standard polyfill where it has none: FormatJS for Intl, core-js for URL, URLSearchParams, structuredClone and the iterator helpers.
- **The pages run without tifa's Go functions.** The bundles use `__tifa_escape`, `__tifa_start_tag`, `__tifa_end_tag`, `__tifa_copy_props` and `__tifa_murmur2__` only when they exist, and no engine here registers them, ours included (checked: they are `undefined` in our VM). Every engine runs React's own code. tifa's GC rules, pool settings and shared bytecode are left out too.

| Engine | Version | How it runs |
|---|---|---|
| liustack-beastjs | v0.32.0 | the packages `intl`, `temporal` and `url` of the module registered |
| liustack-beastjs-polyfill | v0.32.0 | the same engine without those three packages, on the polyfills C QuickJS and Hermes get. In the page table only |
| go-quickjs-interp | go-quickjs v0.43.0 | JIT off, its own Intl |
| moejs | v0.1.1 | its own (partial) Intl, plus a `FinalizationRegistry` stand-in (`native/moejs-shim.js`) |
| C QuickJS | 2026-06-04 | `qjs`, built as shipped |
| Hermes V1 | `260318099.0.8` | no JIT in this release, run as `hermesc -O` bytecode, as an app ships it |
| Node | v26.13.1 | `--jitless`, its own Intl and URL |

Left out: goja, sobek and otto (no async generators, WeakRef or newer regular expression flags), modernc.org/quickjs (QuickJS translated to Go, far slower) and goant (with its JIT off it scores about 300 on the V8 suite, a setting nobody runs, and it has had no change since 2026-09-21).

## Rendering

CPU time of a render of tifa's pages, as a ratio to liustack/beastjs: geometric mean over the pages of a group, median of two paired runs with the lower and higher run in brackets. Above 1 is slower than liustack/beastjs.

| Engine | bench (2 pages) | MUI (3 pages) | shop (20 pages) |
|---|---|---|---|
| liustack-beastjs | 1.00 | 1.00 | 1.00 |
| liustack-beastjs-polyfill | 1.03 (1.01–1.04) | 1.71 (1.71–1.72) | 1.62 (1.61–1.63) |
| go-quickjs-interp | 2.31 (2.25–2.38) | 2.12 (2.09–2.16) | 3.11 (3.09–3.13) |
| moejs | 1.52 (1.47–1.56) | 1.52 (1.49–1.54) | 1.99 (1.94–2.03) |
| C QuickJS 2026-06-04 | 2.06 (2.05–2.07) | 3.63 (3.57–3.69) | 2.59 (2.57–2.61) |
| Hermes V1, `-O` bytecode | 1.08 (1.07–1.09) | 1.90 (1.86–1.94) | 1.86 (1.84–1.88) |
| Node v26.13.1 `--jitless` | 0.99 (0.99–0.99) | 1.11 (1.10–1.12) | 0.90 (0.89–0.90) |

- **The second row** is liustack/beastjs without its built-in Intl, URL and Temporal, on the same polyfills as C QuickJS and Hermes. It shows how much of the lead on the MUI and shop pages comes from the interpreter and how much from Intl written in Go. On the same JavaScript, C QuickJS takes 2.0 times as long on the bench pages, 2.1 times on MUI and 1.6 times on the shop, and Hermes takes 1.05, 1.11 and 1.15 times as long. The rest of the lead on those two columns is the Go Intl.
- **Where polyfills ran.** C QuickJS, Hermes and the second row have no Intl and got FormatJS on the MUI and shop pages. URL, URLSearchParams and structuredClone came from core-js for go-quickjs, C QuickJS, Hermes and the second row, URL, URLSearchParams and the iterator helpers for moejs, and `Iterator.concat` for Hermes and Node. liustack/beastjs and Node need no polyfill for Intl and URL. The bench pages need no Intl, so that column is the same JavaScript on every engine: the claim that this engine is faster than C QuickJS on the same code rests on that column and on the V8 suite. core-js replaces only `Function.prototype.toString` and `Iterator.prototype.drop` and `take` (and `flatMap` on C QuickJS) in C QuickJS and Hermes, and a bench render calls none of them (counted over 3 renders of each page), nor `new URL`, so that column measures the engines.
- **moejs** needs one stand-in to load the bundles: they read `FinalizationRegistry` at load, which moejs v0.1.1 lacks, and `native/moejs-shim.js` gives it a registry that never calls back. With that and core-js, all 25 pages render.
- **Hermes** has no async generators, so for Hermes only esbuild lowered the bundles' async generators and `for await` to ES2017 async functions. A small file of stand-ins (`native/hermes-shim.js`) gives Hermes the builtins the bundles only read at load and it lacks (the resizable ArrayBuffer getters, the RegExp flag getters, `SharedArrayBuffer`, `Float16Array`). Its shell has no `performance.now`, so its time is the wall time of the whole timed loop divided by the number of renders.
- **HTML.** go-quickjs, moejs and Node give the same HTML as ours on all 25 pages. C QuickJS differs on 9 pages, the second row on the same 9 (its HTML is the same as C QuickJS's on all 25), and Hermes on all 25, always inside text that Intl formats or, for Hermes, in the inline script of the router. C QuickJS on FormatJS writes dates as `October 13, 14:32` where Go's Intl writes `October 13 at 14:32`, and a narrow no-break space before `AM` and `PM`. Hermes prints the router's dehydration function as `function($R){return ...}` where the others print the arrow function as written (outside Intl output), and the date text of the MUI and workbench pages differs as in C QuickJS. Every other byte is the same: the pages are listed in the `-pages.md` file with the first place each differs.

## The V8 suite

Score of each engine on the V8 suite version 7, median of 10 rounds, in each of which every side ran once and the order turned from round to round. Higher is better. In brackets: the lowest and highest round.

| Engine | Richards | DeltaBlue | Crypto | RayTrace | EarleyBoyer | RegExp | Splay | NavierStokes | Score |
|---|---|---|---|---|---|---|---|---|---|
| liustack-beastjs | 1370 | 1146 | 1489 | 2626 | 3466 | 1845 | 4236 | 2948 | 2160 (2122–2201) |
| go-quickjs-interp | 1295 | 1240 | 1879 | 2457 | 2596 | 2684 | 3678 | 4210 | 2308 (2219–2357) |
| moejs | 1239 | 1078 | 1342 | 1903 | 2430 | 1360 | 3184 | 3201 | 1816 (1767–1877) |
| C QuickJS 2026-06-04 | 836 | 748 | 932 | 1593 | 1994 | 378 | 2822 | 1813 | 1172 (1146–1191) |
| Hermes V1, `-O` bytecode | 1315 | 1512 | 1748 | 4595 | 4770 | 549 | 2478 | 3542 | 2096 (2053–2132) |
| Node v26.13.1 `--jitless` | 1390 | 1496 | 1030 | 3518 | 5532 | 2933 | 3367 | 1558 | 2240 (2181–2279) |

The ratios below are medians of the ratio of two engines' scores within the same round. liustack/beastjs scores 1.85 times C QuickJS and is ahead of it on every benchmark. Among the engines for Go it scores 1.20 times moejs and is ahead of it in every round (the ratio runs from 1.16 to 1.22), but go-quickjs scores 1.07 times as much as it does, and is ahead in every round (1.04 to 1.07). It is not the fastest engine on this suite: go-quickjs and Node `--jitless` (1.03 times as much) score higher, and it scores 1.03 times Hermes (1.02 to 1.04). Against go-quickjs it leads on EarleyBoyer (1.4 times), Splay (1.1 times), Richards and RayTrace (both 1.05 times), and trails on NavierStokes (go-quickjs 1.4 times), RegExp (1.45 times), Crypto (1.3 times) and DeltaBlue (1.1 times). Against moejs it leads on every benchmark but NavierStokes (moejs 1.07 times). liustack/beastjs leads on Splay (1.1 to 1.7 times every other row).

## The pages

The bundles are the server bundles of three tifa apps built from tifa `63a2099e` (v0.0.10: Node 24.13.0, pnpm 10.33.0, Go 1.27.4): the bench app (`test/apps/bench`, 2 pages), the MUI app (`test/apps/mui`, 3 pages) and the example shop (`examples/shop`, 20 pages: 6 of the workbench and 14 of the store in en-US and zh-CN). They are in `compare/engines/tifa/bundles`, with these SHA256:

```
be4fe9d65453a8628a09b2edc7d712fc5684d86e59722377523ecb137576f517  bench.js
881a59600aca99c1f5ac00775cf150a807d054bc893fff419461e7be85f5a586  mui.js
656886554a87dae25a2ced650fe8fbf47b83b8eefaecb0d8ff1a8c327576c3a9  shop.js
```

Each page is rendered as tifa renders it: `startRender(json)` with the input each app's server sends for that page (`tifa/in`), the promise jobs, then the result. A page's time is the CPU time per render (the mean of 200 timed renders after 50 warm ones for the bench pages, 100 after 20 for MUI and the shop); C QuickJS and Node time the renders in the script by wall time. Every engine's HTML is written out and compared with ours after the router's render timestamps are masked.

## Run it again

See [`compare/engines/README.md`](../../compare/engines/README.md): the commands, the machine it needs, how the bundles are made again and how it measures. Before a release that runs it again is tagged, follow "The comparison with the other engines" in [releasing.md](../internals/releasing.md#the-comparison-with-the-other-engines).


test262 is run on liustack/beastjs only, by the repository's own check ([testing.md](../internals/testing.md#test262)): 83426 tests run and 33 known errors, the same on every run.
