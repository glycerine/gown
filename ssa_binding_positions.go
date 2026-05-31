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

type SSABindingIndex struct {
	Sends      map[sourcePosKey]SendBinding
	Calls      map[sourcePosKey]CallBinding
	Intrinsics map[sourcePosKey]IntrinsicBinding
}

func NewSSABindingIndex(caps *OstampIndex) *SSABindingIndex {
	return &SSABindingIndex{
		Sends:      sendBindingsByPosition(caps),
		Calls:      callBindingsByPosition(caps),
		Intrinsics: intrinsicBindingsByPosition(caps),
	}
}

func sendBindingsByPosition(caps *OstampIndex) map[sourcePosKey]SendBinding {
	byPos := make(map[sourcePosKey]SendBinding)
	if caps == nil {
		return byPos
	}
	for _, binding := range caps.SendBindings {
		byPos[sourcePositionKey(bindingPosition(binding))] = binding
	}
	return byPos
}

func callBindingsByPosition(caps *OstampIndex) map[sourcePosKey]CallBinding {
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

func intrinsicBindingsByPosition(caps *OstampIndex) map[sourcePosKey]IntrinsicBinding {
	byPos := make(map[sourcePosKey]IntrinsicBinding)
	if caps == nil {
		return byPos
	}
	for _, binding := range caps.IntrinsicBindings {
		byPos[sourcePosKey{
			Path:   binding.Path,
			Offset: binding.Offset,
			Line:   binding.Line,
			Col:    binding.Col,
		}] = binding
	}
	return byPos
}

func (idx *SSABindingIndex) Send(pkg *packages.Package, send *ssa.Send) (SendBinding, bool) {
	if idx == nil || pkg == nil || send == nil {
		return SendBinding{}, false
	}
	binding, ok := idx.Sends[sourcePositionKey(pkg.Fset.Position(send.Pos()))]
	return binding, ok
}

func (idx *SSABindingIndex) Call(pkg *packages.Package, call *ssa.Call) (CallBinding, bool) {
	if idx == nil || pkg == nil || call == nil {
		return CallBinding{}, false
	}
	pos := sourcePositionKey(pkg.Fset.Position(call.Pos()))
	if binding, ok := idx.Calls[pos]; ok {
		return binding, true
	}
	for _, binding := range idx.Calls {
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

func (idx *SSABindingIndex) Intrinsic(pkg *packages.Package, call *ssa.Call) (IntrinsicBinding, bool) {
	if idx == nil || pkg == nil || call == nil {
		return IntrinsicBinding{}, false
	}
	pos := sourcePositionKey(pkg.Fset.Position(call.Pos()))
	if binding, ok := idx.Intrinsics[pos]; ok {
		return binding, true
	}
	for _, binding := range idx.Intrinsics {
		if binding.Path != pos.Path || binding.Call == nil {
			continue
		}
		end := sourcePositionKey(pkg.Fset.Position(binding.Call.End()))
		if pos.Offset >= binding.Offset && pos.Offset <= end.Offset {
			return binding, true
		}
	}
	return IntrinsicBinding{}, false
}

func (idx *SSABindingIndex) GoCall(pkg *packages.Package, goInstr *ssa.Go) (CallBinding, bool) {
	if idx == nil || pkg == nil || goInstr == nil {
		return CallBinding{}, false
	}
	return idx.callOnLine(pkg, pkg.Fset.Position(goInstr.Pos()), goInstr.Call.StaticCallee())
}

func (idx *SSABindingIndex) DeferCall(pkg *packages.Package, deferInstr *ssa.Defer) (CallBinding, bool) {
	if idx == nil || pkg == nil || deferInstr == nil {
		return CallBinding{}, false
	}
	return idx.callOnLine(pkg, pkg.Fset.Position(deferInstr.Pos()), deferInstr.Call.StaticCallee())
}

func (idx *SSABindingIndex) callOnLine(pkg *packages.Package, callPos token.Position, callee *ssa.Function) (CallBinding, bool) {
	pos := sourcePositionKey(callPos)
	for _, binding := range idx.Calls {
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
