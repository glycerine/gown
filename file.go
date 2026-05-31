package gown

import "go/types"

type gownFile struct {
	path          string
	annotations   []*AnnotationToken
	capQualifiers []*CapQualifierAnnotation
	intrinsics    []*IntrinsicAnnotation
	unsafeUses    []*UnsafeBoundaryAnnotation
	observers     []*ObserverAnnotation
	iso           []*isoAnnotation
	create        []*createAnew
	boundary      []*boundaryCrossing
}

type boundaryCrossing struct {
	offset   int        // 0-based byte offset
	line     int        // 1-based line number
	col      int        // 1-based column number
	kind     string     // "chan-send", "chan-recv", "go-arg", "go-capture", "pkg-var", "import-sig"
	typeName string     // human-readable type name
	goType   types.Type // resolved type from type checker
	funcName string     // containing function (empty for pkg-var)
	scope    *region    // innermost enclosing block
}

type createAnew struct {
	offset   int     // 0-based byte offset of the creation expression
	line     int     // 1-based line number
	col      int     // 1-based column number (bytes, not runes)
	kind     string  // "new", "make", or "ampersand" (for &T{})
	typeName string  // the type being created (e.g. "payload")
	funcName string  // containing function name (filled in after AST parse)
	scope    *region // innermost enclosing { } block (filled in after AST parse)
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
