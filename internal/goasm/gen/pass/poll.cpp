// Copyright (c) 2026 The slowjs Authors.
// SPDX-License-Identifier: MIT
//
// Polls (-shadow-poll). The generated code is Go assembly, which the Go
// runtime never stops asynchronously: the garbage collector and the
// scheduler wait for a goroutine in the engine until it enters a function
// that checks the stack. A loop of C that calls no such function would keep
// them waiting for as long as it runs. So every cycle of every engine
// function counts down a counter, a tick, and when the counter runs out the
// code calls host_poll, a Go function whose stack check is where the runtime
// stops the goroutine. host_poll also sets the counter again, to the
// interval of the VM, and is where the host sees an interrupt.
//
// The counter is a 64-bit word at a fixed offset (-shadow-poll-offset) from
// the base of the shadow-stack region, which the host keeps for it: the
// region is aligned to its size, so the base is the shadow pointer with its
// low bits cleared. Loops keep it in a register where that pays:
//
//   - Each outermost loop with no call that may tick loads the word into a
//     local at its preheader and stores it back at its exits, so its ticks
//     are a decrement of a register. A loop with such a call counts in
//     memory, as the call does, and its inner loops are taken in turn: a
//     local there would be written back and read again around every call.
//   - A function with a dispatch keeps the word in a local from its entry to
//     its returns, and stores and loads it around every call that may tick.
//   - Ticks on cycles outside these count in memory.
//
// Every retreating edge of a depth-first walk gets a tick, since every cycle
// has one, but those of a loop that scalar evolution proves short
// (boundedEdges): whatever enters such a loop again ticks. An innermost loop
// that calls nothing and is not proved short runs in chunks (stripLoops),
// which makes it short, and the end of each chunk ticks for the chunk.
// Calls of memcpy, memmove and memset, which have no ticks, go through
// helpers that do at most LeafChunk bytes at a time (chunkLeafCalls). The
// dispatch of a computed goto (the interpreter's) counts itself instead: an
// expired count sends the indirect branch to a poll label at the end of its
// table, which polls and dispatches again, so the dispatch stays one block
// that llc copies into the end of every handler.
//
// checkPolls verifies the result on its own: removing the blocks that tick or
// poll from each function must leave no cycle.
#include "llvm/ADT/DenseMap.h"
#include "llvm/ADT/SCCIterator.h"
#include "llvm/Analysis/AssumptionCache.h"
#include "llvm/Analysis/LoopInfo.h"
#include "llvm/Analysis/ScalarEvolution.h"
#include "llvm/Analysis/ScalarEvolutionExpressions.h"
#include "llvm/Analysis/TargetLibraryInfo.h"
#include "llvm/IR/CFG.h"
#include "llvm/IR/Dominators.h"
#include "llvm/IR/IRBuilder.h"
#include "llvm/IR/Instructions.h"
#include "llvm/IR/IntrinsicInst.h"
#include "llvm/IR/MDBuilder.h"
#include "llvm/IR/Module.h"
#include "llvm/Support/CommandLine.h"
#include "llvm/Support/ErrorHandling.h"
#include "llvm/Support/raw_ostream.h"
#include "llvm/Transforms/Utils/BasicBlockUtils.h"
#include "llvm/Transforms/Utils/Cloning.h"
#include "llvm/Transforms/Utils/LoopSimplify.h"
#include "llvm/Transforms/Utils/LoopUtils.h"
#include "llvm/Transforms/Utils/PromoteMemToReg.h"
#include "llvm/Transforms/Utils/ValueMapper.h"
#include <functional>
#include <map>
#include <set>
#include <string>
#include <vector>
using namespace llvm;

static cl::opt<bool> PollOn("shadow-poll", cl::init(false),
                            cl::desc("tick on every cycle and call host_poll when the count runs out"));
static cl::opt<unsigned> PollOffset("shadow-poll-offset", cl::init(0),
                                    cl::desc("offset of the poll counter from the base of the shadow-stack region"));
static cl::opt<std::string> PollFault("shadow-poll-fault", cl::init(""), cl::Hidden,
                                      cl::desc("for the tests of checkPolls only: 'cond' gives every tick outside "
                                               "the dispatch a test that never holds, 'dispatch' leaves the index of "
                                               "the dispatch as it is"));

static const char *TickMD = "goasm.tick";
static const char *PollLabelMD = "goasm.poll.dispatch";

// checkPollOptions stops on an offset that is not a word inside the region.
void checkPollOptions(unsigned sizeLog2) {
  if (!PollOn)
    return;
  if (PollOffset % 8 != 0 || PollOffset + 8 > (1ull << sizeLog2))
    reportFatalUsageError(Twine("-shadow-poll-offset=") + Twine(PollOffset) + " is not a word inside the region");
  if (PollFault != "" && PollFault != "cond" && PollFault != "dispatch")
    reportFatalUsageError(Twine("-shadow-poll-fault=") + PollFault.getValue() + " is not cond or dispatch");
}

// The one indirect branch of F, when F has exactly one: the dispatch of its
// computed gotos.
static IndirectBrInst *dispatchOf(Function &F) {
  IndirectBrInst *IB = nullptr;
  unsigned n = 0;
  for (BasicBlock &B : F)
    if (auto *I = dyn_cast<IndirectBrInst>(B.getTerminator()))
      IB = I, n++;
  return n == 1 ? IB : nullptr;
}

