<h1 align="center">beastjs</h1>

<p align="center"><b>The fastest pure Go JavaScript engine for rendering React.</b></p>

<p align="center">
  <a href="./README.zh-CN.md">简体中文</a> ·
  <a href="#how-fast">How fast</a> ·
  <a href="docs/use/api.md">API</a> ·
  <a href="docs/use/benchmarks.md">Benchmarks</a> ·
  <a href="CHANGELOG.md">Changelog</a>
</p>

<p align="center">
  <a href="https://x.com/liustack"><img src="https://img.shields.io/badge/follow-%40liustack-black?style=flat-square&logo=x&logoColor=white" alt="Follow @liustack on X"></a>
  <a href="https://pkg.go.dev/github.com/liustack/beastjs"><img src="https://img.shields.io/badge/go.dev-reference-007d9c?style=flat-square&logo=go&logoColor=white" alt="Go reference"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/liustack/beastjs?style=flat-square" alt="Go version"></a>
  <img src="https://img.shields.io/badge/CGO-off-success?style=flat-square" alt="CGO off">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="License"></a>
  <img src="https://img.shields.io/badge/Not%20backed%20by-Y%20Combinator-FF6600?style=flat-square&logo=ycombinator&logoColor=white" alt="Not backed by Y Combinator">
  <img src="https://img.shields.io/badge/users-unknown-lightgrey?style=flat-square" alt="Users unknown">
</p>

Your Go server renders React pages itself, faster than C QuickJS on the same code, in one static binary with no cgo and no Node process beside it.

