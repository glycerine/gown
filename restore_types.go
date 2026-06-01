package gown

import (
	"bytes"
	"go/ast"
	"go/types"
)

type RestoreAnnotation struct {
	Span         SourceSpan
	TargetOffset int
	Path         string
}

type RestoreBinding struct {
	Annotation *RestoreAnnotation
	Offset     int
	Line       int
	Col        int
	Path       string
	Assign     *ast.AssignStmt
	Call       *ast.CallExpr
	FuncLit    *ast.FuncLit
	Lhs        []ast.Expr
	LhsPlaces  []Place
	Args       []ast.Expr
	ArgPlaces  []Place
	ParamCaps  []Cap
	ResultCaps []Cap
	Return     *ast.ReturnStmt
}

func recordRestoreAnnotation(gf *gownFile, emit, analysis []byte, span SourceSpan) {
	token := &AnnotationToken{Span: span, Kind: AnnotationRestoreRegion}
	ann := &RestoreAnnotation{
		Span:         span,
		TargetOffset: firstNonSpaceOffset(emit, span.End),
		Path:         gf.path,
	}
	gf.annotations = append(gf.annotations, token)
	gf.restores = append(gf.restores, ann)
	replaceWithSpaces(emit, span)
	replaceWithSpaces(analysis, span)
}

func followedByKeyword(src []byte, offset int, keyword string) bool {
	offset = firstNonSpaceOffset(src, offset)
	if offset == 0 && len(src) > 0 && !isSpace(src[0]) {
		return false
	}
	end := offset + len(keyword)
	if end > len(src) || !bytes.Equal(src[offset:end], []byte(keyword)) {
		return false
	}
	if end < len(src) && isIdentPart(src[end]) {
		return false
	}
	return true
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func restoresByGeneratedFile(files []*gownFile) map[string]map[int]*RestoreAnnotation {
	byFile := make(map[string]map[int]*RestoreAnnotation)
	for _, gf := range files {
		fileName := generatedGoName(gf.path)
		if byFile[fileName] == nil {
			byFile[fileName] = make(map[int]*RestoreAnnotation)
		}
		for _, ann := range gf.restores {
			if ann.TargetOffset == 0 {
				continue
			}
			byFile[fileName][ann.TargetOffset] = ann
		}
	}
	return byFile
}

func (idx *OstampIndex) addRestoreBinding(binding RestoreBinding) {
	if idx == nil || binding.Call == nil || binding.FuncLit == nil || binding.Assign == nil {
		return
	}
	idx.RestoreBindings = append(idx.RestoreBindings, binding)
	i := len(idx.RestoreBindings) - 1
	idx.restoreByCall[binding.Call] = i
	idx.restoreByFuncLit[binding.FuncLit] = i
	idx.restoreByAssign[binding.Assign] = i
}

func (idx *OstampIndex) RestoreBinding(call *ast.CallExpr) (RestoreBinding, bool) {
	if idx == nil || call == nil {
		return RestoreBinding{}, false
	}
	i, ok := idx.restoreByCall[call]
	if !ok || i < 0 || i >= len(idx.RestoreBindings) {
		return RestoreBinding{}, false
	}
	return idx.RestoreBindings[i], true
}

func (idx *OstampIndex) RestoreBindingForFuncLit(fn *ast.FuncLit) (RestoreBinding, bool) {
	if idx == nil || fn == nil {
		return RestoreBinding{}, false
	}
	i, ok := idx.restoreByFuncLit[fn]
	if !ok || i < 0 || i >= len(idx.RestoreBindings) {
		return RestoreBinding{}, false
	}
	return idx.RestoreBindings[i], true
}

func (idx *OstampIndex) RestoreBindingForAssign(stmt *ast.AssignStmt) (RestoreBinding, bool) {
	if idx == nil || stmt == nil {
		return RestoreBinding{}, false
	}
	i, ok := idx.restoreByAssign[stmt]
	if !ok || i < 0 || i >= len(idx.RestoreBindings) {
		return RestoreBinding{}, false
	}
	return idx.RestoreBindings[i], true
}

func (idx *OstampIndex) IsRestoreFuncLit(fn *ast.FuncLit) bool {
	_, ok := idx.RestoreBindingForFuncLit(fn)
	return ok
}

func (binding RestoreBinding) IsZero() bool {
	return binding.Call == nil
}

func (binding RestoreBinding) SourcePath() string {
	if binding.Path != "" {
		return binding.Path
	}
	if binding.Annotation != nil && binding.Annotation.Path != "" {
		return binding.Annotation.Path
	}
	return ""
}

func restoreRootNames(places []Place) map[types.Object]bool {
	out := make(map[types.Object]bool)
	for _, place := range places {
		if place.Root != nil && place.Key().Path == "" {
			out[place.Root] = true
		}
	}
	return out
}

func restoreCallValueOstamp(idx *OstampIndex, call *ast.CallExpr) (ValueOstamp, bool) {
	binding, ok := idx.RestoreBinding(call)
	if !ok || len(binding.ResultCaps) != 1 || binding.ResultCaps[0] != CapIso {
		return ValueOstamp{}, false
	}
	return ValueOstamp{Cap: CapIso, Fresh: true}, true
}

func restoreConsumedNilRoots(caps *OstampIndex, binding RestoreBinding) []Place {
	lhsRoots := restoreRootNames(binding.LhsPlaces)
	var out []Place
	for i, paramCap := range binding.ParamCaps {
		if paramCap != CapIso || i >= len(binding.ArgPlaces) {
			continue
		}
		place := binding.ArgPlaces[i]
		if place.Root == nil || place.Key().Path != "" || !placeCanTransferAsIso(caps, place) {
			continue
		}
		if lhsRoots[place.Root] {
			continue
		}
		out = append(out, place)
	}
	return out
}
