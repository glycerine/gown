package gown

import (
	"fmt"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkInterfaceErasureSSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || ssaPkg == nil || caps == nil {
		return nil
	}
	checker := &ssaInterfaceChecker{
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

type ssaInterfaceChecker struct {
	pkg      *packages.Package
	caps     *CapabilityIndex
	places   *SSAPlaceIndex
	errs     CheckerErrors
	reported map[string]bool
}

func (checker *ssaInterfaceChecker) checkFunction(fn *ssa.Function) {
	if fn == nil {
		return
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			makeInterface, ok := instr.(*ssa.MakeInterface)
			if !ok {
				continue
			}
			checker.checkMakeInterface(makeInterface)
		}
	}
}

func (checker *ssaInterfaceChecker) checkMakeInterface(makeInterface *ssa.MakeInterface) {
	place, ok := checker.places.PlaceForValue(makeInterface.X)
	if !ok || place.Root == nil {
		return
	}
	cap := capForSSAPlace(checker.caps, place)
	if !capTracked(cap) {
		return
	}
	checker.reportCheckerError(newCheckerErrorAtPosition(
		GWN009,
		checker.pkg.Fset.Position(makeInterface.Pos()),
		fmt.Sprintf("cannot erase %s value %q into interface", cap, place.Root.Name()),
	))
}

func (checker *ssaInterfaceChecker) reportCheckerError(err CheckerError) {
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}
