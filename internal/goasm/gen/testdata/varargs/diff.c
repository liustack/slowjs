// Variadic code run twice by TestVarargsNative: compiled plainly, with the
// target's own variable arguments, and through the pass's variadic block.
// Both must print the same. It compiles for x86_64-linux-gnu and
// aarch64-linux-gnu, freestanding: diff_main.c drives it.
typedef __builtin_va_list va_list;
#define va_start __builtin_va_start
#define va_arg __builtin_va_arg
#define va_end __builtin_va_end
#define va_copy __builtin_va_copy

// A host callback: the driver's version dirties a deep stack, as a Go
// callback may grow and move the goroutine stack.
long host_cb(long x);

// Interleaved types, as many as the caller passes: i int, l long, p
// pointer, d double.
__attribute__((noinline)) long mixed(const char *types, ...)
{
    va_list ap;
    va_start(ap, types);
    long s = 0;
    for (const char *t = types; *t; t++) {
        s *= 3;
        switch (*t) {
        case 'i': s += va_arg(ap, int); break;
        case 'l': s += va_arg(ap, long); break;
        case 'p': s += *va_arg(ap, const long *); break;
        case 'd': s += (long)(va_arg(ap, double) * 8); break;
        }
    }
    va_end(ap);
    return s;
}

__attribute__((noinline)) long count(int n, ...)
{
    va_list ap;
    va_start(ap, n);
    long s = 0;
    for (int i = 0; i < n; i++)
        s = s * 7 + va_arg(ap, long);
    va_end(ap);
    return s + n;
}

// Reads the list of int and double pairs twice through va_copy, the copy
// taken after the first pair.
__attribute__((noinline)) long twice(int n, ...)
{
    va_list ap, aq;
    va_start(ap, n);
    long a = va_arg(ap, int), b = 0;
    a += (long)va_arg(ap, double);
    va_copy(aq, ap);
    for (int i = 1; i < n; i++) {
        a = a * 5 + va_arg(ap, int);
        a = a * 5 + (long)(va_arg(ap, double) * 4);
    }
    for (int i = 1; i < n; i++) {
        b = b * 11 + va_arg(aq, int);
        b = b * 11 + (long)(va_arg(aq, double) * 8);
    }
    va_end(aq);
    va_end(ap);
    return a * 1000003 + b;
}

__attribute__((noinline)) long by_value(va_list ap, int n)
{
    long s = 0;
    for (int i = 0; i < n; i++)
        s = s * 13 + va_arg(ap, long);
    return s;
}

__attribute__((noinline)) long by_ref(va_list *ap) { return va_arg(*ap, int) * 100 + (long)va_arg(*ap, double); }

// Helpers read the list by reference (each consumes from it) and then by
// value.
__attribute__((noinline)) long helpers(int n, ...)
{
    va_list ap;
    va_start(ap, n);
    long a = by_ref(&ap);
    long b = by_ref(&ap);
    long c = by_value(ap, n);
    va_end(ap);
    return (a * 1009 + b) * 1013 + c;
}

// Reads after a host callback that dirties the stack.
__attribute__((noinline)) long across(int n, ...)
{
    va_list ap;
    va_start(ap, n);
    long s = va_arg(ap, long);
    for (int i = 1; i < n; i++) {
        s += host_cb(s);
        s = s * 3 + va_arg(ap, long) + (long)va_arg(ap, double);
    }
    va_end(ap);
    return s;
}

// A small formatter: %d int, %l long, %s string, %c char (promoted).
__attribute__((noinline)) long vfmt(char *out, const char *f, va_list ap)
{
    char *o = out;
    for (; *f; f++) {
        if (*f != '%') {
            *o++ = *f;
            continue;
        }
        f++;
        long v;
        switch (*f) {
        case 's':
            for (const char *s = va_arg(ap, const char *); *s; s++)
                *o++ = *s;
            continue;
        case 'c': *o++ = (char)va_arg(ap, int); continue;
        case 'd': v = va_arg(ap, int); break;
        default: v = va_arg(ap, long); break;
        }
        char buf[24];
        int k = 0;
        unsigned long u = v < 0 ? -(unsigned long)v : (unsigned long)v;
        do
            buf[k++] = (char)('0' + u % 10);
        while (u /= 10);
        if (v < 0)
            *o++ = '-';
        while (k)
            *o++ = buf[--k];
    }
    *o = 0;
    return o - out;
}

__attribute__((noinline)) long fmt(char *out, const char *f, ...)
{
    va_list ap;
    va_start(ap, f);
    long n = vfmt(out, f, ap);
    va_end(ap);
    return n;
}

// A variadic function that makes a variadic call, and a call through a
// pointer.
__attribute__((noinline)) long wrap(int n, ...)
{
    va_list ap;
    va_start(ap, n);
    long a = va_arg(ap, long);
    long b = va_arg(ap, long);
    va_end(ap);
    return count(3, a, b, (long)n) * 10 + mixed("ld", a, 0.5);
}

long (*const count_ptr)(int, ...) = count;
__attribute__((noinline)) long (*pick(int i))(int, ...) { return i ? count_ptr : 0; }

static long hash(const char *s)
{
    long h = 0;
    while (*s)
        h = h * 31 + *s++;
    return h;
}

// run fills r with the results and returns how many.
long run(long *r, int one)
{
    long k = 7, m = -3;
    char buf[128];
    int n = 0;
    r[n++] = mixed("");
    r[n++] = mixed("ildpldi", 1, 2L, 0.25, &k, -5L, 1.5, -7);
    r[n++] = mixed("dddddddddddd", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0, 11.0, 12.5);
    r[n++] = mixed("llllllllliiii", 1L, 2L, 3L, 4L, 5L, 6L, 7L, 8L, 9L, 10, 11, 12, 13);
    r[n++] = mixed("idpidpidpidpid", 1, 0.5, &k, 2, 1.5, &m, 3, 2.5, &k, 4, 3.5, &m, 5, 4.5);
    r[n++] = count(0);
    r[n++] = count(1, 9L);
    r[n++] = count(40, 1L, 2L, 3L, 4L, 5L, 6L, 7L, 8L, 9L, 10L, 11L, 12L, 13L, 14L, 15L, 16L, 17L, 18L, 19L, 20L,
                   21L, 22L, 23L, 24L, 25L, 26L, 27L, 28L, 29L, 30L, 31L, 32L, 33L, 34L, 35L, 36L, 37L, 38L, 39L, 40L);
    r[n++] = twice(3, 1, 0.25, 2, 0.5, 3, 0.75);
    r[n++] = twice(6, 1, 1.0, 2, 2.0, 3, 3.0, 4, 4.0, 5, 5.0, 6, 6.0);
    r[n++] = helpers(3, 4, 5.5, 6, 7.5, 8L, 9L, 10L);
    r[n++] = across(4, 1L, 2L, 0.5, 3L, 1.5, 4L, 2.5);
    r[n++] = fmt(buf, "a%db%lc%s%c=%d", -12, 1234567890123L, "xyz", 'Q', 0);
    r[n++] = hash(buf);
    r[n++] = wrap(5, 11L, 22L);
    r[n++] = pick(one)(3, 4L, 5L, 6L);
    return n;
}
