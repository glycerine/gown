# SSA Borrow Checker Detailed Implementation Plan

This plan expands the "What remains to finish the SSA borrow checker" list in
`docs/ssa-checker-architecture.md` into an implementation order. The goal is to
finish the checker semantics before chasing editor tooling or broad compiler
ergonomics. Each milestone is TDD-first: add the smallest failing test that
captures the semantic promise, implement only enough to pass it, then refactor
the local machinery before continuing.

The plan assumes the current repository state:

- SSA is already built in `GownPackage.AnalyzeWithOptions`.
- The checker runner already prefers SSA passes.
- `GWN001` is the main SSA dataflow engine.
- `CapabilityIndex`, `PlaceIndex`, `SSAPlaceIndex`, `SSABindingIndex`, and
  source-position diagnostics already exist.
- Intrinsic tokens are scanned and formatted, but most intrinsic semantics are
  not implemented.
- Normal mode still writes stripped Go, not final semantically rewritten Go.

## Guiding Rules

1. **Keep the hybrid architecture.** AST/types identify source places and user
   intent; SSA provides ordering, liveness, closure/defer shape, and CFG joins.
2. **Prefer conservative rejection to false acceptance.** Any uncertain heap,
   container, interface, unsafe, reflection, or sync flow should collapse or
   frontier until modeled explicitly.
3. **Add tests before implementation.** For each step, first add a red test
   that fails for the missing behavior. If the code currently fails earlier
   during parsing/type-checking, keep that as the red signal until the plumbing
   exists.
4. **Refactor after each semantic slice.** Once a slice is green, clean shared
   APIs before starting the next slice. Do not let each checker grow private
   versions of capability adaptation, intrinsic binding, or diagnostics.
5. **Run focused tests by default.** Use targeted `go test -run ... ./` and
   package-specific tests while developing. Save `go test ./...` for larger
   milestones.

## Milestone 0: Baseline Inventory And Guardrails

Purpose: lock in the current checker behavior before changing semantics.

Red-first tests:

- Add `TestSSAPlanBaselineDocumentsBoundaryStubs` in a small docs-oriented or
  checker test file only if needed. It should assert that
  `checkUntrackedCallBoundariesSSA` and `checkInterfaceErasureSSA` currently
  return no direct hard-boundary errors, because proof-frontier behavior lives
  in `GWN001`.
- Add no broad behavioral changes yet. This milestone is mainly to prevent
  accidentally resurrecting the old hard-boundary model while working on
  `\unsafe`.

Implementation:

- If the tests above would be redundant with existing frontier tests, skip code
  changes and continue.
- Note any surprising baseline failures in this document before beginning the
  semantic work.

Refactor checkpoint:

- None unless a baseline test exposes duplicate or misleading helper names.

Focused verification:

```bash
go test -run 'TestSSAGWN012|TestGWN012|TestSSAGWN008|TestSSAGWN009' ./
```

## Milestone 1: Make Intrinsics Type-Check In Analysis Go

Purpose: allow programs containing expression intrinsics to reach the checker.
Currently `scanAndClassify` rewrites tokens such as `\freeze` to identifiers
such as `freeze_`, but those identifiers do not have analysis-time definitions.

Red-first tests:

- `annotations_test.go`
  - `TestAnalyzeIntrinsicCallsTypeCheckWithSyntheticHelpers`
  - Fixture uses `\mub(x)`, `\rob(x)`, `\freeze(x)`, `\clone(x)`,
    `\new(T{...})`, and `\unsafe(x)` in otherwise valid code.
  - Expected red failure before implementation: `packages.Load` reports
    undefined helper identifiers or type errors.
- `cmd/gown/gown_test.go`
  - `TestRunCheckAcceptsIntrinsicSyntaxForAnalysis`
  - Uses `run([]string{"-check", dir}, &stderr)` and asserts the package reaches
    checker diagnostics rather than failing as invalid Go.

Implementation:

- Add an analysis-only synthetic overlay file per package when any intrinsic is
  present. The file can define generic identity helpers:

```go
func mub_[T any](x T) T { return x }
func rob_[T any](x T) T { return x }
func freeze_[T any](x T) T { return x }
func clone_[T any](x T) T { return x }
func unsafe_[T any](x T) T { return x }
func new_[T any](x T) *T { return &x }
```

