package main

import "unicode/utf8"

type lineMap struct {
	text      []byte
	lineStart []int
}

func newLineMap(text []byte) *lineMap {
	starts := []int{0}
	for i, b := range text {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return &lineMap{text: text, lineStart: starts}
}

func (m *lineMap) offset(pos position) int {
	if m == nil {
		return 0
	}
	if pos.Line < 0 {
		return 0
	}
	if pos.Line >= len(m.lineStart) {
		return len(m.text)
	}
	off := m.lineStart[pos.Line]
	end := len(m.text)
	if pos.Line+1 < len(m.lineStart) {
		end = m.lineStart[pos.Line+1]
	}
	units := 0
	for off < end && units < pos.Character {
		r, size := utf8.DecodeRune(m.text[off:end])
		if r == utf8.RuneError && size == 0 {
			break
		}
		if r == '\n' || r == '\r' {
			break
		}
		if r <= 0xffff {
			units++
		} else {
			units += 2
		}
		off += size
	}
	return off
}

func (m *lineMap) position(offset int) position {
	if m == nil {
		return position{}
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(m.text) {
		offset = len(m.text)
	}
	line := 0
	lo, hi := 0, len(m.lineStart)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		if m.lineStart[mid] <= offset {
			line = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	char := 0
	for off := m.lineStart[line]; off < offset; {
		r, size := utf8.DecodeRune(m.text[off:offset])
		if r == utf8.RuneError && size == 0 {
			break
		}
		if r <= 0xffff {
			char++
		} else {
			char += 2
		}
		off += size
	}
	return position{Line: line, Character: char}
}

func (m *lineMap) rangeForOffsets(start, end int) lspRange {
	return lspRange{Start: m.position(start), End: m.position(end)}
}
