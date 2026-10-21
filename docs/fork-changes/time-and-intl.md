---
summary: 'Go clocks, UTC local time, Intl formatting on shared go-intl data and tz 2026e, Chromium compatibility, and measured costs'
read_when:
  - Changing package intl, fixTimezone, fixClock, lib/clock.go, or the getTimezoneOffset hunk of the C patch
  - fixlib stops on _getTimezoneOffset, a call of libc's local time functions, clockSites or a call of a clock of libc after an upstream release
  - Date.now, new Date() or performance.now is slow, or gives a time other than Go's time.Now
  - A page shows another date or time on the server than in the browser, or a zone name differs from Chromium
  - Asking what Intl costs in program size, VM memory and time
  - Changing intl/bridge.go, intl/registry.go, intl.js or stub.js, or adding a constructor to the bridge
  - A page changed a built-in and Intl gave another result
  - Changing a constructor built on go-intl, or chromiumData in intl/gointl.go
  - A page replaced or deleted Intl or a toLocaleString method before the first use, and Intl put it back
---

# Time and Intl

Server rendered pages must come out the same on every machine and match what a browser shows. [Updating the intl locale data](../upgrading.md#updating-the-intl-locale-data) says how to move to a new CLDR, tzdb or Chromium.

## Local time is UTC in every VM

Release: v0.26.3.

A script's local time is UTC in every VM, whatever zone the process has. The engine used to take the zone from libc's `localtime`, which reads `TZ`. The process shares that libc and that variable with Go's `time.Local`, its logs and SQLite (through modernc.org/libc), so setting `TZ=UTC` to make server rendered pages the same on every machine changed all of those too. Now the engine's offset from UTC is always 0 and the process's zone is left alone. `Intl`, from package `intl`, defaults to UTC as well. What this changes for scripts:

- `getTimezoneOffset()` is 0, and `getHours`, `getDate`, `getDay` and the other local getters give the same values as their `getUTC*` counterparts. The local setters (`setHours`, ...) set UTC fields.
- `new Date(2026, 0, 1)`, `Date.parse` of a date and time without an offset (`'2026-01-01T13:05:00'`) and the other local forms read the fields as UTC.
- `toString`, `toDateString` and `toTimeString` print UTC with `GMT+0000`. The engine's own `toLocaleString`, `toLocaleDateString` and `toLocaleTimeString` print UTC too. Package `intl` replaces those three, and its versions print UTC unless given a `timeZone`.

A script that needs another zone names it to `Intl`, with package `intl` registered: `d.toLocaleString('zh-CN', {timeZone: 'Asia/Shanghai'})` or `new Intl.DateTimeFormat('en-US', {timeZone: 'America/New_York', hour: 'numeric'}).formatToParts(d)`. Without `intl` the engine has no way to show another zone.

Tests: `dateutc_unix_test.go`, `internal/fixlib/main_test.go`.

### When upstream changes it

What to do when `go run ./internal/fixlib` stops on this change after an upstream release. [upgrading.md](../upgrading.md) has the whole procedure and the order in which to update the lists `fixlib` checks against.

If it reports that `_getTimezoneOffset` changed shape, or that the engine calls libc's local time functions (`localtime`, `mktime`, `tzset`, ...) somewhere else, upstream reads the local zone some new way: make that answer UTC too and teach `fixTimezone` about it.

## The engine reads Go's clocks

Release: not released yet.

Every function of the engine that reads the time asks Go instead of libc. The `gettimeofday` and `clock_gettime` of modernc.org/libc v1.77.1 are system calls, while Go's `time.Now` reads the clock without entering the kernel where the system allows it (the vDSO on Linux), so `Date.now()` cost several times what it costs in other Go engines. `fixClock` gives these functions a body that calls `lib/clock.go`:

| Function | What reads it | Clock |
| --- | --- | --- |
| `date_now` | `Date.now()`, `new Date()`, `Date()` | `time.Now().UnixMilli()`: whole milliseconds since 1970, rounded down, as before |
| `js_random_init` | the seed of `Math.random` in a new context | `time.Now().UnixMicro()`, and 1 for 0, as before |
| `get_time_ns` | `performance.now()` of `StdAddHelpers` | Go's monotonic clock, from the start of the process |

The function each one replaces stays in the file, with fixlib's other changes, under a name ending in `_libc`, next to the new one. Nothing calls it, so the linker leaves it out, but `fixlib` checks its text against `clockSites` on every run, and `restoreClock` gives the file back byte for byte. `Atomics.wait` still reads libc's `CLOCK_REALTIME`: it hands the deadline to libc's `pthread_cond_timedwait`, which measures it on that clock, and it is not a hot path. The eval timeout already ran on Go's monotonic clock (see [uncatchable.md](uncatchable.md#the-eval-timeout-runs-on-the-monotonic-clock)), and local time stays UTC.

What a script can see change: `performance.now()` counts from the start of the process, as in Node. Before, it counted from the boot of the system on Linux and macOS, and on Windows and FreeBSD, where quickjs-libc read the wall clock (`gettimeofday`) for it, it gave milliseconds since 1970 and moved when the system clock was set. Its resolution, nanoseconds in a double of milliseconds, does not change, and neither does its rate, except on macOS: the old `CLOCK_MONOTONIC` of macOS goes on while the system sleeps, and Go's monotonic clock (`mach_absolute_time`) does not, so a script that spans a sleep sees less time pass. On Windows, `Date.now()` and `new Date()` no longer overflow in 2038: libc's `timeval` there has 32 bits of seconds.

On one core of an AMD EPYC 7B13 (linux/amd64, without PGO, `BenchmarkClock`, 6 rounds of before and after in turn, the median of 18 runs of each): `Date.now()` takes 122 ns rather than 689, `new Date()` 261 ns rather than 870, and `performance.now()` 107 ns rather than 686. A render of a page of tifa's example shop reads the clock too rarely to gain: over 6 rounds (a seventh, which overlapped another job on the same core, is left out) the sum of the six P50 came out at 0.994 to 1.009 of the time (median 1.000), inside the spread of a round, with the same HTML.

This changes only the generated Go, with no C hunk (`internal/fixlib/clock.go`).

Tests: `lib/clock_test.go` (the values of `Date.now`, `new Date()` and `Date()` for a given `time.Now`, rounding before 1970, agreement with Go's clock, the seed of `Math.random`, and a monotonic `performance.now` that counts from the start of the process, with a fraction that goes on while the wall clock stands still), `internal/fixlib/clock_test.go`, and `BenchmarkClock` in `clock_test.go`.

### When upstream changes it

If it reports that a function of `clockFuncs` changed (a fingerprint other than the one in `internal/fixlib/clocksites.go`), read it against `lib/clock.go`: `date_now` must still give milliseconds since 1970 from the wall clock, `js_random_init` the time in microseconds with 1 for 0, and `get_time_ns` a monotonic clock in nanoseconds. Then put the new fingerprints in (`go run ./internal/fixlib -clock` prints them). If it reports that the engine reads the time some new way, upstream uses a function of libc whose name says it deals with time (`gettimeofday`, `clock_gettime`, `time`, `QueryPerformanceCounter`, `mach_absolute_time`, ...), called or only by address, or a shim such as mingw's `_time64`, outside these functions and `_js_atomics_wait`: make it call `lib/clock.go` too and add it to `clockFuncs`. A function of libc that reads no clock goes in `clockAllowed` instead. `pthread_cond_timedwait` may appear only in `_js_atomics_wait`: a new timed wait must be read against the clock it measures its deadline on. If it reports that a function of `clockFuncs` is gone, upstream removed it: take it out of `clockFuncs`, then update the fingerprints and this page.

## Intl in every locale Chromium has

Release: v0.26.2.

The first locale implementation used CLDR 48.2 (cldr-json 48.2.3) to format every locale Chromium has, where upstream formats en-US only. The shared go-intl packs now replace that data. It covers `Intl.DateTimeFormat`, `Intl.NumberFormat`, `Intl.PluralRules` and the `toLocale*` methods, and its output is compared with Chromium 156 on about 60,000 expressions (`intl/golden_test.go`). Without a locale argument the locale is en-US, and without a `timeZone` option the zone is UTC: `Intl` and the `toLocale*` methods never read the host's settings, and neither does the engine's `Date` (see [Local time is UTC in every VM](#local-time-is-utc-in-every-vm)). `Register` only puts stand-ins on the VM. The first use of `Intl` or a `toLocale*` method installs the rest, from bytecode compiled once per process. The package documentation lists what is not implemented and where the output differs from Chromium.

At that release, measured on darwin/arm64 against the en-US version: a program that uses `intl` is 1.7 MB larger (1.3 MB of it is the locale data). `Register` takes 0.09 ms and 9 KB of the VM's memory, against 0.85 ms and 46 KB before. The first use in a VM then takes 0.15 ms and 42 KB more. A language is decoded into the Go heap the first time it is used, once per process, in about 0.3 ms and 100 to 180 KB. The first use of any language also decodes about 1 MB of shared data. Formatting a date takes 3.3 µs in en-US (4.3 µs before) and 3.7 µs in zh-CN, a number 2.4 µs in en-US (2.4 µs before) and 2.7 µs in de. [Intl calls Go through Builtins](#intl-calls-go-through-builtins) has the times since.

## Fraction digits as ECMA-402 reads them since 2021

Release: v0.26.4.

`Intl.NumberFormat` reads the fraction digits as ECMA-402 has since its 2021 edition, as browsers do. A currency with two default fraction digits and `maximumFractionDigits: 0` threw a `RangeError`, as ECMA-402 did until its 2020 edition: now the minimum is the smaller of its default and the given maximum. The fraction digits are read only when they are used, so `{maximumSignificantDigits: 3, maximumFractionDigits: 200}` formats, where it threw a `RangeError`.

Tests: `intl/all_test.go`.

## Daylight saving time named as ICU does

Release: v0.26.9.

This release corrected zone names when the host used negative daylight saving time, such as Dublin's winter period. It used a rearguard DST table to match ICU. The [shared go-intl migration](#all-intl-formatters-share-go-intl) removes that table and host-zone reading altogether. Offsets and daylight flags now come from the same embedded tz 2026e pack on every machine.

Tests: `intl/zonepack_test.go` and the Chromium zone corpus.

## Intl calls Go through Builtins

Release: v0.26.17.

Package `intl` calls Go through `Builtin`s (see [go-api.md](go-api.md#builtin-a-go-function-called-as-a-built-in)), with no reflection and no JSON on the way in, and keeps its formatters in one registry for the whole process: a `NumberFormat` made with the same locale and options in another VM, or spelled another way, reuses the one already made. The registry keeps up to 1024 formatters and drops the oldest, and an object whose formatter was dropped makes it again on its next call. Its functions have the shape of built-ins (`length`, `name`, no `prototype`, `toString` showing `[native code]`, accessors and descriptors as ECMA-402 has them), and a page that changes `Array.prototype`, `Object.prototype`, `JSON` or other built-ins after `Register` does not change what it does.

On an AMD EPYC 7B13 core: `new Intl.NumberFormat('en')` takes 2.7 µs (16.5 µs before), `format` 3.2 µs (5.6 µs), `formatToParts` 6.7 µs (12.9 µs), a `DateTimeFormat` with a time zone 8.1 µs to make (32.0 µs) and 2.6 µs to format (5.2 µs), `PluralRules.select` 0.9 µs (2.9 µs) and `toLocaleDateString` with options 9.5 µs (41.8 µs). A VM that has used `Intl` holds about 15 KB less. test262's intl402 tests pass 912 runs of 6764 (770 before), none of those that passed before failing.

How it works is in [architecture.md](../architecture.md#packages-next-to-the-engine), and the comment at the top of `intl/bridge.go` says how to add a constructor.

Tests: `intl/registry_test.go`, `intl/shape_test.go`, `intl/tamper_test.go`, `intl/convert_test.go`, `intl/legacy_test.go`.

## Intl constructors on go-intl

Release: not released yet.

Package `intl` adds `Intl.Collator`, `Intl.Segmenter`, `Intl.RelativeTimeFormat`, `Intl.ListFormat`, `Intl.DisplayNames`, `Intl.DurationFormat`, `Intl.Locale` and `Intl.supportedValuesOf`, built on go-intl (`internal/gointl`, see [Updating go-intl](../upgrading.md#updating-go-intl-internalgointl)) over the same bridge and registry as the others. `localeCompare` sorts by the locale's collation, and `toLocaleUpperCase` and `toLocaleLowerCase` follow Turkish, Azeri, Lithuanian and Greek rules as V8 does, where QuickJS ordered by code point and mapped case alike in every language. An `Intl.Locale` stands for its tag wherever a constructor takes locales. The remaining constructors now use the same packs, as described below.

Each constructor takes the locales Chromium 156 has, and serves the data Chromium serves for them: where Chromium's ICU data leaves out a language's relative times, lists or units, which it does for az, bs, kk, kok, pa, sd, uz and some of their scripts and regions, the output is root's or English, as in Chromium (`chromiumData` in `intl/gointl.go`, derived by `TestChromiumData`). The output is compared with Chromium's on 14,269 expressions in every one of those languages (`intl/golden_gointl_test.go`, recorded with `record.cjs golden <set>`). 13,671 now match exactly. The other 598 differ for causes the package documentation lists, each matched by a rule of `goldenDifferences` in `intl/golden_test.go`: 104 of the 2,901 of `Segmenter` (Chromium's own word break rules and Lao and Khmer dictionaries), 475 of the 1,635 of `DisplayNames` (Chromium's ICU data has fewer names, and a few of its own), 6 of the 1,042 of `DurationFormat` (V8 overflows past 2\*\*63 nanoseconds by the machine's rules), 1 of the 23 of `supportedValuesOf` (more currencies) and 12 of the 1,415 of `Locale` (five zones of 001, `-u-va-posix` in `baseName`, unknown regions). `Collator` (1,372), `RelativeTimeFormat` (1,247), `ListFormat` (1,298) and the case mapping (3,336) match in full. Where test262 reads ECMA-402 otherwise than V8, the answer is V8's: a `-u-` keyword with the value `yes`, `getTextInfo` of a script with no known direction, `toLocaleUpperCase` checking only the first locale, and two `DurationFormat` cases. The functions have the shape of built-ins and ignore a page's changes to built-ins, as the others do: `shape.json` and `tamper.json` are recorded in Chromium. Before Temporal and the remaining formatters moved to go-intl, test262's Intl402 tests passed 2282 of 6764 runs. The current full comparison is below.

Historical measurements for this first group of constructors, on an AMD EPYC 7B13 core, without PGO, against the version before:

- Size before the Temporal formatting change below: tifa's example shop, built for linux/amd64, is 9.8 MB larger (37.6 MB against 27.9 MB), nearly all of it go-intl's data: `core.bin` (4.4 MB), `names.bin` (1.9 MB), `cjdict.bin` (2.0 MB) and `tz.bin` (91 KB). Built with `-tags quickjs_intl_nonames,quickjs_intl_nocjdict`, which leave out the display names (`Intl.DisplayNames` then answers each code with its fallback) and the Chinese and Japanese word dictionary (a run of Chinese or Japanese is then one word), it is 5.8 MB larger. go-intl's dates and zone names (2.8 MB), which only its `DateTimeFormat` reads, stayed out of the binary at that stage (see [Updating go-intl](../upgrading.md#updating-go-intl-internalgointl)): with them the shop was 12.6 MB larger.
- Memory and start: `Register` takes 0.01 ms and 4 KB more. The first use in a VM takes 0.49 ms (0.24 ms before), and a VM that has used `Intl` holds 151 KB more than after `Register` (77 KB before). The javascript it installs went from 17 KB to 36 KB of source. The first collation in the process takes 4.7 ms and the first segmentation 2.7 ms, to read their data.
- Calls: `new Intl.Collator('de')` 2.8 µs and `compare` 2.4 µs, `new Intl.Segmenter('en', {granularity: 'word'})` 4.0 µs and iterating the words of 60 characters 78 µs (its graphemes 185 µs), `RelativeTimeFormat.format` 2.4 µs, `ListFormat.format` of three items 6.5 µs, `DisplayNames.of` 1.6 µs, `DurationFormat.format` 11 to 16 µs, `new Intl.Locale` 19 µs, `maximize` 18 µs, `getWeekInfo` 105 µs, `supportedValuesOf('timeZone')` 61 µs, and `toLocaleUpperCase('tr')` of 60 characters 4.8 µs (1.4 µs without a locale).
- Making a `NumberFormat` or `PluralRules` takes 0.1 µs more and a `DateTimeFormat` 0.3 µs more. Formatting takes no longer.

A program that imports `intl` now links `internal/gointl` and `internal/gointl/temporal`, and has to carry their notices (see [NOTICE.md](../../NOTICE.md#what-a-program-that-ships-a-binary-must-include)).

Tests: `intl/golden_gointl_test.go`, `intl/chromiumroot_test.go`, `intl/registry_gointl_test.go`, `intl/values_test.go`, `intl/shape_test.go`, `intl/tamper_test.go`, `intl/link_test.go`, `intl/nonames_test.go` (with `-tags quickjs_intl_nonames`), `internal/gointl/qjs_readers_test.go`, and `builtin_test.go` for `Args.WTF8`.

## Stand-ins leave what a page put in their place

Release: not released yet.

`Register` puts stand-ins on the VM, for `Intl`, the three `toLocale*String` methods of `Date` and the `toLocaleString` of `Number` and `BigInt`, and the first use of one installs the real ones. That install used to write every stand-in's property: a page's own `Date.prototype.toLocaleString`, or an `Intl` it defined with `Object.defineProperty`, was lost at the first use, a property it deleted came back, and a frozen `Date.prototype` or `globalThis` made the first use throw a `TypeError`. Now the install replaces a property only while it still holds the stand-in and can be replaced: what a page put there, deleted or froze stays, as it would with V8's built-ins, and a stand-in left in place forwards to the real thing. `Temporal` installs its stand-ins the same way (see [temporal.md](temporal.md)).

Named zones and aliases now resolve through the embedded pack, without consulting host files.

Tests: `TestStandInsKeepPageCode` in `intl/all_test.go`, `intl/zonepack_test.go`.

## Temporal durations use DurationFormat

Release: not released yet.

`Temporal.Duration.prototype.toLocaleString` uses the same formatter registry and Builtins as `Intl.DurationFormat`. It reads locales and options once, in constructor order, and formats the private duration fields without calling public getters or `toString`. It also works without `intl.Register`. This replaces the ISO duration string with localized units or a digital duration, without adding data packs or changing `fixlib`.

Tests: `TestDurationLocaleString`, `TestChromium` and `TestTamper` in `temporal`. The focused corpus is recorded with `node intl/testdata/chromium/record.cjs temporal duration-locale` using Chromium 156.

## DateTimeFormat takes Temporal values

Release: not released yet.

`Intl.DateTimeFormat` accepts `PlainDate`, `PlainTime`, `PlainDateTime`, `PlainYearMonth`, `PlainMonthDay` and `Instant` in `format`, `formatToParts`, `formatRange` and `formatRangeToParts`. A `ZonedDateTime` or different Temporal types in a range throw `TypeError`. Each type gets its own default fields. Calendar checks use the resolved calendar, and ISO year-month and month-day values work with an explicit ISO calendar. Plain types keep their wall-clock fields regardless of `timeZone`. Valid plain values at Temporal's bounds are not clipped to Date's smaller range. The range methods also accept legacy Date and numeric inputs. As in Chromium 156, reversed ranges keep the caller's order.

The JavaScript shims share a private object rooted in a private field of the VM's global object. It exposes no page-visible property. The Temporal shim supplies a reader of its private slots, and the Intl shim supplies its private locale reader. Registration order, changed prototypes and GC cannot forge or lose a value's brand. Formatter options are read once and retained for type-specific resolution and registry eviction. Range methods convert both arguments before checking their types. DurationFormat also uses the slot reader for real Temporal durations, so replaced getters are not called.

Date and Temporal values now use the same go-intl date and interval formatter. `qjs_temporalformat.go` preserves an unchanged pattern as Chromium 156 does. The [shared data migration](#all-intl-formatters-share-go-intl) records current data size and timings. The host-zone guessing pack stays out, which both packages' link tests check.

The focused corpus has 3,216 date expressions and 219 duration expressions recorded from Chromium 156.0.8078.4. The date cases cover all six accepted types, both range methods and their sources, calendar mismatches, field defaults, styles, fractional seconds, bounds, conversions, zones and changed instance properties. Record them with `node intl/testdata/chromium/record.cjs temporal datetime-format` and `temporal duration-locale`. Constructor shape and prototype tampering are also recorded with `record.cjs shape` and `record.cjs tamper`.

At test262 commit `2e0a5676`, the Intl402 files that mention Temporal ran in both modes on linux/amd64: 4,172 of 4,222 runs pass, against 4,008 on `641e996`. There are 184 newly passing runs and 20 newly failing ones. Chromium 156 fails those same 20 era and hour-cycle assertions. Of the 50 remaining failures, 26 also fail with the same assertion in Chromium. At that stage, the other 24 reflected limited calendar data or calendar enumeration. The shared data migration fixes those cases. The lunisolar comparison fails in Chromium too, but on a different date, so it remains in the calendar group. No engine code changed. The engine's separate 58-error baseline was not rerun.

Tests: `temporal/formatting_test.go`, `temporal/chromium_test.go`, `intl/temporal_test.go`, `intl/shape_test.go`, `intl/tamper_test.go`, `internal/intlhook/state_test.go` and both packages' link tests. No generated engine code, C patch or `fixlib` rewrite changes. After a go-intl update, review the `qjs_temporalformat.go` adapter against the unchanged-pattern, ISO-pattern and supplied-offset behavior, and rerun these Chromium corpora.

## All Intl formatters share go-intl

Release: not released yet.

`NumberFormat`, `DateTimeFormat` for Date values, and `PluralRules` now use go-intl, as do Number and Date locale methods. Number and plural rounding use the full `roundingIncrement` and `roundingPriority` rules. Date formatting supports all 18 calendars Chromium lists. The old `intl/cldr.bin` (1,275,236 bytes), its generator, the host zone loader and its DST table are removed.

The number bridge keeps BigInt digits distinct from numeric strings after primitive conversion. BigInts beyond the Number range stay exact, while numeric strings outside that range become infinity. `TestNumberFormatBigIntBeyondNumberRange` covers both signs, boxed BigInts, object conversion, strings, parts and BigInt locale methods through the public VM API.

The generator selects number, date and zone resources from go-intl's models using Chromium 156's ICU inventory. One set of packs serves every constructor. It preserves missing resources, parent choices, non-Latin number symbols and Chromium's zone names. The Hebrew style cases follow ICU's default pattern after its style constructor fails on missing RBNF data. Mixed date and time styles then follow V8's hour-cycle regeneration. Unknown data errors still fail.

Intl and Temporal share the embedded ICU tz 2026e pack. A fake `ZONEINFO` file cannot change an answer. Chromium 156 predates Manitoba's permanent daylight time: at `2026-11-15T12:00Z`, Winnipeg is `7:00 AM Central Daylight Time` here and `6:00 AM Central Standard Time` there. The migration corpus keeps the browser recording and requires the exact newer answers for three expressions.

The process-wide formatter registry and the captured built-ins remain. A date registry key also includes its actual pattern. V8 can report the same public options for different patterns, such as a quoted `de` in Spanish, so public options alone cannot identify a formatter. PluralRules keys also retain the parsed options because public resolved options omit fraction limits when significant digits are present, even though both affect precision selection. `TestPluralRulesRoundingRegistry` checks both construction orders, across VMs and after eviction, through the public JS API.

All 74,868 existing golden expressions pass their exact comparisons or existing documented difference rules. 356 answers change from the preceding formatting commit: 349 now match Chromium exactly. Seven DisplayNames combinations fix the currency field while retaining known differences in other fields. The base date, number and plural set matches in full. The new migration corpus has 939 expressions, with only the three exact tz-version differences above. All 723 existing Temporal corpus expressions match.

On GCP linux/amd64, test262 `2e0a5676` runs every Intl402 file in both modes with the same fresh-VM runner, registering Intl and Temporal. Against main `641e996`, passes rise from 6276 to 6618 of 6764 runs. There are 366 new passes and 24 new failures. Chromium 156 fails all 24 of those assertions too: Islamic-calendar fallback, the legacy ACT zone alias, and Temporal era and hour-cycle rules. The 146 remaining failures include NumberFormat ranges, second-realm tests the runner cannot host, and existing V8 differences. No new failure passes in Chromium. The engine's separate 58-error suite was not rerun.

The number pack records that its ICU resources are already resolved. Raw go-intl sources keep their separate number and currency alias rules. `qjs-sync.sh test` checks raw full data against upstream corpora, while `test-chromium` checks the shipped packs against the fork's browser corpora.

The six linked data packs total 9,352,287 bytes. The host-zone guessing pack remains unlinked. On GCP linux/amd64 with Go 1.27.2, tifa shop `fb31f616` built by `go tool tifa build` with `-ldflags=-w` and the same application PGO profile is 31,914,341 bytes, against 32,078,049 on main `641e996`. This is 163,708 bytes smaller, about 0.51%. The profile SHA256 is `9a3ca92cd8f8ab68477e5e5fe3e27fafee8719ac59223c6526a122d10d1de64a`.

### Focused timing and memory costs

Against main `641e996` on the same GCP AMD EPYC 7B13, without PGO, the comparison uses logical CPU 1 with sibling 5 idle, nice 19 and a 4 GiB memory limit with no swap. Each row has five alternating main and branch samples, one second per sample. The values are medians, with the minimum and maximum in parentheses.

NumberFormat uses USD currency and `1234567.891`. DateTimeFormat uses medium date and short time styles, UTC and `1755324309123`. `New` measures JavaScript construction after the shared registry is warm. `Format` uses a prepared formatter. `Resolve` constructs the Go formatter without the registry, with locale data already loaded. It excludes the JavaScript bridge. Parsing the test source is outside all three measurements.

| Operation | Main, µs | Branch, µs | Median change |
| --- | ---: | ---: | ---: |
| NumberFormat/en-US/New | 7.865 (7.826 to 7.889) | 7.898 (7.840 to 8.007) | +0.4% |
| NumberFormat/en-US/Format | 3.798 (3.792 to 3.847) | 4.197 (4.186 to 4.222) | +10.5% |
| NumberFormat/en-US/Resolve | 3.418 (3.401 to 3.454) | 26.381 (26.062 to 26.451) | +671.8% |
| DateTimeFormat/en-US/New | 8.280 (8.225 to 8.343) | 8.313 (8.251 to 8.329) | +0.4% |
| DateTimeFormat/en-US/Format | 3.063 (3.040 to 3.075) | 3.964 (3.950 to 3.970) | +29.4% |
| DateTimeFormat/en-US/Resolve | 10.559 (10.497 to 10.623) | 108.693 (108.534 to 109.464) | +929.4% |
| NumberFormat/zh-CN/New | 7.873 (7.856 to 7.906) | 7.875 (7.818 to 7.946) | +0.0% |
| NumberFormat/zh-CN/Format | 3.792 (3.742 to 3.799) | 4.235 (4.203 to 4.298) | +11.7% |
| NumberFormat/zh-CN/Resolve | 3.437 (3.408 to 3.461) | 25.355 (25.183 to 25.575) | +637.7% |
| DateTimeFormat/zh-CN/New | 8.306 (8.258 to 8.389) | 8.329 (8.217 to 8.393) | +0.3% |
| DateTimeFormat/zh-CN/Format | 2.827 (2.799 to 2.835) | 3.943 (3.897 to 4.032) | +39.5% |
| DateTimeFormat/zh-CN/Resolve | 10.302 (10.264 to 10.376) | 108.034 (107.381 to 109.518) | +948.7% |

JavaScript construction stays within the observed spread. Number formatting is about 11% slower, and date formatting 29.4% to 39.5% slower in these cases. Constructing a Go formatter without the registry takes about 7.4 to 10.5 times as long. These are fixed-input costs, not page-render timings.

Each memory case runs in five fresh processes and registers only Intl. VM memory is `VM.MemorySize` after first use and `RunGC`. The Go heap delta is measured after GC against the state before creating the VM. It includes registration, shared data loaded once per process and the first VM's Go state. It is not RSS and is not a cost to multiply by the VM count.

| Case | Main VM, KiB | Branch VM, KiB | Main total first Go heap delta, KiB | Branch total first Go heap delta, KiB |
| --- | ---: | ---: | ---: | ---: |
| NumberFormat/en-US | 358.586 (358.586 to 358.586) | 371.617 (371.617 to 371.617) | 1203.305 (1203.148 to 1203.305) | 677.852 (677.695 to 678.320) |
| DateTimeFormat/en-US | 358.586 (358.586 to 358.586) | 371.617 (371.617 to 371.617) | 1274.445 (1274.133 to 1274.602) | 772.078 (771.922 to 772.078) |
| NumberFormat/zh-CN | 358.586 (358.586 to 358.586) | 371.617 (371.617 to 371.617) | 1177.359 (1177.203 to 1177.516) | 678.414 (678.258 to 678.727) |
| DateTimeFormat/zh-CN | 358.586 (358.586 to 358.586) | 371.617 (371.617 to 371.617) | 1249.047 (1248.891 to 1249.203) | 774.242 (774.086 to 774.711) |

A used VM retains 13.031 KiB more engine memory. The first process use retains about 475 to 525 KiB less Go heap. With the data warm, 32 more VMs retain about 1.08 KiB of Go heap each on main and 1.35 to 1.37 KiB here. The full sample spread for that estimate is below.

| Case | Main warm Go heap per VM, bytes | Branch warm Go heap per VM, bytes |
| --- | ---: | ---: |
| NumberFormat/en-US | 1109 (1109 to 1109) | 1385 (1385 to 1410) |
| DateTimeFormat/en-US | 1116 (1116 to 1117) | 1407 (1382 to 1419) |
| NumberFormat/zh-CN | 1109 (1109 to 1109) | 1385 (1375 to 1410) |
| DateTimeFormat/zh-CN | 1116 (1116 to 1116) | 1407 (1399 to 1412) |

Tests: `TestGoIntlMigration`, `TestEmbeddedZonePack`, every Intl golden set, Temporal formatting corpora, constructor shape, tampering, registry sharing and eviction, and link tests. `qjs-sync.sh pack -check` checks reproducibility, and the pack tests check shared-pool references. No generated engine code, C patch or fixlib rewrite changes. After a go-intl or Chromium update, rerecord the ICU inventory, rebuild the packs and review every changed expression. See [upgrading.md](../upgrading.md#updating-the-intl-locale-data).
