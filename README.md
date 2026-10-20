# quickjs (liustack fork)

English · [简体中文](README.zh-CN.md)

This repository is a fork of two modernc.org modules, merged into one Go module:

| Upstream module | Upstream repository | Here |
|---|---|---|
| `modernc.org/quickjs` (the Go API) | https://gitlab.com/cznic/quickjs | repository root, package `github.com/liustack/quickjs` |
| `modernc.org/libquickjs` (QuickJS transpiled to Go by ccgo) | https://gitlab.com/cznic/libquickjs | `lib/`, package `github.com/liustack/quickjs/lib` |

It starts from quickjs v0.25.0 and libquickjs v0.13.1 (QuickJS release
2026-06-04, commit `3d5e064`), with the changes listed in
[docs/fork-changes](docs/fork-changes/README.md).

## Install

```sh
go get github.com/liustack/quickjs
```

Go 1.27 or later. No cgo and no C compiler: it builds with `CGO_ENABLED=0` and cross-compiles like any Go code.

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

A VM is not safe for several goroutines at once: give each goroutine its own, or keep a pool. To resolve time zone names on a machine without zoneinfo, import `time/tzdata` in your main package.

## What changes for you

- **A limit ends the call.** A script that runs past its eval timeout or its memory limit stops there, whatever it catches: no `catch`, `finally`, promise handler or Go function of the page gets to run on. `*Error` says whether it was a timeout or memory. A panic in a Go function you gave the VM comes back to your Go code, and a VM that ran out of memory still closes without a leak.
- **The same page on every machine.** A script's local time is UTC in every VM, without setting `TZ` for the whole process. `Intl` formats in every locale Chromium has, compared with Chromium 156 on about 60,000 expressions.
- **Strings arrive whole.** A Go string from javascript is always valid UTF-8, and a Go string with invalid bytes reads in javascript as Go itself decodes it.
- **Faster pages.** On the React pages of tifa's example shop, the engine renders 1.63 to 1.66 times as fast as the original modernc.org/quickjs it started from (measured at v0.26.15), and several VMs on several cores no longer wait on one lock.

## Why you can trust it

- **The engine is upstream QuickJS.** `lib/` is what ccgo makes from Bellard's C, merged from upstream libquickjs with its history. The fork changes it only through `internal/fixlib`, which rewrites the generated Go mechanically, and through a C patch for a regeneration. The committed files are exactly what `fixlib` makes from the upstream files, byte for byte ([how to check](docs/testing.md#same-as-upstream-after-fixlib)).
- **It stops when upstream moves.** Every rewrite checks the code it touches against fingerprints or counts. When an upstream release changes that code, `fixlib` stops and names it, rather than rewriting code it has not seen.
- **Every change is written down.** [docs/fork-changes](docs/fork-changes/README.md) gives each change with why it is safe, what it costs in memory, what it gains on which machine, and its tests.
- **The tests are upstream's and more.** test262 gives the same 58 known failures as upstream libquickjs, line for line. On top come over 300 tests of the fork's own, and leak checks under libc's allocator that tracks every allocation.
- **Someone depends on it.** It is kept for [tifa](https://github.com/liustack/tifa), a Go framework that renders React pages in it, and each release is measured on tifa's pages before tifa takes it.

## Documentation

| Doc | Read it when |
| :-- | :-- |
| [Fork changes](docs/fork-changes/README.md) | You want to know what this fork does differently from modernc.org/quickjs, and why |
| [Benchmarks](docs/benchmarks.md) | You are choosing a JavaScript engine for Go and want to compare it with goja, sobek, otto, modernc.org/quickjs and fastschema/qjs |
| [Performance](docs/performance.md) | You want to know how fast each release renders, or what was tried |
| [Changelog](CHANGELOG.md) | You are upgrading and want to know what changed |
| [Architecture](docs/architecture.md) | You are about to change the code |
| [Testing](docs/testing.md) | You are about to commit, or want to check the fork against upstream |
| [Upgrading](docs/upgrading.md) | You are taking a new upstream release, or new locale data |
| [Releasing](docs/releasing.md), [Commits](docs/commit.md) | You are making a release or a commit |
| [The upstream README](docs/upstream.md) | You want upstream's own description of the package |

The API is documented at [pkg.go.dev](https://pkg.go.dev/github.com/liustack/quickjs).

## Upstream and license

The Go API is [modernc.org/quickjs](https://gitlab.com/cznic/quickjs) and the engine [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs), both by the quickjs-go authors (Jan Mercl and others), the engine translated from [QuickJS](https://bellard.org/quickjs) by Fabrice Bellard and Charlie Gordon. The engine lives in the same module as the Go API so that a fix to it ships together with the Go API, without waiting for an upstream release of either module.

The license files of both projects are kept: [LICENSE](LICENSE) and [LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS) at the root, and [lib/LICENSE](lib/LICENSE), [lib/LICENSE-LIBQUICKJS](lib/LICENSE-LIBQUICKJS) and [lib/LICENSE_TEST262](lib/LICENSE_TEST262) for the engine. [intl/LICENSE-DATA](intl/LICENSE-DATA) holds the notices of the locale data `intl` embeds, from Unicode CLDR and Chromium's ICU data: a program that imports `intl` distributes that data and has to include those notices in its documentation or with its copies.
