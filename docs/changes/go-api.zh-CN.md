---
summary: 'Go API 的改动：Call 报告什么，Go 无法转换的字符串，Go 与 javascript 之间的孤立代理项和非法 UTF-8，*Error 对无法捕获的结束说了什么，对已有名字 RegisterFunc，NewVM 干净地失败，SetShapeCache，SetRegExpCache，Builtin，Args.WTF8，Builtin 的字符串在 Go 里转换，Args.Latin1 和 Args.UTF16 以及 NewStringLatin1 和 NewStringUTF16，NewArray，SetStrip，在 Go 里造的 Error 的文本，RunGC、MemorySize 和 GCThreshold，OnClose，SharedBytecode，EvalBytecode 如何等待模块的顶层 await，模块如何释放它的顶层，promise 任务为 AsyncLocalStorage 保存的异步上下文，RegisterEscapeText（用 C 写的 React DOM 的 HTML 转义），RegisterCopyProps（用 C 写的 React 复制元素 props 的循环），RegisterStartTag 和 RegisterEndTag（用 C 写的 React DOM 为元素写的开始标签和结束标签），MemoryUsage 和 ReleaseMemory，已经去掉的模块加载器、shell 辅助函数和 SetCanBlock，以及会让构建停下的情形：更新的 Go，和汇编器选项 -spectre=ret 与 -dynlink，每个入口都拒绝别的 VM 的 Value，值的构造函数报告内存不足，转换失败的结果不留下任何东西，UndefinedValue 和零值 Value 被当作 undefined，在对 VM 的调用进行中拒绝 Close，Then 返回 then getter 的异常，Go map 变成只有自有数据属性的对象，作为 any 返回的 Value 按 Value 接管，结果两次到达的同一个位置只算一个引用，拒绝包含自身的结果，长字符串和结果的转换在调用的限制处停下'
read_when:
  - 在 javascript 调用的 Go 函数里关闭 VM
  - 把 Value 交给 VM 的方法，或者从 Go 函数返回 Value 或 map，这些 Value 由谁释放，以及失败时会怎样
  - 修改 quickjs.go，或者字符串、错误或宿主函数在 Go 与 javascript 之间传递的方式
  - 从 javascript 得到的 Go 字符串里有替换字符，或者从 Go 传出的字符串在 javascript 里读起来不一样
  - 区分中断或内存不足与脚本抛出的错误
  - 用全局对象已经有的名字注册 Go 函数
  - 调用 VM.SetShapeCache 或 VM.SetRegExpCache
  - 给 javascript 一个它经常调用的 Go 函数，或者从 Go 返回一个列表（Builtin、NewArray）
  - 读取孤立代理项有影响的字符串参数，比如要拿去排序规则比较的（Args.WTF8）
  - 接收或返回字符串的 Builtin 很慢，或者它会查看短字符串的字符并且经常原样返回它们（Args.Latin1、Args.UTF16、NewStringLatin1、NewStringUTF16）
  - 隐藏脚本里函数的源码（SetStrip）
  - 在两次进入 javascript 之间运行一次 GC，或者读取一个 VM 占用多少内存（RunGC、MemorySize、GCThreshold）
  - 把某些东西按 VM 保存在 VM 之外，在 VM 关闭时丢掉（OnClose）
  - 把同一份字节码加载进许多个 VM（SharedBytecode、EvalSharedBytecode）
  - 对模块的字节码求值（EvalBytecode），或者返回时顶层 await 仍未完成的模块
  - 写 AsyncLocalStorage，或者写一个必须跟着 async 函数穿过 await 的值（GetAsyncContext、SetAsyncContext）
  - 给 VM 一个用 C 写的 React DOM 的 HTML 转义函数（RegisterEscapeText）
  - 给 VM 一个用 C 写的复制 React 元素 props 的函数（RegisterCopyProps）
  - 给 VM 用 C 写的、React DOM 为元素写的开始标签和结束标签函数（RegisterStartTag、RegisterEndTag）
  - 按类别读取一个 VM 占用的内存，或者把它留着不用的内存还给系统（MemoryUsage、ReleaseMemory）
  - 迁移调用过 SetModuleLoader、SetDefaultModuleLoader、StdAddHelpers 或 SetCanBlock 的代码
  - 构建在 internal/cengine/goasm_tripwire.<arch>.s 处失败，或者想用 -spectre=ret、-dynlink、-buildmode=plugin、-buildmode=shared 或 -linkshared 构建
---

# Go API

[English](go-api.md) · 简体中文

嵌入方在 `github.com/liustack/quickjs` 的方法返回值里看到的改动。中断、超时或内存不足时一次调用如何结束的改动，见 [uncatchable.zh-CN.md](uncatchable.zh-CN.md)。

## Call 报告函数表达式的异常

发布版本：v0.26.0。

`VM.Call` 和 `VM.CallValue` 报告对函数表达式求值时抛出的异常（比如内存不足，或者一个没有定义的名字），不再报告 `cannot call a non-function`。

## Go 无法转换的字符串作为错误返回

发布版本：v0.26.5。

不能转成 Go 的字符串作为错误返回，不再是 `""`。转换含非 ASCII 字符的字符串，以及拼接而成的字符串，都需要内存，所以在接近内存上限时，`Eval`、`Call`、`Object` 的 JSON、`Value.MarshalJSON` 和宿主函数的参数过去会得到空字符串，而内存不足的异常还留在上下文里。`toString` 抛错的 BigInt 现在报告那个异常，不再是编造的错误。`ErrorFromValue` 没有错误返回值：读不出来的字段保持为空，异常被丢掉。读取不是对象的被抛出值的名字和消息（`throw null`，接近内存上限时引擎会抛它，还有 `throw undefined`、symbol）时，不再在上下文里留下 TypeError。

测试：`gostring_test.go`。

## 孤立代理项得到合法的 UTF-8

发布版本：v0.26.8。

由 JS 字符串造出来的 Go 字符串是合法的 UTF-8。JS 字符串可以含孤立代理项，也就是代理对的一半而没有另一半，代码把字符串从一个 emoji 中间切开时就会造出来。`JS_ToCStringLen2` 把它写成它码点的三个字节，这不是 UTF-8，所以渲染成 Go 字符串的页面里有些字节，浏览器会显示成三个替换字符。现在每个孤立代理项变成 U+FFFD，和 Go 的 `utf16.Decode`、`TextEncoder` 和 Node 的 `Buffer` 的处理一样。

测试：`surrogate_test.go`。

## Go 来的非法 UTF-8 每个字节变成一个 U+FFFD

发布版本：v0.26.10。

不是合法 UTF-8 的 Go 字符串，变成 JS 字符串时，每个不是合法序列开头的字节变成一个 U+FFFD，和 Go 解码它的方式（`for range`、`[]rune`、`utf16.Encode`）以及 `encoding/json` 写出它的方式一致，所以原样传递的字符串和放在结构体里（按 JSON 传递）的字符串，到达时是一样的。这涵盖 `Call` 的参数、Go 函数的结果、map 的键和值、`NewString`、`NewAtom`、Go 函数返回的错误的消息、函数名和模块名。引擎过去会跳过非法字节后面的字节，包括 ASCII：`"\x80a"` 变成单个 U+FFFD 并丢掉 `a`，`"\xc0\x80"` 给出一个 U+FFFD 而不是两个，代理项的三字节形式（CESU-8，`0xED 0xA0` 开头）会变成那个代理项，所以两个这样的字节能合成一个 emoji。被截断的序列，比如 `"\xe4\xb8"`，现在每个字节给出一个 U+FFFD，而浏览器的 `TextDecoder` 对整个序列只给一个。源代码不变：解析器仍把非法 UTF-8 报告为 `SyntaxError`，并仍接受三字节形式的代理项。

测试：`invalidutf8_test.go`。

## *Error 说明调用为什么结束

发布版本：v0.26.7。

`*Error` 说明调用是否以脚本无法捕获的异常结束，以及原因，这样嵌入方不必比较名字和消息，而脚本是可以复制名字和消息的。VM 内存不足时（`OutOfMemory`，引擎造不出错误而抛出 `null` 时也算），`Interrupt` 或 eval 超时停住了脚本时（`Interrupted`），以及 javascript 调用的 Go 函数 panic 时（两者都不是），`Uncatchable` 为 true。Go 函数从它所求值的 javascript 得到的错误同样如此。脚本抛出的东西永远不是无法捕获的，即使是 `new InternalError('interrupted')` 或 `{name: 'InternalError', message: 'out of memory'}`，这些过去看起来和真的一模一样。取走一个无法捕获的错误不运行任何页面代码：`Name` 是 `InternalError`，`Message` 和 `Stack` 是错误自己的属性，`Error()` 是 `InternalError: <message>`。它过去会调用页面代码安装的 `Error.prototype.toString` 或 `name` getter，这可能改写消息。中断时在构造错误的过程中内存不足，算作 `OutOfMemory`。

脚本在 eval 超时或 `Interrupt` 之后又跑了 50 ms，并且自从被要求停止以来让引擎工作了 1000 次轮询，就会被杀死。花在脚本调用的 Go 函数里的时间，或者操作系统或 Go 调度器运行别的东西的时间，不会增加轮询次数。脚本停下之后引擎做的事也算：异常展开时引擎要释放脚本造的东西，正在进行的回收也要做完，它们要多久就轮询多久，所以造了很大数组的脚本可能在停下之后被杀掉。在 Apple M4 上于 055d56cd 测量，每个循环在 eval 超时和 `Interrupt` 下各停 12 次，共 24 次：每轮用展开（`[...big]`）复制一个 100 万个数的数组的循环，从请求到调用结束最多 618 次轮询、64.5 ms。数组换成 400 万个数时，24 次调用里有 21 次被杀，在请求之后 96 到 153 ms。100 万或 400 万个数的数组取 rest（`const [x, ...r] = big`）最多 526 次轮询、9.9 ms。在同一提交上，linux/amd64（AMD EPYC 7B13 服务器，测试钉在一个核上）每个循环在 eval 超时下停 12 次：100 万个数的数组取 rest、带空洞时取 rest、`Array.from` 和读到末尾的 `for-of` 最多 454 次轮询、6.1 ms，展开 596 次轮询、46.4 ms，都没被杀。展开 200 万个数时 12 次调用全部被杀，在请求之后 51 到 91 ms，400 万个数时 12 次里 11 次被杀，在请求之后 131 到 173 ms。不算这部分的话，大堆 VM 上的调用会超过期限好几秒才结束：释放 1 GiB 的数组就要这么久。调用返回一个同时匹配 `ErrKilled` 的被中断的 `*Error`（`errors.Is(err, quickjs.ErrKilled)`），而 VM 只能 `Close`：返回错误的方法返回一个匹配 `ErrKilled` 的错误，其他方法 panic，`Value.Free` 什么也不做。`MemorySize` 和 `GCThreshold` 是例外：它们只读一个计数器，所以像在任何 VM 上一样回答，池子可以在替换 VM 之前读到它占了多少内存。

测试：`uncatchable_test.go`、`hostuncatchable_test.go`、`hostpanic_test.go` 和 `killed_test.go`。

## RegisterFunc 不再因已有的名字而 abort

发布版本：v0.26.11。

