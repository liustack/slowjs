---
summary: 'The fork changes to the Go API: what Call reports, strings Go cannot convert, lone surrogates and invalid UTF-8 between Go and javascript, what *Error says about an uncatchable end, RegisterFunc on an existing name, NewVM failing cleanly, SetShapeCache, Builtin, Args.WTF8, NewArray, SetStrip, the text of an Error made in Go, RunGC, MemorySize and GCThreshold, and OnClose'
read_when:
  - Changing quickjs.go, or how strings, errors or host functions cross between Go and javascript
  - A Go string from javascript has replacement characters, or a string from Go reads differently in javascript
  - Telling an interrupt or out of memory from an error a script threw
  - Registering a Go function under a name the global object already has
  - Calling VM.SetShapeCache
  - Giving javascript a Go function that it calls often, or returning a list from Go (Builtin, NewArray)
  - Reading a string argument whose lone surrogates matter, such as one to collate (Args.WTF8)
  - Hiding the source of a script's functions (SetStrip)
  - Running a GC between two calls into javascript, or reading how much memory a VM takes (RunGC, MemorySize, GCThreshold)
  - Keeping something per VM outside it, to drop when the VM is closed (OnClose)
---

# The Go API

Changes an embedder sees in what the methods of `github.com/liustack/quickjs` return. [uncatchable.md](uncatchable.md) has the changes to how a call ends on an interrupt, a timeout or out of memory.

## Call reports the exception of the function expression

Release: v0.26.0.

`VM.Call` and `VM.CallValue` report the exception thrown while evaluating the function expression (for example out of memory, or a name that is not defined) instead of `cannot call a non-function`.

## A string Go cannot convert comes back as the error

Release: v0.26.5.

A string that cannot be converted to Go comes back as the error, not as `""`. Converting needs memory for a string with non-ASCII characters and for one built by concatenation, so near the memory limit `Eval`, `Call`, the JSON of an `Object`, `Value.MarshalJSON` and host function arguments used to get an empty string, and the out of memory exception stayed in the context. A BigInt whose `toString` throws now reports that exception instead of a made-up error. `ErrorFromValue` has no error result: a field it cannot read stays empty, and the exception is dropped. Reading the name and message of a thrown value that is not an object (`throw null`, which the engine throws near the memory limit, `throw undefined`, a symbol) no longer leaves a TypeError in the context.

Test: `gostring_test.go`.

## Valid UTF-8 for lone surrogates

Release: v0.26.8.

A Go string made from a JS string is valid UTF-8. A JS string can hold a lone surrogate, half of a surrogate pair without the other half, which code makes when it cuts a string inside an emoji. `JS_ToCStringLen2` wrote it as the three bytes of its code point, which is not UTF-8, so a page rendered to Go had bytes a browser shows as three replacement characters. Now each lone surrogate becomes U+FFFD, as Go's `utf16.Decode`, `TextEncoder` and Node's `Buffer` make it.

Test: `surrogate_test.go`.

## Invalid UTF-8 from Go becomes one U+FFFD per byte

Release: v0.26.10.

A Go string that is not valid UTF-8 becomes a JS string with U+FFFD for each byte that does not start a valid sequence, as Go decodes it (`for range`, `[]rune`, `utf16.Encode`) and as `encoding/json` writes it, so a string passed as it is and one inside a struct, which goes as JSON, arrive the same. This covers arguments of `Call`, results of Go functions, map keys and values, `NewString`, `NewAtom`, the message of an error a Go function returns, function names and module names. The engine used to skip the bytes after an invalid one, ASCII included: `"\x80a"` became a single U+FFFD and lost the `a`, `"\xc0\x80"` gave one U+FFFD instead of two, and the three-byte form of a surrogate (CESU-8, `0xED 0xA0` and on) became that surrogate, so two of them made an emoji. A sequence cut short, such as `"\xe4\xb8"`, now gives one U+FFFD per byte, where a browser's `TextDecoder` gives one for the sequence. Source code is not changed: the parser still reports invalid UTF-8 as a `SyntaxError`, and still takes a surrogate in the three-byte form.

