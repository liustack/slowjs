---
summary: 'The fork changes to the Go API: what Call reports, strings Go cannot convert, lone surrogates and invalid UTF-8 between Go and javascript, what *Error says about an uncatchable end, RegisterFunc on an existing name, NewVM failing cleanly, SetShapeCache, SetRegExpCache, Builtin, Args.WTF8, strings of Builtins converted in Go, Args.Latin1 and Args.UTF16 with NewStringLatin1 and NewStringUTF16, NewArray, SetStrip, the text of an Error made in Go, RunGC, MemorySize and GCThreshold, OnClose, and how EvalBytecode waits for a module's top-level await'
read_when:
  - Changing quickjs.go, or how strings, errors or host functions cross between Go and javascript
  - A Go string from javascript has replacement characters, or a string from Go reads differently in javascript
  - Telling an interrupt or out of memory from an error a script threw
  - Registering a Go function under a name the global object already has
  - Calling VM.SetShapeCache or VM.SetRegExpCache
  - Giving javascript a Go function that it calls often, or returning a list from Go (Builtin, NewArray)
  - Reading a string argument whose lone surrogates matter, such as one to collate (Args.WTF8)
  - A Builtin that takes or returns strings is slow, or looks at the characters of short strings and often returns them as they are (Args.Latin1, Args.UTF16, NewStringLatin1, NewStringUTF16)
  - Hiding the source of a script's functions (SetStrip)
  - Running a GC between two calls into javascript, or reading how much memory a VM takes (RunGC, MemorySize, GCThreshold)
  - Keeping something per VM outside it, to drop when the VM is closed (OnClose)
  - Evaluating the bytecode of a module (EvalBytecode), or a module whose top-level await is still pending when it returns
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

## SetRegExpCache and DefaultRegExpCache

Release: v0.27.0.

