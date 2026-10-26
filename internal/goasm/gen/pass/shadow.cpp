// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Shadow-stack pass for the new engine generator. It runs after -O2, when the
// only allocas left are the locals whose address is taken and the dynamic
// ones. It moves every one of them onto the VM's shadow stack, a region the
// host allocates outside the Go heap, so no C address ever points into the
// movable Go stack.
//
// Contract of every engine function: argument 0 is the shadow-stack pointer,
// the first free byte of the shadow stack. A function lays its fixed locals
// out from there and hands each callee the first byte above everything it has
// live. Returning frees the frame for free, since the caller's pointer never
// moved. Dynamic allocas keep the running top in the frame's first word.
//
// The region is 2^shadow-size-log2 bytes and aligned to its size, so the
// space left is computed from the pointer alone. __shadow_remaining() gives it
// to C, which checks it at the places that can raise RangeError. A dynamic
// alloca that would cross the end traps: the C checks are meant to make that
// unreachable.
//
// The engine's C has no shadow pointer of its own: with -shadow-add-sp the
// pass gives every function but the register-entry leaves a new argument 0
// and passes it at every call, direct or through a pointer. A JSValue that
// C passes in memory because the argument registers ran out (byval) goes in
// the argument block like any other argument. A computed goto table becomes
// offsets from itself, so it needs no relocation and can live inside its
// function. -shadow-icall-sigs gives every indirect call a "kcfi" operand
// bundle whose type id encodes its signature, which llc keeps on the machine
// call and the plugin in icall.cpp writes out as a symbol at it: the
// lowering reads it to move the arguments into the Go argument area, and
// the post-link check reads it again from llc's object. -shadow-report prints the signature of every function, the
// address-taken ones and the call graph for the generator.
#include "llvm/ADT/DenseMap.h"
#include "llvm/ADT/SmallPtrSet.h"
#include "llvm/Analysis/ValueTracking.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/IntrinsicInst.h"
#include "llvm/IR/Intrinsics.h"
#include "llvm/IR/PassManager.h"
#include "llvm/Passes/PassBuilder.h"
#include "llvm/Plugins/PassPlugin.h"
#include "llvm/Support/CommandLine.h"
#include "llvm/Support/raw_ostream.h"
#include "llvm/TargetParser/Triple.h"
#include "llvm/Transforms/Utils/BasicBlockUtils.h"
#include <map>
#include <set>
#include <string>
using namespace llvm;

static cl::opt<unsigned> SizeLog2("shadow-size-log2", cl::init(20), cl::desc("log2 of the shadow-stack region size"));
static cl::opt<unsigned> Guard("shadow-guard", cl::init(16), cl::desc("bytes kept free at the region end"));
static cl::opt<std::string> CallConv("shadow-cc", cl::init(""),
                                     cl::desc("calling convention of unified entries and host callbacks: empty (C) or preserve_none"));
static cl::opt<unsigned> MaxIntArgs("shadow-max-int-args", cl::init(0),
                                    cl::desc("most integer argument registers a unified function may use: more go in a block (0: no limit)"));
static cl::list<std::string> Leaves("shadow-leaf", cl::CommaSeparated, cl::desc("register-entry leaves: no shadow pointer"));
static cl::opt<bool> AddSP("shadow-add-sp", cl::init(false),
                           cl::desc("add the shadow-stack pointer as argument 0 of every engine function and call"));
static cl::opt<bool> ICallSigs("shadow-icall-sigs", cl::init(false),
                               cl::desc("bind the signature of every indirect call to it as a kcfi type id"));
static cl::opt<bool> Report("shadow-report", cl::init(false),
                            cl::desc("print signatures, address-taken functions and call edges"));
static cl::opt<bool> KeepGoing("shadow-keep-going", cl::init(false),
                               cl::desc("report a function the C must change (REJECT) and replace its body by a trap, instead of stopping"));
static cl::opt<bool> Varargs("shadow-varargs", cl::init(false),
                             cl::desc("pass the variable arguments of every call in a block of 8-byte slots (experimental)"));

// reject stops on a rule the C breaks, or with -shadow-keep-going reports it
// and empties the function so the rest of the unit still goes through.
static bool reject(Function &F, const Twine &why) {
  if (!KeepGoing)
    report_fatal_error(why + " in " + F.getName());
  errs() << "REJECT " << F.getName() << " " << why << "\n";
  F.deleteBody();
  F.setLinkage(GlobalValue::ExternalLinkage);
  BasicBlock *B = BasicBlock::Create(F.getContext(), "", &F);
  IRBuilder<> IB(B);
  IB.CreateIntrinsic(Intrinsic::trap, {});
  IB.CreateUnreachable();
  return true;
}

static void dropMemoryFacts(Function &F) {
  // The pass adds loads and stores through argument 0, so anything -O2
  // inferred about the function's memory use or about that argument is stale.
  F.removeFnAttr(Attribute::Memory);
  F.removeFnAttr(Attribute::NoSync);
  for (auto K : {Attribute::ReadNone, Attribute::ReadOnly, Attribute::WriteOnly,
                 Attribute::Captures, Attribute::NoAlias, Attribute::Dereferenceable,
                 Attribute::DereferenceableOrNull, Attribute::NonNull})
    F.removeParamAttr(0, K);
}

// Integer argument registers a function type takes: a pointer or integer one,
// a [2 x i64] (JSValue on aarch64) two, a double none.
static unsigned intArgRegs(FunctionType *T) {
  unsigned n = 0;
  for (Type *P : T->params()) {
    if (P->isFloatingPointTy())
      continue;
    if (auto *A = dyn_cast<ArrayType>(P))
      n += A->getNumElements();
    else if (auto *S = dyn_cast<StructType>(P))
      n += S->getNumElements();
    else
      n += 1;
  }
  return n;
}

static bool unifiedFn(Function *F, const std::set<std::string> &leaves);

// Option B: every unified entry and every host callback uses preserve_none,
// whose only callee-saved registers are FP and LR (amd64: RSP and RBP). The
// register allocator then keeps across a call only what is live, in the
// frame, and no function saves registers for its caller, which matches Go's
// ABI0 where every register is clobbered. Register-entry leaves stay C.
// preserve_none places the 7th and later integer arguments in registers the
// engine reserves (x26 to x28 on aarch64), so the count is limited: packArgs
// has moved the arguments of every function over it into a block.
static void usePreserveNone(Module &M, const std::set<std::string> &leaves) {
  auto unified = [&](Function *F) { return unifiedFn(F, leaves); };
  for (Function &F : M) {
    if (!unified(&F) || F.use_empty() && F.isDeclaration())
      continue;
    if (MaxIntArgs && intArgRegs(F.getFunctionType()) > MaxIntArgs)
      report_fatal_error(Twine("preserve_none function with more than ") + Twine(MaxIntArgs) +
                         " integer argument registers: " + F.getName());
    F.setCallingConv(CallingConv::PreserveNone);
  }
  for (Function &F : M)
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I)) {
          Function *callee = CB->getCalledFunction();
          if (callee ? !unified(callee) : false)
            continue;
          if (!callee && MaxIntArgs && intArgRegs(CB->getFunctionType()) > MaxIntArgs)
            report_fatal_error(Twine("indirect preserve_none call with too many integer arguments in ") + F.getName());
          CB->setCallingConv(CallingConv::PreserveNone);
        }
}

