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
  - fixBitwise: &, |, ^, <<, >> and >>> of two numbers that are not both
    ints, as bitwiseNumbers of lib/bitwise.go does at the entry of the slow
    functions, here before calling them.

Native C inlines JS_AtomToValue into the interpreter. Here it would be a
call into Go, so the string or symbol of an atom that is not an integer is
taken in C, as __JS_AtomToValue takes it.

goasmInterpreterRules in internal/fixlib/goasm.go lists every file of fixlib
that changes the Go interpreter and says what the generated one does instead:
a fast path written here, behavior the patched C already has, or a change
that only works around Go's compiler. TestGoasmCoversInterpreterRules fails
when a file has no row, or when a row names a change this file does not.
goasmHelperRules and TestGoasmCoversHelperRules do the same for the Go
copies of the helpers the interpreter compiles in (lib/goasm_helpers.txt).
Every replacement must match the number of times it gives.
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

static inline BOOL goasm_number_to_int32(JSValueConst v, int32_t *r)
{
    switch (JS_VALUE_GET_TAG(v)) {
    case JS_TAG_INT:
        *r = JS_VALUE_GET_INT(v);
        return TRUE;
    case JS_TAG_FLOAT64: {
        union { double d; uint64_t u; } x;
        int e;
        x.d = JS_VALUE_GET_FLOAT64(v);
        e = (x.u >> 52) & 0x7ff;
        if (e <= 1023 + 30) {
            *r = (int32_t)x.d;
        } else if (e <= 1023 + 30 + 53) {
            uint64_t m = ((x.u & (((uint64_t)1 << 52) - 1)) | ((uint64_t)1 << 52)) << (e - 1023 - 52 + 32);
            uint32_t y = (uint32_t)(m >> 32);
            *r = (int32_t)((x.u >> 63) ? 0u - y : y);
        } else {
            *r = 0; /* also NaN and the infinities */
        }
        return TRUE;
    }
    default:
        return FALSE;
    }
}

static inline BOOL goasm_bitwise_numbers(JSContext *ctx, JSValue *sp, int op)
{
    int32_t x, y, r;
    if (!goasm_number_to_int32(sp[-2], &x) || !goasm_number_to_int32(sp[-1], &y))
        return FALSE;
    switch (op) {
    case OP_shl: r = (int32_t)((uint32_t)x << (y & 0x1f)); break;
    case OP_sar: r = x >> (y & 0x1f); break;
    case OP_and: r = x & y; break;
    case OP_or: r = x | y; break;
    case OP_xor: r = x ^ y; break;
    case OP_shr: {
        uint32_t u = (uint32_t)x >> (y & 0x1f);
        if (u > INT32_MAX) {
            sp[-2] = __JS_NewFloat64(ctx, (double)u);
            return TRUE;
        }
        r = (int32_t)u;
        break;
    }
    default:
        return FALSE;
    }
    sp[-2] = JS_MKVAL(JS_TAG_INT, r);
    return TRUE;
}

static inline JSValue goasm_atom_to_value(JSContext *ctx, JSAtom atom)
{
    if (!__JS_AtomIsTaggedInt(atom)) {
        JSAtomStruct *p = ctx->rt->atom_array[atom];
        if (p->atom_type == JS_ATOM_TYPE_STRING)
            return JS_DupValue(ctx, JS_MKPTR(JS_TAG_STRING, p));
        return JS_DupValue(ctx, JS_MKPTR(JS_TAG_SYMBOL, p));
    }
    return JS_AtomToValue(ctx, atom);
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
 ('if (js_binary_logic_slow(ctx, sp, opcode))','if (!goasm_bitwise_numbers(ctx, sp, opcode) && js_binary_logic_slow(ctx, sp, opcode))',5),
 ('if (js_shr_slow(ctx, sp))','if (!goasm_bitwise_numbers(ctx, sp, OP_shr) && js_shr_slow(ctx, sp))'),
 ('*sp++ = JS_AtomToValue(ctx, get_u32(pc));','*sp++ = goasm_atom_to_value(ctx, get_u32(pc));'),
 ('*sp++ = JS_AtomToValue(ctx, atom);','*sp++ = goasm_atom_to_value(ctx, atom);',2),
]

def patch(text):
    for r in REPLACEMENTS:
        old,new=r[:2];want=r[2] if len(r)>2 else 1
        n=text.count(old)
        if n!=want:raise ValueError('expected %d matches, found %d: %s'%(want,n,old.splitlines()[0]))
        text=text.replace(old,new)
    return text

if __name__=='__main__':
    pathlib.Path(sys.argv[2]).write_text(patch(pathlib.Path(sys.argv[1]).read_text()))
