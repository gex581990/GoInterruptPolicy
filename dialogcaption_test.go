package main

import (
	"testing"

	"github.com/tailscale/win"
)

// The minimize button and the window menu both send SC_MINIMIZE, but Windows
// keeps bits of its own in the low four bits of the command, so a plain
// comparison would miss some of them.
func TestIsMinimizeCommandIgnoresTheSystemsOwnBits(t *testing.T) {
	for low := uintptr(0); low < 16; low++ {
		if command := uintptr(win.SC_MINIMIZE) | low; !isMinimizeCommand(command) {
			t.Errorf("%#x was not taken for a minimize", command)
		}
	}

	for _, command := range []uintptr{win.SC_MAXIMIZE, win.SC_RESTORE, win.SC_CLOSE, win.SC_MOVE, win.SC_SIZE} {
		if isMinimizeCommand(command) {
			t.Errorf("%#x was taken for a minimize", command)
		}
	}
}

// Only the WM_SHOWWINDOW sent as the owner is minimized or restored is about the
// owner. The one ShowWindow sends carries an lParam of zero whether it shows or
// hides, and must be left to the default handling.
func TestOwnerShowChangeOf(t *testing.T) {
	tests := []struct {
		name           string
		wParam, lParam uintptr
		want           ownerShowChange
	}{
		{name: "hidden as the owner is minimized", wParam: 0, lParam: swParentClosing, want: ownerMinimizing},
		{name: "shown as the owner is restored", wParam: 1, lParam: swParentOpening, want: ownerRestoring},
		{name: "shown by ShowWindow", wParam: 1, lParam: 0, want: ownerUnchanged},
		{name: "hidden by ShowWindow", wParam: 0, lParam: 0, want: ownerUnchanged},
		{name: "a show that claims the owner is closing", wParam: 1, lParam: swParentClosing, want: ownerUnchanged},
		{name: "a hide that claims the owner is opening", wParam: 0, lParam: swParentOpening, want: ownerUnchanged},
	}

	for _, tt := range tests {
		if got := ownerShowChangeOf(tt.wParam, tt.lParam); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
