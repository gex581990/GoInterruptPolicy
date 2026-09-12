package main

import "testing"

func TestColumnsForWidth(t *testing.T) {
	// A core box 190 wide with 6 of spacing between them, eight of them.
	const box, spacing, count = 190, 6, 8

	tests := []struct {
		name  string
		avail int
		want  int
	}{
		{name: "nothing fits, one column anyway", avail: 0, want: 1},
		{name: "exactly one box", avail: 190, want: 1},
		{name: "one box and a bit", avail: 300, want: 1},
		{name: "two boxes exactly", avail: 386, want: 2},  // 190 + 6 + 190
		{name: "one short of three", avail: 581, want: 2}, // 582 would be three
		{name: "three boxes exactly", avail: 582, want: 3},
		{name: "room for more than there are", avail: 4000, want: count},
		{name: "the reporter's window", avail: 1900, want: 8}, // 8 boxes need 1562
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := columnsForWidth(tt.avail, box, spacing, count); got != tt.want {
				t.Errorf("columnsForWidth(%d, %d, %d, %d) = %d, want %d",
					tt.avail, box, spacing, count, got, tt.want)
			}
		})
	}
}

func TestColumnsForWidthDegenerateInput(t *testing.T) {
	if got := columnsForWidth(1000, 190, 6, 0); got != 1 {
		t.Errorf("no boxes gave %d columns, want 1", got)
	}
	if got := columnsForWidth(1000, 0, 6, 8); got != 8 {
		t.Errorf("an unmeasurable box gave %d columns, want all %d on one row", got, 8)
	}
	if got := columnsForWidth(-50, 190, 6, 8); got != 1 {
		t.Errorf("negative width gave %d columns, want 1", got)
	}
}

// Spreading the cores sideways is only worth doing because it buys height back.
// This is the arithmetic the packing loop relies on.
func TestSpreadingColumnsCostsWidthAndSavesHeight(t *testing.T) {
	const box, spacing, count = 190, 6, 8
	const boxHeight = 170

	rowsFor := func(columns int) int { return mathCeilInInt(count, columns) }

	narrow := columnsForWidth(600, box, spacing, count)
	wide := columnsForWidth(1900, box, spacing, count)

	if wide <= narrow {
		t.Fatalf("a wider window gave %d columns against %d, want more", wide, narrow)
	}
	if rowsFor(wide) >= rowsFor(narrow) {
		t.Fatalf("%d columns still needs %d rows against %d rows for %d columns",
			wide, rowsFor(wide), rowsFor(narrow), narrow)
	}

	saved := (rowsFor(narrow) - rowsFor(wide)) * boxHeight
	if saved <= 0 {
		t.Fatalf("spreading saved %d pixels of height", saved)
	}
	t.Logf("%d columns -> %d rows, %d columns -> %d rows, %d px of height saved",
		narrow, rowsFor(narrow), wide, rowsFor(wide), saved)
}
