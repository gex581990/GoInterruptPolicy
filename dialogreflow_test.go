package main

import "testing"

// fakeContent models what the real measurement reports: a grid of count boxes
// laid out over the given rows, next to content that does not change shape.
func fakeContent(count, box, spacing, boxHeight, otherWidth, otherHeight int) func(rows int) (int, int) {
	return fakeGrids([]int{count}, box, spacing, boxHeight, otherWidth, otherHeight)
}

// fakeGrids is fakeContent for grids side by side, each laid out over the
// columns the given rows give it, the way setGridRows lays them out.
func fakeGrids(counts []int, box, spacing, boxHeight, otherWidth, otherHeight int) func(rows int) (int, int) {
	return func(rows int) (int, int) {
		if rows < 1 {
			rows = 1
		}

		width, tallest := 0, 0
		for _, count := range counts {
			columns := mathCeilInInt(count, rows)
			width += columns*box + (columns-1)*spacing
			tallest = max(tallest, mathCeilInInt(count, columns))
		}
		width += (len(counts) - 1) * spacing

		return max(width, otherWidth), otherHeight + tallest*boxHeight
	}
}

// The eight core machine this was reported on, drawn in three rows of three:
// three rows of boxes made the dialog taller than the screen.
func reporterContent() func(int) (int, int) {
	return fakeContent(8, 190, 6, 170, 600, 880)
}

func TestRowsThatFitLeavesAFittingShapeAlone(t *testing.T) {
	measure := reporterContent()

	// Plenty of height: the shape it was drawn as already fits, so keep it.
	if got, ok := rowsThatFit(3, 1, 4000, 4000, measure); got != 3 || !ok {
		t.Errorf("got %d rows, %v, want the 3 it was drawn as and a fit", got, ok)
	}
}

func TestRowsThatFitNeverFoldsTighterThanItWasDrawn(t *testing.T) {
	measure := reporterContent()

	// Eight rows of one fit this easily, but the boxes are laid out the way
	// the machine groups its cores and there is no reason to fold them tighter.
	if got, _ := rowsThatFit(3, 1, 4000, 4000, measure); got > 3 {
		t.Errorf("got %d rows, want no more than the 3 it was drawn as", got)
	}
}

func TestRowsThatFitWidensUntilItFits(t *testing.T) {
	measure := reporterContent()

	// Three rows needs 880 + 3*170 = 1390 of height, which fits.
	if got, ok := rowsThatFit(3, 1, 4000, 1390, measure); got != 3 || !ok {
		t.Errorf("got %d rows, %v, at a height of 1390, want 3 and a fit", got, ok)
	}

	// 1300 does not fit three rows, but two rows (1220) does.
	got, ok := rowsThatFit(3, 1, 4000, 1300, measure)
	if got != 2 || !ok {
		t.Errorf("got %d rows, %v, at a height of 1300, want 2 and a fit", got, ok)
	}
	if _, height := measure(got); height > 1300 {
		t.Errorf("%d rows still needs %d of height", got, height)
	}
}

func TestRowsThatFitWillNotOutgrowTheWidth(t *testing.T) {
	measure := reporterContent()

	// Short screen, so it wants as few rows as it can get, but only 1000 wide.
	// One row is eight across, 8*190 + 7*6 = 1562; two rows is four across,
	// 4*190 + 3*6 = 778.
	got, ok := rowsThatFit(3, 1, 1000, 100, measure)
	if got != 2 {
		t.Errorf("got %d rows, want 2, the fewest that fit 1000", got)
	}
	if ok {
		t.Error("reported a fit on a screen too short for any shape")
	}
	if width, _ := measure(got); width > 1000 {
		t.Errorf("%d rows is %d wide, over the 1000 limit", got, width)
	}
}