// prepareDispatchPolls runs before the computed goto tables become offsets
// and before the shadow pointer is added. Each table gets one more entry, a
// poll label that calls host_poll and dispatches again from its own copy of
// the dispatch. A branch from the label back to the dispatch would give the
// dispatch's phis an incoming value from a block they dominate, and llc then
// stops copying the dispatch into the end of every handler.
void prepareDispatchPolls(Module &M) {
  if (!PollOn)
    return;
  LLVMContext &C = M.getContext();
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
  // host_poll before -shadow-add-sp gives it the shadow pointer.
  FunctionCallee poll = M.getOrInsertFunction("host_poll", FunctionType::get(Type::getVoidTy(C), {}, false));
  for (GlobalVariable *G : tables) {
    auto *A = cast<ConstantArray>(G->getInitializer());
    Function *F = cast<BlockAddress>(A->getOperand(0))->getFunction();
    IndirectBrInst *IB = dispatchOf(*F);
    if (!IB)
      reportFatalUsageError(Twine("poll: computed goto table of a function without exactly one indirect branch: ") +
                            F->getName());
    BasicBlock *D = IB->getParent();
    BasicBlock *PB = BasicBlock::Create(C, "poll.dispatch", F);
    IRBuilder<> B(PB);
    CallInst *call = B.CreateCall(poll, {});
    call->setMetadata(PollLabelMD, MDNode::get(C, {}));
    ValueToValueMapTy VM;
    for (Instruction &I : *D) {
      if (isa<PHINode>(&I))
        continue;
      Instruction *NI = I.clone();
      RemapInstruction(NI, VM, RF_NoModuleLevelChanges | RF_IgnoreMissingLocals);
      B.Insert(NI);
      VM[&I] = NI;
    }
    std::set<BasicBlock *> succs(succ_begin(D), succ_end(D));
    for (BasicBlock *S : succs)
      for (PHINode &Phi : S->phis()) {
        Value *V = Phi.getIncomingValueForBlock(D);
        auto it = VM.find(V);
        Phi.addIncoming(it != VM.end() ? cast<Value>(it->second) : V, PB);
      }
    IB->addDestination(PB);
    SmallVector<Constant *> es;
    for (auto &E : A->operands())
      es.push_back(cast<Constant>(E));
    es.push_back(BlockAddress::get(F, PB));
    auto *T = ArrayType::get(A->getType()->getElementType(), es.size());
    auto *G2 = new GlobalVariable(M, T, true, G->getLinkage(), ConstantArray::get(T, es), "");
    G2->takeName(G);
    G2->setAlignment(G->getAlign());
    G->replaceAllUsesWith(G2);
    G->eraseFromParent();
    errs() << "POLL-DISPATCH " << F->getName() << " " << es.size() - 1 << "\n";
  }
}

// LeafChunk is the most a call of a register-entry leaf (memcpy, memmove,
// memset) copies or sets at once: the leaves have no ticks, so a longer one
// goes through a chunked helper, whose loop ticks LeafChunkWeight per chunk.
static const uint64_t LeafChunk = 256 << 10, LeafChunkWeight = 65536;
static const char *ChunkWeightMD = "goasm.poll.weight";

// chunkHelper defines goasm_chunked_<op>(dst, src or byte, n), which calls
// the leaf op on at most LeafChunk bytes at a time: forward, or for memmove
// onto an overlapping higher destination, backward from the end. It is built
// as clang builds the C (like, a function of the C that calls the leaf):
// with its target attributes and frame pointer.
static Function *chunkHelper(Module &M, StringRef op, Function *leaf, Function *like) {
  LLVMContext &C = M.getContext();
  Type *I64 = Type::getInt64Ty(C), *I32 = Type::getInt32Ty(C), *I8 = Type::getInt8Ty(C);
  PointerType *P = PointerType::getUnqual(C);
  bool set = op == "memset";
  std::string name = ("goasm_chunked_" + op).str();
  if (Function *F = M.getFunction(name))
    return F;
  Function *F = Function::Create(FunctionType::get(Type::getVoidTy(C), {P, set ? (Type *)I32 : P, I64}, false),
                                 GlobalValue::InternalLinkage, name, M);
  for (const Attribute &A : like->getAttributes().getFnAttrs())
    F->addFnAttr(A);
  for (Attribute::AttrKind K : {Attribute::AlwaysInline, Attribute::OptimizeNone, Attribute::Cold, Attribute::Hot,
                                Attribute::NoReturn, Attribute::Naked, Attribute::MinSize, Attribute::OptimizeForSize})
    F->removeFnAttr(K);
  F->addFnAttr(Attribute::NoInline);
  Argument *D = F->getArg(0), *S = F->getArg(1), *N = F->getArg(2);
  BasicBlock *entry = BasicBlock::Create(C, "entry", F);
  BasicBlock *exit = BasicBlock::Create(C, "exit", F);
  ReturnInst::Create(C, exit);
  MDNode *weight = MDNode::get(C, ConstantAsMetadata::get(ConstantInt::get(I64, LeafChunkWeight)));
  auto call = [&](IRBuilder<> &B, Value *d, Value *s, Value *n) {
    Value *a1 = s;
    if (!leaf) {
      if (set)
        B.CreateMemSet(d, B.CreateTrunc(s, I8), n, MaybeAlign());
      else if (op == "memmove")
        B.CreateMemMove(d, MaybeAlign(), s, MaybeAlign(), n);
      else
        B.CreateMemCpy(d, MaybeAlign(), s, MaybeAlign(), n);
      return;
    }
    B.CreateCall(leaf, {d, a1, n});
  };
  // One loop: at offset off (counting up from 0, or down from n), copy
  // umin(left, LeafChunk) bytes.
  auto loop = [&](BasicBlock *from, bool back) -> BasicBlock * {
    BasicBlock *L = BasicBlock::Create(C, back ? "back" : "fwd", F);
    IRBuilder<> FB(from);
    FB.CreateBr(L);
    IRBuilder<> B(L);
    PHINode *left = B.CreatePHI(I64, 2, "left");
    left->addIncoming(N, from);
    Value *c = B.CreateBinaryIntrinsic(Intrinsic::umin, left, B.getInt64(LeafChunk));
    Value *left1 = B.CreateSub(left, c);
    Value *off = back ? left1 : B.CreateSub(N, left);
    Value *d = B.CreateGEP(I8, D, off);
    Value *s = set ? (Value *)S : B.CreateGEP(I8, S, off);
    call(B, d, s, c);
    left->addIncoming(left1, L);
    BranchInst *Br = B.CreateCondBr(B.CreateICmpNE(left1, B.getInt64(0)), L, exit);
    Br->setMetadata(ChunkWeightMD, weight);
    return L;
  };
  if (op == "memmove") {
    IRBuilder<> B(entry);
    Value *d = B.CreatePtrToInt(D, I64), *s = B.CreatePtrToInt(S, I64);
    Value *over = B.CreateAnd(B.CreateICmpUGT(d, s), B.CreateICmpULT(d, B.CreateAdd(s, N)));
    BasicBlock *f = BasicBlock::Create(C, "pickfwd", F), *b = BasicBlock::Create(C, "pickback", F);
    B.CreateCondBr(over, b, f);
    loop(f, false);
    loop(b, true);
  } else {
    loop(entry, false);
  }
  return F;
}

