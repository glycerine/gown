package gown

import (
	"errors"
	"strings"
	"testing"
)

const gownSSABranchUseAfterIsoSendSource = `package example

type payload struct {
	Data string
}

func f(cond bool, ch chan \iso *payload, a \iso *payload) {
	if cond {
		ch <- a
	}
	println(a)
}
`

const gownSSAFieldMoveSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item \iso *payload
}

func f(ch chan \iso *payload, h *holder) {
	ch <- h.Item
}
`

const gownSSAGoIsoCallUseAfterSource = `package example

type payload struct {
	Data string
}

func Take(x \iso *payload) {}

func main() {
	var a \iso *payload
	go Take(a)
	println(a)
}
`

const gownSSAExplicitFreezeUseAfterSource = `package example

type payload struct {
	Data string
}

func main() {
	var x \iso *payload
	y := \freeze(x)
	_ = y
	println(x)
}
`

const gownSSAExplicitFreezeResultImmSource = `package example

type payload struct {
	Data string
}

func main() {
	var x \iso *payload
	y := \freeze(x)
	ch := make(chan \imm *payload)
	ch <- y
	println(y)
}
`

const gownSSAExplicitFreezeLiveBorrowSource = `package example

type payload struct {
	Data string
}

func main() {
	var x \iso *payload
	b := \mub(x)
	y := \freeze(x)
	_, _ = b, y
}
`

const gownSSAExplicitFreezeFieldProjectionSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item *payload
}

func main() {
	var h \iso *holder
	y := \freeze(h.Item)
	_ = y
}
`

const gownSSAExplicitFreezeSelfAssignSource = `package example

type payload struct {
	Data string
}

func newPayload() \iso *payload {
	return &payload{}
}

func main() {
	x := newPayload()
	x = \freeze(x)
}
`

const gownSSAExplicitFreezeImmSource = `package example

type payload struct {
	Data string
}

func main(x \imm *payload) {
	y := \freeze(x)
	ch := make(chan \imm *payload, 1)
	ch <- y
	ch <- x
}
`

func TestSSAGWN001ReportsDirectUseAfterIsoSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "use_after_send.gown", gownUseAfterIsoSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
	if errs[0].Line != 11 {
		t.Fatalf("SSA GWN001 line = %d, want 11: %#v", errs[0].Line, errs[0])
	}
	if !strings.HasSuffix(errs[0].Path, ".gown") {
		t.Fatalf("SSA GWN001 path = %q, want .gown", errs[0].Path)
	}
}

func TestSSAGWN001AllowsUseBeforeIsoSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "use_before_send.gown", gownUseBeforeIsoSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly reported errors: %#v", errs)
	}
}

func TestSSAGWN001ReportsBranchUseAfterIsoSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "branch_use_after_send.gown", gownSSABranchUseAfterIsoSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
	if errs[0].Line != 11 {
		t.Fatalf("SSA branch GWN001 line = %d, want 11: %#v", errs[0].Line, errs[0])
	}
}

func TestSSAGWN001ReportsIsoCallMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "use_after_call.gown", gownUseAfterIsoCallSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
	if errs[0].Line != 12 {
		t.Fatalf("SSA call GWN001 line = %d, want 12: %#v", errs[0].Line, errs[0])
	}
}

func TestSSAGWN001ReportsAssignmentMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "use_after_assign.gown", gownUseAfterIsoAssignSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
	if errs[0].Line != 11 {
		t.Fatalf("SSA assignment GWN001 line = %d, want 11: %#v", errs[0].Line, errs[0])
	}
}

func TestSSAGWN001MovesFreshIsoThroughAssignmentBeforeSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "fresh_assign_send.gown", gownFreshIsoAssignThenSendSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
	if errs[0].Line != 12 {
		t.Fatalf("SSA fresh assignment GWN001 line = %d, want 12: %#v", errs[0].Line, errs[0])
	}
}

func TestSSAGWN001RejectsFieldMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "field_move.gown", gownSSAFieldMoveSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN011)
	if errs[0].Line != 12 {
		t.Fatalf("SSA field move line = %d, want 12: %#v", errs[0].Line, errs[0])
	}
}

