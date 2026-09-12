package main

import (
	"log"

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

// dialogFontFloor and dialogFontCeiling are how far the dialog font may be
// taken from the size the system chose, as fractions of it. The floor is what
// a window too small for its content is allowed to shrink to before the
// ScrollView takes over; the ceiling is how far a window larger than its
// content may magnify it.
const (
	dialogFontFloor   = 3.0 / 4.0
	dialogFontCeiling = 3.0
)

// smallestFontSize and largestFontSize are the range the dialog font may be
// scaled over, given the size the system chose. Keeping them relative to that
// size rather than naming point sizes is what gives a display whose scaling
// makes everything large the same headroom, proportionally, as one that does
// not.
func smallestFontSize(points int) int {
	return scaleFontSize(points, dialogFontFloor)
}

func largestFontSize(points int) int {
	return scaleFontSize(points, dialogFontCeiling)
}

func scaleFontSize(points int, by float64) int {
	if points < 1 {
		return 1
	}

	scaled := int(float64(points) * by)
	if scaled < 1 {
		scaled = 1
	}

	return scaled
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

// dialogShape is a font size and a core column count, the pair that the search
// is over, together with the dpi they were measured at. A point size is a
// physical size and walk renders it at the dpi of the monitor the dialog is
// on, so the same shape on a monitor of another scaling is a different number
// of pixels: dragging the dialog across to one reaches past everything
// remembered here rather than re-using it.
type dialogShape struct {
	dpi     int
	points  int
	columns int
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
	// sizes is what each shape measured. Dragging the window about tries the
	// same handful of shapes over and over, and a remembered one costs nothing,
	// which is what keeps a resize from running a layout pass per pixel.
	sizes map[dialogShape]walk.Size
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
		sizes:  make(map[dialogShape]walk.Size),
	}

	if len(grids) > 0 {
		c.columns = gridColumns(grids[0])
	}

	return c
}

func (c coreGrids) empty() bool {
	return c.dlg == nil || c.scroll == nil || c.font == nil
}

// forget drops the remembered measurements. Showing or hiding a section
// changes what every shape measures, so the old numbers no longer describe
// this dialog.
func (c coreGrids) forget() {
	clear(c.sizes)
}

// apply draws the dialog at the given shape. It does not ask for a layout,
// since the search runs this once per shape it tries on and only the last of
// them is the one to lay out.
func (c coreGrids) apply(points, columns int) {
	font, err := walk.NewFont(c.font.Family(), points, c.font.Style())
	if err != nil {
		log.Println(err)
	} else {
		// walk caches fonts by family, size and style, so this hands back the
		// same handle every time a size is tried again and there is nothing to
		// dispose of.
		c.dlg.SetFont(font)
	}

	for _, grid := range c.grids {
		setGridColumns(grid, columns)
	}

	// Re-cap the content column last: a cap left over from another shape would
	// hold the content at a width that shape wanted, and the measurement taken
	// next would report that width rather than this shape's own.
	pinContentWidth(c.body)
}

// measure reports the outer size the dialog would need to show everything at
// the given shape, drawing it at that shape to find out.
func (c coreGrids) measure(points, columns int) (width, height int) {
	shape := dialogShape{dpi: c.dlg.DPI(), points: points, columns: columns}
	if size, ok := c.sizes[shape]; ok {
		return size.Width, size.Height
	}

	c.apply(points, columns)

	size := contentDialogSize(c.dlg, c.scroll)
	c.sizes[shape] = size
	logf("    try %2dpt over %2d columns -> content %s", points, columns, logSize(size))

	return size.Width, size.Height
}

// fitDialogContent draws the dialog as large as it can be while still fitting
// target, which is an outer window size. ceiling is the largest font size it
// may use, so the caller decides whether this is allowed to magnify the dialog
// or only to shrink it.
//
// This is the whole of the sizing policy, and nothing in it names a pixel or a
// point: a larger window is answered by drawing everything larger rather than
// by padding the content with empty space, a smaller one by drawing everything
// smaller, and past the floor the ScrollView takes over.
func fitDialogContent(c coreGrids, target walk.Size, ceiling int) {
	if c.empty() || target.Width <= 0 || target.Height <= 0 {
		return
	}

	points, columns := largestThatFits(smallestFontSize(c.font.PointSize()), ceiling,
		c.columns, mostChildren(c.grids), target.Width, target.Height, c.measure)

	logf("fit: %s -> %dpt of %dpt over %d columns",
		logSize(target), points, c.font.PointSize(), columns)

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

// fitDialogAtOpen sizes the content for a dialog that is about to be shown on
// a screen of the given work area. It will shrink the dialog to fit but never
// magnify it, so a dialog that already fits opens at the size the user's own
// settings asked for instead of blown up to fill the screen.
func fitDialogAtOpen(c coreGrids, area walk.Size) {
	if c.empty() {
		return
	}

	fitDialogContent(c, area, c.font.PointSize())
}

// fitDialogToWindow redraws the content at the size the window has now. This
// is what makes dragging the window scale the dialog rather than pad it.
func fitDialogToWindow(c coreGrids) {
	if c.empty() {
		return
	}

	fitDialogContent(c, c.dlg.SizePixels(), largestFontSize(c.font.PointSize()))
}
