package gown

import (
	"fmt"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkGoBorrowEscapesSSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *OstampIndex) CheckerErrors {
	if pkg == nil || ssaPkg == nil || caps == nil {
		return nil
	}
	checker := &ssaGoChecker{
		pkg:      pkg,
		caps:     caps,
		places:   buildSSAPlaceIndex(pkg, ssaPkg, caps),
		reported: make(map[string]bool),
	}
	for _, fn := range collectSSAFunctions(ssaPkg) {
		checker.checkFunction(fn)
	}
	return checker.errs
}

type ssaGoChecker struct {
	pkg      *packages.Package
	caps     *OstampIndex
	places   *SSAPlaceIndex
	errs     CheckerErrors
	reported map[string]bool
}

func (checker *ssaGoChecker) checkFunction(fn *ssa.Function) {
	if fn == nil {
		return
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			goInstr, ok := instr.(*ssa.Go)
			if !ok {
				continue
			}
			checker.checkGo(goInstr)
		}
	}
}

func (checker *ssaGoChecker) checkGo(goInstr *ssa.Go) {
	checker.checkGoCallArgs(goInstr)
	checker.checkGoClosureBindings(goInstr)
}

func (checker *ssaGoChecker) checkGoCallArgs(goInstr *ssa.Go) {
	callee := goInstr.Call.StaticCallee()
	if callee == nil {
		return
	}
	fn, _ := callee.Object().(*types.Func)
	funcCap := checker.caps.FuncCap(fn)
	if funcCap == nil {
		return
	}
	for i, paramCap := range funcCap.Params {
		if paramCap != CapMub && paramCap != CapRob {
			continue
		}
		if i >= len(goInstr.Call.Args) {
			continue
		}
		place, ok := checker.places.PlaceForValue(goInstr.Call.Args[i])
		if !ok || place.Root == nil {
			continue
		}
		checker.reportEscape(goInstr, paramCap, place, "cannot pass inferred %s borrow of %q to goroutine")
	}
}

func (checker *ssaGoChecker) checkGoClosureBindings(goInstr *ssa.Go) {
	closure, _ := goInstr.Call.Value.(*ssa.MakeClosure)
	if closure == nil {
		return
	}
	for _, binding := range closure.Bindings {
		place, ok := checker.places.PlaceForValue(binding)
		if !ok || place.Root == nil {
			continue
		}
		cap := capForSSAPlace(checker.caps, place)
		if cap != CapMub && cap != CapRob {
			continue
		}
		checker.reportEscape(goInstr, cap, place, "cannot capture non-sendable %s value %q in goroutine")
	}
}

func (checker *ssaGoChecker) reportEscape(goInstr *ssa.Go, cap Cap, place Place, format string) {
	name := "<unknown>"
	if place.Root != nil {
		name = place.Root.Name()
	}
	checker.reportCheckerError(newCheckerErrorAtPosition(
		GWN004,
		checker.pkg.Fset.Position(goInstr.Pos()),
		fmt.Sprintf(format, cap, name),
	))
}

func (checker *ssaGoChecker) reportCheckerError(err CheckerError) {
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}
