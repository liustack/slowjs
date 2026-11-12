/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The arguments of a frame in the register loop: the arguments object
   (special_object ARGUMENTS and MAPPED_ARGUMENTS, whose pass case is in
   regvm-ops-ctor.h with the other special objects) and rest. Read once for
   each part, see regvm-areas.h.

   A function whose code reads its arguments has rf->args set and
   rf->argmax -1, so that every call of it takes the slow entry of the call
   op (rv_enter_args), which stores the real count in the header. A frame
   called with more arguments than arg_count keeps the extra ones right
   below its func slot (regvm.h): argument i is F[-fofs + 1 + i] below
   arg_count, the extra ones follow from F[-fofs - (argc - arg_count)]. */

#if RV_PART == RV_PART_LIST
RV_OP(ARGS)     /* d kind first16: d = the arguments object (kind 0), the
                   mapped one (1), or the array of the arguments from first
                   (2, rest) */
#elif RV_PART == RV_PART_INFO
    case OP_rest:
        return 0;
#elif RV_PART == RV_PART_PASS_FN
#define RV_ARGS_PLAIN 0
#define RV_ARGS_MAPPED 1
#define RV_ARGS_REST 2

/* ARGS d kind first into the temp of the top of the stack. The function
   then reads its arguments: rf->args. */
static void rv_args_pass(RVPass *s, int kind, int first)
{
    int d = rv_tslot(s, s->sp);
    /* the mapped arguments are var refs into the frame's closure state:
       the compiler marks every argument of the function captured, so a
       function with arguments has one */
    if (kind == RV_ARGS_MAPPED && s->b->arg_count && !s->b->var_ref_count) {
        s->failed = TRUE;
        s->crafted = TRUE;
        return;
    }
    rv_emit(s, RV_ARGS);
    rv_byte(s, d);
    rv_byte(s, kind);
    rv_u16(s, first);
    rv_throws(s, rv_owned(s, 0));
    rv_push(s, RV_E_TEMP, d, 0);
    rv_mark_dst(s, d);
    s->args = TRUE;
}
#elif RV_PART == RV_PART_PASS
    case OP_rest:
        rv_args_pass(s, RV_ARGS_REST, get_u16(p + 1));
        break;
#elif RV_PART == RV_PART_RUN_FN
/* argument i of the frame F called with argc arguments */
static inline JSValueConst rv_arg_at(JSValue *F, const RVFunc *rf, int argc, int i)
{
    if (i < rf->arg_count)
        return F[-rf->fofs + 1 + i];
    return F[-rf->fofs - (argc - rf->arg_count) + (i - rf->arg_count)];
}

/* the arguments object of the frame F, as js_build_arguments makes it from
   the argv of JS_CallInternal */
static JSValue rv_build_arguments(JSContext *ctx, JSValue *F, const RVFunc *rf, int argc)
{
    JSValue val, *tab;
    JSProperty props[3];
    JSObject *p;
    int i;

    props[0].u.value = JS_NewInt32(ctx, argc); /* length */
    props[1].u.value = JS_DupValue(ctx, ctx->array_proto_values); /* Symbol.iterator */
    props[2].u.getset.getter = JS_VALUE_GET_OBJ(JS_DupValue(ctx, ctx->throw_type_error)); /* callee */
    props[2].u.getset.setter = JS_VALUE_GET_OBJ(JS_DupValue(ctx, ctx->throw_type_error)); /* callee */
    val = JS_NewObjectFromShape(ctx, js_dup_shape(ctx->arguments_shape),
                                JS_CLASS_ARGUMENTS, props);
    if (JS_IsException(val))
        return val;
    p = JS_VALUE_GET_OBJ(val);
    tab = NULL;
    if (argc > 0) {
        tab = js_malloc(ctx, sizeof(tab[0]) * argc);
        if (!tab) {
            JS_FreeValue(ctx, val);
            return JS_EXCEPTION;
        }
        for (i = 0; i < argc; i++)
            tab[i] = JS_DupValue(ctx, rv_arg_at(F, rf, argc, i));
    }
    p->u.array.u.values = tab;
    p->u.array.count = argc;
    return val;
}

