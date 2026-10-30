# slowjs

在一个静态 Go 程序里原样运行今天的 npm 代码。

slowjs 渲染 React 比 Node 慢，比其他任何纯 Go JavaScript 引擎都快，不用 cgo，也不用 Node 进程。它是编译成 Go 汇编、为服务端渲染调优过的 QuickJS，为 tifa 而做，tifa 是一个渲染 React 页面的 Go 框架。

给需要在 Go 程序里跑 JavaScript 的场景用，比如在服务端渲染 React。引擎支持 ES2023（模块、异步生成器、Proxy、BigInt），`Intl` 覆盖 Chromium 的全部语言。

它是引擎，不是 Node：没有 `require`、`fs`、`process`。npm 上的代码按浏览器的方式打包后就能原样运行，React 就是这样。

[English](README.md) · 简体中文

## 安装

```sh
go get github.com/liustack/slowjs
```

需要 Go 1.27 或更新。不用 cgo，也不用 C 编译器：`CGO_ENABLED=0` 就能构建，和普通 Go 代码一样交叉编译。引擎的 Go 汇编是照 Go 1.27.2 核对过的。它依赖 Go 怎么开始一个函数、怎么处理出错，所以换更新的 Go 构建之前，先跑一遍测试（见 [Moving to a newer Go](docs/upgrading.md#moving-to-a-newer-go)）。

## 支持的平台

linux/amd64 和 darwin/arm64。在其他平台上构建会马上报错，错误里写明这两个平台。

## 第一个 VM

```go
package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/liustack/slowjs"
	"github.com/liustack/slowjs/intl"
)

func main() {
	vm, err := slowjs.NewVM()
	if err != nil {
		panic(err)
	}
	defer vm.Close()

	vm.SetMemoryLimit(64 << 20) // 这个 VM 最多 64 MiB
	if err := vm.SetEvalTimeout(100 * time.Millisecond); err != nil {
		panic(err)
	}
	if err := intl.Register(vm); err != nil {
		panic(err)
	}

	v, err := vm.Eval(`new Date(0).toLocaleString('zh-CN', {timeZone: 'Asia/Shanghai'})`, slowjs.EvalGlobal)
	fmt.Println(v, err) // 1970/1/1 08:00:00 <nil>

	_, err = vm.Eval(`try { for (;;) {} } catch (e) { 'caught' }`, slowjs.EvalGlobal)
	var e *slowjs.Error
	if errors.As(err, &e) && e.Interrupted {
		fmt.Println("timed out, and no catch saw it")
	}
}
```

一个 VM 不能同时给多个 goroutine 用：每个 goroutine 各用一个，或者建一个池。Intl 和 Temporal 共用内嵌的 tz 2026e，不依赖主机的 zoneinfo。

## 除了 Eval 和 Call，还有什么

API 的文档在 [pkg.go.dev](https://pkg.go.dev/github.com/liustack/slowjs)。除了每个程序都从它开始的 `NewVM`、`VM.Eval`、`VM.Call` 和 `Value`，还有这些：

- **`NewBuiltin(name, length, fn)` 和 `VM.NewBuiltinFunction(b)`**：JavaScript 像调用 `Math.max` 那样调用的 Go 函数，不走反射。`fn` 从 `Args`（`Number`、`String`、`Bool`、`Value`、`This`、`VM`，还有按引擎存放样子给出字符串字符的 `Latin1` 或 `UTF16`）里取参数，不做任何转换，返回一个 `Value`。在 Apple M4 上一次把两个数相加的调用约 55 ns，走 `RegisterHostFunc` 约 470 ns。Builtin 每个进程只建一次：多个 goroutine 上的 VM 会同时调用它。它返回的 `*Error` 按 `Name` 抛成对应的构造器（`Error`、`TypeError`、`RangeError`、`SyntaxError`、`ReferenceError` 或 `InternalError`），其他错误抛成 `TypeError`。[详情](docs/changes/go-api.md#builtin-a-go-function-called-as-a-built-in)
- **`VM.NewArray(vals...)`**：用 Go 里建的值做一个数组，做法和数组字面量一样，页面在 `Array.prototype` 上放的 setter 不会被触发。[详情](docs/changes/go-api.md#newarray)
- **`VM.SetStrip(s)`** 和 `type Strip`：`StripSource` 编译函数时不留源码，函数的 `toString` 显示 `[native code]`，`StripDebug` 不留行号和列号。[详情](docs/changes/go-api.md#setstrip)
- **Go 里建的 `*Error` 的 `Error()`** 是 `Name: Message`，和 `Error.prototype.toString` 的写法一样，以前是空字符串。从 JavaScript 拿到的错误仍是引擎原来的文本。[详情](docs/changes/go-api.md#error-of-an-error-made-in-go)
- **`VM.RunGC()`、`VM.MemorySize()` 和 `VM.GCThreshold()`**：马上跑一次 GC，读 VM 占的内存和下次 GC 的阈值，这样程序可以在两次调用 JavaScript 之间回收，再给下一次设好阈值。[详情](docs/changes/go-api.md#rungc-memorysize-and-gcthreshold)
- **`VM.MemoryUsage()` 和 `VM.ReleaseMemory(keep)`**：按对象、字符串、函数和数组看一个 VM 占了什么，把它留着没用的内存还给系统。[详情](docs/changes/go-api.md#memoryusage-and-releasememory)
- **`VM.RunYoungGC()`**：只回收上次 GC 以后新建的对象。服务端每次渲染后跑一次年轻 GC、隔一阵跑一次完整 GC，每次 GC 花的时间就和一次渲染的垃圾差不多。[详情](docs/changes/young-collection.md)
- **`VM.OnClose(f)`**：VM 关闭时运行 `f`，自己用表按 VM 存东西的包可以在这时删掉对应的项。[详情](docs/changes/go-api.md#onclose)
- **`NewSharedBytecode(bytecode)` 和 `VM.EvalSharedBytecode(b)`**：加载同一份字节码的多个 VM 共用一份函数源码和行号表，tifa 示例商店的每个 VM 少占 2.36 MiB，`toString` 和错误栈不变。[详情](docs/changes/go-api.md#sharedbytecode)
- **`temporal` 包**：`temporal.Register(vm)` 加上 `Temporal`，V8 有的类型和方法都有，由 go-intl 在 Go 里计算，内嵌 ICU 的 tz 2026e 时区数据，从不读主机的 zoneinfo。`Temporal.Now` 读 VM 时钟，`temporal.Clock` 只覆盖 `Temporal.Now` 的时钟。当前程序体积测量见文档。[详情](docs/changes/temporal.md)
- **`*Error` 的 `Uncatchable`、`OutOfMemory` 和 `Interrupted` 字段**：调用为什么结束，脚本伪造不了。[详情](docs/changes/go-api.md#error-says-why-a-call-ended)
- **`VM.SetClock(now)` 和 `VM.Now()`**：设置和读取一个 VM 的墙上时钟，Date、Intl 和 Temporal 都读它。传 nil 恢复实时时钟。[详情](docs/changes/time-and-intl.md#one-wall-clock-per-vm)
- **`VM.SetShapeCache(n)` 和 `DefaultShapeCache`**：一个 VM 留多少个普通对象的 shape（引擎记录一个对象有哪些键、按什么顺序的结构），给下一个键相同的对象用。[详情](docs/changes/go-api.md#setshapecache-and-defaultshapecache)
- **`GetAsyncContext` 和 `SetAsyncContext`**：两个 Builtin，读写一个跟着 promise 任务走的对象：`then` 或 `await` 运行时拿到的是注册时设的那个，所以用它们写的 `AsyncLocalStorage` 在 `await` 之后照样找得到自己的 store，和 Node 一样。从不设置它的 VM 跑 promise 和以前一样。[详情](docs/changes/go-api.md#the-async-context-of-promise-jobs)
- **`VM.RegisterEscapeText(name)`**：在全局对象上定义名为 `name` 的函数，像 React DOM 的字符串渲染器那样给 HTML 转义文本（`&`、`<`、`>`、`"` 和 `'` 换成实体，参数先变成字符串，同 `"" + text`），在引擎里用 C 写成。宿主不调用它，全局对象上就什么都没有。在 tifa 里用了它，示例商店的页面渲染只花原来的 0.974，bench 页面是 0.938。[详情](docs/changes/go-api.md#registerescapetext)

前五项是 v0.26.17 加的。其余的：`*Error` 的字段在 v0.26.7，shape 缓存在 v0.26.16，`VM.OnClose` 和 `temporal` 包在 v0.27.0，`VM.RunYoungGC` 在 v0.27.3，`SharedBytecode` 在 v0.28.1，`VM.SetClock` 和 `VM.Now` 在 v0.28.2，异步上下文在 v0.28.5，`VM.RegisterEscapeText` 还没有发布。

## 用了以后有什么不同

- **限额一到，调用就结束。** 脚本超过执行时限或内存上限就停在那里，不管它怎么 catch：页面里的 `catch`、`finally`、promise 回调和 Go 函数都不会再跑。`*Error` 会说明是超时还是内存。交给 VM 的 Go 函数 panic 了，panic 会回到你自己的 Go 代码，内存耗尽的 VM 也照样能关掉，不漏内存。
- **每台机器上页面都一样。** 每个 VM 里脚本的本地时间都是 UTC，不用给整个进程设 `TZ`。`Intl` 支持 Chromium 有的所有语言，在约 75,000 个表达式上和 Chromium 156 对过结果。
- **字符串完整到达。** 从 JavaScript 拿到的 Go 字符串一定是合法的 UTF-8，带非法字节的 Go 字符串到了 JavaScript 里，读法和 Go 自己解码时一样。
- **C 的速度，不要 C。** 在 [tifa](https://github.com/liustack/tifa)（一个在服务端渲染 React 的 Go 框架）示例商店的 React 页面上，跑同一份 JavaScript，一个页面的耗时是 Bellard 的 C QuickJS 的 0.98 到 1.06 倍（MUI 页面是 0.81），能渲染这些页面的其他纯 Go 引擎要 1.6 到 6.6 倍（见[对比](docs/benchmarks.md#tifa-pages)）。多个 VM 在多个核上也不会等同一把锁。

## 凭什么信它

- **引擎就是上游 QuickJS。** `engine/` 是 Bellard 的 C 代码在一个固定的提交上，加上 slowjs 的改动，每处改动都有记录。LLVM 把它编译成提交进仓库的 Go 汇编，所以构建不需要 C 编译器，而且提交的文件和生成器从 `engine/` 生成的结果不一致时，`make check-gen` 会失败（[怎么核对](docs/testing.md#generate-and-check)）。
- **生成器不放过没核对过的东西。** 遇到它没有规则的指令、调用或栈帧就停下，还会检查测试程序链接后的代码，所以 C 或 Go 本身的改动弄坏了引擎，表现为构建或测试失败，而不是答案错了。
- **每处改动都有记录。** [docs/changes](docs/changes/README.md) 逐项写了为什么安全、内存代价多少、在哪台机器上快了多少，以及对应的测试。
- **测试用上游的，还多加了一层。** test262 的 58 个已知失败和以前逐行相同。另外还有引擎的记录输出、在 AddressSanitizer 和 UndefinedBehaviorSanitizer 下跑的 C 检查，以及 Go API 的测试。
- **有人真在依赖它。** 它是为 [tifa](https://github.com/liustack/tifa) 维护的，tifa 是一个用它渲染 React 页面的 Go 框架，每个版本都先在 tifa 的页面上测过再让 tifa 升级。

## 来自哪里

引擎是 Fabrice Bellard 和 Charlie Gordon 的 [QuickJS](https://bellard.org/quickjs)，用的是 2026-06-04 版。确切版本、怎么合入 QuickJS 的新提交，以及从 modernc.org/quickjs 换过来会遇到什么，见 [docs/upstream.zh-CN.md](docs/upstream.zh-CN.md)。

## 致谢

感谢 Fabrice Bellard 和 Charlie Gordon 写了 QuickJS，感谢 [modernc.org/quickjs](https://gitlab.com/cznic/quickjs) 和 [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs) 的作者 Jan Mercl、Dan Kortschak 等人。他们的工作证明了 QuickJS 可以跑在纯 Go 程序里，也启发了 slowjs。

## 文档

文档是英文的。

| 文档 | 什么时候读 |
| :-- | :-- |
| [Changes](docs/changes/README.md) | 想知道引擎的每处改动为什么安全、代价多少、换来什么 |
| [Performance](docs/performance.md) | 想知道每个版本渲染多快，或者试过哪些办法 |
| [Benchmarks](docs/benchmarks.md) | 想知道这个引擎和其他纯 Go JavaScript 引擎比起来怎样 |
| [更新记录](CHANGELOG.zh-CN.md) | 升级前想知道改了什么 |
| [Architecture](docs/architecture.md) | 准备改代码 |
| [Testing](docs/testing.md) | 准备提交，或者想核对引擎 |
| [Upgrading](docs/upgrading.md) | 要合入上游新版本，或者更新语言数据 |
| [Releasing](docs/releasing.md)、[Commits](docs/commit.md) | 要发版或提交 |
| [上游](docs/upstream.zh-CN.md) | 要从 modernc.org/quickjs 换过来，或者想知道代码从哪来 |

API 文档在 [pkg.go.dev](https://pkg.go.dev/github.com/liustack/slowjs)。

## 许可证

slowjs 新建的文件用 [MIT 许可证](LICENSE-MIT)，其中的源码文件带 SPDX 标识。来自其他项目的代码保留各自的许可证：modernc.org/quickjs 和 modernc.org/libquickjs 留下的部分用 BSD-3-Clause（[LICENSE](LICENSE)、[LICENSE-MODERNC-LIBQUICKJS](LICENSE-MODERNC-LIBQUICKJS)），引擎的 C 代码用 QuickJS 的 MIT 许可证（[LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS)），分配器用 TLSF 的 BSD-3-Clause（[LICENSE-TLSF](LICENSE-TLSF)），小 libc 用 musl 的 MIT 许可证和 [engine/libc/COPYRIGHT](engine/libc/COPYRIGHT) 里的声明。slowjs 对这些文件的改动同时也以 MIT 提供，这些文件仍可只按上游的许可证使用。`intl` 内嵌的语言数据仍用 Unicode License V3 和 [intl/LICENSE-DATA](intl/LICENSE-DATA) 里的其他声明，go-intl 的副本 `internal/gointl` 用 go-intl 的 [Unicode License V3](internal/gointl/LICENSE)，它用到的 ICU 数据，以及 go-intl 从 V8 和 Rust 包移植的代码，另有各自的声明，列在 [NOTICE.md](NOTICE.md#code-and-data-that-go-intl-took-from-other-projects)。

用这个模块构建并分发二进制的程序，要附上这些许可证的声明。哪个路径用哪个许可证、要附哪些文本，见 [NOTICE.md](NOTICE.md)。
