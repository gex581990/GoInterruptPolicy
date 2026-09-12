package main

import (
	"log"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
)

// dialogMinSize is the smallest size the user may shrink the device policy
// dialog to. Like every other size in the declarative layout it is given in
// 96 dpi units, walk scales it to the dpi of the monitor the dialog is on.
var dialogMinSize = walk.Size{Width: 400, Height: 300}

// workArea returns the work area, the monitor without the taskbar and other
// appbars, of the monitor closest to hwnd in native pixels. The zero Rectangle
// is returned when the monitor cannot be determined.
func workArea(hwnd win.HWND) walk.Rectangle {
	var mi win.MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))

	monitor := win.MonitorFromWindow(hwnd, win.MONITOR_DEFAULTTONEAREST)
	if monitor == 0 || !win.GetMonitorInfo(monitor, &mi) {
		return walk.Rectangle{}
	}

	return walk.Rectangle{
		X:      int(mi.RcWork.Left),
		Y:      int(mi.RcWork.Top),
		Width:  int(mi.RcWork.Right - mi.RcWork.Left),
		Height: int(mi.RcWork.Bottom - mi.RcWork.Top),
	}
}

// capToSize limits size to available. Capping an axis is what brings up the
// scrollbar on the other one, and walk reserves no room for the scrollbars of a
// ScrollView that can scroll both ways, so the bar eats into the viewport
// instead. Adding its thickness leaves the content as much visible room as it
// wanted. vScrollWidth and hScrollHeight are the scrollbar thicknesses. An
// empty available means no monitor was found, in which case size is left
// alone.
func capToSize(size, available walk.Size, vScrollWidth, hScrollHeight int) walk.Size {
	if available.Width <= 0 || available.Height <= 0 {
		return size
	}

	if size.Height > available.Height {
		size.Height = available.Height
		size.Width += vScrollWidth
	}

	if size.Width > available.Width {
		size.Width = available.Width
		size.Height += hScrollHeight
		if size.Height > available.Height {
			size.Height = available.Height
		}
	}

	return size
}

// centerBounds returns a rectangle of size that keeps the center of old and is
// moved back into area when it would stick out. A window larger than area ends
// up at the top left corner of area, which is what keeps the caption bar and
// the buttons of an oversized dialog reachable.
func centerBounds(old walk.Rectangle, size walk.Size, area walk.Rectangle) walk.Rectangle {
	bounds := walk.Rectangle{
		X:      old.X + (old.Width-size.Width)/2,
		Y:      old.Y + (old.Height-size.Height)/2,
		Width:  size.Width,
		Height: size.Height,
	}

	if area.Width > 0 && area.Height > 0 {
		bounds.X = clampInt(bounds.X, area.X, area.X+area.Width-bounds.Width)
		bounds.Y = clampInt(bounds.Y, area.Y, area.Y+area.Height-bounds.Height)
	}

	return bounds
}

// clampInt limits value to [lo, hi]. lo wins over hi, so a window wider or
// taller than the work area starts at its corner instead of hanging over the
// top or the left of the screen.
func clampInt(value, lo, hi int) int {
	if value > hi {
		value = hi
	}
	if value < lo {
		value = lo
	}
	return value
}

// contentDialogSize returns the outer size dlg would need to show everything
// without scrolling, with no cap applied. Native pixels throughout.
func contentDialogSize(dlg *walk.Dialog, scroll *walk.ScrollView) walk.Size {
	// A ScrollView with both scrollbars reports a minimum size of zero, so the
	// layout minimum of the dialog covers everything except the scrolled
	// content. SizeHint gives what that content would need, measured on the
	// ScrollView itself so the composite walk keeps inside it is included.
	client := walk.CreateLayoutItemsForContainer(dlg).MinSize()
	content := scroll.SizeHint()

	client.Height += content.Height
	if content.Width > client.Width {
		client.Width = content.Width
	}

	// Add the window decorations to turn the client size into an outer size.
	outer, inner := dlg.SizePixels(), dlg.ClientBoundsPixels().Size()

	return walk.Size{
		Width:  client.Width + outer.Width - inner.Width,
		Height: client.Height + outer.Height - inner.Height,
	}
}

// desiredDialogSize returns the outer size dlg needs to show the contents of
// scroll without scrolling, capped to the work area of the monitor next to
// screen. Everything is measured in native pixels, which keeps the result
// correct on any dpi.
func desiredDialogSize(dlg *walk.Dialog, scroll *walk.ScrollView, screen win.HWND) walk.Size {
	dpi := uint32(dlg.DPI())

	return capToSize(contentDialogSize(dlg, scroll), workArea(screen).Size(),
		int(win.GetSystemMetricsForDpi(win.SM_CXVSCROLL, dpi)),
		int(win.GetSystemMetricsForDpi(win.SM_CYHSCROLL, dpi)))
}

// fitDialogToContent resizes dlg to the size its content asks for, capped to
// the work area of the monitor dlg is on. Use it whenever widgets are shown or
// hidden at runtime: a ScrollView hides those changes from the layout, so walk
// never resizes the dialog on its own.
func fitDialogToContent(dlg *walk.Dialog, scroll *walk.ScrollView) {
	if dlg == nil || scroll == nil {
		return
	}

	// SizeHint builds fresh layout items from the widget tree and Win32 reports
	// the window rectangle live, so measuring right after SetVisible works even
	// though the layout itself only runs once the caller returns.
	size := desiredDialogSize(dlg, scroll, dlg.Handle())

	if err := dlg.SetBoundsPixels(centerBounds(dlg.BoundsPixels(), size, workArea(dlg.Handle()))); err != nil {
		log.Println(err)
	}
}

// startDialogAtContentSize makes dlg open at the size its content needs instead
// of at the layout minimum, which is only as tall as the button row once the
// body of the dialog sits in a ScrollView.
//
// (*walk.Dialog).Show derives the start size from maxSize(layout minimum,
// minimum window size) and cannot be hooked, so the minimum is pinned to the
// wanted size for the first layout and released again as soon as the dialog is
// up. Releasing it matters: a minimum of the full content size would forbid
// dragging the dialog any smaller.
func startDialogAtContentSize(dlg *walk.Dialog, scroll *walk.ScrollView, owner walk.Form) {
	if dlg == nil || scroll == nil {
		return
	}

	// Before Show the dialog still sits wherever Windows created it, so measure
	// against the monitor of the owner window.
	screen := dlg.Handle()
	if owner != nil {
		screen = owner.Handle()
	}

	if err := dlg.SetMinMaxSizePixels(desiredDialogSize(dlg, scroll, screen), walk.Size{}); err != nil {
		log.Println(err)
		return
	}

	dlg.Starting().Attach(func() {
		// Cap the floor to the work area as well. On a small panel at a high
		// scaling a fixed floor would once more forbid shrinking the dialog to
		// a size that fits, which is the very thing this is here to prevent.
		floor := capToSize(walk.SizeFrom96DPI(dialogMinSize, dlg.DPI()), workArea(dlg.Handle()).Size(), 0, 0)

		if err := dlg.SetMinMaxSizePixels(floor, walk.Size{}); err != nil {
			log.Println(err)
		}

		// walk placed the dialog with fitRectToScreen, which picks the monitor
		// from the dialog window and not from the owner, and which only
		// re-positions a dialog that already fits. So measure once more against
		// the monitor the dialog really landed on and put it back into the work
		// area. On a single monitor this is the size it already has.
		fitDialogToContent(dlg, scroll)
	})
}
