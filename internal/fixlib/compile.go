// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

var (
	reDbufClaim      = regexp.MustCompile(`\nfunc Xdbuf_claim\(tls \*libc\.TLS, (\w+) uintptr, len1 Tsize_t\) \(r int32\) \{\n`)
	reCreateFunction = regexp.MustCompile(`\nfunc _js_create_function\(tls \*libc\.TLS, (\w+) uintptr, (\w+) uintptr\) \(r TJSValue\) \{\n`)
	reResolveVars    = regexp.MustCompile(`\nfunc _resolve_variables\(tls \*libc\.TLS, \w+ uintptr, \w+ uintptr\) \(r int32\) \{\n`)
	reResolveLabels  = regexp.MustCompile(`\nfunc _resolve_labels\(tls \*libc\.TLS, (\w+) uintptr, (\w+) uintptr\) \(r int32\) \{\n`)
	reParseErrorV    = regexp.MustCompile(`\nfunc _js_parse_error_v\(tls \*libc\.TLS, (\w+) uintptr, [^\n]*\{\n`)
	reLreCompile     = regexp.MustCompile(`\nfunc Xlre_compile\(tls \*libc\.TLS, [^\n]*\{\n`)
	reScopeVarCall   = regexp.MustCompile(`(\n(\t+)pos_next = _resolve_scope_var\(tls, [^\n]*\)\n)(\t+if pos_next < 0 \{\n\t+goto fail\n\t+\}\n)?\t+XJS_FreeAtom\(tls, \w+, var_name\)\n`)
	reWriteAtoms     = regexp.MustCompile(`\nfunc _JS_WriteObjectAtoms\(tls \*libc\.TLS, (\w+) uintptr\) \(r int32\) \{\n`)
	reWriteObject2   = regexp.MustCompile(`\nfunc XJS_WriteObject2\(tls \*libc\.TLS, (\w+) uintptr, [^\n]*\{\n`)
	reCodeMove       = regexp.MustCompile(`\n(\t+)if (?:\(\*TDynBuf\)\(unsafe\.Pointer\(bc\)\)\.Ferror1 != 0 \|\| )?Xdbuf_claim\(tls, bc, (?:libc\.Uint(?:32|64)FromInt32|uint64)\(chunk_size\)\) != 0 \{\n(?:\t+XJS_ThrowOutOfMemory\(tls, \(\*TJSParseState\)\(unsafe\.Pointer\(\w+\)\)\.Fctx\)\n)?\t+(return -int32\(1\)|goto fail)\n`)
	reNextTokenFree  = regexp.MustCompile(`\n\t_free_token\(tls, s, s\+\d+\)\n`)
	reParseClass     = regexp.MustCompile(`\nfunc _js_parse_class\(tls \*libc\.TLS, s uintptr, [^\n]*\{\n`)
	reBrandPatch     = regexp.MustCompile(`\n\t\t` + reDeref(`Tuint8_t`, `\(\*TJSFunctionDef\)\(unsafe\.Pointer\(\(\*TClassFieldsDef\)\(unsafe\.Pointer\((\w+)\)\)\.Ffields_init_fd\)\)\.Fbyte_code\.Fbuf \+ uintptr\(\(\*TClassFieldsDef\)\(unsafe\.Pointer\(\w+\)\)\.Fbrand_push_pos\)`) + ` = uint8\(EOP_push_true\)\n`)
	reDestructElem   = regexp.MustCompile(`\nfunc _js_parse_destructuring_element\(tls \*libc\.TLS, s uintptr, [^\n]*\{\n`)
	reTestRemoval    = regexp.MustCompile(`\n\t\tlibc\.(?:X__builtin___memset_chk|Xmemset)\(tls, \(\*TJSFunctionDef\)\(unsafe\.Pointer\(\(\*TJSParseState\)\(unsafe\.Pointer\(s\)\)\.Fcur_func\)\)\.Fbyte_code\.Fbuf\+uintptr\(start_addr\), int32\(EOP_nop\), `)
)

// curFuncLost is true in a parser function when the code of the function
// being parsed is lost.
const curFuncLost = "(*TJSFunctionDef)(unsafe.Pointer((*TJSParseState)(unsafe.Pointer(s)).Fcur_func)).Fbyte_code.Ferror1 != 0"

// parserOutOfMemory throws the out of memory error in a parser function.
const parserOutOfMemory = "XJS_ThrowOutOfMemory(tls, (*TJSParseState)(unsafe.Pointer(s)).Fctx)\n"

