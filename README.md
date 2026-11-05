<h1 align="center"><img src="assets/brand/quickjs-banner.png" alt="liustack/quickjs"></h1>

<p align="center"><b>The fastest pure Go JavaScript engine for rendering React.</b></p>

<p align="center">
  <a href="./README.zh-CN.md">简体中文</a> ·
  <a href="#how-fast">How fast</a> ·
  <a href="docs/api.md">API</a> ·
  <a href="docs/benchmarks.md">Benchmarks</a> ·
  <a href="CHANGELOG.md">Changelog</a>
</p>

<p align="center">
  <a href="https://x.com/liustack"><img src="https://img.shields.io/badge/follow-%40liustack-black?style=flat-square&logo=x&logoColor=white" alt="Follow @liustack on X"></a>
  <a href="https://pkg.go.dev/github.com/liustack/quickjs"><img src="https://img.shields.io/badge/go.dev-reference-007d9c?style=flat-square&logo=go&logoColor=white" alt="Go reference"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/liustack/quickjs?style=flat-square" alt="Go version"></a>
  <img src="https://img.shields.io/badge/CGO-off-success?style=flat-square" alt="CGO off">
  <a href="LICENSE-MIT"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
  <img src="https://img.shields.io/badge/Not%20backed%20by-Y%20Combinator-FF6600?style=flat-square&logo=ycombinator&logoColor=white" alt="Not backed by Y Combinator">
  <img src="https://img.shields.io/badge/users-unknown-lightgrey?style=flat-square" alt="Users unknown">
</p>

Your Go server renders React pages itself, faster than C QuickJS on the same code, in one static binary with no cgo and no Node process beside it.

