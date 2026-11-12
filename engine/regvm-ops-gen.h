/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Generators and async functions in the register loop. Read once for each
   part, see regvm-areas.h.

   A suspended generator or async function lives in its JSAsyncFunctionState
   (quickjs.c), the heap frame async_func_init makes, in the old loop's
   layout: arg_buf, var_buf, stack_buf up to cur_sp, var_refs. A resume of a
   function with register code (rv_gen_resume, from async_func_resume) moves
   that state onto the register stack as the entry frame of a run of
   JS_RegvmRun and goes on at the saved pc, or unwinds from it when
   throw_flag is set. A suspension (SUSP) moves the frame back and leaves
   the run. The callers (js_generator_next, the async resolve functions, the
   async generator queue), the GC and the finalizers read the heap frame as
   they do for the old loop: the yielded value at cur_sp[-1], FUNC_RET_* as
   the result.

   A generator frame is always the entry frame of its run: its body is only
   reached through async_func_resume, as the call ops enter register code
   only for JS_CLASS_BYTECODE_FUNCTION. So argv of that run is the state.

   The open var_refs of the frame belong to its closure state RV_SF(F)
   while it runs and to s->frame while it is suspended, where each holds a
   reference to the state as get_var_ref makes it for an async frame. A
   frame whose var_refs are on the heap (rf->vr_heap, regvm-ops-wide.h)
   has a table of them only while it runs: CLW 0 makes it at the first
   resume, a later resume makes it before anything of the state moves (out
   of memory leaves the state as it was), and a suspension frees it once
   the var_refs are back in s->frame.

   A function whose stack is too deep for the temps folds it (regvm-fold.h):
   its heap frame keeps the temps as they are at the suspension, each
   folded range one Array, and SUSP gives their count (rv_suspend). The
   top entry is never folded, so the value a resume sends goes where the
   code reads it. */

#if RV_PART == RV_PART_LIST
RV_OP(SUSP)     /* kind n: suspend with FUNC_RET_<kind>, the stack in T[0 .. n) */
RV_OP(W_GEN)    /* w sub flags: an iterator op of yield* and for await on the window at w */
#elif RV_PART == RV_PART_INFO
    case OP_initial_yield: case OP_yield: case OP_yield_star:
    case OP_async_yield_star: case OP_await:
        return 0;
    case OP_iterator_next: case OP_iterator_call: case OP_iterator_check_object:
    case OP_iterator_get_value_done: case OP_for_await_of_start:
    case OP_for_await_of_next:
        return 0;
#elif RV_PART == RV_PART_PASS_FN
/* a suspension: every entry into its temp, so that T[0 .. n) is the stack
   the heap frame keeps, then SUSP kind n. n is the temp the next push
   would take: in a function that folds (regvm-fold.h) the pass keeps the
   temps in use T[0 .. n), each folded range one Array, with the top entry
   never folded, whatever the layout (that of sp or, as the yield read the
   top, of sp - 1, which rv_block_end would not take). So the top is in
   T[n - 1], where the callers of the resume put the value sent
   (cur_sp[-1]), and the mode goes in T[n] (cur_sp[0]). A throw() resumes
   at its end with every temp owned (the yielded one is undefined then). */
static void rv_suspend(RVPass *s, int kind, int npop, int npush)
{
    int i, n;
    for (i = 0; i < s->sp; i++)
        rv_materialize(s, i);
    n = rv_tslot(s, s->sp) - s->first_temp;
    rv_emit(s, RV_SUSP);
    rv_byte(s, kind);
    rv_byte(s, n);
    rv_throws(s, rv_owned(s, 0));
    s->sp -= npop;
    while (npush-- > 0)
        rv_push(s, RV_E_TEMP, rv_tslot(s, s->sp), 0);
}

