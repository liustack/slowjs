// Each function but plain hands a va_list, or a pointer read from one,
// somewhere the pass cannot follow, so the pass cannot prove the reads there
// keep to the block's 8-byte slots. It must reject each one, and leave plain
// (a va_copy read by a scalar helper) alone.
typedef struct __attribute__((aligned(16))) { long a, b; } S16;
typedef __builtin_va_list va_list;

// read_over is reached only through a va_copy.
__attribute__((noinline)) long read_over(va_list ap)
{
    S16 s = __builtin_va_arg(ap, S16);
    return s.a + s.b;
}

__attribute__((noinline)) long read_long(va_list ap) { return __builtin_va_arg(ap, long); }

__attribute__((noinline)) long copied(int tag, ...)
{
    va_list ap, aq;
    __builtin_va_start(ap, tag);
    __builtin_va_copy(aq, ap);
    long n = read_over(aq);
    __builtin_va_end(aq);
    __builtin_va_end(ap);
    return n;
}

long (*reader)(va_list);

__attribute__((noinline)) long through_pointer(int tag, ...)
{
    va_list ap;
    __builtin_va_start(ap, tag);
    long n = reader(ap);
    __builtin_va_end(ap);
    return n;
}

extern long ext_reader(va_list);

__attribute__((noinline)) long external(int tag, ...)
{
    va_list ap;
    __builtin_va_start(ap, tag);
    long n = ext_reader(ap);
    __builtin_va_end(ap);
    return n;
}

void *saved_list;

__attribute__((noinline)) long stores_list(int tag, ...)
{
    va_list ap;
    __builtin_va_start(ap, tag);
    saved_list = ap;
    long n = __builtin_va_arg(ap, long);
    __builtin_va_end(ap);
    return n;
}

void *saved_block;

__attribute__((noinline)) long stores_block(int tag, ...)
{
    va_list ap;
    __builtin_va_start(ap, tag);
    saved_block = ((void **)ap)[1]; // the overflow pointer (x86-64 layout)
    long n = __builtin_va_arg(ap, long);
    __builtin_va_end(ap);
    return n;
}

va_list global_list;

__attribute__((noinline)) long copies_out(int tag, ...)
{
    va_list ap;
    __builtin_va_start(ap, tag);
    __builtin_va_copy(global_list, ap);
    __builtin_va_end(ap);
    return 0;
}

__attribute__((noinline)) long plain(int tag, ...)
{
    va_list ap, aq;
    __builtin_va_start(ap, tag);
    __builtin_va_copy(aq, ap);
    long n = read_long(aq) + __builtin_va_arg(ap, long);
    __builtin_va_end(aq);
    __builtin_va_end(ap);
    return n;
}

long use(void)
{
    return copied(0, (S16){1, 2}) + through_pointer(0, 1L) + external(0, 2L) + stores_list(0, 3L) +
           stores_block(0, 4L) + copies_out(0, 5L) + plain(0, 6L);
}
