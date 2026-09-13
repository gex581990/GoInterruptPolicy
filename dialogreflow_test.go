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

// A screen with room for the dialog as it was drawn gets it as it was drawn,
// and a screen without room gets it smaller. The size the system chose is the
// ceiling: fitting a screen is the whole point of the reduction, so there is
// never a reason to go above what the user's own settings asked for.
func TestTheDialogIsDrawnAsLargeAsItFits(t *testing.T) {
	measure := reporterContent()

	// 8pt over 8 columns is 1550x1050, which this screen has room for.
	points, columns := largestThatFits(6, 8, 3, 8, 3840, 2160, measure)
	if points != 8 {
		t.Errorf("got %dpt on a screen with room to spare, want the 8pt it was drawn at", points)
	}
	if width, height := measure(points, columns); width > 3840 || height > 2160 {
		t.Errorf("%dpt over %d columns needs %dx%d, over the 3840x2160 it was given", points, columns, width, height)
	}

	// A screen too short for it at the size it was drawn has to get it smaller.
	small, columns := largestThatFits(6, 8, 3, 8, 3840, 1000, measure)
	if small >= points {
		t.Errorf("got %dpt on a screen too short for %dpt, want smaller", small, points)
	}

	// And as large as that screen can take: one point more has to be too big.
	if _, ok := columnsThatFit(3, 8, 3840, 1000, func(c int) (int, int) { return measure(small+1, c) }); ok {
		t.Errorf("drew at %dpt when %dpt also fits 3840x1000", small, small+1)
	}
	if width, height := measure(small, columns); width > 3840 || height > 1000 {
		t.Errorf("%dpt over %d columns needs %dx%d, over the 3840x1000 it was given", small, columns, width, height)
	}
}

// The shape is a function of the screen and of nothing else, so opening the
// dialog twice on the same screen gives the same dialog, whatever happened in
// between.
func TestTheShapeDependsOnlyOnTheScreen(t *testing.T) {
	measure := reporterContent()

	screens := []struct{ width, height int }{
		{1200, 1400}, {3840, 2160}, {800, 900}, {2560, 1440}, {1200, 1400},
	}

	type drawnAs struct{ points, columns int }

	first := map[int]drawnAs{}
	for pass := 0; pass < 2; pass++ {
		for i, screen := range screens {
			points, columns := largestThatFits(6, 8, 3, 8, screen.width, screen.height, measure)
			shape := drawnAs{points: points, columns: columns}

			if pass == 0 {
				first[i] = shape
				continue
			}

			if shape != first[i] {
				t.Errorf("%dx%d drew as %+v the first time and %+v the second", screen.width, screen.height, first[i], shape)
			}
		}
	}
}

// A window too small for even the smallest font gets that smallest font and
// the shape that leaves the least to be scrolled to, rather than nothing.
func TestAWindowTooSmallStillGetsTheSmallestDialog(t *testing.T) {
	measure := reporterContent()

	points, columns := largestThatFits(6, 8, 3, 8, 700, 300, measure)
	if points != 6 {
		t.Errorf("got %dpt, want the 6pt floor", points)
	}
	if columns < 3 {
		t.Errorf("got %d columns, want no fewer than the 3 it was drawn as", columns)
	}
}

// How far the font may be reduced is a fraction of whatever size the system
// chose rather than a point size of its own. A display scaled so that
// everything is large therefore gets the same headroom, proportionally, as one
// that is not, which is what keeps this from being tuned to one machine.
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

	// Never larger than the size it was drawn at, or fitting a screen would
	// turn into magnifying the dialog past what the user asked for.
	for points := 1; points <= 72; points++ {
		if got := smallestFontSize(points); got > points {
			t.Errorf("smallestFontSize(%d) = %d, which is larger than the size it was drawn at", points, got)
		}
	}
}

// Each measurement is a layout pass over the real widget tree, rebuilding the
// layout items of every core box and thread checkbox in the dialog, so the
// search has to stay cheap on a machine with a core box for every column it
// could use.
func TestTheSearchStaysCheap(t *testing.T) {
	measure := fakeContent(32, 190, 6, 170, 600, 880, 8)

	calls := 0
	counted := func(points, columns int) (int, int) {
		calls++
		return measure(points, columns)
	}

	largestThatFits(6, 8, 4, 32, 3840, 2160, counted)

	// Both searches halve their range, so this is a handful of steps over the
	// font sizes times a handful over the column counts, not one measurement
	// per pair of them.
	if calls > 80 {
		t.Errorf("the search took %d measurements, want it to halve its ranges", calls)
	}
	t.Logf("%d measurements", calls)
}

