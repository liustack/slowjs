---
summary: 'Changes to how the engine is generated from the C: the profile of tifa pages that make gen compiles with (what it is, how it was found, what it may decide and what is pinned off, why it is safe, what a stale one costs, the second input of plain loops, its limits and checks), the Go declaration of a register-entry leaf that has a frame, and the one address of a local a phi gets per incoming block'
audience: dev
order: 13
read_when:
  - Asking why make gen compiles twice, or what engine/pgo/profile.txt is for
  - make gen prints a line that starts with "warning: profile:" or "profile:", or a generated function changed after a change of the C that did not touch it
  - Changing the flags the generator gives clang, opt or llc, or the inlining of the generated engine
  - Asking what the profile gains, what it costs, and which scripts it can make slower
  - Training the profile again, or taking a commit of QuickJS that changes the register loop
  - train.sh stops at its check of engine/pgo/loops.js, or asking why the micro benchmarks are not trained on
  - go vet says `function memmove missing Go declaration` or the like for a generated .s file
  - opt stops in the shadow-stack pass with `PHI node has multiple entries for the same basic block with different incoming values!`
---

# The generator

How `make gen` turns the C of `engine/` into the Go assembly in `internal/cengine`. The rest of the generator is in [its README](../../../internal/goasm/gen/README.md). Here are the changes to how it is generated.

## A profile of tifa's pages

Release: v0.30.5.

`make gen` compiles the engine with a profile: counts of how often each branch ran while the engine rendered real pages. LLVM uses the counts to put the likely blocks next to each other, to place spills where the code runs least, and to lay out `switch` cases. The profile is `engine/pgo/profile.txt`, trained on the pages in `engine/pgo/pages` (see [engine/pgo/README.md](../../../engine/pgo/README.md)). On an AMD EPYC server it makes the pages of tifa render 5.4% to 8.4% faster than the same C generated without it, and about 3% faster on an Apple M4.

For v0.30.6 the profile was trained again on the C of that release, with its three new changes (2,774 functions, none stale).

### What make gen does with it

1. First pass, as before and without the profile. It also writes down which calls were inlined (`-Rpass=inline`, `-gline-tables-only`).
2. Second pass: clang and the optimizer over the linked module again, with `-fprofile-instr-use`, and with the inlining held to the first pass: a call is inlined if the first pass inlined it and not otherwise (`-cgscc-inline-replay`, scope Module, fallback NeverInline). llc runs once on the result.

The notes of the first pass are made again from the current C every time, so they cannot go stale. The profile is one text file for amd64 and arm64: the hash of a function depends on the C only. `make gen` turns it into the indexed form in `.build` with `llvm-profdata` of LLVM 22.1.8. The output is a function of `engine/`, the profile file and the flags, so `make check-gen` keeps working. On one core of the build server (linux/amd64, `GEN_JOBS=1`), `make gen` takes 4 minutes 27 seconds for amd64, where it took 1 minute 52 without the profile, and 3 minutes 39 for arm64.

The profile is not made by `make gen`, because training twice gives files that differ in a few counts. It is a checked-in file. `GEN_PGO=0 make gen` generates without it, for comparison, and is not a formal build.

### How it was found

The first try, in October 2026, used the plain profile. It made the generated engine slower: the profile changes what gets inlined, in both directions. `JS_RegvmRun` (the register loop) grew 3.4 times, its Go frame went past the 256 bytes the frame rule holds, and React's page ran 10.1% more instructions (the other pages 0.1% fewer to 1.8% more). It is in [What was tried and not done](../performance.md#what-was-tried-and-not-done).

The second try, on 2026-11-13, kept the inlining as the build without a profile makes it and let the profile decide the rest. That is where the gain is. What the study checked:

- With the weights taken off the IR just before llc, the profile gains nothing (a render ran 0.6% to 1.3% more instructions than without it). The gain is llc's: where it puts blocks, spills and splits of live values, and which side of a branch falls through.
- It does not come from having seen the page. Trained on one half of the pages and tested on the other half, the gain is the same on every page, within 0.2 points of instructions.
- A profile cut down to the register loop, its 20 hottest helpers and what is inlined into them gives about half of the gain.
- A profile trained before two small changes of the C (an empty string on the left of `+`, one case of a property write) kept all of the gain on the changed C.

### What the profile decides

Allowed:

