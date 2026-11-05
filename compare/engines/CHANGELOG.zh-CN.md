# 引擎对比的变更

[English](CHANGELOG.md) · 简体中文

## 2026-11-06：更新的 go-quickjs 和 modernc，一对核加锁

- go-quickjs 是 v0.34.1（之前是 v0.28.0），modernc.org/quickjs 是 v0.27.0
  （之前是 v0.25.0），都来自 `go get <module>@latest`。goant（v0.2.1）和
  moejs（v0.1.0-alpha.11）本来就是最新的。适配器不用改。更早的结果文件里的数字是旧版本的。
- 2026-11-06 的运行在和其他会话共用的 GCP 机器上做：计时的负载和 tifa 页面
  在 CPU 0 和它的同伴核 4 上分成短的几段运行，一次一段，加锁
  （驱动程序用 `-add`，`tifa/run.sh` 用 `TIFA_ONLY`、`TIFA_CPU=0` 和 `TIFA_SIBLING=4`），
  test262 在 CPU 2 上以低优先级运行，再合并进来。测量的
  方法没有变。
- go-quickjs-jit 在页面运行里把 bench 的 `table` 页面在两次
  渲染之间渲染得不一样（`output changed between renders`），这个页面就没有了
  时间。这个页面单独运行，写进
  `go-quickjs-jit-bench-rest.json`，`cmd/pages` 会读它，
  表里用的是通过的那次运行。
- C QuickJS 2026-06-04 有了 V8 套件的一行：`native/qjs-v8.js` 按 `runV8` 的步骤
  在 `qjs` 上跑这个套件。它是单独一场跑的，和 liustack-quickjs 轮流跑
  （[2026-11-06-v0.30.1-gcp-v8-c](results/2026-11-06-v0.30.1-gcp-v8-c.md)）。

## 2026-11-05：行又叫 liustack-quickjs 了

- 模块又是 `github.com/liustack/quickjs` 了，它的行取回了模块还是 `github.com/liustack/slowjs` 之前的名字：
  `slowjs` 现在是 `liustack-quickjs`，`slowjs-tifa` 是 `liustack-tifa`，在
  结果、报告、`TIFA_ONLY`、`-engines` 和 `tifa/run.sh` 写出的文件名里都是。
  测量没有任何不同。
- 模块还是 `github.com/liustack/slowjs` 时生成的结果文件
  （2026-10-24 的 v0.28.6 PGO 页面运行和 2026-10-29 的运行）保留
  名字 `slowjs` 和 `slowjs-tifa`。`-report` 和 `cmd/behind` 两种名字都读。用 `-add` 或 `-merge` 往
  这些运行里追加，会让模块有两行，所以请新开一次运行。

## 2026-10-29：C 引擎，以及改用 Go 的辅助工具

- slowjs 的行运行 C 引擎（v0.29.0）。`slowjs-tifa-nogoasm` 这一行没有了：现在只有一个解释器，它会重复 `slowjs-tifa`。更早的结果文件保留它。
- 辅助工具是从 `compare/engines` 运行的 Go 程序：`cmd/behind`（远远落后的列表）、`cmd/pages`（页面表和存档）、`cmd/modulescript`（把 tifa 的模块 bundle 变成脚本）和 `cmd/notices`（第三方声明）。它们取代了同名的 Python 文件，对同样的输入写出同样的输出。测量的方法没有变。

## 2026-10-28：moejs 搬家

- moejs 现在是 `github.com/MoeForge/moejs`，v0.1.0-alpha.11（之前是 `github.com/Calcium-Ion/moejs`，v0.1.0-alpha.9）。更早的结果文件里的数字是 alpha.9 的。C QuickJS 的行保持它们 Makefile 的构建，没有 `-DNDEBUG`（见 `docs/benchmarks.md`）。

下面是对比的测量方式有哪些变化，好让不同次运行的数字可以并排来读。`results/` 里一次运行的文件会说明
是哪个版本的对比做出了它（这个仓库的提交）。

## 2026-10-21：第一版

- 引擎：liustack/quickjs、modernc.org/quickjs、go-quickjs（和它的 JIT）、
  jsc2go（带 JIT 和不带 JIT）、goant（带 JIT 和不带 JIT）、
  fastschema/qjs。只收能用 `CGO_ENABLED=0` 构建、并且能跑
  当前 JavaScript 的引擎：goja、sobek 和 otto 没有收，因为它们缺
  async generator、WeakRef、较新的正则表达式标志等等
  （otto 只实现 ES5）。
- 带 JIT 的引擎运行两次，以设置命名：go-quickjs-interp
  和 go-quickjs-jit，jsc2go-jit 和 jsc2go-interp，goant-interp 和
  goant-jit。两个都运行每个负载，包括 test262。
- 每个引擎链接进它自己的二进制（`jsbench`，构建标签
  `eng_<name>`），用 `-pgo=off -trimpath` 构建。
