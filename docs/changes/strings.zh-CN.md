---
summary: '字符串搜索的改动：includes、startsWith、endsWith、indexOf 和 lastIndexOf 直接跳到字符串所在的位置，并把查看过的内容计入中断检查，收益是什么，为什么结果不变，symbol 和它的描述为什么不和脚本共用字符串，以及 atom 的哈希表为什么两个桶只放一个 atom'
read_when:
  - String.prototype.includes、indexOf 或 lastIndexOf 给出的结果和以前不一样，或者在长字符串上很慢
  - 用字符串做出的 symbol，或者描述被拿着的 symbol，WeakRef 或 FinalizationRegistry 一直放不掉它
  - 一个 VM 给 atom 占的内存比以前多，或者接近内存上限的脚本在以前不会内存不足的地方内存不足
---

# 字符串

[English](strings.md) · 简体中文

## 搜索直接跳到字符串所在的位置

发布版本：v0.27.3（本条里的计时是在那个版本的引擎上测的，当时还没有 C 引擎）。

`String.prototype.includes`、`startsWith`、`endsWith`、`indexOf` 和 `lastIndexOf` 直接跳到要找的字符串所在的位置。上游是在一个又一个下标处，用 `string_cmp` 比较整个字符串，而 `string_cmp` 通过 `string_get` 读两个字符串的每个字符，所以一次什么也没找到的搜索，对被查找字符串里的每个字符都要付出一次调用和几次字符读取，每个字符约 3 ns。tifa 每次渲染后会在页面的整个 HTML 里找 Suspense 边界的标记，这在 tifa 示例商店的一次渲染里约占 1%。

为什么结果不变：循环仍然在搜索返回的下标处自己做一次比较，所以匹配一定是 `string_cmp` 确认过的，搜索返回的是沿循环方向第一个字符相同的下标，也就是循环自己会找到的第一个。16 位字符的字符串可能只含 0x100 以下的字符，所以不能靠两个字符串的宽度来判断有没有匹配。`string_indexof`（`split`、`replace` 和 `replaceAll` 用它）保持原样：它本来就会跳到第一个字符的下一个位置，而且它处理的字符串大多很短，调用的开销比省下的还多。

在 1 MB 的字符串里什么也找不到的 `indexOf`、`lastIndexOf` 或 `includes` 循环，以前在 10 ms 超时之后分别要过 170 ms、6.9 s 和 170 ms 才停，在 16 位字符的字符串里是 340 ms：每次搜索无论字符串多长，都只算一次轮询。现在它们在开始后 20 ms 内就停下，包括构造 1 MB 字符串的时间。`startsWith` 和 `endsWith` 只在一个下标处比较，不计数。对比不带计数的搜索，在 3 种代码布局下跑 6 轮，每次约 20 个字符的搜索，中位数用时是原来的 0.98 到 1.02，这和 `replace` 与 `split`（它们运行的代码一点没改）的波动一样大（1.031 和 1.006），在 1 KB 和 100 KB 里搜索是 0.995 到 1.002。`indexOf` 或 `lastIndexOf` 的未命中，在计数的两个构建里都慢了 1.7%，约 2 ns，这在上述波动范围内，但可能是真的。

在不含该字符串的 100 KB HTML 里查找，用时 81 µs，以前是 320 µs，HTML 含 16 位字符时是 87 µs，以前是 271 µs。在约 20 个字符的字符串里未命中，`indexOf` 用 108 ns，以前是 164，`includes` 是 102，以前是 119，1 KB 里的 `lastIndexOf` 是 1.6 µs，以前是 3.1，`startsWith` 是 102 ns，以前是 107（linux/amd64，AMD EPYC 7B13，单核，不开 PGO，每项都是用 JavaScript 循环调用同一个函数）。在 tifa 商店六个工作台页面的 Go CPU profile 里，`String.prototype.includes` 从占采样的 1.06% 降到 0.27%：它自己的搜索从约 0.75% 降到 0.13%，剩下的主要是把 HTML 的 rope（几段字符串连成的树，读的时候才拼成一段） 合成一个字符串，渲染之后本来也要读它。在 tifa 测试应用的 MUI 页面上，从 0.63% 降到 0.11%。商店六个页面的一次渲染用时是原来的 0.995（6 种代码布局跑 6 轮，0.975 到 1.032），MUI 页面是 1.003（0.970 到 1.024），都不开 PGO：收益比代码布局之间的波动小，光是代码布局就能让一次渲染变化约 2%。没有哪个页面或微基准变慢。

