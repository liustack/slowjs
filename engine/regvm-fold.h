/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Folding, in the pass of the register loop (included by regvm-pass.h): a
   function whose old stack is deeper than the temps a byte names.

   The part of the stack above the innermost catch offset (the top part) is
   cut into Arrays of R entries, R RV_FOLD_R or RV_FOLD_RMIN. Entry i of the
   part (from its base) is in the Array of level 1 number i / R, which is
   held whole in an Array of level 2 when that is complete and not the last
   one, and so on: lev[k] Arrays of level k + 1 exist, and rv_fold_canon
   gives how many of each there are for n entries. Level 1 has (n - 2) / R,
   so that the top two entries never fold (a jump that pushes one, as a
   gosub does, finds the layout of its target), and each level above it
   (l - 1) / R of the l of the one below. In the temps, from bslot: the
   Arrays of the highest level that are in no other, then those of the next
   level down, and so on, then the entries that are in no Array, in the
   order of the stack. An entry that is the first of an Array that is in no other is an
   RV_E_TEMP with the temp of that Array, every other entry of it is an
   RV_E_FOLD with that temp.

   A catch offset, and the iterator, next method and catch offset of a
   for_of_start, are temps of their own that never fold: the unwinding
   (rv_unwind) walks the temps for them, and the for-of instructions read
   them. When one is pushed, the top part under it is frozen: made one
   Array of all of its temps, at the bslot of the part, and the part above
   the catch offset starts. When it is popped, that Array is read back into
   the temps of the layout it was made from. So the layout is a function of
   the depth and of the chain of catch offsets, which rv_depths finds the
   same on every path to an instruction: every leader has the one its depth
   and chain give, and rv_block_end checks it. The target of a catch, which
   only an exception reaches, has its part still frozen with the exception
   in the temp of the catch offset, as the unwinding leaves it, and its code
   starts by reading the Array back.

   Before an old instruction, the Arrays of the top part are read back into
   temps down to the layout of the depth under what it reads (of the depth
   at the target for a jump that pushes past that, and none for a ret,
   whose GORET goes back to the end of its GOSUB with the layout the GOSUB
   had), and after it the layout of the depth it leaves is made again,
   with W_ARRAY_FROM for each Array made and GETELK for each element read
   back, and TMOV for the temps that move. The Arrays are the engine's own
   and dense, so reading and defining an element runs no javascript.

   The frame of a function that folds is the whole 127 slots, so the temps
   it needs are bounded: per catch offset its temps and one for the part
   frozen under it, and for the top part at most RV_FOLD_LMAX R + R + 1 +
   the most an instruction reads and pushes. RV_FOLD_PINS_MAX in regvm-func.h
   is what is left for the catch offsets, which the parser holds to.

   Same as QuickJS's interpreter: the values and the order of everything
   the code does. Folding allocates Arrays that QuickJS's interpreter does
   not: under a memory limit such a function can run out of memory at a
   fold where QuickJS's interpreter runs on. Only functions whose stack does not fit the temps fold. No new
   op. */

/* how deep the old instruction at pos reads below the top of the stack:
   its pops, or more for those that name entries under them. nip_catch
   takes the top only: what it drops under it is freed, folded or not
   (rv_except_drop frees an Array at its first entry). */
static int rv_fold_reach(RVPass *s, int pos, int npop)
{
    const uint8_t *p = s->bc + pos;
    int m, n;
    switch (p[0]) {
    case OP_for_of_next:
        return 3 + p[1];
    case OP_copy_data_properties:
        m = p[1];
        n = max_int(3, 1 + (m & 3));
        n = max_int(n, 1 + ((m >> 2) & 7));
        return max_int(n, 1 + ((m >> 5) & 7));
    case OP_iterator_next: case OP_iterator_call:
    case OP_iterator_get_value_done:
        return 4;
    case OP_for_await_of_next:
        return 3;
    case OP_nip_catch:
        return 1;
    default:
        return npop;
    }
}

