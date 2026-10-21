# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`modernc.org/quickjs` is a **pure-Go, CGo-free embeddable JavaScript engine** (ES2023). It is an
idiomatic Go wrapper around `modernc.org/libquickjs` — the Bellard QuickJS C library mechanically
transpiled to Go with `ccgo`. There is no C compiler, no CGo, and no shared library involved; the
whole engine cross-compiles as ordinary Go. Confirm this before reaching for CGo: `grep -rn 'import "C"'`
returns nothing.

## Build, lint, test

The workflow is driven by the `Makefile`, not by bare `go` commands.

```sh
make editor        # primary build+lint gate: gofmt -s -w, go test -c (compile only),
                   #   go build generator.go, go install ./..., staticcheck
make test          # go test -failfast -v -timeout 24h -count=1 ./...
make shorttest     # same, adds -short (skips long-running cases)
make leak          # go test -v -run TestMemgrind2 -tags=libc.memgrind  (leak detection)
make benchmark     # delegates to ./compare (see below)
make build_all_targets   # cross-build + compile the tests of every package for all 14 GOOS/GOARCH, ±libc.memgrind
```

Run a single test / subtest:

```sh
go test -run TestEval -v            # one test
go test -run 'TestEval/eval1' -v    # one subtest (t.Run subtests are common here)
```

Lint tools used: `gofmt -s`, `staticcheck`, `golint`. Always run `gofmt -s -w .` (or `make editor`)
before considering a change done.

### Local dev environment

**There is normally no `go.work`.** The whole repo — including `make benchmark` in `compare/`, which
reaches the root module through its `replace` — builds and tests in plain module mode. `go.work` is
untracked (global gitignore) and `make clean` deletes it; treat it as a temporary tool, not config.

Create one with `make work` for exactly one situation: **co-developing the sibling checkouts** of the
transpiled dependencies, `../libc` (`modernc.org/libc`) and `../libquickjs` (`modernc.org/libquickjs`).
Changes to the value representation or new binding surface usually need an unreleased `libquickjs`
next door. Delete it (`make clean`, or `rm -f go.work go.work.sum`) once those land in a release, or
the tree keeps building against whatever is in the sibling working copy rather than the pinned version.

While a `go.work` exists it must keep listing `./compare`: a workspace is found by walking up from the
cwd, so it also governs builds inside `compare/`, which then fail with `main module
(modernc.org/quickjs) does not contain package modernc.org/quickjs/compare`. `make work` also stamps
the current toolchain into the `go` line — a `go.work` left over from an older toolchain, below
`compare/go.mod`'s `go` version, breaks `make benchmark` with `module . listed in go.work file
requires go >= …`.

### Memgrind (leak detection)

`memgrind_test.go` is guarded by `//go:build libc.memgrind`. The `libc.memgrind` tag swaps in libc's
allocation-tracking allocator. Because JS `Value`s are reference-counted C objects, leak tests are the
canonical way to validate `Dup`/`Free` pairing — see the `TestMemgrind` doc.

## Architecture

### The three-layer stack

1. **`modernc.org/libquickjs`** (external) — transpiled QuickJS. Symbol conventions to know when
   editing the wrapper: `lib.X*` = exported C functions (e.g. `XJS_NewRuntime`, `XJS_Eval`),
   `lib.T*` = C types (`TJSValue`, `TJSRuntime`, `TJSContext`, `TJSAtom`), `lib.E*` = enum constants
   (`EJS_TAG_NULL`), `lib.M*` = macro constants (`MJS_EVAL_TYPE_GLOBAL`, `MCONFIG_VERSION`). Every
   transpiled C function takes a `*libc.TLS` as its first argument.
2. **`modernc.org/libc`** (external) — C-runtime emulation. `libc.TLS` is the emulated thread-local
   storage / C stack; memory the engine touches is allocated via `libc.Xcalloc`/`libc.Xfree` and
   strings via `libc.CString` — **not** Go's GC heap.
3. **This package** (`quickjs.go` + friends) — the public Go API.

### Core types (quickjs.go)

- `runtime` (unexported): wraps `lib.TJSRuntime` plus its own `*libc.TLS`. One TLS per runtime.
- `VM` (the main handle): wraps a `lib.TJSContext` (a JS realm). Created by `NewVM()`, released by
  `Close()`. **Not safe for concurrent use by multiple goroutines** — same for `runtime`.
- `Value`: wraps a reference-counted `lib.TJSValue`. `Dup()`/`Free()` must be **strictly paired and
  properly nested** (the type doc spells out how this breaks across goroutines/channels). Getting this
  wrong causes leaks or use-after-free, caught by the memgrind tests.
- `Object`, `Atom` (= `lib.TJSAtom`), `HostFunc` (`func(args []any) (any, error)`).

### 32-bit vs 64-bit value representation — keep both in sync

