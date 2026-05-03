package gown

import "bytes"

type gownFile struct {
	path        string
	iso []*isoAnnotation
}

type region struct {
	beg  int // byte offset of '{' (inclusive)
	endx int // byte offset just past '}' (exclusive)
}

type isoAnnotation struct {
	offset   int     // 0-based byte offset of the '\' in \iso
	line     int     // 1-based line number
	col      int     // 1-based column number (bytes, not runes)
	funcName string  // containing function name (filled in after AST parse)
	scope    *region // innermost enclosing { } block (filled in after AST parse)
}

// scanAndStrip finds all \iso occurrences, records their positions,
// and replaces each \iso with spaces (preserving byte positions).
func scanAndStrip(path string, gownSrc []byte) (goSrc []byte, gf *gownFile) {
	tag := []byte(`\iso`)
	out := make([]byte, len(gownSrc))
	copy(out, gownSrc)
	gf = &gownFile{path: path}

	// Build a line-start offset table for fast line/col lookup.
	lineStarts := []int{0}
	for i, b := range gownSrc {
		if b == '\n' {
			lineStarts = append(lineStarts, i+1)
		}
	}

	search := gownSrc
	base := 0
	for {
		idx := bytes.Index(search, tag)
		if idx < 0 {
			break
		}
		absOffset := base + idx

		lo, hi := 0, len(lineStarts)-1
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if lineStarts[mid] <= absOffset {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		line := lo + 1                        // 1-based
		col := absOffset - lineStarts[lo] + 1 // 1-based

		gf.iso = append(gf.iso, &isoAnnotation{
			offset: absOffset,
			line:   line,
			col:    col,
		})
		for i := 0; i < len(tag); i++ {
			out[absOffset+i] = ' '
		}
		base = absOffset + len(tag)
		search = gownSrc[base:]
	}
	return out, gf
}
