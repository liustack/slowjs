---
summary: 'Index of every change liustack/beastjs makes to the QuickJS C tree and to the Go API, by area: one line per change with the release it shipped in and the doc that has what it does, why it is safe, what it gains and which tests check it'
audience: dev
order: 11
read_when:
  - Asking why a change to the engine is safe, what it costs and what it gains
  - Looking for the doc of a change before changing the same code
  - Adding a new change, to see where it belongs
---

# Changes

The engine is QuickJS release 2026-06-04 (commit `3d5e064`, the C) with the changes below on top. Each doc below takes one area. For each change it says what the change does, why it is safe, what it gains or costs and how that was measured, and which tests check it. [upgrading.md](../upgrading.md) has the procedure of taking a new QuickJS release and [CHANGELOG.md](../../../CHANGELOG.md) what each release changed for users.

The engine is the C in `engine/`, which the generator compiles to Go assembly. A change to it is a change of the C, checked in with the generated files (see [AGENTS.md](../../../AGENTS.md)). Releases up to v0.28 ran QuickJS translated to Go by ccgo, with the changes of liustack/beastjs written into that Go by a tool that is gone now. The entries that name those releases keep the C form of their change here, and their timings were taken on that engine. [performance.md](../performance.md) has the speed of the C engine.

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
| A spread reads `Symbol.iterator` once, closes no iterator when a step of it throws, and goes on from where a getter of `next` left the Array Iterator, as the specification says | v0.30.7 | [c-engine.md](c-engine.md#a-spread-reads-symboliterator-once-and-closes-no-iterator) |
| A spread of a plain array of the realm (`[...a]`, `f(...a)`, `new C(...a)`, `super(...a)`) copies its elements to the array it builds with no Array Iterator | v0.30.7 | [c-engine.md](c-engine.md#the-spread-of-a-plain-array-with-no-iterator) |
| On amd64 every SSE instruction that needs its operand on 16 bytes reads data placed by a `PCALIGN $16`, or a local, or a global, and the generator and the post-link check refuse anything else | v0.30.5 | [c-engine.md](c-engine.md#sse-reads-that-need-16-bytes-stay-on-16) |
| A call through a pointer takes its result from registers, and a generated function that no Go code calls no longer writes its result to the Go stack | v0.30.5 | [c-engine.md](c-engine.md#results-only-in-registers) |
| `o[k]` with a string key that is its own atom is read as GETF reads `o.name`, without the generic path and without a reference to the atom | v0.30.5 | [c-engine.md](c-engine.md#a-string-key-that-is-its-own-atom-is-read-as-a-named-property) |
| A string keeps the number of the atom it is as a key, so a keyed read and `hasOwnProperty.call` take it in one read. Every string is 4 bytes larger | v0.30.5 | [c-engine.md](c-engine.md#a-string-keeps-the-atom-it-is-as-a-key) |
| Two hot calls pass fewer argument words: the interpreter's direct call of a C function and `js_call_c_hop` call `js_call_c_plain` (5 words, not 8), and the engine's own `JS_GetProperty` calls `JS_GetPropertyObj` (5, not 7). On arm64 neither passes its arguments in a block on the shadow stack any more | v0.30.6 | [c-engine.md](c-engine.md#fewer-argument-words-into-hot-calls) |
| The register loop answers `Math.clz32`, `has` and `get` of Map, Set, WeakMap and WeakSet, `Array.isArray`, `push`, `includes` and `indexOf` of an Array, `indexOf`, `includes`, `startsWith` and `endsWith` of a string, `Object.hasOwn`, `hasOwnProperty` and `String` of a string without the call | v0.30.6 | [c-engine.md](c-engine.md#built-ins-answered-in-the-register-loop) |
| `o[k] = v` with a string key that is its own atom and `o.x = v` set an own writable data property or add the key to a plain object without the generic path | v0.30.7 | [c-engine.md](c-engine.md#a-store-that-adds-a-key-to-a-plain-object-skips-the-generic-path) |
| `a[i]` and `a[i] = v` on an Array that is no longer a fast array read and write the index by its atom, without the generic path | v0.31.0 | [c-engine.md](c-engine.md#an-index-of-an-array-that-is-not-a-fast-array-is-read-and-written-by-its-atom) |
| `Math.acosh` of a value below -1 is NaN: musl's fix of `acosh` for negative inputs (commit `3e80328d`), taken as a patch of the small libc | v0.31.0 | [c-engine.md](c-engine.md#mathacosh-below--1-is-nan) |
| A sloppy function with a simple parameter list makes its arguments object only when a read needs it, and `f.apply(this, arguments)` calls `f` in the register loop with the frame's arguments, with no arguments object, no list and one level of call depth | v0.31.1 | [c-engine.md](c-engine.md#passing-arguments-on-with-apply-makes-no-arguments-object) |
| A step of a `for in` loop over an object that still has the shape the loop started from takes its key and branches back in one op of the register loop (FINEXT), without the call of `js_for_in_next` | v0.31.1 | [c-engine.md](c-engine.md#the-next-key-of-a-for-in-loop-is-taken-in-the-register-loop) |
| A read `o[k]` of the key a `for in` loop just gave, on an object of the shape the loop walks, takes the value from the slot of that key in the shape (GETELE, GETELET), without looking the key up | v0.31.1 | [c-engine.md](c-engine.md#the-read-of-the-key-a-for-in-loop-just-gave) |
| The return of a frame of more than 8 slots frees them 8 to a turn, with one poll tick a turn, where it took one slot a turn | v0.31.2 | [c-engine.md](c-engine.md#the-frame-and-the-window-of-a-call) |
| `new` of a constructor given more arguments than it has parameters runs in the register loop, with the extra arguments below its frame as a call keeps them, and starts no run of `JS_RegvmRun` | v0.31.2 | [c-engine.md](c-engine.md#the-frame-and-the-window-of-a-call) |
| A destructuring that renames its fields (`const { a: b } = f()`) reads each field of the temp itself, with no copy of the object for each field | v0.31.3 | [c-engine.md](c-engine.md#the-register-interpreter) |
| `++k` or `--k` of a local that is also read below it (`x[k] = x[++k]`) writes the local in its INC or DEC, with no copy into the local after it | v0.31.3 | [c-engine.md](c-engine.md#the-register-interpreter) |
| A chained assignment into a local (`l = x[k] = v`, `l = o.f = v`) puts the value into the local first and the element or field takes it from there, with no copies, where no closure reads the local and no catch is around | v0.31.3 | [c-engine.md](c-engine.md#the-register-interpreter) |
| A `for` or `while` loop whose test is a compare of locals, arguments, closure variables, constants or fields runs its test again at the end of the body and jumps back from there, in place of a jump to the test at the top: one dispatch less a turn, and the interrupt is still read on every jump back | v0.31.3 | [c-engine.md](c-engine.md#the-register-interpreter) |
| A full collection marks closures and arrays with no call through the class table, and the mark and the finalizer of a closure skip its closure variables when all of them are cells | v0.31.3 | `js_bytecode_function_mark_in` and `js_array_mark_in` in [quickjs.c](../../../engine/quickjs.c) |
| A read of a name (`o.x`, and `o.m` before a call) keeps in its register instruction the slot of the own data property it last found, and looks at that slot of the object's shape first: the same key there with plain flags is the property, with no lookup in the shape's hash. Nothing to invalidate, no reference held. Cost: 4 bytes of register code per read site | v0.31.3 | [c-engine.md](c-engine.md#the-register-interpreter) |
| A read of a name that finds a data property on the immediate prototype keeps its slot with the id of the object's shape (one id: the same prototype, keys and flags) and takes it while the object has a shape of that id, until the shape ids of the runtime run out once. Cost: 4 more bytes per read site, 8 with the slot, and 0.4% to 1.4% more memory in a VM after tifa's pages | v0.31.3 | [c-engine.md](c-engine.md#the-register-interpreter) |
| A write of a name (`o.x = v`) keeps in its register instruction the slot of the own key it last set, and for a write that added the key, the ids of the shape before and after the add: an object of the shape before, extensible and not exotic, takes the shape after and the value with no lookup. Such a step holds while the step count of the runtime is the one the write saw. The count moves when a shape a step was kept for is freed or moved, a prototype's keys change in place or it gains a setter or a read-only key, or the shape ids are renumbered. Nothing is held, so nothing to free. Cost: 24 more bytes per write site, and up to 1% more memory in a VM after tifa's pages | v0.31.4 | `js_shape_step_end` in [quickjs.c](../../../engine/quickjs.c), `PUTF` in [regvm-ops-basic.h](../../../engine/regvm-ops-basic.h) |
| The branches and `!` of the register loop give the truth of an object in the handler (true, unless it has [[IsHTMLDDA]]), and a temp gives its reference back there when it is not the last one: no call out of the loop. The last reference of a temp still goes to the call, which frees the object | v0.31.4 | `RV_TRUTH_OBJ` in [engine/regvm-ops-basic.h](../../../engine/regvm-ops-basic.h) |
| A read of the bytecode of a module that gets no memory for its list of requested modules, of exports or of imports fails with out of memory: the count of the list was set before its array was made, and the free of the module read the array it did not get and crashed | v0.31.7 | `JS_ReadModule` in [quickjs.c](../../../engine/quickjs.c) |
| A finished async generator settles every request queued on it, as the drain of the specification does: each `next` and `throw` settles in turn, and a `return` waits for its value, the drain going on once it settled. Only the first request after the end settled, and those behind it stayed pending for good | v0.31.7 | `js_async_generator_resume_next` in [quickjs.c](../../../engine/quickjs.c) |
| `TypedArray.prototype.slice` of a source whose resizable buffer shrank below the start while `slice` ran copies nothing and forms no pointer from the source, which then has no data (NULL): UndefinedBehaviorSanitizer reports nothing over test262 | v0.31.5 | `js_typed_array_slice` in [quickjs.c](../../../engine/quickjs.c) |
| An array or an arguments object with fast elements takes the slot kept for a key of the prototype too (`a.push`, `a.map` before a call), where an exotic object never did. No more bytes per read site | v0.31.5 | `rv_getf_keep` in [regvm-ops-basic.h](../../../engine/regvm-ops-basic.h) |
| A call of the register loop takes the register code of the function from the function object (`JSFuncObject.rv`), not from its bytecode, and finds what a call and a return read in the 48 bytes right before the code. Object literals, closures and the closure state read the constants of the bytecode from the register code | v0.31.5 | `RV_CALL_BODY` in [engine/regvm-ops-basic.h](../../../engine/regvm-ops-basic.h), `RVFunc` in [engine/regvm-func.h](../../../engine/regvm-func.h) |
| `memcmp` of the small libc compares 8 bytes at a time while 8 or more are left, where musl's loop took one, and gives the same value | v0.31.6 | [libc README](../../../engine/libc/README.md) |
| Every function of the engine whose -1 says an exception is pending is marked `__exception` (`__js_must_check` in quickjs.h), so a call that drops that answer cannot be built (`-Werror=unused-result`). Built-ins that went on after a step that ran out of memory, with the error left pending and an object missing a property, now fail with it: the `prototype` of a function, the `length` of a class and of a `String` object, the seal of a template object, the tag of a module namespace, the `message`, `cause` and `errors` of an error, a descriptor given to a proxy trap, the result of `Proxy.revocable`, the `lastIndex` of a `RegExp` made with a new target, a constructor made by the API and a global variable taken from those not yet declared | v0.31.6 | `__exception` and `js_iterator_close_throw` in [quickjs.c](../../../engine/quickjs.c), `__js_must_check` in [quickjs.h](../../../engine/quickjs.h) |
| An `await` in an async function whose PromiseResolve throws an error a script can catch, as a getter of `constructor` of the promise awaited that throws or the call depth refusing the call of that getter, throws it at the `await`, as the specification and the async generator do, so a `catch` or `finally` around it runs. The body stayed suspended at the `await` for good and its promise was rejected. A stop there still ends the body | v0.31.10 | `js_async_function_resume` in [quickjs.c](../../../engine/quickjs.c) |
| The engine runs on windows/arm64, with the arm64 code of linux and darwin and no room check: the state of the CPU that the system writes at a fault there takes 2.5 KiB, which fits the 4 KiB Go keeps below a stack, and `NewVM` refuses a machine whose state may not fit (SME, SVE longer than 256 bits) | v0.31.12 | [c-engine.md](c-engine.md#room-for-a-fault-on-windows) |

## The generator

| Change | Release | Doc |
| --- | --- | --- |
| `make gen` compiles the engine with a profile of tifa's pages, with the inlining held to what the build without a profile does | v0.30.5 | [generator.md](generator.md#a-profile-of-tifas-pages) |
| The profile is trained on plain loops (`engine/pgo/loops.js`) as well as the pages, so the paths the pages never run are not compiled as never run, and `train.sh` checks that the loops reach them | v0.30.7 | [generator.md](generator.md#a-second-input-plain-loops) |
| A register-entry leaf that saves registers or calls gets a Go declaration, so `go vet` passes on amd64 | v0.30.5 | [generator.md](generator.md#a-register-entry-leaf-with-a-frame-has-a-go-declaration) |
| The loop vectorizer no longer takes the number of turns from the profile: `memmove` is a leaf without a frame again, and a render runs 0.07% to 0.22% fewer instructions | v0.30.7 | [generator.md](generator.md#what-the-profile-decides) |
| The post-link check takes a jump table entry that reaches the end of the function's code, the entry of a `switch` value that cannot occur, when it reaches the same place in the binary | v0.31.0 | [generator.md](generator.md#a-jump-table-entry-may-reach-the-end-of-the-code) |
| The shadow-stack pass gives a phi one address of a local per incoming block, so a phi that names one block several times (cases of one `switch`) passes LLVM's verifier. The generated engine does not change | v0.31.1 | [generator.md](generator.md#a-phi-gets-one-address-of-a-local-per-incoming-block) |

## Commits of upstream QuickJS

| Change | Release | Doc |
| --- | --- | --- |
| Commits of upstream QuickJS after 3d5e064 taken into the C tree one at a time (Array slice and splice, borrowed operands of js_strict_eq2, template literals, unicode_script), and the ones left out | v0.29.0 | [upstream-commits.md](upstream-commits.md) |
| Iterator.concat follows IteratorClose and finishes after a failed step | v0.29.8 | [upstream-commits.md](upstream-commits.md#iterator-concatenation) |
| Reducing an empty iterator without an initial accumulator does not close it | v0.29.8 | [upstream-commits.md](upstream-commits.md#empty-iterator-reduction) |
| Sparse array truncation removes indices before compacting the property table | v0.29.8 | [upstream-commits.md](upstream-commits.md#array-truncation) |
| The spread of a string gives its characters | v0.30.5 | [upstream-commits.md](upstream-commits.md) |
| An append that fails for want of memory leaves the length of the array, and an element after 2^31 - 1 elements, which no script reaches, throws a RangeError | v0.31.1 | [upstream-commits.md](upstream-commits.md#an-append-that-runs-out-of-memory-leaves-the-length) |

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
| A 16-bit string goes to UTF-8 eight ASCII units at a time | v0.30.7 | [go-api.md](go-api.md#a-16-bit-string-goes-to-utf-8-eight-ascii-units-at-a-time) |
| EvalFile gives a script a file name | v0.31.0 | [go-api.md](go-api.md#evalfile-gives-a-script-a-file-name) |
| Strings a script returns or throws stop converting at the top level too | v0.31.1 | [go-api.md](go-api.md#strings-a-script-returns-or-throws-stop-converting-at-the-top-level-too) |
| TrackRejections and UnhandledRejections | v0.31.1 | [go-api.md](go-api.md#trackrejections-and-unhandledrejections) |
| More values than the engine can count fail | v0.31.1 | [go-api.md](go-api.md#more-values-than-the-engine-can-count-fail) |
| Package tifa: `tifa.Register` sets a VM up as tifa does | v0.31.1 | [go-api.md](go-api.md#package-tifa) |
| The start tag function writes img and the void elements as React DOM's `pushSelfClosing` does, and `input` and emotion's inline `style` as `pushStartInstance` does, when its seventh argument, the mode, is 1, 2 or 3 | v0.31.2 | [go-api.md](go-api.md#registerstarttag-and-registerendtag) |
| NewArray makes an array of fewer than 16,384 values in one call of the engine | v0.31.2 | [builtin.go](../../../builtin.go) |
| NewMurmur2Function: the hash of @emotion/hash in C, which package tifa gives as `__tifa_murmur2__` | v0.31.2 | [murmur2.go](../../../murmur2.go) |
| `VM.MemoryFree`: the bytes of MemorySize no block in use holds, the free slots of the arenas of small blocks and the freed blocks the heap keeps | v0.31.4 | [vm.go](../../../vm.go), `JS_GetMemoryFree` in [quickjs.c](../../../engine/quickjs.c) |
| MemoryUsage counts the register code (`RegisterCodeCount`, `RegisterCodeSize`, in MemoryUsedSize too) and has `FreeSize` | v0.31.4 | [memory.go](../../../memory.go), `compute_register_code_size` in [quickjs.c](../../../engine/quickjs.c) |
| The zero `Value` is `undefined` to `Any` and `IsUndefined` too, as it already was as an argument, as `this` and as a property value | v0.31.9 | [value.go](../../../value.go) |
| `RegisterFunc` with a nil function returns the error `NAME is not a function` | v0.31.9 | [register.go](../../../register.go) |
| A nil `*big.Int` argument is `null` | v0.31.9 | [convert.go](../../../convert.go) |
| An argument converted through JSON that runs out of memory or is stopped fails the call with that error | v0.31.9 | [convert.go](../../../convert.go) |

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
| Long text and lists of locales are worked on in checked pieces | v0.30.4 | [uncatchable.md](uncatchable.md#long-text-and-lists-of-locales-are-worked-on-in-checked-pieces) |
| The functions of fmt make a long output in checked pieces | v0.30.4 | [uncatchable.md](uncatchable.md#the-functions-of-fmt-make-a-long-output-in-checked-pieces) |
| A Go function takes the long arrays of a call from the OS | v0.30.4 | [uncatchable.md](uncatchable.md#a-go-function-takes-the-long-arrays-of-a-call-from-the-os) |
| A VM keeps its last block for the next call | v0.30.4 | [uncatchable.md](uncatchable.md#a-vm-keeps-its-last-block-for-the-next-call) |
| The copy of a spread, a rest and Object.assign reads the interrupt flag | v0.30.5 | [uncatchable.md](uncatchable.md#the-copy-of-a-spread-a-rest-and-objectassign-reads-the-interrupt-flag) |
| A VM with a memory limit keeps its block too | v0.30.6 | [uncatchable.md](uncatchable.md#a-vm-with-a-memory-limit-keeps-its-block-too) |
| The parse and the compile read the interrupt flag after a count of their work, not only where the scan of a long token meets an address: the tokens, the walks of the compile passes and of the register conversion over the code, the entries the looks for names go over, and the two loops whose work grows as a square (the jumps made shorter, the vars of an eval in a function) all count, and a long name reads the flag before its atom is made. It costs about 2% of the instructions of a compile (2.0% for the shop bundle, 2.4% for mui, with the profile trained again) and nothing in a render | v0.31.7 | `js_compile_poll`, `js_compile_tick` and `js_code_more` in [quickjs.c](../../../engine/quickjs.c), `rv_pass_stop` in [regvm-pass.h](../../../engine/regvm-pass.h) |
| The polls of a compile from its stop to its return, where it frees the code and the module it made, do not count toward the kill, as that freeing is bounded by what the compile did | v0.31.7 | `freeing_code` and `js_compile_end` in [quickjs.c](../../../engine/quickjs.c), `Poll` in [gohost.go](../../../internal/gohost/gohost.go), [go-api.md](go-api.md#error-says-why-a-call-ended) |
| The link of a module reads the interrupt flag after a count of its work: the resolution of each import and indirect export among the exports, the names of a namespace, the look for each requested module among those loaded, and the walks over the modules of the resolution, the link and the evaluation. A script that is not a module never runs that code | v0.31.7 | `js_resolve_export2`, `get_exported_names1` and `js_inner_module_linking1` in [quickjs.c](../../../engine/quickjs.c) |
| A module stopped in its link or its evaluation can be imported again: a link that failed releases the bindings it made, which a link again took a second time and leaked, the promise of a module settles with no call of its resolving function, which fails while the interrupt is pending, where it stayed pending for good after a stop in the evaluation or in the job that ended it, and a failure to make that promise no longer leaves the exception in its place. The free of the modules a stopped resolution leaves does not count toward the kill, as for a stopped compile | v0.31.7 | `js_unbind_module`, `js_evaluate_module`, `js_module_settle` and `js_freeing_start` in [quickjs.c](../../../engine/quickjs.c) |
| A module with a top-level await that a stop or running out of memory ends in a job, of its own or of an async function it awaits, is recorded as failed with the error once the host takes it, and so is every module still in its asynchronous evaluation: the job never runs again, and the module and those that wait for it stayed evaluating for good, an import of them never settling. A module that is not evaluating, or that ended, is left as it is. Their rejections call nothing, with references to the modules and contexts that the walk of them first took: the tracker of rejections is not told, as the host has the error already, and a tracker that ran the collector freed what the walk was on | v0.31.7 | `js_fail_async_modules` and `JS_GetException` in [quickjs.c](../../../engine/quickjs.c) |
| A job that leaves an uncatchable exception pending fails: `JS_ExecutePendingJob` returns -1 whatever the job returned. The job of an await, whose reaction settles no promise of its own, returned 1 with the error pending, and a host that takes the exception only on -1 never took it, nor failed the modules with it | v0.31.7 | `JS_ExecutePendingJob` in [quickjs.c](../../../engine/quickjs.c) |
| A stop or running out of memory in an async function before its first await, an async generator, a `for await` over a sync iterator, a module with a top-level await, `Promise` built-ins or the close of an iterator by `Set` methods fails the call that ran it, as everywhere else. Each place that catches an exception to settle a promise or keep it as a result ends with an uncatchable one instead, so no promise holds it. Before, the call went on with a pending or rejected promise, or a result, and the stop left pending: `JS_Eval` of a module with a top-level await after a busy loop returned the pending promise of the module, and a getter, a setter, `toJSON` or `valueOf` that started such code succeeded. A throw in the `return` of the keys iterator of `isDisjointFrom` and `isSupersetOf` is now their result, as the specification says, where it was left pending. An async function or generator that a stop ends, also in a step of an await, ends as a failure of its body does, its variables closed and its frame freed, and a stopped generator takes the request it ran for off its queue unsettled, so later calls settle. A job of a promise that gets no memory fails the call that queued it | v0.31.7 | `js_catch_ends`, `js_async_generator_stop` and `fulfill_or_reject_promise` in [quickjs.c](../../../engine/quickjs.c) |
| A module whose link ran out of memory while making its function can be linked again: it kept no code, and the link again read the function it lacked and crashed | v0.31.7 | `js_create_module_bytecode_function` in [quickjs.c](../../../engine/quickjs.c) |
| The handlers of a module with a top-level await, and the step of an `import()` after its module is evaluated, are put on their promise as the spec does, with no look for a species constructor: a script that changed `Promise.prototype.constructor` left the module evaluating for good, or the import pending, with a `TypeError` left pending after a call that did not fail | v0.31.7 | `js_execute_async_module` and `JS_LoadModuleInternal` in [quickjs.c](../../../engine/quickjs.c) |
| When the top-level await of a module ends, finding the modules that waited for it takes time linear in their number: a mark on each module in place of a search of the list found so far. 100,000 waiting modules took 2.5 s and take 0.025 s, run in the same order | v0.31.7 | `gather_available_ancestors` in [quickjs.c](../../../engine/quickjs.c) |
| Finding a loaded module by its name takes a few steps whatever the number of modules: a table by name in place of a walk of all of them, which made the load of n modules that import each other take n^2 / 2 steps and each `import.meta` as many as there were modules. The load of 2,000 modules went past 2,006,999 modules and goes past 3,364. Only the first loaded module of a name is in the table, the others of its name after it, so that many modules of one name are added in a step each, not n^2 / 2. The table costs 40 to 48 bytes a module | v0.31.7 | `js_find_loaded_module` in [quickjs.c](../../../engine/quickjs.c) |
| The engine reads the interrupt flag before it asks the host to load each module, so that a stop during the load of many modules waits for one of them: a module the host makes with no compile, of C or of bytecode, did not read it, and a stop among 2,000 such modules let up to 1,905 more load | v0.31.7 | `js_host_resolve_imported_module` in [quickjs.c](../../../engine/quickjs.c) |
| A settle of an async function or an async generator that fails on an error a script can catch, as the call depth refusing the call of a resolving function, fails the call that ran it, with that error: the call returned its promise, or a job of an await reported success, with the `RangeError` left pending | v0.31.7 | `js_settle_result` in [quickjs.c](../../../engine/quickjs.c) |
| After a stop or running out of memory that is no kill, async work that was in flight may never settle: every method of the VM works, closing it frees all it held, and a host that keeps it must not wait on such a promise. After a kill the VM is good only for Close, as before. Nothing is used after it was freed, and no call or job ends with a value and an error left pending | v0.31.7 | [api.md](../../use/api.md#errors), `TestAfterStopWhatIsSafe` in [afterstop_test.go](../../../afterstop_test.go), `test_stop_async_frees` in [unit-api.c](../../../engine/test/unit-api.c), `test_async_oom_frees` in [unit-oom.c](../../../engine/test/unit-oom.c) |
| The loops over the captures of a match read the interrupt flag: the result of exec, and the captures replace and split read from an exec result, whose length a custom exec sets | v0.31.3 | `js_regexp_exec`, `js_regexp_Symbol_replace` and `js_regexp_Symbol_split` in [quickjs.c](../../../engine/quickjs.c) |
| The matcher reads the interrupt flag in a long run forward and in its long loops: a regexp gets a goto to the next op wherever a run that reads no flag passes 4,096 ticks, and a back reference, the undo of captures and the end of a lookahead read the flag every 4,096 steps | v0.31.4 | `lre_insert_checks` and `lre_exec_backtrack` in [libregexp.c](../../../engine/libregexp.c) |
| The copy of the arguments of a call and of a spread into an array reads the interrupt flag every 4,096 elements: apply, a spread into a call, `new` or an array, `arguments`, a rest parameter and the extra arguments of a call into the register loop | v0.31.4 | `build_arg_list`, `js_append_array` and `js_append_enumerate` in [quickjs.c](../../../engine/quickjs.c), `rv_build_arguments`, `rv_build_mapped_arguments` and `rv_build_rest` in [regvm-ops-args.h](../../../engine/regvm-ops-args.h), `rv_entry_extra` in [regvm-run.h](../../../engine/regvm-run.h) |
| The scan of a regexp literal reads the interrupt flag at its end, so a stop does not wait for the hash of a long literal | v0.31.3 | `js_parse_regexp` in [quickjs.c](../../../engine/quickjs.c) |
| Intl asks Go for no memory sized by a whole list of locales before its first check of the call: the locales it canonicalizes and their seen set grow with the tags gone through | v0.31.3 | [uncatchable.md](uncatchable.md#intl-reads-the-interrupt-flag-in-loops-over-a-long-list) |
| The growth of a fast array that moves its block reads the interrupt flag before each MiB it copies, as the resize of an `ArrayBuffer` does, and a stop leaves the array as it was: the result of a `split` of 2M parts, or an array pushed to that many elements, was copied in one go, and a stop asked then waited for it, more polls than a timeout allows | v0.31.6 | `js_realloc_stop` in [quickjs.c](../../../engine/quickjs.c) |
| The case mapping of Turkish, Azeri, Lithuanian and Greek returns a text with lone surrogates as one string of UTF-16 units made in a block, where it made a list with an entry for each lone surrogate that Go grew and freed with no check: 8 Mi units ran 54 to 67 ms past the timeout on an Apple M4, 751 ms on an AMD EPYC, and were killed there | v0.31.11 | `premapCase` in [casemap.go](../../../intl/casemap.go), `TestCaseLoneSurrogatesAsPieces` in [casemap_test.go](../../../intl/casemap_test.go) |
| The canonical form of a locale tag looks up for an alias only the variants the alias tables name, and a list of locales is checked after every 16,384 units of its tags: a tag of 1,600 variants and 10 aliases took 7 ms to put in canonical form and 18 ms to maximize on an Apple M4 (0.5 and 2.5 ms now), and 64 such tags in a list held the call 44 ms with no check | v0.31.11 | `aliasVariants` in [canonical.go](../../../internal/gointl/canonical.go), `tagTurns` in [gointl.go](../../../intl/gointl.go), [tags_test.go](../../../intl/tags_test.go) |
| `Intl.Segmenter` divides a run of Chinese or Japanese with a check of the call in each pass over it, and takes the arrays of the division from blocks, where it decoded the run and made those arrays whole in Go: 8 Mi units of Chinese held the call 73 ms with no check on an Apple M4 (4.7 ms now) | v0.31.11 | `cjkEngine.divide` in [dictbe.go](../../../internal/gointl/dictbe.go), `TestCJKRunHeldPerPiece` in [runs_test.go](../../../intl/runs_test.go) |
| `Intl.ListFormat` writes a list in the javascript of intl.js from the text its patterns put around the items, which Go answers once for a formatter, or once a call for Spanish and Hebrew, where Go made a list of the items, a list of the parts and a value for each part, grown and cleared whole with no check and outside the memory limit: 1.8 GiB of Go for 8 Mi items, which ran 31 to 55 ms past the timeout on an Apple M4 | v0.31.11 | `lfLiterals` in [bridge.go](../../../intl/bridge.go), `ListFormat.Literals` in [qjs_list.go](../../../internal/gointl/qjs_list.go), `listJoin` in [intl.js](../../../intl/intl.js), [listformat_test.go](../../../intl/listformat_test.go) |
| A compare of two strings that share a prefix puts in NFD only the 32 characters before where they part, back to a normalization boundary, as the context a prefix may match, and the whole prefix only when a prefix matches up to the start of that window: it put the whole prefix of both in NFD, 0.5 GiB and 575 ms for strings of 16 Mi units that part at the end on an Apple M4 (21 ms now), and gave that memory back after a stop | v0.31.11 | `sortElementsWindow` in [collator.go](../../../internal/gointl/collator.go), [qjs_collwindow_test.go](../../../internal/gointl/qjs_collwindow_test.go) |

## Errors and stacks

| Change | Release | Doc |
| --- | --- | --- |
| Error stacks formatted when first read | v0.26.1 | [errors-and-stacks.md](errors-and-stacks.md#error-stacks-formatted-when-first-read) |
| Endless recursion throws RangeError | v0.26.6 | [errors-and-stacks.md](errors-and-stacks.md#endless-recursion-throws-rangeerror) |
| new with a spread says when its callee is not a constructor | v0.30.2 | [errors-and-stacks.md](errors-and-stacks.md#new-with-a-spread-says-when-its-callee-is-not-a-constructor) |
| Calls through call and bound functions leave no line in the stack | v0.30.2 | [errors-and-stacks.md](errors-and-stacks.md#calls-through-call-and-bound-functions-leave-no-line-in-the-stack) |
| When the OS refuses memory while `NewVM` makes the context or the shape cache of a VM, the error wraps what the OS said, as it did when the runtime could not be made | v0.31.9 | [vm.go](../../../internal/cengine/vm.go) |

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
| A Date finds its year, month and day with no loop | v0.31.1 | [time-and-intl.md](time-and-intl.md#a-date-finds-its-year-month-and-day-with-no-loop) |
| The strings of a Date are written without snprintf | v0.31.1 | [time-and-intl.md](time-and-intl.md#the-strings-of-a-date-are-written-without-snprintf) |
| A context keeps the fields of the last time value | v0.31.1 | [time-and-intl.md](time-and-intl.md#a-context-keeps-the-fields-of-the-last-time-value) |
| `Intl.NumberFormat.prototype.formatRange` and `formatRangeToParts`, on the range formatter of `internal/gointl` | v0.31.2 | [intl/intl.js](../../../intl/intl.js), `numberRangeFormatted` in [intl/bridge.go](../../../intl/bridge.go) |
| A collator compares two short Latin strings from a table of their characters, with no normal form, and `localeCompare` with no locales or options keeps one collator | v0.31.2 | [internal/gointl/qjs_collshort.go](../../../internal/gointl/qjs_collshort.go), `localeCompare` in [intl/intl.js](../../../intl/intl.js) |
| `formatToParts` gives the types of parts as atoms, which intl.js makes once from the list of Go | v0.31.2 | `partTypes` in [intl/bridge.go](../../../intl/bridge.go), `parts` in [intl/intl.js](../../../intl/intl.js) |
| In a VM with package intl registered, `Array.prototype.toLocaleString` and `%TypedArray%.prototype.toLocaleString` call the `toLocaleString` of each element with their locales and options, always two arguments, as ECMA-402 says, where QuickJS called it with none. Without Intl they pass none, as ECMA-262 says (`JS_SetLocaleArgs`) | v0.31.9 | `JS_ToLocaleStringFree` in [quickjs.c](../../../engine/quickjs.c), `vmhook.LocaleArgs` in [intl/intl.go](../../../intl/intl.go) |

## Temporal

| Change | Release | Doc |
| --- | --- | --- |
| Temporal on go-intl | v0.27.0 | [temporal.md](temporal.md#temporal-on-go-intl) |

## URL

| Change | Release | Doc |
| --- | --- | --- |
| URL and URLSearchParams on a Go parser, with the limits of the VM | v0.30.3 | [url.md](url.md#url-and-urlsearchparams-on-a-go-parser) |
| A VM keeps the last base it parsed and the last record it made, so a URL made against the same base, and a setter on the URL made last, parse nothing again | v0.31.2 | [url/bridge.go](../../../url/bridge.go) |

## Regular expressions

| Change | Release | Doc |
| --- | --- | --- |
| Skip to where a regexp can match | v0.26.15 | [regexp.md](regexp.md#skip-to-where-a-regexp-can-match) |
| Alternatives that cannot match are not pushed | v0.26.17 | [regexp.md](regexp.md#alternatives-that-cannot-match-are-not-pushed) |
| Where a regexp starts to look is worked out once, with a table of first characters and a compare of a literal prefix. Bytecode version 9 | v0.30.2 | [regexp.md](regexp.md#where-a-regexp-starts-is-worked-out-once) |
| A long list of alternatives that start with a literal goes straight to the ones that start with the character. Bytecode version 10 | v0.30.2 | [regexp.md](regexp.md#long-lists-of-alternatives-that-start-with-a-literal) |
| A regexp looks for the literal every match holds before the backtracking machine runs, and has no match when the string does not hold it. Bytecode version 13 | v0.31.1 | [regexp.md](regexp.md#a-literal-every-match-holds) |
| Only a regexp with a literal every match holds carries it: the plan of one without is 36 bytes again, where v0.31.1 gave every regexp 74. Bytecode version 14 | v0.31.2 | [regexp.md](regexp.md#a-literal-every-match-holds) |
| A regexp takes up to 32,767 capture groups, as V8 does, where upstream stops at 254: a capture index is a u16 in the header and in every op that names a group. Bytecode version 15 | v0.31.3 | [regexp.md](regexp.md#up-to-32767-capture-groups) |
| Two groups of one name in alternatives 256 bars apart, or a multiple of 256, are no longer refused as duplicates: the scope of a group name is a u32 | v0.31.3 | `is_duplicate_group_name` in [libregexp.c](../../../engine/libregexp.c) |
| Compiled regexps kept per VM | v0.27.0 | [regexp.md](regexp.md#compiled-regexps-kept-per-vm) |
| A new VM keeps 512 compiled regexps, not 256, in the same 1 MiB | v0.31.2 | `JS_REGEXP_CACHE_ENTRIES` in [engine/quickjs.h](../../../engine/quickjs.h), `DefaultRegExpCache` in [vm.go](../../../vm.go) |
| Classes of regexps with the v flag parsed as the specification does | v0.28.3 | [regexp.md](regexp.md#classes-of-regexps-with-the-v-flag) |
| A regexp that nothing changed is told by the slots of its shape and of `RegExp.prototype`, so a replace by a regexp whose flag getter a script replaced calls it | v0.31.3 | `js_is_standard_regexp` in [quickjs.c](../../../engine/quickjs.c) |
| split by a regexp that nothing changed runs the regexp forward from each cut, with no splitter and no calls of exec | v0.31.3 | `js_regexp_split_fast` in [quickjs.c](../../../engine/quickjs.c), `lre_exec_inside` in [libregexp.c](../../../engine/libregexp.c) |
| replace with a function and match by a regexp that nothing changed find their matches with no calls of exec | v0.31.3 | `js_regexp_replace_fn` and `js_regexp_match_fast` in [quickjs.c](../../../engine/quickjs.c) |
| A replace or match by a regexp that nothing changed that fails (stopped by the host, out of memory, a throw) leaves lastIndex where the generic path leaves it at the same stage: while it finds the matches, or once it has found them | v0.31.3 | `js_regexp_reset_lastIndex` in [quickjs.c](../../../engine/quickjs.c) |
| The table of first characters of a regexp knows `\s` and `\S`, so a regexp that starts with one skips to where a match can start | v0.31.3 | `lre_first_set` in [libregexp.c](../../../engine/libregexp.c) |

## Strings

| Change | Release | Doc |
| --- | --- | --- |
| Searches skip to where the string is | v0.27.3 | [strings.md](strings.md#searches-skip-to-where-the-string-is) |
| A symbol has a string of its own | v0.30.2 | [strings.md](strings.md#a-symbol-has-a-string-of-its-own) |
| The hash table of atoms holds one atom for two buckets | v0.30.5 | [strings.md](strings.md#the-hash-table-of-atoms-holds-one-atom-for-two-buckets) |
| An empty string on the left of `+` gives the string on the right, with no copy | v0.30.5 | [strings.md](strings.md#an-empty-string-on-the-left-of--gives-the-string-on-the-right) |
| The register loop adds two strings in a small function of its own, not in the generic path of add, sub and the compares | v0.30.7 | [strings.md](strings.md#two-strings-are-added-without-the-generic-path) |
| A power of two prints its shortest digits, as Node does: 54 of them printed one digit too many | v0.31.1 | [strings.md](strings.md#a-power-of-two-prints-its-shortest-digits) |
| A number that is not an integer converts to text in radix 10 by Schubfach, with integer arithmetic and a table of 617 powers of ten, not with big numbers | v0.31.1 | [strings.md](strings.md#numbers-that-are-not-integers-convert-to-text-without-big-numbers) |
| `Array.prototype.join` of a fast array of strings, ropes, `undefined` and `null` makes one block of the length and width of the result and reads the elements in place, without a property read for each | v0.31.2 | `js_join_plan` in [engine/quickjs.c](../../../engine/quickjs.c) |
| The search for a needle of more than 8 characters keeps a table of 256 one-byte shifts and moves over a run of windows in one tight loop, checking the step counter as often as before | v0.31.2 | `js_string_two_way` in [engine/quickjs.c](../../../engine/quickjs.c) |
| A string that is no atom, made a key, keeps the atom it found, with a reference, so later keyed reads by that string skip the hash and the search of the atom table | v0.31.3 | `js_string_to_atom` in [engine/quickjs.c](../../../engine/quickjs.c) |
| The register loop adds two flat 8-bit strings into a temp (the left one of 1 to 8192 characters, the right one of 1 to 512) itself: it grows the left one in place or copies both, as `JS_ConcatString` does, without its three calls | v0.31.4 | `rv_concat8` in [engine/regvm-ops-basic.h](../../../engine/regvm-ops-basic.h) |
| A keyed read (`o[k]`) by a string that keeps no atom looks it up in the atom table without making one: found, the string keeps the atom as a key made from it does. Not found, no object has that name, so the read is undefined through a chain of ordinary objects and fast arrays, with no atom made and freed. An exotic object on the chain, an index, a long key and a string that starts with a digit take the generic path | v0.31.4 | `rv_key_lookup` in [engine/regvm-ops-object.h](../../../engine/regvm-ops-object.h) |

## JSON

| Change | Release | Doc |
| --- | --- | --- |
| JSON copies runs of plain characters | v0.26.17 | [json.md](json.md#json-copies-runs-of-plain-characters) |
| A string that `JSON.stringify` wrote with no escape keeps a mark, a bit of its header, so the next write of the same string, as a value or as a key, copies it without looking at its characters. A marked string is copied, not changed in place, when something is appended to it | v0.31.4 | `JSString.plain` in [engine/quickjs.c](../../../engine/quickjs.c) |
| `JSON.stringify` with no replacer and no indent writes plain objects and arrays from their shapes and elements, and leaves every value that may run code to the general path, which takes over the rest of each open object before such code runs. The check of a cycle is a C array, not a JavaScript one | v0.31.3 | `json_walk_object` in [engine/quickjs.c](../../../engine/quickjs.c) |
| `JSON.parse` with a reviver reads the length of an array, also a Proxy of one, as ToLength and counts with an int64_t. Upstream still reads it as ToUint32 | Unreleased | [json_reviver_length.js](../../../engine/test/compare/json_reviver_length.js) |

## Operators

| Change | Release | Doc |
| --- | --- | --- |
| Bitwise operators on floats skip the slow path | v0.26.17 | [operators.md](operators.md#bitwise-operators-on-floats-skip-the-slow-path) |
| The usual comparisons answered in the interpreter | v0.26.17 | [operators.md](operators.md#the-usual-comparisons-answered-in-the-interpreter) |
| `instanceof` of an ordinary function answers without calling `Symbol.hasInstance` | v0.31.0 | [operators.md](operators.md#instanceof-without-the-call-of-symbolhasinstance) |
| Add and sub of two numbers without the shared tail | v0.31.1 | [operators.md](operators.md#add-and-sub-of-two-numbers-without-the-shared-tail) |

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
| An object rest leaves out the keys of its pattern by a list the parser makes, and is made at its final shape from a cache of the shapes of its sources | v0.30.5 | [objects-and-shapes.md](objects-and-shapes.md#an-object-rest-is-made-at-its-final-shape) |
| The spread of an object literal copies a source of a shape it has seen into its target from a cache of the shapes of both | v0.30.5 | [objects-and-shapes.md](objects-and-shapes.md#a-spread-copies-from-a-cache-of-the-shapes-of-its-target-and-source) |
| A field of a property descriptor that throws, or a `has` trap that throws, ends the conversion with its exception | v0.30.7 | [objects-and-shapes.md](objects-and-shapes.md#a-field-of-a-property-descriptor-that-throws-ends-the-conversion) |
| bind with too little memory fails with the out of memory error, and no longer crashes or returns a bound function without its length or name | v0.30.7 | [objects-and-shapes.md](objects-and-shapes.md#bind-with-too-little-memory-fails-whole) |
| `Object.defineProperty` and the calls that share its conversion read a descriptor that is a plain object from its shape in one pass | v0.30.7 | [objects-and-shapes.md](objects-and-shapes.md#objectdefineproperty-reads-a-plain-descriptor-in-one-pass) |
| `Function.prototype.bind` reads the length and name of a plain target from its shape and gives the bound function the shape of a new function at once | v0.30.7 | [objects-and-shapes.md](objects-and-shapes.md#bind-reads-the-length-and-name-of-a-plain-target-from-its-shape) |
| A `for in` loop makes no shape: the context holds the empty shape with no prototype its iterator starts from | v0.31.0 | [objects-and-shapes.md](objects-and-shapes.md#the-iterator-of-a-for-in-loop-finds-its-empty-shape) |
| `Object.isFrozen` and `Object.isSealed` ask first whether the object is extensible, as the specification says, and no longer leave the list of keys in use when the `isExtensible` trap of a proxy throws | v0.31.1 | [objects-and-shapes.md](objects-and-shapes.md#objectisfrozen-and-objectissealed-ask-first-whether-the-object-is-extensible) |
| Defining a property of a module namespace reads its binding first and throws a `ReferenceError` while the binding is not initialized | v0.31.1 | [objects-and-shapes.md](objects-and-shapes.md#defining-a-property-of-a-module-namespace-reads-its-binding-first) |
| `Object.isFrozen`, `Object.isSealed`, `Object.freeze` and `Object.seal` of an ordinary object or an array go over its shape once, with no list of keys and no lookup of each | v0.31.1 | [objects-and-shapes.md](objects-and-shapes.md#objectisfrozen-objectissealed-objectfreeze-and-objectseal-go-over-the-shape) |
| A shape holds at most 2^26 - 1 properties, so that a lookup cannot miss one past that | v0.31.1 | [objects-and-shapes.md](objects-and-shapes.md#a-shape-holds-at-most-226---1-properties) |
| An element added to an array is refused, or fails, before the array changes: a refused store does not throw the RangeError of a conversion, and an element that fails leaves the length | v0.31.1 | [objects-and-shapes.md](objects-and-shapes.md#an-array-element-is-refused-before-the-array-changes) |
| `Array.from` on a constructor passes the mapping function the whole index and sets the whole length | v0.31.1 | [objects-and-shapes.md](objects-and-shapes.md#arrayfrom-on-a-constructor-passes-the-whole-index-and-length) |
| A define that changes the flags of a property that is there makes the shape the object's own before it changes anything, so a define that runs out of memory leaves the property, the length of an array included. A refused define copies nothing, and shortening an array past index 2^31 - 1 makes no names | v0.31.1 | [objects-and-shapes.md](objects-and-shapes.md#a-define-that-changes-the-flags-of-a-property-fails-before-it-changes-anything) |
| A shape that an object finds for its next key keeps the shape the object left, so that the objects of a constructor find every shape the objects before them went through | v0.31.1 | [objects-and-shapes.md](objects-and-shapes.md#objects-of-a-constructor-find-the-shapes-the-objects-before-them-went-through) |
| `new f()` takes the prototype of the object it makes from the own `prototype` of `f` in its shape when that is a plain data property that holds an object, without the generic read, and reads the interrupt flag once, as a call does | v0.31.2 | [objects-and-shapes.md](objects-and-shapes.md#objects-of-a-constructor-find-the-shapes-the-objects-before-them-went-through) |
| A closure is one block: the function object, its properties and its closure variables, where it took three | v0.31.3 | `js_new_function_object` in [quickjs.c](../../../engine/quickjs.c) |
| `set_name` gives a closure with no name of its own the name in place, without the generic define | v0.31.3 | `js_object_name_in_place` in [quickjs.c](../../../engine/quickjs.c) |
| `Object.getOwnPropertyDescriptor`, Reflect's and `Object.getOwnPropertyDescriptors` make each descriptor at its final shape, which the context keeps for the data form and for the accessor form, and fill its four values, with no define of each key | v0.31.4 | `js_new_desc_object` in [quickjs.c](../../../engine/quickjs.c) |

## Memory

| Change | Release | Doc |
| --- | --- | --- |
| One copy of source text and line tables for every VM, SharedBytecode | v0.28.1 | [memory-and-allocation.md](memory-and-allocation.md#one-copy-of-source-text-and-line-tables-for-every-vm) |
| Shared bytecode is scanned for the polls on its first load only | v0.29.1 | [memory-and-allocation.md](memory-and-allocation.md#shared-bytecode-is-scanned-for-the-polls-on-its-first-load-only) |
| A large block of a built-in collects first when it would pass the memory limit | v0.30.5 | [memory-and-allocation.md](memory-and-allocation.md#a-large-block-collects-first-when-it-would-pass-the-memory-limit) |
| A request the heap refuses lets the kept shapes go before it is tried again, so a VM near its limit fits what it would with the ring empty | v0.31.4 | `js_shape_cache_drop` in [quickjs.c](../../../engine/quickjs.c) |
| The shape cache holds the prototype of the shapes it keeps and follows `JS_SetClassProto` of its own context only, so letting kept shapes go runs no finalizer, and it lets them go from a ring taken out first, so a finalizer that sets the cache again finds it whole | v0.31.6 | `js_shape_cache_proto` in [quickjs.c](../../../engine/quickjs.c) |
| A write of a name that takes the shape its step keeps for an object whose properties must move holds the shape before it asks for the new property array: a refused request let the kept shapes go and the object took a freed shape, which later aborted the engine or corrupted another shape | v0.31.8 | `rv_putf_step` in [regvm-ops-basic.h](../../../engine/regvm-ops-basic.h) |
| The collection a large block of a built-in runs before it would pass the memory limit makes the next ones wait when it leaves the heap near the limit, for 1, 2, 4 and up to 1024 such blocks, and to the next call of the host at most, so a VM that stays within 8 KiB of its limit no longer collects at every array that grows | v0.31.9 | `js_trigger_gc_block_run` and `JS_StartCall` in [quickjs.c](../../../engine/quickjs.c), `enter` in [entry.go](../../../entry.go) |

## Young collection

| Change | Release | Doc |
| --- | --- | --- |
| Young collection, VM.RunYoungGC | v0.27.3 | [young-collection.md](young-collection.md) |
| A full collection frees after its garbage an object whose last reference a finalizer of a class of the host let go of, as a young collection does, where it left the object listed with a count of 0 and a WeakRef to it freed its block while it was still listed | v0.31.5 | [young-collection.md](young-collection.md) |
| A full collection moves the young list to the old one after its weak pass, so an object that a finalizer of a host class makes there is counted like any other, where held by an old object it left a count on its children that kept the runtime from being freed | v0.31.5 | [young-collection.md](young-collection.md) |
| Young objects carry a young bit of their own, so the young collection needs no pass to mark them and takes away the counts and finds the candidates in one pass over the young list, where it took three | v0.31.5 | `JS_RunYoungGC` in [quickjs.c](../../../engine/quickjs.c) |
| A collection clears the WeakRefs to its garbage before it frees any of it, so a finalizer of a class of the host that calls `deref` while the garbage is freed gets `undefined`, where it could get an object being freed | v0.31.6 | `gc_clear_weak` in [quickjs.c](../../../engine/quickjs.c) |
| A weak map record whose key and value are young holds its value in a young collection only once the scan keeps its key, as in a full one, so a key that only its value holds goes in the next young collection, where it stayed until a full one | v0.31.6 | `young_weak_alloc` in [quickjs.c](../../../engine/quickjs.c) |

## Weak collections

| Change | Release | Doc |
| --- | --- | --- |
| WeakMap ephemeron collection | v0.27.2 | [weak-collections.md](weak-collections.md) |
| A WeakRef keeps its target to the end of the job, and the host ends the job when a call returns to Go | v0.30.2 | [weak-collections.md](weak-collections.md#a-weakref-keeps-its-target-to-the-end-of-the-job) |

## Left out of the engine

| Change | Release | Doc |
| --- | --- | --- |
| quickjs-libc.c keeps only what the Go API calls | v0.27.0 | [trimmed.md](trimmed.md#no-shell-library) |

## The command

| Change | Release | Doc |
| --- | --- | --- |
| `beastjs`, a command that runs a JavaScript file in a VM set up as tifa sets it up, with `print`, `console.log` and a timeout and memory limit that hold, prebuilt in the GitHub release of each tag for the five targets of v0.31.0, and for windows/arm64 too from v0.31.12 | v0.31.0 | [command.md](../../use/command.md) |

## Adding a change

- Add one row to the table of its area, or a new area table when none fits, with "Unreleased" as its release. The release step in [releasing.md](../releasing.md#before-the-release) replaces "Unreleased" with the version, in the tables here and in the Release lines of the area docs. Never write it into README.md, which only links here.
- The row says what the change does and links to where it is described. The reasons, the numbers and the tests live in code comments and commit messages. The longer write-ups under this folder are older and stay as they are.
- Add it to [CHANGELOG.md](../../../CHANGELOG.md) in one or two sentences, and a change for speed to the table of [performance.md](../performance.md) (one row per release).
