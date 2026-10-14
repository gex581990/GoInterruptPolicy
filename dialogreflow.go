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
// The font is not a lever. It stays the size walk draws every form at, its
// default of MS Shell Dlg 2 at 8 points scaled for the display's DPI, and
// whatever still does not fit is scrolled to. Shrinking the text to save a
// scroll bar made the dialog hardest to read on exactly the machines with the
// most cores to read through.
//
// Taking a row away never makes the content narrower and never makes it
// taller, and the search below leans on that.

// maxCoresAcross is the most core boxes the dialog will put side by side,
// counting every grid in the row together, since the grids of each efficiency
// class and of each cache group sit beside one another. The search stops
// spreading the boxes out here and leaves the rest to the scroll bar. Without
// it a screen short of height had them stretched into a long thin strip, two
// rows of twelve on a 13900, that looked nothing like the rest of the dialog.
// It is counted in boxes rather than pixels so that it holds at any display
// scaling.
const maxCoresAcross = 8

// rowsThatFit returns the most rows, from drawn down to fewest, whose content
// fits inside maxWidth by maxHeight, and true. When nothing fits it returns
// the fewest rows that at least fit the width, and false, which is the shape
// that leaves the least of the content to be scrolled to.
//
// drawn is the shape the dialog was drawn as, and the search never goes above
// it: that shape came from the topology of the machine, laying the cores of a
// group out the way they are grouped, and there is nothing to be gained by
// folding them into more rows than their author asked for. The one exception
// is a fewest above drawn, and then fewest wins: it is the limit on how wide
// the boxes may go, and that holds whatever they were drawn as.
//
// It searches in rows, not in the columns of any one grid, because rows are
// what the grids share (see setGridRows). The columns of one grid cannot say
// every row count: a grid of two boxes is two rows over one column or one row
// over two, so searching in its columns could never reach the three rows a
// grid of eight beside it was drawn in.
//
// Both halves halve their range each time rather than walking it, since each
// measurement is a layout pass over the real widget tree and a machine of many
// cores has many counts to choose between.
func rowsThatFit(drawn, fewest, maxWidth, maxHeight int, measure func(rows int) (width, height int)) (int, bool) {
	if fewest < 1 {
		fewest = 1
	}

	drawn = max(drawn, fewest)

	// The most rows short enough for the height. Taking a row away never
	// makes the content taller, so once one count is short enough every count
	// below it is too.
	tallest := fewest
	for lo, hi := fewest, drawn; lo <= hi; {
		mid := lo + (hi-lo)/2
		if _, height := measure(mid); height <= maxHeight {
			tallest = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	// Anything with more rows is too tall and anything with fewer is wider
	// still, so this one count is the only candidate there is.
	if width, height := measure(tallest); height <= maxHeight && width <= maxWidth {
		return tallest, true
	}

	// Nothing fits. Taking a row away never makes the content narrower, so the
	// fewest rows that at least fit the width is the shortest shape that can be
	// had without scrolling sideways as well.
	widest := drawn
	for lo, hi := fewest, drawn; lo <= hi; {
		mid := lo + (hi-lo)/2
		if width, _ := measure(mid); width <= maxWidth {
			widest = mid
			hi = mid - 1
		} else {
			lo = mid + 1
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

// drawnRows is how many rows grids holding counts boxes, laid out over the
// given columns, were drawn in: the most that any one of them has.
//
// Laid out over that many rows, each grid gets back exactly the columns it was
// drawn with. Each was drawn over the fewest columns that fit its boxes into
// the rows getLayout chose, and none of them has more rows than this, so no
// fewer columns would do and no more are needed.
func drawnRows(counts, columns []int) int {
	rows := 1
	for i, count := range counts {
		if i < len(columns) && columns[i] > 0 {
			rows = max(rows, mathCeilInInt(count, columns[i]))
		}
	}

	return rows
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

// gridColumns is how many columns a grid is currently laid out over. It says
// so only once setGridRows has laid the grid out: straight out of walk's
// builder, a grid of core boxes sits in cells that do not show its columns
// (see CheckBoxList.columns).
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

// fewestRows is the fewest rows the grids can be laid out over, each over the
// columns that gives it, without coming to more than maxAcross boxes side by
// side. counts is how many boxes each grid holds. A machine with more grids
// than maxAcross still has to be drawn somehow, and gets every grid in a
// single column, the narrowest it can be.
func fewestRows(counts []int, maxAcross int) int {
	most := 1
	for _, count := range counts {
		most = max(most, count)
	}

	// Fewer rows can only mean more boxes across, so the first count within
	// the limit is the fewest there is.
	for rows := 1; rows <= most; rows++ {
		across := 0
		for _, count := range counts {
			across += mathCeilInInt(count, rows)
		}

		if across <= maxAcross {
			return rows
		}
	}

	return most
}

// balanceCoreWidths shares a dialog wider than its content out between the
// core grids in proportion to how many columns each has, so that every core
// box is widened by the same amount. Left to itself a row shares it out equally
// between the things in it, which on a machine of two efficiency classes would
// widen each of the two columns of P-cores twice as much as each of the four
// columns of E-cores beside them.
//
// So every level between a grid and area that lays its children out in a row,
// or in the columns of a grid, is given the number of core columns below it as
// its stretch factor. Run it whenever the grids are laid out over new columns.
func balanceCoreWidths(grids []*walk.Composite, area *walk.Composite) {
	if area == nil {
		return
	}

	columns := map[walk.Widget]int{}
	var widgets []walk.Widget

	for _, grid := range grids {
		n := gridColumns(grid)

		var widget walk.Widget = grid
		for {
			parent := widget.Parent()
			if parent == nil {
				break
			}

			if _, ok := columns[widget]; !ok {
				widgets = append(widgets, widget)
			}
			columns[widget] += n

			if parent.Handle() == area.Handle() {
				break
			}

			next, ok := parent.(walk.Widget)
			if !ok {
				break
			}
			widget = next
		}
	}

	for _, widget := range widgets {
		parent := widget.Parent()

		// A group box keeps its children in a composite inside it, so the way
		// up from a grid passes that composite too, with the group box as its
		// parent. The group box's layout is the composite's own, which does not
		// hold the composite itself, and there is nothing to set there.
		if children := parent.Children(); children == nil || !children.Contains(widget) {
			continue
		}

		factor := max(columns[widget], 1)

		var err error
		switch layout := parent.Layout().(type) {
		case *walk.BoxLayout:
			if layout.Orientation() != walk.Horizontal {
				continue
			}
			err = layout.SetStretchFactor(widget, factor)

		case *walk.GridLayout:
			// A grid shares a wider dialog out between its columns, so the
			// factor goes on the column the widget is in. The cache groups
			// that put their efficiency classes in a grid give it four
			// columns, so up to four classes each have one to themselves.
			cell, ok := layout.Range(widget)
			if !ok || cell.Width != 1 {
				continue
			}
			err = layout.SetColumnStretchFactor(cell.X, factor)

		default:
			continue
		}

		if err != nil {
			log.Println(err)
		}
	}
}

// coreGrids is the dialog seen as something to be fitted to a screen.
// Measuring goes through contentDialogSize, which walks the real widget tree,
// so margins, group box borders, the font's own metrics and the rest are all
// accounted for rather than estimated.
type coreGrids struct {
	dlg    *walk.Dialog
	scroll *walk.ScrollView
	// area is the row the core groups sit in, the level balanceCoreWidths
	// shares a wider dialog out from.
	area  *walk.Composite
	grids []*walk.Composite
	// rows is the shape the core boxes were drawn as, which came from how
	// the machine groups them, worked out from the columns each grid was
	// declared with. It is the most rows the search will use, so a dialog is
	// never folded tighter than its author laid it out, unless that shape is
	// wider than maxCoresAcross allows, and then fewestRows wins (see
	// rowsThatFit).
	rows int
}

// newCoreGrids takes the dialog as it was drawn. columns is what each of grids
// was declared with, as CheckBoxList.Grids returns it, since the grids
// themselves cannot say.
func newCoreGrids(dlg *walk.Dialog, scroll *walk.ScrollView, area *walk.Composite, grids []*walk.Composite, columns []int) coreGrids {
	c := coreGrids{
		dlg:    dlg,
		scroll: scroll,
		area:   area,
		grids:  grids,
	}

	c.rows = drawnRows(boxCounts(grids), columns)

	return c
}

func (c coreGrids) empty() bool {
	return c.dlg == nil || c.scroll == nil
}

// apply lays the core boxes out over the given number of rows. It does not ask
// for a layout, since the search runs this once per shape it tries on and only
// the last of them is the one to lay out.
func (c coreGrids) apply(rows int) {
	setGridRows(c.grids, rows)
}

// fitDialogAtOpen lays the core boxes out to fit a screen of the given work
// area: over as few rows as it takes, but never so few that they come to more
// than maxCoresAcross boxes across.
//
// This is the only place the dialog is ever laid out differently. It runs
// before the dialog is shown, and again when the processor list is switched on
// or off, since that changes how much there is to lay out. It does not run in
// between: nothing watches the window, so resizing shows more of the content or
// less of it, and shares any width over what it needs out between the boxes,
// but never changes how the cores are arranged.
//
// That is deliberate. Changing the row count changes the shape of the dialog,
// and doing that as a window is dragged would mean the thing changing under
// the user's hand with no way to drag back to what they had. Done when the
// content itself changes, it is just the dialog's size.
func fitDialogAtOpen(c coreGrids, area walk.Size) {
	if c.empty() {
		return
	}

	fewest := fewestRows(boxCounts(c.grids), maxCoresAcross)

	// With no screen to fit, the shape the boxes were drawn in, within the
	// limit. They still have to be put there: walk's builder does not leave
	// them in the cells they were declared in (see CheckBoxList.columns).
	rows := max(c.rows, fewest)

	if area.Width > 0 && area.Height > 0 {
		// What each row count measured, for the length of this search and no
		// longer. The search can ask for a count it has measured already, once
		// in each of its halves, and each measurement is a layout pass.
		// Nothing outside wants these numbers, and a table that outlived the
		// search would have to be invalidated by hand every time the dialog
		// changed.
		measured := map[int]walk.Size{}

		measure := func(rows int) (width, height int) {
			if size, ok := measured[rows]; ok {
				return size.Width, size.Height
			}

			c.apply(rows)

			size := contentDialogSize(c.dlg, c.scroll)
			measured[rows] = size

			return size.Width, size.Height
		}

		rows, _ = rowsThatFit(c.rows, fewest, area.Width, area.Height, measure)
	}

	c.apply(rows)
	balanceCoreWidths(c.grids, c.area)

	// Moving a widget to another cell does not ask for a layout on its own, so
	// say so here rather than leave the dialog drawn as whatever the search
	// stopped on. Before the dialog is up there is nothing to lay out yet, and
	// asking for one would only shrink the window to the layout minimum that
	// the opening size is about to replace.
	if c.dlg.Visible() {
		c.dlg.RequestLayout()
	}
}
