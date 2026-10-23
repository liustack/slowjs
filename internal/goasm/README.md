# Offline LLVM interpreter generator

This experiment ships an LLVM-generated `JS_CallInternal` as Go assembly on
linux/amd64. Applications build with `CGO_ENABLED=0 go build -tags goasm`.
Without the tag, the ccgo interpreter remains in use. The generator is a
maintainer tool and does not run during application builds.

The input is the pinned QuickJS source with `lib/internal/patch/quickjs.diff`
applied. LLVM 22.1.8 is required. Python 3, LLVM development headers and a C++
compiler are required only to regenerate the assembly.

## Pipeline

1. Preprocess the patched C and ask Clang for its typed AST. `prepare.py`
   retains the interpreter and selected inline helpers. Every remaining
   call becomes an explicitly typed external function with the SysV ABI.
2. Optimize at `-O2`, with a frame pointer, no red zone and signed wrapping.
   `allocas.cpp` moves all addressable locals to the caller's TLS allocation.
   A dynamic alloca calls `goasmAllocate`. Both small locals and a small alloca
   share the same TLS slot, preserving the fork's recursion accounting.
3. `lower.py` checks the compiler frame shape, converts its local addresses
   to fixed offsets in a Go frame and rejects escaping Go stack addresses.
   It replaces C's R14 with a fixed stack slot. Scratch registers are saved
   and restored using MOV, preserving flags. The actual R14 remains Go's g.
   Computed goto tables use relative offsets within the emitted text.
4. Assemble the transformed GNU syntax offline. `elf2go.py` emits machine
   bytes as Go assembler directives, with real symbolic Go CALL instructions
   for every external relocation. It rejects other relocations. Go supplies
   the stack growth prologue, frame pointer and return sequence.
5. `bridges.py` emits SysV argument marshaling, callee-saved register saves,
   ABI0 calls and typed Go wrappers. Signatures come from the active Go files,
   including libc. The wrappers make indirect calls using ccgo's function
   value representation. The external engine memory contains no movable Go
   stack addresses.

`build.py --help` lists the input paths. Build `allocas.cpp` as a shared LLVM
pass plugin, then pass it with `--plugin`. Produce the Go signature JSON with:

```sh
GOOS=linux GOARCH=amd64 go list -json ./lib modernc.org/libc |
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
It checks the body alignment and rejects any other byte change. Run it on each
code layout used for timing.
