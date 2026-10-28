# C-engine generator (phase-0 prototype)

Prototype of the new generator for the C-source engine. It is separate from
the committed Python generator in `internal/goasm`, which keeps producing the
old interpreter. This one proves the calling convention, the stack rules and
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
   16-byte load from them (see [c-engine.md](../../../docs/fork-changes/c-engine.md#how-a-jsvalue-is-copied)).
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
   area, call, read the result, restore),
   move LLVM's locals onto the Go frame, keep constant pools inside the
   function, and turn every return into a jump to one Go RET. Any shape it
   does not know fails: unknown call forms, calls through memory, tail jumps,
   SP writes, escaping stack addresses, aligned or folded packed stack
   operands (the Go stack is only 8-byte aligned), read-only symbols other
   than constant pools, forbidden instructions. A leaf that uses RBP fails
   too.
5. clang -c (llvm-mc on arm64) re-assembles the lowered text so every branch
   offset is fixed.
6. Emit writes Go assembly: one TEXT per function, the body as bytes, every
   call a Go-visible CALL or BL.
7. VerifyLinked checks the linked binary: body bytes unchanged, a stack-check
   prologue on every entry off the NOSPLIT list and on `host_poll`, none on
   the entries on it, the function table points only at unified entries, no
   hidden calls or tail jumps.

## Call modes

`-mode` picks how a unified call keeps the C callee-saved registers, which Go
code and morestack clobber:

- `body` (default): the call site saves the ones the calling function's body
  uses (amd64 RBX, R12 to R15, arm64 R19 to R25 and F8 to F15).
- `all`: the call site saves all of them (for comparison).
- `stub` (arm64): the call goes through a stub that saves all 15 (the first
  arm64 backend, for comparison).
- `pn`: the code is compiled with preserve_none, so LLVM keeps across a call
  only what is live, in its own frame, and no call site saves anything.
  Arguments then use amd64 R12 to R15, RDI, RSI, RDX, RCX, R8, R9 and arm64
  X20 to X25. A seventh arm64 argument would land in X26, which the scan
  rejects, so the pass moves larger argument lists into a block.

`GEN_NO_SAVES=1` with `body` and `GEN_NO_PACK=1` with `pn` are negative builds
that must fail the tests and the register scan.

## Running

`proto.sh` (from the slowjs root, linux/amd64) builds the pass, generates
`internal/proto`, builds its tests and runs the post-link verify. `ARCH=arm64`
generates the arm64 package and cross-builds its tests: copy the package to
an arm64 machine to run them. `MODE` is the call mode. Set `CALL_OVERHEAD=1`
to run the call-overhead timing. The proto
tests are `testdata/*.txt`, kept as text so this package does not compile
them. `go test` here runs the rule tests. With `SLOWJS_LLVM` (an LLVM bin
dir) and `SLOWJS_SHADOW_PASS` (the built pass) set, it also runs every sample
in `testdata/reject` through the real pipeline, and with `SLOWJS_LLD`
(ld.lld) as well, `TestNarrowArgs` runs `testdata/narrow` natively and
generated, and `TestSampleRuns` does the same for each sample of
`testdata/run`: each defines `long run(long *r, int one)`, is run natively
(also under UBSan) and from Go in body, pn and pn poison builds, and must print
the same lines, and the post-link check must pass on it. `internal/enginecmp` measures
the current engine under the same GC stop setup as the proto tests.

## The whole engine

`cmd/engine` runs the same pipeline over all of `engine/` and `engine/libc`
as one module and writes a Go package (`-out`): the functions in .s files
bounded in size, the read-only data as `engine_rodata`, and the Go
declarations. The host provides `host_now`, `malloc`, `realloc`, `free`,
`malloc_usable_size` and `goasmMissing`, which each stub calls with its index
in `missingNames`. A rejection stops the run and nothing is written (a rejection by the
pass is one `LLVM ERROR:` line and exit status 1, not an abort with opt's
stack), and the default has no stubs. `-keep-going` is for surveys: it lists every function the
C must change, by class, and stubs it instead of stopping. `-release` is the
formal build: it fails on any stub, needs `-out`, and refuses every switch
made for surveys and tests (`-keep-going`, `-loose-varargs`, `-poison`,
`GEN_KEEP_NARROW`, `GEN_EDIT_LOWERED`, `GEN_FORCE_NOSPLIT`, a changed
`-non-format-variadic`) before it does any work. `-mode` is `body` or `pn`.
`-varargs-block` passes variable arguments in a shadow-stack block instead of
rejecting them. `-shadow-size-log2` is the size of the shadow-stack region
the engine is made for (20 by default, 25 for the host's whole 32 MiB, and
from 16 to 40: the driver and the pass refuse any other before they use it):
`engine_decl_<arch>.go` records it as `shadowSizeLog2`, and the host refuses a
smaller stack.

`-arch` is `amd64` (the default) or `arm64`, and every file the driver
writes ends in the architecture (`engine_arm64.s`, `engine_decl_arm64.go`,
`engine_keep_arm64.go`), so one folder holds both and Go picks one. A build
removes only the files of its own architecture. `-build-tag T` puts the tag T in
front of the architecture in the `//go:build` line of every file (`slowjs_cengine && amd64` for the `.s`
files, `slowjs_cengine` for `engine_decl_<arch>.go`), so that a build without
the tag sees none of them. `scripts/gen-engine.sh` always passes
`-build-tag slowjs_cengine`. The arm64 engine runs on
linux/arm64 and darwin/arm64 alike. arm64 takes `-mode pn` only:
preserve_none passes integers in X20 to X25 there, and its next registers,
X26 to X28, are Go's. The other modes stay with the prototype (`proto.sh`).
Measured on the prototype on an Apple M-series core, a call through a stub
or with all callee-saved registers saved took about 3.9 ns, `body` about 1 ns
and `pn` about 1.1 ns, and `pn` has no saves at any call site. arm64 has no
`-poison` or `-loose-varargs` build and refuses both. It lowers each function
in `LowerARM64EngineFunc` (`arm64_engine.go`) and holds every entry, call and
return to the registers llc's facts and probes name, as the amd64 post-link
check does: a narrow integer in another register, a result read from
another, or a call llc's facts do not have stops the build. llc reaches its
data through `adrp` and `:lo12:`: the lowering writes the whole address in
its Go symbol (`engine_rodata`, or a function), takes a
`:lo12:` use only in the block of its `adrp`, and reads a function's own
constant pool and jump tables PC-relative. It refuses `svc`, system
registers, X18, X26 to X29 as values, LR outside a save, tail jumps, a frame
that grows after the prologue, a stack address that escapes and fused
multiply-adds (`fmadd`, `fmla` and the rest), with a test for each
(`TestLowerARM64EngineRejects`, `TestLowerARM64EngineFacts`).

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
[c-engine.md](../../../docs/fork-changes/c-engine.md#polls-and-interrupts)):
every cycle of the code counts down a counter at `-poll-offset` bytes from
the base of the shadow-stack region (`gen.PollCounterOffset`, 384, which
`engine_decl_<arch>.go` records as `pollCounterOffset`), and calls `host_poll`
when it runs out. `-poll=false` builds an engine without polls, to measure
what they cost, and `-release` refuses it. The interpreter counts its
bytecode itself: the C marks where (`poll_count` in `engine/cutils.h`, at
the entry of `JS_CallInternal` and where `pc` does not go forward), the pass
puts a count at each mark, and the edges into its dispatch get none. So the
dispatch stays short, and llc copies it into the end of each handler after
register allocation, which it does for a computed goto of up to 10
instructions. Without the copies the interpreter is much slower.
The driver counts the indirect jumps of each function with a computed goto
table in llc's text and stops below `gen.DispatchCopies` (175 for
`JS_CallInternal`, which has 204). The pass checks its own output: with the
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
stack check unless its TEXT says NOSPLIT, and the driver chooses which ones
do (`ChooseNoSplit`, `nosplit.go`). The list holds the leaves the Go
assembler marks NOSPLIT by itself (no call and a frame under 128 bytes), the
register-entry leaves, and the functions it chooses on top of those: a
function whose address is not taken, that makes no call through a pointer,
that is on no cycle of direct calls and whose chain of NOSPLIT calls fits the
room the linker gives one (792 bytes on amd64 and on arm64, counted
by `AMD64Stack` and `ARM64Stack`, which differ in the frame each call
adds). Every recursion then goes
through a stack check, which is where the stack grows and where the runtime
can stop the goroutine, and every loop still calls `host_poll`, whose own
stack check is the safe point. The room is counted as the linker counts it,
with 64 bytes more below the deepest frame of each chain (`FaultRoom`): a
fault there becomes a call of `runtime.sigpanic` pushed onto the goroutine's
stack, and sigpanic's stack check may call morestack, which needs 16 bytes
above the bottom of the stack and ends the process with "split stack
overflow" when they are not there. A function also leaves room above it for
the ABI wrapper of a Go func value of it (its arguments and 16 bytes). The
linker checks the chains again on every build and fails it when one does
not fit. The driver prints the size of the list, writes each function on it
with its frame and height to `nosplit.txt` in the work folder, and
`engine_decl_<arch>.go` names them in `noSplitList`, for the tests. The post-link
check holds each entry's stack check to the list.

Where a body starts: after Go's prologue each function has a `PCALIGN $16`,
so the body starts on 16 bytes and moves within its lines of 64 bytes when
code before it grows. `-align64 name:offset,...` puts the body of each named
function `offset` bytes past a multiple of 64 instead (a `PCALIGN $64`, which
also puts the function on 64 bytes, then `offset` bytes of 8-byte no-ops,
which run once per call). The default of the amd64 engine is
`JS_CallInternal:40,lre_exec_backtrack:16`, the fastest places measured (see
[performance.md](../../../docs/performance.md#code-positions)). arm64 and a
`-src` build have none unless the flag is given. The post-link check holds each body
to its place and the padding before it to no-ops.

Three negative builds test the tests. `-poison` writes garbage into every
register at each entry before the arguments are loaded, after each unified
call before the result is read and at each return: the engine's tests must
still pass, so no code reads a register the lowering did not set.
`GEN_KEEP_NARROW=1` leaves narrow integers as the generator once took them:
the post-link check must reject the result (`TestNarrowArgs`). Only the value 1
turns it on, and any other value but an empty one is an error. The package of
such a build is marked: `engine_negative_<arch>.go` does not compile without
`-tags goasmnegative`, and `engine_decl_<arch>.go` says so. A later build into the
same folder removes the file. `GEN_FORCE_NOSPLIT=f,g` puts those functions
on the NOSPLIT list whatever the rules say, and marks the package the same
way: the linker must refuse a recursion or a chain too deep for the room
(`TestSampleRuns/nosplit_fault`).

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
and pass it as `-verify-bin`, with the package's import path as
`-verify-pkg`. The check reads llc's own object file and the binary, decodes
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
call before writing it but those results and the saved registers. Each
return stores the registers llc returns. A leaf keeps C's registers, so of
its facts only its calls' targets and the number of its calls and returns
are checked. llc's own code must keep to what the build rules out: a call
through memory fails, and so does a frame access wider than 8 bytes that is
not an unaligned move. Each function's record
must agree with the pass's signature for it, its list of leaves and its
calling convention. Each indirect call must carry the symbol LLVM put at it
in llc's object, of that function, and the signature it names must be the
one the lowering marshalled; a symbol anywhere else fails. It compares engine_rodata and the data each function
carries byte by byte, pointers by what they point to. `-verify-all` also
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
  know, so such a function fails it.
- Only a non-PIE linux binary: linux/amd64, or linux/arm64 for the arm64
  engine. darwin/arm64 links the same code, so the linux/arm64 check covers
  it.

On arm64 the check (`CheckEngineARM64`, `arm64_verify.go`) is narrower. It
holds each generated function in the linked binary to the object llvm-mc
made of the lowered text: Go's prologue (a stack check exactly when the
function is off the NOSPLIT list, whose load of the guard and branch to
morestack it reads but not the compare between them, and the frame the TEXT
asks for, below 4096 bytes or a multiple of 4096), the
loads of the arguments, the no-ops before the body, the body word for word
with each `BL` and `ADRP`/`ADD` at the target the object names, the stores
of the result, Go's epilogue, one RET and the call of morestack. It
compares `engine_rodata` byte for byte, pointers by what they point to. It
does not read llc's own object: that the lowering kept the meaning of llc's
code rests on the lowering's rules and llc's register facts, which the
lowering checks as it goes. `TestCheckARM64Function` changes each part of a
function and requires the check to refuse it.
