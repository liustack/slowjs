---
summary: 'Package temporal, Temporal for a VM on go-intl: what it covers, the clock and zone of Temporal.Now, toLocaleString through package intl, the zone data it carries (tz 2026e) and how a program links only that, its test262 and Chromium results, and what it costs in size, memory and time'
read_when:
  - Changing package temporal, temporal.js, stub.js or internal/intlhook
  - Asking what Temporal costs in program size, VM memory and time
  - A page shows another Temporal value on the server than in the browser, or toString and toLocaleString disagree
  - Moving Temporal's zones to a new tz release, or changing the tz pack of internal/gointl
  - Giving Temporal.Now a clock (temporal.Clock), or asking why it does not change Date.now
---

# Temporal

QuickJS has no Temporal. Package `temporal` adds it for a VM that calls `temporal.Register(vm)`, the way package `intl` adds `Intl`. Nothing of upstream's code changes: `fixlib` does not touch it.

## Temporal on go-intl

Release: not released yet.

`temporal.Register(vm)` installs `Temporal.Now`, `Instant`, `ZonedDateTime`, `PlainDate`, `PlainTime`, `PlainDateTime`, `PlainYearMonth`, `PlainMonthDay` and `Duration` with every method V8 ships, and `Date.prototype.toTemporalInstant`. go-intl's temporal package (`internal/gointl/temporal`, a Go port of temporal_rs, the library V8 builds Temporal on) does the computing. `temporal.js` reads arguments, options and property bags in the order V8 reads them, with V8's conversions and error messages, and keeps each object's internal slots in a private field no page can reach.