// chunkLeafCalls sends every call of memcpy, memmove or memset (the leaves,
// or the memory intrinsics llc turns into calls of them) whose length may
// be over LeafChunk to the chunked helper when it is. It runs before the
// shadow pointer is added, so the helpers get one like any function.
void chunkLeafCalls(Module &M) {
  if (!PollOn)
    return;
  LLVMContext &C = M.getContext();
  Type *I32 = Type::getInt32Ty(C);
  MDBuilder MDB(C);
  struct Site {
    CallBase *CB;
    StringRef op;
  };
  std::vector<Site> sites;
  for (Function &F : M) {
    if (F.isDeclaration() || F.getName().starts_with("goasm_chunked_") || F.getName() == "memcpy" ||
        F.getName() == "memmove" || F.getName() == "memset")
      continue;
    for (BasicBlock &B : F)
      for (Instruction &I : B) {
        auto *CB = dyn_cast<CallBase>(&I);
        if (!CB)
          continue;
        StringRef op;
        if (auto *MI = dyn_cast<MemIntrinsic>(CB)) {
          if (MI->isVolatile() || MI->isForceInlined() || isa<MemSetPatternInst>(MI))
            continue;
          op = isa<MemSetInst>(MI) ? "memset" : isa<MemMoveInst>(MI) ? "memmove" : "memcpy";
        } else if (Function *G = CB->getCalledFunction()) {
          if (G->getName() == "memcpy" || G->getName() == "memmove" || G->getName() == "memset")
            op = G->getName();
        }
        if (op.empty())
          continue;
        Value *n = CB->getArgOperand(2);
        if (auto *K = dyn_cast<ConstantInt>(n); K && K->getZExtValue() <= LeafChunk)
          continue;
        if (!n->getType()->isIntegerTy(64))
          reportFatalUsageError(Twine("poll: a ") + op + " call whose length is not 64-bit in " + F.getName());
        if (!isa<CallInst>(CB))
          reportFatalUsageError(Twine("poll: a ") + op + " call that is not a plain call in " + F.getName());
        sites.push_back({CB, op});
      }
  }
  unsigned n = 0;
  for (Site &st : sites) {
    // The operands as they are now: an earlier site may have replaced the
    // result of its leaf call, which this one used.
    Value *d = st.CB->getArgOperand(0), *src = st.CB->getArgOperand(1), *len = st.CB->getArgOperand(2);
    Function *leaf = M.getFunction(st.op);
    if (leaf && leaf->getFunctionType()->getNumParams() != 3)
      reportFatalUsageError(Twine("poll: ") + st.op + " has an unexpected type");
    Function *H = chunkHelper(M, st.op, leaf, st.CB->getFunction());
    // A leaf's result is its destination.
    if (!st.CB->getType()->isVoidTy())
      st.CB->replaceAllUsesWith(d);
    Instruction *ThenT, *ElseT;
    IRBuilder<> B(st.CB);
    SplitBlockAndInsertIfThenElse(B.CreateICmpUGT(len, B.getInt64(LeafChunk)), st.CB, &ThenT, &ElseT,
                                  MDB.createUnlikelyBranchWeights());
    IRBuilder<> TB(ThenT);
    if (st.op == "memset")
      src = src->getType()->isIntegerTy(32) ? src : TB.CreateZExt(src, I32);
    TB.CreateCall(H, {d, src, len});
    st.CB->moveBefore(ElseT->getIterator());
    n++;
  }
  errs() << "POLL-LEAF calls " << n << "\n";
}

// Retreating edges of a depth-first walk from the entry, once each.
static std::vector<std::pair<BasicBlock *, BasicBlock *>> retreatingEdges(Function &F) {
  std::vector<std::pair<BasicBlock *, BasicBlock *>> edges;
  DenseMap<BasicBlock *, int> state; // 1 on the walk's path, 2 done
  std::vector<std::pair<BasicBlock *, succ_iterator>> st;
  std::set<std::pair<BasicBlock *, BasicBlock *>> seen;
  BasicBlock *E = &F.getEntryBlock();
  st.push_back({E, succ_begin(E)});
  state[E] = 1;
  while (!st.empty()) {
    auto &[B, it] = st.back();
    if (it == succ_end(B)) {
      state[B] = 2;
      st.pop_back();
      continue;
    }
    BasicBlock *S = *it;
    ++it;
    int x = state.lookup(S);
    if (x == 1) {
      if (seen.insert({B, S}).second)
        edges.push_back({B, S});
    } else if (x == 0) {
      state[S] = 1;
      st.push_back({S, succ_begin(S)});
    }
  }
  return edges;
}

