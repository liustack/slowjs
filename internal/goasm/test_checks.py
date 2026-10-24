# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
import pathlib,tempfile,unittest
import checks
from bridges import generate

LL='''define void @goasm_run(ptr %0, ptr %1) {
  %3 = getelementptr i8, ptr %1, i64 16
BODY
  ret void
}
'''
REENTER='''TEXT ·goasm_reenter(SB), NOSPLIT, $64-0
MOVQ DI, 8(SP)
CALL ·goasmRun(SB)
RET
'''
class StackRuleTests(unittest.TestCase):
 def test_c_locals_live_in_the_tls_slot(self):
  checks.no_allocas(LL.replace('BODY','  store i64 0, ptr %3'))
  with self.assertRaisesRegex(ValueError,'C local left on the Go stack'):
   checks.no_allocas(LL.replace('BODY','  %4 = alloca [16 x i8], align 16'))
 def test_goasm_run_has_a_stack_check_and_fixed_frame(self):
  checks.text_header('TEXT ·goasmRun(SB), $608-24',608)
  for bad in ('TEXT ·goasmRun(SB), NOSPLIT, $608-24','TEXT ·goasmRun(SB), NOSPLIT|NOFRAME, $0-24'):
   with self.subTest(bad=bad),self.assertRaisesRegex(ValueError,'stack check'):checks.text_header(bad,608)
  with self.assertRaisesRegex(ValueError,'frame'):checks.text_header('TEXT ·goasmRun(SB), $512-24',608)
 def test_body_enters_itself_only_through_the_stub(self):
  checks.call_targets(['goasm_c_0','goasm_alloca','goasm_reenter'])
  for bad in ('goasmRun','runtime.morestack','memcpy'):
   with self.subTest(bad=bad),self.assertRaisesRegex(ValueError,'directly'):checks.call_targets(['goasm_c_1',bad])
 def test_only_the_stub_calls_goasm_run(self):
  checks.reenter(REENTER)
  with self.assertRaisesRegex(ValueError,'only through goasm_reenter'):
   checks.reenter(REENTER+'TEXT ·goasm_c_3(SB), NOSPLIT, $32-0\nCALL ·goasmRun(SB)\nRET\n')
  with self.assertRaisesRegex(ValueError,'only through goasm_reenter'):
   checks.reenter(REENTER.replace('$64-0','$0-0'))
  with self.assertRaisesRegex(ValueError,'more than'):
   checks.reenter(REENTER.replace('RET','CALL ·other(SB)\nRET'))
 def test_bridges_keep_sp_and_publish_no_stack_address(self):
  for bad in ('SUBQ $16, SP','MOVQ AX, SP','PUSHQ AX','POPQ BX','LEAQ 8(SP), AX','LEAL 8(SP), DI'):
   with self.subTest(bad=bad),self.assertRaises(ValueError):checks.bridge_stack('MOVQ BX, 24(SP)\n'+bad+'\nRET\n')
 def test_generated_bridges_pass(self):
  b=[dict(id=0,kind='indirect',name='indirect',ret='JSValue',args=['uintptr_t','void *','JSValue','int'])]
  with tempfile.TemporaryDirectory() as tmp:
   generate(b,{},pathlib.Path(tmp))
 def test_linked_function_calls_morestack(self):
  # A prologue CALL to morestack at 0x10, then a 16 byte body.
  address,target=0x1000,0x5000
  call=bytes([0xe8])+(target-(address+0x10+5)).to_bytes(4,'little',signed=True)
  function=b'\x90'*0x10+call+b'\x90'*11+b'\xcc'*16
  checks.morestack(function,address,32,16,target)
  with self.assertRaisesRegex(ValueError,'no stack check'):
   checks.morestack(b'\x90'*32+b'\xcc'*16,address,32,16,target)
  # A call inside the body does not count as the prologue's.
  body=b'\x90'*0x10+call+b'\x90'*11
  with self.assertRaisesRegex(ValueError,'no stack check'):
   checks.morestack(b'\x90'*0+body,address,0,len(body),target)

if __name__=='__main__':unittest.main()
