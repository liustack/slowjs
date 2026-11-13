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

The notes of the first pass are made again from the current C every time, so they cannot go stale. The profile is one text file for amd64 and arm64: the hash of a function depends on the C only. `make gen` turns it into the indexed form in `.build` with `llvm-profdata` of LLVM 22.1.8. The output is a function of `engine/`, the profile file and the flags, so `make check-gen` keeps working. `make gen` takes about PGO2-FILL: time of make gen per architecture, with the Mac and linux numbers, where it took about 2 minutes before.

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
- Loop peeling by the number of turns the training saw, only for a loop with an estimated count under 7. This is open: PGO2-FILL: whether a cap of 2 turns is in the release, and what it did to the micro benchmarks.
- SimplifyCFG keeps a branch the training saw go one way 99% of the time as a branch, where it would make a select, and does not merge its conditions. About 108 functions differ. There is no clean switch to pin it, so it is allowed.

Pinned off, because each saves a little where the training ran and costs where other data runs (`PGOPinned` in `internal/goasm/gen/pgo.go`):

- `-pgso`, the size optimization of code the training ran little or never. A bundle load ran 5.6% to 8.2% more instructions with it, and 0.8% to 0.9% fewer without it. Code that is cold in these pages is hot in other work.
- The flat-loop threshold of the unroller, so a loop the training ran under 5 turns is still unrolled as before.
- Vectorizing by block frequency, so the vectorizer does not read the number of turns from the profile.
- LoopSink, which moves work out of the block before a loop into the blocks of the loop the training ran little.

Looked for and not there, in the IR and in the code of LLVM 22: a cold attribute from the profile, splitting of hot and cold code, unswitching of anything but trivial branches, and profile-led decisions of GVN, LICM and JumpThreading.

The generator's own poll pass must not read the profile when it chooses where a loop's counter lives (a register or memory). A loop the training saw at about 3 turns made it choose memory, which costs 2 instructions a turn on a long array. PGO2-FILL: say whether the fix is in this release. If it is: the pass takes the profile's branch weights off while it chooses (`StaticWeights` in `pass/poll.cpp`), and `TestPassPollsProfile` holds that. If it is not: a loop the training saw at few turns keeps its counter in memory, a known loss on long arrays, and a follow-up.

### Why it is safe

The profile weighs choices the compiler makes anyway. It adds no check and removes none, so a script gives the same answer, and a timeout or running out of memory still ends the call at the same places.

- The HTML of every page of the training set is the same as without the profile (every page the study rendered, on both architectures).
- Every depth test passes with the same depths. `JS_RegvmRun` keeps its frame (its shadow frame is 256 bytes on amd64 and 176 on arm64, as before). Other functions move a little: on amd64 254 get a smaller Go frame and 34 a larger one (`json_next_token` +112 bytes at most), and 4 or 5 shadow frames grow (`unicode_sequence_prop` +80). A deep recursion through those has a little less room.
- On arm64 one function, `JS_GetOwnPropertyNamesInternal`, was refused by the proof that drops LLVM's save of LR, because the profile put a reload of a vector pair at the end of a block. The proof now follows the address of such a reload down every path (`internal/goasm/gen/arm64_lr.go`) and still refuses any read of it, an indirect branch, a call that keeps it alive and a register-entry leaf that takes it as an argument. The code was correct. A change of block order from any source could have hit it.
- The "CG Profile" module flag that clang writes with a profile is dropped before and after the post-link optimizer (`StripCGProfile`), since LLVM 22 has no switch for it: the optimizer adds a second one and the verifier refuses, and llc would write directives the lowering does not take.

PGO2-FILL: the gates of the final build: test262 (known errors 33), `make check-full` on amd64 and arm64, the lowest Go version, the depth floors.

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

- A path the training never ran can be slower. Four micro benchmarks run more instructions than without the profile (instruction counts, not times): `array_slice` +12.5%, `typed_array_read` +5.7%, `prop_create` +3.9% and `array_prop_create` +1.9%. PGO2-FILL: final numbers of these four and of any other benchmark that runs more.
- The inline replay misses about 25 call sites in 7 functions of recursive call groups: the profile duplicated those calls and the notes of the first pass do not name the copies. Those copies stay calls. PGO2-FILL: say whether they are fixed or listed, and the list.
- A stale profile loses its gain in the functions it covers (above), and the gain fades as the C moves away from the training until it is trained again.
- The profile comes from pages of two apps, rendered with a fake `Intl` that formats numbers and dates by hand ([engine/pgo/README.md](../../engine/pgo/README.md)). It has no count for the work `Intl` does, and no count from loading a bundle (those counts were dropped on purpose: a profile with them was dominated by the parser and was worse).
- Training twice gives files that differ in about 43 of 47,129 lines, so the training is not byte-reproducible. The checked-in file is what is reproducible.
- LLVM's profile format changes between versions, so a new LLVM means training again. The pin is 22.1.8.
- Anyone who reads the `.s` cannot see why a block moved. The profile and the program that trains it are in the repository for that reason.
- Saving optimization records (`-fsave-optimization-record`, `-opt-record-file`) changes the code of a profile build: it turns on hotness, which keeps block frequency information alive, and 2 functions came out different. The formal build never uses it. If you want the records to look at what the optimizer did, make them on a copy, and do not compare that output with the checked-in files.
- `-Wmisexpect` is on in the generator with a tolerance of 50 percent. It names each `likely()` or `unlikely()` that the training ran the other way more often than not, and writes them to `.build/w-<arch>/pgo/misexpect.txt` (48 entries now). The IR is the same byte for byte with and without the flag. It tells you which hints of the C the pages do not support. It changes nothing.
- The gain is smaller on an Apple M4 (about 3%) than on the AMD EPYC server (6% to 9%): the M4's instruction cache is 6 times larger, so the misses the profile removes cost less there.

### Tests and checks

- `engine/pgo/train.sh` trains the profile. It builds the host as the generator compiles the engine, so the hash of each function in the profile matches the one the generator sees.
- `TestInlineRemarks`, `TestStripCGProfile`, `TestProfileNames`, `TestUnprofiledFunctions`, `TestStaleWarnings` and `TestMisexpectLines` (`internal/goasm/gen/pgo_test.go`) check the notes of the first pass, the module flag, the names in the profile, which functions have none and the warnings.
- `TestPassPollsProfile` (`internal/goasm/gen/poll_test.go`, needs `QUICKJS_LLVM` and `QUICKJS_SHADOW_PASS`) checks that the poll pass chooses the place of a counter as it does without a profile. PGO2-FILL: keep or drop this line with the poll fix.
- `TestLowerARM64EngineLRTuplePaths` (`internal/goasm/gen`) checks the extended proof on arm64.
- `scripts/check-gen-test.sh` checks that `scripts/check-gen.sh` always shows the profile lines. It runs in the `scripts` step of `scripts/check.sh`.
- `go run ./cmd/notices` in `compare/engines` makes the notices of the packages in the bundles of the training (`TestGeneratePages`).
