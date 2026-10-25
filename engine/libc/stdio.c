/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/* snprintf and vsnprintf for the formats the engine uses.
 *
 * Flags: - + space 0 #. Width and precision, a number or *. Length: hh h l ll
 * z j t. Conversions: d i u x X o c s p and %%. Output goes to a buffer and
 * is cut to fit, and the result is the length the whole output has, as in C.
 * There are no floating-point conversions, and any conversion not listed is
 * a trap, so a format the engine starts to use cannot print the wrong thing
 * without notice.
 */
#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>

typedef struct {
	char *buf;
	size_t size;	/* bytes of buf, 0 when there is none */
	size_t len;	/* characters produced, written or not */
} Out;

typedef struct {
	int left, plus, space, zero, alt;
	int width;	/* 0 when none */
	int prec;	/* -1 when none */
} Spec;

enum { LEN_NONE, LEN_HH, LEN_H, LEN_L, LEN_LL, LEN_Z, LEN_J, LEN_T };

static void out_char(Out *o, char c)
{
	if (o->len + 1 < o->size)
		o->buf[o->len] = c;
	o->len++;
}

static void out_fill(Out *o, char c, size_t n)
{
	for (; n; n--)
		out_char(o, c);
}

/* One integer. prefix is "" or what goes in front: a sign or 0x. */
static void out_integer(Out *o, const Spec *sp, const char *prefix, uint64_t v,
			int base, int upper)
{
	char digits[24];	/* 22 digits for 64 bits in base 8 */
	const char *alpha = upper ? "0123456789ABCDEF" : "0123456789abcdef";
	size_t nd = 0, np = 0, zeros, total, pad;
	int prec = sp->prec;

	if (v || prec != 0) {
		do {
			digits[nd++] = alpha[v % base];
			v /= base;
		} while (v);
	}
	/* # in base 8 makes sure the first digit is a zero. */
	if (base == 8 && sp->alt && (nd == 0 || digits[nd - 1] != '0'))
		digits[nd++] = '0';
	while (prefix[np])
		np++;
	zeros = prec > 0 && (size_t)prec > nd ? (size_t)prec - nd : 0;
	total = np + zeros + nd;
	if (sp->zero && !sp->left && prec < 0 && (size_t)sp->width > total)
		zeros += (size_t)sp->width - total, total = (size_t)sp->width;
	pad = (size_t)sp->width > total ? (size_t)sp->width - total : 0;
	if (!sp->left)
		out_fill(o, ' ', pad);
	for (size_t i = 0; i < np; i++)
		out_char(o, prefix[i]);
	out_fill(o, '0', zeros);
	while (nd)
		out_char(o, digits[--nd]);
	if (sp->left)
		out_fill(o, ' ', pad);
}