当全局对象已经有这个名字时，比如在 `globalThis.g = 1` 之后，或者对 `Array` 或 `undefined`，`RegisterFunc` 和 `RegisterHostFunc` 不再让进程 abort。它们过去用 `JS_SetPropertyFunctionList` 安装函数，那是为新对象准备的，属性已存在时会 abort。现在函数会替换一个可配置的属性，和 `Object.defineProperty` 一样，并保持可写、可配置、不可枚举。不可配置的属性（`undefined`、`NaN`、`Infinity`、脚本的 `var` 或 `function`）会保留，不可扩展的全局对象也是：调用返回一个错误。现在名字按 javascript 看到的样子，在 VM 里只能用一次，也就是在非法 UTF-8 被替换之后，所以 `"\xe4\xb8"` 之后的 `"\xc0\x80"`（都是两个 U+FFFD）会得到 `func already registered`，不再是 abort，而带 NUL 字节的名字（NUL 会截断名字）是一个错误。括号里的名字也是字符串：函数列表过去把 `"[Symbol.iterator]"` 读成那个 symbol，而对括号里的其他名字会 abort。

测试：`register_test.go`。

## NewVM 失败时不泄漏

发布版本：v0.26.14。

当 `NewVM` 造不出上下文时，比如内存不够，它会释放之前分配的东西：`toString` 的 `argv` 和中断标志，这两样以前会泄漏。

## SetShapeCache 和 DefaultShapeCache

发布版本：v0.26.16。

