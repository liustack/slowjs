---
summary: '运算符的改动：两个数的位运算，常见的 ===、!==、== 和 != 组合，以及普通函数的 instanceof，不再走慢路径，每项的收益，为什么安全'
read_when:
  - 位运算符或比较给出的结果和以前不一样，或者被比较的值被释放了两次
  - instanceof 给出的答案或错误和以前不一样，或者某个 Symbol.hasInstance 方法没有被调用
---

# 运算符

[English](operators.md) · 简体中文

解释器只对最简单的操作数（两个 int）自己执行运算符，其余都调用引擎的慢函数。这些改动让更多组合不用再调用它。

## 浮点数的位运算跳过慢路径

发布版本：v0.26.17。

两个数的 `&`、`|`、`^`、`<<`、`>>` 和 `>>>`，其中一个是浮点数时，不再走慢路径。原来解释器只对两个 int 自己做运算，其余都调用 `js_binary_logic_slow` 或 `js_shr_slow`，它们先按 BigInt 和对象的检查转换两个操作数，再用通用的 `ToInt32`。乘积超过 2^31 就是浮点数，所以 MUI 的 CSS-in-JS 库 emotion 的字符串哈希，几乎每个运算符都走了这条路。现在对任意两个 int 或浮点数都直接做运算，浮点数的转换方式和 `JS_ToInt32Free` 一样（位完全相同，不用 `fmod`），结果和 `JS_NewUint32` 一样存成 int，或者在 `>>>` 的结果大于 2^31 - 1 时存成浮点数。其他组合照旧走慢路径的其余部分。

为什么安全：数不持有引用，所以没有东西要释放，而且对两个数来说，慢路径别的什么也不做，因为把数转成数值是空操作，不会抛错，也不会调用脚本。解释器本身保持原样：改成让它的六个处理分支调用 `bitwiseNumbers`，商店页面反而慢了 0.8%。5 个文件都会变，因为 `bitwiseNumbers` 要读它们的 struct `JSValue`。

在 linux/amd64、不开 PGO 上，对比 v0.26.16，跑 10 轮，tifa 的 MUI 应用页面每次渲染要对浮点数做约 130,000 到 300,000 次这类运算，用时是原来的 0.952 到 0.960（0.935 到 0.969）。商店每次渲染只走 3 到 24 次慢路径，bench 应用一次也不走，所以它们没有收益：商店测得 1.008（0.999 到 1.042），这是代码布局造成的，不是工作量的变化。HTML 和 VM 的内存不变。

## 常见的比较在解释器里直接回答

发布版本：v0.26.17。

`===`、`!==`、`==` 和 `!=` 对常见的组合不再调用慢路径。原来解释器自己只比较两个 int，其余都调用 `js_strict_eq_slow` 或 `js_eq_slow`，`===` 还会接着走到 `js_strict_eq2`。React 会和 `null`、`undefined` 比较（压缩后写成 `null != x` 和 `void 0 === x`，编译器不会把它们变成 `is_null` 或 `is_undefined`），按身份比较类型、symbol 和对象，也会对字符串做 switch。解释器现在自己回答这些情况：

- 对象和 symbol 按指针比较
- `null`、`undefined` 和布尔值按标签和值比较
- 字符串：是同一个字符串、长度不同，或者都是字符串 atom（引擎给属性名和常用字符串的编号，每段文本在 atom 表里只存一份）
- 对 `===` 和 `!==`，标签不同的值，只要其中一个是 `null`、`undefined`、布尔值、对象或 symbol
- 对 `==` 和 `!=`，`null` 或 `undefined` 和任何值比较（只和 `null`、`undefined` 或 `IsHTMLDDA` 对象相等）

`==` 下其他标签不同的组合，例如 `true == 1` 或 `{} == 'x'`，要转换操作数，仍然走慢路径，数、BigInt、rope（几段字符串连成的树，读的时候才拼成一段） 和需要逐字符比较的字符串也一样。

