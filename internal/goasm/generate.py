# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Lower LLVM's C register allocation while leaving Go's g register untouched."""
import re

REGS = [('r11','r11d','r11w','r11b'),('r10','r10d','r10w','r10b'),
        ('rax','eax','ax','al'),('rcx','ecx','cx','cl'),('rdx','edx','dx','dl')]

def rewrite_r14(line, value_slot, save_slot):
    if not re.search(r'%r14(?:d|w|b)?\b',line):
        return [line]
    op=line.split()[0]
    if op.startswith(('j','call','ret','push','pop')):
        raise ValueError('control flow or stack instruction requires explicit lowering: '+line)
    allowed={'movq','movl','movb','movw','movzbl','movzwl','movslq',
             'movupd','movups','leaq','leal','cmpl','cmpq','addl','addq',
             'incl','testb','testl','shlq','shll','subq','xorl'}
    if op not in allowed:
        raise ValueError('unreviewed R14 instruction: '+line)
    regs=set(re.findall(r'%([a-z0-9]+)',line))
    scratch=next((r for r in REGS if not regs.intersection(r)),None)
    if scratch is None:
        raise ValueError('no scratch register: '+line)
    aliases=dict(zip(('r14','r14d','r14w','r14b'),scratch))
    changed=re.sub(r'%(r14(?:d|w|b)?)\b',lambda m:'%'+aliases[m[1]],line)
    # MOV does not alter flags. Even a 32-bit destination updates the virtual
    # 64-bit register exactly as the original instruction did.
    return [f'movq %{scratch[0]}, {save_slot}(%rsp)',
            f'movq {value_slot}(%rsp), %{scratch[0]}',changed,
            f'movq %{scratch[0]}, {value_slot}(%rsp)',
            f'movq {save_slot}(%rsp), %{scratch[0]}']
