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
| 一次调用用更少的循环轮数设置和释放它的值 | v0.29.1 | [c-engine.md](c-engine.md#how-a-call-sets-and-frees-its-values) |
| 解释器调用 C 函数时不经过 JS_CallInternal | v0.29.4 | [c-engine.md](c-engine.md#how-the-interpreter-calls-a-c-function) |
| 调用结束时跳过没有引用计数的值 | v0.29.5 | [c-engine.md](c-engine.md#the-end-of-a-call-skips-the-values-with-no-reference-count) |
| 一次调用每轮设置八个局部变量 | v0.29.5 | [c-engine.md](c-engine.md#a-call-sets-its-locals-eight-to-a-turn) |
| 一次做几条指令的 opcode：读局部变量加它的 get_field、对字符串做 switch 的 case、typeof 测试 | v0.29.5 | [c-engine.md](c-engine.md#opcodes-that-do-a-few-instructions-at-once) |
| 不会同时存活的局部变量共用栈帧的一个槽 | v0.29.7 | [c-engine.md](c-engine.md#locals-that-share-a-slot) |
| JSON.parse 直接从文本读键和普通字符串，一次解析内用键 atom 的缓存 | v0.29.7 | [c-engine.md](c-engine.md#jsonparse-reads-keys-and-plain-strings-from-the-text) |
| 引擎在释放对象时被请求的回收什么也不做 | v0.29.10 | [c-engine.md](c-engine.md#a-collection-asked-for-while-objects-are-freed) |
| 离开进入引擎的调用时发生到一半的 panic 会让 VM 报废，不论是什么 panic，包括低于 0x1000 的地址错误，之后不会再进入那个引擎，连释放一个值也不行 | v0.29.11 | [c-engine.md](c-engine.md#which-panics-break-a-vm) |
| 每个调用了别的函数的生成函数都有 Go 的栈检查，所以地址错误不再取决于生成器在一串没有栈检查的函数下面留出的空间 | v0.29.11 | [c-engine.md](c-engine.md#functions-without-a-stack-check) |

## 上游 QuickJS 的提交

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 3d5e064 之后上游 QuickJS 的提交，逐个并入 C 树（Array 的 slice 和 splice、js_strict_eq2 借用的操作数、模板字面量、unicode_script），以及没有并入的那些 | v0.29.0 | [upstream-commits.md](upstream-commits.zh-CN.md) |
| Iterator.concat 遵循 IteratorClose，并在一步失败后结束 | v0.29.8 | [upstream-commits.md](upstream-commits.md#iterator-concatenation) |
| 没有初始累加值时归约空迭代器，不关闭它 | v0.29.8 | [upstream-commits.md](upstream-commits.md#empty-iterator-reduction) |
| 稀疏数组截断先移除下标，再压实属性表 | v0.29.8 | [upstream-commits.md](upstream-commits.md#array-truncation) |

## Go API

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| Call 报告函数表达式的异常 | v0.26.0 | [go-api.md](go-api.md#call-reports-the-exception-of-the-function-expression) |
| Go 转换不了的字符串作为错误返回 | v0.26.5 | [go-api.md](go-api.md#a-string-go-cannot-convert-comes-back-as-the-error) |
| `*Error` 说明一次调用为什么结束 | v0.26.7 | [go-api.md](go-api.md#error-says-why-a-call-ended) |
| 孤立代理项得到合法的 UTF-8 | v0.26.8 | [go-api.md](go-api.md#valid-utf-8-for-lone-surrogates) |
| Go 传来的非法 UTF-8 每个字节变成一个 U+FFFD | v0.26.10 | [go-api.md](go-api.md#invalid-utf-8-from-go-becomes-one-ufffd-per-byte) |
| RegisterFunc 遇到已有的名字不再中止 | v0.26.11 | [go-api.md](go-api.md#registerfunc-no-longer-aborts-on-an-existing-name) |
| NewVM 失败时不泄漏任何东西 | v0.26.14 | [go-api.md](go-api.md#newvm-leaks-nothing-when-it-fails) |
| SetShapeCache 和 DefaultShapeCache | v0.26.16 | [go-api.md](go-api.md#setshapecache-and-defaultshapecache) |
| Builtin，作为内置函数调用的 Go 函数 | v0.26.17 | [go-api.md](go-api.md#builtin-a-go-function-called-as-a-built-in) |
| 在 Go 里创建的 Error 的 Error() | v0.26.17 | [go-api.md](go-api.md#error-of-an-error-made-in-go) |
| NewArray | v0.26.17 | [go-api.md](go-api.md#newarray) |
| SetStrip | v0.26.17 | [go-api.md](go-api.md#setstrip) |
| RunGC、MemorySize 和 GCThreshold | v0.26.17 | [go-api.md](go-api.md#rungc-memorysize-and-gcthreshold) |
| Args.WTF8 | v0.27.0 | [go-api.md](go-api.md#argswtf8) |
| SetRegExpCache 和 DefaultRegExpCache | v0.27.0 | [go-api.md](go-api.md#setregexpcache-and-defaultregexpcache) |
| OnClose | v0.27.0 | [go-api.md](go-api.md#onclose) |
| EvalBytecode 在 Go 里等待模块的顶层 await | v0.27.0 | [go-api.md](go-api.md#evalbytecode-waits-for-a-modules-top-level-await-in-go) |
| Builtin 的字符串在 Go 里转换 | v0.27.2 | [go-api.md](go-api.md#strings-of-builtins-converted-in-go) |
| Args.Latin1、Args.UTF16、NewStringLatin1 和 NewStringUTF16 | v0.27.2 | [go-api.md](go-api.md#argslatin1-argsutf16-newstringlatin1-and-newstringutf16) |
| 模块运行完之后释放它的顶层 | v0.28.4 | [go-api.md](go-api.md#a-module-frees-its-top-level-once-it-has-run) |
| promise 任务的异步上下文，供 AsyncLocalStorage 用 | v0.28.5 | [go-api.md](go-api.md#the-async-context-of-promise-jobs) |
| RegisterEscapeText，用 C 写的 React DOM 的 HTML 转义 | v0.29.2 | [go-api.md](go-api.md#registerescapetext) |
| RegisterCopyProps，用 C 写的复制 React 元素 props 的循环 | v0.29.3 | [go-api.md](go-api.md#registercopyprops) |
| RegisterStartTag 和 RegisterEndTag，用 C 写的 React DOM 为元素写出的开始标签和结束标签 | v0.29.10 | [go-api.md](go-api.md#registerstarttag-and-registerendtag) |
| 比引擎检查时用的版本更新的 Go 会让构建停止 | v0.29.11 | [go-api.md](go-api.md#a-go-newer-than-the-one-the-engine-was-checked-with-stops-the-build) |
| 改变汇编器编码引擎方式的构建选项会让构建停止 | v0.29.11 | [go-api.md](go-api.md#a-build-option-that-changes-how-the-assembler-encodes-the-engine-stops-the-build) |

## 守得住的限额

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 转换字符串时内存耗尽不泄漏 | v0.26.0 | [uncatchable.md](uncatchable.md#no-leak-when-converting-a-string-runs-out-of-memory) |
| 嵌套调用保持外层的截止时间 | v0.26.2 | [uncatchable.md](uncatchable.md#nested-calls-keep-the-outer-deadline) |
| 中断不能通过 promise 或迭代器被捕获 | v0.26.6 | [uncatchable.md](uncatchable.md#interrupts-cannot-be-caught-through-promises-or-iterators) |
| 内存耗尽不可捕获 | v0.26.6 | [uncatchable.md](uncatchable.md#out-of-memory-is-uncatchable) |
| Go 函数里的中断和内存耗尽 | v0.26.6 | [uncatchable.md](uncatchable.md#interrupts-and-out-of-memory-inside-go-functions) |
| Go 函数里的 panic | v0.26.6 | [uncatchable.md](uncatchable.md#a-panic-in-a-go-function) |
| 内存所剩不多时编译 | v0.26.6 | [uncatchable.md](uncatchable.md#compiling-with-little-memory-left) |
| 求值超时按单调时钟走 | v0.26.7 | [uncatchable.md](uncatchable.md#the-eval-timeout-runs-on-the-monotonic-clock) |
| Close 期间和之后调用 Interrupt 是安全的 | v0.26.10 | [uncatchable.md](uncatchable.md#interrupt-is-safe-during-and-after-close) |
| 编译正则计入中断检查 | v0.27.2 | [uncatchable.md](uncatchable.md#compiling-a-regexp-counts-toward-the-interrupt-check) |
| 解析和构建字符串计入中断检查 | v0.27.2 | [uncatchable.md](uncatchable.md#parsing-and-building-strings-count-toward-the-interrupt-check) |
| 每个可能运行 JavaScript 的方法都有求值超时 | v0.29.5 | [uncatchable.md](uncatchable.md#every-method-that-can-run-javascript-has-the-eval-timeout) |
| 遍历长字符串的循环分步读取中断标志 | v0.29.5 | [uncatchable.md](uncatchable.md#loops-over-a-long-string-read-the-interrupt-flag-in-steps) |
| 被异常关闭的迭代器的 return 方法里的内存耗尽不会被捕获 | v0.29.11 | [uncatchable.md](uncatchable.md#out-of-memory-in-the-return-method-of-an-iterator-that-an-exception-closes) |
| 开始标签和结束标签函数在每个长循环里读取中断标志 | v0.30.0 | [uncatchable.md](uncatchable.md#the-start-and-end-tags-read-the-interrupt-flag-in-every-long-loop) |
| 很长的 rope 分步压平，每步读取中断标志 | v0.30.1 | [uncatchable.md](uncatchable.md#a-long-rope-is-made-flat-in-steps) |
| repeat 每 65,536 个字符读取一次中断标志 | v0.30.1 | [uncatchable.md](uncatchable.md#repeat-reads-the-interrupt-flag-every-65536-characters) |
| 编译一个很长的正则时，每个长循环都读取中断标志 | v0.30.1 | [uncatchable.md](uncatchable.md#compiling-one-long-regexp-reads-the-interrupt-flag-in-every-long-loop) |
| 正则的类不管有多少字符或字符串，都读取中断标志 | v0.30.1 | [uncatchable.md](uncatchable.md#a-class-of-a-regexp-reads-the-interrupt-flag-however-many-characters-or-strings-it-has) |

## 错误和调用栈

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 错误的 stack 在第一次读取时才格式化 | v0.26.1 | [errors-and-stacks.md](errors-and-stacks.md#error-stacks-formatted-when-first-read) |
| 无限递归抛出 RangeError | v0.26.6 | [errors-and-stacks.md](errors-and-stacks.md#endless-recursion-throws-rangeerror) |

## 时间和 Intl

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 覆盖 Chromium 全部语言的 Intl | v0.26.2 | [time-and-intl.md](time-and-intl.md#intl-in-every-locale-chromium-has) |
| 每个 VM 里本地时间都是 UTC | v0.26.3 | [time-and-intl.md](time-and-intl.md#local-time-is-utc-in-every-vm) |
| 小数位数按 ECMA-402 自 2021 年起的读法处理 | v0.26.4 | [time-and-intl.md](time-and-intl.md#fraction-digits-as-ecma-402-reads-them-since-2021) |
| 夏令时的名称和 ICU 一致 | v0.26.9 | [time-and-intl.md](time-and-intl.md#daylight-saving-time-named-as-icu-does) |
| Intl 通过 Builtin 调用 Go | v0.26.17 | [time-and-intl.md](time-and-intl.md#intl-calls-go-through-builtins) |
| 编译好的 Intl 格式化计划和共享的已解码数据 | v0.27.0 | [time-and-intl.md](time-and-intl.md#compiled-intl-formatting-plans) |
| 所有 Intl 格式化器共用 go-intl | v0.27.0 | [time-and-intl.md](time-and-intl.md#all-intl-formatters-share-go-intl) |
| 基于 go-intl 的 Intl 构造函数 | v0.27.0 | [time-and-intl.md](time-and-intl.md#intl-constructors-on-go-intl) |
| Intl 不动页面放在替身位置上的东西 | v0.27.0 | [time-and-intl.md](time-and-intl.md#stand-ins-leave-what-a-page-put-in-their-place) |
| DateTimeFormat 接受 Temporal 值并格式化范围 | v0.27.0 | [time-and-intl.md](time-and-intl.md#datetimeformat-takes-temporal-values) |
| Temporal 的 duration 使用 DurationFormat | v0.27.0 | [time-and-intl.md](time-and-intl.md#temporal-durations-use-durationformat) |
| 引擎读 Go 的时钟 | v0.27.0 | [time-and-intl.md](time-and-intl.md#the-engine-reads-gos-clocks) |
| Date、Intl 和 Temporal 每个 VM 共用一个墙上时钟，用 `VM.SetClock` 设置 | v0.28.2 | [time-and-intl.md](time-and-intl.md#one-wall-clock-per-vm) |

## Temporal

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 基于 go-intl 的 Temporal | v0.27.0 | [temporal.md](temporal.md#temporal-on-go-intl) |

## 正则表达式

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 跳到正则可能匹配的位置 | v0.26.15 | [regexp.md](regexp.md#skip-to-where-a-regexp-can-match) |
| 不会匹配的分支不入栈 | v0.26.17 | [regexp.md](regexp.md#alternatives-that-cannot-match-are-not-pushed) |
| 按 VM 保存编译好的正则 | v0.27.0 | [regexp.md](regexp.md#compiled-regexps-kept-per-vm) |
| 带 v 标志的正则的类按规范解析 | v0.28.3 | [regexp.md](regexp.md#classes-of-regexps-with-the-v-flag) |

## 字符串

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 搜索直接跳到字符串所在的位置 | v0.27.3 | [strings.md](strings.md#searches-skip-to-where-the-string-is) |

## JSON

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| JSON 成段复制普通字符 | v0.26.17 | [json.md](json.md#json-copies-runs-of-plain-characters) |

## 运算符

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 浮点数的位运算跳过慢路径 | v0.26.17 | [operators.md](operators.md#bitwise-operators-on-floats-skip-the-slow-path) |
| 常见的比较在解释器里直接回答 | v0.26.17 | [operators.md](operators.md#the-usual-comparisons-answered-in-the-interpreter) |

## 对象和 shape

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 普通对象的新属性跳过通用路径 | v0.26.13 | [objects-and-shapes.md](objects-and-shapes.md#new-properties-of-plain-objects-skip-the-generic-path) |
| 保留 shape | v0.26.16 | [objects-and-shapes.md](objects-and-shapes.md#kept-shapes) |
| for in 跳过没有可枚举属性的原型 | v0.26.17 | [objects-and-shapes.md](objects-and-shapes.md#for-in-skips-prototypes-with-no-enumerable-property) |
| 本来就是 atom 的字符串键 | v0.26.17 | [objects-and-shapes.md](objects-and-shapes.md#string-keys-that-are-already-atoms) |
| 新函数一次得到 length 和 name | v0.26.17 | [objects-and-shapes.md](objects-and-shapes.md#new-functions-get-length-and-name-at-once) |
| 字符串、数字和数组的方法在解释器里查找 | v0.27.1 | [objects-and-shapes.md](objects-and-shapes.md#methods-of-strings-numbers-and-arrays-found-in-the-interpreter) |
| 自有键从 shape 列出 | v0.28.5 | [objects-and-shapes.md](objects-and-shapes.md#own-keys-listed-from-the-shape) |
| 普通对象占 48 字节，不是 72 | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.md#ordinary-objects-take-48-bytes-not-72) |
| 对象字面量直接按最终的 shape 创建 | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.md#object-literals-made-at-their-final-shape) |
| for in 循环持有它的对象的 shape | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.md#a-for-in-loop-holds-the-shape-of-its-object) |
| hasOwnProperty.call 在解释器里直接回答 | v0.29.1 | [objects-and-shapes.md](objects-and-shapes.md#hasownpropertycall-answered-in-the-interpreter) |
| 展开、rest 和 Object.assign 在轮到每个键时才查找它 | v0.29.2 | [objects-and-shapes.md](objects-and-shapes.md#spread-rest-and-objectassign-look-each-key-up-when-its-turn-comes) |
| 对象字面量把属性放在对象自己的内存块里 | v0.29.4 | [objects-and-shapes.md](objects-and-shapes.md#object-literals-keep-their-properties-in-the-block-of-the-object) |
| 键相同的对象字面量共用一个 shape | v0.29.5 | [objects-and-shapes.md](objects-and-shapes.md#object-literals-of-the-same-keys-share-one-shape) |
| 因内存不足无法压实属性的 delete 不留下待处理的错误 | v0.29.5 | [objects-and-shapes.md](objects-and-shapes.md#object-literals-keep-their-properties-in-the-block-of-the-object) |
| 复制不了的共享 shape 让调用失败 | v0.29.9 | [objects-and-shapes.md](objects-and-shapes.md#a-shared-shape-that-cannot-be-copied-fails-the-call) |
| 内置类的对象找到它们原型的空 shape | v0.29.10 | [objects-and-shapes.md](objects-and-shapes.md#objects-of-the-built-in-classes-find-the-empty-shape-of-their-prototype) |
| 通过 shape 由哪个 shape 长出来来找到它 | v0.29.10 | [objects-and-shapes.md](objects-and-shapes.md#a-shape-is-found-by-the-shape-it-grew-from) |
| 单独死去的对象立刻释放 | v0.29.10 | [objects-and-shapes.md](objects-and-shapes.md#an-object-that-dies-alone-is-freed-at-once) |

## 内存

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 每个 VM 共用一份源码文本和行表，SharedBytecode | v0.28.1 | [memory-and-allocation.md](memory-and-allocation.md#one-copy-of-source-text-and-line-tables-for-every-vm) |
| 共享字节码只在第一次加载时扫描轮询点 | v0.29.1 | [memory-and-allocation.md](memory-and-allocation.md#shared-bytecode-is-scanned-for-the-polls-on-its-first-load-only) |

## 年轻代回收

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| 年轻代回收，VM.RunYoungGC | v0.27.3 | [young-collection.md](young-collection.zh-CN.md) |

## 弱集合

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| WeakMap 的 ephemeron 回收 | v0.27.2 | [weak-collections.md](weak-collections.zh-CN.md) |

## 引擎里去掉的东西

| 改动 | 版本 | 文档 |
| --- | --- | --- |
| quickjs-libc.c 只保留 Go API 会调用的部分 | v0.27.0 | [trimmed.md](trimmed.md#no-shell-library) |

## 新增一项改动

- 把它放进所属领域的文档，没有合适的领域就在这里新建一份文档，并在该领域的表里加一行，版本写 "Unreleased"。发布步骤（[releasing.md](../releasing.md#before-the-release)）会把这里各表以及各领域文档的 Release 行里的 "Unreleased" 换成版本号。永远不要写进 README.md，它只负责链接到这里。
- 写明它做什么、为什么安全、收益或代价是什么（连同机器）以及怎么测的，还有哪些测试检查它。
- 把它加进 [CHANGELOG.md](../../CHANGELOG.md) 和 [CHANGELOG.zh-CN.md](../../CHANGELOG.zh-CN.md)，为速度做的改动还要加进 [performance.md](../performance.zh-CN.md)。
