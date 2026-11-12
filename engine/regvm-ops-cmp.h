/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Compares and tests in the register loop: lt, lte, gt, gte, eq, neq,
   strict_eq and strict_neq, is_null, is_undefined, is_undefined_or_null and
   the typeof tests, lnot, each as a bool into a temp or fused with the
   branch after it, and the case chains of a switch on strings
   (dup_case_atom). Read once for each part, see regvm-areas.h. */

#if RV_PART == RV_PART_LIST
/* a bool into temp d: d a b, and the K form d a k. The branches: a b off
   and a k off, F jumps when the compare is false, T when it is true. A
   negated compare (neq, strict_neq) branches as its positive form with the
   sense turned. */
RV_OP(LT) RV_OP(LTK) RV_OP(LTE) RV_OP(LTEK) RV_OP(GT) RV_OP(GTK) RV_OP(GTE) RV_OP(GTEK)
RV_OP(EQ) RV_OP(EQK) RV_OP(NE) RV_OP(NEK) RV_OP(SEQ) RV_OP(SEQK) RV_OP(SNE) RV_OP(SNEK)
RV_OP(JLTF) RV_OP(JLTT) RV_OP(JLTKF) RV_OP(JLTKT)
RV_OP(JLTEF) RV_OP(JLTET) RV_OP(JLTEKF) RV_OP(JLTEKT)
RV_OP(JGTF) RV_OP(JGTT) RV_OP(JGTKF) RV_OP(JGTKT)
RV_OP(JGTEF) RV_OP(JGTET) RV_OP(JGTEKF) RV_OP(JGTEKT)
RV_OP(JEQF) RV_OP(JEQT) RV_OP(JEQKF) RV_OP(JEQKT)
RV_OP(JSEQF) RV_OP(JSEQT) RV_OP(JSEQKF) RV_OP(JSEQKT)
RV_OP(JSEQVF)              /* a idx16 off: JSEQF with b the value of the var ref
                               idx, read in place (a GETVR the pass took into it) */
RV_OP(ISX)                  /* d s kind: s is null (0), undefined (1), either (2) */
RV_OP(JISF) RV_OP(JIST)     /* s kind off */
RV_OP(TYPEIS)               /* d s atom: typeof s is the atom */
RV_OP(JTYPEF) RV_OP(JTYPET) /* s atom off */
RV_OP(LNOT)                 /* d s */
RV_OP(JFN) RV_OP(JTN)       /* t off: as JF and JT, the temp t kept (a dup then a branch) */
RV_OP(JFND) RV_OP(JTND)     /* t off: as JFN and JTN, and t freed when the jump is not
                               taken (a dup, a branch, then a drop: a && b, a || b) */
RV_OP(CASES)                /* s n, then n times atom flag off, then off: jump at the
                               first case whose string is s, if flag is 1, or is not,
                               if 0, and by the last off when none holds. With bit 7
                               of n set, a table of the cases comes before them
                               (rv_cases_table) */
#elif RV_PART == RV_PART_INFO
    case OP_lt: case OP_lte: case OP_gt: case OP_gte:
    case OP_eq: case OP_neq: case OP_strict_eq: case OP_strict_neq:
    case OP_is_null: case OP_is_undefined: case OP_is_undefined_or_null:
    case OP_typeof_is: case OP_typeof_is_function: case OP_typeof_is_undefined:
    case OP_lnot: case OP_dup_case_atom:
        return 0;
#elif RV_PART == RV_PART_PASS_FN
/* where the instruction after the one at pos is */
static int rv_next_pos(RVPass *s, int pos)
{
    return pos + short_opcode_info(s->bc[pos]).size;
}

/* the branch right after the instruction at pos (at rv_next_pos) when it is
   one and not a leader: its old target, and in *jt whether it jumps on
   true */
static int rv_next_branch(RVPass *s, int pos, int *jt)
{
    int q = rv_next_pos(s, pos);
    const uint8_t *c = s->bc + q;
    if (q >= s->len || s->leader[q])
        return -1;
    switch (c[0]) {
    case OP_if_false: case OP_if_true:
        *jt = c[0] == OP_if_true;
        return q + 1 + (int32_t)get_u32(c + 1);
    case OP_if_false8: case OP_if_true8:
        *jt = c[0] == OP_if_true8;
        return q + 1 + (int8_t)c[1];
    default:
        return -1;
    }
}

/* the register forms of a compare: the bool form (its K form next to it),
   the first of the four branches, whether the result is negated, and the
   compare with its operands the other way round */
