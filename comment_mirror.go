package gown

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	gownMirrorDirName      = ".gown"
	gownMirrorManifestName = ".gown-manifest"
)

type CommentMirror struct {
	Dir     string
	Lowered []CommentLoweringResult
}

func PrintCommentGownViews(dir string, w io.Writer) error {
	mirror, ok, err := MaterializeCommentMirror(dir, CheckOptions{})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no %s comments found in %s", gownCommentPrefix, dir)
	}
	for i, result := range mirror.Lowered {
		if len(mirror.Lowered) > 1 {
			if i > 0 {
				fmt.Fprintln(w)
			}
			fmt.Fprintf(w, "// %s\n", result.GownPath)
		}
		if _, err := w.Write(result.GownSrc); err != nil {
			return err
		}
		if len(result.GownSrc) == 0 || result.GownSrc[len(result.GownSrc)-1] != '\n' {
			fmt.Fprintln(w)
		}
	}
	return nil
}

func MaterializeCommentMirror(dir string, opts CheckOptions) (*CommentMirror, bool, error) {
	pkgPath, err := canonicalPackagePath(dir)
	if err != nil {
		return nil, false, err
	}
	if filepath.Base(pkgPath) == gownMirrorDirName {
		return nil, false, nil
	}

	entries, err := os.ReadDir(pkgPath)
	if err != nil {
		return nil, false, err
	}

	gownSources := make(map[string][]byte)
	goSources := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(pkgPath, name)
		switch {
		case isGownSourceFileName(name):
			src, ok := lookupGownOverlay(opts.GownOverlay, path)
			if !ok {
				src, err = os.ReadFile(path)
				if err != nil {
					return nil, false, err
				}
			}
			gownSources[name] = src
		case isGoSourceFileName(name):
			src, ok := lookupGoOverlay(opts.GoOverlay, path)
			if !ok {
				src, err = os.ReadFile(path)
				if err != nil {
					return nil, false, err
				}
			}
			goSources[name] = src
		}
	}

	for path, src := range opts.GownOverlay {
		name := filepath.Base(path)
		if isGownSourceFileName(name) && samePackagePath(pkgPath, path) {
			gownSources[name] = src
		}
	}
	for path, src := range opts.GoOverlay {
		name := filepath.Base(path)
		if isGoSourceFileName(name) && samePackagePath(pkgPath, path) {
			goSources[name] = src
		}
	}

	var annotated []string
	for name, src := range goSources {
		if ContainsGownComment(src) {
			annotated = append(annotated, name)
		}
	}
	if len(annotated) == 0 {
		return nil, false, nil
	}
	sort.Strings(annotated)

	mirrorDir := filepath.Join(pkgPath, gownMirrorDirName)
	files := make(map[string][]byte)
	managed := make(map[string]bool)
	lowered := make([]CommentLoweringResult, 0, len(annotated))

	gownBases := make(map[string]string)
	for name := range gownSources {
		base := strings.TrimSuffix(name, ".gown")
		gownBases[base] = name
		files[name] = gownSources[name]
		managed[name] = true
		managed[generatedGoName(name)] = true
	}

	for name, src := range goSources {
		base := strings.TrimSuffix(name, ".go")
		if ContainsGownComment(src) {
			if existing, ok := gownBases[base]; ok {
				return nil, false, fmt.Errorf("%s and annotated %s both lower to %s in %s",
					existing, name, generatedGoName(existing), mirrorDir)
			}
			result, err := LowerGoCommentsToGown(filepath.Join(pkgPath, name), src)
			if err != nil {
				return nil, false, err
			}
			gownName := base + ".gown"
			result.GownPath = filepath.Join(mirrorDir, gownName)
			files[gownName] = result.GownSrc
			files[name] = src
			managed[gownName] = true
			managed[base+".go"] = true
			lowered = append(lowered, *result)
			continue
		}
		if _, shadowed := gownBases[base]; shadowed {
			continue
		}
		files[name] = src
		managed[name] = true
	}

	if err := syncCommentMirror(mirrorDir, files, managed); err != nil {
		return nil, false, err
	}
	sortLoweringResults(lowered)
	return &CommentMirror{Dir: mirrorDir, Lowered: lowered}, true, nil
}

func syncCommentMirror(mirrorDir string, files map[string][]byte, managed map[string]bool) error {
	if err := os.MkdirAll(mirrorDir, 0755); err != nil {
		return err
	}
	oldManaged, err := readMirrorManifest(filepath.Join(mirrorDir, gownMirrorManifestName))
	if err != nil {
		return err
	}
	for name := range oldManaged {
		if managed[name] {
			continue
		}
		if err := os.Remove(filepath.Join(mirrorDir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		path := filepath.Join(mirrorDir, name)
		if err := writeFileIfChanged(path, files[name]); err != nil {
			return err
		}
	}
	return writeMirrorManifest(filepath.Join(mirrorDir, gownMirrorManifestName), managed)
}

func readMirrorManifest(path string) (map[string]bool, error) {
	out := make(map[string]bool)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out[line] = true
		}
	}
	return out, nil
}

func writeMirrorManifest(path string, managed map[string]bool) error {
	names := make([]string, 0, len(managed))
	for name := range managed {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteByte('\n')
	}
	return writeFileIfChanged(path, []byte(b.String()))
}

func writeFileIfChanged(path string, src []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, src) {
		return nil
	}
	return os.WriteFile(path, src, 0644)
}

func isGoSourceFileName(name string) bool {
	if !strings.HasSuffix(name, ".go") {
		return false
	}
	base := filepath.Base(name)
	if strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") {
		return false
	}
	return true
}

func lookupGoOverlay(overlay map[string][]byte, path string) ([]byte, bool) {
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
	realPath, err := canonicalFilePath(path)
	if err == nil {
		if src, ok := overlay[realPath]; ok {
			return src, true
		}
		for overlayPath, src := range overlay {
			realOverlayPath, err := canonicalFilePath(overlayPath)
			if err == nil && realOverlayPath == realPath {
				return src, true
			}
		}
	}
	if src, ok := overlay[filepath.Base(path)]; ok {
		return src, true
	}
	return nil, false
}