`DefaultShapeCache`（2048）是新 VM 在对象消失后继续保留的 shape（引擎记录一个对象有哪些键、按什么顺序的结构） 数量，`VM.SetShapeCache(n)` 为单个 VM 修改它，0 表示关闭。一个在接近内存上限时运行的 VM，开着缓存可能会内存不足，而不开就不会：给这样的 VM 一个更小的缓存，或者不给。它的收益和代价见[保留的 shape](objects-and-shapes.zh-CN.md#保留-shape)。

## SetRegExpCache 和 DefaultRegExpCache

发布版本：v0.27.0。

`DefaultRegExpCache`（256）和 `DefaultRegExpCacheBytes`（1 MiB）是新 VM 最多保留多少个编译好的正则，以及占多少字节（计入它们的模式和标志）。`VM.SetRegExpCache(n, bytes)` 为单个 VM 修改这两者，并放掉它保留的东西，0 表示关闭。`n` 最多是 `math.MaxInt32/8`，和 `SetShapeCache` 一样。一个在接近内存上限时运行的 VM，开着缓存可能会内存不足，而不开就不会：给这样的 VM 一个更小的缓存，或者不给。它的收益和代价见[每个 VM 保留编译好的正则](regexp.zh-CN.md#按-vm-保存编译好的正则)。

## Builtin，作为内建函数调用的 Go 函数

发布版本：v0.26.17。

`Builtin` 是 javascript 当作内置函数调用的 Go 函数，不经过 `RegisterFunc` 和 `RegisterHostFunc` 的反射和转换。`NewBuiltin(name, length, fn)` 创建一个，每个进程一次，`VM.NewBuiltinFunction` 在任何 VM 里用它创建一个函数对象：它有自己的名字和长度，除此之外没有别的，没有 `prototype`，不能用 `new` 调用，它的 `toString` 显示 `[native code]`，和 `Math.max` 一样。`fn` 从 `Args`（`Number`、`String`、`Bool`、`Value`、`This`、`VM`）读它想要的参数，这些读取不做任何转换，并返回一个 `Value`（零值的 `Value` 是 `undefined`）。多个 goroutine 上的 VM 会同时调用它，所以它必须对此安全，并通过 `Args.VM()` 按 VM 保存它需要的东西。

它返回的 `*Error` 会被抛成它的 `Name` 所说的构造函数（`Error`、`TypeError`、`RangeError`、`SyntaxError`、`ReferenceError` 或 `InternalError`，其他名字都当作 `Error`），栈从这个 Builtin 开始，任何其他错误都当作 `TypeError`。

在 Apple M4 上，一次把两个数相加的调用，连循环算在内约 55 ns，而同样的调用用 `RegisterHostFunc` 的函数约 470 ns。VM 的上下文现在带一个 id，调用就是靠它不加锁地找到自己的 VM。`intl` 包以这种方式调用 Go（见 [time-and-intl.md](time-and-intl.zh-CN.md#intl-通过-builtin-调用-go)）。

测试：`builtin_test.go`。

## Args.WTF8

发布版本：v0.27.0。

`Args.WTF8(i)` 按引擎持有字符串的方式读取字符串参数：孤立代理项是它码点的三个字节（0xED，然后是 0xA0 到 0xBF，然后是一个后续字节），而 `Args.String` 对它给出 U+FFFD，代理对仍和以前一样是它那一个字符的 UTF-8。字符串里有孤立代理项时，结果不是合法的 UTF-8。必须区分孤立代理项的 Builtin 用这种方式读它的字符串，`Intl.Collator` 就这样做，以便像 Chromium 一样给它们排序。

测试：`builtin_test.go`。

## Builtin 的字符串在 Go 里转换

发布版本：v0.27.2。

`Args.String`、`Args.WTF8` 和 `VM.NewString` 在 Go 里转换，结果相同。`Args.String` 和 `Args.WTF8` 对引擎里完整一块的字符串直接从它的字符读取，而过去的 `JS_ToCStringLen2` 在引擎代码里扫描每个字符串，为每个非 ASCII 字符串在引擎内存里造一份 UTF-8 副本，Go 再把它复制一遍。`NewString`（以及每个变成 JS 字符串的 Go 字符串：`Call` 的参数、Go 函数的结果等）把字符直接写进新字符串，过去它要把 Go 字符串复制到 C 内存，让 `JS_NewStringLen` 去解码。引擎拼接后留成几块的字符串（rope，即由几段字符串连成的树，读取时才拼平）仍由引擎拼起来，比 javascript 允许的更长的字符串仍走 `JS_NewStringLen`，它像以前一样抛错。

它是安全的，因为结果是相同的字符串：测试把每个字符串用两种方式转换并比较。读取方面，覆盖引擎持有的每种形式（8 位和 16 位、ASCII、Latin-1、每种 UTF-8 长度、代理对、开头、中间和末尾的孤立代理项、rope、最多 65,000 个字符的字符串，以及 2,000 个随机字符串）。造字符串方面，是带每种非法 UTF-8 的同样的字符串，新字符串还必须和 `JS_NewStringLen` 造出来的一样是 8 位或 16 位。UTF-8 检查在每个最多三个字节的字符串上和 Go 的比较。读取一整块字符串不再需要引擎内存，所以接近内存上限时它在这里不会再失败：只有 rope 还可能。

用 `BenchmarkBuiltinString` 在 linux/amd64（AMD EPYC 7B13，固定一个核，3 种代码布局各 6 轮，每次调用的时间连造它的循环一起算，调用一个 javascript 函数是 60 ns）上测得：

| 调用 | 之前 | 之后 |
| --- | --- | --- |
| 原样返回参数的 Builtin | 97 ns | 81 ns |
| 读取一个 16 字符的 ASCII 字符串 | 131 ns | 105 ns |
| 读取并作为新字符串返回 | 230 ns | 134 ns |
| 同样的 16 个 Latin-1 字符，或 CJK 字符 | 359、484 ns | 211、314 ns |
| 读取 1,024 个 ASCII、Latin-1、CJK 字符 | 865、1685、4098 ns | 316、1096、3387 ns |
| 读取并作为新字符串返回它们 | 1338、7243、13699 ns | 522、4792、11286 ns |

把两个数相加的 Builtin 之前是 72 ns，之后是 75 到 81 ns，取决于代码布局：它的路径上没有任何改动，同样的布局也会让它移动同样多。

测试：`jsstring_test.go` 里的 `TestArgStringSameAsEngine`、`TestNewStringSameAsEngine`、`TestUTF8Units` 和 `TestBuiltinStringRoundTrip`。

如果上游改了 `JS_ToCStringLen2` 或 `JS_NewStringLen` 的转换方式，就要同步修改 `jsstring.go`：上面的测试在每种形式的字符串上比较这两者，会先失败。

## Args.Latin1、Args.UTF16、NewStringLatin1 和 NewStringUTF16

发布版本：v0.27.2。

`Args.Latin1(i)` 给出字符串参数的字符，每个一个字节，`Args.UTF16(i)` 给出它的 16 位单元，孤立代理项照原样，都不复制：这些字节和单元是引擎的，在 Builtin 返回之前有效，不可修改。引擎持有字符串只有一种方式，所以对于完整一块的字符串，这两者恰好有一个会回答。rope 两者都不给，不是字符串的值也一样：这些用 `Args.String` 读。`VM.NewStringLatin1(b)` 用字符为 U+0000 到 U+00FF 的字节造一个字符串，`VM.NewStringUTF16(u)` 用 16 位单元造一个，各做一次复制。`NewStringUTF16` 的单元如果都小于 U+0100，就造一个 8 位字符的字符串，和引擎造的一样。比 javascript 允许的更长的字符串是一个错误。

会查看短字符串的字符且经常原样返回参数的 Builtin，比如 HTML 的转义，用这种方式什么也不转换：它原地读字符并返回 `a.Value(0).Dup()`。React 的文本转义这样写，对一个 16 个字符、没有要转义的字符的字符串，每次调用 99 ns，而 React 自己的 javascript 是 200 ns，用 `Args.WTF8` 读取的写法是 156 ns（上面那项改动之后是 125 ns）。对有三个字符要转义的字符串是 163 ns，React 的 javascript 是 3.3 µs。

测试：`jsstring_test.go` 里的 `TestBuiltinLatin1UTF16`，以及 `builtinstring_test.go` 里的 `TestEscapeView`。

## 在 Go 里造的 Error 的 Error()

发布版本：v0.26.17。

在 Go 里造的 `*Error` 的 `Error()` 现在是 `Name: Message`，和 `Error.prototype.toString` 写出的一样，以前它是空的，所以 `RegisterHostFunc` 的函数为这样的错误抛出的 `TypeError`，现在也有这个消息。从 javascript 取来的错误的 `Error()` 和以前一样。

测试：`builtin_test.go`。

## NewArray

发布版本：v0.26.17。

`VM.NewArray(vals...)` 用在 Go 里造的值造一个数组，和数组字面量造的一样：它的元素是它自己的属性，所以页面放在 `Array.prototype` 上的 setter 永远不会运行，而且不需要 JSON。它借用这些值，和 `SetPropertyValue` 一样，并拒绝另一个 VM 的值，数字也一样。Builtin 以这种方式返回一个列表。

测试：`builtin_test.go`。

## SetStrip

发布版本：v0.26.17。

`VM.SetStrip(StripSource)` 编译函数时不带源码，所以它们的 `toString` 显示 `[native code]`，和引擎的内置函数一样，`StripDebug` 还去掉行号和列号。两者都是 `type Strip` 的值。它包装了 `JS_SetStripInfo`：引擎忽略 `"use strip"` 指令。它对 VM 此后编译的每个函数都生效，页面的 `eval` 和 `new Function` 的函数也一样。`Compile` 得到的字节码在任何加载它的 VM 里都保持这样，这正是扮演内置函数的 polyfill 想要的。

测试：`strip_test.go`。

## RunGC、MemorySize 和 GCThreshold

发布版本：v0.26.17。

`VM` 在 `SetGCThreshold` 旁边有三个 GC 方法。`RunGC` 立刻运行一次 GC，`MemorySize` 返回 VM 占用的内存，按内存上限的计数方式，`GCThreshold` 返回阈值。阈值数的是回收器能从中回收的内存，不含寄存器循环的栈。引擎在创建对象且这部分内存会超过阈值时运行 GC。每次完整 GC 之后，不管是引擎自己的还是 `RunGC` 的，阈值都是这部分内存的 1.5 倍，所以阈值不是最后一次设置的那个值，就意味着此后运行过一次完整 GC。取最大值的阈值会关掉引擎的 GC，它保持不变。

到 v0.29.1 为止，`RunGC` 不动阈值。当脚本把堆增长到超过它，而又没有创建对象时（数组元素就是这样），下一次调用的第一个对象会在 `RunGC` 之后又开始一次完整 GC：在一个 2^18 个元素的数组上约 53 次轮询。现在阈值在 `JS_RunGC` 里设置，`js_trigger_gc` 也调用它。在 `RunGC` 之后设置阈值的嵌入方，比如 tifa，看不到变化。

有了这些，嵌入方可以在两次进入 javascript 之间回收，这时 GC 不会拖住任何调用，并为下一次设置阈值，tifa 在两次渲染之间就是这样做的，不必通过 `reflect` 去读运行时。和 `SetGCThreshold` 一样，这期间不能有别的 goroutine 使用这个 VM，而 `Close` 之后调用会 panic。

测试：`gc_test.go`（`TestRunGC`、`TestRunGCSetsTheThreshold`），`internal/cengine/poll_test.go`（`TestRunGCSetsTheThreshold` 数 `RunGC` 之后那次调用的轮询），以及 `engine/test/unit-api.c`（`test_run_gc_threshold`）。

## MemoryUsage 和 ReleaseMemory

发布版本：v0.29.0。

`VM.MemoryUsage()` 返回一个 `MemoryUsage`：`JS_ComputeMemoryUsage` 在 VM 里数出的东西，带 Go 的名字。`MallocSize` 是 `MemorySize` 返回的值，`MallocLimit` 是内存上限（-1 表示没有），其他字段数 atom（引擎给属性名和常用字符串的编号）、字符串、对象、属性、shape、函数的字节码连同它的行表、引擎和 Go 的函数、数组和它们的元素，以及 ArrayBuffer。它遍历每个对象，所以是用来测量的：先运行 `RunGC` 把 javascript 已经够不着的东西排除在外。`Footprint`、`FootprintLimit`、`Chunks` 和 `ReleaseCandidate` 是 VM 从系统拿了多少、分了多少块，以及提出归还的有多少。

`VM.ReleaseMemory(keep)` 把 VM 留着不用的内存还给系统，只留下 `keep` 个字节，返回它还回去多少字节。对象保持原样。VM 正在运行 javascript 时它返回一个错误，比如从 javascript 调用的 Go 函数里。

和 `SetGCThreshold` 一样，这期间不能有别的 goroutine 使用这个 VM，而 `Close` 之后 `MemoryUsage` 会 panic。

测试：`memory_test.go`。

## OnClose

发布版本：v0.27.0。

`VM.OnClose(f)` 让 `Close` 在 VM 释放之后运行 `f`，所以 `f` 不能使用它。`Close` 每个函数运行一次，按给出的顺序。其他任何东西都不会运行它们：垃圾回收器不会，从没被关闭的 VM 也不会。

它是给在自己的表里按 VM 保存东西的包用的。一个值是引用 VM 的闭包的表项，会让 VM 从表里一直可达，所以垃圾回收器本来会为这个 VM 运行的清理永远不会运行，这个表项在进程的整个生命期里一直都在。在 `OnClose` 里丢掉这个表项就解决了这个问题。`temporal` 包就是这样保存 VM 的时钟的。

`Close` 即使有一步 panic 也会完成每一步。如果 `OnClose` 的某个函数 panic，它之后的函数仍会运行，它之后的步骤也会运行（VM 离开找到它的表，它加载的共享字节码被释放）。然后 `Close` 以第一个 panic 来 panic。`OnClose` 的函数里的 `Close` 返回 nil。在 C 引擎上，当操作系统拒绝取消映射 VM 的某些内存时，`Close` 返回这个错误，再次 `Close` 会取消映射剩下的，就像 `internal/cengine` 的 `VM.Close` 那样。其他任何东西都不会运行两次。

测试：`onclose_test.go`。

## EvalBytecode 在 Go 里等待模块的顶层 await

发布版本：v0.27.0。

模块的 `EvalBytecode` 和 `EvalBytecodeValue` 在 Go 里运行挂起的任务，直到模块的 promise 完成，并遵循 `ExecutePendingJobs` 的规则：

- 兑现的模块返回它的结果，被拒绝的模块把拒绝原因作为错误返回，和以前一样。
- 抛错的任务不会停止等待。当页面让 promise 的反应抛错时，它们就会抛，比如 `Promise` 子类的 species 给出一个会抛错的 resolve 函数，和模块毫无关系的 `FinalizationRegistry` 回调也一样。模块继续运行，调用返回第一个这样的错误。模块自己被拒绝时，则它的拒绝原因就是错误，就像 Node 报告模块的错误而丢掉任务的错误那样。
- 无法捕获的异常（中断、eval 超时、内存不足、Go 函数里的 panic）立刻以它的 `*Error` 或它的 panic 结束调用。模块排进队列的其他任务留在队列里，和 `ExecutePendingJobs` 留下它们的方式一样（见 [uncatchable.md](uncatchable.zh-CN.md#中断无法穿过-promise-或迭代器被捕获)）。
- 当没有任务了而模块仍在挂起，比如 `await new Promise(() => {})`，错误会说模块的顶层 await 仍在挂起，且没有任务可以运行（如果有任务抛过错，就和第一个任务的错误连在一起），VM 可以继续使用。模块在等待的东西仍可能从任务队列之外完成：Go 函数返回、Go 以后再兑现的 promise，或者嵌套的 `EvalBytecode` 返回之后由外层调用兑现的 promise。那么之后的 `ExecutePendingJobs` 会运行模块的其余部分。

它们过去调用 `quickjs-libc.c` 的 `js_std_await`，也就是 `qjs` shell 的辅助函数。当任务队列已空而 promise 仍在挂起时，它读 `js_std_init_handlers` 设置的状态，而 VM 从不设置它，进程就因空指针解引用崩溃。失败的任务被打印到 stderr 然后丢掉，所以模块第一个 `await` 之后的 eval 超时，在队列变空后同样崩溃。`qjs` shell 在那里不会崩溃，它在空队列上永远空转，而 Node 对顶层 await 永不完成的模块，以退出码 13 结束它。

测试：`moduleawait_test.go`，以及 `timeout_test.go` 里 `TestNestedEvalKeepsTimeout` 和 `TestNestedEvalKeepsInterrupt` 的模块用例。

## 模块运行后释放它的顶层

发布版本：v0.28.4。

当 `EvalBytecode`、`EvalBytecodeValue`、`EvalSharedBytecode` 或 `EvalSharedBytecodeValue` 把模块运行到结束时，它会释放模块的顶层函数：顶层的代码，以及没有模块里的函数使用的顶层变量，连同它们握着的东西。模块创建的函数保留它们用到的变量，模块把它导出的变量与这个函数分开保存。模块只运行一次，所以没有东西会再调用这个函数。上游把它保留到模块消失，而上下文里的模块活到 VM 关闭，所以加载过模块的 VM，比把同样的代码作为脚本运行的 VM 占得更多，脚本的顶层函数在它返回时就消失了。

调用返回时仍在挂起的模块，以及失败的模块，保留这个函数。

把 tifa 示例商店的 SSR 包作为模块：一个 VM 加载后占 8.66 MiB，不是 9.44 MiB，同一个包作为脚本（一个 IIFE）是 8.63 MiB。在 darwin/arm64 上，在 `RunGC` 之后用 `MemorySize` 测得。

测试：`moduleawait_test.go` 里的 `TestModuleFreesItsTopLevel`。

## SharedBytecode

发布版本：v0.28.1。

`NewSharedBytecode(bytecode)` 把 `Compile` 得到的字节码复制一次，`VM.EvalSharedBytecode(b)` 和 `VM.EvalSharedBytecodeValue(b)` 像 `EvalBytecode` 和 `EvalBytecodeValue` 那样加载它，但把每个函数的源文本和行表留在那一份副本里，不复制进 VM。加载同一个包的一池 VM 只持有它们一份：tifa 示例商店的一个 VM 少占 2.36 MiB。`toString`、错误栈和行号不变。`MemorySize` 和内存上限不再计入这些字节，`SharedBytecode.Len` 返回副本占用的大小。

`SharedBytecode.Close` 放弃 `SharedBytecode` 的持有，加载了它的每个 VM 一直持有它到 `VM.Close`：最后一个释放副本，所以 VM 可以比创建它的池子活得更久。`Close` 之后再加载会返回错误。`SharedBytecode` 对多个 goroutine 同时使用是安全的。它如何工作、能省多少，见 [memory-and-allocation.md](memory-and-allocation.zh-CN.md#所有-vm-共用一份源码文本和行表)。

测试：`sharedbytecode_test.go`。

## promise 任务的异步上下文

发布版本：v0.28.5。

`GetAsyncContext` 和 `SetAsyncContext` 是两个 Builtin（见 [Builtin](#builtin作为内建函数调用的-go-函数)），读取和设置 VM 的异步上下文：一个对象，或者没有。promise 的反应，也就是 `then`、`catch`、`finally` 和 `await` 注册的东西，保存它被注册时 VM 所处的上下文，并带着它运行：处理函数运行期间 VM 有那个上下文，处理函数返回或抛错时恢复到原来的上下文。`AsyncLocalStorage` 需要这一点才能在 `await` 之后找到它的存储，这也是 V8 为 Node 保存的，叫作 continuation preserved embedder data。`SetAsyncContext` 传入任何不是对象的东西，就是设为没有，之后 `GetAsyncContext` 返回 undefined。

这个对象就是整个上下文：`AsyncLocalStorage` 把它的存储放在里面，用一个从每个实例到它的存储的 `Map`，`run` 造一个带有自己存储的新对象，设置它，调用函数，再把旧的设回去。对象一旦设置，就没有东西会改它，所以反应通过引用持有它的上下文。

VM 开始时没有上下文，没有上下文时，一个反应多花一次测试，它的处理函数和以前一样。有上下文时，每个带函数处理函数的反应，以及每个可 thenable 的 `then`，都会得到一个握着上下文和函数的小函数。`Close` 释放 VM 当前的上下文，仍在挂起的反应连同它们一起释放自己的上下文。

哪些会跟着上下文，哪些不会，用 `AsyncLocalStorage` 对照 Node 24 检查过：

- async 函数在每个 `await` 之后都有它被调用时的上下文，多个重叠时也一样。
- 反应带着注册它的那个 `then` 的上下文运行，不是兑现 promise 的代码的上下文。
- 在反应里设置的上下文持续到反应结束。
- 上下文覆盖处理函数，不覆盖整个任务。任务在处理函数之后做的事，也就是兑现 `then` 返回的 promise，以及把没有人处理的拒绝告诉宿主，看到的是任务运行时 VM 所处的上下文。没有上下文时注册的反应不被包装：它的处理函数带着当时 VM 所处的上下文运行，它设置的上下文在它之后继续存在。
- `Promise.all`、`finally`、异步生成器和 `for await` 保持它，因为它们注册反应。
- 不是 promise 的 thenable 的 `then`，在一个任务里带着用它兑现 promise 的代码的上下文运行，和 `await thenable` 一样。
- 引擎里没有定时器、I/O 和事件，所以添加它们的宿主自己保存和设置上下文。

这是最后一个定型的改动。它在 `asyncContextSites` 里给这两个函数和两个任务（`promise_reaction_job` 和 `js_promise_resolve_thenable_job`）记下指纹，当某个发布版本在反应的处理函数里又加了一处存储，或者又加了一处把 thenable 任务排进队列的地方时，就停下。

改动在 C 里（`engine/quickjs.c`）：`quickjs.h` 里的 `JS_GetAsyncContext` 和 `JS_SetAsyncContext`，上下文放在 `JSRuntime` 的 `async_context` 字段里，`js_async_context_wrap` 在同样两个地方被调用。`JS_FreeRuntime` 释放仍设置着的上下文。

## RegisterEscapeText

发布版本：v0.29.2。

`VM.RegisterEscapeText(name)` 在全局对象上定义一个名为 `name` 的函数，它做的是 React DOM 的字符串渲染器里的 `escapeTextForBrowser` 做的事，用 C 写在引擎里。React 对它写出的每段文本和每个属性值都调用这个函数，在 tifa 示例商店的六个工作台页面各渲染一次时，约 15,000 次，而在 JavaScript 里，每次调用的代价是一次正则搜索、对含有五个要转义字符之一的字符串逐字符循环，以及调用本身。

- **它给出什么。**参数按 `"" + text` 变成字符串：字符串保持原样，数字、布尔值或 bigint 变成它的文本，对象以提示 `"default"` 经过 `valueOf` 和 `toString`（或 `Symbol.toPrimitive`），symbol，或者抛错的 `toString`，抛出 `"" + text` 所抛的错。然后 `&`、`<`、`>`、`"` 和 `'` 各自变成 `&amp;`、`&lt;`、`&gt;`、`&quot;` 或 `&#x27;`，一次一个 UTF-16 码元，所以孤立代理项原样通过。不含这五个字符的字符串，作为同一个字符串返回，不复制。没有参数时给出 `"undefined"`。结果超过 1,073,741,823 个码元时抛构造这种字符串时抛的 `RangeError`。
- **选择加入。**引擎自己不往全局对象上放任何东西。只有宿主调用 `RegisterEscapeText` 时，这个函数才存在于 VM 里，名字由宿主选。属性可写、可配置、不可枚举，并替换该名字的可配置属性，和 `RegisterFunc` 一样。全局对象不能放弃的名字（`undefined`、`NaN`、脚本的 `var`）是一个错误。函数的外形像内置函数：`name` 是给定的名字，`length` 是 1，没有 `prototype`，不能用 `new` 调用，`toString` 显示 `[native code]`。
- **限制仍然有效。**它在长字符串每 4096 个码元一块的开头读中断标志，在计数时读一次，写入时再读一次，所以 `Interrupt` 或 eval 超时会在那里停住它，两次读取之间不会超过 4096 个码元。渲染在构造结果时内存不足，就以内存不足错误结束，和任何字符串一样。
- **为什么用C而不是Go。**Go 的 `Builtin` 每次调用比引擎的 C 函数多约 22 ns（48 到 55 ns 对 26 到 38 ns），而对短文本，这就是转义代价的大部分。在 tifa 里，Go 版本让商店渲染得更慢，而这一版让它渲染得更快（[性能](../performance.zh-CN.md#c-里的-react-dom-文本转义)）。

C 的部分是 `engine/quickjs.c` 和 `quickjs.h` 里的 `js_escape_text` 和 `JS_NewEscapeTextFunction(ctx, name, name_len)`，Go 方法调用它们。

测试：`escape_test.go` 把这个函数对着 React 自己的文本运行：一个 13 个字符的字母表上最多 4 个码元的所有字符串（两种宽度、孤立代理项、一个代理对、NUL），1 到 100,003 个码元的字符串，其中第一个、最后一个和块边界的位置上有一个或全部五个字符，rope，34 个不是字符串的值，数调用次数的对象，错误，以及不带参数或多带参数的调用。它检查属性、函数的外形、第二次注册、不能使用的名字、超时会停住它，以及字符串太长的 `RangeError`。`engine/test/unit-api.c` 在 AddressSanitizer 和 UndefinedBehaviorSanitizer 下，在 C 树上运行同样的比较，并检查干净的字符串以同一个指针返回，工作之前和工作中间的中断会停住它，标志在每一步的 4096 个码元内被读到（有计数），在第一次调用拼平一个还没有人拼平的长 rope 期间的中断会停住这次调用且不留下平的副本，内存不足不会留下任何东西，以及带 NUL 字节的名字。

## RegisterCopyProps

发布版本：v0.29.3。

`VM.RegisterCopyProps(name)` 在全局对象上定义一个名为 `name` 的函数，它做的是 React 的 `createElement` 和 `cloneElement` 把配置的键复制进元素 props 的那个循环所做的工作，用 C 写在引擎里。在 QuickJS 里这个循环每个键约 250 ns（一遍 `for in`、一次 `hasOwnProperty.call` 的调用和一次设置），而 tifa 示例应用的页面一次渲染通过它们创建 1,500 到 3,400 个元素。

- **它接收什么，给出什么。**`copy(config, props, hasOwnProperty, isClone)`：配置，React 造的 props 对象（`createElement` 里是 `{}`，`cloneElement` 里是元素 props 的副本），React 自己的 `hasOwnProperty`（它加载时拿到的 `Object.prototype.hasOwnProperty`），以及 `cloneElement` 用的 `true`，此时当 `config.ref` 是 undefined 时它还会跳过 `ref`。复制了就回答 `true`，什么也没改就回答 `false`，此时脚本运行 React 的循环。不管哪种情况，props 都是循环会造出来的样子。
- **何时复制。**只在它的结果和循环的结果在脚本可见的任何方面都不可能不同时。配置是普通对象（不是 proxy、数组、函数、类实例或 String 对象），原型是 `Object.prototype`，自己的键最多 32 个，没有数组下标、`__proto__` 或 `stack`，循环读的每个键都是普通数据属性。getter、setter、symbol 键和不可枚举的键，要么循环不读，要么让调用转去循环。props 是原型相同、可以扩展的普通对象，只有普通的可写数据属性。`Object.prototype` 除了 `__proto__` 之外没有 setter 和只读属性，这样设置一个键就是创建它。`hasOwnProperty` 是内置的，`hasOwnProperty.call` 是内置的 `Function.prototype.call`。页面在 React 加载之后改了其中任何一项，后面的调用就转去循环。
- **如何复制。**对一个还没有任何属性的 props 对象，如果配置的属性全是可枚举、可写、可配置的字符串键，且没有要跳过的，它一步就取得配置的 shape 和值（展开用的那一步）。否则，键一个接一个地添加，props 已有的就改设值。
- **选择加入。**引擎自己不往全局对象上放任何东西。只有宿主调用 `RegisterCopyProps` 时，这个函数才存在于 VM 里，名字由宿主选，属性和名字的规则同 [`RegisterEscapeText`](#registerescapetext)。它的外形像内置函数：`length` 是 4，没有 `prototype`，不能用 `new` 调用，`toString` 显示 `[native code]`。
- **限制仍然有效。**这个函数在写入任何东西之前，最多读配置的 32 个键，以及 props 的 shape 和 `Object.prototype` 的 shape 里最多 64 个槽位（包括被删除的键）。新的 `Object.prototype` 有 12 个槽位，tifa 页面里元素的 props 最多 24 个。任何一项超过，就转去循环，循环由中断标志和 eval 超时照常停住，而限制之内的复制花几百纳秒，所以这个函数不读中断标志。添加一个键时内存不足，调用以内存不足错误结束，和循环里的设置一样。
- **为什么不在JavaScript里做检查。**在 JavaScript 里测试配置，再用一次展开复制它，每次调用要 310 到 380 ns，比循环处理一个有零个、一个或两个键的配置还多，而且它排除不了 proxy、删掉后面某个键的 getter，或者 React 加载之后加到 `Object.prototype` 上的 setter。在 C 里，这些检查是调用的一部分，能排除它们：没有键的配置，经过这个函数比经过空循环用的时间更少。

C 的部分是 `engine/quickjs.c` 和 `quickjs.h` 里的 `js_copy_props` 和 `JS_NewCopyPropsFunction(ctx, name, name_len)`，Go 方法调用它们。它用展开的 shape 那一步（`js_copy_shape`）、键缓存保存的键的种类，以及 `add_property`。`JSContext` 保存它用来比较键的五个 atom（`copy_atoms`），在函数创建时造出，随上下文释放。

测试：`engine/test/compare/copy_props.js` 里有 React 19.3 的函数，一份是 React 写的样子，一份是在循环前面加上这个调用，像 tifa 的插件写的那样，并给它们 147 个配置（普通的和空的，各个位置上的 `key` 和 `ref`，`__self` 和 `__source`，任何键上的 getter 和 setter，会添加、改变或删除键的 getter，symbol，原型，null 原型，类实例，`__proto__` 键，下标键，冻结和密封的对象，数组，函数，原始值，proxy，32 个和 33 个键，被改过的 `Object.prototype`，被替换的 `Function.prototype.call`），配上 `cloneElement` 的 16 种 props（其中有 64、65 和 200 个槽位的 props，以及一个有 75 个被删除的键的 props），一个多了 50 个和 100 个可写属性的 `Object.prototype`，然后是从固定种子生成的 6,000 个随机配置（3,000 轮里每轮造一个 `createElement` 和一个 `cloneElement`，用一种 props）。它还冻结、密封配置、props 和与副本共用 shape 的第三个对象，并给它们换一个原型，检查其他对象不变且仍能增长。每一对必须给出同样的键，同样顺序、同样描述符的 props，同样的错误，以及同样的读取和调用，而列出的配置是否被这个函数复制，必须和文件里写的一致。它还用 React 从不给的参数调用这个函数，检查它回答 `false` 且什么也没改。C 树在 AddressSanitizer 和 UndefinedBehaviorSanitizer 下运行它，`unit-oom` 在每个空位上拒绝内存来复制，`copyprops_test.go` 通过 Go API 运行同一个文件，带上它定义的属性、第二次注册和它不能使用的名字。每个条件都轮流去掉一次，看这个文件是否失败，只有对 `Object.prototype` 上访问器的测试没有失败，因为访问器没有可写标志，对不可写属性的测试也会抓到它。

## RegisterStartTag 和 RegisterEndTag

发布版本：v0.29.10。

`VM.RegisterStartTag(name)` 在全局对象上定义一个名为 `name` 的函数，它像 React DOM 的服务端渲染器在 `pushStartGenericElement`（react-dom 19.3）里那样写出元素的开始标签，连同它调用的那些函数：`startChunkForTag`、`pushAttribute`、`pushStringAttribute`、`pushBooleanAttribute`、`pushStyleAttribute`、`sanitizeURL`、`isAttributeNameSafe` 和 `escapeTextForBrowser`。`VM.RegisterEndTag(name)` 定义一个给出结束标签的函数，像 `endChunkForTag` 那样。两者都用 C 写在引擎里。在 JavaScript 里，元素的每个属性要花一次 `pushAttribute` 调用、一次对字符串比较的 switch、一次转义调用，以及向目标压入五个字符串，而页面上的大多数元素都走这条路径：tifa 的每个商店页面渲染一次，约有 9,700 个开始标签。

- **开始标签函数接收什么，给出什么。**`startTag(target, props, type, formatContext, hasOwnProperty, decline)`：`pushStartGenericElement` 的四个参数，React 自己的 `hasOwnProperty`（它加载时拿到的 `Object.prototype.hasOwnProperty`），以及 `decline`，一个只有调用方持有的对象。它向 `target` 压入一个字符串：带属性的开始标签，以及当 `props.children` 是字符串时，它后面转义过的文本。它返回 React 的函数返回的东西：children 是字符串时返回 `null`，否则返回 `props.children`，没有 children 时返回 `null`。React 把同样的文本分成几个字符串压入，而渲染器只是把它们连起来。
- **何时写。**只在 React 的函数不可能做别的事时。否则它返回 `decline`，什么也不改，脚本运行 React 的函数。它在这些情况下拒绝：
  - `hasOwnProperty` 或 `Function.prototype.call` 不是内置的，或者 `target` 不是普通数组（原型是 `Array.prototype`，除了 length 没有别的属性，可扩展，快速数组，length 等于元素个数），`Array.prototype` 或 `Object.prototype` 上有过下标属性（`push` 会在那里设置：一个 setter，或者一个会抛错的只读属性），或者 `Array.prototype.push` 不是内置的。内置函数是靠它们的 C 函数找到的，所以注册之前被替换的函数也不会被当成内置的。
  - `type` 不是 React 的标签正则 `/^[a-zA-Z][a-zA-Z:_\.\-\d]*$/` 接受的扁平字符串（对其他情况 React 会抛错）。
  - `formatContext.viewTransition` 不是自己的、值为 `null` 的数据属性。
  - `props`，或者设置了的 `style`，不是原型为 `Object.prototype` 的普通对象，自己的键超过 32 个，有数组下标键，或者 `for in` 列出的键里有 getter 或 setter。
  - 设置了 `dangerouslySetInnerHTML`，或者用了少见的属性 `xlinkHref`、`capture`、`download`、`cols`、`rows`、`size`、`span`、`rowSpan` 和 `start` 之一。
  - React 会转成字符串的值是对象（函数被丢掉，和 React 丢掉它一样）或 bigint，样式的值是对象、函数、symbol 或 bigint，或者 URL（`src`、`href`、`action`、`formAction`）按 React 的正则是 `javascript:` URL，或者是还没拼平的字符串。
  - `pushAttribute` 默认分支的属性名不是 ASCII 或有 64 个或更多字符，或者不以 `--` 开头的样式名不是 ASCII。
  - 标签可能比字符串能容纳的更长：它写的每个字符串按 6 个码元计（最长的实体），并为中间的名字和数字留出 8,192 个码元。

  它假定 React 的函数调用的 `String`、`RegExp`、`Map` 和 `Set` 的内置方法就是引擎的方法。在 tifa 示例应用每个页面各渲染一次的 16,310 个开始标签上，它一个也没拒绝。
- **结束标签。**`endTag(type)` 给出 `"</" + type + ">"`。它按字符串的地址保存每个字符串的结束标签，所以同一个标签（JSX 把它造成一个字符串）再次给出同样的结束标签，不用复制。不是字符串的参数按 `"</" + type` 变成字符串（不带提示的 ToPrimitive，然后 ToString），而这个函数抛出那一步所抛的错。
- **选择加入。**引擎自己不往全局对象上放任何东西。只有宿主注册它们时，这些函数才存在于 VM 里，名字由宿主选，属性和名字的规则同 [`RegisterEscapeText`](#registerescapetext)。它们的外形像内置函数：`length` 分别是 6 和 1，没有 `prototype`，不能用 `new` 调用，`toString` 显示 `[native code]`。
- **限制仍然有效。**一次调用最多读 props 的 32 个键和样式的 32 个键。它对比自己的步长更长的字符串所做的每个循环，在每一步之前和最后一步之后读中断标志：转义字符串、写样式名的连字符，或者跳过样式值周围或 URL 之前的空白时，每 4096 个码元一次，扫描标签或样式名，或者复制长的片段（标签、样式名、结束标签、做好的开始标签）时，每 524,288 个码元一次，所以 `Interrupt` 或 eval 超时会在那里停住它（[uncatchable.md](uncatchable.zh-CN.md#开始和结束标签函数在每个长循环里读中断标志)）。它只在引擎没有内存或宿主停住它时才抛错，此时 `target` 保持原样。
- **它保存什么。**每个上下文为两个函数保存一个小缓存，随它们中的第一个创建，里面有它要找的 atom（`viewTransition`、`call`、`push`）。创建它不运行脚本：`Function.prototype.call` 和 `Array.prototype.push` 是引擎在第一次用到时才创建的，这时创建它们，不经过 getter 读取，所以没有超时的注册不可能运行脚本，而缓存要等它完整之后才放进上下文。每个属性名和每个样式名的写法，以及每个标签字符串的结束标签，放进三张表。每张表在第一次需要它的调用时创建，有 16 个条目，半满时翻倍，最多 512 个名字和 256 个结束标签。一个名字放在哪里就留在哪里，直到它的表将超过这个数的一半，此时表被清空再重新填，所以一个页面每个名字只造一次。开始标签写进缓存在两次调用之间保留的一个缓冲区（一开始 128 个码元，在一次调用把它撑到超过 1024 个码元或变成宽字符之后释放），然后复制成一个刚好那么长的字符串，缓存保存的名字和结束标签也都是刚好的长度。留在缓冲区里的字符串会占着缓冲区长到的那么大的块，因为 arena 块从不缩小，而这些字符串一直活到渲染被写出，渲染在这期间创建的长寿对象（页面缓存的条目，新 props 的 shape）就会在渲染被释放后握着几乎空了的 arena。注册既不造表也不造缓冲区：这两个函数给 `MemorySize` 加 1.8 KB，大部分是全局对象的两个属性。在 tifa 的商店上，包加载完后，一个 VM 比没注册它们的多占 1.6 KB，页面渲染过之后这些表占约 8 KB。在 tifa 的池子里，每次渲染之后做一次年轻代回收，商店的一个 VM 在 64 次或 1,000 次渲染之后多占 26 KB（开始标签把缓冲区当作字符串保存时是 107 KB）。缓存里的任何东西对脚本都不可见，它随上下文释放。

C 的部分是 `engine/quickjs.c` 和 `quickjs.h` 里的 `js_start_tag`、`js_end_tag`、`JS_NewStartTagFunction(ctx, name, name_len)` 和 `JS_NewEndTagFunction(ctx, name, name_len)`，Go 方法调用它们。对 `hasOwnProperty.call` 的检查就是 `RegisterCopyProps` 的那个（`js_copy_props_call_plain`）。`JSContext` 保存缓存（`emit_cache`）。

测试：`engine/test/compare/start_tag.js` 里有 react-dom 19.3.1 的原样函数，并让每次调用既经过它们，也经过 `startTag`。139 个列出的调用走遍 `pushAttribute` 和 `pushStyleAttribute` 的每个分支（转义、数字、布尔值、函数、symbol、别名、`data-` 和 `aria-` 名字、`on` 名字、不安全的名字、URL、`ms` 前缀、自定义属性、要裁掉的空白）和上面每一种拒绝，每个都带它必须给出的答案。更多的调用替换 `hasOwnProperty`、`Function.prototype.call` 和 `Array.prototype.push`（用 `unshift`，同一个 C 函数，另一个 magic），给出被冻结、被密封、长度超过元素个数、子类或另一个原型的 target，以及没有普通 `null` 的 `viewTransition` 的 `formatContext` 值。2,400 次调用用到比缓存能容纳的更多的属性名和样式名，4,000 次调用用从固定种子生成的随机 props。对每次调用，`startTag` 必须压入 React 压入的文本并返回 React 返回的东西，或者拒绝并让 `target` 保持原样，并且在 React 会抛错的地方它从不接手这次调用。结束标签对着 `endChunkForTag` 检查：字符串、数字、带 `toString`、`valueOf` 和 `Symbol.toPrimitive` 的对象，以及 symbol 和抛错的 `toString` 的错误。`unit-oom` 在每个空位上拒绝内存来写开始标签和结束标签，包括表创建和增长的时候，以及缓冲区因长文本而超过最大值而增长的时候，并检查 target 没有变。`unit-white`（`test_emit_tables`）检查第一次调用之前没有造表，200 个名字和 100 个结束标签留在它们的表里，所以第二遍不会添加，以及超过最大值的表被清空再重新填。`test_emit_buffer` 检查短的、长的和宽的标签的字符串，名字、样式名和结束标签的字符串，都有与其长度相符的大小类别，缓冲区在一个 300 个码元的标签之后保留，在 2,000 个码元的文本或宽字符之后释放，以及缓冲区被占用时发生的调用会造自己的缓冲区，缓存保留两个中的一个并释放另一个。`tags_test.go` 通过 Go API 运行同一个文件，带上每个函数定义的属性、第二次注册、不能使用的名字、对长的 children 文本、属性、样式值、样式名、标签和结束标签的超时，以及把标签留给 React 的精确长度。`TestTagsMemory` 检查注册不造表，`TestTagsMemoryLimit` 检查表增长时内存不足是无法捕获的内存不足错误，target 不变，并且上限提高后调用能工作。`TestRegisterTagsRunsNoScript` 在 `Array.prototype.push` 和 `Function.prototype.call` 上放 getter，它们或者注册另一个函数，或者永不返回，并在注册之前替换这两者：注册不会运行这两个 getter，而这些调用会拒绝。`TestStartTagIndexOnPrototype` 在 `Array.prototype` 或 `Object.prototype` 上放一个 setter 或只读的下标，调用会拒绝，`start_tag.js` 对着 React 检查下标 0 上的 setter。带 NUL 的名字（`children\u0000x`、`id\u0000x` 等）像 React 跳过它们那样被跳过。`unit-oom`（`test_tags_register_oom`）在每个空位上拒绝内存来注册，并检查再次注册能工作。

## 没有模块加载器、shell 辅助函数和 SetCanBlock

发布版本：v0.29.0。

`VM.SetModuleLoader` 连同 `ModuleLoaderFunc` 和 `ModuleNormalizeFunc`，`VM.SetDefaultModuleLoader`，`VM.StdAddHelpers` 和 `VM.SetCanBlock` 都没有了。它们不是渲染器需要的东西，而 C 引擎一个也没有：它不读文件，也没有线程。

- 用 `EvalModule` 求值的模块只能 import VM 里已经有的模块。它自己的名字是 `<eval>`，所以它可以 import 自己。import 其他任何东西都抛 `ReferenceError`（`could not load module`）。求值之前，把脚本的模块打包成一个。
- `console.log`、`print`、`performance.now` 和 `__loadScript` 没有定义。把 VM 需要的东西作为 Builtin 或用 `RegisterFunc` 给它。
- `Atomics.wait` 抛 `TypeError`（`cannot block in this thread`），除非调用过 `SetCanBlock(true)`，它本来就是这样。

测试：`own_keys_test.go`（一个通过 `<eval>` import 自己的模块）。

## 比检查过引擎的 Go 更新的 Go 会让构建停下

发布版本：v0.29.11。

用比检查过引擎的 Go 更新的 Go 构建，会以一个错误停下：

```
undefined: quickjs_is_checked_up_to_go1_27_see_docs_upgrading_md_or_build_with_tags_quickjs_anygo
```

引擎的 Go 汇编依赖了 Go 里一些 Go 1 承诺没有覆盖的东西：汇编器如何布局栈帧并开始一个函数，它让哪些小函数不做栈检查，以及内存故障变成 panic 时 Go 往栈上放什么（[upgrading.md](../upgrading.zh-CN.md#换用更新的-go)）。更新的 Go 无论如何都能构建这个模块，其中任何一项的变化只有在程序运行时才会显现。现在构建会停下。模块允许构建的每个 Go，1.26.6 和 1.26 的更晚发布版本，以及 1.27 的每个发布版本，都照常构建：构建行里的 `go1.N` 标签跟着构建所用的 Go，所以 `go1.28 && !quickjs_anygo` 这一行对它们都是 false。下限是 `go.mod` 的 `go` 行，在引擎用 Go 1.26 检查过之后同时从 1.27.0 降到 1.26.6（为什么是 1.26.6 而不是 1.26.0，见 [upgrading.md](../upgrading.zh-CN.md#换用更新的-go)）。

- **仍要构建时**，用一个 liustack/quickjs 没有检查过的 Go，加上 `-tags quickjs_anygo`。
- **新的Go发布时**，liustack/quickjs 用它运行 [upgrading.md](../upgrading.zh-CN.md#换用更新的-go) 的检查，并在一个补丁发布里把这一行移到下一个版本。迁到新 Go 的程序也就迁到 liustack/quickjs 的这个发布版本。
- **Go 的补丁发布**不加区分：构建行可以写 `go1.28`，但不能写 `go1.27.3`。

这个错误在它自己的包 `internal/goversion` 里，`internal/cengine` 导入它，所以 go 命令先构建它，它是构建报告的唯一错误。

测试：`TestNewerGoStops`（`internal/goversion`）用被检查过的 Go 的发布标签、接下来两个版本的标签和 `quickjs_anygo` 读构建行，并检查错误的名字和这一行给出的是同一个版本。`TestEngineNewerGo`（`enginedeps_test.go`）用手工给出的这一行的 `go1.N` 标签构建这个包：构建只以 `internal/goversion/newer.go` 的错误失败，其中一个是上面那个名字，而再加上 `quickjs_anygo` 就能构建。手工给出的标签还会让 go 命令说这个文件要求更新的 Go，更新的 Go 不会这么说。

## 会改变汇编器对引擎编码方式的构建选项会让构建停下

发布版本：v0.29.11。

生成的引擎把每个函数的函数体写成原始字节（`BYTE`、`WORD`、`QUAD`），分支偏移是固定的，只把调用和符号地址的加载留给 Go 汇编器。让汇编器用另一个长度来编码其中某一条的构建选项，会移动那些固定分支的目标。构建成功，第一次 `NewVM` 就以 SIGSEGV 崩溃。有两个选项会这样：

- amd64 上的 `-asmflags=all=-spectre=ret`（以及 `-spectre=all`）。2 字节的 `CALL AX` 变成经过 retpoline（`runtime.retpolineAX`）的 5 字节调用，在 `src/cmd/internal/obj/x86/asm6.go`（`span6` 的 `ctxt.Retpoline` 块）里。在 arm64 上汇编器自己拒绝这个选项（`-spectre=ret not supported on arm64`）。
- `-dynlink`，go 命令在 `-buildmode=plugin`、`-buildmode=shared` 和 `-linkshared` 时把它传给汇编器（链接出来的程序为此需要 cgo 和 linux）。在 amd64 上，`LEAQ sym+off(SB)` 变成从 GOT 的一次加载加一个 `LEAQ`，两条指令，R15 带着地址（`obj6.go` 里的 `rewriteToUseGot`）。在 arm64 上，`MOVD $sym(SB)` 变成从 GOT 的一次加载加一个 `ADD`（`obj7.go` 里的 `rewriteToUseGot`）。

这两种构建现在都在汇编器里停下，错误指向 `internal/cengine/goasm_tripwire.<arch>.s` 的某一行：

```
asm: non-retpoline-compatible: 00002 (.../internal/cengine/goasm_tripwire.amd64.s:26)	CALL	(AX)
asm: goasm_tripwire.amd64.s:32: when dynamic linking, R15 is clobbered by a global variable access and is used here: ...	MOVQ	R15, BX
asm: do not know how to handle TYPE_ADDR in 00002 (.../goasm_tripwire.arm64.s:26)	MOVWU	$...goasmTripwireVar(SB), R0 with -dynlink
```

文件里每一行上面的注释说明是哪个选项，并说 liustack/quickjs 不能用它构建。这个函数从不被调用，也从不被链接，所以它不给程序增加任何东西。这些文件用带点的名字 `goasm_tripwire.<arch>.s`，因为 `make gen` 会删除文件夹里每个 `*_<arch>.s`。每一行都是汇编器按原样接受、在该选项下拒绝的指令（`CALL (AX)` 是经内存的调用，retpoline 没法替换它，而在全局访问之后读 R15 在 `-dynlink` 下被拒绝）。

- **让引擎保持原样的选项。**`-shared`（linux 和 darwin 上 `-buildmode=pie` 传的就是它）、`-spectre=index`、`-race`、`-msan`、`-asan`、`-cover` 和 `-gcflags` 到不了汇编器，或者只改变汇编器在固定字节之前写的前导。在 linux/amd64 上，`-shared` 让函数开头的栈检查长 2 个字节（见 `quickjs_0_amd64.s`），这会移动函数体，但不移动里面的任何分支。引擎用 `-buildmode=pie` 在 linux/amd64 上构建时通过它的测试。
- **新的Go发布时**，[upgrading.md](../upgrading.zh-CN.md#换用更新的-go) 的步骤包括再用这些选项构建一次。

测试：`TestEngineAsmOptions`（`enginedeps_test.go`）在它们影响的目标上，用 `-asmflags=<engine package>=-spectre=ret` 和 `-dynlink` 构建引擎，期望得到 tripwire 的错误，没有别的错误。它还用 `-shared` 和 `-spectre=index` 构建，这两个必须能成功。`TestEngineBuildmodeShared` 对 linux/amd64 用 `-buildmode=shared` 构建，它不需要 cgo，期望得到同样的结果。

## 每个入口都拒绝别的 VM 的 Value

发布版本：v0.30.2。

`Value` 持有它所在 VM 内存里的一个引用。交给别的 VM 后，那个 VM 会留着一个指向不归它管的内存的指针：`SetPropertyValue` 把它存进对象，另一个 VM 关闭后，这个对象就指向已经解除映射的内存，两个 VM 都开着时，它也可能被错误的分配器释放。`Call`、`Value.Call`、`NewArray` 和 `Builtin` 的结果检查了这一点。`SetPropertyValue`（它的值和接收者）、`SetProperty`、`GetPropertyValue` 和 `GetProperty` 的接收者、`EvalThis` 和 `EvalThisValue` 的 `this`，以及 `RegisterFunc` 的 Go 函数返回的 `Value`（单独返回、在多个结果里、在切片或 map 里），都没有检查。

现在同一个检查 `VM.foreign` 在进入引擎之前守住每个接收 `Value` 的入口：`Value` 要么属于这个 VM，要么不属于任何 VM（`UndefinedValue`、零值 `Value`），其他的一律拒绝，数字也一样，和 `NewArray` 与 `Builtin` 原来的做法一致。方法返回错误，Go 函数在调用它的脚本里抛出 `TypeError`，和 `Builtin` 一样。被拒绝的 `Value` 既不使用也不释放：它仍归原主，由原主释放。所以返回时会释放 `obj` 的 `EvalThis` 和 `EvalThisValue`，对别的 VM 的 `Value` 原样不动。

`Call` 和 `Value.Call` 原来只拒绝别的 VM 里有引用计数的 `Value`，别的 VM 的数字照样传过去。现在也拒绝它。

代价：入口接收的每个 `Value` 多比较一次两个指针。

测试：`TestValueOfAnotherVM`（`othervm_test.go`）把一个 VM 的字符串和对象交给另一个 VM 的这些入口，要求得到错误或 `TypeError`，并且这两个值的计数保持原样。

## 值的构造函数会报告内存不足

发布版本：v0.30.2。

`NewString`、`NewStringLatin1`、`NewStringUTF16` 和 `NewObjectValue` 不看引擎返回了什么。内存不足时，它们把引擎的异常标记包成 `Value` 返回，错误为 nil，并把内存不足的错误留在上下文里挂起，等下一个调用去发现。

现在它们每一个都返回内存不足的错误（设置了 `Uncatchable` 和 `OutOfMemory` 的 `*Error`），并把它从上下文里取出，和其他方法一样。在 javascript 调用的 Go 函数里，它保持挂起，所以调用这个函数的脚本以它结束（[uncatchable.md](uncatchable.zh-CN.md#go-函数里的中断和内存不足)）：`Builtin` 返回 `NewString` 的错误，或者返回它之前给出的 `Value`，结果都一样。检查放在 `newString` 里，每个 Go 字符串去往 javascript 都要经过它，所以 Go 函数的参数和结果也都覆盖到了。

代价：对新值的标签多比较一次。

测试：`TestConstructorsOutOfMemory`（`memory_test.go`）在 1 MiB 的上限下用每个字符串构造函数造一个 2 MiB 的字符串，把一个传给 `Call`，再不断造对象直到碰到上限，要求得到内存不足的错误，并且不留下挂起的异常。

## 转换失败的结果不留下任何东西

发布版本：v0.30.2。

`RegisterFunc` 或 `RegisterHostFunc` 的 Go 函数把它的结果持有的、指向本 VM 的 `Value` 的引用交给这次调用。结果转换到一半失败时（比如遇到一个 channel），它前面那些结果造出的值永远不会被释放，它后面的 `Value` 也一样：channel 前面的一个 4 KiB 字符串会一直留在 VM 里，脚本每接住一次就多 4 KiB 左右，GC 也收不回来。转换拒绝的结果里的 `Value` 同样留下：数组、指向指针的指针、以 `Value` 为键或键的类型 javascript 用不了的 map（有条目时才拒绝，空 map 和以前一样是 `{}`）。宿主函数连同错误一起返回的值里的 `Value` 也留下，转换 panic 时（比如某个 `Error` 方法 panic）则全部留下。

现在调用在转换任何东西之前先给结果拍一份快照，一次遍历走过每个切片、数组、map（它的键和值）、指针和接口（`result.go`）。快照里有结果交出的每个引用，每个只记一次，调用结束时把每个引用释放一次：结果转换成功时，转换失败时，宿主函数返回错误时，以及转换 panic 时。转换只对放进 javascript 值里的 `Value` 做 `Dup`，失败或 panic 时释放自己造出的值，所以两者分开释放。别的 VM 的 `Value` 不释放（见[每个入口都拒绝别的 VM 的 Value](#每个入口都拒绝别的-vm-的-value)）。

遍历在叶子处停下。叶子整体转换，里面的 `Value` 仍归调用方所有：结构体，和以前一样经由 `encoding/json` 转换，以及类型带 `Error` 方法的值，比如带这个方法的 `type E []Value`，和以前一样转换成 `Error` 返回的字符串，值为 nil 时也一样。`String` 方法不改变什么：带这个方法的切片或 map 和别的一样被遍历。

代价：结果类型装不下 `Value` 的 Go 函数没有代价，比如 int、字符串、结构体或 `map[string]string`：注册函数时类型就说明了这一点，调用直接跳过遍历。单独一个 `Value`，或装在 `any` 里的 `Value`，也不用遍历。其余的结果，里面每个值在快照里占一个节点，每个装着 `Value` 的元素、指针目标或 map 再记一项（见[结果两次到达的同一个位置只算一个引用](#结果两次到达的同一个位置只算一个引用)）。遍历 `[]any` 和 `map[string]any` 时用 Go 的 range。一次调用的快照里，节点（里面每个值和每个结果各一个）、元素和属性、引用、位置都不超过 256 个时，它的空间留在 VM 上给下一次调用，所以同样大小的调用在一次同样的调用之后，遍历不分配内存。不是 `map[string]any` 的 map 遍历时要分配两次。在 GCP 机器上和这些改动之前的代码对比（cpu 0，GOMAXPROCS=1，两边轮流各跑 8 轮，取从 javascript 发起的一次调用的中位数）：int、字符串、结构体和 `Value` 用时不变，宿主函数返回的字符串从 735 ns 变成 711 ns。一个数字、一个字符串和一个布尔值的 `[]any` 从 1.45 µs 变成 1.15 µs，一个数字和一个 20 个元素的 `[]int` 组成的 `[]any` 从 4.93 µs 变成 4.07 µs，10 个元素的 `[]Value` 从 2.17 µs 变成 1.75 µs，20 个字符串的 `map[string]any` 从 11.78 µs 变成 4.59 µs，分配从 52 次变成 2 次，因为遍历用 Go 的 range 读这个 map，不再用反射。

测试：`TestResultReferences` 和 `TestResultsReferences`（`result_test.go`）从两种 Go 函数以上面每种形状返回同一个对象：数组、数组的 map、指向指针的指针、作为 map 键的 `Value`，每种也带上宿主函数返回的错误，还有在切片、嵌套切片、map 和宿主函数的错误里 panic 的 `Error` 方法。要求 GC 之后这个对象只剩测试自己持有的两个引用。`TestResultErrorKeepsItsValues` 返回一个带 `Error` 方法的 `[]Value`，要求脚本里得到它的字符串，它的 `Value` 仍归调用方所有。`TestResultReferencesNested` 返回一个结构体，其中 `Value` 的 `toJSON` 会调用另一个返回 `Value` 的 Go 函数，要求两次调用各自释放自己的引用。`TestResultWalkAllocs` 要求返回 40 个和返回 250 个 `Value` 的调用都和返回 2 个的分配一样多。`TestHoldsRefs` 检查遍历会走进哪些类型。`TestResultShapesMatchMain`（`result_shapes_test.go`）把一组不带 `Value` 的结果（nil 和空的切片与 map、nil 指针、nil 接收者的错误类型、自己命名的类型、结构体、装在 any 里的值）单独返回、装在 any 里返回、和一个 `Value` 并排返回，要求 javascript 看到的和改动之前的 main 一样，main 造出两个对象的地方也不会只有一个。`TestFailedResultLeaksNothing`（`register_test.go`）调用几个 Go 函数，它们在一个 channel 前后返回 4 KiB 的字符串和 `Value`，分别作为多个结果、放在切片里、放在嵌套切片里、放在 map 里，或在 channel 之后嵌套在切片、map 和指针里，还有一个以浮点数为键的 `Value` map，每个调用 500 次，每次接住 `TypeError`。要求 GC 之后 VM 的内存增长少于 100 KiB。改动之前增长了 2 到 6 MB。

## UndefinedValue 作为参数和属性值

发布版本：v0.30.2。

`UndefinedValue` 不属于任何 VM，`Value.Call` 的文档就让人把它当作 `this` 传。把它作为 `Call` 和其他调用方法的参数，或作为 `SetPropertyValue` 的值传入时，会因空指针 panic：这些地方用 `Value.Dup` 复制它，而 `Value.Dup` 需要值所在的 VM。现在它们直接复制值本身，这个值没有引用计数，不需要 VM，所以 `UndefinedValue` 和零值 `Value` 在这些地方都是 `undefined`，和 `NewArray` 以及 `Builtin` 的结果一样。`RegisterFunc` 的 Go 函数返回的 `Value`（单独返回或在切片、map 里）也这样读，所以零值 `Value` 在那里同样是 `undefined`，原来是数字 0。它作为 `Value.Call` 和 `EvalThis` 的 `this`、作为属性方法的接收者时也是这样。

对不属于任何 VM 的 `Value` 调用 `Value.Dup` 仍然会 panic，这能抓住 `Free` 之后还在用的 `Value`。

代价：没有。复制仍是同样的两个字的移动。

测试：`TestValueOfNoVM`（`othervm_test.go`）把 `UndefinedValue` 和零值 `Value` 传给 `Call`，作为 `Value.Call` 的 `this`，传给 `SetPropertyValue` 和 `GetProperty`，并从 Go 函数单独或在切片里返回它们，要求脚本里得到 `undefined`。

## 拒绝在对 VM 的调用进行中 Close

发布版本：v0.30.2。

在这个 VM 上的 javascript 调用的 Go 函数里对 VM 调用 `Close`，会释放运行时、解除内存映射、清空 VM 的状态，而引擎此时还在 Go 函数下面运行，用的正是 `Close` 刚交还的栈帧和内存。Go 函数返回后，调用外围的代码读到被清空的状态，以空指针 panic，从运行脚本的 `Eval` 里冒出来。

现在 `Close` 先检查有没有对这个 VM 的调用在进行：没有脚本，没有 javascript 调用的 Go 函数，也没有别的进入引擎的调用。有的话，它返回错误，什么也不改，VM 继续可用。调用返回之后的 `Close` 才关闭它。返回错误符合 API 的规则（[architecture.md](../architecture.zh-CN.md#核心类型)）：`Close` 本来就返回错误（操作系统拒绝归还内存时），有错误可返回的方法不 panic。`OnClose` 的函数里调用的 `Close` 仍然返回 nil，因为那时 `Close` 已经在运行。

代价：`Close` 开始时多比较一次。

测试：`TestCloseInsideACall`（`closetwice_test.go`）在 javascript 调用的 Go 函数里关闭 VM，要求得到错误、脚本的结果、下一次调用正常，以及之后的 `Close` 关闭 VM。

## Then 返回 then getter 抛出的异常

发布版本：v0.30.2。

`Value.Then` 读取 promise 的 `then` 后，不看读取的结果就去调用。`then` 的 getter 抛出异常时，它调用的是异常标记，返回 `TypeError: not a function`，而不是 getter 抛出的东西。现在它返回读取时的异常，和 `GetPropertyValue` 一样。

代价：多比较一次标签。

测试：`TestThenGetterThrows`（`then_test.go`）对一个 `then` getter 会抛 `RangeError` 的 promise 调用 `Then`，要求得到这个错误，并且不留下挂起的异常。

## Go map 变成只有自有数据属性的对象

发布版本：v0.30.2。

Go 函数返回的 Go map 会变成一个对象，键就是 map 的键。原来每个键用 `JS_SetPropertyStr` 设置，它接收 C 字符串，像 `obj[key] = value` 那样赋值。所以键在第一个 NUL 字节处被截断，`"a"` 和 `"a\x00b"` 变成同一个键，丢掉一个值。键 `"__proto__"` 会运行 `Object.prototype` 的 setter，改掉对象的原型，而它本该只是一个键。页面在 `Object.prototype` 上以某个键的名字放的 setter，会拿到 map 的值运行，这个键也就不在对象上。

现在每个键按它的字节和长度做成 atom（`JS_NewAtomLen`），定义成可枚举、可写、可配置的自有数据属性，和 `JSON.parse` 与对象字面量定义键的方式一样（`JS_DefinePropertyValue`），之后释放 atom。所以 map 的每个键都完整地成为对象的键，`__proto__` 也一样，也不运行任何 setter。键仍按排序后的顺序。

代价：每个键从一次进入引擎变成三次。键带着长度交给引擎，不再作为 C 字符串。在 GCP 机器上测量（cpu 0，GOMAXPROCS=1，两边轮流各跑 8 次），javascript 调用一个返回 20 个字符串键 map 的 Go 函数，每次调用从 10.58 µs 变成 8.60 µs（中位数）。map 是 `RegisterFunc` 和 `RegisterHostFunc` 的结果，不是 `Builtin` 的。

测试：`TestMapResultKeys`（`register_test.go`）返回一个带 NUL 字节键的 map、一个键为 `__proto__` 的 map、一个键名会被 `Object.prototype` 的 setter 截获的 map，要求每个键都在对象上，原型不变，setter 不运行。

## 作为 any 返回的 Value 按 Value 接管

发布版本：v0.30.2。

结果类型为 `any` 的 `RegisterFunc` 的 Go 函数，或者 `RegisterHostFunc` 的函数，都可以返回 `Value`。原来只有类型就是 `Value` 的结果，以及切片或 map 里的 `Value` 会被当作 `Value` 读。放在 `any` 里的 `Value` 走 `Value.MarshalJSON` 的 JSON：别的 VM 的 `Value` 会经由那个 VM 读出，`UndefinedValue` 和零值 `Value` 会因空指针 panic，本 VM 的 `Value` 被复制成一个新值，它持有的引用却永远留在 VM 里，每次返回 4 KiB 字符串的调用多留 4 KiB 左右。

现在每个结果的转换先认出 `Value`，不管装着它的是什么类型：本 VM 的 `Value` 把它的引用交给调用，和类型为 `Value` 的结果一样（[转换失败的结果不留下任何东西](#转换失败的结果不留下任何东西)），别的 VM 的 `Value` 抛出 `TypeError` 且不释放，`UndefinedValue` 和零值 `Value` 是 `undefined`。宿主函数连同值一起返回错误时，抛出这个错误，值里本 VM 的 `Value` 被释放。

代价：单独装在 `any` 里的 `Value` 没有代价，调用像接管 `Value` 类型的结果一样接管它。在切片或 map 里的，见[转换失败的结果不留下任何东西](#转换失败的结果不留下任何东西)。

测试：`TestResultValueTaken`（`register_test.go`）从两种 Go 函数把一个 4 KiB 的字符串作为 `any` 返回，并从宿主函数连同错误一起返回，各 500 次，要求脚本里得到这个字符串，GC 之后 VM 的内存增长少于 100 KiB。改动之前增长了 2 MB。`TestValueOfAnotherVM` 和 `TestValueOfNoVM`（`othervm_test.go`）把别的 VM 的 `Value`、`UndefinedValue` 和零值 `Value` 作为 `any` 返回。

## 结果两次到达的同一个位置只算一个引用

发布版本：v0.30.2。

`Value` 里没有任何东西能区分副本和 `Dup`：两者是同样的两个字。所以一个引用就是一个装着本 VM 的 `Value` 的位置：一个结果，切片或数组的一个元素，map 的一个键或值，指针的目标，以及装着它们之一的 `any`。不经 `Dup` 复制到两个位置的 `Value`，是函数并不拥有的两个引用，`Value` 的文档也这样说。转换每遇到一次 `Value` 就接管一个引用，所以结果两次到达的同一个位置，比如以 `[]any{s, s}` 返回的 `Value` 切片 `s`、返回两次的同一个 map，或者同一个数组上互相重叠的两个切片，会在函数只有一个引用的地方交给引擎两个：对象在脚本还在用的时候就被释放了。

现在结果的遍历（见[转换失败的结果不留下任何东西](#转换失败的结果不留下任何东西)）每个位置只走一次：它记下走过的每个元素、指针目标和 `any` 的地址和类型，map 则只按 map 本身记，所以同一个 map 用它类型的哪个名字都是一个位置，比如 `type M map[string]Value` 的 `M(mp)` 和 `mp`。再遇到的位置就用它第一次在快照里做出的那部分。转换每次把 `Value` 放进 javascript 值时都做一次 `Dup`，所以 `[]any{s, s}` 是两个装着同样对象的数组，每个位置的引用只释放一次。放在两个位置的 `Value` 和它的 `Dup` 是两个引用。

代价：见上一节。

测试：`TestResultReferences`（`result_test.go`）返回两次同一个切片、同一个数组上互相重叠的两个切片、两次同一个指针、一个切片和指向它元素的指针、两次同一个 map、分别用类型的两个名字返回的同一个 map 和同一个切片、两个键下放着同一个切片的 map、一个 `Value` 和它的 `Dup`，以及带着宿主函数错误的两次同一个切片。`TestResultsReferences` 把同一个切片和同一个指针作为两个结果返回。`TestResultReferencesRandom` 随机造 2000 个由切片、map 和指针组成的结果，有嵌套，有共用，有的用自己的类型名，有的包含自身，要求 javascript 值和 `encoding/json` 对这个结果的输出一样，或者是环的 `TypeError`。要求 GC 之后对象只剩测试持有的两个引用。

## 包含自身的结果是一个错误

发布版本：v0.30.2。

Go 函数返回的切片、map 或指针包含自身时，比如 `s[0] = s` 的切片 `s`，转换会一直递归，直到 Go 在 1 GB 的栈上限处以 `fatal error: stack overflow` 停下，任何 `recover` 都接不住：进程直接退出。包含自身的类型（比如 `type loop []loop`）也一样。

现在结果的遍历记下它正在里面的切片、map、指针和 `any`，结果再走回其中之一时抛出 `TypeError`，`a result that contains itself`，和 `encoding/json` 拒绝这种值一样。遍历会继续走完其余部分，所以结果的引用都会被释放。同一个切片或 map 在自身旁边（而不是里面）出现两次不算环。包含自身的结构体经由 `encoding/json`，它本来就会拒绝。

代价：见[转换失败的结果不留下任何东西](#转换失败的结果不留下任何东西)。

测试：`TestResultReferences`（`result_test.go`）返回包含自身的切片、map 和指针，一个里面没有 `Value` 的环，一个包含自身的 `type loop []loop` 值，以及带着宿主函数错误的环。`TestResultsReferences` 在两个结果里把环放在一个 `Value` 之后返回，`TestResultReferencesRandom` 则是随机的环。要求抛出 `TypeError`，并且 GC 之后只剩测试持有的两个引用。改动之前，除带错误的那个以外，每个都会让测试进程退出。

## 结果的引用在程序代码运行之前取下

发布版本：v0.30.2。

结果的转换会运行程序的代码：错误的 `Error` 方法、结构体的 `MarshalJSON`，以及结构体里 `Value` 的 `toJSON`，它还能再调用 Go。这些代码可以在转换进行中改动结果。切片变得包含自身时，比如某个 `Error` 执行了 `s[1] = s`，转换会一直递归到进程因栈溢出退出。放进去的别的 VM 的 `Value`，会在不拥有它的 VM 里被改动计数。被拿走的 `Value` 则永远不会被释放。

现在结果的遍历在这些代码运行之前给结果拍一份快照，遍历本身不运行任何程序代码（见[转换失败的结果不留下任何东西](#转换失败的结果不留下任何东西)）：结果交出的每个引用，装得下 `Value` 的切片、map 和指针当时的样子，以及环和别的 VM 的 `Value` 的检查。转换按快照进行，所以程序代码放进去的环或 `Value` 不会被转换，每个引用也只释放一次，不管那些代码把它挪到哪里。不带 `Value` 的数据，比如字符串、`[]int` 或结构体，是快照的叶子：遍历记下的是叶子本身，即它找到的那个字符串或切片，转换时再读叶子指向的内容。所以程序代码之后把结果里切片或 map 的某个位置换成别的叶子，换上的不会被转换，但叶子共用的数据改了，比如 `[]int` 的元素，javascript 值里就是改过的样子，和以前一样。结果类型都装不下 `Value` 的 Go 函数不拍快照，它的结果在转换时读取。

代价：见[转换失败的结果不留下任何东西](#转换失败的结果不留下任何东西)。

测试：`TestResultReferences`（`result_test.go`）返回一个 `Error` 让它包含自身的切片、一个 `Error` 从里面拿走 `Value` 的切片，以及一个 map，其中结构体的 `MarshalJSON` 让这个 map 包含自身。`TestResultChangedDuringConversion` 返回一个 `Error` 往里放别的 VM 的 `Value` 的切片，要求那个位置是 `null`，那个 `Value` 的计数不变。`TestResultExceptionMovesValue` 返回一个装着 `Value` 和空 `*Object` 的切片，空 `*Object` 的 JSON 解析会失败，并把 `SyntaxError` 的 `toString` 换成一个把 `Value` 挪到切片另一个元素的函数，转换读异常消息时会调用它。要求 GC 之后只剩测试持有的两个引用。`TestResultLeafKeptByWalk` 返回一个切片，它的 `Error` 把其中一个字符串换成另一个，要求得到遍历找到的那个字符串。改动之前，两个环会让测试进程退出。

## NewArray 在调用的限制处停下

发布版本：v0.30.2。

在 javascript 调用的 Go 函数里，`VM.NewArray` 现在每 16,384 个元素看一次调用是否该结束，碰到 `Interrupt`、eval 超时或调用里无法捕获的错误就停下，返回这个错误，由 Builtin 返回出去。引擎加一个元素约 50 ns，每个元素都是从 Go 发起的一次调用，期间不读中断标志：4 Mi 个元素的数组在 M 系列 Mac 上要 0.14 到 0.2 秒。造数组期间来的停止要等到造完才生效，还可能超过脚本被杀掉之前的宽限期（[ErrKilled](go-api.zh-CN.md#error-说明调用为什么结束)）：对 2M 个孤立代理项（每个后面跟一个 `i`）调用 `toLocaleUpperCase('tr')` 会造这样的数组，造数组期间来的超时会让它的 VM 被杀掉。少于 16,384 个元素的数组什么也不读，用时和原来一样：在 M 系列 Mac 上，返回 3 个元素数组的 Builtin 改动前后每次调用都是 155 ns。在调用 javascript 之外造的数组，以及在停止之前造完的数组，和原来一样。

测试：`builtin_test.go` 里的 `TestNewArrayInterrupted`（`Interrupt` 之后造 4 Mi 个元素的数组：NewArray 返回中断错误，不返回数组，之后 VM 仍可用；在调用之外，无论是在被中断的调用之后，还是 VM 空闲时调用过 `Interrupt` 之后，都造出整个数组）。

## 长字符串和结果的转换在调用的限制处停下

发布版本：Unreleased。

在 javascript 调用的 Go 函数里，Go 字符串和引擎字符串之间的转换现在每 256 Ki 个字符（UTF-8 则是字节）看一次调用是否该结束，碰到 `Interrupt`、eval 超时或调用里无法捕获的错误就停下，和 [NewArray](#newarray-在调用的限制处停下) 一样。这时 `Args.String` 和 `Args.WTF8` 返回 ok 为 false，`VM.NewString`、`VM.NewStringLatin1` 和 `VM.NewStringUTF16` 返回这个错误，不返回字符串，不管 Go 函数返回什么，调用都以这个错误结束。`RegisterFunc` 的 Go 函数的字符串参数和字符串结果、它返回的 map 的键，以及 `VM.NewAtom`，也一样。这种函数的结果在转换时还每 16,384 个元素或属性看一次，跨嵌套的切片和 map 一起计数，因为每个元素都要调用一次引擎，每次 50 ns 以上。

Go 转换期间不会轮询。在 amd64 服务器上，读一个 16 Mi 个孤立代理项的字符串要 129 ms，64 Mi 个要 0.5 秒，造一个 16 Mi 个中日韩字符的字符串要 151 ms。期间来的停止要等到转换完才生效，还可能超过脚本被杀掉之前 50 ms 的宽限期（[ErrKilled](#error-说明调用为什么结束)）。

长于 1 GiB 的字符串（字节数超过引擎能容纳的字符数）原来由 `NewString` 交给 `JS_NewStringLen`，它解码时不看停止。现在它和别的字符串一样在 Go 里转换，字符数超过引擎能容纳的字符串抛出同样的 `InternalError: string too long`。

一步要多久：一个在每次查看时记时间的测试跑三轮，取最坏的一次，覆盖字符串的每种转换方式：ASCII、Latin-1、中日韩字符、代理对、孤立代理项，以及后面跟着各类字符的非法 UTF-8：

| 步骤 | amd64 服务器（EPYC，t2d） | Apple M4 |
| --- | --- | --- |
| 循环里的一段，16 Mi 和 64 Mi 个字符的字符串 | 最多 1.6 ms | 最多 2.7 ms |
| 分配读取结果那个 Go 字符串的一步，16 Mi 个字符 | 最多 14 ms | 最多 2.7 ms |
| 同上，64 Mi 个字符 | 最多 52 ms | 最多 55 ms |

最慢的一段是非法 UTF-8，每个坏字节变成 U+FFFD。读取返回的 Go 字符串要由 Go 分配，Go 会把它清零，goroutine 还可能要帮 GC 干活，这一步没法切开，字符串越长、GC 当时越忙，它就越久。所以停止恰好在它之前到来时，几千万字符的字符串仍可能超过宽限期，VM 随后被杀掉，和原来一样。这要单个字符串有几千万字符：64 Mi 个字符的字符串在 VM 里按 8 位字符占 64 MiB，按 16 位单元占 128 MiB，内存上限低于这个数的 VM 根本放不下它。数完字节之后、分配之前还会再看一次停止，所以在那之前到来的停止不会分配任何东西。不超过 256 Ki 的字符串什么也不看，转换走原来的代码，只多一次长度比较。量不出差别：在 Apple M4 上，两个构建轮流各跑六次，`BenchmarkBuiltinString` 里用 `Args.String` 和 `Args.WTF8` 读短字符串、再经 `NewString` 送回去的各项，和改动前相差都在 1.7% 以内（每次调用最多 0.8 ns），而其中只跑 javascript、不经过任何改动代码的那一项，在同样两个构建之间就差到 6%。`BenchmarkUTF16String`（一个 200 Ki 单元的页面）相差在 2.1% 以内，一次慢一次快。

仍然不看停止的转换，以及原因：

- `Args.Latin1` 和 `Args.UTF16` 什么也不复制，`Args.Number` 和 `Args.Bool` 只读一个值。
- `Call`、`CallValue` 或 `SetProperty` 的参数如果不是字符串或数字，会先过 `encoding/json`，再由引擎的 JSON 解析器解析，解析器自己会看中断标志。结果里的结构体也是这样。`encoding/json` 是在程序自己的值上运行的 Go 代码。
- 给结果拍快照的遍历（[结果的引用在程序代码运行之前取下](#结果的引用在程序代码运行之前取下)）只有 Go，每个元素几 ns，和 Go 函数造这个结果所花的时间差不多，而且它必须走完，才能释放结果交出的引用。

测试：`stringstop_test.go` 里的 `TestArgStringStops`、`TestNewStringStops` 和 `TestNewStringCharsStops`（每种转换方式，在 `Interrupt` 之后和 eval 超时时：不返回字符串，调用以中断结束而不是被杀，之后 VM 仍能运行脚本；在调用之外得到整个字符串），`TestConversionStopsMidway`（另一个 goroutine 在第一次查看时调用 `Interrupt`，转换就停在那里），`TestArgStringStopsBeforeAllocation`（在分配之前那次查看时停下，什么也不分配），`TestConversionPieces`（每段 1 到 7 个字符或字节，结果和整体转换一样）。`resultstop_test.go` 里的 `TestResultConversionStops` 和 `TestResultConversionStopsMidway`（65,536 个元素的切片、切片的切片、map、`[]any` 和 `map[string]any`），`TestRegisterFuncStringStops`（字符串参数，此时 Go 函数不运行，以及字符串结果），`TestResultLongKeys`（分段造出的键和整体造出的是同一个属性，长键会停下）和 `TestNewStringTooLong`。改动之前，每个关于停止的测试都失败：不看停止，ok 为 true，得到整个字符串。