/* the sub-ops of W_GEN */
enum {
    RV_G_ITERATOR_NEXT,
    RV_G_ITERATOR_CALL,
    RV_G_ITERATOR_CHECK_OBJECT,
    RV_G_ITERATOR_GET_VALUE_DONE,
    RV_G_FOR_AWAIT_OF_START,
    RV_G_FOR_AWAIT_OF_NEXT,
};

/* an iterator op on the window of the old stack, as P5's window ops
   (regvm-ops-window.h): the top nread entries into their temps, then
   W_GEN w sub flags. Each leaves its operands on the stack when it throws,
   so every temp is owned across it. */
static void rv_w_gen_emit(RVPass *s, int sub, int flags, int nread, int npop, int npush)
{
    int i;
    if (nread > s->sp) {
        s->failed = TRUE;
        return;
    }
    for (i = s->sp - nread; i < s->sp; i++)
        rv_materialize(s, i);
    rv_emit(s, RV_W_GEN);
    rv_byte(s, rv_tslot(s, s->sp));
    rv_byte(s, sub);
    rv_byte(s, flags);
    rv_throws(s, rv_owned(s, 0));
    rv_window_end(s, npop, npush);
}
#elif RV_PART == RV_PART_PASS
    case OP_initial_yield:
        rv_suspend(s, FUNC_RET_INITIAL_YIELD, 0, 0);
        break;
    case OP_yield:
        /* the sent value and the mode come back */
        rv_suspend(s, FUNC_RET_YIELD, 1, 2);
        break;
    case OP_yield_star:
    case OP_async_yield_star:
        rv_suspend(s, FUNC_RET_YIELD_STAR, 1, 2);
        break;
    case OP_await:
        rv_suspend(s, FUNC_RET_AWAIT, 1, 1);
        break;
    /* iter next catch_offset val: each reads up to four entries */
    case OP_iterator_next:
        rv_w_gen_emit(s, RV_G_ITERATOR_NEXT, 0, 4, 4, 4);
        break;
    case OP_iterator_call:
        rv_w_gen_emit(s, RV_G_ITERATOR_CALL, p[1], 4, 4, 5);
        break;
    case OP_iterator_check_object:
        rv_w_gen_emit(s, RV_G_ITERATOR_CHECK_OBJECT, 0, 1, 1, 1);
        break;
    case OP_iterator_get_value_done:
        /* the iterator four below the top is dropped when done */
        rv_w_gen_emit(s, RV_G_ITERATOR_GET_VALUE_DONE, 0, 4, 2, 3);
        break;
    case OP_for_await_of_start:
        rv_w_gen_emit(s, RV_G_FOR_AWAIT_OF_START, 0, 1, 1, 3);
        break;
    case OP_for_await_of_next:
        rv_w_gen_emit(s, RV_G_FOR_AWAIT_OF_NEXT, 0, 3, 3, 4);
        break;
#elif RV_PART == RV_PART_RUN_FN
#define RV_ARGC_RESUME (-2) /* the argc of JS_RegvmRun for a resume, argv the state */

static no_inline void rv_free_extra(JSContext *ctx, JSValue *F); /* regvm-run.h */

/* p, a pointer of a var_ref into an argument or a local of the heap frame
   hs, as a pointer into the same slot of the frame F, or back with to_heap */
static JSValue *rv_gen_rebase(JSStackFrame *hs, JSValue *F, JSValue *p, BOOL to_heap)
{
    RVFunc *rf = ((RVHeader *)F)->rf;
    JSValue *fa = F - rf->fofs + 1, *fv = F - rf->var_count;
    if (to_heap) {
        if (p >= fa && p < fa + rf->arg_count)
            return hs->arg_buf + (p - fa);
        assert(p >= fv && p < fv + rf->var_count);
        return hs->var_buf + (p - fv);
    }
    if (p >= hs->arg_buf && p < hs->arg_buf + rf->arg_count)
        return fa + (p - hs->arg_buf);
    assert(p >= hs->var_buf && p < hs->var_buf + rf->var_count);
    return fv + (p - hs->var_buf);
}

