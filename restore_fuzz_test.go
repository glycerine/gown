package gown

import (
	"errors"
	"testing"
)

func FuzzRestoreScanner(f *testing.F) {
	for _, seed := range []string{
		`a = \restore func(x \iso *node) \iso *node { return x }(a)`,
		`return \restore func(x \iso *node) \iso *node { return x }(a)`,
		`_ = "\restore func(x \iso *node) \iso *node { return x }(a)"`,
		`// \restore func(x \iso *node) \iso *node { return x }(a)`,
		`\restore(x)`,
		`\restore x`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, snippet string) {
		if len(snippet) > 512 {
			t.Skip()
		}
		src := []byte(restoreSource(`func f(a \iso *node) \iso *node {
` + snippet + `
	return a
}
`))
		emitSrc, analysisSrc, gf, err := scanAndClassify("restore_fuzz.gown", src)
		if err != nil {
			return
		}
		for _, ann := range gf.restores {
			for off := ann.Span.Offset; off < ann.Span.End; off++ {
				if emitSrc[off] != ' ' {
					t.Fatalf("restore emit byte at %d = %q, want space", off, emitSrc[off])
				}
				if analysisSrc[off] != ' ' {
					t.Fatalf("restore analysis byte at %d = %q, want space", off, analysisSrc[off])
				}
			}
			if ann.TargetOffset <= ann.Span.End {
				t.Fatalf("restore target offset %d does not follow span [%d,%d)", ann.TargetOffset, ann.Span.Offset, ann.Span.End)
			}
		}
	})
}

func FuzzRestoreGeneratedRegion(f *testing.F) {
	for _, seed := range []struct {
		op     uint8
		define bool
	}{
		{op: 0, define: false},
		{op: 1, define: false},
		{op: 2, define: true},
		{op: 3, define: false},
		{op: 4, define: false},
		{op: 5, define: true},
		{op: 6, define: false},
		{op: 7, define: true},
		{op: 8, define: false},
		{op: 9, define: false},
	} {
		f.Add(seed.op, seed.define)
	}

	f.Fuzz(func(t *testing.T, op uint8, define bool) {
		src, want := restoreFuzzProgram(op, define)
		err := checkGownSource(t, "restore_fuzz_region.gown", src)
		if want == "" {
			if err != nil {
				t.Fatalf("restore fuzz program unexpectedly failed:\n%s\nerror: %v", src, err)
			}
			return
		}
		if err == nil {
			t.Fatalf("restore fuzz program unexpectedly passed; want %s:\n%s", want, src)
		}
		var checkerErrs CheckerErrors
		if !errors.As(err, &checkerErrs) {
			t.Fatalf("restore fuzz program returned non-checker error; want %s:\n%s\nerror: %v", want, src, err)
		}
		for _, checkerErr := range checkerErrs {
			if checkerErr.Code == want {
				return
			}
		}
		t.Fatalf("restore fuzz program returned %v; want %s:\n%s", checkerErrs, want, src)
	})
}

func restoreFuzzProgram(op uint8, define bool) (string, CheckerErrorCode) {
	body, want := restoreFuzzBody(op)
	lhs := "a ="
	ret := "a"
	if define {
		lhs = "dst :="
		ret = "dst"
	}
	return restoreSource(`func f(a \iso *node, b \iso *node, frozen \imm *node, flag bool) \iso *node {
	` + lhs + ` \restore func(x \iso *node, y \iso *node, r \imm *node) \iso *node {
` + body + `
	}(a, b, frozen)
	return ` + ret + `
}
`), want
}

func restoreFuzzBody(op uint8) (string, CheckerErrorCode) {
	switch op % 12 {
	case 0:
		return "\t\treturn x", ""
	case 1:
		return "\t\tx.next = y\n\t\treturn x", ""
	case 2:
		return "\t\ttmp := x.next\n\t\tx.next = y\n\t\ty.next = tmp\n\t\treturn x", ""
	case 3:
		return "\t\tif flag {\n\t\t\tx.next = nil\n\t\t}\n\t\treturn x", ""
	case 4:
		return "\t\treturn y", ""
	case 5:
		return "\t\ttmp := x.next\n\t\t_ = tmp\n\t\treturn x", GWN013
	case 6:
		return "\t\tx.prev = y\n\t\treturn x", GWN013
	case 7:
		return "\t\tx.next = r\n\t\treturn x", GWN013
	case 8:
		return "\t\tvar boxed any\n\t\tboxed = x\n\t\t_ = boxed\n\t\treturn x", GWN013
	case 9:
		return "\t\thelper(x)\n\t\treturn x", GWN013
	case 10:
		return "\t\tgo helper(x)\n\t\treturn x", GWN013
	default:
		return "\t\tx = y\n\t\treturn x", GWN013
	}
}
