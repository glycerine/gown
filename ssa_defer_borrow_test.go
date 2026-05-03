package gown

import "testing"

const gownDeferNamedBorrowArgBlocksSendSource = `package example

type payload struct {
	Data string
}

func Use(x \mub *payload) {}

func main() {
	var a \iso *payload
	var b \mub *payload = a
	defer Use(b)
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownDeferNamedBorrowArgReassignBlocksSendSource = `package example

type payload struct {
	Data string
}

func Use(x \mub *payload) {}

func main() {
	var a \iso *payload
	var b \mub *payload = a
	defer Use(b)
	b = nil
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownDeferClosureNamedBorrowCaptureBlocksSendSource = `package example

type payload struct {
	Data string
}

func Use(x \mub *payload) {}

func main() {
	var a \iso *payload
	var b \mub *payload = a
	defer func() {
		Use(b)
	}()
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownUnrelatedDeferAllowsSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	x := "later"
	defer println(x)
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownDeferredInferredMutableBorrowBlocksSendSource = `package example

type payload struct {
	Data string
}

func Mut(x \mub *payload) {}

func main() {
	var a \iso *payload
	defer Mut(a)
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownDeferredInferredReadBorrowBlocksSendSource = `package example

type payload struct {
	Data string
}

func Read(x \rob *payload) {}

func main() {
	var a \iso *payload
	defer Read(a)
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownDeferredInferredBorrowConflictSource = `package example

type payload struct {
	Data string
}

func Mut(x \mub *payload) {}
func Read(x \rob *payload) {}

func main() {
	var a \iso *payload
	defer Mut(a)
	defer Read(a)
}
`

func TestSSAGWN002RejectsSendAfterDeferredNamedBorrowArg(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_arg.gown", gownDeferNamedBorrowArgBlocksSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002RejectsSendAfterDeferredNamedBorrowArgEvenAfterReassign(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_arg_reassign.gown", gownDeferNamedBorrowArgReassignBlocksSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002RejectsSendAfterDeferredNamedBorrowClosureCapture(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_closure.gown", gownDeferClosureNamedBorrowCaptureBlocksSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002AllowsSendAfterUnrelatedDefer(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_unrelated.gown", gownUnrelatedDeferAllowsSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected unrelated defer: %#v", errs)
	}
}

func TestGWN002RejectsSendAfterDeferredNamedBorrowArg(t *testing.T) {
	err := checkGownSource(t, "defer_arg.gown", gownDeferNamedBorrowArgBlocksSendSource)
	requireCheckerCode(t, err, GWN002)
}

func TestSSAGWN002RejectsSendAfterDeferredInferredMutableBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_inferred_mut.gown", gownDeferredInferredMutableBorrowBlocksSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002RejectsSendAfterDeferredInferredReadBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_inferred_read.gown", gownDeferredInferredReadBorrowBlocksSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002RejectsConflictingDeferredInferredBorrows(t *testing.T) {
	gp := loadGownForSSACheck(t, "defer_inferred_conflict.gown", gownDeferredInferredBorrowConflictSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestGWN002RejectsSendAfterDeferredInferredMutableBorrow(t *testing.T) {
	err := checkGownSource(t, "defer_inferred_mut.gown", gownDeferredInferredMutableBorrowBlocksSendSource)
	requireCheckerCode(t, err, GWN002)
}

func TestGWN002RejectsSendAfterDeferredInferredReadBorrow(t *testing.T) {
	err := checkGownSource(t, "defer_inferred_read.gown", gownDeferredInferredReadBorrowBlocksSendSource)
	requireCheckerCode(t, err, GWN002)
}

func TestGWN002RejectsConflictingDeferredInferredBorrows(t *testing.T) {
	err := checkGownSource(t, "defer_inferred_conflict.gown", gownDeferredInferredBorrowConflictSource)
	requireCheckerCode(t, err, GWN002)
}
