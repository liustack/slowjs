---
summary: 'Go API 在 NewVM、Eval、Call 和 Value 之外还有什么：每个函数和类型、它的用途、在 tifa 里带来的收益，以及它是哪个版本加入的'
audience: user
order: 2
read_when:
  - 想找 Eval 和 Call 之外的 Go 函数
  - 给 Go API 加函数或类型（加在这里）
  - 想知道某个函数是哪个版本加入的
  - 不写 Go 就运行引擎（slowjs 命令）
translatedFrom: 65729a3355a5
---

# Eval 和 Call 之外的 Go API

[English](api.md) · 简体中文

API 的文档在 [pkg.go.dev](https://pkg.go.dev/github.com/liustack/slowjs)。每个程序都从 `NewVM`、`VM.Eval`、`VM.Call` 和 `Value` 开始。除此之外，还有下面这些函数。每一项都链接到一篇文档，说明它为什么安全、是怎么测量的。v0.30.0 到 v0.30.7 这几个版本是以 `github.com/liustack/quickjs` 发布的。

## 从 JavaScript 调用 Go

- **`NewBuiltin(name, length, fn)` 和 `VM.NewBuiltinFunction(b)`**（v0.26.17）：JavaScript 像调用 `Math.max` 那样调用的 Go 函数，不用反射。`fn` 从 `Args` 读参数（`Number`、`String`、`Bool`、`Value`、`This`、`VM`，以及 `Latin1` 或 `UTF16`，按引擎保存字符串的样子读它的字符），这些方法什么都不转换，`fn` 返回一个 `Value`。一次把两个数相加的调用，在 Apple M4 上约 55 ns，经过 `RegisterHostFunc` 则约 470 ns。一个 Builtin 在每个进程里只创建一次：多个 goroutine 上的 VM 会同时调用它。它返回的 `*Error` 按 `Name` 指定的构造函数抛出（`Error`、`TypeError`、`RangeError`、`SyntaxError`、`ReferenceError` 或 `InternalError`），其他错误都作为 `TypeError` 抛出。[详情](../internals/changes/go-api.md#builtin-a-go-function-called-as-a-built-in)
- **对长字符串调用 `Args.String`、`Args.WTF8`、`VM.NewString`、`VM.NewStringLatin1` 和 `VM.NewStringUTF16`**（v0.30.3）时，遇到 `Interrupt`、eval 超时或这次调用的不可捕获错误就会停下，和引擎里的循环一样：`String` 和 `WTF8` 返回的 ok 为 false，`NewString` 这几个函数返回那个错误，不返回字符串，这次调用以这个错误结束。`RegisterFunc` 函数的字符串参数和结果，以及元素很多的结果，也是这样。[详情](../internals/changes/go-api.md#long-strings-and-results-stop-converting-at-the-limits-of-the-call)
- **`Args.StringLen(i)`**（v0.30.3）：字符串参数的长度，按 JavaScript 的算法以 UTF-16 码元计，读的时候既不转换也不拼接字符串，所以 Builtin 可以在读任何内容之前，就拒绝对它来说太长的字符串。不管字符串多长，它花的时间都一样，也不分配内存。[详情](../internals/changes/go-api.md#argsstringlen-reads-the-length-of-a-string-in-place)
- **`VM.NewArray(vals...)`**（v0.26.17）：用 Go 里的值创建一个数组，创建方式和数组字面量一样，所以页面放在 `Array.prototype` 上的 setter 永远不会运行。[详情](../internals/changes/go-api.md#newarray)
- **`GetAsyncContext` 和 `SetAsyncContext`**（v0.28.5）：两个 Builtin，读取和设置 promise 任务随身带着的一个对象。`then` 或 `await` 运行时，带的是它注册时设置的那个对象，所以基于它们写的 `AsyncLocalStorage` 在 `await` 之后也能找到自己的 store，和 Node 一样。从不设置这个对象的 VM，运行 promise 和以前一样。[详情](../internals/changes/go-api.md#the-async-context-of-promise-jobs)

## 错误

- **`*Error` 的字段 `Uncatchable`、`OutOfMemory` 和 `Interrupted`**（v0.26.7）：调用为什么结束，任何脚本都伪造不了。[详情](../internals/changes/go-api.md#error-says-why-a-call-ended)
- **停下之后 VM 还能做什么**（v0.31.7）：以 `Uncatchable` 为真结束的调用，会让 VM 处于两种状态之一。tifa 在任何一次渲染失败后都会关掉 VM，所以在 tifa 里这两种状态都无关紧要。
  - eval 超时或 `Interrupt`（`Interrupted`）之后，或者内存耗尽（`OutOfMemory`）之后，只要 `errors.Is(err, ErrKilled)` 为 false，每个方法都能用：读取值、`MemoryUsage`、`RunGC`、继续调用，以及 `Close`，它会释放 VM 持有的一切。但当时正在进行的异步工作（一个 `await`、一个 promise 任务、一个异步生成器）可能永远完成不了，所以本该由它兑现或拒绝的 promise 可能一直停在 pending：继续留着这个 VM 的程序不能等待这样的 promise。
  - VM 被杀掉之后（`errors.Is(err, ErrKilled)`），它只能用来 `Close`。`Interrupt`、`MemorySize`、`MemoryFree` 和 `GCThreshold` 仍然能用，会返回 error 的方法返回一个能匹配 `ErrKilled` 的错误，其余的方法（包括 `MemoryUsage` 和 `RunGC`）会 panic。

  不管调用是因什么结束的，返回了值的调用都不会留下错误。
- **在 windows/arm64 机器上，发生故障时 CPU 的状态可能放不进 goroutine 栈下方的空间，这时调用 `NewVM`**（v0.31.12）：返回一个错误，写明 windows 打开了哪种扩展状态，windows 给出了大小时也写上大小，并且不创建 VM。这种情况是：扩展状态是 SME，或者是向量长于 256 位的 SVE，或者 windows 没有说明。在那里发生故障时，写入可能越过栈进到堆里。[详情](../internals/changes/c-engine.md#room-for-a-fault-on-windows)
- **在 Go 里创建的 `*Error` 的 `Error()`**（v0.26.17）是 `Name: Message`，和 `Error.prototype.toString` 写出来的一样，以前这里是空的。从 JavaScript 拿到的错误保留引擎给的文本。[详情](../internals/changes/go-api.md#error-of-an-error-made-in-go)
- **`VM.TrackRejections(max)` 和 `VM.UnhandledRejections()`**（v0.31.1）：留下没有处理函数就被拒绝的 promise，最多 `max` 个，在脚本和它的任务跑完之后取出来，每个拒绝原因都读成 `*Error`，其中的文本截到 8 KiB，用来报告没有处理函数接住的拒绝。不开启的话，VM 一个也不留。读取进行的时候，如果它运行的 JavaScript 调用这两个函数，两者都会返回一个错误给这段 JavaScript，并且什么也不改。[详情](../internals/changes/go-api.md#trackrejections-and-unhandledrejections)

## 内存和 GC

- **`VM.RunGC()`、`VM.MemorySize()` 和 `VM.GCThreshold()`**（v0.26.17）：马上跑一次 GC，读取 VM 占用的内存和它下一次 GC 的阈值，这样程序可以在两次调用 JavaScript 之间回收内存，并为下一次调用设好阈值。[详情](../internals/changes/go-api.md#rungc-memorysize-and-gcthreshold)
- **`VM.MemoryUsage()` 和 `VM.ReleaseMemory(keep)`**（v0.26.17）：按对象、字符串、函数和数组查看 VM 持有什么，并把它留着的空闲内存还给系统。[详情](../internals/changes/go-api.md#memoryusage-and-releasememory)
- **`VM.MemoryFree()`**（v0.31.4）：`MemorySize` 中没有被在用的块占着的那部分：一次回收清掉垃圾后，在存放留下对象的内存区（arena）里留下的空洞，以及 VM 留着复用的已释放块。`RunGC` 之后，`MemorySize() - MemoryFree()` 就是留下的对象占的内存，只要留下的还是同样的对象，它就不变，而 `MemorySize` 会随空洞上下波动，所以池可以用它判断一个 VM 是否在变大。`MemoryUsage` 里它叫 `FreeSize`，`MemoryUsage` 还统计跑过的函数的寄存器代码（`RegisterCodeCount`、`RegisterCodeSize`）。[vm.go](../../vm.go)
- **`VM.RunYoungGC()`**（v0.27.3）：只回收上次 GC 之后创建的对象，这样服务器每次渲染后跑一次 young GC，偶尔跑一次完整的 GC，每次 GC 花的时间大约只和一次渲染产生的垃圾相当。[详情](../internals/changes/young-collection.md)
- **`VM.OnClose(f)`**（v0.27.0）：VM 关闭时运行 `f`，这样在自己的表里按 VM 保存东西的包，可以在这时把它删掉。[详情](../internals/changes/go-api.md#onclose)
- **`NewSharedBytecode(bytecode)` 和 `VM.EvalSharedBytecode(b)`**（v0.28.1）：载入同一份字节码的 VM，只保存一份其中函数的源码文本和行号表，所以 tifa 示例商店的每个 VM 少占 2.36 MiB，`toString` 和调用栈不变。[详情](../internals/changes/go-api.md#sharedbytecode)
- **`VM.SetShapeCache(n)` 和 `DefaultShapeCache`**（v0.26.16）：VM 为下一个有相同键的对象留着多少个普通对象的 shape（shape 是引擎的一条记录，记着一个对象有哪些键、按什么顺序）。[详情](../internals/changes/go-api.md#setshapecache-and-defaultshapecache)

## 源码和时间

- **`VM.SetStrip(s)`** 和 `type Strip`（v0.26.17）：`StripSource` 编译函数时不带源码，所以它们的 `toString` 显示 `[native code]`，`StripDebug` 去掉行号和列号。[详情](../internals/changes/go-api.md#setstrip)
- **`VM.EvalFile(name, javascript, flags)`**（v0.31.0）：把 `name` 当作脚本文件名的 `EvalValue`，所以它的栈帧和语法错误写的是 `repro.js:1:31`，而其他 eval 的都写 `<eval>`。名字会被整理成一行合法的 UTF-8，最多 1,024 字节，整理后会变的模块名会报错。[详情](../internals/changes/go-api.md#evalfile-gives-a-script-a-file-name)
- **`VM.SetClock(now)` 和 `VM.Now()`**（v0.28.2）：设置和读取 VM 的当前时间，Date、Intl 和 Temporal 都从这里读。传 nil 恢复成真实的时钟。[详情](../internals/changes/time-and-intl.md#one-wall-clock-per-vm)
- **包 `temporal`**（v0.27.0）：`temporal.Register(vm)` 加上 `Temporal`，包括 V8 提供的每个类型和方法，由 go-intl 在 Go 里计算，内置 ICU 的 tz 2026e 时区，所以从不读主机的 zoneinfo。`Temporal.Now` 读 VM 的时钟，`temporal.Clock` 只对 `Temporal.Now` 替换这个时钟。它让二进制变大多少，见链接的文档。[详情](../internals/changes/temporal.md)
- **包 `url`**（v0.30.3）：`url.Register(vm)` 加上 `URL` 和 `URLSearchParams`，完整实现标准，在 Go 里解析。core-js 发现原生的 URL 通过了它的检查，就什么也不装，所以带着 core-js 的打包文件不用改。它接受的字符串最多 65,536 个 UTF-16 码元，所以一次 Go 调用只要几毫秒：超过这个长度，输入就不算 URL，查询字符串里的名字或值会是 `RangeError`。URL 是 VM 里的对象，受 VM 的内存上限约束，eval 超时和 `Interrupt` 会在下一次调用或检查时结束脚本。和 Node 24（ada）不一样的地方，这里按标准来：它检查域名的 Bidi 规则，ASCII 的 `xn--` 标签保持原样，带 `#` 的输入遇到路径不透明（opaque path）的 base 时解析失败，没有 host 的 URL 在 `host` 设为空值时得到空的 host。[详情](../internals/changes/url.md#url-and-urlsearchparams-on-a-go-parser)

## 让 React 渲染更快

下面每一项都在全局对象上定义一个函数，用 C 写在引擎里，替 React 里一个调用很频繁的函数做它的工作。宿主程序调用它们之前，全局对象上什么也没有。

- **`VM.RegisterEscapeText(name)`**（v0.29.2）：像 React DOM 的字符串渲染器那样为 HTML 转义文本（`&`、`<`、`>`、`"` 和 `'` 变成实体，参数像 `"" + text` 那样变成字符串）。tifa 用上它之后，示例商店的页面渲染时间是原来的 0.974，bench 页面是 0.938。[详情](../internals/changes/go-api.md#registerescapetext)
- **`VM.RegisterCopyProps(name)`**（v0.29.3）：替 React 的 `createElement` 和 `cloneElement` 做那个把 config 的键复制到元素 props 里的循环的工作。只有它的结果不可能和那个循环不同时，它才复制，其他任何 config 它都返回 `false`，脚本再把这个 config 交给那个循环。tifa 用上它之后，示例商店的页面渲染时间是原来的 0.988 和 0.989，MUI 页面是 0.980。[详情](../internals/changes/go-api.md#registercopyprops)
- **`VM.RegisterStartTag(name)` 和 `VM.RegisterEndTag(name)`**（v0.29.10）：像 React DOM 服务端渲染器在 `pushStartGenericElement` 里那样，写出元素的开始标签，带上它的属性、样式和文本，并像 `endChunkForTag` 那样给出结束标签。只有 React 的函数不可能写出别的内容时，开始标签函数才写，其他调用它都返回调用方传入的 `decline` 对象，脚本再把这次调用交给 React。在 tifa 示例页面一次渲染的开始和结束标签上，它们执行的指令约为 React 那几个函数的五分之一。第七个参数是模式，它让开始标签函数像 React DOM 的 `pushSelfClosing` 那样写 `img` 和空元素（void element），像 `pushStartInstance` 那样写 `input` 和内联的 `style`（v0.31.2）。[详情](../internals/changes/go-api.md#registerstarttag-and-registerendtag)
- **`VM.NewMurmur2Function()`**（v0.31.2）：返回一个用 C 写在引擎里的函数，它像 @emotion/hash 的 `murmur2` 那样对字符串求哈希，其他输入都返回 `undefined`。包 `tifa` 用它定义 `__tifa_murmur2__`，前面先检查页面没有替换 `String.prototype.charCodeAt` 或 `Number.prototype.toString`。[详情](../../murmur2.go)
- **包 `tifa`**（v0.31.1）：`tifa.Register(vm)` 把 VM 设置成 tifa 设置它池里每个 VM 的样子：`URL`、`Intl`、`Temporal`，以及它的打包文件要用的、名字是 `__tifa_*` 的那些函数，上面这几个也在其中。[详情](../internals/changes/go-api.md#package-tifa)

## 不写 Go

- **`slowjs` 命令**（v0.31.0）：在这个引擎里运行一个 JavaScript 文件，设置和 tifa 一样（Intl、Temporal、URL），带有 `print`、`console.log`，以及一定生效的超时和内存上限：用来展示一个脚本遇到的问题，或者不写 Go 就跑一个脚本。每个 GitHub release 都带有预先构建好的版本，覆盖 linux 和 darwin 的 amd64 与 arm64，以及 windows/amd64，从 v0.31.12 起也有 windows/arm64。[详情](command.zh-CN.md)
