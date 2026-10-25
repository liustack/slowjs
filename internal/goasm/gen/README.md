# C-engine generator (phase-0 prototype)

Prototype of the new generator for the C-source engine. It is separate from
the committed Python generator in `internal/goasm`, which keeps producing the
old interpreter. This one proves the calling convention, the stack rules and
the shadow stack on amd64, with an architecture-independent structure (`Arch`)
for arm64.

It is a maintainer tool: it shells out to LLVM 22.1.8 (clang, opt, llc) and
runs its own checks and lowering in Go (`debug/elf`,
`golang.org/x/arch/x86/x86asm`). Applications never run it.

## Pipeline

1. clang -O2 to IR with a kept frame pointer, no red zone, signed wrap and no
   tail calls. CheckIR rejects variadics, byval, sret and long double.
2. The shadow-stack pass (`pass/shadow.cpp`) moves every address-taken local
   and dynamic alloca onto the VM's shadow stack: argument 0 of every engine
   function is the first free shadow byte, each function hands its callees
   the byte above everything it has live, and `__shadow_remaining()` gives C
   the space left for its RangeError checks. No alloca may remain.
3. llc -O2 with shrink-wrapping off writes assembly and an object. CheckObject
   rejects writable or thread-local data and any external symbol outside the
   engine and the host callbacks.
4. LowerUnified / LowerRegister rewrite each function as text: strip the
   prologue and epilogues, load the Go stack arguments into the native
   argument registers, marshal each call (save the callee-saved registers,
   move the arguments to the outgoing area, call, read the result, restore),
   move LLVM's locals onto the Go frame, turn aligned stack moves into
   unaligned ones (the Go stack is only 8-byte aligned), keep constant pools
   inside the function, and turn every return into a jump to one Go RET. Any
   shape it does not know fails: unknown call forms, tail jumps, SP writes,
   escaping stack addresses, folded packed stack operands, read-only symbols
   other than constant pools, forbidden instructions.
5. clang -c re-assembles the lowered text so every branch offset is fixed.
6. Emit writes Go assembly: one TEXT per function, the body as bytes, every
   call a Go-visible CALL.
7. VerifyLinked checks the linked binary: body bytes unchanged, stack-check
   prologue on split entries and on `host_poll`, the function table points
   only at unified entries, no hidden calls or tail jumps.

## Running

`proto.sh` (from the slowjs root, linux/amd64) builds the pass, generates
`internal/proto`, builds its tests and runs the post-link verify. The proto
tests are `testdata/*.txt`, kept as text so this package does not compile
them. `go test` here runs the rule tests. With `SLOWJS_LLVM` (an LLVM bin
dir) and `SLOWJS_SHADOW_PASS` (the built pass) set, it also runs every sample
in `testdata/reject` through the real pipeline. `internal/enginecmp` measures
the current engine under the same GC stop setup as the proto tests.