/* the arguments past arg_count that the first resume of s puts below func
   (frame-args.md): those of a function that reads its arguments (rf->args)
   and has more than its parameters, 0 for any other resume. The arguments
   object and rest are made before the first suspension (the prologue comes
   before initial_yield, and an async function has none), so only that run
   reads them. */
static int rv_gen_nextra(JSAsyncFunctionState *s, JSFunctionBytecode *b, RVFunc *rf)
{
    if (rf->args && s->frame.cur_pc == b->byte_code_buf && s->argc > rf->arg_count)
        return s->argc - rf->arg_count;
    return 0;
}

/* the slots the frame of s takes on the register stack, with the extras of
   its first resume and the reference to func below them */
static uintptr_t rv_gen_need(JSAsyncFunctionState *s, JSFunctionBytecode *b, RVFunc *rf)
{
    int x = rv_gen_nextra(s, b, rf);
    return (x ? x + 1 : 0) + rf->room_b / sizeof(JSValue);
}

/* the heap frame of s moved to the frame F on the register stack (its
   slots and the header), and the pc to go on at. With x extras, the slots
   below func get a reference to func, then the extras, each copied with a
   reference (the heap frame keeps its own until the state completes).
   NULL with nothing moved when the table of var_refs on the heap cannot be
   made. */
static no_inline const uint8_t *rv_gen_enter(JSContext *ctx, JSAsyncFunctionState *s,
                                             JSValue *F, int x)
{
    JSStackFrame *hs = &s->frame;
    JSObject *p = JS_VALUE_GET_OBJ(hs->cur_func);
    JSFunctionBytecode *b = p->u.func.function_bytecode;
    RVFunc *rf = b->rv;
    RVHeader *h = (RVHeader *)F;
    JSValue *W = F - rf->fofs, *T = F + rf->temp0, *stack_buf = hs->var_buf + rf->var_count;
    BOOL first = hs->cur_pc == b->byte_code_buf;
    JSVarRef *v, **tab = NULL;
    int i, n;

    if (rf->vr_heap && !first) {
        tab = js_mallocz(ctx, sizeof(tab[0]) * rf->var_ref_count);
        if (!tab)
            return NULL;
    }
    if (x) {
        W[-x - 1] = JS_DupValue(ctx, hs->cur_func);
        for (i = 0; i < x; i++)
            W[i - x] = JS_DupValue(ctx, hs->arg_buf[rf->arg_count + i]);
    }
    W[0] = JS_DupValue(ctx, hs->cur_func);
    for (i = 0; i < rf->arg_count; i++)
        W[1 + i] = hs->arg_buf[i];
    W[1 + rf->arg_count] = JS_DupValue(ctx, s->this_val);
    for (i = 0; i < rf->var_count; i++)
        F[i - rf->var_count] = hs->var_buf[i];
    n = hs->cur_sp - stack_buf;
    for (i = 0; i < n; i++)
        T[i] = stack_buf[i];
    h->caller = NULL;
    h->retpc = NULL;
    h->rf = rf;
    h->k = rf->k;
    h->dst = x ? RV_DST_ENTRY | RV_DST_EXTRA : RV_DST_ENTRY;
    RV_COUNT_RUN(rf);
    h->argc = s->argc;
    RV_PEAK(ctx->rt, F + rf->fsize);
    h->var_refs = p->u.func.var_refs;
    if (rf->vr_heap) {
        /* the closure state as CLW 0 makes it, which the first resume
           runs: no var_ref is open before it */
        if (first)
            goto done;
        RV_SF(F)->prev_frame = NULL;
        RV_SF(F)->cur_func = F[-rf->fofs];
        RV_SF(F)->arg_buf = F - rf->fofs + 1;
        RV_SF(F)->var_buf = F - b->var_count;
        RV_SF(F)->var_refs = tab;
        RV_SF(F)->cur_pc = NULL;
        RV_SF(F)->arg_count = rf->arg_count;
        RV_SF(F)->js_mode = b->js_mode;
        RV_SF(F)->cur_sp = NULL;
    } else if (rf->var_ref_count) {
        rv_clinit(F);
    }
    if (rf->var_ref_count) {
        for (i = 0; i < rf->var_ref_count; i++) {
            v = hs->var_refs[i];
            if (!v)
                continue;
            hs->var_refs[i] = NULL;
            RV_SF(F)->var_refs[i] = v;
            v->stack_frame = RV_SF(F);
            v->pvalue = rv_gen_rebase(hs, F, v->pvalue, FALSE);
            /* the reference an open var_ref of an async frame holds: the
               caller of the resume holds another */
            assert(js_rc(s)->ref_count > 1);
            js_rc(s)->ref_count--;
        }
    }
 done:
    hs->cur_sp = NULL; /* running, as the old loop marks it */
    /* a state that never ran starts at the code's start */
    return first ? rf->code : hs->cur_pc;
}

