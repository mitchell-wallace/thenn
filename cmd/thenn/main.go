package main

import (
	"fmt"
	"os"

	"github.com/mitchell-wallace/thenn/internal/cmd"
)

var version = "dev"

func main() {
	if err := cmd.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