// fixCompile lets the compiler run out of memory safely, the Go form of the
// same change in lib/internal/patch/quickjs.diff. The parser does not stop
// when the buffer it emits code into fails to grow, and the passes after it
// did not expect that buffer, or their own, to fail:
//
//   - Xdbuf_claim takes no more bytes once growing failed, so that a
//     buffer holds a run of whole writes and never a later short write
//     after a failed one.
//   - _js_create_function fails with the out of memory error on code whose
//     buffer failed. It shrinks the line table of a function with
//     Xjs_realloc_rt rather than Xjs_realloc, which throws the out of
//     memory error: it keeps the table when that fails, and the compile
//     used to work and leave the error behind.
//   - _resolve_variables stops when _resolve_scope_var fails. It used to
//     take the -1 as the next position, read before the code and copy it
//     again from the start without taking its atoms.
//   - _resolve_labels stops at the first write that failed. It used to go
//     on and patch jumps at offsets past the end of the buffer. When it
//     fails, it keeps the new code, whose atoms it owns, rather than the
//     old code, some of whose atoms it already freed, and it frees the
//     relocations of the labels it did not reach, which used to leak.
//   - _push_scope marks the code of the function lost when growing its
//     scope array fails, and from then on _push_scope and _pop_scope leave
//     the scopes as they are, and _push_scope fails with the out of memory
//     error. Most callers go on after a failed push, and the pop after it
//     took off the scope around it instead, until a pop read before the
//     scope array. The callers that stop return the failure as it is, so
//     a push that failed without an exception ended the compile with none.
//   - The parser throws the out of memory error when moving the code of a
//     loop fails, where it failed without an exception, and does not move
//     code whose buffer already failed.
//   - The parser stops with the out of memory error once the code of the
//     function it parses is lost: _next_token checks it before each token,
//     and so do the places that patch code in place before reading
//     another token, the constant pool index of a class constructor and
//     the brand of its fields in _js_parse_class, and the test that
//     _js_parse_destructuring_element removes. The parser goes on after a
//     failed emit and patches code at positions it noted, which may then be
//     past what the buffer holds: a Go panic on an empty buffer, or a
//     write past the end of a full one.
//   - _js_parse_error_v reports the out of memory error when the code
//     buffer failed: what the parser reads back of the code is wrong then,
//     and it finds syntax errors the script does not have.
//   - XJS_WriteObject2 and _JS_WriteObjectAtoms fail with the out of memory
//     error when the buffer they write to failed. They used to return the
//     bytecode cut short as if it were whole, or fail without an
//     exception. _bc_put_atom marks that buffer failed when it cannot add
//     an atom to the atom table, as most of its callers write on: the
//     bytecode came out with the error left pending.
//   - Xlre_compile reports running out of memory when the regexp parser
//     failed without a message, or after writing its code failed.
func fixCompile(b []byte) ([]byte, error) {
	// dbuf_claim.
	body, err := funcBody(b, reDbufClaim, "Xdbuf_claim")
	if err != nil {
		return nil, err
	}
	s := string(reDbufClaim.FindSubmatch(b)[1])
	// The size it keeps only stops later writes if growing fails at once on
	// a failed buffer.
	grow := fmt.Sprintf("\tif new_size > (*TDynBuf)(unsafe.Pointer(%[1]s)).Fallocated_size {\n\t\tif (*TDynBuf)(unsafe.Pointer(%[1]s)).Ferror1 != 0 {\n\t\t\treturn -int32(1)\n\t\t}\n", s)
	if bytes.Count(body, []byte(grow)) != 1 {
		return nil, fmt.Errorf("Xdbuf_claim: growing the buffer no longer starts with the test of a failed buffer")
	}
	keep := fmt.Sprintf("\t\t\t(*TDynBuf)(unsafe.Pointer(%[1]s)).Fallocated_size = (*TDynBuf)(unsafe.Pointer(%[1]s)).Fsize\n", s)
	failed := regexp.MustCompile(`\n\t\tif !\(new_buf != 0\) \{\n\t\t\t\(\*TDynBuf\)\(unsafe\.Pointer\(` + s + `\)\)\.Ferror1 = int32\(\w+\)\n`)
	fixed, err := insertAt("Xdbuf_claim", body, keep, func(body []byte) (int, error) {
		loc := findAll(failed, body)
		if len(loc) != 1 {
			return 0, fmt.Errorf("Xdbuf_claim: cannot find the failed realloc")
		}
		return loc[0][1], nil
	})
	if err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

	// js_create_function.
	if body, err = funcBody(b, reCreateFunction, "_js_create_function"); err != nil {
		return nil, err
	}
	m := reCreateFunction.FindSubmatch(b)
	ctx, fd := string(m[1]), string(m[2])
	check := fmt.Sprintf("\tif (*TJSFunctionDef)(unsafe.Pointer(%s)).Fbyte_code.Ferror1 != 0 {\n\t\tXJS_ThrowOutOfMemory(tls, %s)\n\t\tgoto fail\n\t}\n", fd, ctx)
	fixed, err = insertAt("_js_create_function", body, check, func(body []byte) (int, error) {
		return before(body, "\t/* recompute scope linkage */\n", "\nfail:\n", "_js_create_function: cannot find the start or the fail label")
	})
	if err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

	// The line table js_create_function shrinks.
	if body, err = funcBody(b, reCreateFunction, "_js_create_function"); err != nil {
		return nil, err
	}
	shrink := fmt.Sprintf(".Fdebug.Fpc2line_buf = Xjs_realloc_rt(tls, (*TJSContext)(unsafe.Pointer(%s)).Frt, (*TJSFunctionDef)(unsafe.Pointer(%s)).Fpc2line.Fbuf, ", ctx, fd)
	old := fmt.Sprintf(".Fdebug.Fpc2line_buf = Xjs_realloc(tls, %s, (*TJSFunctionDef)(unsafe.Pointer(%s)).Fpc2line.Fbuf, ", ctx, fd)
	switch olds, news := bytes.Count(body, []byte(old)), bytes.Count(body, []byte(shrink)); {
	case olds == 1 && news == 0:
		b = bytes.Replace(b, body, bytes.Replace(body, []byte(old), []byte(shrink), 1), 1)
	case olds != 0 || news != 1:
		return nil, fmt.Errorf("_js_create_function: want 1 place that shrinks the line table, found %d with Xjs_realloc and %d with Xjs_realloc_rt", olds, news)
	}

	// resolve_variables.
	if body, err = funcBody(b, reResolveVars, "_resolve_variables"); err != nil {
		return nil, err
	}
	// Each call of _resolve_scope_var is followed by the check or, as
	// generated, by the free of the name.
	calls := findAll(reScopeVarCall, body)
	if len(calls) != 2 || !bytes.Contains(body, []byte("\nfail:\n")) {
		return nil, fmt.Errorf("_resolve_variables: want 2 calls of _resolve_scope_var and a fail label, found %d calls", len(calls))
	}
	fixed = body
	for i := len(calls) - 1; i >= 0; i-- {
		indent := string(body[calls[i][4]:calls[i][5]])
		if calls[i][6] < 0 {
			fixed = splice(fixed, calls[i][3], indent+"if pos_next < 0 {\n"+indent+"\tgoto fail\n"+indent+"}\n")
		}
	}
	if n := bytes.Count(fixed, []byte("if pos_next < 0 {")); n != len(calls) {
		return nil, fmt.Errorf("_resolve_variables: %d tests of pos_next for %d calls of _resolve_scope_var", n, len(calls))
	}
	b = bytes.Replace(b, body, fixed, 1)

	// resolve_labels.
	if b, err = fixResolveLabels(b); err != nil {
		return nil, err
	}

	// push_scope and pop_scope.
	if b, err = fixScopes(b); err != nil {
		return nil, err
	}

	// The loops whose code the parser moves.
	for _, name := range []string{"_js_parse_for_in_of", "_js_parse_statement_or_decl"} {
		re := regexp.MustCompile(`\nfunc ` + name + `\(tls \*libc\.TLS, (\w+) uintptr, [^\n]*\{\n`)
		if body, err = funcBody(b, re, name); err != nil {
			return nil, err
		}
		ps := string(re.FindSubmatch(b)[1])
		throw := fmt.Sprintf("XJS_ThrowOutOfMemory(tls, (*TJSParseState)(unsafe.Pointer(%s)).Fctx)\n", ps)
		loc := findAll(reCodeMove, body)
		if len(loc) != 1 {
			return nil, fmt.Errorf("%s: want 1 claim for moving code, found %d", name, len(loc))
		}
		indent := string(body[loc[0][2]:loc[0][3]])
		fixed, err := insertAt(name, body, indent+"\t"+throw, func(body []byte) (int, error) {
			loc := findAll(reCodeMove, body)
			if len(loc) != 1 {
				return 0, fmt.Errorf("%s: want 1 claim for moving code, found %d", name, len(loc))
			}
			return loc[0][4] - len(indent) - 1, nil // the start of the return or goto
		})
		if err != nil {
			return nil, err
		}
		lost := "(*TDynBuf)(unsafe.Pointer(bc)).Ferror1 != 0 || "
		if claim := loc[0][3] + len("if "); string(fixed[claim:claim+len(lost)]) != lost {
			fixed = splice(fixed, claim, lost)
		}
		b = bytes.Replace(b, body, fixed, 1)
	}

	// The parser stops once the code is lost.
	if b, err = fixParserStops(b); err != nil {
		return nil, err
	}

	// js_parse_error_v.
	if body, err = funcBody(b, reParseErrorV, "_js_parse_error_v"); err != nil {
		return nil, err
	}
	ps := string(reParseErrorV.FindSubmatch(b)[1])
	check = fmt.Sprintf("\tif (*TJSParseState)(unsafe.Pointer(%[1]s)).Fcur_func != 0 && (*TJSFunctionDef)(unsafe.Pointer((*TJSParseState)(unsafe.Pointer(%[1]s)).Fcur_func)).Fbyte_code.Ferror1 != 0 {\n\t\tXJS_ThrowOutOfMemory(tls, ctx)\n\t\treturn -int32(1)\n\t}\n", ps)
	if fixed, err = insertAt("_js_parse_error_v", body, check, func(body []byte) (int, error) {
		return after(body, fmt.Sprintf("\tctx = (*TJSParseState)(unsafe.Pointer(%s)).Fctx\n", ps), "\t_JS_ThrowError2(tls, ctx, ", "_js_parse_error_v: cannot find where it reads the context")
	}); err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

	// The bytecode writer.
	if b, err = fixWriteObject(b); err != nil {
		return nil, err
	}

	// lre_compile.
	if body, err = funcBody(b, reLreCompile, "Xlre_compile"); err != nil {
		return nil, err
	}
	// The first buffer it frees is the code, the state's first field.
	re := regexp.MustCompile(`\nerror:\n\t;\n(?:\tif [^\n]*\n\t\t_re_parse_out_of_memory\(tls, \w+\)\n\t\}\n)?(\tXdbuf_free\(tls, (\w+)\)\n\tXdbuf_free\(tls, \w+\+\d+\)\n)\tXpstrcpy\(tls, error_msg, error_msg_size, (\w+\+\d+)\)\n`)
	loc := findAll(re, body)
	if len(loc) != 1 {
		return nil, fmt.Errorf("Xlre_compile: cannot find the error label")
	}
	state, msg := string(body[loc[0][4]:loc[0][5]]), string(body[loc[0][6]:loc[0][7]])
	add := fmt.Sprintf("\tif *(*byte)(unsafe.Pointer(%s)) == 0 || (*TDynBuf)(unsafe.Pointer(%s)).Ferror1 != 0 {\n\t\t_re_parse_out_of_memory(tls, %s)\n\t}\n", msg, state, state)
	if fixed, err = insertAt("Xlre_compile", body, add, func(body []byte) (int, error) {
		loc := findAll(re, body)
		if len(loc) != 1 {
			return 0, fmt.Errorf("Xlre_compile: cannot find the error label")
		}
		return loc[0][2], nil
	}); err != nil {
		return nil, err
	}
	return bytes.Replace(b, body, fixed, 1), nil
}

