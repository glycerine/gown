# Gown SSA Checker Architecture

This document describes the checker architecture for Gown's lifetime and borrow
analysis. It is a living design artifact: some pieces are implemented, some are
only partially implemented, and the SSA checker is now the main execution path
for the completed diagnostic slices. The remaining highest-risk work is now
polishing clone emit policy, broader standard-library/synchronization boundary
modeling, and deciding how much interprocedural precision is needed beyond the
current signature and frontier rules.

The central decision is that Go's AST, type checker, and SSA builder remain
ordinary Go tooling. They do not learn about `\iso`, `\mub`, `\rob`, or
`\imm`. Gown carries capability information in side tables keyed by source
offsets, AST nodes, `types.Object`s, function signatures, call sites, SSA
values/instructions, and source places.

The user-facing direction is inference-first. Gown should require the minimum
annotations needed to state ownership boundaries: function signatures, channel
element types, and struct fields. Ordinary borrows should be inferred from
typed context, such as calling a function whose parameter is declared
`\mub *T` or `\rob *T`. Explicit expression forms like `\mub(x)` and
`\freeze(x)` are now supported as leverage points when the programmer wants to
state a longer-lived borrow or freeze directly.

## Current State

The current implementation now has a working front end, side-table binding
layer, SSA construction step, and an SSA-backed checker runner for the current
capability diagnostics. The older AST/place checkers remain useful fallback
implementations and comparison points, but `GownPackage.Check` now builds SSA
and routes the main checker passes through the SSA implementations when SSA is
available.

Implemented:

- `scanAndClassify` recognizes capability qualifiers and explicit intrinsic
  forms, records byte offset/line/column, and produces source views where
  Go parser positions still line up for qualifiers.
- `Check` analyzes generated Go through a `go/packages` overlay, retains
  AST/types information, builds SSA with `golang.org/x/tools/go/ssa`, and only
  writes final emitted `.go` after all checker passes succeed.
- `CapabilityIndex` binds qualifier annotations to `types.Object`s, function
  signatures, channel element types, struct fields, call sites, send sites,
  and source places.
- `PlaceIndex` recovers source places from AST/types, including statically
  typed selector projections such as `x.f.g`; dynamic index expressions
  collapse to the root region.
- SSA-backed checker passes currently reject several capability violations:
  moved `\iso` use, conflicting inferred call borrows, non-sendable sends,
  channel/value capability mismatch, inferred freeze-on-send to `chan \imm`,
  goroutine borrow escapes, escaping closures that capture non-shareable
  tracked values, read-only writes, borrow stores, and returned borrows. They
  also track proof frontiers when capability-tracked values reach untracked
  calls, untracked parameters, explicit `\unsafe`, stores, map updates, or
  interface erasure, then reject later operations that require the old proof.
- Expression-level `\mub`, `\rob`, `\freeze`, `\new`, `\clone`, and
  `\unsafe` are bound into side tables. Borrow/freeze/unsafe intrinsics have
  checker transfer semantics; `\new` and `\clone` produce source-less `\iso`
  values for the current function.
- Channel receives, select sends, return/result ownership transfers, and the
  full viewpoint adaptation matrix are covered by focused SSA tests.
- Diagnostics report structured `GWN` errors against original `.gown` source
  and include source-line context. `GWN012` frontier diagnostics carry related
  notes that point back to the earlier proof-ending operation.
- CLI `-check` mode validates with a `go/packages` overlay and does not write
  generated `.go` files into the package directory.
- Normal mode emits semantic Go for the current straightforward cases:
  capability qualifiers are erased, borrow/unsafe/freeze intrinsics lower to
  their argument, `\new(T{...})` lowers to `&T{...}`, `\clone(x)` lowers to
  `(x).Clone()` after same-type `Clone` validation, and direct consumed `\iso`
  sends/calls/assignments/defers/freezes insert `x = nil`.
- `gownfmt` formats `.gown` source through `go/format` while preserving Gown
  annotations.

Still missing or deliberately conservative:

- `\clone(x)` currently type-checks as a fresh source-less `\iso` and emits to
  `(x).Clone()` when the exact static argument type is a named struct `T` with
  `Clone() T`, or `*T` with `Clone() *T`. This is a trusted user proof that the
  method returns an independent value.
- Heap, container, interface, and interprocedural alias flow are conservative:
  `\iso` stores to escaping locations become proof frontiers, borrows stored
  into escaping locations remain hard errors, and function summaries are still
  mostly represented by signatures plus existing closure/store checks.
- Broader boundary modeling for reflection, `sync`, atomics, and unsafe code.
- Emit insertion is source-local and handles straightforward statement moves;
  branch-sensitive or complex insertion sites should reject rather than emit
  unsound Go until an explicit emit plan records safe insertion points.

This architecture now treats the hybrid AST/types/place plus SSA model as the
main checker shape rather than just a front-end scaffold.

## May 2026 Repository Snapshot

