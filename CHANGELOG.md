# Changelog

What changed in `github.com/liustack/quickjs` that programs using it can see. Each release is a patch version, tagged on `main`. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). [Fork changes](docs/fork-changes/README.md) has the details of each change, and [Performance](docs/performance.md) how each speedup was measured. Times are for a server render of a page of tifa's example shop, on linux/amd64 (AMD EPYC) unless an entry says otherwise.

## [Unreleased]

### Added

- `Intl.Collator`, `Intl.Segmenter`, `Intl.RelativeTimeFormat`, `Intl.ListFormat`, `Intl.DisplayNames`, `Intl.DurationFormat`, `Intl.Locale` and `Intl.supportedValuesOf`, in every locale Chromium has, compared with Chromium 156 on 14,269 expressions. `localeCompare` sorts by the locale's collation, and `toLocaleUpperCase` and `toLocaleLowerCase` follow Turkish, Azeri, Lithuanian and Greek rules. They read go-intl's data, which makes a program that imports `intl` about 9.8 MB larger, or 5.8 MB with the build tags `quickjs_intl_nonames` and `quickjs_intl_nocjdict`, which give up the display names, so that `Intl.DisplayNames` answers each code with its fallback (the code, or `undefined` with `fallback: 'none'`), and the Chinese and Japanese word dictionary, so that a run of Chinese or Japanese is one word. A VM that has used `Intl` holds 151 KB more than after `Register`, against 77 KB, and its first use takes 0.49 ms, against 0.24 ms. A program that imports `intl` now has to carry the notices of `internal/gointl` (see `NOTICE.md`).
- `Args.WTF8`: read a string argument of a Builtin with its lone surrogates as the engine holds them, where `Args.String` gives U+FFFD for each.
- `VM.SetRegExpCache`, `DefaultRegExpCache` and `DefaultRegExpCacheBytes`: how many compiled regexps a VM keeps, and in how many bytes (256 in 1 MiB by default). The cache counts against the memory limit: give a VM that runs close to its limit a smaller cache, or none.
- `VM.OnClose`: run a function when the VM is closed, so a package that keeps something per VM in a table of its own can drop it then.
- Package `temporal`: `temporal.Register(vm)` adds `Temporal`, every type and method V8 ships, and `Date.prototype.toTemporalInstant`, computed in Go by go-intl with ICU's zones for tz 2026e built in, so the host's zoneinfo is never read. `Temporal.Now` uses the zone `intl.TimeZone` sets, and the clock `temporal.Clock` sets. test262 passes 9206 of the 9210 runs of `built-ins/Temporal` and 3972 of the 4058 runs of `intl402/Temporal`. It adds 0.84 MB to the example shop of tifa, and its first use takes 13.4 ms in a process and 1.1 ms in a later VM.

### Changed

- `performance.now()` of `StdAddHelpers` counts from the start of the process. Before, it counted from the boot of the system on Linux and macOS, and from 1970 on Windows and FreeBSD, where it moved when the system clock was set. On macOS it no longer counts the time the system sleeps. `Date.now()` and `new Date()` give the same values as before.

### Removed

