package gown

import (
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

type CapabilityIndex struct {
	ObjectCaps map[types.Object]Cap
	Funcs      map[*types.Func]*FuncCapability
	Calls      []CallBinding
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
}

func newCapabilityIndex() *CapabilityIndex {
	return &CapabilityIndex{
		ObjectCaps: make(map[types.Object]Cap),
		Funcs:      make(map[*types.Func]*FuncCapability),
	}
}

func assignCapabilities(pkg *packages.Package, files []*gownFile) *CapabilityIndex {
	idx := newCapabilityIndex()
	qualsByFile := capQualifiersByGeneratedFile(files)

	for _, file := range pkg.Syntax {
		fileKey := filepath.Base(pkg.Fset.Position(file.Pos()).Filename)
		quals := qualsByFile[fileKey]
		if len(quals) == 0 {
			continue
		}

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				bindFuncDeclCapabilities(pkg, idx, quals, d)
			case *ast.GenDecl:
				bindGenDeclCapabilities(pkg, idx, quals, d)
			}
		}
		bindCallCapabilities(pkg, idx, file)
	}

	return idx
}

func capQualifiersByGeneratedFile(files []*gownFile) map[string]map[int]*CapQualifierAnnotation {
	byFile := make(map[string]map[int]*CapQualifierAnnotation)
	for _, gf := range files {
		fileName := generatedGoName(gf.path)
		if byFile[fileName] == nil {
			byFile[fileName] = make(map[int]*CapQualifierAnnotation)
		}
		for _, ann := range gf.capQualifiers {
			if ann.TargetOffset == 0 {
				continue
			}
			byFile[fileName][ann.TargetOffset] = ann
		}
	}
	return byFile
}

func generatedGoName(gownPath string) string {
	base := filepath.Base(gownPath)
	if strings.HasSuffix(base, ".gown") {
		return strings.TrimSuffix(base, ".gown") + ".go"
	}
	return base
}

func bindFuncDeclCapabilities(pkg *packages.Package, idx *CapabilityIndex, quals map[int]*CapQualifierAnnotation, fn *ast.FuncDecl) {
	obj, _ := pkg.TypesInfo.Defs[fn.Name].(*types.Func)
	if obj == nil {
		return
	}
	sig, _ := obj.Type().(*types.Signature)
	if sig == nil {
		return
	}

	caps := &FuncCapability{
		Params:  make([]Cap, sig.Params().Len()),
		Results: make([]Cap, sig.Results().Len()),
	}
	fillCaps(caps.Params, CapUntracked)
	fillCaps(caps.Results, CapUntracked)

	bindFieldListCapabilities(pkg, idx, quals, fn.Type.Params, sig.Params(), caps.Params)
	bindFieldListCapabilities(pkg, idx, quals, fn.Type.Results, sig.Results(), caps.Results)

	if hasTrackedCaps(caps.Params) || hasTrackedCaps(caps.Results) {
		idx.Funcs[obj] = caps
	}
}

func bindFieldListCapabilities(pkg *packages.Package, idx *CapabilityIndex, quals map[int]*CapQualifierAnnotation, fields *ast.FieldList, tuple *types.Tuple, out []Cap) {
	if fields == nil || tuple == nil {
		return
	}
	tupleIndex := 0
	for _, field := range fields.List {
		fieldCap := capForType(pkg, quals, field.Type)
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n && tupleIndex < len(out); i++ {
			if fieldCap != CapInvalid {
				out[tupleIndex] = fieldCap
				obj := tuple.At(tupleIndex)
				if obj != nil {
					idx.ObjectCaps[obj] = fieldCap
				}
			}
			tupleIndex++
		}
	}
}

func bindGenDeclCapabilities(pkg *packages.Package, idx *CapabilityIndex, quals map[int]*CapQualifierAnnotation, decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			if s.Type == nil {
				continue
			}
			cap := capForType(pkg, quals, s.Type)
			if cap == CapInvalid {
				continue
			}
			for _, name := range s.Names {
				if obj := pkg.TypesInfo.Defs[name]; obj != nil {
					idx.ObjectCaps[obj] = cap
				}
			}
		case *ast.TypeSpec:
			st, ok := s.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				cap := capForType(pkg, quals, field.Type)
				if cap == CapInvalid {
					continue
				}
				for _, name := range field.Names {
					if obj := pkg.TypesInfo.Defs[name]; obj != nil {
						idx.ObjectCaps[obj] = cap
					}
				}
			}
		}
	}
}

func capForType(pkg *packages.Package, quals map[int]*CapQualifierAnnotation, expr ast.Expr) Cap {
	if expr == nil {
		return CapInvalid
	}
	pos := pkg.Fset.Position(expr.Pos()).Offset
	if ann := quals[pos]; ann != nil {
		return ann.Cap
	}
	return CapInvalid
}

func bindCallCapabilities(pkg *packages.Package, idx *CapabilityIndex, file *ast.File) {
	var funcName string
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			prev := funcName
			funcName = x.Name.Name
			ast.Inspect(x.Body, func(bodyNode ast.Node) bool {
				call, ok := bodyNode.(*ast.CallExpr)
				if !ok {
					return true
				}
				bindCallCapability(pkg, idx, funcName, call)
				return true
			})
			funcName = prev
			return false
		}
		return true
	})
}

func bindCallCapability(pkg *packages.Package, idx *CapabilityIndex, funcName string, call *ast.CallExpr) {
	callee := callCallee(pkg, call)
	if callee == nil {
		return
	}
	sig := idx.Funcs[callee]
	if sig == nil {
		return
	}
	pos := pkg.Fset.Position(call.Pos())
	idx.Calls = append(idx.Calls, CallBinding{
		Offset:     pos.Offset,
		Line:       pos.Line,
		Col:        pos.Column,
		FuncName:   funcName,
		Callee:     callee,
		ParamCaps:  append([]Cap(nil), sig.Params...),
		ResultCaps: append([]Cap(nil), sig.Results...),
	})
}

func callCallee(pkg *packages.Package, call *ast.CallExpr) *types.Func {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		fn, _ := pkg.TypesInfo.Uses[fun].(*types.Func)
		return fn
	case *ast.SelectorExpr:
		fn, _ := pkg.TypesInfo.Uses[fun.Sel].(*types.Func)
		return fn
	default:
		return nil
	}
}

func fillCaps(caps []Cap, cap Cap) {
	for i := range caps {
		caps[i] = cap
	}
}

func hasTrackedCaps(caps []Cap) bool {
	for _, cap := range caps {
		if cap != CapInvalid && cap != CapUntracked {
			return true
		}
	}
	return false
}
