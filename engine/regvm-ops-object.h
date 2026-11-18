/*
 * Copyright (c) 2026 The liustack/slowjs Authors.
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
RV_OP(GETELE) RV_OP(GETELET) /* d o key it: GETEL and GETELT of the key of the for in
                               loop whose iterator is the temp it */
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

/* the iterator temp of the innermost for in loop (rv_for_in_ctx,
   regvm-ops-window.h) whose body the old instruction at s->pos is in and
   whose key slot the key entry ek reads, when the iterator is still at its
   depth under the two operands, or -1 */
static int rv_for_in_it(RVPass *s, RVEnt ek)
{
    int i;
    if (ek.kind != RV_E_SLOT)
        return -1;
    for (i = s->fi_n - 1; i >= 0; i--) {
        if (s->pos < s->fi[i].lo || s->pos >= s->fi[i].hi ||
            ek.slot != s->fi[i].key)
            continue;
        if (s->fi[i].d >= s->sp - 2 ||
            s->st[s->fi[i].d].kind != RV_E_TEMP ||
            s->st[s->fi[i].d].slot != s->fi[i].it)
            return -1;
        return s->fi[i].it;
    }
    return -1;
}

/* get_array_el (el2 FALSE): o key -> value, get_array_el2: o key -> o
   value. A read of the key of a for in loop in its body is GETELE or
   GETELET, which name the iterator too. */
