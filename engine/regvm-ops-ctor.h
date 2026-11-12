/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Constructors in the register loop: call_constructor, new.target, the
   checks of a class constructor, super(...) and what a derived constructor
   returns. Read once for each part, see regvm-areas.h.

   new.target. A function that reads it (special_object NEW_TARGET,
   check_ctor, init_ctor: rv_reads_new_target) has a slot for it, the first
   local F[-var_count], below the locals of its bytecode (rf->nt). An
   ordinary call writes undefined there with the other locals, so it costs
   nothing; a construct call writes new.target. A function that does not
   read it has no slot, nor does a generator or an async function, which
   is never constructed: new.target is undefined in it, and its frame on
   the heap (regvm-ops-gen.h) has the locals of its bytecode only.

   A construct call stays in the loop (NEW, rv_new_frame) when the
   constructor has register code and the call fits as an in-loop call would.
   The window is func, new.target and the arguments at w. The callee's frame
   starts one slot higher, at w + 1: func moves there, the arguments are in
   place, new.target goes into its slot, and the object made from
   new.target's prototype (js_create_from_ctor, undefined for a derived
   constructor) into the callee's this. The caller keeps a reference to that
   object in w, and the callee returns into w + 1, where CRES, the
   instruction after NEW, applies the rule of JS_CallConstructorInternal: an
   object returned replaces the one made, anything else is freed. The pcmap
   entry of NEW owns w, so an exception out of the callee frees the object
   in the caller's unwinding. Ordinary calls and returns are unchanged: the
   construct is the caller's business, not the frame's.

   Everything else goes out of the loop as the old loop calls it
   (rv_new_slow, JS_CallConstructorInternal): a constructor without register
   code, a C constructor, a bound function, a proxy, another realm, more
   arguments than the constructor has, no room or no depth left. From C
   (Reflect.construct, a bound function, super(...) of a proxy or a bound
   parent), JS_CallConstructorInternal runs a constructor with register code
   in JS_RegvmRun with new.target given to its slot (rv_entry_fast,
   regvm-func.h).

   The this of a function: a sloppy one coerces it at its push_this
   (THISC), as the old loop does: undefined and null become the global
   object of its realm, a primitive its wrapper object. */

#if RV_PART == RV_PART_LIST
RV_OP(NEW)      /* w argc: func, new.target and the arguments at w, CRES w follows */
RV_OP(CRES)     /* w: the result of a construct, w + 1 returned, w the object made */
RV_OP(CHKCTOR)  /* s: throws when the slot of new.target s is undefined */
RV_OP(INITCTOR) /* d s: d = the parent constructed with new.target s (init_ctor) */
RV_OP(CHKRET)   /* d s: d = whether the derived constructor returns this, for s */
RV_OP(THISC)    /* d: the this of a sloppy function, coerced, to d */
#elif RV_PART == RV_PART_INFO
    case OP_call_constructor:
        *npop = 2 + get_u16(bc + pos + 1);
        *npush = 1;
        return 0;
    case OP_special_object:
    case OP_check_ctor:
    case OP_init_ctor:
    case OP_check_ctor_return:
        return 0;
#elif RV_PART == RV_PART_PASS_FN
/* 1 when the code of b, a function that can be constructed, reads
   new.target: it then gets a slot for it */
static int rv_reads_new_target(JSFunctionBytecode *b)
{
    const uint8_t *bc = b->byte_code_buf;
    int pos, op;
    if (b->func_kind != JS_FUNC_NORMAL)
        return 0;
    for (pos = 0; pos < b->byte_code_len; pos += short_opcode_info(op).size) {
        op = bc[pos];
        if (op == OP_check_ctor || op == OP_init_ctor ||
            (op == OP_special_object && bc[pos + 1] == OP_SPECIAL_OBJECT_NEW_TARGET))
            return 1;
    }
    return 0;
}

/* new: func, new.target and the arguments go into their temps, the window,
   as the old stack has them, and the result into the temp of func. NEW
   owns the temp of func across it (the object made, see above). */
