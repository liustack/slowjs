// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
}
