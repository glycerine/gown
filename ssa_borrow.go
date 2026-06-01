package gown

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkGWN002SSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *OstampIndex) CheckerErrors {
	if pkg == nil || ssaPkg == nil || caps == nil {
		return nil
	}
	checker := &ssaBorrowChecker{
		pkg:      pkg,
		caps:     caps,
		places:   buildSSAPlaceIndex(pkg, ssaPkg, caps),
		reported: make(map[string]bool),
	}
	for _, fn := range collectSSAFunctionsForChecking(ssaPkg, caps) {
		checker.checkFunction(fn)
	}
	return checker.errs
}

type ssaBorrowChecker struct {
	pkg      *packages.Package
	caps     *OstampIndex
	places   *SSAPlaceIndex
	errs     CheckerErrors
	reported map[string]bool
}

func (checker *ssaBorrowChecker) checkFunction(fn *ssa.Function) {
	if fn == nil {
		return
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			call, ok := instr.(*ssa.Call)
			if !ok {
				continue
			}
			checker.checkCall(call)
		}
	}
}

func (checker *ssaBorrowChecker) checkCall(call *ssa.Call) {
	callee := call.Call.StaticCallee()
	if callee == nil {
		return
	}
	fn, _ := callee.Object().(*types.Func)
	funcCap := checker.caps.FuncCap(fn)
	if funcCap == nil {
		return
	}

	state := NewSSAFunctionState()
	var active []SSABorrow
	for i, paramCap := range funcCap.Params {
		if paramCap != CapMub && paramCap != CapRob {
			continue
		}
		if i >= len(call.Call.Args) {
			continue
		}
		place, ok := checker.places.PlaceForValue(call.Call.Args[i])
		if !ok || place.Root == nil {
			continue
		}
		key := place.Key()
		if violation, ok := state.BeginBorrow(key, paramCap); ok {
			checker.reportViolation(callPosition(checker.pkg, call), violation)
			continue
		}
		active = append(active, SSABorrow{Place: key, Cap: paramCap})
	}
	for _, borrow := range active {
		state.EndBorrow(borrow.Place, borrow.Cap)
	}
}

func (checker *ssaBorrowChecker) reportViolation(pos token.Position, violation SSAStateViolation) {
	checker.reportCheckerError(newCheckerErrorAtPosition(
		violation.Code,
		pos,
		violation.Message,
	))
}

func (checker *ssaBorrowChecker) reportCheckerError(err CheckerError) {
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}

func callPosition(pkg *packages.Package, call *ssa.Call) token.Position {
	if pkg == nil || call == nil {
		return token.Position{}
	}
	return pkg.Fset.Position(call.Pos())
}
