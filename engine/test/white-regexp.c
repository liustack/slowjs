/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* white-regexp.c is the regexp engine, libregexp.c itself, with functions
   added at its end that give its static functions to unit-white.c, as white.c
   does for quickjs.c. It is built as the engine is and linked in place of
   libregexp.o: build.sh does that. */
/* the work of the loops of the compiler that read the interrupt flag is
   counted with that of quickjs.c (white.c) */
void white_lre_work(unsigned long n);
#define LRE_WORK(n) white_lre_work(n)
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
    lre_split(&s, bc, &pc, &pc1, text);
    return pc == bc && pc1 == bc + 5;
}

/* white_lre_chains compiles the pattern re with the flags of lre_compile and
   writes into out, at most n bytes, the chains lre_chains kept for it: for
   each, its number of split ops and its kind ('c' for char, 'f' for char_i),
   such as "15c 20f", or "-" when it kept none. Returns -1 when the compile
   fails, else the number of chains. opaque is the context of lre_compile. */
int white_lre_chains(const char *re, int flags, void *opaque, char *out, size_t n)
{
    char msg[64];
    int len, k, count, used = 0;
    uint8_t *bc = lre_compile(&len, msg, sizeof(msg), re, strlen(re), flags, opaque);
    const uint8_t *plan, *p;

    if (!bc)
        return -1;
    plan = bc + RE_HEADER_LEN + get_u32(bc + RE_HEADER_BYTECODE_LEN);
    count = plan[LRE_PLAN_CHAINS];
    p = plan + LRE_PLAN_LEN;
    out[0] = '\0';
    for (k = 0; k < count; k++) {
        used += snprintf(out + used, n - used, "%s%d%c", k ? " " : "", get_u16(p + 4),
                         p[6] == LRE_CHAIN_FOLD ? 'f' : 'c');
        p += lre_chain_size(p);
    }
    if (count == 0)
        snprintf(out, n, "-");
    lre_realloc(opaque, bc, 0);
    return count;
}

/* white_lre_split_same compiles re with flags and, at each split op of its
   code whose way the machine gives to lre_split, calls lre_split as the
   machine does on a string of one character of units (n of them), and on an
   empty one, once with the chains of the regexp and once with none, which is
   the walk over the split ops one at a time. Returns how many of the calls
   with chains gave another pc or pc1, or -1 when the compile fails, and adds
   the calls to *calls. opaque is the context of lre_compile. */
int white_lre_split_same(const char *re, int flags, void *opaque, const uint32_t *units, int n,
                         long *calls)
{
    char msg[64];
    int len, diff = 0, op, i, k, peek;
    uint8_t *bc = lre_compile(&len, msg, sizeof(msg), re, strlen(re), flags, opaque);
    const uint8_t *code, *pc[2], *pc1[2];
    uint32_t pos, bc_len, oplen, u;
    uint8_t text8[1];
    uint16_t text16[2];
    REExecContext s;

    if (!bc)
        return -1;
    memset(&s, 0, sizeof(s));
    bc_len = get_u32(bc + RE_HEADER_BYTECODE_LEN);
    code = bc + RE_HEADER_LEN;
    s.code = code;
    s.chains = code + bc_len + LRE_PLAN_LEN;
    s.is_unicode = (flags & (LRE_FLAG_UNICODE | LRE_FLAG_UNICODE_SETS)) != 0;
    for (pos = 0; pos < bc_len; pos += oplen) {
        op = code[pos];
        oplen = reopcode_info[op].size;
        if (op == REOP_range || op == REOP_range_i)
            oplen += get_u16(code + pos + 1) * 4;
        else if (op == REOP_range32 || op == REOP_range32_i)
            oplen += get_u16(code + pos + 1) * 8;
        else if (op == REOP_back_reference || op == REOP_back_reference_i ||
                 op == REOP_backward_back_reference || op == REOP_backward_back_reference_i)
            oplen += code[pos + 1];
        if (op != REOP_split_goto_first && op != REOP_split_next_first)
            continue;
        for (i = 0; i <= n; i++) {
            /* the character units[i], or none at i == n */
            u = i < n ? units[i] : 0;
            if (!s.is_unicode && u < 256) {
                text8[0] = u;
                s.cbuf = text8;
                s.cbuf_type = 0;
                s.cbuf_end = text8 + (i < n);
            } else {
                k = 0;
                if (u > 0xffff) {
                    text16[k++] = 0xd800 + ((u - 0x10000) >> 10);
                    text16[k++] = 0xdc00 + ((u - 0x10000) & 0x3ff);
                } else {
                    text16[k++] = u;
                }
                s.cbuf = (const uint8_t *)text16;
                s.cbuf_type = s.is_unicode ? 2 : 1;
                s.cbuf_end = (const uint8_t *)(text16 + (i < n ? k : 0));
            }
            peek = 0;
            for (k = 0; k < 2; k++) {
                pc[k] = code + pos + 5;
                if (op == REOP_split_next_first) {
                    pc1[k] = pc[k] + (int)get_u32(code + pos + 1);
                } else {
                    pc1[k] = pc[k];
                    pc[k] += (int)get_u32(code + pos + 1);
                }
                peek = (LRE_PEEK_OPS >> pc1[k][0]) & 1;
                if (!peek)
                    break;
                s.chain_count = k ? 0 : code[bc_len + LRE_PLAN_CHAINS];
                lre_split(&s, code + pos, &pc[k], &pc1[k], s.cbuf);
            }
            if (peek) {
                (*calls)++;
                if (pc[0] != pc[1] || pc1[0] != pc1[1])
                    diff++;
            }
        }
    }
    lre_realloc(opaque, bc, 0);
    return diff;
}
