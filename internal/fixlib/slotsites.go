// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

// slotSites lists every function of the generated engine that uses a
// property slot (see slots.go) or looks up a property descriptor, with its
// uses and why they are safe for a pending stack. A pending stack is a
// marker object in the value of a slot, so a use that hands the value on
// must format it first. auditSlots fails on a function that is not listed or
// whose uses differ. After reviewing the change, update the list with the
// output of go run ./internal/fixlib -slots.
//
// The uses: "slot" is a pointer to a JSProperty, "cell" a stack cell that
// holds one (the out parameter of find_own_property). "read value" and
// "write value" access the value or a part of it, other reads and writes
// name the field (an accessor pair, a var_ref, an autoinit record).
// "guard" is a call of a lib/backtrace.go helper with the slot, and
// "early" marks a use that comes before the first guard of the function.
var slotSites = map[string]slotSite{
	"XJS_ComputeMemoryUsage": {
		"early slot compared=1 early slot read value=1 slot guard _js_backtrace_memory_usage=1",
		"counts the marker as an object, the record by _js_backtrace_memory_usage",
	},
	"XJS_DefineProperty": {
		"cell find=2 early slot address TJSProperty.Fu=2 early slot passed to _JS_AutoInitProperty=1 early slot passed to _remove_global_object_property=1 early slot read *struct{FgetteruintptrFsetteruintptr}.Fgetter=5 early slot read *struct{FgetteruintptrFsetteruintptr}.Fsetter=5 early slot read value=1 early slot write *struct{FgetteruintptrFsetteruintptr}.Fgetter=2 early slot write *struct{FgetteruintptrFsetteruintptr}.Fsetter=2 early slot write value=1 slot address TJSProperty.Fu=5 slot guard _js_backtrace_settle=1 slot read value=2 slot write value=2",
		"early uses are accessor slots, var_refs, autoinit and freeing the old value, settles before the SameValue read",
	},
	"XJS_GetOwnProperty": {
		"descriptor lookup=2",
		"descriptor through JS_GetOwnPropertyInternal, which settles",
	},
	"XJS_GetPropertyInternal": {
		"cell find=1 slot address TJSProperty.Fu=1 slot guard _js_backtrace_settle=1 slot passed to _JS_AutoInitProperty=1 slot read *struct{FgetteruintptrFsetteruintptr}.Fgetter=2 slot read value=1",
		"settles before the flag dispatch",
	},
	"XJS_HasProperty": {
		"descriptor lookup=1",
		"existence only",
	},
	"XJS_SetPropertyInternal": {
		"cell find=2 descriptor lookup=1 slot address TJSProperty.Fu=2 slot compared=1 slot passed to _JS_AutoInitProperty=2 slot passed to _set_value=1 slot read *struct{FgetteruintptrFsetteruintptr}.Fsetter=2 slot write value=1",
		"replaces the value with set_value, attributes through _js_backtrace_own_flags",
	},
	"_JS_AddBrand": {
		"cell find=2 slot compared=2 slot read value=1 slot write value=2",
		"private brands, never 'stack'",
	},
	"_JS_AutoInitProperty": {
		"slot address TJSProperty.Fu=4 slot passed to _js_autoinit_free=1 slot passed to _js_autoinit_get_id=1 slot passed to _js_autoinit_get_realm=1 slot read *struct{Frealm_and_idTuintptr_tFopaqueuintptr}.Fopaque=1 slot write value=2",
		"autoinit slots, never a pending stack",
	},
	"_JS_CallInternal": {
		"cell find=4 early slot address TJSProperty.Fu=1 early slot compared=1 slot guard _js_backtrace_object=3 slot passed to _set_value=1 slot read value=5 slot write value=1",
		"each inline read is guarded (see fixInterpreter), put_field replaces with set_value, the early use is a var_ref",
	},
	"_JS_CheckBrand": {
		"cell find=2 slot read value=1",
		"private brands, never 'stack'",
	},
	"_JS_CopyDataProperties": {
		"descriptor lookup=2",
		"attributes through _js_backtrace_own_flags, values read by JS_GetProperty",
	},
	"_JS_CreateProperty": {
		"cell find=1 slot address TJSProperty.Fu=2 slot compared=1 slot passed to _set_value=1 slot read value=1 slot write *struct{FgetteruintptrFsetteruintptr}.Fgetter=2 slot write *struct{FgetteruintptrFsetteruintptr}.Fsetter=2 slot write value=2",
		"a new slot it fills",
	},
	"_JS_DefineAutoInitProperty": {
		"cell find=1 slot compared=1 slot update *struct{Frealm_and_idTuintptr_tFopaqueuintptr}.Frealm_and_id=1 slot write *struct{Frealm_and_idTuintptr_tFopaqueuintptr}.Fopaque=1 slot write *struct{Frealm_and_idTuintptr_tFopaqueuintptr}.Frealm_and_id=1",
		"a new autoinit slot it fills",
	},
	"_JS_DefinePrivateField": {
		"cell find=1 slot compared=1 slot write value=1",
		"private fields, never 'stack'",
	},
	"_JS_DeleteGlobalVar": {
		"cell find=1",
		"global variables only",
	},
	"_JS_GetGlobalVarRef": {
		"cell find=1 slot address TJSProperty.Fu=1",
		"global variables only",
	},
	"_JS_GetOwnPropertyInternal": {
		"cell find=1 descriptor lookup=1 slot address TJSProperty.Fu=2 slot guard _js_backtrace_settle=1 slot passed to _JS_AutoInitProperty=1 slot read *struct{FgetteruintptrFsetteruintptr}.Fgetter=2 slot read *struct{FgetteruintptrFsetteruintptr}.Fsetter=2 slot read value=1",
		"settles before reading the value",
	},
	"_JS_GetOwnPropertyNames2": {
		"descriptor lookup=1",
		"enumerability through _js_backtrace_own_flags, values read by JS_GetProperty",
	},
	"_JS_GetOwnPropertyNamesInternal": {
		"descriptor lookup=1 slot read *uintptr=1",
		"var_ref of module namespaces, enumerability through _js_backtrace_own_flags",
	},
	"_JS_GetPrivateField": {
		"cell find=1 slot read value=1",
		"private fields, never 'stack'",
	},
	"_JS_NewObjectFromShape": {
		"slot compared=1 slot write *TJSProperty=1 slot write value=2",
		"new slots it fills",
	},
	"_JS_SetPrivateField": {
		"cell find=1 slot passed to _set_value=1",
		"private fields, never 'stack'",
	},
	"_JS_WriteArray": {
		"cell find=2 slot read value=2",
		"arrays and template objects only",
	},
	"_JS_WriteObjectTag": {
		"slot read value=1",
		"plain objects only, an Error is never written",
	},
	"_add_fast_array_element": {
		"slot read value=2 slot write value=1",
		"array length only",
	},
	"_add_property": {
		"slot passed to Xjs_realloc=1 slot returned=2",
		"makes a new slot",
	},
	"_compact_properties": {
		"slot passed to Xjs_realloc=1 slot read *TJSProperty=1 slot write *TJSProperty=1",
		"moves slots with their values, the marker stays an object",
	},
	"_convert_fast_array_to_array": {
		"slot address TJSProperty.Fu=1 slot write value=1",
		"new slots it fills",
	},
	"_delete_property": {
		"slot passed to _free_property=1 slot passed to _remove_global_object_property=1 slot write value=1",
		"frees the value, marker included",
	},
	"_find_property_regexp": {
		"cell find=1",
		"RegExp properties only",
	},
	"_free_object": {
		"slot passed to Xjs_free_rt=1 slot passed to _free_property=1",
		"frees the values, marker included",
	},
	"_free_property": {
		"slot address TJSProperty.Fu=1 slot passed to _js_autoinit_free=1 slot read *struct{FgetteruintptrFsetteruintptr}.Fgetter=2 slot read *struct{FgetteruintptrFsetteruintptr}.Fsetter=2 slot read value=1",
		"frees the value, marker included",
	},
	"_get_prop_string": {
		"cell find=2 slot guard _js_backtrace_prop_string=1 slot read value=1",
		"formats through _js_backtrace_prop_string before the read",
	},
	"_js_allocate_fast_array": {
		"slot passed to _set_value=1",
		"array length only",
	},
	"_js_array_push": {
		"slot read value=2 slot write value=1",
		"array length only",
	},
	"_js_autoinit_free": {
		"slot passed to _js_autoinit_get_realm=1",
		"autoinit slots, never a pending stack",
	},
	"_js_autoinit_get_id": {
		"slot read *struct{Frealm_and_idTuintptr_tFopaqueuintptr}.Frealm_and_id=1",
		"autoinit slots, never a pending stack",
	},
	"_js_autoinit_get_realm": {
		"slot read *struct{Frealm_and_idTuintptr_tFopaqueuintptr}.Frealm_and_id=1",
		"autoinit slots, never a pending stack",
	},
	"_js_autoinit_mark": {
		"slot passed to _js_autoinit_get_realm=1",
		"autoinit slots, never a pending stack",
	},
	"_js_build_module_ns": {
		"slot address TJSProperty.Fu=1 slot compared=1",
		"module namespaces only",
	},
	"_js_closure_define_global_var": {
		"cell find=3 slot address TJSProperty.Fu=6 slot compared=1 slot passed to _JS_AutoInitProperty=1 slot passed to _free_property=1",
		"global variables only",
	},
	"_js_closure_global_var": {
		"cell find=2 slot address TJSProperty.Fu=2 slot passed to _JS_AutoInitProperty=1",
		"global variables only",
	},
	"_js_create_array": {
		"slot passed to _set_value=1",
		"array length only",
	},
	"_js_create_array_free": {
		"slot passed to _set_value=1",
		"array length only",
	},
	"_js_for_in_next": {
		"descriptor lookup=2",
		"existence only",
	},
	"_js_function_bind": {
		"descriptor lookup=1",
		"existence of 'length' only",
	},
	"_js_global_object_find_uninitialized_var": {
		"cell find=1 slot address TJSProperty.Fu=1",
		"global variables only",
	},
	"_js_global_object_get_uninitialized_var": {
		"cell find=1 slot address TJSProperty.Fu=2 slot compared=1",
		"global variables only",
	},
	"_js_is_standard_regexp": {
		"cell find=1 cell passed to _find_property_regexp=1 slot read value=2",
		"RegExp properties only",
	},
	"_js_iterator_proto_set_toStringTag": {
		"descriptor lookup=1",
		"existence only",
	},
	"_js_object___lookupGetter__": {
		"descriptor lookup=1",
		"attributes and accessors through _js_backtrace_own_flags",
	},
	"_js_object_getOwnPropertyDescriptor": {
		"descriptor lookup=1",
		"descriptor through JS_GetOwnPropertyInternal, which settles",
	},
	"_js_object_hasOwn": {
		"descriptor lookup=1",
		"existence only",
	},
	"_js_object_hasOwnProperty": {
		"descriptor lookup=1",
		"existence only",
	},
	"_js_object_has_name": {
		"cell find=1 slot read value=1",
		"'name' only",
	},
	"_js_object_isSealed": {
		"descriptor lookup=1",
		"attributes through _js_backtrace_own_flags",
	},
	"_js_object_propertyIsEnumerable": {
		"descriptor lookup=1",
		"attributes through _js_backtrace_own_flags",
	},
	"_js_object_seal": {
		"descriptor lookup=1",
		"attributes through _js_backtrace_own_flags",
	},
	"_js_operator_private_in": {
		"cell find=1",
		"private brands, never 'stack'",
	},
	"_js_print_array_get_length": {
		"cell find=1 slot read value=1",
		"'length' only",
	},
	"_js_print_object": {
		"early slot address TJSProperty.Fu=2 early slot passed to _js_autoinit_get_id=1 early slot passed to _js_autoinit_get_realm=1 early slot read *struct{FgetteruintptrFsetteruintptr}.Fgetter=2 early slot read *struct{FgetteruintptrFsetteruintptr}.Fsetter=3 early slot read *struct{Frealm_and_idTuintptr_tFopaqueuintptr}.Fopaque=1 slot guard _js_backtrace_print_pending=1 slot read value=1",
		"early uses are accessor and autoinit slots, a placeholder through _js_backtrace_print_pending before the value",
	},
	"_js_proxy_define_own_property": {
		"descriptor lookup=1",
		"the check after the trap, through _js_backtrace_own_define",
	},
	"_js_proxy_delete_property": {
		"descriptor lookup=1",
		"attributes through _js_backtrace_own_flags",
	},
	"_js_proxy_get": {
		"descriptor lookup=1",
		"the check after the trap, through _js_backtrace_own_frozen",
	},
	"_js_proxy_get_own_property": {
		"descriptor lookup=2",
		"full lookup without a trap, attributes through _js_backtrace_own_flags after one",
	},
	"_js_proxy_get_own_property_mode": {
		"descriptor lookup=2",
		"made by fixProxy, keeps the mode without a trap",
	},
	"_js_proxy_get_own_property_names": {
		"descriptor lookup=1",
		"attributes through _js_backtrace_own_flags",
	},
	"_js_proxy_has": {
		"descriptor lookup=1",
		"attributes through _js_backtrace_own_flags",
	},
	"_js_proxy_set": {
		"descriptor lookup=1",
		"the check after the trap, through _js_backtrace_own_frozen",
	},
	"_js_regexp_get_lastIndex": {
		"slot read value=3",
		"RegExp lastIndex only",
	},
	"_js_regexp_set_lastIndex": {
		"slot passed to _set_value=1 slot read value=1",
		"RegExp lastIndex only",
	},
	"_mark_children": {
		"slot address TJSProperty.Fu=1 slot passed to _js_autoinit_mark=1 slot read *struct{FgetteruintptrFsetteruintptr}.Fgetter=2 slot read *struct{FgetteruintptrFsetteruintptr}.Fsetter=2 slot read value=1",
		"marks the value, marker included",
	},
	"_remove_global_object_property": {
		"slot address TJSProperty.Fu=2 slot compared=1",
		"global variables only",
	},
	"_resize_properties": {
		"slot passed to Xjs_realloc=1",
		"moves the prop array",
	},
	"_set_array_length": {
		"slot passed to _set_value=1 slot read value=1 slot write value=1",
		"array length only",
	},
	"_set_value": {
		"slot read value=1 slot write value=1",
		"replaces the value, freeing the old one, marker included",
	},
}