- `lib` no longer has the `std`, `os` and `Worker` modules of QuickJS's shell (`quickjs-libc.c`), which the Go API never registered: `js_init_module_std`, `js_init_module_os`, `js_module_check_attributes`, `js_std_set_worker_new_context_func`, `js_std_init_handlers`, `js_std_free_handlers`, `js_std_promise_rejection_tracker`, `js_std_await`, `js_std_loop`, `js_std_eval_binary` and `js_std_eval_binary_json_module` are gone. `StdAddHelpers`, `SetDefaultModuleLoader` and the bytecode of modules work as before. A program no longer links that code, nor the parts of libc only it called (`fork`, `execve`, `popen`, `setenv`, files, the terminal): one that only calls `Eval` is 354 KB smaller on linux/amd64, and the server of tifa's example shop 181 KB smaller, built as `tifa build` builds it (`-ldflags=-w` with the shop's PGO profile, linux/amd64).

### Fixed

- `Intl` no longer throws on a page that replaced `String.prototype.concat`, which QuickJS calls for a template literal: `Intl.NumberFormat`, `Intl.DateTimeFormat` and the `toLocale` methods threw there in v0.26.17.
- `Intl.PluralRules` selects the plural of a compact or scientific number as ICU does, and every `supportedLocalesOf` reads its `localeMatcher` option.
- A function in a list of locales is read as an object, as ECMA-402 has it, so `new Intl.NumberFormat([function () {}])` throws a RangeError, where v0.26.17 threw a TypeError.
- The first use of `Intl` keeps what a page did before it: its own `Date.prototype.toLocaleString` or `Number.prototype.toLocaleString` stays, so does an `Intl` it defined with `Object.defineProperty` or deleted, and a frozen `Date.prototype` or `globalThis` no longer makes that use throw a `TypeError`.
- A zone whose CLDR name the host's zoneinfo lacks, such as `Asia/Calcutta` or `Europe/Kiev` on a Linux without tzdata-legacy, is loaded under its other names, so `Asia/Kolkata` and `Europe/Kyiv` work there too, and `Africa/Asmera`, which `Intl.supportedValuesOf('timeZone')` lists.
- `EvalBytecode` of a module whose top-level await is still pending when no job is left to run, such as `await new Promise(() => {})`, returns an error that says so instead of crashing the process. If what the module waits for settles later, from Go for one, a later `ExecutePendingJobs` runs the rest of it. It waits for the module as `ExecutePendingJobs` runs jobs: a job that throws no longer stops it, the module runs to its end and the call returns the job's error, or the module's own rejection. The eval timeout, an interrupt, out of memory or a panic in a Go function after a module's first `await` ends the call at once, where it was printed to stderr and the process then crashed.
- On Windows, `Date.now()` and `new Date()` no longer overflow in 2038.
- test262's intl402 tests pass 6274 runs of 6764 (with Temporal's), against 912, none of those that passed before failing.

### Performance

All without PGO, against v0.26.17.

- A VM keeps the bytecode of the regexps it compiled, so code that makes the same regexps on every render compiles them once: the journal post of tifa's example shop that highlights CSS renders in 0.893 of the time, the other pages in the same time. A VM of the shop holds 12 KiB more, and one that rendered the CSS post keeps about 714 KiB of regexps.
- `Date.now()`, `new Date()` and `performance.now()` read the time from Go instead of a system call of modernc.org/libc: 122 ns rather than 689, 261 rather than 870, and 107 rather than 686. A render of the shop does not change.

## [v0.26.17] - 2026-10-21

### Added

- `NewBuiltin` and `VM.NewBuiltinFunction`: a Go function that javascript calls as a built-in, such as `Math.max`, with `Args` to read its arguments without conversions. A call that adds two numbers takes about 55 ns on an Apple M4, against about 470 ns through `RegisterHostFunc`. An `*Error` it returns is thrown as the error its `Name` says.
- `VM.NewArray`: an array of values made in Go, made as an array literal makes one, so no setter a page put on `Array.prototype` runs.
- `VM.SetStrip` with `StripSource` and `StripDebug`: compile functions without their source, so their `toString` shows `[native code]`, or without line and column numbers.
- `VM.RunGC`, `VM.MemorySize` and `VM.GCThreshold`: run a GC now, read the memory a VM takes as the memory limit counts it, and read the threshold of the next GC. A program can collect between two calls into javascript and set the threshold for the next one, as tifa does between two renders.

### Changed

- `Error()` of an `*Error` made in Go is `Name: Message`, as javascript's `Error.prototype.toString` writes it, where it was empty. So the `TypeError` a `RegisterHostFunc` function throws for such an error now has that message. An error taken from javascript keeps the engine's text.
- `Intl` formatters are shared by every VM of the process, up to 1024, and the oldest are dropped first. A VM that has used `Intl` holds about 15 KB less.
- `Intl`'s functions have the shape of built-ins: a `length` and `name` as ECMA-402 gives them, no `prototype`, they cannot be called with `new`, and `toString` shows `[native code]`.
- `Intl.NumberFormat.call(o)` and `Intl.DateTimeFormat.call(o)`, for an `o` that inherits from their prototype, keep the new formatter on `o` and return it, as V8 does.
- The files this fork created are under the MIT license (`LICENSE-MIT`), and its source files carry an SPDX line. 14 files in `lib/` that repeat parts of the engine say `MIT AND BSD-3-Clause`. Code from upstream stays under BSD-3-Clause and QuickJS's MIT license, and the locale data under the Unicode License V3. `NOTICE.md` lists which license covers which path, and which notices a program that ships a binary has to include.
- The README is a short landing page, in English and Chinese. What it held moved to `docs/`: the fork changes by area, testing, upgrading from upstream, performance, releasing, and where the code comes from, with what changes when switching from modernc.org/quickjs.

### Fixed

