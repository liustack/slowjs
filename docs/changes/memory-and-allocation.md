---
summary: "The fork changes to how VMs share memory: one copy of the function source text and line tables for every VM that loads the same bytecode, with what it gains and costs in memory and why it is safe"
read_when:
  - Asking how much memory a VM holds after a GC, or what a VM keeps outside JS_ComputeMemoryUsage
  - Loading one bundle into many VMs (SharedBytecode, EvalSharedBytecode)
---

# Memory and allocation

The engine has an allocator of its own, with a front cache of freed small blocks, which [allocator.md](../allocator.md) describes. This page has what the fork changes about how the VMs of one program share memory.

## One copy of source text and line tables for every VM

Release: Unreleased.

A VM that loads bytecode with `EvalBytecode` copies into its own memory the source text of each function, which `toString` returns, and the table that maps its bytecode to line numbers, which error stacks read. A server that loads one bundle into a pool of VMs holds those bytes once per VM, and no VM ever writes them. `NewSharedBytecode` now copies the bytecode once, outside Go's heap, and `VM.EvalSharedBytecode` (and `EvalSharedBytecodeValue`) loads it with the flag `JS_READ_OBJ_SHARED_DEBUG` of `JS_ReadObject`: the source text and line table of each function it reads point into that copy, and the function says so with the bit `shared_debug` of `JSFunctionBytecode`. Everything else a VM reads, the code, the atoms and the constants, is still its own.

What javascript sees does not change: `toString` gives the same text, and error stacks the same lines and columns, as `TestSharedBytecodeSameAsCopy` checks against `EvalBytecode` with functions, arrows, a class with a getter and a static method, an async method, an error thrown two calls down, a stack made in a function and a function with 256 KiB of source.

Why it is safe:

- The copy stays while the `SharedBytecode` is open or a VM that loaded it is. `SharedBytecode` counts its holds: its own until `Close`, and one per VM that loaded it, which `VM.Close` gives up after the runtime and every function in it are freed. The last one frees the copy. A VM keeps its hold also when the read failed half way, as the functions read before the error point into the copy until the runtime is freed. Loading after `Close` returns an error. So a VM can outlive the pool that made it, and `TestSharedBytecodeLifetime` checks that, with the copy freed only by the last close.
- The engine never writes or moves the source text or line table of a function it read from bytecode, it only writes those of a function it compiles itself. `free_function_bytecode` does not free them when `shared_debug` is set.
- The read checks that each of them lies inside the buffer, and fails on a cut buffer as a copy does (`TestSharedBytecodeTruncated`).
- The memory limit, `MemorySize`, the GC threshold and `JS_ComputeMemoryUsage` count what the runtime allocated, so they no longer count the shared bytes: `compute_bytecode_size` leaves them out. The copy is counted once, by the program, as `SharedBytecode.Len`.

The flag is kept in `BCReaderState` and read by `bc_get_debug_buf`, which `JS_ReadFunctionTag` calls for the two buffers. The copy is a read-only mapping outside every VM (`hostmem.MapReadOnly`): the engine of each VM reads it in place, with no lock, and a write to it faults (`TestSharedBytecodeReadOnly`). The last hold unmaps it.

What it saves, on linux/amd64 (AMD EPYC 7B13), measured in tifa with its pool, which loads the bundle with `EvalSharedBytecode` (`JS_ComputeMemoryUsage` of a VM after loading, the same in each VM):

| Bundle | Bytecode | A VM after loading, before | After | Less per VM |
| --- | --- | --- | --- | --- |
| tifa's example shop | 5.27 MB | 11,459,272 bytes | 8,986,552 | 2.36 MiB (0.784) |
| The MUI test app of tifa | 2.73 MB | 5,436,184 bytes | 4,155,576 | 1.22 MiB (0.764) |

The functions of the bundle that stay after loading hold 1.34 MB of source text and 0.30 MB of line tables in the shop. The rest of the difference is the allocator's headers and rounding of those blocks. With 8 VMs the RSS of the process goes from 122.0 to 100.9 MB for the shop and from 72.7 to 57.9 MB for MUI. Render time does not change. Timed in tifa without PGO, one VM on one thread of an idle core pair, main and the branch taking turns over 3 code layouts (23 pairs of runs), the 6 workbench pages of the shop render in 0.994 of the time at the median (0.979 to 1.015) and the 3 pages of the MUI test app in 0.994 (0.981 to 1.005). The pool collects as often as before (the same count of young and full collections), and the HTML is the same byte for byte.

