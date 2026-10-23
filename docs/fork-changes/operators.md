---
summary: 'The fork changes to operators: bitwise operators on two numbers and the usual pairs of ===, !==, == and != answered without the slow paths, with what each gains, why it is safe and what to do when upstream changes it'
read_when:
  - Changing lib/bitwise.go, lib/eq.go, fixBitwise or fixEq in internal/fixlib
  - fixlib stops on bitwiseSites or eqSites after an upstream release
  - A bitwise operator or a comparison gives another result than before, or a compared value is freed twice
---

# Operators

The interpreter does an operator itself only for the simplest operands, two ints, and calls a slow function of the engine for everything else. These changes answer more pairs without that call. Both change only the generated Go, through `internal/fixlib`, with no C form.

## Bitwise operators on floats skip the slow path

Release: v0.26.17.

`&`, `|`, `^`, `<<`, `>>` and `>>>` of two numbers no longer take the slow path when one of them is a float. The interpreter did the operator itself only for two ints and called `js_binary_logic_slow` or `js_shr_slow` for everything else, which convert both operands with the checks for BigInts and objects and then with the generic `ToInt32`. A product past 2^31 is a float, so the string hash of emotion, the CSS-in-JS library of MUI, took that path for almost every operator. The two slow functions now first call `bitwiseNumbers` (`lib/bitwise.go`), before they take their TLS frame. It does the operator for any two ints or floats, converting a float as `JS_ToInt32Free` does (the same bits, without `fmod`), and stores an int, or for `>>>` a float above 2^31 - 1, as `JS_NewUint32` does. Any other pair goes on through the rest of the slow path as before.

Why this is safe: numbers hold no reference, so nothing is freed, and the slow paths do nothing else for two numbers, since converting a number to a numeric is a no-op and cannot throw or call a script. The interpreter is left as it is: calling `bitwiseNumbers` from its six handlers instead made the shop 0.8% slower. All 5 files change, because `bitwiseNumbers` reads their struct `JSValue`. `fixlib` pins the text of `js_binary_logic_slow`, `js_shr_slow`, `JS_ToNumberHintFree` and `JS_ToInt32Free` by fingerprints (`bitwiseSites`), and stops when the interpreter no longer has 5 calls of the first and 1 of the second, or when either no longer takes its TLS frame first. This changes only the generated Go, with no C hunk (`internal/fixlib/bitwise.go`).

On linux/amd64 without PGO, against v0.26.16 over 10 rounds, the pages of tifa's MUI app, which run about 130,000 to 300,000 of these operators on floats per render, take 0.952 to 0.960 of the time (0.935 to 0.969). The shop runs the slow paths 3 to 24 times per render and the bench app not at all, so they have nothing to gain: the shop measured 1.008 (0.999 to 1.042), which is code layout, not work. The HTML and the memory of a VM stay the same.

