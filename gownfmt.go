package gown

import (
	"bytes"
	"flag"
	"fmt"
	goformat "go/format"
	"io"
	"os"
)

// FormatGown formats Gown source while preserving capability annotations.
func FormatGown(src []byte) ([]byte, error) {
	return formatGownNamed("", src)
}

// FormatGownFile reads and formats one .gown file.
func FormatGownFile(path string) ([]byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return formatGownNamed(path, src)
}

// RunGownfmt is the library entry point used by cmd/gownfmt.
func RunGownfmt(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("gownfmt", flag.ContinueOnError)
	flags.SetOutput(stderr)
	write := flags.Bool("w", false, "write result to source file instead of stdout")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	paths := flags.Args()
	if len(paths) == 0 {
		if *write {
			fmt.Fprintln(stderr, "gownfmt: -w requires at least one file")
			return 2
		}
		src, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "gownfmt: %v\n", err)
			return 1
		}
		formatted, err := formatGownNamed("<standard input>", src)
		if err != nil {
			fmt.Fprintf(stderr, "gownfmt: %v\n", err)
			return 1
		}
		if _, err := stdout.Write(formatted); err != nil {
			fmt.Fprintf(stderr, "gownfmt: %v\n", err)
			return 1
		}
		return 0
	}

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(stderr, "gownfmt: %v\n", err)
			return 1
		}
		if info.IsDir() {
			fmt.Fprintf(stderr, "gownfmt: %s is a directory\n", path)
			return 1
		}

		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "gownfmt: %v\n", err)
			return 1
		}
		formatted, err := formatGownNamed(path, src)
		if err != nil {
			fmt.Fprintf(stderr, "gownfmt: %v\n", err)
			return 1
		}

		if *write {
			if err := os.WriteFile(path, formatted, info.Mode().Perm()); err != nil {
				fmt.Fprintf(stderr, "gownfmt: %v\n", err)
				return 1
			}
			continue
		}
		if _, err := stdout.Write(formatted); err != nil {
			fmt.Fprintf(stderr, "gownfmt: %v\n", err)
			return 1
		}
	}
	return 0
}

type gownfmtReplacement struct {
	marker string
	lexeme string
}

func formatGownNamed(path string, src []byte) ([]byte, error) {
	goSrc, replacements, err := gownfmtGoSource(path, src)
	if err != nil {
		return nil, err
	}

	formatted, err := goformat.Source(goSrc)
	if err != nil {
		if path != "" {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return nil, err
	}
	for _, repl := range replacements {
		formatted = bytes.ReplaceAll(formatted, []byte(repl.marker), []byte(repl.lexeme))
	}
	return formatted, nil
}

func gownfmtGoSource(path string, src []byte) ([]byte, []gownfmtReplacement, error) {
	var out bytes.Buffer
	out.Grow(len(src))
	prefix := gownfmtMarkerPrefix(src)
	lineStarts := buildLineStarts(src)
	var replacements []gownfmtReplacement

	for i := 0; i < len(src); {
		switch src[i] {
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				next := skipLineComment(src, i+2)
				out.Write(src[i:next])
				i = next
				continue
			}
			if i+1 < len(src) && src[i+1] == '*' {
				next := skipBlockComment(src, i+2)
				out.Write(src[i:next])
				i = next
				continue
			}
		case '"':
			next := skipQuoted(src, i+1, '"')
			out.Write(src[i:next])
			i = next
			continue
		case '\'':
			next := skipQuoted(src, i+1, '\'')
			out.Write(src[i:next])
			i = next
			continue
		case '`':
			next := skipRawString(src, i+1)
			out.Write(src[i:next])
			i = next
			continue
		case '\\':
			next, err := writeGownfmtToken(path, src, lineStarts, prefix, &out, &replacements, i)
			if err != nil {
				return nil, nil, err
			}
			i = next
			continue
		}
		out.WriteByte(src[i])
		i++
	}

	return out.Bytes(), replacements, nil
}

func writeGownfmtToken(path string, src []byte, lineStarts []int, prefix string, out *bytes.Buffer, replacements *[]gownfmtReplacement, offset int) (int, error) {
	end := offset + 1
	if end >= len(src) || !isIdentStart(src[end]) {
		return offset + 1, gownfmtError(path, lineStarts, offset, "invalid Gown token")
	}
	for end < len(src) && isIdentPart(src[end]) {
		end++
	}

	lexeme := string(src[offset:end])
	name := lexeme[1:]
	hasCall := followedByCall(src, end)

	switch name {
	case "iso", "imm":
		if hasCall {
			return end, gownfmtError(path, lineStarts, offset, fmt.Sprintf(`%s is a type qualifier, not an intrinsic`, lexeme))
		}
		out.WriteString(newGownfmtMarker(prefix, name, lexeme, true, replacements))
	case "mub", "rob":
		out.WriteString(newGownfmtMarker(prefix, name, lexeme, !hasCall, replacements))
	case "new", "clone", "freeze", "unsafe":
		if !hasCall {
			return end, gownfmtError(path, lineStarts, offset, fmt.Sprintf(`%s must be used as a call`, lexeme))
		}
		out.WriteString(newGownfmtMarker(prefix, name, lexeme, false, replacements))
	default:
		return end, gownfmtError(path, lineStarts, offset, fmt.Sprintf("unknown Gown token %q", lexeme))
	}
	return end, nil
}

func newGownfmtMarker(prefix, name, lexeme string, qualifier bool, replacements *[]gownfmtReplacement) string {
	base := fmt.Sprintf("%s%d_%s__", prefix, len(*replacements), name)
	marker := base
	if qualifier {
		marker = "/*" + base + "*/"
	}
	*replacements = append(*replacements, gownfmtReplacement{
		marker: marker,
		lexeme: lexeme,
	})
	return marker
}

func gownfmtMarkerPrefix(src []byte) string {
	for i := 0; ; i++ {
		prefix := fmt.Sprintf("__gownfmt_marker_%d_", i)
		if !bytes.Contains(src, []byte(prefix)) {
			return prefix
		}
	}
}

func gownfmtError(path string, lineStarts []int, offset int, msg string) error {
	lc := lineColFromStarts(lineStarts, offset)
	if path != "" {
		return fmt.Errorf("%s:%d:%d: %s", path, lc.line, lc.col, msg)
	}
	return fmt.Errorf("%d:%d: %s", lc.line, lc.col, msg)
}
