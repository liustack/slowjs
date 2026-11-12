/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The register loop: JS_RegvmRun and its helpers. Included by regvm.h after
   regvm-pass.h. The handlers of each op are in the area files
   (regvm-areas.h), read here as the parts RV_PART_RUN_FN (the slow sides of
   the area, at file scope) and RV_PART_RUN (the handlers, inside
   JS_RegvmRun). What stays here: the entry, the returns, rv_bad, the
   unwinding of an exception and the exit. */


/* the pcmap entry of the instruction that ends at off, by a binary search
   of the entries (sorted by end), or NULL when no instruction that can
   throw ends there */
static const RVPcMask *rv_pc_find(const RVFunc *rf, uint32_t off)
{
    int lo = 0, hi = rf->pcmap_count - 1, mid;
    while (lo <= hi) {
        mid = (lo + hi) >> 1;
        if (rf->pcmap[mid].end < off)
            lo = mid + 1;
        else if (rf->pcmap[mid].end > off)
            hi = mid - 1;
        else
            return &rf->pcmap[mid];
    }
    return NULL;
}

/* the temps owned across the instruction of the pcmap entry m of rf */
static void rv_pc_mask(const RVFunc *rf, const RVPcMask *m, RVMask *r)
{
    r->w[0] = m->mask;
    if (rf->pcmap_hi)
        memcpy(r->w + 1, rf->pcmap_hi[m - rf->pcmap].mask, sizeof(r->w) - sizeof(r->w[0]));
    else
        r->w[1] = r->w[2] = r->w[3] = 0;
}

/* the highest temp of x, -1 for none */
static int rv_mask_top(const RVMask *x)
{
    int j;
    for (j = 3; j >= 0; j--) {
        if (x->w[j])
            return 32 * j + 31 - clz32(x->w[j]);
    }
    return -1;
}

/* what a stack trace reads of the frame F of the register loop
   (JSFrameWalk in quickjs.c): its function, the pc of the old bytecode
   for its pc, which is the end of an instruction that can throw (the top
   frame saved it before a call out or an exception, a frame below is at
   its call), and the frame below with its pc, NULL under an entry frame */
static JSValue rv_frame_func(JSValue *F)
{
    return F[-((RVHeader *)F)->rf->fofs];
}

static uint32_t rv_frame_pc(JSValue *F, const uint8_t *pc)
{
    const RVFunc *rf = ((RVHeader *)F)->rf;
    const RVPcMask *m = rv_pc_find(rf, pc - rf->code);
    if (!m)
        abort(); /* a pc not saved at a call out or an exception */
    return m->old;
}

static JSValue *rv_frame_caller(JSValue *F, const uint8_t **ppc)
{
    RVHeader *h = (RVHeader *)F;
    *ppc = h->retpc;
    return h->caller;
}


/* the caller_ctx of JS_CallInternal for the frame F of the context ctx: an
   entry frame keeps the one JS_RegvmRun was given, a call inside the loop
   is made only in the realm of its caller */
static JSContext *rv_caller_ctx(JSContext *ctx, JSValue *F)
{
    RVHeader *h = (RVHeader *)F;
    return RV_DST_SLOT(h->dst) == RV_DST_ENTRY ? h->caller_ctx : ctx;
}

static force_inline BOOL rv_is_temp(JSValue *F, int slot)
{
    (void)F;
    return slot >= RV_HDR;
}

/* an operand as a value the caller owns: a temp is moved, a local or an
   argument copied with a reference, a constant copied */
static force_inline JSValue rv_take(JSContext *ctx, JSValue *F, int slot)
{
    if (rv_is_temp(F, slot))
        return F[slot];
    return JS_DupValue(ctx, F[slot]);
}

static force_inline void rv_put(JSContext *ctx, JSValue *F, int d, JSValue v)
{
    if (rv_is_temp(F, d)) {
        F[d] = v;
    } else {
        JSValue old = F[d];
        F[d] = v;
        JS_FreeValue(ctx, old);
    }
}

