---
summary: '测试与验证：提交前要跑的检查，Makefile 目标，重点检查，test262 和它的 33 个已知错误，用原生测试宿主在 C 树上跑 test262 和 sanitizer，竞态检测器的子集，构建每个目标平台，本地的 go.work，生成 C 引擎并检查提交进仓库的就是生成器的结果的 make 目标，分两档运行它的检查，在几个代码位置上给构建计时，以及把这些全搬到另一台机器上做，还有 GitHub 上的 CI 检查什么、不检查什么'
read_when:
  - 准备提交，要确认该跑哪些检查
  - 写新测试或修 bug
  - 改引擎，要确认别处没有变
  - 跑 test262，或者 test262 报出新的错误
  - 改 engine/ 里的 C 树，或者在 sanitizer 下原生运行它
  - 查泄漏，或者改值的复制和释放方式（原生测试宿主的 sanitizer）
  - 改多个 goroutine 或多个 VM 共用的代码
  - 配置 go.work
  - 生成 C 引擎，对改动跑快档或全档检查，或者给一个构建和另一个构建计时对比
  - 在 Google Cloud 构建机上跑上面这些，用指定的核，限定内存
  - 改 `.github/workflows/`，或者 CI 跑红了，要知道它跑了什么
---

# 测试与验证

[English](testing.md) · 简体中文

## 提交前

每个改动都必须通过：

```sh
go build ./...
gofmt -l -s . | grep -v -e '^internal/cengine/engine_'   # prints nothing
go vet ./...
scripts/docs-list.sh --check           # every doc has a summary
go test -count=1 -p 1 .                # about a minute
```