static void rv_getel(RVPass *s, BOOL el2)
{
    int o, d, it = -1;
    RVEnt ek;
    o = rv_slot_of(s, s->sp - 2);
    ek = s->st[s->sp - 1];
    d = rv_tslot(s, s->sp - (el2 ? 1 : 2));
    if (!el2)
        it = rv_for_in_it(s, ek);
    if (it >= 0)
        rv_emit(s, s->st[s->sp - 2].kind == RV_E_TEMP ? RV_GETELET : RV_GETELE);
    else if (el2)
        rv_emit(s, ek.kind == RV_E_K ? RV_GETELK : RV_GETEL);
    else if (s->st[s->sp - 2].kind == RV_E_TEMP)
        rv_emit(s, ek.kind == RV_E_K ? RV_GETELKT : RV_GETELT);
    else
        rv_emit(s, ek.kind == RV_E_K ? RV_GETELK : RV_GETEL);
    rv_byte(s, d);
    rv_byte(s, o);
    rv_byte(s, ek.kind == RV_E_K ? ek.k : ek.slot);
    if (it >= 0)
        rv_byte(s, it);
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

/* GETEL (and GETEL3, which keeps the key) of the Array p by an int key i,
   ip at d o key|k: the index is read by its atom, an int atom, as
   rv_getel_atom reads a string key. The elements of an Array that is not
   a fast array are properties of its shape, and an Array has no other own
   properties than those of its shape and the elements below the count of
   a fast array. So the walk takes a plain data property of p or of an
   object on its chain of prototypes, goes on past an Array that has no
   such own element and past an ordinary object, and gives undefined at the
   end of the chain. Then the result is put in d, o freed for the T forms,
   and it gives TRUE. It gives FALSE with nothing done for a key below 0,
   for an object that is not an Array, and when the walk meets anything
   else (an accessor, a property made on first use, an element of a fast
   array on the chain, any other exotic object: a typed array, a Proxy, a
   String object, arguments): rv_getel_slow then reads it from the start.
   No code runs and nothing is allocated, as in rv_getel_atom, and the key
   is an int, which is never freed. It is a function of its own, so that
   the string key path of rv_getel_atom keeps its code. */
static no_inline BOOL rv_getel_index(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                     BOOL t, JSObject *p, int32_t i)
{
    JSValue obj = F[(int8_t)ip[1]], val;
    JSProperty *pr;
    JSShapeProperty *prs;
    JSAtom atom;
    uint32_t idx = i;

    if (i < 0 || p->class_id != JS_CLASS_ARRAY)
        return FALSE;
    /* every int from 0 is an int atom (2^31 - 1 at most) */
    atom = __JS_AtomFromUInt32(idx);
    for (;;) {
        prs = find_own_property(&pr, p, atom);
        if (prs) {
            if (unlikely(prs->flags & JS_PROP_TMASK))
                return FALSE;
            val = JS_DupValue(ctx, pr->u.value);
            break;
        }
        if (unlikely(p->is_exotic) &&
            (p->class_id != JS_CLASS_ARRAY ||
             (p->fast_array && idx < p->u.array.count)))
            return FALSE;
        p = p->shape->proto;
        if (!p) {
            val = JS_UNDEFINED;
            break;
        }
    }
    if (t)
        JS_FreeValue(ctx, obj);
    rv_put(ctx, F, (int8_t)ip[0], val);
    return TRUE;
}

/* The atom of the string key s that keeps none, for rv_getel_atom: 1 when
   the atom table has a string atom of its characters, which s then keeps
   as its key (key_atom, with a reference, and the hash 1), as
   js_string_to_atom keeps it, with the atom in *patom. 0 when it has none:
   no property of any object has that name, as the shape of each holds the
   atom of each of its keys. -1, with nothing done, for a string that is an
   atom of another kind, is empty, starts with a digit (an index has an int
   atom, which the table never holds) or is long enough that its hash reads
   the interrupt flag (js_new_long_string_atom). */
static no_inline __exception int rv_key_lookup(JSContext *ctx, JSString *s, JSAtom *patom)
{
    JSAtom atom;

    if (s->atom_type != 0 || s->len == 0 || s->len >= JS_ROPE_STEP ||
        is_digit(string_get(s, 0)))
        return -1;
    atom = js_find_string_atom(ctx->rt, s);
    if (atom == JS_ATOM_NULL)
        return 0;
    /* a string atom that does not start with a digit is its own key */
    s->key_atom = JS_DupAtom(ctx, atom);
    s->hash = 1;
    *patom = atom;
    return 1;
}

/* an exotic object whose own keys that are not indices are all in its
   shape: a fast array that is not a typed array (Array and arguments), as
   goasm_field_goes_on passes it for a key that is no index. A typed array
   answers a numeric string such as "-0" or "Infinity" itself. */
static inline BOOL rv_exotic_index_only(JSObject *p)
{
    return p->fast_array &&
        (p->class_id < JS_CLASS_UINT8C_ARRAY || p->class_id > JS_CLASS_FLOAT64_ARRAY);
}

/* GETEL of an object by a string key, ip at d o key|k, read as GETF reads
   a named property when the key is a string atom JS_ValueToAtom takes as
   its own atom (an atom of the type string, not empty, not starting with a
   digit, so never an index: see JS_ValueToAtom): a plain data property of o
   or of an object on its chain of prototypes, or undefined at the end of
   the chain. Then the result is put in d, a key in a temp freed, o too for
   the T forms, and it gives TRUE. It gives FALSE with nothing done for any
   other key, and when the walk meets anything else (an accessor, a closure
   variable of the global object, a property made on first use, the stack
   of an error, an exotic object that is not a fast array GETF passes):
   rv_getel_slow then reads it from the start. No code runs before the
   result is taken, so the key, which its slot keeps alive, keeps its atom
   alive without a reference of its own. It is a function of its own, so
   that the frame of rv_getel_slow, which a getter it calls sits on, does
   not grow. It tests the tags of o and the key itself, so that the loop
   keeps neither live past its fast paths: a test there made the loop
   keep them in registers on the path of an array element too. The loop
   calls it before RV_SYNC_TOP, as it frees a T object on its array path
   without it: nothing here runs code or collects, and freeing a value
   does neither. An int key goes to rv_getel_index. A string that keeps no
   atom yet is looked up in the atom table without making one
   (rv_key_lookup): found, it keeps its atom as js_string_to_atom would
   keep it, and the read goes on with it. Not found, no object has a
   property of that name, so the read is undefined when the chain holds no
   exotic object but the fast arrays GETF passes, with no atom made and
   freed: Emotion reads its registered styles by every class name of an
   element, most of which are not there. */
static no_inline BOOL rv_getel_atom(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                    BOOL k, BOOL t)
{
    JSValue obj = F[(int8_t)ip[1]], key, val;
    JSObject *p;
    JSAtom atom;
    JSProperty *pr;
    JSShapeProperty *prs;
    key = k ? ((RVHeader *)F)->k[ip[2]] : F[(int8_t)ip[2]];
    if (JS_VALUE_GET_TAG(obj) != JS_TAG_OBJECT)
        return FALSE;
    if (JS_VALUE_GET_TAG(key) != JS_TAG_STRING) {
        if (JS_VALUE_GET_TAG(key) != JS_TAG_INT)
            return FALSE;
        return rv_getel_index(ctx, F, ip, t, JS_VALUE_GET_OBJ(obj),
                              JS_VALUE_GET_INT(key));
    }
    /* the test of the key and its atom in one read (JSString.key_atom) */
    atom = JS_VALUE_GET_STRING(key)->key_atom;
    p = JS_VALUE_GET_OBJ(obj);
    if (unlikely(atom == JS_ATOM_NULL)) {
        int n = rv_key_lookup(ctx, JS_VALUE_GET_STRING(key), &atom);
        if (n < 0)
            return FALSE;
        if (n == 0) {
            /* no object has a property of this name: undefined at the
               end of a chain of objects that have no other own keys */
            for (;;) {
                if (unlikely(p->is_exotic) && !rv_exotic_index_only(p))
                    return FALSE;
                p = p->shape->proto;
                if (!p)
                    break;
            }
            val = JS_UNDEFINED;
            goto done;
        }
    }
    for (;;) {
        prs = find_own_property(&pr, p, atom);
        if (prs) {
            if (unlikely(prs->flags & JS_PROP_TMASK))
                return FALSE;
            if (atom == JS_ATOM_stack &&
                JS_VALUE_GET_TAG(pr->u.value) == JS_TAG_OBJECT)
                return FALSE;
            val = JS_DupValue(ctx, pr->u.value);
            break;
        }
        if (unlikely(p->is_exotic) && !goasm_field_goes_on(p, atom))
            return FALSE;
        p = p->shape->proto;
        if (!p) {
            val = JS_UNDEFINED;
            break;
        }
    }
 done:
    if (!k && rv_is_temp(F, (int8_t)ip[2]))
        JS_FreeValue(ctx, key);
    if (t)
        JS_FreeValue(ctx, obj);
    rv_put(ctx, F, (int8_t)ip[0], val);
    return TRUE;
}

/* the slow side of GETEL, ip at d o key|k, as the slow path of
   QuickJS's get_array_el: a key in a temp is freed, o too for the T
   forms */
static no_inline __exception int rv_getel_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
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
static no_inline __exception int rv_getel3_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue *pk = &F[(int8_t)ip[2]], val;
    if (RV_INJ_HIT(ctx))
        return -1;
    /* an index of an Array, as GETEL reads it: d is a temp */
    if (JS_VALUE_GET_TAG(*pk) == JS_TAG_INT &&
        JS_VALUE_GET_TAG(F[(int8_t)ip[1]]) == JS_TAG_OBJECT &&
        rv_getel_index(ctx, F, ip, FALSE, JS_VALUE_GET_OBJ(F[(int8_t)ip[1]]),
                       JS_VALUE_GET_INT(*pk)))
        return 0;
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

/* rv_putel_slow when the element one past the end of a fast array takes
   more room than the array has: a full collection first when that room would
   pass the memory limit (js_trigger_gc_block), then the slow path of
   rv_putel_slow. The elements grow in JS_SetPropertyValue, which may call a
   setter, and o, the key and v are in the frame. It is a function of its own,
   which rv_putel_slow calls last, so that the paths that do not grow the
   array are the code they were. */
static no_inline int rv_putel_grow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue obj = F[(int8_t)ip[0]], key = F[(int8_t)ip[1]];
    JSObject *p = JS_VALUE_GET_OBJ(obj);
    uint32_t new_len = JS_VALUE_GET_INT(key) + 1;
    uint32_t size = p->u.array.u1.size;
    int ret;

    /* the growth of the block of elements (expand_fast_array) */
    js_trigger_gc_block(ctx->rt, sizeof(JSValue) *
                        (size_t)(max_uint32(new_len, size + size / 2) - size));
    /* the slow path and the end of rv_putel_slow. The key is an int, which
       JS_SetPropertyValue frees as nothing, so the key in a local needs no
       JS_SetPropertyValueConst, which rv_putel_slow alone calls and so keeps
       inline. */
    ret = JS_SetPropertyValue(ctx, obj, key, rv_take(ctx, F, (int8_t)ip[2]),
                              rv_throw_flags(F));
    if (rv_is_temp(F, (int8_t)ip[0]))
        JS_FreeValue(ctx, obj);
    return ret < 0 ? -1 : 0;
}

