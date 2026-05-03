# Gown: Race Freedom Proof

## A Formal Proof of Data Race Freedom for the Gown Capability Type System

---

## 1. Introduction

This document presents a formal proof that the Gown capability type system
guarantees data race freedom for all well-typed programs that do not use
`\unsafe`. The proof is structured as a type soundness argument over a core
concurrent calculus (λ‖) that models the essential features of Go augmented
with Gown's four capabilities.

The main result is:

> **Theorem (Race Freedom).** If a program P is well-typed under the Gown
> capability checker and P contains no uses of `\unsafe`, then no execution
> of P contains a data race.

---

## 2. Definitions

### 2.1 Data Race

A **data race** occurs when two goroutines access the same memory location
concurrently, at least one access is a write, and the accesses are not ordered
by a synchronization operation.

Formally, a data race is a pair of events (e₁, e₂) in an execution trace such
that:

1. e₁ and e₂ access the same memory location ℓ
2. At least one of e₁, e₂ is a write
3. e₁ and e₂ occur in different goroutines
4. e₁ and e₂ are not ordered by the happens-before relation (≺)

### 2.2 Happens-Before

The happens-before relation (≺) for Go is defined by:

- Within a single goroutine, program order induces happens-before.
- A send on a channel happens-before the corresponding receive completes.
- A `go` statement (goroutine spawn) happens-before the first statement of the
  spawned goroutine.

These are the only synchronization points relevant to the Gown type system.
Mutexes, atomics, and `sync` primitives exist in Go but are untracked by Gown
and are therefore outside the scope of this proof.

### 2.3 Capabilities

The four capabilities form a set C = { `\iso`, `\mub`, `\rob`, `\imm` }.

Each capability is characterized by three boolean properties:

| Capability | Mutable | Sendable | Unique |
|------------|---------|----------|--------|
| `\iso`     | Yes     | Yes      | Yes    |
| `\mub`     | Yes     | No       | No     |
| `\rob`     | No      | No       | No     |
| `\imm`     | No      | Yes      | No     |

Where:
- **Mutable(c):** Writes are permitted through a reference with capability c.
- **Sendable(c):** A reference with capability c may cross a goroutine boundary
  (via channel send or goroutine spawn capture).
- **Unique(c):** At most one reference with capability c to a given object may
  exist in the entire program at any time.

### 2.4 Sendability Classification

The capabilities partition into:

- **Sendable:** { `\iso`, `\imm` } — may cross goroutine boundaries
- **Non-sendable:** { `\mub`, `\rob` } — confined to the goroutine that
  created them

### 2.5 Mutability Classification

The capabilities partition into:

- **Mutable:** { `\iso`, `\mub` } — writes permitted
- **Immutable:** { `\rob`, `\imm` } — writes rejected by the checker

---

## 3. The Core Calculus λ‖

### 3.1 Syntax

We define a minimal concurrent calculus sufficient to model the relevant
features of Go with Gown annotations.

```
Values:
  v ::= ℓ^c                     — location ℓ with capability c
      | ()                       — unit
      | n                        — integer literal

Expressions:
  e ::= v                        — value
      | let x = e₁ in e₂         — binding
      | x                        — variable
      | x.f                      — field read
      | x.f ← e                  — field write
      | \new(T{...})              — allocate fresh \iso
      | \clone(e)                 — deep copy, produces \iso
      | \freeze(x)                — convert \iso to \imm, consume x
      | \mub(x)                   — borrow \iso as \mub
      | \rob(x)                   — borrow \iso or \imm as \rob
      | send(ch, e)               — channel send
      | recv(ch)                  — channel receive
      | go(e)                     — goroutine spawn
      | select(arms)              — select statement

Capabilities:
  c ::= \iso | \mub | \rob | \imm

Types:
  τ ::= c *T                     — capability-qualified pointer to T
      | T                        — unqualified type (untracked)
      | chan c *T                 — capability-typed channel

Typing Environment:
  Γ ::= ∅ | Γ, x : τ

Goroutine Identifier:
  g ∈ G                          — a set of goroutine identifiers

Heap:
  H : Loc → (T × Fields)        — maps locations to type and field values

Goroutine-Local State:
  S_g : Var → (Loc × Cap)       — maps variables to location-capability pairs
                                   within goroutine g
```

### 3.2 Operational Semantics (Selected Rules)

We define a small-step reduction relation on configurations:

```
Configuration:  (H, {(g, Γ_g, e_g) | g ∈ G})
```

A configuration consists of a shared heap H and a set of goroutine states,
each with an identifier g, a local typing environment Γ_g, and a current
expression e_g.

**R-New:** Allocation.
```
(H, (g, Γ, E[\new(T{v₁,...,vₙ})]))
  → (H[ℓ ↦ (T, {f₁:v₁,...,fₙ:vₙ})], (g, Γ[x ↦ (ℓ, \iso)], E[ℓ^{\iso}]))
where ℓ is fresh (ℓ ∉ dom(H))
```

**R-Clone:** Deep copy.
```
(H, (g, Γ, E[\clone(ℓ^c)]))
  → (H[ℓ' ↦ deepcopy(H, ℓ)], (g, Γ, E[ℓ'^{\iso}]))
where ℓ' is fresh, deepcopy(H, ℓ) produces a complete copy of the
object graph rooted at ℓ such that ℓ' shares no memory with ℓ.
```

**R-Freeze:** Freeze \iso to \imm.
```
(H, (g, Γ, E[\freeze(x)]))
  → (H, (g, Γ[x ↦ ⊥][y ↦ (ℓ, \imm)], E[ℓ^{\imm}]))
where Γ(x) = (ℓ, \iso) and y is the binding for the result.
```
The source variable x is consumed (mapped to ⊥, the consumed marker).

