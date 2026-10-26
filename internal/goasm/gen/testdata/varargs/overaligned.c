// A 16-aligned aggregate read through va_arg. SysV splits it into two plain
// 8-aligned i64 arguments at the call, so the call site cannot tell it apart
// from two longs, yet the callee reads it with the overflow pointer rounded
// up to 16. The 8-byte-slot block cannot reproduce that: the pass must
// reject take, not miscompile it.
typedef struct __attribute__((aligned(16))) { long a, b; } S16;

__attribute__((noinline)) long take(int tag, ...)
{
    __builtin_va_list ap;
    __builtin_va_start(ap, tag);
    (void)__builtin_va_arg(ap, long);
    S16 s = __builtin_va_arg(ap, S16);
    __builtin_va_end(ap);
    return 10 * s.a + s.b;
}

long use16(void) { return take(0, 1L, (S16){2, 3}, 4L); }
