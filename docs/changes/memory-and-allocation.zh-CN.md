---
summary: "VM 之间共享所加载内容、以及 VM 何时回收的改动：加载同一份字节码的所有 VM 共用一份函数源码文本和行表，共享字节码只在第一次加载时扫描轮询点，内置函数要的大块内存会超过内存上限时先回收，每项的收益和代价，为什么安全"
read_when:
  - 想知道一次 GC 之后 VM 占多少内存，或者 VM 在 JS_ComputeMemoryUsage 之外还保留什么
  - 把同一个 bundle 加载进多个 VM（SharedBytecode、EvalSharedBytecode）
  - 加载 bundle 要很久，或者读字节码时失败并报 "invalid bytecode"
  - 有内存上限时脚本带着没清的垃圾内存不足，或者想知道引擎什么时候回收
---

# 内存与分配

[English](memory-and-allocation.md) · 简体中文

引擎有自己的分配器，前面有一层已释放小块的缓存，见 [allocator.md](../allocator.zh-CN.md)。本页写的是 liustack/quickjs 对同一个程序里多个 VM 如何共享内存、以及 VM 何时回收所做的改动。

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

## 大块内存在会超过内存上限时先回收

发布版本：Unreleased。

成环的垃圾要等一次回收才会释放，而引擎只在造对象或调用 JavaScript 函数、并且内存已经超过 GC 阈值时才启动回收。一次回收之后，脚本最多可以留下阈值减去存活部分那么多这样的垃圾：阈值是存活部分的 1.5 倍时，就是存活部分的一半。这时内置函数要一整块大内存，而且之前不造对象的话，就会带着这些垃圾在上限处内存不足：`'x'.repeat(n)` 对一个本来放得下的 `n` 失败了，只要垃圾清掉就放得下。

现在下面这些地方，在这块内存会让 VM 超过内存上限时，先做一次完整回收，再去要这块内存：

| 内置函数 | 检查的位置 |
| --- | --- |
| `String.prototype.repeat`、`padStart`、`padEnd` | 转换完 `this` 和参数之后，建缓冲区之前 |
| `ArrayBuffer` 的数据：它的构造函数、每个类型化数组的构造函数、`slice` 以及其他会造它的地方 | 在造对象之前，`js_create_from_ctor` 读完 `new.target` 的 `prototype` 之后才造对象 |
| `ArrayBuffer.prototype.resize` 和 `transfer` | 转换完长度之后 |
| `Array.prototype.push` | 元素区必须变大时，在内置函数入口处 |
| 寄存器循环里给快速数组末尾后一位赋值 `a[i] = v` | 元素区必须变大时，在 `JS_SetPropertyValue` 之前，它可能调用 setter |
| 按给定长度造的新数组（`toSorted`、`toReversed`、`toSpliced`、`with`），以及 `slice` 和 `splice` 的拷贝 | 在 `JS_NewArray` 之前，它会检查阈值 |
| `Array.prototype.sort` 和 `toSorted` | 待排序的值列表变大之前，在两次读元素之间 |
| `Array.prototype.join` | 缓冲区变大到 64 Ki 个字符或更多时，或者变成 16 位宽时，在两次读元素之间 |
| 用 `+` 拼出的字符串（rope）被摊平 | 在 `String.prototype` 方法的入口处，以及 `JSON.stringify` 处理一个值时 |

为什么安全：凡是造对象、运行 JavaScript 的地方本来就会回收，每个新位置都属于这两种之一。它要么在一次可能调用 `valueOf` 或 `toString` 的转换之后，要么在一次可能调用 getter 的读元素之后，要么在一次可能调用 setter 的赋值之前，要么紧挨着造对象之前，中间什么都没改。在这些位置上，内置函数手里的每个值都有计数：由调用者的帧持有，由它自己的局部变量持有，或者由一个装着有计数的值的缓冲区持有（`sort` 的列表），而且没有造了一半的对象。回收只释放没有任何计数能够到的东西，所以这些值一个也不会被释放，仍在使用的内存块也不会被挪动。阈值为 -1 会关掉引擎的回收，也同样关掉这里的回收。

