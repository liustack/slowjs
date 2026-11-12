/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Closures in the register loop: fclosure, the closure variables
   written, close_loc and the return of a frame with captured locals. Read
   once for each part, see regvm-areas.h.

   A frame with captured locals keeps a JSStackFrame and its var_refs in its
   closure state (RV_SF, regvm.h), so the old loop's get_var_ref, js_closure,
   close_lexical_var and close_var_refs run on it unchanged: a JSVarRef
   points at the slot of the local until the frame closes it. Its returns
   (RETCT to RETCU) and the unwinding (RV_CLOSE_VAR_REFS) close the var_refs
   before the slots are freed, as JS_CallInternal does at its end. */

#if RV_PART == RV_PART_LIST
RV_OP(CLINIT)   /* -         the first instruction of a frame with captured locals */
RV_OP(RETCT) RV_OP(RETCL) RV_OP(RETCK) RV_OP(RETCU)   /* t | s | k | -: RET to RETU after closing */
RV_OP(FCLOS)    /* d idx16   a closure of the constant idx of the bytecode */
RV_OP(PUTVR)    /* t idx16   closure variable idx = the temp t, moved */
RV_OP(SETVR)    /* s idx16   closure variable idx = s, with a reference */
RV_OP(SETVRK)   /* k idx16   closure variable idx = the constant k */
RV_OP(PUTVRC)   /* s idx16   closure variable idx, which must be initialized, = s, taken */
RV_OP(ADDVR)    /* a b idx16 closure variable idx = a + b, a and b taken (add then put_var_ref) */
RV_OP(ADDVRC)   /* a b idx16 the same, the variable must be initialized (add then put_var_ref_check) */
RV_OP(GETVRC)   /* d idx16   d = closure variable idx, which must be initialized */
RV_OP(GETLC)    /* d s       d = the local s, which must be initialized */
RV_OP(GETLCT)   /* d s       the same, for the this of a derived constructor */
RV_OP(PUTLC)    /* d s       the local d, which must be initialized, = s, taken */
RV_OP(PUTLCI)   /* d s       the local d, which must not be initialized, = s, taken */
RV_OP(SETUNI)   /* d         the local d = uninitialized */
RV_OP(CLOSE)    /* idx16     a new binding of the local idx: its var_ref closed */
#elif RV_PART == RV_PART_INFO
    case OP_fclosure8: case OP_fclosure:
    case OP_put_var_ref: case OP_put_var_ref0: case OP_put_var_ref1:
    case OP_put_var_ref2: case OP_put_var_ref3:
    case OP_set_var_ref: case OP_set_var_ref0: case OP_set_var_ref1:
    case OP_set_var_ref2: case OP_set_var_ref3:
    case OP_put_var_ref_check:
    case OP_get_var_ref_check:
    case OP_get_loc_check: case OP_get_loc_checkthis:
    case OP_put_loc_check: case OP_put_loc_check_init: case OP_set_loc_check:
    case OP_set_loc_uninitialized: case OP_close_loc:
        return 0;
#elif RV_PART == RV_PART_PASS_FN
_Static_assert(RV_RETCL == RV_RETCT + 1 && RV_RETCK == RV_RETCT + 2 &&
               RV_RETCU == RV_RETCT + 3, "rv_emit_ret adds the kind to RV_RETCT");

/* put_var_ref and set_var_ref idx: the top of the stack into the closure
   variable idx. A temp is moved by put and copied by set, which leaves it
   on the stack, a local or a constant is copied. Nothing is thrown: the old
   value is freed, which runs no JS. */
static void rv_put_var_ref(RVPass *s, int idx, BOOL keep)
{
    RVEnt e = s->st[s->sp - 1];
    if (e.kind == RV_E_K) {
        rv_emit(s, RV_SETVRK);
        rv_byte(s, e.k);
    } else if (e.kind == RV_E_TEMP && !keep) {
        rv_emit(s, RV_PUTVR);
        rv_byte(s, e.slot);
    } else {
        rv_emit(s, RV_SETVR);
        rv_byte(s, e.slot);
    }
    rv_u16(s, idx);
    if (!keep)
        s->sp--;
}

