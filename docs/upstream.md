---
summary: 'Where the code of this fork comes from, with the exact upstream versions, what the fork keeps and what it changed, how it takes new upstream releases, what someone switching from modernc.org/quickjs runs into, and who wrote the code it is built on'
read_when:
  - Switching a program from modernc.org/quickjs to this fork
  - Asking which upstream release the fork is built on, or who wrote the code
  - Checking what the fork kept from upstream and what it changed
---

# Upstream

English · [简体中文](upstream.zh-CN.md)

## Where the code comes from

The code was built in four steps, each on the one before:

1. **[QuickJS](https://bellard.org/quickjs)**, the JavaScript engine in C by Fabrice Bellard and Charlie Gordon. This fork's engine is the QuickJS release of 2026-06-04, commit `3d5e064e9dd67c70f7962836505a7fa067bf0a4e`.
2. **[modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs)**, that C translated to Go by ccgo, with no cgo. The fork ran v0.13.1 (2026-07-31) up to v0.28. Its patch to QuickJS is where `engine/` started, and the Go of the translation is gone.
3. **[modernc.org/quickjs](https://gitlab.com/cznic/quickjs)**, the Go API over libquickjs. The fork starts from v0.25.0 (2026-10-01), which is commit `ea7b851` in this repository's history.
4. **This fork**, `github.com/liustack/slowjs` (named `github.com/liustack/quickjs` until v0.27.x), which merges the Go API and the engine into one module.

modernc.org/libquickjs and modernc.org/quickjs are by the quickjs-go authors. Their `AUTHORS` file, kept as upstream has it in [AUTHORS](../AUTHORS), lists Jan Mercl for both and Dan Kortschak for modernc.org/quickjs. The authors of this fork are in [AUTHORS-FORK](../AUTHORS-FORK).

## What the fork keeps and what it changed

The fork keeps the Go API of modernc.org/quickjs. The engine is the C in `engine/` (QuickJS at the pinned commit with the fork's changes), compiled to Go assembly by `internal/goasm/gen` and checked in. The generated files are never edited by hand, and `make check-gen` fails when they are not what the generator makes. The test262 suite gives the same 58 known failures as upstream.

What it changed is listed one change at a time in [fork-changes](fork-changes/README.md): limits that no script can catch, the same output on every machine, `Intl` in every locale Chromium has, and faster renders. [upgrading.md](upgrading.md) says how a commit of upstream QuickJS or a new release of modernc.org/quickjs is taken in.

The C tree in `engine/` does not follow upstream release by release. A commit of upstream QuickJS made after the pinned one is taken into it on its own, with a test, when it makes renders faster or removes a crash or a wrong answer a page can reach, and left out otherwise. Those commits, and the ones tried and taken back, are in [upstream-commits.md](fork-changes/upstream-commits.md).

## Switching from modernc.org/quickjs

What a program that used modernc.org/quickjs v0.25.0 runs into:

- **Import paths.** `modernc.org/quickjs` becomes `github.com/liustack/slowjs`, and the package name `quickjs` becomes `slowjs`. Its `intl` and `fmt` packages move with it. The engine is inside the module, so `modernc.org/libquickjs` leaves your `go.mod`, and so does `modernc.org/libc`.
- **Go version.** The module needs Go 1.27, where upstream needs 1.26.
- **Local time is UTC.** A script's local time is UTC in every VM, whatever `TZ` the process has. Name a zone to `Intl` to show another one ([details](fork-changes/time-and-intl.md#local-time-is-utc-in-every-vm)).
- **Intl in every locale.** Package `intl` formats in every locale Chromium has, where upstream formats en-US only. Programs that import it embed the locale data and its notices ([details](fork-changes/time-and-intl.md#intl-in-every-locale-chromium-has)).
- **Limits end the call.** An interrupt, the eval timeout and running out of memory can no longer be caught by the script, through `catch`, promises, iterators or Go functions ([details](fork-changes/uncatchable.md)).
- **Errors say more.** `*Error` has `Uncatchable`, `OutOfMemory` and `Interrupted`. A string that cannot be converted comes back as an error rather than `""`. `Error()` of an `*Error` made in Go is `Name: Message` rather than empty ([details](fork-changes/uncatchable.md#a-panic-in-a-go-function)).
- **Strings.** A Go string from javascript is always valid UTF-8, and invalid UTF-8 from Go becomes one U+FFFD per byte ([details](fork-changes/go-api.md#valid-utf-8-for-lone-surrogates)).
- **Panics.** A panic in the engine itself, or a Go function that calls `runtime.Goexit`, leaves the VM good only for `Close` ([details](fork-changes/uncatchable.md#a-panic-in-a-go-function)).
- **Endless recursion** throws a `RangeError` rather than killing the process ([details](fork-changes/errors-and-stacks.md#endless-recursion-throws-rangeerror)).
- **No std, os or Worker in the engine.** The engine leaves out the parts of QuickJS's shell that the Go API never reached: it has no `js_init_module_std`, `js_init_module_os`, `js_std_loop` or `js_std_init_handlers`. The bytecode of modules works as before, and a program is a few hundred KB smaller ([details](fork-changes/trimmed.md)).
- **No module loading, shell helpers or blocking.** `VM.SetModuleLoader`, `VM.SetDefaultModuleLoader` with `ModuleLoaderFunc` and `ModuleNormalizeFunc`, `VM.StdAddHelpers` (`console.log`, `print`, `performance.now`, `__loadScript`) and `VM.SetCanBlock` are gone. A module can import only a module the VM already has, such as itself by its name `<eval>`, and `Atomics.wait` always throws ([details](fork-changes/go-api.md#no-module-loader-shell-helpers-or-setcanblock)).
- **Two platforms.** The module builds for linux/amd64 and darwin/arm64 only. A build for any other target, windows, darwin/amd64 and linux/arm64 included, stops with an error that names these two ([details](architecture.md#platforms)).
- **Another engine.** The engine is C compiled to Go assembly, where it was QuickJS translated to Go. Scripts give the same results, recursion is bounded by a count of calls (5000 by default, 10000 at most) and JSON nests 2000 levels at most ([details](fork-changes/c-engine.md)). `-tags slowjs_nogoasm`, `-tags slowjs_cengine` and the package `github.com/liustack/slowjs/lib` are gone.
- **New API.** `NewBuiltin`, `VM.NewBuiltinFunction` and `Args`, `VM.NewArray`, `VM.SetStrip` with `type Strip`, `VM.RunGC`, `VM.RunYoungGC` ([details](fork-changes/young-collection.md)), `VM.MemorySize` and `VM.GCThreshold`, `VM.MemoryUsage` with `type MemoryUsage` and `VM.ReleaseMemory`, `VM.SetShapeCache` with `DefaultShapeCache`, `VM.OnClose`, `NewSharedBytecode` with `VM.EvalSharedBytecode`, `GetAsyncContext` and `SetAsyncContext` ([details](fork-changes/go-api.md)), `VM.SetClock` and `VM.Now` ([details](fork-changes/time-and-intl.md#one-wall-clock-per-vm)), and package `temporal` ([details](fork-changes/temporal.md)).

The [changelog](../CHANGELOG.md) has every change of each release.

## Credits

The engine is Fabrice Bellard's and Charlie Gordon's QuickJS. Jan Mercl translated it to Go with ccgo, and the quickjs-go authors, Jan Mercl and Dan Kortschak, wrote the Go API this fork builds on. Their license files stay in the repository: [LICENSE](../LICENSE) and [LICENSE-MODERNC-LIBQUICKJS](../LICENSE-MODERNC-LIBQUICKJS) for the Go code and the patch, [LICENSE-LIBQUICKJS](../LICENSE-LIBQUICKJS) for QuickJS.
