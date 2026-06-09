# Gown Language Specification
## An Ownerstamp-Typed Preprocessor for Go

**Version:** 0.4 (Draft)
**File extension:** `.gown`
**Output:** Plain `.go` files, suitable for any standard Go toolchain

---

## 1. Overview

Gown is a source-to-source preprocessor that adds a four-ownerstamp ownership
type system to Go. An ownerstamp is Gown's term of art for an ownership
annotation: one of `\iso`, `\mub`, `\rob`, or `\imm`. Gown accepts `.gown`
files — valid Go augmented with ownerstamps — and either rejects them with a
structured error, or emits equivalent plain `.go` files with ownerstamps
erased.

The core guarantee is:

> **Race Freedom Theorem:** If a program comprised entirely of `.gown` files
> passes the Gown checker, then no execution of that program contains a data
> race, except through explicit use of `\unsafe`.

Unannotated Go code (`.go` files, the entire stdlib, all existing libraries)
interoperates freely. The race freedom guarantee applies only to code that has
been through the Gown checker. The boundary between checked and unchecked code
is made explicit at call sites.

---

## 2. Design Principles

1. **Minimal ownerstamp set.** Four ownerstamps — `\iso`, `\mub`, `\rob`,
   `\imm` — are necessary and sufficient for a formal race freedom proof with
   ergonomic read-only borrowing. No others are added.

2. **Opt-in, not total.** Unannotated pointers behave exactly as in Go today.
   Existing code compiles and runs unchanged.

3. **`\mub` is the tracked default.** Within ownerstamped code,
   `\mub` is the most natural, least restrictive ownerstamp. It is what you
   reach for first when annotating existing Go code. Untracked pointers remain
   the default for unannotated code.

4. **Annotations on signatures and fields only.** Local variables infer their
   ownerstamp from assignment context. Only function signatures and struct field
   declarations require explicit annotation.

5. **The transpiled output is idiomatic Go.** Ownerstamps erase
   completely. The output should be readable and unsurprising to a Go programmer.

6. **One explicit escape hatch.** `\unsafe` is the only way to cross the
   checked/unchecked boundary with an ownerstamp-tracked value. It is greppable,
   auditable, and semantically equivalent to Go's own `unsafe` bargain.

