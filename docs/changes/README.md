---
summary: 'Index of every change liustack/quickjs makes to the QuickJS C tree and to the Go API, by area: one line per change with the release it shipped in and the doc that has what it does, why it is safe, what it gains and which tests check it'
read_when:
  - Asking why a change to the engine is safe, what it costs and what it gains
  - Looking for the doc of a change before changing the same code
  - Adding a new change, to see where it belongs
---

# Changes

English · [简体中文](README.zh-CN.md)

The engine is QuickJS release 2026-06-04 (commit `3d5e064`, the C) with the changes below on top. Each doc below takes one area. For each change it says what the change does, why it is safe, what it gains or costs and how that was measured, and which tests check it. [upgrading.md](../upgrading.md) has the procedure of taking a new QuickJS release and [CHANGELOG.md](../../CHANGELOG.md) what each release changed for users.

The engine is the C in `engine/`, which the generator compiles to Go assembly. A change to it is a change of the C, checked in with the generated files (see [AGENTS.md](../../AGENTS.md)). Releases up to v0.28 ran QuickJS translated to Go by ccgo, with the changes of liustack/quickjs written into that Go by a tool that is gone now. The entries that name those releases keep the C form of their change here, and their timings were taken on that engine. [performance.md](../performance.md) has the speed of the C engine.

## The C engine

