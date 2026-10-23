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
 def lower(self,body,data='',after=''):
  with tempfile.TemporaryDirectory() as tmp:
   p=pathlib.Path(tmp);(p/'input.s').write_text(PROLOGUE+body+EPILOGUE+after+'.Lfunc_end0:\n'+data)
   r=subprocess.run([sys.executable,str(pathlib.Path(__file__).with_name('lower.py')),str(p/'input.s'),str(p/'output.s')],capture_output=True,text=True)
   return r,(p/'output.s').read_text() if r.returncode==0 else ''
 def test_frame_local_is_rebased_without_changing_go_frame_pointer(self):
  r,s=self.lower('movq -48(%rbp), %r14\nmovq %r14, (%rdi)')
  self.assertEqual(r.returncode,0,r.stderr)
  self.assertNotIn('%rbp',s)
  self.assertIn('48(%rsp)',s)
 def test_r14_is_an_ordinary_c_register(self):
  r,s=self.lower('movq %rdi, %r14\naddl %r14d, %r10d')
  self.assertEqual(r.returncode,0,r.stderr)
  self.assertIn('movq %rdi, %r14\naddl %r14d, %r10d',s)
 def test_dispatch_through_register_is_kept(self):
  r,s=self.lower('jmpq *%rax')
  self.assertEqual(r.returncode,0,r.stderr)
  self.assertIn('jmpq *%rax',s)
 def test_blocks_after_the_epilogue_are_rebased_code(self):
  r,s=self.lower('jmp .LBB0_2\n.LBB0_1:\nnop','','.LBB0_2:\nmovq -48(%rbp), %rax\njmp .LBB0_1\n')
  self.assertEqual(r.returncode,0,r.stderr)
  self.assertIn('jmp .Lgoasm_return\n.LBB0_2:\nmovq 48(%rsp), %rax\njmp .LBB0_1',s)
 def test_blocks_after_the_epilogue_are_checked(self):
  r,_=self.lower('nop','','.LBB0_2:\nsubq %rax, %rsp\n')
  self.assertNotEqual(r.returncode,0)
 def test_dispatch_through_memory_is_rejected(self):
  r,_=self.lower('jmpq *(%r15,%r14,8)')
  self.assertNotEqual(r.returncode,0)
 def test_absolute_label_table_is_rejected(self):
  r,_=self.lower('nop','.section .rodata\n.quad .Ltmp0\n')
  self.assertNotEqual(r.returncode,0)
 def test_dynamic_stack_mutation_is_rejected(self):
  r,_=self.lower('subq %rax, %rsp')
  self.assertNotEqual(r.returncode,0)
 def test_backend_stack_address_escape_is_rejected(self):
  r,_=self.lower('leaq -48(%rbp), %rdi')
  self.assertNotEqual(r.returncode,0)
 def test_access_outside_fixed_frame_is_rejected(self):
  r,_=self.lower('movq -104(%rbp), %rax')
  self.assertNotEqual(r.returncode,0)
 def test_go_arguments_are_never_read(self):
  # 16(%rbp) is past the compiler frame: the Go arguments, among them the
  # typed TLS pointer, which must not reach the frame without a pointer map.
  r,_=self.lower('movq 16(%rbp), %rax')
  self.assertNotEqual(r.returncode,0)
 def test_stack_address_never_escapes(self):
  for bad in ('movq %rsp, %rax','leaq 8(%rsp), %rdi','pushq %rax','popq %rax','addq $8, %rsp'):
   with self.subTest(bad=bad):
    r,_=self.lower(bad)
    self.assertNotEqual(r.returncode,0)
 def test_stack_and_frame_subregisters_are_rejected(self):
  for register,op,source in (('esp','movl','eax'),('sp','movw','ax'),('spl','movb','al'),('ebp','movl','eax'),('bp','movw','ax'),('bpl','movb','al')):
   with self.subTest(register=register):
    r,_=self.lower(op+' %'+source+', %'+register)
    self.assertNotEqual(r.returncode,0)