This update is based on a repository sweep of the current source tree: scanner,
formatter, capability binding, AST fallback checkers, SSA checkers, tests,
vectors, CLI code, the language spec, and the existing architecture notes. The
important conclusion is that the SSA borrow checker is no longer speculative.
The main checker runner builds SSA unconditionally after `go/packages` load and
prefers SSA passes whenever `SSAPkg` is available.

What is solid:

- `checker_runner.go` routes all current checker families through SSA-backed
  implementations first: moved-use/dataflow, call-borrow conflicts, sends,
  goroutine escapes, closure escapes, stores, returns, untracked-call
  frontiers, and interface-erasure frontiers.
- `ssa_gwn001.go` is the real lifetime engine. It runs a forward worklist over
  SSA basic blocks, tracks consumed roots, proof frontiers, active borrows, and
  deferred effects, merges state conservatively at CFG joins, and reuses the
  same engine for deferred closure bodies at function exit.
- `ssa_named_borrow.go`, `ssa_defer_effect.go`, and `ssa_state.go` cover the
  hard borrow-lifetime cases already validated by tests: named local
  `\mub`/`\rob` borrows, branch and loop liveness, deferred borrow extension,
  deferred closure LIFO behavior, and repeated deferred `\iso` moves.
- `ssa_place.go` proves the hybrid model. AST/types seed source places;
  `ssa.GlobalDebug` and `ValueForExpr` connect those places to SSA values; SSA
  then propagates places through `FieldAddr`, `UnOp`, `Store`, and conservative
  collapse points such as `IndexAddr`, `Lookup`, and `MakeInterface`.
- Field-sensitive place keys and overlap checks exist. They distinguish
  sibling fields for borrows and reject ownership moves from field projections
  with `GWN011`.
- Proof frontiers are implemented as `GWN012` inside the SSA moved-use engine:
  untracked calls, untracked parameters in otherwise annotated calls, and
  interface erasure end the local proof without rejecting the boundary itself.
  Later capability-required operations reject the value.
- Closure escape checking is partly source-flow-sensitive. It tracks local
  function-valued variables, assignment replacement, branch joins, returned
  closures, stored closures, and closures passed to untracked calls.
- The test suite has broad focused coverage for the SSA path: inventory,
  place propagation, state merges, direct sends/calls/assignments, branches,
  loops, named borrows, defers, deferred closure CFGs, go statements, stores,
  returns, closure escapes, proof frontiers, and diagnostic mapping back to
  `.gown` source.

What remains to finish the SSA borrow checker:

- Expand clone policy only if needed. The current same-type `Clone` hook is
  intentionally small and auditable; future work may add generated clone helpers
  or broader clone contracts.
- Refine heap/container/interface precision. The current model is sound and
  conservative: escaping `\iso` stores become frontiers, borrow stores hard
  error, map stores frontier, and interface erasure frontiers. More precision
  needs a boundary policy that understands owned fields, containers, and
  interface round trips.
- Decide whether package-local function summaries are worth implementing
  beyond current signature/call-site behavior. Current tests cover the key
  summary-shaped cases, but true summaries would improve diagnostics and avoid
  over-frontiering once heap/container policies become richer.
- Expand emit planning from local edits to recorded transfer sites. Current
  emit handles straightforward statement-level moves and intrinsic lowering;
  complex CFG-sensitive insertion should use an explicit `EmitPlan` populated
  by checker transfer records.
- Broaden unknown-boundary modeling for reflection, `sync`, atomics, cgo, and
  explicit unsafe code.

## End-To-End Pipeline

The long-term checker should use two generated source views:

1. **Analysis Go:** valid Go fed to `go/packages` and `go/ssa`.
2. **Emit Go:** final plain Go written beside `.gown`, with annotations erased
   and move/freeze nil assignments inserted.

The target pipeline is:

1. Scan `.gown` and record every Gown token as an `Annotation`.
2. Classify each token. The mainline syntax is capability type qualifiers.
   Explicit unsafe and intrinsic-like expression forms are scanned into side
   tables and rewritten to analysis-only placeholders.
3. Produce analysis Go:
   - Replace capability type qualifiers with spaces.
   - Leave ordinary Go expressions unchanged.
   - Rewrite recognized explicit expression forms to analysis-only
     placeholders when they are present.
4. Load analysis Go with `go/packages`.
5. Bind annotations back to AST nodes, `types.Object`s, function signatures,
   channel element types, struct fields, and call sites.
6. Build SSA with `golang.org/x/tools/go/ssa`.
7. Seed checker state from capability side tables.
8. Infer borrow/move/freeze behavior from typed contexts and run capability
   checking. Today the completed diagnostics use SSA-backed passes over the
   side tables, with full forward CFG dataflow currently concentrated in
   `GWN001`, including named borrow liveness and deferred borrow snapshots.
9. Report structured GWN errors at original `.gown` positions.
10. If checking succeeds and `-check` is false, emit final Go.

The analysis Go and emit Go views should be produced from the same annotation
index so they cannot drift.

Current status:

- Stages 1 through 7 exist for qualifier-oriented programs.
- Stage 8 exists as SSA-backed checker passes over capability side tables, with
  full CFG dataflow currently concentrated in `GWN001`, including named borrow
  liveness and deferred borrow snapshots.
- Stage 9 exists.
- Stage 10 partially exists: stripped Go is written in normal mode and avoided
  in CLI `-check` mode through a `go/packages` overlay. Nil insertion after
  consumed `\iso` moves remains open.

## Analysis Source Strategy

Type qualifiers remain position-preserving whitespace replacements:

| Gown token | Analysis Go |
| --- | --- |
| `\iso` | four spaces |
| `\mub` | four spaces |
| `\rob` | four spaces |
| `\imm` | four spaces |

Most user code should not need expression-level Gown syntax. Borrow creation is
usually inferred from callee parameter capabilities:

```go
func Mutate(x \mub *T) {}
func Inspect(x \rob *T) {}

func Use(x \iso *T) {
    Mutate(x)  // inferred temporary mutable borrow for the call
    Inspect(x) // inferred temporary read borrow for the call
}
```

The analysis Go for this example is simply ordinary Go with qualifier bytes
erased; no fake `mub_` or `rob_` call is needed.

Explicit expression forms may still be useful for operations that cannot be
inferred safely, such as `\unsafe(x)`, `\clone(x)`, or explicit freeze. The
scanner and formatter already recognize these forms, and `scanAndClassify`
rewrites them to analysis-only placeholders. Their checker semantics are not
yet implemented and they should not drive the mainline inference milestones.

## Capability Side Tables

Capabilities should be indexed outside Go types.

Core model:

```go
type Cap uint8

const (
    CapInvalid Cap = iota
    CapIso
    CapMub
    CapRob
    CapImm
    CapUntracked
)

type Annotation struct {
    Kind   AnnotationKind
    Cap    Cap
    Offset int
    Line   int
    Col    int
}
```

`CapabilityIndex` should bind annotations to checker facts:

- `types.Object` capabilities for variables, parameters, results, and struct
  fields.
- Function signature capabilities for parameters and results.
- Channel element capabilities for `chan \iso *T` and `chan \imm *T`.
- Call-site bindings that record the callee's expected parameter/result
  capabilities, enabling inferred temporary borrows and ownership moves.
- Explicit-operation annotations for expression syntax such as `\unsafe(x)`,
  `\clone(x)`, `\freeze(x)`, `\mub(x)`, and `\rob(x)`.
- Source locations for diagnostics in original `.gown` files.

SSA values are not enough to model Gown ownership. The checker must also track
source places.

```go
type Place struct {
    Root       types.Object
    Projection Projection
    Collapsed  bool
}

type Projection []FieldStep

type FieldStep struct {
    Index int
    Name  string
    Type  types.Type
}
```

Examples:

- `x` is root `x` with empty projection.
- `x.f` is root `x` with projection `.f`.
- `x.f.g` is root `x` with projection `.f.g`.
- `x.slice[i]` is root `x` with `Collapsed=true`.

`Place` is the unit for source-level diagnostics and variable consumption.
`RegionID` is the unit for borrow conflicts. For a field-sensitive region,
`x.f` and `x.g` may be separate sibling regions. For a collapsed place, the
region is the root.

## Field Sensitivity Feasibility

Field-sensitive tracking can work for statically recoverable struct selector
paths. It should not be attempted for dynamic container access or erased
interface paths.

The validation probe against `x/tools/go/ssa` showed these useful patterns:

- Pointer-root field access such as `x.F.G` becomes a chain of
  `*ssa.FieldAddr` instructions, using stable field indexes, followed by
  `*ssa.UnOp` loads when a value is read.
- Field writes such as `x.F.G = v` become `*ssa.Store` to a `FieldAddr` chain.
- `&x.F.G` is represented by the address value produced by the final
  `FieldAddr`.
- Slice indexing uses `*ssa.IndexAddr`; map lookup uses `*ssa.Lookup`.
  These should collapse to the root region.
- Closure captures appear through `*ssa.MakeClosure` bindings; `go` statements
  appear as `*ssa.Go`.

The checker should recover places primarily from AST/types and then propagate
them through SSA:

- At annotated call boundaries, bind argument expressions like `Mutate(x.f)`
  directly from the AST selector chain and `types.Selection`; the callee
  signature supplies whether the argument is treated as `\mub`, `\rob`,
  `\iso`, or `\imm`.
- In SSA transfer, propagate places through `FieldAddr` when the base value has
  a known place and the field index is a statically typed struct field.
- Propagate loads of pointer-typed fields as the place of the loaded pointer
  value when the field itself is capability-tracked.
- Treat `IndexAddr`, `Lookup`, `MakeInterface`, `TypeAssert`, reflection,
  unknown calls, and explicit unsafe boundaries as root-collapse or poison
  points.

Decision rule:

> Support field-sensitive tracking only for statically recoverable struct field
> projections. Otherwise collapse to the root region.

