/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* Exceptions in the register loop: throw, throw_error, catch, nip_catch,
   gosub and ret. Read once for each part, see regvm-areas.h. The unwinding
   is rv_unwind in regvm-run.h.

   A catch offset is a value of its own in the temp of its depth, as QuickJS's
   interpreter has it on its stack: CATCH puts it there, with the offset of its
   own end in the register code, where its jump to the catch is read
   (rv_catch_target), and where a pcmap entry names the old pc before the
   catch, as QuickJS's interpreter names an interrupt it reads after the jump
   to a catch. It is owned like any temp (rv_owned), so the
   pcmap of each instruction in the try names it, and the unwinding stops
   there. The return address of a finally (gosub) is an int in the temp of
   its depth: the one QuickJS's interpreter makes, the offset of the end of the
   old gosub, which code read by JS_ReadObject can make or read as any int. ret
   checks it against ret_bits as QuickJS's interpreter does, and finds the end
   of its GOSUB in the register code in rf->rets. */

#if RV_PART == RV_PART_LIST
RV_OP(THROW)    /* s: throws the value of s, a temp is moved */
RV_OP(THROWE)   /* atom type: throws the error of throw_error */
RV_OP(CATCH)    /* d off: a catch offset into temp d, for the catch at off */
RV_OP(TMOV)     /* d t: temp d = temp t, moved */
RV_OP(GOSUB)    /* d off ret: ret (an old offset) into temp d, a jump to off */
RV_OP(GORET)    /* t: a jump to the return address in temp t */
#elif RV_PART == RV_PART_INFO
    case OP_throw: case OP_throw_error: case OP_ret:
        *term = 1;
        return 0;
    case OP_catch: case OP_gosub:
        *target = pos + 1 + (int32_t)get_u32(bc + pos + 1);
        return 0;
    case OP_nip_catch:
        /* its depth after is that of its catch (rv_depths) */
        return 0;
#elif RV_PART == RV_PART_PASS_FN
/* the entry of the stack at i is dropped, as OP_drop drops the top */
static void rv_except_drop(RVPass *s, int i)
{
    RVEnt e = s->st[i];
    if (e.kind == RV_E_TEMP) {
        rv_emit(s, RV_FREE);
        rv_byte(s, e.slot);
    }
}

/* the pcmap entry rv_throws just made for a GOSUB gets an entry before
   it, at its end less one, for an interrupt that a GORET reads after it
   took the return address: the stack is that of the GOSUB, and QuickJS's
   interpreter names the end of its gosub less one there (OP_ret jumps, then
   reads it) */
static void rv_gosub_entries(RVPass *s)
{
    RVPcMask *g = (RVPcMask *)(s->pcmap.buf + s->pcmap.size) - 1;
    RVPcMask r = *g, last = *g;
    RVPcHi h = *((RVPcHi *)(s->pcmap_hi.buf + s->pcmap_hi.size) - 1);
    r.end = g->end - 1;
    r.old = s->pos + 4;
    *g = r;
    if (dbuf_put(&s->pcmap, (uint8_t *)&last, sizeof(last)) ||
        dbuf_put(&s->pcmap_hi, (uint8_t *)&h, sizeof(h))) /* the same temps */
        s->failed = TRUE;
}

/* the GOSUB just emitted returns to old, the end of its old gosub: GORET
   goes from old to the end of the GOSUB (rf->rets, by old as the pass
   reads the code in order) */
static void rv_ret_put(RVPass *s, uint32_t old)
{
    uint32_t r[2];
    r[0] = old;
    r[1] = s->code.size;
    if (!s->failed && dbuf_put(&s->rets, (uint8_t *)r, sizeof(r)))
        s->failed = TRUE;
}

/* the pcmap entry at the end of a CATCH, for an interrupt read at the jump
   to its catch (rv_unwind): old is the old target less one, as QuickJS's
   interpreter has pc at the target then. CATCH itself does not throw. */
