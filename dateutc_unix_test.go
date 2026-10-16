// Copyright 2026 The quickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build unix

package quickjs // import "github.com/liustack/quickjs"

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
	"unsafe"

	lib "github.com/liustack/quickjs/lib"
	"modernc.org/libc"
)

const dateUTCChild = "QUICKJS_TEST_DATE_UTC_CHILD"

// TestDateLocalTimeIsUTC runs itself again in a process whose TZ is
// Asia/Shanghai (UTC+8, no daylight saving). There javascript's local time
// is UTC, while Go's time.Local and the libc the engine shares with the rest
// of the process keep the process's zone.
func TestDateLocalTimeIsUTC(t *testing.T) {
	if os.Getenv(dateUTCChild) == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDateLocalTimeIsUTC$", "-test.count=1")
		cmd.Env = append(os.Environ(), dateUTCChild+"=1", "TZ=Asia/Shanghai")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}

	host := func() {
		t.Helper()
		if got := os.Getenv("TZ"); got != "Asia/Shanghai" {
			t.Fatalf("TZ is %q", got)
		}
		if _, off := time.Unix(0, 0).Zone(); off != 8*3600 {
			t.Fatalf("time.Local is %v, %d seconds east", time.Local, off)
		}

		tls := libc.NewTLS()
		defer tls.Close()
		name, err := libc.CString("TZ")
		if err != nil {
			t.Fatal(err)
		}
		defer libc.Xfree(tls, name)
		if got := libc.GoString(libc.Xgetenv(tls, name)); got != "Asia/Shanghai" {
			t.Fatalf("libc's TZ is %q", got)
		}
		p := libc.Xcalloc(tls, 1, libc.Tsize_t(unsafe.Sizeof(lib.Ttime_t(0))+unsafe.Sizeof(lib.Ttm{})))
		defer libc.Xfree(tls, p)
		tm := p + unsafe.Sizeof(lib.Ttime_t(0))
		libc.Xlocaltime_r(tls, p, tm)
		if off := (*lib.Ttm)(unsafe.Pointer(tm)).Ftm_gmtoff; off != 8*3600 {
			t.Fatalf("libc's localtime is %d seconds east", off)
		}
	}
	host()

	vm, err := NewVM()
	if err != nil {
		t.Fatal(err)
	}
	defer vm.Close()
	for _, test := range [][2]string{
		{`new Date(0).getHours()`, "0"},
		{`new Date(0).getTimezoneOffset()`, "0"},
		{`new Date(Date.UTC(2026, 6, 1)).getTimezoneOffset()`, "0"},
		{`new Date(0).toString()`, "Thu Jan 01 1970 00:00:00 GMT+0000"},
		{`new Date(0).toTimeString()`, "00:00:00 GMT+0000"},
		{`new Date(2026, 0, 1, 13, 5).getTime() === Date.UTC(2026, 0, 1, 13, 5)`, "true"},
		{`Date.parse('2026-01-01T13:05:00') === Date.UTC(2026, 0, 1, 13, 5)`, "true"},
		{`new Date(0).toLocaleString()`, "01/01/1970, 12:00:00 AM"},
		{`new Date(0).toLocaleTimeString()`, "12:00:00 AM"},
	} {
		v, err := vm.Eval(test[0], EvalGlobal)
		if got := fmt.Sprint(v); err != nil || got != test[1] {
			t.Errorf("%s: got %q, %v, want %q", test[0], got, err, test[1])
		}
	}
	host()
}