static bool isPollCall(const Instruction &I) {
  auto *CB = dyn_cast<CallBase>(&I);
  return CB && CB->getCalledFunction() && CB->getCalledFunction()->getName() == "host_poll";
}

// mayTick tells whether a call can tick or poll before it returns, so that a
// loop keeping the count in a register must store it before the call and
// load it after: a call through a pointer, a function with a cycle or one
// that calls such a function, host_poll and host_call (which may enter the
// engine again). The other host callbacks and the register-entry leaves run
// no engine code.
static std::set<Function *> tickers(Module &M, const std::set<std::string> &leaves) {
  std::set<Function *> t;
  std::map<Function *, std::vector<Function *>> callees;
  std::vector<Function *> fs;
  for (Function &F : M) {
    if (F.isDeclaration()) {
      if (F.getName() == "host_poll" || F.getName() == "host_call")
        t.insert(&F);
      continue;
    }
    if (leaves.count(F.getName().str()))
      continue;
    fs.push_back(&F);
    bool tick = !retreatingEdges(F).empty();
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (auto *CB = dyn_cast<CallBase>(&I)) {
          if (CB->isInlineAsm())
            continue;
          Function *G = CB->getCalledFunction();
          if (!G)
            tick = true;
          else if (!G->isIntrinsic())
            callees[&F].push_back(G);
        }
    if (tick)
      t.insert(&F);
  }
  for (bool changed = true; changed;) {
    changed = false;
    for (Function *F : fs)
      if (!t.count(F))
        for (Function *G : callees[F])
          if (t.count(G)) {
            t.insert(F);
            changed = true;
            break;
          }
  }
  return t;
}

// A loop runs a bounded stretch of code between ticks when it is innermost,
// calls nothing (callFree), and scalar evolution proves it takes its back
// edges at most MaxRun - 1 times per entry, with its runs times its
// instructions within RunBudget. Its back edges need no tick: whatever
// enters it again ticks. A tick then stands for at most RunBudget
// instructions of such a loop, so that the interval of the host bounds the
// time between two polls: 10000 ticks are a few milliseconds at most.
static const uint64_t MaxRun = 256, RunBudget = 256;

using Edge = std::pair<BasicBlock *, BasicBlock *>;

static uint64_t loopSize(const Loop *L) {
  uint64_t n = 0;
  for (BasicBlock *B : L->blocks())
    for (Instruction &I : *B)
      n += !isa<PHINode>(&I) && !I.isDebugOrPseudoInst();
  return n;
}

// callFree: no call in the loop but intrinsics llc expands in place, and a
// memory intrinsic only of a small constant length. Its branches are plain
// ones, so its cycles are the loop's.
static bool callFree(const Loop *L) {
  for (BasicBlock *B : L->blocks()) {
    const Instruction *T = B->getTerminator();
    if (!isa<BranchInst>(T) && !isa<SwitchInst>(T) && !isa<ReturnInst>(T) && !isa<UnreachableInst>(T))
      return false;
    for (Instruction &I : *B) {
      auto *CB = dyn_cast<CallBase>(&I);
      if (!CB)
        continue;
      if (!isa<IntrinsicInst>(CB))
        return false;
      if (auto *MI = dyn_cast<MemIntrinsic>(CB)) {
        auto *N = dyn_cast<ConstantInt>(MI->getLength());
        if (!N || N->getZExtValue() > 256)
          return false;
      }
    }
  }
  return true;
}

// The latch-to-header edges of the loops of F that scalar evolution bounds.
static std::set<Edge> boundedEdges(Function &F, const Triple &T) {
  TargetLibraryInfoImpl TLII(T);
  TargetLibraryInfo TLI(TLII, &F);
  AssumptionCache AC(F);
  DominatorTree DT(F);
  LoopInfo LI(DT);
  ScalarEvolution SE(F, TLI, AC, DT, LI);
  std::set<Edge> edges;
  for (Loop *L : LI.getLoopsInPreorder()) {
    if (!L->isInnermost() || !callFree(L))
      continue;
    auto *BTC = dyn_cast<SCEVConstant>(SE.getConstantMaxBackedgeTakenCount(L));
    if (!BTC || BTC->getAPInt().getActiveBits() > 32)
      continue;
    uint64_t runs = BTC->getAPInt().getZExtValue() + 1;
    if (runs > MaxRun || runs * std::max<uint64_t>(1, loopSize(L)) > RunBudget)
      continue;
    SmallVector<BasicBlock *> latches;
    L->getLoopLatches(latches);
    for (BasicBlock *Lt : latches)
      edges.insert({Lt, L->getHeader()});
  }
  return edges;
}

