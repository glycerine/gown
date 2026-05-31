/-
  Gown: Mechanized Proof of Race Freedom
  ====================================
  Lean 4 formalization. No Mathlib. No sorry. No custom axioms.
  To check: `lean Gown.lean`
-/

inductive Cap where
  | iso : Cap | mub : Cap | rob : Cap | imm : Cap
  deriving DecidableEq, Repr, BEq
open Cap

abbrev Loc := Nat
abbrev GoroutineId := Nat

def Cap.mutable : Cap → Bool
  | iso => true | mub => true | rob => false | imm => false

def Mutable (c : Cap) : Prop := c.mutable = true
instance : Decidable (Mutable c) := inferInstanceAs (Decidable (c.mutable = true))
theorem not_mutable_rob : ¬Mutable rob := by decide
theorem not_mutable_imm : ¬Mutable imm := by decide

-- Viewpoint adaptation

def viewpoint (outer field_cap : Cap) : Cap :=
  match outer with
  | iso => field_cap
  | mub => match field_cap with
    | iso => mub | mub => mub | rob => rob | imm => imm
  | rob => rob
  | imm => imm

theorem viewpoint_imm_is_imm (c : Cap) : viewpoint imm c = imm := by cases c <;> rfl
theorem viewpoint_rob_is_rob (c : Cap) : viewpoint rob c = rob := by cases c <;> rfl
theorem viewpoint_preserves_immutability (outer c : Cap) (h : ¬Mutable outer) :
    ¬Mutable (viewpoint outer c) := by
  cases outer <;> simp [Mutable, Cap.mutable] at h <;>
    cases c <;> simp [viewpoint, Mutable, Cap.mutable]

-- ============================================================================
-- Abstract State
-- ============================================================================

def Owns := GoroutineId → Loc → Cap → Prop
structure Config where
  owns : Owns
  nextLoc : Loc
def canReach (cfg : Config) (g : GoroutineId) (ℓ : Loc) : Prop := ∃ c, cfg.owns g ℓ c

-- ============================================================================
-- Invariants
-- ============================================================================

def Iso (cfg : Config) : Prop :=
  ∀ g₁ g₂ ℓ c, cfg.owns g₁ ℓ c → Mutable c → g₁ ≠ g₂ → ¬canReach cfg g₂ ℓ

def Fresh (cfg : Config) : Prop :=
  ∀ g ℓ c, cfg.owns g ℓ c → ℓ < cfg.nextLoc

def Coherent (cfg : Config) : Prop :=
  ∀ g ℓ c, cfg.owns g ℓ c → Mutable c → ¬cfg.owns g ℓ imm

def WF (cfg : Config) : Prop := Iso cfg ∧ Fresh cfg ∧ Coherent cfg

-- ============================================================================
-- Key Lemmas from Invariants
-- ============================================================================

theorem iso_excl (cfg : Config) (h : Iso cfg) (g : GoroutineId) (ℓ : Loc) :
    cfg.owns g ℓ iso → ∀ g', g' ≠ g → ∀ c, ¬cfg.owns g' ℓ c := by
  intro hown g' hne c hc
  exact h g g' ℓ iso hown (by decide) (fun e => hne e.symm) ⟨c, hc⟩

theorem no_mut_at_imm (cfg : Config) (hi : Iso cfg) (hco : Coherent cfg)
    (g₀ : GoroutineId) (ℓ : Loc) (ho : cfg.owns g₀ ℓ imm)
    (g : GoroutineId) (c : Cap) (hm : Mutable c) : ¬cfg.owns g ℓ c := by
  intro hgc
  by_cases heq : g = g₀
  · subst heq; exact hco g ℓ c hgc hm ho
  · exact hi g g₀ ℓ c hgc hm heq ⟨imm, ho⟩

-- ============================================================================
-- Steps (Relational)
-- ============================================================================