/* the catch offset of the catch or for_of_start at c and the entries that
   go with it: from lo to hi */
static int rv_pin_hi(RVPass *s, int c)
{
    return rv_catch_level(s, c);
}

static int rv_pin_lo(RVPass *s, int c)
{
    return rv_catch_level(s, c) - (s->bc[c] == OP_catch ? 0 : 2);
}

/* where the top part starts with the chain c: depth and temp */
static int rv_chain_base(RVPass *s, int c)
{
    return c < 0 ? 0 : rv_pin_hi(s, c) + 1;
}

static int rv_chain_bslot(RVPass *s, int c)
{
    int t = 0, p;
    for (; c >= 0; c = p) {
        p = s->catchpos[c];
        t += (rv_pin_lo(s, c) > rv_chain_base(s, p)) + rv_pin_hi(s, c) - rv_pin_lo(s, c) + 1;
    }
    return t;
}

/* the chain after the old instruction at pos with the chain c before it
   and the depth d after it: what rv_depths does */
static int rv_chain_next(RVPass *s, int pos, int c, int d)
{
    switch (s->bc[pos]) {
    case OP_catch:
    case OP_for_of_start: case OP_for_await_of_start:
        return pos;
    case OP_drop:
        return c >= 0 && d == rv_catch_level(s, c) ? s->catchpos[c] : c;
    case OP_nip: case OP_nip1:
        return c >= 0 && d - 1 == rv_catch_level(s, c) ? s->catchpos[c] : c;
    case OP_iterator_close:
        return c >= 0 && d + 2 == rv_catch_level(s, c) ? s->catchpos[c] : c;
    case OP_nip_catch:
        return c >= 0 ? s->catchpos[c] : c;
    default:
        return c;
    }
}

/* the Arrays of each level for n entries */
static void rv_fold_canon(RVPass *s, int n, int *v)
{
    int k;
    v[0] = n >= 2 ? (n - 2) / s->R : 0;
    for (k = 1; k <= RV_FOLD_LMAX; k++)
        v[k] = v[k - 1] >= 1 ? (v[k - 1] - 1) / s->R : 0;
    if (v[RV_FOLD_LMAX])
        rv_overflow(s);
}

/* the temps of a part of n entries with the Arrays v */
static int rv_fold_temps(RVPass *s, const int *v, int n)
{
    int k, t = n - s->R * v[0];
    for (k = 0; k < RV_FOLD_LMAX && v[k]; k++)
        t += v[k] - s->R * v[k + 1];
    return t;
}

/* the entries of an Array of level k + 1 */
static int rv_fold_span(RVPass *s, int k)
{
    int i, n = s->R;
    for (i = 0; i < k; i++)
        n *= s->R;
    return n;
}

/* a temp of the frame for the folding, for an entry, or with end for the
   end of a window (the first temp past it): past tcap the function does
   not fit */
static int rv_fold_capw(RVPass *s, int t, BOOL end)
{
    if (t < 0 || t > s->tcap - !end) {
        s->fold_full = TRUE;
        s->failed = TRUE;
        return s->first_temp;
    }
    return s->first_temp + t;
}
#define rv_fold_cap(s, t) rv_fold_capw((s), (t), FALSE)

/* the temp of the first of the Arrays of level k + 1 that are in no other
   in the top part */
static int rv_fold_at(RVPass *s, int k)
{
    int j, t = s->bslot;
    for (j = RV_FOLD_LMAX - 1; j > k; j--)
        t += s->lev[j] - s->R * s->lev[j + 1];
    return t;
}

static BOOL rv_fold_folded(RVPass *s, int i)
{
    return i < s->base || i - s->base < s->R * s->lev[0];
}

