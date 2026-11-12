/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* The first ops of the register loop: moves, constants, add and sub, lt and
   strict_eq with their branches, jumps, fields, closure variables read,
   calls and the free and poll of a slot. Read once for each part, see
   regvm-areas.h. */

#if RV_PART == RV_PART_LIST
/* d: destination slot, a b s o v t f: operand slots, k: index into the
   constants, off: int16 from the end of the instruction, atom: u32. An L in
   the name: d is a local, its old value is freed. A K: the second operand
   is a constant. A T: the object operand is a temp, freed after. */
RV_OP(MOV)      /* d s       temp d = s, with a reference */
RV_OP(MOVL)     /* d s       local d = s, with a reference */
RV_OP(SETL)     /* d t       local d = t, moved */
RV_OP(LOADK)    /* d k */
RV_OP(LOADKL)   /* d k */
RV_OP(LOADKR)   /* d k       a constant with a reference count, which it takes */
RV_OP(LOADKRL)  /* d k */
RV_OP(ADD) RV_OP(ADDL) RV_OP(ADDK) RV_OP(ADDKL)       /* d a b|k */
RV_OP(SUB) RV_OP(SUBL) RV_OP(SUBK) RV_OP(SUBKL)       /* d a b|k */
RV_OP(KADD) RV_OP(KSUB)                               /* d k b: the constant on the left */
RV_OP(JF) RV_OP(JT)                                   /* s off */
RV_OP(GOTO)                                           /* off */
RV_OP(GETF) RV_OP(GETFT)                              /* d o atom */
RV_OP(GETFL)                                          /* d o atom: GETF into the local d */
RV_OP(PUTF)                                           /* o v flags atom */
RV_OP(GETVR)                                          /* d idx16 */
RV_OP(GETVRL)                                         /* d idx16: GETVR into the local d */
RV_OP(CALL)     /* w argc: func and arguments at w, this undefined */
RV_OP(CALLM)    /* w argc: the same, this the temp at w - 1 */
RV_OP(CALLML)   /* t w argc: the same, this the local t */
RV_OP(CALLD) RV_OP(CALLMD) RV_OP(CALLMLD) /* as CALL, CALLM and CALLML, the result dropped */
RV_OP(CALLS)    /* w argc k s..: CALL whose last k arguments it copies into the window from s */
RV_OP(CALLSD)   /* w argc k s..: the same, the result dropped */
RV_OP(RET) RV_OP(RETL) RV_OP(RETK) RV_OP(RETU)        /* t | s | k | -, any fofs, run in regvm-run.h */
RV_OP(FREE)                                           /* t */
RV_OP(POLL)
RV_OP(MOVT)                                           /* d s: temp d = temp s, moved, s left as it was */
RV_OP(SWAP)                                           /* a b: the values of temps a and b exchanged */
#elif RV_PART == RV_PART_INFO
    case OP_get_loc: case OP_put_loc: case OP_set_loc:
    case OP_get_arg: case OP_put_arg: case OP_set_arg:
    case OP_get_loc0: case OP_get_loc1: case OP_get_loc2: case OP_get_loc3:
    case OP_put_loc0: case OP_put_loc1: case OP_put_loc2: case OP_put_loc3:
    case OP_set_loc0: case OP_set_loc1: case OP_set_loc2: case OP_set_loc3:
    case OP_get_arg0: case OP_get_arg1: case OP_get_arg2: case OP_get_arg3:
    case OP_put_arg0: case OP_put_arg1: case OP_put_arg2: case OP_put_arg3:
    case OP_set_arg0: case OP_set_arg1: case OP_set_arg2: case OP_set_arg3:
    case OP_get_loc8: case OP_put_loc8: case OP_set_loc8:
    case OP_push_i32: case OP_push_i8: case OP_push_i16:
    case OP_push_minus1: case OP_push_0: case OP_push_1: case OP_push_2:
    case OP_push_3: case OP_push_4: case OP_push_5: case OP_push_6: case OP_push_7:
    case OP_undefined: case OP_null: case OP_push_true: case OP_push_false:
    case OP_add: case OP_sub:
    case OP_dup: case OP_nip: case OP_swap: case OP_insert2: case OP_insert3:
    case OP_perm3: case OP_rot3r:
    case OP_push_atom_value: case OP_push_const8: case OP_push_const:
    case OP_push_empty_string: case OP_push_bigint_i32:
    case OP_inc_loc: case OP_dec_loc:
    case OP_get_field: case OP_get_field2: case OP_get_length: case OP_put_field:
    case OP_get_loc0_get_field: case OP_get_loc8_get_field:
    case OP_get_arg0_get_field: case OP_get_arg1_get_field:
    case OP_get_var_ref0: case OP_get_var_ref1: case OP_get_var_ref2:
    case OP_get_var_ref3: case OP_get_var_ref:
    case OP_push_this: case OP_drop: case OP_nop:
        return 0;
    case OP_if_false: case OP_if_true:
        *target = rv_old_target(pos, 1 + (int64_t)(int32_t)get_u32(bc + pos + 1));
        return 0;
    case OP_if_false8: case OP_if_true8:
        *target = rv_old_target(pos, 1 + (int8_t)bc[pos + 1]);
        return 0;
    case OP_goto:
        *target = rv_old_target(pos, 1 + (int64_t)(int32_t)get_u32(bc + pos + 1));
        *term = 1;
        return 0;
    case OP_goto8:
        *target = rv_old_target(pos, 1 + (int8_t)bc[pos + 1]);
        *term = 1;
        return 0;
    case OP_goto16:
        *target = rv_old_target(pos, 1 + (int16_t)get_u16(bc + pos + 1));
        *term = 1;
        return 0;
    case OP_call0: case OP_call1: case OP_call2: case OP_call3: case OP_call:
        *npop = 1 + rv_old_argc(bc + pos, op);
        *npush = 1;
        return 0;
    case OP_call_method:
        *npop = 2 + rv_old_argc(bc + pos, op);
        *npush = 1;
        return 0;
    case OP_tail_call:
        *npop = 1 + rv_old_argc(bc + pos, op);
        *npush = 0;
        *term = 1;
        return 0;
    case OP_tail_call_method:
        *npop = 2 + rv_old_argc(bc + pos, op);
        *npush = 0;
        *term = 1;
        return 0;
    case OP_return: case OP_return_undef: case OP_return_async:
        *term = 1;
        return 0;
#elif RV_PART == RV_PART_PASS_FN
/* a binary op: add, sub (the result is a number or a string), lt, strict_eq */
static void rv_binop(RVPass *s, int op_rr, int op_rk, int op_kr)
{
    int a, d, i = s->sp - 2;
    RVEnt *eb = &s->st[s->sp - 1];
    d = rv_tslot(s, i);
    if (op_kr >= 0 && s->st[i].kind == RV_E_K && eb->kind != RV_E_K) {
        /* the constant on the left (KADD, KSUB), the result in its temp */
        rv_emit(s, op_kr);
        rv_byte(s, d);
        rv_byte(s, s->st[i].k);
        rv_byte(s, eb->slot);
        rv_throws(s, rv_owned(s, 2));
        s->sp -= 2;
        rv_push(s, RV_E_TEMP, d, 0);
        rv_mark_dst(s, d);
        return;
    }
    a = rv_slot_of(s, i);
    if (eb->kind == RV_E_K) {
        rv_emit(s, op_rk);
        rv_byte(s, d);
        rv_byte(s, a);
        rv_byte(s, eb->k);
    } else {
        rv_emit(s, op_rr);
        rv_byte(s, d);
        rv_byte(s, a);
        rv_byte(s, eb->slot);
    }
    rv_throws(s, rv_owned(s, 2));
    s->sp -= 2;
    rv_push(s, RV_E_TEMP, d, 0);
    rv_mark_dst(s, d);
}

/* CALLS and CALLSD: the last arguments of a call that the call copies into
   its window itself, k of them, at most RV_CALL_COPY_MAX. Each is an entry
   read lazily, a local, an argument or this (RV_E_SLOT): the MOV the call
   stands for would be made right before it, with nothing between, and the
   entry still holds the value of its slot then (rv_before_store copies it
   before a store to the slot, and a slot a closure can write is never read
   lazily, rv_push). The arguments local is copied as rv_materialize copies
   it (lazy_redo). A constant is not taken: telling a slot from a constant
   at each copy costs more than its LOADK saves. */
