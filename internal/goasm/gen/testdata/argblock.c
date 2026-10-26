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
