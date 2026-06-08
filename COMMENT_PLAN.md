# Comment-Mode Gown Architecture

## Summary

Implement `//gown:` comment-mode as a front-end lowering pass from ordinary
`.go` files into a virtual `.gown` source view. The existing
`.gown -> analysis .go -> checker` pipeline remains the semantic source of
truth, so current checker passes and most tests stay unchanged.

Normal comment-mode checking is non-mutating. Existing `.go` files are never
rewritten. Add a CLI print flag to visualize the virtual `.gown` source that
Gown actually checks.

## Key Changes

- Discover annotated `.go` files containing `//gown:` comments, excluding
  hidden/underscore files and preserving current `.gown` discovery.
- Add a lowerer API:
  - `LowerGoCommentsToGown(path string, src []byte) (*CommentLoweringResult, error)`
  - Result includes virtual `.gown` bytes, original path, and optional source
    mapping.
- Feed lowered virtual `.gown` through existing `scanAndClassify`,
  `assignCapabilities`, intrinsic binding, restore binding, SSA, and checker
  passes.
- Preserve existing `.gown` behavior. Mixed packages are allowed unless a
  `.gown` generated path collides with an annotated `.go` file of the same
  basename.
- Add CLI flag `-print-gown`:
  - Prints the virtual `.gown` view for annotated `.go` files.
  - Does not write files.
  - Exits after lowering/printing unless lowering fails.

## Comment Syntax

Use `//` comments only. No `/* */` comment syntax.

Supported forms:

```go
//gown: param x iso; param y rob; result 0 imm
func F(x *X, y *Y) *Z
```

```go
type Holder struct {
    Owned *Msg      //gown: iso
    Done chan *Msg  //gown: cap imm; elem iso
}
```

```go
var x *Msg //gown: iso
```

```go
b := &Ticket{} //gown: new
r := b         //gown: rob
m := b         //gown: mub
f := b         //gown: freeze
u := b         //gown: unsafe
```

```go
copy := x.clone() //gown: clone x
copy := x.Clone() //gown: Clone x
```

```go
a, b = b, a //gown: swap
```

```go
//gown: restore param a iso; param b iso; result 0 iso; result 1 iso
a, b = func(a *node, b *node) (*node, *node) {
    old := a.next
    a.next = b.next
    b.next = old
    return a, b
}(a, b)
```

```go
//gown: observer fmt.Printf
```

Lowering rules fail closed: if a comment target is ambiguous, does not match
the expected Go shape, or would invent runtime behavior not present in the
`.go` source, Gown reports a lowering error.

## Implementation Details

- Add a comment parser that reads `go/parser.ParseComments` output and resolves
  directives to AST nodes by same-line trailing comments or immediately
  preceding comment lines.
- Build virtual `.gown` by inserting or replacing text on the same line only,
  preserving line numbers.
- Lower ownerstamp comments by inserting `\iso`, `\mub`, `\rob`, or `\imm` at
  the corresponding type position.
- Lower intrinsic comments into existing backslash forms:
  - `new` over `&T{...}` becomes `\new(T{...})`.
  - `mub`, `rob`, `freeze`, and `unsafe` wrap the marked RHS.
  - `clone` and `Clone` validate the method-call shape and lower to
    `\clone(x)` / `\Clone(x)`.
  - `swap` validates `a, b = b, a` and lowers to `\swap(a, b)`.
  - `restore` inserts `\restore` before the IIFE and ownerstamps its
    params/results.
- Add `CheckOptions.GoOverlay map[string][]byte` for editor/test overlays of
  annotated `.go` files; keep `GownOverlay` unchanged.
- Add interface-method ownerstamp binding if needed so comment-mode matches the
  current spec for interface signatures.

## Test Plan

- Unit-test comment lowering into expected virtual `.gown` text for fields,
  vars, function params/results, channel element caps, observer, all intrinsics,
  swap, and restore.
- Add checker tests using ordinary `.go` files with `//gown:` comments for:
  - `iso` move/use-after-move.
  - Channel send/receive ownerstamps.
  - `new`, `mub`, `rob`, `freeze`, `clone`, `unsafe`.
  - `swap` success/failure.
  - `restore` success/failure.
- Add CLI tests:
  - Annotated `.go` packages check without writing files.
  - `-print-gown` emits the virtual `.gown` view.
  - Existing `.gown` tests still pass.
  - Mixed `.go`/`.gown` packages work except basename collisions.
- Run:
  - `go test ./...`
  - Targeted restore/swap/annotation tests while developing.

## Assumptions

- Comment-mode is check-only and non-mutating by default.
- The virtual `.gown` view is the checker input and the visualization surface.
- Only `.go` files containing `//gown:` are lowered.
- `.gown` syntax remains supported as-is.
- Block comments are intentionally unsupported for Gown annotations.
