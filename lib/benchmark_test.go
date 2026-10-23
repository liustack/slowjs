// Copyright 2025 The libquickjs-go Authors. All rights reserved.
// Use of the source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package libquickjs // import "github.com/liustack/slowjs/lib"

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"modernc.org/libc"
)

// 2025-05-12 C						   Go
//
//                                                         goos: linux
//                                                         goarch: amd64
//                                                         pkg: modernc.org/libquickjs
//                                                         cpu: Intel(R) Xeon(R) CPU E5-1650 v2 @ 3.50GHz
//                                                         BenchmarkMicro
//	                  TEST          N TIME (ns)                          TEST          N TIME (ns)
//	            empty_loop     200000     19.77                    empty_loop      50000     82.43
//	       empty_down_loop     100000     25.22               empty_down_loop      20000    112.82
//	      empty_down_loop2     100000     27.51              empty_down_loop2      20000    141.99
//	         empty_do_loop     100000     24.64                 empty_do_loop      20000    108.44
//	              date_now       5000    902.15                      date_now       2000   1069.28
//	            date_parse        500   1220.14                    date_parse        100   4520.99
//	             prop_read      50000     22.82                     prop_read       5000    111.92
//	            prop_write      50000     19.94                    prop_write       5000    103.19
//	           prop_update      20000     29.57                   prop_update       5000    184.86
//	           prop_create       2000     86.18                   prop_create        500    270.46
//	            prop_clone       1000    108.88                    prop_clone        500    395.47
//	           prop_delete       1000    170.97                   prop_delete        200    617.05
//	            array_read      10000     23.02                    array_read       5000     79.65
//	           array_write      10000     21.08                   array_write       5000     60.57
//	     array_prop_create        100     27.36             array_prop_create         20    124.39
//	           array_slice        100     28.36                   array_slice         20    183.53
//	     array_length_decr         50     68.87             array_length_decr          5    417.92
//	array_hole_length_decr         50     94.98        array_hole_length_decr          5    464.69
//	            array_push         50    104.32                    array_push         10    595.57
//	             array_pop         50    115.34                     array_pop         10    723.05
//	      typed_array_read      10000     21.57              typed_array_read       5000     77.96
//	     typed_array_write      10000     22.23             typed_array_write       5000     61.28
//	           global_read      50000     20.11                   global_read       5000    115.40
//	          global_write      50000     24.10                  global_write       5000    122.20
//	   global_write_strict      50000     24.29           global_write_strict       5000    122.00
//	        local_destruct       1000    294.73                local_destruct        200   1194.36
//	       global_destruct       2000    141.37               global_destruct        500    813.64
//	global_destruct_strict       2000    141.24        global_destruct_strict        500    813.83
//	      global_func_call      20000     45.74              global_func_call       2000    285.54
//	             func_call      20000     38.37                     func_call       5000    222.60
//	     func_closure_call      20000     38.94             func_closure_call       5000    219.62
//	             int_arith        200     18.73                     int_arith         50     89.66
//	           float_arith        100     27.89                   float_arith         20    107.60
//	        map_set_string         10    393.73                map_set_string          2   1890.97
//	           map_set_int         10    203.35                   map_set_int          5    892.55
//	        map_set_bigint         10    261.84                map_set_bigint          2   1502.86
//	            map_delete          5    414.65                    map_delete          1   2024.65
//	          weak_map_set         20    162.75                  weak_map_set          1   2042.25
//	       weak_map_delete          5    448.91               weak_map_delete          1   2193.88
//	             array_for       1000     23.52                     array_for        200    107.32
//	          array_for_in        200    104.38                  array_for_in         50    487.78
//	          array_for_of       1000     37.41                  array_for_of        100    237.09
//	              math_min         50     60.36                      math_min         10    353.19
//	          regexp_ascii          2   1042.39                  regexp_ascii          1   3550.83
//	          regexp_utf16          2   1072.32                  regexp_utf16          1   3606.71
//	         string_build1        100     34.19                 string_build1         20    128.84
//	        string_build1x        100     34.16                string_build1x         20    128.76
//	        string_build2c         50     41.94                string_build2c         20    127.95
//	         string_build2         50     71.66                 string_build2         10    317.67
//	         string_build3         50     86.03                 string_build3         10    370.35
//	         string_build4         50     81.81                 string_build4         10    368.26
//	   string_build_large1          1    112.64           string_build_large1          1    390.19
//	   string_build_large2          1    109.75           string_build_large2          1    434.89
//	         int_to_string      10000     85.69                 int_to_string       5000    320.44
//	          int_toString      10000    100.82                  int_toString       2000    476.21
//	       float_to_string       2000    477.14               float_to_string       1000    987.52
//	        float_toString       2000    493.05                float_toString       1000   1234.06
//	         float_toFixed       5000    235.89                 float_toFixed       1000    874.72
//	     float_toPrecision       5000    251.85             float_toPrecision       1000    946.73
//	   float_toExponential       5000    249.45           float_toExponential       1000    950.92
//	         string_to_int      20000    153.47                 string_to_int       5000    429.61
//	       string_to_float      10000    206.96               string_to_float       5000    516.89
//	        bigint32_arith        100     39.07                bigint32_arith         10    287.02
//	        bigint64_arith         50     65.93                bigint64_arith          5    445.02
//	       bigint256_arith         10    202.26               bigint256_arith          5    712.35
//	            sort_bench          1     24.21                    sort_bench          1     81.02
//	                 total             11384.01                         total             44033.46		x 3.868

