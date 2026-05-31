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
	idx.Places = buildPlaceIndex(pkg)
	qualsByFile := capQualifiersByGeneratedFile(files)
	intrinsicsByFile := intrinsicsByGeneratedFile(files)

	for _, file := range pkg.Syntax {
		fileKey := filepath.Base(pkg.Fset.Position(file.Pos()).Filename)
		quals := qualsByFile[fileKey]
		if len(quals) == 0 {
			continue
		}
		recordInvalidChannelElementQualifiers(pkg, idx, quals, file)

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				bindFuncDeclCapabilities(pkg, idx, quals, d)
				bindFuncBodyCapabilities(pkg, idx, quals, d)
			case *ast.GenDecl:
				bindGenDeclCapabilities(pkg, idx, quals, d)
			}
		}
	}

	for _, file := range pkg.Syntax {
		fileKey := filepath.Base(pkg.Fset.Position(file.Pos()).Filename)
		bindCallCapabilities(pkg, idx, file)
		bindIntrinsicCapabilities(pkg, idx, intrinsicsByFile[fileKey], file)
	}

	bindSendBindings(pkg, idx)

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

func intrinsicsByGeneratedFile(files []*gownFile) map[string]map[int]*IntrinsicAnnotation {
	byFile := make(map[string]map[int]*IntrinsicAnnotation)
	for _, gf := range files {
		fileName := generatedGoName(gf.path)
		if byFile[fileName] == nil {
			byFile[fileName] = make(map[int]*IntrinsicAnnotation)
		}
		for _, ann := range gf.intrinsics {
			byFile[fileName][ann.Span.Offset] = ann
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
				chanElemCap := chanElemCapForType(pkg, quals, field.Type)
				if cap == CapInvalid && chanElemCap == CapInvalid {
					continue
				}
				for _, name := range field.Names {
					if obj := pkg.TypesInfo.Defs[name]; obj != nil {
						bindObjectCaps(idx, obj, cap, chanElemCap)
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
			if cap == CapInvalid {
				cap = isoMoveCapForValueExpr(pkg, idx, spec.Values[i])
			}
			if cap == CapInvalid {
				cap = receiveCapForValueExpr(idx, spec.Values[i])
			}
			if cap == CapInvalid {
				cap = resultCapForValueExpr(pkg, idx, spec.Values[i])
			}
		}
		bindObjectCaps(idx, obj, cap, chanElemCap)
	}
}

func bindAssignStmtCapabilities(pkg *packages.Package, idx *CapabilityIndex, quals map[int]*CapQualifierAnnotation, stmt *ast.AssignStmt) {
	if stmt.Tok != token.DEFINE && stmt.Tok != token.ASSIGN {
		return
	}
	if stmt.Tok == token.ASSIGN {
		return
	}
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	for i, lhs := range stmt.Lhs {
		obj := assignedObject(pkg, lhs)
		if obj == nil {
			continue
		}
		cap, chanElemCap := capsForValueExpr(pkg, quals, stmt.Rhs[i])
		if cap == CapInvalid {
			cap = isoMoveCapForValueExpr(pkg, idx, stmt.Rhs[i])
		}
		if cap == CapInvalid {
			cap = receiveCapForValueExpr(idx, stmt.Rhs[i])
		}
		if cap == CapInvalid {
			cap = resultCapForValueExpr(pkg, idx, stmt.Rhs[i])
		}
		bindObjectCaps(idx, obj, cap, chanElemCap)
	}
}

func assignedObject(pkg *packages.Package, expr ast.Expr) types.Object {
	name, ok := expr.(*ast.Ident)
	if !ok {
		return nil
	}
	if obj := pkg.TypesInfo.Defs[name]; obj != nil {
		return obj
	}
	return pkg.TypesInfo.Uses[name]
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
	if ann, ok := capQualifierForType(pkg, quals, expr); ok {
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

func capQualifierForType(pkg *packages.Package, quals map[int]*CapQualifierAnnotation, expr ast.Expr) (*CapQualifierAnnotation, bool) {
	if pkg == nil || expr == nil {
		return nil, false
	}
	pos := pkg.Fset.Position(expr.Pos()).Offset
	ann := quals[pos]
	return ann, ann != nil
}

func recordInvalidChannelElementQualifiers(pkg *packages.Package, idx *CapabilityIndex, quals map[int]*CapQualifierAnnotation, file *ast.File) {
	if pkg == nil || idx == nil || len(quals) == 0 || file == nil {
		return
	}
	path := gownSourcePath(pkg.Fset.Position(file.Pos()).Filename)
	seen := make(map[int]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		ch, ok := n.(*ast.ChanType)
		if !ok {
			return true
		}
		ann, ok := capQualifierForType(pkg, quals, ch.Value)
		if !ok || (ann.Cap != CapMub && ann.Cap != CapRob) {
			return true
		}
		if seen[ann.Span.Offset] {
			return true
		}
		seen[ann.Span.Offset] = true
		idx.InvalidChannelElementQualifiers = append(idx.InvalidChannelElementQualifiers, InvalidChannelElementQualifier{
			Cap:    ann.Cap,
			Path:   path,
			Offset: ann.Span.Offset,
			Line:   ann.Span.Line,
			Col:    ann.Span.Col,
		})
		return true
	})
}

func capsForValueExpr(pkg *packages.Package, quals map[int]*CapQualifierAnnotation, expr ast.Expr) (Cap, Cap) {
	if isFreshOwnedValueExpr(expr) {
		return CapIso, CapInvalid
	}
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

func isoMoveCapForValueExpr(pkg *packages.Package, idx *CapabilityIndex, expr ast.Expr) Cap {
	place, ok := directRootPlace(pkg, expr)
	if !ok || idx.ObjectCap(place.Root) != CapIso {
		return CapInvalid
	}
	return CapIso
}

func receiveCapForValueExpr(idx *CapabilityIndex, expr ast.Expr) Cap {
	if idx == nil || expr == nil {
		return CapInvalid
	}
	expr = unparenExpr(expr)
	recv, ok := expr.(*ast.UnaryExpr)
	if !ok || recv.Op != token.ARROW {
		return CapInvalid
	}
	place, ok := idx.PlaceForExpr(recv.X)
	if !ok || place.Root == nil {
		return CapInvalid
	}
	cap := chanElemCapForPlace(idx, place)
	if !capTracked(cap) {
		return CapInvalid
	}
	return cap
}

func resultCapForValueExpr(pkg *packages.Package, idx *CapabilityIndex, expr ast.Expr) Cap {
	value, ok := valueCapabilityForExpr(pkg, idx, expr)
	if !ok || !capTracked(value.Cap) {
		return CapInvalid
	}
	return value.Cap
}

func isFreshOwnedValueExpr(expr ast.Expr) bool {
	switch expr := expr.(type) {
	case *ast.ParenExpr:
		return isFreshOwnedValueExpr(expr.X)
	case *ast.UnaryExpr:
		_, ok := expr.X.(*ast.CompositeLit)
		return expr.Op == token.AND && ok
	case *ast.CallExpr:
		fun, ok := expr.Fun.(*ast.Ident)
		return ok && fun.Name == "new"
	default:
		return false
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