static void rv_new(RVPass *s, int argc)
{
    int i, n = argc + 2, base = s->sp - n, w;
    if (argc > 255) {
        /* argc is a byte of NEW */
        s->failed = TRUE;
        return;
    }
    for (i = base; i < s->sp; i++)
        rv_materialize(s, i);
    w = rv_tslot(s, base);
    rv_emit(s, RV_NEW);
    rv_byte(s, w);
    rv_byte(s, argc);
    if (argc > 255)
        s->failed = TRUE;
    {
        /* the object under construction at w is owned across it too */
        RVMask m = rv_owned(s, n);
        rv_mask_set(&m, w - s->first_temp);
        rv_throws(s, m);
    }
    rv_emit(s, RV_CRES);
    rv_byte(s, w);
    s->sp = base;
    rv_push(s, RV_E_TEMP, w, 0);
}

/* push_this of a sloppy function: its this, coerced, in a temp */
static void rv_thisc(RVPass *s)
{
    int d = rv_tslot(s, s->sp);
    rv_emit(s, RV_THISC);
    rv_byte(s, d);
    rv_throws(s, rv_owned(s, 0));
    rv_push(s, RV_E_TEMP, d, 0);
    rv_mark_dst(s, d);
}
#elif RV_PART == RV_PART_PASS
    case OP_call_constructor:
        rv_new(s, get_u16(p + 1));
        break;
    case OP_special_object:
        switch (p[1]) {
        case OP_SPECIAL_OBJECT_THIS_FUNC:
            /* the func slot, read lazily: no instruction stores to it */
            rv_push(s, RV_E_SLOT, -(s->b->arg_count + 2 + s->var_count), 0);
            break;
        case OP_SPECIAL_OBJECT_NEW_TARGET:
            /* its slot, read lazily: no instruction stores to it, or
               undefined in a generator or an async function */
            if (s->var_count == s->b->var_count)
                rv_push(s, RV_E_K, 0, rv_k(s, JS_UNDEFINED));
            else
                rv_push(s, RV_E_SLOT, -s->var_count, 0);
            break;
        case OP_SPECIAL_OBJECT_ARGUMENTS:
            rv_args_pass(s, RV_ARGS_PLAIN, 0);
            break;
        case OP_SPECIAL_OBJECT_MAPPED_ARGUMENTS:
            rv_args_pass(s, RV_ARGS_MAPPED, 0);
            break;
        default:
            /* the home object, import.meta and the var object of a
               sloppy direct eval go through OLD (regvm-ops-old.h), the
               others are refused */
            rv_old_special(s, p);
            break;
        }
        break;
    case OP_check_ctor:
        rv_emit(s, RV_CHKCTOR);
        rv_byte(s, -s->var_count);
        rv_throws(s, rv_owned(s, 0));
        break;
    case OP_init_ctor:
        /* only the default constructor of a derived class has it, which has
           no parameters: it passes on all its arguments, which are past
           arg_count, so it reads its arguments (rf->args, regvm-ops-args.h) */
        if (s->b->arg_count != 0) {
            s->failed = TRUE;
            break;
        }
        s->args = TRUE;
        d = rv_tslot(s, s->sp);
        rv_emit(s, RV_INITCTOR);
        rv_byte(s, d);
        rv_byte(s, -s->var_count);
        rv_throws(s, rv_owned(s, 0));
        rv_push(s, RV_E_TEMP, d, 0);
        break;
    case OP_check_ctor_return:
        /* the value stays, the answer goes above it */
        slot = rv_slot_of(s, s->sp - 1);
        d = rv_tslot(s, s->sp);
        rv_emit(s, RV_CHKRET);
        rv_byte(s, d);
        rv_byte(s, slot);
        rv_throws(s, rv_owned(s, 0));
        rv_push(s, RV_E_TEMP, d, 0);
        break;
#elif RV_PART == RV_PART_RUN_FN
/* new out of the loop, pc at the end of NEW w argc: as OP_call_constructor
   of the old loop, the window is its stack. The window is freed on every
   path, the result goes to w, undefined on an exception (NEW owns w). */
static no_inline int rv_new_slow(JSContext *ctx, JSValue *F, const uint8_t *pc)
{
    JSRuntime *rt = ctx->rt;
    const uint8_t *ip = pc - 2;
    JSValue *W = F + (int8_t)ip[0], ret;
    int i, n = ip[1];

    RV_COUNT(rt, JS_VALUE_GET_TAG(W[0]) == JS_TAG_OBJECT &&
             JS_VALUE_GET_OBJ(W[0])->class_id == JS_CLASS_BYTECODE_FUNCTION ? 4 : 5);
    /* a call into the register loop from the constructor starts above the
       window */
    rt->rv_top = W + 2 + n;
    if (RV_INJ_HIT(ctx))
        ret = JS_EXCEPTION;
    else
        ret = JS_CallConstructorInternal(ctx, W[0], W[1], n, W + 2, 0);
    for (i = 0; i < n + 2; i++)
        JS_FreeValue(ctx, W[i]);
    if (unlikely(JS_IsException(ret))) {
        W[0] = JS_UNDEFINED;
        return -1;
    }
    W[0] = ret;
    return 0;
}

