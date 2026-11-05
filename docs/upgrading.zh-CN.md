---
summary: '怎么吸收上游的改动：远端，把上游 QuickJS 的提交一个一个搬进 engine/（连同它的测试和生成的文件），test262 和它钉住的版本，换用更新的 Go，给 intl 换新的 CLDR、tzdb 或 Chromium 数据，internal/gointl 里 go-intl 的新版本，以及给它的数据包换新的 tz 更新'
read_when:
  - 配置一个要跟进上游 QuickJS 的新克隆
  - 把上游 QuickJS 的某个提交或某个版本搬进 engine/
  - 把 intl 换到新的 CLDR、tzdb 或 Chromium 版本
  - 把 internal/gointl 换到新的 go-intl 版本，或者改 qjs.patch、qjs-sync.sh 或 qjsdata/ 里的数据包
  - 把 Temporal 的时区换到新的 tz 版本（internal/gointl/qjstz）
  - 用比 1.27.2 更新的 Go 构建或测试，或者把提交进仓库的 C 引擎汇编移到更新的 Go 上
---

# 从上游升级

[English](upgrading.md) · 简体中文

## 分支和远端

`main` 是 liustack/quickjs 的分支。

在新克隆里设置一次远端。它是 Fabrice Bellard 的仓库，C 代码就从那里来：

```sh
git remote add quickjs https://github.com/bellard/quickjs.git
git fetch quickjs
```