// stripLoops splits each innermost loop that calls nothing (callFree), and
// that scalar evolution does not bound already, into chunks: the loop
// runs at most a chunk of iterations and leaves to a new block, which
// counts the whole chunk with one tick and enters it again from a new outer
// header. Scalar evolution then bounds the loop: it has no tick and keeps
// the poll out of it, so its registers stay as they were. Returns the new
// blocks with the weight of their tick.
static std::map<BasicBlock *, uint64_t> stripLoops(Function &F, unsigned &n) {
  std::map<BasicBlock *, uint64_t> chunks;
  DominatorTree DT(F);
  LoopInfo LI(DT);
  std::set<Edge> bounded = boundedEdges(F, Triple(F.getParent()->getTargetTriple()));
  struct Cand {
    Loop *L;
    uint64_t chunk;
  };
  std::vector<Cand> cands;
  for (Loop *L : LI.getLoopsInPreorder()) {
    if (!L->isInnermost() || !callFree(L) || !L->getLoopLatch() ||
        !isa<BranchInst>(L->getLoopLatch()->getTerminator()))
      continue;
    if (bounded.count({L->getLoopLatch(), L->getHeader()}))
      continue;
    // The chunk keeps a chunk's run, with the counter the split adds, in
    // the budget the check holds bounded loops to.
    uint64_t chunk = std::min<uint64_t>(MaxRun, RunBudget / (loopSize(L) + 4));
    if (chunk < 16)
      continue;
    if (!L->getLoopPreheader() && !InsertPreheaderForLoop(L, &DT, &LI, nullptr, false))
      continue;
    cands.push_back({L, chunk});
  }
  LLVMContext &C = F.getContext();
  Type *I64 = Type::getInt64Ty(C);
  MDBuilder MDB(C);
  for (auto [L, chunk] : cands) {
    BasicBlock *H = L->getHeader(), *Ph = L->getLoopPreheader(), *Lt = L->getLoopLatch();
    // Ph -> O -> H, and Lt -> Y -> H, where Y counts the chunk down and
    // leaves for X at its end, and X goes back to O.
    BasicBlock *O = SplitEdge(Ph, H);
    BasicBlock *Y = SplitEdge(Lt, H);
    O->setName("poll.chunk.outer");
    Y->setName("poll.chunk.latch");
    BasicBlock *X = BasicBlock::Create(C, "poll.chunk.end", &F);
    BranchInst::Create(O, X);
    for (PHINode &Phi : H->phis()) {
      PHINode *Po = PHINode::Create(Phi.getType(), 2, Phi.getName() + ".outer", O->begin());
      Po->addIncoming(Phi.getIncomingValueForBlock(O), Ph);
      Po->addIncoming(Phi.getIncomingValueForBlock(Y), X);
      Phi.setIncomingValueForBlock(O, Po);
    }
    PHINode *K = PHINode::Create(I64, 2, "poll.chunk.left", H->begin());
    IRBuilder<> YB(Y->getTerminator());
    Value *K1 = YB.CreateNUWSub(K, YB.getInt64(1));
    BranchInst *Br = YB.CreateCondBr(YB.CreateICmpEQ(K1, YB.getInt64(0)), X, H, MDB.createUnlikelyBranchWeights());
    Y->getTerminator()->eraseFromParent();
    (void)Br;
    K->addIncoming(ConstantInt::get(I64, chunk), O);
    K->addIncoming(K1, Y);
    chunks[X] = chunk;
    n++;
  }
  return chunks;
}

