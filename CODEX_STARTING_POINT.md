# Codex Starting Point: Gown

This note summarizes the project as of the initial repository read. It is meant
as a fast orientation checkpoint for future work.

## Project Goal

Gown is a source-to-source preprocessor for Go. It accepts `.gown` files, which
are Go source plus backslash-prefixed ownerstamps, and emits plain
`.go` files after ownerstamp checking.

The intended guarantee is race freedom: if all relevant source passes the Gown
checker and does not use `\unsafe`, then no execution has a data race.

The intended ownerstamps are:

| Ownerstamp | Meaning | Mutable | Cross-goroutine |
| --- | --- | --- | --- |
| `\iso` | isolated unique owner | yes | yes, by move |
| `\mub` | mutable borrow | yes | no |
| `\rob` | read-only borrow | no | no |
| `\imm` | deeply immutable shared value | no | yes, by copy/share |

The specs make untracked Go pointers opt-in/legacy: they behave as plain Go and
are outside the guarantee. Passing ownerstamped values to unchecked code must be
an explicit `\unsafe` boundary in the full design.

## Design Scope From Markdown

Primary docs:

- `gown-spec.md` is the normative draft spec. It covers the four ownerstamps,
  channel send/receive rules, goroutine capture rules, freeze, borrow coercion,
  assignment/move semantics, viewpoint adaptation, `select`, `\unsafe`,
  built-ins, interfaces, transpiler output, syntax, errors, and out-of-scope
  items.
- `theory-proof.md` gives the race-freedom argument and, crucially, the checker
  contract in section 15. The key invariants are Isolation, Immutability, and
  Coherent.
- `Gown.lean` is the mechanized proof referenced by the docs, though this note
  focuses on the requested Markdown and Go files.
- `chatcritique.md`, `chatproblem.md`, and `theory-proof-gemini-v2.md` are
  critique/repair notes. They are important because they identify where a
  checker must be stricter than the early prose proof.
- `already_applied/PLAN003_proof_fixup.md` describes a Lean proof repair plan:
  add Coherent, prove preservation for Iso/Fresh/Coherent, and remove custom
  axioms/sorries.
- `AGENTS.md` and `CLAUDE.md` are repository guidance files describing the
  pipeline and build/test commands. `AGENTS.md` was untracked at the time of
  this read and should be treated as user-owned context.

Important design obligations extracted from the proof/critique docs:

- Sending, spawning, freezing, or moving an `\iso` is only sound if no live
  borrows or aliases into its owned region remain in the sender.
- The checker needs explicit borrow/region tracking, not just ownerstamp labels.
  Critique docs propose either region sets or SSA-level borrow state.
- Field-derived and transitive borrows matter. A borrow of `x.f` must pin the
  same ownership region as `x` or a conservative approximation of it.
- Heap reachability is part of the semantic claim, but the proof model is flat.
  The checker must bridge that gap by conservatively tracking owned object
  graphs or by enforcing tree-like ownership.
- Boundaries to untracked code, `any`/interfaces, reflection, `sync`, and
  atomics are soundness hazards unless rejected, modeled, or wrapped in
  `\unsafe`.
- The safest implementation direction in the notes is an SSA-based checker:
  build SSA, annotate values with ownerstamp plus abstract location/region,
  compute liveness, update borrow state at last use, reject escaping borrows,
  and require the active region to contain only the root `\iso` at transfer or
  freeze points.

## Current Go Implementation

The implementation is currently an early front end and inventory engine, not a
full ownerstamp checker.

Pipeline in `check.go`:

1. `NewGownPackage(path)` creates a per-directory package wrapper.
2. `Check()` scans the directory for `.gown` files.
3. Each `.gown` file is stripped by `scanAndStrip()` and written as a sibling
   `.go` file.
4. The generated package is loaded through `golang.org/x/tools/go/packages`.
5. Existing annotations are assigned containing function and block regions.
6. Concurrency and global/import boundaries are detected.
7. Types reachable from those boundaries are computed.
8. Creation points for reachable pointer-bearing types are recorded.