#define RV_NEW_OUT ((JSValue *)1)

/* new in the loop, pc at the end of NEW w argc (the start of CRES): the
   frame of the constructor at w + 1, set up as RV_CALLOP sets up a call,
   with new.target in its slot and the object made in its this, or
   RV_NEW_OUT for a call the loop leaves to rv_new_slow (nothing done
   then), or NULL on an exception (the window freed, w undefined). The
   checks before the object is made are those of the in-loop call; the old
   loop makes the checks of the entry of JS_CallInternal after the object,
   whose prototype may run JS, so the interrupt is read again then. */
static no_inline JSValue *rv_new_frame(JSContext *ctx, JSValue *F, const uint8_t *pc)
{
    JSRuntime *rt = ctx->rt;
    const uint8_t *ip = pc - 2;
    JSValue *W = F + (int8_t)ip[0], *Wc = W + 1, *Fn, fv = W[0], obj, nt;
    int argc = ip[1], i;
    JSObject *p;
    JSFunctionBytecode *cb;
    RVFunc *crf;
    RVHeader *h;

    /* the throw-injection test makes the call out, where it fails */
    if (JS_VALUE_GET_TAG(fv) != JS_TAG_OBJECT || RV_INJ_ARMED)
        return RV_NEW_OUT;
    p = JS_VALUE_GET_OBJ(fv);
    if (p->class_id != JS_CLASS_BYTECODE_FUNCTION || !p->is_constructor)
        return RV_NEW_OUT;
    cb = p->u.func.function_bytecode;
    if (cb->rv == RV_UNCONVERTED)
        rv_convert(ctx, cb);
    crf = cb->rv;
    if (argc > crf->arg_count || cb->realm != ctx ||
        (uint8_t *)Wc + crf->room_b > (uint8_t *)rt->rv_end ||
        poll_flag(&rt->interrupt_flag) || js_uncatchable_pending(rt) ||
        rt->call_depth >= rt->max_call_depth)
        return RV_NEW_OUT;
    if (!cb->is_derived_class_constructor) {
        obj = js_create_from_ctor(ctx, W[1], JS_CLASS_OBJECT);
        if (JS_IsException(obj))
            goto fail;
        if (js_poll_interrupts(ctx) || js_uncatchable_pending(rt)) {
            JS_FreeValue(ctx, obj);
            goto fail;
        }
    } else {
        obj = JS_UNDEFINED;
    }
    rt->call_depth++;
    RV_COUNT_RUN(crf);
    Fn = (JSValue *)((uint8_t *)Wc + crf->fofs_b);
    nt = W[1];
    Wc[0] = fv;
    W[0] = JS_DupValue(ctx, obj);
    if (argc != crf->arg_count)
        rv_undefined(Wc + 1 + argc, crf->arg_count - argc);
    *(JSValue *)((uint8_t *)Wc + crf->this_b) = obj;
    rv_undefined(Fn - crf->var_count, crf->var_count);
    if (crf->nt)
        Fn[-crf->var_count] = nt;
    else
        JS_FreeValue(ctx, nt); /* func or the caller's new.target holds it */
    h = (RVHeader *)Fn;
    h->caller = F;
    h->retpc = pc;
    h->rf = crf;
    h->k = crf->k;
    h->dst = (int8_t)ip[0] + 1;
    h->argc = argc;
    h->var_refs = p->u.func.var_refs;
    return Fn;
 fail:
    for (i = 0; i < argc + 2; i++)
        JS_FreeValue(ctx, W[i]);
    W[0] = JS_UNDEFINED;
    return NULL;
}

static no_inline void rv_not_ctor_call(JSContext *ctx)
{
    JS_ThrowTypeError(ctx, "class constructors must be invoked with 'new'");
}

/* init_ctor, ip at d s: the parent constructed with new.target s and all
   the arguments of the frame, as OP_init_ctor. The function has no
   parameters, so they are the extra ones below func, in order
   (regvm-ops-args.h), and argc is in the header (rf->args). */
