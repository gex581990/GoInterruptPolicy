package main

import (
	"testing"

	"github.com/tailscale/walk"
	"github.com/tailscale/walk/declarative"
)

// Each core grid is recorded with the columns its Grid layout declares, and
// those give back the rows the grids were drawn in. The grids cannot say it
// themselves once walk's builder has placed their boxes (see
// CheckBoxList.columns), so the dialog's drawn shape rests on this record.
func TestCoreGridsRecordTheColumnsTheyAreDeclaredWith(t *testing.T) {
	saved := cs
	defer func() { cs = saved }()

	// Two cache groups of six cores with two threads each, as on a 5900X.
	// getLayout draws them in three rows, each group over two columns, an
	// even count, which is the kind walk's builder leaves unreadable.
	cs = CpuSets{}
	cs.initFrom(fakeCpuSets(24, func(i int, c *SYSTEM_CPU_SET_INFORMATION_Anonymous_CpuSet) {
		c.LogicalProcessorIndex = byte(i)
		c.CoreIndex = byte(i / 2 * 2)
		c.LastLevelCacheIndex = byte(i / 12 * 12)
	}))

	var list CheckBoxList
	var bits Bits
	tree := list.create(&bits)

	declared := map[**walk.Composite]declarative.Composite{}
	var visit func(widgets []declarative.Widget)
	visit = func(widgets []declarative.Widget) {
		for _, w := range widgets {
			switch w := w.(type) {
			case declarative.Composite:
				if w.AssignTo != nil {
					declared[w.AssignTo] = w
				}
				visit(w.Children)
			case declarative.GroupBox:
				visit(w.Children)
			}
		}
	}
	visit(tree)

	if len(list.grids) != 2 || len(list.columns) != 2 {
		t.Fatalf("%d grids recorded with %d column counts, want 2 and 2", len(list.grids), len(list.columns))
	}

	counts := make([]int, len(list.grids))
	for i, holder := range list.grids {
		grid, ok := declared[holder]
		if !ok {
			t.Fatalf("grid %d is not in the dialog", i)
		}

		layout, ok := grid.Layout.(declarative.Grid)
		if !ok {
			t.Fatalf("grid %d has no grid layout", i)
		}
		if layout.Columns != list.columns[i] {
			t.Errorf("grid %d recorded as %d columns, declared with %d", i, list.columns[i], layout.Columns)
		}

		counts[i] = len(grid.Children)
	}

	if got := drawnRows(counts, list.columns); got != 3 {
		t.Errorf("drawn in %d rows, want 3: columns %v for %v boxes", got, list.columns, counts)
	}
}