It is installed as `Intl` is (see [time-and-intl.md](time-and-intl.md#intl-calls-go-through-builtins)). `Register` puts stand-ins for `Temporal` and `toTemporalInstant` on the VM and takes the built-ins Temporal's code uses, before any page runs. The first use installs the rest, from bytecode compiled once per process, through one `Builtin` whose first argument names the operation. The first use replaces a stand-in only while it is still there and can be replaced: what a page put in its place, a property it deleted, or a stand-in frozen with its object stays, as it would with V8's built-ins, and a stand-in left in place forwards to the real thing. A page that changes built-ins after `Register` does not change what Temporal does.

### Clock and zone

`Temporal.Now` reads the clock `temporal.Clock` sets, `time.Now` without it. Only `Temporal.Now` reads it: `Date.now`, `new Date()` and `Intl` keep the host's clock. A clock outside an `Instant`'s range, 1e8 days either side of 1970, makes `Temporal.Now` throw a `RangeError`. The clock must not block: the VM waits for it, and the eval timeout cannot cut it short, since the engine checks for one only between its own instructions. A panic in the clock reaches the caller of `Eval`, and the VM can be used after it. `Close` drops the VM's clock (`VM.OnClose`), so a clock that references its VM does not keep it.

`Temporal.Now.timeZoneId()` is the default zone package `intl` formats in, `intl.TimeZone`, or `UTC` without it, as ECMA-402's DefaultTimeZone is, and the `Now` methods called without a zone use it. The engine's `Date` does not follow it: its local time is UTC in every VM (see [time-and-intl.md](time-and-intl.md#local-time-is-utc-in-every-vm)), so with `intl.TimeZone("Asia/Tokyo")`, `new Date().getHours()` and `Temporal.Now.plainTimeISO().hour` are 9 hours apart.

### toLocaleString

`toLocaleString` is package `intl`'s `Intl.DateTimeFormat`, through `internal/intlhook`, with the options and defaults ECMA-402 gives each type, and works whether or not the VM has `intl` registered. Package `intl` formats in the Gregorian calendar only, so a value in another calendar throws a `RangeError` where Chromium formats it, and so does a `PlainYearMonth` or `PlainMonthDay` in the ISO calendar, as in Chromium without a calendar option. `Duration.prototype.toLocaleString` returns what `toString` does: it does not go through `Intl.DurationFormat` yet.

A `ZonedDateTime` is shown at the wall clock and offset Temporal computed, so `toString` and `toLocaleString` of a value always agree. Package `intl` resolves zones with Go's zone data, the host's zoneinfo or `time/tzdata`, which can have another offset for the zone at that time: it does before 1970 in many zones, where ICU and IANA's backzone file differ, and wherever the two are at different tz releases. The zone is then named by its offset, `GMT-05:00` say, since its names belong to the offsets Go's data has. A named zone still needs a zoneinfo Go can read.

### Zone data

The zones and calendar tables are go-intl's, built into the binary: ICU's `zoneinfo64` from ICU's time zone update for tz 2026e, and the tables of the Chinese, Korean and Umm al-Qura calendars. They are the 92 KB pack `internal/gointl/qjsdata/tz.bin`, which Temporal reads through `gointl.EmbeddedTemporalSource`. `gointl.Embedded`, which package `intl` reads, serves the same pack with the rest, and a chunk read through either is inflated once. Since Temporal imports `intl` for `toLocaleString`, a program that uses it links the packs `intl` links as well (`TestLinksOnlyReadData` in `temporal/link_test.go`). The host's zoneinfo is never read, so Temporal gives the same answers on every machine.

go-intl v0.6.1 carries tz 2026c. The packs take their zones from the update `internal/gointl/qjstz` names instead, made by go-intl's own `tzgen` (see [upgrading.md](../upgrading.md#updating-go-intl-internalgointl)): since 2026c, 2026d keeps the Northwest Territories on -06 from 2026-08-21 and 2026e keeps Manitoba on -05 from 2026-10-31. `tz.bin` holds `tzversion.bin`, the release of its zones, which `TestQJSTZVersion` checks.

### Results

test262 (commit 2e0a5676) passes 9206 of the 9210 runs of `built-ins/Temporal`, all 16 of `Date/prototype/toTemporalInstant`, and 3972 of the 4058 runs of `intl402/Temporal`. The 4 failures of `built-ins/Temporal` are `toLocaleString` of a `PlainYearMonth` and a `PlainMonthDay` in the ISO calendar, which fail in Chromium 156 too. The others come from formatting: `Intl.DateTimeFormat` does not take Temporal values yet, package `intl` formats dates in the Gregorian calendar only, `Duration.prototype.toLocaleString` does not go through `Intl.DurationFormat` yet, and package `intl` names the zone `+00:00` `UTC` where V8 says `GMT`. 723 expressions give what Chromium 156 gives, errors with their messages, but 8 whose formatting differs (`temporal/testdata/corpus.js`, `differFromChromium` in `temporal/chromium_test.go`).

What it costs, on an AMD EPYC 7B13 core, linux/amd64, without PGO: the example shop of tifa, built with `-ldflags=-w` as `tifa build` does, grows by 0.84 MB (27.09 MB to 27.93 MB), all of it code: the zones and calendars are `tz.bin`, which package `intl` links already. `Register` takes 0.06 ms and no measurable memory. The first use in a process takes 13.4 ms, 8.7 ms of it to compile the javascript and most of the rest to inflate the zones and calendars, 236 KB of heap kept for the process. The first use in a later VM takes 1.1 ms and 358 KB. `Temporal.PlainDate.from('2024-01-05')` takes 3.5 µs, `add({months: 1})` 14 µs, `until` with `largestUnit: 'years'` 11 µs, `ZonedDateTime.from` 7.3 µs, its `add({days: 1})` 17 µs, `toString` 1.4 µs for a date and 5.5 µs for a zoned date-time, and `toLocaleString` 3.8 µs for a date and 4.9 µs for a zoned date-time.

Tests: `temporal/all_test.go`, `temporal/shape_test.go`, `temporal/tamper_test.go`, `temporal/chromium_test.go`, `temporal/concurrent_test.go`, `temporal/memgrind_test.go`, `temporal/link_test.go`, `intl/temporal_test.go`, `internal/gointl/qjs_source_test.go`. The checks to run after a change are in [testing.md](../testing.md#before-committing).