/* add at s->pos stored into a closure variable right after, with no jump
   to the store: put_var_ref, or dup then put_var_ref, or set_var_ref (a +=
   b on a closure variable, as React's renderToString builds its page with
   `u !== null && (a += u)`). ADDVR adds into the closure variable, so that
   a string there can grow in place as the old loop's js_add_string_store
   lets it, and when the value stays on the stack GETVR reads it back. The
   store is not read then (skip_to). put_var_ref_check (a let a closure
   writes) makes ADDVRC, which checks the variable after the add as the
   old loop does: a failed check is named at the old pc of the store, by
   the pcmap entry one before the end of ADDVRC. */
static BOOL rv_add_var_ref(RVPass *s)
{
    int q = s->pos + 1, op, idx, a, b, d, keep = FALSE;
    BOOL check = FALSE;
    if (q >= s->len || s->leader[q])
        return FALSE;
    if (s->bc[q] == OP_dup) {
        q++;
        if (q >= s->len || s->leader[q])
            return FALSE;
        keep = TRUE;
    }
    op = s->bc[q];
    if (op == OP_put_var_ref)
        idx = get_u16(s->bc + q + 1);
    else if (op >= OP_put_var_ref0 && op <= OP_put_var_ref3)
        idx = op - OP_put_var_ref0;
    else if (op == OP_put_var_ref_check)
        idx = get_u16(s->bc + q + 1), check = TRUE;
    else if (!keep && op == OP_set_var_ref)
        idx = get_u16(s->bc + q + 1), keep = TRUE;
    else if (!keep && op >= OP_set_var_ref0 && op <= OP_set_var_ref3)
        idx = op - OP_set_var_ref0, keep = TRUE;
    else
        return FALSE;
    a = rv_slot_of(s, s->sp - 2);
    b = rv_slot_of(s, s->sp - 1);
    rv_emit(s, check ? RV_ADDVRC : RV_ADDVR);
    rv_byte(s, a);
    rv_byte(s, b);
    rv_u16(s, idx);
    if (check) {
        RVMask m = rv_owned(s, 2);
        rv_pcmap_put_p(s, s->code.size - 1, rv_old_pc_at(s->bc, q), &m);
    }
    rv_throws(s, rv_owned(s, 2));
    s->sp -= 2;
    if (keep) {
        d = rv_tslot(s, s->sp);
        rv_emit(s, RV_GETVR);
        rv_byte(s, d);
        rv_u16(s, idx);
        rv_push(s, RV_E_TEMP, d, 0);
        rv_mark_dst(s, d);
    }
    s->skip_to = q + short_opcode_info(op).size;
    return TRUE;
}

/* put_loc_check and put_loc_check_init: the top of the stack into the local
   slot after its check. A constant goes into its temp first. */
