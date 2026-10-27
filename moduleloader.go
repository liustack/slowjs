// Copyright 2024 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package slowjs // import "github.com/liustack/slowjs"

import (
	"fmt"
	"sync"

	lib "github.com/liustack/slowjs/lib"
	"modernc.org/libc"
)

var (
	customLoadersMu sync.Mutex
	customLoaders   = map[uintptr]moduleLoaderState{}
)

// StdAddHelpers adds the 'print' and 'console' global objects to 'm'.
func (m *VM) StdAddHelpers() error {
	tls := m.runtime.tls
	ctx := m.cContext
	lib.Xjs_std_add_helpers(tls, ctx, -1, 0)
	return nil
}

// SetDefaultModuleLoader will enable loading module using the default module
// loader.
func (m *VM) SetDefaultModuleLoader() {
	lib.XJS_SetModuleLoaderFunc2(m.runtime.tls, m.runtime.cRuntime, 0, fp(lib.Xjs_module_loader), 0, 0)
}

// ModuleLoaderFunc defines the signature for a custom module loader.
// It must return the module's source code (Javascript).
type ModuleLoaderFunc func(vm *VM, moduleName string) (string, error)

// ModuleNormalizeFunc defines the signature for a custom module normalizer.
// It receives the base name (the module making the import) and the requested name,
// and should return the normalized, absolute module name.
type ModuleNormalizeFunc func(vm *VM, moduleBaseName, moduleName string) (string, error)

type moduleLoaderState struct {
	vm        *VM
	loader    ModuleLoaderFunc
	normalize ModuleNormalizeFunc
}

// jsMallocString allocates a C string using the QuickJS allocator so that QuickJS
// can safely call js_free() on it. We use lib.Xjs_strdup to let the C side handle
// the 32-bit/64-bit memory tracking alignment natively.
func jsMallocString(tls *libc.TLS, ctx uintptr, s string) uintptr {
	temp, err := libc.CString(validUTF8(s))
	if err != nil {
		return 0
	}
	defer libc.Xfree(tls, temp)

	// Duplicate the string natively into the QuickJS memory pool
	return lib.Xjs_strdup(tls, ctx, temp)
}

// SetModuleLoader configures custom Go functions to load and normalize Javascript modules.
// Passing nil for 'loader' removes the custom loader, and so does Close; until then the
// engine keeps 'loader' and 'normalize', and whatever they close over, reachable. If
// 'normalize' is nil, the engine falls back to the default QuickJS module normalizer.
func (m *VM) SetModuleLoader(loader ModuleLoaderFunc, normalize ModuleNormalizeFunc) {
	customLoadersMu.Lock()
	defer customLoadersMu.Unlock()

	if loader == nil {
		delete(customLoaders, m.runtime.cRuntime)
		lib.XJS_SetModuleLoaderFunc2(m.runtime.tls, m.runtime.cRuntime, 0, 0, 0, 0)
		return
	}

	customLoaders[m.runtime.cRuntime] = moduleLoaderState{
		vm:        m,
		loader:    loader,
		normalize: normalize,
	}

	var normFp uintptr
	if normalize != nil {
		normFp = fp(customModuleNormalize)
	}

	lib.XJS_SetModuleLoaderFunc2(
		m.runtime.tls,
		m.runtime.cRuntime,
		normFp,
		fp(customModuleLoader),
		0,
		m.runtime.cRuntime, // opaque
	)
}

func customModuleNormalize(tls *libc.TLS, ctx uintptr, module_base_name, module_name uintptr, opaque uintptr) uintptr {
	customLoadersMu.Lock()
	state, ok := customLoaders[opaque]
	customLoadersMu.Unlock()

	if !ok || state.normalize == nil {
		return 0
	}

	baseName := ""
	if module_base_name != 0 {
		baseName = cGoString(tls, module_base_name)
	}
	name := ""
	if module_name != 0 {
		name = cGoString(tls, module_name)
	}

	var normName string
	var err error
	state.vm.inHost("module normalizer", func() { normName, err = state.normalize(state.vm, baseName, name) })
	if state.vm.uncatchablePending() {
		// See callGo.
		return 0
	}
	if err != nil {
		throwTypeError(tls, ctx, fmt.Sprintf("module normalization failed: %v", err))
		return 0
	}

	return jsMallocString(tls, ctx, normName)
}

func customModuleLoader(tls *libc.TLS, ctx uintptr, module_name uintptr, opaque uintptr, attrs lib.TJSValue) uintptr {
	customLoadersMu.Lock()
	state, ok := customLoaders[opaque]
	customLoadersMu.Unlock()

	if !ok || state.loader == nil {
		return 0
	}

	name := ""
	if module_name != 0 {
		name = cGoString(tls, module_name)
	}

	var source string
	var err error
	state.vm.inHost("module loader", func() { source, err = state.loader(state.vm, name) })
	if state.vm.uncatchablePending() {
		// See callGo.
		return 0
	}
	if err != nil {
		throwTypeError(tls, ctx, fmt.Sprintf("module load failed: %v", err))
		return 0
	}

	ps, err := libc.CString(source)
	if err != nil {
		throwInternalError(tls, ctx, "OOM")
		return 0
	}
	defer libc.Xfree(tls, ps)

	flags := int32(lib.MJS_EVAL_TYPE_MODULE | lib.MJS_EVAL_FLAG_COMPILE_ONLY)
	v := lib.XJS_Eval(tls, ctx, ps, libc.Tsize_t(len(source)), module_name, flags)

	if isException(v) {
		lib.XFreeValue(tls, ctx, v)
		return 0
	}

	ptr := jsvToPtr(v)
	lib.XFreeValue(tls, ctx, v)

	return ptr
}
