---
summary: "Changes to how VMs share what they load: one copy of the function source text and line tables for every VM that loads the same bytecode, and the scan of that bytecode for the polls on its first load only, with what each gains and costs and why it is safe"
read_when:
  - Asking how much memory a VM holds after a GC, or what a VM keeps outside JS_ComputeMemoryUsage
  - Loading one bundle into many VMs (SharedBytecode, EvalSharedBytecode)
  - Loading a bundle takes long, or a read of bytecode fails with "invalid bytecode"
---

# Memory and allocation

The engine has an allocator of its own, with a front cache of freed small blocks, which [allocator.md](../allocator.md) describes. This page has what slowjs changes about how the VMs of one program share memory.

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


## Shared bytecode is scanned for the polls on its first load only

Release: Unreleased.

A VM that reads bytecode holds the code of each function to the rule of the polls (`check_polls`, then `poll_scan`): no run of more than 127 instructions without a count, no opcode the interpreter does not have, and the places `OP_ret` may go. That scan was half of `JS_ReadObject`: 2.1 M of the 4.7 M instructions of reading the 117 KB React bundle, and 26.9 M of the 67.5 M of reading the bundle of the example shop in the comparison. A pool that loads one `SharedBytecode` into every VM scanned the same bytes again for each of them.

`SharedBytecode` now marks itself checked once a VM read it without an error, and `VM.EvalSharedBytecode` loads it from then on with the flag `JS_READ_OBJ_POLLS_CHECKED`. `JS_ReadFunctionBytecode` skips `poll_scan` with it, and still makes the places `OP_ret` may go (`ret_bits`), which it needs for each function it reads. `VM.EvalBytecode` has no such mark and scans every time.

Why it is safe:

- The read that sets the mark has passed `check_polls` for every function of the bytes, since it fails at the first function that does not, and the copy is a read-only mapping that no VM can write (`TestSharedBytecodeReadOnly`). The next read sees the same bytes.
- Bytes that do not read are never marked, so every load of them fails as the first did (`TestSharedBytecodeChecksOnce`). Two loads at once both scan, and either may set the mark.
- The flag is for a caller that read the same bytes before. The Go API sets it only from the mark. `NewSharedBytecode` already says that its bytecode is not checked, and so must not come from an untrusted source.

What it saves, on linux/amd64 (AMD EPYC 7B13), with callgrind at the first code position, per new VM, load and close (the difference of runs of 13 and 3), against main:

| Bundle | Instructions before | After | `JS_ReadObject` before | After |
| --- | ---: | ---: | ---: | ---: |
| React SSR bundle (117 KB) | 11.05 M | 8.90 M (-19.5%) | 4.72 M | 2.56 M |
| Shop bundle of the comparison, with `Intl` | 235.3 M | 208.4 M (-11.4%) | 67.5 M | 40.6 M |

Time, timed in Go benchmarks on one core of an EPYC 7B13 with its sibling idle, at the 8 code positions, 5 rounds: a new VM and `EvalSharedBytecode` of the React bundle take 1.899 ms, then 1.498 ms (0.789, every position faster), and of the shop bundle 34.4 ms, then 29.7 ms (0.862). The same load with `EvalBytecode` takes 2.441 ms, then 2.443 ms: it scans every time. A new VM alone takes the same 0.24 ms.

Tests: `engine/test/unit-white.c` reads code that breaks the rule of the polls with and without the flag (an error without it, read with it), and checks that `OP_ret` to a place that follows no `OP_gosub` is still an `InternalError` with the flag, and a finally block still runs. `sharedbytecode_test.go` checks the mark is set by the first good load and not by bytes that fail, and that each VM shows the same.

The change is in C (`engine/quickjs.c`, `engine/quickjs.h`): `JS_READ_OBJ_POLLS_CHECKED`, `check_polls` with its `scan` argument, and in Go `SharedBytecode.checked`.