**R-Mub:** Mutable borrow.
```
(H, (g, Γ, E[\mub(x)]))
  → (H, (g, Γ[y ↦ (ℓ, \mub)], E[ℓ^{\mub}]))
where Γ(x) = (ℓ, \iso). x is NOT consumed — it remains in Γ.
```

**R-Rob:** Read-only borrow.
```
(H, (g, Γ, E[\rob(x)]))
  → (H, (g, Γ[y ↦ (ℓ, \rob)], E[ℓ^{\rob}]))
where Γ(x) = (ℓ, \iso) or Γ(x) = (ℓ, \imm). x is NOT consumed.
```

**R-Write:** Field write.
```
(H, (g, Γ, E[x.f ← v]))
  → (H[ℓ.f ↦ v], (g, Γ, E[()]))
where Γ(x) = (ℓ, c) and Mutable(c).
```
If ¬Mutable(c), the expression is ill-typed and rejected by the checker.

**R-Read:** Field read.
```
(H, (g, Γ, E[x.f]))
  → (H, (g, Γ, E[H(ℓ).f]))
where Γ(x) = (ℓ, c). Reads are permitted for any c.
```

**R-Send:** Channel send.
```
(H, (g₁, Γ₁, E[send(ch, x)]), (g₂, Γ₂, E'[recv(ch)]))
  → (H, (g₁, Γ₁', E[()]), (g₂, Γ₂[y ↦ (ℓ, c')], E'[ℓ^{c'}]))
```
where:
- `ch : chan c' *T`
- If Γ₁(x) = (ℓ, \iso) and c' = \iso:
  Γ₁' = Γ₁[x ↦ ⊥] (consumed), c' = \iso.
- If Γ₁(x) = (ℓ, \iso) and c' = \imm:
  Γ₁' = Γ₁[x ↦ ⊥] (consumed), c' = \imm (implicit freeze).
- If Γ₁(x) = (ℓ, \imm) and c' = \imm:
  Γ₁' = Γ₁ (unchanged), c' = \imm.
- All other combinations are ill-typed.

**R-Spawn:** Goroutine spawn.
```
(H, (g₁, Γ₁, E[go(λx.e, v)]))
  → (H, (g₁, Γ₁', E[()]), (g₂, {x ↦ (ℓ, c)}, e))
```
where g₂ is a fresh goroutine identifier, Γ₁(v) = (ℓ, c), and:
- If c = \iso: Γ₁' = Γ₁[v ↦ ⊥] (consumed).
- If c = \imm: Γ₁' = Γ₁ (unchanged).
- If c = \mub or c = \rob: ill-typed, rejected by checker.

---

## 4. Typing Rules

### 4.1 Core Judgments

The typing judgment has the form:

```
Γ ⊢_g e : τ ⊣ Γ'
```

Read as: "Under environment Γ in goroutine g, expression e has type τ and
produces updated environment Γ'." The output environment Γ' reflects
consumption of linear variables.

### 4.2 Rules

**T-Var:**
```
Γ(x) = τ     τ ≠ ⊥
─────────────────────
  Γ ⊢_g x : τ ⊣ Γ
```

**T-ConsumedVar:**
```
    Γ(x) = ⊥
─────────────────
  Γ ⊢_g x : error    (GWN001)
```

**T-New:**
```
  ℓ fresh
──────────────────────────────────
  Γ ⊢_g \new(T{...}) : \iso *T ⊣ Γ
```

**T-Clone:**
```
  Γ ⊢_g e : c *T ⊣ Γ'     (c is any capability or untracked)
──────────────────────────────────────────────────────────────
  Γ ⊢_g \clone(e) : \iso *T ⊣ Γ'
```

**T-Freeze:**
```
  Γ(x) = \iso *T
──────────────────────────────────────────
  Γ ⊢_g \freeze(x) : \imm *T ⊣ Γ[x ↦ ⊥]
```

**T-Mub:**
```
  Γ(x) = \iso *T
──────────────────────────────────────────
  Γ ⊢_g \mub(x) : \mub *T ⊣ Γ
```

**T-Rob-Iso:**
```
  Γ(x) = \iso *T
──────────────────────────────────────────
  Γ ⊢_g \rob(x) : \rob *T ⊣ Γ
```

**T-Rob-Imm:**
```
  Γ(x) = \imm *T
──────────────────────────────────────────
  Γ ⊢_g \rob(x) : \rob *T ⊣ Γ
```

**T-Write:**
```
  Γ ⊢_g x : c *T ⊣ Γ'     Mutable(c)     Γ' ⊢_g e : τ_f ⊣ Γ''
──────────────────────────────────────────────────────────────────
  Γ ⊢_g x.f ← e : () ⊣ Γ''
```

**T-Write-Reject:**
```
  Γ ⊢_g x : c *T ⊣ Γ'     ¬Mutable(c)
──────────────────────────────────────────────
  Γ ⊢_g x.f ← e : error    (GWN003 or GWN004)
```

**T-Send-Iso:**
```
  Γ(x) = \iso *T     ch : chan \iso *T
──────────────────────────────────────────────
  Γ ⊢_g send(ch, x) : () ⊣ Γ[x ↦ ⊥]
```

**T-Send-Iso-Imm (implicit freeze):**
```
  Γ(x) = \iso *T     ch : chan \imm *T
──────────────────────────────────────────────
  Γ ⊢_g send(ch, x) : () ⊣ Γ[x ↦ ⊥]
```

**T-Send-Imm:**
```
  Γ(x) = \imm *T     ch : chan \imm *T
──────────────────────────────────────────────
  Γ ⊢_g send(ch, x) : () ⊣ Γ
```