void insertPolls(Module &M, const std::set<std::string> &leaves, unsigned sizeLog2) {
  if (!PollOn)
    return;
  LLVMContext &C = M.getContext();
  Type *I64 = Type::getInt64Ty(C);
  PointerType *P = PointerType::getUnqual(C);
  const uint64_t S = 1ull << sizeLog2;
  FunctionCallee poll = M.getOrInsertFunction("host_poll", FunctionType::get(Type::getVoidTy(C), {P}, false));
  if (cast<Function>(poll.getCallee())->getFunctionType() != poll.getFunctionType())
    reportFatalUsageError("poll: host_poll is declared with another type");
  MDBuilder MDB(C);
  MDNode *tickMD = MDNode::get(C, {});
  std::set<Function *> ticks = tickers(M, leaves);
  unsigned nticks = 0, nreg = 0, nloops = 0, ndispatch = 0, nsync = 0, nstrip = 0, nbounded = 0;
  for (Function &F : M) {
    if (F.isDeclaration() || leaves.count(F.getName().str()))
      continue;
    if (retreatingEdges(F).empty())
      continue;
    if (F.arg_size() == 0 || !F.getArg(0)->getType()->isPointerTy())
      reportFatalUsageError(Twine("poll: function without a shadow pointer: ") + F.getName());
    Argument *sp = F.getArg(0);
    IRBuilder<> E(&*F.getEntryBlock().getFirstInsertionPt());
    auto counterPtr = [&](IRBuilder<> &B) -> Value * {
      Value *base = B.CreateAnd(B.CreatePtrToInt(sp, I64), B.getInt64(~(S - 1)));
      return B.CreateIntToPtr(B.CreateAdd(base, B.getInt64(PollOffset)), P, "poll.counter");
    };
    // The dispatch is the indirect branch that can go to the poll label
    // (the label's own copy of it cannot).
    BasicBlock *dispatch = nullptr, *label = nullptr;
    for (BasicBlock &D : F)
      if (isa<IndirectBrInst>(D.getTerminator()))
        for (BasicBlock *Sx : successors(&D))
          for (Instruction &I : *Sx)
            if (I.getMetadata(PollLabelMD)) {
              if (dispatch)
                reportFatalUsageError(Twine("poll: two dispatches reach a poll label in ") + F.getName());
              dispatch = &D, label = Sx;
            }
    std::map<BasicBlock *, uint64_t> chunks = stripLoops(F, nstrip);
    std::set<Edge> exempt = boundedEdges(F, Triple(M.getTargetTriple()));
    nbounded += exempt.size();
    DominatorTree DT(F);
    LoopInfo LI(DT);
    // The loops that keep the count in a local, which mem2reg puts in a
    // register.
    std::map<Loop *, AllocaInst *> local;
    SmallVector<AllocaInst *> allocas;
    // A call that may tick: through a pointer, or to a function that ticks.
    auto mayTick = [&](Instruction &I) {
      auto *CB = dyn_cast<CallBase>(&I);
      if (!CB || CB->isInlineAsm() || isPollCall(I))
        return false;
      Function *G = CB->getCalledFunction();
      return !G || ticks.count(G) > 0;
    };
    // A call that may tick sees the count, and the code after it sees what
    // the call counted.
    auto syncCalls = [&](ArrayRef<BasicBlock *> blocks, AllocaInst *A) {
      SmallVector<CallBase *> cs;
      for (BasicBlock *Bk : blocks)
        for (Instruction &I : *Bk)
          if (mayTick(I))
            cs.push_back(cast<CallBase>(&I));
      for (CallBase *CB : cs) {
        if (!isa<CallInst>(CB))
          reportFatalUsageError(Twine("poll: a call that is not a plain call in ") + F.getName());
        IRBuilder<> BB(CB);
        BB.CreateStore(BB.CreateLoad(I64, A), counterPtr(BB));
        IRBuilder<> AB(CB->getNextNode());
        AB.CreateStore(AB.CreateLoad(I64, counterPtr(AB)), A);
        nsync++;
      }
    };
    // In a function with a dispatch, the dispatch is on cycles that no
    // natural loop holds, so the whole function keeps the count in one local,
    // from its entry to each return.
    AllocaInst *whole = nullptr;
    if (dispatch) {
      whole = E.CreateAlloca(I64, nullptr, "poll.local");
      allocas.push_back(whole);
      SmallVector<BasicBlock *> blocks;
      for (BasicBlock &Bk : F)
        blocks.push_back(&Bk);
      syncCalls(blocks, whole);
      E.CreateStore(E.CreateLoad(I64, counterPtr(E)), whole);
      for (BasicBlock *Bk : blocks)
        if (isa<ReturnInst>(Bk->getTerminator())) {
          IRBuilder<> RB(Bk->getTerminator());
          RB.CreateStore(RB.CreateLoad(I64, whole), counterPtr(RB));
        }
    }
    // Elsewhere, the outermost loops with no call that may tick keep the
    // count in a local, which mem2reg puts in a register. A loop with such a
    // call counts in memory, as the call does: a local would be written back
    // and read again around every call, which costs more than the tick. Its
    // inner loops without one get a local of their own.
    std::function<void(Loop *)> keepLocal = [&](Loop *L) {
      // A bounded loop has no tick.
      if (L->isInnermost() && exempt.count({L->getLoopLatch(), L->getHeader()}))
        return;
      for (BasicBlock *Bk : L->blocks())
        for (Instruction &I : *Bk)
          if (mayTick(I)) {
            for (Loop *Sub : SmallVector<Loop *>(L->begin(), L->end()))
              keepLocal(Sub);
            return;
          }
      BasicBlock *ph = L->getLoopPreheader();
      if (!ph)
        ph = InsertPreheaderForLoop(L, &DT, &LI, nullptr, false);
      if (!ph)
        return;
      formDedicatedExitBlocks(L, &DT, &LI, nullptr, false);
      if (!L->hasDedicatedExits())
        return;
      AllocaInst *A = E.CreateAlloca(I64, nullptr, "poll.local");
      allocas.push_back(A);
      IRBuilder<> PB(ph->getTerminator());
      PB.CreateStore(PB.CreateLoad(I64, counterPtr(PB)), A);
      SmallVector<BasicBlock *> exits;
      L->getUniqueExitBlocks(exits);
      for (BasicBlock *X : exits) {
        IRBuilder<> XB(&*X->getFirstInsertionPt());
        XB.CreateStore(XB.CreateLoad(I64, A), counterPtr(XB));
      }
      local[L] = A;
      nloops++;
    };
    if (!whole)
      for (Loop *L : SmallVector<Loop *>(LI.begin(), LI.end()))
        keepLocal(L);
    // The local of an edge: that of the loop with one that holds both ends.
    auto localOf = [&](BasicBlock *u, BasicBlock *v) -> AllocaInst * {
      if (whole)
        return whole;
      for (Loop *L = LI.getLoopFor(u); L; L = L->getParentLoop()) {
        auto it = local.find(L);
        if (it != local.end())
          return L->contains(v) ? it->second : nullptr;
      }
      return nullptr;
    };
    // The count of a tick: in the local, or in memory.
    auto tick = [&](IRBuilder<> &B, AllocaInst *A, uint64_t weight) -> Value * {
      Value *at = A ? (Value *)A : counterPtr(B);
      Value *c = B.CreateLoad(I64, at, "poll.c");
      auto *c1 = cast<Instruction>(B.CreateSub(c, B.getInt64(weight), "poll.c1"));
      c1->setMetadata(TickMD, tickMD);
      B.CreateStore(c1, at);
      nticks++;
      if (A)
        nreg++;
      return c1;
    };
    // After host_poll the local takes the count the host set.
    auto reload = [&](IRBuilder<> &B, AllocaInst *A) {
      if (A)
        B.CreateStore(B.CreateLoad(I64, counterPtr(B)), A);
    };
    if (dispatch) {
      // The index into the offsets of the table (relativeGotoTables): an
      // expired count picks the last entry, the poll label.
      GetElementPtrInst *idxGEP = nullptr;
      for (Instruction &I : *dispatch)
        if (auto *G = dyn_cast<GetElementPtrInst>(&I))
          if (auto *GV = dyn_cast<GlobalVariable>(G->getPointerOperand()))
            if (GV->getName().ends_with(".rel") && G->getNumIndices() == 1 &&
                G->getSourceElementType()->isIntegerTy(32))
              idxGEP = G;
      if (!idxGEP)
        reportFatalUsageError(Twine("poll: no table load in the dispatch of ") + F.getName());
      auto *GV = cast<GlobalVariable>(idxGEP->getPointerOperand());
      uint64_t last = cast<ArrayType>(GV->getValueType())->getNumElements() - 1;
      AllocaInst *A = localOf(dispatch, dispatch);
      IRBuilder<> B(idxGEP);
      Value *c1 = tick(B, A, 1);
      Value *idx = idxGEP->getOperand(1);
      Value *expired = B.CreateICmpSLT(c1, B.getInt64(1));
      if (PollFault != "dispatch")
        idxGEP->setOperand(1, B.CreateSelect(expired, ConstantInt::get(idx->getType(), last), idx));
      for (Instruction &I : *label)
        if (I.getMetadata(PollLabelMD)) {
          IRBuilder<> AB(I.getNextNode());
          reload(AB, A);
          break;
        }
      ndispatch++;
    }
    // Where each edge's tick goes: the end of its source when the edge is
    // the only way out of it (or the way out is an indirect branch), else a
    // block of its own on the edge. All are placed before any is inserted.
    std::map<Instruction *, size_t> placed;
    std::vector<std::tuple<Instruction *, AllocaInst *, uint64_t>> at;
    auto placeTick = [&](Instruction *t, AllocaInst *A, uint64_t weight) {
      auto [it, fresh] = placed.insert({t, at.size()});
      if (fresh)
        at.push_back({t, A, weight});
      else
        std::get<2>(at[it->second]) = std::max(std::get<2>(at[it->second]), weight);
    };

    for (auto [u, v] : retreatingEdges(F)) {
      if (v == dispatch || u == label || exempt.count({u, v}))
        continue;
      auto ch = chunks.find(u);
      uint64_t weight = ch == chunks.end() ? 1 : ch->second;
      if (MDNode *W = u->getTerminator()->getMetadata(ChunkWeightMD))
        weight = mdconst::extract<ConstantInt>(W->getOperand(0))->getZExtValue();
      AllocaInst *A = localOf(u, v);
      Instruction *t = u->getTerminator();
      if (!(u->getSingleSuccessor() == v || isa<IndirectBrInst>(t) || isa<CallBrInst>(t)))
        t = SplitEdge(u, v, &DT, &LI)->getTerminator();
      placeTick(t, A, weight);
    }
    for (auto [t, A, weight] : at) {
      IRBuilder<> B(t);
      Value *c1 = tick(B, A, weight);
      // -shadow-poll-fault=cond: c1 < INT64_MIN, which never holds.
      Value *expired = B.CreateICmpSLT(c1, B.getInt64(PollFault == "cond" ? 1ull << 63 : 1));
      Instruction *T = SplitBlockAndInsertIfThen(expired, t, false, MDB.createUnlikelyBranchWeights());
      IRBuilder<> TB(T);
      TB.CreateCall(poll, {sp});
      reload(TB, A);
    }
    if (!allocas.empty()) {
      DominatorTree DT2(F);
      PromoteMemToReg(allocas, DT2);
    }
    for (BasicBlock &B : F)
      for (Instruction &I : B)
        if (isa<AllocaInst>(&I))
          reportFatalUsageError(Twine("poll: a counter left in an alloca in ") + F.getName());
  }
  errs() << "POLL ticks " << nticks << " in registers " << nreg << " loops in registers " << nloops
         << " synced calls " << nsync << " dispatch " << ndispatch << " chunked loops " << nstrip
         << " bounded edges " << nbounded << "\n";
}

