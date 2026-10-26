# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Tests of mathcmp.py: it must refuse what is not a full comparison.

    python3 -m unittest discover -s engine/test
"""
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parent / "mathcmp.py"
ONE = "3ff0000000000000"
TWO = "4000000000000000"


def lines(names, count, out_for=lambda n: ONE, in_for=lambda n: ONE):
    text = ""
    for n in names:
        text += f"in {n} {count} {in_for(n) * count}\n"
        text += f"out {n} {count} {out_for(n) * count}\n"
    return text


class MathCmpTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()

    def tearDown(self):
        self.dir.cleanup()

    def run_cmp(self, first, second, names="sin cos", count=3):
        paths = []
        for i, text in enumerate((first, second)):
            p = os.path.join(self.dir.name, f"f{i}.txt")
            with open(p, "w") as f:
                f.write(text)
            paths.append(p)
        r = subprocess.run([sys.executable, str(SCRIPT), "--count", str(count), "--names", names, *paths],
                           capture_output=True, text=True)
        return r.returncode, r.stdout + r.stderr

    def test_same(self):
        good = lines(["sin", "cos"], 3)
        code, out = self.run_cmp(good, good)
        self.assertEqual(code, 0, out)
        self.assertIn("6 results compared, 0 differ", out)

    def test_difference_is_found(self):
        code, out = self.run_cmp(lines(["sin", "cos"], 3), lines(["sin", "cos"], 3, out_for=lambda n: TWO if n == "cos" else ONE))
        self.assertEqual(code, 1, out)
        self.assertIn("cos      3 compared, 3 differ", out)

    def test_two_values_per_result(self):
        # a function with two results per input, as modf has
        a = lines(["sin"], 3, out_for=lambda n: ONE + TWO)
        b = lines(["sin"], 3, out_for=lambda n: ONE + ONE)
        code, out = self.run_cmp(a, b, names="sin")
        self.assertEqual(code, 1, out)

    def test_empty_files(self):
        code, out = self.run_cmp("", "")
        self.assertEqual(code, 2, out)

    def test_missing_function(self):
        code, out = self.run_cmp(lines(["sin"], 3), lines(["sin", "cos"], 3))
        self.assertEqual(code, 2, out)
        self.assertIn("no in line for cos", out)

    def test_missing_out_line(self):
        text = lines(["sin", "cos"], 3).replace(f"out cos 3 {ONE * 3}\n", "")
        code, out = self.run_cmp(text, lines(["sin", "cos"], 3))
        self.assertEqual(code, 2, out)

    def test_extra_function(self):
        code, out = self.run_cmp(lines(["sin", "cos", "tan"], 3), lines(["sin", "cos"], 3))
        self.assertEqual(code, 2, out)
        self.assertIn("unexpected function", out)

    def test_duplicate(self):
        good = lines(["sin", "cos"], 3)
        code, out = self.run_cmp(good + good, good)
        self.assertEqual(code, 2, out)
        self.assertIn("twice", out)

    def test_wrong_count(self):
        code, out = self.run_cmp(lines(["sin", "cos"], 2), lines(["sin", "cos"], 2))
        self.assertEqual(code, 2, out)
        self.assertIn("entries", out)

    def test_uneven_data(self):
        text = lines(["sin", "cos"], 3).replace(f"out sin 3 {ONE * 3}", f"out sin 3 {ONE * 3}00")
        code, out = self.run_cmp(text, lines(["sin", "cos"], 3))
        self.assertEqual(code, 2, out)
        self.assertIn("whole entries", out)

    def test_widths_differ(self):
        code, out = self.run_cmp(lines(["sin", "cos"], 3), lines(["sin", "cos"], 3, out_for=lambda n: ONE + ONE if n == "sin" else ONE))
        self.assertEqual(code, 2, out)
        self.assertIn("widths", out)

    def test_inputs_differ(self):
        code, out = self.run_cmp(lines(["sin", "cos"], 3), lines(["sin", "cos"], 3, in_for=lambda n: TWO if n == "sin" else ONE))
        self.assertEqual(code, 2, out)
        self.assertIn("inputs differ", out)

    def test_not_hex(self):
        text = lines(["sin", "cos"], 3).replace(ONE * 3, "zz" + ONE[2:] + ONE * 2, 1)
        code, out = self.run_cmp(text, lines(["sin", "cos"], 3))
        self.assertEqual(code, 2, out)
        self.assertIn("not lower case hex", out)

    def test_blank_line(self):
        code, out = self.run_cmp(lines(["sin", "cos"], 3) + "\n", lines(["sin", "cos"], 3))
        self.assertEqual(code, 2, out)


if __name__ == "__main__":
    unittest.main()
