---
summary: 'The goasm interpreter, the default on linux/amd64: how to turn it off, what it gains with one VM and with several, its memory and GC stops, its stack and ABI rules, how to regenerate and check it, and the one check left'
read_when:
  - Turning off the goasm interpreter in a program, or deciding whether to
  - Building or regenerating the goasm interpreter
  - Changing the C patch, the interpreter rules of fixlib, Go versions or LLVM versions
  - fixGoasm, TestGoasmCoversInterpreterRules, TestGoasmCoversHelperRules or TestOneInterpreterPerBuild stops
---

# The goasm interpreter

On linux/amd64 a program runs `JS_CallInternal`, the bytecode interpreter, as
machine code that LLVM compiled from the patched QuickJS C, shipped in the
module as Go assembly. Everything else of the engine stays the Go that ccgo
makes. Other targets run the Go interpreter that ccgo makes from the same C.

## Turning it off

Pass the `slowjs_nogoasm` tag to the Go command that builds the program:

```sh
CGO_ENABLED=0 go build -tags slowjs_nogoasm ./cmd/server
```

- It takes effect on linux/amd64 only. darwin, linux/arm64 and windows
  always run the Go interpreter, with or without the tag.
- The `goasm` tag, which turned the interpreter on up to v0.28.5, is still
  accepted and changes nothing.
- The default build needs nothing more than before: no cgo, no C compiler,
  no LLVM and no shared library. The assembly is committed.
- Scripts give the same results with either interpreter: the same test262
  errors, the same HTML of tifa's pages, the same limits (eval timeout,
  memory limit, stack depth, uncatchable interrupts). A panic in a Go
  function and a nil dereference in the engine still reach Go as before.
- Profiles show the whole goasm interpreter as one function, `goasmRun`, and
  stack traces of a crash inside it end there. Turn it off to profile the
  interpreter by Go function.
- tifa builds with it unless the project file says `build.goasm: false`.

What it gains, measured on 2026-10-24 on a GCP VM of an AMD EPYC 7B13
(one core, GOMAXPROCS=1, no PGO, three code layouts, six alternating rounds
each, sum of the median render times): the six shop workbench pages of
tifa render 6.0% faster than with the Go interpreter (0.922 to 0.964 of
its time), and the three pages of tifa's MUI test app 15.0% faster
(0.834 to 0.875). The gain is largest on code that runs many bytecodes per
call into Go. With PGO on both builds, each with its own profile, the gap
stays the same or grows (6.1% and 14.2% for the previous generated code,
in four rounds of one layout, against 4.0% and 12.9% without PGO).

With several VMs it gains more. Pages rendered a second by tifa's pool, one
goroutine per VM, the pages in turn, 60 seconds after a warmup, three
rounds over three code layouts, no PGO, slowjs v0.28.5 with and without the
tag of that release, on the same machine with nothing else running:

| Pages | VMs and CPUs | Go interpreter | goasm | goasm over Go | P50, P99 (ms) Go | P50, P99 (ms) goasm |
| --- | --- | --- | --- | --- | --- | --- |
| shop | 4 VMs on 4 cores, GOMAXPROCS=4 | 252.6 | 264.2 | 1.046 (1.026 to 1.082) | 10.9, 30.3 | 10.3, 29.8 |
| shop | 8 VMs on 8 hyperthreads, GOMAXPROCS=8 | 302.5 | 328.2 | 1.085 (1.060 to 1.118) | 18.2, 52.2 | 16.7, 48.0 |
| MUI | 4 VMs on 4 cores | 41.3 | 48.4 | 1.171 (1.148 to 1.194) | 70.4, 158 | 60.6, 135 |
| MUI | 8 VMs on 8 hyperthreads | 49.9 | 60.4 | 1.211 (1.205 to 1.219) | 117, 263 | 96.4, 219 |

It also takes less memory. After ten minutes of 8 VMs rendering (about
180,000 to 195,000 shop renders, or 30,000 to 36,000 MUI renders), the process held:

