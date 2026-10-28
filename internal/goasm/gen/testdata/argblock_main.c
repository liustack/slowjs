// Native driver for argblock.c after the pass: a shadow region aligned to
// its size, passed as the argument -shadow-add-sp added.
#include <stdio.h>
#include <stdlib.h>

long three(void *sp, long x);
long nested(void *sp, long x);
long callval(void *sp, long x);

int main(void)
{
    char *region = aligned_alloc(1 << 20, 1 << 20);
    void *sp = region + 8192;
    printf("%ld %ld %ld\n", three(sp, 5), nested(sp, 10), callval(sp, 7));
    return 0;
}