/* the throw flags of a store made by the frame F, for what the old loop
   passes as JS_PROP_THROW_STRICT. That flag asks is_strict_mode, which reads
   the mode of rt->current_stack_frame, and a frame of the register loop has
   no JSStackFrame of its own: the sf of the entry stands for every frame
   above it, strict or sloppy. So a store throws by the mode of the function
   that makes it, read here in the slow side, and nothing is stored on a
   call or a return. No handler passes JS_PROP_THROW_STRICT. */
static force_inline int rv_throw_flags(JSValue *F)
{
    return (((RVHeader *)F)->rf->b->js_mode & JS_MODE_STRICT) ? JS_PROP_THROW : 0;
}

/* n slots from q to undefined: straight code up to 8, as a loop that has
   no call or bound gets a poll tick on each turn
   (internal/goasm/gen/pass/poll.cpp) */
static force_inline void rv_undefined(JSValue *q, int n)
{
    switch (n) {
    case 8: q[7] = JS_UNDEFINED; /* fall through */
    case 7: q[6] = JS_UNDEFINED; /* fall through */
    case 6: q[5] = JS_UNDEFINED; /* fall through */
    case 5: q[4] = JS_UNDEFINED; /* fall through */
    case 4: q[3] = JS_UNDEFINED; /* fall through */
    case 3: q[2] = JS_UNDEFINED; /* fall through */
    case 2: q[1] = JS_UNDEFINED; /* fall through */
    case 1: q[0] = JS_UNDEFINED; /* fall through */
    case 0:
        break;
    default:
        for (; n > 0; n--)
            *q++ = JS_UNDEFINED;
        break;
    }
}

#define RV_PART RV_PART_RUN_FN
#include "regvm-areas.h"
#undef RV_PART

#define RV_FREE_SLOT(q) do {                                            \
        if (unlikely((int64_t)(q)->tag < 0))                            \
            JS_FreeValue(ctx, *(q));                                    \
    } while (0)

/* frees the n slots from q, in order: straight code up to 8, as a loop with
   a call in it gets a poll tick on each turn */
static force_inline void rv_free_slots(JSContext *ctx, JSValue *q, int n)
{
    JSValue *e = q + n;
    switch (n) {
    case 8: RV_FREE_SLOT(e - 8); /* fall through */
    case 7: RV_FREE_SLOT(e - 7); /* fall through */
    case 6: RV_FREE_SLOT(e - 6); /* fall through */
    case 5: RV_FREE_SLOT(e - 5); /* fall through */
    case 4: RV_FREE_SLOT(e - 4); /* fall through */
    case 3: RV_FREE_SLOT(e - 3); /* fall through */
    case 2: RV_FREE_SLOT(e - 2); /* fall through */
    case 1: RV_FREE_SLOT(e - 1); /* fall through */
    case 0:
        break;
    default:
        for (; q < e; q++)
            RV_FREE_SLOT(q);
        break;
    }
}

/* frees func, the arguments, this and the locals of the frame F: the slots
   from F - fofs to F */
static force_inline void rv_exit_scan(JSContext *ctx, JSValue *F, RVFunc *rf)
{
    rv_free_slots(ctx, F - rf->fofs, rf->fofs);
}

/* the extra arguments of a C entry of rf, which reads its arguments, copied
   with a reference from W after a reference to func (regvm.h): the frame
   then starts after them. NULL when they and the frame do not fit in the
   register stack. */
static no_inline JSValue *rv_entry_extra(JSContext *ctx, RVFunc *rf, JSValue *W,
                                         JSValueConst func_obj, int argc,
                                         JSValueConst *argv)
{
    JSRuntime *rt = ctx->rt;
    int i;
    if ((uint8_t *)(W + 1 + (argc - rf->arg_count)) + rf->room_b > (uint8_t *)rt->rv_end)
        return NULL;
    *W++ = JS_DupValue(ctx, func_obj);
    for (i = rf->arg_count; i < argc; i++)
        *W++ = JS_DupValue(ctx, argv[i]);
    return W;
}

