/*
 * Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */

/* OLD: one register op for the rare old instructions, those of classes and
   private fields first. Read once for each part, see regvm-areas.h.

   The instruction OLD is followed by its length, the slot w of the old sp,
   and the old instruction itself, opcode and operands as the old bytecode
   has them. rv_old runs the body of the old handler, the macro
   OP_BODY_<op> of regvm-bodies.h, on the temps under w laid out as the old
   stack, as the window ops of regvm-ops-window.h do: one dispatch slot
   serves every opcode of the list.

   Nothing in the pass is written for one opcode: what an opcode takes and
   leaves is that of the opcode table (n_pop, n_push, size), the pass puts
   the n_pop entries into their temps, and the n_push results are temps at
   their slots. The ops of the list are those whose handler does not jump
   and reads no frame state but the function, its closure variables and
   its captured locals, which rv_old gives the body from F.

   An exception: the body goes to its exception label with its own sp, as
   in QuickJS's interpreter, which then frees the stack from there down.
   rv_old frees the values from that sp down to the first of the n_pop
   operands, and the pcmap entry names the temps below them, which the
   unwinding frees. So each body is right on each of its paths however it
   leaves its operands, and no table of what a body frees has to agree with
   it. When the body ends, its sp must be where the opcode table says, and
   at a throw it must not be under the operands. No bytecode can break
   this, read back by JS_ReadObject or not: the opcode is one of the list
   (the pass emits OLD for no other, and register code is never read
   back), each path of each body moves sp by a fixed amount, which no
   operand and no value changes, and the pass gives the body its n_pop
   operands or refuses the function. Only a wrong body or a wrong list can,
   so a test build (RV_SELF_CHECK, engine/test/build.sh) stops at it, and
   the shipped engine throws an InternalError instead of stopping the
   process. engine/test/regvm/js/k-*.js run each opcode both ways. */

/* the old opcodes OLD runs, but special_object, whose cases are picked by
   rv_old_special */
#define RV_OLD_CASES                                                    \
    case OP_define_class: case OP_define_class_computed:                \
    case OP_define_method_computed:                                     \
    case OP_set_home_object: case OP_set_proto: case OP_set_name_computed: \
    case OP_get_super: case OP_get_super_value: case OP_put_super_value: \
    case OP_private_symbol: case OP_get_private_field:                  \
    case OP_put_private_field: case OP_define_private_field:            \
    case OP_private_in: case OP_add_brand: case OP_check_brand:         \
    case OP_import:                                                     \
    case OP_get_ref_value: case OP_put_ref_value:                       \
    case OP_make_var_ref: case OP_make_var_ref_ref:                     \
    case OP_apply_eval: case OP_delete_var:                             \
    case OP_put_var_ref_check_init

#if RV_PART == RV_PART_LIST
RV_OP(OLD)      /* len w op ...: the old instruction op ... on the temps under w */
#elif RV_PART == RV_PART_INFO
    RV_OLD_CASES:
    case OP_make_loc_ref: case OP_make_arg_ref:
        return 0;
    case OP_eval:
        *npop = 1 + get_u16(bc + pos + 1);
        return 0;
#elif RV_PART == RV_PART_PASS_FN
/* the old instruction at p as OLD: its n_pop operands into their temps,
   then OLD, its length, the slot of the old sp and the old instruction. The
   operands are the body's, freed by rv_old when it throws: the pcmap entry
   names the temps below them. eval takes its arguments past n_pop, as
   its operand counts them. */
static void rv_old_op(RVPass *s, const uint8_t *p)
{
    const JSOpCode *oi = &short_opcode_info(p[0]);
    int npop = oi->n_pop + (oi->fmt == OP_FMT_npop_u16 ? get_u16(p + 1) : 0);
    int npush = oi->n_push, i;

    if (npop > s->sp) {
        rv_crafted(s);
        return;
    }
    for (i = s->sp - npop; i < s->sp; i++)
        rv_materialize(s, i);
    rv_emit(s, RV_OLD);
    rv_byte(s, 2 + oi->size);
    rv_byte(s, rv_tslot(s, s->sp));
    for (i = 0; i < oi->size; i++)
        rv_byte(s, p[i]);
    /* set_home_object and the home object cannot throw: no pcmap entry, so
       that the throw-injection test asks nothing of them */
    if (p[0] != OP_set_home_object &&
        !(p[0] == OP_special_object && p[1] == OP_SPECIAL_OBJECT_HOME_OBJECT))
        rv_throws(s, rv_owned(s, npop));
    s->sp -= npop;
    for (i = 0; i < npush; i++)
        rv_push(s, RV_E_TEMP, rv_tslot(s, s->sp), 0);
}

