---
summary: 'Changes to operators: bitwise operators on two numbers, the usual pairs of ===, !==, == and !=, and add and sub of two numbers answered without the slow paths, with what each gains, why it is safe'
read_when:
  - A bitwise operator, a comparison, an add or a sub gives another result than before, or a compared value is freed twice
---

# Operators

English · [简体中文](operators.zh-CN.md)

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


## Add and sub of two numbers without the shared tail

Release: Unreleased.

`+` and `-` of two numbers no longer go through the tail that the eight forms of add and sub share (`rv_arith_other`) when the result is not an int from two ints. That covers two floats, an int with a float, and two ints whose sum does not fit in an int. Each handler did two ints itself and jumped to the tail for anything else. The tail then found the form again from the opcode byte, read both operands again and tested each tag twice before it computed. Now the handler tests both tags in one step (`JS_TAG_INT` is 0, `JS_TAG_FLOAT64` is 8, and every other tag has another bit set) and jumps to one block for the eight forms (`rv_arith_num`). The block reads the two operands as doubles and stores a float made by `__JS_NewFloat64`, as QuickJS's `OP_add` and `OP_sub` do: NaN, -0 and the infinities are what the C arithmetic gives, and a whole result such as `1.5 + 1.5` stays a float. The tail keeps two strings and the slow path.

Mul is left as it was. Its handler does two ints and jumps to the tail of the other binary operators (`rv_bin_other`), which already did two numbers in its first lines. Division, `%` and the compares are left as they were too: a NavierStokes or RayTrace iteration and a tifa page run too few of them on floats to pay for the work.

Why this is safe: numbers hold no reference, and for two numbers the slow path only converts them, which cannot throw or call a script. A local that receives the result has its old value freed after the write, as the int case does. Under the throw injection the handlers skip the block, as they skip the int case.

What it costs and gains: each handler gets a test of the tags and a jump. NavierStokes runs 1.9% fewer instructions, MUI's pages 0.25% fewer, and no page runs more. Every other way of placing the code made something run more, because the register allocation of the whole interpreter moved with it:

- the number code written into each of the ten handlers of add, sub and mul: NavierStokes 12% fewer, but the shop's pages 0.12% to 0.17% more, nearly all in the call and return path of the interpreter, which read `ctx` from the stack again where it had kept it in a register
- a call of a function from each handler: NavierStokes 27% more
- the float pair alone inline: the MUI pages 1.1% more
- the three register forms inline and the rest in the block: the shop 0.06% to 0.08% more
- mul through a block of its own as add and sub do, tested in the handler, in its own tail or in an `else` of the int test: NavierStokes 2.6% to 4.1% fewer, but V8's Crypto 0.7% to 1.8% more. Crypto multiplies ints, and the int path of mul kept `ip` on the stack.

The counts of each are in [performance.md](../performance.md#numbers-added-and-subtracted-without-the-shared-tail).

Tests: `engine/test/compare/double_arith.js` compares add, sub and mul of floats, ints and their edges (NaN, -0, the infinities, values around 2^53, whole results, int overflow), objects with `valueOf`, strings and the BigInt `TypeError`, in each form, with what Node 24 prints. `engine/test/regvm/js/q4-floatarith.js` runs the forms with old values to free and temps a call below owns, with its goldens for the corpus and the throw injection. A part of `engine/pgo/loops.js` runs the block, which tifa's pages run too little to train it.
