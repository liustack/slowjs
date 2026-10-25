// Register-entry leaves: compiled frameless (-fomit-frame-pointer).
__attribute__((noinline)) long mul(long a, long b) { return a * b + 7; }

// Register-entry leaves used by the call-overhead benchmark.
__attribute__((noinline)) long lf_i(long a, long b) { return a + b; }
__attribute__((noinline)) double lf_d(double a, double b) { return a + b; }
typedef struct { long tag, val; } JSVal;
__attribute__((noinline)) JSVal lf_v(long a, long b) { JSVal r; r.tag = a; r.val = b; return r; }
