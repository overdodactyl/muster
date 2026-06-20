package render

import "strings"

// Bar returns a colored utilization bar of the given character width.
// Uses 8-block sub-character resolution (▏▎▍▌▋▊▉█) for smooth progression,
// and colors the filled portion by threshold: green <50%, yellow <80%, red >=80%.
//
//	Bar(50, 100, 10) -> "█████░░░░░"  (green)
//	Bar(85, 100, 10) -> "████████▌░"  (red, with partial block)
//	Bar(0, 0, 10)    -> "░░░░░░░░░░"  (no data, faint)
func Bar(used, total, width int) string {
	if width <= 0 {
		return ""
	}
	if total <= 0 {
		return ColorFaint(strings.Repeat("░", width))
	}
	if used < 0 {
		used = 0
	}
	if used > total {
		used = total
	}

	// 8 sub-cells per character → eighths.
	eighths := used * width * 8 / total
	full := eighths / 8
	partial := eighths % 8
	if full > width {
		full = width
		partial = 0
	}

	var b strings.Builder
	b.Grow(width)
	for i := 0; i < full; i++ {
		b.WriteRune('█')
	}
	if partial > 0 && full < width {
		b.WriteRune(partialBlocks[partial])
		full++
	}
	for i := full; i < width; i++ {
		b.WriteRune('░')
	}

	bar := b.String()
	pct := used * 100 / total
	switch {
	case pct >= 80:
		return ColorRed(bar)
	case pct >= 50:
		return ColorYellow(bar)
	default:
		return ColorGreen(bar)
	}
}

// partialBlocks indexed by eighths (1..7); 0 and 8 are handled by callers.
var partialBlocks = [...]rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉'}
