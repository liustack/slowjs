// A struct over 16 bytes returned by value (a hidden result pointer).
typedef struct { long a, b, c; } Big;
__attribute__((noinline)) Big g(void *sp, long x) { (void)sp; Big b = {x, x, x}; return b; }
long f(void *sp, long x) { Big b = g(sp, x); return b.a + b.c; }
