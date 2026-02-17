// Package prompt provides interactive terminal prompts with TTY detection.
//
// When stdin is a TTY (interactive terminal), prompts are shown using huh.
// When stdin is not a TTY (CI, piped input), prompts return an error
// so the caller can require explicit flags/arguments instead.
package prompt

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

// IsInteractive reports whether stdin is a terminal.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ErrNonInteractive is returned when a prompt is attempted without a TTY.
var ErrNonInteractive = fmt.Errorf("required argument missing (not running interactively)")

// SelectOption is a single item in a select prompt.
type SelectOption struct {
	Label string
	Value string
}

// Select shows an interactive select list and returns the chosen value.
// Returns ErrNonInteractive if stdin is not a TTY.
func Select(title string, options []SelectOption) (string, error) {
	if !IsInteractive() {
		return "", ErrNonInteractive
	}

	huhOpts := make([]huh.Option[string], len(options))
	for i, o := range options {
		huhOpts[i] = huh.NewOption(o.Label, o.Value)
	}

	var selected string
	err := huh.NewSelect[string]().
		Title(title).
		Options(huhOpts...).
		Value(&selected).
		WithTheme(ThemeCNAP()).
		Run()
	if err != nil {
		return "", err
	}

	return selected, nil
}