Key files:

- `strip.go` defines `gownFile`, `isoAnnotation`, `boundaryCrossing`,
  `createAnew`, and `region`. `scanAndStrip()` currently recognizes only
  literal `\iso`, records byte offset/line/column, and replaces the annotation
  bytes with spaces so Go parser positions still line up.
- `regions.go` maps each `\iso` annotation to its containing function and
  innermost block region. Signature annotations use the function body region.
- `boundary.go` detects package-level pointer-capable variables, channel sends
  and receives, go-statement arguments, closure captures, and imported function
  return values that can hold pointers.
- `reachable.go` performs BFS over Go types starting from boundary types, or
  from annotated `\iso` types as a fallback. Encountering an interface poisons
  the analysis, causing all pointer-containing creation sites to be tracked.
- `creates.go` records reachable creation sites: `new(T)`, pointer-bearing
  `make(...)`, and `&T{}`/container composite literals that can hold pointers.
- `cmd/gown/gown.go` is the CLI. It accepts `-check`, but `Check()` currently
  always writes `.go` files, so the flag is not implemented yet.
- `vprint.go` and `cmd/gown/vprint.go` are duplicated debug/printing helpers.

What is not implemented yet:

- No actual rejection of ownerstamp errors such as use-after-move, invalid
  send, write through immutable/read-only, escaping borrow, or untracked
  boundary without `\unsafe`.
- No parsing/checking for `\mub`, `\rob`, `\imm`, `\freeze`, `\clone`,
  `\new`, or `\unsafe`.
- No insertion of `x = nil` after `\iso` sends, moves, or freezes.
- No region/borrow/liveness analysis yet.
- No SSA layer yet.
- No structured GWNxxx error reporting yet.
- No `-watch`, despite the spec mentioning it.

## Current Tests And Fixtures

The Go unit tests cover the implemented inventory behavior:

- `position_test.go` checks byte-accurate stripping, `.go` output without
  `\iso`, annotation/function mapping, and innermost block regions.
- `boundary_test.go` checks boundary detection for package vars, channel
  send/recv, go args, and closure captures.
- `reachable_test.go` checks transitive type reachability, interface poison,
  and filtering of unreachable creation sites.
- `news_test.go` checks creation detection for `new`, `make`, ampersand
  composite literals, pointer containers, pointer-key maps, named pointer
  container types, and false-positive filtering.

The sample fixture `vectors/iso0/basic.gown` sends a value on a `chan \iso`
and then uses it again with a comment saying it should not typecheck. Today it
still passes through because consumption checking has not been implemented.

Verification run during this read:

```bash
go test ./...
```

Result: passed after allowing Go to use its normal build cache outside the
workspace sandbox.

## Quick Commands

```bash
make all
make test
go test ./...
go test -run TestPositionPrecision ./
go test -run TestRegionDetection ./
make lean
```

`make test` is currently narrower than `go test ./...`: it installs the CLI and
runs `gown vectors/iso0/`.

## Likely Next Milestones

1. Decide the checker core: SSA-based region/borrow state is the strongest fit
   for the critique notes and the proof contract.
2. Extend the scanner/parser strategy beyond `\iso` so all ownerstamp keywords
   and built-ins can be represented before erasure.
3. Implement ownerstamp metadata for signatures, locals, channel element types,
   and struct fields.
4. Add the first negative checker errors: `\iso` consumed on send/move and
   use-after-consume (`GWN001`) is the most obvious end-to-end slice.
5. Implement `-check` so analysis can run without overwriting generated `.go`
   files.
6. Add tests that assert rejected programs, not just inventory metadata.

## Mental Model

The repository currently has a solid Go parser/type-loader scaffold and good
position-preservation tests. The project-wide design is much bigger: it aims
for an ownerstamp and ownership checker whose proof depends on exact tracking of
regions, borrows, liveness, and unsafe boundaries. Treat the present Go code as
the foundation for finding relevant program points, not as a checker that
already enforces the proof.
