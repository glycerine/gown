package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/glycerine/gown"
)

const ProgramName = "gown"

type Config struct {
	Path      string
	CheckOnly bool // true means do not overwrite/generate .go
}

func (c *Config) DefineFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.CheckOnly, "check", false, "do not overwrite .go, only typecheck .gown")
}

func (c *Config) ValidateConfig() error {
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	myflags := flag.NewFlagSet("myflags", flag.ContinueOnError)
	myflags.SetOutput(stderr)
	cfg := &Config{}
	cfg.DefineFlags(myflags)

	err := myflags.Parse(args)
	if err != nil {
		fmt.Fprintf(stderr, "%s command line flag parse error: '%s'\n", ProgramName, err)
		return 2
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
		gp := gown.NewGownPackage(dir)
		if err := gp.Check(); err != nil {
			fmt.Fprintln(stderr, gown.FormatError(err))
			return 1
		}
	}
	return 0
}
