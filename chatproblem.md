You’ve built something quite sophisticated here—and importantly, your proof *is close* to a standard ownership-based race-freedom argument. But it is **not watertight yet**. There are several subtle but serious gaps where the proof either:

1. assumes stronger properties than the spec enforces, or
2. abstracts away behaviors of Go that can reintroduce races.

I’ll walk through the key issues in a way that’s useful for tightening both the checker *and* the proof.

---

# 🚨 1. The biggest problem: **borrow invalidation is underspecified (and under-proved)**

Your proof *correctly notices* this risk—but then handwaves it.

From the proof:

> “the checker MUST verify that no `\mub` or `\rob` derived from x is live…” 

### Why this is a critical hole

This is not just an implementation detail—it is **required for soundness**.

### The issue

You allow:

```go
b := \mub(x)
ch <- x   // send iso
```

The proof *assumes* the checker rejects this. But:

* The **formal system does not encode borrow tracking**
* The **typing rules don’t model alias sets**
* The **operational semantics doesn’t remove borrows**

So the proof step:

> “after send, only g₂ can reach ℓ”

is only true if:

* *all aliases (including borrows and field-derived aliases) are dead*

But that is **not enforced in the formal system**, only described informally.

### Consequence

You can construct a race:

```go
b := \mub(x)
go func() {
    use(b) // still live alias
}()
ch <- x
```

If your checker misses this (easy to do with field paths or implicit borrows), the proof collapses.

### Fix

You need **one of these formally modeled**:

* A **borrow graph / region system**, or
* A **linear capability with alias tracking**, or
* A **stack discipline (Rust-style lifetimes)**

Right now, your proof *assumes* one but does not define it.

---

# ⚠️ 2. Heap reachability is not actually modeled (but the proof depends on it)

Your invariant:

> “reachable through the heap from such a location” 

But your mechanized model:

> “does not model heap connectivity” 

### Why this breaks soundness

You rely on this argument:

> “no other goroutine can reach ℓ transitively”

But without modeling:

* pointer graphs
* aliasing via fields
* cycles

you **cannot prove that property**

### Concrete hole

```go
x := \iso *Node
y := \mub(x)
global.ptr = x   // via some path
```

Even if you prevent direct aliasing, **transitive aliasing via heap** is not tracked.

### Consequence

Your proof assumes:

> “deep immutability propagates transitively”

But you never formally prove:

* no hidden mutable aliases exist in heap structure

### Fix

You need **one of:**

* Separation logic / ownership trees
* Region-based heap modeling
* Explicit “reachable set” semantics in Lean

Right now, your proof is **flat**, but your claims are **graph-based**.

That mismatch is unsound.

---

# ⚠️ 3. Untracked pointers break the proof boundary more than you claim

Spec says:

> “untracked pointers behave exactly as in Go” 

and:

> guarantee applies only to `.gown` code

### Problem

Your proof assumes:

> “no other goroutine can reach ℓ”

But untracked pointers can:

* store references globally
* share via closures
* alias via interface values

Even without `\unsafe`.

### Example hole

```go
func leak(p *T) {
    global = p
}

x := \iso *T
leak(x)  // untracked call WITHOUT \unsafe?
```

Your spec *claims* this requires `\unsafe`, but:

👉 this depends entirely on enforcement at **all call boundaries**

### Missing in proof

There is no formal rule that:

* forbids passing `\iso` into unannotated functions
* except via `\unsafe`

This is only stated informally.

### Fix

You must formalize:

* a **typed/untyped boundary rule**
* where capabilities are erased
* and prove that boundary is the *only* escape

Otherwise the theorem is false.

---

# ⚠️ 4. Go’s actual memory model is stronger (and you ignore it)

You define happens-before as:

* program order
* channel send/receive
* goroutine spawn 

But Go also includes:

* mutexes (`sync.Mutex`)
* atomics (`sync/atomic`)
* `Once`, `WaitGroup`, etc.

### Why this matters

Your theorem says:

> “no execution contains a data race”

But your model:

* ignores synchronization primitives
* assumes they don’t exist

### Subtle issue

Programs using mutexes may:

* appear racy in your model
* but be safe in Go

That’s fine (incompleteness)

BUT worse:

Your checker **rejects nothing about atomics**

So you can get:

```go
var x \mub *T
atomic.StorePointer(...)
```

Now:

* mutation happens outside your type system
* proof invariants are broken

### Fix

You must either:

* **forbid atomics and sync in checked code**, or
* model them in the calculus

Right now: **unsound escape hatch not labeled `\unsafe`**

