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

const gownDeferredClosureIsoCallAfterSendSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	defer func() {
		Take(a)
	}()
	ch := make(chan \iso *payload)
	ch <- a
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

const gownDeferredClosureIsoCallAllowsUseBeforeExitSource = `package example

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

const gownDeferredClosureBorrowAllowsUseBeforeExitSource = `package example

type payload struct {
	Data string
}

func Mut(x \mub *payload) {}

func main() {
	var a \iso *payload
	defer func() {
		Mut(a)
	}()
	println(a)
}
`

const gownPlainDeferredIsoCallStillMovesImmediatelySource = `package example

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

const gownDeferredClosureReadThenTakeLIFOAllowedSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}
func Read(x \rob *payload) {}

func main() {
	var a \iso *payload
	defer func() {
		Take(a)
	}()
	defer func() {
		Read(a)
	}()
}
`

const gownDeferredClosureTakeThenReadLIFORejectsSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}
func Read(x \rob *payload) {}

func main() {
	var a \iso *payload
	defer func() {
		Read(a)
	}()
	defer func() {
		Take(a)
	}()
}
`

const gownDeferredClosureIsoCallReturnSameValueRejectsSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func Make(a \iso *payload) \iso *payload {
	defer func() {
		Take(a)
	}()
	return a
}
`

const gownDeferredClosureIsoCallReturnUnrelatedValueAllowsSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func Make(a \iso *payload, b \iso *payload) \iso *payload {
	defer func() {
		Take(a)
	}()
	return b
}
`

func TestSSAGWN001RejectsSendBeforeDeferredClosureInferredBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_inferred_borrow.gown", gownDeferredClosureInferredBorrowBlocksSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsSendBeforeDeferredClosureInferredBorrow(t *testing.T) {
	err := checkGownSource(t, "defer_closure_inferred_borrow.gown", gownDeferredClosureInferredBorrowBlocksSendSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN001RejectsSendBeforeDeferredClosureIsoCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_iso_call_after_send.gown", gownDeferredClosureIsoCallAfterSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsSendBeforeDeferredClosureIsoCall(t *testing.T) {
	err := checkGownSource(t, "defer_closure_iso_call_after_send.gown", gownDeferredClosureIsoCallAfterSendSource)
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

func TestSSAGWN001AllowsUseBeforeDeferredClosureIsoCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_iso_use_before_exit.gown", gownDeferredClosureIsoCallAllowsUseBeforeExitSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected use before deferred closure exit effect: %#v", errs)
	}
}

func TestGWN001AllowsUseBeforeDeferredClosureIsoCall(t *testing.T) {
	err := checkGownSource(t, "defer_closure_iso_use_before_exit.gown", gownDeferredClosureIsoCallAllowsUseBeforeExitSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN002AllowsUseBeforeDeferredClosureBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_borrow_use_before_exit.gown", gownDeferredClosureBorrowAllowsUseBeforeExitSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected use before deferred closure borrow: %#v", errs)
	}
}

func TestGWN002AllowsUseBeforeDeferredClosureBorrow(t *testing.T) {
	err := checkGownSource(t, "defer_closure_borrow_use_before_exit.gown", gownDeferredClosureBorrowAllowsUseBeforeExitSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN001PlainDeferredIsoCallStillMovesImmediately(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_plain_iso_use_after.gown", gownPlainDeferredIsoCallStillMovesImmediatelySource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001PlainDeferredIsoCallStillMovesImmediately(t *testing.T) {
	err := checkGownSource(t, "defer_plain_iso_use_after.gown", gownPlainDeferredIsoCallStillMovesImmediatelySource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN001AllowsDeferredClosureReadBeforeTakeLIFO(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_read_take_lifo_allowed.gown", gownDeferredClosureReadThenTakeLIFOAllowedSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected deferred closure LIFO read before take: %#v", errs)
	}
}

func TestGWN001AllowsDeferredClosureReadBeforeTakeLIFO(t *testing.T) {
	err := checkGownSource(t, "defer_closure_read_take_lifo_allowed.gown", gownDeferredClosureReadThenTakeLIFOAllowedSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN001RejectsDeferredClosureTakeBeforeReadLIFO(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_take_read_lifo_reject.gown", gownDeferredClosureTakeThenReadLIFORejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsDeferredClosureTakeBeforeReadLIFO(t *testing.T) {
	err := checkGownSource(t, "defer_closure_take_read_lifo_reject.gown", gownDeferredClosureTakeThenReadLIFORejectsSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN001RejectsReturnValueUsedByDeferredClosure(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_return_same.gown", gownDeferredClosureIsoCallReturnSameValueRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsReturnValueUsedByDeferredClosure(t *testing.T) {
	err := checkGownSource(t, "defer_closure_return_same.gown", gownDeferredClosureIsoCallReturnSameValueRejectsSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN001AllowsReturnUnrelatedValueWithDeferredClosure(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_return_unrelated.gown", gownDeferredClosureIsoCallReturnUnrelatedValueAllowsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected unrelated return with deferred closure: %#v", errs)
	}
}

func TestGWN001AllowsReturnUnrelatedValueWithDeferredClosure(t *testing.T) {
	err := checkGownSource(t, "defer_closure_return_unrelated.gown", gownDeferredClosureIsoCallReturnUnrelatedValueAllowsSource)
	if err != nil {
		t.Fatal(err)
	}
}
