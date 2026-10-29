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

`github.com/liustack/slowjs` is a pure Go, cgo-free embeddable JavaScript engine (ES2023), named `github.com/liustack/quickjs` until v0.27.x. The engine is QuickJS in C, in `engine/`, compiled by LLVM to Go assembly that is checked in, so a user of the module needs no C compiler and no LLVM. It is QuickJS release 2026-06-04 (commit `3d5e064`) with the changes that [changes](changes/README.md) lists. Until v0.28 the engine was QuickJS translated to Go by ccgo, which the C engine replaced. [upstream.md](upstream.md) says where the code comes from.

There is no cgo and no shared library: the module cross-compiles as ordinary Go, for the two targets the engine runs on. `grep -rn 'import "C"'` finds nothing. Do not bring cgo in.

## The layers

1. **`engine/`**: the C of QuickJS with slowjs's changes, built with `-ffreestanding -nostdinc` against `engine/libc`, its own small libc. It asks its host only for the callbacks of `engine/host.h`: memory, clocks, the interrupt poll and a few requests (`host_call`). It has no global, no lock and no system call. `engine/test/` is a native host that runs it, for the tests that need no Go.
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
| `engine_unsupported.go` | On any other target, imports `internal/unsupported`, whose error, naming the two targets, is the only one the build reports |

The shared files hold the API: `vm.go` (making, setting and closing a VM), `eval.go`, `call.go`, `convert.go` (Go values to javascript and back), `errors.go`, `register.go` (`RegisterFunc`), `value.go`, `builtin.go`, `jsstring.go`, `promise.go`, `sharedbytecode.go`, `memory.go`, `asynccontext.go`, `vmclock.go` and `etc.go`. `TestEngineDeps` (`enginedeps_test.go`) checks that the package links `internal/cengine` and its Go host.

## Folders

