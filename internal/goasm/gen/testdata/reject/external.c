// A symbol outside the engine and the host callbacks.
extern long write_like(void *sp, long x);
long f(void *sp, long x) { return write_like(sp, x) + 1; }
