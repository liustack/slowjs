// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	gotime "time"
	"unsafe"

	"modernc.org/libc"
	"modernc.org/libc/sys/types"
)

var startTime = gotime.Now() // For clock(3)

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

// func _WaitForMultipleObjects(*libc.TLS, uint32, uintptr, int32, uint32) TDWORD {
// 	panic(todo(""))
// }

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

// Newer llvm-mingw (UCRT) headers emit the time64 spellings; forward them to
// libc so the windows cross-build stays toolchain-version independent.
func _clock_gettime64(tls *libc.TLS, clkID int32, tp uintptr) int32 {
	return libc.Xclock_gettime(tls, clkID, tp)
}

func _pthread_cond_timedwait64(tls *libc.TLS, cond, mutex, abstime uintptr) int32 {
	return libc.Xpthread_cond_timedwait(tls, cond, mutex, abstime)
}

// _GetProcessAffinityMask is used only by run-test262's _cpu_count to size its
// worker pool. libc does not export it, so report every logical CPU as
// available (DWORD_PTR is 64-bit on the windows/{amd64,arm64} targets this file
// serves).
func _GetProcessAffinityMask(tls *libc.TLS, proc, procMask, sysMask uintptr) int32 {
	mask := ^uint64(0)
	if n := runtime.NumCPU(); n < 64 {
		mask = uint64(1)<<uint(n) - 1
	}
	*(*uint64)(unsafe.Pointer(procMask)) = mask
	*(*uint64)(unsafe.Pointer(sysMask)) = mask
	return 1
}

const CLOCKS_PER_SEC = 1000

func _clock(*libc.TLS) int32 {
	return int32(gotime.Since(startTime) * gotime.Duration(CLOCKS_PER_SEC) / gotime.Second)
}

func _getcwd0(tls *libc.TLS, p uintptr, n types.Size_t) uintptr {
	return libc.Xgetcwd(tls, p, n)
}

// int ftw(const char *dirpath,
//
//	int (*fn)(const char *fpath, const struct stat *sb,
//	          int typeflag),
//	int nopenfd);
// func _ftw(tls *libc.TLS, dir, fn uintptr, n int32) int32 {
// 	panic(todo(""))
// }

// https://g.co/gemini/share/2c37d5b57994

// Constants mirroring C's ftw type flags
const (
	FTW_F   = 0 // Regular file
	FTW_D   = 1 // Directory (visited pre-order)
	FTW_DNR = 2 // Directory that cannot be read
	FTW_NS  = 4 // Stat failed (permissions, broken link, etc.)
	FTW_SL  = 4 // Symbolic link (lstat was used)
	// Note: C's ftw might have other flags like FTW_DP (post-order dir) or FTW_SLN
	// which are not directly supported by filepath.WalkDir's simple pre-order traversal.
	// This emulation focuses on the most common flags associated with stat/lstat results.
)

// ftwStopError is used internally to signal that the walk should stop
// because the user callback returned a non-zero value.
type ftwStopError struct {
	stopValue int
}

func (e *ftwStopError) Error() string {
	return fmt.Sprintf("ftw walk stopped by callback with return value %d", e.stopValue)
}

// goFtwFunc is the callback function type, mirroring the C ftw callback.
// It receives the path, file info (if available), and a type flag.
// Returning a non-zero value stops the walk and becomes the return value of Ftw.
// Returning 0 continues the walk.
type goFtwFunc func(path string, info os.FileInfo, typeflag int) int

