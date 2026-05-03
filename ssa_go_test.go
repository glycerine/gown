package gown

import "testing"

func TestSSAGWN004RejectsMutableBorrowGoCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "go_mub.gown", gownGoMubCallSource)

	errs := checkGoBorrowEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN004)
}

func TestSSAGWN004RejectsReadBorrowGoCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "go_rob.gown", gownGoRobCallSource)

	errs := checkGoBorrowEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN004)
}

func TestSSAGWN004AllowsIsoMoveGoCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "go_iso.gown", gownGoIsoCallSource)

	errs := checkGoBorrowEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN004 unexpectedly rejected iso go call: %#v", errs)
	}
}

func TestSSAGWN004RejectsMutableBorrowGoClosureCapture(t *testing.T) {
	gp := loadGownForSSACheck(t, "go_mub_capture.gown", gownGoMubClosureCaptureSource)

	errs := checkGoBorrowEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN004)
}

func TestSSAGWN004RejectsReadBorrowGoClosureCapture(t *testing.T) {
	gp := loadGownForSSACheck(t, "go_rob_capture.gown", gownGoRobClosureCaptureSource)

	errs := checkGoBorrowEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN004)
}
