/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The other arithmetic in the register loop: mul, div, mod, pow, the bit
   ops, the unary ops, typeof and add_loc. Read once for each part, see
   regvm-areas.h. */

#if RV_PART == RV_PART_LIST
/* d a b|k, two ints here, the rest in rv_bin_other: the X form with b a
   slot, the XK form with b a constant. The order of the pairs is that of
   rv_bin_ops. */
RV_OP(MUL) RV_OP(MULK) RV_OP(DIV) RV_OP(DIVK) RV_OP(MOD) RV_OP(MODK)
RV_OP(AND) RV_OP(ANDK) RV_OP(OR) RV_OP(ORK) RV_OP(XOR) RV_OP(XORK)
RV_OP(SHL) RV_OP(SHLK) RV_OP(SAR) RV_OP(SARK) RV_OP(SHR) RV_OP(SHRK)
RV_OP(POW) RV_OP(POWK)
RV_OP(KSHL)     /* d k b: k << b, the constant on the left */
/* d a, the order of rv_un_ops. POSTINC and POSTDEC write d and d + 1. d
   is a temp, but for INC and DEC also a local (inc_loc, dec_loc), whose old
   value the slow side frees. */
RV_OP(NOT) RV_OP(NEG) RV_OP(PLUS) RV_OP(INC) RV_OP(DEC)
RV_OP(POSTINC) RV_OP(POSTDEC)
RV_OP(TYPEOF)   /* d a */
RV_OP(ADDLOC)   /* d v: local d += v */
RV_OP(ADDLOCK)  /* d k: local d += k */
#elif RV_PART == RV_PART_INFO
    case OP_mul: case OP_div: case OP_mod:
    case OP_and: case OP_or: case OP_xor: case OP_shl: case OP_sar:
    case OP_shr: case OP_pow:
    case OP_not: case OP_neg: case OP_plus: case OP_inc: case OP_dec:
    case OP_post_inc: case OP_post_dec: case OP_typeof: case OP_add_loc:
        return 0;
#elif RV_PART == RV_PART_PASS_FN
/* a binary op of rv_bin_ops: rop with b a slot, rop + 1 with b a constant.
   krop, when not -1, is the op for a constant on the left and a slot on
   the right (KSHL). */
static void rv_bin2(RVPass *s, int rop, int krop)
{
    int a, d, i = s->sp - 2;
    RVEnt eb;
    d = rv_tslot(s, i);
    if (krop >= 0 && s->st[i].kind == RV_E_K && s->st[i + 1].kind != RV_E_K) {
        rv_emit(s, krop);
        rv_byte(s, d);
        rv_byte(s, s->st[i].k);
        rv_byte(s, s->st[i + 1].slot);
    } else {
        a = rv_slot_of(s, i);
        eb = s->st[s->sp - 1];
        rv_emit(s, eb.kind == RV_E_K ? rop + 1 : rop);
        rv_byte(s, d);
        rv_byte(s, a);
        rv_byte(s, eb.kind == RV_E_K ? eb.k : eb.slot);
    }
    rv_throws(s, rv_owned(s, 2));
    s->sp -= 2;
    rv_push(s, RV_E_TEMP, d, 0);
    rv_mark_dst(s, d);
}

/* a unary op: d a, the result in the temp of the operand (and the one above
   it for POSTINC and POSTDEC) */