Test: `invalidutf8_test.go`.

## *Error says why a call ended

Release: v0.26.7.

`*Error` says whether the call ended with an exception no script could catch, and why, so that an embedder need not compare names and messages, which a script can copy. `Uncatchable` is true when the VM ran out of memory (`OutOfMemory`, also when the engine could not build the error and threw `null`), when `Interrupt` or the eval timeout stopped the script (`Interrupted`), and when a Go function that javascript called panicked (neither of the two). The same holds for the error a Go function gets from javascript it evaluates. What a script throws is never uncatchable, even `new InternalError('interrupted')` or `{name: 'InternalError', message: 'out of memory'}`, which used to look exactly like the real thing. Taking an uncatchable error runs no page code: `Name` is `InternalError`, `Message` and `Stack` are the error's own properties, and `Error()` is `InternalError: <message>`. It used to call an `Error.prototype.toString` or a `name` getter that page code installed, which could rewrite the message. An interrupt that runs out of memory building its error counts as `OutOfMemory`.

Tests: `uncatchable_test.go`, `hostuncatchable_test.go`, `hostpanic_test.go`.

## RegisterFunc no longer aborts on an existing name

Release: v0.26.11.

`RegisterFunc` and `RegisterHostFunc` no longer abort the process when the global object already has the name, for example after `globalThis.g = 1`, or for `Array` or `undefined`. They installed the function with `JS_SetPropertyFunctionList`, which is meant for new objects and aborts when the property exists. Now the function replaces a configurable property, as `Object.defineProperty` would, and stays writable, configurable and not enumerable. A property that is not configurable (`undefined`, `NaN`, `Infinity`, a script's `var` or `function`) is kept, and so is a global object that is not extensible: the call returns an error. A name is now taken once per VM as javascript sees it, after invalid UTF-8 is replaced, so `"\xc0\x80"` after `"\xe4\xb8"` (both two U+FFFD) is `func already registered` rather than an abort, and a name with a NUL byte, which ended the name, is an error. A name in brackets is a string too: the function lists read `"[Symbol.iterator]"` as that symbol and aborted on other names in brackets.

Test: `register_test.go`.

## NewVM leaks nothing when it fails

Release: v0.26.14.

When `NewVM` cannot make a context, for example with no memory left, it frees what it allocated before: the `argv` of `toString` and the interrupt flag, which it leaked.

Test: `TestNewVMContextFailure` in `close_test.go`, also run under memgrind.

## SetShapeCache and DefaultShapeCache

Release: v0.26.16.

`DefaultShapeCache` (2048) is the number of shapes a new VM keeps once their objects are gone, and `VM.SetShapeCache(n)` changes it for one VM, 0 turning it off. A VM that runs close to its memory limit can run out of memory with the cache on where it would not without it: give such a VM a smaller cache, or none. [Kept shapes](objects-and-shapes.md#kept-shapes) has what it gains and costs.

## Builtin, a Go function called as a built-in

Release: v0.26.17.

`Builtin` is a Go function javascript calls as a built-in function, without the reflection and conversions of `RegisterFunc` and `RegisterHostFunc`. `NewBuiltin(name, length, fn)` makes one, once per process, and `VM.NewBuiltinFunction` makes a function object of it in any VM: it has its name and length and nothing else of its own, no `prototype`, it cannot be called with `new` and its `toString` shows `[native code]`, like `Math.max`. `fn` reads the arguments it wants from `Args` (`Number`, `String`, `Bool`, `Value`, `This`, `VM`), which convert nothing, and returns a `Value` (the zero `Value` is `undefined`). VMs on several goroutines call it at once, so it has to be safe for that, and keeps what it needs per VM by `Args.VM()`.

An `*Error` it returns is thrown as the constructor its `Name` says (`Error`, `TypeError`, `RangeError`, `SyntaxError`, `ReferenceError` or `InternalError`, and any other name as `Error`), with a stack that starts at the Builtin, any other error as a `TypeError`.

On an Apple M4 a call that adds two numbers takes about 55 ns, loop included, where the same call of a `RegisterHostFunc` function takes about 470 ns. A VM's context now carries an id, which is how a call finds its VM without a lock. Package `intl` calls Go this way (see [time-and-intl.md](time-and-intl.md#intl-calls-go-through-builtins)).

Test: `builtin_test.go`.

## Args.WTF8

Release: not released yet.

`Args.WTF8(i)` reads a string argument as the engine holds it: a lone surrogate is the three bytes of its code point (0xED, then 0xA0 to 0xBF, then a continuation byte), where `Args.String` gives U+FFFD for it, and a surrogate pair is its one character in UTF-8, as before. The result is not valid UTF-8 when the string has a lone surrogate. A Builtin that has to tell lone surrogates apart reads its strings this way, as `Intl.Collator` does to order them as Chromium does.

Test: `builtin_test.go`.

## Error() of an Error made in Go

Release: v0.26.17.

`Error()` of an `*Error` made in Go is now `Name: Message`, as `Error.prototype.toString` writes it, where it was empty before, so the `TypeError` a `RegisterHostFunc` function throws for such an error now has that message too. `Error()` of an error taken from javascript is as before.

Test: `builtin_test.go`.

## NewArray

Release: v0.26.17.

`VM.NewArray(vals...)` makes an array of values made in Go, as an array literal makes one: its elements are its own properties, so a setter a page put on `Array.prototype` never runs, and it costs no JSON. It borrows the values, as `SetPropertyValue` does, and refuses a value of another VM, a number too. A Builtin returns a list this way.

Test: `builtin_test.go`.

## SetStrip

Release: v0.26.17.

`VM.SetStrip(StripSource)` compiles functions without their source, so their `toString` shows `[native code]` as the engine's built-ins do, and `StripDebug` leaves out line and column numbers too. Both are values of `type Strip`. It wraps `JS_SetStripInfo`: the engine ignores a `"use strip"` directive. It applies to every function the VM compiles from then on, those of the page's `eval` and `new Function` too. Bytecode from `Compile` keeps it in any VM that loads it, which is what a polyfill playing built-ins wants.

Test: `strip_test.go`.

## RunGC, MemorySize and GCThreshold

Release: v0.26.17.

`VM` has three GC methods next to `SetGCThreshold`. `RunGC` runs a GC now, `MemorySize` returns the memory the VM takes, as the memory limit and the GC threshold count it, and `GCThreshold` returns the threshold. The engine runs a GC when it makes an object and the memory would pass the threshold, then sets the threshold to 1.5 times what the GC left, so a threshold other than the one set last means the engine ran a GC since. `RunGC` leaves the threshold as it is.

With these an embedder can collect between two calls into javascript, where the GC holds up no call, and set the threshold for the next one, as tifa does between two renders, without reading the runtime through `reflect`. As with `SetGCThreshold`, no other goroutine may use the VM meanwhile, and a call after `Close` panics.

Test: `gc_test.go`.

## OnClose

Release: not released yet.

`VM.OnClose(f)` has `Close` run `f`, after the VM is released, so `f` must not use it. `Close` runs the functions once each, in the order given. Nothing else runs them: not the garbage collector, and not a VM that is never closed.

It is for a package that keeps something per VM in a table of its own. An entry whose value is a closure that references the VM keeps the VM reachable from the table, so a cleanup the garbage collector would run for the VM never runs, and the entry stays for the life of the process. Dropping the entry in `OnClose` ends that. Package `temporal` keeps the clock of a VM this way.

Test: `onclose_test.go`.
