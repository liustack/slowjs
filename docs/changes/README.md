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
| On windows every generated function makes room for 12 KiB more below its stack pointer, so that the state of the CPU the system writes at a fault stays inside the stack on CPUs with AMX | Unreleased | [c-engine.md](c-engine.md#room-for-a-fault-on-windows) |
| The generated code reads its arguments and writes its results by name through FP, as Go promises, and no longer at an offset from SP that depends on how the assembler lays out a frame. The machine code is the same | Unreleased | [c-engine.md](c-engine.md#arguments-and-results-by-name) |
| `go vet -asmdecl` finds nothing in the generated engine: the register-entry leaves of amd64 say NOFRAME, and `goasm_room` is in a file of windows/amd64 only, where its Go declaration is. The machine code is the same | Unreleased | [c-engine.md](c-engine.md#go-vet-checks-the-generated-assembly) |

## Commits of upstream QuickJS

| Change | Release | Doc |
| --- | --- | --- |
| Commits of upstream QuickJS after 3d5e064 taken into the C tree one at a time (Array slice and splice, borrowed operands of js_strict_eq2, template literals, unicode_script), and the ones left out | v0.29.0 | [upstream-commits.md](upstream-commits.md) |
| Iterator.concat follows IteratorClose and finishes after a failed step | v0.29.8 | [upstream-commits.md](upstream-commits.md#iterator-concatenation) |
| Reducing an empty iterator without an initial accumulator does not close it | v0.29.8 | [upstream-commits.md](upstream-commits.md#empty-iterator-reduction) |
| Sparse array truncation removes indices before compacting the property table | v0.29.8 | [upstream-commits.md](upstream-commits.md#array-truncation) |

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
| A string argument Intl cannot read ends the call | Unreleased | [uncatchable.md](uncatchable.md#a-string-argument-intl-cannot-read-ends-the-call) |
| Intl reads the interrupt flag in loops over a long list | Unreleased | [uncatchable.md](uncatchable.md#intl-reads-the-interrupt-flag-in-loops-over-a-long-list) |
| The functions of fmt end the call when their output does not fit | Unreleased | [uncatchable.md](uncatchable.md#the-functions-of-fmt-end-the-call-when-their-output-does-not-fit) |

## Errors and stacks

| Change | Release | Doc |
| --- | --- | --- |
| Error stacks formatted when first read | v0.26.1 | [errors-and-stacks.md](errors-and-stacks.md#error-stacks-formatted-when-first-read) |
| Endless recursion throws RangeError | v0.26.6 | [errors-and-stacks.md](errors-and-stacks.md#endless-recursion-throws-rangeerror) |

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
| Intl.Segmenter finds segments as they are read | Unreleased | [time-and-intl.md](time-and-intl.md#intlsegmenter-finds-segments-as-they-are-read) |
| The caches of Temporal formats keep short requests only | Unreleased | [time-and-intl.md](time-and-intl.md#the-caches-of-temporal-formats-keep-short-requests-only) |
| Number options are checked before they are rounded down | Unreleased | [time-and-intl.md](time-and-intl.md#number-options-are-checked-before-they-are-rounded-down) |

## Temporal

| Change | Release | Doc |
| --- | --- | --- |
| Temporal on go-intl | v0.27.0 | [temporal.md](temporal.md#temporal-on-go-intl) |

## Regular expressions

| Change | Release | Doc |
| --- | --- | --- |
| Skip to where a regexp can match | v0.26.15 | [regexp.md](regexp.md#skip-to-where-a-regexp-can-match) |
| Alternatives that cannot match are not pushed | v0.26.17 | [regexp.md](regexp.md#alternatives-that-cannot-match-are-not-pushed) |
| Compiled regexps kept per VM | v0.27.0 | [regexp.md](regexp.md#compiled-regexps-kept-per-vm) |
| Classes of regexps with the v flag parsed as the specification does | v0.28.3 | [regexp.md](regexp.md#classes-of-regexps-with-the-v-flag) |

## Strings

| Change | Release | Doc |
| --- | --- | --- |
| Searches skip to where the string is | v0.27.3 | [strings.md](strings.md#searches-skip-to-where-the-string-is) |

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

## Left out of the engine

| Change | Release | Doc |
| --- | --- | --- |
| quickjs-libc.c keeps only what the Go API calls | v0.27.0 | [trimmed.md](trimmed.md#no-shell-library) |

## Adding a change

- Put it in the doc of its area, or in a new doc here when no area fits, and add a row to the table of that area, with "Unreleased" as its release. The release step in [releasing.md](../releasing.md#before-the-release) replaces "Unreleased" with the version, in the tables here and in the Release lines of the area docs. Never write it into README.md, which only links here.
- Say what it does, why it is safe, what it gains or costs with the machine and how it was measured, and which tests check it.
- Add it to [CHANGELOG.md](../../CHANGELOG.md) and [CHANGELOG.zh-CN.md](../../CHANGELOG.zh-CN.md), and a change for speed to [performance.md](../performance.md).
