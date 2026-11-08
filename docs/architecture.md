---
summary: 'How the module is built: the layers from the Go API down to the C engine, the engine layer of the Go API, what each folder holds, the core types and their rules, the value form, how Go functions are called from javascript, how the eval timeout and Interrupt work, and which platforms the engine runs on'
read_when:
  - Starting work in this repository for the first time
  - Changing the Go API in the root package, calling the engine from it, or adding a method that runs javascript
  - Looking for where a piece of the engine, a change to it or a tool lives
  - Touching how a value is encoded, or reading a JSValue by hand
  - Changing the intl, temporal, fmt or compare packages, or internal/gointl or internal/intlhook
---

# Architecture

English · [简体中文](architecture.zh-CN.md)

`github.com/liustack/quickjs` is a pure Go, cgo-free embeddable JavaScript engine (ES2023), named `github.com/liustack/slowjs` from v0.28.0 to v0.29.11. The engine is QuickJS in C, in `engine/`, compiled by LLVM to Go assembly that is checked in, so a user of the module needs no C compiler and no LLVM. It is QuickJS release 2026-06-04 (commit `3d5e064`) with the changes that [changes](changes/README.md) lists. Until v0.28 the engine was QuickJS translated to Go by ccgo, which the C engine replaced. [upstream.md](upstream.md) says where the code comes from.

There is no cgo and no shared library: the module cross-compiles as ordinary Go, for the targets the engine runs on. `grep -rn 'import "C"'` finds nothing. Do not bring cgo in.

## The layers

1. **`engine/`**: the C of QuickJS with the changes of liustack/quickjs, built with `-ffreestanding -nostdinc` against `engine/libc`, its own small libc. It asks its host only for the callbacks of `engine/host.h`: memory, clocks, the interrupt poll and a few requests (`host_call`). It has no global, no lock and no system call. `engine/test/` is a native host that runs it, for the tests that need no Go.
2. **`internal/cengine/`**: the engine as Go assembly, generated from `engine/` by `internal/goasm/gen` and checked in (`make gen`, `make check-gen`). The Go files next to it are the host of the callbacks and a VM with the calls the root package needs.
3. **The Go host**: `internal/gohost` answers the callbacks of a VM, `internal/hostmem` maps its memory outside Go's heap (chunks, a shadow stack with a guard page), and `internal/idtable` holds the ids that the engine passes back to Go.
4. **The Go API** (`quickjs.go` and the files next to it): what users import.

## The engine layer

The Go API calls the engine only through the files below. The rest of the package names no `cengine.*`.

| File | What it holds |
| --- | --- |
| `engine.go` | The value type `jsValue` and its helpers (`tag`, `newInt32`, `valueAt`, `dupValue`, `stringChars` and the like), the tags, atoms and flags the package uses, the runtime and context of a VM (`engineState`, which `vmState` embeds), and one method per C function the package calls, named after it (`jsEval`, `jsFreeValue`, `jsGetProperty` and so on). A call into the engine takes the shadow pointer, the first free byte of the VM's shadow stack: `m.sp`, which a Go function that javascript called moves above the frames of the call |
| `entry.go` | Going into the engine and out of it (`enter`, `leave`), the eval timeout and `Interrupt`, which the polls of the host carry out, and what a panic leaves behind |
| `hostcall.go` | The Go functions the engine calls (`callGo`, `callBuiltin`, the class of `Then` callbacks) and the function objects that call them |
| `enginememory.go` | `MemoryUsage`, `ReleaseMemory` and the memory of `SharedBytecode`, which is mapped read-only outside every VM |
| `engine_unsupported.go` | On any other target, imports `internal/unsupported`, whose error, naming the targets, is the only one the build reports |

The shared files hold the API: `vm.go` (making, setting and closing a VM), `eval.go`, `call.go`, `convert.go` (Go values to javascript and back), `errors.go`, `register.go` (`RegisterFunc`), `value.go`, `builtin.go`, `jsstring.go`, `promise.go`, `sharedbytecode.go`, `memory.go`, `asynccontext.go`, `vmclock.go` and `etc.go`. `TestEngineDeps` (`enginedeps_test.go`) checks that the package links `internal/cengine` and its Go host.

## Folders

