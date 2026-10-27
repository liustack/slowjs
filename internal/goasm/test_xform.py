# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
import contextlib
import io
import os
import tempfile
import unittest
import xform

CUTILS = """TEXT ·utf8_decode(SB), $16-24
\tPCALIGN $16
\tBYTE $0xc3
\tRET

TEXT ·other(SB), $0-8
\tPCALIGN $16
\tBYTE $0xc3
\tRET

"""
ENGINE = """TEXT ·JS_CallInternal(SB), $352-24
\tNO_LOCAL_POINTERS
\tPCALIGN $16
\tBYTE $0x55
\tRET

"""


def nop(n):
    return xform.nopbytes(n)


class XformTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = self.tmp.name
        self.write("cutils_amd64.s", CUTILS)
        self.write("engine_amd64.s", ENGINE)

    def tearDown(self):
        self.tmp.cleanup()

    def write(self, name, text):
        with open(os.path.join(self.dir, name), "w") as f:
            f.write(text)

    def read(self, name):
        with open(os.path.join(self.dir, name)) as f:
            return f.read()

    def test_shift_pads_after_the_ret_of_the_first_function(self):
        xform.apply(self.dir, ["shift:32"])
        pad = "\tBYTE $0x90\n" * 32
        first, second = CUTILS.split("TEXT ·other")
        self.assertEqual(self.read("cutils_amd64.s"), first.replace("\tRET\n", "\tRET\n" + pad, 1) + "TEXT ·other" + second)
        self.assertEqual(self.read("engine_amd64.s"), ENGINE)

    def test_shift_zero_changes_nothing(self):
        xform.apply(self.dir, ["shift:0"])
        self.assertEqual(self.read("cutils_amd64.s"), CUTILS)

    def test_front_puts_the_nop_after_pcalign_of_the_named_function(self):
        xform.apply(self.dir, ["front:JS_CallInternal:16"])
        want = ENGINE.replace("\tPCALIGN $16\n", "\tPCALIGN $16\n" + nop(16), 1)
        self.assertEqual(self.read("engine_amd64.s"), want)
        self.assertEqual(self.read("cutils_amd64.s"), CUTILS)

    def test_front_nop_sizes_are_exact(self):
        for n in (0, 8, 16, 24):
            self.write("engine_amd64.s", ENGINE)
            xform.apply(self.dir, ["front:JS_CallInternal:%d" % n])
            added = self.read("engine_amd64.s").count("BYTE $0x") - 1
            self.assertEqual(added, n)

    def test_eight_bytes_are_one_multi_byte_nop(self):
        self.assertEqual(
            nop(8),
            "".join("\tBYTE $0x%02x\n" % b for b in (0x0f, 0x1f, 0x84, 0, 0, 0, 0, 0)))

    def test_front_on_the_unindented_form_of_lib(self):
        # lib/goasm_run_linux_amd64.s has no tab before NO_LOCAL_POINTERS and PCALIGN
        self.write("run_amd64.s", "TEXT ·goasmRun(SB), $352-24\nNO_LOCAL_POINTERS\nPCALIGN $16\nRET\n")
        xform.apply(self.dir, ["front:goasmRun:8"])
        self.assertEqual(
            self.read("run_amd64.s"),
            "TEXT ·goasmRun(SB), $352-24\nNO_LOCAL_POINTERS\nPCALIGN $16\n" + nop(8) + "RET\n")

    def test_front_on_a_missing_function_fails(self):
        with self.assertRaises(AssertionError):
            xform.apply(self.dir, ["front:nothing:8"])

    def test_shift_then_front_apply_in_order(self):
        xform.apply(self.dir, ["shift:32", "front:JS_CallInternal:8"])
        self.assertEqual(self.read("cutils_amd64.s").count("BYTE $0x90"), 32)
        self.assertIn(nop(8), self.read("engine_amd64.s"))

    def test_body_replaces_the_alignment(self):
        xform.apply(self.dir, ["body:64:JS_CallInternal"])
        self.assertEqual(self.read("engine_amd64.s"), ENGINE.replace("PCALIGN $16", "PCALIGN $64"))

    def test_endalign_only_the_named_function(self):
        with contextlib.redirect_stdout(io.StringIO()):
            xform.apply(self.dir, ["endalign:32:other"])
        text = self.read("cutils_amd64.s")
        self.assertEqual(text.count("PCALIGN $32"), 1)
        self.assertLess(text.index("TEXT ·other"), text.index("PCALIGN $32"))

    def test_unknown_op_stops(self):
        with self.assertRaises(SystemExit):
            xform.apply(self.dir, ["wobble:1"])


if __name__ == "__main__":
    unittest.main()
