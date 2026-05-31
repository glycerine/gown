package gown

import (
	"go/ast"
	"go/types"
)

type CapabilityIndex struct {
	ObjectCaps                      map[types.Object]Cap
	ChanElemCaps                    map[types.Object]Cap
	Funcs                           map[*types.Func]*FuncCapability
	CallBindings                    []CallBinding
	IntrinsicBindings               []IntrinsicBinding
	InvalidChannelElementQualifiers []InvalidChannelElementQualifier
	Places                          *PlaceIndex
	SendBindings                    []SendBinding
	callBindingByCall               map[*ast.CallExpr]int
	intrinsicByCall                 map[*ast.CallExpr]int
	sendBindingByStmt               map[*ast.SendStmt]int
}

type FuncCapability struct {
	Params  []Cap
	Results []Cap
}

type CallBinding struct {
	Offset     int
	Line       int
	Col        int
	Path       string
	FuncName   string
	Call       *ast.CallExpr
	Callee     *types.Func
	ParamCaps  []Cap
	ResultCaps []Cap
	Args       []ast.Expr
	ArgPlaces  []Place
}

type IntrinsicBinding struct {
	Kind     IntrinsicKind
	Offset   int
	Line     int
	Col      int
	Path     string
	Call     *ast.CallExpr
	Arg      ast.Expr
	ArgPlace Place
	Result   types.Object
}

type InvalidChannelElementQualifier struct {
	Cap    Cap
	Path   string
	Offset int
	Line   int
	Col    int
}

func newCapabilityIndex() *CapabilityIndex {
	return &CapabilityIndex{
		ObjectCaps:        make(map[types.Object]Cap),
		ChanElemCaps:      make(map[types.Object]Cap),
		Funcs:             make(map[*types.Func]*FuncCapability),
		callBindingByCall: make(map[*ast.CallExpr]int),
		intrinsicByCall:   make(map[*ast.CallExpr]int),
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

func chanElemCapForPlace(idx *CapabilityIndex, place Place) Cap {
	if idx == nil || place.Root == nil {
		return CapInvalid
	}
	obj := place.Root
	if len(place.Projection) > 0 {
		field := place.Projection[len(place.Projection)-1].Field
		if field == nil {
			return CapUntracked
		}
		obj = field
	}
	return idx.ChanElemCap(obj)
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

func (idx *CapabilityIndex) IntrinsicBinding(call *ast.CallExpr) (IntrinsicBinding, bool) {
	if idx == nil || call == nil {
		return IntrinsicBinding{}, false
	}
	i, ok := idx.intrinsicByCall[call]
	if !ok || i < 0 || i >= len(idx.IntrinsicBindings) {
		return IntrinsicBinding{}, false
	}
	return idx.IntrinsicBindings[i], true
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
