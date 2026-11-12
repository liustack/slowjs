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
RV_OP(ARGLEN)   /* d a: arguments.length, a the arguments local of
                   rv_lazy_args */
RV_OP(ARGGET)   /* d a key: arguments[key], the same a */
RV_OP(ARGMAT)   /* d a: d = the object in a, built there first if a is
                   undefined: any other read of the same a */
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
    if (kind == RV_ARGS_PLAIN && s->lazy_args) {
        /* nothing at entry: the put_loc of the local after it is read with
           it, and the local stays undefined until ARGGET builds the
           object (rv_lazy_args) */
        s->skip_to = s->pos + 2 + short_opcode_info(s->bc[s->pos + 2]).size;
        s->args = TRUE;
        return;
    }
    /* the mapped arguments are var refs into the frame's closure state:
       the compiler marks every argument of the function captured, so a
       function with arguments has one */
    if (kind == RV_ARGS_MAPPED && s->b->arg_count && !s->b->var_ref_count) {
        rv_crafted(s);
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

/* the local an old instruction names, -1 none, -2 an instruction with a
   local this file does not know. *get: it only pushes the value. */
static int rv_named_local(const uint8_t *p, int op, BOOL *get)
{
    *get = FALSE;
    switch (op) {
    case OP_get_loc:
        *get = TRUE;
        /* fall through */
    case OP_put_loc: case OP_set_loc: case OP_set_loc_uninitialized:
    case OP_get_loc_check: case OP_put_loc_check: case OP_set_loc_check:
    case OP_put_loc_check_init: case OP_get_loc_checkthis: case OP_close_loc:
        return get_u16(p + 1);
    case OP_get_loc8:
        *get = TRUE;
        /* fall through */
    case OP_put_loc8: case OP_set_loc8: case OP_inc_loc: case OP_dec_loc:
    case OP_add_loc: case OP_get_loc8_get_field:
        return p[1];
    case OP_get_loc0: case OP_get_loc1: case OP_get_loc2: case OP_get_loc3:
        *get = TRUE;
        return op - OP_get_loc0;
    case OP_put_loc0: case OP_put_loc1: case OP_put_loc2: case OP_put_loc3:
        return op - OP_put_loc0;
    case OP_set_loc0: case OP_set_loc1: case OP_set_loc2: case OP_set_loc3:
        return op - OP_set_loc0;
    case OP_get_loc0_get_field:
        return 0;
    case OP_make_loc_ref:
        return get_u16(p + 5);
    default:
        switch (short_opcode_info(op).fmt) {
        case OP_FMT_loc: case OP_FMT_loc8: case OP_FMT_none_loc:
            return -2;
        }
        return -1;
    }
}

/* the get of a local at pos goes straight to the object operand of a
   get_length or a get_array_el, with no jump and no block start on the
   way, so that the entry is still the lazy read of the local there: the
   first instruction after it that pops it, through the depths of
   rv_depths */
static BOOL rv_lazy_fast(RVPass *s, int pos)
{
    const uint8_t *bc = s->bc;
    int d = s->depth[pos], q, npop, npush, term, target;

    if (d < 0)
        return FALSE; /* not reached: nothing is emitted for it */
    for (q = pos + short_opcode_info(bc[pos]).size;; q += short_opcode_info(bc[q]).size) {
        if (q >= s->len || s->leader[q] || s->depth[q] < 0)
            return FALSE;
        if (rv_old_info(bc, q, &npop, &npush, &term, &target) < 0)
            return FALSE;
        if (s->depth[q] - npop <= d)
            break;
        if (term || target >= 0)
            return FALSE;
    }
    return (bc[q] == OP_get_length && s->depth[q] == d + 1) ||
           (bc[q] == OP_get_array_el && s->depth[q] == d + 2);
}

/* The arguments object of a strict function made only when a read needs
   it: the slot of its local, or 0 when the function makes it at entry.
   A normal function (no generator, no async function) qualifies when the
   object has the values of entry whenever it is built: no instruction
   writes an argument and no closure captures one, nor the local. Every
   read of the local, but the put_loc of the prologue (special_object
   ARGUMENTS then put_loc), is a get, and at least one goes straight to
   the object operand of a get_length or a get_array_el (rv_lazy_fast):
   ARGLEN and ARGGET read the frame there while the local is undefined.
   Any other get, an escape, is ARGMAT, which builds the object into the
   local if it is not there and copies it: from then on every read of the
   call sees that object, as it would the one made at entry. A direct eval or an arrow function that reads arguments
   captures the local, strict code cannot assign it, and Error.stack and
   the backtrace show no argument. Not for a function that folds or names
   slots past -128 (the pass reads those differently). One walk for the
   prologue and the writes, one for the reads. */
static int rv_lazy_args(RVPass *s)
{
    JSFunctionBytecode *b = s->b;
    const uint8_t *bc = s->bc;
    int pos, op, size, a = -1, init = -1, idx, i;
    BOOL get, fast = FALSE;

    if (b->func_kind != JS_FUNC_NORMAL || !(b->js_mode & JS_MODE_STRICT) ||
        s->fold || s->wide)
        return 0;
    for (pos = 0; pos < s->len; pos += size) {
        op = bc[pos];
        size = short_opcode_info(op).size;
        switch (op) {
        case OP_special_object:
            if (bc[pos + 1] == OP_SPECIAL_OBJECT_MAPPED_ARGUMENTS)
                return 0;
            if (bc[pos + 1] != OP_SPECIAL_OBJECT_ARGUMENTS)
                break;
            if (a >= 0 || pos + 2 >= s->len)
                return 0;
            switch (bc[pos + 2]) {
            case OP_put_loc: case OP_put_loc8:
            case OP_put_loc0: case OP_put_loc1: case OP_put_loc2: case OP_put_loc3:
                break;
            default:
                return 0;
            }
            a = rv_named_local(bc + pos + 2, bc[pos + 2], &get);
            init = pos + 2;
            break;
        case OP_put_arg: case OP_set_arg: case OP_make_arg_ref:
        case OP_put_arg0: case OP_put_arg1: case OP_put_arg2: case OP_put_arg3:
        case OP_set_arg0: case OP_set_arg1: case OP_set_arg2: case OP_set_arg3:
            return 0;
        }
    }
    if (a < 0 || a >= b->var_count)
        return 0;
    if (b->var_ref_count) {
        for (i = 0; i < b->arg_count; i++) {
            if (b->vardefs[i].is_captured)
                return 0;
        }
        if (b->vardefs[b->arg_count + a].is_captured)
            return 0;
    }
    for (pos = 0; pos < s->len; pos += size) {
        op = bc[pos];
        size = short_opcode_info(op).size;
        idx = rv_named_local(bc + pos, op, &get);
        if (idx == -2)
            return 0;
        if (idx != a || pos == init)
            continue;
        if (!get)
            return 0;
        if (!fast && rv_lazy_fast(s, pos))
            fast = TRUE;
    }
    if (!fast)
        return 0; /* every read is an escape: nothing to gain */
    i = rv_loc(s, a);
    return rv_far_slot(i) ? 0 : i;
}

/* get_length of the arguments local of rv_lazy_args, the entry on top */
static void rv_arglen(RVPass *s)
{
    int d = rv_tslot(s, s->sp - 1);
    rv_emit(s, RV_ARGLEN);
    rv_byte(s, d);
    rv_byte(s, s->lazy_args);
    rv_throws(s, rv_owned(s, 1));
    s->sp--;
    rv_push(s, RV_E_TEMP, d, 0);
    rv_mark_dst(s, d);
}

/* get_array_el of it: the local, then the key */
static void rv_argget(RVPass *s)
{
    int k = rv_slot_of(s, s->sp - 1), d = rv_tslot(s, s->sp - 2);
    rv_emit(s, RV_ARGGET);
    rv_byte(s, d);
    rv_byte(s, s->lazy_args);
    rv_byte(s, k);
    rv_throws(s, rv_owned(s, 2));
    s->sp -= 2;
    rv_push(s, RV_E_TEMP, d, 0);
    rv_mark_dst(s, d);
}

/* a get of the arguments local of rv_lazy_args that rv_lazy_fast does not
   take: the object in a temp */
static void rv_argmat(RVPass *s)
{
    int d = rv_tslot(s, s->sp);
    rv_emit(s, RV_ARGMAT);
    rv_byte(s, d);
    rv_byte(s, s->lazy_args);
    rv_throws(s, rv_owned(s, 0));
    rv_push(s, RV_E_TEMP, d, 0);
    rv_mark_dst(s, d);
}

/* the entry i is the read of the arguments local of rv_lazy_args */
static BOOL rv_is_lazy_args(RVPass *s, int i)
{
    return s->lazy_args && s->st[i].kind == RV_E_SLOT && s->st[i].slot == s->lazy_args;
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

/* the slow side of ARGLEN, ip at d a: the length of the object built in
   the local, which a script may have redefined, else the count */
static no_inline int rv_arglen_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue v;
    if (RV_INJ_HIT(ctx))
        return -1;
    if (JS_IsUndefined(F[(int8_t)ip[1]])) {
        v = JS_NewInt32(ctx, ((RVHeader *)F)->argc);
    } else {
        v = JS_GetProperty(ctx, F[(int8_t)ip[1]], JS_ATOM_length);
        if (JS_IsException(v))
            return -1;
    }
    rv_put(ctx, F, (int8_t)ip[0], v);
    return 0;
}

/* the slow side of ARGGET, ip at d a key, the key taken: an argument read
   from the frame, or the get of the key on the object, which is built into
   the local first if it is not there. It is built once in a call, with the
   values of entry (rv_lazy_args), and stored only whole: a getter the get
   runs may keep it and change it, and every later read of the call sees
   that. */
static no_inline int rv_argget_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RVHeader *h = (RVHeader *)F;
    JSValue *pa = &F[(int8_t)ip[1]], key, v;
    key = rv_take(ctx, F, (int8_t)ip[2]);
    if (RV_INJ_HIT(ctx)) {
        JS_FreeValue(ctx, key);
        return -1;
    }
    if (JS_IsUndefined(*pa)) {
        if (JS_VALUE_GET_TAG(key) == JS_TAG_INT &&
            (uint32_t)JS_VALUE_GET_INT(key) < (uint32_t)h->argc) {
            rv_put(ctx, F, (int8_t)ip[0],
                   JS_DupValue(ctx, rv_arg_at(F, h->rf, h->argc, JS_VALUE_GET_INT(key))));
            return 0;
        }
        v = rv_build_arguments(ctx, F, h->rf, h->argc);
        if (JS_IsException(v)) {
            JS_FreeValue(ctx, key);
            return -1;
        }
        *pa = v;
    }
    v = JS_GetPropertyValue(ctx, *pa, key);
    if (JS_IsException(v))
        return -1;
    rv_put(ctx, F, (int8_t)ip[0], v);
    return 0;
}

