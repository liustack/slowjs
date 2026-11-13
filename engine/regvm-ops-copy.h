/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The rest of an object destructuring, `const {a, b, ...r} = s`: COPYR.
   Read once for each part, see regvm-areas.h.

   The parser emits it as OP_copy_rest (js_parse_destructuring_element1):
   the static keys of the pattern are a list of atoms, the keys of an entry
   of object_lits, and only computed keys go into excludeList, which is
   null without them. COPYR makes the target and copies into it what
   CopyDataProperties copies: the own enumerable keys of the source, less
   the excluded ones, in the order [[OwnPropertyKeys]] gives them.

   For a source that is an ordinary object (JS_CLASS_OBJECT) with no
   computed key left out, the shape of the source decides all of it: its
   keys, their order and whether each is an enumerable plain data property.
   When every key the rest copies is a string that is not an array index
   (the order of the shape), and a plain data property (no getter runs, so
   nothing can change the source during the copy), the result is an
   ordinary object of the keys copied, in that order, each enumerable,
   writable and configurable: the shape of an object literal of those keys
   (js_shape_of_keys), which defining them one by one leads to. It is made
   at that shape, its properties in one block of the final size, and the
   values copied with their references.

   Each COPYR has a site in rf->copy that keeps what a source of a shape
   gave: the shape of the result and the slot each value goes to, for up to
   RV_COPY_WAYS shapes. A hit is one compare of the shape and a copy. An
   entry holds a reference to the shape of the source, so that the shape
   does not change in place while it is there: an object of a shape held
   more than once copies it before it gets, loses or changes a key
   (add_property, js_shape_prepare_update), and only a shape of the shape
   hash table is kept, since one out of it is never shared. Only shapes whose
   prototype is null or the Object.prototype of the realm and whose keys are
   all strings are kept, so that an entry keeps nothing else alive: no
   prototype that a script dropped, no symbol a WeakRef could watch. The
   bytecode marks the shapes of its entries for the cycle collector
   (mark_children) and rv_free_func frees them. A full site replaces an
   entry on a miss RV_COPY_SWAPS times, then keeps the four it has: a site
   that sees many shapes in turn makes each result on the miss path, which
   allocates nothing more than the result and its shape. */

#if RV_PART == RV_PART_LIST
RV_OP(COPYR)    /* w offs8 lit16 site16: OP_copy_rest, the result in temp w */
#elif RV_PART == RV_PART_INFO
    case OP_copy_rest:
        return 0;
#elif RV_PART == RV_PART_PASS_FN
#elif RV_PART == RV_PART_PASS
    case OP_copy_rest:
        {
            /* the source and excludeList, named by the offsets byte as
               copy_data_properties names them with the target on top:
               offset o is the entry o below the top before the result */
            uint32_t v = get_u32(p + 1);
            int m = v & 0xff, os = (m >> 2) & 7, oe = (m >> 5) & 7;
            int n = max_int(os, oe), w;
            if ((m & 3) != 0 || os < 1 || oe < 1 || n > s->sp) {
                rv_crafted(s);
                break;
            }
            for (i = s->sp - n; i < s->sp; i++)
                rv_materialize(s, i);
            w = rv_tslot(s, s->sp);
            rv_emit(s, RV_COPYR);
            rv_byte(s, w);
            rv_byte(s, m);
            rv_u16(s, v >> 8);
            rv_u16(s, s->copy_n < 0xffff ? s->copy_n++ : 0xffff);
            rv_throws(s, rv_owned(s, 0));
            rv_push(s, RV_E_TEMP, w, 0);
        }
        break;
#elif RV_PART == RV_PART_RUN_FN
/* the object of the shape dst whose slot map[i] takes the value of slot i
   of ps, for the n slots of ps that map names (RV_COPY_NONE: none) */
static JSValue rv_copy_make(JSContext *ctx, JSShape *dst, const uint8_t *map,
                            uint32_t n, JSObject *ps)
{
    JSValue obj;
    JSObject *p;
    uint32_t i;

    if (likely(!dst->inline_grew &&
               offsetof(JSObject, u) + sizeof(JSProperty) * (size_t)dst->prop_size <=
               JS_OBJ_INLINE_MAX_SIZE))
        obj = js_new_object_inline(ctx, js_dup_shape(dst));
    else
        obj = JS_NewObjectFromShape(ctx, js_dup_shape(dst), JS_CLASS_OBJECT, NULL);
    if (unlikely(JS_IsException(obj)))
        return obj;
    p = JS_VALUE_GET_OBJ(obj);
    for (i = 0; i < n; i++) {
        if (map[i] != RV_COPY_NONE)
            p->prop[map[i]].u.value = JS_DupValue(ctx, ps->prop[i].u.value);
    }
    return obj;
}

/* the source ps, an ordinary object, had a shape its site had not seen:
   the result made from its shape, the shape kept in the site when it can
   be, or JS_UNDEFINED when the shape does not say the result (a key in
   another order than the shape's, or one that is not a plain data
   property), and the rest takes the general path */
