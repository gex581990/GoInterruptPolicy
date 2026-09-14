package main

import (
	"log"
	"math"

	"github.com/tailscale/walk"
)

// The dialog is drawn at whatever size it has been given rather than at one
// fixed size with the leftovers left empty, and there are two levers for that.
//
// The font size is the main one. Every size in the layout is derived from it,
// so changing it scales the whole dialog at once, and the text stays sharp
// because it is rendered at the new size instead of being stretched. It is
// also expressed as a fraction of the size the system chose, never as a number
// of points of its own, which is what makes this behave the same at any
// display scaling: a point size is a request for a physical size and walk
// renders it at the dpi of the monitor the dialog is on.
//
// The other lever is how many columns the core boxes are laid out over, which
// trades the plentiful dimension for the scarce one. On any screen there is
// more width going spare than height, so three rows of boxes cost far more
// than the same boxes spread over one row, and spreading them is what lets a
// wider window be answered with larger text rather than with more empty space.
//
// Both levers are monotonic, and the two searches below lean on it. Content
// only ever grows with the font size, and adding a column never makes the
// content narrower and never makes it taller.

// dialogFontFloor is how far the dialog font may be reduced from the size the
// system chose, as a fraction of it, when the content does not fit the screen.
// It is never taken above that size: the point of the reduction is to fit a
// screen, not to second guess what the user asked their display to do.
const dialogFontFloor = 3.0 / 4.0

// smallestFontSize is the smallest the dialog font may be reduced to, given
// the size the system chose. Keeping it relative to that size rather than
// naming a point size is what gives a display whose scaling makes everything
// large the same headroom, proportionally, as one that does not.
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

