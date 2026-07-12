package render

import "github.com/fatih/color"

// Theme controls a handful of colors that depend on whether the terminal
// background is dark or light. The default is dark; SetTheme("light")
// swaps in higher-contrast values for light-bg terminals.
type Theme string

const (
	ThemeDark  Theme = "dark"
	ThemeLight Theme = "light"
)

var theme Theme = ThemeDark

func SetTheme(t Theme)    { theme = t }
func CurrentTheme() Theme { return theme }

var (
	cRed    = color.New(color.FgRed).SprintFunc()
	cYellow = color.New(color.FgYellow).SprintFunc()
	cGreen  = color.New(color.FgGreen).SprintFunc()
	cCyan   = color.New(color.FgCyan).SprintFunc()
	cBold   = color.New(color.Bold).SprintFunc()
	cFaint  = color.New(color.Faint).SprintFunc()
	// Higher-contrast 'faint' for light themes — Faint just dims toward
	// the terminal bg, which is hard to read on light terminals.
	cFaintLight = color.New(color.FgHiBlack).SprintFunc()
)

func ColorState(class string) string {
	switch class {
	case "idle":
		return cGreen(class)
	case "mixed":
		return cYellow(class)
	case "alloc", "allocated":
		return cGreen(class)
	case "drain", "down", "fail", "not_responding":
		return cRed(class)
	case "reserved":
		return cYellow(class)
	default:
		return class
	}
}

// ColorCount tints a small count: 0 stays default, positive in a category-specific color.
func ColorRed(s string) string    { return cRed(s) }
func ColorYellow(s string) string { return cYellow(s) }
func ColorGreen(s string) string  { return cGreen(s) }
func ColorCyan(s string) string   { return cCyan(s) }
func ColorFaint(s string) string {
	if theme == ThemeLight {
		return cFaintLight(s)
	}
	return cFaint(s)
}
func Bold(s string) string { return cBold(s) }
