package gown

import "testing"

const gownReturnedClosureCapturesMutableBorrowSource = `package example

type payload struct {
	Data string
}

func Make(a \iso *payload) func() {
	var b \mub *payload = a
	return func() {
		println(b)
	}
}
`

const gownReturnedClosureCapturesReadBorrowSource = `package example

type payload struct {
	Data string
}

func Make(a \iso *payload) func() {
	var b \rob *payload = a
	return func() {
		println(b)
	}
}
`

const gownStoredClosureCapturesMutableBorrowSource = `package example

type payload struct {
	Data string
}

var saved func()

func Save(a \iso *payload) {
	var b \mub *payload = a
	saved = func() {
		println(b)
	}
}
`

const gownLocalClosureCapturesMutableBorrowSource = `package example

type payload struct {
	Data string
}

func Run(a \iso *payload) {
	var b \mub *payload = a
	use := func() {
		println(b)
	}
	use()
}
`

const gownClosureCapturingMutableBorrowPassedToUntrackedCallSource = `package example

type payload struct {
	Data string
}

func Accept(fn func()) {}

func Save(a \iso *payload) {
	var b \mub *payload = a
	Accept(func() {
		println(b)
	})
}
`

const gownReturnedClosureCapturesIsoSource = `package example

type payload struct {
	Data string
}

func Make() func() {
	var a \iso *payload
	return func() {
		println(a)
	}
}
`

const gownStoredClosureCapturesIsoSource = `package example

type payload struct {
	Data string
}

var saved func()

func Save() {
	var a \iso *payload
	saved = func() {
		println(a)
	}
}
`

const gownLocalClosureCapturesIsoSource = `package example

type payload struct {
	Data string
}

func Run() {
	var a \iso *payload
	use := func() {
		println(a)
	}
	use()
}
`

const gownClosureCapturingIsoPassedToUntrackedCallSource = `package example

type payload struct {
	Data string
}

func Accept(fn func()) {}

func Save() {
	var a \iso *payload
	Accept(func() {
		println(a)
	})
}
`

const gownReturnedLocalClosureCapturesMutableBorrowSource = `package example

type payload struct {
	Data string
}

func Make(a \iso *payload) func() {
	var b \mub *payload = a
	fn := func() {
		println(b)
	}
	return fn
}
`

const gownStoredLocalClosureCapturesMutableBorrowSource = `package example

type payload struct {
	Data string
}

var saved func()

func Save(a \iso *payload) {
	var b \mub *payload = a
	fn := func() {
		println(b)
	}
	saved = fn
}
`

const gownLocalClosureCapturingMutableBorrowPassedToUntrackedCallSource = `package example

type payload struct {
	Data string
}

func Accept(fn func()) {}

func Save(a \iso *payload) {
	var b \mub *payload = a
	fn := func() {
		println(b)
	}
	Accept(fn)
}
`

func TestSSAGWN007RejectsReturnedClosureCapturingMutableBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_closure_mub.gown", gownReturnedClosureCapturesMutableBorrowSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN007)
}

func TestGWN007RejectsReturnedClosureCapturingMutableBorrow(t *testing.T) {
	err := checkGownSource(t, "return_closure_mub.gown", gownReturnedClosureCapturesMutableBorrowSource)
	requireCheckerCode(t, err, GWN007)
}

func TestSSAGWN007RejectsReturnedClosureCapturingReadBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_closure_rob.gown", gownReturnedClosureCapturesReadBorrowSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN007)
}

func TestGWN007RejectsReturnedClosureCapturingReadBorrow(t *testing.T) {
	err := checkGownSource(t, "return_closure_rob.gown", gownReturnedClosureCapturesReadBorrowSource)
	requireCheckerCode(t, err, GWN007)
}

func TestSSAGWN006RejectsStoredClosureCapturingMutableBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "store_closure_mub.gown", gownStoredClosureCapturesMutableBorrowSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN006)
}

