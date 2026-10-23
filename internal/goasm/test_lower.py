# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
import pathlib,subprocess,sys,tempfile,unittest

PROLOGUE='''goasm_run:
pushq %rbp
movq %rsp, %rbp
pushq %r15
pushq %r14
pushq %r13
pushq %r12
pushq %rbx
subq $56, %rsp
'''
EPILOGUE='''
addq $56, %rsp
popq %rbx
popq %r12
popq %r13
popq %r14
popq %r15
popq %rbp
retq
'''
class FrameTests(unittest.TestCase):
 def lower(self,body):
  with tempfile.TemporaryDirectory() as tmp:
   p=pathlib.Path(tmp);(p/'input.s').write_text(PROLOGUE+body+EPILOGUE)
   r=subprocess.run([sys.executable,str(pathlib.Path(__file__).with_name('lower.py')),str(p/'input.s'),str(p/'output.s')],capture_output=True,text=True)
   return r,(p/'output.s').read_text() if r.returncode==0 else ''
 def test_frame_local_is_rebased_without_changing_go_frame_pointer(self):
  r,s=self.lower('movq -48(%rbp), %r14\nmovq %r14, (%rdi)')
  self.assertEqual(r.returncode,0,r.stderr)
  self.assertNotIn('%rbp',s);self.assertNotIn('%r14',s)
  self.assertIn('48(%rsp)',s)
 def test_dynamic_stack_mutation_is_rejected(self):
  r,_=self.lower('subq %rax, %rsp')
  self.assertNotEqual(r.returncode,0)
 def test_backend_stack_address_escape_is_rejected(self):
  r,_=self.lower('leaq -48(%rbp), %rdi')
  self.assertNotEqual(r.returncode,0)
 def test_access_outside_fixed_frame_is_rejected(self):
  r,_=self.lower('movq -104(%rbp), %rax')
  self.assertNotEqual(r.returncode,0)
 def test_stack_and_frame_subregisters_are_rejected(self):
  for register,op,source in (('esp','movl','eax'),('sp','movw','ax'),('spl','movb','al'),('ebp','movl','eax'),('bp','movw','ax'),('bpl','movb','al')):
   with self.subTest(register=register):
    r,_=self.lower(op+' %'+source+', %'+register)
    self.assertNotEqual(r.returncode,0)
