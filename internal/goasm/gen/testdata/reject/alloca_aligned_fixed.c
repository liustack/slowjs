// A local the code takes the address of, aligned to 32 bytes.
void use(void *p);
long f(void *sp) { (void)sp; _Alignas(32) char buf[64]; use(buf); return buf[3]; }
