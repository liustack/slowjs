// A struct over 16 bytes passed by value (an implicit stack copy).
typedef struct { long a, b, c; } Big;
__attribute__((noinline)) long g(void *sp, Big b) { (void)sp; return b.a + b.b + b.c; }
long f(void *sp, long x) { Big b = {x, x, x}; return g(sp, b); }
