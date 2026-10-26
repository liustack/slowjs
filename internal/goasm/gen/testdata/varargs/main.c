// Native driver for varargs.c after the pass: a shadow region aligned to its
// size, as the Go host gives one, passed as the argument the pass added.
#include <stdio.h>
#include <stdlib.h>

long use(void *sp, int one);
long run_aggr(void *sp);
long run_wrap(void *sp);

int main(void)
{
    char *region = aligned_alloc(1 << 20, 1 << 20);
    void *sp = region + 8192;
    printf("%ld %ld %ld\n", use(sp, 1), run_aggr(sp), run_wrap(sp));
    return 0;
}
