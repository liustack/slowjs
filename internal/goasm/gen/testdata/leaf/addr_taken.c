// A register-entry leaf (myleaf) whose address escapes to a volatile pointer
// and is then called through it. The pass must reject it: an indirect call
// hands every target a shadow pointer, and a leaf has no unified entry that
// takes one.
__attribute__((noinline)) long myleaf(long a, long b) { return a + b; }

long (*volatile leafptr)(long, long);

long use(long x)
{
    leafptr = myleaf;
    return leafptr(x, 1);
}
