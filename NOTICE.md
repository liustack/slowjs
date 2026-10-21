# Notices

This repository holds code and data from several sources, each under its own license. This file says which license covers which path, and which notices a program built with this module has to carry. It states what each license text says. It is not legal advice.

## Who wrote what

| Code | Written by | License | License file |
| --- | --- | --- | --- |
| Files this fork created. Its Go and shell source files carry an SPDX line. Files that carry none: the docs it wrote (`docs/`, `AGENTS.md`, `NOTICE.md`, `README.zh-CN.md`, `CHANGELOG.md`, `CHANGELOG.zh-CN.md`), its test scripts (`lib/testdata/*.js`, `testdata/*.js`, `intl/testdata/node/*.js`), `.gitattributes`, `intl/.gitignore` and `lib/internal/run-test262/go.mod` | The liustack/quickjs Authors ([AUTHORS-FORK](AUTHORS-FORK)) | MIT | [LICENSE-MIT](LICENSE-MIT) |
| 14 of those files, in `lib/`, repeat parts of the engine as ccgo translated it: `atomkey.go`, `backtrace.go`, `backtrace32.go`, `backtrace64.go`, `bitwise.go`, `forin.go`, `frontcache.go`, `funcshape.go`, `jsonstr.go`, `objlit.go`, `outofmemory.go`, `poll.go`, `regexskip.go`, `shapecache.go`. Their SPDX line is `MIT AND BSD-3-Clause`, and their header names QuickJS and modernc.org/libquickjs | The liustack/quickjs Authors. The parts that follow the engine: the libquickjs-go Authors, from QuickJS by Fabrice Bellard and Charlie Gordon | MIT for the fork's code. BSD-3-Clause and MIT (QuickJS) for the parts that follow the engine | [LICENSE-MIT](LICENSE-MIT), [lib/LICENSE](lib/LICENSE), [LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS) |
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
| `intl/tzdst_table.go`, `intl/testdata/tzdata/` | The IANA tz database, release 2026d | Public domain | None needed |
| `internal/gointl/qjsdata/` (`core.bin`, `names.bin`, `cjdict.bin`, `chromium.json`) | Made from go-intl's data, which comes from Unicode CLDR 48.2, ICU 78.3 and the IANA tz database, as [internal/gointl/SOURCES.md](internal/gointl/SOURCES.md) lists. The locale lists come from Chromium 156 | Unicode License V3 | [internal/gointl/LICENSE](internal/gointl/LICENSE) |
| `intl/testdata/chromium/golden.json.gz`, `intl/testdata/node/`, `lib/testdata/*.txt`, `testdata/*.golden` | What Chromium, Node and the engine print for the tests of this fork | Test data, written for this fork | None |

## Go modules a program links

A program that imports `github.com/liustack/quickjs` (or its `lib`, `intl` or `fmt` packages) also links these modules. Which ones depends on the target: `go list -deps` for each target this module builds for gives the column below. Each license was read from the module's files at the version in `go.mod`.

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

The other requirements of `go.mod` are used only by tests and tools (`internal/fixlib`, `generator.go`, `lib/generator.go`): golang.org/x/mod, x/sync and x/tools, modernc.org/ccgo/v4, cc/v4, gc/v2, gc/v3, fileutil, opt, sortutil, strutil and token (all BSD-3-Clause), and github.com/hashicorp/golang-lru/v2 (MPL-2.0). `compare/` is a module of its own with goja, fastschema/qjs and their dependencies, and no program that uses this module links it.

None of the code a program links is under a copyleft license. The two MPL-2.0 parts, `internal/arewefastyet/` and golang-lru, are used only by tests and tools: `internal/arewefastyet/` is read at run time by `TestMemgrind`, which builds only with the `libc.memgrind` tag, and golang-lru comes in only through ccgo, which the tests of `lib/` and upstream's `lib/generator.go` use.

## What a program that ships a binary must include

A program built with this module and given to others as a binary has to reproduce these notices in its documentation or in other materials that come with it, such as a licenses page or a `NOTICE` file:

- **BSD-3-Clause** says that a redistribution in binary form must reproduce the copyright notice, the list of conditions and the disclaimer. Include the texts of [LICENSE](LICENSE) and [lib/LICENSE](lib/LICENSE), and the BSD-3-Clause license files of modernc.org/libc (with the Go license in its `LICENSE-3RD-PARTY.md`), modernc.org/mathutil, modernc.org/memory (`LICENSE`, `LICENSE-GO`, `LICENSE-MMAP-GO`), golang.org/x/sys, github.com/google/uuid and github.com/remyoudompheng/bigfft.
- **MIT** says that the copyright notice and the permission notice must be included in all copies or substantial portions of the software. Include [LICENSE-MIT](LICENSE-MIT) (this fork), [LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS) (QuickJS), the MIT notices of musl, go-netdb and NixOS/nixpkgs in modernc.org/libc's `LICENSE-3RD-PARTY.md`, and the licenses of go-humanize, go-isatty and go-strftime.
- **If the program imports `intl`**, it embeds the locale data, and the Unicode License V3 says its copyright and permission notice must appear with all copies of the data or in the documentation. Include [intl/LICENSE-DATA](intl/LICENSE-DATA), which holds that notice and the notices of ICU's data and Chromium's changes to it.
- **`internal/gointl`** is not linked into any program yet. Once a package of this module uses it, a program that links it has to include [internal/gointl/LICENSE](internal/gointl/LICENSE) in the same way.

The Go toolchain links the Go standard library into every Go program, under Go's own BSD-3-Clause license, which is separate from this module.
