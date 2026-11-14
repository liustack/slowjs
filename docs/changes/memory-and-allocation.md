---
summary: "Changes to how VMs share what they load and to when a VM collects: one copy of the function source text and line tables for every VM that loads the same bytecode, the scan of that bytecode for the polls on its first load only, and a collection before a large block of a built-in that would pass the memory limit, with what each gains and costs and why it is safe"
read_when:
  - Asking how much memory a VM holds after a GC, or what a VM keeps outside JS_ComputeMemoryUsage
  - Loading one bundle into many VMs (SharedBytecode, EvalSharedBytecode)
  - Loading a bundle takes long, or a read of bytecode fails with "invalid bytecode"
  - A script runs out of memory under a memory limit with garbage left, or asking when the engine collects
---

# Memory and allocation

English · [简体中文](memory-and-allocation.zh-CN.md)

The engine has an allocator of its own, with a front cache of freed small blocks, which [allocator.md](../allocator.md) describes. This page has what liustack/quickjs changes about how the VMs of one program share memory.

## One copy of source text and line tables for every VM

Release: v0.28.1.

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

Release: v0.29.1.

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

## A large block collects first when it would pass the memory limit

Release: v0.30.5.

Garbage in reference cycles stays until a collection, and the engine starts one only when it makes an object, or calls a JavaScript function, with the memory past the GC threshold. After a collection a script can leave up to the threshold less what is live as such garbage: half of what is live, with the threshold at 1.5 times it. A built-in that then asks for one large block, and makes no object first, ran out of memory at the limit with that garbage still there: `'x'.repeat(n)` failed for an `n` that fitted once the garbage was gone.

Now these places run a full collection first when the block would take the VM past its memory limit, and then ask for it:

| Built-in | Where the check is |
| --- | --- |
| `String.prototype.repeat`, `padStart`, `padEnd` | after the conversions of `this` and the arguments, before the buffer |
| the data of an `ArrayBuffer`: its constructor, every typed array constructor, `slice` and the others that make one | before the object, which `js_create_from_ctor` makes after reading `prototype` of `new.target` |
| `ArrayBuffer.prototype.resize` and `transfer` | after the conversion of the length |
| `Array.prototype.push` | when the elements must grow, at the entry of the built-in |
| `a[i] = v` one past the end of a fast array, in the register loop | when the elements must grow, before `JS_SetPropertyValue`, which may call a setter |
| a new array of a given length (`toSorted`, `toReversed`, `toSpliced`, `with`), and the copy of `slice` and `splice` | before `JS_NewArray`, which checks the threshold |
| `Array.prototype.sort` and `toSorted` | before the list of values to sort grows, between two reads of an element |
| `Array.prototype.join` | when its buffer grows to 64 Ki characters or more, or is made 16 bits wide, between the reads of two elements |
| a string made by `+` (a rope) made flat | at the entry of a `String.prototype` method, and in `JSON.stringify` for a value |

Why it is safe: a collection already runs wherever an object is made and wherever JavaScript runs, and each new place is one of those. It is after a conversion that may call `valueOf` or `toString`, after the read of an element that may call a getter, before a set that may call a setter, or just before an object is made, with nothing changed in between. There, every value the built-in holds is counted: by the frame of its caller, by its own locals, or by a buffer of counted values (the list of `sort`), and no object is half made. A collection frees only what nothing counted reaches, so it frees none of them, and it moves no block that is still in use. A threshold of -1, which turns the collections of the engine off, turns this off too.