7. **Backslash-prefix syntax.** All Gown ownerstamps begin with `\`. This
   prevents collision with Go's namespace, makes annotations visually distinct,
   and guarantees that any ownerstamp leaking to the `.go` output causes a Go
   compiler error — fail-fast, no silent miscompilation.

---

## 3. Ownerstamps

### 3.1 `\iso` — Isolated, Uniquely Owned

An `\iso` pointer has exactly one live reference at any point in the program.
The owning goroutine may read and write freely. It may be sent on a channel,
after which the sender's reference is consumed (set to `nil`).

- **Aliasing:** No other `\iso`, `\mub`, `\rob`, or `\imm` alias to the same
  object may exist while an `\iso` reference is live.
- **Sendable:** Yes. Sending on a channel transfers ownership to the receiver.
  The send is a *move*, not a copy.
- **Mutable:** Yes.
- **Field access:** Fields accessed through `\iso` yield their declared
  ownerstamp (see §7, Viewpoint Adaptation).

### 3.2 `\mub` — Mutable Borrow

A `\mub` pointer is a mutable, non-owning, goroutine-local alias. It is the
tracked default — the most natural ownerstamp within ownerstamped code, representing
an ordinary mutable pointer that is confined to one goroutine. Most `\mub`
pointers are derived from an `\iso`, but `\mub` may also be the declared
ownerstamp of a struct field that is intended to remain goroutine-local.

- **Aliasing:** Multiple `\mub` aliases to the same object may exist within
  one goroutine. No `\mub` may be live simultaneously with a send or freeze
  of the underlying `\iso`.
- **Sendable:** No.
- **Mutable:** Yes.
- **Lifetime:** A `\mub` derived from an `\iso` must not outlive that `\iso`.
  The checker enforces this via scope analysis.
- **Inference:** Most local variables that would be `\mub` need not be
  annotated. The checker infers `\mub` for any local pointer derived from an
  `\iso`. Explicit annotation is required only on function signatures and struct
  fields.
- **The tracked default:** Within ownerstamped code, `\mub` represents
  the baseline — mutable, goroutine-local, no special treatment. It is what you
  annotate first when adding ownerstamps to existing Go code.

### 3.3 `\rob` — Read-Only Borrow

A `\rob` (read-only borrow) pointer is a non-owning, goroutine-local alias that
permits reading but not writing. It may be derived from either an `\iso` (for
temporary read access without consuming the `\iso`) or an `\imm` (trivially,
since `\imm` is already immutable).

- **Aliasing:** Multiple `\rob` aliases may exist simultaneously within one
  goroutine. A `\rob` and a `\mub` may coexist on the same object within the
  same goroutine, provided neither escapes.
- **Sendable:** No.
- **Mutable:** No. Any write through a `\rob` pointer, at any depth, is a
  compile-time error.
- **Lifetime:** A `\rob` must not outlive the `\iso` or `\imm` from which it is
  derived. The checker enforces this via scope analysis.
- **Use case:** Passing a read-only view of an `\iso` to a function without
  consuming the `\iso` or freezing it permanently to `\imm`.

### 3.4 `\imm` — Deeply Immutable, Freely Shareable

An `\imm` pointer refers to an object — and the entire transitive closure of
objects reachable from it — that is permanently immutable. Any number of
goroutines may hold `\imm` references simultaneously.

- **Aliasing:** Unrestricted. `\imm` references may be freely copied and shared.
- **Sendable:** Yes. Sending an `\imm` on a channel does not consume the
  sender's reference. Since the value is deeply immutable, sharing it across
  goroutines is always safe.
- **Mutable:** No. Any write through an `\imm` pointer, at any depth, is a
  compile-time error.
- **Deep immutability:** Propagates transitively. A field accessed through
  `\imm` is `\imm` regardless of its declared ownerstamp (see §7).
- **Production:** An `\imm` may only be produced by freezing an `\iso` (see
  §6.3). This guarantees no writable aliases exist at the point of conversion.

### 3.5 Default — Untracked

Any pointer without an ownerstamp is untracked. It behaves exactly as
a regular Go pointer. The checker makes no guarantees about it and imposes no
restrictions on its use. Untracked pointers may not be sent on ownerstamp-typed
channels.

The relationship between untracked and `\mub` is one of explicitness: an
untracked pointer makes no promises; a `\mub` pointer promises goroutine-local
confinement and known provenance.

---

## 4. Ownerstamp Summary

| Ownerstamp  | Mutable | Sendable    | Aliasing          | Goroutine-local |
|-------------|---------|-------------|-------------------|-----------------|
| `\iso`      | Yes     | Yes (move)  | None              | Yes             |
| `\mub`      | Yes     | No          | Local aliases ok  | Yes             |
| `\rob`      | No      | No          | Local aliases ok  | Yes             |
| `\imm`      | No      | Yes (copy)  | Unrestricted      | No              |
| untracked   | Yes     | No†         | Unrestricted      | No              |

† Untracked pointers may not be sent on ownerstamp-typed channels.

---

## 5. Ownerstamp Hierarchy

The four ownerstamps form a partial order. The two ownership forms (`\iso`,
`\imm`) are sendable. The two borrow forms (`\mub`, `\rob`) are goroutine-local.
The two mutable forms (`\iso`, `\mub`) permit writes. The two immutable forms
(`\rob`, `\imm`) prohibit writes.

```
        \iso
       /    \
    \mub    \imm
       \    /
        \rob
          |
      untracked
```

Moving up the hierarchy requires proof of the stronger property: uniqueness for
`\iso`, permanent immutability for `\imm`. Moving down loosens restrictions.

---

## 6. Type Rules

### 6.1 Channel Send

A value sent on a channel must be `\iso` or `\imm`. The channel's element type
must be ownerstamp-typed. An untyped channel may not carry `\iso` or `\imm`
values.

```go
// Legal
ch <- x          // x : \iso *T, ch : chan \iso *T  →  x consumed, x = nil emitted
broadcast <- cfg // cfg : \imm *Config               →  cfg retained by sender