// A unified function that needs more integer argument registers than
// preserve_none may use becomes f(sp, blk): the caller stores every argument
// after the shadow pointer in a block, 8-byte aligned in parameter order, and
// the callee loads them back at its entry. The block is an alloca in the
// caller, so the frame layout below puts it on the shadow stack, under the
// shadow pointer the callee receives. This runs before that layout.
static bool unifiedFn(Function *F, const std::set<std::string> &leaves) {
  return F && !F->isIntrinsic() && !leaves.count(F->getName().str()) && F->getName() != "__shadow_remaining";
}

// smallByval returns T, the type of a byval parameter, when it is a small
// aggregate C passes in memory because the argument registers ran out (a
// JSValue). A bigger one stays byval and the IR check after the pass
// rejects it.
static Type *smallByval(Type *T, const DataLayout &DL) {
  return T && DL.getTypeAllocSize(T).getFixedValue() <= 16 ? T : nullptr;
}

// needsBlock: the arguments after the shadow pointer go in a block, because
// there are more of them than the registers allow or one is a byval
// aggregate. Variadic functions are left alone: the IR check rejects them.
static bool needsBlock(FunctionType *T, function_ref<Type *(unsigned)> byval) {
  if (T->isVarArg() || T->getNumParams() < 1)
    return false;
  for (unsigned i = 0; i < T->getNumParams(); i++)
    if (byval(i))
      return true;
  return MaxIntArgs && intArgRegs(T) > MaxIntArgs;
}

// The block holds the arguments after the shadow pointer, each 8-byte
// aligned in parameter order: a byval aggregate as its contents.
static uint64_t blockLayout(FunctionType *T, const DataLayout &DL, function_ref<Type *(unsigned)> byval,
                            SmallVectorImpl<uint64_t> &offs) {
  uint64_t off = 0;
  for (unsigned i = 1; i < T->getNumParams(); i++) {
    off = alignTo(off, 8);
    offs.push_back(off);
    Type *bt = byval(i);
    off += DL.getTypeStoreSize(bt ? bt : T->getParamType(i)).getFixedValue();
  }
  return alignTo(off, 16);
}

static FunctionType *packedType(FunctionType *T) {
  PointerType *P = PointerType::getUnqual(T->getContext());
  return FunctionType::get(T->getReturnType(), {P, P}, false);
}

// Every packed call of a function uses one argument block, as large as the
// largest call needs: the stores go right before each call, after its
// arguments are computed, and the callee is done with them when it returns.
// One block per call site made JS_CallInternal's frame several KB.
using BlockMap = DenseMap<Function *, AllocaInst *>;

static AllocaInst *argBlock(Function *F, uint64_t size, BlockMap &blocks) {
  AllocaInst *&blk = blocks[F];
  Type *I8 = Type::getInt8Ty(F->getContext());
  if (blk && blk->getAllocationSize(F->getParent()->getDataLayout())->getFixedValue() >= size)
    return blk;
  IRBuilder<> E(&*F->getEntryBlock().getFirstInsertionPt());
  AllocaInst *N = E.CreateAlloca(ArrayType::get(I8, size), nullptr, "argblock");
  N->setAlignment(Align(16));
  if (blk) {
    blk->replaceAllUsesWith(N);
    blk->eraseFromParent();
  }
  blk = N;
  return N;
}

static void packCall(CallBase *CB, Value *callee, const DataLayout &DL, BlockMap &blocks) {
  auto *CI = dyn_cast<CallInst>(CB);
  if (!CI)
    report_fatal_error(Twine("argument block on a call that is not a plain call in ") + CB->getFunction()->getName());
  FunctionType *T = CB->getFunctionType();
  auto byval = [&](unsigned i) { return smallByval(CB->isByValArgument(i) ? CB->getParamByValType(i) : nullptr, DL); };
  SmallVector<uint64_t> offs;
  uint64_t size = blockLayout(T, DL, byval, offs);
  Function *F = CB->getFunction();
  LLVMContext &C = F->getContext();
  Type *I8 = Type::getInt8Ty(C);
  AllocaInst *blk = argBlock(F, size, blocks);
  IRBuilder<> B(CB);
  for (unsigned i = 1; i < CB->arg_size(); i++) {
    Value *dst = B.CreateConstGEP1_64(I8, blk, offs[i - 1]);
    if (Type *bt = byval(i))
      B.CreateMemCpy(dst, Align(8), CB->getArgOperand(i), CB->getParamAlign(i),
                     DL.getTypeStoreSize(bt).getFixedValue());
    else
      B.CreateAlignedStore(CB->getArgOperand(i), dst, Align(8));
  }
  CallInst *N = B.CreateCall(packedType(T), callee, {CB->getArgOperand(0), blk});
  N->setCallingConv(CB->getCallingConv());
  N->setAttributes(AttributeList::get(C, CB->getAttributes().getFnAttrs(), CB->getAttributes().getRetAttrs(), {}));
  N->setTailCallKind(CallInst::TCK_NoTail);
  CB->replaceAllUsesWith(N);
  CB->eraseFromParent();
}

static void packArgs(Module &M, const std::set<std::string> &leaves) {
  const DataLayout &DL = M.getDataLayout();
  BlockMap blocks;
  LLVMContext &C = M.getContext();
  Type *I8 = Type::getInt8Ty(C);
  SmallVector<Function *> over;
  for (Function &F : M) {
    auto byval = [&](unsigned i) { return smallByval(F.getParamByValType(i), DL); };
    if (unifiedFn(&F, leaves) && needsBlock(F.getFunctionType(), byval)) {
      if (F.isDeclaration() && !F.use_empty())
        report_fatal_error(Twine("host callback with more than ") + Twine(MaxIntArgs) +
                           " integer argument registers: " + F.getName());
      if (!F.isDeclaration())
        over.push_back(&F);
    }
  }
  for (Function *F : over) {
    FunctionType *T = F->getFunctionType();
    auto byval = [&](unsigned i) { return smallByval(F->getParamByValType(i), DL); };
    SmallVector<uint64_t> offs;
    blockLayout(T, DL, byval, offs);
    Function *NF = Function::Create(packedType(T), F->getLinkage(), F->getAddressSpace(), "", &M);
    NF->copyAttributesFrom(F);
    NF->setAttributes(AttributeList::get(C, F->getAttributes().getFnAttrs(), F->getAttributes().getRetAttrs(), {}));
    NF->takeName(F);
    NF->splice(NF->begin(), F);
    // F's own argument block, if its calls were packed already, moves with
    // the body.
    if (auto it = blocks.find(F); it != blocks.end()) {
      AllocaInst *blk = it->second;
      blocks.erase(it);
      blocks[NF] = blk;
    }
    IRBuilder<> B(&*NF->getEntryBlock().getFirstInsertionPt());
    F->getArg(0)->replaceAllUsesWith(NF->getArg(0));
    for (unsigned i = 1; i < T->getNumParams(); i++) {
      Value *at = B.CreateConstGEP1_64(I8, NF->getArg(1), offs[i - 1]);
      // A byval parameter is the address of the callee's own copy: the
      // copy in the block, which the caller made for this call.
      Value *v = byval(i) ? at : B.CreateAlignedLoad(T->getParamType(i), at, Align(8));
      F->getArg(i)->replaceAllUsesWith(v);
    }
    for (Use &U : make_early_inc_range(F->uses())) {
      auto *CB = dyn_cast<CallBase>(U.getUser());
      if (CB && CB->isCallee(&U))
        packCall(CB, NF, DL, blocks);
    }
    // The remaining uses are the address: in a function table, a compare or
    // an argument. Constants (tables) need the replacement through RAUW.
    F->replaceAllUsesWith(NF);
    F->eraseFromParent();
    errs() << "PACKED " << NF->getName() << "\n";
  }
  for (Function &F : M) {
    SmallVector<CallBase *> ind;
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I))
          if (!CB->getCalledFunction() && !CB->isInlineAsm() &&
              !isa<Function>(CB->getCalledOperand()->stripPointerCasts()) &&
              needsBlock(CB->getFunctionType(), [&](unsigned i) {
                return smallByval(CB->isByValArgument(i) ? CB->getParamByValType(i) : nullptr, DL);
              }))
            ind.push_back(CB);
    for (CallBase *CB : ind) {
      errs() << "PACKED-ICALL " << F.getName() << "\n";
      packCall(CB, CB->getCalledOperand(), DL, blocks);
    }
  }
}