// expiredTest tells whether Cond holds whenever the count Tick leaves has run
// out (is 0 or less): Tick < K with K at least 1, or Tick <= K with K at least
// 0.
static bool expiredTest(Value *Cond, Instruction *Tick) {
  auto *Cmp = dyn_cast<ICmpInst>(Cond);
  auto *K = Cmp ? dyn_cast<ConstantInt>(Cmp->getOperand(1)) : nullptr;
  if (!K || Cmp->getOperand(0) != Tick || K->getBitWidth() > 64)
    return false;
  if (Cmp->getPredicate() == ICmpInst::ICMP_SLT)
    return K->getSExtValue() >= 1;
  if (Cmp->getPredicate() == ICmpInst::ICMP_SLE)
    return K->getSExtValue() >= 0;
  return false;
}

// dispatchPolls tells whether the indirect branch IB goes to a poll label
// whenever the count Tick leaves has run out. Its address must be the table
// of offsets (relativeGotoTables) plus the entry it reads at an index that a
// select on an expired test turns into an entry naming a poll label of IB:
//
//   R + sext(load(R + 4 * select(expired, K, index)))
//   R[K] = trunc(ptrtoint(blockaddress(label)) - ptrtoint(R))
static bool dispatchPolls(IndirectBrInst *IB, Instruction *Tick) {
  auto *A = dyn_cast<GetElementPtrInst>(IB->getAddress());
  auto *R = A ? dyn_cast<GlobalVariable>(A->getPointerOperand()) : nullptr;
  if (!R || !R->isConstant() || !R->hasInitializer() || A->getNumIndices() != 1 ||
      !A->getSourceElementType()->isIntegerTy(8))
    return false;
  auto *X = dyn_cast<SExtInst>(A->getOperand(1));
  auto *L = X ? dyn_cast<LoadInst>(X->getOperand(0)) : nullptr;
  auto *G = L ? dyn_cast<GetElementPtrInst>(L->getPointerOperand()) : nullptr;
  if (!G || G->getPointerOperand() != R || G->getNumIndices() != 1 || !G->getSourceElementType()->isIntegerTy(32))
    return false;
  auto *S = dyn_cast<SelectInst>(G->getOperand(1));
  auto *K = S ? dyn_cast<ConstantInt>(S->getTrueValue()) : nullptr;
  auto *T = dyn_cast<ConstantArray>(R->getInitializer());
  if (!K || !T || !expiredTest(S->getCondition(), Tick) || K->getZExtValue() >= T->getNumOperands())
    return false;
  auto *Tr = dyn_cast<ConstantExpr>(T->getOperand(K->getZExtValue()));
  auto *Sub = Tr && Tr->getOpcode() == Instruction::Trunc ? dyn_cast<ConstantExpr>(Tr->getOperand(0)) : nullptr;
  if (!Sub || Sub->getOpcode() != Instruction::Sub)
    return false;
  auto *To = dyn_cast<ConstantExpr>(Sub->getOperand(0)), *From = dyn_cast<ConstantExpr>(Sub->getOperand(1));
  if (!To || !From || To->getOpcode() != Instruction::PtrToInt || From->getOpcode() != Instruction::PtrToInt ||
      From->getOperand(0) != R)
    return false;
  auto *BA = dyn_cast<BlockAddress>(To->getOperand(0));
  if (!BA || !is_contained(successors(IB->getParent()), BA->getBasicBlock()))
    return false;
  for (Instruction &J : *BA->getBasicBlock())
    if (isPollCall(J) && J.getMetadata(PollLabelMD))
      return true;
  return false;
}

