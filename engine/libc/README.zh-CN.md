# engine/libc

[English](README.md) · 简体中文

`engine/` 里 C 树的小 libc。引擎的 C 代码用 `-ffreestanding -nostdinc` 构建，只能看到这个目录：没有系统头文件，没有系统库。它向外面要的只有 `engine/host.h` 的回调和 `engine/heap.h` 的 `host_memory_grow`。

## 这里有什么

| 部分 | 位置 | 来源 |
| --- | --- | --- |
| 内存和字符串函数：`memmove memset memcmp bcmp bzero memchr memrchr strlen strcmp strchr strchrnul strrchr` | `string/` | musl，原样使用 |
| `memcpy` | `string/memcpy.c` | 这里写的，代替 musl 的。128 字节以内用同一宽度的重叠加载和存储，再往上是每次 128 字节的循环，目标按 16 对齐。只用 SSE2 和 NEON 的移动指令，没有 `REP MOVSB`，没有 AVX。由 `engine/test/libc-test.sh` 测试 |
| 34 个数学函数：22 个需要真正的算法（`sin cos tan asin acos atan atan2 sinh cosh tanh asinh acosh atanh exp expm1 log log10 log1p log2 pow cbrt hypot`），12 个是精确的（`fmod floor ceil trunc round rint lrint modf fmin fmax copysign scalbn`），以及它们用到的辅助函数和表 | `math/`、`internal/libm.h` | musl。`libm.h` 有一个补丁（见下） |
| `snprintf`、`vsnprintf` | `stdio.c` | 这里写的。标志 `- + space 0 #`，宽度和精度（数字或 `*`），长度 `hh h l ll z j t` 只用于整数转换，转换 `d i u x X o c s p %`。没有浮点转换。其他任何情况都会在读取参数之前调用 `abort`：别的转换，给 `c s p %` 加长度，在转换中间结束的格式，装不进 int 的宽度或精度，超过 INT_MAX 的输出 |
| `abort` | `abort.c` | 这里写的。它用魔数 `HOST_CALL_ABORT` 调用 `host_call`。Go 宿主 panic，并把 VM 标为只能关闭。陷阱指令会产生信号，而信号会杀死 Go 进程 |
| `sqrt`、`fabs` | `math.c` | 这里写的，各一条指令。musl 的 `sqrt` 是软件实现 |
| 头文件：`stddef.h stdint.h inttypes.h limits.h float.h stdarg.h stdatomic.h stdlib.h stdio.h string.h strings.h math.h assert.h endian.h fenv.h` | 根目录 | 这里写的，建立在编译器自己的宏和内建函数上。`abort()` 向宿主要（见下），`alloca` 是内建函数，失败的 `assert` 就是一次 `abort()` |

这里没有 malloc 系列，引擎的文件也不调用它：引擎有自己的分配器（`engine/heap.c` 和 `engine/tlsf.c`，见 [docs/allocator.md](../../docs/allocator.zh-CN.md)），放在 `js_malloc` 后面。

这里没有 `errno`，没有 `pthread`，没有时间，也没有标准流。墙上时钟是 `host_now` 回调。

## musl

`string/` 和 `math/` 里的文件（除了 `string/memcpy.c`）、`COPYRIGHT` 和 `internal/libm.h`，来自 musl 的提交 `7ada6dde6f9dc6a2836c3d92c2f762d35fd229e0`（2024-03-02）。所以 `Math.*` 的结果就是为那个 musl 记录下来的（见 `engine/test/math.sh`）。`COPYRIGHT` 是 musl 的许可证文件，MIT。

`MUSL.sha256` 列出 musl 里和这里每个文件的 SHA-256。`vendor.sh --verify` 对着它检查这里的文件。`vendor.sh MUSL_DIR` 从该提交的 musl 检出里把它们复制出来，应用 `patches/`，并重新写出这份列表。

`internal/libm.h` 是 musl 的，带着 `patches/libm.h.patch` 里的改动：

- long double 的布局去掉了，因为 C 树里没有 long double
- `fp_barrier` 返回它的输入，`FORCE_EVAL` 什么也不做，因为 C 树不看浮点标志
- 定义了 `__FP_FAST_FMA` 或 `__FP_FAST_FMAF` 时编译失败

musl 的 `log`、`log2` 和 `pow` 为硬件 fma 准备了第二条路径，两条路径的结果在最后一位上不同。构建在每个目标上都走没有 fma 的路径，带 `-ffp-contract=off -U__FP_FAST_FMA -U__FP_FAST_FMAF`。这样 arm64 和 amd64 给出相同的位。

`internal/features.h` 是 musl 的 `src/include/features.h` 提供给它自己的文件的东西。构建把它放在 musl 的文件和这里写的文件前面。

## 编译参数

引擎的文件和这里的文件，用 `engine/test/flags.sh` 里的参数构建，`engine/test/build.sh`、`engine/test/check-symbols.sh` 和生成器（`internal/goasm/gen`）都读它。列表在那里，每个参数的原因也在。仓库里没有别的地方重复它。

在 arm64 上还要加 `-mno-outline-atomics`，这样原子操作仍是一条指令，不会变成对 `__aarch64_*` 的调用，那是没人定义的函数。

`-ffreestanding` 同时意味着 `-fno-builtin`，所以 `pow(2, 10)` 是一次真正的调用，而定义 `memcpy` 的文件也不会被变成对它自己的调用。`-fno-math-errno` 让 `sqrt` 成为一条指令。`-ffp-contract=off` 和 `-U__FP_FAST_FMA` 让融合乘加不进入数学函数，这样每个目标给出相同的结果。`-U__SIZEOF_INT128__` 让 BigInt 的分段保持 32 位宽，和之前的每个版本一样。引擎的文件要加 `-I engine/libc`，这里的文件要加 `-I engine/libc/internal -include engine/libc/internal/features.h`。

## 检查

- `engine/test/check-symbols.sh` 列出构建出的引擎用到而没有定义的符号，对 `symbols.txt` 不允许的任何符号都会失败。编译器会把结构体的复制和比较变成对 `memcpy`、`memmove`、`memset` 和 `bcmp` 的调用，所以它们总在列表上。
- `engine/test/libc-test.sh` 把字符串函数和 `snprintf` 同系统的比较，并检查 `snprintf` 不支持的格式会让程序停下。
- `engine/test/api-test.sh` 在真实的运行时上测试 `abort` 和 C 树改过的 API。
- `engine/test/math.sh` 对着记录下来的哈希检查 `Math.*` 的结果，以及 libc 的数学函数本身。
- `engine/libc/vendor.sh --verify` 检查取自 musl 的文件。

见 [docs/testing.md](../../docs/testing.zh-CN.md#c-树和它的原生测试宿主)。
