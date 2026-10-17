// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"regexp"
)

var (
	reDbufClaim      = regexp.MustCompile(`\nfunc Xdbuf_claim\(tls \*libc\.TLS, (\w+) uintptr, len1 Tsize_t\) \(r int32\) \{\n`)
	reCreateFunction = regexp.MustCompile(`\nfunc _js_create_function\(tls \*libc\.TLS, (\w+) uintptr, (\w+) uintptr\) \(r TJSValue\) \{\n`)
	reResolveVars    = regexp.MustCompile(`\nfunc _resolve_variables\(tls \*libc\.TLS, \w+ uintptr, \w+ uintptr\) \(r int32\) \{\n`)
	reResolveLabels  = regexp.MustCompile(`\nfunc _resolve_labels\(tls \*libc\.TLS, (\w+) uintptr, (\w+) uintptr\) \(r int32\) \{\n`)
	reParseErrorV    = regexp.MustCompile(`\nfunc _js_parse_error_v\(tls \*libc\.TLS, (\w+) uintptr, [^\n]*\{\n`)
	reLreCompile     = regexp.MustCompile(`\nfunc Xlre_compile\(tls \*libc\.TLS, [^\n]*\{\n`)
	reScopeVarCall   = regexp.MustCompile(`\n(\t+)pos_next = _resolve_scope_var\(tls, [^\n]*\)\n(\t+XJS_FreeAtom\(tls, \w+, var_name\)\n)`)
	reWriteAtoms     = regexp.MustCompile(`\nfunc _JS_WriteObjectAtoms\(tls \*libc\.TLS, (\w+) uintptr\) \(r int32\) \{\n`)
	reWriteObject2   = regexp.MustCompile(`\nfunc XJS_WriteObject2\(tls \*libc\.TLS, (\w+) uintptr, [^\n]*\{\n`)
	reCodeMove       = regexp.MustCompile(`\n(\t+)if (?:\(\*TDynBuf\)\(unsafe\.Pointer\(bc\)\)\.Ferror1 != 0 \|\| )?Xdbuf_claim\(tls, bc, (?:libc\.Uint(?:32|64)FromInt32|uint64)\(chunk_size\)\) != 0 \{\n(?:\t+XJS_ThrowOutOfMemory\([^\n]*\n)?\t+(return -int32\(1\)|goto fail)\n`)
	reNextTokenFree  = regexp.MustCompile(`\n\t_free_token\(tls, s, s\+\d+\)\n`)
	reParseClass     = regexp.MustCompile(`\nfunc _js_parse_class\(tls \*libc\.TLS, s uintptr, [^\n]*\{\n`)
	reBrandPatch     = regexp.MustCompile(`\n\t\t\*\*\(\*\*Tuint8_t\)\(__ccgo_up\(\(\*TJSFunctionDef\)\(unsafe\.Pointer\(\(\*TClassFieldsDef\)\(unsafe\.Pointer\((\w+)\)\)\.Ffields_init_fd\)\)\.Fbyte_code\.Fbuf \+ uintptr\(\(\*TClassFieldsDef\)\(unsafe\.Pointer\(\w+\)\)\.Fbrand_push_pos\)\)\) = uint8\(EOP_push_true\)\n`)
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
	keep := fmt.Sprintf("\t\t\t(*TDynBuf)(unsafe.Pointer(%[1]s)).Fallocated_size = (*TDynBuf)(unsafe.Pointer(%[1]s)).Fsize\n", s)
	if !bytes.Contains(body, []byte(keep)) {
		re := regexp.MustCompile(`\n\t\tif !\(new_buf != 0\) \{\n\t\t\t\(\*TDynBuf\)\(unsafe\.Pointer\(` + s + `\)\)\.Ferror1 = int32\(\w+\)\n`)
		loc := re.FindAllIndex(body, -1)
		if len(loc) != 1 {
			return nil, fmt.Errorf("Xdbuf_claim: cannot find the failed realloc")
		}
		b = bytes.Replace(b, body, splice(body, loc[0][1], keep), 1)
	}

	// js_create_function.
	if body, err = funcBody(b, reCreateFunction, "_js_create_function"); err != nil {
		return nil, err
	}
	m := reCreateFunction.FindSubmatch(b)
	ctx, fd := string(m[1]), string(m[2])
	check := fmt.Sprintf("\tif (*TJSFunctionDef)(unsafe.Pointer(%s)).Fbyte_code.Ferror1 != 0 {\n\t\tXJS_ThrowOutOfMemory(tls, %s)\n\t\tgoto fail\n\t}\n", fd, ctx)
	if !bytes.Contains(body, []byte(check)) {
		anchor := []byte("\t/* recompute scope linkage */\n")
		if bytes.Count(body, anchor) != 1 || !bytes.Contains(body, []byte("\nfail:\n")) {
			return nil, fmt.Errorf("_js_create_function: cannot find the start or the fail label")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, anchor, append([]byte(check), anchor...), 1), 1)
	}

	// The line table js_create_function shrinks.
	if body, err = funcBody(b, reCreateFunction, "_js_create_function"); err != nil {
		return nil, err
	}
	shrink := fmt.Sprintf(".Fdebug.Fpc2line_buf = Xjs_realloc_rt(tls, (*TJSContext)(unsafe.Pointer(%s)).Frt, (*TJSFunctionDef)(unsafe.Pointer(%s)).Fpc2line.Fbuf, ", ctx, fd)
	if !bytes.Contains(body, []byte(shrink)) {
		old := fmt.Sprintf(".Fdebug.Fpc2line_buf = Xjs_realloc(tls, %s, (*TJSFunctionDef)(unsafe.Pointer(%s)).Fpc2line.Fbuf, ", ctx, fd)
		if bytes.Count(body, []byte(old)) != 1 {
			return nil, fmt.Errorf("_js_create_function: cannot find where it shrinks the line table")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, []byte(old), []byte(shrink), 1), 1)
	}

	// resolve_variables.
	if body, err = funcBody(b, reResolveVars, "_resolve_variables"); err != nil {
		return nil, err
	}
	if !bytes.Contains(body, []byte("if pos_next < 0 {")) {
		if n := len(reScopeVarCall.FindAllIndex(body, -1)); n != 2 || !bytes.Contains(body, []byte("\nfail:\n")) {
			return nil, fmt.Errorf("_resolve_variables: want 2 calls of _resolve_scope_var and a fail label, found %d calls", n)
		}
		fixed := reScopeVarCall.ReplaceAllFunc(body, func(call []byte) []byte {
			sm := reScopeVarCall.FindSubmatch(call)
			indent := string(sm[1])
			line := call[:len(call)-len(sm[2])] // "\n", the call and its newline
			return append(append([]byte{}, line...), indent+"if pos_next < 0 {\n"+indent+"\tgoto fail\n"+indent+"}\n"+string(sm[2])...)
		})
		b = bytes.Replace(b, body, fixed, 1)
	}

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
		loc := reCodeMove.FindAllSubmatchIndex(body, -1)
		if len(loc) != 1 {
			return nil, fmt.Errorf("%s: want 1 claim for moving code, found %d", name, len(loc))
		}
		fixed := body
		indent := string(body[loc[0][2]:loc[0][3]])
		if !bytes.Contains(body, []byte(throw)) {
			at := loc[0][4] - len(indent) - 1 // the start of the return or goto
			fixed = splice(fixed, at, indent+"\t"+throw)
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
	if !bytes.Contains(body, []byte(check)) {
		anchor := []byte(fmt.Sprintf("\tctx = (*TJSParseState)(unsafe.Pointer(%s)).Fctx\n", ps))
		if bytes.Count(body, anchor) != 1 || !bytes.Contains(body, []byte("\t_JS_ThrowError2(tls, ctx, ")) {
			return nil, fmt.Errorf("_js_parse_error_v: cannot find where it reads the context")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, anchor, append(anchor, check...), 1), 1)
	}

	// The bytecode writer.
	if b, err = fixWriteObject(b); err != nil {
		return nil, err
	}

	// lre_compile.
	if body, err = funcBody(b, reLreCompile, "Xlre_compile"); err != nil {
		return nil, err
	}
	if !bytes.Contains(body, []byte("\nerror:\n\t;\n\tif *(*byte)(unsafe.Pointer(")) {
		// The first buffer it frees is the code, the state's first field.
		re := regexp.MustCompile(`\nerror:\n\t;\n(\tXdbuf_free\(tls, (\w+)\)\n\tXdbuf_free\(tls, \w+\+\d+\)\n)\tXpstrcpy\(tls, error_msg, error_msg_size, (\w+\+\d+)\)\n`)
		loc := re.FindAllSubmatchIndex(body, -1)
		if len(loc) != 1 {
			return nil, fmt.Errorf("Xlre_compile: cannot find the error label")
		}
		state, msg := string(body[loc[0][4]:loc[0][5]]), string(body[loc[0][6]:loc[0][7]])
		add := fmt.Sprintf("\tif *(*byte)(unsafe.Pointer(%s)) == 0 || (*TDynBuf)(unsafe.Pointer(%s)).Ferror1 != 0 {\n\t\t_re_parse_out_of_memory(tls, %s)\n\t}\n", msg, state, state)
		b = bytes.Replace(b, body, splice(body, loc[0][2], add), 1)
	}
	return b, nil
}

