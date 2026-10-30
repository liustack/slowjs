---
summary: 'Where the engine and the Go API come from, how a commit of upstream QuickJS is taken in, what a program moving from modernc.org/quickjs runs into, and who wrote the code slowjs is built on'
read_when:
  - Moving a program from modernc.org/quickjs to slowjs
  - Asking which QuickJS commit the engine is built on, or who wrote the code
  - Taking a commit of upstream QuickJS into the engine
---

# Upstream

English · [简体中文](upstream.zh-CN.md)

## Where the code comes from

- **The engine** is [QuickJS](https://bellard.org/quickjs), the JavaScript engine in C by Fabrice Bellard and Charlie Gordon, at the release of 2026-06-04, commit `3d5e064e9dd67c70f7962836505a7fa067bf0a4e`. `engine/` holds that C with the changes of slowjs on top. `internal/goasm/gen` compiles it to Go assembly, which is checked in. The generated files are never edited by hand, and `make check-gen` fails when they are not what the generator makes. The test262 suite gives 58 known failures, listed in `engine/test/test262_errors.txt`.
- **The Go API** (`VM`, `Value`, `Eval`, `Call` and the rest at the root of the module) started from [modernc.org/quickjs](https://gitlab.com/cznic/quickjs) v0.25.0 (2026-10-01), commit `ea7b851` in this repository's history, and has been rewritten and extended since. Some of its names, comments and tests remain. Until v0.28 the engine under it was [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs), QuickJS translated to Go by ccgo. That engine is gone, and so is its Go.
- **Everything else** is written for slowjs, or comes from a project named in [NOTICE.md](../NOTICE.md): the allocator (TLSF), the small libc of the engine (musl), the locale data and the Go side of `Intl` and `Temporal` (go-intl, ICU, Chromium).

The authors of slowjs are in [AUTHORS-SLOWJS](../AUTHORS-SLOWJS). The `AUTHORS` file of modernc.org/quickjs stays in the repository as that project has it: [AUTHORS](../AUTHORS).

## Taking QuickJS commits

The C tree in `engine/` does not follow upstream release by release. A commit of upstream QuickJS made after the pinned one is taken into it on its own, with a test, when it fixes a wrong answer, a leak or a crash (whether or not a page can reach it) or makes renders faster, and a new feature or a change for speed when tifa has a use for it. Those commits, and the ones tried and taken back, are in [upstream-commits.md](changes/upstream-commits.md). [upgrading.md](upgrading.md) says how to take one in.

The changes of slowjs to the C are listed one at a time in [changes](changes/README.md): limits that no script can catch, the same output on every machine, `Intl` in every locale Chromium has, and faster renders.

## Moving from modernc.org/quickjs

What a program that used modernc.org/quickjs v0.25.0 runs into:

- **Import paths.** `modernc.org/quickjs` becomes `github.com/liustack/slowjs`, and the package name `quickjs` becomes `slowjs`. Its `intl` and `fmt` packages move with it. The engine is inside the module, so `modernc.org/libquickjs` leaves your `go.mod`.
- **Go version.** The module needs Go 1.27, where modernc.org/quickjs v0.25.0 needs 1.26.
- **Local time is UTC.** A script's local time is UTC in every VM, whatever `TZ` the process has. Name a zone to `Intl` to show another one ([details](changes/time-and-intl.md#local-time-is-utc-in-every-vm)).
- **Intl in every locale.** Package `intl` formats in every locale Chromium has, where modernc.org/quickjs formats en-US only. Programs that import it embed the locale data and its notices ([details](changes/time-and-intl.md#intl-in-every-locale-chromium-has)).
- **Limits end the call.** An interrupt, the eval timeout and running out of memory can no longer be caught by the script, through `catch`, promises, iterators or Go functions ([details](changes/uncatchable.md)).
- **Errors say more.** `*Error` has `Uncatchable`, `OutOfMemory` and `Interrupted`. A string that cannot be converted comes back as an error rather than `""`. `Error()` of an `*Error` made in Go is `Name: Message` rather than empty ([details](changes/uncatchable.md#a-panic-in-a-go-function)).
- **Strings.** A Go string from javascript is always valid UTF-8, and invalid UTF-8 from Go becomes one U+FFFD per byte ([details](changes/go-api.md#valid-utf-8-for-lone-surrogates)).
- **Panics.** A panic in the engine itself, or a Go function that calls `runtime.Goexit`, leaves the VM good only for `Close` ([details](changes/uncatchable.md#a-panic-in-a-go-function)).
- **Endless recursion** throws a `RangeError` rather than killing the process ([details](changes/errors-and-stacks.md#endless-recursion-throws-rangeerror)).
- **No std, os or Worker in the engine.** The engine leaves out the parts of QuickJS's shell that the Go API never reached: it has no `js_init_module_std`, `js_init_module_os`, `js_std_loop` or `js_std_init_handlers`. The bytecode of modules works as before, and a program is a few hundred KB smaller ([details](changes/trimmed.md)).
- **No module loading, shell helpers or blocking.** `VM.SetModuleLoader`, `VM.SetDefaultModuleLoader` with `ModuleLoaderFunc` and `ModuleNormalizeFunc`, `VM.StdAddHelpers` (`console.log`, `print`, `performance.now`, `__loadScript`) and `VM.SetCanBlock` are gone. A module can import only a module the VM already has, such as itself by its name `<eval>`, and `Atomics.wait` always throws ([details](changes/go-api.md#no-module-loader-shell-helpers-or-setcanblock)).
- **Two platforms.** The module builds for linux/amd64 and darwin/arm64 only. A build for any other target, windows, darwin/amd64 and linux/arm64 included, stops with an error that names these two ([details](architecture.md#platforms)).
- **Another engine.** The engine is C compiled to Go assembly, where it was QuickJS translated to Go. Scripts give the same results, recursion is bounded by a count of calls (5000 by default, 10000 at most) and JSON nests 2000 levels at most ([details](changes/c-engine.md)).
- **New API.** `NewBuiltin`, `VM.NewBuiltinFunction` and `Args`, `VM.NewArray`, `VM.SetStrip` with `type Strip`, `VM.RunGC`, `VM.RunYoungGC` ([details](changes/young-collection.md)), `VM.MemorySize` and `VM.GCThreshold`, `VM.MemoryUsage` with `type MemoryUsage` and `VM.ReleaseMemory`, `VM.SetShapeCache` with `DefaultShapeCache`, `VM.OnClose`, `NewSharedBytecode` with `VM.EvalSharedBytecode`, `GetAsyncContext` and `SetAsyncContext` ([details](changes/go-api.md)), `VM.SetClock` and `VM.Now` ([details](changes/time-and-intl.md#one-wall-clock-per-vm)), and package `temporal` ([details](changes/temporal.md)).

The [changelog](../CHANGELOG.md) has every change of each release.

## Credits

The engine is Fabrice Bellard's and Charlie Gordon's QuickJS. The Go API began as modernc.org/quickjs, by the quickjs-go authors Jan Mercl and Dan Kortschak, who also translated QuickJS to Go with ccgo as modernc.org/libquickjs. slowjs thanks them for the API it started from. Their license files stay in the repository, as the code that remains from them needs: [LICENSE](../LICENSE) and [LICENSE-MODERNC-LIBQUICKJS](../LICENSE-MODERNC-LIBQUICKJS), and [LICENSE-LIBQUICKJS](../LICENSE-LIBQUICKJS) for QuickJS.