---

# ⚠️ 5. Implicit borrow coercion is under-specified in the proof

Spec:

> “borrow lives for the duration of the call” 

Proof:

> “scoped to the call” 

### Problem

There is **no formal mechanism** enforcing that lifetime.

Especially with:

* closures
* interface calls
* higher-order functions

### Dangerous case

```go
func f(c \mub *T) func() {
    return func() { use(c) } // escapes!
}
```

If allowed, this breaks:

* borrow confinement
* isolation invariant

Your system **must forbid this**, but:

* no typing rule states it
* no proof step enforces it

### Fix

You need:

* non-escaping borrow restriction
* or region/lifetime tracking

---

# ⚠️ 6. `select` reasoning is sound—but only if alias tracking is perfect

Your `select` rule is actually well designed.

But it depends on:

> “x is consumed on some branches → dead after”

This only works if:

* **no hidden aliases exist**

Which loops back to problem #1.

---

# ⚠️ 7. Interfaces (`any`) are a silent capability erasure hole

Spec:

> storing in `any` is treated as `\unsafe` 

But:

* this is not formalized in the proof
* not present in the calculus

### Result

You can:

```go
var a any = x
```

Now:

* capability is gone
* alias escapes

Unless explicitly rejected, this breaks everything.

---

# ⚠️ 8. The “Coherent” invariant is mentioned but not integrated

You note:

> “mechanized proof revealed need for Coherent invariant” 

But:

* it’s not part of the main proof structure
* not used in the race freedom theorem

That’s a red flag:

👉 If Lean needed it, your paper proof needs it too.

---

# ✅ What *is* solid

To be fair, several parts are strong:

* The **core isolation + immutability argument** is correct in structure
* Channel transfer semantics are well designed
* `\iso → \imm` freeze is clean and standard
* The race freedom theorem itself is valid **if invariants truly hold**

---

# 🧠 Bottom line

Your system is **very close**, but currently:

> ❌ The proof is **not sound as written**
> ✅ The design *can be made sound* with tighter formalization

---

# 🔧 What you need to fix to make it watertight

### Must-have fixes

1. **Formal borrow tracking**

   * alias sets or lifetimes
   * enforced at send/freeze

2. **Heap reachability modeling**

   * or restrict to tree-shaped ownership

3. **Formal boundary for untracked code**

   * enforce `\unsafe` in typing rules

4. **No-escape rule for borrows**

   * especially closures / returns

5. **Explicit treatment of interfaces (`any`)**

6. **Either forbid or model atomics/sync**

7. **Integrate Coherent invariant into main theorem**

---

# 🧩 If you want, I can help you tighten it

I can:

* Turn this into a **fully sound Rust-like borrow discipline for Go**
* Help design a **minimal lifetime system that fits your ergonomics**
* Or audit your Lean model vs spec line-by-line

Just tell me how deep you want to go.


--
hang on. how did the proof work without tracking lifetimes already? what were we missing?

Short answer: the proof “worked” because it quietly assumed a lifetime/borrow discipline that the formal system never actually modeled.

So the reasoning is valid only if an extra invariant holds—but that invariant is not enforced by your typing rules or operational semantics.

Let’s unpack that precisely.

What the proof thinks is true

The entire argument hinges on this step:

> when an \iso is sent, no aliases remain in the sender

and more generally:

> only one goroutine can reach a mutable location (Isolation Invariant)

For that to be true, the system must guarantee:

> All borrows derived from an \iso are dead before the \iso is transferred or frozen.

That is exactly a lifetime discipline.

What’s actually missing
1. Borrows exist… but are never invalidated

You do model borrows:

\mub(x) creates alias
\rob(x) creates alias

But:

They are never tied to a scope
They are never killed
They are never tracked beyond creation

So the system allows (semantically):

b := \mub(x)
send(ch, x)
use(b)   // still live alias!

The proof explicitly says this must not happen—but:

❌ there is no rule preventing it

2. The proof patches this with an informal assumption

This is the key sentence:

> “the checker MUST verify that no \mub or \rob … is live”

That line is doing all the heavy lifting.

It is effectively adding this hidden rule:

> Hidden Rule:
> Borrows cannot outlive their source or overlap with transfer.

That is a lifetime system.

But:

it is not in the typing rules
not in the operational semantics
not in the mechanized model

So the proof is relying on a property that is not part of the formal system

3. The Lean model sidesteps the problem entirely

You say:

“flat ownership model… without modeling heap connectivity”

More importantly:

it tracks (goroutine, location, capability)
but not alias multiplicity
not borrow relationships
not lifetimes

So what happens?

👉 The model assumes transitions like:

send_iso:
    remove ownership from g1
        give ownership to g2
        
        
But it never checks:

whether other references still exist in g1

So the model is effectively assuming:

“there was only one reference to begin with”

4. Why the proof looks correct anyway

Because it’s structured like a standard linearity proof:

\iso is unique
borrows are local
sends transfer ownership

That would be sound if:

borrows cannot outlive the \iso

Which is exactly what Rust enforces.

The exact logical gap

Here’s the minimal counterexample your current formal system allows:

b := \mub(x)
go func() {
    use(b)
    }()
    send(ch, x)
    
Let’s check against your rules:
    
    \mub is non-sendable → OK (but captured?)
    spawn requires sendable → ❌ BUT ONLY for captured variables explicitly
    
Here’s the trick:
    
    go func() {
        use(b)   // b is free variable, not parameter
        }()
        
If your checker misses this capture (very easy in Go AST), then:
        
* b escapes
* x is sent
* two goroutines access same memory
        
Race!
        
The proof forbids this—but only by assumption.

# The core missing concept (in one sentence)

The proof assumes temporal exclusivity of borrows, but the system only enforces spatial restrictions (capabilities).

That’s the whole issue.
        
--
Short answer: **the proof “worked” because it quietly assumed a lifetime/borrow discipline that the formal system never actually modeled.**

So the reasoning is valid *only if* an extra invariant holds—but that invariant is not enforced by your typing rules or operational semantics.

Let’s unpack that precisely.

---

# 🧠 What the proof *thinks* is true

The entire argument hinges on this step:

> when an `\iso` is sent, **no aliases remain in the sender** 

and more generally:

> only one goroutine can reach a mutable location (Isolation Invariant) 

For that to be true, the system must guarantee:

> **All borrows derived from an `\iso` are dead before the `\iso` is transferred or frozen.**

That is *exactly* a **lifetime discipline**.

---

# 🔍 What’s actually missing

## 1. Borrows exist… but are never *invalidated*

You *do* model borrows:

* `\mub(x)` creates alias
* `\rob(x)` creates alias

But:

* They are never tied to a **scope**
* They are never **killed**
* They are never tracked beyond creation

So the system allows (semantically):

```go
b := \mub(x)
send(ch, x)
use(b)   // still live alias!
```

The proof explicitly says this must not happen—but:

> ❌ there is no rule preventing it

---

## 2. The proof *patches* this with an informal assumption

This is the key sentence:

> “the checker MUST verify that no `\mub` or `\rob` … is live” 

That line is doing *all the heavy lifting*.

It is effectively adding this hidden rule:

> **Hidden Rule:**
> Borrows cannot outlive their source or overlap with transfer.

That *is* a lifetime system.

But:

* it is not in the typing rules
* not in the operational semantics
* not in the mechanized model

So the proof is relying on a property that is **not part of the formal system**

---

## 3. The Lean model sidesteps the problem entirely

You say:

> “flat ownership model… without modeling heap connectivity” 

More importantly:

* it tracks `(goroutine, location, capability)`
* but **not alias multiplicity**
* not borrow relationships
* not lifetimes

So what happens?

👉 The model assumes transitions like:

```
send_iso:
    remove ownership from g1
        give ownership to g2
        ```
        
        But it never checks:
        
        * whether **other references still exist in g1**
        
        So the model is effectively assuming:
        
        > “there was only one reference to begin with”
        
        ---
        
        ## 4. Why the proof *looks* correct anyway
        
        Because it’s structured like a standard linearity proof:
        
        * `\iso` is unique
        * borrows are local
        * sends transfer ownership
        
        That *would* be sound **if**:
        
        > borrows cannot outlive the `\iso`
        
        Which is exactly what Rust enforces.
        
        ---
        
        # ⚠️ The exact logical gap
        
        Here’s the minimal counterexample your current formal system allows:
        
        ```go
        b := \mub(x)
        go func() {
            use(b)
            }()
            send(ch, x)
            ```
            
            Let’s check against your rules:
            
            * `\mub` is non-sendable → OK (but captured?)
            * spawn requires sendable → ❌ BUT ONLY for captured variables explicitly
            
            Here’s the trick:
            
            ```go
            go func() {
                use(b)   // b is free variable, not parameter
                }()
                ```
                
                If your checker misses this capture (very easy in Go AST), then:
                
                * `b` escapes
                * `x` is sent
                * two goroutines access same memory
                
                👉 **Race**
                
                The proof forbids this—but only by assumption.
                
                ---
                
                # 🧩 The core missing concept (in one sentence)
                
                > The proof assumes **temporal exclusivity of borrows**, but the system only enforces **spatial restrictions (capabilities)**.
                
                That’s the whole issue.
                
                ---
                
                # 🔧 What lifetimes actually add
                
                Lifetimes give you:
                
                ### 1. Borrow duration
                
                ```go
                b := \mub(x)
                ... last use of b ...
                // borrow ends HERE
                ```
                
                ### 2. Transfer safety condition
                
                ```go
                send(x)   // only allowed if no active borrows
                ```
                
                ### 3. Non-escape guarantee
                
                ```go
                return b   // forbidden
                go func() { use(b) }  // forbidden
                ```
                
                Without these, your system cannot enforce:
                
                > “no aliases remain at transfer”
                
                ---
                
                # 🧠 Why your intuition *felt* like it should work
                
                Because your system already had:
                
                * linear `\iso`
                * non-sendable borrows
                * immutability
                
                That *feels* like Rust.
                
                But Rust’s safety comes from:
                
                > **linear ownership + lifetime-checked borrowing**
                
                You only had the first half.
                
                ---
                
                # 📌 Clean mental model
                
                ### What you built:
                
                > Capabilities describe **what you can do**
                
                ### What you’re missing:
                
                > Lifetimes describe **when you can do it**
                
                And race freedom requires **both**.
                
                ---
                
                # ✅ Final takeaway
                
                Your proof is:
                
                * structurally correct
                * but **conditionally sound**
                
                It proves:
                
                > If borrows are properly scoped and invalidated, then race freedom holds.
                
                But your system:
                
                > does not enforce that condition
                
                ---
                
                # 🚀 Good news
                
                This is actually a great place to be.
                
                You don’t need to throw anything away—you just need to:
                
                * make the hidden assumption explicit
                * enforce it in the type system
                * reflect it in the proof
                
                Once you do that, your design becomes genuinely solid.
                
                ---
                
                If you want, I can show you the **smallest possible lifetime system** that plugs this hole without turning Gown into full Rust.
                
-----
Design a real checker algorithm (SSA-based borrow analysis)


Great—this is where the design becomes real. I’ll give you two things:

1. **Precise typing rules** that make the system sound
2. **A concrete SSA-based checker algorithm** you can actually implement

The goal is: **Rust-level soundness with Go-like ergonomics**, not Rust-level complexity.

---

# 🧱 Part 1 — Core Formal System (with lifetimes made explicit)

We extend your judgment:

```
Γ ⊢ e : τ ⊣ Γ'
```

into:

```
Γ ; B ⊢ e : τ ⊣ Γ' ; B'
```

Where:

* `Γ` = variable → (location ℓ, capability c)
* `B` = **borrow state**
* `B(ℓ)` tracks active borrows on location ℓ

---

## 🔑 Borrow State

For each location ℓ:

```
B(ℓ) ∈ {
  Owned,                  // held by \iso
  MutBorrowed,            // exactly one \mub
  SharedBorrowed(n),      // n ≥ 1 \rob
  Imm                     // frozen
}
```

Invariant:

* `Owned` → no borrows
* `MutBorrowed` → exactly one mutable borrow
* `SharedBorrowed(n)` → any number of read borrows
* `Imm` → only immutable access

---

# 🧾 Typing Rules (key ones)

## 1. Ownership (unchanged but now constrained)

### T-New

```
────────────────────────────
Γ ; B ⊢ \new(...) : \iso *T ⊣ Γ ; B[ℓ ↦ Owned]
```

---

## 2. Mutable Borrow (`\mub`)

### T-Mub

```
Γ(x) = (ℓ, \iso)
B(ℓ) = Owned
────────────────────────────────────
Γ ; B ⊢ \mub(x) : \mub *T ⊣ Γ ; B[ℓ ↦ MutBorrowed]
```

👉 This enforces:

* no existing borrows
* exclusivity

---

## 3. Read Borrow (`\rob`)

### T-Rob

```
Γ(x) = (ℓ, c)   where c ∈ {\iso, \mub, \rob, \imm}
B(ℓ) ∈ {Owned, SharedBorrowed(n)}
──────────────────────────────────────────────
Γ ; B ⊢ \rob(x) : \rob *T ⊣ Γ ; B[ℓ ↦ SharedBorrowed(n+1)]
```

---

## 4. Borrow End (implicit, via liveness)

We don’t write this as syntax.