/* the frame F, an entry frame on top, moved to the heap frame of s with
   the n temps of the old stack, to go on at pc */
static no_inline void rv_gen_suspend(JSContext *ctx, JSAsyncFunctionState *s,
                                     JSValue *F, int n, const uint8_t *pc)
{
    JSStackFrame *hs = &s->frame;
    RVFunc *rf = ((RVHeader *)F)->rf;
    JSValue *W = F - rf->fofs, *T = F + rf->temp0, *stack_buf = hs->var_buf + rf->var_count;
    JSVarRef *v;
    int i;

    /* the extras of the first resume were read by then: a later resume has
       none (rv_gen_nextra) */
    if (((RVHeader *)F)->dst & RV_DST_EXTRA) {
        rv_free_extra(ctx, F);
        ((RVHeader *)F)->dst = RV_DST_ENTRY;
    }
    for (i = 0; i < rf->arg_count; i++)
        hs->arg_buf[i] = W[1 + i];
    for (i = 0; i < rf->var_count; i++)
        hs->var_buf[i] = F[i - rf->var_count];
    for (i = 0; i < n; i++)
        stack_buf[i] = T[i];
    hs->cur_sp = stack_buf + n;
    hs->cur_pc = pc;
    for (i = 0; i < rf->var_ref_count; i++) {
        v = RV_SF(F)->var_refs[i];
        if (!v)
            continue;
        hs->var_refs[i] = v;
        v->stack_frame = hs;
        v->pvalue = rv_gen_rebase(hs, F, v->pvalue, TRUE);
        js_rc(s)->ref_count++;
    }
    if (rf->vr_heap) {
        /* the table of a frame with var_refs on the heap goes with the run */
        js_free(ctx, RV_SF(F)->var_refs);
        RV_SF(F)->var_refs = NULL;
    }
    /* the state keeps its own references to func and this */
    JS_FreeValue(ctx, W[0]);
    JS_FreeValue(ctx, W[1 + rf->arg_count]);
}

/* W_GEN, ip at w sub flags: the old handler of the iterator op on the
   window whose sp is the slot w, as JS_CallInternal runs it (quickjs.c,
   OP_iterator_next to OP_for_await_of_next), -1 when it throws. The stack
   it leaves is what the pass counts (rv_w_gen_emit). The throw-injection
   test (engine/regvm-inject.h) fails each at its first call into the
   engine, which leaves the operands as that call does when it fails. */
