# slowjs

Run today's npm code unchanged inside one static Go binary.

slowjs renders React slower than Node and faster than any other pure Go JavaScript engine, with no cgo and no Node process. It is QuickJS translated to Go and tuned for server rendering, built for tifa, a Go framework for React pages.

It is for Go programs that need to run JavaScript, such as rendering React on the server. The engine runs ES2023 with modules, async generators, proxies, BigInt and `Intl` in every Chromium locale.

It is an engine, not Node: there is no `require`, `fs` or `process`. Code from npm runs unchanged once it is bundled as for a browser, as React is.

English · [简体中文](README.zh-CN.md)

## Install

```sh
go get github.com/liustack/slowjs
```

Go 1.27 or later. No cgo and no C compiler: it builds with `CGO_ENABLED=0` and cross-compiles like any Go code.

## Platforms

linux and darwin on amd64 and arm64, and windows on amd64. A build for any other target stops with an error that names these. If you need freebsd, a 32-bit target, linux on riscv64, ppc64le, s390x or loong64, or windows on arm64, v0.27.x supports them, at the old import path `github.com/liustack/quickjs`.

## Moving from github.com/liustack/quickjs

slowjs is the same engine under a new name, from v0.28.0. Replace the import path `github.com/liustack/quickjs` with `github.com/liustack/slowjs` (the `lib`, `intl`, `temporal` and `fmt` packages too) and the package name `quickjs` with `slowjs`, as in `slowjs.NewVM()`. The build tags `quickjs_intl_nonames` and `quickjs_intl_nocjdict` are now `slowjs_intl_nonames` and `slowjs_intl_nocjdict`. v0.27.x stays available at the old path.

## A first VM

```go
package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/intl"
)

func main() {
	vm, err := slowjs.NewVM()
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

	v, err := vm.Eval(`new Date(0).toLocaleString('zh-CN', {timeZone: 'Asia/Shanghai'})`, slowjs.EvalGlobal)
	fmt.Println(v, err) // 1970/1/1 08:00:00 <nil>

	_, err = vm.Eval(`try { for (;;) {} } catch (e) { 'caught' }`, slowjs.EvalGlobal)
	var e *slowjs.Error
	if errors.As(err, &e) && e.Interrupted {
		fmt.Println("timed out, and no catch saw it")
	}
}
```

A VM is not safe for several goroutines at once: give each goroutine its own, or keep a pool. Intl and Temporal use embedded tz 2026e, so they need no host zoneinfo.

## What slowjs adds to the API

