package prompt

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// ThemeCNAP returns a huh theme matching CNAP's brand identity.
//
// Design: borderless, minimal, with CNAP red as a subtle accent on the
// selector arrow only. The highlighted option is bold+bright while
// non-highlighted options recede into dim gray. This avoids the
// "error/danger" feel that saturated red text creates in terminals.
func ThemeCNAP() *huh.Theme {
	t := huh.ThemeBase()

	var (
		normalFg = lipgloss.AdaptiveColor{Light: "235", Dark: "252"}
		brightFg = lipgloss.AdaptiveColor{Light: "232", Dark: "255"}
		dimFg    = lipgloss.AdaptiveColor{Light: "245", Dark: "243"}
		subtleFg = lipgloss.AdaptiveColor{Light: "250", Dark: "238"}

		// Moderated CNAP red — warm and recognizable without screaming "error".
		accent = lipgloss.AdaptiveColor{Light: "#C04040", Dark: "#D85555"}
		green  = lipgloss.AdaptiveColor{Light: "#2A7A45", Dark: "#3DA060"}
	)

	// --- Focused state ---
	f := &t.Focused

	// Borderless: no left border, just padding for clean alignment.
	f.Base = lipgloss.NewStyle().PaddingLeft(1)
	f.Card = f.Base
	f.Title = f.Title.Foreground(normalFg).Bold(true)
	f.NoteTitle = f.NoteTitle.Foreground(normalFg).Bold(true).MarginBottom(1)
	f.Description = f.Description.Foreground(dimFg)
	f.ErrorIndicator = f.ErrorIndicator.Foreground(accent)
	f.ErrorMessage = f.ErrorMessage.Foreground(accent)

	// Select: ▸ selector in accent, highlighted item bold+bright, rest dim.
	f.SelectSelector = lipgloss.NewStyle().SetString("▸ ").Foreground(accent)
	f.SelectedOption = f.SelectedOption.Foreground(brightFg).Bold(true)
	f.UnselectedOption = f.UnselectedOption.Foreground(dimFg)
	f.Option = f.Option.Foreground(normalFg)
	f.NextIndicator = f.NextIndicator.Foreground(subtleFg)
	f.PrevIndicator = f.PrevIndicator.Foreground(subtleFg)

	// Multi-select: ● / ○ with accent + green.
	f.MultiSelectSelector = lipgloss.NewStyle().SetString("▸ ").Foreground(accent)
	f.SelectedOption = f.SelectedOption.Foreground(brightFg).Bold(true)
	f.SelectedPrefix = lipgloss.NewStyle().Foreground(green).SetString("● ")
	f.UnselectedPrefix = lipgloss.NewStyle().Foreground(dimFg).SetString("○ ")
	f.UnselectedOption = f.UnselectedOption.Foreground(dimFg)

	// Buttons.
	f.FocusedButton = f.FocusedButton.Foreground(lipgloss.Color("255")).Background(accent)
	f.Next = f.FocusedButton
	f.BlurredButton = f.BlurredButton.Foreground(normalFg).Background(lipgloss.AdaptiveColor{Light: "254", Dark: "236"})

	// File picker.
	f.Directory = f.Directory.Foreground(accent)

	// Text input.
	f.TextInput.Cursor = f.TextInput.Cursor.Foreground(accent)
	f.TextInput.Placeholder = f.TextInput.Placeholder.Foreground(lipgloss.AdaptiveColor{Light: "248", Dark: "238"})
	f.TextInput.Prompt = f.TextInput.Prompt.Foreground(accent)

	// --- Blurred state: same styles, no border ---
	t.Blurred = *f
	t.Blurred.Base = lipgloss.NewStyle().PaddingLeft(1)
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	// --- Group ---
	t.Group.Title = f.Title
	t.Group.Description = f.Description

	return t
}
