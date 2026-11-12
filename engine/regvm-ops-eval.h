/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* with and direct eval in the register loop. Read once for each part, see
   regvm-areas.h.

   WITH runs the with_* instructions, which look a name up in an object (the
   object of a with statement, or the var object of a function with a
   sloppy direct eval) and jump when they find it. The body is the old
   handler's (OP_BODY_with in quickjs.c) on the temps laid out as the old
   stack, as the window ops of regvm-ops-window.h run theirs, and the jump
   is the register jump to the old target, whose stack the pass knows
   (rv_depths).

   OLD (regvm-ops-old.h) runs the other instructions of with and eval:
   eval, apply_eval, special_object VAR_OBJECT, delete_var (a delete of a
   name that no scope has) and the references (make_*_ref, get_ref_value,
   put_ref_value).

   A direct eval reads the function, the locals and their var_refs of the
   running frame from rt->current_stack_frame (__JS_EvalInternal), so rv_old
   makes the frame F the running frame for the call: its closure
   state (RV_SF, regvm.h) when it has captured locals, which every local of
   a function with eval is, or a frame on the C stack when it has none
   (rv_eval_enter). That frame is marked as an entry of the register loop
   whose top frame is F and takes the place of the entry frame of the loop
   in the list of frames, so a stack trace made during the eval walks the
   frames of the loop once. Nothing is stored on a call or a return. */

#if RV_PART == RV_PART_LIST
RV_OP(WITH)     /* w op is_with atom32 off16: the with_* op on the temps under w, jump when found */
#elif RV_PART == RV_PART_INFO
    case OP_with_get_var: case OP_with_put_var: case OP_with_delete_var:
    case OP_with_make_ref: case OP_with_get_ref:
        *target = rv_old_target(pos, 5 + (int64_t)(int32_t)get_u32(bc + pos + 5));
        return 0;
#elif RV_PART == RV_PART_PASS_FN
/* with_* at pos: the stack into its temps, as the target wants it, then
   WITH with the slot of the old sp, the old opcode, is_with, the atom and
   the jump. It can throw at its end, with its operands freed by the slow
   side, and when it jumps it reads the interrupt flag at the target, as
   QuickJS's interpreter does: one before its end, with the stack of the
   target, each entry a temp, named at the pc before the target. */
static void rv_with_op(RVPass *s, const uint8_t *p, int pos)
{
    const JSOpCode *oi = &short_opcode_info(p[0]);
    int npop = oi->n_pop, npush = oi->n_push, td, i;
    int target = pos + 5 + (int32_t)get_u32(p + 5);
    RVMask tmask = { { 0, 0, 0, 0 } };

    if (npop > s->sp) {
        rv_crafted(s);
        return;
    }
    /* the depth at the target, as rv_depths has it */
    switch (p[0]) {
    case OP_with_put_var:
        td = s->sp - 2;
        break;
    case OP_with_make_ref: case OP_with_get_ref:
        td = s->sp + 1;
        break;
    default:
        td = s->sp;
        break;
    }
    rv_block_end(s, s->sp);
    rv_emit(s, RV_WITH);
    rv_byte(s, rv_tslot(s, s->sp));
    rv_byte(s, p[0]);
    rv_byte(s, p[9]);
    rv_u32(s, get_u32(p + 1));
    rv_jump_to(s, target);
    for (i = 0; i < td; i++)
        rv_mask_set(&tmask, i);
    rv_pcmap_put(s, s->code.size - 1, target - 1, tmask);
    rv_throws(s, rv_owned(s, npop));
    rv_hooks(s, RV_HOOK_OWN_POLL);
    s->sp -= npop;
    for (i = 0; i < npush; i++)
        rv_push(s, RV_E_TEMP, rv_tslot(s, s->sp), 0);
}
#elif RV_PART == RV_PART_PASS
    case OP_with_get_var: case OP_with_put_var: case OP_with_delete_var:
    case OP_with_make_ref: case OP_with_get_ref:
        rv_with_op(s, p, pos);
        break;
