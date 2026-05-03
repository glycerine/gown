package gown

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

type GownPackage struct {
	path  string // directory containing the package
	pkg   *packages.Package
	files []*gownFile
}

func NewGownPackage(path string) *GownPackage {
	return &GownPackage{path: path}
}

// Check finds all .gown files in the package directory, scans them
// for \iso annotations, strips annotations to produce .go files
// (overwriting any existing .go alongside each .gown), loads the
// package with go/packages, and assigns regions/funcNames from the AST.
func (gp *GownPackage) Check() error {
	entries, err := os.ReadDir(gp.path)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", gp.path, err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".gown") {
			continue
		}
		gownPath := filepath.Join(gp.path, e.Name())
		src, err := os.ReadFile(gownPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", gownPath, err)
		}

		goSrc, _, gf, err := scanAndClassify(e.Name(), src)
		if err != nil {
			return err
		}
		gp.files = append(gp.files, gf)

		goName := strings.TrimSuffix(e.Name(), ".gown") + ".go"
		goPath := filepath.Join(gp.path, goName)
		if err := os.WriteFile(goPath, goSrc, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", goPath, err)
		}
	}

	if len(gp.files) == 0 {
		return fmt.Errorf("no .gown files found in %s", gp.path)
	}

	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedName,
		Dir: gp.path,
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return fmt.Errorf("packages.Load: %w", err)
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("no packages found in %s", gp.path)
	}
	gp.pkg = pkgs[0]
	if len(gp.pkg.Errors) > 0 {
		return fmt.Errorf("package error: %v", gp.pkg.Errors[0])
	}

	for _, gf := range gp.files {
		assignRegions(gp.pkg, gf)
		assignBoundary(gp.pkg, gf)
	}

	// Collect all boundary crossings across files.
	var allBoundary []*boundaryCrossing
	for _, gf := range gp.files {
		allBoundary = append(allBoundary, gf.boundary...)
	}

	// Collect \iso annotation types for fallback when no boundaries exist.
	var isoTypes []types.Type
	for _, gf := range gp.files {
		for _, ann := range gf.iso {
			// Find the AST type near this annotation offset.
			for _, file := range gp.pkg.Syntax {
				for _, decl := range file.Decls {
					fn, ok := decl.(*ast.FuncDecl)
					if !ok {
						continue
					}
					// Check params and results for types near the annotation.
					for _, fields := range []*ast.FieldList{fn.Type.Params, fn.Type.Results} {
						if fields == nil {
							continue
						}
						for _, field := range fields.List {
							pos := gp.pkg.Fset.Position(field.Type.Pos())
							// The type starts right after the \iso + space (4 bytes stripped).
							if pos.Offset == ann.offset+5 || pos.Offset == ann.offset+4 {
								if tv, ok := gp.pkg.TypesInfo.Types[field.Type]; ok {
									isoTypes = append(isoTypes, tv.Type)
								}
							}
						}
					}
				}
			}
			// Also check local variable declarations near the annotation.
			for _, file := range gp.pkg.Syntax {
				ast.Inspect(file, func(n ast.Node) bool {
					vs, ok := n.(*ast.ValueSpec)
					if !ok || vs.Type == nil {
						return true
					}
					pos := gp.pkg.Fset.Position(vs.Type.Pos())
					if pos.Offset == ann.offset+5 || pos.Offset == ann.offset+4 {
						if tv, ok := gp.pkg.TypesInfo.Types[vs.Type]; ok {
							isoTypes = append(isoTypes, tv.Type)
						}
					}
					return true
				})
			}
		}
	}

	reachable, poisoned := computeReachableTypes(allBoundary, isoTypes)

	for _, gf := range gp.files {
		assignCreates(gp.pkg, gf, reachable, poisoned)
	}

	return nil
}