/* frees the extra arguments below func of the frame F, whose dst has
   RV_DST_EXTRA, then the reference to func below them (regvm.h): after the
   frame's own slots, as the old loop's caller frees its argv after the
   call. That reference keeps rf alive while its count and place are read,
   when the frame's own func slot held the last other one. */
static no_inline void rv_free_extra(JSContext *ctx, JSValue *F)
{
    RVHeader *h = (RVHeader *)F;
    int x = h->argc - h->rf->arg_count;
    JSValue *q = F - h->rf->fofs - x, keep = q[-1];
    for (; x > 0; x--, q++)
        JS_FreeValue(ctx, *q);
    JS_FreeValue(ctx, keep);
}

/* the slow entry of a call of the loop (rv_call_args), from the frame F at
   pc: the function at W[0] with argc arguments after it and this_v, the
   caller's slot dst. A frame with more arguments than parameters starts
   after them, with func and the declared arguments moved up to it, so that
   the extra ones stay below func, and a reference to func of its own in
   the slot below them (regvm.h); with fewer, the missing ones are
   undefined. The header gets argc, which ARGS reads. Returns the callee's
   F. */
static no_inline JSValue *rv_enter_args(JSContext *ctx, JSValue *F, const uint8_t *pc,
                                        JSValue *W, int argc, JSValue this_v, int dst)
{
    JSObject *p = JS_VALUE_GET_OBJ(W[0]);
    RVFunc *rf = p->u.func.function_bytecode->rv;
    RVHeader *h;
    JSValue *Fn;
    int i;

    dst = (uint16_t)dst;
    if (argc > rf->arg_count) {
        JSValue *Wn = W + argc + 1;
        for (i = 0; i <= rf->arg_count; i++)
            Wn[i] = W[i];
        /* W[0 .. arg_count] are dead copies now: the last is the slot
           right below the extra ones */
        W[rf->arg_count] = JS_DupValue(ctx, Wn[0]);
        W = Wn;
        dst |= RV_DST_EXTRA;
    } else {
        rv_undefined(W + 1 + argc, rf->arg_count - argc);
    }
    W[1 + rf->arg_count] = this_v;
    Fn = W + rf->fofs;
    rv_undefined(Fn - rf->var_count, rf->var_count);
    h = (RVHeader *)Fn;
    h->caller = F;
    h->retpc = pc;
    h->rf = rf;
    RV_COUNT_RUN(rf);
    h->k = rf->k;
    h->dst = dst;
    h->argc = argc;
    RV_PEAK(ctx->rt, Fn + rf->fsize);
    h->var_refs = p->u.func.var_refs;
    return Fn;
}

/* the catch of a catch offset, which is the end of its CATCH
   (regvm-ops-except.h): the off16 of the jump is the last thing in it */
static inline const uint8_t *rv_catch_target(const uint8_t *end)
{
    return end + (int16_t)get_u16(end - 2);
}

/* the stack of the current exception, if it needs one, with the frame F
   at pc on top: what the old loop does at its exception label */
static void rv_backtrace(JSContext *ctx, JSStackFrame *sf, JSValue *F,
                         const uint8_t *pc)
{
    JSRuntime *rt = ctx->rt;
    if (is_backtrace_needed(ctx, rt->current_exception)) {
        sf->cur_pc = pc;
        sf->cur_sp = F;
        build_backtrace(ctx, rt->current_exception, NULL, 0, 0, 0);
    }
}

