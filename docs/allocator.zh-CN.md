---
summary: 'engine/ 里 C 引擎的内存分配器：它的分层，MemorySize 和内存限制数的是什么，内存块和占用预算怎么工作，内存怎么还回去，前端缓存，已释放大块的缓存，钉住的 TLSF，以及怎么测试它'
read_when:
  - 改 engine/heap.c、engine/heap.h、engine/tlsf.c 或 engine/quickjs.c 里分配器的部分
  - 问 C 引擎里的内存限制、MemorySize 或 GC 阈值数的是什么
  - 问一个 VM 从宿主拿走多少内存，以及什么时候还回去
  - 用 sanitizer 在 C 引擎里找内存 bug
---

# C 引擎的分配器

[English](allocator.md) · 简体中文

`engine/` 里 C 引擎的分配器。Go 宿主和原生测试宿主（`engine/test/`）都用它。

## 分层

```
js_malloc, js_free, js_realloc, js_malloc_usable_size (and the _rt forms)
  front cache: 32 lists of up to 256 freed small blocks       quickjs.c
  cache of large blocks: 57 lists of up to 8 freed blocks     quickjs.c
    of 505 to 4096 bytes, JS_LARGE_CACHE_BYTES in all
  small blocks, 16 to 512 bytes: arenas of 4 KiB               quickjs.c, as upstream
  arenas and every block over 504 bytes: TLSF                  tlsf.c
  chunks, counts, limit, budget, resize, release               heap.c
host_memory_grow, host_call (memory release, remap)            the host
```

- 小块这一层是上游的：31 个大小类，每块 8 字节的头，以及每个块的用户数据之前 4 个字节处的引用计数（`quickjs.h` 里的 `JS_BLOCK_REF_COUNT_OFFSET`，对照 `JSMallocBlockHeader` 检查）。堆内存按 8 字节对齐。
- 前端缓存把释放的小块放在它所属大小类的链表上，这样下一个同类的块就不需要占用 arena。`JS_RunGC`、`JS_RunYoungGC`、`JS_ReleaseStart` 和 `JS_FreeRuntime` 会把它还给 arena，堆分配失败的那次请求也会：缓存回到 arena，再向堆问一次。在到达堆之前就被拒绝的请求（大小为 0，大小超过最大值）不这样做。`JS_SetFrontCache(rt, max)` 把缓存还回去，并设置每条链表的长度（0 到 256，0 表示没有缓存）。大块缓存（见下）在同样的时刻归还，`JS_SetFrontCache(rt, 0)` 也会把它关掉。
- 大块是前面带 8 字节头的 TLSF 块。下一个块空闲时，TLSF 可以原地扩大它。
- 所有状态都在运行时里。运行时放在第一个内存块里，所以没有全局变量，没有锁，也没有系统调用。堆向宿主要 `host_memory_grow(vm, size)`：`size` 个字节，64 KiB 的倍数，可读写，按页对齐，不清零，系统没有内存时返回 0。对于专用内存块（见"内存块"），它还用两种 `host_call`（`engine/host.h`），它们不运行用户代码，也不回调引擎：`HOST_CALL_MEMORY_RELEASE`（地址，大小）归还一个内存块，`HOST_CALL_MEMORY_REMAP`（地址，旧大小，新大小）把它变大。两者传的都是宿主所持内存块的确切地址和大小。

## 数的是什么

- `count`（`JS_GetMemorySize`，`JS_ComputeMemoryUsage` 的 `malloc_size`）是运行时的大小，加上每个在用的块的 TLSF 块大小以及它前面 8 字节的大小字。一个 arena 整个计入，前端缓存里的槽也在内。在大块缓存里等着的已释放大块也计入（见下）。空闲内存和内存块的头不计入。内存限制用的就是 `count`。GC 阈值用的是 `count` 减去寄存器栈（`rv_stack_bytes`），因为回收释放不了它。
- 内存限制（`JS_SetMemoryLimit`）在 `count` 加上所要求的大小（arena，或者用户大小加上 8 字节的头）超过它时，让这次分配失败，然后再计入真正的块。调整大小的检查方式和上游一样：`count` 加上新大小，减去旧块的可用大小，与限制比较（旧块不会算两次，它的 8 字节字留在 `count` 里，因为这个块仍然是一个块）。调整大小完成后，`count` 变化两个可用大小之差。调低限制不会释放任何东西。失败会让引擎抛出内存耗尽。
- 占用是 VM 从宿主拿走的量，也就是它的各个内存块之和。`JS_SetFootprintLimit` 限制它，0 表示不限。它只阻止增长：持有更多的 VM 保持原样，它也不会收回任何东西。它只涵盖内存块，不含影子栈、共享字节码或 Go 一侧。没有默认值。
- TLSF 把块向上取整，最多多 1/32。和 glibc 的 malloc 比，差别在 1.3% 以内：渲染 tifa 的页面 3000 次之后，shop 占 39.84 MB，glibc 是 39.85，MUI 测试应用 29.91 对 29.52，bench 应用 2.15 对 2.12。

