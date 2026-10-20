---
summary: 'The fork changes that make interrupts, timeouts and running out of memory end the call whatever the script does: no catch through promises, iterators or Go functions, panics of Go functions, compiling with little memory, the leak of JS_ToCStringLen2, the eval timeout on the monotonic clock, nested calls, and Interrupt during Close'
read_when:
  - Changing lib/uncatchable.go, lib/outofmemory.go, fixUncatchable, fixOutOfMemory, fixCompile or fixToCString
  - fixlib stops on XJS_Throw, exceptionHandlers, exceptionWriters, the out of memory error, the compiler or XJS_ToCStringLen2 after an upstream release
  - A script caught an interrupt or an out of memory error, or a timeout did not stop a script
  - A Go function given to javascript panics, or gets an error from javascript it evaluates
  - Calling Interrupt from another goroutine, or setting the eval timeout
---

# Uncatchable exceptions, out of memory and interrupts

An embedder that sets a memory limit or a timeout needs the call to end when either is hit, whatever the page code does. These changes make sure no `catch`, `finally`, rejection handler or Go function of the script can take such an exception, and that the VM stays usable or at least closable after it. The Go API reports why a call ended this way (see [go-api.md](go-api.md#error-says-why-a-call-ended)).

## No leak when converting a string runs out of memory

Release: v0.26.0.

A VM that runs out of memory inside `JSON.parse`, or anything else that converts a string with non-ASCII characters to UTF-8, no longer leaks that string. `JS_ToCStringLen2` kept a reference to its argument when allocating the UTF-8 copy failed, so the string outlived `VM.Close`. QuickJS, quickjs-ng and libquickjs still had this bug when the fork was made.

Test: `lib/tocstring_oom_test.go`.

### When upstream changes it

What to do when `go run ./internal/fixlib` stops on this change after an upstream release. [upgrading.md](../upgrading.md) has the whole procedure and the order in which to update the lists `fixlib` checks against.

If `fixlib` reports that `XJS_ToCStringLen2` changed shape, read the new upstream code: QuickJS may have fixed the leak itself. If the fail label now frees `val`, drop `fixToCString` and the `JS_ToCStringLen2` hunk, or the added calls free it twice. If there is a new jump to the label, check that `val` is freed on that path.

## Interrupts cannot be caught through promises or iterators

Release: v0.26.6.

Page code can no longer catch an interrupt (`VM.Interrupt` or the eval timeout) through a promise or an iterator. The engine already skipped every `catch` and `finally` block for an interrupt, but its own handlers of an exception went on with it: an async function and a promise job turned it into a rejection, which an `await`, a `.catch` or a `.then` handler then received, and so did the `Promise` constructor, `Promise.try`, `Promise.all` and the other combinators, async generators and modules. A built-in that runs an iterator's `return` method on an error (`Array.from`, ...) ran it, then threw the interrupt again as an error a `catch` could take, and any later error replaced it. Now an uncatchable exception stays pending until the Go call that ran the script takes it, and while it is pending every call into javascript fails at once. So no `catch`, `finally`, iterator `return`, generator `finally` or rejection handler runs, and the call returns the interrupt, as it does for a script without promises. Where a built-in only records the error, such as a resolve function that rejects its promise when reading `then` throws, the code after it runs on until its next function call, which fails. `ExecutePendingJobs` stops at the job that failed with an interrupt and leaves the rest queued, where it used to run them all.

Tests: `uncatchable_test.go`, `internal/fixlib/uncatchable_test.go`.

### When upstream changes it

If it reports that `XJS_Throw`, `_JS_CallInternal` or `_JS_IteratorClose` changed shape, put the uncatchable check of `lib/uncatchable.go` back where the C change has it.

If it reports a call of `XJS_GetException` that is not in `exceptionHandlers` or `exceptionReaders` (`internal/fixlib/uncatchable.go`), read the new caller: if it hands the exception to a script (a rejection, a handler call, a new throw), add it to `exceptionHandlers`, so that it gets `_js_get_exception`. If it reports a store of the runtime's exception (`current_exception`) in a function that is not in `exceptionWriters`, read it the same way: code that takes the exception out by hand must leave an uncatchable one pending, as `_JS_IteratorClose` does. The same goes when it reports a use of the exception's address (`rt` plus its offset, or `&` of the field): a store through it does not look like one. fixlib sees the address only on a runtime named `rt`, `rt1` and so on, or `Frt`, with the offset written out: a store through a runtime under another name goes through unseen. If it reports that `XJS_Throw` or `_JS_IteratorClose` uses the exception before the uncatchable check, move the check up to before that use.

## Out of memory is uncatchable

Release: v0.26.6.

Running out of memory is uncatchable too, like an interrupt, and the call returns `*Error{Name: "InternalError", Message: "out of memory"}`. Page code used to catch it, so a framework catching every error to show a fallback reported it as an error of the page, and a `finally` block or a rejection handler ran with no memory left. Every way the engine runs out of memory now ends this way: the engine's own out of memory error, the regexp engine running out of memory (it used to throw `InternalError: out of memory in regexp execution`, or a `SyntaxError` when compiling a pattern), and an error the engine could not build. That last case used to throw a catchable `null`, and an error whose message could not be allocated got the exception marker as its message, which read back as nonsense. Now the engine builds the message first, and if the message, the error object or setting the message fails, it throws `null`, uncatchable, which the VM reports as the same out of memory error. A script that throws `null` itself still gets `null`. Page code cannot fill memory on purpose and catch the failure any more: allocating until it fails ends the call. The VM stays usable after the call, and `Close` frees everything.

Tests: `uncatchable_test.go`, `lib/outofmemory_test.go`, `internal/fixlib/outofmemory_test.go`.

### When upstream changes it

If it reports that `XJS_ThrowOutOfMemory`, `_JS_ThrowError2`, the regexp functions or the string table changed shape, compare the new upstream code with `lib/outofmemory.go`: `_js_throw_error2` replaces the end of `JS_ThrowError2` and must build the same error. That includes the end not matching one of the calls `fixThrowError2` expects (the property flags of `message`, the backtrace flags, a call made more than once). If it reports that the uses of the regexp out of memory error, of the `"out of memory"` message, of `lre_exec` or of `lre_compile` differ, read the new function: running out of memory there must throw with `XJS_ThrowOutOfMemory`, as in `_js_regexp_exec`. Then wire it in `fixOutOfMemory` and add it to `checkOutOfMemoryUses`.

## Interrupts and out of memory inside Go functions

Release: v0.26.6.

An interrupt or running out of memory inside a Go function that javascript called ends the whole call too. That covers converting the function's arguments or result, and javascript the function evaluates on the same VM (`Eval`, `Call` and the like), from a function registered with `RegisterFunc` or `RegisterHostFunc`, a `Then` callback, or the module loader and normalizer. The Go function still gets the error, but the exception stays pending until it returns, so further calls into javascript fail at once. Whatever the function returns, the script that called it gets the original uncatchable exception. It used to get a catchable `TypeError` wrapping the message (for example `calling take: argument conversion error: InternalError: out of memory`), or the function's own result when it dropped the error, so a timeout in nested javascript could be caught or lost and the script ran on. The `intl` package now takes its host functions off the global object before its install changes anything, so that an install cut short this way leaves none of them in sight once the next use has installed.

Tests: `hostuncatchable_test.go`, `intl/install_test.go`.

## A panic in a Go function

Release: v0.26.6.

A panic in a Go function that javascript called (`RegisterFunc`, `RegisterHostFunc`, a `Then` callback, the module loader or normalizer) no longer unwinds through the engine. The VM recovers it, ends the javascript with an uncatchable `InternalError` (`panic in Go function <name>: <value>`) as for an interrupt, and panics again with the same value in the Go code that called javascript: in `Eval` or the like, or in the Go function one level up when a Go function called the javascript. Page code sees no catch or finally run for it. The panic used to unwind through the engine, which kept pointing at stack frames that were gone. The VM looked fine after a recover, but the next error it made walked those frames, forever when a new frame took the place of an old one, so the eval timeout of that VM never fired again. The panic value is the same, but its stack trace now starts where the VM panics again.

Tests: `hostpanic_test.go`.

## Compiling with little memory left

Release: v0.26.6.

Compiling a script, a function body, `eval` code or a regexp with little memory left fails with the same out of memory error. The engine used to crash there (a Go panic on a write before or past the code buffer, or a double free), report a `SyntaxError` the script does not have, or, in `Compile`, return bytecode cut short, bytecode with the out of memory error left pending, or `errors.New("OOM")`. The parser went on after its code buffer failed to grow and patched code in place at positions it had noted, past what the buffer held (a Go panic on an empty buffer, or a write past the end of a full one), and the passes after it assumed the buffer was whole. Now the parser stops at the next token once the code buffer of the function it parses failed, and checks it before each patch that does not wait for a token, the buffer takes no more bytes once it failed, and each pass that reads it back stops with the out of memory error. A scope the parser fails to push marks the function's code lost too, and its scopes then stay as they are, each later push failing with the out of memory error: the pop after a failed push used to take off the scope around it, until a pop read before the scope array. The pass that resolves jumps frees, when it fails, the jump records of the labels it has not reached yet, which used to leak. When shrinking the line table of a function fails, the compile keeps the table and leaves no error behind: it used to work and leave the out of memory error pending, for the next call to report.

Tests: `compileoom_test.go`, `internal/fixlib/compile_test.go`.

### When upstream changes it

If it reports that `Xdbuf_claim`, `_push_scope`, `_pop_scope`, `_js_create_function`, `_resolve_variables`, `_resolve_labels`, the loops whose code the parser moves, `_js_parse_error_v`, `_next_token`, `_js_parse_class`, `_js_parse_destructuring_element`, the bytecode writer or `Xlre_compile` changed shape, read the new upstream code against the C change: the parser must stop once a code buffer failed and patch no code in place after that, and every pass that reads back a code buffer must stop when that buffer failed (`internal/fixlib/compile.go` lists each place). If it reports that growing a buffer in `Xdbuf_claim` no longer starts with the test of a failed buffer, read the new `Xdbuf_claim`: `fixCompile` marks a buffer that failed as full, and only an early return for a failed buffer keeps the next write out. If the test only moved and a failed buffer still fails at once, teach `fixCompile` the new place. If it is gone, make `fixCompile` add it. Do not edit the generated code by hand. If it reports a new jump to the fail label of `_resolve_labels`, read where it comes from: the end that `fixCompile` writes keeps the new code and frees the relocations left, which is right only after a failed `_add_reloc`. Make the end right for the new path, then fix the count.

## Nested calls keep the outer deadline

Release: v0.26.2.

A Go function that evaluates more javascript on its own VM, such as `intl` installing its code from inside the page's first call, no longer sets the eval timeout again or drops an `Interrupt` asked for meanwhile. The VM counts how deep the calls into javascript go, and only the outermost one sets the deadline and clears the request (see [architecture.md](../architecture.md#the-eval-timeout-and-interrupt)).

## The eval timeout runs on the monotonic clock

Release: v0.26.7.

The eval timeout (`SetEvalTimeout`) runs on the monotonic clock. It used to run on the wall clock, so setting the system clock while a script ran (NTP, a person, a machine waking from sleep) moved the deadline: set back, the timeout fired that much later or never, and set forward, at once. Even without a jump the two clocks drift apart by a few microseconds per 100 ms, so a server that keeps its own deadline on Go's monotonic clock could see the engine interrupt before that deadline. A timeout too long for the clock, up to `math.MaxInt64`, never fires: the deadline used to wrap around to the past and interrupt the call at once.

Test: `clock_test.go`.

## Interrupt is safe during and after Close

Release: v0.26.10.

`VM.Interrupt` can be called from any goroutine while the VM closes and after `Close`, where it does nothing. It used to read the address of the interrupt flag, which `Close` frees and then clears, so a watchdog firing just as the script finished could write to freed memory or panic on a nil pointer. The race detector also reported a race when `Interrupt` stopped a script and the VM was closed after the call returned. That one was not real: the two goroutines are ordered by the atomic flag, but the flag is in C memory, outside Go's heap, where the race detector ignores atomic operations. `Interrupt` and `Close` now meet at a mutex in Go memory that `Close` never clears.

Test: `interrupt_test.go`.
