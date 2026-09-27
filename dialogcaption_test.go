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
