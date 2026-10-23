# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
import pathlib,tempfile,unittest
from bridges import generate
class BridgeTests(unittest.TestCase):
 def test_aggregate_moves_wholly_to_stack_when_one_integer_register_remains(self):
  # env + four pointers leave only R9. A JSValue needs two registers,
  # so it goes on the C stack and the following integer still uses R9.
  b=[dict(id=0,kind='direct',name='example',ret='JSValue',args=['void *']*4+['JSValue','int'])]
  sig={'github.com/liustack/slowjs/lib:_example':dict(Params=['*libc.TLS']+['uintptr']*4+['TJSValue','int32'],Results=['TJSValue'])}
  with tempfile.TemporaryDirectory() as tmp:
   generate(b,sig,pathlib.Path(tmp));s=(pathlib.Path(tmp)/'goasm_bridges_linux_amd64.s').read_text()
   self.assertIn('MOVL R9, 56(SP)',s)
   self.assertIn('MOVQ 128(SP), AX\nMOVQ AX, 40(SP)',s)
   self.assertIn('MOVQ 136(SP), AX\nMOVQ AX, 48(SP)',s)
   self.assertIn('MOVQ 64(SP), AX\nMOVQ 72(SP), DX',s)
 def test_unknown_c_type_is_rejected(self):
  b=[dict(id=0,kind='direct',name='example',ret='long double',args=[])]
  with tempfile.TemporaryDirectory() as tmp:
   with self.assertRaises(ValueError):generate(b,{},pathlib.Path(tmp))
 def test_varargs_cannot_overwrite_external_locals(self):
  b=[dict(id=0,kind='varargs',name='example',ret='void',args=['void *']+['int']*7)]
  sig={'lib:_example':dict(Params=['*libc.TLS','uintptr','uintptr'],Results=[])}
  with tempfile.TemporaryDirectory() as tmp:
   with self.assertRaises(ValueError):generate(b,sig,pathlib.Path(tmp))
