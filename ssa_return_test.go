package gown

import "testing"

func TestSSAGWN007RejectsReturningMutableBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_mub.gown", gownReturnMubSource)

	errs := checkReturnBorrowEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN007)
}

func TestSSAGWN007RejectsReturningReadBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_rob.gown", gownReturnRobSource)

	errs := checkReturnBorrowEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN007)
}

func TestSSAGWN007AllowsReturningIso(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_iso.gown", gownReturnIsoSource)

	errs := checkReturnBorrowEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA return checker unexpectedly rejected iso return: %#v", errs)
	}
}
