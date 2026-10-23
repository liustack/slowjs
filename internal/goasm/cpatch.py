# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Give the C interpreter the fast paths fixlib writes into the Go one.

The generated interpreter replaces the whole Go JS_CallInternal, so the
changes internal/fixlib makes inside that function are lost unless the C
input has them too. Two of them change what the interpreter does, rather than
how Go compiles it, and are written here in C with the same cases:

  - fixFieldChain: get_field and get_field2 start the prototype loop at the
    prototype of a string or a number, and go on past a fast array that is
    not a typed array, for keys that JS_GetPropertyInternal would look for
    there anyway (lib/fieldchain.go).
  - fixEq: the usual pairs of ===, !==, == and != are answered without
    calling js_strict_eq_slow or js_eq_slow (internal/fixlib/eq.go).

The other interpreter rules of fixlib (cells, pin, inline, poll, oneframe,
alloca, expect, fastframes) work around Go's compiler and have no meaning
for LLVM. Every replacement must match exactly once.
"""
import pathlib,sys

HELPERS = r'''
/* goasm: see internal/goasm/cpatch.py */
static inline JSObject *goasm_field_proto(JSContext *ctx, JSValueConst v, JSAtom atom)
{
    int class_id;
    switch (JS_VALUE_GET_TAG(v)) {
    case JS_TAG_STRING:
    case JS_TAG_STRING_ROPE:
        if (__JS_AtomIsTaggedInt(atom) || atom == JS_ATOM_length)
            return NULL;
        class_id = JS_CLASS_STRING;
        break;
    case JS_TAG_INT:
    case JS_TAG_FLOAT64:
        class_id = JS_CLASS_NUMBER;
        break;
    default:
        return NULL;
    }
    return JS_VALUE_GET_OBJ(ctx->class_proto[class_id]);
}

static inline BOOL goasm_field_goes_on(JSObject *p, JSAtom atom)
{
    return p->fast_array && !__JS_AtomIsTaggedInt(atom) &&
        (p->class_id < JS_CLASS_UINT8C_ARRAY || p->class_id > JS_CLASS_FLOAT64_ARRAY);
}

static inline BOOL goasm_eq_simple_tag(int tag)
{
    return tag == JS_TAG_NULL || tag == JS_TAG_UNDEFINED || tag == JS_TAG_BOOL ||
        tag == JS_TAG_OBJECT || tag == JS_TAG_SYMBOL;
}

/* 1 or 0 when the pair is answered here, -1 for the slow function. */
static inline int goasm_eq_fast(JSValueConst a, JSValueConst b, BOOL strict)
{
    int ta = JS_VALUE_GET_TAG(a), tb = JS_VALUE_GET_TAG(b);
    if (strict) {
        if (ta == tb && (ta == JS_TAG_NULL || ta == JS_TAG_UNDEFINED))
            return 1;
    } else {
        BOOL na = ta == JS_TAG_NULL || ta == JS_TAG_UNDEFINED;
        BOOL nb = tb == JS_TAG_NULL || tb == JS_TAG_UNDEFINED;
        if (na && nb)
            return 1;
        if (na)
            return tb == JS_TAG_OBJECT && JS_VALUE_GET_OBJ(b)->is_HTMLDDA;
        if (nb)
            return ta == JS_TAG_OBJECT && JS_VALUE_GET_OBJ(a)->is_HTMLDDA;
    }
    if (ta == tb) {
        switch (ta) {
        case JS_TAG_OBJECT:
        case JS_TAG_SYMBOL:
            return JS_VALUE_GET_PTR(a) == JS_VALUE_GET_PTR(b);
        case JS_TAG_BOOL:
            return JS_VALUE_GET_INT(a) == JS_VALUE_GET_INT(b);
        case JS_TAG_STRING: {
            JSString *pa = JS_VALUE_GET_STRING(a), *pb = JS_VALUE_GET_STRING(b);
            if (pa == pb)
                return 1;
            if (pa->len != pb->len ||
                (pa->atom_type == JS_ATOM_TYPE_STRING && pb->atom_type == JS_ATOM_TYPE_STRING))
                return 0;
            return -1;
        }
        default:
            return -1;
        }
    }
    if (strict && (goasm_eq_simple_tag(ta) || goasm_eq_simple_tag(tb)))
        return 0;
    return -1;
}

static JSValue JS_CallInternal(JSContext *caller_ctx, JSValueConst func_obj,
                               JSValueConst this_obj, JSValueConst new_target,
                               int argc, JSValue *argv, int flags)
{'''

REPLACEMENTS = [
 # Helpers go right before the interpreter's definition.
 ('''static JSValue JS_CallInternal(JSContext *caller_ctx, JSValueConst func_obj,
                               JSValueConst this_obj, JSValueConst new_target,
                               int argc, JSValue *argv, int flags)
{''', HELPERS),
 ('''#define GET_FIELD_INLINE(name, keep, is_length)                         \\''',
  '''#define GET_FIELD_INLINE(name, keep, is_length, chain)                  \\'''),
 ('''                obj = sp[-1];                                           \\
                if (likely(JS_VALUE_GET_TAG(obj) == JS_TAG_OBJECT)) {   \\
                    p = JS_VALUE_GET_OBJ(obj);                          \\
                    for(;;) {                                           \\''',
  '''                obj = sp[-1];                                           \\
                if (likely(JS_VALUE_GET_TAG(obj) == JS_TAG_OBJECT) ||   \\
                    (chain && (p = goasm_field_proto(ctx, obj, atom)))) { \\
                    if (JS_VALUE_GET_TAG(obj) == JS_TAG_OBJECT)         \\
                        p = JS_VALUE_GET_OBJ(obj);                      \\
                    for(;;) {                                           \\'''),
 ('''                        if (unlikely(p->is_exotic)) {                   \\''',
  '''                        if (unlikely(p->is_exotic) &&                   \\
                            !(chain && goasm_field_goes_on(p, atom))) { \\'''),
 ('GET_FIELD_INLINE(get_field, 0, 0);','GET_FIELD_INLINE(get_field, 0, 0, 1);'),
 ('GET_FIELD_INLINE(get_field2, 1, 0);','GET_FIELD_INLINE(get_field2, 1, 0, 1);'),
 ('GET_FIELD_INLINE(get_length, 0, 1);','GET_FIELD_INLINE(get_length, 0, 1, 0);'),
 ('''            OP_CMP(OP_eq, ==, js_eq_slow(ctx, sp, 0));
            OP_CMP(OP_neq, !=, js_eq_slow(ctx, sp, 1));
            OP_CMP(OP_strict_eq, ==, js_strict_eq_slow(ctx, sp, 0));
            OP_CMP(OP_strict_neq, !=, js_strict_eq_slow(ctx, sp, 1));''',
  '''#define GOASM_OP_EQ(opcode, binary_op, slow_call, strict, neg)        \\
            CASE(opcode):                                       \\
                {                                               \\
                JSValue op1, op2;                               \\
                int res;                                        \\
                op1 = sp[-2];                                   \\
                op2 = sp[-1];                                   \\
                if (likely(JS_VALUE_IS_BOTH_INT(op1, op2))) {   \\
                    sp[-2] = JS_NewBool(ctx, JS_VALUE_GET_INT(op1) binary_op JS_VALUE_GET_INT(op2)); \\
                    sp--;                                       \\
                } else if ((res = goasm_eq_fast(op1, op2, strict)) >= 0) { \\
                    JS_FreeValue(ctx, op1);                     \\
                    JS_FreeValue(ctx, op2);                     \\
                    sp[-2] = JS_NewBool(ctx, res ^ neg);        \\
                    sp--;                                       \\
                } else {                                        \\
                    sf->cur_pc = pc;                            \\
                    if (slow_call)                              \\
                        goto exception;                         \\
                    sp--;                                       \\
                }                                               \\
                }                                               \\
            BREAK
            GOASM_OP_EQ(OP_eq, ==, js_eq_slow(ctx, sp, 0), 0, 0);
            GOASM_OP_EQ(OP_neq, !=, js_eq_slow(ctx, sp, 1), 0, 1);
            GOASM_OP_EQ(OP_strict_eq, ==, js_strict_eq_slow(ctx, sp, 0), 1, 0);
            GOASM_OP_EQ(OP_strict_neq, !=, js_strict_eq_slow(ctx, sp, 1), 1, 1);'''),
]

def patch(text):
    for old,new in REPLACEMENTS:
        n=text.count(old)
        if n!=1:raise ValueError('expected one match, found %d: %s'%(n,old.splitlines()[0]))
        text=text.replace(old,new)
    return text

if __name__=='__main__':
    pathlib.Path(sys.argv[2]).write_text(patch(pathlib.Path(sys.argv[1]).read_text()))
