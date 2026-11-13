---
summary: 'liustack/quickjs 对 QuickJS 的 C 树和 Go API 所做的全部改动的索引，按领域分组：每项改动一行，写明它在哪个版本发布，以及哪份文档说明了它做什么、为什么安全、收益是什么、由哪些测试检查'
read_when:
  - 想知道某项对引擎的改动为什么安全、代价是什么、收益是什么
  - 改动同一段代码之前，想找到相关改动的文档
  - 要新增一项改动，想知道它该放在哪
---

# 改动

[English](README.md) · 简体中文

引擎是 QuickJS 的 2026-06-04 版（提交 `3d5e064`，C 代码），上面加了下面这些改动。下面每份文档讲一个领域。对每项改动，它们说明改动做什么、为什么安全、收益或代价是什么以及怎么测的，还有哪些测试检查它。[upgrading.md](../upgrading.zh-CN.md) 写了怎样接入新的 QuickJS 版本，[CHANGELOG.md](../../CHANGELOG.zh-CN.md) 写了每个版本对用户的改变。

引擎是 `engine/` 里的 C，由生成器编译成 Go 汇编。对它的改动就是对 C 的改动，和生成的文件一起提交（见 [AGENTS.md](../../AGENTS.md)）。到 v0.28 为止的版本，跑的是 ccgo 翻译成 Go 的 QuickJS，liustack/quickjs 的改动由一个现在已经没有的工具写进那份 Go。提到这些版本的条目在这里保留了改动的 C 形式，它们的计时是在那个引擎上测的。[performance.md](../performance.zh-CN.md) 写了 C 引擎的速度。

