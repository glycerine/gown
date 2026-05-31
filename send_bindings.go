package gown

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

type SendBinding struct {
	Stmt        *ast.SendStmt
	Chan        Place
	Value       Place
	ValueFresh  bool
	ValueSource Place
	ValueIso    bool
	ChanElemCap Cap
	ValueCap    Cap
	Offset      int
	Line        int
	Col         int
	Path        string
}

func (binding SendBinding) IsIsoMove() bool {
	return binding.ChanElemCap == CapIso && binding.ValueCap == CapIso
}

func (binding SendBinding) IsIsoConsumingTransfer() bool {
	return binding.ValueCap == CapIso && (binding.ChanElemCap == CapIso || binding.ChanElemCap == CapImm)
}

func (binding SendBinding) TransferKind() string {
	if binding.ChanElemCap == CapImm && binding.ValueCap == CapIso {
		return "freeze send"
	}
	return "send"
}

func (binding SendBinding) ValueKey() PlaceKey {
	return binding.Value.RegionKey()
}

func bindSendBindings(pkg *packages.Package, idx *OstampIndex) {
	if pkg == nil || idx == nil {
		return
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			send, ok := n.(*ast.SendStmt)
			if !ok {
				return true
			}
			bindSendBinding(pkg, idx, send)
			return true
		})
	}
}

func bindSendBinding(pkg *packages.Package, idx *OstampIndex, send *ast.SendStmt) {
	ch, ok := idx.PlaceForExpr(send.Chan)
	if !ok {
		return
	}
	chanElemCap := chanElemCapForPlace(idx, ch)
	value, ok := valueOstampForExpr(pkg, idx, send.Value)
	if !ok {
		value = ValueOstamp{Cap: CapUntracked}
	}
	if chanElemCap == CapIso || chanElemCap == CapImm {
		if value.Cap == CapUntracked && isFreshOwnedValueExpr(send.Value) {
			value = ValueOstamp{Cap: CapIso, Fresh: true}
		}
	}
	pos := pkg.Fset.Position(send.Arrow)
	idx.addSendBinding(SendBinding{
		Stmt:        send,
		Chan:        ch,
		Value:       value.Place,
		ValueFresh:  value.Fresh,
		ValueSource: value.Source,
		ValueIso:    placeCanTransferAsIso(idx, value.Place) || value.Cap == CapIso,
		ChanElemCap: chanElemCap,
		ValueCap:    value.Cap,
		Offset:      pos.Offset,
		Line:        pos.Line,
		Col:         pos.Column,
		Path:        gownSourcePath(pos.Filename),
	})
}

func (idx *OstampIndex) addSendBinding(binding SendBinding) {
	if idx == nil || binding.Stmt == nil {
		return
	}
	idx.SendBindings = append(idx.SendBindings, binding)
	idx.sendBindingByStmt[binding.Stmt] = len(idx.SendBindings) - 1
}
