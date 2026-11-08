---
summary: 'engine/ 里 C 引擎相对上游 QuickJS 和它取代的 ccgo 引擎的改动：小 libc，通过宿主回调取时间和 abort，不阻塞的 Atomics，按运行时计数的类 ID，写进缓冲区的 JS_DumpMemoryUsage 报告，用计数的深度而不是栈来限制递归，C 出错时会怎样，哪些生成的函数不做 Go 的栈检查，生成的代码如何让 Go 把它停下来，Interrupt 如何停住脚本，解释器和正则匹配器落在各自行里的位置，一次调用如何设置和释放它的值，解释器如何不经 JS_CallInternal 调用 C 函数，生成器如何让 LLVM 用两次 8 字节移动复制 JSValue，Go 宿主调用的不带可变参数的 C API，对变量不断追加的字符串如何增长，编译器去掉哪些暂时性死区检查，哪些局部变量共用帧里的一个槽位，JSON.parse 如何直接从文本读键和普通字符串，哪些检查和测试守着这些改动，影子栈有多大，以及系统拒绝给内存时会怎样'
read_when:
  - 修改 engine/libc、engine/host.h，或者 C 树向宿主要的东西
  - 修改 C 树数调用深度或内部深度的位置，或者给它加一处递归
  - 想知道为什么 C 树里 Atomics.wait 会抛错，或者为什么 engine/test 里的 test262.conf 排除了一些测试
  - 在 C 树里调用 JS_NewClassID、JS_DumpMemoryUsage 或 abort，或者写它们的 Go 一侧
  - 想知道 C 引擎取代 ccgo 引擎后，用户这边变了什么
  - 想知道 C 里的野指针、陷阱指令或 abort 对进程和 VM 做了什么，或者修改宿主进入引擎的方式
  - 修改生成的代码在哪里轮询、C 在哪里读中断标志，或者 Interrupt 如何停住脚本
  - 选择影子栈的大小，或者想知道带大栈帧的深递归把栈用满时会怎样
  - 想知道系统拒绝给 VM 内存时，一次渲染会怎样
  - 修改 JSON.parse、它的键缓存或 json_plain_run，或者想知道为什么 JSON.parse 的键不用 StringBuffer
  - 想知道引擎最多能用多少 Go 栈，或者超过 Go 的上限时会怎样
  - 写宿主函数的 Go 一侧、Go 里造的字符串或从 Go 抛出的错误，或者就地读引擎的值（JS_NewHostFunction、JS_NewStringChars、JS_ThrowErrorKind、js_host_layout、JS_NewEscapeTextFunction、JS_NewCopyPropsFunction、JS_NewStartTagFunction、JS_NewEndTagFunction）
  - 修改 C 树里 `+` 和 `+=` 构建字符串的方式，或者想知道为什么字符串末尾后面留有空位
  - 修改生成器放置函数体的位置，或者读到 C 引擎的速度变化可能只是解释器落点不同造成的
  - 修改生成器传给 quickjs.c 的标志，或者想知道为什么 JSValue 用两次 8 字节而不是一次 16 字节来复制
  - 修改 JS_CallInternal 如何设置和释放一次调用的局部变量、操作数和参数
  - 修改编译器读写 let、const 和 class 局部变量的方式，或者它们的暂时性死区检查（tdz_checks）
  - 修改一个函数的哪些局部变量共用帧里的一个槽位（share_local_slots、js_frame_plain）
---

# C 引擎

[English](c-engine.md) · 简体中文

`engine/` 是本模块的引擎：钉住的提交上的 QuickJS 加上 liustack/quickjs 的改动，由生成器编译成 Go 汇编。下面的表列出它和上游 QuickJS 有哪些不同，从上游和 quickjs-ng 取来哪些修复，以及 v0.28 或更早版本的用户相对他们当时跑的引擎（ccgo 翻译成 Go 的 QuickJS，下文叫“旧引擎”）会看到什么。更新记录里有一份给用户看的同样的清单。

## 与上游 QuickJS 的差异

| 改动 | 程序会看到什么变化 | 由什么检查 |
| --- | --- | --- |
| 引擎用 `-ffreestanding -nostdinc` 对着 `engine/libc` 构建 | 在 linux/amd64 上 `Math.*` 的结果和旧引擎逐位相同。在 darwin/arm64 上末位可能不同（见“小libc”） | `check-symbols.sh`、`libc-test.sh`、`math.sh` |
| `Date` 和 `Math.random` 的种子来自 `host_now` | Go 宿主用 VM 的时钟回答 | 原生宿主，`HOST_CLOCK` |
| `abort` 是对宿主的一个请求：带 `HOST_CALL_ABORT` 的 `host_call` | Go 宿主 panic，VM 之后只能关闭。陷阱指令会变成信号，让整个进程结束 | `api-test.c` |
| `Atomics.wait` 抛错，`Atomics.notify` 不唤醒任何人 | `Atomics.wait` 像浏览器主线程一样抛 `TypeError`。引擎里没有线程 | test262，用 `engine/test/test262.conf` |
| `Atomics.pause` 不向 CPU 发提示 | 它检查参数并返回 `undefined`。规范只要求平台能发提示时引擎才发，而引擎里没有内联汇编 | test262 的 `built-ins/Atomics/pause`，生成器（没有桩函数） |
| 格式串要么是字符串常量，要么是某个 `printf` 类函数的格式串，并且和参数一致 | 对程序没有变化。`snprintf` 会 abort 的新格式串造不出来 | 构建（`-Werror=format-nonliteral`），生成器，`libc-test.c` |
| `JS_NewClassID(rt, &id)` 按运行时计数 | Go 一侧按 VM 保存自己的类 ID | `api-test.c` |
| `JS_DumpMemoryUsage(buf, size, ...)` 写进缓冲区 | 没有 `FILE`。Go 一侧拿到一个字符串 | `api-test.c` |
| 打印用的工具没有了：`JS_Dump*`、`dump_token` 之类 | C 里的 `DUMP_*` 选项构建不了 | 构建 |
| `JS_PrintValue` 及其选项只在有 `CONFIG_PRINT_VALUE` 时才有 | 生成的引擎没有值打印器。要用它打印值的宿主要自己定义 `CONFIG_PRINT_VALUE`，原生测试宿主就是这样做的，`DUMP_BYTECODE` 也要它 | 构建，`cycles.sh` |
| `JSClassExoticMethods` 没有 `get_prototype` | 宿主的类不能给自己的对象指定原型。只有 proxy 能，`JS_GetPrototype` 按名字调用它 | 构建，test262 |
| 属性名从不原地增长 | `s += x` 在没有别的引用时，会原地增长局部变量里的字符串。属性名（atom，引擎给属性名和常用字符串的编号）也是这样的字符串，但名字表持有它时不计引用，所以一个局部变量如果握着某个名字的最后一个其他引用，就会改掉这个名字的文本，之后用同样文本造的键就成了另一个属性。现在这样的字符串会被复制 | `unit-white`（`test_string_append_atom`） |
| 对变量不断追加而成的字符串只复制少数几次，不是每次追加都复制 | 对局部变量、参数或闭包变量做 `x += y` 和 `x = x + y` 时，如果变量握着唯一的另一个引用，64 个字符及以上的字符串会被复制一份，并多留出相当于一半长度的空位，之后的追加就填这块空位（见“用追加方式构建的字符串”）。程序读到的内容不变。这样的字符串在释放前最多多占一半长度的内存 | `unit-white`（`test_string_append_flat`），`unit-oom`（`test_string_append_oom`），`compare/string_append.js` |

## 取自上游和 quickjs-ng 的修复

| 改动 | 程序会看到什么变化 | 由什么检查 |
| --- | --- | --- |
| 模板字面量把每个代入值立刻转成字符串 | `${c} ${c.n++}`，以及被后面的代入值改掉的 `Date`，现在打印 Node 打印的结果，被改过的 `String.prototype.concat` 也不再影响模板。上游 QuickJS `f8199dc`，见 [upstream-commits.zh-CN.md](upstream-commits.zh-CN.md) | `compare/template_literal.js` |
| 负的 BigInt 和负的浮点数比较正确 | `-0.5 < -1n` 过去是 true。上游 QuickJS 和旧引擎有同样的 bug | `compare/number_compare.js` |
| 编译 `\p{Script=...}` 时内存不足，调用以内存不足错误结束 | 过去进程崩溃。上游 QuickJS `817509a` | `unit-oom`（`test_regexp_script_oom`） |
| `JSON.stringify` 会释放那个索引无法转成字符串的数组元素 | 索引转换时内存不足。该元素连同它的 shape（引擎记录一个对象有哪些键、按什么顺序的结构） 和键的 atom 一直留到运行时被释放，释放时发现还有块在用。上游 QuickJS 有同样的代码。quickjs-ng `293ba08` | `unit-white`（`test_json_stringify_oom`，以及 `test_json_key_cache`，它会对每次失败的解析所产生的东西做 stringify） |
| `JSON.stringify` 在读取作为缩进参数的 rope（几段字符串连成的树，读的时候才拼成一段） 之前先把它拼平 | 一长串拼接得到的是 rope，它不是 `JSString`，其前 10 个字符过去是从 rope 自己的字段字节里读的：缩进是乱码，里面有指针的字节。包着 rope 的 `String` 对象是对的。上游 QuickJS 有同样的代码。quickjs-ng `35e195c` | `compare/json_stringify_space.js` |
| 数组替换函数的某个名字复制不了时，`JSON.stringify` 以内存不足错误失败 | 它把名字复制到自己的列表里，没看添加一个名字的返回值：没有这个名字也继续往下走，返回的字符串缺少这个属性，错误还挂在那里没处理。上游 QuickJS 和 quickjs-ng 有同样的代码 | `unit-white`（`test_json_stringify_list_oom`） |

## 从 v0.28 或更早版本迁移

和旧引擎相比，除了上面这些行，程序还会看到：

| 改动 | 程序会看到什么变化 | 由什么检查 |
| --- | --- | --- |
| 递归由计数的深度限制，不由栈限制：默认 5000 次调用，最多 10000，C 递归 2000 层 | 旧引擎数 10000 个 TLS 栈槽，所以递归超过 5000 次调用的脚本现在会更早得到 `RangeError`。`JS_SetMaxCallDepth` 取代 `JS_SetMaxStackSize`。以前会崩溃的绑定函数链、`finally` 和迭代器辅助函数现在会抛错，超过 2000 个模块的链（imports、`export * from`、`export { x } from`）也一样。括号嵌套 665 层。`JS_GetCallDepth` 和 `JS_SetCallDepth`（以及内部深度的那一对）让宿主把深度从一个运行时交给另一个 | `unit.c`，test262 |
| 内存不足时，惰性属性保持惰性 | 内置函数或对象（`JSON.parse`、`Reflect`）或函数的 `prototype` 在第一次读它的属性时才创建。创建它占内部深度的一层，到了上限，这次读会抛 `RangeError`，属性也仍保持惰性。旧引擎在创建时内存不足，会让它在这个上下文的整个生命期里都是 `undefined`，所以之后的调用会抛可以捕获的 `TypeError`，而不是内存不足错误。现在下次读会重新创建。为全局对象创建的值如果引用没建成，以及经原型上这样的属性写入的值，也会丢失，释放运行时时还会因此 abort | `unit-oom`（`test_autoinit_oom`） |
| 给代码要用的变量添加条目时内存不足，编译失败 | 编译器在函数的代码（或其中某个函数的代码）第一次用到 `this`、`new.target`、`arguments`、函数表达式的名字、home object 和派生类构造函数的 active function 时，会为它们添加变量，直接 eval 能用的变量也是这样添加。旧引擎添加失败时，把这个函数当作没有这个变量，再往外层找：名字变成了脚本的变量或全局变量，内存不足错误还挂着没处理，所以代码能跑起来却读到错的值，箭头函数里读 `arguments` 或函数名时，则在下标 -1 处读写变量，也就是数组之外。现在编译以内存不足错误失败 | `unit-white`（`test_added_vars_oom`） |
| 编译所要的任何一个块被拒绝时，编译都以内存不足错误失败 | 解析器的前瞻（读到右括号为止的 token，用来区分箭头函数或解构赋值与表达式）把遇到的错误丢掉了，内存不足也一样：编译照常进行，错误却还挂着，或者猜错。现在它只丢掉语法错误，内存不足或中断会让编译失败。函数、类的默认构造函数和设置其字段的函数，拿了无法增长的常量池给出的 `-1` 当下标，最后落在一个 assert 上（旧引擎往池子之前写）。解构参数里没名字的参数，以及正则字面量的源码，添加时没有看结果，带标签的模板遇到非法转义时把内存不足当成了转义非法，把它的文本变成 `undefined`。在 `lre_compile` 里，分组名留下的一条消息，在一个失败的类之后被读成语法错误（`n`），写分组名失败也从没人看：正则编译出来却没有名字。现在这些情况都以内存不足错误让编译失败。只有出错路径变了。上游 QuickJS（19d6bd86）和 quickjs-ng（7a23d8e3）有同样的代码 | `unit-white`（`test_compile_one_failure`，在某一时刻让 `js_malloc_rt` 或 `js_realloc_rt` 失败） |
| 造不出来的名字会抛内存不足错误 | 名字（atom）进运行时的 atom 表，表满了就增长一半。旧引擎在表长不动时没有造名字，也没抛任何东西，所以遇到新标识符的编译，以及用字符串造属性键的代码，会在没有挂起异常的情况下失败。现在造名字会抛内存不足错误，少数过去没有这个名字也继续往下走的调用方（编译器的字符串常量、内置函数的别名、函数的文件名、缩短元素下标超过 2^31 - 1 的数组）也就在那里停下 | `unit-oom`（`test_compile_names_oom`、`test_key_names_oom`） |


## 小 libc

`memcpy` 不是 musl 的，是在这里写的（128 字节以内用同一宽度的重叠加载和存储，之后循环每次 128 字节，目标对齐到 16，只用 SSE2 和 NEON 的移动），因为 musl 的 `memcpy` 对引擎构建的字符串来说太慢。见它的 [README](../../engine/libc/README.zh-CN.md)。

整个引擎也带 `-ffp-contract=off` 构建，生成器会拒绝 IR 里出现 `llvm.fmuladd` 或 `llvm.fma` 以及任何 fast-math 标志（`reassoc`、`nnan`、`ninf`、`nsz`、`arcp`、`contract`、`afn`，或 `unsafe-fp-math` 属性），在 arm64 上还拒绝 llc 文本里出现融合指令（`fmadd`、`fmsub`、`fnmadd`、`fnmsub`、`fmla`、`fmls`）。测了什么，没测什么：

- linux/amd64：`engine/test/math.sh` 逐位检查，对 libc 的 33 个函数各取 100000 个输入，对脚本看到的 33 个 `Math` 运算也各取 100000 个，和 `engine/test/math` 里保存的哈希比较。这些哈希最初是从旧引擎记下来的，C 引擎每个结果的位都和它相同。JavaScript 的检查把 NaN 写成 JavaScript 里唯一的那个 NaN，所以不检查 NaN 的符号和负载。libc 的检查会检查，所以只在 x86-64 上跑。
- macOS arm64：`math.sh` 在那里检查 JavaScript 的哈希。libc 的结果除了 NaN 的符号（由处理器决定，arm64 上为正，amd64 上为负）之外都相同，所以那部分在 arm64 上不检查。
- 引擎只在 amd64 和 arm64 上运行，所以两者末位相同是有证据的。


`snprintf` 读完整个转换（从标志到转换字母）并先检查，再为它取任何参数，`*` 也在内。遇到这些情况它调用 `abort`：浮点转换，`%n`，位置参数，`c`、`s`、`p` 或 `%` 上带长度修饰，`%` 上写的任何标志、宽度或精度（`*` 算数，不管它会拿到什么值），在转换中间结束的格式串，以及放不进 int 的宽度或精度。给 `*` 的宽度是 INT_MIN，以及输出长过 INT_MAX，也会 abort，但要等参数取出之后，因为这取决于参数。引擎里新加的格式串不会打印出错的东西。这个 `abort` 是小 libc 的 `abort`，它像其他所有 abort 一样去问宿主，不是陷阱指令，后者在 Go 里会让进程结束。`libc-test.c` 列出会 abort 的格式串，并数出每个 abort 之前已取走的参数个数，`api-test.c` 检查有一个会到达宿主。`engine/` 里剩下的唯一陷阱是 `abort` 里 `host_call` 之后的那一个，给那种会返回的宿主用。

格式串在到达这个 `abort` 之前就被检查了。引擎文件用 `-Werror=format` 和 `-Werror=format-nonliteral` 构建（`engine/test/flags.sh`），所以格式串和参数一致，并且要么是字符串常量，要么是带 `printf` 属性并把它往下传的函数的格式串（`JS_ThrowError2`、`JS_ThrowError` 和 `js_parse_error_v` 有这个属性，`va_list` 形式用 `format(printf, 3, 0)`）。`JS_ThrowTypeErrorAtom` 和 `JS_ThrowSyntaxErrorAtom` 背后的两个辅助函数，传格式串时用 atom 的名字代替参数，clang 跟不上这一点，于是用一个 pragma 只对这两个函数关掉检查。

`check-symbols.sh` 只证明符号这一层。引擎文件加上 libc，除了 `engine/host.h` 的回调和 `engine/heap.h` 的 `host_memory_grow` 之外，不留任何未定义符号。分配器（[allocator.zh-CN.md](../allocator.zh-CN.md)）是引擎文件之一，引擎不调用 malloc：`dbuf` 和 `CharRange` 没有默认分配器，libc 的 `stdlib.h` 也没有声明。它不能证明机器码不用线程局部存储、不含系统调用指令、不用固定地址。这些检查随生成器一起来，不在这之前。

