---
summary: 'How full GC collects WeakMap ephemerons, including back references and symbol keys, without keeping their values through an unreachable key'
read_when:
  - Changing lib/ephemeron.go or the collector and weak-reference hooks in internal/fixlib
  - WeakMap values survive after their keys become unreachable
  - fixlib reports an ephemeron fingerprint mismatch
---

# Weak collections

Release: Unreleased.

A full collection releases a WeakMap entry when its key has no strong path from a root, even when its value points back to that key. Chains of entries are resolved in the same collection. Both the map and its key must be reachable to keep an entry's value. WeakSet uses the same key cleanup.

Previously, `gc_remove_weak_objects` ran before trial deletion and tested the key's reference count. `js_map_mark` then treated every value as a strong child of its map. A live map retained a value that retained its key, so repeating full GC never broke that cycle. A key held only by a separate cycle required a second collection before its entry disappeared.

## Upstream

Bellard's latest release on 2026-10-22 is [2026-06-04](https://bellard.org/quickjs/Changelog). Its GitHub master at `7d1c13c5d8f8c6b32893786ed35cdcebd88194d2` still marks weak map values from their map.

QuickJS-NG fixed the back-reference case in [a57877c, Retain WeakMap values only when key is reachable](https://github.com/quickjs-ng/quickjs/commit/a57877cef88c3d80aadf69153209b9f0dfc6324f). Release v0.17.0 and master at `270b8c974a4427726e7b580349ae061af08f53a5` mark those values from their object key instead. This fork follows that conditional-edge approach. It uses a temporary key index because Bellard's current weak-reference layout has a runtime list rather than per-target records. It also accounts for symbol keys and checks map reachability.

## Collector

Trial deletion still subtracts every owned value. During the live scan, weak map values are withheld. A reachable map registers each withheld entry under its key. Reaching that key activates its entries and appends newly reachable objects to the collector's existing queue. Each entry is registered and activated at most once. This avoids repeatedly rescanning all entries for a long chain.

Symbols are reference counted but are not nodes on the GC object list. When a weak collection has symbol keys, a preliminary pass counts references to those symbols from GC-managed fields and shape property names. References left over are roots. The live scan follows symbol-valued fields and symbol property names too. The extra pass is skipped when there are no symbol keys.

Before freeing anything, the collector restores all withheld and garbage-owned references. It removes entries with dead keys while recursive object freeing is disabled, then sweeps cycles. A dead key can carry either a trial-deletion mark or a zero strong count on a previously freed shell. The latter occurs when the preliminary weak pass releases a key after visiting another map that uses it. Both cases must be removed before sweeping their values. Changed maps rebuild their hash buckets once, avoiding repeated searches through a collision chain. Finally it clears dead WeakRefs and queues FinalizationRegistry jobs. Collection does not execute JavaScript. Callbacks still run through pending-job execution, and unregister still suppresses them.

The temporary Go maps, lists and callbacks live only for the collection. No Go pointer is stored in C memory. The runtime stores one phase byte. Callbacks passed as ccgo function pointers are held in pinned heap cells and kept alive across all uses, including goroutine stack growth. A runtime with no WeakMap, WeakSet, WeakRef or FinalizationRegistry takes the original collector path. One whose weak maps and sets are all empty runs the original steps, and clears WeakRefs and queues FinalizationRegistry jobs once more after the sweep. `JS_FreeRuntime` keeps its existing teardown path.

The C form is in `lib/internal/patch/quickjs.diff`. It uses temporary hash tables and pending-entry storage in place of Go maps. If C scratch allocation fails before trial deletion, it frees the scratch storage and retains the original collector path. The Go scratch storage is on the Go heap, outside the engine allocator's memory counter, like other Go-side temporary work.

## What it costs

On linux/amd64 without PGO (AMD EPYC 7B13, one core), against main `620658c` (the engine of v0.27.0), over 6 rounds in 3 code layouts, with tifa `b1cb68f` and one VM per page:

- Renders take a little less time: 0.990 of the time on the six shop pages (0.975 to 0.994) and 0.977 on the three pages of tifa's MUI app (0.975 to 0.981), in tifa's default pool. The cause was not traced. The HTML is the same byte for byte.
- A full GC of a VM with WeakMap entries takes longer. Collected after every render, the shop pages take 1.16 of the time per GC (1.08 to 1.19), about 0.4 ms more on 2.2 to 2.6 ms, and the MUI pages 1.11 (0.94 to 1.14). In the default pool these collections run between renders: 44 to 201 per 2,000 renders, under 0.2% of the render time of the shop.
- Large weak maps cost the most. `BenchmarkEphemeronGC` collects a chain of 100,000 live entries in 25.4 ms rather than 2.8 (9.1 times, 9.0 to 9.8), and 10,000 in 2.1 ms rather than 0.29. 100,000 entries whose values point back to unreachable keys take 42.8 ms, and are freed: main takes 8.7 ms and frees none of them. 100,000 ordinary cycles with no weak map take the same time (1.001).
- Memory does not grow over 2,000 renders of a page in the default pool, before or after. After one GC, a VM of the shop keeps 30 weak map entries where main keeps up to 165, and up to 0.9 MiB less memory from the allocator. Within 20 more collections main gets down to the same memory. On the MUI pages the VM keeps 1 entry where main keeps up to 75, and main still holds 0.2 to 0.4 MiB more after 20 more collections.

## Tests

`gc_ephemeron_test.go` checks one full collection through `VM.RunGC`. `lib/ephemeron_test.go` checks the C API, live and dead keys, chains, WeakSet, object and symbol cycles, symbols held through property names and containers, dead maps, mutation after collection, WeakRef, deferred finalization and unregister. A deterministic mixed-graph test compares against an independent reachability oracle.

The `TestMemgrind2Ephemeron` tests in both packages audit those paths with `-tags=libc.memgrind`. `BenchmarkEphemeronGC` measures fresh graphs of 10,000 and 100,000 entries, with setup and teardown outside the timer, including reverse-order live chains and ordinary cycles without weak collections.

`internal/fixlib/ephemeron_test.go` checks every target's rewrite, exact inverse, idempotence, altered collector rejection and partial-hook rejection. It also compares the header masks in `ephemeronbits*.go` against each generated collector, including the reversed bitfields on s390x. The focused and full test262 results and render measurements must be checked when changing these paths.

## When upstream changes it

`internal/fixlib/ephemeronsites.go` fingerprints the collector, value marking, weak record release and key liveness functions on each target. Read the changed ownership and mark transitions before updating a fingerprint with `go run ./internal/fixlib -ephemeron`. `fixEphemeron` is a final rewrite and its exact inverse runs before earlier rules, so unrelated rules still see their original input.

After regenerating from a C source containing the ephemeron patch, remove the corresponding Go helper and hooks only after verifying the full behavioral matrix and all target builds. Do not retain both collectors. Update symbol accounting if upstream introduces another GC-managed location that owns a symbol atom.
