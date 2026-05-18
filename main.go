package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	"htb2kdl/internal/cli"
)

//go:embed style.css
var defaultStylesheet []byte

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
