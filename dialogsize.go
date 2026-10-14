package main

import (
	"log"
	"unsafe"

	"github.com/tailscale/walk"
	"github.com/tailscale/win"
	"golang.org/x/sys/windows"
)

// dialogMinSize is the smallest size the user may shrink the device policy
// dialog to. Like every other size in the declarative layout it is given in
// 96 dpi units, walk scales it to the dpi of the monitor the dialog is on.
var dialogMinSize = walk.Size{Width: 400, Height: 300}

// monitorInfo is what Windows says about the monitor closest to hwnd, and
// false when the monitor cannot be determined.
func monitorInfo(hwnd win.HWND) (win.MONITORINFO, bool) {
	var mi win.MONITORINFO
	mi.CbSize = uint32(unsafe.Sizeof(mi))

	monitor := win.MonitorFromWindow(hwnd, win.MONITOR_DEFAULTTONEAREST)
	if monitor == 0 || !win.GetMonitorInfo(monitor, &mi) {
		return mi, false
	}

	return mi, true
}

// workArea returns the work area, the monitor without the taskbar and other
// appbars, of the monitor closest to hwnd in native pixels. The zero Rectangle
// is returned when the monitor cannot be determined.
func workArea(hwnd win.HWND) walk.Rectangle {
	mi, ok := monitorInfo(hwnd)
	if !ok {
		return walk.Rectangle{}
	}

	return walk.RectangleFromRECT(mi.RcWork)
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
	return max(lo, min(value, hi))
}

