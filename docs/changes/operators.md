---
summary: 'Changes to operators: bitwise operators on two numbers, the usual pairs of ===, !==, == and !=, instanceof of an ordinary function, and add and sub of two numbers answered without the slow paths, with what each gains, why it is safe'
read_when:
  - A bitwise operator, a comparison, an add or a sub gives another result than before, or a compared value is freed twice
  - instanceof gives another answer or error than before, or a Symbol.hasInstance method is not called
---

# Operators

The interpreter does an operator itself only for the simplest operands, two ints, and calls a slow function of the engine for everything else. These changes answer more pairs without that call.

## Bitwise operators on floats skip the slow path

Release: v0.26.17.

`&`, `|`, `^`, `<<`, `>>` and `>>>` of two numbers no longer take the slow path when one of them is a float. The interpreter did the operator itself only for two ints and called `js_binary_logic_slow` or `js_shr_slow` for everything else, which convert both operands with the checks for BigInts and objects and then with the generic `ToInt32`. A product past 2^31 is a float, so the string hash of emotion, the CSS-in-JS library of MUI, took that path for almost every operator. It does the operator for any two ints or floats, converting a float as `JS_ToInt32Free` does (the same bits, without `fmod`), and stores an int, or for `>>>` a float above 2^31 - 1, as `JS_NewUint32` does. Any other pair goes on through the rest of the slow path as before.

Why this is safe: numbers hold no reference, so nothing is freed, and the slow paths do nothing else for two numbers, since converting a number to a numeric is a no-op and cannot throw or call a script. The interpreter is left as it is: calling `bitwiseNumbers` from its six handlers instead made the shop 0.8% slower. All 5 files change, because `bitwiseNumbers` reads their struct `JSValue`.

On linux/amd64 without PGO, against v0.26.16 over 10 rounds, the pages of tifa's MUI app, which run about 130,000 to 300,000 of these operators on floats per render, take 0.952 to 0.960 of the time (0.935 to 0.969). The shop runs the slow paths 3 to 24 times per render and the bench app not at all, so they have nothing to gain: the shop measured 1.008 (0.999 to 1.042), which is code layout, not work. The HTML and the memory of a VM stay the same.

## The usual comparisons answered in the interpreter

Release: v0.26.17.

`===`, `!==`, `==` and `!=` no longer call the slow path for the usual pairs. The interpreter compared two ints itself and called `js_strict_eq_slow` or `js_eq_slow` for everything else, which for `===` went on to `js_strict_eq2`. React compares with `null` and `undefined` (minified as `null != x` and `void 0 === x`, which the compiler does not turn into `is_null` or `is_undefined`), compares types, symbols and objects by identity, and switches over strings. The interpreter now answers these itself:

- objects and symbols by pointer
- `null`, `undefined` and booleans by tag and value
- strings when they are the same string, have different lengths or are both string atoms (the atom table holds each text once)
- for `===` and `!==`, values of different tags when one is `null`, `undefined`, a boolean, an object or a symbol
- for `==` and `!=`, `null` or `undefined` beside anything (equal only to `null`, `undefined` or an `IsHTMLDDA` object)

Other pairs of different tags under `==`, such as `true == 1` or `{} == 'x'`, convert their operands and still take the slow path, as do numbers, BigInts, ropes and strings that need their characters compared.

Strings that are both atoms are told apart only when both are string atoms: the description of a symbol is an atom of another type and can hold the text of a string atom. The text is written into the four handlers, with no call on the usual path, since Go does not inline into a function as big as the interpreter. It passes `eqFree` the stack pointer rather than the values, so the frame of the interpreter changes little: 8 bytes more on amd64 and 16 fewer on arm64, from v0.26.16. All 5 files change, because the text reads their struct `JSValue`. The bit fields it reads (the length and atom type of a string, the `IsHTMLDDA` bit) come from each file's `js_string_eq`, `__JS_FreeValueRT` and `JS_IsHTMLDDA`, and the layout of a value (how far below its pointer the reference count sits, the size of a value on the stack and how a boolean is stored) from its `JS_FreeValue` and `js_strict_eq_slow`.

On linux/amd64 without PGO, against v0.26.16 over 10 rounds, a render of tifa's example shop takes 0.982 of the time (0.974 to 0.989), the spiral and table pages of its bench app 0.940 and 0.942 (with wide ranges, 0.859 to 0.969 and 0.871 to 1.030), and its MUI pages 0.982 to 0.985. The HTML and the memory of a VM stay the same.