升级在一个分支上做（`git switch -c upgrade-quickjs-<date>`），并按 [commit.md](commit.zh-CN.md#分支和合并) 所说合并进 `main`。

## 把上游 QuickJS 搬进 engine/

`engine/` 是 `3d5e064`（2026-06-04）的 QuickJS 加上 liustack/quickjs 的改动，并且它自己在变。它不跟着上游一个版本一个版本地走。上游的提交，凡是修正确性、内存泄漏或崩溃的，不管 tifa 的页面碰不碰得到，都会搬进来，让 tifa 渲染更快的也一样。新功能或只为速度的改动，tifa 用得上才搬。每一个都是单独的提交。[changes/upstream-commits.md](changes/upstream-commits.zh-CN.md) 列出了搬进来的，试过又撤回的，以及没搬的。

对每个提交：

1. `git fetch quickjs`，然后看有什么新的：`git log --oneline 3d5e064..quickjs/master -- quickjs.c quickjs.h libregexp.c libunicode.c cutils.c dtoa.c`。
2. 把它的 diff 应用到 C 树上：`git diff <commit>^ <commit> -- quickjs.c | git apply --directory=engine`。碰到 liustack/quickjs 的改动时（[changes](changes/README.zh-CN.md) 里写了涉及的函数），那个改动保留，上游的文字围着它合并。丢掉修改 `engine/` 里没有的东西的那些块（shell 库、`fuzz/`、Makefile）。
3. 写一个没有这个提交就失败、有了就通过的测试，放在 `engine/test/` 里（`engine/test/compare/` 里的一个脚本加它的输出，或者 `unit.c`、`unit-oom.c` 或 `unit-white.c` 里的一个检查），如 [testing.md](testing.zh-CN.md#c-树和它的原生测试宿主) 所述。
4. 运行 `make check-fast`，为两种架构重新生成引擎（`make gen`、`make gen ARCH=arm64`，见 [testing.md](testing.zh-CN.md#生成并检查)），并把 C、测试和生成的文件一起提交，提交信息里写明上游的提交。
5. 一批改动做完时，运行 `make check-full`。test262 的已知错误（33 个）必须逐行保持不变，除非这个提交修好了其中一些。那就更新 `engine/test/test262_errors.txt` 并说明。
6. 按 [performance.md](performance.zh-CN.md#数字是怎么测的) 所说测量它，在 [changes/upstream-commits.md](changes/upstream-commits.zh-CN.md) 里加一行，用户能看到时在两份更新记录里各加一行。

要把 test262 换到更新的修订，改 `engine/test/test262.sh` 开头的提交和 SHA-256，检查 `engine/test/test262.diff`（上游 QuickJS 给 test262 打的补丁）还能打上，运行 `engine/test/test262.sh`，读过每个新错误之后，用它报告的错误替换 `engine/test/test262_errors.txt`。

## 换用更新的 Go

C 引擎的 Go 汇编（`internal/cengine`，提交进仓库的）是用 Go 1.27.2 生成的，在 linux/amd64 和 darwin/arm64 上用 Go 1.26 和 1.27 核对过。`go.mod` 要求 1.26.6 或更新，更新的 Go 会让构建停下，报 `internal/goversion` 的错误（它的文件 `newer.go` 里的构建行是 `go1.28 && !quickjs_anygo`），因为这份汇编从没对着它核对过。`-tags quickjs_anygo` 仍然能构建。生成器的检查和它的测试依赖 Go 1.27.2 的这些东西：

- 每个非 NOSPLIT 函数开头的栈检查：把栈指针和 goroutine 里的保护值 `g.stackguard0` 比较（在 `g` 的偏移 16 处，`g` 在 amd64 上放在 R14，在 arm64 上放在 X28），一个分支跳到对 `runtime.morestack_noctxt.abi0` 的调用，再跳回开头（`internal/goasm/gen/verify_insn.go`、`arm64_verify.go`）。
- 哪些函数是 NOSPLIT：生成器一个也不选，只标记寄存器入口的叶子，以及 Go 汇编器自己标为 NOSPLIT 的叶子，也就是什么也不调用、带保存的基址指针的栈帧小于 128 字节的函数（`internal/goasm/gen/nosplit.go`）。在引擎里它们都不调用任何东西，所以其中一个里的错误，拥有调用者的栈检查留下的空间，至少 800 字节减去它自己的栈帧，并且需要其中 16 字节（amd64）或 24 字节（arm64），用于 `runtime.sigpanic` 的调用和它对 morestack 的调用。如果汇编器自己标了别的叶子，或者不再标这些，链接后的检查就会失败。
- 抢占锤（`internal/cengine/hammer_test.go`）读 `g` 并写它的栈保护值。

Go 改了其中任何一项，生成器的某个检查或某个测试就会失败。程序在构建时不做这个检查，所以更新的 Go 会让构建停下。因此，在换用更新的 Go 之前，用它运行下面这些，每条命令的标签里都加上 `-tags quickjs_anygo`：

1. 运行 `go test -p 1 -tags testhooks,quickjs_anygo ./internal/cengine .`，以及抢占锤（`CENGINE_HAMMER_N=3000 go test -tags cenginehammer,quickjs_anygo -run Hammer ./internal/cengine`）。故障测试（`TestFault*`、`TestFaultNoSplit`）是能发现信号路径变化的那些。
2. 用新的 Go 运行生成器的测试（`CHECK_TAGS=quickjs_anygo make check-fast`，它的 `goasm` 步骤。光用 `GOFLAGS=-tags=...` 不行：自己带 `-tags` 的命令会替换它，门会把它挡住）。它们构建样本程序，把它们链接起来，检查栈检查、对 `runtime.morestack_noctxt.abi0` 的调用，以及链接后的二进制里哪些函数是 NOSPLIT，所以 shape（引擎记录一个对象有哪些键、按什么顺序的结构） 变了会在那里失败。然后用 [testing.md](testing.zh-CN.md#生成引擎的-go-宿主) 的链接后检查（`-verify-bin`，对一个用 `-tags goasmkeep,quickjs_anygo` 构建的测试二进制）以同样的方式检查引擎本身。`make check-gen` 不测 Go 版本：它只检查提交进仓库的文件就是生成器生成的。
3. 如果故障测试失败，就读新版本里 `runtime.sigpanic` 和栈检查的源码，找出原因。如果汇编器自己把别的函数标成了 NOSPLIT，就把 `internal/goasm/gen/nosplit.go` 里的 `autoNoSplit` 改成你在那里读到的样子，并重新生成引擎。
4. 全部通过后，把 `internal/goversion/newer.go` 的构建行移到下一个版本，把里面的名字改成你核对过的版本，并把新版本写在这里和 README 里，中英文两份都写。`TestNewerGoStops` 检查这一行和名字是一致的。

有三个构建选项不支持，构建会在 `internal/cengine/goasm_tripwire.<arch>.s` 的某一行报错停下：amd64 上的 `-asmflags=-spectre=ret` 和 `-spectre=all`，以及 `-dynlink`（go 命令在 `-buildmode=plugin`、`-buildmode=shared` 和 `-linkshared` 时会传它）。生成的引擎把每个函数体写成字节，分支偏移已定，只把调用和取符号地址的加载留给汇编器。这些选项会让汇编器把它们编码成别的长度（经过 retpoline 的 5 字节调用，而不是 `CALL AX`，对符号做 `LEAQ` 变成两条指令），于是分支落错地方，构建虽然成功，第一个 `NewVM` 就崩溃。绊线是一个从不链接的函数，只有汇编器原样保留指令时才能汇编通过。换到更新的 Go 时，再用这些选项构建一遍，检查绊线仍然拒绝它们（`TestEngineAsmOptions`、`TestEngineBuildmodeShared`），并检查 `-buildmode=pie` 仍然给出能运行的引擎：linux/amd64 的汇编器在 `-shared` 时给每个函数的栈检查多加 2 个字节，这是无害的。如果有新的选项改变了引擎留给汇编器的某条指令的长度，就把它加进绊线。

最低的 Go 是 1.26.6，也就是 `go.mod` 的 `go` 行。在 Go 1.26 上，上面这些东西和 1.27.2 一样：链接器允许的栈，栈检查，出错时压入并运行的东西，ABI 包装函数，以及异步抢占的规则。生成器的测试，两种架构的链接后检查和引擎的测试，用 1.26.0 和 1.26.10 都能通过。Go 的两件事把这一行定在了 1.26.0 之上。1.26.0 和 1.26.1 的编译器在 `internal/hostmem` 的一个测试文件上失败（golang/go#77832，1.26.2 修复）。而在 1.26.6 之前，复制 goroutine 的栈时，Go 注入了一次调用的函数（像它把 `sigpanic` 注入到出错的引擎函数里那样）的栈帧，保留着指向旧栈的帧指针，执行追踪器以及 block 和 mutex 剖析可能会顺着它走（`runtime/stack.go` 里的 `adjustframe`，1.26.6 修复）。`scripts/go-versions.sh` 用最低的 Go 和每个次版本的最新发布运行这些测试。要降低这一行，就用更旧的 Go 运行上面的步骤，并读它的发布说明和上面那些文件从那以后改了什么。

## 更新 intl 语言数据

所有 Intl 构造函数都用下面的 go-intl 数据包。构建和测试不需要网络或浏览器。

数据包生成器读 `intl/testdata/chromium/icu.json.gz`。它记录了 ICU 资源的继承关系，原始的日历和时区字符串，非拉丁数字的符号，以及 Chromium 带的 RBNF 包。生成器把这些选择应用到 go-intl 的模型上，并为每个数据集编码一个共用的池。缺失的 RBNF 数据走 ICU 构造样式失败的路径。它不会用编造的值来代替。

要更新参考浏览器，用一个缓存好的 Chromium 可执行文件和 Playwright 安装。为下面的命令设置 `CHROMIUM` 和 `PLAYWRIGHT`：

```sh
node intl/testdata/chromium/record.cjs icudtl
go run -p 2 -tags gointl_walk ./internal/gointl/internal/qjspack/gen -icudtl /path/to/icudtl.dat
internal/gointl/qjs-sync.sh chromium
internal/gointl/qjs-sync.sh pack
node intl/testdata/chromium/record.cjs golden
node intl/testdata/chromium/record.cjs probes migration
go test -short -p 2 ./intl/... ./temporal/...
```

其他每组 golden 用 `record.cjs golden <set>` 记录，如记录器开头的注释所列。Chromium 156.0.8078.4 使用的 ICU 提交是 `5aa526207171ecadf31597f0980a7b7ad8872f33`。

保留浏览器的原始回答。更新预期值之前，要解释每一个变了的表达式。`TestGoIntlMigration` 里的三个 Winnipeg 用例写的是 tz 2026e 的确切答案，因为参考浏览器的时区数据更旧。

## 更新 go-intl（`internal/gointl`）

`internal/gointl` 是 [go-intl](https://github.com/go-quickjs/go-intl) 的副本，go-intl 是一个纯 Go 的 ECMA-402 和 Temporal 库，版本和提交在 `internal/gointl/UPSTREAM` 里（v0.6.1）。包 `intl` 用它实现每个构造函数、Date 和 Temporal 的格式化以及日期范围。包 `temporal` 用它的 temporal 包做计算，并读取它的时区和日历。它放在 internal 里，是因为它的 API 是 go-intl 的，在版本之间会变，放在模块根目录，是为了让 `intl/` 和 `temporal/` 能 import 它。

名字带 `qjs*` 的文件和目录，以及 `UPSTREAM`，是我们的。`qjs_temporalformat.go` 像 Chromium 156 那样保持 Temporal 的模式不变，为注册表的键提供实际的日期模式，并遵从 Temporal 给出的偏移。更新它之后要重新运行重点的格式化语料。其余都是 go-intl 的，import 路径已改写，只通过 `internal/gointl/qjs.patch` 修改，补丁涵盖数据加载、Chromium 样式的构造和版权声明：`source.go` 提供我们的数据包，而不是 go-intl 18.9 MB 的 `data.pack`，`internal/blob` 告诉打包工具一个语言读了池里的哪些部分，`dictbe.go` 把缺失的断词词典当作没有词典，下面"读取者的数据包"里的数据的加载函数会调用链接它们的数据包的函数，`temporal/solar.go` 和 `temporal/zoneinfo.go` 开头有一段说明，写明其中一部分移植自 calendrical_calculations 并有改动，这是它的 Apache-2.0 许可证对改动过的文件的要求。go-intl 新版本里如果有文件移植自 Apache-2.0 的项目，就通过 `qjs.patch` 加上同样的头。

数据在 `internal/gointl/qjsdata/` 里，由 `internal/gointl/internal/qjspack/gen` 从 go-intl 的 `data/` 目录生成：

- `core.bin`（3.39 MB），除下面这些之外的所有东西。
- `names.bin`（1.9 MB），显示名称。用 `-tags quickjs_intl_nonames` 构建可以把它去掉：这样 `Intl.DisplayNames` 没有名称，`of` 对每个代码都返回回退值，即代码本身，在 `fallback: 'none'` 时返回 `undefined`。
- `cjdict.bin`（2.0 MB），中文和日文的分词词典。用 `-tags quickjs_intl_nocjdict` 构建可以把它去掉：这样一串中文或日文就是一个词，和没有这个词典的 ICU 一样。
- `tz.bin`（92 KB），包 `temporal` 读的东西：ICU 的时区，Temporal 用的时区名称，中国农历、韩国历和 Umm al-Qura 历的表，以及 `tzversion.bin`，即这些时区的 tz 版本。`Embedded` 把它和其余的一起提供，`EmbeddedTemporalSource` 单独提供它。两者共用一份：通过任何一个读到的块只解压一次。
- 读取者的数据包，每个只链接进链接了读它的函数的二进制（`qjs_readers.go`）：`dates.bin`（677 KB），go-intl 的 `DateTimeFormat` 读的东西，`zonenames.bin`（1.27 MB），时区名称，以及 `hostzones.bin`（6 KB），猜测宿主时区用的表。包 `intl` 对 Date 和 Temporal 的值与范围使用这些数据包。它链接日期和时区名称这两个包，但不链接宿主时区包（`TestLinksOnlyReadData`）。读它们的数据的每个加载函数，都会通过 `qjs.patch` 先调用 `qjsUseDates`、`qjsUseZoneNames` 或 `qjsUseHostZones`，如果 go-intl 新版本里的某个函数读了它们的某个数据集而没有这个调用，`TestQJSReadersUsePacks` 就会失败：把它加进补丁。新版本为新的读取者添加的数据集，同样得到自己的数据包，做法是在打包工具的 `packOf` 里、`qjs_readers.go` 的一个变量里，以及它的加载函数里的调用里加上。

语言就是 Chromium 156 的那些。`qjsdata/chromium.json` 记录了对每个 `Intl` 构造函数，Chromium 把 go-intl 的每个可用语言解析成哪个语言，而这里每个服务都有 Chromium 解析到它们自己的那些语言：`TestQJSResolveMatchesChromium` 检查每个服务对这些标签的每一个的解析都和 Chromium 一样。一个服务在 Chromium 有裸语言时，这个语言的数据就保留，连同它继承的语言，`PluralRules` 在 Chromium 有它时，也保留它的复数规则。Chromium 有一些语言，它没有对应裸语言的数据，比如没有 `ha` 的 `ha-GH`：它们在那里和在这里，都用根的数据格式化。打包工具会打印它丢掉的语言，以及只保留了复数规则的语言。

`qjsnumbers.bin` 标记了数据包生成器已经解析过的数字资源。go-intl 的原始数据没有这个标记，保留它原来的 ICU 别名查找。未知的标记版本在读取时失败。

共用的字符串池，排序的根和断词词典是不压缩保存的，因为格式化器只读其中很少的一点，二进制里它没碰的页就留在磁盘上。其余的是分块的 DEFLATE，一个块在格式化器第一次读到其中的文件时解压到堆里，每个进程一次。`QJSInflated` 报告这有多少。

格式化器读到的字节从不能写：不压缩的块是二进制的只读数据，写了会出错，解压后的块被进程里的每个格式化器和 VM 共用。把这样的字节交给 JS 的代码，比如作为 `ArrayBuffer` 之类，要先复制它们。

`make gointl` 运行 `go test ./...` 不跑的检查：打包工具的测试（需要 `gointl_walk` 构建标签），不带可选数据包的构建，以及 `qjsdata/` 就是 go-intl 的数据加上下面的时区所得的结果。

### 时区

go-intl v0.6.1 用 tz 2026c 生成它的时区。数据包带的是更晚的一次更新，也就是 `internal/gointl/qjstz` 所指的那个（现在是 2026e），这样一个此后改了规则的时区，给出的是用更新数据的浏览器给出的结果。`qjs-sync.sh checkout` 从 icu-data 仓库下载 ICU 对应那个版本的更新（`tzdata/icunew/<version>/44`：`zoneinfo64.txt`、`timezoneTypes.txt`、`windowsZones.txt`、`metaZones.txt`）和 ICU 的源码，按 `qjstz` 核对它们的 sha256，并在检出的一份副本 `$GOINTL_UPSTREAM-tz<version>` 里，对它们运行 go-intl 自己的 `tzgen`。只有 `data/tz/`、`data/windowszones.bin` 和 `data/abbreviationzones.bin` 会变，副本里还会得到 `data/tzversion.bin`，它会进入 `tz.bin`。`pack` 读那份副本的 `data/`。`test` 用 go-intl 的数据原样制作它的数据包，时区也包括在内，因为 go-intl 的语料保存的是 Node 在 go-intl 的 tz 版本下的答案。`test-chromium` 对着随发布的数据包和记录下来的浏览器答案运行分叉的测试。在 2026c 和 2026e 之间，这些测试在 Bogota（1992）、Tehran（1979）、Dublin（1925）、Inuvik、Winnipeg 和 Rainy_River，以及 EST5EDT、CST6CDT、MST7MDT 和 PST8PDT（2026d 把它们做成了独立的时区）上不同：都是 tz NEWS 文件列出的改动。时区名称和 metazone 用 go-intl 的模型加上 Chromium 对资源的选择。`TestQJSTZVersion` 检查 `tz.bin` 里放的是哪个版本。

要换到更新的版本，等 ICU 发布之后：

1. 在 `qjstz` 里设置 `TZ_VERSION` 和四个校验和，校验和取自 `tzdata/icunew/<version>/44` 下四个文件的 `sha256sum`。
2. 运行 `qjs-sync.sh checkout`，然后运行 `qjs-sync.sh pack`。
3. 修改 `TestQJSTZVersion` 里、[temporal.md](changes/temporal.zh-CN.md#时区数据) 里和 `temporal` 的包文档里的版本，并在 `temporal/all_test.go` 的 `TestZoneData` 里为更新改过的一个时区加一个用例。
4. 运行 `qjs-sync.sh test -p 2`、`qjs-sync.sh test-chromium -p 2`、`go test ./internal/gointl/... ./temporal` 和 `make gointl`。

当 go-intl 自己换到那次更新或更晚的更新时，`checkout` 写出的仍是 `qjstz` 的时区。那时要把 `qjstz` 设成 go-intl 的版本或更晚的，绝不能更早。

要换到新的 go-intl 版本，在 `internal/gointl/UPSTREAM` 里设置 `VERSION` 和 `COMMIT`，然后：

```sh
internal/gointl/qjs-sync.sh checkout       # clones go-intl into $GOINTL_UPSTREAM (default ~/.cache/quickjs-gointl/go-intl)
internal/gointl/qjs-sync.sh update         # copies its runtime sources here and applies qjs.patch
internal/gointl/qjs-sync.sh chromium       # records what Chromium resolves the new tags to
internal/gointl/qjs-sync.sh pack           # writes qjsdata/ again
internal/gointl/qjs-sync.sh test -p 2      # go-intl's own tests, on packs of every language
internal/gointl/qjs-sync.sh test-chromium -p 2  # the Chromium corpora of liustack/quickjs on the shipped packs
go test ./internal/gointl/... && make gointl
```

`qjsdata/NOTICE-ICU` 放着数据包里的 ICU 数据和断词词典的 ICU 版权声明：ICU 的 `LICENSE` 在标签 `release-78.3` 处的第 1 到 401 行，前面加我们的一小段说明。`update` 会复制 go-intl 的 `LICENSE`，但不复制这个文件。当 go-intl 的数据取自新的 ICU 版本时（它的 `SOURCES.md` 里写明了），要用 `gh api 'repos/unicode-org/icu/contents/LICENSE?ref=release-<version>'`，从那个版本标签处 ICU 的 `LICENSE` 重新写这个文件：同样的几节，内容不变，说明里的版本和行号更新，并为数据包新取的、以前没有的词典或数据加上它的声明。其他 `qjsdata/NOTICE-*` 文件放着 go-intl 从 V8（`NOTICE-V8`）和 Temporal 的 crate（boa-dev 的是 `NOTICE-TEMPORAL`，ICU4X 的是 `NOTICE-ICU4X`）移植的东西的声明，版本是它的 `SOURCES.md` 里写的。go-intl 的某个版本改了其中一个版本，或者从没列在那里的项目移植东西时，就复制那个项目在新版本处的许可证，以及它的 `NOTICE` 文件（如果有的话）（crate 的取自 `https://static.crates.io/crates/<name>/<name>-<version>.crate`，V8 的取自 Node 那个版本的 `deps/v8/LICENSE`），更新开头的说明和 [NOTICE.md](../NOTICE.md#code-and-data-that-go-intl-took-from-other-projects) 里的表。

`chromium` 需要 Node 和 Playwright 驱动的 Chromium，设置方法同[更新 intl 语言数据](#更新-intl-语言数据)里 intl 的 golden 测试（`PLAYWRIGHT`，另一个浏览器用 `CHROMIUM`）。有更新的 Chromium 时也要再运行它。`qjs.patch` 打不上的地方，手工改，然后运行 `qjs-sync.sh patch` 重新写出补丁。当 `pack` 失败，报某个池里有任何语言都够不到的部分时，说明某个数据包开始读它的 `qjs_walk.go` 没有读的东西：让那个遍历去读它。`test` 不含 `pack_test.go` 和 `TestGeneratedTablesAreWellFormed`，它们读的是 go-intl 的 `data.pack` 和 `data/`。`test-chromium` 运行提交进仓库的 go-intl、Intl 和 Temporal 测试。Chromium 除了去掉语言，还会去掉单个资源，所以从 Node 的语料里过滤掉一些语言，并不能把它们变成浏览器的预期值。在 `test` 里保留完整的 Node 检查，对随发布的数据包用记录下来的 Chromium 语料。`test-chromium` 不需要上游的检出，也不需要安装浏览器。