| Pages | Build | RSS after warmup | RSS after 10 minutes | Go heap in use | Memory Go mapped |
| --- | --- | --- | --- | --- | --- |
| shop | Go interpreter | 123.4 MiB | 142.2 MiB | 4.4 MiB | 46.0 MiB |
| shop | goasm | 117.7 MiB | 132.6 MiB | 4.5 MiB | 34.2 MiB |
| MUI | Go interpreter | 93.6 MiB | 109.9 MiB | 2.6 MiB | 45.4 MiB |
| MUI | goasm | 89.1 MiB | 97.2 MiB | 2.6 MiB | 30.0 MiB |

The Go interpreter needs goroutine stacks that grow with the depth of the
JavaScript calls, and the goasm interpreter keeps its frames in the TLS
slots of the engine, so Go maps less. RSS grows over the ten minutes with
both interpreters.

## How it is made

The generator in `internal/goasm` requires LLVM 22.1.8, its development headers,
a C++ compiler and Python 3. It is specific to this interpreter and compiler
version. It does not use goc or c2goasm. See its README for the pipeline.

## Stack and calls

All addressable C locals move to stable TLS memory after LLVM optimization.
The interpreter takes one 2048-byte TLS slot, matching the existing linux/amd64
slot count. A dynamic allocation of up to 768 bytes shares that slot. Larger
ones use libc's allocator, as the ccgo interpreter does. The body addresses
the locals at a fixed offset from the start of the slot, so they take no
register of their own, and it computes each address where it is used, so
the addresses are not spilled to the frame. The fixed Go frame is 352
bytes. Go emits its stack growth prologue and frame pointer maintenance.

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
and bitwise operators on numbers (see [Keeping the two interpreters
together](#keeping-the-two-interpreters-together)). Fast paths that live in
Go helpers, such as atom lookup, shape caches and object literals, apply to
both interpreters.

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
| goasmRun has a fixed frame with no pointer map. The body reads only its `env` and `locals` arguments, both C memory (it addresses the locals from `env`), never the typed TLS argument, so no Go pointer lands in the frame. | `lower.py` (frame shape, accesses outside the compiler frame), `checks.text_header` |
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
| shop | Go interpreter | 25 to 29 µs | 164 to 197 µs | 197 to 229 µs |
| shop | goasm | 25 to 29 µs | 66 to 82 µs | 197 to 229 µs |
| MUI | Go interpreter | 21 to 25 µs | 131 to 164 µs | 164 to 197 µs |
| MUI | goasm | 21 to 25 µs | 41 to 49 µs | 164 to 197 µs |

On four CPUs (two cores) with GOMAXPROCS=4, P50 is 33 to 41 µs and P99 164
to 197 µs for both builds, and the largest stop is 197 to 229 µs, apart from
one stop of 2.1 to 2.6 ms in one of eight MUI rounds of the goasm build.
With GOMAXPROCS=8 on the same four CPUs, the operating system decides the
largest stops: 2.6 to 3.1 ms on shop for both builds, about 1 ms for MUI on
the Go interpreter and 0.7 ms on the goasm interpreter.

With the VMs of the table above (2026-10-24, 4 VMs on 4 cores and 8 VMs on 8
hyperthreads, 200 forced GCs in each of three rounds while every VM
renders), P50 is 29 to 66 µs and P99 197 to 229 µs for both interpreters.
The largest stop is 197 µs on 4 cores for both, and 1.05 ms on 8
hyperthreads for both, apart from 1.31 ms once for the goasm interpreter on
the shop.

The interpreter calls Go often enough (more than 100 thousand bridge calls
for one render of the six shop pages) that the assembly is no slower to stop
than the Go interpreter, so it has no extra preemption point. Measure again when a change
removes Go calls from hot paths.

## Keeping the two interpreters together

The generated interpreter replaces the whole Go `_JS_CallInternal`, so the
changes `fixlib` makes inside that function do not reach it.
`goasmInterpreterRules` in `internal/fixlib/goasm.go` gives each file of
`fixlib` that changes the interpreter a row: written in C by
`internal/goasm/cpatch.py` (fieldchain, eq, bitwise), already in the patched
C (the uncatchable check, the guard against a pending stack, the interrupt
countdown), or only for Go's compiler (cells, pin, inline, one frame,
alloca, defers, fast frames). `TestGoasmCoversInterpreterRules` fails when a
file of `fixlib` changes the interpreter and has no row, when a row is
stale, or when a C row names a change `cpatch.py` does not.

The generated interpreter also compiles in its own copy of the small C
helpers it calls, such as `find_own_property`, `get_shape_prop` and
`set_value`. The generator lists them in `lib/goasm_helpers.txt`. A change
of `fixlib` to the Go function of one of them does not reach the assembly
either, so `goasmHelperRules` gives a row, with the same three kinds, to
each file of `fixlib` that changes one: names the function on its own (as
a list of functions to rewrite, fingerprint or clone does), holds its first
line (as a hook does), or left a "(see fixX)" mark in it. Today every such
file only fingerprints a helper or reshapes its Go frame.
`TestGoasmCoversHelperRules` fails on a missing or stale row in the same
way. Helpers that ccgo writes into their callers have no Go function of
their own and change with the interpreter.

## Verification and maintenance

`fixGoasm` checks the complete original Go interpreter fingerprint and checks
its inverse byte for byte. A changed interpreter requires review before
updating the fingerprint. New C types, call signatures, compiler prologues,
stack instructions or relocations require generator changes, not acceptance
of unknown output.

`internal/goasm/regenerate.py --check` regenerates the assembly from the
committed sources and fails when it differs from the committed files. The
commands of every check are in [testing.md](../testing.md#the-goasm-interpreter).
Compare rendered HTML, then measure the goasm build against the Go one of
`slowjs_nogoasm` across three layouts and six alternating rounds. Check CPU profiles and GC
stopping latency in the final linked application.

### When upstream changes it

- `fixGoasm` stops with "interpreter fingerprint changed": upstream or
  another change of `fixlib` changed the Go `_JS_CallInternal`. Find what
  changed, give the C the same behavior if it is not a Go compiler
  workaround (in `lib/internal/patch/quickjs.diff` or `cpatch.py`), update
  the fingerprint, then regenerate and run the goasm checks.
- `cpatch.py` stops on a replacement count: the C it rewrites changed. Port
  the fast path to the new C.
- `TestGoasmCoversInterpreterRules` or `TestGoasmCoversHelperRules`
  fails: add the row the message asks for, and port the fast path to
  `cpatch.py` if it changes what the interpreter or the helper does.
- `regenerate.py --check` fails: a change to the C patch, a struct of the
  engine or the generator changed the output. Regenerate with
  `regenerate.py` and commit the files with the change that caused them.

## Before it became the default

These held before the interpreter became the default on linux/amd64:

- The drift check (`regenerate.py --check`) runs on every release, and the
  rule table checks and `TestOneInterpreterPerBuild` stay green.
- Every release runs the full checks with both interpreters: short, fixlib,
  memgrind, race, test262, HTML of tifa's pages, all five targets.
- Throughput of several VMs, memory over a long run and GC stops under that
  load were measured: see the tables above.
- tifa's own tests pass with it: `go test ./...`, the SSR tests of the
  example shop, the soak tests (running out of memory, VM reuse) and the
  end-to-end tests.
- It was checked with Go 1.27.2. After a Go upgrade, run the goasm checks
  of [testing.md](../testing.md#the-goasm-interpreter) and `verify_linked.py`
  on a linked program before tifa moves to that Go. The generated code
  relies on ABI0 leaving R14 and X15 free and on the morestack prologue:
  `verify_linked.py` catches a changed prologue or call target, not a
  changed meaning.

One check is left, to be done last, after the other work on the assembly:

- Measure it on the target machine, a 1-core 1 GB VPS: latency, throughput
  and RSS of tifa's shop with and without `slowjs_nogoasm`.

darwin/arm64 is out of scope: it needs another ABI bridge and lowering,
which this generator does not make.