#define RV_CALL_COPY_MAX 4
static int rv_call_copies(RVPass *s, int first)
{
    int k, i;
    if (unlikely(s->fold))
        return 0;
    for (k = 0, i = s->sp - 1; i >= first && k < RV_CALL_COPY_MAX; i--, k++) {
        const RVEnt *e = &s->st[i];
        if (e->kind != RV_E_SLOT ||
            (unlikely(s->lazy_args) && e->slot == s->lazy_args))
            break;
        if (e->slot >= 0)
            rv_overflow(s); /* a slot of F, below the header */
    }
    return k;
}

/* a call: func and the arguments go into their temps, the window, so that
   the callee's frame starts there. A temp is there already, a local or a
   constant is put there by a MOV or a LOADK, or, when it is among the last
   arguments of a call that is not a method call, a local by the call
   (CALLS, see rv_call_copies). The this of a method call is the entry below func: a
   temp is moved, a local copied, by the call. When a drop or a
   return_undef that no jump reaches follows, the result is dropped by the
   call (CALLD, CALLSD, CALLMD, CALLMLD), and the drop only pops it
   (drop_at). */
static void rv_call(RVPass *s, int argc, int method, int tail)
{
    int i, n = argc + (method ? 2 : 1), base = s->sp - n, d, w, k;
    int f = method ? base + 1 : base;
    int q = s->pos + short_opcode_info(s->bc[s->pos]).size;
    int drop = !tail && q < s->len && !s->leader[q] &&
        (s->bc[q] == OP_drop || s->bc[q] == OP_return_undef);
    if (argc > 255) {
        /* argc is a byte of the call */
        rv_crafted(s);
        return;
    }
    k = method ? 0 : rv_call_copies(s, base + 1);
    for (i = f; i < s->sp - k; i++)
        rv_materialize(s, i);
    w = rv_tslot(s, f);
    d = rv_tslot(s, base);
    if (k) {
        rv_emit(s, drop ? RV_CALLSD : RV_CALLS);
        rv_byte(s, w);
        rv_byte(s, argc);
        rv_byte(s, k);
        for (i = s->sp - 1; i >= s->sp - k; i--)
            rv_byte(s, s->st[i].slot); /* the last argument first */
    } else if (!method) {
        rv_emit(s, drop ? RV_CALLD : RV_CALL);
        rv_byte(s, w);
        rv_byte(s, argc);
    } else if (s->st[base].kind == RV_E_SLOT) {
        rv_emit(s, drop ? RV_CALLMLD : RV_CALLML);
        rv_byte(s, s->st[base].slot);
        rv_byte(s, w);
        rv_byte(s, argc);
    } else {
        rv_materialize(s, base);
        rv_emit(s, drop ? RV_CALLMD : RV_CALLM);
        rv_byte(s, w);
        rv_byte(s, argc);
    }
    rv_throws(s, rv_owned(s, n));
    s->sp = base;
    if (tail) {
        /* what a switch or a loop left under the call: freed after it, as
           QuickJS's interpreter frees it with the frame once the call
           returns */
        for (i = 0; i < s->sp; i++) {
            if (s->st[i].kind == RV_E_TEMP) {
                rv_emit(s, RV_FREE);
                rv_byte(s, s->st[i].slot);
            }
        }
        rv_emit_ret(s, 0);
        rv_byte(s, d);
    } else {
        rv_push(s, RV_E_TEMP, d, 0);
        if (drop)
            s->drop_at = q;
    }
}
#elif RV_PART == RV_PART_PASS
    case OP_get_loc: case OP_get_arg:
    case OP_get_loc0: case OP_get_loc1: case OP_get_loc2: case OP_get_loc3:
    case OP_get_arg0: case OP_get_arg1: case OP_get_arg2: case OP_get_arg3:
    case OP_get_loc8:
    case OP_get_loc0_get_field: case OP_get_loc8_get_field:
    case OP_get_arg0_get_field: case OP_get_arg1_get_field:
        /* the fused heads are their read: the get_field after them is read
           next as an instruction of its own */
        slot = rv_local_slot(s, op, p);
        if (unlikely(s->lazy_args) && slot == s->lazy_args && !rv_lazy_fast(s, s->pos)) {
            rv_argmat(s);
            break;
        }
        rv_push(s, RV_E_SLOT, slot, 0);
        break;
    case OP_put_loc: case OP_put_arg:
    case OP_put_loc0: case OP_put_loc1: case OP_put_loc2: case OP_put_loc3:
    case OP_put_arg0: case OP_put_arg1: case OP_put_arg2: case OP_put_arg3:
    case OP_put_loc8:
        rv_store(s, rv_local_slot(s, op, p));
        break;
    case OP_set_loc: case OP_set_arg:
    case OP_set_loc0: case OP_set_loc1: case OP_set_loc2: case OP_set_loc3:
    case OP_set_arg0: case OP_set_arg1: case OP_set_arg2: case OP_set_arg3:
    case OP_set_loc8:
        slot = rv_local_slot(s, op, p);
        rv_store(s, slot);
        rv_push(s, RV_E_SLOT, slot, 0);
        break;
    case OP_push_i32:
        rv_push(s, RV_E_K, 0, rv_k(s, JS_NewInt32(NULL, get_u32(p + 1))));
        break;
    case OP_push_i8:
        rv_push(s, RV_E_K, 0, rv_k(s, JS_NewInt32(NULL, (int8_t)p[1])));
        break;
    case OP_push_i16:
        rv_push(s, RV_E_K, 0, rv_k(s, JS_NewInt32(NULL, (int16_t)get_u16(p + 1))));
        break;
    case OP_push_minus1: case OP_push_0: case OP_push_1: case OP_push_2:
    case OP_push_3: case OP_push_4: case OP_push_5: case OP_push_6: case OP_push_7:
        rv_push(s, RV_E_K, 0, rv_k(s, JS_NewInt32(NULL, op - OP_push_0)));
        break;
    case OP_undefined:
        rv_push(s, RV_E_K, 0, rv_k(s, JS_UNDEFINED));
        break;
    case OP_null:
        rv_push(s, RV_E_K, 0, rv_k(s, JS_NULL));
        break;
    case OP_push_true:
        rv_push(s, RV_E_K, 0, rv_k(s, JS_TRUE));
        break;
    case OP_push_false:
        rv_push(s, RV_E_K, 0, rv_k(s, JS_FALSE));
        break;
    case OP_add:
        if (rv_add_var_ref(s))
            break;
        rv_binop(s, RV_ADD, RV_ADDK, RV_KADD);
        break;
    case OP_sub:
        rv_binop(s, RV_SUB, RV_SUBK, RV_KSUB);
        break;
    case OP_push_atom_value:
        {
            JSAtom atom = get_u32(p + 1);
            JSAtomStruct *as;
            /* an index ("0") has no string: the parser puts it in the
               constants, and rv_operands_ok refused one here */
            if (__JS_AtomIsTaggedInt(atom)) {
                rv_overflow(s);
                break;
            }
            as = s->rt->atom_array[atom];
            rv_push(s, RV_E_K, 0, rv_k(s, JS_MKPTR(as->atom_type == JS_ATOM_TYPE_STRING ?
                                                   JS_TAG_STRING : JS_TAG_SYMBOL, as)));
        }
        break;
    case OP_push_empty_string:
        rv_push(s, RV_E_K, 0, rv_k(s, JS_MKPTR(JS_TAG_STRING,
                                               s->rt->atom_array[JS_ATOM_empty_string])));
        break;
    case OP_push_bigint_i32:
        /* a short BigInt, with no reference count */
        rv_push(s, RV_E_K, 0, rv_k(s, __JS_NewShortBigInt(NULL, (int)get_u32(p + 1))));
        break;
    case OP_push_const8:
        rv_push(s, RV_E_K, 0, rv_k(s, s->b->cpool[p[1]]));
        break;
    case OP_push_const:
        rv_push(s, RV_E_K, 0, rv_k(s, s->b->cpool[get_u32(p + 1)]));
        break;
    case OP_dup:
        {
            static const int8_t m[] = { 0, 0 };
            if (rv_dup_branch(s, pos))
                break;
            rv_shuffle(s, 1, m, 2);
        }
        break;
    case OP_nip:
        {
            static const int8_t m[] = { 1 };
            rv_shuffle(s, 2, m, 1);
        }
        break;
    case OP_swap:
        {
            static const int8_t m[] = { 1, 0 };
            rv_shuffle(s, 2, m, 2);
        }
        break;
    case OP_insert2: /* obj a -> a obj a */
        {
            static const int8_t m[] = { 1, 0, 1 };
            rv_shuffle(s, 2, m, 3);
        }
        break;
    case OP_insert3: /* obj prop a -> a obj prop a */
        {
            static const int8_t m[] = { 2, 0, 1, 2 };
            rv_shuffle(s, 3, m, 4);
        }
        break;
    case OP_perm3: /* obj a b -> a obj b */
        {
            static const int8_t m[] = { 1, 0, 2 };
            rv_shuffle(s, 3, m, 3);
        }
        break;
    case OP_rot3r: /* a b x -> x a b */
        {
            static const int8_t m[] = { 2, 0, 1 };
            rv_shuffle(s, 3, m, 3);
        }
        break;
    case OP_inc_loc: case OP_dec_loc:
        slot = rv_local_slot(s, op, p);
        rv_before_store(s, slot);
        /* ToNumeric(x) + 1, not x + 1: INC and DEC (regvm-ops-arith.h)
           with the local as d and a */
        rv_emit(s, op == OP_inc_loc ? RV_INC : RV_DEC);
        rv_byte(s, slot);
        rv_byte(s, slot);
        rv_throws(s, rv_owned(s, 0));
        break;
    case OP_if_false: case OP_if_true: case OP_if_false8: case OP_if_true8:
        {
            int target, jt = op == OP_if_true || op == OP_if_true8;
            if (op == OP_if_false || op == OP_if_true)
                target = pos + 1 + (int32_t)get_u32(p + 1);
            else
                target = pos + 1 + (int8_t)p[1];
            slot = rv_slot_of(s, s->sp - 1);
            rv_block_end(s, s->sp - 1);
            rv_emit(s, jt ? RV_JT : RV_JF);
            rv_byte(s, slot);
            rv_jump_to(s, target);
            rv_jump_throws(s, rv_owned(s, 1), pos, FALSE);
            s->sp--;
        }
        break;
    case OP_goto: case OP_goto8: case OP_goto16:
        {
            int target;
            if (op == OP_goto)
                target = pos + 1 + (int32_t)get_u32(p + 1);
            else if (op == OP_goto8)
                target = pos + 1 + (int8_t)p[1];
            else
                target = pos + 1 + (int16_t)get_u16(p + 1);
            rv_block_end(s, s->sp);
            rv_emit(s, RV_GOTO);
            rv_jump_to(s, target);
            rv_throws(s, rv_owned(s, 0));
        }
        break;
    case OP_get_field: case OP_get_length:
        if (op == OP_get_length && rv_is_lazy_args(s, s->sp - 1)) {
            rv_arglen(s);
            break;
        }
        /* a constant ("abc".length) goes to its temp first */
        if (s->st[s->sp - 1].kind == RV_E_K)
            rv_materialize(s, s->sp - 1);
        e = s->st[s->sp - 1];
        d = rv_tslot(s, s->sp - 1);
        rv_emit(s, e.kind == RV_E_TEMP ? RV_GETFT : RV_GETF);
        rv_byte(s, d);
        rv_byte(s, e.slot);
        rv_u32(s, op == OP_get_length ? JS_ATOM_length : get_u32(p + 1));
        rv_throws(s, rv_owned(s, 1));
        s->sp--;
        rv_push(s, RV_E_TEMP, d, 0);
        rv_mark_dst(s, d);
        break;
    case OP_get_field2:
        /* a constant ("a,b".split) goes to its temp first */
        if (s->st[s->sp - 1].kind == RV_E_K)
            rv_materialize(s, s->sp - 1);
        e = s->st[s->sp - 1];
        d = rv_tslot(s, s->sp);
        rv_emit(s, RV_GETF);
        rv_byte(s, d);
        rv_byte(s, e.slot);
        rv_u32(s, get_u32(p + 1));
        rv_throws(s, rv_owned(s, 0));
        rv_push(s, RV_E_TEMP, d, 0);
        rv_mark_dst(s, d);
        break;
    case OP_put_field:
        {
            RVEnt eo, ev;
            int flags = 0;
            /* a constant object ("abc".x = 1) goes to its temp first */
            if (s->st[s->sp - 2].kind == RV_E_K)
                rv_materialize(s, s->sp - 2);
            eo = s->st[s->sp - 2];
            ev = s->st[s->sp - 1];
            if (eo.kind == RV_E_TEMP)
                flags |= RV_PUT_OBJ_TEMP;
            if (ev.kind == RV_E_TEMP)
                flags |= RV_PUT_VAL_TEMP;
            if (ev.kind == RV_E_K)
                flags |= RV_PUT_VAL_K;
            rv_emit(s, RV_PUTF);
            rv_byte(s, eo.slot);
            rv_byte(s, ev.kind == RV_E_K ? ev.k : ev.slot);
            rv_byte(s, flags);
            rv_u32(s, get_u32(p + 1));
            rv_throws(s, rv_owned(s, 2));
            s->sp -= 2;
        }
        break;
    case OP_get_var_ref0: case OP_get_var_ref1: case OP_get_var_ref2:
    case OP_get_var_ref3: case OP_get_var_ref:
        d = rv_tslot(s, s->sp);
        rv_emit(s, RV_GETVR);
        rv_byte(s, d);
        rv_u16(s, op == OP_get_var_ref ? get_u16(p + 1) : op - OP_get_var_ref0);
        rv_push(s, RV_E_TEMP, d, 0);
        rv_mark_dst(s, d);
        break;
    case OP_push_this:
        /* a sloppy function coerces its this (THISC) */
        if (!(s->b->js_mode & JS_MODE_STRICT))
            rv_thisc(s);
        else
            rv_push(s, RV_E_SLOT, rv_this(s), 0);
        break;
    case OP_call0: case OP_call1: case OP_call2: case OP_call3: case OP_call:
        rv_call(s, rv_old_argc(p, op), 0, 0);
        break;
    case OP_call_method:
        rv_call(s, rv_old_argc(p, op), 1, 0);
        break;
    case OP_tail_call:
        rv_call(s, rv_old_argc(p, op), 0, 1);
        break;
    case OP_tail_call_method:
        rv_call(s, rv_old_argc(p, op), 1, 1);
        break;
    case OP_return_async: /* the end of a generator or async function
                             (regvm-ops-gen.h): a return of its entry frame */
    case OP_return:
        /* what a switch or a loop left under the value: QuickJS's interpreter
           frees it with the frame, here each temp is freed */
        for (i = 0; i < s->sp - 1; i++) {
            if (s->st[i].kind == RV_E_TEMP) {
                rv_emit(s, RV_FREE);
                rv_byte(s, s->st[i].slot);
            }
        }
        e = s->st[s->sp - 1];
        switch (e.kind) {
        case RV_E_TEMP:
            rv_emit_ret(s, 0);
            rv_byte(s, e.slot);
            break;
        case RV_E_SLOT:
            rv_emit_ret(s, 1);
            rv_byte(s, e.slot);
            break;
        case RV_E_K:
            if (rv_kref(s, e.k)) {
                /* RETK does not take a reference */
                rv_materialize(s, s->sp - 1);
                rv_emit_ret(s, 0);
                rv_byte(s, s->st[s->sp - 1].slot);
                break;
            }
            rv_emit_ret(s, 2);
            rv_byte(s, e.k);
            break;
        }
        s->sp = 0;
        break;
    case OP_return_undef:
        /* the top is a result its call dropped (rv_call) */
        if (s->pos == s->drop_at)
            s->sp--;
        for (i = 0; i < s->sp; i++) {
            if (s->st[i].kind == RV_E_TEMP) {
                rv_emit(s, RV_FREE);
                rv_byte(s, s->st[i].slot);
            }
        }
        s->sp = 0;
        rv_emit_ret(s, 3);
        break;
    case OP_drop:
        e = s->st[--s->sp];
        if (s->pos == s->drop_at)
            break; /* freed by its JFND, JTND or call */
        if (e.kind == RV_E_TEMP) {
            rv_emit(s, RV_FREE);
            rv_byte(s, e.slot);
        }
        break;
    case OP_nop:
        rv_emit(s, RV_POLL);
        rv_throws(s, rv_owned(s, 0));
        break;
