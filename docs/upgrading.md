---
summary: 'How to take a new upstream release: remotes and branches, merging a libquickjs release with git subtree, putting the fork changes back with fixlib, the order in which to update its lists, which doc to read for each report, the C patch, a new release of the Go API, a new modernc.org/libc, new CLDR, tzdb or Chromium data for intl, and a new release of go-intl in internal/gointl, and a new tz update for its packs'
read_when:
  - Setting up a fresh clone that will follow upstream
  - Taking a new release of libquickjs, modernc.org/quickjs or modernc.org/libc
  - go run ./internal/fixlib or its -check reports something after an upstream merge
  - Changing lib/internal/patch/quickjs.diff
  - Moving intl to a new CLDR, tzdb or Chromium release
  - Moving internal/gointl to a new go-intl release, or changing qjs.patch, qjs-sync.sh or the packs in qjsdata/
  - Moving Temporal's time zones to a new tz release (internal/gointl/qjstz)
---

# Upgrading from upstream

## Branches and remotes

`main` carries the fork. `master` and the other upstream branches mirror gitlab.com/cznic/quickjs and get no commits of their own.

Set up the remotes once in a fresh clone. The engine remotes must not fetch tags, because libquickjs tags (v0.13.1, ...) would clash with the tags of this repository:

```sh
git remote add upstream https://gitlab.com/cznic/quickjs.git
git remote add upstream-lib https://gitlab.com/cznic/libquickjs.git
git config remote.upstream-lib.tagOpt --no-tags
git fetch upstream
git fetch upstream-lib 'refs/tags/*:refs/tags/lib/*'   # engine tags as lib/v0.13.2, ...
```

