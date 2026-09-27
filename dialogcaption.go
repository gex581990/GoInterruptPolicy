package main

import (
	"log"
	"syscall"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
)

// addCaptionButtons gives a dialog the minimize and maximize buttons that walk
// leaves off every dialog it creates. Call it after the dialog is created and
// before it is shown.
//
// Maximizing needs nothing more than the button: the dialog already resizes,
// and a maximized dialog is only a large one.
//
// Minimizing needs more. The dialog is modal, so walk disables its owner, the
// main window, for as long as it is up. Minimized on its own, the dialog
// shrinks to a stub of a title bar above the taskbar, with no taskbar button
// of its own to come back from, and leaves that disabled window on the screen.
// So the minimize button minimizes the owner instead. Windows hides the windows
// an owner owns when it is minimized and shows them again when it is restored,
// so the whole program goes to the taskbar and comes back with the dialog
// still on top of it.
func addCaptionButtons(dlg *walk.Dialog) {
	hwnd := dlg.Handle()

	style := uint32(win.GetWindowLong(hwnd, win.GWL_STYLE))
	win.SetWindowLong(hwnd, win.GWL_STYLE, int32(style|win.WS_MINIMIZEBOX|win.WS_MAXIMIZEBOX))

	// Windows keeps the frame it drew; this has it draw the frame again, now
	// with the two buttons.
	win.SetWindowPos(hwnd, 0, 0, 0, 0, 0,
		win.SWP_FRAMECHANGED|win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_NOOWNERZORDER|win.SWP_NOACTIVATE)

	// Record the window procedure before replacing it, so that there is no
	// moment at which a message could reach captionWndProc with nowhere to
	// pass it on to.
	captionWndProcs[hwnd] = win.GetWindowLongPtr(hwnd, win.GWLP_WNDPROC)
	if win.SetWindowLongPtr(hwnd, win.GWLP_WNDPROC, captionWndProcPtr) == 0 {
		log.Println("could not take over the dialog's minimize button")
		delete(captionWndProcs, hwnd)
	}
}

// captionWndProcs holds, for each dialog given caption buttons, the window
// procedure it had before, which every message captionWndProc does not handle
// is passed on to. walk runs all of its windows on one thread, so nothing else
// touches this while a message is being handled.
var captionWndProcs = map[win.HWND]uintptr{}

// captionWndProcPtr is made once, rather than once per dialog: a callback into
// Go from Windows is never freed, and a program only gets a fixed number of
// them.
var captionWndProcPtr = syscall.NewCallback(captionWndProc)

func captionWndProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	prev := captionWndProcs[hwnd]

	switch msg {
	case win.WM_SYSCOMMAND:
		// A dialog that somehow has no visible owner minimizes the ordinary
		// way, which at least leaves it somewhere to be found.
		if isMinimizeCommand(wParam) {
			if owner := win.GetWindow(hwnd, win.GW_OWNER); owner != 0 && win.IsWindowVisible(owner) {
				win.ShowWindow(owner, win.SW_MINIMIZE)
				return 0
			}
		}

	case win.WM_NCDESTROY:
		// The last message a window gets, so the last time prev is needed.
		defer delete(captionWndProcs, hwnd)
	}

	return win.CallWindowProc(prev, hwnd, msg, wParam, lParam)
}

// isMinimizeCommand says whether a WM_SYSCOMMAND asks for the window to be
// minimized. The low four bits of its wParam are the system's own and have to
// be masked off before it is compared.
func isMinimizeCommand(wParam uintptr) bool {
	return wParam&0xFFF0 == win.SC_MINIMIZE
}