static int rv_fold_tslot(RVPass *s, int depth)
{
    int c, lo, p;
    if (depth >= s->base) {
        if (depth - s->base < s->R * s->lev[0]) {
            rv_overflow(s);
            return s->first_temp;
        }
        return rv_fold_capw(s, rv_fold_at(s, -1) + depth - s->base - s->R * s->lev[0], TRUE);
    }
    /* a catch offset or what goes with it */
    for (c = s->chain; c >= 0; c = p) {
        p = s->catchpos[c];
        if (depth > rv_pin_hi(s, c))
            break;
        lo = rv_pin_lo(s, c);
        if (depth >= lo)
            return rv_fold_cap(s, rv_chain_bslot(s, p) + (lo > rv_chain_base(s, p)) + depth - lo);
    }
    rv_overflow(s);
    return s->first_temp;
}

/* the entries from base on, n of them with the Arrays v, laid out from
   the temp bslot */
static void rv_fold_layout(RVPass *s, int base, int bslot, const int *v, int n)
{
    int k, a, e, i, span, t = bslot;
    for (k = RV_FOLD_LMAX - 1; k >= 0; k--) {
        if (!v[k])
            continue;
        span = rv_fold_span(s, k);
        for (a = s->R * v[k + 1]; a < v[k]; a++) {
            e = base + a * span;
            for (i = 0; i < span; i++) {
                s->st[e + i].kind = i ? RV_E_FOLD : RV_E_TEMP;
                s->st[e + i].slot = rv_fold_cap(s, t);
                s->st[e + i].k = 0;
            }
            t++;
        }
    }
    for (i = s->R * v[0]; i < n; i++) {
        s->st[base + i].kind = RV_E_TEMP;
        s->st[base + i].slot = rv_fold_cap(s, t++);
        s->st[base + i].k = 0;
    }
}

/* the e-th temp of that layout: its first entry and how many it holds */
static void rv_fold_item(RVPass *s, const int *v, int n, int e, int *first, int *span)
{
    int k, f;
    for (k = RV_FOLD_LMAX - 1; k >= 0; k--) {
        f = v[k] - s->R * v[k + 1];
        if (!v[k])
            continue;
        if (e < f) {
            *span = rv_fold_span(s, k);
            *first = (s->R * v[k + 1] + e) * *span;
            return;
        }
        e -= f;
    }
    *span = 1;
    *first = s->R * v[0] + e;
    (void)n;
}

/* a fold or a read back can throw (no memory, or the throw-injection
   test): the temps owned, but those of first .. first + n - 1, and no old
   instruction does the same (RV_HOOK_APART, pos -1) */
static void rv_fold_throws(RVPass *s, int first, int n)
{
    RVMask m = { { 0, 0, 0, 0 } };
    int i;
#ifdef RV_INJECT
    int ipos = s->ipos;
    s->ipos = -1;
#endif
    for (i = 0; i < s->sp; i++) {
        if (s->st[i].kind == RV_E_TEMP &&
            (s->st[i].slot < first || s->st[i].slot >= first + n))
            rv_mask_set(&m, s->st[i].slot - s->first_temp);
    }
    rv_pcmap_put_p(s, s->code.size, rv_old_pc_at(s->bc, s->pos), &m);
    rv_hooks(s, RV_HOOK_APART);
#ifdef RV_INJECT
    s->ipos = ipos;
#endif
}

/* the temps of the entries from depth from up past the temp t move by d:
   up from the top down, down from the bottom up. An Array moves with the
   entries it holds. */
static void rv_fold_move(RVPass *s, int from, int t, int d)
{
    int i, j, to;
    for (j = 0; j < s->sp - from; j++) {
        i = d > 0 ? s->sp - 1 - j : from + j;
        if ((s->st[i].kind != RV_E_TEMP && s->st[i].kind != RV_E_FOLD) ||
            s->st[i].slot - s->first_temp <= t)
            continue;
        to = rv_fold_cap(s, s->st[i].slot - s->first_temp + d);
        if (s->st[i].kind == RV_E_TEMP) {
            rv_emit(s, RV_TMOV);
            rv_byte(s, to);
            rv_byte(s, s->st[i].slot);
        }
        s->st[i].slot = to;
    }
}

