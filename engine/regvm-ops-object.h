/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Objects in the register loop: array elements, object literals,
   define_field, globals, in, instanceof, to_propkey and the stack
   shuffles of compound assignments. Read once for each
   part, see regvm-areas.h. */

#if RV_PART == RV_PART_LIST
/* d o key|k: the element key of o into temp d. A K: key is a constant. A
   T: o is a temp, freed after. */
RV_OP(GETEL) RV_OP(GETELT) RV_OP(GETELK) RV_OP(GETELKT)
RV_OP(GETEL3)   /* d o t: as GETEL, o and the temp t kept, t made a key */
RV_OP(PUTEL)    /* o key v: o[key] = v, a temp o freed */
RV_OP(DEFEL)    /* o i v: define o[i] = v, o and i kept */
RV_OP(OBJ)      /* d: a new object */
RV_OP(DEFF)     /* o v atom: define o.atom = v, o kept */
RV_OP(DEFFK)    /* o k atom: define o.atom = the constant k, o kept */
RV_OP(DEFM)     /* o v flags atom: define the method v, o kept */
RV_OP(REGEXP)   /* d a b: a RegExp of the pattern a and the bytecode b */
RV_OP(GETVAR) RV_OP(GETVARU)  /* d idx16: a global, U undefined if missing */
RV_OP(PUTVAR) RV_OP(PUTVARI)  /* v idx16: a global, I its initialization */
RV_OP(IN) RV_OP(INSTOF)       /* d a b */
RV_OP(INK)      /* d k b: the constant key k in b */
RV_OP(TOPK)     /* d a: a as a property key */
#elif RV_PART == RV_PART_INFO
    case OP_get_array_el: case OP_get_array_el2: case OP_get_array_el3:
    case OP_put_array_el: case OP_define_array_el:
    case OP_object: case OP_define_field: case OP_define_method: case OP_regexp:
    case OP_get_var: case OP_get_var_undef: case OP_put_var: case OP_put_var_init:
    case OP_in: case OP_instanceof: case OP_to_propkey:
    case OP_dup1: case OP_dup2: case OP_dup3: case OP_nip1: case OP_swap2:
    case OP_insert4: case OP_perm4: case OP_perm5:
    case OP_rot3l: case OP_rot4l: case OP_rot5l:
        return 0;
#elif RV_PART == RV_PART_PASS_FN
/* emit rop, then d when it is not -1, then the slots of the top n entries,
   constants put in their temps first */
static void rv_obj_op(RVPass *s, int rop, int d, int n)
{
    int i, a[3];
    for (i = 0; i < n; i++)
        a[i] = rv_slot_of(s, s->sp - n + i);
    rv_emit(s, rop);
    if (d != -1)
        rv_byte(s, d);
    for (i = 0; i < n; i++)
        rv_byte(s, a[i]);
}

/* get_array_el (el2 FALSE): o key -> value, get_array_el2: o key -> o
   value */
