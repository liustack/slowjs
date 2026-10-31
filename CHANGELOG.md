# Changelog

What changed in `github.com/liustack/slowjs` (named `github.com/liustack/quickjs` up to v0.27.3) that programs using it can see. Each release is tagged on `main`: a patch version, or the next minor version when it removes or changes a public API. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). [Fork changes](docs/changes/README.md) has the details of each change, and [Performance](docs/performance.md) how each speedup was measured. Times are for a server render of a page of tifa's example shop, on linux/amd64 (AMD EPYC) unless an entry says otherwise.

## [v0.29.6] - 2026-10-31

### Performance

- A call from the generated engine to another of its functions takes the result from the registers the callee returns it in, where it read it back from the Go stack. Calls through a pointer, calls to Go code (the host's callbacks) and calls to a function that failed to generate keep the read, and the check of the linked binary holds every call that skips it to a callee that stores its result from those registers and then runs only Go's epilogue. A render runs 0.37% (the shop workbench) to 0.65% (MUI) fewer instructions on the pages of tifa's example apps and 0.42% fewer on `r_react_ssr`, with the same HTML on linux/amd64 and darwin/arm64. On linux/amd64 the shop workbench took 0.978 of the time, the store 0.980, MUI 0.984, the bench pages 0.995 and `r_react_ssr` 0.973 (4 code positions, 3 rounds, all under 3%), and on darwin/arm64 0.990 to 0.997. The generated assembly is 1.9% (amd64) and 2.1% (arm64) smaller ([Performance](docs/performance.md#what-the-generated-code-costs-against-native-c)).
- The compiler drops the checks of the temporal dead zone that cannot fail: a read or a write of a `let`, `const` or `class` local after its declaration, in code that no jump can reach without going through the declaration, is a plain `get_loc` or `put_loc`, and a local left with no check, no capture and no `eval` in its function is no longer set uninitialized when its block starts, unless a loop or a `finally` runs that block again. A case of a `switch`, a read in a closure or before the declaration, the head of a loop, and code after a `catch` or a `finally` keep their checks, and every `ReferenceError` is thrown as before. A render runs 0.61% fewer instructions on the shop workbench, 0.34% on the store, 0.77% on MUI, 0.11% on the bench pages and 0.06% on a render of `r_react_ssr`, with the same HTML. The compile of a bundle takes a little longer (0.66 M instructions for that of `r_react_ssr`), which a VM of tifa does not pay. On linux/amd64 the shop workbench took 0.995 of the time, MUI 0.997 and the bench pages 0.990 (4 code positions, 3 rounds, all under 3%) ([Performance](docs/performance.md#checks-of-the-temporal-dead-zone)).

## [v0.29.5] - 2026-10-31

### Changed

- Bytecode is version 7, which has opcodes for a read of a local and the property read after it, for the case of a `switch` on a string and for a `typeof` test: bytecode written by an earlier release no longer loads (`SyntaxError: invalid version`), and has to be compiled again. `JS_ReadObject` rejects a fused opcode that is not followed by the instructions it stands for.
- `===`, `!==`, `==`, `!=` and `switch` on two equal strings longer than 65536 characters, or on a rope, and the lookup of a long string or rope key in a `Map` or `Set` that holds an equal one, read the stop every 65536 characters, so they stop within a few polls of an `Interrupt` or the eval timeout. They compared the whole strings in one call, about 3,300 polls (10 ms) for two strings of 32 million characters, and the script was killed when the strings were long enough. The same string or rope is now equal to itself at once.
- A string of more than 65536 characters used as a property key (`o[s] = 1`, `s in o`, `o[s]`, a key of `JSON.parse`) reads the stop every 65536 characters while its hash is made and while it is compared with the atom of an equal string. `JSON.parse` reads it every 65536 bytes of a string of the text, in the key or in a value. Each of them took one pass over the whole string, about 800 polls for 32 million characters (the hash), 3,300 more for the comparison with an equal atom, and 4,000 for the scan of a JSON string, with no stop in them.
- `indexOf`, `lastIndexOf`, `includes`, `startsWith`, `endsWith`, `replace`, `replaceAll` and `split` with a pattern as long as what is left of the string, `Array.prototype.indexOf`, `lastIndexOf` and `includes` and `Object.is` on two equal strings of more than 65536 characters, and `JSON.stringify` of a long string read the stop every 65536 characters. They compared or copied the whole string in one call, about 3,400 polls for 32 million characters.
- `JSON.parse` of a long string reads the stop every 65536 bytes of the whole string, escapes included: a string of nothing but escapes, or of runs of 32768 characters between escapes, was read to the end before. The text is made a C string in steps, a character above 0xff after a long run of ASCII widens the characters before it in steps (so do `padStart` and `padEnd` with such a filler), and a string that holds more than 1 MiB is copied in steps of 1 MiB when it grows.
- The hash of a long key and the compares with the keys of a Map, a Set or an object that have its hash and its length share one budget, and a compare that ends at a difference counts what it did. 131071 characters of hash and compare read the stop after 65536 each time, not 131071, and a chain of keys that differ from the key asked for late in the string is read as often as one of keys that are equal.
- A string of exactly 65536 characters is now read in steps like a longer one: `===` and `!==`, a key of a `Map` or `Set`, `Array.prototype.indexOf`, `lastIndexOf` and `includes`, and a property key. They counted 65536 as short and compared or hashed it in one call.
- A loop that compares many strings shorter than 65536 characters adds the compares up and reads the stop each time the sum passes 65536 characters: `Array.prototype.indexOf`, `lastIndexOf` and `includes` on an array of thousands of strings of one length, and a `Map` or `Set` lookup among keys of one hash and one length. An array of 16384 strings of 65535 characters, searched for one that differs in the last character, made about a billion character compares before it read the stop once.
- `JSON.parse` and `JSON.stringify` read the stop every 32768 characters of a run of 8-bit characters that is copied into a buffer of 16 bits (after a character above 0xff), since each character is looked at and then copied. A buffer of more than 1 MiB that has to grow reads the stop before the first step of its copy. It is copied into a new block, so two blocks are alive for the time of the copy (the allocator could extend a large block without a second one). The limit of the memory is checked as `realloc` checks it, against the memory less the old block, so a buffer that fit the limit before still grows, unless the footprint limit or the host, which may refuse a second block while the old one is there, stops it first. The memory in use can be higher for the time of the copy by about the old block, and that is not an upper bound.

### Fixed

- `GetProperty`, `GetPropertyValue`, `SetProperty`, `SetPropertyValue` (and the `Value` methods that call them), `Value.MarshalJSON`, `Value.Any` and `ErrorFromValue` run inside the eval timeout, as `Eval` and `Call` do. A getter, a setter, a proxy trap, `toJSON`, a replaced `BigInt.prototype.toString` or a `toString` that reads an error could run for as long as it liked, since these methods started no timeout. With a 10 ms timeout a getter of 200 ms returned 42 and no error. `SetProperty` and `SetPropertyValue` now return the exception that the setter threw, where they returned `failed to set property` and left the exception pending ([details](docs/changes/uncatchable.md#every-method-that-can-run-javascript-has-the-eval-timeout)).
- 23 fixes of upstream QuickJS are in the engine, each with a test that prints what Node 24 prints (the lines that follow the current specification are marked): `BigInt.asUintN` of a negative BigInt, `Iterator.prototype.take` and `drop` limits, `''.repeat(2 ** 31)`, `ArrayBuffer.prototype.resize` and `TypedArray.prototype.at` with a conversion that resizes the buffer, the completion value of `try`, `return()` of iterator helpers, `yield` and `await` as names, `f() = 1` in sloppy code, `Promise.try`, `export *` of a binding imported and exported again, `super.x = v` on a non-extensible receiver, the name of a function in a for-in head, the prototype of a bound function, the getOwnPropertyDescriptor and ownKeys traps of a proxy, the order of `Object.defineProperties`, class elements after `get`, `set` and `static`, `Array.prototype.sort` writing back every element, `for await` waiting for `return()` when it leaves early, and string names in exports. Two leaks are gone too: a failing `Atomics.store` and the read of cut-short bytecode with a BigInt. A sealed or frozen array keeps its length when an index past its end is set or defined. test262 has 25 fewer known failures (33 are left). The list and where the C tree differs from upstream or Node 24 are in [upstream-commits.md](docs/changes/upstream-commits.md).
- A `delete` that packs the properties of an object (after 8 or more keys are deleted, at least half of them) no longer leaves an "out of memory" error pending when the VM has no memory for the smaller property list. The delete had worked, but the uncatchable error stayed in the VM and took the place of the next error a script threw. Packing is optional now: the delete works, and the object is packed by a later delete.

### Performance

- The end of a call of a bytecode function skips the values with no reference count, eight at a time, where it looked at them two at a time. A render runs 0.7% to 2.1% fewer instructions on the pages of tifa's example apps and 1.3% to 2.0% fewer on the render scripts, with the same HTML. The 200 locals of a function, none set, cost 37% fewer instructions to call. The one case found slower is a function where every fourth local holds an object: 2.4% more instructions with 200 locals ([Performance](docs/performance.md#the-end-of-a-call-and-the-start-of-one) has the numbers).
- A call of a bytecode function sets its locals to `undefined` eight at a time, where it set four. A render runs 0.2% to 0.5% fewer instructions on the pages of tifa's example apps and on the render scripts, with the same HTML. The locals of a function with 200 locals cost 14% fewer instructions to set and free than with the change above, and a call of a function with 3 locals or fewer takes 1 to 4 instructions more.
- Object literals of the same keys, written in different places, share one shape, as objects built key by key do. The first object of a literal looked for its shape from the empty shape of `Object.prototype`, which the first object that got a key had usually changed in place, so each place made a shape of its own. A VM of tifa's pages holds 12% to 15% fewer shapes, and 0.7% (the shop and the MUI pages) to 1.7% (the store pages) less memory after a render. Renders run the same instructions ([objects-and-shapes.md](docs/changes/objects-and-shapes.md#object-literals-of-the-same-keys-share-one-shape)).
- A read of the first or second argument, of the first local or of a local past the fourth, followed by a read of a property by name (`o.x`), and the case of a `switch` on a string, take one turn of the interpreter. A render of the six workbench pages of tifa's example shop runs 1.85% fewer instructions, the store pages 1.59%, the MUI pages 1.66% and the bench pages 3.41%. The bench pages take 3.4% less time to render (8 code positions, 3 rounds, [c-engine.md](docs/changes/c-engine.md#opcodes-that-do-a-few-instructions-at-once), [Performance](docs/performance.md#opcodes-that-do-a-few-instructions-at-once)).
- `typeof x == "string"` and the like, with `==`, `===`, `!=` or `!==` and any type name, and `typeof x < "u"` and `typeof x > "u"` as minifiers write them, take one turn of the interpreter. A render of tifa's pages runs 0.63% (the shop) to 1.42% (the bench pages) fewer instructions.

## [v0.29.4] - 2026-10-30

### Performance

- An object literal of plain keys keeps its properties in the block of the object, not in a block of its own. A VM of tifa's pages holds 1.0% to 2.4% less after a render, a render runs 0.1% to 0.8% fewer instructions, and no page renders faster or slower by the rule (the shop 0.991 of the time, the MUI pages 0.998, the bench app 0.985, React 0.995). The memory one render adds with the collector off is 0.7% more on average ([Performance](docs/performance.md#object-literals-with-their-properties-in-the-block-of-the-object) has the numbers).
- The interpreter calls a built-in function written in C itself, without first entering the function that runs bytecode, which did no work of its own for such a call. A render of the six workbench pages of tifa's example shop runs 0.74% fewer instructions, its bench pages 2.3% fewer, and the pages took 0.98 to 0.99 of the time, within the noise of the code position. A script recurses exactly as deep as before, also through built-in functions such as `call`, `map` and `sort` ([Performance](docs/performance.md#c-functions-called-from-the-interpreter)).

## [v0.29.3] - 2026-10-30

### Added

- `VM.RegisterCopyProps(name)` defines a function named `name` on the global object that does the work of the loop with which React's `createElement` and `cloneElement` copy the keys of their config into the props of an element, written in C inside the engine. It copies where its result and the loop's cannot differ in anything a script can see (an ordinary config of at most 32 keys with plain values, props of the same kind, an unchanged `Object.prototype` and `Function.prototype.call`) and answers `false`, having changed nothing, for any other config, which the script then gives to the loop. The engine puts nothing on the global object by itself: the function is in a VM only when its host registers it. In tifa, which uses it for React, a render runs 2.1% to 4.1% fewer instructions, and the pages of its example shop render in 0.988 and 0.989 of the time and its MUI pages in 0.980. ([Performance](docs/performance.md#the-copy-of-the-props-of-a-react-element-in-c), [Go API](docs/changes/go-api.md#registercopyprops))

### Changed

- Loops of the engine that walk the items of a string, an array, a Map or a Set and call nothing else for each one stop within a few polls of an `Interrupt` or the eval timeout: `Object.fromEntries`, the spread of an iterable (`[...set]`), `Iterator.prototype.toArray` and `drop`, `Math.sumPrecise`, `Set.prototype.union`, `symmetricDifference` and `isSupersetOf`, and `Array.from`, the typed array constructors and `from`. The engine read the stop when a call started, and the `next` of these iterators is called without one, so a script that spread a Set of two million items, or took the union of two large Sets, ran on until it was killed. The copy of a Set that its methods make before they add to it stops too.
- `replace` and `replaceAll` with a string for the pattern look for it as `indexOf` does, and read the stop every 1024 matches. They used to compare the needle at each index where its first character was, so a needle that almost matches at every index (a hundred `a` and a `b` in a string of a million `a`) took the product of the two lengths, and a `replaceAll` with millions of matches ran on until it was killed. `trim`, `slice`, `substring`, `substr` and the pieces `split` makes stop within a million characters when the piece they cut out is longer than that, and so do `padStart` and `padEnd` with a filler of tens of millions of characters.
- `Map` and `Set` operations (`get`, `set`, `has`, `delete`, `getOrInsert`, `add` and the methods of `Set`) stop within a few thousand characters of an `Interrupt` or the eval timeout when the key is a long string or a rope. Hashing the key walks all of it, tens of milliseconds for ten million characters, and got the script killed.

## [v0.29.2] - 2026-10-30

### Added

- `VM.RegisterEscapeText(name)` defines a function named `name` on the global object that escapes text for HTML as `escapeTextForBrowser` does in React DOM's string renderer, written in C inside the engine. Its argument becomes a string as in `"" + text` (an object goes through `valueOf` and `toString`, a symbol throws), and each of `&`, `<`, `>`, `"` and `'` becomes `&amp;`, `&lt;`, `&gt;`, `&quot;` or `&#x27;`. A string with none of them comes back as the same string. The engine puts nothing on the global object by itself: the function is in a VM only when its host registers it. In tifa, which uses it for React's escape, a render runs 0.7% to 4.5% fewer instructions, and the pages of its example shop render in 0.974 of the time and its bench pages in 0.938. ([Performance](docs/performance.md#react-doms-text-escape-in-c), [Go API](docs/changes/go-api.md#registerescapetext))

### Changed

- `VM.RunGC` sets the GC threshold to 1.5 times what it left, as the GC the engine starts by itself does. It used to leave the threshold where it was, so when a script had grown the heap past it with no object made (array elements do), the first object of the next call started another full GC as long as the first one. A threshold of `^uintptr(0)`, which turns the engine's own GC off, stays. A program that sets the threshold itself after `RunGC`, as tifa does, sees no change.
- The spread of an object (`{...o}`), the rest of a destructuring and `Object.assign` look up each key of the source when its turn comes, as the specification says. A getter that deletes a later key, or makes it not enumerable, now leaves it out of the copy, where it was copied before: as `undefined`, or with the value the prototype has under that name. A getter that makes a later key enumerable now has it copied, where it was left out before. `Object.keys({ ...{ get a() { delete this.b; return 1 }, b: 2, c: 3 } })` is `["a", "c"]`, as in Node.
- `isWellFormed`, `toWellFormed`, `split`, `padStart`, `padEnd`, `trim`, `trimStart`, `trimEnd`, `anchor`, `fontcolor`, `fontsize`, `link`, `escape`, `unescape`, `encodeURI`, `encodeURIComponent`, `decodeURI`, `decodeURIComponent`, `Array.from`, the typed array constructors and `from`, making a rope (a long string joined from parts without copying them) into one string and `<` on two ropes stop within a few thousand characters or elements of an `Interrupt` or the eval timeout. On a string of tens of millions of characters, or as many elements, they ran on to the end, long enough to get the script killed. `===` on two ropes still reads no flag, as on two other strings.

## [v0.29.1] - 2026-10-30

### Performance

- An ordinary object (an object literal, `{}`, the props of a React element) takes 48 bytes of memory, not 72. The highest `MemorySize` of a render of a long Markdown page is 3.8% lower on average, and the 20 pages of the example shop hold 1.8% less at the end of a render. No page renders faster or slower ([Performance](docs/performance.md#ordinary-objects-of-48-bytes) has the numbers).
- `EvalSharedBytecode` loads a bundle in 0.79 (the React page bundle) to 0.86 (a tifa shop bundle) of the time after the first load of the same `SharedBytecode`: the engine checks the code of the bytecode against its rule of the polls once for each `SharedBytecode`, not once for each VM ([Performance](docs/performance.md#what-a-new-vm-and-loading-a-bundle-cost)). `EvalBytecode` scans every time, as before.
- An object literal of plain keys gets all its keys at once, with the shape its function keeps for it and a property array of its exact size. Without PGO on linux/amd64, at 8 code positions with the same HTML, the workbench pages of tifa's example shop render in 0.956 of the time, the MUI pages of tifa's test app in 0.969 and the table page of its bench app in 0.942, and a React server render does not change (0.991). A VM of the shop holds about 0.17 MiB more, and a long Markdown page takes 1.6% less at its peak. [objects-and-shapes.md](docs/changes/objects-and-shapes.md#object-literals-made-at-their-final-shape) has the details.
- A call of a bytecode function sets its locals to `undefined` four at a time and frees its values two at a time. Without PGO on linux/amd64, at 8 code positions with the same HTML, the table and spiral pages of tifa's bench app render in 0.958 and 0.967 of the time. The workbench pages of tifa's example shop (0.996, 2.5% fewer instructions), the MUI pages of tifa's test app (0.999) and a React server render (0.984, 3.1% fewer instructions) do not change by the rule. A loop of calls to a function with no locals runs 3% more instructions. [c-engine.md](docs/changes/c-engine.md#how-a-call-sets-and-frees-its-values) has the details.
- A `for in` loop over an ordinary object holds the shape of the object instead of a list of its keys, and `Object.prototype.hasOwnProperty.call(obj, key)` is answered in the interpreter without calling `call` and `hasOwnProperty`. On top of the change above, the shop renders in 0.984 of the time (2.5% fewer instructions), the MUI pages in 0.981, the spiral page of the bench app in 0.977 and React in 0.988: faster at every position of the shop and the MUI pages, but by less than the rule asks. With both changes the shop takes 0.982 of the time of before, the MUI pages 0.976, the bench pages 0.952 and React 0.969, faster at every position. Memory does not change. [objects-and-shapes.md](docs/changes/objects-and-shapes.md#a-for-in-loop-holds-the-shape-of-its-object) has the details.

### Changed

- The symbols `intl.Register` and `temporal.Register` put on the global object to mark a VM as set up are `Symbol.for("github.com/liustack/slowjs/intl")` and `Symbol.for("github.com/liustack/slowjs/temporal")`, where they were named after modernc.org/quickjs.
- A script is killed only when it has also kept the engine at work after its eval timeout or an `Interrupt`: 1000 polls of the engine since it was asked to stop, besides the 50 ms. Time in which the engine does no work, in a Go function the script called or while the OS or the Go scheduler runs other threads or goroutines, no longer gets a script killed that would have stopped by itself: the call returns the interrupted error and the VM goes on. On a loaded machine a script that stopped in under a millisecond of work could be killed before.
- `String.prototype.normalize`, `localeCompare`, `toUpperCase` and `toLowerCase` stop within a few thousand characters of an `Interrupt` or the eval timeout. On a long string they ran on until they had turned the whole string into code points, compared it or changed its case.
- Bytecode is version 6, which holds the keys of object literals: bytecode written by an earlier release no longer loads (`SyntaxError: invalid version`), and has to be compiled again.

## [v0.29.0] - 2026-10-29

### Changed

- **Breaking:** the engine is the C engine, the only one. It is QuickJS in C (`engine/`) compiled by LLVM to Go assembly that the module ships, so building needs no C compiler and no cgo. Until v0.28 the engine was QuickJS translated to Go by ccgo, and that engine is gone. Results, error stacks and HTML stay the same, with the differences below.
- **Breaking:** the module builds for linux/amd64 and darwin/arm64 only. A build for any other target, windows/amd64, darwin/amd64 and linux/arm64 included, stops with an error that names these two.
- **Breaking:** a script nests calls by a count, not by the stack. `SetMaxStackSize` takes calls: 5000 by default and 10000 at most, where the Go engine counted 10000 slots of stack. A script that recursed past about 5000 calls gets its `RangeError` sooner. JSON and parentheses nest 2000 and 665 levels at most (tifa's `/json/2001` page answers 500). Chains that crashed the engine, such as 300000 bound functions, now throw a `RangeError`.
- A script that runs on 50 ms past its eval timeout or an `Interrupt`, in a Go function it called for example, is killed. The call returns an interrupted `*Error` that also matches `ErrKilled`, and the VM is good only for `Close`.
- `MemorySize` and the memory limit count what the engine's own allocator holds, 13% to 15% less than the ccgo engine counted for the same heap. A VM takes its memory from the system in chunks, and `MemoryUsage` fills `Footprint`, `FootprintLimit`, `Chunks` and `ReleaseCandidate`, which were 0. `ReleaseMemory` gives free memory back and returns how many bytes.
- `Math.*` gives the same bits as before on both targets (`engine/test/math.sh` checks 100000 inputs for each of 33 operations).
- A compile or a lazy property that runs out of memory ends the call with the out of memory error, where the ccgo engine could leave the error pending or read the wrong value. `\p{Script=...}` that runs out of memory no longer crashes the process.
- A template literal converts each substitution to a string at once, as the specification says, and `-0.5 < -1n` is false. Both come from upstream QuickJS commits ([upstream-commits.md](docs/changes/upstream-commits.md)).

### Removed

- The build tags `slowjs_nogoasm`, `goasm`, `slowjs_cengine` and `libc.memgrind`. The first three do nothing now, and the last one tracked the allocations of the ccgo engine.
- The package `github.com/liustack/slowjs/lib`, the ccgo engine and its API. The root package, `intl`, `temporal` and `fmt` keep every exported name they had in the last release: `SetModuleLoader` and `SetDefaultModuleLoader` were removed in v0.27.0 and stay removed.
- The requirements `modernc.org/libc`, `modernc.org/ccgo/v4` and their dependencies. `go.mod` asks for `golang.org/x/sys` and `golang.org/x/arch` only, and a program that imports the module links `golang.org/x/sys` and nothing from modernc.org.

### Performance

Against the last release with its PGO profile, the C engine without PGO, on linux/amd64 (AMD EPYC 7B13, one core, 8 code positions each).

- The pages of tifa's example shop, MUI test app and bench app render in 0.834 to 0.860 of the time (11 pages, every position faster), with the same HTML except the page of the 2001 level JSON.
- The pause of the Go garbage collector stays as short: the P99 of a stop is 131 to 164 µs with 1 VM (164 before) and 197 µs with 3 VMs (197 before), the longest 3.1 ms (4.2 before) and 0.9 ms (2.1 before).
- A VM of the shop holds 8.3 to 8.6 MiB after loading, 12.8 to 12.9 MiB after 300 renders and 9.7 MiB once the pool gives its free memory back. The shop program is 0.5% larger, and its table of frames 152 KB smaller.
- A soak of 150 minutes, 211,076 renders, ended with no error and no VM replaced, and the memory of the VMs did not move. [Performance](docs/performance.md#acceptance-before-the-switch) has the method and the rest.
- On the same JavaScript, a page of tifa's example shop, MUI test app and bench app takes 0.98 to 1.06 times as long as in C QuickJS 2026-06-04, 0.81 on the MUI pages. With its own `Intl`, where C needs a polyfill, the shop takes 0.82 and MUI 0.52. The other pure Go engines that render these pages take 1.6 to 6.6 times as long as slowjs ([Benchmarks](docs/benchmarks.md#tifa-pages)).

### Fixed

- `Close` of a VM that is already closed does nothing and returns nil. It used to panic on a nil pointer, so a deferred `Close` after an explicit one crashed. The functions of `OnClose` still run once.
- `Close` runs every step even when one panics. A function of `OnClose` that panics no longer keeps the functions after it from running, and nothing in `Close` is skipped for good: the VM leaves the table that finds it, the shared bytecode it held is released, and `Close` then panics with the first panic. A `Close` inside a function of `OnClose` returns nil.
- On the C engine, when the OS refuses to unmap some of the memory of a VM, `Close` returns that error, and `Close` again unmaps what is left. It used to return nil the second time and leave the memory mapped.

## [v0.28.6] - 2026-10-24

### Changed

- On linux/amd64 the engine runs JavaScript in the goasm interpreter by default, the interpreter in Go assembly that the `goasm` tag of v0.28.5 turned on. `-tags slowjs_nogoasm` builds a program with the Go interpreter, which every other target uses. The `goasm` tag is still accepted and changes nothing. Results, error stacks and HTML stay the same, and the build still needs no C toolchain. With one VM the pages of tifa's example shop render in 0.940 of the time and the MUI pages of tifa's test app in 0.850, without PGO. With 4 VMs on 4 cores tifa's pool renders 1.046 times as many shop pages a second and 1.171 times as many MUI pages, and with 8 VMs on 8 hyperthreads 1.085 and 1.211 times. After ten minutes of 8 VMs the process holds 0.93 of the RSS on the shop and 0.88 on the MUI pages. GC stops the world as fast as with the Go interpreter. See [The goasm interpreter](docs/changes/go-assembly.md).

## [v0.28.5] - 2026-10-24

### Added

- The `goasm` build tag (`go build -tags goasm`): on linux/amd64 the engine runs JavaScript in an interpreter compiled by LLVM from QuickJS's C and shipped as Go assembly, in place of the Go interpreter that ccgo makes from the same C. It is off by default, and on every other platform the tag changes nothing. Results, error stacks and HTML stay the same, and the build still needs no C toolchain. The pages of tifa's example shop render in 0.940 of the time and the MUI pages of tifa's test app in 0.850, without PGO. [The goasm interpreter](docs/changes/go-assembly.md) has how it is made and what must hold before it becomes the default.
- `GetAsyncContext` and `SetAsyncContext`, two Builtins that read and set the async context of a VM: an object that promise reactions keep. A `then`, `catch`, `finally` or `await` runs with the context the VM had when it was registered, and the `then` of a thenable with the one of the code that resolved a promise with it, as in Node. An `AsyncLocalStorage` written on them finds its store after `await`. A VM that never sets a context runs promises as before.

### Performance

Without PGO, against v0.28.2.

- Listing the own keys of an ordinary object, for `for in`, `Object.keys`, `values` and `entries`, object spread and rest, `Object.assign` and `JSON.stringify`, reads them from its shape in Go, and spread, rest, `values` and `entries` read plain values straight from the object: 0.990 of the time on the bench pages of tifa's example shop, 0.994 on its store pages and 0.987 on the MUI pages of tifa's test app, with the same HTML. `Object.keys` of six keys takes 134 ns rather than 542 and a spread of six keys 74 ns rather than 250 (Apple M4). A VM holds 8 KiB more.

## [v0.28.4] - 2026-10-24

### Changed

- `EvalBytecode`, `EvalBytecodeValue`, `EvalSharedBytecode` and `EvalSharedBytecodeValue` free the top-level function of a module once it has run to its end, with the variables of the top level that no function of the module uses. A VM that loaded a module held them until it was closed. A VM that loads the SSR bundle of tifa's example shop as a module holds 8.66 MiB rather than 9.44 MiB, about what the same bundle as a script gives (8.63 MiB). A module that is still pending or failed keeps its function.

## [v0.28.3] - 2026-10-24

### Fixed

- Regexps with the v flag no longer throw a SyntaxError for what the specification allows and Node accepts: `\` before one of ``& ! # % , : ; < = > @ ` ~`` in a class, as in `[\&\;]` or `[\!\|]`, a `-`, a `/` or a doubled punctuator such as `&&` outside a class, as in `/-/v`, and `\-` or `\&` in `\q{...}`. shiki's JavaScript engine failed on 204 of the 552 regexps it makes for bash, css, go, html, json, typescript and yaml. All but 3 compile now: those have more than 254 capture groups, the most a regexp holds. Patterns that compiled before compile to the same bytecode.

## [v0.28.2] - 2026-10-24

### Added

- `VM.SetClock` and `VM.Now` give one VM a wall clock that `Date.now()`, `new Date()`, `Date()`, Intl formatting without a date and `Temporal.Now` all read. nil restores the live clock. A non-nil `temporal.Clock` still overrides `Temporal.Now` alone. `performance.now`, the eval timeout, `Atomics.wait` and the `Math.random` seed keep their own clocks.

## [v0.28.1] - 2026-10-23

### Added

- `NewSharedBytecode`, `VM.EvalSharedBytecode` and `VM.EvalSharedBytecodeValue`: VMs that load the same bytecode keep the source text of its functions and the tables that give error stacks their line numbers in one copy for the process, where each VM copied them into its own memory. After loading, a VM of tifa's example shop holds 2.36 MiB less (8.99 MB rather than 11.46) and one of tifa's MUI test app 1.22 MiB less (4.16 MB rather than 5.44). `toString`, error stacks and line numbers stay the same. `MemorySize` and the memory limit no longer count those bytes. The copy is freed when the `SharedBytecode` and every VM that loaded it are closed, so a VM can outlive it. Render time and HTML do not change.

## [v0.28.0] - 2026-10-23

### Changed

- **Breaking:** the module is now `github.com/liustack/slowjs` with package `slowjs`, and it supports linux and darwin on amd64 and arm64 and windows on amd64.

## [v0.27.3] - 2026-10-23

### Added

- `VM.RunYoungGC` (`JS_RunYoungGC` in `lib/`): a garbage collection of only the objects made since the last collection, young or full. It frees the cycles among them and keeps the rest, which become old. A full collection, `VM.RunGC`, visits every object of the VM, about 30,000 for the bundle of tifa's example shop before any garbage. A program that runs one piece of work at a time, such as a page render, can run a young collection after each and a full one now and then. What a full collection frees, it still frees after any number of young ones. Young collections keep cycles through old objects and the values of weak maps until the next full one. Programs that do not call it collect as before.

### Fixed

- Loops of `indexOf`, `lastIndexOf` and `includes` of strings stop within 20 ms of a 10 ms timeout: each search counts the characters it looks through toward the interrupt check. In a string of 1 MB that holds no match they ran on for 170 ms, 6.9 s and 170 ms.

### Performance

- `includes`, `indexOf` and `lastIndexOf` of a string find what they look for with the search of Go's package bytes, where upstream compares at one index after the other: a search through 100 KB of HTML takes 81 µs rather than 320. A render of the shop takes 0.995 of the time and the MUI pages of tifa's test app 1.003, both inside the spread of code layouts. Results do not change.
- With tifa running `RunYoungGC` after each render and a full collection now and then, where it ran a full collection every few renders, the P99 of a render of the shop is 0.748 of what it was with one VM and 0.796 with two VMs, and that of the MUI pages of tifa's test app 0.940 and 0.919. The P50 and total CPU stay within 2.1%, and peak memory is 0.82 to 0.88 of what it was. Without PGO.

## [v0.27.2] - 2026-10-23

### Added

- `Args.Latin1` and `Args.UTF16`: the characters of a string argument of a Builtin as the engine holds them, one byte or one 16-bit unit each, without a copy. `VM.NewStringLatin1` and `VM.NewStringUTF16` make a string from such characters. A Builtin that looks at short strings and often returns its argument as it is (`a.Value(0).Dup()`) converts nothing: React's escape of HTML written this way takes 99 ns a call on a 16-character string, against 200 ns in React's javascript.


### Fixed

- The eval timeout and `Interrupt` stop a loop that compiles regexps within a few milliseconds, and stop one long compile in the middle, with the same uncatchable interrupt. Compiling a regexp was one step of the interrupt check however long the pattern, so with a 10 ms timeout a loop of `new RegExp` on a new long pattern each time stopped 225 ms to 184 s late, depending on the pattern, and one compile of 100000 alternatives ran its full 8.5 s. Now they stop at 10 to 12 ms.
- Loops of `JSON.parse`, `JSON.stringify`, `eval`, `new Function`, `repeat`, `padStart`, `replaceAll`, `join` and the other built-ins that parse source or build a string stop within a few milliseconds of the timeout too, where they ran on for 5 to 37 s with inputs of 60 KB to 1 MB. One call still runs to its end. Loops of built-ins that scan or sort without building a string (`indexOf` and `includes` of arrays, `split`, `fill`, `sort`) still stop late. Render time and memory do not change (0.993 of the time on the shop pages and 0.997 on the MUI pages, without PGO, against v0.27.1). A script that overran its deadline inside one long compile now gets the interrupt right after it, where it could finish.
- Full GC releases unreachable WeakMap entries in one collection, including values that point back to their keys and chains of entries. Live keys keep their values. Symbol keys, WeakSet, WeakRef and FinalizationRegistry follow the same rule. A full GC takes about as long as before on tifa's pages, and longer with large weak maps: 2.9 times as long for a chain of 100,000 live entries, 1.9 times for 100,000 live keys.

### Performance

- `Args.String`, `Args.WTF8` and `VM.NewString` (with every Go string that becomes a javascript string) convert in Go, straight from and into the engine's characters, with the same results. A Builtin that reads a 16-character string and returns a new one takes 134 ns rather than 230, and 521 ns rather than 1338 for 1,024 ASCII characters. `Value.Dup` takes no TLS frame: 81 ns rather than 97 for a Builtin that returns its argument.

## [v0.27.1] - 2026-10-23

### Fixed

- Native C builds using `lib/internal/patch/quickjs.diff` retain upstream's computed goto dispatch. The patch disables it only for ccgo and Emscripten. The generated Go engine is unchanged. Uncatchable exceptions, recursion RangeError and error stack rules are retained. Native recursion depth can differ because compiler frame sizes change under the same byte limit.

### Performance

Without PGO, against v0.27.0.

- `o.x` and `o.x(...)` of a string, a number or an array find a method on the prototype in the interpreter, without the generic property read: 0.996 of the time, and 0.975 on the MUI pages of tifa's test app, whose CSS-in-JS code reads string methods up to about 320000 times a render. Memory use does not change.
- `js_malloc` takes a block from the cache of freed small blocks itself, without the four calls it took before: 0.985 of the time, and 0.983 on the MUI pages of tifa's test apps. Memory does not change.

## [v0.27.0] - 2026-10-22

### Added

- `Intl.DateTimeFormat` accepts Temporal values in all four formatting methods, with type-specific defaults and calendar checks. `formatRange` and `formatRangeToParts` also accept Date and numeric values. Temporal date localization uses the same formats. All 18 Chromium calendars, including ISO, are supported through the shared go-intl data packs.
- `Intl.Collator`, `Intl.Segmenter`, `Intl.RelativeTimeFormat`, `Intl.ListFormat`, `Intl.DisplayNames`, `Intl.DurationFormat`, `Intl.Locale` and `Intl.supportedValuesOf`, in every locale Chromium has, compared with Chromium 156 on 14,269 expressions. `localeCompare` sorts by the locale's collation, and `toLocaleUpperCase` and `toLocaleLowerCase` follow Turkish, Azeri, Lithuanian and Greek rules. They share go-intl's data with the other constructors. Optional build tags `quickjs_intl_nonames` and `quickjs_intl_nocjdict` omit display names and the Chinese and Japanese word dictionary. A program using `intl` must carry the notices listed in `NOTICE.md`. Current size and memory measurements are in `docs/changes/time-and-intl.md`.
- `Args.WTF8`: read a string argument of a Builtin with its lone surrogates as the engine holds them, where `Args.String` gives U+FFFD for each.
- `VM.SetRegExpCache`, `DefaultRegExpCache` and `DefaultRegExpCacheBytes`: how many compiled regexps a VM keeps, and in how many bytes (256 in 1 MiB by default). The cache counts against the memory limit: give a VM that runs close to its limit a smaller cache, or none.
- `VM.OnClose`: run a function when the VM is closed, so a package that keeps something per VM in a table of its own can drop it then.
- Package `temporal`: `temporal.Register(vm)` adds `Temporal`, every type and method V8 ships, and `Date.prototype.toTemporalInstant`, computed in Go by go-intl with ICU's zones for tz 2026e built in, so the host's zoneinfo is never read. `Temporal.Now` uses the zone `intl.TimeZone` sets, and the clock `temporal.Clock` sets. The original built-in test262 run passed 9206 of 9210 cases. The current `intl402/Temporal` run passes 4038 of 4058. The original implementation took 13.4 ms on first use in a process and 1.1 ms in a later VM. Current size measurements are in the fork-change docs.

### Changed

- `performance.now()` of `StdAddHelpers` counts from the start of the process. Before, it counted from the boot of the system on Linux and macOS, and from 1970 on Windows and FreeBSD, where it moved when the system clock was set. On macOS it no longer counts the time the system sleeps. `Date.now()` and `new Date()` give the same values as before.
- All Intl constructors, including NumberFormat, DateTimeFormat and PluralRules, now use go-intl. Number and Date locale methods share the same data. Rounding increments and priorities work, and all date calendars are available. The separate `intl/cldr.bin` and its generator are removed. Against main `641e996`, the shop built with its application PGO profile is 0.11 MB smaller. Compiled plans and direct string output cut the measured warm number formatting time by 41% to 42% and date formatting time by 28% to 37% against main. The speedup leaves all recorded answers unchanged. See [measured costs](docs/changes/time-and-intl.md#focused-timing-and-memory-costs).
- Intl resolves named zones, offsets and daylight flags from the embedded tz 2026e pack shared with Temporal. It no longer reads host zoneinfo or needs a DST correction table. Winnipeg follows tz 2026e's permanent daylight time, newer than Chromium 156's data.
- The full Intl402 test262 run passes 6618 of 6764 cases, against 6276 on main `641e996`. All 24 newly failing assertions also fail in Chromium 156. Existing golden sets have no new unexplained differences.

### Removed

- `lib` no longer has the `std`, `os` and `Worker` modules of QuickJS's shell (`quickjs-libc.c`), which the Go API never registered: `js_init_module_std`, `js_init_module_os`, `js_module_check_attributes`, `js_std_set_worker_new_context_func`, `js_std_init_handlers`, `js_std_free_handlers`, `js_std_promise_rejection_tracker`, `js_std_await`, `js_std_loop`, `js_std_eval_binary` and `js_std_eval_binary_json_module` are gone. `StdAddHelpers`, `SetDefaultModuleLoader` and the bytecode of modules work as before. A program no longer links that code, nor the parts of libc only it called (`fork`, `execve`, `popen`, `setenv`, files, the terminal): one that only calls `Eval` is 354 KB smaller on linux/amd64, and the server of tifa's example shop 181 KB smaller, built as `tifa build` builds it (`-ldflags=-w` with the shop's PGO profile, linux/amd64).

### Fixed

- `Temporal.Duration.prototype.toLocaleString` formats through `Intl.DurationFormat`, including styles, unit options and fractional digits, instead of returning an ISO duration string.
- `Intl` no longer throws on a page that replaced `String.prototype.concat`, which QuickJS calls for a template literal: `Intl.NumberFormat`, `Intl.DateTimeFormat` and the `toLocale` methods threw there in v0.26.17.
- `Intl.PluralRules` selects the plural of a compact or scientific number as ICU does, and every `supportedLocalesOf` reads its `localeMatcher` option.
- A function in a list of locales is read as an object, as ECMA-402 has it, so `new Intl.NumberFormat([function () {}])` throws a RangeError, where v0.26.17 threw a TypeError.
- The first use of `Intl` keeps what a page did before it: its own `Date.prototype.toLocaleString` or `Number.prototype.toLocaleString` stays, so does an `Intl` it defined with `Object.defineProperty` or deleted, and a frozen `Date.prototype` or `globalThis` no longer makes that use throw a `TypeError`.
- `EvalBytecode` of a module whose top-level await is still pending when no job is left to run, such as `await new Promise(() => {})`, returns an error that says so instead of crashing the process. If what the module waits for settles later, from Go for one, a later `ExecutePendingJobs` runs the rest of it. It waits for the module as `ExecutePendingJobs` runs jobs: a job that throws no longer stops it, the module runs to its end and the call returns the job's error, or the module's own rejection. The eval timeout, an interrupt, out of memory or a panic in a Go function after a module's first `await` ends the call at once, where it was printed to stderr and the process then crashed.
- On Windows, `Date.now()` and `new Date()` no longer overflow in 2038.

### Performance

All without PGO, against v0.26.17.

- A VM keeps the bytecode of the regexps it compiled, so code that makes the same regexps on every render compiles them once: the journal post of tifa's example shop that highlights CSS renders in 0.893 of the time, the other pages in the same time. A VM of the shop holds 12 KiB more, and one that rendered the CSS post keeps about 714 KiB of regexps.
- `Date.now()`, `new Date()` and `performance.now()` read the time from Go instead of a system call of modernc.org/libc: 122 ns rather than 689, 261 rather than 870, and 107 rather than 686. A render of the shop does not change.
- The likely and unlikely tests of the engine branch on their condition, without making an integer of it first: 0.983 of the time, and 0.976 to 0.987 on the pages of tifa's bench app.
- On linux, the engine takes and frees its TLS frames without calling libc, except in the interpreter function: about 360000 fewer calls in a render of tifa's bench pages. On top of the change above, 0.998 of the time and 0.990 on the bench pages. Both together: 0.979 of the time of v0.26.17, and 0.967 on the bench pages.

## [v0.26.17] - 2026-10-21

### Added

- `NewBuiltin` and `VM.NewBuiltinFunction`: a Go function that javascript calls as a built-in, such as `Math.max`, with `Args` to read its arguments without conversions. A call that adds two numbers takes about 55 ns on an Apple M4, against about 470 ns through `RegisterHostFunc`. An `*Error` it returns is thrown as the error its `Name` says.
- `VM.NewArray`: an array of values made in Go, made as an array literal makes one, so no setter a page put on `Array.prototype` runs.
- `VM.SetStrip` with `StripSource` and `StripDebug`: compile functions without their source, so their `toString` shows `[native code]`, or without line and column numbers.
- `VM.RunGC`, `VM.MemorySize` and `VM.GCThreshold`: run a GC now, read the memory a VM takes as the memory limit counts it, and read the threshold of the next GC. A program can collect between two calls into javascript and set the threshold for the next one, as tifa does between two renders.

### Changed

- `Error()` of an `*Error` made in Go is `Name: Message`, as javascript's `Error.prototype.toString` writes it, where it was empty. So the `TypeError` a `RegisterHostFunc` function throws for such an error now has that message. An error taken from javascript keeps the engine's text.
- `Intl` formatters are shared by every VM of the process, up to 1024, and the oldest are dropped first. A VM that has used `Intl` holds about 15 KB less.
- `Intl`'s functions have the shape of built-ins: a `length` and `name` as ECMA-402 gives them, no `prototype`, they cannot be called with `new`, and `toString` shows `[native code]`.
- `Intl.NumberFormat.call(o)` and `Intl.DateTimeFormat.call(o)`, for an `o` that inherits from their prototype, keep the new formatter on `o` and return it, as V8 does.
- The files this fork created are under the MIT license (`LICENSE-MIT`), and its source files carry an SPDX line. 14 files in `lib/` that repeat parts of the engine say `MIT AND BSD-3-Clause`. Code from upstream stays under BSD-3-Clause and QuickJS's MIT license, and the locale data under the Unicode License V3. `NOTICE.md` lists which license covers which path, and which notices a program that ships a binary has to include.
- The README is a short landing page, in English and Chinese. What it held moved to `docs/`: the fork changes by area, testing, upgrading from upstream, performance, releasing, and where the code comes from, with what changes when switching from modernc.org/quickjs.

### Fixed

- A page that changes `Array.prototype`, `Object.prototype`, `JSON` or other built-ins after `Register` no longer changes what `Intl` does.
- `Intl` converts options, values and locale lists as ECMA-402 does: a Symbol or a BigInt where a string or a number is wanted throws a `TypeError`, `null` locales throw a `TypeError`, and options are read in the order ECMA-402 reads them.
- `Intl.NumberFormat` formats a wrapped BigInt, or an object whose `valueOf` gives a long numeric string, exactly, where it rounded it through a double.
- test262's intl402 tests pass 912 runs of 6764, against 770, none of those that passed before failing.

### Performance

All without PGO, against v0.26.16.

- `===`, `!==`, `==` and `!=` answer the usual pairs (`null`, `undefined`, booleans, objects, symbols, strings that are atoms) in the interpreter: 0.982 of the time.
- A string key that is already an atom is found in one step: 1.3% less time. Reading with such a key takes 13% less time in a micro benchmark, writing 30% less.
- A call of a javascript function takes one TLS frame instead of two on linux, and 34 more functions take a TLS frame less often: about a fifth fewer frames per render, and 1.4% less time.
- A regexp of alternatives no longer tries the alternatives that cannot match at a position: about 1% to 2.5% less time. A `replace` with react-spring's regexp of 148 color names takes 5.9 µs rather than 44.5.
- A new function gets its `length` and `name` without the shapes in between: 0.982 of the time.
- Bitwise operators on floats skip the slow path: the pages of tifa's MUI test app take 0.952 to 0.960 of the time. The shop runs them too rarely to gain.
- A `for in` loop skips prototypes with no enumerable property: up to 3% less time on pages that run many such loops.
- `JSON.stringify` and `JSON.parse` copy runs of plain characters at once: quoting a route's data takes 41% less time, though a render gains less than 1%.
- `Intl`: `new Intl.NumberFormat('en')` takes 2.7 µs rather than 16.5, a `DateTimeFormat` with a time zone 8.1 µs to make rather than 32.0 and 2.6 µs to format rather than 5.2, on an AMD EPYC 7B13 core.

## [v0.26.16] - 2026-10-20

### Added

- `VM.SetShapeCache` and `DefaultShapeCache`: how many shapes of plain objects a VM keeps once their objects are gone. A new VM keeps 2048, and 0 turns the cache off. `lib` has `JS_SetShapeCache`, off until called.

### Changed

- A VM of the shop holds 722 KiB more after a GC (8.03 MiB rather than 7.33), 647 KiB of it the kept shapes. Kept shapes count against the memory limit and are not let go when an allocation fails, so a script that runs close to its limit can run out of memory where it did not before: give such a VM a smaller shape cache, or 0.
- Objects that get keys no other object has, one at a time, take about 6% longer to build than with the shape cache off.

### Performance

- The TLS frame of 8 hot functions, among them property reads, `==` and turning a value into a string, is taken only around the statements that need it. Renders take 1.6% less time with PGO and 1.1% less without, and 2.6% less on darwin/arm64 (Apple M4, with PGO).
- The interpreter writes out its 46 calls of `set_value`: 1.4% less. With the frames above, 2.8% less than v0.26.15 with PGO and 2.9% less without.
- The interpreter counts its interrupt counter down in its own body instead of calling a function on every loop pass: 0.6% less with PGO.
- Each runtime keeps freed blocks of 16 to 512 bytes in a cache of its own, given back at each GC: 0.9% less with PGO.
- A VM keeps the shapes of plain objects, so the next object literal with the same keys makes no shape: object literals with the same keys take 30% less time to build, and a render 3.6% less without PGO. With the two changes above, 4.5% less than without the three. Two VMs on the two hyperthreads of one core render 2.9% more pages a second.

## [v0.26.15] - 2026-10-19

### Changed

- A panic in the engine itself, or a Go function that called `runtime.Goexit` (as `t.Fatal` does), leaves the VM good only for `Close`: every later call into javascript panics. A Go function given to javascript that panics is still recovered as before.
- Outside linux each VM keeps a block of up to 1 MiB of TLS stack until it is closed. On windows that block counts in full against the commit limit.
- A PGO profile collected with an earlier release matches far fewer call sites: collect it again.

### Performance

- Engine functions free their TLS frame before each return instead of in a `defer`, so Go inlines them: about 10% less time, with or without PGO.
- A regexp that starts with one character or one class of characters, such as `/["&<>]/`, starts where that character first occurs: about 3% less.
- `===` and the names of keys take a TLS frame only on their rare paths: 0.7% less with PGO, 2% without.
- Outside linux, VMs on several goroutines no longer wait on libc's allocator lock for their TLS stack: 10 VMs on an Apple M4 render 1.4 to 1.55 times as many pages a second.
- Together, renders take 0.859 of the time of v0.26.14 with a new PGO profile, and 0.843 without PGO.

## [v0.26.14] - 2026-10-19

### Fixed

- `NewVM` no longer leaks the memory it allocated when it cannot make a context.

### Changed

- `internal/fixlib` stops wherever an upstream change could make it fix the generated code wrong. Programs see no change.

## [v0.26.13] - 2026-10-18

### Performance

- The interpreter loop does less work with the stack: about 12% less time on linux/amd64 and 17% less on darwin/arm64 (Apple M4). Its Go stack frame went from 2608 to 896 bytes on amd64.
- Adding a new property to a plain object, as object literals, spread, class fields and `JSON.parse` do, skips the generic path: about 6% less.
- Together, renders take 0.823 of the time of v0.26.12.

## [v0.26.12] - 2026-10-18

### Changed

- On linux, a recursion through bound functions can hold up to about 1 KB more of TLS blocks per level, and on 32-bit targets a block can take up to 1 KB where it took 256 bytes. The worst case of a recursion to the default limit stays about 20.7 MB on linux/amd64.

### Performance

- A JavaScript call allocates no Go memory and no longer takes libc's allocator lock: about 11% less time on linux/amd64 and 14% less on darwin/arm64, and loading bytecode into a VM about 11% less on linux. Several VMs on several cores no longer hold each other back on that lock.

## [v0.26.11] - 2026-10-18

### Fixed

- `RegisterFunc` and `RegisterHostFunc` no longer abort the process when the global object already has the name. The function replaces a configurable property, and the call returns an error for a property that is not configurable, a global object that is not extensible, or a name with a NUL byte.

## [v0.26.10] - 2026-10-18

### Fixed

- `VM.Interrupt` is safe from any goroutine while the VM closes and after `Close`, where it does nothing.
- A Go string that is not valid UTF-8 becomes a JS string with U+FFFD for each byte that does not start a valid sequence, as Go decodes it. The engine used to drop the bytes after an invalid one, ASCII included.

## [v0.26.9] - 2026-10-18

### Fixed

- `intl` names daylight saving time as ICU does on every system. On Linux, Europe/Dublin and a few other zones swapped their summer and winter names.

## [v0.26.8] - 2026-10-18

### Fixed

- The module builds on linux and freebsd again, which v0.26.6 and v0.26.7 broke.
- A Go string made from a JS string is valid UTF-8: a lone surrogate, as left by cutting a string inside an emoji, becomes U+FFFD, as Node writes it.

## [v0.26.7] - 2026-10-17

Does not build on linux and freebsd. Use v0.26.8 or later.

### Added

- `*Error` has `Uncatchable`, `OutOfMemory` and `Interrupted`, which say whether the call ended with an exception no script could catch, and why. What a script throws is never uncatchable, whatever its name and message.

### Fixed

- The eval timeout runs on the monotonic clock, so setting the system clock no longer moves it, and a timeout too long for the clock never fires instead of firing at once.
- Taking an uncatchable error runs no page code, such as an `Error.prototype.toString` the page replaced.

## [v0.26.6] - 2026-10-17

Does not build on linux and freebsd. Use v0.26.8 or later.

### Changed

- An interrupt or the eval timeout can no longer be caught through a promise, an iterator or a Go function: the call returns it, as for a script without promises. `ExecutePendingJobs` stops at the job that failed with it and leaves the rest queued.
- Running out of memory is uncatchable too, and the call returns `*Error{Name: "InternalError", Message: "out of memory"}`. Page code can no longer fill memory and catch the failure.
- A panic in a Go function that javascript called no longer unwinds through the engine. The script ends as for an interrupt, and the Go code that called javascript panics again with the same value.
- Recursing without end throws `RangeError: Maximum call stack size exceeded`, which a script can catch. It used to kill the process with a Go stack overflow. A new runtime starts with a stack limit of 10000 slots.

### Fixed

- Compiling a script, a function body, `eval` code or a regexp with little memory left fails with the out of memory error, where the engine could crash, report a `SyntaxError` the script does not have, or return bytecode cut short.

## [v0.26.5] - 2026-10-17

### Fixed

- A string that cannot be converted to Go, for want of memory, comes back as the error instead of `""`, from `Eval`, `Call`, the JSON of an `Object`, `Value.MarshalJSON` and the arguments of Go functions.
- A BigInt whose `toString` throws reports that exception, and reading the name and message of a thrown value leaves no exception behind.

## [v0.26.4] - 2026-10-16

### Fixed

- `Intl.NumberFormat` reads fraction digits as ECMA-402 has since 2021: a currency with `maximumFractionDigits: 0`, or fraction digits next to significant digits, no longer throws a `RangeError`.

## [v0.26.3] - 2026-10-16

### Changed

- A script's local time is UTC in every VM, whatever zone the process has, without changing `TZ` for the rest of the process. `getTimezoneOffset()` is 0, the local getters and setters work in UTC, and `toString` prints `GMT+0000`. To show another zone, pass a `timeZone` to `Intl`.

## [v0.26.2] - 2026-10-16

### Added

- Package `intl` formats dates, numbers and plurals in every locale Chromium has, from CLDR 48.2, and adds `Intl.PluralRules`. Its output is compared with Chromium 156 on about 60,000 expressions. A program that uses it is 1.7 MB larger.

### Changed

- `intl` defaults to UTC instead of the host's zone, and installs itself the first time a script uses `Intl` or a `toLocale*` method, from bytecode compiled once per process. `Register` takes 0.09 ms and 9 KB of the VM's memory, against 0.85 ms and 46 KB before.

### Fixed

- A Go function that evaluates more javascript on its own VM no longer sets the eval timeout again or drops an `Interrupt` asked for meanwhile.
- An `intl` install cut short by running out of memory, a stack overflow or an interrupt is rolled back, and the next use installs again.

## [v0.26.1] - 2026-10-15

### Changed

- An error that something keeps holds the bytecode of the functions on its stack, and their realm, until its `stack` is read.

### Performance

- Making an error no longer formats its `stack`: the text is made the first time `stack` is read, with the same result. A server render of a 100-item React component with the development build of React took 11.2 ms instead of 42.1 ms on darwin/arm64.

## [v0.26.0] - 2026-10-14

The first release of the fork, from modernc.org/quickjs v0.25.0 and libquickjs v0.13.1.

### Changed

- **Breaking:** the module is `github.com/liustack/quickjs`, and the engine is its package `github.com/liustack/quickjs/lib`, so it no longer depends on `modernc.org/libquickjs`. It needs Go 1.27.

### Fixed

- A VM that runs out of memory inside `JSON.parse`, or anything else that converts a string with non-ASCII characters to UTF-8, no longer leaks that string.
- `VM.Call` and `VM.CallValue` report the exception thrown while evaluating the function expression instead of `cannot call a non-function`.

[Unreleased]: https://github.com/liustack/slowjs/compare/v0.29.4...main
[v0.29.4]: https://github.com/liustack/slowjs/compare/v0.29.3...v0.29.4
[v0.29.3]: https://github.com/liustack/slowjs/compare/v0.29.2...v0.29.3
[v0.29.2]: https://github.com/liustack/slowjs/compare/v0.29.1...v0.29.2
[v0.29.1]: https://github.com/liustack/slowjs/compare/v0.29.0...v0.29.1
[v0.29.0]: https://github.com/liustack/slowjs/compare/v0.28.6...v0.29.0
[v0.28.6]: https://github.com/liustack/slowjs/compare/v0.28.5...v0.28.6
[v0.28.5]: https://github.com/liustack/slowjs/compare/v0.28.4...v0.28.5
[v0.28.4]: https://github.com/liustack/slowjs/compare/v0.28.3...v0.28.4
[v0.28.3]: https://github.com/liustack/slowjs/compare/v0.28.2...v0.28.3
[v0.28.2]: https://github.com/liustack/slowjs/compare/v0.28.1...v0.28.2
[v0.28.1]: https://github.com/liustack/slowjs/compare/v0.28.0...v0.28.1
[v0.28.0]: https://github.com/liustack/slowjs/compare/v0.27.3...v0.28.0
[v0.27.3]: https://github.com/liustack/quickjs/compare/v0.27.2...v0.27.3
[v0.27.2]: https://github.com/liustack/quickjs/compare/v0.27.1...v0.27.2
[v0.27.1]: https://github.com/liustack/quickjs/compare/v0.27.0...v0.27.1
[v0.27.0]: https://github.com/liustack/quickjs/compare/v0.26.17...v0.27.0
[v0.26.17]: https://github.com/liustack/quickjs/compare/v0.26.16...v0.26.17
[v0.26.16]: https://github.com/liustack/quickjs/compare/v0.26.15...v0.26.16
[v0.26.15]: https://github.com/liustack/quickjs/compare/v0.26.14...v0.26.15
[v0.26.14]: https://github.com/liustack/quickjs/compare/v0.26.13...v0.26.14
[v0.26.13]: https://github.com/liustack/quickjs/compare/v0.26.12...v0.26.13
[v0.26.12]: https://github.com/liustack/quickjs/compare/v0.26.11...v0.26.12
[v0.26.11]: https://github.com/liustack/quickjs/compare/v0.26.10...v0.26.11
[v0.26.10]: https://github.com/liustack/quickjs/compare/v0.26.9...v0.26.10
[v0.26.9]: https://github.com/liustack/quickjs/compare/v0.26.8...v0.26.9
[v0.26.8]: https://github.com/liustack/quickjs/compare/v0.26.7...v0.26.8
[v0.26.7]: https://github.com/liustack/quickjs/compare/v0.26.6...v0.26.7
[v0.26.6]: https://github.com/liustack/quickjs/compare/v0.26.5...v0.26.6
[v0.26.5]: https://github.com/liustack/quickjs/compare/v0.26.4...v0.26.5
[v0.26.4]: https://github.com/liustack/quickjs/compare/v0.26.3...v0.26.4
[v0.26.3]: https://github.com/liustack/quickjs/compare/v0.26.2...v0.26.3
[v0.26.2]: https://github.com/liustack/quickjs/compare/v0.26.1...v0.26.2
[v0.26.1]: https://github.com/liustack/quickjs/compare/v0.26.0...v0.26.1
[v0.26.0]: https://github.com/liustack/quickjs/tree/v0.26.0
