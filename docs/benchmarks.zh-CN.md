---
summary: 'liustack/quickjs 和其他能跑当前 JavaScript 的纯 Go JavaScript 引擎比起来怎么样：各自支持什么，在服务端渲染和一般负载上各自多快，liustack/quickjs 在哪里远远落后，各自有什么限制，liustack/quickjs 按 tifa 的方式运行时是什么样，在 tifa 页面上它对 C QuickJS 和 Node 排第几，以及怎样重新运行这个对比'
read_when:
  - 要公开说这个引擎和其他 Go 引擎、C QuickJS 或 Node 比起来怎么样
  - 要为一项工作选纯 Go 的 JavaScript 引擎
  - 想找这个引擎最慢的负载，好把它变快
  - 发布小版本，或者所有者点名的补丁版本（重新运行对比并更新这一页）
  - 改 compare/engines
---

# 纯 Go JavaScript 引擎对比

[English](benchmarks.md) · 简体中文

2026-11-06 的一次运行：liustack/quickjs v0.30.1 对另外五个能用 `CGO_ENABLED=0` 构建的引擎，其中两个跑了两次（带 JIT 和不带 JIT）。
GCP c2d-standard-8（AMD EPYC 7B13），Linux 7.3，Go 1.27.2，每个引擎进程固定在一个 CPU 上，它的超线程同伴核空闲，GOMAXPROCS=1。完整的表在
[`compare/engines/results/2026-11-06-v0.30.1-gcp.md`](../compare/engines/results/2026-11-06-v0.30.1-gcp.md)，
原始数字在它旁边的 `.json`。liustack/quickjs 的数字来自 `main` 上 `5e99b850` 的引擎代码，这是 v0.30.1 所打标签的合并提交。这里的每张表都来自这次运行。go-quickjs 和 modernc.org/quickjs 都是最新版本（go-quickjs v0.34.1，之前是 v0.28.0，modernc.org/quickjs v0.27.0，之前是 v0.25.0）。goant、moejs 和参照行用的是 2026-10-29 那次运行的版本。更早的运行见[历史](#历史)。

liustack/quickjs 的行叫 `liustack-quickjs` 和 `liustack-tifa`。模块还叫 `github.com/liustack/slowjs` 时生成的结果文件（2026-10-24 的 v0.28.6 PGO 页面运行和 2026-10-29 的运行）把它们叫 `slowjs` 和 `slowjs-tifa`。v0.28.6 的页面运行还有一行 `liustack-tifa-nogoasm` 或 `slowjs-tifa-nogoasm`，C 引擎不需要它。

## 概要

- **支持。** go-quickjs 和 goant 通过 99.3% 的 test262（language、built-ins 和 intl402），包括 Temporal 和 Intl。liustack/quickjs
  通过 83.1%：它的 Intl 有全部构造函数，但对比时不带 Temporal 运行它（没有注册包 `temporal`），而 Temporal 占 13426 次运行。moejs 通过 81.3%。在
  ES2023 及更早的部分，每个引擎都通过 99.1% 到 99.3%。liustack/quickjs 不再有模块加载器：`import()` 和静态 `import` 的探测都失败。
- **服务端渲染。** 在真实 tifa 应用的服务端 bundle 上，
  liustack/quickjs 在每组页面上需要的 CPU 都是纯 Go 引擎里最少的（见
  [tifa 页面](#tifa-页面)）。排第二的 go-quickjs（不带 JIT）在 bench 页面上需要 1.9 倍，在商店上 1.8 倍，在 MUI 页面上 1.4
  倍，都是对着普通的 liustack-quickjs 行。在速度表的 React 页面上，
  liustack/quickjs 需要 8.24 ms CPU，带 JIT 的 go-quickjs 需要 8.38 ms
  （1.02 倍），它的墙钟时间更短（7.08 ms 对 8.18 ms）。
  不带 JIT 的 go-quickjs 需要 1.5 倍的 CPU，moejs 1.7 倍。
- **对 C QuickJS。** 同样的 JavaScript，同一个核，liustack/quickjs
  在 bench 页面（不需要 Intl）上用的时间是 C QuickJS 2026-06-04 的 0.76 倍，在 MUI 页面上 0.77 倍，在商店上 0.96 倍（后两者都跑在 FormatJS polyfill 上，因为 C QuickJS 没有 Intl）。用它自己的 Intl，商店用的时间是 C 在 polyfill 上所需的 0.73，MUI 页面是 0.49。
- **对 Node。** 在同样的 JavaScript 上，带 JIT 的 Node 比
  liustack/quickjs 快 4.8、4.8 和 4.3 倍（bench、MUI、商店），带
  `--jitless` 的 Node 快 1.4、1.6 和 1.7 倍（见 [tifa 页面](#tifa-页面)）。
- **按 tifa 的方式运行。** tifa 页面也用按 tifa 的设置配置好的 liustack/quickjs 跑：它自己的 Intl 和 Temporal，没有 polyfill，用 tifa 的 Go
  函数和 tifa 的 GC 规则（见
  [liustack/quickjs 按 tifa 的方式运行](#liustackquickjs-按-tifa-的方式运行)）。在 Go 这一侧，
  这是唯一一行带有 C QuickJS 行和 Node 行不可能有的功能。
  它用的时间在 bench 页面上是 C QuickJS 的 0.75，商店 0.72，MUI 页面 0.34。
- **一般速度。** liustack/quickjs 不是最快的纯 Go 引擎。
  在 V8 套件上，七行里它排第三：带 JIT 的 go-quickjs
  得分是它的 4.1 倍，不带 JIT 的 go-quickjs（默认）是 1.4 倍，
  带 JIT 的 goant 和 moejs 得分比它低。go-quickjs 跑正则表达式，不带 JIT 快 2.9 倍，带 JIT 快 5.2
  倍，在 63 个 QuickJS 微基准里有 32 个更快（JIT：51 个）。moejs 写 JSON 快 2.0 倍，go-quickjs 快 1.3
  倍。goant 解析 JSON 快 1.1 倍。
- **远远落后。** 没有一项达到其他纯 Go 引擎中位数的两倍。
  最远的是空循环（1.71）、数字运算（1.52 到 1.59）和数组上的循环（1.44）：见
  [liustack/quickjs 远远落后的地方](#liustackquickjs-远远落后的地方)。
- **liustack/quickjs 领先的地方。** tifa 页面，每个 VM 占的内存少
  （RSS 1.3 MiB，modernc 1.1 MiB，moejs 1.4 MiB，go-quickjs 2.2 MiB），还有
  一个按存活内存计数、恰好在限制处停下脚本的内存限制，
  只有 modernc 也有。在 React 页面上它比 modernc.org/quickjs（QuickJS 到 Go 的 C 翻译）快 3.6 倍，
  构建字符串比下一个引擎快 1.7 倍，解析 JSON 比
  除 goant 以外的每个引擎都快。
  新建一个 VM 比其他引擎花得多：0.36 ms 对 go-quickjs 的 0.23 ms，
  加载一个编译好的程序要 2.4 ms 对 1.2 ms。

## 引擎

| 引擎 | 版本 | 它是什么 | 对比怎么用它 |
|---|---|---|---|
| liustack-quickjs | v0.30.1（main `5e99b850`） | Bellard 的 QuickJS（固定的提交）的 C 代码，由 LLVM 编译成 Go 汇编，带 liustack/quickjs 的改动 | 字节码编译一次（`VM.Compile`），用 `EvalBytecode` 运行，Intl 来自包 `intl` |
| liustack-tifa | v0.30.1（main `5e99b850`） | 同一个引擎，按 tifa 设置 VM 的方式配置，只用于 tifa 页面 | 包 `intl` 和 `temporal`，tifa 的 Go 函数，tifa 的限制和 GC 规则，共用一个编译好的 bundle（见 [liustack/quickjs 按 tifa 的方式运行](#liustackquickjs-按-tifa-的方式运行)） |
| modernc-quickjs | v0.27.0 | 由 ccgo 把 C 翻译成 Go 的 QuickJS | 同上 |
| go-quickjs-interp | v0.34.1 | 用 Go 重写的 QuickJS-NG | `quickjs.Compile` 一次，`RunProgram`，JIT 关闭，这是默认 |
| go-quickjs-jit | v0.34.1 | 同一个，带 `-tags quickjs_jit` 和 `WithJIT()` | 同上，JIT 标为实验性 |
| goant-interp | v0.2.1 | 用 Go 写的 JavaScript 引擎 | 每个运行时 `Runtime.Compile` 和 `RunProgram`，JIT 关闭，这是默认 |
| goant-jit | v0.2.1 | 同一个，带 `WithJIT(true)` | 同上 |
| moejs | v0.1.0-alpha.11 | 用 Go 写的寄存器字节码引擎，为插件沙箱而做 | `moejs.CompileScript` 一次，在每个运行时上 `RunScript`，运行时带自己的可变内建对象（`MutableIntrinsics`），因为 test262、polyfill 和 tifa 的 bundle 都会写它们 |


goja、sobek 和 otto 没有包括在内：它们缺 async generator、WeakRef、较新的正则表达式标志等等（otto 只实现 ES5）。jsc2go
和 fastschema/qjs 在对比的第一版里出现过，现在不再运行。它们的原始结果没有保留（`compare/engines/CHANGELOG.zh-CN.md`）。

## 支持情况

| 引擎 | test262 | ES2023 及更早 | language | built-ins | intl402 | Temporal | 特性探测（77） | Intl 探测（13） |
|---|---|---|---|---|---|---|---|---|
| goant-interp、goant-jit | 99.3% | 99.3% | 98.7% | 99.9% | 99.6% | 有 | 73 | 10 |
| go-quickjs-interp、go-quickjs-jit | 99.3% | 99.3% | 98.6% | 99.8% | 100.0% | 有 | 77 | 13 |
| liustack-quickjs | 83.1% | 99.2% | 97.3% | 76.5% | 36.3% | 未注册 | 73 | 13 |
| moejs | 81.3% | 99.1% | 97.3% | 74.4% | 24.5% | 无 | 68 | 9 |
| modernc-quickjs | 81.2% | 99.2% | 97.3% | 76.4% | 8.9% | 无 | 63 | 3 |

test262 在 `d53ebc8b`，每个引擎 96540 次运行（sloppy 和 strict），每个引擎都不含模块测试和需要宿主钩子的测试。「built-ins」
算上了 Temporal，它占 13268 次运行。特性探测和 Intl 探测：
答案已知的小脚本，Intl 对着 Node 24 比较。同样的代码跑两次，有几个测试会变（v0.26.17 那次运行里 modernc-quickjs 少 4 个，
goant-interp 少 9 个）。liustack/quickjs 通过 80231 次运行，
比 2026-10-29 的运行多 16 次。modernc.org/quickjs v0.27.0 比 v0.25.0 多通过 12 次，
goant 多 2 次。

liustack/quickjs 从 v0.27.0 起就有 Temporal，在包 `temporal` 里，但
对比只注册了包 `intl`，所以 Temporal 和 Temporal 的 intl402
测试（6742 次 intl402 运行里的 4058 次）都失败。除去它们，
intl402 通过 2684 次里的 2448 次（91.2%）。它的 Intl 有全部构造函数，
包括 Collator（因此有按语言区域的 `localeCompare`）、Locale、DurationFormat、
ListFormat、RelativeTimeFormat、Segmenter 和 DisplayNames，
13 个 Intl 探测全部通过。按 test262 的特性标签，它还缺：
显式资源管理（`using`，820 个里通过 128 个）、`Array.fromAsync`、
JSON 模块的导入属性、ShadowRealm、`Atomics.waitAsync` 和最新的
提案（`import defer`、迭代器分块与拼接、不可变 ArrayBuffer）。C 引擎没有模块加载器：`import()`
和静态 `import` 的探测失败，JSON 模块和尾调用的探测也失败。
moejs 在这些探测上失败：`Intl.Collator`、`Intl.RelativeTimeFormat`、
`Intl.ListFormat`、`Intl.DisplayNames`、`FinalizationRegistry`、迭代器
辅助函数、`RegExp.escape`、JSON 模块和尾调用。

代码高亮是各引擎分歧最大的地方。shiki 4.5.0 把 bash、css、go、html、json、typescript 和 yaml 的
TextMate 语法转成 552 个正则表达式，其中许多带 `v` 标志、后行断言和具名分组（它们在
`engine/test/compare/recorded/shiki`，并且对着 Node 核对过）。每五个里取一个，让 111 个正则表达式在它们所对应的代码上运行，liustack/quickjs、go-quickjs v0.28.0 和 moejs
v0.1.0-alpha.9 对全部 111 个给出和 Node 一样的结果（这一页没有重新跑，moejs 现在是 alpha.11）。其他引擎对其中一些会抛错或
匹配结果不同：`19d6bd86` 的 QuickJS 31 个，`3d5e064e` 的 QuickJS（liustack/quickjs 的 C 引擎来自的那个提交）37 个，`7a23d8e3` 的 quickjs-ng
14 个，modernc.org/quickjs v0.25.0 37 个，goant v0.2.1 31 个。更新的
go-quickjs v0.34.1 和 modernc.org/quickjs v0.27.0 没有试。在
这些引擎上，用 shiki 高亮代码的页面会失败或输出不对。

## 速度

每次操作的 CPU 时间（用户态加系统态，所有线程），单位毫秒，
5 轮的中位数。越低越好，V8 套件得分除外。括号里：
对 liustack/quickjs 的比值。

| 引擎 | React SSR 渲染 | V8 套件得分 | JSON.parse | JSON.stringify | 字符串构建 | RegExp | QuickJS 微基准（时间几何平均） |
|---|---|---|---|---|---|---|---|
| liustack-quickjs | 8.24 | 1268 | 1.58 | 2.78 | 2.90 | 29.9 | 1.00 |
| modernc-quickjs | 29.3 (3.56) | 431 (0.34) | 6.31 (3.99) | 10.1 (3.63) | 11.8 (4.07) | 83.3 (2.79) | 3.41 |
| go-quickjs-interp | 12.1 (1.47) | 1785 (1.41) | 2.04 (1.29) | 2.12 (0.76) | 6.30 (2.17) | 10.5 (0.35) | 1.01 |
| go-quickjs-jit | 8.38 (1.02) | 5157 (4.07) | 2.05 (1.30) | 2.09 (0.75) | 5.10 (1.76) | 5.73 (0.19) | 0.43 |
| goant-interp | 34.6 (4.20) | 348 (0.27) | 1.42 (0.90) | 2.71 (0.97) | 12.5 (4.31) | 61.7 (2.06) | 6.09 |
| goant-jit | 28.0 (3.40) | 1130 (0.89) | 1.45 (0.92) | 2.71 (0.97) | 10.2 (3.52) | 61.1 (2.04) | 2.09 |
| moejs | 13.8 (1.67) | 811 (0.64) | 1.91 (1.21) | 1.35 (0.49) | 4.87 (1.68) | 40.0 (1.34) | 1.56 |

墙钟时间说的是同一回事，只有一处不同：go-quickjs 渲染的中位数
墙钟时间比它的 CPU 时间短，因为它的垃圾回收器在两次操作之间也在工作。在 React 页面上它的中位数是
9.86 ms（JIT：7.08 ms），liustack/quickjs 是 8.18 ms，所以带 JIT 时
它是这个页面上唯一墙钟时间更短的引擎。忙碌的服务器付的是 CPU 时间。

冷启动和内存，用 React bundle（117 KB）：

| 引擎 | 新建 VM，ms | 新建 VM + 从源码加载 | 新建 VM + 加载编译好的程序 | 第一次渲染 | 每个 VM 的 RSS，MiB | 每个 VM 的 Go 堆，MiB |
|---|---|---|---|---|---|---|
| liustack-quickjs | 0.358 | 14.8 | 2.38 | 8.42 | 1.3 | 0 |
| modernc-quickjs | 0.291 | 17.0 | 2.27 | 29.1 | 1.1 | 0 |
| go-quickjs-interp | 0.227 | 10.1 | 1.16 | 18.7 | 2.2 | 5.7 |
| go-quickjs-jit | 0.302 | 8.82 | 1.44 | 76.8 | 11.1 | 11.6 |
| goant-interp | 0.643 | 17.4 | n/a | 39.9 | 8.3 | 7.1 |
| goant-jit | 0.617 | 16.5 | n/a | 42.3 | 8.8 | 7.2 |
| moejs | 0.096 | 17.1 | 0.561 | 14.6 | 1.4 | 1.0 |

内存：100 个 VM 同时存活时，每个 VM 让进程增长的量，每个 VM 都加载了
bundle 并渲染了一个页面。goant 的 Program 只服务编译它的
那个运行时（`goant.go` 205 到 208），所以它从源码加载。moejs
给每个运行时配自己的可变内建对象，它的指南说这大约要多花
25 倍，默认情况是冻结的内建对象由所有运行时共用。

## tifa 页面

HTML 是否相同在运行时检查并存档：`cmd/pages --output`
写出每个页面的 SHA256 摘要（遮掉时间戳之后），以及每个页面是否和 liustack-quickjs 的页面相同的结论（JSON 里的 `same_as_liustack`），
包括单独的 polyfill 和实际部署的对比。缺少 HTML 文件的页面
结论是 null，不算匹配。JSON 里还保留原始进程结果、补跑
的运行、单页探测和元数据文件里的每次尝试。

三个 tifa 应用的服务端 bundle（`tifa/bundles`），按 tifa
渲染它们的方式渲染，数据是每个应用的服务端为每个页面发送的数据。时间
是每次渲染的 CPU（bench 页面是热身 50 次之后 200 次计时渲染的平均，商店和 MUI 是热身 20 次之后 100 次，一个进程里一页接一页），
表示成对 liustack/quickjs 的比值：各页面的几何平均，范围在
括号里。C QuickJS 和 Node 的行是参照，不是纯 Go 引擎。
C QuickJS（`qjs`，用它自己的 Makefile 构建）在脚本里用
墙钟时间计时，独占一个 CPU。Node 也在脚本里计时，在同样的
热身渲染之后，带 JIT（到那时引擎已经编译了热点代码）
和带 `--jitless` 两种。每个页面的 HTML 都和
liustack-quickjs 的页面比较，路由器的渲染时间戳（`u:` 加 13
位数字）会先遮掉。每个引擎的每个页面：
[`compare/engines/results/2026-11-06-v0.30.1-gcp-pages.md`](../compare/engines/results/2026-11-06-v0.30.1-gcp-pages.md)。

在这些页面上，liustack/quickjs 是 SSR 最快的纯 Go JS 引擎：
其他每个纯 Go 引擎渲染一个页面都需要更多 CPU，或者渲染不了。

- 不带 Intl 的页面：go-quickjs-interp 多需要 87%，go-quickjs-jit 需要 2.0
  倍，moejs 2.3 倍，其他的 4.0 到 5.8 倍。
- 商店，按每个引擎会怎么部署：go-quickjs-interp 多需要 75%，
  go-quickjs-jit 2.6 倍（按页面 1.9 到 5.7 倍），其他的 3.4
  到 5.4 倍。moejs 加载不了商店的 bundle（它的函数
  限制在 256 个寄存器）。
- MUI 页面按实际部署：go-quickjs-interp 多需要 41%，go-quickjs-jit
  多 70%，moejs 2.4 倍，其他的 6.2 到 7.0 倍。

对着参照，低于 1 表示比 C QuickJS 2026-06-04 在
同样的页面上更快（一个 bundle 各页面的几何平均，范围在括号里）。
Bench 不需要 Intl，MUI 和商店需要，所以除了
liustack-tifa 行和「只用自己的 Intl」行之外，每一行都在 polyfill 上跑它们。

| 行 | bench（2 个页面） | MUI（3 个页面） | 商店（20 个页面） |
|---|---|---|---|
| liustack-quickjs，同样的 JavaScript（polyfill） | 0.76 (0.71-0.81) | 0.77 (0.69-0.89) | 0.96 (0.89-1.00) |
| liustack-quickjs，只用自己的 Intl（没有 Temporal，没有 Go 函数，没有 tifa 的 GC 规则） | 0.76 (0.71-0.81) | 0.49 (0.36-0.82) | 0.73 (0.65-0.97) |
| **liustack-tifa，按 tifa 的方式运行** | **0.75 (0.69-0.81)** | **0.34 (0.25-0.57)** | **0.72 (0.63-0.87)** |
| C QuickJS 2026-06-04 | 1.00 | 1.00 | 1.00 |
| Node v24.11.1 | 0.16 (0.12-0.21) | 0.16 (0.14-0.20) | 0.22 (0.18-0.28) |
| Node v24.11.1 `--jitless` | 0.52 (0.50-0.55) | 0.48 (0.42-0.58) | 0.58 (0.54-0.61) |

- **对 C。** 在同样的 JavaScript 上，liustack/quickjs 在 bench 页面上比
  C QuickJS 快 24%（0.76），MUI 快 23%（0.77），商店快 4%
  （0.96）。它比自己所编译自的 C 还快：
  liustack/quickjs 对 C 的改动（见 [performance.md](performance.zh-CN.md)）
  补回的比相对原生代码的损失还多。
- **对 Node。** 在同样的 JavaScript 上，带 JIT 的 Node 在 bench 页面上比 liustack/quickjs 快 4.8
  倍，MUI 4.8 倍，商店 4.3 倍。带 `--jitless` 的 Node 快 1.4、1.6 和 1.7 倍。双方各用
  自己的 Intl 时，Node 在 MUI 和商店页面上用的时间是 liustack/quickjs 的 0.22 和 0.24（带 JIT），
  不带 JIT 是 0.68 和 0.61（见下面「按实际部署」的表）。
- `7d1c13c5d8` 的 C QuickJS master 在 bench 页面上用的时间是 liustack/quickjs 的 1.30 倍，
  2026-06-04 的发布版是 1.32。QuickJS-NG v0.17.0 是 1.73
  倍。

### 哪些页面需要 Intl

删掉 `Intl`，并把 `localeCompare` 和 `toLocale*` 方法换成会抛错的桩，
只有 bench 应用的两个页面保持不变：

| 应用 | 页面 | 没有 Intl 时 HTML 相同 | 最先用到 Intl 的地方 |
|---|---|---|---|
| bench | table、spiral | 是，两个都是 | 没有 |
| mui | home、order、orders | 否 | 应用自己的货币 `Intl.NumberFormat('en-US', {style: 'currency'})` 和 `Intl.DateTimeFormat`，home 还有 `Number.prototype.toLocaleString('en-US')`，MUI 的 TablePagination 会检查 Intl |
| shop | 全部 20 个 | bundle 加载不了 | bundle 顶层的 `new Intl.NumberFormat('zh-CN', {style: 'currency', currency: 'CNY'})`，然后是 i18next（`Intl.getCanonicalLocales`、`Intl.PluralRules`）和各页面带时区的 `Intl.DateTimeFormat` |

所以只有 liustack/quickjs、go-quickjs 和 Node 能原样渲染商店和 MUI
页面。为了让所有引擎在商店上可比，它又渲染了两次：
一次是在 bundle 之前加载 FormatJS polyfill（固定在 `tifa/polyfill`，zh-CN 和
en-US 的数据加上时区数据，2.4 MB），一次是按每个
引擎会怎么部署。

### 不带 Intl 的页面


| 引擎 | 加载 ms | 能渲染的页面 | 和 liustack-quickjs 页面相同 | 每次渲染的时间，对 liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs | 62 | 2 of 2 | 2 of 2 | 1.00 (1.00-1.00) |
| liustack-tifa | 65 | 2 of 2 | 2 of 2 | 0.99 (0.98-1.00) |
| modernc-quickjs | 73 | 2 of 2 | 2 of 2 | 3.97 (3.94-3.99) |
| go-quickjs-interp | 64 | 2 of 2 | 2 of 2 | 1.87 (1.86-1.89) |
| go-quickjs-jit | 63 | 2 of 2 | 2 of 2 | 2.03 (2.03-2.03) |
| goant-interp | 72 | 2 of 2 | 2 of 2 | 5.80 (5.47-6.15) |
| goant-jit | 74 | 2 of 2 | 2 of 2 | 5.04 (4.64-5.47) |
| moejs | 92 | 2 of 2 | 2 of 2 | 2.29 (2.24-2.35) |
| C QuickJS 2026-06-04（参照） | 40 | 2 of 2 | 2 of 2 | 1.32 (1.23-1.42) |
| C QuickJS master 7d1c13c5d8（参照） | 44 | 2 of 2 | 2 of 2 | 1.30 (1.21-1.39) |
| QuickJS-NG v0.17.0（参照） | 35 | 2 of 2 | 2 of 2 | 1.73 (1.62-1.83) |
| Node v24.11.1（参照） | 17 | 2 of 2 | 2 of 2 | 0.21 (0.17-0.26) |
| Node v24.11.1 --jitless（参照） | 17 | 2 of 2 | 2 of 2 | 0.69 (0.68-0.70) |

| 页面 | liustack-quickjs | liustack-tifa | modernc-quickjs | go-quickjs-interp | go-quickjs-jit | goant-interp | goant-jit | moejs | quickjs-2026-06-04 | quickjs-master | quickjs-ng | node-jit | node-jitless |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| spiral | 16.4 (1.00) | 16.4 (1.00) | 65.5 (3.99) | 30.5 (1.86) | 33.2 (2.03) | 101 (6.15) | 89.7 (5.47) | 38.6 (2.35) | 20.2 (1.23) | 19.9 (1.21) | 26.6 (1.62) | 4.23 (0.26) | 11.1 (0.68) |
| table | 14.3 (1.00) | 14 (0.98) | 56.2 (3.94) | 26.9 (1.89) | 28.9 (2.03) | 78.1 (5.47) | 66.2 (4.64) | 31.9 (2.24) | 20.2 (1.42) | 19.8 (1.39) | 26.1 (1.83) | 2.43 (0.17) | 10 (0.70) |


C QuickJS 的行显示了生成的 Go 汇编离原生代码有多近：
在这些页面上 liustack/quickjs 用的时间是它所编译自的 C 的 0.76 倍，
在用 polyfill 的商店上是 0.96 倍。

### 商店在 FormatJS polyfill 上，所有引擎

每个引擎先删掉原生 `Intl`，包括有原生 Intl 的那三个，
这样大家跑的是同样的 JavaScript。加载 ms：先是 polyfill，然后是 bundle。


| 引擎 | 加载 ms | 能渲染的页面 | 和 liustack-quickjs 页面相同 | 每次渲染的时间，对 liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs | 573 | 20 of 20 | 20 of 20 | 1.00 (1.00-1.00) |
| liustack-tifa | 没有运行：这一行没有 polyfill | | | |
| modernc-quickjs | 913 | 20 of 20 | 20 of 20 | 2.56 (2.39-2.78) |
| go-quickjs-interp | 485 | 20 of 20 | 20 of 20 | 1.73 (1.42-1.97) |
| go-quickjs-jit | 516 | 20 of 20 | 20 of 20 | 2.85 (1.93-5.00) |
| goant-interp | 1063 | 20 of 20 | 20 of 20 | 4.11 (3.81-4.47) |
| goant-jit | 1073 | 20 of 20 | 20 of 20 | 4.03 (3.36-4.41) |
| moejs | load: SyntaxError: function needs more than 256 registers is not supported yet | 0 of 20 | | |
| C QuickJS 2026-06-04（参照） | 383 | 20 of 20 | 20 of 20 | 1.04 (1.00-1.13) |
| C QuickJS master 7d1c13c5d8（参照） | 384 | 20 of 20 | 20 of 20 | 1.04 (0.99-1.12) |
| QuickJS-NG v0.17.0（参照） | 355 | 20 of 20 | 18 of 20 | 1.29 (1.20-1.39) |
| Node v24.11.1（参照） | 211 | 20 of 20 | 20 of 20 | 0.23 (0.19-0.30) |
| Node v24.11.1 --jitless（参照） | 205 | 20 of 20 | 20 of 20 | 0.60 (0.56-0.64) |


QuickJS-NG 的两个不同的页面（`wb-order-studio`、`wb-order-weekend`）
在 select 的值里，「选择快递公司」后面多一个 React 文本分隔符（`<!-- -->`）。moejs 能编译 polyfill 和另外两个 bundle，
但在商店 bundle 的最外层函数处停下，位置是第 1 行第 2 列。

### 商店按实际部署

每个引擎有自己的 Intl 就用自己的，没有就用 polyfill。
对着用自己 Intl 的 liustack/quickjs。


| 引擎 | 加载 ms | 能渲染的页面 | 和 liustack-quickjs 页面相同 | 每次渲染的时间，对 liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs（自己的 Intl） | 302 | 20 of 20 | 20 of 20 | 1.00 (1.00-1.00) |
| liustack-tifa（自己的 Intl） | 304 | 20 of 20 | 20 of 20 | 0.97 (0.90-1.03) |
| modernc-quickjs（polyfill） | 913 | 20 of 20 | 20 of 20 | 3.36 (2.86-3.87) |
| go-quickjs-interp（自己的 Intl） | 246 | 20 of 20 | 20 of 20 | 1.76 (1.35-2.01) |
| go-quickjs-jit（自己的 Intl） | 267 | 20 of 20 | 20 of 20 | 2.55 (1.89-5.72) |
| goant-interp（polyfill） | 1063 | 20 of 20 | 20 of 20 | 5.38 (4.02-6.20) |
| goant-jit（polyfill） | 1073 | 20 of 20 | 20 of 20 | 5.28 (3.45-6.16) |
| moejs（polyfill） | load: SyntaxError: function needs more than 256 registers is not supported yet | 0 of 20 | | |
| C QuickJS 2026-06-04（参照）（polyfill） | 383 | 20 of 20 | 20 of 20 | 1.36 (1.03-1.55) |
| C QuickJS master 7d1c13c5d8（参照）（polyfill） | 384 | 20 of 20 | 20 of 20 | 1.36 (1.02-1.55) |
| QuickJS-NG v0.17.0（参照）（polyfill） | 355 | 20 of 20 | 18 of 20 | 1.68 (1.34-1.96) |
| Node v24.11.1（参照）（自己的 Intl） | 146 | 20 of 20 | 20 of 20 | 0.24 (0.19-0.36) |
| Node v24.11.1 --jitless（参照）（自己的 Intl） | 91 | 20 of 20 | 20 of 20 | 0.61 (0.56-0.63) |


### MUI 页面按实际部署

三个 MUI 页面用同样的方式。用 polyfill 的引擎渲染的页面
和 liustack-quickjs 的页面差一个字符：FormatJS polyfill 在 `AM` 和 `PM` 前面写
窄不换行空格 U+202F，而 Go 的 `Intl` 和 Node
写的是普通空格（见下），所以只有自带 `Intl` 的引擎
给出的页面和 liustack/quickjs 相同。


| 引擎 | 加载 ms | 能渲染的页面 | 和 liustack-quickjs 页面相同 | 每次渲染的时间，对 liustack-quickjs |
|---|---|---|---|---|
| liustack-quickjs（自己的 Intl） | 153 | 3 of 3 | 3 of 3 | 1.00 (1.00-1.00) |
| liustack-tifa（自己的 Intl） | 158 | 3 of 3 | 3 of 3 | 0.70 (0.68-0.71) |
| modernc-quickjs（polyfill） | 735 | 3 of 3 | 0 of 3 | 6.24 (3.46-9.07) |
| go-quickjs-interp（自己的 Intl） | 140 | 3 of 3 | 3 of 3 | 1.41 (1.30-1.48) |
| go-quickjs-jit（自己的 Intl） | 134 | 3 of 3 | 3 of 3 | 1.70 (1.63-1.82) |
| goant-interp（polyfill） | 725 | 3 of 3 | 0 of 3 | 7.03 (4.92-8.71) |
| goant-jit（polyfill） | 727 | 3 of 3 | 0 of 3 | 6.19 (4.15-7.98) |
| moejs（polyfill） | 466 | 3 of 3 | 0 of 3 | 2.36 (1.68-2.89) |
| C QuickJS 2026-06-04（参照）（polyfill） | 289 | 3 of 3 | 0 of 3 | 2.03 (1.22-2.79) |
| C QuickJS master 7d1c13c5d8（参照）（polyfill） | 287 | 3 of 3 | 0 of 3 | 2.03 (1.24-2.79) |
| QuickJS-NG v0.17.0（参照）（polyfill） | 297 | 3 of 3 | 0 of 3 | 2.16 (1.56-2.62) |
| Node v24.11.1（参照）（自己的 Intl） | 58 | 3 of 3 | 3 of 3 | 0.22 (0.20-0.25) |
| Node v24.11.1 --jitless（参照）（自己的 Intl） | 56 | 3 of 3 | 3 of 3 | 0.68 (0.66-0.71) |
### liustack/quickjs 按 tifa 的方式运行

liustack/quickjs 有两行在跑 tifa 页面，它们测的是不同的东西。

- **liustack-quickjs，同样的 JavaScript。** 上面每张表里都有的那一行。原生
  `Intl` 被删掉，加载 FormatJS polyfill，每个引擎都这样，所以
  所有引擎跑的是同样的 JavaScript。它回答的是引擎运行一个给定的 bundle 有多快。
- **liustack-tifa，按 tifa 的方式运行。** liustack/quickjs 按 tifa 设置它池里的 VM 的方式配置
  （`engine/eng_liustacktifa.go`，从 tifa 的 `internal/ssr` 复制而来，
  不是导入）。它回答的是一个 tifa 应用在 liustack/quickjs 上渲染有多快，
  把 Go 这一侧算作 liustack/quickjs 提供的一部分。它有：
  - 它自己的 Go `Intl`（包 `intl`）和 `Temporal`（包 `temporal`），
    按 tifa 注册它们的方式注册，没有 polyfill
  - tifa 注册的 Go 函数：`__tifa_murmur2__`（`@emotion/hash` 的
    哈希，MUI 的样式用它生成）和 `__tifa_random_bytes__`
    （给 `crypto` 用的安全随机字节）。就这些：tifa 里另一个
    `__tifa_*` 名字 `__tifa_intl_warnings__`，是只有 `tifa dev` 才加载的
    JavaScript
  - tifa 给 VM 的默认值：128 MiB 的内存限制，10000 个槽位的栈，
    3 s 的时间预算（渲染的每一步之前都重新设置），
    以及 VM 的时钟
  - tifa 的 GC 规则：加载后做一次完整 GC，并把 GC 阈值设为存活大小的两倍，
    每次渲染后做一次年轻代回收，256 次渲染后或者
    年轻代回收之后还剩存活大小的 1.5 倍时做一次完整 GC，和 `internal/ssr/pool.go`
    里一样。这部分工作算在一次渲染的 CPU 时间里，不算在它的墙钟时间里，
    和 tifa 里一样，在那里它在页面发出之后才运行
  - 一个 bundle 编译一次，放进 `SharedBytecode`，由 VM 加载
  - 一次渲染由 `startRender(input)`、promise 任务和读取
    结果组成，和 `ssr.RenderVM` 的做法一样

`tifa/bundles` 里固定的 bundle 是在 tifa 插件让
`murmur2` 去调用 `__tifa_murmur2__` 之前构建的。适配器对有 `@emotion/hash` 的 bundle（只有 MUI）的文本做同样的一行改动：结果文件会统计它
（`bundle_rewrites`）和这个 Go 函数的调用次数
（`murmur2_calls`）。固定的 bundle 里没有一个调用 `__tifa_random_bytes__`，
商店和 bench 的 bundle 也不用 `@emotion/hash` 做哈希，所以在它们上，
Go 函数注册了但从不被调用。

C QuickJS 和 Node 的行没有这样的一行，也不可能有。
C QuickJS 自己没有 `Intl` 和 `Temporal`，所以它渲染这些页面的唯一办法
就是 polyfill。它没有能给它 Go 函数的宿主：
`qjs` 是原装程序，而带快速 `murmur2`、GC 策略
和自己 `Intl` 的 C 宿主会是一个新程序，不是这一行所叫的那个引擎。
Node 有自己完整的 `Intl`（它的「自己的 Intl」行，保留）
但没有 tifa 的函数和规则。所以这些行仍然是同样 JavaScript 的参照，
而 liustack/quickjs 的第二行显示 Go 这一侧在上面加了什么。

2026-11-06 的运行，同上（[结果](../compare/engines/results/2026-11-06-v0.30.1-gcp-pages.md)）：

- 在 bench 页面上，tifa 行用的时间和普通行一样（1.00）：它们
  既不用 Intl 也不用 Go 函数，每次渲染后的年轻代回收
  花的差不多就是引擎在别处省下的。
- 在商店上，它用的时间是普通的自带 Intl 行的 0.97（按页面 0.90 到
  1.03）。两个博客文章页面是 0.90 和 0.95，每次渲染后的年轻代回收
  在那里收益最大。
- 在 MUI 上，它用的时间是普通的自带 Intl 行的 0.70。这是 Go 里的哈希
  （`__tifa_murmur2__`）加上 tifa 其余设置的结果。
  两者没有分开测。
- 对着 polyfill 上同样的 JavaScript，普通的自带 Intl 行用的时间是 0.76
  （商店）和 0.64（MUI）。polyfill 花的比引擎还多：
  见 [polyfill 的代价](#polyfill-的代价)。

tifa 行的 HTML 对 polyfill 行的 HTML，一页一页比，
渲染时间戳已遮掉：


| bundle | 页面 | HTML 相同 | 除 U+202F 外相同 | 其他差异 | 没有比较 |
|---|---|---|---|---|---|
| bench | 2 | 2 | 0 | 0 | 0 |
| mui | 3 | 0 | 3 | 0 | 0 |
| shop | 20 | 20 | 0 | 0 | 0 |


三个 MUI 页面差一个字符。FormatJS polyfill 在时间和 `AM` 或 `PM` 之间写
窄不换行空格 U+202F（`08:00 AM`），
而 Go 的 `Intl` 写普通空格。Node 写的也是普通空格（
Node 自带 Intl 的行和 tifa 行有同样的 MUI 页面，3 of 3），所以
是 polyfill 不一样。把 U+202F
换成空格之后，这些页面逐字节相同。

### polyfill 的代价

在 liustack/quickjs 上，来自同样的运行：

- 加载它：bundle 之前每个 VM 278 ms。之后 bundle 加载
  294 ms，没有 polyfill 时 302 ms。
- 每个 VM 的内存（20 个 VM，每个都加载了 bundle，写明的地方也加载了 polyfill，
  并渲染了 zh-CN 的购物车页面）：
  有 polyfill 时 RSS 38.3 MiB，用自己的 Intl 时 17.6 MiB。
  go-quickjs-interp：99.7 MiB 对 49.4 MiB（Go 堆 79.4 对
  41.4 MiB）。
- 占一次渲染的比例：20 个页面加起来，一次渲染的 637.3 ms 里有 143.6 ms
  花在 polyfill 里（23%，1354 次调用），而同样的调用在自己的 Intl 里是 471.2 ms 里的 4.6 ms
  （1%）。

go-quickjs v0.34.1 渲染全部 20 个商店页面，用自己的 Intl 和在
polyfill 上都行，带 JIT 和不带 JIT 都行。它的 JIT 构建不稳：有一次
它在两次渲染之间把一个 bench 页面渲染得不一样（见
[在其他引擎里发现的问题](#在其他引擎里发现的问题)）。

## liustack/quickjs 远远落后的地方

速度表里每个 QuickJS 微基准、V8 套件基准和负载中，
liustack/quickjs 需要的时间至少是另外六个纯 Go 构建的中位数的两倍
（V8 套件是得分只有一半）。这次运行里
一个也没有。最接近的，是中位数的 1.2 倍或以上：

| 项目 | liustack/quickjs 对中位数 | 最快的 |
|---|---|---|
| `empty_loop` | 1.71 | go-quickjs-jit |
| `int_arith` | 1.59 | go-quickjs-jit |
| `float_arith` | 1.52 | go-quickjs-jit |
| `array_for` | 1.44 | go-quickjs-jit |
| `string_to_float` | 1.37 | moejs |
| V8 Crypto | 1.34（得分） | go-quickjs-jit |
| `string_to_int` | 1.31 | go-quickjs-jit |
| `empty_down_loop` | 1.22 | go-quickjs-jit |

中位数取自 modernc-quickjs、go-quickjs-interp、go-quickjs-jit、
goant-interp、goant-jit 和 moejs。在 `compare/engines` 里运行 `go run ./cmd/behind
results/<run>.json [limit]` 可以按你给的限制重新生成这份列表
（默认 2.0）。在 C 引擎之前的最后一次运行
（v0.27.2）里，有十一项在 2.0 到 2.9，空循环和数字
运算排在最前。很小的循环的单个微基准会在同样代码的
两次构建之间变化，因为代码落在了别的地址上：
modernc-quickjs 的同样代码在更早的两次运行里，单个测试用的时间是之前的 0.8 到 2.5
倍，而它们的几何平均是 1.05 倍。把单个测试的比值当作粗略值来读。

## 其他引擎更快的地方

这次运行里，比值取自速度表，是对 liustack/quickjs 的比值（低于 1 表示比
liustack/quickjs 快）：

- **循环里的数字和 V8 套件。** go-quickjs-interp 在 V8 套件上得分是它的 1.41 倍
  （Crypto 1.7，NavierStokes 1.6，RegExp 6.2 倍），
  go-quickjs-jit 是 4.1 倍（NavierStokes 8.2，Crypto 7.5）。go-quickjs 把
  数字存成 NaN-boxed 浮点数（`internal/vm/value.go`），有专门处理
  int32 运算、数组元素读取和整个循环的节点
  （`tree_build.go`、`tree_operand.go`、`vm_tree.go`、`ztree_flow.go`）。在
  更早一次运行的 go-quickjs-jit 和 goant-jit 剖析里，生成的机器
  代码占主导。
- **属性读取和调用。** go-quickjs 在每个访问点缓存 shape（引擎记录一个对象有哪些键、按什么顺序的结构）和槽位
  （`internal/vm/vm_shape.go`），把热点函数当作 Go 闭包的树来运行，没有操作数栈
  （`internal/vm/vm_tree.go`、
  `zcall_tree.go`），在一个节点里原地更新一个属性
  （`ztree_update.go`），调用小的纯函数时不建栈帧
  （`vm_leaf_pure.go`）。moejs 每条指令有一个内联缓存（它的指南
  `docs/guide.md` 的「Implementation」）。
- **JSON**（JSON.parse：goant 0.90 和 0.92，也就是快 1.1 倍。JSON.stringify：
  moejs 0.49，go-quickjs 0.75 和 0.76）。goant 用自己的
  代码在字符串上解析和写出（`internal/engine/builtin_json.go` 571、
  870、190），并缓存它所建对象的 shape 变化
  （`shapes.go` 404）。moejs 的 JSON 是原生 Go，在已知的
  shape 上建对象。自 v0.29.7 起，liustack/quickjs 解析 JSON 比 go-quickjs 快
  （后者的 CPU 时间是它的 1.29 倍），也比 moejs 快（1.21）。
- **正则表达式**（go-quickjs 0.35，带 JIT 0.19）。
  go-quickjs 的正则表达式单独做了剖析。moejs 把它能
  精确翻译的模式放到 Go 的 RE2 上运行（在这个负载上用的时间是
  liustack/quickjs 的 1.34 倍）。
- **字符串转数字**（`string_to_int`、`string_to_float`）。go-quickjs 有
  处理普通整数的路径（`internal/jsnum`），goant 调用 Go 的 `strconv`。
- **复制数组、解构数组**（`array_slice`、`local_destruct`）。
  go-quickjs 读源数组的稠密元素，把它们追加到
  新数组的存储里，对仍然按 `Array.prototype` 的方式迭代的数组，
  跳过迭代器协议（`internal/vm/builtins.go`、
  `internal/vm/iterate.go`）。
- **加载编译好的程序**（go-quickjs 是 liustack/quickjs 用时的 0.49 倍，
  moejs 0.24）。go-quickjs 在运行时之间共用一个编译好的程序，
  运行顶层之前只做准备（`internal/vm/eval.go`）。moejs 的
  `Script` 不可变，可以在任何运行时上运行。liustack/quickjs 把
  字节码读进每个新 VM，然后运行 bundle 的顶层。

文件名对应 go-quickjs v0.28.0，行号对应 goant v0.2.1
和 moejs v0.1.0-alpha.11。go-quickjs 的代码没有在 v0.34.1 上重读，
所以它的原因是 v0.28.0 的。这一页没有拿 C
引擎和这些引擎做剖析对比。

## 限制

| 引擎 | 每个 VM 的内存限制 | 停下脚本 | 递归深度（探测） | JIT 平台 | 其他 |
|---|---|---|---|---|---|
| liustack-quickjs | 精确：分配器超过限制就拒绝，脚本得到一个不可捕获的 out of memory（`VM.SetMemoryLimit`） | `Interrupt`，不可捕获 | 4996：默认是 5000 次调用，`SetMaxStackSize` 最多提高到 10000 | 无 JIT | 同一时刻一个 goroutine 用一个 VM，VM 互相独立。只支持 linux/amd64 和 darwin/arm64 |
| modernc-quickjs | 精确，同样的分配器 | 中断处理函数 | 抛 `InternalError: stack overflow`，不是 RangeError（v0.25.0 会因 Go 栈溢出让进程崩溃） | 无 JIT | `tail-call` 探测抛同样的错误 |
| go-quickjs | 近似：进程自上次遍历以来分配了限制的四分之一后，才遍历一次堆（`docs/known-issues.md` KI-02） | context 或 abort | 8189 | `quickjs_jit` 标签，实验性，linux/amd64、windows/amd64、darwin/arm64（`internal/jit` 的构建标签） | 没有截止时间时，正则表达式在一个步数预算之后停下，报 `SyntaxError: regular expression is too complex`（KI-17） |
| goant | 部分：数组越过了它，字符串增长越过它却没有报错，对象停住了（探测）。编译后的代码在它之外（`goant.go` 576 到 585） | `Interrupt` | 8189（带 JIT 12044） | amd64、arm64（`internal/jitasm`） | 内联缓存的计数器是整个进程共用的，所以一个运行时里的一次回收或原型变化，会让每个运行时的缓存重新填充（`internal/engine/shapes.go` 28 到 48）。默认就有 `setTimeout`、`setImmediate`、`structuredClone` 和 `process` |
| moejs | 计的是一个运行时在两次重置之间分配了多少，不是存活多少，超过后不可捕获地停下脚本（`Options.MemoryLimit`，`docs/guide.md` 的「Memory limit」）。三个探测它都停住了 | `Interrupt`，不可捕获 | 508：调用深度固定为 512（`engine/function.go` 332） | 无 JIT | 一个函数最多用 256 个寄存器（`compiler/compiler.go` 452、483、1043），tifa 的商店 bundle 超过了它。除非设置 `MutableIntrinsics`，内建对象是冻结的，由所有运行时共用。没有定时器，除非宿主加上它的 `eventloop` 包 |

文件名和行号对应引擎表里的版本。

## 该选哪个引擎

- **liustack/quickjs：** 服务端渲染，一个进程里很多带硬内存限制的
  小 VM，每个请求一个新 VM，以及需要
  Intl 或 Temporal 的页面（包 `temporal`，这个对比没有
  测它），在 linux/amd64 和 darwin/arm64 上。
- **go-quickjs-interp：** 在 CPU 密集的脚本重要、又需要完整 Intl 或
  Temporal 时，综合最好的选择。它的内存限制是
  近似的。
- **go-quickjs-jit：** 在它支持的三个平台上长时间运行的数值代码。
  一个页面的第一次渲染要花解释器的 4.1 倍时间，一个 VM
  占 5 倍内存，一个 bench 页面还有一次在
  两次渲染之间变了样（见下）。
- **goant-interp、goant-jit：** 仅次于 go-quickjs 的最完整的 test262 和 Intl，
  JSON 也快。不带 JIT 时其他方面慢。
- **moejs：** 小的插件脚本：新建运行时和编译程序都最便宜，
  JSON 和字符串快，内存少。它固定的调用深度、它的
  寄存器限制和不完整的 Intl，让它不适合 tifa 商店这样的
  大 bundle。
- **modernc-quickjs：** QuickJS 的 C 到 Go 翻译。
  每个负载上都更慢。

## 在其他引擎里发现的问题

每一条都附带在 `compare/engines` 里复现的办法。照发现时的样子报告，
基于上面的版本。2026-10-29 那次运行里 go-quickjs 的两个失败
（商店的博客文章页面，以及 JIT 构建在 FormatJS polyfill 上）
在 v0.34.1 上不再出现。

- **带 JIT 的 go-quickjs：一个 bench 页面有时在两次渲染之间渲染得不一样。**
  在 bench 应用的 `table` 页面上出现 `output changed between renders`，
  这是检查一个页面的每次渲染是否给出同样 HTML 的那一步，版本是 v0.34.1，带 JIT。不是每次运行都出现：在
  2026-11-06 的运行里出现了一次，对两个
  bench 页面以 `-warm 50 -n 200` 做 10 次运行的探测里，
  10 次中有 3 次出现。解释器构建和其他每个引擎每次给出的 HTML 都相同。上面
  表里计时的 `table` 页面来自只跑该页面、并且通过了的一次运行。复现（重复到出现为止）：`./bin/goquickjsjit tifa-ssr -bundle tifa/bundles/bench.js -inputs tifa/in/bench/spiral.json,tifa/in/bench/table.json -warm 50 -n 200`。
- **goant：`string_build_large2` 超过 4 GiB 上限。** 这个在循环里往字符串
  前面添加内容的 QuickJS 微基准，在[存档的 v0.26.17 运行](../compare/engines/results/2026-10-21-v0.26.17-gcp.json)
  的五轮里都超过了 4 GiB 上限，
  在 v0.27.1、v0.27.2、v0.29.0 和 v0.30.1 的运行里也一样。这些运行没有
  测它不设上限时的峰值内存或成功调用的时间。goant 把
  字符串存成平的，所以每次前插都要复制。复现：`./bin/goant upstream -dir ../testdata -only string_build_large2`。

## 重新运行

一切都在 `compare/engines` 里，那是一个独立的 Go 模块，所以其他
引擎永远不会进入这个模块的 `go.mod`。

```sh
cd compare/engines
go run ./cmd/compare                        # every workload, 5 rounds
go run ./cmd/compare -cpu 1 -sibling 5      # Linux: pin to CPU 1, rerun when CPU 5 was busy
go run ./cmd/compare -phases react,v8       # some workloads
go run ./cmd/compare -engines liustack-quickjs,go-quickjs-interp
go run ./cmd/compare -engines liustack-quickjs -phases t262   # one engine, one workload
go run ./cmd/compare -report results/<run>.json   # Markdown again from the JSON
```

根目录的 `make compare-engines ARGS="..."` 做同样的事。每次运行写出
`results/<date>-<version>-<host>.json` 和 `.md`，并把
每个进程的原始输出保存在 `results/raw/<run>/`（不入库）。新的运行永远不会
覆盖旧的。

在共用的机器上，一次运行可以用 `-name` 和
`-add` 切成短的几段：第一段建立这次运行，之后每一段往里追加。计时的
负载每次多跑 `-rounds` 轮，test262 可以一次跑几个
引擎，也可以在另一个 CPU 上以低优先级运行，再合并进来：

```sh
B="go run ./cmd/compare -cpu 1 -sibling 5 -name 2026-10-21-v0.26.17-gcp"
$B -phases features,concurrent
$B -add -phases react -rounds 5
$B -add -phases v8 -rounds 1          # five times
go run ./cmd/compare -cpu-test262 0 -nice-test262 19 -phases t262 -name 2026-10-21-v0.26.17-gcp-t262
go run ./cmd/compare -name 2026-10-21-v0.26.17-gcp -merge results/2026-10-21-v0.26.17-gcp-t262.json
```

一次运行做完之后才加入的引擎，单独运行，再用
`-merge-engines` 合并进来，它会复制这个引擎的全部结果，并记下这次运行
和它的二进制所来自的提交：

```sh
go run ./cmd/compare -cpu 1 -sibling 5 -engines moejs -name 2026-10-21-v0.26.16-gcp-moejs
go run ./cmd/compare -name 2026-10-21-v0.26.16-gcp -merge results/2026-10-21-v0.26.16-gcp-moejs.json -merge-engines
```

要把这个引擎的一个改动和它之前的引擎比，而不是和其他引擎比，
用 [testing.md](testing.zh-CN.md#开发者工具生成检查计时) 里的工具：
它在几个代码位置上构建两者，并按
[measuring.md](measuring.zh-CN.md) 的规程来判断结果。

要在另一台机器上测量，在那里构建，或者交叉构建后把 `bin/` 一起复制过去：

```sh
go run ./cmd/compare -build-only -goos linux -goarch amd64 -bindir bin-linux-amd64
GOOS=linux GOARCH=amd64 go build -o bin-linux-amd64/compare ./cmd/compare
# on the machine, in compare/engines with bin/ = bin-linux-amd64:
./bin/compare -nobuild -cpu 1 -sibling 5
```

驱动程序需要 `compare/testdata` 和 `internal/arewefastyet/v8-v7` 放在它
旁边，和仓库里一样。test262 按固定的提交下载，除非
`-test262` 指向一份检出。

`native/build.sh` 只在构建参照行时才获取这些输入。
它们不随这个仓库发布。脚本固定了 URL 和
SHA256 哈希。下面是主要的许可证，各个下载的发行版里
带的第三方条款仍然保留：

| 下载 | 版本 | 许可证 |
| --- | --- | --- |
| C QuickJS 发布版 | 2026-06-04 | MIT，见压缩包里的 `LICENSE` |
| C QuickJS master 快照 | 7d1c13c5d8 | [MIT](https://github.com/bellard/quickjs/blob/7d1c13c5d8/LICENSE) |
| QuickJS-NG | v0.17.0 | [MIT](https://github.com/quickjs-ng/quickjs/blob/v0.17.0/LICENSE) |
| Node，Linux x64 二进制 | v24.11.1 | [MIT 和第三方声明](https://github.com/nodejs/node/blob/v24.11.1/LICENSE)，包括 V8（BSD-3-Clause）、OpenSSL（Apache-2.0）和 ICU（Unicode） |
| CMake，Linux x86_64 二进制 | 3.31.6 | [BSD-3-Clause](https://github.com/Kitware/CMake/blob/v3.31.6/Copyright.txt)，第三方条款在发行版的 `Licenses/` 下 |

两个 C QuickJS 行用随它们发布的 Makefile 构建，它
没有定义 `NDEBUG`，所以它们的 `assert` 会运行，而 QuickJS-NG 的 CMake
Release 构建定义了 `-DNDEBUG`。为速度而构建的 C QuickJS 行，要在它的
Makefile 里往 `DEFINES` 加 `-DNDEBUG`。要说明表里是哪一种。

tifa 页面和原生参照行有自己的脚本
（Linux，CPU 1 和它的同伴 CPU 5）：

```sh
cd compare/engines
tifa/polyfill/build.sh                      # Node and npm: the FormatJS bundle
native/build.sh "$HOME/jsnative"            # C QuickJS, QuickJS-NG and Node
run="<date>-<version>-<host>"
page_dir=$(mktemp -d "/tmp/$run-pages.XXXXXX")
tifa/run.sh "$page_dir" bin-linux-amd64 "$HOME/jsnative"
go run ./cmd/pages "$page_dir" --output "results/$run-pages"
```

`tifa/run.sh` 运行 bench、MUI 和商店的 bundle。`TIFA_ONLY=<engine>,...`
（名字和结果文件里的一样）只运行这些引擎，这样在共用的机器上
可以把一次运行切成几段，`TIFA_CPU` 和 `TIFA_SIBLING` 把运行固定到
CPU 1 和 5 以外的另一个 CPU 和它的超线程同伴核。检查、计时和
内存的运行属于 `liustack-quickjs`。

`TIFA_BUNDLES=<dir>` 运行另一组 bundle 和输入，布局和
`tifa/` 一样（`bundles/<app>.js`、`in/<app>/*.json`），代替固定的那组。
tifa 把它的 SSR bundle 构建成 ES 模块：把它放在那里，
命名为 `bundles/<app>.mjs`，`liustack-tifa` 的行把它当作模块编译，
再放一份 `bundles/<app>.js`，用 `go run ./cmd/modulescript <app>.mjs <app>.js` 生成，
其他每一行把它当作脚本运行。

当一个页面让某个引擎没法再用时（页面失败后 go-quickjs 会关闭它的运行时），
把商店去掉失败的页面再运行一次，写进同一个目录里的
`<engine>-<kind>-rest.json`：`cmd/pages` 从那里取
缺的页面。大多数页面都失败时，要找出失败的页面，
先把每个页面单独渲染一次（`-inputs <one page> -warm 0 -n 1`）。

每次发布时，照 [releasing.md](releasing.zh-CN.md#重新和其他引擎对比) 里的
「Engine comparison」做。

## 测量方法

- 每个引擎链接进它自己的二进制（`jsbench`，构建标签
  `eng_<name>`），用 `CGO_ENABLED=0 -pgo=off -trimpath` 构建，所以没有引擎
  和另一个引擎共用进程、剖析数据或垃圾回收器。
- 每个引擎按它的 README 推荐的方式使用：脚本编译一次，
  在 API 允许的地方当作程序运行，引擎需要的地方注册 Intl，
  模块走各引擎自己的加载器。结果页面
  逐个引擎列出具体怎么用。
- 计时的负载分轮运行。每一轮里每个引擎在新进程中运行每个
  负载，引擎的顺序每轮都变。表里显示各轮中位数的中位数，括号里是
  各轮中位数的范围。
- 用 `-cpu` 时，Go 驱动程序用 `taskset -c <cpu>` 固定计时进程，
  并设 GOMAXPROCS=1。`tifa/run.sh` 把它的功能检查
  （normal、stub 和 proxy）和计时进程都固定到 CPU 1，走
  同一个带上限的入口，设 TZ=UTC 和 GOMAXPROCS=1。如果那个 CPU 的超线程
  同伴核在一次运行里忙了至少 3 个时钟 tick（30 ms）并且超过运行时间的 5%，或者
  引擎进程拿到的 CPU 时间不到这次运行的 90%（另一个
  进程跑在了它的 CPU 上），这次运行就重来，一共最多尝试三次。结果会统计重试
  次数并保留最后一次。
- test262 不需要安静的 CPU：它可以在另一个 CPU 上以低优先级运行
  （`-cpu-test262 0 -nice-test262 19`），作为单独的一次运行，再用
  `-merge` 把它的结果复制进主运行。
- 每个引擎进程都在内存上限下运行（4 GiB，test262 是 2 GiB）：
  在 Linux 上用 cgroup（`systemd-run --user --scope -p MemoryMax`），它也
  计入 Go 堆以外的内存，再加上对驻留大小的检查。
- tifa 行（`liustack-tifa`）和其他行一样构建进自己的二进制
  （标签 `eng_liustacktifa`），`cmd/compare` 和它们一起构建它，
  但除非 `-engines` 点名，否则不在其他负载上运行它。它只
  在 `tifa/run.sh` 里运行。
- 固定的输入：`d53ebc8b` 的 test262，来自 mozilla/arewefastyet 的 V8 基准套件
  第 7 版（`d2ddc6ce`），用 esbuild 0.28.2 打包的 React 19.3.1，`tifa/bundles`
  里的 tifa bundle（`SHA256SUMS`），`tifa/polyfill/package-lock.json`
  里的 FormatJS 包。

### 负载

| 负载 | 它做什么 |
|---|---|
| React SSR 渲染 | 一个 717 个元素的目录页的 `renderToString`（React 19.3.1，`react-dom/server` 旧版浏览器构建，一个 117 KB 的 IIFE bundle，没有宿主函数）。每个引擎的第一次渲染都和 Node 的输出逐字节比较。 |
| V8 套件 | V8 套件第 7 版的 8 个基准，得分按套件自己的算法计算。许可证：见 `internal/arewefastyet/v8-v7`。 |
| JSON.parse、JSON.stringify | 一个 1000 项的文档（UTF-8 编码 155 KiB），每次调用解析或写出一次。 |
| 字符串构建 | 用 `+=` 追加 20000 次，然后用 `Array.prototype.join` 拼接 5000 段。 |
| RegExp | 五个常见的正则表达式（带 `g` 的 `match`、`exec` 循环、`replace`、`split`），作用于一份 2000 行的访问日志。 |
| QuickJS 微基准 | `compare/testdata`，QuickJS 自己的微基准，每个测试一个进程，以每次迭代的时间相对 liustack/quickjs 表示。 |
| 冷启动 | 新建一个 VM，新建一个从源码加载 bundle 的 VM，从编译好的程序加载的 VM（检查它渲染出同样的页面），以及第一次渲染。 |
| 内存 | 100 个 VM 同时存活时，每个 VM 让进程增长的量，每个都加载了 bundle 并渲染了一个页面。 |
| 特性 | 77 个探测，涵盖 ES2015 到 ES2025、Intl（和 Node 24 的答案比较）和限制。 |
| test262 | `test/language`、`test/built-ins`、`test/intl402`，sloppy 和 strict，每次运行用新 VM。每个引擎都不含模块测试和需要宿主钩子的测试（`$262.createRealm` 之类）。 |
| tifa 页面 | 三个 tifa 应用的服务端 bundle（`tifa/bundles`），按 tifa 渲染它们的方式渲染：`startRender(json)`，然后是 promise 任务，然后是 `renderResult`。输入是这些应用的服务端为每个页面发送的数据。 |

## 历史

| 运行 | liustack/quickjs | 机器 | 备注 |
|---|---|---|---|
| [2026-11-06-v0.30.1-gcp](../compare/engines/results/2026-11-06-v0.30.1-gcp.md) | v0.30.1（main `5e99b850`） | GCP c2d-standard-8，AMD EPYC 7B13，CPU 0 和它的同伴核 4 | 所有引擎重新运行（[页面](../compare/engines/results/2026-11-06-v0.30.1-gcp-pages.md)），go-quickjs 是 v0.34.1，modernc.org/quickjs 是 v0.27.0。计时的负载和 tifa 页面在一对核的锁下分成短的几段运行，test262 在另一对核上以低优先级运行并合并进来。页面运行里 go-quickjs-jit 在两次渲染之间把 bench 的 `table` 页面渲染得不一样，所以那个页面是在单独的一次运行里测的。结果文件里引擎表的版本列写的是 `v0.30.1 => ../../`，驱动程序报 `v0.30.1-dirty`，因为对比的 `go.mod` 为了用更新的引擎被改过 |
| [2026-10-29-v0.29.0-gcp](../compare/engines/results/2026-10-29-v0.29.0-gcp.md) | v0.29.0（main `0cc6d332`） | GCP c2d-standard-8，AMD EPYC 7B13，CPU 0 和它的同伴核 4 | 所有引擎重新运行，这是 C 引擎的第一次运行（[页面](../compare/engines/results/2026-10-29-v0.29.0-gcp-pages.md)）。先是计时的负载，然后是 tifa 页面，然后是两个核上的 test262，一个接一个。go-quickjs 失败的页面在补跑里测。`liustack-tifa-nogoasm` 这一行没有了。结果文件里引擎表的版本列写的是 `v0.26.16 => ../../`，因为对比的 `go.mod` 没有往前移：引擎就是这份检出 |
| [2026-10-24-v0.28.6-gcp-pages](../compare/engines/results/2026-10-24-v0.28.6-gcp-pages.md) | v0.28.6 | GCP c2d-standard-8，AMD EPYC 7B13，CPU 3 | 只有 tifa 页面，用的是 tifa c729ad2 构建的 bundle，不是固定的那些：liustack/quickjs 三次（同样的 JavaScript、按 tifa 的方式运行、以及按 tifa 的方式但不带 Go 汇编解释器运行），C QuickJS 2026-06-04 和 master，QuickJS-NG，以及带 JIT 和不带 JIT 的 Node。一对核 3-7 在锁下。其他引擎没有重新运行 |
| [2026-10-24-main-tifa-row-gcp-pages](../compare/engines/results/2026-10-24-main-tifa-row-gcp-pages.md) | main (c17092a) | GCP c2d-standard-8，AMD EPYC 7B13，CPU 1 | 只有 tifa 页面：liustack/quickjs 两次（同样的 JavaScript、按 tifa 的方式运行），C QuickJS 2026-06-04，以及带 JIT 和不带 JIT 的 Node，MUI 页面第一次计时。一对核 1-5 在锁下。其他引擎没有重新运行 |
| [2026-10-23-v0.27.2-gcp](../compare/engines/results/2026-10-23-v0.27.2-gcp.md) | v0.27.2 | GCP c2d-standard-8，AMD EPYC 7B13，CPU 1 | 所有引擎在同样的版本上重新运行（[页面](../compare/engines/results/2026-10-23-v0.27.2-gcp-pages.md)）。test262 在 CPU 0 上以低优先级运行并合并进来，通过的还是那 80215 次运行。React 页面 CPU 是 15.1 ms，之前是 14.9，V8 得分 835，之前是 854，JSON 和正则表达式快 1% 到 3%。`string_to_int` 和 `array_read` 越过了远远落后的线，`string_to_float` 降到线下 |
| [2026-10-22-v0.27.1-gcp](../compare/engines/results/2026-10-22-v0.27.1-gcp.md) | v0.27.1 | GCP c2d-standard-8，AMD EPYC 7B13，CPU 1 | 所有引擎在同样的版本上重新运行（[页面](../compare/engines/results/2026-10-22-v0.27.1-gcp-pages.md)）。test262 在 CPU 0 上以低优先级运行并合并进来。React 页面 CPU 是 14.9 ms，之前是 16.3，V8 得分 854，之前是 800，`Date.now()` 不再远远落后，13 个 Intl 探测全部通过 |
| [2026-10-21-v0.26.17-gcp](../compare/engines/results/2026-10-21-v0.26.17-gcp.md) | v0.26.17 | GCP c2d-standard-8，AMD EPYC 7B13，CPU 1 | 所有引擎重新运行，go-quickjs 是 v0.28.0（[页面](../compare/engines/results/2026-10-21-v0.26.17-gcp-pages.md)） |
| [2026-10-21-v0.26.16-gcp](../compare/engines/results/2026-10-21-v0.26.16-gcp.md) | v0.26.16 | GCP c2d-standard-8，AMD EPYC 7B13，CPU 1 | 第一次运行，带 tifa 页面（[页面](../compare/engines/results/2026-10-21-v0.26.16-gcp-pages.md)）。moejs 在同一天加入。jsc2go 和 fastschema/qjs 在运行之后拿掉 |