static void rv_getel(RVPass *s, BOOL el2)
{
    int o, d;
    RVEnt ek;
    o = rv_slot_of(s, s->sp - 2);
    ek = s->st[s->sp - 1];
    d = rv_tslot(s, s->sp - (el2 ? 1 : 2));
    if (el2)
        rv_emit(s, ek.kind == RV_E_K ? RV_GETELK : RV_GETEL);
    else if (s->st[s->sp - 2].kind == RV_E_TEMP)
        rv_emit(s, ek.kind == RV_E_K ? RV_GETELKT : RV_GETELT);
    else
        rv_emit(s, ek.kind == RV_E_K ? RV_GETELK : RV_GETEL);
    rv_byte(s, d);
    rv_byte(s, o);
    rv_byte(s, ek.kind == RV_E_K ? ek.k : ek.slot);
    rv_throws(s, rv_owned(s, el2 ? 1 : 2));
    s->sp -= el2 ? 1 : 2;
    rv_push(s, RV_E_TEMP, d, 0);
    rv_mark_dst(s, d);
}
#elif RV_PART == RV_PART_PASS
    case OP_get_array_el:
        if (rv_is_lazy_args(s, s->sp - 2))
            rv_argget(s);
        else
            rv_getel(s, FALSE);
        break;
    case OP_get_array_el2:
        rv_getel(s, TRUE);
        break;
    case OP_get_array_el3:
        /* the key is made a property key in its temp */
        rv_materialize(s, s->sp - 1);
        d = rv_tslot(s, s->sp);
        rv_obj_op(s, RV_GETEL3, d, 2);
        rv_throws(s, rv_owned(s, 0));
        rv_push(s, RV_E_TEMP, d, 0);
        break;
    case OP_put_array_el:
        rv_obj_op(s, RV_PUTEL, -1, 3);
        rv_throws(s, rv_owned(s, 3));
        s->sp -= 3;
        break;
    case OP_define_array_el:
        rv_obj_op(s, RV_DEFEL, -1, 3);
        rv_throws(s, rv_owned(s, 1));
        s->sp--;
        break;
    case OP_object:
        d = rv_tslot(s, s->sp);
        rv_emit(s, RV_OBJ);
        rv_byte(s, d);
        rv_throws(s, rv_owned(s, 0));
        rv_push(s, RV_E_TEMP, d, 0);
        rv_mark_dst(s, d);
        break;
    case OP_define_field:
        if (s->st[s->sp - 1].kind == RV_E_K) {
            /* the value a constant (DEFFK) */
            int o = rv_slot_of(s, s->sp - 2);
            rv_emit(s, RV_DEFFK);
            rv_byte(s, o);
            rv_byte(s, s->st[s->sp - 1].k);
        } else {
            rv_obj_op(s, RV_DEFF, -1, 2);
        }
        rv_u32(s, get_u32(p + 1));
        rv_throws(s, rv_owned(s, 1));
        s->sp--;
        break;
    case OP_define_method:
        rv_obj_op(s, RV_DEFM, -1, 2);
        rv_byte(s, p[5]);
        rv_u32(s, get_u32(p + 1));
        rv_throws(s, rv_owned(s, 1));
        s->sp--;
        break;
    case OP_regexp:
    case OP_in:
    case OP_instanceof:
        d = rv_tslot(s, s->sp - 2);
        if (op == OP_in && s->st[s->sp - 2].kind == RV_E_K &&
            s->st[s->sp - 1].kind != RV_E_K) {
            /* the key a constant (INK) */
            rv_emit(s, RV_INK);
            rv_byte(s, d);
            rv_byte(s, s->st[s->sp - 2].k);
            rv_byte(s, s->st[s->sp - 1].slot);
        } else {
            rv_obj_op(s, op == OP_regexp ? RV_REGEXP : op == OP_in ? RV_IN : RV_INSTOF, d, 2);
        }
        rv_throws(s, rv_owned(s, 2));
        s->sp -= 2;
        rv_push(s, RV_E_TEMP, d, 0);
        rv_mark_dst(s, d);
        break;
    case OP_get_var:
    case OP_get_var_undef:
        d = rv_tslot(s, s->sp);
        rv_emit(s, op == OP_get_var ? RV_GETVAR : RV_GETVARU);
        rv_byte(s, d);
        rv_u16(s, get_u16(p + 1));
        rv_throws(s, rv_owned(s, 0));
        rv_push(s, RV_E_TEMP, d, 0);
        rv_mark_dst(s, d);
        break;
    case OP_put_var:
    case OP_put_var_init:
        rv_obj_op(s, op == OP_put_var ? RV_PUTVAR : RV_PUTVARI, -1, 1);
        rv_u16(s, get_u16(p + 1));
        rv_throws(s, rv_owned(s, 1));
        s->sp--;
        break;
    case OP_to_propkey:
        e = s->st[s->sp - 1];
        if (e.kind == RV_E_K) {
            /* a constant that is a key already stays one */
            int tag = JS_VALUE_GET_TAG(s->k[e.k]);
            if (tag == JS_TAG_INT || tag == JS_TAG_STRING || tag == JS_TAG_SYMBOL)
                break;
        }
        d = rv_tslot(s, s->sp - 1);
        rv_obj_op(s, RV_TOPK, d, 1);
        rv_throws(s, rv_owned(s, 1));
        s->sp--;
        rv_push(s, RV_E_TEMP, d, 0);
        rv_mark_dst(s, d);
        break;
        /* the shuffles of compound assignments to fields and elements, and
           of the rest of the old stack: new entry j is old entry m[j], no
           instruction of their own */
