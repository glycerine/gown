# Gown `\restore` Specification
## Conservative V1 Re-Isolation Regions

**Status:** Draft
**Purpose:** Define a Pony-like checked re-isolation construct for local pointer
surgery on `\iso` object graphs.

---

## 1. Overview

`\restore` is a scoped, auditable way to temporarily open one or more `\iso`
roots, perform local pointer rewiring, and return to ordinary checked `\iso`
ownership. It is not an escape hatch. A restore region is accepted only when the
checker can prove that all temporary aliases are dead, no alias escaped, and the
single returned value is again a valid `\iso` root.

V1 deliberately favors proof simplicity over expressiveness. The accepted source
form is an immediately invoked function literal (IIFE):

```go
dst = \restore func(x \iso *T, y \iso *U) \iso *T {
    // local pointer surgery
    return x
}(srcX, srcY)
```

`\restore` erases to nothing. Ownerstamps erase as usual, leaving plain Go:

```go
dst = func(x *T, y *U) *T {
    return x
}(srcX, srcY)
```

The closure boundary is part of the design. Restore arguments are explicit, the
result is explicit, and local aliases have ordinary Go function scope.

---

## 2. Accepted V1 Form

A v1 restore expression must appear as the complete right-hand side of a simple
assignment or short variable declaration:

```go
lhs =  \restore func(params...) \iso *T { body }(args...)
lhs := \restore func(params...) \iso *T { body }(args...)
```

Required shape:

- The assignment has exactly one left-hand side and exactly one right-hand side.
- The right-hand side is exactly `\restore` followed by an immediately invoked
  function literal.
- The function literal has exactly one result.
- The result is an unnamed `\iso *T`.
- At least one parameter is an `\iso` pointer opened by the restore region.
- Pointer parameters must be ownerstamp-typed. V1 allows `\iso` roots and
  read-only `\imm` inputs; it rejects `\mub`, `\rob`, and untracked pointer
  parameters.
- Non-pointer scalar parameters are allowed for read-only metadata such as
  counters or enum-like values.
- `lhs` must be an assignable ownerstamp destination. For `:=`, the new local is
  bound as `\iso *T`. For `=`, the existing destination must be compatible with
  the returned `\iso *T`.

The outer restore IIFE call is the only call expression allowed by the construct.
No call expression is allowed inside the body.

---

## 3. Argument and Result Semantics

Each `\iso` argument is moved into the restore closure:

- The outside binding is consumed at the call site exactly as with an ordinary
  `\iso` move.
- The corresponding parameter becomes an opened restore root.
- During the body, aliases derived from opened roots are restore-local aliases.
- On normal completion, exactly one `\iso` result is returned and assigned to
  `lhs`.

An opened root that is not part of the returned graph must be consumed, set to
nil, or otherwise proven unreachable from any live restore-local alias. No
opened root may remain available through its old outside binding.

V1 has one returned `\iso`. Multiple return values and multiple restored output
roots are deferred.

---

## 4. Allowed Body Fragment

The body is intentionally a small Go subset.

Allowed statements:

- Local `var` declarations and `:=` declarations.
- Single-target assignments to local variables.
- Single-target assignments to fields reachable from opened roots.
- Assignments of `nil` to restore-local pointer variables or tracked pointer
  fields.
- `if` statements whose condition is made from allowed expressions.
- One final explicit `return expr` statement.

Allowed expressions:

- Identifiers for parameters and locals.
- Selector chains rooted at parameters or locals, such as `x.next`.
- `nil`.
- Parentheses.
- `==` and `!=` comparisons against `nil` or scalar values.
- Boolean combinations of allowed comparisons.
- Non-pointer scalar constants.

Field writes are allowed only when the destination is inside the opened graph and
the field is safe to write:

- Non-pointer scalar fields may be written.
- `\iso` pointer fields may be written with `nil` or restore-local aliases.
- `\imm`, `\mub`, `\rob`, and untracked pointer fields may not be written in v1.

This allows tracked pointer surgery such as:

```go
l = \restore func(l \iso *list, n \iso *node) \iso *list {
    if l.head == nil {
        l.head = n
    } else {
        old := l.head.next
        n.next = old
        l.head.next = n
    }
    return l
}(l, n)
```

