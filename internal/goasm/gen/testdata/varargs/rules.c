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

// Kept: a helper takes the va_list by reference, and a va_copy.
__attribute__((noinline)) long by_ref(va_list *ap) { return __builtin_va_arg(*ap, int); }

__attribute__((noinline)) long kept(int n, ...)
{
    va_list ap, aq;
    __builtin_va_start(ap, n);
    __builtin_va_copy(aq, ap);
    long s = by_ref(&ap) + __builtin_va_arg(aq, long) + (long)__builtin_va_arg(aq, double);
    __builtin_va_end(aq);
    __builtin_va_end(ap);
    return s;
}

long use(void) { return kept(0, 1L, 2, 3.5) + narrow(0, 1); }