#define RV_SHUFFLE(old_op, n_in, n_out, ...)                             \
    case old_op:                                                        \
        {                                                               \
            static const int8_t m[] = { __VA_ARGS__ };                  \
            rv_shuffle(s, n_in, m, n_out);                              \
        }                                                               \
        break;
    RV_SHUFFLE(OP_dup1, 2, 3, 0, 0, 1)            /* a b -> a a b */
    RV_SHUFFLE(OP_dup2, 2, 4, 0, 1, 0, 1)         /* a b -> a b a b */
    RV_SHUFFLE(OP_dup3, 3, 6, 0, 1, 2, 0, 1, 2)   /* a b c -> a b c a b c */
    RV_SHUFFLE(OP_nip1, 3, 2, 1, 2)               /* a b c -> b c */
    RV_SHUFFLE(OP_swap2, 4, 4, 2, 3, 0, 1)        /* a b c d -> c d a b */
    RV_SHUFFLE(OP_insert4, 4, 5, 3, 0, 1, 2, 3)   /* this obj prop a -> a this obj prop a */
    RV_SHUFFLE(OP_perm4, 4, 4, 2, 0, 1, 3)        /* obj prop a b -> a obj prop b */
    RV_SHUFFLE(OP_perm5, 5, 5, 3, 0, 1, 2, 4)     /* this obj prop a b -> a this obj prop b */
    RV_SHUFFLE(OP_rot3l, 3, 3, 1, 2, 0)           /* x a b -> a b x */
    RV_SHUFFLE(OP_rot4l, 4, 4, 1, 2, 3, 0)        /* x a b c -> a b c x */
    RV_SHUFFLE(OP_rot5l, 5, 5, 1, 2, 3, 4, 0)     /* x a b c d -> a b c d x */
#undef RV_SHUFFLE
#elif RV_PART == RV_PART_RUN_FN
/* a typed array whose elements are numbers (not BigInt): reading one
   allocates nothing, and writing one an int or a float64 calls no
   JavaScript and cannot fail (js_set_fast_array_element) */
static inline BOOL rv_ta_num(int class_id)
{
    return (unsigned)(class_id - JS_CLASS_UINT8C_ARRAY) <=
        (unsigned)(JS_CLASS_FLOAT64_ARRAY - JS_CLASS_UINT8C_ARRAY) &&
        class_id != JS_CLASS_BIG_INT64_ARRAY && class_id != JS_CLASS_BIG_UINT64_ARRAY;
}

/* the slow side of GETEL, ip at d o key|k, as the slow path of
   QuickJS's get_array_el: a key in a temp is freed, o too for the T
   forms */
static no_inline int rv_getel_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                   BOOL k, BOOL t)
{
    JSValue obj = F[(int8_t)ip[1]], key, val;
    if (k) {
        key = JS_DupValue(ctx, ((RVHeader *)F)->k[ip[2]]);
    } else if (!rv_is_temp(F, (int8_t)ip[2])) {
        /* a local or an argument is read without a reference, as obj is:
           no other code can write it while the key is made and the
           property read (a captured one is never an operand, rv_push),
           so it keeps the key alive for the whole call */
        if (RV_INJ_HIT(ctx))
            val = JS_EXCEPTION;
        else
            val = JS_GetPropertyValueConst(ctx, obj, F[(int8_t)ip[2]]);
        goto done;
    } else {
        key = F[(int8_t)ip[2]];
    }
    if (RV_INJ_HIT(ctx)) {
        /* JS_GetPropertyValue takes the key when it fails too */
        JS_FreeValue(ctx, key);
        val = JS_EXCEPTION;
    } else {
        val = JS_GetPropertyValue(ctx, obj, key);
    }
 done:
    if (t)
        JS_FreeValue(ctx, obj);
    if (unlikely(JS_IsException(val)))
        return -1;
    rv_put(ctx, F, (int8_t)ip[0], val);
    return 0;
}

