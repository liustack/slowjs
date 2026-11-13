---
summary: '对象和它们的 shape 的改动：普通对象新属性的快速路径，对象消失后仍保留的 shape，跳过没有可枚举属性的原型的 for in 循环，本来就是 atom 的字符串键，一次就放到 shape 上的新函数，在解释器里查找的字符串、数字和数组的方法，从 shape 列出的自有键，占 48 字节而不是 72 的普通对象，直接按最终 shape 创建的对象字面量，把属性放在对象自己内存块里的对象字面量，在轮到每个键时才查找它的展开、rest 和 Object.assign，持有其对象 shape 的 for in 循环，以及在解释器里直接回答的 hasOwnProperty.call，每项在内存上的收益和代价，怎样设定或关闭 shape 缓存，为什么安全'
read_when:
  - VM 的运行接近它的内存上限，想知道要不要给它一个更小的 shape 缓存
  - 调用 VM.SetShapeCache，或者想知道 DefaultShapeCache 的代价
  - 计划在属性访问或对象创建上做更多工作
  - 修改对象 rest 或展开的复制（`js_copy_data_properties`，`engine/regvm-ops-copy.h`）
---

# 对象和 shape

[English](objects-and-shapes.md) · 简体中文

shape 是一个对象的键按顺序排成的列表，键相同的对象共享它。

## 普通对象的新属性跳过通用路径

发布版本：v0.26.13。

给普通对象定义新属性的开销更小。其他情况仍走旧路径，赋值（`o.x = v`、`Object.assign`）也是。没有缓存任何东西，所以内存使用不变。在 linux/amd64（AMD EPYC）上，对象字面量的一个属性约需 55 ns，以前是 95，tifa 示例商店一个页面的服务端渲染少用约 6%，HTML 相同。

改动在 C 里（`engine/quickjs.c`），在 `JS_DefinePropertyValue` 的开头。`engine/test/compare.sh` 会运行 `testdata/objlit.js`。

## 保留 shape

发布版本：v0.26.16。

VM 在普通对象消失之后，仍保留它们的 shape。对象的 shape 是它的键按顺序排成的列表，对象字面量每加一个键就经过一个 shape。以前只有它的对象在用一个 shape 时，`add_property` 会原地增长这个 shape，所以对象有了全部的键之后，中间的 shape 就都没了，最后一个随最后一个对象消失：下一个带同样键的字面量又要把它们全部复制、增长一遍。现在下一个带同样键的字面量，在 shape 哈希表里找到它需要的每一个 shape，一个也不用新建。本条里的计时都是在 linux/amd64（AMD EPYC）上不开 PGO 测得的。一次复制要花一次分配加上到目前为止的键，而一个对象一个一个地得到别的对象都没有的键，每个键都要付出这笔开销，而且永远收不回来：这样的对象构建时间长了 22%。所以 `add_property` 找到的每个 shape 赚得一次复制，最多 64 次，没有额度时 shape 原地增长。这样中间的 shape 仍然被保留，只是晚一点：对象找到下一个被保留的 shape 时离开的那个 shape 会被保留。键是别的对象都没有的对象，现在构建时间比关掉缓存时长约 6%，键相同的对象字面量少用 30%。只有原型是 VM 的 `Object.prototype`、最多 32 个键、而且没有未注册 symbol 作为键的 shape 才会被保留，所以被保留的 shape 除了那个原型和它的键的 atom（引擎给属性名和常用字符串的编号） 之外，不会让任何东西存活，目标是 symbol 的 `WeakRef` 或 `FinalizationRegistry` 会像没有缓存时一样看到它消失。循环回收器释放的 shape 不保留，`JS_FreeRuntime` 会放掉每个被保留的 shape。这个环在运行时的状态里，紧挨着前端缓存（见 [allocator.md](../allocator.zh-CN.md)）。新 VM 保留 `DefaultShapeCache`（2048）个 shape，`VM.SetShapeCache` 可以改它，`VM.Close` 在释放 context 之前先把它关掉。

内存：商店的一个 VM 在一次 GC 之后多占 647 KiB，连前端缓存一起多占 722 KiB（8.03 MiB，以前是 7.33）。1024 个 shape 时是多占 190 KiB 和 461 到 502 KiB，4096 个 shape 加前端缓存多占 1.26 MiB。内存块在 arena 里怎么落，会让这些数字在两次运行之间相差最多 0.5 MiB，剩下的 shape 和对象是一样的。单是加载商店的 bundle 就会保留约 1250 个 shape，所以 1024 的环在第一次渲染之前就满了。被保留的 shape 计入 VM 的内存上限，分配失败时也不会放掉它们：`add_property` 里的一次分配，可能正好发生在对象要取得一个只有缓存持有的 shape 之前。一个有 32 个键、每个键约十个字符、别的对象都没有的 shape，连同它的键的 atom 约占 3 KiB，所以 2048 个这样的 shape 约占 6 MiB，它们消失之后 atom 表保持它长到的大小。接近内存上限运行的脚本，开着缓存可能内存耗尽，不开则不会：给这样的 VM 一个更小的缓存，或者不给。不开 PGO，在 linux/amd64（AMD EPYC）上，HTML 相同，单是 shape 缓存就让一次渲染快 3.6%，加上前端缓存和中断倒计数之后，一次渲染比没有这三者时少用 4.5% 的时间（六轮）。一个核的两个硬件线程上的两个 VM，每秒多渲染 2.9% 的页面。

改动在 C 里（`engine/quickjs.c`）：`JSRuntime` 的字段 `shape_cache`、`js_free_shape0` 里的 `js_shape_keep`、`add_property` 里的 `js_shape_keepable` 和 `js_shape_found`、`quickjs.h` 里的 `JS_SetShapeCache(ctx, n)`，`JS_FreeRuntime` 会放掉每个被保留的 shape。在 `JS_SetShapeCache` 打开它之前，它是关着的。环满时，`js_shape_keep` 返回它放掉的那个 shape，由 `js_free_shape0` 去释放，而不是它自己释放。原生运行器（`jsrun` 和 `run-test262`）像新 VM 一样保留 2048 个 shape。测试还检查：完整回收和年轻代回收都不动被保留的 shape，循环回收器会释放它所释放的环里的 shape，满了的环会放掉它最旧的 shape，带着被保留 shape 被释放的运行时会释放它们和它的 context，以及分配失败时环保持完整。

## for in 跳过没有可枚举属性的原型

发布版本：v0.26.17。

`for in` 循环不再为了确认每个原型没有可枚举属性，而列出它的属性名。循环用完对象自己的属性之后，引擎会检查它原型链上的对象有没有可枚举的字符串键，以决定要不要沿原型链走下去。以前它用 `JS_GetOwnPropertyNamesInternal` 来做这件事，那个函数要 malloc 一个列表，对每个名字取一个引用，最后全部再释放，每个循环、每个原型都这样，主要是 `Object.prototype`，它有几十个名字，却没有一个可枚举。

为什么安全：对不是 exotic 的对象，带这些标志的 `JS_GetOwnPropertyNamesInternal` 只数 shape 里有名字、可枚举标志、字符串键的属性，不调用类的任何方法，对它不数的东西也不会抛错，所以没有这类属性时它的列表是空的。`forInNoEnum` 对每个 exotic 对象（数组、`String` 对象、proxy 等）和每个有可枚举属性的对象（包括 symbol）都回答"否"，引擎照旧列出名字，所以原型链中 proxy 的 trap 照旧运行。唯一的区别在内存耗尽：空列表不需要内存，而引擎原来要 malloc 一项，所以没有内存时，循环现在能继续，原来会失败。它用指纹（`forInSites`）钉住 `js_for_in_prepare_prototype_chain_enum`、`JS_GetOwnPropertyNamesInternal`、`get_shape_prop` 和 `JS_FreePropertyEnum` 的文本。

在 linux/amd64、不开 PGO 上，对比 v0.26.16，跑 10 轮，tifa 的 bench 应用里会运行很多 for in 循环的 spiral 页面，渲染用时是原来的 0.983（范围很宽，0.888 到 1.041），table 页面是 0.971（0.871 到 1.035）。tifa 的示例商店几乎没变（0.996，0.987 到 1.001），MUI 页面也是。HTML 和 VM 的内存不变。

改动在 C 里（`engine/quickjs.c`）：`js_for_in_prepare_prototype_chain_enum` 先调用 `js_for_in_no_enum`。`engine/test/compare.sh` 会运行 `testdata/forin.js`。

## 本来就是 atom 的字符串键

发布版本：v0.26.17。

本来就是 atom 的字符串键开销更小。在 tifa 示例商店的页面上，`JS_ValueToAtom` 收到的字符串里有 92% 是这样的 atom。其他字符串、数和 symbol 走通用路径，数甚至不用发起这次调用。没有缓存任何东西，所以内存使用不变。