// ---- Variable arguments (-shadow-varargs) ----
//
// A variadic call becomes a call with the fixed arguments and one more: the
// address of a block holding the variable ones, each in an 8-byte slot (or
// aligned to 16 when its type asks), as the stack part of a SysV or AAPCS64
// call would hold them. The block is a local whose address is taken, so it
// moves to the shadow stack like any other; one per function serves all its
// variadic calls. A variadic definition takes that
// address as its last parameter, and va_start sets the va_list so every
// va_arg reads from it: x86-64 marks the register areas used up (gp_offset
// 48, fp_offset 176) and points overflow_arg_area at the block, AArch64 sets
// gr_offs and vr_offs to 0 and points __stack at it. clang expands va_arg
// itself, so no other code changes. va_copy keeps working, since the
// va_list holds no address into the machine stack any more.
static FunctionType *fixedPartType(FunctionType *T) {
  SmallVector<Type *> ps(T->param_begin(), T->param_end());
  ps.push_back(PointerType::getUnqual(T->getContext()));
  return FunctionType::get(T->getReturnType(), ps, false);
}

static void lowerVarargs(Module &M) {
  bool arm = Triple(M.getTargetTriple()).isAArch64();
  const DataLayout &DL = M.getDataLayout();
  LLVMContext &C = M.getContext();
  Type *I8 = Type::getInt8Ty(C), *I32 = Type::getInt32Ty(C);
  PointerType *P = PointerType::getUnqual(C);
  // The slot offsets of the variable arguments of CB, false if one has no
  // 8-byte slot (byval, long double, vectors).
  // A variable argument is accepted only if it is a scalar that fits one
  // 8-byte, 8-aligned slot: an integer or pointer of at most 8 bytes, a float
  // or a double. That is the whole set the SysV/AAPCS overflow area holds
  // contiguously. Anything else (a byval aggregate, an i128, a vector, an
  // over-aligned value) is rejected, so the caller is not rewritten.
  auto slots = [&](CallBase *CB, SmallVector<uint64_t> &offs, uint64_t &size) {
    unsigned nf = CB->getFunctionType()->getNumParams();
    size = 0;
    if (!isa<CallInst>(CB))
      return false;
    for (unsigned i = nf; i < CB->arg_size(); i++) {
      Type *t = CB->getArgOperand(i)->getType();
      if (CB->isByValArgument(i) || !(t->isIntegerTy() || t->isPointerTy() || t->isDoubleTy() || t->isFloatTy()))
        return false;
      uint64_t sz = DL.getTypeStoreSize(t).getFixedValue();
      uint64_t al = DL.getABITypeAlign(t).value();
      if (sz > 8 || al > 8)
        return false;
      offs.push_back(size);
      size += 8;
    }
    return true;
  };
  // The block holds each variable argument in a contiguous 8-byte slot, which
  // matches the SysV/AAPCS overflow area only for the scalars slots() accepts.
  // Whether the reads keep to those slots shows only in the code that reads
  // the va_list: clang expands va_arg into loads and stores of the va_list's
  // fields and reads through its overflow pointer, which va_start points at
  // the block. So every use of a va_list, and of a pointer read from it, must
  // be one the pass can account for, and any other rejects the function that
  // makes it: a store of either pointer to memory, a va_copy into anything but
  // a local, a call the pass cannot follow (indirect, external, or in the
  // variadic part), pointer arithmetic it does not know, or an alignment of
  // the block pointer above 8 (an over-aligned aggregate, whose slot the
  // contiguous block cannot reproduce). A va_list handed to a function
  // defined in the module is followed into it.
  //
  // Kinds: a va_list (its storage, with the byte offset into it), the block
  // pointer (loaded from the overflow field) and a read-only pointer (the
  // register save area, which va_start leaves exhausted, or a merge of it
  // with the block pointer). Block and read pointers may be read through;
  // only a block pointer may be stored, and only back into the overflow field.
  const int64_t blockField = arm ? 0 : 8;
  auto readField = [&](int64_t off) { return arm ? (off == 8 || off == 16) : off == 16; };
  auto intField = [&](int64_t off) { return arm ? (off == 24 || off == 28) : (off == 0 || off == 4); };
  DenseMap<Value *, int64_t> vaList; // va_list pointer -> byte offset into it
  DenseMap<Value *, bool> ptrs;      // block or read pointer -> is a block pointer
  std::map<Function *, std::string> bad;
  auto fail = [&](Instruction *I, const Twine &why) {
    Function *F = I->getFunction();
    if (!bad.count(F))
      bad[F] = why.str();
  };
  SmallVector<Value *> work;
  auto addList = [&](Instruction *at, Value *V, int64_t off) {
    auto it = vaList.find(V);
    if (it == vaList.end()) {
      vaList[V] = off;
      work.push_back(V);
    } else if (it->second != off && at) {
      fail(at, "reaches a va_list at two offsets");
    }
  };
  auto addPtr = [&](Value *V) {
    if (!ptrs.count(V)) {
      ptrs[V] = true; // optimistic: a block pointer until shown otherwise
      work.push_back(V);
    }
  };
  auto constOff = [&](GEPOperator *G, int64_t &off) {
    APInt o(DL.getIndexTypeSizeInBits(G->getType()), 0);
    if (!G->accumulateConstantOffset(DL, o))
      return false;
    off = o.getSExtValue();
    return true;
  };
  for (Function &F : M)
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *II = dyn_cast<IntrinsicInst>(&I))
          if (II->getIntrinsicID() == Intrinsic::vastart || II->getIntrinsicID() == Intrinsic::vacopy) {
            Value *O = getUnderlyingObject(II->getArgOperand(0));
            if (!isa<AllocaInst>(O))
              fail(II, "starts or copies into a va_list that is not a local");
            else
              addList(nullptr, O, 0);
          }
  // Follow every derivation (the uses are checked once all kinds settle).
  while (!work.empty()) {
    Value *V = work.pop_back_val();
    bool list = vaList.count(V);
    int64_t off = list ? vaList[V] : 0;
    for (User *U : V->users()) {
      auto *I = dyn_cast<Instruction>(U);
      if (!I)
        continue;
      if (auto *G = dyn_cast<GEPOperator>(I)) {
        if (G->getPointerOperand() != V)
          continue;
        int64_t c;
        if (list) {
          if (constOff(G, c))
            addList(I, I, off + c);
        } else {
          addPtr(I); // a variable step is a read pointer (settled below)
        }
      } else if (auto *L = dyn_cast<LoadInst>(I)) {
        if (list && L->getType()->isPointerTy() && (off == blockField || readField(off)))
          addPtr(L);
      } else if (isa<PHINode>(I) || isa<SelectInst>(I)) {
        if (!list)
          addPtr(I);
      } else if (auto *II = dyn_cast<IntrinsicInst>(I)) {
        if (!list && II->getIntrinsicID() == Intrinsic::ptrmask && II->getArgOperand(0) == V)
          addPtr(II);
      } else if (auto *CB = dyn_cast<CallBase>(I)) {
        auto *callee = dyn_cast<Function>(CB->getCalledOperand()->stripPointerCasts());
        if (!list || off != 0 || !callee || callee->isDeclaration() || CB->getCalledOperand() == V)
          continue;
        for (unsigned i = 0; i < CB->arg_size() && i < CB->getFunctionType()->getNumParams() && i < callee->arg_size(); i++)
          if (CB->getArgOperand(i) == V)
            addList(I, callee->getArg(i), 0);
      }
    }
  }
  // Settle the kinds: a phi, select, step or mask is a block pointer only if
  // everything it is made of is one; anything else it merges makes it read-only.
  auto isBlock = [&](Value *V) {
    auto it = ptrs.find(V);
    return it != ptrs.end() && it->second;
  };
  for (bool changed = true; changed;) {
    changed = false;
    for (auto &[V, blk] : ptrs) {
      if (!blk)
        continue;
      bool now = true;
      if (auto *L = dyn_cast<LoadInst>(V)) {
        now = vaList.count(L->getPointerOperand()) && vaList[L->getPointerOperand()] == blockField;
      } else if (auto *P = dyn_cast<PHINode>(V)) {
        for (Value *in : P->incoming_values())
          now = now && isBlock(in);
      } else if (auto *S = dyn_cast<SelectInst>(V)) {
        now = isBlock(S->getTrueValue()) && isBlock(S->getFalseValue());
      } else if (auto *G = dyn_cast<GEPOperator>(V)) {
        int64_t c;
        now = isBlock(G->getPointerOperand()) && constOff(G, c);
      } else if (auto *II = dyn_cast<IntrinsicInst>(V)) {
        now = isBlock(II->getArgOperand(0));
      }
      if (!now) {
        blk = false;
        changed = true;
      }
    }
  }
  // Check every use.
  auto checkList = [&](Value *V, int64_t off, Use &U) {
    auto *I = cast<Instruction>(U.getUser());
    if (auto *G = dyn_cast<GEPOperator>(I)) {
      int64_t c;
      if (G->getPointerOperand() != V || !constOff(G, c))
        fail(I, "computes a variable address inside a va_list");
      return;
    }
    if (auto *L = dyn_cast<LoadInst>(I)) {
      bool ok = (intField(off) && L->getType()->isIntegerTy(32)) ||
                (L->getType()->isPointerTy() && (off == blockField || readField(off)));
      if (!ok)
        fail(I, "reads a va_list at an offset or type the pass does not know");
      return;
    }
    if (auto *S = dyn_cast<StoreInst>(I)) {
      if (S->getValueOperand() == V) {
        fail(I, "stores a va_list's address to memory, where the pass cannot follow it");
        return;
      }
      Value *v = S->getValueOperand();
      bool ok = (intField(off) && v->getType()->isIntegerTy(32)) || (off == blockField && isBlock(v));
      if (!ok)
        fail(I, "writes a va_list field with a value the pass cannot account for");
      return;
    }
    if (auto *II = dyn_cast<IntrinsicInst>(I)) {
      switch (II->getIntrinsicID()) {
      case Intrinsic::vastart:
      case Intrinsic::vaend:
      case Intrinsic::lifetime_start:
      case Intrinsic::lifetime_end:
        if (off != 0)
          fail(I, "passes an address inside a va_list to an intrinsic");
        return;
      case Intrinsic::vacopy: {
        // Both operands must be va_lists the pass tracks: the destination
        // was seeded from a local, the source must be a tracked list.
        Value *src = II->getArgOperand(1);
        if (off != 0 || !vaList.count(src) || vaList[src] != 0)
          fail(I, "copies a va_list the pass cannot account for");
        return;
      }
      default:
        fail(I, "hands a va_list to an intrinsic the pass cannot follow");
        return;
      }
    }
    if (auto *CB = dyn_cast<CallBase>(I)) {
      auto *callee = dyn_cast<Function>(CB->getCalledOperand()->stripPointerCasts());
      unsigned i = U.getOperandNo();
      if (off != 0 || !callee || callee->isDeclaration() || CB->isCallee(&U) ||
          i >= CB->getFunctionType()->getNumParams() || i >= callee->arg_size())
        fail(I, "hands a va_list to a function the pass cannot follow (indirect, external or variadic)");
      return;
    }
    fail(I, "uses a va_list in a way the pass cannot follow");
  };
  auto checkPtr = [&](Value *V, bool blk, Use &U) {
    auto *I = cast<Instruction>(U.getUser());
    if (auto *G = dyn_cast<GEPOperator>(I)) {
      int64_t c;
      if (G->getPointerOperand() != V || (blk && !constOff(G, c)))
        fail(I, "computes a variable address from the variadic block pointer");
      return;
    }
    if (isa<LoadInst>(I) || isa<PHINode>(I) || isa<SelectInst>(I))
      return;
    if (auto *S = dyn_cast<StoreInst>(I)) {
      if (S->getPointerOperand() == V) {
        fail(I, "writes through a pointer read from a va_list");
        return;
      }
      Value *to = S->getPointerOperand();
      if (!(blk && vaList.count(to) && vaList[to] == blockField))
        fail(I, "stores a pointer into the variadic block to memory, where the pass cannot follow it");
      return;
    }
    if (auto *II = dyn_cast<IntrinsicInst>(I)) {
      switch (II->getIntrinsicID()) {
      case Intrinsic::ptrmask: {
        auto *m = dyn_cast<ConstantInt>(II->getArgOperand(1));
        if (II->getArgOperand(0) != V || !m || (~m->getValue()).ugt(7))
          fail(I, "reads an over-aligned variadic argument (above 8 bytes), which the 8-byte-slot block cannot hold");
        return;
      }
      case Intrinsic::memcpy:
      case Intrinsic::memmove:
        if (II->getArgOperand(1) == V && II->getArgOperand(0) != V)
          return;
        fail(I, "writes through a pointer read from a va_list");
        return;
      default:
        fail(I, "hands a pointer read from a va_list to an intrinsic the pass cannot follow");
        return;
      }
    }
    fail(I, "uses a pointer read from a va_list in a way the pass cannot follow");
  };
  for (auto &[V, off] : vaList)
    for (Use &U : V->uses())
      if (isa<Instruction>(U.getUser()))
        checkList(V, off, U);
  for (auto &[V, blk] : ptrs)
    for (Use &U : V->uses())
      if (isa<Instruction>(U.getUser()))
        checkPtr(V, blk, U);
  for (auto &[F, why] : bad)
    reject(*F, why);
  SmallVector<Function *> fns;
  for (Function &F : M)
    fns.push_back(&F);
  for (Function *F : fns) {
    if (bad.count(F))
      continue;
    SmallVector<CallBase *> calls;
    for (BasicBlock &B : *F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I))
          if (!CB->isInlineAsm() && CB->getFunctionType()->isVarArg() && !isa<IntrinsicInst>(CB) &&
              !bad.count(dyn_cast<Function>(CB->getCalledOperand()->stripPointerCasts())))
            calls.push_back(CB);
    if (calls.empty())
      continue;
    // One block serves every call of F: the stores go right before each
    // call, after its arguments are computed.
    bool ok = true;
    uint64_t most = 8;
    for (CallBase *CB : calls) {
      SmallVector<uint64_t> offs;
      uint64_t size;
      ok = ok && slots(CB, offs, size);
      most = std::max(most, size);
    }
    if (!ok) {
      reject(*F, "variadic call with an argument that has no 8-byte slot");
      continue;
    }
    IRBuilder<> E(&*F->getEntryBlock().getFirstInsertionPt());
    AllocaInst *blk = E.CreateAlloca(ArrayType::get(I8, most), nullptr, "va_block");
    blk->setAlignment(Align(16));
    for (CallBase *CB : calls) {
      FunctionType *T = CB->getFunctionType();
      unsigned nf = T->getNumParams();
      SmallVector<uint64_t> offs;
      uint64_t size;
      slots(CB, offs, size);
      IRBuilder<> B(CB);
      SmallVector<Value *> args(CB->arg_begin(), CB->arg_begin() + nf);
      for (unsigned i = nf; i < CB->arg_size(); i++)
        B.CreateAlignedStore(CB->getArgOperand(i), B.CreateConstGEP1_64(I8, blk, offs[i - nf]), Align(8));
      args.push_back(blk);
      CallInst *N = B.CreateCall(fixedPartType(T), CB->getCalledOperand(), args);
      N->setCallingConv(CB->getCallingConv());
      AttributeList A = CB->getAttributes();
      SmallVector<AttributeSet> pa;
      for (unsigned i = 0; i < nf; i++)
        pa.push_back(A.getParamAttrs(i));
      pa.push_back(AttributeSet());
      N->setAttributes(AttributeList::get(C, A.getFnAttrs(), A.getRetAttrs(), pa));
      N->setTailCallKind(CallInst::TCK_NoTail);
      N->copyMetadata(*CB);
      N->takeName(CB);
      CB->replaceAllUsesWith(N);
      CB->eraseFromParent();
    }
  }
  SmallVector<Function *> defs;
  for (Function &F : M)
    if (F.isVarArg() && !F.isIntrinsic())
      defs.push_back(&F);
  for (Function *F : defs) {
    if (bad.count(F))
      continue; // already emptied to a trap; leave its variadic signature
    FunctionType *T = F->getFunctionType();
    if (F->isDeclaration()) {
      if (!F->use_empty())
        report_fatal_error(Twine("variadic function defined outside the module: ") + F->getName());
      F->eraseFromParent();
      continue;
    }
    Function *NF = Function::Create(fixedPartType(T), F->getLinkage(), F->getAddressSpace(), "", &M);
    NF->copyAttributesFrom(F);
    AttributeList A = F->getAttributes();
    SmallVector<AttributeSet> pa;
    for (unsigned i = 0; i < T->getNumParams(); i++)
      pa.push_back(A.getParamAttrs(i));
    pa.push_back(AttributeSet());
    NF->setAttributes(AttributeList::get(C, A.getFnAttrs(), A.getRetAttrs(), pa));
    NF->copyMetadata(F, 0);
    NF->takeName(F);
    NF->splice(NF->begin(), F);
    for (unsigned i = 0; i < T->getNumParams(); i++) {
      NF->getArg(i)->takeName(F->getArg(i));
      F->getArg(i)->replaceAllUsesWith(NF->getArg(i));
    }
    Argument *va = NF->getArg(T->getNumParams());
    va->setName("va");
    SmallVector<IntrinsicInst *> starts, ends;
    for (BasicBlock &B : *NF)
      for (Instruction &I : B)
        if (auto *II = dyn_cast<IntrinsicInst>(&I)) {
          if (II->getIntrinsicID() == Intrinsic::vastart)
            starts.push_back(II);
          else if (II->getIntrinsicID() == Intrinsic::vaend)
            ends.push_back(II);
        }
    for (IntrinsicInst *II : starts) {
      IRBuilder<> B(II);
      Value *ap = II->getArgOperand(0);
      auto at = [&](uint64_t off) { return B.CreateConstGEP1_64(I8, ap, off); };
      if (arm) {
        B.CreateAlignedStore(va, at(0), Align(8));
        B.CreateAlignedStore(ConstantPointerNull::get(P), at(8), Align(8));
        B.CreateAlignedStore(ConstantPointerNull::get(P), at(16), Align(8));
        B.CreateAlignedStore(ConstantInt::get(I32, 0), at(24), Align(4));
        B.CreateAlignedStore(ConstantInt::get(I32, 0), at(28), Align(4));
      } else {
        B.CreateAlignedStore(ConstantInt::get(I32, 48), at(0), Align(4));
        B.CreateAlignedStore(ConstantInt::get(I32, 176), at(4), Align(4));
        B.CreateAlignedStore(va, at(8), Align(8));
        B.CreateAlignedStore(ConstantPointerNull::get(P), at(16), Align(8));
      }
      II->eraseFromParent();
    }
    for (IntrinsicInst *II : ends)
      II->eraseFromParent();
    F->replaceAllUsesWith(NF);
    F->eraseFromParent();
    errs() << "VARARGS " << NF->getName() << "\n";
  }
  for (Function &F : make_early_inc_range(M))
    if ((F.getIntrinsicID() == Intrinsic::vastart || F.getIntrinsicID() == Intrinsic::vaend) && F.use_empty())
      F.eraseFromParent();
}

