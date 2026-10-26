// One function per varargs rule of the pass, each breaking it in the
// smallest way, next to the uses the pass must keep. It compiles for both
// x86_64-linux-gnu and aarch64-linux-gnu.
typedef __builtin_va_list va_list;
typedef struct { long a, b; } P8;

// An aggregate read through va_arg: two 8-byte slots at once.
__attribute__((noinline)) long aggr(int n, ...)
{
    va_list ap;
    __builtin_va_start(ap, n);
    P8 p = __builtin_va_arg(ap, P8);
    __builtin_va_end(ap);
    return p.a + p.b;
}

// A read of a type C would have promoted (undefined in C, but it compiles).
__attribute__((noinline)) long narrow(int n, ...)
{
    va_list ap;
    __builtin_va_start(ap, n);
#pragma clang diagnostic ignored "-Wvarargs"
    char c = __builtin_va_arg(ap, char);
    __builtin_va_end(ap);
    return c;
}

// Writes the register-area offset back to a value that sends the next
// va_arg to the register save area, which va_start left empty.
__attribute__((noinline)) long resets(int n, ...)
{
    va_list ap;
    __builtin_va_start(ap, n);
#ifdef __aarch64__
    ((int *)&ap)[6] = -64; // __gr_offs
#else
    ((unsigned *)ap)[0] = 0; // gp_offset
#endif
    long v = __builtin_va_arg(ap, long);
    __builtin_va_end(ap);
    return v;
}

// Each helper adds a constant below 2^31 to the register-area offset.
// Twice in a row the i32 add wraps and sends the next va_arg to the
// register save area: on x86_64 48 + 2 * 2147483640 is 32, on aarch64
// 0 + 2147483647 + 1 is negative.
__attribute__((noinline)) void bump_big(va_list *ap)
{
#ifdef __aarch64__
    ((int *)ap)[6] += 2147483647; // __gr_offs
#else
    ((unsigned *)*ap)[0] += 2147483640u; // gp_offset
#endif
}

__attribute__((noinline)) void bump_small(va_list *ap)
{
#ifdef __aarch64__
    ((int *)ap)[6] += 1;
#else
    ((unsigned *)*ap)[0] += 2147483640u;
#endif
}

__attribute__((noinline)) long wraps(int n, ...)
{
    va_list ap;
    __builtin_va_start(ap, n);
    bump_big(&ap);
    bump_small(&ap);
    long v = __builtin_va_arg(ap, long);
    __builtin_va_end(ap);
    return v;
}

// Kept: a helper takes the va_list by value (aarch64 copies it with
// memcpy) and one by reference, and a va_copy.
__attribute__((noinline)) long by_value(va_list ap) { return __builtin_va_arg(ap, long); }
__attribute__((noinline)) long by_ref(va_list *ap) { return __builtin_va_arg(*ap, int); }

__attribute__((noinline)) long kept(int n, ...)
{
    va_list ap, aq;
    __builtin_va_start(ap, n);
    __builtin_va_copy(aq, ap);
    long s = by_value(ap) + by_ref(&ap) + __builtin_va_arg(aq, long) + (long)__builtin_va_arg(aq, double);
    __builtin_va_end(aq);
    __builtin_va_end(ap);
    return s;
}

long use(void) { return kept(0, 1L, 2, 3.5) + narrow(0, 1) + resets(0, 1L) + wraps(0, 11L, 22L, 33L, 44L, 55L, 66L); }
