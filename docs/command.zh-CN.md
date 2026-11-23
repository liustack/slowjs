---
summary: 'slowjs 命令：它用来做什么（在引擎上跑脚本、展示问题、不装 Go 试用某个版本），怎么获取，脚本里有什么（Intl、Temporal、URL、fmt、print、console、scriptArgs），退出码，超时和内存上限的默认值及理由，它有意不做什么（定时器、模块、REPL、文件、网络、报告未处理的拒绝），它的大小和测试'
read_when:
  - 不写 Go 就在这个引擎里运行 JavaScript 文件，或者用一个脚本展示引擎的问题
  - 修改 cmd/slowjs、它的选项或 release 压缩包
  - 想知道 slowjs 为什么没有定时器、模块、REPL 或 Node API
translatedFrom: d72c13228058
---

# slowjs 命令

[English](command.md) · 简体中文

`slowjs` 在这个引擎里运行一个 JavaScript 文件，VM 的设置和 tifa 一样，不用写 Go 程序。它用来做三件事：

- **展示问题。** 一个脚本，加上 `slowjs` 为它打印的内容，就是完整的报告：引擎就是我们发布的那个，设置也和我们用的一样。
- **在引擎上跑脚本**，看它给出什么结果、要多久、要多少内存。
- **不装 Go 试用某个版本。** 从 v0.31.0 起，每个版本的 GitHub release 都带有预先构建好的它，覆盖 linux 和 darwin 的 amd64 与 arm64，以及 windows/amd64，从 v0.31.12 起也有 windows/arm64。

代码在 `cmd/slowjs`。

## 获取