/* the m elements of the Array in the temp a read back into the temps a ..
   a + m - 1, the last first: the e-th the temp of the entries of
   first[e], span[e] of them (an Array of the level below, or an entry) */
static void rv_fold_read(RVPass *s, int a, int m, int base, const int *first, const int *span)
{
    int e, i;
    for (e = m - 1; e >= 0 && !s->failed; e--) {
        rv_emit(s, e ? RV_GETELK : RV_GETELKT);
        rv_byte(s, a + e);
        rv_byte(s, a);
        rv_byte(s, s->fold_k[e]);
        /* GETELKT takes the Array: when it throws, it has freed it */
        rv_fold_throws(s, a, e ? 0 : 1);
        for (i = 0; i < span[e]; i++) {
            s->st[base + first[e] + i].kind = i ? RV_E_FOLD : RV_E_TEMP;
            s->st[base + first[e] + i].slot = a + e;
        }
    }
}

/* the last Array of level k + 1 of the top part, in no other, read back
   into its R elements */
static void rv_fold_unfold_one(RVPass *s, int k)
{
    int R = s->R, a = s->lev[k] - 1, t, e, span, first[RV_FOLD_KMAX], spans[RV_FOLD_KMAX];
    t = rv_fold_at(s, k) + s->lev[k] - R * s->lev[k + 1] - 1;
    rv_fold_move(s, s->base, t, R - 1);
    span = k ? rv_fold_span(s, k - 1) : 1;
    for (e = 0; e < R; e++) {
        first[e] = (a * R + e) * span;
        spans[e] = span;
    }
    s->lev[k]--;
    rv_fold_read(s, rv_fold_cap(s, t), R, s->base, first, spans);
}

/* the last Array of level k + 1 of the top part read back into its R
   elements, after the one of level k + 2 that holds it, and so on up: a
   loop, not a call of itself, as the engine has no recursion the stack
   analysis cannot bound (engine/test/cycles.sh). Whether a level is held
   by the one above depends on the counts of these two levels only, which
   reading back the levels above it does not change, so all are known
   before the first is read back. */
static void rv_fold_unfold(RVPass *s, int k)
{
    int j;
    for (j = k;; j++) {
        if (j + 1 >= RV_FOLD_LMAX) {
            rv_overflow(s);
            return;
        }
        if (s->lev[j] - 1 >= s->R * s->lev[j + 1])
            break;
    }
    for (; j >= k && !s->failed; j--)
        rv_fold_unfold_one(s, j);
}

/* the first R of what is in no Array at level k (the entries for k = 0)
   made the next Array of level k + 1 of the top part */
static void rv_fold_fold(RVPass *s, int k)
{
    int R = s->R, t, a, i, span, from;
    if (k >= RV_FOLD_LMAX) {
        rv_overflow(s);
        return;
    }
    t = rv_fold_at(s, k - 1);
    a = rv_fold_cap(s, t);
    if (!k) {
        for (i = 0; i < R; i++)
            rv_materialize(s, s->base + R * s->lev[0] + i);
    }
    rv_fold_capw(s, t + R, TRUE);
    if (s->failed)
        return;
    rv_emit(s, RV_W_ARRAY_FROM);
    rv_byte(s, a + R);
    rv_u16(s, R);
    /* array_from frees its operands when it fails */
    rv_fold_throws(s, a, R);
    span = rv_fold_span(s, k);
    from = s->base + s->lev[k] * span;
    for (i = 0; i < span; i++) {
        s->st[from + i].kind = i ? RV_E_FOLD : RV_E_TEMP;
        s->st[from + i].slot = a;
    }
    s->lev[k]++;
    rv_fold_move(s, from + span, t, -(R - 1));
}