/* the unwinding of an exception thrown by the instruction of the frame F
   that ends at pc, as the exception label of JS_CallInternal does it. The
   pcmap entry of each instruction that can throw names the temps owned
   across it: they are walked from the top, each freed, until a catch
   offset (a CATCH, or the 0 of a for_of_start: its iterator is closed and
   the walk goes on). At a catch the exception goes into the temp of the
   catch offset and the walk ends at the catch's pc: nothing below it is
   freed and the frame goes on. A frame with no catch is left: its var_refs
   are closed (RV_CLOSE_VAR_REFS, regvm-ops-closure.h), its slots freed,
   and the walk goes on in its caller at its call instruction, until it
   leaves the entry frame. The temps are counted from F[temp0]. An
   uncatchable exception (an interrupt, out of memory) passes every catch
   offset and closes no iterator, but the temps are freed and the var_refs
   closed the same. It returns the pc of the catch, with its frame in
   sf->cur_sp, or NULL when the walk left the entry frame: one pointer, as
   the generator takes no struct for a result (internal/goasm/gen). Cold:
   kept out of JS_RegvmRun, so that nothing of it is live in the loop. */
static no_inline const uint8_t *rv_unwind(JSContext *ctx, JSStackFrame *sf,
                                          JSValue *F, const uint8_t *pc)
{
    JSRuntime *rt = ctx->rt;
    RVHeader *h;
    RVFunc *rf;
    const RVPcMask *m;
    RVMask mask;
    JSValue v;
    int i, pos;

    rv_backtrace(ctx, sf, F, pc);
    for (;;) {
        h = (RVHeader *)F;
        rf = h->rf;
        m = rv_pc_find(rf, pc - rf->code);
        if (!m)
            abort(); /* an instruction that throws with no pcmap entry */
        rv_pc_mask(rf, m, &mask);
        while ((i = rv_mask_top(&mask)) >= 0) {
            rv_mask_clear(&mask, i);
            v = F[rf->temp0 + i];
            if (JS_VALUE_GET_TAG(v) != JS_TAG_CATCH_OFFSET ||
                rt->current_exception_is_uncatchable) {
                JS_FreeValue(ctx, v);
                continue;
            }
            pos = JS_VALUE_GET_INT(v);
            if (pos == 0) {
                /* for_of_start: the next method below it, then the
                   iterator, closed with a throw and freed by the walk */
                JS_FreeValue(ctx, F[rf->temp0 + i - 1]);
                rv_mask_clear(&mask, i - 1);
                rt->rv_top = F + rf->fsize;
                sf->cur_pc = pc;
                sf->cur_sp = F;
                JS_IteratorClose(ctx, F[rf->temp0 + i - 2], TRUE);
                continue;
            }
            /* the jump to the catch counts a poll and reads the interrupt
               flag, as the old loop's. An interrupt here replaces the
               exception and goes on with the walk, named where the pcmap
               entry at the end of the CATCH says (regvm-ops-except.h). */
            poll_count(POLL_WEIGHT);
            if (unlikely(js_poll_interrupts(ctx))) {
                rv_backtrace(ctx, sf, F, rf->code + pos);
                continue;
            }
            F[rf->temp0 + i] = rt->current_exception;
            rt->current_exception = JS_UNINITIALIZED;
            sf->cur_sp = F;
            return rv_catch_target(rf->code + pos);
        }
        RV_CLOSE_VAR_REFS(F, rf);
        rv_exit_scan(ctx, F, rf);
        if (h->dst & RV_DST_EXTRA)
            rv_free_extra(ctx, F);
        rt->call_depth--;
        if (h->dst & RV_DST_SEG)
            rv_seg_back(rt, rv_frame_end(h->caller));
        if (RV_DST_SLOT(h->dst) == RV_DST_ENTRY) {
            return NULL;
        }
        pc = h->retpc;
        F = h->caller;
    }
}


#ifdef RV_INJECT
/* the register loop at the dispatch of pc in the frame F, for the
   throw-injection test (engine/regvm-inject.h): counts the places where the
   instruction there can throw, and when the one asked for is among them
   tells its handler to throw there (rv_inj_armed) */
