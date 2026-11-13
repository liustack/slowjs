/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Array destructuring with no iterator: DSTART and DNEXT. Read once for
   each part, see regvm-areas.h.

   The parser destructures [a, b] = v as for_of_start, for_of_next 0 and
   drop for each element, then iterator_close. DSTART is that for_of_start
   when the next old instruction is not a goto (a for-of loop) or a drop
   (yield*), and the record it makes is read only by its for_of_next and
   iterator_close (rv_dst_scan, regvm-pass.h). When js_dst_ok (quickjs.c)
   holds for v, it makes no iterator: the record is v, the built-in next,
   and in place of the catch offset 0 of an iterator the index of its next
   element (JS_DST_REC, below 0). DNEXT is for_of_next then drop: the
   element in the temp where the value was. Every other reader of such a
   record knows it: for_of_next of the window (js_for_of_next),
   iterator_close (rv_w_iterator_close) and the unwinding (rv_unwind). */

#if RV_PART == RV_PART_LIST
RV_OP(DSTART)   /* w: as W_FOR_OF_START */
RV_OP(DNEXT)    /* w offset8: W_FOR_OF_NEXT then drop, the value in temp w */
#elif RV_PART == RV_PART_INFO
#elif RV_PART == RV_PART_PASS_FN
/* the for_of_start at pos destructures an array and may make DSTART */
static BOOL rv_dst_site(RVPass *s, int pos)
{
    int i, q = pos + 1;
    if (s->fold || s->dst_off || q >= s->len)
        return FALSE;
    switch (s->bc[q]) {
    case OP_goto: case OP_goto8: case OP_goto16: case OP_drop:
        return FALSE;
    }
    for (i = 0; i < s->dst_no_n; i++) {
        if (s->dst_no[i] == pos)
            return FALSE;
    }
    return TRUE;
}

/* for_of_next at s->pos then a drop no jump reaches: DNEXT, which leaves
   the value only. The iterator, its next method and the catch offset are
   temps at their depths, as rv_window leaves them, the entries above them
   are not read. */
static BOOL rv_dst_next(RVPass *s, const uint8_t *p)
{
    int q = s->pos + 2, i, o = p[1], w;
    if (s->fold || q >= s->len || s->bc[q] != OP_drop || s->leader[q])
        return FALSE;
    if (3 + o > s->sp) {
        rv_crafted(s);
        return TRUE;
    }
    for (i = s->sp - 3 - o; i < s->sp - o; i++)
        rv_materialize(s, i);
    w = rv_tslot(s, s->sp);
    rv_emit(s, RV_DNEXT);
    rv_byte(s, w);
    rv_byte(s, o);
    rv_throws(s, rv_owned(s, 0));
    rv_hooks(s, RV_HOOK_APART);
    rv_push(s, RV_E_TEMP, w, 0);
    s->skip_to = q + 1;
    return TRUE;
}
#elif RV_PART == RV_PART_PASS
#elif RV_PART == RV_PART_RUN_FN
/* DSTART: the record of an array with no iterator when js_dst_ok holds,
   or the iterator of W_FOR_OF_START. When there is no memory to make the
   built-in next, the value stays where it was, as when GetIterator
   throws. */
static no_inline int rv_w_dstart(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue *sp = F + (int8_t)ip[0];
    int ok;
    if (!RV_INJ_ARMED) {
        ok = js_dst_ok(ctx, sp[-1], &sp[0]);
        if (ok < 0)
            return -1;
        if (ok) {
            sp[1] = JS_DST_REC(0);
            return 0;
        }
    }
    return rv_w_for_of_start(ctx, F, ip);
}
#elif RV_PART == RV_PART_RUN
 rv_DSTART:
    ip = pc;
    pc += 1;
    RV_SYNC_TOP();
    if (rv_w_dstart(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();

    /* the element of a fast array that has it, with the poll of
       js_for_of_array_next. The rest in js_for_of_next, by the window of
       for_of_next, which leaves the done flag above the value. */
 rv_DNEXT:
    ip = pc;
    pc += 2;
    {
        JSValue *it = F + (int8_t)ip[0] - 3 - ip[1];
        /* n is the index of the element of a record of DSTART, whose
           catch offset is -(n + 1), n at most JS_DST_LAST. The catch
           offset 0 of every other record (n = -1) and the index
           JS_DST_LAST, past which there is none, take the slow way. */
        if (likely(JS_VALUE_GET_TAG(it[2]) == JS_TAG_CATCH_OFFSET &&
                   JS_VALUE_GET_TAG(it[0]) == JS_TAG_OBJECT && !RV_INJ_ARMED)) {
            p = JS_VALUE_GET_OBJ(it[0]);
            n = -JS_VALUE_GET_INT(it[2]) - 1;
            if (likely(p->fast_array && (uint32_t)n < p->u.array.count &&
                       (uint32_t)n < JS_DST_LAST)) {
                if (unlikely(rv_poll_interrupts(rt, ctx))) {
                    /* js_for_of_next frees the iterator when the read
                       throws */
                    JS_FreeValue(ctx, it[0]);
                    it[0] = JS_UNDEFINED;
                    RV_SYNC_TOP();
                    goto rv_exception;
                }
                it[2] = JS_DST_REC(n + 1);
                RV_R(ip[0]) = JS_DupValue(ctx, p->u.array.u.values[n]);
                RV_NEXT();
            }
        }
    }
    RV_SYNC_TOP();
    if (rv_w_for_of_next(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();
#endif
