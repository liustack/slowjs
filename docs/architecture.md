---
summary: 'How the module is built: the three layers from the Go API down to libc, what each folder holds, the core types and their rules, the 32-bit and 64-bit value forms, how Go functions are called from javascript, how the eval timeout and Interrupt work, and which platforms must keep building'
read_when:
  - Starting work in this repository for the first time
  - Changing quickjs.go, quickjs32.go or quickjs64.go, or adding a method that runs javascript
  - Looking for where a piece of the engine, a fork change or a tool lives
  - Touching how a value is encoded, or reading a JSValue by hand
  - Changing the intl, fmt or compare packages, generator.go or internal/autogen
---

# Architecture

`github.com/liustack/quickjs` is a pure Go, cgo-free embeddable JavaScript engine (ES2023). It is a fork of two modernc.org modules, merged into one Go module:

| Upstream module | Upstream repository | Here |
| --- | --- | --- |
| `modernc.org/quickjs` (the Go API) | https://gitlab.com/cznic/quickjs | repository root, package `github.com/liustack/quickjs` |
| `modernc.org/libquickjs` (QuickJS transpiled to Go by ccgo) | https://gitlab.com/cznic/libquickjs | `lib/`, package `github.com/liustack/quickjs/lib` |

The fork starts from quickjs v0.25.0 and libquickjs v0.13.1 (QuickJS release 2026-06-04, commit `3d5e064`). The engine lives in the same module as the Go API so that a fix to it ships together with the Go API, without waiting for an upstream release of either module. [fork-changes](fork-changes/README.md) lists what the fork changed.

There is no C compiler, no cgo and no shared library: the whole engine cross-compiles as ordinary Go. `grep -rn 'import "C"'` finds nothing. Do not bring cgo in.

## The three layers

1. **`lib/`**: QuickJS as ccgo transpiled it, in `lib/ccgo_<goos>_<goarch>.go` (one file per target, `ccgo_windows.go` for both windows targets), plus the fork's hand-written Go next to it. Symbol conventions to know when editing the wrapper: `lib.X*` are exported C functions (`XJS_NewRuntime`, `XJS_Eval`), `lib.T*` C types (`TJSValue`, `TJSRuntime`, `TJSContext`, `TJSAtom`), `lib.E*` enum constants (`EJS_TAG_NULL`), `lib.M*` macro constants (`MJS_EVAL_TYPE_GLOBAL`, `MCONFIG_VERSION`). Every transpiled C function takes a `*libc.TLS` as its first argument.
2. **`modernc.org/libc`** (external): the C runtime in Go. `libc.TLS` is the emulated thread-local storage and C stack, where the engine keeps C locals whose address is taken (its "TLS frames"). Memory the engine touches is allocated with `libc.Xcalloc`, `libc.Xmalloc` and `libc.Xfree`, and strings with `libc.CString`, not on Go's heap.
3. **The Go API** (`quickjs.go` and the files next to it): what users import.

## Folders