/* ARGMAT, ip at d a: the object of the local, built there first if it is
   not there, with the values of entry (rv_lazy_args). Nothing here runs
   JS. */
static no_inline int rv_argmat_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RVHeader *h = (RVHeader *)F;
    JSValue *pa = &F[(int8_t)ip[1]], v;
    if (RV_INJ_HIT(ctx))
        return -1;
    if (JS_IsUndefined(*pa)) {
        v = rv_build_arguments(ctx, F, h->rf, h->argc);
        if (JS_IsException(v))
            return -1;
        *pa = v;
    }
    rv_put(ctx, F, (int8_t)ip[0], JS_DupValue(ctx, *pa));
    return 0;
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

    /* the arguments local of rv_lazy_args: the count and the arguments of
       the frame while it is undefined, the rest in the slow sides */
 rv_ARGLEN:
    ip = pc; pc += 2;
    if (likely(JS_IsUndefined(RV_R(ip[1]))) && !RV_INJ_ARMED) {
        RV_PUTD(ip[0], JS_NewInt32(ctx, ((RVHeader *)F)->argc));
        RV_NEXT();
    }
    RV_SYNC_TOP();
    if (rv_arglen_slow(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();

 rv_ARGGET:
    ip = pc; pc += 3;
    {
        JSValue key = RV_R(ip[2]);
        RVHeader *h = (RVHeader *)F;
        if (likely(JS_IsUndefined(RV_R(ip[1])) && JS_VALUE_GET_TAG(key) == JS_TAG_INT &&
                   (uint32_t)JS_VALUE_GET_INT(key) < (uint32_t)h->argc) && !RV_INJ_ARMED) {
            RV_PUTD(ip[0], JS_DupValue(ctx, rv_arg_at(F, h->rf, h->argc,
                                                      JS_VALUE_GET_INT(key))));
            RV_NEXT();
        }
    }
    RV_SYNC_TOP();
    if (rv_argget_slow(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();

 rv_ARGMAT:
    ip = pc; pc += 2;
    RV_SYNC_TOP();
    if (rv_argmat_slow(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();
#endif