static void rv_cmp_info(int op, int *bop, int *jbase, int *neg, int *mirror)
{
    *neg = 0;
    *mirror = op;
    switch (op) {
    case OP_lt: *bop = RV_LT; *jbase = RV_JLTF; *mirror = OP_gt; break;
    case OP_lte: *bop = RV_LTE; *jbase = RV_JLTEF; *mirror = OP_gte; break;
    case OP_gt: *bop = RV_GT; *jbase = RV_JGTF; *mirror = OP_lt; break;
    case OP_gte: *bop = RV_GTE; *jbase = RV_JGTEF; *mirror = OP_lte; break;
    case OP_eq: *bop = RV_EQ; *jbase = RV_JEQF; break;
    case OP_neq: *bop = RV_NE; *jbase = RV_JEQF; *neg = 1; break;
    case OP_strict_eq: *bop = RV_SEQ; *jbase = RV_JSEQF; break;
    case OP_strict_neq: *bop = RV_SNE; *jbase = RV_JSEQF; *neg = 1; break;
    default: abort();
    }
}

/* whether the last instruction emitted is a GETVR into the temp t, with
   nothing after it: what it reads can be read by the next instruction
   instead, as nothing runs between. A GETVR cannot throw and has no pcmap
   entry, and a block that starts at it starts at what replaces it. */
static BOOL rv_last_getvr(RVPass *s, int t)
{
    const uint8_t *c;
    if (s->last < 0 || s->last + 4 != (int)s->code.size)
        return FALSE;
    c = s->code.buf + s->last;
    return c[0] == RV_GETVR && (int8_t)c[1] == t;
}

/* compare and branch: the two operands are on the stack, the branch, which
   is at q, jumps to target when the compare is jump_true */
static void rv_cmp_branch(RVPass *s, int op_base, int op_kbase, int jump_true,
                          int target, int q)
{
    int a, i = s->sp - 2;
    RVEnt *eb;
    a = rv_slot_of(s, i);
    eb = &s->st[s->sp - 1];
    /* the stack under the operands: canonical at the block end */
    rv_block_end(s, s->sp - 2);
    if (op_base == RV_JSEQF && !jump_true && eb->kind == RV_E_TEMP &&
        rv_last_getvr(s, eb->slot)) {
        /* b is the GETVR just emitted: the jump reads its var ref */
        int idx = get_u16(s->code.buf + s->last + 2);
        s->code.size = s->last;
        rv_emit(s, RV_JSEQVF);
        rv_byte(s, a);
        rv_u16(s, idx);
    } else if (eb->kind == RV_E_K) {
        rv_emit(s, op_kbase + jump_true);
        rv_byte(s, a);
        rv_byte(s, eb->k);
    } else {
        rv_emit(s, op_base + jump_true);
        rv_byte(s, a);
        rv_byte(s, eb->slot);
    }
    rv_jump_to(s, target);
    rv_jump_throws(s, rv_owned(s, 2), q, TRUE);
    s->sp -= 2;
}

/* dup of a temp then a branch on the copy (a && b, a || b): the branch
   reads the temp and keeps it. When a drop of it follows the branch and
   no jump goes to the drop, as in the code of a && b, the branch frees the
   temp where it does not jump (JFND, JTND), and the drop only pops it (it
   starts a block, for it follows a jump: rv_one reads it after the leader
   made the stack canonical, which emits nothing, its entries are temps
   already). */
static BOOL rv_dup_branch(RVPass *s, int pos)
{
    int target, jt, slot, q, drop;
    if (s->st[s->sp - 1].kind != RV_E_TEMP)
        return FALSE;
    target = rv_next_branch(s, pos, &jt);
    if (target < 0)
        return FALSE;
    q = pos + 1;
    q += short_opcode_info(s->bc[q]).size;
    drop = q < s->len && !(s->leader[q] & (RV_LEAD_JUMP | RV_LEAD_CASE)) &&
        s->bc[q] == OP_drop;
    slot = s->st[s->sp - 1].slot;
    rv_block_end(s, s->sp);
    if (drop)
        rv_emit(s, jt ? RV_JTND : RV_JFND);
    else
        rv_emit(s, jt ? RV_JTN : RV_JFN);
    rv_byte(s, slot);
    rv_jump_to(s, target);
    /* the temp is owned when the interrupt throws, before it is freed */
    rv_jump_throws(s, rv_owned(s, 0), rv_next_pos(s, pos), FALSE);
    if (drop)
        s->drop_at = q;
    s->last = -2;
    return TRUE;
}

/* a branch on the top entry, as the if_false or if_true at q makes it: JF
   or JT */
static void rv_branch_top(RVPass *s, int jt, int target, int q)
{
    int slot = rv_slot_of(s, s->sp - 1);
    rv_block_end(s, s->sp - 1);
    rv_emit(s, jt ? RV_JT : RV_JF);
    rv_byte(s, slot);
    rv_jump_to(s, target);
    rv_jump_throws(s, rv_owned(s, 1), q, FALSE);
    s->sp--;
}

#define RV_CASES_MAX 64
/* the fewest cases that get a table: below, the chain costs less than the
   look up (about 23 instructions a case against about 45) */
#define RV_CASES_TABLE 4