static void rv_put_loc_check(RVPass *s, int op, int slot)
{
    int v;
    rv_before_store(s, slot);
    v = rv_slot_of(s, s->sp - 1);
    rv_emit(s, op);
    rv_byte(s, slot);
    rv_byte(s, v);
    rv_throws(s, rv_owned(s, 1));
    s->sp--;
}
#elif RV_PART == RV_PART_PASS
    case OP_fclosure8: case OP_fclosure:
        {
            uint32_t idx = op == OP_fclosure8 ? p[1] : get_u32(p + 1);
            if (idx > 0xffff) {
                /* past the 16 bits of FCLOS: OLD runs it (regvm-ops-old.h) */
                rv_old_op(s, p);
                break;
            }
            d = rv_tslot(s, s->sp);
            rv_emit(s, RV_FCLOS);
            rv_byte(s, d);
            rv_u16(s, idx);
            rv_throws(s, rv_owned(s, 0));
            rv_mark_dst(s, d);
            rv_push(s, RV_E_TEMP, d, 0);
        }
        break;
    case OP_put_var_ref:
        rv_put_var_ref(s, get_u16(p + 1), FALSE);
        break;
    case OP_put_var_ref0: case OP_put_var_ref1: case OP_put_var_ref2: case OP_put_var_ref3:
        rv_put_var_ref(s, op - OP_put_var_ref0, FALSE);
        break;
    case OP_set_var_ref:
        rv_put_var_ref(s, get_u16(p + 1), TRUE);
        break;
    case OP_set_var_ref0: case OP_set_var_ref1: case OP_set_var_ref2: case OP_set_var_ref3:
        rv_put_var_ref(s, op - OP_set_var_ref0, TRUE);
        break;
    case OP_put_var_ref_check:
        /* a constant goes into its temp first */
        slot = rv_slot_of(s, s->sp - 1);
        rv_emit(s, RV_PUTVRC);
        rv_byte(s, slot);
        rv_u16(s, get_u16(p + 1));
        rv_throws(s, rv_owned(s, 1));
        s->sp--;
        break;
    case OP_get_var_ref_check:
        d = rv_tslot(s, s->sp);
        rv_emit(s, RV_GETVRC);
        rv_byte(s, d);
        rv_u16(s, get_u16(p + 1));
        rv_throws(s, rv_owned(s, 0));
        rv_mark_dst(s, d);
        rv_push(s, RV_E_TEMP, d, 0);
        break;
    case OP_get_loc_check: case OP_get_loc_checkthis:
        d = rv_tslot(s, s->sp);
        rv_emit(s, op == OP_get_loc_check ? RV_GETLC : RV_GETLCT);
        rv_byte(s, d);
        rv_byte(s, rv_loc(s, get_u16(p + 1)));
        rv_throws(s, rv_owned(s, 0));
        rv_mark_dst(s, d);
        rv_push(s, RV_E_TEMP, d, 0);
        break;
    case OP_put_loc_check:
        rv_put_loc_check(s, RV_PUTLC, rv_loc(s, get_u16(p + 1)));
        break;
    case OP_put_loc_check_init:
        rv_put_loc_check(s, RV_PUTLCI, rv_loc(s, get_u16(p + 1)));
        break;
    case OP_set_loc_check:
        /* the check of the local (CHKLW, the error of the old loop), then
           set_loc: the value stays on the stack, owned there across the
           check, and no entry is added to it, as the old instruction adds
           none to stack_size. A function has it where tdz_checks of
           quickjs.c keeps the check of an assignment whose value is read
           (with -no-fused-ops it keeps all of them). */
        slot = rv_loc(s, get_u16(p + 1));
        rv_emit(s, RV_CHKLW);
        rv_u16(s, slot);
        rv_byte(s, 0);
        rv_throws(s, rv_owned(s, 0));
        rv_store(s, slot);
        rv_push(s, RV_E_SLOT, slot, 0);
        break;
    case OP_set_loc_uninitialized:
        slot = rv_loc(s, get_u16(p + 1));
        rv_before_store(s, slot);
        rv_emit(s, RV_SETUNI);
        rv_byte(s, slot);
        break;
    case OP_close_loc:
        /* only a captured local has a var_ref to close: in a function
           with none (code that JS_ReadObject read, the compiler closes
           only captured locals) there is nothing to do */
        if (!s->b->var_ref_count)
            break;
        rv_emit(s, RV_CLOSE);
        rv_u16(s, get_u16(p + 1));
        break;
#elif RV_PART == RV_PART_RUN_FN
/* the closure state of the frame F, as JS_CallInternal sets up its
   JSStackFrame: the frame is not on the list of rt->current_stack_frame,
   only the JSVarRefs made for its locals point at it */
static no_inline void rv_clinit(JSValue *F)
{
    RVFunc *rf = ((RVHeader *)F)->rf;
    JSStackFrame *sf = RV_SF(F);
    int i;

    sf->prev_frame = NULL;
    sf->cur_func = F[-rf->fofs];
    sf->arg_buf = F - rf->fofs + 1;
    sf->var_buf = F - rf->b->var_count;
    sf->var_refs = (JSVarRef **)(sf + 1);
    sf->cur_pc = NULL;
    sf->arg_count = rf->arg_count;
    sf->js_mode = rf->b->js_mode;
    sf->cur_sp = NULL;
    for (i = 0; i < rf->var_ref_count; i++)
        sf->var_refs[i] = NULL;
}

