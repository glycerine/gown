package gown

import (
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
	for _, call := range gp.caps.Calls {
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
		if got[i].Line == 0 || got[i].Col == 0 {
			t.Fatalf("call %d %s: missing source position", i, w.name)
		}
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
	if got := gp.caps.ObjectCaps[param]; got != want {
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
