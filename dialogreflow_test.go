package main

import "testing"

// fakeContent models what the real measurement reports: a grid of count boxes
// over n columns, next to content that does not change shape, the whole of it
// drawn at a font of points. Every size is proportional to the font size,
// which is what the real dialog does. walk derives the layout from the
// metrics of the font, so a dialog drawn at twice the point size is twice the
// size in both directions.
func fakeContent(count, box, spacing, boxHeight, otherWidth, otherHeight, drawnAt int) func(points, columns int) (int, int) {
	return func(points, columns int) (int, int) {
		if columns < 1 {
			columns = 1
		}

		scale := func(size int) int { return size * points / drawnAt }

		width := scale(columns*box + (columns-1)*spacing)
		if other := scale(otherWidth); width < other {
			width = other
		}

		return width, scale(otherHeight + mathCeilInInt(count, columns)*boxHeight)
	}
}

// The eight core machine this was reported on, drawn at the 8pt the system
// chose: three rows of boxes made the dialog taller than the screen.
func reporterContent() func(int, int) (int, int) {
	return fakeContent(8, 190, 6, 170, 600, 880, 8)
}

// Fixing the font at the size it was drawn at turns the measurement back into
// one of columns alone, which is what columnsThatFit is given.
func atDrawnSize(measure func(int, int) (int, int)) func(int) (int, int) {
	return func(columns int) (int, int) { return measure(8, columns) }
}

func TestColumnsThatFitLeavesAFittingShapeAlone(t *testing.T) {
	measure := atDrawnSize(reporterContent())

	// Plenty of height: the shape it was drawn as already fits, so keep it.
	if got, ok := columnsThatFit(3, 8, 4000, 4000, measure); got != 3 || !ok {
		t.Errorf("got %d columns, %v, want the 3 it was drawn as and a fit", got, ok)
	}
}

func TestColumnsThatFitNeverFoldsTighterThanItWasDrawn(t *testing.T) {
	measure := atDrawnSize(reporterContent())

	// One column fits this easily, but the boxes are laid out the way the
	// machine groups its cores and there is no reason to fold them tighter.
	if got, _ := columnsThatFit(3, 8, 4000, 4000, measure); got < 3 {
		t.Errorf("got %d columns, want no fewer than the 3 it was drawn as", got)
	}
}

func TestColumnsThatFitWidensUntilItFits(t *testing.T) {
	measure := atDrawnSize(reporterContent())

	// Three columns needs 880 + 3*170 = 1390 of height, which fits.
	if got, ok := columnsThatFit(3, 8, 4000, 1390, measure); got != 3 || !ok {
		t.Errorf("got %d columns, %v, at a height of 1390, want 3 and a fit", got, ok)
	}

	// 1300 does not fit three rows, but two rows (1220) does.
	got, ok := columnsThatFit(3, 8, 4000, 1300, measure)
	if got != 4 || !ok {
		t.Errorf("got %d columns, %v, at a height of 1300, want 4 and a fit", got, ok)
	}
	if _, height := measure(got); height > 1300 {
		t.Errorf("%d columns still needs %d of height", got, height)
	}
}

func TestColumnsThatFitWillNotOutgrowTheWidth(t *testing.T) {
	measure := atDrawnSize(reporterContent())

	// Short screen, so it wants every column it can get, but only 1000 wide.
	// Five columns needs 5*190 + 4*6 = 974; six needs 1170.
	got, ok := columnsThatFit(3, 8, 1000, 100, measure)
	if got != 5 {
		t.Errorf("got %d columns, want 5, the widest that fits 1000", got)
	}
	if ok {
		t.Error("reported a fit on a screen too short for any shape")
	}
	if width, _ := measure(got); width > 1000 {
		t.Errorf("%d columns is %d wide, over the 1000 limit", got, width)
	}
}

// The searches rely on the content never getting taller or narrower as columns
// are added, and never smaller as the font grows. If either stops holding they
// can stop at the wrong place.
func TestMeasurementIsMonotonic(t *testing.T) {
	for _, count := range []int{4, 8, 16, 32, 64} {
		measure := fakeContent(count, 190, 6, 170, 600, 880, 8)

		for points := 6; points <= 24; points++ {
			prevWidth, prevHeight := measure(points, 1)
			for columns := 2; columns <= count; columns++ {
				width, height := measure(points, columns)
				if width < prevWidth {
					t.Errorf("%d cores at %dpt: %d columns is narrower than %d columns", count, points, columns, columns-1)
				}
				if height > prevHeight {
					t.Errorf("%d cores at %dpt: %d columns is taller than %d columns", count, points, columns, columns-1)
				}
				prevWidth, prevHeight = width, height
			}
		}

		for columns := 1; columns <= count; columns++ {
			prevWidth, prevHeight := measure(6, columns)
			for points := 7; points <= 24; points++ {
				width, height := measure(points, columns)
				if width < prevWidth || height < prevHeight {
					t.Errorf("%d cores over %d columns: %dpt is smaller than %dpt", count, columns, points, points-1)
				}
				prevWidth, prevHeight = width, height
			}
		}
	}
}