/* PUTEL, ip at o key v, with a string key that is its own atom (the field
   key_atom of the string, never an index, see rv_getel_atom), when the
   store only writes a value (rv_put_atom_prop): an own writable data
   property gets v, or the key is added as the generic path adds it. 1 when
   done, with v and a key in a temp taken and a temp o freed as
   rv_putel_slow does, 0 with nothing done for anything else, which
   rv_putel_slow then does from the start, -1 when add_property failed, with
   the operands taken and freed as when rv_putel_slow fails. The key, which
   its slot keeps alive, keeps its atom alive: add_property takes its own
   reference. The loop calls it only for a string key, so that a store at
   an index (the end of an array) does not pay the call, and after
   RV_SYNC_TOP: it may throw (no memory, too many properties), and the old
   value it replaces may be the last reference to an object. It is a
   function of its own, so that the frame of rv_putel_slow, which a setter
   it calls sits on, does not grow. */
static no_inline int rv_putel_atom(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue obj = F[(int8_t)ip[0]], key = F[(int8_t)ip[1]];
    JSProperty *pr;
    JSAtom atom;
    int r;

    if (JS_VALUE_GET_TAG(obj) != JS_TAG_OBJECT)
        return 0;
    atom = JS_VALUE_GET_STRING(key)->key_atom;
    if (atom == JS_ATOM_NULL)
        return 0;
    r = rv_put_atom_prop(ctx, JS_VALUE_GET_OBJ(obj), atom, &pr);
    if (r == 0)
        return 0;
    if (r == 1)
        set_value(ctx, &pr->u.value, rv_take(ctx, F, (int8_t)ip[2]));
    else if (r == 2)
        pr->u.value = rv_take(ctx, F, (int8_t)ip[2]);
    else if (rv_is_temp(F, (int8_t)ip[2]))
        JS_FreeValue(ctx, F[(int8_t)ip[2]]);
    if (rv_is_temp(F, (int8_t)ip[1]))
        JS_FreeValue(ctx, key);
    if (rv_is_temp(F, (int8_t)ip[0]))
        JS_FreeValue(ctx, obj);
    return r < 0 ? -1 : 1;
}

