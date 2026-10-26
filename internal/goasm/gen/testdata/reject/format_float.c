// A conversion engine/libc/stdio.c calls abort() on, written in a format.
int vsnprintf(char *, unsigned long, const char *, __builtin_va_list);

int format_float(char *buf, __builtin_va_list ap)
{
    return vsnprintf(buf, 64, "x=%f", ap);
}
