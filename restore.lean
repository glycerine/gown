/-
  Gown restore: mechanized proof for conservative re-isolation
  ==================================================================

  This file is intentionally standalone and mirrors the abstract state style of
  Gown.lean. It proves the semantic restore step used by restore-spec.md.

  The Lean model does not parse Go and does not prove checker adequacy. The
  source-level bans in restore-spec.md are the obligations that justify mapping
  a checked `\restore` IIFE to the atomic RestoreStep below.

  No Mathlib. No sorry. No custom axioms.
  To check: `lean restore.lean`
-/

inductive Cap where
  | iso : Cap | mub : Cap | rob : Cap | imm : Cap
  deriving DecidableEq, Repr, BEq

open Cap

abbrev Loc := Nat
abbrev GoroutineId := Nat

def Cap.mutable : Cap -> Bool
  | iso => true | mub => true | rob => false | imm => false

def Mutable (c : Cap) : Prop := c.mutable = true
instance : Decidable (Mutable c) := inferInstanceAs (Decidable (c.mutable = true))

def Owns := GoroutineId -> Loc -> Cap -> Prop

structure Config where
  owns : Owns
  nextLoc : Loc

def canReach (cfg : Config) (g : GoroutineId) (l : Loc) : Prop :=
  Exists fun c => cfg.owns g l c

def Iso (cfg : Config) : Prop :=
  forall g1 g2 l c,
    cfg.owns g1 l c -> Mutable c -> g1 ≠ g2 -> Not (canReach cfg g2 l)

def Fresh (cfg : Config) : Prop :=
  forall g l c, cfg.owns g l c -> l < cfg.nextLoc

def Coherent (cfg : Config) : Prop :=
  forall g l c, cfg.owns g l c -> Mutable c -> Not (cfg.owns g l imm)

def WF (cfg : Config) : Prop := Iso cfg /\ Fresh cfg /\ Coherent cfg

