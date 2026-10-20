# 更新记录

记录 `github.com/liustack/quickjs` 里用它的程序看得到的变化。每个版本都是补丁版本，tag 打在 `main` 上。格式参照 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)。每项改动的细节见 [Fork changes](docs/fork-changes/README.md)，每项提速怎么测的见 [Performance](docs/performance.md)。没有特别说明时，时间指在 linux/amd64（AMD EPYC）上服务端渲染 tifa 示例商店一个页面的耗时。

## [Unreleased]

### 变更

- README 改成一页简短的落地页，有英文和中文两版。原来的内容挪进了 `docs/`：按领域分的 fork 改动、测试、合入上游、性能、发版，以及上游的 README。

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

[Unreleased]: https://github.com/liustack/quickjs/compare/v0.26.16...main
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
