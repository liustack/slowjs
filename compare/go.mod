module modernc.org/quickjs/compare

go 1.26.5

replace modernc.org/quickjs => ../

require (
	github.com/dop251/goja v0.0.0-20260723142020-b4aef50fa347
	github.com/fastschema/qjs v0.0.6
	// Replaced by ../, so this version is not resolved and nothing reads it.
	// Keep it at or below the newest tag: modernc.org/gomod's updater, which
	// drives the fleet's autodeps, ignores replaces and fails on a require
	// ahead of the local clone.
	modernc.org/quickjs v0.22.0
)

require (
	github.com/dlclark/regexp2/v2 v2.5.2 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/google/pprof v0.0.0-20230207041349-798e818bf904 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/tetratelabs/wazero v1.9.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.3.8 // indirect
	modernc.org/libc v1.77.1 // indirect
	modernc.org/libquickjs v0.13.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)
