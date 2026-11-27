---
summary: 'liustack/beastjs 和其他 Go 的 JavaScript 引擎以及三个原生引擎比起来怎样，都按解释器比：tifa 页面的渲染时间和 V8 测试套件的得分，每个引擎带着什么跑，以及怎么重跑这次对比'
audience: user
order: 4
read_when:
  - 要公开说这个引擎和其他 Go 引擎、C QuickJS、Hermes 或 Node 比起来怎样
  - 为某件事挑选 Go 的 JavaScript 引擎
  - 项目所有者要求重跑对比（重跑并更新这一页）
  - 修改 compare/engines
translatedFrom: aa0bb730e96d
---

# JavaScript 引擎对比

[English](benchmarks.md) · 简体中文

2026-11-24 的一次运行：liustack/beastjs 的 v0.32.0（引擎和 v0.33.0 相同，当时还是旧的模块路径）对比另外两个 Go 引擎和三个原生引擎，机器是 GCP t2d-standard-4（AMD EPYC 7B13，四个核，没有 SMT），Linux 7.0，Go 1.27.4。每个计时的进程都跑在一个核上，整台机器都留给它，GOMAXPROCS=1。这次运行的文件在 [`compare/engines/results/`](../../compare/engines/results/)（`2026-11-24-v0.32.0-gcp-pages` 和 `-v8`，各有 `.md` 和 `.json`），怎么重跑见 [`compare/engines/README.md`](../../compare/engines/README.md)。这些数字只来自这一台机器：不要拿它们和别的机器上的数字比。

## 比的是什么

两张表：tifa 页面的渲染时间和 V8 测试套件。对比从不在其他引擎上跑 test262。

- **对比只比解释器。** 有 JIT 的引擎关掉 JIT 跑，或者不参加。go-quickjs 有一个默认关闭的 JIT，就按关闭的样子跑。原生的参照是 C QuickJS、Hermes V1 和加了 `--jitless` 的 Node。
- **每个引擎都按它本来的样子跑**，就像一个人平常用它那样。有自己的 Intl 和 URL 就用自己的，没有的就用标准的 polyfill：Intl 用 FormatJS，URL、URLSearchParams、structuredClone 和迭代器辅助方法用 core-js。
- **页面运行时不带 tifa 的 Go 函数。** 打包文件只在 `__tifa_escape`、`__tifa_start_tag`、`__tifa_end_tag`、`__tifa_copy_props` 和 `__tifa_murmur2__` 存在时才用它们，这里没有一个引擎注册它们，我们的也不例外（核对过：在我们的 VM 里它们是 `undefined`）。每个引擎跑的都是 React 自己的代码。tifa 的 GC 规则、池的设置和共享字节码也都不用。

| 引擎 | 版本 | 怎么跑 |
|---|---|---|
| liustack-beastjs | v0.32.0 | 注册了本模块的包 `intl`、`temporal` 和 `url` |
| liustack-beastjs-polyfill | v0.32.0 | 同一个引擎，不带这三个包，用 C QuickJS 和 Hermes 用的 polyfill。只在页面那张表里 |
| go-quickjs-interp | go-quickjs v0.43.0 | 关闭 JIT，用它自己的 Intl |
| moejs | v0.1.1 | 用它自己的（不完整的）Intl，外加一个 `FinalizationRegistry` 替代品（`native/moejs-shim.js`） |
| C QuickJS | 2026-06-04 | `qjs`，按发布时的方式构建 |
| Hermes V1 | `260318099.0.8` | 这个版本没有 JIT，跑 `hermesc -O` 字节码，和应用发布时一样 |
| Node | v26.13.1 | `--jitless`，用它自己的 Intl 和 URL |

没有参加的：goja、sobek 和 otto（没有异步生成器、WeakRef 和较新的正则表达式标志），modernc.org/quickjs（QuickJS 翻译成的 Go，慢得多），以及 goant（关掉 JIT 后它在 V8 套件上约 300 分，这种设置没人用，而且它从 2026-09-21 起就没有改动过）。

## 渲染

一次渲染 tifa 页面的 CPU 时间，相对 liustack/beastjs 的比值：一组页面上的几何平均，取两次配对运行的中位数，括号里是较低和较高的那次。大于 1 表示比 liustack/beastjs 慢。