inductive Step : Config → Config → Prop where
  | new_ (o : Owns) (n : Loc) (g : GoroutineId) :
    Step ⟨o, n⟩ ⟨fun g' ℓ c => (g' = g ∧ ℓ = n ∧ c = iso) ∨ o g' ℓ c, n + 1⟩

  | clone_ (o : Owns) (n : Loc) (g : GoroutineId) (ℓs : Loc) (cs : Cap) (_ : o g ℓs cs) :
    Step ⟨o, n⟩ ⟨fun g' ℓ c => (g' = g ∧ ℓ = n ∧ c = iso) ∨ o g' ℓ c, n + 1⟩

  | freeze_ (o : Owns) (n : Loc) (g : GoroutineId) (ℓ : Loc) (_ : o g ℓ iso) :
    Step ⟨o, n⟩ ⟨fun g' ℓ' c => if g' = g ∧ ℓ' = ℓ then c = imm else o g' ℓ' c, n⟩

  | mub_ (o : Owns) (n : Loc) (g : GoroutineId) (ℓ : Loc) (_ : o g ℓ iso) :
    Step ⟨o, n⟩ ⟨fun g' ℓ' c => (g' = g ∧ ℓ' = ℓ ∧ c = mub) ∨ o g' ℓ' c, n⟩

  | rob_iso (o : Owns) (n : Loc) (g : GoroutineId) (ℓ : Loc) (_ : o g ℓ iso) :
    Step ⟨o, n⟩ ⟨fun g' ℓ' c => (g' = g ∧ ℓ' = ℓ ∧ c = rob) ∨ o g' ℓ' c, n⟩

  | rob_imm (o : Owns) (n : Loc) (g : GoroutineId) (ℓ : Loc) (_ : o g ℓ imm) :
    Step ⟨o, n⟩ ⟨fun g' ℓ' c => (g' = g ∧ ℓ' = ℓ ∧ c = rob) ∨ o g' ℓ' c, n⟩

  | write_ (o : Owns) (n : Loc) (g : GoroutineId) (ℓ : Loc) (c : Cap)
    (_ : o g ℓ c) (_ : Mutable c) :
    Step ⟨o, n⟩ ⟨o, n⟩

  | read_ (o : Owns) (n : Loc) (g : GoroutineId) (ℓ : Loc) (c : Cap) (_ : o g ℓ c) :
    Step ⟨o, n⟩ ⟨o, n⟩

  | send_iso (o : Owns) (n : Loc) (gs gr : GoroutineId) (ℓ : Loc)
    (_ : o gs ℓ iso) (_ : gs ≠ gr) :
    Step ⟨o, n⟩ ⟨fun g' ℓ' c =>
      if ℓ' = ℓ then (g' = gr ∧ c = iso) ∨ (g' ≠ gs ∧ o g' ℓ' c)
      else o g' ℓ' c, n⟩

  | send_iso_imm (o : Owns) (n : Loc) (gs gr : GoroutineId) (ℓ : Loc)
    (_ : o gs ℓ iso) (_ : gs ≠ gr) :
    Step ⟨o, n⟩ ⟨fun g' ℓ' c =>
      if ℓ' = ℓ then (g' = gr ∧ c = imm) ∨ (g' ≠ gs ∧ o g' ℓ' c)
      else o g' ℓ' c, n⟩

  | send_imm (o : Owns) (n : Loc) (gs gr : GoroutineId) (ℓ : Loc) (_ : o gs ℓ imm) :
    Step ⟨o, n⟩ ⟨fun g' ℓ' c => (g' = gr ∧ ℓ' = ℓ ∧ c = imm) ∨ o g' ℓ' c, n⟩

  | spawn_iso (o : Owns) (n : Loc) (gp gc : GoroutineId) (ℓ : Loc)
    (_ : o gp ℓ iso) (_ : gp ≠ gc) :
    Step ⟨o, n⟩ ⟨fun g' ℓ' c =>
      if ℓ' = ℓ then (g' = gc ∧ c = iso) ∨ (g' ≠ gp ∧ o g' ℓ' c)
      else o g' ℓ' c, n⟩

  | spawn_imm (o : Owns) (n : Loc) (gp gc : GoroutineId) (ℓ : Loc) (_ : o gp ℓ imm) :
    Step ⟨o, n⟩ ⟨fun g' ℓ' c => (g' = gc ∧ ℓ' = ℓ ∧ c = imm) ∨ o g' ℓ' c, n⟩

-- ============================================================================
-- Race Freedom Theorem
-- ============================================================================

structure Access where
  goroutine : GoroutineId
  loc : Loc
  isWrite : Bool
  cap : Cap

def isRace (a₁ a₂ : Access) : Prop :=
  a₁.loc = a₂.loc ∧ a₁.goroutine ≠ a₂.goroutine ∧ (a₁.isWrite ∨ a₂.isWrite)

def Access.wt (a : Access) : Prop := a.isWrite → Mutable a.cap
def Access.ok (a : Access) (cfg : Config) : Prop := cfg.owns a.goroutine a.loc a.cap

theorem race_freedom (cfg : Config) (a₁ a₂ : Access)
    (hi : Iso cfg) (w₁ : a₁.wt) (w₂ : a₂.wt) (p₁ : a₁.ok cfg) (p₂ : a₂.ok cfg) :
    ¬isRace a₁ a₂ := by
  intro ⟨hloc, hne, hw⟩
  cases hw with
  | inl h => exact hi _ _ _ _ p₁ (w₁ h) hne (hloc ▸ ⟨_, p₂⟩)
  | inr h => exact hi _ _ _ _ p₂ (w₂ h) (fun e => hne e.symm) (hloc.symm ▸ ⟨_, p₁⟩)

-- ============================================================================
-- Isolation Preservation Helpers
-- ============================================================================

private theorem pres_fresh (o : Owns) (n : Loc) (g : GoroutineId)
    (hi : Iso ⟨o, n⟩) (hf : Fresh ⟨o, n⟩) :
    Iso ⟨fun g' ℓ c => (g' = g ∧ ℓ = n ∧ c = iso) ∨ o g' ℓ c, n + 1⟩ := by
  intro g₁ g₂ ℓ c h₁ hm h12 ⟨c₂, h₂⟩
  cases h₁ with
  | inl h₁ =>
    cases h₂ with
    | inl h₂ => exact h12 (h₁.1.trans h₂.1.symm)
    | inr h₂ => rw [h₁.2.1] at h₂; exact Nat.lt_irrefl _ (hf _ _ _ h₂)
  | inr h₁ =>
    cases h₂ with
    | inl h₂ => rw [h₂.2.1] at h₁; exact Nat.lt_irrefl _ (hf _ _ _ h₁)
    | inr h₂ => exact hi _ _ _ _ h₁ hm h12 ⟨_, h₂⟩

