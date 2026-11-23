---
summary: '把 Go 程序从 modernc.org/quickjs v0.25.0 迁到 github.com/liustack/slowjs 时，哪些地方要改、哪些行为会变'
read_when:
  - 把 Go 程序从 modernc.org/quickjs 迁到 github.com/liustack/slowjs
  - 核对哪些 API 或行为和 modernc.org/quickjs v0.25.0 不同
audience: user
order: 5
translatedFrom: 2243ff437490
---

# 从 modernc.org/quickjs 迁移

[English](from-modernc.md) · 简体中文

这一页说明原来用 `modernc.org/quickjs` v0.25.0 的 Go 程序，改用 `github.com/liustack/slowjs` 时会遇到什么。

## 代码和 go.mod 里要改的地方

- **导入路径和包名。** `modernc.org/quickjs` 改成 `github.com/liustack/slowjs`，包名从 `quickjs` 改成 `slowjs`。它下面的 `intl` 和 `fmt` 包一起跟着改。引擎就在本模块里，所以 `modernc.org/libquickjs` 从你的 `go.mod` 里去掉。
- **Go 版本。** 本模块需要 Go 1.26.6 或更新的版本，没有核对过的 Go 版本会停在编译阶段（见 [upgrading.md](../internals/upgrading.md#moving-to-a-newer-go)），而 `modernc.org/quickjs` v0.25.0 需要 Go 1.26。
- **只支持六个平台。** 本模块只为 linux、darwin 和 windows 上的 amd64 与 arm64 构建。为其他目标平台构建会报错停下，并在错误里列出支持的平台（[详情](../internals/architecture.md#platforms)）。
- **没有模块加载、shell 辅助函数和阻塞。** `VM.SetModuleLoader`、`VM.SetDefaultModuleLoader` 以及 `ModuleLoaderFunc` 和 `ModuleNormalizeFunc`、`VM.StdAddHelpers`（`console.log`、`print`、`performance.now`、`__loadScript`）还有 `VM.SetCanBlock` 都去掉了。模块只能导入 VM 里已经有的模块，比如用名字 `<eval>` 导入自己，`Atomics.wait` 总是抛错（[详情](../internals/changes/go-api.md#no-module-loader-shell-helpers-or-setcanblock)）。

## 行为上的变化

- **本地时间是 UTC。** 不管进程里的 `TZ` 设成什么，每个 VM 里脚本看到的本地时间都是 UTC。要显示别的时区，向 `Intl` 指明时区名（[详情](../internals/changes/time-and-intl.md#local-time-is-utc-in-every-vm)）。
- **所有语言环境的 Intl。** 包 `intl` 支持 Chromium 有的每一种语言环境，而 `modernc.org/quickjs` 只格式化 en-US。导入它的程序会把语言环境数据和对应的许可声明编进程序里（[详情](../internals/changes/time-and-intl.md#intl-in-every-locale-chromium-has)）。
- **上限到了就结束调用。** 中断、eval 超时和内存耗尽都不能再被脚本捕获，不管是靠 `catch`、promise、迭代器还是 Go 函数（[详情](../internals/changes/uncatchable.md)）。
- **错误给出的信息更多。** `*Error` 多了 `Uncatchable`、`OutOfMemory` 和 `Interrupted`。无法转换的字符串会返回错误，不再返回 `""`。在 Go 里创建的 `*Error`，它的 `Error()` 返回 `Name: Message`，不再是空字符串（[详情](../internals/changes/go-api.md#error-of-an-error-made-in-go)）。
- **字符串。** 从 JavaScript 拿到的 Go 字符串总是合法的 UTF-8，从 Go 传入的非法 UTF-8 每个字节会变成一个 U+FFFD（[详情](../internals/changes/go-api.md#valid-utf-8-for-lone-surrogates)）。
- **panic。** 引擎自身发生 panic，或者 Go 函数调用了 `runtime.Goexit`，这个 VM 之后只能用来 `Close`（[详情](../internals/changes/uncatchable.md#a-panic-in-a-go-function)）。
- **无限递归**会抛出 `RangeError`，不再让整个进程崩溃（[详情](../internals/changes/errors-and-stacks.md#endless-recursion-throws-rangeerror)）。
- **换了引擎。** 引擎是编译成 Go 汇编的 C 代码，以前是翻译成 Go 的 QuickJS。脚本给出的结果一样，递归按调用次数限制（默认 5000，最多 10000），JSON 最多嵌套 2000 层（[详情](../internals/changes/c-engine.md)）。
- **引擎里没有 std、os 和 Worker。** 引擎去掉了 QuickJS shell 里 Go API 从未用到的部分：没有 `js_init_module_std`、`js_init_module_os`、`js_std_loop` 和 `js_std_init_handlers`。模块的字节码和以前一样可用，程序体积小了几百 KB（[详情](../internals/changes/trimmed.md)）。

## 新增的内容

`NewVM`、`Eval`、`Call` 和 `Value` 之外的 Go API 都在 [api.zh-CN.md](api.zh-CN.md) 里，每个函数都写了是哪个版本加入的。

每个版本的全部改动见 [更新日志](../../CHANGELOG.md)。
