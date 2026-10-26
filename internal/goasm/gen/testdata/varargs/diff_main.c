// Freestanding driver for diff.c on linux: prints run's results, one per
// line. With -DPASS it calls the pass's output, which takes a shadow
// pointer first.
#ifdef PASS
#define SP region + 4096,
#define SPARG void *sp,
#else
#define SP
#define SPARG
#endif

static char region[1 << 20] __attribute__((aligned(1 << 20)));

long run(SPARG long *r, int one);

static long sys3(long nr, long a, long b, long c)
{
#ifdef __aarch64__
    register long x8 __asm__("x8") = nr, x0 __asm__("x0") = a, x1 __asm__("x1") = b, x2 __asm__("x2") = c;
    __asm__ volatile("svc #0" : "+r"(x0) : "r"(x8), "r"(x1), "r"(x2) : "memory");
    return x0;
#else
    long r;
    __asm__ volatile("syscall" : "=a"(r) : "a"(nr), "D"(a), "S"(b), "d"(c) : "rcx", "r11", "memory");
    return r;
#endif
}

#ifdef __aarch64__
#define SYS_WRITE 64
#define SYS_EXIT 93
#else
#define SYS_WRITE 1
#define SYS_EXIT 60
#endif

// The host callback dirties a deep stack before it returns.
__attribute__((noinline)) static long deep(int d, long x)
{
    volatile long junk[64];
    for (int i = 0; i < 64; i++)
        junk[i] = x ^ (0x5a5a5a5a5a5a5a5aL + i);
    return d ? deep(d - 1, x + junk[d & 63]) & 0xffff : junk[3] & 0xff;
}

long host_cb(SPARG long x) { return deep(200, x) % 97; }

void cmain(void)
{
    long r[64];
    char buf[32];
    int n = (int)run(SP r, 1);
    for (int i = 0; i < n; i++) {
        long v = r[i];
        unsigned long u = v < 0 ? -(unsigned long)v : (unsigned long)v;
        int k = sizeof buf;
        buf[--k] = '\n';
        do
            buf[--k] = (char)('0' + u % 10);
        while (u /= 10);
        if (v < 0)
            buf[--k] = '-';
        sys3(SYS_WRITE, 1, (long)(buf + k), sizeof buf - k);
    }
    sys3(SYS_EXIT, 0, 0, 0);
}

__attribute__((naked, noreturn)) void _start(void)
{
#ifdef __aarch64__
    __asm__("mov x29, #0\n\tmov x30, #0\n\tbl cmain\n\tbrk #0");
#else
    __asm__("xor %ebp, %ebp\n\tand $-16, %rsp\n\tcall cmain\n\tud2");
#endif
}
