<h1 align="center"><img src="assets/brand/quickjs-banner.png" alt="liustack/quickjs"></h1>

<p align="center"><b>渲染 React 最快的纯 Go JS 引擎。</b></p>

<p align="center">
  <a href="./README.md">English</a> ·
  <a href="#有多快">有多快</a> ·
  <a href="docs/api.zh-CN.md">API</a> ·
  <a href="docs/benchmarks.zh-CN.md">对比</a> ·
  <a href="CHANGELOG.zh-CN.md">更新记录</a>
</p>

<p align="center">
  <a href="https://x.com/liustack"><img src="https://img.shields.io/badge/follow-%40liustack-black?style=flat-square&logo=x&logoColor=white" alt="在 X 上关注 @liustack"></a>
  <a href="https://pkg.go.dev/github.com/liustack/quickjs"><img src="https://img.shields.io/badge/go.dev-reference-007d9c?style=flat-square&logo=go&logoColor=white" alt="Go 文档"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/liustack/quickjs?style=flat-square" alt="Go 版本"></a>
  <img src="https://img.shields.io/badge/CGO-off-success?style=flat-square" alt="不用 CGO">
  <a href="LICENSE-MIT"><img src="https://img.shields.io/badge/license-MIT-blue?style=flat-square" alt="许可证"></a>
  <img src="https://img.shields.io/badge/Not%20backed%20by-Y%20Combinator-FF6600?style=flat-square&logo=ycombinator&logoColor=white" alt="没有 Y Combinator 投资">
  <img src="https://img.shields.io/badge/users-unknown-lightgrey?style=flat-square" alt="用户数未知">
</p>

你的 Go 服务自己渲染 React 页面：同样的代码比 C 版 QuickJS 还快，一个静态二进制，不用 cgo，旁边也不用跑 Node。