Do the upgrade on a branch (`git switch -c upgrade-libquickjs-<tag>`) and merge it into `main` as [commit.md](commit.md#branches-and-merges) says.

## Taking a new engine release

`lib/` was added with `git subtree`, so an upstream libquickjs release merges in with its history:

```sh
git subtree pull --prefix lib upstream-lib <libquickjs tag, e.g. v0.13.2>
```

### Conflicts to expect

`lib/ccgo_*.go`: every upstream release regenerates these files, so the two lines added to `XJS_ToCStringLen2`, the calls into `lib/backtrace.go` (lazy error stacks), the `getTimezoneOffset` that answers 0 (local time is UTC) and the calls into `lib/uncatchable.go` (uncatchable exceptions) and `lib/outofmemory.go` (out of memory), the compiler's out of memory checks, the stack limit of a new runtime, the call paths without `alloca`, the functions without a TLS frame, the changes to the interpreter loop, the call into `lib/objlit.go`, the call into `lib/regexskip.go`, the cold paths moved out, the TLS frames taken per statement, `set_value` written out in the interpreter, the interrupt countdown, the front cache, the shape cache, the call into `lib/forin.go`, the calls into `lib/bitwise.go`, the comparisons written out in the interpreter, the call into `lib/atomkey.go`, the calls into `lib/jsonstr.go`, the calls into `lib/funcshape.go`, the calls into `lib/regexcache.go`, the clock functions that call `lib/clock.go`, the frame of `JS_CallInternal` taken once on linux, the frames freed without a defer and the part of `quickjs-libc.c` that goes conflict. Take the upstream version of every generated file, then put the changes back:

```sh
git checkout --theirs -- 'lib/ccgo_*.go' 'lib/internal/run-test262/ccgo_*.go'
go run ./internal/fixlib
```

`fixlib` also rewrites `modernc.org/libquickjs` imports in hand-written files under `lib/`. `go run ./internal/fixlib -check` lists files that still need changes.

### The order of the reports

Some steps check the code against a list kept in `internal/fixlib`, and a flag prints the list as the files have it. A report fails while a list it depends on is out of date, so update them in this order: `-frames`, then `-up` and `-qjslibc` (both on the files as taken from upstream), then `-cells`, then `-slots`. Then fix `lazySites` (`internal/fixlib/lazy.go`) and `inlineCallees` (`internal/fixlib/inline.go`) by hand from the numbers `fixlib` reports: no flag prints them. Last come the fingerprints, `-define`, `-backtrace`, `-regex`, `-lazyaddr`, `-inline`, `-poll`, `-runtime`, `-front`, `-shape`, `-forin`, `-bitwise`, `-eq`, `-atomkey`, `-oneframe`, `-json`, `-funcshape`, `-recache` and `-clock`. Each of these runs every change of `fixlib` and skips only the fingerprint checks, so a report that stops on another list, such as "TLS frames taken by statement changed", means that list is out of date, not the one the flag prints. Read every new line before you put it in.

### When fixlib stops

`fixlib` fails when a generated file no longer has the shape a change expects, which means upstream changed the code the change touches and it needs a fresh look. The doc of each change says, under "When upstream changes it", what to read and what to update. Do not edit the generated code by hand: teach `fixlib` the new form instead.

| `fixlib` reports a change to | Read |
| --- | --- |
| `XJS_ToCStringLen2` | [uncatchable.md](fork-changes/uncatchable.md#no-leak-when-converting-a-string-runs-out-of-memory) |
| `XJS_Throw`, `_JS_CallInternal`, `_JS_IteratorClose`, a call of `XJS_GetException`, a store of `current_exception` | [uncatchable.md](fork-changes/uncatchable.md#interrupts-cannot-be-caught-through-promises-or-iterators) |
| `XJS_ThrowOutOfMemory`, `_JS_ThrowError2`, the regexp functions, the string table, the uses of `"out of memory"` | [uncatchable.md](fork-changes/uncatchable.md#out-of-memory-is-uncatchable) |
| `Xdbuf_claim`, `_push_scope`, `_pop_scope`, `_resolve_labels` and the rest of the compiler | [uncatchable.md](fork-changes/uncatchable.md#compiling-with-little-memory-left) |
| `_build_backtrace`, `_get_prop_string`, `_JS_GetFunctionBytecode`, `js_print_error`, `JS_ComputeMemoryUsage`, `ownFlagsCallers`, the slot audit (`-slots`) | [errors-and-stacks.md](fork-changes/errors-and-stacks.md#error-stacks-formatted-when-first-read) |
| `XJS_NewRuntime2`, `_JS_ThrowStackOverflow`, the stack check of a tokenizer, the uses of `"stack overflow"` | [errors-and-stacks.md](fork-changes/errors-and-stacks.md#endless-recursion-throws-rangeerror) |
| `_getTimezoneOffset`, libc's local time functions | [time-and-intl.md](fork-changes/time-and-intl.md#local-time-is-utc-in-every-vm) |
| `clockFuncs` (`-clock`), a use of a time function of libc outside them and `_js_atomics_wait`, a function of `clockFuncs` gone | [time-and-intl.md](fork-changes/time-and-intl.md#the-engine-reads-gos-clocks) |
| The `alloca` of the four call functions, `framesites.go` (`-frames`) | [calls-and-tls-frames.md](fork-changes/calls-and-tls-frames.md#calls-allocate-no-go-memory) |
| A defer, `tls.Alloc` or `tls.Free` that `fixNoDefer` does not know | [calls-and-tls-frames.md](fork-changes/calls-and-tls-frames.md#frames-freed-without-defer) |
| `coldSites`, `coldShapes` | [calls-and-tls-frames.md](fork-changes/calls-and-tls-frames.md#cold-paths-of--and-key-names) |
| `lazySites`, `lazyAddrSites` (`-lazyaddr`) | [calls-and-tls-frames.md](fork-changes/calls-and-tls-frames.md#tls-frames-taken-per-statement) |
| The frame of `_JS_CallInternal`, `oneFrameSites` (`-oneframe`) | [calls-and-tls-frames.md](fork-changes/calls-and-tls-frames.md#one-tls-frame-per-call-on-linux) |
| `__ccgo_up`, `upSites` (`-up`), the cells of `_JS_CallInternal` (`-cells`), `pinVars` | [interpreter-loop.md](fork-changes/interpreter-loop.md#less-stack-work-in-the-interpreter-loop) |
| `set_value`, `inlineCallees` (`-inline`) | [interpreter-loop.md](fork-changes/interpreter-loop.md#set_value-written-out-in-the-interpreter) |
| `js_poll_interrupts` (`-poll`) | [interpreter-loop.md](fork-changes/interpreter-loop.md#the-interrupt-countdown-inside-the-interpreter) |
| `JSRuntime`, `JS_NewRuntime2` (`-runtime`), `frontFuncs`, `JSMallocBlockHeader` (`-front`) | [memory-and-allocation.md](fork-changes/memory-and-allocation.md#a-front-cache-for-small-blocks) |
| `JS_DefinePropertyValue` and the define path (`-define`), `JS_NewObjectFromShape`, `objectBits` | [objects-and-shapes.md](fork-changes/objects-and-shapes.md#new-properties-of-plain-objects-skip-the-generic-path) |
| `shapeFuncs`, `JSShape` (`-shape`), `gc_phase` | [objects-and-shapes.md](fork-changes/objects-and-shapes.md#kept-shapes) |
| `js_for_in_prepare_prototype_chain_enum` and `forInSites` (`-forin`), the enumerable flag, `is_exotic` | [objects-and-shapes.md](fork-changes/objects-and-shapes.md#for-in-skips-prototypes-with-no-enumerable-property) |
| `JS_ValueToAtom`, `atomKeyFuncs` (`-atomkey`), the atom type and length of a string | [objects-and-shapes.md](fork-changes/objects-and-shapes.md#string-keys-that-are-already-atoms) |
| `jsonFuncs` (`-json`), the loops of `JS_ToQuotedString` and `json_parse_string`, `string_get` | [json.md](fork-changes/json.md#json-copies-runs-of-plain-characters) |
| `js_binary_logic_slow`, `js_shr_slow` and `bitwiseSites` (`-bitwise`) | [operators.md](fork-changes/operators.md#bitwise-operators-on-floats-skip-the-slow-path) |
| `js_strict_eq_slow`, `js_eq_slow` and `eqSites` (`-eq`), the bit fields of a string, `JS_FreeValue` | [operators.md](fork-changes/operators.md#the-usual-comparisons-answered-in-the-interpreter) |
| `Xlre_compile`, `Xlre_exec`, `_lre_exec_backtrack`, `Xlre_canonicalize`, `_lre_case_folding_entry` (`-regex`) | [regexp.md](fork-changes/regexp.md#skip-to-where-a-regexp-can-match) |
| The handler of the split ops (`reRegexSplit`, `reRegexSplitPush`, `reRegexNext`), `lre_poll_timeout` | [regexp.md](fork-changes/regexp.md#alternatives-that-cannot-match-are-not-pushed) |
| `funcShapeFuncs`, `JSShape` (`-funcshape`) | [objects-and-shapes.md](fork-changes/objects-and-shapes.md#new-functions-get-length-and-name-at-once) |
| `regexCacheFuncs` (`-recache`), the end of `js_compile_regexp` | [regexp.md](fork-changes/regexp.md#compiled-regexps-kept-per-vm) |
| `quickjsLibcSites` (`-qjslibc`), `quickjsLibcRoots`, `Xrqsort`, `__ccgo_fp` | [trimmed.md](fork-changes/trimmed.md#quickjs-libcc-keeps-only-what-the-go-api-calls) |

fixlib takes a change of its own as made only when its whole text is where it puts it. If it reports that such a text is somewhere else, is there twice, or that `_resolve_labels` has a label `out_of_memory` or part of the change, upstream has likely written part of the change itself: read the new code, then drop that part of fixlib or teach it the new place.

### After fixlib passes

1. `go run ./internal/fixlib -check` reports nothing, and running `go run ./internal/fixlib` again changes nothing.
2. Run the checks of [testing.md](testing.md#before-committing), the [focused checks](testing.md#focused-checks) and [test262](testing.md#test262): the 58 known errors must stay the same, line for line, unless the new release fixed some, in which case update `lib/test262_errors.txt`.
3. Build every target (`make build_all_targets`).
4. Check that `fixlib` makes the committed files from the new upstream files, byte for byte ([Same as upstream after fixlib](testing.md#same-as-upstream-after-fixlib)), with the new release in place of v0.13.1.
5. Release it (see [releasing.md](releasing.md)) and measure it in tifa (see [performance.md](performance.md#measuring-in-tifa)).

## The C patch

`lib/internal/patch/quickjs.diff` holds the same changes in C, so that a regeneration keeps them. Keep these hunks:

- the `JS_ToCStringLen2` hunk, unless upstream QuickJS has fixed the bug
- the `getTimezoneOffset` hunk
- the lazy stack hunks: `JSBacktrace`, the built-in `JS_CLASS_BACKTRACE` class, `js_backtrace_build` and the `js_backtrace_*` calls in `JS_GetPropertyInternal`, `JS_GetOwnPropertyInternal`, `JS_DefineProperty`, the interpreter's `GET_FIELD_INLINE`, `get_prop_string`, `js_print_error`, `js_print_object`, `JS_ComputeMemoryUsage`, the callers that only need the attributes of a property (`js_get_own_property_flags`)
- the uncatchable exception hunks: `js_uncatchable_pending`, `js_get_exception` and its callers, `JS_IsUncatchableException`, and the checks in `JS_Throw`, `JS_CallInternal` and `JS_IteratorClose`
- the out of memory hunks: `JS_ThrowOutOfMemory`, `JS_ThrowError2`, the regexp callers of `JS_ThrowOutOfMemory`
- the compiler hunks: `dbuf_claim` in `cutils.c`, `push_scope`, `pop_scope`, `js_create_function`, `resolve_variables`, `resolve_labels`, the two code moves of the parser, `js_parse_error_v`, `next_token`, the two patches in `js_parse_class`, the test removal in `js_parse_destructuring_element`, `JS_WriteObjectAtoms`, `JS_WriteObject2` and the error label of `lre_compile` in `libregexp.c`
- the stack hunks: the stack size `JS_NewRuntime2` sets, and `JS_ThrowStackOverflow`

After a regeneration that includes them, `lib/backtrace.go`, `lib/uncatchable.go`, `lib/outofmemory.go` and their wiring in `fixlib` are no longer needed.

The calls without `alloca`, the functions without a TLS frame, the changes to the interpreter loop, the calls into `lib/objlit.go` and `lib/regexskip.go`, the cold paths moved out, the frames taken by statement, the calls written out in the interpreter, the interrupt countdown, the front cache, the shape cache, the calls into `lib/forin.go`, `lib/bitwise.go`, `lib/atomkey.go`, `lib/jsonstr.go` and `lib/funcshape.go`, the comparisons written out in the interpreter, the frame of `JS_CallInternal` taken once, the frames freed without a defer and the part of `quickjs-libc.c` that goes have no C form: they are changes to what ccgo generates, so `fixlib` applies them after every regeneration.

## Other files of an engine release

- `lib/go.mod`, `lib/go.sum`: they do not exist here. When upstream changed them, delete them again (`git rm lib/go.mod lib/go.sum`) and bring any newer requirement into the root `go.mod`.
- `lib/internal/run-test262/go.mod` is ours. Upstream has none there.
- `lib/internal/run-test262/ccgo_*.go`: ccgo links a whole copy of the engine into the test262 runner, so as generated the runner tests that copy and not `lib/`. `fixlib` turns each engine API function of the runner into a call of the same function in `lib/` (`lib/test262_runner.go` exports the one ccgo leaves unexported). Take the upstream files and run `fixlib`, as for `lib/ccgo_*.go`.

This fork does not regenerate `lib/ccgo_*.go` itself. `lib/generator.go` still assumes it is the root of its own module.

## Taking a new release of the Go API

```sh
git merge <upstream tag>
```

Upstream commits import `modernc.org/libquickjs` and `modernc.org/quickjs`. After the merge, drop `modernc.org/libquickjs` from `go.mod`, run `grep -rn 'modernc.org/quickjs\|modernc.org/libquickjs' --include='*.go' .` and switch what it finds to `github.com/liustack/quickjs`. The only expected leftover is the Symbol key `"modernc.org/quickjs/intl"` in `intl/stub.js`, which is a runtime marker, not an import path.

`intl/` has been rewritten here, so an upstream change to it conflicts. Keep this fork's version and bring over only what the upstream change fixes, checked with the golden test below.

## Upgrading modernc.org/libc

The VM puts libc's TLS stack back after a panic (`tlsstack_linux.go`, `tlsstack_other.go`) and keeps a block of it outside linux (`warmTLS`), and both know how libc keeps that stack: one block per slot on linux, bytes in segments elsewhere. After a libc release:

- Run `go test -run EnginePanic .` with and without `-tags=libc.memgrind`, and `go test -run TLSWarm .` outside linux.
- Bring the new version into `go.mod`, and run `go mod tidy` in `lib/internal/run-test262` and `compare/` as well.
- In tifa, collect the engine profile again and check that several VMs still scale (`BenchmarkShopParallel`), as tifa's docs/testing.md says: libc's allocator lock is what held several VMs back before v0.26.12.

## Updating the intl locale data

`intl/cldr.bin` is generated from cldr-json and `intl/tzdst_table.go` from tzdb, and both are checked in, so neither a build nor `go test` needs the network or Node. To generate them again:

```sh
go run ./intl/internal/gen -fetch    # downloads what is missing into a cache directory
go test ./intl                       # includes the golden test against Chromium
```

The tzdb step needs `make`, `awk` and `zic` (`-zic` names another zic). It unpacks the release in `tzdbVersion` in `intl/internal/gen/tzdb.go`, after checking its SHA-256 against `tzdbSHA256`, makes `main.zi` and `rearguard.zi` with the release's Makefile and compiles both. The table lists the zones and links whose daylight saving flag differs between the two forms before 2038, with their periods in the rearguard form, which is what ICU builds from. The generator also copies a few zones in both forms to `intl/testdata/tzdata` for the tests. To move to a new release, take the one in the ICU data of the Chromium the golden test records with: the `TZVersion` of `zoneinfo64.res` in its `icudtl.dat` (see below for where to get that file). Set `tzdbVersion` and `tzdbSHA256` (the SHA-256 of `tzdata<version>.tar.gz` from https://data.iana.org/time-zones/releases/, whose signature is the `.asc` next to it) and run the two commands.

To move to a new CLDR release, set `cldrTag` in `intl/internal/gen/main.go` to the cldr-json tag and run the two commands. The golden test then shows every expression whose output changed. Where the new CLDR changed a value Chromium does not have yet, add the value Chromium has to `browserOverrides` in `intl/internal/gen/overrides.go`, with where it comes from. The generator stops on an override CLDR has caught up with, so the list only ever holds live differences.

To compare with a newer Chromium, record its answers again. This needs Node and a Chromium that Playwright drives. Set it up once:

```sh
npm install -g playwright@1.64.0                # Chromium 156.0.8078.4, as recorded here
npx playwright@1.64.0 install chromium
export PLAYWRIGHT="$(npm root -g)/playwright"   # what record.cjs loads
```

Each Playwright release drives one Chromium build, so a newer Chromium needs a newer Playwright: its `browsers.json` names the build.

Then, from the repository root:

```sh
CLDR="$HOME/cldr-json"                                         # any directory for the downloads
go run ./intl/internal/gen -fetch -cldr "$CLDR"                # the list of CLDR locales
node intl/testdata/chromium/record.cjs availability "$CLDR"    # the locales Chromium has
go run ./intl/internal/gen -icudtl "$(node intl/testdata/chromium/record.cjs icudtl)"
go run ./intl/internal/gen -fetch -cldr "$CLDR"                # data for those locales
go test ./intl -run TestGolden -args -golden.corpus            # writes corpus.json
node intl/testdata/chromium/record.cjs golden                  # writes golden.json.gz and golden-<set>.json.gz
node intl/testdata/chromium/record.cjs shape                   # writes shape.json
node intl/testdata/chromium/record.cjs tamper                  # writes tamper.json
go test ./intl
rm -rf "$CLDR"
```

`record.cjs icudtl` prints where the browser keeps `icudtl.dat`, its ICU data file. The same file, byte for byte, is `common/icudtl.dat` in Chromium's copy of ICU, at the commit Chromium's `DEPS` pins for `src/third_party/icu` (5aa52620 for 156.0.8078.4), so summarizing it needs no browser:

```sh
curl -s 'https://chromium.googlesource.com/chromium/deps/icu/+/5aa526207171ecadf31597f0980a7b7ad8872f33/common/icudtl.dat?format=TEXT' |
	base64 -d > icudtl.dat
go run ./intl/internal/gen -icudtl icudtl.dat
```

The generator writes a summary of it to `intl/testdata/chromium/icu.json.gz` and makes every locale take each resource from where Chromium's ICU finds it, so a language whose data Chromium leaves out formats with root's or English data as it does in Chromium. To record another browser, set `CHROMIUM` to its executable for every `record.cjs` command.

Update the Chromium and ICU versions in the package documentation of `intl` and in this doc, and look at each new difference: fix the code when Chromium is right, or add the cause to `goldenDifferences` when the difference is in the data.

## Updating go-intl (`internal/gointl`)

`internal/gointl` is a copy of [go-intl](https://github.com/go-quickjs/go-intl), a pure Go ECMA-402 and Temporal library, at the release and commit in `internal/gointl/UPSTREAM` (v0.6.1). Package `intl` uses it for every constructor but `DateTimeFormat`, `NumberFormat` and `PluralRules`, and package `temporal` computes with its temporal package and reads its zones and calendars. It is internal because its API is go-intl's and changes between releases, and it sits at the module root so that `intl/` and `temporal/` can import it.

The files and directories named `qjs*`, and `UPSTREAM`, are ours. Everything else is go-intl's, with its import path rewritten, and is changed only by `internal/gointl/qjs.patch`, which keeps to loading data and notices: `source.go` serves our packs instead of go-intl's 18.9 MB `data.pack`, `internal/blob` tells the pack tool which pool parts a locale reads, `dictbe.go` takes a missing break dictionary for none, the loaders of the data in the readers' packs below call the function that links their pack, and `temporal/solar.go` and `temporal/zoneinfo.go` start with a header saying that parts are ported from calendrical_calculations and changed, which its Apache-2.0 license asks of a changed file. A file of a new go-intl release that ports from an Apache-2.0 project gets the same header through `qjs.patch`.

The data is in `internal/gointl/qjsdata/`, written by `internal/gointl/internal/qjspack/gen` from go-intl's `data/` directory:

- `core.bin` (4.4 MB), everything but those below.
- `names.bin` (1.9 MB), the display names. Build with `-tags quickjs_intl_nonames` to leave it out: `Intl.DisplayNames` then has no names, and `of` answers every code with its fallback, the code or `undefined` with `fallback: 'none'`.
- `cjdict.bin` (2.0 MB), the Chinese and Japanese word dictionary. Build with `-tags quickjs_intl_nocjdict` to leave it out: a run of Chinese or Japanese is then one word, as in an ICU without the dictionary.
- `tz.bin` (92 KB), what package `temporal` reads: ICU's time zones, the zone names Temporal takes, the tables of the Chinese, Korean and Umm al-Qura calendars, and `tzversion.bin`, the tz release of the zones. `Embedded` serves it with the rest, and `EmbeddedTemporalSource` serves it alone. The two share one copy: a chunk read through either is inflated once.
- The readers' packs, each linked only into a binary that links a function reading it (`qjs_readers.go`): `dates.bin` (728 KB), what go-intl's `DateTimeFormat` reads, `zonenames.bin` (2.1 MB), the zone names, and `hostzones.bin` (6 KB), the tables of the guess at the host's zone. Package `intl` formats dates with its own `cldr.bin`, so a program that uses it links none of the three (`TestLinksOnlyReadData`). Each loader of their data calls `qjsUseDates`, `qjsUseZoneNames` or `qjsUseHostZones` first, through `qjs.patch`, and `TestQJSReadersUsePacks` fails when a function of a new go-intl release reads one of their data sets without that call: add it to the patch. A data set a new release adds for a new reader gets a pack of its own the same way, in `packOf` of the pack tool, a variable in `qjs_readers.go` and the calls in its loaders.

The locales are those of Chromium 156. `qjsdata/chromium.json` records which locale Chromium resolves each of go-intl's available locales to, for every `Intl` constructor, and each service here has the locales Chromium resolves to themselves: `TestQJSResolveMatchesChromium` checks that every service resolves every one of those tags as Chromium does. A language's data is kept when Chromium has the bare language for a service, with the languages it inherits from, and its plural rules when Chromium has it for `PluralRules`. Chromium has some locales whose language it has no data for, such as `ha-GH` without `ha`: they format with the root's data there, and here. The pack tool prints the languages it drops and those it keeps the plural rules of only.

The shared pools of strings, the collation root and the break dictionaries are kept uncompressed, because a formatter reads only a little of each and the pages of the binary it does not touch stay on disk. The rest is DEFLATE in chunks, and a chunk is inflated into the heap, once per process, the first time a formatter reads a file in it. `QJSInflated` reports how much that is.

The bytes a formatter reads are never to be written: an uncompressed chunk is the binary's read-only data, where a write faults, and an inflated one is shared by every formatter and VM in the process. Code that hands such bytes to JS, as an `ArrayBuffer` or the like, copies them first.

`make gointl` runs the checks `go test ./...` does not: the pack tool's tests, which need the `gointl_walk` build tag, the build without the optional packs, and that `qjsdata/` is what go-intl's data gives with the time zones below.

### Time zones

go-intl v0.6.1 makes its zones from tz 2026c. The packs carry those of a later update, the one `internal/gointl/qjstz` names (2026e now), so that a zone that changed its rules since gives what browsers on the newer data give. `qjs-sync.sh checkout` downloads ICU's update for that release from the icu-data repository (`tzdata/icunew/<version>/44`: `zoneinfo64.txt`, `timezoneTypes.txt`, `windowsZones.txt`, `metaZones.txt`) and ICU's sources, checks their sha256 against `qjstz`, and runs go-intl's own `tzgen` on them in a copy of the checkout, `$GOINTL_UPSTREAM-tz<version>`. Only `data/tz/`, `data/windowszones.bin` and `data/abbreviationzones.bin` change, and the copy gets `data/tzversion.bin`, which goes into `tz.bin`. `pack` and `test-chromium` read that copy's `data/`. `test` makes its packs of go-intl's data as it is, zones included, since go-intl's corpora hold Node's answers at go-intl's tz release, and `test-chromium` leaves out the three tests that compare zones with Node (`tz_skips` in the script) while the releases differ. Between 2026c and 2026e those tests differ in Bogota (1992), Tehran (1979), Dublin (1925), Inuvik, Winnipeg and Rainy_River, and EST5EDT, CST6CDT, MST7MDT and PST8PDT, which 2026d made zones of their own: all changes the tz NEWS file lists. Zone names and metazones stay those of go-intl's data. `TestQJSTZVersion` checks the release `tz.bin` holds.

To move to a newer update, once ICU has published it:

1. Set `TZ_VERSION` in `qjstz`, and the four checksums, from `sha256sum` of the four files under `tzdata/icunew/<version>/44`.
2. Run `qjs-sync.sh checkout`, then `qjs-sync.sh pack`.
3. Change the version in `TestQJSTZVersion`, in [temporal.md](fork-changes/temporal.md#zone-data) and in the package doc of `temporal`, and add a case to `TestZoneData` in `temporal/all_test.go` for a zone the update changed.
4. Run `qjs-sync.sh test -p 2`, `qjs-sync.sh test-chromium -p 2`, `go test ./internal/gointl/... ./temporal` and `make gointl`.

When go-intl itself moves to that update or a later one, `checkout` still writes the zones of `qjstz`. Set `qjstz` to go-intl's release then, or later, never earlier.

To move to a new go-intl release, set `VERSION` and `COMMIT` in `internal/gointl/UPSTREAM`, then:

```sh
internal/gointl/qjs-sync.sh checkout       # clones go-intl into $GOINTL_UPSTREAM (default ~/.cache/quickjs-gointl/go-intl)
internal/gointl/qjs-sync.sh update         # copies its runtime sources here and applies qjs.patch
internal/gointl/qjs-sync.sh chromium       # records what Chromium resolves the new tags to
internal/gointl/qjs-sync.sh pack           # writes qjsdata/ again
internal/gointl/qjs-sync.sh test -p 2      # go-intl's own tests, on packs of every language
internal/gointl/qjs-sync.sh test-chromium -p 2  # and on the packs built in
go test ./internal/gointl/... && make gointl
```

`qjsdata/NOTICE-ICU` holds ICU's notices for the ICU data and word break dictionaries in the packs: lines 1 to 401 of ICU's `LICENSE` at the tag `release-78.3`, after a short note of ours. `update` copies go-intl's `LICENSE` but not this file. When go-intl takes its data from a new ICU release (its `SOURCES.md` names it), write the file again from ICU's `LICENSE` at that release's tag, with `gh api 'repos/unicode-org/icu/contents/LICENSE?ref=release-<version>'`: the same sections, unchanged, with the version and line numbers in the note updated, and add the notice of any dictionary or data the packs take that was not there before. The other `qjsdata/NOTICE-*` files hold the notices of what go-intl ported from V8 (`NOTICE-V8`) and from Temporal's crates (`NOTICE-TEMPORAL` for boa-dev's, `NOTICE-ICU4X` for ICU4X's), at the versions in its `SOURCES.md`. When a release of go-intl moves one of those versions, or ports from a project not listed there, copy that project's license, and its `NOTICE` file if it has one, at the new version (a crate's from `https://static.crates.io/crates/<name>/<name>-<version>.crate`, V8's from `deps/v8/LICENSE` of the Node release), update the note at the top and the tables in [NOTICE.md](../NOTICE.md#code-and-data-that-go-intl-took-from-other-projects).

`chromium` needs Node and the Chromium that Playwright drives, set up as for the intl golden test in [Updating the intl locale data](#updating-the-intl-locale-data) (`PLAYWRIGHT`, and `CHROMIUM` for another browser). Run it again for a newer Chromium too. Where `qjs.patch` no longer applies, make the change by hand and run `qjs-sync.sh patch` to write the patch again. When `pack` fails with parts of a pool that no locale reaches, a data package has started reading something its `qjs_walk.go` does not: make that walk read it. `test` leaves out `pack_test.go` and `TestGeneratedTablesAreWellFormed`, which read go-intl's `data.pack` and `data/`. `test-chromium` takes out of go-intl's corpora the cases of the locales the packs leave out, and leaves out the tests that name such a locale in their code or count locales or cases (`chromium_skips` in the script).
