---
summary: 'The commits of upstream QuickJS taken into engine/ one at a time after the pinned 3d5e064 (Array slice and splice, borrowed operands in js_strict_eq2, template literals, unicode_script), the faster indexOf and includes that perf found, the ones tried and taken back, the BigInt comparison fixed in the fork, and the rest that are left out, with tests and speed'
read_when:
  - Asking which upstream QuickJS commits after 2026-06-04 the C tree has, and which it does not
  - Changing js_array_slice, js_array_splice, js_strict_eq2, js_array_indexOf, js_array_includes, js_parse_template or unicode_script in engine/
  - Taking another commit of upstream QuickJS into engine/
  - Reading the speed of a C engine commit that came from upstream
---

# Upstream commits taken into the C tree

`engine/` starts from QuickJS `3d5e064` (2026-06-04) and changes on its own ([c-engine.md](c-engine.md)). Upstream has added 81 commits since (to `19d6bd8`, 2026-10-26). This page lists the ones taken in, each as a commit of its own that names the upstream commit, the ones tried and taken back, and the ones left out. It is not a promise to follow upstream. A commit is taken when it makes tifa's renders faster, or removes a crash or a wrong answer that a page can reach, and every one comes with a test that prints what Node 24 prints (or, for the crash, a sweep of the memory left).

Where an upstream commit meets a change of the fork, the fork's change stays and the upstream text is merged around it. To see whether a commit still applies, run `git apply --directory=engine --check` on its diff.

| Upstream commit | What it does | How it was taken | Test |
| --- | --- | --- | --- |
| `42d08be` optimized Array.prototype.slice and splice | slice and splice of a dense Array copy or move the elements directly | all of it, with `js_call_constructor` (which counts the call depth) where upstream calls `JS_CallConstructor` | `compare/array_slice_splice.js` |
| `8f0c037` use JSValueConst arguments for js_strict_eq2 | `indexOf`, `lastIndexOf`, `includes`, `Map` and `Set` stop duplicating and freeing the values they compare | all but its last hunk, which edits macros the fork does not have | `compare/strict_equal.js` |
| `f8199dc` Fix template literal substitution | a template converts each substitution to a string at once, as the specification orders, and no longer calls `String.prototype.concat` | all of it | `compare/template_literal.js` |
| `817509a` removed infinite loop in unicode_script() | the out of memory path of `\p{Script=...}` returns instead of jumping to itself | all of it | `unit-oom`, `test_regexp_script_oom` |

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

## What changes for a program

- A template literal converts each substitution to a string before it evaluates the next one, so `${c} ${c.n++}` and a `Date` that a later substitution changes print what Node prints. A changed `String.prototype.concat` no longer changes templates. A symbol in a template still throws a `TypeError`.
- A regexp with `\p{Script=...}` or `\p{Script_Extensions=...}` that cannot be compiled for lack of memory ends the call with the out of memory error. It crashed the process before (clang took the endless loop for undefined behaviour).
- Comparing a negative BigInt with a negative float gives the right answer.

## Left out

Judged by reading each diff against the fork's tree. None is a speed change tifa can use.

- Fixes of the specification that a page compiled by Vite does not reach: `abf571a`, `8ac9d9b`, `5b6356a`, `4984249`, `cfc9464`, `df8de73`, `13876a2`, `55c5f0b`, `fce112d`, `efc48a3`, `ce0dbb7`, `535a7c2`, `1aa7429`, `8e68fcf`, `1134912`, `1ec58aa`, `0808cbc`, `19d6bd8`, `c17f7fc`, `d88b1ed`, `21cbd86` (which also changes the message of `repeat`), `9a0197f` (sort writes back every element, which costs writes), `a479eff` (the v flag).
- Two leaks on paths a page does not take: `c8ef889` (`Atomics.store` out of bounds) and `79f58c1` (a BigInt in truncated bytecode).
- Already fixed in the fork: `cd97c2b` (the stack of a syntax error out of memory, upstream #529).
- `4b0addb` (`JS_IteratorNext` without two unused arguments) is a cleanup of a signature. It was not tried.
- New features, which Vite compiles away: the iterator helpers, `Promise.allKeyed`, `Array.fromAsync`, explicit resource management, `Atomics.waitAsync`.
- API for embedders that tifa does not use (the `JS_To*` exports and the like), upstream's Temporal and Intl (the fork has go-intl) and upstream's test tools.

