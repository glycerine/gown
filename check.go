package gown

import (
	"fmt"
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

		goSrc, gf := scanAndStrip(e.Name(), src)
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
		assignCreates(gp.pkg, gf)
	}

	return nil
}