// Illegal
ch <- y          // y is untracked — checker error
```

After an `\iso` send, the variable is consumed. The checker treats any
subsequent use of that variable in the same goroutine as a compile error. The
transpiler emits `x = nil` immediately after the send as a runtime safety net.

### 6.2 Goroutine Spawn

All variables captured by reference in a `go func()` literal must be `\iso`
(consumed by the closure), `\imm` (freely shareable), or not captured at all
(the closure takes a copy of a non-pointer value).

```go
// Legal: \iso transferred into goroutine
go func(r \iso *Request) {
    handle(r)
}(req)           // req consumed here

// Legal: \imm shared freely
go func() {
    process(cfg) // cfg : \imm *Config
}()

// Illegal: \mub or untracked captured by reference into goroutine
go func() {
    x.Field = 1  // x is \mub or untracked — checker error
}()
```

The transpiler emits the closure unchanged. The ownerstamp check is purely
static.

### 6.3 Freeze — `\iso` to `\imm`

An `\iso` pointer may be permanently converted to `\imm` using `\freeze`.
After freezing, the `\iso` reference is consumed and the result is `\imm`. No
write may occur through any alias after this point — guaranteed by linearity of
`\iso` (there are no other aliases to convert).

```go
cfg := buildConfig()       // cfg : \iso *Config
icfg := \freeze(cfg)       // icfg : \imm *Config; cfg consumed
broadcast <- icfg          // legal: \imm is sendable
```

`\freeze` erases to:

```go
icfg := cfg
cfg = nil
```

### 6.4 Borrow Mutable — `\iso` to `\mub`

Within a single goroutine, a `\mub` may be derived from an `\iso` for local
mutation. The `\mub` must not outlive the `\iso` and must not be live at any
point where the `\iso` is sent or frozen.

```go
func populate(r \iso *Request) \iso *Request {
    b := \mub(r)         // b : \mub *Request; explicit form
    b.Header = "..."     // legal: \mub is mutable
    b.Body = readBody()
    return r             // r still \iso; b is out of scope
}
```

`\mub` erases to a plain assignment: `b := r`.

### 6.5 Borrow Read-Only — `\iso` or `\imm` to `\rob`

A `\rob` may be derived from either an `\iso` or an `\imm`. The `\rob` must not
outlive its source and must not be live when the source `\iso` is sent or frozen.

```go
func Inspect(c \rob *Config) string {
    return c.Name        // legal: read through \rob
}

cfg := buildConfig()             // cfg : \iso *Config
name := Inspect(\rob(cfg))       // explicit form; cfg still \iso after call
icfg := \freeze(cfg)             // legal: no \rob aliases live here
```

`\rob` erases to a plain assignment: `c := cfg`.

### 6.6 Implicit Borrow Coercion at Call Sites

Explicit `\mub` and `\rob` calls are only required when creating a named borrow
for use across multiple statements. At function call sites, the checker performs
**implicit borrow coercion**: if the callee's parameter is declared `\mub` or
`\rob`, the checker automatically infers the appropriate borrow from the
argument's ownerstamp, without requiring an explicit call.

The coercion rules at a call site `f(arg)` where the parameter is declared as
`\mub *T` or `\rob *T` are:

| Argument ownerstamp | Parameter declared as | Coercion applied    | Arg after call  |
|---------------------|-----------------------|---------------------|-----------------|
| `\iso`              | `\mub *T`             | implicit `\mub`     | still `\iso`    |
| `\iso`              | `\rob *T`             | implicit `\rob`     | still `\iso`    |
| `\mub`              | `\mub *T`             | none (direct)       | still `\mub`    |
| `\mub`              | `\rob *T`             | implicit read-only  | still `\mub`    |
| `\imm`              | `\rob *T`             | none (direct)       | still `\imm`    |
| `\imm`              | `\mub *T`             | checker error       | —               |
| untracked           | `\mub *T` or `\rob *T`| checker error       | —               |

The key properties are:

- **`\iso` is never consumed by a borrow coercion.** Passing an `\iso` to a
  `\mub` or `\rob` parameter does not move it. The caller retains the `\iso`
  after the call returns. Only a channel send or explicit `\freeze` consumes
  an `\iso`.
- **Coercion is invisible in the transpiled output.** Implicit borrows erase to
  nothing — the argument is passed as-is. The borrow exists only in the
  checker's analysis.
- **The borrow's lifetime is the call duration.** An implicitly coerced borrow
  is live only for the duration of the call. The checker treats it as released
  at the return point, making the source `\iso` available for send or freeze
  immediately after.
- **`\imm` cannot be coerced to `\mub`.** Passing an `\imm` where a `\mub`
  (mutable) parameter is expected is always a checker error — it would require
  silently discarding the immutability guarantee.

```go
func Inspect(c \rob *Config) string { return c.Name }
func Mutate(c \mub *Config)         { c.Timeout = 30 }
func Consume(c \iso *Config)        { /* takes ownership */ }

