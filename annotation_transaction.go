package gown

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type AnnotationTransactionOptions struct {
	Dir     string
	Path    string
	Before  []byte
	After   []byte
	Overlay map[string][]byte
}

type PlannedAnnotationTransaction struct {
	ID          string               `json:"id"`
	Kind        string               `json:"kind"`
	Root        AnnotationChange     `json:"root"`
	Edits       []AnnotationTextEdit `json:"edits"`
	Reasons     []AnnotationReason   `json:"reasons"`
	Suggestions []string             `json:"suggestions,omitempty"`
	SourceHash  map[string]string    `json:"sourceHash,omitempty"`
	Status      string               `json:"status"`
	CreatedAt   time.Time            `json:"createdAt"`
}

type AnnotationChange struct {
	Path      string `json:"path"`
	Offset    int    `json:"offset"`
	End       int    `json:"end"`
	Before    string `json:"before"`
	After     string `json:"after"`
	CapBefore Cap    `json:"capBefore"`
	CapAfter  Cap    `json:"capAfter"`
	Site      string `json:"site"`
	SiteKind  string `json:"siteKind"`
}

type AnnotationTextEdit struct {
	Path    string `json:"path"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	OldText string `json:"oldText"`
	NewText string `json:"newText"`
	Reason  string `json:"reason"`
	Site    string `json:"site"`
	Cap     Cap    `json:"cap"`
}

type AnnotationReason struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

type ForcedAnnotationPropagation struct {
	Dir       string               `json:"dir"`
	Edits     []AnnotationTextEdit `json:"edits"`
	Conflicts []string             `json:"conflicts,omitempty"`
}

type ForcedAnnotationConflicts []string

func (conflicts ForcedAnnotationConflicts) Error() string {
	if len(conflicts) == 1 {
		return conflicts[0]
	}
	return fmt.Sprintf("%s and %d more propagation conflicts", conflicts[0], len(conflicts)-1)
}

type annotationDelta struct {
	kind      string
	path      string
	oldOffset int
	oldEnd    int
	newOffset int
	newEnd    int
	oldLexeme string
	newLexeme string
	oldCap    Cap
	newCap    Cap
}

type annotationSite struct {
	key          string
	label        string
	kind         string
	path         string
	obj          types.Object
	typeExpr     ast.Expr
	insertOffset int
	cap          Cap
	ann          *CapQualifierAnnotation
	chanElem     bool
}

type annotationEdge struct {
	to     string
	reason string
}

type annotationGraph struct {
	sites      map[string]*annotationSite
	objSites   map[types.Object]string
	chanSites  map[types.Object]string
	edges      map[string][]annotationEdge
	quals      map[string]map[int]*CapQualifierAnnotation
	analysis   *GownAnalysis
	sourceByFn map[string][]byte
	pathAlias  map[string]string
}

func PlanAnnotationTransaction(opts AnnotationTransactionOptions) (*PlannedAnnotationTransaction, error) {
	if opts.Dir == "" {
		opts.Dir = filepath.Dir(opts.Path)
	}
	absPath, err := filepath.Abs(opts.Path)
	if err != nil {
		return nil, err
	}
	delta, err := detectAnnotationDelta(absPath, opts.Before, opts.After)
	if err != nil {
		return nil, err
	}
	if delta.kind == "" {
		return nil, errors.New("no single ownerstamp delta detected")
	}

	overlay := make(map[string][]byte, len(opts.Overlay)+1)
	for path, src := range opts.Overlay {
		overlay[path] = src
		if abs, err := filepath.Abs(path); err == nil {
			overlay[abs] = src
		}
	}
	overlay[absPath] = opts.After

	gp := NewGownPackage(opts.Dir)
	analysis, analyzeErr := gp.AnalyzeWithOptions(CheckOptions{
		GownOverlay: overlay,
	})
	if analysis == nil {
		return nil, analyzeErr
	}

	graph := buildAnnotationGraph(analysis, overlay)
	root := graph.rootSite(delta)
	if root == nil {
		return nil, fmt.Errorf("could not map annotation delta to an editable ownerstamp site")
	}

	txn := &PlannedAnnotationTransaction{
		ID:         newAnnotationTransactionID(),
		Kind:       delta.kind,
		CreatedAt:  time.Now().UTC(),
		Status:     "planned",
		SourceHash: sourceHashes(overlay, absPath, opts.Before, opts.After),
		Root: AnnotationChange{
			Path:      absPath,
			Offset:    rootDeltaOffset(delta),
			End:       rootDeltaEnd(delta),
			Before:    delta.oldLexeme,
			After:     delta.newLexeme,
			CapBefore: delta.oldCap,
			CapAfter:  delta.newCap,
			Site:      root.label,
			SiteKind:  root.kind,
		},
	}

	switch delta.kind {
	case "add", "convert":
		graph.planAddOrConvert(txn, root, delta.newCap)
	case "delete":
		txn.Suggestions = append(txn.Suggestions, "Deletion propagation requires applied transaction provenance; gownpls handles this from .gownpls/transactions.jsonl.")
	}
	sortAnnotationEdits(txn.Edits)
	return txn, nil
}

func ForcePropagateAnnotations(dir string) (*ForcedAnnotationPropagation, error) {
	gp := NewGownPackage(dir)
	analysis, err := gp.AnalyzeWithOptions(CheckOptions{})
	if analysis == nil {
		return nil, err
	}
	graph := buildAnnotationGraph(analysis, nil)
	result := graph.planForcedPropagation(dir)
	if len(result.Conflicts) > 0 {
		return result, ForcedAnnotationConflicts(result.Conflicts)
	}
	if err := applyAnnotationEdits(result.Edits); err != nil {
		return result, err
	}
	return result, nil
}

func detectAnnotationDelta(path string, before, after []byte) (annotationDelta, error) {
	prefix := 0
	for prefix < len(before) && prefix < len(after) && before[prefix] == after[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(before)-prefix && suffix < len(after)-prefix &&
		before[len(before)-1-suffix] == after[len(after)-1-suffix] {
		suffix++
	}
	oldSeg := before[prefix : len(before)-suffix]
	newSeg := after[prefix : len(after)-suffix]
	oldTok := capLexemesInSegment(oldSeg)
	newTok := capLexemesInSegment(newSeg)

	delta := annotationDelta{path: path}
	switch {
	case len(oldTok) == 0 && len(newTok) == 1:
		delta.kind = "add"
		delta.newOffset = prefix + newTok[0].off
		delta.newEnd = prefix + newTok[0].end
		delta.newLexeme = newTok[0].lexeme
		delta.newCap = newTok[0].cap
		delta.oldOffset = prefix
		delta.oldEnd = prefix
	case len(oldTok) == 1 && len(newTok) == 0:
		delta.kind = "delete"
		delta.oldOffset = prefix + oldTok[0].off
		delta.oldEnd = prefix + oldTok[0].end
		delta.oldLexeme = oldTok[0].lexeme
		delta.oldCap = oldTok[0].cap
		delta.newOffset = prefix
		delta.newEnd = prefix
	case len(oldTok) == 1 && len(newTok) == 1 && oldTok[0].cap != newTok[0].cap:
		delta.kind = "convert"
		delta.oldOffset = prefix + oldTok[0].off
		delta.oldEnd = prefix + oldTok[0].end
		delta.oldLexeme = oldTok[0].lexeme
		delta.oldCap = oldTok[0].cap
		delta.newOffset = prefix + newTok[0].off
		delta.newEnd = prefix + newTok[0].end
		delta.newLexeme = newTok[0].lexeme
		delta.newCap = newTok[0].cap
	default:
		return annotationDelta{}, errors.New("edit does not contain exactly one complete ownerstamp delta")
	}
	return delta, nil
}

type segmentCapLexeme struct {
	off    int
	end    int
	lexeme string
	cap    Cap
}

func capLexemesInSegment(src []byte) []segmentCapLexeme {
	var out []segmentCapLexeme
	for i := 0; i < len(src); i++ {
		if src[i] != '\\' {
			continue
		}
		for _, cap := range []Cap{CapIso, CapMub, CapRob, CapImm} {
			lexeme := cap.String()
			if strings.HasPrefix(string(src[i:]), lexeme) {
				end := i + len(lexeme)
				if end == len(src) || !isIdentPart(src[end]) {
					out = append(out, segmentCapLexeme{off: i, end: end, lexeme: lexeme, cap: cap})
				}
			}
		}
	}
	return out
}

func buildAnnotationGraph(analysis *GownAnalysis, overlay map[string][]byte) *annotationGraph {
	graph := &annotationGraph{
		sites:      make(map[string]*annotationSite),
		objSites:   make(map[types.Object]string),
		chanSites:  make(map[types.Object]string),
		edges:      make(map[string][]annotationEdge),
		quals:      capQualifiersByGeneratedFile(analysis.Files),
		analysis:   analysis,
		sourceByFn: sourceByFilename(analysis, overlay),
		pathAlias:  pathAliasesFromOverlay(overlay),
	}
	graph.collectSites()
	graph.collectEdges()
	return graph
}

func (graph *annotationGraph) collectSites() {
	pkg := graph.analysis.Package
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				graph.collectFuncSites(d)
				if d.Body != nil {
					ast.Inspect(d.Body, func(n ast.Node) bool {
						switch n := n.(type) {
						case *ast.ValueSpec:
							graph.collectValueSpecSites(n)
						case *ast.AssignStmt:
							graph.collectMakeChanAssignSites(n)
						}
						return true
					})
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						graph.collectValueSpecSites(s)
					case *ast.TypeSpec:
						if st, ok := s.Type.(*ast.StructType); ok {
							graph.collectStructSites(st)
						}
					}
				}
			}
		}
	}
}

func (graph *annotationGraph) collectFuncSites(fn *ast.FuncDecl) {
	obj, _ := graph.analysis.Package.TypesInfo.Defs[fn.Name].(*types.Func)
	if obj == nil {
		return
	}
	sig, _ := obj.Type().(*types.Signature)
	if sig == nil {
		return
	}
	graph.collectFieldListSites(fn.Type.Params, sig.Params(), "param")
	graph.collectFieldListSites(fn.Type.Results, sig.Results(), "result")
}

func (graph *annotationGraph) collectFieldListSites(fields *ast.FieldList, tuple *types.Tuple, kind string) {
	if fields == nil || tuple == nil {
		return
	}
	tupleIndex := 0
	for _, field := range fields.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n && tupleIndex < tuple.Len(); i++ {
			graph.addObjectSite(tuple.At(tupleIndex), kind, field.Type)
			graph.addChanElemSite(tuple.At(tupleIndex), kind+" channel element", field.Type)
			tupleIndex++
		}
	}
}

func (graph *annotationGraph) collectValueSpecSites(spec *ast.ValueSpec) {
	for i, name := range spec.Names {
		obj, _ := graph.analysis.Package.TypesInfo.Defs[name].(*types.Var)
		if obj == nil {
			continue
		}
		if spec.Type != nil {
			graph.addObjectSite(obj, "var", spec.Type)
			graph.addChanElemSite(obj, "var channel element", spec.Type)
		}
		if i < len(spec.Values) {
			graph.addMakeChanValueSite(obj, spec.Values[i])
		}
	}
}

func (graph *annotationGraph) collectMakeChanAssignSites(stmt *ast.AssignStmt) {
	if stmt.Tok != token.DEFINE && stmt.Tok != token.ASSIGN {
		return
	}
	if len(stmt.Lhs) != len(stmt.Rhs) {
		return
	}
	for i, lhs := range stmt.Lhs {
		obj := assignedObject(graph.analysis.Package, lhs)
		if obj == nil {
			continue
		}
		graph.addMakeChanValueSite(obj, stmt.Rhs[i])
	}
}

func (graph *annotationGraph) collectStructSites(st *ast.StructType) {
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			obj, _ := graph.analysis.Package.TypesInfo.Defs[name].(*types.Var)
			if obj == nil {
				continue
			}
			graph.addObjectSite(obj, "field", field.Type)
			graph.addChanElemSite(obj, "field channel element", field.Type)
		}
	}
}

func (graph *annotationGraph) addMakeChanValueSite(obj types.Object, expr ast.Expr) {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "make" {
		return
	}
	if _, ok := call.Args[0].(*ast.ChanType); !ok {
		return
	}
	graph.addChanElemSite(obj, "make channel element", call.Args[0])
}

func (graph *annotationGraph) addObjectSite(obj types.Object, kind string, typ ast.Expr) {
	if obj == nil || typ == nil {
		return
	}
	if _, ok := obj.(*types.Var); !ok {
		return
	}
	site := graph.newSite(objectSiteKey(obj), objectLabel(obj), kind, obj, typ, false)
	graph.objSites[obj] = site.key
}

func (graph *annotationGraph) addChanElemSite(obj types.Object, kind string, typ ast.Expr) {
	ch, ok := typ.(*ast.ChanType)
	if !ok || obj == nil {
		return
	}
	site := graph.newSite(channelSiteKey(obj), objectLabel(obj)+" channel element", kind, obj, ch.Value, true)
	graph.chanSites[obj] = site.key
}

func (graph *annotationGraph) newSite(key, label, kind string, obj types.Object, typ ast.Expr, chanElem bool) *annotationSite {
	if site := graph.sites[key]; site != nil {
		return site
	}
	pos := graph.analysis.Package.Fset.Position(typ.Pos())
	insertOffset := pos.Offset
	fileKey := filepath.Base(pos.Filename)
	ann := graph.quals[fileKey][insertOffset]
	cap := CapUntracked
	if ann != nil {
		cap = ann.Cap
	}
	path := graph.displayPath(gownSourcePath(pos.Filename))
	site := &annotationSite{
		key:          key,
		label:        label,
		kind:         kind,
		path:         path,
		obj:          obj,
		typeExpr:     typ,
		insertOffset: insertOffset,
		cap:          cap,
		ann:          ann,
		chanElem:     chanElem,
	}
	graph.sites[key] = site
	return site
}

func (graph *annotationGraph) collectEdges() {
	caps := graph.analysis.Caps
	for _, binding := range caps.CallBindings {
		sig, _ := binding.Callee.Type().(*types.Signature)
		if sig == nil {
			continue
		}
		for i, place := range binding.ArgPlaces {
			if i >= sig.Params().Len() || i >= len(binding.ParamCaps) || binding.ParamCaps[i] == CapUntracked {
				continue
			}
			graph.addEdge(objectSiteKey(sig.Params().At(i)), graph.placeSiteKey(place), fmt.Sprintf("call to %s argument %d", binding.Callee.Name(), i+1))
		}
	}
	for _, binding := range caps.SendBindings {
		graph.addEdge(channelSiteKey(binding.Chan.Root), graph.placeSiteKey(binding.Value), "channel send")
	}
	graph.collectSourceEdges()
}

func (graph *annotationGraph) collectSourceEdges() {
	pkg := graph.analysis.Package
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			fnObj, _ := pkg.TypesInfo.Defs[fn.Name].(*types.Func)
			if fnObj == nil {
				continue
			}
			sig, _ := fnObj.Type().(*types.Signature)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CallExpr:
					callee := callCallee(pkg, n)
					if callee != nil {
						if sig, _ := callee.Type().(*types.Signature); sig != nil {
							if len(n.Args) == 1 {
								if callArg, ok := n.Args[0].(*ast.CallExpr); ok {
									graph.addCallResultEdges(callArg, sig.Params().Len(), func(i int) string {
										return objectSiteKey(sig.Params().At(i))
									}, "call result argument")
								}
							}
							for i, arg := range n.Args {
								if i < sig.Params().Len() {
									paramKey := objectSiteKey(sig.Params().At(i))
									graph.addEdge(paramKey, graph.exprSiteKey(arg), "call argument")
									if callArg, ok := arg.(*ast.CallExpr); ok {
										graph.addCallResultEdges(callArg, 1, func(int) string {
											return paramKey
										}, "call result argument")
									}
								}
							}
						}
					}
				case *ast.ReturnStmt:
					if sig != nil {
						if len(n.Results) == 1 {
							if call, ok := n.Results[0].(*ast.CallExpr); ok {
								graph.addCallResultEdges(call, sig.Results().Len(), func(i int) string {
									return objectSiteKey(sig.Results().At(i))
								}, "return call result")
							}
						}
						for i, expr := range n.Results {
							if i < sig.Results().Len() {
								graph.addEdge(objectSiteKey(sig.Results().At(i)), graph.exprSiteKey(expr), "return result")
								if call, ok := expr.(*ast.CallExpr); ok {
									resultIndex := i
									graph.addCallResultEdges(call, 1, func(int) string {
										return objectSiteKey(sig.Results().At(resultIndex))
									}, "return call result")
								}
							}
						}
					}
				case *ast.SendStmt:
					call, ok := n.Value.(*ast.CallExpr)
					if !ok {
						break
					}
					ch, ok := directRootPlace(pkg, n.Chan)
					if !ok {
						break
					}
					graph.addCallResultEdges(call, 1, func(int) string {
						return channelSiteKey(ch.Root)
					}, "channel send call result")
				case *ast.AssignStmt:
					if len(n.Lhs) == len(n.Rhs) {
						for i := range n.Lhs {
							graph.addEdge(graph.exprSiteKey(n.Lhs[i]), graph.exprSiteKey(n.Rhs[i]), "assignment")
						}
					}
					if len(n.Rhs) == 1 {
						if call, ok := n.Rhs[0].(*ast.CallExpr); ok {
							graph.addCallResultEdges(call, len(n.Lhs), func(i int) string {
								return graph.exprSiteKey(n.Lhs[i])
							}, "call result assignment")
						}
					} else if len(n.Lhs) == len(n.Rhs) {
						for i, rhs := range n.Rhs {
							call, ok := rhs.(*ast.CallExpr)
							if !ok {
								continue
							}
							lhsIndex := i
							graph.addCallResultEdges(call, 1, func(int) string {
								return graph.exprSiteKey(n.Lhs[lhsIndex])
							}, "call result assignment")
						}
					}
				case *ast.ValueSpec:
					if len(n.Values) == 1 {
						if call, ok := n.Values[0].(*ast.CallExpr); ok {
							graph.addCallResultEdges(call, len(n.Names), func(i int) string {
								obj, _ := pkg.TypesInfo.Defs[n.Names[i]].(*types.Var)
								return objectSiteKey(obj)
							}, "call result initialization")
						}
					}
					for i, name := range n.Names {
						if i < len(n.Values) {
							obj, _ := pkg.TypesInfo.Defs[name].(*types.Var)
							graph.addEdge(objectSiteKey(obj), graph.exprSiteKey(n.Values[i]), "variable initialization")
							if call, ok := n.Values[i].(*ast.CallExpr); ok {
								nameIndex := i
								graph.addCallResultEdges(call, 1, func(int) string {
									obj, _ := pkg.TypesInfo.Defs[n.Names[nameIndex]].(*types.Var)
									return objectSiteKey(obj)
								}, "call result initialization")
							}
						}
					}
				}
				return true
			})
		}
	}
}

func (graph *annotationGraph) addCallResultEdges(call *ast.CallExpr, targetCount int, targetKey func(int) string, reason string) {
	if call == nil || targetCount == 0 {
		return
	}
	callee := callCallee(graph.analysis.Package, call)
	if callee == nil {
		return
	}
	sig, _ := callee.Type().(*types.Signature)
	if sig == nil || sig.Results() == nil || sig.Results().Len() == 0 {
		return
	}
	results := sig.Results()
	if targetCount != results.Len() {
		if targetCount != 1 || results.Len() != 1 {
			return
		}
	}
	for i := 0; i < results.Len() && i < targetCount; i++ {
		graph.addEdge(objectSiteKey(results.At(i)), targetKey(i), reason)
	}
}

func (graph *annotationGraph) addEdge(a, b, reason string) {
	if a == "" || b == "" || a == b {
		return
	}
	if graph.sites[a] == nil || graph.sites[b] == nil {
		return
	}
	graph.edges[a] = append(graph.edges[a], annotationEdge{to: b, reason: reason})
	graph.edges[b] = append(graph.edges[b], annotationEdge{to: a, reason: reason})
}

func (graph *annotationGraph) exprSiteKey(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	if place, ok := graph.analysis.Caps.PlaceForExpr(expr); ok {
		return graph.placeSiteKey(place)
	}
	if obj := assignedObject(graph.analysis.Package, expr); obj != nil {
		return objectSiteKey(obj)
	}
	return ""
}

func (graph *annotationGraph) placeSiteKey(place Place) string {
	if len(place.Projection) > 0 {
		if field := place.Projection[len(place.Projection)-1].Field; field != nil {
			return objectSiteKey(field)
		}
	}
	return objectSiteKey(place.Root)
}

func (graph *annotationGraph) rootSite(delta annotationDelta) *annotationSite {
	if delta.kind == "add" || delta.kind == "convert" {
		for _, site := range graph.sites {
			if !sameFilePath(site.path, delta.path) {
				continue
			}
			if site.ann != nil && site.ann.Span.Offset == delta.newOffset {
				return site
			}
		}
	}
	offset := delta.newOffset
	if delta.kind == "delete" {
		offset = delta.newOffset
	}
	for _, site := range graph.sites {
		if !sameFilePath(site.path, delta.path) {
			continue
		}
		if site.insertOffset >= offset && site.insertOffset <= offset+len(delta.newLexeme)+2 {
			return site
		}
		if delta.kind == "delete" && site.insertOffset >= offset && site.insertOffset <= offset+len(delta.oldLexeme)+2 {
			return site
		}
	}
	return nil
}

func (graph *annotationGraph) planAddOrConvert(txn *PlannedAnnotationTransaction, root *annotationSite, cap Cap) {
	seen := make(map[string]bool)
	queue := []string{root.key}
	seen[root.key] = true
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		for _, edge := range graph.edges[key] {
			txn.Reasons = append(txn.Reasons, AnnotationReason{
				From:   graph.sites[key].label,
				To:     graph.sites[edge.to].label,
				Reason: edge.reason,
			})
			if !seen[edge.to] {
				seen[edge.to] = true
				queue = append(queue, edge.to)
			}
		}
	}
	for key := range seen {
		if key == root.key {
			continue
		}
		site := graph.sites[key]
		if site == nil {
			continue
		}
		if site.chanElem && cap != CapIso && cap != CapImm {
			txn.Suggestions = append(txn.Suggestions,
				fmt.Sprintf("%s is a channel element; v1 only auto-applies \\iso or \\imm channel element annotations", site.label))
			continue
		}
		if site.cap == cap {
			continue
		}
		edit, ok := graph.editForSite(site, cap, "implied by "+root.label)
		if ok {
			txn.Edits = append(txn.Edits, edit)
		}
	}
}

func (graph *annotationGraph) planForcedPropagation(dir string) *ForcedAnnotationPropagation {
	result := &ForcedAnnotationPropagation{Dir: dir}
	seen := make(map[string]bool)
	editByKey := make(map[string]AnnotationTextEdit)

	for key := range graph.sites {
		if seen[key] {
			continue
		}
		component := graph.connectedComponent(key)
		for _, componentSite := range component {
			seen[componentSite.key] = true
		}
		cap, roots, conflicts := graph.componentOstamp(component)
		if len(conflicts) > 0 {
			result.Conflicts = append(result.Conflicts, conflicts...)
			continue
		}
		if cap == CapUntracked {
			continue
		}
		for _, site := range component {
			if site.cap == cap {
				continue
			}
			if site.chanElem && cap != CapIso && cap != CapImm {
				result.Conflicts = append(result.Conflicts,
					fmt.Sprintf("%s implies %s for channel element %s, but channel elements may only be \\iso or \\imm",
						strings.Join(roots, ", "), cap, site.label))
				continue
			}
			edit, ok := graph.editForSite(site, cap, "forced propagation from "+strings.Join(roots, ", "))
			if !ok {
				continue
			}
			editKey := fmt.Sprintf("%s:%d:%d:%s", edit.Path, edit.Start, edit.End, edit.NewText)
			editByKey[editKey] = edit
		}
	}
	for _, edit := range editByKey {
		result.Edits = append(result.Edits, edit)
	}
	sortAnnotationEdits(result.Edits)
	sort.Strings(result.Conflicts)
	return result
}

func (graph *annotationGraph) connectedComponent(start string) []*annotationSite {
	seen := make(map[string]bool)
	queue := []string{start}
	seen[start] = true
	var component []*annotationSite
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if site := graph.sites[key]; site != nil {
			component = append(component, site)
		}
		for _, edge := range graph.edges[key] {
			if !seen[edge.to] {
				seen[edge.to] = true
				queue = append(queue, edge.to)
			}
		}
	}
	return component
}

func (graph *annotationGraph) componentOstamp(component []*annotationSite) (Cap, []string, []string) {
	cap := CapUntracked
	var roots []string
	var conflicts []string
	for _, site := range component {
		if !capTracked(site.cap) || site.ann == nil {
			continue
		}
		roots = append(roots, site.label+" "+site.cap.String())
		if cap == CapUntracked {
			cap = site.cap
			continue
		}
		if cap != site.cap {
			conflicts = append(conflicts,
				fmt.Sprintf("conflicting annotations in implied set: %s wants %s but %s is %s",
					strings.Join(roots[:len(roots)-1], ", "), cap, site.label, site.cap))
		}
	}
	sort.Strings(roots)
	return cap, roots, conflicts
}

func applyAnnotationEdits(edits []AnnotationTextEdit) error {
	byPath := make(map[string][]AnnotationTextEdit)
	for _, edit := range edits {
		byPath[edit.Path] = append(byPath[edit.Path], edit)
	}
	for path, pathEdits := range byPath {
		sortAnnotationEdits(pathEdits)
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		next := append([]byte(nil), src...)
		for _, edit := range pathEdits {
			if edit.Start < 0 || edit.End < edit.Start || edit.End > len(next) {
				return fmt.Errorf("%s: invalid edit range [%d,%d)", path, edit.Start, edit.End)
			}
			if edit.OldText != "" && string(next[edit.Start:edit.End]) != edit.OldText {
				return fmt.Errorf("%s: edit at offset %d expected %q, found %q",
					path, edit.Start, edit.OldText, string(next[edit.Start:edit.End]))
			}
			replacement := []byte(edit.NewText)
			updated := make([]byte, 0, len(next)-(edit.End-edit.Start)+len(replacement))
			updated = append(updated, next[:edit.Start]...)
			updated = append(updated, replacement...)
			updated = append(updated, next[edit.End:]...)
			next = updated
		}
		if err := os.WriteFile(path, next, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func (graph *annotationGraph) editForSite(site *annotationSite, cap Cap, reason string) (AnnotationTextEdit, bool) {
	start, end := site.insertOffset, site.insertOffset
	if site.ann != nil {
		start, end = site.ann.Span.Offset, site.ann.Span.End
	}
	oldText := graph.textAt(site.path, start, end)
	newText := cap.String()
	if site.ann == nil {
		newText += " "
	}
	return AnnotationTextEdit{
		Path:    site.path,
		Start:   start,
		End:     end,
		OldText: oldText,
		NewText: newText,
		Reason:  reason,
		Site:    site.label,
		Cap:     cap,
	}, true
}

func (graph *annotationGraph) textAt(path string, start, end int) string {
	src := graph.sourceByFn[path]
	if len(src) == 0 {
		if data, err := os.ReadFile(path); err == nil {
			src = data
		}
	}
	if start < 0 || end < start || end > len(src) {
		return ""
	}
	return string(src[start:end])
}

func (graph *annotationGraph) displayPath(path string) string {
	if graph == nil || len(graph.pathAlias) == 0 {
		return path
	}
	realPath, err := canonicalFilePath(path)
	if err != nil {
		return path
	}
	if alias := graph.pathAlias[realPath]; alias != "" {
		return alias
	}
	return path
}

func sourceByFilename(analysis *GownAnalysis, overlay map[string][]byte) map[string][]byte {
	out := make(map[string][]byte)
	for path, src := range overlay {
		abs, err := filepath.Abs(path)
		if err == nil {
			out[abs] = src
		}
		if realPath, err := canonicalFilePath(path); err == nil {
			out[realPath] = src
		}
		out[path] = src
	}
	if analysis != nil {
		for _, file := range analysis.Package.Syntax {
			pos := analysis.Package.Fset.Position(file.Pos())
			gownPath := gownSourcePath(pos.Filename)
			if _, ok := out[gownPath]; !ok {
				if data, err := os.ReadFile(gownPath); err == nil {
					out[gownPath] = data
				}
			}
		}
	}
	return out
}

func pathAliasesFromOverlay(overlay map[string][]byte) map[string]string {
	aliases := make(map[string]string)
	for path := range overlay {
		absPath, err := filepath.Abs(path)
		if err != nil {
			absPath = path
		}
		realPath, err := canonicalFilePath(path)
		if err != nil {
			continue
		}
		if _, ok := aliases[realPath]; !ok {
			aliases[realPath] = absPath
		}
	}
	return aliases
}

func sameFilePath(a, b string) bool {
	if a == b {
		return true
	}
	realA, errA := canonicalFilePath(a)
	realB, errB := canonicalFilePath(b)
	return errA == nil && errB == nil && realA == realB
}

func sourceHashes(overlay map[string][]byte, rootPath string, before, after []byte) map[string]string {
	hashes := make(map[string]string)
	for path, src := range overlay {
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		hashes[abs] = sha256Hex(src)
	}
	hashes[rootPath+":before"] = sha256Hex(before)
	hashes[rootPath+":after"] = sha256Hex(after)
	return hashes
}

func sha256Hex(src []byte) string {
	sum := sha256.Sum256(src)
	return hex.EncodeToString(sum[:])
}

func newAnnotationTransactionID() string {
	return "anno-" + time.Now().UTC().Format("20060102T150405.000000000")
}

func objectSiteKey(obj types.Object) string {
	if obj == nil {
		return ""
	}
	return fmt.Sprintf("obj:%p", obj)
}

func channelSiteKey(obj types.Object) string {
	if obj == nil {
		return ""
	}
	return fmt.Sprintf("chan:%p", obj)
}

func objectLabel(obj types.Object) string {
	if obj == nil {
		return "<unknown>"
	}
	if obj.Pkg() != nil {
		return obj.Pkg().Path() + "." + obj.Name()
	}
	return obj.Name()
}

func sortAnnotationEdits(edits []AnnotationTextEdit) {
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].Path != edits[j].Path {
			return edits[i].Path < edits[j].Path
		}
		return edits[i].Start > edits[j].Start
	})
}

func rootDeltaOffset(delta annotationDelta) int {
	if delta.kind == "delete" {
		return delta.oldOffset
	}
	return delta.newOffset
}

func rootDeltaEnd(delta annotationDelta) int {
	if delta.kind == "delete" {
		return delta.oldEnd
	}
	return delta.newEnd
}