It is for Go programs that need to run JavaScript, such as rendering React on the server. The engine runs ES2023 with modules, async generators, proxies, BigInt, and `Intl` in every Chromium locale and `Temporal`, both built in. It is built for [tifa](https://github.com/liustack/tifa), a Go framework for React pages.

It is an engine, not Node: there is no `require`, `fs` or `process`. Code from npm runs unchanged once it is bundled as for a browser, as React is.

## How fast

CPU time of one render of the server bundles of tifa's apps, on one core, as a multiple of liustack/beastjs (lower is faster):

| Engine | Pure Go | React bench pages | MUI pages | Shop pages |
| :-- | :-: | --: | --: | --: |
| **liustack/beastjs** | yes | **1.00** | **1.00** | **1.00** |
| liustack/beastjs, on polyfills | yes | 1.03 | 1.71 | 1.62 |
| moejs | yes | 1.52 | 1.52 | 1.99 |
| go-quickjs | yes | 2.31 | 2.12 | 3.11 |
| C QuickJS 2026-06-04 | no, C | 2.06 | 3.63 | 2.59 |
| Hermes V1 | no, C++ | 1.08 | 1.90 | 1.86 |
| Node 26 `--jitless` | no, C++ | 0.99 | 1.11 | 0.90 |

Measured on v0.32.0 (the engine of v0.33.0, under the module path of then) on a GCP t2d-standard-4 (AMD EPYC 7B13), Go 1.27.4. The comparison is among interpreters: go-quickjs runs with its JIT off, as it comes, Hermes runs `hermesc -O` bytecode and Node runs with `--jitless`. Every engine runs as it comes, on its own `Intl` where it has one and on the FormatJS polyfill where it has none (C QuickJS and Hermes). The second row is liustack/beastjs without its built-in `Intl`, `URL` and `Temporal`, on the same polyfills as C QuickJS and Hermes: it shows how much of the lead on the MUI and shop pages comes from the interpreter and how much from `Intl` written in Go. The bench pages need no `Intl`, so that column is the same JavaScript on every engine.

It is not the fastest on everything: on the V8 benchmark suite go-quickjs and Node score higher, and Hermes about the same.

| Engine | Pure Go | V8 suite score |
| :-- | :-: | --: |
| go-quickjs | yes | 2308 |
| Node 26 `--jitless` | no, C++ | 2240 |
| **liustack/beastjs** | yes | **2160** |
| Hermes V1 | no, C++ | 2096 |
| moejs | yes | 1816 |
| C QuickJS 2026-06-04 | no, C | 1172 |

It scores 1.85 times C QuickJS and 1.20 times moejs, and is ahead of each of them in every round. go-quickjs scores 1.07 times as much as it does, and leads on the RegExp, NavierStokes, Crypto and DeltaBlue benchmarks.

[docs/use/benchmarks.md](docs/use/benchmarks.md) has both tables in full, what each engine was given to run, and how to run it all again.

## Install

```sh
go get github.com/liustack/beastjs
```

Go 1.26.6 or later, up to Go 1.27. No cgo and no C compiler: it builds with `CGO_ENABLED=0` and cross-compiles like any Go code. The Go assembly of the engine was checked against every patch release from Go 1.26.6 to 1.27.4, and a new patch release is checked every week. It leans on how Go starts a function and handles a fault, so a newer Go stops the build until liustack/beastjs has been checked with it and moves its limit in a patch release. `-tags beastjs_anygo` builds anyway (see [Moving to a newer Go](docs/internals/upgrading.md#moving-to-a-newer-go)).

## Platforms

linux, darwin and windows, on amd64 and arm64. A build for any other target stops with an error that names them.

## A first VM

```go
package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/liustack/beastjs"
	"github.com/liustack/beastjs/intl"
)

func main() {
	vm, err := beastjs.NewVM()
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

	v, err := vm.Eval(`new Date(0).toLocaleString('zh-CN', {timeZone: 'Asia/Shanghai'})`, beastjs.EvalGlobal)
	fmt.Println(v, err) // 1970/1/1 08:00:00 <nil>

	_, err = vm.Eval(`try { for (;;) {} } catch (e) { 'caught' }`, beastjs.EvalGlobal)
	var e *beastjs.Error
	if errors.As(err, &e) && e.Interrupted {
		fmt.Println("timed out, and no catch saw it")
	}
}
```

A VM is not safe for several goroutines at once: give each goroutine its own, or keep a pool. Intl and Temporal use embedded tz 2026e, so they need no host zoneinfo.

Beyond `Eval` and `Call`, the API has Go functions that JavaScript calls as built-ins, control of the GC and the memory of a VM, a clock per VM, `Temporal`, and parts of React's server renderer written in C. [docs/use/api.md](docs/use/api.md) lists them, and [pkg.go.dev](https://pkg.go.dev/github.com/liustack/beastjs) documents each.

## What changes for you

- **A limit ends the call.** A script that runs past its eval timeout or its memory limit stops there, whatever it catches: no `catch`, `finally`, promise handler or Go function of the page gets to run on. `*Error` says whether it was a timeout or memory. A panic in a Go function you gave the VM comes back to your Go code, and a VM that ran out of memory still closes without a leak.
- **The same page on every machine.** A script's local time is UTC in every VM, without setting `TZ` for the whole process. `Intl` formats in every locale Chromium has, compared with Chromium 156 on about 75,000 expressions.
- **Strings arrive whole.** A Go string from JavaScript is always valid UTF-8, and a Go string with invalid bytes reads in JavaScript as Go itself decodes it.
- **Faster than C, without C.** It renders faster than C QuickJS on the same pages, even though it runs as Go assembly (see [the table above](#how-fast)). Several VMs on several cores do not wait on one lock.

## Why you can trust it

- **Every change can be traced.** `engine/` started from Bellard's QuickJS at one pinned commit, and every change since is written down. LLVM compiles it to Go assembly that is checked in, so building needs no C compiler, and `make check-gen` fails when the checked-in files are not what the generator makes from `engine/` ([how to check](docs/internals/testing.md#generate-and-check)).
- **The generator refuses what it has not checked.** It stops on an instruction, a call or a stack frame it has no rule for, and checks the linked code of a test binary, so a change of the C or of Go itself that breaks the engine shows up as a failed build or test, not as a wrong answer.
- **Every change is written down.** [docs/internals/changes](docs/internals/changes/README.md) gives each change with why it is safe, what it costs in memory, what it gains on which machine, and its tests.
- **The tests are upstream's and more.** test262 gives 33 known failures, 25 fewer since the fixes taken from upstream QuickJS, and the same ones from run to run. On top come the recorded outputs of the engine, checks in C under AddressSanitizer and UndefinedBehaviorSanitizer, and the Go tests of the API.
- **Someone depends on it.** It is kept for [tifa](https://github.com/liustack/tifa), a Go framework that renders React pages in it, and each release is measured on tifa's pages before tifa takes it.

## Documentation

| Doc | Read it when |
| :-- | :-- |
| [API](docs/use/api.md) | You want the Go API beyond `Eval` and `Call` |
| [Changes](docs/internals/changes/README.md) | You want to know why each change to the engine is safe, what it costs and what it gains |
| [Performance](docs/internals/performance.md) | You want to know how fast each release renders, or what was tried |
| [Benchmarks](docs/use/benchmarks.md) | You want to know how this engine compares with the other pure Go JavaScript engines |
| [Changelog](CHANGELOG.md) | You are upgrading and want to know what changed |
| [Architecture](docs/internals/architecture.md) | You are about to change the code |
| [Testing](docs/internals/testing.md) | You are about to commit, or want to check the engine |
| [Upgrading](docs/internals/upgrading.md) | You are taking a commit of upstream QuickJS, new locale data or a newer Go |
| [Releasing](docs/internals/releasing.md), [Commits](docs/internals/commit.md) | You are making a release or a commit |
| [Moving from modernc.org/quickjs](docs/use/from-modernc.md) | You are moving a program from modernc.org/quickjs |
| [Upstream](docs/internals/upstream.md) | You want to know where the code comes from |

## Where it comes from

The engine is [QuickJS](https://bellard.org/quickjs) by Fabrice Bellard and Charlie Gordon, the release of 2026-06-04. [docs/internals/upstream.md](docs/internals/upstream.md) has the exact version, how new QuickJS commits are taken in, and what to expect when you move from modernc.org/quickjs.

Thanks to Fabrice Bellard and Charlie Gordon for QuickJS, and to Jan Mercl, Dan Kortschak and the other authors of [modernc.org/quickjs](https://gitlab.com/cznic/quickjs) and [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs). Their work showed that QuickJS can run inside a pure Go program, and it inspired liustack/beastjs.

## Community

Issues are welcome any time: [open one](https://github.com/liustack/beastjs/issues/new/choose). Tell us what you run in it, where it is slow, and what should come next. Follow the liustack WeChat official account, and come find me on X: **[@liustack](https://x.com/liustack)**. What changes in liustack/beastjs and tifa is shared on WeChat and X.

The **liustack** WeChat official account: AI startup opportunities, indie-dev insights, and hands-on AI tooling, delivered as they happen. Scan the QR code in WeChat, or search for "liustack":

<p align="center">
  <img src="https://raw.githubusercontent.com/liustack/beastjs/main/assets/wechat-qrcode.png" width="420" alt="liustack WeChat official account" />
</p>

## Contributing

liustack/beastjs does not accept pull requests. One maintainer reviews and owns every line, on purpose, so the engine stays dependable. Two ways that help:

- **[Open an issue](https://github.com/liustack/beastjs/issues/new/choose).** A wrong answer, a slow render, a limit that did not hold, an unclear doc. Issues are read and decide what gets built next.
- **Fork it.** Your copy is yours to change and publish.

A security problem goes in a private report, as [SECURITY.md](SECURITY.md) says. [CONTRIBUTING.md](CONTRIBUTING.md) has the rest.

## License

liustack/beastjs is under the [MIT license](LICENSE). Code that came from other projects keeps its own license: the C of QuickJS, the TLSF allocator, musl's libc functions, the Go side and the locale data of `Intl` and `Temporal` from go-intl, ICU and Chromium, the URL parser from nlnwa/whatwg-url and a few files of Go. [NOTICE.md](NOTICE.md) says which license covers which path, and which notices a program that ships this module has to carry.
