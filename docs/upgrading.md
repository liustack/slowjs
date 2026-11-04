---
summary: 'How to take upstream changes: the remote, taking a commit of upstream QuickJS into engine/ one at a time with its test and the generated files, test262 and its pin, moving to a newer Go, new CLDR, tzdb or Chromium data for intl, a new release of go-intl in internal/gointl, and a new tz update for its packs'
read_when:
  - Setting up a fresh clone that will follow upstream QuickJS
  - Taking a commit or a release of upstream QuickJS into engine/
  - Moving intl to a new CLDR, tzdb or Chromium release
  - Moving internal/gointl to a new go-intl release, or changing qjs.patch, qjs-sync.sh or the packs in qjsdata/
  - Moving Temporal's time zones to a new tz release (internal/gointl/qjstz)
  - Building or testing with a newer Go than 1.27.2, or moving the checked-in assembly of the C engine to one
---

# Upgrading from upstream

## Branches and remotes

`main` is the branch of slowjs.

Set up the remote once in a fresh clone. It is Fabrice Bellard's repository, where the C comes from:

```sh
git remote add quickjs https://github.com/bellard/quickjs.git
git fetch quickjs
```

Do the upgrade on a branch (`git switch -c upgrade-quickjs-<date>`) and merge it into `main` as [commit.md](commit.md#branches-and-merges) says.

## Taking upstream QuickJS into engine/

`engine/` is QuickJS at `3d5e064` (2026-06-04) with slowjs's changes, and it changes on its own. It does not follow upstream release by release. A commit of upstream that fixes a wrong answer, a leak or a crash is taken, whether or not a page of tifa can reach it, and so is one that makes tifa's renders faster. A new feature or a change for speed is taken when tifa has a use for it. Each one is a commit of its own. [changes/upstream-commits.md](changes/upstream-commits.md) lists the ones taken, the ones tried and taken back, and the ones left out.

For each commit:

1. `git fetch quickjs`, then see what is new: `git log --oneline 3d5e064..quickjs/master -- quickjs.c quickjs.h libregexp.c libunicode.c cutils.c dtoa.c`.
2. Apply its diff to the C tree: `git diff <commit>^ <commit> -- quickjs.c | git apply --directory=engine`. Where it meets a change of slowjs ([changes](changes/README.md) names the functions), slowjs's change stays and the upstream text is merged around it. Leave out hunks that edit things `engine/` does not have (the shell library, `fuzz/`, the Makefile).
3. Write a test that fails without the commit and passes with it, in `engine/test/` (a script of `engine/test/compare/` with its output, or a check in `unit.c`, `unit-oom.c` or `unit-white.c`), as [testing.md](testing.md#the-c-tree-and-its-native-test-host) describes.
4. Run `make check-fast`, make the generated engine again for both architectures (`make gen`, `make gen ARCH=arm64`, see [testing.md](testing.md#generate-and-check)) and commit the C, the test and the generated files together, with the upstream commit named in the message.
5. At the end of a batch, run `make check-full`. The known test262 errors (33) must stay the same, line for line, unless the commit fixed some. Then update `engine/test/test262_errors.txt` and say so.
6. Measure it as [performance.md](performance.md#how-the-numbers-were-measured) says, add a row to [changes/upstream-commits.md](changes/upstream-commits.md), and a line to both changelogs when users can see it.

To move test262 to a newer revision, change the commit and the SHA-256 at the top of `engine/test/test262.sh`, check that `engine/test/test262.diff` (upstream QuickJS's patch to test262) still applies, run `engine/test/test262.sh`, and replace `engine/test/test262_errors.txt` with the errors it reports after reading each new one.

## Moving to a newer Go

The Go assembly of the C engine (`internal/cengine`, checked in) was made with Go 1.27.2 and checked with Go 1.26 and 1.27, on linux/amd64 and darwin/arm64. `go.mod` asks for 1.26.6 or later, and a newer Go stops the build with the error of `internal/goversion` (its file `newer.go` has the build line `go1.28 && !quickjs_anygo`), since the assembly was never checked against it. `-tags quickjs_anygo` builds anyway. The checks of the generator and its tests depend on these things of Go 1.27.2:

- The stack check at the start of every function that is not NOSPLIT: a compare of the stack pointer with the guard in the goroutine, `g.stackguard0` (offset 16 of `g`, with `g` in R14 on amd64 and X28 on arm64), a branch to a call of `runtime.morestack_noctxt.abi0`, and a jump back to the start (`internal/goasm/gen/verify_insn.go`, `arm64_verify.go`).
- Which functions are NOSPLIT: the generator chooses none, and marks only the register-entry leaves and the leaves the Go assembler marks NOSPLIT by itself, a function that calls nothing and has a frame under 128 bytes with the saved base pointer (`internal/goasm/gen/nosplit.go`). In the engine none of them calls anything, so a fault in one has the room the stack check of its caller left, at least 800 bytes less its frame, and needs 16 bytes of it on amd64 and 24 on arm64 for the call of `runtime.sigpanic` and its call of morestack. If the assembler marks other leaves by itself, or stops marking these, the post-link check fails.
- The preemption hammer (`internal/cengine/hammer_test.go`) reads `g` and writes its stack guard.

When Go changes any of these, a check of the generator or a test fails. A program is not checked when it is built, which is why a newer Go stops the build. So, before you move to a newer Go, run these with it, each with `-tags quickjs_anygo` added to its tags:

1. Run `go test -p 1 -tags testhooks,quickjs_anygo ./internal/cengine .`, and the hammer (`CENGINE_HAMMER_N=3000 go test -tags cenginehammer,quickjs_anygo -run Hammer ./internal/cengine`). The fault tests (`TestFault*`, `TestFaultNoSplit`) are the ones that see a changed signal path.
2. Run the generator's tests with the new Go (`CHECK_TAGS=quickjs_anygo make check-fast`, its `goasm` step. `GOFLAGS=-tags=...` alone does not do: a command that gives its own `-tags` replaces it, and the gate stops it). They build sample programs, link them, and check the stack check, the call of `runtime.morestack_noctxt.abi0` and which functions are NOSPLIT in the linked binary, so a changed shape fails there. Then check the engine itself the same way, with the post-link check of [testing.md](testing.md#the-go-host-of-the-generated-engine) (`-verify-bin`, on a test binary built with `-tags goasmkeep,quickjs_anygo`). `make check-gen` does not test the Go version: it only checks that the files checked in are what the generator makes.
3. If the fault tests fail, read the source of `runtime.sigpanic` and of the stack check of the new release to find out why. If the assembler marks other functions NOSPLIT by itself, change `autoNoSplit` in `internal/goasm/gen/nosplit.go` to what you read there and make the engine again.
4. When all pass, move the build line of `internal/goversion/newer.go` to the next version and the name in it to the version you checked, and write the new version here and in the README, in both languages. `TestNewerGoStops` checks that the line and the name agree.

Three build options are not supported, and the build stops with an error at a line of `internal/cengine/goasm_tripwire.<arch>.s`: `-asmflags=-spectre=ret` and `-spectre=all` on amd64, and `-dynlink`, which the go command passes for `-buildmode=plugin`, `-buildmode=shared` and `-linkshared`. The generated engine writes each function body as bytes with the branch offsets fixed and leaves its calls and loads of symbol addresses to the assembler. These options make the assembler encode those with other lengths (a 5 byte call through a retpoline instead of `CALL AX`, two instructions instead of a `LEAQ` of a symbol), so the branches land wrong and the first `NewVM` crashes though the build succeeds. The tripwire is a function that is never linked and assembles only when the assembler keeps the instructions as written. When you move to a newer Go, build with these options again and check that the tripwire still refuses them (`TestEngineAsmOptions`, `TestEngineBuildmodeShared`), and that `-buildmode=pie` still gives an engine that runs: the linux/amd64 assembler adds 2 bytes to the stack check of each function with `-shared`, which is harmless. Add a new option to the tripwire when one changes the length of an instruction the engine leaves to the assembler.

The lowest Go is 1.26.6, the `go` line of `go.mod`. On Go 1.26 the things above are as in 1.27.2: the stack the linker allows, the stack check, what a fault pushes and runs, the ABI wrappers and the rules of async preemption. The generator's tests, the post-link checks of both architectures and the tests of the engine pass with 1.26.0 and 1.26.10. Two things of Go set the line above 1.26.0. The compiler of 1.26.0 and 1.26.1 fails on a test file of `internal/hostmem` (golang/go#77832, fixed in 1.26.2). And before 1.26.6, when a goroutine's stack was copied, the frame of a function into which Go injected a call, as it injects `sigpanic` into the engine function that faulted, kept a frame pointer into the old stack, which the execution tracer and the block and mutex profiles may follow (`adjustframe` in `runtime/stack.go`, fixed in 1.26.6). `scripts/go-versions.sh` runs the tests with the lowest Go and the newest release of each minor version. To lower the line, run the steps above with the older Go, and read what its release notes and the files above changed since.

## Updating the intl locale data

All Intl constructors use the go-intl packs below. `intl/cldr.bin`, its generator and the host-zone DST table have been removed. Builds and tests need no network or browser.

The pack generator reads `intl/testdata/chromium/icu.json.gz`. It records ICU resource inheritance, raw calendar and zone strings, non-Latin number symbols and the RBNF bundles Chromium ships. The generator applies those choices to go-intl's models and encodes one shared pool for each data set. Missing RBNF data follows ICU's style-construction failure path. It does not substitute an invented value.

To update the reference browser, use a cached Chromium executable and Playwright installation. Set `CHROMIUM` and `PLAYWRIGHT` for the commands below:

```sh
node intl/testdata/chromium/record.cjs icudtl
go run -p 2 -tags gointl_walk ./internal/gointl/internal/qjspack/gen -icudtl /path/to/icudtl.dat
internal/gointl/qjs-sync.sh chromium
internal/gointl/qjs-sync.sh pack
node intl/testdata/chromium/record.cjs golden
node intl/testdata/chromium/record.cjs probes migration
go test -short -p 2 ./intl/... ./temporal/...
```

Record each other golden set with `record.cjs golden <set>`, as the recorder's header lists. Chromium 156.0.8078.4 uses ICU commit `5aa526207171ecadf31597f0980a7b7ad8872f33`. The pack tool now owns the ICU parser formerly in `intl/internal/gen`.

Keep the raw browser answers. Explain every changed expression before updating expectations. The three Winnipeg cases in `TestGoIntlMigration` name exact tz 2026e answers because the reference browser has older zones.

## Updating go-intl (`internal/gointl`)

`internal/gointl` is a copy of [go-intl](https://github.com/go-quickjs/go-intl), a pure Go ECMA-402 and Temporal library, at the release and commit in `internal/gointl/UPSTREAM` (v0.6.1). Package `intl` uses it for every constructor, Date and Temporal formatting, and date ranges. Package `temporal` computes with its temporal package and reads its zones and calendars. It is internal because its API is go-intl's and changes between releases, and it sits at the module root so that `intl/` and `temporal/` can import it.

The files and directories named `qjs*`, and `UPSTREAM`, are ours. `qjs_temporalformat.go` keeps unchanged Temporal patterns as Chromium 156 does, exposes the actual date pattern for registry keys and honors an offset supplied by Temporal. Rerun the focused formatting corpora after updating it. Everything else is go-intl's, with its import path rewritten, and is changed only by `internal/gointl/qjs.patch`, which covers data loading, Chromium style construction and notices: `source.go` serves our packs instead of go-intl's 18.9 MB `data.pack`, `internal/blob` tells the pack tool which pool parts a locale reads, `dictbe.go` takes a missing break dictionary for none, the loaders of the data in the readers' packs below call the function that links their pack, and `temporal/solar.go` and `temporal/zoneinfo.go` start with a header saying that parts are ported from calendrical_calculations and changed, which its Apache-2.0 license asks of a changed file. A file of a new go-intl release that ports from an Apache-2.0 project gets the same header through `qjs.patch`.

The data is in `internal/gointl/qjsdata/`, written by `internal/gointl/internal/qjspack/gen` from go-intl's `data/` directory:

- `core.bin` (3.39 MB), everything but those below.
- `names.bin` (1.9 MB), the display names. Build with `-tags quickjs_intl_nonames` to leave it out: `Intl.DisplayNames` then has no names, and `of` answers every code with its fallback, the code or `undefined` with `fallback: 'none'`.
- `cjdict.bin` (2.0 MB), the Chinese and Japanese word dictionary. Build with `-tags quickjs_intl_nocjdict` to leave it out: a run of Chinese or Japanese is then one word, as in an ICU without the dictionary.
- `tz.bin` (92 KB), what package `temporal` reads: ICU's time zones, the zone names Temporal takes, the tables of the Chinese, Korean and Umm al-Qura calendars, and `tzversion.bin`, the tz release of the zones. `Embedded` serves it with the rest, and `EmbeddedTemporalSource` serves it alone. The two share one copy: a chunk read through either is inflated once.
- The readers' packs, each linked only into a binary that links a function reading it (`qjs_readers.go`): `dates.bin` (677 KB), what go-intl's `DateTimeFormat` reads, `zonenames.bin` (1.27 MB), the zone names, and `hostzones.bin` (6 KB), the tables of the guess at the host's zone. Package `intl` uses these packs for Date and Temporal values and ranges. It links the date and zone-name packs, but not the host-zone pack (`TestLinksOnlyReadData`). Each loader of their data calls `qjsUseDates`, `qjsUseZoneNames` or `qjsUseHostZones` first, through `qjs.patch`, and `TestQJSReadersUsePacks` fails when a function of a new go-intl release reads one of their data sets without that call: add it to the patch. A data set a new release adds for a new reader gets a pack of its own the same way, in `packOf` of the pack tool, a variable in `qjs_readers.go` and the calls in its loaders.

The locales are those of Chromium 156. `qjsdata/chromium.json` records which locale Chromium resolves each of go-intl's available locales to, for every `Intl` constructor, and each service here has the locales Chromium resolves to themselves: `TestQJSResolveMatchesChromium` checks that every service resolves every one of those tags as Chromium does. A language's data is kept when Chromium has the bare language for a service, with the languages it inherits from, and its plural rules when Chromium has it for `PluralRules`. Chromium has some locales whose language it has no data for, such as `ha-GH` without `ha`: they format with the root's data there, and here. The pack tool prints the languages it drops and those it keeps the plural rules of only.

`qjsnumbers.bin` marks number resources that the pack generator has already resolved. Raw go-intl data has no marker and keeps its original ICU alias lookup. Unknown marker versions fail when read.

The shared pools of strings, the collation root and the break dictionaries are kept uncompressed, because a formatter reads only a little of each and the pages of the binary it does not touch stay on disk. The rest is DEFLATE in chunks, and a chunk is inflated into the heap, once per process, the first time a formatter reads a file in it. `QJSInflated` reports how much that is.

The bytes a formatter reads are never to be written: an uncompressed chunk is the binary's read-only data, where a write faults, and an inflated one is shared by every formatter and VM in the process. Code that hands such bytes to JS, as an `ArrayBuffer` or the like, copies them first.

`make gointl` runs the checks `go test ./...` does not: the pack tool's tests, which need the `gointl_walk` build tag, the build without the optional packs, and that `qjsdata/` is what go-intl's data gives with the time zones below.

### Time zones

go-intl v0.6.1 makes its zones from tz 2026c. The packs carry those of a later update, the one `internal/gointl/qjstz` names (2026e now), so that a zone that changed its rules since gives what browsers on the newer data give. `qjs-sync.sh checkout` downloads ICU's update for that release from the icu-data repository (`tzdata/icunew/<version>/44`: `zoneinfo64.txt`, `timezoneTypes.txt`, `windowsZones.txt`, `metaZones.txt`) and ICU's sources, checks their sha256 against `qjstz`, and runs go-intl's own `tzgen` on them in a copy of the checkout, `$GOINTL_UPSTREAM-tz<version>`. Only `data/tz/`, `data/windowszones.bin` and `data/abbreviationzones.bin` change, and the copy gets `data/tzversion.bin`, which goes into `tz.bin`. `pack` reads that copy's `data/`. `test` makes its packs of go-intl's data as it is, zones included, since go-intl's corpora hold Node's answers at go-intl's tz release. `test-chromium` runs the fork's tests against the shipped packs and recorded browser answers. Between 2026c and 2026e those tests differ in Bogota (1992), Tehran (1979), Dublin (1925), Inuvik, Winnipeg and Rainy_River, and EST5EDT, CST6CDT, MST7MDT and PST8PDT, which 2026d made zones of their own: all changes the tz NEWS file lists. Zone names and metazones use go-intl's models with Chromium's resource choices. `TestQJSTZVersion` checks the release `tz.bin` holds.

To move to a newer update, once ICU has published it:

1. Set `TZ_VERSION` in `qjstz`, and the four checksums, from `sha256sum` of the four files under `tzdata/icunew/<version>/44`.
2. Run `qjs-sync.sh checkout`, then `qjs-sync.sh pack`.
3. Change the version in `TestQJSTZVersion`, in [temporal.md](changes/temporal.md#zone-data) and in the package doc of `temporal`, and add a case to `TestZoneData` in `temporal/all_test.go` for a zone the update changed.
4. Run `qjs-sync.sh test -p 2`, `qjs-sync.sh test-chromium -p 2`, `go test ./internal/gointl/... ./temporal` and `make gointl`.

When go-intl itself moves to that update or a later one, `checkout` still writes the zones of `qjstz`. Set `qjstz` to go-intl's release then, or later, never earlier.

To move to a new go-intl release, set `VERSION` and `COMMIT` in `internal/gointl/UPSTREAM`, then:

```sh
internal/gointl/qjs-sync.sh checkout       # clones go-intl into $GOINTL_UPSTREAM (default ~/.cache/quickjs-gointl/go-intl)
internal/gointl/qjs-sync.sh update         # copies its runtime sources here and applies qjs.patch
internal/gointl/qjs-sync.sh chromium       # records what Chromium resolves the new tags to
internal/gointl/qjs-sync.sh pack           # writes qjsdata/ again
internal/gointl/qjs-sync.sh test -p 2      # go-intl's own tests, on packs of every language
internal/gointl/qjs-sync.sh test-chromium -p 2  # slowjs's Chromium corpora on the shipped packs
go test ./internal/gointl/... && make gointl
```

`qjsdata/NOTICE-ICU` holds ICU's notices for the ICU data and word break dictionaries in the packs: lines 1 to 401 of ICU's `LICENSE` at the tag `release-78.3`, after a short note of ours. `update` copies go-intl's `LICENSE` but not this file. When go-intl takes its data from a new ICU release (its `SOURCES.md` names it), write the file again from ICU's `LICENSE` at that release's tag, with `gh api 'repos/unicode-org/icu/contents/LICENSE?ref=release-<version>'`: the same sections, unchanged, with the version and line numbers in the note updated, and add the notice of any dictionary or data the packs take that was not there before. The other `qjsdata/NOTICE-*` files hold the notices of what go-intl ported from V8 (`NOTICE-V8`) and from Temporal's crates (`NOTICE-TEMPORAL` for boa-dev's, `NOTICE-ICU4X` for ICU4X's), at the versions in its `SOURCES.md`. When a release of go-intl moves one of those versions, or ports from a project not listed there, copy that project's license, and its `NOTICE` file if it has one, at the new version (a crate's from `https://static.crates.io/crates/<name>/<name>-<version>.crate`, V8's from `deps/v8/LICENSE` of the Node release), update the note at the top and the tables in [NOTICE.md](../NOTICE.md#code-and-data-that-go-intl-took-from-other-projects).

`chromium` needs Node and the Chromium that Playwright drives, set up as for the intl golden test in [Updating the intl locale data](#updating-the-intl-locale-data) (`PLAYWRIGHT`, and `CHROMIUM` for another browser). Run it again for a newer Chromium too. Where `qjs.patch` no longer applies, make the change by hand and run `qjs-sync.sh patch` to write the patch again. When `pack` fails with parts of a pool that no locale reaches, a data package has started reading something its `qjs_walk.go` does not: make that walk read it. `test` leaves out `pack_test.go` and `TestGeneratedTablesAreWellFormed`, which read go-intl's `data.pack` and `data/`. `test-chromium` runs the checked-in go-intl, Intl and Temporal tests. Chromium removes individual resources as well as locales, so filtering locales out of Node's corpora cannot turn them into browser expectations. Keep the full Node checks in `test`, and use the recorded Chromium corpora for the shipped packs. `test-chromium` does not need an upstream checkout or a browser installed.