cfg := \new(Config{...})     // cfg : \iso *Config

// All of these are legal without explicit \rob or \mub:
name := Inspect(cfg)          // implicit \rob coercion; cfg still \iso
Mutate(cfg)                   // implicit \mub coercion; cfg still \iso
Consume(cfg)                  // \iso passed as \iso; cfg consumed (move)

// This is a checker error — cfg was consumed above:
_ = cfg.Name                  // GWN001: use of consumed \iso
```

```go
icfg := \freeze(cfg2)         // icfg : \imm *Config

name := Inspect(icfg)         // legal: \imm → \rob coercion
Mutate(icfg)                  // GWN010: cannot coerce \imm to \mub
```

#### Explicit `\mub` and `\rob` — When Still Needed

The explicit forms remain necessary when a borrow must persist across multiple
statements rather than being scoped to a single call:

```go
// Needs explicit form: borrow spans multiple statements
b := \mub(cfg)          // b : \mub *Config
b.Timeout = 30
b.Retries = 3
b.Name = "updated"
// b goes out of scope here; cfg is \iso again
ch <- cfg               // legal: no \mub aliases live
```

If all mutations can be expressed as a sequence of calls, the implicit form
suffices and is preferred:

```go
// Preferred: implicit coercions, no explicit calls needed
SetTimeout(cfg, 30)     // implicit \mub coercion
SetRetries(cfg, 3)      // implicit \mub coercion
SetName(cfg, "updated") // implicit \mub coercion
ch <- cfg               // legal
```

### 6.7 Assignment

Assigning an `\iso` to another variable is a move, not a copy. After assignment,
the source is consumed.

```go
a := newThing()   // a : \iso *Thing
b := a            // b : \iso *Thing; a consumed
_ = a             // checker error: a was consumed
```

Assigning an `\imm` does not consume the source. Both source and destination
remain valid.

```go
x := frozenThing  // x : \imm *Thing
y := x            // y : \imm *Thing; x still valid
```

### 6.8 Writes Through `\rob` or `\imm`

Any write to a field or element reachable through a `\rob` or `\imm` pointer is
a compile-time error, regardless of how many pointer dereferences are involved.

```go
cfg : \imm *Config
cfg.Timeout = 5        // error: write through \imm
cfg.Sub.Field = "x"    // error: write through \imm (transitive)

rcfg : \rob *Config
rcfg.Timeout = 5       // error: write through \rob
```

### 6.9 Channel Types

A channel's element type may be annotated with an ownerstamp. Only `\iso` and
`\imm` are legal element ownerstamps, because `\mub` and `\rob` are
goroutine-local by definition and sending them on a channel would violate that
constraint. Declaring a `chan \mub *T` or `chan \rob *T` is a checker error.

```go
var work chan \iso *Request       // ownership-transfer channel
var broadcast chan \imm *Config   // shared-immutable channel
var plain chan *Request           // untracked channel — no ownerstamp checking
var done \imm chan \iso *Request  // stable channel handle carrying \iso values
```

The channel variable itself usually requires no ownerstamp. Channels
are concurrency-safe by construction in Go — the runtime mediates all sends and
receives — so a channel value may be freely shared across goroutines without
ownerstamp tracking.

When a channel handle is stored in a field that must be read after the
containing `\iso` object has moved, the field itself must be declared `\imm`.
This means the channel handle slot is stable: it may be read and shared, but
not reassigned. It does not make channel operations read-only; sends and
receives are still allowed because the Go channel runtime synchronizes the
channel's internal state.

```go
type Ticket struct {
    Done \imm chan \iso *Ticket
}