static void rv_un1(RVPass *s, int rop, int npush, BOOL throws)
{
    int a, d, i = s->sp - 1;
    a = rv_slot_of(s, i);
    d = rv_tslot(s, i);
    rv_emit(s, rop);
    rv_byte(s, d);
    rv_byte(s, a);
    if (throws)
        rv_throws(s, rv_owned(s, 1));
    s->sp--;
    rv_push(s, RV_E_TEMP, d, 0);
    if (npush == 2)
        rv_push(s, RV_E_TEMP, d + 1, 0);
    else
        rv_mark_dst(s, d);
}
#elif RV_PART == RV_PART_PASS
    case OP_mul: rv_bin2(s, RV_MUL, -1); break;
    case OP_div: rv_bin2(s, RV_DIV, -1); break;
    case OP_mod: rv_bin2(s, RV_MOD, -1); break;
    case OP_and: rv_bin2(s, RV_AND, -1); break;
    case OP_or: rv_bin2(s, RV_OR, -1); break;
    case OP_xor: rv_bin2(s, RV_XOR, -1); break;
    case OP_shl: rv_bin2(s, RV_SHL, RV_KSHL); break;
    case OP_sar: rv_bin2(s, RV_SAR, -1); break;
    case OP_shr: rv_bin2(s, RV_SHR, -1); break;
    case OP_pow: rv_bin2(s, RV_POW, -1); break;
    case OP_not: rv_un1(s, RV_NOT, 1, TRUE); break;
    case OP_neg: rv_un1(s, RV_NEG, 1, TRUE); break;
    case OP_plus: rv_un1(s, RV_PLUS, 1, TRUE); break;
    case OP_inc: rv_un1(s, RV_INC, 1, TRUE); break;
    case OP_dec: rv_un1(s, RV_DEC, 1, TRUE); break;
    case OP_post_inc: rv_un1(s, RV_POSTINC, 2, TRUE); break;
    case OP_post_dec: rv_un1(s, RV_POSTDEC, 2, TRUE); break;
    case OP_typeof: rv_un1(s, RV_TYPEOF, 1, FALSE); break;
    case OP_add_loc:
        /* a lazy read of the local still on the stack, the operand
           included, becomes a copy first */
        slot = rv_loc(s, p[1]);
        rv_before_store(s, slot);
        e = s->st[s->sp - 1];
        rv_emit(s, e.kind == RV_E_K ? RV_ADDLOCK : RV_ADDLOC);
        rv_byte(s, slot);
        rv_byte(s, e.kind == RV_E_K ? e.k : e.slot);
        rv_throws(s, rv_owned(s, 1));
        s->sp--;
        break;
#elif RV_PART == RV_PART_RUN_FN
static const uint8_t rv_bin_ops[] = {
    OP_mul, OP_div, OP_mod, OP_and, OP_or, OP_xor, OP_shl, OP_sar, OP_shr,
    OP_pow,
};
static const uint8_t rv_un_ops[] = {
    OP_not, OP_neg, OP_plus, OP_inc, OP_dec, OP_post_inc, OP_post_dec,
};

/* the slow side of the ops of rv_bin_ops, ip at d a b, d a k when k has
   1, d k b when it has 2 (KSHL): as the old loop's binary_arith_slow and the
   slow side of its bit ops */
static no_inline int rv_bin_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                 int op, int k)
{
    JSValue ops[2];
    if (k & 2)
        ops[0] = JS_DupValue(ctx, ((RVHeader *)F)->k[ip[1]]);
    else
        ops[0] = rv_take(ctx, F, (int8_t)ip[1]);
    if (k & 1)
        ops[1] = JS_DupValue(ctx, ((RVHeader *)F)->k[ip[2]]);
    else
        ops[1] = rv_take(ctx, F, (int8_t)ip[2]);
    switch (op) {
    case OP_mul: case OP_div: case OP_mod: case OP_pow:
        if (RV_INJ_HIT(ctx) || js_binary_arith_slow(ctx, ops + 2, op))
            goto fail;
        break;
    case OP_shr:
        if (RV_INJ_HIT(ctx) ||
            (!goasm_bitwise_numbers(ctx, ops + 2, op) && js_shr_slow(ctx, ops + 2)))
            goto fail;
        break;
    default:
        if (RV_INJ_HIT(ctx) ||
            (!goasm_bitwise_numbers(ctx, ops + 2, op) &&
             js_binary_logic_slow(ctx, ops + 2, op)))
            goto fail;
        break;
    }
    rv_put(ctx, F, (int8_t)ip[0], ops[0]);
    return 0;
 fail:
    JS_FreeValue(ctx, ops[0]);
    JS_FreeValue(ctx, ops[1]);
    return -1;
}

/* the slow side of the ops of rv_un_ops, ip at d a */
static no_inline int rv_un_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                int op)
{
    JSValue buf[2];
    int d = (int8_t)ip[0];
    buf[0] = rv_take(ctx, F, (int8_t)ip[1]);
    buf[1] = JS_UNDEFINED;
    switch (op) {
    case OP_not:
        if (RV_INJ_HIT(ctx) || js_not_slow(ctx, buf + 1))
            goto fail;
        break;
    case OP_post_inc: case OP_post_dec:
        if (RV_INJ_HIT(ctx) || js_post_inc_slow(ctx, buf + 1, op))
            goto fail;
        F[d + 1] = buf[1];
        break;
    default:
        if (RV_INJ_HIT(ctx) || js_unary_arith_slow(ctx, buf + 1, op))
            goto fail;
        break;
    }
    rv_put(ctx, F, d, buf[0]);
    return 0;
 fail:
    /* js_post_inc_slow leaves the number it made in buf[0] and undefined
       in buf[1] */
    JS_FreeValue(ctx, buf[0]);
    return -1;
}