/* special_object: the kinds OLD runs (regvm-ops-ctor.h sends the others
   here), among them the var object of a function with a sloppy direct
   eval. A byte of no kind is code no compiler makes. */
static void rv_old_special(RVPass *s, const uint8_t *p)
{
    switch (p[1]) {
    case OP_SPECIAL_OBJECT_HOME_OBJECT:
    case OP_SPECIAL_OBJECT_IMPORT_META:
    case OP_SPECIAL_OBJECT_VAR_OBJECT:
        rv_old_op(s, p);
        break;
    default:
        rv_crafted(s); /* no kind */
        break;
    }
}
#elif RV_PART == RV_PART_PASS
    RV_OLD_CASES:
        rv_old_op(s, p);
        break;
    case OP_eval:
        rv_old_op(s, p);
        break;
    case OP_make_loc_ref: case OP_make_arg_ref:
        /* a reference to a local or an argument is a var_ref of the
           closure state, which a function without captured locals has not */
        if (!s->b->var_ref_count) {
            rv_crafted(s);
            break;
        }
        rv_old_op(s, p);
        break;
#elif RV_PART == RV_PART_RUN_FN
/* OLD, ip at len w op: the body of op on the window of the frame F under
   the slot w, with pc at its operands. 0, or -1 with the body's operands
   freed when it throws. The frame state a body reads: the function (the
   home object), its closure variables and the closure state of the frame
   (a class constructor made by define_class captures as fclosure does, a
   reference to a local is a var_ref of it), and the strict mode of the
   frame for a store and a reference (put_super_value, get_ref_value,
   put_ref_value), which is_strict_mode would read from the entry frame of
   the loop. A direct eval runs with F as the running frame (rv_eval_enter,
   regvm-ops-eval.h). */