// Ftw emulates the C standard library function ftw(3).
// It walks the directory tree starting at 'dirpath' and calls the 'callback'
// function for each entry encountered.
//
// Parameters:
//   - dirpath: The root directory path for the traversal.
//   - callback: The goFtwFunc to call for each file system entry.
//   - nopenfd: This parameter is part of the C ftw signature but is IGNORED
//     in this Go implementation. Go's filepath.WalkDir manages concurrency
//     and file descriptors internally.
//
// Returns:
//   - 0 on successful completion of the walk.
//   - The non-zero value returned by the callback, if the callback terminated the walk.
//   - -1 if an error occurred during the walk that wasn't handled by calling
//     the callback with FTW_DNR or FTW_NS (e.g., error accessing the initial dirpath).
func ftw(dirpath string, callback goFtwFunc, nopenfd int) int {
	// nopenfd is ignored in this Go implementation.

	walkErr := filepath.WalkDir(dirpath, func(path string, d fs.DirEntry, err error) error {
		var info os.FileInfo
		var typeflag int

		// --- Handle errors passed by WalkDir ---
		if err != nil {
			panic(todo(""))
			// // Check if the error is related to accessing a directory
			// if errors.Is(err, fs.ErrPermission) || errors.Is(err, unix.EACCES) { // Added syscall.EACCES check
			//         // Try to determine if it's a directory we can't read
			//         // We might not have 'd' if the error occurred trying to list 'path' contents
			//         // Let's try a direct Lstat on the path itself if d is nil
			//         lstatInfo, lstatErr := os.Lstat(path)
			//         if lstatErr == nil && lstatInfo.IsDir() {
			//                 typeflag = FTW_DNR // Directory, but WalkDir errored (likely reading it)
			//                 info = lstatInfo   // Provide the info we could get
			//         } else {
			//                 // Can't confirm it's a directory, or Lstat itself failed
			//                 typeflag = FTW_NS // Treat as general stat failure
			//                 // info remains nil
			//         }
			// } else {
			//         // Other errors (e.g., broken symlink during traversal, I/O error)
			//         typeflag = FTW_NS
			//         // Attempt to get Lstat info even if WalkDir had an error, maybe it's available
			//         lstatInfo, _ := os.Lstat(path) // Ignore error here, if it fails info stays nil
			//         info = lstatInfo
			// }
			// // Even with errors, call the callback with the path and appropriate flag
			// stopVal := callback(path, info, typeflag)
			// if stopVal != 0 {
			//         return &ftwStopError{stopValue: stopVal}
			// }
			// // If the error was on a directory, returning the error might stop WalkDir
			// // from descending. If it was fs.ErrPermission on a dir, WalkDir might
			// // pass filepath.SkipDir implicitly or continue depending on implementation.
			// // Let's return nil here to *try* to continue the walk for other siblings
			// // if the callback didn't stop it. The callback *was* notified.
			// // If the error prevents further progress WalkDir will stop anyway.
			// return nil // Allow walk to potentially continue elsewhere
		}

		// --- No error from WalkDir, process the DirEntry ---
		info, err = d.Info() // Get FileInfo (like C's stat/lstat result)
		if err != nil {
			// Error getting info for an entry WalkDir *could* list (rare, maybe permissions changed?)
			typeflag = FTW_NS
			// info remains nil
		} else {
			// Determine type flag based on file mode
			mode := info.Mode()
			if mode&fs.ModeSymlink != 0 {
				typeflag = FTW_SL
			} else if mode.IsDir() {
				typeflag = FTW_D // Visited pre-order
			} else if mode.IsRegular() {
				typeflag = FTW_F
			} else {
				// Other types (device, socket, pipe, etc.) - C ftw usually lumps these under FTW_F
				// or might have FTW_NS if stat fails. Let's treat non-dir, non-link, non-regular
				// as FTW_F for simplicity, aligning with common C practice, or FTW_NS if stat failed above.
				// Since we have info here, we know stat didn't fail.
				// Let's be more specific, maybe treat others as FTW_NS? Or stick to FTW_F?
				// C ftw man page isn't super specific about all types. FTW_F seems reasonable.
				typeflag = FTW_F // Treat other valid types as 'files' for simplicity
			}
		}

		// --- Call the user callback ---
		stopVal := callback(path, info, typeflag)
		if stopVal != 0 {
			// User wants to stop the walk
			return &ftwStopError{stopValue: stopVal}
		}

		return nil // Continue walk
	})

	// --- Handle WalkDir's final return value ---
	if walkErr == nil {
		return 0 // Success
	}

	// Check if the error was our custom stop signal
	var stopErr *ftwStopError
	if errors.As(walkErr, &stopErr) {
		return stopErr.stopValue // Return the value from the callback
	}

	// Otherwise, it was an unhandled error during the walk
	// (e.g., initial dirpath access error, or other error not mapped to FTW_NS/DNR)
	return -1 // General error return
}
func _ftw(tls *libc.TLS, path uintptr, fn uintptr, fd_limit int32) (r int32) {
	return int32(ftw(
		libc.GoString(path),
		func(path string, info os.FileInfo, typeflag int) int {
			cs, _ := libc.CString(filepath.ToSlash(path))

			defer libc.Xfree(tls, cs)

			return int((*(*func(*libc.TLS, uintptr, uintptr, int32) int32)(unsafe.Pointer(&struct{ uintptr }{fn})))(tls, cs, 0, int32(typeflag)))
		},
		int(fd_limit),
	))
}
