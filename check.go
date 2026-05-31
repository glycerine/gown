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
		if !e.IsDir() && isGownSourceFileName(e.Name()) {
			gownNames[e.Name()] = true
		}
	}
	for path := range opts.GownOverlay {
		if isGownSourceFileName(filepath.Base(path)) && samePackagePath(gp.path, path) {
			gownNames[filepath.Base(path)] = true
		}
	}

	overlay := make(map[string][]byte)
	emitSources := make(map[string][]byte)
	needIntrinsicHelpers := false
	firstOverlayPath := ""
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
		if len(gf.intrinsics) > 0 {
			needIntrinsicHelpers = true
		}

		goName := strings.TrimSuffix(name, ".gown") + ".go"
		goPath := filepath.Join(gp.path, goName)
		absGoPath, err := filepath.Abs(goPath)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", goPath, err)
		}
		overlay[absGoPath] = analysisSrc
		emitSources[absGoPath] = emitSrc
		if firstOverlayPath == "" {
			firstOverlayPath = absGoPath
		}
	}

	if len(gownNames) == 0 {
		return nil, fmt.Errorf("no .gown files found in %s", gp.path)
	}

	if len(gp.files) == 0 {
		return nil, fmt.Errorf("no .gown files found in %s", gp.path)
	}

	if needIntrinsicHelpers && firstOverlayPath != "" {
		overlay[firstOverlayPath] = append(append([]byte(nil), overlay[firstOverlayPath]...), []byte(intrinsicAnalysisHelperDecls())...)
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

	if !opts.CheckOnly {
		for _, gf := range gp.files {
			goPath := filepath.Join(gp.path, generatedGoName(gf.path))
			absGoPath, err := filepath.Abs(goPath)
			if err != nil {
				return nil, fmt.Errorf("resolving %s: %w", goPath, err)
			}
			emitSrc := emitSources[absGoPath]
			finalSrc, err := buildEmitSource(gp.pkg, gp.caps, gf, emitSrc)
			if err != nil {
				return nil, err
			}
			if err := os.WriteFile(goPath, finalSrc, 0644); err != nil {
				return nil, fmt.Errorf("writing %s: %w", goPath, err)
			}
		}
	}

	return gp.analysis(), nil
}

func intrinsicAnalysisHelperDecls() string {
	return `
func mub_[T any](x T) T { return x }
func rob_[T any](x T) T { return x }
func freeze_[T any](x T) T { return x }
func clone_[T any](x T) T { return x }
func unsafe_[T any](x T) T { return x }
func new_[T any](x T) *T { return &x }
`
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

func isGownSourceFileName(name string) bool {
	if !strings.HasSuffix(name, ".gown") {
		return false
	}
	base := filepath.Base(name)
	if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") {
		return false
	}
	return true
}