/* the slow side of GETEL3, ip at d o t, as QuickJS's get_array_el3 */
static no_inline int rv_getel3_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue *pk = &F[(int8_t)ip[2]], val;
    if (RV_INJ_HIT(ctx))
        return -1;
    switch (JS_VALUE_GET_TAG(*pk)) {
    case JS_TAG_INT:
    case JS_TAG_STRING:
    case JS_TAG_SYMBOL:
        /* undefined and null are tested in JS_GetPropertyValue() */
        break;
    default:
        /* must be tested before JS_ToPropertyKey */
        if (unlikely(JS_IsUndefined(F[(int8_t)ip[1]]) || JS_IsNull(F[(int8_t)ip[1]]))) {
            JS_ThrowTypeError(ctx, "value has no property");
            return -1;
        }
        val = JS_ToPropertyKey(ctx, *pk);
        if (JS_IsException(val))
            return -1;
        JS_FreeValue(ctx, *pk);
        *pk = val;
        break;
    }
    val = JS_GetPropertyValue(ctx, F[(int8_t)ip[1]], JS_DupValue(ctx, *pk));
    if (unlikely(JS_IsException(val)))
        return -1;
    F[(int8_t)ip[0]] = val;
    return 0;
}

/* the slow side of PUTEL, ip at o key v: the end of a fast array, as
   QuickJS's put_array_el, else JS_SetPropertyValue. key and v are taken,
   a temp o freed. */
static no_inline int rv_putel_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue obj = F[(int8_t)ip[0]], key = F[(int8_t)ip[1]];
    int ret = 0;
    if (RV_INJ_HIT(ctx)) {
        /* JS_SetPropertyValue takes the key and the value when it fails */
        JS_FreeValue(ctx, rv_take(ctx, F, (int8_t)ip[1]));
        JS_FreeValue(ctx, rv_take(ctx, F, (int8_t)ip[2]));
        ret = -1;
        goto done;
    }
    if (JS_VALUE_GET_TAG(obj) == JS_TAG_OBJECT && JS_VALUE_GET_TAG(key) == JS_TAG_INT) {
        JSObject *p = JS_VALUE_GET_OBJ(obj);
        uint32_t idx = JS_VALUE_GET_INT(key), new_len, array_len;
        if (p->class_id != JS_CLASS_ARRAY)
            goto slow_path;
        if (idx < (uint32_t)p->u.array.count) {
            set_value(ctx, &p->u.array.u.values[idx], rv_take(ctx, F, (int8_t)ip[2]));
            goto done;
        }
        if (unlikely(idx != (uint32_t)p->u.array.count ||
                     !p->fast_array ||
                     !can_extend_fast_array(p)))
            goto slow_path;
        if (likely(JS_VALUE_GET_TAG(p->prop[0].u.value) != JS_TAG_INT))
            goto slow_path;
        /* cannot overflow otherwise the length would not be an integer */
        new_len = idx + 1;
        if (unlikely(new_len > p->u.array.u1.size))
            goto slow_path;
        array_len = JS_VALUE_GET_INT(p->prop[0].u.value);
        if (new_len > array_len) {
            if (unlikely(!(get_shape_prop(p->shape)->flags & JS_PROP_WRITABLE)))
                goto slow_path;
            p->prop[0].u.value = JS_NewInt32(ctx, new_len);
        }
        p->u.array.count = new_len;
        p->u.array.u.values[idx] = rv_take(ctx, F, (int8_t)ip[2]);
        goto done;
    }
 slow_path:
    ret = JS_SetPropertyValue(ctx, obj, rv_take(ctx, F, (int8_t)ip[1]),
                              rv_take(ctx, F, (int8_t)ip[2]), rv_throw_flags(F));
 done:
    if (rv_is_temp(F, (int8_t)ip[0]))
        JS_FreeValue(ctx, obj);
    return ret < 0 ? -1 : 0;
}