// fixResolveLabels makes _resolve_labels stop at a failed write, keep the
// new code when it fails, and free the relocations of the labels it did not
// reach then.
func fixResolveLabels(b []byte) ([]byte, error) {
	body, err := funcBody(b, reResolveLabels, "_resolve_labels")
	if err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte("\nout_of_memory:\n")) {
		return fixResolveLabelsRelocs(b)
	}
	m := reResolveLabels.FindSubmatch(b)
	ctx := string(m[1])
	bc := regexp.MustCompile(`/\* bc_out at (bp\+\d+) \*/`).FindSubmatch(body)
	if bc == nil {
		return nil, fmt.Errorf("_resolve_labels: cannot find bc_out")
	}
	failed := fmt.Sprintf("if (*TDynBuf)(unsafe.Pointer(%s)).Ferror1 != 0 {\n", bc[1])

	// The new code, as the end sets it.
	set := regexp.MustCompile(`\t/\* set the new byte code \*/\n(\tXdbuf_free\(tls, [^\n]*\)\n\t\(\*TJSFunctionDef\)\(unsafe\.Pointer\(\w+\)\)\.Fbyte_code = [^\n]*\n\t\(\*TJSFunctionDef\)\(unsafe\.Pointer\(\w+\)\)\.Fuse_short_opcodes = int32\(\w+\)\n)`).FindSubmatch(body)
	if set == nil {
		return nil, fmt.Errorf("_resolve_labels: cannot find where it sets the new code")
	}

	head := []byte("\tpos = 0\n\tfor {\n\t\tif !(pos < bc_len) {\n\t\t\tbreak\n\t\t}\n")
	after := []byte("\t/* check that there were no missing labels */\n")
	fail := []byte(fmt.Sprintf("fail:\n\t;\n\t/* XXX: not safe */\n\tXdbuf_free(tls, %s)\n\treturn -int32(1)\n}\n", bc[1]))
	if bytes.Count(body, head) != 1 || bytes.Count(body, after) != 1 || !bytes.HasSuffix(body, fail) {
		return nil, fmt.Errorf("_resolve_labels: cannot find the loop over the code, its end or the fail label")
	}
	fixed := bytes.Replace(body, head, append(append([]byte{}, head...), "\t\t"+failed+"\t\t\tgoto out_of_memory\n\t\t}\n"...), 1)
	fixed = bytes.Replace(fixed, after, append([]byte("\t"+failed+"\t\tgoto out_of_memory\n\t}\n"), after...), 1)
	end := fmt.Sprintf("out_of_memory:\n\t;\n\tXJS_ThrowOutOfMemory(tls, %s)\nfail:\n\t;\n%s\treturn -int32(1)\n}\n", ctx, set[1])
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
	if bytes.Contains(body, []byte(marker)) {
		return b, nil
	}
	m := reResolveLabels.FindSubmatch(b)
	ctx, fd := string(m[1]), string(m[2])
	fail := []byte("\nfail:\n\t;\n")
	next := []byte("\t\t\tre_next = (*TRelocEntry)(unsafe.Pointer(re)).Fnext\n")
	slots := []byte(fmt.Sprintf("\tlabel_slots = (*TJSFunctionDef)(unsafe.Pointer(%s)).Flabel_slots\n", fd))
	if bytes.Count(body, fail) != 1 || !bytes.Contains(body, next) || bytes.Count(body, slots) != 1 {
		return nil, fmt.Errorf("_resolve_labels: cannot find the fail label, label_slots or how it frees a relocation")
	}
	free := marker + fmt.Sprintf("\ti = 0\n\tfor ; i < (*TJSFunctionDef)(unsafe.Pointer(%[2]s)).Flabel_count; i++ {\n"+
		"\t\tls = label_slots + uintptr(i)*uintptr(unsafe.Sizeof(TLabelSlot{}))\n"+
		"\t\tfor re = (*TLabelSlot)(unsafe.Pointer(ls)).Ffirst_reloc; re != 0; re = re_next {\n"+
		"\t\t\tre_next = (*TRelocEntry)(unsafe.Pointer(re)).Fnext\n"+
		"\t\t\tXjs_free(tls, %[1]s, re)\n\t\t}\n"+
		"\t\t(*TLabelSlot)(unsafe.Pointer(ls)).Ffirst_reloc = 0\n\t}\n", ctx, fd)
	return bytes.Replace(b, body, bytes.Replace(body, fail, append(append([]byte{}, fail...), free...), 1), 1), nil
}

