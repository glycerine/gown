package gown

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

type GownPackage struct {
	path    string // directory containing the package
	pkg     *packages.Package
	files   []*gownFile
	caps    *CapabilityIndex
	ssaProg *ssa.Program
	ssaPkg  *ssa.Package
}

type CheckOptions struct {
	CheckOnly   bool
	GownOverlay map[string][]byte
}

type GownAnalysis struct {
	Path    string
	Package *packages.Package
	Files   []*gownFile
	Caps    *CapabilityIndex
	SSAPkg  *ssa.Package
}

func NewGownPackage(path string) *GownPackage {
	return &GownPackage{path: path}
}

// Check finds all .gown files in the package directory, scans them
// for \iso annotations, strips annotations to produce .go files
// (overwriting any existing .go alongside each .gown), loads the
// package with go/packages, and assigns regions/funcNames from the AST.
func (gp *GownPackage) Check() error {
	return gp.CheckWithOptions(CheckOptions{})
}

// CheckWithOptions runs the Gown checker. In normal mode it writes generated
// .go files beside .gown files. In check-only mode it keeps generated sources
// in a go/packages overlay so validation does not modify the package directory.
func (gp *GownPackage) CheckWithOptions(opts CheckOptions) error {
	_, err := gp.AnalyzeWithOptions(opts)
	return err
}

func (gp *GownPackage) AnalyzeWithOptions(opts CheckOptions) (*GownAnalysis, error) {
	gp.pkg = nil
	gp.files = nil
	gp.caps = nil
	gp.ssaProg = nil
	gp.ssaPkg = nil

	if len(opts.GownOverlay) > 0 {
		opts.CheckOnly = true
	}

	entries, err := os.ReadDir(gp.path)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", gp.path, err)
	}

	gownNames := make(map[string]bool)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".gown") {
			gownNames[e.Name()] = true
		}
	}
	for path := range opts.GownOverlay {
		if strings.HasSuffix(path, ".gown") && samePackagePath(gp.path, path) {
			gownNames[filepath.Base(path)] = true
		}
	}

	overlay := make(map[string][]byte)
	for name := range gownNames {
		gownPath := filepath.Join(gp.path, name)
		src, ok := lookupGownOverlay(opts.GownOverlay, gownPath)
		if !ok {
			src, err = os.ReadFile(gownPath)
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", gownPath, err)
			}
		}

		emitSrc, analysisSrc, gf, err := scanAndClassify(name, src)
		if err != nil {
			return nil, err
		}
		gp.files = append(gp.files, gf)

		goName := strings.TrimSuffix(name, ".gown") + ".go"
		goPath := filepath.Join(gp.path, goName)
		if opts.CheckOnly {
			absGoPath, err := filepath.Abs(goPath)
			if err != nil {
				return nil, fmt.Errorf("resolving %s: %w", goPath, err)
			}
			overlay[absGoPath] = analysisSrc
			continue
		}
		if err := os.WriteFile(goPath, emitSrc, 0644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", goPath, err)
		}
	}

	if len(gp.files) == 0 {
		return nil, fmt.Errorf("no .gown files found in %s", gp.path)
	}

	cfg := &packages.Config{
		Mode: packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedName |
			packages.NeedImports | packages.NeedTypesSizes,
		Dir: gp.path,
	}
	if len(overlay) > 0 {
		cfg.Overlay = overlay
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return nil, fmt.Errorf("packages.Load: %w", err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found in %s", gp.path)
	}
	gp.pkg = pkgs[0]
	if len(gp.pkg.Errors) > 0 {
		return nil, fmt.Errorf("package error: %v", gp.pkg.Errors[0])
	}
	gp.caps = assignCapabilities(gp.pkg, gp.files)
	if err := gp.buildSSA(); err != nil {
		return nil, err
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

	if errs := runCheckerPasses(gp.pkg, gp.ssaPkg, gp.caps); len(errs) > 0 {
		return gp.analysis(), errs
	}

	return gp.analysis(), nil
}

func (gp *GownPackage) analysis() *GownAnalysis {
	return &GownAnalysis{
		Path:    gp.path,
		Package: gp.pkg,
		Files:   gp.files,
		Caps:    gp.caps,
		SSAPkg:  gp.ssaPkg,
	}
}

func samePackagePath(pkgPath, filePath string) bool {
	absPkg, err := filepath.Abs(pkgPath)
	if err != nil {
		return false
	}
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}
	return filepath.Dir(absFile) == absPkg
}

func lookupGownOverlay(overlay map[string][]byte, path string) ([]byte, bool) {
	if len(overlay) == 0 {
		return nil, false
	}
	if src, ok := overlay[path]; ok {
		return src, true
	}
	absPath, err := filepath.Abs(path)
	if err == nil {
		if src, ok := overlay[absPath]; ok {
			return src, true
		}
	}
	if src, ok := overlay[filepath.Base(path)]; ok {
		return src, true
	}
	return nil, false
}
