package gown

import (
	"go/ast"
	"go/types"
)

type CapabilityIndex struct {
	ObjectCaps        map[types.Object]Cap
	ChanElemCaps      map[types.Object]Cap
	Funcs             map[*types.Func]*FuncCapability
	CallBindings      []CallBinding
	Places            *PlaceIndex
	SendBindings      []SendBinding
	callBindingByCall map[*ast.CallExpr]int
	sendBindingByStmt map[*ast.SendStmt]int
}

type FuncCapability struct {
	Params  []Cap
	Results []Cap
}

type CallBinding struct {
	Offset     int
	Line       int
	Col        int
	FuncName   string
	Call       *ast.CallExpr
	Callee     *types.Func
	ParamCaps  []Cap
	ResultCaps []Cap
	Args       []ast.Expr
	ArgPlaces  []Place
}

func newCapabilityIndex() *CapabilityIndex {
	return &CapabilityIndex{
		ObjectCaps:        make(map[types.Object]Cap),
		ChanElemCaps:      make(map[types.Object]Cap),
		Funcs:             make(map[*types.Func]*FuncCapability),
		callBindingByCall: make(map[*ast.CallExpr]int),
		sendBindingByStmt: make(map[*ast.SendStmt]int),
	}
}

func (idx *CapabilityIndex) ObjectCap(obj types.Object) Cap {
	if idx == nil || obj == nil {
		return CapUntracked
	}
	if cap, ok := idx.ObjectCaps[obj]; ok {
		return cap
	}
	return CapUntracked
}

func (idx *CapabilityIndex) ChanElemCap(obj types.Object) Cap {
	if idx == nil || obj == nil {
		return CapUntracked
	}
	if cap, ok := idx.ChanElemCaps[obj]; ok {
		return cap
	}
	return CapUntracked
}

func (idx *CapabilityIndex) FuncCap(fn *types.Func) *FuncCapability {
	if idx == nil || fn == nil {
		return nil
	}
	return idx.Funcs[fn]
}

func (idx *CapabilityIndex) PlaceForExpr(expr ast.Expr) (Place, bool) {
	if idx == nil || idx.Places == nil {
		return Place{}, false
	}
	return idx.Places.PlaceForExpr(expr)
}

func (idx *CapabilityIndex) CallBinding(call *ast.CallExpr) (CallBinding, bool) {
	if idx == nil || call == nil {
		return CallBinding{}, false
	}
	i, ok := idx.callBindingByCall[call]
	if !ok || i < 0 || i >= len(idx.CallBindings) {
		return CallBinding{}, false
	}
	return idx.CallBindings[i], true
}

func (idx *CapabilityIndex) SendBinding(stmt *ast.SendStmt) (SendBinding, bool) {
	if idx == nil || stmt == nil {
		return SendBinding{}, false
	}
	i, ok := idx.sendBindingByStmt[stmt]
	if !ok || i < 0 || i >= len(idx.SendBindings) {
		return SendBinding{}, false
	}
	return idx.SendBindings[i], true
}