static no_inline void rv_inj_new(JSContext *ctx, JSValue *F, const uint8_t *pc)
{
    static const JSValue *last_F;
    static uint32_t last_off;
    const RVFunc *rf = ((RVHeader *)F)->rf;
    uint32_t off = pc - rf->code;
    const RVPcMask *m, *f, *end = rf->pcmap + rf->pcmap_count;
    int lo = 0, hi = rf->pcmap_count - 1, mid, first, n, own;
    RVInjCount *e;

    rv_inj_just = 0;
    if (rv_inj_armed) {
        /* the instruction that was to throw ran to its end */
        const char *name = JS_RegvmOpName(rv_inj_op);
        char what[64];
        snprintf(what, sizeof(what), "miss %s", name ? name : "?");
        rv_inj_none(what);
        return;
    }
    if (rv_inj_fired)
        return;
    /* a call of a function not converted yet converts it and runs the
       call again: one run */
    if (F == last_F && off == last_off) {
        switch (rf->code[off]) {
        case RV_CALL: case RV_CALLM: case RV_CALLML:
        case RV_CALLD: case RV_CALLMD: case RV_CALLMLD:
            return;
        }
    }
    last_F = F;
    last_off = off;
    m = NULL;
    while (lo <= hi) {
        mid = (lo + hi) >> 1;
        if (rf->pcmap[mid].start < off)
            lo = mid + 1;
        else if (rf->pcmap[mid].start > off)
            hi = mid - 1;
        else {
            m = &rf->pcmap[mid];
            break;
        }
    }
    if (!m)
        return;
    /* an instruction with more than one entry (a conditional jump, a
       GOSUB): the one at its end, the last of them, says where it throws */
    for (f = m; f > rf->pcmap && f[-1].start == m->start; f--)
        ;
    while (m + 1 < end && m[1].start == m->start)
        m++;
    if (m->hooks == RV_HOOK_NONE)
        return;
    first = f == rf->pcmap || f[-1].pos != m->pos;
    e = rv_inj_count(rf->b->rv_inj_id, m->pos);
    if (first)
        e->n++;
    if (!e->site)
        e->site = ++rv_inj_sites;
    n = m->hooks == RV_HOOK_OWN_POLL || m->hooks == RV_HOOK_APART2 ? 2 : 1;
    rv_inj_seen += n;
    e->places += n;
    if (rv_inj_site) {
        /* the j-th place run at the site */
        if (e->site != rv_inj_site || rv_inj_site_j <= (long)e->places - n ||
            rv_inj_site_j > (long)e->places)
            return;
        rv_inj_armed = rv_inj_site_j - ((long)e->places - n);
    } else {
        if (rv_inj_n <= rv_inj_seen - n || rv_inj_n > rv_inj_seen)
            return;
        rv_inj_armed = rv_inj_n - (rv_inj_seen - n);
    }
    rv_inj_place = rv_inj_armed;
    rv_inj_frame = ctx->rt->current_stack_frame;
    rv_inj_op = rf->code[off];
    /* the old loop can throw the same when it is the instruction itself */
    own = m->hooks == RV_HOOK_OWN || (m->hooks == RV_HOOK_OWN_POLL && rv_inj_armed == 1);
    if (first && own)
        snprintf(rv_inj_key, sizeof(rv_inj_key), "at %u %u %u",
                 rf->b->rv_inj_id, m->pos, e->n);
    else
        snprintf(rv_inj_key, sizeof(rv_inj_key), "nokey");
}
#endif


/* the table entries of RV_RETN: a frame has two slots at least, func and
   this */
#define RV_RETN_ENTRY(k, n) [RV_RETN + (k) * (RV_RETN_MAX + 1) + (n)] = &&rv_RETN_##k##_##n,
#define RV_RETN_ROW(k)                                                  \
    [RV_RETN + (k) * (RV_RETN_MAX + 1)] = &&rv_bad,                     \
    [RV_RETN + (k) * (RV_RETN_MAX + 1) + 1] = &&rv_bad,                 \
    RV_RETN_ENTRY(k, 2) RV_RETN_ENTRY(k, 3) RV_RETN_ENTRY(k, 4)         \
    RV_RETN_ENTRY(k, 5) RV_RETN_ENTRY(k, 6) RV_RETN_ENTRY(k, 7)         \
    RV_RETN_ENTRY(k, 8)

