package main

import "testing"

// fakeMeasure models what the real measurement reports: a grid of count boxes
// over n columns, next to content that does not change shape. Width is the
// wider of the grid and that other content; height is the other content plus a
// row of boxes per grid row.
func fakeMeasure(count, box, spacing, boxHeight, otherWidth, otherHeight int) func(int) (int, int) {
	return func(columns int) (int, int) {
		if columns < 1 {
			columns = 1
		}

		width := columns*box + (columns-1)*spacing
		if width < otherWidth {
			width = otherWidth
		}

		return width, otherHeight + mathCeilInInt(count, columns)*boxHeight
	}
}

// The eight core machine this was reported on: three rows of boxes made the
// dialog taller than the screen.
func reporterMeasure() func(int) (int, int) {
	return fakeMeasure(8, 190, 6, 170, 600, 880)
}

func TestFewestColumnsThatFitLeavesAFittingDialogAlone(t *testing.T) {
	measure := reporterMeasure()

	// Plenty of height: three columns already fits, so do not reshape it.
	if got := fewestColumnsThatFit(3, 8, 4000, 4000, measure); got != 3 {
		t.Errorf("got %d columns, want the 3 it started at", got)
	}
}

func TestFewestColumnsThatFitWidensUntilItFits(t *testing.T) {
	measure := reporterMeasure()

	// 1400 of height. Three columns needs 880 + 3*170 = 1390, which fits.
	if got := fewestColumnsThatFit(3, 8, 4000, 1390, measure); got != 3 {
		t.Errorf("got %d columns at a height of 1390, want 3", got)
	}

	// 1300 does not fit three rows, but two rows (1220) does.
	got := fewestColumnsThatFit(3, 8, 4000, 1300, measure)
	if got != 4 {
		t.Errorf("got %d columns at a height of 1300, want 4 so the boxes take two rows", got)
	}
	if _, height := measure(got); height > 1300 {
		t.Errorf("%d columns still needs %d of height", got, height)
	}
}

func TestFewestColumnsThatFitWillNotOutgrowTheWidth(t *testing.T) {
	measure := reporterMeasure()

	// Short screen, so it wants every column it can get, but only 1000 wide.
	// Five columns needs 5*190 + 4*6 = 974; six needs 1170.
	got := fewestColumnsThatFit(3, 8, 1000, 100, measure)
	if got != 5 {
		t.Errorf("got %d columns, want 5, the widest that fits 1000", got)
	}
	if width, _ := measure(got); width > 1000 {
		t.Errorf("%d columns is %d wide, over the 1000 limit", got, width)
	}
}

// Both searches rely on the content never getting taller or narrower as
// columns are added. If that stops holding they can stop at the wrong place.
func TestMeasurementIsMonotonicInColumns(t *testing.T) {
	for _, count := range []int{4, 8, 16, 32, 64} {
		measure := fakeMeasure(count, 190, 6, 170, 600, 880)

		prevWidth, prevHeight := measure(1)
		for columns := 2; columns <= count; columns++ {
			width, height := measure(columns)
			if width < prevWidth {
				t.Errorf("%d cores: %d columns is narrower than %d columns", count, columns, columns-1)
			}
			if height > prevHeight {
				t.Errorf("%d cores: %d columns is taller than %d columns", count, columns, columns-1)
			}
			prevWidth, prevHeight = width, height
		}
	}
}

// The font may only be reduced so far, and the limit is a fraction of whatever
// size the system chose rather than a point size of its own. A display scaled
// so that everything is large therefore gets the same headroom, proportionally,
// as one that is not, which is what keeps this from being tuned to one machine.
func TestSmallestFontSize(t *testing.T) {
	tests := []struct{ points, want int }{
		{points: 8, want: 6},  // walk's default, MS Shell Dlg 2 at 8pt
		{points: 9, want: 6},  // 6.75 truncated
		{points: 10, want: 7}, // 7.5 truncated
		{points: 12, want: 9},
		{points: 16, want: 12},
		{points: 1, want: 1}, // never below a point
		{points: 0, want: 1},
		{points: -3, want: 1},
	}

	for _, tt := range tests {
		if got := smallestFontSize(tt.points); got != tt.want {
			t.Errorf("smallestFontSize(%d) = %d, want %d", tt.points, got, tt.want)
		}
	}

	// Never larger than what it started from, or it would scale the dialog up.
	for points := 1; points <= 72; points++ {
		if got := smallestFontSize(points); got > points {
			t.Errorf("smallestFontSize(%d) = %d, which is larger than the size it started at", points, got)
		}
	}
}
