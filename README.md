# slowjs

Run today's npm code unchanged inside one static Go binary.

slowjs renders React slower than Node and faster than any other pure Go JavaScript engine, with no cgo and no Node process. It is QuickJS compiled to Go assembly and tuned for server rendering, built for tifa, a Go framework for React pages.

It is for Go programs that need to run JavaScript, such as rendering React on the server. The engine runs ES2023 with modules, async generators, proxies, BigInt and `Intl` in every Chromium locale.

It is an engine, not Node: there is no `require`, `fs` or `process`. Code from npm runs unchanged once it is bundled as for a browser, as React is.

English · [简体中文](README.zh-CN.md)

## Install

```sh
go get github.com/liustack/slowjs
```

Go 1.27 or later. No cgo and no C compiler: it builds with `CGO_ENABLED=0` and cross-compiles like any Go code. The Go assembly of the engine was checked against Go 1.27.2. It leans on how Go starts a function and handles a fault, so before you build with a newer Go, run the tests (see [Moving to a newer Go](docs/upgrading.md#moving-to-a-newer-go)).

## Platforms

linux/amd64 and darwin/arm64. A build for any other target stops with an error that names these two.

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
- **`VM.MemoryUsage()` and `VM.ReleaseMemory(keep)`**: see what a VM holds, by objects, strings, functions and arrays, and give memory it keeps free back to the system. [More](docs/fork-changes/go-api.md#memoryusage-and-releasememory)
- **`VM.RunYoungGC()`**: collect only the objects made since the last GC, so a server that runs a young GC after each render and a full one now and then spends on each GC about as long as the garbage of one render. [More](docs/fork-changes/young-collection.md)
- **`VM.OnClose(f)`**: run `f` when the VM is closed, so a package that keeps something per VM in a table of its own can drop it then. [More](docs/fork-changes/go-api.md#onclose)
- **`NewSharedBytecode(bytecode)` and `VM.EvalSharedBytecode(b)`**: VMs that load the same bytecode keep the source text and line tables of its functions in one copy, so each VM of tifa's example shop holds 2.36 MiB less, with the same `toString` and stacks. [More](docs/fork-changes/go-api.md#sharedbytecode)
- **Package `temporal`**: `temporal.Register(vm)` adds `Temporal`, every type and method V8 ships, computed in Go by go-intl with ICU's zones for tz 2026e built in, so the host's zoneinfo is never read. `Temporal.Now` reads the VM clock, and `temporal.Clock` overrides it for `Temporal.Now` alone. See the linked doc for current binary size measurements. [More](docs/fork-changes/temporal.md)
- **`*Error` fields `Uncatchable`, `OutOfMemory` and `Interrupted`**: why a call ended, which no script can fake. [More](docs/fork-changes/go-api.md#error-says-why-a-call-ended)
- **`VM.SetClock(now)` and `VM.Now()`**: set and read the wall clock of a VM, which Date, Intl and Temporal all read. nil restores the live clock. [More](docs/fork-changes/time-and-intl.md#one-wall-clock-per-vm)
- **`VM.SetShapeCache(n)` and `DefaultShapeCache`**: how many shapes of plain objects (a shape is the engine's record of which keys an object has, in which order) a VM keeps for the next object with the same keys. [More](docs/fork-changes/go-api.md#setshapecache-and-defaultshapecache)
- **`GetAsyncContext` and `SetAsyncContext`**: two Builtins that read and set an object promise jobs carry: a `then` or an `await` runs with the one set when it was registered, so an `AsyncLocalStorage` written on them finds its store after `await` as in Node. A VM that never sets one runs promises as before. [More](docs/fork-changes/go-api.md#the-async-context-of-promise-jobs)

The first five came in v0.26.17. Of the rest, the `*Error` fields came in v0.26.7, the shape cache in v0.26.16, `VM.OnClose` and package `temporal` in v0.27.0, `VM.RunYoungGC` in v0.27.3, `SharedBytecode` in v0.28.1, `VM.SetClock` and `VM.Now` in v0.28.2, and the async context in v0.28.5.

## What changes for you

- **A limit ends the call.** A script that runs past its eval timeout or its memory limit stops there, whatever it catches: no `catch`, `finally`, promise handler or Go function of the page gets to run on. `*Error` says whether it was a timeout or memory. A panic in a Go function you gave the VM comes back to your Go code, and a VM that ran out of memory still closes without a leak.
- **The same page on every machine.** A script's local time is UTC in every VM, without setting `TZ` for the whole process. `Intl` formats in every locale Chromium has, compared with Chromium 156 on about 75,000 expressions.
- **Strings arrive whole.** A Go string from javascript is always valid UTF-8, and a Go string with invalid bytes reads in javascript as Go itself decodes it.
- **Faster pages.** On the React pages of the example shop of [tifa](https://github.com/liustack/tifa), a Go framework that renders React on the server, a page takes 0.83 to 0.86 of the time it took on the engine of v0.28 and before, which was QuickJS translated to Go (see [the acceptance run](docs/performance.md#acceptance-before-the-switch)). Several VMs on several cores do not wait on one lock.

## Why you can trust it

- **The engine is upstream QuickJS.** `engine/` is Bellard's C at one pinned commit, with the changes of slowjs on top, each written down. LLVM compiles it to Go assembly that is checked in, so building needs no C compiler, and `make check-gen` fails when the checked-in files are not what the generator makes from `engine/` ([how to check](docs/testing.md#generate-and-check)).
- **The generator refuses what it has not checked.** It stops on an instruction, a call or a stack frame it has no rule for, and checks the linked code of a test binary, so a change of the C or of Go itself that breaks the engine shows up as a failed build or test, not as a wrong answer.
- **Every change is written down.** [docs/fork-changes](docs/fork-changes/README.md) gives each change with why it is safe, what it costs in memory, what it gains on which machine, and its tests.
- **The tests are upstream's and more.** test262 gives the same 58 known failures as before, line for line. On top come the recorded outputs of the engine, checks in C under AddressSanitizer and UndefinedBehaviorSanitizer, and the Go tests of the API.
- **Someone depends on it.** It is kept for [tifa](https://github.com/liustack/tifa), a Go framework that renders React pages in it, and each release is measured on tifa's pages before tifa takes it.

## Where it comes from

The engine is [QuickJS](https://bellard.org/quickjs) by Fabrice Bellard and Charlie Gordon, the release of 2026-06-04. Jan Mercl translated it to Go with ccgo as [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs), which slowjs ran up to v0.28 and which its C engine replaced. The quickjs-go authors wrote the Go API, [modernc.org/quickjs](https://gitlab.com/cznic/quickjs) (v0.25.0 here, at the root). slowjs keeps that API and puts its own engine under it. [docs/upstream.md](docs/upstream.md) has the exact versions, what the fork kept and changed, and what changes when you switch from modernc.org/quickjs.

## Documentation

| Doc | Read it when |
| :-- | :-- |
| [Fork changes](docs/fork-changes/README.md) | You want to know what slowjs does differently from modernc.org/quickjs, and why |
| [Performance](docs/performance.md) | You want to know how fast each release renders, or what was tried |
| [Benchmarks](docs/benchmarks.md) | You want to know how this engine compares with the other pure Go JavaScript engines |
| [Changelog](CHANGELOG.md) | You are upgrading and want to know what changed |
| [Architecture](docs/architecture.md) | You are about to change the code |
| [Testing](docs/testing.md) | You are about to commit, or want to check the engine |
| [Upgrading](docs/upgrading.md) | You are taking a commit of upstream QuickJS, or new locale data |
| [Releasing](docs/releasing.md), [Commits](docs/commit.md) | You are making a release or a commit |
| [Upstream](docs/upstream.md) | You are switching from modernc.org/quickjs, or want to know where the code comes from |

The API is documented at [pkg.go.dev](https://pkg.go.dev/github.com/liustack/slowjs).

## License

The files slowjs created are under the [MIT license](LICENSE-MIT), and its source files carry an SPDX line. Everything that came from upstream keeps its own license: the Go API is under BSD-3-Clause ([LICENSE](LICENSE), [LICENSE-MODERNC-LIBQUICKJS](LICENSE-MODERNC-LIBQUICKJS)), the C of the engine under QuickJS's MIT license ([LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS)), the allocator under TLSF's BSD-3-Clause ([LICENSE-TLSF](LICENSE-TLSF)), and the small libc under musl's MIT license and the notices in [engine/libc/COPYRIGHT](engine/libc/COPYRIGHT). slowjs's changes to those files are offered under MIT as well, and the files stay usable under their upstream licenses. The locale data that `intl` embeds stays under the Unicode License V3 and the other notices in [intl/LICENSE-DATA](intl/LICENSE-DATA), and `internal/gointl`, a copy of go-intl, under go-intl's [Unicode License V3](internal/gointl/LICENSE), with the notices of the ICU data and of the V8 and Rust code that go-intl ported, which [NOTICE.md](NOTICE.md#code-and-data-that-go-intl-took-from-other-projects) lists.

A program that ships a binary built with this module has to carry the notices of these licenses. [NOTICE.md](NOTICE.md) says which license covers which path, and which texts to include.
