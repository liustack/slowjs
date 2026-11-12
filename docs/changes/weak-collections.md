---
summary: 'How full GC collects WeakMap ephemerons without keeping their values through an unreachable key, and how a WeakRef keeps its target to the end of the job'
read_when:
  - WeakMap values survive after their keys become unreachable
  - WeakRef.prototype.deref gives undefined or an object you did not expect, or a host ends a job
---

# Weak collections

English · [简体中文](weak-collections.zh-CN.md)

Release: v0.27.2.

A full collection releases a WeakMap entry when its key has no strong path from a root, even when its value points back to that key. Chains of entries are resolved in the same collection. Both the map and its key must be reachable to keep an entry's value. WeakSet uses the same key cleanup.

Previously, `gc_remove_weak_objects` ran before trial deletion and tested the key's reference count. `js_map_mark` then treated every value as a strong child of its map. A live map retained a value that retained its key, so repeating full GC never broke that cycle. A key held only by a separate cycle required a second collection before its entry disappeared.

## Upstream

Bellard's latest release on 2026-10-22 is [2026-06-04](https://bellard.org/quickjs/Changelog). Its GitHub master at `7d1c13c5d8f8c6b32893786ed35cdcebd88194d2` still marks weak map values from their map.

QuickJS-NG fixed the back-reference case in [a57877c, Retain WeakMap values only when key is reachable](https://github.com/quickjs-ng/quickjs/commit/a57877cef88c3d80aadf69153209b9f0dfc6324f). Release v0.17.0 and master at `270b8c974a4427726e7b580349ae061af08f53a5` mark those values from their object key instead. liustack/quickjs follows that conditional-edge approach. It uses a temporary key index because Bellard's current weak-reference layout has a runtime list rather than per-target records. It also accounts for symbol keys and checks map reachability.

## Collector

Trial deletion still subtracts every owned value. During the live scan, weak map values are withheld. A reachable map marks the value of an entry whose key is already reached, and registers the others under their key in a temporary index. Reaching that key activates its entries and appends newly reachable objects to the collector's existing queue. Each entry is registered and activated at most once. This avoids repeatedly rescanning all entries for a long chain. The scan loop itself is the one of `gc_scan`, and only looks further at weak maps and sets and at objects that are a weak target.

Symbols are reference counted but are not nodes on the GC object list. When a weak collection has symbol keys, a preliminary pass counts references to those symbols from GC-managed fields and shape property names. References left over are roots. The live scan follows symbol-valued fields and symbol property names too. The extra pass is skipped when there are no symbol keys.

Before freeing anything, the collector restores all withheld and garbage-owned references. The entries still registered are exactly those with dead keys. It removes them while recursive object freeing is disabled, then sweeps cycles. A dead key can carry either a trial-deletion mark or a zero strong count on a shell that only weak references keep allocated. The weak pass that runs before trial deletion skips weak maps and sets, so entries with a shell key go through the scan and are removed with the others. Changed maps rebuild their hash buckets once, avoiding repeated searches through a collision chain. Finally it clears dead WeakRefs and queues FinalizationRegistry jobs. Collection does not execute JavaScript. Callbacks still run through pending-job execution, and unregister still suppresses them.

The key index, the list of registered entries, the symbol table and the callbacks live only for the collection, and the index and list are made only when an entry has to wait. They take 56 to 88 bytes per weak map entry on 64-bit targets for that collection. No Go pointer is stored in C memory. The runtime stores one phase byte. A runtime with no WeakMap, WeakSet, WeakRef or FinalizationRegistry takes the original collector path. One whose weak maps and sets are all empty runs the original steps, and clears WeakRefs and queues FinalizationRegistry jobs once more after the sweep. `JS_FreeRuntime` keeps its existing teardown path.

It gives the same result with the first version's steps: it keeps the full weak pass before trial deletion, scans every map's entries for dead keys afterwards and has none of the cost reductions above. It uses temporary hash tables and pending-entry storage in place of Go maps. If C scratch allocation fails before trial deletion, it frees the scratch storage and retains the original collector path. The Go scratch storage is on the Go heap, outside the engine allocator's memory counter, like other Go-side temporary work.

## What it costs

On linux/amd64 without PGO (AMD EPYC 7B13, one core), against main `453def5` (the engine of v0.27.1), over 6 rounds in 3 code layouts, with tifa `b1cb68f` and one VM per page:

- A full GC of a VM with WeakMap entries takes about as long as before on tifa's pages. Collected after every render, the shop pages take 0.989 of the time per GC (0.936 to 1.077) and the MUI pages 1.046 (0.979 to 1.229), on 2.0 to 3.6 ms. Under callgrind those collections run 1.0% more instructions on the shop and 0.3% more on the MUI pages.
- Render time does not change, since the fix touches only the GC. Against v0.27.1, whole renders of v0.27.2 take 1.006 of the time on the shop pages and 0.991 on the MUI pages, inside the spread of code layouts. The HTML is the same byte for byte.
- Large weak maps cost more. `BenchmarkEphemeronGC` collects a chain of 100,000 live entries in 8.2 ms rather than 2.8 (2.9 times, 2.7 to 3.0), the same chain spread over 10,000 maps in 10.5 ms rather than 3.4 (3.1 times), and 100,000 live keys held from an array in 14.4 ms rather than 7.3 (1.9 times). 100,000 entries whose values point back to unreachable keys take 24.7 ms, and are freed: main takes 10.2 ms and frees none of them. 100,000 ordinary cycles with no weak map take 1.02 of the time.
- Most of what is left comes with the conditional edge: the scan reads the header of each key to see whether it was reached, and an entry whose key was not reached yet goes into the index and is found again when the key is reached, two reads at random places in a table of 32 to 64 bytes per entry. Main reads neither. The GC object header has no free bits to keep a waiting entry on the key instead.
- Memory does not grow over 2,000 renders of a page in the default pool, before or after. After one GC, a VM of the shop keeps 30 weak map entries where main keeps up to 165, and up to 0.9 MiB less memory from the allocator. Within 20 more collections main gets down to the same memory. On the MUI pages the VM keeps 1 entry where main keeps up to 69. The memory in use is the same: main holds more allocator memory on two of the MUI pages and less on `/orders`.

## Tests

`gc_ephemeron_test.go` checks one full collection through `VM.RunGC`. A deterministic mixed-graph test compares against an independent reachability oracle.

`BenchmarkEphemeronGC` measures fresh graphs of 10,000 and 100,000 entries, with setup and teardown outside the timer, including reverse-order live chains in one map and spread over 1 map per 10 entries, live keys held from an array, and ordinary cycles without weak collections.

The focused and full test262 results and render measurements must be checked when changing these paths.

## A WeakRef keeps its target to the end of the job

Release: v0.30.2.

The specification keeps the target of a `WeakRef` alive to the end of the job that made the `WeakRef` or called its `deref` (AddToKeptObjects, and ClearKeptObjects when the job ends), so that a script reads the same answer from `deref` for the rest of the job. QuickJS never had that list, and neither have upstream QuickJS and quickjs-ng at their latest commits: reference counting frees a target as soon as its last other reference goes. So `new WeakRef({}).deref()` and `const w = new WeakRef(o); o = null; w.deref()` gave `undefined` in the same job, where Node 24 gives the object. test262 does not see it: its tests of `WeakRef` hold the target in a variable, and none of the 33 known errors is about `WeakRef`.

Now the runtime keeps the targets of a job in a set by address (`rt->kept`), so a target read many times is kept once. The constructor and `deref` add theirs (`js_keep_target`). The set is counted under the memory limit: when it cannot grow, the call fails with the out of memory error. `JS_ClearKeptObjects`, new in `engine/quickjs.h`, frees the targets and the set. Only the host knows where its job ends, so the host calls it:

- The Go API calls it when the outermost call into javascript returns to Go (`leave` in `entry.go`), after `Eval`, `Call`, `ExecutePendingJobs` and every other entry, and before each job that `ExecutePendingJobs` or an `Eval` that awaits runs in the outermost call. It runs inside the entry of the call, under its eval timeout, as the conversion of the result does. A Go function that javascript calls, or that `Call` runs to convert an argument, is inside the job of the call, so the targets stay there for it and for the javascript it evaluates, also when it runs the jobs: those jobs run inside the job of the call.
- `JS_ExecutePendingJob` does not call it, since it cannot tell whether javascript of the host's job is still running below it. The test hosts of `engine/test` (`jsrun`, `render`, `run-test262`, the unit programs) call it after the script and before each job, as the Go API does. `run-test262` also gives `$262.clearKeptObjects`.
- `JS_FreeRuntime` calls it first.

So a VM that a pool uses for many renders keeps no target from one call to the next, and nothing is kept when a call ends with an error. A VM that a kill left good only for Close lets them go at Close. A target the host still holds, through a `Value`, stays as any other object does. One thing a script can no longer do: wait within one call, in a loop, for `deref` to give `undefined`. Such a loop does not end, since the target is kept to the end of the call whatever collections run. The target is let go when the call returns to Go or the job ends, so a program that waits for it asks for a collection between two calls (`RunGC`) and reads `deref` in the next one.

A program that read `deref` in a later job sees the same as before. A test of the engine that made a `WeakRef` and dropped its target without the job ending now ends the job first (`test_free_order` in `engine/test/unit.c`, `test_young_leaves_no_zero_count` in `engine/test/unit-white.c`).

### What it costs

Under callgrind on linux/amd64, against main `32307c7e`, both built with `-pgo=off`, counting 10 renders after 2: a render of tifa's bench page runs 265,495,178 instructions where main runs 265,470,587 (+0.01%), 263,961,491 of them in the engine where main runs 263,960,294. A render of the React page runs 51,089,722 where main runs 51,091,721 (0.00%), 50,726,736 of them in the engine where main runs 50,730,514. With no `WeakRef` made, the end of a call or a job is one call into the engine that finds no set. The HTML of the shop (20 pages), MUI (3 pages) and bench (2 pages) is the same byte for byte. While a job runs, the set takes 32 to 64 bytes for each target on 64-bit targets (16 a slot, at most half full), and the end of the job gives it all back.

### Tests

- `TestWeakRefKeptDuringJob` (`weakref_test.go`): the target of a new `WeakRef` and of `deref` is there in its call, in a Go function the call runs, in the javascript that function evaluates and after Go code ran the jobs inside the call (a Go function, or the `MarshalJSON` of an argument of `Call`), and gone in the next call. Each promise job of `ExecutePendingJobs` is a job of its own. `TestWeakRefKeptReleasedAfterTimeout`: a call the eval timeout stopped lets its targets go too.
- `TestWeakRefKeptReleasedAfterCall`: 200 calls on one VM, each making a `WeakRef` to an array of 65,536 numbers, leave no more memory after a GC than the first call did. `TestWeakRefKeptThroughLoop`: a loop in one call that reads `deref` of a dead cycle while it allocates 300,000 objects still reads the target at its end, and after the call and a `RunGC` the next call reads `undefined`.
- `test_weakref_kept` (`engine/test/unit-api.c`) checks the same in the C tree, with a host that ends the job as the Go API does, a symbol target, a function of the host that runs the jobs inside javascript, a full and a young collection in the job, 10,000 targets read 100 times each in one job, and a runtime freed while it keeps targets. `test_weakref_kept_oom` (`engine/test/unit-oom.c`) makes and reads 200 `WeakRef`s, or reads 200 made in an earlier job, with every room up to 64 KiB: each run gives the sum or the out of memory error and leaves no block behind.
- `test_weak_ref` of the upstream `test_builtin.js` (`engine/test/compare/upstream/`) reads the target after it dropped it, ends the job with `clearKeptObjects()` of `jsrun`, then reads `undefined`. `gc_ephemeron.js` and `young_gc.js` end the job before each collection they ask for.