- Keep this helper file out of emit output and out of source diagnostics.
- Place helper generation near `AnalyzeWithOptions`, because it owns
  `go/packages` overlays.
- Avoid helper names that can collide with user code in normal source. If the
  current marker names are too ordinary, update scanner/formatter markers and
  tests together.

Refactor checkpoint:

- Extract helper-overlay construction into a small function such as
  `intrinsicAnalysisOverlay(files []*gownFile) map[string][]byte`.
- Keep scanner classification separate from package analysis.

Focused verification:

```bash
go test -run 'TestAnalyzeIntrinsic|TestRunCheckAcceptsIntrinsic' ./ ./cmd/gown
```

## Milestone 2: Bind Intrinsic Calls Into Side Tables

Purpose: turn parsed intrinsic calls into checker facts. This is plumbing only;
each individual intrinsic's semantic effect is implemented in later milestones.

Red-first tests:

- `capability_test.go`
  - `TestCapabilityIndexBindsIntrinsicCalls`
  - Fixture should include all intrinsic forms and assert an ordered side table
    of operation kind, path, offset, argument place, and result object when the
    result is assigned.
- `side_tables_test.go`
  - `TestIntrinsicBindingMapsGeneratedPositionToGownSource`
  - Ensures a call in analysis Go reports the original `.gown` path/position.

Implementation:

- Add types:

```go
type IntrinsicBinding struct {
    Kind IntrinsicKind
    Path string
    Offset int
    Line int
    Col int
    Call *ast.CallExpr
    Arg ast.Expr
    ArgPlace Place
    Result types.Object // optional, filled for direct assignment/value specs
}
```

- Add `IntrinsicBindings []IntrinsicBinding` and lookup maps to
  `CapabilityIndex`.
- During capability assignment, walk calls whose function names are the
  analysis helper names and bind them to scanner-recorded intrinsic tokens by
  generated file and offset.
- Add helpers similar to `CallBinding` and `SendBinding`:
  `idx.IntrinsicBinding(call)` and `NewSSAIntrinsicBindingIndex(caps)`.

Refactor checkpoint:

- Do not let every SSA checker rediscover intrinsic calls by helper name.
  Intrinsics should be source facts in `CapabilityIndex`.
- If `CallBinding`, `SendBinding`, and `IntrinsicBinding` begin duplicating
  position fields, extract a tiny source-position helper.

Focused verification:

```bash
go test -run 'TestCapabilityIndexBindsIntrinsic|TestIntrinsicBinding' ./
```

## Milestone 3: Explicit Borrow Intrinsics `\mub(x)` And `\rob(x)`

Purpose: implement expression-level borrow creation so named borrows can be
created without explicit local type annotations.

Red-first tests:

- `ssa_named_borrow_test.go`
  - `TestSSAExplicitMubBorrowBlocksIsoSend`
    - `b := \mub(x); ch <- x; _ = b`
    - Want `GWN002`.
  - `TestSSAExplicitRobBorrowBlocksIsoSend`
    - `r := \rob(x); ch <- x; _ = r`
    - Want `GWN002`.
  - `TestSSAExplicitBorrowDiesBeforeSend`
    - Borrow used and dead before send.
    - Want no error.
  - `TestSSAExplicitRobFromImmAllowed`
    - `r := \rob(imm); Inspect(r)`
    - Want no error.
  - `TestSSAExplicitMubFromImmRejected`
    - `b := \mub(imm)`
    - Want a checker error, preferably `GWN010` or a new coercion-specific code
      after error-code reconciliation.

Implementation:

- Extend capability binding so local variables initialized from `\mub(x)` or
  `\rob(x)` acquire the corresponding object cap even without an explicit local
  type annotation.
- Extend `collectSSANamedBorrows` so intrinsic borrow calls define named borrow
  sources exactly like `var b \mub *T = x`.
- Validate source compatibility with the existing `namedBorrowSourceAllowed`
  rules.
- Treat explicit borrow result values as non-sendable and non-escaping using
  the existing store/return/go/closure checks.

Refactor checkpoint:

- Unify local capability inference for explicit local annotations,
  `\mub`/`\rob` intrinsic results, fresh allocation, clone, freeze, and function
  result caps behind one helper. A likely shape is:
  `capabilityForValueExpr(pkg, caps, expr) (direct Cap, chanElem Cap, source Place)`.

Focused verification:

```bash
go test -run 'TestSSAExplicit.*Borrow|TestSSAGWN002|TestGWN002' ./
```

## Milestone 4: Fresh Ownership Intrinsics `\new` And `\clone`

Purpose: produce fresh `\iso` values from explicit constructors. This gives
programmers a clean on-ramp before implementing freeze and final emit.

Red-first tests:

- `capability_test.go`
  - `TestCapabilityIndexInfersNewIntrinsicAsIso`
    - `x := \new(T{...})`
    - Want `caps.ObjectCap(x) == CapIso`.
  - `TestCapabilityIndexInfersCloneIntrinsicAsIso`
    - `y := \clone(x)`
    - Want `caps.ObjectCap(y) == CapIso`, source `x` not consumed.
- `ssa_send_test.go`
  - `TestSSACloneSendDoesNotConsumeOriginal`
    - `ch <- \clone(x); println(x)`
    - Want no `GWN001`.
  - `TestSSANewSendConsumesFreshResultOnly`
    - `ch <- \new(T{...})`
    - Want no error and no attempt to nil a nonexistent source place.

Implementation:

- Extend local/result capability inference:
  - `\new(T{...})` returns `\iso *T`.
  - `\clone(x)` returns `\iso` and does not consume or frontier `x`.
- Extend send/call/result binding so capability-producing call expressions can
  satisfy `\iso` parameters or channel elements without needing an intermediate
  local variable.
- In `GWN001`, avoid consuming a source root for fresh-producing expressions
  that have no source owner.

Refactor checkpoint:

- Introduce a `ValueCapability` record for expressions:

```go
type ValueCapability struct {
    Cap Cap
    Place Place
    Fresh bool
    Source Place
}
```

- Make sends, calls, returns, and local inference ask for `ValueCapability`
  rather than open-coding object-cap lookups.

Focused verification:

```bash
go test -run 'Test.*NewIntrinsic|Test.*CloneIntrinsic|TestSSAClone|TestSSANew' ./
```

## Milestone 5: Explicit Freeze `\freeze(x)`

Purpose: implement `\iso -> \imm` conversion with the same exclusivity rules as
inferred freeze-send.

Red-first tests:

- `ssa_gwn001_test.go`
  - `TestSSAExplicitFreezeConsumesIso`
    - `y := \freeze(x); println(x)`
    - Want `GWN001`.
  - `TestSSAExplicitFreezeResultIsImm`
    - `y := \freeze(x); chImm <- y; println(y)`
    - Want no error.
  - `TestSSAExplicitFreezeRejectsLiveMubBorrow`
    - `b := \mub(x); y := \freeze(x); _ = b; _ = y`
    - Want `GWN002`.
  - `TestSSAExplicitFreezeRejectsFieldProjection`
    - `y := \freeze(h.Item)`
    - Want `GWN011`.
- `store_checker_test.go` or `ssa_store_test.go`
  - `TestSSAExplicitFreezeResultRejectsWrites`
    - `y := \freeze(x); y.Field = 1`
    - Want `GWN005`.

Implementation:

- Infer `\freeze(x)` result as `CapImm`.
- In `GWN001`, consume the source root at the freeze instruction using kind
  `"freeze"`.
- Reuse `consumeRootAtInstruction` so frontier checks, field projection
  rejection, and named-borrow liveness all apply.
- Allow `\freeze` only from proven `\iso`; reject `\mub`, `\rob`, `\imm`, and
  untracked sources.

Refactor checkpoint:

- Share one transfer path for:
  - `\iso` send to `chan \iso`
  - `\iso` send to `chan \imm`
  - `\iso` call to `\iso` parameter
  - assignment move
  - explicit freeze
  - later return moves/freezes

Focused verification:

```bash
go test -run 'TestSSAExplicitFreeze|TestGWN001|TestGWN002|TestGWN011' ./
```

## Milestone 6: Explicit Unsafe `\unsafe(x)`