| 引擎 | bench（2 个页面） | MUI（3 个页面） | 商店（20 个页面） |
|---|---|---|---|
| liustack-beastjs | 1.00 | 1.00 | 1.00 |
| liustack-beastjs-polyfill | 1.03 (1.01–1.04) | 1.71 (1.71–1.72) | 1.62 (1.61–1.63) |
| go-quickjs-interp | 2.31 (2.25–2.38) | 2.12 (2.09–2.16) | 3.11 (3.09–3.13) |
| moejs | 1.52 (1.47–1.56) | 1.52 (1.49–1.54) | 1.99 (1.94–2.03) |
| C QuickJS 2026-06-04 | 2.06 (2.05–2.07) | 3.63 (3.57–3.69) | 2.59 (2.57–2.61) |
| Hermes V1，`-O` 字节码 | 1.08 (1.07–1.09) | 1.90 (1.86–1.94) | 1.86 (1.84–1.88) |
| Node v26.13.1 `--jitless` | 0.99 (0.99–0.99) | 1.11 (1.10–1.12) | 0.90 (0.89–0.90) |

- **第二行**是不带内置 Intl、URL 和 Temporal 的 liustack/beastjs，用和 C QuickJS、Hermes 一样的 polyfill。它显示 MUI 和商店页面上的领先有多少来自解释器，多少来自用 Go 写的 Intl。在同样的 JavaScript 上，C QuickJS 在 bench 页面上花的时间是它的 2.0 倍，MUI 上是 2.1 倍，商店上是 1.6 倍，Hermes 则是 1.05、1.11 和 1.15 倍。这两列上剩下的领先来自 Go 的 Intl。
- **polyfill 用在哪里。** C QuickJS、Hermes 和第二行没有 Intl，在 MUI 和商店页面上用了 FormatJS。来自 core-js 的有：go-quickjs、C QuickJS、Hermes 和第二行的 URL、URLSearchParams 和 structuredClone，moejs 的 URL、URLSearchParams 和迭代器辅助方法，以及 Hermes 和 Node 的 `Iterator.concat`。liustack/beastjs 和 Node 的 Intl 和 URL 不需要 polyfill。bench 页面用不到 Intl，所以这一列每个引擎跑的是同样的 JavaScript：「这个引擎在同样的代码上比 C QuickJS 快」这个说法，依据的就是这一列和 V8 套件。在 C QuickJS 和 Hermes 里，core-js 只替换 `Function.prototype.toString` 以及 `Iterator.prototype.drop` 和 `take`（在 C QuickJS 上还有 `flatMap`），而 bench 的渲染一个也不调用（在每个页面的 3 次渲染上数过），也不调用 `new URL`，所以这一列量的是引擎本身。
- **moejs** 需要一个替代品才能载入打包文件：打包文件在载入时读取 `FinalizationRegistry`，moejs v0.1.1 没有它，`native/moejs-shim.js` 给它一个从不回调的 registry。有了它和 core-js，全部 25 个页面都能渲染。
- **Hermes** 没有异步生成器，所以只为 Hermes，esbuild 把打包文件里的异步生成器和 `for await` 降级成 ES2017 的 async 函数。一个放替代品的小文件（`native/hermes-shim.js`）给 Hermes 补上打包文件只在载入时读取、而它又没有的内置功能（可调整大小的 ArrayBuffer 的 getter、RegExp 标志的 getter、`SharedArrayBuffer`、`Float16Array`）。它的 shell 没有 `performance.now`，所以它的时间是整个计时循环实际经过的时间除以渲染次数。
- **HTML。** go-quickjs、moejs 和 Node 在全部 25 个页面上给出的 HTML 都和我们的一样。C QuickJS 在 9 个页面上不同，第二行在同样的 9 个页面上不同（它的 HTML 在全部 25 个页面上都和 C QuickJS 的一样），Hermes 在全部 25 个页面上都不同，不同之处总在 Intl 格式化的文本里，对 Hermes 来说还有路由的内联脚本。C QuickJS 用 FormatJS 时把日期写成 `October 13, 14:32`，Go 的 Intl 则写成 `October 13 at 14:32`，它还在 `AM` 和 `PM` 前写一个窄的不换行空格。Hermes 把路由的脱水（dehydration）函数打印成 `function($R){return ...}`，其他引擎则按原样打印那个箭头函数（这处在 Intl 输出之外），MUI 和 workbench 页面的日期文本也和 C QuickJS 一样有差别。其余每个字节都相同：`-pages.md` 文件列出了这些页面，以及每个页面第一处不同的位置。

## V8 测试套件

