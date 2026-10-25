# C-engine generator (phase-0 prototype)

Prototype of the new generator for the C-source engine (see
`.issues/2026-10-26-c-engine/plan.md`). It is separate from the committed
Python generator in `internal/goasm`, which keeps producing the old
interpreter. This one proves the plan's calling convention and stack rules on
amd64, with an architecture-independent structure (`Arch`) ready for arm64.

It is a maintainer tool: it shells out to LLVM 22.1.8 (clang, llc) and runs its
own checks and lowering in Go (`debug/elf`, `golang.org/x/arch/x86/x86asm`).
Applications never run it.

## Pipeline

1. clang at -O2 with a kept frame pointer, no red zone, signed wrap, no tail
   calls and shrink-wrapping off compiles each engine function to one canonical
   prologue. Leaves compile frameless.
2. LowerUnified / LowerRegister rewrite each function as text: strip the
   prologue/epilogue, add an entry shim loading the Go stack arguments into the
   native argument registers, marshal each internal call (save live
   callee-saved, move arguments to the outgoing area, call, read the result,
   restore), rewrite frame spills onto the Go frame, and turn every return into
   a store plus a jump to one Go RET.
3. clang -c re-assembles the lowered text so every branch offset is fixed.
4. Emit disassembles the object and writes Go assembly: each function a TEXT,
   the body as machine-code bytes, every call a Go-visible CALL (direct via a
   relocation, indirect as CALL <reg>).
5. Checks: CheckIR (no variadics, byval, sret, long double), CheckObject (no
   writable data, closed external allow list), the lowering's instruction
   rules (forbidden instructions, SP writes, escaping stack addresses,
   unreviewed indirect jumps), and VerifyLinked after the Go linker (body bytes
   unchanged, morestack prologue on split entries, function table points only
   at unified entries, no hidden calls or tail jumps).

## Scope

Proven on amd64: unified and register-entry calling conventions, direct and
indirect calls, int/float/16-byte-JSValue arguments and returns, deep
recursion through morestack, concurrent GC, back-edge interrupt poll, the
static checks and the post-link verify. Not yet built: the shadow-stack
allocator (address-taken locals, dynamic alloca, overflow RangeError), the
Go-callback reentry hand-off with panic/Goexit, the floating-point constant
pool, and the arm64 backend (rules recorded in phase0-arm64).

testdata/*.go.txt are the proto behavior, timing and GC tests, kept as .txt so
they are not compiled as part of this package. The driver is cmd/gen.
