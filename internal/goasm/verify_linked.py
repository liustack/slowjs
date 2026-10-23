# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Check that Go linking preserved the offline body, except CALL relocations."""
import pathlib,struct,sys,json

def elf(path):
 b=pathlib.Path(path).read_bytes()
 if b[:6]!=b'\x7fELF\x02\x01':raise ValueError('expected ELF64 little endian')
 off=struct.unpack_from('<Q',b,40)[0];size,count,names=struct.unpack_from('<HHH',b,58)
 sections=[struct.unpack_from('<IIQQQQIIQQ',b,off+i*size) for i in range(count)]
 def data(s):return b[s[4]:s[4]+s[5]]
 def string(tab,n):return tab[n:tab.index(b'\0',n)].decode()
 strings=data(sections[names]);named={string(strings,s[0]):s for s in sections}
 return sections,named,data,string

def verify(obj,binary):
 sections,named,data,string=elf(obj);text=named['.text'];original=bytearray(data(text));relocations=[]
 for s in sections:
  if s[1]!=4 or sections[s[7]]!=text:continue
  r=data(s)
  for p in range(0,len(r),24):
   off,info,add=struct.unpack_from('<QQq',r,p)
   if info&0xffffffff!=4 or add!=-4 or original[off-1]!=0xe8:raise ValueError('unexpected relocation')
   relocations.append(off)
 sections,named,data,string=elf(binary)
 symbols=named['.symtab'];strings=data(sections[symbols[6]]);raw=data(symbols);symbol=None
 for p in range(0,len(raw),24):
  name,info,other,index,value,size=struct.unpack_from('<IBBHQQ',raw,p)
  if string(strings,name)=='github.com/liustack/slowjs/lib.goasmRun.abi0':symbol=(index,value,size)
 if symbol is None:raise ValueError('assembly interpreter absent')
 index,address,size=symbol;s=sections[index];function=data(s)[address-s[3]:address-s[3]+size]
 prefix=bytes(original[:16]);start=function.find(prefix)
 if start<0 or function.find(prefix,start+1)>=0:raise ValueError('cannot uniquely locate generated body')
 if (address+start)%16:raise ValueError('generated body lost alignment')
 actual=bytearray(function[start:start+len(original)])
 for off in relocations:
  original[off:off+4]=b'\0'*4;actual[off:off+4]=b'\0'*4
 if original!=actual:
  mismatch=next((i for i,(a,b) in enumerate(zip(original,actual)) if a!=b),len(actual))
  raise ValueError('linked body changed at byte '+str(mismatch))
 return dict(binary=str(binary),symbol_address=address,body_offset=start,body_bytes=len(original),calls=len(relocations))

if __name__=='__main__':
 for binary in sys.argv[2:]:print(json.dumps(verify(sys.argv[1],binary)))
