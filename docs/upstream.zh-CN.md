# 上游

[English](upstream.md) · 简体中文

## 代码从哪来

- **引擎**是 [QuickJS](https://bellard.org/quickjs)，Fabrice Bellard 和 Charlie Gordon 用 C 写的 JavaScript 引擎，取 2026-06-04 版，提交 `3d5e064e9dd67c70f7962836505a7fa067bf0a4e`。`engine/` 是这份 C 加上 slowjs 的改动。`internal/goasm/gen` 把它编译成 Go 汇编，提交进仓库。生成的文件从不手改，和生成器的结果不一致时 `make check-gen` 会失败。test262 有 58 个已知失败，列在 `engine/test/test262_errors.txt`。
- **Go API**（模块根目录的 `VM`、`Value`、`Eval`、`Call` 等）起步于 [modernc.org/quickjs](https://gitlab.com/cznic/quickjs) v0.25.0（2026-10-01），也就是本仓库历史里的提交 `ea7b851`，之后重写并扩充过。其中一些名字、注释和测试还在。v0.28 之前，它下面的引擎是 [modernc.org/libquickjs](https://gitlab.com/cznic/libquickjs)，ccgo 把 QuickJS 转译成的 Go。那个引擎已经删掉，它的 Go 也不在了。
- **其余部分**是为 slowjs 写的，或者来自 [NOTICE.md](../NOTICE.md) 里列出的项目：分配器（TLSF）、引擎自带的小 libc（musl）、语言数据，以及 `Intl` 和 `Temporal` 的 Go 部分（go-intl、ICU、Chromium）。

slowjs 的作者列在 [AUTHORS-SLOWJS](../AUTHORS-SLOWJS)。modernc.org/quickjs 的 `AUTHORS` 文件按它原样保留为 [AUTHORS](../AUTHORS)。

## 合入 QuickJS 的提交

`engine/` 里的 C 树不跟着上游的每个版本走。钉住的提交之后上游的提交，凡是修正确性、内存泄漏、崩溃的（不管页面碰不碰得到），或者让渲染更快的，单独搬进来，带测试。新功能和只为速度的改动，tifa 用得上才搬。搬进来的、试过又撤回的提交都在 [upstream-commits.md](changes/upstream-commits.md)。怎么合入一个提交，见 [upgrading.md](upgrading.md)。

slowjs 对这份 C 的改动逐项列在 [changes](changes/README.md)：脚本捕获不了的限额、每台机器上一样的输出、覆盖 Chromium 全部语言的 `Intl`，以及更快的渲染。

## 从 modernc.org/quickjs 换过来

用过 modernc.org/quickjs v0.25.0 的程序会遇到这些：

- **import 路径。** `modernc.org/quickjs` 改成 `github.com/liustack/slowjs`，包名 `quickjs` 改成 `slowjs`，它的 `intl` 和 `fmt` 包跟着改。引擎在模块里面，所以 `go.mod` 里不再有 `modernc.org/libquickjs`。
- **Go 版本。** 需要 Go 1.27，modernc.org/quickjs v0.25.0 需要 1.26。
- **本地时间是 UTC。** 每个 VM 里脚本的本地时间都是 UTC，不管进程的 `TZ` 是什么。要显示别的时区，把时区名交给 `Intl`（[详情](changes/time-and-intl.md#local-time-is-utc-in-every-vm)）。
- **Intl 覆盖所有语言。** `intl` 包支持 Chromium 有的所有语言，modernc.org/quickjs 只有 en-US。import 它的程序会内嵌语言数据和这些数据的版权声明（[详情](changes/time-and-intl.md#intl-in-every-locale-chromium-has)）。
- **限额一到，调用就结束。** 中断、执行时限和内存耗尽都不能再被脚本捕获，`catch`、promise、迭代器和 Go 函数都不行（[详情](changes/uncatchable.md)）。
- **错误信息更多。** `*Error` 有 `Uncatchable`、`OutOfMemory` 和 `Interrupted`。转换不了的字符串会作为错误返回，不再是 `""`。Go 里建的 `*Error` 的 `Error()` 是 `Name: Message`，不再是空字符串（[详情](changes/uncatchable.md#a-panic-in-a-go-function)）。
- **字符串。** 从 JavaScript 拿到的 Go 字符串一定是合法的 UTF-8，Go 传过去的非法 UTF-8 每个字节变成一个 U+FFFD（[详情](changes/go-api.md#valid-utf-8-for-lone-surrogates)）。
- **panic。** 引擎自身 panic，或者 Go 函数调用了 `runtime.Goexit`，这个 VM 之后只能 `Close`（[详情](changes/uncatchable.md#a-panic-in-a-go-function)）。
- **无限递归** 抛 `RangeError`，不会让进程退出（[详情](changes/errors-and-stacks.md#endless-recursion-throws-rangeerror)）。
- **引擎里没有 std、os 和 Worker。** 引擎去掉了 QuickJS 命令行里 Go API 从来走不到的部分：没有 `js_init_module_std`、`js_init_module_os`、`js_std_loop` 和 `js_std_init_handlers`。模块的字节码照常可用，程序小几百 KB（[详情](changes/trimmed.md)）。
- **不能加载模块，没有命令行的辅助函数，不能阻塞。** `VM.SetModuleLoader`、`VM.SetDefaultModuleLoader` 以及 `ModuleLoaderFunc` 和 `ModuleNormalizeFunc`，`VM.StdAddHelpers`（`console.log`、`print`、`performance.now`、`__loadScript`），还有 `VM.SetCanBlock` 都删了。模块只能 import VM 里已有的模块，比如用名字 `<eval>` import 它自己，`Atomics.wait` 总是抛错（[详情](changes/go-api.md#no-module-loader-shell-helpers-or-setcanblock)）。
- **只支持两个平台。** 模块只为 linux/amd64 和 darwin/arm64 构建。在其他平台上构建会报错，windows、darwin/amd64 和 linux/arm64 也一样，错误里写明这两个平台（[详情](architecture.md#platforms)）。
- **换了引擎。** 引擎是编译成 Go 汇编的 C，以前是翻译成 Go 的 QuickJS。脚本的结果相同，递归深度按调用次数限制（默认 5000，最多 10000），JSON 最多嵌套 2000 层（[详情](changes/c-engine.md)）。
- **新 API。** `NewBuiltin`、`VM.NewBuiltinFunction` 和 `Args`，`VM.NewArray`，`VM.SetStrip` 和 `type Strip`，`VM.RunGC`、`VM.RunYoungGC`（[详情](changes/young-collection.md)）、`VM.MemorySize` 和 `VM.GCThreshold`，`VM.MemoryUsage` 和 `type MemoryUsage`，`VM.ReleaseMemory`，`VM.SetShapeCache` 和 `DefaultShapeCache`，`VM.OnClose`，`NewSharedBytecode` 和 `VM.EvalSharedBytecode`，`GetAsyncContext` 和 `SetAsyncContext`（[详情](changes/go-api.md)），`VM.SetClock` 和 `VM.Now`（[详情](changes/time-and-intl.md#one-wall-clock-per-vm)），以及 `temporal` 包（[详情](changes/temporal.md)）。

每个版本的所有改动见[更新记录](../CHANGELOG.zh-CN.md)。

## 致谢

引擎是 Fabrice Bellard 和 Charlie Gordon 的 QuickJS。Go API 起步于 modernc.org/quickjs，作者是 quickjs-go 的 Jan Mercl 和 Dan Kortschak，Jan Mercl 还用 ccgo 把 QuickJS 转译成 Go，做成 modernc.org/libquickjs。slowjs 感谢他们带来的最初那套 API。还留在仓库里的那部分代码需要他们的许可证文件，所以都保留着：[LICENSE](../LICENSE) 和 [LICENSE-MODERNC-LIBQUICKJS](../LICENSE-MODERNC-LIBQUICKJS)，QuickJS 的是 [LICENSE-LIBQUICKJS](../LICENSE-LIBQUICKJS)。
