# 更新记录

记录 `github.com/liustack/quickjs` 里用它的程序看得到的变化。每个版本的 tag 打在 `main` 上：一般是补丁版本，删掉或改动公开 API 时升次版本号。格式参照 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)。每项改动的细节见 [Fork changes](docs/fork-changes/README.md)，每项提速怎么测的见 [Performance](docs/performance.md)。没有特别说明时，时间指在 linux/amd64（AMD EPYC）上服务端渲染 tifa 示例商店一个页面的耗时。

## [Unreleased]

### 修复

- eval 超时和 `Interrupt` 能在几毫秒内停下反复编译正则的循环，单次很长的编译也能在中途停下，抛出的是同一个不可捕获的中断。以前编译一个正则不管模式多长都只算中断检查的一步，所以 10 ms 超时下，每次用新的长模式 `new RegExp` 的循环要晚 225 ms 到 184 s 才停（看模式而定），一次编译 10 万个分支要跑满 8.5 s。现在都在 10 到 12 ms 停下。
- `JSON.parse`、`JSON.stringify`、`eval`、`new Function`、`repeat`、`padStart`、`replaceAll`、`join` 以及其他解析源码或拼字符串的内置函数，放在循环里也能在超时后几毫秒内停下，以前输入在 60 KB 到 1 MB 时要多跑 5 到 37 s。单次调用仍会跑完。只扫描或排序、不拼字符串的内置函数（`indexOf`、`split`、`fill`、`sort`）放在循环里仍会晚停。渲染耗时和内存不变（不开 PGO，和 v0.27.1 比，商店页面为原来的 0.993，MUI 页面为 0.997）。在一次很长的编译里超过截止时间的脚本，现在编译完就收到中断，以前可能直接跑完。
- 完整 GC 在一次回收中释放不可达的 WeakMap 条目，包括 value 指回 key 的循环和条目链。仍然可达的 key 会保留其 value。Symbol key、WeakSet、WeakRef 和 FinalizationRegistry 按同样的规则处理。tifa 页面上的完整 GC 耗时和以前差不多，WeakMap 很大时更慢：10 万个存活条目连成的链约为原来的 2.9 倍，10 万个存活 key 约为 1.9 倍。

## [v0.27.1] - 2026-10-23

### 修复

- 使用 `lib/internal/patch/quickjs.diff` 的原生 C 构建保留上游的 computed goto 分发，仅为 ccgo 和 Emscripten 禁用。生成的 Go 引擎保持不变，保留不可捕获异常、递归 RangeError 和错误栈规则。原生 C 在相同字节栈限额下的递归次数会随编译器帧大小改变。

### 性能

都是不开 PGO，和 v0.27.0 比。

- 字符串、数字和数组的 `o.x` 和 `o.x(...)` 在解释器里直接到原型上找方法，不再走通用的属性读取：商店耗时为原来的 0.996，tifa 测试应用的 MUI 页面为 0.975（它们的 CSS-in-JS 代码每次渲染读字符串方法最多约 32 万次）。内存占用不变。
- `js_malloc` 直接从释放小块的缓存里取块，不再经过原来的四层调用：耗时为原来的 0.985，tifa 的 MUI 测试应用的页面为 0.983。内存不变。

## [v0.27.0] - 2026-10-22

### 新增

