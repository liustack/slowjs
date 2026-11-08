---
summary: '模块是怎么搭起来的：从 Go API 到 C 引擎的各层，Go API 的引擎层，每个目录放什么，核心类型和它们的规则，值的表示，javascript 怎么调用 Go 函数，eval 超时和 Interrupt 怎么工作，以及引擎在哪些平台上运行'
read_when:
  - 第一次在这个仓库里干活
  - 改根包的 Go API，从它调用引擎，或者加一个运行 javascript 的方法
  - 找引擎的某一块、对它的某项改动或某个工具放在哪
  - 动值的编码方式，或者手工读一个 JSValue
  - 改 intl、temporal、fmt 或 compare 包，或者 internal/gointl、internal/intlhook
---

# 架构

[English](architecture.md) · 简体中文

`github.com/liustack/quickjs` 是纯 Go、不用 cgo、可嵌入的 JavaScript 引擎（ES2023），v0.28.0 到 v0.29.11 叫 `github.com/liustack/slowjs`。引擎是 `engine/` 里的 C 版 QuickJS，由 LLVM 编译成 Go 汇编并提交进仓库，所以模块的使用者不需要 C 编译器，也不需要 LLVM。它是 2026-06-04 发布的 QuickJS（提交 `3d5e064`）加上 [changes](changes/README.zh-CN.md) 列出的改动。v0.28 之前，引擎是 ccgo 转译成 Go 的 QuickJS，后来被 C 引擎取代。代码从哪来，见 [upstream.md](upstream.zh-CN.md)。

没有 cgo，也没有共享库：模块当作普通 Go 代码交叉编译，目标是引擎能运行的那些平台。`grep -rn 'import "C"'` 什么也找不到。不要引入 cgo。

## 分层

1. **`engine/`**：带有 liustack/quickjs 改动的 QuickJS 的 C 代码，用 `-ffreestanding -nostdinc` 对着 `engine/libc`（它自己的小 libc）编译。它只向宿主要 `engine/host.h` 里的回调：内存、时钟、中断轮询和几个请求（`host_call`）。它没有全局变量，没有锁，也没有系统调用。`engine/test/` 是运行它的原生宿主，用于不需要 Go 的测试。
2. **`internal/cengine/`**：作为 Go 汇编的引擎，由 `internal/goasm/gen` 从 `engine/` 生成并提交进仓库（`make gen`、`make check-gen`）。旁边的 Go 文件是回调的宿主，以及一个带有根包所需调用的 VM。
3. **Go 宿主**：`internal/gohost` 回答 VM 的回调，`internal/hostmem` 把它的内存映射到 Go 堆之外（内存块，带保护页的影子栈），`internal/idtable` 保存引擎交还给 Go 的 id。
4. **Go API**（`quickjs.go` 和旁边的文件）：用户 import 的部分。

## 引擎层

Go API 只通过下面这些文件调用引擎。包里其余部分不出现 `cengine.*`。

| 文件 | 内容 |
| --- | --- |
| `engine.go` | 值类型 `jsValue` 和它的辅助函数（`tag`、`newInt32`、`valueAt`、`dupValue`、`stringChars` 等），包用到的 tag、atom（引擎给属性名和常用字符串的编号） 和标志，VM 的运行时和上下文（`engineState`，由 `vmState` 内嵌），以及包调用的每个 C 函数各一个方法，按 C 函数命名（`jsEval`、`jsFreeValue`、`jsGetProperty` 等）。调用引擎时要传影子指针，也就是 VM 影子栈的第一个空闲字节：`m.sp`，javascript 调用的 Go 函数会把它移到这次调用的帧之上 |
| `entry.go` | 进入引擎和离开引擎（`enter`、`leave`），eval 超时和 `Interrupt`（由宿主的轮询执行），以及 panic 之后留下什么 |
| `hostcall.go` | 引擎调用的 Go 函数（`callGo`、`callBuiltin`、`Then` 回调的类），以及调用它们的函数对象 |
| `enginememory.go` | `MemoryUsage`、`ReleaseMemory`，以及 `SharedBytecode` 的内存（映射在所有 VM 之外，只读） |
| `engine_unsupported.go` | 在其他任何平台上，import `internal/unsupported`，构建只报它的错误，错误里写明支持的平台 |