代价：检查拿这块内存会让已用内存增加多少，和上限下剩的空间比一比，只有放不下时才在一个单独的函数里回收。一块内存按分配器的算法计数，再多算 8 KiB，留给它没算进去的东西（块头，以及在它旁边造对象时新开的一片小块内存，比如新 `ArrayBuffer` 的那个对象）：变大的一块只算它多出来的部分，已经摊平过的 rope 不算，所以接近上限的缓冲区一小步一小步变大，或者再次搜索同一个 rope，都不会回收。`push` 和 `a[i] = v` 只在元素区必须变大时才检查，而且放在单独的函数里：往数组已有空间里写，执行的指令和以前一样，寄存器解释器的主循环和各个帧也一样。在 arm64 上，`rv_putel_slow` 有三条路径多一次从栈上读：它们把不是数字的值存进类型化数组，这次读都在转换这个值的调用之后。`String.prototype` 方法先看是不是扁平字符串，是的话像 `JS_ToString` 一样直接返回它，所以对 rope 的检查不会让扁平字符串多花什么：在扁平字符串上循环调用 `indexOf`，执行的指令比以前少 4.7%。`join` 记下自己的缓冲区，缓冲区每变大一次多一次比较，到 64 Ki 个字符才真正检查：拼四个短字符串的 `join` 大约多 20 条指令，占调用它的循环的 1%。指令数在 Apple M4 上用 `/usr/bin/time -l` 数，Go 引擎，各跑 3 次。放得下的页面做的回收和以前一样，tifa 页面的 HTML 不变。回收之后还是放不下时，这次调用和以前一样以内存不足结束，只是多做了一次回收。

收益，在 Apple M4 上用原生 C 树（`engine/test`）量：新运行时在内存上限下，先回收一次，再造成环的垃圾直到刚好低于阈值，然后看每个内置函数能成功要到的最大一块内存（精确到 64 KiB）。以前垃圾会让这条线降下来那么多，现在这条线和没有垃圾时一样，误差在 64 KiB 以内：

| 上限，存活 | 内置函数 | 以前，有垃圾时 | 现在 | 没有垃圾时 |
| --- | --- | ---: | ---: | ---: |
| 16 MiB，4 MiB | `repeat`、`ArrayBuffer`、类型化数组、`resize`、`transfer`、`padEnd` | 10.06 MiB | 11.94 MiB | 11.94 MiB |
| 16 MiB，4 MiB | `push`、`a[i] = v` | 7.44 MiB | 11.19 MiB | 11.19 MiB |
| 64 MiB，16 MiB | `repeat` 及上面其他几个 | 39.94 MiB | 47.81 MiB | 47.81 MiB |
| 64 MiB，25 MiB | `push`、`a[i] = v` | 25.56 MiB | 38.31 MiB | 38.31 MiB |
| 64 MiB，16 MiB | `join` | 21.50 MiB | 29.69 MiB | 29.75 MiB |
| 128 MiB，51 MiB | `repeat` 及上面其他几个 | 51.62 MiB | 76.94 MiB | 76.94 MiB |
| 128 MiB，32 MiB | `push`、`a[i] = v` | 57.50 MiB | 86.31 MiB | 86.31 MiB |
| 512 MiB，128 MiB | `repeat` 及上面其他几个 | 320.12 MiB | 383.88 MiB | 383.88 MiB |

对一个很大的对象数组做 `JSON.stringify`，以前和现在都不受垃圾影响：它为每个对象造一个键的数组，等它的缓冲区让内存超过阈值后，这些数组就会启动回收。

没有改的地方：在不确定能否安全回收的地方要的大块内存，保持原样。包括在别处摊平 rope 时的那份拷贝（属性键、比较、模板字符串、Go 宿主读一次调用的结果），其他一步步扩大缓冲区的内置函数（`replace`、`split`、`JSON.stringify` 自己的缓冲区），在其他内置函数里变大的数组元素区（`concat`、`map`、`Array.from`），套在另外八个 `join` 里面运行的 `join` 的缓冲区，属性很多的对象，以及 `Map` 和 `Set`。它们的线仍可能和以前一样，最多低出阈值下留着的那些垃圾。许多小块内存也保持原样，比如 `sort` 没有比较函数时为每个元素做的字符串。

测试：`unit-api` 的 `test_gc_block_line` 在 16 MiB 和 32 MiB 的上限下，找出十二个内置函数有垃圾和没有垃圾时的线，要求有垃圾时的线和没有时相差不超过 128 KiB。改之前它的 24 个用例里有 23 个失败。`unit-white` 的 `test_gc_block` 数回收次数：没有上限时，十五块 4 MiB 左右的内存（其中有元素为数组的 `join`，每个元素自己又是一次 `join`，有用一个宽字符填充的 `padStart`，还有 `splice`）一次也不回收。有垃圾挡着时，每块都会回收并且成功。阈值为 -1 时，每块都不回收，以内存不足结束。一块内存要用到的东西（要复制的数组、rope、要改大小的缓冲区）在造垃圾之前做好，所以数到的回收就是这块内存引起的。`test_gc_block_fits` 在离上限 1 MiB、没有垃圾的情况下，做十六次每次 4 KiB 的 `resize`，以及十六次搜索已经摊平过的 rope：它们都不回收，以前每次都回收一次。`test_gc_block_widen` 在有垃圾挡着时，让 `join` 的缓冲区原地变成 16 位宽：要这块宽的内存之前先回收，`join` 成功。