#elif RV_PART == RV_PART_RUN_FN
/* the slow side of add, sub and the compares, ip at the operands: d a b,
   or d a k when k has 1, d k b when it has 2 (KADD, KSUB), a idx16 of a
   var ref when it has 4 (JSEQVF). Stores the result for arith, returns 0
   or 1 for a compare, -1 on an exception. For a compare and branch, d is
   absent. */
static no_inline int rv_binop_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                   int op, int k, BOOL has_d)
{
    JSValue ops[2];
    int d = 0, res;
    if (has_d)
        d = (int8_t)*ip++;
    if (k & 2)
        ops[0] = JS_DupValue(ctx, ((RVHeader *)F)->k[ip[0]]);
    else
        ops[0] = rv_take(ctx, F, (int8_t)ip[0]);
    if (k & 4)
        ops[1] = JS_DupValue(ctx, *RV_VREF(F, get_u16(ip + 1))->pvalue);
    else if (k & 1)
        ops[1] = JS_DupValue(ctx, ((RVHeader *)F)->k[ip[1]]);
    else
        ops[1] = rv_take(ctx, F, (int8_t)ip[1]);
    switch (op) {
    case OP_add:
    case OP_sub:
        {
            uint32_t t1 = JS_VALUE_GET_TAG(ops[0]), t2 = JS_VALUE_GET_TAG(ops[1]);
            if (!RV_INJ_ARMED &&
                (t1 == JS_TAG_INT || JS_TAG_IS_FLOAT64(t1)) &&
                (t2 == JS_TAG_INT || JS_TAG_IS_FLOAT64(t2))) {
                double d1 = t1 == JS_TAG_INT ? JS_VALUE_GET_INT(ops[0]) : JS_VALUE_GET_FLOAT64(ops[0]);
                double d2 = t2 == JS_TAG_INT ? JS_VALUE_GET_INT(ops[1]) : JS_VALUE_GET_FLOAT64(ops[1]);
                ops[0] = __JS_NewFloat64(ctx, op == OP_add ? d1 + d2 : d1 - d2);
            } else if (op == OP_add) {
                if (RV_INJ_HIT(ctx))
                    goto fail;
                if (JS_IsString(ops[0]) && JS_IsString(ops[1])) {
                    /* as QuickJS's OP_add: into a local or an
                       argument (d below 0, ADDL and ADDKL), the string of
                       d grows in place when it is the left operand */
                    if (has_d && !rv_is_temp(F, d))
                        ops[0] = js_add_string_into(ctx, ops[0], ops[1], &F[d]);
                    else
                        ops[0] = JS_ConcatString(ctx, ops[0], ops[1]);
                    if (JS_IsException(ops[0]))
                        return -1;
                } else if (js_add_slow(ctx, ops + 2)) {
                    goto fail;
                }
            } else {
                if (RV_INJ_HIT(ctx) || js_binary_arith_slow(ctx, ops + 2, op))
                    goto fail;
            }
            rv_put(ctx, F, d, ops[0]);
            return 0;
        }
    case OP_lt: case OP_lte: case OP_gt: case OP_gte:
        if (RV_INJ_HIT(ctx) || js_relational_slow(ctx, ops + 2, op))
            goto fail;
        break;
    case OP_eq: case OP_neq:
        if (RV_INJ_HIT(ctx) || js_eq_slow(ctx, ops + 2, op == OP_neq))
            goto fail;
        break;
    case OP_strict_eq: case OP_strict_neq:
        if (RV_INJ_HIT(ctx) || js_strict_eq_slow(ctx, ops + 2, op == OP_strict_neq))
            goto fail;
        break;
    default:
        abort();
    }
    res = JS_VALUE_GET_BOOL(ops[0]);
    if (has_d)
        rv_put(ctx, F, d, ops[0]);
    return res;
 fail:
    JS_FreeValue(ctx, ops[0]);
    JS_FreeValue(ctx, ops[1]);
    return -1;
}

