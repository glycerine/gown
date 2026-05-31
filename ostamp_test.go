package gown

import (
	"go/ast"
	"go/types"
	"strings"
	"testing"
)

const gownOstampBindingSource = `package example

type Msg struct {
	Data string
}

func Mutate(x \mub *Msg) {}

func Inspect(x \rob *Msg) {}

func Take(x \iso *Msg) {}

func Frozen() \imm *Msg {
	return nil
}

func Use(x \iso *Msg) {
	Mutate(x)
	Inspect(x)
	Take(x)
}
`

const gownOstampObjectSource = `package example

type Msg struct {
	Data string
}

type Holder struct {
	Owned  \iso *Msg
	Frozen \imm *Msg
	Local  \mub *Msg
	Read   \rob *Msg
}

func Channels(work chan \iso *Msg, broadcast chan \imm *Msg) {}

func Locals() {
	var x \iso *Msg
	var y \mub *Msg
	ch := make(chan \iso *Msg)
	_ = x
	_ = y
	_ = ch
}
`

const gownReceiveOstampSource = `package example

type Msg struct{}

func Iso(ch chan \iso *Msg) {
	x := <-ch
	_ = x
}

func Imm(ch chan \imm *Msg) {
	x := <-ch
	_ = x
}
`

const gownFunctionResultOstampSource = `package example

type Msg struct{}

func MakeIso() \iso *Msg {
	return nil
}

func MakeImm() \imm *Msg {
	return nil
}

func Use() {
	x := MakeIso()
	y := MakeImm()
	_, _ = x, y
}
`

const gownIntrinsicIsoMoveInferenceSource = `package example

type Msg struct{}

func Use() {
	j := \new(Msg{})
	a := j
	_ = a
}
`

const gownPlainFreshAllocationUntrackedSource = `package example

type Msg struct{}

func Use() {
	x := &Msg{}
	_ = x
}
`

func TestOstampIndexBindsFunctionSignatures(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"caps.gown": gownOstampBindingSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	if gp.caps == nil {
		t.Fatal("ostamp index is nil")
	}

	wantParamCap(t, gp, "Mutate", 0, CapMub)
	wantParamCap(t, gp, "Inspect", 0, CapRob)
	wantParamCap(t, gp, "Take", 0, CapIso)
	wantParamCap(t, gp, "Use", 0, CapIso)
	wantResultCap(t, gp, "Frozen", 0, CapImm)
}

func TestOstampIndexBindsAnnotatedCallSites(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"calls.gown": gownOstampBindingSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	if gp.caps == nil {
		t.Fatal("ostamp index is nil")
	}

	var got []CallBinding
	for _, call := range gp.caps.CallBindings {
		if call.FuncName == "Use" {
			got = append(got, call)
		}
	}

	want := []struct {
		name string
		cap  Cap
	}{
		{"Mutate", CapMub},
		{"Inspect", CapRob},
		{"Take", CapIso},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d annotated call bindings in Use, got %d", len(want), len(got))
	}
	for i, w := range want {
		if got[i].Callee == nil || got[i].Callee.Name() != w.name {
			t.Fatalf("call %d: got callee %v, want %s", i, got[i].Callee, w.name)
		}
		if len(got[i].ParamCaps) != 1 || got[i].ParamCaps[0] != w.cap {
			t.Fatalf("call %d %s: got param caps %v, want [%v]",
				i, w.name, got[i].ParamCaps, w.cap)
		}
		if len(got[i].Args) != 1 {
			t.Fatalf("call %d %s: got %d args, want 1", i, w.name, len(got[i].Args))
		}
		arg, ok := got[i].Args[0].(*ast.Ident)
		if !ok || arg.Name != "x" {
			t.Fatalf("call %d %s: got arg %#v, want ident x", i, w.name, got[i].Args[0])
		}
		if got[i].Line == 0 || got[i].Col == 0 {
			t.Fatalf("call %d %s: missing source position", i, w.name)
		}
	}
}

func TestOstampIndexBindsChannelElementCaps(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"objects.gown": gownOstampObjectSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	channels := lookupFunc(t, gp, "Channels")
	params := channels.Type().(*types.Signature).Params()
	work := params.At(0)
	broadcast := params.At(1)

	if got := gp.caps.ObjectCap(work); got != CapUntracked {
		t.Fatalf("work object cap = %v, want untracked channel value", got)
	}
	if got := gp.caps.ChanElemCap(work); got != CapIso {
		t.Fatalf("work channel elem cap = %v, want %v", got, CapIso)
	}
	if got := gp.caps.ChanElemCap(broadcast); got != CapImm {
		t.Fatalf("broadcast channel elem cap = %v, want %v", got, CapImm)
	}

	local := lookupLocalVar(t, gp, "Locals", "ch")
	if got := gp.caps.ChanElemCap(local); got != CapIso {
		t.Fatalf("local make channel elem cap = %v, want %v", got, CapIso)
	}
}