/* typeof, ip at d a: a temp operand is freed */
static no_inline void rv_typeof(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    JSValue v = F[(int8_t)ip[1]];
    JSAtom atom = js_operator_typeof(ctx, v);
    if (rv_is_temp(F, (int8_t)ip[1]))
        JS_FreeValue(ctx, v);
    rv_put(ctx, F, (int8_t)ip[0], JS_AtomToString(ctx, atom));
}

/* the strings and the slow side of add_loc, ip at d v|k, as the old loop's
   OP_add_loc */
static no_inline int rv_addloc_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                    BOOL k)
{
    JSValue *pv = &F[(int8_t)ip[0]], op2;
    if (k)
        op2 = JS_DupValue(ctx, ((RVHeader *)F)->k[ip[1]]);
    else
        op2 = rv_take(ctx, F, (int8_t)ip[1]);
    if (RV_INJ_HIT(ctx)) {
        /* each way below takes the operand when it fails */
        JS_FreeValue(ctx, op2);
        return -1;
    }
    if (JS_VALUE_GET_TAG(*pv) == JS_TAG_STRING &&
        JS_VALUE_GET_TAG(op2) == JS_TAG_STRING) {
        if (JS_ConcatStringInPlace(ctx, JS_VALUE_GET_STRING(*pv), op2)) {
            JS_FreeValue(ctx, op2);
        } else {
            /* the in place append failed already, and would fail again
               with the reference of the dup */
            if (JS_VALUE_GET_STRING(*pv)->len >= JS_STRING_APPEND_MIN)
                op2 = JS_ConcatStringAppend(ctx, JS_DupValue(ctx, *pv), op2, FALSE);
            else
                op2 = JS_ConcatString(ctx, JS_DupValue(ctx, *pv), op2);
            if (JS_IsException(op2))
                return -1;
            set_value(ctx, pv, op2);
        }
    } else {
        JSValue ops[2];
        /* js_add_slow frees both on an exception: *pv is copied */
        ops[0] = JS_DupValue(ctx, *pv);
        ops[1] = op2;
        if (js_add_slow(ctx, ops + 2))
            return -1;
        set_value(ctx, pv, ops[0]);
    }
    return 0;
}
#elif RV_PART == RV_PART_RUN
    /* the ops of rv_bin_ops: two ints here, the rest in rv_bin_other,
       which finds the op from the opcode byte as rv_arith_other does */
#define RV_BIN(label, bexpr, ...)                                      \
 label:                                                                 \
    ip = pc; pc += 3;                                                   \
    {                                                                   \
        JSValue a = RV_R(ip[1]), bv = (bexpr);                          \
        if (likely(JS_VALUE_IS_BOTH_INT(a, bv)) && !RV_INJ_ARMED) {     \
            int32_t x = JS_VALUE_GET_INT(a), y = JS_VALUE_GET_INT(bv);  \
            __VA_ARGS__                                                 \
        }                                                               \
        goto rv_bin_other;                                              \
    }
#define RV_BIN_INT(e) { RV_PUTD(ip[0], JS_MKVAL(JS_TAG_INT, (e))); RV_NEXT(); }
#define RV_BIN_MUL {                                                    \
        int64_t r = (int64_t)x * y;                                     \
        /* -0 when one is negative and the product 0 */                 \
        if (likely((int)r == r && (r != 0 || (x | y) >= 0)))            \
            RV_BIN_INT((int)r)                                          \
    }
#define RV_BIN_DIV { RV_PUTD(ip[0], JS_NewFloat64(ctx, (double)x / (double)y)); RV_NEXT(); }
    /* not for 0, INT32_MIN % -1 and the results that are -0 */
