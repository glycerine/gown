package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"

	"github.com/glycerine/gown"
)

const ProgramName = "gown"

type Config struct {
	Path      string
	CheckOnly bool // true means do not overwrite/generate .go
	Propagate bool // true means rewrite .gown annotations before checking
	PrintGown bool // true means print virtual .gown sources for comment-mode .go
	Version   bool // true means print build info and exit
}

func (c *Config) DefineFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.CheckOnly, "check", false, "do not overwrite .go, only typecheck .gown")
	fs.BoolVar(&c.Propagate, "propagate", false, "force-propagate implied ownerstamps before checking")
	fs.BoolVar(&c.PrintGown, "print-gown", false, "print virtual .gown sources lowered from //gown: comments")
	fs.BoolVar(&c.Version, "version", false, "print build information and exit")
}

func (c *Config) ValidateConfig() error {
	return nil
}

func main() {
	os.Exit(runWithWriters(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	return runWithWriters(args, stderr, stderr)
}

func runWithWriters(args []string, stdout, stderr io.Writer) int {
	myflags := flag.NewFlagSet("myflags", flag.ContinueOnError)
	myflags.SetOutput(stderr)
	cfg := &Config{}
	cfg.DefineFlags(myflags)

	err := myflags.Parse(args)
	if err != nil {
		fmt.Fprintf(stderr, "%s command line flag parse error: '%s'\n", ProgramName, err)
		return 2
	}
	if cfg.Version {
		fmt.Fprint(stdout, buildInfoString())
		return 0
	}
	err = cfg.ValidateConfig()
	if err != nil {
		fmt.Fprintf(stderr, "%s command line flag error: '%s'\n", ProgramName, err)
		return 2
	}

	dirs := myflags.Args()
	if len(dirs) == 0 {
		fmt.Fprintln(stderr, "must provide packages to typecheck as arguments.")
		return 2
	}

	for _, dir := range dirs {
		if cfg.PrintGown {
			if err := gown.PrintCommentGownViews(dir, stdout); err != nil {
				fmt.Fprintln(stderr, gown.FormatError(err))
				return 1
			}
			continue
		}
		if cfg.Propagate {
			result, err := gown.ForcePropagateAnnotations(dir)
			if result != nil {
				if len(result.Edits) > 0 {
					fmt.Fprintf(stderr, "%s propagated %d annotation edit(s) in %s\n", ProgramName, len(result.Edits), dir)
				}
				for _, conflict := range result.Conflicts {
					fmt.Fprintf(stderr, "%s propagation conflict: %s\n", ProgramName, conflict)
				}
			}
			if err != nil {
				fmt.Fprintln(stderr, gown.FormatError(err))
				return 1
			}
		}
		gp := gown.NewGownPackage(dir)
		if err := gp.CheckWithOptions(gown.CheckOptions{CheckOnly: cfg.CheckOnly}); err != nil {
			fmt.Fprintln(stderr, gown.FormatError(err))
			return 1
		}
	}
	return 0
}

func buildInfoString() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "build info unavailable\n"
	}
	text := info.String()
	if len(text) == 0 || text[len(text)-1] != '\n' {
		text += "\n"
	}
	return text
}
