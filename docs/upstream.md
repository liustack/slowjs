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

1. **[QuickJS](https://bellard.org/quickjs)**, the JavaScript engine in C by Fabrice Bellard and Charlie Gordon. This fork's engine is the QuickJS release of 2026-06-04, commit `3d5e064e9dd67c70f7962836505a7fa067bf0a4e` (see `qjsCommit` in `lib/generator.go`).
2. **[modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs)**, that C translated to Go by ccgo, with no cgo. The fork took v0.13.1 (2026-07-31) into `lib/` with `git subtree`.
3. **[modernc.org/quickjs](https://gitlab.com/cznic/quickjs)**, the Go API over libquickjs. The fork starts from v0.25.0 (2026-10-01), which is commit `ea7b851` in this repository's history.
4. **This fork**, `github.com/liustack/quickjs`, which merges the Go API and the engine into one module.

modernc.org/libquickjs and modernc.org/quickjs are by the quickjs-go authors. Their `AUTHORS` files, kept as upstream has them in [AUTHORS](../AUTHORS) and [lib/AUTHORS](../lib/AUTHORS), list Jan Mercl for both and Dan Kortschak for modernc.org/quickjs. The authors of this fork are in [AUTHORS-FORK](../AUTHORS-FORK).

## What the fork keeps and what it changed

The fork keeps the Go API of modernc.org/quickjs and the engine as ccgo makes it. The generated files under `lib/` are never edited by hand: `internal/fixlib` rewrites them mechanically after every upstream release, and the committed files are exactly what `fixlib` makes from the upstream files, byte for byte ([how to check](testing.md#same-as-upstream-after-fixlib)). The test262 suite gives the same 58 known failures as upstream.

What it changed is listed one change at a time in [fork-changes](fork-changes/README.md): limits that no script can catch, the same output on every machine, `Intl` in every locale Chromium has, and faster renders. [upgrading.md](upgrading.md) says how a new release of libquickjs, of modernc.org/quickjs or of modernc.org/libc is taken in.

## Switching from modernc.org/quickjs

What a program that used modernc.org/quickjs v0.25.0 runs into:

- **Import paths.** `modernc.org/quickjs` becomes `github.com/liustack/quickjs`, and its `intl` and `fmt` packages move with it. The engine is `github.com/liustack/quickjs/lib`, so `modernc.org/libquickjs` leaves your `go.mod`.
- **Go version.** The module needs Go 1.27, where upstream needs 1.26.
- **Local time is UTC.** A script's local time is UTC in every VM, whatever `TZ` the process has. Name a zone to `Intl` to show another one ([details](fork-changes/time-and-intl.md#local-time-is-utc-in-every-vm)).
- **performance.now.** The `performance.now` of `StdAddHelpers` counts from the start of the process, where upstream counts from the boot of the system, or from 1970 on Windows and FreeBSD (it reads the wall clock there), and on macOS it leaves out the time the system sleeps ([details](fork-changes/time-and-intl.md#the-engine-reads-gos-clocks)).
- **Intl in every locale.** Package `intl` formats in every locale Chromium has, where upstream formats en-US only. Programs that import it embed the locale data and its notices ([details](fork-changes/time-and-intl.md#intl-in-every-locale-chromium-has)).
- **Limits end the call.** An interrupt, the eval timeout and running out of memory can no longer be caught by the script, through `catch`, promises, iterators or Go functions ([details](fork-changes/uncatchable.md)).
- **Errors say more.** `*Error` has `Uncatchable`, `OutOfMemory` and `Interrupted`. A string that cannot be converted comes back as an error rather than `""`. `Error()` of an `*Error` made in Go is `Name: Message` rather than empty ([details](fork-changes/go-api.md)).
- **Strings.** A Go string from javascript is always valid UTF-8, and invalid UTF-8 from Go becomes one U+FFFD per byte ([details](fork-changes/go-api.md#valid-utf-8-for-lone-surrogates)).
- **Panics.** A panic in the engine itself, or a Go function that calls `runtime.Goexit`, leaves the VM good only for `Close` ([details](fork-changes/calls-and-tls-frames.md#frames-freed-without-defer)).
- **Endless recursion** throws a `RangeError` rather than killing the process ([details](fork-changes/errors-and-stacks.md#endless-recursion-throws-rangeerror)).
- **No std, os or Worker in `lib`.** The engine leaves out the parts of QuickJS's shell that the Go API never reached: `lib` no longer has `js_init_module_std`, `js_init_module_os`, `js_std_loop`, `js_std_init_handlers` and six more. `StdAddHelpers`, `SetDefaultModuleLoader` and the bytecode of modules work as before, and a program is a few hundred KB smaller ([details](fork-changes/trimmed.md)).
- **New API.** `NewBuiltin`, `VM.NewBuiltinFunction` and `Args`, `VM.NewArray`, `VM.SetStrip` with `type Strip`, `VM.RunGC`, `VM.RunYoungGC` ([details](fork-changes/young-collection.md)), `VM.MemorySize` and `VM.GCThreshold`, and `VM.SetShapeCache` with `DefaultShapeCache` ([details](fork-changes/go-api.md)).

The [changelog](../CHANGELOG.md) has every change of each release.

## Credits

The engine is Fabrice Bellard's and Charlie Gordon's QuickJS. Jan Mercl translated it to Go with ccgo, and the quickjs-go authors, Jan Mercl and Dan Kortschak, wrote the Go API this fork builds on. Their license files stay in the repository: [LICENSE](../LICENSE) and [lib/LICENSE](../lib/LICENSE) for the Go code, [LICENSE-LIBQUICKJS](../LICENSE-LIBQUICKJS) for QuickJS.
