package gown

import (
	"go/ast"
	"go/types"
	"testing"
)

const gownCapabilityBindingSource = `package example

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

const gownCapabilityObjectSource = `package example

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
	_ = x
	_ = y
}
`

func TestCapabilityIndexBindsFunctionSignatures(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"caps.gown": gownCapabilityBindingSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	if gp.caps == nil {
		t.Fatal("capability index is nil")
	}

	wantParamCap(t, gp, "Mutate", 0, CapMub)
	wantParamCap(t, gp, "Inspect", 0, CapRob)
	wantParamCap(t, gp, "Take", 0, CapIso)
	wantParamCap(t, gp, "Use", 0, CapIso)
	wantResultCap(t, gp, "Frozen", 0, CapImm)
}

func TestCapabilityIndexBindsAnnotatedCallSites(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"calls.gown": gownCapabilityBindingSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	if gp.caps == nil {
		t.Fatal("capability index is nil")
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

func TestCapabilityIndexBindsChannelElementCaps(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"objects.gown": gownCapabilityObjectSource})

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
}

func TestCapabilityIndexBindsStructFields(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"objects.gown": gownCapabilityObjectSource})

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

func TestCapabilityIndexBindsLocalVars(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"objects.gown": gownCapabilityObjectSource})

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

func wantParamCap(t *testing.T, gp *GownPackage, funcName string, index int, want Cap) {
	t.Helper()
	fn := lookupFunc(t, gp, funcName)
	sig := gp.caps.Funcs[fn]
	if sig == nil {
		t.Fatalf("missing capability signature for %s", funcName)
	}
	if index >= len(sig.Params) {
		t.Fatalf("%s has %d params in capability signature, want index %d",
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
		t.Fatalf("missing capability signature for %s", funcName)
	}
	if index >= len(sig.Results) {
		t.Fatalf("%s has %d results in capability signature, want index %d",
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