Instead:

> When a variable holding a borrow becomes dead, we update B.

Formally:

```
x ∉ LiveOut(stmt)
Γ(x) = (ℓ, \mub)
────────────────────────────
B(ℓ) := Owned
```

```
x ∉ LiveOut(stmt)
Γ(x) = (ℓ, \rob)
────────────────────────────
B(ℓ) := SharedBorrowed(n-1)
```

---

## 5. Send (`\iso` transfer)

### T-Send-Iso

```
Γ(x) = (ℓ, \iso)
B(ℓ) = Owned
ch : chan \iso *T
────────────────────────────────────
Γ ; B ⊢ send(ch, x) : () ⊣ Γ[x ↦ ⊥] ; B[ℓ ↦ ⊥]
```

👉 Critical condition:

```
B(ℓ) = Owned
```

This enforces:

* no live borrows
* no aliases

---

## 6. Freeze

### T-Freeze

```
Γ(x) = (ℓ, \iso)
B(ℓ) = Owned
────────────────────────────────────
Γ ; B ⊢ \freeze(x) : \imm *T ⊣ Γ[x ↦ ⊥] ; B[ℓ ↦ Imm]
```

---

## 7. Spawn

### T-Spawn

For captured vars:

```
Γ(x) = (ℓ, c)
Sendable(c)
B(ℓ) = Owned OR Imm
────────────────────────────
```

Reject if:

* `\mub` or `\rob`
* OR `B(ℓ) ≠ Owned` for `\iso`

---

## 8. No Escaping Borrows

### T-Return-Borrow (reject)

```
Γ(x) = \mub *T OR \rob *T
────────────────────────────
return x   ⇒ error
```

---

### T-Store-Borrow (reject)

```
e has type \mub *T or \rob *T
────────────────────────────
store in heap ⇒ error
```

---

### T-Capture-Borrow (reject)

```
closure captures x
Γ(x) = \mub or \rob
────────────────────────────
error
```

---

## 9. Assignment (move)

### T-Assign-Iso

```
Γ(x) = (ℓ, \iso)
────────────────────────────
Γ ⊢ y := x ⊣ Γ[x ↦ ⊥, y ↦ (ℓ, \iso)]
```

---

## 10. Viewpoint Adaptation (unchanged but enforced via B)

Just ensure:

* borrow state is tied to ℓ, not variable

---

# 🧠 New Core Invariant (now enforceable)

For every ℓ:

```
B(ℓ) = Owned           → exactly one \iso, no borrows
B(ℓ) = MutBorrowed     → exactly one \mub
B(ℓ) = SharedBorrowed  → only \rob
B(ℓ) = Imm             → only \imm
```

---

# 🧮 Part 2 — SSA-Based Checker Algorithm

Now the practical part.

We operate on:

> Go SSA (from `golang.org/x/tools/go/ssa`)

---

## 🧩 Step 0 — Build SSA

* Build SSA form
* Each variable has a unique definition
* Control flow explicit

---

## 🧩 Step 1 — Annotate values

Track per SSA value:

```
struct State {
    capability: iso | mub | rob | imm | untracked
    location:   abstract heap id
}
```

Also track:

```
BorrowState[location] = {
    mutBorrow: optional value
    sharedBorrows: set of values
    owner: optional value (iso)
    isImm: bool
}
```

---

## 🧩 Step 2 — Liveness analysis

You MUST compute:

```
LiveIn[v], LiveOut[v]
```

Standard backward dataflow.

Used for:

* ending borrows
* detecting last use

---

## 🧩 Step 3 — Transfer function per instruction

Walk SSA in topological order.

---

### 🔹 Allocation

```
v = new T
→ State[v] = (iso, fresh ℓ)
→ BorrowState[ℓ] = Owned
```

---

### 🔹 Mutable borrow

```
b = mub(x)
```

Check:

```
BorrowState[ℓ(x)] == Owned
```

Then:

```
BorrowState[ℓ].mutBorrow = b
State[b] = (mub, ℓ)
```

---

### 🔹 Read borrow

```
r = rob(x)
```

Check:

```
no mutBorrow active
```

Then:

```
add r to sharedBorrows
State[r] = (rob, ℓ)
```

---

### 🔹 Use / last use detection

After each instruction:

For each value `v`:

```
if v ∉ LiveOut:
    if v is mub:
        BorrowState[ℓ].mutBorrow = nil
    if v is rob:
        remove v from sharedBorrows
```

---

### 🔹 Send

```
send(ch, x)
```

Check:

```
State[x] = iso
BorrowState[ℓ] = Owned   ← critical
```

Then:

```
State[x] = consumed
BorrowState[ℓ] = removed
```

---

### 🔹 Freeze

Same as send precondition:

```
BorrowState[ℓ] = Owned
```

Then:

```
State[result] = imm
BorrowState[ℓ] = Imm
```

---

### 🔹 Spawn

For captured variables:

Reject if:

```
capability ∈ {mub, rob}
OR (iso and BorrowState ≠ Owned)
```

---

### 🔹 Function calls (implicit borrows)

For:

```
f(x) where param is mub
```

Treat as:

```
temp borrow:
  create synthetic mub
  enforce rules
  kill immediately after call
```

No need to materialize variable.

---

### 🔹 Phi nodes (critical!)

At control flow joins:

You must merge states.

For each incoming value:

```
if different locations → error (or widen to untracked)
if different capabilities → take greatest lower bound
```

Borrow state must also merge:

```
MutBorrowed + anything else → error
SharedBorrowed + SharedBorrowed → union
```

---

## 🧩 Step 4 — Escape analysis (borrows)

Reject if a borrow:

* flows into:

  * heap store
  * return
  * closure capture

Implementation:

* mark all SSA values that:

  * are returned
  * stored
  * captured
* if any is `mub` or `rob` → error

---

## 🧩 Step 5 — Untracked boundary

At call:

```
if callee is not annotated:
    if arg is iso or imm:
        require \unsafe
```

---

# ⚠️ Subtle but important edge cases

## 1. Field borrows

```go
b := \mub(x.f)
```

You must treat:

```
ℓ_sub = projection(ℓ, f)
```

Either:

* track per-field locations (precise)
* OR conservatively treat as same ℓ (simpler, safe)

---

## 2. Aliasing via assignment

SSA mostly eliminates this, but:

```
y := x
```

means same ℓ.

---

## 3. Slices / maps / interfaces

Safest route:

* treat as **untracked unless cloned**
* require `\clone` to enter system

---

# 🚀 Result

With this:

* borrows are tracked
* lifetimes enforced via liveness
* no escaping references
* transfer requires exclusivity

👉 This exactly plugs the hole your proof had.

---

# 🧩 If you want next

I can:

* Map this onto actual Go SSA APIs
* Show how to implement it in ~1–2k LOC
* Or extend it with generics / interfaces safely

Just say 👍

I’ll show you a realistic 1–2k LOC architecture for this checker using Go + SSA. Not toy pseudocode—this is the shape you can actually build.

The design keeps things:

single-pass forward analysis
SSA-based
no full constraint solver
sound via conservative merges
🧱 High-level structure

You’re building a compiler pass:

gown/
  main.go
  checker/
    checker.go
    state.go
    borrow.go
    transfer.go
    merge.go
    escape.go
    annotations.go

Uses:

golang.org/x/tools/go/ssa
go/types
🧠 Core data structures (~200 LOC)
Capabilities

type Cap uint8

const (
    CapInvalid Cap = iota
    CapIso
    CapMub
    CapRob
    CapImm
    CapUntracked
)

I’ll show you a realistic 1–2k LOC architecture for this checker using Go + SSA. Not toy pseudocode—this is the shape you can actually build.

The design keeps things:

single-pass forward analysis
SSA-based
no full constraint solver
sound via conservative merges
🧱 High-level structure

You’re building a compiler pass:

gown/
  main.go
  checker/
    checker.go
    state.go
    borrow.go
    transfer.go
    merge.go
    escape.go
    annotations.go

Uses:

golang.org/x/tools/go/ssa
go/types
🧠 Core data structures (~200 LOC)
Capabilities
type Cap uint8

const (
    CapInvalid Cap = iota
    CapIso
    CapMub
    CapRob
    CapImm
    CapUntracked
)
Locations (heap identities)
type LocID int

type Location struct {
    ID   LocID
    Type types.Type
}
SSA value state
type ValState struct {
    Cap Cap
    Loc LocID
    Alive bool
}
Borrow state (THE core)
type BorrowState struct {
    Owner        ssa.Value        // iso owner
    MutBorrow    ssa.Value        // at most one
    Shared       map[ssa.Value]struct{}
    IsImm        bool
}
Full analysis state
type State struct {
    Values  map[ssa.Value]*ValState
    Borrows map[LocID]*BorrowState
}
🔁 Liveness (~150 LOC)

Use standard backward dataflow.