What it costs: the check compares what the block adds to the memory with the room left under the limit, and runs a collection, in a function of its own, only when the block would not fit. A block counts as the allocator counts it, with 8 KiB more for what it leaves out (the headers, and a new arena of small blocks for an object made next to it, such as the object of a new `ArrayBuffer`): a block that grows counts only what it adds, and a rope made flat before counts nothing, so that a buffer near the limit that grows by small steps, or a rope searched again, runs no collection. `push` and `a[i] = v` check only when the elements must grow, in a function of their own: a write into room the array has runs the instructions it ran, and the loop of the register interpreter and the frames are the same. On arm64, `rv_putel_slow` takes one more load from the stack on three paths that store into a typed array a value that is not a number, each after the call that converts the value. A `String.prototype` method tests for a flat string first, which it gives back as `JS_ToString` did, so that the test of a rope costs a flat string nothing: a loop of `indexOf` on a flat string runs 4.7% fewer instructions than before. `join` notes its buffer, tests one compare each time the buffer grows, and checks only from 64 Ki characters: a join of four short strings runs about 20 more instructions, 1% of the loop that calls it. Counted with `/usr/bin/time -l` on an Apple M4, 3 runs each, the Go engine. A page that fits under its limit runs the same collections as before, and tifa's pages give the same HTML. When the block still does not fit after the collection, the call ends with out of memory as before, one collection later.

What it gains, measured on an Apple M4 with the native C tree (`engine/test`): the largest block of each built-in that works in a new runtime under a memory limit, to 64 KiB, after a collection and with cyclic garbage made up to just under the threshold. Before, the garbage took that much off the line. Now the line is where it is with no garbage, to 64 KiB:

| Limit, live | Built-in | Before, with the garbage | Now | With no garbage |
| --- | --- | ---: | ---: | ---: |
| 16 MiB, 4 MiB | `repeat`, `ArrayBuffer`, typed arrays, `resize`, `transfer`, `padEnd` | 10.06 MiB | 11.94 MiB | 11.94 MiB |
| 16 MiB, 4 MiB | `push`, `a[i] = v` | 7.44 MiB | 11.19 MiB | 11.19 MiB |
| 64 MiB, 16 MiB | `repeat` and the others above | 39.94 MiB | 47.81 MiB | 47.81 MiB |
| 64 MiB, 25 MiB | `push`, `a[i] = v` | 25.56 MiB | 38.31 MiB | 38.31 MiB |
| 64 MiB, 16 MiB | `join` | 21.50 MiB | 29.69 MiB | 29.75 MiB |
| 128 MiB, 51 MiB | `repeat` and the others above | 51.62 MiB | 76.94 MiB | 76.94 MiB |
| 128 MiB, 32 MiB | `push`, `a[i] = v` | 57.50 MiB | 86.31 MiB | 86.31 MiB |
| 512 MiB, 128 MiB | `repeat` and the others above | 320.12 MiB | 383.88 MiB | 383.88 MiB |

`JSON.stringify` of a large array of objects did not move with the garbage, before or now: it makes an array of the keys of each object, and those arrays start the collection once its buffer has taken the memory past the threshold.

What it leaves as it was: a large block made where a collection is not known to be safe. That is the flat copy of a rope made anywhere else (a property key, a comparison, a template, the Go host reading the result of a call), the buffers of the other built-ins that grow step by step (`replace`, `split`, `JSON.stringify`'s own buffer), the elements of an array that grows inside other built-ins (`concat`, `map`, `Array.from`), the buffer of a `join` that runs inside eight others, objects with many properties, and `Map` and `Set`. Their line can still be lower by up to the garbage left under the threshold, as it was. Many small blocks are left as they were too, such as the strings `sort` makes of its elements when it is given no compare function.

Tests: `test_gc_block_line` of `unit-api` finds the line of twelve built-ins under limits of 16 and 32 MiB, with and without the garbage, and requires the line with the garbage to be within 128 KiB of the one without it. Before the change 23 of its 24 cases failed. `test_gc_block` of `unit-white` counts the collections: with no limit, fifteen blocks of about 4 MiB (among them a join whose elements are arrays, each a join of its own, a pad of one wide character, and `splice`) run none, with the garbage in the way each runs a collection and works, and with a threshold of -1 each runs none and ends with out of memory. What a block needs (the array to copy, the rope, the buffer to resize) is made before the garbage, so that the collection counted is the one of the block. `test_gc_block_fits` runs, 1 MiB under the limit and with no garbage, sixteen `resize` steps of 4 KiB and sixteen searches of a rope made flat before: they run no collection, where each ran one before. `test_gc_block_widen` makes the buffer of a join 16 bits wide where it is, with the garbage in the way: the wide block collects first and the join works.