worker <- t
t = <-t.Done        // legal: Done is a stable \imm channel handle
```

Without the `\imm` on `Done`, reading `t.Done` after `t` has moved would be a
use-after-move error. The new owner could reassign the field concurrently,
which would be an ordinary race on the field slot even though the old and new
channel handles themselves are safe.

#### Send Compatibility

The ownerstamp of the argument must be compatible with the channel's element
ownerstamp. The full matrix:

| Argument | Channel element | Result |
|----------|-----------------|--------|
| `\iso *T` | `chan \iso *T` | Ownership transfer. Sender's reference consumed. |
| `\iso *T` | `chan \imm *T` | Implicit freeze + send. Sender's reference consumed. Receiver gets `\imm`. |
| `\imm *T` | `chan \imm *T` | Deeply immutable — safe to share. Sender retains `\imm`. |
| `\imm *T` | `chan \iso *T` | Checker error — cannot produce `\iso` from `\imm`. |
| `\mub *T` | any | Checker error — `\mub` is goroutine-local. |
| `\rob *T` | any | Checker error — `\rob` is goroutine-local. |
| untracked | ownerstamp chan | Checker error — untracked on ownerstamp channel. |
| any | untracked chan | Plain Go semantics, no ownerstamp checking. |

When an `\iso` is sent on a `chan \imm *T`, the implicit freeze is equivalent to
`\freeze` followed by a send — the `\iso` is permanently converted to `\imm`
and the sender's reference is consumed. If the sender wants to retain a readable
reference after sending, it must freeze explicitly before the send:

```go
// Sender wants to keep reading after publish
icfg := \freeze(cfg)       // cfg consumed, icfg : \imm
broadcast <- icfg          // icfg retained — \imm is copyable
useLocally(icfg)           // legal
```

#### Receive

The receiver gets a value with the channel's element ownerstamp:

```go
x := <-work       // x : \iso *Request — receiver now owns it
c := <-broadcast   // c : \imm *Config — receiver gets immutable view
```

For `chan \iso *T`, the receive is an ownership acquisition — the receiver gets
the sole `\iso` reference. For `chan \imm *T`, the receive produces an `\imm`
that can be freely shared onward.

#### `select` with Ownerstamp-Typed Sends

The behavior of `select` follows directly from the ownerstamp of the value
being sent. There are two cases:

**Sending `\imm` in a `select`:** Nothing special happens. `\imm` is not
consumed on send — it is deeply immutable, so sharing is always safe. Whether
the send succeeds or falls
through to `default`, the sender retains its `\imm` reference. The `\imm` is
available after the `select` unconditionally.

```go
select {
    case broadcast <- cfg:   // cfg : \imm, retained
    default:                 // cfg : \imm, retained
}
// cfg still \imm here — always valid
```

**Sending `\iso` in a `select`:** If any branch of a `select` sends an `\iso`
value directly (not via `\clone` or another `\iso`-returning expression), the
checker must conservatively assume the `\iso` was consumed after the `select`.
This is because at least one branch moves the `\iso`, and the checker cannot
know at compile time which branch the runtime will take.

The same `\iso` may appear in multiple send cases — the runtime guarantees
exactly one case fires, so the linearity invariant is preserved.

The checker tracks ownerstamps per-branch:

- In a branch where the `\iso` appears in a send case: the `\iso` is consumed.
- In a branch where the `\iso` does not appear in a send case (including
  `default`): the `\iso` retains its pre-`select` ownerstamp.
- After the `select`: the `\iso` is not available, because it is not live on all
  branches.

```go
select {
    case ch1 <- x:
        // x consumed — was sent on ch1
    case ch2 <- x:
        // x consumed — was sent on ch2
    default:
        // x still \iso — send did not happen
}
// x not available here — consumed on at least one branch
```

This means the programmer must handle the `\iso` within each branch where it is
still live. On send branches, it is consumed — there is nothing to do. On
non-send branches (including `default`), the `\iso` is still live and may be
used, sent elsewhere, frozen, or left unused:

```go
select {
    case ch <- x:
        // x consumed
    default:
        retryQueue = append(retryQueue, x)  // x still \iso, used here
}
```

To produce a consistent binding after the `select`, assign a result on every
branch:

```go
var y \iso *Item
select {
    case ch <- x:
        y = buildFallback()
    default:
        y = x               // x still \iso, moved to y
}
// y is \iso here — assigned on both paths
processItem(y)
```

#### Recommended Pattern: clone at Send Site

For code adapted from existing Go, the simplest `select` pattern is to clone at
the send site rather than sending the original `\iso`:

```go
select {
    case ch <- \clone(x):
        // a fresh copy was sent; x unchanged
    default:
        // x unchanged
}
// x still available here — was never at risk
```

Because `\clone` produces a fresh `\iso` that is immediately consumed by the
send, the original value's ownerstamp is unaffected regardless of which branch
fires. This eliminates the need for per-branch tracking entirely.

More generally, the checker recognizes that an `\iso` is *not* consumed by a
`select` send if the send expression is a function call or expression that
produces a fresh `\iso` — such as `\clone(x)`, `\new(...)`, or any function
returning `\iso *T`. In these cases the original variable does not participate
in the send and retains its ownerstamp across the `select`.

---

## 7. Viewpoint Adaptation

When a field is accessed through an ownerstamp-qualified pointer, the effective
ownerstamp of the field is the "meet" of the outer ownerstamp and the field's
declared ownerstamp. The outer ownerstamp takes precedence where it is more
restrictive.

| Outer \ Field declared as | `\iso`  | `\mub`  | `\rob`  | `\imm`  | untracked |
|---------------------------|---------|---------|---------|---------|-----------|
| `\iso`                    | `\iso`  | `\mub`  | `\rob`  | `\imm`  | untracked |
| `\mub`                    | `\mub`  | `\mub`  | `\rob`  | `\imm`  | untracked |
| `\rob`                    | `\rob`  | `\rob`  | `\rob`  | `\rob`  | `\rob`    |
| `\imm`                    | `\imm`  | `\imm`  | `\imm`  | `\imm`  | `\imm`    |
| untracked                 | —       | —       | `\rob`  | `\imm`  | untracked |

The `\rob` row propagates `\rob` everywhere — you can read but not write through
a read-only borrow, regardless of what the fields declare. The `\imm` row
propagates `\imm` — deep immutability is unconditional. The untracked row cannot
produce `\iso` or `\mub` — accessing such a field through an untracked pointer
is a checker error unless the field is `\rob` or `\imm`.

An effective `\imm` field projection may be read even after the outer `\iso`
root has moved. This is sound because the field slot is immutable and therefore
the new owner cannot race by changing that slot. Other fields of the moved root
remain unavailable.

```go
type Ticket struct {
    Done    \imm chan \iso *Ticket
    Outcome string
}

