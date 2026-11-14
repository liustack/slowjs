---
summary: 'How liustack/quickjs compares with the other JavaScript engines for Go and with three native engines, all as interpreters: the render time of tifa pages and the V8 suite score, what each engine runs with, and how to run the comparison again'
read_when:
  - Saying anything in public about how this engine compares with other Go engines, C QuickJS, Hermes or Node
  - Choosing a JavaScript engine for Go for a job
  - Making a minor release, or a patch release the owner names (rerun the comparison and update this page)
  - Changing compare/engines
---

# JavaScript engines compared

English · [简体中文](benchmarks.zh-CN.md)

Run of 2026-11-14: liustack/quickjs v0.30.7 against two other engines for Go and three native engines, on a GCP t2d-standard-4 (AMD EPYC 7B13, four cores without SMT), Linux 7.0, Go 1.27.4. Every timed process ran on one core with the whole machine held for it, GOMAXPROCS=1. The files of the run are in [`compare/engines/results/`](../compare/engines/results/) (`2026-11-14-v0.30.7-gcp-pages` and `-v8`, each as `.md` and `.json`), and [`compare/engines/README.md`](../compare/engines/README.md) says how to run it again. The numbers are from this one machine: do not compare them with numbers from another. The run was made as v0.30.7, under the module path `github.com/liustack/quickjs`.

## What is compared

Two tables: the render time of tifa's pages and the V8 suite. The comparison never runs test262 on other engines.

- **The comparison is among interpreters.** An engine with a JIT runs with it off, or is left out. go-quickjs has a JIT that is off by default, and runs that way. The native references are C QuickJS, Hermes V1 and Node with `--jitless`.
- **Every engine runs as it comes**, the way a person would use it. It uses its own Intl and URL where it has them, and a standard polyfill where it has none: FormatJS for Intl, core-js for URL, URLSearchParams, structuredClone and the iterator helpers.
- **The pages run without tifa's Go functions.** The bundles use `__tifa_escape`, `__tifa_start_tag`, `__tifa_end_tag`, `__tifa_copy_props` and `__tifa_murmur2__` only when they exist, and no engine here registers them, ours included (checked: they are `undefined` in our VM). Every engine runs React's own code. tifa's GC rules, pool settings and shared bytecode are left out too.

| Engine | Version | How it runs |
|---|---|---|
| liustack-quickjs | v0.30.7 | the packages `intl`, `temporal` and `url` of the module registered |
| go-quickjs-interp | go-quickjs v0.37.3 | JIT off, its own Intl |
| moejs | v0.1.0 | its own (partial) Intl |
| C QuickJS | 2026-06-04 | `qjs`, built as shipped |
| Hermes V1 | `260318099.0.0-stable` | no JIT in this release, run as `hermesc -O` bytecode, as an app ships it |
| Node | v24.22.3 | `--jitless`, its own Intl and URL |

Left out: goja, sobek and otto (no async generators, WeakRef or newer regular expression flags), modernc.org/quickjs (QuickJS translated to Go, far slower) and goant (with its JIT off it scores about 300 on the V8 suite, a setting nobody runs, and it has had no change since 2026-09-21).

## Rendering

CPU time of a render of tifa's pages, as a ratio to liustack/quickjs: geometric mean over the pages of a group, range in brackets. Above 1 is slower than liustack/quickjs.

| Engine | bench (2 pages) | MUI (3 pages) | shop (20 pages) |
|---|---|---|---|
| liustack-quickjs | 1.00 | 1.00 | 1.00 |
| go-quickjs-interp | 2.35 (2.33–2.36) | 2.39 (2.32–2.51) | 3.32 (2.26–5.29) |
| moejs | does not render | does not render | does not render |
| C QuickJS 2026-06-04 | 1.76 (1.63–1.90) | 3.53 (2.18–4.97) | 2.26 (1.45–2.95) |
| Hermes V1, `-O` bytecode | 0.91 (0.86–0.97) | 1.83 (1.18–2.50) | 1.66 (0.79–4.15) |
| Node v24.22.3 `--jitless` | 0.85 (0.83–0.87) | 1.09 (1.03–1.11) | 0.86 (0.64–0.96) |

