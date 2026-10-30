---
summary: 'The commits of upstream QuickJS taken into engine/ one at a time after the pinned 3d5e064 (the fixes of wrong answers, leaks and crashes, Array slice and splice, borrowed operands in js_strict_eq2, template literals, unicode_script), the faster indexOf and includes that perf found, the ones tried and taken back, the BigInt comparison fixed in slowjs, and the rest that are left out, with tests and speed'
read_when:
  - Asking which upstream QuickJS commits after 2026-06-04 the C tree has, and which it does not
  - Changing js_array_slice, js_array_splice, js_strict_eq2, js_array_indexOf, js_array_includes, js_parse_template or unicode_script in engine/
  - Taking another commit of upstream QuickJS into engine/
  - Asking why a built-in or a parse rule behaves as a commit of upstream QuickJS says, or differs from Node 24 (Iterator helpers, Promise.try, yield and await as names, class elements, module exports, Proxy traps, sort, bind, BigInt.asUintN)
  - Reading the speed of a C engine commit that came from upstream
---

# Upstream commits taken into the C tree

`engine/` starts from QuickJS `3d5e064` (2026-06-04) and changes on its own ([c-engine.md](c-engine.md)). Upstream has added 81 commits since (to `19d6bd8`, 2026-10-26). This page lists the ones taken in, each as a commit of its own that names the upstream commit, the ones tried and taken back, and the ones left out. It is not a promise to follow upstream. A commit that fixes a wrong answer, a leak or a crash is taken whether or not a page can reach it, and so is one that makes tifa's renders faster. A new feature or a change for speed is taken when tifa has a use for it. Every one comes with a test that prints what Node 24 prints (the lines that follow the current specification instead are marked in the file), or, for a leak, a sweep of the memory left.

Where an upstream commit meets a change of slowjs, slowjs's change stays and the upstream text is merged around it. To see whether a commit still applies, run `git apply --directory=engine --check` on its diff.

