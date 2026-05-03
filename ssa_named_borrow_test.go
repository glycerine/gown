package gown

import (
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
)

const gownNamedMutableBorrowLiveAtSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	var b \mub *payload = a
	ch := make(chan \iso *payload)
	ch <- a
	println(b)
}
`

const gownNamedReadBorrowLiveAtSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	var b \rob *payload = a
	ch := make(chan \iso *payload)
	ch <- a
	println(b)
}
`

const gownNamedMutableBorrowDeadBeforeSendSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \iso *payload
	var b \mub *payload = a
	println(b)
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownNamedFieldBorrowLiveAtRootSendSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item *payload
}

func main() {
	var h \iso *holder
	var b \mub *payload = h.Item
	ch := make(chan \iso *holder)
	ch <- h
	println(b)
}
`

const gownNamedMutableBorrowLiveAtIsoCallSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	var b \mub *payload = a
	Take(a)
	println(b)
}
`

const gownNamedMutableBorrowDeadBeforeIsoCallSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	var b \mub *payload = a
	println(b)
	Take(a)
}
`

const gownNamedMutableBorrowLiveAtIsoGoCallSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	var b \mub *payload = a
	go Take(a)
	println(b)
}
`

const gownNamedMutableBorrowDeadBeforeIsoGoCallSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	var b \mub *payload = a
	println(b)
	go Take(a)
}
`

const gownNamedBorrowLiveAcrossBranchSendSource = `package example

type payload struct {
	Data string
}

func main(cond bool) {
	var a \iso *payload
	var b \mub *payload = a
	ch := make(chan \iso *payload)
	if cond {
		ch <- a
	}
	println(b)
}
`

const gownNamedBorrowDeadBeforeBranchJoinSendSource = `package example

type payload struct {
	Data string
}

func main(cond bool) {
	var a \iso *payload
	var b \mub *payload = a
	if cond {
		println(b)
	}
	ch := make(chan \iso *payload)
	ch <- a
}
`

const gownNamedBorrowAssignedInBranchLiveAtSendSource = `package example

type payload struct {
	Data string
}

func main(cond bool) {
	var a \iso *payload
	var b \mub *payload
	if cond {
		b = a
	}
	ch := make(chan \iso *payload)
	ch <- a
	println(b)
}
`

func TestSSAGWN002RejectsIsoSendWhileNamedBorrowLive(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_live.gown", gownNamedMutableBorrowLiveAtSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002RejectsIsoSendWhileNamedReadBorrowLive(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_read_borrow_live.gown", gownNamedReadBorrowLiveAtSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestGWN002RejectsIsoSendWhileNamedBorrowLive(t *testing.T) {
	err := checkGownSource(t, "named_borrow_live.gown", gownNamedMutableBorrowLiveAtSendSource)
	requireCheckerCode(t, err, GWN002)
}

func TestSSAGWN002AllowsIsoSendAfterNamedBorrowDead(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_dead.gown", gownNamedMutableBorrowDeadBeforeSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected send after dead named borrow: %#v", errs)
	}
}

func TestSSAGWN002RejectsRootSendWhileNamedFieldBorrowLive(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_field_borrow_live.gown", gownNamedFieldBorrowLiveAtRootSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002RejectsBranchIsoSendWhileNamedBorrowLive(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_branch_live.gown", gownNamedBorrowLiveAcrossBranchSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002AllowsBranchJoinSendAfterNamedBorrowDead(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_branch_dead.gown", gownNamedBorrowDeadBeforeBranchJoinSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected branch send after dead named borrow: %#v", errs)
	}
}

func TestSSAGWN002RejectsSendAfterBranchAssignedNamedBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_branch_assign.gown", gownNamedBorrowAssignedInBranchLiveAtSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002RejectsIsoCallWhileNamedBorrowLive(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_call_live.gown", gownNamedMutableBorrowLiveAtIsoCallSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002RejectsIsoGoCallWhileNamedBorrowLive(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_go_live.gown", gownNamedMutableBorrowLiveAtIsoGoCallSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002AllowsIsoGoCallAfterNamedBorrowDead(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_go_dead.gown", gownNamedMutableBorrowDeadBeforeIsoGoCallSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected go call after dead named borrow: %#v", errs)
	}
}

func TestSSAGWN002AllowsIsoCallAfterNamedBorrowDead(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_call_dead.gown", gownNamedMutableBorrowDeadBeforeIsoCallSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected call after dead named borrow: %#v", errs)
	}
}

func TestSSANamedBorrowLivenessMarksBorrowLiveAfterSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_live.gown", gownNamedMutableBorrowLiveAtSendSource)
	b := lookupLocalVar(t, gp, "main", "b")

	fn := lookupSSAFunction(t, gp, "main")
	info := collectSSANamedBorrows(gp.pkg, gp.caps)[fn.Object().(*types.Func)]
	if _, ok := info.Borrows[b]; !ok {
		t.Fatalf("named borrow b was not collected: %#v", info.Borrows)
	}
	live := buildSSANamedBorrowLiveness(fn, gp.caps, info)
	send := firstSSAInstructionOfType[*ssa.Send](t, fn)
	if !live.LiveAfterInstruction(send)[b] {
		t.Fatalf("borrow b is not live after send")
	}
}

func TestSSACallBindingMatchesNamedBorrowIsoCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "named_borrow_call_live.gown", gownNamedMutableBorrowLiveAtIsoCallSource)
	fn := lookupSSAFunction(t, gp, "main")
	call := firstSSAInstructionOfType[*ssa.Call](t, fn)

	binding, ok := NewSSABindingIndex(gp.caps).Call(gp.pkg, call)
	if !ok {
		t.Fatalf("SSA call at %#v did not match call bindings %#v", gp.pkg.Fset.Position(call.Pos()), gp.caps.CallBindings)
	}
	if len(binding.ParamCaps) != 1 || binding.ParamCaps[0] != CapIso {
		t.Fatalf("call binding param caps = %#v, want one %s", binding.ParamCaps, CapIso)
	}
}

func lookupSSAFunction(t *testing.T, gp *GownPackage, name string) *ssa.Function {
	t.Helper()
	for _, fn := range collectSSAFunctions(gp.ssaPkg) {
		if fn.Name() == name {
			return fn
		}
	}
	t.Fatalf("could not find SSA function %s", name)
	return nil
}

func firstSSAInstructionOfType[T ssa.Instruction](t *testing.T, fn *ssa.Function) T {
	t.Helper()
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if typed, ok := instr.(T); ok {
				return typed
			}
		}
	}
	var zero T
	t.Fatalf("could not find SSA instruction of requested type in %s", fn.Name())
	return zero
}
