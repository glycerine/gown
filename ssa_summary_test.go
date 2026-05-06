package gown

import "testing"

const gownSummaryIsoParamReturnedSource = `package example

type payload struct {
	Data string
}

func Identity(x \iso *payload) \iso *payload {
	return x
}

func main() {
	var x \iso *payload
	y := Identity(x)
	println(x)
	_ = y
}
`

const gownSummaryBorrowDoesNotEscapeSource = `package example

type payload struct {
	Data string
}

func Touch(x \mub *payload) {}

func main(ch chan \iso *payload) {
	var x \iso *payload
	Touch(x)
	ch <- x
}
`

const gownSummaryBorrowStoreRejectsSource = `package example

type payload struct {
	Data string
}

var sink *payload

func Escape(x \mub *payload) {
	sink = x
}

func main() {
	var x \iso *payload
	Escape(x)
}
`

const gownSummaryReturnedClosureCapturesBorrowSource = `package example

type payload struct {
	Data string
}

func Escape(x \mub *payload) func() {
	return func() {
		println(x)
	}
}
`

const gownSummaryRecursiveBorrowSource = `package example

type payload struct {
	Data string
}

func Recurse(n int, x \mub *payload) {
	if n > 0 {
		Recurse(n-1, x)
	}
}

func main(ch chan \iso *payload) {
	var x \iso *payload
	Recurse(1, x)
	ch <- x
}
`

func TestSSASummaryIsoParamReturnedAsIsoConsumesAtCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "summary_iso_return.gown", gownSummaryIsoParamReturnedSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestSSASummaryBorrowDoesNotEscapeForSynchronousCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "summary_borrow_sync.gown", gownSummaryBorrowDoesNotEscapeSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected non-escaping borrow call: %#v", errs)
	}
}

func TestSSASummaryBorrowStoreRejectsAtCallSite(t *testing.T) {
	gp := loadGownForSSACheck(t, "summary_borrow_store.gown", gownSummaryBorrowStoreRejectsSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN006)
}

func TestSSASummaryClosureReturnCapturesBorrowRejectsAtCallSite(t *testing.T) {
	gp := loadGownForSSACheck(t, "summary_closure_return.gown", gownSummaryReturnedClosureCapturesBorrowSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN007)
}

func TestSSASummaryRecursiveFunctionConservative(t *testing.T) {
	gp := loadGownForSSACheck(t, "summary_recursive.gown", gownSummaryRecursiveBorrowSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected recursive non-escaping borrow: %#v", errs)
	}
}
