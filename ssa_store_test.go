package gown

import "testing"

const gownExplicitFreezeWriteSource = `package example

type payload struct {
	Data string
}

func main() {
	var x \iso *payload
	y := \freeze(x)
	y.Data = "changed"
}
`

const gownRobRootWriteToMubFieldSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item \mub *payload
}

func main() {
	var h \rob *holder
	h.Item.Data = "changed"
}
`

const gownImmRootWriteToUntrackedFieldSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item *payload
}

func main() {
	var h \imm *holder
	h.Item.Data = "changed"
}
`

const gownIsoStoreToGlobalFrontierSource = `package example

type payload struct {
	Data string
}

var sink *payload

func main(ch chan \iso *payload) {
	var x \iso *payload
	sink = x
	ch <- x
}
`

const gownIsoStoreToFieldFrontierSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item *payload
}

func main(ch chan \iso *payload) {
	var x \iso *payload
	var h holder
	h.Item = x
	ch <- x
}
`

const gownImmStoreToGlobalAllowedSource = `package example

type payload struct {
	Data string
}

var sink *payload

func main() {
	var y \imm *payload
	sink = y
	println(y)
}
`

const gownBorrowStoreToSliceSource = `package example

type payload struct {
	Data string
}

func main(items []*payload) {
	var b \mub *payload
	items[0] = b
}
`

const gownIsoStoreToMapFrontierSource = `package example

type payload struct {
	Data string
}

func main(ch chan \iso *payload, items map[string]*payload) {
	var x \iso *payload
	items["x"] = x
	ch <- x
}
`

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

func TestSSAExplicitFreezeResultRejectsWrites(t *testing.T) {
	gp := loadGownForSSACheck(t, "freeze_write.gown", gownExplicitFreezeWriteSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN005)
}

func TestSSAWriteThroughRobRootToMubFieldRejected(t *testing.T) {
	gp := loadGownForSSACheck(t, "rob_root_mub_field_write.gown", gownRobRootWriteToMubFieldSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN005)
}

func TestSSAWriteThroughImmRootToUntrackedFieldRejected(t *testing.T) {
	gp := loadGownForSSACheck(t, "imm_root_untracked_field_write.gown", gownImmRootWriteToUntrackedFieldSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN005)
}

func TestSSAIsoStoreToGlobalCreatesFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_store_global.gown", gownIsoStoreToGlobalFrontierSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
}

func TestSSAIsoStoreToFieldCreatesFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_store_field.gown", gownIsoStoreToFieldFrontierSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
}

func TestSSAImmStoreToGlobalAllowedByPolicy(t *testing.T) {
	gp := loadGownForSSACheck(t, "imm_store_global.gown", gownImmStoreToGlobalAllowedSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	if len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly rejected imm store to global: %#v", errs)
	}
}

func TestSSABorrowStoreToSliceRejected(t *testing.T) {
	gp := loadGownForSSACheck(t, "borrow_store_slice.gown", gownBorrowStoreToSliceSource)

	errs := checkStoreCapabilitiesSSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN006)
}

func TestSSAIsoStoreToMapCreatesFrontier(t *testing.T) {
	gp := loadGownForSSACheck(t, "iso_store_map.gown", gownIsoStoreToMapFrontierSource)

	errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps)
	requireSSAErrorCode(t, errs, GWN012)
}