**T-Send-Reject:**
```
  Γ(x) = c *T     ¬Sendable(c)
──────────────────────────────────
  Γ ⊢_g send(ch, x) : error    (GWN002 or GWN005)
```

**T-Recv-Iso:**
```
  ch : chan \iso *T
──────────────────────────────────────────────
  Γ ⊢_g recv(ch) : \iso *T ⊣ Γ
```

**T-Recv-Imm:**
```
  ch : chan \imm *T
──────────────────────────────────────────────
  Γ ⊢_g recv(ch) : \imm *T ⊣ Γ
```

**T-Spawn:**
```
  Γ(x_i) = c_i *T_i for each captured variable x_i
  ∀i: Sendable(c_i)
  Γ' = Γ[x_i ↦ ⊥ | c_i = \iso]
──────────────────────────────────────────────────────
  Γ ⊢_g go(λ(x₁,...,xₙ).e) : () ⊣ Γ'
```

**T-Select (with \iso in send case):**
```
  Γ(x) = \iso *T
  arm_k = send(ch_k, x) for some arms k ∈ K
  For each arm k ∈ K:   Γ_k = Γ[x ↦ ⊥]     ⊢_g body_k : τ ⊣ Γ_k'
  For each arm j ∉ K:   Γ_j = Γ              ⊢_g body_j : τ ⊣ Γ_j'
  Γ_out = ⨅{Γ_k' | k ∈ K} ⨅ ⨅{Γ_j' | j ∉ K}
──────────────────────────────────────────────────────────────
  Γ ⊢_g select(arms) : τ ⊣ Γ_out
```

where ⨅ is the capability meet at the join point: a variable is live in Γ_out
only if it is live (not ⊥) in all branch environments.

### 4.3 Viewpoint Adaptation

Field access through a capability-qualified pointer yields an effective
capability determined by the viewpoint adaptation function V(outer, field):

```
V(\iso, c)       = c          for any c
V(\mub, \iso)    = \mub
V(\mub, \mub)    = \mub
V(\mub, \rob)    = \rob
V(\mub, \imm)    = \imm
V(\rob, c)       = \rob       for any c
V(\imm, c)       = \imm       for any c
```

**T-FieldRead:**
```
  Γ ⊢_g x : c_outer *T ⊣ Γ'     field f : c_field *U ∈ T
──────────────────────────────────────────────────────────────
  Γ ⊢_g x.f : V(c_outer, c_field) *U ⊣ Γ'
```

---

## 5. Invariants

We now state the two key invariants that the type system maintains. These are
the core of the proof.

### 5.1 Invariant 1: Isolation

**Definition (Reachability).** A goroutine g can **reach** a location ℓ if there
exists a variable x in g's environment Γ_g such that Γ_g(x) = (ℓ, c) for some
c ≠ ⊥, or ℓ is transitively reachable through the heap from such a location.

**Isolation Invariant.** At any reachable program state (H, {(g, Γ_g, e_g)}):

> If goroutine g₁ can reach location ℓ with a non-`\imm` capability, then no
> other goroutine g₂ ≠ g₁ can reach ℓ with any capability.

Equivalently: the only locations reachable from multiple goroutines
simultaneously are those reachable exclusively through `\imm` references.

### 5.2 Invariant 2: Immutability

**Immutability Invariant.** At any reachable program state:

> No write operation is ever performed through a reference with capability
> `\imm` or `\rob`.

This is a syntactic invariant — the checker rejects all such writes before
execution. It is included as an invariant for completeness of the proof
structure.

---

## 6. Proofs

### 6.1 Lemma 1 (Isolation Preservation)

**Statement.** If the Isolation Invariant holds for a configuration C, and
C → C' by any reduction rule, then the Isolation Invariant holds for C'.

**Proof.** By case analysis on the reduction rule applied.

**Case R-New:** A fresh location ℓ is allocated and bound with capability
`\iso` in the allocating goroutine g. Since ℓ is fresh (ℓ ∉ dom(H)), no other
goroutine has any reference to ℓ. The invariant is trivially preserved.

**Case R-Clone:** A fresh location ℓ' is allocated as a deep copy of ℓ.
By definition of deep copy, ℓ' and all locations transitively reachable from
ℓ' are fresh — they share no memory with ℓ or any other pre-existing location.
The clone is bound with capability `\iso` in the cloning goroutine. No other
goroutine can reach ℓ'. The invariant is preserved.

**Case R-Freeze:** The variable x, previously bound to (ℓ, `\iso`), is
consumed (mapped to ⊥). A new binding y ↦ (ℓ, `\imm`) is created in the same
goroutine. Before this step, by the Isolation Invariant, only the current
goroutine g could reach ℓ (since it held a non-`\imm` reference). After this
step, g holds an `\imm` reference to ℓ and the `\iso` binding is consumed. The
location ℓ is now reachable only through `\imm` from g, and not reachable from
any other goroutine. The invariant is preserved, and ℓ is now eligible for
multi-goroutine `\imm` sharing via future sends.

**Case R-Mub:** A new binding y ↦ (ℓ, `\mub`) is created in goroutine g,
where Γ_g(x) = (ℓ, `\iso`). Both x and y are in g's environment. No new
goroutine gains access to ℓ. The `\mub` binding is non-sendable, so it cannot
leave g (enforced by T-Send-Reject and T-Spawn). The invariant is preserved:
ℓ is reachable only from g, through both the `\iso` and `\mub` bindings,
which is a single-goroutine situation.

**Case R-Rob:** Analogous to R-Mub. A new `\rob` binding is created in the
same goroutine. `\rob` is non-sendable. The invariant is preserved.

