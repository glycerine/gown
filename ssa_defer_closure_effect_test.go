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

const gownDeferredClosureBranchExclusiveTakeOrReadAllowsSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}
func Read(x \rob *payload) {}

func main(cond bool) {
	var a \iso *payload
	defer func() {
		if cond {
			Take(a)
		} else {
			Read(a)
		}
	}()
}
`

const gownDeferredClosureBranchExclusiveDuplicateTakeAllowsSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main(cond bool) {
	var a \iso *payload
	defer func() {
		if cond {
			Take(a)
		} else {
			Take(a)
		}
	}()
}
`

const gownDeferredClosureReadAfterSendRejectsSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	defer func() {
		println(a)
	}()
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownDeferredClosureBranchExclusiveSendOrReadAllowsSource = `package example

type payload struct {
	Data string
}

func main(cond bool) {
	var a \iso *payload
	ch := make(chan \iso *payload)
	defer func() {
		if cond {
			ch <- a
		} else {
			println(a)
		}
	}()
}
`

const gownDeferredClosureSamePathUseAfterTakeRejectsSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}
func Read(x \rob *payload) {}

func main(cond bool) {
	var a \iso *payload
	defer func() {
		if cond {
			Take(a)
			Read(a)
		}
	}()
}
`

const gownDeferredClosurePostMergeUseAfterPossibleTakeRejectsSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}
func Read(x \rob *payload) {}

func main(cond bool) {
	var a \iso *payload
	defer func() {
		if cond {
			Take(a)
		}
		Read(a)
	}()
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

func TestSSAGWN001AllowsDeferredClosureBranchExclusiveTakeOrRead(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_branch_take_read.gown", gownDeferredClosureBranchExclusiveTakeOrReadAllowsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected branch-exclusive deferred closure effects: %#v", errs)
	}
}

func TestGWN001AllowsDeferredClosureBranchExclusiveTakeOrRead(t *testing.T) {
	err := checkGownSource(t, "defer_closure_branch_take_read.gown", gownDeferredClosureBranchExclusiveTakeOrReadAllowsSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN001AllowsDeferredClosureBranchExclusiveDuplicateTake(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_branch_duplicate_take.gown", gownDeferredClosureBranchExclusiveDuplicateTakeAllowsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected mutually exclusive deferred moves: %#v", errs)
	}
}

func TestGWN001AllowsDeferredClosureBranchExclusiveDuplicateTake(t *testing.T) {
	err := checkGownSource(t, "defer_closure_branch_duplicate_take.gown", gownDeferredClosureBranchExclusiveDuplicateTakeAllowsSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN001RejectsDeferredClosureReadAfterSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_read_after_send.gown", gownDeferredClosureReadAfterSendRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsDeferredClosureReadAfterSend(t *testing.T) {
	err := checkGownSource(t, "defer_closure_read_after_send.gown", gownDeferredClosureReadAfterSendRejectsSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN001AllowsDeferredClosureBranchExclusiveSendOrRead(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_branch_send_read.gown", gownDeferredClosureBranchExclusiveSendOrReadAllowsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected branch-exclusive deferred send/read: %#v", errs)
	}
}

func TestGWN001AllowsDeferredClosureBranchExclusiveSendOrRead(t *testing.T) {
	err := checkGownSource(t, "defer_closure_branch_send_read.gown", gownDeferredClosureBranchExclusiveSendOrReadAllowsSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN001RejectsDeferredClosureSamePathUseAfterTake(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_same_path_take_read.gown", gownDeferredClosureSamePathUseAfterTakeRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsDeferredClosureSamePathUseAfterTake(t *testing.T) {
	err := checkGownSource(t, "defer_closure_same_path_take_read.gown", gownDeferredClosureSamePathUseAfterTakeRejectsSource)
	requireCheckerCode(t, err, GWN001)
}

func TestSSAGWN001RejectsDeferredClosurePostMergeUseAfterPossibleTake(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure_post_merge_take_read.gown", gownDeferredClosurePostMergeUseAfterPossibleTakeRejectsSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestGWN001RejectsDeferredClosurePostMergeUseAfterPossibleTake(t *testing.T) {
	err := checkGownSource(t, "defer_closure_post_merge_take_read.gown", gownDeferredClosurePostMergeUseAfterPossibleTakeRejectsSource)
	requireCheckerCode(t, err, GWN001)
}