// What the dialog was doing wrong: a bigger window left the content the size
// it was and padded it with empty space. Every extra bit of window has to end
// up as a bigger font, a wider spread of cores, or both.
func TestABiggerWindowDrawsABiggerDialog(t *testing.T) {
	measure := reporterContent()

	small, _ := largestThatFits(6, 24, 3, 8, 1200, 1400, measure)
	large, _ := largestThatFits(6, 24, 3, 8, 3840, 2160, measure)

	if large <= small {
		t.Errorf("a 3840x2160 window draws at %dpt and a 1200x1400 one at %dpt, want larger", large, small)
	}
}

func TestTheDialogIsDrawnAsLargeAsItFits(t *testing.T) {
	measure := reporterContent()

	points, columns := largestThatFits(6, 24, 3, 8, 3840, 2160, measure)

	if width, height := measure(points, columns); width > 3840 || height > 2160 {
		t.Errorf("%dpt over %d columns needs %dx%d, over the 3840x2160 it was given", points, columns, width, height)
	}

	// One point larger has to be too big for it, or it was not the largest.
	if _, ok := columnsThatFit(3, 8, 3840, 2160, func(c int) (int, int) { return measure(points+1, c) }); ok {
		t.Errorf("drew at %dpt when %dpt also fits 3840x2160", points, points+1)
	}
}

// Dragging a window out and back has to land on the shape it started at. The
// shape is a function of the size of the window and of nothing else, so there
// is nothing for a drag to ratchet.
func TestTheShapeDependsOnlyOnTheWindow(t *testing.T) {
	measure := reporterContent()

	sizes := []struct{ width, height int }{
		{1200, 1400}, {3840, 2160}, {800, 900}, {2560, 1440}, {1200, 1400},
	}

	first := map[int]dialogShape{}
	for pass := 0; pass < 2; pass++ {
		for i, size := range sizes {
			points, columns := largestThatFits(6, 24, 3, 8, size.width, size.height, measure)
			shape := dialogShape{points: points, columns: columns}

			if pass == 0 {
				first[i] = shape
				continue
			}

			if shape != first[i] {
				t.Errorf("%dx%d drew as %+v the first time and %+v the second", size.width, size.height, first[i], shape)
			}
		}
	}
}

// A window too small for even the smallest font gets that smallest font and
// the shape that leaves the least to be scrolled to, rather than nothing.
func TestAWindowTooSmallStillGetsTheSmallestDialog(t *testing.T) {
	measure := reporterContent()

	points, columns := largestThatFits(6, 24, 3, 8, 700, 300, measure)
	if points != 6 {
		t.Errorf("got %dpt, want the 6pt floor", points)
	}
	if columns < 3 {
		t.Errorf("got %d columns, want no fewer than the 3 it was drawn as", columns)
	}
}

// The range the font is scaled over is a fraction of whatever size the system
// chose rather than a point size of its own. A display scaled so that
// everything is large therefore gets the same headroom, proportionally, as one
// that is not, which is what keeps this from being tuned to one machine.
func TestFontSizeRange(t *testing.T) {
	tests := []struct{ points, smallest, largest int }{
		{points: 8, smallest: 6, largest: 24}, // walk's default, MS Shell Dlg 2 at 8pt
		{points: 9, smallest: 6, largest: 27}, // 6.75 truncated
		{points: 10, smallest: 7, largest: 30},
		{points: 12, smallest: 9, largest: 36},
		{points: 16, smallest: 12, largest: 48},
		{points: 1, smallest: 1, largest: 3}, // never below a point
		{points: 0, smallest: 1, largest: 1},
		{points: -3, smallest: 1, largest: 1},
	}

	for _, tt := range tests {
		if got := smallestFontSize(tt.points); got != tt.smallest {
			t.Errorf("smallestFontSize(%d) = %d, want %d", tt.points, got, tt.smallest)
		}
		if got := largestFontSize(tt.points); got != tt.largest {
			t.Errorf("largestFontSize(%d) = %d, want %d", tt.points, got, tt.largest)
		}
	}

	for points := 1; points <= 72; points++ {
		if got := smallestFontSize(points); got > points {
			t.Errorf("smallestFontSize(%d) = %d, which is larger than the size it was drawn at", points, got)
		}
		if got := largestFontSize(points); got < points {
			t.Errorf("largestFontSize(%d) = %d, which is smaller than the size it was drawn at", points, got)
		}
	}
}

// Each measurement is a layout pass over the real widget tree, and the search
// runs on every drag event, so it has to stay cheap on a machine with a core
// box for every column it could use.
func TestTheSearchStaysCheap(t *testing.T) {
	measure := fakeContent(32, 190, 6, 170, 600, 880, 8)

	calls := 0
	counted := func(points, columns int) (int, int) {
		calls++
		return measure(points, columns)
	}

	largestThatFits(6, 24, 4, 32, 3840, 2160, counted)

	// Both searches halve their range, so this is a handful of steps over the
	// font sizes times a handful over the column counts, not one measurement
	// per pair of them.
	if calls > 80 {
		t.Errorf("the search took %d measurements, want it to halve its ranges", calls)
	}
	t.Logf("%d measurements", calls)
}
