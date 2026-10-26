// The over-aligned va_arg read moved into a plain, non-variadic helper that
// the variadic function hands its va_list to (the printf/vsnprintf shape).
// The variadic function itself has no over-aligned read, so following the
// va_list only into variadic definitions would miss it: the pass must follow
// the va_list into read_s and reject it there.
typedef struct __attribute__((aligned(16))) { long a, b; } S16;

__attribute__((noinline)) long read_s(__builtin_va_list ap)
{
    (void)__builtin_va_arg(ap, long);
    S16 s = __builtin_va_arg(ap, S16);
    return 10 * s.a + s.b;
}

__attribute__((noinline)) long take(int tag, ...)
{
    __builtin_va_list ap;
    __builtin_va_start(ap, tag);
    long n = read_s(ap);
    __builtin_va_end(ap);
    return n;
}

long use(void) { return take(0, 1L, (S16){2, 3}, 4L); }