- A page that changes `Array.prototype`, `Object.prototype`, `JSON` or other built-ins after `Register` no longer changes what `Intl` does.
- `Intl` converts options, values and locale lists as ECMA-402 does: a Symbol or a BigInt where a string or a number is wanted throws a `TypeError`, `null` locales throw a `TypeError`, and options are read in the order ECMA-402 reads them.
- `Intl.NumberFormat` formats a wrapped BigInt, or an object whose `valueOf` gives a long numeric string, exactly, where it rounded it through a double.
- test262's intl402 tests pass 912 runs of 6764, against 770, none of those that passed before failing.

### Performance

All without PGO, against v0.26.16.

- `===`, `!==`, `==` and `!=` answer the usual pairs (`null`, `undefined`, booleans, objects, symbols, strings that are atoms) in the interpreter: 0.982 of the time.
- A string key that is already an atom is found in one step: 1.3% less time. Reading with such a key takes 13% less time in a micro benchmark, writing 30% less.
- A call of a javascript function takes one TLS frame instead of two on linux, and 34 more functions take a TLS frame less often: about a fifth fewer frames per render, and 1.4% less time.
- A regexp of alternatives no longer tries the alternatives that cannot match at a position: about 1% to 2.5% less time. A `replace` with react-spring's regexp of 148 color names takes 5.9 µs rather than 44.5.
- A new function gets its `length` and `name` without the shapes in between: 0.982 of the time.
- Bitwise operators on floats skip the slow path: the pages of tifa's MUI test app take 0.952 to 0.960 of the time. The shop runs them too rarely to gain.
- A `for in` loop skips prototypes with no enumerable property: up to 3% less time on pages that run many such loops.
- `JSON.stringify` and `JSON.parse` copy runs of plain characters at once: quoting a route's data takes 41% less time, though a render gains less than 1%.
- `Intl`: `new Intl.NumberFormat('en')` takes 2.7 µs rather than 16.5, a `DateTimeFormat` with a time zone 8.1 µs to make rather than 32.0 and 2.6 µs to format rather than 5.2, on an AMD EPYC 7B13 core.

## [v0.26.16] - 2026-10-20

### Added

- `VM.SetShapeCache` and `DefaultShapeCache`: how many shapes of plain objects a VM keeps once their objects are gone. A new VM keeps 2048, and 0 turns the cache off. `lib` has `JS_SetShapeCache`, off until called.

### Changed

- A VM of the shop holds 722 KiB more after a GC (8.03 MiB rather than 7.33), 647 KiB of it the kept shapes. Kept shapes count against the memory limit and are not let go when an allocation fails, so a script that runs close to its limit can run out of memory where it did not before: give such a VM a smaller shape cache, or 0.
- Objects that get keys no other object has, one at a time, take about 6% longer to build than with the shape cache off.

### Performance

- The TLS frame of 8 hot functions, among them property reads, `==` and turning a value into a string, is taken only around the statements that need it. Renders take 1.6% less time with PGO and 1.1% less without, and 2.6% less on darwin/arm64 (Apple M4, with PGO).
- The interpreter writes out its 46 calls of `set_value`: 1.4% less. With the frames above, 2.8% less than v0.26.15 with PGO and 2.9% less without.
- The interpreter counts its interrupt counter down in its own body instead of calling a function on every loop pass: 0.6% less with PGO.
- Each runtime keeps freed blocks of 16 to 512 bytes in a cache of its own, given back at each GC: 0.9% less with PGO.
- A VM keeps the shapes of plain objects, so the next object literal with the same keys makes no shape: object literals with the same keys take 30% less time to build, and a render 3.6% less without PGO. With the two changes above, 4.5% less than without the three. Two VMs on the two hyperthreads of one core render 2.9% more pages a second.

## [v0.26.15] - 2026-10-19

### Changed

- A panic in the engine itself, or a Go function that called `runtime.Goexit` (as `t.Fatal` does), leaves the VM good only for `Close`: every later call into javascript panics. A Go function given to javascript that panics is still recovered as before.
- Outside linux each VM keeps a block of up to 1 MiB of TLS stack until it is closed. On windows that block counts in full against the commit limit.
- A PGO profile collected with an earlier release matches far fewer call sites: collect it again.

### Performance

- Engine functions free their TLS frame before each return instead of in a `defer`, so Go inlines them: about 10% less time, with or without PGO.
- A regexp that starts with one character or one class of characters, such as `/["&<>]/`, starts where that character first occurs: about 3% less.
- `===` and the names of keys take a TLS frame only on their rare paths: 0.7% less with PGO, 2% without.
- Outside linux, VMs on several goroutines no longer wait on libc's allocator lock for their TLS stack: 10 VMs on an Apple M4 render 1.4 to 1.55 times as many pages a second.
- Together, renders take 0.859 of the time of v0.26.14 with a new PGO profile, and 0.843 without PGO.

