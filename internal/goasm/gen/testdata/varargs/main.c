// Native driver for varargs.c after the pass: a shadow region aligned to its
// size, as the Go host gives one, passed as the argument the pass added.
#include <stdio.h>
#include <stdlib.h>

long use(void *sp, int one);

int main(void)
{
    char *region = aligned_alloc(1 << 20, 1 << 20);
    printf("%ld\n", use(region + 8192, 1));
    return 0;
}