| Change | Release | Doc |
| --- | --- | --- |
| The C tree of the new engine: a small libc, host callbacks, no blocking Atomics, class IDs per runtime, recursion bounded by counted depths | v0.29.0 | [c-engine.md](c-engine.md) |
| A call sets and frees its values in fewer loop turns | v0.29.1 | [c-engine.md](c-engine.md#how-a-call-sets-and-frees-its-values) |
| The interpreter calls a C function without JS_CallInternal | v0.29.4 | [c-engine.md](c-engine.md#how-the-interpreter-calls-a-c-function) |
| The end of a call skips the values with no reference count | v0.29.5 | [c-engine.md](c-engine.md#the-end-of-a-call-skips-the-values-with-no-reference-count) |
| A call sets its locals eight to a turn | v0.29.5 | [c-engine.md](c-engine.md#a-call-sets-its-locals-eight-to-a-turn) |
| Opcodes that do a few instructions at once: a read of a local and its get_field, the case of a switch on a string, typeof tests | v0.29.5 | [c-engine.md](c-engine.md#opcodes-that-do-a-few-instructions-at-once) |
| Locals that are never live together share a slot of the frame | v0.29.7 | [c-engine.md](c-engine.md#locals-that-share-a-slot) |
| JSON.parse reads keys and plain strings from the text, with a cache of key atoms for one parse | v0.29.7 | [c-engine.md](c-engine.md#jsonparse-reads-keys-and-plain-strings-from-the-text) |
| A collection asked for while the engine frees objects does nothing | v0.29.10 | [c-engine.md](c-engine.md#a-collection-asked-for-while-objects-are-freed) |
| A panic that leaves a call into the engine halfway breaks the VM, whatever the panic is, a fault below 0x1000 too, and nothing enters that engine again, not even to free a value | v0.29.11 | [c-engine.md](c-engine.md#which-panics-break-a-vm) |
| Every generated function that calls something has Go's stack check, so a fault no longer depends on room the generator kept below a chain of functions without one | v0.29.11 | [c-engine.md](c-engine.md#functions-without-a-stack-check) |
| JavaScript runs in a register interpreter: a function is converted from its bytecode at its first call, calls between functions stay in one loop on a register stack of segments, and the stack interpreter of QuickJS is gone | v0.30.2 | [c-engine.md](c-engine.md#the-register-interpreter) |
| On windows every generated function makes room for 12 KiB more below its stack pointer, so that the state of the CPU the system writes at a fault stays inside the stack on CPUs with AMX | v0.30.2 | [c-engine.md](c-engine.md#room-for-a-fault-on-windows) |
| The generated code reads its arguments and writes its results by name through FP, as Go promises, and no longer at an offset from SP that depends on how the assembler lays out a frame. The machine code is the same | v0.30.2 | [c-engine.md](c-engine.md#arguments-and-results-by-name) |
| `go vet -asmdecl` finds nothing in the generated engine: the register-entry leaves of amd64 say NOFRAME, and `goasm_room` is in a file of windows/amd64 only, where its Go declaration is. The machine code is the same | v0.30.2 | [c-engine.md](c-engine.md#go-vet-checks-the-generated-assembly) |
| The string cases of a `switch` that end with any conditional jump take one opcode, and a chain of them runs in one dispatch. Bytecode version 8 | v0.30.2 | [c-engine.md](c-engine.md#a-read-of-a-local-and-its-get_field-and-the-case-of-a-switch-on-a-string) |
| A call of more than 32 arguments and a tagged template of more than 30 substitutions pass the arguments past the first 32 in an array, as a call with a spread does | v0.30.2 | [c-engine.md](c-engine.md#calls-of-more-than-32-arguments-pass-them-in-an-array) |
| The parser refuses at compile time a function the register loop cannot run: more than 32,765 parameters and locals, or a deep stack with more than 48 temps of catch offsets | v0.30.2 | [c-engine.md](c-engine.md#the-limits-of-a-function-in-the-register-loop) |
| A C function that call, apply, Reflect.apply or a bound function calls runs without JS_CallInternal, and JS_CallInternal makes the checks of a call inline | v0.30.2 | [c-engine.md](c-engine.md#a-c-function-that-call-apply-or-a-bound-function-calls) |
| The register loop answers `String.prototype.charCodeAt` of a string or a flat rope at an int index without the call | v0.30.2 | [c-engine.md](c-engine.md#charcodeat-answered-in-the-register-loop) |
| A closure holds the value of a captured variable no code can change once the closure has it, not a reference shared with the frame. Such closures no longer keep a suspended generator or async function alive | v0.30.3 | [c-engine.md](c-engine.md#captures-that-never-change) |
| A read refuses bytecode in which a catch offset leaves its place or a jump goes before the start of the code | v0.30.3 | [c-engine.md](c-engine.md#catch-offsets-and-jumps-stay-in-their-function) |
| Every jump table of arm64 takes 4 bytes an entry, and the generator refuses a table its lowering made too narrow | v0.30.3 | [c-engine.md](c-engine.md#jump-tables-on-arm64-take-4-bytes-an-entry) |
| A call copies its last arguments itself when they are locals, arguments or `this`, where a `MOV` came before it for each (`CALLS`, `CALLSD`) | v0.30.3 | [c-engine.md](c-engine.md#a-call-copies-its-last-locals-into-its-window) |
| `o[k]` with the key in a local or an argument reads the key without a reference | v0.30.3 | [c-engine.md](c-engine.md#a-key-in-a-local-is-read-without-a-reference) |
| `o[k] = v` with the key in a local or an argument sets it without a reference to the key | v0.30.3 | [c-engine.md](c-engine.md#a-key-in-a-local-is-set-without-a-reference) |
| `JS_ToBool` and the branches of the register loop on a local or an argument read the value without a reference | v0.30.3 | [c-engine.md](c-engine.md#the-truth-of-a-value-is-read-without-a-reference) |
| An array literal moves the references of its elements into the array | v0.30.3 | [c-engine.md](c-engine.md#an-array-literal-takes-the-references-of-its-elements) |
| On arm64 a function that calls no longer saves and loads LR itself, which Go's prologue and epilogue already do: two instructions less a call, with the same frames. amd64 is the same | v0.30.3 | [c-engine.md](c-engine.md#lr-is-saved-once-on-arm64) |
| The register loop takes a plain array of the realm apart with no iterator: DSTART and DNEXT read its elements, and the Array Iterator is made only where one is needed | v0.30.3 | [c-engine.md](c-engine.md#array-destructuring-with-no-iterator) |
| On amd64 every SSE instruction that needs its operand on 16 bytes reads data placed by a `PCALIGN $16`, or a local, or a global, and the generator and the post-link check refuse anything else | Unreleased | [c-engine.md](c-engine.md#sse-reads-that-need-16-bytes-stay-on-16) |
| A call through a pointer takes its result from registers, and a generated function that no Go code calls no longer writes its result to the Go stack | Unreleased | [c-engine.md](c-engine.md#results-only-in-registers) |
| `o[k]` with a string key that is its own atom is read as GETF reads `o.name`, without the generic path and without a reference to the atom | Unreleased | [c-engine.md](c-engine.md#a-string-key-that-is-its-own-atom-is-read-as-a-named-property) |
| A string keeps the number of the atom it is as a key, so a keyed read and `hasOwnProperty.call` take it in one read. Every string is 4 bytes larger | Unreleased | [c-engine.md](c-engine.md#a-string-keeps-the-atom-it-is-as-a-key) |

## Commits of upstream QuickJS

| Change | Release | Doc |
| --- | --- | --- |
| Commits of upstream QuickJS after 3d5e064 taken into the C tree one at a time (Array slice and splice, borrowed operands of js_strict_eq2, template literals, unicode_script), and the ones left out | v0.29.0 | [upstream-commits.md](upstream-commits.md) |
| Iterator.concat follows IteratorClose and finishes after a failed step | v0.29.8 | [upstream-commits.md](upstream-commits.md#iterator-concatenation) |
| Reducing an empty iterator without an initial accumulator does not close it | v0.29.8 | [upstream-commits.md](upstream-commits.md#empty-iterator-reduction) |
| Sparse array truncation removes indices before compacting the property table | v0.29.8 | [upstream-commits.md](upstream-commits.md#array-truncation) |
| The spread of a string gives its characters | Unreleased | [upstream-commits.md](upstream-commits.md) |

## Go API

| Change | Release | Doc |
| --- | --- | --- |
| Call reports the exception of the function expression | v0.26.0 | [go-api.md](go-api.md#call-reports-the-exception-of-the-function-expression) |
| A string Go cannot convert comes back as the error | v0.26.5 | [go-api.md](go-api.md#a-string-go-cannot-convert-comes-back-as-the-error) |
| `*Error` says why a call ended | v0.26.7 | [go-api.md](go-api.md#error-says-why-a-call-ended) |
| Valid UTF-8 for lone surrogates | v0.26.8 | [go-api.md](go-api.md#valid-utf-8-for-lone-surrogates) |
| Invalid UTF-8 from Go becomes one U+FFFD per byte | v0.26.10 | [go-api.md](go-api.md#invalid-utf-8-from-go-becomes-one-ufffd-per-byte) |
| RegisterFunc no longer aborts on an existing name | v0.26.11 | [go-api.md](go-api.md#registerfunc-no-longer-aborts-on-an-existing-name) |
| NewVM leaks nothing when it fails | v0.26.14 | [go-api.md](go-api.md#newvm-leaks-nothing-when-it-fails) |
| SetShapeCache and DefaultShapeCache | v0.26.16 | [go-api.md](go-api.md#setshapecache-and-defaultshapecache) |
| Builtin, a Go function called as a built-in | v0.26.17 | [go-api.md](go-api.md#builtin-a-go-function-called-as-a-built-in) |
| Error() of an Error made in Go | v0.26.17 | [go-api.md](go-api.md#error-of-an-error-made-in-go) |
| NewArray | v0.26.17 | [go-api.md](go-api.md#newarray) |
| SetStrip | v0.26.17 | [go-api.md](go-api.md#setstrip) |
| RunGC, MemorySize and GCThreshold | v0.26.17 | [go-api.md](go-api.md#rungc-memorysize-and-gcthreshold) |
| Args.WTF8 | v0.27.0 | [go-api.md](go-api.md#argswtf8) |
| SetRegExpCache and DefaultRegExpCache | v0.27.0 | [go-api.md](go-api.md#setregexpcache-and-defaultregexpcache) |
| OnClose | v0.27.0 | [go-api.md](go-api.md#onclose) |
| EvalBytecode waits for a module's top-level await in Go | v0.27.0 | [go-api.md](go-api.md#evalbytecode-waits-for-a-modules-top-level-await-in-go) |
| Strings of Builtins converted in Go | v0.27.2 | [go-api.md](go-api.md#strings-of-builtins-converted-in-go) |
| Args.Latin1, Args.UTF16, NewStringLatin1 and NewStringUTF16 | v0.27.2 | [go-api.md](go-api.md#argslatin1-argsutf16-newstringlatin1-and-newstringutf16) |
| A module frees its top level once it has run | v0.28.4 | [go-api.md](go-api.md#a-module-frees-its-top-level-once-it-has-run) |
| The async context of promise jobs, for AsyncLocalStorage | v0.28.5 | [go-api.md](go-api.md#the-async-context-of-promise-jobs) |
| RegisterEscapeText, React DOM's HTML escape written in C | v0.29.2 | [go-api.md](go-api.md#registerescapetext) |
| RegisterCopyProps, the loop that copies the props of a React element written in C | v0.29.3 | [go-api.md](go-api.md#registercopyprops) |
| RegisterStartTag and RegisterEndTag, the start and end tags React DOM writes for an element written in C | v0.29.10 | [go-api.md](go-api.md#registerstarttag-and-registerendtag) |
| A Go newer than the one the engine was checked with stops the build | v0.29.11 | [go-api.md](go-api.md#a-go-newer-than-the-one-the-engine-was-checked-with-stops-the-build) |
| A build option that changes how the assembler encodes the engine stops the build | v0.29.11 | [go-api.md](go-api.md#a-build-option-that-changes-how-the-assembler-encodes-the-engine-stops-the-build) |
| A Value of another VM is refused at every entry that takes one | v0.30.2 | [go-api.md](go-api.md#a-value-of-another-vm-is-refused-at-every-entry) |
| The constructors of values report running out of memory | v0.30.2 | [go-api.md](go-api.md#the-constructors-of-values-report-running-out-of-memory) |
| A result that fails to convert leaves nothing behind | v0.30.2 | [go-api.md](go-api.md#a-result-that-fails-to-convert-leaves-nothing-behind) |
| UndefinedValue as an argument and a property value | v0.30.2 | [go-api.md](go-api.md#undefinedvalue-as-an-argument-and-a-property-value) |
| Close inside a call into the VM is refused | v0.30.2 | [go-api.md](go-api.md#close-inside-a-call-into-the-vm-is-refused) |
| Then returns the exception of a then getter | v0.30.2 | [go-api.md](go-api.md#then-returns-the-exception-of-a-then-getter) |
| A Go map becomes an object of its own data properties | v0.30.2 | [go-api.md](go-api.md#a-go-map-becomes-an-object-of-its-own-data-properties) |
| A Value returned as an any is taken as a Value | v0.30.2 | [go-api.md](go-api.md#a-value-returned-as-an-any-is-taken-as-a-value) |
| A place a result reaches twice is one reference | v0.30.2 | [go-api.md](go-api.md#a-place-a-result-reaches-twice-is-one-reference) |
| A result that contains itself is an error | v0.30.2 | [go-api.md](go-api.md#a-result-that-contains-itself-is-an-error) |
| The references of a result are taken before code of the program runs | v0.30.2 | [go-api.md](go-api.md#the-references-of-a-result-are-taken-before-code-of-the-program-runs) |
| NewArray stops at the limits of the call | v0.30.2 | [go-api.md](go-api.md#newarray-stops-at-the-limits-of-the-call) |
| Long strings and results stop converting at the limits of the call | v0.30.3 | [go-api.md](go-api.md#long-strings-and-results-stop-converting-at-the-limits-of-the-call) |
| Args.StringLen reads the length of a string in place | v0.30.3 | [go-api.md](go-api.md#argsstringlen-reads-the-length-of-a-string-in-place) |

## Limits that hold

| Change | Release | Doc |
| --- | --- | --- |
| No leak when converting a string runs out of memory | v0.26.0 | [uncatchable.md](uncatchable.md#no-leak-when-converting-a-string-runs-out-of-memory) |
| Nested calls keep the outer deadline | v0.26.2 | [uncatchable.md](uncatchable.md#nested-calls-keep-the-outer-deadline) |
| Interrupts cannot be caught through promises or iterators | v0.26.6 | [uncatchable.md](uncatchable.md#interrupts-cannot-be-caught-through-promises-or-iterators) |
| Out of memory is uncatchable | v0.26.6 | [uncatchable.md](uncatchable.md#out-of-memory-is-uncatchable) |
| Interrupts and out of memory inside Go functions | v0.26.6 | [uncatchable.md](uncatchable.md#interrupts-and-out-of-memory-inside-go-functions) |
| A panic in a Go function | v0.26.6 | [uncatchable.md](uncatchable.md#a-panic-in-a-go-function) |
| Compiling with little memory left | v0.26.6 | [uncatchable.md](uncatchable.md#compiling-with-little-memory-left) |
| The eval timeout runs on the monotonic clock | v0.26.7 | [uncatchable.md](uncatchable.md#the-eval-timeout-runs-on-the-monotonic-clock) |
| Interrupt is safe during and after Close | v0.26.10 | [uncatchable.md](uncatchable.md#interrupt-is-safe-during-and-after-close) |
| Compiling a regexp counts toward the interrupt check | v0.27.2 | [uncatchable.md](uncatchable.md#compiling-a-regexp-counts-toward-the-interrupt-check) |
| Parsing and building strings count toward the interrupt check | v0.27.2 | [uncatchable.md](uncatchable.md#parsing-and-building-strings-count-toward-the-interrupt-check) |
| Every method that can run javascript has the eval timeout | v0.29.5 | [uncatchable.md](uncatchable.md#every-method-that-can-run-javascript-has-the-eval-timeout) |
| Loops over a long string read the interrupt flag in steps | v0.29.5 | [uncatchable.md](uncatchable.md#loops-over-a-long-string-read-the-interrupt-flag-in-steps) |
| Out of memory in the return method of an iterator that an exception closes is not caught | v0.29.11 | [uncatchable.md](uncatchable.md#out-of-memory-in-the-return-method-of-an-iterator-that-an-exception-closes) |
| The start and end tag functions read the interrupt flag in every long loop | v0.30.0 | [uncatchable.md](uncatchable.md#the-start-and-end-tags-read-the-interrupt-flag-in-every-long-loop) |
| A long rope is made flat in steps that read the interrupt flag | v0.30.1 | [uncatchable.md](uncatchable.md#a-long-rope-is-made-flat-in-steps) |
| repeat reads the interrupt flag every 65,536 characters | v0.30.1 | [uncatchable.md](uncatchable.md#repeat-reads-the-interrupt-flag-every-65536-characters) |
| Compiling one long regexp reads the interrupt flag in every long loop | v0.30.1 | [uncatchable.md](uncatchable.md#compiling-one-long-regexp-reads-the-interrupt-flag-in-every-long-loop) |
| A class of a regexp reads the interrupt flag however many characters or strings it has | v0.30.1 | [uncatchable.md](uncatchable.md#a-class-of-a-regexp-reads-the-interrupt-flag-however-many-characters-or-strings-it-has) |
| The bytecode calls convert their result inside the eval timeout | v0.30.2 | [uncatchable.md](uncatchable.md#the-bytecode-calls-convert-their-result-inside-the-eval-timeout) |
| An uncatchable exception met while an exception is read ends the call | v0.30.2 | [uncatchable.md](uncatchable.md#an-uncatchable-exception-met-while-an-exception-is-read-ends-the-call) |
| A string argument Intl cannot read ends the call | v0.30.2 | [uncatchable.md](uncatchable.md#a-string-argument-intl-cannot-read-ends-the-call) |
| Intl reads the interrupt flag in loops over a long list | v0.30.2 | [uncatchable.md](uncatchable.md#intl-reads-the-interrupt-flag-in-loops-over-a-long-list) |
| The functions of fmt end the call when their output does not fit | v0.30.2 | [uncatchable.md](uncatchable.md#the-functions-of-fmt-end-the-call-when-their-output-does-not-fit) |
| Long operations on BigInts read the interrupt flag | v0.30.2 | [uncatchable.md](uncatchable.md#long-operations-on-bigints-read-the-interrupt-flag) |
| Turning a long string into a number reads the interrupt flag | v0.30.2 | [uncatchable.md](uncatchable.md#turning-a-long-string-into-a-number-reads-the-interrupt-flag) |
| Parsing a long source reads the interrupt flag | v0.30.2 | [uncatchable.md](uncatchable.md#parsing-a-long-source-reads-the-interrupt-flag) |
| Comparing and copying long strings reads the interrupt flag | v0.30.2 | [uncatchable.md](uncatchable.md#comparing-and-copying-long-strings-reads-the-interrupt-flag) |
| Resizing a large ArrayBuffer reads the interrupt flag | v0.30.2 | [uncatchable.md](uncatchable.md#resizing-a-large-arraybuffer-reads-the-interrupt-flag) |
| Out of memory in normalize and in the growth of a Map | v0.30.2 | [uncatchable.md](uncatchable.md#out-of-memory-in-normalize-and-in-the-growth-of-a-map) |
| A loop with no call ends at an uncatchable exception left pending | v0.30.2 | [uncatchable.md](uncatchable.md#a-loop-with-no-call-ends-at-an-uncatchable-exception-left-pending) |
| The flags of a regexp are checked without a pass over a long string | v0.30.2 | [uncatchable.md](uncatchable.md#the-flags-of-a-regexp-are-checked-without-a-pass-over-a-long-string) |
| A short argument of Intl and Temporal is measured before it is read | v0.30.3 | [uncatchable.md](uncatchable.md#a-short-argument-of-intl-and-temporal-is-measured-before-it-is-read) |
| The arithmetic of a calendar reads the interrupt flag | v0.30.3 | [uncatchable.md](uncatchable.md#the-arithmetic-of-a-calendar-reads-the-interrupt-flag) |
| fmt bounds a call with no memory limit, and writes in pieces | v0.30.3 | [uncatchable.md](uncatchable.md#fmt-bounds-a-call-with-no-memory-limit-and-writes-in-pieces) |
| A run no Builtin can cut reads the interrupt flag | v0.30.3 | [uncatchable.md](uncatchable.md#a-run-no-builtin-can-cut-reads-the-interrupt-flag) |
| Long text and lists of locales are worked on in checked pieces | Unreleased | [uncatchable.md](uncatchable.md#long-text-and-lists-of-locales-are-worked-on-in-checked-pieces) |
| The functions of fmt make a long output in checked pieces | Unreleased | [uncatchable.md](uncatchable.md#the-functions-of-fmt-make-a-long-output-in-checked-pieces) |
| A Go function takes the long arrays of a call from the OS | Unreleased | [uncatchable.md](uncatchable.md#a-go-function-takes-the-long-arrays-of-a-call-from-the-os) |
| A VM keeps its last block for the next call | Unreleased | [uncatchable.md](uncatchable.md#a-vm-keeps-its-last-block-for-the-next-call) |
| The copy of a spread, a rest and Object.assign reads the interrupt flag | Unreleased | [uncatchable.md](uncatchable.md#the-copy-of-a-spread-a-rest-and-objectassign-reads-the-interrupt-flag) |

## Errors and stacks

| Change | Release | Doc |
| --- | --- | --- |
| Error stacks formatted when first read | v0.26.1 | [errors-and-stacks.md](errors-and-stacks.md#error-stacks-formatted-when-first-read) |
| Endless recursion throws RangeError | v0.26.6 | [errors-and-stacks.md](errors-and-stacks.md#endless-recursion-throws-rangeerror) |
| new with a spread says when its callee is not a constructor | v0.30.2 | [errors-and-stacks.md](errors-and-stacks.md#new-with-a-spread-says-when-its-callee-is-not-a-constructor) |
| Calls through call and bound functions leave no line in the stack | v0.30.2 | [errors-and-stacks.md](errors-and-stacks.md#calls-through-call-and-bound-functions-leave-no-line-in-the-stack) |

## Time and Intl

| Change | Release | Doc |
| --- | --- | --- |
| Intl in every locale Chromium has | v0.26.2 | [time-and-intl.md](time-and-intl.md#intl-in-every-locale-chromium-has) |
| Local time is UTC in every VM | v0.26.3 | [time-and-intl.md](time-and-intl.md#local-time-is-utc-in-every-vm) |
| Fraction digits as ECMA-402 reads them since 2021 | v0.26.4 | [time-and-intl.md](time-and-intl.md#fraction-digits-as-ecma-402-reads-them-since-2021) |
| Daylight saving time named as ICU does | v0.26.9 | [time-and-intl.md](time-and-intl.md#daylight-saving-time-named-as-icu-does) |
| Intl calls Go through Builtins | v0.26.17 | [time-and-intl.md](time-and-intl.md#intl-calls-go-through-builtins) |
| Compiled Intl formatting plans and shared decoded data | v0.27.0 | [time-and-intl.md](time-and-intl.md#compiled-intl-formatting-plans) |
| All Intl formatters share go-intl | v0.27.0 | [time-and-intl.md](time-and-intl.md#all-intl-formatters-share-go-intl) |
| Intl constructors on go-intl | v0.27.0 | [time-and-intl.md](time-and-intl.md#intl-constructors-on-go-intl) |
| Intl leaves what a page put in place of a stand-in | v0.27.0 | [time-and-intl.md](time-and-intl.md#stand-ins-leave-what-a-page-put-in-their-place) |
| DateTimeFormat takes Temporal values and formats ranges | v0.27.0 | [time-and-intl.md](time-and-intl.md#datetimeformat-takes-temporal-values) |
| Temporal durations use DurationFormat | v0.27.0 | [time-and-intl.md](time-and-intl.md#temporal-durations-use-durationformat) |
| The engine reads Go's clocks | v0.27.0 | [time-and-intl.md](time-and-intl.md#the-engine-reads-gos-clocks) |
| One wall clock per VM for Date, Intl and Temporal, set with `VM.SetClock` | v0.28.2 | [time-and-intl.md](time-and-intl.md#one-wall-clock-per-vm) |
| Intl.Segmenter finds segments as they are read | v0.30.2 | [time-and-intl.md](time-and-intl.md#intlsegmenter-finds-segments-as-they-are-read) |
| The caches of Temporal formats keep short requests only | v0.30.2 | [time-and-intl.md](time-and-intl.md#the-caches-of-temporal-formats-keep-short-requests-only) |
| Number options are checked before they are rounded down | v0.30.2 | [time-and-intl.md](time-and-intl.md#number-options-are-checked-before-they-are-rounded-down) |
| A long locale tag resolves in time linear in its length | v0.30.2 | [time-and-intl.md](time-and-intl.md#a-long-locale-tag-resolves-in-time-linear-in-its-length) |
| A numeric string is read as Number reads it | v0.30.3 | [time-and-intl.md](time-and-intl.md#a-numeric-string-is-read-as-number-reads-it) |

## Temporal

| Change | Release | Doc |
| --- | --- | --- |
| Temporal on go-intl | v0.27.0 | [temporal.md](temporal.md#temporal-on-go-intl) |

## URL

| Change | Release | Doc |
| --- | --- | --- |
| URL and URLSearchParams on a Go parser, with the limits of the VM | v0.30.3 | [url.md](url.md#url-and-urlsearchparams-on-a-go-parser) |

## Regular expressions

| Change | Release | Doc |
| --- | --- | --- |
| Skip to where a regexp can match | v0.26.15 | [regexp.md](regexp.md#skip-to-where-a-regexp-can-match) |
| Alternatives that cannot match are not pushed | v0.26.17 | [regexp.md](regexp.md#alternatives-that-cannot-match-are-not-pushed) |
| Where a regexp starts to look is worked out once, with a table of first characters and a compare of a literal prefix. Bytecode version 9 | v0.30.2 | [regexp.md](regexp.md#where-a-regexp-starts-is-worked-out-once) |
| A long list of alternatives that start with a literal goes straight to the ones that start with the character. Bytecode version 10 | v0.30.2 | [regexp.md](regexp.md#long-lists-of-alternatives-that-start-with-a-literal) |
| Compiled regexps kept per VM | v0.27.0 | [regexp.md](regexp.md#compiled-regexps-kept-per-vm) |
| Classes of regexps with the v flag parsed as the specification does | v0.28.3 | [regexp.md](regexp.md#classes-of-regexps-with-the-v-flag) |

## Strings

| Change | Release | Doc |
| --- | --- | --- |
| Searches skip to where the string is | v0.27.3 | [strings.md](strings.md#searches-skip-to-where-the-string-is) |
| A symbol has a string of its own | v0.30.2 | [strings.md](strings.md#a-symbol-has-a-string-of-its-own) |
| The hash table of atoms holds one atom for two buckets | Unreleased | [strings.md](strings.md#the-hash-table-of-atoms-holds-one-atom-for-two-buckets) |
| An empty string on the left of `+` gives the string on the right, with no copy | Unreleased | [strings.md](strings.md#an-empty-string-on-the-left-of--gives-the-string-on-the-right) |

## JSON

| Change | Release | Doc |
| --- | --- | --- |
| JSON copies runs of plain characters | v0.26.17 | [json.md](json.md#json-copies-runs-of-plain-characters) |

## Operators

| Change | Release | Doc |
| --- | --- | --- |
| Bitwise operators on floats skip the slow path | v0.26.17 | [operators.md](operators.md#bitwise-operators-on-floats-skip-the-slow-path) |
| The usual comparisons answered in the interpreter | v0.26.17 | [operators.md](operators.md#the-usual-comparisons-answered-in-the-interpreter) |

## Objects and shapes

| Change | Release | Doc |
| --- | --- | --- |
| New properties of plain objects skip the generic path | v0.26.13 | [objects-and-shapes.md](objects-and-shapes.md#new-properties-of-plain-objects-skip-the-generic-path) |
| Kept shapes | v0.26.16 | [objects-and-shapes.md](objects-and-shapes.md#kept-shapes) |
| for in skips prototypes with no enumerable property | v0.26.17 | [objects-and-shapes.md](objects-and-shapes.md#for-in-skips-prototypes-with-no-enumerable-property) |
| String keys that are already atoms | v0.26.17 | [objects-and-shapes.md](objects-and-shapes.md#string-keys-that-are-already-atoms) |
| New functions get length and name at once | v0.26.17 | [objects-and-shapes.md](objects-and-shapes.md#new-functions-get-length-and-name-at-once) |
| Methods of strings, numbers and arrays found in the interpreter | v0.27.1 | [objects-and-shapes.md](objects-and-shapes.md#methods-of-strings-numbers-and-arrays-found-in-the-interpreter) |
| Own keys listed from the shape | v0.28.5 | [objects-and-shapes.md](objects-and-shapes.md#own-keys-listed-from-the-shape) |
| Ordinary objects take 48 bytes, not 72 | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.md#ordinary-objects-take-48-bytes-not-72) |
| Object literals made at their final shape | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.md#object-literals-made-at-their-final-shape) |
| A for in loop holds the shape of its object | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.md#a-for-in-loop-holds-the-shape-of-its-object) |
| hasOwnProperty.call answered in the interpreter | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.md#hasownpropertycall-answered-in-the-interpreter) |
| Spread, rest and Object.assign look each key up when its turn comes | v0.29.2 | [objects-and-shapes.md](objects-and-shapes.md#spread-rest-and-objectassign-look-each-key-up-when-its-turn-comes) |
| Object literals keep their properties in the block of the object | v0.29.4 | [objects-and-shapes.md](objects-and-shapes.md#object-literals-keep-their-properties-in-the-block-of-the-object) |
| Object literals of the same keys share one shape | v0.29.5 | [objects-and-shapes.md](objects-and-shapes.md#object-literals-of-the-same-keys-share-one-shape) |
| A delete that cannot pack the properties for lack of memory leaves no error pending | v0.29.5 | [objects-and-shapes.md](objects-and-shapes.md#object-literals-keep-their-properties-in-the-block-of-the-object) |
| A shared shape that cannot be copied fails the call | v0.29.9 | [objects-and-shapes.md](objects-and-shapes.md#a-shared-shape-that-cannot-be-copied-fails-the-call) |
| Objects of the built-in classes find the empty shape of their prototype | v0.29.10 | [objects-and-shapes.md](objects-and-shapes.md#objects-of-the-built-in-classes-find-the-empty-shape-of-their-prototype) |
| A shape is found by the shape it grew from | v0.29.10 | [objects-and-shapes.md](objects-and-shapes.md#a-shape-is-found-by-the-shape-it-grew-from) |
| An object that dies alone is freed at once | v0.29.10 | [objects-and-shapes.md](objects-and-shapes.md#an-object-that-dies-alone-is-freed-at-once) |
| A proxy whose defineProperty trap refuses a store throws in strict code | v0.30.2 | [objects-and-shapes.md](objects-and-shapes.md#a-proxy-that-refuses-a-store-throws-in-strict-code) |
| Object literals have the room of an empty object | v0.30.2 | [objects-and-shapes.md](objects-and-shapes.md#object-literals-have-the-room-of-an-empty-object) |
| A for in loop asks a shape once whether it lists its keys | v0.30.2 | [objects-and-shapes.md](objects-and-shapes.md#a-for-in-loop-asks-a-shape-once-whether-it-lists-its-keys) |
| An array that is appended to takes room for 8 elements at first | v0.30.3 | [objects-and-shapes.md](objects-and-shapes.md#an-array-that-is-appended-to-takes-room-for-8-elements-at-first) |
| The `in` operator walks a prototype chain without references | v0.30.3 | [objects-and-shapes.md](objects-and-shapes.md#the-in-operator-walks-a-prototype-chain-without-references) |
| An object rest leaves out the keys of its pattern by a list the parser makes, and is made at its final shape from a cache of the shapes of its sources | Unreleased | [objects-and-shapes.md](objects-and-shapes.md#an-object-rest-is-made-at-its-final-shape) |

## Memory

| Change | Release | Doc |
| --- | --- | --- |
| One copy of source text and line tables for every VM, SharedBytecode | v0.28.1 | [memory-and-allocation.md](memory-and-allocation.md#one-copy-of-source-text-and-line-tables-for-every-vm) |
| Shared bytecode is scanned for the polls on its first load only | v0.29.1 | [memory-and-allocation.md](memory-and-allocation.md#shared-bytecode-is-scanned-for-the-polls-on-its-first-load-only) |

## Young collection

| Change | Release | Doc |
| --- | --- | --- |
| Young collection, VM.RunYoungGC | v0.27.3 | [young-collection.md](young-collection.md) |

## Weak collections

| Change | Release | Doc |
| --- | --- | --- |
| WeakMap ephemeron collection | v0.27.2 | [weak-collections.md](weak-collections.md) |
| A WeakRef keeps its target to the end of the job, and the host ends the job when a call returns to Go | v0.30.2 | [weak-collections.md](weak-collections.md#a-weakref-keeps-its-target-to-the-end-of-the-job) |

## Left out of the engine

| Change | Release | Doc |
| --- | --- | --- |
| quickjs-libc.c keeps only what the Go API calls | v0.27.0 | [trimmed.md](trimmed.md#no-shell-library) |

## Adding a change

- Put it in the doc of its area, or in a new doc here when no area fits, and add a row to the table of that area, with "Unreleased" as its release. The release step in [releasing.md](../releasing.md#before-the-release) replaces "Unreleased" with the version, in the tables here and in the Release lines of the area docs. Never write it into README.md, which only links here.
- Say what it does, why it is safe, what it gains or costs with the machine and how it was measured, and which tests check it.
- Add it to [CHANGELOG.md](../../CHANGELOG.md) and [CHANGELOG.zh-CN.md](../../CHANGELOG.zh-CN.md), and a change for speed to [performance.md](../performance.md).
