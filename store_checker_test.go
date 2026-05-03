package gown

import "testing"

const gownRobFieldWriteSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \rob *payload
	a.Data = "changed"
}
`

const gownImmFieldWriteSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \imm *payload
	a.Data = "changed"
}
`

const gownMubFieldWriteSource = `package example

type payload struct {
	Data string
}

func main() {
	var a \mub *payload
	a.Data = "changed"
}
`

const gownRobFieldIncSource = `package example

type payload struct {
	Count int
}

func main() {
	var a \rob *payload
	a.Count++
}
`

const gownMubStoreToFieldSource = `package example

type payload struct {
	Data string
}

type holder struct {
	Item *payload
}

func main() {
	var b \mub *payload
	var h holder
	h.Item = b
}
`

const gownRobStoreToGlobalSource = `package example

type payload struct {
	Data string
}

var sink *payload

func main() {
	var b \rob *payload
	sink = b
}
`

const gownMubLocalAliasSource = `package example

type payload struct {
	Data string
}

func main() {
	var b \mub *payload
	c := b
	_ = c
}
`

func TestGWN005RejectsFieldWriteThroughReadBorrow(t *testing.T) {
	err := checkGownSource(t, "rob_write.gown", gownRobFieldWriteSource)
	requireCheckerCode(t, err, GWN005)
}

func TestGWN005RejectsFieldWriteThroughImmutable(t *testing.T) {
	err := checkGownSource(t, "imm_write.gown", gownImmFieldWriteSource)
	requireCheckerCode(t, err, GWN005)
}

func TestGWN005AllowsFieldWriteThroughMutableBorrow(t *testing.T) {
	err := checkGownSource(t, "mub_write.gown", gownMubFieldWriteSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGWN005RejectsIncThroughReadBorrow(t *testing.T) {
	err := checkGownSource(t, "rob_inc.gown", gownRobFieldIncSource)
	requireCheckerCode(t, err, GWN005)
}

func TestGWN006RejectsMutableBorrowStoredToField(t *testing.T) {
	err := checkGownSource(t, "mub_store_field.gown", gownMubStoreToFieldSource)
	requireCheckerCode(t, err, GWN006)
}

func TestGWN006RejectsReadBorrowStoredToGlobal(t *testing.T) {
	err := checkGownSource(t, "rob_store_global.gown", gownRobStoreToGlobalSource)
	requireCheckerCode(t, err, GWN006)
}

func TestGWN006AllowsLocalBorrowAliasForNow(t *testing.T) {
	err := checkGownSource(t, "mub_local_alias.gown", gownMubLocalAliasSource)
	if err != nil {
		t.Fatal(err)
	}
}
