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
	var h *holder
	var x any = h.Item
	_ = x
}
`

func TestSSAGWN009AllowsIsoStoredIntoInterfaceAsFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_interface.gown", gownIsoToInterfaceValueSpecSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA interface checker unexpectedly rejected proof frontier: %#v", errs)
	}
}

func TestSSAGWN009AllowsBorrowAssignedIntoInterfaceAsFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_interface.gown", gownMubToInterfaceAssignSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA interface checker unexpectedly rejected borrow proof frontier: %#v", errs)
	}
}

func TestSSAGWN009AllowsUntrackedValueStoredIntoInterface(t *testing.T) {
	gp := loadGownForSSACheck(t, "untracked_interface.gown", gownUntrackedToInterfaceSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA interface checker unexpectedly rejected untracked value: %#v", errs)
	}
}

func TestSSAGWN009AllowsTrackedFieldStoredIntoInterfaceAsFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_field_interface.gown", gownIsoFieldToInterfaceSource)

	errs := checkInterfaceErasureSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA interface checker unexpectedly rejected field proof frontier: %#v", errs)
	}
}