从[最新的 release](https://github.com/liustack/slowjs/releases/latest) 下载你系统对应的压缩包，用 `checksums.txt` 核对，然后解压。压缩包按 Go 对目标平台的叫法命名：`slowjs-linux-amd64.tar.gz`、`slowjs-linux-arm64.tar.gz`、`slowjs-darwin-amd64.tar.gz`（macOS，Intel）、`slowjs-darwin-arm64.tar.gz`（macOS，Apple 芯片）、`slowjs-windows-amd64.zip` 和 `slowjs-windows-arm64.zip`。

```sh
curl -LO https://github.com/liustack/slowjs/releases/latest/download/slowjs-linux-amd64.tar.gz
curl -LO https://github.com/liustack/slowjs/releases/latest/download/checksums.txt
sha256sum -c --ignore-missing checksums.txt
tar -xzf slowjs-linux-amd64.tar.gz
./slowjs-linux-amd64/slowjs --version
```

每个压缩包里有一个文件夹，放着二进制和它必须附带的许可证文本（[NOTICE.md](../NOTICE.md#what-a-program-that-ships-a-binary-must-include)）。二进制是静态的，用 `CGO_ENABLED=0` 构建：linux 版在 glibc 和 musl 系统上都能跑。浏览器下载的二进制没有开发者签名，macOS 会阻止它运行：`curl` 不会打上这种标记，`xattr -d com.apple.quarantine slowjs` 可以去掉它。

## 运行文件

```sh
slowjs [flags] file.js [args...]
slowjs [flags] -e code [args...]
```

| 选项 | 作用 |
| --- | --- |
| `-e code` | 运行 `code`，不运行文件 |
| `--timeout d` | 运行 `d` 之后结束，比如 `30s` 或 `2m`。`0` 表示永不结束。默认 `1m` |
| `--memory-limit n` | VM 占用超过 `n` 字节时结束运行。`K`、`M` 和 `G`（或 `KiB`、`MiB`、`GiB`）是 1024、1024² 和 1024³。`0` 表示不设上限。默认 `1GiB` |
| `--version` | 在一行里打印 `slowjs <version> <go release> <os>/<arch>` 后退出，比如 `slowjs v0.31.0 go1.27.2 linux/amd64` |
| `-h`、`--help` | 打印选项说明后退出 |

文件在一个新的 VM 里作为全局脚本（不是模块）运行，然后运行它排进队列的 promise 任务，所以 `then` 回调和 async 函数都会执行完。

## 脚本里有什么

VM 的设置和 tifa 的渲染器设置它每个 VM 的一样，顺序也一样，在设好超时之后：

- 调用最多可以嵌套 10000 层，这是 tifa 的默认值，也是引擎允许的最大值。
- `URL` 和 `URLSearchParams`（包 `url`）。
- `Intl`，覆盖 Chromium 有的所有语言，以及用到它的 `toLocaleString` 方法和 `localeCompare`（包 `intl`）。
- `Temporal`（包 `temporal`）。

然后是 tifa 没有的：

- `fmt`：Go 的 `fmt.printf`、`fmt.println`、`fmt.sprintf` 和 `fmt.sprint`（包 `fmt`）。`printf` 和 `println` 写到 stdout。
- `print(...args)` 和 `console.log(...args)`：每个参数像 `String(x)` 那样变成字符串，用空格连起来，再加一个换行，写到 stdout。对象打印成 `[object Object]`。两者单独拿出来调用也能用，比如 `var p = console.log; p(1)`。
- `console.error(...args)`：同上，写到 stderr。
- `scriptArgs`：选项之后的参数：文件以及它后面的参数，用 `-e` 时只有后面的参数。

脚本也能看到 tifa 给它的打包文件准备的、名字是 `__tifa_*` 的那些函数，因为命令用 `tifa.Register` 设置 VM（[api.zh-CN.md](api.zh-CN.md#让-react-渲染更快)）。

每次调用都会立刻写出它的那一行，并且写 stderr 的一行之前，会先把 stdout 写出去，所以终端或管道在脚本打印这些行的时候就能看到它们，顺序也不变。单独的代理项（lone surrogate）打印成 U+FFFD。

未捕获的异常会把它的文本和调用栈打印到 stderr，最多 50 帧。调用栈里的文件名就是命令行上给出的路径，整理成一行，最多 1,024 字节，`-e` 的代码写作 `<eval>`。

## 展示问题

1. 把脚本尽量删短，只要问题还在。
2. 运行它：`slowjs repro.js; echo $?`。
3. 报告脚本、命令、它打印的全部内容、退出码，以及 `slowjs --version` 打印的那一行（bug 报告模板会要这些）。

用默认的上限，卡住不动的复现脚本一分钟后以退出码 3 结束，无休止分配内存的复现脚本在 1 GiB 时以退出码 4 结束，所以从来不用手动杀掉一次运行。

## 退出码

| 代码 | 什么时候 |
| --- | --- |
| 0 | 脚本和它的任务都跑完了 |
| 1 | 未捕获的异常，包括语法错误 |
| 2 | 命令行写错了，或者读不了文件。什么都没运行 |
| 3 | 运行超过了 `--timeout` |
| 4 | VM 超过了 `--memory-limit` |

Go 自身崩溃也以 2 结束，并打印 `panic:` 或 `fatal error:` 和各个 goroutine。

## 上限

超时或内存上限一到，不管脚本做什么，运行都会结束：之后不会再运行任何 `catch`、`finally` 或 promise 处理函数，和 Go API 的每次调用一样（[uncatchable.md](changes/uncatchable.md)）。

- **超时**管的是整次运行，脚本加上它的任务，而不是分别计算。默认一分钟，是给运行脚本并等着它的人用的：卡住的脚本会在人还在等的时候结束，并给出一条写明这个选项的消息。tifa 的页面几毫秒就渲染完，tifa 在 3 秒后结束一次渲染，所以一分钟足够跑比页面重得多的脚本。要跑更久，设 `--timeout 0` 或更长的时间。
- **内存上限**算的是 VM 占用的内存：它的对象、字符串和引擎自己的结构。默认 1 GiB，在机器耗尽内存、开始用交换区之前，让无休止分配内存的脚本以一个错误停下。tifa 给每个 VM 128 MiB，所以 1 GiB 能容下的脚本，是页面最大允许大小的八倍。需要更多内存的运行，设 `--memory-limit 0` 或更大的值。低于 128 KiB 的上限按 128 KiB 算。
- `print` 和 `console` 把一行分成每段最多 65,536 个 UTF-16 码元交给 Go，所以 Go 为写出它而做的拷贝每次最多 192 KiB，这部分不算在上限里，不管脚本在上限之内造出的字符串有多长。`fmt` 自己在上限之内限制它的输出（见 `fmt.Register` 的文档注释）。同样不算在上限里的还有：源文件（Go 在运行前把它整个读进来），以及 Go 运行时的内存。

## 有意不做的

命令运行的就是引擎在 tifa 和 Go 程序里运行的东西，需要宿主程序另外加上的一概没有：

- **没有定时器。** `setTimeout` 和 `setInterval` 是 ReferenceError。tifa 的渲染不跑定时器，Go API 的 VM 也没有定时器，所以两者的问题不用定时器也能展示出来。promise 任务照常运行。
- **没有 ES 模块。** 在全局脚本里 `import` 和 `export` 是语法错误，而且 Go API 从 v0.27.0 起就没有模块加载器（[go-api.md](changes/go-api.md#no-module-loader-shell-helpers-or-setcanblock)）。
- **没有 REPL，也不读 stdin。** 既没有文件也没有 `-e` 时，它打印用法，并以 2 退出。
- **没有文件系统、网络或 Node API。** 没有 `load`、`require`、`process`、`fs` 或 `fetch`。它是引擎，不是 Node。
- **不报告未处理的 promise 拒绝。** 一个被拒绝、又没有任何处理函数的 promise，运行照样以 0 结束，因为这个命令不让 VM 记录它们（`VM.TrackRejections`）。

二进制的大小，按 release 的方式构建（`-trimpath -ldflags "-s -w"`，Go 1.27.2），单位 MiB：

| 目标平台 | slowjs |
| --- | --: |
| linux/amd64 | 19.6 |
| linux/arm64 | 18.5 |
| darwin/amd64 | 19.7 |
| darwin/arm64 | 18.7 |
| windows/amd64 | 19.9 |
| windows/arm64 | 18.7 |

大部分是 `intl` 的语言数据。压缩包是 8.3 到 8.9 MiB。

## 测试

`go test ./cmd/slowjs` 构建一个写入了版本号的命令，并在一些小脚本上运行它：`print` 和 `console` 写出什么、按什么顺序，一个代理对跨在两段之间的长字符串，`scriptArgs`，带调用栈的未捕获异常，语法错误，promise 任务，`Intl`、`Temporal`、`URL` 和 `fmt`，调用深度，超时结束一个会 catch 的死循环（以及一次任务超过超时的运行），内存上限结束一个不停分配内存的循环，默认值，`--version`、`-h` 和写错的命令行。`TestV8Suite`（`-short` 会跳过它）通过命令、不设超时地跑一个用到引擎很多部分的长脚本，并拿它打印的内容和 Go API 的 VM 对比。

设了 `SLOWJS_BIN` 和 `SLOWJS_VERSION` 时，测试运行那个二进制，而不是自己构建一个，并期望它是那个版本。发版任务就是这样，在它构建的每个二进制自己的系统上跑这些测试（[releasing.md](releasing.md#the-release-job)）。
