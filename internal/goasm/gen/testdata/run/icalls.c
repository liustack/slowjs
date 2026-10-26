// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

// The same function called directly and through a pointer, for each shape of
// signature an indirect call has (TestSampleRuns). run writes the pairs in
// order: the result of a direct call, then the result of the call of the
// same function through a table, whose entry one - 1 or one it is. The two
// of each pair must be equal, and the lines must be what native C gives. An
// indirect call that carries another call's signature moves its arguments
// the way that signature says, which these functions read differently.
typedef long (*f1)(long);
typedef long (*f2)(long, long);
typedef long (*f6)(long, long, long, long, long, long);
typedef long (*f7)(long, long, long, long, long, long, long);
typedef double (*fd)(long, double);
typedef struct {
    long a, b;
} V;
typedef V (*fv)(V, long);
typedef long (*fvp)(V, V, long);

__attribute__((noinline)) long inc(long x) { return x * 3 + 1; }
__attribute__((noinline)) long dbl(long x) { return x * 5 - 2; }
__attribute__((noinline)) long add2(long x, long y) { return x * 7 + y; }
__attribute__((noinline)) long sub2(long x, long y) { return x - y * 9; }
__attribute__((noinline)) long sum6(long a, long b, long c, long d, long e, long f)
{
    return a + 3 * b + 5 * c + 7 * d + 11 * e + 13 * f;
}
__attribute__((noinline)) long alt6(long a, long b, long c, long d, long e, long f)
{
    return a * 13 + 11 * b + 7 * c + 5 * d + 3 * e + f;
}
__attribute__((noinline)) long sum7(long a, long b, long c, long d, long e, long f, long g)
{
    return a + 3 * b + 5 * c + 7 * d + 11 * e + 13 * f + 17 * g;
}
__attribute__((noinline)) long alt7(long a, long b, long c, long d, long e, long f, long g)
{
    return a * 17 + 13 * b + 11 * c + 7 * d + 5 * e + 3 * f + g;
}
__attribute__((noinline)) double scale(long a, double x) { return (double)a + x * 4; }
__attribute__((noinline)) double shrink(long a, double x) { return (double)a * 2 - x; }
__attribute__((noinline)) V swapv(V v, long k)
{
    V r = {v.b + k, v.a * 2};
    return r;
}
__attribute__((noinline)) V rotv(V v, long k)
{
    V r = {v.a - k, v.b + v.a};
    return r;
}
__attribute__((noinline)) long dotv(V v, V w, long k) { return v.a * w.a + v.b * w.b + k; }
__attribute__((noinline)) long difv(V v, V w, long k) { return v.a * 3 - w.b * 5 + k; }

static f1 const t1[] = {inc, dbl};
static f2 const t2[] = {add2, sub2};
static f6 const t6[] = {sum6, alt6};
static f7 const t7[] = {sum7, alt7};
static fd const td[] = {scale, shrink};
static fv const tv[] = {swapv, rotv};
static fvp const tp[] = {dotv, difv};

long run(long *r, int one)
{
    int n = 0;
    for (long x = -3; x <= 4; x += 7) {
        long a = x * one, b = (x + 1) * one, c = (x + 2) * one, d = (x + 3) * one, e = (x + 4) * one, f = (x + 5) * one,
             g = (x + 6) * one;
        double h = 0.25 * x * one;
        V v = {a + 1, b - 2}, w = {c * 3, d + 4};
        r[n++] = inc(a);
        r[n++] = t1[one - 1](a);
        r[n++] = dbl(a);
        r[n++] = t1[one](a);
        r[n++] = add2(a, b);
        r[n++] = t2[one - 1](a, b);
        r[n++] = sub2(a, b);
        r[n++] = t2[one](a, b);
        r[n++] = sum6(a, b, c, d, e, f);
        r[n++] = t6[one - 1](a, b, c, d, e, f);
        r[n++] = alt6(a, b, c, d, e, f);
        r[n++] = t6[one](a, b, c, d, e, f);
        r[n++] = sum7(a, b, c, d, e, f, g);
        r[n++] = t7[one - 1](a, b, c, d, e, f, g);
        r[n++] = alt7(a, b, c, d, e, f, g);
        r[n++] = t7[one](a, b, c, d, e, f, g);
        r[n++] = (long)(scale(a, h) * 8);
        r[n++] = (long)(td[one - 1](a, h) * 8);
        r[n++] = (long)(shrink(a, h) * 8);
        r[n++] = (long)(td[one](a, h) * 8);
        V p = swapv(v, c), q = tv[one - 1](v, c), s = rotv(v, c), u = tv[one](v, c);
        r[n++] = p.a * 1000 + p.b;
        r[n++] = q.a * 1000 + q.b;
        r[n++] = s.a * 1000 + s.b;
        r[n++] = u.a * 1000 + u.b;
        r[n++] = dotv(v, w, d);
        r[n++] = tp[one - 1](v, w, d);
        r[n++] = difv(v, w, d);
        r[n++] = tp[one](v, w, d);
    }
    return n;
}
