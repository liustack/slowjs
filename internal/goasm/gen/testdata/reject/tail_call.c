// A forced tail call: a jump to another function instead of a CALL.
__attribute__((noinline)) long g(void *sp, long n) { (void)sp; return n * 3; }
long f(void *sp, long n) { __attribute__((musttail)) return g(sp, n + 1); }
