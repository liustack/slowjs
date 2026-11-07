# 更新记录

记录 `github.com/liustack/quickjs`（v0.27.3 及之前和 v0.30.0 起都叫这个名字，v0.28.0 到 v0.29.11 叫 `github.com/liustack/slowjs`）里用它的程序看得到的变化。每个版本的 tag 打在 `main` 上：一般是补丁版本，删掉或改动公开 API 时升次版本号。格式参照 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)。每项改动的细节见 [Fork changes](docs/changes/README.md)，每项提速怎么测的见 [Performance](docs/performance.md)。没有特别说明时，时间指在 linux/amd64（AMD EPYC）上服务端渲染 tifa 示例商店一个页面的耗时。

## [Unreleased]

### 修复

- 在 windows/amd64 上，引擎里的内存错误（只有引擎自身的 bug 才会引起）在每种 CPU 上都直接变成它的 panic，不再先踩坏进程的内存。windows 在出错时把 CPU 的状态写在栈指针下面，带 AMX 的 Intel 上是 12.8 KiB，而 Go 只在 goroutine 的栈下面留 4 KiB（golang/go#81238），所以这次写入可能越过栈写进堆里。现在每个生成函数都确保栈指针下面再多 12 KiB，一共至少 16.9 KiB。只有 windows 的代码变了：在 linux 上用这份代码测量，那里的渲染慢 0 到 2.4%，运行一个很小脚本的 goroutine 占 32 KiB 栈，原来是 8 KiB。Go 代码里的错误，不管是宿主的还是程序的，在 Go 修好之前仍然只有 Go 的 4 KiB（[c-engine.md](docs/changes/c-engine.zh-CN.md#windows-上给故障留的空间)）。
- `EvalBytecode`、`EvalBytecodeValue`、`EvalSharedBytecode` 和 `EvalSharedBytecodeValue` 现在从读取字节码到把结果转换完，都是一次进入 javascript 的调用，在 eval 超时之内。结果的转换原来在调用结束之后才进行，所以字节码最后得到的对象的 `toJSON` 或 getter，或被替换的 `BigInt.prototype.toString`，运行时没有超时，`Interrupt` 也停不下它（[uncatchable.md](docs/changes/uncatchable.zh-CN.md#字节码调用在-eval-超时内转换结果)）。
- 读取脚本抛出的异常时（它的 `toString`，或者它的 `name`、`message`、`stack` 的 getter），如果被中断、eval 超时或内存不足停住，调用现在以这个错误结束，`Uncatchable` 已设置。过去读取把它和脚本抛出的东西一起丢掉，所以调用返回一个字段为空、也不是无法捕获的 `*Error`。Go 函数的参数转换遇到这样的异常时，脚本接住一个 `TypeError`，一直运行到结束。这种情况下 `ErrorFromValue` 返回被中断或内存不足的错误，过去它把被超时截断的字段留空（[uncatchable.md](docs/changes/uncatchable.zh-CN.md#读取异常时遇到的无法捕获的异常结束调用)）。
- 每个接收 `Value` 的方法现在都在进入引擎之前拒绝别的 VM 的 `Value`：`SetPropertyValue`（它的值和接收者），`SetProperty`、`GetPropertyValue` 和 `GetProperty` 的接收者，`EvalThis` 和 `EvalThisValue` 的 `this`，以及 `RegisterFunc` 的 Go 函数返回的 `Value`（单独返回、在多个结果里、在切片或 map 里），后者会抛出 `TypeError`。`SetPropertyValue` 原来会把这样的值存进对象，对象于是指向另一个 VM 的内存，那个 VM 关闭后这块内存就被解除映射。被拒绝的 `Value` 不会被释放。`Call` 和 `Value.Call` 现在也拒绝别的 VM 的数字，和 `NewArray` 以及 `Builtin` 一样（[Go API](docs/changes/go-api.zh-CN.md#每个入口都拒绝别的-vm-的-value)）。
- `NewString`、`NewStringLatin1`、`NewStringUTF16` 和 `NewObjectValue` 在 VM 内存不足时现在返回内存不足的错误。它们原来把引擎的异常标记当作 `Value` 返回，错误为 nil，并把错误留着挂起，等下一个调用发现（[Go API](docs/changes/go-api.zh-CN.md#值的构造函数会报告内存不足)）。
- `RegisterFunc` 或 `RegisterHostFunc` 的 Go 函数的结果转换失败时不再泄漏：失败那一项之前的结果造出的值，以及结果持有的每个本 VM 的 `Value`，都会被释放，不管转换是到一半失败、拒绝了这个结果（数组、指向指针的指针、作为 map 键的 `Value`）、遇到一个会 panic 的 `Error` 方法，还是宿主函数连同值一起返回了错误。channel 前面的一个 4 KiB 字符串过去每次调用后都永远留在 VM 里（[Go API](docs/changes/go-api.zh-CN.md#转换失败的结果不留下任何东西)）。
- 把 `UndefinedValue` 作为 `Call` 和其他调用方法的参数，或者作为 `SetPropertyValue` 的值传入时，得到的是 `undefined`，原来会因空指针 panic。`RegisterFunc` 的 Go 函数返回的零值 `Value` 也是 `undefined`，原来是 0，作为 `Value.Call` 的 `this` 时也是，和 `NewArray` 以及 `Builtin` 一样（[Go API](docs/changes/go-api.zh-CN.md#undefinedvalue-作为参数和属性值)）。
- 在对 VM 的调用进行中（比如在这个 VM 上的 javascript 调用的 Go 函数里）对它调用 `Close`，现在返回错误，什么也不改，VM 继续可用。原来它会释放还在 Go 函数下面运行的引擎，调用随后以空指针 panic（[Go API](docs/changes/go-api.zh-CN.md#拒绝在对-vm-的调用进行中-close)）。
- `Value.Then` 返回 `then` 的 getter 抛出的东西，原来返回 `TypeError: not a function`（[Go API](docs/changes/go-api.zh-CN.md#then-返回-then-getter-抛出的异常)）。
- Go 函数返回的 Go map 变成的对象，键都是自有数据属性，每个键完整保留。原来键在第一个 NUL 字节处被截断，`"a"` 和 `"a\x00b"` 成了同一个键，键 `__proto__` 会改掉对象的原型，`Object.prototype` 上以某个键的名字放的 setter 会拿到它的值运行（[Go API](docs/changes/go-api.zh-CN.md#go-map-变成只有自有数据属性的对象)）。
- Go 函数作为 `any` 返回的 `Value`（来自 `RegisterFunc` 或 `RegisterHostFunc`）现在按 `Value` 接管：本 VM 的把它的引用交给调用，别的 VM 的抛出 `TypeError`，`UndefinedValue` 和零值 `Value` 是 `undefined`。原来它走 JSON，遇到 `UndefinedValue` 会 panic，本 VM 的每个 `Value` 的引用都永远留下。宿主函数连同值一起返回错误时，现在会释放值里的 `Value`（[Go API](docs/changes/go-api.zh-CN.md#作为-any-返回的-value-按-value-接管)）。
- Go 函数的结果经由同一个位置两次到达的 `Value`，比如以 `[]any{s, s}` 返回的同一个 `Value` 切片、返回两次的同一个 map（不管用它类型的哪个名字），或者同一个数组上互相重叠的两个切片，只算一个引用，只释放一次。原来转换每遇到一次就再接管一次引用，所以对象在脚本还在用的时候就被释放了。一个 `Value` 和它的 `Dup` 是两个引用，`Value` 的副本不是（[Go API](docs/changes/go-api.zh-CN.md#结果两次到达的同一个位置只算一个引用)）。
- 包含自身的 Go 函数结果，比如 `s[0] = s` 的切片 `s`、装着自身的 map 或指向自身的指针，会抛出 `TypeError`。原来转换一直递归到 Go 的栈上限，`fatal error: stack overflow`，进程直接退出（[Go API](docs/changes/go-api.zh-CN.md#包含自身的结果是一个错误)）。
- Go 函数的结果按函数返回那一刻的样子转换。转换会运行程序的代码，`Error` 方法、`MarshalJSON` 或结构体里 `Value` 的 `toJSON`，它们可以在转换进行中改动结果：变得包含自身的切片让进程因栈溢出退出，放进去的别的 VM 的 `Value` 在不拥有它的 VM 里被改动计数，被拿走的 `Value` 则永远留在 VM 里。类型带 `Error` 方法的值和以前一样转换成那个字符串，里面的 `Value` 和结构体里的一样仍归调用方所有（[Go API](docs/changes/go-api.zh-CN.md#结果从程序改不动的快照转换)）。

## [v0.30.1] - 2026-11-06

### 修复

- 用 `+` 拼出来的长字符串（rope）在被拼成一整块时，`Interrupt` 和 eval 超时现在也能停下这次调用。以前拷贝它的每一段时都不读中断标志，几千万字符的 `+` 拼接字符串第一次被内置函数、属性键或标签函数用到时，要整个拷完才能停：超时设 10 ms 时，6400 万字符的样式值在 CI 上要到调用开始后 68 ms 才停下。现在拷贝时每 65536 个字符读一次标志，这样的调用会马上停下。拼出来的字符串逐字节不变（[uncatchable.md](docs/changes/uncatchable.md#a-long-rope-is-made-flat-in-steps)）。
- `String.prototype.repeat` 现在每 65536 个字符读一次中断标志。以前每 1048576 个字符才读一次，而且只在两次整段拷贝之间读，不到 1048576 个字符的调用根本不读。`'x'.repeat(1 << 20)` 要跑 105 次引擎轮询（每次 10000 个 tick）都不读标志，`'x'.repeat(1e5)` 的循环要到下一次调用才停。结果不变（[uncatchable.md](docs/changes/uncatchable.md#repeat-reads-the-interrupt-flag-every-65536-characters)）。
- `Interrupt` 和 eval 超时现在会用中断结束一个长正则的编译，以前可能直接杀掉 VM。编译里有四段不读中断标志：把 pattern 转成 UTF-8、代码变长时的拷贝、扫描代码数寄存器、把代码拷进字符串。1320 万字符的 pattern 光扫寄存器就要跑约 1360 次引擎轮询（每次 10000 个 tick），超过 Go 宿主杀掉脚本的 1000 次，于是调用以 `ErrKilled` 结束，VM 只能关掉。计数、`\u{…}` 和组名里很长的数字，`&&` 和 `--` 的操作数，`\q{…}` 的字符串，其中一个字符串和它之前同哈希字符串的比较，以及在正则缓存里查一个长 pattern，也都不读标志。现在这些地方都分步读标志，这样的编译几次轮询内就停下。正则编译出的代码逐字节不变（[uncatchable.md](docs/changes/uncatchable.md#compiling-one-long-regexp-reads-the-interrupt-flag-in-every-long-loop)）。
- 含很多字符或很多字符串的正则字符类，现在在 `Interrupt` 或 eval 超时后几次轮询内就停下，编译也快得多。以前类里每个字符都要和前面的区间整体重建一遍来合并，中间不检查中断，6 万个互不相邻字符的类要跑 557,426 次引擎轮询（每次 10000 个 tick）才编译完。现在按顺序的类每个字符一步，乱序的类只排序一次，同一个类 51 到 137 次轮询就编译完。`\q{…}` 的字符串排序、哈希表变大时的搬移、停下后的释放，都是一趟到底不检查中断（26 万个字符串时最多 146 次轮询），遍历一个空表也不检查。这些现在都分步检查。正则编译出的代码逐字节不变（[uncatchable.md](docs/changes/uncatchable.md#a-class-of-a-regexp-reads-the-interrupt-flag-however-many-characters-or-strings-it-has)）。

## [v0.30.0] - 2026-11-05

### 变更

- **不兼容：** 模块改回 `github.com/liustack/quickjs`（v0.27.3 及之前用的就是这个名字），根包名改回 `quickjs`（v0.28.0 到 v0.29.11 是 `github.com/liustack/slowjs`，包名 `slowjs`）。API 不变。程序要改 import 路径（`github.com/liustack/slowjs/intl` 改成 `github.com/liustack/quickjs/intl`，其余同理），把代码里的 `slowjs.` 改成 `quickjs.`，原来加 `slowjs_intl_` 构建标签的，改加 `-tags quickjs_intl_nonames` 或 `quickjs_intl_nocjdict`，原来加 `slowjs_anygo` 的改加 `quickjs_anygo`。脚本用的环境变量改以 `QUICKJS_` 开头（`QUICKJS_LLVM`、`QUICKJS_REMOTE` 等），`intl` 和 `temporal` 标记 VM 用的 `Symbol.for` 键换成新路径（`Symbol.for('github.com/liustack/quickjs/intl')`），根包的错误信息改以 `quickjs:` 开头。v0.30.0 是新路径下的第一个版本，v0.28.0 到 v0.29.11 留在 `github.com/liustack/slowjs`（[upstream.zh-CN.md](docs/upstream.zh-CN.md#从-githubcomliustackslowjs-换过来)）。

### 修复

- `RegisterStartTag` 和 `RegisterEndTag` 注册的 `startTag`、`endTag` 现在遍历长字符串的每个循环都会读中断标志，标签、样式名、URL 或文本有几百万个字符时，`Interrupt` 和 eval 超时也能在几毫秒内停下这次调用。以前有四个循环从不读它：标签的检查、在样式名里找非 ASCII 字符、URL 前面的空白、拼好的开始标签的复制。超时设 10 ms 时，`TestTagsStop` 在 CI 上要到调用开始后 53 到 119 ms 才停下。在 linux/amd64 上，3200 万字符的标签最长停顿从 19.0 ms 降到 5.9 ms，前面有 3200 万个空白的 URL 从 10.5 ms 降到 3.3 ms。渲染 tifa 的页面多跑 0.002% 到 0.018% 的指令，时间的变化小于测量能分辨的 1%。输出逐字节不变（[uncatchable.md](docs/changes/uncatchable.md#the-start-and-end-tags-read-the-interrupt-flag-in-every-long-loop)）。

## [v0.29.11] - 2026-11-05

### 变更

- 这是 `github.com/liustack/slowjs` 这个路径下的最后一个版本，`go.mod` 把模块标成了已弃用。从 v0.30.0 起模块重新叫 `github.com/liustack/quickjs`（v0.27.3 及之前就叫这个名字），API 不变：改导入路径，把包名 `slowjs` 改成 `quickjs`。
- 模块现在用 Go 1.26.6 或更新就能构建，以前要 Go 1.27.0。引擎已经照 Go 1.26 核对过。1.26.6 之前的 1.26 版本不在范围内：1.26.0 和 1.26.1 的编译器编不过模块里的一个测试，1.26.6 之前的运行时在引擎出内存错误后可能留下指向旧栈的帧指针（[upgrading.md](docs/upgrading.md#moving-to-a-newer-go)）。
- 用比引擎核对过的版本更新的 Go（目前是 Go 1.28 及以后）构建时，会直接报一个错，错误里写着核对过的版本（`slowjs_is_checked_up_to_go1_27_see_docs_upgrading_md_or_build_with_tags_slowjs_anygo`）。引擎的 Go 汇编依赖 Go 的一些实现细节，Go 1 的兼容承诺不包括这些，其中哪一项变了，要到程序运行时才会暴露。Go 1.26.6 及以后的版本和所有 Go 1.27 版本都能构建。新版 Go 发布后，slowjs 用它核对一遍，在补丁版本里放开。加 `-tags slowjs_anygo` 可以用任何版本的 Go 构建（[Go API](docs/changes/go-api.md#a-go-newer-than-the-one-the-engine-was-checked-with-stops-the-build)）。
- 生成的引擎里，凡是调用了别的函数的函数，开头现在都有 Go 的栈检查。以前生成器另外给大约 450 个函数省掉了栈检查，它们安全的前提是引擎出内存错误时，下面还剩 64 字节空间。这个数是从 Go 1.27.2 的运行时源码里读来的，Go 并不承诺。现在内存错误能用的空间和任何 Go 程序里的空指针一样。tifa 示例应用的页面渲染最多慢 0.6%（指令多 0.27% 到 0.36%），HTML 不变（[c-engine.md](docs/changes/c-engine.md#functions-without-a-stack-check)）。

- 用会让汇编器改变引擎指令编码长度的构建选项构建时，现在会直接报错，以前能构建成功，却在第一次 `NewVM` 时崩溃：amd64 上的 `-asmflags=-spectre=ret` 和 `-spectre=all`，以及 `-dynlink`（`-buildmode=plugin`、`-buildmode=shared` 和 `-linkshared` 会带上它）。错误指向 `internal/cengine/goasm_tripwire.<arch>.s` 里的一行，那一行的注释写明了原因（[Go API](docs/changes/go-api.md#a-build-option-that-changes-how-the-assembler-encodes-the-engine-stops-the-build)）。`-buildmode=pie` 和其他选项照旧能构建。

### 修复

- 异常离开 `for...of` 循环或数组解构时会关闭迭代器，如果迭代器的 `return` 方法这时内存不够，`catch` 不再能接住这个内存不足的错误。调用会带着这个错误结束，和其他地方内存不足一样（[uncatchable.md](docs/changes/uncatchable.md#out-of-memory-in-the-return-method-of-an-iterator-that-an-exception-closes)）。
- 引擎在 0x1000 以下的地址出内存错误（C 里的空指针就会这样）时，VM 现在会标成损坏，之后除了 `Close` 什么调用都拒绝，和其他地址的内存错误一样。Go 把这种错误变成空指针解引用的 panic，里面没有地址，而以前只有带地址的 panic 才会让 VM 标成损坏：调用方 recover 之后，还能接着用一个引擎停在半路的 VM。在 JavaScript 调用的 Go 函数里，这种错误还会被当成 Go 函数自己的 panic，变成不可捕获的错误，引擎接着往下跑。现在只要 panic 或 Goexit 离开了一次还没返回的引擎调用，不管 panic 的是什么，VM 都会标成损坏。JavaScript 调用的 Go 函数在自己的 Go 代码里（不在引擎调用之内）出的内存错误，现在和这个函数的其他 panic 一样处理，VM 照常可用（[C 引擎](docs/changes/c-engine.md#which-panics-break-a-vm)）。
- panic 让一次引擎调用停在半路之后，Go API 不再调用这个引擎，连释放值也不调：方法在退出途中释放值（`Call`、`CallValue`、`SetProperty`、`Then`、值的转换）时不再进引擎，JavaScript 调用的 Go 函数 recover 了这个 panic 再接着调用引擎时也不行。这样的调用会像在损坏的 VM 上一样 panic，释放则什么也不做。以前这些释放会在内存错误、kill、abort 或 trap 留下的状态上执行，可能再出一次内存错误，把第一个 panic 盖掉，或者把状态改得更乱。内存由 `Close` 收回（[C 引擎](docs/changes/c-engine.md#which-panics-break-a-vm)）。

## [v0.29.10] - 2026-11-04

### 新增

- `VM.RegisterStartTag(name)` 和 `VM.RegisterEndTag(name)` 在全局对象上定义两个函数，一个像 React DOM 服务端渲染器（react-dom 19.3）的 `pushStartGenericElement` 那样写出元素的开标签，连同属性、样式和字符串 children 转义后的文本，一个像 `endChunkForTag` 那样给出闭标签，在引擎里用 C 写成。开标签函数接 `pushStartGenericElement` 的参数、React 的 `hasOwnProperty` 和调用方的一个 `decline` 对象。它把整个标签作为一个字符串推进 target，返回 React 那个函数返回的值。React 的函数可能写出别的东西的地方（getter、被换掉的内建函数、`dangerouslySetInnerHTML`、少见的属性、自带 `toString` 的值、`javascript:` 链接，以及文档里列的其他情况），它返回 `decline`，什么都不改，脚本再跑 React 的函数。引擎自己不往全局对象上放任何东西：宿主注册了，VM 里才有这两个函数。在 tifa 示例页面一次渲染的 16,310 个开标签上，它们一个都没退回，写出的文本相同，一对开闭标签跑的指令约是 React 那几个函数的五分之一。tifa 的 React DOM 调用它们之后，商店工作台页面每次渲染的耗时少 7.3%，商店前台页面少 7.0%，MUI 测试应用的页面少 3.8%，bench 页面少 33%，tifa 池里每个 VM 多占约 26 KB 内存。（[性能](docs/performance.md#react-doms-start-and-end-tags-in-c)，[Go API](docs/changes/go-api.md#registerstarttag-and-registerendtag)）

### 性能

- linux/arm64 和 darwin/arm64 上的生成引擎，也像 amd64 一样，是在 C 文件链接之后，再让优化器跑第二遍得到的。tifa 示例应用的页面，每次渲染少跑 0.56% 到 0.99% 的指令（linux/arm64），16 个页面遮掉时间戳之后 HTML 逐字节相同。在 M4 上没有观察到变慢（页面总和是 main 的 0.987 倍，8 轮的中位数，轮与轮之间有 8% 到 16% 的噪声），arm64 的生成汇编大了 12%（[性能](docs/performance.md#on-arm64)）。
- 内建类的对象从原型的空 shape 开始，这个 shape 由上下文保存。对象的下一个 shape 是从当前 shape 长出来的时候，只比较最后一个键。在收集之外引用数归零、又没有别的东西要一起释放的对象，当场释放。tifa 的商店和 MUI 应用的页面，每次渲染少跑 1.84% 到 2.13% 的指令，bench 页面少 0.49%，bench 页面的耗时少 0.73%（[性能](docs/performance.md#shapes-of-built-in-classes-shapes-found-by-their-parent-objects-that-die-alone)）。

### 修复

- C 引擎在释放对象的过程中（宿主类的 finalizer 里，或 `host_release` 里）被调用 `JS_RunGC`、`JS_RunYoungGC` 时，现在什么也不做直接返回。在那里新建对象也不会开始回收，释放结束后新建下一个对象时会重新检查阈值。以前这次回收会把引擎正在释放的对象一起释放掉（正在跑 finalizer 的那个也在内），结果是进程退出，或者读到已经释放的内存。Go 程序碰不到这个问题：Go API 不注册类，它的 `host_release` 也不跑用户代码。上游 QuickJS 和 quickjs-ng 的代码也是这样（[C 引擎](docs/changes/c-engine.md#a-collection-asked-for-while-objects-are-freed)）。

## [v0.29.9] - 2026-11-03

### 新增

- 模块可以在 linux/arm64 上构建和运行，用的生成引擎和 darwin/arm64 是同一份。CI 在 `ubuntu-24.04-arm` 上跑测试。
- 模块可以在 darwin/amd64 上构建和运行，用的生成引擎和 linux/amd64 是同一份。CI 在 `macos-15` 上用 Rosetta 跑测试。
- 模块可以在 windows/amd64 上构建和运行，用的生成引擎和 linux/amd64 是同一份。CI 在 `windows-2025` 上跑测试。在 windows 上，引擎里的内存错误（只有引擎自身的 bug 才会引起）在变成 panic 之前，可能先踩坏进程的内存，带 AMX 的 Intel 这类 CPU 上会发生。这是 Go 运行时的 bug，见 golang/go#81238。

### 修复

- 用别的类的原型建数组时（比如 `F.prototype` 设成 `Map.prototype` 后调 `Reflect.construct(Array, [], F)`），如果加 `length` 时内存不够，现在会抛出内存不够的错误。以前会往空指针里写值，整个进程直接退出。上游 QuickJS 也有这个问题。
- 对 shape 被共享的对象调 `Object.setPrototypeOf`，内存不够时不再让新原型一直留在内存里。上游 QuickJS 也有这个问题。

## [v0.29.8] - 2026-11-01

### 修复

- `Iterator.concat` 的 `return` 按 IteratorClose 规则关闭底层迭代器，返回新的完成结果。推进或关闭失败后，不再继续推进或重复关闭（QuickJS `aac9bc5`）。
- `Iterator.prototype.reduce` 在没有初始累加值的空迭代器上抛错时，不再读取或调用 `return`（QuickJS `de4c9e0`）。
- 稀疏数组截断会先删除所有应删的元素，再压缩属性表。截断后重新写入原有索引会正确更新长度（QuickJS `3c4c6ed`）。
- `JSON.stringify` 处理数组时，如果把元素的下标转成字符串那一步内存不足，不再漏掉这个元素。原来这个元素连同它键的 atom 一直留到 `VM.Close`，释放时会发现还有块在用。上游 QuickJS 也有这个问题，quickjs-ng 在 `293ba08` 修了。
- `JSON.stringify` 的 space 参数是拼接出来的长字符串（rope）时，现在用这个字符串的前 10 个字符缩进。原来缩进用的是从 rope 内部字段读出的字节，里面还有指针。上游 QuickJS 也有这个问题，quickjs-ng 在 `35e195c` 修了。
- `JSON.stringify` 的 replacer 是一个名字数组时，如果复制这些名字时内存不足，现在会以内存不足的错误失败。原来它跳过没复制成的名字接着跑，返回的字符串里少了那个属性，错误还挂着没处理。上游 QuickJS 和 quickjs-ng 也有这个问题。

## [v0.29.7] - 2026-11-01

### 变更

- 一个 `WeakRef` 的目标如果只被一个已经不再用的局部变量拿着，现在可能更早拿到 `undefined`：那个值在同一槽位的下一个局部变量被写入时就释放，不再等到调用结束。

### 性能

- linux/amd64 上生成的引擎，现在是在 C 文件链接成一个模块之后，让优化器再跑一遍得到的，所以文件之间的调用（`memcpy`、`memset`、`memcmp`，分配器和堆里的小函数，垃圾回收里的函数）都内联进了调用者。tifa 示例应用的页面每次渲染少跑 0.6% 到 1.2% 的指令，`r_react_ssr` 一次运行少 3.5%，HTML 不变。页面的耗时仍在代码位置的波动范围内（React 脚本和 bench 页面约为 0.98），汇编体积大 13%，解释器的栈帧大小不变。darwin/arm64 没有变。
- 一个函数里从不同时存活的局部变量共用帧里的一个槽位，调用时要置空和释放的槽位变少：tifa shop 包里的 2021 个函数原来有 12191 个局部变量，现在只要 9983 个槽位，React DOM 的 `pushStartInstance` 从 138 个降到 11 个。被闭包捕获的、`eval`、`with` 或映射的 `arguments` 够得着的、还留着暂时性死区检查的、在 `catch` 或 `finally` 开头还活着的局部变量，照旧独占槽位。shop 工作台每次渲染少跑 1.43% 的指令，store 1.02%，MUI 0.82%，`r_react_ssr` 1.81%，HTML 不变。在生成构建上，一次渲染的用时是 main 的 1.002、1.000、0.995 和 0.994 倍，都在代码位置带来的波动之内。编译一个包会更慢（tifa shop 的包多 3700 万条指令），tifa 的 VM 不付这个钱，`r_react_ssr` 连编译带渲染跑一次多花 2.6% 的时间（细节见 [c-engine.md](docs/changes/c-engine.md#locals-that-share-a-slot)）。
- `JSON.parse` 遇到不带转义的 ASCII 键，直接从文本里读，键的 atom 从一个 64 格的缓存里取。缓存只活一次解析，结束时把 atom 的引用还回去。不带转义的 ASCII 字符串值按实际长度一次拷好，这种连续段的结尾一次找 8 个字节，新数组末尾的元素不走通用路径，文本转成 C 字符串也一次处理 8 个字节。一次解析的指令数：1000 行的表格（92 KB）少 60%，tifa 商店的订单少 31%，大商店页 1 MB 的数据少 54%，npm 的 `package-lock.json` 少 49%，GitHub API 的仓库列表少 50%。解析出的值、键的顺序、报错和报错位置、读中断标志的位置、嵌套上限都和原来一样。bench 应用表格页每次渲染少跑 3.91% 的指令，六个工作台页面少 0.31%，tifa 各应用的 16 个页面 HTML 都不变。linux/amd64 上 `JSON.parse` 用时：1000 行表格是原来的 0.357，订单 0.685，商店数据 0.517，两份公开数据 0.629 和 0.509。渲染用时：表格页 0.968，`/inject/1` 0.831，`/mall/en-US` 0.951（8 个代码位置、3 轮），其他页面变化都在 3% 以内，指令数变少，螺旋页持平（[Performance](docs/performance.md#jsonparse-reads-keys-and-plain-strings-from-the-text)）。

## [v0.29.6] - 2026-10-31

### 性能

- 生成的引擎调用自己的另一个函数时，直接从被调函数返回结果的寄存器里取结果，不再从 Go 栈读回。经指针的调用、调用 Go 代码（宿主回调）、调用没生成成功的函数，照旧读回。链接后的检查要求每个省掉读回的调用，被调函数都从这些寄存器存结果，之后只跑 Go 的收尾指令。tifa 示例应用的页面每次渲染少跑 0.37%（shop 工作台）到 0.65%（MUI）的指令，`r_react_ssr` 少跑 0.42%，linux/amd64 和 darwin/arm64 上 HTML 都不变。linux/amd64 上 shop 工作台用时是原来的 0.978，store 0.980，MUI 0.984，bench 页面 0.995，`r_react_ssr` 0.973（4 个代码位置、3 轮，都在 3% 以内），darwin/arm64 上是 0.990 到 0.997。生成的汇编小了 1.9%（amd64）和 2.1%（arm64）（数据见 [Performance](docs/performance.md#what-the-generated-code-costs-against-native-c)）。
- 编译器去掉不可能失败的暂时性死区检查：`let`、`const`、`class` 局部变量在声明之后、而且没有跳转能绕过声明到达的代码里的读写，直接用 `get_loc` 和 `put_loc`。一个局部变量如果检查全没了、没被闭包捕获、所在函数也没有 `eval`，它的块开始时就不再置为未初始化，除非循环或 `finally` 会再跑一遍这个块。`switch` 的 case、闭包里或声明之前的读、循环头、`catch` 和 `finally` 之后的代码，检查照旧保留，该抛的 `ReferenceError` 一个不少。shop 工作台每次渲染少跑 0.61% 的指令，store 0.34%，MUI 0.77%，bench 页面 0.11%，`r_react_ssr` 的一次渲染 0.06%，HTML 不变。编译一个包会稍慢一点（`r_react_ssr` 的包多 66 万条指令），tifa 的 VM 不付这个钱。linux/amd64 上 shop 工作台用时是原来的 0.995，MUI 0.997，bench 页面 0.990（4 个代码位置、3 轮，都在 3% 以内）（数据见 [Performance](docs/performance.md#checks-of-the-temporal-dead-zone)）。

## [v0.29.5] - 2026-10-31

### 变更

- 字节码升到版本 7，多了几个操作码：读局部变量后紧跟按名字读属性、字符串 `switch` 的 case、`typeof` 判断。旧版本写出的字节码不再能加载（`SyntaxError: invalid version`），要重新编译。`JS_ReadObject` 遇到后面没跟着原指令的合并操作码会拒绝。
- 对两个相等且长于 65536 个字符的字符串、或对 rope 做 `===`、`!==`、`==`、`!=` 和 `switch`，以及在 `Map` 或 `Set` 里用长字符串或 rope 作键查找一个相等的键，现在每 65536 个字符读一次中断标志，`Interrupt` 或 eval 超时后几次轮询内就会停下。原来一次调用比完整个字符串，两个 3200 万字符的字符串要约 3300 次轮询（10 ms），字符串再长脚本就会被杀。同一个字符串或 rope 与自己比较现在直接判相等。
- 超过 65536 个字符的字符串用作属性键（`o[s] = 1`、`s in o`、`o[s]`、`JSON.parse` 的键），在算哈希和与相等字符串的原子比较时，每 65536 个字符读一次停止标志。`JSON.parse` 扫描文本里的字符串（键或值）时，每 65536 个字节读一次。原来这几处都是对整个字符串一口气走完，3200 万个字符的哈希约 800 次轮询，与相等的原子比较再约 3300 次，扫描 JSON 字符串约 4000 次，中途都不停。
- 对超过 65536 个字符的相等字符串做 `indexOf`、`lastIndexOf`、`includes`、`startsWith`、`endsWith`、`replace`、`replaceAll`、`split`（模式和字符串剩下的部分一样长），做 `Array.prototype.indexOf`、`lastIndexOf`、`includes` 和 `Object.is`，以及 `JSON.stringify` 一个长字符串，现在每 65536 个字符读一次停止标志。原来它们一次调用就把整个字符串比完或拷完，3200 万字符约 3400 次轮询。
- `JSON.parse` 长字符串时，整个字符串（含转义）共用一份预算，每 65536 个字节读一次停止标志。原来全是转义的字符串，或每 32768 个字符夹一个转义的字符串，要读到底才停。文本转成 C 字符串、长 ASCII 之后出现大于 0xff 的字符而把前面的字符扩宽、缓冲区超过 1 MiB 后增长时的拷贝，现在都分步做并读标志（`padStart`、`padEnd` 用宽字符补位也一样）。
- 长键的哈希，以及它和 Map、Set 或对象里哈希相同、长度相同的键的比较，共用一份预算，比到不同处为止的比较也要计入它做过的工作。哈希加比较的 131071 个字符每 65536 个读一次标志，不再是 131071，一串和所找的键到很晚才不同的键，读标志的次数和一串相等的键一样多。
- 恰好 65536 个字符的字符串现在也和更长的字符串一样分步读停止标志：`===` 和 `!==`、`Map` 或 `Set` 的键、`Array.prototype.indexOf`、`lastIndexOf`、`includes`，以及属性键。原来它们把 65536 当成短串，一次调用就比较或算完哈希。
- 一个循环要比较很多个短于 65536 个字符的字符串时，把每次比较的长度加起来，加到超过 65536 个字符就读一次停止标志：对几千个等长字符串组成的数组做 `Array.prototype.indexOf`、`lastIndexOf`、`includes`，以及在哈希和长度都相同的一串键里查找 `Map` 或 `Set`。一个由 16384 个 65535 字符的字符串组成的数组，找一个只有最后一个字符不同的字符串，原来要做约十亿个字符的比较才读一次停止标志。
- `JSON.parse` 和 `JSON.stringify` 在缓冲区已经是 16 位（出现过大于 0xff 的字符）之后，把一段 8 位字符拷进去，每 32768 个字符读一次停止标志，因为每个字符要先看一遍再拷贝。超过 1 MiB 的缓冲区要增长时，在拷贝的第一步之前先读一次标志。它现在拷进新的内存块，拷贝期间新旧两块同时存在（原来分配器可以不要第二块就把大块原地扩开）。内存上限按 `realloc` 的方式检查，即对照扣掉旧块之后的用量，所以以前放得下的缓冲区现在照样能增长，除非先碰到足迹上限，或者主机在旧块还在时拒绝再映射一块。拷贝期间占用的内存可能多出约一个旧块，这个说法不是严格的上界。

### 修复

- `GetProperty`、`GetPropertyValue`、`SetProperty`、`SetPropertyValue`（以及调用它们的 `Value` 方法）、`Value.MarshalJSON`、`Value.Any` 和 `ErrorFromValue` 现在和 `Eval`、`Call` 一样受求值超时管。以前这些方法不启动计时，getter、setter、代理拦截器、`toJSON`、被改过的 `BigInt.prototype.toString`，或读取错误时调用的 `toString` 想跑多久就跑多久。超时设为 10 毫秒时，一个跑 200 毫秒的 getter 照样返回 42，没有错误。`SetProperty` 和 `SetPropertyValue` 现在返回 setter 抛出的异常，以前返回 `failed to set property`，还把异常留在上下文里没取走（[细节](docs/changes/uncatchable.md#every-method-that-can-run-javascript-has-the-eval-timeout)）。
- 引擎里搬进了 23 个上游 QuickJS 的修正，每个都带测试，测试输出和 Node 24 一样（照现行规范、Node 24 还没跟上的几行有标注）：负数 BigInt 的 `BigInt.asUintN`、`Iterator.prototype.take` 和 `drop` 的上限、`''.repeat(2 ** 31)`、转换时改了缓冲区大小的 `ArrayBuffer.prototype.resize` 和 `TypedArray.prototype.at`、`try` 的完成值、迭代器辅助方法的 `return()`、`yield` 和 `await` 当名字用、宽松模式下的 `f() = 1`、`Promise.try`、导入后又导出的绑定在 `export *` 里不再算歧义、非扩展接收者上的 `super.x = v`、for-in 头里函数的名字、绑定函数的原型、代理的 getOwnPropertyDescriptor 和 ownKeys 拦截器、`Object.defineProperties` 的顺序、`get`、`set`、`static` 后面的类成员、`Array.prototype.sort` 把每个元素写回、`for await` 提前离开时等 `return()`，以及导出里的字符串名字。两处泄漏也修了：失败的 `Atomics.store`，和读取被截断、带 BigInt 的字节码。密封或冻结的数组，在越过末尾的下标上赋值或定义时，长度不再变。test262 的已知失败少了 25 个，剩 33 个。清单，以及 C 树和上游、Node 24 不同的地方，见 [upstream-commits.md](docs/changes/upstream-commits.md)。
- 删除对象的键时，已删的键攒到 8 个以上且占一半以上，引擎会把属性列表压紧。压紧时 VM 没有内存放更小的属性列表，原来会留下一个「内存不足」错误挂在 VM 上，尽管这次删除已经成功。这个不可捕获的错误会顶替脚本接下来抛的下一个错误。现在压紧是可选的：内存不足时删除照样成功，对象留到之后的删除再压紧，不留错误。

### 性能

- 字节码函数的调用结束时，没有引用计数的值八个八个地跳过，原来是两个两个地看。tifa 示例应用的页面少跑 0.7% 到 2.1% 的指令，渲染脚本少 1.3% 到 2.0%，HTML 不变。一个函数有 200 个局部变量、一个都没赋值，这些局部变量在一次调用里少花 37% 的指令。找到的唯一变慢的情形是每四个局部变量里有一个放着对象的函数：200 个局部变量时多 2.4% 的指令（数字见 [Performance](docs/performance.md#the-end-of-a-call-and-the-start-of-one)）。
- 字节码函数的调用开始时，把局部变量设成 `undefined` 改为八个八个地设，原来是四个四个。tifa 示例应用的页面和渲染脚本少跑 0.2% 到 0.5% 的指令，HTML 不变。有 200 个局部变量的函数，设置和释放这些局部变量比上一条的改动之后又少花 14% 的指令，只有 3 个及以下局部变量的函数，一次调用多 1 到 4 条指令。
- 写在不同地方、键相同的对象字面量共用一个形状，和逐个加键建出来的对象一样。字面量的第一个对象原来从 `Object.prototype` 的空形状开始找自己的形状，而这个空形状通常已被第一个加键的对象原地改掉，于是每处字面量各建一个形状。tifa 页面的一个 VM 渲染后形状少 12% 到 15%，内存少 0.7%（shop 和 MUI 页面）到 1.7%（店面页面）。渲染跑的指令不变（[objects-and-shapes.md](docs/changes/objects-and-shapes.md#object-literals-of-the-same-keys-share-one-shape)）。
- 读第一个或第二个参数、第一个局部变量或第四个之后的局部变量，再紧跟按名字读属性（`o.x`），以及字符串 `switch` 的 case，解释器一轮做完。tifa 示例商店的六个工作台页面每次渲染少跑 1.85% 的指令，店面页面少 1.59%，MUI 页面少 1.66%，bench 页面少 3.41%。bench 页面渲染时间少 3.4%（8 个代码位置，3 轮，[c-engine.md](docs/changes/c-engine.md#opcodes-that-do-a-few-instructions-at-once)，[性能](docs/performance.md#opcodes-that-do-a-few-instructions-at-once)）。
- `typeof x == "string"` 这类判断（`==`、`===`、`!=`、`!==` 配任意类型名），以及压缩工具写出的 `typeof x < "u"` 和 `typeof x > "u"`，解释器一轮做完。tifa 页面每次渲染少跑 0.63%（shop）到 1.42%（bench 页面）的指令。

## [v0.29.4] - 2026-10-30

### 性能

- 只有普通键的对象字面量把属性放在对象自己的块里，不再单独分一块。tifa 页面的一个 VM 渲染并回收后少占 1.0% 到 2.4% 内存，一次渲染少跑 0.1% 到 0.8% 的指令，按规则看没有页面变快或变慢（shop 为 0.991，MUI 页面 0.998，bench 应用 0.985，React 0.995）。关掉回收器时，一次渲染新增的内存平均多 0.7%（数字见 [Performance](docs/performance.md#object-literals-with-their-properties-in-the-block-of-the-object)）。
- 解释器调用 C 写的内置函数时直接调，不再先进一层跑字节码的函数（这种调用里它什么也不做）。tifa 示例商店的六个工作台页面每次渲染少跑 0.74% 的指令，bench 页面少 2.3%，用时是原来的 0.98 到 0.99，在代码位置带来的波动之内。脚本递归的深度和以前完全一样，经过 `call`、`map`、`sort` 这类内置函数的递归也一样（[性能](docs/performance.md#c-functions-called-from-the-interpreter)）。

## [v0.29.3] - 2026-10-30

### 新增

- `VM.RegisterCopyProps(name)` 在全局对象上定义名为 `name` 的函数，做 React 的 `createElement` 和 `cloneElement` 把 config 的键拷进元素 props 的那个循环所做的事，在引擎里用 C 写成。它只拷结果和循环不可能不同的 config（页面看不出区别的那种：键不超过 32 个、值都是普通值的普通 config，同一种 props，没改过的 `Object.prototype` 和 `Function.prototype.call`），其余的 config 它回答 `false` 且什么都没改，脚本再把 config 交给循环。引擎自己不往全局对象上放任何东西：宿主注册了，VM 里才有这个函数。在 tifa 里，React 用了它，渲染一次少跑 2.1% 到 4.1% 的指令，示例商店的页面渲染只花原来的 0.988 和 0.989，MUI 页面是 0.980。（[性能](docs/performance.md#the-copy-of-the-props-of-a-react-element-in-c)，[Go API](docs/changes/go-api.md#registercopyprops)）

### 变更

- 引擎里逐个遍历字符串、数组、Map 或 Set 的元素、每个元素除此之外不调用别的东西的循环，在 `Interrupt` 或执行时限之后，几次轮询之内就停下：`Object.fromEntries`、可迭代对象的展开（`[...set]`）、`Iterator.prototype.toArray` 和 `drop`、`Math.sumPrecise`、`Set.prototype.union`、`symmetricDifference` 和 `isSupersetOf`，以及 `Array.from`、类型化数组的构造函数和 `from`。引擎原来只在一次调用开始时读停止标志，而这些迭代器的 `next` 不经过这样的调用，所以展开一个有两百万项的 Set，或求两个大 Set 的并集，会一直跑到脚本被杀。Set 的方法在添加之前对 Set 做的那份拷贝也会停下。
- 模式是字符串时，`replace` 和 `replaceAll` 像 `indexOf` 一样查找，并且每 1024 次匹配读一次停止标志。原来它们在每个首字符相同的位置逐个比较针串，所以一根几乎处处匹配的针（一百个 `a` 加一个 `b`，放在一百万个 `a` 的字符串里）要花两个长度的乘积，匹配几百万次的 `replaceAll` 会一直跑到被杀。`trim`、`slice`、`substring`、`substr` 和 `split` 切出的片段超过一百万个字符时，在一百万个字符之内就停下，`padStart` 和 `padEnd` 的填充串有几千万个字符时也一样。
- 键是很长的字符串或 rope 时，`Map` 和 `Set` 的操作（`get`、`set`、`has`、`delete`、`getOrInsert`、`add` 和 `Set` 的各个方法）在 `Interrupt` 或执行时限之后，几千个字符之内就停下。给键算哈希要走完整个键，一千万个字符要几十毫秒，脚本会因此被杀。

## [v0.29.2] - 2026-10-30

### 新增

- `VM.RegisterEscapeText(name)` 在全局对象上定义名为 `name` 的函数，像 React DOM 字符串渲染器的 `escapeTextForBrowser` 那样给 HTML 转义文本，在引擎里用 C 写成。参数先变成字符串，同 `"" + text`（对象走 `valueOf` 和 `toString`，symbol 会抛错），再把 `&`、`<`、`>`、`"` 和 `'` 换成 `&amp;`、`&lt;`、`&gt;`、`&quot;` 或 `&#x27;`。里面没有这五个字符的字符串原样（同一个字符串）返回。引擎自己不往全局对象上放任何东西：宿主注册了，VM 里才有这个函数。在 tifa 里，React 的转义用了它，渲染一次少跑 0.7% 到 4.5% 的指令，示例商店的页面渲染只花原来的 0.974，bench 页面是 0.938。（[性能](docs/performance.md#react-doms-text-escape-in-c)，[Go API](docs/changes/go-api.md#registerescapetext)）

### 变更

- `VM.RunGC` 跑完后把 GC 阈值设成它留下的内存的 1.5 倍，和引擎自己启动的 GC 一样。以前它不动阈值：脚本把堆撑过阈值但没有新建对象（数组元素就这样）时，`RunGC` 之后下一次调用的第一个对象会再启动一次同样长的完整 GC。阈值是 `^uintptr(0)`（关掉引擎自己的 GC）时保持不变。自己在 `RunGC` 之后设阈值的程序（比如 tifa）看不到变化。
- 对象展开（`{...o}`）、解构的剩余部分和 `Object.assign` 按规范，轮到每个键时再查一次源对象。前面的 getter 删掉后面的键，或把它改成不可枚举，这个键现在不会被复制。以前它照样被复制：值是 `undefined`，或者是原型上同名属性的值。前面的 getter 把后面不可枚举的键改成可枚举，这个键现在会被复制，以前会漏掉。`Object.keys({ ...{ get a() { delete this.b; return 1 }, b: 2, c: 3 } })` 现在是 `["a", "c"]`，和 Node 一致。
- `isWellFormed`、`toWellFormed`、`split`、`padStart`、`padEnd`、`trim`、`trimStart`、`trimEnd`、`anchor`、`fontcolor`、`fontsize`、`link`、`escape`、`unescape`、`encodeURI`、`encodeURIComponent`、`decodeURI`、`decodeURIComponent`、`Array.from`、类型化数组的构造函数和 `from`、把 rope（不复制各部分、只把它们连起来的长字符串）变成普通字符串、两个 rope 用 `<` 比较，在 `Interrupt` 或执行时限之后，几千个字符或元素之内就停下。字符串有几千万个字符（或同样多的元素）时，它们以前要跑到结束，长到足以让脚本被杀。两个 rope 用 `===` 比较仍然不读标志，和别的字符串一样。

## [v0.29.1] - 2026-10-30

### 性能

- 普通对象（对象字面量、`{}`、React 元素的 props）占 48 字节内存，不再是 72 字节。长 Markdown 页面渲染时 `MemorySize` 的最高值平均低 3.8%，示例商店的 20 个页面渲染结束时占用少 1.8%。没有页面渲染变快或变慢（数字见 [Performance](docs/performance.md#ordinary-objects-of-48-bytes)）。
- `EvalSharedBytecode` 加载同一个 `SharedBytecode` 时，第一次之后只花原来的 0.79（React 页面的包）到 0.86（tifa 商店的包）：引擎对字节码代码的检查（是否遵守轮询规则）每个 `SharedBytecode` 只做一次，不再每个 VM 做一次（见 [Performance](docs/performance.md#what-a-new-vm-and-loading-a-bundle-cost)）。`EvalBytecode` 仍然每次都检查。
- 只有普通键的对象字面量一次拿到全部键，用所在函数为它留着的 shape，属性数组正好那么大。不开 PGO，linux/amd64，8 个代码位置，HTML 不变：tifa 示例商店的工作台页面耗时为原来的 0.956，tifa 测试应用的 MUI 页面为 0.969，bench 应用的表格页面为 0.942，React 服务端渲染没有变化（0.991）。商店的一个 VM 多占约 0.17 MiB，长 Markdown 页面的峰值少 1.6%。细节见 [objects-and-shapes.md](docs/changes/objects-and-shapes.md#object-literals-made-at-their-final-shape)。
- 字节码函数的调用把局部变量设成 `undefined` 时一次设四个，结束时一次释放两个值。不开 PGO，linux/amd64，8 个代码位置，HTML 不变：tifa bench 应用的表格和螺旋页面耗时为原来的 0.958 和 0.967。tifa 示例商店的工作台页面（0.996，指令少 2.5%）、tifa 测试应用的 MUI 页面（0.999）和 React 服务端渲染（0.984，指令少 3.1%）按规则不算变化。反复调用一个没有局部变量的函数，指令多 3%。细节见 [c-engine.md](docs/changes/c-engine.md#how-a-call-sets-and-frees-its-values)。
- 普通对象上的 `for in` 循环拿住对象的 shape，不再列一份键的清单，`Object.prototype.hasOwnProperty.call(obj, key)` 在解释器里直接给出答案，不再调用 `call` 和 `hasOwnProperty`。在上一项之上，商店耗时为原来的 0.984（指令少 2.5%），MUI 页面 0.981，bench 应用的螺旋页面 0.977，React 0.988：商店和 MUI 页面每个位置都更快，但幅度没到规则的要求。两项合起来，商店耗时为改动前的 0.982，MUI 页面 0.976，bench 页面 0.952，React 0.969，每个位置都更快。内存不变。细节见 [objects-and-shapes.md](docs/changes/objects-and-shapes.md#a-for-in-loop-holds-the-shape-of-its-object)。

### 变更

- `intl.Register` 和 `temporal.Register` 在全局对象上标记 VM 已装好的符号改名为 `Symbol.for("github.com/liustack/slowjs/intl")` 和 `Symbol.for("github.com/liustack/slowjs/temporal")`，原来用的是 modernc.org/quickjs 的名字。
- 脚本在执行时限或 `Interrupt` 之后，除了过了 50 毫秒，还要让引擎又干了 1000 次轮询的活，才会被杀。引擎没在干活的时间（在脚本调用的 Go 函数里，或者 OS、Go 调度器在跑别的线程或 goroutine）不再让一个本来会自己停下的脚本被杀：调用返回 interrupted 错误，VM 照常可用。以前在负载高的机器上，一个不到 1 毫秒的活就能停下的脚本也可能被杀。
- `String.prototype.normalize`、`localeCompare`、`toUpperCase` 和 `toLowerCase` 在 `Interrupt` 或执行时限之后，几千个字符之内就停下。以前遇到长字符串，它们要把整个字符串转成码点、比较完或改完大小写才停。
- 字节码版本升到 6，用来存对象字面量的键：旧版本写出的字节码不能再读入（`SyntaxError: invalid version`），要重新编译。

## [v0.29.0] - 2026-10-29

### 变更

- **不兼容：** 引擎换成 C 引擎，而且只有它。它是 C 写的 QuickJS（`engine/`），由 LLVM 编译成 Go 汇编，随模块一起发布，所以构建不需要 C 编译器，也不用 cgo。到 v0.28 为止，引擎是 ccgo 翻译成 Go 的 QuickJS，那个引擎已经删掉。结果、错误堆栈和 HTML 不变，下面几条除外。
- **不兼容：** 模块只为 linux/amd64 和 darwin/arm64 构建。在其他平台上构建，包括 windows/amd64、darwin/amd64 和 linux/arm64，会报错并写明这两个平台。
- **不兼容：** 脚本的嵌套调用按次数限制，不再按栈。`SetMaxStackSize` 的单位是调用次数：默认 5000，最多 10000，而 Go 引擎数的是 10000 个栈槽。递归超过约 5000 次调用的脚本会更早得到 `RangeError`。JSON 和括号最多嵌套 2000 层和 665 层（tifa 的 `/json/2001` 页面返回 500）。以前会让引擎崩溃的链，比如 300000 个绑定函数，现在抛 `RangeError`。
- 脚本在执行时限或 `Interrupt` 之后又跑了 50 毫秒还没停下（比如卡在它调用的 Go 函数里），就被杀掉。这次调用返回一个 interrupted 的 `*Error`，它也匹配 `ErrKilled`，这个 VM 之后只能 `Close`。
- `MemorySize` 和内存限额数的是引擎自己的分配器占着的内存，同一堆对象比 ccgo 引擎数的少 13% 到 15%。VM 按块向系统要内存，`MemoryUsage` 会填上 `Footprint`、`FootprintLimit`、`Chunks` 和 `ReleaseCandidate`，以前这四项是 0。`ReleaseMemory` 还回空闲内存，并返回还了多少字节。
- `Math.*` 在两个平台上给出和以前相同的位（`engine/test/math.sh` 对 33 个运算各检查 100000 个输入）。
- 编译或惰性属性在内存耗尽时，调用以内存不足错误结束，ccgo 引擎可能留下挂起的错误或读到错的值。编译 `\p{Script=...}` 时内存耗尽不再让进程崩溃。
- 模板字符串按规范马上把每个替换项转成字符串，`-0.5 < -1n` 是 false。两处都来自上游 QuickJS 的提交（[upstream-commits.md](docs/changes/upstream-commits.md)）。

### 移除

- 构建标签 `slowjs_nogoasm`、`goasm`、`slowjs_cengine` 和 `libc.memgrind`。前三个现在什么都不做，最后一个以前用来跟踪 ccgo 引擎的分配。
- 包 `github.com/liustack/slowjs/lib`，也就是 ccgo 引擎和它的 API。根包、`intl`、`temporal` 和 `fmt` 保留上一个版本里所有导出的名字：`SetModuleLoader` 和 `SetDefaultModuleLoader` 在 v0.27.0 就已删除，仍然没有。
- 依赖 `modernc.org/libc`、`modernc.org/ccgo/v4` 以及它们的依赖。`go.mod` 现在只要 `golang.org/x/sys` 和 `golang.org/x/arch`，import 这个模块的程序链接 `golang.org/x/sys`，不再链接 modernc.org 的任何包。

### 性能

对比上一个版本（带它的 PGO 采样），C 引擎不开 PGO，linux/amd64（AMD EPYC 7B13，单核，各 8 个代码位置）。

- tifa 示例商店、MUI 测试应用和 bench 应用的 11 个页面，渲染耗时是原来的 0.834 到 0.860（每个位置都更快），HTML 相同，只有 2001 层 JSON 的页面除外。
- Go 垃圾回收的停顿保持一样短：1 个 VM 时一次停顿的 P99 是 131 到 164 µs（以前 164），3 个 VM 是 197 µs（以前 197），最长 3.1 ms（以前 4.2）和 0.9 ms（以前 2.1）。
- 商店的一个 VM 加载后占 8.3 到 8.6 MiB，渲染 300 次后 12.8 到 12.9 MiB，池把空闲内存还给系统后 9.7 MiB。商店程序大 0.5%，帧信息表小 152 KB。
- 150 分钟、211,076 次渲染的 soak 没有错误，没有 VM 被换掉，VM 的内存没有变化。方法和其余数字见 [历史](docs/history.zh-CN.md#切换前的验收)。
- 跑同一份 JavaScript，tifa 示例商店、MUI 测试应用和 bench 应用的页面耗时是 C QuickJS 2026-06-04 的 0.98 到 1.06 倍，MUI 页面是 0.81。用自带的 `Intl`（C 得靠 polyfill）时，商店是 0.82，MUI 是 0.52。能渲染这些页面的其他纯 Go 引擎耗时是 slowjs 的 1.6 到 6.6 倍（[Benchmarks](docs/benchmarks.md#tifa-pages)）。

### 修复

- 对已经关闭的 VM 再调 `Close`，什么都不做并返回 nil。以前它会空指针 panic，所以 `defer vm.Close()` 加一次显式 `Close` 会崩。`OnClose` 的函数仍然只跑一次。
- `Close` 中间某一步 panic 时，后面的步骤照样做完。`OnClose` 里的函数 panic，不再让排在它后面的函数永远不跑。`Close` 里的清理也不会被永久跳过：VM 会从查找表里去掉，它占着的共享字节码会释放，最后 `Close` 把第一个 panic 继续抛出去。在 `OnClose` 的函数里再调 `Close`，直接返回 nil。
- C 引擎上，系统拒绝解除一个 VM 的部分内存映射时，`Close` 返回那个错误，再调 `Close` 会把剩下的映射解掉。以前第二次返回 nil，映射还留着。

## [v0.28.6] - 2026-10-24

### 变更

- 在 linux/amd64 上，引擎默认用 goasm 解释器执行 JavaScript，也就是 v0.28.5 的 `goasm` 标签打开的那个 Go 汇编解释器。加 `-tags slowjs_nogoasm` 构建，程序就改用 Go 解释器，其他平台一直用的就是它。`goasm` 标签仍然可以写，但不再有任何作用。运行结果、错误栈和 HTML 都不变，构建仍不需要 C 工具链。不开 PGO、一个 VM 时，tifa 示例商店的页面渲染耗时是原来的 0.940，tifa 测试应用的 MUI 页面是 0.850。tifa 的 VM 池在 4 个核上跑 4 个 VM 时，每秒渲染的商店页面是原来的 1.046 倍，MUI 页面是 1.171 倍，在 8 个超线程上跑 8 个 VM 时是 1.085 倍和 1.211 倍。8 个 VM 跑十分钟后，进程的 RSS 在商店页面上是原来的 0.93，在 MUI 页面上是 0.88。GC 让所有 goroutine 停下来的时间和用 Go 解释器时一样快。见 [The goasm interpreter](internal/goasm/gen/README.md)。

## [v0.28.5] - 2026-10-24

### 新增

- `goasm` 构建标签（`go build -tags goasm`）：在 linux/amd64 上，引擎改用一个解释器执行 JavaScript，它由 LLVM 从 QuickJS 的 C 源码编译而来，以 Go 汇编的形式随包发布，替换 ccgo 从同一份 C 生成的 Go 解释器。默认不开，在其他平台上加这个标签什么都不变。运行结果、错误栈和 HTML 都不变，构建仍不需要 C 工具链。不开 PGO 时，tifa 示例商店的页面渲染耗时是原来的 0.940，tifa 测试应用的 MUI 页面是 0.850。它怎么生成、默认打开前还要满足哪些条件，见 [The goasm interpreter](internal/goasm/gen/README.md)。
- `GetAsyncContext` 和 `SetAsyncContext`，两个 Builtin，读写 VM 的异步上下文：一个 promise 反应会带着走的对象。`then`、`catch`、`finally` 或 `await` 运行时用的是注册那一刻 VM 的上下文，thenable 的 `then` 用的是拿它去 resolve 一个 promise 的那段代码的上下文，和 Node 一样。用它们写的 `AsyncLocalStorage` 在 `await` 之后照样找得到自己的 store。从不设置上下文的 VM 跑 promise 和以前一样。

### 性能

都是不开 PGO，和 v0.28.2 比。

- 列普通对象的自有键（`for in`、`Object.keys`、`values` 和 `entries`、对象展开和剩余解构、`Object.assign`、`JSON.stringify`）改在 Go 里直接从它的 shape 读，展开、剩余解构、`values` 和 `entries` 直接从对象里取普通属性的值：tifa 示例商店的 bench 页面耗时为原来的 0.990，store 页面为 0.994，tifa 测试应用的 MUI 页面为 0.987，HTML 不变。六个键的 `Object.keys` 从 542 ns 降到 134 ns，展开六个键从 250 ns 降到 74 ns（Apple M4）。每个 VM 多占 8 KiB。

## [v0.28.4] - 2026-10-24

### 变更

- `EvalBytecode`、`EvalBytecodeValue`、`EvalSharedBytecode` 和 `EvalSharedBytecodeValue` 把一个模块跑完后，释放它的顶层函数，连同顶层里没有被模块中任何函数用到的变量。以前加载过模块的 VM 要到关闭时才释放它们。把 tifa 示例商店的 SSR 包当模块加载，一个 VM 占 8.66 MiB，原来 9.44 MiB，和同一个包当脚本加载（8.63 MiB）差不多。还在等待中或失败的模块保留它的顶层函数。

## [v0.28.3] - 2026-10-24

### 修复

- 带 v 标志的正则接受规范允许、Node 也接受的写法：字符类里 `\` 加 ``& ! # % , : ; < = > @ ` ~`` 中的一个（如 `[\&\;]`、`[\!\|]`），字符类外的 `-`、`/` 和 `&&` 这类成对标点（如 `/-/v`），以及 `\q{...}` 里的 `\-`、`\&`。以前这些都报 SyntaxError，shiki 的 JavaScript 引擎为 bash、css、go、html、json、typescript 和 yaml 生成的 552 个正则里有 204 个编译失败。现在除 3 个以外都能编译，那 3 个的捕获组超过 254 个，这是一个正则能容纳的上限。以前能编译的正则，编译出的字节码不变。

## [v0.28.2] - 2026-10-24

### 新增

- `VM.SetClock` 和 `VM.Now` 给一个 VM 设一个墙上时钟，`Date.now()`、`new Date()`、`Date()`、Intl 不传日期的格式化和 `Temporal.Now` 都读它。传 nil 恢复实时时钟。非 nil 的 `temporal.Clock` 仍只覆盖 `Temporal.Now`。`performance.now`、eval 超时、`Atomics.wait` 和 `Math.random` 的种子继续用各自的时钟。

## [v0.28.1] - 2026-10-23

### 新增

- `NewSharedBytecode`、`VM.EvalSharedBytecode` 和 `VM.EvalSharedBytecodeValue`：加载同一份字节码的多个 VM 共用一份函数源码和给错误栈提供行号的表，整个进程只存一份，以前每个 VM 各复制一份到自己的内存里。加载后，tifa 示例商店的一个 VM 少占 2.36 MiB（8.99 MB，原来 11.46 MB），tifa 的 MUI 测试应用少占 1.22 MiB（4.16 MB，原来 5.44 MB）。`toString`、错误栈和行号不变。`MemorySize` 和内存上限不再算这部分。`SharedBytecode` 和所有用它加载过的 VM 都关闭后，这份副本才释放，所以 VM 可以比它活得久。渲染时间和 HTML 不变。

## [v0.28.0] - 2026-10-23

### 变更

- **不兼容：** 模块现在是 `github.com/liustack/slowjs`，包名是 `slowjs`，支持 amd64 和 arm64 上的 linux 与 darwin，以及 amd64 上的 windows。

## [v0.27.3] - 2026-10-23

### 新增

- `VM.RunYoungGC`（`lib/` 里是 `JS_RunYoungGC`）：只回收上次回收（年轻或完整的都算）以后新建的对象。它释放这些对象之间的循环，留下的对象变成老对象。完整回收 `VM.RunGC` 要看 VM 里的每个对象，tifa 示例商店的 bundle 在还没有垃圾时就有约 30,000 个。一次只做一件事（比如渲染一个页面）的程序，可以每做完一件跑一次年轻回收，隔一阵再跑一次完整回收。完整回收能释放的东西，跑过多少次年轻回收它照样能释放。经过老对象的循环和 WeakMap 的值，年轻回收会留到下一次完整回收。不调用它的程序回收方式不变。

### 修复

- 字符串的 `indexOf`、`lastIndexOf` 和 `includes` 放在循环里，能在 10 ms 超时后 20 ms 内停下：每次查找按扫过的字符数计入中断检查。以前在 1 MB 且没有匹配的字符串里，它们要多跑 170 ms、6.9 s 和 170 ms。

### 性能

- 字符串的 `includes`、`indexOf` 和 `lastIndexOf` 用 Go 标准库 bytes 包的查找直接定位目标串，上游是逐个位置比较：在 100 KB 的 HTML 里找一个不存在的串从 320 µs 降到 81 µs。商店渲染耗时为原来的 0.995，tifa 测试应用的 MUI 页面为 1.003，都在代码布局带来的波动之内。结果不变。
- tifa 原来每隔几次渲染做一次完整回收，改成每次渲染后跑 `RunYoungGC`、隔一阵再做一次完整回收后，商店渲染的 P99 在一个 VM 时是原来的 0.748，两个 VM 时是 0.796，tifa 测试应用的 MUI 页面是 0.940 和 0.919。P50 和总 CPU 的变化都在 2.1% 以内，内存峰值是原来的 0.82 到 0.88。不开 PGO。

## [v0.27.2] - 2026-10-23

### 新增

- `Args.Latin1` 和 `Args.UTF16`：按引擎存放的样子拿到 Builtin 字符串参数的字符，每个字符一个字节或一个 16 位单元，不复制。`VM.NewStringLatin1` 和 `VM.NewStringUTF16` 用这样的字符造字符串。只看短字符串、常常原样返回参数（`a.Value(0).Dup()`）的 Builtin 这样写就什么都不用转换：这样写的 React HTML 转义处理 16 个字符的字符串每次调用 99 ns，React 自己的 JavaScript 要 200 ns。


### 修复

- eval 超时和 `Interrupt` 能在几毫秒内停下反复编译正则的循环，单次很长的编译也能在中途停下，抛出的是同一个不可捕获的中断。以前编译一个正则不管模式多长都只算中断检查的一步，所以 10 ms 超时下，每次用新的长模式 `new RegExp` 的循环要晚 225 ms 到 184 s 才停（看模式而定），一次编译 10 万个分支要跑满 8.5 s。现在都在 10 到 12 ms 停下。
- `JSON.parse`、`JSON.stringify`、`eval`、`new Function`、`repeat`、`padStart`、`replaceAll`、`join` 以及其他解析源码或拼字符串的内置函数，放在循环里也能在超时后几毫秒内停下，以前输入在 60 KB 到 1 MB 时要多跑 5 到 37 s。单次调用仍会跑完。只扫描或排序、不拼字符串的内置函数（数组的 `indexOf` 和 `includes`、`split`、`fill`、`sort`）放在循环里仍会晚停。渲染耗时和内存不变（不开 PGO，和 v0.27.1 比，商店页面为原来的 0.993，MUI 页面为 0.997）。在一次很长的编译里超过截止时间的脚本，现在编译完就收到中断，以前可能直接跑完。
- 完整 GC 在一次回收中释放不可达的 WeakMap 条目，包括 value 指回 key 的循环和条目链。仍然可达的 key 会保留其 value。Symbol key、WeakSet、WeakRef 和 FinalizationRegistry 按同样的规则处理。tifa 页面上的完整 GC 耗时和以前差不多，WeakMap 很大时更慢：10 万个存活条目连成的链约为原来的 2.9 倍，10 万个存活 key 约为 1.9 倍。

### 性能

- `Args.String`、`Args.WTF8` 和 `VM.NewString`（以及所有变成 JavaScript 字符串的 Go 字符串）在 Go 里直接读写引擎的字符，结果不变。读一个 16 字符的字符串再返回新字符串的 Builtin 从 230 ns 降到 134 ns，1,024 个 ASCII 字符从 1338 ns 降到 521 ns。`Value.Dup` 不再占 TLS 帧：原样返回参数的 Builtin 从 97 ns 降到 81 ns。

## [v0.27.1] - 2026-10-23

### 修复

- 使用 `lib/internal/patch/quickjs.diff` 的原生 C 构建保留上游的 computed goto 分发，仅为 ccgo 和 Emscripten 禁用。生成的 Go 引擎保持不变，保留不可捕获异常、递归 RangeError 和错误栈规则。原生 C 在相同字节栈限额下的递归次数会随编译器帧大小改变。

### 性能

都是不开 PGO，和 v0.27.0 比。

- 字符串、数字和数组的 `o.x` 和 `o.x(...)` 在解释器里直接到原型上找方法，不再走通用的属性读取：商店耗时为原来的 0.996，tifa 测试应用的 MUI 页面为 0.975（它们的 CSS-in-JS 代码每次渲染读字符串方法最多约 32 万次）。内存占用不变。
- `js_malloc` 直接从释放小块的缓存里取块，不再经过原来的四层调用：耗时为原来的 0.985，tifa 的 MUI 测试应用的页面为 0.983。内存不变。

## [v0.27.0] - 2026-10-22

### 新增

- `Intl.DateTimeFormat` 的四个格式化方法支持 Temporal 值，按类型补默认字段并检查日历。`formatRange` 和 `formatRangeToParts` 也支持 Date 和数字。Temporal 日期的本地化使用同一套格式，通过共用的 go-intl 数据包支持 Chromium 的全部 18 种日历，包括 ISO。
- `Intl.Collator`、`Intl.Segmenter`、`Intl.RelativeTimeFormat`、`Intl.ListFormat`、`Intl.DisplayNames`、`Intl.DurationFormat`、`Intl.Locale` 和 `Intl.supportedValuesOf`，覆盖 Chromium 有的所有语言，在 14,269 个表达式上和 Chromium 156 对过结果。`localeCompare` 按语言的排序规则比较，`toLocaleUpperCase` 和 `toLocaleLowerCase` 遵循土耳其语、阿塞拜疆语、立陶宛语和希腊语的规则。它们和其他构造器共用 go-intl 数据。构建标签 `quickjs_intl_nonames` 和 `quickjs_intl_nocjdict` 可以省去显示名称及中文、日文词典。用了 `intl` 的程序需要附上 `NOTICE.md` 所列声明。当前体积和内存测量见 `docs/changes/time-and-intl.md`。
- `Args.WTF8`：读 Builtin 的字符串参数时，孤立代理项保持引擎里的样子，`Args.String` 则把每个换成 U+FFFD。
- `VM.SetRegExpCache`、`DefaultRegExpCache` 和 `DefaultRegExpCacheBytes`：一个 VM 最多留多少个编译好的正则，占多少字节（默认 256 个、1 MiB）。缓存算进内存上限，贴着上限跑的 VM 可以把缓存调小或关掉。
- `VM.OnClose`：VM 关闭时运行一个函数，自己用表按 VM 存东西的包可以在这时删掉对应的项。
- `temporal` 包：`temporal.Register(vm)` 加上 `Temporal`，V8 有的类型和方法都有，还有 `Date.prototype.toTemporalInstant`。计算由 go-intl 在 Go 里完成，内嵌 ICU 的 tz 2026e 时区数据，从不读主机的 zoneinfo。`Temporal.Now` 用 `intl.TimeZone` 设的时区和 `temporal.Clock` 设的时钟。原始 test262 `built-ins/Temporal` 测量通过 9210 次中的 9206 次。当前 `intl402/Temporal` 通过 4058 次中的 4038 次。原始实现的测量中，进程里第一次用要 13.4 ms，之后每个新 VM 第一次用要 1.1 ms。当前体积测量见 fork-changes 文档。

### 变更

- `StdAddHelpers` 的 `performance.now()` 改为从进程启动开始计时。以前在 Linux 和 macOS 上从系统启动开始，在 Windows 和 FreeBSD 上从 1970 年开始，还会随系统时钟的调整而跳动。在 macOS 上不再计入系统睡眠的时间。`Date.now()` 和 `new Date()` 的值和原来一样。
- 全部 Intl 构造器统一使用 go-intl，包括 NumberFormat、DateTimeFormat 和 PluralRules。Number、Date 的本地化方法共用这些数据，支持舍入增量、舍入优先级及全部日期日历。删除独立的 `intl/cldr.bin` 及其生成器。与 main `641e996` 比，使用应用 PGO profile 构建的商店程序小 0.11 MB。通过预编译格式方案和直接生成字符串，指定用例中预热后的数字格式化比 main 耗时减少 41% 至 42%，日期格式化耗时减少 28% 至 37%。本次提速保持全部已记录答案不变。见[测量结果](docs/changes/time-and-intl.md#focused-timing-and-memory-costs)。
- Intl 的时区名、偏移和夏令时标记统一来自与 Temporal 共用的内嵌 tz 2026e，不再读取主机 zoneinfo，也不需要夏令时修正表。Winnipeg 按较新的 tz 2026e 保持永久夏令时，与 Chromium 156 的旧数据有已记录的差异。
- 完整 Intl402 test262 通过 6764 次中的 6618 次，main `641e996` 为 6276 次。新增的 24 次失败断言在 Chromium 156 中也失败。已有 golden 集没有新增未解释的差异。

### 移除

- `lib` 去掉了 QuickJS 命令行（`quickjs-libc.c`）里的 `std`、`os` 和 `Worker` 模块，Go API 从来不注册它们：`js_init_module_std`、`js_init_module_os`、`js_module_check_attributes`、`js_std_set_worker_new_context_func`、`js_std_init_handlers`、`js_std_free_handlers`、`js_std_promise_rejection_tracker`、`js_std_await`、`js_std_loop`、`js_std_eval_binary` 和 `js_std_eval_binary_json_module` 都没有了。`StdAddHelpers`、`SetDefaultModuleLoader` 和模块的字节码照常可用。程序不再链接这些代码，也不再链接只有它们调用的 libc 部分（`fork`、`execve`、`popen`、`setenv`、文件和终端相关的函数）：只调用 `Eval` 的程序在 linux/amd64 上小 354 KB，tifa 示例商店的服务端按 `tifa build` 的方式构建（`-ldflags=-w`，用商店的 PGO 配置文件，linux/amd64）小 181 KB。

### 修复

- `Temporal.Duration.prototype.toLocaleString` 通过 `Intl.DurationFormat` 输出本地化时长，支持样式、单位选项和小数位，以前返回 ISO 时长字符串。
- 页面替换了 `String.prototype.concat`（QuickJS 用它实现模板字符串）时，`Intl` 不再抛错：v0.26.17 里 `Intl.NumberFormat`、`Intl.DateTimeFormat` 和 `toLocale` 系列方法在这种页面上会抛错。
- `Intl.PluralRules` 给紧凑记数法和科学记数法的数选复数形式时和 ICU 一样，每个 `supportedLocalesOf` 都会读 `localeMatcher` 选项。
- 语言列表里的函数按 ECMA-402 当作对象读，所以 `new Intl.NumberFormat([function () {}])` 抛 RangeError，v0.26.17 抛的是 TypeError。
- 第一次用 `Intl` 时保留页面在那之前做的改动：页面自己的 `Date.prototype.toLocaleString` 或 `Number.prototype.toLocaleString` 留着，用 `Object.defineProperty` 定义或删掉的 `Intl` 也留着，冻结了 `Date.prototype` 或 `globalThis` 时这次使用也不再抛 `TypeError`。
- 模块的顶层 await 还没完成、又没有任务可跑时（比如 `await new Promise(() => {})`），`EvalBytecode` 返回一个说明这种情况的错误，不再让进程崩溃。模块等的东西之后如果完成了（比如由 Go 完成），之后的 `ExecutePendingJobs` 会跑完模块剩下的部分。它等模块的方式和 `ExecutePendingJobs` 跑任务一样：某个任务抛错不再让它停下，模块会跑到底，调用返回那个任务的错误，模块自己被拒绝时返回模块的错误。模块第一个 `await` 之后碰上超时、中断、内存耗尽或 Go 函数 panic，现在立即结束调用，以前会打印到 stderr，然后进程崩溃。
- 在 Windows 上，`Date.now()` 和 `new Date()` 到 2038 年不再溢出。

### 性能

都是不开 PGO，和 v0.26.17 比。

- VM 留下编译好的正则字节码，每次渲染都新建同样正则的代码只编译一次：tifa 示例商店里高亮 CSS 的那篇文章渲染耗时为原来的 0.893，其他页面不变。商店页面的 VM 多占 12 KiB，渲染过那篇 CSS 文章的 VM 留着约 714 KiB 的正则。
- `Date.now()`、`new Date()` 和 `performance.now()` 从 Go 读时间，不再走 modernc.org/libc 的系统调用：分别从 689 ns 降到 122 ns、从 870 ns 降到 261 ns、从 686 ns 降到 107 ns。商店页面的渲染时间不变。
- 引擎里标了 likely 和 unlikely 的判断直接按条件跳转，不再先把条件变成整数：耗时为原来的 0.983，tifa 的 bench 测试应用的页面为 0.976 到 0.987。
- 在 linux 上，引擎取用和释放 TLS 帧不再调用 libc，解释器函数除外：渲染一次 tifa 的 bench 页面少调用约 36 万次。在上一条的基础上耗时为 0.998，bench 页面为 0.990。两条合起来是 v0.26.17 的 0.979，bench 页面为 0.967。

## [v0.26.17] - 2026-10-21

### 新增

- `NewBuiltin` 和 `VM.NewBuiltinFunction`：JavaScript 像调用 `Math.max` 这类内置函数那样调用的 Go 函数，用 `Args` 读参数，不做转换。在 Apple M4 上一次把两个数相加的调用约 55 ns，走 `RegisterHostFunc` 约 470 ns。它返回的 `*Error` 按 `Name` 抛成对应的错误。
- `VM.NewArray`：用 Go 里建的值做一个数组，做法和数组字面量一样，页面在 `Array.prototype` 上放的 setter 不会被触发。
- `VM.SetStrip`，配 `StripSource` 和 `StripDebug`：编译函数时不留源码（`toString` 显示 `[native code]`），或者不留行号和列号。
- `VM.RunGC`、`VM.MemorySize` 和 `VM.GCThreshold`：马上跑一次 GC，按内存上限的算法读 VM 占的内存，读下次 GC 的阈值。程序可以在两次调用 JavaScript 之间回收，再给下一次设好阈值，tifa 就在两次渲染之间这样做。

### 变更

- Go 里建的 `*Error` 的 `Error()` 改为 `Name: Message`，和 JavaScript 的 `Error.prototype.toString` 写法一样，以前是空字符串。所以 `RegisterHostFunc` 函数为这种错误抛出的 `TypeError` 现在也带上这条消息。从 JavaScript 拿到的错误仍是引擎原来的文本。
- `Intl` 的格式化器由进程里所有 VM 共用，最多 1024 个，满了先丢最早的。用过 `Intl` 的 VM 少占约 15 KB。
- `Intl` 的函数和内置函数一个样子：`length` 和 `name` 按 ECMA-402 给，没有 `prototype`，不能用 `new` 调用，`toString` 显示 `[native code]`。
- 对继承了 `Intl.NumberFormat` 或 `Intl.DateTimeFormat` 原型的 `o` 调用 `Intl.NumberFormat.call(o)` 或 `Intl.DateTimeFormat.call(o)`，新的格式化器挂在 `o` 上并返回 `o`，和 V8 一样。
- 这个 fork 新建的文件改用 MIT 许可证（`LICENSE-MIT`），其中的源码文件带 SPDX 标识。`lib/` 里有 14 个文件照着引擎的部分代码写，标识是 `MIT AND BSD-3-Clause`。来自上游的代码仍是 BSD-3-Clause 和 QuickJS 的 MIT 许可证，语言数据仍是 Unicode License V3。`NOTICE.md` 列出哪个路径用哪个许可证，以及分发二进制的程序要附上哪些声明。
- README 改成一页简短的落地页，有英文和中文两版。原来的内容挪进了 `docs/`：按领域分的 fork 改动、测试、合入上游、性能、发版，以及代码从哪来、从 modernc.org/quickjs 换过来会遇到什么。

### 修复

- 页面改了 `Array.prototype`、`Object.prototype`、`JSON` 或其他内置对象，只要是在 `Register` 之后改的，都不再影响 `Intl` 的行为。
- `Intl` 按 ECMA-402 的规则转换选项、值和语言列表：该给字符串或数字的地方给了 Symbol 或 BigInt 会抛 `TypeError`，语言传 `null` 会抛 `TypeError`，选项的读取顺序也和 ECMA-402 一致。
- `Intl.NumberFormat` 格式化包装过的 BigInt，或者 `valueOf` 返回一长串数字的对象时，结果是精确的，以前会经过 double 被舍入。
- test262 的 intl402 测试通过 6764 次中的 912 次，以前是 770 次，以前通过的都没有变成失败。

### 性能

都是不开 PGO，和 v0.26.16 比。

- `===`、`!==`、`==` 和 `!=` 的常见组合（`null`、`undefined`、布尔值、对象、symbol、同为 atom 的字符串）由解释器直接给出结果：耗时为原来的 0.982。
- 已经是 atom 的字符串键一步找到：渲染时间少 1.3%。微基准里用这种键读属性少 13%，写属性少 30%。
- 在 linux 上调用一次 JavaScript 函数只取一个 TLS 帧，原来是两个，另有 34 个函数取 TLS 帧的次数变少：每次渲染的帧少了约五分之一，时间少 1.4%。
- 由多个分支组成的正则不再尝试在当前位置不可能匹配的分支：时间少约 1% 到 2.5%。用 react-spring 那个含 148 个颜色名的正则做一次 `replace`，从 44.5 µs 降到 5.9 µs。
- 新函数直接拿到 `length` 和 `name`，不再经过中间的 shape：耗时为原来的 0.982。
- 浮点数的位运算不走慢路径：tifa 的 MUI 测试应用的页面耗时为原来的 0.952 到 0.960。商店页面用得太少，没有收益。
- `for in` 循环跳过没有可枚举属性的原型：大量使用这种循环的页面最多少约 3%。
- `JSON.stringify` 和 `JSON.parse` 把成段的普通字符一次拷完：把一个路由的数据转成带引号的字符串少 41%，但渲染整体的收益不到 1%。
- `Intl`：在一个 AMD EPYC 7B13 核上，`new Intl.NumberFormat('en')` 从 16.5 µs 降到 2.7 µs，带时区的 `DateTimeFormat` 创建从 32.0 µs 降到 8.1 µs，格式化从 5.2 µs 降到 2.6 µs。

## [v0.26.16] - 2026-10-20

### 新增

- `VM.SetShapeCache` 和 `DefaultShapeCache`：普通对象的 shape 在对象没了以后，一个 VM 留多少个。新 VM 留 2048 个，传 0 关掉。`lib` 里有 `JS_SetShapeCache`，不调用就是关的。

### 变更

- 商店的一个 VM 在 GC 后多占 722 KiB（8.03 MiB，原来 7.33），其中 647 KiB 是留下的 shape。留下的 shape 计入内存上限，分配失败时也不会放掉，所以贴着上限跑的脚本可能在以前不会耗尽内存的地方耗尽：给这样的 VM 小一点的 shape 缓存，或者设成 0。
- 一个一个加上别的对象都没有的键的对象，构建时间比关掉 shape 缓存时多约 6%。

### 性能

- 8 个热点函数（包括属性读取、`==` 和把值转成字符串）只在需要 TLS 帧的语句前后取帧。开 PGO 渲染时间少 1.6%，不开少 1.1%，darwin/arm64（Apple M4，开 PGO）少 2.6%。
- 解释器把 46 处 `set_value` 调用展开写在原地：少 1.4%。加上上一条，比 v0.26.15 开 PGO 少 2.8%，不开少 2.9%。
- 解释器在自己的函数体里给中断计数器倒数，不再每圈循环调用一次函数：开 PGO 少 0.6%。
- 每个 runtime 把释放的 16 到 512 字节的小块留在自己的缓存里，每次 GC 时还回去：开 PGO 少 0.9%。
- VM 留下普通对象的 shape，下一个键相同的对象字面量不用再建 shape：键相同的对象字面量构建时间少 30%，不开 PGO 渲染少 3.6%。和上面两条合起来，比三项都没有时少 4.5%。同一个核的两个超线程上跑两个 VM，每秒多渲染 2.9% 的页面。

## [v0.26.15] - 2026-10-19

### 变更

- 引擎自身 panic，或者 Go 函数调用了 `runtime.Goexit`（`t.Fatal` 就会调），之后这个 VM 只能 `Close`：再调用 JavaScript 都会 panic。交给 JavaScript 的 Go 函数 panic 时仍和以前一样被接住。
- 在 linux 以外，每个 VM 留着最多 1 MiB 的 TLS 栈，直到关闭。在 windows 上这一块整个计入提交上限（commit limit）。
- 用旧版本采集的 PGO profile 能匹配上的调用点少了很多：要重新采集。

### 性能

- 引擎函数在每个 return 前释放 TLS 帧，不再用 `defer`，Go 就能内联它们：渲染时间少约 10%，开不开 PGO 都一样。
- 以单个字符或一类字符开头的正则（比如 `/["&<>]/`）从那个字符第一次出现的地方开始匹配：少约 3%。
- `===` 和键名只在少见的路径上取 TLS 帧：开 PGO 少 0.7%，不开少 2%。
- 在 linux 以外，多个 goroutine 上的 VM 不再为 TLS 栈等 libc 分配器的锁：Apple M4 上 10 个 VM 每秒渲染的页面是原来的 1.4 到 1.55 倍。
- 合起来，换上新 PGO profile 后渲染时间是 v0.26.14 的 0.859，不开 PGO 是 0.843。

## [v0.26.14] - 2026-10-19

### 修复

- `NewVM` 建不出 context 时，不再泄漏它已经分配的内存。

### 变更

- 上游改动可能让 `internal/fixlib` 改错生成代码的地方，它都会停下。使用这个模块的程序看不到变化。

## [v0.26.13] - 2026-10-18

### 性能

- 解释器循环少做栈上的工作：linux/amd64 上渲染时间少约 12%，darwin/arm64（Apple M4）上少 17%。它的 Go 栈帧在 amd64 上从 2608 字节降到 896 字节。
- 给普通对象加新属性（对象字面量、展开、类字段和 `JSON.parse` 都会这样做）跳过通用路径：少约 6%。
- 合起来，渲染时间是 v0.26.12 的 0.823。

## [v0.26.12] - 2026-10-18

### 变更

- 在 linux 上，经过绑定函数的递归每层最多多占约 1 KB 的 TLS 块，32 位平台上一个块最多占 1 KB，原来是 256 字节。递归到默认上限的最坏情况在 linux/amd64 上仍是约 20.7 MB。

### 性能

- JavaScript 调用不再分配 Go 内存，也不再拿 libc 分配器的锁：linux/amd64 上渲染时间少约 11%，darwin/arm64 上少 14%，linux 上把字节码载入 VM 少约 11%。多个核上的多个 VM 不再因为这把锁互相拖慢。

## [v0.26.11] - 2026-10-18

### 修复

- 全局对象上已经有同名属性时，`RegisterFunc` 和 `RegisterHostFunc` 不再让进程 abort。函数会替换可配置的属性，属性不可配置、全局对象不可扩展或名字里有 NUL 字节时返回错误。

## [v0.26.10] - 2026-10-18

### 修复

- VM 关闭过程中和 `Close` 之后，在任何 goroutine 上调用 `VM.Interrupt` 都安全，`Close` 之后什么也不做。
- 不是合法 UTF-8 的 Go 字符串变成 JS 字符串时，每个不能开始合法序列的字节换成一个 U+FFFD，和 Go 自己解码一样。引擎以前会把非法字节后面的字节一起吞掉，ASCII 也不例外。

## [v0.26.9] - 2026-10-18

### 修复

- `intl` 在所有系统上都按 ICU 的方式命名夏令时。在 Linux 上，Europe/Dublin 等几个时区的夏季和冬季名称以前是反的。

## [v0.26.8] - 2026-10-18

### 修复

- 模块在 linux 和 freebsd 上又能构建了，v0.26.6 和 v0.26.7 在这两个平台上编译不过。
- 从 JS 字符串得到的 Go 字符串是合法的 UTF-8：孤立的代理项（比如在 emoji 中间截断字符串留下的）变成 U+FFFD，和 Node 的写法一样。

## [v0.26.7] - 2026-10-17

在 linux 和 freebsd 上编译不过。请用 v0.26.8 或更新的版本。

### 新增

- `*Error` 有了 `Uncatchable`、`OutOfMemory` 和 `Interrupted`，说明这次调用是不是因为脚本捕获不了的异常结束的，以及原因。脚本自己抛出的东西，不管名字和消息是什么，都不算捕获不了。

### 修复

- 执行时限改用单调时钟，调系统时钟不再影响它。长到时钟表示不了的时限永远不会触发，以前会立刻触发。
- 取捕获不了的错误时不运行任何页面代码，比如页面换掉的 `Error.prototype.toString`。

## [v0.26.6] - 2026-10-17

在 linux 和 freebsd 上编译不过。请用 v0.26.8 或更新的版本。

### 变更

- 中断和执行超时不能再通过 promise、迭代器或 Go 函数被捕获：调用直接返回它，和没有 promise 的脚本一样。`ExecutePendingJobs` 停在因它失败的那个任务上，其余任务留在队列里。
- 内存耗尽也捕获不了了，调用返回 `*Error{Name: "InternalError", Message: "out of memory"}`。页面代码不能再故意占满内存然后捕获失败。
- JavaScript 调用的 Go 函数 panic 时，不再穿过引擎展开栈。脚本像被中断一样结束，调用 JavaScript 的 Go 代码用同一个值再次 panic。
- 无限递归抛出脚本可以捕获的 `RangeError: Maximum call stack size exceeded`，以前会因 Go 栈溢出让进程退出。新 runtime 的栈上限是 10000 个槽位。

### 修复

- 内存所剩无几时编译脚本、函数体、`eval` 代码或正则，会以内存耗尽错误失败。以前引擎可能崩溃、报告脚本里并不存在的 `SyntaxError`，或者返回被截断的字节码。

## [v0.26.5] - 2026-10-17

### 修复

- 因为内存不够转换不成 Go 的字符串，`Eval`、`Call`、`Object` 的 JSON、`Value.MarshalJSON` 和 Go 函数的参数都返回这个错误，而不是 `""`。
- `toString` 会抛异常的 BigInt 报告这个异常，读取被抛出值的名字和消息后不再留下异常。

## [v0.26.4] - 2026-10-16

### 修复

- `Intl.NumberFormat` 按 ECMA-402 自 2021 年起的规则读小数位：货币配 `maximumFractionDigits: 0`，或者小数位和有效数字一起给，都不再抛 `RangeError`。

## [v0.26.3] - 2026-10-16

### 变更

- 不管进程是什么时区，每个 VM 里脚本的本地时间都是 UTC，也不用为此改动进程其余部分的 `TZ`。`getTimezoneOffset()` 是 0，本地的 getter 和 setter 按 UTC 工作，`toString` 打印 `GMT+0000`。要显示别的时区，给 `Intl` 传 `timeZone`。

## [v0.26.2] - 2026-10-16

### 新增

- `intl` 包能按 Chromium 有的所有语言格式化日期、数字和复数，数据来自 CLDR 48.2，并新增 `Intl.PluralRules`。输出在约 60,000 个表达式上和 Chromium 156 对过。用了它的程序大 1.7 MB。

### 变更

- `intl` 默认用 UTC 而不是宿主机的时区，并在脚本第一次用到 `Intl` 或 `toLocale*` 方法时才安装，安装用的字节码每个进程只编译一次。`Register` 耗时 0.09 ms、占 VM 9 KB 内存，原来是 0.85 ms 和 46 KB。

### 修复

- Go 函数在自己的 VM 上再执行 JavaScript 时，不再重设执行时限，也不再丢掉期间发出的 `Interrupt`。
- `intl` 安装因内存耗尽、栈溢出或中断而中途停下时会回滚，下次使用时重新安装。

## [v0.26.1] - 2026-10-15

### 变更

- 被保留下来的错误在读取 `stack` 之前，会让它栈上那些函数的字节码和所在的 realm 一直存活。

### 性能

- 创建错误时不再格式化 `stack`：第一次读取 `stack` 时才生成文本，结果不变。用 React 开发版服务端渲染一个 100 项的组件，在 darwin/arm64 上从 42.1 ms 降到 11.2 ms。

## [v0.26.0] - 2026-10-14

fork 的第一个版本，起点是 modernc.org/quickjs v0.25.0 和 libquickjs v0.13.1。

### 变更

- **不兼容：** 模块改名为 `github.com/liustack/quickjs`，引擎是它的包 `github.com/liustack/quickjs/lib`，不再依赖 `modernc.org/libquickjs`。需要 Go 1.27。

### 修复

- VM 在 `JSON.parse` 里，或在其他把含非 ASCII 字符的字符串转成 UTF-8 的地方耗尽内存时，不再泄漏那个字符串。
- `VM.Call` 和 `VM.CallValue` 报告求值函数表达式时抛出的异常，不再报 `cannot call a non-function`。

[Unreleased]: https://github.com/liustack/quickjs/compare/v0.30.1...main
[v0.30.1]: https://github.com/liustack/quickjs/compare/v0.30.0...v0.30.1
[v0.30.0]: https://github.com/liustack/quickjs/compare/v0.29.11...v0.30.0
[v0.29.11]: https://github.com/liustack/slowjs/compare/v0.29.10...v0.29.11
[v0.29.10]: https://github.com/liustack/slowjs/compare/v0.29.9...v0.29.10
[v0.29.9]: https://github.com/liustack/slowjs/compare/v0.29.8...v0.29.9
[v0.29.8]: https://github.com/liustack/slowjs/compare/v0.29.7...v0.29.8
[v0.29.7]: https://github.com/liustack/slowjs/compare/v0.29.6...v0.29.7
[v0.29.6]: https://github.com/liustack/slowjs/compare/v0.29.5...v0.29.6
[v0.29.5]: https://github.com/liustack/slowjs/compare/v0.29.4...v0.29.5
[v0.29.4]: https://github.com/liustack/slowjs/compare/v0.29.3...v0.29.4
[v0.29.3]: https://github.com/liustack/slowjs/compare/v0.29.2...v0.29.3
[v0.29.2]: https://github.com/liustack/slowjs/compare/v0.29.1...v0.29.2
[v0.29.1]: https://github.com/liustack/slowjs/compare/v0.29.0...v0.29.1
[v0.29.0]: https://github.com/liustack/slowjs/compare/v0.28.6...v0.29.0
[v0.28.6]: https://github.com/liustack/slowjs/compare/v0.28.5...v0.28.6
[v0.28.5]: https://github.com/liustack/slowjs/compare/v0.28.4...v0.28.5
[v0.28.4]: https://github.com/liustack/slowjs/compare/v0.28.3...v0.28.4
[v0.28.3]: https://github.com/liustack/slowjs/compare/v0.28.2...v0.28.3
[v0.28.2]: https://github.com/liustack/slowjs/compare/v0.28.1...v0.28.2
[v0.28.1]: https://github.com/liustack/slowjs/compare/v0.28.0...v0.28.1
[v0.28.0]: https://github.com/liustack/slowjs/compare/v0.27.3...v0.28.0
[v0.27.3]: https://github.com/liustack/quickjs/compare/v0.27.2...v0.27.3
[v0.27.2]: https://github.com/liustack/quickjs/compare/v0.27.1...v0.27.2
[v0.27.1]: https://github.com/liustack/quickjs/compare/v0.27.0...v0.27.1
[v0.27.0]: https://github.com/liustack/quickjs/compare/v0.26.17...v0.27.0
[v0.26.17]: https://github.com/liustack/quickjs/compare/v0.26.16...v0.26.17
[v0.26.16]: https://github.com/liustack/quickjs/compare/v0.26.15...v0.26.16
[v0.26.15]: https://github.com/liustack/quickjs/compare/v0.26.14...v0.26.15
[v0.26.14]: https://github.com/liustack/quickjs/compare/v0.26.13...v0.26.14
[v0.26.13]: https://github.com/liustack/quickjs/compare/v0.26.12...v0.26.13
[v0.26.12]: https://github.com/liustack/quickjs/compare/v0.26.11...v0.26.12
[v0.26.11]: https://github.com/liustack/quickjs/compare/v0.26.10...v0.26.11
[v0.26.10]: https://github.com/liustack/quickjs/compare/v0.26.9...v0.26.10
[v0.26.9]: https://github.com/liustack/quickjs/compare/v0.26.8...v0.26.9
[v0.26.8]: https://github.com/liustack/quickjs/compare/v0.26.7...v0.26.8
[v0.26.7]: https://github.com/liustack/quickjs/compare/v0.26.6...v0.26.7
[v0.26.6]: https://github.com/liustack/quickjs/compare/v0.26.5...v0.26.6
[v0.26.5]: https://github.com/liustack/quickjs/compare/v0.26.4...v0.26.5
[v0.26.4]: https://github.com/liustack/quickjs/compare/v0.26.3...v0.26.4
[v0.26.3]: https://github.com/liustack/quickjs/compare/v0.26.2...v0.26.3
[v0.26.2]: https://github.com/liustack/quickjs/compare/v0.26.1...v0.26.2
[v0.26.1]: https://github.com/liustack/quickjs/compare/v0.26.0...v0.26.1
[v0.26.0]: https://github.com/liustack/quickjs/tree/v0.26.0
