package render

import "strings"

var sparkRunes = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline renders a sequence of 0-100 values as an 8-level unicode bar
// sparkline of the given character width. If fewer samples than `width`
// exist, the line is left-padded with spaces so the most recent sample
// always sits at the right edge. The line is colored by the most recent
// value: green <50%, yellow <80%, red >=80%.
func Sparkline(values []int, width int) string {
	if width <= 0 {
		return ""
	}
	if len(values) == 0 {
		return ColorFaint(strings.Repeat(" ", width))
	}

	src := values
	if len(src) > width {
		src = src[len(src)-width:]
	}

	var b strings.Builder
	b.Grow(width)
	for i := 0; i < width-len(src); i++ {
		b.WriteRune(' ')
	}
	for _, v := range src {
		b.WriteRune(sparkRuneFor(v))
	}

	out := b.String()
	last := src[len(src)-1]
	switch {
	case last >= 80:
		return ColorRed(out)
	case last >= 50:
		return ColorYellow(out)
	default:
		return ColorGreen(out)
	}
}

func sparkRuneFor(v int) rune {
	if v <= 0 {
		return sparkRunes[0]
	}
	if v >= 100 {
		return sparkRunes[len(sparkRunes)-1]
	}
	idx := v * (len(sparkRunes) - 1) / 100
	return sparkRunes[idx]
}
