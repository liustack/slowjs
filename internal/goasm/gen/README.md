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
   tail calls. On amd64 the stack is declared 8-byte aligned, as Go's is. On
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
   callback and call to them uses preserve_none. Register-entry leaves stay C.
3. llc -O2 with shrink-wrapping off writes assembly and an object. CheckObject
   rejects writable or thread-local data and any external symbol outside the
   engine and the host callbacks.
4. LowerUnified / LowerRegister (amd64) and LowerARM64 rewrite each function
   as text: strip the prologue and epilogues, load the Go stack arguments into
   the native argument registers after Go's prologue, marshal each call inline
   (keep the registers the mode asks for, move the arguments to the outgoing
   area, call, read the result, restore),
   move LLVM's locals onto the Go frame, turn aligned stack moves into
   unaligned ones (the Go stack is only 8-byte aligned), keep constant pools
   inside the function, and turn every return into a jump to one Go RET. Any
   shape it does not know fails: unknown call forms, tail jumps, SP writes,
   escaping stack addresses, folded packed stack operands, read-only symbols
   other than constant pools, forbidden instructions.
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
in `testdata/reject` through the real pipeline. `internal/enginecmp` measures
the current engine under the same GC stop setup as the proto tests.