## [v0.26.14] - 2026-10-19

### Fixed

- `NewVM` no longer leaks the memory it allocated when it cannot make a context.

### Changed

- `internal/fixlib` stops wherever an upstream change could make it fix the generated code wrong. Programs see no change.

## [v0.26.13] - 2026-10-18

### Performance

- The interpreter loop does less work with the stack: about 12% less time on linux/amd64 and 17% less on darwin/arm64 (Apple M4). Its Go stack frame went from 2608 to 896 bytes on amd64.
- Adding a new property to a plain object, as object literals, spread, class fields and `JSON.parse` do, skips the generic path: about 6% less.
- Together, renders take 0.823 of the time of v0.26.12.

## [v0.26.12] - 2026-10-18

### Changed

- On linux, a recursion through bound functions can hold up to about 1 KB more of TLS blocks per level, and on 32-bit targets a block can take up to 1 KB where it took 256 bytes. The worst case of a recursion to the default limit stays about 20.7 MB on linux/amd64.

### Performance

- A JavaScript call allocates no Go memory and no longer takes libc's allocator lock: about 11% less time on linux/amd64 and 14% less on darwin/arm64, and loading bytecode into a VM about 11% less on linux. Several VMs on several cores no longer hold each other back on that lock.

## [v0.26.11] - 2026-10-18

### Fixed

- `RegisterFunc` and `RegisterHostFunc` no longer abort the process when the global object already has the name. The function replaces a configurable property, and the call returns an error for a property that is not configurable, a global object that is not extensible, or a name with a NUL byte.

## [v0.26.10] - 2026-10-18

### Fixed

- `VM.Interrupt` is safe from any goroutine while the VM closes and after `Close`, where it does nothing.
- A Go string that is not valid UTF-8 becomes a JS string with U+FFFD for each byte that does not start a valid sequence, as Go decodes it. The engine used to drop the bytes after an invalid one, ASCII included.

## [v0.26.9] - 2026-10-18

### Fixed

- `intl` names daylight saving time as ICU does on every system. On Linux, Europe/Dublin and a few other zones swapped their summer and winter names.

## [v0.26.8] - 2026-10-18

### Fixed

- The module builds on linux and freebsd again, which v0.26.6 and v0.26.7 broke.
- A Go string made from a JS string is valid UTF-8: a lone surrogate, as left by cutting a string inside an emoji, becomes U+FFFD, as Node writes it.

## [v0.26.7] - 2026-10-17

Does not build on linux and freebsd. Use v0.26.8 or later.

### Added

- `*Error` has `Uncatchable`, `OutOfMemory` and `Interrupted`, which say whether the call ended with an exception no script could catch, and why. What a script throws is never uncatchable, whatever its name and message.

### Fixed

- The eval timeout runs on the monotonic clock, so setting the system clock no longer moves it, and a timeout too long for the clock never fires instead of firing at once.
- Taking an uncatchable error runs no page code, such as an `Error.prototype.toString` the page replaced.

## [v0.26.6] - 2026-10-17

Does not build on linux and freebsd. Use v0.26.8 or later.

### Changed

- An interrupt or the eval timeout can no longer be caught through a promise, an iterator or a Go function: the call returns it, as for a script without promises. `ExecutePendingJobs` stops at the job that failed with it and leaves the rest queued.
- Running out of memory is uncatchable too, and the call returns `*Error{Name: "InternalError", Message: "out of memory"}`. Page code can no longer fill memory and catch the failure.
- A panic in a Go function that javascript called no longer unwinds through the engine. The script ends as for an interrupt, and the Go code that called javascript panics again with the same value.
- Recursing without end throws `RangeError: Maximum call stack size exceeded`, which a script can catch. It used to kill the process with a Go stack overflow. A new runtime starts with a stack limit of 10000 slots.

### Fixed

- Compiling a script, a function body, `eval` code or a regexp with little memory left fails with the out of memory error, where the engine could crash, report a `SyntaxError` the script does not have, or return bytecode cut short.

## [v0.26.5] - 2026-10-17

### Fixed

- A string that cannot be converted to Go, for want of memory, comes back as the error instead of `""`, from `Eval`, `Call`, the JSON of an `Object`, `Value.MarshalJSON` and the arguments of Go functions.
- A BigInt whose `toString` throws reports that exception, and reading the name and message of a thrown value leaves no exception behind.

