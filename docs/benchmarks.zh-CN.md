---
summary: 'liustack/slowjs 和 Go 的其他 JavaScript 引擎以及三个原生引擎比起来怎样，全部作为解释器来比：tifa 页面的渲染时间和 V8 套件得分，每个引擎用什么运行，以及怎么重新运行这个对比'
read_when:
  - 对外说这个引擎和其他 Go 引擎、C QuickJS、Hermes 或 Node 比起来怎样
  - 为一项工作选 Go 的 JavaScript 引擎
  - 发次版本，或所有者点名的补丁版本（重新运行对比并更新这一页）
  - 改 compare/engines
---

# JavaScript 引擎对比

[English](benchmarks.md) · 简体中文

2026-11-16 的运行：liustack/slowjs v0.31.1 对 Go 的另外两个引擎和三个原生引擎，在 GCP t2d-standard-4（AMD EPYC 7B13，四核，没有 SMT）上，Linux 7.0，Go 1.27.4。每个计时的进程都在一个核上运行，整台机器为它独占，GOMAXPROCS=1。这次运行的文件在 [`compare/engines/results/`](../compare/engines/results/)（`2026-11-16-v0.31.1-gcp-pages` 和 `-v8`，各有 `.md` 和 `.json`），怎么重新运行见 [`compare/engines/README.zh-CN.md`](../compare/engines/README.zh-CN.md)。数字都来自这一台机器：不要拿它们和别的机器的数字比。

## 比什么

两张表：tifa 页面的渲染时间和 V8 套件。对比从不在其他引擎上运行 test262。

- **对比只在解释器之间进行。** 带 JIT 的引擎关掉 JIT 来跑，或者不比。go-quickjs 的 JIT 默认是关的，就这样跑。原生参照是 C QuickJS、Hermes V1 和带 `--jitless` 的 Node。
- **每个引擎都照原样运行**，像人们真实使用的那样。它有自己的 Intl 和 URL 就用自己的，没有就加载标准的 polyfill：Intl 用 FormatJS，URL、URLSearchParams、structuredClone 和迭代器辅助方法用 core-js。
- **页面不带 tifa 的 Go 函数运行。** 这些包只在 `__tifa_escape`、`__tifa_start_tag`、`__tifa_end_tag`、`__tifa_copy_props` 和 `__tifa_murmur2__` 存在时才用它们，而这里没有引擎注册它们，我们的也不例外（已核对：在我们的 VM 里它们是 `undefined`）。每个引擎跑的都是 React 自己的代码。tifa 的 GC 规则、池的设置和共享字节码也都不用。

| 引擎 | 版本 | 怎么运行 |
|---|---|---|
| liustack-slowjs | v0.31.1 | 注册本模块的 `intl`、`temporal` 和 `url` 三个包 |
| liustack-slowjs-polyfill | v0.31.1 | 同一个引擎，不带这三个包，用 C QuickJS 和 Hermes 用的那些 polyfill。只在页面那张表里 |
| go-quickjs-interp | go-quickjs v0.38.0 | 关 JIT，用它自己的 Intl |
| moejs | v0.1.0 | 用它自己的（不完整的）Intl，外加一个 `FinalizationRegistry` 替身（`native/moejs-shim.js`） |
| C QuickJS | 2026-06-04 | `qjs`，照发布的方式构建 |
| Hermes V1 | `260318099.0.0-stable` | 这个版本没有 JIT，运行 `hermesc -O` 编好的字节码，就像应用发布的那样 |
| Node | v24.22.3 | `--jitless`，用它自己的 Intl 和 URL |

没有比的：goja、sobek 和 otto（没有异步生成器、WeakRef 和较新的正则表达式标志）、modernc.org/quickjs（QuickJS 翻译成 Go，慢得多）和 goant（关掉 JIT 后它的 V8 套件得分约 300，没有人这样用，而且 2026-09-21 以后没有改动）。

## 渲染

tifa 页面一次渲染的 CPU 时间，表示成相对 liustack/slowjs 的比值：一组页面上的几何平均，括号里是范围。大于 1 表示比 liustack/slowjs 慢。

