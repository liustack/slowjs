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
  long cd = 10000;
  for (long i = 0; i < n; i++) {
    acc += i * i;
    if (--cd == 0) { cd = 10000; if (host_poll(sp)) return acc; }
  }
  return acc;
}

// ---- Shadow stack, reentry and interrupts (items 2, 4, 5, 6, 7) ----
// __shadow_remaining is resolved by the shadow-stack pass: the bytes left
// above everything the calling function has live. host_call is the Go
// callback bridge: it may run Go code that enters the engine again.
extern long __shadow_remaining(void);
extern long host_call(void *sp, long id, long arg);

#define INTERRUPTED (-2)
#define RANGE_ERROR (-3)
#define CORRUPT (-4)
#define MARGIN 2048

enum { CB_GROW = 1, CB_REENTER = 2, CB_SHRINK = 3, CB_INNER_EVAL = 4, CB_LOG = 5, CB_OBSERVE = 6 };

// The address escapes to Go, so the object must live in memory.
#define observe(sp, p) host_call(sp, CB_OBSERVE, (long)(p))

// Item 2: the callee takes the address of its parameters, changes the copies,
// writes the result through the caller's pointer, then lets Go move its stack.
__attribute__((noinline)) long take_addr(void *sp, JSValue v, double d, JSValue *out) {
  JSValue *pv = &v;
  double *pd = &d;
  observe(sp, pv);
  observe(sp, pd);
  pv->val += 1000;
  *pd *= 2;
  out->tag = pv->tag;
  out->val = pv->val;
  host_call(sp, CB_GROW, 0);
  return pv->val + (long)*pd;
}
long addr_param(void *sp, long tag, long val, double d) {
  JSValue orig = {tag, val};
  JSValue out = {0, 0};
  observe(sp, &orig);
  long r = take_addr(sp, orig, d, &out);
  if (orig.tag != tag || orig.val != val) return CORRUPT;
  if (out.tag != tag || out.val != val + 1000) return CORRUPT - 1;
  return r;
}

// Item 4: a double and two vectors live across a call, so LLVM spills them to
// the frame. The Go stack is only 8-byte aligned, so every spill must be an
// unaligned access.
typedef double v2d __attribute__((vector_size(16)));
// The calls go to Go (CB_OBSERVE returns its argument's low bit), so LLVM
// cannot see through them and must keep x, y and s in the frame.
__attribute__((noinline)) long nop_call(void *sp, long x) { (void)sp; return x; }
double vec_spill(void *sp, double a, double b) {
  v2d x = {a, b};
  v2d y = {b, a};
  double s = a * b;
  long k = host_call(sp, CB_OBSERVE, 1);
  x = x * y + (v2d){(double)k, (double)k};
  y = y - x;
  k = host_call(sp, CB_OBSERVE, 2) + 1;
  return x[0] + x[1] + y[0] * y[1] + s + (double)k + (double)nop_call(sp, 0);
}

// Item 5: each level fills a fixed local and a dynamic alloca with its own
// sentinel, reports their shadow addresses to Go, calls back (Go may enter
// the engine again from there), then checks the sentinels survived.
long reenter(void *sp, long level, long n) {
  long fixed[8];
  long *dyn = __builtin_alloca(n * sizeof(long));
  for (int i = 0; i < 8; i++) fixed[i] = level * 1000 + i;
  for (long i = 0; i < n; i++) dyn[i] = level * 100000 + i;
  observe(sp, fixed);
  host_call(sp, CB_LOG, (long)&fixed[0]);
  host_call(sp, CB_LOG, (long)&dyn[0]);
  host_call(sp, CB_LOG, (long)(&dyn[n - 1] + 1));
  long r = host_call(sp, CB_REENTER, level);
  for (int i = 0; i < 8; i++) if (fixed[i] != level * 1000 + i) return CORRUPT;
  for (long i = 0; i < n; i++) if (dyn[i] != level * 100000 + i) return CORRUPT;
  return r + level;
}

// Item 3 (shrink): live C state across a callback that grows the Go stack
// and lets GC shrink it again.
long shrink_frame(void *sp, long a, long b) {
  long keep[4] = {a, b, a + b, a * b};
  observe(sp, keep);
  long r = host_call(sp, CB_SHRINK, 0);
  return keep[0] + keep[1] + keep[2] + keep[3] + r;
}

// Item 6: a fragment that cannot fail runs to the end even when an interrupt
// is requested; the request is latched and only consumed at the end, where
// the function can report an error.
long fragment(void *sp, long *latch, long n, long *done) {
  long acc = 0, cd = 1000, i;
  for (i = 0; i < n; i++) {
    acc += i;
    if (--cd == 0) { cd = 1000; if (host_poll(sp)) *latch = 1; }
  }
  *done = i;
  if (*latch) return INTERRUPTED;
  return acc;
}
// The outer loop calls back into Go each round. The callback runs inner
// evaluations that get interrupted and ignores their errors; the outer loop
// must still stop with INTERRUPTED at its next poll.
long outer_loop(void *sp, long *latch, long rounds) {
  for (long r = 0; r < rounds; r++) {
    host_call(sp, CB_INNER_EVAL, r);
    if (host_poll(sp)) { *latch = 1; return INTERRUPTED; }
  }
  return 0;
}

