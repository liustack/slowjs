---
summary: 'The fork changes to operators: bitwise operators on two numbers and the usual pairs of ===, !==, == and != answered without the slow paths, with what each gains, why it is safe'
read_when:
  - A bitwise operator or a comparison gives another result than before, or a compared value is freed twice
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