func TestGWN006RejectsStoredClosureCapturingMutableBorrow(t *testing.T) {
	err := checkGownSource(t, "store_closure_mub.gown", gownStoredClosureCapturesMutableBorrowSource)
	requireCheckerCode(t, err, GWN006)
}

func TestSSAGWN007AllowsLocalClosureCapturingMutableBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "local_closure_mub.gown", gownLocalClosureCapturesMutableBorrowSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA closure escape checker unexpectedly rejected local closure: %#v", errs)
	}
}

func TestSSAGWN008RejectsClosureCapturingBorrowPassedToUntrackedCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "call_closure_mub.gown", gownClosureCapturingMutableBorrowPassedToUntrackedCallSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN008RejectsClosureCapturingBorrowPassedToUntrackedCall(t *testing.T) {
	err := checkGownSource(t, "call_closure_mub.gown", gownClosureCapturingMutableBorrowPassedToUntrackedCallSource)
	requireCheckerCode(t, err, GWN008)
}

func TestSSAGWN007RejectsReturnedClosureCapturingIso(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_closure_iso.gown", gownReturnedClosureCapturesIsoSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN007)
}

func TestGWN007RejectsReturnedClosureCapturingIso(t *testing.T) {
	err := checkGownSource(t, "return_closure_iso.gown", gownReturnedClosureCapturesIsoSource)
	requireCheckerCode(t, err, GWN007)
}

func TestSSAGWN006RejectsStoredClosureCapturingIso(t *testing.T) {
	gp := loadGownForSSACheck(t, "store_closure_iso.gown", gownStoredClosureCapturesIsoSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN006)
}

func TestGWN006RejectsStoredClosureCapturingIso(t *testing.T) {
	err := checkGownSource(t, "store_closure_iso.gown", gownStoredClosureCapturesIsoSource)
	requireCheckerCode(t, err, GWN006)
}

func TestSSAGWN007AllowsLocalClosureCapturingIso(t *testing.T) {
	gp := loadGownForSSACheck(t, "local_closure_iso.gown", gownLocalClosureCapturesIsoSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA closure escape checker unexpectedly rejected local iso closure: %#v", errs)
	}
}

func TestSSAGWN008RejectsClosureCapturingIsoPassedToUntrackedCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "call_closure_iso.gown", gownClosureCapturingIsoPassedToUntrackedCallSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN008RejectsClosureCapturingIsoPassedToUntrackedCall(t *testing.T) {
	err := checkGownSource(t, "call_closure_iso.gown", gownClosureCapturingIsoPassedToUntrackedCallSource)
	requireCheckerCode(t, err, GWN008)
}

func TestSSAGWN007RejectsReturnedLocalClosureCapturingMutableBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_local_closure_mub.gown", gownReturnedLocalClosureCapturesMutableBorrowSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN007)
}

func TestGWN007RejectsReturnedLocalClosureCapturingMutableBorrow(t *testing.T) {
	err := checkGownSource(t, "return_local_closure_mub.gown", gownReturnedLocalClosureCapturesMutableBorrowSource)
	requireCheckerCode(t, err, GWN007)
}

func TestSSAGWN006RejectsStoredLocalClosureCapturingMutableBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "store_local_closure_mub.gown", gownStoredLocalClosureCapturesMutableBorrowSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN006)
}

func TestGWN006RejectsStoredLocalClosureCapturingMutableBorrow(t *testing.T) {
	err := checkGownSource(t, "store_local_closure_mub.gown", gownStoredLocalClosureCapturesMutableBorrowSource)
	requireCheckerCode(t, err, GWN006)
}

func TestSSAGWN008RejectsLocalClosureCapturingBorrowPassedToUntrackedCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "call_local_closure_mub.gown", gownLocalClosureCapturingMutableBorrowPassedToUntrackedCallSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN008RejectsLocalClosureCapturingBorrowPassedToUntrackedCall(t *testing.T) {
	err := checkGownSource(t, "call_local_closure_mub.gown", gownLocalClosureCapturingMutableBorrowPassedToUntrackedCallSource)
	requireCheckerCode(t, err, GWN008)
}