| 引擎 | bench（2 个页面） | MUI（3 个页面） | 商店（20 个页面） |
|---|---|---|---|
| liustack-slowjs | 1.00 | 1.00 | 1.00 |
| liustack-slowjs-polyfill | 1.02 (1.02–1.03) | 1.70 (1.19–2.12) | 1.63 (1.19–2.36) |
| go-quickjs-interp | 2.51 (2.44–2.58) | 2.56 (2.46–2.62) | 3.55 (2.68–6.00) |
| moejs | 1.29 (1.27–1.32) | 1.52 (1.49–1.55) | 1.86 (1.39–3.09) |
| C QuickJS 2026-06-04 | 1.85 (1.71–1.99) | 3.64 (2.17–5.10) | 2.44 (1.74–3.29) |
| Hermes V1，`-O` 字节码 | 0.97 (0.90–1.05) | 1.92 (1.21–2.56) | 1.78 (0.96–4.57) |
| Node v24.22.3 `--jitless` | 0.91 (0.90–0.93) | 1.15 (1.13–1.16) | 0.94 (0.75–1.00) |

- **第二行**是不带内置 Intl、URL 和 Temporal 的 liustack/slowjs，和 C QuickJS、Hermes 用同样的 polyfill。它显示 MUI 和商店页面上的领先有多少来自解释器，多少来自用 Go 写的 Intl。跑同样的 JavaScript 时，C QuickJS 在 bench 页面上用 1.8 倍的时间，MUI 上 2.1 倍，商店上 1.5 倍，Hermes 大致持平（0.95、1.13 和 1.09 倍）。这两列上其余的领先来自 Go 写的 Intl。
- **哪些引擎用了 polyfill。** C QuickJS、Hermes 和第二行没有 Intl，在 MUI 和商店页面上用了 FormatJS。go-quickjs、C QuickJS、Hermes 和第二行的 URL、URLSearchParams 和 structuredClone 来自 core-js，moejs 的 URL、URLSearchParams 和迭代器辅助方法来自 core-js，Hermes 和 Node 的 `Iterator.concat` 也是。liustack/slowjs 和 Node 的 Intl 与 URL 不需要 polyfill。bench 页面不需要 Intl，所以那一列是每个引擎跑同样的 JavaScript：说这个引擎在同样的代码上比 C QuickJS 快，靠的是那一列和 V8 套件。core-js 在 C QuickJS 和 Hermes 里只替换了 `Function.prototype.toString` 和 `Iterator.prototype.drop`、`take`（C QuickJS 里还有 `flatMap`），而 bench 页面的渲染一个都不调用（每个页面数过 3 次渲染），也不调用 `new URL`，所以那一列测的是引擎本身。
- **moejs** 要一个替身才能加载这些包：包在加载时读 `FinalizationRegistry`，moejs v0.1.0 没有它，`native/moejs-shim.js` 给它一个从不回调的注册表。有了它再加 core-js，25 个页面全部渲染。
- **Hermes** 没有异步生成器，所以只为 Hermes，用 esbuild 把包里的异步生成器和 `for await` 降级成 ES2017 的 async 函数。一个很小的替身文件（`native/hermes-shim.js`）给 Hermes 补上包只在加载时读、而它没有的内置对象（可调整大小的 ArrayBuffer 的取值器、RegExp 标志的取值器、`SharedArrayBuffer`、`Float16Array`）。它的 shell 没有 `performance.now`，所以它的时间是整个计时循环的墙钟时间除以渲染次数。
- **HTML。** go-quickjs、moejs 和 Node 在全部 25 个页面上和我们的 HTML 相同。C QuickJS 有 9 个页面不同，第二行也是这 9 个（它的 HTML 在全部 25 个页面上和 C QuickJS 的相同），Hermes 是全部 25 个，不同的地方总是 Intl 格式化的文字，Hermes 还有路由的内嵌脚本。C QuickJS 用 FormatJS 把日期写成 `October 13, 14:32`，Go 的 Intl 写成 `October 13 at 14:32`，`AM` 和 `PM` 前面还是一个窄的不换行空格。Hermes 把路由的脱水函数打印成 `function($R){return ...}`，别的引擎打印的是原样的箭头函数（不在 Intl 的输出里），MUI 和工作台页面的日期文字和 C QuickJS 一样不同。其余每个字节都相同：不同的页面在 `-pages.md` 文件里列出，附上各自第一处不同的位置。

