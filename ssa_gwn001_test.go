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

func loadGownForSSACheck(t *testing.T, name, source string) *GownPackage {
	t.Helper()
	dir := writeGownDir(t, map[string]string{name: source})
	gp := NewGownPackage(dir)
	err := gp.Check()
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