/* the top part made the layout of n entries */
static void rv_fold_canonical(RVPass *s, int n)
{
    int v[RV_FOLD_LMAX + 1], k;
    rv_fold_canon(s, n, v);
    for (k = RV_FOLD_LMAX - 1; k >= 0 && !s->failed; k--) {
        while (s->lev[k] > v[k] && !s->failed)
            rv_fold_unfold(s, k);
    }
    for (k = 0; k < RV_FOLD_LMAX && !s->failed; k++) {
        while (s->lev[k] < v[k] && !s->failed)
            rv_fold_fold(s, k);
    }
}

/* the frozen part of n entries from base, in the Array at the temp bslot,
   read back into its layout: the entries above it from the depth above
   move up */
static void rv_fold_thaw(RVPass *s, int base, int bslot, int n, int above)
{
    int v[RV_FOLD_LMAX + 1], m, e, first[RV_FOLD_KMAX], spans[RV_FOLD_KMAX];
    s->base = base;
    s->bslot = bslot;
    memset(s->lev, 0, sizeof(s->lev));
    if (!n)
        return;
    rv_fold_canon(s, n, v);
    m = rv_fold_temps(s, v, n);
    if (m > RV_FOLD_KMAX || s->st[base].kind != RV_E_TEMP ||
        s->st[base].slot != rv_fold_cap(s, bslot)) {
        rv_overflow(s);
        return;
    }
    rv_fold_move(s, above, bslot, m - 1);
    for (e = 0; e < m; e++)
        rv_fold_item(s, v, n, e, &first[e], &spans[e]);
    rv_fold_read(s, rv_fold_cap(s, bslot), m, base, first, spans);
    memcpy(s->lev, v, sizeof(s->lev));
}

/* the top part up to the depth end frozen into one Array at its bslot,
   for the catch offset of the instruction read now: the entries above
   move down */
static void rv_fold_freeze(RVPass *s, int end)
{
    int n = end - s->base, m, i, a;
    rv_fold_canonical(s, n);
    if (s->failed)
        return;
    if (n) {
        for (i = s->base + s->R * s->lev[0]; i < end; i++)
            rv_materialize(s, i);
        m = rv_fold_temps(s, s->lev, n);
        a = rv_fold_cap(s, s->bslot);
        rv_fold_capw(s, s->bslot + m, TRUE);
        if (m > RV_FOLD_KMAX - 1 || s->failed) {
            rv_overflow(s);
            return;
        }
        rv_emit(s, RV_W_ARRAY_FROM);
        rv_byte(s, a + m);
        rv_u16(s, m);
        rv_fold_throws(s, a, m);
        for (i = s->base; i < end; i++) {
            s->st[i].kind = i > s->base ? RV_E_FOLD : RV_E_TEMP;
            s->st[i].slot = a;
        }
        rv_fold_move(s, end, s->bslot, -(m - 1));
        s->bslot++;
    }
    s->base = end;
    memset(s->lev, 0, sizeof(s->lev));
    s->trans = TRUE;
}

/* the constants the elements of an Array are read by: the first of the
   function, so that a byte names them */
static void rv_fold_init(RVPass *s)
{
    int e, k;
    s->chain = -1;
    s->base = 0;
    s->bslot = 0;
    s->trans = FALSE;
    s->fold_full = FALSE;
    s->fold_m = 0;
    memset(s->lev, 0, sizeof(s->lev));
    for (e = 0; e < RV_FOLD_KMAX; e++) {
        k = rv_k(s, JS_MKVAL(JS_TAG_INT, e));
        if (k >= RV_K_NEAR)
            rv_overflow(s);
        s->fold_k[e] = k;
    }
}

/* the stack model at the leader pos, reached by a fall through or not:
   the layout of its depth and chain */