// ---- Shadow pointer insertion (-shadow-add-sp) ----

// rebuildWithSP replaces a call by one that passes a placeholder shadow
// pointer first; the frame layout sets the real one.
static CallInst *rebuildWithSP(CallBase *CB, Value *callee) {
  auto *CI = dyn_cast<CallInst>(CB);
  if (!CI)
    report_fatal_error(Twine("call that is not a plain call in ") + CB->getFunction()->getName());
  LLVMContext &C = CB->getContext();
  PointerType *P = PointerType::getUnqual(C);
  FunctionType *T = CB->getFunctionType();
  SmallVector<Type *> ps{P};
  ps.append(T->param_begin(), T->param_end());
  FunctionType *NT = FunctionType::get(T->getReturnType(), ps, T->isVarArg());
  SmallVector<Value *> args{PoisonValue::get(P)};
  args.append(CB->arg_begin(), CB->arg_end());
  IRBuilder<> B(CB);
  CallInst *N = B.CreateCall(NT, callee, args);
  N->setCallingConv(CB->getCallingConv());
  AttributeList A = CB->getAttributes();
  SmallVector<AttributeSet> pa{AttributeSet()};
  for (unsigned i = 0; i < CB->arg_size(); i++)
    pa.push_back(A.getParamAttrs(i));
  N->setAttributes(AttributeList::get(C, A.getFnAttrs(), A.getRetAttrs(), pa));
  N->setTailCallKind(CallInst::TCK_NoTail);
  N->copyMetadata(*CB);
  N->takeName(CB);
  CB->replaceAllUsesWith(N);
  CB->eraseFromParent();
  return N;
}