// splice inserts s into b at i.
func splice(b []byte, i int, s string) []byte {
	return append(append(append([]byte{}, b[:i]...), s...), b[i:]...)
}

// fixWriteObject makes the bytecode writer check its buffer.
func fixWriteObject(b []byte) ([]byte, error) {
	body, err := funcBody(b, reWriteAtoms, "_JS_WriteObjectAtoms")
	if err != nil {
		return nil, err
	}
	s := string(reWriteAtoms.FindSubmatch(b)[1])
	throw := fmt.Sprintf("\t\tXJS_ThrowOutOfMemory(tls, (*TBCWriterState)(unsafe.Pointer(%s)).Fctx)\n", s)
	if !bytes.Contains(body, []byte(throw)) {
		init := regexp.MustCompile(`\n\t_js_dbuf_init\(tls, \(\*TBCWriterState\)\(unsafe\.Pointer\(` + s + `\)\)\.Fctx, (` + s + `\+\d+)\)\n`).FindSubmatch(body)
		claim := regexp.MustCompile(`\n\tif (Xdbuf_claim\(tls, bp, (?:libc\.Uint(?:32|64)FromInt32|uint64)\(atoms_size\)\) != 0) \{\n\t\tgoto fail\n`)
		loc := claim.FindAllSubmatchIndex(body, -1)
		if init == nil || len(loc) != 1 {
			return nil, fmt.Errorf("_JS_WriteObjectAtoms: cannot find the atoms buffer or the claim that moves the atoms")
		}
		fixed := splice(body, loc[0][1]-len("\t\tgoto fail\n"), throw)
		fixed = splice(fixed, loc[0][2], fmt.Sprintf("(*TDynBuf)(unsafe.Pointer(%s)).Ferror1 != 0 || ", init[1]))
		b = bytes.Replace(b, body, fixed, 1)
	}

	re := regexp.MustCompile(`\nfunc _bc_put_atom\(tls \*libc\.TLS, s uintptr, atom TJSAtom\) \(r int32\) \{\n`)
	if body, err = funcBody(b, re, "_bc_put_atom"); err != nil {
		return nil, err
	}
	mark := "\t\t\t(*TBCWriterState)(unsafe.Pointer(s)).Fdbuf.Ferror1 = int32(MTRUE)\n"
	if !bytes.Contains(body, []byte(mark)) {
		failed := []byte("\t\tif _bc_atom_to_idx(tls, s, bp, atom) != 0 {\n")
		if bytes.Count(body, failed) != 1 {
			return nil, fmt.Errorf("_bc_put_atom: cannot find where adding the atom fails")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, failed, append(append([]byte{}, failed...), mark...), 1), 1)
	}

	if body, err = funcBody(b, reWriteObject2, "XJS_WriteObject2"); err != nil {
		return nil, err
	}
	ctx := string(reWriteObject2.FindSubmatch(b)[1])
	init := regexp.MustCompile(`\n\t_js_dbuf_init\(tls, ` + ctx + `, (s\+\d+)\)\n`).FindSubmatch(body)
	if init == nil {
		return nil, fmt.Errorf("XJS_WriteObject2: cannot find the buffer")
	}
	check := fmt.Sprintf("\tif (*TDynBuf)(unsafe.Pointer(%s)).Ferror1 != 0 {\n\t\tXJS_ThrowOutOfMemory(tls, %s)\n\t\tgoto fail\n\t}\n", init[1], ctx)
	if !bytes.Contains(body, []byte(check)) {
		rec := []byte("\tif _JS_WriteObjectRec(tls, s, obj) != 0 {\n\t\tgoto fail\n\t}\n")
		if bytes.Count(body, rec) != 1 {
			return nil, fmt.Errorf("XJS_WriteObject2: cannot find the call of _JS_WriteObjectRec")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, rec, append(append([]byte{}, rec...), check...), 1), 1)
	}
	return b, nil
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
	marked := []byte("\t\t\t\tif !(new_buf != 0) {\n\t\t\t\t\t" + lost + " = int32(MTRUE)\n\t\t\t\t\treturn -int32(1)\n")
	fixed := body
	if bytes.Count(fixed, marked) != 2 {
		if bytes.Count(fixed, failed) != 2 {
			return nil, fmt.Errorf("_push_scope: want 2 failed reallocs")
		}
		fixed = bytes.ReplaceAll(fixed, failed, marked)
	}
	if !bytes.Contains(fixed, []byte(check)) {
		scope := []byte("\t\tscope = (*TJSFunctionDef)(unsafe.Pointer(fd)).Fscope_count\n")
		if bytes.Count(fixed, scope) != 1 {
			return nil, fmt.Errorf("_push_scope: cannot find where it reads the scope count")
		}
		fixed = bytes.Replace(fixed, scope, append(append([]byte{}, scope...), check...), 1)
	}
	b = bytes.Replace(b, body, fixed, 1)

	re = regexp.MustCompile(`\nfunc _pop_scope\(tls \*libc\.TLS, s uintptr\) \{\n`)
	if body, err = funcBody(b, re, "_pop_scope"); err != nil {
		return nil, err
	}
	check = "\t\tif " + lost + " != 0 {\n\t\t\treturn\n\t\t}\n"
	if !bytes.Contains(body, []byte(check)) {
		level := []byte("\t\tscope = (*TJSFunctionDef)(unsafe.Pointer(fd)).Fscope_level\n")
		if bytes.Count(body, level) != 1 {
			return nil, fmt.Errorf("_pop_scope: cannot find where it reads the scope level")
		}
		b = bytes.Replace(b, body, bytes.Replace(body, level, append(append([]byte{}, level...), check...), 1), 1)
	}
	return b, nil
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
	if bytes.Contains(body, []byte(check)) {
		return b, nil
	}
	loc := at.FindAllIndex(body, -1)
	if len(loc) != 1 {
		return nil, fmt.Errorf("%s: want 1 place to check that the code is not lost, found %d", name, len(loc))
	}
	return bytes.Replace(b, body, splice(body, loc[0][0]+1, check), 1), nil
}

// guardAfterComment is guard for a place that at matches from the line of
// the comment before it: check goes after the comment.
func guardAfterComment(b []byte, head *regexp.Regexp, name string, at *regexp.Regexp, check string) ([]byte, error) {
	body, err := funcBody(b, head, name)
	if err != nil {
		return nil, err
	}
	if bytes.Contains(body, []byte(check)) {
		return b, nil
	}
	loc := at.FindAllIndex(body, -1)
	if len(loc) != 1 {
		return nil, fmt.Errorf("%s: want 1 place to check that the code is not lost, found %d", name, len(loc))
	}
	line := bytes.IndexByte(body[loc[0][0]+1:], '\n') + loc[0][0] + 2 // after the comment
	return bytes.Replace(b, body, splice(body, line, check), 1), nil
}
