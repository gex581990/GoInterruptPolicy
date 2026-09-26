package main

import (
	"log"

	"github.com/tailscale/walk"
)

// The dialog is fitted to the screen it opens on with one lever: how many rows
// the core boxes are laid out in. Fewer rows make the content shorter and
// wider, so on a screen short of height the boxes are spread out sideways
// until the dialog fits, or until they are as wide as they may go.
//
// The font is not a lever. It stays at the size the system chose, which is the
// size the user's own display settings asked for, and whatever still does not
// fit is scrolled to. Shrinking the text to save a scroll bar made the dialog
// hardest to read on exactly the machines with the most cores to read through.
//
// Adding a column never makes the content narrower and never makes it taller,
// and the search below leans on that.

// maxCoresAcross is the most core boxes the dialog will put side by side,
// counting every grid in the row together, since the grids of each efficiency
// class and of each cache group sit beside one another. The search stops
// spreading the boxes out here and leaves the rest to the scroll bar. Without
// it a screen short of height had them stretched into a long thin strip, two
// rows of twelve on a 13900, that looked nothing like the rest of the dialog.
// It is counted in boxes rather than pixels so that it holds at any display
// scaling.
const maxCoresAcross = 8

// columnsThatFit returns the fewest columns, from start up to most, whose
// content fits inside maxWidth by maxHeight, and true. When nothing fits it
// returns the most columns that at least fit the width, and false, which is
// the shape that leaves the least of the content to be scrolled to.
//
// start is the shape the dialog was drawn as, and the search never goes below
// it: that shape came from the topology of the machine, laying the cores of a
// group out the way they are grouped, and there is nothing to be gained by
// folding it into a narrower column than its author asked for. The one
// exception is a most below start, and then most wins: it is the limit on how
// wide the boxes may go, and that holds whatever they were drawn as.
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

// boxCounts is how many boxes each grid holds, in the order the dialog lays
// the grids out.
func boxCounts(grids []*walk.Composite) []int {
	counts := make([]int, len(grids))
	for i, grid := range grids {
		counts[i] = grid.Children().Len()
	}

	return counts
}

// mostColumns is the most columns the first grid can be laid out over without
// the grids, side by side at the row count that gives, coming to more than
// maxAcross boxes. counts is how many boxes each grid holds, the first grid
// first. It is never less than one, since a machine with more grids than
// maxAcross still has to be drawn somehow.
func mostColumns(counts []int, maxAcross int) int {
	most := 1
	if len(counts) == 0 {
		return most
	}

	// More columns means fewer rows, and fewer rows can only mean more boxes
	// across, so the first count over the limit ends the search.
	for columns := 1; columns <= counts[0]; columns++ {
		rows := mathCeilInInt(counts[0], columns)

		across := 0
		for _, count := range counts {
			across += mathCeilInInt(count, rows)
		}

		if across > maxAcross {
			break
		}

		most = columns
	}

	return most
}

// coreGrids is the dialog seen as something to be fitted to a screen.
// Measuring goes through contentDialogSize, which walks the real widget tree,
// so margins, group box borders, the font's own metrics and the rest are all
// accounted for rather than estimated.
type coreGrids struct {
	dlg    *walk.Dialog
	scroll *walk.ScrollView
	body   *walk.Composite
	grids  []*walk.Composite
	// columns is the shape the core boxes were drawn as, which came from how
	// the machine groups them. It is the narrowest shape the search will use,
	// so a dialog is never folded tighter than its author laid it out.
	columns int
}

// newCoreGrids takes the dialog as it was drawn, so call it before anything
// has changed the shape of the core grids.
func newCoreGrids(dlg *walk.Dialog, scroll *walk.ScrollView, body *walk.Composite, grids []*walk.Composite) coreGrids {
	c := coreGrids{
		dlg:    dlg,
		scroll: scroll,
		body:   body,
		grids:  grids,
	}

	if len(grids) > 0 {
		c.columns = gridColumns(grids[0])
	}

	return c
}

func (c coreGrids) empty() bool {
	return c.dlg == nil || c.scroll == nil
}

// apply lays the core boxes out over the given number of columns. It does not
// ask for a layout, since the search runs this once per shape it tries on and
// only the last of them is the one to lay out.
func (c coreGrids) apply(columns int) {
	setGridRows(c.grids, rowsForColumns(c.grids, columns))

	// Re-cap the content column last: a cap left over from another shape would
	// hold the content at a width that shape wanted, and the measurement taken
	// next would report that width rather than this shape's own.
	pinContentWidth(c.body)
}

// fitDialogAtOpen lays the core boxes out to fit a screen of the given work
// area: over as few rows as it takes, but never so few that they come to more
// than maxCoresAcross boxes across.
//
// This is the only place the dialog is ever laid out differently. It runs
// before the dialog is shown, and again when the processor list is switched on
// or off, since that changes how much there is to lay out. It does not run in
// between: nothing watches the window, so resizing shows more of the content or
// less of it and changes nothing else.
//
// That is deliberate. Changing the row count changes the shape of the dialog,
// and doing that as a window is dragged would mean the thing changing under
// the user's hand with no way to drag back to what they had. Done when the
// content itself changes, it is just the dialog's size.
func fitDialogAtOpen(c coreGrids, area walk.Size) {
	if c.empty() || area.Width <= 0 || area.Height <= 0 {
		return
	}

	// What each row count measured, for the length of this search and no
	// longer. The search asks in columns of the first grid, but the rows those
	// come to are ceil(boxes / columns), which is many to one: on a machine of
	// 32 core boxes, the counts the search can ask for are only a handful of
	// different grids. Keying on the rows is what stops the same grid being
	// measured over and over. Nothing outside wants these numbers, and a table
	// that outlived the search would have to be invalidated by hand every time
	// the dialog changed.
	measured := map[int]walk.Size{}

	measure := func(columns int) (width, height int) {
		rows := rowsForColumns(c.grids, columns)
		if size, ok := measured[rows]; ok {
			return size.Width, size.Height
		}

		c.apply(columns)

		size := contentDialogSize(c.dlg, c.scroll)
		measured[rows] = size

		return size.Width, size.Height
	}

	most := mostColumns(boxCounts(c.grids), maxCoresAcross)
	columns, _ := columnsThatFit(c.columns, most, area.Width, area.Height, measure)

	c.apply(columns)

	// Moving a widget to another cell does not ask for a layout on its own, so
	// say so here rather than leave the dialog drawn as whatever the search
	// stopped on. Before the dialog is up there is nothing to lay out yet, and
	// asking for one would only shrink the window to the layout minimum that
	// the opening size is about to replace.
	if c.dlg.Visible() {
		c.dlg.RequestLayout()
	}
}