Purpose: make unsafe boundaries explicit and auditable without reverting to the
old hard-boundary model.

Policy for first implementation:

- `\unsafe(x)` allows passing `x` to untracked or otherwise unknown code.
- It still ends the local proof for `x` unless a later spec decision says the
  annotation is an assertion strong enough to preserve proof. Ending the proof
  is conservative and fits the current frontier model.
- Later capability-required operations report `GWN012` with the frontier kind
  `"unsafe"`.

Red-first tests:

- `ssa_frontier_test.go`
  - `TestSSAUnsafeCallCreatesExplicitFrontier`
    - `Plain(\unsafe(x)); ch <- x`
    - Want `GWN012`, message mentions unsafe.
  - `TestSSAUnsafeCallAllowsOrdinaryUntrackedUse`
    - `Plain(\unsafe(x)); println(x)`
    - Want no capability error.
  - `TestSSAUnsafeDoesNotRequireUntrackedCall`
    - `_ = \unsafe(x); ch <- x`
    - Want `GWN012` after the explicit unsafe expression itself.
- `annotations_test.go`
  - Ensure `\unsafe` remains classified as `AnnotationUnsafeBoundary`.

Implementation:

- Add intrinsic binding for unsafe to `GWN001`.
- When encountered, call `EnterFrontier` with kind `"unsafe"`.
- Suppress duplicate untracked-call frontier messages if both `\unsafe(x)` and
  `Plain(...)` are observed at the same source operation.
- Do not hard-error in `checkUntrackedCallBoundariesSSA`.

Refactor checkpoint:

- Introduce `enterFrontier(place, kind, pos)` as a shared checker helper that
  can attach future notes.

Focused verification:

```bash
go test -run 'TestSSAUnsafe|TestSSAGWN012|TestGWN012' ./
```

## Milestone 7: Channel Receives

Purpose: bind receive results from capability-typed channels.

Red-first tests:

- `capability_test.go`
  - `TestCapabilityIndexInfersIsoReceiveLocal`
    - `x := <-chIso`
    - Want `x` cap `CapIso`.
  - `TestCapabilityIndexInfersImmReceiveLocal`
    - `x := <-chImm`
    - Want `x` cap `CapImm`.
- `ssa_checker_test.go` or `ssa_send_test.go`
  - `TestSSAReceiveIsoThenSendConsumesReceivedValue`
    - `x := <-chIso; out <- x; println(x)`
    - Want `GWN001`.
  - `TestSSAReceiveImmCanBeShared`
    - `x := <-chImm; out <- x; println(x)`
    - Want no moved-use error.
- `ssa_return_test.go`
  - `TestSSAReceiveBorrowCannotExist`
    - If `chan \mub`/`\rob` declarations are currently accepted, add a red test
      here or in a channel declaration checker milestone.

Implementation:

- Extend capability binding for `ValueSpec` and `AssignStmt` RHS receive
  expressions.
- If the channel element has `\iso` or `\imm`, bind the receiving local to that
  cap.
- Add SSA/source binding for receive expressions if SSA value identity is not
  enough.
- Keep untracked channel receives untracked.

Refactor checkpoint:

- The same local inference helper should handle function results, intrinsic
  results, receive results, fresh allocations, and direct moves.

Focused verification:

```bash
go test -run 'Test.*Receive|TestSSAReceive' ./
```

## Milestone 8: Select Semantics

Purpose: model Go `select` with capability-typed sends and receives.

Red-first tests:

- `ssa_inventory_test.go`
  - `TestSSAInventoryRecordsSelectInstruction`
    - Ensure `*ssa.Select` appears in inventory facts.
- `ssa_send_test.go`
  - `TestSSASelectIsoSendConsumesAfterSelect`
    - `select { case ch <- x: default: }; println(x)`
    - Want `GWN001`.
  - `TestSSASelectRepeatedSameIsoSendAllowedBeforePostUse`
    - `select { case ch1 <- x: case ch2 <- x: }`
    - Want no repeated-move error before any later use.
  - `TestSSASelectCloneSendDoesNotConsumeOriginal`
    - `select { case ch <- \clone(x): default: }; println(x)`
    - Want no error.
  - `TestSSASelectImmSendRetainsValue`
    - `select { case chImm <- y: default: }; println(y)`
    - Want no error.