/* define_array_el, ip at o i v */
static no_inline int rv_defel(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    if (RV_INJ_HIT(ctx)) {
        /* JS_DefinePropertyValueValue takes the value when it fails */
        JS_FreeValue(ctx, rv_take(ctx, F, (int8_t)ip[2]));
        return -1;
    }
    return JS_DefinePropertyValueValue(ctx, F[(int8_t)ip[0]],
                                       JS_DupValue(ctx, F[(int8_t)ip[1]]),
                                       rv_take(ctx, F, (int8_t)ip[2]),
                                       JS_PROP_C_W_E | JS_PROP_THROW) < 0 ? -1 : 0;
}

/* define_field, ip at o v atom, or o k atom (DEFFK, k set) */
static no_inline int rv_deff(JSContext *ctx, JSValue *F, const uint8_t *ip, BOOL k)
{
    JSValue v = k ? JS_DupValue(ctx, ((RVHeader *)F)->k[ip[1]]) :
        rv_take(ctx, F, (int8_t)ip[1]);
    if (RV_INJ_HIT(ctx)) {
        /* JS_DefinePropertyValue takes the value when it fails */
        JS_FreeValue(ctx, v);
        return -1;
    }
    return JS_DefinePropertyValue(ctx, F[(int8_t)ip[0]], get_u32(ip + 2), v,
                                  JS_PROP_C_W_E | JS_PROP_THROW) < 0 ? -1 : 0;
}

/* define_method, ip at o v flags atom, as QuickJS's OP_define_method */
static no_inline int rv_defm(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue getter, setter, value, v = rv_take(ctx, F, (int8_t)ip[1]);
    JSValueConst obj = F[(int8_t)ip[0]];
    JSAtom atom = get_u32(ip + 3);
    int flags, ret, op_flags = ip[2];

    flags = JS_PROP_HAS_CONFIGURABLE | JS_PROP_CONFIGURABLE |
        JS_PROP_HAS_ENUMERABLE | JS_PROP_THROW;
    if (op_flags & OP_DEFINE_METHOD_ENUMERABLE)
        flags |= JS_PROP_ENUMERABLE;
    op_flags &= 3;
    value = JS_UNDEFINED;
    getter = JS_UNDEFINED;
    setter = JS_UNDEFINED;
    if (op_flags == OP_DEFINE_METHOD_METHOD) {
        value = v;
        flags |= JS_PROP_HAS_VALUE | JS_PROP_HAS_WRITABLE | JS_PROP_WRITABLE;
    } else if (op_flags == OP_DEFINE_METHOD_GETTER) {
        getter = v;
        flags |= JS_PROP_HAS_GET;
    } else {
        setter = v;
        flags |= JS_PROP_HAS_SET;
    }
    ret = RV_INJ_HIT(ctx) ? -1 : js_method_set_properties(ctx, v, atom, flags, obj);
    if (ret >= 0)
        ret = JS_DefineProperty(ctx, obj, atom, value, getter, setter, flags);
    JS_FreeValue(ctx, v);
    return ret < 0 ? -1 : 0;
}

/* regexp, in and instanceof, ip at d a b: both taken */
static no_inline int rv_obj2_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                  int rop)
{
    JSValue ops[2];
    int ret;
    if (rop == RV_INK)
        ops[0] = JS_DupValue(ctx, ((RVHeader *)F)->k[ip[1]]);
    else
        ops[0] = rv_take(ctx, F, (int8_t)ip[1]);
    ops[1] = rv_take(ctx, F, (int8_t)ip[2]);
    if (rop == RV_REGEXP) {
        if (RV_INJ_HIT(ctx)) {
            /* JS_NewRegexp takes both when it fails */
            JS_FreeValue(ctx, ops[0]);
            JS_FreeValue(ctx, ops[1]);
            return -1;
        }
        ops[0] = JS_NewRegexp(ctx, ops[0], ops[1]);
        if (JS_IsException(ops[0]))
            return -1;
    } else {
        if (RV_INJ_HIT(ctx))
            ret = -1;
        else if (rop == RV_IN || rop == RV_INK)
            ret = js_operator_in(ctx, ops + 2);
        else
            ret = js_operator_instanceof(ctx, ops + 2);
        if (ret) {
            JS_FreeValue(ctx, ops[0]);
            JS_FreeValue(ctx, ops[1]);
            return -1;
        }
    }
    rv_put(ctx, F, (int8_t)ip[0], ops[0]);
    return 0;
}

