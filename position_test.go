package gown

import (
	"bytes"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const gownSource = `package example

type Msg struct {
	Data []byte
}

func Send(ch chan *Msg, m \iso *Msg) {
	ch <- m
}

func Recv(ch chan *Msg) \iso *Msg {
	return <-ch
}
`

func TestPositionPrecision(t *testing.T) {
	// 1. Scan and strip
	goSrc, annotations := scanAndStrip([]byte(gownSource))
	t.Logf("found %d \\iso annotations", len(annotations))
	for i, a := range annotations {
		t.Logf("  annotation %d: offset=%d line=%d col=%d context=%q",
			i, a.offset, a.line, a.col, gownSource[a.offset:a.offset+4])
	}
	if len(annotations) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(annotations))
	}

	// 2. Verify the stripped source is valid Go (no \iso left)
	if bytes.Contains(goSrc, []byte(`\iso`)) {
		t.Fatal("stripped source still contains \\iso")
	}
	t.Logf("stripped source:\n%s", goSrc)

	// 3. Write to temp dir with a go.mod
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"),
		[]byte("module example\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}
	goFile := filepath.Join(tmpDir, "example.go")
	if err := os.WriteFile(goFile, goSrc, 0644); err != nil {
		t.Fatal(err)
	}

	// 4. Load with go/packages
	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedName,
		Dir: tmpDir,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages loaded")
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		for _, e := range pkg.Errors {
			t.Errorf("package error: %v", e)
		}
		t.Fatal("package had errors")
	}
	if len(pkg.Syntax) == 0 {
		t.Fatal("no syntax trees")
	}

	// 5. Walk AST, collect positions of parameter/return types
	type paramInfo struct {
		funcName  string
		paramName string
		typeStr   string
		offset    int
	}
	var params []paramInfo

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			// Check parameters
			if fn.Type.Params != nil {
				for _, field := range fn.Type.Params.List {
					pos := pkg.Fset.Position(field.Type.Pos())
					name := ""
					if len(field.Names) > 0 {
						name = field.Names[0].Name
					}
					params = append(params, paramInfo{
						funcName:  fn.Name.Name,
						paramName: name,
						typeStr:   fmt.Sprintf("%v", field.Type),
						offset:    pos.Offset,
					})
				}
			}
			// Check return types
			if fn.Type.Results != nil {
				for _, field := range fn.Type.Results.List {
					pos := pkg.Fset.Position(field.Type.Pos())
					params = append(params, paramInfo{
						funcName:  fn.Name.Name,
						paramName: "(return)",
						typeStr:   fmt.Sprintf("%v", field.Type),
						offset:    pos.Offset,
					})
				}
			}
		}
	}

	t.Logf("\nAST parameter positions:")
	for _, p := range params {
		t.Logf("  %s.%s type=%s offset=%d", p.funcName, p.paramName, p.typeStr, p.offset)
	}

	// 6. Check that \iso annotations line up with AST positions.
	// For each \iso at offset X, the type node should start at X+5
	// (4 bytes for \iso + 1 space).
	// Also verify line/col are 1-based and correct.
	matched := 0
	for _, ann := range annotations {
		expected := ann.offset + 5 // \iso + space
		for _, p := range params {
			if p.offset == expected {
				t.Logf("MATCH: \\iso@%d:%d:%d → %s.%s type@%d",
					ann.offset, ann.line, ann.col,
					p.funcName, p.paramName, p.offset)
				matched++

				// Verify the original bytes
				orig := gownSource[ann.offset : ann.offset+5]
				if orig != `\iso ` {
					t.Errorf("expected original bytes %q, got %q", `\iso `, orig)
				}
				// Verify the stripped bytes are spaces
				stripped := string(goSrc[ann.offset : ann.offset+4])
				if strings.TrimSpace(stripped) != "" {
					t.Errorf("expected spaces at stripped offset, got %q", stripped)
				}
			}
		}
	}

	if matched != len(annotations) {
		t.Errorf("matched %d of %d annotations", matched, len(annotations))
		t.Log("\nDumping all offsets for debugging:")
		for _, ann := range annotations {
			t.Logf("  \\iso@%d:%d:%d, expected type@%d", ann.offset, ann.line, ann.col, ann.offset+5)
		}
		for _, p := range params {
			t.Logf("  AST %s.%s offset=%d", p.funcName, p.paramName, p.offset)
		}
		t.Fatal("not all annotations matched AST positions")
	}

	// 7. Verify line/col are 1-based and match expected positions in the source.
	// gownSource line 7: "func Send(ch chan *Msg, m \iso *Msg) {"
	// gownSource line 11: "func Recv(ch chan *Msg) \iso *Msg {"
	if annotations[0].line != 7 {
		t.Errorf("annotation 0: expected line 7, got %d", annotations[0].line)
	}
	if annotations[1].line != 11 {
		t.Errorf("annotation 1: expected line 11, got %d", annotations[1].line)
	}
	if annotations[0].col < 1 {
		t.Errorf("annotation 0: col must be >= 1, got %d", annotations[0].col)
	}
	if annotations[1].col < 1 {
		t.Errorf("annotation 1: col must be >= 1, got %d", annotations[1].col)
	}

	t.Logf("\nAll %d annotations matched — offset, line, col all confirmed", matched)
}
