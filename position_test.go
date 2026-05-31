package gown

import (
	"bytes"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gownSource = `package example

type Msg struct {
	Data []byte
}

func Send(ch chan *Msg, m \iso *Msg) {
	_ = ch
	_ = m
}

func Recv(ch chan *Msg) \iso *Msg {
	return \new(Msg{})
}
`

func writeGownDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module example\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPositionPrecision(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"example.gown": gownSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	if len(gf.iso) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(gf.iso))
	}
	for i, a := range gf.iso {
		t.Logf("annotation %d: %s:%d:%d func=%s",
			i, gf.path, a.line, a.col, a.funcName)
	}

	// Verify stripped .go has no \iso left.
	goBytes, err := os.ReadFile(filepath.Join(dir, "example.go"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(goBytes, []byte(`\iso`)) {
		t.Fatal("stripped .go still contains \\iso")
	}

	// Verify byte-precision: \iso at offset X → AST type at X+5.
	goSrc, _ := scanAndStrip("", []byte(gownSource))
	matched := 0
	for _, ann := range gf.iso {
		expected := ann.offset + 5
		for _, file := range gp.pkg.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				for _, fields := range [...]*ast.FieldList{fn.Type.Params, fn.Type.Results} {
					if fields == nil {
						continue
					}
					for _, field := range fields.List {
						pos := gp.pkg.Fset.Position(field.Type.Pos())
						if pos.Offset == expected {
							matched++
							orig := gownSource[ann.offset : ann.offset+5]
							if orig != `\iso ` {
								t.Errorf("expected original bytes %q, got %q", `\iso `, orig)
							}
							stripped := string(goSrc[ann.offset : ann.offset+4])
							if strings.TrimSpace(stripped) != "" {
								t.Errorf("expected spaces at stripped offset, got %q", stripped)
							}
						}
					}
				}
			}
		}
	}
	if matched != len(gf.iso) {
		t.Fatalf("matched %d of %d annotations to AST positions", matched, len(gf.iso))
	}

	if gf.iso[0].line != 7 || gf.iso[1].line != 11 {
		t.Errorf("lines: got %d,%d want 7,11", gf.iso[0].line, gf.iso[1].line)
	}
	if gf.iso[0].funcName != "Send" || gf.iso[1].funcName != "Recv" {
		t.Errorf("funcNames: got %q,%q want Send,Recv", gf.iso[0].funcName, gf.iso[1].funcName)
	}

	t.Logf("All %d annotations matched — offset, line, col, funcName confirmed", matched)
}

const gownRegionSource = `package example

type Msg struct {
	Data []byte
}

func Process(ch chan *Msg) {
	if true {
		var m \iso *Msg
		_ = m
	}
	{
		var n \iso *Msg
		_ = n
	}
}
`

func TestRegionDetection(t *testing.T) {
	dir := writeGownDir(t, map[string]string{"regions.gown": gownRegionSource})

	gp := NewGownPackage(dir)
	if err := gp.Check(); err != nil {
		t.Fatal(err)
	}
	gf := gp.files[0]

	if len(gf.iso) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(gf.iso))
	}

	for i, ann := range gf.iso {
		if ann.funcName != "Process" {
			t.Errorf("annotation %d: expected funcName Process, got %q", i, ann.funcName)
		}
		if ann.scope == nil {
			t.Fatalf("annotation %d: scope is nil", i)
		}
		t.Logf("annotation %d: %s:%d:%d func=%s scope=[%d,%d)",
			i, gf.path, ann.line, ann.col, ann.funcName, ann.scope.beg, ann.scope.endx)
	}

	if gf.iso[0].scope.beg == gf.iso[1].scope.beg &&
		gf.iso[0].scope.endx == gf.iso[1].scope.endx {
		t.Fatal("both annotations in same scope — expected different scopes")
	}

	for i, ann := range gf.iso {
		if ann.offset < ann.scope.beg || ann.offset >= ann.scope.endx {
			t.Errorf("annotation %d: offset %d outside scope [%d,%d)",
				i, ann.offset, ann.scope.beg, ann.scope.endx)
		}
	}

	t.Log("Region detection confirmed: 2 annotations in 2 different scopes")
}
