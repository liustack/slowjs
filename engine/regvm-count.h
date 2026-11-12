/* Copyright (c) 2026 The liustack/quickjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 *
 * A counting build for the offline estimate of register bytecode (not part of
 * the engine: included by quickjs.c only with -DJS_REGVM_COUNT, in the native
 * test host). For each function that runs while counting is on it keeps, per
 * byte of its bytecode, how often the instruction there ran and how often the
 * top of the stack held a value with a reference count after it, and for each
 * call opcode which function it reached. The host gives the memory
 * (js_rv_alloc) and takes the text of the dump (js_rv_emit). */

__attribute__((weak)) void *js_rv_alloc(size_t size) { (void)size; abort(); }
__attribute__((weak)) void js_rv_emit(const char *s, size_t len) { (void)s; (void)len; }

int js_rv_on;

typedef struct RVCountFunc {
    struct RVCountFunc *next;
    JSFunctionBytecode *b;
    uint32_t id;
    uint32_t len;
    uint64_t *n;     /* runs of the instruction at each offset */
    uint64_t *rc;    /* of those, the top of the stack after it had a count */
    uint64_t act;    /* activations */
    uint64_t from_c; /* activations not made by a call opcode of the interpreter */
    struct RVCountFunc *all_next;
} RVCountFunc;

typedef struct RVCall {
    struct RVCall *next;
    uint32_t caller, off, callee; /* callee: id, or 0 for a function that is no bytecode */
    int argc;
    int kind; /* 0 call, 1 call_method, 2 call_constructor, 3 C function direct, 4 hasOwnProperty.call */
    uint64_t n;
} RVCall;

#define RV_HASH 65536
static RVCountFunc *rv_tab[RV_HASH];
static RVCountFunc *rv_all;
static uint32_t rv_next_id = 1;
static RVCall *rv_calls[RV_HASH];

/* the trace of the register pass over each function at the dump: for each
   register instruction it emits, its offset and the old instruction it was
   emitted for (and whether it was emitted at the end of the block before a
   leader, before the leader itself), and the leaders, so that a tool weighs
   the register code with the counts of the old loop (Q, R and L lines) */
struct RVFunc;
static struct RVFunc *rv_build(JSRuntime *rt, JSFunctionBytecode *b, BOOL *oom);
static void rv_free_func(JSRuntime *rt, struct RVFunc *rf);
const char *JS_RegvmOpName(int op);
#define RV_TRACE_MAX 65536
static int rv_trace_on, rv_trace_n;
static int32_t *rv_trace_off, *rv_trace_pos;
static uint8_t *rv_trace_leader;

static void rv_trace(int off, int pos)
{
    if (rv_trace_n < RV_TRACE_MAX) {
        rv_trace_off[rv_trace_n] = off;
        rv_trace_pos[rv_trace_n] = pos;
        rv_trace_n++;
    }
}

/* set by a call opcode right before it calls, read and cleared by the entry */
static int rv_pending;
static uint32_t rv_site_id, rv_site_off;
static int rv_site_kind;

static void __attribute__((format(printf, 1, 2))) rv_printf(const char *fmt, ...)
{
    char buf[1024];
    va_list ap;
    int n;
    va_start(ap, fmt);
    n = vsnprintf(buf, sizeof buf, fmt, ap);
    va_end(ap);
    if (n > (int)sizeof buf - 1)
        n = sizeof buf - 1;
    js_rv_emit(buf, n);
}

static uint32_t rv_hash_ptr(const void *p)
{
    uintptr_t v = (uintptr_t)p;
    v ^= v >> 17;
    v *= 0x9E3779B97F4A7C15ull;
    return (uint32_t)(v >> 40) & (RV_HASH - 1);
}

static RVCountFunc *rv_func(JSFunctionBytecode *b)
{
    uint32_t h = rv_hash_ptr(b);
    RVCountFunc *f;
    for (f = rv_tab[h]; f; f = f->next)
        if (f->b == b)
            return f;
    f = js_rv_alloc(sizeof(*f));
    f->b = b;
    f->id = rv_next_id++;
    f->len = b->byte_code_len;
    f->n = js_rv_alloc(sizeof(uint64_t) * (f->len + 1));
    f->rc = js_rv_alloc(sizeof(uint64_t) * (f->len + 1));
    f->next = rv_tab[h];
    rv_tab[h] = f;
    f->all_next = rv_all;
    rv_all = f;
    return f;
}