// Item 7: recursion with an address-taken local per level; the check before
// each recursive call raises RangeError when the shadow stack runs out.
long deep(void *sp, long n) {
  long buf[32];
  if (__shadow_remaining() < MARGIN) return RANGE_ERROR;
  for (int i = 0; i < 32; i++) buf[i] = n + i;
  observe(sp, buf);
  if (n == 0) return buf[31];
  long r = deep(sp, n - 1);
  if (r < 0) return r;
  return r + buf[0] - n;
}
// Item 7: a dynamic alloca larger than what is left raises RangeError.
long dyn_alloc(void *sp, long bytes) {
  if (__shadow_remaining() < bytes + MARGIN) return RANGE_ERROR;
  char *p = __builtin_alloca(bytes);
  for (long i = 0; i < bytes; i += 4096) p[i] = (char)i;
  observe(sp, p);
  return bytes;
}

// spin with a chosen poll period, to compare GC stops at the same time
// between polls as the current engine's JS loop.
long spin_every(void *sp, long n, long every) {
  long acc = 0;
  long cd = every;
  for (long i = 0; i < n; i++) {
    acc += i * i;
    if (--cd == 0) { cd = every; if (host_poll(sp)) return acc; }
  }
  return acc;
}

// ---- Argument registers, live values and frame sizes (option B checks) ----
enum { CB_SLOTS = 7, CB_ECHO = 8 };

// 5, 6 and 7 integer argument slots counting the shadow pointer, called
// directly and through pointers, plain and mixed with JSValue and double.
// Over a mode's register limit the pass moves the arguments into a block on
// the caller's shadow frame.
typedef long (*fn5)(void *, long, long, long, long);
typedef long (*fn6)(void *, long, long, long, long, long);
typedef long (*fn7)(void *, long, long, long, long, long, long);
typedef long (*fn7v)(void *, JSValue, double, long, JSValue, long);
__attribute__((noinline)) long s5(void *sp, long a, long b, long c, long d) {
  (void)sp;
  return a + 3 * b + 5 * c + 7 * d;
}
__attribute__((noinline)) long s6(void *sp, long a, long b, long c, long d, long e) {
  (void)sp;
  return a + 3 * b + 5 * c + 7 * d + 11 * e;
}
// s7 calls back into Go, which may run slots_ind again on the same VM above
// this call's block.
__attribute__((noinline)) long s7(void *sp, long a, long b, long c, long d, long e, long f) {
  long r = host_call(sp, CB_SLOTS, a);
  return a + 3 * b + 5 * c + 7 * d + 11 * e + 13 * f + 17 * r;
}
__attribute__((noinline)) long s7v(void *sp, JSValue v, double x, long a, JSValue w, long b) {
  long r = host_call(sp, CB_ECHO, b);
  return v.tag + 3 * v.val + (long)(x * 4) + 5 * a + 7 * w.tag + 11 * w.val + 13 * b + 17 * r;
}
long slots_ind(void *sp, void **fns, long k) {
  long d5 = s5(sp, k, k + 1, k + 2, k + 3);
  long d6 = s6(sp, k, k + 1, k + 2, k + 3, k + 4);
  long d7 = s7(sp, k, k + 1, k + 2, k + 3, k + 4, k + 5);
  long d7v = s7v(sp, (JSValue){k, k + 1}, (double)k * 0.5, k + 2, (JSValue){k + 3, k + 4}, k + 5);
  long i5 = ((fn5)fns[0])(sp, k + 10, k + 11, k + 12, k + 13);
  long i6 = ((fn6)fns[1])(sp, k + 10, k + 11, k + 12, k + 13, k + 14);
  long i7 = ((fn7)fns[2])(sp, k + 10, k + 11, k + 12, k + 13, k + 14, k + 15);
  long i7v = ((fn7v)fns[3])(sp, (JSValue){k + 10, k + 11}, (double)k * 0.25, k + 12, (JSValue){k + 13, k + 14}, k + 15);
  return d5 + 2 * d6 + 4 * d7 + 8 * d7v + 16 * i5 + 32 * i6 + 64 * i7 + 128 * i7v;
}

// Integers, doubles and a vector live across unified calls, with a leaf
// call between them. i3 and d2 have their last use as a call argument.
long live_mix(void *sp, long a, double x) {
  long i1 = a * 3, i2 = a ^ 0x55, i3 = a + 7, i4 = a * a;
  double d1 = x * 1.5, d2 = x - 2.0, d3 = x * x;
  v2d v = {x, (double)a};
  long r1 = host_call(sp, CB_ECHO, i1);
  long m = mul(i2, 3);
  long r2 = uf_i(sp, i3, r1);
  double r3 = uf_d(sp, d1, d2);
  v = v * (v2d){d1, d3};
  long r4 = host_call(sp, CB_ECHO, m + i4);
  double r5 = uf_d(sp, v[0], v[1]);
  return i1 + i2 + i4 + r1 + m + r2 + r4 + (long)(r3 * 8) + (long)(r5 * 16) + (long)(d1 * 2) +
         (long)(d3 * 4) + (long)(v[0] * 32) + (long)(v[1] * 64);
}

