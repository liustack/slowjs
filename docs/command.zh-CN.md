---
summary: 'slowjs 命令：怎么拿到预编译的版本，怎么运行一个文件，脚本里有什么（print、console.log、console.error、scriptArgs），退出码，超时和内存上限的默认值及理由，基准测试工具怎么运行它，有意不做的东西（REPL、模块、stdin、文件、网络、定时器、Node、Intl），带不带 Intl 的二进制大小，以及它的测试'
read_when:
  - 不写 Go，想在这个引擎里运行一个 JavaScript 文件，或者在命令行给引擎跑基准
  - 修改 cmd/slowjs、它的参数或它的发布压缩包
  - 被问到 slowjs 为什么没有 Intl、模块、REPL 或 Node 的 API
---

# slowjs 命令

[English](command.md) · 简体中文

`slowjs` 在这个引擎里运行一个 JavaScript 文件，让你不写 Go 也能试用它、给它跑基准。从 v0.31.0 起，每个版本的 GitHub release 里都有预编译好的它，覆盖 amd64 和 arm64 上的 linux 和 darwin，以及 windows/amd64。代码在 `cmd/slowjs`。

## 获取

从[最新的 release](https://github.com/liustack/quickjs/releases/latest) 下载你的系统对应的压缩包，用 `checksums.txt` 校验，然后解压：

| 系统 | 压缩包 |
| --- | --- |
| linux/amd64 | `slowjs-x86_64-unknown-linux-gnu.tar.gz` |
| linux/arm64 | `slowjs-aarch64-unknown-linux-gnu.tar.gz` |
| macOS，Apple 芯片 | `slowjs-aarch64-apple-darwin.tar.gz` |
| macOS，Intel | `slowjs-x86_64-apple-darwin.tar.gz` |
| windows/amd64 | `slowjs-x86_64-pc-windows-msvc.zip` |

```sh
curl -LO https://github.com/liustack/quickjs/releases/latest/download/slowjs-x86_64-unknown-linux-gnu.tar.gz
curl -LO https://github.com/liustack/quickjs/releases/latest/download/checksums.txt
sha256sum -c --ignore-missing checksums.txt
tar -xzf slowjs-x86_64-unknown-linux-gnu.tar.gz
./slowjs-x86_64-unknown-linux-gnu/slowjs --version
```

每个压缩包里是一个文件夹，放着二进制和它必须附带的许可证文本（[NOTICE.md](../NOTICE.md#what-a-program-that-ships-a-binary-must-include)）。二进制是静态的，用 `CGO_ENABLED=0` 构建：linux 版在 glibc 和 musl 的系统上都能跑。浏览器下载的二进制没有开发者签名，macOS 会拦着不让运行：`curl` 不会打这个标记，`xattr -d com.apple.quarantine slowjs` 可以去掉它。

## 运行一个文件

```sh
slowjs [flags] file.js [args...]
slowjs [flags] -e code [args...]
```

| 参数 | 作用 |
| --- | --- |
| `-e code` | 运行 `code`，不读文件 |
| `--timeout d` | 运行到 `d` 就结束，比如 `30s` 或 `2m`。`0` 表示永不结束。默认 `10m` |
| `--memory-limit n` | VM 占用超过 `n` 字节就结束运行。`K`、`M`、`G`（或 `KiB`、`MiB`、`GiB`）分别是 1024、1024² 和 1024³。`0` 表示不设上限。默认 `2GiB` |
| `--version` | 在一行里打印 `slowjs <version>`，然后退出 |
| `-h`、`--help` | 打印参数说明，然后退出 |

文件在一个新的 VM 里作为全局脚本（不是模块）运行，然后运行它排进队列的 promise 任务，所以 `then` 回调和 async 函数都会跑完。脚本里有引擎支持的整个语言（ES2023，包括 BigInt、proxy、generator 和 async 函数），另外还有：

- `print(...args)` 和 `console.log(...args)`：每个参数按 `String(x)` 的方式变成字符串，用空格连起来，加一个换行，写到 stdout。和 `qjs` 一样，对象打印成 `[object Object]`。两者单独调用也能用，比如 `var p = console.log; p(1)`。
- `console.error(...args)`：一样，写到 stderr。
- `scriptArgs`：参数之后的那些命令行参数，和 `qjs` 一样：文件名和它后面的参数，用 `-e` 时只有后面的参数。

每次调用当场写出它那一行，写 stderr 的一行之前先写完 stdout，所以终端或者从管道读的工具在脚本打印时就能看到这些行，顺序也不乱。孤立的代理项打印成 U+FFFD。

没有被捕获的异常会把它的文本和调用栈打印到 stderr，最多 50 帧。栈里的文件名是 `<eval>`，这是 Go API 执行脚本时用的名字，不是文件的名字。

## 退出码

| 退出码 | 什么时候 |
| --- | --- |
| 0 | 脚本和它的任务都运行完了 |
| 1 | 没有被捕获的异常，包括语法错误 |
| 2 | 命令行写错了，或者文件读不了。什么都没运行 |
| 3 | 运行超过了 `--timeout` |
| 4 | VM 超过了 `--memory-limit` |

Go 自己崩溃时也以 2 结束，并打印 `panic:` 或 `fatal error:` 和各个 goroutine。

## 上限

超时或内存上限一到，不管脚本在做什么，运行都会结束：之后不会再运行任何 `catch`、`finally` 或 promise 的处理函数，和 Go API 的每次调用一样（[uncatchable.md](changes/uncatchable.zh-CN.md)）。

- **超时**管的是整次运行，脚本加上它的任务，不是每一部分各算各的。默认 10 分钟，是为了让一个永远不结束的脚本不会把无人看管时运行它的机器占上几个小时，比如本身没有超时的基准任务。它是跑一遍 V8 基准套件所需时间的 10 倍以上（在一台忙碌的 Apple M4 上跑一次是 36 秒，C QuickJS 和 QuickJS-NG 在 js-engine-benchmark 的 runner 上是 31 到 47 秒），也是那张表里最慢的引擎（152 秒）的 4 倍。更长的工作负载用 `--timeout 0` 或更长的时间。
- **内存上限**算的是 VM 占用的内存：它的对象、字符串和引擎自己的结构。默认 2 GiB，是为了让一个不停分配的脚本在系统内存耗尽、开始换页或结束别的进程之前，先因为一个错误停下。它是 V8 套件所需内存的 15 倍：套件在 `--memory-limit 128MiB` 下内存不足，不设上限时能跑完，常驻内存峰值 141 MB（Apple M4，各跑一次）。需要更多内存的工作负载用 `--memory-limit 0` 或更大的值。低于 128 KiB 的上限按 128 KiB 算。
- `print` 和 `console` 把一行分成最多 65,536 个 UTF-16 单元的片段交给 Go，所以不管脚本在上限内造出的字符串有多长，Go 为了写出它做的拷贝每次最多 192 KiB，这部分不算在上限里。同样不算在上限里的还有：源文件（Go 在运行前把它整个读进来），以及 Go 运行时自己的内存。

## 在基准测试工具里

在命令行运行各个引擎的工具，比如 [js-engine-benchmark](https://github.com/ahaoboy/js-engine-benchmark)，需要四样东西，slowjs 都有：

- 它运行 `slowjs <file>`，套件合成一个文件：V8 套件的 run.js，把每个 `load('x.js')` 换成那个文件的内容。slowjs 没有 `load`。
- 套件用 `print` 打印结果，有 `console.log` 时就用它，而且是单独调用（`var print = console.log`），工具从 stdout 读 `Name: score` 这样的行。
- 它用 `v([\d.]+...)` 这样的模式从 `--version` 读版本号，所以版本号在一行里。
- 预编译二进制的安装工具（js-engine-benchmark 用 easy-install）按文件名里的目标名找某个系统的压缩包，目标名按 Rust 的叫法。

用同样的方式运行 `compare/engines` 用的那份 V8 套件（在 `internal/arewefastyet/v8-v7`）：

```sh
cd internal/arewefastyet/v8-v7
for f in base richards deltablue crypto raytrace earley-boyer regexp splay navier-stokes; do cat $f.js; echo; done >/tmp/v8.js
sed -n '/^var success/,$p' run.js >>/tmp/v8.js
slowjs /tmp/v8.js
```

## 有意不做的

这个命令是能按上面那些工具的方式运行文件的最小命令。它不做的：

- **没有 REPL。** 既没有文件也没有 `-e` 时，它打印用法并以 2 退出。
- **没有 ES 模块。** 在全局脚本里 `import` 和 `export` 是语法错误，而 Go API 从 v0.27.0 起就没有模块加载器了（[go-api.md](changes/go-api.zh-CN.md#没有模块加载器shell-辅助函数和-setcanblock)）。读过的基准工具都不需要模块：js-engine-benchmark 运行的是一个普通脚本。
- **没有 stdin。** 读过的工具都不通过 stdin 传脚本。
- **没有文件系统、网络、定时器和 Node 的 API。** 没有 `load`、`require`、`process`、`fs`、`setTimeout` 和 `fetch`。它是引擎，不是 Node。
- **没有 Intl 和 Temporal。** 用到 `Intl` 的脚本会得到 ReferenceError，`toLocaleString` 和 `localeCompare` 的行为和没有 Intl 的 QuickJS 一样。Chromium 每个语言区域的数据会让二进制变成 2.6 倍大（见下表），而 V8 套件里没有一项基准需要它。需要它们的 Go 程序在自己的 VM 上注册 `intl` 和 `temporal`（[api.md](api.zh-CN.md)）。
- **不报告没有被处理的 promise 拒绝。** 没人处理的被拒绝的 promise 照样以 0 结束运行，因为 Go API 没有能告诉它这件事的钩子。

按 release 的方式构建（`-trimpath -ldflags "-s -w"`，Go 1.27.2）的二进制大小，单位 MiB：

| 目标 | slowjs | 带 `intl` | 带 `intl` 和 `temporal` |
| --- | --: | --: | --: |
| linux/amd64 | 7.1 | 18.0 | 18.7 |
| linux/arm64 | 6.5 | 17.1 | 17.8 |
| darwin/amd64 | 7.2 | 18.1 | 18.8 |
| darwin/arm64 | 6.5 | 17.3 | 17.9 |
| windows/amd64 | 7.4 | 18.3 | 19.0 |

压缩包是 2.3 到 2.7 MiB。用 gzip 压缩，linux/amd64 的二进制单独是 2.6 MiB，带 `intl` 是 8.1 MiB。

## 测试

`go test ./cmd/slowjs` 构建一个打上版本号的命令，在一些小脚本上运行它：`print` 和 `console` 写了什么、按什么顺序写，一个代理对跨在两个片段之间的长字符串，`scriptArgs`，没有被捕获的异常和它的栈，语法错误，promise 任务，超时结束一个带 catch 的死循环（以及任务超时的一次运行），内存上限结束一个不停分配的循环，`--version`、`-h` 和各种写错的命令行。`TestV8Suite`（`-short` 会跳过它）像基准工具那样通过命令运行 V8 套件，检查每项基准都给出了分数，并且自身检查通过的基准，和在 Go API 的 VM 里通过的那些一样。它不计时。

设置了 `SLOWJS_BIN` 和 `SLOWJS_VERSION` 时，测试运行那个二进制而不是自己构建一个，并期望它报告那个版本号。发布任务就这样在每个系统上对它构建的每个二进制运行这些测试（[releasing.md](releasing.zh-CN.md#发布任务)）。
