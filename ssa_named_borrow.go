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
	LiveAfter map[ssa.Instruction]map[types.Object]bool
}

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
		LiveAfter: make(map[ssa.Instruction]map[types.Object]bool),
	}
	if fn == nil || len(info.Borrows) == 0 {
		return liveness
	}

	use := make(map[*ssa.BasicBlock]map[types.Object]bool)
	def := make(map[*ssa.BasicBlock]map[types.Object]bool)
	liveIn := make(map[*ssa.BasicBlock]map[types.Object]bool)
	liveOut := make(map[*ssa.BasicBlock]map[types.Object]bool)

	for _, block := range fn.Blocks {
		blockUse := make(map[types.Object]bool)
		blockDef := make(map[types.Object]bool)
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
		liveIn[block] = make(map[types.Object]bool)
		liveOut[block] = make(map[types.Object]bool)
	}

	changed := true
	for changed {
		changed = false
		for i := len(fn.Blocks) - 1; i >= 0; i-- {
			block := fn.Blocks[i]
			nextOut := make(map[types.Object]bool)
			for _, succ := range block.Succs {
				addNamedBorrowSet(nextOut, liveIn[succ])
			}
			nextIn := copyNamedBorrowSet(nextOut)
			for obj := range def[block] {
				delete(nextIn, obj)
			}
			addNamedBorrowSet(nextIn, use[block])
			if !equalNamedBorrowSet(liveOut[block], nextOut) || !equalNamedBorrowSet(liveIn[block], nextIn) {
				liveOut[block] = nextOut
				liveIn[block] = nextIn
				changed = true
			}
		}
	}

	for _, block := range fn.Blocks {
		live := copyNamedBorrowSet(liveOut[block])
		for i := len(block.Instrs) - 1; i >= 0; i-- {
			instr := block.Instrs[i]
			liveness.LiveAfter[instr] = copyNamedBorrowSet(live)
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

func (liveness *SSANamedBorrowLiveness) LiveAfterInstruction(instr ssa.Instruction) map[types.Object]bool {
	if liveness == nil || instr == nil {
		return nil
	}
	return liveness.LiveAfter[instr]
}

func namedBorrowUses(caps *CapabilityIndex, info SSANamedBorrowInfo, instr ssa.Instruction) map[types.Object]bool {
	uses := make(map[types.Object]bool)
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

func namedBorrowDefs(info SSANamedBorrowInfo, instr ssa.Instruction) map[types.Object]bool {
	defs := make(map[types.Object]bool)
	debug, ok := instr.(*ssa.DebugRef)
	if !ok {
		return defs
	}
	for _, obj := range info.Defs[debugRefExprKey(debug.Expr)] {
		defs[obj] = true
	}
	return defs
}

func copyNamedBorrowSet(src map[types.Object]bool) map[types.Object]bool {
	dst := make(map[types.Object]bool)
	addNamedBorrowSet(dst, src)
	return dst
}

func addNamedBorrowSet(dst, src map[types.Object]bool) {
	for obj := range src {
		dst[obj] = true
	}
}

func equalNamedBorrowSet(a, b map[types.Object]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for obj := range a {
		if !b[obj] {
			return false
		}
	}
	return true
}