static no_inline int rv_w_gen(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue *sp = F + (int8_t)ip[0], method, ret;
    BOOL ret_flag;
    int flags = ip[2];

    switch (ip[1]) {
    case RV_G_ITERATOR_NEXT:
        /* iter next catch_offset val */
        if (RV_INJ_HIT(ctx))
            return -1;
        ret = js_call(ctx, sp[-3], sp[-4], 1, (JSValueConst *)(sp - 1));
        if (JS_IsException(ret))
            return -1;
        JS_FreeValue(ctx, sp[-1]);
        sp[-1] = ret;
        return 0;
    case RV_G_ITERATOR_CALL:
        if (RV_INJ_HIT(ctx))
            return -1;
        method = JS_GetProperty(ctx, sp[-4], (flags & 1) ?
                                JS_ATOM_throw : JS_ATOM_return);
        if (JS_IsException(method))
            return -1;
        if (JS_IsUndefined(method) || JS_IsNull(method)) {
            ret_flag = TRUE;
        } else {
            if (flags & 2) /* no argument */
                ret = JS_CallFree(ctx, method, sp[-4], 0, NULL);
            else
                ret = JS_CallFree(ctx, method, sp[-4], 1, (JSValueConst *)(sp - 1));
            if (JS_IsException(ret))
                return -1;
            JS_FreeValue(ctx, sp[-1]);
            sp[-1] = ret;
            ret_flag = FALSE;
        }
        sp[0] = JS_NewBool(ctx, ret_flag);
        return 0;
    case RV_G_ITERATOR_CHECK_OBJECT:
        if (likely(JS_IsObject(sp[-1]))) {
            RV_INJ_NONE("skip"); /* an object has no check to fail */
            return 0;
        }
        if (!RV_INJ_HIT(ctx))
            JS_ThrowTypeError(ctx, "iterator must return an object");
        return -1;
    case RV_G_ITERATOR_GET_VALUE_DONE:
        if (RV_INJ_HIT(ctx))
            return -1;
        return js_iterator_get_value_done(ctx, sp);
    case RV_G_FOR_AWAIT_OF_START:
        if (RV_INJ_HIT(ctx))
            return -1;
        if (js_for_of_start(ctx, sp, TRUE))
            return -1;
        sp[1] = JS_NewCatchOffset(ctx, 0);
        return 0;
    case RV_G_FOR_AWAIT_OF_NEXT:
        if (RV_INJ_HIT(ctx))
            return -1;
        return js_for_await_of_next(ctx, sp);
    default:
        abort();
    }
}


/* the end of a state the register loop could not run: its var_refs
   closed, its frame freed */
static void rv_gen_fail(JSRuntime *rt, JSAsyncFunctionState *s)
{
    s->is_completed = TRUE;
    close_var_refs(rt, JS_VALUE_GET_OBJ(s->frame.cur_func)->u.func.function_bytecode,
                   &s->frame);
    async_func_free_frame(rt, s);
}

/* async_func_resume: FUNC_RET_* at a suspension, or the value of the
   completion, or an exception (the state is then completed) */
