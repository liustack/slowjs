---
summary: 'The fork changes to time and Intl: javascript local time is UTC in every VM, Intl in every locale Chromium has with what it costs, fraction digits as ECMA-402 reads them since 2021, daylight saving time named as ICU does on every system, and Intl calling Go through Builtins with formatters shared across VMs'
read_when:
  - Changing package intl, fixTimezone, or the getTimezoneOffset hunk of the C patch
  - fixlib stops on _getTimezoneOffset or a call of libc's local time functions after an upstream release
  - A page shows another date or time on the server than in the browser, or a zone name differs from Chromium
  - Asking what Intl costs in program size, VM memory and time
  - Changing intl/bridge.go, intl/registry.go, intl.js or stub.js, or adding a constructor to the bridge
  - A page changed a built-in and Intl gave another result
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

## Intl in every locale Chromium has

Release: v0.26.2.

Package `intl` formats in every locale Chromium has, from CLDR 48.2 (cldr-json 48.2.3), where upstream formats en-US only. It covers `Intl.DateTimeFormat`, `Intl.NumberFormat`, `Intl.PluralRules` and the `toLocale*` methods, and its output is compared with Chromium 156 on about 60,000 expressions (`intl/golden_test.go`). Without a locale argument the locale is en-US, and without a `timeZone` option the zone is UTC: `Intl` and the `toLocale*` methods never read the host's settings, and neither does the engine's `Date` (see [Local time is UTC in every VM](#local-time-is-utc-in-every-vm)). `Register` only puts stand-ins on the VM. The first use of `Intl` or a `toLocale*` method installs the rest, from bytecode compiled once per process. The package documentation lists what is not implemented and where the output differs from Chromium.

What it costs, measured on darwin/arm64 against the en-US version: a program that uses `intl` is 1.7 MB larger (1.3 MB of it is the locale data). `Register` takes 0.09 ms and 9 KB of the VM's memory, against 0.85 ms and 46 KB before. The first use in a VM then takes 0.15 ms and 42 KB more. A language is decoded into the Go heap the first time it is used, once per process, in about 0.3 ms and 100 to 180 KB. The first use of any language also decodes about 1 MB of shared data. Formatting a date takes 3.3 µs in en-US (4.3 µs before) and 3.7 µs in zh-CN, a number 2.4 µs in en-US (2.4 µs before) and 2.7 µs in de. [Intl calls Go through Builtins](#intl-calls-go-through-builtins) has the times since.

## Fraction digits as ECMA-402 reads them since 2021

Release: v0.26.4.

`Intl.NumberFormat` reads the fraction digits as ECMA-402 has since its 2021 edition, as browsers do. A currency with two default fraction digits and `maximumFractionDigits: 0` threw a `RangeError`, as ECMA-402 did until its 2020 edition: now the minimum is the smaller of its default and the given maximum. The fraction digits are read only when they are used, so `{maximumSignificantDigits: 3, maximumFractionDigits: 200}` formats, where it threw a `RangeError`.

Tests: `intl/all_test.go`.

## Daylight saving time named as ICU does

Release: v0.26.9.

Package `intl` tells daylight saving time from standard time as ICU does, whichever form of the tz data Go reads. In the main form, which Linux distributions and Go's `time/tzdata` ship, Europe/Dublin's summer time is its standard time and its winter time is daylight saving time, an hour behind. Africa/Casablanca, Africa/El_Aaiun, Africa/Windhoek and Europe/Prague (1946 to 1947) have such negative daylight saving time too. ICU builds from the rearguard form, where daylight saving time is always ahead of standard time, and macOS ships that form. Go's `IsDST` says what the data says, so on Linux the names of these zones came out swapped: Dublin in winter was `Irish Standard Time`, the name Chromium gives its summer. For these zones and their links the flag now comes from a table of the rearguard form of tzdb 2026d, the release in Chromium 156's ICU (`intl/tzdst_table.go`, see [Updating the intl locale data](../upgrading.md#updating-the-intl-locale-data)). When Go reads another release and the table has another offset for a time, or the time is past 2037, the flag is the one of the zone's last period with the same offset.

Tests: `intl/tzdst_test.go`.

## Intl calls Go through Builtins

Release: not released yet.

Package `intl` calls Go through `Builtin`s (see [go-api.md](go-api.md#builtin-a-go-function-called-as-a-built-in)), with no reflection and no JSON on the way in, and keeps its formatters in one registry for the whole process: a `NumberFormat` made with the same locale and options in another VM, or spelled another way, reuses the one already made. The registry keeps up to 1024 formatters and drops the oldest, and an object whose formatter was dropped makes it again on its next call. Its functions have the shape of built-ins (`length`, `name`, no `prototype`, `toString` showing `[native code]`, accessors and descriptors as ECMA-402 has them), and a page that changes `Array.prototype`, `Object.prototype`, `JSON` or other built-ins after `Register` does not change what it does.

On an AMD EPYC 7B13 core: `new Intl.NumberFormat('en')` takes 2.7 µs (16.5 µs before), `format` 3.2 µs (5.6 µs), `formatToParts` 6.7 µs (12.9 µs), a `DateTimeFormat` with a time zone 8.1 µs to make (32.0 µs) and 2.6 µs to format (5.2 µs), `PluralRules.select` 0.9 µs (2.9 µs) and `toLocaleDateString` with options 9.5 µs (41.8 µs). A VM that has used `Intl` holds about 15 KB less. test262's intl402 tests pass 912 runs of 6764 (770 before), none of those that passed before failing.

How it works is in [architecture.md](../architecture.md#packages-next-to-the-engine), and the comment at the top of `intl/bridge.go` says how to add a constructor.

Tests: `intl/registry_test.go`, `intl/shape_test.go`, `intl/tamper_test.go`, `intl/convert_test.go`, `intl/legacy_test.go`.
