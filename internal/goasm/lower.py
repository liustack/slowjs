# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Convert a fixed LLVM frame to Go's fixed frame, rejecting unknown mutations.

R14 stays an ordinary C register. Go's ABI0 leaves R14 and X15 undefined:
Go code reloads g from thread local storage when it calls or is called from
ABI0 code, and the signal handlers do the same. The bridges save R14 with the
other SysV callee-saved registers.
"""
import re,json,sys,pathlib
src=pathlib.Path(sys.argv[1]).read_text().splitlines()
entry=next(i for i,l in enumerate(src) if l.startswith('goasm_run:'))
sub=next(i for i in range(entry,len(src)) if re.search(r'subq\s+\$\d+, %rsp',src[i]))
saved=re.findall(r'pushq\s+%(\w+)', '\n'.join(src[entry:sub]))
if saved!=['rbp','r15','r14','r13','r12','rbx']:raise ValueError('unreviewed LLVM prologue '+str(saved))
stack=int(re.search(r'\$(\d+)',src[sub])[1])+8*(len(saved)-1)
frame=(stack+15)//16*16
ret=next(i for i,l in enumerate(src) if l.strip()=='retq')
end=next(i for i in range(ret,-1,-1) if re.search(r'addq\s+\$\d+, %rsp',src[i]))
if re.findall(r'popq\s+%(\w+)', '\n'.join(src[end:ret]))!=list(reversed(saved)):raise ValueError('unreviewed LLVM epilogue')
def clean(line):
 line=line.split('#')[0].strip()
 if not line:return None
 if line.startswith(('.cfi_','.file','.section','.text','.globl','.type','.size','.ident','.addrsig')):return None
 return line
body=[]
for raw in src[sub+1:end]:
 line=clean(raw)
 if not line:continue
 if re.search(r'%(?:esp|sp|spl|ebp|bp|bpl)\b',line):raise ValueError('stack or frame subregister '+line)
 line=re.sub(r'(-?\d*)\(%rbp\)',lambda m:str(stack+int(m[1] or 0))+'(%rsp)',line)
 if '%rbp' in line:raise ValueError('unhandled frame pointer '+line)
 if re.search(r'%rsp(?![a-z]|\))',line) or (line.startswith('lea') and '%rsp' in line):raise ValueError('escaping stack address '+line)
 for m in re.finditer(r'(-?\d*)\(%rsp\)',line):
  if not 0<=int(m[1] or 0)<stack:raise ValueError('stack access outside compiler frame '+line)
 if re.search(r',\s*%rsp\b',line) or line.startswith(('push','pop','leave')):raise ValueError('SP write '+line)
 line=line.replace('movapd','movupd').replace('movaps','movups')
 # Computed gotos jump through a register holding table base plus offset.
 if line.startswith('jmp') and '*' in line and not re.fullmatch(r'jmpq\s+\*%r\w+',line):raise ValueError('unreviewed indirect jump '+line)
 body.append(line)
body+=['jmp .Lgoasm_return']
data=[]
for raw in src[ret+1:]+src[:entry]:
 line=clean(raw)
 if not line:continue
 if re.match(r'\.quad\s+\.Ltmp',line):raise ValueError('absolute label address '+line)
 data.append(line)
out=['.text','.globl goasm_run','goasm_run:',f'movq {frame+24}(%rsp), %rdi',f'movq {frame+32}(%rsp), %rsi']+body+data+['.Lgoasm_return:']
assert not any(re.search(r'%rbp\b',l) for l in out)
pathlib.Path(sys.argv[2]).write_text('\n'.join(out)+'\n')
pathlib.Path(sys.argv[2]+'.json').write_text(json.dumps(dict(frame=frame,stack=stack)))
