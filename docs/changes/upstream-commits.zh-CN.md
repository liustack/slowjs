---
summary: '在钉住的 3d5e064 之后，逐个并入 engine/ 的上游 QuickJS 提交（修正确答案、泄漏和崩溃的，Array 的 slice 和 splice，js_strict_eq2 里借用的操作数，模板字面量，unicode_script），perf 找出来的更快的 indexOf 和 includes，试过又撤回的，在 liustack/quickjs 里修好的 BigInt 比较，以及其余没有并入的，附测试和速度'
read_when:
  - 想知道 C 树有 2026-06-04 之后的哪些上游 QuickJS 提交，哪些没有
  - 改 engine/ 里的 js_array_slice、js_array_splice、js_strict_eq2、js_array_indexOf、js_array_includes、js_parse_template 或 unicode_script
  - 要把上游 QuickJS 的另一个提交并入 engine/
  - 想知道为什么某个内置函数或解析规则的行为和上游 QuickJS 的某个提交一致，或者和 Node 24 不同（Iterator helpers、Promise.try、作为名字的 yield 和 await、类元素、模块导出、Proxy trap、sort、bind、BigInt.asUintN）
  - 想看来自上游的某个 C 引擎提交的速度
---

# 并入 C 树的上游提交

[English](upstream-commits.md) · 简体中文

`engine/` 起步于 QuickJS `3d5e064`（2026-06-04），之后自己演变（[c-engine.md](c-engine.zh-CN.md)）。最初的调研覆盖了到 `19d6bd8`（2026-10-26）为止的 81 个上游提交。表里还包括 2026-10-31 推送的那批挑选出来的修复。本页列出并入的提交（每个都是单独的一个提交，并写明它对应的上游提交）、试过又撤回的，以及没有并入的。这不是跟随上游的承诺。修正确答案、泄漏或崩溃的提交，不管页面碰不碰得到，都会并入，让 tifa 的渲染更快的也是。新功能或者为速度做的改动，在 tifa 用得上时才并入。每一个都带一个测试，输出和 Node 24 的输出一致（按当前规范而不同的行，在文件里有标记），泄漏则用一次对剩余内存的扫描来检查。

上游的某个提交和 liustack/quickjs 的某项改动碰在一起时，保留我们的改动，把上游的文本合并在它周围。想知道某个提交是否仍然适用，对它的 diff 运行 `git apply --directory=engine --check`。

