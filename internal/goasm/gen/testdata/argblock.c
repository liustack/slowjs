// Calls with more integer arguments than the registers allow: with
// -shadow-max-int-args=6 they share one argument block. three makes three
// such calls (one 64-byte block), nested feeds one packed call's result into
// another (the shared block must not be clobbered while the inner call runs).
__attribute__((noinline)) long many(long a, long b, long c, long d, long e, long f, long g)
{
    return a + b + c + d + e + f + g;
}

long three(long x)
{
    return many(x, 1, 2, 3, 4, 5, 6) + many(1, x, 2, 3, 4, 5, 6) + many(1, 2, x, 3, 4, 5, 6);
}

long nested(long x)
{
    return many(many(x, 0, 0, 0, 0, 0, 0), 1, 2, 3, 4, 5, 6);
}

// A value of two words that C passes in memory (byval) once the registers
// ran out, as quickjs.c passes a JSValue on amd64: the block holds its two
// words, and the pass reports it as the pair the Go host passes.
typedef struct {
    long u, tag;
} val;

__attribute__((noinline)) long withval(long a, long b, long c, long d, long e, long f, val v)
{
    return a + b + c + d + e + f + v.u * v.tag;
}

long callval(long x)
{
    val v = {x, 3};
    return withval(1, 2, 3, 4, 5, 6, v);
}