- 负载：React 19 对一个 717 个元素页面的 `renderToString`，V8
  基准套件第 7 版，JSON.parse，JSON.stringify，字符串构建，
  正则表达式，`compare/testdata` 里的 QuickJS 微基准，
  冷启动，每个 VM 的内存（100 个 VM 同时存活），特性探测，test262。
- QuickJS 微基准每个测试一个进程，这样一个
  超出内存上限的测试只损失它自己。
- 计时的负载：5 轮，每轮引擎的顺序不同，
  GOMAXPROCS=1。在 Linux 上固定到一个 CPU，如果它的超线程
  同伴核忙了至少 3 个时钟 tick 并且超过运行时间的 5%，或者
  引擎进程拿到的 CPU 时间不到它墙钟时间的 90%
  （另一个进程跑在了它的 CPU 上），这次运行就重来。进程 CPU 时间
  和墙钟时间一起报告。
- `d53ebc8b` 的 test262：language、built-ins 和 intl402，不含模块
  测试和需要宿主钩子的测试。每次运行用新 VM，同一时刻每个引擎
  一个进程。它可以在另一个 CPU 上以低优先级运行，
  作为单独的一次运行，再合并进主运行。
- 每个引擎进程都在内存上限下运行：test262 是 2 GiB，其余
  是 4 GiB。在 Linux 上这个上限是 cgroup（systemd-run），它也
  计入 Go 堆以外的内存。一个 test262 测试如果单独运行时超出上限，
  就算内存不足。
- 有能在任何 VM 上运行的程序的引擎（liustack、modernc、go-quickjs、
  jsc2go、fastschema），冷启动和内存都从一个编译好的
  程序加载，冷启动还会检查这样的 VM 渲染出同样的页面。
  jsc2go 是在 2026-10-21 的运行期间开始这样做的：它在那里的冷启动和
  内存两行来自用那个适配器的重跑。
- fastschema 的运行时在它们的 context 取消之前关闭。
  先取消的话，`Runtime.Close` 会 panic 并留下内存。
  2026-10-21 的 test262 运行在这之前，所以 fastschema-qjs 在那里的内存
  重启比现在要多。
- tifa 页面（`tifa/run.sh`）：三个 tifa 应用的服务端 bundle，
  在每个引擎上渲染。不带 Intl 的页面原样运行，商店
  页面在固定的 FormatJS polyfill 上运行（去掉原生 Intl），有自带 Intl 的
  引擎还在它上面运行。C QuickJS 2026-06-04 和
  master，QuickJS-NG v0.17.0 和 Node v24.11.1（带 JIT 和不带 JIT）
  作为参照行渲染同样的页面，不是竞争者。

## 2026-10-21：去掉 jsc2go 和 fastschema/qjs

- 不再对比：fastschema/qjs，因为它在大多数负载上最慢，它的 tifa 渲染一直没有结束，白白花了几个小时的机器时间。jsc2go，因为它在这里意义不大（没有 Intl，没有内存限制）。被去掉的行来自 v0.26.16 上 2026-10-21 的运行。它们的原始结果没有保留在这里，所以不对它们做数值比较。

## 2026-10-21：加入 moejs

- moejs（github.com/Calcium-Ion/moejs，v0.1.0-alpha.9）加入对比。它没有 Intl，所以在 FormatJS polyfill 上渲染 tifa 商店。它的调用深度固定为 512：因调用深度而失败的页面或测试，算作 moejs 的限制，这个限制不会调高。运行时用可变的内建对象创建，因为 test262、polyfill 和 tifa 的 bundle 都会写它们。

## 2026-10-21：go-quickjs v0.28.0

- go-quickjs 从 v0.27.0 升到 v0.28.0（按它的发布说明，API 没有变化，有新的 JIT 和更快的解释器），用于 liustack/quickjs v0.26.17 的那次运行。go-intl 保持在 v0.6.0，这是 go-quickjs 要求的版本。
- 在 tifa 商店上，当一个引擎的大多数页面失败时，先把每个页面单独放在自己的进程里渲染，找出失败的页面，然后像以前一样，在不含它们的一次运行里测量其余的页面。

## 2026-10-22：存档页面是否相同，并对齐运行器

- 页面导出现在保留规范化后的 HTML SHA256 摘要和每个页面是否相同的
  结论，连同原始结果、补跑的运行、探测和尝试。已有的
  两次运行保留它们原来的数据和运行时的相同性结论。
- tifa 的路径从调用者的目录解析。功能检查现在用
  和计时同样的带上限、固定核的入口和 UTC 环境。被干扰的
  进程最多一共尝试三次。
- 生成的报告去掉末尾的空白和空行。
  bundle 的字节不变，因为它们的空白出现在字符串里。
- bundle 的包版本和许可证文本在 `tifa/THIRD_PARTY_NOTICES.md`。
  文档里没有依据的精确剖析和历史说法已删掉。