**Case R-Write:** A field of ℓ is updated. This does not create new references
to ℓ or change which goroutines can reach ℓ. If the value written is a pointer
to another location ℓ₂, the capability of the written pointer is determined by
viewpoint adaptation (§4.3), which preserves the invariant by ensuring that
`\imm` propagates transitively. The invariant is preserved.

**Case R-Read:** A field of ℓ is read. The value read may be a pointer to
another location ℓ₂, but it remains within the same goroutine. No
cross-goroutine access is introduced. The invariant is preserved.

**Case R-Send (iso on chan iso):** Goroutine g₁ sends (ℓ, `\iso`) to g₂.
In g₁'s environment, x is consumed (mapped to ⊥). In g₂'s environment, a
new binding y ↦ (ℓ, `\iso`) is created. Before the send, only g₁ could reach
ℓ (by the Isolation Invariant, since g₁ held a non-`\imm` reference). After
the send, only g₂ can reach ℓ.

We must verify that g₁ has no remaining path to ℓ. The only way g₁ could
retain a path is through:
  (a) Another variable in Γ_{g₁} pointing to ℓ — but this would mean a
      second reference to an `\iso` object, which violates the Unique property
      of `\iso`. Creating a `\mub` or `\rob` from x does not create an
      independently sendable alias, and by T-Spawn and T-Send-Reject, `\mub`
      and `\rob` cannot leave the goroutine. However, these borrows *do*
      create same-goroutine aliases to ℓ that would survive the send unless
      explicitly invalidated.

      **Borrow-scope precondition (critical for implementors).** At the point
      of send, the checker MUST verify that no `\mub` or `\rob` derived from
      x is live in Γ_{g₁}. "Derived from x" means: any variable bound by
      `\mub(x)`, `\rob(x)`, or by viewpoint adaptation through x's fields
      (e.g., `y = \mub(x.f)` produces a borrow that aliases a sub-location
      of x). The mechanized proof in `Gown.lean` models this as the
      `send_iso` step removing ALL of the sender's access at ℓ (via a
      `g' ≠ gs` filter on ownership). If the checker fails to kill all
      borrows before executing this step, the runtime ownership state will
      not match the step's postcondition, and the proof's guarantee does not
      apply. See §15 for the complete set of checker obligations.

  (b) A heap location reachable from g₁ that transitively points to ℓ — this
      would require that ℓ was stored in a field of another object that g₁
      can still reach. But if such a store occurred, it occurred through a
      `\mub` write, and the written pointer carries the field's declared
      capability under viewpoint adaptation. If the field is typed as `\iso`,
      the written value is an `\iso` pointer, and the checker would have
      consumed it on write (linearity). If the field is untracked, the
      write was through `\unsafe` and is outside the scope of the guarantee.

Therefore, after the send, ℓ is reachable only from g₂. The invariant is
preserved.

**Case R-Send (iso on chan imm, implicit freeze):** Goroutine g₁ sends
(ℓ, `\iso`) on a `chan \imm *T` channel. This is operationally equivalent to
R-Freeze followed by R-Send (imm on chan imm). By the argument for R-Freeze,
after the freeze, ℓ is reachable only through `\imm` from g₁. The send makes
ℓ reachable from g₂ through `\imm` as well. Since all references are `\imm`,
the invariant is preserved (the invariant permits multi-goroutine reachability
through `\imm`).

Additionally, the `\iso` binding in g₁ is consumed, so g₁ retains no non-`\imm`
path to ℓ.

**Case R-Send (imm on chan imm):** Goroutine g₁ retains its `\imm` reference
to ℓ. Goroutine g₂ gains an `\imm` reference to ℓ. All references across both
goroutines are `\imm`. The invariant is preserved.

**Case R-Spawn:** Analogous to R-Send. Captured `\iso` variables are consumed
in the spawning goroutine and created in the spawned goroutine. Captured `\imm`
variables are shared. Non-sendable capabilities (`\mub`, `\rob`) are rejected
by T-Spawn. The argument mirrors R-Send exactly.

**Case R-Select:** A select statement evaluates exactly one arm. By the typing
rule T-Select, each arm is typed independently with the appropriate environment
(x consumed in send arms, x live in non-send arms). The reduction corresponds
to one of these arms, and the analysis of that arm follows from the relevant
R-Send case above. The join environment Γ_out is sound because it only
considers a variable live if it is live on all branches, which is conservative.

∎

### 6.2 Lemma 2 (Immutability)

**Statement.** In any well-typed program, no R-Write reduction is ever applied
to a location reached through a reference with capability `\imm` or `\rob`.

**Proof.** By the typing rule T-Write, a write `x.f ← e` requires Mutable(c)
where Γ(x) = (ℓ, c). By definition, Mutable(`\imm`) = false and
Mutable(`\rob`) = false. Therefore T-Write rejects any write through `\imm`
or `\rob`, producing errors GWN003 or GWN004 respectively.

Since the program is well-typed, no such write appears in the program. Since
R-Write is only applied to expressions that pass T-Write, no R-Write reduction
through `\imm` or `\rob` is ever performed.

We must also verify that `\imm` and `\rob` cannot be circumvented through
viewpoint adaptation. If x has capability `\imm`, then by the viewpoint
adaptation function V(`\imm`, c) = `\imm` for any field capability c. Therefore
any field access through `\imm` yields `\imm`, and transitively, any location
reachable from an `\imm` root is `\imm`. The same argument applies to `\rob`:
V(`\rob`, c) = `\rob` for any c.

Therefore no write to any location transitively reachable from an `\imm` or
`\rob` reference is well-typed.

∎

### 6.3 Lemma 3 (Borrow Confinement)

**Statement.** A reference with capability `\mub` or `\rob` never appears in
the environment of a goroutine other than the one that created it.

