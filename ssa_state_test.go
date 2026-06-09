package gown

import (
	"go/token"
	"go/types"
	"testing"
)

func TestSSAFunctionStateRejectsProjectedMoves(t *testing.T) {
	x := ssaStateTestRoot("x")
	state := NewSSAFunctionState()

	if violation, ok := state.ConsumeRoot(PlaceKey{Root: x, Path: ".f"}, SSAMoveSite{Kind: "send"}); !ok {
		t.Fatal("expected projected move violation")
	} else if violation.Code != GWN011 {
		t.Fatalf("projected move code = %s, want %s", violation.Code, GWN011)
	}

	if _, moved := state.CheckUse(PlaceKey{Root: x}); moved {
		t.Fatal("rejected projected move should not consume root")
	}
}

func TestSSAFunctionStateConsumesProjectedPlacePrecisely(t *testing.T) {
	x := ssaStateTestRoot("x")
	state := NewSSAFunctionState()

	if violation, ok := state.ConsumePlace(PlaceKey{Root: x, Path: ".f"}, SSAMoveSite{Kind: "call"}); ok {
		t.Fatalf("unexpected projected consume violation: %#v", violation)
	}
	if _, moved := state.CheckUse(PlaceKey{Root: x, Path: ".f"}); !moved {
		t.Fatal("field use should be invalid after projected consume")
	}
	if _, moved := state.CheckUse(PlaceKey{Root: x, Path: ".g"}); moved {
		t.Fatal("sibling field should remain usable after projected consume")
	}
}

func TestSSAFunctionStateRootMoveInvalidatesRootAndFields(t *testing.T) {
	x := ssaStateTestRoot("x")
	y := ssaStateTestRoot("y")
	state := NewSSAFunctionState()

	if violation, ok := state.ConsumeRoot(PlaceKey{Root: x}, SSAMoveSite{Kind: "send"}); ok {
		t.Fatalf("unexpected root consume violation: %#v", violation)
	}

	for _, use := range []PlaceKey{
		{Root: x},
		{Root: x, Path: ".f"},
		{Root: x, Path: ".g.h"},
	} {
		if _, moved := state.CheckUse(use); !moved {
			t.Fatalf("use %#v should be invalid after root move", use)
		}
	}
	if _, moved := state.CheckUse(PlaceKey{Root: y}); moved {
		t.Fatal("move of x should not invalidate y")
	}
}

func TestSSAFunctionStateRejectsRepeatedRootMove(t *testing.T) {
	x := ssaStateTestRoot("x")
	state := NewSSAFunctionState()

	if violation, ok := state.ConsumeRoot(PlaceKey{Root: x}, SSAMoveSite{Kind: "send"}); ok {
		t.Fatalf("unexpected first consume violation: %#v", violation)
	}
	if violation, ok := state.ConsumeRoot(PlaceKey{Root: x}, SSAMoveSite{Kind: "call"}); !ok {
		t.Fatal("expected repeated consume violation")
	} else if violation.Code != GWN001 {
		t.Fatalf("repeated consume code = %s, want %s", violation.Code, GWN001)
	}
}

func TestSSAFunctionStateRepeatedFieldMoveAfterRootMoveIsMovedUse(t *testing.T) {
	x := ssaStateTestRoot("x")
	state := NewSSAFunctionState()

	if violation, ok := state.ConsumeRoot(PlaceKey{Root: x}, SSAMoveSite{Kind: "send"}); ok {
		t.Fatalf("unexpected first consume violation: %#v", violation)
	}
	if violation, ok := state.ConsumeRoot(PlaceKey{Root: x, Path: ".f"}, SSAMoveSite{Kind: "call"}); !ok {
		t.Fatal("expected moved field consume violation")
	} else if violation.Code != GWN001 {
		t.Fatalf("moved field consume code = %s, want %s", violation.Code, GWN001)
	}
}

func TestSSAFunctionStateFieldBorrowPrecision(t *testing.T) {
	x := ssaStateTestRoot("x")
	state := NewSSAFunctionState()

	if violation, ok := state.BeginBorrow(PlaceKey{Root: x, Path: ".f"}, CapMub); ok {
		t.Fatalf("unexpected first field borrow violation: %#v", violation)
	}
	if violation, ok := state.BeginBorrow(PlaceKey{Root: x, Path: ".g"}, CapMub); ok {
		t.Fatalf("sibling field borrow should not conflict: %#v", violation)
	}
	if violation, ok := state.BeginBorrow(PlaceKey{Root: x, Path: ".f"}, CapMub); !ok {
		t.Fatal("expected same-field mutable borrow conflict")
	} else if violation.Code != GWN002 {
		t.Fatalf("same-field borrow code = %s, want %s", violation.Code, GWN002)
	}
}

