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
