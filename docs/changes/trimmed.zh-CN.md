---
summary: '引擎里因为 Go API 永远用不到而去掉的东西：QuickJS shell 的 std、os、Worker 模块和事件循环，以及为什么用这个模块的程序更小，也不会链接任何 fork、exec 或打开文件的代码'
read_when:
  - 在引擎里找 js_init_module_std、js_std_loop、js_os_exec 之类的符号
  - 想知道为什么用这个模块的程序不链接 fork、exec 或 popen
---

# 引擎里去掉的东西

[English](trimmed.md) · 简体中文

## 没有 shell 库

`quickjs-libc.c` 是 QuickJS 的 shell `qjs` 的 C 文件：`std`、`os` 和 `Worker` 模块、事件循环和几个辅助函数。`engine/` 里的 C 树没有它。Go API 不注册这类模块，所以通过它运行的脚本碰不到它们，shell 原来给脚本的东西，Go API 用 Go 来给：类似 `console` 的辅助函数由程序自己加成 [Builtin](go-api.md#builtin-a-go-function-called-as-a-built-in)，模块加载器由程序自己提供，模块顶层 `await` 在 Go 里等待（见 [go-api.md](go-api.md#evalbytecode-waits-for-a-modules-top-level-await-in-go)）。

引擎仍然需要那个文件里的少量东西，它们写在 `engine/` 或 Go host 里：`JS_PrintValue` 打印错误（只在有 `CONFIG_PRINT_VALUE` 时，原生测试 host 会设它）、模块的 `import.meta`（由 Go host 设置），以及原生测试 host 加载文件（`engine/test/host.c`）。

为什么安全：Go API 从来不调用 shell 的代码。模块字节码给出的结果和以前一样，test262 给出同样的 58 个已知错误。

只执行 JavaScript 的程序不会链接任何调用 `fork`、`execve`、`popen`、`pipe`、`kill`、`waitpid` 或 `tcsetattr` 的东西：引擎只向它的 host 要 `engine/host.h` 里的回调，其他什么都不要（`engine/test/check-symbols.sh` 检查引擎可以用的符号列表），Go host 也不链接 libc。