static void addShadowPointer(Module &M, const std::set<std::string> &leaves) {
  LLVMContext &C = M.getContext();
  PointerType *P = PointerType::getUnqual(C);
  auto needsSP = [&](Function *F) { return unifiedFn(F, leaves); };
  // Calls through a pointer, and direct calls whose type differs from the
  // callee's, take the new argument by their own type.
  SmallPtrSet<CallBase *, 32> done;
  SmallVector<CallBase *> ind;
  for (Function &F : M)
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I))
          if (!CB->isInlineAsm() && !CB->getCalledFunction()) {
            auto *Fn = dyn_cast<Function>(CB->getCalledOperand()->stripPointerCasts());
            if (Fn && !needsSP(Fn))
              continue;
            if (Fn)
              errs() << "MISMATCH " << F.getName() << " " << Fn->getName() << "\n";
            ind.push_back(CB);
          }
  for (CallBase *CB : ind)
    done.insert(rebuildWithSP(CB, CB->getCalledOperand()));
  SmallVector<Function *> fs;
  for (Function &F : M)
    if (needsSP(&F))
      fs.push_back(&F);
  for (Function *F : fs) {
    if (F->use_empty() && F->isDeclaration()) {
      F->eraseFromParent();
      continue;
    }
    FunctionType *T = F->getFunctionType();
    SmallVector<Type *> ps{P};
    ps.append(T->param_begin(), T->param_end());
    FunctionType *NT = FunctionType::get(T->getReturnType(), ps, T->isVarArg());
    Function *NF = Function::Create(NT, F->getLinkage(), F->getAddressSpace(), "", &M);
    NF->copyAttributesFrom(F);
    AttributeList A = F->getAttributes();
    SmallVector<AttributeSet> pa{AttributeSet()};
    for (unsigned i = 0; i < T->getNumParams(); i++)
      pa.push_back(A.getParamAttrs(i));
    NF->setAttributes(AttributeList::get(C, A.getFnAttrs(), A.getRetAttrs(), pa));
    NF->copyMetadata(F, 0);
    NF->takeName(F);
    if (!F->isDeclaration()) {
      NF->splice(NF->begin(), F);
      for (unsigned i = 0; i < T->getNumParams(); i++) {
        NF->getArg(i + 1)->takeName(F->getArg(i));
        F->getArg(i)->replaceAllUsesWith(NF->getArg(i + 1));
      }
    }
    NF->getArg(0)->setName("sp");
    for (Use &U : make_early_inc_range(F->uses())) {
      auto *CB = dyn_cast<CallBase>(U.getUser());
      if (CB && CB->isCallee(&U) && !done.count(CB) && CB->getFunctionType() == T)
        rebuildWithSP(CB, NF);
    }
    F->replaceAllUsesWith(NF);
    F->eraseFromParent();
  }
}