static void rv_record_call(uint32_t caller, uint32_t off, uint32_t callee, int argc, int kind)
{
    uint32_t h = (caller * 2654435761u ^ off * 40503u ^ callee * 97u ^ (uint32_t)argc * 7u ^ (uint32_t)kind) & (RV_HASH - 1);
    RVCall *c;
    for (c = rv_calls[h]; c; c = c->next)
        if (c->caller == caller && c->off == off && c->callee == callee &&
            c->argc == argc && c->kind == kind) {
            c->n++;
            return;
        }
    c = js_rv_alloc(sizeof(*c));
    c->caller = caller;
    c->off = off;
    c->callee = callee;
    c->argc = argc;
    c->kind = kind;
    c->n = 1;
    c->next = rv_calls[h];
    rv_calls[h] = c;
}

static void rv_dump_func(JSRuntime *rt, RVCountFunc *f, int all)
{
    JSFunctionBytecode *b = f->b;
    char nb[ATOM_GET_STR_BUF_SIZE], fb[ATOM_GET_STR_BUF_SIZE];
    uint32_t i;
    int line = 0, col = 0;
    uint64_t any = 0;

    for (i = 0; i < f->len; i++)
        any |= f->n[i];
    if (!any && !f->act && !all)
        return;
    if (b->has_debug && b->debug.pc2line_buf) {
        /* the first line of the function, as the pc2line table starts */
        const uint8_t *p = b->debug.pc2line_buf, *end = p + b->debug.pc2line_len;
        uint32_t v;
        if (get_leb128(&v, p, end) > 0)
            line = v + 1;
        (void)col;
    }
    rv_printf("F %u len=%u args=%u vars=%u stack=%u varrefs=%u closures=%u cpool=%d kind=%u mode=%u derived=%u act=%llu fromc=%llu line=%d ran=%d name=%s file=%s\n",
              f->id, f->len, b->arg_count, b->var_count, b->stack_size,
              b->var_ref_count, b->closure_var_count, b->cpool_count,
              b->func_kind, b->js_mode, b->is_derived_class_constructor,
              (unsigned long long)f->act, (unsigned long long)f->from_c, line, (any || f->act) ? 1 : 0,
              JS_AtomGetStrRT(rt, nb, sizeof nb, b->func_name),
              b->has_debug ? JS_AtomGetStrRT(rt, fb, sizeof fb, b->debug.filename) : "?");
    for (i = 0; i < (uint32_t)(b->arg_count + b->var_count); i++) {
        JSBytecodeVarDef *vd = &b->vardefs[i];
        rv_printf("V %u %u %u %u %u %u\n", f->id, i, vd->is_captured,
                  vd->is_lexical, vd->is_const, vd->var_kind);
    }
    {
        static const char hex[] = "0123456789abcdef";
        char line_buf[512 + 32];
        uint32_t pos = 0;
        while (pos < f->len) {
            int k = snprintf(line_buf, 32, "B %u %u ", f->id, pos);
            uint32_t j;
            for (j = 0; j < 240 && pos < f->len; j++, pos++) {
                line_buf[k++] = hex[b->byte_code_buf[pos] >> 4];
                line_buf[k++] = hex[b->byte_code_buf[pos] & 15];
            }
            line_buf[k++] = '\n';
            js_rv_emit(line_buf, k);
        }
    }
    for (i = 0; i < f->len; i++)
        if (f->n[i])
            rv_printf("C %u %u %llu %llu\n", f->id, i,
                      (unsigned long long)f->n[i], (unsigned long long)f->rc[i]);
}

/* when the function is freed, its record goes out first (an id is never
   used twice, the address of a function may be) */