/* the slow side of GETVAR, ip at d idx16: the variable is not initialized,
   as QuickJS's OP_get_var */
static no_inline int rv_getvar_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                    BOOL throw_ref_error)
{
    JSClosureVar *cv = &((RVHeader *)F)->rf->b->closure_var[get_u16(ip + 1)];
    JSValue val;
    if (RV_INJ_HIT(ctx))
        return -1;
    if (cv->is_lexical) {
        JS_ThrowReferenceErrorUninitialized(ctx, cv->var_name);
        return -1;
    }
    val = JS_GetPropertyInternal(ctx, ctx->global_obj, cv->var_name,
                                 ctx->global_obj, throw_ref_error);
    if (JS_IsException(val))
        return -1;
    rv_put(ctx, F, (int8_t)ip[0], val);
    return 0;
}

/* the slow side of PUTVAR, ip at v idx16: the variable is not initialized
   or is a constant, as QuickJS's OP_put_var. v is taken. */
static no_inline int rv_putvar_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                    BOOL init)
{
    int idx = get_u16(ip + 1), ret;
    JSVarRef *var_ref = RV_VREF(F, idx);
    JSClosureVar *cv = &((RVHeader *)F)->rf->b->closure_var[idx];
    JSValue v = rv_take(ctx, F, (int8_t)ip[0]);
    if (RV_INJ_HIT(ctx))
        goto fail;
    if (var_ref->is_lexical) {
        if (init) {
            set_value(ctx, var_ref->pvalue, v);
            return 0;
        }
        if (JS_IsUninitialized(*var_ref->pvalue))
            JS_ThrowReferenceErrorUninitialized(ctx, cv->var_name);
        else
            JS_ThrowTypeErrorReadOnly(ctx, JS_PROP_THROW, cv->var_name);
        goto fail;
    }
    ret = JS_HasProperty(ctx, ctx->global_obj, cv->var_name);
    if (ret < 0)
        goto fail;
    /* a store to an undeclared variable creates it in sloppy code */
    if (ret == 0 && rv_throw_flags(F)) {
        JS_ThrowReferenceErrorNotDefined(ctx, cv->var_name);
        goto fail;
    }
    ret = JS_SetPropertyInternal(ctx, ctx->global_obj, cv->var_name, v,
                                 ctx->global_obj, rv_throw_flags(F));
    return ret < 0 ? -1 : 0;
 fail:
    JS_FreeValue(ctx, v);
    return -1;
}

/* the slow side of TOPK, ip at d a: a temp a is freed */
static no_inline int rv_topk_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue v = F[(int8_t)ip[1]], r;
    r = RV_INJ_HIT(ctx) ? JS_EXCEPTION : JS_ToPropertyKey(ctx, v);
    if (rv_is_temp(F, (int8_t)ip[1]))
        JS_FreeValue(ctx, v);
    if (JS_IsException(r))
        return -1;
    rv_put(ctx, F, (int8_t)ip[0], r);
    return 0;
}
#elif RV_PART == RV_PART_RUN
    /* get_array_el: an element of a fast array here, as QuickJS's
       GET_ARRAY_EL_INLINE, the rest in rv_getel_slow */
