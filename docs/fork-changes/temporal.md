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

Release: v0.27.0.

`temporal.Register(vm)` installs `Temporal.Now`, `Instant`, `ZonedDateTime`, `PlainDate`, `PlainTime`, `PlainDateTime`, `PlainYearMonth`, `PlainMonthDay` and `Duration` with every method V8 ships, and `Date.prototype.toTemporalInstant`. go-intl's temporal package (`internal/gointl/temporal`, a Go port of temporal_rs, the library V8 builds Temporal on) does the computing. `temporal.js` reads arguments, options and property bags in the order V8 reads them, with V8's conversions and error messages, and keeps each object's internal slots in a private field no page can reach.

It is installed as `Intl` is (see [time-and-intl.md](time-and-intl.md#intl-calls-go-through-builtins)). `Register` puts stand-ins for `Temporal` and `toTemporalInstant` on the VM and takes the built-ins Temporal's code uses, before any page runs. The first use installs the rest, from bytecode compiled once per process, through one `Builtin` whose first argument names the operation. The first use replaces a stand-in only while it is still there and can be replaced: what a page put in its place, a property it deleted, or a stand-in frozen with its object stays, as it would with V8's built-ins, and a stand-in left in place forwards to the real thing. A page that changes built-ins after `Register` does not change what Temporal does.

### Clock and zone

`Temporal.Now` reads the clock `temporal.Clock` sets, `time.Now` without it. Only `Temporal.Now` reads it: `Date.now`, `new Date()` and `Intl` keep the host's clock. A clock outside an `Instant`'s range, 1e8 days either side of 1970, makes `Temporal.Now` throw a `RangeError`. The clock must not block: the VM waits for it, and the eval timeout cannot cut it short, since the engine checks for one only between its own instructions. A panic in the clock reaches the caller of `Eval`, and the VM can be used after it. `Close` drops the VM's clock (`VM.OnClose`), so a clock that references its VM does not keep it.

`Temporal.Now.timeZoneId()` is the default zone package `intl` formats in, `intl.TimeZone`, or `UTC` without it, as ECMA-402's DefaultTimeZone is, and the `Now` methods called without a zone use it. The engine's `Date` does not follow it: its local time is UTC in every VM (see [time-and-intl.md](time-and-intl.md#local-time-is-utc-in-every-vm)), so with `intl.TimeZone("Asia/Tokyo")`, `new Date().getHours()` and `Temporal.Now.plainTimeISO().hour` are 9 hours apart.

### toLocaleString

`toLocaleString` uses package `intl` through `internal/intlhook`, whether or not the VM has `intl` registered. The date types use the same type-specific formats as `Intl.DateTimeFormat.format`, `formatToParts`, `formatRange` and `formatRangeToParts`. Each type gets its own default fields and calendar checks. Plain values ignore `timeZone`, including skipped local dates and times. An `Instant` uses the formatter's zone. A `ZonedDateTime` uses its own zone in `toLocaleString`, but passing it to `DateTimeFormat` throws `TypeError`. Range endpoints must have the same Temporal type.

The ISO calendar is supported. A `PlainYearMonth` or `PlainMonthDay` with that calendar needs a matching `calendar: "iso8601"` option or locale extension, as in Chromium. All 18 calendars listed by Chromium are supported. An ISO date or date-time can use the resolved calendar. Other date, date-time, year-month and month-day values require a matching calendar and throw `RangeError` otherwise.

`Duration.prototype.toLocaleString` uses `Intl.DurationFormat`, including styles, units and fractional digits. Both paths read private slots and captured built-ins, so page changes to getters, constructors or prototypes cannot change their output. An `Intl.Locale` is read from its slots too.

Date formatting now reads go-intl's ICU zone data, the same pack Temporal uses. `ZonedDateTime.toLocaleString` also keeps the offset Temporal supplied if the two ever disagree, naming that offset instead. Named zones are also validated through the embedded pack. No host zoneinfo is needed.

Tests: `temporal/formatting_test.go` compares 3,216 date expressions and 219 duration expressions with Chromium 156.0.8078.4. It also checks private slots after GC, either installation order, cache eviction and changed built-ins. [Time and Intl](time-and-intl.md#datetimeformat-takes-temporal-values) describes the bridge and data cost.

### Zone data

The zones and calendar tables are go-intl's, built into the binary: ICU's `zoneinfo64` from ICU's time zone update for tz 2026e, and the tables of the Chinese, Korean and Umm al-Qura calendars. They are the 92 KB pack `internal/gointl/qjsdata/tz.bin`, which Temporal reads through `gointl.EmbeddedTemporalSource`. `gointl.Embedded`, which package `intl` reads, serves the same pack with the rest, and a chunk read through either is inflated once. Since Temporal imports `intl` for `toLocaleString`, a program that uses it links the packs `intl` links as well (`TestLinksOnlyReadData` in `temporal/link_test.go`). The host's zoneinfo is never read, so Temporal gives the same answers on every machine.

go-intl v0.6.1 carries tz 2026c. The packs take their zones from the update `internal/gointl/qjstz` names instead, made by go-intl's own `tzgen` (see [upgrading.md](../upgrading.md#updating-go-intl-internalgointl)): since 2026c, 2026d keeps the Northwest Territories on -06 from 2026-08-21 and 2026e keeps Manitoba on -05 from 2026-10-31. `tz.bin` holds `tzversion.bin`, the release of its zones, which `TestQJSTZVersion` checks.

### Results

The original Temporal implementation passed 9206 of 9210 `built-ins/Temporal` runs and all 16 `Date/prototype/toTemporalInstant` runs at test262 commit `2e0a5676`. Those suites were not rerun for the formatting change. The four known built-in failures are ISO `PlainYearMonth` and `PlainMonthDay` default locale formatting, which Chromium 156 rejects too.

The focused formatting comparison is recorded in [Time and Intl](time-and-intl.md#datetimeformat-takes-temporal-values). All 723 existing Temporal corpus expressions now match Chromium, including the Japanese calendar and error messages. The earlier era-only, Chinese month and duration differences are gone.

Before this formatting change, on an AMD EPYC 7B13 core, linux/amd64, without PGO: the example shop of tifa, built with `-ldflags=-w` as `tifa build` does, grows by 0.84 MB (27.09 MB to 27.93 MB), all of it code: the zones and calendars are `tz.bin`, which package `intl` links already. `Register` takes 0.06 ms and no measurable memory. The first use in a process takes 13.4 ms, 8.7 ms of it to compile the javascript and most of the rest to inflate the zones and calendars, 236 KB of heap kept for the process. The first use in a later VM takes 1.1 ms and 358 KB. `Temporal.PlainDate.from('2024-01-05')` takes 3.5 µs, `add({months: 1})` 14 µs, `until` with `largestUnit: 'years'` 11 µs, `ZonedDateTime.from` 7.3 µs, its `add({days: 1})` 17 µs, `toString` 1.4 µs for a date and 5.5 µs for a zoned date-time, and `toLocaleString` 3.8 µs for a date and 4.9 µs for a zoned date-time.

The later [Intl data migration](time-and-intl.md#all-intl-formatters-share-go-intl) replaces those packs with Chromium-selected resources and removes `cldr.bin`. That section records the current shop size and Intl timings. Date and duration formatting also reuse the [compiled Intl plans](time-and-intl.md#compiled-intl-formatting-plans). Their recorded answers are unchanged. [Performance](../performance.md#intl-formatting) records the Intl calls and the ten-page shop comparison.

Tests: `temporal/all_test.go`, `temporal/shape_test.go`, `temporal/tamper_test.go`, `temporal/chromium_test.go`, `temporal/concurrent_test.go`, `temporal/memgrind_test.go`, `temporal/link_test.go`, `intl/temporal_test.go`, `internal/gointl/qjs_source_test.go`. The checks to run after a change are in [testing.md](../testing.md#before-committing).
