# Copyright (c) 2026 The slowjs Authors.
# SPDX-License-Identifier: MIT
# Use of this source code is governed by the MIT license in LICENSE-MIT at the
# root of this repository.
"""Extract interpreter and inline helpers, making each Go boundary explicit."""
import json,pathlib,re,sys
src=pathlib.Path(sys.argv[1]).read_text();ast=json.loads(pathlib.Path(sys.argv[2]).read_text());out=pathlib.Path(sys.argv[3]);out.mkdir(exist_ok=True)
def walk(n):
 yield n
 for x in n.get('inner',[]):yield from walk(x)
def pos(n,end=False):
 x=n['range']['end' if end else 'begin'];return x['offset']+(x.get('tokLen',0) if end else 0)
def body(n):return next((x for x in n.get('inner',[]) if x.get('kind')=='CompoundStmt'),None)
def params(n):return [x for x in n.get('inner',[]) if x.get('kind')=='ParmVarDecl']
def rettype(n):return n['type']['qualType'].split('(')[0].strip()
def declref(n):
 if n.get('referencedDecl',{}).get('kind')=='FunctionDecl':return n['referencedDecl']['name']
 for c in n.get('inner',[]):
  v=declref(c)
  if v:return v
 return None
funcs={};defs={}
for n in ast['inner']:
 if n['kind']=='FunctionDecl':
  funcs[n['name']]=n
  if body(n):defs[n['name']]=n
entry=defs['JS_CallInternal']
blocked={'js_check_stack_overflow','js_malloc','js_mallocz','js_realloc','js_free','js_malloc_rt','js_free_rt','js_realloc_rt','js_mallocz_rt','js_backtrace_object','js_poll_interrupts'}
# Keep poll inline: its Go call is explicit once the counter expires.
blocked.remove('js_poll_interrupts')
keep={name for name,n in defs.items() if n.get('inline') and name not in blocked}
keep.add('js_uncatchable_pending')
used=set();queue=['JS_CallInternal'];address_refs=set();indirects=[]
while queue:
 name=queue.pop();n=defs[name]
 for x in walk(body(n)):
  if x.get('kind')=='CallExpr':
   callee=declref(x['inner'][0])
   if callee in keep and callee not in used:used.add(callee);queue.append(callee)
print('retained inline helpers',len(used),sorted(used))
# The original declarations retain all type and enum definitions. Bodies are removed.
edits=[(pos(body(n)),pos(body(n),True),';') for n in defs.values()]
prefix=src
for a,b,t in sorted(edits,reverse=True):prefix=prefix[:a]+t+prefix[b:]
header='''
typedef struct goasm_args {
 void *tls;
 JSContext *caller_ctx;
 JSValue func_obj, this_obj, new_target;
 int argc;
 JSValue *argv;
 int flags;
 JSValue result;
 void *allocated;
} goasm_args;
'''
bridges=[];registry={}
def bridge(kind,name,ret,types):
 key=(kind,name,ret,tuple(types))
 if key not in registry:
  idx=len(bridges);registry[key]=idx;bridges.append(dict(id=idx,kind=kind,name=name,ret=ret,args=types))
 return 'goasm_c_'+str(registry[key])
def signature(n,newname):
 ps=[x['type']['qualType']+' '+x.get('name','arg'+str(i)) for i,x in enumerate(params(n))]
 return rettype(n)+' '+newname+'(goasm_args *goasm_env'+(' , '+', '.join(ps) if ps else '')+')'
def rewrite(n,isentry=False):
 b=body(n);a=pos(b);z=pos(b,True);changes=[];callee_ranges=[]
 for x in walk(b):
  if x.get('kind')!='CallExpr':continue
  children=x['inner'];callee=children[0];name=declref(callee);cs=pos(callee);ce=pos(callee,True);callee_ranges.append((cs,ce))
  if name and name.startswith('__builtin_'):continue
  argnodes=children[1:]
  ret=x['type']['qualType']
  if name in used:
   target='goasm_inline_'+name;first='goasm_env'
  elif name:
   fn=funcs[name];types=[q['type']['qualType'] for q in params(fn)]
   if fn.get('variadic'):types=[q['type']['qualType'] for q in argnodes]
   target=bridge('varargs' if fn.get('variadic') else 'direct',name,ret,types);first='goasm_env'
  else:
   types=[q['type']['qualType'] for q in argnodes]
   target=bridge('indirect','indirect',ret,['uintptr_t']+types);first='goasm_env, (uintptr_t)('+src[cs:ce]+')'
  changes.append((cs,ce,target))
  # Opening parenthesis follows the callee expression.
  op=src.index('(',ce,pos(x,True))
  changes.append((op+1,op+1,first+(', ' if argnodes else '')))
 for x in walk(b):
  if x.get('kind')=='DeclRefExpr' and x.get('referencedDecl',{}).get('kind')=='FunctionDecl':
   cs=pos(x);ce=pos(x,True);name=x['referencedDecl']['name']
   if any(l<=cs and ce<=r for l,r in callee_ranges):continue
   target=bridge('address',name,'uintptr_t',[])
   changes.append((cs,ce,'((__typeof__(&'+name+'))'+target+'(goasm_env))'))
  if isentry and x.get('kind')=='ReturnStmt':
   cs=pos(x);ce=pos(x,True)
   changes.append((cs,cs+6,'{ goasm_env->result ='))
   semi=src.index(';',ce)
   changes.append((semi,semi+1,'; return; }'))
 text=src[a:z]
 for l,r,t in sorted(changes,reverse=True):text=text[:l-a]+t+text[r-a:]
 if isentry:
  init='''
 JSContext *caller_ctx=goasm_env->caller_ctx;
 JSValue func_obj=goasm_env->func_obj, this_obj=goasm_env->this_obj, new_target=goasm_env->new_target;
 int argc=goasm_env->argc, flags=goasm_env->flags;
 JSValue *argv=goasm_env->argv;
'''
  text='{'+init+text[1:]
 return text
bodies=[]
for name in sorted(used,key=lambda k:pos(defs[k])):
 n=defs[name];bodies.append('static inline __attribute__((always_inline)) '+signature(n,'goasm_inline_'+name)+rewrite(n))
entrybody=rewrite(entry,True)
protos=[]
for d in bridges:
 ty=['goasm_args *']+d['args'];protos.append('extern '+d['ret']+' goasm_c_'+str(d['id'])+'('+', '.join(ty)+');')
inlineprotos=['static inline __attribute__((always_inline)) '+signature(defs[n],'goasm_inline_'+n)+';' for n in sorted(used)]
text=prefix+header+'\n'.join(protos+inlineprotos+bodies)+'\nvoid goasm_run(goasm_args *goasm_env, void *goasm_locals) '+entrybody+'\n'
(out/'interpreter.c').write_text(text);(out/'bridges.json').write_text(json.dumps(bridges,indent=2));(out/'inline.json').write_text(json.dumps(sorted(used),indent=2))
print('boundaries',len(bridges))
