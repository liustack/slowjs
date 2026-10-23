// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// Every generated file has no alloca left and a marker in each of the four
// functions that had one, and fixAlloca leaves a fixed file as it is.
func TestAllocaEveryFile(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "..", "lib", "ccgo_*.go"))
	if err != nil || len(files) == 0 {
		t.Fatal(files, err)
	}
	for _, fn := range files {
		b, err := readEngine(fn)
		if err != nil {
			t.Fatal(err)
		}
		out, err := fixAlloca(b)
		if err != nil {
			t.Fatalf("%s: %v", fn, err)
		}
		if !bytes.Equal(out, b) {
			t.Fatalf("%s: fixAlloca changed a fixed file", fn)
		}
		if bytes.Contains(b, []byte("X__builtin_alloca")) || reAllocaRegister.Match(b) {
			t.Errorf("%s: alloca left", fn)
		}
		framed := 0
		for _, name := range allocaFuncs {
			body, err := funcBody(b, funcHead(name), name)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(body, []byte(allocaInFrame)) {
				framed++
			}
		}
		const want = 3
		if framed != want {
			t.Errorf("%s: %d alloca in a frame, want %d", fn, framed, want)
		}
	}
}

// The shapes fixAlloca takes: a frame with a parameter copy, a pointer
// into the frame and the darwin registration, the linux registration with
// a conditional alloca, and a function without a frame.
const (
	allocaFramed = `
func _JS_CallInternal(tls *libc.TLS, ctx uintptr, _func_obj TJSValue, argc int32, flags int32) (r TJSValue) {
	bp := tls.Alloc(64)
	defer tls.Free(64)
	*(*TJSValue)(unsafe.Pointer(bp + 16)) = _func_obj
	var local_buf, s, sf uintptr
	var size Tsize_t
	_, _, _, _ = local_buf, s, sf, size
	defer tls.FreeAlloca()()
	sf = bp + 32
	if flags != 0 {
		s = *(*uintptr)(unsafe.Pointer(bp + 16))
		sf = s + 8
		(*TStackFrame)(unsafe.Pointer(sf)).Fx = 1
		goto done
	}
	if int32((**(**TJSValue)(__ccgo_up(bp + 16))).Ftag) != 0 {
		goto not_a_function
	}
	goto _2
not_a_function:
	;
	return _throw(tls, ctx)
_2:
	;
	size = uint64(16) * libc.Uint64FromInt32(argc)
	if _js_check_stack_overflow(tls, 0, size) != 0 {
		return r
	}
	(*TStackFrame)(unsafe.Pointer(sf)).Fx = 2
	local_buf = libc.X__builtin_alloca(tls, size)
	*(*TJSValue)(unsafe.Pointer(local_buf)) = **(**TJSValue)(__ccgo_up(bp + 16))
done:
	return r
}
`
	allocaFramedFixed = `
func _JS_CallInternal(tls *libc.TLS, ctx uintptr, _func_obj TJSValue, argc int32, flags int32) (r TJSValue) {
	frame_size := 64
	var frame_malloc uintptr
	bp := tls.Alloc(frame_size)
	defer func() {
		if frame_malloc != 0 {
			libc.Xfree(tls, frame_malloc)
		}
		tls.Free(frame_size)
	}()
	*(*TJSValue)(unsafe.Pointer(bp + 16)) = _func_obj
	var local_buf, s, sf uintptr
	var size Tsize_t
	_, _, _, _ = local_buf, s, sf, size
	sf = bp + 32
	if flags != 0 {
		s = *(*uintptr)(unsafe.Pointer(bp + 16))
		sf = s + 8
		(*TStackFrame)(unsafe.Pointer(sf)).Fx = 1
		goto done
	}
	if int32((**(**TJSValue)(__ccgo_up(bp + 16))).Ftag) != 0 {
		goto not_a_function
	}
	goto _2
not_a_function:
	;
	return _throw(tls, ctx)
_2:
	;
	size = uint64(16) * libc.Uint64FromInt32(argc)
	` + allocaInFrame + `	if size <= 768 {
		tls.Free(frame_size)
		frame_size = 64 + int(size)
		bp = tls.Alloc(frame_size)
		*(*TJSValue)(unsafe.Pointer(bp + 16)) = _func_obj
		sf = bp + 32
	}
	if _js_check_stack_overflow(tls, 0, size) != 0 {
		return r
	}
	(*TStackFrame)(unsafe.Pointer(sf)).Fx = 2
	if size <= 768 {
		local_buf = bp + uintptr(64)
	} else {
		local_buf = libc.Xmalloc(tls, size)
		frame_malloc = local_buf
	}
	*(*TJSValue)(unsafe.Pointer(local_buf)) = **(**TJSValue)(__ccgo_up(bp + 16))
done:
	return r
}
`
	allocaCond = `
func _js_call_c_function(tls *libc.TLS, ctx uintptr, func_obj TJSValue, argc int32, argv uintptr) (r TJSValue) {
	bp := tls.Alloc(40)
	defer tls.Free(40)
	var arg_buf, sf uintptr
	var arg_count int32
	_, _, _ = arg_buf, arg_count, sf
	tls.AllocaEntry()
	defer tls.AllocaExit()
	sf = bp + 8
	arg_count = _length(tls, func_obj)
	if _js_check_stack_overflow(tls, 0, uint32(8)*libc.Uint32FromInt32(arg_count)) != 0 {
		return r
	}
	arg_buf = argv
	if libc.BoolInt32(!!(argc < arg_count)) != 0 {
		/* ensure that at least argc_count arguments are readable */
		arg_buf = libc.X__builtin_alloca(tls, uint32(8)*libc.Uint32FromInt32(arg_count))
	}
	(*TStackFrame)(unsafe.Pointer(sf)).Farg_buf = arg_buf
	return r
}
`
	allocaCondFixed = `
func _js_call_c_function(tls *libc.TLS, ctx uintptr, func_obj TJSValue, argc int32, argv uintptr) (r TJSValue) {
	frame_size := 40
	var frame_malloc uintptr
	bp := tls.Alloc(frame_size)
	defer func() {
		if frame_malloc != 0 {
			libc.Xfree(tls, frame_malloc)
		}
		tls.Free(frame_size)
	}()
	var arg_buf, sf uintptr
	var arg_count int32
	_, _, _ = arg_buf, arg_count, sf
	sf = bp + 8
	arg_count = _length(tls, func_obj)
	` + allocaInFrame + `	if libc.BoolInt32(!!(argc < arg_count)) != 0 && uint32(8)*libc.Uint32FromInt32(arg_count) <= 768 {
		tls.Free(frame_size)
		frame_size = 40 + int(uint32(8)*libc.Uint32FromInt32(arg_count))
		bp = tls.Alloc(frame_size)
		sf = bp + 8
	}
	if _js_check_stack_overflow(tls, 0, uint32(8)*libc.Uint32FromInt32(arg_count)) != 0 {
		return r
	}
	arg_buf = argv
	if libc.BoolInt32(!!(argc < arg_count)) != 0 {
		/* ensure that at least argc_count arguments are readable */
		if uint32(8)*libc.Uint32FromInt32(arg_count) <= 768 {
			arg_buf = bp + uintptr(40)
		} else {
			arg_buf = libc.Xmalloc(tls, uint32(8)*libc.Uint32FromInt32(arg_count))
			frame_malloc = arg_buf
		}
	}
	(*TStackFrame)(unsafe.Pointer(sf)).Farg_buf = arg_buf
	return r
}
`
	allocaBound = `
func _js_call_bound_function(tls *libc.TLS, ctx uintptr, func_obj TJSValue, argc int32) (r TJSValue) {
	var arg_buf uintptr
	_ = arg_buf
	tls.AllocaEntry()
	defer tls.AllocaExit()
	if _js_check_stack_overflow(tls, 0, uint32(8)*libc.Uint32FromInt32(argc)) != 0 {
		return r
	}
	arg_buf = libc.X__builtin_alloca(tls, uint32(8)*libc.Uint32FromInt32(argc))
	return _call(tls, ctx, func_obj, argc, arg_buf)
}
`
	allocaBoundFixed = `
func _js_call_bound_function(tls *libc.TLS, ctx uintptr, func_obj TJSValue, argc int32) (r TJSValue) {
	var arg_buf uintptr
	_ = arg_buf
	if _js_check_stack_overflow(tls, 0, uint32(8)*libc.Uint32FromInt32(argc)) != 0 {
		return r
	}
	` + allocaInMalloc + `	arg_buf = libc.Xmalloc(tls, uint32(8)*libc.Uint32FromInt32(argc))
	defer libc.Xfree(tls, arg_buf)
	return _call(tls, ctx, func_obj, argc, arg_buf)
}
`
	allocaData = `
func _js_c_function_data_call(tls *libc.TLS, ctx uintptr, func_obj TJSValue, argc int32, argv uintptr) (r TJSValue) {
	var arg_buf, s uintptr
	_, _ = arg_buf, s
	defer tls.FreeAlloca()()
	s = _opaque(tls, func_obj)
	if libc.BoolInt64(!!(argc < libc.Int32FromUint8((*TRecord)(unsafe.Pointer(s)).Flength))) != 0 {
		arg_buf = libc.X__builtin_alloca(tls, uint64(16)*uint64((*TRecord)(unsafe.Pointer(s)).Flength))
	} else {
		arg_buf = argv
	}
	return _call(tls, ctx, func_obj, argc, arg_buf)
}
`
	allocaDataFixed = `
func _js_c_function_data_call(tls *libc.TLS, ctx uintptr, func_obj TJSValue, argc int32, argv uintptr) (r TJSValue) {
	var arg_buf, s uintptr
	_, _ = arg_buf, s
	s = _opaque(tls, func_obj)
	if libc.BoolInt64(!!(argc < libc.Int32FromUint8((*TRecord)(unsafe.Pointer(s)).Flength))) != 0 {
		` + allocaInMalloc + `		arg_buf = libc.Xmalloc(tls, uint64(16)*uint64((*TRecord)(unsafe.Pointer(s)).Flength))
		defer libc.Xfree(tls, arg_buf)
	} else {
		arg_buf = argv
	}
	return _call(tls, ctx, func_obj, argc, arg_buf)
}
`
)