// fixResolveLabels makes _resolve_labels stop at a failed write, keep the
// new code when it fails, and free the relocations of the labels it did not
// reach then.
func fixResolveLabels(b []byte) ([]byte, error) {
	body, err := funcBody(b, reResolveLabels, "_resolve_labels")
	if err != nil {
		return nil, err
	}
	m := reResolveLabels.FindSubmatch(b)
	ctx := string(m[1])
	bc := regexp.MustCompile(`/\* bc_out at (bp\+\d+) \*/`).FindSubmatch(body)
	if bc == nil {
		return nil, fmt.Errorf("_resolve_labels: cannot find bc_out")
	}
	failed := fmt.Sprintf("if (*TDynBuf)(unsafe.Pointer(%s)).Ferror1 != 0 {\n", bc[1])
	// The end that fixlib writes keeps the new code and frees the
	// relocations left. That is right for the four failed calls of
	// _add_reloc, and the jump ccgo puts after the last return: a new way
	// to the label needs a look.
	if n := bytes.Count(body, []byte("goto fail\n")); n != 5 {
		return nil, fmt.Errorf("_resolve_labels: want 5 jumps to the fail label, found %d: check that the end fixlib writes is right for the new one", n)
	}

	// The new code, as the end sets it.
	set := regexp.MustCompile(`\t/\* set the new byte code \*/\n(\tXdbuf_free\(tls, [^\n]*\)\n\t\(\*TJSFunctionDef\)\(unsafe\.Pointer\(\w+\)\)\.Fbyte_code = [^\n]*\n\t\(\*TJSFunctionDef\)\(unsafe\.Pointer\(\w+\)\)\.Fuse_short_opcodes = int32\(\w+\)\n)`).FindSubmatch(body)
	if set == nil {
		return nil, fmt.Errorf("_resolve_labels: cannot find where it sets the new code")
	}

	head := []byte("\tpos = 0\n\tfor {\n\t\tif !(pos < bc_len) {\n\t\t\tbreak\n\t\t}\n")
	after := []byte("\t/* check that there were no missing labels */\n")
	fail := []byte(fmt.Sprintf("fail:\n\t;\n\t/* XXX: not safe */\n\tXdbuf_free(tls, %s)\n\treturn -int32(1)\n}\n", bc[1]))
	// The three parts of the change, whole: the test at the head of the
	// loop, the test after it, and the end. A file has all or none.
	inLoop := append(append([]byte{}, head...), "\t\t"+failed+"\t\t\tgoto out_of_memory\n\t\t}\n"...)
	afterLoop := append([]byte("\t"+failed+"\t\tgoto out_of_memory\n\t}\n"), after...)
	endHead := fmt.Sprintf("\nout_of_memory:\n\t;\n\tXJS_ThrowOutOfMemory(tls, %s)\nfail:\n\t;\n", ctx)
	endTail := string(set[1]) + "\treturn -int32(1)\n}\n"
	end := endHead[1:] + endTail
	parts := 0
	if bytes.Count(body, inLoop) == 1 {
		parts++
	}
	if bytes.Count(body, afterLoop) == 1 {
		parts++
	}
	// fixResolveLabelsRelocs puts its frees between the two.
	if i := bytes.Index(body, []byte(endHead)); i >= 0 && bytes.HasSuffix(body, []byte(endTail)) {
		if between := body[i+len(endHead) : len(body)-len(endTail)]; len(between) == 0 || bytes.HasPrefix(between, []byte("\t/* free the relocations of the labels not reached */\n")) {
			parts++
		}
	}
	if parts == 3 && bytes.Count(body, []byte("goto out_of_memory\n")) == 2 && bytes.Count(body, []byte("\nout_of_memory:\n")) == 1 {
		return fixResolveLabelsRelocs(b)
	}
	if parts != 0 || bytes.Contains(body, []byte("out_of_memory")) {
		return nil, fmt.Errorf("_resolve_labels: has out_of_memory or part of the change of fixlib: read the new code, upstream may have made part of the change")
	}
	if bytes.Count(body, head) != 1 || bytes.Count(body, after) != 1 || !bytes.HasSuffix(body, fail) {
		return nil, fmt.Errorf("_resolve_labels: cannot find the loop over the code, its end or the fail label")
	}
	fixed := bytes.Replace(body, head, inLoop, 1)
	fixed = bytes.Replace(fixed, after, afterLoop, 1)
	fixed = append(bytes.TrimSuffix(fixed, fail), end...)
	return fixResolveLabelsRelocs(bytes.Replace(b, body, fixed, 1))
}

