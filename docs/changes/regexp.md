---
summary: 'Changes to regular expressions: skipping to where a regexp can start to match, working out once per regexp where it starts to look, not pushing the alternatives that cannot match at a position, keeping the bytecode of compiled regexps per VM, and parsing the classes of regexps with the v flag as the specification does, with what each gains and costs in memory, which regexps take them, how timeouts still stop them, why a regexp still holds at most 254 capture groups'
read_when:
  - Changing the parser of classes in libregexp.c, or a regexp with the v flag throws a SyntaxError that Node does not
  - A regexp throws "too many captures"
  - Measuring a regexp with BenchmarkRegexColors or BenchmarkRegexShapes
  - A regexp gives another result than before, or a timeout does not stop a regexp
---

# Regular expressions

English · [简体中文](regexp.zh-CN.md)

The engine compiles a regexp to bytecode and runs it in a backtracking machine. The C tree in `engine/` has them in C, as each section says.

## Skip to where a regexp can match

Release: v0.26.15.

A regexp that is not sticky no longer tries every position of the string in the backtracking machine. Its bytecode starts with a loop that tries the match at one position, steps over one character and tries again. A regexp that starts with `^` outside multiline mode is run at position 0 only. Unicode regexps (`u`, `v`), regexps that ignore case or are multiline (`i`, `m`), and every other first op took the old path, until the change for [alternatives](#alternatives-that-cannot-match-are-not-pushed). `lreSkip` checks for an interrupt after every 65,536 characters it looks at, as the backtracking machine does every 10,000 steps, so the eval timeout and `Interrupt` stop a regexp that runs again and again on a long string as before. A server render of a page of tifa's example shop takes about 3% less time on linux/amd64 (AMD EPYC), with the same HTML.

The change is in C (`engine/libregexp.c`): `lre_skip`, `lre_first_set`, `lre_find` and `lre_run`, called by `lre_exec`. A char at the start of a regexp is found in an 8-bit string with `memchr`. `lre_first_set` walks the ways of the splits with a list of its own instead of recursion, which gives the same set and the same budget. `engine/test/compare.sh` runs `testdata/regex.js`, `regexsplit.js`, `regexpv.js`, `shikire.js` and `engine/test/compare/regexskip.js` (strings longer than a chunk, 8-bit and 16-bit, alternations past both budgets).

## Alternatives that cannot match are not pushed

Release: v0.26.17.

A regexp of alternatives no longer tries each alternative that cannot match at a position. An alternation such as `(red|blue|...)` compiles to a chain of split ops, each pushing a state for the next alternative before the machine tries one, so at every position of the string the machine pushed and popped a state for each alternative, which for the 148 color names react-spring tests its values against (`new RegExp('(' + names.join('|') + ')(?!\\w)', 'g')`) was 147 states per position.

- The split ops of `lre_exec_backtrack` ask `lreSplit` first whether the way they push can start at the position: when it begins with a character or a class of characters, after nothing but saves of captures, and the character at the position does not match it, the way would fail as soon as it is popped, and nothing is pushed. When the way the machine takes is another split op, as in an alternation, `lreSplit` goes on with that one in the same call, for up to 4096 split ops, after which it gives back the one it is at to be pushed as usual. It reads the character as the machine does and tests it as the op does, with the same `lre_canonicalize` for a regexp that ignores case, so this holds for every regexp and flag, lookbehinds and sticky regexps included.
- `lreSkip` knows more first ops. `lreFirstSet` follows the bytecode from the start of the regexp through both ways of every split, over saves and assertions (`^`, `$`, `\b`, lookaheads and lookbehinds, to the op after them), up to the first op on each path that reads a character, and collects the characters a match can start with: a set of the 256 Latin-1 units and one bit for all others. It gives up, and the regexp takes the old path, when a path can reach the end without reading a character (the regexp can match the empty string), at a jump back, at an op it does not know (`.`, `\s`, a back reference, a class that ignores case, ...), and after 1024 ops. A character that ignores case counts as itself and its lower case when it is ASCII: outside unicode mode `lre_canonicalize` gives no other unit an ASCII canonical form (`test_canonicalize_ascii` in `engine/test/unit.c` checks every unit). Regexps that ignore case or are multiline now take this path too. `lre_exec` then calls `lreRun` in place of the machine: it tries the match at the first position that can start one, without the loop over positions at the start of the bytecode, and when that fails, finds the next such position in Go and tries again, for the regexps of `lreSkip` that start with one character or class too. That is what the loop does: it tries each position in turn, from the captures as they were before the loop, all cleared, which `lreRun` clears again before each try. It polls the interrupt counter of the machine after each failed try and checks for an interrupt every 65,536 characters it looks at.

Why this is safe: a state that is not pushed is one that would have been popped only to fail at its first character, after saves the machine undoes when it fails, and a position that is skipped is one where no path of the regexp can read its character. Results, captures, `lastIndex` and the errors a regexp throws are the same. The machine counts a step toward its interrupt check, and checks, at each `goto`, each loop and each failed pop, and a greedy `x+` jumps back by a split op, so over a long string it counted only through the pops of the states it pushed. A split op that pushes nothing therefore counts a step and checks itself, and one long match still stops when the eval timeout fires or `Interrupt` is called. What differs is how many steps are counted: one for each call of `lreSplit` that pushes nothing, however many splits it stepped over, where the machine counted one for each state it popped. So an interrupt stops a match after a different number of ops. And states not pushed take no room on the backtracking stack, so a regexp near the memory limit can go on where it ran out of memory.

`BenchmarkRegexColors` times a `replace` with the react-spring regexp of color names: on linux/amd64 (AMD EPYC, one core) it takes 5.9 µs where it took 44.5 on a value with no color name, and 8.5 µs where it took 20.2 on one with a name (the medians of 10 runs). Without PGO, with the same HTML and the same memory of a VM, a render of tifa's example shop takes about 1% to 2.5% less time, and the MUI pages of tifa's test apps about 3% to 5% less. The medians of 10 rounds against `a84c8ed`, before the split ops counted their steps toward the interrupt check, were 0.989 for the shop (0.981 to 1.015 over the rounds) and 0.946 to 0.954 for the MUI pages, and two runs against `80261af` gave 0.983 and 0.987 for the shop and 0.947 to 0.965 for the MUI pages. With the steps counted, two runs of 3 rounds against `de42e83` gave 0.983 and 0.975 for the shop and 0.962 to 0.993 for the MUI pages. The bench pages moved by less than their noise (table varies by 10% between rounds by itself). Other work ran on other cores of the machine during the rounds (a load average of up to 6).

The change is in C (`engine/libregexp.c`): the split ops of `lre_exec_backtrack` call `lre_split`, with the same ops, budget and interrupt step.

## Where a regexp starts is worked out once

Release: v0.30.2.

What [the skip](#skip-to-where-a-regexp-can-match) needs to know of a regexp, which way it looks for a start and the set of characters a match can start with, depends on its bytecode only, yet `lre_exec` worked it out again on every call: `lre_first_set` walked the bytecode for each `test`, `exec` and match of a `replace`. Now `lre_compile` works it out once, in `lre_plan`, and keeps it in 36 bytes right after the bytecode. The scan then uses it in two new ways:

- A regexp whose matches start with one class, or with one of a set of characters, looks for a start in an 8-bit string through a table of 256 bits: one test of a bit for each character, where it made a binary search of the class for each character. In a 16-bit string a class keeps its binary search, since the table holds only the characters below 256.
- A regexp that starts with a literal of several characters, such as emotion's `label:\s*([^\s;{]+)\s*(;|$)`, finds its first character with `memchr` as before, then compares the next ones, up to 255 of them, and runs the backtracking machine only where the string holds the whole literal. A regexp that ignores case does not take this, since its first op is not a plain character, and neither does a unicode or sticky one, which has no loop to skip.

Why this is safe: the table holds the set `lre_first_set` gave, so the same positions are tried. A position where the literal is not whole is one where the machine fails before it reads past the literal, after saves it undoes, and its captures stay cleared. The matches, the captures, `lastIndex` and the errors are the same. The scan still reads the interrupt flag every 65,536 characters, and a class in a 16-bit string every 16,384, a quarter, because its binary search costs up to four times what the table costs for each character. A position the literal rules out still polls the counter of the machine, as a failed try did. Before, the binary search of seroval's class in a string of 32 Mi characters that never matched let 11 to 14 polls see a stop, where the eval timeout allows `stepStopPolls`. Now one of each way of the scan stops within them.

The plan depends on the bytecode only. Regexps that share their bytecode, the objects of one literal, `new RegExp(re)` and the objects the [cache](#compiled-regexps-kept-per-vm) gives, in any realm of the runtime, read the same plan, and nothing writes it after `lre_compile`. Its 36 bytes are part of the bytecode string, so they count against the memory limit of the VM and the bytes of the cache, and are freed with the bytecode.

The bytecode of a function holds the bytecode of its regexp literals, so the bytecode version goes from 8 to 9: bytecode written by `Compile` of an earlier release no longer loads (`SyntaxError: invalid version`), and has to be compiled again. A plan read back is the one its writer made: the machine takes the bytecode of a regexp it reads as it is, and the plan adds nothing to that.

Callgrind on linux/arm64, the instructions of the engine in a render, against `abe4e5a9` with the same HTML: the shop's workbench page 0.57% fewer, its English storefront home 0.96%, its Chinese product list 0.36%, the MUI orders page 0.97% and `r_react_ssr` 0.27%. For the patterns they run most, the matcher and its scan take, per render:

| Pattern | Library | Before | After |
| --- | --- | ---: | ---: |
| seroval's test of characters to escape, `[\x00-\x07\x0b\x0e-\x1f<\u2028\u2029\ud800-\udfff]`, English storefront | seroval | 1,350K | 242K |
| the same, MUI orders | seroval | 512K | 93K |
| the same, Chinese product list, where the value is a 16-bit string | seroval | 861K | 848K |
| the trailing spaces of a URL, `(^\|[^\u0000-\u0020])[\u0000-\u0020]+$`, English storefront | core-js | 4,553K | 3,840K |
| `[\x00-\x1f\x7f]`, English storefront | TanStack Router | 211K | 79K |
| `[%\\\x00-\x1f\x7f]`, English storefront | TanStack Router | 228K | 74K |
| `label:\s*([^\s;{]+)\s*(;\|$)`, MUI orders | emotion | 3,889K | 2,428K |
| `["'&<>]`, `r_react_ssr` | React DOM | 581K | 493K |
| the 148 color names, workbench | react-spring | 3,958K | 3,523K |

`BenchmarkRegexShapes` and `BenchmarkRegexColors` give the gain for each shape without tifa's pages. On darwin/arm64 (Apple M, with other work on the machine), the medians of 6 rounds that ran both builds in turn, ns a run, and the median of the ratios of the rounds:

| Shape | Before | After | Ratio |
| --- | ---: | ---: | ---: |
| seroval's escape test on a value of 867 characters (`escape-class`) | 3,966 | 677 | 0.19 |
| core-js's trailing spaces of a URL (`url-trailing`) | 1,569 | 889 | 0.55 |
| emotion's labels in a style of 322 characters (`emotion-label`) | 423 | 296 | 0.70 |
| react-spring's color names, a value with a name (`name`) | 6,410 | 5,499 | 0.84 |
| react-spring's color names, a shadow with no name (`shadow`) | 4,819 | 4,413 | 0.94 |
| React's escape of a short label (`html-escape`) | 68 | 65 | 0.94 |
| react-spring's numbers in a value (`number-match`) | 1,726 | 1,592 | 0.93 |

The last three moved by less than the spread of their rounds.


The change is in C (`engine/libregexp.c`): `lre_plan`, called by `lre_compile`, `lre_find_set` and `lre_prefix`, and `lre_skip` and `lre_run`, which read the plan. `lre_get_groupnames` and `lre_dump_bytecode` step over it, and `BC_VERSION` in `engine/quickjs.c` is 9. `engine/test/unit.c` checks how often each way of the scan polls, `engine/test/unit-api.c` that bytecode with regexps is written and read back and that an older version is refused, and the scripts `q3-regexp-skip.js` and `q3-regexp-prefix.js` of `engine/test/regvm/js` give the results of regexps of each kind, as Node gives them. `TestRegexLongScanStops` stops one long scan of each way, and `BenchmarkRegexShapes` times the shapes above.

## Long lists of alternatives that start with a literal

Release: v0.30.2.

An alternation such as react-spring's 148 color names, `(aliceblue|antiquewhite|...|yellowgreen)`, compiles to one split op for each alternative but the first, one after the other. At each position the machine came to, `lre_split` stepped over all of them to find which alternatives can start there, 147 steps whatever the character. Now `lre_compile` finds each run of at least 15 such split ops (16 alternatives) whose alternatives all start with a literal character, and keeps for it, after the plan, which is the first alternative that starts with each character and, for each alternative, the next one that starts with the same. `lre_split` reads the character once and goes straight to the first alternative that can start with it, or past the run when none can. After it pushed an alternative, the machine comes back to the next split op, and `lre_split` goes on from the next alternative with the same character.

- A run ends at an alternative that does not start with a literal: a class, a dot, an escape such as `\d` or `\b`, an empty alternative, a group of other modifiers that changes `i`. The alternatives after it make a run of their own. At most 4 runs a regexp are kept, the first in the code.
- With `i`, the alternatives start with folded characters (`char_i`), and `lre_split` folds the character of the string as the machine does, so `K` and `k` find the same alternatives, and under `iu` the Kelvin sign too. A run does not mix alternatives that ignore case with ones that do not. With `u` and `v` a character above U+FFFF is read whole, as the machine reads it.
- Characters from 256 up share one entry. `lre_split` then tests those alternatives one at a time, as before.
- An alternation inside a lookbehind is read backward, and its alternatives start with a step back, so it makes no run. A lookbehind before an alternation does not change the alternation.

Why this is safe: an alternative is skipped only when its first character is not the one in the string, where `lre_split` would have tested it and pushed nothing. So every call of `lre_split` gives the machine what the walk over the split ops one at a time gave: the same alternatives pushed in the same order, so among those that start with the same character the order of the pattern holds (`a|ab` still prefers `a`), and the same calls that push nothing, after which the machine reads the interrupt flag. It also counts each split op it skips toward the 4096 steps of one call and stops where the walk would have stopped. Matches, captures, `lastIndex`, the errors and how often the flag is read are the same.

A run takes 8 bytes and 2 more for each of 257 entries and for each of its split ops, 816 bytes for the color names, in the bytecode string: they count against the memory limit and the bytes of the cache. The bytecode of a function holds the bytecode of its regexp literals, so the bytecode version goes from 9 to 10: bytecode written by `Compile` of an earlier release no longer loads (`SyntaxError: invalid version`).

Callgrind on linux/arm64, the instructions of the engine in a render, against `641f7dc7` with the same HTML: the shop's workbench page, where react-spring tests its color names, 1.36% fewer (172.32M to 169.98M), and in the matcher 4,877K to 2,601K. On the other pages no regexp has such a list, and the matcher runs up to 69K more instructions a render, for the test of whether a regexp has a run: the English storefront home +0.02%, the Chinese product list +0.01%, the MUI orders page +0.02% and `r_react_ssr` +0.01%.

`BenchmarkRegexColors` on darwin/arm64 (Apple M, with other work on the machine), the medians of 6 rounds that ran both builds in turn: a value with a name 3,971 ns to 1,921 ns (0.48), a shadow with no name 3,132 ns to 878 ns (0.28). The shapes of `BenchmarkRegexShapes`, which have no such list, moved by less than the spread of their rounds.


The change is in C (`engine/libregexp.c`): `lre_chains`, called by `lre_compile`, and `lre_chain_next` and `lre_split`, which read the chains. `lre_get_groupnames` and `lre_dump_bytecode` step over them, and `BC_VERSION` in `engine/quickjs.c` is 10. `engine/test/unit-white.c` checks which runs are kept (`test_regexp_chains`) and that `lre_split` gives what the walk one at a time gives at every split op of each kind of run, on characters that fold to one another, from 256 up and above U+FFFF and at the end of the string, and for runs of 4095 to 4097 split ops (`test_regexp_split_same`). `engine/test/unit.c` checks that the flag is read as often as before. `q3-regexp-alt.js` of `engine/test/regvm/js` gives the matches and captures of such regexps under every builtin, as Node gives them, and `TestRegexLongScanStops` stops a long run of them.

## Compiled regexps kept per VM

Release: v0.27.0.

A VM keeps the bytecode of the regexps it compiled. Code that makes a new `RegExp` of the same pattern on every call compiled it every time: a syntax highlighter made anew for each render, as react-markdown makes one, compiles the regexps of its grammar again on each page.

Why this is safe: the bytecode is a string the engine never changes once `lre_compile` wrote it, and `lastIndex` is a property of each object, so objects made from the cache share it, as the objects of one literal and `new RegExp(re)` already did. Only a pattern that is a flat string, with flags that are undefined or a flat string, is looked up and kept: converting anything else can run a script or allocate, which `js_compile_regexp` does before it compiles, and a rope (a string made of others and not yet flattened) is converted too. A compile that fails is not kept, so a pattern that throws a `SyntaxError` throws it every time. What differs: a hit can succeed where compiling again would fail, for lack of memory or of stack.

The cache keeps at most 256 regexps in 1 MiB, counting each entry, its pattern, flags and bytecode, and lets go of the one used least recently first. Its table of hash chains is not in those bytes: it has one pointer for each entry that fits in them, at most 65,536 (512 KiB on 64-bit targets), however many entries the limit allows, and counts against the memory limit of the VM. With the defaults it takes 2 KiB. The limits come from the pages measured: the journal post of tifa's example shop that highlights CSS compiles 392 regexps per render, 162 of them different, which take 714 KiB, so they fit with room to spare. The other pages of the shop and of an app built on MUI compile at most 83 per render, of at most 11 patterns, in under 15 KiB. A new VM keeps `DefaultRegExpCache` regexps in `DefaultRegExpCacheBytes`, and [`VM.SetRegExpCache`](go-api.md#setregexpcache-and-defaultregexpcache) changes it. `JS_FreeRuntime` lets every entry go first.

Memory: the entries are kept in C memory, in the state of the runtime next to the [shape cache](objects-and-shapes.md#kept-shapes), so they count against the memory limit of the VM. They are not let go when an allocation fails: freeing a string can free an atom, and the allocation that fails can be one in the middle of growing the atom table. So a script that runs close to its memory limit can run out of memory with the cache on where it would not without it: give such a VM a smaller cache, or none. One VM of the shop holds 12 KiB more after a GC (12.84 MiB rather than 12.83, measured with tifa's `TestShopVMMemory`), and a VM that rendered the CSS post holds the 714 KiB of its 162 regexps.

Without PGO, on linux/amd64 (AMD EPYC 7B13, one core), against `ff35232` (the engine of v0.26.17), the CSS post renders in 0.893 of the time: the median of 6 rounds in 3 code layouts, 0.841 to 0.903 over them. The other pages, which compile no regexp after their first render, render in the same time: 1.003 for the shop, 1.000 for the other store pages and 0.999 for the MUI pages, inside the 1% by which the code layout moves a render. The HTML is the same.

The change is in C (`engine/quickjs.c`): the field `regexp_cache` of `JSRuntime`, `js_regexp_cache_get` and `js_regexp_cache_put` in `js_compile_regexp`, `JS_SetRegExpCache(rt, n, bytes)` and the defaults `JS_REGEXP_CACHE_ENTRIES` and `JS_REGEXP_CACHE_BYTES` in `quickjs.h`, and `JS_FreeRuntime` lets every entry go. `engine/test/compare.sh` runs `testdata/regexcache.js`, and `engine/test/unit.c` checks that regexps of the same pattern share their bytecode only with the cache on. It also checks the limits of entries and of bytes, that setting them lets every entry go, that a regexp works once the cache let its bytecode go, that the cache keeps its entries and works after allocations that fail in turn, and that it outlives the context that filled it.

## Classes of regexps with the v flag

Release: v0.28.3.

The parser reads a class of a regexp with the v flag (unicodeSets) as the specification does. It threw a SyntaxError for three things the specification allows and Node accepts:

- In a class, `\` before one of ``& ! # % , : ; < = > @ ` ~`` (a ClassSetReservedPunctuator) is that character, as in `[\&\;]` or `[\!\|]`. `get_class_atom` took `\-` already, and threw "invalid escape sequence" for the others. Outside a class, and in a class of a regexp with the u flag, these escapes stay a SyntaxError, as the specification says.
- `get_class_atom` also reads a character outside a class, and checked it as in a class: a doubled punctuator such as `&&`, and `( ) [ ] { } / - |`, threw there too, so `/-/v`, `/a&&b/v` and `new RegExp('/', 'v')` did not compile. This is upstream's commit a479eff ("fixed regexp unicode set parsing", after the release this fork follows), which checks them only in a class.
- `\q{...}` read its characters as outside a class. With a479eff alone, `\q{(}`, `\q{-}` and `\q{&&}` would then compile, which Node rejects. It reads them as in a class instead, which also lets `\q{\-}` and `\q{\&}` compile.

shiki's JavaScript engine, which shiki offers in place of its Oniguruma engine where there is no WebAssembly, as in this engine, turns the grammars it highlights with into regexps with the v flag full of such escapes: of the 552 regexps it makes for its grammars of bash, css, go, html, json, typescript and yaml, the engine threw for 204. Now it compiles all of them but 3, which have more capture groups than a regexp can hold (see [At most 254 capture groups](#at-most-254-capture-groups)).

Why this is safe: a pattern that compiled before compiles to the same bytecode. Only patterns that threw now compile, and only those the specification allows, as `engine/test/compare/recorded/regexpv.js` checks against Node for every punctuator escaped and not, in and out of classes and `\q{}`, with the v flag, the u flag and neither. A native build of the patched C gives the same transcripts as Node.

## At most 254 capture groups

A regexp holds at most 254 capture groups, as in upstream QuickJS and QuickJS-NG, and throws `SyntaxError: too many captures` past that. V8 allows 32,767. shiki's JavaScript engine makes three regexps with more, for its grammars of typescript (551 and 555 groups) and html (912), so those two grammars still fail to highlight in this engine.

The limit stays, as raising it changes the format of the bytecode of every regexp. The bytecode keeps the index of a group in one byte: in the ops that save where a group starts and ends, in the op that clears groups before a quantifier repeats, in the list of groups of a back reference, and in the header, which holds the count. Two bytes for each costs 2 bytes more for each group, 2 for each clearing op and 1 for each group of a back reference, and the header grows by 2. A match takes 16 bytes of heap for each group, for the array of where groups start and end, and the backtracking stack saves a group only when the match changes it, so no stack grows with the number of groups. A native build of the C with two-byte indices and a limit of 32,767 gives Node's transcripts, highlights the code of `testdata/shiki/input.md` with shiki as Node does, typescript and html included, and runs regexps with and without groups in the same time within the noise.