static void rv_catch_entry(RVPass *s, int old)
{
    rv_pcmap_put(s, s->code.size, old, rv_owned(s, 0));
    rv_hooks(s, RV_HOOK_NONE);
}
#elif RV_PART == RV_PART_PASS
    case OP_throw:
        slot = rv_slot_of(s, s->sp - 1);
        rv_emit(s, RV_THROW);
        rv_byte(s, slot);
        rv_throws(s, rv_owned(s, 1));
        /* it throws at every run: the test has nothing to add */
        rv_hooks(s, RV_HOOK_NONE);
        s->sp = -1;
        break;
    case OP_throw_error:
        rv_emit(s, RV_THROWE);
        rv_u32(s, get_u32(p + 1));
        rv_byte(s, p[5]);
        rv_throws(s, rv_owned(s, 0));
        rv_hooks(s, RV_HOOK_NONE);
        s->sp = -1;
        break;
    case OP_catch:
        /* the stack under it is canonical at the catch's target */
        rv_block_end(s, s->sp);
        d = rv_tslot(s, s->sp);
        rv_emit(s, RV_CATCH);
        rv_byte(s, d);
        rv_jump_to(s, pos + 1 + (int32_t)get_u32(p + 1));
        rv_catch_entry(s, pos + (int32_t)get_u32(p + 1));
        rv_push(s, RV_E_TEMP, d, 0);
        break;
    case OP_nip_catch:
        {
            /* catch_offset ... a -> a: what is between is dropped, the catch
               offset has no reference, a goes in its place */
            int l = rv_catch_level(s, s->catchpos[pos]);
            e = s->st[s->sp - 1];
            for (i = s->sp - 2; i > l; i--)
                rv_except_drop(s, i);
            s->sp = l;
            if (e.kind == RV_E_TEMP) {
                d = rv_tslot(s, l);
                rv_emit(s, RV_TMOV);
                rv_byte(s, d);
                rv_byte(s, e.slot);
                rv_push(s, RV_E_TEMP, d, 0);
            } else {
                rv_push(s, e.kind, e.slot, e.k);
            }
        }
        break;
    case OP_gosub:
        {
            int target = pos + 1 + (int32_t)get_u32(p + 1);
            rv_block_end(s, s->sp);
            rv_emit(s, RV_GOSUB);
            rv_byte(s, rv_tslot(s, s->sp));
            rv_jump_to(s, target);
            rv_u32(s, pos + 5);
            rv_throws(s, rv_owned(s, 0));
            if (!s->failed)
                rv_gosub_entries(s);
            rv_ret_put(s, pos + 5);
        }
        break;
    case OP_ret:
        /* a return address is an int as any other: code that JS_ReadObject
           read may make one with push_i32 or compute it, so a constant
           goes to its temp, and GORET checks the value as QuickJS's
           interpreter does */
        if (s->st[s->sp - 1].kind != RV_E_TEMP)
            rv_materialize(s, s->sp - 1);
        e = s->st[s->sp - 1];
        rv_block_end(s, s->sp - 1);
        rv_emit(s, RV_GORET);
        rv_byte(s, e.slot);
        /* a value that is no return address stays in its temp when GORET
           throws, as QuickJS's OP_ret leaves it on its stack: the
           unwinding frees it. (An interrupt read after the jump is at the
           entry of the GOSUB, rv_gosub_entries.) */
        rv_throws(s, rv_owned(s, 0));
        rv_hooks(s, RV_HOOK_OWN_POLL);
        s->sp = -1;
        break;
#elif RV_PART == RV_PART_RUN_FN
/* the error of throw_error, as QuickJS's OP_throw_error */
static no_inline void rv_throw_error(JSContext *ctx, JSAtom atom, int type)
{
    if (type == JS_THROW_VAR_RO)
        JS_ThrowTypeErrorReadOnly(ctx, JS_PROP_THROW, atom);
    else if (type == JS_THROW_VAR_REDECL)
        JS_ThrowSyntaxErrorVarRedeclaration(ctx, atom);
    else if (type == JS_THROW_VAR_UNINITIALIZED)
        JS_ThrowReferenceErrorUninitialized(ctx, atom);
    else if (type == JS_THROW_ERROR_DELETE_SUPER)
        JS_ThrowReferenceError(ctx, "unsupported reference to 'super'");
    else if (type == JS_THROW_ERROR_ITERATOR_THROW)
        JS_ThrowTypeError(ctx, "iterator does not have a throw method");
    else if (type == JS_THROW_ERROR_INVALID_LVALUE_CALL)
        JS_ThrowReferenceError(ctx, "invalid assignment left-hand side");
    else
        JS_ThrowInternalError(ctx, "invalid throw var type %d", type);
}

