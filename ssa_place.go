package gown

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

type SSAPlaceIndex struct {
	ValuePlaces       map[ssa.Value]Place
	InstructionPlaces map[ssa.Instruction]Place
	AmbiguousValues   map[ssa.Value]bool
	ValueSources      map[ssa.Value]SSASourceSpan
	AmbiguousSources  map[ssa.Value]bool
}

type SSASourceSpan struct {
	Start token.Pos
	End   token.Pos
}

func (span SSASourceSpan) Valid() bool {
	return span.Start.IsValid()
}

func (span SSASourceSpan) Position(fset *token.FileSet) token.Position {
	if fset == nil || !span.Valid() {
		return token.Position{}
	}
	return fset.Position(span.Start)
}

func buildSSAPlaceIndex(pkg *packages.Package, ssaPkg *ssa.Package, caps *OstampIndex) *SSAPlaceIndex {
	idx := &SSAPlaceIndex{
		ValuePlaces:       make(map[ssa.Value]Place),
		InstructionPlaces: make(map[ssa.Instruction]Place),
		AmbiguousValues:   make(map[ssa.Value]bool),
		ValueSources:      make(map[ssa.Value]SSASourceSpan),
		AmbiguousSources:  make(map[ssa.Value]bool),
	}
	if pkg == nil || ssaPkg == nil {
		return idx
	}

	places := (*PlaceIndex)(nil)
	if caps != nil {
		places = caps.Places
	}
	if places == nil {
		places = buildPlaceIndex(pkg)
	}

	idx.seedGlobals(ssaPkg)
	idx.seedFromASTPlaces(pkg, ssaPkg, places)
	idx.propagateSSAPlaces(ssaPkg)
	return idx
}

func (idx *SSAPlaceIndex) PlaceForValue(value ssa.Value) (Place, bool) {
	if idx == nil || value == nil {
		return Place{}, false
	}
	place, ok := idx.ValuePlaces[value]
	return place, ok
}

func (idx *SSAPlaceIndex) ValuePlaceAmbiguous(value ssa.Value) bool {
	if idx == nil || value == nil {
		return false
	}
	return idx.AmbiguousValues[value]
}

func (idx *SSAPlaceIndex) PlaceForInstruction(instr ssa.Instruction) (Place, bool) {
	if idx == nil || instr == nil {
		return Place{}, false
	}
	if value, ok := instr.(ssa.Value); ok {
		if place, ok := idx.PlaceForValue(value); ok {
			return place, true
		}
	}
	place, ok := idx.InstructionPlaces[instr]
	return place, ok
}

func (idx *SSAPlaceIndex) seedGlobals(ssaPkg *ssa.Package) {
	for _, member := range ssaPkg.Members {
		global, ok := member.(*ssa.Global)
		if !ok {
			continue
		}
		if obj, ok := global.Object().(*types.Var); ok {
			idx.setValuePlace(global, Place{Root: obj})
		}
	}
}

func (idx *SSAPlaceIndex) seedFromASTPlaces(pkg *packages.Package, ssaPkg *ssa.Package, places *PlaceIndex) {
	for _, fn := range collectSSAFunctions(ssaPkg) {
		syntax := fn.Syntax()
		if syntax == nil {
			continue
		}
		skips := sourceValueSeedSkips(syntax)
		ast.Inspect(syntax, func(n ast.Node) bool {
			expr, ok := n.(ast.Expr)
			if !ok {
				return true
			}
			if skips[expr] {
				return true
			}
			place, ok := places.PlaceForExpr(expr)
			if !ok {
				return true
			}
			value, _ := fn.ValueForExpr(expr)
			if value != nil {
				idx.setValuePlaceWithSource(value, place, sourceSpanForExpr(expr))
			}
			return true
		})
	}
}

func sourceSpanForExpr(expr ast.Expr) SSASourceSpan {
	if expr == nil {
		return SSASourceSpan{}
	}
	return SSASourceSpan{Start: expr.Pos(), End: expr.End()}
}

func sourceValueSeedSkips(syntax ast.Node) map[ast.Expr]bool {
	skips := make(map[ast.Expr]bool)
	ast.Inspect(syntax, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				skips[lhs] = true
			}
		case *ast.ValueSpec:
			if len(n.Values) > 0 {
				for _, name := range n.Names {
					skips[name] = true
				}
			}
		}
		return true
	})
	return skips
}

func (idx *SSAPlaceIndex) propagateSSAPlaces(ssaPkg *ssa.Package) {
	for _, fn := range collectSSAFunctions(ssaPkg) {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				switch instr := instr.(type) {
				case *ssa.ChangeInterface:
					idx.propagateValuePlace(instr, instr.X)
				case *ssa.ChangeType:
					idx.propagateValuePlace(instr, instr.X)
				case *ssa.Convert:
					idx.propagateValuePlace(instr, instr.X)
				case *ssa.FieldAddr:
					if place, ok := idx.fieldAddrPlace(instr); ok {
						idx.forceValuePlace(instr, place)
						if idx.ValuePlaceAmbiguous(instr.X) {
							idx.AmbiguousValues[instr] = true
						}
					}
				case *ssa.IndexAddr:
					if place, ok := idx.PlaceForValue(instr.X); ok {
						idx.forceValuePlace(instr, collapsePlace(place))
						if idx.ValuePlaceAmbiguous(instr.X) {
							idx.AmbiguousValues[instr] = true
						}
					}
				case *ssa.Lookup:
					if place, ok := idx.PlaceForValue(instr.X); ok {
						idx.forceValuePlace(instr, collapsePlace(place))
						if idx.ValuePlaceAmbiguous(instr.X) {
							idx.AmbiguousValues[instr] = true
						}
					}
				case *ssa.MakeInterface:
					if place, ok := idx.PlaceForValue(instr.X); ok {
						idx.forceValuePlace(instr, collapsePlace(place))
						if idx.ValuePlaceAmbiguous(instr.X) {
							idx.AmbiguousValues[instr] = true
						}
					}
				case *ssa.Phi:
					if place, ok := idx.phiPlace(instr); ok {
						idx.forceValuePlace(instr, place)
					}
				case *ssa.Store:
					if place, ok := idx.PlaceForValue(instr.Addr); ok {
						idx.InstructionPlaces[instr] = place
					}
				case *ssa.UnOp:
					if instr.Op == token.MUL {
						idx.propagateValuePlace(instr, instr.X)
					}
				}
			}
		}
	}
}

