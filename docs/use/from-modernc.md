---
summary: 'What breaks and what changes in behaviour when moving a Go program from modernc.org/quickjs v0.25.0 to github.com/liustack/slowjs'
read_when:
  - Moving a Go program from modernc.org/quickjs to github.com/liustack/slowjs
  - Checking which APIs or behaviours differ from modernc.org/quickjs v0.25.0
audience: user
order: 5
---

# Moving from modernc.org/quickjs

English · [简体中文](from-modernc.zh-CN.md)

This page covers what a Go program that used `modernc.org/quickjs` v0.25.0 runs into when switching to `github.com/liustack/slowjs`.

## What to change in your code and go.mod

- **Import paths and package name.** `modernc.org/quickjs` becomes `github.com/liustack/slowjs`, and the package name changes from `quickjs` to `slowjs`. Its `intl` and `fmt` packages move with it. The engine is inside the module, so `modernc.org/libquickjs` leaves your `go.mod`.
- **Go version.** The module needs Go 1.26.6 or later, and stops a Go version it has not been checked with (see [upgrading.md](../internals/upgrading.md#moving-to-a-newer-go)), where `modernc.org/quickjs` v0.25.0 needs Go 1.26.
- **Six platforms only.** The module builds for linux, darwin and windows, on amd64 and arm64, only. A build for any other target stops with an error that names the supported ones ([details](../internals/architecture.md#platforms)).
- **No module loading, shell helpers or blocking.** `VM.SetModuleLoader`, `VM.SetDefaultModuleLoader` with `ModuleLoaderFunc` and `ModuleNormalizeFunc`, `VM.StdAddHelpers` (`console.log`, `print`, `performance.now`, `__loadScript`) and `VM.SetCanBlock` are gone. A module can import only a module the VM already has, such as itself by its name `<eval>`, and `Atomics.wait` always throws ([details](../internals/changes/go-api.md#no-module-loader-shell-helpers-or-setcanblock)).

## What behaves differently

- **Local time is UTC.** A script's local time is UTC in every VM, whatever `TZ` the process has. Name a zone to `Intl` to show another one ([details](../internals/changes/time-and-intl.md#local-time-is-utc-in-every-vm)).
- **Intl in every locale.** Package `intl` formats in every locale Chromium has, where `modernc.org/quickjs` formats en-US only. Programs that import it embed the locale data and its notices ([details](../internals/changes/time-and-intl.md#intl-in-every-locale-chromium-has)).
- **Limits end the call.** An interrupt, the eval timeout and running out of memory can no longer be caught by the script, through `catch`, promises, iterators or Go functions ([details](../internals/changes/uncatchable.md)).
- **Errors say more.** `*Error` has `Uncatchable`, `OutOfMemory` and `Interrupted`. A string that cannot be converted comes back as an error rather than `""`. `Error()` of an `*Error` made in Go is `Name: Message` rather than empty ([details](../internals/changes/go-api.md#error-of-an-error-made-in-go)).
- **Strings.** A Go string from JavaScript is always valid UTF-8, and invalid UTF-8 from Go becomes one U+FFFD per byte ([details](../internals/changes/go-api.md#valid-utf-8-for-lone-surrogates)).
- **Panics.** A panic in the engine itself, or a Go function that calls `runtime.Goexit`, leaves the VM good only for `Close` ([details](../internals/changes/uncatchable.md#a-panic-in-a-go-function)).
- **Endless recursion** throws a `RangeError` rather than killing the process ([details](../internals/changes/errors-and-stacks.md#endless-recursion-throws-rangeerror)).
- **Another engine.** The engine is C compiled to Go assembly, where it was QuickJS translated to Go. Scripts give the same results, recursion is bounded by a count of calls (5000 by default, 10000 at most) and JSON nests 2000 levels at most ([details](../internals/changes/c-engine.md)).
- **No std, os or Worker in the engine.** The engine leaves out the parts of QuickJS's shell that the Go API never reached: it has no `js_init_module_std`, `js_init_module_os`, `js_std_loop` or `js_std_init_handlers`. The bytecode of modules works as before, and a program is a few hundred KB smaller ([details](../internals/changes/trimmed.md)).

## What is new

Everything the Go API has beyond `NewVM`, `Eval`, `Call` and `Value` is in [api.md](api.md), with the release each function came in.

The [changelog](../../CHANGELOG.md) has every change of each release.