#define RV_GETEL(label, kexpr, isk, temp)                               \
 label:                                                                 \
    ip = pc; pc += 3;                                                   \
    {                                                                   \
        JSValue obj = RV_R(ip[1]), key = (kexpr);                       \
        if (likely(JS_VALUE_GET_TAG(obj) == JS_TAG_OBJECT &&            \
                   JS_VALUE_GET_TAG(key) == JS_TAG_INT) && !RV_INJ_ARMED) { \
            uint32_t idx = JS_VALUE_GET_INT(key);                       \
            p = JS_VALUE_GET_OBJ(obj);                                  \
            if (likely(p->class_id == JS_CLASS_ARRAY &&                 \
                       idx < p->u.array.count)) {                       \
                RV_PUTD(ip[0], JS_DupValue(ctx, p->u.array.u.values[idx])); \
                if (temp)                                               \
                    JS_FreeValue(ctx, obj);                             \
                RV_NEXT();                                              \
            }                                                           \
            if (rv_ta_num(p->class_id)) {                               \
                JSValue v_ = rv_ta_get(ctx, p, idx);                    \
                if (JS_VALUE_GET_TAG(v_) != JS_TAG_UNINITIALIZED) {     \
                    RV_PUTD(ip[0], v_);                                 \
                    if (temp)                                           \
                        JS_FreeValue(ctx, obj);                         \
                    RV_NEXT();                                          \
                }                                                       \
            }                                                           \
        }                                                               \
        RV_SYNC_TOP();                                                  \
        if (rv_getel_slow(ctx, F, ip, isk, temp))                       \
            goto rv_exception;                                          \
    }                                                                   \
    RV_NEXT();
    RV_GETEL(rv_GETEL, RV_R(ip[2]), FALSE, FALSE)
    RV_GETEL(rv_GETELT, RV_R(ip[2]), FALSE, TRUE)
    RV_GETEL(rv_GETELK, RV_K(ip[2]), TRUE, FALSE)
    RV_GETEL(rv_GETELKT, RV_K(ip[2]), TRUE, TRUE)
