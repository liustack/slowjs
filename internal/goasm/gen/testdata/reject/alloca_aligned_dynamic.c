// A dynamic alloca aligned to 32 bytes: the shadow stack aligns to 16 only.
void use(void *p);
long f(void *sp, unsigned long n) { (void)sp; char *p = __builtin_alloca_with_align(n, 256); use(p); return p[3]; }