| Upstream commit | What it does | How it was taken | Test |
| --- | --- | --- | --- |
| `42d08be` optimized Array.prototype.slice and splice | slice and splice of a dense Array copy or move the elements directly | all of it, with `js_call_constructor` (which counts the call depth) where upstream calls `JS_CallConstructor` | `compare/array_slice_splice.js` |
| `8f0c037` use JSValueConst arguments for js_strict_eq2 | `indexOf`, `lastIndexOf`, `includes`, `Map` and `Set` stop duplicating and freeing the values they compare | all but its last hunk, which edits macros slowjs does not have | `compare/strict_equal.js` |
| `f8199dc` Fix template literal substitution | a template converts each substitution to a string at once, as the specification orders, and no longer calls `String.prototype.concat` | all of it | `compare/template_literal.js` |
| `817509a` removed infinite loop in unicode_script() | the out of memory path of `\p{Script=...}` returns instead of jumping to itself | all of it | `unit-oom`, `test_regexp_script_oom` |
| `c17f7fc` fixed BigInt.AsUintN() | `BigInt.asUintN` of a negative BigInt wrapped it only below 64 bits | all of it | `compare/bigint_as_n.js`, `test_bigint.js` of upstream |
| `d88b1ed` Reject out-of-range limits in Iterator.prototype.take/drop | a finite limit above 2**53-1 throws a RangeError and closes the iterator | all of it | `compare/iterator_take_drop.js` |
| `21cbd86` added JS_ToInt64SatF() | `''.repeat(2**31)` is an empty string, `take` and `drop` read their limit as a saturated integer | merged around the polls of `repeat` | `compare/string_repeat.js` |
| `c8ef889` fixed memory leak in Atomics.store | a BigInt made by the conversion of the value was not freed when the store threw | all of it | `compare/atomics_store.js` (the heap after the failing stores) |
| `5b6356a` fixed ArrayBuffer.prototype.resize | the length is read with `JS_ToIndex`, so 2**64 + 8 is not 8 | all of it | `compare/array_buffer_resize.js` |
| `8ac9d9b` fixed TypedArray.prototype.at | `at` counts from the length it read before it converted its argument | all of it | `compare/typed_array_at.js` |
| `4984249` fixed returned value of empty catch blocks | the completion value of `try { 2; throw 0 } catch (e) { }` is undefined | all of it | `compare/completion_values.js` |
| `abf571a` fixed 'return' invocation in Iterator helpers | `return()` of a helper calls the `return` method of the iterator under it, and the inner iterator of `flatMap` has its `next` read once | all of it with `js_call`, and three changes of ours (see below) | `compare/iterator_helpers_return.js` |
| `cfc9464` simplified and fixed yield/await identifier parsing | `yield` and `await` are names or keywords by the context of the function, its name and its arrow parameters | all of it | `compare/yield_await_names.js` |
| `df8de73` Annex B: allow call expression as assignment target | `f() = 1`, `f()++` and `for (f() in o)` compile in sloppy code and throw a ReferenceError when they run | all of it | `compare/call_as_target.js` |
| `8e68fcf` spec update for Promise.try | a promise the callback returns comes back as it is, and the callback runs before the constructor is used | with `js_call` and `js_get_exception`, which keep an interrupt from being caught | `compare/promise_try.js` (lines marked ES2026) |
| `1aa7429` fixed module ambiguous export test | a binding a module imports and exports again is the one it imported | all of it | `compare/module_ambiguous_export.mjs` |
| `fce112d` fix super property assignment on non-extensible receiver | `super.x = v` and `Reflect.set` change a property a non-extensible receiver has | all of it, and a change of ours (see below) | `compare/set_non_extensible.js`, `compare/array_add_index.js` |
| `13876a2` fix anonymous function name inference in for-in loop initializers | `for (var f = function () {} in o)` names the function `f` | all of it | `compare/for_in_initializer.js` |
| `efc48a3` fixed Function.prototype.bind prototype inheritance | a bound function has the prototype of the function it binds | all of it | `compare/bind_prototype.js` |
| `ce0dbb7` fix Proxy getOwnPropertyDescriptor invariant check | the trap cannot report another value or other accessors for a property that is not configurable, and the descriptor of the target no longer leaks | all of it | `compare/proxy_own_descriptor.js` (with the heap after it) |
| `535a7c2` respect the spec operation order in Object.defineProperties | every key, descriptor and value is read before the first definition | all of it | `compare/define_properties_order.js` |
| `55c5f0b` fixed class elements parsing after a line terminator | `get`, `set` and `static` before a line break | all of it | `compare/class_element_names.js` |
| `79f58c1` Fix leak in JS_ReadBigInt on truncated input | the limbs a read of cut-short bytecode had made are freed | all of it | `unit-api`, `test_read_truncated_bigint` |
| `9a0197f` Write back every element in Array.prototype.sort | an element that did not move is written too | all of it | `compare/sort_write_back.js`, `test_builtin.js` of upstream |
| `1134912` Fix Proxy ownKeys trap accepting a primitive result | a number or a string from the trap is a TypeError | all of it | `compare/proxy_own_keys_result.js` |
| `1ec58aa` Await the async iterator return() when a for-await loop breaks | a for await loop that leaves early waits for `return()`, and one that ends does not call it | without its `using` branch, with a check of the result added | `compare/for_await_close.js` |
| `19d6bd8` fixed arbitrary module namespace names | `export { x as "a-b" }` and `export * as "name" from` | all of it, and test262 runs the feature | `compare/module_string_names.mjs` |
| `a479eff` fixed regexp unicode set parsing | `/-&&/v` is a valid regexp | it was in the C tree already | `test_builtin.js` of upstream, which now has the line |

## Speed

