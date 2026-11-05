---
summary: "VM 之间共享所加载内容的改动：加载同一份字节码的所有 VM 共用一份函数源码文本和行表，共享字节码只在第一次加载时扫描轮询点，每项的收益和代价，为什么安全"
read_when:
  - 想知道一次 GC 之后 VM 占多少内存，或者 VM 在 JS_ComputeMemoryUsage 之外还保留什么
  - 把同一个 bundle 加载进多个 VM（SharedBytecode、EvalSharedBytecode）
  - 加载 bundle 要很久，或者读字节码时失败并报 "invalid bytecode"
---

# 内存与分配

[English](memory-and-allocation.md) · 简体中文

引擎有自己的分配器，前面有一层已释放小块的缓存，见 [allocator.md](../allocator.zh-CN.md)。本页写的是 liustack/quickjs 对同一个程序里多个 VM 如何共享内存所做的改动。

## 所有 VM 共用一份源码文本和行表

发布版本：v0.28.1。

用 `EvalBytecode` 加载字节码的 VM，会把每个函数的源码文本（`toString` 返回的内容）和把字节码映射到行号的表（错误 stack 要读它）复制进自己的内存。把一个 bundle 加载进一池 VM 的服务器，每个 VM 都存了一份这些字节，而且没有 VM 会写它们。现在 `NewSharedBytecode` 在 Go 堆之外把字节码复制一次，`VM.EvalSharedBytecode`（和 `EvalSharedBytecodeValue`）用 `JS_ReadObject` 的标志 `JS_READ_OBJ_SHARED_DEBUG` 加载它：读出的每个函数的源码文本和行表都指向这份副本，函数用 `JSFunctionBytecode` 的 `shared_debug` 位表明这一点。VM 读出的其他东西（代码、atom（引擎给属性名和常用字符串的编号） 和常量）仍然是各自私有的。

JavaScript 看到的不变：`toString` 给出同样的文本，错误 stack 给出同样的行和列。`TestSharedBytecodeSameAsCopy` 拿它和 `EvalBytecode` 对照检查过，覆盖函数、箭头函数、带 getter 和静态方法的类、async 方法、在两层调用之下抛出的错误、在函数里生成的 stack，以及有 256 KiB 源码的函数。

为什么安全：

- 只要 `SharedBytecode` 还开着，或者加载过它的某个 VM 还开着，副本就一直在。`SharedBytecode` 数着持有它的引用：自己持有一份直到 `Close`，每个加载过它的 VM 持有一份，`VM.Close` 在运行时和里面所有函数都释放之后才放掉它。最后一个放掉的会释放副本。读到一半失败时，VM 也保留它的那份，因为错误之前读出的函数一直指向副本，直到运行时被释放。`Close` 之后再加载会返回错误。所以 VM 可以比创建它的池活得更久，`TestSharedBytecodeLifetime` 检查了这一点，副本只在最后一次关闭时才释放。
- 引擎从不写入或移动从字节码读出的函数的源码文本或行表，它只写自己编译的函数的这两样。设了 `shared_debug` 时，`free_function_bytecode` 不会释放它们。
- 读取时会检查它们每一个都落在缓冲区内，缓冲区被截断时和复制版本一样失败（`TestSharedBytecodeTruncated`）。
- 内存上限、`MemorySize`、GC 阈值和 `JS_ComputeMemoryUsage` 统计的是运行时分配的内存，所以不再统计共享的这些字节：`compute_bytecode_size` 把它们排除在外。副本由程序统计一次，即 `SharedBytecode.Len`。

这个标志存在 `BCReaderState` 里，由 `bc_get_debug_buf` 读取，`JS_ReadFunctionTag` 对这两个缓冲区调用它。副本是在所有 VM 之外的只读映射（`hostmem.MapReadOnly`）：每个 VM 的引擎就地读取，不加锁，往里写会触发错误（`TestSharedBytecodeReadOnly`）。最后一个持有者会取消映射。

在 linux/amd64（AMD EPYC 7B13）上省下多少，在 tifa 里用它的池测得，池用 `EvalSharedBytecode` 加载 bundle（加载后一个 VM 的 `JS_ComputeMemoryUsage`，每个 VM 都一样）：

| Bundle | 字节码 | 加载后一个 VM，之前 | 之后 | 每个 VM 少 |
| --- | --- | --- | --- | --- |
| tifa 的示例商店 | 5.27 MB | 11,459,272 字节 | 8,986,552 | 2.36 MiB (0.784) |
| tifa 的 MUI 测试应用 | 2.73 MB | 5,436,184 字节 | 4,155,576 | 1.22 MiB (0.764) |

