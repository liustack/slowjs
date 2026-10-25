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
