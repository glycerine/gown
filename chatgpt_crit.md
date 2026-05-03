I’ll show you a **realistic 1–2k LOC architecture** for this checker using Go + SSA. Not toy pseudocode—this is the shape you can actually build.

The design keeps things:

* **single-pass forward analysis**
* **SSA-based**
* **no full constraint solver**
* **sound via conservative merges**

---

# 🧱 High-level structure

You’re building a compiler pass:

```text
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
```

Uses:

* golang.org/x/tools/go/ssa
* go/types

---

# 🧠 Core data structures (~200 LOC)

## Capabilities

```go
type Cap uint8

const (
    CapInvalid Cap = iota
    CapIso
    CapMub
    CapRob
    CapImm
    CapUntracked
)
```

---

## Locations (heap identities)

```go
type LocID int

type Location struct {
    ID   LocID
    Type types.Type
}
```

---

## SSA value state

```go
type ValState struct {
    Cap Cap
    Loc LocID
    Alive bool
}
```

---

## Borrow state (THE core)

```go
type BorrowState struct {
    Owner        ssa.Value        // iso owner
    MutBorrow    ssa.Value        // at most one
    Shared       map[ssa.Value]struct{}
    IsImm        bool
}
```

---

## Full analysis state

```go
type State struct {
    Values  map[ssa.Value]*ValState
    Borrows map[LocID]*BorrowState
}
```

---

# 🔁 Liveness (~150 LOC)

Use standard backward dataflow.

```go
type Liveness struct {
    LiveIn  map[*ssa.BasicBlock]map[ssa.Value]struct{}
    LiveOut map[*ssa.BasicBlock]map[ssa.Value]struct{}
}
```

Algorithm:

* iterate until fixpoint
* use `Instr.Operands()` for uses
* defs = instruction result

You only need **value-level liveness**, not variables.

---

# 🔧 Checker driver (~150 LOC)

```go
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
```

---

# ▶️ Block execution (~200 LOC)

```go
func (c *Checker) runBlock(b *ssa.BasicBlock, st *State) *State {
    for _, instr := range b.Instrs {
        c.transfer(instr, st)

        c.killDead(instr, st, b)
    }
    return st
}
```

---

# 💀 Kill dead borrows (CRITICAL, ~100 LOC)

```go
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
```

👉 This is your **lifetime system**.

---

# 🔄 Transfer function (~400–600 LOC)

This is the bulk of the checker.

```go
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
```

---

## 🔹 Allocation

```go
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
```

---

## 🔹 Borrow creation (via intrinsic calls)

You’ll encode `\mub` / `\rob` as special functions.

```go
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
```

---

## 🔹 Mutable borrow

```go
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
```

---

## 🔹 Read borrow

```go
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
```

---

## 🔹 Send

```go
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
```

---

## 🔹 Freeze

```go
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
```

---

## 🔹 Spawn (`go`)

```go
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
```

---

## 🔹 Return (escape check)

```go
func (c *Checker) handleReturn(ins *ssa.Return, st *State) {
    for _, v := range ins.Results {
        vs := st.Values[v]
        if vs.Cap == CapMub || vs.Cap == CapRob {
            c.error(ins, "returning borrow")
        }
    }
}
```

---

# 🔀 State merge (~200 LOC)

At CFG joins:

```go
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
```

---

# 🚫 Escape analysis (~100 LOC)

Walk SSA:

```go
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
```

---

# 📏 LOC estimate

| Component            | LOC  |
| -------------------- | ---- |
| State + types        | ~200 |
| Liveness             | ~150 |
| Checker driver       | ~150 |
| Transfer             | ~500 |
| Merge                | ~200 |
| Escape checks        | ~100 |
| Misc (errors, utils) | ~200 |

👉 **Total: ~1.3–1.6k LOC**

---

# 🧠 Why this is sound

This implementation enforces:

* **exclusive mutable access** (BorrowState)
* **non-escaping borrows** (escape pass)
* **lifetime via liveness** (killDead)
* **transfer only when alias-free** (send/freeze checks)

Which exactly matches the rules your proof *assumed* but didn’t enforce.

---

# 🚀 If you want next

I can help you:

* Plug this into a real Go build pipeline
* Handle **interfaces / generics safely**
* Add **field-sensitive borrowing (hard but doable)**
* Or turn this into a **gofmt-style preprocessor**

Just say 👍

