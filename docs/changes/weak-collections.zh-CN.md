---
summary: '完整 GC 如何回收 WeakMap 的 ephemeron，不会通过一个不可达的键把它的值留住，以及 WeakRef 如何把目标留到任务结束'
read_when:
  - WeakMap 的值在键已经不可达之后仍然活着
  - WeakRef.prototype.deref 给出意料之外的 undefined 或对象，或者宿主要结束一个任务
---

# 弱集合

[English](weak-collections.md) · 简体中文

发布版本：v0.27.2。

完整回收时，只要一个 WeakMap 条目的键没有从根出发的强引用路径，就会释放这个条目，即使它的值反过来指向这个键。条目链在同一次回收里解决。要保住一个条目的值，map 和它的键必须都可达。WeakSet 用同样的键清理。

以前，`gc_remove_weak_objects` 在试探删除之前运行，并且检查键的引用计数。然后 `js_map_mark` 把每个值都当作它所在 map 的强子节点。一个活着的 map 留住一个值，这个值又留住它的键，所以重复做完整 GC 也打不破这个环。一个只被另一个环持有的键，要再回收一次，它的条目才会消失。

## 上游

Bellard 在 2026-10-22 的最新发布是 [2026-06-04](https://bellard.org/quickjs/Changelog)。它在 GitHub 上的 master `7d1c13c5d8f8c6b32893786ed35cdcebd88194d2` 仍然从 map 出发标记弱 map 的值。

QuickJS-NG 在 [a57877c, Retain WeakMap values only when key is reachable](https://github.com/quickjs-ng/quickjs/commit/a57877cef88c3d80aadf69153209b9f0dfc6324f) 里修了反向引用的情形。v0.17.0 和 master `270b8c974a4427726e7b580349ae061af08f53a5` 改成从对象键出发标记这些值。liustack/quickjs 沿用这种条件边的办法。它用一个临时的键索引，因为 Bellard 当前的弱引用布局是运行时上的一条列表，而不是每个目标一份记录。它还处理了 symbol 键，并检查 map 是否可达。

## 回收器

试探删除仍然减去每个被拥有的值。在存活扫描时，弱 map 的值先不标记。一个可达的 map 会标记键已经被扫到的条目的值，把其他条目登记在临时索引里它们的键下面。扫到那个键时，激活它的条目，并把新可达的对象追加到回收器已有的队列里。每个条目最多登记一次、激活一次。这样一条很长的链不用反复重扫所有条目。扫描循环本身就是 `gc_scan` 的循环，只对弱 map、弱 set 和作为弱目标的对象多看一步。

Symbol 有引用计数，但不是 GC 对象列表上的节点。弱集合有 symbol 键时，先做一遍预备扫描，统计 GC 管理的字段和 shape（引擎记录一个对象有哪些键、按什么顺序的结构） 属性名对这些 symbol 的引用。剩下的引用就是根。存活扫描也会沿着值为 symbol 的字段和 symbol 属性名走。没有 symbol 键时跳过这一遍额外的扫描。

在释放任何东西之前，回收器会恢复所有被暂扣的引用和垃圾持有的引用。仍然登记着的条目恰好就是键已死的那些。它在禁用递归释放对象的情况下移除它们，然后清扫环。死键可能带着试探删除的标记，也可能是只靠弱引用保持分配的外壳，强引用计数为零。试探删除之前运行的弱引用清理跳过弱 map 和弱 set，所以键是外壳的条目会经过扫描，和其他条目一起被移除。有变化的 map 只重建一次哈希桶，避免在冲突链里反复查找。最后它清除已死的 WeakRef，并把 FinalizationRegistry 的任务排进队列。回收过程不执行 JavaScript。回调仍然通过待处理任务的执行来运行，unregister 仍然会让它们不再触发。

键索引、已登记条目的列表、symbol 表和回调只在回收期间存在，其中索引和列表只在有条目需要等待时才创建。对那次回收来说，在 64 位目标上每个弱 map 条目占 56 到 88 字节。C 内存里不存任何 Go 指针。运行时存一个阶段字节。没有 WeakMap、WeakSet、WeakRef 或 FinalizationRegistry 的运行时走原来的回收路径。弱 map 和弱 set 都为空的运行时执行原来的步骤，并在清扫之后再清除一次 WeakRef、再把 FinalizationRegistry 的任务排一次队。`JS_FreeRuntime` 仍走原来的拆除路径。

它和第一版的步骤给出同样的结果：第一版保留试探删除之前完整的弱引用清理，之后扫描每个 map 的条目找死键，也没有上面这些降低开销的办法。第一版用临时哈希表和待处理条目存储来代替 Go map。如果试探删除之前 C 的临时分配失败，它会释放临时存储，并保留原来的回收路径。Go 的临时存储在 Go 堆上，不计入引擎分配器的内存计数，和 Go 一侧其他临时工作一样。

## 代价

在 linux/amd64、不开 PGO（AMD EPYC 7B13，单核）上，对比 main `453def5`（v0.27.1 的引擎），3 种代码布局跑 6 轮，用 tifa `b1cb68f`，每个页面一个 VM：

- 带 WeakMap 条目的 VM 做一次完整 GC，在 tifa 的页面上和以前用时差不多。每次渲染后都回收，商店页面每次 GC 用时是原来的 0.989（0.936 到 1.077），MUI 页面是 1.046（0.979 到 1.229），每次 2.0 到 3.6 ms。在 callgrind 下，这些回收在商店上多执行 1.0% 的指令，在 MUI 页面上多 0.3%。
- 渲染时间不变，因为这个修复只动 GC。对比 v0.27.1，v0.27.2 的完整渲染在商店页面上用时是原来的 1.006，MUI 页面上是 0.991，都在代码布局的波动之内。HTML 逐字节相同。
- 大的弱 map 代价更高。`BenchmarkEphemeronGC` 回收一条含 100,000 个活条目的链用 8.2 ms，以前是 2.8 (2.9 倍，2.7 到 3.0)，同样的链分散在 10,000 个 map 里用 10.5 ms，以前是 3.4 (3.1 倍)，由数组持有的 100,000 个活键用 14.4 ms，以前是 7.3 (1.9 倍)。100,000 个值指回不可达键的条目用 24.7 ms，并且被释放了：main 用 10.2 ms，一个也没释放。100,000 个不涉及弱 map 的普通环，用时是原来的 1.02。
- 剩下的大部分来自条件边：扫描要读每个键的头来看它有没有被扫到，键还没被扫到的条目要进索引，等键被扫到时再找回来，也就是在每条目 32 到 64 字节的表里随机位置读两次。main 两样都不读。GC 对象头里没有空闲的位，不能把等待中的条目挂在键上。
- 在默认池里渲染一个页面 2,000 次，内存不增长，之前和之后都是。一次 GC 之后，商店的一个 VM 保留 30 个弱 map 条目，main 最多保留 165 个，分配器内存最多少 0.9 MiB。再回收 20 次之内，main 会降到同样的内存。在 MUI 页面上，VM 保留 1 个条目，main 最多保留 69 个。使用中的内存相同：main 在两个 MUI 页面上持有更多分配器内存，在 `/orders` 上更少。

## 测试

`gc_ephemeron_test.go` 通过 `VM.RunGC` 检查一次完整回收。一个确定性的混合图测试拿一个独立的可达性判定做对照。

`BenchmarkEphemeronGC` 测量 10,000 和 100,000 个条目的新图，准备和清理都不在计时之内，包括同一个 map 里逆序的活链、每 10 个条目分散在 1 个 map 的活链、由数组持有的活键，以及不带弱集合的普通环。

改动这些路径时，必须检查针对性的和完整的 test262 结果以及渲染测量。

## WeakRef 把目标留到任务结束

发布版本：未发布。

规范要求 `WeakRef` 的目标一直活到创建这个 `WeakRef` 或调用它 `deref` 的那个任务结束（AddToKeptObjects，任务结束时 ClearKeptObjects），这样脚本在这个任务剩下的部分里从 `deref` 读到的答案不变。QuickJS 从来没有这张表，上游 QuickJS 和 quickjs-ng 的最新提交也没有：引用计数在目标的最后一个其他引用消失时就把它释放了。所以 `new WeakRef({}).deref()` 和 `const w = new WeakRef(o); o = null; w.deref()` 在同一个任务里得到 `undefined`，而 Node 24 得到那个对象。test262 看不到这个问题：它测 `WeakRef` 的用例都把目标存在变量里，33 个已知错误里也没有和 `WeakRef` 有关的。

现在运行时把一个任务的目标放进一个按地址查找的集合（`rt->kept`），一个目标读很多次也只留一份。构造函数和 `deref` 把各自的目标加进去（`js_keep_target`）。这个集合计入内存上限：它长不了的时候，这次调用以内存不足错误失败。`engine/quickjs.h` 新增的 `JS_ClearKeptObjects` 放掉这些目标并释放集合。只有宿主知道它的任务在哪里结束，所以由宿主调用它：

- Go API 在最外层的 javascript 调用回到 Go 时调用它（`entry.go` 的 `leave`），`Eval`、`Call`、`ExecutePendingJobs` 和其他每个入口之后都是，最外层调用里 `ExecutePendingJobs` 或要等待的 `Eval` 运行每个任务之前也是。它在这次调用的入口之内运行，受调用的 eval 超时约束，和转换结果一样。javascript 调用的 Go 函数，或者 `Call` 为转换参数而运行的 Go 代码，都在这次调用的任务之内，所以目标对它和它求值的 javascript 仍然在，它运行任务时也是：那些任务在这次调用的任务之内运行。
- `JS_ExecutePendingJob` 不调用它，因为它看不出宿主任务里的 javascript 是否还在下面运行。`engine/test` 的测试宿主（`jsrun`、`render`、`run-test262` 和各个 unit 程序）像 Go API 一样，在脚本之后、每个任务之前调用它。`run-test262` 还提供 `$262.clearKeptObjects`。
- `JS_FreeRuntime` 最先调用它。

所以一个池子拿来做很多次渲染的 VM，不会把目标从一次调用留到下一次，调用以错误结束时也什么都不留。被 kill 之后只能 Close 的 VM 在 Close 时放掉它们。宿主仍通过 `Value` 持有的目标，和其他对象一样留着。

在后面的任务里读 `deref` 的程序看到的和以前一样。引擎里那些创建 `WeakRef`、丢掉目标却没让任务结束的测试，现在先结束任务（`engine/test/unit.c` 的 `test_free_order`，`engine/test/unit-white.c` 的 `test_young_leaves_no_zero_count`）。

### 代价

在 linux/amd64 上用 callgrind 对比 main `32307c7e`，两边都用 `-pgo=off` 构建，计 2 次渲染之后的 10 次：tifa 的 bench 页面渲染一次执行 265,495,178 条指令，main 是 265,470,587（+0.01%），其中引擎里 263,961,491 条，main 是 263,960,294 条。React 页面渲染一次执行 51,089,722 条，main 是 51,091,721（0.00%），其中引擎里 50,726,736 条，main 是 50,730,514 条。没有创建 `WeakRef` 时，一次调用或一个任务结束只是一次进入引擎的调用，发现没有集合就返回。商店（20 个页面）、MUI（3 个页面）和 bench（2 个页面）的 HTML 逐字节相同。任务运行期间，在 64 位目标上集合每个目标占 32 到 64 字节（每个槽 16 字节，最多半满），任务结束时全部还回去。

### 测试

- `TestWeakRefKeptDuringJob`（`weakref_test.go`）：新 `WeakRef` 的目标和 `deref` 的目标在它所在的调用里、在这次调用运行的 Go 函数里、在这个函数求值的 javascript 里、在 Go 代码于调用之中运行过任务之后（Go 函数，或者 `Call` 参数的 `MarshalJSON`）都在，下一次调用里就没了。`ExecutePendingJobs` 的每个 promise 任务各是一个任务。`TestWeakRefKeptReleasedAfterTimeout`：被 eval 超时停下的调用也会放掉它的目标。
- `TestWeakRefKeptReleasedAfterCall`：在一个 VM 上调用 200 次，每次创建一个指向 65,536 个数字的数组的 `WeakRef`，GC 之后留下的内存不比第一次调用之后多。
- `test_weakref_kept`（`engine/test/unit-api.c`）在 C 树里用一个像 Go API 那样结束任务的宿主检查同样的事，还有 symbol 目标、从 javascript 里面运行任务的宿主函数、任务之中的完整回收和年轻代回收、一个任务里 10,000 个目标各读 100 次，以及在保留着目标时释放运行时。`test_weakref_kept_oom`（`engine/test/unit-oom.c`）在 64 KiB 以内的每种余量下创建并读取 200 个 `WeakRef`，或者读取之前一个任务里创建的 200 个：每次运行要么得到总和，要么得到内存不足错误，并且不留下任何块。
- 上游 `test_builtin.js`（`engine/test/compare/upstream/`）的 `test_weak_ref` 在丢掉目标之后还能读到它，用 `jsrun` 的 `clearKeptObjects()` 结束任务，然后读到 `undefined`。`gc_ephemeron.js` 和 `young_gc.js` 在每次要求回收之前先结束任务。
