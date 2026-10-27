/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* white-regexp.c is the regexp engine, libregexp.c itself, with functions
   added at its end that give its static functions to unit-white.c, as white.c
   does for quickjs.c. It is built as the engine is and linked in place of
   libregexp.o: build.sh does that. */
#include "../libregexp.c"

/* lre_split on the bytecode of a split op that takes itself, pushes a char
   'z', over a string of 'a' (the bytecode is made by hand: lre_compile never
   writes a chain of splits that leads back to itself). lre_split steps over
   at most LRE_SPLIT_BUDGET split ops in one call, and then gives back the
   one it is at to be pushed. Returns 1 when it returned the split at offset 0
   and its char at offset 5, else 0. */
int white_lre_split_budget(void)
{
    uint8_t bc[8], text[1] = { 'a' };
    REExecContext s;
    const uint8_t *pc = bc, *pc1 = bc + 5;

    bc[0] = REOP_split_goto_first;
    put_u32(bc + 1, (uint32_t)-5); /* back to itself */
    bc[5] = REOP_char;
    put_u16(bc + 6, 'z');
    memset(&s, 0, sizeof(s));
    s.cbuf = text;
    s.cbuf_end = text + 1;
    s.cbuf_type = 0;
    lre_split(&s, &pc, &pc1, text);
    return pc == bc && pc1 == bc + 5;
}