/* the truth of a branch operand that is not an int or a bool: a temp is
   freed */
static no_inline int rv_tobool_slow(JSContext *ctx, JSValue *F, int slot)
{
    return JS_ToBoolFree(ctx, rv_take(ctx, F, slot));
}

/* the length of a string or a rope, or -1 for any other value */
static inline int rv_str_length(JSValueConst v)
{
    if (JS_VALUE_GET_TAG(v) == JS_TAG_STRING)
        return JS_VALUE_GET_STRING(v)->len;
    if (JS_VALUE_GET_TAG(v) == JS_TAG_STRING_ROPE)
        return JS_VALUE_GET_STRING_ROPE(v)->len;
    return -1;
}

/* the slow side of get_field: an object operand that is a temp is freed */
static no_inline int rv_getf_slow(JSContext *ctx, JSValue *F, const uint8_t *ip,
                                  BOOL temp)
{
    JSValue obj = F[(int8_t)ip[1]], val;
    if (RV_INJ_HIT(ctx))
        val = JS_EXCEPTION;
    else
        val = JS_GetPropertyInternal(ctx, obj, get_u32(ip + 2), obj, 0);
    if (temp)
        JS_FreeValue(ctx, obj);
    if (JS_IsException(val))
        return -1;
    rv_put(ctx, F, (int8_t)ip[0], val);
    return 0;
}

static no_inline int rv_putf_slow(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    int flags = ip[2], ret;
    JSValue obj = F[(int8_t)ip[0]], val;
    if (flags & RV_PUT_VAL_K)
        val = JS_DupValue(ctx, ((RVHeader *)F)->k[ip[1]]);
    else if (flags & RV_PUT_VAL_TEMP)
        val = F[(int8_t)ip[1]];
    else
        val = JS_DupValue(ctx, F[(int8_t)ip[1]]);
    if (RV_INJ_HIT(ctx)) {
        /* JS_SetPropertyInternal takes the value when it fails too */
        JS_FreeValue(ctx, val);
        ret = -1;
    } else {
        ret = JS_SetPropertyInternal(ctx, obj, get_u32(ip + 3), val, obj,
                                     rv_throw_flags(F));
    }
    if (flags & RV_PUT_OBJ_TEMP)
        JS_FreeValue(ctx, obj);
    return ret < 0 ? -1 : 0;
}

/* A call out of the loop of a C function that JS_NewCFunction3 marked
   (JSObject.rv_fast), answered here with what the C function returns, when
   its this and arguments are of the kinds for which it runs no script,
   throws nothing and allocates nothing. Returns the result, or
   JS_UNINITIALIZED to make the call, which gives the same result in the
   other cases.
   Function.prototype.call: hasOwnProperty.call(o, k), js_has_own_call.
   String.prototype.charCodeAt of a string, or of a rope that a call before
   made flat (js_linearize_string_rope keeps the flat string in it), at an
   index that is an int: the code unit at the index, NaN out of range. It
   needs none of what a call does: no level of call depth (a recursion
   through it only goes deeper), no count of a poll (the instructions of
   the loop around it are counted, see check_polls). Of the checks of a
   call only the one of an uncatchable exception pending stays, which makes
   the call fail. */
static no_inline JSValue rv_fast_call(JSContext *ctx, JSValueConst *W, int n,
                                      JSValueConst this_v)
{
    JSString *p;
    uint32_t idx;

    switch (JS_VALUE_GET_OBJ(W[0])->rv_fast) {
    case RV_FAST_CALL:
        return js_has_own_call(ctx, this_v, W + 1, n);
    case RV_FAST_CHAR_CODE_AT:
        /* the call fails while an uncatchable exception is pending
           (js_call_checks), so the code after it does not run */
        if (n < 1 || JS_VALUE_GET_TAG(W[1]) != JS_TAG_INT ||
            unlikely(js_uncatchable_pending(ctx->rt)))
            break;
        if (JS_VALUE_GET_TAG(this_v) == JS_TAG_STRING) {
            p = JS_VALUE_GET_STRING(this_v);
        } else if (JS_VALUE_GET_TAG(this_v) == JS_TAG_STRING_ROPE) {
            JSStringRope *r = JS_VALUE_GET_STRING_ROPE(this_v);
            if (JS_VALUE_GET_TAG(r->right) != JS_TAG_STRING ||
                JS_VALUE_GET_STRING(r->right)->len != 0 ||
                JS_VALUE_GET_TAG(r->left) != JS_TAG_STRING)
                break;
            p = JS_VALUE_GET_STRING(r->left);
        } else {
            break;
        }
        idx = JS_VALUE_GET_INT(W[1]);
        if (idx >= p->len)
            return JS_NAN;
        return JS_NewInt32(ctx, string_get(p, idx));
    }
    return JS_UNINITIALIZED;
}