/* the mapped arguments object of the frame F, as js_build_mapped_arguments
   makes it: the arguments below arg_count are var refs into the frame,
   made on its closure state (RV_SF), the others values, and callee the
   frame's function */
static JSValue rv_build_mapped_arguments(JSContext *ctx, JSValue *F, const RVFunc *rf,
                                         int argc)
{
    JSValue val;
    JSProperty props[3];
    JSVarRef **tab, *var_ref;
    JSObject *p;
    int i, j, n = min_int(argc, rf->arg_count);

    props[0].u.value = JS_NewInt32(ctx, argc); /* length */
    props[1].u.value = JS_DupValue(ctx, ctx->array_proto_values); /* Symbol.iterator */
    props[2].u.value = JS_DupValue(ctx, F[-rf->fofs]); /* callee */
    val = JS_NewObjectFromShape(ctx, js_dup_shape(ctx->mapped_arguments_shape),
                                JS_CLASS_MAPPED_ARGUMENTS, props);
    if (JS_IsException(val))
        return val;
    p = JS_VALUE_GET_OBJ(val);
    tab = NULL;
    if (argc > 0) {
        tab = js_malloc(ctx, sizeof(tab[0]) * argc);
        if (!tab)
            goto fail;
        for (i = 0; i < n; i++) {
            var_ref = get_var_ref(ctx, RV_SF(F), i, TRUE);
            if (!var_ref)
                goto fail1;
            tab[i] = var_ref;
        }
        for (i = n; i < argc; i++) {
            var_ref = js_create_var_ref(ctx, FALSE);
            if (!var_ref) {
            fail1:
                for (j = 0; j < i; j++)
                    free_var_ref(ctx->rt, tab[j]);
                js_free(ctx, tab);
                goto fail;
            }
            var_ref->value = JS_DupValue(ctx, rv_arg_at(F, rf, argc, i));
            tab[i] = var_ref;
        }
    }
    p->u.array.u.var_refs = tab;
    p->u.array.count = argc;
    return val;
 fail:
    JS_FreeValue(ctx, val);
    return JS_EXCEPTION;
}

/* the array of rest, the arguments from first, as js_create_array makes it
   for OP_rest */
static JSValue rv_build_rest(JSContext *ctx, JSValue *F, const RVFunc *rf, int argc,
                             int first)
{
    JSValue obj;
    JSObject *p;
    int i, len;

    first = min_int(first, argc);
    len = argc - first;
    obj = JS_NewArray(ctx);
    if (JS_IsException(obj))
        return JS_EXCEPTION;
    if (len > 0) {
        p = JS_VALUE_GET_OBJ(obj);
        if (expand_fast_array(ctx, p, len) < 0) {
            JS_FreeValue(ctx, obj);
            return JS_EXCEPTION;
        }
        p->u.array.count = len;
        for (i = 0; i < len; i++)
            p->u.array.u.values[i] = JS_DupValue(ctx, rv_arg_at(F, rf, argc, first + i));
        set_value(ctx, &p->prop[0].u.value, JS_NewInt32(ctx, len));
    }
    return obj;
}

/* ARGS, ip at d kind first16. Nothing here runs JS. */
static no_inline int rv_args(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RVHeader *h = (RVHeader *)F;
    JSValue v;

    if (RV_INJ_HIT(ctx))
        return -1;
    switch (ip[1]) {
    case RV_ARGS_PLAIN:
        v = rv_build_arguments(ctx, F, h->rf, h->argc);
        break;
    case RV_ARGS_MAPPED:
        v = rv_build_mapped_arguments(ctx, F, h->rf, h->argc);
        break;
    default:
        v = rv_build_rest(ctx, F, h->rf, h->argc, get_u16(ip + 2));
        break;
    }
    if (JS_IsException(v))
        return -1;
    rv_put(ctx, F, (int8_t)ip[0], v);
    return 0;
}
#elif RV_PART == RV_PART_RUN
 rv_ARGS:
    ip = pc; pc += 4;
    if (rv_args(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();
#endif
