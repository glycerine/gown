package gown

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
)

type EmitEdit struct {
	Start   int
	End     int
	NewText []byte
	Reason  string
}

func buildEmitSource(pkg *packages.Package, caps *CapabilityIndex, gf *gownFile, src []byte) ([]byte, error) {
	if pkg == nil || caps == nil || gf == nil {
		return src, nil
	}
	file := syntaxFileForGownFile(pkg, gf)
	if file == nil {
		return src, nil
	}
	edits, err := collectIntrinsicEmitEdits(pkg, caps, gf, src)
	if err != nil {
		return nil, err
	}
	edits = append(edits, collectMoveNilEmitEdits(pkg, caps, file, src)...)
	return applyEmitEdits(src, edits)
}

func syntaxFileForGownFile(pkg *packages.Package, gf *gownFile) *ast.File {
	want := generatedGoName(gf.path)
	for _, file := range pkg.Syntax {
		if generatedGoName(pkg.Fset.Position(file.Pos()).Filename) == want {
			return file
		}
	}
	return nil
}

func collectIntrinsicEmitEdits(pkg *packages.Package, caps *CapabilityIndex, gf *gownFile, src []byte) ([]EmitEdit, error) {
	var edits []EmitEdit
	for _, binding := range caps.IntrinsicBindings {
		if binding.Call == nil {
			continue
		}
		if generatedGoName(binding.Path) != generatedGoName(gf.path) {
			continue
		}
		start, end := nodeOffsets(pkg, binding.Call)
		if !validRange(src, start, end) {
			continue
		}
		var replacement []byte
		switch binding.Kind {
		case IntrinsicMub, IntrinsicRob, IntrinsicFreeze, IntrinsicUnsafe:
			replacement = nodeText(pkg, src, binding.Arg)
		case IntrinsicNew:
			arg := nodeText(pkg, src, binding.Arg)
			if len(arg) == 0 {
				continue
			}
			replacement = append([]byte("&"), arg...)
		case IntrinsicClone:
			if err, ok := checkCloneIntrinsic(pkg.TypesInfo, binding); ok {
				return nil, err
			}
			arg := nodeText(pkg, src, binding.Arg)
			if len(arg) == 0 {
				continue
			}
			replacement = append([]byte("("), arg...)
			replacement = append(replacement, []byte(").Clone()")...)
		default:
			continue
		}
		edits = append(edits, EmitEdit{
			Start:   start,
			End:     end,
			NewText: replacement,
			Reason:  "lower intrinsic",
		})
	}
	return edits, nil
}

func collectMoveNilEmitEdits(pkg *packages.Package, caps *CapabilityIndex, file *ast.File, src []byte) []EmitEdit {
	var edits []EmitEdit
	nilSuppressions := collectNilSuppressedImmutableProjectionRoots(caps, file)
	var currentSuppressions map[types.Object]bool
	ast.Inspect(file, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok {
			currentSuppressions = nilSuppressions[fn]
			return true
		}
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		stmt, ok := n.(ast.Stmt)
		if !ok {
			return true
		}
		roots := nilRootsForStatement(pkg, caps, stmt, currentSuppressions)
		if len(roots) == 0 {
			return true
		}
		end := pkg.Fset.Position(stmt.End()).Offset
		if end < 0 || end > len(src) {
			return true
		}
		indent := lineIndent(src, pkg.Fset.Position(stmt.Pos()).Offset)
		var insert bytes.Buffer
		for _, root := range roots {
			insert.WriteByte('\n')
			insert.Write(indent)
			insert.WriteString(root)
			insert.WriteString(" = nil")
		}
		edits = append(edits, EmitEdit{
			Start:   end,
			End:     end,
			NewText: insert.Bytes(),
			Reason:  "nil consumed iso",
		})
		return true
	})
	return edits
}

func collectNilSuppressedImmutableProjectionRoots(caps *CapabilityIndex, file *ast.File) map[*ast.FuncDecl]map[types.Object]bool {
	out := make(map[*ast.FuncDecl]map[types.Object]bool)
	if caps == nil || file == nil {
		return out
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			place, ok := caps.PlaceForExpr(expr)
			if !ok || place.Root == nil || place.Key().Path == "" {
				return true
			}
			if capForSSAPlace(caps, place) != CapImm {
				return true
			}
			if out[fn] == nil {
				out[fn] = make(map[types.Object]bool)
			}
			out[fn][place.Root] = true
			return true
		})
	}
	return out
}