static no_inline int rv_old(JSContext *ctx, JSValue *F, const uint8_t *ip)
{
    RVHeader *h = (RVHeader *)F;
    RVFunc *rf = h->rf;
    JSValue *sp = F + (int8_t)ip[1], *base;
    const uint8_t *pc = ip + 3;
    int opcode = ip[2];
    const JSOpCode *oi = &short_opcode_info(opcode);
    JSStackFrame *esf = NULL, tsf;
    JSValue *call_argv, ret_val;
    int call_argc;

    base = sp - oi->n_pop - (oi->fmt == OP_FMT_npop_u16 ? get_u16(pc) : 0);
    /* the throw-injection test fails the body as at its first call into the
       engine: the operands are on the stack, and the exit below frees them
       as at a real error. set_proto of a value that is not an object or
       null calls nothing. */
    if (RV_INJ_ARMED && opcode == OP_set_proto && !JS_IsObject(sp[-1]) && !JS_IsNull(sp[-1]))
        RV_INJ_NONE("skip");
    if (RV_INJ_HIT(ctx))
        goto exception; /* the test of regvm-inject.h, before the body */
    switch (opcode) {
    case OP_define_class:
    case OP_define_class_computed:
        OP_BODY_define_class(ctx, opcode, sp, pc, h->var_refs,
                             rf->var_ref_count ? RV_SF(F) : NULL, exception)
        break;
    case OP_define_method_computed:
        {
            int mop = opcode; /* the body changes it */
            OP_BODY_define_method(ctx, mop, sp, pc, exception)
        }
        break;
    case OP_set_home_object:
        OP_BODY_set_home_object(ctx, sp)
        break;
    case OP_set_proto:
        OP_BODY_set_proto(ctx, sp, (void)0, exception)
        break;
    case OP_set_name_computed:
        OP_BODY_set_name_computed(ctx, sp, exception)
        break;
    case OP_get_super:
        OP_BODY_get_super(ctx, sp, (void)0, exception)
        break;
    case OP_get_super_value:
        OP_BODY_get_super_value(ctx, sp, (void)0, exception)
        break;
    case OP_put_super_value:
        OP_BODY_put_super_value(ctx, sp,
                                (rf->b->js_mode & JS_MODE_STRICT) ? JS_PROP_THROW : 0,
                                (void)0, exception)
        break;
    case OP_private_symbol:
        OP_BODY_private_symbol(ctx, sp, pc, exception)
        break;
    case OP_get_private_field:
        OP_BODY_get_private_field(ctx, sp, exception)
        break;
    case OP_put_private_field:
        OP_BODY_put_private_field(ctx, sp, exception)
        break;
    case OP_define_private_field:
        OP_BODY_define_private_field(ctx, sp, exception)
        break;
    case OP_private_in:
        OP_BODY_private_in(ctx, sp, (void)0, exception)
        break;
    case OP_add_brand:
        OP_BODY_add_brand(ctx, sp, exception)
        break;
    case OP_check_brand:
        OP_BODY_check_brand(ctx, sp, exception)
        break;
    case OP_import:
        /* the script of the frame: the walk of the frames from the top
           one, which RV_SYNC_TOP saved (JS_GetScriptOrModuleName) */
        OP_BODY_import(ctx, sp, (void)0, exception)
        break;
    case OP_get_ref_value:
        OP_BODY_get_ref_value(ctx, sp, rf->b->js_mode & JS_MODE_STRICT, (void)0, exception)
        break;
    case OP_put_ref_value:
        OP_BODY_put_ref_value(ctx, sp, rf->b->js_mode & JS_MODE_STRICT,
                              (rf->b->js_mode & JS_MODE_STRICT) ? JS_PROP_THROW : 0,
                              (void)0, exception)
        break;
    case OP_make_var_ref:
        OP_BODY_make_var_ref(ctx, sp, pc, (void)0, exception)
        break;
    case OP_make_loc_ref:
    case OP_make_arg_ref:
    case OP_make_var_ref_ref:
        {
            JSRuntime *rt = ctx->rt;
            OP_BODY_make_ref(ctx, opcode, sp, pc, h->var_refs,
                             rf->var_ref_count ? RV_SF(F) : NULL, exception)
        }
        break;
    case OP_eval:
        esf = rv_eval_enter(ctx, F, &tsf);
        OP_BODY_eval(ctx, sp, pc, call_argc, call_argv, ret_val, (void)0, eval_exception)
        rv_eval_leave(ctx, F, esf);
        break;
    case OP_apply_eval:
        esf = rv_eval_enter(ctx, F, &tsf);
        OP_BODY_apply_eval(ctx, sp, pc, ret_val, (void)0, eval_exception)
        rv_eval_leave(ctx, F, esf);
        break;
    eval_exception:
        rv_eval_leave(ctx, F, esf);
        goto exception;
    case OP_delete_var:
        OP_BODY_delete_var(ctx, sp, pc, (void)0, exception)
        break;
    case OP_put_var_ref_check_init:
        /* the initialization of the this of a derived constructor by
           super() in an arrow function */
        OP_BODY_put_var_ref_check_init(ctx, rf->b, h->var_refs, sp, pc, exception)
        break;
    case OP_special_object:
        switch (*pc++) {
        case OP_SPECIAL_OBJECT_HOME_OBJECT:
            {
                JSObject *p = JS_VALUE_GET_OBJ(F[-rf->fofs]);
                OP_BODY_home_object(ctx, p, sp)
            }
            break;
        case OP_SPECIAL_OBJECT_IMPORT_META:
            OP_BODY_import_meta(ctx, sp, exception)
            break;
        case OP_SPECIAL_OBJECT_VAR_OBJECT:
            OP_BODY_var_object(ctx, sp, exception)
            break;
        default:
            goto broken;
        }
        break;
    case OP_fclosure:
        /* a constant past the 16 bits of FCLOS (regvm-ops-closure.h), as
           rv_fclosure makes the closure */
        *sp = js_closure(ctx, JS_DupValue(ctx, rf->b->cpool[get_u32(pc)]), h->var_refs,
                         rf->var_ref_count ? RV_SF(F) : NULL, FALSE);
        if (JS_IsException(*sp))
            goto exception;
        sp++;
        break;
    default:
        goto broken;
    }
    if (unlikely(sp != base + oi->n_push))
        goto broken;
    return 0;
 exception:
    if (unlikely(sp < base))
        goto broken;
    while (sp > base)
        JS_FreeValue(ctx, *--sp);
    return -1;
 broken:
    /* an opcode the pass does not send here, or a body that moved sp
       otherwise than the opcode table says: a fault of this file or of
       a body, never of the bytecode (see above). A test build stops. The
       shipped engine throws and frees nothing, as what the body owns is
       not known: the window is left to leak, and the temps under it that
       the body took are made undefined, so that the unwinding does not
       free them again. */
#ifdef RV_SELF_CHECK
    abort();
#endif
    if (sp < F + rf->temp0)
        sp = F + rf->temp0;
    while (sp < base)
        *sp++ = JS_UNDEFINED;
    JS_ThrowInternalError(ctx, "register loop: opcode %d broke its stack", opcode);
    return -1;
}
#elif RV_PART == RV_PART_RUN
    /* the body may run JS (a getter, a conversion, the class heritage), so
       the register stack is given to it above this frame, and pc is at the
       end of the instruction for the pcmap */
 rv_OLD:
    ip = pc;
    pc += ip[0];
    RV_SYNC_TOP();
    if (rv_old(ctx, F, ip))
        goto rv_exception;
    RV_NEXT();
#endif