// columnsThatFit returns the fewest columns, from start up to most, whose
// content fits inside maxWidth by maxHeight, and true. When nothing fits it
// returns the most columns that at least fit the width, and false, which is
// the shape that leaves the least of the content to be scrolled to.
//
// start is the shape the dialog was drawn as, and the search never goes below
// it: that shape came from the topology of the machine, laying the cores of a
// group out the way they are grouped, and there is nothing to be gained by
// folding it into a narrower column than its author asked for.
//
// Both halves halve their range each time rather than walking it, since each
// measurement is a layout pass over the real widget tree and a machine of many
// cores has many counts to choose between.
func columnsThatFit(start, most, maxWidth, maxHeight int, measure func(columns int) (width, height int)) (int, bool) {
	if most < 1 {
		most = 1
	}

	start = clampInt(start, 1, most)

	// The fewest columns short enough for the height. Adding a column never
	// makes the content taller, so once one count is short enough every count
	// above it is too.
	shortest := most
	for lo, hi := start, most; lo <= hi; {
		mid := lo + (hi-lo)/2
		if _, height := measure(mid); height <= maxHeight {
			shortest = mid
			hi = mid - 1
		} else {
			lo = mid + 1
		}
	}

	// Anything narrower than that is too tall and anything wider is wider
	// still, so this one count is the only candidate there is.
	if width, height := measure(shortest); height <= maxHeight && width <= maxWidth {
		return shortest, true
	}

	// Nothing fits. Adding a column never makes the content narrower, so the
	// widest count that at least fits the width is the shortest shape that can
	// be had without scrolling sideways as well.
	widest := start
	for lo, hi := start, most; lo <= hi; {
		mid := lo + (hi-lo)/2
		if width, _ := measure(mid); width <= maxWidth {
			widest = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	return widest, false
}

// largestThatFits returns the largest font size in [smallest, largest] whose
// content fits inside maxWidth by maxHeight, and the fewest columns that make
// it fit. A size that fits means every smaller one does too, so the range can
// be halved each time instead of walked. When not even smallest fits, smallest
// is returned with the shape that hides the least of the content.
func largestThatFits(smallest, largest, start, most, maxWidth, maxHeight int, measure func(points, columns int) (width, height int)) (points, columns int) {
	if smallest < 1 {
		smallest = 1
	}
	if largest < smallest {
		largest = smallest
	}

	fits := func(points int) (int, bool) {
		return columnsThatFit(start, most, maxWidth, maxHeight, func(columns int) (int, int) {
			return measure(points, columns)
		})
	}

	best := smallest
	for lo, hi := smallest, largest; lo <= hi; {
		mid := lo + (hi-lo)/2
		if _, ok := fits(mid); ok {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	columns, _ = fits(best)

	return best, columns
}

// gridMove is one box being given a cell.
type gridMove struct {
	box  int
	x, y int
}

// gridMoves is the sequence of cells to give count boxes so that they end up
// laid out over the given number of columns, whatever they were laid out over
// before.
//
// It has to be a sequence rather than just a mapping because of how walk moves
// a widget between cells: it empties the cell the widget is recorded in and
// then fills the new one, and it does not check that the cell it empties still
// holds that widget. Move each box straight to where it belongs and that
// emptying wipes boxes that have already been moved. Laying eight boxes that
// were over six columns out over three, the fourth box is put in the cell the
// seventh is still recorded as being in, so moving the seventh empties it
// again: the fourth and fifth boxes end up in no cell at all.
//
// A box in no cell is not laid out. It is not drawn, and it is not measured
// either, so the grid is reported shorter than it is and the dialog is then
// sized to that wrong answer. Which boxes are lost depends on what the grid
// was laid out over before, which made the dialog come out differently
// depending on the shapes the search had already tried on the way there.
//
// So every box is parked in a row of its own first and moved to where it
// belongs afterwards. No box's parking space is ever another box's old cell,
// and no box's final cell is ever another box's parking space, so neither pass
// can empty a cell that is holding something.
func gridMoves(count, columns int) []gridMove {
	if count < 1 || columns < 1 {
		return nil
	}

	moves := make([]gridMove, 0, count*2)
	for i := 0; i < count; i++ {
		moves = append(moves, gridMove{box: i, x: i, y: 0})
	}
	for i := 0; i < count; i++ {
		moves = append(moves, gridMove{box: i, x: i % columns, y: i / columns})
	}

	return moves
}

// setGridColumns lays the children of a grid composite out over the given
// number of columns. The boxes themselves are untouched and keep whatever the
// user has ticked.
func setGridColumns(grid *walk.Composite, columns int) {
	if grid == nil || columns < 1 {
		return
	}

	layout, ok := grid.Layout().(*walk.GridLayout)
	if !ok {
		return
	}

	children := grid.Children()
	for _, move := range gridMoves(children.Len(), columns) {
		cell := walk.Rectangle{X: move.x, Y: move.y, Width: 1, Height: 1}
		if err := layout.SetRange(children.At(move.box), cell); err != nil {
			// Carry on rather than return. The moves are two passes and every
			// box is parked in a row of its own between them, so stopping
			// half way through the second leaves the boxes that have not been
			// moved yet sitting in that parking row, which is a worse grid
			// than the one a single failed move gives.
			log.Println(err)
		}
	}
}

// rowsForColumns is how many rows the first grid has when it is laid out over
// the given number of columns. It is the first grid that the search picks a
// column count for, and this is what the other grids follow.
func rowsForColumns(grids []*walk.Composite, columns int) int {
	if len(grids) == 0 || columns < 1 {
		return 1
	}

	if rows := mathCeilInInt(grids[0].Children().Len(), columns); rows > 0 {
		return rows
	}

	return 1
}

// setGridRows lays every grid out over the number of columns that gives it the
// given number of rows.
//
// The grids are not laid out over the same number of columns as each other,
// and giving them one count is wrong. A machine whose cores come in more than
// one efficiency class gets a grid per class, and getLayout picks a single row
// count for the whole set and then gives each class the columns that fit its
// own cores into those rows, so that the classes line up beside one another. A
// six core class over two columns and an eight core class over three are both
// three rows tall; put both over two and the eight core class is four rows,
// taller than the machine was drawn to be and no longer level with its
// neighbour. So rows are what is held in common here, exactly as getLayout
// had it.
func setGridRows(grids []*walk.Composite, rows int) {
	if rows < 1 {
		rows = 1
	}

	for _, grid := range grids {
		setGridColumns(grid, mathCeilInInt(grid.Children().Len(), rows))
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

// dialogShape is a font size and a row count: what the dialog is actually laid
// out as, rather than what was asked for.
//
// The asking is in columns, and several column counts give one layout, since
// the rows they come to is ceil(boxes / columns) and that is many to one. On a
// machine of 32 core boxes drawn over 8 columns, the 25 counts the search can
// ask for are only 4 different grids. Naming a measurement by the layout it was
// taken of rather than by the number that was asked for is what stops the same
// grid being measured over and over.
type dialogShape struct {
	points int
	rows   int
}

// coreGrids is the dialog seen as something to be scaled. Measuring goes
// through contentDialogSize, which walks the real widget tree, so margins,
// group box borders, the font's own metrics and the rest are all accounted for
// rather than estimated.
type coreGrids struct {
	dlg    *walk.Dialog
	scroll *walk.ScrollView
	body   *walk.Composite
	grids  []*walk.Composite
	// font is the size the system chose. Every size tried is derived from it,
	// so scaling twice cannot compound, and the family and style the user's own
	// settings asked for are kept.
	font *walk.Font
	// columns is the shape the core boxes were drawn as, which came from how
	// the machine groups them. It is the narrowest shape the search will use,
	// so a dialog is never folded tighter than its author laid it out.
	columns int
}

// newCoreGrids takes the dialog as it was drawn, so call it before anything
// has changed the font or the shape of the core grids.
func newCoreGrids(dlg *walk.Dialog, scroll *walk.ScrollView, body *walk.Composite, grids []*walk.Composite) coreGrids {
	c := coreGrids{
		dlg:    dlg,
		scroll: scroll,
		body:   body,
		grids:  grids,
		font:   dlg.Font(),
	}

	if len(grids) > 0 {
		c.columns = gridColumns(grids[0])
	}

	return c
}

func (c coreGrids) empty() bool {
	return c.dlg == nil || c.scroll == nil || c.font == nil
}

// apply draws the dialog at the given shape and says whether it managed to. It
// does not ask for a layout, since the search runs this once per shape it tries
// on and only the last of them is the one to lay out.
func (c coreGrids) apply(points, columns int) bool {
	font, err := walk.NewFont(c.font.Family(), points, c.font.Style())
	if err != nil {
		// Stop rather than carry on with the rest. Measuring what is left is
		// measuring the font the previous shape was drawn at, and recording
		// that answer under this shape would have the search choose a size
		// from a measurement of a different one.
		log.Println(err)
		return false
	}

	// walk caches fonts by family, size and style, so this hands back the same
	// handle every time a size is tried again and there is nothing to dispose
	// of.
	c.dlg.SetFont(font)
	setGridRows(c.grids, rowsForColumns(c.grids, columns))

	// Re-cap the content column last: a cap left over from another shape would
	// hold the content at a width that shape wanted, and the measurement taken
	// next would report that width rather than this shape's own.
	pinContentWidth(c.body)

	return true
}

// fitDialogAtOpen lays the content out to fit a screen of the given work area,
// as large as it can be without going over the size the system chose.
//
// This is the only place the dialog is ever laid out differently. It runs
// before the dialog is shown, and again when the processor list is switched on
// or off, since that changes how much there is to lay out. It does not run in
// between: nothing watches the window, so resizing shows more of the content or
// less of it and changes nothing else.
//
// That is deliberate. Nothing here can scale the dialog: the only size that can
// be changed is the font, in whole points, while the margins and spacings
// around it are fixed in the layout and do not follow. Changing the font
// therefore lays the dialog out differently rather than magnifying it, in steps
// of about a tenth of its size, and the number of columns the cores are over
// steps as well. Doing that as a window is dragged would mean the shape of the
// thing changing under the user's hand with no way to drag back to what they
// had. Done when the content itself changes, it is just the dialog's size.
func fitDialogAtOpen(c coreGrids, area walk.Size) {
	if c.empty() || area.Width <= 0 || area.Height <= 0 {
		return
	}

	// What each layout measured, for the length of this search and no longer.
	// The two searches below re-probe counts they have already been through, so
	// remembering saves real work, but nothing outside wants these numbers and
	// a table that outlived the search would have to be invalidated by hand
	// every time the dialog changed.
	measured := map[dialogShape]walk.Size{}

	measure := func(points, columns int) (width, height int) {
		shape := dialogShape{points: points, rows: rowsForColumns(c.grids, columns)}
		if size, ok := measured[shape]; ok {
			return size.Width, size.Height
		}

		if !c.apply(points, columns) {
			// Leave the layout unmeasured rather than remember a size taken at
			// whatever the dialog is still drawn as, and report a size nothing
			// can hold. A layout that could not be drawn must not read as one
			// that fits perfectly, which is what a zero would do, so the search
			// steps away from it rather than settling on it.
			return math.MaxInt32, math.MaxInt32
		}

		size := contentDialogSize(c.dlg, c.scroll)
		measured[shape] = size

		return size.Width, size.Height
	}

	drawn := c.font.PointSize()
	points, columns := largestThatFits(smallestFontSize(drawn), drawn,
		c.columns, mostChildren(c.grids), area.Width, area.Height, measure)

	c.apply(points, columns)

	// Moving a widget to another cell does not ask for a layout on its own, and
	// the font may well be the one the last shape tried was measured at, so say
	// so here rather than leave the dialog drawn as whatever the search stopped
	// on. Before the dialog is up there is nothing to lay out yet, and asking
	// for one would only shrink the window to the layout minimum that the
	// opening size is about to replace.
	if c.dlg.Visible() {
		c.dlg.RequestLayout()
	}
}