/* rv_putel_slow on an Array p that is not a fast array, at an index idx
   from an int key: when p has idx as an own writable data property (its
   elements are properties of its shape), v takes its place, as
   JS_SetPropertyInternal does for such a property of the receiver, and it
   gives TRUE with v taken. FALSE with nothing done for anything else (no
   such own element, so that the prototypes and the length matter, an
   accessor, a read-only element, an int key below 0), which rv_putel_slow
   then does as before. No code runs before v is stored: the old value is
   freed after. It is a function of its own, so that the frame of
   rv_putel_slow, which a setter it calls sits on, does not grow. */
static no_inline BOOL rv_putel_index(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                     JSObject *p, uint32_t idx)
{
    JSProperty *pr;
    JSShapeProperty *prs;

    if (idx > JS_ATOM_MAX_INT)
        return FALSE;
    prs = find_own_property(&pr, p, __JS_AtomFromUInt32(idx));
    if (!prs || (prs->flags & (JS_PROP_TMASK | JS_PROP_WRITABLE | JS_PROP_LENGTH)) !=
        JS_PROP_WRITABLE)
        return FALSE;
    set_value(ctx, &pr->u.value, rv_take(ctx, F, (int8_t)ip[2]));
    return TRUE;
}

/* the slow side of PUTEL, ip at o key v: the end of a fast array, as
   QuickJS's put_array_el, else JS_SetPropertyValue. v and a key in a temp
   are taken, a temp o freed. */
static no_inline __exception int rv_putel_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
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
                     !can_extend_fast_array(p))) {
            if (!p->fast_array && rv_putel_index(ctx, F, ip, p, idx))
                goto done;
            goto slow_path;
        }
        /* the length of an array is an int below 2^31 */
        if (unlikely(JS_VALUE_GET_TAG(p->prop[0].u.value) != JS_TAG_INT))
            goto slow_path;
        /* cannot overflow otherwise the length would not be an integer. At
           idx 2^31 - 1 new_len is past the room, which never passes
           2^31 - 1 (expand_fast_array), so the element goes through
           rv_putel_grow to JS_SetPropertyValue, which throws */
        new_len = idx + 1;
        if (unlikely(new_len > p->u.array.u1.size))
            return rv_putel_grow(ctx, F, ip);
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
    if (rv_is_temp(F, (int8_t)ip[1]))
        ret = JS_SetPropertyValue(ctx, obj, key, rv_take(ctx, F, (int8_t)ip[2]),
                                  rv_throw_flags(F));
    else
        /* a key in a local or an argument is read without a reference, as
           obj is: no other code can write it while the key is made and the
           property set (a captured one is never an operand, rv_push) */
        ret = JS_SetPropertyValueConst(ctx, obj, key, rv_take(ctx, F, (int8_t)ip[2]),
                                       rv_throw_flags(F));
 done:
    if (rv_is_temp(F, (int8_t)ip[0]))
        JS_FreeValue(ctx, obj);
    return ret < 0 ? -1 : 0;
}