改动在 C 里（`engine/quickjs.c`），带有自己的搜索：`js_string_indexOf` 和 `js_string_includes` 调用 `js_string_find`，它只会返回已经比较过整个被查找字符串的下标，所以这个下标不用再比较一次。找一个字符时用 `memchr`，16 位字符的字符串里和向前找时用循环。最多 8 个字符的字符串，先用同样的办法找它的第一个字符，再在那里比较其余字符，每个下标最多 8 个单位的工作，这省掉了 Two-Way 搜索先要在被查找字符串上做的工作（每次搜索约 20 ns）。更长的字符串用 musl 的 `memmem` 里的 Two-Way 搜索，写成支持 8 位和 16 位字符、被查找字符串宽度不同的情况和两个方向。它的用时与两个字符串的长度成线性关系，不管内容是什么，几乎处处都匹配的被查找字符串也一样：在 linux/amd64（AMD EPYC 7B13）上，在第一个字符所在的每个下标处比较被查找字符串，`'a'.repeat(131072).indexOf('a'.repeat(65536) + 'b')` 要 2.5 s，Two-Way 搜索只要 0.5 ms。只有一个下标要看的搜索，例如 `startsWith` 和 `endsWith` 的，以及两个长度相同的字符串的，就在那里比较被查找字符串，不检查中断计数器：它的耗时与被查找字符串的长度成线性关系，`===`、拼接和其他与字符串长度成线性关系的操作也是这样，它们都不检查。`engine/test/compare/stringsearch.js`（由 `engine/test/compare.sh` 运行）用只有几个字母反复出现的长被查找字符串来搜索。`engine/test/unit.c` 检查这些事：在长字符串里搜索会运行中断处理函数，一次长搜索会调用它很多次（不管有没有匹配、字符串是哪种宽度、两种宽度混用都一样），处理函数要求停止时以错误 `interrupted` 结束，`try` 和 `catch` 无法捕获它，处理函数去掉之后上下文可以再次运行脚本和搜索，只有一个下标要看的搜索不调用它，以及几乎处处都匹配的被查找字符串，向前和向后搜索，在各种宽度的字符串和两种宽度混用时，都只要不到一秒的一小部分。

## symbol 有自己的字符串

版本：v0.30.2。

`Symbol(s)` 以前把字符串 `s` 本身改成 symbol，哪怕数组、变量还拿着它，或者它是函数里的数字字符串字面量（`Symbol("12")`，函数每次运行共用同一个字符串）。`description` 也把 symbol 本身当作字符串返回。不管哪种，脚本拿着的字符串都让 symbol 一直活着：只要字符串还在，指向它的 `WeakRef` 永远不变空，`FinalizationRegistry` 永远不回调。Node 在下一次回收时就释放这样的 symbol。

现在 symbol 和脚本看到的字符串不再共用字符。`JS_NewSymbol`（`engine/quickjs.c`）在别处还拿着 `s` 或者 `s` 是 atom 时，给 `Symbol(s)` 一份 `s` 的副本，`description`（`js_symbol_get_description`）返回的也是副本。symbol 字符串的其他用处，比如消息里的名字、`String(symbol)` 和方法名 `[desc]`，本来就另做一个新字符串，或者只在这次调用里用，照旧共用。超过一百万个字符的副本分步复制，每步之间读中断标志（`js_string_copy`，经过 `js_sub_string_long`）。不会死的东西照旧共用：注册过的 symbol（`Symbol.for`）不能被弱引用，以及运行时自带的 symbol（比如 `Symbol.iterator`）。字符串变成字符串 atom 时，不管谁拿着它都照旧原地变：字符串 atom 对每个持有者都还是同一个字符串。