// fixResolveLabelsRelocs makes the fail path of _resolve_labels free the
// relocations of the labels it did not reach. They used to leak.
func fixResolveLabelsRelocs(b []byte) ([]byte, error) {
	const marker = "\t/* free the relocations of the labels not reached */\n"
	body, err := funcBody(b, reResolveLabels, "_resolve_labels")
	if err != nil {
		return nil, err
	}
	m := reResolveLabels.FindSubmatch(b)
	ctx, fd := string(m[1]), string(m[2])
	fail := []byte("\nfail:\n\t;\n")
	next := []byte("\t\t\tre_next = (*TRelocEntry)(unsafe.Pointer(re)).Fnext\n")
	slots := []byte(fmt.Sprintf("\tlabel_slots = (*TJSFunctionDef)(unsafe.Pointer(%s)).Flabel_slots\n", fd))
	free := marker + fmt.Sprintf("\ti = 0\n\tfor ; i < (*TJSFunctionDef)(unsafe.Pointer(%[2]s)).Flabel_count; i++ {\n"+
		"\t\tls = label_slots + uintptr(i)*uintptr(unsafe.Sizeof(TLabelSlot{}))\n"+
		"\t\tfor re = (*TLabelSlot)(unsafe.Pointer(ls)).Ffirst_reloc; re != 0; re = re_next {\n"+
		"\t\t\tre_next = (*TRelocEntry)(unsafe.Pointer(re)).Fnext\n"+
		"\t\t\tXjs_free(tls, %[1]s, re)\n\t\t}\n"+
		"\t\t(*TLabelSlot)(unsafe.Pointer(ls)).Ffirst_reloc = 0\n\t}\n", ctx, fd)
	if bytes.Contains(body, []byte(marker)) && !bytes.Contains(body, []byte(free)) {
		return nil, fmt.Errorf("_resolve_labels: the comment of fixlib without what it frees")
	}
	fixed, err := insertAt("_resolve_labels", body, free, func(body []byte) (int, error) {
		if bytes.Count(body, fail) != 1 || !bytes.Contains(body, next) || bytes.Count(body, slots) != 1 {
			return 0, fmt.Errorf("_resolve_labels: cannot find the fail label, label_slots or how it frees a relocation")
		}
		return bytes.Index(body, fail) + len(fail), nil
	})
	if err != nil {
		return nil, err
	}
	return bytes.Replace(b, body, fixed, 1), nil
}

