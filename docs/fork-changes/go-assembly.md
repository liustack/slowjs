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
slot count. A dynamic allocation of up to 768 bytes shares that slot. Larger
ones use libc's allocator, as the ccgo interpreter does. The fixed Go frame
is 608 bytes. Go emits its stack growth prologue and frame pointer
maintenance.

The generated body uses R14 as an ordinary C register. Go's ABI0 leaves R14
undefined: Go reloads g from thread local storage after an ABI0 call and in
signal handlers, and every bridge saves R14 with the other SysV callee-saved
registers. RBP stays Go's frame pointer. The generator rejects dynamic SP
changes and addresses of Go stack locals. The compiler red zone is disabled.

A call of the interpreter to itself stays in assembly: it takes the next TLS
slot as the Go interpreter does and enters the new frame through a small
stub, so Go still checks the goroutine stack. A call to a native function
takes the slot and calls the function through a bridge. Typed bridges convert
SysV arguments and results to Go ABI0. A bridge to a function of `lib` calls
it directly, with the TLS in place of the environment. The raw assembly frame
contains no Go heap pointers or published Go stack pointers.

The C interpreter carries the fast paths that `internal/fixlib` adds to the
Go interpreter for property reads along the prototype chain, `===` and `==`,
and bitwise operators on numbers. Fast paths that live in Go helpers, such as
atom lookup, shape caches and object literals, apply to both interpreters.

Computed goto remains enabled. Relative jump tables stay inside the generated
text. External calls are symbolic Go CALL instructions. Other ELF relocations
are rejected.

An engine panic or Goexit retains the existing rule: the VM may only Close.
A nil fault in the assembly is tested for recovery and a Go stack trace naming
`goasmRun`. This does not promise recovery from arbitrary memory corruption.

## Stack rules

Go moves a goroutine stack when it grows, and its GC scans each frame by its
pointer map. The generator enforces these rules and stops when one breaks.
Each check has a test with a small input it rejects
(`internal/goasm/test_checks.py`, `test_lower.py`).

| Rule | Checked by |
| --- | --- |
| goasmRun has a fixed frame with no pointer map. The body reads only its `env` and `locals` arguments, both C memory, never the typed TLS argument, so no Go pointer lands in the frame. | `lower.py` (frame shape, accesses outside the compiler frame), `checks.text_header` |
| The body never changes SP and never takes the address of a stack slot. Bridges neither. | `lower.py`, `checks.bridge_stack` |
| Every addressable C local lives in the TLS slot. | `checks.no_allocas` on the IR after `allocas.cpp` |
| goasmRun is not NOSPLIT, so Go gives it the stack check prologue. | `checks.text_header`, and `verify_linked.py` finds the call to `runtime.morestack_noctxt` in the linked function |
| The interpreter enters itself only through `goasm_reenter`, which calls goasmRun and so passes that prologue. | `checks.call_targets`, `checks.reenter` |

The bridges and `goasm_reenter` are NOSPLIT with small fixed frames. The Go
linker checks that every chain of NOSPLIT frames fits the stack guard.
`TestStackGrowthInDeepRecursion` renders HTML through 1500 nested JS calls on
a new goroutine, directly and through `Array.prototype.map`, and checks that
the stack moved and the HTML is right.

## GC stopping

The assembly has no asynchronous safe points: the signal Go sends to stop a
goroutine finds no safe point in goasmRun. GC stops the goroutine at the
next stack check prologue of a Go function the interpreter calls. A script
that calls nothing still reaches one: every call and every goto, if_true
and if_false, which include every loop back edge, count the interrupt
countdown down, and every
10000 counts the countdown calls `__js_poll_interrupts`, a Go function with
a prologue. `verify_linked.py` checks that prologue in the linked binary, and
`TestGCStopsLoopWithoutCalls` runs GC against the three kinds of loop.

Measured on tifa's pages (2026-10-23, 8 vCPU AMD EPYC 7B13, GOMAXPROCS=2 on
one core's two threads): four VMs render on four goroutines while another
goroutine forces 300 GCs, three rounds per build. The time to stop the world,
from `/sched/pauses/stopping/gc:seconds` (the histogram gives a range):

| Pages | Build | P50 | P99 | Max |
| --- | --- | --- | --- | --- |
| shop | default | 25 to 29 µs | 164 to 197 µs | 197 to 229 µs |
| shop | goasm | 25 to 29 µs | 66 to 82 µs | 197 to 229 µs |
| MUI | default | 21 to 25 µs | 131 to 164 µs | 164 to 197 µs |
| MUI | goasm | 21 to 25 µs | 41 to 49 µs | 164 to 197 µs |

The interpreter calls Go often enough (more than 100 thousand bridge calls
for one render of the six shop pages) that the assembly is no slower to stop
than the Go interpreter, so it has no extra preemption point. Measure again when a change
removes Go calls from hot paths.

## Verification and maintenance

`fixGoasm` checks the complete original Go interpreter fingerprint and checks
its inverse byte for byte. A changed interpreter requires review before
updating the fingerprint. New C types, call signatures, compiler prologues,
stack instructions or relocations require generator changes, not acceptance
of unknown output.

Run the generator tests, the short Go suite with `-tags goasm`, CallDepth,
TLS, EnginePanic, EngineGoexit, StackGrowth and GCStops checks, memgrind,
and test262 with `-xtags goasm`. Compare rendered HTML, then measure the
default and tagged builds across three layouts and six alternating rounds.
Check CPU profiles and GC stopping latency in the final linked application.

Measured on 2026-10-23 (no PGO, three layouts, six alternating rounds, sum
of the P50 render times): the tagged build renders the six shop workbench
pages 4.2% faster than the default build and the three MUI pages 11.0%
faster.

Darwin/arm64 uses the Go fallback. Native ARM assembly needs a separate ABI
bridge and register and frame lowering implementation. It is not produced by
this generator.
