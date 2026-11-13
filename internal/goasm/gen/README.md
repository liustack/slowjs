# Engine generator

English · [简体中文](README.zh-CN.md)

The generator turns the C engine in `engine/` into the Go assembly in
`internal/cengine`. It handles the calling convention, the stack rules and
the shadow stack on amd64 and arm64.

It is a maintainer tool: it shells out to LLVM 22.1.8 (clang, opt, llc) and
runs its own checks and lowering in Go (`debug/elf`,
`golang.org/x/arch/x86/x86asm`). Applications never run it.

## Pipeline

1. clang -O2 to IR with a kept frame pointer, no red zone, signed wrap and no
   tail calls. On amd64 the stack is declared 8-byte aligned, as Go's is, and
   the target feature slow-two-mem-ops keeps llc from folding the load of a
   call target into the call, so every indirect call is through a register. On
   arm64 the target is aarch64-linux-gnu with x18 and x26 to x28 fixed, x29
   reserved and LR kept for return addresses. The portable flags (freestanding,
   no system headers, wrapping signed overflow, unsigned char and the others)
   are not written here: init runs `engine/test/flags.sh`, which the scripts of
   the C tree read too, and appends what it gives (`TestFlagsComeFromFlagsSh`
   keeps the Go lists and `cycles.sh` from repeating them). On amd64 `quickjs.c`,
   the one file that holds JSValues, also gets `-fno-slp-vectorize` and the
   target feature `slow-unaligned-mem-16` (`EngineValueFlags`), so that LLVM
   copies a JSValue (16 bytes) as two 8-byte moves and not one SSE move: the
   interpreter writes the two halves one at a time, and Zen 3 cannot forward a
   16-byte load from them (see [c-engine.md](../../../docs/changes/c-engine.md#how-a-jsvalue-is-copied)).
   The small libc is left out, since its `memcpy` moves 16 bytes at a time on
   purpose. CheckIR rejects variadics, byval, sret and long double.
2. The shadow-stack pass (`pass/shadow.cpp`) moves every address-taken local
   and dynamic alloca onto the VM's shadow stack: argument 0 of every engine
   function is the first free shadow byte, each function hands its callees
   the byte above everything it has live, and `__shadow_remaining()` gives C
   the space left for its RangeError checks. No alloca may remain. A function
   that needs more integer argument registers than the target allows becomes
   `f(sp, blk)`: every call site stores the arguments in a block on its
   shadow frame. With `-shadow-cc=preserve_none` every unified entry, host
   callback and call to them uses preserve_none. Register-entry leaves stay C
   and get a reserved frame pointer: no frame, and llc never allocates RBP,
   which Go owns. Every unified function and every call to one takes and
   returns an i1, i8 or i16 as an i32: the caller extends it (sign or zero,
   as the C says, zero for a bool), the callee truncates it. preserve_none
   puts only 32 and 64-bit integers in its own registers (amd64 R12 on, arm64
   X20 on) and leaves narrow ones to the C rules (EDI or W0 on), and Go
   writes whole words. The signatures the pass reports then have no narrow
   integer, and the generator refuses one (and a float, which Go would move
   as a whole float64). With `-shadow-icall-sigs` every indirect call gets a
   `"kcfi"` operand bundle whose type id encodes its signature (`icall.go`).
   Without the `"kcfi"` module flag llc emits no check code for it, but it
   keeps the id on the machine call. `-shadow-probes` writes a module with
   one probe per unified definition, of its type, convention and attributes,
   that reads every argument and returns a loaded result. No trap instruction
   is left: every `llvm.trap` (the `__builtin_trap` of `abort`, and the pass's
   own check on a dynamic alloca that would cross the end of the shadow stack)
   becomes a call to `host_trap(sp)`, a callback of the host that panics,
   since SIGILL and SIGTRAP end a Go process whatever it does. LLVM 22.1.8
   writes no trap for an `unreachable` on the two linux triples, so
   `llvm.trap` is the only source. `debugtrap` and `ubsantrap` stop the pass,
   and so does a trap in a leaf, which has no shadow pointer to pass.
3. llc -O2 with shrink-wrapping off writes assembly and an object, every
   function in `.text` (`-profile-guided-section-prefix=false`: otherwise
   a cold function such as `abort` goes in `.text.unlikely.`). The whole
   engine loads `pass/icall.cpp` and `pass/facts.cpp` (built into the same
   shared object as the pass) with `-load`: after the machine passes this run
   has (the outliner and function splitting are off, and the insertion point
   has to be checked again if that changes) icall.cpp names a symbol at every
   indirect call, `goasm.icall.<function>.<n>.<id>`, a label right before
   the call in the text and a local symbol at its address in the object. An
   indirect call without an id stops llc. facts.cpp (`-goasm-facts`) writes
   down the registers llc assigned: each function's argument registers, the
   ones its code reads at the entry, and for each call and return the
   argument, result and live registers (`facts.go`). llc runs over the probe
   module too, for the registers of every argument and result. CheckObject
   rejects writable or thread-local data and any external symbol outside the
   engine and the host callbacks.
4. LowerUnified / LowerRegister (amd64) and LowerARM64 rewrite each function
   as text: strip the prologue and epilogues, load the Go stack arguments into
   the native argument registers after Go's prologue, marshal each call inline
   (keep the registers the mode asks for, move the arguments to the outgoing
   area, call, read the result, restore), where a call to a unified function
   the driver lowers takes the result from the registers the callee stored
   it from and reads nothing back from the Go stack (`LowerEnv.RegResult`),
   move LLVM's locals onto the Go frame, keep constant pools inside the
   function, and turn every return into a jump to one Go RET. A function
   starts in the lowered object where its body will sit in the binary
   (`FuncPlace`: on 16 bytes, or at its `-align64` place in a line of 64),
   so that the `.p2align` of each pool and table holds where the code runs.
   Any shape it does not know fails: unknown call forms, calls through memory,
   tail jumps,
   SP writes, escaping stack addresses, aligned or folded packed stack
   operands (the Go stack is only 8-byte aligned), read-only symbols other
   than constant pools, forbidden instructions. A leaf that uses RBP fails
   too.
5. clang -c (llvm-mc on arm64) re-assembles the lowered text so every branch
   offset is fixed.
6. Emit writes Go assembly: one TEXT per function, the body as bytes, every
   call a Go-visible CALL or BL, and every load of an argument and store of
   the result as a move by name through FP (`a0+0(FP)`, `ret+24(FP)`, the
   names of `GoSlots` and of the Go declarations). Go promises to find an
   argument that way, and not at an offset from SP, which depends on how the
   assembler lays out the frame. On amd64 the lowered text still has the move
   at the offset Go uses now, so that clang lays out the branches around it,
   and the Go assembler writes the same bytes (`TestFPMovesAssembleAsLowered`).
   Any other instruction that reaches above the frame from SP stops the
   build, and the data after a function's code follows a `// data` line.
   The size bound of a `.s` file of the whole engine counts each function as
   the text it takes with its instructions written as bytes, so that a move
   by name moves no function to another file, and so to another place in
   the order the linker lays the functions out in.
7. VerifyLinked checks the linked binary: body bytes unchanged, a stack-check
   prologue on every entry off the NOSPLIT list and on `host_poll`, none on
   the entries on it, on windows the room check for a fault on every unified
   entry, the function table points only at unified entries, no hidden calls
   or tail jumps.

## Call modes

`-mode` picks how a unified call keeps the C callee-saved registers, which Go
code and morestack clobber:

- `body` (default): the call site saves the ones the calling function's body
  uses (amd64 RBX, R12 to R15, arm64 R19 to R25 and F8 to F15).
- `all`: the call site saves all of them (for comparison).
- `stub` (arm64): the call goes through a stub that saves all 15 (for
  comparison).
- `pn`: the code is compiled with preserve_none, so LLVM keeps across a call
  only what is live, in its own frame, and no call site saves anything.
  Arguments then use amd64 R12 to R15, RDI, RSI, RDX, RCX, R8, R9 and arm64
  X20 to X25. A seventh arm64 argument would land in X26, which the scan
  rejects, so the pass moves larger argument lists into a block.

`pn` is the mode of the engine: it is as fast as `body` or faster, and the call
site saves nothing, which keeps the lowering simplest.

`GEN_NO_SAVES=1` with `body` and `GEN_NO_PACK=1` with `pn` are negative builds
that must fail the tests and the register scan.

## Running

The test package is a small C module of samples that exercises the generator on its own. `proto.sh` (from the liustack/quickjs root, linux/amd64) builds the pass, generates
it as `internal/proto` (not checked in), builds its tests and runs the post-link verify. `ARCH=arm64`
generates the arm64 package and cross-builds its tests: copy the package to
an arm64 machine to run them. `MODE` is the call mode. Set `CALL_OVERHEAD=1`
to run the call-overhead timing. Its
tests are `testdata/*.txt`, kept as text so this package does not compile
them. `go test` here runs the rule tests. With `QUICKJS_LLVM` (an LLVM bin
dir) and `QUICKJS_SHADOW_PASS` (the built pass) set, it also runs every sample
in `testdata/reject` through the real pipeline, and with `QUICKJS_LLD`
(ld.lld) as well, `TestNarrowArgs` runs `testdata/narrow` natively and
generated, and `TestSampleRuns` does the same for each sample of
`testdata/run`: each defines `long run(long *r, int one)`, is run natively
(also under UBSan) and from Go in body, pn and pn poison builds, and must print
the same lines, and the post-link check must pass on it. `internal/enginecmp` measures
the current engine under the same GC stop setup as those tests.

## The whole engine

`cmd/engine` runs the same pipeline over all of `engine/` and `engine/libc`
as one module and writes a Go package (`-out`): the functions in .s files
bounded in size, the read-only data as `engine_rodata`, and the Go
declarations. The host provides the callbacks of `engine/host.h` and `goasmMissing`, which each stub calls with its index
in `missingNames`. `-allow` names more external symbols the run accepts. It names none by default. A rejection stops the run and nothing is written (a rejection by the
pass is one `LLVM ERROR:` line and exit status 1, not an abort with opt's
stack), and the default has no stubs. `-keep-going` is for surveys: it lists every function the
C must change, by class, and stubs it instead of stopping. `-release` is the
formal build: it fails on any stub, needs `-out`, and refuses every switch
made for surveys and tests (`-keep-going`, `-loose-varargs`, `-poison`,
`GEN_KEEP_NARROW`, `GEN_EDIT_LOWERED`, a changed
`-non-format-variadic`, a `-postlink-opt` other than `auto`, changed
`-postlink-opt-args`) before it does any work. `-mode` is `body` or `pn`.
`-varargs-block` passes variable arguments in a shadow-stack block instead of
rejecting them. `-shadow-size-log2` is the size of the shadow-stack region
the engine is made for (20 by default, 25 for the host's whole 32 MiB, and
from 16 to 40: the driver and the pass refuse any other before they use it):
`engine_decl_<arch>.go` records it as `shadowSizeLog2`, and the host refuses a
smaller stack.

The files are compiled one at a time (step 1 above, with the format checks on the
IR as written) and linked with llvm-link. On amd64 and arm64 the optimizer then runs over
the linked module (`-postlink-opt`, default `auto`: `PostLinkOptFor`) before the
shadow-stack pass, with `PostLinkOptArgs`: `-O2`, the inline threshold of 400
that `engine/test/flags.sh` gives clang, and the SLP vectorizer off. clang had
optimized each file alone, so the calls from one file to another (the small
libc, the allocator and the heap into the interpreter and the collector) were
never inlined. The second run does it. It saves 1.5% to 1.7% of the
instructions of a page in native C, 0.6% to 1.2% in the generated engine of
amd64 and 0.6% to 1.0% in that of arm64, and makes the generated assembly 13%
larger (the code 19% on amd64). A threshold of 1000
for inlining makes the interpreter bigger and its frame 32 bytes taller: not
used.

The flags clang took from the command line travel in the IR as attributes of the
functions (`+slow-unaligned-mem-16` of `quickjs.c`), and stay with their
functions. `-fno-slp-vectorize` is the exception, since it is an option of the
pipeline and not an attribute: a second -O2 would join the two 8-byte moves of a
JSValue into one of 16 bytes again (52 lines of vector code in the IR of
`JS_CallInternal`). The SLP vectorizer is turned off with a threshold no tree
reaches (`TestPostLinkOptKeepsValueFlags`).

The optimizer changes names, and each place of the generator that goes by a name
reads the module as it stands after it:

- A static function inlined into all its callers is gone (the pass reports,
  and the Go declarations and `engine_keep` list, what is in the module). An
  external function that is inlined stays defined and declared, since the Go
  side may call it.
- It clones a function for the constant arguments of a call
  (`mark_children.specialized.2`). The clone is a unified function like any
  other, with the Go name `mark_children_specialized_2` (`GoNames`), and its
  code goes with the file of the function it was made from (`cloneGroups`,
  `cloneOf`, which also take the numbers llvm-link adds). The register-entry
  leaves (`-leaves`, `-shadow-leaf`) are the functions of those names and
  nothing else: a clone of `memcpy` would be a unified function.
- `-align64` (`lre_exec_backtrack`), `DispatchCopies`, the
  host callbacks, `host_poll` of the post-link check and `js_host_layout` name
  external functions and data, which the optimizer keeps under their names.

`-postlink-opt=on` or `off` forces the run and `-postlink-opt-args` changes its
options, for tests and surveys (`-release` refuses both). `TestPostLinkOpt` (amd64 and arm64, with the
post-link check over a module cross-built for each) and the `postlink.c` sample of
`TestSampleRuns` (which runs the code, on linux/amd64 only) hold all of this.

`-arch` is `amd64` (the default) or `arm64`, and every file the driver
writes ends in the architecture (`engine_arm64.s`, `engine_decl_arm64.go`,
`engine_keep_arm64.go`), so one folder holds both and Go picks one. A build
removes only the files of its own architecture. `-build-tag T` puts the tag T in
front of the architecture in the `//go:build` line of every file (`T && amd64` for the `.s`
files, `T` for `engine_decl_<arch>.go`). `scripts/gen-engine.sh` passes no tag:
the checked-in files carry the architecture alone (`amd64`, `arm64`). The arm64 engine runs on
darwin/arm64 and linux/arm64. arm64 takes `-mode pn` only:
preserve_none passes integers in X20 to X25 there, and its next registers,
X26 to X28, are Go's. The other modes are only in the test package (`proto.sh`). arm64 has no
`-poison` or `-loose-varargs` build and refuses both. It lowers each function
in `LowerARM64EngineFunc` (`arm64_engine.go`) and holds every entry, call and
return to the registers llc's facts and probes name, as the amd64 post-link
check does: a narrow integer in another register, a result read from
another, or a call llc's facts do not have stops the build. llc writes arm64
in the tiny code model (`-code-model=tiny`), so each reference to data is one
instruction that names the whole address, and the lowering rewrites each on
its own to the same address: an `adr` of `engine_rodata` or of a function
becomes Go's `MOVD $sym(SB)`, an `adr` of the function's own constant pool or
jump table stays PC-relative, and a load of a label reads the data in the
function's body (the plan copies an object there when a function loads it
that way, as amd64 does for a load through the PC). The page forms of the
small code model, `adrp` and `:lo12:`, stop the build: their second
instruction is right only while the register still holds the page of the
first, which no single instruction shows. llc writes its object for the checks
with a section per function, since an `adr` from one function to another may
be further than the 1 MiB it reaches. It refuses `svc`, system
registers, X18, X26 to X29 as values, LR outside a save, tail jumps, a frame
that grows after the prologue, a stack address that escapes and fused
multiply-adds (`fmadd`, `fmla` and the rest), with a test for each
(`TestLowerARM64EngineRejects`, `TestLowerARM64EngineFacts`,
`TestLowerARM64EngineTiny`).

llc picks the form of a branch or a jump table from the sizes of the code it
writes, and the lowering then adds code inside the function (the calls, the
polls, the frame), so what fit in llc's text can be too far in the lowered one.
Two flags of llc leave the lowering that room. `-aarch64-tbz-offset-bits=13`
gives `tbz` and `tbnz` half their reach of 32 KiB, so a farther one becomes a
`tbz` over a `b`. `-aarch64-enable-compress-jump-tables=false` gives every jump
table 4 bytes an entry, the distance from the `adr` of its dispatch, where llc
would write 1 or 2 bytes, steps of 4 bytes from the lowest target, when they
fit its own code: a switch of `js_start_tag` once came to 272 steps after the
lowering, past the 255 a byte holds. The dispatch is as long either way (`adr`,
a load, an `add`, `br`), and the tables take about 10 KB more. The lowering does
not rest on the flag: it sizes its code (4 bytes an instruction, the
alignments from where the function is placed) and refuses a function with a jump
table entry whose distance does not fit its width, with the table, the entry
and the steps it came to (`a64CheckJumpTables` in `arm64_jumptable.go`,
`TestA64CheckJumpTables`, `TestLowerARM64EngineJumpTable`).


A function whose arguments need more integer registers than a unified entry
has (10 on amd64, 6 on arm64) takes the shadow pointer and the address of a
block of them instead (`f(sp, blk)`). Its generated entry is named
`<name>_block`, and the driver writes a Go function by its C name and
signature into `engine_decl_<arch>.go` (`PackedGo` in `packed.go`): it stores
each argument in its 8-byte slot (16 for a JSValue) of a block at the shadow
pointer, as the pass lays the block out, and calls the entry with the shadow
pointer above the block. So a host calls the API by the C signature on every
architecture, while which functions are packed differs (20 on amd64, 205 on
arm64).

The same C gives the same package, byte for byte, whatever the number of
compile jobs (`-j`) and the order the compiles finish in, and no file holds the
path of the work folder (`TestOutputReproducible`).

The engine polls (`pass/poll.cpp`, see
[c-engine.md](../../../docs/changes/c-engine.md#polls-and-interrupts)):
every cycle of the code counts down a counter at `-poll-offset` bytes from
the base of the shadow-stack region (`gen.PollCounterOffset`, 384, which
`engine_decl_<arch>.go` records as `pollCounterOffset`), and calls `host_poll`
when it runs out. `-poll=false` builds an engine without polls, to measure
what they cost, and `-release` refuses it. The interpreter counts its
bytecode itself: the C marks where (`poll_count` in `engine/cutils.h`, at
the entry of `JS_RegvmRun` and where `pc` does not go forward), the pass
puts a count at each mark, and the edges into its dispatch get none. So the
dispatch stays short, and llc copies it into the end of each handler after
register allocation, which it does for a computed goto of up to 10
instructions. Without the copies the interpreter is much slower.
The driver counts the indirect jumps of each function with a computed goto
table in llc's text and stops below `gen.DispatchCopies` (42 for
`JS_RegvmRun`, which has 59 on amd64). The pass checks its own output: with the
blocks that count or poll taken out, the back edges of the loops it proves
short, and the edges into the dispatch of the interpreter, no function has a
cycle left. The interpreter must count at its entry, and each of its marks
must come right after a count of its weight. Each count takes at least 1,
and polls whenever it runs out: its branch tests that it is 0 or less and
calls `host_poll` on the side taken. Each count takes from
the count as it stands, each store to the counter stores the count as it
stands, and each call that may poll and each return find the count in
memory: code that keeps it in a register stores it before such a call and
loads it after, and stores it where it is left. The hidden option
`-shadow-poll-fault` breaks one of these on purpose, so that a test can see
the check stop the pass.

Functions without a stack check: every unified entry starts with Go's
stack check unless its TEXT says NOSPLIT, and the generator chooses no
function to go without one (`MarkNoSplit`, `nosplit.go`). The NOSPLIT list
holds only the functions that cannot have one: the register-entry leaves,
and the leaves the Go assembler marks NOSPLIT by itself (no call and a frame
under 128 bytes with the saved base pointer, counted by `AMD64Stack` and
`ARM64Stack`, which differ in how a frame is rounded). An automatic leaf
calls nothing, and a register-entry leaf calls only other register-entry
leaves (the engine's three call nothing): the driver fails on any other
call, on a cycle of them, and on a chain of them that takes 128 bytes or
more below its entry. A fault in a leaf becomes a call of
`runtime.sigpanic` pushed onto the
goroutine's stack, and sigpanic's stack check may call morestack: that
needs 16 bytes on amd64 and 24 on arm64, and the stack check of the leaf's
caller left at least 800 bytes, less the leaf's frame. Every recursion goes
through a stack check, which is where the stack grows and where the runtime
can stop the goroutine, and every loop still calls `host_poll`, whose own
stack check is the safe point. The linker checks every chain of NOSPLIT
calls again on each build, the ABI wrapper of a Go func value included. The
driver prints the size of the list, writes each function on it with its
kind and frame to `nosplit.txt` in the work folder, and
`engine_decl_<arch>.go` names them in `noSplitList`, for the tests. The
generator writes NOSPLIT in the TEXT of each function on the list, and the
post-link check holds each entry's stack check to it: a Go assembler that
marks another leaf NOSPLIT by itself, or stops marking one, fails the check.

Room for a fault on windows: windows writes the state of the CPU at a fault
below the stack pointer, on the goroutine's stack, and Go keeps 4 KiB below
each stack for it (golang/go#81238), less than a CPU with AMX needs. So in
the assembly of windows only (`#ifdef GOOS_windows`), every unified function
checks after Go's prologue that its stack pointer less `RoomBelow` (12 KiB,
`emit.go`) is above the guard in g, and calls `goasm_room` when it is not: a
function that does nothing, with a frame of `RoomFrame` bytes, whose own
stack check makes the runtime grow the stack. A function on the NOSPLIT list
loads g first, and one without a frame says NOFRAME, so that the call does
not change its frame. The branch over the call lands on a one-byte no-op,
not on the `PCALIGN` of the body: Go before 1.26.11 and 1.27.4 loops forever
on a forward branch to a `PCALIGN` (golang/go#81792). A register-entry leaf
has no check: its caller's check leaves it the room. The callbacks of the host
make the room again before they return into the engine
(`internal/cengine/host.go`). On linux and darwin the code is what it was.
The post-link check of a windows binary holds every unified function to the
check, its call to `goasm_room` and its branch to right after that call,
and `goasm_room` to a stack check of at least `RoomBelow` - 8 bytes below
its own stack pointer, so that its caller has `RoomBelow`. `TestPostLinkFormats` changes each of these in a
windows binary and the check must refuse it. See docs/changes/c-engine.md
(When the engine fails) for the numbers.

Where a body starts: after Go's prologue each function has a `PCALIGN $16`,
so the body starts on 16 bytes and moves within its lines of 64 bytes when
code before it grows. `-align64 name:offset,...` puts the body of each named
function `offset` bytes past a multiple of 64 instead (a `PCALIGN $64`, which
also puts the function on 64 bytes, then `offset` bytes of 8-byte no-ops,
which run once per call). The default of the amd64 engine is
`lre_exec_backtrack:16`: the matcher at the fastest place measured
(see [performance.md](../../../docs/performance.md#code-positions)). The
interpreter, the register loop `JS_RegvmRun`, has no fixed place: its place
is not measured yet. arm64 and a `-src` build have none unless the flag is given. The post-link check holds each body
to its place and the padding before it to no-ops.

Two negative builds test the tests. `-poison` writes garbage into every
register at each entry before the arguments are loaded, after each unified
call before the result is read and at each return: the engine's tests must
still pass, so no code reads a register the lowering did not set. A poison
build reads every result back from the Go stack, since its returns write
over the result registers.
`GEN_KEEP_NARROW=1` leaves narrow integers as the generator once took them:
the post-link check must reject the result (`TestNarrowArgs`). Only the value 1
turns it on, and any other value but an empty one is an error. The package of
such a build is marked: `engine_negative_<arch>.go` does not compile without
`-tags goasmnegative`, and `engine_decl_<arch>.go` says so. A later build into the
same folder removes the file.

```sh
go build -o /tmp/engine-gen ./internal/goasm/gen/cmd/engine
L=/usr/lib/llvm-22/bin P=internal/goasm/gen/pass
$L/clang++ -shared -fPIC -O1 $($L/llvm-config --cxxflags) $P/shadow.cpp $P/poll.cpp $P/icall.cpp $P/facts.cpp \
    -o internal/goasm/gen/internal/_shadow.so
/tmp/engine-gen -engine engine -pass internal/goasm/gen/internal/_shadow.so \
    -keep-going -report /tmp/rej.json -out /tmp/cengine/cengine
```

Before the pass, the C is also compiled without optimization and every call
that reaches `engine/libc`'s formatter (`snprintf`, `vsnprintf` and any
variadic function but `code_match` and `unicode_prop_ops`) is held to the
formats `stdio.c` takes (`formats.go`, `CheckFormats`). A format must be a
string constant `FormatError` accepts, or a parameter that a wrapper forwards
(a variadic function's last fixed parameter, or `JS_ThrowError2`). Anything
else stops the build, so a `%f` cannot reach `abort()` at run time. The accept
table mirrors `stdio.c` and `testdata/formats/cases.txt` lists the formats both
must agree on. `engine/test/libc-test.c` feeds that list to `vsnprintf`, so that `stdio.c` and
the table cannot drift apart.

The post-link check is the gate for the output. Build a binary of the
package with `-tags goasmkeep` (`engine_keep_<arch>.go` then keeps every function)
and pass it as `-verify-bin` (several, separated by commas, are checked one
after another), with the package's import path as `-verify-pkg`. The check reads llc's own object file and the binary, decodes
both with x/arch, and matches every generated function instruction by
instruction. It refuses code that would not run as decoded: a prefix the
processor rejects or ignores, an unknown or cut-short instruction, a no-op
other than the assemblers' padding. The only differences it allows are the
rewrites the lowering wrote down for that function, and it works out what
each must be from llc's object: the prologue and epilogue it replaced (llc's
pushes and reserved locals), where each frame offset moves and that the
access stays inside the Go frame, each call's saves, argument moves, target
and result, and where each data reference points. The registers in those
moves are llc's, not the generator's model of the convention: each entry
loads every argument word into the register llc's probe reads it from, and
llc's code may read no other register at the entry before writing it but
the ones its dropped prologue pushes. Each call stores every argument word
from the register llc passes it in (for a call of a unified definition,
the one the definition reads it from) and loads each result word into the
register llc takes it from, and llc's code may read no register after the
call before writing it but those results and the saved registers. A call
whose record says it takes the result from registers (`RegResult`) has no
result loads: its target must be a lowered unified function, not a stub, a
leaf or Go code, and the registers llc takes the result from must be the
ones the callee's probe returns it in. Each return stores the registers llc
returns and jumps to Go's epilogue, which touches only BP and SP, so those
registers still hold the result after the RET. A leaf keeps C's registers, so of
its facts only its calls' targets and the number of its calls and returns
are checked. llc's own code must keep to what the build rules out: a call
through memory fails, and so does a frame access wider than 8 bytes that is
not an unaligned move. Each function's record
must agree with the pass's signature for it, its list of leaves and its
calling convention. Each indirect call must carry the symbol LLVM put at it
in llc's object, of that function, and the signature it names must be the
one the lowering marshalled; a symbol anywhere else fails. It compares engine_rodata and the data each function
carries byte by byte, pointers by what they point to. It also holds
each pool and table to its alignment at the address where it sits in the
binary. `-verify-all` also
fails when a function was not in the binary. `GEN_EDIT_LOWERED` edits the
lowered text before it is assembled, for the check's own tests
(`TestPostLinkCheck`, from C through the driver, and `TestCheckCases`, from
hand-written llc code).

What the check takes on trust, and where it stops:

- The pass's signatures, for the number of argument and result words only:
  the one it reports for each function, and the type id it gives each
  indirect call from the call's own IR type. That LLVM keeps the id on the
  right call is LLVM's (instruction selection copies it to the machine call
  and tail merging keeps calls of different ids apart), and the check reads
  it from llc's object, not from the text the lowering read.
- llc's facts: that a probe gets the registers of the function it stands
  for (llc assigns them from the type, convention and attributes, which the
  probe copies), and that liveness as LLVM's LivePhysRegs computes it after
  the last machine pass is what the code reads. The facts of a call are
  matched to the calls of llc's object in order, and each names its target
  or type id, which the check compares.
- The width of a narrow integer: the registers do not say which bits a
  callee relies on. The pass's widening makes every one a whole i32, and
  `TestNarrowArgs` runs it with garbage above each narrow value.
- A Go frame of 4096 bytes or more has a stack check the check does not
  know, so such a function fails it. For a smaller frame the check holds the
  compare to the frame: on amd64 no LEA up to 128 bytes (the frame with the
  8 bytes of BP) and `LEAQ -(frame-128)(SP), R12` past it, on arm64 the words
  of the compare. `host_poll` has its stack check read on both architectures.
  A dynamic alloca aligned beyond 16 is refused by the shadow-stack pass, as
  a fixed one is.
- The loader. The check reads the binary in the format of its system: ELF
  for linux, a static executable linked at a fixed address, which nothing
  writes into after the kernel maps it (an ELF PIE, `-buildmode=pie`, and
  a file with a dynamic table, a program interpreter or a section of
  relocations, as cgo makes, are refused), Mach-O for darwin and PE for
  windows, which Go links as a PIE by
  default. It reads every byte through what the loader maps from the file
  (the PT_LOAD program headers of ELF, the segments of Mach-O, the headers
  and sections of PE, each up to what it takes from the file), and refuses
  mappings that overlap in a page, a section that is not where its mapping
  takes it from, and a PE section at an offset of the file that is no
  multiple of 0x200, which the loader rounds down. Mach-O and PE give no symbol sizes, so each symbol runs to the
  next one, and what lies between a function and the next is held to
  padding. In a PIE each pointer of engine_rodata must hold its target at the
  image base of the file and have the fixup that tells the loader to move it:
  a rebase of the dyld info (the Go linker), an entry of the chained fixups
  (ld64, which links a darwin program with cgo) or a DIR64 base relocation
  (PE). Any other place the loader writes inside code or data the check
  read fails it: another fixup, a slot of the import address table of a PE
  or a slot its load config names. So do two fixups of one place, and a
  table the check does not read (lazy or weak binds, the pointer formats of
  arm64e, other PE relocation types, the TLS, delay import, bound import and
  CLR directories of a PE). A PE directory is read when it has an address,
  whatever its size, as the loader does, and one with a size and no address
  is refused. Every fixup must lie, its whole width, in the bytes the file
  holds of a section the linker puts pointers in (`__DATA` and
  `__DATA_CONST` of Mach-O, `.data` and `.rdata` of PE), and no two may
  overlap: so none is in the headers, the
  load commands, `__LINKEDIT`, the import and relocation tables or code.
  What the check reads of a PE to find the fixups is recorded read by read,
  and the load config, which lies in `.rdata`, as a whole: a fixup there is
  refused too, as the loader applies it before it reads them. A binary cut short or with
  bent tables is refused with an error (`TestOpenBinBent`). That the loader applies
  each fixup as its format says is trusted. On amd64 Go's prologue loads g
  in the form of the system (from FS on linux, from GS on darwin, through
  `runtime.tls_g` on windows), and each form is taken only for its own
  system. `TestPostLinkFormats` checks a binary of each of the five
  targets and refuses a changed byte, a wrong pointer, a dropped fixup and
  a second fixup of one pointer in each Mach-O and PE one, a rebase of the
  load commands, of `__LINKEDIT`, of `__DATA,__bss` and of code in each
  Mach-O one, a relocation table, a TLSDESC or COPY relocation at the end of
  `.data` and a dynamic table the section table does not show in each ELF
  one, and in the PE one a relocation that ends a byte past what `.data`
  holds in the file (one that ends at its last byte is taken), an import
  table or a slot of the load config in engine_rodata (also with the size
  of its directory at 0), a directory with a size and no address, a
  relocation of an import descriptor, a lookup table, the load config (also
  of its Size field with both sizes at 0), the headers, the base relocation
  table, code or the inside of an import slot, a TLS directory and a delay
  import directory. It also refuses, where they apply, a segment or section
  cut short in the file before engine_rodata, a section read from a copy
  while the loader maps changed bytes, a second PT_LOAD over the code and a
  PE section at an offset the loader rounds down.

On arm64 the check (`CheckEngineARM64`, `arm64_verify.go`) is narrower. It
holds each generated function in the linked binary to the object llvm-mc
made of the lowered text: Go's prologue (a stack check exactly when the
function is off the NOSPLIT list, whose load of the guard, compare and branch to
morestack it reads word for word as Go writes them for the frame the TEXT
asks for, SP against the guard up to 128 bytes and SP less (frame - 128) in x17
up to 4096, with a frame past 4096 refused, and the frame itself), the
loads of the arguments, the no-ops before the body, the body word for word
with each `BL` and `ADRP`/`ADD` at the target the object names, the stores
of the result, Go's epilogue, one RET and the call of morestack. Each call
in the body is the next one of the record, and it loads its result back
from the Go stack unless the record says it takes it from x0, x1 or d0, which
only a call to a lowered unified function may: the check holds each of those
to storing its result from them and then running only Go's epilogue (x29,
x30 and SP). A direct call must agree with the pass on whether its
target is a leaf and, when it is not, on its signature, so no record can
skip a load by calling a unified function a leaf or one that returns
nothing. It compares
`engine_rodata` byte for byte, pointers by what they point to. It
does not read llc's own object: that the lowering kept the meaning of llc's
code rests on the lowering's rules and llc's register facts, which the
lowering checks as it goes. `TestCheckARM64Function` changes each part of a
function and requires the check to refuse it.
