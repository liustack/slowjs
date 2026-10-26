// Indirect calls of several signatures for the indirect-call symbols: two
// branches that call through pointers of different types and join, a call
// with a double, one with a 16-byte struct passed and returned, and one in
// a loop.
typedef long (*f1)(long);
typedef long (*f2)(long, long);
typedef double (*fd)(long, double);
typedef struct { long a, b; } V;
typedef V (*fv)(V, long);

long branches(int c, f1 a, f2 b, long x)
{
    long r;
    if (c)
        r = a(x + 1);
    else
        r = b(x, x + 2);
    return r * 3;
}

long mixed(f1 a, fd b, long x) { return a(x) + (long)b(x, 1.5); }

V pair(fv g, V v) { return g(v, 3); }

long loop(f1 *t, long n)
{
    long s = 0;
    for (long i = 0; i < n; i++)
        s += t[i & 3](i);
    return s;
}