- llc's block weights and layout, and the order of the cases of a `switch`. This is most of the gain.
- Loop peeling by the number of turns the training saw, for a loop with an estimated count of 7 or less: those first turns are copied out in front of the loop. The build without a profile peels 1 or 2 turns, and only where the code calls for it. Its cost depends on the data: a loop the training saw at 3 turns that runs a thousand pays for the copies at each entry, and a peeled loop can lose the unrolling it had. Measured and allowed: a cap of 2 turns (`-unroll-peel-max-count=2`, which also caps the peeling the build without a profile does) changes 114 functions on amd64, `JS_RegvmRun` among them, and a render runs 0.34% to 1.11% more instructions (store +0.58%, workbench +0.72%, MUI +0.34%, React +1.06%, the 400 KB Markdown +1.11%), the 64 micro benchmarks 0.52% more (`array_push` +4.2%, `weak_map_delete` +3.5%, `float_toFixed` +3.4%, and only `array_for_in` fewer, by 1.0%). A cap of 1 turn costs 1.51% to 1.80% a render and 1.07% on the micro benchmarks. Timed on the AMD EPYC server (16 layouts, delta 1%, the mean of the layouts), the cap makes MUI 1.66% slower, the store 1.02% and the 100 KB Markdown 1.57%, and shows no regression beyond the delta or cannot tell on the others.
- SimplifyCFG keeps a branch the training saw go one way 99% of the time as a branch, where it would make a select, and does not merge its conditions. About 108 functions differ. Its cost depends on the data too: on data that goes both ways the branch is mispredicted where the select had no branch. There is no clean switch to pin it (the same threshold decides the branches of `likely()`), so it is allowed.
- The flat-loop threshold of the unroller: a loop the training ran under 5 turns gets no runtime unrolling. Measured and allowed, since it decides almost nothing: at 0 (no loop is flat) only `js_Date_parse` changes on amd64 and nothing on arm64, and a render runs 0.01% to 0.03% fewer instructions. At 1,000,000 (every loop the profile counts is flat) 20 functions change on amd64, `JS_RegvmRun` among them, and 2 on arm64, and a render runs 0.44% to 1.14% more instructions (store +0.57%, workbench +0.77%, MUI +0.44%, React +1.14%, the Markdown +0.02%), the micro benchmarks the same.
- LoopSink: work moves out of the block before a loop into the blocks of the loop the training ran little. LLVM runs it only with a profile. Measured and allowed: turned off (`-max-uses-for-sinking=0`) it changes 92 functions on amd64 with several Go frames 16 bytes larger, and a render runs 0.01% to 0.32% more instructions (store +0.32%, workbench +0.30%, MUI +0.08%, React +0.01%, the Markdown +0.21%), the micro benchmarks 0.23% more. Timed the same way, it shows no regression beyond the delta on the workbench (-0.31%), MUI (-0.09%), the store (-0.33%) and the Markdown (-0.30%), and cannot tell on React: the instructions it adds do not show in the time, and nothing is gained either.

Pinned off, because it saves a little where the training ran and costs where other data runs (`PGOPinned` in `internal/goasm/gen/pgo.go`, given to clang through `-mllvm` and to the post-link optimizer. llc gets only `-pgso=false`, from `PGOLLCFlags`):

- `-pgso`, the size optimization of code the training ran little or never. A bundle load ran 5.6% to 8.2% more instructions with it, and 0.8% to 0.9% fewer without it. Code that is cold in these pages is hot in other work.
- `-loop-vectorize-with-block-frequency` (v0.30.7, given to clang and opt as `=false`). With it the loop vectorizer takes the number of turns of a loop from the profile, and a loop the training ran a few turns can be left without its wide body, though other work runs it over long strings. On amd64 it also made LLVM keep a value in `rbx` in musl's `memmove`, which gave that leaf a frame and Go's prologue at every call (`NOSPLIT, $8-0`), and it grew the Go frame of `vsnprintf` by 144 bytes on amd64 and 176 on arm64. Pinned off, it changes 19 functions on amd64 and 78 on arm64: `memmove` is `NOSPLIT|NOFRAME, $0-0` again and `vsnprintf` has the frame of the build without a profile. Counted with callgrind on linux/amd64 against v0.30.6, a render runs 0.07% to 0.22% fewer instructions (store -0.07%, workbench -0.11%, MUI -0.10%, React -0.12%, the 400 KB Markdown -0.22%), and the 64 micro benchmarks 0.11% fewer (geometric mean, with `string_to_int` -2.65%, the five `float_to*` -0.9% to -1.5%, `weak_map_delete` +1.25%, which moves by about as much with any change of the code). The HTML is the same, and so are the frames of `JS_RegvmRun` and `JS_CallInternal` and the depths of every depth test, on both architectures. Timed on the AMD EPYC server (16 layouts, delta 1%, the mean of the layouts), the gain is under what a timing can see: no regression beyond the delta on the workbench (-0.56%), MUI (+0.13%) and the store (+0.35%), and the timing cannot tell on React (+0.12%) and the Markdown (-0.05%).