This allows precision where Go gives stable selector information and preserves
soundness everywhere else.

## Hybrid Region Model

Regions form a tree rooted at an owning place:

```text
x
  x.f
    x.f.g
  x.h
```

The conflict model is ancestor/descendant based:

- A borrow of `x.f` conflicts with transfers/freezes of `x`.
- A borrow of `x.f.g` conflicts with transfers/freezes of `x` and `x.f`.
- A borrow of `x.f` does not necessarily conflict with access to sibling
  `x.h`, as long as no root-level operation is being performed.
- Any collapsed access under `x` pins the root region `x`.

For the first implementation milestones, the checker may use root-only
regions internally while preserving the `Place` and `Projection` data model.
That keeps the first slice small and leaves a direct path to field-sensitive
precision.

## Checker State

The checker should keep function-local dataflow state:

```go
type PlaceState struct {
    Cap      Cap
    Region   RegionID
    Consumed bool
}

type BorrowState struct {
    Owner      Place
    MutBorrow  *Place
    Shared     map[Place]struct{}
    Imm        bool
    Collapsed  bool
}

type FunctionState struct {
    Places  map[Place]PlaceState
    Values  map[ssa.Value]Place
    Borrows map[RegionID]BorrowState
}
```

`Values` maps SSA values to source places when known. It is deliberately
secondary. A source variable may be consumed even if SSA still has a value that
can be printed or passed around; Gown's linearity is source-place semantics.

## SSA Dataflow

The checker should run a forward dataflow over each `ssa.Function`:

- Entry state is seeded from function parameter capabilities and captured
  free variables.
- Each basic block executes transfer functions in SSA instruction order.
- Instruction-level liveness is required so explicit and implicit borrows can
  end at last use.
- CFG joins use conservative merges.

Required transfer handlers:

- `Call`: apply inferred borrow/move/share behavior from callee signature
  metadata. When a capability-tracked value flows to an untracked callee or
  untracked parameter, record a proof frontier rather than failing immediately.
  Later operations that require a proven capability must reject the value.
- `Send`: validate channel element capability; consume `\iso` sends; reject
  non-sendable `\mub` and `\rob`. Ownership moves are root-only: sending
  `x.f` as an owned move is rejected rather than silently clearing or rewriting
  the user's field.
- `Go`: validate arguments and closure bindings; consume captured `\iso`;
  reject borrow captures.
- `Store`: reject writes through `\rob`/`\imm`; reject heap stores of borrows.
- `Return`: reject returned borrows; consume/move `\iso` results where the
  signature requires ownership transfer.
- `FieldAddr`: append field projection when statically recoverable.
- `IndexAddr`, `Lookup`, interface operations, and unknown operations:
  collapse or poison the root region conservatively.

Merge rules:

- If a place is consumed on any predecessor, treat it as unavailable after the
  join unless every predecessor carries the same live capability.
- Region states merge by unioning live shared borrows.
- Different live mutable borrows for the same region are an error.
- Unknown or incompatible locations collapse to the root region rather than
  inventing precision.

## Soundness Contract

The checker must ensure each accepted operation corresponds to the proof's
ownership steps:

- `\iso` send, spawn, move, and freeze require no live conflicting alias in
  the relevant region.
- Root-level transfer/freeze of `x` requires no live borrow in `x` or any
  descendant region.
- `\mub` and `\rob` are non-sendable and must not escape through returns,
  heap stores, closures, goroutines, interfaces, or unknown calls.
- Writes through `\rob` or `\imm` are rejected at every recoverable depth.
- Passing capability-tracked values to untracked code or into interfaces ends
  the local proof for that place. The boundary itself is not a hard error
  during incremental adoption, but later `\iso`, `\mub`, `\rob`, or `\imm`
  operations must not pretend the capability is still proven.
- Reflection, `sync`, atomics, and unsafe operations are treated as boundaries
  until explicitly modeled.

Conservative false rejections are acceptable. False acceptance is not.

## Examples

### `\iso` Send And Use After Send

```go
func f(ch chan \iso *T, x \iso *T) {
    ch <- x
    println(x) // GWN001: x consumed by send
}
```

Root-only and field-sensitive region models both treat `x` as consumed after
the send. This is the recommended first vertical slice.

### Direct Borrow Blocks Transfer

```go
func Mutate(x \mub *T) {}

func f(ch chan \iso *T, x \iso *T) {
    Mutate(x) // temporary inferred borrow, dead after call
    ch <- x   // ok if no other borrow remains live
}
```

Temporary call borrows live for the synchronous call duration only. They do not
block a later send once the call returns.

Named local borrows exist today when a local variable is explicitly annotated
and initialized from an allowed source, and also when a local is initialized
from expression-level `\mub(x)` or `\rob(x)`. The explicit intrinsic form:

```go
func f(ch chan \iso *T, x \iso *T) {
    b := \mub(x)
    ch <- x // error: active borrow in region x
    _ = b
}
```