func TestOstampIndexBindsStructFields(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"objects.gown": gownOstampObjectSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	holder := lookupNamedType(t, gp, "Holder")
	st := holder.Type().Underlying().(*types.Struct)
	want := map[string]Cap{
		"Owned":  CapIso,
		"Frozen": CapImm,
		"Local":  CapMub,
		"Read":   CapRob,
	}
	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		if got := gp.caps.ObjectCap(field); got != want[field.Name()] {
			t.Fatalf("field %s cap = %v, want %v", field.Name(), got, want[field.Name()])
		}
	}
}

func TestOstampIndexBindsLocalVars(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"objects.gown": gownOstampObjectSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "Locals", "x")); got != CapIso {
		t.Fatalf("local x cap = %v, want %v", got, CapIso)
	}
	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "Locals", "y")); got != CapMub {
		t.Fatalf("local y cap = %v, want %v", got, CapMub)
	}
}

func TestOstampIndexBindsIntrinsicCalls(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"intrinsics.gown": gownIntrinsicSource})

	gp := NewGownPackage(dir)
	if err := gp.CheckWithOptions(CheckOptions{CheckOnly: true}); err != nil {
		t.Fatal(err)
	}

	want := []struct {
		kind   IntrinsicKind
		result string
		arg    string
	}{
		{IntrinsicMub, "b", "x"},
		{IntrinsicRob, "r", "x"},
		{IntrinsicFreeze, "y", "x"},
		{IntrinsicClone, "z", "x"},
		{IntrinsicCloneExported, "z", "x"},
		{IntrinsicUnsafe, "u", "x"},
		{IntrinsicNew, "p", ""},
		{IntrinsicSwap, "", "x"},
	}
	if len(gp.caps.IntrinsicBindings) != len(want) {
		t.Fatalf("intrinsic bindings = %#v, want %d", gp.caps.IntrinsicBindings, len(want))
	}
	for i, w := range want {
		got := gp.caps.IntrinsicBindings[i]
		if got.Kind != w.kind {
			t.Fatalf("binding %d kind = %v, want %v", i, got.Kind, w.kind)
		}
		if intrinsicResultName(got) != w.result {
			t.Fatalf("binding %d result = %q, want %q", i, intrinsicResultName(got), w.result)
		}
		if w.arg == "" {
			if got.ArgPlace.Root != nil {
				t.Fatalf("binding %d arg place = %#v, want none", i, got.ArgPlace)
			}
		} else if got.ArgPlace.Root == nil || got.ArgPlace.Root.Name() != w.arg {
			t.Fatalf("binding %d arg place = %#v, want root %s", i, got.ArgPlace, w.arg)
		}
		if got.Path == "" || !strings.HasSuffix(got.Path, ".gown") {
			t.Fatalf("binding %d path = %q, want original .gown path", i, got.Path)
		}
		if got.Line == 0 || got.Col == 0 {
			t.Fatalf("binding %d missing source position: %#v", i, got)
		}
		if w.kind == IntrinsicSwap && len(got.ArgPlaces) != 2 {
			t.Fatalf("swap binding arg places = %d, want 2", len(got.ArgPlaces))
		}
		if _, ok := gp.caps.IntrinsicBinding(got.Call); !ok {
			t.Fatalf("binding %d not found by call lookup", i)
		}
	}
}

func TestOstampIndexInfersNewIntrinsicAsIso(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"intrinsics.gown": gownIntrinsicSource})

	gp := NewGownPackage(dir)
	if err := gp.CheckWithOptions(CheckOptions{CheckOnly: true}); err != nil {
		t.Fatal(err)
	}

	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "UseNew", "p")); got != CapIso {
		t.Fatalf("new intrinsic local cap = %v, want %v", got, CapIso)
	}
}

func TestOstampIndexInfersCloneIntrinsicAsIso(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"intrinsics.gown": gownIntrinsicSource})

	gp := NewGownPackage(dir)
	if err := gp.CheckWithOptions(CheckOptions{CheckOnly: true}); err != nil {
		t.Fatal(err)
	}

	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "UseClone", "z")); got != CapIso {
		t.Fatalf("clone intrinsic local cap = %v, want %v", got, CapIso)
	}
	if got := gp.caps.ObjectCap(lookupParamVar(t, gp, "UseClone", "x")); got != CapImm {
		t.Fatalf("clone source cap = %v, want original %v", got, CapImm)
	}
}

func TestOstampIndexInfersIsoReceiveLocal(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"receive.gown": gownReceiveOstampSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "Iso", "x")); got != CapIso {
		t.Fatalf("iso receive local cap = %v, want %v", got, CapIso)
	}
}