static no_inline JSValue JS_RegvmRun(JSContext *caller_ctx, JSValueConst func_obj,
                                     JSValueConst this_obj, JSValue new_target,
                                     int argc, JSValueConst *argv)
{
    static const void * const rv_table[256] = {
#define RV_OP(name) [RV_##name] = &&rv_##name,
#define RV_PART RV_PART_LIST
#include "regvm-areas.h"
#undef RV_PART
#undef RV_OP
        RV_RETN_ROW(0) RV_RETN_ROW(1) RV_RETN_ROW(2) RV_RETN_ROW(3)
        [RV_OP_COUNT ... 255] = &&rv_bad,
    };
    JSRuntime *rt = caller_ctx->rt;
    JSContext *ctx;
    JSObject *p;
    JSFunctionBytecode *b;
    RVFunc *rf;
    RVHeader *h;
    JSStackFrame sf_s, *sf = &sf_s;
    JSValue *F, *W, *base, ret_val, this_v;
    const uint8_t *pc, *ip;
    int i, n, dst, arith;
#ifdef RV_INJECT
    int inj_outer;
#endif

#ifdef RV_INJECT
#define RV_NEXT() do {                                                  \
        if (unlikely(rv_inj_any))                                       \
            rv_inj_new(ctx, F, pc);                                     \
        goto *rv_table[(RV_COUNT(rt, 0), RV_COUNT_OP(rt, *pc), *pc++)];    \
    } while (0)
#else
#define RV_NEXT() goto *rv_table[(RV_COUNT(rt, 0), RV_COUNT_OP(rt, *pc), *pc++)]
#endif
/* what the loop has of the register stack, for a call out of it that may
   enter it again (JS_Call), and its top frame and pc, for a stack trace
   made out of it */
#define RV_SYNC_TOP() (rt->rv_top = F + ((RVHeader *)F)->rf->fsize,         \
                       sf->cur_pc = pc, sf->cur_sp = F)
#define RV_R(i) F[(int8_t)(i)]
/* the result of an op into its d: a temp is written, a local (any d below the
   header) gets its old value freed after the write, so that the pass can
   point the d of any op that writes through it at a local (rv_mark_dst).
   The operands are read before. */
#define RV_PUTD(i, v) rv_put(ctx, F, (int8_t)(i), (v))
#define RV_K(i) (((RVHeader *)F)->k[(i)])
/* the reading of the interrupt flag of a conditional jump, with pc at the
   end of its instruction and before it moves: an interrupt leaves pc where
   the pcmap names what the jump did, as the old loop reads the flag once
   it has jumped or not (rv_jump_throws in regvm-pass.h). taken: it jumps.
   own: the instruction can throw by itself, at its end. Nothing of it runs
   while no interrupt is pending. */