共用的文件放 API：`vm.go`（创建、设置和关闭 VM）、`eval.go`、`call.go`、`convert.go`（Go 值和 javascript 之间互转）、`errors.go`、`register.go`（`RegisterFunc`）、`result.go`（Go 函数结果的快照，连同结果交出的 `Value` 引用）、`value.go`、`builtin.go`、`jsstring.go`、`promise.go`、`sharedbytecode.go`、`memory.go`、`asynccontext.go`、`vmclock.go` 和 `etc.go`。`TestEngineDeps`（`enginedeps_test.go`）检查这个包链接了 `internal/cengine` 和它的 Go 宿主。

## 目录

| 路径 | 内容 |
| --- | --- |
| 根目录的 `quickjs.go` 和其他 `.go` 文件 | Go API，建在引擎层上（见[引擎层](#引擎层)） |
| `engine/` | 引擎的 C 树：钉住提交的 QuickJS 加上 liustack/quickjs 对 C 的改动。没有 Go 文件，Go 包目录里也不能放 C 文件（`layout_test.go`） |
| `engine/libc/` | C 树的小 libc：musl 的内存、字符串和数学函数，一个 `snprintf` 和头文件，让引擎能用 `-ffreestanding -nostdinc` 构建（见它的 [README](../engine/libc/README.zh-CN.md)） |
| `engine/host.h` | 引擎向宿主要的回调。原生测试宿主和 Go 宿主都实现它们 |
| `engine/test/` | 原生测试宿主：用普通 clang 和它的小 libc 构建的 C 树，test262、记录下来的输出和 C 写的检查都在它上面跑，也在 AddressSanitizer 和 UndefinedBehaviorSanitizer 下跑（见 [testing.md](testing.zh-CN.md#c-树和它的原生测试宿主)）。从不属于 Go 构建 |
| `internal/cengine/` | 生成的引擎（amd64 和 arm64 的 `.s`、`engine_decl_<arch>.go`、`engine_keep_<arch>.go`，提交进仓库），以及旁边的宿主文件：汇编调用的回调，一个带 Eval、RunGC、ReleaseMemory 和 Close 的 VM，测量它的 `cmd/jsrun`、`cmd/gcpause`、`cmd/interrupt`，以及绊线文件对 `goasm_tripwire.<arch>.s` 和 `.go`：一个从不链接的函数，只有构建原样保留指令时才能汇编通过 |
| `internal/goasm/gen/` | 用 LLVM 22.1.8 把 `engine/` 编译成 Go 汇编的生成器，它的 pass 和检查（见它的 [README](../internal/goasm/gen/README.zh-CN.md)）。`internal/goasm/cmd/xform` 移动代码，好在几个位置上给一个构建计时 |
| `internal/hostmem/` | Go 宿主给引擎的内存，映射在 Go 堆之外：一个 VM 堆的内存块和它们的总量，以及带保护页的影子栈 |
| `internal/gohost/` | 引擎回调的 Go 一侧：按 id 和按影子栈找 VM，它们在 `internal/hostmem` 上的内存，时钟，随机种子，中断，以及引擎自己的 `host_call` 请求。不需要引擎也能跑 |
| `internal/idtable/` | 引擎交还给 Go 的 id 表 |
| `internal/unsupported/` | 为引擎不支持的平台构建时的错误 |
| `internal/goversion/` | 用比引擎核对过的版本更新的 Go 构建时的错误，由 `internal/cengine` import |
| `internal/faulttest/` | 给故意制造内存错误的测试用：`Room` 由测试在 Go 代码里的错误之前紧接着调用，让 windows 有空间写下 CPU 的状态。`Unsafe` 用于 `Room` 没法紧挨在前面的 Go 代码里的错误（见 [c-engine.md](changes/c-engine.zh-CN.md#windows-上给故障留的空间)） |
| `internal/measure/` | `go tool measure bench`：在几个代码位置上构建一个程序，轮流计时，并数指令数 |
| `intl/` | 包 `github.com/liustack/quickjs/intl`，给 VM 用的 `Intl` |
| `internal/gointl/internal/qjspack/gen/` | 共用的 Intl 数据包，按 Chromium 的 ICU 清单挑选 |
| `intl/testdata/chromium/` | Chromium 的答案，给 golden 测试用，以及记录它们的 `record.cjs` |
| `temporal/` | 包 `github.com/liustack/quickjs/temporal`，给 VM 用的 `Temporal` |
| `temporal/testdata/` | 和 Chromium 对照的表达式及它的答案，以及链接测试构建的程序 |
| `internal/gointl/` | go-intl（纯 Go 的 ECMA-402 和 Temporal）的副本，在每个 `intl` 构造函数和 `temporal` 后面（见 [upgrading.md](upgrading.zh-CN.md#更新-go-intlinternalgointl)） |
| `internal/intlhook/` | 包 `temporal` 从包 `intl` 拿的东西：它的 `toLocaleString` 的格式化，以及 VM 的默认时区 |
| `fmt/` | 包 `github.com/liustack/quickjs/fmt` |
| `compare/` | 一个独立的模块，从上游保留下来，在这个引擎、goja 和 fastschema/qjs 上跑 QuickJS 的微基准 |
| `internal/arewefastyet/` | compare/engines 运行的 V8 基准套件（v8-v7） |
| `scripts/docs-list.sh` | `docs/` 的索引 |

## 核心类型

- `VM`，主要的句柄：一个 JS realm 和它在引擎里的运行时，带着 Go 宿主给它的内存。由 `NewVM()` 创建，`Close()` 释放。多个 goroutine 并发使用它不安全。有错误结果的方法用这个错误报告它能看出的误用，而不是 panic：在对 VM 的调用进行中（比如在 javascript 调用的 Go 函数里）调用 `Close` 和 `ReleaseMemory`，会返回错误，什么也不改。
- `Value`：包装一个引用计数的 `jsValue`。`Dup()` 和 `Free()` 必须严格成对，并且正确嵌套（类型的文档说明了跨 goroutine 和 channel 时怎么会出错）。用错会导致泄漏或释放后使用，原生测试宿主的 sanitizer 和 `internal/cengine` 的检查能抓到。
- `Object`、`Atom` 和 `HostFunc`（`func(args []any) (any, error)`）。
- `Builtin`，javascript 当作内置函数调用的 Go 函数，以及 `Args`，它被调用时的参数（见[从 JavaScript 调用的 Go 函数](#从-javascript-调用的-go-函数)）。

## 值

`JSValue` 是两个字的结构体，一个联合体加一个 tag（`engine.go` 里的 `jsValue`），引擎用两个寄存器返回它。所有支持的平台都是 64 位小端。`engine.go` 定义了读取和构造它的辅助函数：`newInt32`、`newFloat`、`newBool`、`tag`、`isException`、`jsvToInt32`、`jsvToFloat64` 等。int32 或 bool 的载荷放在联合体的低 32 位，float64 或指针的载荷占满整个联合体。带引用计数的值，计数在它所持地址之前 4 个字节处（`cengine.RefCountOffset`），字符串的长度、宽度和字符，则按引擎导出的布局表（`js_host_layout`，见 [c-engine.md](changes/c-engine.zh-CN.md)）就地读取。

## 从 JavaScript 调用的 Go 函数

`RegisterFunc`、`RegisterHostFunc` 和 `Then` 的 Go 函数由一个 id（`keepFunc`）保存，调用它们的函数对象持有这个 id。这个 id 是 VM 的宿主的一个句柄（`gohost.VM.Keep`），引擎释放函数对象时把它交还，`Close` 清掉剩下的。引擎通过 `host_call` 调用 Go，`internal/cengine` 把它转给 `hostCall`。

`Builtin`（`builtin.go`）是快路径：每个进程用 `NewBuiltin` 创建一次，保存在写时复制的 `builtins.list` 里，以它的 magic 为下标，每个 VM 用 `VM.NewBuiltinFunction` 把它变成函数对象。`callBuiltin` 从上下文的 opaque 找到它的 VM，也就是 `vms` 里这个 VM 的 id，这个表的读取不加锁。它保留了 `callGo` 对 panic 和无法捕获错误的处理：改一个，另一个也要改。`VM.NewArray` 用 `JS_DefinePropertyValueUint32` 构建数组，所以 `Array.prototype` 上的 setter 不会运行：Builtin 用它返回列表。

## eval 超时和 Interrupt

每个运行 javascript 的方法（`Eval`、`EvalValue`、`Call`、`CallValue`、`ExecutePendingJobs` 等）在运行 javascript 之前都先调用 `m.enter()` 和 `defer m.leave(&err)`。`enter` 记录调用嵌套了多深，只有最外层的调用才会按 `m.timeout` 设定截止时间，并在宿主里清掉中断标志。所以，在同一个 VM 上再执行更多 javascript 的 Go 函数，仍在外层调用的时限之内，也保留着待处理的 `Interrupt()`。引擎在生成的代码做轮询的地方，以及 C 里读这个标志的地方读它（[c-engine.md](changes/c-engine.zh-CN.md#轮询与中断)）。截止时间用单调时钟（见 [uncatchable.md](changes/uncatchable.zh-CN.md#eval-超时用单调时钟)）。

`SetEvalTimeout` 设置时长。`Interrupt()` 以原子方式设置标志，可以从任何 goroutine 调用。它通过 `VM.intr` 到达标志，`VM.intr` 把 VM 的宿主放在 Go 内存里，并用 `Close` 排好顺序。`Close` 只清内嵌的 `vmState`，从不清 `intr`，所以 `Interrupt` 在 `Close` 期间和之后从任何 goroutine 调用都是安全的。新的 VM 字段放进 `vmState`。加一个运行 javascript 的方法时，要加上 `enter` 和 `leave` 这一对，否则时限对它不起作用。所有可能碰到 getter、setter、proxy 陷阱、`toJSON`、`toString` 或被替换的内置方法的方法都算，不只是执行源码的那些：`GetProperty`、`SetProperty`、`Value.Any`、`Value.MarshalJSON` 和 `ErrorFromValue` 也有这一对，而 `outer`（没有时限，不能终止）留给不可能运行脚本的调用。由异常生成的 `*Error` 用 `ErrorFromValue` 读它的字段，所以在调用之外读出的错误也受时限约束。

## 引擎旁边的包

- **`intl/`**：所有 `Intl` 构造函数、`toLocale*` 和 `localeCompare`，给调用了 `Register(vm)` 的 VM 用。QuickJS 没有 `Intl`，所以没调用 `Register` 就去用它的页面会得到 `ReferenceError`。javascript 一侧是 `intl.js` 和 `stub.js`，决定在 Go 里做，建在 `internal/gointl` 上，`bridge.go` 顶部的注释说明怎么加一个构造函数。改过格式化器之后，`intl/golden_test.go` 是最要紧的检查：数据知道规范不知道的细节。它怎么工作，输出怎么和 Chromium 对照：见 [time-and-intl.md](changes/time-and-intl.zh-CN.md)。
- **`temporal/`**：`Temporal`，给调用了 `Register(vm)` 的 VM 用，和 `intl/` 的构造方式一样。go-intl 的 temporal 包做所有计算，通过 `bridge.go` 里的一个 Builtin，它的第一个参数写明操作名（`opNames` 列表）。见 [temporal.md](changes/temporal.zh-CN.md)。
- **`internal/gointl/`**：go-intl（纯 Go 的 ECMA-402 和 Temporal）的副本，停在 `internal/gointl/UPSTREAM` 里的提交，`intl` 和 `temporal` 使用它。只有 `qjs*` 文件和 `UPSTREAM` 是我们的。go-intl 的文件只能通过 `qjs.patch` 和 `qjs-sync.sh` 改（见[更新 go-intl](upgrading.zh-CN.md#更新-go-intlinternalgointl)）。`make gointl` 运行需要构建标签的检查，并检查 `qjsdata/` 里的数据包是最新的。
- **`fmt/`**：包 `github.com/liustack/quickjs/fmt`，是这个模块的一部分。它用 `Register(obj Value)` 给 VM 提供 Go 的 `fmt.Printf`、`Println`、`Sprintf` 和 `Sprintln`，在 javascript 里用小写名字（`fmt.println`）。
- **`compare/`**：一个独立的模块（`github.com/liustack/quickjs/compare`），在这个引擎、goja 和 `github.com/fastschema/qjs` 上运行 `compare/testdata/` 里的 QuickJS 微基准。`make benchmark` 在那里运行。它独立成模块，是为了让它的依赖不进库的 `go.mod`。`compare/go.mod` 把 `github.com/liustack/quickjs` 替换成 `../`，所以基准构建的是工作树，而 `go mod tidy`（`benchmark` 目标先运行它）从根 `go.mod` 取其他版本：不要手工复制它们。

## 平台

引擎在 linux 和 darwin 的 amd64 与 arm64，以及 windows/amd64 上运行。每个系统运行的是各自架构下同一份生成的代码。其他平台不支持，所以为它们构建会停下来报错，错误里写明支持的平台（`internal/unsupported`）。哪个 Go 能构建，更新的 Go 会怎样，见 [upgrading.md](upgrading.zh-CN.md#换用更新的-go)。改动必须保证 `make build_all_targets` 在全部五个平台上通过（见 [testing.md](testing.zh-CN.md#每个目标平台)）。

## 让引擎更快

为了速度做的改动守这几条规则，liustack/sqlite 也守同样的规则：

1. 不碰 Go 私有的东西：不用 `go:linkname` 接进运行时，不依赖 Go 怎么用某个寄存器（比如闭包寄存器），不依赖运行时的内部实现。这些在 Go 的版本之间会变，而模块要能用 1.26.6 起的每个补丁版本构建。生成的代码对 Go 的汇编器和链接器的依赖，由链接后的检查在 `scripts/go-versions.sh` 跑的每个版本上把关。
2. 引擎跑在 goroutine 的栈上，从不像 cgo、wazero 和 purego 那样用一个自己的栈，这样 C 里的一次故障只结束一个 VM，不结束整个进程。
3. [measuring.md](measuring.zh-CN.md) 的计时分不出它和代码位置噪声（一次渲染 2% 到 3%）的收益，不管改动多干净都不要。
4. 每个平台上的位都一样：引擎用 `-ffp-contract=off` 构建，生成器拒绝融合乘加（[c-engine.md](changes/c-engine.zh-CN.md#小-libc)）。
5. 引擎给出的东西不变：tifa 页面的 HTML，test262 的 33 个已知错误，以及调用深度。
6. C 只做机械的改动，从不重写，每处改动都写进 [docs/changes](changes/README.zh-CN.md)。这一条和 liustack/sqlite 不同：它从不改发布出来的 C，而这里的 C 树是 QuickJS 加上 docs/changes 里的改动。

试过又放弃的，以及原因：见 [performance.md](performance.zh-CN.md#试过但没做的)，v0.28 之前的引擎见 [history.md](history.zh-CN.md#在旧引擎上试过但没做的)。
