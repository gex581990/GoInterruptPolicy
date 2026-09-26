package main

import "testing"

// fakeContent models what the real measurement reports: a grid of count boxes
// over n columns, next to content that does not change shape.
func fakeContent(count, box, spacing, boxHeight, otherWidth, otherHeight int) func(columns int) (int, int) {
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
func reporterContent() func(int) (int, int) {
	return fakeContent(8, 190, 6, 170, 600, 880)
}

func TestColumnsThatFitLeavesAFittingShapeAlone(t *testing.T) {
	measure := reporterContent()

	// Plenty of height: the shape it was drawn as already fits, so keep it.
	if got, ok := columnsThatFit(3, 8, 4000, 4000, measure); got != 3 || !ok {
		t.Errorf("got %d columns, %v, want the 3 it was drawn as and a fit", got, ok)
	}
}

func TestColumnsThatFitNeverFoldsTighterThanItWasDrawn(t *testing.T) {
	measure := reporterContent()

	// One column fits this easily, but the boxes are laid out the way the
	// machine groups its cores and there is no reason to fold them tighter.
	if got, _ := columnsThatFit(3, 8, 4000, 4000, measure); got < 3 {
		t.Errorf("got %d columns, want no fewer than the 3 it was drawn as", got)
	}
}

func TestColumnsThatFitWidensUntilItFits(t *testing.T) {
	measure := reporterContent()

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
	measure := reporterContent()

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

// A screen too short for the boxes even at the widest they may go gets them at
// that widest and no wider, and is told they do not fit, so that the rest is
// left to the scroll bar rather than to a longer, thinner strip of boxes.
func TestColumnsThatFitStopsAtTheLimitAndLeavesTheRestToScroll(t *testing.T) {
	measure := reporterContent()

	// Two rows over four columns needs 1220 of height; the screen has 1000,
	// and eight columns in one row would fit it, but four is the limit.
	got, ok := columnsThatFit(3, 4, 4000, 1000, measure)
	if got != 4 {
		t.Errorf("got %d columns, want the limit of 4", got)
	}
	if ok {
		t.Error("reported a fit for a shape taller than the screen")
	}
}

// The limit on how wide the boxes may go holds even against the shape they
// were drawn as.
func TestColumnsThatFitKeepsToTheLimitOverTheDrawnShape(t *testing.T) {
	measure := reporterContent()

	if got, ok := columnsThatFit(6, 4, 4000, 4000, measure); got != 4 || !ok {
		t.Errorf("got %d columns, %v, from a shape drawn over 6 with a limit of 4, want 4 and a fit", got, ok)
	}
}

// The search relies on the content never getting taller or narrower as columns
// are added. If that stops holding it can stop at the wrong place.
func TestMeasurementIsMonotonic(t *testing.T) {
	for _, count := range []int{4, 8, 16, 32, 64} {
		measure := fakeContent(count, 190, 6, 170, 600, 880)

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

// The shape is a function of the screen and of nothing else, so opening the
// dialog twice on the same screen gives the same dialog, whatever happened in
// between.
func TestTheShapeDependsOnlyOnTheScreen(t *testing.T) {
	measure := reporterContent()

	screens := []struct{ width, height int }{
		{1200, 1400}, {3840, 2160}, {800, 900}, {2560, 1440}, {1200, 1400},
	}

	first := map[int]int{}
	for pass := 0; pass < 2; pass++ {
		for i, screen := range screens {
			columns, _ := columnsThatFit(3, 8, screen.width, screen.height, measure)

			if pass == 0 {
				first[i] = columns
				continue
			}

			if columns != first[i] {
				t.Errorf("%dx%d drew over %d columns the first time and %d the second", screen.width, screen.height, first[i], columns)
			}
		}
	}
}

// Each measurement is a layout pass over the real widget tree, rebuilding the
// layout items of every core box and thread checkbox in the dialog, so the
// search has to stay cheap on a machine with a core box for every column it
// could use.
func TestTheSearchStaysCheap(t *testing.T) {
	measure := fakeContent(32, 190, 6, 170, 600, 880)

	calls := 0
	counted := func(columns int) (int, int) {
		calls++
		return measure(columns)
	}

	// Too small for any shape, so both halves of the search run.
	columnsThatFit(4, 32, 1000, 1000, counted)

	// Both halves halve their range, so this is a handful of measurements
	// rather than one per column count.
	if calls > 16 {
		t.Errorf("the search took %d measurements, want it to halve its ranges", calls)
	}
	t.Logf("%d measurements", calls)
}

// boxesAcross is how many boxes the grids come to side by side, each laid out
// over the given number of rows.
func boxesAcross(counts []int, rows int) int {
	across := 0
	for _, count := range counts {
		across += mathCeilInInt(count, rows)
	}

	return across
}

// The limit worked through the fake processors this repository builds, grid by
// grid in the order the dialog lays them out: the fastest class first, and a
// grid for each cache group.
func TestMostColumnsKeepsTheCoresWithinTheLimit(t *testing.T) {
	tests := []struct {
		name   string
		counts []int
		rows   int // the fewest rows the limit allows
		across int // how many boxes across that comes to
	}{
		{name: "8 cores in one grid", counts: []int{8}, rows: 1, across: 8},
		{name: "6 cores in one grid", counts: []int{6}, rows: 1, across: 6},
		{name: "13900, 8 P-cores beside 16 E-cores", counts: []int{8, 16}, rows: 4, across: 6},
		{name: "13600KF, 6 P-cores beside 8 E-cores", counts: []int{6, 8}, rows: 2, across: 7},
		{name: "9950X3D, two cache groups of 8", counts: []int{8, 8}, rows: 2, across: 8},
		{name: "5900X, two cache groups of 6", counts: []int{6, 6}, rows: 2, across: 6},
		{name: "64 cores in one grid", counts: []int{64}, rows: 8, across: 8},
	}

	for _, tt := range tests {
		most := mostColumns(tt.counts, 8)
		rows := mathCeilInInt(tt.counts[0], most)

		if rows != tt.rows {
			t.Errorf("%s: at most %d columns is %d rows, want %d", tt.name, most, rows, tt.rows)
		}
		if across := boxesAcross(tt.counts, rows); across != tt.across {
			t.Errorf("%s: %d rows is %d across, want %d", tt.name, rows, across, tt.across)
		}
	}
}

// Whatever the grids and the limit, the answer is the widest the first grid can
// go: at the limit or under it, and one column more is over it.
func TestMostColumnsIsTheWidestWithinTheLimit(t *testing.T) {
	for first := 1; first <= 32; first++ {
		for second := 0; second <= 32; second += 4 {
			counts := []int{first}
			if second > 0 {
				counts = append(counts, second)
			}

			for limit := 1; limit <= 12; limit++ {
				most := mostColumns(counts, limit)
				if most < 1 || most > first {
					t.Fatalf("%v within %d: %d columns, want 1 to %d", counts, limit, most, first)
				}

				// Over the limit is only allowed when even one column is.
				if across := boxesAcross(counts, mathCeilInInt(first, most)); across > limit && boxesAcross(counts, first) <= limit {
					t.Errorf("%v within %d: %d columns comes to %d across", counts, limit, most, across)
				}

				if most < first {
					if across := boxesAcross(counts, mathCeilInInt(first, most+1)); across <= limit {
						t.Errorf("%v within %d: stopped at %d columns, but %d is only %d across", counts, limit, most, most+1, across)
					}
				}
			}
		}
	}
}

// A machine with more grids than the limit still gets a column for each.
func TestMostColumnsIsNeverLessThanOne(t *testing.T) {
	if got := mostColumns([]int{1, 1, 1, 1, 1, 1, 1, 1, 1}, 8); got != 1 {
		t.Errorf("nine grids of one within 8: got %d columns, want 1", got)
	}
	if got := mostColumns(nil, 8); got != 1 {
		t.Errorf("no grids: got %d columns, want 1", got)
	}
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