## instanceof without the call of Symbol.hasInstance

Release: v0.31.0.

`x instanceof C` in the register loop no longer calls the C function `Function.prototype[Symbol.hasInstance]` when that is the method it would call. The specification's `InstanceofOperator` reads `C[Symbol.hasInstance]` and calls it, and for an ordinary function that is the built-in of `Function.prototype`, whose call is `OrdinaryHasInstance(C, x)`. Before, every test went through `rv_obj2_slow`, `JS_IsInstanceOf`, two property lookups (`Symbol.hasInstance`, a miss on the function and a hit on `Function.prototype`, then `prototype`), `JS_CallFree` with a frame for the built-in, `JS_OrdinaryIsInstanceOf`, and the boolean back through `JS_NewBool` and `JS_ToBoolFree`: about 475 instructions on amd64. The handler of `INSTOF` now asks `rv_instof` first, which answers in place or does nothing and leaves the test to the old path.

`rv_instof` answers only when all of these hold, and the old path runs for anything else:

- The interrupt flag is clear. The call of the built-in it replaces fails at its entry when the host asked to stop or an uncatchable exception is pending (an async function whose first run ran out of memory or was interrupted leaves one pending and returns its promise), and the flag holds both. So the statements after the test do not run, as before. It is one load and a branch.
- `C` is an object that is not exotic (so not a proxy), not a bound function, and callable (a function of bytecode, or an object whose class has a call handler).
- From `C` up to the `Function.prototype` of the realm that runs the code, no object has an own `Symbol.hasInstance` (data, getter or not made yet) and none is exotic, and the chain reaches that `Function.prototype`. The property there is the built-in the realm was made with: it is neither writable nor configurable, so nothing can replace or delete it. Every way a script can make another method visible (`static [Symbol.hasInstance]`, `Object.defineProperty`, `Reflect.defineProperty`, a parent class that has one, `Object.setPrototypeOf` of `C` to an object with one or to a proxy, a function of another realm, whose chain reaches the other `Function.prototype`) puts an own property on an object of that walk, or a proxy in it, or takes the chain away from this `Function.prototype`. The walk reads the shapes as they are at the test, so there is no flag to keep and nothing to clear. The property on that `Function.prototype` must be made already (see below).
- `x` is not an object: the answer is `false`, after the checks the specification makes before it (a right side that is not callable throws first, through the old path).
- Else `C` has an own `prototype` that is a plain data property (not a getter, not the prototype of a function that is not made yet) whose value is an object, and the chain of `x` is walked as `JS_OrdinaryIsInstanceOf` walks it. At an object with no prototype in its shape that is exotic and not a fast array (a proxy, whose `getPrototypeOf` trap could run), the old path takes over.

Why this is safe: nothing on the path can run JavaScript, throw or allocate, so when it gives up after reading part of a chain, the old path does all of it again and nobody can tell. Every error (a right side that is not callable, a `prototype` that is not an object, a throwing getter) comes from the old path, with its message and from the realm it was made in before. The call it replaces also counted a level of depth: without it, a script that recurses through `instanceof` goes at least as deep as before. A throw injected at `INSTOF` turns the fast path off for that run, as for every handler, so the injection set still tests the old path's freeing.

The memory stays the same too. `Function.prototype[Symbol.hasInstance]` is made the first time it is read, like every built-in method, and the old path reads it at every test. So the path answers only once that property is made: the first test of a realm goes the old way and makes it, as before. Without that check a script that tests `instanceof` would end with one function object less, and the goldens of the register loop, which record the objects of the runtime at the end of a run, catch it (98 corpus runs and almost every seed of rvgen).

What it gains: on amd64 an `instanceof` that takes the path costs about 95 instructions on the native C tree. EarleyBoyer of the V8 suite tests 491,887 times an iteration in Boyer and 9,770 in Earley. tifa's pages test 0 (React) to 680 (the store's home page) times a render.

`test_uncatchable_instanceof` in `engine/test/unit-oom.c` leaves an interrupt and an out of memory error pending in an async function, then tests `instanceof` with an object and with a primitive on the left: the run must end with that uncatchable error and the statement after the test must not run.