// Calls inside unified code use one convention: C (fastcc, which LLVM's
// interprocedural passes gave internal functions, becomes C again) or, with
// -shadow-cc=preserve_none, preserve_none after this.
static void plainC(Module &M) {
  for (Function &F : M) {
    if (F.getCallingConv() == CallingConv::Fast)
      F.setCallingConv(CallingConv::C);
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I))
          if (CB->getCallingConv() == CallingConv::Fast)
            CB->setCallingConv(CallingConv::C);
  }
}

// A computed goto table (a constant array of the label addresses of one
// function) becomes 32-bit offsets of each label from the table, and every
// dispatch adds the loaded offset to the table address: no relocation is
// left, so the generator places the table inside its function.
static void relativeGotoTables(Module &M) {
  LLVMContext &C = M.getContext();
  Type *I32 = Type::getInt32Ty(C), *I64 = Type::getInt64Ty(C), *I8 = Type::getInt8Ty(C);
  SmallVector<GlobalVariable *> tables;
  for (GlobalVariable &G : M.globals()) {
    auto *A = G.hasInitializer() ? dyn_cast<ConstantArray>(G.getInitializer()) : nullptr;
    if (!A || A->getNumOperands() == 0)
      continue;
    bool labels = true;
    for (auto &E : A->operands())
      labels &= isa<BlockAddress>(E);
    if (labels)
      tables.push_back(&G);
  }
  for (GlobalVariable *G : tables) {
    auto *A = cast<ConstantArray>(G->getInitializer());
    Function *F = cast<BlockAddress>(A->getOperand(0))->getFunction();
    auto *T = ArrayType::get(I32, A->getNumOperands());
    auto *R = new GlobalVariable(M, T, true, GlobalValue::InternalLinkage, nullptr, G->getName() + ".rel");
    R->setAlignment(Align(4));
    Constant *base = ConstantExpr::getPtrToInt(R, I64);
    SmallVector<Constant *> offsets;
    for (auto &E : A->operands()) {
      if (cast<BlockAddress>(E)->getFunction() != F)
        report_fatal_error(Twine("computed goto table with labels of two functions: ") + G->getName());
      offsets.push_back(ConstantExpr::getTrunc(ConstantExpr::getSub(ConstantExpr::getPtrToInt(cast<Constant>(E), I64), base), I32));
    }
    R->setInitializer(ConstantArray::get(T, offsets));
    for (User *U : make_early_inc_range(G->users())) {
      auto *P = dyn_cast<GetElementPtrInst>(U);
      if (!P || P->getNumIndices() != 1 || P->getFunction() != F)
        report_fatal_error(Twine("unreviewed use of the computed goto table ") + G->getName());
      for (User *L : make_early_inc_range(P->users())) {
        auto *Load = dyn_cast<LoadInst>(L);
        if (!Load || Load->getPointerOperand() != P)
          report_fatal_error(Twine("unreviewed use of a computed goto entry in ") + F->getName());
        IRBuilder<> B(Load);
        Value *index = B.CreateSExtOrTrunc(P->getOperand(P->getNumOperands() - 1), I64);
        Value *offset = B.CreateLoad(I32, B.CreateInBoundsGEP(I32, R, index));
        // Not inbounds: the label lies outside the table.
        Load->replaceAllUsesWith(B.CreateGEP(I8, R, B.CreateSExt(offset, I64)));
        Load->eraseFromParent();
      }
      P->eraseFromParent();
    }
    if (!G->use_empty())
      report_fatal_error(Twine("computed goto table still used: ") + G->getName());
    G->eraseFromParent();
    errs() << "GOTO-TABLE " << F->getName() << "\n";
  }
}

// sigCode names a function type the way the generator reads it: one letter
// per argument word (i integer or pointer, d double, f float, j a pair of
// 64-bit integers), '>' and the result (v void, i, d, f, j), '?' for a type
// it cannot carry, "..." for variadic.
static std::string sigLetter(Type *t) {
  if (t->isVoidTy())
    return "v";
  if (t->isPointerTy())
    return "i";
  if (t->isIntegerTy())
    return t->getIntegerBitWidth() <= 64 ? "i" : "?";
  if (t->isDoubleTy())
    return "d";
  if (t->isFloatTy())
    return "f";
  auto pair = [](Type *a, Type *b) { return a->isIntegerTy(64) && b->isIntegerTy(64); };
  if (auto *S = dyn_cast<StructType>(t))
    if (S->getNumElements() == 2 && pair(S->getElementType(0), S->getElementType(1)))
      return "j";
  if (auto *A = dyn_cast<ArrayType>(t))
    if (A->getNumElements() == 2 && A->getElementType()->isIntegerTy(64))
      return "j";
  return "?";
}