## 内存块

- 第一个内存块是 64 KiB，放运行时、TLSF 的控制结构和第一个池。之后的内存块是占用量的四分之一，在 64 KiB 到 8 MiB 之间，是 64 KiB 的倍数。接近预算时，共用的内存块缩小到请求所需的大小（加上 TLSF 找到它所需的余量，`need + need/32 + 112 + 64`，向上取整到 64 KiB），即使这样也装不下时就被拒绝。
- 超过下一个内存块大约一半的请求（所以长期来看就是超过 4 MiB 的）得到一个专用内存块。确切的判断是拿 `need + need/32 + 64` 和内存块去掉头部和 TLSF 的字之后的一半比，`heap.h` 的 `HEAP_SHARED_MAX(chunk)` 给出一个新的共享内存块能接的最大请求：64 KiB 的内存块是 31,690 字节，128 KiB 的是 63,465 字节。不该单独占一个内存块的块按它定大小，寄存器栈的每一段就是这样（`engine/regvm.h`）：多一个字节，宿主就要为这一个块单独映射和解除映射一个内存块。专用内存块在头里标记（`kind`），只放一个块，不归 TLSF 管：32 字节的头，块的大小字，块本身（向上取整到 8 字节），一个结束标记，以及到下一个 64 KiB 为止的余地，块可以长进这块余地，别人得不到它。块按它的大小计入，不按余地计入。`heap_check` 和 `heap_contains` 两种内存块都认得。
- 专用内存块里的块被释放，或者被 `heap_realloc` 移走时，这个内存块在那次调用里就还给宿主（`HOST_CALL_MEMORY_RELEASE`）。堆先把内存块从它的链表和计数里取出来，再去问，之后绝不再读这个内存块。没有已释放专用内存块的缓存。调整大小时先得到新块并复制，再归还旧的内存块。
- 放得进内存块、并且用掉至少一半的块，留在其中。当它长得超出了内存块，并且宿主说可以重新映射（`JS_SetHostRemap(rt, 1)`，在 `JS_NewRuntime` 之后立即调用，linux），堆只针对增长的部分检查预算，并请宿主重新映射这个内存块（`HOST_CALL_MEMORY_REMAP`，带 `MREMAP_MAYMOVE` 的 `mremap`），这样内容保留，地址可能改变。宿主拒绝就是内存耗尽：调整大小失败，什么也不变，没有复制。没有重新映射，或者块缩小到不足内存块的一半时，堆取一个新块，复制，并归还旧的内存块。正在增长的块在它的新内存块里得到它大小的八分之一作为余地，这样一个一点一点增长的缓冲区，不会每一步都得到新的内存块、一次复制或一次系统调用（预算可以收走这块余地）。
- 一次调整大小，如果不能原地扩大，也不能在内存块之间移动，就需要一个新的内存块，而旧块还在它的内存块里，所以预算可以拒绝计数允许的调整大小。被拒绝的调整大小，会让旧块、它的地址、内容和可用大小保持原样，堆的 `count` 也是。在运行时这一层，失败的请求先归还前端缓存，这可能清空 arena 并降低 `count`，所以只有堆本身才保证被拒绝的调用不改变计数。
- `JSHeap.stop` 存着一个中断标志的地址时，要移动的块每次复制 1 MiB（`heap_copy`，`HEAP_COPY_STEP`），每步之前读一次这个标志。运行时只在改 `ArrayBuffer` 的大小时设置它（`js_array_buffer_realloc`），其他时候都是 NULL。不到 1 MiB 的复制不读。标志置上时，新块还回去，专用内存块立刻还给宿主，这次调整大小像被拒绝的那样失败，并且在下一次调整大小之前 `stopped` 一直置着。运行时这时不会在归还前端缓存之后再试一次，而是抛出中断（见 [uncatchable.md](changes/uncatchable.zh-CN.md#改变很大的-arraybuffer-的大小时读中断标志)）。
- 最大的请求是 2^32 - 8 个字节：更大的在对大小做任何运算之前就失败。

## 把内存还回去

专用内存块立刻归还（见"内存块"）。共用的内存块不归还：除非宿主要求，否则它们什么也不还。`JS_ReleaseStart(rt, keep)` 先把前端缓存和大块缓存还给堆，然后计划提供除 `keep` 个字节之外的所有空闲内存。`JS_ReleaseNextChunk` 随后返回整个空闲的内存块（不含第一个），最大的先返回，把它们从堆里和占用量里取出来，直到它的长度为 0。`JS_ReleaseNextRange` 返回 4 KiB 或更大的空闲块的内部，宿主可以丢掉这些页：TLSF 从不读它，只在拆分或合并块时才往那里写。在此期间宿主不使用运行时，取消内存块的映射，并把每段范围向内取整到页的大小。一段范围可能在以后的归还中被再次提供。`JS_ComputeMemoryUsage` 的 `release_candidate` 汇总提供了多少。`keep` 是目标，不是确切的量：内存块按它们的整个大小计，范围按空闲块去掉 TLSF 的字之后剩下的计，而一段范围至少 4 KiB，所以一个很大的空闲块可以让总数越过目标。

用 realloc 增长、超过半个内存块的缓冲区，得到自己专用的内存块，这样它后面就不会一直映射着一串空的内存块。一个以 1 MiB 为步长构建的 1 GiB 字符串（`staging/sm/String/replace-math.js`），有重新映射时占 1.05 GB，用 1.2 秒（glibc：1.09 GB，1.2 秒），没有时占 2.0 GB，用 4.5 秒，因为旧的内存块要等复制结束。

## 大块缓存

505 到 4096 字节的大块被释放后，在它所属大小类的链表里等着，下一个同类的请求会取走它，TLSF 里不做任何工作。脚本构建的字符串和缓冲区就是这些大小，轮流创建又丢弃，而 TLSF 每次要做一次位图查找、一次拆分和一次合并。这个缓存在 `quickjs.c` 里，紧挨着前端缓存，前端缓存开着它就开着：`JS_SetFrontCache(rt, 0)` 把两个都关掉，任何其他值都让大块缓存保持原样。

- **大小类。** 每 64 字节一类，共 57 类。505 到 4096 字节的请求，在缓存开启时向上取整到 64 的倍数，这样它得到的块会归入以后再被请求的那个类（1000 字节的块有 1024 字节的空间）。一个块归入它的可用大小除以 64 再向下取整的那个类，不管它当初是按多大要的：被调整过大小的块，或者 TLSF 多给了一点的块，可以满足最大到它大小的那些类。可用字节数小于 512 或大于 4159 的块，被释放时回到堆里。
- **限制。** 每个类最多保留 8 个块（`JS_LARGE_CACHE_N`），所有的类加起来最多 256 KiB（`JS_LARGE_CACHE_BYTES`），按 TLSF 给这些块的大小计，包括 8 字节的头。会超过其中任何一个限制的块，立刻回到堆里，不会为了腾地方而淘汰任何块。这 256 KiB 是每个运行时的，而 tifa 在一个进程里运行好几个：n 个运行时最多保留 n 乘 256 KiB。为什么是 256 KiB：在速度测试的 37 个脚本上数命中次数。在 128 KiB 时，构建字符串的脚本损失 4 到 8 个百分点（`c_string_concat` 的大块请求命中 67%，没有上限时是 75%，React 页面是 93% 对 97%）。在 256 KiB 时，除 `v8_regexp` 之外的都在没有上限的结果的一个百分点之内，再往上只有 `v8_regexp` 还有提高（256 KiB 时 3%，1 MiB 时 8%）。每个类在 4 到 32 个块之间，命中率几乎不变。256 KiB 是 tifa 最大的应用渲染 3000 次之后占用的 40 MB 的 0.6%，是最小的那个 2 MB 的 12%。
- **数的是什么。** 就堆而言，缓存的块是在用的块。`count`（MemorySize，以及不含寄存器栈的 GC 阈值）、`blocks`（`JS_ComputeMemoryUsage` 的 `malloc_count`）和内存限制都把它算进去，`heap_check` 在在用的块里找到它，总和与其他任何块相同。`JS_CheckMemory` 加上堆看不到的东西：缓存的每个块都是堆的一个大块，归在它的类下，链表的计数和 `large_bytes` 就是各链表里放的东西。从缓存里取出一个块或放进一个块，这些计数一个都不变：一个块的代价在堆给出它时计入，在堆把它收回时扣除。
- **内存限制。** 缓存能满足的请求不给 `count` 增加任何东西，所以限制不看它，即使 `count` 已经超过限制（限制可以被调到 `count` 以下）。到达堆并被拒绝的请求，不管是被限制、占用预算还是宿主拒绝，都会归还两个缓存，再向堆问一次，和前端缓存的做法一样。所以缓存的块自己绝不会让一个请求失败，而失败的请求会让两个缓存都变空。这个范围内的请求，向堆要的是它取整后的大小，最多多 63 字节，限制看到的是这个大小。
- **归还。** `JS_SetFrontCache`（任何值），`JS_RunGC`，`JS_RunYoungGC`，`JS_ReleaseStart`（在它计划提供什么之前，这样放过缓存块的内存块可以空出来），`JS_FreeRuntime`（在它数剩下的块之前，这样缓存的块不算泄漏），以及每一次到达堆的失败的请求或调整大小。专用内存块从不在缓存里：它的块都超过 4 MiB。
- **毒化。** 在 `asan-heap` 里，进入缓存的块整个被毒化，取出时解除毒化。指向下一个块的链接放在块的前 8 个字节里，只在缓存读或写它的那一刻才打开，所以缓存块的头几个字节也能被发现，和前端缓存的不同。`asan`（每个块都来自系统的 malloc）没有大块缓存，因为它的要点就是每个块各自独立。

## TLSF

`engine/tlsf.c` 和 `engine/tlsf.h` 是 Matthew Conte 的 TLSF 3.1（BSD-3-Clause，见 `NOTICE.md`），提交 `deff9ab509341f264addbd3c8ada533678591905`（`tlsf.c` 的 SHA-256 是 `2a0f8cfc9cfe6114ccdc6cf22339059440b16f1149b5107bead4ae4c3a0d50e2`，`tlsf.h` 的是 `f7f73c48810ba60203095667c226e5a600a6ea0f69afba48efff6efbaa628d4f`，两者的行尾都是 CRLF），已转成 LF，并做了这些改动：

- TLSF 装不下的请求或池会失败（该项目的第 9、22 和 35 号 issue）。`engine/test/tlsf-edges.sh` 在钉住的文件上运行这些边界情况，其中有四个失败或崩溃，在 `engine/tlsf.c` 上则全部通过。
- 没有 `stdio.h`、`stdlib.h`、调试自测和 `tlsf_memalign`。
- 给堆加了四个函数：`tlsf_realloc_inplace`（不移动地调整大小），`tlsf_next_free`（遍历空闲块，最大的类先走），`tlsf_free_inside`（空闲块里 TLSF 从不读的那一部分）和 `tlsf_pool_is_free`。

## 测试

全部在原生测试宿主上运行（见 [testing.md](testing.zh-CN.md#c-树和它的原生测试宿主)）。

```sh
engine/test/tlsf-edges.sh                    # TLSF edge cases, pinned file and engine/tlsf.c
engine/test/build.sh plain                   # also asan-heap, asan-heap-iter, ubsan
engine/test/heap.sh -m plain                 # scenarios and random sequences, heap and runtime
engine/test/build.sh fuzz && engine/test/heap.sh -m plain -f 300   # the libFuzzer target
engine/test/build.sh asan                    # every block from the system malloc, with LeakSanitizer
engine/test/build.sh asan-heap               # the real allocator, free memory poisoned
engine/test/heap.sh -m asan-heap             # also poison-test: what the poison covers
engine/test/build.sh asan-heap-iter          # the same with JS_MALLOC_USE_ITER, which keeps a list of every allocated block
engine/test/heap.sh -m asan-heap-iter        # the macro must keep building and passing
engine/test/host-test.sh tsan                # the test host, make and free runtimes on two threads
```

- `heap-test.c` 有一个假宿主，它映射内存块，能拒绝第 n 次请求，并把每段归还的范围填成 0xdd。每个块里放着一个图案，在它被释放或调整大小时检查。每隔几千步，`heap_check` 和 `JS_CheckMemory`（用 `JS_HEAP_CHECK` 编译进去）遍历 TLSF、arena、前端缓存和大块缓存，测试把计数和它持有的块的总和比较。运行时上的随机运行也会请求大块缓存范围内的大小，把缓存的块算进失败的请求必须归还的内容里，并按请求取整到它的类之后的大小来判断一次拒绝。测试构建保留的一个计数（`JS_LargeCacheHits`）告诉我们缓存什么时候满足了一次请求，那时它必须从缓存里取了一个块。`scenario_large_cache` 把下一节的规则精确到字节。
- 在 `asan-heap` 里，引擎毒化空闲小槽的用户字节，空闲 TLSF 块的载荷，arena 的剩余部分，缓存的小块（除了指向下一个的指针）和大块缓存里的块（全部：前 8 个字节里的链接，只在分配器读或写它时才打开），这样就能发现我们自己内存里的释放后使用。`heap.c` 和 `tlsf.c` 在那里不带 AddressSanitizer 构建，因为毒化由它们自己放上和取下。`poison-test.c`（由 `heap.sh -m asan-heap` 运行）问 AddressSanitizer 哪些字节被毒化了，并作为子进程运行自己，去读写大块缓存里一个已释放的块（在中间，在链接处，在末尾），检查 AddressSanitizer 对每一处都有报告，并且使用一个活块或一个重新从缓存取出的块的子进程以状态 0 退出，它的错误输出被完整读取，没有任何种类的报告。毒化没有覆盖的东西：一个活的小块之后那个槽的 8 字节头保持可读，因为分配器会读写头，所以那里最多 8 字节的越界发现不了。越界进入另一个活块，或者缓存块的下一个指针（对 8 字节的块，是它的整个用户区），也发现不了。`asan` 能发现这些，它在每个块周围有红区，但它没有 arena，也没有前端缓存。两个都要运行。
- `host-test.c` 对着引擎的桩单独测试宿主：启动不了的运行时不留 VM，也不留内存块，两个创建并释放运行时的线程不会有竞态（`engine/test/host-test.sh tsan` 用 ThreadSanitizer 构建它）。
- 专用内存块：`heap-test` 有一个场景测它们（为一个块创建，在释放它的那次调用里归还，在内存块里和出内存块调整大小，被宿主拒绝，预算和限制下旧内存块保持完整），还有一个测重新映射（宿主拒绝就是内存耗尽且不复制，预算只检查增长的部分，每次增长 64 KiB 的缓冲区只得到很少的内存块）。它的随机运行会归还专用内存块并重新映射它们（奇数种子，那时假宿主会重新映射），做不到就失败。
- 假宿主用两种方式重新映射：`mremap`（linux），以及新建一个映射，复制，再取消旧内存块的映射，后者总是把内存块放在另一个地址上，并且在每个系统上都能用（在 linux 上一次运行里的各个堆轮流使用两种方式，macOS 只有第二种）。一个场景在专用内存块链表的头、中间和尾部增长块，每种方式各一次，并在宿主拒绝之后再来一次，然后以六种顺序各释放这三个，中间用 `heap_check` 检查，这样一个仍记着已搬走内存块地址的链表会走进一个未映射的内存块。
- 被中断标志停下的移动：`scenario_move_stop` 在不重新映射的堆里把 6 MiB 的专用块扩大到 12 MiB，再把一个缩小到 2 MiB，在每次移动对标志的每一次读取处停下它（假的 `host_poll` 在要求的那次读取时置上标志），要求它就在那次读取处失败、`stopped` 置着，堆、它的内存块和这个块都保持原样，然后在标志清掉时移动，每 1 MiB 读一次。`asan` 构建用的分配器 `heap-system.c` 也按同样的步子移动 1 MiB 以上、要变大的块，因为系统的 `realloc` 停不下来，缩小则交给这个 `realloc`。
- 随机运行按内存块的规则，用占用预算来判断一次拒绝，而不是按 TLSF 的池。一个请求最少增加的是它得到的内存块（它自己的，或者它缩小成的那个共用的），重新映射的内存块只需要它的增长部分，留在内存块里或变小的块什么也不需要。运行时上的运行用同样的规则、按它所在专用内存块的大小，判断大块的增长。一个场景把这些规则精确到字节，针对新内存块、重新映射、复制和从共用内存块里搬出：比最少增长量少一个单位，请求就被拒绝，宿主没被问，什么也没变，恰好等于它，请求就通过。另一个请求最大的请求 `JS_HEAP_MAX_REQUEST`（近 4 GiB，稀疏映射，只写了前 64 个字节和最后 64 个字节），什么也不拒绝的宿主必须整块给出，再多一个字节，堆就拒绝，宿主没被问。`api-test` 在真实的原生宿主上做同样的事：它映射的内存块，在一次 malloc、一次调整大小和一次释放之后立刻计数。
- 运行时必须以 `JS_FreeRuntime` 返回 0 结束。它返回堆里仍在用的块的数目，一个 arena 不管有多少槽在用都算一个块，所以这不是泄漏对象的个数。否则测试宿主以状态 87 退出。
- `engine/test/render.c` 在 C 树上渲染 tifa 的页面，带 `soak=N` 时，每 100 次渲染打印一次计数、块数和占用。
