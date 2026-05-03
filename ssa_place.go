package gown

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

type SSAPlaceIndex struct {
	ValuePlaces       map[ssa.Value]Place
	InstructionPlaces map[ssa.Instruction]Place
}

func buildSSAPlaceIndex(pkg *packages.Package, ssaPkg *ssa.Package, caps *CapabilityIndex) *SSAPlaceIndex {
	idx := &SSAPlaceIndex{
		ValuePlaces:       make(map[ssa.Value]Place),
		InstructionPlaces: make(map[ssa.Instruction]Place),
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

func (idx *SSAPlaceIndex) seedFromASTPlaces(pkg *packages.Package, ssaPkg *ssa.Package, places *PlaceIndex) {
	for _, fn := range collectSSAFunctions(ssaPkg) {
		for _, file := range pkg.Syntax {
			ast.Inspect(file, func(n ast.Node) bool {
				expr, ok := n.(ast.Expr)
				if !ok {
					return true
				}
				place, ok := places.PlaceForExpr(expr)
				if !ok {
					return true
				}
				value, _ := fn.ValueForExpr(expr)
				if value != nil {
					idx.ValuePlaces[value] = place
				}
				return true
			})
		}
	}
}

func (idx *SSAPlaceIndex) propagateSSAPlaces(ssaPkg *ssa.Package) {
	for _, fn := range collectSSAFunctions(ssaPkg) {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				switch instr := instr.(type) {
				case *ssa.FieldAddr:
					if place, ok := idx.fieldAddrPlace(instr); ok {
						idx.ValuePlaces[instr] = place
					}
				case *ssa.IndexAddr:
					if place, ok := idx.PlaceForValue(instr.X); ok {
						idx.ValuePlaces[instr] = collapsePlace(place)
					}
				case *ssa.Lookup:
					if place, ok := idx.PlaceForValue(instr.X); ok {
						idx.ValuePlaces[instr] = collapsePlace(place)
					}
				case *ssa.MakeInterface:
					if place, ok := idx.PlaceForValue(instr.X); ok {
						idx.ValuePlaces[instr] = collapsePlace(place)
					}
				case *ssa.Store:
					if place, ok := idx.PlaceForValue(instr.Addr); ok {
						idx.InstructionPlaces[instr] = place
					}
				case *ssa.UnOp:
					if place, ok := idx.PlaceForValue(instr.X); ok {
						idx.ValuePlaces[instr] = place
					}
				}
			}
		}
	}
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
