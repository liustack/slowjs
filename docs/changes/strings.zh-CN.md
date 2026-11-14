---
summary: '字符串搜索的改动：includes、startsWith、endsWith、indexOf 和 lastIndexOf 直接跳到字符串所在的位置，并把查看过的内容计入中断检查，收益是什么，为什么结果不变，symbol 和它的描述为什么不和脚本共用字符串，atom 的哈希表为什么两个桶只放一个 atom，+ 左边是空字符串时为什么直接给出右边的字符串，以及寄存器循环为什么不走通用路径就把两个字符串相加'
read_when:
  - String.prototype.includes、indexOf 或 lastIndexOf 给出的结果和以前不一样，或者在长字符串上很慢
  - 用字符串做出的 symbol，或者描述被拿着的 symbol，WeakRef 或 FinalizationRegistry 一直放不掉它
  - 一个 VM 给 atom 占的内存比以前多，或者接近内存上限的脚本在以前不会内存不足的地方内存不足
  - 左边是空字符串的加法，比如 '' + x 或以替换开头的模板字符串，给出的字符串和操作数是同一个
  - 两个字符串的加法给出的字符串、字符串种类或异常和以前不一样，或者一个数加一个不是数的值变慢了
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

发布版本：v0.30.5。

atom 是引擎给属性名或者只存一份的字符串编的号。用字符串做键，比如 `o['key' + i]`、JSON 的键或者解析器里的名字，都要在 atom 的哈希表里查这个字符串（`__JS_NewAtom`），沿着它所在桶的链往下走。上游在表里平均每个桶有两个 atom 时才把表加倍，所以一次查找在找到自己的 atom 或者确定没有之前，平均最多要走过大约两个 atom，每个都要从 atom 数组里读一次。现在 `engine/quickjs.c` 里的 `JS_ATOM_COUNT_RESIZE` 在两个桶有一个 atom 时就把表加倍。在原生 C 树的计数构建里，每次渲染沿链走的步数，MUI 订单页从 87K 降到 16K，MUI 首页从 38.5K 降到 5.4K，工作台 studio 页从 10.1K 降到 1.8K，商店英文首页从 6.7K 降到 0.9K。

为什么安全：只有表的大小变了，一个字符串得到哪个 atom、它的编号和键的顺序都不变。表还是由同一个 `JS_ResizeAtomHash` 加倍，只是换了一个时机，加倍失败时 atom 照旧建在现有的表里。商店、MUI 和 React 的 24 个页面 HTML 相同，test262 还是那 33 个已知错误。

代价：表占四倍的空间。在原生 C 树上，20 次渲染再做一次完整回收之后，商店的一个 VM 多占 196 KB（首页 13.65 MB，以前是 13.45 MB），MUI 的一个 VM 多占 49 KB 到 73 KB。footprint 不变。在新运行时里造 600 个名字（`test_key_names_oom`）所需的最小余量，从 62,096 字节变成 68,240 字节。

收益：一次渲染的指令数的变化不超过各轮之间的波动（在 Apple M4 上跑 tifa 的 `test/ssr`，三轮，每组都在 0.3% 以内）。沿链走的步数少了，就是读的次数少了，而每次读都可能缓存未命中，所以如果有收益，会体现在时间上，而不是指令上。

测试：`engine/test/unit-oom.c` 里的 `test_key_names_oom` 让一个造 600 个名字的脚本在从 0 到 80 KiB 的余量下各跑一次，这样表变大以后也有余量能让它跑完。`unit`、`unit-api`、`unit-white` 和 `unit-oom` 的其他检查都和以前一样通过。

## `+` 左边是空字符串时直接给出右边的字符串

发布版本：v0.30.5。

`'' + x` 是 React 和很多代码把一个值变成字符串的写法，以替换开头的模板字符串（`` `${x}px` ``）也编译成空字符串后面跟同样的 `+`。QuickJS 在右边为空时直接给出左边的字符串，在左边为空而右边是 rope 时直接给出右边的。左边为空、右边是普通字符串时，走的是 `JS_ConcatString2`，它在能做到时把左边的字符串原地加长。空字符串几乎总是 atom 的那一个，从不原地加长，所以它会分配一个新字符串，把右边的复制进去。现在 `JS_ConcatString`（`engine/quickjs.c`）在右边是不超过 512 个字符的普通字符串时，释放左边的空字符串，直接给出右边的。右边更长时照旧做成 rope：这在页面上很少见。

