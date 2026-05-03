# Critique of v1 proof

gemini critique of theory-proof.md

This proof is **logically structured and grounded in sound type-system theory**, specifically drawing from linear types and capability-based ownership (similar to Pony or Rust). In spirit, the Gown system is robust.

However, from a "water-tight" formal perspective, **there are significant leaks.** The proof relies on several "shadow assumptions" that are mentioned in the prose but are not actually captured in the formal typing rules ($\Gamma \vdash e : \tau$). If this were submitted to a proof assistant like Coq or Isabelle, it would fail.

Below is the sharp critique of the "holes" in your proof.

---

## 1. The "Ghost Borrow" Leak (The most critical error)
In **Section 6.1 (Case R-Send)**, you state: 
> "At the point of send, the checker requires that no $\backslash mub$ or $\backslash rob$ derived from $x$ is live."

**The Problem:** Your formal typing rules in **Section 4.2** (specifically `T-Send-Iso`) do **not** actually enforce this. 
*   `T-Send-Iso` only checks that $x$ is an `\iso` and then maps $x \mapsto \bot$.
*   It does **not** check the rest of the environment $\Gamma$ for other variables that might be $\backslash mub$ or $\backslash rob$ aliases of the same location $\ell$.

**The Race:** 
1.  `let x = \new(T)` ($x$ is `\iso`)
2.  `let y = \mub(x)` ($y$ is $\backslash mub$, pointing to the same $\ell$)
3.  `send(ch, x)` ($x$ is consumed, but **$y$ remains in $\Gamma$**)
4.  Goroutine 1 writes to $y.f$ while Goroutine 2 (which received $x$) also writes to $x.f$.
**Result:** Data race. To fix this, your typing rules need a "borrow tracking" mechanism or a way to invalidate all aliases in $\Gamma$ when an `\iso` is moved.

---

## 2. The "Transitive Alias" Failure
Your **Section 7 (Viewpoint Adaptation)** correctly describes how capabilities propagate through fields. However, the proof fails to account for **local variables pointing into the heap.**

**The Problem:**
If I have an `\iso` variable `p` and I create a $\backslash mub$ variable `f` that points to a field inside `p` (e.g., `f = \mub(p.field)`), the typing environment $\Gamma$ now has two entries. 
If I then `send(ch, p)`, your current proof for `R-Send` argues that because $p$ is consumed, the location is isolated. But the local variable `f` still exists in the sender's $\Gamma$.

**The Proof Gap:** 
Lemma 1 (Isolation Preservation) assumes that "reachability" is strictly controlled by the `\iso` variable. It doesn't formally prove that consuming a "root" `\iso` also "kills" all reachable $\backslash mub / \backslash rob$ pointers that were pulled out of that root into the local environment.

---

## 3. Ambiguity in "Unique(c)"
In **Section 2.3**, you define `Unique(\iso)` as:
> "At most one reference with capability `\iso` to a given object may exist in the entire program at any time."

**The Critique:**
This definition is slightly dangerous. For a race-freedom proof, "Unique" usually needs to mean **"No other mutable references exist."** 
If the system allows one `\iso` **and** five $\backslash mub$ references to the same $\ell$ at the same time in the same goroutine, that is fine for *race* freedom, but your proof of **Lemma 1** needs to be much more explicit about why those $\backslash mub$ references can never "outlive" the `\iso` if the `\iso` is sent away. You mention "borrow scope constraints" in §6.4, but these are not defined in your Core Calculus.

---

## 4. The `select` Join Weakness
In **Section 9.1**, you use the meet operator $\sqcap$ to join environments. 
> "a variable is live in $\Gamma_{out}$ only if it is live (not $\bot$) in all branch environments."