/* whether the CASES of the n cases of atom gets a table: enough cases, and
   all of them string atoms, which goasm_case_atom tells apart by their
   pointer alone */
static BOOL rv_cases_tabled(RVPass *s, const JSAtom *atom, int n)
{
    int i;
    if (n < RV_CASES_TABLE)
        return FALSE;
    for (i = 0; i < n; i++) {
        if (__JS_AtomIsTaggedInt(atom[i]) ||
            s->rt->atom_array[atom[i]]->atom_type != JS_ATOM_TYPE_STRING)
            return FALSE;
    }
    return TRUE;
}

/* The table of a CASES, after its s and n: k, the stop when no case is
   equal, 2^k pointers to the strings of the atoms (NULL in a free slot)
   and 2^k stops, the string of an atom at its hash (JSString.hash) or in
   the next free slot after it, 2^k at least twice the cases. A stop is the
   case where the chain ends for that value, worked out by the chain's own
   rule: a case holds when its compare gives its flag, and the compare gives
   1 for the case's own atom only, so a case given twice stops at its first,
   and a case of flag 0 stops every other value. The strings live as long
   as the code: the bytecode holds a reference to each atom it names, and a
   resize of the atom table moves its array of pointers, not the strings.
   Returns the bytes it adds. */
static int rv_cases_table(RVPass *s, const JSAtom *atom, const int *jt, int n)
{
    JSString *key[2 * RV_CASES_MAX];
    uint8_t stop[2 * RV_CASES_MAX];
    int k = 3, size, i, j, h, miss;
    while ((1 << k) < 2 * n)
        k++;
    size = 1 << k;
    memset(key, 0, sizeof(key[0]) * size);
    memset(stop, 0, size);
    for (miss = 0; miss < n && jt[miss]; miss++)
        ;
    for (i = 0; i < n; i++) {
        JSString *p = s->rt->atom_array[atom[i]];
        for (h = p->hash & (size - 1); key[h] && key[h] != p; h = (h + 1) & (size - 1))
            ;
        if (key[h])
            continue;
        key[h] = p;
        for (j = 0; j < n && (atom[j] == atom[i]) != jt[j]; j++)
            ;
        stop[h] = j;
    }
    rv_byte(s, k);
    rv_byte(s, miss);
    if (dbuf_put(&s->code, (const uint8_t *)key, sizeof(key[0]) * size) ||
        dbuf_put(&s->code, stop, size))
        s->failed = TRUE;
    return 2 + (int)(sizeof(key[0]) + 1) * size;
}

/* dup_case_atom at pos and the cases that the same dispatch can take after
   it, as QuickJS's dup_case_atom takes them: dup_case_atom
   push_atom_value strict_eq and a conditional jump, the value at the top of
   the stack all along. The next case is the instruction after the jump
   (if_true: the case's body is elsewhere), when no jump lands on it, or the
   jump's target (if_false: the body is right after the case), when it lies
   further on. A case taken by its target is marked RV_LEAD_CASE: the pass
   still makes it a CASES of its own for any other jump that lands there, but
   one that does not follow targets, so that a long switch does not make its
   chain again from each case. The CASES ends with where to go when no case
   holds: the instruction where the chain stopped. */