// splice inserts s into b at i.
func splice(b []byte, i int, s string) []byte {
	return append(append(append([]byte{}, b[:i]...), s...), b[i:]...)
}

// insertAt inserts text into body, the text of the function name, at the
// place where returns, or checks that it is there already. where gets the
// body without text, so that a body fixlib changed is checked against the
// place fixlib would pick for it. The text somewhere else, or more than
// once, stops fixlib: upstream may have written part of the change itself.
func insertAt(name string, body []byte, text string, where func([]byte) (int, error)) ([]byte, error) {
	switch n := bytes.Count(body, []byte(text)); n {
	case 0:
		i, err := where(body)
		if err != nil {
			return nil, err
		}
		return splice(body, i, text), nil
	case 1:
		i := bytes.Index(body, []byte(text))
		j, err := where(append(append([]byte{}, body[:i]...), body[i+len(text):]...))
		if err != nil {
			return nil, err
		}
		if j != i {
			return nil, fmt.Errorf("%s: %q is not where fixlib puts it: read the new code, upstream may have made part of the change", name, firstLine(text))
		}
		return body, nil
	default:
		return nil, fmt.Errorf("%s: %q %d times, want once: read the new code, upstream may have made part of the change", name, firstLine(text), n)
	}
}

// firstLine returns the first line of s, without its indent.
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// before returns the place of anchor, which body must have once, and
// requires need in body too.
func before(body []byte, anchor, need, msg string) (int, error) {
	if bytes.Count(body, []byte(anchor)) != 1 || !bytes.Contains(body, []byte(need)) {
		return 0, fmt.Errorf("%s", msg)
	}
	return bytes.Index(body, []byte(anchor)), nil
}