static no_inline JSValue rv_copy_miss(JSContext *ctx, RVCopySite *site,
                                      JSObject *ps, const JSAtom *excl, int nexcl)
{
    JSRuntime *rt = ctx->rt;
    JSShape *sh = ps->shape, *dst;
    JSShapeProperty *prs = get_shape_prop(sh);
    JSObject *oproto = JS_VALUE_GET_OBJ(ctx->class_proto[JS_CLASS_OBJECT]);
    uint32_t i, j, n = sh->prop_count, v;
    JSAtom keys[RV_COPY_KEYS];
    uint8_t map[RV_COPY_KEYS];
    RVCopyEntry *e;
    JSValue obj;
    BOOL keep;

    if (n > RV_COPY_KEYS)
        return JS_UNDEFINED;
    keep = site && sh->is_hashed && (!sh->proto || sh->proto == oproto);
    for (i = j = 0; i < n; i++) {
        JSAtom atom = prs[i].atom;
        int kind = js_own_key_kind(rt, atom, &v);
        map[i] = RV_COPY_NONE;
        switch (kind) {
        case JS_OWN_KIND_NONE:
            /* a deleted key */
            continue;
        case JS_OWN_KIND_STRING:
            break;
        case JS_OWN_KIND_INDEX:
            /* listed before the strings, whatever its slot */
            return JS_UNDEFINED;
        default:
            /* a symbol or a private name: listed after the strings, or
               not at all. An entry would keep it alive */
            keep = FALSE;
            if (kind == JS_OWN_KIND_SYMBOL &&
                (prs[i].flags & JS_PROP_ENUMERABLE))
                return JS_UNDEFINED;
            continue;
        }
        if (!(prs[i].flags & JS_PROP_ENUMERABLE) ||
            js_copy_excluded(ctx, NULL, excl, nexcl, atom))
            continue;
        /* a getter, a setter, an autoinit or a binding runs code or reads
           another value, and a 'stack' key can hold the marker of an
           error stack not formatted yet (js_copy_shape) */
        if ((prs[i].flags & JS_PROP_TMASK) != JS_PROP_NORMAL ||
            atom == JS_ATOM_stack)
            return JS_UNDEFINED;
        keys[j] = atom;
        map[i] = j++;
    }
    dst = js_shape_of_keys(ctx, keys, j);
    if (!dst)
        return JS_EXCEPTION;
    if (keep) {
        for (i = 0; i < RV_COPY_WAYS && site->e[i]; i++)
            ;
        if (i == RV_COPY_WAYS) {
            if (site->swaps >= RV_COPY_SWAPS)
                keep = FALSE;
            else
                i = site->swaps++ % RV_COPY_WAYS;
        }
        /* no memory for an entry: the result is made all the same */
        e = keep ? js_malloc_rt(rt, sizeof(*e) + n) : NULL;
        if (e) {
            e->src = js_dup_shape(sh);
            e->dst = js_dup_shape(dst);
            e->n = n;
            memcpy(e->map, map, n);
            if (site->e[i])
                rv_copy_entry_free(rt, site->e[i]);
            site->e[i] = e;
        }
    }
    obj = rv_copy_make(ctx, dst, map, n, ps);
    js_free_shape(rt, dst);
    return obj;
}

/* COPYR: the source and excludeList by the offsets ip[1], the static keys
   left out in the object literal ip[2..3] of the function (0xffff: none),
   the site ip[4..5] (0xffff: none), the result into the temp of ip[0] */
static no_inline int rv_copy_rest(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue *sp = F + (int8_t)ip[0];
    int m = ip[1], k = get_u16(ip + 2), site = get_u16(ip + 4), nexcl = 0, i;
    JSValueConst src = sp[-((m >> 2) & 7)], excl = sp[-((m >> 5) & 7)];
    RVFunc *rf = ((RVHeader *)F)->rf;
    const JSAtom *atoms = NULL;
    RVCopySite *cs = NULL;
    JSValue obj;

    if (k != 0xffff) {
        const JSObjectLit *lit = &rf->b->object_lits[k];
        atoms = js_object_lit_atoms(rf->b, lit);
        nexcl = lit->count;
    }
    if (!RV_INJ_ARMED && JS_VALUE_GET_TAG(excl) == JS_TAG_NULL &&
        JS_VALUE_GET_TAG(src) == JS_TAG_OBJECT &&
        JS_VALUE_GET_OBJ(src)->class_id == JS_CLASS_OBJECT) {
        JSObject *ps = JS_VALUE_GET_OBJ(src);
        if (site != 0xffff) {
            cs = &rf->copy[site];
            for (i = 0; i < RV_COPY_WAYS && cs->e[i]; i++) {
                RVCopyEntry *e = cs->e[i];
                if (e->src == ps->shape) {
                    obj = rv_copy_make(ctx, e->dst, e->map, e->n, ps);
                    goto done;
                }
            }
        }
        obj = rv_copy_miss(ctx, cs, ps, atoms, nexcl);
        if (!JS_IsUndefined(obj))
            goto done;
    }
    if (RV_INJ_HIT(ctx))
        return -1;
    obj = JS_NewObject(ctx);
    if (JS_IsException(obj))
        return -1;
    if (js_copy_data_properties(ctx, obj, src, excl, atoms, nexcl, FALSE)) {
        JS_FreeValue(ctx, obj);
        return -1;
    }
 done:
    if (JS_IsException(obj))
        return -1;
    sp[0] = obj;
    return 0;
}
#elif RV_PART == RV_PART_RUN
 rv_COPYR:
    ip = pc;
    pc += 6;
    RV_SYNC_TOP();
    if (rv_copy_rest(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();
#endif