两个都是 atom 的字符串，只有在两者都是字符串 atom 时才能这样区分：symbol 的描述是另一种类型的 atom，可能和某个字符串 atom 的文本相同。这段逻辑直接写在四个处理分支里，常见路径上不调用函数，因为 Go 不会把函数内联进解释器这么大的函数。它给 `eqFree` 传的是栈指针而不是值本身，所以解释器的栈帧变化很小：相比 v0.26.16，amd64 上多 8 字节，arm64 上少 16 字节。5 个文件都会变，因为这段逻辑要读它们的 struct `JSValue`。它读取的位字段（字符串的长度和 atom 类型、`IsHTMLDDA` 位）来自各文件的 `js_string_eq`、`__JS_FreeValueRT` 和 `JS_IsHTMLDDA`，值的布局（引用计数在指针下方多远、栈上一个值的大小、布尔值怎么存）来自它的 `JS_FreeValue` 和 `js_strict_eq_slow`。

在 linux/amd64、不开 PGO 上，对比 v0.26.16，跑 10 轮，tifa 示例商店的一次渲染用时是原来的 0.982（0.974 到 0.989），它的 bench 应用的 spiral 和 table 页面是 0.940 和 0.942（范围很宽，分别是 0.859 到 0.969 和 0.871 到 1.030），MUI 页面是 0.982 到 0.985。HTML 和 VM 的内存不变。

## instanceof 不再调用 Symbol.hasInstance

发布版本：v0.31.0。

寄存器循环里的 `x instanceof C`，在它要调用的方法就是 `Function.prototype[Symbol.hasInstance]` 时，不再调用这个 C 函数。规范的 `InstanceofOperator` 读出 `C[Symbol.hasInstance]` 再调用它，对普通函数来说这就是 `Function.prototype` 上的内置函数，调用它等于 `OrdinaryHasInstance(C, x)`。以前每次测试都要经过 `rv_obj2_slow`、`JS_IsInstanceOf`、两次属性查找（`Symbol.hasInstance` 在函数上找不到，在 `Function.prototype` 上找到，然后是 `prototype`）、为这个内置函数建一个调用帧的 `JS_CallFree`、`JS_OrdinaryIsInstanceOf`，布尔值再经过 `JS_NewBool` 和 `JS_ToBoolFree` 回来：在 amd64 上大约 475 条指令。现在 `INSTOF` 的处理分支先问 `rv_instof`，它要么就地给出答案，要么什么都不做，把测试留给原来的路径。

`rv_instof` 只在下面几条全都成立时才回答，其余情况都走原来的路径：

- 中断标志没有置位。它替换掉的那次内置函数调用，在宿主要求停止或者有不可捕获的异常待处理时，在入口就失败（一个 async 函数第一次运行时内存不足或被中断，会留下一个待处理的异常并返回它的 promise），而这个标志两者都包括。所以测试之后的语句不会运行，和以前一样。代价是一次读内存和一次分支。
- `C` 是对象，不是 exotic 对象（所以不是 proxy），不是绑定函数，并且可以调用（字节码函数，或者所属的类有调用处理函数）。
- 从 `C` 往上直到运行这段代码的 realm 的 `Function.prototype`，没有哪个对象有自己的 `Symbol.hasInstance`（数据属性、getter 或者还没生成的），也没有哪个是 exotic 对象，而且这条链确实走到了这个 `Function.prototype`。它上面的这个属性就是 realm 创建时放上去的内置函数：既不可写也不可配置，所以不能被替换或删除。脚本能让另一个方法生效的每一种办法（`static [Symbol.hasInstance]`、`Object.defineProperty`、`Reflect.defineProperty`、父类有这个方法、用 `Object.setPrototypeOf` 把 `C` 的原型换成带这个方法的对象或者 proxy、另一个 realm 的函数，它的链走到另一个 `Function.prototype`），要么在这次遍历经过的某个对象上放一个自有属性，要么在链里放一个 proxy，要么让链走不到这个 `Function.prototype`。遍历读的是测试那一刻的 shape，所以不需要维护标志，也没有要清除的东西。这个 `Function.prototype` 上的属性必须已经生成（见下文）。
- `x` 不是对象：答案是 `false`，在规范要求先做的检查之后（右边不可调用时先抛错，走原来的路径）。
- 否则 `C` 有自己的 `prototype`，它是普通数据属性（不是 getter，也不是还没生成的函数原型），值是对象，然后像 `JS_OrdinaryIsInstanceOf` 那样遍历 `x` 的链。遇到 shape 里没有原型、又是 exotic、又不是快速数组的对象（proxy，它的 `getPrototypeOf` trap 可能会运行）时，交给原来的路径。