func TestOstampIndexInfersImmReceiveLocal(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"receive.gown": gownReceiveOstampSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "Imm", "x")); got != CapImm {
		t.Fatalf("imm receive local cap = %v, want %v", got, CapImm)
	}
}

func TestOstampIndexInfersLocalFromFunctionIsoResult(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"results.gown": gownFunctionResultOstampSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "Use", "x")); got != CapIso {
		t.Fatalf("iso result local cap = %v, want %v", got, CapIso)
	}
}

func TestOstampIndexInfersLocalFromFunctionImmResult(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"results.gown": gownFunctionResultOstampSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "Use", "y")); got != CapImm {
		t.Fatalf("imm result local cap = %v, want %v", got, CapImm)
	}
}

func TestOstampIndexInfersIsoMoveFromIntrinsicResult(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"intrinsic_move.gown": gownIntrinsicIsoMoveInferenceSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "Use", "j")); got != CapIso {
		t.Fatalf("intrinsic source local cap = %v, want %v", got, CapIso)
	}
	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "Use", "a")); got != CapIso {
		t.Fatalf("moved local cap = %v, want %v", got, CapIso)
	}
}

func TestOstampIndexLeavesPlainFreshAllocationUntracked(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"plain_fresh.gown": gownPlainFreshAllocationUntrackedSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}

	if got := gp.caps.ObjectCap(lookupLocalVar(t, gp, "Use", "x")); got != CapUntracked {
		t.Fatalf("plain fresh local cap = %v, want %v", got, CapUntracked)
	}
}

func intrinsicResultName(binding IntrinsicBinding) string {
	if binding.Result == nil {
		return ""
	}
	return binding.Result.Name()
}

func wantParamCap(t *testing.T, gp *GownPackage, funcName string, index int, want Cap) {
	t.Helper()
	fn := lookupFunc(t, gp, funcName)
	sig := gp.caps.Funcs[fn]
	if sig == nil {
		t.Fatalf("missing ostamp signature for %s", funcName)
	}
	if index >= len(sig.Params) {
		t.Fatalf("%s has %d params in ostamp signature, want index %d",
			funcName, len(sig.Params), index)
	}
	if sig.Params[index] != want {
		t.Fatalf("%s param %d cap = %v, want %v", funcName, index, sig.Params[index], want)
	}

	param := fn.Type().(*types.Signature).Params().At(index)
	if got := gp.caps.ObjectCap(param); got != want {
		t.Fatalf("%s param object cap = %v, want %v", funcName, got, want)
	}
}

func wantResultCap(t *testing.T, gp *GownPackage, funcName string, index int, want Cap) {
	t.Helper()
	fn := lookupFunc(t, gp, funcName)
	sig := gp.caps.Funcs[fn]
	if sig == nil {
		t.Fatalf("missing ostamp signature for %s", funcName)
	}
	if index >= len(sig.Results) {
		t.Fatalf("%s has %d results in ostamp signature, want index %d",
			funcName, len(sig.Results), index)
	}
	if sig.Results[index] != want {
		t.Fatalf("%s result %d cap = %v, want %v", funcName, index, sig.Results[index], want)
	}
}

func lookupFunc(t *testing.T, gp *GownPackage, name string) *types.Func {
	t.Helper()
	obj := gp.pkg.Types.Scope().Lookup(name)
	fn, ok := obj.(*types.Func)
	if !ok {
		t.Fatalf("could not find function %s", name)
	}
	return fn
}

func lookupNamedType(t *testing.T, gp *GownPackage, name string) *types.TypeName {
	t.Helper()
	obj := gp.pkg.Types.Scope().Lookup(name)
	tn, ok := obj.(*types.TypeName)
	if !ok {
		t.Fatalf("could not find named type %s", name)
	}
	return tn
}

func lookupLocalVar(t *testing.T, gp *GownPackage, funcName, varName string) *types.Var {
	t.Helper()
	for _, file := range gp.pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != funcName || fn.Body == nil {
				continue
			}
			var found *types.Var
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if found != nil {
					return false
				}
				id, ok := n.(*ast.Ident)
				if !ok || id.Name != varName {
					return true
				}
				if obj, ok := gp.pkg.TypesInfo.Defs[id].(*types.Var); ok {
					found = obj
					return false
				}
				return true
			})
			if found != nil {
				return found
			}
		}
	}
	t.Fatalf("could not find local var %s in %s", varName, funcName)
	return nil
}

func lookupParamVar(t *testing.T, gp *GownPackage, funcName, varName string) *types.Var {
	t.Helper()
	fn := lookupFunc(t, gp, funcName)
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Params() == nil {
		t.Fatalf("could not inspect params for %s", funcName)
	}
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		if param.Name() == varName {
			return param
		}
	}
	t.Fatalf("could not find param %s in %s", varName, funcName)
	return nil
}