`JSValue` uses NaN-boxing whose layout depends on pointer size. This is split across two files with
opposite build tags, and **any change to value encoding must be mirrored in both**:

- `quickjs64.go` (`//go:build !(386 || arm)`): `TJSValue` is a struct with an `.Ftag` field.
- `quickjs32.go` (`//go:build 386 || arm`): `TJSValue` is a `uint64` with explicit NaN-boxing bit math.

Both define the same set: `sizeofJsValue`, `null`, `undefined`, `newFloat`, `newBool`, `tag`,
`isException`, `jsvToInt32`, `jsvSetInt32`, `jsvToFloat64`, `jsvToPtr`.

linux/s390x is the only **big-endian** target and it makes the 64-bit union layout observable: `ccgo`
represents `JSValueUnion` as its widest member, an `uint64`, and accesses the narrow members in the
*value* domain (`JS_MKVAL` zero-extends into the low 32 bits, `JS_VALUE_GET_INT` truncates). So the
`int32`/bool payload lives in the low 32 bits, which are the *last* four bytes of the union on s390x.
Read and write it only through `jsvToInt32`/`jsvSetInt32` — a raw `*(*int32)(unsafe.Pointer(&v))`
silently decodes every JS integer and boolean as 0/false there. The float64 and pointer payloads are
as wide as the union itself, so those stay plain casts.

### C → Go callbacks

`fp(f)` extracts the raw code pointer from a Go func value (second word of the interface) so it can be
handed to C as a function pointer (used for the interrupt handler, host functions, module loaders).
Go functions exposed to JS via `RegisterFunc`/`RegisterHostFunc` are kept alive in package-global
registries keyed by an int32 "magic": `goFuncs` (+ `freeMagic` for reuse) under `goFuncsMu`, magics
handed out by the `staticMagic` atomic counter. Custom module loaders live in `customLoaders` under
`customLoadersMu`. These globals are process-wide, not per-VM.

A `Builtin` (`builtin.go`) is the fast path: made once per process by `NewBuiltin`, kept in the
copy-on-write `builtins.list` indexed by its magic, and turned into a function object per VM by
`VM.NewBuiltinFunction`. `callBuiltin` finds its VM from the context's opaque, the VM's id in `vms`, a
table read without a lock. It keeps the `host` count, the TLS mark and the panic and uncatchable
handling of `callGo`: change one, change the other.

### Interrupt / eval timeout

Each `VM` owns an `interruptData` struct (allocated in libc memory) holding `interrupt int32` and
`timeLimit int64`. Every eval-family entry point (`Eval`, `EvalValue`, `Call`, `CallValue`,
`ExecutePendingJobs`, …) calls `m.enter()` / `defer m.leave()` **before** running JS. `enter` counts
the nesting depth and only the outermost call runs `configureInterrupt()`, which re-arms the deadline
from `m.timeout` and clears the interrupt flag; a host function that evaluates more JS on the same VM
stays inside the outer call's budget and keeps a pending `Interrupt()`. The C-side `interruptHandler`
polls both fields. `SetEvalTimeout` sets the duration; `Interrupt()` sets the flag atomically for async
cancellation from another goroutine. It reaches the flag through `VM.intr`, a Go-heap struct whose mutex
orders it with `Close` (which frees the flag): `Close` clears only the embedded `vmState`, never `intr`, so
`Interrupt` is safe from any goroutine during and after `Close`. Keep new VM fields in `vmState`. When adding a new entry point that runs JS, remember the
`enter`/`leave` pair or the timeout won't apply.

## Sub-packages and generated assets

- `fmt/` — package `modernc.org/quickjs/fmt` of *this* module (no `go.mod` of its own, unlike
  `compare/`); exposes Go's `fmt.Printf/Println/Sprintf/Sprintln` to a VM via `Register(obj Value)`
  (JS names are lower-cased, e.g. `fmt.println`).