static std::string sigCode(FunctionType *T, function_ref<bool(unsigned)> byval) {
  std::string s;
  for (unsigned i = 0; i < T->getNumParams(); i++)
    s += byval(i) ? "B" : sigLetter(T->getParamType(i));
  if (T->isVarArg())
    s += "...";
  return s + ">" + sigLetter(T->getReturnType());
}

// icallID encodes a signature code as the 32-bit type id of a "kcfi"
// bundle: bit 31 set, the parameter count in bits 27 to 30, the result in
// bits 24 to 26 (v i d f j) and two bits per parameter from bit 0 (i d f j).
// A code it cannot hold (more than 12 parameters, byval, variadic or an
// unknown type) gets icallUnencodable, which the generator rejects. The Go
// side is ICallID in icall.go.
static const uint32_t icallUnencodable = 0xffffffffu;

static uint32_t icallID(const std::string &code) {
  size_t gt = code.find('>');
  if (gt == std::string::npos || gt > 12)
    return icallUnencodable;
  const std::string params = "idfj", results = "vidfj";
  size_t r = code.size() == gt + 2 ? results.find(code[gt + 1]) : std::string::npos;
  if (r == std::string::npos)
    return icallUnencodable;
  uint32_t id = 1u << 31 | uint32_t(gt) << 27 | uint32_t(r) << 24;
  for (size_t i = 0; i < gt; i++) {
    size_t p = params.find(code[i]);
    if (p == std::string::npos)
      return icallUnencodable;
    id |= uint32_t(p) << (2 * i);
  }
  return id;
}

// bindSignatures gives every indirect call a "kcfi" bundle with its
// signature's id. A call through a pointer to a known function is a direct
// call to llc and gets none.
static void bindSignatures(Module &M) {
  Type *I32 = Type::getInt32Ty(M.getContext());
  for (Function &F : M) {
    SmallVector<CallBase *> ind;
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I))
          if (!CB->isInlineAsm() && !isa<Function>(CB->getCalledOperand()->stripPointerCasts()))
            ind.push_back(CB);
    for (CallBase *CB : ind) {
      if (CB->getOperandBundle(LLVMContext::OB_kcfi))
        report_fatal_error(Twine("indirect call with a kcfi bundle already in ") + F.getName());
      std::string code = sigCode(CB->getFunctionType(), [&](unsigned i) { return CB->isByValArgument(i); });
      OperandBundleDef kcfi("kcfi", ConstantInt::get(I32, icallID(code)));
      CallBase *N = CallBase::addOperandBundle(CB, LLVMContext::OB_kcfi, kcfi, CB->getIterator());
      N->takeName(CB);
      CB->replaceAllUsesWith(N);
      CB->eraseFromParent();
    }
  }
}

static void report(Module &M, const std::set<std::string> &leaves) {
  for (Function &F : M) {
    if (F.isIntrinsic() || (F.isDeclaration() && F.use_empty()))
      continue;
    std::string code = sigCode(F.getFunctionType(), [&](unsigned i) { return F.getParamByValType(i) != nullptr; });
    errs() << "SIG " << F.getName() << " " << code << " " << (F.isDeclaration() ? "decl" : "def") << " "
           << (leaves.count(F.getName().str()) ? "leaf" : "unified") << "\n";
    bool addr = false;
    for (const Use &U : F.uses()) {
      auto *CB = dyn_cast<CallBase>(U.getUser());
      if (!CB || !CB->isCallee(&U))
        addr = true;
    }
    if (addr)
      errs() << "ADDR " << F.getName() << "\n";
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I)) {
          if (CB->isInlineAsm())
            continue;
          if (auto *Fn = dyn_cast<Function>(CB->getCalledOperand()->stripPointerCasts())) {
            if (Fn->getIntrinsicID() == Intrinsic::frameaddress)
              errs() << "FRAMEADDR " << F.getName() << "\n";
            if (!Fn->isIntrinsic())
              errs() << "CALL " << F.getName() << " " << Fn->getName() << "\n";
          } else {
            std::string code = sigCode(CB->getFunctionType(), [&](unsigned i) { return CB->isByValArgument(i); });
            errs() << "ICALL " << F.getName() << " " << code << "\n";
          }
        }
  }
}