为什么安全：字符串没有脚本能看到的身份，结果的字符和宽度都一样。右边的字符串现在多了一个持有者，所以原地加长字符串的拼接（`js_concat_string_in_place`，它要求除了自己知道的持有者之外没有别人）会复制它，而不是改它。结果现在可能是 atom 的字符串，这些拼接本来就不碰 atom。留住结果的地方现在留住的就是右边那个字符串：`f` 为空时 `new RegExp(p, f + 'g')` 放进正则缓存的是 atom 的 `'g'`，不再是一份拷贝，所以语料里有三个脚本（`q3-regexp-prefix.js`、`q3-regexp-skip.js` 和 `shikire.js`）结束时多一个 atom，对象数不变，它们的 golden 重新写过。正则缓存留着这样的 flags 字符串时，最多比原来那份拷贝多占 32 字节和一个 atom，几个缓存条目共用这个 atom 时反而更少，因为以前每个条目各有一份拷贝。两个正则脚本的注入 golden 也因此重新写过。

收益，在 3ae98161 的原生 C 树上数的（linux/amd64 callgrind，整个进程，一次预热之后三次和一次计时渲染之差）：React 的页面每次渲染做 2,151 次这样的加法，指令少 1.28%，MUI 的三个页面 4,306 次、0.49%，十四个商店页面 2,426 次、0.17%，六个工作台页面 1,018 次、0.21%，每次加法大约省 150 到 210 条指令。表格见 [performance.md](../performance.zh-CN.md#左边是空字符串的加法)。

测试：`engine/test/unit-white.c` 的 `test_string_empty_left` 检查 `'' + s`、空字符串局部变量的 `e + s` 和 `t = t + s`、`''.concat(s)` 和 `` `${s}` `` 给出的就是字符串 `s` 本身，并检查对结果的原地拼接（`add_loc`，以及加到局部变量里）不改动 `s`（`s` 有 76 个字符，超过不做原地拼接的 64）。把 `js_concat_string_in_place` 的检查放宽一个持有者，加到局部变量里那一例就会改动 `s`，测试失败。

## 两个字符串的加法不走通用路径

发布版本：v0.30.7。

在寄存器循环里，操作数不是两个数的加法都交给 `rv_binop_slow`，也就是加、减和比较共用的慢路径：它重新读一遍操作码和操作数，先看是不是数，再看是不是两个字符串，最后才调用 `JS_ConcatString`。页面上的加法大多是两个字符串相加，拼 HTML、类名和键。现在加法的处理代码把两个字符串交给 `rv_add_str`（`engine/regvm-ops-basic.h`），它只做这一件事：按 `rv_binop_slow` 的方式取操作数（临时值直接拿走，局部变量、参数和常量复制一份），调用同一个 `JS_ConcatString`，结果写进局部变量或参数时调用 `js_add_string_into`，它可能把局部变量的字符串原地加长。处理代码在调用前看一个操作数的标签，`rv_add_str` 两个都看。有一个不是字符串时，它什么也不做就返回，处理代码照旧调用 `rv_binop_slow`。

加法的各种形式，以及现在各自怎么走：

- ADD（结果放进临时值）、ADDL（放进局部变量或参数）、ADDK 和 ADDKL（常量在右边，`x + 'px'`）：处理代码看左操作数。四种都交给 `rv_add_str`。
- KADD（常量在左边，`'<' + x`）：处理代码看右操作数，因为左边是常量。交给 `rv_add_str`。
- ADDLOC 和 ADDLOCK（局部变量上的 `s += x`）以及 ADDVR 和 ADDVRC（闭包变量上的 `s += x`）本来就在自己的辅助函数 `rv_addloc_slow` 和 `rv_addvr_slow` 里加两个字符串，不经过 `rv_binop_slow`，没有改。
- `o.s += x` 和 `o[k] += x` 是一次读、一次 ADD、一次写，所以走新路径。SUB、SUBL、SUBK、SUBKL 和 KSUB 都不走。

每个页面都会跑到改过的这五种形式。MUI 订单页一次渲染通过它们加两个字符串 34,500 次，它的首页 16,600 次，商店页 13,700 次（首页）和 19,400 次（商品页），工作台页 7,300 次（studio）和 3,900 次（stats），React 的页面 2,700 次。其中 KADD 在 MUI 订单页占 10%，在 React 的页面占 65%。表格见 [performance.md](../performance.zh-CN.md#两个字符串的加法不走通用路径)。

为什么安全：条件是两个操作数的标签都是字符串或 rope（`JS_IsString`），别的什么都不看。这时两个都是原始字符串，加法不会运行脚本的任何代码：不调用 `valueOf`、`toString` 或 `Symbol.toPrimitive`，`String.prototype` 上的也不调用。结果和 `rv_binop_slow` 对同样的操作数给出的一样，因为对它们调用的是同一个函数：同样种类的字符串（8 位或 16 位的普通字符串，或者按同样的长度规则做成的 rope），同样的字符，同样地原地加长局部变量的字符串，超过 2^30-1 个字符时同样抛出 `InternalError: string too long`，内存用完时同样抛出异常。atom 的字符串，以及记着自己作为键的 atom 的字符串，在这里和别的字符串一样：结果是一个新字符串，左边为空时就是右操作数本身（[见上文](#-左边是空字符串时直接给出右边的字符串)）。任何一边是 String 对象、数、BigInt、Symbol 或别的值，都照旧交给 `rv_binop_slow`。`rv_add_str` 只有一个可以抛出的位置，就是 `rv_binop_slow` 在两个字符串相加时的那一个，在取完操作数之后，所以抛出注入的位置和它们的 golden 不变。`JS_RegvmRun` 的帧不变，影子栈在 amd64 上 256 字节，在 arm64 上 176 字节。

代价：一个数加一个不是数的值（`n + ''`）在进 `rv_binop_slow` 之前多看一个标签，大约 3 条指令。被看的那个操作数是字符串、另一个不是的加法（`s + n`、`s + obj`、`1 + s`）还要多付一次 `rv_add_str` 的调用，它立刻返回。七个页面里，不是两个字符串、走 `rv_binop_slow` 的加法最多的是 store 商品页，每次渲染约 3,100 次，其中只有一部分要付这个调用。微基准 `int_to_string2` 在原生构建上多跑 0.7% 的指令。处理代码里两个整数或两个浮点数相加的部分，指令和以前一样。这个改动动了 `JS_RegvmRun` 的 C 代码，所以在重新训练 tifa 页面的性能画像之前，生成器编译这个函数时不用画像，页面的指令反而多 0.6% 到 1.1%（[performance.md](../performance.zh-CN.md#两个字符串的加法不走通用路径)）。

这和[试过但没做的](../performance.zh-CN.md#试过但没做的)里的「`rv_binop_slow` 只传 5 个字」不是一回事：那个分支给每一次走慢路径的加法和比较少传参数字，因为 clang 换了 `JS_RegvmRun` 里严格比较处理代码的排布而没有采用。这个改动不碰 `rv_binop_slow` 和对它的调用，只是把两个字符串的加法从它那里拿走。

测试：`engine/test/compare/string_add.js`（输出和 Node 24 核对过）用 13 种形式（临时值、局部变量、参数、局部变量在任一边、局部变量上的 `+=`、属性、元素、连加）和任一边 8 种常量，把 12 种字符串（空串、atom 的、8 位和 16 位普通字符串、单独的代理项、对象的键、600 和 9,000 个字符、8 位和 16 位的 rope）两两相加，还有不是字符串的操作数（带 `valueOf`、`toString` 或 `Symbol.toPrimitive` 的对象、String 对象、数组、数、`-0`、`NaN`、布尔值、`null`、`undefined`、BigInt、函数、会抛出的 Symbol），并替换 `String.prototype` 的方法，表明一个都不会运行，严格和非严格代码都有。`engine/test/regvm/js/r-stradd.js` 检查每种形式的长度上限，出错后要写入的局部变量还是原来的字符串，操作数也没有变，并让抛出注入打到每种形式（`inject-hit` 行）。`engine/test/unit-white.c` 的 `test_string_add_kinds` 检查每种形式给出的字符串种类（rope 还是普通，8 位还是 16 位）和字符都和 `JS_ConcatString` 一样。`engine/test/unit-oom.c` 的 `test_string_add_oom` 让每一次分配轮流失败来跑这些形式，检查结果或异常，出错后要写入的参数还是原来的字符串或者是完整的新字符串，原地加长的字符串对它的其他持有者没有变，只有局部变量持有的字符串不管是原地加长还是带余量复制，都完整地加长了。

## 寄存器循环里直接读字符串的一个字符

版本：Unreleased。

`s[i]` 里 `s` 是字符串、`i` 是 int 时（寄存器循环的 GETEL、GETELT、GETELK 和 GETELKT），以前要走 `rv_getel_slow`，再到 `JS_GetPropertyValue`、`JS_ValueToAtom` 和通用的 `JS_GetPropertyInternal`，由它做出 `i` 处那个码元的字符串。React 的页面每次渲染这样读 1,002 个字符，商店页面 804 个（首页）和 1,168 个（商品页），工作台页面 440 个（studio）和 195 个（stats），MUI 页面 752 个（订单）和 97 个（首页），长 Markdown 1,856 个。现在循环自己的数组路径答不了时会调 `rv_getel_atom`，它把以 int 为键的字符串交给 `rv_getel_str`（`engine/regvm-ops-object.h`）：下标在长度之内时，它做出一个码元的字符串（码元小于 256 用 8 位，其余用 16 位），放进目标。单独的代理项给出它自己那个码元，和以前一样。

为什么安全：字符串长度之内的下标，是这次读取把字符串转成的 String 对象的自有属性，不可配置也不可写（规范里的 StringGetOwnProperty）。所以 `String.prototype`、`Object.prototype` 或者脚本给 `String.prototype` 换上的原型上的下标、getter 或 Proxy，都没有机会先回答，而转成对象这一步不跑任何代码。键是 int，转成属性键也不跑代码。结果和 `js_get_property` 对同一次读取做出的字符串一样：同一个码元，同样的宽度。其余情况都照旧交给 `rv_getel_slow`：长度之外或负的下标（要去问 `String.prototype` 和它后面的对象，它们可能有这个键），rope（它有自己的标签），键是 double、字符串或 symbol，String 对象（它是对象，走对象的路径），以及其他原始值。页面上以 int 为键读字符串的每一次都在长度之内：没有一次在长度之外、是负数或者是 rope。没有内存做这个字符串时，`rv_getel_str` 什么都不做就返回，也不留下异常，由 `rv_getel_slow` 再做一次并抛出内存不足的错误。循环只在测试的抛出注入没有启用时才调 `rv_getel_atom`，和以前一样，所以注入的位置和它们的 golden 都不变。`rv_getel_str` 是单独的函数，由 `rv_getel_atom` 最后调用：寄存器循环的代码和以前一样，帧也一样（amd64 上 SHADOW 256，arm64 上 176），`JS_CallInternal` 也一样（112 和 96）。

收益：React 的页面在 amd64 原生构建上少跑 0.50% 的指令（M4 上 0.47%），tifa 的页面少跑 0.04% 到 0.10%，V8 套件的 Crypto 少跑 0.12%。表格在 [performance.zh-CN.md](../performance.zh-CN.md#寄存器循环里读字符串的一个字符)。

测试：`engine/test/compare/string_index.js`（内容就是 Node 24 的输出）读 8 位和 16 位字符串、单独的代理项、rope、100,001 个字符的字符串和空字符串，键是常量、局部变量和临时值，字符串是局部变量和临时值，下标取遍每一个位置，还有 -1、-0、小数、2^31 - 1、2^31、2^32 - 2、2^32 - 1、2^32、2^53、`NaN` 和正负无穷，键也有字符串（`"0"`、`"01"`、`"1.0"`、`"-0"`、`" 1"`、`"length"`、方法名、`"__proto__"`）和 symbol，接收者也有 String 对象、下标超出长度的 String 对象、给某个下标定义了 getter 的 `String` 子类、`String.prototype` 和 `Object.prototype` 上的下标和 getter（getter 能看到接收者是那个字符串）、作为 `String.prototype` 原型的 Proxy、数、布尔值、BigInt、Symbol、`null` 和 `undefined`。`engine/test/regvm/js/o-getel-string.js` 用 -2 到超出长度的每个 int 和不是下标的键去读 13 个字符串（其中一个来自另一个 realm），覆盖 GETEL 的每种形式，结果也写进键和字符串所在的槽，每次读取都和对 String 对象做的 `Reflect.get` 比较：原型不变时、本 realm 的原型上有下标和 getter 时、`String.prototype` 的原型是 Proxy 时，以及另一个 realm 的 `String.prototype` 上有下标、由那个 realm 的函数去读时。`engine/test/unit-oom.c` 的 `test_string_index_oom` 在 16 KiB 以内的每种剩余空间下和堆满时读 8 位和 16 位字符串的字符，检查结果或者内存不足的错误。