#elif RV_PART == RV_PART_RUN_FN
/* WITH, ip at w op is_with atom32 off16: the body of the with_* op on the
   window under the slot w. 1 when it found the name (the jump), 0 when not,
   -1 with the operands freed when it throws. A store and a missing name
   throw by the mode of the frame's function. */
static no_inline int rv_with_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSFunctionBytecode *b = ((RVHeader *)F)->rf->b;
    JSValue *sp = F + (int8_t)ip[0], *base;
    int opcode = ip[1], is_with = ip[2], taken = 0;
    JSAtom atom = get_u32(ip + 3);
    BOOL strict = (b->js_mode & JS_MODE_STRICT) != 0;

    base = sp - short_opcode_info(opcode).n_pop;
    if (RV_INJ_HIT(ctx))
        goto exception; /* the test of regvm-inject.h, before the lookup */
    OP_BODY_with(ctx, opcode, sp, atom, is_with, strict,
                 strict ? JS_PROP_THROW : 0, taken, exception)
    if (!taken)
        RV_INJ_NONE("skip"); /* no jump: no interrupt to read */
    return taken;
 exception:
    if (sp < base)
        abort(); /* the body freed more than its operands */
    while (sp > base)
        JS_FreeValue(ctx, *--sp);
    return -1;
}

/* F as the running frame of rt for a direct eval (see the top of this
   file), tsf the frame on the C stack for a function with no captured
   locals. Returns the entry frame of the loop, which rv_eval_leave puts
   back. */
static JSStackFrame *rv_eval_enter(JSContext *ctx, JSValue *F, JSStackFrame *tsf)
{
    JSRuntime *rt = ctx->rt;
    RVFunc *rf = ((RVHeader *)F)->rf;
    JSStackFrame *esf = rt->current_stack_frame, *sf;

    if (rf->var_ref_count) {
        sf = RV_SF(F);
    } else {
        sf = tsf;
        sf->cur_func = F[-rf->fofs];
        sf->arg_buf = F - rf->fofs + 1;
        sf->var_buf = F - rf->b->var_count;
        sf->var_refs = NULL;
        sf->arg_count = rf->arg_count;
    }
    sf->js_mode = rf->b->js_mode | JS_MODE_REGVM;
    sf->cur_pc = esf->cur_pc;
    sf->cur_sp = esf->cur_sp;
    sf->prev_frame = esf->prev_frame;
    rt->current_stack_frame = sf;
    return esf;
}

/* after the eval: the entry frame of the loop is the running frame again,
   and the closure state of F as rv_clinit left it */
static void rv_eval_leave(JSContext *ctx, JSValue *F, JSStackFrame *esf)
{
    JSStackFrame *sf = ctx->rt->current_stack_frame;

    sf->js_mode = ((RVHeader *)F)->rf->b->js_mode;
    sf->cur_pc = NULL;
    sf->cur_sp = NULL;
    sf->prev_frame = NULL;
    ctx->rt->current_stack_frame = esf;
}
#elif RV_PART == RV_PART_RUN
    /* it may run JS (a getter, a proxy), so the register stack is given
       to it above this frame. A jump back counts, as every jump back
       of the loop does, and it reads the interrupt flag after it jumps,
       as QuickJS's interpreter does, and names it one before its end
       (rv_with_op). */
 rv_WITH:
    ip = pc;
    pc += 9;
    RV_SYNC_TOP();
    {
        int r = rv_with_slow(ctx, F, ip);
        if (r) {
            int off;
            if (r < 0)
                goto rv_exception;
            off = (int16_t)get_u16(pc - 2);
            if (off < 0)
                poll_count(POLL_WEIGHT);
            pc += off;
            if (RV_INTERRUPTED()) {
                pc = ip + 8;
                goto rv_exception;
            }
        }
    }
    RV_NEXT();
#endif
