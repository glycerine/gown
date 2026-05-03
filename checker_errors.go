package gown

import (
	"fmt"
	"strings"
)

type CheckerErrorCode string

const (
	GWN001 CheckerErrorCode = "GWN001"
)

type CheckerError struct {
	Code    CheckerErrorCode
	Path    string
	Offset  int
	Line    int
	Col     int
	Message string
}

func (err CheckerError) Error() string {
	loc := err.Path
	if err.Line != 0 || err.Col != 0 {
		loc = fmt.Sprintf("%s:%d:%d", err.Path, err.Line, err.Col)
	}
	if err.Message == "" {
		return fmt.Sprintf("%s: %s", loc, err.Code)
	}
	return fmt.Sprintf("%s: %s: %s", loc, err.Code, err.Message)
}

type CheckerErrors []CheckerError

func (errs CheckerErrors) Error() string {
	switch len(errs) {
	case 0:
		return ""
	case 1:
		return errs[0].Error()
	default:
		return fmt.Sprintf("%s and %d more checker errors", errs[0].Error(), len(errs)-1)
	}
}

func gownSourcePath(path string) string {
	if strings.HasSuffix(path, ".go") {
		return strings.TrimSuffix(path, ".go") + ".gown"
	}
	return path
}
