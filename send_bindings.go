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
}

func bindSendCapabilities(pkg *packages.Package, idx *CapabilityIndex) {
	if pkg == nil || idx == nil {
		return
	}
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			send, ok := n.(*ast.SendStmt)
			if !ok {
				return true
			}
			bindSendCapability(pkg, idx, send)
			return true
		})
	}
}

func bindSendCapability(pkg *packages.Package, idx *CapabilityIndex, send *ast.SendStmt) {
	ch, ok := directRootPlace(pkg, send.Chan)
	if !ok {
		return
	}
	value, ok := directRootPlace(pkg, send.Value)
	if !ok {
		return
	}
	pos := pkg.Fset.Position(send.Arrow)
	idx.SendBindings = append(idx.SendBindings, SendBinding{
		Stmt:        send,
		Chan:        ch,
		Value:       value,
		ChanElemCap: idx.ChanElemCap(ch.Root),
		ValueCap:    idx.ObjectCap(value.Root),
		Offset:      pos.Offset,
		Line:        pos.Line,
		Col:         pos.Column,
	})
	idx.sendBindingByStmt[send] = len(idx.SendBindings) - 1
}