- `Intl.DateTimeFormat` 的四个格式化方法支持 Temporal 值，按类型补默认字段并检查日历。`formatRange` 和 `formatRangeToParts` 也支持 Date 和数字。Temporal 日期的本地化使用同一套格式，通过共用的 go-intl 数据包支持 Chromium 的全部 18 种日历，包括 ISO。
- `Intl.Collator`、`Intl.Segmenter`、`Intl.RelativeTimeFormat`、`Intl.ListFormat`、`Intl.DisplayNames`、`Intl.DurationFormat`、`Intl.Locale` 和 `Intl.supportedValuesOf`，覆盖 Chromium 有的所有语言，在 14,269 个表达式上和 Chromium 156 对过结果。`localeCompare` 按语言的排序规则比较，`toLocaleUpperCase` 和 `toLocaleLowerCase` 遵循土耳其语、阿塞拜疆语、立陶宛语和希腊语的规则。它们和其他构造器共用 go-intl 数据。构建标签 `quickjs_intl_nonames` 和 `quickjs_intl_nocjdict` 可以省去显示名称及中文、日文词典。用了 `intl` 的程序需要附上 `NOTICE.md` 所列声明。当前体积和内存测量见 `docs/fork-changes/time-and-intl.md`。
- `Args.WTF8`：读 Builtin 的字符串参数时，孤立代理项保持引擎里的样子，`Args.String` 则把每个换成 U+FFFD。
- `VM.SetRegExpCache`、`DefaultRegExpCache` 和 `DefaultRegExpCacheBytes`：一个 VM 最多留多少个编译好的正则，占多少字节（默认 256 个、1 MiB）。缓存算进内存上限，贴着上限跑的 VM 可以把缓存调小或关掉。
- `VM.OnClose`：VM 关闭时运行一个函数，自己用表按 VM 存东西的包可以在这时删掉对应的项。
- `temporal` 包：`temporal.Register(vm)` 加上 `Temporal`，V8 有的类型和方法都有，还有 `Date.prototype.toTemporalInstant`。计算由 go-intl 在 Go 里完成，内嵌 ICU 的 tz 2026e 时区数据，从不读主机的 zoneinfo。`Temporal.Now` 用 `intl.TimeZone` 设的时区和 `temporal.Clock` 设的时钟。原始 test262 `built-ins/Temporal` 测量通过 9210 次中的 9206 次。当前 `intl402/Temporal` 通过 4058 次中的 4038 次。原始实现的测量中，进程里第一次用要 13.4 ms，之后每个新 VM 第一次用要 1.1 ms。当前体积测量见 fork-changes 文档。

### 变更