out <- t
next := <-t.Done     // legal: stable immutable projection
_ = t.Outcome        // error: ordinary field read after move
```

---

## 8. The `\unsafe` Escape Hatch

Passing an ownerstamp-tracked value to an unannotated function (stdlib, external
library, legacy code) requires an explicit `\unsafe` annotation at the call
site. The same rule applies when returning, assigning, storing, sending, or
erasing a tracked value into an untracked Go location: once a value is in the
ownerstamp-typed universe, it can leave only through `\unsafe`.

```go
json.Marshal(\unsafe(cfg))   // cfg : \iso *Config
                             // programmer asserts callee does not alias
```

Compiler built-ins such as `println` are treated as non-retaining operations,
not as ordinary untracked callees.

`\unsafe` erases to its argument in the transpiled output. In the current
checker it is conservative: the boundary itself is allowed, but it ends the
local ownerstamp proof for the argument. Later operations that require the old
proof, such as sending the same `\iso` or returning it as a tracked result,
are rejected with `GWN012` and a note pointing back to the `\unsafe` site. Its
presence is auditable via grep. It carries the same social contract as Go's
`unsafe`: you are asserting a property the checker cannot verify.

The race freedom guarantee holds for all code that does not use `\unsafe`.

For repeated non-retaining observer calls, a source file may declare a trusted
observer target with a line directive:

```go
\\\\observer fmt.Printf
\\\\observer debugTicket
```

Arguments passed to matching observer calls do not require per-argument
`\unsafe` wrappers. The directive must appear on its own line, names either a
function or selector, and may include an optional trailing `()`. It applies only
to argument checking for the call itself; checked return values, sends, stores,
and other ownerstamp boundaries are still enforced normally. The transpiler
preserves byte and line alignment by emitting the directive as a comment:

```go
//\\observer fmt.Printf
```

---

## 9. Built-ins

All Gown built-ins use the backslash prefix. They are preprocessor constructs —
they do not exist as Go functions and are fully erased on output.

| Built-in         | Input         | Output        | Erases to             |
|------------------|---------------|---------------|-----------------------|
| `\new(...)`      | struct literal| `\iso *T`     | `&T{...}`             |
| `\clone(x)`      | `T` or `*T` with same-type `clone` | fresh `\iso` same type | `(x).clone()` |
| `\mub(x)`        | `\iso *T`     | `\mub *T`     | plain assignment      |
| `\rob(x)`        | `\iso *T`     | `\rob *T`     | plain assignment      |
| `\rob(x)`        | `\imm *T`     | `\rob *T`     | plain assignment      |
| `\freeze(x)`     | `\iso *T`     | `\imm *T`     | `dst := x; x = nil`  |
| `\swap(a, b)`    | two `\iso` assignable places of identical type | no value | `a, b = b, a` |
| `\unsafe(x)`     | any           | same          | `x`                   |

`\new` is the preferred constructor for `\iso` values. It guarantees the
returned pointer is freshly allocated with no existing aliases.

`\clone` performs a trusted user-defined copy and returns a fresh `\iso`. If
`x` has static type `T`, that exact type must define `clone() T`; if `x` has
static type `*T`, that exact type must define `clone() *T`. In v1, `T` must be
a named struct type. The source may have any ownerstamp, including untracked,
and is not consumed. The returned `\iso` is trusted to share no mutable memory
with the original. The transpiler emits `(x).clone()`.

`\swap(a, b)` exchanges the contents of two assignable `\iso` owner cells. Both
arguments must be locals or field places with identical static Go types and
effective ownerstamp `\iso`. The operation has no result value and emits Go's
ordinary simultaneous assignment, `a, b = b, a`. Root/field overlap is allowed;
Go's assignment evaluation order defines the exact operation.

---

## 10. Interfaces

Ownerstamps may appear on interface method signatures.

```go
type Handler interface {
    Handle(r \iso *Request) \imm *Response
    Inspect(c \rob *Config) string
}
```

Storing an ownerstamp-typed value in an `any` (empty interface) is a checker
error unless the value is explicitly wrapped in `\unsafe`. A future version of
the spec may introduce ownerstamp-parameterized interfaces.

---

## 11. Transpiler Output

The transpiler emits a single `.go` file for each `.gown` file, preserving line
numbers where possible.

| Construct                  | Transpiled to                  |
|----------------------------|--------------------------------|
| Ownerstamp qualifier       | Erased from type               |
| `\iso` send on channel     | Send + `x = nil`               |
| `\iso` assignment (move)   | Assignment + `src = nil`       |
| `\freeze(x)`               | `dst := x; x = nil`           |
| `\mub(x)`                  | `b := x`                      |
| `\rob(x)`                  | `r := x`                      |
| `\clone(x)`                | `(x).clone()`                  |
| `\new(...)`                | `&T{...}`                      |
| `\unsafe(x)`               | `x`                            |

No runtime support library is required. The transpiled output has zero
additional dependencies.

---

## 12. Syntax

Gown introduces ownerstamp keywords as type qualifiers prefixed with `\`. They
appear before the `*` in a pointer type.

```
CapType = ( "\iso" | "\mub" | "\rob" | "\imm" ) "*" Type
```

### Examples

```go
// Function signatures
func NewConfig() \iso *Config
func ProcessConfig(c \imm *Config) Result
func MutateLocally(c \mub *Config)
func Inspect(c \rob *Config) string

