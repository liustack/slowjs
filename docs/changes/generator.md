---
summary: 'Changes to how the engine is generated from the C: the profile of tifa pages that make gen compiles with (what it is, how it was found, what it may decide and what is pinned off, why it is safe, what a stale one costs, its limits and checks)'
read_when:
  - Asking why make gen compiles twice, or what engine/pgo/profile.txt is for
  - make gen prints a line that starts with "warning: profile:" or "profile:", or a generated function changed after a change of the C that did not touch it
  - Changing the flags the generator gives clang, opt or llc, or the inlining of the generated engine
  - Asking what the profile gains, what it costs, and which scripts it can make slower
  - Training the profile again, or taking a commit of QuickJS that changes the register loop
---

# The generator

English · [简体中文](generator.zh-CN.md)

How `make gen` turns the C of `engine/` into the Go assembly in `internal/cengine`. The rest of the generator is in [its README](../../internal/goasm/gen/README.md). Here are the changes that move the speed of the generated code.

## A profile of tifa's pages

Release: Unreleased.

`make gen` compiles the engine with a profile: counts of how often each branch ran while the engine rendered real pages. LLVM uses the counts to put the likely blocks next to each other, to place spills where the code runs least, and to lay out `switch` cases. The profile is `engine/pgo/profile.txt`, trained on the pages in `engine/pgo/pages` (see [engine/pgo/README.md](../../engine/pgo/README.md)). It makes renders 3% to 9% faster than the same C generated without it ([performance.md](../performance.md#a-profile-of-tifas-pages)).

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

The study and its numbers are in [performance.md](../performance.md#a-profile-of-tifas-pages).

### What the profile decides

Allowed:

- llc's block weights and layout, and the order of the cases of a `switch`. This is most of the gain.
- Loop peeling by the number of turns the training saw, for a loop with an estimated count of 7 or less: those first turns are copied out in front of the loop. The build without a profile peels 1 or 2 turns, and only where the code calls for it. Its cost depends on the data: a loop the training saw at 3 turns that runs a thousand pays for the copies at each entry, and a peeled loop can lose the unrolling it had. A cap of 2 turns (`-unroll-peel-max-count=2`) is a follow-up, not measured yet.
- SimplifyCFG keeps a branch the training saw go one way 99% of the time as a branch, where it would make a select, and does not merge its conditions. About 108 functions differ. Its cost depends on the data too: on data that goes both ways the branch is mispredicted where the select had no branch. There is no clean switch to pin it (the same threshold decides the branches of `likely()`), so it is allowed.
- Three more decisions follow the profile in this release and are follow-ups to pin once measured: the flat-loop threshold of the unroller (a loop the training ran under 5 turns is not unrolled), vectorizing by block frequency (the vectorizer reads the number of turns from the profile), and LoopSink (work moves out of the block before a loop into the blocks of the loop the training ran little).

Pinned off, because it saves a little where the training ran and costs where other data runs (`PGOPinned` in `internal/goasm/gen/pgo.go`, given to clang, opt and llc):

- `-pgso`, the size optimization of code the training ran little or never. A bundle load ran 5.6% to 8.2% more instructions with it, and 0.8% to 0.9% fewer without it. Code that is cold in these pages is hot in other work.

Looked for and not there, in the IR and in the code of LLVM 22: a cold attribute from the profile, splitting of hot and cold code, unswitching of anything but trivial branches, and profile-led decisions of GVN, LICM and JumpThreading.

The generator's own poll pass must not read the profile when it chooses where a loop's counter lives (a register or memory). A loop the training saw at about 3 turns made it choose memory, which costs 2 instructions a turn on a long array. So the pass takes every branch weight without the "expected" mark off while it chooses (`StaticWeights` in `pass/poll.cpp`). The weights of `likely()` and `unlikely()` carry that mark and stay, and so do those of the rare branches the pass adds itself (`pollUnlikely`), which it marks the same way: a profile weight with the same numbers is the same node to LLVM, so only the mark tells them apart. Run on the module that goes into the pass, once as it is and once with the profile's weights taken off, the pass makes the same choice in all of its 3,595 decisions on amd64 and 3,855 on arm64. `TestPassPollsProfile` holds it.

### Why it is safe

The profile weighs choices the compiler makes anyway. It adds no check and removes none, so a script gives the same answer, and a timeout or running out of memory still ends the call at the same places.

- The HTML of tifa's 24 pages (the 23 of the two apps and React's page) is the same as without the profile, rendered by tifa through the Go API, on darwin/arm64 and on amd64.
- Every depth test passes with the same depths, on both architectures, with the Go of `go.mod` (1.26.6) and with 1.27.2. `JS_RegvmRun` keeps its frame (its shadow frame is 256 bytes on amd64 and 176 on arm64, as before). Other functions move a little. Go frames: on amd64 192 get smaller and 40 larger, on arm64 83 smaller and 49 larger, `vsnprintf` +144 and +176 bytes at most, then `json_next_token` +128. Shadow frames: on amd64 4 grow (`unicode_sequence_prop` and `js_string_raw` +80, `js_front_drain` +32, `js_parse_statement_or_decl1` +16), on arm64 2 (`unicode_sequence_prop` +80, `js_front_drain` +32). None of them is on a path a recursion repeats: the shadow stack the deepest test of each kind uses moves by at most 32 bytes in all (amd64 `method` 7,344 to 7,376 bytes at 4,998 levels, the parser 16 bytes), or gets smaller (amd64 `to_string_nested` 160 KB less, `to_string` and `value_of` 6% less). On arm64 every test uses the same bytes as before.
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

### Limits

Known and open in this release. Each is a follow-up.

- A path the training never ran can be slower. Of the 64 micro benchmarks of `compare/testdata` (callgrind, instructions per turn of each benchmark's loop, linux/amd64, counts and not times), 18 run more than 0.5% more instructions than without the profile, 39 more than 0.5% fewer, and all 64 together 1.9% fewer (geometric mean). The ones that run more: `typed_array_read` +7.5%, `float_arith` +4.9%, `global_read` +4.4%, `array_read` +3.8%, `prop_create` +3.8%, `prop_read` +3.1%, `int_arith` +2.9%, `prop_write` +2.4%, `prop_update` +2.1%, `array_for` +2.1%, `empty_down_loop2` +2.0%, `empty_do_loop` +1.6%, `empty_down_loop` +1.0%, and five more between +0.5% and +1.0% (`string_build2c`, `string_build1x`, `string_build1`, `string_build_large1`, `global_func_call`). The pages train none of these loops: a profile that adds them is a follow-up, with the benchmarks split in two halves so that the half it does not train still tells what an untrained path costs.
- The inline replay misses a few calls. In a group of functions that call each other in a cycle, the profile changes the order in which the inliner meets the calls, so a call reaches the second pass in a context the notes of the first pass do not name, and it stays a call. On amd64 that is 22 calls in 8 functions: `rv_putel_slow` (10), `JS_ToBigInt64Free` (4), `free_gc_object` (3), and `rv_w_gen`, `js_set_fast_array_element_out_of_line`, `js_backtrace_build`, `JS_ToPrimitiveFree` and `JS_SetPropertyValue` (1 each). They call 8 small functions that the build without a profile inlines everywhere and that keep a body of their own here: `JS_FreeValue`, `JS_IsException`, `JS_StringToBigIntErr`, `JS_ToNumberFree`, `JS_VALUE_IS_NAN`, `__JS_NewShortBigInt`, `__js_rc` and `js_dbuf_init`. On arm64 it is 12 calls in 5 of those functions, to 5 of those small ones. They are slow or rare paths: a typed array write of a value that is not a number, a BigInt conversion, freeing the frame of an async function, a backtrace. Of the frames of the 8 functions only the Go frame of `free_gc_object` grows, by 32 bytes on both architectures (amd64 96 to 128, arm64 88 to 120), and its shadow frame stays 0. It is on a cycle of calls (`free_gc_object`, `__JS_FreeValueRT`, `free_gc_object`), but the C runs that cycle one level deep (`free_zero_refcount` in `engine/quickjs.c`: an object that dies while another is freed only joins a list), so no recursion repeats the 32 bytes and no depth floor moves. The other frames are the same or smaller. Inlining those calls in the post-link optimizer is a follow-up.
- A stale profile loses its gain in the functions it covers (above), and the gain fades as the C moves away from the training until it is trained again.
- The profile comes from pages of two apps, rendered with a fake `Intl` that formats numbers and dates by hand ([engine/pgo/README.md](../../engine/pgo/README.md)). It has no count for the work `Intl` does, and no count from loading a bundle (those counts were dropped on purpose: a profile with them was dominated by the parser and was worse).
- Training twice gives files that differ in a few counts: two trainings on the Mac in 40 of 47,170 lines, a training on Linux and one on the Mac in 45. The engines they generate differ too (27 functions on amd64 between the two Mac trainings). So the training is not reproducible and the checked-in file is: `make gen` from it gives the same bytes on macOS and Linux.
- LLVM's profile format changes between versions, so a new LLVM means training again. The pin is 22.1.8.
- Anyone who reads the `.s` cannot see why a block moved. The profile and the program that trains it are in the repository for that reason.
- Saving optimization records (`-fsave-optimization-record`, `-opt-record-file`) changes the code of a profile build: it turns on hotness, which keeps block frequency information alive, and 2 functions came out different. The formal build never uses it. If you want the records to look at what the optimizer did, make them on a copy, and do not compare that output with the checked-in files.
- `-Wmisexpect` is on in the generator with a tolerance of 50 percent. It names each `likely()` or `unlikely()` that the training ran the other way more often than not, and writes them to `.build/w-<arch>/pgo/misexpect.txt` (about 50 entries, a few more or fewer from one training to the next). The IR is the same byte for byte with and without the flag. It tells you which hints of the C the pages do not support. It changes nothing.
- The gain is smaller on an Apple M4 (about 3%) than on the AMD EPYC server (6% to 9%): the M4's instruction cache is 6 times larger, so the misses the profile removes cost less there.

### Tests and checks

- `engine/pgo/train.sh` trains the profile. It builds the host as the generator compiles the engine, so the hash of each function in the profile matches the one the generator sees.
- `TestInlineRemarks`, `TestStripCGProfile`, `TestProfileNames`, `TestUnprofiledFunctions`, `TestStaleWarnings` and `TestMisexpectLines` (`internal/goasm/gen/pgo_test.go`) check the notes of the first pass, the module flag, the names in the profile, which functions have none and the warnings.
- `TestPassPollsProfile` (`internal/goasm/gen/poll_test.go`, needs `QUICKJS_LLVM` and `QUICKJS_SHADOW_PASS`) checks that the poll pass chooses the place of a counter as it does without a profile, also when a weight of the profile has the numbers of the pass's own rare branches.
- `TestLowerARM64EngineLRTuplePaths` (`internal/goasm/gen`) checks the extended proof on arm64.
- `scripts/check-gen-test.sh` checks that `scripts/check-gen.sh` always shows the profile lines. It runs in the `scripts` step of `scripts/check.sh`.
- `go run ./cmd/notices` in `compare/engines` makes the notices of the packages in the bundles of the training (`TestGeneratePages`).