// after returns the place after anchor, which body must have once, and
// requires need in body too.
func after(body []byte, anchor, need, msg string) (int, error) {
	i, err := before(body, anchor, need, msg)
	return i + len(anchor), err
}

// fixWriteObject makes the bytecode writer check its buffer.
func fixWriteObject(b []byte) ([]byte, error) {
	body, err := funcBody(b, reWriteAtoms, "_JS_WriteObjectAtoms")
	if err != nil {
		return nil, err
	}
	s := string(reWriteAtoms.FindSubmatch(b)[1])
	throw := fmt.Sprintf("\t\tXJS_ThrowOutOfMemory(tls, (*TBCWriterState)(unsafe.Pointer(%s)).Fctx)\n", s)
	atoms := regexp.MustCompile(`\n\t_js_dbuf_init\(tls, \(\*TBCWriterState\)\(unsafe\.Pointer\(` + s + `\)\)\.Fctx, (` + s + `\+\d+)\)\n`).FindSubmatch(body)
	if atoms == nil {
		return nil, fmt.Errorf("_JS_WriteObjectAtoms: cannot find the atoms buffer")
	}
	lost := fmt.Sprintf("(*TDynBuf)(unsafe.Pointer(%s)).Ferror1 != 0 || ", atoms[1])
	claim := regexp.MustCompile(`\n\tif (` + regexp.QuoteMeta(lost) + `)?(Xdbuf_claim\(tls, bp, (?:libc\.Uint(?:32|64)FromInt32|uint64)\(atoms_size\)\) != 0) \{\n\t\tgoto fail\n`)
	fixed, err := insertAt("_JS_WriteObjectAtoms", body, throw, func(body []byte) (int, error) {
		loc := findAll(claim, body)
		if len(loc) != 1 {
			return 0, fmt.Errorf("_JS_WriteObjectAtoms: cannot find the claim that moves the atoms")
		}
		return loc[0][1] - len("\t\tgoto fail\n"), nil
	})
	if err != nil {
		return nil, err
	}
	// The claim, followed by the throw now, also tests the buffer.
	thrown := regexp.MustCompile(`\n\tif (` + regexp.QuoteMeta(lost) + `)?Xdbuf_claim\(tls, bp, [^\n]*\(atoms_size\)\) != 0 \{\n` + regexp.QuoteMeta(throw))
	m := findAll(thrown, fixed)
	if len(m) != 1 {
		return nil, fmt.Errorf("_JS_WriteObjectAtoms: cannot find the claim that moves the atoms")
	}
	if m[0][2] < 0 {
		fixed = splice(fixed, m[0][0]+len("\n\tif "), lost)
	}
	b = bytes.Replace(b, body, fixed, 1)

	re := regexp.MustCompile(`\nfunc _bc_put_atom\(tls \*libc\.TLS, s uintptr, atom TJSAtom\) \(r int32\) \{\n`)
	if body, err = funcBody(b, re, "_bc_put_atom"); err != nil {
		return nil, err
	}
	mark := "\t\t\t(*TBCWriterState)(unsafe.Pointer(s)).Fdbuf.Ferror1 = int32(1)\n"
	if fixed, err = insertAt("_bc_put_atom", body, mark, func(body []byte) (int, error) {
		return after(body, "\t\tif _bc_atom_to_idx(tls, s, bp, atom) != 0 {\n", "", "_bc_put_atom: cannot find where adding the atom fails")
	}); err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

	if body, err = funcBody(b, reWriteObject2, "XJS_WriteObject2"); err != nil {
		return nil, err
	}
	ctx := string(reWriteObject2.FindSubmatch(b)[1])
	init := regexp.MustCompile(`\n\t_js_dbuf_init\(tls, ` + ctx + `, (s\+\d+)\)\n`).FindSubmatch(body)
	if init == nil {
		return nil, fmt.Errorf("XJS_WriteObject2: cannot find the buffer")
	}
	check := fmt.Sprintf("\tif (*TDynBuf)(unsafe.Pointer(%s)).Ferror1 != 0 {\n\t\tXJS_ThrowOutOfMemory(tls, %s)\n\t\tgoto fail\n\t}\n", init[1], ctx)
	if fixed, err = insertAt("XJS_WriteObject2", body, check, func(body []byte) (int, error) {
		return after(body, "\tif _JS_WriteObjectRec(tls, s, obj) != 0 {\n\t\tgoto fail\n\t}\n", "", "XJS_WriteObject2: cannot find the call of _JS_WriteObjectRec")
	}); err != nil {
		return nil, err
	}
	return bytes.Replace(b, body, fixed, 1), nil
}