int vsnprintf(char *restrict buf, size_t size, const char *restrict fmt, va_list ap)
{
	Out o = { buf, size, 0 };

	for (; *fmt; fmt++) {
		Spec sp = { 0, 0, 0, 0, 0, 0, -1 };
		int len = LEN_NONE;
		char conv;

		if (*fmt != '%') {
			out_char(&o, *fmt);
			continue;
		}
		for (fmt++;; fmt++) {
			if (*fmt == '-') sp.left = 1;
			else if (*fmt == '+') sp.plus = 1;
			else if (*fmt == ' ') sp.space = 1;
			else if (*fmt == '0') sp.zero = 1;
			else if (*fmt == '#') sp.alt = 1;
			else break;
		}
		if (*fmt == '*') {
			sp.width = va_arg(ap, int);
			if (sp.width < 0) {
				sp.left = 1;
				sp.width = -sp.width;
			}
			fmt++;
		} else {
			for (; *fmt >= '0' && *fmt <= '9'; fmt++)
				sp.width = sp.width * 10 + (*fmt - '0');
		}
		if (*fmt == '.') {
			fmt++;
			sp.prec = 0;
			if (*fmt == '*') {
				sp.prec = va_arg(ap, int);	/* negative: none */
				fmt++;
			} else {
				for (; *fmt >= '0' && *fmt <= '9'; fmt++)
					sp.prec = sp.prec * 10 + (*fmt - '0');
			}
		}
		switch (*fmt) {
		case 'h':
			len = fmt[1] == 'h' ? LEN_HH : LEN_H;
			fmt += len == LEN_HH ? 2 : 1;
			break;
		case 'l':
			len = fmt[1] == 'l' ? LEN_LL : LEN_L;
			fmt += len == LEN_LL ? 2 : 1;
			break;
		case 'z': len = LEN_Z; fmt++; break;
		case 'j': len = LEN_J; fmt++; break;
		case 't': len = LEN_T; fmt++; break;
		}
		conv = *fmt;
		switch (conv) {
		case 'd':
		case 'i': {
			int64_t v;
			uint64_t mag;
			char prefix[2] = { 0, 0 };

			switch (len) {
			case LEN_HH: v = (signed char)va_arg(ap, int); break;
			case LEN_H: v = (short)va_arg(ap, int); break;
			case LEN_L: v = va_arg(ap, long); break;
			case LEN_LL: v = va_arg(ap, long long); break;
			case LEN_Z:
			case LEN_T: v = va_arg(ap, ptrdiff_t); break;
			case LEN_J: v = va_arg(ap, intmax_t); break;
			default: v = va_arg(ap, int); break;
			}
			mag = v < 0 ? -(uint64_t)v : (uint64_t)v;
			prefix[0] = v < 0 ? '-' : sp.plus ? '+' : sp.space ? ' ' : 0;
			out_integer(&o, &sp, prefix, mag, 10, 0);
			break;
		}
		case 'u':
		case 'x':
		case 'X':
		case 'o': {
			uint64_t v;

			switch (len) {
			case LEN_HH: v = (unsigned char)va_arg(ap, unsigned); break;
			case LEN_H: v = (unsigned short)va_arg(ap, unsigned); break;
			case LEN_L: v = va_arg(ap, unsigned long); break;
			case LEN_LL: v = va_arg(ap, unsigned long long); break;
			case LEN_Z:
			case LEN_T: v = va_arg(ap, size_t); break;
			case LEN_J: v = va_arg(ap, uintmax_t); break;
			default: v = va_arg(ap, unsigned); break;
			}
			out_integer(&o, &sp,
				    sp.alt && v && conv == 'x' ? "0x" :
				    sp.alt && v && conv == 'X' ? "0X" : "",
				    v, conv == 'u' ? 10 : conv == 'o' ? 8 : 16, conv == 'X');
			break;
		}
		case 'p': {
			uintptr_t v = (uintptr_t)va_arg(ap, void *);

			sp.prec = -1;
			sp.zero = 0;
			out_integer(&o, &sp, "0x", v, 16, 0);
			break;
		}
		case 'c': {
			size_t pad = sp.width > 1 ? (size_t)sp.width - 1 : 0;

			if (!sp.left)
				out_fill(&o, ' ', pad);
			out_char(&o, (char)(unsigned char)va_arg(ap, int));
			if (sp.left)
				out_fill(&o, ' ', pad);
			break;
		}
		case 's': {
			const char *s = va_arg(ap, const char *);
			size_t n = 0, pad;

			if (!s)
				s = "(null)";
			while ((sp.prec < 0 || n < (size_t)sp.prec) && s[n])
				n++;
			pad = (size_t)sp.width > n ? (size_t)sp.width - n : 0;
			if (!sp.left)
				out_fill(&o, ' ', pad);
			for (size_t i = 0; i < n; i++)
				out_char(&o, s[i]);
			if (sp.left)
				out_fill(&o, ' ', pad);
			break;
		}
		case '%':
			out_char(&o, '%');
			break;
		default:
			__builtin_trap();
		}
	}
	if (size)
		buf[o.len < size ? o.len : size - 1] = 0;
	return (int)o.len;
}

int snprintf(char *restrict buf, size_t size, const char *restrict fmt, ...)
{
	va_list ap;
	int n;

	va_start(ap, fmt);
	n = vsnprintf(buf, size, fmt, ap);
	va_end(ap);
	return n;
}