| 上游提交 | 做了什么 | 怎么并入的 | 测试 |
| --- | --- | --- | --- |
| `42d08be` optimized Array.prototype.slice and splice | 稠密 Array 的 slice 和 splice 直接复制或移动元素 | 全部，上游调用 `JS_CallConstructor` 的地方用 `js_call_constructor`（它会计入调用深度） | `compare/array_slice_splice.js` |
| `8f0c037` use JSValueConst arguments for js_strict_eq2 | `indexOf`、`lastIndexOf`、`includes`、`Map` 和 `Set` 不再复制和释放它们比较的值 | 除了最后一个 hunk 之外的全部，那个 hunk 改的是 liustack/quickjs 没有的宏 | `compare/strict_equal.js` |
| `f8199dc` Fix template literal substitution | 模板按规范的要求立即把每个替换值转成字符串，不再调用 `String.prototype.concat` | 全部 | `compare/template_literal.js` |
| `817509a` removed infinite loop in unicode_script() | `\p{Script=...}` 在内存耗尽的路径上返回，而不是跳转到自己 | 全部 | `unit-oom`、`test_regexp_script_oom` |
| `c17f7fc` fixed BigInt.AsUintN() | 负数 BigInt 的 `BigInt.asUintN` 只在 64 位以下才回绕 | 全部 | `compare/bigint_as_n.js`、上游的 `test_bigint.js` |
| `d88b1ed` Reject out-of-range limits in Iterator.prototype.take/drop | 大于 2**53-1 的有限上限会抛出 RangeError 并关闭迭代器 | 全部 | `compare/iterator_take_drop.js` |
| `21cbd86` added JS_ToInt64SatF() | `''.repeat(2**31)` 是空字符串，`take` 和 `drop` 把它们的上限读成饱和整数 | 合并在 `repeat` 的轮询点周围 | `compare/string_repeat.js` |
| `c8ef889` fixed memory leak in Atomics.store | 存储抛错时，转换值产生的 BigInt 没有被释放 | 全部 | `compare/atomics_store.js`（失败的存储之后的堆） |
| `5b6356a` fixed ArrayBuffer.prototype.resize | 长度用 `JS_ToIndex` 读取，所以 2**64 + 8 不等于 8 | 全部 | `compare/array_buffer_resize.js` |
| `8ac9d9b` fixed TypedArray.prototype.at | `at` 按它在转换参数之前读到的长度来计数 | 全部 | `compare/typed_array_at.js` |
| `4984249` fixed returned value of empty catch blocks | `try { 2; throw 0 } catch (e) { }` 的完成值是 undefined | 全部 | `compare/completion_values.js` |
| `abf571a` fixed 'return' invocation in Iterator helpers | helper 的 `return()` 调用它下面那个迭代器的 `return` 方法，`flatMap` 的内层迭代器的 `next` 只读一次 | 全部，用 `js_call`，另有我们自己的三处改动（见下） | `compare/iterator_helpers_return.js` |
| `cfc9464` simplified and fixed yield/await identifier parsing | `yield` 和 `await` 是名字还是关键字，由函数的上下文、它的名字和它的箭头函数参数决定 | 全部 | `compare/yield_await_names.js` |
| `df8de73` Annex B: allow call expression as assignment target | `f() = 1`、`f()++` 和 `for (f() in o)` 在非严格代码里能编译，运行时抛出 ReferenceError | 全部 | `compare/call_as_target.js` |
| `8e68fcf` spec update for Promise.try | 回调返回的 promise 原样返回，回调在使用构造函数之前运行 | 用 `js_call` 和 `js_get_exception`，它们让中断不会被捕获 | `compare/promise_try.js`（标了 ES2026 的行） |
| `1aa7429` fixed module ambiguous export test | 模块导入后又导出的绑定，就是它导入的那个 | 全部 | `compare/module_ambiguous_export.mjs` |
| `fce112d` fix super property assignment on non-extensible receiver | `super.x = v` 和 `Reflect.set` 会改动不可扩展的接收者已有的属性 | 全部，另有我们自己的一处改动（见下） | `compare/set_non_extensible.js`、`compare/array_add_index.js` |
| `13876a2` fix anonymous function name inference in for-in loop initializers | `for (var f = function () {} in o)` 把函数命名为 `f` | 全部 | `compare/for_in_initializer.js` |
| `efc48a3` fixed Function.prototype.bind prototype inheritance | 绑定函数的原型是它所绑定的函数的原型 | 全部 | `compare/bind_prototype.js` |
| `ce0dbb7` fix Proxy getOwnPropertyDescriptor invariant check | 对不可配置的属性，trap 不能报告别的值或别的访问器，目标的描述符也不再泄漏 | 全部 | `compare/proxy_own_descriptor.js`（连同其后的堆） |
| `535a7c2` respect the spec operation order in Object.defineProperties | 在第一次定义之前，先读完每个键、描述符和值，并转换每个描述符 | 全部，另有我们自己的一处改动（见下） | `compare/define_properties_order.js` |
| `55c5f0b` fixed class elements parsing after a line terminator | 换行前的 `get`、`set` 和 `static` | 全部 | `compare/class_element_names.js` |
| `79f58c1` Fix leak in JS_ReadBigInt on truncated input | 读取被截断的字节码时已经创建的 limb 会被释放 | 全部 | `unit-api`、`test_read_truncated_bigint` |
| `9a0197f` Write back every element in Array.prototype.sort | 没有移动的元素也写回 | 全部 | `compare/sort_write_back.js`、上游的 `test_builtin.js` |
| `1134912` Fix Proxy ownKeys trap accepting a primitive result | trap 返回数字或字符串是 TypeError | 全部 | `compare/proxy_own_keys_result.js` |
| `1ec58aa` Await the async iterator return() when a for-await loop breaks | 提前离开的 for await 循环会等待 `return()`，正常结束的则不调用它 | 去掉它的 `using` 分支，并加了对结果的检查 | `compare/for_await_close.js` |
| `19d6bd8` fixed arbitrary module namespace names | `export { x as "a-b" }` 和 `export * as "name" from` | 全部，test262 会运行这个特性 | `compare/module_string_names.mjs` |
| `a479eff` fixed regexp unicode set parsing | `/-&&/v` 是合法的正则 | C 树里已经有了 | 上游的 `test_builtin.js`，现在有了这一行 |
| `3c4c6ed` fixed slow Array truncation | 稀疏数组截断先删除所有符合条件的下标，再压实属性表 | 全部，保留 liustack/quickjs 对 shape（引擎记录一个对象有哪些键、按什么顺序的结构） 的处理 | `compare/array_truncate.js`、`TestUpstreamArrayTruncate` |
| `de4c9e0` fixed Iterator.prototype.reduce empty iterator closing | 没有初始累加值的空迭代器抛错，不读取也不调用 `return` | 全部 | `compare/iterator_reduce.js`、`TestUpstreamIteratorReduce` |
| `aac9bc5` fixed Iterator.concat logic | 通过 IteratorClose 关闭，并在任何一步失败后结束 helper | 合并进单独的 next 和 return 函数，return 返回一个新的已完成结果 | `compare/iterator_concat.js`、`TestUpstreamIteratorConcat` |

