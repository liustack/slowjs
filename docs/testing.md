---
summary: 'Testing and verification: the checks to run before committing, the Makefile targets, the focused checks of each fork change, test262 and its 58 known errors, test262 and the sanitizers on the C tree with the native test host, leak checks with memgrind, the race detector subset, building every target, checking that fixlib turns the upstream files into the committed ones byte for byte, and the local go.work'
read_when:
  - Getting ready to commit and checking which checks to run
  - Writing a new test or fixing a bug
  - Changing the engine, internal/fixlib or lib/internal/patch, and checking nothing else moved
  - Running test262, or test262 reports new errors
  - Changing the C tree in engine/, or running it natively under the sanitizers
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
go test ./internal/hostmem                               # the host memory of the new engine: chunks, footprint, remap, shadow stack pages and guard page, and what each keeps when the OS refuses
HOSTMEM_RESIDENCY=1 go test -run StackResidency ./internal/hostmem # exact counts of shadow stack pages in memory, a snapshot the system can change
HOSTMEM_BIGCOMMIT=1 go test -run ManyStacks ./internal/hostmem     # on windows: 300 stacks of 32 MiB, which commit 9.4 GiB
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

## The C tree and its native test host

`engine/` is the C tree of the engine to come. `engine/test/` is its native test host: the C tree built with its own small libc (`engine/libc`, see its [README](../engine/libc/README.md)) by plain clang, and test262 run on it. It tests the C itself, with no ccgo and no Go in the way. It is a tool for testing, never part of a Go build or of the product, and `engine/` holds no Go file (`python3 -m unittest discover -s internal/goasm` fails if it does, or if a folder with Go files holds a C file). It started as the pinned archive with `lib/internal/patch/quickjs.diff` and `internal/goasm/cpatch.py` applied, and nothing compares it with them now. Until the switch, a fix to the C of the old engine is made in `engine/` by hand too when it applies there.

```sh
engine/test/build.sh asan              # plain (default), asan, asan-heap, ubsan or asan-ubsan: engine/test/out/MODE/run-test262
engine/test/test262.sh -m asan -T 2    # test262 on that build, with 2 runner threads
engine/test/libc-test.sh               # the string functions and snprintf of engine/libc against the system's
engine/test/api-test.sh asan           # JS_DumpMemoryUsage, JS_NewClassID, abort and the limit of a resize, on real runtimes
engine/test/host-test.sh tsan          # the test host alone, two threads making and freeing runtimes
engine/test/math.sh                    # Math.* and the libm functions against the Go engine, bit for bit (linux/amd64)
engine/test/check-symbols.sh           # what the engine uses and does not define
engine/test/cycles.sh                  # the recursions no counted depth bounds yet, against cycles.txt
engine/libc/vendor.sh --verify         # the files taken from musl are the ones in MUSL.sha256
python3 -m unittest discover -s engine/test   # the checks of mathcmp.py and cycles.py
```

It builds and runs on Linux and on macOS (Apple clang 15 and later). `math.sh` is the exception: its reference is the Go engine on linux/amd64.

