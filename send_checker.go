package gown

import (
	"fmt"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

func checkSendCapabilities(pkg *packages.Package, caps *OstampIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, binding := range caps.SendBindings {
		if err, ok := checkSendOstampMatch(binding); ok {
			errs = append(errs, err)
		}
		if !capSendable(binding.ValueCap) {
			errs = append(errs, nonSendableError(pkg, binding))
		}
	}
	return errs
}

func nonSendableError(pkg *packages.Package, binding SendBinding) CheckerError {
	name := "<unknown>"
	pos := bindingPosition(binding)
	var root types.Object
	if root := binding.ValueKey().Root; root != nil {
		name = root.Name()
	}
	root = binding.ValueKey().Root
	return newCheckerErrorAtObject(
		pkg,
		GWN003,
		root,
		pos,
		fmt.Sprintf("cannot send non-sendable %s value %q", binding.ValueCap, name),
	)
}

func bindingPosition(binding SendBinding) token.Position {
	return token.Position{
		Filename: binding.Path,
		Offset:   binding.Offset,
		Line:     binding.Line,
		Column:   binding.Col,
	}
}

func checkSendOstampMatch(binding SendBinding) (CheckerError, bool) {
	if binding.ValueIso && (binding.ChanElemCap == CapIso || binding.ChanElemCap == CapImm) {
		return CheckerError{}, false
	}
	if sendOstampAllowed(binding.ChanElemCap, binding.ValueCap) {
		return CheckerError{}, false
	}
	name := "<unknown>"
	if root := binding.ValueKey().Root; root != nil {
		name = root.Name()
	}
	return newCheckerErrorAtSource(
		GWN010,
		binding.Path,
		binding.Offset,
		binding.Line,
		binding.Col,
		fmt.Sprintf("cannot send %s value %q on %s channel", binding.ValueCap, name, binding.ChanElemCap),
	), true
}

func capSendable(cap Cap) bool {
	return cap != CapMub && cap != CapRob
}

func sendOstampAllowed(chanElemCap, valueCap Cap) bool {
	if !capTracked(chanElemCap) {
		return true
	}
	if valueCap == chanElemCap {
		return true
	}
	return chanElemCap == CapImm && valueCap == CapIso
}
