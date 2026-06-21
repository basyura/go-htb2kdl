package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	"htb2kdl/internal/cli"
)

// defaultStylesheet holds the CSS embedded into generated EPUB files when no
// external stylesheet is specified.
//
//go:embed style.css
var defaultStylesheet []byte

// main starts the CLI with process arguments and reports user-facing errors to
// stderr before exiting with a non-zero status.
func main() {
	if err := cli.Run(
		context.Background(),
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		cli.WithDefaultStylesheet(defaultStylesheet),
	); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