代价：`Symbol` 运行时，它的参数还拿着传进来的字符串，所以用字符串做的 symbol 总是副本，按描述长度分配一次。不全是数字的字面量，比如 `Symbol('react.element')`，本来就是 atom，以前也要复制。每次读 `description` 都分配一份副本，原来是共用字符串。上游 QuickJS 两个方向都共用字符串。

`engine/test/compare/symbol_shared_string.js`（由 `engine/test/compare.sh` 运行）检查数组、变量和数字字面量拿着的字符串做出的 symbol，以及描述被拿着的 symbol，在做出各个 `WeakRef` 的任务结束后（`clearKeptObjects()`）会被释放，字符串仍是字符串，它们的 description 和 `Symbol.for`，还有几百万个字符的 8 位和 16 位描述。对照文件是 Node 24 的输出。`engine/test/unit-oom.c` 里的 `test_symbol_copy_oom` 在用被拿着的字符串做 symbol、读它们描述的每一处让内存耗尽，检查得到的是内存不足错误，而且什么都没留下。

## atom 的哈希表两个桶只放一个 atom

发布版本：Unreleased。

atom 是引擎给属性名或者只存一份的字符串编的号。用字符串做键，比如 `o['key' + i]`、JSON 的键或者解析器里的名字，都要在 atom 的哈希表里查这个字符串（`__JS_NewAtom`），沿着它所在桶的链往下走。上游在表里平均每个桶有两个 atom 时才把表加倍，所以一次查找在找到自己的 atom 或者确定没有之前，平均最多要走过大约两个 atom，每个都要从 atom 数组里读一次。现在 `engine/quickjs.c` 里的 `JS_ATOM_COUNT_RESIZE` 在两个桶有一个 atom 时就把表加倍。在原生 C 树的计数构建里，每次渲染沿链走的步数，MUI 订单页从 87K 降到 16K，MUI 首页从 38.5K 降到 5.4K，工作台 studio 页从 10.1K 降到 1.8K，商店英文首页从 6.7K 降到 0.9K。

为什么安全：只有表的大小变了，一个字符串得到哪个 atom、它的编号和键的顺序都不变。表还是由同一个 `JS_ResizeAtomHash` 加倍，只是换了一个时机，加倍失败时 atom 照旧建在现有的表里。商店、MUI 和 React 的 24 个页面 HTML 相同，test262 还是那 33 个已知错误。

代价：表占四倍的空间。在原生 C 树上，20 次渲染再做一次完整回收之后，商店的一个 VM 多占 196 KB（首页 13.65 MB，以前是 13.45 MB），MUI 的一个 VM 多占 49 KB 到 73 KB。footprint 不变。在新运行时里造 600 个名字（`test_key_names_oom`）所需的最小余量，从 62,096 字节变成 68,240 字节。

收益：一次渲染的指令数的变化不超过各轮之间的波动（在 Apple M4 上跑 tifa 的 `test/ssr`，三轮，每组都在 0.3% 以内）。沿链走的步数少了，就是读的次数少了，而每次读都可能缓存未命中，所以如果有收益，会体现在时间上，而不是指令上。

测试：`engine/test/unit-oom.c` 里的 `test_key_names_oom` 让一个造 600 个名字的脚本在从 0 到 80 KiB 的余量下各跑一次，这样表变大以后也有余量能让它跑完。`unit`、`unit-api`、`unit-white` 和 `unit-oom` 的其他检查都和以前一样通过。
