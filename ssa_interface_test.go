package gown

import "testing"

const gownIsoFieldToInterfaceSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item \iso *payload
}

func main() {
	var h \iso *holder
	var x any = h.Item
	_ = x
}
`

func TestSSAGWN009RejectsIsoStoredIntoInterface(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_interface.gown", gownIsoToInterfaceValueSpecSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN009)
}

func TestSSAGWN009RejectsBorrowAssignedIntoInterface(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_interface.gown", gownMubToInterfaceAssignSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN009)
}

func TestSSAGWN009AllowsUntrackedValueStoredIntoInterface(t *testing.T) {
	gp := loadGownForSSACheck(t, "untracked_interface.gown", gownUntrackedToInterfaceSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA interface checker unexpectedly rejected untracked value: %#v", errs)
	}
}

func TestSSAGWN009RejectsTrackedFieldStoredIntoInterface(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_field_interface.gown", gownIsoFieldToInterfaceSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN009)
}