static void rv_cases(RVPass *s, int pos)
{
    JSAtom atom[RV_CASES_MAX];
    int jt[RV_CASES_MAX], target[RV_CASES_MAX], n = 0, q = pos, i, slot;
    int follow = !(s->leader[pos] & RV_LEAD_CASE), contig = TRUE, skip = pos;
    int at[RV_CASES_MAX], other0 = 0, tab, tsize = 0;
    RVMask mask;
    uint32_t start, end;
    while (n < RV_CASES_MAX && q + 8 < s->len && s->bc[q] == OP_dup_case_atom &&
           (q == pos || !contig || !(s->leader[q] & RV_LEAD_JUMP))) {
        const uint8_t *c = s->bc + q;
        int jop = c[7], len, t;
        if (c[1] != OP_push_atom_value || c[6] != OP_strict_eq ||
            s->leader[q + 1] || s->leader[q + 6] || s->leader[q + 7])
            break;
        if (jop == OP_if_false8 || jop == OP_if_true8) {
            t = q + 8 + (int8_t)c[8];
            len = 2;
        } else if (jop == OP_if_false || jop == OP_if_true) {
            t = q + 8 + (int32_t)get_u32(c + 8);
            len = 5;
        } else {
            break;
        }
        if (q != pos && !contig)
            s->leader[q] |= RV_LEAD_CASE;
        if (contig)
            skip = q + 7 + len;
        atom[n] = get_u32(c + 2);
        at[n] = q;
        if (jop == OP_if_true8 || jop == OP_if_true) {
            jt[n] = 1;
            target[n] = t;
            q += 7 + len;
        } else if (follow && t > q && t + 8 < s->len && s->bc[t] == OP_dup_case_atom) {
            /* the body is next: jump there when the string is s, go on
               with the case at the target when it is not */
            jt[n] = 1;
            target[n] = q + 7 + len;
            q = t;
            contig = FALSE;
        } else {
            jt[n] = 0;
            target[n] = t;
            q += 7 + len;
        }
        /* where QuickJS's interpreter goes when the first case does not do
           what its register jump does (the interrupt below) */
        if (n == 0)
            other0 = q;
        n++;
    }
    if (n == 0) {
        /* not a whole case: a dup */
        static const int8_t m[] = { 0, 0 };
        rv_shuffle(s, 1, m, 2);
        return;
    }
    rv_block_end(s, s->sp);
    slot = s->st[s->sp - 1].slot;
    tab = rv_cases_tabled(s, atom, n);
    rv_emit(s, RV_CASES);
    rv_byte(s, slot);
    rv_byte(s, n | (tab ? 0x80 : 0));
    if (tab)
        tsize = rv_cases_table(s, atom, jt, n);
    for (i = 0; i < n; i++) {
        rv_u32(s, atom[i]);
        rv_byte(s, jt[i]);
        rv_jump_to(s, target[i]);
    }
    end = s->code.size;
    /* where the chain stopped: the next instruction, or a jump there */
    if (q == skip)
        rv_u16(s, 0);
    else
        rv_jump_to(s, q);
    /* Where CASES can throw, each with the pc it leaves. A compare of case
       i that needs the characters and is stopped (rv_case_slow): the end of
       the atom of the case, named at its strict_eq, the seventh byte of the
       old case. An interrupt: QuickJS's interpreter reads the flag after each
       case, so it stops at the first one, CASES reads it once after the chain
       and says what the first case did, as a conditional jump does
       (rv_jump_throws): its end less two when the first case jumped, named
       before its target, less one when it did not, named where QuickJS's
       interpreter goes then: in its branch, or before the case at its
       target. The value the table answers for stops where the chain would
       have, at the same places. */
    mask = rv_owned(s, 0);
    start = s->last;
    for (i = 0; i < n; i++)
        rv_pcmap_put(s, start + 3 + tsize + 7 * i + 4, at[i] + 6, mask);
    rv_pcmap_put(s, end - 2, target[0] - 1, mask);
    rv_pcmap_put(s, end - 1, other0 - 1, mask);
    rv_throws(s, mask);
    rv_hooks(s, RV_HOOK_APART2);
    s->skip_to = skip;
}
#elif RV_PART == RV_PART_PASS
    case OP_lt: case OP_lte: case OP_gt: case OP_gte:
    case OP_eq: case OP_neq: case OP_strict_eq: case OP_strict_neq:
        {
            int target, jt, bop, jbase, neg, mirror;
            RVEnt *ea = &s->st[s->sp - 2], *eb = &s->st[s->sp - 1];
            rv_cmp_info(op, &bop, &jbase, &neg, &mirror);
            /* a constant on the left only: the operands are swapped and
               the compare mirrored. A primitive constant has nothing to run
               when converted, so the order of the conversions cannot be
               seen. */
            if (ea->kind == RV_E_K && eb->kind != RV_E_K &&
                JS_VALUE_GET_TAG(s->k[ea->k]) != JS_TAG_OBJECT) {
                RVEnt t = *ea;
                *ea = *eb;
                *eb = t;
                rv_cmp_info(mirror, &bop, &jbase, &neg, &mirror);
            }
            target = rv_next_branch(s, pos, &jt);
            if (target >= 0) {
                rv_cmp_branch(s, jbase, jbase + 2, jt ^ neg, target, rv_next_pos(s, pos));
                /* the branch is read as part of this instruction */
                s->last = -2;
            } else {
                rv_binop(s, bop, bop + 1, -1);
            }
        }
        break;
    case OP_is_null: case OP_is_undefined: case OP_is_undefined_or_null:
    case OP_typeof_is: case OP_typeof_is_function: case OP_typeof_is_undefined:
        {
            int target, jt, kind = 0;
            BOOL ty = TRUE;
            JSAtom atom = JS_ATOM_NULL;
            switch (op) {
            case OP_is_null: ty = FALSE; kind = 0; break;
            case OP_is_undefined: ty = FALSE; kind = 1; break;
            case OP_is_undefined_or_null: ty = FALSE; kind = 2; break;
            case OP_typeof_is: atom = get_u32(p + 1); break;
            case OP_typeof_is_function: atom = JS_ATOM_function; break;
            default: atom = JS_ATOM_undefined; break;
            }
            slot = rv_slot_of(s, s->sp - 1);
            target = rv_next_branch(s, pos, &jt);
            if (target >= 0) {
                rv_block_end(s, s->sp - 1);
                rv_emit(s, ty ? (jt ? RV_JTYPET : RV_JTYPEF) : (jt ? RV_JIST : RV_JISF));
                rv_byte(s, slot);
                if (ty)
                    rv_u32(s, atom);
                else
                    rv_byte(s, kind);
                rv_jump_to(s, target);
                rv_jump_throws(s, rv_owned(s, 1), rv_next_pos(s, pos), FALSE);
                s->sp--;
                s->last = -2;
            } else {
                d = rv_tslot(s, s->sp - 1);
                rv_emit(s, ty ? RV_TYPEIS : RV_ISX);
                rv_byte(s, d);
                rv_byte(s, slot);
                if (ty)
                    rv_u32(s, atom);
                else
                    rv_byte(s, kind);
                s->sp--;
                rv_push(s, RV_E_TEMP, d, 0);
                rv_mark_dst(s, d);
            }
        }
        break;
    case OP_lnot:
        {
            int target, jt;
            e = s->st[s->sp - 1];
            if (e.kind == RV_E_K && !rv_kref(s, e.k) &&
                (uint32_t)JS_VALUE_GET_TAG(s->k[e.k]) <= JS_TAG_UNDEFINED) {
                /* !0 and !1, the true and false of minified code */
                s->sp--;
                rv_push(s, RV_E_K, 0, rv_k(s, JS_NewBool(NULL, !JS_VALUE_GET_INT(s->k[e.k]))));
                break;
            }
            target = rv_next_branch(s, pos, &jt);
            if (target >= 0) {
                /* !x then a branch: a branch on x the other way */
                rv_branch_top(s, !jt, target, rv_next_pos(s, pos));
                s->last = -2;
                break;
            }
            slot = rv_slot_of(s, s->sp - 1);
            d = rv_tslot(s, s->sp - 1);
            rv_emit(s, RV_LNOT);
            rv_byte(s, d);
            rv_byte(s, slot);
            s->sp--;
            rv_push(s, RV_E_TEMP, d, 0);
            rv_mark_dst(s, d);
        }
        break;
    case OP_dup_case_atom:
        rv_cases(s, pos);
        break;
