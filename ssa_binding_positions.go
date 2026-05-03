package gown

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

type sourcePosKey struct {
	Path   string
	Offset int
	Line   int
	Col    int
}

func sendBindingsByPosition(caps *CapabilityIndex) map[sourcePosKey]SendBinding {
	byPos := make(map[sourcePosKey]SendBinding)
	if caps == nil {
		return byPos
	}
	for _, binding := range caps.SendBindings {
		byPos[sourcePositionKey(bindingPosition(binding))] = binding
	}
	return byPos
}

func callBindingsByPosition(caps *CapabilityIndex) map[sourcePosKey]CallBinding {
	byPos := make(map[sourcePosKey]CallBinding)
	if caps == nil {
		return byPos
	}
	for _, binding := range caps.CallBindings {
		byPos[sourcePosKey{
			Path:   binding.Path,
			Offset: binding.Offset,
			Line:   binding.Line,
			Col:    binding.Col,
		}] = binding
	}
	return byPos
}

func ssaSendBinding(pkg *packages.Package, bindings map[sourcePosKey]SendBinding, send *ssa.Send) (SendBinding, bool) {
	if pkg == nil || send == nil {
		return SendBinding{}, false
	}
	binding, ok := bindings[sourcePositionKey(pkg.Fset.Position(send.Pos()))]
	return binding, ok
}

func ssaCallBinding(pkg *packages.Package, bindings map[sourcePosKey]CallBinding, call *ssa.Call) (CallBinding, bool) {
	if pkg == nil || call == nil {
		return CallBinding{}, false
	}
	pos := sourcePositionKey(pkg.Fset.Position(call.Pos()))
	if binding, ok := bindings[pos]; ok {
		return binding, true
	}
	for _, binding := range bindings {
		if binding.Path != pos.Path || binding.Call == nil {
			continue
		}
		end := sourcePositionKey(pkg.Fset.Position(binding.Call.End()))
		if pos.Offset >= binding.Offset && pos.Offset <= end.Offset {
			return binding, true
		}
	}
	return CallBinding{}, false
}

func ssaGoCallBinding(pkg *packages.Package, bindings map[sourcePosKey]CallBinding, goInstr *ssa.Go) (CallBinding, bool) {
	if pkg == nil || goInstr == nil {
		return CallBinding{}, false
	}
	pos := sourcePositionKey(pkg.Fset.Position(goInstr.Pos()))
	callee := goInstr.Call.StaticCallee()
	for _, binding := range bindings {
		if binding.Path != pos.Path || binding.Line != pos.Line || binding.Call == nil {
			continue
		}
		if callee != nil && binding.Callee != nil {
			if fn, ok := callee.Object().(*types.Func); ok && fn != binding.Callee {
				continue
			}
		}
		end := sourcePositionKey(pkg.Fset.Position(binding.Call.End()))
		if pos.Offset <= end.Offset {
			return binding, true
		}
	}
	return CallBinding{}, false
}

func sourcePositionKey(pos token.Position) sourcePosKey {
	return sourcePosKey{
		Path:   gownSourcePath(pos.Filename),
		Offset: pos.Offset,
		Line:   pos.Line,
		Col:    pos.Column,
	}
}
