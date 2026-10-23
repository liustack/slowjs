---
summary: 'How full GC collects WeakMap ephemerons, including back references and symbol keys, without keeping their values through an unreachable key'
read_when:
  - Changing lib/ephemeron.go or the collector and weak-reference hooks in internal/fixlib
  - WeakMap values survive after their keys become unreachable
  - fixlib reports an ephemeron fingerprint mismatch
---

# Weak collections

Release: v0.27.2.

A full collection releases a WeakMap entry when its key has no strong path from a root, even when its value points back to that key. Chains of entries are resolved in the same collection. Both the map and its key must be reachable to keep an entry's value. WeakSet uses the same key cleanup.

Previously, `gc_remove_weak_objects` ran before trial deletion and tested the key's reference count. `js_map_mark` then treated every value as a strong child of its map. A live map retained a value that retained its key, so repeating full GC never broke that cycle. A key held only by a separate cycle required a second collection before its entry disappeared.

## Upstream

Bellard's latest release on 2026-10-22 is [2026-06-04](https://bellard.org/quickjs/Changelog). Its GitHub master at `7d1c13c5d8f8c6b32893786ed35cdcebd88194d2` still marks weak map values from their map.

QuickJS-NG fixed the back-reference case in [a57877c, Retain WeakMap values only when key is reachable](https://github.com/quickjs-ng/quickjs/commit/a57877cef88c3d80aadf69153209b9f0dfc6324f). Release v0.17.0 and master at `270b8c974a4427726e7b580349ae061af08f53a5` mark those values from their object key instead. This fork follows that conditional-edge approach. It uses a temporary key index because Bellard's current weak-reference layout has a runtime list rather than per-target records. It also accounts for symbol keys and checks map reachability.

## Collector

Trial deletion still subtracts every owned value. During the live scan, weak map values are withheld. A reachable map marks the value of an entry whose key is already reached, and registers the others under their key in a temporary index. Reaching that key activates its entries and appends newly reachable objects to the collector's existing queue. Each entry is registered and activated at most once. This avoids repeatedly rescanning all entries for a long chain. The scan loop itself is the one of `gc_scan`, and only looks further at weak maps and sets and at objects that are a weak target.

Symbols are reference counted but are not nodes on the GC object list. When a weak collection has symbol keys, a preliminary pass counts references to those symbols from GC-managed fields and shape property names. References left over are roots. The live scan follows symbol-valued fields and symbol property names too. The extra pass is skipped when there are no symbol keys.

Before freeing anything, the collector restores all withheld and garbage-owned references. The entries still registered are exactly those with dead keys. It removes them while recursive object freeing is disabled, then sweeps cycles. A dead key can carry either a trial-deletion mark or a zero strong count on a shell that only weak references keep allocated. The weak pass that runs before trial deletion skips weak maps and sets, so entries with a shell key go through the scan and are removed with the others. Changed maps rebuild their hash buckets once, avoiding repeated searches through a collision chain. Finally it clears dead WeakRefs and queues FinalizationRegistry jobs. Collection does not execute JavaScript. Callbacks still run through pending-job execution, and unregister still suppresses them.

The key index, the list of registered entries, the symbol table and the callbacks live only for the collection, and the index and list are made only when an entry has to wait. They take 56 to 88 bytes per weak map entry on 64-bit targets for that collection. No Go pointer is stored in C memory. The runtime stores one phase byte. Callbacks passed as ccgo function pointers are held in pinned heap cells and kept alive across all uses, including goroutine stack growth. A runtime with no WeakMap, WeakSet, WeakRef or FinalizationRegistry takes the original collector path. One whose weak maps and sets are all empty runs the original steps, and clears WeakRefs and queues FinalizationRegistry jobs once more after the sweep. `JS_FreeRuntime` keeps its existing teardown path.

The C form is in `lib/internal/patch/quickjs.diff`. It gives the same result with the first version's steps: it keeps the full weak pass before trial deletion, scans every map's entries for dead keys afterwards and has none of the cost reductions above. It uses temporary hash tables and pending-entry storage in place of Go maps. If C scratch allocation fails before trial deletion, it frees the scratch storage and retains the original collector path. The Go scratch storage is on the Go heap, outside the engine allocator's memory counter, like other Go-side temporary work.

## What it costs

On linux/amd64 without PGO (AMD EPYC 7B13, one core), against main `453def5` (the engine of v0.27.1), over 6 rounds in 3 code layouts, with tifa `b1cb68f` and one VM per page:

- A full GC of a VM with WeakMap entries takes about as long as before on tifa's pages. Collected after every render, the shop pages take 0.989 of the time per GC (0.936 to 1.077) and the MUI pages 1.046 (0.979 to 1.229), on 2.0 to 3.6 ms. Under callgrind those collections run 1.0% more instructions on the shop and 0.3% more on the MUI pages.
- Render time does not change, since the fix touches only the GC. Against v0.27.1, whole renders of v0.27.2 take 1.006 of the time on the shop pages and 0.991 on the MUI pages, inside the spread of code layouts. The HTML is the same byte for byte.
- Large weak maps cost more. `BenchmarkEphemeronGC` collects a chain of 100,000 live entries in 8.2 ms rather than 2.8 (2.9 times, 2.7 to 3.0), the same chain spread over 10,000 maps in 10.5 ms rather than 3.4 (3.1 times), and 100,000 live keys held from an array in 14.4 ms rather than 7.3 (1.9 times). 100,000 entries whose values point back to unreachable keys take 24.7 ms, and are freed: main takes 10.2 ms and frees none of them. 100,000 ordinary cycles with no weak map take 1.02 of the time.
- Most of what is left comes with the conditional edge: the scan reads the header of each key to see whether it was reached, and an entry whose key was not reached yet goes into the index and is found again when the key is reached, two reads at random places in a table of 32 to 64 bytes per entry. Main reads neither. The GC object header has no free bits to keep a waiting entry on the key instead.
- Memory does not grow over 2,000 renders of a page in the default pool, before or after. After one GC, a VM of the shop keeps 30 weak map entries where main keeps up to 165, and up to 0.9 MiB less memory from the allocator. Within 20 more collections main gets down to the same memory. On the MUI pages the VM keeps 1 entry where main keeps up to 69. The memory in use is the same: main holds more allocator memory on two of the MUI pages and less on `/orders`.

## Tests

`gc_ephemeron_test.go` checks one full collection through `VM.RunGC`. `lib/ephemeron_test.go` checks the C API, live and dead keys, chains, WeakSet, object and symbol cycles, symbols held through property names and containers, dead maps, mutation after collection, WeakRef, deferred finalization and unregister. A deterministic mixed-graph test compares against an independent reachability oracle.

The `TestMemgrind2Ephemeron` tests in both packages audit those paths with `-tags=libc.memgrind`. `BenchmarkEphemeronGC` measures fresh graphs of 10,000 and 100,000 entries, with setup and teardown outside the timer, including reverse-order live chains in one map and spread over 1 map per 10 entries, live keys held from an array, and ordinary cycles without weak collections.

`internal/fixlib/ephemeron_test.go` checks every target's rewrite, exact inverse, idempotence, altered collector rejection and partial-hook rejection. It also compares the header masks in `ephemeronbits.go` against each generated collector. The focused and full test262 results and render measurements must be checked when changing these paths.

## When upstream changes it

`internal/fixlib/ephemeronsites.go` fingerprints the collector, value marking, weak record release and key liveness functions on each target. Read the changed ownership and mark transitions before updating a fingerprint with `go run ./internal/fixlib -ephemeron`. `fixEphemeron` is a final rewrite and its exact inverse runs before earlier rules, so unrelated rules still see their original input.

After regenerating from a C source containing the ephemeron patch, remove the corresponding Go helper and hooks only after verifying the full behavioral matrix and all target builds. Do not retain both collectors. Update symbol accounting if upstream introduces another GC-managed location that owns a symbol atom.