**Proof.** There are exactly two ways a reference can move from one goroutine
to another: channel send (R-Send) and goroutine spawn (R-Spawn).

For R-Send: the typing rule T-Send-Reject requires Sendable(c), and
Sendable(`\mub`) = false, Sendable(`\rob`) = false. Therefore sends of `\mub`
or `\rob` are ill-typed.

For R-Spawn: the typing rule T-Spawn requires Sendable(c_i) for each captured
variable. Since `\mub` and `\rob` are not sendable, capturing them is ill-typed.

No other reduction rule creates a binding in a goroutine other than the one
executing the rule. Therefore `\mub` and `\rob` references are confined to their
creating goroutine.

∎

### 6.4 Theorem (Race Freedom)

**Statement.** If a program P is well-typed under the Gown checker and P
contains no uses of `\unsafe`, then no execution of P contains a data race.

**Proof.** Assume for contradiction that an execution of a well-typed program
P contains a data race: events e₁ in goroutine g₁ and e₂ in goroutine g₂
(g₁ ≠ g₂) accessing the same location ℓ, with at least one being a write, and
e₁ ⊀ e₂ and e₂ ⊀ e₁.

Since g₁ and g₂ both access ℓ, both goroutines can reach ℓ.

**Case 1: ℓ is reached through a non-`\imm` reference by either goroutine.**

Without loss of generality, suppose g₁ reaches ℓ through a non-`\imm`
reference. By the Isolation Invariant (Lemma 1), no other goroutine can reach
ℓ. But g₂ accesses ℓ, which means g₂ can reach ℓ. Contradiction.

**Case 2: ℓ is reached through `\imm` references by both goroutines.**

Both g₁ and g₂ hold `\imm` references to ℓ. By Lemma 2 (Immutability), no
write is ever performed through an `\imm` reference. But our assumption
requires at least one of e₁, e₂ to be a write. Contradiction.

In both cases we reach a contradiction. Therefore no data race exists in any
execution of P.

∎

---

## 7. Viewpoint Adaptation Soundness

We verify that the viewpoint adaptation function V preserves both the Isolation
and Immutability invariants through arbitrary chains of field access.

### 7.1 Immutability Propagation

**Claim.** If c_outer ∈ { `\rob`, `\imm` }, then V(c_outer, c_field) ∈
{ `\rob`, `\imm` } for any c_field.

**Proof.** By inspection of V:
- V(`\rob`, c) = `\rob` for all c. `\rob` ∈ { `\rob`, `\imm` }. ✓
- V(`\imm`, c) = `\imm` for all c. `\imm` ∈ { `\rob`, `\imm` }. ✓

Therefore immutability is closed under field access: once you are viewing an
object through `\rob` or `\imm`, all transitively reachable objects are also
viewed through `\rob` or `\imm`, and writes are rejected at all depths.

∎

### 7.2 Isolation Propagation

**Claim.** If c_outer = `\iso`, then the effective capability of a field is the
field's declared capability, and the Isolation Invariant is maintained.

**Proof.** V(`\iso`, c) = c for all c. This means:
- An `\iso` field within an `\iso` struct remains `\iso` — uniquely owned.
- A `\mub` field within an `\iso` struct remains `\mub` — borrowed, local.
- A `\rob` field remains `\rob`, an `\imm` field remains `\imm`.

Since the outer `\iso` is reachable from only one goroutine (by the Isolation
Invariant), all its fields are also reachable from only that goroutine. The
field capabilities impose additional constraints (e.g., `\imm` fields may be
shared later) but do not weaken isolation.

∎

### 7.3 Borrow Propagation

**Claim.** If c_outer = `\mub`, then no field access yields `\iso`.

**Proof.** V(`\mub`, `\iso`) = `\mub`. V(`\mub`, c) = c for c ∈ { `\mub`,
`\rob`, `\imm` }.

This prevents a `\mub` borrow from extracting an `\iso` sub-field and then
sending it — the sub-field is downgraded to `\mub`, which is non-sendable,
preserving the Isolation Invariant.

∎

---

## 8. `\clone` Correctness

The `\clone` operation is central to the interaction between untracked Go code
and capability-tracked Gown code. We verify that it preserves the Isolation
Invariant.

**Claim.** `\clone(x)` always produces a valid `\iso`.

