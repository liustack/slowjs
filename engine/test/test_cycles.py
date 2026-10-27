# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Tests of cycles.py: it must find a cycle that no count cuts, through
direct and indirect calls, and only those, and a path from one function to
another, through counts too.

    python3 -m unittest discover -s engine/test
"""
import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))
import cycles  # noqa: E402

HEAD = """\
@table = internal constant [1 x ptr] [ptr @target]
"""

TAIL = """\
!1 = !{i32 111}
!2 = !{i32 222}
"""


def module(body):
    return HEAD + body + TAIL


class CyclesTests(unittest.TestCase):
    def test_self_call(self):
        ir = module("""\
define internal void @rec(i32 noundef %n) {
entry:
  call void @rec(i32 noundef 1)
  ret void
}
""")
        self.assertEqual(cycles.cycles(ir), [["rec"]])

    def test_count_cuts(self):
        ir = module("""\
define internal void @a() {
entry:
  %call = call i32 @js_internal_enter(ptr noundef null)
  call void @b()
  ret void
}
define internal void @b() {
entry:
  call void @a()
  ret void
}
""")
        self.assertEqual(cycles.cycles(ir), [])

    def test_two_functions(self):
        ir = module("""\
define internal { i64, i64 } @a() {
entry:
  %call = call { i64, i64 } @b()
  ret { i64, i64 } %call
}
define internal { i64, i64 } @b() {
entry:
  %call = call { i64, i64 } @a()
  ret { i64, i64 } %call
}
""")
        self.assertEqual(cycles.cycles(ir), [["a", "b"]])

    def test_indirect_call_by_type(self):
        ir = module("""\
define internal void @target(ptr noundef %f) !kcfi_type !1 {
entry:
  %0 = load ptr, ptr %f
  call void %0(ptr noundef %f) [ "kcfi"(i32 111) ]
  ret void
}
""")
        self.assertEqual(cycles.cycles(ir), [["target"]])

    def test_indirect_call_other_type(self):
        ir = module("""\
define internal void @target(ptr noundef %f) !kcfi_type !1 {
entry:
  %0 = load ptr, ptr %f
  call void %0(ptr noundef %f) [ "kcfi"(i32 222) ]
  ret void
}
""")
        self.assertEqual(cycles.cycles(ir), [])

    def test_address_not_taken(self):
        ir = """\
define internal void @f(ptr noundef %p) !kcfi_type !1 {
entry:
  %0 = load ptr, ptr %p
  call void %0(ptr noundef %p) [ "kcfi"(i32 111) ]
  ret void
}
!1 = !{i32 111}
"""
        self.assertEqual(cycles.cycles(ir), [])

    def test_reach_direct_and_indirect(self):
        ir = module("""\
define internal void @a() {
entry:
  call void @b()
  ret void
}
define internal void @b() {
entry:
  %0 = load ptr, ptr @table
  call void %0(ptr noundef null) [ "kcfi"(i32 111) ]
  ret void
}
define internal void @target(ptr noundef %f) !kcfi_type !1 {
entry:
  %call = call i32 @js_internal_enter(ptr noundef null)
  call void @c()
  ret void
}
define internal void @c() {
entry:
  ret void
}
""")
        pairs = [("a", "c"), ("c", "a"), ("a", "missing")]
        self.assertEqual(cycles.reaches(ir, pairs),
                         [("a", "c", ["a", "b", "target", "c"])])

    def test_reach_other_type(self):
        ir = module("""\
define internal void @a() {
entry:
  %0 = load ptr, ptr @table
  call void %0(ptr noundef null) [ "kcfi"(i32 222) ]
  ret void
}
define internal void @target(ptr noundef %f) !kcfi_type !1 {
entry:
  ret void
}
""")
        self.assertEqual(cycles.reaches(ir, [("a", "target")]), [])

    def test_untyped_indirect_call(self):
        ir = module("""\
define internal void @f(ptr noundef %p) {
entry:
  call void %p()
  ret void
}
""")
        with self.assertRaises(SystemExit):
            cycles.cycles(ir)


if __name__ == "__main__":
    unittest.main()
