# 上游

[English](upstream.md) · 简体中文

## 代码从哪来

代码分四步做成，每一步建在上一步之上：

1. **[QuickJS](https://bellard.org/quickjs)**：Fabrice Bellard 和 Charlie Gordon 用 C 写的 JavaScript 引擎。这个 fork 的引擎是 QuickJS 2026-06-04 版，提交 `3d5e064e9dd67c70f7962836505a7fa067bf0a4e`（见 `lib/generator.go` 里的 `qjsCommit`）。
2. **[modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs)**：ccgo 把这份 C 转译成的 Go，不用 cgo。fork 用 `git subtree` 把 v0.13.1（2026-07-31）放进了 `lib/`。
3. **[modernc.org/quickjs](https://gitlab.com/cznic/quickjs)**：libquickjs 之上的 Go API。fork 从 v0.25.0（2026-10-01）开始，也就是本仓库历史里的提交 `ea7b851`。
4. **这个 fork**：`github.com/liustack/slowjs`（v0.27.x 之前叫 `github.com/liustack/quickjs`），把 Go API 和引擎合成一个模块。

modernc.org/libquickjs 和 modernc.org/quickjs 出自 quickjs-go 的作者们。它们的 `AUTHORS` 文件按上游原样保留为 [AUTHORS](../AUTHORS) 和 [lib/AUTHORS](../lib/AUTHORS)，两个模块都列了 Jan Mercl，modernc.org/quickjs 还列了 Dan Kortschak。这个 fork 的作者列在 [AUTHORS-FORK](../AUTHORS-FORK)。

## fork 保留了什么，改了什么

fork 保留 modernc.org/quickjs 的 Go API，以及 ccgo 生成的引擎。`lib/` 下生成的文件从不手改：每次上游发版后由 `internal/fixlib` 机械地改写，提交的文件和 `fixlib` 从上游文件生成的结果逐字节一致（[怎么核对](testing.md#same-as-upstream-after-fixlib)）。test262 的 58 个已知失败和上游相同。

改了什么逐项列在 [fork-changes](fork-changes/README.md)：脚本捕获不了的限额、每台机器上一样的输出、覆盖 Chromium 全部语言的 `Intl`，以及更快的渲染。怎么合入 libquickjs、modernc.org/quickjs 或 modernc.org/libc 的新版本，见 [upgrading.md](upgrading.md)。

## 从 modernc.org/quickjs 换过来

用过 modernc.org/quickjs v0.25.0 的程序会遇到这些：

- **import 路径。** `modernc.org/quickjs` 改成 `github.com/liustack/slowjs`，包名 `quickjs` 改成 `slowjs`，它的 `intl` 和 `fmt` 包跟着改。引擎是 `github.com/liustack/slowjs/lib`，所以 `go.mod` 里不再有 `modernc.org/libquickjs`。
- **Go 版本。** 需要 Go 1.27，上游需要 1.26。
- **本地时间是 UTC。** 每个 VM 里脚本的本地时间都是 UTC，不管进程的 `TZ` 是什么。要显示别的时区，把时区名交给 `Intl`（[详情](fork-changes/time-and-intl.md#local-time-is-utc-in-every-vm)）。
- **performance.now。** `StdAddHelpers` 的 `performance.now` 从进程启动开始计时。上游从系统启动开始，在 Windows 上读的是墙上时钟，从 1970 年开始。在 macOS 上不计入系统睡眠的时间（[详情](fork-changes/time-and-intl.md#the-engine-reads-gos-clocks)）。
- **Intl 覆盖所有语言。** `intl` 包支持 Chromium 有的所有语言，上游只有 en-US。import 它的程序会内嵌语言数据和这些数据的版权声明（[详情](fork-changes/time-and-intl.md#intl-in-every-locale-chromium-has)）。
- **限额一到，调用就结束。** 中断、执行时限和内存耗尽都不能再被脚本捕获，`catch`、promise、迭代器和 Go 函数都不行（[详情](fork-changes/uncatchable.md)）。
- **错误信息更多。** `*Error` 有 `Uncatchable`、`OutOfMemory` 和 `Interrupted`。转换不了的字符串会作为错误返回，不再是 `""`。Go 里建的 `*Error` 的 `Error()` 是 `Name: Message`，不再是空字符串（[详情](fork-changes/go-api.md)）。
- **字符串。** 从 JavaScript 拿到的 Go 字符串一定是合法的 UTF-8，Go 传过去的非法 UTF-8 每个字节变成一个 U+FFFD（[详情](fork-changes/go-api.md#valid-utf-8-for-lone-surrogates)）。
- **panic。** 引擎自身 panic，或者 Go 函数调用了 `runtime.Goexit`，这个 VM 之后只能 `Close`（[详情](fork-changes/calls-and-tls-frames.md#frames-freed-without-defer)）。
- **无限递归** 抛 `RangeError`，不会让进程退出（[详情](fork-changes/errors-and-stacks.md#endless-recursion-throws-rangeerror)）。
- **`lib` 里没有 std、os 和 Worker。** 引擎去掉了 QuickJS 命令行里 Go API 从来走不到的部分：`lib` 不再有 `js_init_module_std`、`js_init_module_os`、`js_std_loop`、`js_std_init_handlers` 等 11 个函数。`StdAddHelpers`、`SetDefaultModuleLoader` 和模块的字节码照常可用，程序小几百 KB（[详情](fork-changes/trimmed.md)）。
- **只支持五个平台。** 模块只为 linux 和 darwin 的 amd64 与 arm64，以及 windows 的 amd64 构建。在其他平台上构建会报错，错误里写明这几个平台（[详情](architecture.md#platforms)）。
- **linux/amd64 上换了解释器。** 这里的字节码解释器是从 C 编译出的 Go 汇编，加 `-tags slowjs_nogoasm` 就换回 Go 解释器。两种解释器跑脚本的结果相同（[详情](fork-changes/go-assembly.md)）。
- **新 API。** `NewBuiltin`、`VM.NewBuiltinFunction` 和 `Args`，`VM.NewArray`，`VM.SetStrip` 和 `type Strip`，`VM.RunGC`、`VM.RunYoungGC`（[详情](fork-changes/young-collection.md)）、`VM.MemorySize` 和 `VM.GCThreshold`，`VM.SetShapeCache` 和 `DefaultShapeCache`，`VM.OnClose`，`NewSharedBytecode` 和 `VM.EvalSharedBytecode`，`GetAsyncContext` 和 `SetAsyncContext`（[详情](fork-changes/go-api.md)），`VM.SetClock` 和 `VM.Now`（[详情](fork-changes/time-and-intl.md#one-wall-clock-per-vm)），以及 `temporal` 包（[详情](fork-changes/temporal.md)）。

每个版本的所有改动见[更新记录](../CHANGELOG.zh-CN.md)。

## 致谢

引擎是 Fabrice Bellard 和 Charlie Gordon 的 QuickJS。Jan Mercl 用 ccgo 把它转译成 Go，quickjs-go 的作者 Jan Mercl 和 Dan Kortschak 写了这个 fork 所基于的 Go API。他们的许可证文件都留在仓库里：Go 代码的 [LICENSE](../LICENSE) 和 [lib/LICENSE](../lib/LICENSE)，QuickJS 的 [LICENSE-LIBQUICKJS](../LICENSE-LIBQUICKJS)。