func (idx *SSAPlaceIndex) setValuePlace(value ssa.Value, place Place) {
	idx.setValuePlaceWithSource(value, place, SSASourceSpan{})
}

func (idx *SSAPlaceIndex) setValuePlaceWithSource(value ssa.Value, place Place, source SSASourceSpan) {
	if idx == nil || value == nil || place.Root == nil {
		return
	}
	if existing, ok := idx.ValuePlaces[value]; ok && existing.Key() != place.Key() {
		if isBareFieldPlace(existing) && !isBareFieldPlace(place) {
			idx.ValuePlaces[value] = place
			idx.setValueSource(value, source)
			return
		}
		if !isBareFieldPlace(existing) && isBareFieldPlace(place) {
			return
		}
		idx.AmbiguousValues[value] = true
		idx.AmbiguousSources[value] = true
		delete(idx.ValueSources, value)
		return
	}
	idx.ValuePlaces[value] = place
	idx.setValueSource(value, source)
}

func (idx *SSAPlaceIndex) forceValuePlace(value ssa.Value, place Place) {
	if idx == nil || value == nil || place.Root == nil {
		return
	}
	idx.ValuePlaces[value] = place
}

func (idx *SSAPlaceIndex) setValueSource(value ssa.Value, source SSASourceSpan) {
	if idx == nil || value == nil || !source.Valid() {
		return
	}
	if idx.AmbiguousSources[value] {
		return
	}
	if existing, ok := idx.ValueSources[value]; ok && existing != source {
		idx.AmbiguousSources[value] = true
		delete(idx.ValueSources, value)
		return
	}
	idx.ValueSources[value] = source
}

func (idx *SSAPlaceIndex) SourceForValue(value ssa.Value) (SSASourceSpan, bool) {
	if idx == nil || value == nil {
		return SSASourceSpan{}, false
	}
	if idx.AmbiguousSources[value] {
		return SSASourceSpan{}, false
	}
	source, ok := idx.ValueSources[value]
	return source, ok && source.Valid()
}

func (idx *SSAPlaceIndex) propagateValuePlace(dst ssa.Value, src ssa.Value) {
	if place, ok := idx.PlaceForValue(src); ok {
		idx.forceValuePlace(dst, place)
		if _, hasSource := idx.SourceForValue(dst); !hasSource {
			if source, ok := idx.SourceForValue(src); ok {
				idx.setValueSource(dst, source)
			}
		}
		if idx.ValuePlaceAmbiguous(src) {
			idx.AmbiguousValues[dst] = true
		}
	}
}

func isBareFieldPlace(place Place) bool {
	if place.Root == nil || len(place.Projection) != 0 || place.Collapsed {
		return false
	}
	field, ok := place.Root.(*types.Var)
	return ok && field.IsField()
}

func (idx *SSAPlaceIndex) phiPlace(instr *ssa.Phi) (Place, bool) {
	if instr == nil || len(instr.Edges) == 0 {
		return Place{}, false
	}
	var out Place
	var outKey PlaceKey
	for _, edge := range instr.Edges {
		if idx.ValuePlaceAmbiguous(edge) {
			return Place{}, false
		}
		place, ok := idx.PlaceForValue(edge)
		if !ok || place.Root == nil {
			return Place{}, false
		}
		key := place.Key()
		if out.Root == nil {
			out = place
			outKey = key
			continue
		}
		if key != outKey {
			return Place{}, false
		}
	}
	return out, out.Root != nil
}

func (idx *SSAPlaceIndex) fieldAddrPlace(instr *ssa.FieldAddr) (Place, bool) {
	base, ok := idx.PlaceForValue(instr.X)
	if !ok {
		return Place{}, false
	}
	if base.Collapsed {
		return base, true
	}
	field, ok := fieldAddrField(instr)
	if !ok {
		return collapsePlace(base), true
	}
	base.Projection = append(base.Projection, FieldProjection{
		Name:  field.Name(),
		Index: instr.Field,
		Field: field,
	})
	return base, true
}

func fieldAddrField(instr *ssa.FieldAddr) (*types.Var, bool) {
	if instr == nil || instr.X == nil || instr.X.Type() == nil {
		return nil, false
	}
	ptr, ok := instr.X.Type().Underlying().(*types.Pointer)
	if !ok {
		return nil, false
	}
	st, ok := ptr.Elem().Underlying().(*types.Struct)
	if !ok || instr.Field < 0 || instr.Field >= st.NumFields() {
		return nil, false
	}
	return st.Field(instr.Field), true
}

func capObjectForSSAPlace(caps *OstampIndex, place Place) types.Object {
	if caps == nil || place.Root == nil {
		return nil
	}
	if len(place.Projection) > 0 {
		field := place.Projection[len(place.Projection)-1].Field
		if fieldCap := caps.ObjectCap(field); capTracked(fieldCap) {
			return field
		}
	}
	return place.Root
}