Tests: `engine/test/compare/instanceof.js`, which prints what Node 24 prints, runs classes and subclasses, built-ins, primitives and objects with a null prototype on the left, arrow, async and bound functions (a bound function with an own `prototype` too), callable proxies, proxies in the chain of either side, an own or inherited `Symbol.hasInstance` (a method, a getter, `null`, not callable, giving values that are not booleans, throwing), one added and deleted between tests and on a parent class, `Object.setPrototypeOf` of the function to an object with one, to `null` and to a proxy, a `prototype` that is a getter (one that changes the chain of the left side, one that tests `instanceof` itself, one that throws), inherited, not an object or not made yet, and the order of the steps the specification makes observable. `engine/test/regvm/js/o-instanceof.js` runs the same across two realms: the code, the right side and the left side from either, a function whose chain goes through the other realm's `Function.prototype`, a shadow on a function of the other realm, and the message and realm of each error. Its throw injection golden has 1,107 places, and its `inject-hit` line makes the set fail when no run throws at `INSTOF`. Each guard of `rv_instof` was taken out in turn, and these tests fail for each (the walk of `Symbol.hasInstance`, the bound and callable checks, the proxy on the left, the value of `prototype`, the free of a temp, and the check that the property is made, which the memory counts of the goldens catch), but for two that stay as a second line of defense. The test of an exotic object in the walk of the right side: a proxy there has no prototype in its shape, so the walk ends at it either way, and no other exotic class answers a symbol from outside its shape. The test that `prototype` is a data property: the slot of a getter or of a prototype not made yet never reads as an object, so the test of the value sends both to the old path anyway.

## Add and sub of two numbers without the shared tail

Release: v0.31.1.

`+` and `-` of two numbers no longer go through the tail that the eight forms of add and sub share (`rv_arith_other`) when the result is not an int from two ints. That covers two floats, an int with a float, and two ints whose sum does not fit in an int. Each handler did two ints itself and jumped to the tail for anything else. The tail then found the form again from the opcode byte, read both operands again and tested each tag twice before it computed. Now the handler tests both tags in one step (`JS_TAG_INT` is 0, `JS_TAG_FLOAT64` is 8, and every other tag has another bit set) and jumps to one block for the eight forms (`rv_arith_num`). The block reads the two operands as doubles and stores a float made by `__JS_NewFloat64`, as QuickJS's `OP_add` and `OP_sub` do: NaN, -0 and the infinities are what the C arithmetic gives, and a whole result such as `1.5 + 1.5` stays a float. The tail keeps two strings and the slow path.

Mul is left as it was. Its handler does two ints and jumps to the tail of the other binary operators (`rv_bin_other`), which already did two numbers in its first lines. Division, `%` and the compares are left as they were too: a NavierStokes or RayTrace iteration and a tifa page run too few of them on floats to pay for the work.

Why this is safe: numbers hold no reference, and for two numbers the slow path only converts them, which cannot throw or call a script. A local that receives the result has its old value freed after the write, as the int case does. Under the throw injection the handlers skip the block, as they skip the int case.

What it costs and gains: each handler gets a test of the tags and a jump. NavierStokes runs 1.9% fewer instructions, MUI's pages 0.25% fewer, and no page runs more. Every other way of placing the code made something run more, because the register allocation of the whole interpreter moved with it:

- the number code written into each of the ten handlers of add, sub and mul: NavierStokes 12% fewer, but the shop's pages 0.12% to 0.17% more, nearly all in the call and return path of the interpreter, which read `ctx` from the stack again where it had kept it in a register
- a call of a function from each handler: NavierStokes 27% more
- the float pair alone inline: the MUI pages 1.1% more
- the three register forms inline and the rest in the block: the shop 0.06% to 0.08% more
- mul through a block of its own as add and sub do, tested in the handler, in its own tail or in an `else` of the int test: NavierStokes 2.6% to 4.1% fewer, but V8's Crypto 0.7% to 1.8% more. Crypto multiplies ints, and the int path of mul kept `ip` on the stack.

Tests: `engine/test/compare/double_arith.js` compares add, sub and mul of floats, ints and their edges (NaN, -0, the infinities, values around 2^53, whole results, int overflow), objects with `valueOf`, strings and the BigInt `TypeError`, in each form, with what Node 24 prints. `engine/test/regvm/js/q4-floatarith.js` runs the forms with old values to free and temps a call below owns, with its goldens for the corpus and the throw injection. A part of `engine/pgo/loops.js` runs the block, which tifa's pages run too little to train it.