static void rv_release(JSRuntime *rt, JSFunctionBytecode *b)
{
    uint32_t h = rv_hash_ptr(b);
    RVCountFunc **pf, *f;
    for (pf = &rv_tab[h]; (f = *pf); pf = &f->next)
        if (f->b == b) {
            rv_dump_func(rt, f, 0);
            *pf = f->next;
            f->b = NULL;
            return;
        }
}

void JS_RegvmCount(JSRuntime *rt, int on)
{
    (void)rt;
    js_rv_on = on;
}

/* Q id 1 code_len, or Q id 0 and the refusal (JS_RegvmStat 3 and 4), then
   for a function the pass converts an R line for each register instruction
   (offset, old offset, 1 when it is a move at the end of the block before a
   leader, name) and an L line for each leader */
static void rv_trace_func(JSContext *ctx, RVCountFunc *f)
{
    JSFunctionBytecode *b = f->b;
    RVFunc *rf;
    int i;
    if (!rv_trace_off) {
        rv_trace_off = js_rv_alloc(sizeof(int32_t) * RV_TRACE_MAX);
        rv_trace_pos = js_rv_alloc(sizeof(int32_t) * RV_TRACE_MAX);
        rv_trace_leader = js_rv_alloc(30001);
    }
    rv_trace_on = 1;
    rv_trace_n = 0;
    memset(rv_trace_leader, 0, 30001);
    rf = rv_build(ctx->rt, b, NULL);
    rv_trace_on = 0;
    if (!rf) {
        rv_printf("Q %u 0 %d %d\n", f->id, ctx->rt->rv_stat[3], ctx->rt->rv_stat[4]);
        return;
    }
    rv_printf("Q %u 1 %u\n", f->id, rf->code_len);
    for (i = 0; i < rv_trace_n; i++)
        rv_printf("R %u %d %d %d %s\n", f->id, rv_trace_off[i], rv_trace_pos[i] & ~(1 << 30),
                  (rv_trace_pos[i] >> 30) & 1, JS_RegvmOpName(rf->code[rv_trace_off[i]]));
    for (i = 0; i < (int)b->byte_code_len && i < 30001; i++)
        if (rv_trace_leader[i])
            rv_printf("L %u %d\n", f->id, i);
    rv_free_func(ctx->rt, rf);
}

void JS_RegvmDump(JSRuntime *rt)
{
    RVCountFunc *f;
    uint32_t h;
    RVCall *c;
    struct list_head *el;
    /* every function still alive, also those that never ran: the static
       counts of the functions the conversion would refuse */
    list_for_each(el, &rt->gc_obj_list) {
        JSGCObjectHeader *gp = list_entry(el, JSGCObjectHeader, link);
        if (js_rc(gp)->gc_obj_type == JS_GC_OBJ_TYPE_FUNCTION_BYTECODE)
            rv_func((JSFunctionBytecode *)gp);
    }
    for (f = rv_all; f; f = f->all_next)
        if (f->b)
            rv_dump_func(rt, f, 1);
    if (!list_empty(&rt->context_list)) {
        JSContext *ctx = list_entry(rt->context_list.next, JSContext, link);
        for (f = rv_all; f; f = f->all_next)
            if (f->b)
                rv_trace_func(ctx, f);
    }
    for (h = 0; h < RV_HASH; h++)
        for (c = rv_calls[h]; c; c = c->next)
            rv_printf("K %u %u %u %d %d %llu\n", c->caller, c->off, c->callee,
                      c->argc, c->kind, (unsigned long long)c->n);
}

static inline void rv_hit(RVCountFunc **pf, int *prev, JSFunctionBytecode *b,
                          const uint8_t *pc, const JSValue *sp,
                          const JSValue *stack_buf)
{
    RVCountFunc *f;
    int off;
    if (!js_rv_on)
        return;
    f = *pf;
    if (!f || f->b != b)
        f = *pf = rv_func(b);
    off = pc - b->byte_code_buf;
    if (*prev >= 0 && sp > stack_buf && JS_VALUE_GET_TAG(sp[-1]) < 0)
        f->rc[*prev]++;
    if ((uint32_t)off < f->len)
        f->n[off]++;
    *prev = off;
    rv_site_off = off; /* the call opcodes read it */
    rv_site_id = f->id;
}
