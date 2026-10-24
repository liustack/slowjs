# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Stack rules of the generated interpreter. Each check raises ValueError.

Go may move a goroutine stack at any call that grows it, and its GC scans
frames by their pointer maps. The interpreter therefore keeps these rules:

- goasmRun has a fixed frame with no pointer map and no Go pointer in it:
  the body reads only the env and locals arguments, both C memory, and
  never the TLS argument (lower.py rejects frame accesses outside the
  compiler's own area).
- The body never changes SP and never takes the address of a stack slot
  (lower.py), and no bridge does either (bridge_stack).
- Every addressable C local lives in the TLS slot (no_allocas).
- goasmRun is not NOSPLIT, so Go gives it the stack check prologue
  (text_header here, and verify_linked.py in the linked binary).
- The interpreter enters itself only through goasm_reenter, which calls
  goasmRun and so passes that prologue (call_targets, reenter).
"""
import re

def no_allocas(ll):
 """No alloca may remain in goasm_run after allocas.cpp."""
 m=re.search(r'^define [^\n]*@goasm_run\(.*?^}',ll,re.M|re.S)
 if not m:raise ValueError('goasm_run is not defined')
 for line in m[0].splitlines():
  if re.search(r'=\s*alloca\b',line):raise ValueError('C local left on the Go stack: '+line.strip())

def text_header(line,frame):
 """goasmRun is split checked, with the fixed frame lower.py computed."""
 m=re.fullmatch(r'TEXT ·goasmRun\(SB\), \$(\d+)-24',line)
 if not m:raise ValueError('goasmRun must have a stack check and a fixed frame: '+line)
 if int(m[1])!=frame:raise ValueError('goasmRun frame %s, want %d'%(m[1],frame))

def call_targets(names):
 """The body calls only bridges, the alloca helper and goasm_reenter."""
 for n in names:
  if not (re.fullmatch(r'goasm_c_\d+',n) or n in ('goasm_alloca','goasm_reenter')):
   raise ValueError('body calls '+n+' directly')

def bridge_stack(asm):
 """Bridges keep SP fixed and publish no Go stack address."""
 for line in asm.splitlines():
  s=line.strip()
  if re.match(r'(PUSH|POP)',s) or re.search(r',\s*SP$',s):raise ValueError('bridge changes SP: '+s)
  if re.match(r'LEA',s) and 'SP)' in s:raise ValueError('bridge takes a Go stack address: '+s)

def reenter(asm):
 """goasmRun is called once, from goasm_reenter, which has a frame."""
 blocks=re.split(r'^(?=TEXT )',asm,flags=re.M)
 callers=[b.split('\n',1)[0] for b in blocks if 'CALL ·goasmRun(SB)' in b]
 if callers!=['TEXT ·goasm_reenter(SB), NOSPLIT, $64-0']:
  raise ValueError('goasmRun must be entered only through goasm_reenter: '+str(callers))
 b=next(b for b in blocks if b.startswith('TEXT ·goasm_reenter(SB)'))
 if b.count('CALL')!=1:raise ValueError('goasm_reenter calls more than goasmRun')

def morestack(function,address,start,length,target):
 """The linked goasmRun calls runtime.morestack outside the body."""
 for i in range(len(function)-4):
  if start<=i<start+length:continue
  if function[i]==0xe8:
   rel=int.from_bytes(function[i+1:i+5],'little',signed=True)
   if address+i+5+rel==target:return
 raise ValueError('goasmRun has no stack check prologue')
