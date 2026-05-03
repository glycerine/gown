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

const gownReturnedCopiedClosureCapturesMutableBorrowSource = `package example

type payload struct {
	Data string
}

func Make(a \iso *payload) func() {
	var b \mub *payload = a
	fn := func() {
		println(b)
	}
	g := fn
	return g
}
`

const gownReturnedBranchClosureCapturesMutableBorrowSource = `package example

type payload struct {
	Data string
}

func Make(cond bool, a \iso *payload) func() {
	var b \mub *payload = a
	var fn func()
	if cond {
		fn = func() {
			println(b)
		}
	} else {
		fn = func() {}
	}
	return fn
}
`

const gownStoredCopiedClosureCapturesMutableBorrowSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Fn func()
}

func Save(a \iso *payload) {
	var h holder
	var b \mub *payload = a
	fn := func() {
		println(b)
	}
	g := fn
	h.Fn = g
}
`

const gownCopiedClosureCapturingMutableBorrowPassedToUntrackedCallSource = `package example

type payload struct {
	Data string
}

func Accept(fn func()) {}

func Save(a \iso *payload) {
	var b \mub *payload = a
	fn := func() {
		println(b)
	}
	g := fn
	Accept(g)
}
`

const gownOverwrittenClosureCapturingMutableBorrowReturnedSource = `package example

type payload struct {
	Data string
}

func Make(a \iso *payload) func() {
	var b \mub *payload = a
	fn := func() {
		println(b)
	}
	fn = func() {}
	return fn
}
`

const gownOverwrittenClosureCapturingMutableBorrowStoredSource = `package example

type payload struct {
	Data string
}

var saved func()

func Save(a \iso *payload) {
	var b \mub *payload = a
	fn := func() {
		println(b)
	}
	fn = func() {}
	saved = fn
}
`

const gownBranchClosureOverwrittenBeforeReturnSource = `package example

type payload struct {
	Data string
}

func Make(cond bool, a \iso *payload) func() {
	var b \mub *payload = a
	var fn func()
	if cond {
		fn = func() {
			println(b)
		}
		fn()
	} else {
		fn = func() {}
	}
	fn = func() {}
	return fn
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

func TestSSAGWN007RejectsReturnedCopiedClosureCapturingMutableBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_copied_closure_mub.gown", gownReturnedCopiedClosureCapturesMutableBorrowSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN007)
}

func TestGWN007RejectsReturnedCopiedClosureCapturingMutableBorrow(t *testing.T) {
	err := checkGownSource(t, "return_copied_closure_mub.gown", gownReturnedCopiedClosureCapturesMutableBorrowSource)
	requireCheckerCode(t, err, GWN007)
}

func TestSSAGWN007RejectsReturnedBranchClosureCapturingMutableBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_branch_closure_mub.gown", gownReturnedBranchClosureCapturesMutableBorrowSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN007)
}

func TestGWN007RejectsReturnedBranchClosureCapturingMutableBorrow(t *testing.T) {
	err := checkGownSource(t, "return_branch_closure_mub.gown", gownReturnedBranchClosureCapturesMutableBorrowSource)
	requireCheckerCode(t, err, GWN007)
}

func TestSSAGWN006RejectsStoredCopiedClosureCapturingMutableBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "store_copied_closure_mub.gown", gownStoredCopiedClosureCapturesMutableBorrowSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN006)
}

func TestGWN006RejectsStoredCopiedClosureCapturingMutableBorrow(t *testing.T) {
	err := checkGownSource(t, "store_copied_closure_mub.gown", gownStoredCopiedClosureCapturesMutableBorrowSource)
	requireCheckerCode(t, err, GWN006)
}

func TestSSAGWN008RejectsCopiedClosureCapturingBorrowPassedToUntrackedCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "call_copied_closure_mub.gown", gownCopiedClosureCapturingMutableBorrowPassedToUntrackedCallSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN008)
}

func TestGWN008RejectsCopiedClosureCapturingBorrowPassedToUntrackedCall(t *testing.T) {
	err := checkGownSource(t, "call_copied_closure_mub.gown", gownCopiedClosureCapturingMutableBorrowPassedToUntrackedCallSource)
	requireCheckerCode(t, err, GWN008)
}

func TestSSAGWN007AllowsOverwrittenClosureCapturingMutableBorrowReturn(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_overwritten_closure_mub.gown", gownOverwrittenClosureCapturingMutableBorrowReturnedSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA closure escape checker unexpectedly rejected overwritten closure return: %#v", errs)
	}
}

func TestGWN007AllowsOverwrittenClosureCapturingMutableBorrowReturn(t *testing.T) {
	err := checkGownSource(t, "return_overwritten_closure_mub.gown", gownOverwrittenClosureCapturingMutableBorrowReturnedSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN006AllowsOverwrittenClosureCapturingMutableBorrowStore(t *testing.T) {
	gp := loadGownForSSACheck(t, "store_overwritten_closure_mub.gown", gownOverwrittenClosureCapturingMutableBorrowStoredSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA closure escape checker unexpectedly rejected overwritten closure store: %#v", errs)
	}
}

func TestGWN006AllowsOverwrittenClosureCapturingMutableBorrowStore(t *testing.T) {
	err := checkGownSource(t, "store_overwritten_closure_mub.gown", gownOverwrittenClosureCapturingMutableBorrowStoredSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN007AllowsBranchClosureOverwrittenBeforeReturn(t *testing.T) {
	gp := loadGownForSSACheck(t, "return_branch_overwritten_closure_mub.gown", gownBranchClosureOverwrittenBeforeReturnSource)

	errs := checkClosureEscapesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA closure escape checker unexpectedly rejected branch overwritten closure return: %#v", errs)
	}
}

func TestGWN007AllowsBranchClosureOverwrittenBeforeReturn(t *testing.T) {
	err := checkGownSource(t, "return_branch_overwritten_closure_mub.gown", gownBranchClosureOverwrittenBeforeReturnSource)
	if err != nil {
		t.Fatal(err)
	}
}
