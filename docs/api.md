---
summary: 'What the Go API has beyond NewVM, Eval, Call and Value: each function and type, what it is for, what it gains in tifa, and the release it came in'
read_when:
  - Looking for a Go function of the API that is not Eval or Call
  - Adding a function or type to the Go API (add it here, in both languages)
  - Asking which release added a function
---

# The Go API beyond Eval and Call

English · [简体中文](api.zh-CN.md)

The API is documented at [pkg.go.dev](https://pkg.go.dev/github.com/liustack/quickjs). Every program starts with `NewVM`, `VM.Eval`, `VM.Call` and `Value`. Besides them, it has the functions below. Each links to the doc that says why it is safe and how it was measured. Releases v0.28.0 to v0.29.11 were published as `github.com/liustack/slowjs`.

## Calling Go from JavaScript

- **`NewBuiltin(name, length, fn)` and `VM.NewBuiltinFunction(b)`** (v0.26.17): a Go function JavaScript calls as it calls `Math.max`, with no reflection. `fn` reads its arguments from `Args` (`Number`, `String`, `Bool`, `Value`, `This`, `VM`, and `Latin1` or `UTF16` for the characters of a string as the engine holds them), which convert nothing, and returns a `Value`. A call that adds two numbers takes about 55 ns on an Apple M4, against about 470 ns through `RegisterHostFunc`. Make a Builtin once per process: VMs on several goroutines call it at once. An `*Error` it returns is thrown as the constructor its `Name` says (`Error`, `TypeError`, `RangeError`, `SyntaxError`, `ReferenceError` or `InternalError`), any other error as a `TypeError`. [More](changes/go-api.md#builtin-a-go-function-called-as-a-built-in)
- **`Args.StringLen(i)`** (Unreleased): the length of a string argument, in UTF-16 code units as JavaScript counts it, read without converting or joining the string, so a Builtin can refuse a string too long for it before it reads any of it. It takes the same time for a string of any length and allocates nothing. [More](changes/go-api.md#argsstringlen-reads-the-length-of-a-string-in-place)
- **`VM.NewArray(vals...)`** (v0.26.17): an array of values made in Go, made as an array literal makes one, so a setter a page put on `Array.prototype` never runs. [More](changes/go-api.md#newarray)
- **`GetAsyncContext` and `SetAsyncContext`** (v0.28.5): two Builtins that read and set an object promise jobs carry. A `then` or an `await` runs with the one set when it was registered, so an `AsyncLocalStorage` written on them finds its store after `await` as in Node. A VM that never sets one runs promises as before. [More](changes/go-api.md#the-async-context-of-promise-jobs)

## Errors

- **`*Error` fields `Uncatchable`, `OutOfMemory` and `Interrupted`** (v0.26.7): why a call ended, which no script can fake. [More](changes/go-api.md#error-says-why-a-call-ended)
- **`Error()` of an `*Error` made in Go** (v0.26.17) is `Name: Message`, as `Error.prototype.toString` writes it, where it was empty. An error taken from JavaScript keeps the engine's text. [More](changes/go-api.md#error-of-an-error-made-in-go)

## Memory and GC

- **`VM.RunGC()`, `VM.MemorySize()` and `VM.GCThreshold()`** (v0.26.17): run a GC now, read the memory a VM takes and the threshold of its next GC, so a program can collect between two calls into JavaScript and set the threshold for the next one. [More](changes/go-api.md#rungc-memorysize-and-gcthreshold)
- **`VM.MemoryUsage()` and `VM.ReleaseMemory(keep)`** (v0.26.17): see what a VM holds, by objects, strings, functions and arrays, and give memory it keeps free back to the system. [More](changes/go-api.md#memoryusage-and-releasememory)
- **`VM.RunYoungGC()`** (v0.27.3): collect only the objects made since the last GC, so a server that runs a young GC after each render and a full one now and then spends on each GC about as long as the garbage of one render. [More](changes/young-collection.md)
- **`VM.OnClose(f)`** (v0.27.0): run `f` when the VM is closed, so a package that keeps something per VM in a table of its own can drop it then. [More](changes/go-api.md#onclose)
- **`NewSharedBytecode(bytecode)` and `VM.EvalSharedBytecode(b)`** (v0.28.1): VMs that load the same bytecode keep the source text and line tables of its functions in one copy, so each VM of tifa's example shop holds 2.36 MiB less, with the same `toString` and stacks. [More](changes/go-api.md#sharedbytecode)
- **`VM.SetShapeCache(n)` and `DefaultShapeCache`** (v0.26.16): how many shapes of plain objects (a shape is the engine's record of which keys an object has, in which order) a VM keeps for the next object with the same keys. [More](changes/go-api.md#setshapecache-and-defaultshapecache)

## Source and time

- **`VM.SetStrip(s)`** with `type Strip` (v0.26.17): `StripSource` compiles functions without their source, so their `toString` shows `[native code]`, and `StripDebug` leaves out line and column numbers. [More](changes/go-api.md#setstrip)
- **`VM.SetClock(now)` and `VM.Now()`** (v0.28.2): set and read the wall clock of a VM, which Date, Intl and Temporal all read. nil restores the live clock. [More](changes/time-and-intl.md#one-wall-clock-per-vm)
- **Package `temporal`** (v0.27.0): `temporal.Register(vm)` adds `Temporal`, every type and method V8 ships, computed in Go by go-intl with ICU's zones for tz 2026e built in, so the host's zoneinfo is never read. `Temporal.Now` reads the VM clock, and `temporal.Clock` overrides it for `Temporal.Now` alone. The linked doc has the binary size it adds. [More](changes/temporal.md)

## Rendering React faster

Each of these defines a function on the global object, written in C inside the engine, that does the work of a hot function of React. Nothing is on the global object until a host calls them.

- **`VM.RegisterEscapeText(name)`** (v0.29.2): escapes text for HTML as React DOM's string renderer does (`&`, `<`, `>`, `"` and `'` become entities, and the argument becomes a string as in `"" + text`). With it in tifa, the pages of its example shop render in 0.974 of the time and its bench pages in 0.938. [More](changes/go-api.md#registerescapetext)
- **`VM.RegisterCopyProps(name)`** (v0.29.3): does the work of the loop with which React's `createElement` and `cloneElement` copy the keys of their config into the props of an element. It copies only where its result and the loop's cannot differ, and answers `false` for any other config, which the script gives to the loop. With it in tifa, the pages of its example shop render in 0.988 and 0.989 of the time and its MUI pages in 0.980. [More](changes/go-api.md#registercopyprops)
- **`VM.RegisterStartTag(name)` and `VM.RegisterEndTag(name)`** (v0.29.10): write the start tag of an element, with its attributes, style and text, as React DOM's server renderer does in `pushStartGenericElement`, and give its end tag as `endChunkForTag` does. The start tag function writes only where React's functions could write nothing else, and returns the caller's `decline` object for any other call, which the script gives to React. On the start and end tags of one render of tifa's example pages they run about a fifth of the instructions React's functions take. [More](changes/go-api.md#registerstarttag-and-registerendtag)