static void rv_fold_leader(RVPass *s, int pos, BOOL fell)
{
    int d = s->sp, c, p, t, lo, i, nonempty;

    s->chain = s->catchpos[pos];
    s->trans = FALSE;
    /* the parts frozen under each catch offset, and the catch offsets */
    t = rv_chain_bslot(s, s->chain);
    for (c = s->chain; c >= 0 && !s->failed; c = p) {
        p = s->catchpos[c];
        lo = rv_pin_lo(s, c);
        nonempty = lo > rv_chain_base(s, p);
        t -= nonempty + rv_pin_hi(s, c) - lo + 1;
        for (i = rv_chain_base(s, p); i < lo; i++) {
            s->st[i].kind = i > rv_chain_base(s, p) ? RV_E_FOLD : RV_E_TEMP;
            s->st[i].slot = rv_fold_cap(s, t);
            s->st[i].k = 0;
        }
        for (i = lo; i <= rv_pin_hi(s, c); i++) {
            s->st[i].kind = RV_E_TEMP;
            s->st[i].slot = rv_fold_cap(s, t + nonempty + i - lo);
            s->st[i].k = 0;
        }
    }
    s->base = rv_chain_base(s, s->chain);
    s->bslot = rv_chain_bslot(s, s->chain);
    memset(s->lev, 0, sizeof(s->lev));
    if (d < s->base) {
        rv_overflow(s);
        return;
    }
    if (s->leader[pos] & RV_LEAD_CATCH) {
        /* the part under the catch offset is still frozen, the exception
           is in the temp of the catch offset (rv_unwind): only an
           exception comes here */
        if (fell || (s->leader[pos] & RV_LEAD_OTHER) || d < s->base + 1) {
            rv_overflow(s);
            return;
        }
        nonempty = d - 1 > s->base;
        if (nonempty) {
            for (i = s->base; i < d - 1; i++) {
                s->st[i].kind = i > s->base ? RV_E_FOLD : RV_E_TEMP;
                s->st[i].slot = rv_fold_cap(s, s->bslot);
                s->st[i].k = 0;
            }
        }
        s->st[d - 1].kind = RV_E_TEMP;
        s->st[d - 1].slot = rv_fold_cap(s, s->bslot + nonempty);
        s->st[d - 1].k = 0;
        rv_fold_thaw(s, s->base, s->bslot, d - 1 - s->base, d - 1);
        rv_fold_canonical(s, d - s->base);
    } else {
        rv_fold_canon(s, d - s->base, s->lev);
        rv_fold_layout(s, s->base, s->bslot, s->lev, d - s->base);
    }
    s->fold_m = d - s->base;
}

/* the end of a block with n entries: the layout of its depth and chain */
static void rv_fold_block_end(RVPass *s, int n)
{
    int v[RV_FOLD_LMAX + 1];
    if (s->trans) {
        /* the catch that just froze the part under it (regvm-ops-except.h) */
        if (n != s->base || s->bc[s->pos] != OP_catch)
            rv_overflow(s);
        return;
    }
    rv_fold_canon(s, s->fold_m, v);
    if (s->base != rv_chain_base(s, s->chain) || n < s->base ||
        n - s->base < s->fold_m - 1 ||
        memcmp(v, s->lev, sizeof(v)))
        rv_overflow(s);
}

/* before the old instruction at pos: a catch offset it pushes freezes
   the part under it, else the Arrays are read back down to the layout of
   the depth under what it reads, which is the depth a branch jumps with.
   A dup reads the top entry, which is never folded, and a dup and the
   branch after it (a && b) jump with the depth they have, so it reads
   nothing back, nor does dup_case_atom, whose cases (CASES,
   regvm-ops-cmp.h) jump with the value still on the stack. */
