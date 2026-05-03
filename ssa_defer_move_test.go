package gown

import "testing"

const gownDeferredIsoCallUseAfterSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	defer Take(a)
	println(a)
}
`

const gownDeferredIsoCallWhileNamedBorrowLiveSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	var b \mub *payload = a
	defer Take(a)
	println(b)
}
`

const gownDeferredIsoFieldMoveSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item \iso *payload
}

func Take(x \iso *payload) {}

func main(h *holder) {
	defer Take(h.Item)
}
`

func TestSSAGWN001ReportsDeferredIsoCallMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_iso_call_use.gown", gownDeferredIsoCallUseAfterSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001ReportsDeferredIsoCallMove(t *testing.T) {
	err := checkGownSource(t, "defer_iso_call_use.gown", gownDeferredIsoCallUseAfterSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN002RejectsDeferredIsoCallWhileNamedBorrowLive(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_iso_call_live_borrow.gown", gownDeferredIsoCallWhileNamedBorrowLiveSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestGWN002RejectsDeferredIsoCallWhileNamedBorrowLive(t *testing.T) {
	err := checkGownSource(t, "defer_iso_call_live_borrow.gown", gownDeferredIsoCallWhileNamedBorrowLiveSource)
	requireCheckerCode(t, err, GWN002)
}

func TestSSAGWN011RejectsDeferredFieldProjectionMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_iso_field_move.gown", gownDeferredIsoFieldMoveSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN011)
}

func TestGWN011RejectsDeferredFieldProjectionMove(t *testing.T) {
	err := checkGownSource(t, "defer_iso_field_move.gown", gownDeferredIsoFieldMoveSource)
	requireCheckerCode(t, err, GWN011)
}