#define RV_BIN_MOD { if (likely(x >= 0 && y > 0)) RV_BIN_INT(x % y) }
#define RV_BIN_PAIR(name, ...)                                          \
    RV_BIN(rv_##name, RV_R(ip[2]), __VA_ARGS__)                         \
    RV_BIN(rv_##name##K, RV_K(ip[2]), __VA_ARGS__)

    RV_BIN_PAIR(MUL, RV_BIN_MUL)
    RV_BIN_PAIR(DIV, RV_BIN_DIV)
    RV_BIN_PAIR(MOD, RV_BIN_MOD)
    RV_BIN_PAIR(AND, RV_BIN_INT(x & y))
    RV_BIN_PAIR(OR, RV_BIN_INT(x | y))
    RV_BIN_PAIR(XOR, RV_BIN_INT(x ^ y))
    RV_BIN_PAIR(SHL, RV_BIN_INT((int32_t)((uint32_t)x << (y & 0x1f))))
    RV_BIN_PAIR(SAR, RV_BIN_INT(x >> (y & 0x1f)))
    /* a float above INT32_MAX, as JS_NewUint32 makes it */
    RV_BIN_PAIR(SHR, { RV_PUTD(ip[0], JS_NewUint32(ctx, (uint32_t)x >> (y & 0x1f))); RV_NEXT(); })
#undef RV_BIN_PAIR
#undef RV_BIN_MOD
#undef RV_BIN_DIV
#undef RV_BIN_MUL
#undef RV_BIN_INT
#undef RV_BIN

    /* shl with the constant on the left (1 << x): ints here */
 rv_KSHL:
    ip = pc; pc += 3;
    {
        JSValue a = RV_K(ip[1]), bv = RV_R(ip[2]);
        if (likely(JS_VALUE_IS_BOTH_INT(a, bv)) && !RV_INJ_ARMED) {
            RV_PUTD(ip[0], JS_MKVAL(JS_TAG_INT, (int32_t)((uint32_t)JS_VALUE_GET_INT(a) <<
                                                          (JS_VALUE_GET_INT(bv) & 0x1f))));
            RV_NEXT();
        }
        RV_SYNC_TOP();
        if (rv_bin_slow(ctx, F, ip, OP_shl, 2) < 0)
            goto rv_exception;
    }
    RV_NEXT();

    /* pow: always the slow side, as the old loop's OP_pow */
 rv_POW:
 rv_POWK:
    ip = pc; pc += 3;
    goto rv_bin_other;

 rv_bin_other:
    /* mul of two numbers as the old loop's OP_mul, the rest in rv_bin_slow */
    n = ip[-1] - RV_MUL;
    if (n < 2 && !RV_INJ_ARMED) {
        JSValue a = RV_R(ip[1]), bv = n ? RV_K(ip[2]) : RV_R(ip[2]);
        uint32_t ta = JS_VALUE_GET_TAG(a), tb = JS_VALUE_GET_TAG(bv);
        if ((ta == JS_TAG_INT || JS_TAG_IS_FLOAT64(ta)) &&
            (tb == JS_TAG_INT || JS_TAG_IS_FLOAT64(tb))) {
            double d1 = ta == JS_TAG_INT ? JS_VALUE_GET_INT(a) : JS_VALUE_GET_FLOAT64(a);
            double d2 = tb == JS_TAG_INT ? JS_VALUE_GET_INT(bv) : JS_VALUE_GET_FLOAT64(bv);
            RV_PUTD(ip[0], __JS_NewFloat64(ctx, d1 * d2));
            RV_NEXT();
        }
    }
    RV_SYNC_TOP();
    if (rv_bin_slow(ctx, F, ip, rv_bin_ops[n >> 1], n & 1) < 0)
        goto rv_exception;
    RV_NEXT();

    /* the unary ops: ints (and the floats of neg and plus) here, the rest
       in rv_un_slow */
 rv_NOT:
    ip = pc; pc += 2;
    {
        JSValue v = RV_R(ip[1]);
        if (likely(JS_VALUE_GET_TAG(v) == JS_TAG_INT) && !RV_INJ_ARMED) {
            RV_PUTD(ip[0], JS_MKVAL(JS_TAG_INT, ~JS_VALUE_GET_INT(v)));
            RV_NEXT();
        }
    }
    goto rv_un_other;
 rv_NEG:
    ip = pc; pc += 2;
    {
        JSValue v = RV_R(ip[1]);
        uint32_t tag = JS_VALUE_GET_TAG(v);
        if (RV_INJ_ARMED)
            goto rv_un_other;
        if (tag == JS_TAG_INT || tag == JS_TAG_BOOL || tag == JS_TAG_NULL) {
            int val = JS_VALUE_GET_INT(v);
            /* -0 and -INT32_MIN are not ints */
            if (likely(val != 0 && val != INT32_MIN))
                RV_PUTD(ip[0], JS_MKVAL(JS_TAG_INT, -val));
            else
                RV_PUTD(ip[0], __JS_NewFloat64(ctx, -(double)val));
            RV_NEXT();
        }
        if (JS_TAG_IS_FLOAT64(tag)) {
            RV_PUTD(ip[0], __JS_NewFloat64(ctx, -JS_VALUE_GET_FLOAT64(v)));
            RV_NEXT();
        }
    }
    goto rv_un_other;
 rv_PLUS:
    ip = pc; pc += 2;
    {
        JSValue v = RV_R(ip[1]);
        uint32_t tag = JS_VALUE_GET_TAG(v);
        if (RV_INJ_ARMED)
            goto rv_un_other;
        if (tag == JS_TAG_INT || JS_TAG_IS_FLOAT64(tag)) {
            RV_PUTD(ip[0], v);
            RV_NEXT();
        }
        if (tag == JS_TAG_NULL || tag == JS_TAG_BOOL) {
            RV_PUTD(ip[0], JS_MKVAL(JS_TAG_INT, JS_VALUE_GET_INT(v)));
            RV_NEXT();
        }
    }
    goto rv_un_other;
#define RV_INCDEC(label, delta, limit, post)                            \
 label:                                                                 \
    ip = pc; pc += 2;                                                   \
    {                                                                   \
        JSValue v = RV_R(ip[1]);                                        \
        if (likely(JS_VALUE_GET_TAG(v) == JS_TAG_INT) &&                \
            likely(JS_VALUE_GET_INT(v) != (limit)) && !RV_INJ_ARMED) {  \
            int val = JS_VALUE_GET_INT(v) + (delta);                    \
            if (post) {                                                 \
                RV_R(ip[0] + 1) = JS_MKVAL(JS_TAG_INT, val);            \
                RV_R(ip[0]) = v;                                        \
            } else {                                                    \
                RV_PUTD(ip[0], JS_MKVAL(JS_TAG_INT, val));              \
            }                                                           \
            RV_NEXT();                                                  \
        }                                                               \
    }                                                                   \
    goto rv_un_other;
    RV_INCDEC(rv_INC, 1, INT32_MAX, 0)
    RV_INCDEC(rv_DEC, -1, INT32_MIN, 0)
    RV_INCDEC(rv_POSTINC, 1, INT32_MAX, 1)
    RV_INCDEC(rv_POSTDEC, -1, INT32_MIN, 1)
#undef RV_INCDEC
 rv_un_other:
    RV_SYNC_TOP();
    if (rv_un_slow(ctx, F, ip, rv_un_ops[ip[-1] - RV_NOT]) < 0)
        goto rv_exception;
    RV_NEXT();

 rv_TYPEOF:
    ip = pc; pc += 2;
    rv_typeof(ctx, F, ip);
    RV_NEXT();

    /* add_loc: ints and floats here, as the old loop's OP_add_loc, strings
       and the rest in rv_addloc_slow */
#define RV_ADDLOC(label, bexpr, isk)                                    \
 label:                                                                 \
    ip = pc; pc += 2;                                                   \
    {                                                                   \
        JSValue a = RV_R(ip[0]), bv = (bexpr);                          \
        if (RV_INJ_ARMED)                                               \
            goto label ## _slow;                                        \
        if (likely(JS_VALUE_IS_BOTH_INT(a, bv))) {                      \
            int64_t r = (int64_t)JS_VALUE_GET_INT(a) + JS_VALUE_GET_INT(bv); \
            if (likely((int)r == r))                                    \
                RV_R(ip[0]) = JS_MKVAL(JS_TAG_INT, (int)r);             \
            else                                                        \
                RV_R(ip[0]) = __JS_NewFloat64(ctx, (double)r);          \
            RV_NEXT();                                                  \
        }                                                               \
        if (JS_VALUE_IS_BOTH_FLOAT(a, bv)) {                            \
            RV_R(ip[0]) = __JS_NewFloat64(ctx, JS_VALUE_GET_FLOAT64(a) + \
                                          JS_VALUE_GET_FLOAT64(bv));    \
            RV_NEXT();                                                  \
        }                                                               \
        goto label ## _slow; /* the label is used in every build */    \
    label ## _slow:                                                     \
        RV_SYNC_TOP();                                                  \
        if (rv_addloc_slow(ctx, F, ip, isk))                            \
            goto rv_exception;                                          \
    }                                                                   \
    RV_NEXT();
    RV_ADDLOC(rv_ADDLOC, RV_R(ip[1]), FALSE)
    RV_ADDLOC(rv_ADDLOCK, RV_K(ip[1]), TRUE)
#undef RV_ADDLOC
#endif
