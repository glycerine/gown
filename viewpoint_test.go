package gown

import (
	"go/token"
	"go/types"
	"testing"
)

func TestEffectivePlaceCapAppliesViewpointMatrix(t *testing.T) {
	root := types.NewVar(token.NoPos, nil, "root", types.Typ[types.Int])
	field := types.NewVar(token.NoPos, nil, "field", types.Typ[types.Int])
	place := Place{
		Root: root,
		Projection: Projection{{
			Name:  "field",
			Field: field,
		}},
	}

	for _, tt := range []struct {
		name     string
		rootCap  Cap
		fieldCap Cap
		want     Cap
	}{
		{"iso uses declared iso field", CapIso, CapIso, CapIso},
		{"iso uses untracked field as untracked", CapIso, CapUntracked, CapUntracked},
		{"mub downgrades iso field", CapMub, CapIso, CapMub},
		{"mub preserves read field", CapMub, CapRob, CapRob},
		{"rob forces mutable field read-only", CapRob, CapMub, CapRob},
		{"imm forces untracked field immutable", CapImm, CapUntracked, CapImm},
		{"untracked cannot prove iso field", CapUntracked, CapIso, CapUntracked},
		{"untracked preserves declared read field", CapUntracked, CapRob, CapRob},
		{"untracked preserves declared immutable field", CapUntracked, CapImm, CapImm},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caps := newOstampIndex()
			caps.ObjectCaps[root] = tt.rootCap
			if tt.fieldCap != CapUntracked {
				caps.ObjectCaps[field] = tt.fieldCap
			}
			if got := EffectivePlaceCap(caps, place); got != tt.want {
				t.Fatalf("EffectivePlaceCap = %v, want %v", got, tt.want)
			}
		})
	}
}