为什么安全：这条路径上的任何东西都不会运行 JavaScript、抛错或者分配内存，所以它读了半条链之后放弃，原来的路径从头再做一遍，谁也看不出区别。每个错误（右边不可调用、`prototype` 不是对象、getter 抛错）都来自原来的路径，消息和创建它的 realm 都和以前一样。被替换掉的调用还算一层深度：少了它，通过 `instanceof` 递归的脚本至少能递归得和以前一样深。和每个处理分支一样，注入到 `INSTOF` 的抛错会在那一次运行里关掉快路径，所以注入测试仍然检查原来路径的释放。

内存也和以前一样。`Function.prototype[Symbol.hasInstance]` 和每个内置方法一样，第一次被读时才生成，而原来的路径每次测试都读它。所以这条路径只在这个属性已经生成之后才回答：一个 realm 的第一次测试走原来的路径，像以前一样把它生成出来。没有这个检查，做 `instanceof` 测试的脚本结束时会少一个函数对象，寄存器循环的 golden 记录了运行结束时运行时里的对象，会发现这一点（98 次语料运行，以及 rvgen 几乎每个种子）。

收益：在 amd64 上，走这条路径的一次 `instanceof` 在原生 C 树上大约花 95 条指令。V8 测试套件的 EarleyBoyer 每轮在 Boyer 里测试 491,887 次，在 Earley 里 9,770 次。tifa 的页面每次渲染测试 0 次（React）到 680 次（商店首页）。数字见 [performance.md](../performance.zh-CN.md#instanceof-不再调用-symbolhasinstance)。

`engine/test/unit-oom.c` 的 `test_uncatchable_instanceof` 先在一个 async 函数里留下待处理的中断和内存不足错误，再分别用对象和原始值在左边做 `instanceof` 测试：运行必须以这个不可捕获的错误结束，测试之后的语句不能运行。

测试：`engine/test/compare/instanceof.js` 打印 Node 24 打印的内容，覆盖类和子类、内置构造函数、左边是原始值和原型为 null 的对象、箭头函数、async 函数和绑定函数（也包括有自己 `prototype` 的绑定函数）、可调用的 proxy、任一边链上的 proxy、自有的或继承来的 `Symbol.hasInstance`（方法、getter、`null`、不可调用、返回非布尔值、抛错）、在两次测试之间添加又删除、加在父类上、用 `Object.setPrototypeOf` 把函数的原型换成带这个方法的对象、`null` 和 proxy、`prototype` 是 getter（改变左边的链的、自己做 `instanceof` 测试的、抛错的）、继承来的、不是对象的、还没生成的，以及规范里看得到的各步骤的顺序。`engine/test/regvm/js/o-instanceof.js` 在两个 realm 之间做同样的事：代码、右边和左边各自来自任一个 realm，链经过另一个 realm 的 `Function.prototype` 的函数，另一个 realm 的函数上的遮蔽属性，以及每个错误的消息和所属 realm。它的抛错注入 golden 有 1,107 个位置，它的 `inject-hit` 行让注入测试在没有一次运行在 `INSTOF` 抛错时失败。`rv_instof` 的每个检查都轮流去掉过一次，每次这些测试都会失败（`Symbol.hasInstance` 的遍历、绑定函数和可调用的检查、左边的 proxy、`prototype` 的值、临时值的释放，以及属性已经生成的检查，由 golden 的内存计数发现），只有两个检查例外，它们留作第二道防线。一个是右边遍历里对 exotic 对象的检查：那里的 proxy 在 shape 里没有原型，遍历无论如何都在它那里结束，而且没有别的 exotic 类会在 shape 之外给出 symbol 键的属性。另一个是 `prototype` 必须是数据属性的检查：getter 或者还没生成的原型，它们的槽位读出来永远不是对象，所以对值的检查无论如何都会把它们交给原来的路径。
