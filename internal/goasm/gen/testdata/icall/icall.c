// Indirect calls of several signatures for the indirect-call symbols: two
// branches that call through pointers of different types and join, a call
// with a double, one with a 16-byte struct passed and returned, one in a
// loop, and the same type called twice on two lines. TestICallSymbols holds
// each call to the signature written after it in a sig: comment. That is
// what C says the call's type is, and the one thing the test does not take
// from the pass.
typedef long (*f1)(long);
typedef long (*f2)(long, long);
typedef double (*fd)(long, double);
typedef struct { long a, b; } V;
typedef V (*fv)(V, long);

long branches(int c, f1 a, f2 b, long x)
{
    long r;
    if (c)
        r = a(x + 1); /* sig: i>i */
    else
        r = b(x, x + 2); /* sig: ii>i */
    return r * 3;
}

long mixed(f1 a, fd b, long x)
{
    long p = a(x); /* sig: i>i */
    double q = b(x, 1.5); /* sig: id>d */
    return p + (long)q;
}

V pair(fv g, V v)
{
    return g(v, 3); /* sig: iii>j */
}

long loop(f1 *t, long n)
{
    long s = 0;
    for (long i = 0; i < n; i++)
        s += t[i & 3](i); /* sig: i>i */
    return s;
}

long swap(f1 a, f2 b, long x)
{
    long p = b(x, 5); /* sig: ii>i */
    long q = a(p); /* sig: i>i */
    long r = b(q, p); /* sig: ii>i */
    return a(r) + q; /* sig: i>i */
}
