package render

import (
	"testing"

	"github.com/fatih/color"
)

func TestSparkline_EmptyAndShort(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	if got := Sparkline(nil, 5); got != "     " {
		t.Errorf("empty input -> %q want 5 spaces", got)
	}
	if got := Sparkline([]int{50}, 5); got != "    ▄" {
		t.Errorf("short input padded wrong: %q", got)
	}
	if got := Sparkline([]int{0, 25, 50, 75, 100}, 5); got != "▁▂▄▆█" {
		t.Errorf("full ramp wrong: %q", got)
	}
}

func TestSparkline_TruncatesToWidth(t *testing.T) {
	color.NoColor = true
	defer func() { color.NoColor = false }()

	vals := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	got := Sparkline(vals, 3)
	if r := []rune(got); len(r) != 3 {
		t.Errorf("expected 3 runes, got %d (%q)", len(r), got)
	}
}

func TestSparkRuneFor_Bounds(t *testing.T) {
	if sparkRuneFor(-5) != sparkRunes[0] {
		t.Error("negative should clamp to lowest")
	}
	if sparkRuneFor(999) != sparkRunes[len(sparkRunes)-1] {
		t.Error("over 100 should clamp to highest")
	}
}
