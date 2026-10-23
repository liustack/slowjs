---
summary: 'The young collection, JS_RunYoungGC and VM.RunYoungGC: a cycle collection over the GC objects made since the last collection only, what it keeps that a full collection frees, how it treats weak maps, what it gains on tifa pages and what to do when fixlib stops on it'
read_when:
  - Changing lib/younggc.go, fixYoungGC in internal/fixlib, or the young hunks of the C patch
  - Using VM.RunYoungGC, or deciding when to run a full GC next to it
  - fixlib reports a young GC fingerprint mismatch or a new reader of gc_obj_list
---

# Young collection

Release: Unreleased.

`JS_RunYoungGC` in `lib/` and `VM.RunYoungGC` collect the reference cycles among the GC objects made since the last collection, young or full, and make the objects they keep old. They visit only those objects. A full collection, `JS_RunGC`, visits every object twice, and on a VM of tifa's example shop that is about 30,000 objects of the bundle before any garbage. An embedder that runs one unit of work at a time, such as a page render, can run a young collection after each and a full one now and then, and spend on each collection about as long as the garbage of that unit.

## How it works

`add_gc_object` puts a new GC object on a young list of the runtime instead of `gc_obj_list`. `JS_RunYoungGC` runs the trial deletion of `JS_RunGC` over the young list only: it takes away the references young objects hold to each other, so what is left of a young object's count comes from outside the young list (old objects, the stack, Go, values that are not GC objects). Those objects and what they reach stay, and the rest are cycles of garbage, which `gc_free_cycles` frees as in `JS_RunGC`. The survivors move to the end of `gc_obj_list`. An old object is never visited, so its count does not change during the trial deletion, and no write barrier is needed.

`JS_RunGC`, the GC the engine runs by itself at its threshold, `JS_FreeRuntime`, `JS_ComputeMemoryUsage` and `JS_DumpMemoryUsage` first move the young list to the end of `gc_obj_list`, so they see every object in the order it was made, as before. A program that never calls `JS_RunYoungGC` collects exactly as before.

Young objects are told from old ones by the mark bit of their header (`ephemeronMarkMask`), which is 0 on every GC object outside a collection. The young collection sets it on the young list, clears it on the survivors before it frees anything, and the garbage keeps it, as in `JS_RunGC`. On s390x the bit fields of the header are in the reverse order, and the mask is the one `lib/ephemeronbits_s390x.go` takes from that target's generated collector.

An old object that only the garbage held reaches a count of 0 while the garbage is freed. `JS_RunGC` leaves such an object where it is, since every such object is garbage there. In a young collection it may be an old object outside the garbage: left on `gc_obj_list` with a count of 0, `js_weakref_free` would free it while it is still listed, when the weak pass removes a weak map entry whose key it is. `__JS_FreeValueRT` and `async_func_free` therefore call `youngDefer` in that case, which moves the object to a list of its own, and the young collection frees it after the garbage, as the engine frees an object outside a collection.

## Weak references

The young collection runs with the ephemeron phase of the [full collection](weak-collections.md) off, so a weak map value is a strong child of its map. An entry of a reachable or old weak map keeps its value, and through it a key the value points back to, until the next full collection, which judges it as before. A key that nothing else holds is garbage of the young collection. After the garbage is freed, the young collection runs `gc_remove_weak_objects`, which removes the weak map entries, clears the WeakRefs and queues the FinalizationRegistry jobs of the objects that are gone, and frees the shells weak references kept of them. It runs no javascript.

So what a full collection frees, it still frees after any number of young collections, and a young collection frees nothing a full one would keep. What young collections keep that a full one would free: a cycle that runs through an old object, and weak map values with what they hold. An embedder runs a full collection from time to time for those, such as when `MemorySize` after a young collection has grown well past what it was after the last full one.

