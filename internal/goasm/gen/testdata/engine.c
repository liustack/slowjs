// Unified-entry engine functions. Params[0] is the shadow-stack pointer.
typedef struct { long tag; long val; } JSValue;
long mul(long a, long b); // register-entry leaf, called directly

// Deep self-recursion: n held live across a unified self-call (sp threaded).
long rec(void *sp, long n) {
  if (n <= 0) return 0;
  return rec(sp, n - 1) + n;
}
// Two direct calls to the register-entry leaf.
long poly(void *sp, long x) { (void)sp; return mul(x, x) + mul(x, 3); }
// 16-byte JSValue by value: modify a copy, return a new value.
JSValue bump(void *sp, JSValue v, long d) {
  (void)sp;
  JSValue r;
  r.tag = v.tag;
  r.val = v.val + d;
  return r;
}
// Integer and floating-point arguments mixed, double result.
double mix(void *sp, long a, double b, long c) { (void)sp; return (double)(a + c) + b; }
// Indirect call through a unified function pointer.
typedef long (*ufn)(void *, long);
long apply(void *sp, ufn f, long x) { return f(sp, x) + 1; }

// Call-overhead benchmark. Each b_* is a unified entry that loops n times
// calling one callee. The register loop calls a register-entry leaf (bare
// call); the unified loop calls a unified entry (stack args, callee-saved
// save/restore, split-check prologue). lf_* are leaves, uf_* unified.
long lf_i(long a, long b);
double lf_d(double a, double b);
typedef struct { long tag, val; } JSVal;
JSVal lf_v(long a, long b);
__attribute__((noinline)) long uf_i(void *sp, long a, long b) { (void)sp; return a + b; }
__attribute__((noinline)) double uf_d(void *sp, double a, double b) { (void)sp; return a + b; }
__attribute__((noinline)) JSVal uf_v(void *sp, long a, long b) { (void)sp; JSVal r; r.tag = a; r.val = b; return r; }

long b_reg_i(void *sp, long n) { long acc = 0; for (long i = 0; i < n; i++) acc += lf_i(i, 1); return acc; }
long b_uni_i(void *sp, long n) { long acc = 0; for (long i = 0; i < n; i++) acc += uf_i(sp, i, 1); return acc; }
double b_reg_d(void *sp, long n) { double acc = 0; for (long i = 0; i < n; i++) acc += lf_d((double)i, (double)i); return acc; }
double b_uni_d(void *sp, long n) { double acc = 0; for (long i = 0; i < n; i++) acc += uf_d(sp, (double)i, (double)i); return acc; }
long b_reg_v(void *sp, long n) { long acc = 0; for (long i = 0; i < n; i++) { JSVal r = lf_v(i, 1); acc += r.tag + r.val; } return acc; }
long b_uni_v(void *sp, long n) { long acc = 0; for (long i = 0; i < n; i++) { JSVal r = uf_v(sp, i, 1); acc += r.tag + r.val; } return acc; }

// Interrupt poll on back edges: a loop that otherwise calls nothing reaches a
// Go safepoint every 10000 iterations through host_poll, so GC can stop it and
// an interrupt can be latched. host_poll is a host callback implemented in Go.
extern long host_poll(void *sp);
long spin(void *sp, long n) {
  long acc = 0;
  long cd = 256;
  for (long i = 0; i < n; i++) {
    acc += i * i;
    if (--cd == 0) { cd = 256; if (host_poll(sp)) return acc; }
  }
  return acc;
}
