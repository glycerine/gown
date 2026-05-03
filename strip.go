package gown

import "bytes"

type isoAnnotation struct {
	offset int // 0-based byte offset of the '\' in \iso
	line   int // 1-based line number
	col    int // 1-based column number (bytes, not runes)
}

// scanAndStrip finds all \iso occurrences, records their positions,
// and replaces each \iso with spaces (preserving byte positions).
func scanAndStrip(gownSrc []byte) (goSrc []byte, annotations []isoAnnotation) {
	tag := []byte(`\iso`)
	out := make([]byte, len(gownSrc))
	copy(out, gownSrc)

	// Build a line-start offset table for fast line/col lookup.
	// lineStarts[i] is the byte offset where line i+1 begins.
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

		// Binary search for the line containing absOffset.
		lo, hi := 0, len(lineStarts)-1
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if lineStarts[mid] <= absOffset {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		line := lo + 1                       // 1-based
		col := absOffset - lineStarts[lo] + 1 // 1-based

		annotations = append(annotations, isoAnnotation{
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
	return out, annotations
}