- `go vet` 也检查生成的汇编（asmdecl），只针对它运行所在的系统。`TestEngineAsmDecl`（`internal/goasm/gen`）在全部五个系统上对 `internal/cengine` 运行这项检查。
- 改了 `engine/` 里的 C、生成器，或者 `scripts/gen-engine.sh` 和 `engine/test/flags.sh` 的参数，还要重新生成引擎，放在同一个提交里（`make gen`、`make gen ARCH=arm64`），并运行 `make check-fast`。一批改动做完时运行 `make check-full`（见[生成并检查](#生成并检查)）。
- 改了 `intl/`，还要运行 `go test ./intl`（golden 测试在里面）。
- 改了 `internal/gointl`，还要运行 `go test ./internal/gointl/...` 和 `make gointl`。
- 改了 `compare/engines`，还要在那里运行 `go test ./cmd/... ./internal/... ./tifa`。它是独立的模块，上面的命令够不着它，CI 也不跑它。这些测试覆盖辅助工具（落后很远的列表，页面表格和归档，模块转脚本的工具，版权声明生成器）和 `tifa/` 的 shell 脚本（用替身）。
- 改了 `temporal/`，还要运行不带 `-short` 的 `go test ./temporal`（和 Chromium 的对照以及链接测试都在里面），以及[重点检查](#重点检查)里 Temporal 的那几行和竞态检测器一节。
- 改了引擎，还要运行它所涉及部分的[重点检查](#重点检查)，以及 [test262](#test262)。
- `-p 1` 一次只跑一个包：根包的测试会启动很多 VM，每个 VM 都映射自己的内存。

新行为和原因已知的 bug：先写测试，看它失败，再写修复。对引擎的改动要在 `engine/test/` 里测试（一个脚本加它记录的输出，或者 C 写的检查，见 [C 树和它的原生测试宿主](#c-树和它的原生测试宿主)），也要在根包里测试（通过 Go API）。

## Makefile 目标

| 目标 | 运行什么 |
| --- | --- |
| `make editor` | `gofmt -l -s -w .`，每个包的 `go test -c`（只编译），`go install ./...`，`staticcheck` |
| `make test` | `go test -failfast -v -timeout 24h -count=1 ./...` |
| `make shorttest` | 同上但带 `-short`，跳过耗时长的用例（test262 在其中） |
| `make build_all_targets` | `./build_all_targets.sh`（见[每个目标平台](#每个目标平台)） |
| `make gen`、`make gen ARCH=arm64` | 把 `engine/` 的引擎生成为 `internal/cengine` 里的 Go 汇编，要在装了 LLVM 22.1.8 的机器上（见[生成并检查](#生成并检查)） |
| `make check-fast`、`make check-full` | 对 C 引擎改动的检查，最后出一张表：快档每个提交都跑，全档在一批改动做完时跑（见[生成并检查](#生成并检查)） |
| `make check-gen` | 检查提交进仓库的引擎就是现在 `make gen` 的结果，两种架构都查（`check-full` 的一部分） |
| `make bench ARGS="..."` | `go tool measure bench`：代码位置计时和 callgrind（见[给两个构建计时对比](#给两个构建计时对比)） |
| `make remote RUN="..."` | 在另一台机器上运行别的 make 目标（见[在另一台机器上](#在另一台机器上)） |
| `make gointl` | `go test ./...` 不跑的 `internal/gointl` 检查：打包工具的测试（需要 `gointl_walk` 构建标签），不带可选数据包的构建（`-tags quickjs_intl_nonames,quickjs_intl_nocjdict`，在那里和 `intl` 里的 `Intl.DisplayNames` 都要跑），以及 `qjsdata/` 就是 go-intl 的数据加上 `internal/gointl/qjstz` 所指 tz 更新里的时区所得的结果（这会克隆 go-intl，并下载那个更新和 ICU 的源码） |
| `make benchmark` | `make -C compare benchmark`：`compare/testdata/` 里 QuickJS 的 64 个微基准（goja 或 fastschema/qjs 有 6 个跑不过，所以三个引擎共有的是另外 58 个），每个都在这个引擎、goja 和 fastschema/qjs（wazero 上的 QuickJS）上跑，也就是 QuickJS 自己的测试集 |
| `make compare-engines` | 在 `compare/engines` 里运行 `go run ./cmd/compare $(ARGS)`：和其他纯 Go JavaScript 引擎的对比，见 [benchmarks.md](benchmarks.md)（支持度，速度，内存，限额，tifa 页面），每个引擎在自己的二进制里 |
| `make cpu`、`make mem` | `make -C compare cpu` 和 `make -C compare mem`：对比基准的 CPU 和内存剖析 |
| `make cover` | 根包的覆盖率，以 HTML 显示 |
| `make work` | 写一个 `go.work`（见 [go.work](#gowork)） |
| `make clean` | 删除测试二进制、剖析文件和 `go.work` |

这里不要求跑 `staticcheck` 和 `golint`。不带 `-short` 的 `make test` 会完整运行每个包，包括 test262，耗时很长：只跑你需要的部分。

跑一个测试或一个子测试：`go test -run TestEval -v .` 或 `go test -run 'TestEval/eval1' -v .`。

## 缺工具时会跳过的测试

`internal/goasm/gen` 里生成器的测试需要 LLVM，没给就跳过（并且仍算通过），所以单独运行 `go test ./internal/goasm/gen` 只检查规则。设置这三个变量就能全部运行：

```sh
export QUICKJS_LLVM=/usr/lib/llvm-22/bin                          # LLVM 22.1.8 bin directory (clang, opt, llc)
export QUICKJS_SHADOW_PASS=$PWD/internal/goasm/gen/internal/_shadow.so   # the built pass, made by internal/goasm/gen/proto.sh
export QUICKJS_LLD=/usr/lib/llvm-22/bin/ld.lld                    # the linker (GNU ld, /usr/bin/ld, also works on linux)
go test -p 2 -timeout 30m ./internal/goasm/gen/...
```

只设 `QUICKJS_LLVM` 和 `QUICKJS_SHADOW_PASS` 时，需要 `ld.lld` 的测试（`TestVarargsNative`、`TestSampleRuns`、`TestNarrowArgs`）仍然跳过。被跳过的测试会打印 `--- SKIP` 和它要的变量名：相信一次绿色的结果之前，先带 `-v` 运行并找一找。`QUICKJS_QEMU_AARCH64`（可选）指向一个模拟器，让 arm64 的用例能在 amd64 主机上跑。

`go test -race` 需要 `CGO_ENABLED=1` 和 C 编译器，因为竞态运行时是通过 cgo 链接的。这个仓库用 `CGO_ENABLED=0` 构建，所以只对这条命令设置：`CGO_ENABLED=1 go test -race ...`。本文档里每条 `-race` 命令都是这个意思。

## 重点检查

记录下来的输出、sanitizer 和 C 写的检查在 [C 树和它的原生测试宿主](#c-树和它的原生测试宿主) 里。下面是 Go 一侧的检查，按名字列出：

```sh
go test -short -timeout 30m ./...                        # all packages
go test -run 'TestErrorStack' .                          # stack text and memory of lazy error stacks
go test -run 'CallDepth|ShadowStack|StackGrowth|CallAllocs' .  # depth limits, shadow stack use, deep recursion while Go moves the stack, no Go allocation per call
go test -run 'HostPanic|Handover|Clock' .                # a panic or Goexit in a Go function or a clock, a VM handed to another goroutine
go test -run 'Interrupt|Killed' .                        # Interrupt, and the kill of a script that does not stop
go test -tags testhooks -run TestInterruptStopsLongKeys ./internal/cengine # the first use of a string of tens of millions of characters as a key, and a JSON text with one, stops within a few polls
go test -tags testhooks -run TestInterruptStopsRegExpCompile ./internal/cengine # the compile of a class of many characters or strings stops within a few polls
go test -run 'Regex(Scan|Match)Stops' .                  # a timeout stops a regexp on a long string
go test -run 'RegExpCacheStops|BuiltinWorkStops|StringSearchStops' . # a timeout stops loops of regexp compiles, parsing, string building and searches within a few polls
go test -run 'AtomKeys|FieldChain|OwnKeys' .             # keys and property reads give what Node gives (testdata/*.golden)
go test -run 'ShapeCache|RegExpCache' .                  # the shape cache and the regexp cache give what Node gives and keep to their limits
go test -run OnClose .                                   # OnClose runs once, in order, and keeps no VM alive
go test -run 'Close(Twice|CallbackPanics)' .             # Close again does nothing, and a panicking OnClose function keeps no other step from running
go test -tags testhooks -run 'CloseUnmap|CloseSharedDrop|MapRefused|ReleaseRefused|RemapRefused' . # Close again frees what the OS refused to unmap, and a refused map ends the call with out of memory
go test -p 2 -run 'VMClock|MonoNow' .                    # VM.SetClock: Date, Intl and Temporal share one clock, reset, Close and timeouts
go test -tags testhooks ./internal/cengine ./internal/gohost ./internal/hostmem ./internal/idtable # the generated engine's host, with the test hooks
HOSTMEM_RESIDENCY=1 go test -run StackResidency ./internal/hostmem # exact counts of shadow stack pages in memory, a snapshot the system can change
go test -p 2 ./temporal -run 'DateTimeFormatTemporal|DurationLocaleString|Chromium' # Temporal formatting against Chromium 156
go test -p 2 ./intl -run 'GoIntlMigration|EmbeddedZonePack|FormatStringMatchesParts' # rounding, calendars and embedded time zones
go test -p 2 ./internal/intlhook                        # private state across GC and Close
go test -run LinksOnlyReadData ./temporal               # the packs a program with Temporal links
go test -run QJSTZVersion ./internal/gointl              # tz.bin holds the zones of the update qjstz names
internal/gointl/qjs-sync.sh pack -check                  # shared Intl data is what the generator makes
go test -run '^$' -bench Call .                          # the cost of each kind of call
go test -run '^$' -bench RegexColors .                   # the cost of a replace with react-spring's color names
go test -run '^$' -bench CopyProps .                    # the cost of createElement and cloneElement with React's loop and with the function of RegisterCopyProps, on configs of a page
go test -run '^$' -bench EscapeText .                    # the cost of React DOM's escape in C against React's own and tifa's cache, on the texts of a page
go test -run '^$' -bench Clock .                         # the cost of Date.now and new Date()
```

停止类测试（`TestBuiltinWorkStops`、`TestStringSearchStops`、`TestRegExpCacheStops`、`TestEscapeTextStops` 和 `TestTagsStop`）用 10 ms 的超时或 `Interrupt` 来停一个循环，判断是否停住看的是见到请求的轮询次数（`gohost.VM.StopPolls`），不看用了多久：最多 5 次（`stoppolls_test.go` 里的 `stopPolls`），每 65,536 个字符读一次标志、每次带一个 tick 的循环最多 9 次。这个次数取决于引擎在两次读标志之间运行的代码，和机器的速度、负载以及竞态检测器都无关，所以慢的运行环境不会让它们失败，而差了几次轮询就会失败。

[changes](changes/README.zh-CN.md) 下每份文档都列出了它那些改动的测试。

## test262

test262 用原生测试宿主 `engine/test/test262.sh` 在 C 树上运行（见下一节）：

```sh
engine/test/build.sh plain
engine/test/test262.sh -m plain -T 2      # about 1.5 minutes
```

- `engine/test/test262_errors.txt` 列出已知会失败的 33 个测试，每个一行（文件、行号和错误）。脚本把找到的错误和这份列表逐行比较，有任何差别就失败。对引擎的改动必须留下同样的 33 个错误。
- 运行器是 `engine/test/run-test262.c`，也就是上游 QuickJS 的运行器加了几处改动。配置是 `engine/test/test262.conf`，给 test262 打的补丁是 `engine/test/test262.diff`。
- Go API 和 Go 宿主不在 test262 上运行。它们新增的部分由根包的 Go 测试检查，Go 和引擎之间交互的部分由 `internal/cengine` 的测试检查。

## C 树和它的原生测试宿主

`engine/` 是引擎的 C 树。`engine/test/` 是它的原生测试宿主：用普通 clang 和它自己的小 libc（`engine/libc`，见它的 [README](../engine/libc/README.zh-CN.md)）构建的 C 树，test262 就跑在它上面。它测的是 C 本身，中间没有 Go。它是测试用的工具，从不属于 Go 构建或产品，`engine/` 里也没有 Go 文件（根包里的 `TestRepositoryLayout` 在有 Go 文件时会失败，在有 Go 文件的目录里放了 C 文件时也会失败）。

```sh
engine/test/build.sh asan              # plain (default), asan, asan-heap, asan-heap-iter, ubsan or asan-ubsan: engine/test/out/MODE/run-test262
engine/test/test262.sh -m asan -T 2    # test262 on that build, with 2 runner threads
engine/test/libc-test.sh               # the string functions and snprintf of engine/libc against the system's, and vsnprintf against the generator's table of formats
engine/test/api-test.sh asan           # JS_DumpMemoryUsage, JS_NewClassID, abort, JS_SetOpaque of an ordinary object and the limit of a resize, on real runtimes
engine/test/host-test.sh tsan          # the test host alone: threads making and freeing runtimes, stops asked from another thread
engine/test/math.sh                    # Math.* and the libm functions against the hashes kept in engine/test/math, bit for bit
engine/test/check-symbols.sh           # what the engine uses and does not define
engine/test/cycles.sh                  # the recursions no counted depth bounds yet, against cycles.txt
engine/libc/vendor.sh --verify         # the files taken from musl are the ones in MUSL.sha256
go test ./internal/goasm/cmd/cycles      # the checks of the cycles tool
```

它能在 Linux 和 macOS（Apple clang 15 及以后）上构建和运行。

- 引擎和 `engine/libc` 以独立（freestanding）方式构建，带 `-nostdinc`，头文件只用 `engine/libc` 的（参数写在 `flags.sh` 里，`build.sh`、`check-symbols.sh`、`cycles.sh`、`libc-test.sh` 和 `internal/goasm/gen` 的生成器都读它，所以构建的内容和检查的内容不会不一致）。宿主、测试运行器和测试程序用系统的头文件和 libc。引擎不链接系统的 libm：`Math.*` 用的是 musl 的，所以结果在每台机器上都一样。
- `run-test262.c` 是上游的运行器加了几处改动（见它开头的注释）：它包含 `host.h`，用 `host_new_runtime()` 创建运行时、用 `host_free_runtime()` 释放，通过缓冲区打印内存报告，像 Go API 的 VM 那样在每个测试里保留 2048 个 shape（引擎记录一个对象有哪些键、按什么顺序的结构），并且不再调用 `JS_SetCanBlock`。`host.c` 从 `quickjs-libc.c` 抄来运行器要的三个函数，中断运行超过 120 秒的测试（运行器自己做不到：宿主的一个线程请这样的运行时停下，引擎的线程在下一次 `host_poll` 时设上它的中断标志，见 [c-engine.md](changes/c-engine.zh-CN.md#轮询与中断)），并实现 `engine/host.h` 的回调。它还根据线程的栈定义 `__shadow_remaining`，即剩余的栈字节数：引擎自己数递归深度，只在限制 alloca 和留出余量时才问这个值（见 [c-engine.md](changes/c-engine.zh-CN.md#调用深度与-c-递归)）。运行器的线程，以及 `jsrun` 和 `unit` 的深度测试的线程，栈都是 512 MiB（`HOST_THREAD_STACK`，用到才真正占用），这样递归到最深的限制也不会跑出栈外。
- `host.c` 还提供分配器的内存（见 [allocator.md](allocator.zh-CN.md)）：`host_memory_grow` 映射一个运行时的每个内存块，`host_free_runtime` 在 `JS_FreeRuntime` 之后把它们取消映射。如果那时运行时还有在用的块，就是泄漏：进程以状态 87 退出，`test262.sh` 把它报告为失败。`HOST_MAX_MEMORY=BYTES` 让宿主拒绝给运行时超过这个数的内存，就像系统没有内存了一样。`HOST_TIMEOUT_SEC=N` 改变运行时可以运行的 120 秒，0 表示不限（`render` 设为 0）。宿主还回答 `host_call` 的两种内存请求（归还一个专用内存块，或者重新映射）。在 linux 上它告诉引擎可以重新映射（`JS_SetHostRemap`），`HOST_NO_REMAP=1` 把这一点去掉，这样超出内存块的专用块会得到一个新块并复制过去：`staging/sm/String/replace-math.js` 用 realloc 把一个字符串长到 1 GiB，有重新映射时占 1.05 GB、用 1.2 秒，没有时占 2.0 GB、用 4.5 秒。
- 时间是固定的。`host_now` 的墙上时钟回答 2026-01-01T00:00:00Z，这固定了 `Date` 和 `Math.random` 的种子。有几个测试要等时钟走动，比如 `language/expressions/dynamic-import/await-import-evaluation.js`，在固定的时钟上永远结束不了，所以 `test262.sh` 设了 `HOST_CLOCK=running`：时间从同一时刻开始，并且会流逝。测试可以用 `host_set_wall` 把自己线程的墙上时钟设成自己要的时间（负数表示 1970 年以前），`host.h` 里的 `host_poll_reads` 和 `host_poll_stop_after` 数中断标志被读了几次，并在第 n 次读时把标志置上。
- `test262.sh` 下载它钉住的 test262 归档（或者用 `-z` 指定的副本，按 SHA-256 核对），打上 `engine/test/test262.diff`，使用 `engine/test/test262.conf`，运行 `-a`。它把找到的错误写出来，和 `engine/test/test262_errors.txt` 逐行比较。只有两边的行完全相同，它才以 0 退出。结果在 `engine/test/out/test262-MODE/` 里：`run.log`、`test262_errors.txt` 和 `errors.diff`。
- `engine/test/test262.conf` 是上游的配置加了一处：它的 `[exclude]` 一节里"no blocking Atomics"下的测试，共 59 个启动了 agent，另有 7 个在主线程上等一个超时。引擎没有线程，所以 `Atomics.wait` 抛 `TypeError`，`Atomics.notify` 不会唤醒任何人。那 59 个用 `harness/atomicsHelper.js` 的 `$262.agent.safeBroadcast` 启动 agent，它一开头就对一个不同的值做 `Atomics.wait`。这个调用本该立刻返回，现在却抛错。59 个里有 53 个，agent 还要等一次唤醒或一次超时。有 6 个没有人在等（`notify-with-no-agents-waiting.js`、`notify-with-no-matching-agents-waiting.js`，以及 `wait` 的四个 `*-throws-agent.js` 测试），只有那次探测让它们进不来。没有这 59 个，运行器会永远等一个已经结束的 agent。
- 完整运行（83426 个测试，排除 3422 个）在 plain 模式下约 1.5 分钟，UndefinedBehaviorSanitizer 下 2.5 分钟，AddressSanitizer 下 11 分钟，用的是一个 EPYC 核的 2 个超线程。用 2 个线程时，全都给出 33 个已知错误。sanitizer 的报告进 `run.log`。AddressSanitizer 构建在报告之后会继续运行：用 `ASAN_OPTIONS=halt_on_error=0:detect_leaks=1` 运行它。UndefinedBehaviorSanitizer 构建总是继续运行：给它 `UBSAN_OPTIONS=print_stacktrace=1`，它每个位置只报告一次。
- `asan` 的每个块都来自系统的 malloc（`JS_HEAP_SYSTEM_MALLOC`，用 `heap-system.c` 代替 `heap.c` 和 `tlsf.c`），这样红区和 LeakSanitizer 能单独看到每个块。`asan-heap` 保留引擎自己的分配器，并把里面空闲的部分毒化（`JS_HEAP_POISON`：空闲槽，空闲的 TLSF 块，一个 arena 的剩余部分，在缓存里等着的块），这样能找到我们自己内存里的释放后使用。`poison-test.c` 问 AddressSanitizer 哪些字节被毒化了，对一个缓存的大块做一次释放后使用，看 AddressSanitizer 是否报告，并说明毒化没有覆盖什么（见 [allocator.md](allocator.zh-CN.md#测试)）。两种都要在 test262 上运行。
- AddressSanitizer 在 test262 上什么也不报，泄漏也没有。UndefinedBehaviorSanitizer 报告一处，在 QuickJS 自己的代码里：`js_typed_array_slice` 给一个空指针加了偏移（`quickjs.c`，`slice_memcpy`）。报告里出现任何别的东西都是新的。
- `libc-test.sh` 构建 `engine/libc`，把它的函数改名为 `sl_NAME`，这样它们能和系统的函数放在同一个程序里，然后在固定种子的随机输入上比较它们：字符串函数在每种对齐和每个不超过 300 的长度上比较。`memcpy` 单独在从 0 到 1100 字节的每个大小上比较，源和目标各取离 64 的倍数 16 种偏移（目标先填成一个源里没有的字节，这样写到范围外的字节会被发现，源不能被改变），并在 300 个不超过 70000 的随机大小上比较。然后再来一遍，让源紧贴一页不可读的内存，分别结束在它之前和开始在它之后，在不超过 1100 的每个大小和 300 个不超过 70000 的随机大小上比较，这样读过源的任何一端都会出错，在 amd64 和 arm64 上都做。`snprintf` 在引擎用到的每种格式和许多它没用到的格式上比较，输出从空到超出末尾两个字节，每个长度都截一次，缓冲区后面放一个金丝雀。它还在子进程里运行必须中止的格式（浮点，`%n`，位置参数，给 `c`、`s`、`p` 或 `%` 加长度，在转换中间结束的格式，装不进 int 的数，超过 INT_MAX 的输出），检查它死于 SIGABRT 而不是陷阱，并且那时一个参数也没取过：`libc-test.sh` 用一个对每次 `va_arg` 计数的 `stdarg.h`（`engine/test/counting`）构建 `stdio.c`，像 `%*ls`、`%.*f`、`%*2$d`、`%*%` 和 `%.*%` 这样的格式必须在 `*` 取走它的 int 之前就被拒绝。它还检查很大的宽度配很小的缓冲区不花代价，并把生成器的格式表（`internal/goasm/gen/testdata/formats/cases.txt`，`formats_test.go` 让它和检查器保持一致）的每一行喂给 `vsnprintf`：标为 `ok` 的格式必须在恰好取走它所写明的参数后返回，标为 `bad` 的必须死于 SIGABRT。表和 `stdio.c` 对不上的格式会在这里失败。这个程序里的 `abort` 是系统的。`api-test.c` 通过引擎自己的 `abort` 和宿主，对其中几个做同样的检查。它像 `build.sh` 一样接受 MODE。
- `api-test.sh` 在真实的运行时上运行 `api-test.c`。`JS_DumpMemoryUsage` 拿到一个持有对象的运行时，这样列出类名的那一部分会运行，检查它报告的文本，检查没有缓冲区时的长度，并从 0 到超出末尾的每个大小各截一次，截断处后面放一个金丝雀。在 asan 模式下运行它，这样能抓到栈上缓冲区的越界写。`JS_NewClassID` 的检查包括两个运行时轮流取 ID，重复查询，预设的 ID，以及保持原来编号的内置类。报告里保留一位小数的平均值，对大约 190000 对数字和 C 库的 `%.1f` 比较，其中有 1.05、0.15、0.05、2.45 和 1.25（报告只对精确值舍入一次）。没有人在等时，`Atomics.notify` 给出 0，`Atomics.wait` 按规范的顺序转换它的参数，并在抛出它不能阻塞之前，先抛出转换时该抛的错（索引、值和超时各自的 Symbol 和会抛错的 `valueOf`）。`abort`，以及 `snprintf` 不支持的格式，必须在宿主听到之后以 `SIGABRT` 结束进程。一个普通对象的 `JS_SetOpaque` 也必须这样，它的块在不透明指针之前就结束了。其中每一项都在自己的进程里运行，也就是把测试带上用例作为参数重新启动，而不是 fork，原因和 `unit-white` 的用例一样。调整大小时碰到内存限制，和上游一样（`count` + 新大小 - 旧块的可用大小），精确到字节，覆盖原地增长，搬移，独立的内存块，缩小和大小不变的情况，被拒绝的调整大小会让块和计数保持原样。
- `host-test.sh` 运行 `host-test.c`，也就是测试宿主（`host.c`）链接上代替引擎的桩。启动不了的运行时不能在宿主里留下 VM 和内存块，两个创建并释放运行时的线程不能有竞态，由另一个线程请求的停止（`host_interrupt`），或者由结束长测试的线程请求的停止（那里是 1 秒后），必须在运行该运行时的线程的下一次 `host_poll` 时设上中断标志，且没有别的线程写这个标志：`host-test.sh tsan` 用 ThreadSanitizer 构建它，`asan` 和 `ubsan` 代替不了。它像 `build.sh` 一样接受 MODE。
- `math.sh` 把 C 树的数学运算和 `engine/test/math` 里保存的哈希按位比较，分两种方式，输入来自 `mathcmp.js` 的固定生成器（随机位，数值范围，接近 pi/2 倍数的数，两个参数的函数还有每一对边界值，如零，无穷，NaN，次正规数和 `exp` 的极限），每个函数 100000 个。每个文件对每个函数保存输入那一行和结果那一行的 SHA-256。
  - JavaScript 看到的 Math（`math/js.hashes`）：`mathcmp.js` 在 C 树上运行（`mathdump`），共 33 种运算。NaN 一律写成 JavaScript 里唯一的那个 NaN，所以这里不检查它的符号和载荷。并不是每种运算都会用到同名的 libc 函数：`Math.round` 是引擎自己的代码，Uint8ClampedArray 的存储和 `Atomics.pause` 只在转换和判断时用 `lrint` 和 `modf`。
  - libc 函数本身（`math/libc.hashes`）：`libmath` 调用引擎用到的 `engine/libc` 的 33 个函数（包括 `round`、`modf` 和 `lrint`）。NaN 原样取用，它的符号由机器决定，所以只在 x86-64 上检查这一项。
  - 哈希是在 linux/amd64 上记录的，C 树给出同样的位。`MATH_UPDATE=1 engine/test/math.sh` 在 x86-64 上从 C 树重新写出它们，提交前要先看一遍。
- `check-symbols.sh` 以几种优化级别、带和不带 `NDEBUG`，用独立方式构建引擎文件和 `engine/libc`，如果它们用了 `engine/libc/symbols.txt` 不允许的符号就失败：只允许 libc 函数、`engine/host.h` 的回调和 `host_memory_grow`。分配器就在引擎文件里（`heap.c`、`tlsf.c`），所以列表里没有 malloc。给它目标三元组就能检查其他平台（`CC=clang NM=llvm-nm engine/test/check-symbols.sh x86_64-linux-gnu aarch64-linux-gnu`）：对象只编译，所以不需要交叉工具链。对一个对象运行 `nm` 失败会让脚本结束，它开头还会检查自己会拒绝一个不在列表里的符号和一个读不了的对象。它只证明符号这一层。它不能证明机器码里没有线程局部存储，没有系统调用指令，没有固定地址。那些检查随生成器一起提供，在那之前没有。
- `cycles.sh` 检查带计数的深度限制了引擎的每一处递归（见 [c-engine.md](changes/c-engine.zh-CN.md#调用深度与-c-递归)）。它用生成器的参数，在 -O0 下把引擎文件编译成一个 LLVM IR 模块（这样源码里的每个函数都在，每个计数都是一次独立的调用），并带 `-fsanitize=kcfi`，然后 `internal/goasm/cmd/cycles` 构建调用图：直接调用，以及把每个间接调用连到它的 kcfi 类型相同且地址被取过的每个函数（生成器也这样连）。数一次深度的函数（`js_call_enter`、`js_internal_enter`、`js_internal_enter_rt`、`lre_depth_enter`、`js_nest_enter`）会切断经过它的环。剩下的环必须和 `cycles.txt` 逐行一致，而它是空的：每一处递归都经过一次计数。这个工具还检查一些函数对，其中第一个无论走计数的路还是不走计数的路都不许到达第二个（`noReach`），对到得了的就打印一条最短路径：生成错误（`JS_ThrowError2`）既到不了 `JS_CallInternal`，也到不了 `JS_ThrowInterrupted`，更到不了读中断标志的函数（`js_poll_interrupts`、`lre_check_timeout`、`lre_poll_timeout`、`rqsort_stop`），一次回收（`JS_RunGC`、`JS_RunYoungGC`）到不了 `JS_ThrowError2`，`free_zero_refcount` 和 `free_zero_refcount_first` 也不会启动回收。错误嵌套和释放嵌套的深度就靠这几条来限定。一对函数里如果有模块没定义的，就是错误，所以改名不会让某条检查失效。值打印器只在带 `CONFIG_PRINT_VALUE` 时才构建，`build.sh` 为宿主定义了它，`cycles.sh` 没有，生成器也没有。出现新的环会让检查失败，一个环消失了而没删它的那一行也会失败。它需要 LLVM 22（参数是 `clang` 和 `llvm-link` 所在的目录，默认 `/usr/lib/llvm-22/bin`）。`internal/goasm/cmd/cycles` 的测试（`check.sh` 的 `goasm` 步骤会运行它们）检查这个工具能找到经过直接调用和带类型的间接调用的环，一次计数能切断它，也能找到经过直接调用、带类型的间接调用和计数，从一个函数到另一个函数的路径，这正是 `noReach` 需要的。
- `vendor.sh --verify` 检查 `engine/libc` 里取自 musl 的文件就是 `MUSL.sha256` 列出的那些，没有缺的也没有多的，补丁就是记录下来的那个。给它一份 musl 的检出（`vendor.sh --verify MUSL_DIR`），它还会检查每个文件在 musl 里记录的哈希，并把补丁重新打到原文件上。
- `engine/test/compare.sh [MODE]` 把 C 树和仓库里保存的文件比较：`engine/test/compare/recorded` 和 `engine/test/compare/upstream` 的输出记录（`NAME.js` 旁边的 `NAME.txt`）和 `testdata` 的输出记录（`NAME.golden`），它们是 Node 24 或改动之前的引擎打印的内容，还有 `engine/test/compare/` 里的用例（`NAME.js` 旁边的 `NAME.txt`）。宿主的 `jsrun` 在全新的运行时里运行每个脚本，然后运行它的任务，打印它的完成值（如果脚本是函数，则打印它的返回值）。输出必须和文件逐字节一致，对 C 里做的保持引擎结果不变的改动，这是成立的。`regexcache.js` 运行三次，正则缓存分别取默认值、4 项和关闭，每次打印的内容相同。`recorded`、`upstream` 和 `testdata` 的文件里，孤立代理项按引擎转换的样子保存（`jsrun -raw`），`engine/test/compare/` 的文件把它写成 `\u{...}`。以 `.mjs` 结尾的文件作为模块运行（它给出 `globalThis.result`，它的 import 从它所在的目录读取），`-result` 给出以一个任务结束的脚本的 `globalThis.result`，`-call` 把文件当作宿主调用的函数体，它下面的栈上什么也没有。脚本里有 `gc()`、`youngGC()`、`memorySize()`、`memoryBlocks()`、`heapChange(bytes, blocks)` 和 `setGCThreshold(n)`（`n` 为负表示引擎从不自己回收），分别运行 `JS_RunGC` 和 `JS_RunYoungGC`，给出 `JS_GetMemorySize` 和在用的堆块数，把堆和之前读到的两个数比较，以及设置 `JS_SetGCThreshold`。`engine/test/compare/upstream` 里的脚本（`test_bigint.js`、`test_closure.js`、`test_language.js`、`test_loop.js` 和 `test_builtin.js`，最后这个的前面有几行提供 `std.gc` 和 `os.setTimeout`）也会运行，断言失败时它们打印异常，而不是 `undefined`。`testdata/error_stack.js` 运行两次，一次作为脚本，一次作为从 C 调用的函数体，对照 golden 文件里栈之前的那一部分，栈之后的内容只有 Go 宿主能读。`engine/test/compare` 里的脚本在一行 `// jsrun: OPTIONS` 里写它需要的选项。`fast_get.js`、`fast_set.js`、`numeric_keys.js`、`array_length.js`、`rope_concat.js` 和 `to_string.js` 运行属性层的各项精简所改动的路径（见 [c-engine.md](changes/c-engine.zh-CN.md#调用深度与-c-递归)），它们的文件就是 Node 24 打印的内容。`array_slice_splice.js`、`strict_equal.js`、`number_compare.js`、`string_concat.js` 和 `template_literal.js` 打印的也是 Node 24 打印的内容，对应从上游取来的提交（见 [upstream-commits.md](changes/upstream-commits.zh-CN.md)）。那张表里提到的其他脚本（`bigint_as_n.js`、`iterator_take_drop.js`、`string_repeat.js`、`atomics_store.js`、`array_buffer_resize.js`、`typed_array_at.js`、`completion_values.js`、`iterator_helpers_return.js`、`yield_await_names.js`、`call_as_target.js`、`promise_try.js`、`module_ambiguous_export.mjs`、`module_string_names.mjs`、`set_non_extensible.js`、`array_add_index.js`、`for_in_initializer.js`、`bind_prototype.js`、`proxy_own_descriptor.js`、`define_properties_order.js`、`class_element_names.js`、`sort_write_back.js`、`proxy_own_keys_result.js` 和 `for_await_close.js`）打印的也是 Node 24 打印的内容，只有它们标着 `ES2026` 或 `spec:` 的那几行例外，那里现行规范和 Node 24 不同，并且脚本开头有说明。其中两个带模块的，从 `engine/test/compare/modules` import 它们。会生成新值并在之后检查堆的脚本（`atomics_store.js`、`proxy_own_descriptor.js`、`define_properties_order.js`、`sort_write_back.js`、`proxy_own_keys_result.js`）像 `strict_equal.js` 那样 require 它。`unit-api` 有 `test_read_truncated_bigint`，它读取一个在每种长度处被截断的 BigInt，让 `vm_free` 的泄漏检查去发现多出来的块。`array_slice_splice.js` 把它的用例运行两遍，先开启稠密数组的快速路径，再关闭，用索引碰一下 `Array.prototype` 就会永久关闭它们。`strict_equal.js`、`string_concat.js` 和 `template_literal.js` 还要求，每次用新生成的值运行之后，堆保持原样：在用的块相同（用 TLSF 时是 arena 和大块，用系统 malloc 时是每个块），字节数也相同，TLSF 下每个块允许差 56 字节上下，因为同一个请求可能得到一个最多大 24 字节的块，或者来自大块缓存、最多大 56 字节的块。`call_full_stack.js` 从没有局部变量的函数里做尾调用和尾方法调用，这些函数在栈满时释放它们的值，偶数个和奇数个各做一次，并要求做了 20000 个之后堆保持原样。`to_well_formed.js` 打印 Node 24 对含孤立代理项的字符串做 `toWellFormed` 的结果，孤立代理项放在它的循环读中断标志的位置附近（每 4096 个字符一次），也包括很长的一串代理项对。`copy_data.js` 打印 Node 24 对展开、剩余和 `Object.assign` 的结果，来源对象的 getter 会删除一个键、让它不可枚举或添加一个键（见 [objects-and-shapes.md](changes/objects-and-shapes.zh-CN.md#展开rest-和-objectassign-在轮到每个键时才查找它)）。`object_literal.js` 打印 Node 24 对对象字面量的结果，一连串普通的键会一次做好（见 [objects-and-shapes.md](changes/objects-and-shapes.zh-CN.md#对象字面量直接按最终的-shape-创建)），并要求创建又丢弃函数字面量之后同样如此。`fused_ops.js` 和 `typeof_is.js` 运行两次，分别开启和关闭一次做几条指令的操作码（`jsrun -no-fused-ops`），它们的文件是引擎在有这些操作码之前打印的内容（见 [c-engine.md](changes/c-engine.zh-CN.md#一次分发做几条指令的操作码)）。`share_slots.js` 同样运行两次，它的文件就是 Node 24 打印的内容（见 [c-engine.md](changes/c-engine.zh-CN.md#共用槽位的局部变量)）。还没有文件的脚本会让这次运行失败：`COMPARE_UPDATE=native engine/test/compare.sh` 根据 C 树打印的内容写出缺的文件，提交之前要先看一遍。先构建宿主。输出在 `engine/test/out/compare-MODE/` 里。`jsrun` 以非 0 状态退出时，即使输出和文件一致，这个用例也算失败（释放运行时时发现的泄漏，会在所有输出写完之后以 87 结束运行）。`engine/test/compare-test.sh` 用一个代替 `jsrun` 的程序测试这一点。`compare.sh` 设了 `UBSAN_OPTIONS=halt_on_error=1:exitcode=86`，所以 `ubsan` 模式下 UndefinedBehaviorSanitizer 的报告会让运行结束，而不是被打印出来后放过。然后它运行 `unit`（`engine/test/unit.c`），通过公开 API 检查脚本看不到的东西，比如缓存留下和放掉什么，以及递归的限制：普通函数递归到调用深度（5000，可调高到 10000，可调低到 100，超过限制或代码正在运行时被拒绝），回调进来的宿主函数会数它的进入，嵌套在 JSON、解析器、正则、`flat` 和 proxy 里停在内部深度，从 C 递归到 C 的链（`instanceof` 和 `Reflect.construct` 下的 300000 个绑定函数，`Promise.prototype.finally` 链，迭代器辅助方法）以 `RangeError` 结束，模块链也是（import，从字节码读取或通过加载器读取，`export * from`，`export { x } from`）。改写成循环的递归保持结果不变：两端都建过并重新平衡的 rope（几段字符串连成的树，读的时候才拼成一段），释放一百万个嵌套对象和每种对象的一层深度，大小写转换和每个码点的四种规范形式以及 emoji 序列（对照递归代码的哈希），正则里 `\q{...}` 的字符串，带 reviver 的 `JSON.parse` 的解析记录，每层 3 个的括号，以及嵌套到解析器能到的最深处的函数。一条模块链，如果它的解析或函数在超过内部深度之后失败了，就在同一个上下文里从中间重新运行。创建惰性属性（`Reflect`、`JSON.parse`、函数的原型），以及在 String 对象和两种 arguments 对象上定义属性或添加元素，在内部深度处抛 `RangeError`，低一层则正常工作。对五段使用外层函数变量的代码片段，编译在每一次计数处依次失败（宿主的 `host_stack_fail_at` 让栈在第 n 次计数时看起来不够），要么抛 `RangeError`，要么运行得到结果。每次之后，运行时都还能运行代码，递归到和之前一样深，并且释放后不留任何块。宿主函数看到的深度（`JS_GetCallDepth`、`JS_GetInternalDepth`）是当前打开的层数，写入会移动限制开始计数的位置（在限制处什么也不运行，低三层时可以放进一次调用，超过限制的值会被保留但拒绝运行，范围之外的值什么也不改变），两个互相调用直到 `RangeError` 的运行时，在彼此把自己的深度交给对方时，形成一条长度为一个限制的链，不交时则是两倍。内部深度的限制是构建时的常量，用数字写下来：JSON、括号和 proxy 链分别是 2000、665 和 2000 层，下一层就抛错。每个读中断标志的地方都单独测试：脚本从工作中间设上标志（通过参数的 `toString` 或 `valueOf`，或者脚本调用的函数），解释器、正则的执行和编译、字符串搜索、`repeat`、类型化数组的搜索、`fill` 和 `copyWithin`，以及 `JSON.parse` 和 `JSON.stringify`，都必须抛 `interrupted`，外面包的 `try` 无法捕获它，而同样的工作在没设标志时要运行到结果。正则在工作的过程中读中断标志，而不是工作结束时才读：6 个在 4Mi 个字符的字符串里什么也找不到的正则（8 位和 16 位，有的能找出匹配可以从哪里开始，有的匹配在每个位置都失败），7 个在一次匹配里运行很久的正则（在 128Ki 个字符上的贪婪 `x+`），以及其中 6 个粘性（sticky）的（匹配只试一次，所以只有不压入任何东西的拆分操作才读标志），如果标志在字符串被转换时就已设上，就抛 `interrupted`，宿主在第 50 次读取时设上标志（`host_poll_stop_after`）也一样。`lre_skip` 每看过 65536 个字符读一次标志，而不是在字符串末尾才读，次数按 `host_poll` 的读取数（`host_poll_reads`）计，用的是手工编译的正则和字符串。`lre_canonicalize` 在非 unicode 模式下，对每个码元，对 128 以上的字符都不给出 ASCII 字符。`Date.now`、`new Date()` 和 `Date()` 给出由 `host_set_wall` 设定的墙上时钟的整毫秒，从 2023 年和从 1970 年起都是如此（1970 年以前不是整毫秒的时间向零舍入）。一个线程在对 4Mi 个元素的类型化数组排序到第 20 ms 时请宿主停止运行时，必须在 0.2 秒内停下，元素保持不变，而 `Array.prototype.sort` 必须在某个元素的转换设上标志之后的下一次比较处停下，不写回。`rqsort_stop` 在设上停止的那次比较之后最多再做 14 次比较，对 2 到 64 个元素的排序在每次比较处停止，对 100 到 5000 个元素的排序在分布其中的若干点停止（上限在 `cutils.h` 里）。由普通键组成的对象字面量，得到的属性数组大小恰好是它的大小。在 `systemd-run -p MemoryMax=4G` 或类似的限制下运行它：出了错的链会很快吃掉内存。

- `unit-oom`（`engine/test/unit-oom.c`，由 `build.sh` 构建，`compare.sh` 在 `unit` 之后运行）在内存所剩无几时运行引擎，这是脚本做不到的。它扫过留出的空间大小（把 `JS_SetMemoryLimit` 设成运行时已占的内存加上一段余量），或者先填满小块 arena 里的每个空闲槽（光靠限额仍然会从空闲槽里放出小块），再释放几个块。`JS_ThrowError2` 抛出的要么是 null，要么无法捕获，要么是带字符串消息的完整错误，绝不会是消息就是异常标记的错误，留出一些空间就给出完整的错误。内置函数的 `TypeError` 和 `eval` 的语法错误，函数名和文件名取非 ASCII 的，在每种不超过 24 KiB 的余量下创建，它们的栈要么完整要么为 null（语法错误的栈还可能不存在，因为它保持为异常），要么运行以内存耗尽错误失败，并且不会读已经释放的东西。从通用转换和属性路径里拆出来的路径（用字符串键读写快速数组的一个元素，把 rope 转成数字，是和不是类型化数组规范数字键的字符串，通过 `valueOf` 设置长度，把 rope 变平，把对象转成字符串和键），在每种不超过 24 KiB 的余量下，要么给出同样的结果，要么以内存耗尽错误失败。空字符串能在堆已满时创建。`FinalizationRegistry` 的回调会运行，或者在回收器没有内存给它时丢掉它的任务，且不留任何东西。`JS_ToCStringLen2` 在 UTF-8 副本失败时放掉它的参数。`JS_DefinePropertyValue` 失败时返回 -1，保留对象，并释放给它的那个引用。用 `JS_NewHostFunction`、`JS_NewHostFunctionRelease` 或 `JS_NewCFunction2` 创建的函数，名字是数字，在每种余量下、在堆满但归还了几个块时、以及在堆满但除一种大小之外的每种大小都归还了块时，要么是完整的（有名字和长度，没有待处理的异常），要么调用返回异常、待处理的是内存耗尽错误且不留任何东西，`JS_NewHostFunctionRelease` 创建的函数在被释放时恰好释放一次它的句柄，调用失败时从不释放。编译（16 个脚本，以 32 字节和 4 字节为步长）和写字节码（写出来的字节码能运行）要么成功，要么以内存耗尽错误失败并让运行时继续可用，`DynBuf` 在一次增长失败之后不再多拿字节。一个有 600 个新名字、会让 atom（引擎给属性名和常用字符串的编号） 表增长的脚本，编译要么成功，要么以内存耗尽错误失败，从字符串生成 600 个属性键的代码也一样，在每种不超过 64 KiB 的余量下。读一个待处理的错误栈，余量从零到 16 KiB，函数名取非 ASCII 的也做一遍，给出完整的文本或内存耗尽错误，之后再读又是完整的。给 `stack` 赋值或定义它不需要内存，对 shape 是共享的错误也一样。打印一个错误，包括原始转储，不会留下异常。在没有内存时抛一个没有栈的错误不会崩溃。编译七个 `\p{Script=...}` 和 `\p{Script_Extensions=...}` 的模式，余量从零到 96 KiB，以 8 字节为步长，要么成功，要么以内存耗尽错误失败（`unicode_script` 里的一个死循环曾让它崩溃）。`JSON.stringify` 和 `JSON.parse` 对长字符串抛内存耗尽错误，内存回来之后给出正确的字符串。在第一次访问属性时才创建的内置函数或对象，或者函数的原型，如果创建时内存耗尽，就留到下次访问时再创建，在全局对象上是这样，通过写入继承了它的对象也是这样，而且它已创建的东西一个不丢。内存耗尽不会被 18 种在退出路径上捕获或运行代码的结构（`catch`、`finally`、迭代器的 `return`、async 函数、promise 任务等）中的任何一种捕获，在正则引擎里也一样，而脚本自己抛出的东西从来不是无法捕获的。中断结束一次调用的方式和内存耗尽一样：这 18 种结构，以及从字符串、数字或数组上取到的 getter 或方法（`'abc'.boom`、`(1.5).boom`、`[1].boom` 和各种方法），以 `InternalError: interrupted` 结束，`catch`、`finally`、迭代器的 return 或 rejection 处理函数都看不到它，或者以内存耗尽错误结束。以内存耗尽错误结束的任务或 `async` 调用，可能让它保持待处理并返回一个值，所以宿主在每次之后都要问 `JS_HasException`。泄漏了一个块的运行时以状态 87 结束进程。

- `unit-api`（`engine/test/unit-api.c`，由 `compare.sh` 运行）通过公开 API 检查运行时在出现错误及其栈之后持有什么。读取 10000 个一直待处理的错误的栈，最多增加 2 个 shape（每个错误先得到另一个属性时也一样），原型被替换过的错误不会保留旧的原型，把栈读两次得到同一个字符串，2000 个带栈中函数的错误和一个持有错误的闭包都会被释放，脚本运行时产生的错误不会保留脚本的代码，`JS_ParseJSON` 对 31 个是或不是 UTF-8 的字节串，给出码元，或者给出错误和它的位置。宿主用 `JS_SetPropertyFunctionList` 放在 proxy 上的属性，先于任何陷阱被找到，proxy 被撤销之后也一样。普通对象（300 个，有 70 个属性和空洞，被冻结，被密封，用作原型，用作弱集合的键，处于循环中）经过引擎，`JS_ComputeMemoryUsage` 对每个的计数少于 64 字节，如果用量遍历或不透明 getter 碰到了普通对象没有的那一部分，系统 malloc 的构建（`asan`）会把它当作越过块的读取抓出来。普通对象的 `JS_GetOpaque`、`JS_GetOpaque2` 和 `JS_GetAnyOpaque` 不给出指针。`JS_NewHostFunctionRelease` 的函数通过 `host_release` 恰好释放一次它的句柄：在它最后一个引用被丢掉时，在回收一个环时，或者在运行时被释放时，`JS_NewHostFunction` 的函数则从不释放。`JS_RunGC` 把下一次回收的阈值设成它留下的量的 1.5 倍，不管原来是多少，所以它之后的第一个对象不会启动回收，阈值为 -1 时保持不变。`JS_FreeModuleTopLevel` 释放运行过的模块的顶层，包括过了顶层 `await` 的，并保留它的函数的变量和它的命名空间。1000 个停在 `for in` 里的生成器，遍历 40 个字符串键的对象，占的内存不比遍历 1 个键的对象多：循环持有的是对象的 shape，而不是它的键列表。`JS_NewEscapeTextFunction` 的函数给出 React DOM 的 `escapeTextForBrowser` 的文本（它的生产构建的文本在测试里），覆盖 13 个字符上所有不超过 4 个码元的字符串，两种宽度、1 到 100,003 个码元、五个字符中的每一个出现在开头、末尾和分块边缘的字符串，rope，34 个不是字符串的值，以及统计被调用次数的对象。没有要转义的内容的字符串，返回同一个指针。调用前设上的中断，以及工作中途设上的中断，都会让它停下。它在每个 4096 码元的块开始处读标志（计数阶段的第一个块除外，写入阶段则在第一个块之前读，用 `host_poll_reads` 数）。第一次调用把拼接得到的长 rope 变平时，它能被停下（带着错误，并且不保留变平后的副本）。结果装不下时，它以内存耗尽错误结束并且不留任何东西，结果超过最长字符串时抛 `RangeError`。
- `unit-white`（`engine/test/unit-white.c`，由 `compare.sh` 运行）测公开 API 够不到的东西。`white.c` 就是 `quickjs.c` 本身，末尾加了几个函数，像引擎那样构建，并代替 `quickjs.o` 链接，而用系统头文件构建的 `unit-white.c` 调用它们。它给 `JSON.parse` 和 `JSON.stringify` 的静态运行喂各种空间和种类的字符串缓冲区（9 种空间，4 种填充，8 位和 16 位，138 个文本，以及没有剩余内存时的同样运行，还有一个被做成看起来和最长字符串一样长的缓冲区），检查它们留下的字符和 `string_buffer_putc` 会留下的一样，并且从不越过空间。它检查 `JS_ThrowError2` 同时最多生成两个错误：在两个之内抛 null，无法捕获，在零个或一个之内就生成错误，每次都让计数保持原样。它检查 `js_function_set_properties`（给每个新函数设上长度和名字），在名字是数字（索引 atom 不持有字符串）而满堆没有空间放这个字符串时，在上下文为新函数保留的 shape 上和在同时定义两个属性的路径上，都返回 -1 并留下待处理的内存耗尽错误，有内存时则两个都给出。它检查编译器在没有内存添加这些变量时，只以内存耗尽错误失败，不会有别的错误：`this`、`new.target`、home object、活动函数、`arguments`，或者函数表达式的名字（函数的代码或其中的箭头函数用到它们），以及直接 eval 可以用到的变量，而有内存时每一个都会添加。它检查普通对象的块容纳的空间小于一个 `JSObject`，数组的块则容纳全部。对象字面量的块还有放它的属性的空间，`test_object_inline_props` 和 `test_object_inline_oom` 检查哪些对象经过各种改动后仍把属性放在自己的块里，并在一个对象增长或收紧时，让分配器的每次调用依次失败（见 [objects-and-shapes.md](changes/objects-and-shapes.zh-CN.md#对象字面量把属性放在对象自己的内存块里)）。它检查只看 `stack` 的属性特性的操作会让待处理的栈保持待处理，标记的类没有调用或奇异（exotic）处理函数，变量引用从不被读成标记，有 1001 帧的待处理栈会算进内存用量，回收器的年轻代列表和老年列表只含有计数且没有标记的对象：在第一次年轻代回收之后，在 `JS_ComputeMemoryUsage` 之后，以及在一次释放只有垃圾持有的东西的年轻代回收之后，对盯着该对象的 weak map、`WeakRef`、registry、async 函数和生成器都检查。宿主类的终结器和 `host_release`，如果在引擎释放对象的过程中（丢掉一个数组，丢掉每个年轻对象和每个老年对象，在完整回收和年轻代回收里回收一个环，丢掉被年轻环持有的老数组，丢掉键已死的 `WeakMap` 的值以及键在环里的 `WeakMap` 的值，释放运行时）调用 `JS_RunGC`、`JS_RunYoungGC`、堆已超过阈值时的 `JS_NewObject`，或 `JS_ComputeMemoryUsage` 和 `JS_DumpMemoryUsage`，不会启动回收（`JS_GC_RUN` 在引擎里是空的，`white.c` 用它数运行了几次回收），每个对象都恰好被终结一次且不留块，释放之后的下一个对象，在释放没有重新设阈值时，会启动阈值要求的那次回收（`test_gc_in_free`）。每个用例在自己的进程里运行，也就是把测试带上用例作为参数重新启动，启动时不持有宿主的任何锁。它检查解释器两次计数之间的字节码上限，用的是它自己写的字节码：`check_polls` 接受 128 条指令的路径，拒绝 129 条的，在直线代码里，在向前跳转之后（以及 `with_` 之后），以及在 `finally` 块里，把向后跳转、一个 `OP_nop` 和一个 catch 当作计数，并拒绝未知的操作码，被末尾截断的指令，跳到指令中间或代码之外，以及跑出末尾的代码。`insert_polls` 在路径到达 127 条指令处放一个 `OP_nop`，在结束路径的指令之前不放，并让每个跳转仍落在它的指令上：到不了的短跳转改用长形式。一个 400 行的函数得到它的 `OP_nop`，能运行，并保留它的错误所在的行。它的字节码能读回，但把它的 `OP_nop` 换掉或带未知操作码就不行（`SyntaxError: invalid bytecode`），关掉 `insert_polls` 之后，编译器会在自己的输出上停下（`InternalError: compiled bytecode`）。`check_polls` 还拒绝跳到 -1，也就是代码之前的那个字节，短跳转、长跳转和 catch 都一样。用 `JS_READ_OBJ_POLLS_CHECKED` 读取时不扫描轮询：违反规则的代码带这个标志能读，不带就是 `SyntaxError`，指向不跟在任何 `OP_gosub` 后面的位置的 `OP_ret` 仍然是 `InternalError`，`finally` 块仍然运行。读回来的字节码里，`OP_ret` 带自己的数字就抛 `InternalError: invalid ret value`：指向一条操作数里含 `OP_return_undef` 的指令的中间，指向一条 255 条指令、没有计数、运行在 `check_polls` 接受的两段指令的操作数里的路径，以及指向不跟在任何 `OP_gosub` 后面的指令。读回来的 `finally` 块能运行。对象字面量的字节码能读回并运行，但改了它的 `OP_object_lit` 的计数或索引就不行（`SyntaxError: invalid object literal`）。键相同、写在不同位置的对象字面量共用一个 shape，也会采用一个逐个加键得到的对象的 shape（`test_object_lit_shared_shape`）。编译器在开启时，把 `fuse_code` 的各段和 `typeof` 测试合成一个操作码，关闭时只用以前的形式，当有跳转落在其中一段内部时让这一段保持分开，这样的字节码能读回并运行，但 `check_polls` 拒绝一个被改过的融合操作码所在的那一段（`test_fused_ops`、`test_typeof_is`）。`test_short_opcode_index` 检查反汇编器为每个把索引放在操作码里的操作码打印的索引，融合读取也包括在内。`test_share_local_slots` 数 18 个函数的槽数，这些函数的局部变量有的共用槽、有的不共用，并对照 Node 的结果运行每个函数，还读回一个闭包指名了共用槽之外的局部变量的函数。`test_fused_edges` 在开启和关闭两种情况下，对带 [[IsHTMLDDA]] 的对象、对模块命名空间、对一个刚被直接 eval 或 `with` 块在读取前设过值的参数或局部变量，运行融合读取和 `typeof` 操作码，并检查开启时代码里确有这些操作码。它检查 `JS_GetOwnPropertyNames` 列出 21 种对象自己的键，对它的标志的每种组合各做两遍（第二遍键的种类来自键缓存），和非奇异对象的 shape 所说的一致（索引按值，然后是字符串，再是 symbol 和私有名，种类和旧函数给出的相同），并且对奇异对象不失败。它检查 `JSON.parse` 的 ASCII 一段最迟在 `JS_STRING_LEN_MAX + 1` 字节之后结束，用的是编造的地址（不会真的读一个 GB）。带 `JS_READ_OBJ_SHARED_DEBUG` 时，一个 64 KiB 的函数的源文本和行表指向读入的缓冲区，函数会这样声明，运行时既不统计也不释放它们（少算 64 KiB），它们内容相同。正则缓存只保留模式是字符串、标志是 undefined 或字符串的项（每次运行之后检查链表，哈希链，计数和字节数），失败的编译什么也不保留，它的项，它的表和它们持有的字符串就是它占的内存（关闭它就全部还回），带着项被释放的运行时不留块。`white-regexp.c` 之于 `libregexp.c`，就像 `white.c` 之于 `quickjs.c`，并在链接中取代它的位置：它给 `lre_split` 一个会跳到自己的拆分操作的字节码，调用必须交回拆分和它的字符（如果不返回，20 秒后警报会结束测试）。把 58 个脚本（循环，标签，带字段和私有方法的类，箭头函数参数，解构，带命名分组的正则，模板，异步代码，`with`，`eval` 和四个模块）各编译一遍，让一个请求失败，其余正常，对每个请求依次进行，要么以内存耗尽错误失败且不留异常，要么给出能运行并得到同样结果的代码。这里的请求指向堆的请求（一个新的 arena 或一个大块），第二轮扫描则是对 `js_malloc_rt` 或 `js_realloc_rt` 的任何调用，小块和前端缓存也算在内（`quickjs.c` 里的 `JS_HEAP_FAILS` 和 `JS_ALLOC_FAILS` 在引擎里是 0，`white.c` 用它们）。大小为 0 的调用，以及能放进现有块的 `js_realloc_rt`，不计入：引擎从不让它们失败。每个用例在自己的进程里运行，也就是把测试带上扫描、脚本、请求和它必须给出的结果作为参数重新启动，所以一次中止，一个泄漏的块或一个信号只是以脚本和请求命名的一个失败用例，而不是整个测试的终点。fork 不行：宿主的看门狗线程每秒有一瞬间持有它的锁，在那时 fork 出的子进程会永远等它。进程通过 `host_leave` 离开，它保留宿主对仍在用的块的检查，并跳过 sanitizer 的泄漏检查。在 `asan` 和 `asan-ubsan` 构建里，每个块都是一个独立的对堆的请求（它单独来自系统的 malloc），所以两轮扫描在那里几乎一样，两轮都运行。脚本在 `compile-scripts.h` 和 `unit-white.c` 里。往局部变量、参数或闭包变量上追加而构建的字符串，超过 QuickJS 会改成 rope 的长度之后仍保持为一个平坦字符串，而一个属性名（atom）的字符串，如果它最后一个别的引用由某个变量持有，就会被复制，从不原地增长，这样之后用同样文本生成的键仍是同一个属性。那个测试不在 `asan` 和 `asan-ubsan` 构建里运行：系统 malloc 的块在所要求的大小之后没有余地，所以不会有原地增长。`test_poll_work_gaps` 检查一个长循环在两次读中断标志之间做了多少工作，而不只是读得多频繁：循环用 `JS_WORK(n)` 说明自己做了什么（在引擎里是空的，由 `white.c` 计数，每次读标志时它结束一段），一个填充物在一步中间结束的补齐，一个扫描在一步中间结束的切片，以及由 65534、65536 和 65535 个字符的叶子构成的 Map 或 Set 的键，每个都必须让每一段（第一次读之前的那一段和最后一次读之后的那一段也算）精确到各自循环的步长，并且加起来等于它们必须做的工作。`test_poll_work_gaps` 的每个用例都给出它的总数，用例包括带转义的、以及在很长一串 ASCII 之后带一个宽字符的 JSON 文本，同一个哈希的 64 个键组成的链，在相等的长字符串上的搜索和 `Array.prototype.indexOf`，以及 `JSON.stringify`。`test_buffer_copy_work` 给出一个必须增长的 1,500,000 字符的字符串缓冲区在复制和加宽时做的工作。

### 分配器

```sh
engine/test/tlsf-edges.sh                        # the edge cases of TLSF, on the pinned file and on engine/tlsf.c
engine/test/build.sh plain && engine/test/heap.sh    # scenarios and random sequences, on the heap and on the runtime
engine/test/build.sh fuzz && engine/test/heap.sh -f 300   # the same sequences as a libFuzzer target, for 300 seconds
engine/test/build.sh plain && engine/test/out/plain/render bundle=B.js inputs=A.json,C.json soak=3000
```

### 生成引擎的 Go 宿主

`internal/gohost` 回答引擎的回调（内存，时钟，轮询，`host_call`），它的测试在任何机器上都能运行，不需要引擎。`internal/cengine` 是生成器写出 Go 汇编形式的引擎的地方，旁边是提交进仓库的宿主文件。它由 `make gen` 重新生成（见 [internal/goasm/gen/README.md](../internal/goasm/gen/README.zh-CN.md)）。测试 Go 宿主和引擎：

```sh
go test -race -tags testhooks ./internal/gohost         # VMs by id and by shadow stack, chunks, release and remap, clocks, poll, abort, what is left when the OS refuses
go test -tags testhooks ./internal/cengine      # scripts on the engine: results, leaks, footprint, release and remap counts, Go heap, the shape cache, Close and New under panics and refusals, polls and Interrupt
go run ./internal/cengine/cmd/jsrun -n 10 -mem file.js   # one script, timed, on the C engine
go test -p 1 .                    # the tests of package quickjs on the C engine (linux/amd64, linux/arm64, darwin/arm64)
go run ./internal/cengine/cmd/jsrun -n 10 file.js                      # the same on package quickjs
go test -run 'TestShadowStack' -v .      # how far up its 32 MiB shadow stack a VM goes at depth limits of 5000 and 10000, and in C recursion
go test -tags testhooks -run 'TestMapRefused|TestReleaseRefused|TestRemapRefused' .   # the system refuses memory in the middle of a render
go test -tags testhooks -run TestInterruptStopsLongBuiltins -v ./internal/cengine   # long calls of built-ins stop within a few polls of Interrupt
go run ./internal/cengine/cmd/interrupt -n 10       # time from Interrupt to the return of Eval, for the scripts of cmd/scenarios
go run ./internal/cengine/cmd/gcpause -secs 10      # how long the Go GC waits to stop VMs that stay inside one operation, for the same scripts
```

引擎只在 linux 和 darwin 的 amd64 与 arm64，以及 windows/amd64 上构建，其他任何平台都会得到 `internal/unsupported`，它的错误写明支持的平台。darwin/amd64 和 windows/amd64 运行的 amd64 代码就是 linux/amd64 运行的那份。在 Apple 芯片的 Mac 上，`GOARCH=amd64 go test` 通过 Rosetta 运行它。在那里，进程看到的页是 4 KiB，而内核按 16 KiB 的页分配内存，所以 `internal/hostmem` 的测试对每个碰过的字节都数出 4 页常驻。在 linux/amd64 的构建机上用 `-arch arm64 -mode pn` 生成 arm64：每种架构的文件都以它的名字结尾，所以两种架构能同时放在 `internal/cengine` 里。同一个 arm64 包在 Mac 上运行（像上面那样在那里运行测试），也在 linux/arm64 上运行，CI 在 `ubuntu-24.04-arm` 上跑它。构建机没有 arm64 的核：要在那里运行 linux/arm64，就交叉构建测试二进制（`GOOS=linux GOARCH=arm64 go test -c -tags testhooks`），并在各自包的目录里用 `qemu-aarch64` 运行，模拟器有三处限制：把测试二进制重新作为子进程启动的测试（`TestExceptionConversionThrows`、`TestCloseRetry`、`TestCloseTwice`、`TestRegisterExistingName`）会失败并报 "exec format error"，除非 binfmt 来运行它，`TestStackNoHugePages` 会失败，因为 qemu 不转发 `madvise(MADV_NOHUGEPAGE)`，而 `TestInterruptStopsLongBuiltins` 曾在那里失败，在 50 ms 宽限后被杀，因为它按时钟给停止计时，而杀进程只按时间进行。链接后的检查（`-verify-bin`）读五个平台中每一个的测试程序，在哪台机器上构建都行：`GOOS=darwin GOARCH=arm64 go test -c -tags goasmkeep ./internal/cengine`，其他平台同理，交给该架构的生成器运行（arm64 用 `-arch arm64`）。它读 linux 的 ELF、darwin 的 Mach-O 和 windows 的 PE，就是 `go build` 默认做出的样子。在 darwin 和 windows 上，那是一个加载器可以放到任意地址的程序（PIE），检查要求 `engine_rodata` 的每个指针在文件里是它的目标的地址，并且有一条告诉加载器移动它的记录：dyld info 里的一个 rebase，ld64 链接带 cgo 的 darwin 程序时写的 chained fixups 里的一项，或者基址重定位表里的一个 DIR64 项。加载器在检查读过的代码或数据里写的任何别的地方都会让检查失败，比如 PE 导入的一个槽，同一个地方有两条记录，检查不读的记录表，落在链接器放指针的节之外的记录，或者检查为了找出记录而读的表上的记录，也一样失败。它读的每个字节都经由加载器从文件映射的内容，而不是经由节。它只读静态的 ELF 可执行文件，也就是 linux 上不带 cgo 时 `go build` 做出的那种，遇到 ELF 的 PIE（`-buildmode=pie`）或者带动态表、程序解释器或重定位节的文件就停下。

要检查引擎的两个构建打印的内容相同，就把用两个构建各自构建的 `internal/cengine/cmd/jsrun` 跑在 `engine/test/compare.sh` 的脚本上，逐个文件比较输出。`compare.sh` 记录的输出在 darwin/arm64 和 linux/amd64 上相同。

引擎的正式构建给生成器传 `-release`：遇到任何桩就停下，并拒绝为勘察和测试做的开关（`-keep-going`、`-poison`、`GEN_KEEP_NARROW` 和 README 列出的其他开关），所以它写出的包里没有 `goasmMissing` 桩。在 C 里最后一个桩消失之前，它会停在那一个上。

生成器每次从同样的 C 写出同样的包，逐字节相同（`TestOutputReproducible` 用 1 个和 4 个编译任务运行它，并比较每个文件）。

`jsrun` 每次运行只对一次调用计时，也就是执行脚本、运行它的任务并把结果转成字符串，它的 `timed:` 行这样说明。源码已经是字符串，VM 的创建、回收、释放和关闭都在计时之外。

标签 `testhooks` 打开这些测试的钩子，普通构建里没有它们：`hostmem.SetFault` 让对操作系统的调用失败，`internal/cengine` 的 `callbackHook` 在回调里停住引擎（旁边还有一个引擎拆除次数的计数），`gohost.VM.InterruptAtPoll` 让一次设定的轮询请求停止，在运行引擎的 goroutine 上。没有这个标签，代码里就没有针对它们的检查，所以普通构建不付任何代价。不带标签的运行会跳过需要它们的测试，并这样说明（`TestFailingCalls`、`TestRefusedCalls`、`TestLifecycleHooks`），所以整套测试都要带这个标签。

`TestFault*`（`internal/cengine/fault_test.go`，带 `-tags testhooks`）故意让引擎失败，办法是用没有哪个调用者会给的东西去调用它的入口：通过指向未映射内存的指针读和写（0xdead0000，1 << 46，0x4000，在 linux/arm64 上它低于 Go 链接代码的 0x10000，还有一个在 amd64 上不是规范形式的地址），给一个入口一个什么也不剩的影子栈，`abort`，以及一个陷阱（`callOverrideHook` 回答 abort 自己的 `host_call`，这样它之后的代码，即 `host_trap` 的调用，能运行）。每一个都必须是一个能 recover 的 panic，VM 损坏，除 `Close` 外每个入口都返回 `ErrBroken`，宿主在 `Close` 之后不持有内存，goroutine 的 `SetPanicOnFault` 保持原样，并且同一个进程里的新 VM 能运行脚本。它们在 windows 上也运行：在那里，生成的引擎为系统在出错时写到栈指针下面的 CPU 状态留出了空间，其他在 Go 代码里故意制造错误的测试在错误之前紧接着调用 `faulttest.Room`。只有 `TestRegistry`（`internal/gohost`）里错误发生在 `ByStack` 内部的两个用例在 windows 上跳过（`faulttest.Unsafe`，[docs/changes/c-engine.md](changes/c-engine.zh-CN.md#windows-上给故障留的空间)），因为那里没法留出空间。`TestCallbacksMakeRoom` 要求生成代码调用的每个宿主回调最后都再留一次空间，`TestRoomCheck`、`TestRoomCheckAssembles` 和 `TestPostLinkFormats` 里 windows 的用例（`internal/goasm/gen`）检查生成器写的空间检查、windows 的汇编器接受它，以及链接后的检查拒绝没有它的二进制。`TestFaultsAmongVMs` 在 4 个各自在自己的 VM 上运行脚本的 goroutine 旁边制造 40 个错误。`TestFaultNoSplit` 有两个用例，在没有栈检查的函数里出错（生成器的 NOSPLIT 列表，`noSplitList`），从全新的 goroutine 发起调用，在它们栈的 12 KiB 范围内每隔 8 字节调用一次。引擎列表里的每个函数都是什么也不调用的叶子，所以它的调用者有栈检查，为出错时添加的 `runtime.sigpanic` 调用和它的 morestack 调用留出了空间。每个用例都要求，在 `runtime.Callers` 里，从出错处往上列表中的引擎栈帧恰好就是它指名的那个叶子，每种架构上都是如此。`unicode_to_utf8` 是 Go 汇编器自己标为 NOSPLIT 的叶子，给它一个指向无处的缓冲区，它在自己里面出错。`goasm_chunked_memcpy` 被给了一个指向无处的源，在 `memcpy` 里出错，`memcpy` 是一个寄存器入口的叶子，在有栈检查的 `goasm_chunked_memcpy` 之下。如果引擎的某个改动让其中一个函数调用了别的东西，或者在那里内联了 `memcpy`，用例就会失败，需要另一个叶子。生成器的 `TestSampleRuns/nosplit_fault` 检查一个有 24 层调用链的样本的 NOSPLIT 列表（就是它的两个叶子），从以最小的栈起步的 goroutine，在最底下的叶子里出错，并要求在只有 sigpanic 让栈增长的地方出错。同样的错误，以及一次除以零，也在 proto 包生成的代码上运行（`testdata/fault_test.go.txt`），arm64 也一样：在 linux/amd64 上用 `ARCH=arm64 OUT=internal/goasm/gen/internal/proto-a64 sh internal/goasm/gen/proto.sh` 生成它，把目录复制到 Mac 上，在那里运行 `go test -vet=off`。`TestBigFrames` 找名为 `proto` 的包的栈帧，所以在那里要把目录命名为 `proto`，否则它只因为名字就会失败。`go test -run '^$' -bench 'Enter|EvalTiny' ./internal/cengine` 和 `-bench EnterLeave ./internal/gohost` 给出一次进入的代价，其中包含 `SetPanicOnFault`。

轮询测试（`internal/cengine/poll_test.go`）需要用 `-poll` 生成的引擎，没有它就自己跳过。`TestPollEveryCycle` 运行解释器的紧循环（包括通过 `do while` 向回跳的循环，`continue` 到标签，catch，`finally` 块和 `with`，以及一个 3000 行、没有跳转的函数）和 C 的紧循环（类型化数组，正则，字符串搜索，`repeat`，排序，JSON），以及分配类脚本，分别用默认的间隔和间隔为 1（让每次计数都轮询）：结果必须相同，这说明轮询保住了生成的代码放在寄存器里的东西。`TestGCInTightLoops` 在另一个 goroutine 不断启动回收的同时运行它们。`TestInterrupt`（带标签 `testhooks`）让每个无限脚本的一次设定的轮询请求停止，并要求在继续运行的部分抛出 `InternalError: interrupted`（栈顶的帧：脚本的循环，或者内置函数），在 `gohost.KillPolls` 次轮询之内，并且 VM 在这之后能运行代码和捕获异常。`TestInterruptOneProc` 在 `GOMAXPROCS=1` 下从另一个 goroutine 调用 `Interrupt`，那个 goroutine 只有在引擎的 goroutine 在轮询处停下时才能运行。`TestKill` 杀掉一个没有及时读标志的脚本，宽限期分别为 30 ms 和 0：得到 `ErrKilled`，VM 损坏，`SetPanicOnFault` 保持原样，新的 VM 能运行。`TestNoKillBeforeThePolls` 以 0 的宽限期中断每个无限脚本：每个都在 `gohost.KillPolls` 次轮询到来之前以 `InternalError: interrupted` 停下，没有一个被杀。`TestTimeout` 在 VM 的超时时停下一个无限循环，每个 `Eval` 都有完整的超时，并在截止时间之后的宽限期结束时杀掉一个 join。`TestNoKillOutsideScripts` 在 VM 回收的同时不断调用 `Interrupt`，宽限期为 0，每个周期都轮询，什么也没被杀。`TestPollKill`、`TestPollStallNoKill` 和 `TestPollDeadline`（`internal/gohost`）直接调用 `Poll`：杀进程既要有宽限期，也要有轮询，线程停顿超过宽限期但其间没有轮询，不会杀任何东西。生成器的 `TestPassPolls`（`internal/goasm/gen`，需要设 `QUICKJS_LLVM` 和 `QUICKJS_SHADOW_PASS`）检查 pass 把计数放在哪里，放在寄存器里还是内存里，并检查 pass 自己的检查会在 `-shadow-poll-fault` 放进去的每个错误上停下：测试从不轮询的计数，没有入口计数的解释器，没有计数的解释器标记，以及放在寄存器里的计数，它在可能轮询的调用之前没有写回，在调用之后没有重新读取，或者在它的循环离开处没有写回。`TestInterpreterJumpsCount`（`internal/goasm/gen`，不需要 LLVM）读 `engine/quickjs.c` 里的 `JS_CallInternal`，在 `pc` 的移动不是一个常数步长向前、`JUMP_TO` 或 `JUMP_COUNTED` 时失败，在这些不再计数时失败，在入口计数不在最前面或者 `OP_nop` 不计数时失败，并检查把其中每一项弄坏的副本都会失败。`unit-white` 检查 `check_polls` 和 `insert_polls`（见下）。

`TestShadowStackAtDepth` 和 `TestShadowStackInternalDepth`（根包）在 30 种递归和 11 种 C 递归之后，读影子栈里最高的非零字，并记录每一层占的字节数。每种递归都必须以 `RangeError` 结束，带普通栈帧的递归必须留下四分之一的栈。`TestHasOwnCallDepth` 通过 `hasOwnProperty.call(obj, key)` 递归，解释器在可能时不做那两次调用而直接回答，栈帧里有 0 到 250 个局部变量，并要求它和用数组做的同样递归在同样的深度结束，后者总是做那两次调用，在深度限制处和栈末尾都一样。`TestCCallDepth` 通过解释器自己调用的 C 函数（`call`、`apply`、`map`、`sort`、`Math.max`、没有接收者的 `abs` 和两个尾调用）递归，有 0 到 300 个局部变量，限制为 5000 和 10000，并要求开启和关闭这些直接调用（`JS_SetDirectCCalls`）时，`RangeError` 的深度和栈相同。`TestCCallDepthAfterExits` 通过 `RangeError`、被捕获的 `Error` 和中断离开嵌套的直接调用，同一个 VM 里的下一次递归必须和第一次结束在同一处。`TestMapRefusedSweep`、`TestMapRefusedAmongVMs`、`TestReleaseRefused` 和 `TestRemapRefused`（`-tags testhooks`）让 `hostmem.SetFault` 在脚本运行中间拒绝系统的内存块、释放和重新映射，对每个内存块依次进行，并对四个 goroutine 中的一个进行：被拒绝的那次调用以内存耗尽结束，`Close` 之后没有任何映射残留，其他 VM 都不失败。`TestInterruptStopsLongBuiltins` 运行 `replace`、`fill`、`join`、`normalize`、`localeCompare`、`toUpperCase` 和 `toLowerCase`（还包括在 4 Mi 个大小写无关的字符之前和之后各有一个大写 sigma 的情形）、`isWellFormed`、`toWellFormed`（开头和末尾各有一个孤立代理项，以及开头一个孤立代理项后面跟着很长一串代理项对，修复的循环每次处理两个字符）、分隔符不存在的 `split`、`padStart`、`padEnd`、`trim`、`trimEnd`、`anchor`、`encodeURI`、`decodeURI`、`escape`、`unescape`，以及两个 rope 的 `<`，输入的大小让每次调用至少要 640 次轮询，让一连串调用里的第一次调用，在均匀分布的 6 个位置上的轮询请求停止（`gohost.VM.InterruptAtPoll`，带标签 `testhooks`），并要求错误来自内置函数（栈顶的帧），调用停下之前看到请求的轮询不超过 16 次（`gohost.VM.StopPolls`），并且 VM 没有被杀。生成很多片段结果的内置函数（拆成很多段的 `split`，对字符串和类数组对象的 `Array.from`，对字符串的 `Uint8Array.from`，以及对 Array 的 `new Uint8Array`）运行在回收器关闭的 VM 上，因为调用中间的一次回收本身就是一批轮询。释放它们已经做出的那部分结果，在停止之后要用掉一些轮询，而以加倍方式增长的结果的复制不读标志，所以一次停止除了那 16 次轮询之外，还可能花掉释放一整个结果的时间和调用的四分之一。对这些，只要求错误所在的帧符合：如果它读了标志，帧就是内置函数，没读就是它之后的解释器。位置和停止按轮询次数计，而不按时间，从一次回收开始，并且不把调用的结果转成字符串，所以在任何机器上位置都一样，一台负载高、让线程等待的机器不会让它失败，而它的输入在循环之前做好，保持存活，标记和释放都很便宜，这样脚本停止时的一次回收或释放不会算到内置函数头上。新的、有长循环的内置函数应该放进这个测试。rope 的字符串由 memcpy 复制，计的轮询很少，所以把 rope 变平由 `engine/test/unit.c` 的 `test_rope_polls` 来检查，它数原生宿主读标志的次数（`host_poll_reads`），并在第 20 次时停下一次调用，还有 `test_rope_flat_polls`，它把长字符串的 rope 变平，并在每次的第一次，中间一次和最后一次读取时停下。`test_regexp_compile_polls` 对长正则的编译做同样的事（几百万个字符的模式，以及几百万个字符的单个项：一个计数，一个转义或前导零的分组名，类里的 `&&` 和 `--`，`\q{...}` 的字符串），`test_regexp_cache_polls` 对正则缓存中哈希并比较一个 900,002 字符模式的查找做同样的事。`cmd/gcpause` 和 `cmd/interrupt` 运行 `cmd/scenarios` 的脚本，每个都停留在一次长操作里：`fib`、正则、`JSON`、字符串搜索、`split`、`join`、排序、`fill`、`normalize` 和 BigInt 乘积。`gcpause` 每隔几毫秒强制一次 GC，并报告运行时等待 VM 停下所用的时间。`interrupt` 在随机的点上停下每个脚本，并报告 `Eval` 返回用了多久。数字见 [performance.md](performance.zh-CN.md#负载下的稳定性)。

`TestDeepRecursion` 递归到引擎的调用深度限制以及超过它（得到 `RangeError`，进程不出错）。它需要为整个 32 MiB 影子栈生成的引擎（生成器的 `-shadow-size-log2=25`），对更小的引擎就自己跳过，依据是 `engine_decl_<arch>.go` 记录的 `shadowSizeLog2`，`New` 也按它限定栈的大小。

同样的测试还有两种运行，守护 Go 和生成的代码之间的调用约定。毒化构建（生成器的 `-poison`）在每个入口加载参数之前，每次调用之后读结果之前，以及每次返回时，往寄存器里写垃圾：测试在它上面也必须通过。抢占锤让 goroutine 在引擎里的栈保护不断被设上，这样下一个生成的函数的栈检查就会走调度器。它报告自己发出了多少请求（对保护的写入），这比实际发生的抢占多，因为调度器回应之前发出的请求会合并。它写进运行时的 `g`，所以有自己的标签：

```sh
CENGINE_HAMMER_N=3000 go test -tags cenginehammer -run Hammer ./internal/cengine
```

[allocator.md](allocator.zh-CN.md) 说明了测什么、怎么测。`heap.sh` 接受 `-m MODE`，取 `asan-heap`、`asan-heap-iter` 和 `ubsan`。`asan-heap-iter` 是带 `JS_MALLOC_USE_ITER` 的 `asan-heap`，这是一个引擎不用的、列出所有已分配块的清单：它让这个宏保持能够构建，场景、随机序列和 `poison-test` 都通过。对 `engine/heap.c`、`engine/tlsf.c` 或 `engine/quickjs.c` 里分配器部分的改动，要把这些全部运行一遍：test262 在 `plain` 和 `asan-heap` 下，针对系统 malloc 的 `asan` 跑 test262，以及 tifa 页面的 3000 次渲染的浸泡测试（`render.c`），它的计数、块数和占用在最初的一百次之后不得增长。

## 竞态检测器

引擎本身没有共享状态，但 Go 函数的注册表，错误栈的惰性初始化，以及 intl 共用的语言数据，是进程里所有 VM 共享的。动过它们之后，带 `-race` 运行这些：

```sh
go test -race -run 'TestInterrupt|TestConcurrentVMsMemory|TestErrorStackConcurrent' .
go test -race -run TestErrorStackFirstUseRace .       # on its own, so no other test made a stack first
go test -race -run 'TestConcurrentVMs|TestRegistryConcurrent|TestRegistryGoIntlConcurrent' ./intl
go test -race -run QJS ./internal/gointl               # the packs go-intl's loaders add to Embedded
go test -race -run 'Concurrent|Tamper|Clock' ./temporal  # Temporal on VMs at once, pages changing built-ins, clocks per VM
go test -race -tags testhooks ./internal/hostmem         # VMs mapping memory on many goroutines, and one VM handed between them
```

整个 `./intl` 在 `-race` 下，在 EPYC 的一个核上约需 25 分钟，大部分是 golden 测试：给它 `-timeout 60m`。

竞态检测器会忽略对 Go 堆之外的内存（比如 C 内存里的中断标志）的原子操作。所以 `Interrupt` 和 `Close` 要在 Go 堆上的一个互斥锁处会合（见 [architecture.md](architecture.zh-CN.md#eval-超时和-interrupt)）。

## 每个目标平台

```sh
make build_all_targets
```

它构建每个包，并编译每个包的测试（不只是根包），每个包编译成自己的测试二进制（因为有两个包都叫 `gen`），目标是引擎运行的那些平台（linux 和 darwin 的 amd64 与 arm64，以及 windows/amd64），分普通和带 `-tags testhooks` 两种。它要花很长时间。想快速看一眼，就构建主要的几个：

```sh
for t in linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64; do
	GOOS=${t%/*} GOARCH=${t#*/} go build ./...
done
```

为任何其他平台构建，都会以 `internal/unsupported` 的错误失败，不会有别的错误（`TestEngineUnsupportedTarget` 对 freebsd/amd64、windows/arm64、linux/386 和 linux/riscv64 检查这一点）。用比引擎核对过的版本更新的 Go 构建，会以 `internal/goversion` 的错误失败，不会有别的错误，用 `-tags quickjs_anygo` 则能构建（`TestEngineNewerGo` 手工给出 `go1.N` 标签，包里的 `TestNewerGoStops`）。在 amd64 上带 `-asmflags=-spectre=ret` 构建，或带 `-dynlink`（`-buildmode=plugin`、`-buildmode=shared` 和 `-linkshared` 会传它）构建，会以 `internal/cengine` 里绊线的错误失败，不会有别的错误，而 `-shared`（`-buildmode=pie`）和 `-spectre=index` 仍能构建（`TestEngineAsmOptions` 把这个选项限定在引擎包，`TestEngineBuildmodeShared` 为 linux/amd64 用 `-buildmode=shared` 构建，`-short` 会跳过它）。

## go.work

通常没有 `go.work`。整个仓库，包括 `compare/` 里的 `make benchmark`（它通过自己的 `replace` 找到根模块），都在普通的模块模式下构建和测试。`go.work` 被忽略，`make clean` 会删掉它：把它当作临时工具，不是配置。

只在一种情况下用 `make work` 建一个：对着这份检出，在旁边的另一个模块（比如 tifa）里工作。用完就删掉它（`make clean`，或者 `rm -f go.work go.work.sum`），否则这个目录会一直对着隔壁工作副本里的东西构建，而不是 `go.mod` 里的版本。

只要有 `go.work`，它就必须一直列着 `./compare`。工作区是从当前目录一路往上找到的，所以它也管着 `compare/` 里的构建，那里的构建否则会失败，报 `main module (github.com/liustack/quickjs) does not contain package github.com/liustack/quickjs/compare`。`make work` 把当前的工具链写进 `go` 那一行：留下来的旧工具链的 `go.work`，如果低于 `compare/go.mod` 的 `go` 版本，会让 `make benchmark` 失败，报 `module . listed in go.work file requires go >= …`。

## 开发者工具：生成、检查、计时

make 目标在你所在的机器上干活。`make gen` 和 `make check-*` 的生成器测试只需要装了 LLVM 22.1.8（`/usr/lib/llvm-22/bin`，可用 `LLVM_BIN` 改）的 Linux。`make remote` 通过 ssh 在另一台机器上运行同样的目标，供没有 LLVM 的 Mac 或笔记本使用。

### 生成并检查

```sh
make gen                       # the engine into internal/cengine: the pass, the generator, a formal build (-release)
make gen ARCH=arm64            # the other architecture, next to the first
make check-gen                 # the files checked in are what the generator makes now, for both architectures
GEN_KEEP=.build/gen-base make gen    # also keep a copy of the generated files
GEN_OUT=.build/pl GEN_VERIFY_BIN=linux.test,darwin.test,windows.test scripts/gen-engine.sh amd64   # the post-link check of test binaries of internal/cengine (go test -c -tags goasmkeep)
CHECK_LIST=1 scripts/check.sh full   # the steps and their tier
make check-fast                # every commit
make check-full                # the end of a batch
CHECK_ONLY="goasm root" make check-full     # some steps (CHECK_SKIP leaves some out, CHECK_STOP=1 stops at the first failure)
CHECK_TAGS=quickjs_anygo make check-fast      # with tags added to every go command, such as past the gate of a newer Go
```

生成的引擎是提交进仓库的：`internal/cengine` 里有 amd64 和 arm64 的 `.s`、`engine_decl_<arch>.go` 和 `engine_keep_<arch>.go` 文件（约 18 MiB，压缩后 3 MiB），因为模块的使用者没有 LLVM。对 `engine/` 的 C、生成器或 `scripts/gen-engine.sh` 的参数的改动，要连同 `make gen` 和 `make gen ARCH=arm64` 重新生成的文件放在同一个提交里，没有的话 `make check-gen` 会失败。它比较的是 git 暂存区里的字节，所以在 `make gen` 之后、`make check-gen` 之前要运行 `git add internal/cengine`。不管 `GEN_JOBS` 取什么、在哪个目录里运行，生成器给出的字节都一样。

`scripts/gen-engine.sh` 检查 LLVM 版本，pass 的源码变了就重新构建它，构建生成器，并以正式构建的参数运行它（`GEN_RELEASE=0` 允许勘察和测试用的开关，并且会说明）。`scripts/check.sh` 一个接一个地运行它的步骤，为每一步在 `.build/check-logs/` 下写一份日志（可用 `CHECK_LOGS` 改），失败后继续，最后给出一张表。`go test` 里的跳过会被计数并在表里点名，因为被跳过的测试什么也没检查：预期的跳过只有一个，`TestVarargsNative/aarch64`，它要 `ld.lld` 和一个 aarch64 模拟器。需要引擎的步骤用的是提交进仓库的那个，所以只有在改动需要新文件之后才先运行 `make gen`。

| 步骤 | 档 | 运行什么 |
| --- | --- | --- |
| `build`、`gofmt`、`docs` | 快档 | `go build ./...`，`gofmt -l -s .`（不含生成的文件），`scripts/docs-list.sh --check` |
| `goasm` | 快档 | `internal/goasm/...` 的测试，设了 `QUICKJS_LLVM`、`QUICKJS_SHADOW_PASS` 和 `QUICKJS_LLD`，这样它们不会跳过 |
| `cengine` | 快档 | `go test -tags testhooks ./internal/cengine` |
| `host-plain`、`compare-plain`、`cycles` | 快档 | `engine/test/build.sh plain`、`compare.sh plain`、`cycles.sh` |
| `scripts` | 快档 | `engine/test/compare-test.sh`、`scripts/go-versions-test.sh`：检查所用 shell 脚本的测试，用代替它们所运行程序的桩 |
| `gen` | 全档 | `scripts/check-gen.sh`（`make check-gen`）：把两种架构生成到 `.build/check-gen/` 里，要求 `internal/cengine` 的每个文件字节相同，没有缺的，没有多余的，也没有在 git 之外的。提交进仓库的就是 git 暂存区里的内容（被忽略的文件不算，`make gen` 之后要 `git add` 这些文件），目录必须字节相同，生成的文件靠其中任意位置的 `Code generated` 头找到，旧版生成器的文件名（`engine_decl.go`、`engine_keep.go`、`engine_negative.go`）不管有没有这个头，都算多余的 |
| `math` | 全档 | `engine/test/math.sh`（约 4 分钟，大部分是在 shell 里算哈希） |
| `testhooks` | 全档 | `go test -tags testhooks ./internal/gohost ./internal/hostmem ./internal/idtable` |
| `root` | 全档 | `go test -p 1 .` |
| `race` | 全档 | 根包和上面三个包的 `go test -race -short -p 1` |
| `host-asan`、`compare-asan`、`host-ubsan`、`compare-ubsan` | 全档 | sanitizer 下的 `engine/test/build.sh` 和 `compare.sh` |
| `test262` | 全档 | `engine/test/test262.sh -m plain -T 2` |

这份列表在 `scripts/check.sh` 里。新的检查要加到那里，也要加到上面的表里。

### 给两个构建计时对比

速度结果按 [measuring.md](measuring.zh-CN.md) 的规程来测。`go tool measure bench`（`internal/measure` 里的 Go 程序，只用标准库，在 `go.mod` 里登记为 tool）构建各种布局，在均衡的分块里给它们计时，并判断结果。`make bench ARGS="..."` 运行同样的东西。

```sh
# 1. the 16 protocol layouts of each side. Generate the engine in each tree first (make gen).
go tool measure bench build -side base -tree ../base -app ../tifa -pkg ./test/ssr -test -out bin
go tool measure bench build -side new  -tree .       -app ../tifa -pkg ./test/ssr -test -out bin
# 2. time them in blocks, 2 rounds (even). Records are appended to the file, so a stopped run goes on (shop.jsonl.manifest.json must match).
go tool measure bench run -bin bin -out shop.jsonl -kind shop -rounds 2 -parse gobench:p50-ms \
    -dir ../tifa/test/ssr -env TIFA_SSR_SHOP=1 -env GOMAXPROCS=1 -cpu 2 -sib 6 \
    -- -test.run '^$' -test.bench 'Shop/^render' -test.benchtime 20x
# 3. the verdict, as Markdown
go tool measure bench judge -in shop.jsonl -kind shop -base base -new new           # -detail adds each layout, -delta 0.01 for 1%
```

- `bench build` 复制生成的 `*_amd64.s` 文件，用 `internal/goasm/cmd/xform` 移动代码，并用 `go build -overlay` 构建，所以源码树不会被动。一种布局写成 `shift.front`，比如 `1664.8`：在正式二进制里排在最前的引擎函数之后放 `shift` 个字节的 NOP（它先构建正式的二进制来找到这个函数），再在解释器函数体开头放 `front` 个字节的 NOP。写成 `shift.front.shift2`，比如 `64.8.2208`，则还在正则匹配器 `lre_exec_backtrack` 之后放 `shift2` 个字节的 NOP，`shift2` 取 32 的倍数。`-positions protocol`（默认）是 [measuring.md](measuring.zh-CN.md#规程) 里的那一组。`full` 是在 shift 为 0 和 32 时 front 取 0 到 56，`all` 和 `quick` 是更早的、各有 8 个和 4 个的那些组。它把每种布局和这一边的正式二进制核对（在没有要求 0.0 时，正式二进制构建在 `.work` 里）：匹配器移动了 64 的倍数，且在 shift 的 32 字节之内，shift 所放的函数之后、匹配器之前的每个引擎函数恰好移动了 shift，匹配器到解释器之间的每个引擎函数恰好移动了匹配器移动的量加 `shift2`，解释器跟着移动（有 `shift2` 时移动在这个和的 32 字节之内的 64 的倍数），它的函数体和 10 个热点处理函数在它里面晚 front 个字节的位置，否则构建失败。它写出 `layouts.tsv`，每个二进制一行，带它的 SHA-256 以及函数体和处理函数的地址，重新构建的二进制会替换它那一行（`bench addrs BIN...` 打印同样的行）。解释器在生成的汇编里是 `JS_CallInternal_block`，因为它的参数是成块传入的，`bench build` 自己会找到这个名字。带 `-app` 时，它通过自己写的一个 `go.work` 在那个模块（tifa）里构建，`go.work` 的 `go` 行取两个模块的 `go` 行中较高的那个。`-xform BIN` 使用一个已构建好的 xform，而不是构建 `-tree` 里的那个。只有 amd64 有布局。
- `bench run` 把一种布局的每一边作为一个块运行，一个接一个，每一轮里块的顺序随机，正式布局运行 `-formal-reps` 次（3），双数轮里各边的顺序随机，下一轮里按相反的顺序。它先写 `OUT.manifest.json`：一个随机的实验 id、计划、每个二进制的 SHA-256 和这次运行的设置，其中目录写成绝对路径，进程的整个环境也写进去（shell 的 HOME、PATH 和 TMPDIR，然后是 `-env`：shell 里别的都不传）。用 `-sides` 或 `-positions` 要求的、没有二进制的边或位置是错误。顺序由实验 id 和轮数抽出，所以在文件里接着做的运行会重复它自己的顺序，而新文件会得到新的顺序。用别的二进制、别的设置或别的布局接着做会被拒绝，更多的轮数（`-round0`）和更高的 `-tries` 则不会。带 `-cpu` 和 `-sib` 时，它读 `/proc/stat`，当兄弟超线程或计时核上的其他工作超过 `-busy` 百分比（5）、或者偷走的时间超过 `-steal` 百分比（1）出现在它任一进程期间时，把整个块重新运行，最多 `-tries` 次（3）。`-fresh-pages`（linux 上默认开启）在每个进程之前同步一个二进制，丢掉它缓存的页（`fadvise DONTNEED`），并用 `mincore` 检查没有剩下的，剩下了就报错停下，这样一份副本不会把它的物理页从一个进程保留到下一个进程。记录里有 `fresh_pages`（要求过）和 `fresh_verified`（核实过）。`-parse gobench:UNIT` 读每个 Benchmark 行里 UNIT 这个指标，`-parse jsrun` 读 `jsrun` 的 `eval:` 行的中位数。
- 进程的内存是它自己打印的 `vmhwm-kb: N` 那一行（`internal/measure/hwm`，`jsrun` 使用它）。用 `wait4` 读子进程的峰值是错的：子进程是带着父进程的峰值起步的。
- `bench judge` 按清单里的计划读记录：单位是一种布局在第 2k 轮和第 2k+1 轮的两个块，要么整个用，要么都不用。一个失败的进程，一条重复的记录，另一个实验的或计划之外的记录，一个不是有限正数的指标，或者一个在两次会话里运行的块，都会让它停下。没有可用单位的布局让这次运行不完整：它打印数字，但不给任何结论。然后它读各指标的和（由它做决定）以及每个指标单独的情况，如 [measuring.md](measuring.zh-CN.md#读一个结果) 所说。对没有清单的文件，`-rounds`、`-round0`、`-formal-reps` 和 `-positions` 给出计划，二进制不核对。`bench report` 是更早的按中位数和 2% 规则的读法，留着读旧记录。

```sh
go tool measure bench cg -bin bin/base@0.0 -bin bin/new@0.0 -small 0 -big 3 -dumps -sim -out cg \
    -env GODEBUG=asyncpreemptoff=1 -- -test.run '^$' -test.bench AppCG -test.benchtime {N}x   # counts per render inside the window
go tool measure bench cg -bin bin/base@0.0 -bin bin/new@0.0 -small 2 -big 12 -out cg \
    -- -test.run '^$' -test.bench 'Shop/^render' -test.benchtime {N}x    # the older difference of two runs
```

带 `-dumps` 和 `-small 0` 的 `bench cg` 对每个二进制运行一次 valgrind 的 callgrind，只读基准自己标出的那个窗口：热身之后它把计数清零（客户端请求 `CALLGRIND_ZERO_STATS`），把每个页面渲染 `{N}` 次并写出（`CALLGRIND_DUMP_STATS`），工具读的就是那一份转储。在 amd64 上，客户端请求是一段固定的指令（对 `rdi` 做四次 `rolq`，分别移 3、13、61 和 51，然后 `xchgq rbx, rbx`，参数的地址放在 `rax` 里），也就是几行 Go 汇编。tifa 的 `test/ssr` 里还没有这样的基准（上面的 `AppCG` 代指一个：每个页面一个池，热身，然后在两个请求之间把每个页面渲染 `{N}` 次）。不要用 `-toggle` 数一个 Go 函数：当里面的 goroutine 换到另一个栈或线程时，callgrind 丢掉了大约十分之一的计数，每个函数丢的比例相同。`-sim` 加上分支和缓存的模拟。它保留原始的 `.cg` 文件、每个函数一行的表（`.fn.tsv`）和进程的输出，并打印整个程序以及解释器自己的代码每次渲染的数字。callgrind 需要 `GODEBUG=asyncpreemptoff=1`，并以 `--vgdb=no` 运行。不带 `-small 0` 时，它把每个二进制运行两次并取差，这样加载和编译就抵消了。valgrind 没有安装时，`-valgrind-root DIR` 指向一个里面有 `usr/bin/valgrind` 的目录（`apt-get download valgrind` 再 `dpkg -x` 到一个目录里就够了）。

改了工具或换了机器之后，要做一次 A/A 运行（把同一个二进制复制成第二个边的名字），并按 [measuring.md](measuring.zh-CN.md#什么时候不要信一个结果) 所说去读它。

### 在另一台机器上

`scripts/remote.sh`（POSIX sh）在一台能 ssh 到的机器上，在后台，在它自己的检出里，用指定的核并限定内存，运行一个 make 目标。仓库里没有任何关于那台机器的信息：全部来自环境变量。

```sh
export QUICKJS_REMOTE=user@host QUICKJS_REMOTE_DIR='~/work'     # the machine and a folder it may own
export QUICKJS_REMOTE_CPUS=1,5 QUICKJS_REMOTE_MEM=4G           # optional: taskset and a systemd memory cap
make remote RUN="gen"                          # HEAD, as the tree "main" on the machine
make remote REF=my-branch TREE=new RUN="check-fast"
make remote RUN="bench ARGS='run ...'" QUICKJS_REMOTE_CPUS=1      # a timing job on one core, its sibling idle
scripts/remote.sh -g shop.jsonl HEAD -- make bench ARGS='report ...'      # copy a file back to .build/remote/
```

- 先推送分支。机器从 git 远端 `origin` 取提交（机器需要有权访问它，目标运行之前先运行 `. ~/.profile`）。没推送的提交，或者机器取不到的提交，会作为 git bundle 发过去，只带机器缺的部分。一棵树是它自己的克隆，与 `DIR/repo.git` 共用对象，从不是 git worktree。新的提交只移动被跟踪的文件：生成的引擎和 `.build/` 留在原处。
- 目标的日志会打印出来（最后 40 行），并保存在 `.build/remote/` 里。`-w SECONDS`（默认 3600）是等多久，之后脚本以 124 退出，任务继续运行，它的日志和 done 文件在 `DIR/jobs` 里。退出状态是目标的退出状态。
- 核：计时任务用一个核，让它的超线程兄弟空闲。构建和检查可以两个都用。机器是共享的时候，只用属于你的核。
- 这里没有任何东西会杀进程。你启动的任务通过它自己的 scope 来停止。

## CI

GitHub Actions（`.github/workflows/`）在 linux/amd64（`ubuntu-24.04`）、linux/arm64（`ubuntu-24.04-arm`）、darwin/arm64（`macos-15`）、darwin/amd64（`macos-15` 加 `GOARCH=amd64`，其二进制由 Rosetta 运行，不带原生 C 引擎，原生 C 引擎是为运行器构建的）和 windows/amd64（`windows-2025`，在 Git Bash 里，不带原生 C 引擎，因为那需要 clang，也不带格式和文档检查，这两项在哪都一样）上检查每次 push 和每个来自 fork 的 pull request，这些就是 C 引擎的目标平台。它检查的是对不对，不计时：共享的机器太吵，做不了那件事，所以计时留在构建机上。

`ci.yml` 有这些步骤，每一步单独成项，这样运行页面能显示每步用了多久（在每个平台上，Go 缓存热的时候一共 8 到 12 分钟，大部分是根包和原生 C 引擎）：

| 步骤 | 运行什么 |
| --- | --- |
| Build, Vet | `go build ./...` 和 `go vet ./...` |
| Format and docs | `scripts/check.sh` 的 `gofmt` 和 `docs` 步骤 |
| Test, packages other than the root | 对除根包之外的每个包运行 `go test -count=1 -short -p 2` |
| Test, internal packages with the test hooks | 对 `internal/cengine`、`internal/gohost`、`internal/hostmem` 和 `internal/idtable` 运行 `go test -tags testhooks` |
| Test, root package | `root` 步骤（`go test -p 1 .`） |
| Native C engine | `host-plain` 和 `compare-plain` 步骤：`engine/test/build.sh plain` 和 `compare.sh plain`，用运行器上有的 C 编译器构建（每个上面都是 clang，所以不用安装任何东西） |

`llvm-steps.yml` 是第二个工作流，只有一个任务，只在 linux/amd64 上，它从 apt.llvm.org 安装 LLVM 22.1.8（15 到 25 秒，这是 `scripts/gen-engine.sh` 坚持要求的版本）并运行这些步骤。它会拦下一个让提交进仓库的生成引擎与生成器从 `engine/` 生成的结果不一样的提交：

| 步骤 | 运行什么 |
| --- | --- |
| Generated engine | `make check-gen`（`GEN_JOBS=4`）：把 amd64 和 arm64 的 `.s` 和 `engine_*` 文件，和一次全新的 `make gen` 对比 |
| Generator and cycles | `CHECK_ONLY="goasm cycles" scripts/check.sh fast`：`internal/goasm`（生成器）的测试，以及检查 C 引擎的每一处递归都有计数 |

`scripts/check.sh` 的步骤用 `CHECK_ONLY` 选择，它们的日志作为 artifact 保留 7 天。其中的跳过是预期的：`TestStackResidency` 要 `HOSTMEM_RESIDENCY=1`，重新映射的测试和 `TestStackWithoutHugePages` 只在 linux 上。

`go-latest.yml` 每周一次，在最新的稳定版 Go 上，在五个平台上运行 C 引擎的测试（`internal/cengine`、`internal/gohost`、`internal/hostmem`、`internal/idtable` 和根包）。它带 `-tags quickjs_anygo` 构建它们，越过更新的 Go 会停下的那个错误。它的第二个任务在 linux/amd64、linux/arm64、darwin/arm64 和 windows/amd64 上运行 `scripts/go-versions.sh`，用从 `go.mod` 的 `go` 行到 `internal/goversion` 停下的那个次版本之前的每个 Go 补丁版本，由 go 命令取来（`GOTOOLCHAIN`）。补丁版本也可能改变汇编所依赖的东西，所以一个都不漏，新的补丁版本会在下一次每周运行里测到。每个版本都跑 `go build`、`go vet` 和上面那些 Go 测试。在 linux/amd64 上它还装 LLVM 22.1.8，跑生成器的测试（`scripts/check.sh` 的 `goasm` 一步）和对 amd64、arm64 整个引擎的链接后检查（`-verify-bin` 加 `-verify-all`，对象是这个版本的 Go 带 `-tags goasmkeep` 编出的 `internal/cengine` 测试程序，amd64 有 linux、darwin 和 windows 三个，arm64 有 linux 和 darwin 两个，经由 `scripts/gen-engine.sh` 的 `GEN_VERIFY_BIN`）。这两项检查把引擎写进 `.build`，不动 `internal/cengine`。发布之前运行这个脚本。在装了 LLVM 的机器上它做全部这些，没有 LLVM 时（`GO_VERSIONS_LLVM=0`）不做生成器的检查，`GO_VERSIONS_LIST=1` 会打印它选了哪些版本。取不到或读不懂版本列表时（go list 失败，或列表里没有 `go` 行的那个版本），脚本以状态 2 停下，什么也不检查：它不会退到只检查最低版本。`scripts/go-versions-test.sh` 用一个代替 `go` 的程序测试这一点。提交进仓库的汇编依赖 Go 内部的一些细节，新版本一改动其中一个，这个检查就能早早发现。它不阻止合并。运行变红是去看新的 Go 改了什么的理由。

有意不做的：

- **test262、竞态检测器和 sanitizer 构建。** 它们耗时太长。用 `make check-full` 运行它们。
- **计时和内存测量**、`make gointl`（它会克隆 go-intl）和 `make build_all_targets`。

这个任务在 `ubuntu-24.04` 上约需 4 分钟：安装 15 秒，`make check-gen` 2.3 分钟，另外两步合计 1.1 分钟。`clang --version` 必须打印 22.1.8：如果 apt.llvm.org 换成了更新的 22.1.x，就把包的版本钉住。

## 速度

`go test -run '^$' -bench Call .` 给出每种调用的代价，`make benchmark` 从 `compare/` 在这个引擎、goja 和 fastschema/qjs 上运行 QuickJS 的微基准。选引擎的用户会去读的对比是 `compare/engines`，它是紧挨着的一个独立模块：它在同样的工作负载上运行这个引擎、modernc.org/quickjs、go-quickjs、goant 和 moejs，每个在自己的二进制里，钉在一个 CPU 上，并把 C 版 QuickJS、QuickJS-NG 和 Node 作为参考行放在 tifa 的页面上（[benchmarks.md](benchmarks.md)）。渲染速度在 tifa 里测量，用它示例应用的页面：见 [performance.md](performance.zh-CN.md#在-tifa-里测量)。