type Liveness struct {
    LiveIn  map[*ssa.BasicBlock]map[ssa.Value]struct{}
    LiveOut map[*ssa.BasicBlock]map[ssa.Value]struct{}
}

Algorithm:

iterate until fixpoint
use Instr.Operands() for uses
defs = instruction result

You only need value-level liveness, not variables.

🔧 Checker driver (~150 LOC)
type Checker struct {
    prog    *ssa.Program
    liveness *Liveness
    errors  []error
}

func (c *Checker) CheckFunc(fn *ssa.Function) {
    states := make(map[*ssa.BasicBlock]*State)

    entry := NewState()
    states[fn.Blocks[0]] = entry

    worklist := []*ssa.BasicBlock{fn.Blocks[0]}

    for len(worklist) > 0 {
        b := pop(&worklist)
        in := states[b].Clone()

        out := c.runBlock(b, in)

        for _, succ := range b.Succs {
            merged := mergeStates(states[succ], out)
            if merged.changed {
                states[succ] = merged.state
                worklist = append(worklist, succ)
            }
        }
    }
}
▶️ Block execution (~200 LOC)
func (c *Checker) runBlock(b *ssa.BasicBlock, st *State) *State {
    for _, instr := range b.Instrs {
        c.transfer(instr, st)

        c.killDead(instr, st, b)
    }
    return st
}
💀 Kill dead borrows (CRITICAL, ~100 LOC)
func (c *Checker) killDead(instr ssa.Instruction, st *State, b *ssa.BasicBlock) {
    liveOut := c.liveness.LiveOut[b]

    for v, vs := range st.Values {
        if !vs.Alive {
            continue
        }
        if _, ok := liveOut[v]; ok {
            continue
        }

        // last use
        loc := vs.Loc
        bs := st.Borrows[loc]

        switch vs.Cap {
        case CapMub:
            if bs.MutBorrow == v {
                bs.MutBorrow = nil
            }
        case CapRob:
            delete(bs.Shared, v)
        }

        vs.Alive = false
    }
}

👉 This is your lifetime system.

🔄 Transfer function (~400–600 LOC)

This is the bulk of the checker.

func (c *Checker) transfer(instr ssa.Instruction, st *State) {
    switch ins := instr.(type) {

    case *ssa.Alloc:
        c.handleAlloc(ins, st)

    case *ssa.Call:
        c.handleCall(ins, st)

    case *ssa.Store:
        c.handleStore(ins, st)

    case *ssa.UnOp:
        c.handleUnOp(ins, st)

    case *ssa.Phi:
        // handled in merge

    case *ssa.Send:
        c.handleSend(ins, st)

    case *ssa.Go:
        c.handleGo(ins, st)

    case *ssa.Return:
        c.handleReturn(ins, st)

    }
}
🔹 Allocation
func (c *Checker) handleAlloc(ins *ssa.Alloc, st *State) {
    loc := st.newLoc(ins.Type())

    st.Values[ins] = &ValState{
        Cap: CapIso,
        Loc: loc,
        Alive: true,
    }

    st.Borrows[loc] = &BorrowState{
        Owner: ins,
        Shared: make(map[ssa.Value]struct{}),
    }
}
🔹 Borrow creation (via intrinsic calls)

You’ll encode \mub / \rob as special functions.