It rejects untracked backlink updates such as:

```go
n.prev = l.head // rejected in v1 when prev is plain *node
```

Backlinks need a separate, audited design. They are not part of v1 restore.

---

## 5. Hard Bans

The restore body must not contain:

- Ordinary function calls, method calls, builtin calls, or Gown intrinsic calls.
- `\unsafe`.
- Channel send, receive, `select`, or channel operations of any kind.
- `go`, `defer`, `goto`, labels, `fallthrough`, `break`, or `continue`.
- `for`, `range`, `switch`, or type switch.
- `return` except for the single final statement of the body.
- `panic` or Go's `recover`.
- `new`, `make`, `append`, address-of expressions, composite literals that
  allocate pointer-bearing data, map/slice operations, or indexing into maps or
  slices.
- Interface boxing, type assertions, reflection, or unsafe-package operations.
- Mutation of package globals.
- Assignment through or into untracked pointer fields.
- Storing any restore-local alias into a location outside the opened graph.
- Capturing mutable outer variables.

Captures are rejected except for:

- `\imm` values used read-only.
- Non-pointer scalar constants or locals used read-only.

Even these safe captures should be rare. Passing values as explicit parameters is
preferred because it keeps the restore boundary inspectable.

---

## 6. Boundary Check

At the final return, the checker must prove the restore boundary condition:

1. The returned expression has capability `\iso *T`.
2. Every restore-local pointer alias is dead after the IIFE returns.
3. No restore-local alias was stored outside the opened graph.
4. No restore-local alias was sent, captured by a goroutine, captured by an
   escaping closure, boxed into an interface, passed to a function, or exposed
   through `\unsafe`.
5. Every durable pointer edge in the returned graph respects ownerstamp rules.
6. Each non-nil location in the restored `\iso` graph has at most one durable
   `\iso` owner path.
7. Opened `\iso` roots are either incorporated into the returned graph, consumed,
   nil, or otherwise unreachable from live checked state.
8. No `\imm` location became mutable, and no mutable location gained an `\imm`
   alias.

If any of these facts is uncertain, the checker rejects the restore.

---

## 7. Relationship to `\unsafe`

`\restore` is checked safe code. It must not launder an unsafe proof hole back
into `\iso`.

Any `\unsafe` use inside a restore body is rejected. Any restore-local value that
crosses an untracked boundary is rejected. If a future implementation allows a
trusted observer inside restore, it must prove the observer is non-retaining and
does not receive restore-local pointer aliases. V1 has no such exception.

---

## 8. Proof Contract

The Lean proof models restore as one atomic, same-goroutine re-isolation step.
The step is sound only because the checker enforces the syntactic restrictions
above.

The semantic side conditions are:

- Restore does not change ownership held by other goroutines.
- Restore does not introduce locations beyond the allocation counter.
- The post-restore state has no cross-goroutine mutable reachability.
- The post-restore state has no mutable capability coexisting with `\imm`.
- The result is a single restored `\iso` root in the current goroutine.

The proof does not model Go syntax, heap connectivity, or field paths directly.
Those are checker obligations. The checker must conservatively reject source
programs whenever it cannot justify mapping the restore IIFE to the atomic
restore step.

---

## 9. Future Checker Tests

Acceptance tests should include:

- A tracked linked-list insertion that rewires only `\iso` next pointers.
- A restore with `:=` binding a fresh local `\iso`.
- A restore with an `if` and one final return.

Rejection tests should include:

- Capturing a mutable outer variable.
- Ordinary calls, method calls, builtin calls, and Gown intrinsics in the body.
- Channel send, receive, `select`, `go`, `defer`, `goto`, loops, range, switch,
  labels, early return, and missing final return.
- `\unsafe`, reflection, unsafe-package operations, interface boxing, type
  assertions, address-of, `new`, `make`, `append`, map/slice indexing, and
  pointer-bearing composite literals.
- Mutation of package globals.
- Stores into untracked pointer fields such as linked-list `prev` backlinks.
- Storing a restore-local alias outside the opened graph.
- Returning multiple values or a non-`\iso` result.
- Returning a graph with duplicate durable `\iso` owner paths.

