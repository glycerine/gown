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
	caps    *OstampIndex
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
	Caps    *OstampIndex
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
	//vv("GOWN AnalyzeWithOptions start path=%q checkOnly=%v gownOverlay=%d", gp.path, opts.CheckOnly, len(opts.GownOverlay))

	entries, err := os.ReadDir(gp.path)
	if err != nil {
		//vv("GOWN AnalyzeWithOptions return: os.ReadDir failed path=%q err=%v", gp.path, err)
		return nil, fmt.Errorf("reading directory %s: %w", gp.path, err)
	}
	//vv("GOWN read directory path=%q entries=%d", gp.path, len(entries))

	gownNames := make(map[string]bool)
	for _, e := range entries {
		if !e.IsDir() && isGownSourceFileName(e.Name()) {
			gownNames[e.Name()] = true
			vv("GOWN discovered .gown file name=%q", e.Name()) // seen
		}
	}
	for path := range opts.GownOverlay {
		if isGownSourceFileName(filepath.Base(path)) && samePackagePath(gp.path, path) {
			gownNames[filepath.Base(path)] = true
			vv("GOWN discovered overlay .gown file path=%q base=%q", path, filepath.Base(path)) // not seen
		}
	}
	vv("GOWN .gown discovery complete path=%q count=%d", gp.path, len(gownNames))

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
				vv("GOWN AnalyzeWithOptions return: os.ReadFile failed path=%q err=%v", gownPath, err)
				return nil, fmt.Errorf("reading %s: %w", gownPath, err)
			}
		}
		vv("GOWN loaded .gown source path=%q overlay=%v bytes=%d", gownPath, ok, len(src))

		emitSrc, analysisSrc, gf, err := scanAndClassify(name, src)
		if err != nil {
			vv("GOWN AnalyzeWithOptions return: scanAndClassify failed file=%q err=%v", name, err) // not seen
			return nil, err
		}
		vv("emitSrc = '%v';\n\n analysisSrc = '%v'", string(emitSrc), string(analysisSrc))
		gp.files = append(gp.files, gf)
		if len(gf.intrinsics) > 0 {
			needIntrinsicHelpers = true
		}
		vv("GOWN classified file=%q iso=%d intrinsics=%d boundary=%d creates=%d", gf.path, len(gf.iso), len(gf.intrinsics), len(gf.boundary), len(gf.create))

		goName := strings.TrimSuffix(name, ".gown") + ".go"
		goPath := filepath.Join(gp.path, goName)
		absGoPath, err := filepath.Abs(goPath)
		if err != nil {
			vv("GOWN AnalyzeWithOptions return: filepath.Abs failed path=%q err=%v", goPath, err)
			return nil, fmt.Errorf("resolving %s: %w", goPath, err)
		}
		overlay[absGoPath] = analysisSrc
		emitSources[absGoPath] = emitSrc
		vv("GOWN prepared generated source goPath=%q abs=%q analysisBytes=%d emitBytes=%d", goPath, absGoPath, len(analysisSrc), len(emitSrc))
		if firstOverlayPath == "" {
			firstOverlayPath = absGoPath
		}
	}

	if len(gownNames) == 0 {
		vv("GOWN AnalyzeWithOptions return: no .gown files after discovery path=%q", gp.path)
		return nil, fmt.Errorf("no .gown files found in %s", gp.path)
	}

	if len(gp.files) == 0 {
		vv("GOWN AnalyzeWithOptions return: no gownFile objects path=%q gownNames=%d", gp.path, len(gownNames))
		return nil, fmt.Errorf("no .gown files found in %s", gp.path)
	}
	vv("got to here, have len gownNames[len %v] = '%#v'; needIntrinsicHelpers = %v", len(gownNames), gownNames, needIntrinsicHelpers)

	if needIntrinsicHelpers && firstOverlayPath != "" {
		overlay[firstOverlayPath] = append(append([]byte(nil), overlay[firstOverlayPath]...), []byte(intrinsicAnalysisHelperDecls())...)
		vv("GOWN appended intrinsic analysis helpers firstOverlayPath=%q overlayBytes=%d", firstOverlayPath, len(overlay[firstOverlayPath])) // check.go:147 [goID 1] 2026-06-01 02:31:38.196213381 +0000 UTC GOWN appended intrinsic analysis helpers firstOverlayPath="/home/jaten/go/src/github.com/glycerine/gown/vectors/linkedlist00/main.go" overlayBytes=3089
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
	vv("GOWN packages.Load begin dir=%q overlay=%d", cfg.Dir, len(cfg.Overlay)) // check.go:159 [goID 1] 2026-06-01 02:31:38.196229752 +0000 UTC GOWN packages.Load begin dir="." overlay=1
	for path, src := range cfg.Overlay {
		_, statErr := os.Stat(path)
		vv("GOWN packages.Load overlay path=%q bytes=%d diskExists=%v statErr=%v", path, len(src), statErr == nil, statErr) // check.go:162 [goID 1] 2026-06-01 02:31:38.196248938 +0000 UTC GOWN packages.Load overlay path="/home/jaten/go/src/github.com/glycerine/gown/vectors/linkedlist00/main.go" bytes=3089 diskExists=false statErr=stat /home/jaten/go/src/github.com/glycerine/gown/vectors/linkedlist00/main.go: no such file or directory
	}
	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		vv("GOWN AnalyzeWithOptions return: packages.Load failed dir=%q err=%v", cfg.Dir, err)
		return nil, fmt.Errorf("packages.Load: %w", err)
	}
	vv("GOWN packages.Load complete dir=%q packages=%d", cfg.Dir, len(pkgs)) // check.go:169 [goID 1] 2026-06-01 02:31:38.209256387 +0000 UTC GOWN packages.Load complete dir="." packages=1
	if len(pkgs) == 0 {
		vv("GOWN AnalyzeWithOptions return: packages.Load returned zero packages dir=%q", cfg.Dir) // not seen.
		return nil, fmt.Errorf("no packages found in %s", gp.path)
	}
	gp.pkg = pkgs[0]
	vv("GOWN package selected name=%q id=%q syntax=%d errors=%d", gp.pkg.Name, gp.pkg.ID, len(gp.pkg.Syntax), len(gp.pkg.Errors)) // check.go:175 [goID 1] 2026-06-01 02:31:38.209275945 +0000 UTC GOWN package selected name="" id="." syntax=0 errors=1
	if len(gp.pkg.Errors) > 0 {
		for i, pkgErr := range gp.pkg.Errors {
			vv("GOWN package error[%d]: %v", i, pkgErr) // check.go:177 [goID 1] 2026-06-01 02:26:11.292790589 +0000 UTC GOWN package error[0]: -: no Go files in /mnt/oldrog/home/jaten/go/src/github.com/glycerine/gown/vectors/linkedlist00
		}
		vv("GOWN AnalyzeWithOptions return: first package error=%v", gp.pkg.Errors[0]) // check.go:179 [goID 1] 2026-06-01 02:26:11.292813934 +0000 UTC GOWN AnalyzeWithOptions return: first package error=-: no Go files in /mnt/oldrog/home/jaten/go/src/github.com/glycerine/gown/vectors/linkedlist00
		return nil, fmt.Errorf("package error: %v", gp.pkg.Errors[0])
	}
	vv("len gp.pkg.Errors == 0. assignCapabilities begin files=%d", len(gp.files))
	gp.caps = assignCapabilities(gp.pkg, gp.files)
	vv("GOWN assignCapabilities complete")
	vv("GOWN buildSSA begin")
	if err := gp.buildSSA(); err != nil {
		vv("GOWN AnalyzeWithOptions return: buildSSA failed err=%v", err)
		return nil, err
	}
	vv("GOWN buildSSA complete ssaPkgNil=%v", gp.ssaPkg == nil)

	for _, gf := range gp.files {
		//vv("GOWN assignRegions begin file=%q", gf.path)
		assignRegions(gp.pkg, gf)
		//vv("GOWN assignBoundary begin file=%q", gf.path)
		assignBoundary(gp.pkg, gf)
		//vv("GOWN region/boundary complete file=%q boundary=%d", gf.path, len(gf.boundary))
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

	//vv("GOWN computeReachableTypes begin boundary=%d isoTypes=%d", len(allBoundary), len(isoTypes))
	reachable, poisoned := computeReachableTypes(allBoundary, isoTypes)
	//vv("GOWN computeReachableTypes complete reachable=%d poisoned=%v", len(reachable), poisoned)

	for _, gf := range gp.files {
		//vv("GOWN assignCreates begin file=%q", gf.path)
		assignCreates(gp.pkg, gf, reachable, poisoned)
		//vv("GOWN assignCreates complete file=%q creates=%d", gf.path, len(gf.create))
	}

	//vv("GOWN runCheckerPasses begin")
	if errs := runCheckerPasses(gp.pkg, gp.ssaPkg, gp.caps); len(errs) > 0 {
		//vv("GOWN AnalyzeWithOptions return: checker errors=%d first=%v", len(errs), errs[0])
		return gp.analysis(), errs
	}
	//vv("GOWN runCheckerPasses complete no errors")

	if !opts.CheckOnly {
		for _, gf := range gp.files {
			goPath := filepath.Join(gp.path, generatedGoName(gf.path))
			absGoPath, err := filepath.Abs(goPath)
			if err != nil {
				//vv("GOWN AnalyzeWithOptions return: filepath.Abs emit failed path=%q err=%v", goPath, err)
				return nil, fmt.Errorf("resolving %s: %w", goPath, err)
			}
			emitSrc := emitSources[absGoPath]
			//vv("GOWN buildEmitSource begin file=%q goPath=%q emitBytes=%d", gf.path, goPath, len(emitSrc))
			finalSrc, err := buildEmitSource(gp.pkg, gp.caps, gf, emitSrc)
			if err != nil {
				//vv("GOWN AnalyzeWithOptions return: buildEmitSource failed file=%q err=%v", gf.path, err)
				return nil, err
			}
			//vv("GOWN write generated .go begin path=%q bytes=%d", goPath, len(finalSrc))
			if err := os.WriteFile(goPath, finalSrc, 0644); err != nil {
				//vv("GOWN AnalyzeWithOptions return: os.WriteFile failed path=%q err=%v", goPath, err)
				return nil, fmt.Errorf("writing %s: %w", goPath, err)
			}
			//vv("GOWN write generated .go complete path=%q", goPath)
		}
	}

	//vv("GOWN AnalyzeWithOptions success path=%q", gp.path)
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
func swap_(xs ...any) {}
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
