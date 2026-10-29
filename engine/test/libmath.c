/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/*
 * libmath INPUTS: calls each math function of engine/libc that the engine uses
 * on the inputs that mathcmp.js made (the "in" lines of mathdump's output), and
 * prints the bits of the results in the format of mathdump. math.sh checks them
 * against the hashes kept in engine/test/math.
 *
 * It is linked with the math objects of engine/libc and not with a libm, and
 * built with -fno-builtin, so every call is a call of engine/libc.
 * Results are written as they are, with no change of a NaN.
 */
#include <math.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define COUNT 100000

enum { PLAIN, LRINT, MODF };

typedef struct {
    const char *name;  /* the function of libc */
    const char *input; /* the name of the inputs in mathdump's output */
    int nargs;
    double (*f1)(double);
    double (*f2)(double, double);
    int kind;
} Fn;

static const Fn fns[] = {
    { "acos", "acos", 1, acos, 0, PLAIN },
    { "acosh", "acosh", 1, acosh, 0, PLAIN },
    { "asin", "asin", 1, asin, 0, PLAIN },
    { "asinh", "asinh", 1, asinh, 0, PLAIN },
    { "atan", "atan", 1, atan, 0, PLAIN },
    { "atan2", "atan2", 2, 0, atan2, PLAIN },
    { "atanh", "atanh", 1, atanh, 0, PLAIN },
    { "cbrt", "cbrt", 1, cbrt, 0, PLAIN },
    { "ceil", "ceil", 1, ceil, 0, PLAIN },
    { "cos", "cos", 1, cos, 0, PLAIN },
    { "cosh", "cosh", 1, cosh, 0, PLAIN },
    { "exp", "exp", 1, exp, 0, PLAIN },
    { "expm1", "expm1", 1, expm1, 0, PLAIN },
    { "fabs", "abs", 1, fabs, 0, PLAIN },
    { "floor", "floor", 1, floor, 0, PLAIN },
    { "fmax", "max", 2, 0, fmax, PLAIN },
    { "fmin", "min", 2, 0, fmin, PLAIN },
    { "fmod", "mod", 2, 0, fmod, PLAIN },
    { "hypot", "hypot", 2, 0, hypot, PLAIN },
    { "log", "log", 1, log, 0, PLAIN },
    { "log10", "log10", 1, log10, 0, PLAIN },
    { "log1p", "log1p", 1, log1p, 0, PLAIN },
    { "log2", "log2", 1, log2, 0, PLAIN },
    { "lrint", "clamp", 1, 0, 0, LRINT },
    { "modf", "pause", 1, 0, 0, MODF },
    { "pow", "pow", 2, 0, pow, PLAIN },
    { "round", "round", 1, round, 0, PLAIN },
    { "sin", "sin", 1, sin, 0, PLAIN },
    { "sinh", "sinh", 1, sinh, 0, PLAIN },
    { "sqrt", "sqrt", 1, sqrt, 0, PLAIN },
    { "tan", "tan", 1, tan, 0, PLAIN },
    { "tanh", "tanh", 1, tanh, 0, PLAIN },
    { "trunc", "trunc", 1, trunc, 0, PLAIN },
};

static uint64_t from_hex(const char *p)
{
    uint64_t v = 0;
    int i;

    for (i = 0; i < 16; i++) {
        int c = p[i];
        int d = c >= '0' && c <= '9' ? c - '0' : c >= 'a' && c <= 'f' ? c - 'a' + 10 : -1;
        if (d < 0) {
            fprintf(stderr, "libmath: bad hex digit in the input\n");
            exit(2);
        }
        v = v << 4 | d;
    }
    return v;
}

static double as_double(uint64_t u)
{
    double d;

    memcpy(&d, &u, sizeof d);
    return d;
}

static uint64_t as_bits(double d)
{
    uint64_t u;

    memcpy(&u, &d, sizeof u);
    return u;
}

/* The input lines of the file, by name. */
typedef struct {
    char *name;
    char *data;
} Input;

static Input inputs[64];
static int n_inputs;

static void read_inputs(const char *path)
{
    FILE *f = fopen(path, "r");
    char *line = NULL;
    size_t cap = 0;
    ssize_t len;

    if (!f) {
        perror(path);
        exit(2);
    }
    while ((len = getline(&line, &cap, f)) > 0) {
        char *sp1, *sp2, *sp3;

        if (strncmp(line, "in ", 3))
            continue;
        sp1 = line + 2;
        sp2 = strchr(sp1 + 1, ' ');
        sp3 = sp2 ? strchr(sp2 + 1, ' ') : NULL;
        if (!sp2 || !sp3 || n_inputs == 64) {
            fprintf(stderr, "libmath: bad input line\n");
            exit(2);
        }
        *sp2 = 0;
        if (line[len - 1] == '\n')
            line[len - 1] = 0;
        inputs[n_inputs].name = strdup(sp1 + 1);
        inputs[n_inputs].data = strdup(sp3 + 1);
        n_inputs++;
    }
    free(line);
    fclose(f);
}

static const char *find_input(const char *name)
{
    int i;

    for (i = 0; i < n_inputs; i++)
        if (!strcmp(inputs[i].name, name))
            return inputs[i].data;
    fprintf(stderr, "libmath: no inputs named %s\n", name);
    exit(2);
}

int main(int argc, char **argv)
{
    size_t i;

    if (argc != 2) {
        fprintf(stderr, "usage: libmath INPUTS\n");
        return 2;
    }
    read_inputs(argv[1]);
    for (i = 0; i < sizeof fns / sizeof *fns; i++) {
        const Fn *fn = &fns[i];
        const char *data = find_input(fn->input);
        int k;

        printf("in %s %d %s\nout %s %d ", fn->name, COUNT, data, fn->name, COUNT);
        for (k = 0; k < COUNT; k++) {
            double x = as_double(from_hex(data + (size_t)k * 16 * fn->nargs));
            double y = fn->nargs == 2 ? as_double(from_hex(data + (size_t)k * 32 + 16)) : 0;

            if (fn->kind == LRINT) {
                /* beyond the range of a long the conversion is not defined */
                long v = fabs(x) < 4.0e18 ? lrint(x) : 0;
                printf("%016llx", (unsigned long long)v);
            } else if (fn->kind == MODF) {
                double ip;
                double frac = modf(x, &ip);
                printf("%016llx%016llx", (unsigned long long)as_bits(frac), (unsigned long long)as_bits(ip));
            } else {
                double r = fn->nargs == 2 ? fn->f2(x, y) : fn->f1(x);
                printf("%016llx", (unsigned long long)as_bits(r));
            }
        }
        printf("\n");
    }
    return 0;
}