- `intl/` — package `modernc.org/quickjs/intl`, also part of this module: an opt-in ECMA-402
  (`Intl.DateTimeFormat`, `Intl.NumberFormat`) implementation in Go, installed by
  `Register(vm *VM, opts ...Option)`. QuickJS has no `Intl` — neither Bellard's engine nor quickjs-ng
  implements it, so it is not a build option — and a page reaching for `Intl` gets a `ReferenceError`.
  Register also replaces the `toLocale*` methods, which ECMA-402 redefines in terms of those two
  constructors and whose QuickJS fallbacks silently ignore the `timeZone` argument.
  - The javascript surface lives in `intl.js` (`go:embed`ed by `intl.go`) and every formatting
    decision in Go. `intl.js` is a function of the `Builtin`s in `bridge.go`, which `stub.js`'s
    install calls it with, so none of them is ever on `globalThis`. A constructor resolves its
    locales and options into an id in the process-wide `registry.go` (bounded, shared by all VMs);
    every later call passes the id, and a Builtin answers `undefined` for an id the registry
    dropped, on which `intl.js` resolves the same arguments again. Values cross as strings,
    numbers and booleans, results that are objects as JSON. `stub.js` takes the built-ins both scripts use before any page code runs (Register time), and `intl.js` uses only those, null-prototype descriptors and arrays, and private fields for internal slots: `intl/testdata/node/tamper.js` lists what a page may change.
    `intl/memgrind_test.go` guards the `Value` handling (and `make leak` now runs `./...`).
  - Locale data is en-US only: Go ships zoneinfo but no CLDR. The tables in `data.go` were extracted
    from ICU, not hand-written. Where CLDR data is missing the code falls back to something
    derivable (a GMT offset for a long time zone name, the code for a currency name), never to a
    guess.
  - **`intl/icu_test.go` is the check that matters** after touching a formatter: `TestAgainstICU`
    evaluates ~4000 expressions in both node's `Intl` and this package and fails on any difference
    not in its table of four known, documented ones. It skips when `node` is absent (so the fleet
    never runs it) and in `-short` mode. The spec says what the options mean; only CLDR knows that
    a minute beside an hour grows a leading zero while a minute alone does not — do not "fix" a
    formatter against the prose without running this.
- `internal/gointl/`: a copy of go-intl (pure Go ECMA-402 + Temporal) at the commit in
  `internal/gointl/UPSTREAM`, not yet wired into JS. Only `qjs*` files and `UPSTREAM` are ours. Edit
  go-intl's files only through `qjs.patch` and `qjs-sync.sh` (see README, "Updating go-intl"). Its
  data is the packs in `qjsdata/`, written by `internal/qjspack/gen`, not go-intl's `data.pack`.
  `make gointl` runs the checks that need build tags, and checks that the packs are current.
- `compare/` — **separate module** (`modernc.org/quickjs/compare`, own `go.mod`) benchmarking this
  engine against `github.com/dop251/goja` and `github.com/fastschema/qjs`, plus the arewefastyet V8
  suite in `internal/arewefastyet/`. `make benchmark` runs here. It is its own module so that the
  benchmark-only deps (goja, qjs, wazero, regexp2, …) and its newer `go` directive stay out of the
  library's `go.mod`, where every consumer would see them. `compare/go.mod` carries
  `replace modernc.org/quickjs => ../`, so the benchmarks always build the working tree and
  `go mod tidy` derives `libquickjs`/`libc` from the root `go.mod` — never hand-sync those versions.
  Its indirect requires are only lower bounds, and a bump in the root `go.mod` otherwise stops the
  benchmarks with `updates to go.mod needed`, so the `benchmark` target runs `go mod tidy` first.
  The `modernc.org/quickjs vX.Y.Z` require line is *not* resolved (the replace wins) and nothing reads
  it; keep it at or below the newest tag, `modernc.org/gomod`'s updater — the fleet's autodeps — errors
  out on a require ahead of the local clone.
- `internal/autogen/*.mod` — 14 per-`GOOS_GOARCH` snapshots of `go.mod`, maintained by the
  modernc.org builder's "autogen" feature (see `builder.json`); the recent "<arch> auto generate"
  commits are these. Don't hand-edit. Emptying them is how you ask the fleet to re-run `go generate`
  on every platform: the builder regenerates when the snapshot differs from the root `go.mod`.
- `testdata/benchmarks/<os>_<arch>.txt` — recorded benchmark output, one file per platform, first line
  `# root go.mod sha256 <hex>` recording what it was measured against.
- `generator.go` (`//go:build ignore`, standalone `package main`, run via `//go:generate go run
  generator.go`): runs `make benchmark` for the host platform and **rewrites the `# Performance` table
  inside the `quickjs.go` package doc comment** from the geomean rows of every
  `testdata/benchmarks/*.txt`. If you edit that table by hand it will be overwritten. The `CCGO:` label
  is the `modernc.org/libquickjs` version from the root `go.mod` — the engine that was measured; the
  `GOJA:`/`QJS:` labels come from `compare/go.mod`, where they *are* resolved.

  It benchmarks only when `testdata/benchmarks/<goos>_<goarch>.txt` is missing or its recorded
  fingerprint differs from the current root `go.mod`, then re-renders the table unconditionally
  (rendering is idempotent, so a builder with nothing to measure commits nothing). Keying on the root
  `go.mod` makes "the fleet regenerated" and "these numbers are stale" the same event: the builder's
  own autogen gate compares that same file against `internal/autogen/<goos>_<goarch>.mod`. Nothing has
  to be bumped by hand to re-measure — but note a local `go generate` after a dependency bump does run
  the full benchmark suite.

## Supported platforms

14 combos (see the `quickjs.go` package doc and `builder.json`): darwin/{amd64,arm64},
freebsd/{amd64,arm64}, linux/{386,amd64,arm,arm64,loong64,ppc64le,riscv64,s390x}, windows/{amd64,arm64}.
Changes must keep `make build_all_targets` green across all of them.