`DefaultRegExpCache` (256) and `DefaultRegExpCacheBytes` (1 MiB) are how many compiled regexps a new VM keeps at most, and in how many bytes, counting their patterns and flags. `VM.SetRegExpCache(n, bytes)` changes both for one VM and lets go of what it kept, 0 turning it off. `n` can be at most `math.MaxInt32/8`, as for `SetShapeCache`. A VM that runs close to its memory limit can run out of memory with the cache on where it would not without it: give such a VM a smaller cache, or none. [Compiled regexps kept per VM](regexp.md#compiled-regexps-kept-per-vm) has what it gains and costs.

## Builtin, a Go function called as a built-in

Release: v0.26.17.

`Builtin` is a Go function javascript calls as a built-in function, without the reflection and conversions of `RegisterFunc` and `RegisterHostFunc`. `NewBuiltin(name, length, fn)` makes one, once per process, and `VM.NewBuiltinFunction` makes a function object of it in any VM: it has its name and length and nothing else of its own, no `prototype`, it cannot be called with `new` and its `toString` shows `[native code]`, like `Math.max`. `fn` reads the arguments it wants from `Args` (`Number`, `String`, `Bool`, `Value`, `This`, `VM`), which convert nothing, and returns a `Value` (the zero `Value` is `undefined`). VMs on several goroutines call it at once, so it has to be safe for that, and keeps what it needs per VM by `Args.VM()`.

An `*Error` it returns is thrown as the constructor its `Name` says (`Error`, `TypeError`, `RangeError`, `SyntaxError`, `ReferenceError` or `InternalError`, and any other name as `Error`), with a stack that starts at the Builtin, any other error as a `TypeError`.

On an Apple M4 a call that adds two numbers takes about 55 ns, loop included, where the same call of a `RegisterHostFunc` function takes about 470 ns. A VM's context now carries an id, which is how a call finds its VM without a lock. Package `intl` calls Go this way (see [time-and-intl.md](time-and-intl.md#intl-calls-go-through-builtins)).

Test: `builtin_test.go`.

## Args.WTF8

Release: v0.27.0.

`Args.WTF8(i)` reads a string argument as the engine holds it: a lone surrogate is the three bytes of its code point (0xED, then 0xA0 to 0xBF, then a continuation byte), where `Args.String` gives U+FFFD for it, and a surrogate pair is its one character in UTF-8, as before. The result is not valid UTF-8 when the string has a lone surrogate. A Builtin that has to tell lone surrogates apart reads its strings this way, as `Intl.Collator` does to order them as Chromium does.

Test: `builtin_test.go`.

## Strings of Builtins converted in Go

Release: v0.27.2.

`Args.String`, `Args.WTF8` and `VM.NewString` convert in Go, with the same results. `Args.String` and `Args.WTF8` read a string the engine holds in one piece straight from its characters, where `JS_ToCStringLen2` scanned every string in engine code, made a UTF-8 copy in engine memory of any string that was not ASCII, and Go copied that again. `NewString` (and every Go string that becomes a JS string: arguments of `Call`, results of Go functions and the rest) writes the characters straight into the new string, where it copied the Go string to C memory for `JS_NewStringLen` to decode. A string the engine left in pieces by concatenation (a rope) is still joined by the engine, and a string longer than javascript allows still goes through `JS_NewStringLen`, which throws as before. `Value.Dup` adds its reference in Go, where `JS_DupValue` took a TLS frame to do it.

It is safe because the results are the same strings: the tests convert each string both ways and compare. For reading, every form the engine holds (8-bit and 16-bit, ASCII, Latin-1, every UTF-8 length, surrogate pairs, lone surrogates at the start, middle and end, ropes, strings up to 65,000 characters and 2,000 random ones). For making strings, the same strings with each kind of invalid UTF-8, where the new string must also be 8-bit or 16-bit as `JS_NewStringLen` made it. The UTF-8 check is compared with Go's on every string of up to three bytes. Reading a string in one piece no longer needs engine memory, so near the memory limit it can no longer fail there: only a rope still can.

Measured with `BenchmarkBuiltinString` on linux/amd64 (AMD EPYC 7B13, one pinned core, 6 rounds in 3 code layouts, times per call with the loop that makes it, where a call of a javascript function takes 60 ns):

| Call | Before | After |
| --- | --- | --- |
| A Builtin returning its argument as it is | 97 ns | 81 ns |
| Reading a 16-character ASCII string | 131 ns | 105 ns |
| Reading and returning it as a new string | 230 ns | 134 ns |
| The same with 16 characters of Latin-1, or of CJK | 359, 484 ns | 211, 314 ns |
| Reading 1,024 characters of ASCII, Latin-1, CJK | 865, 1685, 4098 ns | 316, 1096, 3387 ns |
| Reading and returning them as a new string | 1338, 7243, 13699 ns | 522, 4792, 11286 ns |

A Builtin that adds two numbers took 72 ns before and 75 to 81 ns after, depending on the code layout: nothing on its path changed, and the same layouts move it by as much.

Tests: `TestArgStringSameAsEngine`, `TestNewStringSameAsEngine`, `TestUTF8Units` and `TestBuiltinStringRoundTrip` in `jsstring_test.go`, also run under memgrind.

When upstream changes it: `lib/strchars.go` reads and writes a string with the offsets of its length, its wide bit and its characters that fixlib takes from `JSString` for [the JSON strings](json.md), so a new layout reaches it through those constants, and it makes strings with `js_alloc_string`. If upstream changes how `JS_ToCStringLen2` or `JS_NewStringLen` convert, change `jsstring.go` to match: the tests above compare the two on every form of string and fail first.

## Args.Latin1, Args.UTF16, NewStringLatin1 and NewStringUTF16

Release: v0.27.2.

`Args.Latin1(i)` gives the characters of a string argument one byte each, and `Args.UTF16(i)` its 16-bit units, lone surrogates as they are, without a copy: the bytes and units are the engine's, valid until the Builtin returns, and must not be changed. The engine holds a string one way or the other, so for a string in one piece exactly one of the two answers. A rope gives neither, and so does any value that is not a string: read those with `Args.String`. `VM.NewStringLatin1(b)` makes a string of bytes that are characters U+0000 to U+00FF, and `VM.NewStringUTF16(u)` one of 16-bit units, each with one copy. `NewStringUTF16` of units all below U+0100 makes a string of 8-bit characters, as the engine makes it. A string longer than javascript allows is an error.

A Builtin that looks at the characters of short strings and often returns its argument as it is, such as an escape for HTML, converts nothing this way: it reads the characters in place and returns `a.Value(0).Dup()`. React's escape of text written that way takes 99 ns a call for a 16-character string without a character to escape, against 200 ns for React's own javascript and 156 ns before (125 ns after the change above) for one that reads with `Args.WTF8`. For a string with three characters to escape it takes 163 ns, where React's javascript takes 3.3 µs.

Test: `TestBuiltinLatin1UTF16` in `jsstring_test.go`, also run under memgrind, and `TestEscapeView` in `builtinstring_test.go`.

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

Release: v0.27.0.

`VM.OnClose(f)` has `Close` run `f`, after the VM is released, so `f` must not use it. `Close` runs the functions once each, in the order given. Nothing else runs them: not the garbage collector, and not a VM that is never closed.

It is for a package that keeps something per VM in a table of its own. An entry whose value is a closure that references the VM keeps the VM reachable from the table, so a cleanup the garbage collector would run for the VM never runs, and the entry stays for the life of the process. Dropping the entry in `OnClose` ends that. Package `temporal` keeps the clock of a VM this way.

Test: `onclose_test.go`.

## EvalBytecode waits for a module's top-level await in Go

Release: v0.27.0.

`EvalBytecode` and `EvalBytecodeValue` of a module run pending jobs until the module's promise settles, in Go, and follow the rule of `ExecutePendingJobs`:

- A fulfilled module returns its result, and a rejected one returns the rejection as the error, as before.
- A job that throws does not stop the wait. Promise reactions throw when a page makes them throw, such as a `Promise` subclass whose species gives a resolve function that throws, and so does a `FinalizationRegistry` callback that has nothing to do with the module. The module runs on, and the first such error is what the call returns. When the module itself rejects, its rejection is the error instead, as Node reports the module's error and drops the job's.
- An uncatchable exception (an interrupt, the eval timeout, out of memory, a panic in a Go function) ends the call at once with its `*Error`, or its panic. Other jobs the module queued stay in the queue, as `ExecutePendingJobs` leaves them (see [uncatchable.md](uncatchable.md#interrupts-cannot-be-caught-through-promises-or-iterators)).
- When no job is left and the module is still pending, as for `await new Promise(() => {})`, the error says the module's top-level await is still pending and no job is left to run (joined with the first job error, if a job threw), and the VM can go on. What the module waits for may still settle from outside the job queue: a promise that a Go function returned and Go resolves later, or one that an outer call resolves after a nested `EvalBytecode` returned. Then a later `ExecutePendingJobs` runs the rest of the module.

They called `js_std_await` of `quickjs-libc.c`, the helper of the `qjs` shell. When the job queue ran dry with the promise pending, it read the state that `js_std_init_handlers` sets up, which a VM never sets up, and the process crashed with a nil pointer dereference. A job that failed was printed to stderr and dropped, so the eval timeout after a module's first `await` crashed the same way once the queue was empty. The `qjs` shell does not crash there, it spins on the empty queue for good, and Node ends a module whose top-level await never settles with exit code 13.

Test: `moduleawait_test.go`, also run under memgrind, and the module case of `TestNestedEvalKeepsTimeout` and `TestNestedEvalKeepsInterrupt` in `timeout_test.go`.