// fixScopes makes _push_scope and _pop_scope leave the scopes as they are
// once the function's code is lost.
func fixScopes(b []byte) ([]byte, error) {
	const lost = "(*TJSFunctionDef)(unsafe.Pointer(fd)).Fbyte_code.Ferror1"
	re := regexp.MustCompile(`\nfunc _push_scope\(tls \*libc\.TLS, s uintptr\) \(r int32\) \{\n`)
	body, err := funcBody(b, re, "_push_scope")
	if err != nil {
		return nil, err
	}
	check := "\t\tif " + lost + " != 0 {\n\t\t\tXJS_ThrowOutOfMemory(tls, (*TJSParseState)(unsafe.Pointer(s)).Fctx)\n\t\t\treturn -int32(1)\n\t\t}\n"
	failed := []byte("\t\t\t\tif !(new_buf != 0) {\n\t\t\t\t\treturn -int32(1)\n")
	marked := []byte("\t\t\t\tif !(new_buf != 0) {\n\t\t\t\t\t" + lost + " = int32(1)\n\t\t\t\t\treturn -int32(1)\n")
	if bytes.Count(body, marked)+bytes.Count(body, failed) != 2 {
		return nil, fmt.Errorf("_push_scope: want 2 failed reallocs")
	}
	fixed, err := insertAt("_push_scope", bytes.ReplaceAll(body, failed, marked), check, func(body []byte) (int, error) {
		return after(body, "\t\tscope = (*TJSFunctionDef)(unsafe.Pointer(fd)).Fscope_count\n", "", "_push_scope: cannot find where it reads the scope count")
	})
	if err != nil {
		return nil, err
	}
	b = bytes.Replace(b, body, fixed, 1)

	re = regexp.MustCompile(`\nfunc _pop_scope\(tls \*libc\.TLS, s uintptr\) \{\n`)
	if body, err = funcBody(b, re, "_pop_scope"); err != nil {
		return nil, err
	}
	check = "\t\tif " + lost + " != 0 {\n\t\t\treturn\n\t\t}\n"
	if fixed, err = insertAt("_pop_scope", body, check, func(body []byte) (int, error) {
		return after(body, "\t\tscope = (*TJSFunctionDef)(unsafe.Pointer(fd)).Fscope_level\n", "", "_pop_scope: cannot find where it reads the scope level")
	}); err != nil {
		return nil, err
	}
	return bytes.Replace(b, body, fixed, 1), nil
}

