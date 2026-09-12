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
	// font is the size the system chose, kept so that scaling always starts
	// from it rather than from whatever a previous pass left behind.
	font *walk.Font
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

// packCoreGrids gives the cores the shape the dialog opens at: the fewest
// columns that make it short enough for the screen. It runs once, when the
// dialog is built and again if the processor list is switched on, and the
// shape it picks is then the shape for good.
func packCoreGrids(c coreGrids, area walk.Size) {
	if c.empty() || area.Width <= 0 || area.Height <= 0 {
		return
	}

	// Start from the shape the layout heuristic chose, so a dialog that already
	// fits is left the way its author drew it.
	start := c.columns()
	logf("pack: fitting %d boxes into %s, starting from %d columns",
		mostChildren(c.grids), logSize(area), start)

	columns := fewestColumnsThatFit(start, mostChildren(c.grids), area.Width, area.Height, c.measure)
	c.apply(columns)
	logf("pack: chose %d columns", columns)
}

// dialogFontFloor is how far the dialog font may shrink, as a fraction of the
// size the system chose. Keeping the floor relative rather than naming a point
// size is what makes this behave the same at any display scaling: the font is
// only ever reduced when the content does not fit, and never below three
// quarters of what the user's own settings asked for.
const dialogFontFloor = 3.0 / 4.0

// scaleDialogToFit shrinks the dialog font until the content is short enough
// for the work area.
//
// This is the lever that actually suits the problem. Every size in the dialog
// is derived from the font, so reducing it scales the whole thing rather than
// reshaping any one part, and it stays sharp because the text is rendered at
// the smaller size instead of being stretched. It is also what the trouble
// usually is: at a high display scaling the content is too tall not because
// there is too much of it but because every control is drawn several times the
// size the layout was drawn around.
//
// At a scaling where everything already fits this does nothing at all, which
// is what keeps it right on displays other than the one it was measured on.
func scaleDialogToFit(c coreGrids, area walk.Size) {
	if c.empty() || c.font == nil || area.Height <= 0 {
		return
	}

	// Always start from the size the system chose, so running this again later
	// cannot shrink the dialog a second time.
	c.dlg.SetFont(c.font)
	pinContentWidth(c.body)

	family, style := c.font.Family(), c.font.Style()
	points := c.font.PointSize()

	for size := points; size >= smallestFontSize(points) && size > 0; size-- {
		font, err := walk.NewFont(family, size, style)
		if err != nil {
			log.Println(err)
			return
		}

		c.dlg.SetFont(font)
		pinContentWidth(c.body)

		height := contentDialogSize(c.dlg, c.scroll).Height
		logf("scale: %dpt of %dpt -> content %d tall, work area %d",
			size, points, height, area.Height)

		if height <= area.Height {
			return
		}
	}
}

// smallestFontSize is the smallest point size the dialog font may be reduced
// to, given the size the system chose. It is a fraction of that size rather
// than a fixed number of points, so a display whose scaling makes everything
// large has the same headroom proportionally as one that does not.
func smallestFontSize(points int) int {
	if points < 1 {
		return 1
	}

	smallest := int(float64(points) * dialogFontFloor)
	if smallest < 1 {
		smallest = 1
	}

	return smallest
}
