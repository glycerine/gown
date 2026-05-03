package gown

import "testing"

func TestSSAGWN008RejectsIsoPassedToUntrackedUserCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_plain_call.gown", gownIsoToPlainCallSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestSSAGWN008RejectsBorrowPassedToUntrackedUserCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_plain_call.gown", gownMubToPlainCallSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestSSAGWN008AllowsUntrackedValuePassedToUntrackedUserCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "untracked_plain_call.gown", gownUntrackedToPlainCallSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA untracked-call checker unexpectedly rejected untracked value: %#v", errs)
	}
}

func TestSSAGWN008AllowsTrackedValuePassedToBuiltin(t *testing.T) {
	gp := loadGownForSSACheck(t, "tracked_builtin.gown", gownTrackedToBuiltinSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA untracked-call checker unexpectedly rejected builtin: %#v", errs)
	}
}