/* closes the var_refs of the frame F: each takes a copy of its local, as at
   the end of JS_CallInternal. Every exit of a frame with captured locals
   calls it before it frees the slots: the returns RETCT to RETCU and the
   unwinding, through RV_CLOSE_VAR_REFS. */
static no_inline void rv_close_frame(JSRuntime *rt, JSValue *F)
{
    close_var_refs(rt, ((RVHeader *)F)->rf->b, RV_SF(F));
}

/* fclosure, ip at d idx16: the closure takes the closure variables of the
   frame's function and the var_refs of its captured locals, made here when
   it is the first closure of a local, as OP_fclosure does */
static no_inline int rv_fclosure(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RVHeader *h = (RVHeader *)F;
    JSFunctionBytecode *b = h->rf->b;
    JSValue v;

    if (RV_INJ_HIT(ctx))
        return -1;
    v = js_closure(ctx, JS_DupValue(ctx, b->cpool[get_u16(ip + 1)]), h->var_refs,
                   h->rf->var_ref_count ? RV_SF(F) : NULL, FALSE);
    if (JS_IsException(v))
        return -1;
    rv_put(ctx, F, (int8_t)ip[0], v);
    return 0;
}

/* the error of a check that failed, ip at the operands of PUTVRC, GETVRC,
   GETLC, GETLCT, PUTLC or PUTLCI: the ReferenceError of the old loop's handler,
   and the value a put took freed. The this of a derived constructor
   (GETLCT) is thrown in the context of its caller, as the old loop throws
   it (rv_caller_ctx). */
static no_inline void rv_check_throw(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RVFunc *rf = ((RVHeader *)F)->rf;
    switch (ip[-1]) {
    case RV_GETVRC:
        JS_ThrowReferenceErrorUninitialized2(ctx, rf->b, get_u16(ip + 1), TRUE);
        break;
    case RV_PUTVRC:
        JS_ThrowReferenceErrorUninitialized2(ctx, rf->b, get_u16(ip + 1), TRUE);
        if (rv_is_temp(F, (int8_t)ip[0]))
            JS_FreeValue(ctx, F[(int8_t)ip[0]]);
        break;
    case RV_GETLC:
        JS_ThrowReferenceErrorUninitialized2(ctx, rf->b, (int8_t)ip[1] + rf->b->var_count, FALSE);
        break;
    case RV_GETLCT:
        JS_ThrowReferenceErrorUninitialized2(rv_caller_ctx(ctx, F), rf->b,
                                             (int8_t)ip[1] + rf->b->var_count, FALSE);
        break;
    case RV_PUTLC:
        JS_ThrowReferenceErrorUninitialized2(ctx, rf->b, (int8_t)ip[0] + rf->b->var_count, FALSE);
        if (rv_is_temp(F, (int8_t)ip[1]))
            JS_FreeValue(ctx, F[(int8_t)ip[1]]);
        break;
    case RV_PUTLCI:
        JS_ThrowReferenceError(ctx, "'this' can be initialized only once");
        if (rv_is_temp(F, (int8_t)ip[1]))
            JS_FreeValue(ctx, F[(int8_t)ip[1]]);
        break;
    default:
        abort();
    }
}

static no_inline void rv_close_loc(JSContext *ctx, JSValue *F, int idx)
{
    close_lexical_var(ctx, ((RVHeader *)F)->rf->b, RV_SF(F), idx);
}

/* for the unwinding (rv_exception): the var_refs of the frame F of rf
   closed, before rv_exit_scan frees its slots. rt in scope. */
#define RV_CLOSE_VAR_REFS(F, rf) do {                                   \
        if (unlikely((rf)->var_ref_count)) {                            \
            if (unlikely((rf)->vr_heap))                                \
                rv_clw_exit(rt, (F));                                   \
            else                                                        \
                rv_close_frame(rt, (F));                                \
        }                                                               \
    } while (0)

