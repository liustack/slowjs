---
summary: 'slowjs 命令：用来做什么（在引擎上跑脚本、复现问题、不装 Go 试用发布版），怎么获取，脚本里有什么（Intl、Temporal、URL、fmt、print、console、scriptArgs），退出码，超时和内存上限的默认值及理由，故意不做的（定时器、模块、REPL、文件、网络、未处理拒绝的报告），体积和测试'
read_when:
  - 不写 Go 就在这个引擎里跑一个 JavaScript 文件，或用一个脚本复现引擎的问题
  - 修改 cmd/slowjs、它的参数或发布压缩包
  - 被问到 slowjs 为什么没有定时器、模块、REPL 或 Node API
---

# slowjs 命令

[English](command.md) · 简体中文

`slowjs` 在这个引擎里运行一个 JavaScript 文件，虚拟机按 tifa 的方式配置，不用写 Go 程序。它用来做三件事：

- **复现问题。** 一个脚本加上 `slowjs` 对它的输出就是完整的报告：引擎就是我们发布的那个，配置也和我们用的一样。
- **在引擎上跑脚本**，看它给出什么、花多久、要多少内存。
- **不装 Go 试用发布版。** 从 v0.31.0 起，每个版本的 GitHub release 里都有预编译好的二进制，覆盖 amd64 和 arm64 上的 linux 和 darwin，以及 windows/amd64。

代码在 `cmd/slowjs`。

## 获取

