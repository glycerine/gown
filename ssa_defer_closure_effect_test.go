package gown

import "testing"

const gownDeferredClosureInferredBorrowBlocksSendSource = `package example

type payload struct {
	Data string
}

func Mut(x \mub *payload) {}

func main() {
	var a \iso *payload
	defer func() {
		Mut(a)
	}()
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownDeferredClosureIsoCallUseAfterSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	defer func() {
		Take(a)
	}()
	println(a)
}
`

const gownDeferredClosureIsoFieldMoveSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item \iso *payload
}

func Take(x \iso *payload) {}

func main(h *holder) {
	defer func() {
		Take(h.Item)
	}()
}
`

func TestSSAGWN002RejectsSendAfterDeferredClosureInferredBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_inferred_borrow.gown", gownDeferredClosureInferredBorrowBlocksSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestGWN002RejectsSendAfterDeferredClosureInferredBorrow(t *testing.T) {
	err := checkGownSource(t, "defer_closure_inferred_borrow.gown", gownDeferredClosureInferredBorrowBlocksSendSource)
	requireCheckerCode(t, err, GWN002)
}

func TestSSAGWN001ReportsDeferredClosureIsoCallMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_iso_call.gown", gownDeferredClosureIsoCallUseAfterSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001ReportsDeferredClosureIsoCallMove(t *testing.T) {
	err := checkGownSource(t, "defer_closure_iso_call.gown", gownDeferredClosureIsoCallUseAfterSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN011RejectsDeferredClosureFieldProjectionMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_field_move.gown", gownDeferredClosureIsoFieldMoveSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN011)
}

func TestGWN011RejectsDeferredClosureFieldProjectionMove(t *testing.T) {
	err := checkGownSource(t, "defer_closure_field_move.gown", gownDeferredClosureIsoFieldMoveSource)
	requireCheckerCode(t, err, GWN011)
}
