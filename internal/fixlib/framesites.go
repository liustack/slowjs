// Copyright (c) 2026 The liustack/quickjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.

package main

// frameSites are the functions whose TLS frame fixFrames replaces with a
// Go array: the frame size and the number of cell uses in
// lib/ccgo_darwin_arm64.go, and what the cells are for. They are the
// functions with a frame that call TLS.Alloc most often in a server render
// of a React page (linux/amd64 profile, 2026-10-18). Each must qualify
// (see fixFrames).
var frameSites = map[string]frameSite{
	"_set_value":              {16, 2, "the old value, to read its pointer (JS_FreeValue)"},
	"_free_property":          {16, 6, "a getter, setter or value, to read its pointer (JS_FreeValueRT)"},
	"x__JS_FreeValueRT":       {32, 11, "the value parameter and an atom's value, to read their pointers"},
	"_js_operator_typeof":     {16, 4, "the operand, to read its pointer"},
	"XJS_MarkValue":           {16, 4, "the value parameter, to read its tag and pointer"},
	"_get_proto_obj":          {16, 3, "the prototype parameter, to read its tag and pointer"},
	"XJS_IsFunction":          {16, 3, "the value parameter, to read its tag and pointer"},
	"_free_var_ref":           {16, 2, "the detached value, to read its pointer (JS_FreeValueRT)"},
	"_JS_ToBoolFree":          {32, 25, "the value parameter and float conversions"},
	"_JS_IsHTMLDDA":           {16, 3, "the value parameter, to read its tag and pointer"},
	"_js_free_shape0":         {16, 2, "the prototype, to read its pointer (JS_FreeValueRT)"},
	"_JS_ConcatStringInPlace": {16, 3, "the value parameter, to read its pointer"},
	"XJS_GetOpaque":           {16, 3, "the object parameter, to read its tag and pointer"},
	"_map_normalize_key":      {16, 5, "the key parameter and a float conversion"},

	// Added from the counts of tls.Alloc per function in server renders
	// of tifa's example shop and of its test apps (coverage, 2026-10-20).
	"_js_same_value_zero":             {16, 4, "the two operands, to read their pointers"},
	"_close_var_ref":                  {16, 2, "the value of the variable, to read its pointer (JS_DupValue)"},
	"_free_token":                     {16, 8, "a token's value or atom value, to read its pointer (JS_FreeValue)"},
	"_js_bytecode_function_finalizer": {32, 6, "the object parameter and a freed value, to read their pointers"},
	"XJS_SetOpaque":                   {16, 3, "the object parameter, to read its tag and pointer"},
	"_js_for_in_iterator_finalizer":   {32, 4, "the object parameter and the freed object, to read their pointers"},
	"_js_bytecode_function_mark":      {16, 2, "the object parameter, to read its pointer"},
	"_js_array_iterator_finalizer":    {32, 4, "the object parameter and the freed object, to read their pointers"},
	"Xjs_dtoa_max_len":                {16, 4, "a float conversion"},
	"XJS_IsCFunction":                 {16, 3, "the value parameter, to read its tag and pointer"},
	"_js_free_desc":                   {16, 6, "a getter, setter or value, to read its pointer (JS_FreeValue)"},
	"_js_get_fast_array":              {16, 3, "the object parameter, to read its tag and pointer"},
	"_js_array_mark":                  {16, 2, "the object parameter, to read its pointer"},
	"XJS_IsConstructor":               {16, 3, "the value parameter, to read its tag and pointer"},
	"_free_arg_list":                  {16, 2, "an argument, to read its pointer (JS_FreeValue)"},
	"XJS_FreeContext":                 {16, 28, "the context's values, to read their pointers (JS_FreeValue)"},
	"_js_map_mark":                    {16, 2, "the object parameter, to read its pointer"},
	"XJS_SetConstructorBit":           {16, 3, "the function parameter, to read its tag and pointer"},
	"_js_mapped_arguments_finalizer":  {16, 2, "the object parameter, to read its pointer"},
	"_js_get_this":                    {16, 2, "this, to read its pointer (JS_DupValue)"},
	"_js_regexp_finalizer":            {32, 6, "the object parameter and its two strings, to read their pointers"},
	"_js_bound_function_mark":         {16, 2, "the object parameter, to read its pointer"},
}