从[最新 release](https://github.com/liustack/quickjs/releases/latest) 下载你系统的压缩包，用 `checksums.txt` 校验后解压。压缩包按 Go 的目标名命名：`slowjs-linux-amd64.tar.gz`、`slowjs-linux-arm64.tar.gz`、`slowjs-darwin-amd64.tar.gz`（Intel 芯片的 macOS）、`slowjs-darwin-arm64.tar.gz`（Apple 芯片的 macOS）和 `slowjs-windows-amd64.zip`。

```sh
curl -LO https://github.com/liustack/quickjs/releases/latest/download/slowjs-linux-amd64.tar.gz
curl -LO https://github.com/liustack/quickjs/releases/latest/download/checksums.txt
sha256sum -c --ignore-missing checksums.txt
tar -xzf slowjs-linux-amd64.tar.gz
./slowjs-linux-amd64/slowjs --version
```

每个压缩包里是一个文件夹，放着二进制和它必须附带的许可证文本（[NOTICE.md](../NOTICE.md#what-a-program-that-ships-a-binary-must-include)）。二进制是静态的，用 `CGO_ENABLED=0` 构建：linux 版在 glibc 和 musl 系统上都能跑。macOS 会拦下浏览器下载的、没有开发者签名的二进制：`curl` 不会打这个标记，`xattr -d com.apple.quarantine slowjs` 可以去掉它。

## 运行文件

```sh
slowjs [flags] file.js [args...]
slowjs [flags] -e code [args...]
```

| 参数 | 作用 |
| --- | --- |
| `-e code` | 运行 `code` 而不是文件 |
| `--timeout d` | 运行 `d` 之后结束，如 `30s` 或 `2m`。`0` 表示永不结束。默认 `1m` |
| `--memory-limit n` | 虚拟机占用超过 `n` 字节时结束运行。`K`、`M`、`G`（或 `KiB`、`MiB`、`GiB`）分别是 1024、1024² 和 1024³。`0` 表示不限。默认 `1GiB` |
| `--version` | 在一行里打印 `slowjs <版本> <Go 版本> <系统>/<架构>` 后退出，如 `slowjs v0.31.0 go1.27.2 linux/amd64` |
| `-h`、`--help` | 打印参数说明后退出 |

文件作为全局脚本（不是模块）在一个新虚拟机里运行，之后运行它排队的 promise 任务，所以 `then` 回调和 async 函数会跑完。

## 脚本里有什么

虚拟机按 tifa 渲染器配置每个虚拟机的方式配置，顺序相同，在设置超时之后：

- 调用最多可以嵌套 10000 层，这是 tifa 的默认值，也是引擎允许的最大值。
- `URL` 和 `URLSearchParams`（`url` 包）。
- `Intl`，含 Chromium 的全部语言区域，以及用到它的 `toLocaleString` 系列方法和 `localeCompare`（`intl` 包）。
- `Temporal`（`temporal` 包）。

另外还有这些，tifa 没有：

- `fmt`：Go 的 `fmt.printf`、`fmt.println`、`fmt.sprintf` 和 `fmt.sprint`（`fmt` 包）。`printf` 和 `println` 写到 stdout。
- `print(...args)` 和 `console.log(...args)`：每个参数按 `String(x)` 转成字符串，用空格连接，加换行，写到 stdout。对象打印为 `[object Object]`。两者单独调用也能用，如 `var p = console.log; p(1)`。
- `console.error(...args)`：同上，写到 stderr。
- `scriptArgs`：参数之后的命令行参数：文件名和它后面的参数，用 `-e` 时只有后面的参数。

tifa 为自己的 bundle 加的东西这里没有：它那些名为 `__tifa_*` 的函数（随机字节、哈希、React 输出的文本转义和标签、props 拷贝）。它们属于 tifa，不属于引擎。

每次调用立刻写出它的那一行，写 stderr 的一行之前先写完 stdout，所以终端或管道看到的行和脚本打印的时间、顺序一致。单独的代理项打印为 U+FFFD。

未捕获的异常把文本和调用栈打印到 stderr，最多 50 帧。栈里的文件名就是命令行上给的路径，清理成一行、最多 1,024 字节，`-e` 的代码则叫 `<eval>`。

## 复现问题

1. 把脚本删减到仍能复现问题的最小程度。
2. 运行：`slowjs repro.js; echo $?`。
3. 报告里写上脚本、命令、全部输出、退出码和 `slowjs --version` 的那一行（bug 报告模板会要这些）。

按默认上限，卡住的复现脚本一分钟后以退出码 3 结束，无限分配内存的在 1 GiB 时以退出码 4 结束，不用手动去杀进程。

## 退出码

| 退出码 | 何时 |
| --- | --- |
| 0 | 脚本和它的任务都跑完了 |
| 1 | 未捕获的异常，包括语法错误 |
| 2 | 命令行有误，或文件读不了。什么都没运行 |
| 3 | 运行超过了 `--timeout` |
| 4 | 虚拟机超过了 `--memory-limit` |

Go 本身崩溃也以 2 结束，并打印 `panic:` 或 `fatal error:` 和各个 goroutine。

## 上限

超时或内存上限会结束运行，不管脚本做什么：之后不会再运行 `catch`、`finally` 或 promise 处理函数，和 Go API 的每次调用一样（[uncatchable.zh-CN.md](changes/uncatchable.zh-CN.md)）。

- **超时**管整个运行，脚本加它的任务，不是分别计算。默认一分钟，是给手动跑脚本并等着看结果的人用的：卡住的脚本在人还在等的时候就结束，并给出指明参数的消息。tifa 的页面渲染只要几毫秒，tifa 在 3 秒时结束一次渲染，所以一分钟足够跑比页面重得多的脚本。更长的运行用 `--timeout 0` 或更长的时间。
- **内存上限**计算虚拟机占用的内存：对象、字符串和引擎自己的结构。默认 1 GiB，让无限分配的脚本在出错时停下，不至于把机器的内存耗尽而开始换页。tifa 给每个虚拟机 128 MiB，所以 1 GiB 能容下比页面大八倍的脚本。需要更多的运行用 `--memory-limit 0` 或更大的值。低于 128 KiB 的上限按 128 KiB 算。
- `print` 和 `console` 把一行分成最多 65,536 个 UTF-16 单元的片段交给 Go，所以 Go 为写出而做的拷贝每次最多 192 KiB，在上限之外，不管脚本在上限内造出的字符串有多长。`fmt` 自己在上限内约束它的输出（见 `fmt.Register` 的文档注释）。同样在上限之外的还有：源文件（Go 在运行前整个读入）和 Go 运行时的内存。

## 故意不做的

这个命令运行的就是引擎在 tifa 和 Go 程序里运行的东西，不加任何需要宿主补上的功能：

- **没有定时器。** `setTimeout` 和 `setInterval` 会报 ReferenceError。tifa 的渲染不跑定时器，Go API 的虚拟机也没有，所以两者的问题不用定时器就能复现。promise 任务照常运行。
- **没有 ES 模块。** `import` 和 `export` 在全局脚本里是语法错误，Go API 从 v0.27.0 起就没有模块加载器（[go-api.zh-CN.md](changes/go-api.zh-CN.md#没有模块加载器shell-辅助函数和-setcanblock)）。
- **没有 REPL，也不读 stdin。** 没给文件也没给 `-e` 时，打印用法并以 2 退出。
- **没有文件系统、网络或 Node API。** 没有 `load`、`require`、`process`、`fs` 或 `fetch`。它是引擎，不是 Node。
- **不报告未处理的 promise 拒绝。** 没人处理的被拒绝 promise 不影响退出码，运行以 0 结束，因为 Go API 目前还没有办法得知。

二进制大小，按发布的方式构建（`-trimpath -ldflags "-s -w"`，Go 1.27.2），单位 MiB：

| 目标 | slowjs |
| --- | --: |
| linux/amd64 | 19.0 |
| linux/arm64 | 18.0 |
| darwin/amd64 | 19.1 |
| darwin/arm64 | 18.2 |
| windows/amd64 | 19.3 |

大部分是 `intl` 的语言区域数据。压缩包是 8.1 to 8.7 MiB。

## 测试

`go test ./cmd/slowjs` 构建一个打上版本号的命令，用小脚本运行它：`print` 和 `console` 写出什么、按什么顺序，代理对跨两个片段的长字符串，`scriptArgs`，带调用栈的未捕获异常，语法错误，promise 任务，`Intl`、`Temporal`、`URL` 和 `fmt`，调用深度，超时结束一个会 catch 的死循环（以及任务超时的运行），内存上限结束一个不断分配的循环，默认值，`--version`、`-h` 和错误的命令行。`TestV8Suite`（`-short` 时跳过）不设超时，通过命令运行一个用到引擎大部分功能的长脚本，并和 Go API 虚拟机的结果对照。

设了 `SLOWJS_BIN` 和 `SLOWJS_VERSION` 时，测试不构建，改为运行那个二进制，并期望那个版本号。发布任务就这样在各自的系统上测试它构建的每个二进制（[releasing.zh-CN.md](releasing.zh-CN.md#发布任务)）。