#undef RV_GETEL

 rv_GETEL3:
    ip = pc; pc += 3;
    {
        JSValue obj = RV_R(ip[1]), key = RV_R(ip[2]);
        if (likely(JS_VALUE_GET_TAG(obj) == JS_TAG_OBJECT &&
                   JS_VALUE_GET_TAG(key) == JS_TAG_INT) && !RV_INJ_ARMED) {
            uint32_t idx = JS_VALUE_GET_INT(key);
            p = JS_VALUE_GET_OBJ(obj);
            if (likely(p->class_id == JS_CLASS_ARRAY &&
                       idx < p->u.array.count)) {
                RV_R(ip[0]) = JS_DupValue(ctx, p->u.array.u.values[idx]);
                RV_NEXT();
            }
            if (rv_ta_num(p->class_id) &&
                js_get_fast_array_element_out_of_line(ctx, p, idx, &RV_R(ip[0])))
                RV_NEXT();
        }
        RV_SYNC_TOP();
        if (rv_getel3_slow(ctx, F, ip))
            goto rv_exception;
    }
    RV_NEXT();

    /* put_array_el: an element in the count of a fast array here, when
       the old value has no reference count, so that nothing is freed
       before o; the rest in rv_putel_slow */
 rv_PUTEL:
    ip = pc; pc += 3;
    {
        JSValue obj = RV_R(ip[0]), key = RV_R(ip[1]);
        if (likely(JS_VALUE_GET_TAG(obj) == JS_TAG_OBJECT &&
                   JS_VALUE_GET_TAG(key) == JS_TAG_INT) && !RV_INJ_ARMED) {
            uint32_t idx = JS_VALUE_GET_INT(key);
            p = JS_VALUE_GET_OBJ(obj);
            if (likely(p->class_id == JS_CLASS_ARRAY &&
                       idx < p->u.array.count &&
                       !JS_VALUE_HAS_REF_COUNT(p->u.array.u.values[idx]))) {
                p->u.array.u.values[idx] = rv_take(ctx, F, (int8_t)ip[2]);
                if (rv_is_temp(F, (int8_t)ip[0]))
                    JS_FreeValue(ctx, obj);
                RV_NEXT();
            }
            if (rv_ta_num(p->class_id) && idx < p->u.array.count &&
                (JS_VALUE_GET_TAG(RV_R(ip[2])) == JS_TAG_INT ||
                 JS_TAG_IS_FLOAT64(JS_VALUE_GET_TAG(RV_R(ip[2]))))) {
                js_set_fast_array_element_out_of_line(ctx, p, idx, rv_take(ctx, F, (int8_t)ip[2]), 0);
                if (rv_is_temp(F, (int8_t)ip[0]))
                    JS_FreeValue(ctx, obj);
                RV_NEXT();
            }
        }
        RV_SYNC_TOP();
        if (rv_putel_slow(ctx, F, ip))
            goto rv_exception;
    }
    RV_NEXT();

 rv_DEFEL:
    ip = pc; pc += 3;
    RV_SYNC_TOP();
    if (rv_defel(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();

 rv_OBJ:
    ip = pc; pc += 1;
    /* JS_NewObject can collect */
    RV_SYNC_TOP();
    {
        JSValue v = RV_INJ_HIT(ctx) ? JS_EXCEPTION : JS_NewObject(ctx);
        if (unlikely(JS_IsException(v)))
            goto rv_exception;
        RV_PUTD(ip[0], v);
    }
    RV_NEXT();

 rv_DEFF:
    ip = pc; pc += 6;
    RV_SYNC_TOP();
    if (rv_deff(ctx, F, ip, FALSE))
        goto rv_exception;
    RV_NEXT();
 rv_DEFFK:
    ip = pc; pc += 6;
    RV_SYNC_TOP();
    if (rv_deff(ctx, F, ip, TRUE))
        goto rv_exception;
    RV_NEXT();

 rv_DEFM:
    ip = pc; pc += 7;
    RV_SYNC_TOP();
    if (rv_defm(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();

 rv_REGEXP:
 rv_IN:
 rv_INK:
 rv_INSTOF:
    ip = pc; pc += 3;
    RV_SYNC_TOP();
    if (rv_obj2_slow(ctx, F, ip, ip[-1]))
        goto rv_exception;
    RV_NEXT();

    /* get_var: an initialized variable here, the rest in rv_getvar_slow */
#define RV_GETVAR(label, throw_ref_error)                               \
 label:                                                                 \
    ip = pc; pc += 3;                                                   \
    {                                                                   \
        JSValue v = *RV_VREF(F, get_u16(ip + 1))->pvalue; \
        if (likely(!JS_IsUninitialized(v)) && !RV_INJ_ARMED) {          \
            RV_PUTD(ip[0], JS_DupValue(ctx, v));                        \
            RV_NEXT();                                                  \
        }                                                               \
        RV_SYNC_TOP();                                                  \
        if (rv_getvar_slow(ctx, F, ip, throw_ref_error))                \
            goto rv_exception;                                          \
    }                                                                   \
    RV_NEXT();
    RV_GETVAR(rv_GETVAR, TRUE)
    RV_GETVAR(rv_GETVARU, FALSE)
#undef RV_GETVAR

#define RV_PUTVAR(label, init)                                          \
 label:                                                                 \
    ip = pc; pc += 3;                                                   \
    {                                                                   \
        JSVarRef *var_ref = RV_VREF(F, get_u16(ip + 1)); \
        if (likely(!JS_IsUninitialized(*var_ref->pvalue) &&             \
                   !var_ref->is_const) && !RV_INJ_ARMED) {              \
            set_value(ctx, var_ref->pvalue, rv_take(ctx, F, (int8_t)ip[0])); \
            RV_NEXT();                                                  \
        }                                                               \
        RV_SYNC_TOP();                                                  \
        if (rv_putvar_slow(ctx, F, ip, init))                           \
            goto rv_exception;                                          \
    }                                                                   \
    RV_NEXT();
    RV_PUTVAR(rv_PUTVAR, FALSE)
    RV_PUTVAR(rv_PUTVARI, TRUE)
#undef RV_PUTVAR

 rv_TOPK:
    ip = pc; pc += 2;
    {
        JSValue v = RV_R(ip[1]);
        switch (RV_INJ_ARMED ? JS_TAG_UNDEFINED : JS_VALUE_GET_TAG(v)) {
        case JS_TAG_INT:
        case JS_TAG_STRING:
        case JS_TAG_SYMBOL:
            RV_PUTD(ip[0], rv_take(ctx, F, (int8_t)ip[1]));
            RV_NEXT();
        }
        RV_SYNC_TOP();
        if (rv_topk_slow(ctx, F, ip))
            goto rv_exception;
    }
    RV_NEXT();

#endif
