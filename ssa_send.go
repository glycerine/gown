package gown

import (
	"fmt"
	"go/token"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

func checkSendCapabilitiesSSA(pkg *packages.Package, ssaPkg *ssa.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || ssaPkg == nil || caps == nil {
		return nil
	}
	checker := &ssaSendChecker{
		pkg:      pkg,
		caps:     caps,
		places:   buildSSAPlaceIndex(pkg, ssaPkg, caps),
		bindings: NewSSABindingIndex(caps),
		reported: make(map[string]bool),
	}
	for _, fn := range collectSSAFunctions(ssaPkg) {
		checker.checkFunction(fn)
	}
	return checker.errs
}

type ssaSendChecker struct {
	pkg      *packages.Package
	caps     *CapabilityIndex
	places   *SSAPlaceIndex
	bindings *SSABindingIndex
	errs     CheckerErrors
	reported map[string]bool
}

func (checker *ssaSendChecker) checkFunction(fn *ssa.Function) {
	if fn == nil {
		return
	}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			send, ok := instr.(*ssa.Send)
			if !ok {
				continue
			}
			checker.checkSend(send)
		}
	}
}

func (checker *ssaSendChecker) checkSend(send *ssa.Send) {
	if binding, ok := checker.bindings.Send(checker.pkg, send); ok {
		checker.checkBoundSend(binding, send)
		return
	}
	chPlace, ok := checker.places.PlaceForValue(send.Chan)
	if !ok || chPlace.Root == nil {
		return
	}
	valuePlace, ok := checker.places.PlaceForValue(send.X)
	if !ok || valuePlace.Root == nil {
		return
	}
	chCap := checker.caps.ChanElemCap(chPlace.Root)
	valueCap := capForSSAPlace(checker.caps, valuePlace)
	pos := checker.pkg.Fset.Position(send.Pos())

	if !sendCapabilityAllowed(chCap, valueCap) {
		name := valuePlace.Root.Name()
		checker.reportCheckerError(newCheckerErrorAtPosition(
			GWN010,
			pos,
			fmt.Sprintf("cannot send %s value %q on %s channel", valueCap, name, chCap),
		))
		return
	}
	if !capSendable(valueCap) {
		name := valuePlace.Root.Name()
		checker.reportCheckerError(newCheckerErrorAtPosition(
			GWN003,
			checker.nonSendablePosition(valuePlace, pos),
			fmt.Sprintf("cannot send non-sendable %s value %q", valueCap, name),
		))
	}
}

func (checker *ssaSendChecker) checkBoundSend(binding SendBinding, send *ssa.Send) {
	pos := checker.pkg.Fset.Position(send.Pos())
	if !sendCapabilityAllowed(binding.ChanElemCap, binding.ValueCap) {
		name := "<unknown>"
		if binding.Value.Root != nil {
			name = binding.Value.Root.Name()
		}
		checker.reportCheckerError(newCheckerErrorAtPosition(
			GWN010,
			pos,
			fmt.Sprintf("cannot send %s value %q on %s channel", binding.ValueCap, name, binding.ChanElemCap),
		))
		return
	}
	if !capSendable(binding.ValueCap) {
		name := "<unknown>"
		if binding.Value.Root != nil {
			name = binding.Value.Root.Name()
		}
		checker.reportCheckerError(newCheckerErrorAtPosition(
			GWN003,
			checker.nonSendablePosition(binding.Value, pos),
			fmt.Sprintf("cannot send non-sendable %s value %q", binding.ValueCap, name),
		))
	}
}

func (checker *ssaSendChecker) nonSendablePosition(place Place, fallback token.Position) token.Position {
	obj := capObjectForSSAPlace(checker.caps, place)
	if checker.pkg == nil || obj == nil || !obj.Pos().IsValid() {
		return fallback
	}
	return checker.pkg.Fset.PositionFor(obj.Pos(), false)
}

func (checker *ssaSendChecker) reportCheckerError(err CheckerError) {
	reportCheckerErrorOnce(&checker.errs, checker.reported, err)
}