| Path | What it holds |
| --- | --- |
| `quickjs.go` and the other `.go` files of the root | The Go API, on the engine layer (see [The engine layer](#the-engine-layer)) |
| `engine/` | The C tree of the engine: QuickJS at the pinned commit with the C changes of liustack/quickjs. No Go file, and no Go package may hold a C file (`layout_test.go`) |
| `engine/libc/` | The small libc of the C tree: musl's memory, string and math functions, a `snprintf` and the headers, so that the engine builds with `-ffreestanding -nostdinc` (see its [README](../engine/libc/README.md)) |
| `engine/host.h` | The callbacks the engine asks of its host. The native test host and the Go host implement them |
| `engine/test/` | The native test host: the C tree built by plain clang with its small libc, and test262, the recorded outputs and the checks in C run on it, also under AddressSanitizer and UndefinedBehaviorSanitizer (see [testing.md](testing.md#the-c-tree-and-its-native-test-host)). Never part of a Go build |
| `internal/cengine/` | The generated engine (`.s`, `engine_decl_<arch>.go`, `engine_keep_<arch>.go` for amd64 and arm64, checked in) and the host files next to it: the callbacks the assembly calls, a VM with Eval, RunGC, ReleaseMemory and Close, and `cmd/jsrun`, `cmd/gcpause`, `cmd/interrupt`, which measure it, and the tripwire pair `goasm_tripwire.<arch>.s` and `.go`, a function that is never linked and assembles only when the build keeps the instructions as written |
| `internal/goasm/gen/` | The generator that compiles `engine/` to Go assembly with LLVM 22.1.8, its pass and its checks (see its [README](../internal/goasm/gen/README.md)). `internal/goasm/cmd/xform` moves the code to time a build at several positions |
| `internal/hostmem/` | The memory the Go host gives the engine, mapped outside the Go heap: the chunks of a VM's heap and what they add up to, and its shadow stack with a guard page |
| `internal/gohost/` | The Go side of the engine's callbacks: VMs by id and by shadow stack, their memory on `internal/hostmem`, clocks, seed, interrupt and the engine's own `host_call` requests. Runs without the engine |
| `internal/idtable/` | The table of ids that the engine passes back to Go |
| `internal/unsupported/` | The error of a build for a target the engine does not run on |
| `internal/goversion/` | The error of a build with a Go newer than the one the engine was checked with, which `internal/cengine` imports |
| `internal/faulttest/` | For the tests that make a memory fault on purpose: `Room`, which a test calls right before a fault in Go code so that windows has the room to write the state of the CPU, and `Unsafe`, for the faults in Go code that `Room` cannot come right before (see [c-engine.md](changes/c-engine.md#room-for-a-fault-on-windows)) |
| `internal/measure/` | `go tool measure bench`: builds a program at several code positions, times them in turn and counts instructions |
| `intl/` | Package `github.com/liustack/quickjs/intl`, `Intl` for a VM |
| `internal/gointl/internal/qjspack/gen/` | The shared Intl data packs, selected with Chromium's ICU inventory |
| `intl/testdata/chromium/` | What Chromium answers, for the golden test, and `record.cjs`, which records it |
| `temporal/` | Package `github.com/liustack/quickjs/temporal`, `Temporal` for a VM |
| `temporal/testdata/` | The expressions compared with Chromium and its answers, and the program the link test builds |
| `internal/gointl/` | A copy of go-intl (pure Go ECMA-402 and Temporal), behind every `intl` constructor and behind `temporal` (see [upgrading.md](upgrading.md#updating-go-intl-internalgointl)) |
| `internal/intlhook/` | What package `temporal` takes from package `intl`: the formatting of its `toLocaleString` and a VM's default zone |
| `internal/vmhook/` | How the Go functions of `intl`, `temporal` and `fmt` keep the limits of the VM that called them: `Stop` for the eval timeout and `Interrupt`, `Room` and `OutOfMemory` for the memory limit, which package `quickjs` sets (`vmhook.go`) |
| `fmt/` | Package `github.com/liustack/quickjs/fmt` |
| `compare/` | A module of its own, kept from upstream, that runs QuickJS's micro benchmarks on this engine, goja and fastschema/qjs |
| `internal/arewefastyet/` | The V8 benchmark suite (v8-v7) that compare/engines runs |
| `scripts/docs-list.sh` | The index of `docs/` |

## Core types

- `VM`, the main handle: a JS realm and its runtime in the engine, with the memory the Go host gave it. Made by `NewVM()`, released by `Close()`. Not safe for concurrent use by several goroutines.
- `Value`: wraps a reference-counted `jsValue`. `Dup()` and `Free()` must be strictly paired and properly nested (the type's doc says how this breaks across goroutines and channels). Getting it wrong causes leaks or use after free, which the sanitizers of the native test host and the checks of `internal/cengine` catch.
- `Object`, `Atom` and `HostFunc` (`func(args []any) (any, error)`).
- `Builtin`, a Go function javascript calls as a built-in, and `Args`, the arguments of its call (see [Go functions called from javascript](#go-functions-called-from-javascript)).

## Values

`JSValue` is a struct of two words, the union and the tag (`jsValue` in `engine.go`), and the engine returns it in two registers. Every supported target is 64-bit and little-endian. `engine.go` defines the helpers that read and build it: `newInt32`, `newFloat`, `newBool`, `tag`, `isException`, `jsvToInt32`, `jsvToFloat64` and the like. An int32 or bool payload lives in the low 32 bits of the union, and a float64 or pointer payload fills it. The reference count of a value that has one is 4 bytes before the address it holds (`cengine.RefCountOffset`), and a string's length, width and characters are read in place from the layout table the engine exports (`js_host_layout`, see [c-engine.md](changes/c-engine.md)).

## Go functions called from javascript

The Go functions of `RegisterFunc`, `RegisterHostFunc` and `Then` are kept by an id (`keepFunc`) that the function object calling them holds. The id is a handle of the host of the VM (`gohost.VM.Keep`), which the engine gives back when it frees the function object, and `Close` drops the rest. The engine calls Go through `host_call`, which `internal/cengine` sends to `hostCall`.

A `Builtin` (`builtin.go`) is the fast path: made once per process by `NewBuiltin`, kept in the copy-on-write `builtins.list` indexed by its magic, and turned into a function object per VM by `VM.NewBuiltinFunction`. `callBuiltin` finds its VM from the context's opaque, the VM's id in `vms`, a table read without a lock. It keeps the panic and uncatchable handling of `callGo`: change one, change the other. `VM.NewArray` builds an array with `JS_DefinePropertyValueUint32`, so no setter on `Array.prototype` runs: a Builtin returns a list with it.

## The eval timeout and Interrupt

Every method that runs javascript (`Eval`, `EvalValue`, `Call`, `CallValue`, `ExecutePendingJobs` and the others) calls `m.enter()` and `defer m.leave(&err)` before it runs javascript. `enter` counts how deep the calls go, and only the outermost call arms the deadline from `m.timeout` and clears the interrupt flag, in the host. So a Go function that evaluates more javascript on the same VM stays inside the outer call's time and keeps a pending `Interrupt()`. The engine reads the flag where the generated code polls and at the places of the C that read it ([c-engine.md](changes/c-engine.md#polls-and-interrupts)). The deadline is on the monotonic clock (see [uncatchable.md](changes/uncatchable.md#the-eval-timeout-runs-on-the-monotonic-clock)).

`SetEvalTimeout` sets the duration. `Interrupt()` sets the flag atomically, from any goroutine. It reaches the flag through `VM.intr`, which holds the host of the VM in Go memory and orders it with `Close`. `Close` clears only the embedded `vmState`, never `intr`, so `Interrupt` is safe from any goroutine during and after `Close`. Keep new VM fields in `vmState`. When you add a method that runs javascript, add the `enter` and `leave` pair, or the timeout will not apply to it. That is every method that can reach a getter, a setter, a proxy trap, `toJSON`, `toString` or a replaced built-in method, not only the ones that evaluate source: `GetProperty`, `SetProperty`, `Value.Any`, `Value.MarshalJSON` and `ErrorFromValue` have the pair too, and `outer` (no timeout, no kill) is for calls that cannot run a script. The `*Error` made of an exception reads its fields with `ErrorFromValue`, so an error read outside a call is bounded too.

No poll runs while a Go function that javascript called works in Go, so neither the timeout nor `Interrupt` stops it there, and what it allocates in Go is outside the memory limit. `internal/vmhook` is how a Go function of this module keeps the limits: it calls `vmhook.Stop` every so often in a long loop and returns once it is true, which leaves the uncatchable "interrupted" pending, as a poll would (`gohost.VM.Stopping` is what the poll checks), and it checks `vmhook.Room` before it builds a result whose size the script controls, ending the call with `vmhook.OutOfMemory` when the result would not fit. A function keeps the limits only where it makes these calls. Now they are made by:

- every Builtin of `intl` and `temporal` whose string argument cannot be read;
- the loops of Intl over a list a script passed, of locales or of the items of `Intl.ListFormat`, which check the list every 256 tags or items;
- `Intl.Segmenter`, which finds the segments a batch at a time, as far as they are read, and checks every 256;
- the functions of `fmt`, which check a bound on their output against `vmhook.Room` before Go makes it.

Work in Go that a script can make long and that makes none of these calls still runs to its end, and what it allocates is outside the memory limit: the parse and format of a long number string by `Intl.NumberFormat`, `Intl.Collator` and `localeCompare`, the case mapping of `toLocaleUpperCase` and `toLocaleLowerCase`, the arithmetic of a calendar other than ISO in `Temporal`, the read of one long locale tag or list item, the parse of a long duration by `Intl.DurationFormat`, and the scan of the dictionary for one batch of `Intl.Segmenter`. Go makes the output of `fmt` with no check of the call either, in a time that grows with its bound, but what the output may make it allocate is checked against the room of the VM before it starts ([uncatchable.md](changes/uncatchable.md#the-functions-of-fmt-end-the-call-when-their-output-does-not-fit)).

## Packages next to the engine

- **`intl/`**: All `Intl` constructors, `toLocale*` and `localeCompare`, for a VM that calls `Register(vm)`. QuickJS has no `Intl`, so a page that reaches for it without `Register` gets a `ReferenceError`. The javascript side is `intl.js` and `stub.js`, the decisions are made in Go on `internal/gointl`, and the comment at the top of `bridge.go` says how to add a constructor. After touching a formatter, `intl/golden_test.go` is the check that matters: the data knows details the spec does not. How it works, and how the output is checked against Chromium: [time-and-intl.md](changes/time-and-intl.md).
- **`temporal/`**: `Temporal`, for a VM that calls `Register(vm)`, built as `intl/` is. go-intl's temporal package does every computation, through one Builtin in `bridge.go` whose first argument names the operation (the `opNames` list). See [temporal.md](changes/temporal.md).
- **`internal/gointl/`**: a copy of go-intl (pure Go ECMA-402 and Temporal) at the commit in `internal/gointl/UPSTREAM`, used by `intl` and `temporal`. Only the `qjs*` files and `UPSTREAM` are ours. Edit go-intl's files only through `qjs.patch` and `qjs-sync.sh` (see [Updating go-intl](upgrading.md#updating-go-intl-internalgointl)). `make gointl` runs the checks that need build tags, and checks that the packs in `qjsdata/` are current.
- **`fmt/`**: package `github.com/liustack/quickjs/fmt`, part of this module. It gives a VM Go's `fmt.Printf`, `Println`, `Sprintf` and `Sprint` with `Register(obj Value)`, with lower-case names in javascript (`fmt.println`). Each first bounds the bytes of its output from the format and the arguments (`fmt/bound.go`), and ends the call with the out of memory error when what the output may make Go's fmt allocate, 16 times the bound and a few KiB for the printer that makes it, is more than the VM has left under its memory limit (what Go's runtime sets up for its pools is not counted). Go makes the output with no check of the VM, so neither `Interrupt` nor the eval timeout ends it.
- **`compare/`**: a module of its own (`github.com/liustack/quickjs/compare`) that runs the micro benchmarks of QuickJS in `compare/testdata/` on this engine, on goja and on `github.com/fastschema/qjs`. `make benchmark` runs there. It is a module of its own so that its dependencies stay out of the library's `go.mod`. `compare/go.mod` replaces `github.com/liustack/quickjs` with `../`, so the benchmarks build the work tree and `go mod tidy` (which the `benchmark` target runs first) takes the other versions from the root `go.mod`: never copy them by hand.

## Platforms

The engine runs on linux and darwin, on amd64 and arm64, and on windows/amd64. Every system runs the same generated code of each architecture. Other targets are not supported, so a build for one stops with an error that names the supported ones (`internal/unsupported`). Which Go builds it, and what a newer Go does, is in [upgrading.md](upgrading.md#moving-to-a-newer-go). A change must keep `make build_all_targets` passing on all five (see [testing.md](testing.md#every-target)).

## Making the engine faster

A change made for speed keeps to these rules, which liustack/sqlite keeps too:

1. Nothing private to Go: no `go:linkname` into the runtime, no reliance on how Go uses a register (such as the closure register), no runtime internals. They change between Go releases, and the module builds with every patch release from 1.26.6 on. What the generated code needs of Go's assembler and linker is held by the post-link check, on every release `scripts/go-versions.sh` runs.
2. The engine runs on the goroutine stack, never on a stack of its own as cgo, wazero and purego do, so that a fault in the C ends one VM, not the process.
3. A gain the timing of [measuring.md](measuring.md) cannot tell from the noise of the code position (2% to 3% for a render) is not taken, however clean the change.
4. The same bits on every target: the engine is built with `-ffp-contract=off` and the generator refuses a fused multiply-add ([c-engine.md](changes/c-engine.md#the-small-libc)).
5. What the engine gives stays the same: the HTML of tifa's pages, the 33 known errors of test262 and the call depth.
6. The C is changed mechanically, never rewritten, and each change is written up in [docs/changes](changes/README.md). This is where liustack/sqlite differs: it never edits the C as released, and here the C tree is QuickJS with the changes of docs/changes.

What was tried and dropped, and why: [performance.md](performance.md#what-was-tried-and-not-done), and for the engine before v0.28 [history.md](history.md#what-was-tried-and-not-done-on-the-old-engine).