const allocaFile = "package p\n" + allocaFramed + allocaCond + allocaBound + allocaData
const allocaFileFixed = "package p\n" + allocaFramedFixed + allocaCondFixed + allocaBoundFixed + allocaDataFixed

// fixAlloca takes each shape to its fixed form, and leaves that as it is.
func TestAllocaShapes(t *testing.T) {
	out, err := fixAlloca([]byte(allocaFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != allocaFileFixed {
		t.Fatalf("got\n%s\nwant\n%s", out, allocaFileFixed)
	}
	again, err := fixAlloca(out)
	if err != nil || !bytes.Equal(again, out) {
		t.Fatalf("a second run changed the file (%v)", err)
	}

	// A block that changes a pointer into the frame may also end in an if
	// and else that both leave, as JS_CallInternal's does.
	leave := "\t\tif argc == 0 {\n\t\t\tgoto done\n\t\t} else {\n\t\t\treturn r\n\t\t}\n"
	if _, err := fixAlloca([]byte(strings.Replace(allocaFile, "\t\tgoto done\n", leave, 1))); err != nil {
		t.Fatal(err)
	}
}

// What fixAlloca cannot prove keeps the slots and the frame as they were
// stops it.
func TestAllocaRejects(t *testing.T) {
	for _, c := range []struct {
		name, old, new, want string
	}{
		{"another alloca", "\targ_buf = argv\n\tif", "\targ_buf = libc.X__builtin_alloca(tls, 16)\n\tif", "5 calls of alloca"},
		{"no alloca", "\t\targ_buf = libc.X__builtin_alloca(tls, uint32(8)*libc.Uint32FromInt32(arg_count))\n", "", "3 calls of alloca"},
		{"no registration", "\tdefer tls.FreeAlloca()()\n\tsf = bp + 32\n", "\tsf = bp + 32\n", "0 registrations"},
		{"a write before the check", "\tsize = uint64(16)", "\t*(*int32)(unsafe.Pointer(bp + 48)) = 1\n\tsize = uint64(16)", "bp+48 used before the stack check"},
		{"a parameter copy written", "\tsize = uint64(16)", "\t*(*uintptr)(unsafe.Pointer(bp + 16)) = 1\n\tsize = uint64(16)", "bp+16 written before the stack check"},
		{"the frame passed on", "\treturn _throw(tls, ctx)\n", "\treturn _throw(tls, bp)\n", "the frame used before the stack check"},
		{"a pointer into the frame used", "\tsize = uint64(16)", "\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 3\n\tsize = uint64(16)", "sf, a pointer into the frame, used before"},
		{"a pointer into the frame changed", "\tsize = uint64(16)", "\tsf = s\n\tsize = uint64(16)", "sf, a pointer into the frame, changes"},
		{"the size changed", "\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 2\n", "\tsize++\n", "size changes between"},
		{"the condition changed", "\targ_buf = argv\n\tif", "\targ_count = 1\n\tif", "arg_count changes between"},
		{"a label after the check", "\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 2\n", "_9:\n\t;\n", "a label between"},
		{"a goto back", "\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 2\n", "\tif argc == 0 {\n\t\tgoto _2\n\t}\n", "goes back before it"},
		{"a size read from memory", "libc.X__builtin_alloca(tls, size)", "libc.X__builtin_alloca(tls, *(*Tsize_t)(unsafe.Pointer(s)))", "reads more than variables"},
		{"no stack check", "\tif _js_check_stack_overflow(tls, 0, size) != 0 {\n\t\treturn r\n\t}\n", "", "no stack check before the alloca"},
		{"alloca in a loop", "\t\targ_buf = libc.X__builtin_alloca(tls, uint32(8)*libc.Uint32FromInt32(arg_count))\n", "\t\tfor {\n\t\t\targ_buf = libc.X__builtin_alloca(tls, uint32(8)*libc.Uint32FromInt32(arg_count))\n\t\t}\n", "alloca inside a *ast.ForStmt"},
		{"a nested label after the check", "\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 2\n", "\tif argc == 1 {\n\t_8:\n\t\t;\n\t}\n", "a label between"},
		{"a size from a call", "libc.X__builtin_alloca(tls, size)", "libc.X__builtin_alloca(tls, _len(tls, size))", "reads more than variables"},
		{"a size from a libc call", "libc.X__builtin_alloca(tls, size)", "libc.X__builtin_alloca(tls, libc.Xstrlen(tls, size))", "reads more than variables"},
		{"a parameter changed before the check", "\tsize = uint64(16)", "\t_func_obj = TJSValue{}\n\tsize = uint64(16)", "parameter _func_obj changes before the stack check"},
		{"a pointer changed in a block whose if goes on", "\t\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 1\n\t\tgoto done\n", "\t\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 1\n\t\tif argc == 0 {\n\t\t\tgoto done\n\t\t}\n", "sf, a pointer into the frame, changes in a block"},
		{"a pointer changed in a block that goes back", "\t\tgoto done\n\t}\n\tif int32(", "\t\tgoto _2\n\t}\n\tif int32(", "sf, a pointer into the frame, changes in a block"},
		{"a pointer changed in a block whose else goes back", "\t\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 1\n\t\tgoto done\n", "\t\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 1\n\t\tif argc == 0 {\n\t\t\tgoto done\n\t\t} else {\n\t\t\tgoto not_a_function\n\t\t}\n", "sf, a pointer into the frame, changes in a block"},
		{"a pointer changed in a block that goes on", "\t\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 1\n\t\tgoto done\n", "\t\t(*TStackFrame)(unsafe.Pointer(sf)).Fx = 1\n", "sf, a pointer into the frame, changes in a block that does not end in a return or a goto"},
		{"alloca in an else", "\t} else {\n\t\targ_buf = argv\n", "\t} else {\n\t\targ_buf = libc.X__builtin_alloca(tls, 16)\n", "5 calls of alloca"},
	} {
		if strings.Count(allocaFile, c.old) != 1 {
			t.Fatalf("%s: %q is not in the file once", c.name, c.old)
		}
		_, err := fixAlloca([]byte(strings.Replace(allocaFile, c.old, c.new, 1)))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}

	// Two in one function and none in another.
	b := strings.Replace(allocaFile, "\targ_buf = argv\n\tif", "\targ_buf = libc.X__builtin_alloca(tls, 16)\n\tif", 1)
	b = strings.Replace(b, "\t\targ_buf = libc.X__builtin_alloca(tls, uint64(16)*uint64((*TRecord)(unsafe.Pointer(s)).Flength))\n", "", 1)
	if _, err := fixAlloca([]byte(b)); err == nil || !strings.Contains(err.Error(), "more than one call of alloca") {
		t.Errorf("two in one function: got %v", err)
	}

	// A fixed file without one of the tests of the size.
	b = strings.Replace(allocaFileFixed, "if size <= 768 {\n\t\tlocal_buf", "if size < 769 {\n\t\tlocal_buf", 1)
	if _, err := fixAlloca([]byte(b)); err == nil || !strings.Contains(err.Error(), "1 tests of the size") {
		t.Errorf("a missing test of the size: got %v", err)
	}

	// A fixed file whose marker went missing.
	b = strings.Replace(allocaFileFixed, allocaInMalloc, "", 1)
	if _, err := fixAlloca([]byte(b)); err == nil || !strings.Contains(err.Error(), "0 markers") {
		t.Errorf("a missing marker: got %v", err)
	}
}