#elif RV_PART == RV_PART_RUN_FN
/* the stop of a CASES with a table for v (goasm_case_table), out of the
   loop: inline, the look up took registers of the loop, and on amd64 the
   calls spilled two more of them */
static no_inline int rv_case_table(JSValueConst v, const uint8_t *tab)
{
    return goasm_case_table(v, tab);
}

/* a case whose string the atom does not tell apart from s by its pointer
   and length */
static no_inline int rv_case_slow(JSContext *ctx, JSValueConst v, JSAtom atom)
{
    JSValue a;
    int res;
    if (RV_INJ_HIT(ctx))
        return -1;
    a = JS_AtomToValue(ctx, atom);
    if (JS_IsException(a))
        return -1;
    res = js_strict_eq_poll(ctx, v, a);
    JS_FreeValue(ctx, a);
    return res;
}

/* the truth of a branch operand kept in its temp */
static no_inline int rv_tobool_keep(JSContext *ctx, JSValueConst v)
{
    return JS_ToBool(ctx, v);
}

static force_inline BOOL rv_isx(JSValueConst v, int kind)
{
    uint32_t tag = JS_VALUE_GET_TAG(v);
    switch (kind) {
    case 0: return tag == JS_TAG_NULL;
    case 1: return tag == JS_TAG_UNDEFINED;
    default: return tag == JS_TAG_NULL || tag == JS_TAG_UNDEFINED;
    }
}

/* two numbers (an int or a float each) as doubles, for a relational
   compare, which then gives what js_relational_slow gives them: an int is
   exact as a double, and a compare with NaN is false */
static force_inline BOOL rv_num_pair(JSValueConst a, JSValueConst b,
                                     double *d1, double *d2)
{
    uint32_t ta = JS_VALUE_GET_TAG(a), tb = JS_VALUE_GET_TAG(b);
    if (!((ta == JS_TAG_INT || JS_TAG_IS_FLOAT64(ta)) &&
          (tb == JS_TAG_INT || JS_TAG_IS_FLOAT64(tb))))
        return FALSE;
    *d1 = ta == JS_TAG_INT ? JS_VALUE_GET_INT(a) : JS_VALUE_GET_FLOAT64(a);
    *d2 = tb == JS_TAG_INT ? JS_VALUE_GET_INT(b) : JS_VALUE_GET_FLOAT64(b);
    return TRUE;
}

/* the relational compare rel (0 <, 1 <=, 2 >, 3 >=, as OP_lt to OP_gte) */
static force_inline BOOL rv_rel(int rel, double d1, double d2)
{
    switch (rel) {
    case 0: return d1 < d2;
    case 1: return d1 <= d2;
    case 2: return d1 > d2;
    default: return d1 >= d2;
    }
}
#elif RV_PART == RV_PART_RUN
    /* a compare into a bool: two ints here, the equalities that need no
       conversion (goasm_eq_fast, as QuickJS's interpreter), the rest in
       rv_binop_slow. eqk: -1 a relational compare, 0 eq, 1 strict. */
