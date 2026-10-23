// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"modernc.org/libc"
)

func __putenv(*libc.TLS, uintptr) {
	panic(todo(""))
}

func ___p__environ(*libc.TLS) uintptr {
	panic(todo(""))
}

func _tmpfile(*libc.TLS) uintptr {
	panic(todo(""))
}

func _feof(*libc.TLS, uintptr) int32 {
	panic(todo(""))
}

func _clearerr(*libc.TLS, uintptr) {
	panic(todo(""))
}

func __get_osfhandle(*libc.TLS, int32) uintptr {
	panic(todo(""))
}

func _signal(*libc.TLS, int32, uintptr) {
	panic(todo(""))
}

func _WaitForMultipleObjects(*libc.TLS, uint32, uintptr, int32, uint32) TDWORD {
	panic(todo(""))
}

func __utime64(*libc.TLS, uintptr, uintptr) int32 {
	panic(todo(""))
}

func __fullpath(*libc.TLS, uintptr, uintptr, uint64) int32 {
	panic(todo(""))
}

func _pthread_attr_setdetachstate(*libc.TLS, uintptr, int32) {
	panic(todo(""))
}

func _mkdir0(*libc.TLS, uintptr, uint16) int32 {
	panic(todo(""))
}

// Newer llvm-mingw (UCRT) headers resolve these to the time64 / non-suffixed
// spellings that older toolchains did not emit; forward them to libc so the
// windows cross-build stays toolchain-version independent.
func __utime(tls *libc.TLS, filename, times uintptr) int32 {
	return libc.Xutime(tls, filename, times)
}

func _clock_gettime64(tls *libc.TLS, clkID int32, tp uintptr) int32 {
	return libc.Xclock_gettime(tls, clkID, tp)
}

func _pthread_cond_timedwait64(tls *libc.TLS, cond, mutex, abstime uintptr) int32 {
	return libc.Xpthread_cond_timedwait(tls, cond, mutex, abstime)
}