- `capability_test.go`
  - `TestCapabilityIndexInfersSelectReceiveResult` if Go's AST/SSA shape gives
    a recoverable assignment site for selected receives.

Implementation:

- Add `Select` to SSA inventory.
- Implement a conservative first transfer:
  - Any select state containing a direct `\iso` send of a source place marks
    that source consumed after the select.
  - Multiple cases sending the same root do not report duplicate move by
    themselves, because runtime selects at most one case.
  - Sending a fresh expression such as `\clone(x)` or `\new(...)` does not
    consume `x`.
  - `\imm` sends do not consume.
- Use source AST `SelectStmt` if SSA `Select` does not expose enough source
  expression identity for precise place mapping.

Refactor checkpoint:

- If select handling needs source side tables, create `SelectBinding` rather
  than stuffing select cases into send bindings.

Focused verification:

```bash
go test -run 'TestSSA.*Select|Test.*Select' ./
```

## Milestone 9: Full Viewpoint Adaptation

Purpose: make effective field capabilities match the spec's meet matrix rather
than simply choosing a tracked field cap or the root cap.

Red-first tests:

- `side_tables_test.go` or a new `viewpoint_test.go`
  - `TestViewpointIsoUsesDeclaredFieldCapability`
  - `TestViewpointMubForcesMutableBorrowOnIsoField`
  - `TestViewpointRobForcesReadOnlyOnMutableField`
  - `TestViewpointImmForcesImmutableOnUntrackedField`
  - `TestViewpointUntrackedCannotProduceIsoOrMubFieldProof`
- `ssa_store_test.go`
  - `TestSSAWriteThroughRobRootToMubFieldRejected`
  - `TestSSAWriteThroughImmRootToUntrackedFieldRejected`
- `ssa_send_test.go`
  - `TestSSASendFieldThroughRobRootRejectedAsNonSendable`
  - `TestSSASendImmRootFieldOnImmChannelAllowed`

Implementation:

- Add a shared effective-capability function:

```go
func EffectivePlaceCap(caps *CapabilityIndex, place Place) Cap
```

- Fold from root to each projection using the spec matrix:
  - `\iso` root exposes declared field capability.
  - `\mub` root downgrades `\iso` fields to `\mub` but preserves `\rob` and
    `\imm`.
  - `\rob` root exposes every reachable field as `\rob`.
  - `\imm` root exposes every reachable field as `\imm`.
  - untracked root cannot prove `\iso`/`\mub` fields; `\rob`/`\imm` fields may
    be used as declared.
- Replace `capForSSAPlace` and AST fallback direct `ObjectCap(root)` uses where
  the expression is projected.

Refactor checkpoint:

- Centralize capability adaptation. No checker should decide field effective
  caps by hand.
- Add table-driven unit tests for the meet matrix independent of package load.

Focused verification:

```bash
go test -run 'TestViewpoint|TestSSAWriteThrough|TestSSASendField' ./
```

## Milestone 10: Return And Result Ownership Semantics

Purpose: treat returns as ownership transfers or freezes, not just borrow
escape sites.

Red-first tests:

- `ssa_return_test.go`
  - `TestSSAReturnIsoResultMovesSourceBeforeDeferredUse`
    - `defer func(){ println(x) }(); return x` from `func f(x \iso *T) \iso *T`
    - Want `GWN001`.
  - `TestSSAReturnIsoResultRejectsLiveNamedBorrow`
    - `b := \mub(x); return x; _ = b`
    - Want `GWN002`.
  - `TestSSAReturnImmResultFreezesIso`
    - `return x` from `func f(x \iso *T) \imm *T`
    - Want source consumed/freeze semantics; with deferred later use, want
      `GWN001`.
  - `TestSSAReturnImmResultAllowsImmSource`
    - `return y` where `y \imm *T`
    - Want no move.
  - `TestSSAReturnTrackedResultAfterFrontierRejected`
    - Already covered for frontier; extend with `\imm` result if missing.
- `capability_test.go`
  - `TestCapabilityIndexInfersLocalFromFunctionIsoResult`
  - `TestCapabilityIndexInfersLocalFromFunctionImmResult`

Implementation:

- Extend call-result capability propagation into local inference if not already
  complete.
- In `GWN001.applyReturnTransfer`, for each result:
  - `\iso` result requires proven `\iso` source or fresh `\iso` expression and
    consumes the source root before deferred closures run.
  - `\imm` result accepts `\imm` source without consumption; accepts `\iso`
    source as inferred freeze and consumes it; rejects `\mub`, `\rob`, and
    untracked.
  - `\mub`/`\rob` result remains rejected by return-borrow checks.
- Reuse named-borrow liveness and frontier checks.
- Handle named result variables conservatively.

Refactor checkpoint:

- Pull send/call/return transfer rules into shared helpers:
  `requireIsoTransfer`, `requireImmShareOrFreeze`, `applyCapabilityResult`.

Focused verification:

```bash
go test -run 'TestSSAReturn|TestGWN007|TestGWN012' ./
```

## Milestone 11: Heap, Container, Interface, And Unknown Alias Flow

Purpose: make escaping alias behavior explicit and conservative.

Red-first tests:

- `ssa_store_test.go`
  - `TestSSAIsoStoreToGlobalCreatesFrontier`
  - `TestSSAIsoStoreToFieldCreatesFrontier`
  - `TestSSAImmStoreToGlobalAllowedOrFrontieredByPolicy`
  - `TestSSABorrowStoreToSliceRejected`
  - `TestSSAIsoStoreToMapCreatesFrontier`
- `ssa_interface_test.go`
  - `TestSSAInterfaceErasureFrontierIncludesFieldProjection`
  - `TestSSATypeAssertFromInterfaceIsUntracked`
- `ssa_frontier_test.go`
  - `TestSSAContainerFrontierRejectsLaterIsoSend`

Implementation:

- Decide per capability:
  - `\mub`/`\rob` stored into escaping heap/container remains hard error
    (`GWN006`).
  - `\iso` stored into an escaping location should either be a move to an
    owned field/container with tracked declaration or a proof frontier. First
    implementation should frontier unless the target is a tracked `\iso` field.
  - `\imm` store is safe to share but may still require effective immutability
    through viewpoint adaptation. Prefer allow when target does not weaken it.
- Extend `ssaStoreChecker` and/or `GWN001` frontier transfer for stores.
- Treat dynamic containers as root collapse or frontier.
- Ensure interface erasure always uses `GWN001` frontier state, not hard
  `GWN009`, for ordinary values.

Refactor checkpoint:

- Extract a boundary policy module:
  `boundaryEffectForStore`, `boundaryEffectForInterface`, `boundaryEffectForUnknownCall`.

Focused verification:

```bash
go test -run 'TestSSA.*Store.*Frontier|TestSSA.*Interface|TestSSAContainer' ./
```

## Milestone 12: Interprocedural Summaries

Purpose: avoid either missing cross-function flows or rejecting too much once
heap/container/interface behavior becomes visible.

Red-first tests:

- New `ssa_summary_test.go`
  - `TestSSASummaryIsoParamReturnedAsIsoConsumesAtCall`
    - Callee returns its `\iso` parameter; caller assignment moves ownership.
  - `TestSSASummaryBorrowDoesNotEscapeForSynchronousCall`
    - Callee takes `\mub`, does not store/return it; caller can later send.
  - `TestSSASummaryBorrowStoreRejectsAtCallSite`
    - Callee stores `\mub` param to global; caller sees error when calling.
  - `TestSSASummaryClosureReturnCapturesBorrowRejectsAtCallSite`
  - `TestSSASummaryRecursiveFunctionConservative`

Implementation:

- Start with package-local summaries only.
- Summary fields:

```go
type FuncSummary struct {
    ParamEffects []ParamEffect // consumed, borrowed, frontiered, stored, returned
    ResultSources []ResultSource
    CapturedEscapes []Place
    UnsafeBoundary bool
    Conservative bool
}
```

- Compute summaries to a fixpoint over package functions.
- Use summaries at call transfer points when callee body is available.
- For imported/unavailable functions, keep current signature/frontier behavior.

Refactor checkpoint:

- Keep summaries optional. The current signature-based checking must remain
  understandable and safe without summaries.

Focused verification:

```bash
go test -run 'TestSSASummary' ./
```

