package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Stdin, os.Stdout, os.Stderr))
}

func run(in io.Reader, out io.Writer, stderr io.Writer) int {
	srv := newServer(in, out, stderr)
	if err := srv.serve(); err != nil {
		fmt.Fprintf(stderr, "gownpls: %v\n", err)
		return 1
	}
	return 0
}