## 2026-10-24：按 tifa 的方式运行的 slowjs，以及 MUI 页面

- tifa 页面运行里的新行 `liustack-tifa`：slowjs 按 tifa
  设置它池里 VM 的方式配置（包 `intl` 和 `temporal`，没有 polyfill，
  `__tifa_murmur2__` 和 `__tifa_random_bytes__` 作为 Go 函数，128 MiB 的内存
  限制，10000 个槽位的栈，3 s 的时间预算，tifa 的 GC
  规则，共用一个编译好的 bundle）。这段 Go 从 tifa 的 `internal/ssr` 复制而来，
  不是导入。`liustack-quickjs` 保留它的行，和每个引擎一样，在
  polyfill 上跑同样的 JavaScript。tifa 行有 bench、商店和
  MUI 的运行，用它自己的 Intl，没有 polyfill 的运行，而 C QuickJS 和 Node 的
  行没有对应的行：它们不能接收 Go 函数或 Go 的 `Intl`。
  文档说明了每一行测的是什么（docs/benchmarks.md，「slowjs as tifa runs
  it」）。
- 有 `@emotion/hash` 的 bundle（MUI）在适配器里得到 tifa 插件
  对 `murmur2` 做的那一行改动，因为固定的 bundle 是在插件有这个改动
  之前构建的。它的结果文件会统计改写的次数和这些 Go 函数
  的调用次数。
- bundle 在一次渲染之后的工作（年轻代回收，按池的规则做的完整 GC）
  算在这一行每次渲染的 CPU 时间里，不算在它的墙钟时间里，和 tifa 里一样。
- MUI bundle 第一次计时，每个引擎都在 polyfill 上，有自带 Intl 的
  也在自带 Intl 上（`mui-pf`、`mui-native`）。
- `pages.py` 把 tifa 行的 HTML 和 polyfill 行的比较，统计
  只在 U+202F 上有差异的页面（polyfill 在 AM 和 PM 之前写它，
  Go 的 Intl 和 Node 写普通空格），并把每个 bundle 表示成对
  C QuickJS 2026-06-04 的比值。
- `tifa/run.sh` 接受 `TIFA_ONLY`（只运行一些引擎），`pagerun.sh` 接受
  `TIFA_CPU` 和 `TIFA_SIBLING`（用 1 和 5 以外的另一对 CPU）。
- `cmd/compare` 和其他引擎一起构建 tifa 行，除非 `-engines` 点名，
  否则在其他负载里不运行它。
- 2026-10-24 的运行（`results/2026-10-24-main-tifa-row-gcp-pages`）只覆盖
  五行的 tifa 页面。其他引擎没有重新运行。

## 2026-10-24：今天的 tifa bundle，以及不带 Go 汇编的 slowjs

- `tifa/run.sh` 接受 `TIFA_BUNDLES=<dir>`，一个布局和 `tifa/` 一样的目录
  （`bundles/<app>.js`、`in/<app>/*.json`），用来运行 tifa 现在构建的
  bundle，代替固定的那些。固定的 bundle 和它们的运行保持
  原样。
- 自 tifa 6e062d6 起，tifa 把它的 SSR bundle 构建成 ES 模块。
  `liustack-tifa` 的行像 tifa 一样把 `bundles/<app>.mjs` 当作模块编译，并像 tifa 一样
  给 VM 提供 `__tifa_async_context__`。其他每一行
  运行 `bundles/<app>.js`，它由 `tifa/module-script.py` 从模块生成：
  去掉最后的 `export{};`，其余放进一个 strict 函数。
- 新行 `liustack-tifa-nogoasm`：带 `slowjs_nogoasm` 构建的
  `liustack-tifa` 行，就像 tifa 用 `build.goasm: false` 构建应用那样，
  所以在 linux/amd64 上是 Go 写的解释器，不是 Go 汇编写的那个。
- Node 的行在 bundle 运行之前，把 Node 在 `globalThis` 上的
  getter（`crypto`、`navigator`）改成普通值，因为新 bundle 的宿主
  补丁会给它们赋值，并加上 `Iterator.concat`，它是
  TanStack Router 调用的，而 Node v24.11.1 没有。

## 2026-10-24：行叫 slowjs

- slowjs 的行取模块的名字：`liustack-quickjs` 现在是
  `slowjs`，`liustack-tifa` 是 `slowjs-tifa`，`liustack-tifa-nogoasm` 是
  `slowjs-tifa-nogoasm`，在结果、报告、`TIFA_ONLY`、`-engines`
  和 `tifa/run.sh` 写出的文件名里都是。测量没有任何不同。
- 在此之前生成的结果文件保留旧名字。`-report` 和
  `behind.py` 两种都读。用 `-add` 或 `-merge` 往旧的运行里追加，会
  让 slowjs 有两行，所以请新开一次运行。
