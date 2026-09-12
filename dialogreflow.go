package main

import (
	"log"

	"github.com/tailscale/walk"
)

// columnsForWidth returns how many columns of boxWidth fit in avail, given the
// spacing between them, clamped to [1, count]. Every value is native pixels.
func columnsForWidth(avail, boxWidth, spacing, count int) int {
	if count < 1 {
		return 1
	}
	if boxWidth < 1 {
		return count
	}

	// The first column costs its own width, every later one costs a spacing too.
	cols := 1
	for used := boxWidth; cols < count; cols++ {
		used += spacing + boxWidth
		if used > avail {
			break
		}
	}

	return clampInt(cols, 1, count)
}

// setGridColumns lays the children of a grid composite out over the given
// number of columns. walk builds the grid by giving every child a cell, and
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

// gridColumns is the number of columns a grid is currently laid out over.
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

// widestChild returns the width of the widest box in a grid, in native pixels.
func widestChild(grid *walk.Composite) int {
	if grid == nil {
		return 0
	}

	children := grid.Children()

	widest := 0
	for i := 0; i < children.Len(); i++ {
		if w := children.At(i).MinSizeHint().Width; w > widest {
			widest = w
		}
	}

	return widest
}

// mostChildren returns the largest number of boxes any one grid holds.
func mostChildren(grids []*walk.Composite) int {
	most := 0
	for _, grid := range grids {
		if n := grid.Children().Len(); n > most {
			most = n
		}
	}

	return most
}

// spreadCoreGrids lays the core boxes out over as many columns as fit in avail,
// and reports whether that changed anything. Widening the dialog therefore puts
// more cores on a row rather than leaving the space empty, and every row it
// saves is a row of height the dialog no longer needs.
func spreadCoreGrids(grids []*walk.Composite, avail int) bool {
	if len(grids) == 0 {
		return false
	}

	// Grids sit side by side, so each one gets its share of the width.
	share := avail / len(grids)

	changed := false
	for _, grid := range grids {
		count := grid.Children().Len()
		if count == 0 {
			continue
		}

		columns := columnsForWidth(share, widestChild(grid), gridSpacing(grid), count)
		if columns == gridColumns(grid) {
			continue
		}

		setGridColumns(grid, columns)
		changed = true
	}

	return changed
}

// gridSpacing returns the gap a grid leaves between its boxes, in native pixels.
func gridSpacing(grid *walk.Composite) int {
	if grid == nil {
		return 0
	}

	layout, ok := grid.Layout().(*walk.GridLayout)
	if !ok {
		return 0
	}

	return walk.IntFrom96DPI(layout.Spacing(), grid.DPI())
}

// packCoreGrids widens the core grids until the dialog is short enough for the
// work area, or until widening it any further would push it off the side.
//
// This is what stops a machine with a lot of cores opening a dialog taller than
// the screen: three rows of core boxes cost far more height than the same boxes
// spread over one row cost width.
func packCoreGrids(dlg *walk.Dialog, scroll *walk.ScrollView, body *walk.Composite, grids []*walk.Composite, area walk.Size) {
	if dlg == nil || scroll == nil || len(grids) == 0 || area.Width <= 0 || area.Height <= 0 {
		return
	}

	// Start from the shape the layout heuristic already chose and only widen
	// from there, so a dialog that fits is left the way its author drew it.
	start := 1
	for _, grid := range grids {
		if columns := gridColumns(grid); columns > start {
			start = columns
		}
	}

	most := mostChildren(grids)

	for columns := start; columns <= most; columns++ {
		for _, grid := range grids {
			setGridColumns(grid, columns)
		}
		pinContentWidth(body)

		size := contentDialogSize(dlg, scroll)
		if size.Width > area.Width {
			// One column too far. Step back, unless there is nowhere to step.
			if columns > 1 {
				for _, grid := range grids {
					setGridColumns(grid, columns-1)
				}
				pinContentWidth(body)
			}

			return
		}
		if size.Height <= area.Height {
			return
		}
	}
}