#define RV_CMPB(label, opc, cop, bexpr, isk, eqk, neg)                  \
 label:                                                                 \
    ip = pc; pc += 3;                                                   \
    {                                                                   \
        JSValue a = RV_R(ip[1]), bv = (bexpr);                          \
        int res;                                                        \
        if (likely(JS_VALUE_IS_BOTH_INT(a, bv)) && !RV_INJ_ARMED) {     \
            RV_PUTD(ip[0], JS_NewBool(ctx, JS_VALUE_GET_INT(a) cop JS_VALUE_GET_INT(bv))); \
            RV_NEXT();                                                  \
        }                                                               \
        if ((eqk) < 0)                                                  \
            goto rv_cmp_other;                                          \
        if ((eqk) >= 0 && !RV_INJ_ARMED &&                              \
            (res = goasm_eq_fast(a, bv, (eqk) > 0)) >= 0) {             \
            if (rv_is_temp(F, (int8_t)ip[1]))                           \
                JS_FreeValue(ctx, a);                                   \
            if (!(isk) && rv_is_temp(F, (int8_t)ip[2]))                 \
                JS_FreeValue(ctx, bv);                                  \
            RV_PUTD(ip[0], JS_NewBool(ctx, res ^ (neg)));               \
            RV_NEXT();                                                  \
        }                                                               \
        RV_SYNC_TOP();                                                  \
        if (rv_binop_slow(ctx, F, ip, opc, isk, TRUE) < 0)              \
            goto rv_exception;                                          \
    }                                                                   \
    RV_NEXT();

    RV_CMPB(rv_LT, OP_lt, <, RV_R(ip[2]), FALSE, -1, 0)
    RV_CMPB(rv_LTK, OP_lt, <, RV_K(ip[2]), TRUE, -1, 0)
    RV_CMPB(rv_LTE, OP_lte, <=, RV_R(ip[2]), FALSE, -1, 0)
    RV_CMPB(rv_LTEK, OP_lte, <=, RV_K(ip[2]), TRUE, -1, 0)
    RV_CMPB(rv_GT, OP_gt, >, RV_R(ip[2]), FALSE, -1, 0)
    RV_CMPB(rv_GTK, OP_gt, >, RV_K(ip[2]), TRUE, -1, 0)
    RV_CMPB(rv_GTE, OP_gte, >=, RV_R(ip[2]), FALSE, -1, 0)
    RV_CMPB(rv_GTEK, OP_gte, >=, RV_K(ip[2]), TRUE, -1, 0)
    RV_CMPB(rv_EQ, OP_eq, ==, RV_R(ip[2]), FALSE, 0, 0)
    RV_CMPB(rv_EQK, OP_eq, ==, RV_K(ip[2]), TRUE, 0, 0)
    RV_CMPB(rv_NE, OP_neq, !=, RV_R(ip[2]), FALSE, 0, 1)
    RV_CMPB(rv_NEK, OP_neq, !=, RV_K(ip[2]), TRUE, 0, 1)
    RV_CMPB(rv_SEQ, OP_strict_eq, ==, RV_R(ip[2]), FALSE, 1, 0)
    RV_CMPB(rv_SEQK, OP_strict_eq, ==, RV_K(ip[2]), TRUE, 1, 0)
    RV_CMPB(rv_SNE, OP_strict_neq, !=, RV_R(ip[2]), FALSE, 1, 1)
    RV_CMPB(rv_SNEK, OP_strict_neq, !=, RV_K(ip[2]), TRUE, 1, 1)
#undef RV_CMPB

    /* compare and branch, as a compare then if_false or if_true. w: the
       bytes of b, 2 for the index of a var ref (isk 4, rv_binop_slow) */
#define RV_JCMP(label, opc, cop, bexpr, isk, eqk, jt, w)                \
 label:                                                                 \
    ip = pc; pc += 3 + (w);                                             \
    {                                                                   \
        JSValue a = RV_R(ip[0]), bv = (bexpr);                          \
        int res;                                                        \
        if (likely(JS_VALUE_IS_BOTH_INT(a, bv)) && !RV_INJ_ARMED) {     \
            res = JS_VALUE_GET_INT(a) cop JS_VALUE_GET_INT(bv);         \
        } else if ((eqk) < 0) {                                         \
            goto rv_jcmp_other;                                         \
        } else if ((eqk) >= 0 && !RV_INJ_ARMED &&                       \
                   (res = goasm_eq_fast(a, bv, (eqk) > 0)) >= 0) {      \
            if (rv_is_temp(F, (int8_t)ip[0]))                           \
                JS_FreeValue(ctx, a);                                   \
            if (!(isk) && rv_is_temp(F, (int8_t)ip[1]))                 \
                JS_FreeValue(ctx, bv);                                  \
        } else {                                                        \
            RV_SYNC_TOP();                                              \
            res = rv_binop_slow(ctx, F, ip, opc, isk, FALSE);           \
            if (res < 0)                                                \
                goto rv_exception;                                      \
            RV_JUMP_SLOW(res == (jt), (int16_t)get_u16(ip + 1 + (w)),   \
                         1);                                            \
        }                                                               \
        RV_JUMP_POLL(res == (jt), 1);                                   \
        if (res == (jt)) {                                              \
            int off = (int16_t)get_u16(ip + 1 + (w));                   \
            if (off < 0)                                                \
                poll_count(POLL_WEIGHT);                                \
            pc += off;                                                  \
        }                                                               \
    }                                                                   \
    RV_NEXT();