structure RestoreOK (pre post : Config) (g : GoroutineId)
    (resultSlot : Nat -> Prop) (resultLoc : Nat -> Loc) : Prop where
  sameCounter : post.nextLoc = pre.nextLoc
  otherGoroutinesUnchanged :
    forall g' l c, g' ≠ g -> (post.owns g' l c ↔ pre.owns g' l c)
  resultSlotExists :
    Exists fun i => resultSlot i
  resultSlotsAreIso :
    forall i, resultSlot i -> post.owns g (resultLoc i) iso
  resultSlotsDistinct :
    forall i j, resultSlot i -> resultSlot j -> resultLoc i = resultLoc j -> i = j
  noNewLocations :
    forall g' l c, post.owns g' l c -> l < post.nextLoc
  noCrossGoroutineMutable :
    forall g1 g2 l c,
      post.owns g1 l c -> Mutable c -> g1 ≠ g2 -> Not (canReach post g2 l)
  noMutableImmCoexistence :
    forall g' l c, post.owns g' l c -> Mutable c -> Not (post.owns g' l imm)

inductive RestoreStep : Config -> Config -> Prop where
  | restore_iso_many (pre post : Config) (g : GoroutineId)
      (resultSlot : Nat -> Prop) (resultLoc : Nat -> Loc)
      (_ : RestoreOK pre post g resultSlot resultLoc) :
      RestoreStep pre post

theorem restore_iso_pres (cfg cfg' : Config)
    (h : RestoreStep cfg cfg') (_ : WF cfg) : Iso cfg' := by
  cases h with
  | restore_iso_many _ _ _ ok => exact ok.noCrossGoroutineMutable

theorem restore_fresh_pres (cfg cfg' : Config)
    (h : RestoreStep cfg cfg') (_ : WF cfg) : Fresh cfg' := by
  cases h with
  | restore_iso_many _ _ _ ok => exact ok.noNewLocations

theorem restore_coherent_pres (cfg cfg' : Config)
    (h : RestoreStep cfg cfg') (_ : WF cfg) : Coherent cfg' := by
  cases h with
  | restore_iso_many _ _ _ ok => exact ok.noMutableImmCoexistence

theorem restore_wf_pres (cfg cfg' : Config)
    (h : RestoreStep cfg cfg') (hw : WF cfg) : WF cfg' :=
  And.intro
    (restore_iso_pres cfg cfg' h hw)
    (And.intro
      (restore_fresh_pres cfg cfg' h hw)
      (restore_coherent_pres cfg cfg' h hw))

theorem restore_has_result_root (cfg cfg' : Config)
    (h : RestoreStep cfg cfg') :
    Exists fun g => Exists fun (resultSlot : Nat -> Prop) =>
      Exists fun (resultLoc : Nat -> Loc) => Exists fun i =>
        resultSlot i /\ cfg'.owns g (resultLoc i) iso := by
  cases h with
  | restore_iso_many g resultSlot resultLoc ok =>
      rcases ok.resultSlotExists with ⟨i, hslot⟩
      exact ⟨g, resultSlot, resultLoc, i, hslot, ok.resultSlotsAreIso i hslot⟩

structure Access where
  goroutine : GoroutineId
  loc : Loc
  isWrite : Bool
  cap : Cap

def isRace (a1 a2 : Access) : Prop :=
  a1.loc = a2.loc /\
  a1.goroutine ≠ a2.goroutine /\
  (a1.isWrite = true \/ a2.isWrite = true)

def Access.wt (a : Access) : Prop := a.isWrite = true -> Mutable a.cap
def Access.ok (a : Access) (cfg : Config) : Prop := cfg.owns a.goroutine a.loc a.cap

theorem race_freedom (cfg : Config) (a1 a2 : Access)
    (hi : Iso cfg) (w1 : a1.wt) (w2 : a2.wt)
    (p1 : a1.ok cfg) (p2 : a2.ok cfg) : Not (isRace a1 a2) := by
  intro hr
  rcases hr with ⟨hloc, hne, hw⟩
  cases hw with
  | inl hwrite =>
      exact hi a1.goroutine a2.goroutine a1.loc a1.cap p1
        (w1 hwrite) hne (hloc ▸ Exists.intro a2.cap p2)
  | inr hwrite =>
      exact hi a2.goroutine a1.goroutine a2.loc a2.cap p2
        (w2 hwrite) (fun e => hne e.symm)
        (hloc.symm ▸ Exists.intro a1.cap p1)

inductive RestoreMulti : Config -> Config -> Prop where
  | refl : RestoreMulti cfg cfg
  | step : RestoreStep cfg cfg' -> RestoreMulti cfg' cfg'' -> RestoreMulti cfg cfg''

theorem restore_iso_multi (cfg cfg' : Config)
    (h : RestoreMulti cfg cfg') (hw : WF cfg) : Iso cfg' := by
  induction h with
  | refl => exact hw.1
  | step hs _ ih => exact ih (restore_wf_pres _ _ hs hw)

theorem restore_multi_race_free (c0 cf : Config) (a1 a2 : Access)
    (hw : WF c0) (hr : RestoreMulti c0 cf)
    (w1 : a1.wt) (w2 : a2.wt)
    (p1 : a1.ok cf) (p2 : a2.ok cf) : Not (isRace a1 a2) :=
  race_freedom cf a1 a2 (restore_iso_multi c0 cf hr hw) w1 w2 p1 p2

/-
  Summary:

  * RestoreOK records the semantic facts the checker must establish at the
    boundary of a `\restore` IIFE, including one or more returned iso roots.
  * RestoreStep is same-goroutine re-isolation as one atomic transition.
  * restore_wf_pres proves that the transition preserves Iso, Fresh, and
    Coherent.
  * restore_multi_race_free composes restore steps with the ordinary race
    freedom argument.

  The syntactic bans in restore-spec.md are intentionally stronger than this
  semantic model. They exist so the implementation can justify RestoreOK without
  recursive function summaries, escape through Go features, or untracked pointer
  aliases.
-/