Looked for and not there, in the IR and in the code of LLVM 22: a cold attribute from the profile, splitting of hot and cold code, unswitching of anything but trivial branches, and profile-led decisions of GVN, LICM and JumpThreading.

The generator's own poll pass must not read the profile when it chooses where a loop's counter lives (a register or memory). A loop the training saw at about 3 turns made it choose memory, which costs 2 instructions a turn on a long array. So the pass takes every branch weight without the "expected" mark off while it chooses (`StaticWeights` in `pass/poll.cpp`). The weights of `likely()` and `unlikely()` carry that mark and stay, and so do those of the rare branches the pass adds itself (`pollUnlikely`), which it marks the same way: a profile weight with the same numbers is the same node to LLVM, so only the mark tells them apart. Run on the module that goes into the pass, once as it is and once with the profile's weights taken off, the pass makes the same choice in all of its 3,595 decisions on amd64 and 3,855 on arm64. `TestPassPollsProfile` holds it.

### Why it is safe

The profile weighs choices the compiler makes anyway. It adds no check and removes none, so a script gives the same answer, and a timeout or running out of memory still ends the call at the same places.

- The HTML of tifa's 24 pages (the 23 of the two apps and React's page) is the same as without the profile, rendered by tifa through the Go API, on darwin/arm64 and on amd64.
- Every depth test passes with the same depths, on both architectures, with the Go of `go.mod` (1.26.6) and with 1.27.2. `JS_RegvmRun` keeps its frame (its shadow frame is 256 bytes on amd64 and 176 on arm64, as before). Other functions move a little. Go frames: on amd64 192 get smaller and 37 larger, on arm64 74 smaller and 48 larger, `json_next_token` +128 bytes at most (with the vectorizer pinned, as above. Before that, `vsnprintf` grew by 144 and 176 bytes). Shadow frames: on amd64 4 grow (`unicode_sequence_prop` and `js_string_raw` +80, `js_front_drain` +32, `js_parse_statement_or_decl1` +16), on arm64 2 (`unicode_sequence_prop` +80, `js_front_drain` +32). None of them is on a path a recursion repeats: the shadow stack the deepest test of each kind uses moves by at most 32 bytes in all (amd64 `method` 7,344 to 7,376 bytes at 4,998 levels, the parser 16 bytes), or gets smaller (amd64 `to_string_nested` 160 KB less, `to_string` and `value_of` 6% less). On arm64 every test uses the same bytes as before.
- On arm64 one function, `JS_GetOwnPropertyNamesInternal`, was refused by the proof that drops LLVM's save of LR, because the profile put a reload of a vector pair at the end of a block. The proof now follows the address of such a reload down every path (`internal/goasm/gen/arm64_lr.go`) and still refuses any read of it, an indirect branch, a call that keeps it alive and a register-entry leaf that takes it as an argument. The code was correct. A change of block order from any source could have hit it.
- The "CG Profile" module flag that clang writes with a profile is dropped before and after the post-link optimizer (`StripCGProfile`), since LLVM 22 has no switch for it: the optimizer adds a second one and the verifier refuses, and llc would write directives the lowering does not take.

The gates the build passed: `make gen` gives the same bytes on macOS and on Linux for both architectures, the depth tests above, the HTML above, and a sample of 4,781 tests of test262 run through the Go API (`go run ./cmd/jsbench t262` in `compare/engines`), which pass and fail on exactly the same tests with and without the profile. test262 on the C tree (33 known errors) does not see the profile, which only the generated engine has.

### A stale profile

A function whose C changed since the training has no usable profile, and compiles as it would without one. Every other function keeps its counts. So an old profile loses part of the gain and nothing else. With the profile trained before two small commits it kept all of the gain, and clang dropped the profile of 1 function of 2,264 in `quickjs.c`.

