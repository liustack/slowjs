---
summary: 'Testing and verification: the checks to run before committing, the Makefile targets, the focused checks of each fork change, test262 and its 58 known errors, leak checks with memgrind, the race detector subset, building every target, checking that fixlib turns the upstream files into the committed ones byte for byte, and the local go.work'
read_when:
  - Getting ready to commit and checking which checks to run
  - Writing a new test or fixing a bug
  - Changing the engine, internal/fixlib or lib/internal/patch, and checking nothing else moved
  - Running test262, or test262 reports new errors
  - Looking for a leak, or changing how values are duplicated and freed
  - Changing code that several goroutines or VMs share
  - Checking that the committed lib/ccgo_*.go files are exactly what fixlib makes from upstream
  - Co-developing modernc.org/libc next to this repository with a go.work
---

# Testing and verification

## Before committing

Every change must pass:

```sh
go build ./...
gofmt -l -s . | grep -v ccgo_          # prints nothing
go vet ./internal/fixlib ./intl/... ./fmt
go run ./internal/fixlib -check        # the fork's changes are all in lib/
scripts/docs-list.sh --check           # every doc has a summary
go test -short -p 2 -timeout 30m . ./lib ./internal/fixlib
```

- `go vet` on the root package and on `lib/` reports the `unsafe.Pointer` conversions of the tests and of the generated code, so nothing runs it there. Upstream's Makefile of `lib/` passes `-vet=off` to its tests, since vet is slow on the generated files, and so do some commands below.
- `-p 2` builds at most two packages at once: the generated files are large, and building several of them together takes much memory.
- The tests of `internal/fixlib` take about 16 minutes on two hyperthreads of one AMD EPYC 7B13 core, with 20 minutes of CPU, longer than the 10 minutes `go test` gives by default, so every command that runs them passes `-timeout 30m`. On cores other work is using they take much longer, past an hour.
- A change to `intl/` also runs `go test ./intl` (the golden test is part of it).
- A change to `internal/gointl` also runs `go test ./internal/gointl/...` and `make gointl`.
- A change to `temporal/` also runs `go test ./temporal` without `-short` (the Chromium comparison and the link test are part of it), and the Temporal lines of the focused checks, the memgrind section and the race detector section.
- A change to the engine, to `internal/fixlib` or to the C patch also runs the [focused checks](#focused-checks) of what it touches, and [test262](#test262).
- On linux/amd64 these commands test the goasm interpreter, the default there. A change to the engine, to the C patch, to `internal/goasm`, to a rule of `internal/fixlib` that changes `_JS_CallInternal` or a helper it compiles in, or to the Go version also runs the [goasm checks](#the-goasm-interpreter), which test the Go interpreter of linux/amd64 too.

New behavior and bugs with a known cause: write the test first, see it fail, then write the fix. Tests live next to the code: a change to the engine is tested in `lib/` (through the C API) or in the root package (through the Go API), and a change of `fixlib` in `internal/fixlib`, which checks both the rewrite and that it stops on code it does not expect.

## Makefile targets

| Target | What it runs |
| --- | --- |
| `make editor` | `gofmt -l -s -w .`, `go test -c` of every package (compile only), `go build generator.go`, `go install ./...`, `staticcheck` |
| `make test` | `go test -failfast -v -timeout 24h -count=1 ./...` |
| `make shorttest` | The same with `-short`, which skips the long cases (test262 among them) |
| `make leak` | `go test -v -run TestMemgrind2 -tags=libc.memgrind ./...` |
| `make build_all_targets` | `./build_all_targets.sh` (see [Every target](#every-target)) |
| `make gointl` | The checks of `internal/gointl` that `go test ./...` does not run: the pack tool's tests, which need the `gointl_walk` build tag, the build without the optional packs (`-tags slowjs_intl_nonames,slowjs_intl_nocjdict`), there and for `Intl.DisplayNames` in `intl`, and that `qjsdata/` is what go-intl's data gives with the time zones of the tz update `internal/gointl/qjstz` names (this clones go-intl and downloads that update and ICU's sources) |
| `make benchmark` | `make -C compare benchmark`: the 64 micro benchmarks of QuickJS in `compare/testdata/` (58 of them, as goja or fastschema/qjs fails the other 6), each run on this engine, on goja and on fastschema/qjs (QuickJS on wazero), the suite kept from upstream |
| `make compare-engines` | `go run ./cmd/compare $(ARGS)` in `compare/engines`: the comparison with the other pure Go JavaScript engines in [benchmarks.md](benchmarks.md) (support, speed, memory, limits, tifa pages), each engine in a binary of its own |
| `make cover` | Coverage of the root package, shown as HTML |
| `make work` | Writes a `go.work` (see [go.work](#gowork)) |
| `make clean` | Deletes test binaries, profiles and `go.work` |

`staticcheck` and `golint` are upstream's linters and are not required here. `make test` without `-short` runs every package in full, test262 included, which takes long: run the parts you need instead.

To run one test or one subtest: `go test -run TestEval -v .` or `go test -run 'TestEval/eval1' -v .`.

## Focused checks

For the native C dispatch condition, apply `lib/internal/patch/quickjs.diff` to the pinned C source and inspect `DIRECT_DISPATCH` with both gcc and clang preprocessing. It must be `1` with neither translation macro, and `0` with `__CCGO__`, `__EMSCRIPTEN__`, or both. Build the native interpreter with the upstream Makefile and check interrupt and allocation failure propagation, recursion RangeError and error stack descriptors with computed goto enabled. The [native dispatch notes](fork-changes/interpreter-loop.md#native-c-keeps-computed-goto) explain why the Go translation remains unchanged.

```sh
go test -short -timeout 30m ./...                        # all packages (internal/fixlib takes about 16 minutes)
go test -vet=off -run 'TestToCString' ./lib              # the leak of JS_ToCStringLen2 out of memory
go test -run 'TestErrorStack' .                          # stack text and memory of lazy error stacks
go test -vet=off -run 'TestBacktrace' ./lib              # shapes, retries and writes of pending stacks
go run ./internal/fixlib -check                          # fork changes present in lib/
internal/gointl/qjs-sync.sh pack -check                  # shared Intl data is what the generator makes
go test -timeout 30m ./internal/fixlib                   # fixlib notices new reads of property values and new shapes of what it changes
go test -run 'CallDepth|TLSStackBalance|CallAllocs' .    # depth limits, TLS balance, no Go allocation per call
go test -run EnginePanic .                               # the TLS stack after a panic in the engine (also with -tags=libc.memgrind)
go test -run 'Regex|Canonicalize' ./lib                 # regexps give what they gave before lreSkip, and what Node gives
go test -vet=off -run 'RegexpV|RegexpShiki' ./lib        # classes of v regexps and shiki's regexps give what Node gives
go test -run ClassEscapes ./internal/fixlib              # fixClassEscapes puts its change back and stops on a changed function
go test -run 'Regex(Scan|Match)Stops' .                  # a timeout stops a regexp on a long string
go test -run ColdPath ./lib                              # ===, Object.is and key names give what they gave before fixColdFrames
go test -run ForIn ./lib                                 # for in gives what it gave before fixForIn
go test -run Bitwise ./lib                               # bitwise operators give what they gave before fixBitwise
go test -run Eq ./lib                                    # comparisons give what they gave before fixEq, and free each value once
go test -vet=off -run AtomKey ./lib . ./internal/fixlib  # string keys give the atoms and counts of the generic path
go test -vet=off -run FieldChain . ./internal/fixlib     # o.x of strings, numbers and arrays gives what Node gives
go test -run JSON ./lib                                  # JSON strings give what Node gives, and runs stay in their buffer
go test -run 'StringSearch|StringFind' ./lib ./internal/fixlib # includes, indexOf and the other searches find what a search with charCodeAt finds
go test -run FuncShape ./lib                             # new functions get what Node gives, and the shapes held go with their context
go test -vet=off -run OwnKeys . ./lib ./internal/fixlib # every listing of own keys gives what the engine gave, and the key cache drops shapes that change
go test -run 'ShapeCache|FrontCache' ./lib               # the shape cache and the front cache
go test -vet=off -run 'RegexCache|RegExpCache' ./lib .  # compiled regexps give what Node gives, and the cache keeps to its limits
go test -vet=off -run 'RegExpCacheStops|BuiltinWorkStops' . # a timeout stops loops of regexp compiles, parsing and string building within 50ms
go test -run 'RegexCompilePoll|RegexCacheCounts|Charge' ./lib # the steps the regexp parser, the regexp cache and the string builder count
go test -run 'RegexPoll|WorkCharge' ./internal/fixlib    # fixRegexPoll and fixWorkCharge put their counts back and stop on a changed site
go test -run 'RunGC|GCThreshold|GCAfterClose' .          # RunGC, MemorySize and GCThreshold (gc_test.go)
go test -run OnClose .                                   # OnClose runs once, in order, and keeps no VM alive
go test -run 'SameAsEngine|UTF8Units|BuiltinStringRoundTrip|BuiltinLatin1UTF16|EscapeView' . # strings of Builtins converted in Go give what the engine gives (also with -tags=libc.memgrind)
go test -p 2 ./temporal -run 'DateTimeFormatTemporal|DurationLocaleString|Chromium' # Temporal formatting against Chromium 156
go test -p 2 ./intl -run 'GoIntlMigration|EmbeddedZonePack|FormatStringMatchesParts' # rounding, calendars and embedded time zones
go test -p 2 ./internal/intlhook                        # private state across GC and Close
go test -run LinksOnlyReadData ./temporal               # the packs a program with Temporal links
go test -run QJSTZVersion ./internal/gointl              # tz.bin holds the zones of the update qjstz names
go test -run 'DateNow|RandomSeed|Monotonic' ./lib        # Date.now, the seed of Math.random and performance.now read Go's clocks
go test -p 2 -vet=off -run VMClock . ./lib ./temporal    # VM.SetClock: Date, Intl and Temporal share one clock, reset, Close and timeouts
go test -timeout 30m -run VMClock ./internal/fixlib      # fixVMClock puts its two calls back and stops on a moved, missing or extra one
go test -run QuickJSLibc . ./internal/fixlib            # what stays of quickjs-libc.c works, and what goes is not linked
go test -run Expect ./internal/fixlib                    # every likely and unlikely test branches on its condition
go test -run FastFrame ./lib ./internal/fixlib           # TLS frames taken and freed as libc does, on linux (lib) and in the rewrite
go test -run '^$' -bench Call .                          # the cost of each kind of call
go test -run '^$' -bench RegexColors .                   # the cost of a replace with react-spring's color names
go test -run '^$' -bench Clock .                         # the cost of Date.now, new Date() and performance.now
```

Each doc under [fork-changes](fork-changes/README.md) names the tests of its changes.

Focused weak-collection checks:

```sh
go test -p 2 -vet=off -run Ephemeron . ./lib
go test -p 2 -vet=off -tags=libc.memgrind -run Ephemeron . ./lib
go test -p 2 -timeout 120m -run Ephemeron ./internal/fixlib
go test -p 2 -vet=off -run '^$' -bench EphemeronGC -benchtime 20x .
```

Focused young collection checks (see [young-collection.md](fork-changes/young-collection.md#tests)):

```sh
go test -p 2 -vet=off -run 'YoungGC|Ephemeron' . ./lib
go test -p 2 -vet=off -tags=libc.memgrind -run 'YoungGC|Ephemeron' . ./lib
go test -p 2 -timeout 120m -run 'YoungGC|Ephemeron' ./internal/fixlib
```

Focused checks of shared source text and line tables (see [memory-and-allocation.md](fork-changes/memory-and-allocation.md#one-copy-of-source-text-and-line-tables-for-every-vm)):

```sh
go test -p 2 -vet=off -run 'SharedBytecode|SharedDebug' . ./lib
go test -p 2 -vet=off -race -run SharedBytecode .
go test -p 2 -vet=off -tags=libc.memgrind -run 'Memgrind2Shared' . ./lib
go test -p 2 -run SharedDebug ./internal/fixlib
```

## test262

`Test262` in `lib/` runs the test262 suite on the engine in `lib/`. It is skipped with `-short` and when the test262 archive is missing:

```sh
cd lib
make download                          # the pinned QuickJS and test262 archives (wget)
go test -vet=off -run Test262 -v .     # unpacks test262/, builds the runner, runs it, cleans up
```

- `lib/internal/run-test262` is a module of its own, so its 69 MiB of transpiled test runner stay out of the module zip. `Test262` builds it with `go build -C`. As ccgo generates it, the runner links a whole copy of the engine. `fixlib` turns each engine API function of the runner into a call of the same function in `lib/` (`lib/test262_runner.go` exports the one ccgo leaves unexported), so the runner tests the engine in `lib/` and the fork's changes in it.
- `lib/test262_errors.txt` lists the 58 tests known to fail, one line each (the file, the line and the error). The runner compares its results with that list: the test fails when the result line counts any `new` error. A change to the engine must leave the same 58 errors, line for line. Pass `-keep` to keep `test262/` and its report, and `-f <file>` to run one test.
- `lib/testdata/test262*.conf` holds the runner's settings per target, and `lib/testdata/test262_exclude_<goos>_<goarch>` the tests that fail only in the Go build on that target.

## Leaks: memgrind

`memgrind_test.go` and the other files with `//go:build libc.memgrind` run only with that tag, which swaps in libc's allocator that tracks every allocation. JS values are reference-counted C objects, so the leak tests are how `Dup` and `Free` pairing is checked (see the doc of `TestMemgrind`):

```sh
make leak                                             # TestMemgrind2 and the tests named after it, in every package
go test -tags=libc.memgrind -run EnginePanic .        # the TLS stack is whole after a panic
go test -vet=off -tags=libc.memgrind -run Memgrind2Objlit ./lib   # objlitDefine running out of memory
go test -tags=libc.memgrind -run Memgrind ./temporal  # Temporal leaks nothing
```

## The race detector

The engine itself has no shared state, but the registries of Go functions, the lazy setup of error stacks and intl's shared locale data are shared by every VM of the process. Run these with `-race` after touching them:

```sh
go test -race -run 'TestInterrupt|TestConcurrentVMsMemory|TestErrorStackConcurrent' .
go test -race -run TestErrorStackFirstUseRace .       # on its own, so no other test made a stack first
go test -race -run 'TestConcurrentVMs|TestRegistryConcurrent|TestRegistryGoIntlConcurrent' ./intl
go test -race -run QJS ./internal/gointl               # the packs go-intl's loaders add to Embedded
go test -race -run 'Concurrent|Tamper|Clock' ./temporal  # Temporal on VMs at once, pages changing built-ins, clocks per VM
go test -race -vet=off -run 'TestShapeCacheRuntimes|TestFrontCacheRuntimes|TestBacktraceThrowStacklessOOM' ./lib
```

The whole of `./intl` under `-race` takes about 25 minutes on one core of an EPYC, most of it the golden test: give it `-timeout 60m`.

The race detector ignores atomic operations on memory outside Go's heap, such as the interrupt flag in C memory. `Interrupt` and `Close` meet at a mutex on Go's heap for that reason (see [architecture.md](architecture.md#the-eval-timeout-and-interrupt)).

## Every target

```sh
make build_all_targets
```

It builds every package and compiles the tests of every package (not only the root), each package to a test binary of its own, as two packages share the name `gen`, for the 5 supported targets, with and without `-tags=libc.memgrind`. It takes a long time. For a quick look, build the main ones:

```sh
for t in linux/amd64 linux/arm64 darwin/arm64 windows/amd64; do
	GOOS=${t%/*} GOARCH=${t#*/} go build ./...
done
```

`internal/fixlib/macros_test.go` checks that every macro constant the generated code and the Go files next to it use is declared in each generated file. v0.26.6 and v0.26.7 did not build on linux because of one that only the darwin and windows files declared.

## The goasm interpreter

On linux/amd64 the engine runs an interpreter generated from the patched C, unless the `slowjs_nogoasm` build tag is given (see [go-assembly.md](fork-changes/go-assembly.md)). Its generated files must be exactly what the generator makes from the committed sources. Check it on linux/amd64 with LLVM 22.1.8 (`clang-22`, `clang++-22`, `opt-22`, `llc-22` and the LLVM development headers):

```sh
python3 internal/goasm/regenerate.py --llvm /usr/lib/llvm-22/bin --check
```

It downloads the QuickJS archive `lib/Makefile` pins, or takes the copy that `--zip` names (`make download` in `lib/` puts one there), checks its SHA-256 against the one in the script, applies `lib/internal/patch/quickjs.diff`, runs the generator and compares the files it makes in `lib/` (the assembly, its Go bridges and `goasm_helpers.txt`) byte for byte. It exits with status 1 and names each file that differs. Without `--check` it writes the files. Set `CXXFLAGS` when the LLVM headers or the C++ library are outside the compiler's default paths. It takes a few minutes, under a minute once the Go build cache is warm.

`TestGoasmCoversInterpreterRules` in `internal/fixlib` checks that every rule of `fixlib` that changes `_JS_CallInternal` has a row in `goasmInterpreterRules` (`internal/fixlib/goasm.go`): written in C by `internal/goasm/cpatch.py`, already in the patched C, or only for Go's compiler. It fails when a new rule has no row, so a fast path added to the Go interpreter cannot be lost in the generated one unnoticed. `TestGoasmCoversHelperRules` does the same, in `goasmHelperRules`, for the rules that change the Go function of a C helper the generated interpreter compiles in, listed in `lib/goasm_helpers.txt`. Both run with the other tests of `fixlib`.

`TestOneInterpreterPerBuild` in `lib/` reads the build constraints of the goasm files for each of the five targets, with no tag, with `slowjs_nogoasm`, with the old `goasm` tag and with both: linux/amd64 without `slowjs_nogoasm` gets every goasm file and no `goasm_disabled.go`, every other build gets `goasm_disabled.go` alone. It runs with the other tests of `lib/`.

Run the generator tests, then the engine checks of linux/amd64 twice: as they are, for the goasm interpreter, and with `slowjs_nogoasm`, for the Go one:

```sh
python3 -m unittest discover -s internal/goasm
for tags in "" slowjs_nogoasm; do
  go test -short -p 2 -tags "$tags" -timeout 30m . ./lib ./internal/fixlib
  go test -p 2 -tags "$tags" -timeout 90m ./internal/fixlib
  go test -p 2 -vet=off -tags "$tags,libc.memgrind" -run 'Memgrind|EnginePanic|EngineGoexit|CallDepth|TLS|Goasm|StackGrowth|GCStops' ./...
  go test -race -short -p 2 -vet=off -tags "$tags" -timeout 2h . ./lib
  (cd lib && go test -vet=off -tags "$tags" -run Test262 -v . -args -xtags "$tags")
done
```

`-xtags` builds the test262 runner with the same tags. Both builds must give the same test262 errors. Then compare the HTML of tifa's pages between the two, and check a linked tifa binary with `python3 internal/goasm/verify_linked.py <work>/build/lowered.o <binary>` (pass `--work` to `regenerate.py` to keep `lowered.o`). Other targets build the Go interpreter with or without the tag, which [Every target](#every-target) covers when run with `-tags slowjs_nogoasm` as well.

## Same as upstream after fixlib

The committed `lib/ccgo_*.go` and `lib/internal/run-test262/ccgo_*.go` must be exactly what `fixlib` makes from the files of the upstream release, byte for byte. To check it, put the upstream files of libquickjs v0.13.1 from the module cache over them, run `fixlib`, and look for a difference:

```sh
go mod download modernc.org/libquickjs@v0.13.1
M="$(go env GOMODCACHE)/modernc.org/libquickjs@v0.13.1"
for f in lib/ccgo_*.go; do cp "$M/$(basename "$f")" "$f"; done
for f in lib/internal/run-test262/ccgo_*.go; do cp "$M/internal/run-test262/$(basename "$f")" "$f"; done
go run ./internal/fixlib       # about 40 seconds
git status --short            # prints nothing
```

`cp` onto an existing file keeps its mode, so the files stay writable though the module cache is read-only. If `git status` lists a file, `fixlib` no longer makes what is committed: either the committed file was edited by hand, or a change of `fixlib` was committed without the files it rewrites. After moving to a new upstream release, use that release's version in place of v0.13.1. Run `git checkout -- lib` to get back to the committed files when the check fails.

To compare behavior with the original engine instead, depend on `modernc.org/quickjs@v0.25.0` (with libquickjs v0.13.1) in a scratch module: it has the same `Compile`, `EvalBytecode` and `RegisterHostFunc` signatures as this fork, so only the import path differs.

## go.work

There is normally no `go.work`. The whole repository, `make benchmark` in `compare/` included (it reaches the root module through its `replace`), builds and tests in plain module mode. `go.work` is ignored and `make clean` deletes it: treat it as a temporary tool, not as configuration.

Make one with `make work` for one case only: working on a checkout of `modernc.org/libc` next to this one (`../libc`), when a change here needs an unreleased libc. Delete it (`make clean`, or `rm -f go.work go.work.sum`) once the change is in a libc release, or the tree keeps building against whatever is in the working copy next door instead of the version in `go.mod`.

While a `go.work` exists it must keep listing `./compare`. A workspace is found by walking up from the current folder, so it also governs builds inside `compare/`, which then fail with `main module (github.com/liustack/slowjs) does not contain package github.com/liustack/slowjs/compare`. `make work` writes the current toolchain into the `go` line: a `go.work` left over from an older toolchain, below the `go` version of `compare/go.mod`, breaks `make benchmark` with `module . listed in go.work file requires go >= …`.

## Speed

`go test -run '^$' -bench Call .` gives the cost of each kind of call, and `make benchmark` upstream's micro benchmarks on this engine, goja and fastschema/qjs, from `compare/`, the suite kept from upstream. The comparison a user choosing an engine would read is `compare/engines`, a module of its own next to it: it runs this engine, modernc.org/quickjs, go-quickjs, goant and moejs on the same workloads, each in its own binary pinned to one CPU, and C QuickJS, QuickJS-NG and Node as reference rows on tifa's pages ([benchmarks.md](benchmarks.md)). Render speed is measured in tifa, on the pages of its example app: see [performance.md](performance.md#measuring-in-tifa).
