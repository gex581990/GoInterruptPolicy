package main

import (
	"log"

	"github.com/tailscale/walk"
)

// The core boxes are laid out over a number of columns, and that number is the
// one thing in this dialog worth changing to make it fit: height is the scarce
// dimension on any screen and width is the plentiful one, so three rows of
// boxes cost far more than the same boxes spread over one row.
//
// Both dimensions are monotonic in the column count. Adding a column never
// makes the content narrower and never makes it taller, which is what lets the
// two searches below stop at the first count that satisfies their bound.

// fewestColumnsThatFit returns the fewest columns, from start upwards, whose
// content is no taller than maxHeight, refusing to grow wider than maxWidth to
// get there. It returns the widest count that still fit the width if the
// content cannot be made short enough.
func fewestColumnsThatFit(start, most, maxWidth, maxHeight int, measure func(columns int) (width, height int)) int {
	if most < 1 {
		return 1
	}

	start = clampInt(start, 1, most)

	best := start
	for columns := start; columns <= most; columns++ {
		width, height := measure(columns)
		if width > maxWidth && columns > start {
			break
		}

		best = columns
		if height <= maxHeight {
			break
		}
	}

	return best
}

// widestColumnsWithin returns the most columns, up to limit, whose content is
// no wider than maxWidth. One column is the answer when even that does not fit,
// since the boxes have to go somewhere.
func widestColumnsWithin(limit, maxWidth int, measure func(columns int) (width, height int)) int {
	if limit < 1 {
		return 1
	}

	best := 1
	for columns := 1; columns <= limit; columns++ {
		if width, _ := measure(columns); width > maxWidth {
			break
		}

		best = columns
	}

	return best
}

// setGridColumns lays the children of a grid composite out over the given
// number of columns. walk builds a grid by giving every child a cell, and
// SetRange moves a child to another one, so the boxes themselves are untouched
// and keep whatever the user has ticked.
func setGridColumns(grid *walk.Composite, columns int) {
	if grid == nil || columns < 1 {
		return
	}

	layout, ok := grid.Layout().(*walk.GridLayout)
	if !ok {
		return
	}

	children := grid.Children()
	for i := 0; i < children.Len(); i++ {
		cell := walk.Rectangle{X: i % columns, Y: i / columns, Width: 1, Height: 1}
		if err := layout.SetRange(children.At(i), cell); err != nil {
			log.Println(err)
			return
		}
	}
}

// gridColumns is how many columns a grid is currently laid out over.
func gridColumns(grid *walk.Composite) int {
	if grid == nil {
		return 0
	}

	layout, ok := grid.Layout().(*walk.GridLayout)
	if !ok {
		return 0
	}

	children := grid.Children()

	columns := 0
	for i := 0; i < children.Len(); i++ {
		if cell, ok := layout.Range(children.At(i)); ok && cell.X+1 > columns {
			columns = cell.X + 1
		}
	}

	return columns
}

// mostChildren is the largest number of boxes any one grid holds.
func mostChildren(grids []*walk.Composite) int {
	most := 0
	for _, grid := range grids {
		if n := grid.Children().Len(); n > most {
			most = n
		}
	}

	return most
}

// coreGrids carries what the two searches need to try a column count on for
// size. Measuring goes through contentDialogSize, which walks the real widget
// tree, so margins, group box borders and the rest are all accounted for
// rather than estimated.
type coreGrids struct {
	dlg    *walk.Dialog
	scroll *walk.ScrollView
	body   *walk.Composite
	grids  []*walk.Composite
}

func (c coreGrids) empty() bool {
	return c.dlg == nil || c.scroll == nil || len(c.grids) == 0
}

func (c coreGrids) columns() int {
	if len(c.grids) == 0 {
		return 0
	}

	return gridColumns(c.grids[0])
}

// apply puts every grid on the given number of columns and re-pins the content
// column, which has to happen before measuring: a pin left over from another
// column count would cap the width the measurement reports.
func (c coreGrids) apply(columns int) {
	for _, grid := range c.grids {
		setGridColumns(grid, columns)
	}
	pinContentWidth(c.body)
}

// measure reports the outer size the dialog needs for a column count.
func (c coreGrids) measure(columns int) (width, height int) {
	c.apply(columns)
	size := contentDialogSize(c.dlg, c.scroll)
	logf("    try %2d columns -> content %s", columns, logSize(size))

	return size.Width, size.Height
}

// packCoreGrids gives the cores the shape the dialog will open at: the fewest
// columns that make it short enough for the screen. Returns that count, which
// is the shape resize then treats as the roomy end of the range.
func packCoreGrids(c coreGrids, area walk.Size) int {
	if c.empty() || area.Width <= 0 || area.Height <= 0 {
		return 0
	}

	// Start from the shape the layout heuristic chose, so a dialog that already
	// fits is left the way its author drew it.
	start := c.columns()
	logf("pack: fitting %d boxes into %s, starting from %d columns",
		mostChildren(c.grids), logSize(area), start)

	columns := fewestColumnsThatFit(start, mostChildren(c.grids), area.Width, area.Height, c.measure)
	c.apply(columns)
	logf("pack: chose %d columns", columns)

	return columns
}

// refitCoreGrids folds the cores onto more rows when the dialog is too narrow
// for the shape it opened at, and unfolds them again as it is widened back.
// Reports whether anything changed.
//
// This deliberately depends on the dialog width alone, and never on the
// scrollbar or on the content height. Those depend back on the column count,
// and letting them decide it is what made the layout change its mind while the
// window was being dragged.
func refitCoreGrids(c coreGrids, packed, outerWidth int) bool {
	if c.empty() || packed < 1 || outerWidth <= 0 {
		return false
	}

	before := c.columns()
	logf("refit: dialog is %d wide, packed shape was %d columns, currently %d",
		outerWidth, packed, before)

	columns := widestColumnsWithin(packed, outerWidth, c.measure)
	c.apply(columns)
	logf("refit: chose %d columns (%s)", columns,
		map[bool]string{true: "changed", false: "unchanged"}[columns != before])

	return columns != before
}
