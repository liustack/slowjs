// Variadic definitions and calls (direct and through a pointer) for the
// pass's -shadow-varargs. Like the engine's C it has no shadow pointer:
// -shadow-add-sp gives every function one.
typedef __builtin_va_list va_list;

__attribute__((noinline)) static long vsum(long n, va_list ap)
{
    long s = 0;
    for (long i = 0; i < n; i++)
        s += __builtin_va_arg(ap, long);
    return s;
}

__attribute__((noinline)) long sum(long n, ...)
{
    va_list ap;
    __builtin_va_start(ap, n);
    long s = vsum(n, ap);
    __builtin_va_end(ap);
    return s;
}

// ints and doubles in turn, read twice through va_copy.
__attribute__((noinline)) double mixed(int n, ...)
{
    va_list ap, aq;
    __builtin_va_start(ap, n);
    __builtin_va_copy(aq, ap);
    double s = 0;
    for (int i = 0; i < n; i++) {
        s += __builtin_va_arg(ap, int);
        s += __builtin_va_arg(ap, double);
    }
    for (int i = 0; i < n; i++) {
        s += __builtin_va_arg(aq, int);
        (void)__builtin_va_arg(aq, double);
    }
    __builtin_va_end(aq);
    __builtin_va_end(ap);
    return s;
}

long (*const sum_ptr)(long, ...) = sum;

// Hides the callee, so use calls sum through a pointer.
__attribute__((noinline)) long (*pick(int i))(long, ...) { return i ? sum_ptr : 0; }

long use(int one)
{
    return sum(3, 1L, 2L, 3L) * 1000 + (long)(mixed(2, 1, 2.5, 3, 4.25) * 10) +
           pick(one)(8, 1L, 2L, 3L, 4L, 5L, 6L, 7L, 8L) * 1000000;
}

// An 8-aligned 16-byte struct read through va_arg. SysV splits it into two
// i64 arguments at the call, which the 8-byte-slot block holds contiguously.
typedef struct { long a, b; } P8;

__attribute__((noinline)) long aggr(int n, ...)
{
    va_list ap;
    __builtin_va_start(ap, n);
    long t = 0;
    for (int i = 0; i < n; i++) {
        P8 p = __builtin_va_arg(ap, P8);
        t += 100 * p.a + p.b;
    }
    __builtin_va_end(ap);
    return t;
}

// A variadic function that itself makes a variadic call.
__attribute__((noinline)) long wrap(long k, ...)
{
    va_list ap;
    __builtin_va_start(ap, k);
    long first = __builtin_va_arg(ap, long);
    __builtin_va_end(ap);
    return first + sum(2, 100L, 200L);
}

// Non-variadic entry points main can call: aggr = 102 + 304, wrap = 7 + 300.
long run_aggr(void) { return aggr(2, (P8){1, 2}, (P8){3, 4}); }
long run_wrap(void) { return wrap(0, 7L); }
