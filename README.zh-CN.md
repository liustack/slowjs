# quickjs（liustack fork）

npm 上的现代 JavaScript 原样跑在一个静态 Go 程序里。

给需要在 Go 程序里跑 JavaScript 的场景用，比如在服务端渲染 React，不用 cgo，也不用 Node 进程。引擎支持 ES2023（模块、异步生成器、Proxy、BigInt），`Intl` 覆盖 Chromium 的全部语言。

它是引擎，不是 Node：没有 `require`、`fs`、`process`。npm 上的代码按浏览器的方式打包后就能原样运行，React 就是这样。

[English](README.md) · 简体中文

## 安装

```sh
go get github.com/liustack/quickjs
```

需要 Go 1.27 或更新。不用 cgo，也不用 C 编译器：`CGO_ENABLED=0` 就能构建，和普通 Go 代码一样交叉编译。

## 第一个 VM

```go
package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/liustack/quickjs"
	"github.com/liustack/quickjs/intl"
)

func main() {
	vm, err := quickjs.NewVM()
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

	v, err := vm.Eval(`new Date(0).toLocaleString('zh-CN', {timeZone: 'Asia/Shanghai'})`, quickjs.EvalGlobal)
	fmt.Println(v, err) // 1970/1/1 08:00:00 <nil>

	_, err = vm.Eval(`try { for (;;) {} } catch (e) { 'caught' }`, quickjs.EvalGlobal)
	var e *quickjs.Error
	if errors.As(err, &e) && e.Interrupted {
		fmt.Println("timed out, and no catch saw it")
	}
}
```

一个 VM 不能同时给多个 goroutine 用：每个 goroutine 各用一个，或者建一个池。机器上没有 zoneinfo 时，要解析时区名就在 main 包里 import `time/tzdata`。

## fork 在 API 上加了什么

