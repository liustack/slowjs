/* Copies of a 16-byte pair of 8-byte words, as the interpreter copies a
   JSValue. */
typedef struct {
    unsigned long long u;
    long long tag;
} Pair;

/* LLVM's SLP vectorizer joins these two 8-byte moves into one 16-byte move. */
void copy_fields(Pair *d, const Pair *s)
{
    d->u = s->u;
    d->tag = s->tag;
}

/* A copy of the whole struct is a memcpy of 16 bytes, which LLVM expands. */
void copy_struct(Pair *d, const Pair *s)
{
    *d = *s;
}
