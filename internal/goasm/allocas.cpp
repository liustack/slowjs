// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
// Use of this source code is governed by the MIT license in LICENSE-MIT at the
// root of this repository.
// Move every addressable C local off the movable Go stack after optimization.
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/IntrinsicInst.h"
#include "llvm/IR/PassManager.h"
#include "llvm/Passes/PassBuilder.h"
#include "llvm/Plugins/PassPlugin.h"
#include "llvm/Support/raw_ostream.h"
using namespace llvm;
struct ExternalAllocas : PassInfoMixin<ExternalAllocas> {
 PreservedAnalyses run(Module &M, ModuleAnalysisManager &) {
  auto *F=M.getFunction("goasm_run");
  if (!F) report_fatal_error("missing goasm_run");
  F->removeParamAttr(1, Attribute::ReadNone);
  F->removeParamAttr(1, Attribute::Captures);
  const auto &DL=M.getDataLayout();
  SmallVector<AllocaInst *> work;
  for(auto &B:*F) for(auto &I:B) if(auto *A=dyn_cast<AllocaInst>(&I)) work.push_back(A);
  SmallVector<Instruction *> lifetimes;
  for(auto &B:*F) for(auto &I:B) if(auto *II=dyn_cast<IntrinsicInst>(&I))
   if(II->getIntrinsicID()==Intrinsic::lifetime_start || II->getIntrinsicID()==Intrinsic::lifetime_end) lifetimes.push_back(II);
  for(auto *I:lifetimes) I->eraseFromParent();
  uint64_t offset=0;
  for(auto *A:work) {
   IRBuilder<> B(A); Value *replacement;
   if(auto *N=dyn_cast<ConstantInt>(A->getArraySize())) {
    offset=alignTo(offset,A->getAlign().value());
    replacement=B.CreateConstGEP1_64(B.getInt8Ty(),F->getArg(1),offset);
    offset+=DL.getTypeAllocSize(A->getAllocatedType()).getFixedValue()*N->getZExtValue();
   } else {
    auto callee=M.getOrInsertFunction("goasm_alloca",FunctionType::get(B.getPtrTy(),{B.getPtrTy(),B.getInt64Ty()},false));
    Value *size=B.CreateZExtOrTrunc(A->getArraySize(),B.getInt64Ty());
    size=B.CreateMul(size,B.getInt64(DL.getTypeAllocSize(A->getAllocatedType()).getFixedValue()));
    replacement=B.CreateCall(callee,{F->getArg(0),size});
   }
   A->replaceAllUsesWith(replacement); A->eraseFromParent();
  }
  errs()<<"GOASM_LOCALS_SIZE="<<alignTo(offset,16)<<"\n";
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