func TestSSAGWN001ReportsGoIsoCallMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "go_iso_call_use.gown", gownSSAGoIsoCallUseAfterSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
	if errs[0].Line != 12 {
		t.Fatalf("SSA go call GWN001 line = %d, want 12: %#v", errs[0].Line, errs[0])
	}
}

func TestSSAGWN001ReportsGoClosureCaptureMove(t *testing.T) {
	gp := loadGownForSSACheck(t, "go_iso_capture_use.gown", gownGoIsoClosureCaptureUseAfterSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
	if errs[0].Line != 12 {
		t.Fatalf("SSA go capture GWN001 line = %d, want 12: %#v", errs[0].Line, errs[0])
	}
}

func TestSSAGWN001AllowsGoClosureCaptureWithoutLaterUse(t *testing.T) {
	gp := loadGownForSSACheck(t, "go_iso_capture.gown", gownGoIsoClosureCaptureSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly reported go capture errors: %#v", errs)
	}
}

func TestSSAExplicitFreezeConsumesIso(t *testing.T) {
	gp := loadGownForSSACheck(t, "freeze_use_after.gown", gownSSAExplicitFreezeUseAfterSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestSSAExplicitFreezeResultIsImm(t *testing.T) {
	gp := loadGownForSSACheck(t, "freeze_result_imm.gown", gownSSAExplicitFreezeResultImmSource)

	if errs := checkSendCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA send checker unexpectedly rejected explicit freeze result: %#v", errs)
	}
	if errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected explicit freeze result: %#v", errs)
	}
}

func TestSSAExplicitFreezeRejectsLiveMubBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "freeze_live_borrow.gown", gownSSAExplicitFreezeLiveBorrowSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAExplicitFreezeRejectsFieldProjection(t *testing.T) {
	gp := loadGownForSSACheck(t, "freeze_field.gown", gownSSAExplicitFreezeFieldProjectionSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN011)
}

func TestSSAGWN010FreezeSelfAssignReportsAssignmentMismatch(t *testing.T) {
	err := checkGownSource(t, "freeze_self_assign.gown", gownSSAExplicitFreezeSelfAssignSource)
	requireCheckerCode(t, err, GWN010)
	text := err.Error()
	for _, want := range []string{
		`cannot assign \imm pointer returned by \freeze to \iso pointer "x"`,
		`\freeze produces \imm, not \iso`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("error %q does not contain %q", text, want)
		}
	}
	if strings.Contains(text, `cannot freeze`) {
		t.Fatalf("error still reports freeze itself instead of assignment mismatch: %q", text)
	}
	if strings.Contains(text, `use of moved`) {
		t.Fatalf("invalid freeze assignment should not cascade into use-after-move errors: %q", text)
	}
}

func TestSSAExplicitFreezeAllowsImmPointer(t *testing.T) {
	err := checkGownSource(t, "freeze_imm.gown", gownSSAExplicitFreezeImmSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSSAGWN001ReportsUseAfterInferredFreezeSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_to_imm_use.gown", gownIsoSendToImmChannelUseAfterSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN001)
}

func TestSSAGWN002RejectsInferredFreezeSendWhileNamedBorrowLive(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_to_imm_live_borrow.gown", gownIsoSendToImmChannelLiveBorrowSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN011RejectsFieldProjectionInferredFreezeSend(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_field_to_imm.gown", gownIsoFieldSendToImmChannelSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN011)
}

func loadGownForSSACheck(t *testing.T, name, source string) *GownPackage {
	t.Helper()
	dir := writeGownDir(t, map[string]string{name: source})
	gp := NewGownPackage(dir)
	err := gp.CheckWithOptions(CheckOptions{CheckOnly: true})
	if err == nil {
		return gp
	}
	var checkerErrs CheckerErrors
	if errors.As(err, &checkerErrs) {
		return gp
	}
	t.Fatal(err)
	return nil
}

func requireSSAErrorCode(t *testing.T, errs CheckerErrors, code CheckerErrorCode) {
	t.Helper()
	if len(errs) == 0 {
		t.Fatalf("expected SSA checker error %s, got nil", code)
	}
	if errs[0].Code != code {
		t.Fatalf("SSA checker error code = %s, want %s; errs: %#v", errs[0].Code, code, errs)
	}
}