#define RV_JUMP_POLL(taken, own) do {                                   \
        if (RV_INJ_HIT(ctx) || unlikely(js_poll_interrupts(ctx))) {     \
            pc -= (own) + ((taken) ? 1 : 0);                            \
            goto rv_exception;                                          \
        }                                                               \
    } while (0)

    poll_count_entry(POLL_WEIGHT);
    if (unlikely(argc < 0)) {
        /* a resume of a generator or async function (regvm-ops-gen.h) */
        if (argc == RV_ARGC_RESUME)
            goto rv_gen_entry;
        /* rv_measure_frame: before the checks, as JS_CallInternal
           measures its own */
        rt->rv_frame = __shadow_remaining();
        return JS_UNDEFINED;
    }
    p = JS_VALUE_GET_OBJ(func_obj);
    b = p->u.func.function_bytecode;
    rf = b->rv;
    /* the frame does not fit in the segment of the register stack in use:
       a run on the next one (engine/regvm-entry.h) */
    if (unlikely((uint8_t *)rt->rv_top + rf->room_b > (uint8_t *)rt->rv_end))
        return rv_run_seg(caller_ctx, rf, func_obj, this_obj, new_target, argc, argv);
    if (js_call_checks(caller_ctx)) {
        JS_FreeValue(caller_ctx, new_target);
        return JS_EXCEPTION;
    }
    rt->rv_stat[2]++;
    ctx = b->realm;

    /* the entry frame: func, the arguments and this copied with a
       reference, as JS_CallInternal copies them for JS_CALL_FLAG_COPY_ARGV.
       Arguments past arg_count are copied below func only for a function
       that reads them (rf->args, then argmax is -1): for any other, argv
       keeps them alive until the call returns. new_target, which this frame
       takes, is undefined but for a construct call of a function with a slot
       for it (rv_ctor_new_target): it goes into the first local, which is
       that slot then, and is undefined like the others else (with no
       locals, the header written after it takes its place). */
    base = rt->rv_top;
    W = base;
    dst = RV_DST_ENTRY;
    n = argc;
    if (unlikely(argc > rf->argmax)) {
        n = min_int(argc, rf->arg_count);
        if (rf->args && argc > rf->arg_count) {
            W = rv_entry_extra(ctx, rf, W, func_obj, argc, argv);
            if (unlikely(!W)) {
                /* no room for them: a run on the next segment of the
                   register stack, made large enough (engine/regvm-entry.h) */
                js_call_leave(rt);
                return rv_run_seg(caller_ctx, rf, func_obj, this_obj, new_target, argc, argv);
            }
            dst = RV_DST_ENTRY | RV_DST_EXTRA;
        }
    }
    W[0] = JS_DupValue(ctx, func_obj);
    for (i = 0; i < n; i++)
        W[1 + i] = JS_DupValue(ctx, argv[i]);
    for (; i < rf->arg_count; i++)
        W[1 + i] = JS_UNDEFINED;
    W[1 + rf->arg_count] = JS_DupValue(ctx, this_obj);
    F = W + rf->fofs;
    rv_undefined(F - rf->var_count, rf->var_count);
    F[-rf->var_count] = new_target;
    h = (RVHeader *)F;
    h->caller = NULL;
    h->caller_ctx = caller_ctx;
    h->rf = rf;
    h->k = rf->k;
    h->dst = dst;
    RV_COUNT_RUN(rf);
    RV_PEAK(rt, F + rf->fsize);
    h->argc = argc;
    h->var_refs = p->u.func.var_refs;
    rt->rv_top = F + rf->fsize;

    /* a frame of the register loop: what a stack trace reads of the frames
       inside it is the top F in cur_sp and its pc in cur_pc (js_frame_next
       in quickjs.c), which the loop saves before each call out of it */
    sf->js_mode = b->js_mode | JS_MODE_REGVM;
    sf->cur_func = (JSValue)func_obj;
    sf->arg_buf = W + 1;
    sf->arg_count = rf->arg_count;
    sf->var_buf = F - b->var_count;
    sf->var_refs = NULL;
    sf->cur_pc = rf->code;
    sf->cur_sp = F;
    sf->prev_frame = rt->current_stack_frame;
    rt->current_stack_frame = sf;
#ifdef RV_INJECT
    RV_INJ_ENTER(inj_outer);
#endif

    pc = rf->code;
    RV_NEXT();

#define RV_PART RV_PART_RUN
#include "regvm-areas.h"
#undef RV_PART

    /* the returns of RV_RETN: the value as RV_RET to RV_RETU take it, then
       the n slots of the frame freed in straight code */
#define RV_RETN_VAL0 ret_val = RV_R(pc[0]);
#define RV_RETN_VAL1 ret_val = RV_R(pc[0]); RV_R(pc[0]) = JS_UNDEFINED;
#define RV_RETN_VAL2 ret_val = RV_K(pc[0]);
#define RV_RETN_VAL3 ret_val = JS_UNDEFINED;
#define RV_RETN_H(k, n)                                                 \
 rv_RETN_##k##_##n:                                                     \
    RV_RETN_VAL##k                                                      \
    rv_free_slots(ctx, F - (n), (n));                                   \
    goto rv_return_tail;