The borrow pins region `x`. Sending `x` requires the active region to contain
only the root owner, so the send is rejected.

### Field Borrow Blocks Root Transfer

```go
func MutatePart(x \mub *Part) {}

func f(ch chan \iso *Outer, x \iso *Outer) {
    MutatePart(x.f) // inferred field borrow for call duration
    ch <- x         // ok after the call returns
}
```

Field-sensitive tracking records place `x.f` during the call. Root transfer of
`x` requires no live descendant borrows, so this is safe once the call borrow
has ended. If an explicit named borrow of `x.f` remains live, root transfer is
rejected. A root-only implementation may conservatively collapse `x.f` to `x`.

### Freeze Requires Exclusivity

```go
func Inspect(x \rob *T) {}

func good(x \iso *T) \imm *T {
    Inspect(x) // inferred read borrow ends when call returns
    return x   // if result context requires \imm, checker may freeze/move here
}

func bad(x \iso *T) \imm *T {
    leakBorrowSomehow(x) // proof frontier: x is no longer proven isolated
    return x
}
```

Freeze, whether explicit or inferred from return/send context, requires no
live mutable or read borrows in the region being frozen. For inferred call
borrows, the lifetime is the call. For named borrows, instruction-level
liveness is required; block-level liveness is not precise enough. SSA liveness
for named local borrows is implemented for sends, calls, goroutine calls,
deferred calls, deferred closures, branches, and loops; explicit `\freeze`
and inferred return-freeze still need equivalent treatment.

## Current And Next Implementation Slices

Completed foundation:

- Scan and classify all capability qualifiers and explicit intrinsic tokens.
- Produce stripped analysis/emit Go for qualifier-oriented programs.
- Load stripped Go with `go/packages`.
- Build SSA and retain the `ssa.Program`/`ssa.Package`.
- Bind qualifiers to objects, function signatures, channel element types,
  struct fields, call sites, send sites, and source places in
  `CapabilityIndex`.
- Recover field-sensitive AST places for statically typed selector paths.
- Report checker diagnostics against original `.gown` source with context.

Completed checker slices:

- `GWN001`: moved `\iso` use after send, inferred freeze-send, call,
  deferred call, goroutine capture, deferred borrow, or assignment move.
- `GWN002`: conflicting inferred call borrows, including field-sensitive
  sibling-vs-overlap checks.
- `GWN003` and `GWN010`: non-sendable sends and channel/value capability
  mismatches.
- `GWN004`: goroutine borrow escapes through arguments and closure captures.
- `GWN005`: writes through `\rob`/`\imm`.
- `GWN006`: storing borrows into escaping locations.
- `GWN007`: returning borrows.
- `GWN008`/`GWN009`: historical hard-boundary checks for untracked calls and
  interface erasure. The active AST and SSA boundary passes now return no hard
  errors for ordinary tracked values; proof frontier tracking in `GWN001`
  records the boundary and later reports `GWN012` if code tries to use the
  value as proven capability again. The closure-escape checker still reports
  `GWN008` when a closure capturing a non-shareable tracked value is passed to
  an untracked call.
- `GWN012`: using a value as a proven capability after its proof has ended at
  an untracked call, untracked parameter, or interface-erasure frontier.

Important limitation:

The completed checker slices now run through SSA when SSA is available.
`GWN001` uses a forward worklist over SSA basic blocks with conservative state
merges. The other completed slices are still simpler SSA instruction scans
backed by AST/types place side tables; they validate the instruction mapping
and side-table precision, but they are not yet general lifetime/liveness
passes.

## SSA Dataflow Spike Status

The high-risk SSA work started as a spike rather than a broad rewrite. The
goal was to validate that an SSA dataflow engine can reproduce useful checker
behaviors while preserving original `.gown` diagnostics and field-sensitive
place recovery. That spike has succeeded for the current diagnostic set, and
the SSA path is now wired into the main checker runner.

Spike questions:

- Can we map the SSA instructions we care about back to `Place` reliably:
  `Send`, `Call`, `Go`, `MakeClosure`, `Store`, `FieldAddr`, `UnOp`, `Phi`,
  `IndexAddr`, `Lookup`, and interface operations?
- Should place recovery remain primarily AST/types-based with SSA used for
  control flow and liveness, or should SSA value propagation own more of the
  place model?
- What is the smallest `FunctionState` that can model consumed `\iso` places,
  temporary inferred call borrows, and conservative CFG merges?
- Can instruction-level liveness end temporary borrows precisely enough without
  introducing unsoundness around closures, goroutines, defers, and stores?
- Can field projections survive through `FieldAddr` chains, and when should
  the engine collapse to root?

Original spike non-goals:

- Do not remove the existing AST/place checker code until the SSA path has
  enough soak time.
- Freeze/new/unsafe semantics and first-pass nil insertion are now implemented;
  clone still needs a production emit policy.
- Do not solve all loop precision. Conservative rejection at loop joins is
  acceptable for the spike.

