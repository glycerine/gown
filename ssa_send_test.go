package gown

import "testing"

func TestSSAGWN003RejectsMutableBorrowSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_send.gown", gownMubSendSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN003)
}

func TestSSAGWN003RejectsReadBorrowSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "rob_send.gown", gownRobSendSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN003)
}

func TestSSAGWN003AllowsImmutableSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "imm_send.gown", gownImmSendSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA send checker unexpectedly rejected immutable send: %#v", errs)
	}
}

func TestSSAGWN010RejectsUntrackedValueToIsoChannel(t *testing.T) {
	gp := loadGownForSSACheck(t, "untracked_to_iso.gown", gownUntrackedSendToIsoChannelSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN010)
}

func TestSSAInferredFreezeAllowsIsoValueToImmChannel(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_to_imm.gown", gownIsoSendToImmChannelSource)

	errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA send checker unexpectedly rejected inferred freeze send: %#v", errs)
	}
}
