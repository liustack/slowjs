---
summary: 'How to take a new upstream release: remotes and branches, merging a libquickjs release with git subtree, putting the fork changes back with fixlib, the order in which to update its lists, which doc to read for each report, the C patch, a new release of the Go API, a new modernc.org/libc, and new CLDR, tzdb or Chromium data for intl'
read_when:
  - Setting up a fresh clone that will follow upstream
  - Taking a new release of libquickjs, modernc.org/quickjs or modernc.org/libc
  - go run ./internal/fixlib or its -check reports something after an upstream merge
  - Changing lib/internal/patch/quickjs.diff
  - Moving intl to a new CLDR, tzdb or Chromium release
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

`lib/ccgo_*.go`: every upstream release regenerates these files, so the two lines added to `XJS_ToCStringLen2`, the calls into `lib/backtrace.go` (lazy error stacks), the `getTimezoneOffset` that answers 0 (local time is UTC) and the calls into `lib/uncatchable.go` (uncatchable exceptions) and `lib/outofmemory.go` (out of memory), the compiler's out of memory checks, the stack limit of a new runtime, the call paths without `alloca`, the functions without a TLS frame, the changes to the interpreter loop, the call into `lib/objlit.go`, the call into `lib/regexskip.go`, the cold paths moved out, the TLS frames taken per statement, `set_value` written out in the interpreter, the interrupt countdown, the front cache, the shape cache, the frame of `JS_CallInternal` taken once on linux and the frames freed without a defer conflict. Take the upstream version of every generated file, then put the changes back:

```sh
git checkout --theirs -- 'lib/ccgo_*.go' 'lib/internal/run-test262/ccgo_*.go'
go run ./internal/fixlib
```

`fixlib` also rewrites `modernc.org/libquickjs` imports in hand-written files under `lib/`. `go run ./internal/fixlib -check` lists files that still need changes.

### The order of the reports

Some steps check the code against a list kept in `internal/fixlib`, and a flag prints the list as the files have it. A report fails while a list it depends on is out of date, so update them in this order: `-frames`, then `-up` (on the files as taken from upstream), then `-cells`, then `-slots`. Then fix `lazySites` (`internal/fixlib/lazy.go`) and `inlineCallees` (`internal/fixlib/inline.go`) by hand from the numbers `fixlib` reports: no flag prints them. Last come the fingerprints, `-define`, `-backtrace`, `-regex`, `-lazyaddr`, `-inline`, `-poll`, `-runtime`, `-front`, `-shape` and `-oneframe`. Each of these runs every change of `fixlib` and skips only the fingerprint checks, so a report that stops on another list, such as "TLS frames taken by statement changed", means that list is out of date, not the one the flag prints. Read every new line before you put it in.

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
| `Xlre_compile`, `Xlre_exec`, `_lre_exec_backtrack` (`-regex`) | [regexp.md](fork-changes/regexp.md#skip-to-where-a-regexp-can-match) |

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

The calls without `alloca`, the functions without a TLS frame, the changes to the interpreter loop, the calls into `lib/objlit.go` and `lib/regexskip.go`, the cold paths moved out, the frames taken by statement, the calls written out in the interpreter, the interrupt countdown, the front cache, the shape cache, the frame of `JS_CallInternal` taken once and the frames freed without a defer have no C form: they are changes to what ccgo generates, so `fixlib` applies them after every regeneration.

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
node intl/testdata/chromium/record.cjs golden                  # writes golden.json.gz
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