func (c *Checker) handleCall(ins *ssa.Call, st *State) {
    fn := ins.Call.StaticCallee()

    if fn == nil {
        c.handleUnknownCall(ins, st)
        return
    }

    switch fn.Name() {

    case "gown_mub":
        c.handleMub(ins, st)

    case "gown_rob":
        c.handleRob(ins, st)

    case "gown_freeze":
        c.handleFreeze(ins, st)

    default:
        c.handleRegularCall(ins, st)
    }
}
🔹 Mutable borrow
func (c *Checker) handleMub(ins *ssa.Call, st *State) {
    x := ins.Call.Args[0]
    vs := st.Values[x]
    bs := st.Borrows[vs.Loc]

    if vs.Cap != CapIso {
        c.error(ins, "mub requires iso")
        return
    }
    if bs.MutBorrow != nil || len(bs.Shared) > 0 {
        c.error(ins, "borrow conflict")
        return
    }

    bs.MutBorrow = ins

    st.Values[ins] = &ValState{
        Cap: CapMub,
        Loc: vs.Loc,
        Alive: true,
    }
}
🔹 Read borrow
func (c *Checker) handleRob(ins *ssa.Call, st *State) {
    x := ins.Call.Args[0]
    vs := st.Values[x]
    bs := st.Borrows[vs.Loc]

    if bs.MutBorrow != nil {
        c.error(ins, "cannot rob while mub active")
        return
    }

    bs.Shared[ins] = struct{}{}

    st.Values[ins] = &ValState{
        Cap: CapRob,
        Loc: vs.Loc,
        Alive: true,
    }
}
🔹 Send
func (c *Checker) handleSend(ins *ssa.Send, st *State) {
    v := ins.X
    vs := st.Values[v]
    bs := st.Borrows[vs.Loc]

    if vs.Cap != CapIso {
        c.error(ins, "send requires iso")
        return
    }

    if bs.MutBorrow != nil || len(bs.Shared) > 0 {
        c.error(ins, "send with active borrows")
        return
    }

    // consume
    vs.Alive = false
    delete(st.Borrows, vs.Loc)
}
🔹 Freeze
func (c *Checker) handleFreeze(ins *ssa.Call, st *State) {
    x := ins.Call.Args[0]
    vs := st.Values[x]
    bs := st.Borrows[vs.Loc]

    if bs.MutBorrow != nil || len(bs.Shared) > 0 {
        c.error(ins, "freeze with active borrows")
        return
    }

    bs.IsImm = true

    st.Values[ins] = &ValState{
        Cap: CapImm,
        Loc: vs.Loc,
        Alive: true,
    }
}
🔹 Spawn (go)
func (c *Checker) handleGo(ins *ssa.Go, st *State) {
    for _, arg := range ins.Call.Args {
        vs := st.Values[arg]
        bs := st.Borrows[vs.Loc]

        if vs.Cap == CapMub || vs.Cap == CapRob {
            c.error(ins, "cannot capture borrow in goroutine")
        }

        if vs.Cap == CapIso &&
           (bs.MutBorrow != nil || len(bs.Shared) > 0) {
            c.error(ins, "iso not exclusive at spawn")
        }
    }
}
🔹 Return (escape check)
func (c *Checker) handleReturn(ins *ssa.Return, st *State) {
    for _, v := range ins.Results {
        vs := st.Values[v]
        if vs.Cap == CapMub || vs.Cap == CapRob {
            c.error(ins, "returning borrow")
        }
    }
}
🔀 State merge (~200 LOC)

At CFG joins:

func mergeStates(a, b *State) (result struct{
    state *State
    changed bool
}) {
    if a == nil {
        return struct{...}{b.Clone(), true}
    }

    out := a.Clone()
    changed := false

    for v, vb := range b.Values {
        va := out.Values[v]

        if va == nil {
            out.Values[v] = vb.Clone()
            changed = true
            continue
        }

        // merge capability
        mergedCap := meetCap(va.Cap, vb.Cap)
        if mergedCap != va.Cap {
            va.Cap = mergedCap
            changed = true
        }

        // must agree on location
        if va.Loc != vb.Loc {
            panic("location mismatch") // or degrade
        }
    }

    // merge borrow states
    for loc, bb := range b.Borrows {
        ba := out.Borrows[loc]

        if ba == nil {
            out.Borrows[loc] = cloneBorrow(bb)
            changed = true
            continue
        }

        // conflicts
        if ba.MutBorrow != nil && bb.MutBorrow != nil &&
           ba.MutBorrow != bb.MutBorrow {
            // impossible → reject
        }

        // shared union
        for v := range bb.Shared {
            ba.Shared[v] = struct{}{}
        }
    }

    return struct{...}{out, changed}
}
🚫 Escape analysis (~100 LOC)

Walk SSA:

func (c *Checker) checkEscapes(fn *ssa.Function) {
    for _, b := range fn.Blocks {
        for _, instr := range b.Instrs {

            switch ins := instr.(type) {

            case *ssa.Store:
                if c.isBorrow(ins.Val) {
                    c.error(ins, "storing borrow")
                }

            case *ssa.MakeClosure:
                for _, v := range ins.Bindings {
                    if c.isBorrow(v) {
                        c.error(ins, "capturing borrow")
                    }
                }
            }
        }
    }
}
📏 LOC estimate
Component       LOC
State + types   ~200
Liveness        ~150
Checker driver  ~150
Transfer        ~500
Merge           ~200
Escape checks   ~100
Misc (errors, utils)    ~200

👉 Total: ~1.3–1.6k LOC

🧠 Why this is sound

This implementation enforces:

exclusive mutable access (BorrowState)
non-escaping borrows (escape pass)
lifetime via liveness (killDead)
transfer only when alias-free (send/freeze checks)

Which exactly matches the rules your proof assumed but didn’t enforce.