API 沿用 modernc.org/quickjs，文档在 [pkg.go.dev](https://pkg.go.dev/github.com/liustack/quickjs)，另外加了这些：

- **`NewBuiltin(name, length, fn)` 和 `VM.NewBuiltinFunction(b)`**：JavaScript 像调用 `Math.max` 那样调用的 Go 函数，不走反射。`fn` 从 `Args`（`Number`、`String`、`Bool`、`Value`、`This`、`VM`）里取参数，不做任何转换，返回一个 `Value`。在 Apple M4 上一次把两个数相加的调用约 55 ns，走 `RegisterHostFunc` 约 470 ns。Builtin 每个进程只建一次：多个 goroutine 上的 VM 会同时调用它。它返回的 `*Error` 按 `Name` 抛成对应的构造器（`Error`、`TypeError`、`RangeError`、`SyntaxError`、`ReferenceError` 或 `InternalError`），其他错误抛成 `TypeError`。[详情](docs/fork-changes/go-api.md#builtin-a-go-function-called-as-a-built-in)
- **`VM.NewArray(vals...)`**：用 Go 里建的值做一个数组，做法和数组字面量一样，页面在 `Array.prototype` 上放的 setter 不会被触发。[详情](docs/fork-changes/go-api.md#newarray)
- **`VM.SetStrip(s)`** 和 `type Strip`：`StripSource` 编译函数时不留源码，函数的 `toString` 显示 `[native code]`，`StripDebug` 不留行号和列号。[详情](docs/fork-changes/go-api.md#setstrip)
- **Go 里建的 `*Error` 的 `Error()`** 是 `Name: Message`，和 `Error.prototype.toString` 的写法一样，以前是空字符串。从 JavaScript 拿到的错误仍是引擎原来的文本。[详情](docs/fork-changes/go-api.md#error-of-an-error-made-in-go)
- **`VM.RunGC()`、`VM.MemorySize()` 和 `VM.GCThreshold()`**：马上跑一次 GC，读 VM 占的内存和下次 GC 的阈值，这样程序可以在两次调用 JavaScript 之间回收，再给下一次设好阈值。[详情](docs/fork-changes/go-api.md#rungc-memorysize-and-gcthreshold)
- **`VM.OnClose(f)`**：VM 关闭时运行 `f`，自己用表按 VM 存东西的包可以在这时删掉对应的项。[详情](docs/fork-changes/go-api.md#onclose)
- **`temporal` 包**：`temporal.Register(vm)` 加上 `Temporal`，V8 有的类型和方法都有，由 go-intl 在 Go 里计算，内嵌 ICU 的 tz 2026e 时区数据，从不读主机的 zoneinfo。`temporal.Clock` 设定 `Temporal.Now` 用的时钟。tifa 的示例商店因此大 0.84 MB。[详情](docs/fork-changes/temporal.md)
- **`*Error` 的 `Uncatchable`、`OutOfMemory` 和 `Interrupted` 字段**：调用为什么结束，脚本伪造不了。[详情](docs/fork-changes/go-api.md#error-says-why-a-call-ended)
- **`VM.SetShapeCache(n)` 和 `DefaultShapeCache`**：一个 VM 留多少个普通对象的 shape（引擎记录一个对象有哪些键、按什么顺序的结构），给下一个键相同的对象用。[详情](docs/fork-changes/go-api.md#setshapecache-and-defaultshapecache)

前五项是 v0.26.17 加的。接下来两项在 `main` 上，还没进任何发布版本。

## 用了以后有什么不同

- **限额一到，调用就结束。** 脚本超过执行时限或内存上限就停在那里，不管它怎么 catch：页面里的 `catch`、`finally`、promise 回调和 Go 函数都不会再跑。`*Error` 会说明是超时还是内存。交给 VM 的 Go 函数 panic 了，panic 会回到你自己的 Go 代码，内存耗尽的 VM 也照样能关掉，不漏内存。
- **每台机器上页面都一样。** 每个 VM 里脚本的本地时间都是 UTC，不用给整个进程设 `TZ`。`Intl` 支持 Chromium 有的所有语言，在约 75,000 个表达式上和 Chromium 156 对过结果。
- **字符串完整到达。** 从 JavaScript 拿到的 Go 字符串一定是合法的 UTF-8，带非法字节的 Go 字符串到了 JavaScript 里，读法和 Go 自己解码时一样。
- **页面渲染更快。** 在 [tifa](https://github.com/liustack/tifa)（一个在服务端渲染 React 的 Go 框架）示例商店的 React 页面上，引擎渲染速度是起点 modernc.org/quickjs 的 1.63 到 1.66 倍（v0.26.15 时测的），多个 VM 在多个核上也不再等同一把锁。

## 凭什么信它

- **引擎就是上游 QuickJS。** `lib/` 是 ccgo 从 Bellard 的 C 代码生成的，连同历史从上游 libquickjs 合并过来。fork 只通过两种方式改它：`internal/fixlib` 对生成的 Go 做机械改写，以及供重新生成时使用的 C 补丁。提交的文件和 `fixlib` 从上游文件生成的结果逐字节一致（[怎么核对](docs/testing.md#same-as-upstream-after-fixlib)）。
- **上游一动就停。** 每处改写都用指纹或计数核对它改的代码。上游新版本改了那段代码，`fixlib` 就停下并指出位置，不去改它没见过的代码。
- **每处改动都有记录。** [docs/fork-changes](docs/fork-changes/README.md) 逐项写了为什么安全、内存代价多少、在哪台机器上快了多少，以及对应的测试。
- **测试用上游的，还多加了一层。** test262 的 58 个已知失败和上游 libquickjs 逐行相同。另外还有 fork 自己的 300 多个测试，以及在 libc 跟踪每次分配的分配器下跑的泄漏检查。
- **有人真在依赖它。** 它是为 [tifa](https://github.com/liustack/tifa) 维护的，tifa 是一个用它渲染 React 页面的 Go 框架，每个版本都先在 tifa 的页面上测过再让 tifa 升级。

## 来自哪里

引擎是 Fabrice Bellard 和 Charlie Gordon 的 [QuickJS](https://bellard.org/quickjs)，用的是 2026-06-04 版。Jan Mercl 用 ccgo 把它转译成 Go，就是 [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs)（这里用 v0.13.1，在 `lib/` 下），quickjs-go 的作者们在它上面写了 Go API，就是 [modernc.org/quickjs](https://gitlab.com/cznic/quickjs)（这里用 v0.25.0，在根目录）。这个 fork 把两者合成一个模块，修引擎可以和 Go API 一起发布，不用等任何一个上游发版。确切版本、fork 保留和改动了什么、从 modernc.org/quickjs 换过来会遇到什么，见 [docs/upstream.zh-CN.md](docs/upstream.zh-CN.md)。

## 文档

文档是英文的。

| 文档 | 什么时候读 |
| :-- | :-- |
| [Fork changes](docs/fork-changes/README.md) | 想知道这个 fork 和 modernc.org/quickjs 有什么不同，为什么 |
| [Performance](docs/performance.md) | 想知道每个版本渲染多快，或者试过哪些办法 |
| [Benchmarks](docs/benchmarks.md) | 想知道这个引擎和其他纯 Go JavaScript 引擎比起来怎样 |
| [更新记录](CHANGELOG.zh-CN.md) | 升级前想知道改了什么 |
| [Architecture](docs/architecture.md) | 准备改代码 |
| [Testing](docs/testing.md) | 准备提交，或者想拿 fork 和上游对照 |
| [Upgrading](docs/upgrading.md) | 要合入上游新版本，或者更新语言数据 |
| [Releasing](docs/releasing.md)、[Commits](docs/commit.md) | 要发版或提交 |
| [上游](docs/upstream.zh-CN.md) | 要从 modernc.org/quickjs 换过来，或者想知道代码从哪来 |

API 文档在 [pkg.go.dev](https://pkg.go.dev/github.com/liustack/quickjs)。

## 许可证

这个 fork 新建的文件用 [MIT 许可证](LICENSE-MIT)，其中的源码文件带 SPDX 标识。有 14 个文件照着引擎的部分代码写，SPDX 标识是 `MIT AND BSD-3-Clause`。来自上游的部分保留各自的许可证：Go API 和 ccgo 转译的引擎（包括 `lib/` 里生成的文件，以及 fork 对它们的改写）用 BSD-3-Clause（[LICENSE](LICENSE)、[lib/LICENSE](lib/LICENSE)）和 QuickJS 的 MIT 许可证（[LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS)）。fork 对这些文件的改动同时也以 MIT 提供，这些文件仍可只按上游的许可证使用。`intl` 内嵌的语言数据仍用 Unicode License V3 和 [intl/LICENSE-DATA](intl/LICENSE-DATA) 里的其他声明，go-intl 的副本 `internal/gointl` 用 go-intl 的 [Unicode License V3](internal/gointl/LICENSE)，它用到的 ICU 数据，以及 go-intl 从 V8 和 Rust 包移植的代码，另有各自的声明，列在 [NOTICE.md](NOTICE.md#code-and-data-that-go-intl-took-from-other-projects)。

用这个模块构建并分发二进制的程序，要附上这些许可证的声明。哪个路径用哪个许可证、要附哪些文本，见 [NOTICE.md](NOTICE.md)。
