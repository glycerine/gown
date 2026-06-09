package gown

import (
	"bytes"
	"errors"
	"fmt"
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

type commentMirrorOrigin struct {
	path           string
	goSrc          []byte
	gownSrc        []byte
	edits          []EmitEdit
	goLineStarts   []int
	gownLineStarts []int
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

func (mirror *CommentMirror) RemapError(err error) error {
	if err == nil || mirror == nil || len(mirror.Lowered) == 0 {
		return err
	}
	origins := mirror.originAliases()
	if len(origins) == 0 {
		return err
	}

	switch typed := err.(type) {
	case CheckerErrors:
		return remapCheckerErrors(typed, origins)
	case CheckerError:
		return remapCheckerError(typed, origins)
	}

	var checkerErrs CheckerErrors
	if errors.As(err, &checkerErrs) {
		return remapCheckerErrors(checkerErrs, origins)
	}
	return err
}

func remapCheckerErrors(errs CheckerErrors, origins map[string]commentMirrorOrigin) CheckerErrors {
	out := make(CheckerErrors, len(errs))
	for i, err := range errs {
		out[i] = remapCheckerError(err, origins)
	}
	return out
}

func remapCheckerError(err CheckerError, origins map[string]commentMirrorOrigin) CheckerError {
	if origin, ok := lookupCommentMirrorOrigin(origins, err.Path); ok {
		err.Path, err.Offset, err.Line, err.Col = remapCommentMirrorLocation(origin, err.Offset, err.Line, err.Col)
	}
	for i, note := range err.Notes {
		if origin, ok := lookupCommentMirrorOrigin(origins, note.Path); ok {
			note.Path, note.Offset, note.Line, note.Col = remapCommentMirrorLocation(origin, note.Offset, note.Line, note.Col)
			err.Notes[i] = note
		}
	}
	return err
}

func (mirror *CommentMirror) originAliases() map[string]commentMirrorOrigin {
	origins := make(map[string]commentMirrorOrigin)
	for _, result := range mirror.Lowered {
		origin := commentMirrorOrigin{
			path:           result.Path,
			goSrc:          result.GoSrc,
			gownSrc:        result.GownSrc,
			edits:          cloneEmitEdits(result.Edits),
			goLineStarts:   buildLineStarts(result.GoSrc),
			gownLineStarts: buildLineStarts(result.GownSrc),
		}
		if len(origin.goSrc) == 0 {
			if src, err := os.ReadFile(origin.path); err == nil {
				origin.goSrc = src
				origin.goLineStarts = buildLineStarts(src)
			}
		}
		addCommentMirrorAlias(origins, result.GownPath, origin)
		addCommentMirrorAlias(origins, filepath.Join(mirror.Dir, generatedGoName(result.GownPath)), origin)
	}
	return origins
}

func addCommentMirrorAlias(origins map[string]commentMirrorOrigin, path string, origin commentMirrorOrigin) {
	if path == "" {
		return
	}
	origins[path] = origin
	if abs, err := filepath.Abs(path); err == nil {
		origins[abs] = origin
	}
	if real, err := canonicalFilePath(path); err == nil {
		origins[real] = origin
	}
}

func lookupCommentMirrorOrigin(origins map[string]commentMirrorOrigin, path string) (commentMirrorOrigin, bool) {
	if origin, ok := origins[path]; ok {
		return origin, true
	}
	if abs, err := filepath.Abs(path); err == nil {
		if origin, ok := origins[abs]; ok {
			return origin, true
		}
	}
	if real, err := canonicalFilePath(path); err == nil {
		if origin, ok := origins[real]; ok {
			return origin, true
		}
	}
	return commentMirrorOrigin{}, false
}

func remapCommentMirrorLocation(origin commentMirrorOrigin, offset, line, col int) (string, int, int, int) {
	if len(origin.goSrc) == 0 || len(origin.gownSrc) == 0 {
		return origin.path, offset, line, col
	}
	gownOffset, ok := commentMirrorGeneratedOffset(origin, offset, line, col)
	if !ok {
		return origin.path, offset, line, col
	}
	goOffset := originalOffsetForGeneratedOffset(gownOffset, origin.edits)
	if goOffset < 0 {
		goOffset = 0
	}
	if goOffset > len(origin.goSrc) {
		goOffset = len(origin.goSrc)
	}
	lc := lineColFromStarts(origin.goLineStarts, goOffset)
	return origin.path, goOffset, lc.line, lc.col
}

func commentMirrorGeneratedOffset(origin commentMirrorOrigin, offset, line, col int) (int, bool) {
	if offset > 0 || (offset == 0 && line == 1 && col == 1) {
		if offset >= 0 && offset <= len(origin.gownSrc) {
			return offset, true
		}
	}
	if line > 0 && col > 0 {
		return offsetForLineCol(origin.gownLineStarts, origin.gownSrc, line, col)
	}
	return 0, false
}

func offsetForLineCol(lineStarts []int, src []byte, line, col int) (int, bool) {
	if line <= 0 || line > len(lineStarts) {
		return 0, false
	}
	if col <= 0 {
		col = 1
	}
	lineStart := lineStarts[line-1]
	lineEnd := len(src)
	if line < len(lineStarts) {
		lineEnd = lineStarts[line] - 1
	}
	offset := lineStart + col - 1
	if offset > lineEnd {
		offset = lineEnd
	}
	return offset, true
}

func originalOffsetForGeneratedOffset(generatedOffset int, edits []EmitEdit) int {
	if len(edits) == 0 {
		return generatedOffset
	}
	sorted := cloneEmitEdits(edits)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Start < sorted[j].Start
	})
	delta := 0
	for _, edit := range sorted {
		oldLen := edit.End - edit.Start
		newLen := len(edit.NewText)
		generatedStart := edit.Start + delta
		generatedEnd := generatedStart + newLen
		if generatedOffset < generatedStart {
			return generatedOffset - delta
		}
		if generatedOffset < generatedEnd {
			relative := generatedOffset - generatedStart
			if relative > oldLen {
				relative = oldLen
			}
			return edit.Start + relative
		}
		delta += newLen - oldLen
	}
	return generatedOffset - delta
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