#define RV_JCMP4(name, opc, cop, eqk)                                   \
    RV_JCMP(rv_J##name##F, opc, cop, RV_R(ip[1]), FALSE, eqk, 0, 1)     \
    RV_JCMP(rv_J##name##T, opc, cop, RV_R(ip[1]), FALSE, eqk, 1, 1)     \
    RV_JCMP(rv_J##name##KF, opc, cop, RV_K(ip[1]), TRUE, eqk, 0, 1)     \
    RV_JCMP(rv_J##name##KT, opc, cop, RV_K(ip[1]), TRUE, eqk, 1, 1)
    RV_JCMP4(LT, OP_lt, <, -1)
    RV_JCMP4(LTE, OP_lte, <=, -1)
    RV_JCMP4(GT, OP_gt, >, -1)
    RV_JCMP4(GTE, OP_gte, >=, -1)
    RV_JCMP4(EQ, OP_eq, ==, 0)
    RV_JCMP4(SEQ, OP_strict_eq, ==, 1)
#undef RV_JCMP4
    RV_JCMP(rv_JSEQVF, OP_strict_eq, ==,
            *((RVHeader *)F)->var_refs[get_u16(ip + 1)]->pvalue, 4, 1, 0, 2)
#undef RV_JCMP

    /* the rest of the relational compares, shared by the eight into a bool
       and by the sixteen jumps: two numbers here, as doubles, else
       rv_binop_slow. Which one it is comes from the opcode byte again, as in
       rv_arith_other */
 rv_cmp_other:
    n = ip[-1] - RV_LT;
    {
        JSValue a = RV_R(ip[1]), bv = (n & 1) ? RV_K(ip[2]) : RV_R(ip[2]);
        double d1, d2;
        if (!RV_INJ_ARMED && rv_num_pair(a, bv, &d1, &d2)) {
            RV_PUTD(ip[0], JS_NewBool(ctx, rv_rel(n >> 1, d1, d2)));
            RV_NEXT();
        }
        RV_SYNC_TOP();
        if (rv_binop_slow(ctx, F, ip, OP_lt + (n >> 1), n & 1, TRUE) < 0)
            goto rv_exception;
    }
    RV_NEXT();
 rv_jcmp_other:
    n = ip[-1] - RV_JLTF;
    {
        JSValue a = RV_R(ip[0]), bv = (n & 2) ? RV_K(ip[1]) : RV_R(ip[1]);
        double d1, d2;
        int res, jt = n & 1;
        if (!RV_INJ_ARMED && rv_num_pair(a, bv, &d1, &d2)) {
            res = rv_rel(n >> 2, d1, d2);
        } else {
            RV_SYNC_TOP();
            res = rv_binop_slow(ctx, F, ip, OP_lt + (n >> 2), (n >> 1) & 1, FALSE);
            if (res < 0)
                goto rv_exception;
            RV_JUMP_SLOW(res == jt, (int16_t)get_u16(ip + 2), 1);
        }
        RV_JUMP_POLL(res == jt, 1);
        if (res == jt) {
            int off = (int16_t)get_u16(ip + 2);
            if (off < 0)
                poll_count(POLL_WEIGHT);
            pc += off;
        }
    }
    RV_NEXT();

    /* the tests of one value: a temp operand is freed */
 rv_ISX:
    ip = pc; pc += 3;
    {
        JSValue v = RV_R(ip[1]);
        BOOL res = rv_isx(v, ip[2]);
        if (rv_is_temp(F, (int8_t)ip[1]))
            JS_FreeValue(ctx, v);
        RV_PUTD(ip[0], JS_NewBool(ctx, res));
    }
    RV_NEXT();
 rv_TYPEIS:
    ip = pc; pc += 6;
    {
        JSValue v = RV_R(ip[1]);
        BOOL res = js_operator_typeof(ctx, v) == get_u32(ip + 2);
        if (rv_is_temp(F, (int8_t)ip[1]))
            JS_FreeValue(ctx, v);
        RV_PUTD(ip[0], JS_NewBool(ctx, res));
    }
    RV_NEXT();
#define RV_JTEST(label, len, test, jt)                                  \
 label:                                                                 \
    ip = pc; pc += (len);                                               \
    {                                                                   \
        JSValue v = RV_R(ip[0]);                                        \
        BOOL res = (test);                                              \
        if (rv_is_temp(F, (int8_t)ip[0]))                               \
            JS_FreeValue(ctx, v);                                       \
        RV_JUMP_POLL(res == (jt), 0);                                   \
        if (res == (jt)) {                                              \
            int off = (int16_t)get_u16(pc - 2);                         \
            if (off < 0)                                                \
                poll_count(POLL_WEIGHT);                                \
            pc += off;                                                  \
        }                                                               \
    }                                                                   \
    RV_NEXT();
    RV_JTEST(rv_JISF, 4, rv_isx(v, ip[1]), 0)
    RV_JTEST(rv_JIST, 4, rv_isx(v, ip[1]), 1)
    RV_JTEST(rv_JTYPEF, 7, js_operator_typeof(ctx, v) == get_u32(ip + 1), 0)
    RV_JTEST(rv_JTYPET, 7, js_operator_typeof(ctx, v) == get_u32(ip + 1), 1)
#undef RV_JTEST

 rv_LNOT:
    ip = pc; pc += 2;
    {
        JSValue v = RV_R(ip[1]);
        int res;
        if ((uint32_t)JS_VALUE_GET_TAG(v) <= JS_TAG_UNDEFINED) {
            res = JS_VALUE_GET_INT(v) != 0;
        } else {
            RV_SYNC_TOP();
            res = rv_tobool_slow(ctx, F, (int8_t)ip[1]);
        }
        RV_PUTD(ip[0], JS_NewBool(ctx, !res));
    }
    RV_NEXT();

#define RV_JBOOLN(label, jt, drop)                                      \
 label:                                                                 \
    ip = pc; pc += 3;                                                   \
    {                                                                   \
        JSValue v = RV_R(ip[0]);                                        \
        int res;                                                        \
        if ((uint32_t)JS_VALUE_GET_TAG(v) <= JS_TAG_UNDEFINED) {        \
            res = JS_VALUE_GET_INT(v) != 0;                             \
        } else {                                                        \
            RV_SYNC_TOP();                                              \
            res = rv_tobool_keep(ctx, v);                               \
            if (!(drop) || res == (jt))                                 \
                RV_JUMP_SLOW(res == (jt), (int16_t)get_u16(ip + 1), 0); \
            RV_JUMP_POLL(0, 0);                                         \
            JS_FreeValue(ctx, v);                                       \
            RV_NEXT();                                                  \
        }                                                               \
        RV_JUMP_POLL(res == (jt), 0);                                   \
        if (res == (jt)) {                                              \
            int off = (int16_t)get_u16(ip + 1);                         \
            if (off < 0)                                                \
                poll_count(POLL_WEIGHT);                                \
            pc += off;                                                  \
        } else if (drop) {                                              \
            JS_FreeValue(ctx, v);                                       \
        }                                                               \
    }                                                                   \
    RV_NEXT();
    RV_JBOOLN(rv_JFN, 0, 0)
    RV_JBOOLN(rv_JTN, 1, 0)
    RV_JBOOLN(rv_JFND, 0, 1)
    RV_JBOOLN(rv_JTND, 1, 1)
#undef RV_JBOOLN

    /* a chain of string cases in one dispatch, as dup_case_atom of QuickJS's
       interpreter: the first case that holds jumps, and when none does, the
       last offset */
 rv_CASES:
    ip = pc;
    {
        JSValue v = RV_R(ip[0]);
        const uint8_t *c = ip + 2, *c0;
        int res, off, n = ip[1], st = -1;
        if (n & 0x80) {
            /* a table first (rv_cases_table): the stop of v, when it
               tells it, in place of the compares */
            int tsize = 2 + (int)((sizeof(JSString *) + 1) << c[0]);
            n &= 0x7f;
            if (!RV_INJ_ARMED)
                st = rv_case_table(v, c);
            c += tsize;
        }
        c0 = c;
        pc = c + 7 * n;
        if (st >= 0) {
            c += 7 * st;
        } else {
            for (; c < pc; c += 7) {
                res = RV_INJ_ARMED ? -1 : goasm_case_atom(rt, v, get_u32(c));
                if (unlikely(res < 0)) {
                    RV_SYNC_TOP();
                    res = rv_case_slow(ctx, v, get_u32(c));
                    if (res < 0) {
                        pc = c + 4; /* rv_cases */
                        goto rv_exception;
                    }
                }
                if (res == c[4])
                    break;
            }
        }
        RV_JUMP_POLL(c == c0, 1);
        if (c < pc) {
            off = (int16_t)get_u16(c + 5);
            pc = c + 7 + off;
        } else {
            off = (int16_t)get_u16(pc);
            pc += 2 + off;
        }
        if (off < 0)
            poll_count(POLL_WEIGHT);
    }
    RV_NEXT();
#endif
