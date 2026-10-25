// Minimal engine for the arm64 frame check. Plain AAPCS inside C.
typedef unsigned long u64;
typedef long i64;
typedef struct { u64 u; i64 tag; } JSValue;
#define NOINLINE __attribute__((noinline))

// Go callback, reached through a generated stub.
i64 host_cb(u64 ssp, i64 depth, double x);

// Leaves: no stack, no calls. Register entry only.
NOINLINE i64 leaf_mix(i64 a, i64 b) { return (a * 31) ^ (b + 7); }
NOINLINE double leaf_f(double a, i64 b) { return a * 0.5 + (double)b; }

// Non-leaf called from C through a stub (unified entry).
NOINLINE JSValue mid(u64 ssp, JSValue v, double d) {
  JSValue r;
  i64 x = leaf_mix((i64)v.u, v.tag);
  double y = leaf_f(d, 3);
  r.u = (u64)x + (u64)y + ssp;
  r.tag = v.tag + 1;
  return r;
}

// Non-leaf with many values live across a Go callback, int and float.
JSValue outer(u64 ssp, JSValue v, double d, i64 depth) {
  i64 a0 = (i64)v.u + 1, a1 = v.tag * 3, a2 = depth ^ 0x55, a3 = a0 * a1;
  i64 a4 = a2 + a3, a5 = a4 * 7, a6 = a5 ^ a0, a7 = a6 + a2;
  i64 a8 = a7 * 5 + 1, a9 = a8 ^ a3, a10 = a9 + a4, a11 = a10 * 9;
  double f0 = d + 1.0, f1 = d * 2.0, f2 = f0 * f1, f3 = f2 - d;
  double f4 = f3 * 1.5, f5 = f4 + f0, f6 = f5 * 0.25, f7 = f6 + f2;
  double f8 = f7 * 3.0, f9 = f8 - f1;
  i64 h = host_cb(ssp + 64, depth, f9);
  JSValue m = mid(ssp + 128, v, f3);
  i64 h2 = host_cb(ssp + 192, depth, f5);
  i64 s = a0 + a1 + a2 + a3 + a4 + a5 + a6 + a7 + a8 + a9 + a10 + a11;
  double t = f0 + f1 + f2 + f3 + f4 + f5 + f6 + f7 + f8 + f9;
  JSValue r;
  r.u = ((u64)s ^ (u64)h ^ ((u64)h2 << 1)) + m.u + (u64)(i64)t;
  r.tag = m.tag + leaf_mix(s, h);
  return r;
}

// ---- Call-overhead benchmark (stub cost of R19-R25 and F8-F15) ----
// A register-entry leaf and a non-leaf unified callee doing the same trivial
// work. A C caller reaches the leaf with a direct BL (no stub) and the
// unified callee through the call_ stub that saves and restores R19-R25 and
// F8-F15. The volatile forces un_i and un_d to have a frame, so conv treats
// them as unified entries reached through the stub.
NOINLINE i64 bl_i(i64 a, i64 b) { return a + b; }
NOINLINE i64 un_i(u64 ssp, i64 a, i64 b) { volatile i64 t = (i64)ssp + a + b; return t; }
NOINLINE double bl_d(double a, double b) { return a + b; }
NOINLINE double un_d(u64 ssp, double a, double b) { volatile double t = a + b + (double)ssp; return t; }

i64 bench_reg_i(u64 ssp, i64 n) { (void)ssp; i64 acc = 0; for (i64 i = 0; i < n; i++) acc += bl_i(i, 1); return acc; }
i64 bench_uni_i(u64 ssp, i64 n) { i64 acc = 0; for (i64 i = 0; i < n; i++) acc += un_i(ssp, i, 1); return acc; }
i64 bench_reg_d(u64 ssp, i64 n) { (void)ssp; double acc = 0; for (i64 i = 0; i < n; i++) acc += bl_d((double)i, 1.0); return (i64)acc; }
i64 bench_uni_d(u64 ssp, i64 n) { double acc = 0; for (i64 i = 0; i < n; i++) acc += un_d(ssp, (double)i, 1.0); return (i64)acc; }
