---
summary: 'The fork changes to the Go API: what Call reports, strings Go cannot convert, lone surrogates and invalid UTF-8 between Go and javascript, what *Error says about an uncatchable end, RegisterFunc on an existing name, NewVM failing cleanly, and SetShapeCache'
read_when:
  - Changing quickjs.go, or how strings, errors or host functions cross between Go and javascript
  - A Go string from javascript has replacement characters, or a string from Go reads differently in javascript
  - Telling an interrupt or out of memory from an error a script threw
  - Registering a Go function under a name the global object already has
  - Calling VM.SetShapeCache
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