The study prototype ran the weak pass before the trial deletion, as `JS_RunGC` did before the ephemeron fix, and left old objects with a count of 0 on `gc_obj_list`. With an old object as the key of a weak map entry whose map the young garbage held, `js_weakref_free` freed that object while it was still listed, and a later full collection read freed memory. Of the tests below, `TestYoungGCLeavesNoZeroCount` and `TestYoungGCWeakObservers` in `lib/`, and `TestYoungGCFreesWhatOnlyGarbageHeld` and the key cycle of `TestYoungGCEphemeron` in the root package, fail on that form.

## What it gains and costs

Measured in tifa as [Measuring in tifa](../performance.md#measuring-in-tifa) describes, without PGO, on an AMD EPYC 7B13: tifa's pool at the time, which runs a full collection between renders once a VM holds 1.5 times what the last one left, against the same pool running a young collection after each render and a full one when a young one leaves 1.5 times the live size, after a render in which the engine collected, and every 256 renders. One VM on one thread, or two VMs on the two threads of one core, 6 rounds in turn over 3 code layouts. Each round renders every page 256 times to warm up and 300 times measured. The HTML is the same byte for byte.

| Pages, VMs | P99 of a render, sum over the pages | P50, sum over the pages | CPU | Peak RSS |
| --- | --- | --- | --- | --- |
| The shop's 10 pages, 1 VM, pages in a new random order each pass | 0.748 (0.714 to 0.804), 386.6 ms against 529.5 | 1.010 | 0.994 | 0.872 |
| The shop, 1 VM, the same order each pass | 0.994 (0.972 to 1.003) | 0.999 | 1.015 | 0.874 |
| The shop, 2 VMs, the same order | 0.796 (0.775 to 0.824), 615.5 ms against 776.7 | 1.021 | 0.996 | 0.827 |
| The 4 MUI pages of tifa's test app, 1 VM, random order | 0.940 (0.920 to 1.029) | 0.997 | 0.993 | 0.883 |
| MUI, 1 VM, the same order | 0.975 (0.953 to 0.992) | 1.005 | 1.001 | 0.874 |
| MUI, 2 VMs, the same order | 0.919 (0.916 to 0.945) | 1.005 | 1.000 | 0.824 |

The full collection between renders takes 6 to 11 ms on these pages, and the render that waits for it carries that time. With the pages in the same order each pass and one VM, it landed on the same page each time, `product-en` on the shop, so the P99 of the other pages did not see it and the sum moved little: `product-en` went to 0.719 of its P99, and `weekend-stats` to 1.48 (8.9 to 13.1 ms). In random order and with two VMs, it falls on every page, as on a server, and the P99 of the shop's small pages fell from 28 to 30 ms to 13 to 15 ms.

A VM of the shop holds 14.2 MB after a collection against 15.5, and the MUI pages 6.6 MB against 8.5: the full collection left what the last render held, a young one after the render does not. The P50 of the shop's small pages, 8 to 11 ms, goes up by 4% to 5%, about 0.5 ms, with one VM, the larger pages by under 1.5%. One round of the shop takes 2252 young collections and 8 full ones, where it took 113 to 131 full ones. Total CPU stays the same within 1.5%. Running a full collection every 64 renders raised the P99 of the shop by 1.5% to 4%, and every 1024 gave the same P99 and memory as 256.

## Tests

`lib/younggc_test.go` checks the C API: the young list and when it moves (`TestYoungGCLists`), that no listed object is left with a count of 0 or a mark after a young collection with weak maps, WeakRefs, registries, async functions and generators around (`TestYoungGCLeavesNoZeroCount`), every case of the ephemeron tests with young collections in between (`TestYoungGCEphemeron`), random graphs against an independent reachability oracle (`TestYoungGCMixedGraph`), and WeakRef, FinalizationRegistry and a key whose value points back to it, kept until the full collection (`TestYoungGCWeakObservers`). `younggc_test.go` in the root package checks the Go API: 400 rounds of random graphs with closures, Maps, Sets, arrays, old objects, an old closure variable and an old WeakMap keep what the roots reach after each young and each seventh full collection, and in the end a full collection frees as much as without young collections (`TestYoungGCKeepsReachable`). A young collection frees young cycles as the full one does and keeps a suspended generator whose frame holds a cycle (`TestYoungGCFreesCycles`), frees an old object that only young garbage held, with its weak map entry, WeakRef or registration (`TestYoungGCFreesWhatOnlyGarbageHeld`), and leaves every case of the ephemeron tests to the next full collection unless the key is garbage on its own (`TestYoungGCEphemeron`). The `TestMemgrind2YoungGC` tests of both packages run them with `-tags=libc.memgrind`.

`TestYoungGCDefersSuspendedAsyncFrame` creates an async call suspended on an unreachable promise and exports a factory for closures over its local payload. A full GC makes the frame and its attached var-ref old. A new closure, held only by a young self-cycle, becomes the var-ref's only owner. Freeing that closure releases the old var-ref during the young sweep, so `async_func_free` must defer the old frame. The test first checks that the old frame is suspended with one reference, then checks that it leaves the GC lists and its payload's WeakRef clears. On darwin/arm64, removing only the `youngDefer` call in `async_func_free` through a Go overlay makes this test fail with a zero-count object left on the old list. Removing only the call in `__JS_FreeValueRT` makes all six cases of the existing `TestYoungGCLeavesNoZeroCount` fail with the same invariant violation. The generated files themselves are unchanged by these mutation checks.

`internal/fixlib/younggc_test.go` checks on every target that the rewrite gives the committed file from what `restoreYoungGC` gives, that it is idempotent and exactly undone, and that it stops on a changed collector, a partial rewrite, an edited hook and a new function that reads `gc_obj_list`. It tests runtime parameter names `rt`, `rt2` and `runtime` on all 13 generated files.

The reader audit parses offset additions and `unsafe.Add`, and also checks `Fgc_obj_list` selectors. It does not depend on the name of the base. Unknown `uintptr` origins require review. It excludes a TLS allocation only when every assignment of the base comes from TLS storage and its address does not escape. A pointer parameter to a struct declared without `Fgc_obj_list`, including its unchanged aliases, also proves a different origin. Addresses of global arrays of those structs are excluded too, including the class table initialized at the same offset. Each `init` declaration is checked separately. Merely casting a `uintptr` to another struct at a read does not prove that it is not a runtime. `TestYoungReadersOrigins` checks these exclusions, aliases, reassignment and address escape. This keeps the interpreter's TLS slots at the same numeric offset out of the reader list without exempting the interpreter function itself.

```sh
go test -p 2 -vet=off -run 'YoungGC|Ephemeron' . ./lib
go test -p 2 -vet=off -tags=libc.memgrind -run 'YoungGC|Ephemeron' . ./lib
go test -p 2 -timeout 120m -run 'YoungGC|Ephemeron' ./internal/fixlib
```

## C form

`lib/internal/patch/quickjs.diff` has the same change in C: `young_obj_list`, `young_dead_list` and `young_sweep` in `JSRuntime`, `add_gc_object` on the young list, `young_splice` at the start of `JS_RunGCInternal`, `JS_ComputeMemoryUsage` and `JS_DumpMemoryUsage`, `young_defer` in `__JS_FreeValueRT` and `async_func_free`, and `JS_RunYoungGC` with its callbacks, declared in `quickjs.h`. The patched C passes the same random graphs, cycles, old objects freed through the young garbage and ephemeron cases natively and with AddressSanitizer.

## When upstream changes it

`internal/fixlib/younggcsites.go` fingerprints, on each target and after the earlier final changes, the functions `lib/younggc.go` repeats or relies on: the collector and its callbacks, `mark_children`, the weak pass, and the functions that free an object at a count of 0. `youngListReaders` lists every function that uses `gc_obj_list`: each belongs to the collector, sees the young list moved in first, or is never called. When `fixlib` stops on either, read the changed functions against `lib/younggc.go` before updating the list with `go run ./internal/fixlib -young`. A new reader of `gc_obj_list` that runs outside a full collection needs the young list moved in first, as `JS_ComputeMemoryUsage` has.

`fixYoungGC` is the last final change, made after `fixEphemeron` and undone before it. After a regeneration from a C source with the young hunks of the patch, drop `lib/younggc.go` and `fixYoungGC` only once the tests above pass on the generated code.
