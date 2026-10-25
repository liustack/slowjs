// An initialized mutable global.
long table[2] = {1, 2};
long f(void *sp, long i) { (void)sp; table[i & 1]++; return table[0]; }