/* define_array_el, ip at o i v */
static no_inline __exception int rv_defel(JSContext *ctx, JSValue *F, const uint8_t *ip)
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
static no_inline __exception int rv_deff(JSContext *ctx, JSValue *F, const uint8_t *ip, BOOL k)
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

/* x instanceof C with nothing run, ip at d a b: 1 with the boolean in d and
   the temps a and b freed, 0 with nothing done, for rv_obj2_slow. The
   specification's InstanceofOperator(a, b) calls GetMethod(b, @@hasInstance),
   then that method. When b is a function (callable, not a proxy, not bound)
   and no object from b up to the Function.prototype of this realm has an own
   @@hasInstance or is exotic, the method is the one Function.prototype got
   when the realm was made, js_function_hasInstance (not writable and not
   configurable, so it is still that one, once a read has made it), and its
   call is
   OrdinaryHasInstance(b, a): false for a that is not an object, else the own
   data property prototype of b, an object, looked for in the chain of a,
   which must have no proxy. Anything else (a getter, the prototype not made
   yet, an error to throw, a proxy) is left to the generic path, which does
   it all again: nothing here can be seen, throws, allocates or calls. The
   call this skips is the one of the built-in, with the checks at its entry:
   the interrupt flag is read here as there, and the level of depth it
   counted is not counted (a script recurses deeper, never less deep). */
static no_inline int rv_instof(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue v = F[(int8_t)ip[1]], fv = F[(int8_t)ip[2]];
    JSObject *f, *p, *fproto;
    JSProperty *pr;
    JSShapeProperty *prs;
    BOOL ret;

    /* the call of the built-in stops at the host's stop and while an
       uncatchable exception is pending (its 2 in the flag), so the code
       after the test does not run: the generic path makes that call */
    if (unlikely(poll_flag(&ctx->rt->interrupt_flag)) ||
        JS_VALUE_GET_TAG(fv) != JS_TAG_OBJECT)
        return 0;
    f = JS_VALUE_GET_OBJ(fv);
    if (f->is_exotic || f->class_id == JS_CLASS_BOUND_FUNCTION ||
        (f->class_id != JS_CLASS_BYTECODE_FUNCTION &&
         !ctx->rt->class_array[f->class_id].call))
        return 0;
    /* @@hasInstance: the realm's own, with no other on the way */
    fproto = JS_VALUE_GET_OBJ(ctx->function_proto);
    for (p = f; p != fproto; ) {
        if (p->is_exotic || find_own_property(&pr, p, JS_ATOM_Symbol_hasInstance))
            return 0;
        p = p->shape->proto;
        if (!p)
            return 0;
    }
    /* and made already: the read of the generic path makes it the first
       time, and a test answered here must leave the memory as that read */
    prs = find_own_property(&pr, fproto, JS_ATOM_Symbol_hasInstance);
    if (!prs || (prs->flags & JS_PROP_TMASK))
        return 0;
    if (JS_VALUE_GET_TAG(v) != JS_TAG_OBJECT) {
        ret = FALSE;
    } else {
        prs = find_own_property(&pr, f, JS_ATOM_prototype);
        if (!prs || (prs->flags & JS_PROP_TMASK) ||
            JS_VALUE_GET_TAG(pr->u.value) != JS_TAG_OBJECT)
            return 0;
        fproto = JS_VALUE_GET_OBJ(pr->u.value);
        /* the walk of JS_OrdinaryIsInstanceOf: only an exotic object with
           no prototype in its shape (a proxy) has another one */
        for (p = JS_VALUE_GET_OBJ(v); ; ) {
            JSObject *p1 = p->shape->proto;
            if (!p1) {
                if (p->is_exotic && !p->fast_array)
                    return 0;
                ret = FALSE;
                break;
            }
            p = p1;
            if (p == fproto) {
                ret = TRUE;
                break;
            }
        }
    }
    if (rv_is_temp(F, (int8_t)ip[1]))
        JS_FreeValue(ctx, v);
    if (rv_is_temp(F, (int8_t)ip[2]))
        JS_FreeValue(ctx, fv);
    rv_put(ctx, F, (int8_t)ip[0], JS_NewBool(ctx, ret));
    return 1;
}

