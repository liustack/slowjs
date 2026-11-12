/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* What rvscore-engine.c, built freestanding, asks of the system: the number
   of renders the counts are of, RVSCORE_RENDERS in the environment. */
#include <stdlib.h>

int rvscore_renders(void)
{
    const char *s = getenv("RVSCORE_RENDERS");
    int n = s ? atoi(s) : 1;
    return n > 0 ? n : 1;
}
