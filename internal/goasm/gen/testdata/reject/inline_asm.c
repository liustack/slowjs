// Inline assembly reading the time stamp counter.
long f(void *sp) { (void)sp; unsigned lo, hi; __asm__ volatile("rdtsc" : "=a"(lo), "=d"(hi)); return ((long)hi << 32) | lo; }
