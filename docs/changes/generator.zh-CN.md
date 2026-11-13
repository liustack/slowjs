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

第一遍的记录每次都由当前的 C 重新做出，所以不会过期。画像是一份文本文件，amd64 和 arm64 共用：函数的哈希只取决于 C。`make gen` 用 LLVM 22.1.8 的 `llvm-profdata` 在 `.build` 里把它转成索引格式。输出由 `engine/`、画像文件和参数决定，所以 `make check-gen` 仍然有效。`make gen` 大约要 PGO2-FILL: time of make gen per architecture, with the Mac and linux numbers，以前约 2 分钟。

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
- 按训练里看到的循环次数展开循环（peeling），只对估计次数小于 7 的循环。这一项还没定：PGO2-FILL: whether a cap of 2 turns is in the release, and what it did to the micro benchmarks。
- SimplifyCFG 对训练里 99% 都走同一边的分支，保留成分支，不改成 select，也不合并它的条件。约 108 个函数因此不同。没有干净的开关能把它固定住，所以允许。

关掉的，因为每一项在训练跑过的地方省一点，在别的数据跑到的地方要多花（`internal/goasm/gen/pgo.go` 里的 `PGOPinned`）：

- `-pgso`，对训练里跑得少或没跑过的代码按体积优化。开着它，加载一个 bundle 多跑 5.6% 到 8.2% 的指令，关掉则少 0.8% 到 0.9%。在这些页面里冷的代码，在别的工作里是热的。
- 展开器的 flat-loop 阈值，让训练里跑不到 5 圈的循环仍像以前一样展开。
- 按块频率向量化，让向量化器不从画像里读循环次数。
- LoopSink，它把循环前那个块里的工作移进训练里很少跑的循环的块。

找过但没有，在 IR 和 LLVM 22 的代码里都找过：来自画像的冷属性，冷热代码的拆分，非平凡分支的 unswitch，GVN、LICM 和 JumpThreading 里由画像带动的决定。

生成器自己的轮询 pass 在选一个循环的计数器放在哪里（寄存器还是内存）时，不能读画像。训练里平均只看到 3 圈的循环，会让它选内存，在长数组上每圈多花 2 条指令。PGO2-FILL: say whether the fix is in this release。如果在：pass 选的时候把画像的分支权重先拿掉（`pass/poll.cpp` 的 `StaticWeights`），`TestPassPollsProfile` 守着这一点。如果不在：训练里很少圈数的循环，计数器仍放在内存，这是长数组上已知的损失，也是后续要做的事。

### 为什么安全

画像只给编译器本来就要做的选择加权。它不增加也不去掉任何检查，所以脚本得到同样的结果，超时和内存不足仍在同样的地方结束调用。

- 训练集每个页面的 HTML 和不用画像时相同（研究渲染过的每个页面，两个架构上都是）。
- 每个深度测试都通过，深度相同。`JS_RegvmRun` 的栈帧不变（它的 shadow 栈帧在 amd64 上是 256 字节，arm64 上是 176 字节，和以前一样）。别的函数有些小变化：amd64 上 254 个函数的 Go 栈帧变小，34 个变大（`json_next_token` 最多加 112 字节），4 或 5 个 shadow 栈帧变大（`unicode_sequence_prop` 加 80）。经过它们的深递归，余量略少一点。
- arm64 上有一个函数 `JS_GetOwnPropertyNamesInternal` 被去掉 LLVM 保存 LR 的那个证明拒绝，因为画像把一个向量对的重新加载放到了块的末尾。证明现在沿每条路径跟随这类重新加载的地址（`internal/goasm/gen/arm64_lr.go`），仍然拒绝对它的任何读取、间接跳转、让它跨过调用活着的情况，以及把它当参数的寄存器入口叶函数。那段代码本来就是对的。块顺序的任何变化都可能碰上它。
- clang 带画像时写入的 "CG Profile" 模块标志，在链接后优化器之前和之后各去掉一次（`StripCGProfile`），因为 LLVM 22 没有关掉它的开关：优化器会再加一个，校验器就拒绝，llc 还会写出降级器不接受的指令。