## V8 套件

每个引擎在 V8 套件第 7 版上的得分，在同一次会话里 5 轮的中位数，每一轮每一方各跑一次，顺序轮流变。越高越好。括号里是最低和最高的一轮。

| 引擎 | Richards | DeltaBlue | Crypto | RayTrace | EarleyBoyer | RegExp | Splay | NavierStokes | 得分 |
|---|---|---|---|---|---|---|---|---|---|
| liustack-slowjs | 1133 | 1004 | 1425 | 2202 | 3031 | 1120 | 4645 | 2454 | 1854 (1842–1863) |
| go-quickjs-interp | 998 | 882 | 1299 | 1583 | 1863 | 2901 | 3069 | 2338 | 1702 (1693–1706) |
| moejs | 1243 | 1080 | 1361 | 1933 | 2512 | 1475 | 3357 | 3145 | 1864 (1848–1875) |
| C QuickJS 2026-06-04 | 829 | 753 | 889 | 1571 | 2012 | 375 | 3121 | 1777 | 1178 (1176–1183) |
| Hermes V1，`-O` 字节码 | 1297 | 1521 | 1749 | 4593 | 4855 | 553 | 2857 | 3394 | 2136 (2107–2151) |
| Node v24.22.3 `--jitless` | 1356 | 1445 | 1051 | 3534 | 5695 | 3037 | 3374 | 1645 | 2272 (2259–2290) |

liustack/slowjs 的得分是 C QuickJS 的 1.57 倍，每个子项都领先它。在 Go 的引擎里它和 moejs 打平（1854 对 1864，两边各轮的范围有重叠），是 go-quickjs 的 1.09 倍。它不是这个套件上最快的引擎：Hermes 的得分是它的 1.15 倍，Node `--jitless` 是 1.23 倍。对 moejs，它在 Crypto、RayTrace、EarleyBoyer 和 Splay 上领先，在 RegExp（moejs 1.3 倍）、NavierStokes（1.3 倍）、Richards 和 DeltaBlue 上落后。go-quickjs 只在 RegExp 上领先（2.6 倍）。liustack/slowjs 在 Splay 上领先（是其他各行的 1.4 到 1.6 倍）。

## 页面

这些包是 tifa 三个应用的服务端包，按 tifa main `ba584f71` 构建（Node 24.13.0，pnpm 10.33.0，Go 1.27.2）：bench 应用（`test/apps/bench`，2 个页面）、MUI 应用（`test/apps/mui`，3 个页面）和示例商店（`examples/shop`，20 个页面：工作台 6 个，en-US 和 zh-CN 的店面各 7 个）。它们在 `compare/engines/tifa/bundles` 里，SHA256 如下：

```
8df28d3d550cfeb198f00e7d165b6a4e8aa6f53cc3049a53006785cea14beebe  bench.js
5d3cbbb1d6b24d42b3b7bfd3f8e6df757d843fb9b4a51588fdb3159de9733770  mui.js
b0d5bc4bf6c795e6fe50de3beb10b4a55c7ece542c65dd01c71dcba3a8f6e439  shop.js
```

每个页面都按 tifa 渲染的方式渲染：用各应用的服务端为这个页面发来的输入（`tifa/in`）调用 `startRender(json)`，再跑 promise 任务，然后取结果。一个页面的时间是每次渲染的 CPU 时间（bench 页面热身 50 次后取 200 次计时渲染的平均，MUI 和商店热身 20 次后取 100 次），C QuickJS 和 Node 在脚本里用墙钟时间给渲染计时。每个引擎的 HTML 都写出来，在遮掉路由的渲染时间戳之后和我们的比。

## 重新运行

见 [`compare/engines/README.zh-CN.md`](../compare/engines/README.zh-CN.md)：命令、需要的机器、怎么重新构建包，以及怎么测。要重新跑对比的版本，打标签之前按 [releasing.md](releasing.zh-CN.md#和其他引擎的对比) 里的“和其他引擎的对比”做。


test262 只在 liustack/slowjs 上运行
，用仓库自己的检查（[testing.md](testing.zh-CN.md#test262)）：83426 个测试，33 个已知错误，每次运行都一样。
