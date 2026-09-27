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
// still on top of it, the way the user left it: see captionWndProc for why that
// last part needs doing by hand.
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

// captionHiddenByOwner holds the dialogs Windows has hidden because their owner
// was minimized, so that those and only those are shown again when it is
// restored.
var captionHiddenByOwner = map[win.HWND]bool{}

// The lParam of the WM_SHOWWINDOW Windows sends an owned window as its owner is
// minimized or restored. tailscale/win does not define them.
const (
	swParentClosing = 1
	swParentOpening = 3
)

// ownerShowChange is what a WM_SHOWWINDOW says about the owner of the window it
// was sent to.
type ownerShowChange int

const (
	ownerUnchanged ownerShowChange = iota
	ownerMinimizing
	ownerRestoring
)

// ownerShowChangeOf reads a WM_SHOWWINDOW. A window is hidden as its owner is
// minimized and shown as its owner is restored; any other WM_SHOWWINDOW, such as
// one from ShowWindow, which sends an lParam of zero, says nothing about the
// owner.
func ownerShowChangeOf(wParam, lParam uintptr) ownerShowChange {
	switch {
	case wParam == 0 && lParam == swParentClosing:
		return ownerMinimizing
	case wParam != 0 && lParam == swParentOpening:
		return ownerRestoring
	}

	return ownerUnchanged
}

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

	case win.WM_SHOWWINDOW:
		switch ownerShowChangeOf(wParam, lParam) {
		case ownerMinimizing:
			// Let Windows hide the dialog as it would, and remember that it did.
			captionHiddenByOwner[hwnd] = true

		case ownerRestoring:
			if !captionHiddenByOwner[hwnd] {
				break
			}
			delete(captionHiddenByOwner, hwnd)

			// Left to the default handling, the dialog is shown again the way
			// SW_SHOWNOACTIVATE shows a window, which like SW_SHOWNORMAL returns
			// a maximized or snapped window to its original size and position:
			// for this dialog, where it first opened. SW_SHOWNA shows it in the
			// size and position it has, still maximized or snapped, and like
			// SW_SHOWNOACTIVATE leaves activation to Windows.
			win.ShowWindow(hwnd, win.SW_SHOWNA)
			return 0
		}

	case win.WM_NCDESTROY:
		// The last message a window gets, so the last time prev is needed.
		defer delete(captionWndProcs, hwnd)
		delete(captionHiddenByOwner, hwnd)
	}

	return win.CallWindowProc(prev, hwnd, msg, wParam, lParam)
}

// isMinimizeCommand says whether a WM_SYSCOMMAND asks for the window to be
// minimized. The low four bits of its wParam are the system's own and have to
// be masked off before it is compared.
func isMinimizeCommand(wParam uintptr) bool {
	return wParam&0xFFF0 == win.SC_MINIMIZE
}
