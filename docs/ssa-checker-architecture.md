# Gown SSA Checker Architecture

This document describes the planned checker architecture for Gown's lifetime
and borrow analysis. It is a design artifact only: it does not describe code
that already exists, and it does not change current checker behavior.

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

The current implementation is a front end, capability binding pass, and program
inventory pass:

- `scanAndClassify` recognizes capability qualifiers, records byte
  offset/line/column, and replaces qualifier bytes with spaces so Go parser
  positions still line up.
- `CapabilityIndex` binds qualifier annotations to `types.Object`s, function
  signatures, and annotated call sites.
- `Check` writes stripped `.go` files beside `.gown` files, loads the package
  with `go/packages`, and retains the resulting AST/types information.
- `assignRegions` maps `\iso` annotations to containing functions and block
  regions.
- `assignBoundary`, `computeReachableTypes`, and `assignCreates` find
  concurrency/global/import boundaries, reachable pointer-bearing types, and
  relevant allocation sites.
- No SSA checker exists yet, and no capability errors are rejected yet.

This architecture extends that scaffold rather than replacing it.

## End-To-End Pipeline

The future checker should use two generated source views:

1. **Analysis Go:** valid Go fed to `go/packages` and `go/ssa`.
2. **Emit Go:** final plain Go written beside `.gown`, with annotations erased
   and move/freeze nil assignments inserted.

The pipeline is:

1. Scan `.gown` and record every Gown token as an `Annotation`.
2. Classify each token. The mainline syntax is capability type qualifiers;
   explicit unsafe or intrinsic-like expression forms are optional/future
   syntax.
3. Produce analysis Go:
   - Replace capability type qualifiers with spaces.
   - Leave ordinary Go expressions unchanged.
   - If future explicit expression forms are enabled, rewrite them to
     analysis-only placeholders.
4. Load analysis Go with `go/packages`.
5. Bind annotations back to AST nodes, `types.Object`s, function signatures,
   channel element types, struct fields, and call sites.
6. Build SSA with `golang.org/x/tools/go/ssa`.
7. Seed checker state from capability side tables.
8. Infer borrow/move/freeze behavior from typed contexts and run capability
   dataflow over SSA.
9. Report structured GWN errors at original `.gown` positions.
10. If checking succeeds and `-check` is false, emit final Go.

The analysis Go and emit Go views should be produced from the same annotation
index so they cannot drift.

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

Explicit expression forms may still be useful later for operations that cannot
be inferred safely, such as `\unsafe(x)`, `\clone(x)`, or explicit freeze.
If/when enabled, these forms should be rewritten to analysis-only placeholders
and recognized by the checker. They are not required for the first
inference-based checker milestones.

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
- Optional explicit-operation annotations for future expression syntax such as
  `\unsafe(x)` or `\clone(x)`.
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
  non-sendable `\mub` and `\rob`.
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

- Scan and classify all capability qualifiers.
- Produce stripped analysis/emit Go for qualifier-only programs.
- Load stripped Go with `go/packages`.
- Bind qualifiers to objects, function signatures, and call sites in
  `CapabilityIndex`.

The next implementation milestone should be `GWN001` for `\iso`
use-after-send/move:

- Build SSA and map relevant sends/uses back to source places.
- Mark an `\iso` place consumed after a direct send on `chan \iso *T`.
- Reject subsequent uses of that place with a structured diagnostic.

The following slice should add inferred call borrows:

- For a call whose callee parameter is `\mub` or `\rob`, create a temporary
  borrow of the argument place for the duration of the call.
- Reject calls that would violate borrow exclusivity.
- Do not require users to write `\mub(x)` or `\rob(x)`.

These slices should not implement explicit expression intrinsics or nil
insertion yet. They should use the data model above so field sensitivity and
explicit unsafe/clone/freeze operations can be added without redesign.

## Open Implementation Notes

- The `-check` CLI flag currently exists but is not wired through to avoid
  writing `.go` files. The architecture assumes check-only mode will eventually
  load analysis Go without changing committed generated files.
- Explicit expression syntax, if enabled later, should not pollute final output
  or collide silently with user declarations.
- Diagnostics should always point to `.gown` positions, never generated
  analysis Go positions.
- Field-sensitive precision should be tested with small SSA fixtures before it
  is used for acceptance decisions.