Proposed TDD slices:

1. SSA inventory tests.
   Add small fixtures that assert how Go lowers direct sends, calls, selector
   chains, field loads/stores, goroutines, closure captures, and simple branch
   joins. The output should be stable helper facts, not brittle full SSA text.
2. Place propagation tests.
   Given SSA values/instructions plus the existing `PlaceIndex`, prove the
   spike can recover `x`, `x.f`, `x.f.g`, and root-collapsed `x.slice[i]`.
3. Minimal state-machine tests.
   Unit-test consume/use, borrow begin/end, and CFG merge behavior without
   loading a package.
4. First integration parity test.
   Run an opt-in SSA checker path for `GWN001` use-after-send and verify it
   reports the same original `.gown` diagnostic shape as the current checker.
5. Field-sensitive integration test.
   Verify the SSA path can distinguish sibling fields for a call-borrow
   conflict case, or document exactly why AST/place binding must remain the
   source of truth for that precision.

Spike deliverables:

- A package-private SSA dataflow prototype and focused SSA checkers.
- A written decision in this document: continue with the hybrid AST/types/place
  plus SSA-control-flow architecture.
- A migrated main checker runner that prefers SSA implementations and falls
  back to AST/place implementations when SSA is unavailable.

Spike exit criteria:

- Existing `go test ./...` remains green. Completed.
- The spike can model `GWN001` over SSA without worse diagnostics. Completed.
- The spike can recover field-sensitive struct projections through SSA while
  preserving the hybrid rule: AST/types provide places, SSA provides ordering,
  liveness, and CFG joins. Completed.
- The team has enough evidence to choose the next vertical slice without
  redesigning the side-table model. Completed.

Spike progress:

- SSA inventory tests now confirm that the package exposes the instruction
  categories needed by the checker: calls, sends, goroutines, closure creation,
  defers, field addresses, loads, stores, dynamic indexes, map lookups, interface
  boxing, and branch phis.
- SSA is built with `ssa.GlobalDebug` so `ssa.Function.ValueForExpr` can seed
  SSA values from AST/source places.
- A prototype `SSAPlaceIndex` can propagate `Place` facts through `FieldAddr`,
  `UnOp`, and `Store`, preserving field-sensitive paths such as `h.Inner.Item`.
- The same prototype collapses dynamic or erased operations, including
  `IndexAddr`, `Lookup`, and `MakeInterface`, back to the root place.
- This supports the hybrid design: AST/types remain the source of truth for
  source places, while SSA can provide ordering, liveness, and CFG joins.
- A prototype `SSAFunctionState` models consumed root places, temporary borrows,
  and conservative CFG merges. It explicitly rejects ownership moves from field
  projections (`x.f`) instead of inserting hidden nil assignments or silently
  consuming sibling state.
- SSA parity passes now cover `GWN001` through `GWN010` plus `GWN011`, with
  `GWN001` implemented as CFG dataflow. `GWN001` now also uses backward SSA
  liveness for named `\mub`/`\rob` borrows and rejects root transfers while a
  derived named borrow is live. Sending `\iso` on `chan \imm` is modeled as an
  inferred freeze-send transfer that consumes the sender's root and consults
  the same named-borrow liveness guard. Deferred arguments whose parameter
  types are `\iso` are consumed at the defer statement; deferred arguments
  whose parameter types infer `\mub` or `\rob`, deferred named borrow
  arguments, and deferred closure captures are modeled as borrows that remain
  active until function exit.
- The same `GWN001` dataflow now exercises loop fixpoints directly: loop-body
  sends are visible after the loop, loop-local named borrows can die before a
  later transfer, named borrows assigned inside a loop remain live after the
  loop when SSA liveness says they may be used, and repeated deferred closure
  `\iso` moves are rejected because each defer registration creates another
  function-exit move.
- `GownPackage.Check` now routes the main checker runner through the SSA passes
  when SSA is available, while keeping AST/place checkers as fallbacks.
- Integration tests cover precision improvements that the root-only AST passes
  missed, including tracked struct fields passed to untracked functions and
  tracked struct fields erased into interfaces.
- Diagnostic placement remains an explicit contract: SSA checks may use SSA
  instruction positions for operation errors, but should prefer annotated
  object/field positions when the user needs to see the source qualifier in the
  reported `.gown` line.

## Spike Learnings

The main architectural learning is that the hybrid approach is not just a
fallback; it is the right shape for Gown. AST/types should remain the source of
truth for source places because they preserve user intent, selector syntax, and
original `.gown` diagnostics. SSA should provide instruction ordering, control
flow, liveness, closure/call/send/store forms, and CFG joins.

SSA can still carry field-sensitive information far enough to be useful. With
`ssa.GlobalDebug`, `ssa.Function.ValueForExpr` can seed SSA values from the
existing AST place index. From there, a small prototype can propagate places
through `FieldAddr`, `UnOp`, and `Store`, preserving paths like
`h.Inner.Item`. Dynamic or erased operations such as `IndexAddr`, `Lookup`, and
`MakeInterface` can collapse back to the root, matching the soundness rule in
the field-sensitivity design.