- The engine and `engine/libc` are built freestanding, with `-nostdinc` and only `engine/libc` for headers (the flags are in `flags.sh`, which `build.sh` and `check-symbols.sh` both read, so that what is built and what is checked cannot differ). The host, the test runner and the test programs use the system's headers and libc. The engine does not link the system's libm: `Math.*` is musl's, so its results are the same on every machine.
- `run-test262.c` is upstream's runner with a few changes (see its header): it includes `host.h`, makes its runtimes with `host_new_runtime()` and frees them with `host_free_runtime()`, prints the memory report through a buffer, keeps 2048 shapes in each test as a VM of the Go API does, and no longer calls `JS_SetCanBlock`. `host.c` copies the three functions the runner takes from `quickjs-libc.c`, interrupts a test that runs over 120 seconds, which the runner cannot do, and implements the callbacks of `engine/host.h`. It also defines `__shadow_remaining`, the bytes of stack left, from the stack of its thread: the engine counts its recursion and asks for that only to bound an alloca and keep a margin (see [c-engine.md](fork-changes/c-engine.md#depth-of-calls-and-of-c-recursion)). The runner's threads, and those of `jsrun` and of the depth tests of `unit`, have stacks of 512 MiB (`HOST_THREAD_STACK`, only reserved until used), so that a recursion to the deepest limits never runs off one.
- `host.c` also gives the memory of the allocator (see [allocator.md](allocator.md)): `host_memory_grow` maps each chunk of a runtime, and `host_free_runtime` unmaps them after `JS_FreeRuntime`. A runtime that still has blocks in use then is a leak: the process exits with status 87, which `test262.sh` reports as a failure. `HOST_MAX_MEMORY=BYTES` makes the host refuse a runtime more memory than that, as a system without memory would. `HOST_TIMEOUT_SEC=N` changes the 120 seconds a runtime may run, 0 for no limit (`render` sets 0). The host also answers the two memory kinds of `host_call` (a dedicated chunk goes back, or is remapped). On linux it tells the engine it can remap (`JS_SetHostRemap`), and `HOST_NO_REMAP=1` leaves that out, so that a dedicated block that outgrows its chunk gets a new chunk and a copy: `staging/sm/String/replace-math.js`, a string of 1 GiB grown by realloc, takes 1.05 GB and 1.2 seconds with remap, 2.0 GB and 4.5 seconds without.
- Time is fixed. `host_now` answers 2026-01-01T00:00:00Z for the wall clock, which fixes `Date` and the seed of `Math.random`. A few tests wait for the clock to move, such as `language/expressions/dynamic-import/await-import-evaluation.js`, and never end on a fixed one, so `test262.sh` sets `HOST_CLOCK=running`: the time starts at the same instant and passes.
- `test262.sh` downloads the test262 archive `lib/Makefile` pins (or takes the copy `-z` names, checked against its SHA-256), applies `lib/internal/patch/test262.diff`, uses `engine/test/test262.conf` and runs `-a` as the test of `lib/` does. It writes the errors it found and compares them line by line with `lib/test262_errors.txt`. It exits 0 only when they are the same lines. The results are in `engine/test/out/test262-MODE/`: `run.log`, `test262_errors.txt` and `errors.diff`.
- `engine/test/test262.conf` is `lib/testdata/test262.conf` with one addition, the tests under "no blocking Atomics" in its `[exclude]` section: 59 that start agents, and 7 that wait on the main thread for a time out. The engine has no threads, so `Atomics.wait` throws a `TypeError` and `Atomics.notify` wakes nobody. The 59 start their agents with `$262.agent.safeBroadcast` of `harness/atomicsHelper.js`, which begins with an `Atomics.wait` on a value that differs. It must return at once, and now throws. In 53 of the 59 an agent also waits for a wake up or a time out. In 6 nobody waits (`notify-with-no-agents-waiting.js`, `notify-with-no-matching-agents-waiting.js` and four `*-throws-agent.js` tests of `wait`), and only that probe keeps them out. Without the 59, the runner waits for ever for an agent that has ended.
- A full run (83426 tests, 3422 excluded) takes about 1.5 minutes in plain mode, 2.5 minutes under UndefinedBehaviorSanitizer and 11 minutes under AddressSanitizer, on 2 hyperthreads of one EPYC core. All give the 58 known errors with 2 threads. Sanitizer reports go to `run.log`. The AddressSanitizer build goes on after a report: run it with `ASAN_OPTIONS=halt_on_error=0:detect_leaks=1`. The UndefinedBehaviorSanitizer build always goes on: give it `UBSAN_OPTIONS=print_stacktrace=1`, and it reports each place once.
- `asan` gets every block from the system malloc (`JS_HEAP_SYSTEM_MALLOC`, `heap-system.c` in place of `heap.c` and `tlsf.c`), so that red zones and LeakSanitizer see each block alone. `asan-heap` keeps the engine's allocator and poisons what is free in it (`JS_HEAP_POISON`: free slots, free TLSF blocks, the rest of an arena), which finds a use after free inside our own memory. `poison-test.c` asks AddressSanitizer which bytes are poisoned and says what the poison does not cover (see [allocator.md](allocator.md#testing)). Run both over test262.
- AddressSanitizer reports nothing over test262, leaks included. UndefinedBehaviorSanitizer reports one place, in QuickJS's own code: `js_typed_array_slice` adds an offset to a null pointer (`quickjs.c`, `slice_memcpy`). It used to report three more. A null source of length 0 passed to `memcpy` was one, because the system's header says the pointer is not null and `engine/libc/string.h` does not. The calls of `js_realloc_rt` by `dbuf_claim` and `dbuf_free` through the type `DynBufReallocFunc` (`cutils.c`) were the other two: a `DynBuf` now calls it through `js_dbuf_realloc_rt`, which has that type. Anything else in a report is new.
- `libc-test.sh` builds `engine/libc` with its functions renamed to `sl_NAME`, so that they sit in one program with the system's, and compares them on random inputs with fixed seeds: the string functions at every alignment and length up to 300, and `snprintf` on every format the engine uses and many it does not, cut at every length from nothing to two past the end of the output, with a canary after the buffer. It also runs formats that must abort (floating point, `%n`, positional arguments, a length on `c`, `s`, `p` or `%`, a format that ends inside a conversion, numbers that do not fit an int, an output over INT_MAX) in a child process and checks that it died of SIGABRT, and not of a trap, and that it had taken no argument by then: `libc-test.sh` builds `stdio.c` with a `stdarg.h` that counts every `va_arg` (`engine/test/counting`), and formats like `%*ls`, `%.*f`, `%*2$d`, `%*%` and `%.*%` must be refused before the `*` takes its int. It also checks that a huge width with a small buffer costs nothing. In this program `abort` is the system's. `api-test.c` checks the same for a few of them through the engine's own `abort` and the host. It takes MODE like `build.sh`.
- `api-test.sh` runs `api-test.c` on real runtimes. `JS_DumpMemoryUsage` is given a runtime that holds objects, so that the part that names classes runs, and its report is checked for its text, for its length when there is no buffer, and cut to every size from 0 to past the end, with a canary after the cut. Run it in asan mode: the first version wrote past a buffer on the stack, in the part that a call with no runtime does not reach. `JS_NewClassID` is checked with two runtimes taking IDs by turns, a repeated query, a preset ID, and the built-in classes, which keep their numbers. The averages of the report, with one decimal, are compared with `%.1f` of the C library for about 190000 pairs of numbers, 1.05, 0.15, 0.05, 2.45 and 1.25 among them (the report rounds the exact value once). `Atomics.notify` gives 0 when nobody waits, and `Atomics.wait` converts its arguments in the order of the specification and throws what they throw before it throws that it cannot block (the Symbol and the throwing `valueOf` for the index, the value and the timeout). `abort`, and a format `snprintf` does not do, must end the process with `SIGABRT` after the host has heard it. A resize meets the memory limit as upstream does (`count` + the new size - the usable size of the old block), to the byte, for growing in place, moving, a chunk of its own, shrinking and the same size, and a refused resize leaves the block and the count as they were.
- `host-test.sh` runs `host-test.c`, the test host (`host.c`) linked with stubs in place of the engine. A runtime that does not start must leave no VM and no chunk in the host, and two threads that make and free runtimes must not race: `host-test.sh tsan` builds it with ThreadSanitizer, which `asan` and `ubsan` cannot replace. It takes MODE like `build.sh`.
- `math.sh` compares the math of the C tree with the Go engine in two ways, on inputs from the fixed generator of `mathcmp.js` (random bits, ranges of values, numbers near multiples of pi/2, every pair of edge values such as zeros, infinities, NaN, subnormals and the limits of `exp` for the functions of two arguments), 100000 for each function. The results must be the same bit for bit.
  - Math as JavaScript sees it: `mathcmp.js` runs on the C tree (`mathdump`) and on the Go engine (`internal/mathdump`) for 33 operations. A NaN is written as the one NaN of JavaScript, so its sign and payload are not compared here. Not every operation reaches the libc function of its name: `Math.round` is the engine's own code, and a Uint8ClampedArray store and `Atomics.pause` use `lrint` and `modf` only for a conversion and a test.
  - The libc functions themselves: `libmath` calls the 33 functions of `engine/libc` the engine uses (`round`, `modf` and `lrint` included) and `internal/libmdump` calls the same ones of `modernc.org/libc`, on the same inputs. These results are compared with a NaN as it is.
  - The Go engine is the reference because on linux/amd64 it runs musl's libm as `modernc.org/libc` translated it, from the same commit. The script stops on any other machine, and prints GOOS, GOARCH, the Go version and the commit it ran at. `mathcmp.py` refuses files that are empty, miss a function, hold one twice, have another count than 100000, are not hex or are not whole entries, or have different inputs or widths. Its checks are in `test_mathcmp.py`.