// 2025-05-12 C						   Go
//
//                                                         goos: linux
//                                                         goarch: amd64
//                                                         pkg: modernc.org/libquickjs
//                                                         cpu: AMD Ryzen 9 3900X 12-Core Processor
//                                                         BenchmarkMicro
//	                  TEST          N TIME (ns)                          TEST          N TIME (ns)
//	            empty_loop     200000     13.28                    empty_loop      50000     67.46
//	       empty_down_loop     200000     18.61               empty_down_loop      20000    107.72
//	      empty_down_loop2     100000     19.92              empty_down_loop2      20000    128.41
//	         empty_do_loop     200000     18.00                 empty_do_loop      50000     97.61
//	              date_now      50000     60.26                      date_now       5000    710.08
//	            date_parse        500    865.53                    date_parse        200   2996.17
//	             prop_read      50000     12.71                     prop_read      10000     93.58
//	            prop_write      50000     13.46                    prop_write      10000     80.43
//	           prop_update      50000     18.80                   prop_update       5000    133.23
//	           prop_create       2000     67.17                   prop_create       1000    195.72
//	            prop_clone       2000     81.42                    prop_clone        500    279.45
//	           prop_delete       1000    122.26                   prop_delete        500    449.86
//	            array_read      20000     10.99                    array_read       5000     62.27
//	           array_write      50000      9.77                   array_write       5000     49.63
//	     array_prop_create        200     19.49             array_prop_create         20    100.93
//	           array_slice        200     19.69                   array_slice         20    144.29
//	     array_length_decr         50     51.37             array_length_decr         10    335.49
//	array_hole_length_decr         50     70.04        array_hole_length_decr         10    386.32
//	            array_push        100     77.95                    array_push         10    476.14
//	             array_pop         50     80.98                     array_pop         10    561.78
//	      typed_array_read      20000     10.92              typed_array_read       5000     64.84
//	     typed_array_write      20000     10.73             typed_array_write       5000     49.49
//	           global_read      50000     11.65                   global_read      10000     95.14
//	          global_write      50000     18.77                  global_write       5000     99.67
//	   global_write_strict      50000     18.78           global_write_strict      10000     99.57
//	        local_destruct       1000    220.81                local_destruct        500    782.51
//	       global_destruct       5000    100.64               global_destruct        500    554.46
//	global_destruct_strict       5000    100.40        global_destruct_strict        500    547.61
//	      global_func_call      20000     31.97              global_func_call       5000    234.71
//	             func_call      20000     26.39                     func_call       5000    177.92
//	     func_closure_call      20000     27.63             func_closure_call       5000    180.20
//	             int_arith        200     12.93                     int_arith         50     79.10
//	           float_arith        200     15.76                   float_arith         50     95.56
//	        map_set_string         10    285.29                map_set_string          2   1432.47
//	           map_set_int         20    138.69                   map_set_int          5    734.57
//	        map_set_bigint         20    196.84                map_set_bigint          2   1237.14
//	            map_delete         10    314.30                    map_delete          2   1534.94
//	          weak_map_set         20    120.36                  weak_map_set          2   1015.33
//	       weak_map_delete         10    328.84               weak_map_delete          2   1619.16
//	             array_for       2000     16.65                     array_for        500     84.23
//	          array_for_in        500     79.74                  array_for_in        100    354.30
//	          array_for_of       1000     26.60                  array_for_of        200    195.10
//	              math_min         50     41.35                      math_min         10    292.18
//	          regexp_ascii          5    752.10                  regexp_ascii          1   2260.99
//	          regexp_utf16          5    769.51                  regexp_utf16          1   2275.36
//	         string_build1        100     27.76                 string_build1         20    109.85
//	        string_build1x        100     27.73                string_build1x         20    110.57
//	        string_build2c        100     33.05                string_build2c         20    108.35
//	         string_build2         50     56.08                 string_build2         10    248.11
//	         string_build3         50     66.11                 string_build3         10    284.80
//	         string_build4         50     61.40                 string_build4         10    285.09
//	   string_build_large1          2     83.21           string_build_large1          1    295.68
//	   string_build_large2          2     81.04           string_build_large2          1    328.25
//	         int_to_string      10000     71.20                 int_to_string       5000    255.20
//	          int_toString      10000     71.28                  int_toString       2000    367.02
//	       float_to_string       2000    374.77               float_to_string       1000    721.29
//	        float_toString       2000    382.30                float_toString       1000    819.43
//	         float_toFixed       5000    175.94                 float_toFixed       2000    601.09
//	     float_toPrecision       5000    192.85             float_toPrecision       2000    634.08
//	   float_toExponential       5000    193.07           float_toExponential       2000    638.76
//	         string_to_int      20000    116.75                 string_to_int      10000    366.87
//	       string_to_float      20000    155.22               string_to_float       5000    446.63
//	        bigint32_arith        100     27.27                bigint32_arith         10    217.79
//	        bigint64_arith         50     50.71                bigint64_arith         10    311.35
//	       bigint256_arith         20    153.42               bigint256_arith          5    508.80
//	            sort_bench          1     17.23                    sort_bench          1     59.40
//	                 total              7747.74                         total             31271.53		x 4.036