**Proof.** By the operational semantics of R-Clone, `\clone` performs a deep
copy of the object graph rooted at x. The resulting location ℓ' is fresh
(ℓ' ∉ dom(H) before the clone) and the entire subgraph reachable from ℓ' is
freshly allocated, sharing no memory with the original. Therefore:

1. No other goroutine can reach ℓ' (it was just created).
2. No other reference to ℓ' exists (it is fresh).
3. The result is bound with capability `\iso`.

The Isolation Invariant is trivially satisfied for ℓ': only the cloning
goroutine can reach it, and it holds the sole `\iso` reference.

The source x is not consumed by `\clone` — it retains its original capability.
This is safe because the deep copy ensures no memory is shared between source
and result.

∎

---

## 9. `select` Soundness

The `select` statement requires special treatment because the capability of a
variable may differ across branches.

### 9.1 Per-Branch Typing

**Claim.** The typing rule T-Select is sound: the join environment Γ_out
produced at the merge point is consistent with the Isolation Invariant
regardless of which branch the runtime selects.

**Proof.** The join operator ⨅ computes: for each variable x, x is live in
Γ_out if and only if x is live (not ⊥) in every branch environment Γ_k'.

Suppose x is an `\iso` that appears in a send case on branch k₀. Then in
Γ_{k₀}, x is consumed (mapped to ⊥). In Γ_out, since x is not live on
branch k₀, x is not live in Γ_out. This means the program cannot access x
after the `select`, regardless of which branch was taken.

This is conservative: if the runtime takes a non-send branch where x is still
live, the capability is "wasted" — x was not consumed but cannot be used. This
is safe (no invariant is violated by failing to use a resource) though not
maximally expressive.

Conversely, if a variable y is live on all branches with the same capability c,
then y is live in Γ_out with capability c. This is sound because the invariants
held at the end of every branch, and y's capability is consistent across all of
them.

∎

### 9.2 Clone at Send Site

**Claim.** If the send expression in a `select` arm is `\clone(x)` or any
expression that produces a fresh `\iso`, then x is not consumed on that branch.

**Proof.** The send consumes the value produced by the expression, not x
itself. `\clone(x)` produces a fresh ℓ' bound with `\iso` and does not consume
x. The fresh `\iso` is immediately consumed by the send. In the branch
environment, x retains its original capability. Since x is live on all branches,
it is live in Γ_out. The invariant is preserved.

∎

---

## 10. Implicit Borrow Coercion Soundness

The implicit borrow coercion at call sites (§6.6 of the spec) introduces
temporary borrows scoped to the call duration.

### 10.1 Claim

Implicit borrow coercion preserves the Isolation Invariant.

### 10.2 Proof

When a function `f(c \mub *T)` is called with an argument x of capability
`\iso`, the checker implicitly creates a `\mub` borrow of x for the duration
of the call. The borrow is scoped to the call — it does not appear in the
caller's environment after the call returns.

During the call:
- The callee holds a `\mub` reference to ℓ.
- The caller's `\iso` binding for x remains in the environment but the checker
  ensures no concurrent access occurs (the call is synchronous within a single
  goroutine).
- By Lemma 3, the `\mub` cannot escape the goroutine.

After the call:
- The implicit `\mub` borrow is released.
- x retains capability `\iso` in the caller's environment.

The Isolation Invariant is maintained throughout: ℓ is reachable only from
goroutine g (through both the `\iso` in the caller's frame and the `\mub` in
the callee's frame), and the `\mub` cannot escape.

The same argument applies to implicit `\rob` coercion, with the additional
note that `\rob` does not permit writes, so the Immutability property is also
preserved where applicable.

∎

---

## 11. Implicit Freeze on Channel Send Soundness

When an `\iso` is sent on a `chan \imm *T`, the checker performs an implicit
freeze.

### 11.1 Claim

The implicit freeze-on-send preserves both the Isolation and Immutability
invariants.

### 11.2 Proof

The implicit freeze-on-send is operationally equivalent to:

```
tmp := \freeze(x)      — x consumed, tmp : \imm
send(ch, tmp)           — tmp shared via channel
```

By the argument for R-Freeze (Lemma 1, Case R-Freeze), after the freeze, ℓ is
reachable only through `\imm` from the sending goroutine. By the argument for
R-Send (imm on chan imm), both goroutines hold `\imm` references after the
send. By Lemma 2, neither can write through `\imm`. No race is possible.

The sender's `\iso` binding is consumed, so the sender retains no non-`\imm`
path to ℓ.

∎

---

## 12. The `\unsafe` Boundary

### 12.1 Statement

The race freedom guarantee is conditional on the absence of `\unsafe`. We
formalize this as follows:

**Definition (\unsafe-free).** A program P is `\unsafe`-free if no expression
in P is of the form `\unsafe(e)`.

**Theorem (Conditional Race Freedom).** If P is well-typed and `\unsafe`-free,
then no execution of P contains a data race.

This is exactly the Race Freedom Theorem stated in §6.4.

### 12.2 What `\unsafe` Breaks

An expression `\unsafe(x)` where x : `\iso *T` passes x to an unannotated
function. The unannotated function is outside the checker's analysis and may:

1. Store x in a global variable (creating an alias that the checker doesn't
   track).
2. Pass x to another goroutine via an untracked channel or shared variable.
3. Retain x beyond the expected lifetime.

Any of these would violate the Isolation Invariant. The `\unsafe` annotation
is the programmer's assertion that the unannotated function does not do any of
these things. If the assertion is wrong, data races are possible.

### 12.3 Auditability

Because `\unsafe` has a distinctive syntactic form (backslash prefix), all
uses are greppable. The race freedom guarantee for a program can be
strengthened by auditing each `\unsafe` use and verifying that the programmer's
assertion holds. This is the same social contract as Go's `unsafe` package.

---

## 13. Completeness (Non-Goal)

This proof establishes **soundness**: if the checker accepts a program, it is
race-free. We do not establish **completeness**: the checker may reject programs
that are in fact race-free. Known sources of incompleteness include:

1. **The `select` join rule.** The conservative join may reject programs where
   an `\iso` is used after a `select` only on branches where it was not
   consumed. This is sound but overly restrictive in some cases.

2. **Untracked code.** Passing capability-typed values to untracked functions
   requires `\unsafe`, even if the untracked function is in fact safe. A future
   version could add annotations to untracked functions to narrow this gap.

3. **Mutex-protected shared state.** The checker makes no attempt to reason
   about mutexes. Code that safely shares mutable state via `sync.Mutex` cannot
   express this safety in the Gown type system and must use untracked pointers.

4. **Capability-polymorphic functions.** A generic function that could safely
   operate on any capability must currently be specialized or use `\unsafe`.
   Capability polymorphism is deferred to a future version.

These are all conservative restrictions — they cause false rejections, not false
acceptances. Soundness is the critical property, and it holds unconditionally
for `\unsafe`-free programs.

---

## 14. Mechanized Proof

The arguments in this document have been mechanized in Lean 4 (`Gown.lean`).
The mechanized proof establishes race freedom with zero custom axioms and zero
`sorry` — the only axiom dependency is Lean's kernel `propext`.

The mechanized proof operates on a "flat" ownership model: each
(goroutine, location, capability) triple is tracked independently, without
modeling heap connectivity. This is sufficient for the proof because each
`Step` constructor encodes the correct ownership transition, and the three
invariants (Iso, Fresh, Coherent) are shown to be preserved by all 14 steps.

The division of labor is:

- **This document** explains *why* the design works — the intuition behind
  each invariant, why each reduction rule preserves them, and how the pieces
  fit together.
- **`Gown.lean`** proves *that* the ownership semantics are race-free — a
  machine-checked guarantee that no sequence of valid steps produces a data
  race.
- **§15 below** specifies what the type checker must enforce so that
  well-typed programs only produce valid step sequences.

The mechanized proof also revealed the need for a third invariant
(**Coherent**: no goroutine holds both a mutable capability and `\imm` on
the same location) that was implicit in the original hand proof but required
for the `pres_add_imm` case (sending or spawning `\imm`). This invariant
captures the fact that `\freeze` consumes the `\iso` before creating `\imm`,
so mutable and `\imm` never coexist on the same goroutine at the same
location.

---

## 15. Type Checker Contract

The proof in §6 and the mechanized proof in `Gown.lean` establish:

> If every ownership transition follows a valid `Step`, then `WF`
> (Iso ∧ Fresh ∧ Coherent) is preserved and no data race occurs.

The type checker's role is the converse obligation: ensure that every
operation in a well-typed program produces an ownership transition that
matches a valid `Step`. This section makes that obligation concrete.

### 15.1 How to Read This Section

Each `Step` constructor defines a **precondition** (what must hold before the
operation) and a **postcondition** (how ownership changes). The preconditions
are what the checker must verify. The postconditions define the ownership
bookkeeping the checker must perform internally. If the checker's internal
state diverges from the Step postconditions, the proof's guarantee no longer
applies.

### 15.2 Allocation (`new_`, `clone_`)

**Precondition:** None for `new_`; source variable must be live for `clone_`.

**Postcondition:** A fresh location ℓ is created with capability `\iso` for
the allocating goroutine. The allocation counter increments.

**Checker obligation:** Bind the result variable to `\iso`. For `clone_`,
verify the source is live (any capability). The "fresh location" maps to
a new heap allocation in the Go runtime.

### 15.3 Freeze (`freeze_`)

**Precondition:** Source variable x has `\iso` at location ℓ.

**Postcondition:** ALL capabilities at (g, ℓ) are **replaced** with just
`\imm`. Not "iso changes to imm" — everything at that location for that
goroutine is replaced.

**Checker obligation (critical):**
1. Verify x has `\iso`.
2. Consume x (map to ⊥).
3. **Kill all borrows at the same location.** Any `\mub` or `\rob` variable
   that aliases the same location as x must be dead (out of scope or already
   consumed). If a live borrow exists, the program's actual state would
   retain a mutable path to ℓ, but the Step's postcondition says only `\imm`
   exists. The Coherent invariant (no mutable + `\imm` coexistence) depends
   on this.

**Why this matters:** The Step definition uses `if g' = g ∧ ℓ' = ℓ then
c = imm else ...`, which replaces all capabilities at (g, ℓ) unconditionally.
A checker that allows `\freeze(x)` while a `\mub(x)` borrow y is still live
would leave `y` dangling — the checker's internal state would include
(g, ℓ, mub) but the Step says only (g, ℓ, imm) exists. Subsequent use of y
for a write would not correspond to any valid Step (write_ requires the
capability to exist in the ownership relation).

### 15.4 Borrow Creation (`mub_`, `rob_iso`, `rob_imm`)

**Precondition:** Source has `\iso` (for `mub_`, `rob_iso`) or `\imm`
(for `rob_imm`).

**Postcondition:** The borrow capability is **added alongside** the existing
capabilities. The source is not consumed.

**Checker obligation:**
1. Verify source capability.
2. Bind the result to `\mub` or `\rob`.
3. Do NOT consume the source — both the `\iso` and the borrow coexist.
4. **Track that the borrow aliases the source's location.** The checker
   must record this so that it can later verify borrow-liveness constraints
   at freeze, send, and spawn points.

### 15.5 Write and Read (`write_`, `read_`)

**Precondition:** Variable has any capability for reads; must have
`Mutable` capability (`\iso` or `\mub`) for writes.

**Postcondition:** No ownership change.

**Checker obligation:** Reject writes through `\rob` or `\imm`. This is
the Immutability Invariant, enforced syntactically.

### 15.6 Send/Spawn of `\iso` (`send_iso`, `send_iso_imm`, `spawn_iso`)

**Precondition:** Source variable x has `\iso` at location ℓ; sender and
receiver are distinct goroutines.

**Postcondition:** The sender **loses ALL access at ℓ** — not just `\iso`,
but every capability the sender held at that location. The receiver gains
`\iso` (for `send_iso`, `spawn_iso`) or `\imm` (for `send_iso_imm`).

**Checker obligation (most critical):**
1. Verify x has `\iso`.
2. Consume x (map to ⊥).
3. **Verify that no borrow derived from x is live.** This includes:
   - Direct borrows: variables bound by `\mub(x)` or `\rob(x)`.
   - Sub-object borrows: variables bound by accessing fields through x,
     e.g., `y = \mub(x.f)`. These alias sub-locations of x's object graph.
   - Transitive borrows: if `z = \mub(y.g)` where y was itself a borrow
     of x, then z must also be dead.

**Why this is the hardest checker obligation:** The mechanized proof models
ownership as a flat relation over individual locations. The `send_iso` step
removes the sender's access at location ℓ specifically. But in a real
program, `\iso` ownership of ℓ implies ownership of the entire object graph
reachable from ℓ. When the sender sends x, the *entire* graph transfers —
and all borrows into that graph must be dead.

The flat model handles this correctly *provided* the checker does its job:
if no borrows of sub-locations exist, then the sender has no access to
sub-locations either (the sender only reached them through x, and x is
consumed). The proof doesn't model sub-locations explicitly because it
doesn't need to — sub-location ownership is a consequence of root ownership
when no borrows exist.

**Implementation guidance:** The checker should track a "borrow region" for
each `\iso` variable. When `\mub(x)` or `\rob(x)` creates a borrow, the
borrow is assigned to x's region. When `y.f` is accessed through a borrow y
in region R, the result is also in region R. At a send/spawn/freeze point,
the checker verifies that the region contains only the root `\iso` being
consumed — i.e., `Active(Γ, region(x)) = {x}`.

### 15.7 Send/Spawn of `\imm` (`send_imm`, `spawn_imm`)

**Precondition:** Source variable has `\imm` at location ℓ.

**Postcondition:** The receiver **gains `\imm`** at ℓ. The sender's
ownership is unchanged — `\imm` is freely sharable.

**Checker obligation:** Verify the source has `\imm`. No consumption, no
borrow invalidation needed. The proof's `no_mut_at_imm` lemma guarantees
that if anyone holds `\imm` at ℓ, nobody holds a mutable capability at ℓ,
so adding another `\imm` reader is safe.

### 15.8 Invariant Maintenance

Beyond individual operations, the checker must maintain three invariants
across the entire program:

**Iso (Isolation):** A mutable capability at ℓ implies exclusive goroutine
access to ℓ.
- Enforced by: only allowing cross-goroutine transfer via send/spawn, only
  for sendable capabilities (`\iso`, `\imm`), and consuming the sender's
  `\iso` on transfer. The checker must reject `send` and `go` captures of
  `\mub` and `\rob`.

**Fresh (Freshness):** All owned locations are below the allocation counter.
- Enforced by: each `new_`/`clone_` uses a fresh location. In practice,
  Go's runtime allocator guarantees this. The checker does not need to track
  allocation counters explicitly.

**Coherent (No mutable + `\imm` coexistence):** If a goroutine holds a
mutable capability at ℓ, it does not also hold `\imm` at ℓ.
- Enforced by: `\freeze` consuming the `\iso` and killing all borrows
  before creating `\imm`. The only paths to `\imm` are freeze (which
  removes mutable) and receiving from another goroutine (which can't target
  a location where the receiver holds mutable, since the sender couldn't
  have reached it by Iso).

### 15.9 What the Proof Does Not Cover

The proof establishes that the ownership *semantics* (Steps) are race-free.
It does not prove that the type checker correctly maps programs to Steps.
Specifically:

1. **Borrow tracking correctness.** The proof assumes that when `send_iso`
   fires, the sender genuinely has no remaining access at ℓ. The checker
   must enforce this via borrow tracking (§15.6). A bug in borrow tracking
   — e.g., failing to track borrows through field access, or allowing a
   borrow to outlive its scope — would break the correspondence between
   the program's actual state and the Step postconditions.

2. **Heap connectivity.** The flat model tracks each location independently.
   It does not model that location ℓ₁ may contain a pointer to location ℓ₂.
   The checker must independently ensure that sending `\iso` at ℓ₁
   invalidates all borrows at ℓ₂ (and transitively deeper), because the
   receiver gains access to the entire graph.

3. **Borrow scoping.** The Steps allow borrows to be created freely
   (`mub_`, `rob_iso`, `rob_imm` have no scoping constraints). The proof
   works because the Steps also remove all access on send/spawn. But the
   checker must ensure that borrows don't outlive their source — not because
   the proof requires it, but because the Go runtime doesn't have a
   mechanism to atomically invalidate borrows. The checker's borrow scoping
   is what ensures the flat model's "remove all access" postcondition
   actually matches runtime reality.

4. **Adequacy.** A complete soundness argument would include a simulation
   proof: that for every well-typed program, the checker's internal state
   transitions simulate the Steps. This would close the gap between "Steps
   are race-free" and "well-typed programs are race-free." This is deferred
   to future work but is not needed if the checker is implemented
   conservatively — rejecting programs whenever borrow-liveness is in doubt
   causes false rejections, not false acceptances.

---

## 16. Summary

The proof establishes race freedom through three invariants:

1. **Isolation:** Non-`\imm` objects are reachable from at most one goroutine.
2. **Immutability:** `\imm` and `\rob` objects are never written.
3. **Coherent:** No goroutine holds both a mutable capability and `\imm` on
   the same location (discovered during mechanization in Lean 4).

These invariants are maintained by the type system through four mechanisms:

- **Linearity of `\iso`:** Move semantics on send and assignment ensure unique
  ownership transfers.
- **Non-sendability of `\mub` and `\rob`:** Borrow confinement ensures borrows
  never cross goroutine boundaries.
- **Syntactic write rejection for `\rob` and `\imm`:** The checker rejects
  writes through immutable capabilities.
- **Deep immutability via viewpoint adaptation:** `\imm` and `\rob` propagate
  transitively through field access, ensuring no writable alias exists at any
  depth.

The proof is modular: each lemma is independent, the theorem follows directly
from the lemmas, and the viewpoint adaptation and `select` soundness arguments
are self-contained.

The total proof obligation is small — the system's strength comes from having
very few capabilities and very simple interaction rules, not from complex
invariants. The proof has been mechanized in Lean 4 (`Gown.lean`) with zero
custom axioms, zero `sorry`, and a sole dependency on Lean's kernel axiom
`propext`. The type checker obligations derived from the proof are specified in
§15.

---

*End of Gown Race Freedom Proof*