商店里加载后留下的 bundle 函数，持有 1.34 MB 的源码文本和 0.30 MB 的行表。差额的其余部分是分配器给这些块加的头和取整。有 8 个 VM 时，进程的 RSS 在商店上从 122.0 降到 100.9 MB，在 MUI 上从 72.7 降到 57.9 MB。渲染时间不变。在 tifa 里不开 PGO 计时，一个 VM 跑在一个空闲核对的一个线程上，main 和这个分支在 3 种代码布局下轮流跑（23 对运行），商店的 6 个工作台页面渲染用时的中位数是原来的 0.994（0.979 到 1.015），MUI 测试应用的 3 个页面是 0.994（0.981 到 1.005）。池的回收频率和以前一样（年轻代回收和完整回收的次数相同），HTML 逐字节相同。


## 共享字节码只在第一次加载时扫描轮询点

发布版本：v0.29.1。

读取字节码的 VM 要按轮询点的规则检查每个函数的代码（先 `check_polls`，再 `poll_scan`）：连续超过 127 条指令不能没有计数，不能出现解释器没有的 opcode，还有 `OP_ret` 可以去的地方。这次扫描占了 `JS_ReadObject` 的一半：读 117 KB 的 React bundle 的 470 万条指令里有 210 万条，读对比里示例商店的 bundle 的 6750 万条里有 2690 万条。把同一个 `SharedBytecode` 加载进每个 VM 的池，要为每个 VM 把同样的字节再扫一遍。

现在 `SharedBytecode` 在某个 VM 无错读完它之后，给自己打上已检查的标记，`VM.EvalSharedBytecode` 之后加载它时带上标志 `JS_READ_OBJ_POLLS_CHECKED`。`JS_ReadFunctionBytecode` 有这个标志时跳过 `poll_scan`，但仍然生成 `OP_ret` 可以去的地方（`ret_bits`），因为读出的每个函数都要用到它。`VM.EvalBytecode` 没有这样的标记，每次都扫描。

为什么安全：

- 设标记的那次读取，对这些字节里的每个函数都通过了 `check_polls`，因为它在第一个不通过的函数处就会失败，而且副本是没有 VM 能写的只读映射（`TestSharedBytecodeReadOnly`）。下一次读取看到的是同样的字节。
- 读不通的字节从不打标记，所以每次加载都会像第一次一样失败（`TestSharedBytecodeChecksOnce`）。两次同时加载都会扫描，哪一次设标记都可以。
- 这个标志是给之前读过同样字节的调用者用的。Go API 只会根据标记来设它。`NewSharedBytecode` 已经说明它的字节码没有经过检查，所以不能来自不可信的来源。

省下多少，在 linux/amd64（AMD EPYC 7B13）上，用 callgrind 在第一个代码位置测得，每个新 VM 的加载加关闭（运行 13 次和 3 次的差），对比 main：

| Bundle | 指令数，之前 | 之后 | `JS_ReadObject`，之前 | 之后 |
| --- | ---: | ---: | ---: | ---: |
| React SSR bundle (117 KB) | 11.05 M | 8.90 M (-19.5%) | 4.72 M | 2.56 M |
| 对比里的商店 bundle，带 `Intl` | 235.3 M | 208.4 M (-11.4%) | 67.5 M | 40.6 M |

时间：用 Go 基准在 EPYC 7B13 的一个核上计时，它的同伴线程空闲，8 个代码位置，5 轮：新建 VM 并 `EvalSharedBytecode` React bundle，用时 1.899 ms，之后 1.498 ms (0.789，每个位置都更快)，商店 bundle 是 34.4 ms，之后 29.7 ms (0.862)。同样的加载用 `EvalBytecode`，是 2.441 ms，之后 2.443 ms：它每次都扫描。单独新建一个 VM 用时相同，是 0.24 ms。

测试：`engine/test/unit-white.c` 读取违反轮询点规则的代码，分别带和不带这个标志（不带时出错，带时读得进去），并检查 `OP_ret` 跳到不跟在任何 `OP_gosub` 之后的位置时，带标志也仍然是 `InternalError`，finally 块也仍然会运行。`sharedbytecode_test.go` 检查标记由第一次成功的加载设置，读不通的字节不会设置，并且每个 VM 看到的都一样。

改动在 C 里（`engine/quickjs.c`、`engine/quickjs.h`）：`JS_READ_OBJ_POLLS_CHECKED`、带 `scan` 参数的 `check_polls`，以及 Go 里的 `SharedBytecode.checked`。