| Path | What it holds |
| --- | --- |
| `quickjs.go`, `quickjs32.go`, `quickjs64.go`, `promise.go`, `builtin.go`, `etc.go` | The Go API |
| `tlsstack_linux.go`, `tlsstack_other.go` | How the VM puts libc's TLS stack back after a panic, and the 1 MiB block kept outside linux |
| `lib/ccgo_*.go` | The generated engine. Never edited by hand: `internal/fixlib` rewrites them |
| `lib/*.go` (the rest) | Hand-written Go the generated engine calls: `backtrace.go` (lazy error stacks), `uncatchable.go`, `outofmemory.go`, `objlit.go`, `regexskip.go`, `poll.go`, `runtime.go` (the fork's state at the end of `JSRuntime`), `frontcache.go`, `shapecache.go`, `forin.go`, `bitwise.go`, `eq.go`, `atomkey.go`, `jsonstr.go`, `funcshape.go`, `test262_runner.go` |
| `lib/internal/patch/quickjs.diff` | The fork's changes in C, for a regeneration of `lib/` (see [upgrading.md](upgrading.md#the-c-patch)) |
| `lib/internal/patch/test262.diff` | Upstream's patch to test262 |
| `lib/internal/run-test262/` | The test262 runner, a module of its own (see [testing.md](testing.md#test262)) |
| `lib/README.md`, `lib/CLAUDE.md`, `lib/Makefile`, `lib/generator.go`, `lib/builder.json` | Upstream libquickjs files, kept as they are so that a `git subtree pull` merges cleanly. `lib/generator.go` still assumes it is the root of its own module, and this fork does not regenerate `lib/` with it |
| `internal/fixlib/` | The command that puts the fork's changes into the generated Go (see [upgrading.md](upgrading.md)) |
| `intl/` | Package `github.com/liustack/quickjs/intl`, `Intl` for a VM |
| `intl/internal/gen/` | The generator of `intl/cldr.bin` and `intl/tzdst_table.go` |
| `intl/testdata/chromium/` | What Chromium answers, for the golden test, and `record.cjs`, which records it |
| `fmt/` | Package `github.com/liustack/quickjs/fmt` |
| `compare/` | A module of its own that benchmarks the engine against goja and fastschema/qjs |
| `internal/arewefastyet/` | The V8 benchmark suite the memgrind test and `compare/` run |
| `internal/autogen/`, `builder.json`, `generator.go`, `testdata/benchmarks/` | Upstream's builder fleet and benchmark table (see below) |
| `scripts/docs-list.sh` | The index of `docs/` |

## Core types

- `runtime` (unexported): wraps `lib.TJSRuntime` and its own `*libc.TLS`. One TLS per runtime.
- `VM`, the main handle: wraps a `lib.TJSContext` (a JS realm). Made by `NewVM()`, released by `Close()`. Not safe for concurrent use by several goroutines, and neither is `runtime`.
- `Value`: wraps a reference-counted `lib.TJSValue`. `Dup()` and `Free()` must be strictly paired and properly nested (the type's doc says how this breaks across goroutines and channels). Getting it wrong causes leaks or use after free, which the memgrind tests catch.
- `Object`, `Atom` (`lib.TJSAtom`) and `HostFunc` (`func(args []any) (any, error)`).
- `Builtin`, a Go function javascript calls as a built-in, and `Args`, the arguments of its call (see [Go functions called from javascript](#go-functions-called-from-javascript)).

## 32-bit and 64-bit values

`JSValue` is NaN-boxed, and its layout depends on the pointer size. It is split across two files with opposite build tags, and any change to how values are encoded must be made in both:

- `quickjs64.go` (`//go:build !(386 || arm)`): `TJSValue` is a struct with an `.Ftag` field.
- `quickjs32.go` (`//go:build 386 || arm`): `TJSValue` is a `uint64` with the NaN-boxing done by bit math.

Both define the same set: `sizeofJsValue`, `null`, `undefined`, `newFloat`, `newBool`, `tag`, `isException`, `jsvToInt32`, `jsvSetInt32`, `jsvToFloat64`, `jsvToPtr`.

linux/s390x is the only big-endian target, and there the layout of the 64-bit union shows. ccgo represents `JSValueUnion` as its widest member, a `uint64`, and reads the narrow members as values (`JS_MKVAL` zero-extends into the low 32 bits, `JS_VALUE_GET_INT` truncates). So the `int32` or bool payload lives in the low 32 bits, which are the last four bytes of the union on s390x. Read and write it only through `jsvToInt32` and `jsvSetInt32`: a raw `*(*int32)(unsafe.Pointer(&v))` reads every JS integer and boolean as 0 or false there. The float64 and pointer payloads are as wide as the union, so those stay plain casts.

## Go functions called from javascript

`fp(f)` takes the raw code pointer out of a Go func value (the second word of the interface) so that it can be handed to C as a function pointer: the interrupt handler, host functions and module loaders use it. Go functions given to javascript with `RegisterFunc` or `RegisterHostFunc` are kept alive in registries for the whole process, keyed by an int32 "magic": `goFuncs` (and `freeMagic` for reuse) under `goFuncsMu`, with magics handed out by the atomic counter `staticMagic`. Module loaders live in `customLoaders` under `customLoadersMu`. These are global to the process, not per VM.

A `Builtin` (`builtin.go`) is the fast path: made once per process by `NewBuiltin`, kept in the copy-on-write `builtins.list` indexed by its magic, and turned into a function object per VM by `VM.NewBuiltinFunction`. `callBuiltin` finds its VM from the context's opaque, the VM's id in `vms`, a table read without a lock. It keeps the `host` count, the TLS mark and the panic and uncatchable handling of `callGo`: change one, change the other. `VM.NewArray` builds an array with `JS_DefinePropertyValueUint32`, so no setter on `Array.prototype` runs: a Builtin returns a list with it.

## The eval timeout and Interrupt

Each `VM` has an `interruptData` struct, allocated in libc memory, that holds `interrupt int32` and `timeLimit int64`. Every method that runs javascript (`Eval`, `EvalValue`, `Call`, `CallValue`, `ExecutePendingJobs` and the others) calls `m.enter()` and `defer m.leave()` before it runs javascript. `enter` counts how deep the calls go, and only the outermost call runs `configureInterrupt()`, which sets the deadline again from `m.timeout` and clears the interrupt flag. So a Go function that evaluates more javascript on the same VM stays inside the outer call's time and keeps a pending `Interrupt()`. The C side, `interruptHandler`, reads both fields. The deadline is on the monotonic clock (see [uncatchable.md](fork-changes/uncatchable.md#the-eval-timeout-runs-on-the-monotonic-clock)).

`SetEvalTimeout` sets the duration. `Interrupt()` sets the flag atomically, from any goroutine. It reaches the flag through `VM.intr`, a struct on Go's heap whose mutex orders it with `Close`, which frees the flag. `Close` clears only the embedded `vmState`, never `intr`, so `Interrupt` is safe from any goroutine during and after `Close`. Keep new VM fields in `vmState`. When you add a method that runs javascript, add the `enter` and `leave` pair, or the timeout will not apply to it.

## Packages next to the engine

- **`intl/`**: `Intl.DateTimeFormat`, `Intl.NumberFormat`, `Intl.PluralRules` and the `toLocale*` methods, for a VM that calls `Register(vm)`. QuickJS has no `Intl`: neither Bellard's engine nor quickjs-ng implements ECMA-402, so it is not a build option, and a page that reaches for `Intl` without `Register` gets a `ReferenceError`. The locales are those Chromium has, from CLDR 48.2, and the output is compared with Chromium (see [time-and-intl.md](fork-changes/time-and-intl.md#intl-in-every-locale-chromium-has)). The javascript side is `intl.js` and `stub.js` (embedded with `go:embed`), and every formatting decision is made in Go. `intl.js` is a function of the `Builtin`s in `bridge.go`, which the install in `stub.js` calls it with, so none of them is ever on `globalThis`. A constructor resolves its locales and options into an id in the process-wide `registry.go` (bounded, shared by all VMs). Every later call passes the id, and a Builtin answers `undefined` for an id the registry dropped, on which `intl.js` resolves the same arguments again. Values cross as strings, numbers and booleans, lists as arrays (`VM.NewArray`), and `resolvedOptions()` as JSON. `stub.js` takes the built-ins both scripts use at `Register` time, before any page code runs, and `intl.js` uses only those, null-prototype descriptors and arrays, and a private field for internal slots (`intl/testdata/node/tamper.js` lists what a page may change). The comment at the top of `bridge.go` says how to add a constructor. `intl/memgrind_test.go`, which `make leak` runs, checks the handling of `Value`s. `intl/golden_test.go` is the check that matters after touching a formatter: the spec says what the options mean, but only the data knows details such as when a minute grows a leading zero.
- **`fmt/`**: package `github.com/liustack/quickjs/fmt`, part of this module (it has no `go.mod` of its own). It gives a VM Go's `fmt.Printf`, `Println`, `Sprintf` and `Sprintln` with `Register(obj Value)`, with lower-case names in javascript (`fmt.println`).
- **`compare/`**: a module of its own (`github.com/liustack/quickjs/compare`) that benchmarks this engine against `github.com/dop251/goja` and `github.com/fastschema/qjs`, plus the arewefastyet V8 suite in `internal/arewefastyet/`. `make benchmark` runs there. It is a module of its own so that its dependencies (goja, qjs, wazero, regexp2 and so on) stay out of the library's `go.mod`, where every user would see them. `compare/go.mod` has `replace github.com/liustack/quickjs => ../`, so the benchmarks always build the work tree and `go mod tidy` takes the versions of `libc` and the rest from the root `go.mod`: never copy those versions by hand. Its indirect requirements are only lower bounds, and a newer version in the root `go.mod` stops the benchmarks with `updates to go.mod needed`, so the `benchmark` target runs `go mod tidy` first. The `github.com/liustack/quickjs` require line is not resolved (the replace wins) and nothing reads it.
- **`internal/autogen/*.mod`**: 14 copies of `go.mod`, one per `GOOS_GOARCH`, kept by the "autogen" feature of upstream's modernc.org builder fleet (see `builder.json`). The "<arch> auto generate" commits before the fork are these. Do not edit them by hand.
- **`testdata/benchmarks/<os>_<arch>.txt`**: recorded benchmark output, one file per platform. The first line, `# root go.mod sha256 <hex>`, says what it was measured against.
- **`generator.go`** (`//go:build ignore`, run by `//go:generate go run generator.go`): runs `make benchmark` for the host platform and writes the `# Performance` table in the package doc of `quickjs.go` from the geomean rows of every `testdata/benchmarks/*.txt`. A table edited by hand is overwritten. It benchmarks only when the file of the host platform is missing or was measured against another root `go.mod`, then writes the table again either way. So a local `go generate` after a dependency upgrade runs the whole benchmark suite. The `CCGO:` label is the engine in `lib/`, and the `GOJA:` and `QJS:` labels come from `compare/go.mod`. This table compares engines on micro benchmarks: [benchmarks.md](benchmarks.md) has the comparison with other Go engines, and [performance.md](performance.md) render speed in tifa.

## Platforms

14 targets (see the package doc of `quickjs.go` and `builder.json`): darwin/{amd64,arm64}, freebsd/{amd64,arm64}, linux/{386,amd64,arm,arm64,loong64,ppc64le,riscv64,s390x} and windows/{amd64,arm64}. A change must keep `make build_all_targets` passing on all of them (see [testing.md](testing.md#every-target)).
