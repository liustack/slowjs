module github.com/liustack/slowjs/compare

go 1.27.0

replace github.com/liustack/slowjs => ../

require (
	github.com/dop251/goja v0.0.0-20260723142020-b4aef50fa347
	github.com/fastschema/qjs v0.0.6
	// Replaced by ../, so this version is not resolved and nothing reads it.
	github.com/liustack/slowjs v0.0.0-00010101000000-000000000000
)

require (
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/tetratelabs/wazero v1.9.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.3.8 // indirect
)
