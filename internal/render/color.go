package render

import "github.com/fatih/color"

var (
	cRed    = color.New(color.FgRed).SprintFunc()
	cYellow = color.New(color.FgYellow).SprintFunc()
	cGreen  = color.New(color.FgGreen).SprintFunc()
	cCyan   = color.New(color.FgCyan).SprintFunc()
	cBold   = color.New(color.Bold).SprintFunc()
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
func Bold(s string) string        { return cBold(s) }
