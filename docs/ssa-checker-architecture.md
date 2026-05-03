# Gown SSA Checker Architecture

This document describes the checker architecture for Gown's lifetime and borrow
analysis. It is a living design artifact: some pieces are implemented, some are
only partially implemented, and the SSA dataflow engine remains the highest
risk planned work.

The central decision is that Go's AST, type checker, and SSA builder remain
ordinary Go tooling. They do not learn about `\iso`, `\mub`, `\rob`, or
`\imm`. Gown carries capability information in side tables keyed by source
offsets, AST nodes, `types.Object`s, function signatures, call sites, SSA
values/instructions, and source places.

The user-facing direction is inference-first. Gown should require the minimum
annotations needed to state ownership boundaries: function signatures, channel
element types, and struct fields. Ordinary borrows should be inferred from
typed context, such as calling a function whose parameter is declared
`\mub *T` or `\rob *T`. Explicit expression forms like `\mub(x)` are not part
of the main path and should not drive the first checker milestones.

## Current State

The current implementation now has a working front end, side-table binding
layer, SSA construction step, and a suite of AST/place-based checker passes.
The full SSA dataflow engine described below is not implemented yet.

Implemented:

- `scanAndClassify` recognizes capability qualifiers and explicit intrinsic
  forms, records byte offset/line/column, and produces source views where
  Go parser positions still line up for qualifiers.
- `Check` writes stripped `.go` files beside `.gown` files, loads the package
  with `go/packages`, retains AST/types information, and builds SSA with
  `golang.org/x/tools/go/ssa`.
- `CapabilityIndex` binds qualifier annotations to `types.Object`s, function
  signatures, channel element types, struct fields, call sites, send sites,
  and source places.
- `PlaceIndex` recovers source places from AST/types, including statically
  typed selector projections such as `x.f.g`; dynamic index expressions
  collapse to the root region.
- Checker passes currently reject several capability violations: moved `\iso`
  use, conflicting inferred call borrows, non-sendable sends, channel/value
  capability mismatch, goroutine borrow escapes, read-only writes, borrow
  stores, returned borrows, untracked call boundaries, and interface erasure.
- Diagnostics report structured `GWN` errors against original `.gown` source
  and include source-line context.
- `gownfmt` formats `.gown` source through `go/format` while preserving Gown
  annotations.

Still missing:

- A real SSA CFG/dataflow engine with instruction-level transfer functions,
  liveness, and conservative merge behavior.
- A check-only load path that avoids writing generated `.go` files.
- Final emit behavior that inserts nil assignments after consumed `\iso`
  moves.
- Semantics for freeze/clone/unsafe beyond token recognition and formatting.
- Broader boundary modeling for reflection, `sync`, atomics, and unsafe code.

This architecture now extends the working AST/place checker rather than just a
front-end scaffold.

## End-To-End Pipeline

The long-term checker should use two generated source views:

1. **Analysis Go:** valid Go fed to `go/packages` and `go/ssa`.
2. **Emit Go:** final plain Go written beside `.gown`, with annotations erased
   and move/freeze nil assignments inserted.

The target pipeline is:

1. Scan `.gown` and record every Gown token as an `Annotation`.
2. Classify each token. The mainline syntax is capability type qualifiers.
   Explicit unsafe or intrinsic-like expression forms can be scanned and
   formatted today, but most checker semantics are future work.
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
   checking. Today this is done by AST/place-based passes; the target is a
   forward SSA dataflow engine.
9. Report structured GWN errors at original `.gown` positions.
10. If checking succeeds and `-check` is false, emit final Go.

The analysis Go and emit Go views should be produced from the same annotation
index so they cannot drift.

Current status:

- Stages 1 through 7 exist for qualifier-oriented programs.
- Stage 8 exists as AST/place checker passes, not as SSA dataflow.
- Stage 9 exists.
- Stage 10 partially exists: stripped Go is written, but check-only mode and
  nil insertion after consumed `\iso` moves remain open.

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
- Optional explicit-operation annotations for expression syntax such as
  `\unsafe(x)` or `\clone(x)`, whose semantics are still mostly future work.
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
  metadata; require an explicit unsafe boundary for untracked code that
  receives capability-tracked values.
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
- Passing capability-tracked values to untracked code requires `\unsafe`.
- Interfaces, reflection, `sync`, atomics, and unsafe operations are treated
  as boundaries until explicitly modeled.

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