/* CALLS w argc k s1 .. sk: the last k arguments of the call copied with a
   reference from the slots s1 .. sk of F, s1 into the last one, w + argc,
   s2 into the one before it, and so on. Unrolled, not a loop, and one test
   for the most common call, k = 1: each copy is the work of a MOV without
   its dispatch. */
static force_inline void rv_call_copy(JSContext *ctx, JSValue *F,
                                      const uint8_t *ip)
{
    JSValue *E = F + (int8_t)ip[0] + ip[1];
    int k = ip[2];
    E[0] = JS_DupValue(ctx, F[(int8_t)ip[3]]);
    if (k >= 2) {
        E[-1] = JS_DupValue(ctx, F[(int8_t)ip[4]]);
        if (k >= 3) {
            E[-2] = JS_DupValue(ctx, F[(int8_t)ip[5]]);
            if (k >= 4)
                E[-3] = JS_DupValue(ctx, F[(int8_t)ip[6]]);
        }
    }
}

/* TRUE when a call of f from the loop is one rv_unwrap_call takes apart:
   a bound function, or Function.prototype.call of this realm */
static inline BOOL rv_unwrap_kind(JSContext *ctx, JSValueConst f)
{
    JSObject *p;

    if (JS_VALUE_GET_TAG(f) != JS_TAG_OBJECT)
        return FALSE;
    p = JS_VALUE_GET_OBJ(f);
    if (p->class_id == JS_CLASS_BOUND_FUNCTION)
        return p->u.bound_function->argc == 0;
    return p->class_id == JS_CLASS_C_FUNCTION &&
        p->u.cfunc.cproto == JS_CFUNC_generic &&
        p->u.cfunc.c_function.generic == js_function_call &&
        p->u.cfunc.realm == ctx;
}

/* rv_unwrap_kind in RV_CALL_OUT, where is_call already tells whether f is
   Function.prototype.call (its rv_fast): a plain C function, most of
   the calls out of the loop, then pays one test of its class, not the
   tests of rv_unwrap_kind again (about 12 instructions a call, 0.8% of a
   render of tifa's MUI pages, whose calls out are mostly charCodeAt) */
static inline BOOL rv_unwrap_first(JSContext *ctx, JSValueConst f, int is_call)
{
    JSObject *p;

    if (is_call)
        return JS_VALUE_GET_OBJ(f)->u.cfunc.realm == ctx;
    if (JS_VALUE_GET_TAG(f) != JS_TAG_OBJECT)
        return FALSE;
    p = JS_VALUE_GET_OBJ(f);
    return p->class_id == JS_CLASS_BOUND_FUNCTION &&
        p->u.bound_function->argc == 0;
}

/* TRUE when f, what a call that was taken apart now calls, is one the loop
   calls as it would have been called: a function of bytecode of this realm,
   a plain C function, or one to take apart again. A proxy, a function of
   another realm, a C function with data and a value that is not a function
   stay behind the call, which calls or refuses them as before. */
static inline BOOL rv_unwrap_to(JSContext *ctx, JSValueConst f)
{
    JSObject *p;

    if (JS_VALUE_GET_TAG(f) != JS_TAG_OBJECT)
        return FALSE;
    p = JS_VALUE_GET_OBJ(f);
    if (p->class_id == JS_CLASS_BYTECODE_FUNCTION)
        return p->u.func.function_bytecode->realm == ctx;
    return p->class_id == JS_CLASS_C_FUNCTION || rv_unwrap_kind(ctx, f);
}

/* A call of the loop of a bound function with no bound arguments, or of
   Function.prototype.call, made as the call it stands for, in a loop over
   both, with no recursion: the window W (the function, then n arguments)
   and this_v become those of the function they call, the values moved
   with their references. Returns the new n, and the new this in
   rt->rv_unwrap_this, or -1 when there was nothing to take apart. Such a
   call then does not count a level of call depth or poll for the wrappers,
   and leaves no frame of Function.prototype.call ("at call (native)") in a
   stack trace, as in V8. new.target is not involved: NEW does not come
   here. */
static no_inline int rv_unwrap_call(JSContext *ctx, JSValue *W, int n, JSValue this_v)
{
    int done = 0;

    for (;;) {
        JSValue f = W[0], t;
        JSObject *p;

        /* a stop asked while a long chain is taken apart leaves the rest to
           the call out of the loop, which polls and throws it */
        if (!rv_unwrap_kind(ctx, f) || unlikely(poll_flag(&ctx->rt->interrupt_flag)))
            break;
        p = JS_VALUE_GET_OBJ(f);
        if (p->class_id == JS_CLASS_BOUND_FUNCTION) {
            JSBoundFunction *bf = p->u.bound_function;
            if (!rv_unwrap_to(ctx, bf->func_obj))
                break;
            W[0] = JS_DupValue(ctx, bf->func_obj);
            t = this_v;
            this_v = JS_DupValue(ctx, bf->this_val);
            JS_FreeValue(ctx, f);
            JS_FreeValue(ctx, t);
        } else {
            /* f is call: what it calls is the this of the call */
            if (!rv_unwrap_to(ctx, this_v))
                break;
            W[0] = this_v;
            if (n == 0) {
                this_v = JS_UNDEFINED;
            } else {
                this_v = W[1];
                memmove(W + 1, W + 2, sizeof(W[0]) * (n - 1));
                W[n] = JS_UNDEFINED;
                n--;
            }
            JS_FreeValue(ctx, f);
        }
        done = 1;
    }
    if (!done)
        return -1;
    ctx->rt->rv_unwrap_this = this_v;
    return n;
}

/* TRUE when f, what rv_unwrap_call left to call, is a converted function
   (of this realm, as rv_unwrap_to took it): the loop calls it in a frame
   of its own (rv_call_args), as the call of it from the loop would */