- **Where polyfills ran.** C QuickJS and Hermes have no Intl and got FormatJS on the MUI and shop pages. URL, URLSearchParams and structuredClone came from core-js for go-quickjs, C QuickJS and Hermes, URL, URLSearchParams and the iterator helpers for moejs, and `Iterator.concat` for Hermes and Node. liustack/quickjs and Node need no polyfill for Intl and URL. The bench pages need no Intl, so that column is the same JavaScript on every engine: the claim that this engine is faster than C QuickJS on the same code rests on that column and on the V8 suite.
- **moejs does not render any page.** The bundles read `FinalizationRegistry` at load, which moejs v0.1.0 does not have, and no standard polyfill exists for it.
- **Hermes** has no async generators, so for Hermes only esbuild lowered the bundles' async generators and `for await` to ES2017 async functions. Its shell has no `performance.now`, so its time is the wall time of the whole timed loop divided by the number of renders.
- **HTML.** go-quickjs and Node give the same HTML as ours on all 25 pages. C QuickJS differs on 9 pages, and Hermes on all 25, always inside text that Intl formats or, for Hermes, in the inline script of the router. C QuickJS on FormatJS writes dates as `October 13, 14:32` where Go's Intl writes `October 13 at 14:32`, and a narrow no-break space before `AM` and `PM`. Hermes prints the router's dehydration function as `function($R){return ...}` where the others print the arrow function as written (outside Intl output), and the date text of the MUI and workbench pages differs as in C QuickJS. Every other byte is the same: the pages are listed in the `-pages.md` file with the first place each differs.

## The V8 suite

Score of each engine on the V8 suite version 7, median of 5 rounds in one session, in which every side ran once per round and the order turned from round to round. Higher is better. In brackets: the lowest and highest round.

| Engine | Richards | DeltaBlue | Crypto | RayTrace | EarleyBoyer | RegExp | Splay | NavierStokes | Score |
|---|---|---|---|---|---|---|---|---|---|
| liustack-quickjs | 1152 | 1011 | 1062 | 1553 | 2332 | 557 | 4238 | 2360 | 1493 (1440–1536) |
| go-quickjs-interp | 989 | 864 | 1263 | 1543 | 1729 | 2947 | 2567 | 2335 | 1630 (1538–1712) |
| moejs | 1261 | 1071 | 1319 | 1890 | 2519 | 1504 | 3151 | 3171 | 1852 (1741–1878) |
| C QuickJS 2026-06-04 | 821 | 731 | 934 | 1561 | 2017 | 379 | 2809 | 1812 | 1161 (1132–1208) |
| Hermes V1, `-O` bytecode | 1297 | 1513 | 1734 | 4520 | 4757 | 548 | 2537 | 3446 | 2080 (2022–2179) |

liustack/quickjs scores 1.29 times C QuickJS. It is ahead of it on every benchmark but RayTrace (0.99, a tie within the rounds). It is not the fastest engine on this suite: moejs scores 1.24 times as much, go-quickjs 1.09 and Hermes 1.39. The gaps are in RegExp (go-quickjs 5.3 times, moejs 2.7), Crypto, RayTrace and NavierStokes. liustack/quickjs leads on Splay (1.3 to 1.7 times every other row).

## The pages

The bundles are the server bundles of three tifa apps built from tifa main `ba584f71` (Node 24.13.0, pnpm 10.33.0, Go 1.27.2): the bench app (`test/apps/bench`, 2 pages), the MUI app (`test/apps/mui`, 3 pages) and the example shop (`examples/shop`, 20 pages: 6 of the workbench and 14 of the store in en-US and zh-CN). They are in `compare/engines/tifa/bundles`, with these SHA256:

```
8df28d3d550cfeb198f00e7d165b6a4e8aa6f53cc3049a53006785cea14beebe  bench.js
5d3cbbb1d6b24d42b3b7bfd3f8e6df757d843fb9b4a51588fdb3159de9733770  mui.js
b0d5bc4bf6c795e6fe50de3beb10b4a55c7ece542c65dd01c71dcba3a8f6e439  shop.js
```

Each page is rendered as tifa renders it: `startRender(json)` with the input each app's server sends for that page (`tifa/in`), the promise jobs, then the result. A page's time is the CPU time per render (the mean of 200 timed renders after 50 warm ones for the bench pages, 100 after 20 for MUI and the shop); C QuickJS and Node time the renders in the script by wall time. Every engine's HTML is written out and compared with ours after the router's render timestamps are masked.

## Run it again

See [`compare/engines/README.md`](../compare/engines/README.md): the commands, the machine it needs, how the bundles are made again and how it measures. At each release, follow "Compare with the other engines again" in [releasing.md](releasing.md#compare-with-the-other-engines-again).

test262 is run on liustack/quickjs only, by the repository's own check ([testing.md](testing.md#test262)): 83426 tests run and 33 known errors, the same on every run.