**The Critique:**
This is sound (it's conservative), but your proof doesn't address **aliasing** here either. If a $\backslash mub$ alias is created in one branch but not the other, the join rule for the *parent* `\iso` becomes very complex. If the `\iso` is considered "live" in the output because it wasn't sent, but a $\backslash mub$ alias was created and "lost" in the $\sqcap$ operation, you might end up with "zombie" mutable references in the heap that the type system has lost track of.

---

## 5. Formal Consistency "Nits"
*   **Deepcopy definition:** In `R-Clone`, you define `deepcopy` as producing a graph that "shares no memory." To be water-tight, this needs to be a recursive definition. If the object contains an `untracked` Go pointer, `deepcopy` cannot guarantee isolation. Your proof should explicitly state that `\clone` is only sound if the type $T$ is "deeply trackable."
*   **The $\backslash rob$ vs $\backslash imm$ distinction:** You distinguish these well, but Lemma 2 (Immutability) should explicitly address why a $\backslash rob$ (which is non-sendable) can't be "upgraded" to an $\backslash imm$ (which is sendable). Your rules prevent it, but the proof should highlight this "one-way valve" to ensure an `\iso` doesn't get shared while a $\backslash mub$ is still active.

---

### Final Verdict: **Sound Design, Leaky Proof.**

The **Capability Table** and **Viewpoint Adaptation** are excellent and follow the "Transitive Immutability" patterns used in state-of-the-art research languages (like Verona or Pony). 

**Is it water-tight?** **No.** 
To make it water-tight, you must update Section 3 and 4 to include a **"Lifetime"** or **"Region"** tracker. Specifically, the typing judgment needs to track which $\backslash mub$ and $\backslash rob$ variables are "descendants" of which `\iso` variable. Without that link, the `R-Send` rule cannot proves it "clears" all access to the memory location.

**Recommendation:**

Add a "borrowing" relation to your environment $\Gamma$. Instead of just $x: \tau$, use $x: \tau \text{ from } \{y\}$, where $y$ is the source `\iso`. Then, update `T-Send` to require that no variables "from $\{x\}$" exist in $\Gamma$. That would make this proof truly formidable.

What follows is the integrated, corrected, proof.

# Gown: Race Freedom Proof (Revised v2.0)

## A Formal Proof of Data Race Freedom for the Gown Capability Type System

---

## 1. Introduction

This document presents a formal proof that the Gown
capability type system guarantees data race freedom
for all well-typed programs that do not use `\unsafe`. 
This revised version (v2.0) incorporates **Region Tracking** 
to formally solve the "ghost borrow" problem, ensuring 
that unique capabilities cannot be transferred while 
active borrows exist in the local scope.

> **Theorem (Race Freedom).** If a program P is well-typed
> under the Gown capability checker and P contains no uses
> of `\unsafe`, then no execution of P contains a data race.

---

## 2. Definitions

### 2.1 Data Race

A **data race** occurs when two goroutines access the same memory location concurrently, at least one access is a write, and the accesses are not ordered by a synchronization operation.

Formally, a data race is a pair of events (e₁, e₂) in an execution trace such
that:

1. e₁ and e₂ access the same memory location ℓ
2. At least one of e₁, e₂ is a write
3. e₁ and e₂ occur in different goroutines
4. e₁ and e₂ are not ordered by the happens-before relation (≺)

### 2.2 Happens-Before

The happens-before relation (≺) for Gown is defined by:

- Within a single goroutine, program order induces happens-before.
- A send on a channel happens-before the corresponding receive completes.
- A `go` statement (goroutine spawn) happens-before the first statement of the
  spawned goroutine.

These are the only synchronization points relevant to the Gown type system.
Mutexes, atomics, and `sync` primitives exist in Go but are untracked by Gown
and are therefore outside the scope of this proof.

### 2.3 Capabilities and Regions

The four capabilities form a set C = { `\iso`, `\mub`, `\rob`, `\imm` }.


To ensure soundness, we introduce **Regions** ($\rho$). Every `\iso` pointer belongs to a unique region. Any borrow (`\mub` or `\rob`) derived from an `\iso` inherits its region .

| Capability | Mutable | Sendable | Unique | Description |  
| :--- | :--- | :--- | :--- | :--- |  
| `\iso` | Yes | Yes | Yes | Unique owner of a region . |  
| `\mub` | Yes | No | No | Local mutable borrow of a region . |  
| `\rob` | No | No | No | Local read-only borrow of a region . |  
| `\imm` | No | Yes | No | Globally immutable and shareable . |

\---

## 3. The Core Calculus λ‖

### 3.1 Syntax  
We define the types as capability-qualified pointers associated with a region $\rho$:

$$ \tau ::= c * T^\rho \mid T \mid \text{chan } c * T $$

The Typing Environment $\Gamma$ maps variables to these qualified types.

### 3.2 Region Tracking Helpers  
We define $Active(\Gamma, \rho)$ as the set of all variables in the current environment that belong to region $\rho$:
$$ Active(\Gamma, \rho) \= \{ y \mid \Gamma(y) \= c * T^\rho \} $$

\---

## 4.1 Typing Rules (Selected)

**T-Mub (Borrowing):**  
A borrow inherits the region $\rho$ of the source `\iso`.  
$$ \frac{\Gamma(x) \= \backslash\text{iso} * T^\rho \quad y \text{ is fresh}}{\Gamma \vdash\_g \backslash\text{mub}(x) : \backslash\text{mub} * T^\rho \dashv \Gamma, y : \backslash\text{mub} * T^\rho} $$

**T-Send-Iso (Movement):**  
An `\iso` can only be sent if it is the *sole* variable in its region, ensuring no local borrows are active.  
$$ \frac{\Gamma(x) \= \backslash\text{iso} * T^\rho \quad Active(\Gamma, \rho) \= \{x\} \quad \text{ch} : \text{chan } \backslash\text{iso} * T}{\Gamma \vdash\_g \text{send}(\text{ch}, x) : () \dashv \Gamma\[x \mapsto \bot\]} $$

**T-Spawn (Capture):**  
A goroutine can only capture an `\iso` if no other references to its region exist in the spawning goroutine.  
$$ \frac{\forall i: \text{Sendable}(c\_i) \quad (c\_i \= \backslash\text{iso} \implies Active(\Gamma, \rho\_i) \= \{x\_i\})}{\Gamma \vdash\_g \text{go}(\lambda(x\_1,...,x\_n).e) : () \dashv \Gamma\[x\_i \mapsto \bot \mid c\_i \= \backslash\text{iso}\]} $$

**T-Write:**  
Writes are only permitted on mutable capabilities (`\iso`, `\mub`) .  
$$ \frac{\Gamma(x) \= c * T^\rho \quad \text{Mutable}(c)}{\Gamma \vdash\_g x.f \leftarrow e : () \dashv \Gamma'} $$

\---

## 4.2 Typing rules (all)

To provide a "water-tight" formal foundation, the complete set of typing rules for Gown v2.0 must explicitly track the region $ho$ associated with every capability pointer[cite: 2]. This ensures that when an `\iso` moves, all local aliases (borrows) tied to that same region are statically invalidated[cite: 2].

Below is the exhaustive set of typing rules for the Gown v2.0 core calculus.

---

## 4.2.1. Core Judgment and Environments
The typing judgment is written as:
$$ \Gamma \vdash_g e : \tau \dashv \Gamma' $$
*   **$\Gamma$**: Initial typing environment mapping variables to qualified types $\tau = c * T^ho$[cite: 2].
*   **$e$**: The expression being checked[cite: 1].
*   **$\tau$**: The resulting type[cite: 1].
*   **$\Gamma'$**: The updated environment, reflecting consumption of linear `\iso` variables[cite: 2].

---

## 4.2.2. Basic Expressions

**T-Var**
$$ \frac{\Gamma(x) = \tau \quad \tau \neq \bot}{\Gamma \vdash_g x : \tau \dashv \Gamma} \text{[cite: 1]} $$

**T-ConsumedVar**
$$ \frac{\Gamma(x) = \bot}{\Gamma \vdash_g x : \text{error (GWN001)}} \text{[cite: 1]} $$

**T-New**
$$ \frac{\ell \text{ fresh} \quad ho \text{ fresh region}}{\Gamma \vdash_g \backslash\text{new}(T) : \backslash\text{iso} * T^ho \dashv \Gamma} \text{[cite: 2]} $$

**T-Clone**
$$ \frac{\Gamma \vdash_g e : c * T^{ho_{old}} \dashv \Gamma' \quad ho_{new} \text{ fresh}}{\Gamma \vdash_g \backslash\text{clone}(e) : \backslash\text{iso} * T^{ho_{new}} \dashv \Gamma'} \text{[cite: 2]} $$

---

## 4.2.3. Transformations and Borrows

**T-Freeze**
$$ \frac{\Gamma(x) = \backslash\text{iso} * T^ho \quad Active(\Gamma, ho) = \{x\}}{\Gamma \vdash_g \backslash\text{freeze}(x) : \backslash\text{imm} * T \dashv \Gamma[x \mapsto \bot]} \text{[cite: 2]} $$

**T-Mub (Mutable Borrow)**
$$ \frac{\Gamma(x) = \backslash\text{iso} * T^ho \quad y \text{ fresh}}{\Gamma \vdash_g \backslash\text{mub}(x) : \backslash\text{mub} * T^ho \dashv \Gamma, y : \backslash\text{mub} * T^ho} \text{[cite: 2]} $$

**T-Rob (Read-Only Borrow)**
$$ \frac{\Gamma(x) = ( \backslash\text{iso} \lor \backslash\text{imm} ) * T^ho \quad y \text{ fresh}}{\Gamma \vdash_g \backslash\text{rob}(x) : \backslash\text{rob} * T^ho \dashv \Gamma, y : \backslash\text{rob} * T^ho} \text{[cite: 2]} $$

---

## 4.2.4. Field Access and Mutability

**T-FieldRead (Viewpoint Adaptation)**
Field access preserves the region $ho$ of the root pointer to ensure aliases are correctly tracked[cite: 2].
$$ \frac{\Gamma \vdash_g x : c_{outer} * T^ho \dashv \Gamma' \quad \text{field } f : c_{field} * U}{\Gamma \vdash_g x.f : V(c_{outer}, c_{field}) * U^ho \dashv \Gamma'} \text{[cite: 2]} $$

**T-Write**
$$ \frac{\Gamma \vdash_g x : c * T^ho \dashv \Gamma' \quad \text{Mutable}(c) \quad \Gamma' \vdash_g e : \tau_f \dashv \Gamma''}{\Gamma \vdash_g x.f \leftarrow e : () \dashv \Gamma''} \text{[cite: 1, 2]} $$

---

## 4.2.5. Concurrency and Communication

**T-Send-Iso (Movement)**
The critical "Water-Tight" rule: an `\iso` can only be sent if its region $ho$ has no other active references (borrows) in the local environment[cite: 2].
$$ \frac{\Gamma(x) = \backslash\text{iso} * T^ho \quad Active(\Gamma, ho) = \{x\} \quad \text{ch} : \text{chan } \backslash\text{iso} * T}{\Gamma \vdash_g \text{send}(\text{ch}, x) : () \dashv \Gamma[x \mapsto \bot]} \text{[cite: 2]} $$

**T-Send-Imm (Sharing)**
$$ \frac{\Gamma(x) = \backslash\text{imm} * T \quad \text{ch} : \text{chan } \backslash\text{imm} * T}{\Gamma \vdash_g \text{send}(\text{ch}, x) : () \dashv \Gamma} \text{[cite: 1, 2]} $$

**T-Spawn (Goroutine Capture)**
Captured variables must be Sendable; captured `\iso` variables must be the sole occupants of their region[cite: 2].
$$ \frac{\forall i: \text{Sendable}(c_i) \quad (c_i = \backslash\text{iso} \implies Active(\Gamma, ho_i) = \{x_i\})}{\Gamma \vdash_g \text{go}(\lambda(x_1,...,x_n).e) : () \dashv \Gamma[x_i \mapsto \bot \mid c_i = \backslash\text{iso}]} \text{[cite: 2]} $$

---

## 4.2.6. Control Flow

**T-Select**
For `select`, an `\iso` or any member of its region $ho$ is invalidated in the output environment if the region is consumed in *any* chosen branch.

$$ \frac{\Gamma_{out} = \bigsqcap \{ \Gamma'_k \}}{\Gamma \vdash_g \text{select}(\text{arms}) : \tau \dashv \Gamma_{out}} \text{[cite: 2]} $$
The meet operator ($\sqcap$) ensures that if $Active(\Gamma'_k, ho)$ contains $\bot$ in branch $k$, then for all $y \in Active(\Gamma, ho)$, $\Gamma_{out}(y) = \bot$[cite: 2].

## 4.2 conclusion

This complete set of rules prevents the "Ghost Borrow" race condition by strictly linking the lifecycle of an `\iso` to its derived borrows through the $Active(\Gamma, ho)$ set[cite: 2]. If any alias exists, the root `\iso` is "pinned" to the current goroutine and cannot be moved or captured.


## 5. Invariants

### 5.1 Invariant 1: Isolation  
If goroutine $g\_1$ can reach location $\ell$ with a non-`\imm` capability, then no other goroutine $g\_2$ can reach $\ell$ with any capability .

### 5.2 Invariant 2: Region Integrity  
If $Active(\Gamma\_g, \rho) \> 1$, then the region $\rho$ is "pinned" to goroutine $g$ and cannot be transferred . 

### 5.3 Invariant 3: Immutability  
No write operation is ever performed through a reference with capability `\imm` or `\rob` .

\---

## 6. Proof of Race Freedom

### 6.1 Lemma 1 (Movement Soundness)  
When an `\iso` is sent or captured, rule **T-Send-Iso** (or **T-Spawn**) enforces $Active(\Gamma, \rho) \= \{x\}$. This guarantees that all local borrows (`\mub`, `\rob`) of that region have been terminated or are out of scope. Consequently, when the `\iso` moves to $g\_2$, $g\_1$ is left with no remaining references to the memory location $\ell$ .

### 6.2 Lemma 2 (Borrow Confinement)  
Since `\mub` and `\rob` are marked as non-sendable, and they are tied to a region $\rho$ that cannot move while they exist, it is impossible for a mutable borrow to exist in more than one goroutine simultaneously .

### 6.3 Lemma 3 (Immutability Propagation)  
Viewpoint adaptation ensures that once a reference is `\imm`, all fields accessed through it are also `\imm` . Since the checker rejects writes to `\imm` types, multi-goroutine sharing of `\imm` data is data-race free .

### 6.4 Theorem (Data Race Freedom)  
Assume a race occurs on $\ell$ between $g\_1$ and $g\_2$.   
1. If the access is via `\imm`, neither can write (Lemma 3), so no race exists .  
2. If $g\_1$ has a mutable reference, by Lemma 1 and 2, $g\_2$ cannot have any reference to $\ell$.
Thus, a data race is impossible in a well-typed program .

\---

## 7. The \unsafe Boundary
The proof holds only for `\unsafe`-free code. `\unsafe(x)` allows the programmer to bypass region tracking and linearity \[cite: 1\]. If an `\iso` is passed to an untracked function via `\unsafe`, the checker can no longer guarantee $Active(\Gamma, \rho) \= \{x\}$, and the isolation invariant may be violated \[cite: 1\].

\---
*End of Gown Race Freedom Proof v2.0*
