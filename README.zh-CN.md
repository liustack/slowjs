# quickjs（liustack fork）

<!-- 待 docs/benchmarks.md 确认范围，只在部分负载上最快时改成那类负载 -->最快的纯 Go JavaScript 引擎，现代 JavaScript 一样不少。

别的纯 Go 引擎总要你放弃点什么：新语法、`Intl`、异步代码，或者速度。这个支持 ES2023（模块、异步生成器、Proxy、BigInt），`Intl` 覆盖 Chromium 的全部语言。

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

## 用了以后有什么不同

- **限额一到，调用就结束。** 脚本超过执行时限或内存上限就停在那里，不管它怎么 catch：页面里的 `catch`、`finally`、promise 回调和 Go 函数都不会再跑。`*Error` 会说明是超时还是内存。交给 VM 的 Go 函数 panic 了，panic 会回到你自己的 Go 代码，内存耗尽的 VM 也照样能关掉，不漏内存。
- **每台机器上页面都一样。** 每个 VM 里脚本的本地时间都是 UTC，不用给整个进程设 `TZ`。`Intl` 支持 Chromium 有的所有语言，在约 60,000 个表达式上和 Chromium 156 对过结果。
- **字符串完整到达。** 从 JavaScript 拿到的 Go 字符串一定是合法的 UTF-8，带非法字节的 Go 字符串到了 JavaScript 里，读法和 Go 自己解码时一样。
- **页面渲染更快。** 在 tifa 示例商店的 React 页面上，引擎渲染速度是起点 modernc.org/quickjs 的 1.63 到 1.66 倍（v0.26.15 时测的），多个 VM 在多个核上也不再等同一把锁。[和其他 Go 引擎的对比](docs/benchmarks.md)。

## 凭什么信它

- **引擎就是上游 QuickJS。** `lib/` 是 ccgo 从 Bellard 的 C 代码生成的，连同历史从上游 libquickjs 合并过来。fork 只通过两种方式改它：`internal/fixlib` 对生成的 Go 做机械改写，以及供重新生成时使用的 C 补丁。提交的文件和 `fixlib` 从上游文件生成的结果逐字节一致（[怎么核对](docs/testing.md#same-as-upstream-after-fixlib)）。
- **上游一动就停。** 每处改写都用指纹或计数核对它改的代码。上游新版本改了那段代码，`fixlib` 就停下并指出位置，不去改它没见过的代码。
- **每处改动都有记录。** [docs/fork-changes](docs/fork-changes/README.md) 逐项写了为什么安全、内存代价多少、在哪台机器上快了多少，以及对应的测试。
- **测试用上游的，还多加了一层。** test262 的 58 个已知失败和上游 libquickjs 逐行相同。另外还有 fork 自己的 300 多个测试，以及在 libc 跟踪每次分配的分配器下跑的泄漏检查。
- **有人真在依赖它。** 它是为 [tifa](https://github.com/liustack/tifa) 维护的，tifa 是一个用它渲染 React 页面的 Go 框架，每个版本都先在 tifa 的页面上测过再让 tifa 升级。

## 来自哪里

这个仓库 fork 了 modernc.org 的两个模块，合并成一个 Go 模块：

| 上游模块 | 上游仓库 | 本仓库位置 |
|---|---|---|
| `modernc.org/quickjs`（Go API） | https://gitlab.com/cznic/quickjs | 仓库根目录，包 `github.com/liustack/quickjs` |
| `modernc.org/libquickjs`（ccgo 转译成 Go 的 QuickJS） | https://gitlab.com/cznic/libquickjs | `lib/`，包 `github.com/liustack/quickjs/lib` |

起点是 quickjs v0.25.0 和 libquickjs v0.13.1（QuickJS 2026-06-04 版，提交 `3d5e064`），改动列在 [docs/fork-changes](docs/fork-changes/README.md)。

## 文档

文档是英文的。

| 文档 | 什么时候读 |
| :-- | :-- |
| [Fork changes](docs/fork-changes/README.md) | 想知道这个 fork 和 modernc.org/quickjs 有什么不同，为什么 |
| [Benchmarks](docs/benchmarks.md) | 在给 Go 选 JavaScript 引擎，想和 goja、sobek、otto、modernc.org/quickjs、fastschema/qjs 比一比 |
| [Performance](docs/performance.md) | 想知道每个版本渲染多快，或者试过哪些办法 |
| [更新记录](CHANGELOG.zh-CN.md) | 升级前想知道改了什么 |
| [Architecture](docs/architecture.md) | 准备改代码 |
| [Testing](docs/testing.md) | 准备提交，或者想拿 fork 和上游对照 |
| [Upgrading](docs/upgrading.md) | 要合入上游新版本，或者更新语言数据 |
| [Releasing](docs/releasing.md)、[Commits](docs/commit.md) | 要发版或提交 |
| [The upstream README](docs/upstream.md) | 想看上游自己对这个包的说明 |

API 文档在 [pkg.go.dev](https://pkg.go.dev/github.com/liustack/quickjs)。

## 上游与许可证

Go API 来自 [modernc.org/quickjs](https://gitlab.com/cznic/quickjs)，引擎来自 [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs)，两者的作者都是 quickjs-go authors（Jan Mercl 等人），引擎由 Fabrice Bellard 和 Charlie Gordon 的 [QuickJS](https://bellard.org/quickjs) 转译而来。引擎和 Go API 放在同一个模块里，这样修引擎可以和 Go API 一起发布，不用等任何一个上游模块发版。

两个项目的许可证文件都保留着：根目录的 [LICENSE](LICENSE) 和 [LICENSE-LIBQUICKJS](LICENSE-LIBQUICKJS)，以及引擎的 [lib/LICENSE](lib/LICENSE)、[lib/LICENSE-LIBQUICKJS](lib/LICENSE-LIBQUICKJS) 和 [lib/LICENSE_TEST262](lib/LICENSE_TEST262)。[intl/LICENSE-DATA](intl/LICENSE-DATA) 是 `intl` 内嵌的语言数据的版权声明，数据来自 Unicode CLDR 和 Chromium 的 ICU 数据：import 了 `intl` 的程序会分发这些数据，必须在文档里或随副本附上这些声明。
