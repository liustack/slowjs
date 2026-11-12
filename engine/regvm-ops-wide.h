/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The ops of the functions too large for a byte operand: a constant past
   the 256 a byte names, and the slots below F past -128 (the locals,
   arguments, this and func of a function of more than 128 of them), and the
   jumps past the 32 KB an off16 reaches. Every other op names its operands
   with a byte and its jump with an off16, and a large function reaches
   what is past them only through these, which the pass (regvm-pass.h) puts
   in before the op that needs the value, so that no op of a function within
   the limits changes:

   - a far local, argument, this or func is copied into its temp when it is
     pushed (MOVW, in rv_push), and a store to it goes through its temp
     (SETLW, in rv_store);
   - the old instructions that name a local themselves (inc_loc, dec_loc,
     add_loc, the checks of let and const, set_loc_uninitialized) are read
     here when the local is far (rv_wide_one): the value goes through the
     scratch slot of the frame, the one past its stack, and CHKLW makes the
     check of the TDZ with the message of the old loop;
   - a jump of a function whose code is too long for its off16 goes through
     an island after it, a JMPW with an off32 (rv_islands);
   - a frame with so many captured locals that their var_refs would push
     its temps past slot 127 keeps its JSStackFrame only, and its var_refs
     are on the heap (rf->vr_heap): CLW 0 makes them in place of CLINIT, and
     CLW 1 closes and frees them before each return, which is then the one
     of a frame with no closure state. The unwinding does the same
     (RV_CLOSE_VAR_REFS). A frame within the limits runs CLINIT and the
     RETC returns as before.

   Read once for each part, see regvm-areas.h. */

#if RV_PART == RV_PART_LIST
RV_OP(LOADKW)   /* d16 k32   temp d16 = constant k32, with a reference */
RV_OP(MOVW)     /* d16 s16   F[d16] = F[s16], with a reference */
RV_OP(SETLW)    /* d16 t16   local d16 = temp t16, moved, the old value freed */
RV_OP(CHKLW)    /* s16 kind  throws when the local s16 is uninitialized (kind 0)
                             or when it is not (kind 1) */
RV_OP(JMPW)     /* off32     pc += off32, the interrupt flag not polled: the
                             islands of a long function (rv_islands) */
RV_OP(CLW)      /* kind      the var_refs of the closure state on the heap:
                             made (kind 0, the first instruction), closed and
                             freed (kind 1, before a return) */
#elif RV_PART == RV_PART_PASS_FN
/* the old instructions that name a far local themselves, in a function with
   far slots (rv_build sets s->wide): TRUE when read here. Each goes through
   the scratch slot or a check, then the local is read or written by MOVW
   and SETLW. */
static BOOL rv_wide_one(RVPass *s, int op, const uint8_t *p)
{
    int slot, d, scr = rv_scratch(s);
    RVEnt e;

    switch (op) {
    case OP_inc_loc: case OP_dec_loc:
        slot = rv_loc(s, p[1]);
        if (!rv_far_slot(slot))
            return FALSE;
        rv_movw(s, RV_MOVW, scr, slot);
        rv_emit(s, op == OP_inc_loc ? RV_INC : RV_DEC);
        rv_byte(s, scr);
        rv_byte(s, scr);
        rv_throws(s, rv_owned(s, 0));
        rv_movw(s, RV_SETLW, slot, scr);
        return TRUE;
    case OP_add_loc:
        slot = rv_loc(s, p[1]);
        if (!rv_far_slot(slot))
            return FALSE;
        rv_movw(s, RV_MOVW, scr, slot);
        e = s->st[s->sp - 1];
        rv_emit(s, e.kind == RV_E_K ? RV_ADDK : RV_ADD);
        rv_byte(s, scr);
        rv_byte(s, scr);
        rv_byte(s, e.kind == RV_E_K ? e.k : e.slot);
        rv_throws(s, rv_owned(s, 1));
        s->sp--;
        rv_movw(s, RV_SETLW, slot, scr);
        return TRUE;
    case OP_get_loc_check: case OP_get_loc_checkthis:
        slot = rv_loc(s, get_u16(p + 1));
        if (!rv_far_slot(slot))
            return FALSE;
        rv_emit(s, RV_CHKLW);
        rv_u16(s, slot);
        rv_byte(s, 0);
        rv_throws(s, rv_owned(s, 0));
        d = rv_tslot(s, s->sp);
        rv_movw(s, RV_MOVW, d, slot);
        rv_push(s, RV_E_TEMP, d, 0);
        return TRUE;
    case OP_put_loc_check: case OP_put_loc_check_init:
        slot = rv_loc(s, get_u16(p + 1));
        if (!rv_far_slot(slot))
            return FALSE;
        /* the value stays on the stack across the check, owned there */
        rv_emit(s, RV_CHKLW);
        rv_u16(s, slot);
        rv_byte(s, op == OP_put_loc_check_init);
        rv_throws(s, rv_owned(s, 0));
        rv_store(s, slot);
        return TRUE;
    case OP_set_loc_uninitialized:
        slot = rv_loc(s, get_u16(p + 1));
        if (!rv_far_slot(slot))
            return FALSE;
        /* LOADKW: the constant may be past the 256 a byte names */
        rv_emit(s, RV_LOADKW);
        rv_u16(s, scr);
        rv_u32(s, rv_k(s, JS_UNINITIALIZED));
        rv_movw(s, RV_SETLW, slot, scr);
        return TRUE;
    default:
        return FALSE;
    }
}