/* where GORET at ip goes: the end of the GOSUB whose old gosub ends at the
   value, or NULL with an exception. The value is checked against ret_bits
   as QuickJS's OP_ret checks it, so the same ints are taken. Every
   gosub of ret_bits has its GOSUB in rf->rets (rv_build refuses a function
   with a gosub it did not convert), sorted by old. */
static no_inline const uint8_t *rv_ret_target(JSContext *ctx, JSValue *F,
                                              const uint8_t *ip)
{
    const RVFunc *rf = ((RVHeader *)F)->rf;
    const JSFunctionBytecode *b = rf->b;
    JSValue v = F[(int8_t)ip[0]];
    uint32_t pos;
    int lo = 0, hi = rf->ret_count - 1, mid;
    if (RV_INJ_HIT(ctx))
        return NULL;
    if (JS_VALUE_GET_TAG(v) != JS_TAG_INT)
        goto fail;
    pos = JS_VALUE_GET_INT(v);
    if (pos >= b->byte_code_len || !b->ret_bits ||
        !((b->ret_bits[pos >> 5] >> (pos & 31)) & 1))
        goto fail;
    while (lo <= hi) {
        mid = (lo + hi) >> 1;
        if (rf->rets[2 * mid] < pos)
            lo = mid + 1;
        else if (rf->rets[2 * mid] > pos)
            hi = mid - 1;
        else
            return rf->code + rf->rets[2 * mid + 1];
    }
#ifdef RV_SELF_CHECK
    abort(); /* a gosub of ret_bits with no GOSUB: a fault of rv_build */
#endif
 fail:
    JS_ThrowInternalError(ctx, "invalid ret value");
    return NULL;
}
#elif RV_PART == RV_PART_RUN
 rv_THROW:
    ip = pc; pc += 1;
    JS_Throw(ctx, rv_take(ctx, F, (int8_t)ip[0]));
    goto rv_exception;
 rv_THROWE:
    ip = pc; pc += 5;
    rv_throw_error(ctx, get_u32(ip), ip[4]);
    goto rv_exception;
 rv_CATCH:
    ip = pc; pc += 3;
    RV_R(ip[0]) = JS_NewCatchOffset(ctx, pc - ((RVHeader *)F)->rf->code);
    RV_NEXT();
 rv_TMOV:
    ip = pc; pc += 2;
    RV_R(ip[0]) = RV_R(ip[1]);
    RV_NEXT();
    /* gosub and ret: as QuickJS's, but the interrupt flag is read
       before pc moves, so that pc names the instruction for the pcmap */
 rv_GOSUB:
    ip = pc; pc += 7;
    if (RV_INJ_HIT(ctx) || unlikely(js_poll_interrupts(ctx)))
        goto rv_exception;
    RV_R(ip[0]) = JS_NewInt32(ctx, get_u32(ip + 3));
    {
        int off = (int16_t)get_u16(ip + 1);
        if (off < 0)
            poll_count(POLL_WEIGHT);
        pc = ip + 3 + off; /* off is from the end of off */
    }
    RV_NEXT();
 rv_GORET:
    ip = pc; pc += 1;
    {
        const uint8_t *to = rv_ret_target(ctx, F, ip);
        if (unlikely(!to))
            goto rv_exception;
        poll_count(POLL_WEIGHT);
        if (RV_INJ_HIT(ctx) || unlikely(js_poll_interrupts(ctx))) {
            pc = to - 1; /* rv_gosub_entries */
            goto rv_exception;
        }
        pc = to;
    }
    RV_NEXT();
#endif