在 linux/amd64、不开 PGO（AMD EPYC 7B13，单核）上，对比 v0.26.16，微基准里用字符串键读取少用 13% 的时间，写入少用 30%，运行 `k in o` 少用 26%，`hasOwnProperty(k)` 少用 14%，商店一个页面的服务端渲染少用 1.3% 的时间（6 轮的中位数，各轮 0.9% 到 1.5%），HTML 相同。这些是在 [for in](#for-in-跳过没有可枚举属性的原型)、位运算和比较的改动之前测的（见 [operators.md](operators.zh-CN.md)）。

改动在 C 里（`engine/quickjs.c`），在 `JS_ValueToAtom` 的开头：这样的字符串返回它自己下标的 `JS_DupAtom`。`engine/test/compare.sh` 会运行 `testdata/atom_keys.js`，`engine/test/unit.c` 检查运行时由字符串创建、被当作键用了很多次的 atom，会随着最后一个持有它的值一起被释放。

## 新函数一次得到 length 和 name

发布版本：v0.26.17。

新函数得到 `length` 和 `name` 时，不经过中间的 shape。以前 `js_function_set_properties` 通过 `JS_DefinePropertyValue` 和 `add_property`，一个一个地给每个新函数这两个属性：函数从它原型的空 shape 出发，经过只有 `length` 的 shape，再到有两者的 shape。没有东西会长时间持有空 shape 或只有 `length` 的 shape，所以每个函数都要重新创建它们，过一会儿又释放（3,000 个箭头函数、方法和构造函数创建了 5,001 个 shape）。现在对新函数的四种原型（`Function.prototype` 以及 async、generator 和 async generator 函数的原型），在第一个函数经过 `add_property` 时，记住空 shape 和有两者的 shape，之后的函数用那个空 shape 时，直接放到有两者的 shape 上，再写入这两个值（同样 3,000 个函数创建了 2,000 个 shape，用于对象字面量和构造函数的 `prototype`）。

为什么安全：shape 是共享的，多于一个对象持有它时从不原地改变（`add_property` 和 `js_shape_prepare_update` 先复制它），而 `js_function_shape` 同时持有这两个，所以它们保持原样，空 shape 也不会被释放、内存也不会被别的 shape 占用。有两者的 shape 就是 `add_property` 会给出的那个，原型、键、顺序、标志和属性数组的大小都相同，name 在函数改变之前创建，所以可能在那时运行的循环回收器看到的是原样。这些 shape 是为 context 持有的，就像 context 持有它的数组和 `arguments` 对象的 shape 一样：`JS_MarkContext` 标记它们，所以循环回收器会释放一个只靠它们才活着的 context，`JS_FreeContext` 会放掉它们。其他一切（不是普通可扩展对象、或者 shape 不同的对象）走旧路径。

持有的 shape 给一个 VM 多加一两个 shape（商店的一个 VM 里是 88 到 176 字节）。不开 PGO，在 linux/amd64（AMD EPYC，单核）上，HTML 相同，tifa 示例商店的一次渲染，用时是只有[正则分支改动](regexp.zh-CN.md#不会匹配的分支不入栈)时的 0.982（10 轮的中位数，0.967 到 0.989），MUI 页面是 0.993 到 1.000，bench 页面是 0.984 和 0.995，每一侧各用一个构建。对比 `80261af`，其他地方的代码不同，商店是 0.972（0.951 到 1.003），MUI 页面是 0.990 到 1.004，bench 页面是 0.993 和 0.995。在 `de42e83` 上，3 轮给出商店 0.988，MUI 页面 0.983 到 0.997。

改动在 C 里（`engine/quickjs.c`）：`js_function_set_properties` 先调用 `js_function_shape`。那里 shape 是按 context 持有的，放在 `JSContext` 的字段 `func_shapes` 里，而不是按运行时、给创建最后一个函数的 context 持有，所以一个运行时的两个 context 不会互相抢槽位。`JS_MarkContext` 标记它们，`JS_FreeContext` 放掉它们。`engine/test/compare.sh` 会运行 `testdata/funcshape.js`，`engine/test/unit.c` 检查四种原型各自保留它们的两个 shape 并且新函数会重用它们，两个 context 各保留自己的，循环回收器会释放一个只靠它们才活着的 context，而在存活的 context 里则保留它们。

## 字符串、数字和数组的方法在解释器里查找

发布版本：v0.27.1。

`s.charCodeAt(i)`、`n.toFixed(2)` 和 `a.push(x)` 在解释器里找到它们的方法。`get_field` 和 `get_field2`（`o.x` 和 `o.x(...)` 的 opcode）在自己的循环里，在对象和它的原型上找键，其他一切都交给 `JS_GetPropertyInternal`：每个不是对象的值，以及每个有 exotic 行为的对象，包括数组。tifa 的 MUI 页面的 CSS-in-JS 代码，在渲染订单列表时读字符串方法约 320000 次，tifa 的两个应用也都会读几千次数组方法。其他值或对象像以前一样走 `JS_GetPropertyInternal`。

为什么安全：这些键正是 `JS_GetPropertyInternal` 自己也直接去找原型的键。字符串自己只有它的下标和 `length`，数字什么都没有。快速数组的元素放在下标上，类型化数组对规范形式是数字的键（如 `Infinity`）也回答 `undefined`，所以类型化数组仍走旧路径。值留在解释器的栈上，所以链上更远处的 getter、proxy 或其他 exotic 对象，仍然带着这个值作为 `this` 走慢路径，后来加到原型上的方法或 getter，也能在它所在的位置被找到。没有缓存任何东西，所以内存使用不变。

不开 PGO，在 linux/amd64（AMD EPYC 7B13，单核）上，对比 v0.27.0，HTML 相同，tifa 测试应用的四个 MUI 页面渲染用时是原来的 0.975（3 种代码布局跑 6 轮，0.966 到 0.986），商店的六个页面是 0.996（0.992 到 0.998）。在 callgrind 下，引擎在 MUI 页面上少执行 1.0% 的指令，在商店上少执行不到 0.1%。时间降得比指令数多：`JS_GetPropertyInternal` 和它的属性查找大多在等内存。

测试：`fieldchain_test.go`，它的 `testdata/field_chain.js` 在字符串、rope（几段字符串连成的树，读的时候才拼成一段）、数字和数组上读取方法、不存在的键和后来加上的键，带上非严格和严格模式下的 getter，只读键，`String.prototype` 上方的 proxy 和被冻结的对象，被冻结、被密封、稀疏的和类型化的数组，`arguments`，String 对象，symbol，`with` 和 `eval`，并对照 Node，还检查这类 getter 或方法里的中断或内存耗尽不能被捕获。

## 自有键从 shape 列出

发布版本：v0.28.5。

列出普通对象自有键的开销更小。展开和 rest（`JS_CopyDataProperties`）以及 `Object.keys`、`values` 和 `entries`（`JS_GetOwnPropertyNames2`）以前要为读取值而把每个键再查找一遍。React 的 `jsx` 几乎在每个元素里都展开和复制 props。对不是 exotic 的对象，它们现在在 Go 里按引擎的顺序（数组下标按值，然后是字符串，然后是 symbol 和私有名字，每类按 shape 顺序）从 shape 列出键，展开、rest、`values` 和 `entries` 直接从对象读取普通数据属性的值。把键全是可枚举、可写、可配置的字符串然后是 symbol 的对象，展开到一个空的对象字面量里，并且原型相同时，新对象得到源对象的 shape，这正是一个一个定义键最终会到达的 shape，然后复制值。

shape 的各个键的种类保存在运行时的键缓存里，256 项，每项是最多 24 个键的种类，每个 VM 约 8 KiB，这样下次列出同一 shape 的对象时，不用读任何 atom。它和被放弃的 [shape 步骤表](../performance.zh-CN.md#试过但没做的) 不同：那张表会让 shape 一直活着，每个 VM 多占 28 KB 到 142 KB，还会改变循环回收器的运行时机。键缓存不持有 shape、atom 或对象，每个键只有一个字节。它只为 shape 哈希表里的 shape 保存条目，shape 离开哈希表时丢掉它的条目（`js_shape_hash_unlink` 调用 `ownKeyCacheDrop`）。哈希表里的 shape 从不原地改变：加键之前（`add_shape_property`）、删键或改变其标志之前（`js_shape_prepare_update`）以及释放之前（`js_free_shape0`），它都会先离开哈希表。所以缓存里某个 shape 的种类就是它的键的种类，它的内存也不会被别的 shape 占去。标志在每次列出时从 shape 读取。

为什么安全：对不是 exotic 的对象，`JS_GetOwnPropertyNamesInternal` 只读 shape，不调用类的任何方法，只会在遇到尚未初始化的模块命名空间绑定时抛错，用 `JS_GPN_ENUM_ONLY` 或 `JS_GPN_SET_ENUM` 列出时才会这样。`ownKeys` 把这样的绑定，以及数组下标多于 16 个的对象，留给引擎处理。Exotic 对象（数组、`arguments`、`String` 对象、类型化数组、proxy、模块命名空间）走旧路径，所以 proxy 的 trap 照旧运行。只有列出的每个键都是普通数据属性时，才直接读取值：这时在列出和读取之间，没有任何会改变对象的东西在运行。有 getter、setter 或绑定的对象，或者有 `stack` 键的对象（它可能装着还没格式化的[错误 stack](errors-and-stacks.zh-CN.md#错误的-stack-在第一次读取时才格式化) 的标记），对它的值走引擎自己的路径。`Object.assign` 是设置而不是定义，列出之后保持它自己的路径。上游和 Node 不同的地方保持不变：展开一个字符串原始值什么也不复制。

在 linux/amd64、不开 PGO（AMD EPYC 7B13，单核）上，对比 v0.28.2，HTML 相同，tifa 示例商店的六个 bench 页面渲染用时是原来的 0.990（3 种代码布局跑 6 轮，0.958 到 1.020），它的十个 store 页面是 0.994（0.952 到 1.036），tifa 测试应用的四个 MUI 页面是 0.987（0.957 到 1.047）。收益取决于代码的布局：一种布局下是 0.976 到 0.978，另外两种是 0.991 到 1.006。一次渲染里 99.9% 的列出走新路径，每次约四个键，约四分之一的展开取得了源对象的 shape。在 Apple M4 上，展开六个键到新字面量用 74 ns，以前是 250，`{...a, ...b}` 是 155，以前是 372，解构的 rest 是 239，以前是 316，六个键的 `Object.keys` 是 134，以前是 542，`Object.values` 是 128，以前是 567，`Object.entries` 是 529，以前是 1452，单独的 `for in` 是 261，以前是 291。键缓存给一个 VM 增加 8 KiB。

测试：`own_keys_test.go`，它的 `testdata/own_keys.js` 用各种方式列出各种对象（整数键、symbol、私有名字、隐藏键、访问器、被删除的键、被改变的标志、被冻结的、被密封的和原型为 null 的对象、原型链、exotic 对象、在展开期间改变对象的 getter、对象在变化时的 `for in`、共享或离开一个 shape 的对象、120 个随机对象），它的 `testdata/own_keys.golden` 是改动之前引擎给出的结果，另外还有模块命名空间在绑定设置前后的情形。

改动在 C 里（`engine/quickjs.c`）：`JS_GetOwnPropertyNamesInternal`、`JS_CopyDataProperties` 和 `JS_GetOwnPropertyNames2` 先调用 `js_own_keys`、`js_copy_data` 和 `js_own_keys_array`，键缓存是 `JSRuntime` 的字段 `own_key_cache`，`js_shape_hash_unlink` 把一个 shape 从缓存里去掉。列出的键用 `js_own_keys_next` 遍历，而不是用回调。rest 里被排除的键，像引擎一样用 `js_get_own_property_flags` 查找。`engine/test/compare.sh` 会运行 `testdata/own_keys.js` 和 `engine/test/compare/ownkeys.js`，后者通过每条路径列出许多种 shape 的对象，这些对象的创建和丢弃让新 shape 占用已释放 shape 的内存，并把每次列出的结果和它根据所加的键推算出来的顺序对比。它还在键缓存有了某个 shape 之后列出一个对象，同时加键、删键、把键变成不可枚举、把键变成 getter 以及改变原型。`engine/test/unit.c` 检查新对象的列出结果，这些对象的 shape 占用了循环回收器释放的 shape 的内存。

## 普通对象占 48 字节，不是 72

发布版本：v0.29.1。

普通对象，即 `Object` 类的对象（对象字面量、`{}`、`Object.create(p)`、普通类的实例、React 元素或它的 props），占一个 48 字节的块，以前每个对象占 72 字节。`JSObject` 是一个 40 字节的头（回收器的链接、标志、shape 和属性数组），加一个 24 字节的联合体，装这个类的数据：数组的元素、函数的闭包、`Map` 的状态。普通对象没有这类数据，所以这个联合体白白占着空间。`JS_NewObjectFromShape` 现在对 `Object` 类只分配头，不写联合体，`free_object` 把它的保险性写入留给其他类。

为什么安全：联合体是按类读写的。唯一一个会查看每个对象的 switch，即 `JS_ComputeMemoryUsage`，现在为 `Object` 类加了一个不读任何东西的 case。公开的 getter `JS_GetOpaque`、`JS_GetOpaque2` 和 `JS_GetAnyOpaque` 对普通对象返回 NULL，和以前一样，因为从来没有东西设置过它的 opaque 指针。对普通对象调用 `JS_SetOpaque` 会中止进程，和引擎对调用者不该做的调用的处理一样，而不是越过块的末尾去写。引擎从不这样调用，Go API 更是完全不调用它。`JS_ComputeMemoryUsage` 在 `obj_size` 里按更小的块计数。

内存，在 C 树原生构建上（Apple M4，分配器和标志与生成的引擎相同），对比 v0.29.0，HTML 相同：

- 渲染 11 个 300 到 500 KB 的 Markdown 页面（react-markdown 加 remark-gfm 和 rehype-highlight）时 `MemorySize` 的最高值，平均低 3.8%（从低 11.8% 到高 2.7%：峰值正好在回收器的阈值处，即上次回收后大小的 1.5 倍，所以一个页面可能恰好落在一次回收之前或之后）。400 KB 的页面，峰值是 180.8 MB，之后是 181.1 MB。
- 关掉回收器，这样没有回收来移动终点，同一个页面渲染结束时持有 394.3 MB，之后是 379.9 MB（少 3.7%，14.4 MB）。
- tifa 示例商店的 20 个页面，峰值合计是 253.5 MB，之后是 240.9 MB（少 5.0%），结束时是 244.3 MB，关掉回收器之后是 240.0 MB（少 1.8%）。

在 linux/amd64（AMD EPYC 7B13，单核，8 个代码位置）上的速度：没有变化。React 页面、bench、MUI 和商店页面用时是原来的 0.993 到 1.004，引擎在创建和释放对象时因为多了一次测试，多执行 0.01% 到 0.27% 的指令（表见 [performance.md](../performance.zh-CN.md#48-字节的普通对象)）。

测试：`engine/test/unit-api.c` 运行 300 个有 70 个属性的普通对象，带空洞、被冻结、被密封、带访问器，作为原型，作为 `WeakMap` 和 `WeakSet` 的键，在 `WeakRef` 和 `FinalizationRegistry` 之下，以及处在环里，然后运行 `JS_ComputeMemoryUsage` 和那些 getter。在使用系统 malloc 的构建上（`build.sh asan`），每个块单独存在，带红区，所以越过 40 字节的读或写都会被报告：第一次尝试只用更小的块，就在 `JS_NewObjectFromShape` 里被抓到，然后是 `free_object`、`JS_ComputeMemoryUsage` 和 `JS_GetOpaque`。`engine/test/unit-white.c` 检查普通对象的块比一个 `JSObject` 小，数组的块则装得下整个 `JSObject`。`engine/test/api-test.c` 检查对普通对象调用 `JS_SetOpaque` 会在 host 听到之后以 `SIGABRT` 结束进程。

改动在 C 里（`engine/quickjs.c`）：`JS_NewObjectFromShape`、`free_object`、`JS_ComputeMemoryUsage`、`JS_SetOpaque`、`JS_GetOpaque` 和 `JS_GetAnyOpaque`，以及 `quickjs.h` 里 `JS_SetOpaque` 处的一条注释。

## 展开、rest 和 Object.assign 在轮到每个键时才查找它

发布版本：v0.29.2。

`JS_CopyDataProperties` 是 `{...o}`、解构的 rest 和 `Object.assign` 背后的函数，以前它一次性列出源对象可枚举的自有键，然后对每个键用 `JS_GetProperty` 读取。规范是列出所有自有键，然后在轮到每个键时才查找它（先 `[[GetOwnProperty]]`，再 `Get`），所以删掉了后面某个键、或者让它变得不可枚举的 getter，会让这个键被排除在外，而让不可枚举的键变得可枚举的 getter，会让它被复制。这里已经没有的键仍然被复制，什么都没剩下时复制成 `undefined`，或者复制成原型上同名属性的值，而变得可枚举的键被排除在外。`var o = { get a() { delete this.b; return 1 }, b: 2, c: 3 }; Object.keys({...o})` 给出 `a,b,c`，而 Node 给出 `a,c`。

现在列出所有自有键，不管是否可枚举，并且每个键在读它的值之前都再查找一次：普通对象通过它的 shape 用 `find_own_property`，开销是一次哈希查找，exotic 对象（数组、类型化数组、String 对象、`arguments`、proxy）通过 `js_get_own_property_flags`，proxy 的键本来就是这样处理的。模块命名空间保持原样：它用 `JS_GPN_ENUM_ONLY` 列出，在遇到尚未初始化的绑定时就在那里抛错，它没有 getter，它的第一个键也不会再查找一次。`js_copy_data` 是源对象的键全是普通数据属性时走的路径，不运行任何 getter，保持原样。添加键的 getter 添加的键仍然被排除在外，因为键是先列出来的。

测试：`engine/test/compare/copy_data.js` 对照 Node 24：在普通对象、数组、类实例和有 30 个键的源对象里，删除、改变或添加键的 getter，带有被删除的键的原型，抛错的 getter，trap 按规范顺序运行的 proxy，让不可枚举的键、symbol 或下标变得可枚举的 getter，排除了被 getter 改变的键的 rest，以及对每一种情形做这三种操作。结果的键用 `Reflect.ownKeys` 列出，因为 JSON 会略去 symbol。`testdata/own_keys.golden` 里有变化中的源对象的展开、rest 和 `Object.assign` 这三行，是 Node 打印的样子。

## 对象字面量直接按最终的 shape 创建

发布版本：v0.29.1。

只含普通键的对象字面量一次得到它所有的键。编译器原来先发出 `OP_object`，然后每个键发出一个 `OP_define_field`，每个键都要经过 `add_property`，在 shape 哈希表里查找下一个 shape，并且把属性数组从 2 个槽增长到 4 个，再到 8 个。现在字面量里一串键，只要它们是普通名字或字符串，后面跟着 `:` 和一个值，或者写成简写，就编译成这些值加一个 `OP_object_lit`。它的操作数是值的个数和这串键在函数里的下标。函数的字节码保存每一串键，以及这一串第一次执行之后它最终得到的 shape。之后的对象直接取这个 shape 和刚好这么大的属性数组，并按下标取得它们的值。React 元素和它们的 props 就是这样的字面量，markdown 解析器创建的大多数对象也是。

一串键在以下位置结束：计算出来的键、展开、方法、getter 或 setter、`__proto__`、是不超过 2^31 - 1 的整数的键（引擎把它当作数而不是字符串保存）、字面量里已有的键，或者 32 个键之后。一个函数里这样的字面量超过 65535 个之后，其余的按旧办法创建。中断之前的键是一个 `OP_object_lit`，其余走旧路径，所以 `{ a, b, ...c, d }` 一次创建前两个键，在展开之后定义第三个。一开始就没有普通键的字面量仍是 `OP_object`。

为什么安全：规范（对象初始化器那一节里的 PropertyDefinitionEvaluation）是先创建对象，然后对每个属性，计算它的键和值并定义它。键是普通键时，没有什么需要计算键，也没有哪个值表达式能碰到这个对象：只有方法或访问器能通过它们的 home object 碰到，而这两种会结束这一串键。所以在值之后创建对象，结果相同，键相同、顺序相同，每个都可枚举、可写、可配置，函数名也相同。抛错的值在对象还不存在时就抛了，脚本分辨不出来。shape 带着函数所在 realm 的 `Object.prototype` 在 shape 哈希表里查找，或者放进去，就像 `add_property` 会找到它一样，所以 `OP_object_lit` 创建的对象和一个键一个键构建的对象有相同的 shape，之后的键也走同样的路。函数的 realm 从不改变，它的字面量的原型也不改变。被多于一个对象持有的 shape 从不原地增长，所以函数持有的 shape 保持创建时的样子。因此该字面量的对象之后得到新键时会复制这个 shape，而 `add_property` 现在在链入这样的副本时会增大 shape 哈希表，和 `js_new_shape2` 一样：哈希表原来只在新建 shape 时增长，所以有很多副本时，它会超过一半满，只能一次长一级来追赶。循环回收器标记字节码持有的 shape（它们持有 `Object.prototype`，并通过它持有 realm），释放函数就会释放它们。

字节码版本从 5 变成 6：读取器在每个函数的闭包变量之后读取键，检查计数（最多 65535 串，每串 1 到 32 个键，一串里没有重复的键），并拒绝下标或计数与它读到的键不符的 `OP_object_lit`（`SyntaxError: invalid object literal`）。共享字节码时，每个 VM 读取自己的函数，持有自己的 shape，所以不需要按运行时或按 context 的表。

在 linux/amd64（AMD EPYC 7B13，单核，它的同伴线程空闲）上，对比改动之前的引擎，在 8 个代码位置上跑 4 轮，HTML 相同：tifa 示例商店的六个工作台页面渲染用时是原来的 0.956（各位置 0.935 到 0.963），tifa 测试应用的三个 MUI 页面是 0.969（0.948 到 0.976），它的 bench 应用的 table 页面是 0.942（0.914 到 0.950），它的 spiral 页面（0.989，指令少 3.5%）和 `r_react_ssr`（0.991，指令少 3.1%）按规则看没有变化。商店的一个 VM 在第一次渲染之后多占约 0.17 MiB，用于它的字面量的键和它们的 shape，而一个很长的 markdown 页面的峰值少 1.6%，因为它的对象没有多余的槽。其余见 [performance.md](../performance.zh-CN.md#一次建成最终-shape-的对象字面量)。

改动在 C 里（`engine/quickjs.c`、`engine/quickjs-opcode.h`）：`js_parse_object_literal` 收集键，`emit_object_lit` 发出这个 opcode，`JSFunctionBytecode` 有 `object_lits` 和 `object_lit_count`，`js_new_object_lit` 和 `js_object_lit_shape` 创建对象，`mark_children` 标记这些 shape，`free_function_bytecode` 释放它们，`JS_WriteFunctionTag` 和 `bc_read_object_lits` 写入和读取键，`add_property` 在链入复制出来的 shape 之前增大 shape 哈希表（`js_shape_keepable` 以前只对它自己的副本这样做）。`engine/test/compare.sh` 会运行 `engine/test/compare/object_literal.js`，它的文件就是 Node 24 打印的内容：键和值的计算顺序、重复的键、下标键、各种形式的 `__proto__`、计算出来的键、展开、方法、getter 和 setter、作为键的 `get`、`set` 和 `async`、函数名、抛错的值、用到周围变量或同一字面量之前创建的对象的值、32、33 和 45 个键、一个字面量数超过 16 位能表示的函数、解构、`Object.prototype` 上的 setter，以及创建后丢弃的函数的字面量让堆里仍在使用的块保持不变。`engine/test/unit.c` 检查字面量得到刚好这么大的属性数组，`engine/test/unit-white.c` 检查它的字节码能读回来并运行，但改动了 `OP_object_lit` 的操作数后就不行。`internal/cengine` 的 `TestShapeCache` 检查，几轮字面量的对象各自得到一个独有的键，在第四轮之后 VM 的内存保持不变。

## 对象字面量把属性放在对象自己的内存块里

发布版本：v0.29.4。

`OP_object_lit` 创建的对象和它的属性数组是一个块，而不是两个。属性占据对象块的末尾，也就是其他类的数据（数组的元素、函数的闭包）会放的地方，`p->prop` 指向那里。这在创建对象时省一次分配器调用，释放时省另一次，还省掉第二个块的 8 字节头。React 元素和 markdown 解析器创建的大多数对象就是这样的对象。

当整个块仍是分配器的一个小块（最多 504 字节：40 字节的头和最多 29 个 16 字节的属性，所以有 30 个键的字面量照旧创建）时，对象得到它的 shape 所具有的空间（`prop_size` 个属性），和它在单独的块里时一样。`js_props_inline(p)` 区分这两种情形：`p->prop` 是不是 `p` 的块里 `offsetof(JSObject, u)` 处的地址。单独的块永远不可能有这个地址，因为它在 `p` 的块之内。每个分配、调整大小或释放这个数组的地方都要看它：

- `free_object` 只在数组是单独的块时才释放它。
- `js_resize_prop` 给 `resize_properties`、`add_property` 里找到 shape 的分支和 `js_copy_shape` 它们所要的数组。在对象的块里的数组，会被复制到一个新的单独的块，它留下的空间不再使用。其他的用 `js_realloc` 调整大小。没有内存时它返回 NULL，对象保持原样，所以添加失败的键不会让对象损坏。
- `compact_properties` 原地把属性往上移，保持对象的块不变，因为它没法还回块的一部分。压实是可选的：当新 shape（或更小的属性数组）的内存耗尽时，`delete_property` 仍然会删除，对象保留被删除的槽留待以后再试，也不留下待处理的错误。`compact_properties` 用 `js_malloc_rt` 和 `js_realloc_rt` 取内存，它们不抛任何异常，而 `delete_property` 有意忽略它的结果。以前它会忽略已经抛出的 `-1`，所以一次成功的删除之后，"out of memory" 错误仍然处于待处理状态。
- `JS_ComputeMemoryUsage` 对这样的数组不计单独的块，仍然计它在 `prop_size` 里的空间。它的各项（`obj_size`、`prop_size`、`memory_used_size`）描述的是存活的对象和它们的 shape 所具有的空间，而不是它们保留的每一个字节：属性移出之后对象块里剩下的空间，以及压实被删除的键后留下的尾部，不在任何一项里。`MallocSize`（`malloc_size`）是分配器持有的量，和以前相同，也是内存上限和回收器用的数字。
- 其他改变对象的操作都让数组留在原处：标志（`Object.freeze`、`defineProperty`）、原型（在 shape 里原地设置）、删除键，以及 shape 离开 shape 哈希表。shape 的属性数从不多于块里的空间，因为 `prop_size` 不同的 shape 只能通过 `js_resize_prop` 到达。
- `js_copy_props`（`RegisterCopyProps`）只读配置的 `prop`，并用 `add_property` 给新的 props 加键，所以它自己不需要任何东西。

对象在键多于块里的空间时留在块里的空间，在对象消失之前都浪费了。所以当一个把属性放在自己块里的对象得到一个键时，它的 shape 会记下这一点（`inline_grew`，在 `JSShape` 的填充位里），然后 `OP_object_lit` 对这个 shape 的下一批对象，像以前一样把数组分开放。这样事后才填充的字面量（`const o = { a, b }; o.c = 1`）对每个 shape 只浪费一次空间。没有这个标志，商店的 store 页面渲染之后，VM 比改动之前多占 0.7%。

在 linux/amd64 上，对比 main，没有哪组页面按规则算更快或更慢（商店 0.991，MUI 页面 0.998，bench 应用 0.985，一次 React 渲染 0.995），一次渲染的指令少 0.14% 到 0.82%，渲染并回收一次之后 VM 少占 1.0% 到 2.4%（表见 [performance.md](../performance.zh-CN.md#属性放在对象自己块里的对象字面量)）。

改动在 C 里（`engine/quickjs.c`）：`js_new_object_lit` 和 `js_new_object_inline` 创建对象，`js_props_inline`、`js_resize_prop`、`free_object`、`compact_properties`、`add_property`、`js_copy_shape`、`resize_properties` 和 `JS_ComputeMemoryUsage` 认得这两种情形，`JSShape` 有 `inline_grew`。

测试：`engine/test/unit.c` 里的 `test_object_lit_one_block` 检查 1000 个有四个键的字面量占用的内存，比同样的对象一个键一个键构建的更少，对象在增长、压实、冻结和换新原型之后仍保有它们的键，而且一切都被还回去。`engine/test/unit-white.c` 里的 `test_object_inline_props` 和 `test_object_inline_oom` 按地址检查哪些对象把属性放在块里（字面量是，`{}`、数组和 `Object.create` 不是），添加键、把键删到压实后的 shape、冻结、密封、换新原型、访问器和标志，都让它们留在该在的地方，环里的、在 `WeakRef` 之下的和作为 `WeakMap` 键的对象都被释放，`inline_grew` 关掉了该字面量下一批对象的空间，而且只对它关，以及，让分配器的每次调用依次失败，同时让对象在 shape 哈希表里得到一个键、增长到超出它的空间、压实它被删除的键，或者创建一个字面量的第一个对象，检查它保有它的键，第一次运行失败不影响下一次运行，而且没有任何泄漏。成功的删除却留下待处理的错误，会让测试失败。抛错的检查算作失败的检查。`test_object_inline_limit` 检查有 27、28 和 29 个键的字面量把属性放在它的块里，而有 30 和 31 个键的不会。它们也在系统 malloc 上的 AddressSanitizer 下运行，那里每个块单独存在，带红区。

## 键相同的对象字面量共用一个 shape

发布版本：v0.29.5。

`OP_object_lit` 第一次运行时，在 shape 哈希表里找一个带有它字面量的键的 shape，这样写在不同地方、键相同的字面量，就和彼此以及和一个键一个键构建的对象共用一个 shape。它原来是从 `Object.prototype` 的空 shape 出发，一次一个键地走过去，而那个空 shape 常常已经没有了：第一个得到键的对象会原地增长它，因为没有别的对象持有它。这样走过去就什么也找不到，每个写了这种字面量的地方都创建了自己的 shape。在 tifa 的示例商店里，4203 个 shape 中有 588 个是这样来的。

现在按最终的哈希来查找 shape。shape 的哈希是从原型开始一个键一个键地构建的（先 `shape_initial_hash`，再对每个键和它的标志做 `shape_hash`，和 `add_shape_property` 一样），所以字面量的键的哈希不需要中间的 shape 就能知道。找到的 shape 必须有相同的原型、刚好这么多键，以及按相同顺序排列的相同的键，每个都带 `OP_define_field` 的标志（`JS_PROP_C_W_E`）。它的空间可以比字面量的键数更大（shape 的空间从 2 增长到 3、4、6 和 9），对象于是得到这么大的空间，和一个键一个键构建的对象一样。它也可能表示它的对象超出了它们的块（`inline_grew`），那么这个字面量的下一批对象就把属性分开放，和字面量自己创建的 shape 一样。

为什么安全：一个键一个键构建的对象，只要到达同样的键、同样的顺序、同样的标志和原型，就以同样的方式得到哈希表里找到的 shape（每个键处的 `find_hashed_shape_prop`），所以共享只是对字面量做 `add_property` 本来就做的事。shape 只在一个对象持有它时才原地改变，而且是在它离开哈希表之后，字面量持有它找到的 shape，所以字面量的对象得到一个键时会复制这个 shape，和以前一样。

tifa 页面的一个 VM 在一次渲染之后少持有 12% 到 15% 的 shape（商店是 3615 个，以前是 4203，store 页面是 4403，以前是 5210，MUI 页面是 3425，以前是 3896，bench 页面是 928，以前是 940），一次渲染之后它的 `MallocSize` 在商店和 MUI 页面上低 0.7%，在 store 页面上低 1.7%（66 KB、201 KB 和 41 KB），bench 页面相同。一次渲染的指令数不变（linux/amd64 上的 callgrind：商店 -0.13%，store 页面 -0.03%，MUI 页面 +0.01%，bench 页面 +0.02%，`r_react_ssr` +0.03%），16 个页面的 HTML 相同。

改动在 C 里（`engine/quickjs.c`）：`js_object_lit_shape` 调用 `find_hashed_shape_keys`。`engine/test/unit-white.c` 里的 `test_object_lit_shared_shape`，在 shape 缓存开着和关着两种情况下检查：两个函数里各有一个键的字面量，以及三个地方（其中一个在 `eval` 里）各有三个键的字面量，共用一个 shape，键的顺序不同或某个键的标志不同会得到另一个 shape，字面量会取一个能装 6 个属性的、一个键一个键构建的对象的 shape 并保留那么大的空间，以及一个 shape 的对象超出了它们的块之后，另一个键相同的字面量的下一批对象会把属性分开放。没有这个改动，它的 8 项检查会失败。

## for in 循环持有它的对象的 shape

发布版本：v0.29.1。

`for in` 循环在开始时用 `JS_GetOwnPropertyNamesInternal` 列出它的对象的自有键：一个 malloc 出来的列表，带着对每个键的引用，在循环结束时释放。React 和它周围的库，对大多数元素的 props 和许多样式对象都会运行这样的循环。现在，对于不是 exotic、shape 在 shape 哈希表里、字符串键里没有数组下标和模块命名空间的绑定的对象，循环改为取一个对 shape 的引用，原地遍历它的键，略去被删除的键、symbol 和循环开始时不可枚举的键。只要对象还是这个 shape，它就有这个 shape 的每个键，循环不用再查找就给出每个键。对象有了另一个 shape 时，循环像以前一样逐个查找每个键。

为什么安全：哈希表里的 shape 从不原地改变。在添加键、删除键或改变标志之前它会先离开哈希表，而且循环持有第二个引用期间，对象得到的是要改动的副本（`add_property`、`js_shape_prepare_update`）。所以循环持有的 shape 保持循环开始时对象所具有的键和标志，顺序就是 `JS_GetOwnPropertyNamesInternal` 给出的（没有数组下标时，字符串键按 shape 的顺序）。循环期间被删除的键不给出，添加的键不给出，变成不可枚举的键仍然给出，和以前一样。原型上有可枚举的键时，循环把这个 shape 的键作为已访问的键，和它原来处理列表时一样。数组、`String` 对象、`arguments`、proxy 和其他 exotic 对象，带数组下标键或绑定的对象，以及 shape 不在哈希表里的对象，走旧路径。循环回收器标记循环持有的 shape。

循环不再持有列表。1000 个停在一个循环里的 generator，循环遍历一个有 40 个键的对象时，每个比遍历只有 1 个键的对象多持有 344 字节，现在持有的一样多。在商店的工作台页面里，列出键、检查原型和释放循环，每次渲染少约 1.7 M 条指令，约 1%。

改动在 C 里（`engine/quickjs.c`）：`JSForInIterator` 有字段 `sh`，由 `build_for_in_iterator` 设置，`js_for_in_next` 读取，`js_for_in_prepare_prototype_chain_enum`、迭代器的 finalizer 和标记都认得它。`engine/test/compare.sh` 会运行 `engine/test/compare/for_in_has_own.js`，它的文件是改动之前引擎打印的内容：对各种对象的循环（原型为 null、数组下标键、symbol、不可枚举的键、getter、被冻结和被密封的、40 个键、数组、`String` 对象、`arguments`、proxy 以及作为原型的 proxy、类实例、函数、原始值），循环期间添加、删除、删除后再添加、变得可枚举或不可枚举的键，循环期间被设置或得到键的原型，`Object.prototype` 上的可枚举键，嵌套的循环，被 throw 或 `break` 离开的循环，循环期间被冻结、被密封、被清空大部分键的对象，以及停在 generator 里跨越多次回收的循环。`engine/test/unit-api.c` 检查一个停下的循环持有多少内存。

## hasOwnProperty.call 在解释器里直接回答

发布版本：v0.29.1。

React 和它周围的库用 `Object.prototype.hasOwnProperty.call(obj, key)` 来测试它们 `for in` 循环里的键。这是两次内置函数的调用，先 `call` 再 `hasOwnProperty`，每次都经过 `JS_CallInternal` 和 `js_call_c_function`。现在当被调用的函数是 `Function.prototype.call`，它的 `this` 是 `hasOwnProperty`（内置的 C 函数，任何 realm 的都行），对象不是 exotic，键是一个本身就是 atom 的字符串或一个是数组下标的整数时，`OP_call_method` 自己回答它。它像 `hasOwnProperty` 那样，用 `find_own_property` 在 shape 里查找这个键。

为什么安全：对这些参数，两次调用都不会运行脚本或抛错。键不需要可能调用方法的转换，对象就是它自己的 `ToObject`，普通对象的自有属性在它的 shape 里就能找到。被脚本替换的 `call` 或 `hasOwnProperty`，或者被绑定的，是另一个函数，仍然走这些调用。这两次调用在进入时做的事先检查：调用深度（两层）、影子栈（留两个余量，因为每次调用的栈帧先于它的检查）、待处理的中断和待处理的无法捕获异常。任何一次调用本来会抛错的地方，仍然发出这些调用并像以前一样抛错。模块命名空间的绑定在设置之前可能抛错，也走这些调用。没有发出调用时，那里不为下一次轮询计数：循环仍然在它们的跳转处计数。

在商店的工作台页面里，每次渲染有 7,782 次调用走新路径，一次渲染少执行约 2.5 M 条指令，约 1.5%。连同 [for in 循环](#for-in-循环持有它的对象的-shape)，商店少执行 2.5% 的指令，tifa 测试应用的 MUI 页面少 3.4%，`r_react_ssr` 少 1.8%，`r_bench_spiral` 少 2.8%。时间见 [performance.md](../performance.zh-CN.md#调用for-in-和-hasownpropertycall)。

改动在 C 里（`engine/quickjs.c`）：当函数是 `Function.prototype.call` 时，`OP_call_method` 和 `OP_tail_call_method` 调用 `js_has_own_call`。`engine/test/compare/for_in_has_own.js` 用各种类型的键（字符串、作为数和字符串的数组下标、`"01"`、空字符串、symbol、`null`、`undefined`、布尔值、BigInt、带 `toString` 的对象、抛错的对象）和各种 `this` 值（原始值、函数、数组、getter、被删除的键、proxy、全局对象），通过 `call`、`apply`、`Reflect.apply`、`bind` 以及作为方法调用 `hasOwnProperty`，并替换和还原 `call` 与 `hasOwnProperty`。`TestHasOwnCallDepth`（根包）检查通过它的递归，和通过这些调用的递归在同样的深度结束。

## 复制不了的共享 shape 让调用失败

发布版本：v0.29.9。

有两个地方假定它们所改动的对象的 shape 是它自己的，而当这个 shape 是共享的（被同一原型的另一个对象持有），并且复制它时内存耗尽，就会出错。上游 QuickJS 有同样的两处。

- 用另一个原型创建的数组，比如 `Reflect.construct(Array, [], F)`，其中 `F.prototype` 设为 `Map.prototype`，从那个原型的空 shape 出发，并加上 `length`。`JS_NewObjectFromShape` 认为这不会失败，通过 `add_property` 给出的 null 去写长度。现在它会释放正在创建的对象并抛出内存耗尽错误。
- `Object.setPrototypeOf` 在 `js_shape_prepare_update` 复制 shape 之前就取了对新原型的引用，复制失败时返回却没有把引用还回去，所以这个原型和它持有的一切都一直活着。现在它在 shape 成为对象自己的之后才取这个引用。

两条路径都只在内存耗尽时才运行，所以其他一切不变，也没有变快或变慢。`engine/test/unit-white.c` 里的 `test_shared_shape_oom` 让分配器的每次调用依次失败地运行两者，检查每次调用要么抛出内存耗尽错误，要么给出结果，并且留下的对象和之前一样多。没有这个修复，第一个会以段错误结束测试，第二个会多留下一个对象。

## 内置类的对象找到它们原型的空 shape

发布版本：v0.29.10。

内置类的对象，比如 `Map`、`Promise`、`Date`，或者解构数组时用的迭代器，从它的类的原型的空 shape 出发。没有东西持有这个 shape：shape 缓存只保留原型是 `Object.prototype` 的 shape，所以这个 shape 随该类的最后一个对象一起被释放，又为下一个对象重新创建。商店的工作台页面每渲染一个页面，要创建并释放它约 1,300 次。现在 context 在第一个用这个原型创建的该类对象出现时，持有 `class_proto[class_id]` 的空 shape，之后的对象在 shape 哈希表里找到它。带数组原型的 `new Array(n)` 从数组的 shape 出发，这个 shape 已经有 `length`，和数组字面量一样，而以前它是从空 shape 经过 `add_property` 出发。

为什么安全：对象得到的 shape 和以前相同，原型相同、没有键、哈希的大小相同，只是不再是新建的。被 context 持有的 shape 被多于一个持有者持有，所以得到一个键的对象会复制它（`add_property` 和 `js_shape_prepare_update`），被持有的 shape 保持为空。被持有的 shape 只持有它的原型，而 context 本来就把这个原型作为 `class_proto[class_id]` 持有，`JS_SetClassProto` 在替换那个原型时放掉这个 shape，所以这个 shape 不会让 context 本来不会保留的任何对象存活。`JS_MarkContext` 标记这些 shape，所以循环回收器会释放一个只靠它们才活着的 context，`JS_FreeContext` 会放掉它们，和新函数的 shape 一样。原型不是这个 context 里它的类的原型的对象，比如子类的对象、`newTarget` 的 `prototype` 不同的对象、或另一个 realm 的对象，走旧路径。新函数 `js_new_proto_shape` 只在 shape 哈希表里没有这个原型的空 shape 的地方运行，并且不内联：`JS_NewObjectProtoClass` 被内联进解释器，它路径上多加一项检查，就会改变解释器的栈帧并增加它的指令。

代价是每个 context 里每个内置类一个指针的数组（约 500 字节），以及页面用到的每个内置类一个约 90 字节的 shape，一个 VM 里有几十个。在原生 C 树上用 callgrind 测得，商店的工作台页面一轮少执行 0.61% 的指令，store 页面少 0.69%，MUI 页面少 0.19%，bench 页面相同。

改动在 C 里（`engine/quickjs.c`）：`JS_NewObjectProtoClass` 调用 `js_new_proto_shape`，它把 shape 保存在 `JSContext` 的字段 `class_shapes` 里，`JS_SetClassProto` 放掉它，`js_create_from_ctor` 对带数组原型的数组调用 `JS_NewArray`。`engine/test/compare/class_shapes.js` 接连创建 15 个内置类的对象，给其中一些加上键、symbol 和隐藏属性再删掉，并检查每个类的下一个对象不列出任何自有键。它还用 `new Array`、`Array` 和 `Reflect.construct` 创建数组，创建子类和其他 `newTarget` 的对象，在脚本改动了 `Map.prototype` 之后以及在设置了原型之后创建 `Map`，创建带 `Map` 原型的数组，以及在脚本改动了 `Array.prototype` 的继承对象之后创建数组。它的输出是在改动之前的引擎上记录的。`engine/test/unit.c` 里的 `test_class_shapes` 检查七种对象中每种的一个对象给 context 留下它的 shape，再多创建一百个也不会多出来，`new Array(3)` 不留下任何 shape，释放两个 context 中的一个会释放它的 shape，而另一个 context 的对象保持原样，以及年轻代回收会保留年轻 context 的 shape 和一个旧 context 自上次回收以来取得的 shape。`test_class_shape_set_proto` 用 `JS_SetClassProto` 给 `Map` 类一个 host 的原型，创建一个该类的对象使 context 持有这个 shape，替换原型，并检查旧的在 host 放手之后被释放，下一个对象得到新的。

## 通过 shape 由哪个 shape 长出来来找到它

发布版本：v0.29.10。

对象得到一个键时，`add_property` 在 shape 哈希表里找一个带有对象已有的键和新键的 shape。以前，按哈希找到的 shape，要把它的每个键和标志与对象的逐个比较，而商店页面的对象在新键之前有约 4 个键。现在哈希表里的每个 shape 有一个编号 `id`，以及它得到最后一个键之前的那个 shape 的编号 `parent_id`。找到的 shape 的父编号就是对象的 shape 的编号时，只比较它的最后一个键。在商店的工作台页面里，找到的 shape 有 80% 是这样，在 store、MUI 和 bench 页面上是 77% 到 95%（在原生 C 树上统计）。

为什么安全：哈希表里编号相同的两个 shape，原型、键和标志都相同，所以父编号等于对象 shape 编号的 shape，是以对象的键开头的。哈希表里的 shape 只有一种改变方式，即在末尾得到一个键（`add_shape_property`），这给它一个新编号，并让它原来的编号成为它的父编号。其他任何改变 shape 的事，删除、新标志、getter、新原型、不再稠密的数组，都先经过 `js_shape_prepare_update`，它把这个 shape 永久地拿出哈希表（没有共享的离开哈希表，共享的被复制），并给它编号 0。唯一一个不经过它、原地改变标志的地方，即在全局对象的一个可配置属性上重新声明全局函数（`js_closure_define_global_var`），改变的是全局对象的 shape，而它从不在哈希表里：这个 shape 在哈希表之外创建，在哈希表之外创建的 shape 永远不会进入哈希表，只有 shape 在哈希表里的对象才会取哈希表里的另一个 shape。引擎在那里有断言。`add_property` 放进哈希表的副本保留它所复制的 shape 的编号，它们有相同的键，直到它的键让它成为新 shape，并且在因内存不足而无法添加键时一直保留，所以哈希表里的两个 shape 可能有同一个编号，原型、键和标志都相同。计数器从不给出两次同样的编号：32 位用完时，哈希表里的每个 shape 从 1 起重新编号，且没有父编号，接下来的 shape 比较每个键，直到它们再次得到键。按编号找到的 shape，就是比较键会找到的那个，所以对象得到的 shape 和以前相同，键和顺序也相同。

代价是每个 shape 多 8 字节，在它的键之前是 64 字节，以前是 56：渲染一个页面的 VM 持有约 4,000 个，约 32 KB。在 tifa 里用 callgrind 测得（`measure bench cg`，对比它之前的那项改动），一次渲染少执行的指令，商店页面是 0.82%，store 页面是 0.88%，MUI 页面是 1.38%，bench 页面是 0.03%，解释器自己的指令不变。

改动在 C 里（`engine/quickjs.c`）：`JSShape` 有 `id` 和 `parent_id`，`JSRuntime` 有 `shape_id_next`，`js_shape_new_id` 给出编号，`js_shape_renumber` 给哈希表重新编号，`js_new_shape2` 和 `add_shape_property` 给 shape 编号，`js_shape_prepare_update` 拿走编号，`find_hashed_shape_prop` 比较编号。把 `JS_SHAPE_ID_CHECK` 设为 1 时，引擎对按编号找到的 shape 也比较每个键，不同就中止：这样构建的原生测试 host 运行 `engine/test/compare.sh` 和 test262（33 个已知错误），没有中止。`engine/test/compare/shape_ids.js` 让对象经过 shape 的每一种改变而增长：删除（一个键以及最后一个键）、新标志、冻结和密封、变成 getter 再变回来、新原型和 `null`、不再稠密的数组和 `arguments`、共享 shape 并继续增长的对象、重新声明的全局函数，以及作为 symbol 和数字的键，然后再来一遍同样的键，并列出它们的键和标志。它的输出是在改动之前的引擎上记录的。`engine/test/unit-white.c` 里的 `test_shape_ids`，带着 `JS_SHAPE_ID_CHECK` 构建，在每一步之后检查：哈希表里编号相同的 shape 原型、键和标志相同，每个父编号都属于一个少一个键、之前的键相同的 shape，shape 缓存开着和关着都是这样：一个一个得到键的对象，按编号找到之前这样做过的对象的 shape，以上的各种改变，重新声明的全局函数（之后全局对象的 shape 仍然在哈希表之外），编号在一批对象中间用完（这会给哈希表重新编号，并保留 shape 和键），以及之后又按编号找到的 shape。`test_shape_ids_copy_oom` 让分配器的每次调用依次失败，同时让一个与另一个对象共享 shape 的对象得到一个键，并检查键没能添加时留在哈希表里、带着它所复制的 shape 的编号的副本，仍遵守这条规则，两个对象都保有它们的键。

## 单独死去的对象立刻释放

发布版本：v0.29.10。

在回收之外，一个对象的计数降到零时，`__JS_FreeValueRT` 过去是把它从回收器的列表上取下来，放到 `gc_zero_ref_count_list` 上，再调用 `free_zero_refcount`，后者又把它从那个列表上取下来并释放它，然后释放和它一起死去的对象，这些对象加到列表的前端。在商店的工作台页面里，被释放的对象有 70% 是单独死去的。这样的对象现在立刻释放（`free_zero_refcount_first`）：它仍然进入 `gc_zero_ref_count_list` 的前端，但在那里被释放，不经过 `free_zero_refcount` 的循环，只有当有东西和它一起死去时，这个循环才在它之后运行。函数的字节码、async 函数或计数降到零的模块走旧路径。

为什么安全：`free_zero_refcount` 本来会先取这个对象，因为它在列表的前端，而在它被释放时死去的对象按同样的顺序加到列表的前端，所以每个对象都在和以前相同的时间点被释放。对象被释放时和以前一样在 `gc_zero_ref_count_list` 上，而不在回收器的任何一个列表上，所以在它被释放期间遍历那些列表（比如 host 的 finalizer 调用的 `JS_ComputeMemoryUsage`），不会找到一个 shape 已经没有的它。对象被释放期间阶段是 `JS_GC_PHASE_DECREF`，和以前一样，所以引擎在释放期间发起的任何调用，都不会开始一次回收，也不会立刻释放另一个对象，释放始终只有一层深（`engine/test/cycles.sh` 检查 `free_zero_refcount_first` 不通过引擎的调用到达任何回收）。host 的回调，比如类的 finalizer，不在 `cycles.sh` 检查的图里：阶段不是 `JS_GC_PHASE_NONE` 时，它们请求的回收什么也不做（见 [引擎在释放对象时被请求的回收](c-engine.zh-CN.md#释放对象时被要求回收)），`unit-white` 的 `test_gc_in_free` 对这条路径也做了检查，对年轻对象和旧对象都检查。它的标记先设好，和以前一样，所以 finalizer 运行期间 `js_weakref_free` 不会释放它的块，因为弱引用而保留的对象，它的标记也像以前一样回到 0。在回收期间什么都不变：阶段不是 `JS_GC_PHASE_NONE` 时死去的对象加入列表，在环被移除期间死去的则由 `young_defer` 放在一边。

新路径自己调用 `free_object`，而不是 `free_gc_object`，并让 `free_zero_refcount` 保持原样。如果让 `__JS_FreeValueRT` 调用 `free_gc_object`，或者改动 `free_zero_refcount`，这些函数就会有别的调用者或另一个大小，编译器随后就会在某一个架构上把 `free_zero_refcount` 或 `async_func_free` 内联进 `JS_CallInternal`，这会改变它的栈帧（amd64 上 480 字节，arm64 上 520 字节）。现在这样，`JS_CallInternal` 是同样的代码、同样的栈帧，`__JS_FreeValueRT` 仍然是解释器调用的独立函数。

它在内存上没有代价。在 tifa 里用 callgrind 测得（`measure bench cg`，对比它之前的那项改动），一次渲染少执行的指令，商店页面是 0.65%，store 页面是 0.71%，MUI 页面是 0.64%，bench 页面是 0.45%，解释器自己的指令不变。

改动在 C 里（`engine/quickjs.c`）：`__JS_FreeValueRT` 调用 `free_zero_refcount_first`，`internal/goasm/cmd/cycles` 检查它到达不了 `JS_RunGC` 和 `JS_RunYoungGC`。`engine/test/unit.c` 里的 `test_free_order` 给出一个类的对象，其 finalizer 写下它们的编号，并丢弃一棵通过属性、数组、闭包、`Map`、`Set`、`WeakRef`、`FinalizationRegistry` 以及会释放更多对象的 finalizer 持有的对象树，然后是由回收释放的环和由年轻代回收释放的环，并把它们被释放的顺序和改动之前的引擎上记录的顺序对比，检查 `WeakRef` 在丢弃之后发现它的目标已经没有了，以及 `FinalizationRegistry` 的回调在一次回收之后得到它的值。`test_finalizer_memory_usage` 释放一个对象（先年轻的，再旧的），它的 finalizer 调用 `JS_ComputeMemoryUsage`：这项改动的第一个版本，在对象被释放期间把它留在回收器的列表上，在那里崩溃了。`engine/test/compare.sh` 和 `unit`（一百万个嵌套对象一次释放，以及各种对象由它们的 finalizer 和回收释放）在 AddressSanitizer 构建（`engine/test/build.sh asan`）下运行，没有任何报告。

## 拒绝写入的 proxy 在严格模式代码里抛错

发布版本：v0.30.2。

严格模式代码里的 `o.x = 1` 或 `o[k] = 1` 这类写入，如果对象是没有 `set` trap、`defineProperty` trap 返回 false 的 proxy，现在会抛 `TypeError`，和 Node 24 一样，也和规范一致：trap 返回 false 让 proxy 的 `[[Set]]` 返回 false，而严格模式代码的写入在 `[[Set]]` 返回 false 时要抛错。不管目标上已经有这个属性还是没有，都是这样，接收者是这种 proxy 的 `super.x = 1` 和经过 `with` 的写入也一样。在任何代码里都必须抛错的写入，比如 `Object.assign`、`Array.prototype.fill` 和 `push` 里的写入，以及 Go API 的 `SetProperty` 和 `SetPropertyValue`，在目标已有该属性时原来会继续往下走，现在也会抛错。原来引擎不报错继续往下走，上游 QuickJS 和 quickjs-ng 现在也还是这样。非严格模式代码的写入照旧继续，`Reflect.set` 照旧返回 false。

为什么安全：有两处丢掉了表示「写入被拒绝要抛错」的标志。`js_proxy_define_own_property1` 检查 trap 结果时只看 `JS_PROP_THROW`，而其他所有拒绝写入的地方（只读属性、缺少 setter、proxy 的 `set` trap 返回 false）在严格模式代码里遇到 `JS_PROP_THROW_STRICT` 也会抛错，走的是 `JS_ThrowTypeErrorOrFalse`。现在它也调用 `JS_ThrowTypeErrorOrFalse`。另外，属性已经存在时，`JS_SetPropertyInternal` 在接收者上重新定义它的值，完全不带抛错标志，所以接收者返回 false 时，任何代码都会继续往下走。现在它传入这次写入的 `JS_PROP_THROW` 和 `JS_PROP_THROW_STRICT`，和新属性的做法一样。`engine/quickjs.c` 里另外五处对 `JS_PROP_THROW` 的检查本来就考虑了 `JS_PROP_THROW_STRICT`。

改动在 C 里（`engine/quickjs.c` 的 `js_proxy_define_own_property1` 和 `JS_SetPropertyInternal`）。`engine/test/compare/proxy_define_false.js` 在严格和非严格模式代码里通过这样的 proxy 写入，用名字、下标和计算出来的键，写目标已有的属性和没有的属性，还有复合赋值、接收者继承自该 proxy 的写入、`super` 和 `with`、以该 proxy 为目标和为接收者的 `Reflect.set`、`Object.assign`、`Array.prototype.fill` 和 `push`，它的结果文件就是 Node 24 打印的内容。`TestSetPropertyRefused` 检查 `SetProperty` 和 `SetPropertyValue`。test262 里没有对应的用例。

## 对象字面量的 shape 至少有空对象的空间

发布：v0.30.2。

对象字面量第一次运行时建的 shape，至少能放两个属性，和空对象的 shape 一样（`JS_PROP_INITIAL_SIZE`），以前是刚好放下它的键。shape 哈希表里的 shape 也是一个个加键的对象的下一个 shape：`add_property` 在表里找到下一个 shape，把对象的属性数组挪到这个 shape 的空间。空对象能放 2 个。如果它的第一个键正好是包里某个单键字面量的键，找到的 shape 只能放 1 个，数组就缩到一格，加第二个键时又涨回两格。引擎的迭代器结果（`js_create_iterator_result` 先建空对象，再加 `value` 和 `done`）在包里有 `{ value: ... }` 这种字面量时，每个结果都这样挪：商店英文首页每次渲染 9,020 个结果，每个挪两次，占了一次渲染里属性数组 33,000 次挪动中的 18,040 次。现在这种字面量的 shape 能放 2 个，正好合空对象，不再挪动。

一次渲染里属性数组的挪动次数，在原生 C 树上按 tifa 池子的设置（保留 shape）数：英文首页 33,000 降到 12,800，中文商品页 55,100 降到 22,000，工作台 studio 页 24,200 降到 15,600，MUI 的 orders 页 30,700 降到 25,000。保留 shape 不影响这些挪动：关掉时分别是 33,200、57,100、25,900 和 34,500。

为什么安全：读属性数组的地方都只读 shape 的前 `prop_count` 格，多出来的格是留给以后的键的，和 `js_new_shape` 或 `JS_NewObjectProtoClassAlloc` 建的 shape 一样，后者本来就至少给 `JS_PROP_INITIAL_SIZE`。属性放在对象自己块里的单键字面量（`js_new_object_inline`），块里多留出第二格。shape 还是原来在表里找到或放进表里的那个，键和标志都一样，所以对象得到的 shape 一样，键和顺序也一样。

代价是每个刚好一个键的字面量对象多 16 字节。在原生 C 树上用 callgrind 量（linux/arm64，只算计时的渲染，按 tifa 池子的设置），商店的页面每次渲染少 0.20% 到 0.28% 的指令，MUI 的页面少 0.09% 到 0.12%，React 的 `r_react_ssr` 不变，HTML 相同。在 darwin/arm64 上计时（轮流跑 3 轮，每次取 40 次渲染的中位数），四个页面变化在 -0.7% 到 +1.3% 之间，没有超出这台机器的噪声。渲染 200 次并做一次完整回收后，VM 持有的内存在英文首页多 0.15%（13.52 MB 到 13.54 MB），在 MUI 的 orders 页多 0.1%。

改动在 C 里（`engine/quickjs.c` 的 `js_object_lit_shape`）。`engine/test/compare.sh`、`unit-white` 和商店与 MUI 的 23 个页面的 HTML（SHA-256 和 main 相同）覆盖它。

## for in 循环对一个 shape 只问一次能不能从它列出键

发布：v0.30.2。

`for in` 循环遍历的对象，如果它的 shape 在 shape 哈希表里，而且 shape 的字符串键里没有数组下标也没有绑定，就[从 shape 列出键](#for-in-循环持有它的对象的-shape)。为了知道这一点，以前每次循环都要把 shape 的键过一遍（`js_own_keys_scan`）。MUI 的页面每次渲染要对样式和 props 对象跑大约 10,000 次循环，1% 的指令花在这上面。现在 shape 把答案存在它填充位里的一个字节（`for_in_keys`）：第一次遍历这个 shape 的对象时看一遍键，之后的循环直接读这个字节。

为什么安全：对不是 exotic 的对象，答案只取决于 shape，也就是它的键和键的标志，对象自己的 `is_exotic` 标志每次循环仍然会读。只有 shape 在 shape 哈希表里时才读这个字节，而表里的 shape 不会原地改变：加键之前它先离开表（`add_shape_property`），删键或改键的标志之前它永久离开表或者被复制（`js_shape_prepare_update`），释放之前也先离开表，这和键种类缓存依赖的规则相同（`js_own_key_kinds`）。shape 每次进表或出表时这个字节都清回 0（`js_shape_hash_link`、`js_shape_hash_unlink`），所以复制出来的、长大的、重新放回的 shape 都从没有答案开始。

内存上没有代价：`JSShape` 大小不变。在原生 C 树上用 callgrind 量（linux/arm64，只算计时的渲染，按 tifa 池子的设置），MUI 的页面每次渲染少 0.82% 到 1.08% 的指令，商店的页面少 0.12% 到 0.28%，React 的 `r_react_ssr` 少 0.29%，HTML 相同。在 darwin/arm64 上计时（轮流跑 3 轮，每次取 40 次渲染的中位数），MUI 的 orders 页时间少 1.4%，商店的页面变化在 -0.6% 到 +0.7% 之间，没有超出这台机器的噪声。渲染 200 次后 VM 持有的内存一个字节都不差。

改动在 C 里（`engine/quickjs.c`）：`JSShape` 有了 `for_in_keys`，由 `js_for_in_keys_of_shape` 为 `build_for_in_iterator` 读和设。`engine/test/compare/for_in_shape_keys.js` 对同一个 shape 的对象循环多次，然后遍历这个 shape 的对象在加了数组下标、symbol、不可枚举的键、被改成不可枚举的键、getter 或删了键之后的样子，遍历先有下标的 shape，遍历在循环中加键和删键的对象，遍历同一个 shape 的两个对象之一而另一个在变，遍历原型上有键的对象和后来才加键的原型，还有数组、`arguments`、`String` 对象、类型化数组、全局对象和 40 个键的对象。它的文件由改动前的引擎写出，和 Node 24 打印的相同。

## 往数组里追加元素时，第一次就留 8 个元素的空间

发布版本：v0.30.3。

用 `push`、在长度处按下标写、展开、`splice`，或者一个一个往数组里填元素的内置函数（`Object.entries`、`split`、`JSON.parse`、`concat`、`map`）追加元素时，数组第一个元素只分到 1 个槽，之后依次是 2、3、4、6、9，每一步都是一次分配，加上一次复制已有的元素。这样建出来的数组大多只有几个元素（商店页每次渲染有 1.7K 到 3K 个数组超出了第一次分配，MUI 订单页 3K 个）。现在 `engine/quickjs.c` 里的 `expand_fast_array_append`，在一个还没有空间的数组追加第一个元素时，给它 `JS_ARRAY_MIN_ROOM`（8）个槽，之后照常 1.5 倍增长。只有追加的三个地方调用它（`add_fast_array_element`、`push` 的快速路径、`splice` 让数组变长的地方）。一开始就知道大小的数组（字面量、`slice`、剩余参数、`arguments`、自身键的数组）仍然只要刚好的大小。

为什么安全：只有数组的容量变了。长度、元素、空洞和所有属性，都和一个一个槽长出来的数组一样，没有空间时第一次分配也同样以内存不足错误失败。脚本读不到容量。

收益，按一次渲染的指令数的比例（callgrind，linux/arm64 原生 C 树，`render tifa=1 young=1 shapes=2048`，两次预热后一次计时渲染，不算 `host_poll` 和 `__shadow_remaining`）：React 少 0.095%，商店首页 0.227%，商品页 0.231%，一个工作台页面 0.191%，MUI 订单页 0.392%。加上测量用的 URL 替身（`urlmemo.js`，商店页要跑的 URL polyfill 在 tifa 里不会有）后，分别是 0.142%、0.141%、0.192% 和 0.387%。留 4 个槽时同样的页面是 0.091%、0.103%、0.185%、0.199% 和 0.361%，所以留 8。渲染 200 次、每次后做一次年轻代回收之后，一个 VM 占的内存相差不到 0.1%（商品页使用字节 14,063,072 到 14,071,200，MUI 订单页 7,369,312 到 7,385,728，footprint 不变），因为这些数组到那时大多已经死了。一个留下来的小数组（`Object.entries` 的一对）占 8 个 16 字节的槽，比以前最多多 96 字节。

测试：`engine/test/compare/array_room.js` 用 push、按下标和展开建 0 到 9 个元素的数组，带空洞、把长度改小和改大、对稀疏数组和带 `Symbol.isConcatSpreadable` 的类数组做 concat、对回调里改长度的数组和带 `Symbol.species` 的子类做 map、`Array(n)` 从 0 到 2^32-1，以及自身键的数组。它的文件是改动之前的引擎写出来的。`engine/test/unit-oom.c` 里的 `test_array_room_oom` 在 push、一次 push 很多、按下标、展开、splice、`Object.entries`、`split` 和 `JSON.parse` 建数组的第一次分配处耗尽内存。

## `in` 运算符遍历原型链时不加引用

发布版本：v0.30.3。

`in`、`Reflect.has`、属性描述符的各个字段（`Object.defineProperty`）、`every` 和 `concat` 查找的空位，都要经过 `JS_HasProperty`。它以前对原型链上的每个对象先加一个引用，问完这个对象有没有这个键（`JS_GetOwnPropertyInternal`）再放掉。现在只有不是快速数组的 exotic 对象才这样做：`String` 对象，或者类自带 `get_own_property` 的对象。不是 exotic 的对象、数组、`arguments` 和类型化数组都不加引用直接问。

为什么安全：这个引用是为了在可能放掉对象的代码运行时让对象活着。不带描述符时，`JS_GetOwnPropertyInternal` 对不是 exotic 的对象和快速数组不运行任何 JavaScript：它在 shape 里找键，遇到全局对象上还没初始化的绑定时生成一个错误，这也不运行 JavaScript。生成错误时的分配可能触发一次回收，回收只释放谁也够不着的东西，而这个对象从 `obj` 经过它前面的原型就能够着，调用方持有 `obj`。Proxy 和模块命名空间照旧带着引用走它们的 `has_property`。生成代码的轮询在这次查找里抛出的 kill（`ErrKilled`）之后不再释放任何东西，所以那里有没有这个引用也一样。

在原生 C 树上用 callgrind 量（linux/amd64，只算计时的渲染，按 tifa 池子的设置，不算 `host_poll`），商店的英文首页和中文商品页每次渲染少 0.29% 的指令，workbench studio 页少 0.17%，MUI 的 orders 页少 0.05%，React 的 `r_react_ssr` 少 0.06%，商店和 MUI 的 23 个页面 HTML 相同。商店页面上的遍历大多来自 URL polyfill：把商店的 URL 对象复用起来（同样的页面，用一个替身代替 `URL`），收益是 0.05% 到 0.07%。它没有加分支：判断 exotic 对象的那次测试本来就有。

改动在 C 里（`engine/quickjs.c`，`JS_HasProperty`）。`engine/test/compare/has_property_chain.js` 遍历普通的链、快速数组、类型化数组、`String` 对象和全局对象，遍历带 Proxy 的链（它的 `has` trap 剪断链并丢掉 target），遍历原型上有 getter 的描述符（getter 删掉后面的字段并剪断链），用 `toString` 会抛错、会剪断链或删掉键的键，以及由原型上的 getter 为 `every` 和 `concat` 填上的空位。它的文件和 goldens 由改动前的引擎写出。`unit-oom` 在内存不够时把同样的遍历扫一遍（`test_has_property_oom`）。

## 对象的 rest 直接按最终的 shape 创建

发布版本：Unreleased。

`const {a, b, ...r} = s` 以前为每次 rest 建一个装着 `a` 和 `b` 的对象来排除它们（先建空对象，再逐个定义键），然后建一个空的 `r`，把 `s` 的键一个个给它，每个键都要在那个对象里查一次。用这种写法拆 props 的 React 组件，每次要付两个对象的钱，每个键还要多查一次。MUI 和商店的页面每次渲染跑 800 到 2,200 次 rest，每次 3 到 5 个键，排除 1.5 到 7 个名字。

现在解析器把模式里的静态键放进一张 atom 列表，作为函数的对象字面量表（`object_lits`）的一项，规则相同：最多 32 个键，不能重复。然后生成带这张列表下标的 `OP_copy_rest`。只有计算出来的键（`{[k]: x, ...r}`）还像以前一样放进一个对象，静态键超过 32 个的模式也走老路。寄存器循环把它当作 `COPYR` 执行（`engine/regvm-ops-copy.h`）。如果源对象是普通对象，自有键都是不是数组下标的字符串，每个都是普通数据属性（不会运行 getter），那么源对象的 shape 就决定了一切：复制哪些键、什么顺序、什么标志。`COPYR` 于是直接按这些键的对象字面量的 shape 建出结果，也就是逐个定义这些键最后会得到的 shape，属性放在按最终大小分配的一个内存块里，然后带着引用复制各个值。每个 `COPYR` 为最多 4 个源对象的 shape 记下结果的 shape 和每个值去哪个槽：命中只要一次比较和一次复制。其他源对象（带 getter 的类实例、proxy、数组、`arguments`、下标键或 symbol 键、被排除的计算键）走通用路径 `js_copy_data_properties`，它现在也接受这张列表。

为什么安全：键在查找之前就被排除，和 CopyDataProperties 的规定一致，列表里是模式读过的键，形式和 `get_field` 用的一样。通用路径就是原来那条。快速路径只在源对象的 shape 能决定结果时才走：没有 getter，所以复制过程中没有东西能改动源对象，每个字符串键都按 shape 的顺序列出（下标键和 symbol 键在 `[[OwnPropertyKeys]]` 里排在最前或最后，遇到就走通用路径），可能装着错误的延迟调用栈的 `stack` 键也走通用路径。缓存项持有源对象 shape 的引用，所以这个 shape 在缓存项还在时不会被原地修改（一个 shape 被两处持有时，对象在增删键之前会先复制它，见 `js_shape_prepare_update`），而且只记 shape 哈希表里的 shape，因为不在表里的 shape 从不共享。只记原型是 null 或本 realm 的 `Object.prototype`、且没有 symbol 的 shape，所以缓存项不会让脚本丢掉的原型活着，也不会让 `WeakRef` 能观察到的 symbol 活着。字节码为循环回收器标记它缓存项里的 shape，并和寄存器代码一起释放它们。见到超过 4 个 shape 的位置最多替换 16 次缓存项，之后每次都在未命中路径上建结果，除了结果本身不再多分配。

代价：每个 rest 在函数的寄存器代码旁边占一个 40 字节的位置（4 个指针和一个计数），每记一个 shape 多一项 28 字节，外加源对象每个键 1 字节。字节码版本是 12（`OP_copy_rest`）：更早版本写出的字节码不再能加载。

收益：Apple M4 上的原生微基准（每次操作的指令数，含循环本身），`var {a, ...r} = o3` 从 2,591 降到 1,303，排除五个名字从 4,651 降到 1,712，八个键从 4,117 降到 1,441。一次渲染在 workbench 和 MUI 的页面上少执行 2.5% 的指令（产品计数器，M4，预热 3 次后计 8 次），商店页面少 0.7%，React 页面没有 rest，持平。24 个页面的 HTML 相同。

测试：`engine/test/compare/object_rest.js`（输出和 Node 24 一样）在三个位置各跑三次 39 种源对象，在一个位置轮流跑 9 种 shape 共 40 轮，还覆盖记下的 shape 对应的对象之后增加、删除或改动键，计算出来的键及其顺序，重复键、数字键和 `__proto__` 键，默认值，40 个静态键，嵌套模式，参数，`for of`，赋值，`catch`，箭头函数和生成器。`engine/test/unit-white.c` 里的 `test_object_rest_cache` 检查缓存项：结果的 shape 和逐个建键的对象得到的一样，类实例、symbol 和删过键的对象不记，原型为 null 的记，轮流见到七个 shape 后留下 4 项和 16 次替换，shape 随函数一起释放（泄漏检查）。`engine/test/regvm/js/q3-objrest.js` 在抛错注入测试里跑 rest 的各种抛错。test262 的解构测试给出的已知错误不变。

## 展开按目标和源对象 shape 的缓存复制

发布版本：Unreleased。

`{type: x, ...props}` 以前把 `props` 的键一个个复制过去，每个键都是对目标的一次定义：查一次键，把目标移到下一个 shape，属性数组一路按 2、4、8 增长。商店页面的展开里，80% 的键是这样复制的（每次展开 10.5 个键）。展开到空目标时，如果源对象是普通对象，原来就已经一次拿到它的 shape（`js_copy_shape`）。

现在寄存器循环把对象字面量里的展开当作 `COPYS` 执行（`engine/regvm-ops-copy.h`），带一个和 rest 类似的缓存，以目标和源对象两者的 shape 为键：记下结果的 shape，以及源对象每个槽的值去目标的哪个槽。目标已有的键留在原位，取源对象的值，新键按源对象的顺序排在后面。命中时，目标换成结果的 shape，属性数组一次长到那个 shape 的大小，然后复制各个值。条件是目标的每个键都是字符串、不是下标、可枚举、可写、可配置（字面量里没有 getter），源对象的条件和 rest 一样。其他目标或源对象走通用路径。展开 null 或 undefined 直接返回。

为什么安全：目标得到的，就是依次定义源对象每个键得到的结果，由两个 shape 决定，和 rest 一样：对目标已有的、可写可配置的普通数据属性做定义，只改它的值，新键以数据属性的标志加在最后。目标是字面量的对象，可扩展，类是普通对象（每次执行都检查，因为 `preventExtensions` 不改 shape）。缓存项持有目标、源对象和结果三个 shape 的引用，规则和 rest 一样。

代价：对象字面量里的每个展开，在函数的寄存器代码旁边占一个 40 字节的位置（4 个指针和一个计数），每记一对 shape 多一项 28 字节，外加源对象每个键 1 字节。

收益：Apple M4 上的原生微基准（每次操作的指令数，含循环本身），`{z: j, ...o3}` 从 1,920 降到 1,180，8 个键的 `{z: j, ...o}` 从 3,172 降到 1,356，`{...o3, z: j}` 从 1,524 降到 1,422，`{...o}` 从 1,437 降到 1,296。和只有 rest 时相比，一次渲染在商店页面少执行 1.5% 的指令，workbench 页面少 1.8%，MUI 少 2.6%，React 少 0.2%（产品计数器，M4，预热 3 次后计 8 次）。连同 rest，和两者之前的代码相比：React 少 0.6%，商店少 2.2%（每页 1.0% 到 3.2%），workbench 少 4.2%（3.9% 到 4.4%），MUI 少 5.1%（3.3% 到 7.3%）。24 个页面的 HTML 相同。

还试过把空目标直接按结果的大小建出来：释放字面量的空对象，在一个内存块里建出结果。这样更慢：`{...o3, z}` 从 1,422 变成 1,542 条指令，`{...o2, z}` 从 1,272 变成 1,499。释放空对象比让它的数组增长更贵，而且属性放在对象内存块里的结果，在下一个键到来时还要把属性搬出去。

测试：`engine/test/compare/object_spread.js`（输出和 Node 24 一样）把 30 种源对象展开到 7 种目标里，各跑三次，在一个位置轮流跑 7 种 shape 共 50 轮，还覆盖一个位置上其他 shape 的目标，记下的 shape 对应的对象之后发生改动，会增删键的 getter，以及 `Object.prototype` 上的 setter 和一个热循环。`engine/test/unit-white.c` 里的 `test_object_spread_cache` 检查缓存项，`engine/test/regvm/js/q3-objspread.js` 在抛错注入测试里跑展开的各种抛错。
