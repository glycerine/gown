package gown

import "testing"

const gownReturnIsoDeferredUseSource = `package example

type payload struct {
	Data string
}

func Move(x \iso *payload) \iso *payload {
	defer func() {
		println(x)
	}()
	return x
}
`

const gownReturnIsoLiveBorrowSource = `package example

type payload struct {
	Data string
}

func Move(x \iso *payload) \iso *payload {
	b := \mub(x)
	defer func() {
		println(b)
	}()
	return x
}
`

const gownReturnImmFreezesIsoSource = `package example

type payload struct {
	Data string
}

func Freeze(x \iso *payload) \imm *payload {
	defer func() {
		println(x)
	}()
	return x
}
`

const gownReturnImmSourceAllowed = `package example

type payload struct {
	Data string
}

func Share(y \imm *payload) \imm *payload {
	return y
}
`

const gownReturnImmAfterFrontierSource = `package example

type payload struct {
	Data string
}

func Plain(x *payload) {}

func Freeze(x \iso *payload) \imm *payload {
	Plain(x)
	return x
}
`

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

func TestSSAReturnIsoResultMovesSourceBeforeDeferredUse(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_iso_defer_use.gown", gownReturnIsoDeferredUseSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestSSAReturnIsoResultRejectsLiveNamedBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_iso_live_borrow.gown", gownReturnIsoLiveBorrowSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAReturnImmResultFreezesIso(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_imm_freeze.gown", gownReturnImmFreezesIsoSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestSSAReturnImmResultAllowsImmSource(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_imm_source.gown", gownReturnImmSourceAllowed)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected imm return: %#v", errs)
	}
}

func TestSSAReturnTrackedResultAfterUntrackedCallBoundaryRejected(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_imm_frontier.gown", gownReturnImmAfterFrontierSource)

	errs := checkUntrackedCallBoundariesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}