// A screen too short for the boxes even at the widest they may go gets them at
// that widest and no wider, and is told they do not fit, so that the rest is
// left to the scroll bar rather than to a longer, thinner strip of boxes.
func TestRowsThatFitStopsAtTheLimitAndLeavesTheRestToScroll(t *testing.T) {
	measure := reporterContent()

	// Two rows of four needs 1220 of height and the screen has 1100. One row
	// of eight would fit it, at 1050, but two rows is the limit.
	got, ok := rowsThatFit(3, 2, 4000, 1100, measure)
	if got != 2 {
		t.Errorf("got %d rows, want the limit of 2", got)
	}
	if ok {
		t.Error("reported a fit for a shape taller than the screen")
	}
}

// The limit on how wide the boxes may go holds even against the shape they
// were drawn as.
func TestRowsThatFitKeepsToTheLimitOverTheDrawnShape(t *testing.T) {
	measure := reporterContent()

	if got, ok := rowsThatFit(1, 2, 4000, 4000, measure); got != 2 || !ok {
		t.Errorf("got %d rows, %v, from a shape drawn in 1 with a limit of 2, want 2 and a fit", got, ok)
	}
}

// The search relies on the content never getting taller or narrower as rows
// are taken away. This checks that the stand-in the other tests measure with
// keeps to that. The real measurement does because walk's grid counts the
// width of a column, and the height of a row, only when it holds something
// (gridLayoutItem.MinSizeForSize).
func TestMeasurementIsMonotonic(t *testing.T) {
	for _, counts := range [][]int{{4}, {8}, {16}, {32}, {64}, {2, 8}, {6, 8, 2}, {8, 16}} {
		measure := fakeGrids(counts, 190, 6, 170, 600, 880)

		most := 1
		for _, count := range counts {
			most = max(most, count)
		}

		prevWidth, prevHeight := measure(most)
		for rows := most - 1; rows >= 1; rows-- {
			width, height := measure(rows)
			if width < prevWidth {
				t.Errorf("%v: %d rows is narrower than %d rows", counts, rows, rows+1)
			}
			if height > prevHeight {
				t.Errorf("%v: %d rows is taller than %d rows", counts, rows, rows+1)
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
			rows, _ := rowsThatFit(3, 1, screen.width, screen.height, measure)

			if pass == 0 {
				first[i] = rows
				continue
			}

			if rows != first[i] {
				t.Errorf("%dx%d drew in %d rows the first time and %d the second", screen.width, screen.height, first[i], rows)
			}
		}
	}
}

// Each measurement is a layout pass over the real widget tree, rebuilding the
// layout items of every core box and thread checkbox in the dialog, so the
// search has to stay cheap on a machine with many row counts to choose from.
func TestTheSearchStaysCheap(t *testing.T) {
	measure := fakeContent(32, 190, 6, 170, 600, 880)

	calls := 0
	counted := func(rows int) (int, int) {
		calls++
		return measure(rows)
	}

	// Too small for any shape, so both halves of the search run.
	rowsThatFit(32, 1, 1000, 1000, counted)

	// Both halves halve their range, so this is a handful of measurements
	// rather than one per row count.
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
func TestFewestRowsKeepsTheCoresWithinTheLimit(t *testing.T) {
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
		{name: "6 P-cores and 8 E-cores beside 2 more E-cores", counts: []int{6, 8, 2}, rows: 2, across: 8},
		// Counted in the columns of the first grid, this could only be two
		// rows, 9 across, or one; in rows it is three, 7 across.
		{name: "2 P-cores beside 16 E-cores", counts: []int{2, 16}, rows: 3, across: 7},
	}

	for _, tt := range tests {
		rows := fewestRows(tt.counts, 8)

		if rows != tt.rows {
			t.Errorf("%s: got %d rows, want %d", tt.name, rows, tt.rows)
		}
		if across := boxesAcross(tt.counts, rows); across != tt.across {
			t.Errorf("%s: %d rows is %d across, want %d", tt.name, rows, across, tt.across)
		}
	}
}

// Whatever the grids and the limit, the answer is the fewest rows within it:
// at the limit or under it, and one row fewer is over it.
func TestFewestRowsIsTheFewestWithinTheLimit(t *testing.T) {
	for first := 1; first <= 32; first++ {
		for second := 0; second <= 32; second += 4 {
			counts := []int{first}
			if second > 0 {
				counts = append(counts, second)
			}
			most := max(first, second)

			for limit := 1; limit <= 12; limit++ {
				rows := fewestRows(counts, limit)
				if rows < 1 || rows > most {
					t.Fatalf("%v within %d: %d rows, want 1 to %d", counts, limit, rows, most)
				}

				// Over the limit is only allowed when even a single column
				// for every grid is.
				if across := boxesAcross(counts, rows); across > limit && boxesAcross(counts, most) <= limit {
					t.Errorf("%v within %d: %d rows comes to %d across", counts, limit, rows, across)
				}

				if rows > 1 {
					if across := boxesAcross(counts, rows-1); across <= limit {
						t.Errorf("%v within %d: stopped at %d rows, but %d is only %d across", counts, limit, rows, rows-1, across)
					}
				}
			}
		}
	}
}

// A machine with more grids than the limit still gets a column for each.
func TestFewestRowsWithMoreGridsThanTheLimit(t *testing.T) {
	if got := fewestRows([]int{1, 1, 1, 1, 1, 1, 1, 1, 1}, 8); got != 1 {
		t.Errorf("nine grids of one within 8: got %d rows, want 1", got)
	}
	if got := fewestRows([]int{3, 1, 1, 1, 1, 1, 1, 1, 1}, 8); got != 3 {
		t.Errorf("nine grids within 8: got %d rows, want every grid in one column, 3 rows", got)
	}
	if got := fewestRows(nil, 8); got != 1 {
		t.Errorf("no grids: got %d rows, want 1", got)
	}
}

// Laid out over the rows they were drawn in, the grids get back exactly the
// columns they were drawn with, whatever the row count getLayout chose. That
// is what lets the search start from the drawn shape.
func TestDrawnRowsGiveBackTheDrawnColumns(t *testing.T) {
	for first := 1; first <= 24; first++ {
		for second := 0; second <= 24; second++ {
			for third := 0; third <= 4; third += 2 {
				counts := []int{first}
				for _, n := range []int{second, third} {
					if n > 0 {
						counts = append(counts, n)
					}
				}

				for chosen := 1; chosen <= 12; chosen++ {
					columns := make([]int, len(counts))
					for i, count := range counts {
						columns[i] = mathCeilInInt(count, chosen)
					}

					rows := drawnRows(counts, columns)
					if rows > chosen {
						t.Fatalf("%v drawn for %d rows: drawnRows says %d", counts, chosen, rows)
					}
					for i, count := range counts {
						if got := mathCeilInInt(count, rows); got != columns[i] {
							t.Fatalf("%v drawn for %d rows: grid %d over %d rows is %d columns, drawn as %d", counts, chosen, i, rows, got, columns[i])
						}
					}
				}
			}
		}
	}
}

// Each processor drawn the way the dialog draws it: getLayout picks the rows
// from the threads of each efficiency class, and each grid is declared over
// the fewest columns that fit its cores into those rows. Starting from the
// rows those declared columns give, as newCoreGrids does, with room to spare
// the search keeps every grid exactly as declared.
func TestTheDrawnShapeOpensWhenItFits(t *testing.T) {
	tests := []struct {
		name    string
		threads []int // per efficiency class, the way getLayout is given them
		cores   []int // per grid, in the order the dialog lays them out
	}{
		// Counted in the P-core grid's single column, a search in columns
		// could only reach two rows or one, and two put the E-cores in four
		// columns, not three.
		{name: "i5-1235U, 2 P-cores and 8 E-cores", threads: []int{8, 4}, cores: []int{2, 8}},
		// The fake this repository builds. Counted in the P-core grid's two
		// declared columns, a search in columns could reach three rows or
		// fewer, never the four the first E-core grid was drawn in.
		{name: "6 P-cores and 8 E-cores beside 2 more E-cores", threads: []int{10, 12}, cores: []int{6, 8, 2}},
		{name: "13900, 8 P-cores beside 16 E-cores", threads: []int{16, 16}, cores: []int{8, 16}},
		{name: "13600KF, 6 P-cores beside 8 E-cores", threads: []int{8, 12}, cores: []int{6, 8}},
		// Two cache groups each declared over two columns, which walk's
		// builder leaves with every box in the first, so read back from the
		// grids this shape would come out as two columns of six.
		{name: "5900X, two cache groups of 6", threads: []int{24}, cores: []int{6, 6}},
		{name: "8 cores with hyper-threading", threads: []int{16}, cores: []int{8}},
	}

	for _, tt := range tests {
		chosen, _ := getLayout(tt.threads...)

		columns := make([]int, len(tt.cores))
		for i, count := range tt.cores {
			columns[i] = mathCeilInInt(count, chosen)
		}

		drawn := drawnRows(tt.cores, columns)
		fewest := fewestRows(tt.cores, maxCoresAcross)
		if fewest > drawn {
			t.Errorf("%s: drawn %d across, over the limit", tt.name, boxesAcross(tt.cores, drawn))
			continue
		}

		rows, ok := rowsThatFit(drawn, fewest, 100000, 100000, fakeGrids(tt.cores, 190, 6, 170, 600, 880))
		if !ok {
			t.Errorf("%s: no fit with room to spare", tt.name)
		}
		for i, count := range tt.cores {
			if got := mathCeilInInt(count, rows); got != columns[i] {
				t.Errorf("%s: grid %d opens in %d columns, drawn in %d", tt.name, i, got, columns[i])
			}
		}
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

// A grid with nothing in any cell yet is the one case the parking pass has
// nothing to protect against, and it still has to get it right. A grid
// straight out of walk's builder is not that case: see
// TestGridMovesLayOutAGridFromWalksBuilder.
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

// builderCells is where walk's declarative builder puts count boxes in a grid
// declared over columns when each box has a grid layout of its own, as a core
// box has: after such a box it carries on one column too far and one row down
// (declarative/builder.go, InitWidget).
func builderCells(count, columns int) *walkGrid {
	grid := newWalkGrid()

	row, col := 0, 0
	for box := 0; box < count; box++ {
		if col+1 > columns {
			row++
			col = 0
		}
		grid.run([]gridMove{{box: box, x: col, y: row}})

		row, col = row+1, col+2
	}

	return grid
}

// The dialog's first layout starts from the cells walk's builder put the core
// boxes in, which are not the cells they were declared in. Every box still
// ends up where it belongs.
func TestGridMovesLayOutAGridFromWalksBuilder(t *testing.T) {
	// Declared over two columns, every box lands in the first.
	for box, cell := range builderCells(4, 2).laidOut() {
		if cell[0] != 0 {
			t.Fatalf("box %d of a grid declared over two columns is in column %d, want every box in the first", box, cell[0])
		}
	}

	for _, count := range []int{1, 2, 3, 5, 8, 12, 16, 32, 64} {
		for declared := 1; declared <= count; declared++ {
			for to := 1; to <= count; to++ {
				grid := builderCells(count, declared)
				grid.run(gridMoves(count, to))

				laidOut := grid.laidOut()
				if len(laidOut) != count {
					t.Fatalf("%d boxes declared over %d columns, laid out over %d: %d of them are in a cell",
						count, declared, to, len(laidOut))
				}

				for box := 0; box < count; box++ {
					want := [2]int{box % to, box / to}
					if got := laidOut[box]; got != want {
						t.Fatalf("%d boxes declared over %d columns, laid out over %d: box %d is at %v, want %v",
							count, declared, to, box, got, want)
					}
				}
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