/* the slow side of ADDVR and ADDVRC, ip at a b idx16: a + b as the old
   loop's OP_add, then into the closure variable idx as put_var_ref, or
   put_var_ref_check for ADDVRC: -2 when that check fails, the sum freed.
   Two strings go through js_add_string_into, which grows the variable's
   string when it is the left operand. */
static no_inline int rv_addvr_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue ops[2], *pv, old;
    ops[0] = rv_take(ctx, F, (int8_t)ip[0]);
    ops[1] = rv_take(ctx, F, (int8_t)ip[1]);
    if (RV_INJ_HIT(ctx)) {
        JS_FreeValue(ctx, ops[0]);
        JS_FreeValue(ctx, ops[1]);
        return -1;
    }
    if (JS_IsString(ops[0]) && JS_IsString(ops[1])) {
        ops[0] = js_add_string_into(ctx, ops[0], ops[1],
                                    ((RVHeader *)F)->var_refs[get_u16(ip + 2)]->pvalue);
        if (JS_IsException(ops[0]))
            return -1;
    } else if (js_add_slow(ctx, ops + 2)) {
        /* it freed both */
        return -1;
    }
    /* read after the add, which can run JS */
    pv = ((RVHeader *)F)->var_refs[get_u16(ip + 2)]->pvalue;
    if (ip[-1] == RV_ADDVRC && unlikely(JS_IsUninitialized(*pv))) {
        JS_FreeValue(ctx, ops[0]);
        JS_ThrowReferenceErrorUninitialized2(ctx, ((RVHeader *)F)->rf->b, get_u16(ip + 2), TRUE);
        return -2;
    }
    old = *pv;
    *pv = ops[0];
    JS_FreeValue(ctx, old);
    return 0;
}
#elif RV_PART == RV_PART_RUN
 rv_CLINIT:
    rv_clinit(F);
    RV_NEXT();

    /* the returns of a frame with captured locals: the var_refs closed
       first, so that a closure keeps the value of the local, then the
       return of RV_RET to RV_RETU */
 rv_RETCT:
    rv_close_frame(rt, F);
    goto rv_RET;
 rv_RETCL:
    rv_close_frame(rt, F);
    goto rv_RETL;
 rv_RETCK:
    rv_close_frame(rt, F);
    goto rv_RETK;
 rv_RETCU:
    rv_close_frame(rt, F);
    goto rv_RETU;

 rv_FCLOS:
    ip = pc; pc += 3;
    if (rv_fclosure(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();

    /* the closure variables written, as set_value: the new value in, then
       the old one freed */
 rv_PUTVR:
    ip = pc; pc += 3;
    {
        JSValue *pv = ((RVHeader *)F)->var_refs[get_u16(ip + 1)]->pvalue, old = *pv;
        *pv = RV_R(ip[0]);
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();
 rv_SETVR:
    ip = pc; pc += 3;
    {
        JSValue *pv = ((RVHeader *)F)->var_refs[get_u16(ip + 1)]->pvalue, old = *pv;
        *pv = JS_DupValue(ctx, RV_R(ip[0]));
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();
 rv_SETVRK:
    ip = pc; pc += 3;
    {
        JSValue *pv = ((RVHeader *)F)->var_refs[get_u16(ip + 1)]->pvalue, old = *pv;
        *pv = JS_DupValue(ctx, RV_K(ip[0]));
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();

    /* add then put_var_ref: two ints with an int result here */
 rv_ADDVR:
    ip = pc; pc += 4;
    {
        JSValue a = RV_R(ip[0]), bv = RV_R(ip[1]);
        if (likely(JS_VALUE_IS_BOTH_INT(a, bv)) && !RV_INJ_ARMED) {
            int64_t r = (int64_t)JS_VALUE_GET_INT(a) + JS_VALUE_GET_INT(bv);
            if (likely((int)r == r)) {
                JSValue *pv = ((RVHeader *)F)->var_refs[get_u16(ip + 2)]->pvalue, old = *pv;
                *pv = JS_MKVAL(JS_TAG_INT, (int)r);
                JS_FreeValue(ctx, old);
                RV_NEXT();
            }
        }
        RV_SYNC_TOP();
        if (rv_addvr_slow(ctx, F, ip))
            goto rv_exception;
    }
    RV_NEXT();
    /* add then put_var_ref_check: two ints with an int result into a
       variable that is initialized here. A failed check leaves pc one
       before the end, where the pcmap names the old store. */
 rv_ADDVRC:
    ip = pc; pc += 4;
    {
        JSValue a = RV_R(ip[0]), bv = RV_R(ip[1]);
        if (likely(JS_VALUE_IS_BOTH_INT(a, bv)) && !RV_INJ_ARMED) {
            int64_t r = (int64_t)JS_VALUE_GET_INT(a) + JS_VALUE_GET_INT(bv);
            JSValue *pv = ((RVHeader *)F)->var_refs[get_u16(ip + 2)]->pvalue, old = *pv;
            if (likely((int)r == r && !JS_IsUninitialized(old))) {
                *pv = JS_MKVAL(JS_TAG_INT, (int)r);
                JS_FreeValue(ctx, old);
                RV_NEXT();
            }
        }
        RV_SYNC_TOP();
        n = rv_addvr_slow(ctx, F, ip);
        if (n) {
            if (n == -2)
                pc = ip + 3;
            goto rv_exception;
        }
    }
    RV_NEXT();

    /* the checks of let and const before their initialization */
 rv_PUTVRC:
    ip = pc; pc += 3;
    {
        JSValue *pv = ((RVHeader *)F)->var_refs[get_u16(ip + 1)]->pvalue, v, old;
        if (RV_INJ_HIT(ctx) || unlikely(JS_IsUninitialized(*pv)))
            goto rv_check_fail;
        v = rv_take(ctx, F, (int8_t)ip[0]);
        old = *pv;
        *pv = v;
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();
 rv_GETVRC:
    ip = pc; pc += 3;
    {
        JSValue v = *((RVHeader *)F)->var_refs[get_u16(ip + 1)]->pvalue;
        if (RV_INJ_HIT(ctx) || unlikely(JS_IsUninitialized(v)))
            goto rv_check_fail;
        RV_PUTD(ip[0], JS_DupValue(ctx, v));
    }
    RV_NEXT();
 rv_GETLC:
 rv_GETLCT:
    ip = pc; pc += 2;
    {
        JSValue v = RV_R(ip[1]);
        if (RV_INJ_HIT(ctx) || unlikely(JS_IsUninitialized(v)))
            goto rv_check_fail;
        RV_PUTD(ip[0], JS_DupValue(ctx, v));
    }
    RV_NEXT();
 rv_PUTLC:
    ip = pc; pc += 2;
    if (RV_INJ_HIT(ctx) || unlikely(JS_IsUninitialized(RV_R(ip[0]))))
        goto rv_check_fail;
    {
        JSValue v = rv_take(ctx, F, (int8_t)ip[1]), old = RV_R(ip[0]);
        RV_R(ip[0]) = v;
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();
 rv_PUTLCI:
    ip = pc; pc += 2;
    if (RV_INJ_HIT(ctx) || unlikely(!JS_IsUninitialized(RV_R(ip[0]))))
        goto rv_check_fail;
    {
        JSValue v = rv_take(ctx, F, (int8_t)ip[1]), old = RV_R(ip[0]);
        RV_R(ip[0]) = v;
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();
 rv_check_fail:
    rv_check_throw(ctx, F, ip);
    RV_INJ_AGAIN(ctx);
    goto rv_exception;
 rv_SETUNI:
    ip = pc; pc += 1;
    {
        JSValue old = RV_R(ip[0]);
        RV_R(ip[0]) = JS_UNINITIALIZED;
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();

 rv_CLOSE:
    ip = pc; pc += 2;
    rv_close_loc(ctx, F, get_u16(ip));
    RV_NEXT();
#endif