private theorem pres_add_imm (o : Owns) (n : Loc) (g_src gr : GoroutineId)
    (ℓ : Loc) (hi : Iso ⟨o, n⟩) (hco : Coherent ⟨o, n⟩) (ho : o g_src ℓ imm) :
    Iso ⟨fun g' ℓ' c => (g' = gr ∧ ℓ' = ℓ ∧ c = imm) ∨ o g' ℓ' c, n⟩ := by
  intro g₁ g₂ ℓ' c h₁ hm h12 ⟨c₂, h₂⟩
  cases h₁ with
  | inl h₁ => rw [h₁.2.2] at hm; exact absurd hm not_mutable_imm
  | inr h₁ =>
    cases h₂ with
    | inl h₂ =>
      rw [h₂.2.1] at h₁
      exact absurd h₁ (no_mut_at_imm ⟨o, n⟩ hi hco g_src ℓ ho g₁ c hm)
    | inr h₂ => exact hi _ _ _ _ h₁ hm h12 ⟨_, h₂⟩

private theorem pres_transfer (o : Owns) (n : Loc) (gs gr : GoroutineId) (ℓ : Loc)
    (rc : Cap) (hi : Iso ⟨o, n⟩) (ho : o gs ℓ iso) (_ : gs ≠ gr)
    (_ : Mutable rc → rc = iso) :
    Iso ⟨fun g' ℓ' c =>
      if ℓ' = ℓ then (g' = gr ∧ c = rc) ∨ (g' ≠ gs ∧ o g' ℓ' c)
      else o g' ℓ' c, n⟩ := by
  intro g₁ g₂ ℓ' c h₁ hm h12 ⟨c₂, h₂⟩
  by_cases hℓ : ℓ' = ℓ
  · subst hℓ; simp only [ite_true] at h₁ h₂
    cases h₁ with
    | inl h₁ =>
      cases h₂ with
      | inl h₂ => exact h12 (h₁.1.trans h₂.1.symm)
      | inr h₂ => exact iso_excl ⟨o,n⟩ hi gs ℓ' ho g₂ h₂.1 c₂ h₂.2
    | inr h₁ => exact absurd h₁.2 (iso_excl ⟨o,n⟩ hi gs ℓ' ho g₁ h₁.1 c)
  · simp only [show ¬(ℓ' = ℓ) from hℓ, ite_false] at h₁ h₂
    exact hi _ _ _ _ h₁ hm h12 ⟨_, h₂⟩

-- ============================================================================
-- Isolation Preservation
-- ============================================================================

theorem iso_pres (cfg cfg' : Config) (h : Step cfg cfg') (hw : WF cfg) : Iso cfg' := by
  obtain ⟨hi, hf, hc⟩ := hw
  cases h with

  | new_ o n g => exact pres_fresh o n g hi hf
  | clone_ o n g _ _ _ => exact pres_fresh o n g hi hf
  | write_ o n _ _ _ _ _ => exact hi
  | read_ o n _ _ _ _ => exact hi

  | freeze_ o n g ℓ ho =>
    intro g₁ g₂ ℓ' c h₁ hm h12 ⟨c₂, h₂⟩
    by_cases hg₁ : g₁ = g ∧ ℓ' = ℓ
    · simp only [hg₁.1, hg₁.2, and_self, ite_true] at h₁
      rw [h₁] at hm; exact absurd hm not_mutable_imm
    · simp only [show ¬(g₁ = g ∧ ℓ' = ℓ) from hg₁, ite_false] at h₁
      by_cases hg₂ : g₂ = g ∧ ℓ' = ℓ
      · rw [hg₂.2] at h₁
        exact iso_excl ⟨o,n⟩ hi g ℓ ho g₁ (fun e => hg₁ ⟨e, hg₂.2⟩) c h₁
      · simp only [show ¬(g₂ = g ∧ ℓ' = ℓ) from hg₂, ite_false] at h₂
        exact hi _ _ _ _ h₁ hm h12 ⟨_, h₂⟩

  | mub_ o n g ℓ ho =>
    intro g₁ g₂ ℓ' c h₁ hm h12 ⟨c₂, h₂⟩
    cases h₁ with
    | inl h₁ =>
      cases h₂ with
      | inl h₂ => exact h12 (h₁.1.trans h₂.1.symm)
      | inr h₂ =>
        rw [h₁.2.1] at h₂
        exact iso_excl ⟨o,n⟩ hi g ℓ ho g₂ (fun e => h12 (h₁.1.trans e.symm)) c₂ h₂
    | inr h₁ =>
      cases h₂ with
      | inl h₂ =>
        rw [h₂.2.1] at h₁
        exact iso_excl ⟨o,n⟩ hi g ℓ ho g₁ (fun e => h12 (e.trans h₂.1.symm)) c h₁
      | inr h₂ => exact hi _ _ _ _ h₁ hm h12 ⟨_, h₂⟩

  | rob_iso o n g ℓ ho =>
    intro g₁ g₂ ℓ' c h₁ hm h12 ⟨c₂, h₂⟩
    cases h₁ with
    | inl h₁ => rw [h₁.2.2] at hm; exact absurd hm not_mutable_rob
    | inr h₁ =>
      cases h₂ with
      | inl h₂ =>
        rw [h₂.2.1] at h₁
        exact iso_excl ⟨o,n⟩ hi g ℓ ho g₁ (fun e => h12 (e.trans h₂.1.symm)) c h₁
      | inr h₂ => exact hi _ _ _ _ h₁ hm h12 ⟨_, h₂⟩

  | rob_imm o n g ℓ ho =>
    intro g₁ g₂ ℓ' c h₁ hm h12 ⟨c₂, h₂⟩
    cases h₁ with
    | inl h₁ => rw [h₁.2.2] at hm; exact absurd hm not_mutable_rob
    | inr h₁ =>
      cases h₂ with
      | inl h₂ =>
        rw [h₂.2.1] at h₁
        exact hi _ _ _ _ h₁ hm (fun e => h12 (e.trans h₂.1.symm)) ⟨imm, ho⟩
      | inr h₂ => exact hi _ _ _ _ h₁ hm h12 ⟨_, h₂⟩

  | send_iso o n gs gr ℓ ho hsr =>
    exact pres_transfer o n gs gr ℓ iso hi ho hsr (fun _ => rfl)

  | send_iso_imm o n gs gr ℓ ho hsr =>
    exact pres_transfer o n gs gr ℓ imm hi ho hsr (fun h => absurd h not_mutable_imm)

  | send_imm o n gs gr ℓ ho => exact pres_add_imm o n gs gr ℓ hi hc ho
  | spawn_imm o n gp gc ℓ ho => exact pres_add_imm o n gp gc ℓ hi hc ho

  | spawn_iso o n gp gc ℓ ho hpc =>
    exact pres_transfer o n gp gc ℓ iso hi ho hpc (fun _ => rfl)

-- ============================================================================
-- Fresh Preservation
-- ============================================================================

theorem fresh_pres (cfg cfg' : Config) (h : Step cfg cfg') (hw : WF cfg) : Fresh cfg' := by
  obtain ⟨_, hf, _⟩ := hw
  cases h with
  | new_ o n g =>
    intro g' ℓ c hown
    cases hown with
    | inl h => rw [h.2.1]; exact Nat.le.refl
    | inr h => exact Nat.le.step (hf _ _ _ h)
  | clone_ o n g _ _ _ =>
    intro g' ℓ c hown
    cases hown with
    | inl h => rw [h.2.1]; exact Nat.le.refl
    | inr h => exact Nat.le.step (hf _ _ _ h)
  | freeze_ o n g ℓ ho =>
    intro g' ℓ' c hown
    by_cases hgl : g' = g ∧ ℓ' = ℓ
    · rw [hgl.2]; exact hf _ _ _ ho
    · simp only [show ¬(g' = g ∧ ℓ' = ℓ) from hgl, ite_false] at hown
      exact hf _ _ _ hown
  | mub_ o n g ℓ ho =>
    intro g' ℓ' c hown
    cases hown with
    | inl h => rw [h.2.1]; exact hf _ _ _ ho
    | inr h => exact hf _ _ _ h
  | rob_iso o n g ℓ ho =>
    intro g' ℓ' c hown
    cases hown with
    | inl h => rw [h.2.1]; exact hf _ _ _ ho
    | inr h => exact hf _ _ _ h
  | rob_imm o n g ℓ ho =>
    intro g' ℓ' c hown
    cases hown with
    | inl h => rw [h.2.1]; exact hf _ _ _ ho
    | inr h => exact hf _ _ _ h
  | write_ o n _ _ _ _ _ => exact hf
  | read_ o n _ _ _ _ => exact hf
  | send_iso o n gs gr ℓ ho _ =>
    intro g' ℓ' c hown
    by_cases hℓ : ℓ' = ℓ
    · subst hℓ; simp only [ite_true] at hown
      cases hown with
      | inl _ => exact hf _ _ _ ho
      | inr h => exact hf _ _ _ h.2
    · simp only [show ¬(ℓ' = ℓ) from hℓ, ite_false] at hown
      exact hf _ _ _ hown
  | send_iso_imm o n gs gr ℓ ho _ =>
    intro g' ℓ' c hown
    by_cases hℓ : ℓ' = ℓ
    · subst hℓ; simp only [ite_true] at hown
      cases hown with
      | inl _ => exact hf _ _ _ ho
      | inr h => exact hf _ _ _ h.2
    · simp only [show ¬(ℓ' = ℓ) from hℓ, ite_false] at hown
      exact hf _ _ _ hown
  | send_imm o n _ _ ℓ ho =>
    intro g' ℓ' c hown
    cases hown with
    | inl h => rw [h.2.1]; exact hf _ _ _ ho
    | inr h => exact hf _ _ _ h
  | spawn_iso o n gp gc ℓ ho _ =>
    intro g' ℓ' c hown
    by_cases hℓ : ℓ' = ℓ
    · subst hℓ; simp only [ite_true] at hown
      cases hown with
      | inl _ => exact hf _ _ _ ho
      | inr h => exact hf _ _ _ h.2
    · simp only [show ¬(ℓ' = ℓ) from hℓ, ite_false] at hown
      exact hf _ _ _ hown
  | spawn_imm o n _ _ ℓ ho =>
    intro g' ℓ' c hown
    cases hown with
    | inl h => rw [h.2.1]; exact hf _ _ _ ho
    | inr h => exact hf _ _ _ h

-- ============================================================================
-- Coherent Preservation
-- ============================================================================

theorem coherent_pres (cfg cfg' : Config) (h : Step cfg cfg') (hw : WF cfg) :
    Coherent cfg' := by
  obtain ⟨hi, hf, hc⟩ := hw
  cases h with
  | write_ o n _ _ _ _ _ => exact hc
  | read_ o n _ _ _ _ => exact hc

  | new_ o n g =>
    intro g' ℓ c hown hm himm
    have himm' : o g' ℓ imm := by
      cases himm with
      | inl h => exact absurd h.2.2 (by decide)
      | inr h => exact h
    cases hown with
    | inl h =>
      rw [h.2.1] at himm'
      exact Nat.lt_irrefl _ (hf _ _ _ himm')
    | inr h => exact hc _ _ _ h hm himm'

  | clone_ o n g _ _ _ =>
    intro g' ℓ c hown hm himm
    have himm' : o g' ℓ imm := by
      cases himm with
      | inl h => exact absurd h.2.2 (by decide)
      | inr h => exact h
    cases hown with
    | inl h =>
      rw [h.2.1] at himm'
      exact Nat.lt_irrefl _ (hf _ _ _ himm')
    | inr h => exact hc _ _ _ h hm himm'

  | freeze_ o n g ℓ _ =>
    intro g' ℓ' c hown hm himm
    by_cases hgl : g' = g ∧ ℓ' = ℓ
    · simp only [hgl.1, hgl.2, and_self, ite_true] at hown
      rw [hown] at hm; exact absurd hm not_mutable_imm
    · simp only [show ¬(g' = g ∧ ℓ' = ℓ) from hgl, ite_false] at hown himm
      exact hc _ _ _ hown hm himm

  | mub_ o n g ℓ ho =>
    intro g' ℓ' c hown hm himm
    have himm' : o g' ℓ' imm := by
      cases himm with
      | inl h => exact absurd h.2.2 (by decide)
      | inr h => exact h
    cases hown with
    | inl h =>
      rw [h.1, h.2.1] at himm'
      exact hc _ _ _ ho (by decide) himm'
    | inr h => exact hc _ _ _ h hm himm'

  | rob_iso o n g ℓ _ =>
    intro g' ℓ' c hown hm himm
    have himm' : o g' ℓ' imm := by
      cases himm with
      | inl h => exact absurd h.2.2 (by decide)
      | inr h => exact h
    cases hown with
    | inl h => rw [h.2.2] at hm; exact absurd hm not_mutable_rob
    | inr h => exact hc _ _ _ h hm himm'

  | rob_imm o n g ℓ _ =>
    intro g' ℓ' c hown hm himm
    have himm' : o g' ℓ' imm := by
      cases himm with
      | inl h => exact absurd h.2.2 (by decide)
      | inr h => exact h
    cases hown with
    | inl h => rw [h.2.2] at hm; exact absurd hm not_mutable_rob
    | inr h => exact hc _ _ _ h hm himm'

  | send_iso o n gs gr ℓ ho _ =>
    intro g' ℓ' c hown hm himm
    by_cases hℓ : ℓ' = ℓ
    · subst hℓ; simp only [ite_true] at himm
      cases himm with
      | inl h => exact absurd h.2 (by decide)
      | inr h => exact iso_excl ⟨o,n⟩ hi gs ℓ' ho g' h.1 imm h.2
    · simp only [show ¬(ℓ' = ℓ) from hℓ, ite_false] at hown himm
      exact hc _ _ _ hown hm himm

  | send_iso_imm o n gs gr ℓ ho _ =>
    intro g' ℓ' c hown hm himm
    by_cases hℓ : ℓ' = ℓ
    · subst hℓ; simp only [ite_true] at hown
      cases hown with
      | inl h => rw [h.2] at hm; exact absurd hm not_mutable_imm
      | inr h => exact iso_excl ⟨o,n⟩ hi gs ℓ' ho g' h.1 c h.2
    · simp only [show ¬(ℓ' = ℓ) from hℓ, ite_false] at hown himm
      exact hc _ _ _ hown hm himm

  | send_imm o n gs gr ℓ ho =>
    intro g' ℓ' c hown hm himm
    cases hown with
    | inl h => rw [h.2.2] at hm; exact absurd hm not_mutable_imm
    | inr h =>
      cases himm with
      | inl h' =>
        rw [h'.2.1] at h
        exact absurd h (no_mut_at_imm ⟨o,n⟩ hi hc gs ℓ ho g' c hm)
      | inr h' => exact hc _ _ _ h hm h'

  | spawn_iso o n gp gc ℓ ho _ =>
    intro g' ℓ' c hown hm himm
    by_cases hℓ : ℓ' = ℓ
    · subst hℓ; simp only [ite_true] at himm
      cases himm with
      | inl h => exact absurd h.2 (by decide)
      | inr h => exact iso_excl ⟨o,n⟩ hi gp ℓ' ho g' h.1 imm h.2
    · simp only [show ¬(ℓ' = ℓ) from hℓ, ite_false] at hown himm
      exact hc _ _ _ hown hm himm

  | spawn_imm o n gp gc ℓ ho =>
    intro g' ℓ' c hown hm himm
    cases hown with
    | inl h => rw [h.2.2] at hm; exact absurd hm not_mutable_imm
    | inr h =>
      cases himm with
      | inl h' =>
        rw [h'.2.1] at h
        exact absurd h (no_mut_at_imm ⟨o,n⟩ hi hc gp ℓ ho g' c hm)
      | inr h' => exact hc _ _ _ h hm h'

-- ============================================================================
-- WF Preservation (no axioms)
-- ============================================================================

theorem wf_pres (cfg cfg' : Config) (h : Step cfg cfg') (hw : WF cfg) : WF cfg' :=
  ⟨iso_pres cfg cfg' h hw, fresh_pres cfg cfg' h hw, coherent_pres cfg cfg' h hw⟩

-- ============================================================================
-- Multi-Step Race Freedom
-- ============================================================================

inductive Multi : Config → Config → Prop where
  | refl : Multi cfg cfg
  | step : Step cfg cfg' → Multi cfg' cfg'' → Multi cfg cfg''

theorem iso_multi (cfg cfg' : Config) (h : Multi cfg cfg') (hw : WF cfg) : Iso cfg' := by
  induction h with
  | refl => exact hw.1
  | step hs _ ih => exact ih (wf_pres _ _ hs hw)

theorem multi_race_free (c₀ cf : Config) (a₁ a₂ : Access)
    (hw : WF c₀) (hr : Multi c₀ cf) (w₁ : a₁.wt) (w₂ : a₂.wt)
    (p₁ : a₁.ok cf) (p₂ : a₂.ok cf) : ¬isRace a₁ a₂ :=
  race_freedom cf a₁ a₂ (iso_multi _ _ hr hw) w₁ w₂ p₁ p₂

-- ============================================================================
-- Select Soundness
-- ============================================================================

inductive SelOut where | send (i : Nat) | dflt

def capSel (pre : Cap) (sent : Nat → Bool) : SelOut → Option Cap
  | .send i => if sent i then (if pre = imm then some imm else none) else some pre
  | .dflt => some pre

theorem iso_consumed (s : Nat → Bool) (i : Nat) (h : s i = true) :
    capSel iso s (.send i) = none := by simp [capSel, h]

theorem imm_kept (s : Nat → Bool) (o : SelOut) : capSel imm s o = some imm := by
  cases o with
  | send i => simp [capSel]
  | dflt => rfl

-- ============================================================================
-- Viewpoint extras
-- ============================================================================

theorem deep_imm (c₁ c₂ : Cap) :
    viewpoint imm (viewpoint (viewpoint imm c₁) c₂) = imm := by simp [viewpoint]
theorem deep_rob (c₁ c₂ : Cap) :
    viewpoint rob (viewpoint (viewpoint rob c₁) c₂) = rob := by simp [viewpoint]
theorem stable_imm_field_from_iso :
    viewpoint iso imm = imm := by rfl
theorem stable_imm_field_from_mub :
    viewpoint mub imm = imm := by rfl

/-
  SUMMARY
  ═══════
  Proven without sorry or custom axioms:
  • race_freedom: Iso cfg → no data race
  • iso_pres: every Step preserves Iso (14 cases via 3 helper patterns)
  • fresh_pres: every Step preserves Fresh
  • coherent_pres: every Step preserves Coherent
  • wf_pres: every Step preserves WF (combines iso/fresh/coherent)
  • iso_multi / multi_race_free: induction over traces
  • capSel soundness: iso consumed, imm retained
  • viewpoint soundness
  • stable \imm fields keep immutable viewpoint through \iso/\mub owners

  Key invariants:
  • Iso: mutable ownerstamp ⟹ exclusive goroutine access
  • Fresh: owned locations below allocation counter
  • Coherent: no goroutine holds both mutable and \imm on same location

  DESIGN NOTE: Isolation constrains MUTABLE ownerstamps only.
  \rob (read-only) is excluded — it cannot write and can be safely
  created from shared \imm. Discovered during mechanization.
-/
