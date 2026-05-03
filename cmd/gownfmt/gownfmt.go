package main

import (
	"os"

	"github.com/glycerine/gown"
)

func main() {
	os.Exit(gown.RunGownfmt(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
