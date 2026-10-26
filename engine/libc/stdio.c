/*
 * Copyright (c) 2026 The slowjs Authors.
 * SPDX-License-Identifier: MIT
 * Use of this source code is governed by the MIT license in LICENSE-MIT at the
 * root of this repository.
 */
/* snprintf and vsnprintf for the formats the engine uses.
 *
 * Flags: - + space 0 #. Width and precision, a number or *. Length: hh h l ll
 * z j t, on d i u x X o only. Conversions: d i u x X o c s p and %%. Output
 * goes to a buffer and is cut to fit, and the result is the length the whole
 * output has, as in C.
 *
 * Whatever else calls abort: floating-point conversions, %n, positional
 * arguments, a length on c s p or %%, a flag, width or precision on %%
 * (written there, whatever a * would be given), a format that ends inside a
 * conversion, a width or precision that does not fit an int, and an output
 * longer than INT_MAX. A conversion is read and checked as a whole, and none of
 * its arguments, the * included, is taken before it is found good. So a format
 * the engine starts to use cannot print the wrong thing without notice. This is
 * the abort of engine/libc, which asks the host, and not a trap: a trap is a
 * signal, and in Go a signal ends the process.
 */
#include <stdarg.h>
#include <stddef.h>
#include <limits.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>

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

/* n more characters are produced, written or not. */
static void out_count(Out *o, size_t n)
{
	if (n > (size_t)INT_MAX - o->len)
		abort();
	o->len += n;
}

static void out_char(Out *o, char c)
{
	if (o->len + 1 < o->size)
		o->buf[o->len] = c;
	out_count(o, 1);
}

/* Only what fits is written, however long the fill. */
static void out_fill(Out *o, char c, size_t n)
{
	size_t room = o->len + 1 < o->size ? o->size - 1 - o->len : 0;
	size_t i;

	for (i = 0; i < n && i < room; i++)
		o->buf[o->len + i] = c;
	out_count(o, n);
}

/* A width or a precision: digits, and an abort when they do not fit an int. */
static int parse_number(const char *restrict *pp)
{
	const char *p = *pp;
	int n = 0;

	for (; *p >= '0' && *p <= '9'; p++) {
		if (n > (INT_MAX - (*p - '0')) / 10)
			abort();
		n = n * 10 + (*p - '0');
	}
	*pp = p;
	return n;
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
		int width_star = 0, prec_star = 0, has_width = 0, has_prec = 0;
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
		/* The whole conversion is read first, and no argument with it. */
		if (*fmt == '*') {
			width_star = has_width = 1;
			fmt++;
		} else if (*fmt >= '0' && *fmt <= '9') {
			has_width = 1;
			sp.width = parse_number(&fmt);
		}
		if (*fmt == '.') {
			has_prec = 1;
			fmt++;
			if (*fmt == '*') {
				prec_star = 1;
				fmt++;
			} else {
				sp.prec = parse_number(&fmt);
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
		/* What the conversion is, checked by what is written in the format and
		   not by the values of its arguments: a length means something for the
		   integers only, and %% takes no flag, width or precision, not even
		   a * that happens to be 0. */
		switch (conv) {
		case 'd': case 'i': case 'u': case 'x': case 'X': case 'o':
			break;
		case 'c': case 's': case 'p':
			if (len != LEN_NONE)
				abort();
			break;
		case '%':
			if (len != LEN_NONE || sp.left || sp.plus || sp.space || sp.zero ||
			    sp.alt || has_width || has_prec)
				abort();
			break;
		default:
			abort();
		}
		/* Now the arguments: the width, the precision, and the value. */
		if (width_star) {
			sp.width = va_arg(ap, int);
			if (sp.width == INT_MIN)
				abort();
			if (sp.width < 0) {
				sp.left = 1;
				sp.width = -sp.width;
			}
		}
		if (prec_star) {
			sp.prec = va_arg(ap, int);	/* negative: none */
			if (sp.prec < 0)
				sp.prec = -1;
		}
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
			abort();
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
