package gown

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

type SendBinding struct {
	Stmt        *ast.SendStmt
	Chan        Place
	Value       Place
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

func (binding SendBinding) ValueKey() PlaceKey {
	return binding.Value.RegionKey()
}

func bindSendBindings(pkg *packages.Package, idx *CapabilityIndex) {
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

func bindSendBinding(pkg *packages.Package, idx *CapabilityIndex, send *ast.SendStmt) {
	ch, ok := directRootPlace(pkg, send.Chan)
	if !ok {
		return
	}
	value, ok := directRootPlace(pkg, send.Value)
	if !ok {
		return
	}
	pos := pkg.Fset.Position(send.Arrow)
	idx.addSendBinding(SendBinding{
		Stmt:        send,
		Chan:        ch,
		Value:       value,
		ChanElemCap: idx.ChanElemCap(ch.Root),
		ValueCap:    idx.ObjectCap(value.Root),
		Offset:      pos.Offset,
		Line:        pos.Line,
		Col:         pos.Column,
		Path:        gownSourcePath(pos.Filename),
	})
}

func (idx *CapabilityIndex) addSendBinding(binding SendBinding) {
	if idx == nil || binding.Stmt == nil {
		return
	}
	idx.SendBindings = append(idx.SendBindings, binding)
	idx.sendBindingByStmt[binding.Stmt] = len(idx.SendBindings) - 1
}
