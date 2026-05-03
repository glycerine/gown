package gown

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type CheckerErrorCode string

const (
	GWN001 CheckerErrorCode = "GWN001"
	GWN002 CheckerErrorCode = "GWN002"
	GWN003 CheckerErrorCode = "GWN003"
	GWN004 CheckerErrorCode = "GWN004"
	GWN005 CheckerErrorCode = "GWN005"
	GWN006 CheckerErrorCode = "GWN006"
	GWN007 CheckerErrorCode = "GWN007"
	GWN008 CheckerErrorCode = "GWN008"
	GWN009 CheckerErrorCode = "GWN009"
	GWN010 CheckerErrorCode = "GWN010"
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

func FormatError(err error) string {
	if err == nil {
		return ""
	}
	var checkerErrs CheckerErrors
	if !errors.As(err, &checkerErrs) {
		return err.Error()
	}
	lines := make([]string, 0, len(checkerErrs)*3)
	for _, checkerErr := range checkerErrs {
		lines = append(lines, formatCheckerError(checkerErr)...)
	}
	return strings.Join(lines, "\n")
}

func formatCheckerError(err CheckerError) []string {
	lines := []string{err.Error()}
	sourceLine, ok := readSourceLine(err.Path, err.Line)
	if !ok {
		return lines
	}
	lines = append(lines, sourceLine)
	if err.Col > 0 {
		lines = append(lines, caretLine(sourceLine, err.Col))
	}
	return lines
}

func readSourceLine(path string, line int) (string, bool) {
	if path == "" || line <= 0 {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(data), "\n")
	if line > len(lines) {
		return "", false
	}
	return lines[line-1], true
}

func caretLine(sourceLine string, col int) string {
	if col <= 1 {
		return "^"
	}
	var b strings.Builder
	for i := 1; i < col; i++ {
		if i-1 < len(sourceLine) && sourceLine[i-1] == '\t' {
			b.WriteByte('\t')
		} else {
			b.WriteByte(' ')
		}
	}
	b.WriteByte('^')
	return b.String()
}

func gownSourcePath(path string) string {
	if strings.HasSuffix(path, ".go") {
		return strings.TrimSuffix(path, ".go") + ".gown"
	}
	return path
}
