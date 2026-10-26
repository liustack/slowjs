// A format that comes from outside the function and no wrapper forwards.
int vsnprintf(char *, unsigned long, const char *, __builtin_va_list);

int format_variable(char *buf, const char *fmt, __builtin_va_list ap)
{
    return vsnprintf(buf, 64, fmt, ap);
}