static void rv_fold_before(RVPass *s, int pos)
{
    int npop, npush, term, target, r, i, m, v[RV_FOLD_LMAX + 1];
    if (s->sp < 0 || s->failed)
        return;
    switch (s->bc[pos]) {
    case OP_catch:
        rv_fold_freeze(s, s->sp);
        return;
    case OP_for_of_start: case OP_for_await_of_start:
        rv_fold_freeze(s, s->sp - 1);
        return;
    case OP_dup: case OP_dup_case_atom:
        return;
    case OP_ret:
        /* GORET goes back to the end of the GOSUB with the layout of this
           depth, which the GOSUB had: it reads the top entry, which never
           folds */
        return;
    }
    if (rv_old_info(s->bc, pos, &npop, &npush, &term, &target) < 0)
        return;
    r = rv_fold_reach(s, pos, npop);
    m = max_int(0, s->sp - r - s->base);
    /* a jump goes with the layout of the depth at its target, which a
       gosub (the return address) and a with_make_ref or with_get_ref (the
       object and the result) push one past what they read: that layout
       folds nothing they read, as the top two entries never fold */
    if (target >= 0 && target < s->len && s->depth[target] - s->base > m) {
        m = s->depth[target] - s->base;
        rv_fold_canon(s, m, v);
        if (s->R * v[0] > max_int(0, s->sp - r - s->base))
            rv_overflow(s);
    }
    /* the layout after the last instruction has as many Arrays of each
       level as that of a depth under it or more: only reading back, but
       for a jump to a depth past what it reads */
    rv_fold_canonical(s, m);
    s->fold_m = m;
    /* under the top part it reads catch offsets only: a frozen part has
       no temp (rv_fold_tslot) */
    for (i = max_int(0, s->sp - r); i < s->base; i++)
        rv_fold_tslot(s, i);
}

/* after the old instruction at pos: a catch offset pushed starts the top
   part above it, one popped reads back the part frozen under it, and the
   top part gets the layout of its depth */
static void rv_fold_after(RVPass *s, int pos)
{
    int c = rv_chain_next(s, pos, s->chain, s->sp), old = s->chain, i, lo;
    /* what the instruction pushed is in temps under tcap: a push may name
       the temp at tcap, the end of the temps (rv_fold_tslot), and then the
       function does not fit */
    for (i = max_int(0, s->sp - 8); i < s->sp; i++) {
        if (s->st[i].kind == RV_E_TEMP)
            rv_fold_cap(s, s->st[i].slot - s->first_temp);
    }
    if (s->failed)
        return;
    if (c != old) {
        if (c == pos && s->trans) {
            /* the catch offset and what goes with it, past the frozen part */
            s->chain = c;
            s->trans = FALSE;
            lo = rv_pin_lo(s, c);
            if (lo != s->base || s->sp != rv_pin_hi(s, c) + 1) {
                rv_overflow(s);
                return;
            }
            for (i = lo; i < s->sp; i++) {
                if (s->st[i].kind != RV_E_TEMP ||
                    s->st[i].slot != rv_fold_cap(s, s->bslot + i - lo)) {
                    rv_overflow(s);
                }
            }
            if (rv_chain_bslot(s, c) != s->bslot + s->sp - lo)
                rv_overflow(s);
            s->base = s->sp;
            s->bslot = rv_chain_bslot(s, c);
            memset(s->lev, 0, sizeof(s->lev));
        } else if (old >= 0 && c == s->catchpos[old] && !s->trans) {
            int p = c;
            lo = rv_pin_lo(s, old);
            s->chain = c;
            rv_fold_thaw(s, rv_chain_base(s, p), rv_chain_bslot(s, p),
                         lo - rv_chain_base(s, p), lo);
        } else {
            rv_overflow(s);
            return;
        }
    } else if (s->trans) {
        rv_overflow(s);
        return;
    }
    if (s->sp < s->base) {
        rv_overflow(s);
        return;
    }
    rv_fold_canonical(s, s->sp - s->base);
    s->fold_m = s->sp - s->base;
}
