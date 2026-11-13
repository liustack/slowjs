---
summary: 'Go API 在 NewVM、Eval、Call 和 Value 之外还有什么：每个函数和类型是做什么的，在 tifa 里带来多少收益，从哪个版本开始有'
read_when:
  - 找 Eval 和 Call 之外的某个 Go API 函数
  - 给 Go API 加函数或类型（中英两份都写在这里）
  - 想知道某个函数是哪个版本加的
---

# Eval 和 Call 之外的 Go API

[English](api.md) · 简体中文

API 文档在 [pkg.go.dev](https://pkg.go.dev/github.com/liustack/quickjs)。每个程序都从 `NewVM`、`VM.Eval`、`VM.Call` 和 `Value` 开始。除此之外还有下面这些。每一条都链到一份文档，讲它为什么安全、怎么测的。v0.28.0 到 v0.29.11 发布时的模块路径是 `github.com/liustack/slowjs`。

## 从 JavaScript 调 Go

- **`NewBuiltin(name, length, fn)` 和 `VM.NewBuiltinFunction(b)`**（v0.26.17）：一个 Go 函数，JavaScript 调它就像调 `Math.max`，不经过反射。`fn` 从 `Args` 读参数（`Number`、`String`、`Bool`、`Value`、`This`、`VM`，以及按引擎内部存法读字符串字符的 `Latin1` 或 `UTF16`），这些都不做转换，返回一个 `Value`。在 Apple M4 上，一次把两个数相加的调用约 55 ns，走 `RegisterHostFunc` 约 470 ns。每个进程只建一次 Builtin：多个 goroutine 上的 VM 会同时调它。它返回的 `*Error` 按 `Name` 指定的构造函数抛出（`Error`、`TypeError`、`RangeError`、`SyntaxError`、`ReferenceError` 或 `InternalError`），其他错误作为 `TypeError` 抛出。[详情](changes/go-api.zh-CN.md#builtin作为内建函数调用的-go-函数)
- **长字符串上的 `Args.String`、`Args.WTF8`、`VM.NewString`、`VM.NewStringLatin1` 和 `VM.NewStringUTF16`**（v0.30.3）碰到 `Interrupt`、eval 超时或调用里无法捕获的错误就停下，和引擎的循环一样：`String` 和 `WTF8` 返回 ok 为 false，`NewString` 这几个函数返回这个错误，不返回字符串，调用以这个错误结束。`RegisterFunc` 函数的字符串参数和结果，以及元素很多的结果，也一样。[详情](changes/go-api.zh-CN.md#长字符串和结果的转换在调用的限制处停下)
- **`Args.StringLen(i)`**（v0.30.3）：字符串参数的长度，按 JavaScript 的算法以 UTF-16 码元计，读它时不转换也不拼接字符串，所以 Builtin 能在读之前就拒绝对它来说太长的字符串。字符串多长都花一样的时间，也不分配内存。[详情](changes/go-api.zh-CN.md#argsstringlen-就地读字符串的长度)
- **`VM.NewArray(vals...)`**（v0.26.17）：在 Go 里建一个数组，建法和数组字面量一样，所以页面放在 `Array.prototype` 上的 setter 永远不会被触发。[详情](changes/go-api.zh-CN.md#newarray)
- **`GetAsyncContext` 和 `SetAsyncContext`**（v0.28.5）：两个 Builtin，读和设 promise 任务携带的一个对象。`then` 或 `await` 运行时用的是注册它那一刻设的那个，所以基于它们写的 `AsyncLocalStorage` 在 `await` 之后也能找到自己的 store，和 Node 一样。从不设它的 VM，promise 的行为和以前一样。[详情](changes/go-api.zh-CN.md#promise-任务的异步上下文)

## 错误

- **`*Error` 的字段 `Uncatchable`、`OutOfMemory` 和 `Interrupted`**（v0.26.7）：一次调用为什么结束，脚本没法伪造。[详情](changes/go-api.zh-CN.md#error-说明调用为什么结束)
- **在 Go 里建的 `*Error` 的 `Error()`**（v0.26.17）是 `Name: Message`，和 `Error.prototype.toString` 的写法一样，以前是空的。从 JavaScript 拿到的错误保留引擎给的文字。[详情](changes/go-api.zh-CN.md#在-go-里造的-error-的-error)

## 内存和 GC

- **`VM.RunGC()`、`VM.MemorySize()` 和 `VM.GCThreshold()`**（v0.26.17）：立刻跑一次 GC，读一个 VM 占的内存和下次 GC 的阈值，这样程序可以在两次调用 JavaScript 之间回收，并设好下一次的阈值。[详情](changes/go-api.zh-CN.md#rungcmemorysize-和-gcthreshold)
- **`VM.MemoryUsage()` 和 `VM.ReleaseMemory(keep)`**（v0.26.17）：按对象、字符串、函数和数组看一个 VM 占了什么，把它留着没用的内存还给系统。[详情](changes/go-api.zh-CN.md#memoryusage-和-releasememory)
- **`VM.RunYoungGC()`**（v0.27.3）：只回收上次 GC 之后新建的对象。服务器每次渲染后跑一次年轻代 GC，隔一阵跑一次完整 GC，每次 GC 的耗时就和一次渲染产生的垃圾差不多。[详情](changes/young-collection.zh-CN.md)
- **`VM.OnClose(f)`**（v0.27.0）：VM 关闭时运行 `f`，这样在自己的表里给每个 VM 存了东西的包可以在那时删掉。[详情](changes/go-api.zh-CN.md#onclose)
- **`NewSharedBytecode(bytecode)` 和 `VM.EvalSharedBytecode(b)`**（v0.28.1）：加载同一份字节码的多个 VM 共用一份函数源码和行号表，tifa 示例商店的每个 VM 因此少占 2.36 MiB，`toString` 和调用栈不变。[详情](changes/go-api.zh-CN.md#sharedbytecode)
- **`VM.SetShapeCache(n)` 和 `DefaultShapeCache`**（v0.26.16）：一个 VM 为下一个同样键的普通对象留多少个 shape（shape 是引擎记录一个对象有哪些键、按什么顺序的结构）。[详情](changes/go-api.zh-CN.md#setshapecache-和-defaultshapecache)

## 源码和时间

- **`VM.SetStrip(s)`** 和 `type Strip`（v0.26.17）：`StripSource` 编译函数时不留源码，函数的 `toString` 显示 `[native code]`，`StripDebug` 不留行号和列号。[详情](changes/go-api.zh-CN.md#setstrip)
- **`VM.SetClock(now)` 和 `VM.Now()`**（v0.28.2）：设置和读取一个 VM 的墙上时钟，Date、Intl 和 Temporal 都读它。传 nil 恢复真实时钟。[详情](changes/time-and-intl.zh-CN.md#每个-vm-一个墙上时钟)
- **`temporal` 包**（v0.27.0）：`temporal.Register(vm)` 加上 `Temporal`，V8 提供的每个类型和方法都有，由 go-intl 在 Go 里计算，内置 ICU 的 tz 2026e 时区数据，从不读主机的 zoneinfo。`Temporal.Now` 读 VM 的时钟，`temporal.Clock` 只给 `Temporal.Now` 换一个时钟。它让二进制变大多少，见链接的文档。[详情](changes/temporal.zh-CN.md)
- **`url` 包**（v0.30.3）：`url.Register(vm)` 加上 `URL` 和 `URLSearchParams`，标准的全部，解析在 Go 里做。core-js 见到过得了它自己检查的原生 URL 就什么也不装，所以带着它的 bundle 不用改。它接受的字符串最多 65,536 个 UTF-16 单位，所以一次 Go 的调用只要几毫秒：超过的输入不是 URL，查询串里超过的名或值是 `RangeError`。一个 URL 是 VM 里的对象，受内存上限管，eval 超时和 `Interrupt` 在下一次调用或下一次轮询时结束脚本。Node 24（ada）和标准不同的地方，这里按标准：检查域名的 Bidi 规则，只含 ASCII 的 `xn--` 标签照写保留，带 `#` 的输入对有 opaque path 的 base 解析会失败，把没有主机的 URL 的 `host` 设成空值时给它一个空主机。[详情](changes/url.zh-CN.md#解析在-go-里的-url-和-urlsearchparams)

## 让 React 渲染更快

下面每一个都在全局对象上定义一个函数，用引擎里的 C 写成，替 React 的某个热点函数干活。主机不调用它们，全局对象上就什么都没有。

- **`VM.RegisterEscapeText(name)`**（v0.29.2）：像 React DOM 的字符串渲染器那样为 HTML 转义文本（`&`、`<`、`>`、`"` 和 `'` 变成实体，参数按 `"" + text` 变成字符串）。tifa 用上它以后，示例商店的页面渲染时间变成原来的 0.974，bench 页面变成 0.938。[详情](changes/go-api.zh-CN.md#registerescapetext)
- **`VM.RegisterCopyProps(name)`**（v0.29.3）：替 React 的 `createElement` 和 `cloneElement` 里把 config 的键复制进元素 props 的那个循环干活。只在它和循环的结果不可能不同的时候复制，其他 config 一律回答 `false`，脚本再交给循环处理。tifa 用上它以后，示例商店的页面渲染时间变成原来的 0.988 和 0.989，MUI 页面变成 0.980。[详情](changes/go-api.zh-CN.md#registercopyprops)
- **`VM.RegisterStartTag(name)` 和 `VM.RegisterEndTag(name)`**（v0.29.10）：像 React DOM 服务端渲染器的 `pushStartGenericElement` 那样写出元素的开始标签，带属性、样式和文本，像 `endChunkForTag` 那样给出结束标签。开始标签函数只在 React 的函数也只可能写出同样内容时才写，其他调用一律返回调用方给的 `decline` 对象，脚本再交给 React 处理。在 tifa 示例页面一次渲染的所有开始和结束标签上，它们跑的指令数约为 React 函数的五分之一。[详情](changes/go-api.zh-CN.md#registerstarttag-和-registerendtag)