#define RV_RETN_HROW(k)                                                 \
    RV_RETN_H(k, 2) RV_RETN_H(k, 3) RV_RETN_H(k, 4) RV_RETN_H(k, 5)     \
    RV_RETN_H(k, 6) RV_RETN_H(k, 7) RV_RETN_H(k, 8)
    RV_RETN_HROW(0) RV_RETN_HROW(1) RV_RETN_HROW(2) RV_RETN_HROW(3)
#undef RV_RETN_H
#undef RV_RETN_HROW

 rv_RET:
    ret_val = RV_R(pc[0]);
    goto rv_return;
 rv_RETL:
    ret_val = RV_R(pc[0]);
    RV_R(pc[0]) = JS_UNDEFINED;
    goto rv_return;
 rv_RETK:
    ret_val = RV_K(pc[0]);
    goto rv_return;
 rv_RETU:
    ret_val = JS_UNDEFINED;
 rv_return:
    rv_exit_scan(ctx, F, ((RVHeader *)F)->rf);
 rv_return_tail:
    h = (RVHeader *)F;
    rt->call_depth--;
    /* a temp of the caller with no flag, the one test: an entry frame, a
       frame with extra arguments or that starts a segment of the register
       stack, and any other slot go out of line */
    if (unlikely(!RV_DST_FAST(h->dst)))
        goto rv_return_other;
    pc = h->retpc;
    F = h->caller;
    F[h->dst] = ret_val;
    RV_NEXT();
 rv_return_other:
    if (h->dst & RV_DST_EXTRA)
        rv_free_extra(ctx, F);
    if (h->dst & RV_DST_SEG)
        rv_seg_back(rt, rv_frame_end(h->caller));
    dst = RV_DST_SLOT(h->dst);
    if (dst == RV_DST_ENTRY)
        goto rv_done;
    pc = h->retpc;
    F = h->caller;
    if (dst == RV_DST_DROP) {
        JS_FreeValue(ctx, ret_val);
        RV_NEXT();
    }
    if (dst < 0)
        JS_FreeValue(ctx, F[dst]);
    F[dst] = ret_val;
    RV_NEXT();

 rv_bad:
    abort();

 rv_exception:
    /* pc is at the end of the instruction that threw (each handler moves
       it there first) */
    RV_INJ_NONE("real"); /* it threw by itself before the test did */
    {
        const uint8_t *to = rv_unwind(ctx, sf, F, pc);
        if (unlikely(!to)) {
            ret_val = JS_EXCEPTION;
            goto rv_done;
        }
        F = sf->cur_sp;
        pc = to;
    }
    RV_NEXT();
 rv_done:
#ifdef RV_INJECT
    RV_INJ_LEAVE(inj_outer);
#endif
    rt->current_stack_frame = sf->prev_frame;
    rt->rv_top = base;
    /* the outermost run, after a step onto a next segment: the stack
       settles (engine/regvm-entry.h) */
    if (unlikely(rt->rv_hw) && base == rt->rv_base)
        rv_settle(rt);
    return ret_val;
#undef RV_NEXT
#undef RV_SYNC_TOP
#undef RV_R
#undef RV_PUTD
#undef RV_K
#undef RV_JUMP_POLL
}

/* rt->rv_frame: the stack left here less what JS_RegvmRun finds left at
   its entry, called from here, as js_measure_call_frame measures
   JS_CallInternal. argc is read from memory so that the call is made. */
static no_inline void rv_measure_frame(JSContext *ctx)
{
    JSRuntime *rt = ctx->rt;
    long left = __shadow_remaining();
    volatile int argc = -1;

    JS_RegvmRun(ctx, JS_UNDEFINED, JS_UNDEFINED, JS_UNDEFINED, argc, NULL);
    rt->rv_frame = left - rt->rv_frame;
}
