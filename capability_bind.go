package gown

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

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
				bindFuncBodyCapabilities(pkg, idx, quals, d)
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
		fieldCap := directCapForType(pkg, quals, field.Type)
		chanElemCap := chanElemCapForType(pkg, quals, field.Type)
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n && tupleIndex < len(out); i++ {
			obj := tuple.At(tupleIndex)
			if fieldCap != CapInvalid {
				out[tupleIndex] = fieldCap
			}
			bindObjectCaps(idx, obj, fieldCap, chanElemCap)
			tupleIndex++
		}
	}
}

func bindGenDeclCapabilities(pkg *packages.Package, idx *CapabilityIndex, quals map[int]*CapQualifierAnnotation, decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.ValueSpec:
			bindValueSpecCapabilities(pkg, idx, quals, s)
		case *ast.TypeSpec:
			st, ok := s.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				cap := directCapForType(pkg, quals, field.Type)
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

func bindFuncBodyCapabilities(pkg *packages.Package, idx *CapabilityIndex, quals map[int]*CapQualifierAnnotation, fn *ast.FuncDecl) {
	if fn.Body == nil {
		return
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.ValueSpec:
			bindValueSpecCapabilities(pkg, idx, quals, stmt)
		case *ast.AssignStmt:
			bindAssignStmtCapabilities(pkg, idx, quals, stmt)
		}
		return true
	})
}

func bindValueSpecCapabilities(pkg *packages.Package, idx *CapabilityIndex, quals map[int]*CapQualifierAnnotation, spec *ast.ValueSpec) {
	var typeCap Cap = CapInvalid
	var typeChanElemCap Cap = CapInvalid
	if spec.Type != nil {
		typeCap = directCapForType(pkg, quals, spec.Type)
		typeChanElemCap = chanElemCapForType(pkg, quals, spec.Type)
	}
	for i, name := range spec.Names {
		obj := pkg.TypesInfo.Defs[name]
		if obj == nil {
			continue
		}
		cap := typeCap
		chanElemCap := typeChanElemCap
		if spec.Type == nil && i < len(spec.Values) {
			cap, chanElemCap = capsForValueExpr(pkg, quals, spec.Values[i])
		}
		bindObjectCaps(idx, obj, cap, chanElemCap)
	}
}

func bindAssignStmtCapabilities(pkg *packages.Package, idx *CapabilityIndex, quals map[int]*CapQualifierAnnotation, stmt *ast.AssignStmt) {
	if stmt.Tok != token.DEFINE || len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	for i, lhs := range stmt.Lhs {
		name, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}
		obj := pkg.TypesInfo.Defs[name]
		if obj == nil {
			continue
		}
		cap, chanElemCap := capsForValueExpr(pkg, quals, stmt.Rhs[i])
		bindObjectCaps(idx, obj, cap, chanElemCap)
	}
}

func bindObjectCaps(idx *CapabilityIndex, obj types.Object, cap, chanElemCap Cap) {
	if obj == nil {
		return
	}
	if cap != CapInvalid {
		idx.ObjectCaps[obj] = cap
	}
	if chanElemCap != CapInvalid {
		idx.ChanElemCaps[obj] = chanElemCap
	}
}

func directCapForType(pkg *packages.Package, quals map[int]*CapQualifierAnnotation, expr ast.Expr) Cap {
	if expr == nil {
		return CapInvalid
	}
	pos := pkg.Fset.Position(expr.Pos()).Offset
	if ann := quals[pos]; ann != nil {
		return ann.Cap
	}
	return CapInvalid
}

func chanElemCapForType(pkg *packages.Package, quals map[int]*CapQualifierAnnotation, expr ast.Expr) Cap {
	ch, ok := expr.(*ast.ChanType)
	if !ok {
		return CapInvalid
	}
	return directCapForType(pkg, quals, ch.Value)
}

func capsForValueExpr(pkg *packages.Package, quals map[int]*CapQualifierAnnotation, expr ast.Expr) (Cap, Cap) {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return directCapForType(pkg, quals, expr), chanElemCapForType(pkg, quals, expr)
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "make" || len(call.Args) == 0 {
		return CapInvalid, CapInvalid
	}
	return CapInvalid, chanElemCapForType(pkg, quals, call.Args[0])
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