// checkPolls holds every function to the rule, apart from how the ticks were
// placed: with the blocks that tick (a decrement tagged goasm.tick) or call
// host_poll taken out, and the back edges of the loops scalar evolution
// bounds (boundedEdges, worked out again here), the control flow graph has
// no cycle left. A tick must take at least 1 from the count, and its block's
// way out must poll whenever the count it leaves has run out: a conditional
// branch on a test of the tick that holds then (expiredTest), whose taken
// side calls host_poll, or, in a dispatch, an indirect branch whose index
// turns to the entry of the poll label then (dispatchPolls).
void checkPolls(Module &M, const std::set<std::string> &leaves) {
  if (!PollOn)
    return;
  for (Function &F : M) {
    if (F.isDeclaration() || leaves.count(F.getName().str()))
      continue;
    std::set<Edge> bounded = boundedEdges(F, Triple(M.getTargetTriple()));
    std::set<BasicBlock *> cut;
    for (BasicBlock &B : F)
      for (Instruction &I : B) {
        if (isPollCall(I))
          cut.insert(&B);
        if (!I.getMetadata(TickMD))
          continue;
        cut.insert(&B);
        auto *Sub = dyn_cast<BinaryOperator>(&I);
        auto *W = Sub && Sub->getOpcode() == Instruction::Sub ? dyn_cast<ConstantInt>(Sub->getOperand(1)) : nullptr;
        if (!W || W->getBitWidth() > 64 || W->getSExtValue() < 1)
          reportFatalUsageError(Twine("poll check: a tick that does not take at least 1 from the count in ") +
                                F.getName());
        Instruction *T = B.getTerminator();
        if (auto *IB = dyn_cast<IndirectBrInst>(T)) {
          if (!dispatchPolls(IB, &I))
            reportFatalUsageError(
                Twine("poll check: a dispatch whose index does not turn to the poll label when the count runs out in ") +
                F.getName());
          continue;
        }
        bool ok = false;
        if (auto *Br = dyn_cast<BranchInst>(T))
          if (Br->isConditional() && expiredTest(Br->getCondition(), &I))
            for (Instruction &J : *Br->getSuccessor(0))
              ok |= isPollCall(J);
        if (!ok)
          reportFatalUsageError(Twine("poll check: a tick whose branch does not poll when the count runs out in ") +
                                F.getName());
      }
    for (scc_iterator<Function *> I = scc_begin(&F); !I.isAtEnd(); ++I) {
      if (!I.hasCycle())
        continue;
      // A cycle of the SCC that avoids the cut blocks: search the SCC with
      // them taken out.
      std::set<BasicBlock *> in;
      for (BasicBlock *B : *I)
        if (!cut.count(B))
          in.insert(B);
      // Kahn's algorithm on the SCC minus the cut: whatever is left has a
      // cycle.
      std::map<BasicBlock *, int> indeg;
      for (BasicBlock *B : in)
        indeg[B];
      for (BasicBlock *B : in)
        for (BasicBlock *Sx : successors(B))
          if (in.count(Sx) && !bounded.count({B, Sx}))
            indeg[Sx]++;
      std::vector<BasicBlock *> q;
      for (auto &[B, d] : indeg)
        if (d == 0)
          q.push_back(B);
      size_t done = 0;
      while (!q.empty()) {
        BasicBlock *B = q.back();
        q.pop_back();
        done++;
        for (BasicBlock *Sx : successors(B))
          if (in.count(Sx) && !bounded.count({B, Sx}) && --indeg[Sx] == 0)
            q.push_back(Sx);
      }
      if (done != in.size()) {
        std::string names;
        for (auto &[B, d] : indeg)
          if (d > 0 && names.size() < 200)
            names += " " + B->getName().str();
        reportFatalUsageError(Twine("poll check: a cycle without a tick or a poll in ") + F.getName() + ":" + names);
      }
    }
  }
}
