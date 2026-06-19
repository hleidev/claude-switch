package main

import (
	"fmt"
	"os"

	"github.com/hleidev/claude-switch/cmd"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cmd.SetVersion(version)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