func nilRootsForStatement(pkg *packages.Package, caps *CapabilityIndex, stmt ast.Stmt, suppressNilRoots map[types.Object]bool) []string {
	seen := make(map[string]bool)
	var roots []string
	add := func(place Place) {
		if place.Root == nil || place.Key().Path != "" {
			return
		}
		if suppressNilRoots[place.Root] {
			return
		}
		name := place.Root.Name()
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		roots = append(roots, name)
	}

	switch stmt := stmt.(type) {
	case *ast.SendStmt:
		if binding, ok := caps.SendBinding(stmt); ok && binding.IsIsoConsumingTransfer() {
			add(binding.Value)
		}
	case *ast.AssignStmt:
		for _, place := range assignmentMoveSources(caps, stmt.Lhs, stmt.Rhs) {
			add(place)
		}
	case *ast.DeclStmt:
		if decl, ok := stmt.Decl.(*ast.GenDecl); ok {
			for _, spec := range decl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, place := range assignmentMoveSources(caps, identsAsExprs(valueSpec.Names), valueSpec.Values) {
					add(place)
				}
			}
		}
	}

	ast.Inspect(stmt, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if binding, ok := caps.CallBinding(call); ok {
			for i, paramCap := range binding.ParamCaps {
				if paramCap != CapIso || i >= len(binding.ArgPlaces) {
					continue
				}
				place := binding.ArgPlaces[i]
				if placeCanTransferAsIso(caps, place) {
					add(place)
				}
			}
		}
		if binding, ok := caps.IntrinsicBinding(call); ok && binding.Kind == IntrinsicFreeze {
			if placeCanTransferAsIso(caps, binding.ArgPlace) {
				add(binding.ArgPlace)
			}
		}
		return true
	})

	sort.Strings(roots)
	return roots
}

func assignmentMoveSources(caps *CapabilityIndex, lhs, rhs []ast.Expr) []Place {
	if len(lhs) != len(rhs) {
		return nil
	}
	var out []Place
	for i := range rhs {
		src, ok := caps.PlaceForExpr(rhs[i])
		if !ok || src.Root == nil || !placeCanTransferAsIso(caps, src) || src.Key().Path != "" {
			continue
		}
		dst, ok := caps.PlaceForExpr(lhs[i])
		if !ok || dst.Root == nil || dst.Root == src.Root || capForSSAPlace(caps, dst) != CapIso {
			continue
		}
		out = append(out, src)
	}
	return out
}

func identsAsExprs(idents []*ast.Ident) []ast.Expr {
	exprs := make([]ast.Expr, len(idents))
	for i, id := range idents {
		exprs[i] = id
	}
	return exprs
}

func applyEmitEdits(src []byte, edits []EmitEdit) ([]byte, error) {
	sort.SliceStable(edits, func(i, j int) bool {
		return edits[i].Start > edits[j].Start
	})
	out := append([]byte(nil), src...)
	lastStart := len(out) + 1
	for _, edit := range edits {
		if !validRange(out, edit.Start, edit.End) {
			return nil, fmt.Errorf("invalid emit edit %s range [%d,%d)", edit.Reason, edit.Start, edit.End)
		}
		if edit.End > lastStart {
			return nil, fmt.Errorf("overlapping emit edits around offset %d", edit.Start)
		}
		next := make([]byte, 0, len(out)-(edit.End-edit.Start)+len(edit.NewText))
		next = append(next, out[:edit.Start]...)
		next = append(next, edit.NewText...)
		next = append(next, out[edit.End:]...)
		out = next
		lastStart = edit.Start
	}
	return out, nil
}

func nodeText(pkg *packages.Package, src []byte, node ast.Node) []byte {
	start, end := nodeOffsets(pkg, node)
	if !validRange(src, start, end) {
		return nil
	}
	return append([]byte(nil), src[start:end]...)
}

func nodeOffsets(pkg *packages.Package, node ast.Node) (int, int) {
	if pkg == nil || node == nil {
		return 0, 0
	}
	return pkg.Fset.Position(node.Pos()).Offset, pkg.Fset.Position(node.End()).Offset
}

func validRange(src []byte, start, end int) bool {
	return start >= 0 && end >= start && end <= len(src)
}

func lineIndent(src []byte, offset int) []byte {
	if offset < 0 || offset > len(src) {
		return nil
	}
	start := offset
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	end := start
	for end < len(src) {
		switch src[end] {
		case ' ', '\t':
			end++
		default:
			return append([]byte(nil), src[start:end]...)
		}
	}
	return append([]byte(nil), src[start:end]...)
}