func TestSSAFunctionStateReadBorrowsCanCoexist(t *testing.T) {
	x := ssaStateTestRoot("x")
	state := NewSSAFunctionState()

	if violation, ok := state.BeginBorrow(PlaceKey{Root: x, Path: ".f"}, CapRob); ok {
		t.Fatalf("unexpected first read borrow violation: %#v", violation)
	}
	if violation, ok := state.BeginBorrow(PlaceKey{Root: x, Path: ".f"}, CapRob); ok {
		t.Fatalf("read borrows should coexist: %#v", violation)
	}
	if violation, ok := state.BeginBorrow(PlaceKey{Root: x, Path: ".f"}, CapMub); !ok {
		t.Fatal("expected mutable borrow to conflict with overlapping read borrows")
	} else if violation.Code != GWN002 {
		t.Fatalf("read/mutable conflict code = %s, want %s", violation.Code, GWN002)
	}
}

func TestSSAFunctionStateBorrowBlocksRootMoveUntilEnded(t *testing.T) {
	x := ssaStateTestRoot("x")
	state := NewSSAFunctionState()

	if violation, ok := state.BeginBorrow(PlaceKey{Root: x, Path: ".f"}, CapMub); ok {
		t.Fatalf("unexpected field borrow violation: %#v", violation)
	}
	if violation, ok := state.ConsumeRoot(PlaceKey{Root: x}, SSAMoveSite{Kind: "send"}); !ok {
		t.Fatal("expected root move to conflict with active field borrow")
	} else if violation.Code != GWN002 {
		t.Fatalf("root move conflict code = %s, want %s", violation.Code, GWN002)
	}

	state.EndBorrow(PlaceKey{Root: x, Path: ".f"}, CapMub)
	if violation, ok := state.ConsumeRoot(PlaceKey{Root: x}, SSAMoveSite{Kind: "send"}); ok {
		t.Fatalf("ended borrow should not block root move: %#v", violation)
	}
}

func TestMergeSSAFunctionStatesTreatsAnyBranchMoveAsConsumed(t *testing.T) {
	x := ssaStateTestRoot("x")
	left := NewSSAFunctionState()
	right := NewSSAFunctionState()

	if violation, ok := left.ConsumeRoot(PlaceKey{Root: x}, SSAMoveSite{Kind: "send"}); ok {
		t.Fatalf("unexpected consume violation: %#v", violation)
	}

	merged, violations := MergeSSAFunctionStates(left, right)
	if len(violations) != 0 {
		t.Fatalf("unexpected merge violations: %#v", violations)
	}
	if _, moved := merged.CheckUse(PlaceKey{Root: x}); !moved {
		t.Fatal("post-merge use should be invalid when one branch consumed x")
	}

	unmoved, violations := MergeSSAFunctionStates(NewSSAFunctionState(), NewSSAFunctionState())
	if len(violations) != 0 {
		t.Fatalf("unexpected empty merge violations: %#v", violations)
	}
	if _, moved := unmoved.CheckUse(PlaceKey{Root: x}); moved {
		t.Fatal("post-merge use should be valid when no branch consumed x")
	}
}

func TestMergeSSAFunctionStatesUnionsBorrowsConservatively(t *testing.T) {
	x := ssaStateTestRoot("x")
	left := NewSSAFunctionState()
	right := NewSSAFunctionState()

	if violation, ok := left.BeginBorrow(PlaceKey{Root: x, Path: ".f"}, CapRob); ok {
		t.Fatalf("unexpected left borrow violation: %#v", violation)
	}
	if violation, ok := right.BeginBorrow(PlaceKey{Root: x, Path: ".g"}, CapRob); ok {
		t.Fatalf("unexpected right borrow violation: %#v", violation)
	}

	merged, violations := MergeSSAFunctionStates(left, right)
	if len(violations) != 0 {
		t.Fatalf("unexpected sibling read merge violations: %#v", violations)
	}
	if got := len(merged.Borrows); got != 2 {
		t.Fatalf("merged borrow count = %d, want 2", got)
	}

	conflicting := NewSSAFunctionState()
	if violation, ok := conflicting.BeginBorrow(PlaceKey{Root: x, Path: ".f"}, CapMub); ok {
		t.Fatalf("unexpected conflicting borrow setup violation: %#v", violation)
	}
	_, violations = MergeSSAFunctionStates(left, conflicting)
	if len(violations) != 1 || violations[0].Code != GWN002 {
		t.Fatalf("merge conflict violations = %#v, want one %s", violations, GWN002)
	}
}

func ssaStateTestRoot(name string) *types.Var {
	return types.NewVar(token.NoPos, nil, name, types.Typ[types.Int])
}