每个引擎在 V8 测试套件第 7 版上的得分，取 10 轮的中位数，每一轮里每一方各跑一次，先后顺序每轮轮换。越高越好。括号里是最低和最高的一轮。

| 引擎 | Richards | DeltaBlue | Crypto | RayTrace | EarleyBoyer | RegExp | Splay | NavierStokes | 得分 |
|---|---|---|---|---|---|---|---|---|---|
| liustack-beastjs | 1370 | 1146 | 1489 | 2626 | 3466 | 1845 | 4236 | 2948 | 2160 (2122–2201) |
| go-quickjs-interp | 1295 | 1240 | 1879 | 2457 | 2596 | 2684 | 3678 | 4210 | 2308 (2219–2357) |
| moejs | 1239 | 1078 | 1342 | 1903 | 2430 | 1360 | 3184 | 3201 | 1816 (1767–1877) |
| C QuickJS 2026-06-04 | 836 | 748 | 932 | 1593 | 1994 | 378 | 2822 | 1813 | 1172 (1146–1191) |
| Hermes V1，`-O` 字节码 | 1315 | 1512 | 1748 | 4595 | 4770 | 549 | 2478 | 3542 | 2096 (2053–2132) |
| Node v26.13.1 `--jitless` | 1390 | 1496 | 1030 | 3518 | 5532 | 2933 | 3367 | 1558 | 2240 (2181–2279) |

下面的比值，是同一轮里两个引擎得分之比的中位数。liustack/beastjs 的得分是 C QuickJS 的 1.85 倍，每一项都领先它。在 Go 引擎里，它的得分是 moejs 的 1.20 倍，并且每一轮都领先 moejs（比值从 1.16 到 1.22），但 go-quickjs 的得分是它的 1.07 倍，每一轮都领先它（1.04 到 1.07）。它不是这个套件上最快的引擎：go-quickjs 和 Node `--jitless`（是它的 1.03 倍）得分更高，它的得分是 Hermes 的 1.03 倍（1.02 到 1.04）。和 go-quickjs 比，它在 EarleyBoyer（1.4 倍）、Splay（1.1 倍）、Richards 和 RayTrace（都是 1.05 倍）上领先，在 NavierStokes（go-quickjs 是它的 1.4 倍）、RegExp（1.45 倍）、Crypto（1.3 倍）和 DeltaBlue（1.1 倍）上落后。和 moejs 比，它除了 NavierStokes（moejs 是它的 1.07 倍）以外每一项都领先。liustack/beastjs 在 Splay 上领先（是其他每一行的 1.1 到 1.7 倍）。

## 页面

打包文件是三个 tifa 应用的服务端打包文件，用 tifa `63a2099e` 构建（v0.0.10：Node 24.13.0、pnpm 10.33.0、Go 1.27.4）：bench 应用（`test/apps/bench`，2 个页面）、MUI 应用（`test/apps/mui`，3 个页面）和示例商店（`examples/shop`，20 个页面：6 个 workbench 页面，14 个 store 页面，分 en-US 和 zh-CN）。它们在 `compare/engines/tifa/bundles`，SHA256 如下：

```
be4fe9d65453a8628a09b2edc7d712fc5684d86e59722377523ecb137576f517  bench.js
881a59600aca99c1f5ac00775cf150a807d054bc893fff419461e7be85f5a586  mui.js
656886554a87dae25a2ced650fe8fbf47b83b8eefaecb0d8ff1a8c327576c3a9  shop.js
```

每个页面都按 tifa 的方式渲染：用每个应用的服务器为这个页面发送的输入（`tifa/in`）调用 `startRender(json)`，跑 promise 任务，然后取结果。一个页面的时间是每次渲染的 CPU 时间（bench 页面是 50 次预热后 200 次计时渲染的平均值，MUI 和商店是 20 次预热后 100 次）。C QuickJS 和 Node 在脚本里按实际经过的时间给渲染计时。每个引擎的 HTML 都写出来，遮掉路由的渲染时间戳后，和我们的对比。

## 重跑

见 [`compare/engines/README.md`](../../compare/engines/README.md)：命令、需要的机器、怎么重新生成打包文件，以及它怎么测量。重跑了对比的版本在打标签之前，按 [releasing.md](../internals/releasing.md#the-comparison-with-the-other-engines) 里「The comparison with the other engines」一节去做。


test262 只在 liustack/beastjs 上跑，由仓库自己的检查来跑（[testing.md](../internals/testing.md#test262)）：跑了 83426 个测试，有 32 个已知错误，每次运行都一样。