- `check-symbols.sh` builds the engine files and `engine/libc` freestanding at several optimization levels, with and without `NDEBUG`, and fails when they use a symbol that `engine/libc/symbols.txt` does not allow: only the libc functions, the callbacks of `engine/host.h` and `host_memory_grow`. The allocator is among the engine files (`heap.c`, `tlsf.c`), so there is no malloc on the list. Give it target triples to check other targets (`CC=clang NM=llvm-nm engine/test/check-symbols.sh x86_64-linux-gnu aarch64-linux-gnu`): the objects are only compiled, so it needs no cross toolchain. A failure of `nm` on an object ends the script, and it checks at the start that it rejects an unlisted symbol and an object it cannot read. It proves the symbol level only. It does not show that the machine code has no thread local storage, no system call instruction and no fixed address. Those checks come with the generator, not before.
- `cycles.sh` checks that the counted depths bound every recursion of the engine (see [c-engine.md](fork-changes/c-engine.md#depth-of-calls-and-of-c-recursion)). It compiles the engine files to one module of LLVM IR with the flags of the generator, at -O0 so that every function of the source is there and every count is a call of its own, and with `-fsanitize=kcfi`, then `cycles.py` builds the call graph: direct calls, and each indirect call joined to every function of its kcfi type whose address is taken, as the generator joins them. A function that counts a depth (`js_call_enter`, `js_internal_enter`, `lre_depth_enter`, `js_nest_enter`) cuts the cycles through it. The cycles left must be those of `cycles.txt`, line for line: today the one cycle of the property layer, which its data bounds and the generator cannot prove, still to be cut. The value printer is built only with `CONFIG_PRINT_VALUE`, which `build.sh` defines for the host and `cycles.sh` does not, as the generator does not. A new cycle fails the check, and so does a cycle that goes away without its line. It needs LLVM 22 (the folder of `clang` and `llvm-link` as its argument, `/usr/lib/llvm-22/bin` by default). `test_cycles.py` checks that `cycles.py` finds a cycle through direct and typed indirect calls and that a count cuts it.
- `vendor.sh --verify` checks that the files of `engine/libc` taken from musl are the ones `MUSL.sha256` lists, that none is missing or added, and that the patch is the recorded one. Given a musl checkout (`vendor.sh --verify MUSL_DIR`) it also checks the recorded hash of each file in musl and applies the patch to the original again.
- `engine/test/compare.sh [MODE]` compares the C tree with the Go engine on scripts: the transcripts of `lib/testdata` and `testdata` (forin, objlit, regex, jsonstr, own_keys and the others it lists) and the cases in `engine/test/compare/`. `jsrun` of the host and `internal/jsrun` run each script the same way, in a fresh runtime, then its jobs, and print its completion value (or what it returns when it is a function). The two outputs have to be the same byte for byte, which they are when a change made in Go by `fixlib` is made the same way in C. Build the host first. The outputs are in `engine/test/out/compare-MODE/`. It then runs `unit` (`engine/test/unit.c`), which checks through the public API what the scripts cannot see, such as what the caches keep and let go, and the limits of recursion: a plain function recurses to the call depth (5000, raised to 10000, lowered to 100, refused past the limit or while code runs), a host function that calls back in counts its entry, nesting stops at the internal depth in JSON, the parsers, regexps, `flat` and proxies, chains that recursed from C to C (300000 bound functions under `instanceof` and `Reflect.construct`, a `Promise.prototype.finally` chain, iterator helpers) end with a `RangeError`, and so do chains of modules (imports, read from bytecode or through the loader, `export * from`, `export { x } from`). The recursions rewritten as loops keep their results: ropes built at either end and rebalanced, freeing a million nested objects and every kind of object one level deep, the case conversions and the four normal forms of every code point and the emoji sequences against the hashes of the recursive code, the strings of `\q{...}` in regexps, the parse records of `JSON.parse` with a reviver, parentheses at 3 levels each and functions nested as deep as the parser goes. After each, the runtime runs code, recurses as deep as before, and frees with no block left. Run it inside `systemd-run -p MemoryMax=4G` or similar: a chain that went wrong takes memory fast.

### The allocator

```sh
engine/test/tlsf-edges.sh                        # the edge cases of TLSF, on the pinned file and on engine/tlsf.c
engine/test/build.sh plain && engine/test/heap.sh    # scenarios and random sequences, on the heap and on the runtime
engine/test/build.sh fuzz && engine/test/heap.sh -f 300   # the same sequences as a libFuzzer target, for 300 seconds
engine/test/build.sh plain && engine/test/out/plain/render bundle=B.js inputs=A.json,C.json soak=3000
```

[allocator.md](allocator.md) says what is tested and how. `heap.sh` takes `-m MODE` for `asan-heap` and `ubsan`. A change to `engine/heap.c`, `engine/tlsf.c` or the allocator part of `engine/quickjs.c` runs all of them, test262 in `plain` and `asan-heap`, `asan` over test262 for the system malloc, and a soak of 3000 renders of tifa's pages (`render.c`), whose count, blocks and footprint must not grow after the first hundred.

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
go test -race ./internal/hostmem                         # VMs mapping memory on many goroutines, and one VM handed between them
```

The whole of `./intl` under `-race` takes about 25 minutes on one core of an EPYC, most of it the golden test: give it `-timeout 60m`.

The race detector ignores atomic operations on memory outside Go's heap, such as the interrupt flag in C memory. `Interrupt` and `Close` meet at a mutex on Go's heap for that reason (see [architecture.md](architecture.md#the-eval-timeout-and-interrupt)).

## Every target

```sh
make build_all_targets
```

It builds every package and compiles the tests of every package (not only the root), each package to a test binary of its own, as two packages share the name `gen`, for the 5 supported targets, with and without `-tags=libc.memgrind`, and linux/amd64 with `-tags=slowjs_nogoasm` as well. It takes a long time. For a quick look, build the main ones:

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

It takes the pinned QuickJS archive `lib/Makefile` names (downloaded, or the copy that `--zip` names, checked against the SHA-256 in the script, and `make download` in `lib/` puts one there), applies `lib/internal/patch/quickjs.diff` and `internal/goasm/cpatch.py`, runs the generator and compares the files it makes in `lib/` (the assembly, its Go bridges and `goasm_helpers.txt`) byte for byte. It exits with status 1 and names each file that differs. Without `--check` it writes the files. Set `CXXFLAGS` when the LLVM headers or the C++ library are outside the compiler's default paths. It takes a few minutes, under a minute once the Go build cache is warm. `--llvm` names the folder with `clang-22`, `clang++-22`, `opt-22` and `llc-22`. On Ubuntu that is `/usr/bin` (`/usr/lib/llvm-22/bin` has them without the suffix), with `CXXFLAGS=-I/usr/lib/llvm-22/include` for the headers. The generator never reads `engine/`.

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

`-xtags` builds the test262 runner with the same tags. Both builds must give the same test262 errors. Then compare the HTML of tifa's pages between the two, and check a linked tifa binary with `python3 internal/goasm/verify_linked.py <work>/build/lowered.o <binary>` (pass `--work` to `regenerate.py` to keep `lowered.o`). Other targets build the Go interpreter with or without the tag, and [Every target](#every-target) builds linux/amd64 both ways.

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