func BenchmarkMicro(b *testing.B) {
	buf, err := os.ReadFile(filepath.Join("testdata", "microbench.js"))
	if err != nil {
		b.Fatal(err)
	}

	tls := libc.NewTLS()

	defer tls.Close()

	rt := XJS_NewRuntime(tls)

	defer XJS_FreeRuntime(tls, rt)

	ctx := XJS_NewContext(tls, rt)

	defer XJS_FreeContext(tls, ctx)

	XJS_SetStripInfo(tls, rt, 0)

	Xjs_std_add_helpers(tls, ctx, -1, 0)
	js := string(buf)
	cs, err := libc.CString(js)
	if err != nil {
		b.Fatalf("FAIL: err=%v", err)
	}

	defer libc.Xfree(tls, cs)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := XJS_Eval(tls, ctx, cs, Size_t(len(js)), uintptr(unsafe.Pointer(&evalFN)), MJS_EVAL_TYPE_GLOBAL)
		tg := tag(v)
		XFreeValue(tls, ctx, v)
		if tg == EJS_TAG_EXCEPTION {
			b.Fatal(exception(tls, ctx))
		}
	}
}

func exception(tls *libc.TLS, ctx uintptr) (err error) {
	e := XJS_GetException(tls, ctx)

	defer XFreeValue(tls, ctx, e)

	p := XToCString(tls, ctx, e)

	defer XJS_FreeCString(tls, ctx, p)

	return fmt.Errorf("%s", libc.GoString(p))
}