PGO2-FILL: the gates of the final build: test262 (known errors 33), `make check-full` on amd64 and arm64, the lowest Go version, the depth floors。

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

- 训练从没跑过的路径可能更慢。四个微基准比不用画像多跑指令（是指令数，不是时间）：`array_slice` +12.5%，`typed_array_read` +5.7%，`prop_create` +3.9%，`array_prop_create` +1.9%。PGO2-FILL: final numbers of these four and of any other benchmark that runs more。
- 内联重放漏掉了递归调用组里 7 个函数中约 25 个调用点：画像把这些调用复制了，第一遍的记录没有给副本命名。这些副本仍是调用。PGO2-FILL: say whether they are fixed or listed, and the list。
- 过期的画像会丢掉它覆盖的函数里的收益（见上），C 离训练越远，收益越少，直到重新训练。
- 画像来自两个应用的页面，用一个手写格式化数字和日期的假 `Intl` 渲染（[engine/pgo/README.zh-CN.md](../../engine/pgo/README.zh-CN.md)）。它没有 `Intl` 内部工作的计数，也没有加载 bundle 的计数（这些计数是特意去掉的：带上它们的画像被解析器主导，效果更差）。
- 同样的训练跑两次，约 47,129 行里有 43 行不同，所以训练不能逐字节复现。能复现的是入库的那个文件。
- LLVM 的画像格式在版本之间会变，所以换 LLVM 就要重新训练。版本固定为 22.1.8。
- 读 `.s` 的人看不出某个块为什么移动了。画像和训练它的程序都在仓库里，就是为了这个。
- 保存优化记录（`-fsave-optimization-record`、`-opt-record-file`）会改变带画像构建的代码：它打开 hotness，让块频率信息一直保留，有 2 个函数因此不同。正式构建从不用它。想看优化器做了什么，在一份副本上生成记录，并且不要把那份输出和入库的文件比较。
- 生成器里开着 `-Wmisexpect`，容差 50%。它指出每个训练里多数时候跑成反方向的 `likely()` 或 `unlikely()`，写进 `.build/w-<arch>/pgo/misexpect.txt`（现在有 48 项）。开不开这个标志，IR 逐字节相同。它告诉你 C 里哪些提示页面不支持，不改变任何东西。
- 在 Apple M4 上收益较小（约 3%），在 AMD EPYC 服务器上是 6% 到 9%：M4 的指令缓存大 6 倍，画像去掉的缺失在那里代价更小。

### 测试和检查

- `engine/pgo/train.sh` 训练画像。它按生成器编译引擎的方式构建宿主，所以画像里每个函数的哈希和生成器看到的一致。
- `TestInlineRemarks`、`TestStripCGProfile`、`TestProfileNames`、`TestUnprofiledFunctions`、`TestStaleWarnings` 和 `TestMisexpectLines`（`internal/goasm/gen/pgo_test.go`）检查第一遍的记录、模块标志、画像里的名字、哪些函数没有画像，以及警告。
- `TestPassPollsProfile`（`internal/goasm/gen/poll_test.go`，需要 `QUICKJS_LLVM` 和 `QUICKJS_SHADOW_PASS`）检查轮询 pass 选计数器位置的方式和没有画像时相同。PGO2-FILL: keep or drop this line with the poll fix。
- `TestLowerARM64EngineLRTuplePaths`（`internal/goasm/gen`）检查 arm64 上扩展后的证明。
- `scripts/check-gen-test.sh` 检查 `scripts/check-gen.sh` 总会显示画像的行。它在 `scripts/check.sh` 的 `scripts` 步骤里运行。
- `compare/engines` 里的 `go run ./cmd/notices` 为训练用 bundle 里的包做声明文件（`TestGeneratePages`）。
