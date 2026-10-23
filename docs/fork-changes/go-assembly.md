---
summary: 'Experimental LLVM interpreter on linux/amd64: build tag, stack and ABI boundaries, generator maintenance and verification'
read_when:
  - Building or regenerating the experimental goasm interpreter
  - Changing Go versions, LLVM versions or interpreter call signatures
---

# Experimental LLVM interpreter

The `goasm` build tag replaces `JS_CallInternal` on linux/amd64 with assembly
generated offline from the patched C. Applications use ordinary Go builds with
`CGO_ENABLED=0`. They need no C compiler, LLVM installation or shared library.
The default build and other targets retain the ccgo interpreter.

The generator in `internal/goasm` requires LLVM 22.1.8, its development headers,
a C++ compiler and Python 3. It is specific to this interpreter and compiler
version. It does not use goc or c2goasm. See its README for the pipeline.

## Stack and calls

All addressable C locals move to stable TLS memory after LLVM optimization.
The interpreter takes one 2048-byte TLS slot, matching the existing linux/amd64
slot count. Small dynamic allocations share that slot. Larger ones use libc's
allocator, as the ccgo interpreter does. The fixed Go frame is 560 bytes.
Go emits its stack growth prologue and frame pointer maintenance.

The generated body never uses the actual R14 or RBP as C registers. A fixed
stack slot represents LLVM's R14, with checked instruction rewrites. RBP stays
Go's frame pointer. The generator rejects dynamic SP changes and addresses of
Go stack locals. The compiler red zone is disabled.

Typed bridges convert SysV arguments and results to Go ABI0 and preserve C's
callee-saved registers across Go calls. They pass the current TLS to the Go
engine. The entry's typed TLS argument keeps that Go pointer reachable. The
raw assembly frame contains no Go heap pointers or published Go stack pointers.

Computed goto remains enabled. Relative jump tables stay inside the generated
text. External calls are symbolic Go CALL instructions. Other ELF relocations
are rejected. The interrupt countdown calls Go every 10000 instructions.
Assembly itself has no asynchronous safe points, so GC stopping latency must
be measured under real load when changing this implementation.

An engine panic or Goexit retains the existing rule: the VM may only Close.
A nil fault in the assembly is tested for recovery and a Go stack trace naming
`goasmRun`. This does not promise recovery from arbitrary memory corruption.

## Verification and maintenance

`fixGoasm` checks the complete original Go interpreter fingerprint and checks
its inverse byte for byte. A changed interpreter requires review before
updating the fingerprint. New C types, call signatures, compiler prologues,
stack instructions or relocations require generator changes, not acceptance
of unknown output.

Run the generator tests, the short Go suite with `-tags goasm`, CallDepth,
TLS, EnginePanic and EngineGoexit checks, memgrind, and test262 with
`-xtags goasm`. Compare rendered HTML, then measure the default and tagged
builds across three layouts and six alternating rounds. Check CPU profiles
and GC stopping latency in the final linked application.

Darwin/arm64 uses the Go fallback. Native ARM assembly needs a separate ABI
bridge and register and frame lowering implementation. It is not produced by
this generator.