// walkGrid models how walk moves a widget between cells: it empties the cell
// the widget is recorded in and then fills the new one, without checking that
// the cell it empties still holds that widget. A box whose cell was emptied by
// another box's move is in no cell at all, and a box in no cell is neither
// drawn nor measured.
type walkGrid struct {
	cell map[int][2]int // box -> the cell it is recorded in
	at   map[[2]int]int // cell -> the box laid out in it
}

func newWalkGrid() *walkGrid {
	return &walkGrid{cell: map[int][2]int{}, at: map[[2]int]int{}}
}

func (g *walkGrid) clone() *walkGrid {
	out := newWalkGrid()
	for k, v := range g.cell {
		out.cell[k] = v
	}
	for k, v := range g.at {
		out.at[k] = v
	}

	return out
}

func (g *walkGrid) run(moves []gridMove) {
	for _, move := range moves {
		if old, ok := g.cell[move.box]; ok {
			delete(g.at, old)
		}

		cell := [2]int{move.x, move.y}
		g.cell[move.box] = cell
		g.at[cell] = move.box
	}
}

// laidOut is where each box ended up, leaving out any that were lost.
func (g *walkGrid) laidOut() map[int][2]int {
	out := map[int][2]int{}
	for cell, box := range g.at {
		out[box] = cell
	}

	return out
}

// The bug this is all here for. Eight core boxes laid out over six columns and
// then over three, which is a pair of shapes the dialog really used, loses two
// of them if each box is moved straight to where it belongs.
func TestMovingBoxesStraightToTheirCellLosesThem(t *testing.T) {
	straight := func(count, columns int) []gridMove {
		moves := make([]gridMove, 0, count)
		for i := 0; i < count; i++ {
			moves = append(moves, gridMove{box: i, x: i % columns, y: i / columns})
		}

		return moves
	}

	grid := newWalkGrid()
	grid.run(straight(8, 6))
	grid.run(straight(8, 3))

	lost := []int{}
	for box := 0; box < 8; box++ {
		if _, ok := grid.laidOut()[box]; !ok {
			lost = append(lost, box)
		}
	}

	if len(lost) != 2 || lost[0] != 3 || lost[1] != 4 {
		t.Fatalf("boxes %v were lost, want the 3 and 4 that the reported layout was missing", lost)
	}
}

// Every box has to end up in the cell it belongs in, from every shape to every
// other shape. A box lost on the way is a box that is neither drawn nor
// measured, and a grid measured with a hole in it is what made the same window
// size come out differently depending on the sizes it had been dragged
// through.
func TestGridMovesNeverLoseABox(t *testing.T) {
	for _, count := range []int{1, 2, 3, 5, 8, 12, 16, 32, 64} {
		for from := 1; from <= count; from++ {
			start := newWalkGrid()
			start.run(gridMoves(count, from))

			for to := 1; to <= count; to++ {
				grid := start.clone()
				grid.run(gridMoves(count, to))

				laidOut := grid.laidOut()
				if len(laidOut) != count {
					t.Fatalf("%d boxes over %d columns laid out over %d: %d of them are in a cell",
						count, from, to, len(laidOut))
				}

				for box := 0; box < count; box++ {
					want := [2]int{box % to, box / to}
					if got := laidOut[box]; got != want {
						t.Fatalf("%d boxes over %d columns laid out over %d: box %d is at %v, want %v",
							count, from, to, box, got, want)
					}
				}
			}
		}
	}
}

// A freshly built grid has nothing in any cell yet, which is the one case the
// parking pass has nothing to protect against and still has to get right.
func TestGridMovesLayOutAFreshGrid(t *testing.T) {
	for _, columns := range []int{1, 3, 5, 8} {
		grid := newWalkGrid()
		grid.run(gridMoves(8, columns))

		for box := 0; box < 8; box++ {
			want := [2]int{box % columns, box / columns}
			if got := grid.laidOut()[box]; got != want {
				t.Errorf("over %d columns box %d is at %v, want %v", columns, box, got, want)
			}
		}
	}
}

func TestGridMovesOfNothing(t *testing.T) {
	if got := gridMoves(0, 3); got != nil {
		t.Errorf("gridMoves(0, 3) = %v, want no moves", got)
	}
	if got := gridMoves(8, 0); got != nil {
		t.Errorf("gridMoves(8, 0) = %v, want no moves", got)
	}
}
