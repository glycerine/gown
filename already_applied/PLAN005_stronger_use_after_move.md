Stronger moved-use checking
===========================

2026-May-30

Plan:

Make GWN001 a proof-quality moved-use check: after an \iso root is moved by channel send, \iso call, go call, defer, return, assignment transfer, select send, or freeze, every later overlapping use is rejected unless the root is explicitly rebound to a fresh/new owner.

The current engine is close, but the hardening target is: DebugRef is for diagnostics, not soundness. The semantic backstop should be SSA operands plus state transitions.

Core Invariant
For every function CFG:

- Consumed means "this place is no longer available on this path."
- Merging paths unions Consumed, so "maybe moved" means unavailable.
- Any later read, write-through, call arg, send value, return value, interface conversion, closure capture, go, defer, selector, deref, or repeated transfer of an overlapping place is GWN001.
- Assignment to the root itself may clear Consumed only if we deliberately support rebinding and the RHS is a valid fresh/owned replacement. Field writes never clear it.

Implementation Slices
1. State-Level Repeated Consume Guard

Target: [ssa_state.go](/Users/jaten/go/src/github.com/glycerine/gown/ssa_state.go:57)

Add a first check inside ConsumeRoot:

- If state.CheckUse(place) is already moved, return GWN001.
- Fresh field projection moves still return GWN011.
- Moving while a borrow is active still returns GWN002.

Red tests:

- root move, then root move -> GWN001
- root move, then field projection use/move -> GWN001
- fresh field projection move -> still GWN011
- active borrow then move -> still GWN002

2. Operand-Level Use Checking

Target: [ssa_gwn001.go](/Users/jaten/go/src/github.com/glycerine/gown/ssa_gwn001.go:164)

Extend checkInstructionUses:

- Keep DebugRef checking for precise source positions.
- Add checkInstructionOperandUses(instr, state).
- Walk instr.Operands(nil).
- Map each operand through [SSAPlaceIndex](/Users/jaten/go/src/github.com/glycerine/gown/ssa_place.go:39).
- If operand place overlaps a consumed place, report GWN001.

Important classification:

- Most operands are uses.
- Store.Addr for root reassignment is a definition, not a use, if we choose to allow rebinding.
- Store.Addr for field/index/projection is a use/write-through and must reject after move.
- Store.Val, Send.X, call args, return values, MakeInterface.X, deref, selector/index base values are uses.
- DebugRef.IsAddr remains non-use unless a test proves we need a narrower rule.

3. Place Propagation Audit

Target: [ssa_place.go](/Users/jaten/go/src/github.com/glycerine/gown/ssa_place.go:94)

Add propagation only where needed by red tests:

- ChangeType, Convert, ChangeInterface: preserve place.
- Phi: preserve place only when all known incoming places agree on the same root/key; otherwise collapse conservatively or leave unmapped.
- Confirm FieldAddr, IndexAddr, Lookup, MakeInterface, UnOp behavior remains conservative.

4. Permanent Adversarial Test File

Add ssa_use_after_move_hardening_test.go.

Red-first matrix:

- send -> send
- send -> \iso call
- call -> send
- call -> call
- send -> return
- call -> return
- assignment move -> source use
- assignment move -> source send/call
- selector read after send
- selector write after send
- deref after send
- compare to nil after send
- interface conversion after send
- method receiver after send
- closure capture after send
- go use after send
- defer use after send
- loop body sends same x twice across iterations
- branch maybe-send then use
- branch send-and-return, then use on only non-moved path should be allowed
- select with mutually exclusive sends remains allowed
- select send then post-use rejects
- \clone(x) send still does not consume x
- \new(...) send still consumes no source
- \imm send remains shareable

5. Rebinding Decision

We should explicitly decide this before coding operand skips:

go
ch <- x
x = \new(T{})
println(x)

I would allow this eventually, because it is a new owner in the same variable slot. For this hardening pass, either:

- implement root rebinding carefully, with tests, or
- reject rebinding after move for now, as a conservative rule.

My preference: implement root rebinding only for obviously fresh/owned RHS: \new, \clone, receive from chan \iso, or another valid \iso move.

6. Proof Note In Docs

Update [docs/ssa-checker-architecture.md](/Users/jaten/go/src/github.com/glycerine/gown/docs/ssa-checker-architecture.md:136) with the crisp statement:

"DebugRef improves diagnostics, but moved-use soundness is enforced by SSA operand checks plus conservative consumed-state merging."

Verification
Run focused tests first:

go test -run 'TestSSAFunctionState|TestGWN001|TestSSAGWN001|TestUseAfterMove|TestSSASelect|TestSSAReceiveIsoThenSend' ./


This plan keeps the work aimed at the actual proof critique: after a move, the source owner must be dead on every path unless it is re-established by a checked rebinding.


Revise: to include "implement root rebinding carefully, with tests". This is super common, so we require it.

Agreed. Root rebinding should be in the hardening pass, not deferred.

Revised Rule
After an \iso root is consumed, the same variable may become usable again only by a checked root rebinding:

go
ch <- x          // x consumed
x = \new(T{})    // x rebound to fresh iso
Use(x)           // allowed


A rebinding is valid only when assigning directly to the consumed root, not a field/index/projection, and the RHS is a valid owned replacement.

Allowed v1 RHS forms:

- \new(T{...})
- \clone(y) where clone validation passed
- receive from chan \iso T
- another \iso place move, if that source is valid and becomes consumed
- fresh owned composite/address expression already recognized by the checker

Rejected:

- x.f = ... after x moved
- x[i] = ... after x moved
- x = untracked
- x = \mub(y) / x = \rob(y) / x = \unsafe(y)
- x = y if y is not provably \iso
- x = y if y is already consumed/frontiered
- use of x on RHS of its own rebinding after move

Implementation Change
In the hardening plan, Slice 2’s operand checker needs a special case for root rebinding:

For ssa.Store / assignment debug mapping:

- If the instruction corresponds to x = rhs where x is a direct root and x is currently consumed:
  - Do not treat the LHS address as a use.
  - Validate rhs as producing CapIso.
  - If RHS is non-fresh \iso place, check it is not consumed/frontiered, then consume RHS.
  - Clear consumed/frontiered state for x.
- If assignment target is not direct root, treat target base as a normal use and reject if moved.

This should reuse existing assignment-move binding where possible, then extend it to distinguish:

- root rebinding to fresh owned value
- root rebinding by moving another iso root
- invalid rebinding

Extra Tests
Add these to ssa_use_after_move_hardening_test.go:

- send then x = \new(payload{}) then use x allowed
- call then x = \clone(y) then use x allowed
- send then x = <-in from chan \iso *payload then use x allowed
- send then x = y where y \iso then use x allowed and later use of y rejects
- send then x = y where y untracked rejects
- send then x = \rob(y) rejects
- send then x = x rejects
- send then x.Field = ... rejects
- send then x = y after y frontiered rejects
- branch where only one path rebinds x, then post-merge use rejects
- branch where all paths rebind x, then post-merge use allowed only if merge logic can prove both paths unconsumed; if not, conservatively reject until we model definite rebinding

That last one is important: rebinding introduces "definitely restored" reasoning. For v1, we can be conservative at joins. The required common case is linear rebinding in the same block or along simple straight-line control flow.