Measured as [performance.md](../performance.md#how-the-numbers-were-measured) says, on the Google Cloud VM, at the 8 code positions of [Code positions](../performance.md#code-positions) (the whole program moved by 0 or 32 bytes and the body of the interpreter by 0, 8, 16 or 24 bytes, so the interpreter's entry stays on its line of 64 bytes and only the other functions move with the first), 5 rounds, each commit against the one before it. A change counts when the median over the positions is off 1 by more than 2% and by more than half of the range of the baseline over its own positions, with most positions on the same side. The stage rows are under the generator of 5e22e512.

| Commit | Script | Time | Positions | Range of the baseline |
| --- | --- | ---: | --- | ---: |
| `42d08be` | `q_array_slice` | 0.076 | 0.075 to 0.086 | 0.6% |
| | `c_array_methods` | 0.994, within noise | 0.926 to 0.998 | 9.2% |
| | `r_react_ssr` | 0.998, within noise | 0.993 to 1.003 | 5.5% |
| `8f0c037` | `c_array_methods` | 0.955 | 0.926 to 0.959 | 1.7% |
| | `c_map_set_iter` | 0.980 | 0.963 to 0.986 | 2.0% |
| | `r_react_ssr` | 0.975, within noise (8 of 8 positions below 1) | 0.955 to 0.990 | 5.2% |
| `f8199dc` | `c_string_methods` | 0.985, within noise | 0.977 to 1.006 | 1.8% |
| | `r_bench_table`, `r_bench_spiral` | 1.003, 0.995, within noise | | 2.6%, 1.0% |
| | `r_react_ssr` | 1.002, within noise | 0.998 to 1.007 | 2.7% |

`42d08be` makes `q_array_slice`, a loop of `slice` on a dense array, 13 times faster: 338 ms to 26 ms. `r_react_ssr` does not slice enough to move.

### The whole branch against main

All 37 scripts, `c4dcea91` (main) against the branch with everything above, the generator of that main for both, 2 rounds at the 8 positions:

- Geometric mean of the 37 scripts 0.917 (0.914 to 0.934 at the 8 positions). Without `q_array_slice` (0.085) and `c_array_methods` (0.530) it is 0.997.
- `r_react_ssr` 0.977 (positions 0.961 to 1.001, range of main 5.3%), within noise, and not slower at 7 of 8 positions. The other render scripts: `r_bench_table` 0.991, `r_bench_spiral` 0.990, `r_shiki_regexp` 0.981.
- Marked faster by the rule without a change of code behind them: `c_json_stringify` 0.963 and `c_string_concat` 0.957. `q_string_build2` moved to 0.91 to 0.97 with a position range of 11% to 21% in three runs. None of these runs code that a commit above changed, so they are not credited to it: where the code lands moves them.
- Marked slower: `v8_regexp` 1.024 (0 of 8 positions below 1, range of main 1.1%), also 1.017 and 1.019 in two earlier runs. callgrind counts 7,107,881,253 instructions for main and 7,109,594,044 for the branch (+0.02%), so it is where the code lands, and the matcher's own place is fixed already.
- The interpreter's entry sits on its line of 64 bytes at every position (the generator puts it there). The shift of the whole program moves the other functions only, so a change in the code around the interpreter is not seen through it.

## indexOf and includes decide most elements without a call

Not from upstream. perf on `c_array_methods` against the same C built natively with clang (`engine/test`) put the gap in `js_array_includes` and `js_array_indexOf`: 6020 and 3915 samples against 1826 and 957. Their loops called `js_strict_eq2` for each element, and a loop with a call that may poll keeps its poll counter in memory: one read, decrement and write of a word at each turn, which was 36% of the samples of `js_array_includes`. `js_quick_strict_eq` answers the pairs that need no call (ints, bools, null, undefined, objects, symbols, floats, an int with a float, strings that are the same string, differ in length or are both atoms, and values of two kinds that are never equal), and `js_fast_array_find` loops over a dense Array with it and calls `js_strict_eq2` only for the rest: two strings that are not atoms, a rope and a BigInt. `lastIndexOf` has no loop over a dense Array and is not changed. `compare/strict_equal.js` searches arrays of ints, floats, atoms, fresh strings, mixed values and one of every kind, with from-indexes, and prints what Node 24 prints.

On the merged state of the branch (commit `a57b9148`) against the same plus this change: `c_array_methods` 0.583 (positions 0.562 to 0.588, baseline range 3.9%, 8 of 8 positions below 1). `r_react_ssr` 1.001, `c_map_set_iter` 1.002, `c_string_methods` 0.987, `q_array_read` 1.003, all within noise.

## Tried and taken back

Three more commits were taken and then reverted, each by a commit of its own, because they gained nothing that passes the rule, and one of them made `r_react_ssr` slower. Their tests stay.

- `088cf5e` (floats in `<`, `<=`, `>` and `>=` in the interpreter): `r_react_ssr` took 1.044 of the time, every position between 1.036 and 1.051, with the baseline's range at 2.8%. `q_float_arith` 0.999, `v8_navier_stokes` 0.978 (range 11.8%), `v8_raytrace` 1.009. The macro grows the interpreter, and tifa's pages compare ints.
- `445624b` (the number cases of `==` and `===`, merged into `goasm_eq_fast`, which already has its other cases): nothing moved. `r_react_ssr` 0.995, `q_float_arith` 1.004, `v8_navier_stokes` 1.004, `v8_raytrace` 0.986 (range 2.0%). Upstream's follow-up `762e6fc` fixes a typo in the macros it adds, which the fork never had.
- `888481d` (`String.prototype.concat` in one buffer): `c_string_methods` 0.997, `c_escape_html` 0.995, `c_string_concat` 1.019 (range 5.8%), `r_react_ssr` 1.002. tifa's pages do not call `concat`.

## A negative BigInt and a negative float

`js_bigint_float64_cmp` compared the exponents of a BigInt and a float of the same sign and answered "less" for the smaller exponent whatever the sign. For two negative numbers the smaller exponent is the larger number, so `-0.5 < -1n` was true and `-2147483648 < -1n` false. It is reported upstream as [bellard/quickjs#583](https://github.com/bellard/quickjs/issues/583). It is fixed in `engine/` only. `compare/number_compare.js` found it: it prints all eight comparison operators for every pair of 41 numbers and 19 other values, and the call order of `valueOf`.

## Where a taken commit differs from upstream or from Node 24

Each file under `engine/test/compare` is what Node 24 prints, except for the lines it marks. These are the places where the C tree is not upstream's text or Node's answer.

- `abf571a`: upstream's new code closed the inner iterator of `flatMap` when it ended and when its `next` threw, and left a helper that threw open to `return()`. Node 24 does neither. The C tree does not close the inner iterator in those two cases, and marks a helper whose call threw as done.
- `fce112d`: upstream removed the early test of extensibility in `JS_SetPropertyInternal` and relied on `JS_CreateProperty`, which grew the length of an array before it found the array not extensible. `a[10] = 1` on a sealed array then left it at length 11. `JS_CreateProperty` tests it first now, which also fixes `Reflect.defineProperty(preventedArray, 7, ...)`, wrong before the commit.
- `1ec58aa`: upstream checks nothing about what `return()` resolves to. The C tree throws a TypeError if it is not an object, as the specification says. A body that throws still does not wait for a `return()` that succeeds, where Node 24 waits.
- `8e68fcf`, `d88b1ed`: the current specification, which Node 24 does not follow yet. `Promise.try(() => p)` is `p` for a promise made by the same constructor. A finite limit above 2**53-1 in `take` and `drop` throws a RangeError, where Node accepts it.
- `df8de73`: code in strict mode with `f() = 1` is a syntax error, as in the specification. V8 compiles it and throws a ReferenceError when it runs. `super() = 1` is a syntax error too.
- `1aa7429`: one namespace object that two paths export under one name is not ambiguous in the specification, and is in V8.
- `535a7c2`: a descriptor that is not an object is found when its definition is made, so the definitions before it stay, where the specification finds it with the first reads and defines nothing.
- `9a0197f`: the specification writes an array of one element back and throws for a frozen one, as the C tree does. V8 returns it as it is.
- `21cbd86` and `d88b1ed` read the limit of `take` and `drop` as a saturated integer, which upstream calls not strictly compliant for a limit such as 2**31-1 + 0.5.

## What changes for a program

- A template literal converts each substitution to a string before it evaluates the next one, so `${c} ${c.n++}` and a `Date` that a later substitution changes print what Node prints. A changed `String.prototype.concat` no longer changes templates. A symbol in a template still throws a `TypeError`.
- A regexp with `\p{Script=...}` or `\p{Script_Extensions=...}` that cannot be compiled for lack of memory ends the call with the out of memory error. It crashed the process before (clang took the endless loop for undefined behaviour).
- Comparing a negative BigInt with a negative float gives the right answer.
- `BigInt.asUintN` of a negative BigInt wraps it at every width. `BigInt.asUintN(64, -1n)` is `2n ** 64n - 1n`.
- `''.repeat(2 ** 31)` is an empty string. `Iterator.prototype.take` and `drop` throw a RangeError for a finite limit above 2**53-1 and close the iterator.
- A failing `Atomics.store` and a read of cut-short bytecode leak nothing. `ArrayBuffer.prototype.resize` reads its length as an index. `TypedArray.prototype.at` counts from the length it read first.
- An iterator helper closes the iterator under it with `return()`. A for await loop that leaves early waits for `return()`, and one that runs to its end does not call it.
- `yield` and `await` are names or keywords as the context of the function says, `f() = 1` compiles in sloppy code and throws when it runs, a class element after `get`, `set` or `static` and a line break is read as the specification reads it, and `export { x as "a-b" }` is accepted.
- `Promise.try` gives back a promise its callback returns. `Function.prototype.bind` keeps the prototype of the function it binds. `Object.defineProperties` reads everything before it defines. A proxy cannot report another value or other accessors for a property that is not configurable, nor a primitive from `ownKeys`. `Array.prototype.sort` writes every element back. `super.x = v` sets a property that a non-extensible receiver has.
- A sealed or frozen array keeps its length when an index past its end is set or defined, which `Reflect.defineProperty` did not.

## Left out

Judged by reading each diff against slowjs's tree.

- `0808cbc` (nonextensible-applies-to-private) is a proposal, not a fix. A private field or a brand cannot be added to a non-extensible object. Node 24 adds it, so the commit would make the C tree differ from Node. Taking it later is one commit: its two checks are in `JS_DefinePrivateField` and `JS_AddBrand`, and its change of `test262.conf` turns the feature on.
- Fixes outside the table that this tree cannot use: `cac72873` (a Symbol key in `JS_PrintValue`, which the Go API does not call), `0d9e5855` (32 bit hosts), `44e85b16` (the `os` and `std` modules of the `qjs` shell), and the fixes of upstream's Temporal and Intl (`4d56487d`, `4490e58b`, `ea3deb14`, `a37f62af`, `bcb9ad17`), which slowjs does not have.
- Already fixed in slowjs: `cd97c2b` (the stack of a syntax error out of memory, upstream #529).
- `4b0addb` (`JS_IteratorNext` without two unused arguments) is a cleanup of a signature. It was not tried.
- New features, which Vite compiles away: the new iterator helpers (`zip`, `zipKeyed`, `join`, `includes`, chunking), `Promise.allKeyed`, `Array.fromAsync`, explicit resource management, `Atomics.waitAsync`.
- API for embedders that tifa does not use (the `JS_To*` exports and the like), upstream's Temporal and Intl (slowjs has go-intl) and upstream's test tools.