## 时钟、abort 与宿主

`engine/host.h` 声明引擎向宿主要什么。`host_now(vm, kind)` 给出墙上时钟或单调时钟，单位纳秒。`host_call(vm, &this, argc, argv, magic)` 调用宿主的一个函数，小于 0 的 magic 是引擎的请求：`HOST_CALL_ABORT` 对应 `abort()`。this 值按地址传，引擎的请求没有 this，传 NULL：JSValue 按值传是两个字，加上影子指针、`vm`、`argc`、`argv` 和 `magic` 一共 7 个，而生成的代码在 `body` 调用模式下最多用寄存器传 6 个字给回调。`Math.random` 不问时钟：它从 `JS_NewRuntime` 得到的种子开始，同一个运行时的每个上下文都一样。`abort` 不知道自己属于哪个 VM，传 0。`host_memory_grow` 在 `engine/heap.h` 里，随分配器一起来。小于 0 的 `host_call` 有两种属于分配器：`HOST_CALL_MEMORY_RELEASE` 归还一个专用的内存块，`HOST_CALL_MEMORY_REMAP` 把它扩大（linux 上，宿主用 `JS_SetHostRemap` 说明可以时）。它们不跑用户代码，也不回调引擎。`host_poll` 是 Go 在引擎里停住 goroutine 的地方，也是中断到达引擎的地方（见[轮询与中断](#轮询与中断)）。`JS_FreeRuntime` 返回仍在使用的块数，没有泄漏时为 0。如果还有对象被引用着，比如宿主从没释放的值，它不会像上游的 assert 那样 abort：它会在释放剩下的东西之前停下，由宿主整块收回内存。

## 调用深度与 C 递归

引擎从不读栈指针。生成的汇编跑在 Go 栈上，Go 栈会移动和增长，而且引擎把取了地址的局部变量放在自己的影子栈上，所以栈上的一个地址说明不了引擎有多深。它改为计数，运行时里有两个计数，超过任何一个就抛 `RangeError: Maximum call stack size exceeded`。两个计数都从不重置：进入的每一层在每条路径上都会退出，进不去的一层不计数，所以出错路径不会再往下递归。

- **调用深度**数的是正在运行的每次函数调用。`JS_CallInternal` 在把调用交给某个类之前先计数，所以内置函数、绑定函数和 proxy 都像 JavaScript 函数一样占一层。解释器不经 `JS_CallInternal` 直接调用普通 C 函数时，自己数这一层（见[解释器如何调用C函数](#解释器如何调用-c-函数)）。`JS_CallConstructorInternal` 交给某个类的构造函数在那里计数。`JS_IteratorNext2` 的快速路径直接调用内置迭代器的 C 版 `next`，它也占一层：以前一串迭代器辅助函数经它递归，没有其他计数。运行代码的导出函数（`JS_Call`、`JS_CallConstructor`、`JS_CallConstructor2`、`JS_Invoke`、`JS_Eval`、`JS_EvalThis`、`JS_EvalFunction`、`JS_ExecutePendingJob`）为宿主的这次进入占一层，这样，调回引擎的宿主函数即使到下一次调用的路径很长（比如 `JS_Eval` 的解析器里），也会加一层。引擎自己调用的是不计数的版本（`js_call` 之类）。上限默认 5000（`JS_DEFAULT_MAX_CALL_DEPTH`），最多 10000（`JS_MAX_CALL_DEPTH_LIMIT`）。`JS_SetMaxCallDepth(rt, n)` 设置它，`n` 超出这个范围或运行时正在运行代码时返回 -1 且什么也不改。宿主用 `JS_Eval` 启动时，一个普通递归函数离上限差 2 层：一层给这次进入，一层给全局代码。
- **内部深度**数的是不做调用的 C 递归：解析器（`js_parse_unary`、`js_parse_postfix_expr`、`js_parse_assign_expr2`、`js_parse_statement_or_decl`、`js_parse_destructuring_element` 和 `js_parse_function_decl2`，它们切断了解析器的每个环：二元和逻辑运算符是在循环里读的），创建脚本的函数（`js_create_function`，以及读外层函数变量的 `get_closure_var`），`json_parse_value`、`internalize_json_property`、`js_json_to_str`，正则解析器（`re_parse_disjunction` 和 `re_parse_nested_class`，经 `lre_depth_enter`），模块（`js_resolve_module` 处理 import，`js_create_module_function`，命名空间的 `get_exported_names`，导出的 `js_resolve_export1`，链接和求值），`JS_ReadObjectRec` 和 `JS_WriteObjectRec`，`JS_FlattenIntoArray`，proxy 里会继续转到目标对象的方法（除了 call 和 construct，它们按调用计数，以及原型，`JS_GetPrototype` 按名字问 proxy），绑定函数上的 `JS_OrdinaryIsInstanceOf`，`JS_GetFunctionRealm`，创建惰性属性（`JS_AutoInitProperty`，它的值会定义的属性也可能是惰性的：内置对象的表给它设了界，调用图看不出来），以及 String 对象或 arguments 对象的属性定义，它带 `JS_PROP_NO_EXOTIC` 转到普通定义（`js_define_own_property_ordinary`：调用图看不到这个标志）。多数是在原函数外面套一层计数的包装，原函数改名，末尾加 `1`（`js_resolve_export1` 的名字已经带 `1`，它包装的是 `js_resolve_export2`）。上限是 `JS_MAX_INTERNAL_DEPTH`，2000，它是构建时的常量，不像 `JS_SetMaxCallDepth` 那样是个设置：引擎能用多少栈的上界，等于它乘以这些递归里最深的那个所用的栈，而上界需要一个固定的最大值，不管调用深度是多少。宿主不能改它，`JS_SetInternalDepth`（见下）移动的是计数的起点，不是上限。太深的正则抛 `SyntaxError: stack overflow`，和以前一样。

每个运行时各数各的，而一个宿主的几个运行时共用的 Go 栈看得到它们全部。所以在一个运行时的调用里跑另一个运行时的代码的宿主，要把深度交过去，让整条链受同一个上限限制，不是每个运行时各有一个上限。`JS_GetCallDepth(rt)` 和 `JS_SetCallDepth(rt, n)` 读写调用深度，`JS_GetInternalDepth` 和 `JS_SetInternalDepth` 读写内部深度。围绕这次调用，宿主读出发起调用的运行时的两个深度，把另一个运行时的深度设成它们，做调用，再把另一个运行时原来的深度设回去（两个运行时互相调用时，另一个运行时可能已经在链里，它原来的深度就是它自己外层的那些层）。写入接受从 0 到上限（`JS_MAX_CALL_DEPTH_LIMIT`、`JS_MAX_INTERNAL_DEPTH`）的值，其他值返回 -1 且什么也不改。超过该运行时上限的深度（另一个运行时的上限更大）会保留，它里面的下一次调用是 `RangeError`：链比它允许的更深。和 `JS_SetMaxCallDepth` 不同，这些写入在运行时正在运行代码时也能用，宿主正是那时写的，而设过的深度必须在原来运行的代码继续之前设回去，因为每个结束的调用都会从当时的深度上减去一层。内部深度也要交过去：Go 栈上的界把栈上所有东西的内部层数加起来，各数各的运行时会让链里每个运行时都占满一次上限。`unit.c` 让两个互相调用的运行时一直跑到 `RangeError`：交了深度，整条链是一个上限那么长，不交就是两倍。

上限为 2000 时嵌套能有多深，在原生宿主上测得，方法是把一种形式不断嵌套在自己里面，直到下一层抛错（`RangeError`，正则则是 `SyntaxError`）：

| 形式（每个嵌套在自己里面） | 能放下的层数 | 每层计数 |
| --- | --- | --- |
| 括号 `((1))`、调用 `f(f(1))`、数组 `[[]]` | 665、665、666 | 3（一个赋值表达式、一个一元表达式和一个后缀表达式，不管中间有什么运算符） |
| 对象字面量 `({a: {a: 1}})` | 664 | 3 |
| 箭头函数 `() => () => 1` | 998 | 2 |
| 函数表达式 `(function () { return (function () { return 1 }) })` | 249 | 8 |
| 块 `{{}}`，以及 `if (1) if (1) ;` | 2000、1997 | 1 |
| 带标签的块 `L0:{L1:{}}` | 1000 | 2 |
| 嵌套数组的 `JSON.parse` 和 `JSON.stringify` | 2000、2000 | 1 |

JSON 嵌套超过上限（`JSON.parse`、它的 reviver 和 `JSON.stringify`）会抛 `RangeError: JSON nesting too deep`，这样错误指出的是数据，不是某个递归的调用。栈先不够用时，或者别的递归，仍然抛 `RangeError: Maximum call stack size exceeded`。在一台 Mac 上测过的引擎里，V8 能解析任意深度（它的解析器从 v7.6 起没有递归），stringify 约 6200 层，Bellard 的 QuickJS 2026-06-04 解析 6500 层、stringify 4300 层，QuickJS-NG 0.17 解析 8100 层、stringify 4600 层，都以栈溢出错误告终。

真实代码远低于这个上限。在 79525 个文件里（npm 包、esbuild 编译的 JSX、emscripten、wasm2js、peggy、ajv 之类的输出，还有 JSON），最深的解析到 632 层，是 sql.js 的 asm.js 构建，它把一个大 `switch` 的各个 case 写成 290 个互相嵌套的带标签的块。其他文件都没超过 282 层，千分之一的文件到 100 层，中位数文件是 17 层。一个有超过约 1000 个跳转目标的 `switch` 的 C 程序，如果构建成 asm.js，就解析不了。渲染 tifa 示例商店的页面，解析器最深到 110 层，它的 MUI 页面 99 层，基准页面 62 层（带 Intl polyfill），全部 test262 是 256 层。这些页面上其他东西都很小：正则在编译它们的解析器之上再计 49 层，闭包 15 层（`get_closure_var`），创建函数 8 层，JSON 6 层，`JS_GetFunctionRealm` 2 层。这些渲染的调用深度都在 600 以下。

每次计数也检查剩下的栈。`__shadow_remaining()` 给出剩下的字节数：生成器把它变成影子栈剩下的空间，原生测试宿主则给出它的线程栈剩下的空间。进入任何一种深度，都需要剩下 `JS_STACK_MARGIN`（64 KiB），而一次调用的 `alloca`（`JS_CallInternal` 的局部变量，绑定函数的参数）还要在这之上再有它自己的大小，否则抛同样的 `RangeError`。这样，不管深度允许多少，动态 alloca 都不会越过影子栈的末端。

有几处递归不能失败，所以会抛错的计数不适合它们，它们各自是循环，或者有证明得出的界。rope 是用循环读取、哈希、拼平和释放的（`string_buffer_concat_rope` 按顺序复制 rope 里的字符串）。一次拼接如果让 rope 深过 `JS_STRING_ROPE_MAX_DEPTH`（60），就按顺序用二分计数从它的字符串重建，不管长度如何最深 30 层（上游按长度重建，可能留下深 53 层的，还会递归）。带 reviver 的 `JSON.parse` 的解析记录和函数定义是用循环释放的。Unicode 表里的大小写转换、分解和 emoji 序列，以及 `v` 正则的类里 `\q{...}` 的字符串，都是循环。值打印器过去也会递归，现在去掉了（见上面的 `CONFIG_PRINT_VALUE`）。释放一个对象可能释放别的对象：`free_zero_refcount` 在自己的循环里释放运行期间计数变成零的每个对象，它调用的东西都不会开始回收，所以它从不会被进入两次。`js_nest_enter(rt, JS_NEST_FREE, 1)` 给它计数，超过 1 就 abort，这不会发生：宿主的终结函数要求回收时得不到回收（见[释放对象时被要求回收](#释放对象时被要求回收)），引擎自己的终结函数也不开始回收（`cycles.sh`）。

属性层（属性、atom、转换、错误、字符串缓冲区、大整数和回收器）过去在静态调用图里是一个 173 个函数的环，靠数据而不是靠计数来设界。现在它被切断了，`engine/test/cycles.txt` 是空的：`cycles.sh` 找不到没被某个计数切断的环。热路径上没有加任何计数。切法如下：

- **创建错误。**`JS_ThrowError2` 要分配内存，内存不足时它又用 `JS_ThrowError2` 造出内存不足错误。它自己的一个槽位 `JS_NEST_ERROR`（`JS_NEST_ERROR_MAX`，2）给正在创建的错误计数，第三个（只有 bug 才会引出）会被当作内存不足抛出（null，无法捕获），不再递归。上限 2 靠 `internal/goasm/cmd/cycles` 在调用图上检查的两个事实：创建错误不跑 JavaScript，也不读中断标志（`JS_ThrowError2` 到不了 `JS_CallInternal`、`JS_ThrowInterrupted`，也到不了任何读标志的函数），以及回收不抛任何东西（`JS_RunGC` 和 `JS_RunYoungGC` 到不了 `JS_ThrowError2`，`FinalizationRegistry` 的任务是用不会抛错的内存排队的）。所以新错误的消息和栈是用 `add_property` 加上的，栈里的名字用 `js_string_to_cstring` 转成 UTF-8，它只取一个字符串，什么也不转换。
- **数据永远不会造成的调用**，被拆出来，让调用图看不到它们：通用属性路径读写的快速数组元素（`js_get_fast_array_element`、`js_set_fast_array_element`、`JS_GetPropertyValue` 和 `JS_SetPropertyValue` 的快速路径），规范的数字键转成数字再转回来（`js_string_to_number`、`js_number_to_string`），`ToPrimitive` 结果的 ToString（`js_primitive_to_string`），数字转数组长度（`js_number_to_array_length`），以及空字符串（取自它的 atom）。
- **rope** 用循环拼平（见上）。
- **内部深度的两处计数**（见上）：`JS_AutoInitProperty` 和 `js_define_own_property_ordinary`。

检查：`unit.c`（见 [testing.zh-CN.md](../testing.zh-CN.md#c-树和它的原生测试宿主)）和 test262，后者的 58 个已知错误保持不变。

### Go 栈空间

生成的代码跑在调用它的 goroutine 的栈上，而 Go 运行时在这个栈超过 1 GB 的上限（64 位目标的默认值）时，会让整个进程结束，没有可以恢复的 panic。两个计数让引擎远低于这个上限。在它们最大的设置下（调用深度 10000，内部深度 2000），一个 VM 大约用 65 MB 的 Go 栈，这是根据在生成的代码上测得的每层栈用量算出来的：略多于上限的 6%，用默认值（调用深度 5000）时更少。许多 VM 在一个 goroutine 上互相嵌套调用时共用一个栈，而上面说的深度交接把整条链限制在一个上限内，所以要大约 100 个 VM 才能到 1 GB。

这是测量，不是证明。设计出发时用的界（每层调用 5600 字节、每层内部深度 4704 字节，最大设置下是 62.4 MiB）是针对调用还没内联的引擎算的，对现在提交进仓库的代码还没有重新算。证明以及生成器对它的检查，要等换到新引擎之后重做。在那之前，靠的是这个余量（一个 VM 用量的 15 倍）。

## 引擎出错时

C 能在 Go 代码不会出错的地方出错，而 Go 进程对其中大多数情况会结束：读写映射到空地址的地址，会打印“unexpected fault address”然后退出，因为默认只有低于 0x1000 的地址才是 panic，陷阱指令（amd64 上的 `ud2`，arm64 上的 `brk`）则是 SIGILL 或 SIGTRAP，运行时从不让程序恢复它们。生成的引擎受这样的约束：它每种出错的方式要么是宿主能恢复的 panic，要么不会发生。出过错的 VM 就是坏了，和任何一次穿过引擎栈帧的 panic 之后一样：除了 `Close` 它拒绝每一个调用，进程和其他 VM 继续运行。

| C 做了什么 | 宿主看到什么 |
| --- | --- |
| 读写没有映射的地址：野指针，或影子栈上方的保护页 | 指明地址的 `runtime.Error` panic。`internal/gohost` 的 `Enter` 为这个 goroutine 打开 `debug.SetPanicOnFault`，与之配对的 `Leave` 把 goroutine 原来的设置放回去。amd64 上不规范的指针出错时没有地址，也是 panic。低于 0x1000 时，比如通过 NULL 指针，panic 是空指针解引用那种，不指明地址 |
| 除以零（C 里是未定义行为） | amd64 触发除法错误，Go 总是把它变成 panic。arm64 没有这样的故障，结果为 0 |
| 调用 `abort` | 带 `HOST_CALL_ABORT` 的 `host_call` 以 `gohost.ErrAbort` panic |
| 碰到陷阱指令 | 输出里一个也不留。影子栈 pass 把 C 里的 `__builtin_trap`（`abort` 里 `host_call` 之后的那一个）和它自己对动态 `alloca` 越过影子栈末端的检查，换成对 `host_trap(sp)` 的调用。它以 `gohost.ErrTrap` panic |
| 影子栈用完 | 计数先抛 `RangeError`。保护页是它们后面的安全网 |

生成器守着这一点：它的检查拒绝文本中和解码后的代码里出现 `ud0`、`ud1`、`ud2`、`hlt` 和 `int3`，范围包括 llc 的目标文件和 amd64 的链接后二进制，以及 arm64 上的 `brk`、`udf`、`hlt` 和 `dcps`，在文本里和汇编后的目标文件的字里都查，arm64 的链接后检查把链接后的代码逐字与之比对。`debugtrap` 和 `ubsantrap` 会让 pass 停下，寄存器入口的叶子函数里的陷阱也是，因为它没有影子指针可以拿去调用 `host_trap`。

在 windows 上，系统把故障时 CPU 的状态写在栈指针下面，而 Go 在那里留的空间比带 AMX 的 CPU 需要的少（[golang/go#81238](https://github.com/golang/go/issues/81238)）。生成的引擎自己留出这块空间，所以引擎里的故障在那里也是同样的 panic（见 [windows 上给故障留的空间](#windows-上给故障留的空间)）。

没有覆盖的，和用 C 写的程序一样：通过坏指针写入已映射的内存，会改掉那块内存而没人通知。一个永不结束的循环会一直跑，直到宿主中断它（见[轮询与中断](#轮询与中断)）。goroutine 栈超过 1 GB 时运行时会结束进程，计数的深度让引擎碰不到这一点。系统也可能因为内存而杀掉进程。

打开 `SetPanicOnFault` 要调用两次运行时，每次从 Go 进入引擎花 3.5 ns（一次进入加它的 `Leave` 从 2.5 变成 6.0 ns，一个几乎什么也不做的脚本，1.5 µs，多花 10 ns），从引擎内部的回调进入则不花任何东西。速度基准集里的脚本每个跑 0.3 到 0.8 秒，只进入引擎几次，所以在那里看不出来。它放在 `Enter` 里，不放在池子借出 VM 的地方，因为这个设置属于 goroutine，而一个 VM 每次可能借给不同的 goroutine。[testing.zh-CN.md](../testing.zh-CN.md#生成引擎的-go-宿主) 里的 `TestFault*` 在真实引擎上逐个运行这些故障。

### 没有栈检查的函数

每个有调用的生成函数都以 Go 的栈检查开头。唯一没有栈检查的函数（NOSPLIT）是三个寄存器入口的叶子函数（`memcpy`、`memmove`、`memset`），以及 Go 汇编器自己标成 NOSPLIT 的叶子函数：amd64 上 174 个，arm64 上 172 个，每个都是不调用任何东西、没有栈帧的函数。其中一个出故障时，会变成信号处理函数在故障处把一个对 `runtime.sigpanic` 的调用压到 goroutine 栈上，而 sigpanic 的栈检查可能调用 morestack，它在栈底之上需要 amd64 16 字节、arm64 24 字节，这些不够时进程会以“split stack overflow”结束。叶子函数的调用方通过了它的栈检查，这保证至少留下 800 字节，所以余量和任何 Go 程序里的空指针一样。

生成器过去选更多函数不做栈检查，amd64 上 446 个，arm64 上 456 个：凡是地址没被取过、不经指针调用、不在任何调用环上、且它的 NOSPLIT 调用链放得进链接器允许的 792 字节的函数，同时最深的栈帧下面留 64 字节给故障用。`JS_FreeRuntime` 下面最深的那条链恰好留下这 64 字节，这个数是从 Go 1.27.2 的运行时里读出来的，Go 并不承诺它，更新的 Go 可能改掉它而不让任何构建失败。现在生成器一个也不选了（`internal/goasm/gen/nosplit.go` 里的 `MarkNoSplit`），并且在名单上的函数调用了寄存器入口叶子之外的任何东西时失败，或者这样的叶子互相成环、或在入口以下用了 128 字节或更多时也失败。链接后的检查仍然让每个入口的栈检查符合名单，所以 Go 汇编器如果自己另外标记了别的叶子，或者不再标记这些，就会让检查失败。生成的代码就是改动之前的代码，去掉了这些函数的 `NOSPLIT`，有些函数挪到了下一个文件。

代价，在 linux/amd64 上，对用 main 的 053d158e 加同一份名单生成的引擎测得。Callgrind 数 tifa 的 `BenchmarkAppCG` 里一次渲染的指令。同一个引擎的两次构建之间，整个程序会因为年轻代回收的工作而有最多 0.4% 的波动，所以算进去的代价，是丢了 NOSPLIT 的那些函数多执行的部分：每次检查约 3.2 条指令。

| 页面 | 指令 | 每次渲染多出的栈检查 |
| --- | ---: | ---: |
| tifa 的商店，六个工作台页面 | +0.27% | 643,000 |
| 商店的店面页面 | +0.32% | 1,350,000 |
| tifa 的 MUI 测试应用，三个页面 | +0.36% | 616,000 |
| tifa 的基准页面 | +0.29% | 185,000 |

时间，按 [measuring.zh-CN.md](../measuring.zh-CN.md) 的方法：16 种布局，2 轮，delta 1%，cpu 0 且它的兄弟线程空闲，0 表示干扰时重新跑。页面之和，95% 区间：

| 页面 | 各布局的均值 | 结论 | 正式布局 | 结论 |
| --- | --- | --- | --- | --- |
| shop | +0.47% [+0.00%, +0.94%] | 更慢，但不到 delta | +0.61% [-0.36%, +1.60%] | 无定论 |
| store | +0.04% [-0.51%, +0.59%] | 没有超过 delta 的退步 | +1.09% [-0.11%, +2.31%] | 无定论 |
| MUI | +0.63% [+0.22%, +1.04%] | 更慢 | +1.07% [+0.35%, +1.79%] | 更慢 |
| bench | +0.17% [-0.34%, +0.68%] | 没有超过 delta 的退步 | +0.81% [+0.24%, +1.38%] | 更慢 |

所以按各布局的均值，一次渲染多花 0.04% 到 0.63% 的时间，没有哪个应用被证明超过 1%。引擎的代码段大了 14 KB。保留余量更大的选择（320 字节，不是 64）会多花 0.05% 到 0.07% 的指令，但它仍然要靠一个从运行时里读出来的数。

测试：`TestMarkNoSplit` 和 `TestMarkNoSplitErrors`（`internal/goasm/gen`）在一个小引擎上对两种架构检查名单，并拒绝调用统一函数的寄存器入口叶子、互相成环的叶子，以及 128 字节的叶子链。链接后检查的两个用例（`TestCheckCases`）：一个让名单之外的函数没有栈检查，一个让名单上的函数有栈检查（用一个太大、汇编器不会去标记的栈帧）。`TestSampleRuns/nosplit_fault` 检查一个调用链 24 层深的样本的名单，也就是它的两个叶子，并在 goroutine 栈底的每个位置，让最底下的叶子出故障。`TestFaultNoSplit`（`internal/cengine`）在栈的 12 KiB 范围内每隔 8 字节，让 `unicode_to_utf8` 和 `goasm_chunked_memcpy` 下的 `memcpy` 出故障。

### windows 上给故障留的空间

出错时，windows 先把 CPU 的状态存到出错线程的栈上，就在栈指针下面，然后才调用 Go 运行时的处理函数：一个 `CONTEXT`，带着 CPU 的扩展状态（`CONTEXT_EX`，也就是 XSAVE 存的东西），一个 `EXCEPTION_RECORD`，还有 `KiUserExceptionDispatcher` 的栈帧。在引擎里这个栈就是 goroutine 的栈：生成的代码在它上面运行，影子栈只放 C 的栈帧。Go 在 windows 上给系统在每个栈的 guard 下面留 4,096 字节（`stackSystem`），所以在 windows/amd64 上 guard 在栈底以上 5,024 字节。状态有多大取决于 CPU：CPUID 的 0xD 号叶给出系统打开的那些特性的 XSAVE 大小。在带 AMX 的 Intel 上，tile 数据在 AVX-512 的 2.7 KiB 之外再加 8,192 字节，它的配置加 64 字节，`CONTEXT_EX` 占 11.6 到 11.7 KiB，系统一共在栈指针下面写 12,816 字节，这是在 GitHub 的 windows 机器上量的（[golang/go#81238](https://github.com/golang/go/issues/81238)）。在栈底附近，这次写入会越过栈写进堆里。

生成的引擎自己留出这块空间。只在 windows 的汇编里（`#ifdef GOOS_windows`），每个统一函数在 Go 的序言之后紧接着检查：它的栈指针减去 12 KiB（`internal/goasm/gen/emit.go` 的 `RoomBelow`）是否还在 goroutine 的 guard 之上，不在就调用 `goasm_room`。这个函数什么也不做，栈帧是 12,416 字节，它自己的栈检查让运行时把栈加大。NOSPLIT 列表上的函数先载入 g。这样，一个函数体的栈指针下面至少有 12,288 + 5,024 = 17,312 字节，它调用的寄存器入口叶子函数（`memcpy`、`memmove`、`memset`，用不到 128 字节外加返回地址）至少有 17,176 字节。这比 AMX 的 12,816 字节多 4,360 字节，也比 16 KiB 多。

栈移动时这块空间依然在。运行时加大栈的办法是复制它，空间不会变少。它只在 goroutine 在一次调用处为运行时停下的地方缩小栈，从不在汇编中间。在引擎里，这只可能是生成函数的栈检查，它接着运行时会再做一次空间检查，或者是宿主的回调，它在返回引擎之前再留一次空间（`internal/cengine/host.go` 里的 `makeRoom`）。函数返回到调用者时，调用者的栈指针更高，所以调用者至少有它自己那么多的空间。

它管不到的：Go 代码里的错误。宿主或程序的 Go 函数里的一次空指针，下面只有 Go 的 4 KiB，和 windows 上的任何 Go 程序一样，要等 Go 修好 golang/go#81238（正在审的修法留出 `InitializeContext` 报的大小再加 2 KiB）。宿主 Go 代码里的错误是宿主的 bug，就像引擎里的错误是引擎的 bug。故意在 Go 代码里出错的测试在出错之前紧接着调用 `faulttest.Room`，一个栈帧和 `goasm_room` 一样的函数。`TestRegistry` 里在 `ByStack` 内部出错的两个用例做不到这一点，在 windows 上跳过（`faulttest.Unsafe`）。

linux 和 darwin 没有改：它们的代码除了函数的位置以外逐字节相同，所以 tifa 的页面、已知的 test262 错误和调用深度都和原来一样。深度由引擎自己计数，不取决于 Go 的栈。

在 windows 上的代价：每次调用生成的函数多 4 条指令（一条 `LEA`、一条 `CMP`、一次跳转、一个单字节的空操作），NOSPLIT 列表上的 176 个函数为载入 g 再多 2 到 3 条，回调返回时调用一次 `goasm_room`。在 linux/amd64（AMD EPYC 7B13）上打开 windows 的代码（`-asmflags=github.com/liustack/quickjs/internal/cengine=-D=GOOS_windows`）测量，6 轮交替运行，取每轮变化的中位数，括号里是各轮的范围：

| 渲染 | 变化 |
| --- | --- |
| `compare/engines` 的 React SSR | +0.0%（-0.6% 到 +1.4%） |
| tifa 商店的 studio 工作台页 | +1.0%（-0.8% 到 +3.0%） |
| tifa 商店的首页 | +2.4%（+1.1% 到 +2.9%） |
| tifa 商店的一个商品页 | +1.7%（+0.7% 到 +2.7%） |

引擎里的 goroutine 的栈下面多出 12 KiB。运行一个很小脚本的 goroutine 占 32 KiB 栈，原来是 8 KiB，渲染 React SSR 或递归 2,000 层的 goroutine 和原来一样（64 KiB 和 1 MiB），这是在关掉回收器时测 64 个 goroutine 得到的。windows 的生成代码大 36 KB（从 2,662,176 到 2,698,336 字节，+1.4%）。

测试：`TestRoomCheck` 检查 `funcHeader` 给每种函数写的检查，以及 `faulttest.Room` 的栈帧和 `goasm_room` 一样，`TestRoomCheckAssembles` 检查 windows/amd64 的汇编器在一个长函数体之前接受它（Go 1.26.11 和 1.27.4 之前，汇编器遇到跳向 `PCALIGN` 的分支会无限循环，[golang/go#81792](https://github.com/golang/go/issues/81792)，单字节的空操作避开了它）。对 windows 二进制的链接后检查要求每个统一函数都有这个检查，并要求 `goasm_room` 有它的栈帧：`TestPostLinkFormats` 给一个 windows 二进制换上要的空间更少的检查、去掉调用的检查和短了 8 字节的 `goasm_room`，要求每一个都被拒绝。`TestCallbacksMakeRoom`（`internal/cengine`）要求生成代码调用的每个回调最后都留一次空间。[testing.md](../testing.zh-CN.md#生成引擎的-go-宿主) 里的故障测试重新在 windows 上运行。

### 哪些 panic 会让 VM 报废

只要有 panic 或者 `runtime.Goexit` 从一个还没返回的、进入引擎的调用里穿出来，Go API 就把 VM 标为损坏，不管 panic 是什么。Go API 每次进入引擎的调用，在进入前给 VM 的 `inEngine` 加 1，返回后减回去，而 javascript 调用的 Go 函数从 0 开始数自己的调用（`hostcall.go` 里的 `enterGo`）。`entry.go` 的 `leave`（它结束每个运行 javascript 的方法）和 `hostcall.go` 的 `called`（它结束 javascript 调用的每个 Go 函数），在计数没回到 0 时把 VM 标为损坏。`outer` 包装的是在任何运行 javascript 的方法之外发起的调用，它的调用没有返回时就把 VM 标为损坏，`internal/cengine` 的 `run` 也是这样。两次进入引擎之间 Go 代码的 panic，比如参数的 `MarshalJSON`，以及 javascript 调用的 Go 函数的 panic（引擎把它变成无法捕获的错误，在引擎返回之后继续向上），都没有穿过引擎的栈帧，让 VM 保持原样。javascript 调用的 Go 函数里、它进入引擎的调用之外的 Go 代码故障，和该函数的其他 panic 一样。

过去是靠 panic 来判断的：带 `Addr` 方法的值，也就是 `SetPanicOnFault` 给故障的那种。运行时只对 0x1000 及以上的故障才给它。低于这个地址时，`sigpanic`（`runtime/signal_unix.go` 和 `signal_windows.go`）在看 `SetPanicOnFault` 之前就造出空指针解引用的 panic，amd64 的除法错误也没有地址。所以 C 里的 NULL 指针，故障发生在第一页，在调用方恢复了这个 panic 之后，VM 看起来还能用，而引擎停在半途：下一次调用就跑在那个状态上。在 javascript 调用的 Go 函数里，该函数发起的某次调用里这样的故障，变成了该函数 panic 的无法捕获错误，调用该函数的引擎继续往下走，虽然该函数发起的那次调用停在了半途。

停在半途的引擎不会再被进入。每次进入引擎的调用都经过 `toEngine`（`engine.go`），VM 已损坏，或这一层进入引擎的某次调用没有返回（`abandoned`）时，它就像在损坏的 VM 上那样 panic，这时 `jsFreeValue` 和 `jsFreeAtom` 什么也不做。这对 panic 向上传播、`leave` 把 VM 标为损坏之前释放值的那些 defer 成立（`Call`、`CallValue`、`SetProperty`、`Then`、参数和值的转换，以及在 defer 里释放的其他方法），对恢复了自己进入引擎的调用的 panic 并继续往下走的 Go 函数也成立。这些释放过去是在故障、杀死、abort 或陷阱留下的状态上跑的：一个让上下文不可用的故障，会在释放时再次出故障，取代第一个 panic，在停在半途的堆上释放还可能把它改得更糟。这些值握着的东西随 `Close` 归还。被杀死的 VM 仍然回答的那些读取（`killedReads`）只读计数器，不经过 `toEngine`。

代价：每次进入引擎前后对 VM 的一个字段各加一次，再加上 `toEngine` 的检查。`BenchmarkCall` 和 `BenchmarkBuiltinCall`（M4 上每个跑 8 次，darwin/arm64）显示没有变化：每次调用 17 到 32 ns 的那些用例变化小于 1 ns，有升有降。加上 `toEngine` 的检查之后，每个用例也都不比 main 慢，那一轮里快 1 到 3 ns，这在不同次运行之间的噪声范围内。

测试：`TestLowFaultBreaksVM`、`TestHostLowFaultBreaksVM` 和 `TestNestedLowFaultRecoveredBreaksVM`（`broken_test.go`）让引擎的上下文位于地址 0，使它的第一次读就在 0x1000 以下出故障，分别是在 `Eval` 调用里，在 Go 函数从 javascript 发起的调用里，以及在恢复了故障的 Go 函数的 `Eval` 里。每个都检查 panic 不指明地址、VM 已损坏。它们在这次改动之前都会失败。`TestLowFaultInCallFreesNothing` 让故障出现在 `Call` 的某个参数的转换里，`Call` 的 defer 运行时上下文仍在 0，这样任何进入引擎的释放都会再次出故障并留下计数：它检查剩下的是一个半途而废的调用，不是两个。`TestHostLowFaultRecoveredRefusesEngine` 让一个 Go 函数恢复自己那次调用的故障，再次调用引擎，这会像在损坏的 VM 上一样 panic。这两个在引擎拒绝停在半途的 VM 之前都会失败。

### 系统拒绝给内存时

VM 以内存块为单位向系统要内存（`host_memory_grow`），系统拒绝时，需要这块内存的那次调用以无法捕获的内存不足错误结束，和碰到内存上限一样。`TestMapRefusedSweep`、`TestMapRefusedAmongVMs`、`TestReleaseRefused` 和 `TestRemapRefused`（`-tags testhooks`，它们通过 `hostmem.SetFault` 让调用失败）守着这样做的结果：

- 一个要 13 块内存的脚本，从第 n 块起每块都被拒绝，对每个不超过 13 的 n 都以内存不足结束。然后 VM 运行一个小脚本，`Close` 返回 nil 且不留任何映射，下一个 VM 能跑完整个脚本。
- 单独一块被拒绝多半没人察觉：引擎改要小一点的，脚本带着结果完成。
- 4 个 VM 各在自己的 goroutine 上，每次渲染都新建一个 VM，对其中一个 goroutine 拒绝每一块内存 400 ms，只有那个 goroutine 的渲染失败。另外三个一次也没失败，拒绝结束后第一个也恢复渲染。
- 被拒绝的 `release` 调用不会 panic：`ReleaseMemory` 归还得少一些。被拒绝的 `remap` 让调用带着结果或以内存不足结束，从不 panic。

### 影子栈的大小

影子栈是 32 MiB（生成器的 `-shadow-size-log2=25`），在引擎生成时固定。一层调用占约 1.25 KB，加上函数的每个参数、局部变量和操作数槽位各 16 字节，所以引擎计数的每一层占 1.2 到 2.1 KB（中间有原生函数，如 `sort` 或 `map`，会多算一次自己的计数）。`TestShadowStackAtDepth` 和 `TestShadowStackInternalDepth` 测栈到达的最高点，方法是找最高的非零字：

- 深度上限 10000 时，局部变量少的函数用栈的 37% 到 62%。最多的是又一次排序的 `sort` 比较函数（20.7 MB，因为 `sort` 和比较函数各占一层）。有 100 个局部变量或参数的函数用 85%（28 MB）。有 250 个局部变量的函数到 6379 层深，下一个栈帧放不下，就在那里抛 `RangeError`。上限 5000 时用量减半。
- C 的递归（解析器、JSON、正则解析器、`flat`、stringify）在内部深度 2000 层结束，用量不到 3%。一个由嵌套 100000 层的数组组成的字符串占 47%。
- 渲染示例商店的 18 个页面，最多占 0.85 MiB（2.6%）。
- 所以 32 MiB 够容纳深度上限 10000 且最多约 130 个槽位的任何函数，以及深度上限 5000 且约 340 个槽位的函数。更大的栈帧会在栈用完之前以 `RangeError` 结束，保护页从不会被碰到。把栈加到 64 MiB，只会把这两个数移到 340 和 760 个槽位，而且在 linux 和 darwin 上占地址空间，在 windows 上给每个 VM 占已提交内存。

## 按名字读写参数和结果

生成的函数经过 Go 的 FP 伪寄存器按名字读参数、写结果（`MOVQ a0+0(FP), R12`、`MOVQ AX, ret+24(FP)`），名字就是它们在 `engine_decl_<arch>.go` 里的 Go 声明用的名字。这是 Go 承诺的拿到汇编函数参数的方式（[doc/asm.html](https://go.dev/doc/asm)）。amd64 的代码以前在离 SP 固定偏移的地方读它们，这个偏移写死在函数体的字节里：栈帧，加上 Go 保存的基址指针，再加上返回地址，所以一个 `$64-32` 的函数在 80(SP) 读第一个参数。这个偏移是汇编器现在排布栈帧的方式（`cmd/internal/obj/x86/obj6.go`），不是承诺，它一变，每次调用都会读错字。arm64 的代码本来就按名字读写。

改的是 amd64 的 `.s` 文件。每次移动参数或结果都换成一条 Go 指令，不再是它的字节（1,375 个统一函数里共 8,039 条），函数代码之后的数据跟在一行 `// data` 后面。文件的大小上限按每个函数全写成字节时的大小来算，所以每个函数都留在原来的文件里，在二进制里的位置也不变。arm64 的文件不变。

为什么安全：生成的代码还是同样的指令，在同样的位置。lowering 后的文本仍把每条移动写在 Go 现在用的偏移上，所以 clang 像以前一样排它前后的分支，Go 的汇编器按名字编码这条移动时，寄存器和位移都一样。和改动之前的引擎相比，在每个系统上把它构建进 `internal/cengine` 的测试二进制（`-tags goasmkeep`），每个生成的函数都在同一个地址，指令也一样：在 linux/amd64、linux/arm64 和 darwin/arm64 上字节相同，在 darwin/amd64 和 windows/amd64 上除了对挪动过的数据的引用的位移以外字节相同，这些引用指向同一个符号和偏移（Go 代码的行号表变大了，排在它后面的数据随之挪动）。所以速度、tifa 的页面、test262 的 33 个已知错误、调用深度和计时用的布局都和原来一样。链接后的检查像以前一样，要求每条移动落在 Go 的序言在二进制里给参数区留的位置上。

它管不到的：一条移动的长度取决于它的位移，127 以内是一个字节，超过是四个字节。如果 Go 换一种方式排布栈帧，位移在 128 字节附近的移动会变长或变短，挪动跨过它的那些分支，而这些分支是写成字节的：参数还能找到，代码却跑不了。链接后的检查拿 `scripts/go-versions.sh` 跑的每个 Go 版本比对这些字节，`internal/goversion` 会挡住比 1.27 新的 Go。1.26 或 1.27 新出的补丁版本挡不住：它若有这样的改动，要等 `scripts/go-versions.sh` 用它跑时才会发现，程序用它构建时发现不了。引擎发出的调用的参数仍放在 0(SP) 往上，也就是 ABI0 里调用者写它们的地方，和 Go 自己在 doc/asm.html 里的例子一样。

有了名字，`go vet -asmdecl` 不再报 amd64 的那 634 个函数没写结果，`go vet` 能检查生成的文件，靠的就是这一点（[go vet 检查生成的汇编](#go-vet-检查生成的汇编)）。

测试：`TestEngineArgsByName`（`internal/goasm/gen`）解码两种架构每个生成函数的字节，只要有指令从 SP 够过栈帧的局部变量就失败。`TestEmitFPMoves` 检查 emit 写出的移动，并且拒绝访问保存的基址指针或返回地址、访问不是参数或结果的字、宽度不对、或者经过不对的那类寄存器的指令。`TestFPMovesAssembleAsLowered` 为 amd64 的 linux、darwin 和 windows 构建每种栈帧的函数（一个有调用的，一个没有栈帧的 NOSPLIT 叶子函数，一个有栈帧的，一个位移超过 8 位的），并在二进制里找到每个函数体，和 lowering 后的一样。`TestPostLinkCheck` 的两个用例从下一个字加载参数、把结果存进参数字，检查要拒绝这两个。

## go vet 检查生成的汇编

`go vet -asmdecl` 在生成的引擎的汇编里什么也不报，所以每次提交和 CI 里的 `go vet ./...` 都带着它跑，不再加 `-asmdecl=false`。它检查的是：函数用名字访问的每个参数和结果，都和它的 Go 声明里的一样，偏移和大小都对，每个有结果的函数都写了结果，每个有栈帧的函数都有 Go 声明。以前它会报 amd64 的代码，那时代码在离 SP 的偏移上读参数（[按名字读写参数和结果](#按名字读写参数和结果)），还会报四个没有 Go 声明的函数。

生成器给 amd64 写出的东西有两处不同：

- 寄存器入口叶子函数 `memcpy`、`memmove` 和 `memset` 写成 `NOSPLIT|NOFRAME, $0-0`，和 arm64 上已有的写法一样。它们没有栈帧，也不调用别的函数，所以不管写不写，Go 都不给它们写序言，而没有 Go 声明的函数，检查要的就是 NOFRAME。有栈帧的叶子函数，或者调用了别的叶子函数的，会保留 Go 给它保存的 BP，那就得改成给它一个 Go 声明。引擎里现在没有这样的叶子函数，真出现了 `TestEngineAsmDecl` 会发现。
- `goasm_room` 放进 `engine_room_windows_amd64.s`，不再放在 `engine_room_amd64.s` 的 `#ifdef GOOS_windows` 里面，那个文件在每个 amd64 系统上都参与构建。它的 Go 声明在 `room_windows_amd64.go` 里，所以这两个文件名让函数和它的声明在同样的系统上构建。其他文件里的空间检查只在 amd64 文件的 `#ifdef GOOS_windows` 里调用它，也就是只在 windows/amd64 上，它正好在那里。另一种做法是在每个 amd64 系统上都给它 Go 声明，那样在 windows 之外就声明了一个不存在的函数。

为什么安全：机器码不变。拿改动前后构建的 `internal/cengine` 测试程序（`-tags goasmkeep`）比，amd64 的 linux、darwin、windows 和 arm64 的 linux、darwin 上，每个生成函数都在同一个地址，字节相同。linux/amd64 上 `.text`、`.rodata`、`.gopclntab` 和 `.data` 的字节都相同。arm64 的文件没有变。

代价：这项检查多花一两秒。包已经编译好时，在一台 arm64 的 Mac（darwin/arm64）上 `go vet ./...` 不带它是 1.1 秒，带它是 2.1 秒，在一台 linux/amd64 机器的两个核上是 5.4 秒和 7.4 秒，用的是 Go 1.27.2。

测试：`TestEngineAsmDecl`（`internal/goasm/gen`）在 amd64 的 linux、darwin、windows 和 arm64 的 linux、darwin 上对 `internal/cengine` 运行 `go vet -asmdecl`，要求一条报告也没有。`TestRoomCheck` 检查没有栈帧、有栈帧和有调用的叶子函数的 TEXT，并检查 `goasm_room` 的文件只在 windows/amd64 上构建。

## 释放对象时被要求回收

引擎在释放对象期间，也就是 `gc_phase` 不是 `JS_GC_PHASE_NONE` 时，`JS_RunGC` 和 `JS_RunYoungGC` 立刻返回，什么也不做：在释放引用计数降到零的对象的 `free_zero_refcount` 里，在立即释放引用计数降到零的普通对象的 `free_zero_refcount_first` 里（见 [objects-and-shapes.zh-CN.md](objects-and-shapes.zh-CN.md)），在释放一次回收找到的环的 `gc_free_cycles` 里，以及释放运行时的这两处。创建对象时阈值要求的回收也经过 `JS_RunGC`，所以这时创建的对象同样不会开始回收。阈值不动，在释放之后创建下一个对象时会再检查。

宿主在那里跑代码：它用 `JS_NewClass` 创建的类的终结函数，以及 `JS_NewHostFunctionRelease` 的函数的 `host_release`。回收器把状态存在运行时里（引用计数降到零的对象的列表，正在释放的环的列表，阶段），而终结函数里开始的一次回收，会在调用终结函数的那次释放进行到一半时重用这份状态。在 `free_zero_refcount` 里，内层回收第二次进入 `free_zero_refcount`，`JS_NEST_FREE` 用一次 abort 把它拦下，而年轻代回收会先释放列表上的每个对象，正在跑终结函数的那个也在其中。在 `gc_free_cycles` 里，内层回收重新开始了外层正在走的列表，并释放了外层随后又去读的对象：AddressSanitizer 报告读了已释放的内存。引擎自己的终结函数不开始回收（`cycles.sh` 对这两处都检查 `free_zero_refcount`），所以只有用 C 写的宿主，比如原生测试宿主，才能碰到。Go API 碰不到：它不创建类，释放期间唯一会跑的 Go 代码是 `gohost.Release`，它清掉句柄，不运行用户的代码。

上游 QuickJS 和 quickjs-ng 有同样的入口，没有检查。Bellard 修了引擎里到达嵌套回收的一条路径，即用可能抛错的内存排队的 `FinalizationRegistry` 任务（bellard/quickjs#494，`5022f2b`），liustack/quickjs 也用不会抛错的内存排这个任务。CPython 和 Lua 做的和这次改动一样：回收运行期间 `gc.collect()` 返回 0，Lua 在终结函数运行期间停止它的回收步骤和 `lua_gc`。

为什么安全：跳过的回收什么也没释放，它本来要释放的东西仍然是垃圾，下一次回收会释放。阈值不动，在释放之后创建的下一个对象时会再检查，那时堆如果仍然超过阈值，就开始回收。做了这次释放的完整回收在结尾会重新设置阈值，一如既往。在终结函数里调用了 `JS_RunGC` 的宿主得不到回收，可以在释放对象的那个调用返回后再要求。终结函数也可以调用 `JS_ComputeMemoryUsage` 和 `JS_DumpMemoryUsage`，它们读存活对象的列表，而释放不动那个列表。终结函数仍然不能做的，是 QuickJS 的规矩：运行 JavaScript，释放运行时，或在运行时被释放期间创建对象，那会在运行时消失后让这个对象仍留在使用中。

代价：经常运行的路径上没有任何代价。检查在 `JS_RunGC` 和 `JS_RunYoungGC` 里，以及 `js_trigger_gc` 放进 `JS_NewObjectFromShape` 和 `JS_CallInternal_block` 的 `JS_RunGC` 副本里，那里它排在阈值测试之后，所以低于阈值时创建对象不多跑一条指令。生成的引擎只在这四个函数里有差别，`JS_CallInternal_block` 的栈帧不变（amd64 $496，arm64 $504）。用 callgrind 在 tifa 的四个应用（shop、store、mui、bench）上数，一次渲染的指令变化分别是 -0.00%、+0.10%、-0.01% 和 +0.03%，`JS_CallInternal_block` 的是 +0.03% 到 +0.07%，来自它的代码围绕新块重新布局。store 的 +0.10% 大部分是引擎之外的 Go 运行时的回收器。这些应用的 16 个页面产生同样的 HTML。

测试：`unit-white` 的 `test_gc_in_free`，在九种释放里，分别从宿主的终结函数和从 `host_release` 运行 `JS_RunGC`、`JS_RunYoungGC`、堆超过阈值时创建的对象，以及 `JS_ComputeMemoryUsage` 加 `JS_DumpMemoryUsage`。九种释放是：宿主丢掉一个数组的最后一个引用，宿主丢掉每个年轻对象和每个老对象的最后一个引用（一个个地，经 `free_zero_refcount_first`），`JS_RunGC` 里的一个环，`JS_RunYoungGC` 里的一个环，一个老数组，它的最后一个引用被一个年轻环握着（它等年轻代回收结束，在 `free_zero_refcount` 里释放），一个 `WeakMap` 的值，它的键已经死了（`gc_remove_weak_objects`），一个 `WeakMap` 的值，它的键在一个环里（`gc_ephemeron_prune`，然后是 `gc_free_cycles`），以及释放运行时时带着一个挂起的异常和一个经过全局对象的环。`JS_GC_RUN` 在引擎里是空的，在 `white.c` 里它数通过阶段检查的完整回收和年轻代回收。每个用例检查每个对象都被终结、每个函数都只释放一次，释放期间除了该用例自己开始的那次之外没有回收运行，释放没有重新设置阈值时，释放之后的下一个对象会开始阈值所要求的回收，然后运行时能回收、能工作、能释放，不留任何块。每个用例在自己的进程里运行，测试带着该用例作为参数重新启动，这样它开始时没有宿主的锁，也有自己的看门狗。终结函数不能在运行时被释放时创建对象，所以那个用例被去掉了，剩下 35 个。改动之前其中 26 个失败：每个会开始回收的用例。除了 `JS_RunGC` 里的环和键在环里的 `WeakMap` 的值之外，引擎在所有种类的释放里都 abort，那两种情况下普通构建以段错误崩溃，AddressSanitizer 报告在 `gc_free_cycles` 里读写了已释放的内存。`JS_ComputeMemoryUsage` 的九个用例在改动之前也通过。

## 轮询与中断

Go 运行时要停住一个 goroutine（为了回收，或者为了运行另一个），只能在函数检查栈的地方，而它在 Go 汇编的其他任何地方都不会停。一个不调用任何函数的引擎循环，只要它在跑，就会拖住整个进程的每一次回收。C 只在解释器里参与，解释器自己数它的字节码（见下）。

- 计数器是影子栈区域里的一个字，距起点 384 字节（`gen.PollCounterOffset`），代码把影子指针的低位清零就能找到它。每个循环按 LLVM 对它各个块运行频率的估计，把计数放在指令更少的地方：放在寄存器里，进入循环时读，离开时写回，并且在循环里每个可能轮询的调用前后写回再读，或者放在内存里，每次计数时读写。所以一个大多数次运行都调用可能轮询的函数的循环用内存计数，按块运行的循环（见下）也是，它每个块计一次，热代码已经几乎用完寄存器的循环也是。内层循环如果外层循环有寄存器，就用外层循环的寄存器计数。
- 循环里每条往回走的边计一次数，但在解释器里除外。
- 解释器 `JS_CallInternal` 在内存里计 32（`POLL_WEIGHT`）：在它的入口，在每个不向前的跳转处，在每个从 `finally` 块返回处，在每次捕获异常处，以及在每个 `OP_nop` 处。C 标出这些地方（`cutils.h` 里的 `poll_count`），pass 在每个标记处放一次计数，回到分发的路上不放，这样分发保持足够短，llc 能把它复制进每个处理函数的末尾。两次计数之间，解释器在字节码里只往前走。编译器在每条超过 127 条指令的前向路径里放一个 `OP_nop`（`quickjs.c` 里的 `insert_polls`），而每个函数的字节码在能运行之前都要检查，编译器产生的和 `JS_ReadObject` 读入的都一样（`check_polls`）：两次计数之间的路径不长于 128 条指令（`POLL_RUN`），每个操作码都是解释器有的，每个跳转都落在指令的开头，最后一条指令不会跑到末尾之外。`JS_ReadObject` 对其他字节码以 `SyntaxError: invalid bytecode` 失败。`OP_ret` 从栈上取要返回的位置，它只会去 `OP_gosub` 之后的那条指令：`check_polls` 用位图标出每个函数的这些位置，代码的每个字节一位（只对有 `OP_gosub` 的函数做），`OP_ret` 到任何其他数字都抛 `InternalError: invalid ret value`，这样，读入的字节码即使栈上放着它自己编的数字，也不能把解释器带到某条指令中间，或带上检查没走过的路径。所以一次 32 的计数最多代表 4 条字节码指令，也就是和 C 里一样的几百条机器指令，而在默认间隔下，轮询在 40,000 条字节码指令之内到来，分发里每条指令都计数时是在 10,000 之内。在循环里它来得比以前更早：少于 32 条指令的循环，每圈计的数不止一次。一个 Go 测试（`TestInterpreterJumpsCount`）约束 `JS_CallInternal` 做到这一点：`pc` 只能往前走一个固定步长，或经 `JUMP_TO`（它对不向前的移动计数）和 `JUMP_COUNTED`（它总是计数）。
- 当 LLVM 的标量演化能证明一个循环很短时，它不需要自己的计数：最内层，不调用任何东西，每次进入最多 256 条指令。进入它的东西会计数。其他不调用任何东西的最内层循环按这个大小分块运行，每块结束时对整块计数。所以一次计数最多代表几百条指令，间隔限定了两次轮询之间的时间。
- 长度可能超过 256 KiB 的 `memcpy`、`memmove` 和 `memset`，走一些每次做 256 KiB 的辅助函数，每一块计一次数。这些函数没有影子指针，也没有计数。
- pass 自己检查它的输出：每个函数里，去掉计数或轮询的块以及它证明很短的循环的回边之后，不剩任何环。在解释器里，进入它的分发的边也去掉，它的入口必须计数，而每个标记必须紧跟在一次该权重的计数之后。而且每次计数至少取 1，计数为 0 或更小时就轮询：它的分支测试这一点，在走到的那一侧调用 `host_poll`。并且它在每个函数里跟踪计数：每次计数从当前的计数值减，每次写计数器写的是当前的计数值，每个可能轮询的调用和每个返回都在内存里找到它。如果寄存器里的计数被被调用者看不到，或者写掉了被调用者数过的东西，就会让两次轮询之间过去的计数超过间隔。

`host_poll` 把计数器设回 VM 的间隔（`SetPollInterval`，默认 10000）。间隔不改变速度，只改变轮询来得多勤。

`JSRuntime` 里有一个中断标志，宿主用 `JS_GetInterruptFlag` 拿到它的地址。Go 里的 `Interrupt` 记下时间，下一次 `host_poll` 往标志里写 1。只有运行脚本的 goroutine 写这个标志，所以 C 用普通加载读它。原生测试宿主用线程做同样的事：另一个线程要求的停止（`host_interrupt`，或者 120 秒后结束测试的线程）在宿主里等着，为这个宿主构建、带 `JS_HOST_POLL` 的引擎，在每次读标志时（`cutils.h` 里的 `poll_flag`）调用宿主的 `host_poll`，宿主在引擎的线程上写标志。为 Go 构建时，`poll_flag` 是一次普通读，生成的代码和没有它时一样。引擎在能抛错的地方读标志：解释器在调用、跳转和它计数的其他地方，正则的执行和编译，字符串搜索，类型化数组 `indexOf`、`lastIndexOf`、`includes`、`fill` 和 `copyWithin` 的循环，`String.prototype.repeat`，`Array.prototype.fill`（每 4096 个元素）和 `join`（每 1024 个），`String.prototype.normalize`（把字符串变成码点时每 4096 个字符，它的三遍中每一遍，以及构建结果时），`localeCompare`（同样，比较时再加每 4096 个码点），`toUpperCase` 和 `toLowerCase`（每 4096 个字符，以及找大写 sigma 周围单词结尾时），`isWellFormed` 和 `toWellFormed`（每 4096 个字符，以及 `toWellFormed` 复制的 4096 字符一块之间），`split`（它的搜索，和 `indexOf` 一样，以及每 1024 段），用字符串的 `replace` 和 `replaceAll`（它们的搜索，和 `indexOf` 一样，以及每 1024 次匹配），`padStart` 和 `padEnd`（每写 65536 个字符，比这更长的填充物按这么多一步），`trim`、`trimStart` 和 `trimEnd`（每跳过 4096 个字符），`trim`、`slice`、`substring`、`substr` 和 `split` 切出的片段超过 1048576 个字符时（16 位的字符串先扫描有没有超过 0xff 的字符，再按这么多一块复制），`anchor`、`fontcolor`、`fontsize` 和 `link` 的属性（每 1024 个字符），`escape`、`unescape`、`encodeURI`、`encodeURIComponent`、`decodeURI` 和 `decodeURIComponent`（每 1024 个字符），`Array.from`、`TypedArray.from` 和类型化数组的构造函数（从类数组读取时每 1024 个元素），内置迭代器的 `next` 的调用（字符串、数组、Map、Set 或迭代器辅助函数的迭代器：`JS_IteratorNext2` 不经 `JS_CallInternal` 调用它，后者在调用开始时读标志，所以它自己读标志，而 `Object.fromEntries`、展开、`Iterator.prototype.toArray`、`Math.sumPrecise` 和遍历迭代器的 `Set` 的方法，都随它停止），`Set` 的方法对自己的集合做的复制（每 1024 条记录），长字符串或 rope 的 `Map` 或 `Set` 键的哈希（每 65536 个字符，rope 的一段字符串算比它的长度多一：`map_hash_key_poll` 用它查找和添加，而表增长时，已经在表里的键哈希时不读标志），以及把 rope 拼平，或用 `<` 比较两个 rope（每 65536 个字符，rope 的各段字符串算比它们的长度多一），BigInt 的长循环（乘法和除法的每一行，转成字符串时的除法和数字，从字符串转来时的乘法和数字，每 32,768 个 limb 或数字读一次：`js_bigint_poll`），把字符串转成数字或 BigInt（前后的空白、数字的扫描、数字的复制、`js_atod` 读的数字、BigInt 开头的零，以及 JSON 文本里的数字和空白，每 65536 个字符读一次），脚本和 JSON 的解析器（两个记号之间的空白和换行、注释、标识符、字符串、模板、正则，以及看下一个记号，每 65536 个字节读一次，还有长记号的 atom、行号列号和保存的源码），`<` 和不带比较函数的排序对两个长字符串的比较，`Symbol.for` 的键，以及把长字符串复制进字符串缓冲区（每 65536 个字符，按字节复制时每 1 MiB 读一次），在 C 里做匹配的全局 `replace` 的循环，`JSON.parse` 和 `JSON.stringify` 在每个值上，以及排序在每次比较之后。它们的长循环按最多 64Ki 个元素或 1 MiB 一块运行，两块之间读标志。这些读取代价很小。`JS_IteratorNext2` 里的那一次，是内置迭代器每次 `next` 一次加载一次分支，对数组做 `for of` 时每个元素 680 条指令里的 2 条（指令多 0.29%，Map 多 0.44%，数组、Set 和字符串的展开多 0.43%）。tifa 的页面和一次 React 渲染，指令多或少都在 0.15% 以内（callgrind，商店的六个工作台页面渲染 +0.05%，MUI 页面 -0.05%，bench 应用的页面 +0.14%，`r_react_ssr` +0.04%）。用字符串的 `replace` 和 `replaceAll` 用 `js_string_find` 搜索，对 16 个字符的字符串多约 160 条指令，而 `Map` 或 `Set` 对短字符串键的一次操作多 7 到 16 条。类型化数组的搜索一次看 16 个元素，在生成器证明很短的有计数循环里，所以不需要自己的计数。在那里引擎抛 `InternalError: interrupted`，任何 `try` 都无法捕获，宿主在下一次从 Go 进入引擎时清掉标志。宿主转换这个错误时不运行 JavaScript：标志置位时，任何函数调用都会再次抛错。

排序在第一次比较发现标志置位或有异常时停止（`cutils.c` 里的 `rqsort_stop`）：`Array.prototype.sort` 然后什么也不写回，而没有比较函数、原地工作的类型化数组排序，会让元素处于某种顺序。从不读标志的循环，比如 `Array.prototype.slice`、`TypedArray.prototype.set`，会一直跑下去。如果 `Interrupt` 之后的宽限（`SetKillGrace`，默认 50 ms）过了脚本仍在运行，且自从设置标志的那次轮询以来引擎已轮询了 1000 次（`gohost.KillPolls`），那次 `host_poll` 会以 `gohost.ErrKilled` panic，它穿过里面的调用（包括回调以及回调里运行的脚本）向上传播，goroutine 恢复它的 `SetPanicOnFault`。最外层的调用返回一个与 `quickjs.ErrKilled` 匹配（`errors.Is`）的被中断的 `*Error`，而这个 VM 只能 `Close`：返回错误的方法返回一个与 `ErrKilled` 匹配的错误，其他方法 panic，`Value.Free` 什么也不做。轮询次数用来区分仍在运行的脚本和线程没得到运行的脚本：宽限是时钟上的时间，而操作系统或 Go 调度器让它等着的线程，或者脚本调用的 Go 函数，不用引擎做任何工作就会过完它。只看时间的话，一个本来几次轮询就会停下的脚本，只要有这样一次 50 ms 的等待落在 `Interrupt` 和它停下之间就会被杀掉，负载高的机器（3 核的 CI 机器跑两个包的测试，或者 goroutine 比核多的服务器）时不时就会这样。在 Apple M 的性能核心上，读标志的脚本在标志置位后 1 到 13 次轮询内停下，先要释放或回收它所创建的东西的脚本约 600 次内停下（对 500,000 个数字的副本做默认排序是 10 ms），而从不读标志的循环每毫秒轮询 33 到 163 次，所以 1000 次轮询落在它的宽限之内。

只有运行脚本的调用才会被杀死。宿主用 `EnterScript` 为它进入引擎，其他任何事（回收、内存报告或归还内存）用 `Enter`，它的轮询只置标志：在一次长回收期间调用 `Interrupt` 的看门狗，不会弄坏一个没运行任何页面代码的 VM。最外层的入口是什么，决定了它里面的调用。带超时的 `EnterScript` 还在单调时钟上设一个截止时间，过了它的第一次 `host_poll` 的行为，就像 `Interrupt` 在截止时间调用了一样：标志置位，宽限从截止时间开始算。晚于截止时间的 `Interrupt` 不会把它往后推：宽限从两者中较早的那个开始算。它里面某个回调运行的脚本，得到的是剩下的时间。

`host_poll`、`host_trap` 和 abort 只靠影子指针找到它们的 VM。`gohost.New` 把 VM 的 id 和它的补码写在影子栈最前面 8 个字节里，宿主把指针向下取整到栈的大小，读出这个头，检查它指名的 VM 的栈确实在那里：没有锁，也不写别的 VM 要读的内存，所以多个核上轮询的 VM 不会互相拖慢。宿主的临时空间和引擎的栈帧从这个头之上开始。按 id 排列的 VM 在一张原子指针表里（`internal/idtable`），已关闭 VM 的 id 会交给后面的 VM。

在 linux/amd64 上测得（GCP 上的 AMD EPYC 7B13，Go 1.27），每个脚本固定在一个核上，对比用 `-poll=false` 构建的同一个引擎：

- 回收。当一个紧凑循环在运行时（解释器的 `for` 循环、类型化数组的搜索、`fill` 和 `copyWithin`、正则、字符串搜索、`repeat`、类型化数组的排序、react），回收最多等引擎停下 0.33 ms，没有轮询时最多要等 1.3 秒。分配很多的脚本（对 2Mi 个数字的 `JSON.parse`，带比较函数的排序）偶尔等到 2.6 ms，第 99 百分位在 0.25 ms 以下。解释器的直线代码，3000 条语句、没有循环的函数，轮询可能要 40,000 条字节码指令才来，让回收最多等 0.1 ms。
- 中断，`GOMAXPROCS=1`，脚本运行 100 ms 后：`for` 循环、正则、类型化数组搜索、`fill`、`copyWithin`、react 和解释器的直线代码，在 0.3 ms 内停下（直线代码在 0.15 ms 内），杀死发生在宽限结束后 0.2 ms 内。引擎在不读标志的地方干活时，停得更晚：引擎自己堆的一次回收（分配错误可能触发它，堆里数组的每个元素约 18 ns：两个 1Mi 个数字的数组是 19 ms，所以对 1Mi 个数字排序在 8 到 19 ms 后停下），或者一个长循环，比如 `Array.prototype.slice`（每个元素约 40 ns）。
- 速度，核心的另一个超线程空闲，取 8 个代码位置上的中位数：react 和渲染脚本慢 5%，fib 慢 4%，`for` 循环慢 14%，对 2Mi 个数字的 `JSON.parse` 慢 17%，先构建文本时慢 12%，类型化数组的排序慢 10%，字符串搜索慢 8%，正则在 2% 以内。类型化数组 `indexOf` 慢 25%，`repeat` 最多慢 9%，`fill` 和 `copyWithin` 在 1% 以内。如果现在按块运行的那些循环每次运行都计数，react 会慢 6%，正则慢 20%。

## 解释器和正则匹配器的位置

在 Go 的前导之后，每个生成的函数都有一个 `PCALIGN $16`，所以它的函数体从 16 字节处开始，并且只要它之前链接的代码增长，就在它的 64 字节行里移动。`JS_CallInternal` 的函数体从行内 40 字节处开始，`lre_exec_backtrack` 的从 16 处开始，这是测得最快的位置（解释器在 JSValue 改成两次 8 字节移动复制之前，在 0 处，见[下面](#jsvalue-如何复制)）。空操作每次调用执行一次：解释器 66 字节，匹配器 69 字节。链接后的检查让每个函数体守在它的位置，它前面的填充只能是空操作，还有一个测试构建出偏离位置 8 字节的函数体，看检查拒绝它们。

函数代码之后保留的常量池和跳转表，由 `.p2align` 对齐，它从函数的开头算起。行内 40 字节处开始的函数体，起点比 16 的倍数多 8，所以解释器的常量池过去在它运行的地方偏离 16 有 8 字节。没有指令用需要 16 字节对齐的方式读它（用常量池作内存操作数的 `paddd` 或 `movdqa`，在偏离 16 的池上会出故障），所以什么也没坏。LLVM 会这样做的构建（一个 profile-guided 的构建在解释器里产生了 19 处这样的读取）在让函数体偏 8 字节的那些位置上以 SIGSEGV 死掉。现在生成器把一个函数在它 lowering（把 LLVM 的中间表示换成目标机器的指令） 后的目标文件里，放到它的函数体在二进制里占据的位置（解释器用 `.p2align 6` 和 `.skip 40`，即 `FuncPlace`），这样每个常量池和每张表的对齐在二进制里都成立。对 `JS_CallInternal`，这把它的常量池在函数体里挪了 8 字节：唯一变化的生成文件是装着它的那个，变在跳转表的偏移和 8 字节的填充上。两种架构上链接后的检查，都要求每个常量池和每张表在二进制里位于其对齐的整数倍处，而 `TestSampleRuns` 运行一些函数，它们在行内 8、16 和 40 字节处开始的函数体里，用 `paddd` 和 `pxor` 读自己的常量池（`testdata/run/vecpool.c`）。`xform` 的 `front:` 位置里不是 16 的倍数的那些（8 和 24）会把函数体和常量池一起挪动，所以解释器用这样的指令读常量池的构建，不能在这些位置上运行。要为这两个位置生成，用 `-align64 JS_CallInternal:48` 或 `:0`，这样常量池保持在 16 字节上。

引擎别处的改动现在只会按整行移动这些函数体。它仍会多少改变它们的速度，因为别的代码和数据在移动：数字，以及测量的代码位置如何配合固定的位置，见 [performance.zh-CN.md](../performance.zh-CN.md#代码位置)。

解释器用上游的 computed goto 做分发，就是 C 树构建它的方式。ccgo 引擎的一个早期补丁在原生构建里把它关掉了，只关它一项，V8-v7 得分就降到未打补丁 C 的 0.91（gcc 15.3）和 0.88（clang 22.1.8）（[数字](../history.zh-CN.md#原生-c-上的补丁开销)）。要保持开着。

## 一次调用如何设置和释放它的值

`JS_CallInternal` 在调用开始时把字节码函数的每个局部变量设为 `undefined`，在返回时释放局部变量和留在栈上的操作数。`OP_call` 和 `OP_call_method` 在调用返回后释放函数、`this` 和参数。这些循环每圈处理一个值。在生成的代码里，设置一个局部变量的一圈要 9 条指令：LLVM 在每次存储后都重新读 `b->var_count`，因为存储一个 JSValue 可能改变它，而轮询 pass 会把计数不知道的循环切成带自己计数的块。释放一个值的一圈约 10 条。tifa 示例商店六个工作台页面的一次平均渲染，进入 `JS_CallInternal` 113,866 次，其中 67,076 次运行字节码，设置 520,962 个局部变量，在调用结束时释放 581,838 个值：这两个循环占它指令的约 6.5%。

现在局部变量四个一圈地设置，然后是剩下的两个和一个，计数只读一次，而末尾的值则在至少还剩两个时两个一圈地释放（`sp - pval >= 2`：尾调用让栈是满的，这时 `pval + 1 < sp` 会指向帧之外，C 不允许）。调用在遍历参数的循环之前，自己先释放函数和 `this`。每次渲染，callgrind 数出六个工作台页面是 167.0 M 条指令，而不是 171.4 M（少 2.5%），`r_react_ssr` 少 3.1%，`r_bench_table` 少 4.8%。`q_int_arith` 不变。没有局部变量的函数 `q_func_call` 多跑 3.0%，每次调用约 13 条指令：入口把 `b` 留在栈上，并为剩下的两个和一个做测试。只读一次计数而不按四个一圈，React 少 1% 的指令，但在算术循环上多花 3.9%，所以没保留。时间在 [performance.zh-CN.md](../performance.zh-CN.md#调用for-in-和-hasownpropertycall)。

把局部变量的空间向上取整到四个值，这样一圈不需要余数处理，这个做法被放弃了：每个栈帧多 48 字节，会让带大栈帧的深递归在影子栈上更早结束（见[影子栈的大小](#影子栈的大小)）。

## 解释器如何调用 C 函数

`OP_call`、`OP_call_method` 以及它们的尾调用形式，过去把每次调用都交给 `JS_CallInternal`，后者按类找到 C 函数，经类表交给 `js_call_c_function`。对 C 函数来说，这个入口自己什么也没做，却仍然跑完整个前导：amd64 上 105 条指令（`r_react_ssr` 一次运行里有 452,219 次这样的调用，占 `JS_CallInternal` 入口的 28%），包括 Go 的栈检查、它位置的空操作、参数块和共享的返回。

现在解释器自己调用普通的 C 函数（`js_call_c_direct`）：类为 `JS_CLASS_C_FUNCTION` 的函数，调用深度低于上限，且剩下的栈够 `JS_CallInternal` 为它检查的量。绑定函数、带数据的函数（`JS_CLASS_C_FUNCTION_DATA`）、proxy、构造调用，以及靠近任一上限的调用，仍和以前一样经过 `JS_CallInternal`，所有来自 C 的调用（`JS_Call`、`js_call`）也一样。直接调用按同样的顺序做 `JS_CallInternal` 的入口对 C 函数做的事：向下一次轮询计入 `POLL_WEIGHT`（`poll_count`，所以一次调用仍只计一次），读中断标志，有无法捕获的异常挂起时立即返回，计入一层调用深度，不带标志调用 `js_call_c_function`，然后退出这一层。两条路径之前都设置 `sf->cur_pc`，而 `js_call_c_function` 建出错误栈所显示的那个帧并切换 realm，和以前一样。`js_call_c_function` 从不内联，这样它的栈帧不会并入解释器的栈帧。

需要小心的是栈。`JS_CallInternal` 在检查之前就把它的帧放在影子栈上，所以旧的调用是在占了这个帧之后检查的，C 函数以及它调用的所有东西，都是在少了这么多栈的情况下运行的：amd64 上 1472 字节。没有它的话，一个经 C 函数、带大栈帧的递归在栈用完之前能走得更深（经 `f.call`、有 300 个局部变量的递归能到 4999 层，原来在 4168 层结束），这违反了脚本递归深度和以前一样的规矩。所以运行时在创建它的第一个上下文时测出这个帧（`js_measure_call_frame` 带 `JS_CALL_FLAG_FRAME` 调用 `JS_CallInternal`，它只写它发现剩下的栈），`rt->stack_debt` 保存正在运行的直接调用的这个量之和。栈的每一次检查（`js_stack_short`）都把这个和当作已占用的。测量放在入口的计数之后、检查之前，所以宿主可以在调用深度已到上限、已请求中断或栈剩得很少时，像以前一样创建第一个上下文。它位于对象不是函数的那个分支里，那里先测类型再做检查（`js_call_checks`，两个分支里相同），所以函数调用不为此付任何代价。用 callgrind 数（amd64，tifa d90d1fe），`JS_CallInternal` 本身在一次 `r_react_ssr` 的渲染里比测量被挪走之前少执行 86 万条指令，在商店工作台的一次渲染里少 30 万条，而在入口顶部测试标志的那个版本，让它多执行 350 万条。`TestShadowStackAtDepth` 的每个递归，以及 98 个经 `call`、`apply`、`map`、`sort` 和 `Math.max`、带 1 到 300 个局部变量、上限为 5000 和 10000 的递归，都在和以前相同的深度结束，错误也出在相同的位置，在 linux/amd64 和 darwin/arm64 上都是。这笔债务在每次检查时多花一条指令。

债务属于一个运行时。原生 C 宿主在同一个线程上从一个运行时调用另一个运行时时，会交接调用深度（`JS_SetCallDepth`）但不交接债务，所以每个运行时只把自己的直接调用对着线程的栈来算，在两者之间来回跳的递归，在栈检查把它停下之前，可能比以前走得稍深一些。Go 宿主没有这种情况：每个 VM 都跑在自己的影子栈上。

`JS_SetDirectCCalls(ctx, 0)` 让运行时重新经 `JS_CallInternal` 调用每个 C 函数（它的帧永远放不下），供测试使用。`TestCCallDepth` 运行经 `call`、`apply`、`map`、`sort`、`Math.max`、不带接收者调用的 `abs`，以及尾调用 `return [1].map(f)` 和 `return apply(f, null, [])` 的递归，局部变量 0 到 300 个，上限为 5000 和 10000，直接调用开和关，要求深度相同，`RangeError` 的栈也相同。没有这笔债务时其中 13 个不同。`TestCCallDepthAfterExits` 让嵌套的直接调用以 `RangeError`、被捕获的 `Error` 和中断各离开一次，都在同一个 VM 里，下一个递归和第一个的结束方式一样。`unit-white` 的 `test_tail_c_calls` 检查这些 return 编译成 `OP_tail_call` 和 `OP_tail_call_method`，而 `unit` 的 `test_first_context_any_state` 在深度上限处、已请求中断的情况下创建第一个上下文。

每次渲染的指令数，callgrind 在位置 0.0，对比 main（a9d61185）：tifa 商店的六个工作台页面 -0.74%，它的三个 MUI 页面 -0.34%，它的两个基准页面 -2.32%，`r_react_ssr` -0.87%。没有这笔债务时，这次改动给出的是 -0.84%、-0.60%、-2.66% 和 -1.11%。时间在 [performance.zh-CN.md](../performance.zh-CN.md#解释器调用的-c-函数)。

`JS_CallInternal` 的 arm64 构建随着这些调用而变大，直到 llc 留下的一条 `tbnz` 在 lowering 加入代码之后落到了它 32 KiB 的跳转范围之外，汇编器拒绝了它。现在生成器让 llc 在 arm64 上只取这个范围的一半给 `tbz` 和 `tbnz`（`-aarch64-tbz-offset-bits=13`），这样更远的跳转会变成跨过一条 `b` 的 `tbz`，lowering 也就有空间让函数翻倍。amd64 的构建不变。

检查：`TestCCallDepth`、`TestCCallDepthAfterExits`、`TestShadowStackAtDepth`、`TestHasOwnCallDepth`、`Interrupt` 和错误栈的测试，带 `unit` 和 `unit-white` 的 `compare.sh`，以及 58 个已知错误的 test262。

## 调用结束时跳过没有引用计数的值

字节码函数的一次调用结束时，会释放它的局部变量和留在栈上的操作数（见[上面](#一次调用如何设置和释放它的值)）。它们多数没有引用计数可还：没有代码设置过的局部变量是 `undefined`，其余的，数字、布尔值和 `null` 与字符串、对象一样多。这个循环仍然一对一对地看它们，每对 15 条指令。它在循环体里调用一个函数，所以 LLVM 把指针、计数和轮询计数器在调用前后保存在栈上，每圈开头都读回来，没有释放任何东西的圈也一样。一个有 200 个局部变量、一个也没设置的函数，调用时比没有局部变量的函数多 2,314 条指令：每个局部变量 11.6 条，其中约 7.5 条是释放。

带引用计数的值，标签是负数，其余所有标签都不是（`JS_TAG_FIRST` 是 -9，一个 `_Static_assert` 守着其他值的标签是 0 或更大）。所以八个值的标签按位或在一起，只有当八个里有一个需要释放时才是负数。调用结束时先数它的值（`i = sp - local_buf`），然后用一个没有调用的循环每八个一组地过一遍，这样生成的代码把变量保存在寄存器里。里面有带计数的值的一组八个，被当作两组四个来看，而有这种值的那组四个，按从第一个到最后一个的顺序逐个用 `JS_FreeValue` 释放，和以前的循环一样，所以释放什么、释放几次、按什么顺序，都没有变。最多剩下七个，按 `i` 的各个位用四个、两个、一个一组来释放。没有任何东西指到帧之外：循环测的是剩余的计数，不是指针（栈是满的时，`sp` 是帧的末尾）。

一个有 200 个局部变量、一个也没设置的函数，现在比没有局部变量的函数多 1,466 条指令（每个局部变量 7.3 条）。找到的最慢的情形，是每第四个局部变量握着一个对象的函数，每组四个里都有一个要释放的值：200 个局部变量时它比以前多 2.4% 的指令（每次调用 5,530 对 5,403），32 个局部变量时多 1.5%。每次渲染，callgrind 在位置 0.0 数出这些指令（应用页面是 4 次渲染减 1 次的差，脚本是 3 次减 1 次）：

| 页面 | 之前 | 之后 | |
| --- | ---: | ---: | ---: |
| tifa 的商店，六个工作台页面（合计） | 934.3 M | 924.3 M | -1.07% |
| tifa 的商店，五个店面页面（合计） | 1,765.0 M | 1,752.5 M | -0.71% |
| tifa 的 MUI 测试应用，三个页面（合计） | 719.1 M | 713.7 M | -0.76% |
| tifa 的 bench 应用，table 和 spiral（合计） | 297.5 M | 291.3 M | -2.10% |
| `r_react_ssr` | 2,512.4 M | 2,479.9 M | -1.29% |
| `r_bench_table` | 3,346.7 M | 3,278.4 M | -2.04% |
| `r_bench_spiral` | 3,220.0 M | 3,178.4 M | -1.29% |

时间在 [performance.zh-CN.md](../performance.zh-CN.md#调用的结尾和开头)。测试是 `TestFrameValuesFreed`（`internal/cengine`：局部变量数从 0 到 70 的每个数量的函数，再加几个别的，一个局部变量在每个位置握着每一种值，其中有短的和长的 BigInt、一个 rope 字符串和一个 symbol，还有尾调用、迭代器循环里的 `return`、闭包、异常，以及宿主对没有块遗留的检查），`engine/test/compare` 里的 `frame_values.js`（同样种类的调用，也在 sanitizer 下运行），`engine/test/unit-api.c` 里的 `test_release_order` 和 `TestLocalsFreedInFewTurns`。`test_release_order` 给值设置宿主释放，并读出调用返回时已释放的句柄：9、12 和 16 个局部变量的函数在第 0、3、4、7 和 8 个槽位，从 0 到 15 的每个满槽位数，以及只有最后一个槽位被设置的情形，一个函数占三个槽位，一个函数握着另一个且每个都在较早的槽位里，还有两个互相握着的函数，它们要等回收。它看到调用结束时释放了哪些值、释放几次、按什么顺序，这是返回值和对遗留块的检查看不到的。`TestLocalsFreedInFewTurns` 通过轮询来数这两个循环的圈数（间隔为 1 时每圈都轮询）：一个有 200 个局部变量的函数的一次调用过去是 153 圈，现在是 78 圈。生成器和 async 函数的帧不是由这个循环释放的，它们在函数完成时有自己的结束方式：它们在 `TestFrameValuesFreed`（它们的 yield、返回值和 rejection 在任务跑完后检查）和 `frame_values.js`（一个生成器）里，用来看它们周围的调用没有变化，而不是作为这个循环的测试。

## 调用每次设置八个局部变量

一次调用开始时，过去四个局部变量一圈地设置它们（见[上面](#一次调用如何设置和释放它的值)）：一圈 15 条指令，8 条是对四个 JSValue 的两半的存储，7 条是这一圈本身（对剩余数量的测试、轮询计数、两次加法和跳转），所以每个局部变量 3.75 条。现在局部变量八个一圈地设置，一圈 22 条指令，每个局部变量 2.75 条，然后按剩余数量的各个位用四个、两个、一个一组来设置。有 3 个或更少局部变量的函数，一次调用因为多一次对计数的测试而多 1 到 4 条指令，有 4 个或更多局部变量的则更少：8 个局部变量的函数少 6 条，200 个局部变量的少 200 条。

对比它前面的提交，计数方式同上：

| 页面 | 之前 | 之后 | |
| --- | ---: | ---: | ---: |
| tifa 的商店，六个工作台页面（合计） | 924.3 M | 921.1 M | -0.35% |
| tifa 的商店，五个店面页面（合计） | 1,752.5 M | 1,749.6 M | -0.16% |
| tifa 的 MUI 测试应用，三个页面（合计） | 713.7 M | 712.4 M | -0.18% |
| tifa 的 bench 应用，table 和 spiral（合计） | 291.3 M | 289.7 M | -0.54% |
| `r_react_ssr` | 2,479.9 M | 2,472.1 M | -0.32% |
| `r_bench_table` | 3,278.4 M | 3,261.4 M | -0.52% |
| `r_bench_spiral` | 3,178.4 M | 3,168.6 M | -0.31% |

没有一项超过代码位置规则要求计时的 2%。`TestLocalsFreedInFewTurns` 现在要求一个有 200 个局部变量的函数的一次调用，这两个循环的圈数不超过 60，这个提交里是 53。

## 一次分发做几条指令的操作码

tifa 的页面经常运行的两种指令序列，现在各只用解释器的一次分发：读一个参数或局部变量，接着按名字读属性，以及一次 `typeof` 测试。

### 读局部变量加 get_field，以及字符串 switch 的 case

编译器做完自己的改写（`resolve_labels`）并放入轮询（`insert_polls`）之后，`fuse_code` 把下面每种序列的第一条指令变成一个操作码：

| 序列 | 操作码 |
| --- | --- |
| `get_arg0` `get_field` | `get_arg0_get_field` |
| `get_arg1` `get_field` | `get_arg1_get_field` |
| `get_loc0` `get_field` | `get_loc0_get_field` |
| `get_loc8 n` `get_field` | `get_loc8_get_field n` |
| `dup` `push_atom_value` `strict_eq` `if_false8`，字符串 `switch` 的 case | `dup_case_atom` |

只改那一个字节。新操作码的大小、栈效果和格式都和它所替换的指令相同，序列的其余部分仍照原样留在代码里。所以每条指令保持它的位置：跳转、行列表、轮询和函数的栈大小都和以前一样。序列里如果有跳转落在它里面，在第一条指令之后，就不动它（先标出每个跳转的目标，以及 `gosub` 之后的那条指令）。

这些操作码做什么：

- 四种读取不取引用地查看局部变量的值，并走 `get_field` 对不需要运行任何东西就能找到的属性所走的路径：自己的数据属性，或者越过非 exotic 对象的原型链上的属性，或者字符串或数字的原型，或者链的末端。然后它们压入这个属性，并跳过 `get_field`。其他所有情况：getter、setter、proxy、exotic 对象、`undefined` 或 `null`、错误的 `stack`，它们像单独读取那样压入局部变量，之后的 `get_field` 照常运行，带着它自己的错误以及行列号。
- `dup_case_atom` 在答案不需要看字符时，拿 `switch` 的值和 case 的字符串比较：atom 在运行时里是唯一的，所以是 atom 的字符串只有在同一个 atom 时才相等，长度不同的字符串不相等，不是字符串的值永远不等。然后它像 `if_false8` 那样分支，包括轮询。否则（不是 atom 的字符串，rope）它做 `dup`，其余的照常运行。

为什么借用是安全的：在快速路径上，读局部变量和压入属性之间不运行任何东西，所以没有东西能改变或释放这个局部变量。任何可能运行代码的情况（getter、proxy 陷阱、其中的回收）都走慢速路径，它像那两条指令一样先取引用。`fuse_code` 作用在已完成的代码上，所以 `arguments` 对象、闭包变量和 `let`、`const` 声明之前的检查，编译方式和以前一样：处于暂时性死区的局部变量用 `get_loc_check` 读，它不被融合。

字节码版本从 6 升到 7。`JS_ReadObject` 用 `poll_scan` 检查它读入的代码，现在它还拒绝后面没有跟着其序列其余部分的融合操作码（`invalid bytecode: a fused opcode without the instructions it reads`），所以宿主写入的字节码不能让解释器跨过一条它没检查过的指令。

`JS_SetFusedOps(ctx, 0)` 让运行时把这些序列重新分开编译，供测试使用。

### typeof 作为一个操作码

用 `==`、`===`、`!=` 或 `!==` 的 `typeof x == "<name>"`，对任何字符串都是一个操作码：`typeof_is_undefined` 和 `typeof_is_function` 和以前一样，其他的用新的 `typeof_is`，它握着字符串的 atom。取反的测试在它之后走另一个分支，或者后面跟着 `lnot`。以前只有 `"undefined"` 和 `"function"` 被合并，而取反的测试只有在 `if_false` 跟在后面时才合并。`typeof x < "u"` 和 `typeof x > "u"` 是 `typeof_is_undefined`，`<` 时取反，因为 `"undefined"` 是 `typeof` 的结果里唯一不排在 `"u"` 之前的。压缩器会写出这些形式：tifa 商店的包里有 189 处 `"string"` 的测试，165 处 `"function"`，112 处 `"object"`，65 处 `"number"`，35 处 `< "u"` 和 11 处 `> "u"`，等等。字符串在前（`"string" == typeof x`）的写法在其中很少见，保持原样。

`typeof` 从不抛错，两个字符串的比较不运行任何东西，所以这个操作码给出的和那些指令给出的相同，对标成 `document.all` 的对象的 `typeof` 结果也一样。`JS_SetFusedOps(ctx, 0)` 也让新形式回到以前的样子。

### 收益

每个应用每个页面一次渲染的指令数，callgrind 在 linux/amd64 上，每项改动对比它前一项：融合的读取和 case，在 tifa 商店的六个工作台页面上少 1.85%，店面页面少 1.59%，tifa 测试应用的 MUI 页面少 1.66%，bench 页面少 3.41%，在这之上 `typeof` 操作码又分别少 0.63%、0.83%、0.75% 和 1.42%。16 个页面的 HTML 相同。在 8 个代码位置、3 轮计时，上述形式的融合读取和 case，对比 v0.29.4：bench 页面用 0.966 的时间，`r_react_ssr` 是 1.018，它低于 callgrind 起决定作用的 3%。三项一起：bench 页面 0.943，工作台页面 0.976，店面页面 0.980。

编译器为查找这些序列付出代价：`fuse_code` 用一张说明哪些操作码是跳转或序列起点的表（`fuse_kind`），多走一遍每个函数的代码，对小于 256 字节的函数把它的标记放在栈上。编译 `r_react_ssr` 的包大约多花 0.98 M 条指令，它每次运行都要编译再渲染，所以三项改动一起让那里的一次运行多 1.46% 的指令。tifa 的每个 VM 读的是编译好一次的字节码，所以渲染不付这笔钱。数字见 [performance.zh-CN.md](../performance.zh-CN.md#一次做好几条指令的操作码)。

这些操作码占了 256 以下 11 个空闲编号里的 6 个（`OP_COUNT` 过去是 245），所以还剩 5 个。

### 测试

`engine/test/compare.sh` 在开和关这些操作码的情况下，运行 `engine/test/compare/fused_ops.js` 和 `engine/test/compare/typeof_is.js`，两者打印的文件，都和改动之前引擎打印的一致。`fused_ops.js` 用四种读取各读 32 种值（自己的和继承的数据属性、getter、setter、proxy 以及作为原型的 proxy、冻结的和被删除的键、数组、类型化数组、`String` 对象、`arguments`、函数、类、错误、每种原始值、`undefined` 和 `null`），有会改变或丢掉局部变量并运行回收的 getter 和 proxy 陷阱，映射的和非映射的 `arguments`，处于死区的局部变量，写局部变量的闭包，循环，一个生成器和一个 async 函数，读 `undefined` 和 `null` 的错误以及它们的行列号，以及对 atom 或非 atom 字符串、rope、宽字符串、数字、对象、symbol 和带 getter 的 case 做的 `switch`。`typeof_is.js` 对每种形式测试每种类型的值，既作为值也作为分支。`TestFusedOps`（根包）在生成的引擎上开和关操作码各运行这两个文件。`engine/test/unit-white.c` 的 `test_fused_ops` 和 `test_typeof_is` 检查，开和关时编译器产生了哪些操作码，跳进一对指令中间的跳转让它们保持分开，字节码能读回并运行，以及 `poll_scan` 拒绝序列被改过的融合操作码。`test_fused_edges` 在每个 `typeof` 测试里对带 [[IsHTMLDDA]] 的对象运行它们，对模块命名空间的导出运行它们（这些是经引用读取的，所以离开快速路径），对紧接着读取之前被直接 eval 或 `with` 块设置的参数或局部变量运行它们，并检查代码里有融合操作码。`test_short_opcode_index` 检查反汇编器为每个把局部变量或参数的下标放在操作码里的操作码所打印的下标。

## 不会失败的暂时性死区检查

在声明之前读或写的 `let`、`const` 或 `class` 局部变量会抛 `ReferenceError`，所以编译器用 `get_loc_check` 和 `put_loc_check` 读写它，并在块开始处用 `set_loc_uninitialized` 把它设为未初始化。多数读取在声明之后，在不能以其他方式进入的代码里，不需要检查。`resolve_labels` 在改写代码之前调用的 `tdz_checks`，把这些变成 `get_loc` 和 `put_loc`。

规则，对一个局部变量的某个检查 u：在 u 之前有一条指令 p 使该局部变量处于已初始化状态（对它的 `put_loc` 或 `set_loc`，或者对它的检查，它在未初始化时会抛错），并且从 p 到 u 之后的最小一段代码，其中每个跳转到里面某个标签的跳转都起于 p 之后。这一段里没有标签是 `catch` 的目标（异常可以从它的 `try` 覆盖的任何地方到达）或者 `gosub` 之后 `ret` 要返回的位置，也没有该局部变量的 `set_loc_uninitialized`。那么到达 u 的每条路都经过了 p。随着遍历的进行，这一段只会增长，所以它是对代码的一遍扫描，不是数据流分析，而对标签的遍历有预算（64K 加每字节代码 8），超出后检查保留。

保留检查的情形：

- `switch` 的 case：各个 case 共用一个块，而头部的分发跳过了前面各个 case 的声明。
- 在声明之前、在它自己的初始化表达式里读（`let a = a + 1`），以及在闭包里读。闭包通过引用读这个局部变量，保留自己的检查。闭包不能再次把局部变量设为未初始化，所以声明它的函数在声明之后去掉自己的检查。
- 声明该局部变量的循环的头部，以及这一段所包含的 `catch` 或 `finally` 之后的代码。
- `this`（`super()` 之前的派生构造函数），以及设置了 `JS_SetFusedOps(ctx, 0)` 时的所有检查。

`yield` 或 `await` 从它之后的那条指令继续，所以生成器和 async 函数遵循同样的规则。

当一个局部变量没有检查、没有别的读取、没有被闭包捕获，且函数没有直接 `eval` 时，它的 `set_loc_uninitialized` 也被去掉：第一次写之前没有东西读这个槽位。这条指令还会释放槽位里原来的值，所以它留在代码可以再次运行它的地方，在循环里或 `finally` 里（往回跳，或 `ret` 到它之前）：在那里去掉的话，上一次运行的值会一直活到下一次写入，而两个这样的值可能同时存活。只运行一次时，槽位里是帧开始时的 `undefined`。`with` 块或 `eval` 同样保留它。

这个 pass 不抛错地申请它的内存。没有内存时，检查保留，编译照旧继续。遇到有目标未知的跳转的代码，它就放弃。字节码对这次改动之前的引擎同样有效，所以它的版本不变。

### 收益

tifa 商店的六个工作台页面，每次渲染少 0.61% 的指令，店面页面少 0.34%，MUI 页面少 0.77%，bench 页面少 0.11%，`r_react_ssr` 的一次渲染少 0.06%（callgrind，linux/amd64，位置 0.0，16 个页面的 HTML 相同）。六个工作台页面的一轮运行 12.6K 次 `get_loc_check`，原来是 117.9K 次，运行 28.6K 次 `set_loc_uninitialized`，原来是 68.4K 次：分发少 1.8%。`r_react_ssr` 的一次运行编译它的包并渲染一次，多 0.62% 的指令：这个 pass 占编译它的包的 0.66 M 条指令。tifa 的每个 VM 读的是编译好一次的字节码，所以渲染不付这笔钱。计时见 [performance.zh-CN.md](../performance.zh-CN.md#暂时性死区的检查)。

### 测试

`engine/test/compare/tdz_checks.js` 对约 90 个用例打印 Node 24 打印的内容：声明之后的读写，`if`、循环、三元表达式和短路之后的读写，声明之前和它的初始化表达式里的读写，有贯穿的 `switch`，跳过声明的跳转，`default` 在最前面或在循环里，case 里的 `const` 和 `class` 以及读后面声明的 case，声明之前和之后以及循环里的闭包，`for`、`for of` 和 `for in` 的头部，`continue`，穿过 `finally` 的带标签的 `break` 和带标签的 `continue`，`try`、`catch` 和 `finally`，穿过 `finally` 带 `throw` 和 `return` 的生成器，async 函数和被 reject 的 `await`，`super()` 之前和之后的 `this`，抛错的字段初始化表达式，读后面或前面参数的参数，声明之前的解构以及 `with` 里的解构，`eval` 和 `with`，以及超过 256 个局部变量的函数等。`compare.sh` 也在保留检查的情况下运行它，`TestFusedOps`（根包）在生成的引擎上运行它。`engine/test/unit-white.c` 的 `test_tdz_checks` 数 28 个函数里剩下的检查和 `set_loc_uninitialized`，把每个对着 Node 给出的结果运行，检查设置 `JS_SetFusedOps(ctx, 0)` 时每个检查都保留，并运行一个循环，它每次运行都创建一个 8 MiB 的缓冲区，限制是只容得下一个。

## 共用槽位的局部变量

帧里为函数的每个局部变量都有一个槽位，调用开始时把它设为 `undefined`，调用结束时释放它。压缩过的代码把局部变量在整个函数里声明一次，而 React DOM 服务端渲染器的大函数里有很多只在一个分支里用到的局部变量：`pushStartInstance` 有 138 个局部变量，`renderElement` 有 92 个。`resolve_labels` 在 `tdz_checks` 之后调用的 `share_local_slots`，在没有哪段代码同时需要两个局部变量的值时，让它们共用一个槽位。`pushStartInstance` 于是有 11 个槽位，`renderElement` 有 27 个，而 tifa 商店包里的 2021 个函数有 9983 个槽位，原来是 12191 个局部变量。

当某段代码从这条指令往后，在写之前读到一个局部变量时，它在这条指令处是活跃的：这是在代码的各个块上、从读取处往回做的数据流分析。当两个局部变量中，每一个被写的地方，另一个都不活跃时（没有任何东西读的写也算），它们共用一个槽位。那么每次读，都会找到它自己那个局部变量最后一次写入留在那里的东西，或者帧开始时的 `undefined`：在某条路径上写之前就被读的局部变量，从函数开始处起就是活跃的，所以那条路径上它活跃期间不会写别的局部变量。局部变量按下标顺序处理，每个得到与它冲突的局部变量都没有的第一个槽位。局部变量的下标从不会变大，而代码保持每条指令的大小：这个 pass 改写阶段 2 的长指令的下标，在 `resolve_labels` 选择短形式之前，短形式现在能覆盖更多局部变量。

一个局部变量保留自己的槽位，如果除了点它名字的指令之外还有东西读写它，或者它的槽位里放的不止它的值：

- 闭包捕获它，或者常量池里已编译的某个函数点了它的名字。
- 函数开始时写它：`this`、`new.target`、home object、active function、`arguments` 和函数表达式的名字。
- 它上面还留着暂时性死区的检查，这个检查读槽位的状态并在错误里点出该局部变量的名字，或者有 `close_loc`。
- 它是类的私有名字。
- 它在 `catch` 或 `finally` 块开始处是活跃的，因为异常可以在 `try` 代码的任何一条指令处离开。每个 `gosub` 之后的块都被当作跟在每个 `ret` 之后。

有直接 `eval`、`with` 块或映射的 `arguments` 对象的函数，eval 或模块的代码，超过 1024 个局部变量的函数，或者这个 pass 不认识的代码，保持它们原来的槽位（`js_frame_plain`），设置了 `JS_SetFusedOps(ctx, 0)` 时每个函数也一样。`yield` 或 `await` 从它之后的指令继续，帧保持原样，所以生成器和 async 函数遵循同样的规则。

槽位改变带来的变化：代码，函数保留的下标（`this_var_idx` 等），常量池中点了它的局部变量的函数的闭包变量（`var_idx`），以及它的局部变量列表。一个槽位保留它第一个局部变量的名字，用于暂时性死区检查的错误（它的局部变量有自己的槽位）和字节码的转储。错误栈点的是函数和行，不是局部变量，所以不变，而引擎没有调试器（`debugger` 被跳过）。`tdz_checks` 去掉的 `set_loc_uninitialized`，现在用一个越过所有局部变量的下标（`LOC_DROPPED`）保留它的位置，因为共用一个槽位的两个局部变量可能一个被去掉、一个被保留。字节码对这次改动之前的引擎同样有效，所以它的版本不变。

一个值在共用它槽位的下一个局部变量被写入时就释放，而以前它会活到调用结束，并且一个帧占的栈更少。所以只被一个已死的局部变量握着目标的 `WeakRef`，现在可能比以前更早给出 `undefined`。引擎从没像规范要求的那样，让目标一直存活到任务结束。

这个 pass 不抛错地申请它的内存。没有内存时什么也不变，编译照旧继续。

### 收益

一次渲染的指令数，在 linux/amd64 上用 callgrind 对生成的构建在位置 0.0 数，对比 main 加上对链接后模块的优化器（619e1bc8），shop、store 和 MUI 页面的 HTML 相同：六个工作台页面少 1.43%，店面页面少 1.02%，MUI 页面少 0.82%，`r_react_ssr` 的一次渲染少 1.81%。编译器为此付出代价：这个 pass 占编译 tifa 商店的包的 3700 万条指令（`js_create_function` 的 3.9%），占编译 `r_react_ssr` 的 420 万条，所以编译它的包并渲染一次的 `r_react_ssr` 运行，多 2.45% 的指令和 2.6% 的时间。tifa 的 VM 读的是编译好一次的字节码，不付这笔钱。在生成的构建上，8 个代码位置、3 轮计时，没有哪个页面的耗时能和 main 的区分开：六个工作台页面 1.002（各位置从 0.996 到 1.010），店面页面 1.000，MUI 页面 0.995，单独一次 `r_react_ssr` 渲染 0.994（0.981 到 1.004）。一次 `r_react_ssr` 运行的内存峰值是 8.3 MiB，原来是 8.7。数字见 [performance.zh-CN.md](../performance.zh-CN.md#共用一个槽的局部变量)。

### 测试

`engine/test/compare/share_slots.js` 打印 Node 24 打印的内容，涵盖：一个接一个的块里 13 种值，分支里和循环每圈里在任何写之前被读的局部变量，没人读的写，`catch` 读 `try` 里写的局部变量的 `try`，读 `return` 拿走的局部变量的 `finally`，循环里的闭包和嵌套的闭包，一个下标变小的局部变量的闭包，栈上有 `switch` 的值而某个 case 返回的情形，一个 yield 的生成器和一个在某个范围中间 await 的 async 函数，一个永远挂起的，宽松模式和严格模式下的 `arguments` 对象，直接 `eval` 和 `with`，递归，紧挨着共用槽位的局部变量的暂时性死区检查，以及一个有 40 个 case、每个 case 有自己 `var` 的 `switch`，并要求这些运行让堆保持原样。`compare.sh` 也在槽位不共用的情况下运行它，`TestFusedOps`（根包）在生成的引擎上运行它。`engine/test/unit-white.c` 的 `test_share_local_slots` 在 pass 开和关时数 18 个函数的槽位，把每个对着 Node 给出的结果运行，检查有 40 个 case 的 switch 占 2 个槽位，暂时性死区检查的错误点的是它自己的局部变量，以及一个闭包读共用槽位之外的局部变量的函数，能用 `JS_ReadObject` 读回并运行。

## JSValue 如何复制

JSValue 是 16 字节，一个值加一个标签。解释器把它写到值栈上时是两次 8 字节的存储，因为值在寄存器里而标签是常量。LLVM 复制一个 JSValue 用一次 16 字节的 SSE 移动。它用两种方式做这个移动：SLP 向量化器把两个相邻的 8 字节移动连起来，以及一次结构体复制变成 LLVM 展开的 16 字节 `memcpy`。在 Zen 3 上，一次 16 字节的加载不能从两个仍在途中的 8 字节存储得到数据：它要等两者都到达缓存（在什么也不做的循环里是 24 个周期，而 8 字节的存储被 8 字节的加载读取是 1 个周期）。从一次 16 字节的存储里做一次 8 字节的加载也慢，11 到 13 个周期。

用 valgrind lackey 对短运行追踪每一次加载和存储，数出前 120 条指令里由两个或更多存储提供数据的加载，以及由 16 字节存储提供数据的小于 16 字节的加载。其中 95% 到 99.8% 在 `JS_CallInternal` 里。每 100 条指令，先是第一种再是第二种：`v8_richards` 0.45 和 0.87，`q_func_call` 0.28 和 0.55，`q_int_arith` 0.44 和 0.88，`r_react_ssr` 0.13 和 0.24。含有它们最多的操作码是调用（`call_method`、`call1`、`call`、`tail_call_method`：它们把函数和 `this` 复制进被调用者的参数块），`put_field`（它按值传对象和值），`put_loc*`（把栈顶复制进局部变量），`inc_loc`（它把局部变量复制到临时变量，再读它的两半），`get_loc*`，以及在字符串循环里的 `add`。

这个特性告诉 LLVM，16 字节的未对齐移动是慢的。JSValue 只对齐到 8 字节，所以 LLVM 现在把它的复制展开成两次 8 字节加载和两次 8 字节存储，8 字节的加载可以从中得到数据。SLP 向量化器做的移动不会再被展开，所以它被关掉了。对生成的引擎做同样的追踪，发现剩下的最多 1.5%（`r_react_ssr` 4,541 和 14,349，对比 433,938 和 813,223，`v8_richards` 776 和 2,056，对比 408,409 和 790,662，`q_func_call` 538 和 740，对比 100,676 和 201,060），而 `JS_CallInternal` 里已经没有 SSE 移动了。

没有做的事，以及原因：

- **小libc和其他C文件。**libc 的 `memcpy` 和 `memmove` 特意一次移动 16 字节，有这个特性时，17 到 128 字节的复制会变成 8 字节的移动，加载和存储翻倍。其他文件里没有 JSValue。
- **开着SLP向量化器的这个特性。**它在 `JS_CallInternal` 里留下 29 个 SSE 移动（向量化器在 `put_loc` 等处做的复制），并让 `r_shiki_regexp` 慢 2.8% 到 3.3%，在 8 个位置里的 7 个或 8 个上。
- **单独关掉向量化器。**它在 `JS_CallInternal` 里留下 201 个 SSE 移动，也就是被展开的 `memcpy`。
- **arm64。**不给这些标志：这个特性只有 x86 有，那里也没测过什么。

指令数会略有增加，因为一次复制是四条指令而不是两条：对比之前的引擎，统计的 14 个脚本里的 13 个多 0.03% 到 1.8%，`q_prop_write` 多 5.3%，`r_react_ssr` 多 0.7%。复制挡着路的地方时间会降低，见 [performance.zh-CN.md](../performance.zh-CN.md#c-引擎用两次-8-字节的移动复制-jsvalue)。

这次改动挪动了解释器函数体的最佳位置。用之前的代码时它是行内 0 字节。用 8 字节复制时，在渲染脚本上试过的 8 个位置里，40 最快（见[上面](#解释器和正则匹配器的位置)）。

## 没有线程的 Atomics

`Atomics.wait` 按规范给出的顺序转换它的参数，然后抛 `TypeError: cannot block in this thread`。`Atomics.notify` 转换它的参数并返回 0。test262 给出和以前相同的 58 个已知错误，`engine/test/test262.conf` 里排除了 66 个测试，全是 `built-ins/Atomics/wait` 和 `notify` 的：7 个在主线程上等超时，59 个用 `$262.agent.safeBroadcast` 启动 agent。那个函数开头是对一个不同的值做 `Atomics.wait`，它本该立即返回，现在却抛错。这 59 个里有 53 个，某个 agent 还要等唤醒或超时。有 6 个没人在等，只是那个探测把它们挡在外面。`api-test.c` 保留了这 6 个所说的、现在仍然成立的东西：没人在等时 `Atomics.notify` 给出 0，`Atomics.wait` 按规范的顺序转换它的参数（数组及其共享缓冲区、下标、值、超时），并在抛出无法阻塞之前，先抛出转换所抛的错误。它还检查，值不同或没有超时的 wait 抛同样的错。

`Atomics.pause` 是 `Atomics` 里唯一一个属于提示而不是操作的函数。它过去以内联汇编运行 `pause`（x86）或 `yield`（arm64），生成器没法处理这个，所以它是生成器最后一个必须用桩替代的函数。ecma262 只要求平台支持向 CPU 发信号时引擎才发提示，所以这个函数现在什么也不做。它的参数仍然检查：undefined 或整数 Number，否则 `TypeError`。树所钉住的 test262 测试这一点（`non-integral-iterationnumber-throws.js`），但 ecma262 在 2026 年 6 月去掉了这个参数（pull request 3847），test262 几周后也不再测试它（pull request 5075），所以钉住的版本往前移之后，这项检查可以去掉。

## Go 宿主的 C API

Go 宿主通过生成的代码调用引擎，它不传可变参数，也不从 Go 里取 C 函数指针。所以树里为它准备了这些：

- `JS_NewHostFunction(ctx, name, name_len, length, id)` 创建一个外形像内置函数的函数：有名字和长度，没有 `prototype`，不是构造函数，`toString` 里是 `[native code]`。对它的调用是 `host_call(vm, &this, argc, argv, id)`，`argv` 里至少有 `length` 个可读的参数。id 是 0 或更大的 int32，保存在函数对象里内置函数 16 位 magic 的旁边（cproto `JS_CFUNC_host`）。负的 id 抛 `RangeError`：`host_call` 把负的 magic 留给引擎自己的请求。没有内存创建函数、它的名字或它的属性时，它返回异常并挂起内存不足错误，和 `JS_NewCFunction2` 以及其他创建函数的函数一样。
- `JS_NewStringChars(ctx, len, wide)` 创建一个字符串，宿主在字符串被使用之前就地写入它的字符。8 位字符串带它的 NUL。`len` 为 0 给出空字符串，长度超过字符串最多能容纳的长度抛 `RangeError`，没有内存抛内存不足错误。
- `JS_ThrowErrorKind(ctx, kind, msg, len)` 抛出一个 `JSErrorKind` 的错误（每个原生错误构造函数一种），消息在 `JS_ThrowTypeError(ctx, "%s", msg)` 截断的地方截断：在 NUL 字节处和 255 字节处。它不认识的 kind 抛 `InternalError`。
- `js_host_layout` 是一张按 `JS_HOST_LAYOUT_*` 索引的 int64 表，保存宿主就地读写的东西：引用计数相对于值所握的地址的位置（-4），字符串第一个字里长度的掩码和宽位，它的字符开始的位置（12），字符串最多能容纳的字符数，以及 Go API 用到的内置 atom 的编号（`message`、`stack`、`then` 等）。引擎里没有东西读它。生成器从 llc 的文本里读出它，并把它作为 Go 常量写进 `engine_decl_<arch>.go`（`layoutStringChars`、`atomMessage` 等），当表的条目比它有名字的多或少时就停下（`gen.HostLayoutNames`）。要在两处的末尾加一个条目。
- `JS_NewHostFunctionRelease(ctx, name, name_len, length, id)` 为一个是宿主句柄的 id 创建同样的函数，句柄是宿主为它保管的东西，比如一个 Go 函数。引擎释放这个函数时（丢掉它的最后一个引用、在一次回收中，或释放运行时时），会从它的终结函数里调用一次 `host_release(vm, id)`（`host.h`）。`host_release` 不运行脚本，也不回调引擎。失败的调用没有创建函数，也不释放任何东西：句柄仍属于宿主。
- `JS_NewEscapeTextFunction(ctx, name, name_len)` 创建 `VM.RegisterEscapeText` 的函数：用 C 写的 React DOM 的 `escapeTextForBrowser`，外形像内置函数，长度为 1。它是引擎的一个 C 函数，不经 `host_call` 调用，所以一次调用的花费，约等于调用一个空 JavaScript 函数的花费，不是调用一个 Go 函数的花费。参数按 `"" + text` 的方式转换，五个字符一次一个 UTF-16 单元地变成实体，一个不含它们的字符串原样返回。循环每 4096 个单元读一次中断标志。见 [go-api.md](go-api.zh-CN.md#registerescapetext)。
- `JS_NewCopyPropsFunction(ctx, name, name_len)` 创建 `VM.RegisterCopyProps` 的函数：React 把配置的键复制到元素 props 里的那个循环，用 C 写的，外形像内置函数，长度为 4。`js_copy_props` 在复制之后回答 true，什么也没改时回答 false，而且只在它知道这个循环不会做别的事时才复制：配置、props、`Object.prototype`、`hasOwnProperty` 和 `Function.prototype.call` 都在调用里检查，除了 `JSContext.copy_atoms` 里的五个 atom（`key`、`ref`、`__self`、`__source`、`call`）外，调用之间不保存状态。它以配置的 32 个键和 props 以及 `Object.prototype` 的 shape 的 64 个槽位为界，所以它不读中断标志。见 [go-api.md](go-api.zh-CN.md#registercopyprops)。
- `JS_NewStartTagFunction(ctx, name, name_len)` 和 `JS_NewEndTagFunction(ctx, name, name_len)` 创建 `VM.RegisterStartTag` 和 `VM.RegisterEndTag` 的函数：React DOM 的 `pushStartGenericElement` 写出的带属性、样式和文本的开始标签，以及 `endChunkForTag` 的结束标签，用 C 写的，外形像内置函数，长度分别为 6 和 1。`js_start_tag` 在缓存保留的一个缓冲区里构建标签（`js_emit_buf_take`、`js_emit_buf_end`），把它复制成一个刚好那么长的字符串（因为 `__js_realloc` 从不把 arena 块移到更小的类别），并原地把它加到目标数组的末尾，凡是 React 的函数可能做别的事的地方，它都原样返回它的 `decline` 参数，什么也不改。这两个函数在每个上下文里共用一个缓存（`JSContext.emit_cache`，由它们中的第一个创建，不运行脚本：`js_emit_make_builtin` 在 `Function.prototype.call` 和 `Array.prototype.push` 还没创建时，经 `JS_AutoInitProperty` 创建它们，缓存要完整了才放进上下文）：三个 atom，以及三张在第一次进入时创建、半满时翻倍的表（开放寻址）：最多 512 个属性名和 512 个样式名的写法，以及按标签字符串的地址找到的最多 256 个结束标签。表满了就清空再重新填。一次调用以 props 的 32 个键和样式的 32 个键为界，对它转义或裁剪的长字符串每 4096 个单元读一次中断标志，对它复制的长字符串每 524,288 个单元读一次。见 [go-api.md](go-api.zh-CN.md#registerstarttag-和-registerendtag)。
- `JS_FreeModuleTopLevel(ctx, m)` 释放一个已经运行到结束、没有异常的模块的顶层函数，连同顶层的代码，以及模块里没有函数、没有导出握着的顶层变量（模块链接之后，把它的导出单独放在它的导出条目里）。QuickJS 把这个函数一直保留到模块消失，对上下文里的模块来说，那是运行时被释放的时候。对没有运行过、仍在顶层 `await` 处等待、或者抛过错的模块，它什么也不做。

在 Go 一侧，`internal/cengine` 的 `host_call` 把 0 或更大的 id 交给用 `SetHostFunc` 注册的 `HostFunc`（只注册一次，在 init 函数里），把引擎的请求交给 `gohost`。函数拿到调用的影子指针：它按地址传给引擎的东西，以及回调引擎的调用的栈帧，都在它之上。

## 用追加方式构建的字符串

QuickJS 只在一种情况下原地增长字符串：对局部变量做 `x += y`（`add_loc`），且没有别的东西握着这个字符串。其他每次追加，在结果短于 8192 个字符时（`JS_STRING_ROPE_SHORT2_LEN`）都复制整个字符串，之后才造 rope，也就是两个字符串连在一起，到结果被读取时才拼平。在 `x = x + y` 的 `+` 处，或对参数或闭包变量做 `x += y` 时，变量和栈都握着这个字符串，所以它的引用计数是 2，总是被复制。一个由 n 次短追加构建的字符串就这样被复制 n 次，复制的总量是它长度的平方。

React 构建页面就是这样的：`renderToString` 把每一段 HTML（1 到 15 个字符）追加到一个闭包变量上（`push(u) { a += u }`）。页面上只要有 Latin-1 之外的字符，字符串就是 16 位的。一次 `r_react_ssr` 脚本的渲染，在 400,000 次复制里复制了 780 MB，占它全部 `memcpy` 的 96%。上游 QuickJS（2026-10-26）和 quickjs-ng（2026-10-24）也是这么做的。V8 对 13 个字符或更长的结果都造 rope（`ConsString`）。

引擎现在改为找到那个变量：

- 左边是至少 64 个字符、引用计数为 2 的字符串的 `OP_add`（`JS_STRING_APPEND_MIN`），读下一条指令，如果有一个 `dup`，就跳过它读再下一条：一个由闭包检查暂时性死区的 `let`，用 `dup` 和 `put_var_ref_check` 存储。当它存入的局部变量、参数或闭包变量里的值就是这同一个字符串时，另一个引用就是这个变量的，存储会丢掉它（`js_add_string_store`）。
- 然后 `JS_ConcatStringAppend` 在块有空位时原地追加。否则它复制这个字符串，并多留出相当于它长度一半的空位，这样接下来的追加就能原地进行。`add_loc` 在原地追加失败时，采用同样的复制。
- 右边超过 512 个字符时，像以前一样造 rope，其他情况（左边是 rope、有其他持有者、存入别的东西）走 `JS_ConcatString`。
- 属性名（atom）从不增长，见上面那一行。

一个由 n 次追加构建的字符串，于是被复制约 log1.5(n) 次。一个 64 个字符及以上、带空位复制过的字符串，在释放前最多有三分之一的块没有用。一个左边是字符串的字符串 `+`，在解释器里为这些检查多跑约 2 条指令。它的收益和代价见 [performance.zh-CN.md](../performance.zh-CN.md#靠追加建出来的字符串)。

## JSON.parse 直接从文本读键和普通字符串

`JSON.parse` 过去为文本里的每个键和每个字符串值，都在一个 `StringBuffer` 里造字符串，这个缓冲区开始时有 32 个字符的空位，会增长，最后缩到它的长度。键然后要经过 `JS_ValueToAtom`，它对字符串做哈希，找到 atom，再释放字符串。一张表格的行重复同样的几个键成千上万次，所以这些工作大多找到的是解析刚刚找到过的 atom。

现在，在没有 JSON 超集时（`JSON.parse` 和 `JS_ParseJSON`）：

- 一个由 ASCII 字符组成、没有转义、不以数字开头、后面跟着 `:` 的键，由 `json_scan_key` 直接从文本里读。`json_key_atom` 从一个 64 个槽位的缓存里取它的 atom，槽位按键的长度和它的三个字节来选，做一次字节比较，否则就用 `JS_NewAtomLen` 从它的字符里取（它在创建字符串之前先找 atom），并把 atom 放进它的槽位。`:` 这个 token 被跳过。
- 一个由 ASCII 字符组成、没有转义的字符串值，一次复制就按它的长度创建（`js_new_string8_len`）。继续出现转义或多字节字符的字符串，用它开头那段 ASCII 作为缓冲区的起点，然后和以前一样继续。
- `json_plain_run` 一次 8 个字节地找这样一段到哪里结束：对一个字做三次测试，标出小于 0x20 的字节、引号和反斜杠，最高位标出大于 0x7f 的字节，最低的标记就是结束这一段的第一个字节（借位只可能标出已找到的那个字节之后的字节）。
- 新数组最后一个元素之后的元素，用 `add_fast_array_element` 放进去，这就是 `JS_DefinePropertyValueUint32` 最终调用的东西，只是省掉了它前面的查找。
- 文本是 8 位时，一次 8 个字节地把它变成 C 字符串：大于 0x7f 的字节按字计数，连续 8 个 ASCII 字节按字复制。对 16 位文本的循环，每 65536 个字符才问一次是不是该读中断标志了，不是每个字符都问。

### 为什么它给出相同的值和错误

- atom 就是字符串会给出的那个：由 ASCII 组成、不以数字开头的键永远不是下标，所以 `JS_NewAtomStr` 也会像 `JS_NewAtomLen` 那样按字符查找它。是下标或看起来像数字的键（`"0"`、`"4294967294"`、`"-0"`、`"01"`），带转义的键，带大于 0x7f 的字符或有 65535 个字节或更多的键，以及 JSON 超集的每个键，仍然像以前一样经过 token。缓存里的键与它的 atom 的 8 位字符串逐字节比较，只有 8 位的 atom 才进缓存。
- 步骤的顺序相同：键的 atom 在循环原来从 token 里取它的地方取，在对象创建之后，而 `:` 是一个不会失败的 token。键之前和 `:` 前后的空白，是 JSON 允许的空白，由 `json_next_token` 跳过。当文本不是一个普通的键加一个 `:` 时，什么也不动，由 token 去读它，所以每个错误的消息和位置都相同。
- 重复的键、`__proto__` 和键的顺序不变：属性由同一个 `JS_DefinePropertyValue` 定义，重复的键，不管有没有转义，都是同一个 atom。
- 缓存存在 `JS_ParseJSON3` 里，在栈上，每个槽位握着对它的 atom 的一个引用。`json_key_cache_free` 在解析结束时、每条路径上（返回值、语法错误、内存不足、中断）都把它们还回去，所以没有 atom 活得比解析更久，也没有东西从一次解析保留到下一次。reviver 在解析之后，在它创建的值上运行。
- 中断标志在相同的地方读。普通的一段短于 65535 个字节，所以它创建的字符串以前也没有读过标志，而 C 字符串的字则按同样的 65536 个字符一步来计数。`white.c` 计数的、两次读取之间的工作单位，在 8 位、Latin-1 和 16 位文本的表上完全相同，一个单位不差。
- 内存不足抛同样的无法捕获错误。字符串现在按它的长度申请，不是先要 32 个字符再增长，所以到达内存上限的位置可能移动几个字节。
- 抛“JSON nesting too deep”的嵌套深度相同，1999 个对象或数组。

### 收益

对一张有 1000 行的表（92 KB）做 `JSON.parse`，指令少 60%，tifa 商店工作台加载的订单（11 KB）少 31%。数字，包括不是表格的文本，见 [performance.zh-CN.md](../performance.zh-CN.md#jsonparse-从文本里直接读键和普通字符串)。

### 测试

`engine/test/compare/json_keys.js` 打印 Node 24 对这些情形打印的内容：是下标或看起来像数字的键，重复的键，`__proto__`，转义的键和孤立代理项，键和冒号周围的空白，reviver，`JSON.rawJSON`，各行键相同的表，键顺序不同、键更少或更多的表，一种形状嵌套 400 层，以及超过 65536 个字节的 ASCII、Latin-1 和 16 位文本的字符串。`engine/test/compare/recorded/json_key_errors.js` 保存 36 个会抛错的文本的消息、位置和栈，以及能解析的最深嵌套，都是这次改动之前的引擎打印的内容。在 `engine/test/unit-white.c` 里，`test_json_plain_run` 对照字节循环检查 `json_plain_run`，覆盖直到 40 的每个长度、一段可能结束的每个位置、随机文本，以及它 65535 字节的上限。`test_json_key_cache` 检查，缓存持有的每个键 atom，在一次正常返回的解析、一次抛错的解析、一次带 reviver 的解析，以及每次分配依次失败的解析之后，都已归还，`test_poll_work_gaps` 则检查 8 位、Latin-1 和 16 位表上两次读中断标志之间的工作单位。
