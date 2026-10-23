# Offline LLVM interpreter generator

This experiment ships an LLVM-generated `JS_CallInternal` as Go assembly on
linux/amd64. Applications build with `CGO_ENABLED=0 go build -tags goasm`.
Without the tag, the ccgo interpreter remains in use. The generator is a
maintainer tool and does not run during application builds.

The input is the pinned QuickJS source with `lib/internal/patch/quickjs.diff`
applied. LLVM 22.1.8 is required. Python 3, LLVM development headers and a C++
compiler are required only to regenerate the assembly.

## Pipeline

1. `cpatch.py` gives the C interpreter the fast paths that `internal/fixlib`
   adds only to the Go interpreter: property reads that walk the prototype
   chain past exotic objects (fixFieldChain), the usual `===` and `==` pairs
   (fixEq), and bitwise operators on two numbers (fixBitwise). It also takes
   the string or symbol of an atom in C. Each replacement must match the
   expected number of times.
2. Preprocess the patched C and ask Clang for its typed AST. `prepare.py`
   retains the interpreter and selected inline helpers. Every remaining
   call becomes an explicitly typed external function with the SysV ABI. A
   call of the interpreter to itself takes the next TLS slot in C and enters
   the new frame through `goasm_reenter`, with no Go code in between. A call
   to a native function takes the slot and calls the function directly. The
   stack depth check reads the TLS slot index in C.
3. Optimize at `-O2`, with a frame pointer, no red zone and signed wrapping.
   `allocas.cpp` moves all addressable locals to the caller's TLS slot, at
   a fixed offset from the slot start so they need no register of their
   own, and turns the computed goto table into offsets from itself. A dynamic alloca
   of up to 768 bytes follows the locals in the slot. A larger one calls
   `goasmAllocate`, which mallocs it.
4. `lower.py` checks the compiler frame shape, converts its local addresses
   to fixed offsets in a Go frame and rejects escaping Go stack addresses.
   R14 is an ordinary C register: Go's ABI0 leaves R14 undefined, and Go
   reloads g from thread local storage around ABI0 calls and in signal
   handlers. Dispatch must jump through a register.
5. Assemble the transformed GNU syntax offline. `elf2go.py` emits machine
   bytes as Go assembler directives, with real symbolic Go CALL instructions
   for every external relocation. It rejects other relocations. Go supplies
   the stack growth prologue, frame pointer and return sequence.
6. `bridges.py` emits, for each external call, a stub that saves the SysV
   callee-saved registers (R14 included), moves the arguments to an ABI0
   frame and calls Go. When the target is a function of `lib` with matching
   argument sizes, the stub calls it directly with the TLS in place of the
   environment. Other targets get a typed Go wrapper. Indirect calls use
   ccgo's function value representation. The external engine memory
   contains no movable Go stack addresses.

`checks.py` holds the stack rules Go's movable stack needs. Each step runs
the checks for what it can break, and generation stops on a violation. See
the Stack rules section of `docs/fork-changes/go-assembly.md`.

`build.py --help` lists the input paths. Build `allocas.cpp` as a shared LLVM
pass plugin, then pass it with `--plugin`. Produce the Go signature JSON with:

```sh
GOOS=linux GOARCH=amd64 go list -tags goasm -json ./lib modernc.org/libc |
  go run ./internal/goasm/cmd/signatures > signatures.json
```

Run `python3 -m unittest discover -s internal/goasm` for generator checks.
The engine selection is inserted by `internal/fixlib`, checks the complete
original interpreter fingerprint and verifies an exact inverse.

## Review boundaries

The generator deliberately supports one interpreter and compiler version.
New C types, indirect signatures, prologues, stack operations, relocations or
TLS local sizes require review. Changing the compiler requires regeneration,
inspection of the final linked instructions, engine tests, test262, HTML
comparison and page timing again. A successful offline build is not evidence
that arbitrary C functions can use this path safely.

The experimental implementation is not enabled by default. Darwin/arm64 has
no implementation in this branch.

After linking a tagged ELF binary, check that every local byte and jump table
survived Go assembly and linking:

```sh
python3 internal/goasm/verify_linked.py output/lowered.o application
```

This permits only the four relocated displacement bytes of each symbolic CALL.
It checks the body alignment and rejects any other byte change. It also checks
that goasmRun and `__js_poll_interrupts` keep their stack check prologue. Run it on each
code layout used for timing.