Named or escaping borrows are a future feature. If explicit `\mub(x)` syntax is
enabled later, then this form blocks transfer until the borrow is dead:

```go
func f(ch chan \iso *T, x \iso *T) {
    b := \mub(x) // future explicit syntax
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
has ended. If a future explicit/named borrow of `x.f` remains live, root
transfer is rejected. A root-only implementation may conservatively collapse
`x.f` to `x`.

### Freeze Requires Exclusivity

```go
func Inspect(x \rob *T) {}

func good(x \iso *T) \imm *T {
    Inspect(x) // inferred read borrow ends when call returns
    return x   // if result context requires \imm, checker may freeze/move here
}

func bad(x \iso *T) \imm *T {
    leakBorrowSomehow(x) // untracked/escaping borrow requires unsafe or reject
    return x
}
```

Freeze, whether explicit or inferred from return/send context, requires no
live mutable or read borrows in the region being frozen. For inferred call
borrows, the lifetime is the call. For future named borrows, instruction-level
liveness is required; block-level liveness is not precise enough.

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

- `GWN001`: moved `\iso` use after send, call, goroutine capture, or
  assignment move.
- `GWN002`: conflicting inferred call borrows, including field-sensitive
  sibling-vs-overlap checks.
- `GWN003` and `GWN010`: non-sendable sends and channel/value capability
  mismatches.
- `GWN004`: goroutine borrow escapes through arguments and closure captures.
- `GWN005`: writes through `\rob`/`\imm`.
- `GWN006`: storing borrows into escaping locations.
- `GWN007`: returning borrows.
- `GWN008`: passing tracked values to untracked user calls.
- `GWN009`: erasing tracked values into interfaces.

Important limitation:

The completed checker slices are AST/place-based. They use the same side-table
model the SSA checker should use, but they do not yet perform forward dataflow
over SSA basic blocks, instruction-level liveness, or conservative CFG merges.

## SSA Dataflow Spike Plan

The next high-risk work should be a spike, not a broad rewrite. The goal is to
validate that an SSA dataflow engine can reproduce the first useful checker
behaviors while preserving original `.gown` diagnostics and field-sensitive
place recovery.

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
  introducing unsoundness around closures, defers, goroutines, and stores?
- Can field projections survive through `FieldAddr` chains, and when should
  the engine collapse to root?

Spike non-goals:

- Do not replace every existing checker pass.
- Do not implement freeze/clone/unsafe semantics.
- Do not implement nil insertion.
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

- A small package-private SSA dataflow prototype, ideally isolated in
  `ssa_dataflow.go` and test helpers.
- A written decision in this document: continue with full SSA dataflow,
  keep a hybrid AST/place plus SSA-control-flow architecture, or defer SSA
  dataflow if it does not buy enough precision yet.
- A short list of existing checker passes that should be migrated first, if
  the spike succeeds.

Spike exit criteria:

- Existing `go test ./...` remains green.
- The spike can model `GWN001` over SSA without worse diagnostics.
- The spike can either recover field-sensitive struct projections through SSA
  or clearly validates the hybrid approach: AST/types provide places, SSA
  provides ordering, liveness, and CFG joins.
- The team has enough evidence to choose the next vertical slice without
  redesigning the side-table model.

Spike progress:

- SSA inventory tests now confirm that the package exposes the instruction
  categories needed by the checker: calls, sends, goroutines, closure creation,
  field addresses, loads, stores, dynamic indexes, map lookups, interface
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

## Open Implementation Notes

- The `-check` CLI flag currently exists but is not wired through to avoid
  writing `.go` files. The architecture assumes check-only mode will eventually
  load analysis Go without changing committed generated files.
- Explicit expression syntax is recognized by the scanner and formatter, but
  most semantics are not implemented. It should not pollute final output or
  collide silently with user declarations.
- Diagnostics should always point to `.gown` positions, never generated
  analysis Go positions. The current diagnostics already do this, and the SSA
  spike must preserve it.
- Field-sensitive precision already exists for AST/types selector paths. The
  SSA spike should validate whether SSA propagation can preserve that precision
  or whether the long-term design should be explicitly hybrid.
- The existing checker passes provide useful safety coverage, but their
  AST traversal order is not a substitute for SSA CFG dataflow once named
  borrows, loops, branches, defers, closures, and precise liveness matter.
