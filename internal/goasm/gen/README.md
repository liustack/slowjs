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
   reserved and LR kept for return addresses. CheckIR rejects variadics,
   byval, sret and long double.
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
   that reads every argument and returns a loaded result.
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
7. VerifyLinked checks the linked binary: body bytes unchanged, stack-check
   prologue on split entries and on `host_poll`, the function table points
   only at unified entries, no hidden calls or tail jumps.

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
`GEN_KEEP_NARROW`, `GEN_EDIT_LOWERED`, `-arch arm64`, a changed
`-non-format-variadic`) before it does any work. `-mode` is `body` or `pn`.
`-varargs-block` passes variable arguments in a shadow-stack block instead of
rejecting them. `-shadow-size-log2` is the size of the shadow-stack region
the engine is made for (20 by default, 25 for the host's whole 32 MiB):
`engine_decl.go` records it as `shadowSizeLog2`, and the host refuses a
smaller stack. `-arch arm64` stops after llc and counts what the arm64
lowering would have to handle.

Two negative builds test the tests. `-poison` writes garbage into every
register at each entry before the arguments are loaded, after each unified
call before the result is read and at each return: the engine's tests must
still pass, so no code reads a register the lowering did not set.
`GEN_KEEP_NARROW=1` leaves narrow integers as the generator once took them:
the post-link check must reject the result (`TestNarrowArgs`). Only the value 1
turns it on, and any other value but an empty one is an error. The package of
such a build is marked: `engine_negative.go` does not compile without
`-tags goasmnegative`, and `engine_decl.go` says so. A later build into the
same folder removes the file.

```sh
go build -o /tmp/engine-gen ./internal/goasm/gen/cmd/engine
L=/usr/lib/llvm-22/bin P=internal/goasm/gen/pass
$L/clang++ -shared -fPIC -O1 $($L/llvm-config --cxxflags) $P/shadow.cpp $P/icall.cpp $P/facts.cpp \
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
must agree on. Keeping `stdio.c` and the table in step needs a libc test in
`engine/test` that feeds that list to `vsnprintf`, which is the C's to add.

The post-link check is the gate for the output. Build a binary of the
package with `-tags goasmkeep` (`engine_keep.go` then keeps every function)
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
- Only a non-PIE linux/amd64 binary.