## [v0.26.4] - 2026-10-16

### Fixed

- `Intl.NumberFormat` reads fraction digits as ECMA-402 has since 2021: a currency with `maximumFractionDigits: 0`, or fraction digits next to significant digits, no longer throws a `RangeError`.

## [v0.26.3] - 2026-10-16

### Changed

- A script's local time is UTC in every VM, whatever zone the process has, without changing `TZ` for the rest of the process. `getTimezoneOffset()` is 0, the local getters and setters work in UTC, and `toString` prints `GMT+0000`. To show another zone, pass a `timeZone` to `Intl`.

## [v0.26.2] - 2026-10-16

### Added

- Package `intl` formats dates, numbers and plurals in every locale Chromium has, from CLDR 48.2, and adds `Intl.PluralRules`. Its output is compared with Chromium 156 on about 60,000 expressions. A program that uses it is 1.7 MB larger.

### Changed

- `intl` defaults to UTC instead of the host's zone, and installs itself the first time a script uses `Intl` or a `toLocale*` method, from bytecode compiled once per process. `Register` takes 0.09 ms and 9 KB of the VM's memory, against 0.85 ms and 46 KB before.

### Fixed

- A Go function that evaluates more javascript on its own VM no longer sets the eval timeout again or drops an `Interrupt` asked for meanwhile.
- An `intl` install cut short by running out of memory, a stack overflow or an interrupt is rolled back, and the next use installs again.

## [v0.26.1] - 2026-10-15

### Changed

- An error that something keeps holds the bytecode of the functions on its stack, and their realm, until its `stack` is read.

### Performance

- Making an error no longer formats its `stack`: the text is made the first time `stack` is read, with the same result. A server render of a 100-item React component with the development build of React took 11.2 ms instead of 42.1 ms on darwin/arm64.

## [v0.26.0] - 2026-10-14

The first release of the fork, from modernc.org/quickjs v0.25.0 and libquickjs v0.13.1.

### Changed

- **Breaking:** the module is `github.com/liustack/quickjs`, and the engine is its package `github.com/liustack/quickjs/lib`, so it no longer depends on `modernc.org/libquickjs`. It needs Go 1.27.

### Fixed

- A VM that runs out of memory inside `JSON.parse`, or anything else that converts a string with non-ASCII characters to UTF-8, no longer leaks that string.
- `VM.Call` and `VM.CallValue` report the exception thrown while evaluating the function expression instead of `cannot call a non-function`.

[Unreleased]: https://github.com/liustack/quickjs/compare/v0.26.17...main
[v0.26.17]: https://github.com/liustack/quickjs/compare/v0.26.16...v0.26.17
[v0.26.16]: https://github.com/liustack/quickjs/compare/v0.26.15...v0.26.16
[v0.26.15]: https://github.com/liustack/quickjs/compare/v0.26.14...v0.26.15
[v0.26.14]: https://github.com/liustack/quickjs/compare/v0.26.13...v0.26.14
[v0.26.13]: https://github.com/liustack/quickjs/compare/v0.26.12...v0.26.13
[v0.26.12]: https://github.com/liustack/quickjs/compare/v0.26.11...v0.26.12
[v0.26.11]: https://github.com/liustack/quickjs/compare/v0.26.10...v0.26.11
[v0.26.10]: https://github.com/liustack/quickjs/compare/v0.26.9...v0.26.10
[v0.26.9]: https://github.com/liustack/quickjs/compare/v0.26.8...v0.26.9
[v0.26.8]: https://github.com/liustack/quickjs/compare/v0.26.7...v0.26.8
[v0.26.7]: https://github.com/liustack/quickjs/compare/v0.26.6...v0.26.7
[v0.26.6]: https://github.com/liustack/quickjs/compare/v0.26.5...v0.26.6
[v0.26.5]: https://github.com/liustack/quickjs/compare/v0.26.4...v0.26.5
[v0.26.4]: https://github.com/liustack/quickjs/compare/v0.26.3...v0.26.4
[v0.26.3]: https://github.com/liustack/quickjs/compare/v0.26.2...v0.26.3
[v0.26.2]: https://github.com/liustack/quickjs/compare/v0.26.1...v0.26.2
[v0.26.1]: https://github.com/liustack/quickjs/compare/v0.26.0...v0.26.1
[v0.26.0]: https://github.com/liustack/quickjs/tree/v0.26.0
