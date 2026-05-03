# Lean Proof Repair Plan: Gown Race Freedom

## Context

The Gown capability type system for Go has four capabilities (`\iso`, `\mub`, `\rob`, `\imm`) that guarantee data-race freedom for well-typed programs. The file `/Users/jaten/gown2/Gown.lean` contains a 347-line Lean 4 mechanized proof that currently fails with 16 compile errors. The hand proof in `theory-proof.md` is sound; the Lean formalization has both syntactic bugs and one fundamental logical gap.

**Goal:** Produce a compiling Lean 4 proof with **zero axioms** and **zero sorry** that proves:
1. `race_freedom`: Iso invariant implies no data race
2. `iso_pres`: every Step preserves Iso (14 cases)
3. `wf_pres`: every Step preserves the full well-formedness invariant
4. `multi_race_free`: race freedom over arbitrary execution traces

## Root Cause Analysis

### Syntactic errors (3 categories, 13 of 16 errors)

1. **Equality direction** (lines 157, 193, 237): After `obtain ⟨rfl,...⟩`, hypothesis `h₂.1` has type `g₂ = g₁` but `h12 : g₁ ≠ g₂` needs `g₁ = g₂`. Fix: `.symm`.

2. **Omega failures** (lines 158, 161): `omega` can't reduce `⟨o, n⟩.nextLoc` to `n`. Fix: use `Nat.lt_irrefl` directly or annotate with `show`.

3. **Unknown identifiers** (lines 196, 199, 227, 238, 243, 254): `obtain ⟨rfl, rfl⟩` eliminates the wrong variable in Lean 4.23 — the Step pattern variables (`g`, `ℓ`) get substituted away instead of the intro'd variables (`g₂`, `ℓ'`). Fix: use named hypotheses + explicit `subst`.

### Fundamental logical gap (pres_add_imm)

`pres_add_imm` proves adding `(gr, ℓ, imm)` to ownership preserves `Iso`, given `o g_src ℓ imm`. In the case where `g₁` holds mutable `c` at `ℓ` and `g₂ = gr` holds the new `imm`:

- If `g₁ ≠ g_src`: old `Iso` + `ho` gives contradiction. Works.
- If `g₁ = g_src`: same goroutine holds both mutable and `imm`. `Iso` only constrains *cross*-goroutine access. **Stuck.**

This scenario (mutable + imm at same location, same goroutine) is unreachable in the type system (freeze consumes iso before creating imm), but the formalization doesn't capture this constraint.

**Fix:** Add a `Coherent` invariant: "if a goroutine holds a mutable capability at a location, it doesn't also hold `imm` there." Verified preserved by all 14 step constructors.

## Implementation Plan

### Step 1: Add `Coherent` invariant and helper lemma

After `Fresh` definition (~line 61):

```lean
def Coherent (cfg : Config) : Prop :=
  ∀ g ℓ c, cfg.owns g ℓ c → Mutable c → ¬cfg.owns g ℓ imm
```

Update `WF`:
```lean
def WF (cfg : Config) : Prop := Iso cfg ∧ Fresh cfg ∧ Coherent cfg
```

Add derived lemma:
```lean
theorem no_mut_at_imm (cfg : Config) (hi : Iso cfg) (hc : Coherent cfg)
    (g₀ : GoroutineId) (ℓ : Loc) (ho : cfg.owns g₀ ℓ imm)
    (g : GoroutineId) (c : Cap) (hm : Mutable c) : ¬cfg.owns g ℓ c
```

### Step 2: Promote `iso_excl` from axiom to theorem

```lean
theorem iso_excl (cfg : Config) (h : Iso cfg) (g : GoroutineId) (ℓ : Loc) :
    cfg.owns g ℓ iso → ∀ g', g' ≠ g → ∀ c, ¬cfg.owns g' ℓ c
```

Proof: apply `Iso` with `Mutable iso` (by decide).

### Step 3: Fix `pres_fresh` proof

- Fix equality direction: `h12 h₂.1` → `h12 h₂.1.symm`
- Fix omega: `(by omega)` → `Nat.lt_irrefl _ (hf _ _ _ h₂)` or `(show n < n from hf _ _ _ h₂)`

### Step 4: Fix `pres_add_imm` proof

Add `hc : Coherent ⟨o, n⟩` parameter. In the problematic case (h₁ from old, h₂ from new imm), use `no_mut_at_imm` to derive `¬Mutable c`, contradicting `hm`.

### Step 5: Fix `pres_transfer` proof

- Replace `obtain ⟨rfl, rfl⟩` with named hypotheses + explicit `subst` (or `simp`/`rw`) to control which variables survive
- Fix equality direction in `fun e => h₂.1 e.symm` → `fun e => h₂.1 (Eq.symm e)` or restructure

### Step 6: Fix `iso_pres` proof

- Update `obtain ⟨hi, hf⟩ := hw` → `obtain ⟨hi, hf, hc⟩ := hw`
- Fix freeze_, mub_, rob_iso, rob_imm cases: replace `obtain ⟨rfl, rfl⟩` with named + `subst` to keep pattern variables in scope
- Pass `hc` to `pres_add_imm` calls (send_imm, spawn_imm)

### Step 7: Prove `fresh_pres` (new, ~50 lines)

```lean
theorem fresh_pres (cfg cfg' : Config) (h : Step cfg cfg') (hw : WF cfg) : Fresh cfg'
```

14 cases. Key patterns:
- `new_`/`clone_`: nextLoc increases; new loc `n < n+1`, old locs `< n < n+1`
- All others: nextLoc unchanged; all locations from old ownership, bounded by old `Fresh`

### Step 8: Prove `coherent_pres` (new, ~80 lines)

```lean
theorem coherent_pres (cfg cfg' : Config) (h : Step cfg cfg') (hw : WF cfg) : Coherent cfg'
```

14 cases. Key patterns:
- `write_`/`read_`: no ownership change, exact old Coherent
- `new_`/`clone_`: fresh location, no imm (by Fresh)
- `freeze_`: replaces (g,ℓ) with imm only, mutable gone
- `mub_`: adds mub; old Coherent ensures no imm alongside iso
- `rob_iso`/`rob_imm`: adds rob (neither mutable nor imm), no conflict
- `send_iso`/`spawn_iso`: sender removed, receiver gets iso; `iso_excl` ensures receiver had nothing
- `send_imm`/`spawn_imm`: adds imm; `no_mut_at_imm` ensures no mutable at ℓ

### Step 9: Replace `wf_pres` axiom with theorem

```lean
theorem wf_pres (cfg cfg' : Config) (h : Step cfg cfg') (hw : WF cfg) : WF cfg' :=
  ⟨iso_pres cfg cfg' h hw, fresh_pres cfg cfg' h hw, coherent_pres cfg cfg' h hw⟩
```

### Step 10: Update `iso_multi` / `multi_race_free`

These should work unchanged since `wf_pres` keeps the same signature.

## Files to modify

- `/Users/jaten/gown2/Gown.lean` — the only file that changes

## Verification

Run `make` (which executes `lean Gown.lean &> lean.run.log`). Success = empty log file, exit code 0.

After completion, `#print axioms multi_race_free` should show only Lean kernel axioms (propext, Quot.sound, etc.) — no custom axioms.

## Estimated size

Current: 347 lines with 2 axioms and 16 errors.
Target: ~500-550 lines with 0 axioms, 0 sorry, 0 errors.