| Path | What it holds |
| --- | --- |
| `quickjs.go` and the other `.go` files of the root | The Go API, on the engine layer (see [The engine layer](#the-engine-layer)) |
| `engine/` | The C tree of the engine: QuickJS at the pinned commit with slowjs's C changes. No Go file, and no Go package may hold a C file (`layout_test.go`) |
| `engine/libc/` | The small libc of the C tree: musl's memory, string and math functions, a `snprintf` and the headers, so that the engine builds with `-ffreestanding -nostdinc` (see its [README](../engine/libc/README.md)) |
| `engine/host.h` | The callbacks the engine asks of its host. The native test host and the Go host implement them |
| `engine/test/` | The native test host: the C tree built by plain clang with its small libc, and test262, the recorded outputs and the checks in C run on it, also under AddressSanitizer and UndefinedBehaviorSanitizer (see [testing.md](testing.md#the-c-tree-and-its-native-test-host)). Never part of a Go build |
| `internal/cengine/` | The generated engine (`.s`, `engine_decl_<arch>.go`, `engine_keep_<arch>.go` for amd64 and arm64, checked in) and the host files next to it: the callbacks the assembly calls, a VM with Eval, RunGC, ReleaseMemory and Close, and `cmd/jsrun`, `cmd/gcpause`, `cmd/interrupt`, which measure it |
| `internal/goasm/gen/` | The generator that compiles `engine/` to Go assembly with LLVM 22.1.8, its pass and its checks (see its [README](../internal/goasm/gen/README.md)). `internal/goasm/cmd/xform` moves the code to time a build at several positions |
| `internal/hostmem/` | The memory the Go host gives the engine, mapped outside the Go heap: the chunks of a VM's heap and what they add up to, and its shadow stack with a guard page |
| `internal/gohost/` | The Go side of the engine's callbacks: VMs by id and by shadow stack, their memory on `internal/hostmem`, clocks, seed, interrupt and the engine's own `host_call` requests. Runs without the engine |
| `internal/idtable/` | The table of ids that the engine passes back to Go |
| `internal/unsupported/` | The error of a build for a target the engine does not run on |
| `internal/sjdev/` | `go tool sjdev bench`: builds a program at several code positions, times them in turn and counts instructions |
| `intl/` | Package `github.com/liustack/slowjs/intl`, `Intl` for a VM |
| `internal/gointl/internal/qjspack/gen/` | The shared Intl data packs, selected with Chromium's ICU inventory |
| `intl/testdata/chromium/` | What Chromium answers, for the golden test, and `record.cjs`, which records it |
| `temporal/` | Package `github.com/liustack/slowjs/temporal`, `Temporal` for a VM |
| `temporal/testdata/` | The expressions compared with Chromium and its answers, and the program the link test builds |
| `internal/gointl/` | A copy of go-intl (pure Go ECMA-402 and Temporal), behind every `intl` constructor and behind `temporal` (see [upgrading.md](upgrading.md#updating-go-intl-internalgointl)) |
| `internal/intlhook/` | What package `temporal` takes from package `intl`: the formatting of its `toLocaleString` and a VM's default zone |
| `fmt/` | Package `github.com/liustack/slowjs/fmt` |
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

`SetEvalTimeout` sets the duration. `Interrupt()` sets the flag atomically, from any goroutine. It reaches the flag through `VM.intr`, which holds the host of the VM in Go memory and orders it with `Close`. `Close` clears only the embedded `vmState`, never `intr`, so `Interrupt` is safe from any goroutine during and after `Close`. Keep new VM fields in `vmState`. When you add a method that runs javascript, add the `enter` and `leave` pair, or the timeout will not apply to it.

## Packages next to the engine

- **`intl/`**: All `Intl` constructors on `internal/gointl`, and the `toLocale*` methods and `localeCompare`, for a VM that calls `Register(vm)`. QuickJS has no `Intl`: neither Bellard's engine nor quickjs-ng implements ECMA-402, so it is not a build option, and a page that reaches for `Intl` without `Register` gets a `ReferenceError`. The locales are those Chromium has, from CLDR 48.2, and the output is compared with Chromium (see [time-and-intl.md](changes/time-and-intl.md#intl-in-every-locale-chromium-has)). The javascript side is `intl.js` and `stub.js` (embedded with `go:embed`), and every formatting decision is made in Go. `intl.js` is a function of the `Builtin`s in `bridge.go`, which the install in `stub.js` calls it with, so none of them is ever on `globalThis`. A constructor resolves its locales and options into an id in the process-wide `registry.go` (bounded, shared by all VMs). Every later call passes the id, and a Builtin answers `undefined` for an id the registry dropped, on which `intl.js` resolves the same arguments again. Values cross as strings, numbers and booleans, lists as arrays (`VM.NewArray`), and `resolvedOptions()` as JSON. `stub.js` takes the built-ins both scripts use at `Register` time, before any page code runs, and `intl.js` uses only those, null-prototype descriptors and arrays, and a private field for internal slots (`intl/testdata/chromium/tamper.js` lists what a page may change). The comment at the top of `bridge.go` says how to add a constructor. `intl/memgrind_test.go`, which `make leak` runs, checks the handling of `Value`s. `intl/golden_test.go` is the check that matters after touching a formatter: the spec says what the options mean, but only the data knows details such as when a minute grows a leading zero.
- **`temporal/`**: `Temporal`, for a VM that calls `Register(vm)`, built as `intl/` is: stand-ins in `stub.js`, then on first use `temporal.js`, a function of one `Builtin` in `bridge.go` whose first argument names the operation (the `opNames` list). go-intl's temporal package does every computation. An object keeps its internal slots as numbers and strings in a private field, out of every page's reach, and passes them to the Builtin, which rebuilds the go-intl value for the call and answers with an array of numbers (the groups are in the comment at the top of `values.go`), or with an error's name and message for `temporal.js` to throw. `toLocaleString` goes to package `intl` through `internal/intlhook`, which `intl` fills when it is initialized, so `intl` does not import `temporal`. Both shims receive a private object rooted in a private field of the global object. It shares Temporal slot and Intl locale readers without exposing them to page code. Each VM's clock is kept in a table keyed by the VM, which `VM.OnClose` empties. `link_test.go` checks which of go-intl's packs a program with Temporal links: `tz.bin` and those of `intl`, the date and zone-name packs for formatting, and no host-zone guessing pack (see [temporal.md](changes/temporal.md)).
- **`internal/gointl/`**: a copy of go-intl (pure Go ECMA-402 and Temporal) at the commit in `internal/gointl/UPSTREAM`, which `intl` uses for every constructor, including Date and Temporal formatting and date ranges, and `temporal` for all of Temporal. Only the `qjs*` files and `UPSTREAM` are ours. Edit go-intl's files only through `qjs.patch` and `qjs-sync.sh` (see [Updating go-intl](upgrading.md#updating-go-intl-internalgointl)). Its data is the packs in `qjsdata/`, written by `internal/qjspack/gen`, not go-intl's `data.pack`. `make gointl` runs the checks that need build tags, and checks that the packs are current.
- **`fmt/`**: package `github.com/liustack/slowjs/fmt`, part of this module (it has no `go.mod` of its own). It gives a VM Go's `fmt.Printf`, `Println`, `Sprintf` and `Sprintln` with `Register(obj Value)`, with lower-case names in javascript (`fmt.println`).
- **`compare/`**: a module of its own (`github.com/liustack/slowjs/compare`) that runs the micro benchmarks of QuickJS in `compare/testdata/` on this engine, on `github.com/dop251/goja` and on `github.com/fastschema/qjs`, leaving out the 6 that goja or qjs fails. `make benchmark` runs there. It is a module of its own so that its dependencies (goja, qjs, wazero, regexp2 and so on) stay out of the library's `go.mod`, where every user would see them. `compare/go.mod` has `replace github.com/liustack/slowjs => ../`, so the benchmarks always build the work tree and `go mod tidy` takes the versions of the rest from the root `go.mod`: never copy those versions by hand. Its indirect requirements are only lower bounds, and a newer version in the root `go.mod` stops the benchmarks with `updates to go.mod needed`, so the `benchmark` target runs `go mod tidy` first. The `github.com/liustack/slowjs` require line is not resolved (the replace wins) and nothing reads it.

## Platforms

The engine runs on linux/amd64 and darwin/arm64. Other targets are not supported: the module has no generated engine for them, so a build for one stops with an error that names the two targets (`internal/unsupported`). The Go assembly was made and checked with Go 1.27.2 (see [upgrading.md](upgrading.md#moving-to-a-newer-go)). A change must keep `make build_all_targets` passing on both (see [testing.md](testing.md#every-target)).
