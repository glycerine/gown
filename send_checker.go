package gown

import (
	"fmt"
	"go/token"

	"golang.org/x/tools/go/packages"
)

func checkSendCapabilities(pkg *packages.Package, caps *CapabilityIndex) CheckerErrors {
	if pkg == nil || caps == nil {
		return nil
	}
	var errs CheckerErrors
	for _, binding := range caps.SendBindings {
		if err, ok := checkSendCapabilityMatch(binding); ok {
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
	if root := binding.ValueKey().Root; root != nil {
		name = root.Name()
		pos = pkg.Fset.Position(root.Pos())
	}
	return CheckerError{
		Code:    GWN003,
		Path:    gownSourcePath(pos.Filename),
		Offset:  pos.Offset,
		Line:    pos.Line,
		Col:     pos.Column,
		Message: fmt.Sprintf("cannot send non-sendable %s value %q", binding.ValueCap, name),
	}
}

func bindingPosition(binding SendBinding) token.Position {
	return token.Position{
		Filename: binding.Path,
		Offset:   binding.Offset,
		Line:     binding.Line,
		Column:   binding.Col,
	}
}

func checkSendCapabilityMatch(binding SendBinding) (CheckerError, bool) {
	if !capTracked(binding.ChanElemCap) || binding.ValueCap == binding.ChanElemCap {
		return CheckerError{}, false
	}
	name := "<unknown>"
	if root := binding.ValueKey().Root; root != nil {
		name = root.Name()
	}
	return CheckerError{
		Code:    GWN010,
		Path:    binding.Path,
		Offset:  binding.Offset,
		Line:    binding.Line,
		Col:     binding.Col,
		Message: fmt.Sprintf("cannot send %s value %q on %s channel", binding.ValueCap, name, binding.ChanElemCap),
	}, true
}

func capSendable(cap Cap) bool {
	return cap != CapMub && cap != CapRob
}