给需要在 Go 程序里跑 JavaScript 的场景用，比如在服务端渲染 React。引擎支持 ES2023（模块、异步生成器、Proxy、BigInt），`Intl` 覆盖 Chromium 的全部语言。它是编译成 Go 汇编、为服务端渲染调优过的 QuickJS，为 [tifa](https://github.com/liustack/tifa) 而做，tifa 是一个渲染 React 页面的 Go 框架。

它是引擎，不是 Node：没有 `require`、`fs`、`process`。npm 上的代码按浏览器的方式打包后就能原样运行，React 就是这样。

## 有多快

tifa 几个应用的服务端代码渲染一次的 CPU 时间，同样的 JavaScript，跑在一个核上，以 liustack/quickjs 为 1（越小越快）：

| 引擎 | 纯 Go | React bench 页面 | MUI 页面 | 商店页面 |
| :-- | :-: | --: | --: | --: |
| **liustack/quickjs** | 是 | **1.00** | **1.00** | **1.00** |
| C QuickJS 2026-06-04 | 否，C | 1.32 | 1.30 | 1.04 |
| go-quickjs | 是 | 1.87 | 1.39 | 1.73 |
| go-quickjs 开 JIT | 是 | 2.03 | 1.58 | 2.85 |
| moejs | 是 | 2.29 | 1.51 | 加载不了 |
| modernc.org/quickjs | 是 | 3.97 | 4.00 | 2.56 |
| goant 开 JIT | 是 | 5.04 | 3.97 | 4.03 |

liustack/quickjs v0.30.1，GCP c2d-standard-8（AMD EPYC 7B13），Go 1.27.2。MUI 和商店页面要用 `Intl`，C QuickJS 和大多数其他引擎没有，所以每个引擎都在 FormatJS polyfill 上跑这两组。用它自己的 `Intl`，也就是 tifa 的跑法，liustack/quickjs 的耗时是 C QuickJS 的 0.75、0.34 和 0.72。Node 24 开 JIT 的耗时是 liustack/quickjs 的 0.21 到 0.23，`--jitless` 是 0.60 到 0.69。

它不是每一项都最快的引擎。V8 测试套件的得分（越高越好）：

| 引擎 | 纯 Go | V8 套件得分 |
| :-- | :-: | --: |
| go-quickjs 开 JIT | 是 | 5157 |
| go-quickjs | 是 | 1785 |
| C QuickJS 2026-06-04 | 否，C | 1315 |
| **liustack/quickjs** | 是 | **1268** |
| goant 开 JIT | 是 | 1130 |
| moejs | 是 | 811 |
| modernc.org/quickjs | 是 | 431 |

C QuickJS 是在同一台机器上单独一场跑的，和 liustack/quickjs 轮流跑，那一场 liustack/quickjs 得 1283 分：C 的得分是它的 1.025 倍，RayTrace、Crypto、EarleyBoyer 和 NavierStokes 领先，RegExp、Splay、Richards 和 DeltaBlue 落后。

所有表格、每个引擎的 test262 结果、liustack/quickjs 最慢的几类负载，以及怎么重跑，见 [docs/benchmarks.zh-CN.md](docs/benchmarks.zh-CN.md)。

## 安装

```sh
go get github.com/liustack/quickjs
```

需要 Go 1.26.6 或更新，最高到 Go 1.27。不用 cgo，也不用 C 编译器：`CGO_ENABLED=0` 就能构建，和普通 Go 代码一样交叉编译。引擎的 Go 汇编对 Go 1.26.6 到 1.27.3 的每个补丁版本都核对过，新的补丁版本每周核对一次。它依赖 Go 怎么开始一个函数、怎么处理出错，所以用更新的 Go 构建会直接报错，等 liustack/quickjs 用新版核对过、在补丁版本里放开上限再用。加 `-tags quickjs_anygo` 可以照样构建（见[换用更新的 Go](docs/upgrading.zh-CN.md#换用更新的-go)）。

## 支持的平台

linux 和 darwin 的 amd64 与 arm64，还有 windows/amd64。在其他平台上构建会马上报错，错误里写明支持的平台。

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

	vm.SetMemoryLimit(64 << 20) // 64 MiB for this VM
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

一个 VM 不能同时给多个 goroutine 用：每个 goroutine 一个，或者用池。Intl 和 Temporal 内嵌了 tz 2026e，不需要主机的 zoneinfo。

除了 `Eval` 和 `Call`，API 还有：JavaScript 像调内建函数一样调用的 Go 函数、控制 VM 的 GC 和内存、每个 VM 一个时钟、`Temporal`，以及用 C 写的 React 服务端渲染器的几个部分。清单见 [docs/api.zh-CN.md](docs/api.zh-CN.md)，每个函数的文档在 [pkg.go.dev](https://pkg.go.dev/github.com/liustack/quickjs)。

## 用了以后有什么不同

- **限额一到，调用就结束。** 脚本超过 eval 超时或内存上限就停在那里，不管它怎么 catch：页面里的 `catch`、`finally`、promise 回调和 Go 函数都不会再跑。`*Error` 会说明是超时还是内存。交给 VM 的 Go 函数 panic 了，panic 会回到你自己的 Go 代码，内存耗尽的 VM 也照样能关掉，不漏内存。
- **每台机器上页面都一样。** 每个 VM 里脚本的本地时间都是 UTC，不用给整个进程设 `TZ`。`Intl` 支持 Chromium 有的所有语言，在约 75,000 个表达式上和 Chromium 156 对过结果。
- **字符串完整到达。** 从 JavaScript 拿到的 Go 字符串一定是合法的 UTF-8，带非法字节的 Go 字符串到了 JavaScript 里，读法和 Go 自己解码时一样。
- **比 C 还快，不要 C。** liustack/quickjs 对 QuickJS 的 C 做的改动，补回的比编译成 Go 汇编损失的还多（见[上面的表](#有多快)）。多个 VM 在多个核上也不会等同一把锁。

## 凭什么信它

- **引擎就是上游 QuickJS。** `engine/` 是 Bellard 的 C 代码在一个固定的提交上，加上 liustack/quickjs 的改动，每处改动都有记录。LLVM 把它编译成提交进仓库的 Go 汇编，所以构建不需要 C 编译器，而且提交的文件和生成器从 `engine/` 生成的结果不一致时，`make check-gen` 会失败（[怎么核对](docs/testing.zh-CN.md#生成并检查)）。
- **生成器不放过没核对过的东西。** 遇到它没有规则的指令、调用或栈帧就停下，还会检查测试程序链接后的代码，所以 C 或 Go 本身的改动弄坏了引擎，表现为构建或测试失败，而不是答案错了。
- **每处改动都有记录。** [docs/changes](docs/changes/README.zh-CN.md) 逐项写了为什么安全、内存代价多少、在哪台机器上快了多少，以及对应的测试。
- **测试用上游的，还多加了一层。** test262 有 33 个已知失败，比搬进上游的修正之前少了 25 个，每次跑都是同样这些。另外还有引擎的记录输出、在 AddressSanitizer 和 UndefinedBehaviorSanitizer 下跑的 C 检查，以及 Go API 的测试。
- **有人真在依赖它。** 它是为 [tifa](https://github.com/liustack/tifa) 维护的，tifa 是一个用它渲染 React 页面的 Go 框架，每个版本都先在 tifa 的页面上测过再让 tifa 升级。

## 文档

| 文档 | 什么时候读 |
| :-- | :-- |
| [API](docs/api.zh-CN.md) | 想找 `Eval` 和 `Call` 之外的 Go API |
| [改动](docs/changes/README.zh-CN.md) | 想知道引擎的每处改动为什么安全、代价多少、换来什么 |
| [性能](docs/performance.zh-CN.md) | 想知道每个版本渲染多快，或者试过哪些办法 |
| [对比](docs/benchmarks.zh-CN.md) | 想知道这个引擎和其他纯 Go JavaScript 引擎比起来怎样 |
| [更新记录](CHANGELOG.zh-CN.md) | 升级前想知道改了什么 |
| [架构](docs/architecture.zh-CN.md) | 准备改代码 |
| [测试](docs/testing.zh-CN.md) | 准备提交，或者想核对引擎 |
| [升级](docs/upgrading.zh-CN.md) | 要合入上游 QuickJS 的提交、新的语言数据或更新的 Go |
| [发版](docs/releasing.zh-CN.md)、[提交](docs/commit.zh-CN.md) | 要发版或提交 |
| [上游](docs/upstream.zh-CN.md) | 要从 modernc.org/quickjs 或 github.com/liustack/slowjs 换过来，或者想知道代码从哪来 |
| [历史](docs/history.zh-CN.md) | 想看 v0.29 之前那个引擎（ccgo 把 QuickJS 转译成的 Go）的数字 |

## 来自哪里

引擎是 Fabrice Bellard 和 Charlie Gordon 的 [QuickJS](https://bellard.org/quickjs)，用的是 2026-06-04 版。确切版本、怎么合入 QuickJS 的新提交，以及从 modernc.org/quickjs 换过来会遇到什么，见 [docs/upstream.zh-CN.md](docs/upstream.zh-CN.md)。

感谢 Fabrice Bellard 和 Charlie Gordon 写了 QuickJS，感谢 [modernc.org/quickjs](https://gitlab.com/cznic/quickjs) 和 [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs) 的作者 Jan Mercl、Dan Kortschak 等人。他们的工作证明了 QuickJS 可以跑在纯 Go 程序里，也启发了 liustack/quickjs。

## 社区

随时欢迎提 issue：[新建一个](https://github.com/liustack/quickjs/issues/new/choose)。你拿它跑什么、哪里慢、下一步该做什么，都可以说。推荐你关注公众号 liustack，也欢迎你来 X 上聊：**[@liustack](https://x.com/liustack)**。liustack/quickjs 和 tifa 有什么变化，都会在公众号和 X 同步更新。

关注微信公众号「liustack」：AI 创业机会、独立开发见解、AI 实战与工具，第一时间推送。微信扫码，或搜一搜「liustack」：

<p align="center">
  <img src="https://raw.githubusercontent.com/liustack/quickjs/main/assets/wechat-qrcode.png" width="420" alt="微信公众号 liustack" />
</p>

## 参与贡献

liustack/quickjs 不接受 pull request。每一行都由一个维护者审读、负责到底，这是有意的选择，为的是引擎靠得住。真正有用的参与方式有两种：

- **[提一个 issue](https://github.com/liustack/quickjs/issues/new/choose)。** 算错了，渲染慢，限制没拦住，文档不清楚，都可以提。每个 issue 都会读，它决定下一步做什么。
- **Fork 一份。** 你的那份完全归你，想怎么改、怎么发布都行。

安全问题请按 [SECURITY.zh-CN.md](SECURITY.zh-CN.md) 私下报告。其余见 [CONTRIBUTING.zh-CN.md](CONTRIBUTING.zh-CN.md)。

## 许可证

liustack/quickjs 新建的文件用 [MIT 许可证](LICENSE-MIT)。来自其他项目的代码保留各自的许可证：QuickJS 的 C 代码、modernc.org/quickjs 留下的部分、TLSF 分配器、musl 的 libc 函数，以及 `intl` 和 go-intl 的语言数据。banner 里的 gopher 照 Renée French 画的 Go gopher 画成（CC BY 4.0）。哪条路径用哪个许可证、发布用到本模块的程序要带哪些声明，见 [NOTICE.md](NOTICE.md)。