static inline BOOL rv_unwrap_in_loop(JSValueConst f)
{
    JSObject *p;

    if (JS_VALUE_GET_TAG(f) != JS_TAG_OBJECT)
        return FALSE;
    p = JS_VALUE_GET_OBJ(f);
    return p->class_id == JS_CLASS_BYTECODE_FUNCTION &&
        p->u.func.function_bytecode->rv->arg_count >= 0;
}
#elif RV_PART == RV_PART_RUN
    /* moves */
 rv_MOV:
    ip = pc; pc += 2;
    RV_R(ip[0]) = JS_DupValue(ctx, RV_R(ip[1]));
    RV_NEXT();
 rv_MOVL:
    ip = pc; pc += 2;
    {
        JSValue v = JS_DupValue(ctx, RV_R(ip[1])), old = RV_R(ip[0]);
        RV_R(ip[0]) = v;
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();
 rv_SETL:
    ip = pc; pc += 2;
    {
        JSValue old = RV_R(ip[0]);
        RV_R(ip[0]) = RV_R(ip[1]);
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();
    /* LOADK and LOADKL: a constant with no reference count */
 rv_LOADK:
    ip = pc; pc += 2;
    RV_R(ip[0]) = RV_K(ip[1]);
    RV_NEXT();
 rv_LOADKL:
    ip = pc; pc += 2;
    {
        JSValue old = RV_R(ip[0]);
        RV_R(ip[0]) = RV_K(ip[1]);
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();
 rv_LOADKR:
    ip = pc; pc += 2;
    RV_R(ip[0]) = JS_DupValue(ctx, RV_K(ip[1]));
    RV_NEXT();
 rv_LOADKRL:
    ip = pc; pc += 2;
    {
        JSValue old = RV_R(ip[0]);
        RV_R(ip[0]) = JS_DupValue(ctx, RV_K(ip[1]));
        JS_FreeValue(ctx, old);
    }
    RV_NEXT();
    /* the shuffles of the stack ops between temps: no reference changes */
 rv_MOVT:
    ip = pc; pc += 2;
    RV_R(ip[0]) = RV_R(ip[1]);
    RV_NEXT();
 rv_SWAP:
    ip = pc; pc += 2;
    {
        JSValue v = RV_R(ip[0]);
        RV_R(ip[0]) = RV_R(ip[1]);
        RV_R(ip[1]) = v;
    }
    RV_NEXT();

    /* add and sub: two ints with an int result here, the rest in
       rv_arith_other, shared by the eight of them, so that the code of
       each stays as short as the int case needs */
#define RV_ARITH(label, cop, bexpr, isloc)                              \
 label:                                                                 \
    ip = pc; pc += 3;                                                   \
    {                                                                   \
        JSValue a = RV_R(ip[1]), bv = (bexpr);                          \
        if (likely(JS_VALUE_IS_BOTH_INT(a, bv)) && !RV_INJ_ARMED) {     \
            int64_t r = (int64_t)JS_VALUE_GET_INT(a) cop JS_VALUE_GET_INT(bv); \
            if (likely((int)r == r)) {                                  \
                if (isloc) {                                            \
                    JSValue old = RV_R(ip[0]);                          \
                    RV_R(ip[0]) = JS_MKVAL(JS_TAG_INT, (int)r);         \
                    JS_FreeValue(ctx, old);                             \
                } else {                                                \
                    RV_R(ip[0]) = JS_MKVAL(JS_TAG_INT, (int)r);         \
                }                                                       \
                RV_NEXT();                                              \
            }                                                           \
        }                                                               \
        goto rv_arith_other;                                            \
    }

    RV_ARITH(rv_ADD, +, RV_R(ip[2]), FALSE)
    RV_ARITH(rv_ADDL, +, RV_R(ip[2]), TRUE)
    RV_ARITH(rv_ADDK, +, RV_K(ip[2]), FALSE)
    RV_ARITH(rv_ADDKL, +, RV_K(ip[2]), TRUE)
    RV_ARITH(rv_SUB, -, RV_R(ip[2]), FALSE)
    RV_ARITH(rv_SUBL, -, RV_R(ip[2]), TRUE)
    RV_ARITH(rv_SUBK, -, RV_K(ip[2]), FALSE)
    RV_ARITH(rv_SUBKL, -, RV_K(ip[2]), TRUE)

    /* add and sub with the constant on the left (1 + x, "<" + x, 31 - x):
       two ints with an int result here, the rest in rv_binop_slow */
#define RV_KARITH(label, cop, opc)                                      \
 label:                                                                 \
    ip = pc; pc += 3;                                                   \
    {                                                                   \
        JSValue a = RV_K(ip[1]), bv = RV_R(ip[2]);                      \
        if (likely(JS_VALUE_IS_BOTH_INT(a, bv)) && !RV_INJ_ARMED) {     \
            int64_t r = (int64_t)JS_VALUE_GET_INT(a) cop JS_VALUE_GET_INT(bv); \
            if (likely((int)r == r)) {                                  \
                RV_PUTD(ip[0], JS_MKVAL(JS_TAG_INT, (int)r));           \
                RV_NEXT();                                              \
            }                                                           \
        }                                                               \
        RV_SYNC_TOP();                                                  \
        if (rv_binop_slow(ctx, F, ip, opc, 2, TRUE) < 0)                \
            goto rv_exception;                                          \
    }                                                                   \
    RV_NEXT();
    RV_KARITH(rv_KADD, +, OP_add)
    RV_KARITH(rv_KSUB, -, OP_sub)
#undef RV_KARITH

    /* the rest of add and sub, as QuickJS's OP_add and OP_sub: a float
       of two ints that overflow, or of an int or a float with a float, else
       rv_binop_slow. Which of the eight it is comes from the opcode byte
       again (arith: 1 a local destination, 2 a constant operand, 4 sub),
       not from a value each handler sets: a value set on the way out made
       LLVM keep it, and pc, in registers through the int case of all
       eight. */
 rv_arith_other:
    arith = ip[-1] - RV_ADD;
    {
        JSValue a = RV_R(ip[1]), bv = (arith & 2) ? RV_K(ip[2]) : RV_R(ip[2]);
        uint32_t ta = JS_VALUE_GET_TAG(a), tb = JS_VALUE_GET_TAG(bv);
        double d1, d2;
        if (!RV_INJ_ARMED &&
            (ta == JS_TAG_INT || JS_TAG_IS_FLOAT64(ta)) &&
            (tb == JS_TAG_INT || JS_TAG_IS_FLOAT64(tb))) {
            d1 = ta == JS_TAG_INT ? JS_VALUE_GET_INT(a) : JS_VALUE_GET_FLOAT64(a);
            d2 = tb == JS_TAG_INT ? JS_VALUE_GET_INT(bv) : JS_VALUE_GET_FLOAT64(bv);
            ret_val = __JS_NewFloat64(ctx, (arith & 4) ? d1 - d2 : d1 + d2);
            if (arith & 1) {
                JSValue old = RV_R(ip[0]);
                RV_R(ip[0]) = ret_val;
                JS_FreeValue(ctx, old);
            } else {
                RV_R(ip[0]) = ret_val;
            }
            RV_NEXT();
        }
        RV_SYNC_TOP();
        if (rv_binop_slow(ctx, F, ip, (arith & 4) ? OP_sub : OP_add, (arith >> 1) & 1, TRUE) < 0)
            goto rv_exception;
    }
    RV_NEXT();

#define RV_JBOOL(label, jt)                                             \
 label:                                                                 \
    ip = pc; pc += 3;                                                   \
    {                                                                   \
        JSValue v = RV_R(ip[0]);                                        \
        int res;                                                        \
        if ((uint32_t)JS_VALUE_GET_TAG(v) <= JS_TAG_UNDEFINED) {        \
            res = JS_VALUE_GET_INT(v) != 0;                             \
        } else {                                                        \
            RV_SYNC_TOP();                                              \
            res = rv_tobool_slow(ctx, F, (int8_t)ip[0]);                \
            RV_JUMP_SLOW(res == (jt), (int16_t)get_u16(ip + 1), 0);     \
        }                                                               \
        RV_JUMP_POLL(res == (jt), 0);                                   \
        if (res == (jt)) {                                              \
            int off = (int16_t)get_u16(ip + 1);                         \
            if (off < 0)                                                \
                poll_count(POLL_WEIGHT);                                \
            pc += off;                                                  \
        }                                                               \
    }                                                                   \
    RV_NEXT();

    RV_JBOOL(rv_JF, 0)
    RV_JBOOL(rv_JT, 1)

 rv_GOTO:
    ip = pc; pc += 2;
    {
        int off = (int16_t)get_u16(ip);
        if (RV_INTERRUPTED())
            goto rv_exception;
        if (off < 0)
            poll_count(POLL_WEIGHT);
        pc += off;
    }
    RV_NEXT();

    /* get_field: a data property found with nothing run, as
       GET_LOCAL_FIELD of JS_CallInternal, else rv_getf_slow. GETF writes a
       temp, which holds nothing to free (the pass gives each result a
       temp no entry owns), GETFL a local (rv_store) and GETFT either. */
#define RV_GETFIELD(label, temp, tdst)                                  \
 label:                                                                 \
    ip = pc; pc += 6;                                                   \
    {                                                                   \
        JSValue obj = RV_R(ip[1]), val;                                 \
        JSAtom atom = get_u32(ip + 2);                                  \
        JSProperty *pr;                                                 \
        JSShapeProperty *prs;                                           \
        if (likely(JS_VALUE_GET_TAG(obj) == JS_TAG_OBJECT))             \
            p = JS_VALUE_GET_OBJ(obj);                                  \
        else                                                            \
            p = goasm_field_proto(ctx, obj, atom);                      \
        if (likely(p) && !RV_INJ_ARMED) {                               \
            for(;;) {                                                   \
                prs = find_own_property(&pr, p, atom);                  \
                if (prs) {                                              \
                    if (unlikely(prs->flags & JS_PROP_TMASK))           \
                        goto label ## _slow;                            \
                    if (atom == JS_ATOM_stack &&                        \
                        JS_VALUE_GET_TAG(pr->u.value) == JS_TAG_OBJECT) \
                        goto label ## _slow;                            \
                    val = JS_DupValue(ctx, pr->u.value);                \
                    break;                                              \
                }                                                       \
                if (unlikely(p->is_exotic) &&                           \
                    !goasm_field_goes_on(p, atom))                      \
                    goto label ## _slow;                                \
                p = p->shape->proto;                                    \
                if (!p) {                                               \
                    val = JS_UNDEFINED;                                 \
                    break;                                              \
                }                                                       \
            }                                                           \
            if (tdst)                                                   \
                RV_R(ip[0]) = val;                                      \
            else                                                        \
                RV_PUTD(ip[0], val);                                    \
            if (temp)                                                   \
                JS_FreeValue(ctx, obj);                                 \
            RV_NEXT();                                                  \
        }                                                               \
        /* the length of a string, which has no shape to look in */     \
        if (atom == JS_ATOM_length && !RV_INJ_ARMED) {                  \
            int n_ = rv_str_length(obj);                                \
            if (n_ >= 0) {                                              \
                RV_PUTD(ip[0], JS_NewInt32(ctx, n_));                   \
                if (temp)                                               \
                    JS_FreeValue(ctx, obj);                             \
                RV_NEXT();                                              \
            }                                                           \
        }                                                               \
    label ## _slow:                                                     \
        RV_SYNC_TOP();                                                  \
        if (rv_getf_slow(ctx, F, ip, temp))                             \
            goto rv_exception;                                          \
    }                                                                   \
    RV_NEXT();

    RV_GETFIELD(rv_GETF, FALSE, TRUE)
    RV_GETFIELD(rv_GETFL, FALSE, FALSE)
    RV_GETFIELD(rv_GETFT, TRUE, FALSE)

 rv_PUTF:
    ip = pc; pc += 7;
    {
        int flags = ip[2];
        JSValue obj = RV_R(ip[0]);
        JSProperty *pr;
        JSShapeProperty *prs;
        if (likely(JS_VALUE_GET_TAG(obj) == JS_TAG_OBJECT) && !RV_INJ_ARMED) {
            p = JS_VALUE_GET_OBJ(obj);
            prs = find_own_property(&pr, p, get_u32(ip + 3));
            if (prs && likely((prs->flags & (JS_PROP_TMASK | JS_PROP_WRITABLE |
                                             JS_PROP_LENGTH)) == JS_PROP_WRITABLE)) {
                JSValue val;
                if (flags & RV_PUT_VAL_K)
                    val = JS_DupValue(ctx, RV_K(ip[1]));
                else if (flags & RV_PUT_VAL_TEMP)
                    val = RV_R(ip[1]);
                else
                    val = JS_DupValue(ctx, RV_R(ip[1]));
                set_value(ctx, &pr->u.value, val);
                if (flags & RV_PUT_OBJ_TEMP)
                    JS_FreeValue(ctx, obj);
                RV_NEXT();
            }
        }
        RV_SYNC_TOP();
        if (rv_putf_slow(ctx, F, ip))
            goto rv_exception;
    }
    RV_NEXT();

    /* GETVR writes a temp, which holds nothing to free, GETVRL a local */
 rv_GETVR:
    ip = pc; pc += 3;
    RV_R(ip[0]) = JS_DupValue(ctx, *RV_VREF(F, get_u16(ip + 1))->pvalue);
    RV_NEXT();
 rv_GETVRL:
    ip = pc; pc += 3;
    RV_PUTD(ip[0], JS_DupValue(ctx, *RV_VREF(F, get_u16(ip + 1))->pvalue));
    RV_NEXT();

 rv_FREE:
    ip = pc; pc += 1;
    JS_FreeValue(ctx, RV_R(ip[0]));
    RV_NEXT();

 rv_POLL:
    poll_count(POLL_WEIGHT);
    if (RV_INTERRUPTED())
        goto rv_exception;
    RV_NEXT();

    /* call: w argc, func and the arguments at w, where the old stack has
       them. this is undefined (CALL), the temp at w - 1, moved (CALLM), or
       the local t of CALLML t w argc, copied with a reference. The result
       goes to the first slot of the call: w, or w - 1 with a this. A
       converted function of the same realm with no more arguments than
       parameters gets its frame from w, anything else is called as QuickJS's
       interpreter calls it.

       The poll comes first, while only F and pc are live: values live
       across its call to the host would be kept on the stack, on the way
       of every call. A call out of the loop counts it again in
       JS_CallInternal, which only polls sooner. The first call of a
       function not yet converted converts it and runs the instruction
       again (CALLS goes on after its copies). */
#define RV_CALLOP(label, len, dstexpr, thisexpr, args, seg, out)             \
 label:                                                                 \
    poll_count(POLL_WEIGHT);                                            \
    ip = pc + (len) - 2;                                                \
    pc += (len);                                                        \
    RV_CALL_BODY(label, dstexpr, args, seg, out,                        \
                 pc -= (len) + 1; RV_NEXT(), thisexpr)
/* thisexpr last, as JS_UNDEFINED has commas */
#define RV_CALL_BODY(label, dstexpr, args, seg, out, again, ...)        \
    {                                                                   \
        int argc1 = ip[1];                                              \
        JSValue fv, *Wc = F + (int8_t)ip[0];                            \
        dst = (dstexpr);                                                \
        fv = Wc[0];                                                     \
        /* the throw-injection test makes the call out, where it fails */ \
        if (likely(JS_VALUE_GET_TAG(fv) == JS_TAG_OBJECT) && !RV_INJ_ARMED) { \
            JSFunctionBytecode *cb;                                     \
            RVFunc *crf;                                                \
            JSValue *Fn;                                                \
            p = JS_VALUE_GET_OBJ(fv);                                   \
            if (unlikely(p->class_id != JS_CLASS_BYTECODE_FUNCTION))    \
                goto label ## _out;                                     \
            cb = p->u.func.function_bytecode;                           \
            crf = cb->rv;                                               \
            /* not converted yet, refused, more arguments than          \
               parameters, or a function that reads its arguments */    \
            if (unlikely(argc1 > crf->argmax)) {                        \
                if (crf->arg_count >= 0) {                              \
                    this_v = (__VA_ARGS__);                             \
                    W = Wc;                                             \
                    n = argc1;                                          \
                    goto args;                                          \
                }                                                       \
                if (crf != RV_UNCONVERTED)                              \
                    goto label ## _out;                                 \
                RV_SYNC_TOP();                                          \
                /* left unconverted: out of memory, thrown by the  */   \
                /* call out (JS_CallInternal) */                        \
                if (rv_convert(ctx, cb) == RV_UNCONVERTED)              \
                    goto label ## _out;                                 \
                again;                                                  \
            }                                                           \
            if (unlikely(cb->realm != ctx))                             \
                goto label ## _out;                                     \
            Fn = (JSValue *)((uint8_t *)Wc + crf->fofs_b);              \
            if (unlikely((uint8_t *)Wc + crf->room_b >                  \
                         (uint8_t *)rt->rv_end))                        \
                goto label ## _seg;                                     \
            /* the checks of the entry of JS_CallInternal */            \
            if (unlikely(poll_flag(&rt->interrupt_flag)) ||             \
                unlikely(js_uncatchable_pending(rt)) ||                 \
                unlikely(rt->call_depth >= rt->max_call_depth))         \
                goto label ## _out;                                     \
            rt->call_depth++;                                           \
            if (unlikely(argc1 != crf->arg_count))                      \
                rv_undefined(Wc + 1 + argc1, crf->arg_count - argc1);   \
            *(JSValue *)((uint8_t *)Wc + crf->this_b) = (__VA_ARGS__);  \
            if (crf->var_count)                                         \
                rv_undefined(Fn - crf->var_count, crf->var_count);      \
            h = (RVHeader *)Fn;                                         \
            h->caller = F;                                              \
            h->retpc = pc;                                              \
            h->rf = crf;                                                \
            RV_COUNT_RUN(crf);                                          \
            RV_PEAK(rt, Fn + crf->fsize);                              \
            h->k = crf->k;                                              \
            h->dst = (uint16_t)dst;                                     \
            h->var_refs = p->u.func.var_refs;                           \
            F = Fn;                                                     \
            pc = crf->code;                                             \
            RV_NEXT();                                                  \
        }                                                               \
    label ## _out:                                                      \
        this_v = (__VA_ARGS__);                                         \
        W = Wc;                                                         \
        n = argc1;                                                      \
        goto out;                                                       \
    label ## _seg:                                                      \
        this_v = (__VA_ARGS__);                                         \
        W = Wc;                                                         \
        n = argc1;                                                      \
        goto seg;                                                       \
    }

    RV_CALLOP(rv_CALL, 2, (int8_t)ip[0], JS_UNDEFINED, rv_call_args, rv_call_seg, rv_call_out)
    RV_CALLOP(rv_CALLM, 2, (int8_t)ip[0] - 1, F[dst], rv_call_args, rv_call_seg, rv_call_out)
    RV_CALLOP(rv_CALLML, 3, (int8_t)ip[0] - 1, JS_DupValue(ctx, RV_R(ip[-1])),
              rv_call_args, rv_call_seg, rv_call_out)
    /* the same, the result dropped: a frame of the loop gets RV_DST_DROP,
       which its return frees, a call out of the loop frees it here */
    RV_CALLOP(rv_CALLD, 2, RV_DST_DROP, JS_UNDEFINED, rv_call_args_d, rv_call_seg_d, rv_call_out_d)
    RV_CALLOP(rv_CALLMD, 2, RV_DST_DROP, F[(int8_t)ip[0] - 1], rv_call_args_d, rv_call_seg_d, rv_call_out_d)
    RV_CALLOP(rv_CALLMLD, 3, RV_DST_DROP, JS_DupValue(ctx, RV_R(ip[-1])),
              rv_call_args_d, rv_call_seg_d, rv_call_out_d)

    /* CALLS w argc k s1 .. sk: CALL, the last k arguments copied into the
       window first (rv_call_copy), before any check that can leave the
       instruction, so that every way out finds the window full and the
       call's own, as after the MOVs it stands for. The first call of a
       function not yet converted goes on after the copies. */
#define RV_CALLSOP(label, dstexpr, args, seg, out)                      \
 label:                                                                 \
    poll_count(POLL_WEIGHT);                                            \
    ip = pc;                                                            \
    pc += 3 + ip[2];                                                    \
    rv_call_copy(ctx, F, ip);                                           \
 label ## _go:                                                          \
    RV_CALL_BODY(label, dstexpr, args, seg, out, goto label ## _go,     \
                 JS_UNDEFINED)

    RV_CALLSOP(rv_CALLS, (int8_t)ip[0], rv_call_args, rv_call_seg, rv_call_out)
    RV_CALLSOP(rv_CALLSD, RV_DST_DROP, rv_call_args_d, rv_call_seg_d, rv_call_out_d)
#undef RV_CALLOP
#undef RV_CALLSOP
#undef RV_CALL_BODY

    /* a converted function called with more arguments than parameters,
       or one that reads its arguments: the checks of the fast entry, then
       its frame from rv_enter_args, which moves the declared arguments up
       past the extra ones (their room counted) and stores argc. Only F and
       pc are live after it. */
#define RV_CALL_ARGS(label, seg, out)                                   \
 label:                                                                 \
    p = JS_VALUE_GET_OBJ(W[0]);                                         \
    rf = p->u.func.function_bytecode->rv;                               \
    if (unlikely(p->u.func.function_bytecode->realm != ctx))            \
        goto out;                                                       \
    if (unlikely((uint8_t *)(W + (n > rf->arg_count ? n + 1 : 0)) + rf->room_b > \
                 (uint8_t *)rt->rv_end))                                \
        goto seg;                                                       \
    if (unlikely(poll_flag(&rt->interrupt_flag)) ||                     \
        unlikely(js_uncatchable_pending(rt)) ||                         \
        unlikely(rt->call_depth >= rt->max_call_depth))                 \
        goto out;                                                       \
    rt->call_depth++;                                                   \
    F = rv_enter_args(ctx, F, pc, W, n, this_v, dst);                   \
    pc = ((RVHeader *)F)->rf->code;                                     \
    RV_NEXT();

    /* the frame of a converted callee of the same realm does not fit in
       the segment of the register stack in use: the window moved to the
       next segment, made large enough for it, and the frame made there by
       the slow entry, flagged so that its return steps back
       (engine/regvm-entry.h). The checks of the fast entry first. */
#define RV_CALL_SEG(label, out)                                         \
 label:                                                                 \
    rf = JS_VALUE_GET_OBJ(W[0])->u.func.function_bytecode->rv;          \
    if (unlikely(poll_flag(&rt->interrupt_flag)) ||                     \
        unlikely(js_uncatchable_pending(rt)) ||                         \
        unlikely(rt->call_depth >= rt->max_call_depth))                 \
        goto out;                                                       \
    W = rv_seg_move(ctx, F, W, n, this_v, rf);                          \
    if (unlikely(!W))                                                   \
        goto rv_exception;                                              \
    rt->call_depth++;                                                   \
    F = rv_enter_args(ctx, F, pc, W, n, this_v, dst);                   \
    ((RVHeader *)F)->dst |= RV_DST_SEG;                                 \
    pc = rf->code;                                                      \
    RV_NEXT();

    /* a call out of the loop: JS_CallInternal throws what the entry
       refused. The result goes to the slot dst, or is freed (store). */
#define RV_CALL_OUT(label, store, args)                                 \
 label:                                                                 \
    RV_COUNT(rt, JS_VALUE_GET_TAG(W[0]) == JS_TAG_OBJECT &&             \
             JS_VALUE_GET_OBJ(W[0])->class_id == JS_CLASS_BYTECODE_FUNCTION ? 2 : 3); \
    rt->rv_top = W + 1 + n;                                             \
    sf->cur_pc = pc;                                                    \
    sf->cur_sp = F;                                                     \
    if (unlikely((i = JS_VALUE_GET_TAG(W[0]) == JS_TAG_OBJECT ?         \
                      JS_VALUE_GET_OBJ(W[0])->rv_fast : RV_FAST_NONE) !=  \
                 RV_FAST_NONE) && !RV_INJ_ARMED &&                       \
        !JS_IsUninitialized(ret_val = rv_fast_call(ctx, W, n, this_v)))  \
        ; /* answered without the call: hasOwnProperty.call(o, k), as   \
             QuickJS's OP_call_method, and charCodeAt (rv_fast_call).   \
             One call site for both keeps the loop's registers as they  \
             were (a second one spilled on arm64). i keeps the kind     \
             for rv_unwrap_first */                                     \
    else if (RV_INJ_HIT(ctx))                                           \
        ret_val = JS_EXCEPTION;                                         \
    else if (unlikely(rv_unwrap_first(ctx, W[0], i == RV_FAST_CALL)) && \
             !RV_INJ_ARMED &&                                           \
             (i = rv_unwrap_call(ctx, W, n, this_v)) >= 0 &&            \
             (n = i, this_v = rt->rv_unwrap_this,                       \
              rv_unwrap_in_loop(W[0])))                                 \
        goto args; /* bound or call taken apart: a call in the loop */  \
    else if (js_call_c_direct_ok(rt, W[0]))                             \
        ret_val = js_call_c_direct(ctx, W[0], this_v, n, W + 1);        \
    else if (rv_entry_fast(rt, W[0]))                                   \
        /* register code called with more arguments than it has, or     \
           from another realm: a run of its own, as QuickJS's           \
           interpreter calls it */                                      \
        ret_val = JS_RegvmRun(ctx, W[0], this_v, JS_UNDEFINED, n, W + 1); \
    else                                                                \
        ret_val = JS_CallInternal(ctx, W[0], this_v, JS_UNDEFINED, n, W + 1, 0); \
    for (i = 0; i <= n; i++)                                            \
        JS_FreeValue(ctx, W[i]);                                        \
    JS_FreeValue(ctx, this_v);                                          \
    if (unlikely(JS_IsException(ret_val)))                              \
        goto rv_exception;                                              \
    store;                                                              \
    RV_NEXT();

    RV_CALL_ARGS(rv_call_args, rv_call_seg, rv_call_out)
    RV_CALL_ARGS(rv_call_args_d, rv_call_seg_d, rv_call_out_d)
    RV_CALL_SEG(rv_call_seg, rv_call_out)
    RV_CALL_SEG(rv_call_seg_d, rv_call_out_d)
    RV_CALL_OUT(rv_call_out, F[dst] = ret_val, rv_call_args)
    RV_CALL_OUT(rv_call_out_d, JS_FreeValue(ctx, ret_val), rv_call_args_d)
#undef RV_CALL_ARGS
#undef RV_CALL_SEG
#undef RV_CALL_OUT

#endif
