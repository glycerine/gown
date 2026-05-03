package gown

import "testing"

func TestSSAGWN002RejectsTwoMutableBorrowsInOneCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_twice.gown", gownMubTwiceSameArgSource)

	errs := checkGWN002SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002RejectsMutableAndReadBorrowInOneCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_rob.gown", gownMubRobSameArgSource)

	errs := checkGWN002SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002AllowsTwoReadBorrowsInOneCall(t *testing.T) {
	gp := loadGownForSSACheck(t, "rob_twice.gown", gownRobTwiceSameArgSource)

	errs := checkGWN002SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN002 unexpectedly rejected read borrows: %#v", errs)
	}
}

func TestSSAGWN002AllowsMutableBorrowsOfSiblingFields(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_sibling_fields.gown", gownMubSiblingFieldsSource)

	errs := checkGWN002SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN002 unexpectedly rejected sibling fields: %#v", errs)
	}
}

func TestSSAGWN002RejectsMutableBorrowsOfSameField(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_same_field.gown", gownMubSameFieldSource)

	errs := checkGWN002SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}

func TestSSAGWN002RejectsMutableBorrowsOfRootAndField(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_root_field.gown", gownMubRootAndFieldSource)

	errs := checkGWN002SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN002)
}