// Struct fields
type Pipeline struct {
    Config  \imm *Config
    Buffer  \iso *Buffer
    Done    \imm chan \iso *Request // stable channel handle, \iso elements
    Name    string          // untracked, no annotation needed
}

// Channel types
var ch chan \iso *Request
var broadcast chan \imm *Config
var done \imm chan \iso *Request

// Variables (annotation optional; inferred from RHS)
var x \iso *Config = NewConfig()
x := NewConfig()             // infers \iso from return type
```

### Annotation Placement Rules

- **Function return types:** annotated at the return type position
- **Function parameters:** annotated at the parameter type position
- **Struct fields:** annotated at the field type position
- **Channel element types:** annotated inside the `chan` type
- **Stable channel handles:** annotated before the `chan` type, usually as
  `\imm chan ...`
- **Local variables:** inferred; annotation is allowed but not required
- **Interface method signatures:** annotated at each parameter and return type

---

## 13. Error Model

The checker produces structured errors with file, line, error code, description,
and where possible a suggested fix.

| Code    | Meaning                                                                  |
|---------|--------------------------------------------------------------------------|
| GWN001  | Use of consumed `\iso` after an ownership move, send, call, return, freeze, defer, or assignment |
| GWN002  | Conflicting borrow or live named borrow blocks a move/freeze             |
| GWN003  | Non-sendable `\mub` or `\rob` value sent across a channel                |
| GWN004  | Borrow escape through goroutine argument or capture                      |
| GWN005  | Write through a read-only or immutable viewpoint                         |
| GWN006  | `\mub`/`\rob` borrow or borrowing closure stored into an escaping location |
| GWN007  | Returned borrow or returned closure capturing a non-shareable tracked value |
| GWN008  | Ownerstamp-tracked value passed to an untracked call/parameter without `\unsafe` |
| GWN009  | Ownerstamp-tracked value erased into an interface without `\unsafe`      |
| GWN010  | Channel/value mismatch, invalid coercion, or tracked value stored/returned/sent to untracked Go without `\unsafe` |
| GWN011  | Attempted ownership move from a field projection                         |
| GWN012  | Ownerstamp proof frontier violation after explicit `\unsafe`, with a note at the earlier frontier |

---

## 14. Toolchain Integration

### Build

```bash
gown ./...              # check packages; source .go files are not overwritten
gown -watch ./...       # incremental, re-check on file change
```

### go generate

```go
//go:generate gownc ./...
```

### File layout

```
mypkg/
  config.gown       ← ownerstamped source (authored)
  config.go         ← transpiled output (generated, committed)
  handler.gown
  handler.go
  util.go           ← plain Go, no preprocessor involved
