package cmd

import (
	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"os"
)

// isInteractive reports whether stdin is a TTY (so we may prompt).
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// readSecret prompts for a value with hidden input. Used for API keys so they
// never land in shell history or the process table.
func readSecret(prompt string) (string, error) {
	var v string
	err := huh.NewInput().
		Title(prompt).
		EchoMode(huh.EchoModePassword).
		Value(&v).
		Run()
	return v, err
}

// readLine prompts for a value with normal visible input. Used for typing
// non-secret reference URIs (env:VAR, keychain:svc/acc, op://..., cmd:...).
func readLine(prompt, placeholder string) (string, error) {
	var v string
	err := huh.NewInput().
		Title(prompt).
		Placeholder(placeholder).
		EchoMode(huh.EchoModeNormal).
		Value(&v).
		Run()
	return v, err
}

// confirm asks a yes/no question, defaulting to no.
func confirm(prompt string) (bool, error) {
	return confirmWithDefault(prompt, false)
}

// confirmWithDefault asks a yes/no question with a specified default selection.
func confirmWithDefault(prompt string, def bool) (bool, error) {
	ok := def
	err := huh.NewConfirm().Title(prompt).Affirmative("Yes").Negative("No").Value(&ok).Run()
	return ok, err
}