// contentDialogSize returns the outer size dlg would need to show everything
// without scrolling, with no cap applied. Native pixels throughout.
func contentDialogSize(dlg *walk.Dialog, scroll *walk.ScrollView) walk.Size {
	// A ScrollView with both scrollbars reports a minimum size of zero, so the
	// layout minimum of the dialog covers everything except the scrolled
	// content. What that content would need is the ScrollView's ideal size,
	// measured again where it holds text that wraps (see scrolledContentSize).
	//
	// Take it out of the tree just built rather than asking the ScrollView for
	// it. Building that tree already walked the whole scrolled subtree, every
	// core box and thread checkbox of it, and kept its minimum as the
	// ScrollView's ideal size; SizeHint would build the identical subtree a
	// second time, and hand back that ideal size without the second
	// measurement. This is the innermost step of the search that sizes the
	// dialog, so it is worth not paying twice.
	items := walk.CreateLayoutItemsForContainer(dlg)
	client := items.MinSize()
	content := scrolledContentSize(items, scroll)

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

// scrolledContentSize is the size the contents of scroll want, read from an
// already built layout item tree. If scroll is not a child of that tree it
// builds scroll's own items instead and reads those the same way, so the
// answer cannot depend on where in the dialog the ScrollView is put.
func scrolledContentSize(items walk.ContainerLayoutItem, scroll *walk.ScrollView) walk.Size {
	var item walk.LayoutItem
	for _, child := range items.Children() {
		if child.Handle() == scroll.Handle() {
			item = child
			break
		}
	}
	if item == nil {
		item = walk.CreateLayoutItemsForContainer(scroll)
	}

	sizer, ok := item.(walk.IdealSizer)
	if !ok {
		return scroll.SizeHint()
	}

	size := sizer.IdealSize()

	// That is the content's minimum at the size its composite was last given,
	// and until the dialog's first layout has been applied it has been given
	// none. Text that wraps, the processor group note, is then measured at no
	// width at all, which walk takes as a single unwrapped line, and the
	// dialog came out lines short of its content. So when the content holds
	// such text, shown or not, measure it again at its own minimum width, the
	// narrowest it is ever laid out at. The text wraps the most there, so the
	// dialog is never short of it. A window made wider than that is taller
	// than it needs to be by the lines the note then saves.
	if container, ok := item.(walk.ContainerLayoutItem); ok {
		if children := container.Children(); len(children) == 1 {
			if content, ok := children[0].(walk.ContainerLayoutItem); ok && content.HasHeightForWidth() {
				size = content.MinSizeForSize(size)
			}
		}
	}

	return size
}

// desiredDialogSize returns the outer size dlg needs to show the contents of
// scroll without scrolling, capped to available. Everything is measured in
// native pixels, which keeps the result correct on any dpi.
func desiredDialogSize(dlg *walk.Dialog, scroll *walk.ScrollView, available walk.Size) walk.Size {
	dpi := uint32(dlg.DPI())

	return capToSize(contentDialogSize(dlg, scroll), available,
		int(win.GetSystemMetricsForDpi(win.SM_CXVSCROLL, dpi)),
		int(win.GetSystemMetricsForDpi(win.SM_CYHSCROLL, dpi)))
}

// frameInsets are how far a window's rectangle reaches past the frame Windows
// draws for it, on each side. Windows 10 and 11 give a resizable window borders
// to drag by on its left, right and bottom that are part of its rectangle but
// are never drawn, so a window placed by its rectangle against the edge of the
// work area stops that far short of it.
type frameInsets struct {
	left, top, right, bottom int
}

// invisibleFrame measures the borders of hwnd that are not drawn, by comparing
// its rectangle with the frame DWM draws. It only means anything for a window
// that is on screen in its normal state. No insets come back when DWM cannot
// say.
func invisibleFrame(hwnd win.HWND) frameInsets {
	var window, drawn win.RECT
	if !win.GetWindowRect(hwnd, &window) {
		return frameInsets{}
	}

	if win.FAILED(win.DwmGetWindowAttribute(hwnd, win.DWMWA_EXTENDED_FRAME_BOUNDS, unsafe.Pointer(&drawn), uint32(unsafe.Sizeof(drawn)))) {
		return frameInsets{}
	}

	return insetsBetween(window, drawn)
}

// insetsBetween is how far window reaches past drawn on each side. A drawn
// frame that is empty or does not lie inside window is not the frame of that
// window, and gives no insets.
func insetsBetween(window, drawn win.RECT) frameInsets {
	if drawn.Right <= drawn.Left || drawn.Bottom <= drawn.Top ||
		drawn.Left < window.Left || drawn.Top < window.Top ||
		drawn.Right > window.Right || drawn.Bottom > window.Bottom {
		return frameInsets{}
	}

	return frameInsets{
		left:   int(drawn.Left - window.Left),
		top:    int(drawn.Top - window.Top),
		right:  int(window.Right - drawn.Right),
		bottom: int(window.Bottom - drawn.Bottom),
	}
}

// outset is area grown by the insets on each side: the room a window's
// rectangle may take for its drawn frame to fill area exactly. An empty area,
// meaning no monitor was found, stays empty.
func (in frameInsets) outset(area walk.Rectangle) walk.Rectangle {
	if area.Width <= 0 || area.Height <= 0 {
		return area
	}

	return walk.Rectangle{
		X:      area.X - in.left,
		Y:      area.Y - in.top,
		Width:  area.Width + in.left + in.right,
		Height: area.Height + in.top + in.bottom,
	}
}

// layoutArea is the room to arrange the core boxes for when the content of dlg
// changes while it is open. Normally that is the work area of its monitor,
// since fitDialogToContent then resizes the dialog to whatever shape is chosen.
// A snapped dialog is the exception: fitDialogToContent leaves it at the size
// the snap gave it, so its boxes are arranged for that size instead, less the
// width of the vertical scroll bar. A snap fixes the height, so the content
// will usually be taller than it and scroll, and walk reserves no room for that
// bar, the same as capToSize allows for.
//
// A dialog snapped after the content last changed keeps the boxes arranged
// for the size it had then, since resizing never rearranges them.
func layoutArea(dlg *walk.Dialog) walk.Size {
	hwnd := dlg.Handle()
	if !isWindowArranged(hwnd) {
		return workArea(hwnd).Size()
	}

	size := dlg.SizePixels()
	size.Width -= int(win.GetSystemMetricsForDpi(win.SM_CXVSCROLL, uint32(dlg.DPI())))

	return size
}

// fitDialogToContent resizes dlg to the size its content asks for, capped to
// the work area of the monitor dlg is on. Use it whenever widgets are shown or
// hidden at runtime: a ScrollView hides those changes from the layout, so walk
// never resizes the dialog on its own.
//
// Only a dialog in its normal state is resized. One the user has maximized or
// snapped stays exactly where they put it, since resizing it would take it out
// of that state and leave it at neither size; its content is in a ScrollView,
// so all of it is still there to scroll to.
func fitDialogToContent(dlg *walk.Dialog, scroll *walk.ScrollView) {
	if dlg == nil || scroll == nil {
		return
	}

	hwnd := dlg.Handle()

	// Snapped to half the screen, a corner or a column of a snap layout. A
	// snapped window has a size to be restored to as well, but nothing this
	// program can call changes it and leaves the window snapped: the one call
	// that can, ApplyWindowAction, is a Limited Access Feature that needs an
	// unlock token from Microsoft, and SetWindowPlacement would un-snap it. So
	// a snapped dialog is left entirely alone, and un-snaps to the size it had
	// before it was snapped.
	if isWindowArranged(hwnd) {
		return
	}

	// contentDialogSize builds fresh layout items from the widget tree and
	// Win32 reports the window rectangle live, so measuring right after
	// SetVisible works even though the layout itself only runs once the caller
	// returns.
	//
	// Maximized: stay maximized, and change the size it will be restored to
	// instead, so that un-maximizing gives the size the dialog would have had
	// if the change had been made while it was not maximized. Its borders
	// cannot be measured while it is maximized, so this one is fitted to the
	// work area by its rectangle.
	if win.IsZoomed(hwnd) {
		setRestoredSize(hwnd, desiredDialogSize(dlg, scroll, workArea(hwnd).Size()))
		return
	}

	// Let the rectangle reach past the work area by the borders that are not
	// drawn, so a dialog too big for the screen has the frame the user sees
	// against the edges of the work area, the way a snapped window's is.
	area := invisibleFrame(hwnd).outset(workArea(hwnd))
	size := desiredDialogSize(dlg, scroll, area.Size())

	if err := dlg.SetBoundsPixels(centerBounds(dlg.BoundsPixels(), size, area)); err != nil {
		log.Println(err)
	}
}

// setRestoredSize gives a maximized window a new size to be restored to, about
// the centre of the place it was to be restored to, kept inside the work area
// the way centerBounds keeps a normal window, and leaves it maximized.
func setRestoredSize(hwnd win.HWND, size walk.Size) {
	var wp win.WINDOWPLACEMENT
	wp.Length = uint32(unsafe.Sizeof(wp))

	if !win.GetWindowPlacement(hwnd, &wp) {
		log.Println("GetWindowPlacement failed, the dialog keeps the size it was to be restored to")
		return
	}

	// The show state that comes back is SW_SHOWMAXIMIZED, and handing it back
	// unchanged is what keeps the window maximized.
	restored := centerBounds(walk.RectangleFromRECT(wp.RcNormalPosition), size, workspaceArea(hwnd))
	wp.RcNormalPosition = rectFromRectangle(restored)

	if !win.SetWindowPlacement(hwnd, &wp) {
		log.Println("SetWindowPlacement failed, the dialog keeps the size it was to be restored to")
	}
}

// workspaceArea is the work area of the monitor closest to hwnd, in the
// workspace coordinates that the place a maximized window is restored to is
// kept in. The zero Rectangle is returned when the monitor cannot be
// determined, which centerBounds takes as no area to keep the window inside.
//
// Microsoft documents workspace coordinate (0,0) as the top left of the work
// area. This takes that per monitor, as window managers commonly do: screen
// coordinates less the room the taskbar and other appbars take at the top and
// left of the monitor, so the work area starts where the monitor does. Were it
// taken from the main monitor's work area instead, the difference would only
// show on a second monitor while the main one's taskbar is at its top or left,
// and then only as the width of that taskbar.
func workspaceArea(hwnd win.HWND) walk.Rectangle {
	mi, ok := monitorInfo(hwnd)
	if !ok {
		return walk.Rectangle{}
	}

	return workspaceFromMonitor(mi.RcMonitor, mi.RcWork)
}

// workspaceFromMonitor is a monitor's work area in workspace coordinates.
func workspaceFromMonitor(monitor, work win.RECT) walk.Rectangle {
	area := walk.RectangleFromRECT(work)
	area.X, area.Y = int(monitor.Left), int(monitor.Top)

	return area
}

// rectFromRectangle is the win.RECT covering r, the reverse of
// walk.RectangleFromRECT.
func rectFromRectangle(r walk.Rectangle) win.RECT {
	return win.RECT{
		Left:   int32(r.X),
		Top:    int32(r.Y),
		Right:  int32(r.X + r.Width),
		Bottom: int32(r.Y + r.Height),
	}
}

// isWindowArrangedProc is looked up when first used rather than linked, since
// the call has no import library. Microsoft documents it from Windows 10
// version 1903 on.
var isWindowArrangedProc = windows.NewLazySystemDLL("user32.dll").NewProc("IsWindowArranged")

// isWindowArranged says whether the user has snapped hwnd, which Windows treats
// as a state of its own like maximized. A Windows without the call has nothing
// taken as snapped.
func isWindowArranged(hwnd win.HWND) bool {
	if isWindowArrangedProc.Find() != nil {
		return false
	}

	arranged, _, _ := isWindowArrangedProc.Call(uintptr(hwnd))

	return arranged != 0
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

	if err := dlg.SetMinMaxSizePixels(desiredDialogSize(dlg, scroll, workArea(screen).Size()), walk.Size{}); err != nil {
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
