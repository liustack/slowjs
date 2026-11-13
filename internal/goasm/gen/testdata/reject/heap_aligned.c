// 16 bytes read from memory the caller hands in, which the IR says is on 16
// bytes (the type of a vector asks for it): on amd64 llc may read it with an
// SSE instruction that faults off 16, and only locals and globals are on 16.
typedef long v2 __attribute__((vector_size(16)));
long f(void *sp, v2 *p) { (void)sp; v2 x = *p + *p; return x[0] ^ x[1]; }