## Iterator 拼接

`aac9bc5` 通过 `JS_IteratorClose` 关闭当前的迭代器。没有、为 undefined 或为 null 的 return 方法都接受，返回原始值的方法抛出 TypeError，抛错的 getter 或方法仍然让 concat helper 处于已完成状态。迭代器打开失败、查找 next 失败、调用 next 失败，或者读取 done/value 失败，都会释放当前的迭代器和所有剩下的可迭代对象，既不关闭也不打开它们。每个打开的迭代器，next 方法只读一次。下标和计数用上游的无符号类型。

这个分支保留单独的 next 和 return 函数。它的 return 函数在清理之后创建一个新的 `{ value: undefined, done: true }` 结果，包括第一次调用 next 之前和迭代耗尽之后，而不是转发内层的 return 值。重入的 next 和 return 调用仍然是错误。回归测试对照相应的 [test262 concat 用例](https://github.com/tc39/test262/tree/91c70759363c46557b694792bf34a2551a7af5ba/test/built-ins/Iterator/concat)检查这些路径，脚本里写了用例名。Node 24.13.0 没有 Iterator.concat。不做性能方面的主张。

## 空迭代器的归约

`de4c9e0` 采用释放累加器和方法的异常路径，不关闭已耗尽的迭代器。抛错的归约回调仍然会关闭它的迭代器。回归测试把普通的和抛错的 return getter 与方法、初始累加值和失败的回调，与 Node 24.13.0 做对比。不做性能方面的主张。

## 数组截断

`3c4c6ed` 先准备一个私有 shape，移除属性但不移动剩下的条目，然后只压实一次。以前在删除循环里压实，会跳过一些条目，所以截断之后写一个旧下标，可能让长度不变。不可配置的下标仍然让截断停在最高的这类下标加一处。回归测试把清空、重新填充、访问器、非下标键和另一个数组，与 Node 24.13.0 做对比。

生成的 linux/amd64 引擎，把下标为 `i * 10` 的 100,000 个元素截断到零，中位数用时之前是 2.616 ms，之后是 2.170 ms（是原来的 0.830）。旧引擎留下 50,000 个属性，修好的引擎一个也没留。每个构建 21 个样本，在 GCP VM 上交替跑三批、每批七个，限定在 CPU 3 和 7 上。Go 的单调时钟包住 `Eval("a.length = 0")`，不包括构造和之后对自有键的计数。这是一个针对正确性的工作负载，不是对 tifa 渲染的测量。

## 速度

测量方法见 [performance.md](../performance.zh-CN.md#数字是怎么测的)，在 Google Cloud VM 上，用 [Code positions](../performance.zh-CN.md#代码位置) 的 8 个代码位置（整个程序移动 0 或 32 字节，解释器主体移动 0、8、16 或 24 字节，这样解释器的入口始终在它的 64 字节行上，只有其他函数随第一个移动），5 轮，每个提交和它前一个对比。当各位置的中位数偏离 1 超过 2%，而且超过基线在自己各位置上范围的一半，并且大多数位置在同一侧时，才算有变化。各阶段的行用的是 5e22e512 的生成器。

| 提交 | 脚本 | 用时 | 各位置 | 基线的范围 |
| --- | --- | ---: | --- | ---: |
| `42d08be` | `q_array_slice` | 0.076 | 0.075 到 0.086 | 0.6% |
| | `c_array_methods` | 0.994，在噪声之内 | 0.926 到 0.998 | 9.2% |
| | `r_react_ssr` | 0.998，在噪声之内 | 0.993 到 1.003 | 5.5% |
| `8f0c037` | `c_array_methods` | 0.955 | 0.926 到 0.959 | 1.7% |
| | `c_map_set_iter` | 0.980 | 0.963 到 0.986 | 2.0% |
| | `r_react_ssr` | 0.975，在噪声之内（8 个位置全部低于 1） | 0.955 到 0.990 | 5.2% |
| `f8199dc` | `c_string_methods` | 0.985，在噪声之内 | 0.977 到 1.006 | 1.8% |
| | `r_bench_table`、`r_bench_spiral` | 1.003、0.995，在噪声之内 | | 2.6%、1.0% |
| | `r_react_ssr` | 1.002，在噪声之内 | 0.998 到 1.007 | 2.7% |

`42d08be` 让 `q_array_slice`（对稠密数组循环调用 `slice`）快了 13 倍：338 ms 变成 26 ms。`r_react_ssr` 调用 slice 的次数不够多，看不出变化。

### 整个分支对比 main

全部 37 个脚本，`c4dcea91`（main）对比带有以上所有改动的分支，两者都用那个 main 的生成器，在 8 个位置上跑 2 轮：

- 37 个脚本的几何平均是 0.917（8 个位置上是 0.914 到 0.934）。去掉 `q_array_slice`（0.085）和 `c_array_methods`（0.530）之后是 0.997。
- `r_react_ssr` 是 0.977（各位置 0.961 到 1.001，main 的范围是 5.3%），在噪声之内，8 个位置里有 7 个没有变慢。其他渲染脚本：`r_bench_table` 0.991，`r_bench_spiral` 0.990，`r_shiki_regexp` 0.981。
- 按规则标为更快、但背后没有代码改动的：`c_json_stringify` 0.963 和 `c_string_concat` 0.957。`q_string_build2` 在三次运行中移到 0.91 到 0.97，位置范围是 11% 到 21%。这些运行都不执行上面哪个提交改过的代码，所以不算在它们头上：代码落在哪里会让它们变动。
- 标为更慢的：`v8_regexp` 1.024（8 个位置没有一个低于 1，main 的范围是 1.1%），之前两次运行还有 1.017 和 1.019。callgrind 数出 main 是 7,107,881,253 条指令，分支是 7,109,594,044（+0.02%），所以是代码落点的问题，而匹配器自己的位置已经固定了。
- 解释器的入口在每个位置都落在它的 64 字节行上（生成器把它放在那里）。整个程序的移动只移动其他函数，所以解释器周围代码的改动，从它那里看不出来。

## indexOf 和 includes 对大多数元素不用调用就能判断

不是来自上游。在 `c_array_methods` 上，把 perf 的结果和同一份 C 用 clang 原生构建的结果（`engine/test`）对比，差距出在 `js_array_includes` 和 `js_array_indexOf`：6020 和 3915 个样本，对 1826 和 957。它们的循环对每个元素调用 `js_strict_eq2`，而带有可能轮询的调用的循环，会把轮询计数器放在内存里：每一轮读、减、写一个字，这占了 `js_array_includes` 样本的 36%。`js_quick_strict_eq` 回答不需要调用的那些组合（int、布尔值、null、undefined、对象、symbol、浮点数、int 与浮点数、是同一个字符串或长度不同或都是 atom（引擎给属性名和常用字符串的编号） 的字符串，以及永远不相等的两种不同类型的值），`js_fast_array_find` 用它遍历稠密 Array，只对其余的调用 `js_strict_eq2`：不是 atom 的两个字符串、rope（几段字符串连成的树，读的时候才拼成一段） 和 BigInt。`lastIndexOf` 没有针对稠密 Array 的循环，没有改。`compare/strict_equal.js` 在 int、浮点数、atom、新建的字符串、混合值以及每种类型各一个的数组里搜索，带 from-index，输出和 Node 24 一致。

在分支合并后的状态（提交 `a57b9148`）上，对比加上这次改动之后的同一状态：`c_array_methods` 0.583（各位置 0.562 到 0.588，基线范围 3.9%，8 个位置全部低于 1）。`r_react_ssr` 1.001，`c_map_set_iter` 1.002，`c_string_methods` 0.987，`q_array_read` 1.003，都在噪声之内。

## 试过又撤回的

另有三个提交先并入，后来各自用一个单独的提交撤回了，因为它们的收益达不到规则的标准，其中一个还让 `r_react_ssr` 变慢。它们的测试保留。

- `088cf5e`（解释器里 `<`、`<=`、`>` 和 `>=` 处理浮点数）：`r_react_ssr` 用时是原来的 1.044，每个位置都在 1.036 到 1.051 之间，基线的范围是 2.8%。`q_float_arith` 0.999，`v8_navier_stokes` 0.978（范围 11.8%），`v8_raytrace` 1.009。这个宏让解释器变大了，而 tifa 的页面比较的是 int。
- `445624b`（`==` 和 `===` 的数字情形，合并进已经有其他情形的 `goasm_eq_fast`）：没有变化。`r_react_ssr` 0.995，`q_float_arith` 1.004，`v8_navier_stokes` 1.004，`v8_raytrace` 0.986（范围 2.0%）。上游的后续提交 `762e6fc` 修的是它新增的宏里的一个笔误，这个分支从来没有那个宏。
- `888481d`（`String.prototype.concat` 用一个缓冲区）：`c_string_methods` 0.997，`c_escape_html` 0.995，`c_string_concat` 1.019（范围 5.8%），`r_react_ssr` 1.002。tifa 的页面不调用 `concat`。

2026-10-31 的 23 个修复合在一起，对比 main（`bcd2eb1a`，带 stability 5 那一批），关掉 Go 的 GC（`GOGC=off`），在 EPYC 7B13 的一个核上、它的同伴线程空闲、位置 0.0 测量。指令数是 callgrind 的，是一个 VM 上渲染一个应用的每个页面一次的开销，这个 VM 之前每个页面渲染过一次（3 次渲染的一次运行和 1 次渲染的一次运行之差）。没有测时间。可能让速度变差的修复是 `9a0197f`（sort 的每个元素都写回）、`fce112d`（可扩展性测试往下挪了）、`abf571a` 和 `1ec58aa`。

| 应用 | Main | 这个分支 | 变化 |
| --- | ---: | ---: | ---: |
| tifa 的示例商店 | 870,152,415 | 870,432,574 | +0.032% |
| tifa 的 MUI 应用 | 694,737,147 | 691,694,215 | -0.44% |

商店、store、MUI 应用和 bench 应用共 16 个页面的 HTML 字节相同（各自匹配到的渲染时间已经屏蔽）。在 stability 5 那一批之前，对 main（`4d2cdd77`）做的第一次测量，用另一套工具，商店是 +0.0015%（902,036,110 和 902,049,510），MUI 应用是 +0.013%（706,587,440 和 706,682,625）。那次测量里，渲染 120 次之后 VM 的内存，商店（9,959,296 和 9,963,400 字节）和 bench 应用（相同）在 0.1% 以内，store 少 0.7%（13,085,392 和 12,987,992），MUI 应用多 0.4%（5,847,560 和 5,872,256）。

## 负的 BigInt 和负的浮点数

`js_bigint_float64_cmp` 比较同号的 BigInt 和浮点数的指数，指数较小的就回答"小于"，不管符号。对两个负数来说，指数较小的反而是较大的数，所以 `-0.5 < -1n` 是 true，`-2147483648 < -1n` 是 false。这个问题已经报告给上游：[bellard/quickjs#583](https://github.com/bellard/quickjs/issues/583)。只在 `engine/` 里修了。`compare/number_compare.js` 发现了它：它对 41 个数和 19 个其他值的每一对，打印全部八个比较运算符的结果，以及 `valueOf` 的调用顺序。

## 并入的提交和上游或 Node 24 不同的地方

`engine/test/compare` 下的每个文件，打印的都是 Node 24 打印的内容，只有它标出的行例外。下面是 C 树不是上游文本、也不是 Node 的答案的地方。

- `abf571a`：上游的新代码在 `flatMap` 的内层迭代器结束时和它的 `next` 抛错时关闭它，并且让抛过错的 helper 对 `return()` 仍保持打开。Node 24 两样都不做。C 树在这两种情况下不关闭内层迭代器，并把调用抛了错的 helper 标记为已完成。
- `fce112d`：上游去掉了 `JS_SetPropertyInternal` 里对可扩展性的提前检查，依靠 `JS_CreateProperty`，而它在发现数组不可扩展之前已经增大了数组的长度。于是对密封数组做 `a[10] = 1`，会让它的长度变成 11。现在 `JS_CreateProperty` 先做这项检查，这也修好了 `Reflect.defineProperty(preventedArray, 7, ...)`，它在这个提交之前是错的。
- `1ec58aa`：上游不检查 `return()` resolve 到什么。C 树按规范的要求，在它不是对象时抛出 TypeError。循环体抛错时，对一个成功的 `return()` 仍然不等待，而 Node 24 会等。
- `8e68fcf`、`d88b1ed`：当前的规范，Node 24 还没有跟上。对同一个构造函数创建的 promise，`Promise.try(() => p)` 就是 `p`。`take` 和 `drop` 里大于 2**53-1 的有限上限抛出 RangeError，而 Node 接受它。
- `df8de73`：严格模式下的代码里 `f() = 1` 是语法错误，和规范一致。V8 会编译它，运行时抛出 ReferenceError。`super() = 1` 也是语法错误。
- `1aa7429`：两条路径以同一个名字导出的同一个命名空间对象，在规范里不算有歧义，在 V8 里算。
- `535a7c2`：上游把描述符对象保留到第二遍，所以后面某个键的 getter 可以改动前面的描述符，描述符的字段在所有键之后才读，不是对象的描述符或者某个字段抛错的描述符，要在它之前的定义都做完之后才被发现。C 树在读到键时就转换对应的描述符，并用转换后的描述符来定义，和规范的要求一致，打印的内容和 Node 24 一致。
- `9a0197f`：规范会把单元素的数组写回，对冻结的数组抛错，C 树就是这样做的。V8 原样返回它。
- `21cbd86` 和 `d88b1ed` 把 `take` 和 `drop` 的上限读成饱和整数，上游说这对 2**31-1 + 0.5 这样的上限并不严格合规。

## 对程序有什么改变

- 模板字面量先把每个替换值转成字符串，再计算下一个，所以 `${c} ${c.n++}` 和被后面的替换值改动的 `Date` 打印的内容和 Node 一致。改动 `String.prototype.concat` 不再影响模板。模板里的 symbol 仍然抛出 `TypeError`。
- 带 `\p{Script=...}` 或 `\p{Script_Extensions=...}` 的正则，如果因内存不足编译不了，调用以内存耗尽的错误结束。以前它会让进程崩溃（clang 把那个死循环当作未定义行为）。
- 比较负的 BigInt 和负的浮点数，得到正确的答案。
- 负数 BigInt 的 `BigInt.asUintN` 在每个宽度都会回绕。`BigInt.asUintN(64, -1n)` 是 `2n ** 64n - 1n`。
- `''.repeat(2 ** 31)` 是空字符串。`Iterator.prototype.take` 和 `drop` 对大于 2**53-1 的有限上限抛出 RangeError 并关闭迭代器。
- 失败的 `Atomics.store` 和读取被截断的字节码不再泄漏任何东西。`ArrayBuffer.prototype.resize` 把它的长度读成下标。`TypedArray.prototype.at` 按它最先读到的长度计数。
- Iterator helper 用 `return()` 关闭它下面的迭代器。通过 `break`、`continue` 到外层循环或 `return` 离开的 for await 循环，会等待 `return()`，一直运行到结束的则不调用它。循环体抛错的循环仍然关闭迭代器，并且不等待 `return()`（和 Node 24 已知的不同之处）。
- `yield` 和 `await` 按函数的上下文是名字还是关键字，`f() = 1` 在非严格代码里能编译、运行时抛错，`get`、`set` 或 `static` 加换行之后的类元素按规范的读法来读，`export { x as "a-b" }` 被接受。
- `Promise.try` 把回调返回的 promise 原样给出。`Function.prototype.bind` 保留所绑定函数的原型。`Object.defineProperties` 在定义之前先读完所有东西。proxy 不能对不可配置的属性报告别的值或别的访问器，也不能让 `ownKeys` 返回原始值。`Array.prototype.sort` 把每个元素写回。`super.x = v` 会设置不可扩展的接收者已有的属性。
- 在密封或冻结的数组上，设置或定义超出末尾的下标时，数组保持原来的长度，`Reflect.defineProperty` 以前做不到这一点。

## 没有并入的

对照 liustack/quickjs 的树阅读每个 diff 后做的判断。

- `0808cbc`（nonextensible-applies-to-private）是一个提案，不是修复。私有字段或品牌不能加到不可扩展的对象上。Node 24 能加，所以这个提交会让 C 树和 Node 不同。以后要并入只需一个提交：它的两处检查在 `JS_DefinePrivateField` 和 `JS_AddBrand` 里，它对 `test262.conf` 的改动会打开这个特性。
- 表外这棵树用不上的修复：`cac72873`（`JS_PrintValue` 里的 Symbol 键，Go API 不调用它）、`0d9e5855`（32 位主机）、`44e85b16`（`qjs` shell 的 `os` 和 `std` 模块），以及上游的 Temporal 和 Intl 的修复（`4d56487d`、`4490e58b`、`ea3deb14`、`a37f62af`、`bcb9ad17`），liustack/quickjs 没有这些。
- liustack/quickjs 里已经修了的：`cd97c2b`（语法错误在内存耗尽时的 stack，上游 #529）。
- `4b0addb`（`JS_IteratorNext` 去掉两个没用的参数）是对签名的清理。没有试过。
- 新功能，Vite 会把它们编译掉：新的 iterator helper（`zip`、`zipKeyed`、`join`、`includes`、分块）、`Promise.allKeyed`、`Array.fromAsync`、显式资源管理、`Atomics.waitAsync`。
- tifa 用不到的嵌入方 API（`JS_To*` 的导出之类）、上游的 Temporal 和 Intl（liustack/quickjs 有 go-intl），以及上游的测试工具。