static JSValue rv_gen_resume(JSContext *ctx, JSAsyncFunctionState *s)
{
    JSRuntime *rt = ctx->rt;
    JSStackFrame *hs = &s->frame;
    JSFunctionBytecode *b = JS_VALUE_GET_OBJ(hs->cur_func)->u.func.function_bytecode;
    RVFunc *rf = b->rv;
    JSValue ret;
    uintptr_t need;
    int i;

    /* the body converts at its first resume */
    if (unlikely(rf == RV_UNCONVERTED)) {
        rf = rv_convert(ctx, b);
        if (rf == RV_UNCONVERTED) {
            /* out of memory: the state ends with the error, and another
               call of the function tries the conversion again */
            rt->rv_oom_b = NULL;
            JS_ThrowOutOfMemory(ctx);
            rv_gen_fail(rt, s);
            return JS_EXCEPTION;
        }
    }
    if (unlikely(rf->arg_count < 0)) {
        rv_cannot_run(ctx, b, RV_NO_REFUSED);
        rv_gen_fail(rt, s);
        return JS_EXCEPTION;
    }
    need = rv_gen_need(s, b, rf);
    if (unlikely(rt->rv_top + need > rt->rv_end)) {
        /* no room in the segment in use: a run on the next one, as
           rv_run_seg runs a call from C (engine/regvm-entry.h) */
        JSValue *top = rt->rv_top;
        if (!rv_seg_next(rt, need)) {
            JS_ThrowOutOfMemory(ctx);
            rv_gen_fail(rt, s);
            return JS_EXCEPTION;
        }
        ret = JS_RegvmRun(ctx, hs->cur_func, s->this_val, JS_UNDEFINED, RV_ARGC_RESUME,
                          (JSValueConst *)s);
        rv_seg_back(rt, top);
        if (rt->rv_hw && top == rt->rv_base)
            rv_settle(rt);
    } else {
        ret = JS_RegvmRun(ctx, hs->cur_func, s->this_val, JS_UNDEFINED, RV_ARGC_RESUME,
                          (JSValueConst *)s);
    }
    if (hs->cur_sp) {
        /* suspended, or stopped by the checks of the entry with the state
           as it was */
        if (unlikely(JS_IsException(ret)))
            rv_gen_fail(rt, s);
        return ret;
    }
    /* the frame left the loop: it closed its var_refs and freed its slots.
       What the heap frame still owns: the arguments past arg_count, func
       and this. */
    s->is_completed = TRUE;
    for (i = rf->arg_count; i < hs->arg_count; i++)
        JS_FreeValue(ctx, hs->arg_buf[i]);
    JS_FreeValue(ctx, hs->cur_func);
    JS_FreeValue(ctx, s->this_val);
    return ret;
}
#elif RV_PART == RV_PART_RUN
    /* a suspension: the frame goes to the heap frame of the state, the run
       ends with FUNC_RET_<kind>, as the old loop's done_generator */
 rv_SUSP:
    ip = pc;
    pc += 2;
    /* the throw-injection test fails it in place of the suspension, with
       the frame as a throw() resumes it */
    if (RV_INJ_HIT(ctx))
        goto rv_exception;
    rv_gen_suspend(ctx, (JSAsyncFunctionState *)argv, F, ip[1], pc);
    rt->call_depth--;
    ret_val = JS_NewInt32(ctx, ip[0]);
    goto rv_done;

    /* the iterator ops: they may run JS, so the register stack above this
       frame is given to them, and pc is at the end for the pcmap */
 rv_W_GEN:
    ip = pc;
    pc += 3;
    RV_SYNC_TOP();
    if (rv_w_gen(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();

    /* a resume (rv_gen_resume): the state's frame onto the register stack
       as an entry frame, then on at its pc or into the unwinding there */
 rv_gen_entry:
    if (js_call_checks(caller_ctx))
        return JS_EXCEPTION;
    rt->rv_stat[2]++;
    {
        JSAsyncFunctionState *gs = (JSAsyncFunctionState *)argv;
        p = JS_VALUE_GET_OBJ(gs->frame.cur_func);
        b = p->u.func.function_bytecode;
        rf = b->rv;
        ctx = b->realm;
        base = rt->rv_top;
        n = rv_gen_nextra(gs, b, rf);
        W = base + (n ? n + 1 : 0);
        F = W + rf->fofs;
        pc = rv_gen_enter(ctx, gs, F, n);
        if (unlikely(!pc)) {
            /* no table for the var_refs: the state stays as it was */
            js_call_leave(rt);
            return JS_EXCEPTION;
        }
        rt->rv_top = F + rf->fsize;
        sf->js_mode = b->js_mode | JS_MODE_REGVM;
        sf->cur_func = (JSValue)func_obj;
        sf->arg_buf = W + 1;
        sf->arg_count = rf->arg_count;
        sf->var_buf = F - rf->var_count;
        sf->var_refs = NULL;
        sf->cur_pc = pc;
        sf->cur_sp = F;
        sf->prev_frame = rt->current_stack_frame;
        rt->current_stack_frame = sf;
#ifdef RV_INJECT
        /* the injection of the run that resumed it waits, as at the entry
           of a call (rv_done gives it back) */
        RV_INJ_ENTER(inj_outer);
#endif
        if (gs->throw_flag)
            goto rv_exception;
    }
    RV_NEXT();
#endif