struct ShadowStack : PassInfoMixin<ShadowStack> {
  PreservedAnalyses run(Module &M, ModuleAnalysisManager &) {
    std::set<std::string> leaves(Leaves.begin(), Leaves.end());
    const DataLayout &DL = M.getDataLayout();
    LLVMContext &C = M.getContext();
    Type *I64 = Type::getInt64Ty(C), *I8 = Type::getInt8Ty(C);
    PointerType *P = PointerType::getUnqual(C);
    const uint64_t S = 1ull << SizeLog2;
    relativeGotoTables(M);
    if (Varargs)
      lowerVarargs(M);
    if (AddSP) {
      addShadowPointer(M, leaves);
      plainC(M);
    }
    if (MaxIntArgs || AddSP)
      packArgs(M, leaves);
    for (Function &F : M) {
      if (F.isDeclaration())
        continue;
      if (leaves.count(F.getName().str())) {
        // A register-entry leaf takes no shadow pointer and may own no
        // memory: it must not have a local whose address is taken. It may
        // call only other leaves, and it has no frame of its own. Its
        // address must not be taken: an indirect call reaching it would pass
        // a shadow pointer the leaf does not expect (plan.md: every
        // address-taken function goes through the unified entry, and a leaf
        // has none). The backend's own libcalls to it are direct calls that
        // llc adds after this pass, so they are not a use here.
        for (const Use &U : F.uses()) {
          auto *CB = dyn_cast<CallBase>(U.getUser());
          if (!CB || !CB->isCallee(&U))
            report_fatal_error(Twine("register-entry leaf whose address is taken (it has no unified entry for an indirect call): ") + F.getName());
        }
        for (BasicBlock &B : F)
          for (Instruction &I : B) {
            if (isa<AllocaInst>(&I))
              report_fatal_error(Twine("register-entry leaf with an addressable local: ") + F.getName());
            if (auto *CB = dyn_cast<CallBase>(&I)) {
              auto *Fn = dyn_cast<Function>(CB->getCalledOperand()->stripPointerCasts());
              if (!Fn || !(Fn->isIntrinsic() || leaves.count(Fn->getName().str())))
                report_fatal_error(Twine("register-entry leaf calls a unified function: ") + F.getName());
            }
          }
        // "reserved": no frame is set up, and llc never allocates RBP,
        // which Go keeps as its frame pointer.
        if (AddSP)
          F.addFnAttr("frame-pointer", "reserved");
        continue;
      }
      if (F.arg_size() == 0 || !F.getArg(0)->getType()->isPointerTy())
        report_fatal_error(Twine("function without a shadow-stack pointer: ") + F.getName());
      dropMemoryFacts(F);
      // The Go frame cannot be realigned: LLVM must use the stack alignment
      // the target is told (8 on amd64) and spill with unaligned moves.
      F.addFnAttr("no-realign-stack");
      Argument *ss = F.getArg(0);

      bool rejected = false;
      SmallVector<AllocaInst *> fixed, dynamic;
      SmallVector<IntrinsicInst *> lifetimes, saves, restores;
      SmallVector<CallBase *> calls;
      for (BasicBlock &B : F)
        for (Instruction &I : B) {
          if (auto *A = dyn_cast<AllocaInst>(&I)) {
            if (A->isStaticAlloca())
              fixed.push_back(A);
            else
              dynamic.push_back(A);
          } else if (auto *II = dyn_cast<IntrinsicInst>(&I)) {
            switch (II->getIntrinsicID()) {
            case Intrinsic::lifetime_start:
            case Intrinsic::lifetime_end:
              lifetimes.push_back(II);
              break;
            case Intrinsic::stacksave:
              saves.push_back(II);
              break;
            case Intrinsic::stackrestore:
              restores.push_back(II);
              break;
            default:
              break;
            }
          } else if (auto *CB = dyn_cast<CallBase>(&I)) {
            if (CB->isInlineAsm())
              rejected = true;
            calls.push_back(CB);
          }
        }
      if (rejected && reject(F, "inline assembly"))
        continue;
      for (auto *II : lifetimes)
        II->eraseFromParent();

      const bool dyn = !dynamic.empty() || !saves.empty();
      uint64_t off = dyn ? 16 : 0; // word 0 holds the running top
      SmallVector<std::pair<AllocaInst *, uint64_t>> placed;
      for (auto *A : fixed) {
        uint64_t al = A->getAlign().value();
        if (al > 16)
          rejected = true;
        off = alignTo(off, al);
        placed.push_back({A, off});
        off += A->getAllocationSize(DL)->getFixedValue();
      }
      if (rejected && reject(F, "local aligned beyond 16"))
        continue;
      const uint64_t frame = alignTo(off, 16);

      IRBuilder<> E(&*F.getEntryBlock().getFirstInsertionPt());
      if (dyn)
        E.CreateStore(E.CreateConstGEP1_64(I8, ss, frame), ss);
      // The first free byte above everything live at a point.
      auto top = [&](IRBuilder<> &B) -> Value * {
        if (dyn)
          return B.CreateLoad(P, ss, "shadow.top");
        return frame ? B.CreateConstGEP1_64(I8, ss, frame) : ss;
      };

      // Each use computes the address from argument 0 where it needs it, so
      // no derived pointer has to survive a call in a register.
      for (auto [A, o] : placed) {
        for (Use &U : make_early_inc_range(A->uses())) {
          auto *I = cast<Instruction>(U.getUser());
          Instruction *at = I;
          if (auto *Phi = dyn_cast<PHINode>(I))
            at = Phi->getIncomingBlock(U)->getTerminator();
          IRBuilder<> B(at);
          U.set(B.CreateConstGEP1_64(I8, ss, o));
        }
        A->eraseFromParent();
      }

      for (auto *A : dynamic) {
        IRBuilder<> B(A);
        Value *cur = B.CreateLoad(P, ss, "shadow.top");
        Value *ci = B.CreatePtrToInt(cur, I64);
        Value *start = B.CreateAnd(B.CreateAdd(ci, B.getInt64(15)), B.getInt64(~15ull));
        Value *n = B.CreateMul(B.CreateZExtOrTrunc(A->getArraySize(), I64),
                               B.getInt64(DL.getTypeAllocSize(A->getAllocatedType()).getFixedValue()));
        n = B.CreateAnd(B.CreateAdd(n, B.getInt64(15)), B.getInt64(~15ull));
        Value *end = B.CreateAdd(start, n);
        Value *base = B.CreateAnd(B.CreatePtrToInt(ss, I64), B.getInt64(~(S - 1)));
        Value *limit = B.CreateAdd(base, B.getInt64(S - Guard));
        // end < start catches a size that wrapped around.
        Value *bad = B.CreateOr(B.CreateICmpUGT(end, limit), B.CreateICmpULT(end, start));
        Instruction *trap = SplitBlockAndInsertIfThen(bad, A, true);
        IRBuilder<> T(trap);
        T.CreateIntrinsic(Intrinsic::trap, {});
        B.SetInsertPoint(A);
        Value *p = B.CreateGEP(I8, cur, B.CreateSub(start, ci));
        B.CreateStore(B.CreateGEP(I8, cur, B.CreateSub(end, ci)), ss);
        A->replaceAllUsesWith(p);
        A->eraseFromParent();
      }
      for (auto *II : saves) {
        IRBuilder<> B(II);
        II->replaceAllUsesWith(top(B));
        II->eraseFromParent();
      }
      for (auto *II : restores) {
        IRBuilder<> B(II);
        B.CreateStore(II->getArgOperand(0), ss);
        II->eraseFromParent();
      }

      for (auto *CB : calls) {
        Function *callee = CB->getCalledFunction();
        if (callee && callee->isIntrinsic())
          continue;
        IRBuilder<> B(CB);
        if (callee && callee->getName() == "__shadow_remaining") {
          Value *used = B.CreateAnd(B.CreatePtrToInt(top(B), I64), B.getInt64(S - 1));
          CB->replaceAllUsesWith(B.CreateSub(B.getInt64(S - Guard), used));
          CB->eraseFromParent();
          continue;
        }
        if (callee && leaves.count(callee->getName().str()))
          continue;
        if (CB->arg_size() == 0 || !CB->getArgOperand(0)->getType()->isPointerTy())
          report_fatal_error(Twine("call without a shadow-stack pointer in ") + F.getName());
        CB->removeFnAttr(Attribute::Memory);
        for (auto K : {Attribute::ReadNone, Attribute::ReadOnly, Attribute::WriteOnly,
                       Attribute::Captures, Attribute::NoAlias})
          CB->removeParamAttr(0, K);
        CB->setArgOperand(0, top(B));
      }
      errs() << "SHADOW " << F.getName() << " " << frame << (dyn ? " dynamic" : "") << "\n";
    }
    for (Function &F : M)
      for (BasicBlock &B : F)
        for (Instruction &I : B)
          if (isa<AllocaInst>(&I))
            report_fatal_error(Twine("alloca left on the Go stack in ") + F.getName());
    if (CallConv == "preserve_none")
      usePreserveNone(M, leaves);
    else if (!CallConv.empty())
      report_fatal_error(Twine("unknown -shadow-cc ") + CallConv);
    if (Function *R = M.getFunction("__shadow_remaining"))
      if (R->use_empty())
        R->eraseFromParent();
    if (Report)
      report(M, leaves);
    if (ICallSigs)
      bindSignatures(M);
    return PreservedAnalyses::none();
  }
};

extern "C" LLVM_ATTRIBUTE_WEAK PassPluginLibraryInfo llvmGetPassPluginInfo() {
  return {LLVM_PLUGIN_API_VERSION, "goasm-shadow-stack", "22.1.8", [](PassBuilder &PB) {
            PB.registerPipelineParsingCallback(
                [](StringRef name, ModulePassManager &PM, ArrayRef<PassBuilder::PipelineElement>) {
                  if (name != "goasm-shadow-stack")
                    return false;
                  PM.addPass(ShadowStack());
                  return true;
                });
          }};
}
