package gown

import "testing"

func TestSSAGWN005RejectsReadOnlyFieldWrite(t *testing.T) {
	gp := loadGownForSSACheck(t, "rob_write.gown", gownRobFieldWriteSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN005)
}

func TestSSAGWN005RejectsImmutableFieldWrite(t *testing.T) {
	gp := loadGownForSSACheck(t, "imm_write.gown", gownImmFieldWriteSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN005)
}

func TestSSAGWN005AllowsMutableFieldWrite(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_write.gown", gownMubFieldWriteSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA store checker unexpectedly rejected mutable write: %#v", errs)
	}
}

func TestSSAGWN005RejectsIncThroughReadBorrow(t *testing.T) {
	gp := loadGownForSSACheck(t, "rob_inc.gown", gownRobFieldIncSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN005)
}

func TestSSAGWN006RejectsMutableBorrowStoreToField(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_store_field.gown", gownMubStoreToFieldSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN006)
}

func TestSSAGWN006RejectsReadBorrowStoreToGlobal(t *testing.T) {
	gp := loadGownForSSACheck(t, "rob_store_global.gown", gownRobStoreToGlobalSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN006)
}

func TestSSAGWN006AllowsMutableBorrowLocalAlias(t *testing.T) {
	gp := loadGownForSSACheck(t, "mub_local_alias.gown", gownMubLocalAliasSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA store checker unexpectedly rejected local alias: %#v", errs)
	}
}
