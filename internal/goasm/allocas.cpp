// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Move every addressable C local off the movable Go stack after optimization,
// and make the computed goto table hold offsets from itself instead of
// absolute addresses, which would need relocations Go cannot express inside
// the assembled body. The address is base plus offset, computed in IR, so the
// register allocator picks the scratch register of each dispatch.
#include "llvm/Transforms/Utils/BasicBlockUtils.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/IntrinsicInst.h"
#include "llvm/IR/PassManager.h"
#include "llvm/Passes/PassBuilder.h"
#include "llvm/Plugins/PassPlugin.h"
#include "llvm/Support/CommandLine.h"
#include "llvm/Support/raw_ostream.h"
using namespace llvm;
// The locals sit at a fixed offset in the TLS slot that the first argument
// points to. Addressing them from that argument, not from the locals
// argument, keeps one register free for the interpreter.
static cl::opt<unsigned> LocalsOffset("goasm-locals",cl::desc("offset of the C locals from the goasm_args pointer"),cl::Required);
static void relativeDispatch(Module &M, Function *F) {
 auto &C=M.getContext(); auto *I32=Type::getInt32Ty(C); auto *I64=Type::getInt64Ty(C);
 SmallVector<GlobalVariable *> tables;
 for(auto &G:M.globals()) {
  auto *A=G.hasInitializer()?dyn_cast<ConstantArray>(G.getInitializer()):nullptr;
  if(!A) continue;
  bool labels=true;
  for(auto &E:A->operands()) if(!isa<BlockAddress>(E)) labels=false;
  if(labels) tables.push_back(&G);
 }
 if(tables.size()!=1) report_fatal_error("expected one computed goto table");
 auto *G=tables[0]; auto *A=cast<ConstantArray>(G->getInitializer());
 auto *T=ArrayType::get(I32,A->getNumOperands());
 auto *R=new GlobalVariable(M,T,true,GlobalValue::InternalLinkage,nullptr,G->getName()+".rel");
 R->setAlignment(Align(4));
 Constant *base=ConstantExpr::getPtrToInt(R,I64);
 SmallVector<Constant *> offsets;
 for(auto &E:A->operands()) {
  if(cast<BlockAddress>(E)->getFunction()!=F) report_fatal_error("dispatch label outside goasm_run");
  offsets.push_back(ConstantExpr::getTrunc(ConstantExpr::getSub(ConstantExpr::getPtrToInt(cast<Constant>(E),I64),base),I32));
 }
 R->setInitializer(ConstantArray::get(T,offsets));
 for(auto *U:make_early_inc_range(G->users())) {
  auto *P=dyn_cast<GetElementPtrInst>(U);
  if(!P || P->getNumIndices()!=1 || P->getFunction()!=F) report_fatal_error("unreviewed use of the dispatch table");
  for(auto *L:make_early_inc_range(P->users())) {
   auto *Load=dyn_cast<LoadInst>(L);
   if(!Load || Load->getPointerOperand()!=P) report_fatal_error("unreviewed use of a dispatch entry");
   IRBuilder<> B(Load);
   Value *index=B.CreateSExtOrTrunc(P->getOperand(1),I64);
   Value *offset=B.CreateLoad(I32,B.CreateInBoundsGEP(I32,R,index));
   // Not inbounds: the label lies outside the table.
   Load->replaceAllUsesWith(B.CreateGEP(B.getInt8Ty(),R,B.CreateSExt(offset,I64)));
   Load->eraseFromParent();
  }
  P->eraseFromParent();
 }
 if(!G->use_empty()) report_fatal_error("dispatch table still used");
 G->eraseFromParent();
}
struct ExternalAllocas : PassInfoMixin<ExternalAllocas> {
 PreservedAnalyses run(Module &M, ModuleAnalysisManager &) {
  auto *F=M.getFunction("goasm_run");
  if (!F) report_fatal_error("missing goasm_run");
  F->removeParamAttr(1, Attribute::ReadNone);
  F->removeParamAttr(1, Attribute::Captures);
  const auto &DL=M.getDataLayout();
  relativeDispatch(M,F);
  SmallVector<AllocaInst *> work;
  for(auto &B:*F) for(auto &I:B) if(auto *A=dyn_cast<AllocaInst>(&I)) work.push_back(A);
  SmallVector<Instruction *> lifetimes;
  for(auto &B:*F) for(auto &I:B) if(auto *II=dyn_cast<IntrinsicInst>(&I))
   if(II->getIntrinsicID()==Intrinsic::lifetime_start || II->getIntrinsicID()==Intrinsic::lifetime_end) lifetimes.push_back(II);
  for(auto *I:lifetimes) I->eraseFromParent();
  // Fixed allocas share the locals area. A variable one of up to
  // GOASM_SMALL bytes takes the space right after it, inline. A larger one
  // calls goasm_alloca, which mallocs it.
  // Each use computes its local's address from the slot start right where
  // it needs it. One address made at the entry and kept for the whole
  // function would be spilled to the frame on every call.
  Value *slot=F->getArg(0);
  uint64_t offset=0;
  SmallVector<AllocaInst *> variable;
  for(auto *A:work) {
   auto *N=dyn_cast<ConstantInt>(A->getArraySize());
   if(!N) { variable.push_back(A); continue; }
   offset=alignTo(offset,A->getAlign().value());
   for(Use &U:make_early_inc_range(A->uses())) {
    auto *I=cast<Instruction>(U.getUser());
    Instruction *at=I;
    if(auto *P=dyn_cast<PHINode>(I)) at=P->getIncomingBlock(U)->getTerminator();
    IRBuilder<> B(at);
    U.set(B.CreateConstGEP1_64(B.getInt8Ty(),slot,LocalsOffset+offset));
   }
   offset+=DL.getTypeAllocSize(A->getAllocatedType()).getFixedValue()*N->getZExtValue();
   A->eraseFromParent();
  }
  // The inline space is one, so it is only right for a single variable
  // alloca: the frame's own, in JS_CallInternal.
  if(variable.size()>1) report_fatal_error("more than one variable alloca");
  const uint64_t small=768, area=alignTo(offset,16);
  for(auto *A:variable) {
   IRBuilder<> B(A);
   auto callee=M.getOrInsertFunction("goasm_alloca",FunctionType::get(B.getPtrTy(),{B.getPtrTy(),B.getInt64Ty()},false));
   Value *size=B.CreateZExtOrTrunc(A->getArraySize(),B.getInt64Ty());
   size=B.CreateMul(size,B.getInt64(DL.getTypeAllocSize(A->getAllocatedType()).getFixedValue()));
   Value *inl=B.CreateConstGEP1_64(B.getInt8Ty(),slot,LocalsOffset+area);
   Value *big=B.CreateICmpUGT(size,B.getInt64(small));
   Instruction *then=SplitBlockAndInsertIfThen(big,A,false);
   B.SetInsertPoint(then);
   Value *heap=B.CreateCall(callee,{F->getArg(0),size});
   B.SetInsertPoint(A);
   PHINode *phi=B.CreatePHI(B.getPtrTy(),2);
   phi->addIncoming(inl,cast<Instruction>(big)->getParent());
   phi->addIncoming(heap,then->getParent());
   A->replaceAllUsesWith(phi); A->eraseFromParent();
  }
  errs()<<"GOASM_LOCALS_SIZE="<<area<<"\n";
  return PreservedAnalyses::none();
 }
};
extern "C" LLVM_ATTRIBUTE_WEAK PassPluginLibraryInfo llvmGetPassPluginInfo() {
 return {LLVM_PLUGIN_API_VERSION,"goasm-external-allocas","22.1.8",[](PassBuilder &PB) {
  PB.registerPipelineParsingCallback([](StringRef name,ModulePassManager &PM,ArrayRef<PassBuilder::PipelineElement>){
   if(name!="goasm-external-allocas") return false;
   PM.addPass(ExternalAllocas()); return true;
  });
 }};
}
