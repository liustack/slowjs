//go:build windows

package intl

import (
	"strings"
	"syscall"
	"unsafe"
)

var procGetLocaleInfoEx = kernel32.NewProc("GetLocaleInfoEx")

// hostLocaleID is uprv_getDefaultLocaleID on Windows: the user's locale
// name, "en-US", which ICU canonicalizes; "en_US" where there is none.
func hostLocaleID() string {
	const localeSName = 0x5c
	var buf [85]uint16
	n, _, _ := procGetLocaleInfoEx.Call(0, localeSName, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return "en_US"
	}
	return strings.ReplaceAll(syscall.UTF16ToString(buf[:]), "-", "_")
}