The spike also clarified the ownership move rule. Ownership moves are root-only:
moving `x` can consume `x`, but moving `x.f` would leave `x` still holding the
same field value unless Gown silently rewrote the user's field. Gown should not
write hidden nil assignments into user fields. Therefore, field projections are
valid for borrow precision, but they are rejected as ownership move sources
with `GWN011`.

The spike answered the first high-risk question: SSA has the right instruction
shapes, field-sensitive places can be represented, the state machine can run
over real SSA CFGs, and `GWN001` can preserve original `.gown` diagnostics
while using SSA ordering and CFG merges. The next lifetime slice extended that
machinery to named `\mub` and `\rob` borrows created through annotated local
variables. The remaining risk is generalizing it further to explicit
freeze/clone semantics, loops, complex closures, and unknown
synchronization/unsafe boundaries. The first freeze inference slice is now
implemented for sends:
`ch <- x` where `ch` has element capability `\imm` and `x` is `\iso` is
accepted, consumes `x`, rejects live named borrows of `x`, and rejects field
projection transfers.

A further SSA parity finding is that assignment moves are source-level events
that optimized SSA can erase. A move such as `b := a` may not survive as a
distinct dynamic SSA instruction even though it is semantically meaningful to
Gown's ownership model. The checker should therefore model assignment moves as
hybrid source events anchored to SSA debug/source positions, such as
`DebugRef`s for the right-hand side expression, so they can run in SSA block
order without pretending assignment is always a normal SSA instruction.

The same parity slice showed that raw SSA value operands are not authoritative
for source-place uses after assignment. The same SSA value may represent both
`a` and `b` after `b := a`, so a generic operand scan can falsely report
`println(b)` as a use of moved `a`. Source-use checks should prefer exact
`DebugRef` expressions and the AST `PlaceIndex`; SSA values should carry places
for transfer reasoning, not override source identity in diagnostics.

Named borrow liveness produced a stronger version of that lesson. After
`var b \mub *T = a`, SSA may reuse one value for both `a` and `b`; transfer
checks cannot rely on the raw call/send operand to identify the source owner.
For sends, calls, and goroutine calls, Gown now prefers source side-table
bindings to recover the original argument or send expression, then asks the
SSA liveness set whether any named borrow derived from that place is live after
the transfer instruction. This is the durable rule: source bindings identify
what the user wrote; SSA tells us where it is live.

Defer handling adds a Go-specific lifetime rule that is easy to miss. For a
plain deferred call, Go evaluates and saves the argument values at the point
where the defer is registered, not when the deferred call runs at function
exit. Therefore, a call such as `defer Take(a)`, where `Take` takes
`\iso *T`, consumes `a` at the defer statement. A call such as
`defer Mut(a)`, where `Mut` takes `\mub *T`, creates an inferred mutable borrow
of `a` that remains live until function exit. Similarly, `defer use(b)`
extends the source borrow behind a named `\mub` or `\rob` variable `b` until
function exit even if the local variable `b` is reassigned later. Deferred
closures also need analysis because they may capture named borrow variables;
Gown now uses SSA closure bindings and a source-position fallback for deferred
function literals to conservatively activate those captured borrows until
function exit. Deferred function literals are now recorded as pending closure
effects rather than applied at the defer registration site. At each SSA return,
pending deferred closures are checked in LIFO order by running the closure's
own SSA CFG against the outer function's exit state. That means `\iso` calls
and sends move at function exit, inferred `\mub`/`\rob` calls borrow
temporarily while that deferred closure runs, ordinary reads can detect a
previous move, and returning the same place a deferred closure will later use
is rejected. The older flat AST call-effect list remains useful as a summary
for return conflicts and repeated-defer move detection, but it no longer
defines intra-closure ordering when the SSA closure body is available.

The deferred-closure CFG slice removed an important false-positive source.
Flattening calls from a function literal made mutually exclusive branches look
sequential: `if cond { Take(a) } else { Read(a) }` looked like `Take(a);
Read(a)`. Running the closure body through the same SSA dataflow engine instead
preserves branch structure: branch-exclusive take/read and take/take effects
are allowed, while `Take(a); Read(a)` on one path and `if cond { Take(a) };
Read(a)` after a join are rejected. The same machinery also catches deferred
closures that only use ordinary expressions, such as `println(a)`, after the
outer function has moved `a`.

Loops validated the state-merge rule that had been implicit in the SSA
prototype. At a loop join, consumed roots and live named borrows are interpreted
as may-have-happened facts: if one iteration or one branch can move `a`, a use
after the loop is rejected; if a borrow is scoped entirely inside the loop body
and is not live at the back edge or after the loop, a later move remains
allowed. Deferred closure effects need an extra multiplicity bit. A single
defer statement in a loop may register the same closure more than once, so a
deferred closure that eventually performs a root `\iso` move is rejected as a
possible repeated move, while repeated deferred read-only effects are allowed.
This is conservative but matches Go's runtime defer stack semantics.