## Milestone 13: Frontier Diagnostics With Related Notes

Purpose: make proof-frontier errors explain both the later invalid operation
and the earlier proof-ending event.

Red-first tests:

- `checker_errors_test.go`
  - `TestFormatErrorIncludesRelatedNotes`
  - Expected output includes primary `GWN012` line plus a note pointing to the
    untracked call/interface/unsafe/store frontier.
- `ssa_frontier_test.go`
  - `TestGWN012ReportsFrontierNoteForUntrackedCall`
  - `TestGWN012ReportsFrontierNoteForInterfaceErasure`
  - `TestGWN012ReportsFrontierNoteForUnsafe`

Implementation:

- Extend `CheckerError` with optional related notes:

```go
type CheckerNote struct {
    Path string
    Offset int
    Line int
    Col int
    Message string
}
```

- Update `FormatError` to render notes after the primary source line.
- Update `SSAFrontierSite` to carry enough source information to create notes.
- Preserve compatibility for tests that only check `Code`, `Line`, and primary
  message.

Refactor checkpoint:

- Centralize note construction in `checker_errors.go`; do not format notes in
  individual checkers.

Focused verification:

```bash
go test -run 'TestFormatErrorIncludesRelatedNotes|TestGWN012ReportsFrontierNote' ./
```

## Milestone 14: Correct Emit For Moves, Freeze, Clone, And Intrinsics

Purpose: emit plain Go that enforces consumed `\iso` values at runtime and
erases or lowers intrinsics correctly.

Red-first tests:

- New `emit_test.go`
  - `TestEmitNilAfterIsoSend`
    - Input `ch <- x`; output includes `ch <- x` followed by `x = nil`.
  - `TestEmitNilAfterIsoCall`
  - `TestEmitNilAfterIsoAssignmentMove`
  - `TestEmitNilAfterDeferIsoCall`
  - `TestEmitFreezeAssignment`
    - `y := \freeze(x)` becomes `y := x` plus `x = nil`.
  - `TestEmitMubRobEraseToPlainAssignment`
  - `TestEmitUnsafeErasesToExpression`
  - `TestEmitCloneUsesGeneratedCloneHelperOrConfiguredClone`
  - `TestEmitDoesNotNilFreshCloneOrNewSource`
  - `TestEmitPreservesLineMappingEnoughForGoCompilerErrors`
- `cmd/gown/gown_test.go`
  - `TestRunNormalModeWritesSemanticGo`
  - `TestRunCheckDoesNotWriteSemanticGo`

Implementation:

- Split `AnalyzeWithOptions` responsibilities:
  - analysis view for packages/checker
  - emit planning
  - final emit writing
- Build an `EmitPlan` from checker side tables and, where needed, SSA transfer
  sites:

```go
type EmitEdit struct {
    Path string
    Start int
    End int
    NewText string
    Reason string
}
```

- For simple statement-level moves, insert `x = nil` immediately after the
  statement.
- For branches/select/deferred closure situations, prefer conservative and
  source-local insertion. If no correct insertion point exists, reject with a
  clear checker error rather than emitting unsound Go.
- Lower `\freeze(x)` as expression replacement plus nil insertion.
- Lower `\mub(x)`, `\rob(x)`, and `\unsafe(x)` to `x`.
- Lower `\new(T{...})` to `&T{...}`.
- For `\clone(x)`, start with a generated helper hook or require a configured
  user clone function; do not silently shallow-copy pointer graphs.

Refactor checkpoint:

- Keep emit edits separate from checker diagnostics. The checker should decide
  legality and record move sites; the emitter should only rewrite legal
  programs.
- Ensure `-check` never writes emit output.

Focused verification:

```bash
go test -run 'TestEmit|TestRunNormalModeWritesSemanticGo|TestRunCheckDoesNotWrite' ./
```

## Milestone 15: Spec And Error-Code Reconciliation

Purpose: make docs match implementation so future work does not fight stale
contracts.

Red-first tests:

- Optional doc consistency test if desired:
  - `TestDocumentedGWNErrorCodesMentionCurrentCodes`
  - It can be a simple grep-style test over `gown-spec.md`, but do not overdo
    this if it becomes brittle.

Implementation:

- Update `gown-spec.md`:
  - Replace the old error-code table with current codes:
    - `GWN001` moved use
    - `GWN002` conflicting borrows / live borrow blocks move
    - `GWN003` non-sendable send
    - `GWN004` goroutine borrow escape
    - `GWN005` read-only/immutable write
    - `GWN006` borrow store escape
    - `GWN007` returned borrow / escaping returned closure
    - `GWN008` closure capturing non-shareable tracked value passed to
      untracked call, or retire if we choose a new code
    - `GWN009` historical interface-erasure hard boundary, likely retired
    - `GWN010` channel/value capability mismatch
    - `GWN011` field projection ownership move
    - `GWN012` proof frontier violation
  - Rewrite `\unsafe` semantics to match the chosen frontier policy.
  - Add explicit wording for named local borrows via annotated locals and for
    expression-level borrow intrinsics once implemented.
  - Clarify receive/select status once milestones 7 and 8 land.
- Update `docs/ssa-checker-architecture.md` to point to this implementation
  plan or fold completed milestones back into the current-state section.

Refactor checkpoint:

- Remove or rename stale tests whose names imply hard `GWN008/GWN009` boundary
  errors when the expected behavior is now frontier-based.

Focused verification:

```bash
go test -run 'TestDocumentedGWN|TestFormatError' ./
```

## Suggested Overall Order

1. Baseline guardrails.
2. Analysis-time intrinsic helpers.
3. Intrinsic side-table binding.
4. Explicit `\mub`/`\rob` named borrow creation.
5. `\new` and `\clone` fresh ownership.
6. Explicit `\freeze`.
7. Explicit `\unsafe` frontier.
8. Channel receives.
9. Select semantics.
10. Full viewpoint adaptation.
11. Return/result ownership transfer and inferred return-freeze.
12. Heap/container/interface frontier and alias policies.
13. Package-local interprocedural summaries.
14. Frontier diagnostics with related notes.
15. Correct final emit.
16. Spec and error-code reconciliation.

The order intentionally implements source-expression semantics before receive,
select, and return. That gives later transfer logic a single way to ask, "what
capability does this expression produce, and does it have a source root that
must be consumed or frontiered?" Without that shared expression model, receive,
select, return, and emit will each grow their own half-checker.

## Refactoring Themes To Apply Throughout

- **Expression capability API:** create one shared way to compute the effective
  capability of an expression or place. This should subsume fresh expressions,
  intrinsic results, function results, channel receives, field viewpoint
  adaptation, and root object caps.
- **Transfer API:** create one shared way to perform root moves, inferred
  freezes, frontier entry, and live-borrow checks. Sends, calls, returns,
  assignment moves, explicit freeze, goroutine calls, and deferred effects
  should all use it.
- **Source binding indexes:** keep source side tables for calls, sends,
  receives, selects, intrinsics, returns, and assignments when SSA value identity
  is too lossy.
- **Diagnostics:** keep primary diagnostics at the operation the user must fix,
  and attach related notes for earlier frontiers or source qualifiers when that
  helps.
- **Emit planning:** do not emit directly from checker loops. Record legal move
  and intrinsic-lowering sites, then apply text edits in a separate emit pass.
- **Fallback checkers:** keep AST fallback paths until SSA has enough soak time,
  but do not add new semantics to AST-only code unless it is cheap and clearly
  useful for fallback correctness.

## Completion Criteria

The SSA borrow checker is "feature complete enough for research validation"
when all of these are true:

- Programs using qualifier annotations, named local borrows, explicit
  `\mub`/`\rob`, `\new`, `\clone`, `\freeze`, and `\unsafe` reach the checker
  and receive meaningful diagnostics.
- Sends, receives, select sends, calls, goroutine calls, defers, deferred
  closures, returns, stores, interface erasure, and untracked calls all use one
  coherent transfer/frontier model.
- Viewpoint adaptation matches the spec matrix for statically recoverable
  fields.
- `GWN012` reports both the invalid later operation and the earlier proof
  frontier.
- Normal `gown` output is semantically rewritten Go with nil insertion after
  consumed `\iso` moves; `gown -check` remains non-writing.
- The architecture document and language spec no longer contradict the current
  implementation.
