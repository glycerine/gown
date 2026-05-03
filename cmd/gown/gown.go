package main

import (
	"flag"
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

	myflags := flag.NewFlagSet("myflags", flag.ExitOnError)
	cfg := &Config{}
	cfg.DefineFlags(myflags)

	err := myflags.Parse(os.Args[1:])
	if err != nil {
		panicf("%s command line flag parse error: '%s'", ProgramName, err)
	}
	err = cfg.ValidateConfig()
	if err != nil {
		panicf("%s command line flag error: '%s'", ProgramName, err)
	}

	dirs := myflags.Args()
	if len(dirs) == 0 {
		panicf("must provide pacakges to typecheck as arguments.")
	}

	for _, dir := range dirs {
		gp := gown.NewGownPackage(dir)
		if err := gp.Check(); err != nil {
			panic(err)
		}
	}
}
