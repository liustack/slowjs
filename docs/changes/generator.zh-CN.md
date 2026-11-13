---
summary: '引擎从 C 生成时的改动：make gen 用 tifa 页面训练出的性能画像编译（它是什么，怎么发现的，它可以决定什么、哪些被关掉，为什么安全，过期的画像损失什么，它的限制和检查）'
read_when:
  - 想知道 make gen 为什么编译两遍，或 engine/pgo/profile.txt 有什么用
  - make gen 打印了以 "warning: profile:" 或 "profile:" 开头的行，或者改了一处没碰到某个生成函数的 C 之后，那个函数变了
  - 改生成器给 clang、opt 或 llc 的参数，或改生成引擎的内联
  - 想知道画像带来什么、花掉什么，哪些脚本可能因它变慢
  - 重新训练画像，或取入一个改了寄存器循环的 QuickJS 提交
---

# 生成器

[English](generator.md) · 简体中文

`make gen` 怎样把 `engine/` 的 C 变成 `internal/cengine` 里的 Go 汇编。生成器的其余部分见[它的 README](../../internal/goasm/gen/README.md)。这里是会改变生成代码速度的改动。

## tifa 页面的性能画像

发布：Unreleased。

`make gen` 用一份性能画像（profile）编译引擎：引擎渲染真实页面时，每个分支各跑了多少次的计数。LLVM 用这些计数把常走的块放在一起，把溢出（spill）放在最少执行的地方，并排好 `switch` 各分支的顺序。画像是 `engine/pgo/profile.txt`，用 `engine/pgo/pages` 里的页面训练（见 [engine/pgo/README.zh-CN.md](../../engine/pgo/README.zh-CN.md)）。同样的 C 不用画像生成，渲染要慢 3% 到 9%（[performance.zh-CN.md](../performance.zh-CN.md#tifa-页面的性能画像)）。

### make gen 怎么用它

1. 第一遍，和以前一样，不用画像。它还记下哪些调用被内联了（`-Rpass=inline`、`-gline-tables-only`）。
2. 第二遍：clang 和链接后模块上的优化器再跑一次，带 `-fprofile-instr-use`，内联固定成第一遍的结果：第一遍内联了的调用才内联，其余不内联（`-cgscc-inline-replay`，范围 Module，兜底 NeverInline）。llc 对结果只跑一次。

第一遍的记录每次都由当前的 C 重新做出，所以不会过期。画像是一份文本文件，amd64 和 arm64 共用：函数的哈希只取决于 C。`make gen` 用 LLVM 22.1.8 的 `llvm-profdata` 在 `.build` 里把它转成索引格式。输出由 `engine/`、画像文件和参数决定，所以 `make check-gen` 仍然有效。在构建服务器的一个核上（linux/amd64，`GEN_JOBS=1`），`make gen` 生成 amd64 要 4 分 27 秒，不用画像时是 1 分 52 秒，生成 arm64 要 3 分 39 秒。

画像不由 `make gen` 做出，因为同样的训练跑两次，文件里会有几个计数不同。它是入库的文件。`GEN_PGO=0 make gen` 不用画像生成，用来对照，不是正式构建。

### 怎么发现的

第一次尝试在 2026 年 10 月，用的是完整的画像。生成的引擎变慢了：画像会改变内联的结果，两个方向都有。`JS_RegvmRun`（寄存器循环）变大 3.4 倍，它的 Go 栈帧超过栈帧规则的 256 字节，React 页面多跑了 10.1% 的指令（其余页面从少 0.1% 到多 1.8%）。记在[试过但没做的](../performance.zh-CN.md#试过但没做的)里。

第二次尝试在 2026-11-13，内联保持成不带画像的构建所做的结果，其余让画像决定。收益就在这里。研究查过：

- 在 llc 之前把 IR 上的权重拿掉，画像一点收益也没有（渲染比不用画像多跑 0.6% 到 1.3% 的指令）。收益来自 llc：它把块、溢出和活跃值的拆分放在哪里，以及分支的哪一边顺着走下去。
- 收益不是因为见过这个页面。用一半页面训练，在另一半上测，每个页面的收益相同，指令数相差在 0.2 个百分点内。
- 只保留寄存器循环、它最热的 20 个辅助函数和内联进它们的函数的画像，得到约一半收益。
- 在两处小改动（`+` 左边是空字符串，属性写入的一种情况）之前训练的画像，在改过的 C 上保住了全部收益。

研究和数字见 [performance.zh-CN.md](../performance.zh-CN.md#tifa-页面的性能画像)。

### 画像决定什么

允许：

- llc 的块权重和布局，以及 `switch` 各分支的顺序。收益大部分在这里。
- 按训练里看到的循环次数剥出循环的前几圈（peeling），只对估计次数不超过 7 的循环：这几圈被复制到循环前面。不用画像的构建只剥 1 或 2 圈，而且只在代码本身需要的地方。它的代价取决于数据：训练里看到 3 圈、实际跑一千圈的循环，每次进入都要为这些副本付代价，剥过的循环还可能失去原来的展开。把上限压到 2 圈（`-unroll-peel-max-count=2`）是后续要做的事，还没测。
- SimplifyCFG 对训练里 99% 都走同一边的分支，保留成分支，不改成 select，也不合并它的条件。约 108 个函数因此不同。它的代价也取决于数据：数据两边都走时，分支会预测错，而 select 没有分支。没有干净的开关能把它固定住（`likely()` 的分支也由同一个阈值决定），所以允许。
- 本次发布里还有三项决定跟着画像走，测过之后再固定，是后续要做的事：展开器的 flat-loop 阈值（训练里跑不到 5 圈的循环不展开），按块频率向量化（向量化器从画像里读循环次数），LoopSink（把循环前那个块里的工作移进训练里很少跑的循环块）。

关掉的，因为它在训练跑过的地方省一点，在别的数据跑到的地方要多花（`internal/goasm/gen/pgo.go` 里的 `PGOPinned`，clang、opt 和 llc 都会拿到）：

- `-pgso`，对训练里跑得少或没跑过的代码按体积优化。开着它，加载一个 bundle 多跑 5.6% 到 8.2% 的指令，关掉则少 0.8% 到 0.9%。在这些页面里冷的代码，在别的工作里是热的。

找过但没有，在 IR 和 LLVM 22 的代码里都找过：来自画像的冷属性，冷热代码的拆分，非平凡分支的 unswitch，GVN、LICM 和 JumpThreading 里由画像带动的决定。

生成器自己的轮询 pass 在选一个循环的计数器放在哪里（寄存器还是内存）时，不能读画像。训练里平均只看到 3 圈的循环，会让它选内存，在长数组上每圈多花 2 条指令。所以 pass 选的时候，把所有没有 "expected" 标记的分支权重先拿掉（`pass/poll.cpp` 的 `StaticWeights`）。`likely()` 和 `unlikely()` 的权重带这个标记，会留下，pass 自己加的罕见分支（`pollUnlikely`）也用同样的方式标记，也会留下：数字相同的画像权重在 LLVM 里是同一个节点，只有这个标记能把它们分开。在送进 pass 的模块上跑两次，一次原样，一次去掉画像的权重，pass 在 amd64 的全部 3,595 个决定和 arm64 的全部 3,855 个决定上选得一样。`TestPassPollsProfile` 守着这一点。

### 为什么安全

画像只给编译器本来就要做的选择加权。它不增加也不去掉任何检查，所以脚本得到同样的结果，超时和内存不足仍在同样的地方结束调用。

- tifa 的 24 个页面（两个应用的 23 个和 React 的页面）由 tifa 经 Go API 渲染，HTML 和不用画像时相同，在 darwin/arm64 和 amd64 上都是。
- 每个深度测试都通过，深度相同，两个架构都是，用 `go.mod` 的 Go（1.26.6）和 1.27.2 都是。`JS_RegvmRun` 的栈帧不变（它的 shadow 栈帧在 amd64 上是 256 字节，arm64 上是 176 字节，和以前一样）。别的函数有些小变化。Go 栈帧：amd64 上 192 个变小，40 个变大，arm64 上 83 个变小，49 个变大，最多是 `vsnprintf` 加 144 和 176 字节，其次是 `json_next_token` 加 128。shadow 栈帧：amd64 上 4 个变大（`unicode_sequence_prop` 和 `js_string_raw` 加 80，`js_front_drain` 加 32，`js_parse_statement_or_decl1` 加 16），arm64 上 2 个（`unicode_sequence_prop` 加 80，`js_front_drain` 加 32）。它们都不在递归会重复的路径上：每类测试最深处用的 shadow 栈，总共最多多 32 字节（amd64 的 `method` 在 4,998 层时从 7,344 字节到 7,376 字节，解析器多 16 字节），或者变少（amd64 的 `to_string_nested` 少 160 KB，`to_string` 和 `value_of` 少 6%）。arm64 上每个测试用的字节和以前相同。
- arm64 上有一个函数 `JS_GetOwnPropertyNamesInternal` 被去掉 LLVM 保存 LR 的那个证明拒绝，因为画像把一个向量对的重新加载放到了块的末尾。证明现在沿每条路径跟随这类重新加载的地址（`internal/goasm/gen/arm64_lr.go`），仍然拒绝对它的任何读取、间接跳转、让它跨过调用活着的情况，以及把它当参数的寄存器入口叶函数。那段代码本来就是对的。块顺序的任何变化都可能碰上它。
- clang 带画像时写入的 "CG Profile" 模块标志，在链接后优化器之前和之后各去掉一次（`StripCGProfile`），因为 LLVM 22 没有关掉它的开关：优化器会再加一个，校验器就拒绝，llc 还会写出降级器不接受的指令。

这次构建通过的检查：`make gen` 在 macOS 和 Linux 上为两个架构生成相同的字节，上面的深度测试，上面的 HTML，以及经 Go API 跑的 test262 抽样 4,781 个测试（`compare/engines` 里的 `go run ./cmd/jsbench t262`），带不带画像，通过和失败的测试完全相同。C 树上的 test262（33 个已知错误）看不到画像，只有生成的引擎才有画像。

### 过期的画像

训练之后 C 改过的函数没有可用的画像，编译时就像没有画像一样。其余函数保留各自的计数。所以旧画像只会丢掉一部分收益，别的都不会坏。用两个小提交之前训练的画像，收益全部保住，clang 丢掉了 `quickjs.c` 里 2,264 个函数中 1 个函数的画像。

`make gen` 打印它发现的情况，从不因此失败：

- `profile engine/pgo/profile.txt: N functions, M whose C changed since (no profile used: ...), K new (...)` 各列出前 20 个名字。
- 寄存器循环过期时打印 `warning: profile: JS_RegvmRun has no profile ...`。没有它的画像，只剩 60% 到 80% 的收益。
- 改过的函数超过 30 个时打印 `warning: profile: the C of N functions changed ...`。
- `profile: N likely() or unlikely() ...`：C 里的 `likely` 和 `unlikely` 有多少个被训练跑成了反方向（见下）。

`make check-full` 会把这些行按两个架构显示出来（`scripts/check-gen.sh`）。发布前怎么处理它们，见 [releasing.zh-CN.md](../releasing.zh-CN.md#性能画像)。

### 限制

本次发布里已知且未解决的。每一项都是后续要做的事。

- 训练从没跑过的路径可能更慢。`compare/testdata` 的 64 个微基准（callgrind，每个基准的循环每圈的指令数，linux/amd64，是指令数，不是时间）里，18 个比不用画像多跑超过 0.5% 的指令，39 个少跑超过 0.5%，64 个合起来少 1.9%（几何平均）。多跑的：`typed_array_read` +7.5%，`float_arith` +4.9%，`global_read` +4.4%，`array_read` +3.8%，`prop_create` +3.8%，`prop_read` +3.1%，`int_arith` +2.9%，`prop_write` +2.4%，`prop_update` +2.1%，`array_for` +2.1%，`empty_down_loop2` +2.0%，`empty_do_loop` +1.6%，`empty_down_loop` +1.0%，另有五个在 +0.5% 到 +1.0% 之间（`string_build2c`、`string_build1x`、`string_build1`、`string_build_large1`、`global_func_call`）。页面不训练这些循环。把它们加进画像是后续要做的事，基准分成两半，这样没训练的那一半仍能说明没训练过的路径要多花多少。
- 内联重放会漏掉少数调用。在一组互相成环调用的函数里，画像改变了内联器遇到调用的顺序，于是一个调用在第二遍里出现在第一遍记录没有命名的上下文中，仍是调用。amd64 上是 8 个函数里的 22 个调用：`rv_putel_slow`（10 个），`JS_ToBigInt64Free`（4 个），`free_gc_object`（3 个），`rv_w_gen`、`js_set_fast_array_element_out_of_line`、`js_backtrace_build`、`JS_ToPrimitiveFree` 和 `JS_SetPropertyValue`（各 1 个）。它们调用的 8 个小函数，在不用画像的构建里处处内联，这里各自留下一个函数体：`JS_FreeValue`、`JS_IsException`、`JS_StringToBigIntErr`、`JS_ToNumberFree`、`JS_VALUE_IS_NAN`、`__JS_NewShortBigInt`、`__js_rc` 和 `js_dbuf_init`。arm64 上是其中 5 个函数里的 12 个调用，调用其中 5 个小函数。它们都是慢路径或少见的路径：往类型化数组写一个不是数字的值，BigInt 转换，释放 async 函数的帧，生成回溯。这 8 个函数的栈帧里，只有 `free_gc_object` 的 Go 栈帧变大，两个架构都是 32 字节（amd64 从 96 到 128，arm64 从 88 到 120），它的 shadow 栈帧仍是 0。它在一个调用环上（`free_gc_object`、`__JS_FreeValueRT`、`free_gc_object`），但 C 只让这个环走一层（`engine/quickjs.c` 的 `free_zero_refcount`：一个对象在释放别的对象时死掉，只会加进一个列表），所以没有递归会重复这 32 字节，也没有深度下限会动。其余栈帧相同或变小。在链接后优化器里把这些调用内联，是后续要做的事。
- 过期的画像会丢掉它覆盖的函数里的收益（见上），C 离训练越远，收益越少，直到重新训练。
- 画像来自两个应用的页面，用一个手写格式化数字和日期的假 `Intl` 渲染（[engine/pgo/README.zh-CN.md](../../engine/pgo/README.zh-CN.md)）。它没有 `Intl` 内部工作的计数，也没有加载 bundle 的计数（这些计数是特意去掉的：带上它们的画像被解析器主导，效果更差）。
- 训练两次得到的文件有几个计数不同：Mac 上训练两次，47,170 行里 40 行不同，Linux 上一次和 Mac 上一次，45 行不同。它们生成的引擎也不同（两次 Mac 训练之间，amd64 上 27 个函数不同）。所以训练不可复现，入库的文件可复现：`make gen` 用它在 macOS 和 Linux 上生成相同的字节。
- LLVM 的画像格式在版本之间会变，所以换 LLVM 就要重新训练。版本固定为 22.1.8。
- 读 `.s` 的人看不出某个块为什么移动了。画像和训练它的程序都在仓库里，就是为了这个。
- 保存优化记录（`-fsave-optimization-record`、`-opt-record-file`）会改变带画像构建的代码：它打开 hotness，让块频率信息一直保留，有 2 个函数因此不同。正式构建从不用它。想看优化器做了什么，在一份副本上生成记录，并且不要把那份输出和入库的文件比较。
- 生成器里开着 `-Wmisexpect`，容差 50%。它指出每个训练里多数时候跑成反方向的 `likely()` 或 `unlikely()`，写进 `.build/w-<arch>/pgo/misexpect.txt`（约 50 项，每次训练会多几项或少几项）。开不开这个标志，IR 逐字节相同。它告诉你 C 里哪些提示页面不支持，不改变任何东西。
- 在 Apple M4 上收益较小（约 3%），在 AMD EPYC 服务器上是 6% 到 9%：M4 的指令缓存大 6 倍，画像去掉的缺失在那里代价更小。

### 测试和检查

- `engine/pgo/train.sh` 训练画像。它按生成器编译引擎的方式构建宿主，所以画像里每个函数的哈希和生成器看到的一致。
- `TestInlineRemarks`、`TestStripCGProfile`、`TestProfileNames`、`TestUnprofiledFunctions`、`TestStaleWarnings` 和 `TestMisexpectLines`（`internal/goasm/gen/pgo_test.go`）检查第一遍的记录、模块标志、画像里的名字、哪些函数没有画像，以及警告。
- `TestPassPollsProfile`（`internal/goasm/gen/poll_test.go`，需要 `QUICKJS_LLVM` 和 `QUICKJS_SHADOW_PASS`）检查轮询 pass 选计数器位置的方式和没有画像时相同，画像里某个权重的数字恰好和 pass 自己的罕见分支相同时也一样。
- `TestLowerARM64EngineLRTuplePaths`（`internal/goasm/gen`）检查 arm64 上扩展后的证明。
- `scripts/check-gen-test.sh` 检查 `scripts/check-gen.sh` 总会显示画像的行。它在 `scripts/check.sh` 的 `scripts` 步骤里运行。
- `compare/engines` 里的 `go run ./cmd/notices` 为训练用 bundle 里的包做声明文件（`TestGeneratePages`）。
