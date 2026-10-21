# Notices

This repository holds code and data from several sources, each under its own license. This file says which license covers which path, and which notices a program built with this module has to carry. It states what each license text says. It is not legal advice.

## Who wrote what

| Code | Written by | License | License file |
| --- | --- | --- | --- |
| Files this fork created. Its Go and shell source files carry an SPDX line. Files that carry none: the docs it wrote (`docs/`, `AGENTS.md`, `NOTICE.md`, `README.zh-CN.md`, `CHANGELOG.md`, `CHANGELOG.zh-CN.md`), its test scripts (`lib/testdata/*.js`, `testdata/*.js`, `intl/testdata/chromium/shape.js`, `intl/testdata/chromium/tamper.js`), `.gitattributes`, `intl/.gitignore` and `lib/internal/run-test262/go.mod` | The liustack/quickjs Authors ([AUTHORS-FORK](AUTHORS-FORK)) | MIT | [LICENSE-MIT](LICENSE-MIT) |
| 15 of those files, in `lib/`, repeat parts of the engine as ccgo translated it: `atomkey.go`, `backtrace.go`, `backtrace32.go`, `backtrace64.go`, `bitwise.go`, `clock.go`, `forin.go`, `frontcache.go`, `funcshape.go`, `jsonstr.go`, `objlit.go`, `outofmemory.go`, `poll.go`, `regexskip.go`, `shapecache.go`. Their SPDX line is `MIT AND BSD-3-Clause`, and their header names QuickJS and modernc.org/libquickjs | The liustack/quickjs Authors. The parts that follow the engine: the libquickjs-go Authors, from QuickJS by Fabrice Bellard and Charlie Gordon | MIT for the fork's code. BSD-3-Clause and MIT (QuickJS) for the parts that follow the engine | [LICENSE-MIT](LICENSE-MIT), [lib/LICENSE](lib/LICENSE), [LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS) |
| The Go API and everything else that came from modernc.org/quickjs v0.25.0: the root package, `intl/` and `fmt/` files that upstream had, `compare/`, `internal/autogen/`, `generator.go`, `builder.json`, `testdata/benchmarks/`, `logo.png` | The quickjs-go Authors (Jan Mercl, Dan Kortschak) | BSD-3-Clause | [LICENSE](LICENSE) |
| The engine as ccgo translated it, from modernc.org/libquickjs v0.13.1: `lib/ccgo_*.go`, `lib/internal/run-test262/ccgo_*.go` and the other files of `lib/` that upstream had | The libquickjs-go Authors (Jan Mercl), from QuickJS by Fabrice Bellard and Charlie Gordon | BSD-3-Clause for the translation, MIT for QuickJS | [lib/LICENSE](lib/LICENSE), [lib/LICENSE-LIBQUICKJS](lib/LICENSE-LIBQUICKJS), also at the root as [LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS) |
| QuickJS's micro benchmarks, `compare/testdata/*.js` | Fabrice Bellard, Charlie Gordon | MIT | [compare/testdata/LICENSE](compare/testdata/LICENSE) |
| `internal/gointl/`, except the files named `qjs*` and `UPSTREAM`: a copy of go-intl v0.6.1 (commit `59e7b7a`) | The go-intl Authors | Unicode License V3 | [internal/gointl/LICENSE](internal/gointl/LICENSE) |
| The test262 suite, which the tests of `lib/` download and run. Only its settings, expected errors and a patch to it are kept here, as files of libquickjs | Ecma International | BSD (Ecma's test262 license) | [lib/LICENSE_TEST262](lib/LICENSE_TEST262) |
| `internal/arewefastyet/`: the V8 benchmark suite v7 as AreWeFastYet kept it | The V8 project authors and others, each named in its file | AreWeFastYet: MPL-2.0. The benchmarks: the license in each `.js` file (BSD-3-Clause for most, an MIT-style license for the Prototype code in `raytrace.js`) | [internal/arewefastyet/LICENSE](internal/arewefastyet/LICENSE) and the headers of `internal/arewefastyet/v8-v7/*.js` |

A file the fork changed but did not create stays under its upstream license. The fork's changes to it are offered under MIT as well, so the file can still be used under its upstream license alone. The generated `lib/ccgo_*.go` files, with the rewrites `internal/fixlib` makes to them, are such files: BSD-3-Clause and MIT (QuickJS). So are `README.md` and `CLAUDE.md`, which upstream had and the fork rewrote. The patterns and the replacement text in `internal/fixlib`, and the inputs of its tests, quote lines of the ccgo output: those lines stay under BSD-3-Clause and MIT (QuickJS) as well.

## Data

| Data | Comes from | License | Notice |
| --- | --- | --- | --- |
| `intl/cldr.bin`, `intl/testdata/chromium/icu.json.gz`, the values in `intl/internal/gen/overrides.go` | Unicode CLDR 48.2 (cldr-json 48.2.3), the ICU 78.2 data of Chromium 156.0.8078.4, with Chromium's changes to it | Unicode License V3, ICU's license for the data, Chromium's BSD-3-Clause license | [intl/LICENSE-DATA](intl/LICENSE-DATA) |
| `intl/testdata/chromium/golden.json.gz`, `golden-*.json.gz`, `shape.json`, `tamper.json` and `availability.json` | What Chromium 156.0.8078.4 answered to `Intl` calls, made from the same ICU 78.2 data. The expressions in `golden.json.gz` were written for this fork | The same notices as `icu.json.gz` | [intl/LICENSE-DATA](intl/LICENSE-DATA) |
| `intl/tzdst_table.go`, `intl/testdata/tzdata/` | The IANA tz database, release 2026d | Public domain | None needed |
| `internal/gointl/qjsdata/` (`core.bin`, `names.bin`, `cjdict.bin`, `tz.bin`, `dates.bin`, `zonenames.bin`, `hostzones.bin`, `chromium.json`) | Made from go-intl's data, which comes from Unicode CLDR 48.2, ICU 78.3 and the IANA tz database, as [internal/gointl/SOURCES.md](internal/gointl/SOURCES.md) lists. The time zones in `tz.bin` and the Windows and abbreviation zones in `hostzones.bin` come instead from ICU's time zone update for tz 2026e (`internal/gointl/qjstz`), which go-intl's `tzgen` makes them from. The locale lists come from Chromium 156 | Unicode License V3 | [internal/gointl/LICENSE](internal/gointl/LICENSE) |
| The word break dictionaries in `internal/gointl/qjsdata/`: Thai, Lao, Khmer and Burmese in `core.bin`, Chinese and Japanese in `cjdict.bin` | ICU 78.3 (`icudt78l.dat`). The Chinese and Japanese one is Google's, made from the Libtabe and IPADIC word lists. The Lao and Burmese ones are word lists by their authors | Unicode License V3, the ICU License for ICU 1.8.1 to 57.1, and each dictionary's notice: Google's BSD license with the Libtabe and IPADIC licenses for Chinese and Japanese, a BSD license by the word list's authors for Lao and for Burmese | [internal/gointl/qjsdata/NOTICE-ICU](internal/gointl/qjsdata/NOTICE-ICU), lines 1 to 401 of ICU's LICENSE at `release-78.3` |
| `temporal/testdata/chromium.json`, `temporal/testdata/chromium-shape.json` | What Chromium 156.0.8078.4 answered to the Temporal expressions in `temporal/testdata/corpus.js`, written for this fork, and the shape of its Temporal objects. The answers of `toLocaleString` are made from Chromium's ICU 78.2 data | The same notices as `icu.json.gz` | [intl/LICENSE-DATA](intl/LICENSE-DATA) |
| `lib/testdata/*.txt`, `testdata/*.golden` | What Node and the engine print for the tests of this fork, which wrote the scripts that make them | MIT, as test data of this fork | [LICENSE-MIT](LICENSE-MIT) |

## Code and data that go-intl took from other projects

go-intl's own [LICENSE](internal/gointl/LICENSE) names the Unicode License V3, for its code, for the data it made from Unicode's and ICU's, and for code adapted from ICU4C and ICU4X. Its [SOURCES.md](internal/gointl/SOURCES.md) says that it also took code and data from V8 and from the Rust crates that Temporal in Node is built from, at the versions Node v26.10.0 pins. Those ports are modified works: Go written from the C++ or the Rust and changed to fit go-intl, and SOURCES.md says what is ported from which project, at which version. The notices of those projects are in `internal/gointl/qjsdata/`, each copied unchanged from the project at that version, after a short note of ours on what it covers. They sit there because `qjs-sync.sh update` deletes the files of `internal/gointl` that are not named `qjs*`.

| File | Project, version | License |
| --- | --- | --- |
| [NOTICE-ICU](internal/gointl/qjsdata/NOTICE-ICU) | ICU 78.3: the break rules and the five word break dictionaries | Unicode License V3, the ICU License for ICU 1.8.1 to 57.1, and the dictionaries' notices (see Data above) |
| [NOTICE-ICU4X](internal/gointl/qjsdata/NOTICE-ICU4X) | ICU4X's crates: ixdtf 0.6.4, zoneinfo64 0.3.0, icu_calendar 2.2.1, icu_calendar_data 2.2.0, icu_locale_core 2.2.0, calendrical_calculations 0.2.4 | Unicode-3.0, except calendrical_calculations: Apache-2.0 only. No NOTICE file |
| [NOTICE-TEMPORAL](internal/gointl/qjsdata/NOTICE-TEMPORAL) | boa-dev's crates: temporal_rs 0.2.3, temporal_capi 0.2.3, timezone_provider 0.2.3 | MIT or Apache-2.0, at the user's choice. No NOTICE file |
| [NOTICE-V8](internal/gointl/qjsdata/NOTICE-V8) | V8 as Node v26.10.0 vendors it | BSD-3-Clause |

Apache-2.0 asks three things of a program that passes on a changed work (section 4): (a) give its recipients a copy of the license, which NOTICE-ICU4X holds, (b) make each changed file say that it was changed, which the header that `qjs.patch` puts on `temporal/solar.go` and `temporal/zoneinfo.go`, the files ported from calendrical_calculations, does, and (c) keep the copyright notices of the source, which that header and NOTICE-ICU4X do. For temporal_rs, temporal_capi and timezone_provider this module takes the MIT license, one of the two they offer.

Which package needs which:

| Package | What it took | Notices |
| --- | --- | --- |
| `internal/gointl` itself and its `internal/` packages | ICU's data and dictionaries, in `core.bin` and `cjdict.bin`, and ICU's time zones, in `tz.bin`. Tables made from icu_calendar 2.2.1 and timezone_provider 0.2.3, in `tz.bin`. `internal/eastasian`, ported from icu_calendar 2.2.1 | LICENSE, NOTICE-ICU, NOTICE-ICU4X, NOTICE-TEMPORAL |
| `internal/gointl/date` | V8's Date: the offset cache, the parser and the strings. It imports `internal/gointl` itself | The four above and NOTICE-V8 |
| `internal/gointl/temporal` | temporal_rs, temporal_capi, timezone_provider, ixdtf, zoneinfo64, icu_locale_core, icu_calendar and calendrical_calculations, and V8's code where its comments say so. It imports `internal/gointl` itself | The four above and NOTICE-V8 |

A program reaches `internal/gointl` only through `internal/gointl` itself, `date` or `temporal`, the last two import `internal/gointl` itself, and that package embeds `core.bin`. So every program that links `internal/gointl` needs the first four.

## Go modules a program links

A program that imports `github.com/liustack/quickjs` (or its `lib`, `intl`, `temporal` or `fmt` packages) also links these modules. Which ones depends on the target: `go list -deps` for each target this module builds for gives the column below. Each license was read from the module's files at the version in `go.mod`.

| Module | Version | Linked on | License |
| --- | --- | --- | --- |
| modernc.org/libc | v1.77.1 | Every target | BSD-3-Clause. Its `LICENSE-3RD-PARTY.md` adds parts of Go (BSD-3-Clause), musl libc (MIT, with parts under their own terms: TRE under a 2-clause BSD license, the ARM memcpy of Bionic libc under a 2-clause BSD license, David Burren's DES under a BSD license, and others it names), go-netdb (MIT) and NixOS/nixpkgs (MIT) |
| modernc.org/mathutil | v1.7.1 | Every target | BSD-3-Clause |
| modernc.org/memory | v1.12.1 | Every target | BSD-3-Clause, with `LICENSE-GO` (BSD-3-Clause) and `LICENSE-MMAP-GO` (BSD-3-Clause) |
| golang.org/x/sys | v0.48.0 | Every target | BSD-3-Clause |
| github.com/google/uuid | v1.6.0 | Every target but windows | BSD-3-Clause |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | Every target but linux/loong64 and linux/riscv64 | BSD-3-Clause |
| github.com/dustin/go-humanize | v1.0.1 | Every target | MIT |
| github.com/mattn/go-isatty | v0.0.24 | darwin, freebsd and windows, not linux | MIT |
| github.com/ncruces/go-strftime | v1.0.0 | darwin, freebsd and windows, not linux | MIT |

The other requirements of `go.mod` are used only by tests and tools (`internal/fixlib`, `generator.go`, `lib/generator.go`): golang.org/x/mod, x/sync and x/tools, modernc.org/ccgo/v4, cc/v4, gc/v2, gc/v3, fileutil, opt, sortutil, strutil and token (all BSD-3-Clause), and github.com/hashicorp/golang-lru/v2 (MPL-2.0). `compare/` is a module of its own, and no program that uses this module links it. Its benchmarks link, besides this module and the modules above: github.com/dop251/goja (MIT), github.com/fastschema/qjs v0.0.6 (MIT, with `qjs.wasm`, QuickJS built for WebAssembly, which its README credits to QuickJS and QuickJS-NG), github.com/dlclark/regexp2/v2 (MIT, with an `ATTRIB` file for code ported from .NET), github.com/go-sourcemap/sourcemap (BSD-2-Clause), github.com/google/pprof (Apache-2.0), github.com/tetratelabs/wazero (Apache-2.0, with a `NOTICE` file) and golang.org/x/text (BSD-3-Clause), at the versions in `compare/go.mod`.

None of the code a program links is under a copyleft license. The two MPL-2.0 parts, `internal/arewefastyet/` and golang-lru, are used only by tests and tools: `internal/arewefastyet/` is read at run time by `TestMemgrind`, which builds only with the `libc.memgrind` tag, and golang-lru comes in only through ccgo, which the tests of `lib/` and upstream's `lib/generator.go` use.

## What a program that ships a binary must include

A program built with this module and given to others as a binary has to reproduce these notices in its documentation or in other materials that come with it, such as a licenses page or a `NOTICE` file:

- **BSD-3-Clause** says that a redistribution in binary form must reproduce the copyright notice, the list of conditions and the disclaimer. Include the texts of [LICENSE](LICENSE) and [lib/LICENSE](lib/LICENSE), and the BSD-3-Clause license files of the modules the target links (see the table above): modernc.org/mathutil, modernc.org/memory (`LICENSE`, `LICENSE-GO`, `LICENSE-MMAP-GO`), golang.org/x/sys, github.com/google/uuid and github.com/remyoudompheng/bigfft.
- **modernc.org/libc**, on every target: include its `LICENSE` and its `LICENSE-3RD-PARTY.md` whole. The second holds the notices of the parts of Go, musl libc, go-netdb and NixOS/nixpkgs it contains, and musl's own list of the code under other terms, such as TRE and the ARM memcpy of Bionic libc (2-clause BSD) and David Burren's DES (BSD).
- **MIT** says that the copyright notice and the permission notice must be included in all copies or substantial portions of the software. Include [LICENSE-MIT](LICENSE-MIT) (this fork), [LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS) (QuickJS), and the licenses of go-humanize, and of go-isatty and go-strftime on the targets that link them.
- **If the program imports `intl`**, it embeds the locale data, and the Unicode License V3 says its copyright and permission notice must appear with all copies of the data or in the documentation. Include [intl/LICENSE-DATA](intl/LICENSE-DATA), which holds that notice and the notices of ICU's data and Chromium's changes to it.
- **`internal/gointl`** is linked by `intl`, which imports `internal/gointl` and `internal/gointl/temporal`. A program that imports `intl` has to include, in the same way, [internal/gointl/LICENSE](internal/gointl/LICENSE), and from `internal/gointl/qjsdata/` [NOTICE-ICU](internal/gointl/qjsdata/NOTICE-ICU), [NOTICE-ICU4X](internal/gointl/qjsdata/NOTICE-ICU4X) and [NOTICE-TEMPORAL](internal/gointl/qjsdata/NOTICE-TEMPORAL), and [NOTICE-V8](internal/gointl/qjsdata/NOTICE-V8), which every program that imports `intl` links through `internal/gointl/temporal` (see [the table above](#code-and-data-that-go-intl-took-from-other-projects)). Apache-2.0, under which calendrical_calculations comes, asks that recipients get a copy of the license text, which NOTICE-ICU4X holds. A build with the `quickjs_intl_nocjdict` tag leaves the Chinese and Japanese dictionary out, but `core.bin` still holds the other four, so the whole of NOTICE-ICU is still needed.
- **If the program imports `temporal`**, it imports `intl` too, and needs the notices of the two items above. The data and code it takes from go-intl are covered by the same notices: ICU's zones, the tables of icu_calendar and timezone_provider, and the ports of temporal_rs, ICU4X's crates and V8.
- **Go itself.** The Go toolchain links the Go standard library and runtime into every Go program, under Go's own BSD-3-Clause license, which is separate from this module. Its binary clause applies as well: include Go's `LICENSE`, which is in the root of the Go distribution (`$(go env GOROOT)/LICENSE`).