It is for Go programs that need to run JavaScript, such as rendering React on the server. The engine runs ES2023 with modules, async generators, proxies, BigInt and `Intl` in every Chromium locale. It is QuickJS compiled to Go assembly and tuned for server rendering, built for [tifa](https://github.com/liustack/tifa), a Go framework for React pages.

It is an engine, not Node: there is no `require`, `fs` or `process`. Code from npm runs unchanged once it is bundled as for a browser, as React is.

## How fast

CPU time of one render of the server bundles of tifa's apps, with the same JavaScript on one core, as a multiple of liustack/quickjs (lower is faster):

| Engine | Pure Go | React bench pages | MUI pages | Shop pages |
| :-- | :-: | --: | --: | --: |
| **liustack/quickjs** | yes | **1.00** | **1.00** | **1.00** |
| C QuickJS 2026-06-04 | no, C | 1.32 | 1.30 | 1.04 |
| go-quickjs | yes | 1.87 | 1.39 | 1.73 |
| go-quickjs with its JIT | yes | 2.03 | 1.58 | 2.85 |
| moejs | yes | 2.29 | 1.51 | does not load |
| modernc.org/quickjs | yes | 3.97 | 4.00 | 2.56 |
| goant with its JIT | yes | 5.04 | 3.97 | 4.03 |

liustack/quickjs v0.30.1 on a GCP c2d-standard-8 (AMD EPYC 7B13), Go 1.27.2. The MUI and shop pages need `Intl`, which C QuickJS and most of the others do not have, so every engine runs them on the FormatJS polyfill. With its own `Intl`, as tifa runs it, liustack/quickjs takes 0.75, 0.34 and 0.72 of the time of C QuickJS. Node 24 with its JIT takes 0.21 to 0.23 of the time of liustack/quickjs, and 0.60 to 0.69 with `--jitless`.

It is not the fastest engine on everything. On the V8 benchmark suite (higher is better):

| Engine | Pure Go | V8 suite score |
| :-- | :-: | --: |
| go-quickjs with its JIT | yes | 5157 |
| go-quickjs | yes | 1785 |
| C QuickJS 2026-06-04 | no, C | 1315 |
| **liustack/quickjs** | yes | **1268** |
| goant with its JIT | yes | 1130 |
| moejs | yes | 811 |
| modernc.org/quickjs | yes | 431 |

C QuickJS was run in a session of its own on the same machine, taking turns with liustack/quickjs, which scored 1283 there: C scores 1.025 times as high, ahead on RayTrace, Crypto, EarleyBoyer and NavierStokes and behind on RegExp, Splay, Richards and DeltaBlue.

[docs/benchmarks.md](docs/benchmarks.md) has every table, the test262 results of each engine, the workloads where liustack/quickjs is slowest, and how to run it all again.

## Install

```sh
go get github.com/liustack/quickjs
```

Go 1.26.6 or later, up to Go 1.27. No cgo and no C compiler: it builds with `CGO_ENABLED=0` and cross-compiles like any Go code. The Go assembly of the engine was checked against every patch release from Go 1.26.6 to 1.27.3, and a new patch release is checked every week. It leans on how Go starts a function and handles a fault, so a newer Go stops the build until liustack/quickjs has been checked with it and moves its limit in a patch release. `-tags quickjs_anygo` builds anyway (see [Moving to a newer Go](docs/upgrading.md#moving-to-a-newer-go)).

## Platforms

linux and darwin, on amd64 and arm64, and windows/amd64. A build for any other target stops with an error that names them.

## A first VM

```go
package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/intl"
)

func main() {
	vm, err := quickjs.NewVM()
	if err != nil {
		panic(err)
	}
	defer vm.Close()

	vm.SetMemoryLimit(64 << 20) // 64 MiB for this VM
	if err := vm.SetEvalTimeout(100 * time.Millisecond); err != nil {
		panic(err)
	}
	if err := intl.Register(vm); err != nil {
		panic(err)
	}

	v, err := vm.Eval(`new Date(0).toLocaleString('zh-CN', {timeZone: 'Asia/Shanghai'})`, quickjs.EvalGlobal)
	fmt.Println(v, err) // 1970/1/1 08:00:00 <nil>

	_, err = vm.Eval(`try { for (;;) {} } catch (e) { 'caught' }`, quickjs.EvalGlobal)
	var e *quickjs.Error
	if errors.As(err, &e) && e.Interrupted {
		fmt.Println("timed out, and no catch saw it")
	}
}
```

A VM is not safe for several goroutines at once: give each goroutine its own, or keep a pool. Intl and Temporal use embedded tz 2026e, so they need no host zoneinfo.

Beyond `Eval` and `Call`, the API has Go functions that JavaScript calls as built-ins, control of the GC and the memory of a VM, a clock per VM, `Temporal`, and parts of React's server renderer written in C. [docs/api.md](docs/api.md) lists them, and [pkg.go.dev](https://pkg.go.dev/github.com/liustack/quickjs) documents each.

## What changes for you

- **A limit ends the call.** A script that runs past its eval timeout or its memory limit stops there, whatever it catches: no `catch`, `finally`, promise handler or Go function of the page gets to run on. `*Error` says whether it was a timeout or memory. A panic in a Go function you gave the VM comes back to your Go code, and a VM that ran out of memory still closes without a leak.
- **The same page on every machine.** A script's local time is UTC in every VM, without setting `TZ` for the whole process. `Intl` formats in every locale Chromium has, compared with Chromium 156 on about 75,000 expressions.
- **Strings arrive whole.** A Go string from JavaScript is always valid UTF-8, and a Go string with invalid bytes reads in JavaScript as Go itself decodes it.
- **Faster than C, without C.** The changes of liustack/quickjs to the C of QuickJS make up for more than what compiling it to Go assembly costs (see [the table above](#how-fast)). Several VMs on several cores do not wait on one lock.

## Why you can trust it

- **The engine is upstream QuickJS.** `engine/` is Bellard's C at one pinned commit, with the changes of liustack/quickjs on top, each written down. LLVM compiles it to Go assembly that is checked in, so building needs no C compiler, and `make check-gen` fails when the checked-in files are not what the generator makes from `engine/` ([how to check](docs/testing.md#generate-and-check)).
- **The generator refuses what it has not checked.** It stops on an instruction, a call or a stack frame it has no rule for, and checks the linked code of a test binary, so a change of the C or of Go itself that breaks the engine shows up as a failed build or test, not as a wrong answer.
- **Every change is written down.** [docs/changes](docs/changes/README.md) gives each change with why it is safe, what it costs in memory, what it gains on which machine, and its tests.
- **The tests are upstream's and more.** test262 gives 33 known failures, 25 fewer since the fixes taken from upstream QuickJS, and the same ones from run to run. On top come the recorded outputs of the engine, checks in C under AddressSanitizer and UndefinedBehaviorSanitizer, and the Go tests of the API.
- **Someone depends on it.** It is kept for [tifa](https://github.com/liustack/tifa), a Go framework that renders React pages in it, and each release is measured on tifa's pages before tifa takes it.

## Documentation

| Doc | Read it when |
| :-- | :-- |
| [API](docs/api.md) | You want the Go API beyond `Eval` and `Call` |
| [Changes](docs/changes/README.md) | You want to know why each change to the engine is safe, what it costs and what it gains |
| [Performance](docs/performance.md) | You want to know how fast each release renders, or what was tried |
| [Benchmarks](docs/benchmarks.md) | You want to know how this engine compares with the other pure Go JavaScript engines |
| [Changelog](CHANGELOG.md) | You are upgrading and want to know what changed |
| [Architecture](docs/architecture.md) | You are about to change the code |
| [Testing](docs/testing.md) | You are about to commit, or want to check the engine |
| [Upgrading](docs/upgrading.md) | You are taking a commit of upstream QuickJS, new locale data or a newer Go |
| [Releasing](docs/releasing.md), [Commits](docs/commit.md) | You are making a release or a commit |
| [Upstream](docs/upstream.md) | You are moving from modernc.org/quickjs or github.com/liustack/slowjs, or want to know where the code comes from |
| [History](docs/history.md) | You want the numbers of the engine before v0.29, QuickJS translated to Go by ccgo |

## Where it comes from

The engine is [QuickJS](https://bellard.org/quickjs) by Fabrice Bellard and Charlie Gordon, the release of 2026-06-04. [docs/upstream.md](docs/upstream.md) has the exact version, how new QuickJS commits are taken in, and what to expect when you move from modernc.org/quickjs.

Thanks to Fabrice Bellard and Charlie Gordon for QuickJS, and to Jan Mercl, Dan Kortschak and the other authors of [modernc.org/quickjs](https://gitlab.com/cznic/quickjs) and [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs). Their work showed that QuickJS can run inside a pure Go program, and it inspired liustack/quickjs.

## Community

Issues are welcome any time: [open one](https://github.com/liustack/quickjs/issues/new). Tell us what you run in it, where it is slow, and what should come next. Follow the liustack WeChat official account, and come find me on X: **[@liustack](https://x.com/liustack)**. What changes in liustack/quickjs and tifa is shared on WeChat and X.

The **liustack** WeChat official account: AI startup opportunities, indie-dev insights, and hands-on AI tooling, delivered as they happen. Scan the QR code in WeChat, or search for "liustack":

<p align="center">
  <img src="https://raw.githubusercontent.com/liustack/quickjs/main/assets/wechat-qrcode.png" width="420" alt="liustack WeChat official account" />
</p>

## License

The files liustack/quickjs created are under the [MIT license](LICENSE-MIT). Code that came from other projects keeps its own license: the C of QuickJS, what remains from modernc.org/quickjs, the TLSF allocator, musl's libc functions and the locale data of `intl` and go-intl. The gopher of the banner is drawn after the Go gopher by Renée French (CC BY 4.0). [NOTICE.md](NOTICE.md) says which license covers which path, and which notices a program that ships this module has to carry.