`make gen` prints what it found, and never fails for it:

- `profile engine/pgo/profile.txt: N functions, M whose C changed since (no profile used: ...), K new (...)` lists the first 20 names of each.
- `warning: profile: JS_RegvmRun has no profile ...` when the register loop is stale. Without its profile 60% to 80% of the gain is left.
- `warning: profile: the C of N functions changed ...` when more than 30 changed.
- `profile: N likely() or unlikely() ...`: how many of the C's `likely` and `unlikely` the training ran the other way (below).

`make check-full` shows these lines for both architectures (`scripts/check-gen.sh`). Before a release, [releasing.md](../releasing.md#the-profile) says what to do with them.

### A second input: plain loops

Release: v0.30.7.

The pages leave many paths of the engine at a count of 0: typed arrays, DataView, regular expressions on UTF-16 text, BigInt, `Date.parse`, the slow paths of the operators and more. LLVM takes a count of 0 for a path that never runs: it moves spills into the path and its blocks out of the way. In the profile of v0.30.6 the cold count (the count of the 99.9999th percentile, below which LLVM calls a count cold) is 44, and `rv_ta_get`, which the register loop calls to read an element of a typed array, has a count of 0. `typed_array_read` ran 5.3% more instructions than without a profile.

So `engine/pgo/train.sh` now merges a second input with the pages: `engine/pgo/loops.js`, plain loops that run each such kind of path a few thousand times ([engine/pgo/README.md](../../../engine/pgo/README.md#the-loops)). It is merged as it is, about 3% of the counts. The loops run 1553 of the 1601 paths that the pages leave cold and the micro benchmarks of `compare/testdata` run at least once a turn (97.0%), and `train.sh` stops below 95%, or when a part of the loops no longer reaches the function it is there for.

Against the profile of v0.30.6 (callgrind, linux/amd64, the same C):

| Page | Instructions a render |
| --- | ---: |
| store's home | +0.083% |
| workbench | +0.105% |
| MUI's orders | -0.860% |
| React | -0.006% |
| the long Markdown page | -0.026% |

Counting the same build twice moves React's page by up to 0.15% and the others by less than 0.03%. Of the 64 micro benchmarks (instructions per turn), two run more than 0.5% more (`weak_map_set` +0.75%, `weak_map_delete` +0.69%) and 17 more than 0.5% fewer: `map_set_bigint` -7.0%, `typed_array_read` -4.3%, `regexp_utf16` -3.6%, `regexp_ascii` -3.2%, `typed_array_write` -2.4%, and the float and integer to string conversions about -1% to -2%. All 64 together run 0.57% fewer (geometric mean), and 3.48% fewer than without a profile, where the profile of v0.30.6 ran 2.92% fewer. `typed_array_read` runs 1301 instructions a turn, 1360 with the profile of v0.30.6 and 1291 without one.

Timed against v0.30.6 (the mean of 16 layouts), no kind of page is slower beyond 1% and the long Markdown page is 0.56% faster. MUI's 0.86% fewer instructions show as -0.20%, which the timing cannot tell from no change. `typed_array_read` and `typed_array_write` are 6.55% and 6.70% faster. `regexp_utf16` and `array_prop_create` are 3.65% and 3.15% slower, though they run fewer instructions.

Why loops and not the micro benchmarks themselves. Training on half of the 64 (split by name, the other half held out) did bring `typed_array_read` back, but it made two held-out neighbours slower: with the half at about 0.1% of the counts, `typed_array_write` ran 9.1% more instructions and `regexp_utf16` 5.0% more, and at about 10% every page ran 0.45% to 1.0% more. A benchmark trains the exact loop that measures it and leaves its neighbours as they were. The micro benchmarks stay what they are for: they measure what the profile does to paths it was not trained on, and `train.sh` runs them only to check the loops. The scripts that test every register instruction (`engine/test/regvm/corpus.sh`) were tried as the second input too. They read a typed array only 4,388 times, which stays below the cold count unless that input is about 10% of the profile, and at that share the pages ran 0.25% to 1.2% more.

Scaling a profile is not neutral. The profile of v0.30.6 multiplied by 100 and nothing else (`llvm-profdata merge -weighted-input=100`) changed 29 of the 32 assembly files of amd64, and MUI's page ran 0.32% more instructions. Neither input is scaled for that reason: the share of the loops is set by how much they run, not by a weight. The likely cause is how clang turns counts into branch weights, but it was not followed further.

`prop_create` is not a matter of training. The pages already call `add_property` about 3.2 million times and `JS_SetPropertyInternal` about 1.4 million times in the profile, far above the cold count, and none of the second inputs tried made `prop_create` faster: they left it within 0.1% or made it up to 1.9% slower. What costs it is where the profile puts the spills of `JS_SetPropertyInternal` for the path the pages run most. That is a separate look.

### Limits

Known and open in this release. Each is a follow-up.

- A path the training never ran can be slower, and its time can grow more than its instructions do. Timed on the AMD EPYC server (12 of the micro benchmarks of `compare/testdata`, the mean of 16 layouts), the profile makes six slower: `typed_array_read` 14.3% slower for 7.5% more instructions, `array_for` 4.5% for 2.1%, `prop_create` 4.0% for 3.8%, `int_arith` 3.9% for 2.9%, `array_read` 2.4% for 3.8% and `prop_read` 1.4% for 3.1%. It makes `array_prop_create` 6.1% faster (2.6% fewer instructions), and the timing cannot tell for the other five. These times are for one placement of the data: timed again with k small objects allocated before the loop (k = 0 to 15, 2026-11-14), `array_read` and `global_read` changed level with the placement alone, while the difference of v0.30.5 to v0.30.4 on `typed_array_read`, `array_for` and `prop_create` stayed at every k. So the counts below are not the whole cost. Of the 64 micro benchmarks (callgrind, instructions per turn of each benchmark's loop, linux/amd64), 18 run more than 0.5% more instructions than without the profile, 39 more than 0.5% fewer, and all 64 together 1.9% fewer (geometric mean). The ones that run more: `typed_array_read` +7.5%, `float_arith` +4.9%, `global_read` +4.4%, `array_read` +3.8%, `prop_create` +3.8%, `prop_read` +3.1%, `int_arith` +2.9%, `prop_write` +2.4%, `prop_update` +2.1%, `array_for` +2.1%, `empty_down_loop2` +2.0%, `empty_do_loop` +1.6%, `empty_down_loop` +1.0%, and five more between +0.5% and +1.0% (`string_build2c`, `string_build1x`, `string_build1`, `string_build_large1`, `global_func_call`). Those numbers are of the profile of v0.30.5. With the profile of v0.30.6 three of the 64 ran more than 0.5% more instructions than without a profile (`typed_array_read` +5.3%, `prop_create` +3.3%, `float_arith` +0.5%). With the [second input](#a-second-input-plain-loops) two are left: `prop_create` +3.2%, which training does not reach, and `typed_array_read` +0.8%. By time, `regexp_utf16` and `array_prop_create` are about 3% slower than with the profile of v0.30.6 for fewer instructions ([above](#a-second-input-plain-loops)). A path that neither the pages nor the loops run is still compiled as one that never runs, and the check of `train.sh` only covers the paths the micro benchmarks run.
- The inline replay misses a few calls. In a group of functions that call each other in a cycle, the profile changes the order in which the inliner meets the calls, so a call reaches the second pass in a context the notes of the first pass do not name, and it stays a call. On amd64 that is 22 calls in 8 functions: `rv_putel_slow` (10), `JS_ToBigInt64Free` (4), `free_gc_object` (3), and `rv_w_gen`, `js_set_fast_array_element_out_of_line`, `js_backtrace_build`, `JS_ToPrimitiveFree` and `JS_SetPropertyValue` (1 each). They call 8 small functions that the build without a profile inlines everywhere and that keep a body of their own here: `JS_FreeValue`, `JS_IsException`, `JS_StringToBigIntErr`, `JS_ToNumberFree`, `JS_VALUE_IS_NAN`, `__JS_NewShortBigInt`, `__js_rc` and `js_dbuf_init`. On arm64 it is 12 calls in 5 of those functions, to 5 of those small ones. They are slow or rare paths: a typed array write of a value that is not a number, a BigInt conversion, freeing the frame of an async function, a backtrace. Counted with callgrind on linux/amd64, the 8 small functions run 0 instructions in all 64 micro benchmarks and in the renders of the store, the workbench and MUI. Of the frames of the 8 functions only the Go frame of `free_gc_object` grows, by 32 bytes on both architectures (amd64 96 to 128, arm64 88 to 120), and its shadow frame stays 0. It is on a cycle of calls (`free_gc_object`, `__JS_FreeValueRT`, `free_gc_object`), but the C runs that cycle one level deep (`free_zero_refcount` in `engine/quickjs.c`: an object that dies while another is freed only joins a list), so no recursion repeats the 32 bytes and no depth floor moves. The other frames are the same or smaller. The replay has no cheap fix. The notes of the first pass name each call it inlined by the chain of call sites it was inlined through (`__js_rc` in `free_gc_object` is `JS_FreeValueRT` @ `async_func_free_frame` @ `__async_func_free` @ `free_gc_object`), and the cycle `JS_FreeValueRT`, `async_func_free_frame`, `JS_FreeValueRT` makes the second pass reach it through another chain. The notes of calls not inlined carry no chain. A fix would either add every chain the notes imply (C inlined into X, and X into F, gives C in F through both) or replay with AlwaysInline as the fallback and a list of the calls the first pass kept, read with their chains from its IR. Either is about a day of work with its tests, for 0 instructions on what was counted, so it is not done.
- A stale profile loses its gain in the functions it covers (above), and the gain fades as the C moves away from the training until it is trained again.
- The profile comes from pages of two apps, rendered with a fake `Intl` that formats numbers and dates by hand ([engine/pgo/README.md](../../../engine/pgo/README.md)). It has no count for the work `Intl` does, and no count from loading a bundle (those counts were dropped on purpose: a profile with them was dominated by the parser and was worse).
- Training twice gives files that differ in a few counts: two trainings on the Mac in 40 of 47,170 lines, a training on Linux and one on the Mac in 45. The engines they generate differ too (on amd64, 27 functions between the two Mac trainings and 126 between the Linux and the Mac training). So the training is not reproducible and the checked-in file is: `make gen` from it gives the same bytes on macOS and Linux.
- LLVM's profile format changes between versions, so a new LLVM means training again. The pin is 22.1.8.
- Anyone who reads the `.s` cannot see why a block moved. The profile and the program that trains it are in the repository for that reason.
- Saving optimization records (`-fsave-optimization-record`, `-opt-record-file`) changes the code of a profile build: it turns on hotness, which keeps block frequency information alive, and 2 functions came out different. The formal build never uses it. If you want the records to look at what the optimizer did, make them on a copy, and do not compare that output with the checked-in files.
- `-Wmisexpect` is on in the generator with a tolerance of 50 percent. It names each `likely()` or `unlikely()` that the training ran the other way more often than not, and writes them to `.build/w-<arch>/pgo/misexpect.txt` (about 50 entries, a few more or fewer from one training to the next). The IR is the same byte for byte with and without the flag. It tells you which hints of the C the pages do not support. It changes nothing.
- The gain is smaller on an Apple M4 (about 3%, read in the study) than on the AMD EPYC server (5.4% to 8.4%, timed for the release): the M4's instruction cache is 6 times larger, so the misses the profile removes cost less there.

### Tests and checks

- `engine/pgo/train.sh` trains the profile. It builds the host as the generator compiles the engine, so the hash of each function in the profile matches the one the generator sees. It stops when `engine/pgo/loops.js` runs less than 95% of the paths the pages leave cold and the micro benchmarks run, or when a part of the loops no longer reaches its function (`engine/pgo/cover.awk`).
- `TestInlineRemarks`, `TestStripCGProfile`, `TestProfileNames`, `TestUnprofiledFunctions`, `TestStaleWarnings` and `TestMisexpectLines` (`internal/goasm/gen/pgo_test.go`) check the notes of the first pass, the module flag, the names in the profile, which functions have none and the warnings.
- `TestPassPollsProfile` (`internal/goasm/gen/poll_test.go`, needs `TIFAJS_LLVM` and `TIFAJS_SHADOW_PASS`) checks that the poll pass chooses the place of a counter as it does without a profile, also when a weight of the profile has the numbers of the pass's own rare branches.
- `TestLowerARM64EngineLRTuplePaths` (`internal/goasm/gen`) checks the extended proof on arm64.
- `scripts/check-gen-test.sh` checks that `scripts/check-gen.sh` always shows the profile lines. It runs in the `scripts` step of `scripts/check.sh`.
- `go run ./cmd/notices` in `compare/engines` makes the notices of the packages in the bundles of the training (`TestGeneratePages`).

## A register-entry leaf with a frame has a Go declaration

Release: v0.30.5.

A register-entry leaf (`memcpy`, `memmove`, `memset`) takes its arguments in registers, and only the generated code calls it. Its TEXT says `NOFRAME` when it saves no register and calls nothing. When it does either, the TEXT keeps a frame (`NOSPLIT, $8-0`), and `go vet -asmdecl` asks a Go declaration of every TEXT that is not `NOFRAME`. The generator now writes one for such a leaf into `engine_decl_<arch>.go`: `func memmove()`, with no arguments and no result, as its TEXT has. The `.s` files stay the same.

It came up with the profile: on amd64 the profile makes `memmove` save RBX, and `go vet ./...` failed for linux/amd64 (`memmove: function memmove missing Go declaration`). A Mac runs `go vet` for arm64 only, so it did not see it. Since the loop vectorizer no longer reads the profile ([what the profile decides](#what-the-profile-decides), v0.30.7), `memmove` saves no register and is `NOFRAME` again, so the generator writes no declaration for it and no leaf has one. The rule stays, for the next leaf that saves a register or calls.

Why it is safe: the declaration only names the function, and the code and the frame are what they were. Go code must not call a leaf, so the generator stops when `internal/cengine/gocalls.txt` names one.

Tests: `TestLeafDecl` (`internal/goasm/gen`) checks that a leaf that saved registers and a leaf that calls get a declaration that `go vet -asmdecl` takes, and that a leaf with neither says `NOFRAME` and gets none. `TestEngineAsmDecl` runs `go vet -asmdecl` on the engine for the six targets.

## A jump table entry may reach the end of the code

Release: v0.31.0.

When LLVM proves that a value of a `switch` cannot occur, the entry of the jump table for that value goes to a block that holds no instruction. llc may put that block last in the function, and then the entry reaches the end of the function's code, where no instruction is. With the profile of v0.30.7 one such table came to be in the engine, in `put_lvalue` on amd64. The post-link check wants every entry to reach an instruction it has matched, so it refused that table (`entry 2 reaches ..., no instruction of the body`), and the check with every Go release failed for v0.30.7 and after.

The check now takes such an entry on two conditions: in the binary it reaches the end of the generated code that llc's code explains, the same place, and what is there is the jump to the Go epilogue or the epilogue itself, where a return of the function goes. A function that never returns has its data there, and its entry is refused. An entry that reaches the end in llc's object and any other place in the binary is refused, and so is a branch that reaches the end. The generated code does not change.

Why it is safe: the entry is for a value LLVM proved the `switch` never gets, so it is never taken, in the native build or in the generated one. The check still holds the entry to the place llc gave it.

Tests: `hole` in `internal/goasm/gen/testdata/check/mini.c` is a `switch` with such an entry. `TestPostLinkCheck` checks that llc's entry reaches the end of the code, that the check passes on it, and that an entry moved to another block is refused.

## A phi gets one address of a local per incoming block

Release: v0.31.1.

The shadow-stack pass (`internal/goasm/gen/pass/shadow.cpp`) moves every local onto the shadow stack and writes the address of the local, argument 0 plus its offset, where each use needs it. For a phi, that is at the end of the block the value comes from. It made one address for each use, and a phi that names one block several times (three cases of one `switch` that go to the same block) holds one use per entry, so it got several different addresses for that block. LLVM's verifier refuses such a phi (`PHI node has multiple entries for the same basic block with different incoming values!`), and the generation stopped: no wrong code was written. The optimizer makes this shape from a small array of two locals indexed by a counter (`next[n++] = t`) in code with a `switch`. The engine had none, so C that made one could not be generated.

The pass now makes one address per phi and incoming block, and every entry of that phi for that block takes it. The generated engine does not change: `make check-gen` gives the files checked in, for amd64 and arm64.

Why it is safe: in valid IR every entry of a phi for one block holds the same value, so when one of them is a local, all of them are that local, and they all get its address. Entries for other blocks, which may hold other locals, get addresses of their own, made in their own block. The address is made before the terminator of the incoming block, so it is there on every edge that leaves it, whatever the terminator is (a branch, a `switch`, an `indirectbr`, an `invoke`). A phi that names each block once gets the same address in the same place as before.

Tests: `TestPhiNamesBlockTwice` (`internal/goasm/gen`) runs the pass on `internal/goasm/gen/testdata/phi/switch.ll`, two phis that each name the block of a `switch` three times, each with another local from that block and from a second block. It checks that opt passes the verifier, and that each phi has one address per block, made in that block, at the offset of the right local.