Incremental annotation needs a different rule for unannotated code than the
original hard-boundary design. Unannotated values and functions are outside the
proof by default. Passing `a \iso *T` to `Plain(a)` or erasing it into `any`
therefore records a proof frontier instead of reporting an immediate error.
The checker marks the source place as no longer proven; ordinary unannotated
uses may continue, but later operations that require the original proof, such
as sending `a` on `chan \iso *T`, calling `Take(a)` where `Take` requires
`\iso`, borrowing `a` for a `\rob`/`\mub` parameter, or returning it as a
tracked result, are rejected with `GWN012`. Frontier facts merge
conservatively across branches: if any predecessor can end the proof for `a`,
the join treats `a` as no longer proven.

Escaping closures add a second closure rule. Go function values are ordinary
copyable values, so a returned, stored, or untracked-call-passed closure cannot
safely capture `\iso`, `\mub`, or `\rob`. `\iso` is included because a copied
function value could invoke the same captured unique value more than once or
from multiple places. The current checker rejects direct escaping function
literals and tracks local function-valued variables with a flow-sensitive
closure environment. Assignment replaces the current closure value, so
`fn := func(){ use(b) }; fn = func(){}; return fn` is allowed. Branch joins
merge only the closure values that can reach that program point, so
`fn := func(){ ... }; g := fn; return g` and branch-possible escaping captures
are still rejected. Non-escaping local closure calls remain allowed.

Current SSA checker coverage includes `GWN001` parity for direct sends,
inferred freeze-sends to `chan \imm`, iso-consuming calls, deferred
iso-consuming calls, assignment moves, branch merges, goroutine calls, closure
captures, named borrow liveness, branch-sensitive named borrow liveness,
loop fixpoint state, loop-local named borrow death, loop-carried named borrow
liveness, deferred named borrow snapshots, deferred inferred borrow arguments,
deferred closure borrow captures, deferred closure inferred borrow effects,
deferred closure iso-consuming effects with LIFO exit ordering, deferred
closure body CFG precision for branches, sends, calls, and ordinary reads,
repeated deferred closure `\iso` move rejection, and projected field-move
rejection. It also tracks proof frontiers for untracked calls, untracked
parameters of partially annotated calls, and interface erasure, rejecting later
capability-required operations with `GWN012`. It
also includes SSA-backed checks for inferred call-borrow conflicts, send
capability checks, goroutine borrow escapes, escaping closures that capture
non-shareable tracked values, read-only writes, borrow stores, and returned
borrows. These SSA checks are now wired into the main checker pipeline, with
AST/place implementations retained as fallbacks and comparison references.

Wiring the SSA runner into the main pipeline exposed one diagnostic lesson:
operation positions and annotation positions are both valuable, but not
interchangeable. For example, a non-sendable `\mub` send is logically detected
at the send instruction, while the most helpful source context may be the
annotated declaration or field. SSA checkers should choose positions based on
what the user needs to see to understand and fix the error, while always
mapping generated `.go` paths back to original `.gown` paths.

## Open Implementation Notes

- Normal `Check` still writes generated `.go` files; CLI `-check` uses an
  overlay to avoid writing. Future library APIs may want a clearer split
  between check-only analysis, emit, and combined check-and-emit workflows.
- Explicit expression syntax is recognized by the scanner and formatter, but
  most semantics are not implemented. It should not pollute final output or
  collide silently with user declarations.
- Diagnostics should always point to `.gown` positions, never generated
  analysis Go positions. The current diagnostics already do this, and SSA
  checks must preserve it.
- Field-sensitive precision exists for AST/types selector paths and now also
  survives through key SSA forms. The long-term design should remain explicitly
  hybrid.
- The existing SSA checker passes provide useful safety coverage, but only
  `GWN001` currently performs full CFG dataflow. Named borrow liveness now
  covers straight-line code and branches for sends, inferred freeze-sends,
  calls, goroutine calls, deferred inferred borrow arguments, deferred named
  borrow arguments, deferred closure captures, and tracked calls inside
  deferred function literals. Closure escape checks now cover direct function
  literals and flow-sensitive local closure aliases returned, stored into
  escaping locations, or passed to untracked calls. Loop coverage now exercises
  root moves, named borrow liveness, deferred closure effects, and local closure
  aliases across back edges. Deferred closure body coverage now uses the
  closure's own SSA CFG for branches, sends, calls, ordinary reads, and
  post-merge move checks at function exit. Proof frontier coverage now handles
  direct untracked calls, untracked parameters in otherwise annotated calls,
  interface erasure, branch merges, later sends, later tracked calls, later
  borrows, and tracked returns. Explicit freeze/clone, interprocedural and
  heap/container/interface closure flow, closure bodies with nested defers or
  more complex escaping effects, surfaced frontier notes, and precise
  unsafe/synchronization boundaries still need broader treatment.
