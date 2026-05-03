package gown

import (
	"go/ast"
	"go/types"
)

type CapabilityIndex struct {
	ObjectCaps   map[types.Object]Cap
	ChanElemCaps map[types.Object]Cap
	Funcs        map[*types.Func]*FuncCapability
	CallBindings []CallBinding
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
	Callee     *types.Func
	ParamCaps  []Cap
	ResultCaps []Cap
	Args       []ast.Expr
}

func newCapabilityIndex() *CapabilityIndex {
	return &CapabilityIndex{
		ObjectCaps:   make(map[types.Object]Cap),
		ChanElemCaps: make(map[types.Object]Cap),
		Funcs:        make(map[*types.Func]*FuncCapability),
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
