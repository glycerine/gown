package gown

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

type SSANamedBorrow struct {
	Borrow types.Object
	Source Place
	Cap    Cap
}

type SSANamedBorrowInfo struct {
	Borrows map[types.Object]SSANamedBorrow
	Defs    map[ast.Expr][]types.Object
}

type SSANamedBorrowLiveness struct {
	LiveAfter map[ssa.Instruction]objectSet
}

type objectSet map[types.Object]bool

func collectSSANamedBorrows(pkg *packages.Package, caps *CapabilityIndex) map[*types.Func]SSANamedBorrowInfo {
	byFunc := make(map[*types.Func]SSANamedBorrowInfo)
	if pkg == nil || caps == nil {
		return byFunc
	}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fnDecl, ok := decl.(*ast.FuncDecl)
			if !ok || fnDecl.Body == nil {
				continue
			}
			fnObj, _ := pkg.TypesInfo.Defs[fnDecl.Name].(*types.Func)
			if fnObj == nil {
				continue
			}
			info := byFunc[fnObj]
			ast.Inspect(fnDecl.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncLit:
					return false
				case *ast.ValueSpec:
					collectNamedBorrowValueSpec(pkg, caps, &info, n)
				case *ast.AssignStmt:
					collectNamedBorrowAssign(pkg, caps, &info, n)
				}
				return true
			})
			if len(info.Borrows) != 0 {
				byFunc[fnObj] = info
			}
		}
	}
	return byFunc
}

func collectNamedBorrowValueSpec(pkg *packages.Package, caps *CapabilityIndex, info *SSANamedBorrowInfo, spec *ast.ValueSpec) {
	if len(spec.Names) == 0 || len(spec.Names) != len(spec.Values) {
		return
	}
	for i, name := range spec.Names {
		obj, _ := pkg.TypesInfo.Defs[name].(*types.Var)
		recordNamedBorrow(info, caps, obj, spec.Values[i])
	}
}

func collectNamedBorrowAssign(pkg *packages.Package, caps *CapabilityIndex, info *SSANamedBorrowInfo, stmt *ast.AssignStmt) {
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	for i, lhs := range stmt.Lhs {
		target, ok := caps.PlaceForExpr(lhs)
		if !ok || target.Root == nil || len(target.Projection) != 0 || target.Collapsed {
			continue
		}
		obj, _ := target.Root.(*types.Var)
		recordNamedBorrow(info, caps, obj, stmt.Rhs[i])
	}
}

func recordNamedBorrow(info *SSANamedBorrowInfo, caps *CapabilityIndex, obj *types.Var, sourceExpr ast.Expr) {
	if info == nil || obj == nil || sourceExpr == nil {
		return
	}
	borrowCap := caps.ObjectCap(obj)
	if borrowCap != CapMub && borrowCap != CapRob {
		return
	}
	source, ok := caps.PlaceForExpr(sourceExpr)
	if !ok || source.Root == nil || !namedBorrowSourceAllowed(caps, borrowCap, source) {
		return
	}
	if info.Borrows == nil {
		info.Borrows = make(map[types.Object]SSANamedBorrow)
	}
	if info.Defs == nil {
		info.Defs = make(map[ast.Expr][]types.Object)
	}
	info.Borrows[obj] = SSANamedBorrow{
		Borrow: obj,
		Source: source,
		Cap:    borrowCap,
	}
	key := debugRefExprKey(sourceExpr)
	info.Defs[key] = append(info.Defs[key], obj)
}

func namedBorrowSourceAllowed(caps *CapabilityIndex, borrowCap Cap, source Place) bool {
	sourceCap := capForSSAPlace(caps, source)
	switch borrowCap {
	case CapMub:
		return sourceCap == CapIso || sourceCap == CapMub
	case CapRob:
		return sourceCap == CapIso || sourceCap == CapMub || sourceCap == CapRob || sourceCap == CapImm
	default:
		return false
	}
}

func buildSSANamedBorrowLiveness(fn *ssa.Function, caps *CapabilityIndex, info SSANamedBorrowInfo) *SSANamedBorrowLiveness {
	liveness := &SSANamedBorrowLiveness{
		LiveAfter: make(map[ssa.Instruction]objectSet),
	}
	if fn == nil || len(info.Borrows) == 0 {
		return liveness
	}

	use := make(map[*ssa.BasicBlock]objectSet)
	def := make(map[*ssa.BasicBlock]objectSet)
	liveIn := make(map[*ssa.BasicBlock]objectSet)
	liveOut := make(map[*ssa.BasicBlock]objectSet)

	for _, block := range fn.Blocks {
		blockUse := make(objectSet)
		blockDef := make(objectSet)
		for _, instr := range block.Instrs {
			for obj := range namedBorrowUses(caps, info, instr) {
				if !blockDef[obj] {
					blockUse[obj] = true
				}
			}
			for obj := range namedBorrowDefs(info, instr) {
				blockDef[obj] = true
			}
		}
		use[block] = blockUse
		def[block] = blockDef
		liveIn[block] = make(objectSet)
		liveOut[block] = make(objectSet)
	}

	changed := true
	for changed {
		changed = false
		for i := len(fn.Blocks) - 1; i >= 0; i-- {
			block := fn.Blocks[i]
			nextOut := make(objectSet)
			for _, succ := range block.Succs {
				nextOut.addAll(liveIn[succ])
			}
			nextIn := nextOut.clone()
			for obj := range def[block] {
				delete(nextIn, obj)
			}
			nextIn.addAll(use[block])
			if !liveOut[block].equal(nextOut) || !liveIn[block].equal(nextIn) {
				liveOut[block] = nextOut
				liveIn[block] = nextIn
				changed = true
			}
		}
	}

	for _, block := range fn.Blocks {
		live := liveOut[block].clone()
		for i := len(block.Instrs) - 1; i >= 0; i-- {
			instr := block.Instrs[i]
			liveness.LiveAfter[instr] = live.clone()
			for obj := range namedBorrowDefs(info, instr) {
				delete(live, obj)
			}
			for obj := range namedBorrowUses(caps, info, instr) {
				live[obj] = true
			}
		}
	}

	return liveness
}

func (liveness *SSANamedBorrowLiveness) LiveAfterInstruction(instr ssa.Instruction) objectSet {
	if liveness == nil || instr == nil {
		return nil
	}
	return liveness.LiveAfter[instr]
}

func namedBorrowUses(caps *CapabilityIndex, info SSANamedBorrowInfo, instr ssa.Instruction) objectSet {
	uses := make(objectSet)
	debug, ok := instr.(*ssa.DebugRef)
	if !ok || debug.IsAddr {
		return uses
	}
	place, ok := caps.PlaceForExpr(debug.Expr)
	if !ok || place.Root == nil {
		return uses
	}
	if _, ok := info.Borrows[place.Root]; ok {
		uses[place.Root] = true
	}
	return uses
}

func namedBorrowDefs(info SSANamedBorrowInfo, instr ssa.Instruction) objectSet {
	defs := make(objectSet)
	debug, ok := instr.(*ssa.DebugRef)
	if !ok {
		return defs
	}
	for _, obj := range info.Defs[debugRefExprKey(debug.Expr)] {
		defs[obj] = true
	}
	return defs
}

func (set objectSet) clone() objectSet {
	clone := make(objectSet)
	clone.addAll(set)
	return clone
}

func (set objectSet) addAll(src objectSet) {
	for obj := range src {
		set[obj] = true
	}
}

func (set objectSet) equal(other objectSet) bool {
	if len(set) != len(other) {
		return false
	}
	for obj := range set {
		if !other[obj] {
			return false
		}
	}
	return true
}