The API is that of modernc.org/quickjs, documented at [pkg.go.dev](https://pkg.go.dev/github.com/liustack/slowjs), with these additions:

- **`NewBuiltin(name, length, fn)` and `VM.NewBuiltinFunction(b)`**: a Go function javascript calls as it calls `Math.max`, with no reflection. `fn` reads its arguments from `Args` (`Number`, `String`, `Bool`, `Value`, `This`, `VM`, and `Latin1` or `UTF16` for the characters of a string as the engine holds them), which convert nothing, and returns a `Value`. A call that adds two numbers takes about 55 ns on an Apple M4, against about 470 ns through `RegisterHostFunc`. Make a Builtin once per process: VMs on several goroutines call it at once. An `*Error` it returns is thrown as the constructor its `Name` says (`Error`, `TypeError`, `RangeError`, `SyntaxError`, `ReferenceError` or `InternalError`), any other error as a `TypeError`. [More](docs/fork-changes/go-api.md#builtin-a-go-function-called-as-a-built-in)
- **`VM.NewArray(vals...)`**: an array of values made in Go, made as an array literal makes one, so a setter a page put on `Array.prototype` never runs. [More](docs/fork-changes/go-api.md#newarray)
- **`VM.SetStrip(s)`** with `type Strip`: `StripSource` compiles functions without their source, so their `toString` shows `[native code]`, and `StripDebug` leaves out line and column numbers. [More](docs/fork-changes/go-api.md#setstrip)
- **`Error()` of an `*Error` made in Go** is `Name: Message`, as `Error.prototype.toString` writes it, where it was empty. An error taken from javascript keeps the engine's text. [More](docs/fork-changes/go-api.md#error-of-an-error-made-in-go)
- **`VM.RunGC()`, `VM.MemorySize()` and `VM.GCThreshold()`**: run a GC now, read the memory a VM takes and the threshold of its next GC, so a program can collect between two calls into javascript and set the threshold for the next one. [More](docs/fork-changes/go-api.md#rungc-memorysize-and-gcthreshold)
- **`VM.RunYoungGC()`**: collect only the objects made since the last GC, so a server that runs a young GC after each render and a full one now and then spends on each GC about as long as the garbage of one render. [More](docs/fork-changes/young-collection.md)
- **`VM.OnClose(f)`**: run `f` when the VM is closed, so a package that keeps something per VM in a table of its own can drop it then. [More](docs/fork-changes/go-api.md#onclose)
- **Package `temporal`**: `temporal.Register(vm)` adds `Temporal`, every type and method V8 ships, computed in Go by go-intl with ICU's zones for tz 2026e built in, so the host's zoneinfo is never read. `temporal.Clock` sets the clock of `Temporal.Now`. See the linked doc for current binary size measurements. [More](docs/fork-changes/temporal.md)
- **`*Error` fields `Uncatchable`, `OutOfMemory` and `Interrupted`**: why a call ended, which no script can fake. [More](docs/fork-changes/go-api.md#error-says-why-a-call-ended)
- **`VM.SetShapeCache(n)` and `DefaultShapeCache`**: how many shapes of plain objects (a shape is the engine's record of which keys an object has, in which order) a VM keeps for the next object with the same keys. [More](docs/fork-changes/go-api.md#setshapecache-and-defaultshapecache)

The first five came in v0.26.17, the next two in v0.27.0.

## What changes for you

- **A limit ends the call.** A script that runs past its eval timeout or its memory limit stops there, whatever it catches: no `catch`, `finally`, promise handler or Go function of the page gets to run on. `*Error` says whether it was a timeout or memory. A panic in a Go function you gave the VM comes back to your Go code, and a VM that ran out of memory still closes without a leak.
- **The same page on every machine.** A script's local time is UTC in every VM, without setting `TZ` for the whole process. `Intl` formats in every locale Chromium has, compared with Chromium 156 on about 75,000 expressions.
- **Strings arrive whole.** A Go string from javascript is always valid UTF-8, and a Go string with invalid bytes reads in javascript as Go itself decodes it.
- **Faster pages.** On the React pages of the example shop of [tifa](https://github.com/liustack/tifa), a Go framework that renders React on the server, the engine renders 1.63 to 1.66 times as fast as the original modernc.org/quickjs it started from (measured at v0.26.15), and several VMs on several cores no longer wait on one lock.

## Why you can trust it

- **The engine is upstream QuickJS.** `lib/` is what ccgo makes from Bellard's C, merged from upstream libquickjs with its history. slowjs changes it only through `internal/fixlib`, which rewrites the generated Go mechanically, and through a C patch for a regeneration. The committed files are exactly what `fixlib` makes from the upstream files, byte for byte ([how to check](docs/testing.md#same-as-upstream-after-fixlib)).
- **It stops when upstream moves.** Every rewrite checks the code it touches against fingerprints or counts. When an upstream release changes that code, `fixlib` stops and names it, rather than rewriting code it has not seen.
- **Every change is written down.** [docs/fork-changes](docs/fork-changes/README.md) gives each change with why it is safe, what it costs in memory, what it gains on which machine, and its tests.
- **The tests are upstream's and more.** test262 gives the same 58 known failures as upstream libquickjs, line for line. On top come over 300 tests of its own, and leak checks under libc's allocator that tracks every allocation.
- **Someone depends on it.** It is kept for [tifa](https://github.com/liustack/tifa), a Go framework that renders React pages in it, and each release is measured on tifa's pages before tifa takes it.

## Where it comes from

The engine is [QuickJS](https://bellard.org/quickjs) by Fabrice Bellard and Charlie Gordon, the release of 2026-06-04. Jan Mercl translated it to Go with ccgo as [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs) (v0.13.1 here, under `lib/`), and the quickjs-go authors wrote the Go API over it, [modernc.org/quickjs](https://gitlab.com/cznic/quickjs) (v0.25.0 here, at the root). slowjs merges the two into one module, so that a fix to the engine ships with the Go API without waiting for an upstream release of either. [docs/upstream.md](docs/upstream.md) has the exact versions, what the fork kept and changed, and what changes when you switch from modernc.org/quickjs.

## Documentation

| Doc | Read it when |
| :-- | :-- |
| [Fork changes](docs/fork-changes/README.md) | You want to know what slowjs does differently from modernc.org/quickjs, and why |
| [Performance](docs/performance.md) | You want to know how fast each release renders, or what was tried |
| [Benchmarks](docs/benchmarks.md) | You want to know how this engine compares with the other pure Go JavaScript engines |
| [Changelog](CHANGELOG.md) | You are upgrading and want to know what changed |
| [Architecture](docs/architecture.md) | You are about to change the code |
| [Testing](docs/testing.md) | You are about to commit, or want to check slowjs against upstream |
| [Upgrading](docs/upgrading.md) | You are taking a new upstream release, or new locale data |
| [Releasing](docs/releasing.md), [Commits](docs/commit.md) | You are making a release or a commit |
| [Upstream](docs/upstream.md) | You are switching from modernc.org/quickjs, or want to know where the code comes from |

The API is documented at [pkg.go.dev](https://pkg.go.dev/github.com/liustack/slowjs).

## License

The files slowjs created are under the [MIT license](LICENSE-MIT), and its source files carry an SPDX line. 14 of them repeat parts of the engine, and their SPDX line says `MIT AND BSD-3-Clause`. Everything that came from upstream keeps its own license: the Go API and the engine as ccgo translated it, including the generated files in `lib/` and the fork's rewrites of them, are under BSD-3-Clause ([LICENSE](LICENSE), [lib/LICENSE](lib/LICENSE)) and QuickJS's MIT license ([LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS)). slowjs's changes to those files are offered under MIT as well, and the files stay usable under their upstream licenses. The locale data that `intl` embeds stays under the Unicode License V3 and the other notices in [intl/LICENSE-DATA](intl/LICENSE-DATA), and `internal/gointl`, a copy of go-intl, under go-intl's [Unicode License V3](internal/gointl/LICENSE), with the notices of the ICU data and of the V8 and Rust code that go-intl ported, which [NOTICE.md](NOTICE.md#code-and-data-that-go-intl-took-from-other-projects) lists.

A program that ships a binary built with this module has to carry the notices of these licenses. [NOTICE.md](NOTICE.md) says which license covers which path, and which texts to include.