// Frames of 40 and 560 words: every value is loaded before a callback that
// may change memory and used after it, so each one is spilled. The Go
// prologue checks these frames with R20 (over 0xf0 bytes) and R27 (4096 bytes
// and more) on arm64, and with R12 (over 128 and over 4096) on amd64.
#define D10(f, p) f(p##0) f(p##1) f(p##2) f(p##3) f(p##4) f(p##5) f(p##6) f(p##7) f(p##8) f(p##9)
#define D100(f, p) D10(f, p##0) D10(f, p##1) D10(f, p##2) D10(f, p##3) D10(f, p##4) \
  D10(f, p##5) D10(f, p##6) D10(f, p##7) D10(f, p##8) D10(f, p##9)
#define BIG_DECL(n) long v##n = *p++ ^ k;
#define BIG_SUM(n) acc = acc * 3 + (v##n ^ r);
__attribute__((noinline)) long big_f0(void *sp, long *src, long k) {
  long *p = src;
  D10(BIG_DECL, ) D10(BIG_DECL, 1) D10(BIG_DECL, 2) D10(BIG_DECL, 3)
  long r = host_call(sp, CB_GROW, k);
  long acc = r;
  D10(BIG_SUM, ) D10(BIG_SUM, 1) D10(BIG_SUM, 2) D10(BIG_SUM, 3)
  return acc;
}
__attribute__((noinline)) long big_4k(void *sp, long *src, long k) {
  long *p = src;
  D100(BIG_DECL, ) D100(BIG_DECL, 1) D100(BIG_DECL, 2) D100(BIG_DECL, 3) D100(BIG_DECL, 4)
  D10(BIG_DECL, 50) D10(BIG_DECL, 51) D10(BIG_DECL, 52) D10(BIG_DECL, 53) D10(BIG_DECL, 54) D10(BIG_DECL, 55)
  long r = host_call(sp, CB_GROW, k);
  long acc = r;
  D100(BIG_SUM, ) D100(BIG_SUM, 1) D100(BIG_SUM, 2) D100(BIG_SUM, 3) D100(BIG_SUM, 4)
  D10(BIG_SUM, 50) D10(BIG_SUM, 51) D10(BIG_SUM, 52) D10(BIG_SUM, 53) D10(BIG_SUM, 54) D10(BIG_SUM, 55)
  return acc;
}
// C to C calls into both frames, with the caller's own values live across.
long big_caller(void *sp, long *src, long k) {
  long a = k * 5, b = k ^ 0x3c;
  long x = big_f0(sp, src, k);
  long y = big_4k(sp, src, k + 1);
  return a + 3 * b + 5 * x + 7 * y;
}

// Call overhead from a function whose first phase keeps ten values live
// across a call, so its body uses many callee-saved registers, while its
// loop has only a few values live at each call: saving what the body uses
// then costs more per call than keeping what is live.
#define PHASE(f) \
  long a0 = n * 3, a1 = n ^ 5, a2 = n + 11, a3 = n * 7, a4 = n ^ 0x77, a5 = n - 13, a6 = n * 17, \
       a7 = n ^ 0x191, a8 = n + 23, a9 = n * 29; \
  long r = f(a0, a1); \
  long s = a0 + 2 * a1 + 3 * a2 + 4 * a3 + 5 * a4 + 6 * a5 + 7 * a6 + 8 * a7 + 9 * a8 + 10 * a9 + r; \
  long acc = 0; \
  for (long i = 0; i < n; i++) acc += f(i, s); \
  return acc;
long b_reg_p(void *sp, long n) {
#define LF(a, b) lf_i(a, b)
  PHASE(LF)
#undef LF
}
long b_uni_p(void *sp, long n) {
#define UF(a, b) uf_i(sp, a, b)
  PHASE(UF)
#undef UF
}

// Faults on purpose (fault_test.go): what a bug in the C would do. The Go
// runtime ends the process for an access to an address that is mapped to
// nothing unless the host asked for a panic with SetPanicOnFault, and for a
// trap instruction whatever the host does.
long peek(void *sp, long *p) { return *p; }
long poke(void *sp, long *p, long v) { *p = v; return v; }
// A local the host sees lives on the shadow stack: with the stack all but
// full, the stores go past its end.
long hit_end(void *sp) {
  long buf[16];
  for (long i = 0; i < 16; i++) buf[i] = i;
  observe(sp, buf);
  return buf[15];
}
// The sums keep the trap behind more than a few bytes of code: the proto's
// post-link check finds a body by its first 16 bytes, which a call in them
// would change by relocation.
long trap_now(void *sp, long n) {
  long a = n * 3 + 1, b = n ^ 5, c = n + 11;
  if (a + b + c != 0x7fffffff) __builtin_trap();
  return a;
}
