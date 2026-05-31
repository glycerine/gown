package gown

import "testing"

const gownCloneValueTypeSource = `package example

type payload struct {
	Data string
}

func (p payload) clone() payload { return p }

func main() {
	var x payload
	y := \clone(x)
	_ = y
}
`

const gownClonePointerTypeSource = `package example

type payload struct {
	Data string
}

func (p *payload) clone() *payload { return &payload{Data: p.Data} }

func main() {
	var x *payload
	y := \clone(x)
	_ = y
}
`

const gownCloneMissingMethodSource = `package example

type payload struct {
	Data string
}

func main() {
	var x *payload
	y := \clone(x)
	_ = y
}
`

const gownExportedClonePointerTypeSource = `package example

type payload struct {
	Data string
}

func (p *payload) Clone() *payload { return &payload{Data: p.Data} }

func main() {
	var x *payload
	y := \Clone(x)
	_ = y
}
`

const gownLowerCloneDoesNotUseExportedMethodSource = `package example

type payload struct {
	Data string
}

func (p *payload) Clone() *payload { return &payload{Data: p.Data} }

func main() {
	var x *payload
	y := \clone(x)
	_ = y
}
`

const gownUpperCloneDoesNotUsePrivateMethodSource = `package example

type payload struct {
	Data string
}

func (p *payload) clone() *payload { return &payload{Data: p.Data} }

func main() {
	var x *payload
	y := \Clone(x)
	_ = y
}
`

const gownCloneWrongReturnSource = `package example

type payload struct {
	Data string
}

func (p *payload) clone() payload { return payload{Data: p.Data} }

func main() {
	var x *payload
	y := \clone(x)
	_ = y
}
`

const gownCloneExtraParamSource = `package example

type payload struct {
	Data string
}

func (p *payload) clone(extra bool) *payload { return &payload{Data: p.Data} }

func main() {
	var x *payload
	y := \clone(x)
	_ = y
}
`

const gownCloneValueDoesNotUsePointerMethodSource = `package example

type payload struct {
	Data string
}

func (p *payload) clone() payload { return payload{Data: p.Data} }

func main() {
	var x payload
	y := \clone(x)
	_ = y
}
`

const gownCloneNonStructSource = `package example

type count int

func (c count) clone() count { return c }

func main() {
	var x count
	y := \clone(x)
	_ = y
}
`

const gownCloneFromCapabilitiesSource = `package example

type payload struct {
	Data string
}

func (p *payload) clone() *payload { return &payload{Data: p.Data} }

func FromIso(ch chan \iso *payload, x \iso *payload) {
	y := \clone(x)
	ch <- y
	println(x)
}

func FromMub(x \iso *payload) {
	b := \mub(x)
	y := \clone(b)
	_, _ = b, y
}

func FromRob(x \iso *payload) {
	r := \rob(x)
	y := \clone(r)
	_, _ = r, y
}

func FromImm(x \imm *payload) {
	y := \clone(x)
	_ = y
}

func FromUntracked(x *payload) {
	y := \clone(x)
	_ = y
}
`

func TestCloneValueTypeWithSameTypeCloneSucceeds(t *testing.T) {
	err := checkGownSource(t, "clone_value.gown", gownCloneValueTypeSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestClonePointerTypeWithSameTypeCloneSucceeds(t *testing.T) {
	err := checkGownSource(t, "clone_pointer.gown", gownClonePointerTypeSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestExportedClonePointerTypeWithSameTypeCloneSucceeds(t *testing.T) {
	err := checkGownSource(t, "exported_clone_pointer.gown", gownExportedClonePointerTypeSource)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCloneMissingMethodRejected(t *testing.T) {
	err := checkGownSource(t, "clone_missing.gown", gownCloneMissingMethodSource)
	requireCheckerCode(t, err, GWN010)
}

func TestLowerCloneDoesNotUseExportedMethod(t *testing.T) {
	err := checkGownSource(t, "lower_clone_exported_method.gown", gownLowerCloneDoesNotUseExportedMethodSource)
	requireCheckerCode(t, err, GWN010)
}

func TestUpperCloneDoesNotUsePrivateMethod(t *testing.T) {
	err := checkGownSource(t, "upper_clone_private_method.gown", gownUpperCloneDoesNotUsePrivateMethodSource)
	requireCheckerCode(t, err, GWN010)
}

func TestCloneWrongReturnTypeRejected(t *testing.T) {
	err := checkGownSource(t, "clone_wrong_return.gown", gownCloneWrongReturnSource)
	requireCheckerCode(t, err, GWN010)
}

func TestCloneExtraParamRejected(t *testing.T) {
	err := checkGownSource(t, "clone_extra_param.gown", gownCloneExtraParamSource)
	requireCheckerCode(t, err, GWN010)
}

func TestCloneValueDoesNotUsePointerOnlyMethod(t *testing.T) {
	err := checkGownSource(t, "clone_value_pointer_method.gown", gownCloneValueDoesNotUsePointerMethodSource)
	requireCheckerCode(t, err, GWN010)
}

func TestCloneNonStructTypeRejected(t *testing.T) {
	err := checkGownSource(t, "clone_non_struct.gown", gownCloneNonStructSource)
	requireCheckerCode(t, err, GWN010)
}

func TestCloneWorksFromAllSourceCapabilitiesWithoutConsumingSource(t *testing.T) {
	gp := loadGownForSSACheck(t, "clone_caps.gown", gownCloneFromCapabilitiesSource)

	if errs := checkGWN001SSA(gp.pkg, gp.ssaPkg, gp.caps); len(errs) != 0 {
		t.Fatalf("SSA GWN001 unexpectedly consumed clone source: %#v", errs)
	}
}