```

Plain `.go` files in the same package coexist freely and are untracked from the
ownerstamp checker's perspective. The committed `.go` files are valid Go and
gopls operates on them normally.

---

## 15. Formal Foundations

The race freedom proof is structured as a type soundness argument over a core
concurrent calculus (λ‖).

**Key judgments:**

```
Γ ⊢ e : \iso T     — uniquely owned
Γ ⊢ e : \mub T     — mutable borrow, goroutine-scoped
Γ ⊢ e : \rob T     — read-only borrow, goroutine-scoped
Γ ⊢ e : \imm T     — deeply immutable
```

**Isolation Lemma:** At any reachable program state, no two goroutines hold a
reference to the same heap location unless all such references are `\imm`.

**Immutability Lemma:** No write ever occurs through a reference whose
ownerstamp is `\imm` or `\rob` at its introduction point.

**Race Freedom Theorem:** Follows from the two lemmas. A data race requires
concurrent access with at least one write. The Isolation Lemma rules out
concurrent access to non-`\imm` objects. The Immutability Lemma rules out writes
to `\imm` and `\rob` objects. Together they rule out all races.

The proof follows the Wright-Felleisen (progress and preservation) methodology
adapted for a concurrent reduction semantics. The formal development is deferred
to a companion technical report.

### Primary Literature

- Gordon et al., "Uniqueness and Reference Immutability for Safe Parallelism,"
  OOPSLA 2012 — race freedom proof technique for capability systems over GC'd
  languages; closest prior work to this design
- Boyland, Noble, Retert, "Capabilities for Sharing," ECOOP 2001 — foundational
  capability formalism
- Clebsch et al., "Deny Capabilities for Safe, Fast Actors," AGERE 2015 —
  Pony's capability system and its simplification rationale
- Sergey and Clarke, "Gradual Ownership Types," ESOP 2012 — formal treatment of
  the checked/unchecked boundary

---

## 16. Out of Scope (v0.4)

- **Cycle collection.** `\iso` and `\imm` objects are GC-managed by Go's
  collector. No changes to cycle handling are made.
- **New concurrency primitives.** Go's goroutines and channels are used as-is.
- **Ownerstamp-parameterized interfaces.** Deferred to a future version.
- **Generics.** Interaction between ownerstamp types and Go generics is deferred
  to a future version.
- **Mutable shared state.** Mutexes, atomics, and `sync` primitives remain
  fully available and fully untracked.
- **Cross-goroutine borrowing.** `\mub` and `\rob` are strictly
  goroutine-local. Cross-goroutine borrowing is not supported.

---

*End of Gown v0.4 Specification*