## C 引擎

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 新引擎的 C 树：小型 libc、host 回调、没有阻塞的 Atomics、按运行时分配的 class ID、用计数深度限制递归 | v0.29.0 | [c-engine.md](c-engine.zh-CN.md) |
| 一次调用用更少的循环轮数设置和释放它的值 | v0.29.1 | [c-engine.md](c-engine.zh-CN.md#一次调用如何设置和释放它的值) |
| 解释器调用 C 函数时不经过 JS_CallInternal | v0.29.4 | [c-engine.md](c-engine.zh-CN.md#解释器如何调用-c-函数) |
| 调用结束时跳过没有引用计数的值 | v0.29.5 | [c-engine.md](c-engine.zh-CN.md#调用结束时跳过没有引用计数的值) |
| 一次调用每轮设置八个局部变量 | v0.29.5 | [c-engine.md](c-engine.zh-CN.md#调用每次设置八个局部变量) |
| 一次做几条指令的 opcode：读局部变量加它的 get_field、对字符串做 switch 的 case、typeof 测试 | v0.29.5 | [c-engine.md](c-engine.zh-CN.md#一次分发做几条指令的操作码) |
| 不会同时存活的局部变量共用栈帧的一个槽 | v0.29.7 | [c-engine.md](c-engine.zh-CN.md#共用槽位的局部变量) |
| JSON.parse 直接从文本读键和普通字符串，一次解析内用键 atom（引擎给属性名和常用字符串的编号） 的缓存 | v0.29.7 | [c-engine.md](c-engine.zh-CN.md#jsonparse-直接从文本读键和普通字符串) |
| 引擎在释放对象时被请求的回收什么也不做 | v0.29.10 | [c-engine.md](c-engine.zh-CN.md#释放对象时被要求回收) |
| 离开进入引擎的调用时发生到一半的 panic 会让 VM 报废，不论是什么 panic，包括低于 0x1000 的地址错误，之后不会再进入那个引擎，连释放一个值也不行 | v0.29.11 | [c-engine.md](c-engine.zh-CN.md#哪些-panic-会让-vm-报废) |
| 每个调用了别的函数的生成函数都有 Go 的栈检查，所以地址错误不再取决于生成器在一串没有栈检查的函数下面留出的空间 | v0.29.11 | [c-engine.md](c-engine.zh-CN.md#没有栈检查的函数) |
| JavaScript 跑在寄存器解释器里：函数在第一次调用时从字节码转换，函数之间的调用留在一个循环里，用分段的寄存器栈，QuickJS 的栈式解释器去掉了 | v0.30.2 | [c-engine.md](c-engine.zh-CN.md#寄存器解释器) |
| 在 windows 上每个生成函数在栈指针下面多留出 12 KiB 空间，这样在带 AMX 的 CPU 上，系统在出错时写下的 CPU 状态仍在栈内 | v0.30.2 | [c-engine.md](c-engine.zh-CN.md#windows-上给故障留的空间) |
| 生成的代码按 Go 承诺的方式，经过 FP 按名字读参数、写结果，不再依赖离 SP 的偏移，那个偏移取决于汇编器怎样排布栈帧。机器码不变 | v0.30.2 | [c-engine.md](c-engine.zh-CN.md#按名字读写参数和结果) |
| `go vet -asmdecl` 在生成的引擎里什么也不报：amd64 的寄存器入口叶子函数写了 NOFRAME，`goasm_room` 放在只属于 windows/amd64 的文件里，它的 Go 声明也在那里。机器码不变 | v0.30.2 | [c-engine.md](c-engine.zh-CN.md#go-vet-检查生成的汇编) |
| 以任何条件跳转结尾的 `switch` 字符串 case 都是一个操作码，一串这样的 case 在一次分发里跑完。字节码版本 8 | v0.30.2 | [c-engine.md](c-engine.zh-CN.md#读局部变量加-get_field以及字符串-switch-的-case) |
| 超过 32 个参数的调用和超过 30 个替换的带标签模板，把前 32 个之后的参数放进数组传，和带展开参数的调用一样 | v0.30.2 | [c-engine.md](c-engine.zh-CN.md#超过-32-个参数的调用用数组传参) |
| 寄存器循环跑不了的函数在编译时就被解析器拒绝：参数和局部变量超过 32,765 个，或者栈很深、catch 偏移又要占超过 48 个临时槽 | v0.30.2 | [c-engine.md](c-engine.zh-CN.md#寄存器循环里一个函数的上限) |
| 由 call、apply、Reflect.apply 或绑定函数调用的 C 函数不经过 JS_CallInternal 运行，JS_CallInternal 内联做调用的检查 | v0.30.2 | [c-engine.md](c-engine.zh-CN.md#由-callapply-或绑定函数调用的-c-函数) |
| 寄存器循环不经调用，直接回答字符串或拼平的 rope 在 int 下标处的 `String.prototype.charCodeAt` | v0.30.2 | [c-engine.md](c-engine.zh-CN.md#寄存器循环直接回答-charcodeat) |
| 闭包对拿到之后再没有代码能改的被捕获变量直接存值，不再存和帧共享的引用。这样的闭包不再让暂停中的 generator 或 async 函数活着 | v0.30.3 | [c-engine.md](c-engine.zh-CN.md#捕获后不再改变的变量) |
| 读取时拒绝 catch 偏移离开原处、或者跳转目标在代码开头之前的字节码 | v0.30.3 | [c-engine.md](c-engine.zh-CN.md#catch-偏移和跳转不离开所在的函数) |
| arm64 的每张跳转表每项占 4 字节，生成器拒绝被 lowering 撑得太窄的表 | v0.30.3 | [c-engine.md](c-engine.zh-CN.md#arm64-的跳转表每项占-4-字节) |
| 一次调用的最后几个参数是局部变量、参数或 `this` 时，由调用自己复制，原来每个前面都有一条 `MOV`（`CALLS`、`CALLSD`） | v0.30.3 | [c-engine.md](c-engine.zh-CN.md#一次调用自己把最后几个局部变量复制进窗口) |
| 键在局部变量或参数里的 `o[k]` 不给键加引用就读 | v0.30.3 | [c-engine.md](c-engine.zh-CN.md#局部变量里的键不加引用就读) |
| 键在局部变量或参数里的 `o[k] = v` 不给键加引用就写 | v0.30.3 | [c-engine.md](c-engine.zh-CN.md#局部变量里的键不加引用就写) |
| `JS_ToBool` 和寄存器循环对局部变量或参数的分支不给值加引用就判断真假 | v0.30.3 | [c-engine.md](c-engine.zh-CN.md#值的真假不加引用就判断) |
| 数组字面量把元素的引用直接搬进数组 | v0.30.3 | [c-engine.md](c-engine.zh-CN.md#数组字面量直接接过元素的引用) |
| 在 arm64 上，有调用的函数不再自己保存和读回 LR，Go 的序言和尾声已经做了这件事：每次调用少两条指令，栈帧不变。amd64 不变 | v0.30.3 | [c-engine.md](c-engine.zh-CN.md#arm64-只保存一次-lr) |
| 寄存器循环解构本 realm 的普通数组时不造迭代器：DSTART 和 DNEXT 直接读元素，只在需要的地方才造 Array Iterator | v0.30.3 | [c-engine.md](c-engine.zh-CN.md#不造迭代器的数组解构) |
| amd64 上每条要求操作数在 16 字节上的 SSE 指令，读的都是由 `PCALIGN $16` 放好的数据、局部变量或全局变量，其余的由生成器和链接后的检查拒绝 | Unreleased | [c-engine.md](c-engine.zh-CN.md#需要-16-字节的-sse-读取保持在-16-上) |
| 经过指针的调用从寄存器取结果，没有 Go 代码调用的生成函数不再把结果写到 Go 栈上 | Unreleased | [c-engine.md](c-engine.zh-CN.md#结果只在寄存器里) |
| `o[k]` 的键是自身就是原子的字符串时，像 GETF 读 `o.name` 那样读取，不走通用路径，也不给原子加引用 | Unreleased | [c-engine.md](c-engine.zh-CN.md#自身就是原子的字符串键按具名属性读取) |
| 字符串保存自己作为键时的原子编号，按键读取和 `hasOwnProperty.call` 读一次就能拿到。每个字符串大 4 字节 | Unreleased | [c-engine.md](c-engine.zh-CN.md#字符串保存自己作为键时的原子) |
| 两个热调用少传参数字：解释器直接调 C 函数和 `js_call_c_hop` 改调 `js_call_c_plain`（5 个字，原来 8 个），引擎自己的 `JS_GetProperty` 改调 `JS_GetPropertyObj`（5 个，原来 7 个）。在 arm64 上，这两个调用都不再在影子栈上用参数块传参 | Unreleased | [c-engine.md](c-engine.zh-CN.md#热调用少传参数字) |

## 生成器

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| `make gen` 用 tifa 页面的性能画像编译引擎，内联固定成不带画像的构建所做的结果 | Unreleased | [generator.md](generator.zh-CN.md#tifa-页面的性能画像) |
| 存寄存器或有调用的寄存器入口叶子函数有 Go 声明，amd64 上 `go vet` 能过 | Unreleased | [generator.md](generator.zh-CN.md#有栈帧的寄存器入口叶子函数有-go-声明) |

## 上游 QuickJS 的提交

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 3d5e064 之后上游 QuickJS 的提交，逐个并入 C 树（Array 的 slice 和 splice、js_strict_eq2 借用的操作数、模板字面量、unicode_script），以及没有并入的那些 | v0.29.0 | [upstream-commits.md](upstream-commits.zh-CN.md) |
| Iterator.concat 遵循 IteratorClose，并在一步失败后结束 | v0.29.8 | [upstream-commits.md](upstream-commits.zh-CN.md#iterator-拼接) |
| 没有初始累加值时归约空迭代器，不关闭它 | v0.29.8 | [upstream-commits.md](upstream-commits.zh-CN.md#空迭代器的归约) |
| 稀疏数组截断先移除下标，再压实属性表 | v0.29.8 | [upstream-commits.md](upstream-commits.zh-CN.md#数组截断) |
| 展开字符串得到它的各个字符 | Unreleased | [upstream-commits.md](upstream-commits.zh-CN.md) |

## Go API

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| Call 报告函数表达式的异常 | v0.26.0 | [go-api.md](go-api.zh-CN.md#call-报告函数表达式的异常) |
| Go 转换不了的字符串作为错误返回 | v0.26.5 | [go-api.md](go-api.zh-CN.md#go-无法转换的字符串作为错误返回) |
| `*Error` 说明一次调用为什么结束 | v0.26.7 | [go-api.md](go-api.zh-CN.md#error-说明调用为什么结束) |
| 孤立代理项得到合法的 UTF-8 | v0.26.8 | [go-api.md](go-api.zh-CN.md#孤立代理项得到合法的-utf-8) |
| Go 传来的非法 UTF-8 每个字节变成一个 U+FFFD | v0.26.10 | [go-api.md](go-api.zh-CN.md#go-来的非法-utf-8-每个字节变成一个-ufffd) |
| RegisterFunc 遇到已有的名字不再中止 | v0.26.11 | [go-api.md](go-api.zh-CN.md#registerfunc-不再因已有的名字而-abort) |
| NewVM 失败时不泄漏任何东西 | v0.26.14 | [go-api.md](go-api.zh-CN.md#newvm-失败时不泄漏) |
| SetShapeCache 和 DefaultShapeCache | v0.26.16 | [go-api.md](go-api.zh-CN.md#setshapecache-和-defaultshapecache) |
| Builtin，作为内置函数调用的 Go 函数 | v0.26.17 | [go-api.md](go-api.zh-CN.md#builtin作为内建函数调用的-go-函数) |
| 在 Go 里创建的 Error 的 Error() | v0.26.17 | [go-api.md](go-api.zh-CN.md#在-go-里造的-error-的-error) |
| NewArray | v0.26.17 | [go-api.md](go-api.zh-CN.md#newarray) |
| SetStrip | v0.26.17 | [go-api.md](go-api.zh-CN.md#setstrip) |
| RunGC、MemorySize 和 GCThreshold | v0.26.17 | [go-api.md](go-api.zh-CN.md#rungcmemorysize-和-gcthreshold) |
| Args.WTF8 | v0.27.0 | [go-api.md](go-api.zh-CN.md#argswtf8) |
| SetRegExpCache 和 DefaultRegExpCache | v0.27.0 | [go-api.md](go-api.zh-CN.md#setregexpcache-和-defaultregexpcache) |
| OnClose | v0.27.0 | [go-api.md](go-api.zh-CN.md#onclose) |
| EvalBytecode 在 Go 里等待模块的顶层 await | v0.27.0 | [go-api.md](go-api.zh-CN.md#evalbytecode-在-go-里等待模块的顶层-await) |
| Builtin 的字符串在 Go 里转换 | v0.27.2 | [go-api.md](go-api.zh-CN.md#builtin-的字符串在-go-里转换) |
| Args.Latin1、Args.UTF16、NewStringLatin1 和 NewStringUTF16 | v0.27.2 | [go-api.md](go-api.zh-CN.md#argslatin1argsutf16newstringlatin1-和-newstringutf16) |
| 模块运行完之后释放它的顶层 | v0.28.4 | [go-api.md](go-api.zh-CN.md#模块运行后释放它的顶层) |
| promise 任务的异步上下文，供 AsyncLocalStorage 用 | v0.28.5 | [go-api.md](go-api.zh-CN.md#promise-任务的异步上下文) |
| RegisterEscapeText，用 C 写的 React DOM 的 HTML 转义 | v0.29.2 | [go-api.md](go-api.zh-CN.md#registerescapetext) |
| RegisterCopyProps，用 C 写的复制 React 元素 props 的循环 | v0.29.3 | [go-api.md](go-api.zh-CN.md#registercopyprops) |
| RegisterStartTag 和 RegisterEndTag，用 C 写的 React DOM 为元素写出的开始标签和结束标签 | v0.29.10 | [go-api.md](go-api.zh-CN.md#registerstarttag-和-registerendtag) |
| 比引擎检查时用的版本更新的 Go 会让构建停止 | v0.29.11 | [go-api.md](go-api.zh-CN.md#比检查过引擎的-go-更新的-go-会让构建停下) |
| 改变汇编器编码引擎方式的构建选项会让构建停止 | v0.29.11 | [go-api.md](go-api.zh-CN.md#会改变汇编器对引擎编码方式的构建选项会让构建停下) |
| 每个接收 Value 的入口都拒绝别的 VM 的 Value | v0.30.2 | [go-api.md](go-api.zh-CN.md#每个入口都拒绝别的-vm-的-value) |
| 值的构造函数会报告内存不足 | v0.30.2 | [go-api.md](go-api.zh-CN.md#值的构造函数会报告内存不足) |
| 转换失败的结果不留下任何东西 | v0.30.2 | [go-api.md](go-api.zh-CN.md#转换失败的结果不留下任何东西) |
| UndefinedValue 作为参数和属性值 | v0.30.2 | [go-api.md](go-api.zh-CN.md#undefinedvalue-作为参数和属性值) |
| 拒绝在对 VM 的调用进行中 Close | v0.30.2 | [go-api.md](go-api.zh-CN.md#拒绝在对-vm-的调用进行中-close) |
| Then 返回 then getter 抛出的异常 | v0.30.2 | [go-api.md](go-api.zh-CN.md#then-返回-then-getter-抛出的异常) |
| Go map 变成只有自有数据属性的对象 | v0.30.2 | [go-api.md](go-api.zh-CN.md#go-map-变成只有自有数据属性的对象) |
| 作为 any 返回的 Value 按 Value 接管 | v0.30.2 | [go-api.md](go-api.zh-CN.md#作为-any-返回的-value-按-value-接管) |
| 结果两次到达的同一个位置只算一个引用 | v0.30.2 | [go-api.md](go-api.zh-CN.md#结果两次到达的同一个位置只算一个引用) |
| 包含自身的结果是一个错误 | v0.30.2 | [go-api.md](go-api.zh-CN.md#包含自身的结果是一个错误) |
| 结果的引用在程序代码运行之前取下 | v0.30.2 | [go-api.md](go-api.zh-CN.md#结果的引用在程序代码运行之前取下) |
| NewArray 在调用的限制处停下 | v0.30.2 | [go-api.md](go-api.zh-CN.md#newarray-在调用的限制处停下) |
| 长字符串和结果的转换在调用的限制处停下 | v0.30.3 | [go-api.md](go-api.zh-CN.md#长字符串和结果的转换在调用的限制处停下) |
| Args.StringLen 就地读字符串的长度 | v0.30.3 | [go-api.md](go-api.zh-CN.md#argsstringlen-就地读字符串的长度) |

## 守得住的限额

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 转换字符串时内存耗尽不泄漏 | v0.26.0 | [uncatchable.md](uncatchable.zh-CN.md#转换字符串时内存不足不再泄漏) |
| 嵌套调用保持外层的截止时间 | v0.26.2 | [uncatchable.md](uncatchable.zh-CN.md#嵌套调用保留外层的截止时间) |
| 中断不能通过 promise 或迭代器被捕获 | v0.26.6 | [uncatchable.md](uncatchable.zh-CN.md#中断无法穿过-promise-或迭代器被捕获) |
| 内存耗尽无法捕获 | v0.26.6 | [uncatchable.md](uncatchable.zh-CN.md#内存不足无法捕获) |
| Go 函数里的中断和内存耗尽 | v0.26.6 | [uncatchable.md](uncatchable.zh-CN.md#go-函数里的中断和内存不足) |
| Go 函数里的 panic | v0.26.6 | [uncatchable.md](uncatchable.zh-CN.md#go-函数里的-panic) |
| 内存所剩不多时编译 | v0.26.6 | [uncatchable.md](uncatchable.zh-CN.md#内存很少时编译) |
| eval 超时按单调时钟走 | v0.26.7 | [uncatchable.md](uncatchable.zh-CN.md#eval-超时用单调时钟) |
| Close 期间和之后调用 Interrupt 是安全的 | v0.26.10 | [uncatchable.md](uncatchable.zh-CN.md#close-期间和之后-interrupt-是安全的) |
| 编译正则计入中断检查 | v0.27.2 | [uncatchable.md](uncatchable.zh-CN.md#编译正则计入中断检查) |
| 解析和构建字符串计入中断检查 | v0.27.2 | [uncatchable.md](uncatchable.zh-CN.md#解析和构建字符串计入中断检查) |
| 每个可能运行 JavaScript 的方法都有eval 超时 | v0.29.5 | [uncatchable.md](uncatchable.zh-CN.md#每个能运行-javascript-的方法都有-eval-超时) |
| 遍历长字符串的循环分步读取中断标志 | v0.29.5 | [uncatchable.md](uncatchable.zh-CN.md#对长字符串的循环按步读中断标志) |
| 被异常关闭的迭代器的 return 方法里的内存耗尽不会被捕获 | v0.29.11 | [uncatchable.md](uncatchable.zh-CN.md#异常关闭迭代器时迭代器-return-方法里的内存不足) |
| 开始标签和结束标签函数在每个长循环里读取中断标志 | v0.30.0 | [uncatchable.md](uncatchable.zh-CN.md#开始和结束标签函数在每个长循环里读中断标志) |
| 很长的 rope（几段字符串连成的树，读的时候才拼成一段） 分步拼平，每步读取中断标志 | v0.30.1 | [uncatchable.md](uncatchable.zh-CN.md#长-rope-分步拼平) |
| repeat 每 65,536 个字符读取一次中断标志 | v0.30.1 | [uncatchable.md](uncatchable.zh-CN.md#repeat-每-65536-个字符读一次中断标志) |
| 编译一个很长的正则时，每个长循环都读取中断标志 | v0.30.1 | [uncatchable.md](uncatchable.zh-CN.md#编译一个长正则时在每个长循环里读中断标志) |
| 正则的类不管有多少字符或字符串，都读取中断标志 | v0.30.1 | [uncatchable.md](uncatchable.zh-CN.md#无论正则的类有多少字符或字符串都读中断标志) |
| 字节码调用在 eval 超时内转换结果 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#字节码调用在-eval-超时内转换结果) |
| 读取异常时遇到的无法捕获的异常结束调用 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#读取异常时遇到的无法捕获的异常结束调用) |
| Intl 读不了的字符串参数会结束调用 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#intl-读不了的字符串参数会结束调用) |
| Intl 在长列表的循环里读中断标志 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#intl-在长列表的循环里读中断标志) |
| fmt 的函数在输出放不下时结束调用 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#fmt-的函数在输出放不下时结束调用) |
| BigInt 的长运算读中断标志 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#bigint-的长运算读中断标志) |
| 把长字符串转成数字时读中断标志 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#把长字符串转成数字时读中断标志) |
| 解析很长的源码时读中断标志 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#解析很长的源码时读中断标志) |
| 比较和复制很长的字符串时读中断标志 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#比较和复制很长的字符串时读中断标志) |
| 改变很大的 ArrayBuffer 的大小时读中断标志 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#改变很大的-arraybuffer-的大小时读中断标志) |
| normalize 和 Map 变大时的内存不足 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#normalize-和-map-变大时的内存不足) |
| 没有调用的循环也会在挂起的无法捕获的异常处结束 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#没有调用的循环也会在挂起的无法捕获的异常处结束) |
| 检查正则的 flags 时不再遍历长字符串 | v0.30.2 | [uncatchable.md](uncatchable.zh-CN.md#检查正则的-flags-时不再遍历长字符串) |
| Intl 和 Temporal 的短参数在读之前先量长度 | v0.30.3 | [uncatchable.md](uncatchable.zh-CN.md#intl-和-temporal-的短参数在读之前先量长度) |
| 日历的运算读中断标志 | v0.30.3 | [uncatchable.md](uncatchable.zh-CN.md#日历的运算读中断标志) |
| 没有内存上限时 fmt 也限制调用并分段写出 | v0.30.3 | [uncatchable.md](uncatchable.zh-CN.md#没有内存上限时-fmt-也限制调用并分段写出) |
| Builtin 切不开的一段文字读中断标志 | v0.30.3 | [uncatchable.md](uncatchable.zh-CN.md#builtin-切不开的一段文字读中断标志) |
| 长文本和 locale 列表分块处理并检查调用 | v0.30.4 | [uncatchable.md](uncatchable.zh-CN.md#长文本和-locale-列表分块处理并检查调用) |
| fmt 的函数分段造出长输出并检查调用 | v0.30.4 | [uncatchable.md](uncatchable.zh-CN.md#fmt-的函数分段造出长输出并检查调用) |
| Go 函数的长数组直接向系统要内存 | v0.30.4 | [uncatchable.md](uncatchable.zh-CN.md#go-函数的长数组直接向系统要内存) |
| VM 留下最后一块内存给下一次调用 | v0.30.4 | [uncatchable.md](uncatchable.zh-CN.md#vm-留下最后一块内存给下一次调用) |
| 展开、rest 和 Object.assign 的复制读中断标志 | Unreleased | [uncatchable.md](uncatchable.zh-CN.md#展开rest-和-objectassign-的复制读中断标志) |
| 有内存上限的 VM 也留下最后一块 | Unreleased | [uncatchable.md](uncatchable.zh-CN.md#有内存上限的-vm-也留下最后一块) |

## 错误和调用栈

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 错误的 stack 在第一次读取时才格式化 | v0.26.1 | [errors-and-stacks.md](errors-and-stacks.zh-CN.md#错误的-stack-在第一次读取时才格式化) |
| 无限递归抛出 RangeError | v0.26.6 | [errors-and-stacks.md](errors-and-stacks.zh-CN.md#无限递归抛出-rangeerror) |
| 带展开参数的 new 会说出被调用的不是构造函数 | v0.30.2 | [errors-and-stacks.md](errors-and-stacks.zh-CN.md#带展开参数的-new-会说出被调用的不是构造函数) |
| 通过 call 和绑定函数的调用不在调用栈里多出一行 | v0.30.2 | [errors-and-stacks.md](errors-and-stacks.zh-CN.md#通过-call-和绑定函数的调用不在调用栈里多出一行) |

## 时间和 Intl

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 覆盖 Chromium 全部语言的 Intl | v0.26.2 | [time-and-intl.md](time-and-intl.zh-CN.md#覆盖-chromium-全部语言的-intl) |
| 每个 VM 里本地时间都是 UTC | v0.26.3 | [time-and-intl.md](time-and-intl.zh-CN.md#每个-vm-里本地时间都是-utc) |
| 小数位数按 ECMA-402 自 2021 年起的读法处理 | v0.26.4 | [time-and-intl.md](time-and-intl.zh-CN.md#小数位数按-ecma-402-自-2021-年起的读法处理) |
| 夏令时的名称和 ICU 一致 | v0.26.9 | [time-and-intl.md](time-and-intl.zh-CN.md#夏令时的名称和-icu-一致) |
| Intl 通过 Builtin 调用 Go | v0.26.17 | [time-and-intl.md](time-and-intl.zh-CN.md#intl-通过-builtin-调用-go) |
| 编译好的 Intl 格式化计划和共享的已解码数据 | v0.27.0 | [time-and-intl.md](time-and-intl.zh-CN.md#编译好的-intl-格式化计划) |
| 所有 Intl 格式化器共用 go-intl | v0.27.0 | [time-and-intl.md](time-and-intl.zh-CN.md#所有-intl-格式化器共用-go-intl) |
| 基于 go-intl 的 Intl 构造函数 | v0.27.0 | [time-and-intl.md](time-and-intl.zh-CN.md#基于-go-intl-的-intl-构造函数) |
| Intl 不动页面放在替身位置上的东西 | v0.27.0 | [time-and-intl.md](time-and-intl.zh-CN.md#intl-不动页面放在替身位置上的东西) |
| DateTimeFormat 接受 Temporal 值并格式化范围 | v0.27.0 | [time-and-intl.md](time-and-intl.zh-CN.md#datetimeformat-接受-temporal-值) |
| Temporal 的 duration 使用 DurationFormat | v0.27.0 | [time-and-intl.md](time-and-intl.zh-CN.md#temporal-的-duration-使用-durationformat) |
| 引擎读 Go 的时钟 | v0.27.0 | [time-and-intl.md](time-and-intl.zh-CN.md#引擎读-go-的时钟) |
| Date、Intl 和 Temporal 每个 VM 共用一个墙上时钟，用 `VM.SetClock` 设置 | v0.28.2 | [time-and-intl.md](time-and-intl.zh-CN.md#每个-vm-一个墙上时钟) |
| Intl.Segmenter 边读边找分段 | v0.30.2 | [time-and-intl.md](time-and-intl.zh-CN.md#intlsegmenter-边读边找分段) |
| Temporal 格式的缓存只保留短请求 | v0.30.2 | [time-and-intl.md](time-and-intl.zh-CN.md#temporal-格式的缓存只保留短请求) |
| 数字选项先检查范围再向下取整 | v0.30.2 | [time-and-intl.md](time-and-intl.zh-CN.md#数字选项先检查范围再向下取整) |
| 长的 locale 标签按长度的线性时间解析 | v0.30.2 | [time-and-intl.md](time-and-intl.zh-CN.md#长的-locale-标签按长度的线性时间解析) |
| 数字字符串按 Number 的方式读 | v0.30.3 | [time-and-intl.md](time-and-intl.zh-CN.md#数字字符串按-number-的方式读) |

## Temporal

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 基于 go-intl 的 Temporal | v0.27.0 | [temporal.md](temporal.zh-CN.md#基于-go-intl-的-temporal) |

## URL

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 解析在 Go 里的 URL 和 URLSearchParams，守 VM 的限额 | v0.30.3 | [url.md](url.zh-CN.md#解析在-go-里的-url-和-urlsearchparams) |

## 正则表达式

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 跳到正则可能匹配的位置 | v0.26.15 | [regexp.md](regexp.zh-CN.md#跳到正则可能匹配的位置) |
| 不会匹配的分支不入栈 | v0.26.17 | [regexp.md](regexp.zh-CN.md#不会匹配的分支不入栈) |
| 正则从哪里开始找在编译时算一次，用首字符表，并先比较字面量前缀。字节码版本 9 | v0.30.2 | [regexp.md](regexp.zh-CN.md#正则从哪里开始找编译时算一次) |
| 以字面量开头的一长串分支直接跳到以这个字符开头的那些。字节码版本 10 | v0.30.2 | [regexp.md](regexp.zh-CN.md#以字面量开头的一长串分支) |
| 按 VM 保存编译好的正则 | v0.27.0 | [regexp.md](regexp.zh-CN.md#按-vm-保存编译好的正则) |
| 带 v 标志的正则的类按规范解析 | v0.28.3 | [regexp.md](regexp.zh-CN.md#带-v-标志的正则的类) |

## 字符串

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 搜索直接跳到字符串所在的位置 | v0.27.3 | [strings.md](strings.zh-CN.md#搜索直接跳到字符串所在的位置) |
| symbol 有自己的字符串 | v0.30.2 | [strings.md](strings.zh-CN.md#symbol-有自己的字符串) |
| atom 的哈希表两个桶只放一个 atom | Unreleased | [strings.md](strings.zh-CN.md#atom-的哈希表两个桶只放一个-atom) |
| `+` 左边是空字符串时直接给出右边的字符串，不再复制 | Unreleased | [strings.md](strings.zh-CN.md#-左边是空字符串时直接给出右边的字符串) |

## JSON

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| JSON 成段复制普通字符 | v0.26.17 | [json.md](json.zh-CN.md#json-成段复制普通字符) |

## 运算符

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 浮点数的位运算跳过慢路径 | v0.26.17 | [operators.md](operators.zh-CN.md#浮点数的位运算跳过慢路径) |
| 常见的比较在解释器里直接回答 | v0.26.17 | [operators.md](operators.zh-CN.md#常见的比较在解释器里直接回答) |

## 对象和 shape

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 普通对象的新属性跳过通用路径 | v0.26.13 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#普通对象的新属性跳过通用路径) |
| 保留 shape（引擎记录一个对象有哪些键、按什么顺序的结构） | v0.26.16 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#保留-shape) |
| for in 跳过没有可枚举属性的原型 | v0.26.17 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#for-in-跳过没有可枚举属性的原型) |
| 本来就是 atom 的字符串键 | v0.26.17 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#本来就是-atom-的字符串键) |
| 新函数一次得到 length 和 name | v0.26.17 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#新函数一次得到-length-和-name) |
| 字符串、数字和数组的方法在解释器里查找 | v0.27.1 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#字符串数字和数组的方法在解释器里查找) |
| 自有键从 shape 列出 | v0.28.5 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#自有键从-shape-列出) |
| 普通对象占 48 字节，不是 72 | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#普通对象占-48-字节不是-72) |
| 对象字面量直接按最终的 shape 创建 | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#对象字面量直接按最终的-shape-创建) |
| for in 循环持有它的对象的 shape | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#for-in-循环持有它的对象的-shape) |
| hasOwnProperty.call 在解释器里直接回答 | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#hasownpropertycall-在解释器里直接回答) |
| 展开、rest 和 Object.assign 在轮到每个键时才查找它 | v0.29.2 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#展开rest-和-objectassign-在轮到每个键时才查找它) |
| 对象字面量把属性放在对象自己的内存块里 | v0.29.4 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#对象字面量把属性放在对象自己的内存块里) |
| 键相同的对象字面量共用一个 shape | v0.29.5 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#键相同的对象字面量共用一个-shape) |
| 因内存不足无法压实属性的 delete 不留下待处理的错误 | v0.29.5 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#对象字面量把属性放在对象自己的内存块里) |
| 复制不了的共享 shape 让调用失败 | v0.29.9 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#复制不了的共享-shape-让调用失败) |
| 内置类的对象找到它们原型的空 shape | v0.29.10 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#内置类的对象找到它们原型的空-shape) |
| 通过 shape 由哪个 shape 长出来来找到它 | v0.29.10 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#通过-shape-由哪个-shape-长出来来找到它) |
| 单独死去的对象立刻释放 | v0.29.10 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#单独死去的对象立刻释放) |
| defineProperty trap 拒绝写入的 proxy 在严格模式代码里抛错 | v0.30.2 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#拒绝写入的-proxy-在严格模式代码里抛错) |
| 对象字面量的 shape 至少有空对象的空间 | v0.30.2 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#对象字面量的-shape-至少有空对象的空间) |
| for in 循环对一个 shape 只问一次能不能从它列出键 | v0.30.2 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#for-in-循环对一个-shape-只问一次能不能从它列出键) |
| 往数组里追加元素时，第一次就留 8 个元素的空间 | v0.30.3 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#往数组里追加元素时第一次就留-8-个元素的空间) |
| `in` 运算符遍历原型链时不加引用 | v0.30.3 | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#in-运算符遍历原型链时不加引用) |
| 对象的 rest 用解析器生成的列表排除模式里的键，并按源对象 shape 的缓存直接建成最终的 shape | Unreleased | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#对象的-rest-直接按最终的-shape-创建) |
| 对象字面量里的展开遇到见过的 shape 时，按目标和源对象 shape 的缓存复制 | Unreleased | [objects-and-shapes.md](objects-and-shapes.zh-CN.md#展开按目标和源对象-shape-的缓存复制) |

## 内存

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 每个 VM 共用一份源码文本和行表，SharedBytecode | v0.28.1 | [memory-and-allocation.md](memory-and-allocation.zh-CN.md#所有-vm-共用一份源码文本和行表) |
| 共享字节码只在第一次加载时扫描轮询点 | v0.29.1 | [memory-and-allocation.md](memory-and-allocation.zh-CN.md#共享字节码只在第一次加载时扫描轮询点) |
| 内置函数要的大块内存会超过内存上限时先回收 | Unreleased | [memory-and-allocation.md](memory-and-allocation.zh-CN.md#大块内存在会超过内存上限时先回收) |

## 年轻代回收

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 年轻代回收，VM.RunYoungGC | v0.27.3 | [young-collection.md](young-collection.zh-CN.md) |

## 弱集合

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| WeakMap 的 ephemeron 回收 | v0.27.2 | [weak-collections.md](weak-collections.zh-CN.md) |
| WeakRef 把目标留到任务结束，调用回到 Go 时宿主结束任务 | v0.30.2 | [weak-collections.md](weak-collections.zh-CN.md#weakref-把目标留到任务结束) |

## 引擎里去掉的东西

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| quickjs-libc.c 只保留 Go API 会调用的部分 | v0.27.0 | [trimmed.md](trimmed.zh-CN.md#没有-shell-库) |

## 新增一项改动

- 把它放进所属领域的文档，没有合适的领域就在这里新建一份文档，并在该领域的表里加一行，版本写 "Unreleased"。发布步骤（[releasing.md](../releasing.zh-CN.md#发布前)）会把这里各表以及各领域文档的 Release 行里的 "Unreleased" 换成版本号。永远不要写进 README.md，它只负责链接到这里。
- 写明它做什么、为什么安全、收益或代价是什么（连同机器）以及怎么测的，还有哪些测试检查它。
- 把它加进 [CHANGELOG.md](../../CHANGELOG.zh-CN.md) 和 [CHANGELOG.zh-CN.md](../../CHANGELOG.zh-CN.md)，为速度做的改动还要加进 [performance.md](../performance.zh-CN.md)。
