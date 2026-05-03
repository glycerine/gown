package gown

import "testing"

const gownLoopIsoSendUseAfterSource = `package example

type payload struct {
	Data string
}

func main(cond bool) {
	var a \iso *payload
	ch := make(chan \iso *payload)
	for cond {
		ch <- a
	}
	println(a)
}
`

const gownLoopReadBeforeIsoSendSource = `package example

type payload struct {
	Data string
}

func main(cond bool) {
	var a \iso *payload
	for cond {
		println(a)
	}
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownLoopLocalNamedBorrowDiesBeforeSendSource = `package example

type payload struct {
	Data string
}

func main(cond bool) {
	var a \iso *payload
	for cond {
		var b \mub *payload = a
		println(b)
	}
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownLoopAssignedNamedBorrowSurvivesSendSource = `package example

type payload struct {
	Data string
}

func main(cond bool) {
	var a \iso *payload
	var b \mub *payload
	for cond {
		b = a
	}
	ch := make(chan \iso *payload)
	ch <- a
	println(b)
}
`

const gownLoopDeferredClosureIsoMoveRejectsSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main(cond bool) {
	var a \iso *payload
	for cond {
		defer func() {
			Take(a)
		}()
	}
	println(a)
}
`

const gownLoopDeferredClosureRejectsReturnSameValueSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func Make(cond bool, a \iso *payload) \iso *payload {
	for cond {
		defer func() {
			Take(a)
		}()
	}
	return a
}
`

const gownLoopRepeatedDeferredClosureIsoMoveRejectsSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	for i := 0; i < 2; i++ {
		defer func() {
			Take(a)
		}()
	}
}
`

const gownLoopRepeatedDeferredClosureReadsAllowedSource = `package example

type payload struct {
	Data string
}

func Read(x \rob *payload) {}

func main() {
	var a \iso *payload
	for i := 0; i < 2; i++ {
		defer func() {
			Read(a)
		}()
	}
}
`

const gownLoopClosureAliasOverwrittenBeforeReturnSource = `package example

type payload struct {
	Data string
}

func Make(cond bool, a \iso *payload) func() {
	var b \mub *payload = a
	fn := func() {}
	for cond {
		fn = func() {
			println(b)
		}
		fn()
		fn = func() {}
	}
	return fn
}
`

const gownLoopClosureAliasMayEscapeSource = `package example

type payload struct {
	Data string
}

func Make(cond bool, a \iso *payload) func() {
	var b \mub *payload = a
	var fn func()
	for cond {
		fn = func() {
			println(b)
		}
	}
	return fn
}
`

func TestSSAGWN001RejectsUseAfterLoopIsoSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "loop_send_use_after.gown", gownLoopIsoSendUseAfterSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsUseAfterLoopIsoSend(t *testing.T) {
	err := checkGownSource(t, "loop_send_use_after.gown", gownLoopIsoSendUseAfterSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN001AllowsLoopReadsBeforeIsoSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "loop_read_before_send.gown", gownLoopReadBeforeIsoSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected loop reads before send: %#v", errs)
	}
}

func TestGWN001AllowsLoopReadsBeforeIsoSend(t *testing.T) {
	err := checkGownSource(t, "loop_read_before_send.gown", gownLoopReadBeforeIsoSendSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN002AllowsLoopLocalNamedBorrowBeforeSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "loop_local_borrow_send.gown", gownLoopLocalNamedBorrowDiesBeforeSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected loop-local borrow before send: %#v", errs)
	}
}

func TestGWN002AllowsLoopLocalNamedBorrowBeforeSend(t *testing.T) {
	err := checkGownSource(t, "loop_local_borrow_send.gown", gownLoopLocalNamedBorrowDiesBeforeSendSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN002RejectsLoopAssignedNamedBorrowAtSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "loop_assigned_borrow_send.gown", gownLoopAssignedNamedBorrowSurvivesSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestGWN002RejectsLoopAssignedNamedBorrowAtSend(t *testing.T) {
	err := checkGownSource(t, "loop_assigned_borrow_send.gown", gownLoopAssignedNamedBorrowSurvivesSendSource)
	requireCheckerCode(t, err, GWN002)
}

func TestSSAGWN001RejectsLoopDeferredClosureIsoMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "loop_defer_iso_move.gown", gownLoopDeferredClosureIsoMoveRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsLoopDeferredClosureIsoMove(t *testing.T) {
	err := checkGownSource(t, "loop_defer_iso_move.gown", gownLoopDeferredClosureIsoMoveRejectsSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN001RejectsReturnAfterLoopDeferredClosure(t *testing.T) {
	gp := loadGownForSSACheck(t, "loop_defer_return_same.gown", gownLoopDeferredClosureRejectsReturnSameValueSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsReturnAfterLoopDeferredClosure(t *testing.T) {
	err := checkGownSource(t, "loop_defer_return_same.gown", gownLoopDeferredClosureRejectsReturnSameValueSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN001RejectsRepeatedLoopDeferredClosureIsoMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "loop_repeated_defer_iso.gown", gownLoopRepeatedDeferredClosureIsoMoveRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsRepeatedLoopDeferredClosureIsoMove(t *testing.T) {
	err := checkGownSource(t, "loop_repeated_defer_iso.gown", gownLoopRepeatedDeferredClosureIsoMoveRejectsSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN001AllowsRepeatedLoopDeferredClosureReads(t *testing.T) {
	gp := loadGownForSSACheck(t, "loop_repeated_defer_read.gown", gownLoopRepeatedDeferredClosureReadsAllowedSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected repeated deferred reads: %#v", errs)
	}
}

func TestGWN001AllowsRepeatedLoopDeferredClosureReads(t *testing.T) {
	err := checkGownSource(t, "loop_repeated_defer_read.gown", gownLoopRepeatedDeferredClosureReadsAllowedSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN007AllowsLoopClosureAliasOverwrittenBeforeReturn(t *testing.T) {
	gp := loadGownForSSACheck(t, "loop_closure_overwritten.gown", gownLoopClosureAliasOverwrittenBeforeReturnSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA closure escape checker unexpectedly rejected loop overwritten closure: %#v", errs)
	}
}

func TestGWN007AllowsLoopClosureAliasOverwrittenBeforeReturn(t *testing.T) {
	err := checkGownSource(t, "loop_closure_overwritten.gown", gownLoopClosureAliasOverwrittenBeforeReturnSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN007RejectsLoopClosureAliasMayEscape(t *testing.T) {
	gp := loadGownForSSACheck(t, "loop_closure_may_escape.gown", gownLoopClosureAliasMayEscapeSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN007)
}

func TestGWN007RejectsLoopClosureAliasMayEscape(t *testing.T) {
	err := checkGownSource(t, "loop_closure_may_escape.gown", gownLoopClosureAliasMayEscapeSource)
	requireCheckerCode(t, err, GWN007)
}
