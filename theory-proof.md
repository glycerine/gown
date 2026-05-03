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
      of `\iso`. The checker enforces uniqueness: creating a `\mub` or `\rob`
      from x does not create an independently sendable alias, and by T-Spawn
      and T-Send-Reject, `\mub` and `\rob` cannot leave the goroutine. At
      the point of send, the checker requires that no `\mub` or `\rob`
      derived from x is live (this is the borrow scope constraint from §6.4
      of the spec).
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

## 14. Summary

The proof establishes race freedom through two invariants:

1. **Isolation:** Non-`\imm` objects are reachable from at most one goroutine.
2. **Immutability:** `\imm` and `\rob` objects are never written.

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
are self-contained. The total proof obligation is small — the system's strength
comes from having very few capabilities and very simple interaction rules,
not from complex invariants.

---

*End of Gown Race Freedom Proof*
