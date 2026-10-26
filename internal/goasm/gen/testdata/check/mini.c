// The post-link check's own test program: each rewrite the lowering makes
// appears at least once. Calls with arguments and results (one returning a
// JSValue), values kept across calls in the frame and in callee-saved
// registers, floating-point constants, a jump table, engine_rodata holding
// pointers to strings and to functions and tables of relative offsets, an
// indirect call, the address of a function, a call to a register-entry leaf,
// a cold function (which llc would put in .text.unlikely.) and a function the
// generator cannot lower (a stub).

typedef struct {
	long tag, val;
} JSValue;

__attribute__((noinline)) long leafmix(long a, long b) { return a * 3 + b; }

__attribute__((noinline)) long add3(long a, long b, long c) { return a + b + c + 1234567; }

__attribute__((noinline)) long mul3(long a, long b, long c) { return a * b * c; }

__attribute__((noinline)) double scale(double x, long n) { return x * 1.5 + (double)n * 0.25; }

__attribute__((noinline)) JSValue pair(long a, long b) {
	JSValue v = {a, b};
	return v;
}

long usepair(long a) {
	JSValue v = pair(a, a + 1);
	return v.tag * 10 + v.val + leafmix(v.tag, a);
}

long spill(long a, long b, long c, long d, long e, long f) {
	long v1 = add3(a, b, c), v2 = mul3(b, c, d), v3 = add3(c, d, e), v4 = mul3(d, e, f);
	long v5 = add3(e, f, a), v6 = mul3(f, a, b), v7 = add3(v1, v2, v3);
	long v8 = mul3(v4, v5, v6);
	return v1 + v2 * 3 + v3 * 5 + v4 * 7 + v5 * 11 + v6 * 13 + v7 * 17 + v8 * 19 + a + b + c + d + e + f;
}

long pick(long k, long x) {
	switch (k) {
	case 0:
		return x + 11;
	case 1:
		return x * 13;
	case 2:
		return mul3(x, k, 5);
	case 3:
		return x ^ 19;
	case 4:
		return x << 3;
	case 5:
		return add3(x, k, 1);
	case 6:
		return x / 7;
	default:
		return -1;
	}
}

const char *name(long k) {
	switch (k) {
	case 0:
		return "zero";
	case 1:
		return "one";
	case 2:
		return "two";
	case 3:
		return "three";
	case 4:
		return "four";
	default:
		return "many";
	}
}

static const char *const words[] = {"alpha", "beta", "gamma"};

long wordlen(long i) {
	const char *s = words[i % 3];
	long n = 0;
	while (s[n])
		n++;
	return n;
}

// A table of structs keeps absolute pointers (llc makes relative tables of
// plain pointer arrays only).
static const struct item {
	const char *s;
	long n;
} items[] = {{"red", 1}, {"green", 2}, {"blue", 3}};

long itemsum(long i) {
	const struct item *it = &items[i % 3];
	return it->n + it->s[1];
}

typedef long (*op3)(long, long, long);

static const op3 ops[] = {add3, mul3};

long apply(long i, long a, long b) { return ops[i & 1](a, b, i); }

op3 getop(long i) { return i ? add3 : mul3; }

long fib(long n) { return n < 2 ? n : fib(n - 1) + fib(n - 2); }

__attribute__((cold, noinline)) long rare(long x) { return x * 5 - 3; }

long seldom(long x) { return x > 1000 ? rare(x) : x + 1; }

__attribute__((noinline)) long rejected(long x) {
	__asm__ volatile("" : "+r"(x));
	return x + 1;
}

long callsrejected(long x) { return rejected(x) * 2 + scale(1.0, x); }