// fixParserStops makes the parser stop with the out of memory error once
// the code of the function it parses is lost: before each token, and before
// each patch of the code in place that does not wait for the next token.
func fixParserStops(b []byte) ([]byte, error) {
	check := "\tif (*TJSParseState)(unsafe.Pointer(s)).Fcur_func != 0 && " + curFuncLost + " {\n\t\t" + parserOutOfMemory + "\t\treturn -int32(1)\n\t}\n"
	b, err := guard(b, reNextToken, "_next_token", reNextTokenFree, check)
	if err != nil {
		return nil, err
	}

	body, err := funcBody(b, reParseClass, "_js_parse_class")
	if err != nil {
		return nil, err
	}
	if !bytes.Contains(body, []byte("\tctx = (*TJSParseState)(unsafe.Pointer(s)).Fctx\n")) || !bytes.Contains(body, []byte("\tfd = (*TJSParseState)(unsafe.Pointer(s)).Fcur_func\n")) || !bytes.Contains(body, []byte("\nfail:\n")) {
		return nil, fmt.Errorf("_js_parse_class: cannot find ctx, fd or the fail label")
	}
	ctor := regexp.MustCompile(`\n\t/\* patch the constant pool index for the constructor \*/\n\t\(\*Tpacked_u32\)\(unsafe\.Pointer\(\(\*TJSFunctionDef\)\(unsafe\.Pointer\(fd\)\)\.Fbyte_code\.Fbuf \+ uintptr\(ctor_cpool_offset\)\)\)\.Fv = `)
	if b, err = guardAfterComment(b, reParseClass, "_js_parse_class", ctor, "\tif (*TJSFunctionDef)(unsafe.Pointer(fd)).Fbyte_code.Ferror1 != 0 {\n\t\tXJS_ThrowOutOfMemory(tls, ctx)\n\t\tgoto fail\n\t}\n"); err != nil {
		return nil, err
	}
	m := reBrandPatch.FindSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("_js_parse_class: cannot find where it patches the brand")
	}
	brand := fmt.Sprintf("\t\tif (*TJSFunctionDef)(unsafe.Pointer((*TClassFieldsDef)(unsafe.Pointer(%s)).Ffields_init_fd)).Fbyte_code.Ferror1 != 0 {\n\t\t\tXJS_ThrowOutOfMemory(tls, ctx)\n\t\t\tgoto fail\n\t\t}\n", m[1])
	if b, err = guard(b, reParseClass, "_js_parse_class", reBrandPatch, brand); err != nil {
		return nil, err
	}

	return guard(b, reDestructElem, "_js_parse_destructuring_element", reTestRemoval, "\t\tif "+curFuncLost+" {\n\t\t\t"+parserOutOfMemory+"\t\t\treturn -int32(1)\n\t\t}\n")
}

// guard inserts check in the function that head starts, before the one
// place that at matches, unless it is there. at starts with a newline.
func guard(b []byte, head *regexp.Regexp, name string, at *regexp.Regexp, check string) ([]byte, error) {
	body, err := funcBody(b, head, name)
	if err != nil {
		return nil, err
	}
	fixed, err := insertAt(name, body, check, func(body []byte) (int, error) {
		loc := findAll(at, body)
		if len(loc) != 1 {
			return 0, fmt.Errorf("%s: want 1 place to check that the code is not lost, found %d", name, len(loc))
		}
		return loc[0][0] + 1, nil
	})
	if err != nil {
		return nil, err
	}
	return bytes.Replace(b, body, fixed, 1), nil
}

// guardAfterComment is guard for a place that at matches from the line of
// the comment before it: check goes after the comment.
func guardAfterComment(b []byte, head *regexp.Regexp, name string, at *regexp.Regexp, check string) ([]byte, error) {
	body, err := funcBody(b, head, name)
	if err != nil {
		return nil, err
	}
	fixed, err := insertAt(name, body, check, func(body []byte) (int, error) {
		loc := findAll(at, body)
		if len(loc) != 1 {
			return 0, fmt.Errorf("%s: want 1 place to check that the code is not lost, found %d", name, len(loc))
		}
		return bytes.IndexByte(body[loc[0][0]+1:], '\n') + loc[0][0] + 2, nil // after the comment
	})
	if err != nil {
		return nil, err
	}
	return bytes.Replace(b, body, fixed, 1), nil
}
