<!-- Copyright (c) 2026 The liustack/slowjs Authors. -->
<!-- SPDX-License-Identifier: MIT -->

# compare/engines

[English](README.md) · 简体中文

liustack/slowjs 和其他 Go 的 JavaScript 引擎、以及三个原生引擎的对比。它是一个独立的 Go 模块，所以其他引擎不会进入根目录的 `go.mod`。结果和结论见 [docs/benchmarks.zh-CN.md](../../docs/benchmarks.zh-CN.md)。

## 比什么

两张表：

1. **渲染。** tifa 页面一次渲染的 CPU 时间：bench 应用（2 个页面）、MUI 应用（3 个）和商店（20 个），表示成相对 liustack/slowjs 的比值。
2. **V8 套件。** 每个引擎的得分，以及它 8 个子项各自的得分。

对比从不在其他引擎上运行 test262。

引擎集合的规则：**对比只在解释器之间进行。** 带 JIT 的引擎关掉 JIT 来跑，或者不比。Go 引擎有 liustack/slowjs、go-quickjs（JIT 默认关）和 moejs。原生参照是 C QuickJS 2026-06-04、Hermes V1（发布版，它没有 x86-64 的 JIT，照它的发布方式：用 `hermesc -O` 编好的字节码）和带 `--jitless` 的 Node。

每个引擎都照人们真实使用的方式运行。它有自己的 Intl 和 URL 就用自己的，没有就加载标准的 polyfill：Intl 用 FormatJS，URL、URLSearchParams、structuredClone 和迭代器辅助方法用 core-js。polyfill 只加载进缺这个功能的引擎（清单在 `tifa/run.sh`）。对比里的包不带 tifa 的 Go 函数（`__tifa_escape`、`__tifa_start_tag`、`__tifa_end_tag`、`__tifa_copy_props`、`__tifa_murmur2__`、`__tifa_random_bytes__`）、tifa 的 GC 规则和池的设置，也不共用字节码：没有哪个引擎注册它们，我们的也不例外，所以每个引擎跑的都是 React 自己的代码。

## 目录

| 路径 | 内容 |
| --- | --- |
| `cmd/compare` | 驱动程序：给每个引擎构建一个 `jsbench`，运行 V8 套件（`-phases v8`，默认值） |
| `cmd/jsbench` | 链接了一个引擎的程序：`v8`、`tifa-ssr` 和较早的负载 |
| `engine/` | 每个引擎一个适配器，放在构建标签（`eng_<name>`）后面 |
| `tifa/` | 三个服务端包（`bundles/`）、25 个页面输入（`in/`）、polyfill（`polyfill/`、`prelude/`）、`run.sh` 和 `pagerun.sh` |
| `native/` | `build.sh`（C QuickJS、Hermes、Node）和各原生引擎的驱动（`ssr-driver.js`、`node-ssr.cjs`、`hermes-ssr.js`、`hermes-run.sh`、`qjs-v8.js`、`hermes-v8.js`） |
| `cmd/pages` | 一次 `tifa/run.sh` 运行的页面表格和归档 |
| `cmd/modulescript`、`cmd/notices` | 把 tifa 的模块包变成脚本。生成 `tifa/THIRD_PARTY_NOTICES.md` |
| `results/` | 最近一次运行的文件 |

## 重新运行

在没有别的任务的四核 Linux 机器上，装有 Go、clang，以及 Node 和 npm（`native/build.sh` 带的那个 Node 就行）。运行测的是它所在的检出，所以先检出标签。

```sh
cd compare/engines
sh native/build.sh $HOME/jsnative                 # C QuickJS、Hermes、Node
export PATH=$HOME/jsnative/node-v24.22.3-linux-x64/bin:$PATH
tifa/polyfill/build.sh                            # 两个 polyfill 文件
go build -pgo=off -trimpath -o bin/compare ./cmd/compare
./bin/compare -build-only -bindir bin             # 每个引擎一个 jsbench
```

tifa 页面每次调用一个引擎，每次调用独占一个核，并用 `~/bin/quiet` 这类锁住整台机器（这里是 0 号核），然后出表：

```sh
run=<日期>-<版本>-<主机>
page_dir=$HOME/pages-$run
for e in liustack-slowjs go-quickjs-interp quickjs-2026-06-04 node-jitless hermes moejs; do
  TIFA_ONLY=$e TIFA_CPU=0 TIFA_SIBLING=3 tifa/run.sh $page_dir bin $HOME/jsnative
done
go run ./cmd/pages $page_dir --output results/$run-pages
```

V8 套件跑五轮，每一轮每一方各跑一次，顺序轮流变。Go 的各方在 `GOMAXPROCS=1 taskset -c 0` 下运行 `bin/<name> v8 -dir ../../internal/arewefastyet/v8-v7`，C QuickJS 运行 `qjs --std native/qjs-v8.js <dir>`，Hermes 运行 `hermes -Xhermes-internal-test-methods native/hermes-v8.js -- hbc <dir>`，文件先用 `hermesc -O` 编好。

机器是共用的话，运行可以切成几段：`-name` 建立一次运行，`-add` 往里加，`TIFA_ONLY` 挑页面运行的引擎。

## 重新构建包

`tifa/bundles/*.js` 和 `tifa/in/*/*.json` 来自 tifa 的一份一次性克隆（永远不要在 tifa 的工作副本里构建）：提交、Node、pnpm 和 Go 的版本以及每个包的 SHA256 在 [docs/benchmarks.zh-CN.md](../../docs/benchmarks.zh-CN.md#页面) 里。在克隆里，到 `test/apps/bench`、`test/apps/mui` 和 `examples/shop` 里运行 `go tool tifa build`，会写出各应用的 `.tifa/dist/ssr.js`。`go run ./cmd/modulescript <ssr.js> tifa/bundles/<app>.js` 把它变成脚本。输入是各应用的服务端为一个页面交给 `startRender` 的内容：[engine/pgo/README.zh-CN.md](../../engine/pgo/README.zh-CN.md) 的做法记录了它们（在 `internal/ssr` 里放一个 `zz_dump.go` 文件，设置 `TIFA_DUMP_INPUT`，每个页面请求一次，商店的工作台要设 `SHOP_OPEN=1`）。`go run ./cmd/notices <tifa 克隆> tifa` 重新写第三方声明，`shasum -a 256` 写 `SHA256SUMS`。

## 怎么测

- 每个引擎链接进自己的二进制（`jsbench`，构建标签 `eng_<name>`），用 `CGO_ENABLED=0 -pgo=off -trimpath` 构建，所以没有哪个引擎和别的引擎共用进程、画像或垃圾回收器。
- 每个计时的进程都在一个核上运行（`taskset -c`），GOMAXPROCS=1，TZ=UTC，内存限制 4 GiB，整台机器为它独占。进程拿到的 CPU 时间不到这次运行的 90% 时，`pagerun.sh` 会把一组页面再跑一遍，最多三次。
- Go 引擎一个页面的时间是每次渲染的 CPU 时间（bench 页面热身 50 次后取 200 次计时渲染的平均，MUI 和商店热身 20 次后取 100 次）。C QuickJS 和 Node 在脚本里用独占一个核的墙钟时间给渲染计时。Hermes 的 shell 没有比毫秒更细的时钟，所以它的时间是整个计时循环的墙钟时间除以渲染次数。
- 每个页面的 HTML 都写出来，在遮掉路由的渲染时间戳（`u:` 加 13 位数字）之后和我们的比。`cmd/pages` 列出每个不同的页面和第一处不同的位置，好分清不同的是 Intl 的输出还是别的地方。