Tests: `lib/bitwise_test.go`, whose `testdata/bitwise.txt` is what the engine gave before the change (Node gives the same) and which checks `bitwiseNumbers` on about 41,000 pairs of numbers against `ToInt32` of the specification (which the engine's `JS_ToInt32Free` must also give) and the operator done in Go, and `internal/fixlib/bitwise_test.go`.

### When upstream changes it

What to do when `go run ./internal/fixlib` stops on this change after an upstream release. [upgrading.md](../upgrading.md) has the whole procedure and the order in which to update the lists `fixlib` checks against.

If it reports that `js_binary_logic_slow`, `js_shr_slow`, `JS_ToNumberHintFree` or `JS_ToInt32Free` changed (a fingerprint other than the one in `internal/fixlib/bitwisesites.go`), read them against `lib/bitwise.go`: for two ints or floats the slow paths must still give `ToInt32` of each, the same operator with the count masked to 5 bits, and an int, or a float for a `>>>` above 2^31 - 1. If it reports another number of calls in `_JS_CallInternal`, read the new interpreter: each call of the slow functions must have the two operands on top of the stack, and the opcode in `op`. Then put the new fingerprints in (`go run ./internal/fixlib -bitwise` prints them) and run `go test -run Bitwise ./lib`.

## The usual comparisons answered in the interpreter

Release: v0.26.17.

`===`, `!==`, `==` and `!=` no longer call the slow path for the usual pairs. The interpreter compared two ints itself and called `js_strict_eq_slow` or `js_eq_slow` for everything else, which for `===` went on to `js_strict_eq2`. React compares with `null` and `undefined` (minified as `null != x` and `void 0 === x`, which the compiler does not turn into `is_null` or `is_undefined`), compares types, symbols and objects by identity, and switches over strings. The interpreter now answers these itself:

- objects and symbols by pointer
- `null`, `undefined` and booleans by tag and value
- strings when they are the same string, have different lengths or are both string atoms (the atom table holds each text once)
- for `===` and `!==`, values of different tags when one is `null`, `undefined`, a boolean, an object or a symbol
- for `==` and `!=`, `null` or `undefined` beside anything (equal only to `null`, `undefined` or an `IsHTMLDDA` object)

Other pairs of different tags under `==`, such as `true == 1` or `{} == 'x'`, convert their operands and still take the slow path, as do numbers, BigInts, ropes and strings that need their characters compared.

Why this is safe: each answer is the one the slow path gives for that pair, none of them can throw or call a script, and both operands lose their reference as `JS_FreeValue` does, freed through `eqFree` (`lib/eq.go`) when it was the last. Strings that are both atoms are told apart only when both are string atoms: the description of a symbol is an atom of another type and can hold the text of a string atom. The text is written into the four handlers, with no call on the usual path, since Go does not inline into a function as big as the interpreter. It passes `eqFree` the stack pointer rather than the values, so the frame of the interpreter changes little: 8 bytes more on amd64 and 16 fewer on arm64, from v0.26.16. All 5 files change, because the text reads their struct `JSValue`. The bit fields it reads (the length and atom type of a string, the `IsHTMLDDA` bit) come from each file's `js_string_eq`, `__JS_FreeValueRT` and `JS_IsHTMLDDA`, and the layout of a value (how far below its pointer the reference count sits, the size of a value on the stack and how a boolean is stored) from its `JS_FreeValue` and `js_strict_eq_slow`. `fixlib` pins those and `js_strict_eq_slow`, `js_strict_eq2`, `js_eq_slow` and `__JS_FreeValue` by fingerprints (`eqSites`), and stops when the interpreter no longer has one call of each slow path per operator. This changes only the generated Go, with no C hunk (`internal/fixlib/eq.go`).

On linux/amd64 without PGO, against v0.26.16 over 10 rounds, a render of tifa's example shop takes 0.982 of the time (0.974 to 0.989), the spiral and table pages of its bench app 0.940 and 0.942 (with wide ranges, 0.859 to 0.969 and 0.871 to 1.030), and its MUI pages 0.982 to 0.985. The HTML and the memory of a VM stay the same.

Tests: `lib/eq_test.go`, whose `testdata/eq.txt` is what the engine gave before the change (Node gives the same) and which checks that compared values are freed once, and `internal/fixlib/eq_test.go`.

### When upstream changes it

If it reports that `js_strict_eq_slow`, `js_strict_eq2`, `js_eq_slow`, `js_string_eq`, `__JS_FreeValueRT`, `__JS_FreeValue` or `JS_IsHTMLDDA` changed (a fingerprint other than the one in `internal/fixlib/eqsites.go`), read them against the cases in the comment of `fixEq`: each pair it answers must still get that answer from the slow path, with both operands freed, and string atoms of one text must still be one atom. If it reports that a bit field, `JS_FreeValue` or `js_strict_eq_slow` changed shape, or that a slow call pops another size than a value, find where the bits, the reference count or the result are now and update the patterns in `internal/fixlib/eq.go`. If it reports another number of calls in `_JS_CallInternal`, read the new interpreter: each slow call must still have the two operands on top of the stack and pop one. Then put the new fingerprints in (`go run ./internal/fixlib -eq` prints them) and run `go test -run Eq ./lib`.