static no_inline int rv_init_ctor(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RVFunc *rf = ((RVHeader *)F)->rf;
    int argc = ((RVHeader *)F)->argc;
    JSValue nt = F[(int8_t)ip[1]], super, ret;

    if (RV_INJ_HIT(ctx))
        return -1;
    if (JS_IsUndefined(nt)) {
        rv_not_ctor_call(ctx);
        return -1;
    }
    super = JS_GetPrototype(ctx, F[-rf->fofs]);
    if (JS_IsException(super))
        return -1;
    ret = js_call_constructor2(ctx, super, nt, argc, F - rf->fofs - argc);
    JS_FreeValue(ctx, super);
    if (JS_IsException(ret))
        return -1;
    F[(int8_t)ip[0]] = ret;
    return 0;
}

static no_inline void rv_chkret_throw(JSContext *ctx, JSValue *F)
{
    JS_ThrowTypeError(rv_caller_ctx(ctx, F),
                      "derived class constructor must return an object or undefined");
}

/* THISC, ip at d: as OP_push_this of the old loop for a sloppy function,
   whose this is not an object (the handler copies an object) */
static no_inline int rv_thisc_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RVFunc *rf = ((RVHeader *)F)->rf;
    JSValue t = F[-(rf->var_count + 1)], v;
    uint32_t tag = JS_VALUE_GET_TAG(t);

    if (RV_INJ_HIT(ctx))
        return -1;
    if (tag == JS_TAG_OBJECT) {
        v = JS_DupValue(ctx, t);
    } else if (tag == JS_TAG_NULL || tag == JS_TAG_UNDEFINED) {
        v = JS_DupValue(ctx, ctx->global_obj);
    } else {
        v = JS_ToObject(ctx, t);
        if (JS_IsException(v))
            return -1;
    }
    rv_put(ctx, F, (int8_t)ip[0], v);
    return 0;
}
#elif RV_PART == RV_PART_RUN
 rv_NEW:
    poll_count(POLL_WEIGHT);
    pc += 2;
    /* the frame and pc for a backtrace made in the prototype getter or
       inside a constructor called out of the loop */
    RV_SYNC_TOP();
    W = rv_new_frame(ctx, F, pc);
    if (likely(W > RV_NEW_OUT)) {
        F = W;
        pc = ((RVHeader *)F)->rf->code;
        RV_NEXT();
    }
    if (!W)
        goto rv_exception;
    if (rv_new_slow(ctx, F, pc))
        goto rv_exception;
    pc += 2; /* past CRES */
    RV_NEXT();
 rv_CRES:
    ip = pc; pc += 1;
    {
        JSValue *Wr = F + (int8_t)ip[0];
        if (JS_VALUE_GET_TAG(Wr[1]) == JS_TAG_OBJECT) {
            JS_FreeValue(ctx, Wr[0]);
            Wr[0] = Wr[1];
        } else {
            JS_FreeValue(ctx, Wr[1]);
        }
    }
    RV_NEXT();
 rv_CHKCTOR:
    ip = pc; pc += 1;
    if (RV_INJ_HIT(ctx))
        goto rv_exception;
    if (unlikely(JS_IsUndefined(RV_R(ip[0])))) {
        rv_not_ctor_call(ctx);
        goto rv_exception;
    }
    RV_NEXT();
 rv_INITCTOR:
    ip = pc; pc += 2;
    RV_SYNC_TOP();
    if (rv_init_ctor(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();
 rv_CHKRET:
    ip = pc; pc += 2;
    if (RV_INJ_HIT(ctx))
        goto rv_exception;
    {
        JSValue v = RV_R(ip[1]);
        if (JS_VALUE_GET_TAG(v) == JS_TAG_OBJECT) {
            RV_R(ip[0]) = JS_FALSE;
        } else if (likely(JS_IsUndefined(v))) {
            RV_R(ip[0]) = JS_TRUE;
        } else {
            rv_chkret_throw(ctx, F);
            goto rv_exception;
        }
    }
    RV_NEXT();
 rv_THISC:
    ip = pc; pc += 1;
    {
        JSValue t = F[-(((RVHeader *)F)->rf->var_count + 1)];
        if (likely(JS_VALUE_GET_TAG(t) == JS_TAG_OBJECT) && !RV_INJ_ARMED)
            RV_PUTD(ip[0], JS_DupValue(ctx, t));
        else if (rv_thisc_slow(ctx, F, ip))
            goto rv_exception;
    }
    RV_NEXT();
#endif