- `StdAddHelpers` 的 `performance.now()` 改为从进程启动开始计时。以前在 Linux 和 macOS 上从系统启动开始，在 Windows 和 FreeBSD 上从 1970 年开始，还会随系统时钟的调整而跳动。在 macOS 上不再计入系统睡眠的时间。`Date.now()` 和 `new Date()` 的值和原来一样。
- 全部 Intl 构造器统一使用 go-intl，包括 NumberFormat、DateTimeFormat 和 PluralRules。Number、Date 的本地化方法共用这些数据，支持舍入增量、舍入优先级及全部日期日历。删除独立的 `intl/cldr.bin` 及其生成器。与 main `641e996` 比，使用应用 PGO profile 构建的商店程序小 0.11 MB。通过预编译格式方案和直接生成字符串，指定用例中预热后的数字格式化比 main 耗时减少 41% 至 42%，日期格式化耗时减少 28% 至 37%。本次提速保持全部已记录答案不变。见[测量结果](docs/fork-changes/time-and-intl.md#focused-timing-and-memory-costs)。
- Intl 的时区名、偏移和夏令时标记统一来自与 Temporal 共用的内嵌 tz 2026e，不再读取主机 zoneinfo，也不需要夏令时修正表。Winnipeg 按较新的 tz 2026e 保持永久夏令时，与 Chromium 156 的旧数据有已记录的差异。
- 完整 Intl402 test262 通过 6764 次中的 6618 次，main `641e996` 为 6276 次。新增的 24 次失败断言在 Chromium 156 中也失败。已有 golden 集没有新增未解释的差异。

### 移除

- `lib` 去掉了 QuickJS 命令行（`quickjs-libc.c`）里的 `std`、`os` 和 `Worker` 模块，Go API 从来不注册它们：`js_init_module_std`、`js_init_module_os`、`js_module_check_attributes`、`js_std_set_worker_new_context_func`、`js_std_init_handlers`、`js_std_free_handlers`、`js_std_promise_rejection_tracker`、`js_std_await`、`js_std_loop`、`js_std_eval_binary` 和 `js_std_eval_binary_json_module` 都没有了。`StdAddHelpers`、`SetDefaultModuleLoader` 和模块的字节码照常可用。程序不再链接这些代码，也不再链接只有它们调用的 libc 部分（`fork`、`execve`、`popen`、`setenv`、文件和终端相关的函数）：只调用 `Eval` 的程序在 linux/amd64 上小 354 KB，tifa 示例商店的服务端按 `tifa build` 的方式构建（`-ldflags=-w`，用商店的 PGO 配置文件，linux/amd64）小 181 KB。

### 修复

- `Temporal.Duration.prototype.toLocaleString` 通过 `Intl.DurationFormat` 输出本地化时长，支持样式、单位选项和小数位，以前返回 ISO 时长字符串。
- 页面替换了 `String.prototype.concat`（QuickJS 用它实现模板字符串）时，`Intl` 不再抛错：v0.26.17 里 `Intl.NumberFormat`、`Intl.DateTimeFormat` 和 `toLocale` 系列方法在这种页面上会抛错。
- `Intl.PluralRules` 给紧凑记数法和科学记数法的数选复数形式时和 ICU 一样，每个 `supportedLocalesOf` 都会读 `localeMatcher` 选项。
- 语言列表里的函数按 ECMA-402 当作对象读，所以 `new Intl.NumberFormat([function () {}])` 抛 RangeError，v0.26.17 抛的是 TypeError。
- 第一次用 `Intl` 时保留页面在那之前做的改动：页面自己的 `Date.prototype.toLocaleString` 或 `Number.prototype.toLocaleString` 留着，用 `Object.defineProperty` 定义或删掉的 `Intl` 也留着，冻结了 `Date.prototype` 或 `globalThis` 时这次使用也不再抛 `TypeError`。
- 模块的顶层 await 还没完成、又没有任务可跑时（比如 `await new Promise(() => {})`），`EvalBytecode` 返回一个说明这种情况的错误，不再让进程崩溃。模块等的东西之后如果完成了（比如由 Go 完成），之后的 `ExecutePendingJobs` 会跑完模块剩下的部分。它等模块的方式和 `ExecutePendingJobs` 跑任务一样：某个任务抛错不再让它停下，模块会跑到底，调用返回那个任务的错误，模块自己被拒绝时返回模块的错误。模块第一个 `await` 之后碰上超时、中断、内存耗尽或 Go 函数 panic，现在立即结束调用，以前会打印到 stderr，然后进程崩溃。
- 在 Windows 上，`Date.now()` 和 `new Date()` 到 2038 年不再溢出。

### 性能

都是不开 PGO，和 v0.26.17 比。

- VM 留下编译好的正则字节码，每次渲染都新建同样正则的代码只编译一次：tifa 示例商店里高亮 CSS 的那篇文章渲染耗时为原来的 0.893，其他页面不变。商店页面的 VM 多占 12 KiB，渲染过那篇 CSS 文章的 VM 留着约 714 KiB 的正则。
- `Date.now()`、`new Date()` 和 `performance.now()` 从 Go 读时间，不再走 modernc.org/libc 的系统调用：分别从 689 ns 降到 122 ns、从 870 ns 降到 261 ns、从 686 ns 降到 107 ns。商店页面的渲染时间不变。
- 引擎里标了 likely 和 unlikely 的判断直接按条件跳转，不再先把条件变成整数：耗时为原来的 0.983，tifa 的 bench 测试应用的页面为 0.976 到 0.987。
- 在 linux 上，引擎取用和释放 TLS 帧不再调用 libc，解释器函数除外：渲染一次 tifa 的 bench 页面少调用约 36 万次。在上一条的基础上耗时为 0.998，bench 页面为 0.990。两条合起来是 v0.26.17 的 0.979，bench 页面为 0.967。

## [v0.26.17] - 2026-10-21

### 新增

- `NewBuiltin` 和 `VM.NewBuiltinFunction`：JavaScript 像调用 `Math.max` 这类内置函数那样调用的 Go 函数，用 `Args` 读参数，不做转换。在 Apple M4 上一次把两个数相加的调用约 55 ns，走 `RegisterHostFunc` 约 470 ns。它返回的 `*Error` 按 `Name` 抛成对应的错误。
- `VM.NewArray`：用 Go 里建的值做一个数组，做法和数组字面量一样，页面在 `Array.prototype` 上放的 setter 不会被触发。
- `VM.SetStrip`，配 `StripSource` 和 `StripDebug`：编译函数时不留源码（`toString` 显示 `[native code]`），或者不留行号和列号。
- `VM.RunGC`、`VM.MemorySize` 和 `VM.GCThreshold`：马上跑一次 GC，按内存上限的算法读 VM 占的内存，读下次 GC 的阈值。程序可以在两次调用 JavaScript 之间回收，再给下一次设好阈值，tifa 就在两次渲染之间这样做。

### 变更

- Go 里建的 `*Error` 的 `Error()` 改为 `Name: Message`，和 JavaScript 的 `Error.prototype.toString` 写法一样，以前是空字符串。所以 `RegisterHostFunc` 函数为这种错误抛出的 `TypeError` 现在也带上这条消息。从 JavaScript 拿到的错误仍是引擎原来的文本。
- `Intl` 的格式化器由进程里所有 VM 共用，最多 1024 个，满了先丢最早的。用过 `Intl` 的 VM 少占约 15 KB。
- `Intl` 的函数和内置函数一个样子：`length` 和 `name` 按 ECMA-402 给，没有 `prototype`，不能用 `new` 调用，`toString` 显示 `[native code]`。
- 对继承了 `Intl.NumberFormat` 或 `Intl.DateTimeFormat` 原型的 `o` 调用 `Intl.NumberFormat.call(o)` 或 `Intl.DateTimeFormat.call(o)`，新的格式化器挂在 `o` 上并返回 `o`，和 V8 一样。
- 这个 fork 新建的文件改用 MIT 许可证（`LICENSE-MIT`），其中的源码文件带 SPDX 标识。`lib/` 里有 14 个文件照着引擎的部分代码写，标识是 `MIT AND BSD-3-Clause`。来自上游的代码仍是 BSD-3-Clause 和 QuickJS 的 MIT 许可证，语言数据仍是 Unicode License V3。`NOTICE.md` 列出哪个路径用哪个许可证，以及分发二进制的程序要附上哪些声明。
- README 改成一页简短的落地页，有英文和中文两版。原来的内容挪进了 `docs/`：按领域分的 fork 改动、测试、合入上游、性能、发版，以及代码从哪来、从 modernc.org/quickjs 换过来会遇到什么。

### 修复

- 页面改了 `Array.prototype`、`Object.prototype`、`JSON` 或其他内置对象，只要是在 `Register` 之后改的，都不再影响 `Intl` 的行为。
- `Intl` 按 ECMA-402 的规则转换选项、值和语言列表：该给字符串或数字的地方给了 Symbol 或 BigInt 会抛 `TypeError`，语言传 `null` 会抛 `TypeError`，选项的读取顺序也和 ECMA-402 一致。
- `Intl.NumberFormat` 格式化包装过的 BigInt，或者 `valueOf` 返回一长串数字的对象时，结果是精确的，以前会经过 double 被舍入。
- test262 的 intl402 测试通过 6764 次中的 912 次，以前是 770 次，以前通过的都没有变成失败。

### 性能

都是不开 PGO，和 v0.26.16 比。

- `===`、`!==`、`==` 和 `!=` 的常见组合（`null`、`undefined`、布尔值、对象、symbol、同为 atom 的字符串）由解释器直接给出结果：耗时为原来的 0.982。
- 已经是 atom 的字符串键一步找到：渲染时间少 1.3%。微基准里用这种键读属性少 13%，写属性少 30%。
- 在 linux 上调用一次 JavaScript 函数只取一个 TLS 帧，原来是两个，另有 34 个函数取 TLS 帧的次数变少：每次渲染的帧少了约五分之一，时间少 1.4%。
- 由多个分支组成的正则不再尝试在当前位置不可能匹配的分支：时间少约 1% 到 2.5%。用 react-spring 那个含 148 个颜色名的正则做一次 `replace`，从 44.5 µs 降到 5.9 µs。
- 新函数直接拿到 `length` 和 `name`，不再经过中间的 shape：耗时为原来的 0.982。
- 浮点数的位运算不走慢路径：tifa 的 MUI 测试应用的页面耗时为原来的 0.952 到 0.960。商店页面用得太少，没有收益。
- `for in` 循环跳过没有可枚举属性的原型：大量使用这种循环的页面最多少约 3%。
- `JSON.stringify` 和 `JSON.parse` 把成段的普通字符一次拷完：把一个路由的数据转成带引号的字符串少 41%，但渲染整体的收益不到 1%。
- `Intl`：在一个 AMD EPYC 7B13 核上，`new Intl.NumberFormat('en')` 从 16.5 µs 降到 2.7 µs，带时区的 `DateTimeFormat` 创建从 32.0 µs 降到 8.1 µs，格式化从 5.2 µs 降到 2.6 µs。

## [v0.26.16] - 2026-10-20

### 新增

- `VM.SetShapeCache` 和 `DefaultShapeCache`：普通对象的 shape 在对象没了以后，一个 VM 留多少个。新 VM 留 2048 个，传 0 关掉。`lib` 里有 `JS_SetShapeCache`，不调用就是关的。

### 变更

- 商店的一个 VM 在 GC 后多占 722 KiB（8.03 MiB，原来 7.33），其中 647 KiB 是留下的 shape。留下的 shape 计入内存上限，分配失败时也不会放掉，所以贴着上限跑的脚本可能在以前不会耗尽内存的地方耗尽：给这样的 VM 小一点的 shape 缓存，或者设成 0。
- 一个一个加上别的对象都没有的键的对象，构建时间比关掉 shape 缓存时多约 6%。

### 性能

- 8 个热点函数（包括属性读取、`==` 和把值转成字符串）只在需要 TLS 帧的语句前后取帧。开 PGO 渲染时间少 1.6%，不开少 1.1%，darwin/arm64（Apple M4，开 PGO）少 2.6%。
- 解释器把 46 处 `set_value` 调用展开写在原地：少 1.4%。加上上一条，比 v0.26.15 开 PGO 少 2.8%，不开少 2.9%。
- 解释器在自己的函数体里给中断计数器倒数，不再每圈循环调用一次函数：开 PGO 少 0.6%。
- 每个 runtime 把释放的 16 到 512 字节的小块留在自己的缓存里，每次 GC 时还回去：开 PGO 少 0.9%。
- VM 留下普通对象的 shape，下一个键相同的对象字面量不用再建 shape：键相同的对象字面量构建时间少 30%，不开 PGO 渲染少 3.6%。和上面两条合起来，比三项都没有时少 4.5%。同一个核的两个超线程上跑两个 VM，每秒多渲染 2.9% 的页面。

## [v0.26.15] - 2026-10-19

### 变更

- 引擎自身 panic，或者 Go 函数调用了 `runtime.Goexit`（`t.Fatal` 就会调），之后这个 VM 只能 `Close`：再调用 JavaScript 都会 panic。交给 JavaScript 的 Go 函数 panic 时仍和以前一样被接住。
- 在 linux 以外，每个 VM 留着最多 1 MiB 的 TLS 栈，直到关闭。在 windows 上这一块整个计入提交上限（commit limit）。
- 用旧版本采集的 PGO profile 能匹配上的调用点少了很多：要重新采集。

### 性能

- 引擎函数在每个 return 前释放 TLS 帧，不再用 `defer`，Go 就能内联它们：渲染时间少约 10%，开不开 PGO 都一样。
- 以单个字符或一类字符开头的正则（比如 `/["&<>]/`）从那个字符第一次出现的地方开始匹配：少约 3%。
- `===` 和键名只在少见的路径上取 TLS 帧：开 PGO 少 0.7%，不开少 2%。
- 在 linux 以外，多个 goroutine 上的 VM 不再为 TLS 栈等 libc 分配器的锁：Apple M4 上 10 个 VM 每秒渲染的页面是原来的 1.4 到 1.55 倍。
- 合起来，换上新 PGO profile 后渲染时间是 v0.26.14 的 0.859，不开 PGO 是 0.843。

## [v0.26.14] - 2026-10-19

### 修复

- `NewVM` 建不出 context 时，不再泄漏它已经分配的内存。

### 变更

- 上游改动可能让 `internal/fixlib` 改错生成代码的地方，它都会停下。使用这个模块的程序看不到变化。

## [v0.26.13] - 2026-10-18

### 性能

- 解释器循环少做栈上的工作：linux/amd64 上渲染时间少约 12%，darwin/arm64（Apple M4）上少 17%。它的 Go 栈帧在 amd64 上从 2608 字节降到 896 字节。
- 给普通对象加新属性（对象字面量、展开、类字段和 `JSON.parse` 都会这样做）跳过通用路径：少约 6%。
- 合起来，渲染时间是 v0.26.12 的 0.823。

## [v0.26.12] - 2026-10-18

### 变更

- 在 linux 上，经过绑定函数的递归每层最多多占约 1 KB 的 TLS 块，32 位平台上一个块最多占 1 KB，原来是 256 字节。递归到默认上限的最坏情况在 linux/amd64 上仍是约 20.7 MB。

### 性能

- JavaScript 调用不再分配 Go 内存，也不再拿 libc 分配器的锁：linux/amd64 上渲染时间少约 11%，darwin/arm64 上少 14%，linux 上把字节码载入 VM 少约 11%。多个核上的多个 VM 不再因为这把锁互相拖慢。

## [v0.26.11] - 2026-10-18

### 修复

- 全局对象上已经有同名属性时，`RegisterFunc` 和 `RegisterHostFunc` 不再让进程 abort。函数会替换可配置的属性，属性不可配置、全局对象不可扩展或名字里有 NUL 字节时返回错误。

## [v0.26.10] - 2026-10-18

### 修复

- VM 关闭过程中和 `Close` 之后，在任何 goroutine 上调用 `VM.Interrupt` 都安全，`Close` 之后什么也不做。
- 不是合法 UTF-8 的 Go 字符串变成 JS 字符串时，每个不能开始合法序列的字节换成一个 U+FFFD，和 Go 自己解码一样。引擎以前会把非法字节后面的字节一起吞掉，ASCII 也不例外。

## [v0.26.9] - 2026-10-18

### 修复

- `intl` 在所有系统上都按 ICU 的方式命名夏令时。在 Linux 上，Europe/Dublin 等几个时区的夏季和冬季名称以前是反的。

## [v0.26.8] - 2026-10-18

### 修复

- 模块在 linux 和 freebsd 上又能构建了，v0.26.6 和 v0.26.7 在这两个平台上编译不过。
- 从 JS 字符串得到的 Go 字符串是合法的 UTF-8：孤立的代理项（比如在 emoji 中间截断字符串留下的）变成 U+FFFD，和 Node 的写法一样。

## [v0.26.7] - 2026-10-17

在 linux 和 freebsd 上编译不过。请用 v0.26.8 或更新的版本。

### 新增

- `*Error` 有了 `Uncatchable`、`OutOfMemory` 和 `Interrupted`，说明这次调用是不是因为脚本捕获不了的异常结束的，以及原因。脚本自己抛出的东西，不管名字和消息是什么，都不算捕获不了。

### 修复

- 执行时限改用单调时钟，调系统时钟不再影响它。长到时钟表示不了的时限永远不会触发，以前会立刻触发。
- 取捕获不了的错误时不运行任何页面代码，比如页面换掉的 `Error.prototype.toString`。

## [v0.26.6] - 2026-10-17

在 linux 和 freebsd 上编译不过。请用 v0.26.8 或更新的版本。

### 变更

- 中断和执行超时不能再通过 promise、迭代器或 Go 函数被捕获：调用直接返回它，和没有 promise 的脚本一样。`ExecutePendingJobs` 停在因它失败的那个任务上，其余任务留在队列里。
- 内存耗尽也捕获不了了，调用返回 `*Error{Name: "InternalError", Message: "out of memory"}`。页面代码不能再故意占满内存然后捕获失败。
- JavaScript 调用的 Go 函数 panic 时，不再穿过引擎展开栈。脚本像被中断一样结束，调用 JavaScript 的 Go 代码用同一个值再次 panic。
- 无限递归抛出脚本可以捕获的 `RangeError: Maximum call stack size exceeded`，以前会因 Go 栈溢出让进程退出。新 runtime 的栈上限是 10000 个槽位。

### 修复

- 内存所剩无几时编译脚本、函数体、`eval` 代码或正则，会以内存耗尽错误失败。以前引擎可能崩溃、报告脚本里并不存在的 `SyntaxError`，或者返回被截断的字节码。

## [v0.26.5] - 2026-10-17

### 修复

- 因为内存不够转换不成 Go 的字符串，`Eval`、`Call`、`Object` 的 JSON、`Value.MarshalJSON` 和 Go 函数的参数都返回这个错误，而不是 `""`。
- `toString` 会抛异常的 BigInt 报告这个异常，读取被抛出值的名字和消息后不再留下异常。

## [v0.26.4] - 2026-10-16

### 修复

- `Intl.NumberFormat` 按 ECMA-402 自 2021 年起的规则读小数位：货币配 `maximumFractionDigits: 0`，或者小数位和有效数字一起给，都不再抛 `RangeError`。

## [v0.26.3] - 2026-10-16

### 变更

- 不管进程是什么时区，每个 VM 里脚本的本地时间都是 UTC，也不用为此改动进程其余部分的 `TZ`。`getTimezoneOffset()` 是 0，本地的 getter 和 setter 按 UTC 工作，`toString` 打印 `GMT+0000`。要显示别的时区，给 `Intl` 传 `timeZone`。

## [v0.26.2] - 2026-10-16

### 新增

- `intl` 包能按 Chromium 有的所有语言格式化日期、数字和复数，数据来自 CLDR 48.2，并新增 `Intl.PluralRules`。输出在约 60,000 个表达式上和 Chromium 156 对过。用了它的程序大 1.7 MB。

### 变更

- `intl` 默认用 UTC 而不是宿主机的时区，并在脚本第一次用到 `Intl` 或 `toLocale*` 方法时才安装，安装用的字节码每个进程只编译一次。`Register` 耗时 0.09 ms、占 VM 9 KB 内存，原来是 0.85 ms 和 46 KB。

### 修复

- Go 函数在自己的 VM 上再执行 JavaScript 时，不再重设执行时限，也不再丢掉期间发出的 `Interrupt`。
- `intl` 安装因内存耗尽、栈溢出或中断而中途停下时会回滚，下次使用时重新安装。

## [v0.26.1] - 2026-10-15

### 变更

- 被保留下来的错误在读取 `stack` 之前，会让它栈上那些函数的字节码和所在的 realm 一直存活。

### 性能

- 创建错误时不再格式化 `stack`：第一次读取 `stack` 时才生成文本，结果不变。用 React 开发版服务端渲染一个 100 项的组件，在 darwin/arm64 上从 42.1 ms 降到 11.2 ms。

## [v0.26.0] - 2026-10-14

fork 的第一个版本，起点是 modernc.org/quickjs v0.25.0 和 libquickjs v0.13.1。

### 变更

- **不兼容：** 模块改名为 `github.com/liustack/quickjs`，引擎是它的包 `github.com/liustack/quickjs/lib`，不再依赖 `modernc.org/libquickjs`。需要 Go 1.27。

### 修复

- VM 在 `JSON.parse` 里，或在其他把含非 ASCII 字符的字符串转成 UTF-8 的地方耗尽内存时，不再泄漏那个字符串。
- `VM.Call` 和 `VM.CallValue` 报告求值函数表达式时抛出的异常，不再报 `cannot call a non-function`。

[Unreleased]: https://github.com/liustack/quickjs/compare/v0.27.1...main
[v0.27.1]: https://github.com/liustack/quickjs/compare/v0.27.0...v0.27.1
[v0.27.0]: https://github.com/liustack/quickjs/compare/v0.26.17...v0.27.0
[v0.26.17]: https://github.com/liustack/quickjs/compare/v0.26.16...v0.26.17
[v0.26.16]: https://github.com/liustack/quickjs/compare/v0.26.15...v0.26.16
[v0.26.15]: https://github.com/liustack/quickjs/compare/v0.26.14...v0.26.15
[v0.26.14]: https://github.com/liustack/quickjs/compare/v0.26.13...v0.26.14
[v0.26.13]: https://github.com/liustack/quickjs/compare/v0.26.12...v0.26.13
[v0.26.12]: https://github.com/liustack/quickjs/compare/v0.26.11...v0.26.12
[v0.26.11]: https://github.com/liustack/quickjs/compare/v0.26.10...v0.26.11
[v0.26.10]: https://github.com/liustack/quickjs/compare/v0.26.9...v0.26.10
[v0.26.9]: https://github.com/liustack/quickjs/compare/v0.26.8...v0.26.9
[v0.26.8]: https://github.com/liustack/quickjs/compare/v0.26.7...v0.26.8
[v0.26.7]: https://github.com/liustack/quickjs/compare/v0.26.6...v0.26.7
[v0.26.6]: https://github.com/liustack/quickjs/compare/v0.26.5...v0.26.6
[v0.26.5]: https://github.com/liustack/quickjs/compare/v0.26.4...v0.26.5
[v0.26.4]: https://github.com/liustack/quickjs/compare/v0.26.3...v0.26.4
[v0.26.3]: https://github.com/liustack/quickjs/compare/v0.26.2...v0.26.3
[v0.26.2]: https://github.com/liustack/quickjs/compare/v0.26.1...v0.26.2
[v0.26.1]: https://github.com/liustack/quickjs/compare/v0.26.0...v0.26.1
[v0.26.0]: https://github.com/liustack/quickjs/tree/v0.26.0
