# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`modernc.org/libquickjs` is a **pure Go embeddable JavaScript engine** produced by
transpiling Bellard's [QuickJS](https://bellard.org/quickjs/) (C) to Go with
[`ccgo`](https://pkg.go.dev/modernc.org/ccgo/v4). It is *not* hand-written Go —
almost all of the code is machine-generated. This package exposes the raw,
low-level C ABI. The idiomatic Go wrapper lives in a separate repo,
[`modernc.org/quickjs`](https://pkg.go.dev/modernc.org/quickjs).

Pinned upstream: QuickJS commit `3d5e064…` (release 2026-06-04), test262 commit
`5c82069…`. Both hashes appear in `generator.go`, `Makefile`, and `builder.json`
and must stay in sync when bumping versions.

## Commands

Tests pass `-vet=off` because `go vet` chokes on the ~180K-line generated files;
keep that flag when running tests manually.

```sh
make editor      # fast compile-check loop: gofmt + `go test -c` (no run) + build generator
make shorttest   # go test -short  (skips the heavy Test262 suite)
make test        # full suite incl. Test262: go test -vet=off -v -timeout 24h -failfast
make bench       # microbenchmarks (BenchmarkMicro); `make cpu` adds a pprof profile
make generate    # regenerate ccgo_*.go for the CURRENT GOOS/GOARCH (see below)
make download    # fetch the pinned QuickJS + test262 zips (prerequisite for generate)
```

Running individual tests (add `-vet=off`):
- `go test -vet=off -run Test2` — arithmetic/`Date` eval cases (`make test2` is the same).
- `go test -vet=off -run Test` — evaluates the `.js` files under `testdata/tests/`.
- `go test -vet=off -run Test262` — full ECMAScript conformance run (needs the test262 zip).
- `go test -vet=off -run Test262 -f <path>` — run a single test262 file.
- Test flags: `-re <regexp>` filters subtests, `-keep` keeps the unpacked `test262/` dir, `-f <file>` selects one file, `-xtags <tags>` sets build tags for the run-test262 binary.

## Architecture

**Generated vs. hand-written.** The bulk of the repo is the `ccgo_<goos>_<goarch>.go`
files (~180K lines / ~5MB each), one per platform, each marked `DO NOT EDIT` and
carrying a `//go:build <goos> && <goarch>` constraint (`ccgo_windows.go` carries
`windows && (amd64 || arm64)`, and `0unsupported.go` stops a windows/arm64 build). **Never hand-edit these — regenerate instead.**
Fixes to transpilation belong in the patches (`internal/patch/quickjs.diff`,
`internal/patch/test262.diff`) or the generator, not in the output.

The only hand-written package files are small: `libquickjs.go` (package doc,
`init`, `MaxStackSlots`, the `_js_check_stack_overflow` override), `etc.go`
(`origin`/`todo`/`trc` debug helpers), `types_{linux,other}.go` (the `Size_t`
alias), and `64_test.go` (the `TJSValue` `tag`/`float64` decode helpers).
`libquickjs_windows.go` provides platform-specific libc stubs.

**The generation pipeline** (`generator.go`, `//go:build ignore`, run via
`go generate` / `make generate`): unzips the pinned QuickJS + test262 sources →
applies the `internal/patch/*.diff` patches via `git apply` → builds QuickJS's
`libquickjs.a` through ccgo's `-exec make` → transpiles the archive into
`ccgo_<goos>_<goarch>.go` → post-processes identifiers with `sed`. It also
regenerates the transpiled test262 runner under `internal/run-test262/`.
`make generate` only emits the file for the host platform; per-platform files are
produced by the modernc builder farm (the `... auto generate` commits), driven by
`builder.json` and the per-arch go.mod snapshots in `internal/autogen/`. Cross-
generating windows/amd64 (`make w64` / `make dev-w64`) requires a mingw toolchain
at a path hard-coded in `generator.go`.

**Naming conventions** come from the ccgo `--prefix-*` flags plus `sed` renames in
`generator.go`. The public QuickJS `JS_*` C functions become **`XJS_*`** in Go
(e.g. `XJS_NewRuntime`, `XJS_Eval`, `XFreeValue`); tagged structs get a `T` prefix
(`TJSValue`, `TJSRuntime`), enum constants an `E` prefix (`EJS_TAG_INT`), macros an
`M` prefix (`MJS_EVAL_TYPE_GLOBAL`), and internal symbols a `_` prefix. There are
no idiomatic Go types here — you call the C ABI directly.

**libc / TLS.** Every generated function takes a `*libc.TLS` (execution context)
from `modernc.org/libc` as its first argument, and memory is managed manually
(`libc.CString`, `libc.Xfree`, `libc.GoString`). `all_test.go`'s `eval` helpers are
the canonical example of driving the raw API: `NewTLS` → `XJS_NewRuntime` →
`XJS_NewContext` → `XJS_Eval` → inspect the `TJSValue` tag → free everything.

**Stack-overflow detection is special.** ccgo is invoked with
`-hide js_check_stack_overflow`, so QuickJS's native C stack check is dropped and
replaced by the hand-written `_js_check_stack_overflow` in `libquickjs.go`, which
compares `tls.StackSlots()` against a limit (the runtime's `Fstack_size`, defaulting
to `MaxStackSlots = 1000`). This matters because transpiled recursion runs on the Go
stack; the constant's comments record the tuning history.

**test262 conformance.** `Test262` unpacks the pinned suite, patches it, builds
`internal/run-test262` (itself a transpiled C program), and runs it against
`testdata/test262.conf` (with optional per-OS/arch overrides like
`testdata/test262_darwin_arm64.conf` and `testdata/test262_exclude_<os>_<arch>`).
The committed baseline of expected failures is `test262_errors.txt`; the test
parses the runner's `Result:` line and **fails only on *new* errors** vs. that
baseline. When a QuickJS bump changes conformance, update `test262_errors.txt`
(and `test262_report.txt`) accordingly.

## Supported platforms

darwin/{amd64,arm64}, linux/{amd64,arm64}, windows/amd64.
Building for a platform requires its `ccgo_<goos>_<goarch>.go` to be present in the
repo.
