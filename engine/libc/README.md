# engine/libc

The small libc of the C tree in `engine/`. The engine C is built with `-ffreestanding -nostdinc` and sees only this folder: no system header, no system library. What it asks of the outside is the callbacks of `engine/host.h` and the allocator.

## What is here

| Part | Where | Source |
| --- | --- | --- |
| Memory and string functions: `memcpy memmove memset memcmp bcmp bzero memchr strlen strcmp strchr strrchr` | `string/` | musl, as it is |
| 34 math functions: the 22 that need a real algorithm (`sin cos tan asin acos atan atan2 sinh cosh tanh asinh acosh atanh exp expm1 log log10 log1p log2 pow cbrt hypot`), the 12 that are exact (`fmod floor ceil trunc round rint lrint modf fmin fmax copysign scalbn`), and the helpers and tables they use | `math/`, `internal/libm.h` | musl. `libm.h` has a patch (below) |
| `snprintf`, `vsnprintf` | `stdio.c` | written here. Flags `- + space 0 #`, width and precision (a number or `*`), lengths `hh h l ll z j t` on the integer conversions only, conversions `d i u x X o c s p %`. No floating-point conversions. Anything else calls `abort` before the argument is read: another conversion, a length on `c s p %`, a format that ends inside a conversion, a width or precision that does not fit an int, an output over INT_MAX |
| `abort` | `abort.c` | written here. It calls `host_call` with the magic `HOST_CALL_ABORT`. The Go host panics and marks the VM close-only, as it does today. A trap would be a signal, and a signal kills the Go process |
| `sqrt`, `fabs` | `math.c` | written here, one instruction each. musl's `sqrt` is in software |
| Headers: `stddef.h stdint.h inttypes.h limits.h float.h stdarg.h stdatomic.h stdlib.h stdio.h string.h strings.h math.h assert.h endian.h fenv.h` | the root | written here, on the compiler's own macros and builtins. `abort()` asks the host (below), `alloca` is the builtin, a failed `assert` is an `abort()` |

The malloc family (`malloc realloc free malloc_usable_size`) is declared in `stdlib.h` but not defined here. The engine will bring its own allocator. Until then the native test host links the system's.

There is no `errno`, no `pthread`, no time and no standard stream. The wall clock is the `host_now` callback.

## musl

The files in `string/` and `math/`, `COPYRIGHT` and `internal/libm.h` come from musl commit `7ada6dde6f9dc6a2836c3d92c2f762d35fd229e0` (2024-03-02), the one `modernc.org/libc` v1.77.1 translates. So the `Math.*` results of linux/amd64 stay what they are in the Go engine. `COPYRIGHT` is musl's license file, MIT.

`MUSL.sha256` lists the SHA-256 of each file in musl and here. `vendor.sh --verify` checks the files here against it. `vendor.sh MUSL_DIR` copies them out of a musl checkout at that commit, applies `patches/` and writes the list again.

`internal/libm.h` is musl's with the changes in `patches/libm.h.patch`:

- the long double layouts are gone, as the C tree has no long double
- `fp_barrier` returns its input and `FORCE_EVAL` does nothing, as the C tree does not look at the floating-point flags
- it fails to compile when `__FP_FAST_FMA` or `__FP_FAST_FMAF` is defined

musl's `log`, `log2` and `pow` have a second path for a hardware fma, and the two paths differ in the last bit. The build takes the path without fma on every target, with `-ffp-contract=off -U__FP_FAST_FMA -U__FP_FAST_FMAF`. Then arm64 and amd64 give the same bits.

`internal/features.h` is what musl's `src/include/features.h` gives its own files. The build puts it in front of the musl files and of the written ones.

## Flags

The engine files and these are built with the flags of `engine/test/flags.sh`, which `engine/test/build.sh` and `engine/test/check-symbols.sh` both read:

```
-ffreestanding -nostdinc -fno-math-errno -ffp-contract=off -U__FP_FAST_FMA -U__FP_FAST_FMAF
-U__SIZEOF_INT128__ -fno-stack-protector -fwrapv -funsigned-char
```

On arm64 `-mno-outline-atomics` comes with them, so that an atomic stays an instruction and does not become a call of `__aarch64_*`, a function nobody defines.

`-ffreestanding` also means `-fno-builtin`, so a call to `pow(2, 10)` is a call, and the file that defines `memcpy` cannot be turned into a call to itself. `-fno-math-errno` makes `sqrt` an instruction. `-U__SIZEOF_INT128__` keeps the BigInt limbs 32 bits wide, as they are in the Go engine. Add `-I engine/libc` for the engine files, and `-I engine/libc/internal -include engine/libc/internal/features.h` for these.

## Checks

- `engine/test/check-symbols.sh` lists the symbols the built engine uses and does not define, and fails for any that `symbols.txt` does not allow. The compiler turns struct copies and compares into calls of `memcpy`, `memmove`, `memset` and `bcmp`, which is why they are always on the list.
- `engine/test/libc-test.sh` compares the string functions and `snprintf` with the system's, and checks that the formats `snprintf` does not do stop the program.
- `engine/test/api-test.sh` tests `abort` and the API the C tree changed, on real runtimes.
- `engine/test/math.sh` compares the `Math.*` results, and the libc math functions themselves, with the Go engine's.
- `engine/libc/vendor.sh --verify` checks the files taken from musl.

See [docs/testing.md](../../docs/testing.md#the-c-tree-and-its-native-test-host).