/* regexp, in and instanceof, ip at d a b: both taken */
static no_inline __exception int rv_obj2_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
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
static no_inline __exception int rv_getvar_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
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
static no_inline __exception int rv_putvar_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
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
static no_inline __exception int rv_topk_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
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
       GET_ARRAY_EL_INLINE, a string key that is its own atom and an index
       of an Array that is not a fast array in rv_getel_atom, the rest in
       rv_getel_slow */
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
        if (!RV_INJ_ARMED && rv_getel_atom(ctx, F, ip, isk, temp))     \
            RV_NEXT();                                                  \
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

    /* the read of the key of a for in loop (rv_getel, d o key it): when o
       has the shape the iterator in the temp it holds and the key is the
       atom string of the slot before the one the iterator is at, the key
       the loop gave last, the value of that slot when it is plain data, as
       rv_getel_atom finds it. The iterator holds its shape, so an object
       of that shape got it before the loop and keeps it until it changes
       a key or a flag, which gives it another shape: the slot is the key's
       on any object of it. Else as GETEL and GETELT of a string key. */
#define RV_GETELE(label, temp)                                          \
 label:                                                                 \
    ip = pc; pc += 4;                                                   \
    {                                                                   \
        JSValue obj = RV_R(ip[1]), key = RV_R(ip[2]), itv = RV_R(ip[3]); \
        if (likely(JS_VALUE_GET_TAG(key) == JS_TAG_STRING)) {           \
            JSAtom atom = JS_VALUE_GET_STRING(key)->key_atom;           \
            /* rv_getel_atom gives up on a key with no atom, such as   \
               the new string of an index a loop over an array, a      \
               string or arguments gives */                            \
            if (unlikely(atom == JS_ATOM_NULL))                         \
                goto label##_slow;                                      \
            if (likely(JS_VALUE_GET_TAG(obj) == JS_TAG_OBJECT &&        \
                       JS_VALUE_GET_TAG(itv) == JS_TAG_OBJECT && !RV_INJ_ARMED && \
                       JS_VALUE_GET_OBJ(itv)->class_id == JS_CLASS_FOR_IN_ITERATOR)) { \
                JSForInIterator *it = JS_VALUE_GET_OBJ(itv)->u.for_in_iterator; \
                p = JS_VALUE_GET_OBJ(obj);                              \
                if (likely(p->shape == it->sh && it->idx != 0)) {       \
                    JSShapeProperty *prs = &get_shape_prop(p->shape)[it->idx - 1]; \
                    JSValue v_ = p->prop[it->idx - 1].u.value;          \
                    if (likely(prs->atom == atom && !(prs->flags & JS_PROP_TMASK) && \
                               !(atom == JS_ATOM_stack &&               \
                                 JS_VALUE_GET_TAG(v_) == JS_TAG_OBJECT))) { \
                        RV_PUTD(ip[0], JS_DupValue(ctx, v_));           \
                        if (temp)                                       \
                            JS_FreeValue(ctx, obj);                     \
                        RV_NEXT();                                      \
                    }                                                   \
                }                                                       \
            }                                                           \
        }                                                               \
        if (!RV_INJ_ARMED && rv_getel_atom(ctx, F, ip, FALSE, temp))   \
            RV_NEXT();                                                  \
    label##_slow:                                                       \
        RV_SYNC_TOP();                                                  \
        if (rv_getel_slow(ctx, F, ip, FALSE, temp))                     \
            goto rv_exception;                                          \
    }                                                                   \
    RV_NEXT();
    RV_GETELE(rv_GETELE, FALSE)
    RV_GETELE(rv_GETELET, TRUE)
#undef RV_GETELE

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
       before o; a string key that is its own atom in rv_putel_atom, the
       rest in rv_putel_slow */
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
                /* a number into a typed array in range cannot fail */
                (void)js_set_fast_array_element_out_of_line(ctx, p, idx, rv_take(ctx, F, (int8_t)ip[2]), 0);
                if (rv_is_temp(F, (int8_t)ip[0]))
                    JS_FreeValue(ctx, obj);
                RV_NEXT();
            }
        }
        RV_SYNC_TOP();
        if (JS_VALUE_GET_TAG(key) == JS_TAG_STRING && !RV_INJ_ARMED) {
            int r = rv_putel_atom(ctx, F, ip);
            if (r > 0)
                RV_NEXT();
            if (r < 0)
                goto rv_exception;
        }
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

 rv_INSTOF:
    if (!RV_INJ_ARMED && rv_instof(ctx, F, pc)) {
        pc += 3;
        RV_NEXT();
    }
 rv_REGEXP:
 rv_IN:
 rv_INK:
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
