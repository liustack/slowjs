# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
import unittest
from generate import rewrite_r14

class RegisterTests(unittest.TestCase):
    def test_g_is_not_used_as_a_c_register(self):
        for line in ['movq %rdi, %r14', 'movq 8(%r14), %r11', 'addl %r14d, %r10d', 'cmpq %r14, %r11', 'movb %r14b, (%rax)']:
            out=rewrite_r14(line, 8, 16)
            self.assertNotIn('%r14', '\n'.join(out))
            self.assertEqual(out[0].split()[0], 'movq')
            self.assertEqual(out[-1].split()[0], 'movq')
    def test_implicit_register_instruction_is_rejected(self):
        with self.assertRaises(ValueError):
            rewrite_r14('cmpxchgq %r10, (%r11,%r14,8)',8,16)
    def test_unrelated_instruction_is_unchanged(self):
        self.assertEqual(rewrite_r14('movq %rax, %rbx',8,16),['movq %rax, %rbx'])

if __name__=='__main__':unittest.main()
