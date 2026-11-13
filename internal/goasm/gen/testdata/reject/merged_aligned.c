// Two reads of 8 bytes from memory the caller hands in, the first of which the
// IR says is on 16 bytes: on amd64 llc merges them into one movapd, which
// faults when the pointer is 8 bytes off 16. The access is narrower than 16
// bytes, so the rule must go by the alignment, not by the width.
typedef double v2 __attribute__((vector_size(16)));
double f(void *sp, double *p) {
	(void)sp;
	p = __builtin_assume_aligned(p, 16);
	v2 x = {p[0], p[1]};
	x *= (v2){2, 3};
	return x[0] + x[1];
}