/* CLW 0, the first instruction of a frame whose var_refs are on the heap.
   It can throw (no memory): its entry names no temp and the first old
   instruction for the line, and the throw-injection test has no old
   instruction to compare it with (pos -1, RV_HOOK_APART). */
static void rv_clw_init_pass(RVPass *s)
{
    RVMask none;

    memset(&none, 0, sizeof(none));
    s->pos = 0;
#ifdef RV_INJECT
    s->ipos = -1;
#endif
    rv_emit(s, RV_CLW);
    rv_byte(s, 0);
    rv_throws(s, none);
    rv_hooks(s, RV_HOOK_APART);
}
#elif RV_PART == RV_PART_RUN_FN
/* the error of CHKLW, ip at s16 kind, as rv_check_throw makes those of
   GETLC, PUTLC and PUTLCI */
static no_inline void rv_chklw_throw(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RVFunc *rf = ((RVHeader *)F)->rf;
    if (ip[2] == 0)
        JS_ThrowReferenceErrorUninitialized2(ctx, rf->b, (int16_t)get_u16(ip) + rf->b->var_count,
                                             FALSE);
    else
        JS_ThrowReferenceError(ctx, "'this' can be initialized only once");
}

/* CLW 0: the closure state of the frame F as rv_clinit makes it, with its
   var_refs on the heap. Until they are, sf->var_refs is NULL, which the
   unwinding of a failure here reads as nothing to close. */
static no_inline int rv_clw_init(JSContext *ctx, JSValue *F)
{
    RVFunc *rf = ((RVHeader *)F)->rf;
    JSStackFrame *sf = RV_SF(F);
    JSVarRef **tab;

    sf->prev_frame = NULL;
    sf->cur_func = F[-rf->fofs];
    sf->arg_buf = F - rf->fofs + 1;
    sf->var_buf = F - rf->b->var_count;
    sf->var_refs = NULL;
    sf->cur_pc = NULL;
    sf->arg_count = rf->arg_count;
    sf->js_mode = rf->b->js_mode;
    sf->cur_sp = NULL;
    if (RV_INJ_HIT(ctx))
        return -1;
    tab = js_mallocz(ctx, sizeof(tab[0]) * rf->var_ref_count);
    if (!tab)
        return -1;
    sf->var_refs = tab;
    return 0;
}

/* CLW 1 and the unwinding (RV_CLOSE_VAR_REFS): the var_refs of the frame F
   closed, as rv_close_frame does, then freed */
static no_inline void rv_clw_exit(JSRuntime *rt, JSValue *F)
{
    JSStackFrame *sf = RV_SF(F);

    if (!sf->var_refs)
        return;
    close_var_refs(rt, ((RVHeader *)F)->rf->b, sf);
    js_free_rt(rt, sf->var_refs);
    sf->var_refs = NULL;
}
#elif RV_PART == RV_PART_RUN
 rv_LOADKW:
    ip = pc; pc += 6;
    F[(int16_t)get_u16(ip)] = JS_DupValue(ctx, RV_K(get_u32(ip + 2)));
    RV_NEXT();
 rv_MOVW:
    ip = pc; pc += 4;
    F[(int16_t)get_u16(ip)] = JS_DupValue(ctx, F[(int16_t)get_u16(ip + 2)]);
    RV_NEXT();
 rv_SETLW:
    ip = pc; pc += 4;
    {
        JSValue *q = &F[(int16_t)get_u16(ip)], old = *q;
        *q = F[(int16_t)get_u16(ip + 2)];
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();
 rv_CHKLW:
    ip = pc; pc += 3;
    if (RV_INJ_HIT(ctx) || unlikely(JS_IsUninitialized(F[(int16_t)get_u16(ip)]) == !ip[2])) {
        rv_chklw_throw(ctx, F, ip);
        RV_INJ_AGAIN(ctx);
        goto rv_exception;
    }
    RV_NEXT();
 rv_CLW:
    ip = pc; pc += 1;
    if (ip[0] == 0) {
        if (rv_clw_init(ctx, F))
            goto rv_exception;
    } else {
        rv_clw_exit(rt, F);
    }
    RV_NEXT();
 rv_JMPW:
    ip = pc; pc += 4;
    {
        int32_t off = get_u32(ip);
        if (off < 0)
            poll_count(POLL_WEIGHT);
        pc += off;
    }
    RV_NEXT();
#endif
