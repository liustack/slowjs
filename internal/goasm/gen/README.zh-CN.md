# 引擎生成器

[English](README.md) · 简体中文

生成器把 `engine/` 里的 C 引擎变成 `internal/cengine` 里的 Go 汇编。它处理 amd64 和 arm64 上的调用约定、栈规则和影子栈。

它是维护者用的工具：它调用 LLVM 22.1.8（clang、opt、llc），并用 Go 自己做检查和 lowering（把 LLVM 的中间表示换成目标机器的指令）（`debug/elf`、`golang.org/x/arch/x86/x86asm`）。应用程序从不运行它。

## 流水线

1. clang -O2 生成 IR，保留帧指针，不用红区，有符号溢出按回绕处理，不做尾调用。在 amd64 上，栈声明为 8 字节对齐（Go 的栈就是这样），目标特性 slow-two-mem-ops 让 llc 不要把调用目标的加载折叠进调用里，所以每个间接调用都经过寄存器。在 arm64 上，目标是 aarch64-linux-gnu，x18 和 x26 到 x28 固定，x29 保留，LR 留给返回地址。可移植的参数（独立运行环境、不用系统头文件、有符号溢出回绕、unsigned char 等）不写在这里：init 运行 `engine/test/flags.sh`（C 树的脚本也读它），并追加它给出的内容（`TestFlagsComeFromFlagsSh` 防止 Go 的列表和 `cycles.sh` 重复它们）。在 amd64 上，`quickjs.c` 是唯一持有 JSValue 的文件，它还会得到 `-fno-slp-vectorize` 和目标特性 `slow-unaligned-mem-16`（`EngineValueFlags`），这样 LLVM 会把一个 JSValue（16 字节）复制成两次 8 字节的移动，而不是一次 SSE 移动：解释器一次写入它的两半，而 Zen 3 无法从它们转发一次 16 字节的加载（见 [c-engine.md](../../../docs/changes/c-engine.zh-CN.md#jsvalue-如何复制)）。小 libc 不在此列，因为它的 `memcpy` 有意每次移动 16 字节。CheckIR 拒绝可变参数、byval、sret 和 long double。
2. 影子栈 pass（`pass/shadow.cpp`）把每个被取过地址的局部变量和动态 alloca 移到 VM 的影子栈上：每个引擎函数的参数 0 是第一个空闲的影子栈字节，每个函数把它自己所有存活数据之上的那个字节交给被调用者，`__shadow_remaining()` 把剩余空间交给 C，供它的 RangeError 检查使用。不允许留下任何 alloca。需要的整数参数寄存器比目标允许的更多的函数，变成 `f(sp, blk)`：每个调用点把参数存进它的影子栈帧上的一个块里。带 `-shadow-cc=preserve_none` 时，每个统一入口、宿主回调和对它们的调用都使用 preserve_none。寄存器入口的叶子函数仍然是 C，并得到一个保留的帧指针：没有栈帧，llc 也从不分配 RBP，因为它归 Go 所有。每个统一函数（unified function）和每个对它的调用，把 i1、i8 或 i16 当作 i32 来接收和返回：调用者负责扩展它（按 C 的规定做符号扩展或零扩展，bool 做零扩展），被调用者负责截断它。preserve_none 只把 32 位和 64 位整数放进它自己的寄存器（amd64 从 R12 起，arm64 从 X20 起），把窄的留给 C 的规则（EDI 或 W0 起），而 Go 写的是整个字。所以 pass 报告的签名里没有窄整数，生成器会拒绝有窄整数的签名（也拒绝浮点，Go 会把它当作整个 float64 来移动）。带 `-shadow-icall-sigs` 时，每个间接调用都得到一个 `"kcfi"` 操作数束，其类型 id 编码了它的签名（`icall.go`）。没有 `"kcfi"` 模块标志时，llc 不为它生成检查代码，但会把 id 保留在机器调用上。`-shadow-probes` 写出一个模块，每个统一定义一个探针，带它的类型、约定和属性，探针读取每个参数并返回一个加载出来的结果。不留任何陷阱指令：每个 `llvm.trap`（`abort` 的 `__builtin_trap`，以及 pass 自己对会越过影子栈末尾的动态 alloca 做的检查）都变成对 `host_trap(sp)` 的调用，这是宿主的一个会 panic 的回调，因为 SIGILL 和 SIGTRAP 无论如何都会结束 Go 进程。LLVM 22.1.8 在这两个 linux 三元组上，不会为 `unreachable` 生成陷阱，所以 `llvm.trap` 是唯一的来源。`debugtrap` 和 `ubsantrap` 会让 pass 停下，叶子函数里的陷阱也会，因为叶子函数没有影子指针可传。
3. llc -O2，关闭 shrink-wrapping，写出汇编和一个对象文件，每个函数都放在 `.text` 里（`-profile-guided-section-prefix=false`：否则像 `abort` 这样的冷函数会进 `.text.unlikely.`）。整个引擎用 `-load` 加载 `pass/icall.cpp` 和 `pass/facts.cpp`（和 pass 构建在同一个共享对象里）：在这次运行有的机器 pass 之后（outliner 和函数拆分是关闭的，如果这点变了，插入点必须重新检查），icall.cpp 在每个间接调用处命名一个符号 `goasm.icall.<function>.<n>.<id>`，即文本里调用之前的一个标签，以及对象文件里它地址上的一个局部符号。没有 id 的间接调用会让 llc 停下。facts.cpp（`-goasm-facts`）记下 llc 分配的寄存器：每个函数的参数寄存器，它的代码在入口处读取的那些，以及每个调用和返回的参数、结果和存活寄存器（`facts.go`）。llc 也在探针模块上运行，为的是得到每个参数和结果的寄存器。CheckObject 拒绝可写的或线程局部的数据，以及引擎和宿主回调之外的任何外部符号。
4. LowerUnified / LowerRegister（amd64）和 LowerARM64 把每个函数改写成文本：去掉函数序言和尾声，在 Go 的序言之后把 Go 栈上的参数加载进原生的参数寄存器，就地展开每个调用（保留模式要求保留的寄存器，把参数移到传出区域，调用，读结果，恢复），其中对驱动程序所 lowering 的统一函数的调用，直接从被调用者存放结果的寄存器里取结果，不从 Go 栈上读回任何东西（`LowerEnv.RegResult`），把 LLVM 的局部变量移到 Go 的栈帧上，把常量池放在函数里面，并把每个返回变成跳到同一个 Go RET。一个函数在 lowering 后的对象里的起点，就是它的函数体在二进制里的位置（`FuncPlace`：按 16 字节，或者在 64 字节一行里它的 `-align64` 位置），这样每个池和表的 `.p2align` 在代码运行的地方成立。它不认识的任何形态都会失败：未知的调用形式，经过内存的调用，尾跳转，对 SP 的写入，逃逸的栈地址，对齐的或折叠了的打包栈操作数（Go 栈只有 8 字节对齐），常量池之外的只读符号，被禁止的指令。用到 RBP 的叶子函数也会失败。
5. clang -c（arm64 上是 llvm-mc）把 lowering 后的文本重新汇编，这样每个分支偏移都固定下来。
6. Emit 写出 Go 汇编：每个函数一个 TEXT，函数体是字节，每个调用都是 Go 看得见的 CALL 或 BL，每次加载参数和存储结果都是经过 FP 按名字的移动（`a0+0(FP)`、`ret+24(FP)`，名字来自 `GoSlots`，与 Go 声明里的一样）。Go 承诺的是按这种方式找到参数，而不是在离 SP 某个偏移的地方，那个偏移取决于汇编器怎样排布栈帧。在 amd64 上，lowering 后的文本里这条移动仍然写在 Go 现在用的偏移上，好让 clang 排好它前后的分支，Go 的汇编器写出的字节也一样（`TestFPMovesAssembleAsLowered`）。其他任何从 SP 够到栈帧之上的指令都会让构建停下，函数代码之后的数据跟在一行 `// data` 后面。整个引擎的 `.s` 文件的大小上限按每个函数的指令都写成字节时的文本来算，这样按名字的移动不会把函数挪到另一个文件里，也就不会挪到链接器排布函数的顺序里的另一个位置。
7. VerifyLinked 检查链接后的二进制：函数体字节没变，每个不在 NOSPLIT 列表里的入口和 `host_poll` 上都有栈检查的序言，列表上的入口则没有，windows 上每个统一入口都有给故障留空间的检查，函数表只指向统一入口，没有隐藏的调用或尾跳转。

## 调用模式

`-mode` 选择一次统一调用如何保存 C 的被调用者保存寄存器，这些寄存器会被 Go 代码和 morestack 破坏：

- `body`（默认）：调用点保存调用方函数体用到的那些（amd64 的 RBX、R12 到 R15，arm64 的 R19 到 R25 和 F8 到 F15）。
- `all`：调用点保存所有这些（用于对比）。
- `stub`（arm64）：调用经过一个保存全部 15 个的桩（用于对比）。
- `pn`：代码用 preserve_none 编译，所以 LLVM 只在自己的栈帧里保留跨调用仍然存活的东西，没有调用点保存任何东西。这时参数使用 amd64 的 R12 到 R15、RDI、RSI、RDX、RCX、R8、R9，和 arm64 的 X20 到 X25。arm64 的第七个参数会落在 X26，扫描会拒绝它，所以 pass 把更长的参数列表移进一个块里。

`pn` 是引擎用的模式：它和 `body` 一样快或者更快，调用点什么都不保存，这让 lowering 最简单。

`body` 配 `GEN_NO_SAVES=1` 和 `pn` 配 `GEN_NO_PACK=1` 是反向构建，必须让测试和寄存器扫描失败。

## 运行

测试包是一个小的 C 样本模块，单独用来演练生成器。`proto.sh`（在 liustack/quickjs 的根目录运行，linux/amd64）构建 pass，把它生成为 `internal/proto`（不提交进仓库），构建它的测试，并运行链接后的验证。`ARCH=arm64` 生成 arm64 的包并交叉构建它的测试：把包复制到 arm64 机器上运行它们。`MODE` 是调用模式。设置 `CALL_OVERHEAD=1` 可以运行调用开销的计时。它的测试是 `testdata/*.txt`，以文本形式保存，这样这个包不会编译它们。这里的 `go test` 运行规则测试。设置了 `QUICKJS_LLVM`（LLVM 的 bin 目录）和 `QUICKJS_SHADOW_PASS`（构建好的 pass）后，它还会让 `testdata/reject` 里的每个样本走一遍真实的流水线，再设置 `QUICKJS_LLD`（ld.lld），`TestNarrowArgs` 会原生地和生成后各运行一次 `testdata/narrow`，`TestSampleRuns` 对 `testdata/run` 里的每个样本做同样的事：每个样本定义 `long run(long *r, int one)`，原生运行（也在 UBSan 下），并从 Go 里在 body、pn 和 pn 毒化构建中运行，必须打印同样的行，并且链接后的检查必须通过。`internal/enginecmp` 在和这些测试相同的 GC 停止设置下，测量当前的引擎。

## 整个引擎

`cmd/engine` 把同样的流水线跑遍 `engine/` 和 `engine/libc` 的全部内容，当作一个模块，并写出一个 Go 包（`-out`）：大小受限的 .s 文件里的函数，作为 `engine_rodata` 的只读数据，以及 Go 声明。宿主提供 `engine/host.h` 的回调和 `goasmMissing`，每个桩用它在 `missingNames` 里的下标调用它。`-allow` 指定这次运行接受的更多外部符号。默认一个也不指定。被拒绝时运行会停下，什么也不写（pass 的拒绝是一行 `LLVM ERROR:` 加退出状态 1，而不是带着 opt 的栈的 abort），默认没有桩。`-keep-going` 用于勘察：它按类别列出 C 必须改的每个函数，并为它们生成桩，而不是停下。`-release` 是正式构建：遇到任何桩都失败，需要 `-out`，并在做任何工作之前，拒绝所有为勘察和测试做的开关（`-keep-going`、`-loose-varargs`、`-poison`、`GEN_KEEP_NARROW`、`GEN_EDIT_LOWERED`、改动过的 `-non-format-variadic`、不是 `auto` 的 `-postlink-opt`、改动过的 `-postlink-opt-args`）。`-mode` 是 `body` 或 `pn`。`-varargs-block` 把可变参数放进影子栈的一个块里传递，而不是拒绝它们。`-shadow-size-log2` 是引擎所针对的影子栈区域的大小（默认 20，宿主完整的 32 MiB 是 25，范围 16 到 40：驱动程序和 pass 在使用之前会拒绝其他任何值）：`engine_decl_<arch>.go` 把它记录为 `shadowSizeLog2`，宿主会拒绝更小的栈。

文件逐个编译（上面的第 1 步，对写出的 IR 做格式检查），再用 llvm-link 链接。在 amd64 和 arm64 上，优化器随后在链接后的模块上运行（`-postlink-opt`，默认 `auto`：`PostLinkOptFor`），在影子栈 pass 之前，带 `PostLinkOptArgs`：`-O2`，`engine/test/flags.sh` 给 clang 的 400 的内联阈值，以及关闭 SLP 向量化器。clang 之前是对每个文件单独优化的，所以从一个文件到另一个文件的调用（小 libc，分配器，以及堆进入解释器和回收器的调用）从未被内联。第二次运行做了这件事。它让原生 C 里一个页面的指令数减少 1.5% 到 1.7%，在 amd64 生成的引擎里减少 0.6% 到 1.2%，在 arm64 的引擎里减少 0.6% 到 1.0%，并使生成的汇编变大 13%（amd64 上代码大 19%）。内联阈值取 1000 会让解释器变大，它的栈帧高出 32 字节：不采用。

clang 从命令行取的参数，以函数属性的形式随 IR 一起传递（`quickjs.c` 的 `+slow-unaligned-mem-16`），并留在它们的函数上。`-fno-slp-vectorize` 是例外，因为它是流水线的一个选项而不是属性：第二次 -O2 会把一个 JSValue 的两次 8 字节移动再合并成一次 16 字节的（`JS_CallInternal` 的 IR 里有 52 行向量代码）。SLP 向量化器用一个没有任何树能达到的阈值关掉（`TestPostLinkOptKeepsValueFlags`）。

优化器会改名字，生成器里每个按名字行事的地方，读的都是它之后的模块：

- 被内联进它所有调用者的静态函数没有了（pass 会报告，Go 声明和 `engine_keep` 列表里是模块里有的东西）。被内联的外部函数仍然被定义和声明，因为 Go 一侧可能调用它。
- 它会为一次调用的常量参数克隆一个函数（`mark_children.specialized.2`）。克隆是和其他函数一样的统一函数，Go 名字是 `mark_children_specialized_2`（`GoNames`），它的代码跟着它所来自的函数的文件走（`cloneGroups`、`cloneOf`，它们也处理 llvm-link 加的编号）。寄存器入口的叶子函数（`-leaves`、`-shadow-leaf`）只是那些名字的函数，没有别的：`memcpy` 的克隆会是一个统一函数。
- `-align64`（`lre_exec_backtrack`）、`DispatchCopies`、宿主回调、链接后检查的 `host_poll` 和 `js_host_layout`，指名的是外部函数和数据，优化器会按它们的名字保留。

`-postlink-opt=on` 或 `off` 强制这次运行，`-postlink-opt-args` 改变它的选项，供测试和勘察使用（`-release` 两者都拒绝）。`TestPostLinkOpt`（amd64 和 arm64，对为每种架构交叉构建的模块做链接后检查）和 `TestSampleRuns` 的 `postlink.c` 样本（它运行代码，只在 linux/amd64 上）涵盖了这一切。

`-arch` 是 `amd64`（默认）或 `arm64`，驱动程序写出的每个文件都以架构名结尾（`engine_arm64.s`、`engine_decl_arm64.go`、`engine_keep_arm64.go`），所以一个目录可以放两种，Go 会挑其中一种。一次构建只删除它自己架构的文件。`-build-tag T` 把标签 T 放在每个文件的 `//go:build` 行里架构名的前面（`.s` 文件是 `T && amd64`，`engine_decl_<arch>.go` 是 `T`）。`scripts/gen-engine.sh` 不传标签：提交进仓库的文件只带架构名（`amd64`、`arm64`）。arm64 的引擎在 darwin/arm64 和 linux/arm64 上运行。arm64 只接受 `-mode pn`：preserve_none 在那里把整数放进 X20 到 X25，它的下一批寄存器 X26 到 X28 归 Go。其他模式只在测试包里有（`proto.sh`）。arm64 没有 `-poison` 或 `-loose-varargs` 构建，并拒绝这两者。它在 `LowerARM64EngineFunc`（`arm64_engine.go`）里 lowering 每个函数，并让每个入口、调用和返回都限定在 llc 的 facts 和探针所指名的寄存器里，就像 amd64 的链接后检查一样：窄整数在另一个寄存器里，结果从另一个寄存器读，或者出现 llc 的 facts 里没有的调用，都会让构建停下。llc 用 tiny 代码模型写 arm64（`-code-model=tiny`），所以每个对数据的引用是一条指令，指名整个地址，lowering 单独把每一个改写成同一个地址：对 `engine_rodata` 或一个函数的 `adr` 变成 Go 的 `MOVD $sym(SB)`，对函数自己的常量池或跳转表的 `adr` 保持相对 PC，对一个标签的加载读函数体里的数据（当一个函数这样加载时，计划把一个对象复制到那里，amd64 对经过 PC 的加载也是这样做的）。small 代码模型的页形式，`adrp` 和 `:lo12:`，会让构建停下：它们的第二条指令只有在寄存器仍持有第一条指令的页时才对，而没有哪一条指令能表明这一点。llc 为检查写出的对象每个函数一个节，因为从一个函数到另一个函数的 `adr` 可能超出它够得到的 1 MiB。它拒绝 `svc`、系统寄存器、X18、作为值的 X26 到 X29、保存之外的 LR、尾跳转、序言之后还会增长的栈帧、逃逸的栈地址和融合乘加（`fmadd`、`fmla` 等），每项都有一个测试（`TestLowerARM64EngineRejects`、`TestLowerARM64EngineFacts`、`TestLowerARM64EngineTiny`）。

参数需要的整数寄存器比统一入口所有的更多（amd64 是 10 个，arm64 是 6 个）的函数，改为接收影子指针和一个参数块的地址（`f(sp, blk)`）。它生成的入口叫 `<name>_block`，驱动程序按它的 C 名字和签名，把一个 Go 函数写进 `engine_decl_<arch>.go`（`packed.go` 里的 `PackedGo`）：它把每个参数存进影子指针处一个块里它自己的 8 字节槽（JSValue 是 16 字节），块的布局和 pass 排的一样，然后用块之上的影子指针调用入口。所以宿主在每种架构上都按 C 签名调用 API，而哪些函数被打包则因架构而异（amd64 上 20 个，arm64 上 205 个）。

同样的 C 给出同样的包，逐字节相同，不管编译任务的数量（`-j`）和编译完成的顺序如何，也没有文件含有工作目录的路径（`TestOutputReproducible`）。

引擎会轮询（`pass/poll.cpp`，见 [c-engine.md](../../../docs/changes/c-engine.zh-CN.md#轮询与中断)）：代码的每个环都对一个计数器倒数，这个计数器位于影子栈区域的基址之后 `-poll-offset` 个字节处（`gen.PollCounterOffset`，384，`engine_decl_<arch>.go` 把它记录为 `pollCounterOffset`），耗尽时调用 `host_poll`。`-poll=false` 构建一个没有轮询的引擎，用来测量它们的代价，`-release` 拒绝它。解释器自己对它的字节码计数：C 标出位置（`engine/cutils.h` 里的 `poll_count`，在 `JS_RegvmRun` 的入口，以及 `pc` 不向前走的地方），pass 在每个标记处放一次计数，进入它的分发的边则不放。这样分发保持很短，llc 在寄存器分配之后把它复制到每个处理函数的末尾，它对最多 10 条指令的计算 goto 会这样做。没有这些副本，解释器会慢得多。驱动程序统计 llc 文本里每个带计算 goto 表的函数的间接跳转数，低于 `gen.DispatchCopies`（`JS_RegvmRun` 是 42，它在 amd64 上有 59 个）就停下。pass 检查自己的输出：把做计数或轮询的块去掉，把它证明为很短的环的回边去掉，再去掉进入解释器分发的边之后，没有函数还留着环。解释器必须在入口处计数，它的每个标记都必须紧跟在一次与其权重相符的计数之后。每次计数至少取 1，耗尽时就轮询：它的分支测试它是否小于等于 0，并在走的那一侧调用 `host_poll`。每次计数都从计数的当前值扣除，每次对计数器的存储都存当前的计数，每个可能轮询的调用和每次返回，都在内存里找到计数：把它放在寄存器里的代码，在这样的调用之前存储它，在调用之后加载它，并在离开它的地方存储它。隐藏选项 `-shadow-poll-fault` 故意破坏其中一项，让测试能看到检查让 pass 停下。

没有栈检查的函数：每个统一入口都以 Go 的栈检查开头，除非它的 TEXT 写着 NOSPLIT，而生成器不选任何函数不带栈检查（`MarkNoSplit`、`nosplit.go`）。NOSPLIT 列表里只有不可能有栈检查的函数：寄存器入口的叶子函数，以及 Go 汇编器自己标为 NOSPLIT 的叶子函数（不调用任何东西，带保存的基址指针的栈帧小于 128 字节，由 `AMD64Stack` 和 `ARM64Stack` 计算，它们对栈帧取整的方式不同）。自动的叶子函数什么也不调用，寄存器入口的叶子函数只调用别的寄存器入口的叶子函数（引擎里的三个什么也不调用）：驱动程序在遇到任何其他调用、它们之间的环，以及在其入口以下占用 128 字节或更多的调用链时失败。叶子函数里的一次错误会变成压到 goroutine 栈上的一次对 `runtime.sigpanic` 的调用，而 sigpanic 的栈检查可能调用 morestack：这需要 amd64 上 16 字节、arm64 上 24 字节，而叶子函数的调用者的栈检查至少留下了 800 字节，减去叶子函数的栈帧。每个递归都经过栈检查，栈在那里增长，运行时在那里可以停下 goroutine，每个循环仍然调用 `host_poll`，它自己的栈检查就是安全点。链接器在每次构建时都会再检查每条 NOSPLIT 调用链，包括 Go 函数值的 ABI 包装函数。驱动程序打印列表的大小，把列表上的每个函数连同它的种类和栈帧写到工作目录里的 `nosplit.txt`，`engine_decl_<arch>.go` 在 `noSplitList` 里列出它们，供测试使用。生成器在列表上每个函数的 TEXT 里写 NOSPLIT，链接后的检查让每个入口的栈检查与此相符：Go 汇编器自己把另一个叶子函数标为 NOSPLIT，或者不再标某个，都会让检查失败。

windows 上给故障留的空间：windows 把故障时 CPU 的状态写在栈指针下面，也就是 goroutine 的栈上，而 Go 在每个栈下面只给它留了 4 KiB（golang/go#81238），比带 AMX 的 CPU 需要的少。所以只在 windows 的汇编里（`#ifdef GOOS_windows`），每个统一函数在 Go 的序言之后检查它的栈指针减去 `RoomBelow`（12 KiB，`emit.go`）是否还在 g 里的 guard 之上，不在就调用 `goasm_room`：这个函数什么也不做，栈帧是 `RoomFrame` 字节，它自己的栈检查让运行时把栈加大。NOSPLIT 列表上的函数先载入 g，没有栈帧的函数还写上 NOFRAME，这样这次调用不改变它的栈帧。跳过这次调用的分支落在一个单字节的空操作上，而不是函数体的 `PCALIGN` 上：Go 1.26.11 和 1.27.4 之前，遇到跳向 `PCALIGN` 的前向分支会无限循环（golang/go#81792）。寄存器入口的叶子函数没有这个检查，它的调用者给它留了空间。宿主的回调在返回引擎之前再留一次空间（`internal/cengine/host.go`）。linux 和 darwin 上的代码和原来一样。对 windows 二进制的链接后检查要求每个统一函数都有这个检查、它对 `goasm_room` 的调用和跳到这次调用之后的分支，并要求 `goasm_room` 的栈检查在它自己的栈指针下面至少要 `RoomBelow` - 8 字节，这样它的调用者就有 `RoomBelow`。`TestPostLinkFormats` 在一个 windows 二进制里逐一改掉这些，检查必须拒绝每一处。具体数字见 docs/changes/c-engine.zh-CN.md（引擎出错时）。

函数体从哪里开始：在 Go 的序言之后，每个函数都有一个 `PCALIGN $16`，所以函数体从 16 字节处开始，当它前面的代码增长时，会在它那 64 字节的行内移动。`-align64 name:offset,...` 把每个被指名函数的函数体，放在 64 的倍数之后 `offset` 个字节处（一个 `PCALIGN $64`，它也把函数放在 64 字节处，然后是 `offset` 个字节的 8 字节空操作，每次调用运行一次）。amd64 引擎的默认值是 `lre_exec_backtrack:16`：匹配器放在测得最快的位置（见 [performance.md](../../../docs/performance.zh-CN.md#代码位置)）。解释器也就是寄存器循环 `JS_RegvmRun` 没有固定位置，它的位置还没有测。arm64 和 `-src` 构建没有，除非给了这个参数。链接后的检查让每个函数体在它的位置上，它前面的填充都是空操作。

有两种反向构建用来测试这些测试。`-poison` 在每个入口加载参数之前、每个统一调用之后读结果之前、以及每次返回时，往每个寄存器里写垃圾：引擎的测试必须仍然通过，所以没有代码读取 lowering 没有设置过的寄存器。毒化构建从 Go 栈上读回每个结果，因为它的返回会覆盖结果寄存器。`GEN_KEEP_NARROW=1` 让窄整数保持生成器曾经处理它们的样子：链接后的检查必须拒绝结果（`TestNarrowArgs`）。只有值 1 才会打开它，除了空字符串之外的任何其他值都是错误。这样一次构建的包有标记：`engine_negative_<arch>.go` 不带 `-tags goasmnegative` 就不能编译，`engine_decl_<arch>.go` 也这样说。之后往同一个目录里的构建会删除这个文件。

```sh
go build -o /tmp/engine-gen ./internal/goasm/gen/cmd/engine
L=/usr/lib/llvm-22/bin P=internal/goasm/gen/pass
$L/clang++ -shared -fPIC -O1 $($L/llvm-config --cxxflags) $P/shadow.cpp $P/poll.cpp $P/icall.cpp $P/facts.cpp \
    -o internal/goasm/gen/internal/_shadow.so
/tmp/engine-gen -engine engine -pass internal/goasm/gen/internal/_shadow.so \
    -keep-going -report /tmp/rej.json -out /tmp/cengine/cengine
```

在 pass 之前，C 还会不经优化地再编译一次，每个到达 `engine/libc` 的格式化函数（`snprintf`、`vsnprintf`，以及除了 `code_match` 和 `unicode_prop_ops` 之外的任何可变参数函数）的调用，都要符合 `stdio.c` 接受的格式（`formats.go`、`CheckFormats`）。格式必须是 `FormatError` 接受的字符串常量，或者是被包装函数转发的参数（可变参数函数的最后一个固定参数，或 `JS_ThrowError2`）。别的任何情况都会让构建停下，所以 `%f` 不可能在运行时到达 `abort()`。接受表照着 `stdio.c` 写，`testdata/formats/cases.txt` 列出两者必须一致的格式。`engine/test/libc-test.c` 把这份列表喂给 `vsnprintf`，这样 `stdio.c` 和表就不会各走各的。

链接后的检查是输出的关卡。用 `-tags goasmkeep` 构建这个包的一个二进制（这样 `engine_keep_<arch>.go` 会保留每个函数），把它作为 `-verify-bin` 传入（用逗号隔开的几个会逐个检查），并把包的 import 路径作为 `-verify-pkg`。检查读 llc 自己的对象文件和这个二进制，用 x/arch 对两者解码，并逐条指令地对上每个生成的函数。它拒绝按解码结果运行不了的代码：处理器会拒绝或忽略的前缀，未知的或被截断的指令，汇编器的填充之外的空操作。它允许的差别只有 lowering 为该函数记下的那些改写，并且它根据 llc 的对象推算每一处必须是什么：被替换的序言和尾声（llc 的 push 和保留的局部变量），每个栈帧偏移移到哪里并且访问仍在 Go 栈帧之内，每个调用的保存、参数移动、目标和结果，以及每个数据引用指向哪里。这些移动里的寄存器是 llc 的，不是生成器对约定的模型：每个入口把每个参数字加载进 llc 的探针读取它的那个寄存器，而 llc 的代码在入口处写入之前，除了被丢掉的序言所 push 的那些，不得读任何别的寄存器。每个调用从 llc 传递它的那个寄存器（对统一定义的调用，是定义读取它的那个）存储每个参数字，并把每个结果字加载进 llc 取它的那个寄存器，而 llc 的代码在调用之后、写入之前，除了这些结果和保存的寄存器，不得读任何寄存器。记录说它从寄存器取结果的调用（`RegResult`）没有结果加载：它的目标必须是 lowering 后的统一函数，而不是桩、叶子函数或 Go 代码，llc 取结果的寄存器必须是被调用者的探针返回结果所用的寄存器。每次返回存储 llc 返回的寄存器，并跳到 Go 的尾声，尾声只碰 BP 和 SP，所以这些寄存器在 RET 之后仍持有结果。叶子函数保留 C 的寄存器，所以它的 facts 里只检查它的调用的目标，以及它的调用和返回的数量。llc 自己的代码必须遵守构建排除的东西：经过内存的调用会失败，宽于 8 字节且不是未对齐移动的栈帧访问也会失败。每个函数的记录必须与 pass 对它的签名、它的叶子函数列表和它的调用约定一致。每个间接调用必须带着 LLVM 在 llc 的对象里放在那里的符号，属于那个函数，它所指名的签名必须是 lowering 所编排的那个，别处的符号会失败。它逐字节比较 engine_rodata 和每个函数携带的数据，指针则按它们指向的东西比较。它还让每个池和表在它于二进制里所处的地址上保持对齐。`-verify-all` 也会在有函数不在二进制里时失败。`GEN_EDIT_LOWERED` 在 lowering 后的文本汇编之前编辑它，供检查自己的测试使用（`TestPostLinkCheck`，从 C 经过驱动程序，以及 `TestCheckCases`，来自手写的 llc 代码）。

检查所信任的东西，以及它在哪里止步：

- pass 的签名，只就参数字和结果字的个数而言：它为每个函数报告的那个，以及它根据调用自己的 IR 类型给每个间接调用的类型 id。LLVM 把 id 留在正确的调用上，这一点靠 LLVM（指令选择把它复制到机器调用上，尾合并把不同 id 的调用分开），检查从 llc 的对象里读它，不从 lowering 读的文本里读。
- llc 的 facts：探针得到的是它所代表的函数的寄存器（llc 根据类型、约定和属性分配它们，探针把这些都复制了），以及 LLVM 的 LivePhysRegs 在最后一个机器 pass 之后算出的活跃性，就是代码所读的。一个调用的 facts 按顺序与 llc 对象里的调用相匹配，每个都指名它的目标或类型 id，检查会比较它们。
- 窄整数的宽度：寄存器不说明被调用者依赖哪些位。pass 的加宽让每一个都成为整个 i32，`TestNarrowArgs` 在每个窄值之上带着垃圾运行它。
- 4096 字节或更大的 Go 栈帧有一个检查不认识的栈检查，所以这样的函数通不过。更小的栈帧，检查要求比较和栈帧对得上：amd64 上不超过 128 字节（栈帧加上保存 BP 的 8 字节）时没有 LEA，超过时是 `LEAQ -(栈帧-128)(SP), R12`，arm64 上是比较的那几个字。两种架构都读 `host_poll` 的栈检查。影子栈 pass 像拒绝固定的 alloca 一样，拒绝对齐超过 16 的动态 alloca。
- 加载器。检查按二进制所属系统的格式读它：linux 的 ELF，是链接在固定地址的静态可执行文件，内核映射之后没有谁再往里写（ELF 的 PIE，即 `-buildmode=pie`，以及 cgo 做出的那种带动态表、程序解释器或重定位节的文件，都会被拒绝），darwin 的 Mach-O 和 windows 的 PE，Go 默认把后两者链接成 PIE。它读的每个字节都经由加载器从文件映射的内容（ELF 的 PT_LOAD 程序头，Mach-O 的段，PE 的头和节，各自只到它从文件取的那部分为止），并拒绝在同一页里重叠的映射、不在其映射从文件取它的位置上的节，以及文件偏移不是 0x200 倍数的 PE 节（加载器会把它向下取整）。Mach-O 和 PE 不给符号大小，所以每个符号算到下一个符号为止，一个函数和下一个函数之间的东西必须是填充。在 PIE 里，engine_rodata 的每个指针必须在文件里是它的目标按文件的映像基址算出的地址，并且有告诉加载器移动它的那条记录：dyld info 里的一个 rebase（Go 的链接器），chained fixups 里的一项（ld64，它链接带 cgo 的 darwin 程序），或者一个 DIR64 基址重定位（PE）。加载器在检查读过的代码或数据里写任何别的地方都会让检查失败：别的记录，PE 导入地址表的一个槽，或者它的 load config 指名的一个槽。同一个地方有两条记录也失败，检查不读的表也失败（lazy 或 weak bind，arm64e 的指针格式，PE 的其他重定位类型，PE 的 TLS、延迟导入、绑定导入和 CLR 目录）。PE 的目录只要有地址就读，不管它的大小，和加载器一样，有大小却没有地址的目录会被拒绝。每条记录连同它的全部宽度，都必须落在文件里链接器放指针的那些节的字节里（Mach-O 的 `__DATA` 和 `__DATA_CONST`，PE 的 `.data` 和 `.rdata`），两条记录也不能重叠，所以头、加载命令、`__LINKEDIT`、导入表和重定位表以及代码上都不会有记录。检查为了找出记录而读的 PE 字节逐次记下，位于 `.rdata` 的 load config 整个记下，这些字节上的记录也会被拒绝，因为加载器在读它们之前就先应用了这条记录。截断的或表被改坏的二进制会被拒绝并报错（`TestOpenBinBent`）。加载器按格式应用每条记录，这一点是信任的。在 amd64 上，Go 的序言按系统的方式加载 g（linux 从 FS，darwin 从 GS，windows 经过 `runtime.tls_g`），每种方式只对它自己的系统才认。`TestPostLinkFormats` 检查五个平台各一个二进制，并拒绝每个 Mach-O 和 PE 二进制里改掉的一个字节、错的一个指针、去掉的一条记录和同一个指针的第二条记录，在每个 Mach-O 二进制里还拒绝加载命令、`__LINKEDIT`、`__DATA,__bss` 和代码上的 rebase，在每个 ELF 二进制里拒绝重定位表、`.data` 末尾的 TLSDESC 或 COPY 重定位以及节表里看不到的动态表，在 PE 二进制里拒绝结束在 `.data` 文件内字节之外一个字节的重定位（结束在最后一个字节的照样接受），在 PE 二进制里还拒绝落在 engine_rodata 里的导入表或 load config 的槽（目录大小改成 0 时也一样），有大小却没有地址的目录，导入描述符、查找表、load config（包括两个大小都是 0 时它的 Size 字段）、头、基址重定位表、代码上的重定位或落在导入槽中间的重定位，以及 TLS 目录和延迟导入目录。在适用的二进制里，它还拒绝在文件里结束在 engine_rodata 之前的段或节、从一份副本读、而加载器映射的是改过的字节的节、盖在代码上的第二个 PT_LOAD，以及偏移会被加载器向下取整的 PE 节。

在 arm64 上，检查（`CheckEngineARM64`、`arm64_verify.go`）要窄一些。它让链接后的二进制里每个生成的函数，都符合 llvm-mc 由 lowering 后的文本做出的对象：Go 的序言（恰好在函数不在 NOSPLIT 列表上时才有栈检查，它逐字读其中对保护值的加载、比较和跳到 morestack 的分支，要求它们和 Go 为 TEXT 要求的栈帧写出的一样：栈帧不超过 128 字节时 SP 和保护值比，不超过 4096 字节时把 SP 减去（栈帧 - 128）放在 x17 里比，超过 4096 的栈帧被拒绝，还有栈帧本身），参数的加载，函数体之前的空操作，函数体逐字对上，其中每个 `BL` 和 `ADRP`/`ADD` 指向对象所指名的目标，结果的存储，Go 的尾声，一个 RET 和对 morestack 的调用。函数体里的每个调用是记录里的下一个，它从 Go 栈上读回它的结果，除非记录说它从 x0、x1 或 d0 取结果，这只有对 lowering 后的统一函数的调用才可以：检查要求其中每一个都从它们存储结果，然后只运行 Go 的尾声（x29、x30 和 SP）。直接调用必须在它的目标是不是叶子函数上与 pass 一致，不是时则在它的签名上与 pass 一致，所以没有哪条记录能通过把一个统一函数说成叶子函数、或者说成什么也不返回，来跳过一次加载。它逐字节比较 `engine_rodata`，指针按它们指向的东西比较。它不读 llc 自己的对象：lowering 保住了 llc 代码的含义，这一点靠 lowering 的规则和 llc 的寄存器 facts，lowering 边做边检查它们。`TestCheckARM64Function` 改动一个函数的每个部分，并要求检查拒绝它。
