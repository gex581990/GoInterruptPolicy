package main

import (
	"fmt"
	"log"
	"os"

	"github.com/tailscale/walk"
)

// logging is on only when -log named a file. The release build is linked with
// -H windowsgui and so has no console for the standard logger to write to,
// which is why this writes to a file rather than to stderr.
var logging bool

// openLogFile points the standard logger at the file -log named, so that
// everything the program already logs lands there along with the layout
// diagnostics below.
func openLogFile() {
	if flagLog == "" {
		return
	}

	file, err := os.Create(flagLog)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot write the log:", err)
		return
	}

	log.SetOutput(file)
	logging = true

	log.Printf("log started, %s", os.Args)
}

// logf records a line of diagnostics, and costs nothing when -log was not given.
func logf(format string, args ...any) {
	if !logging {
		return
	}

	log.Output(2, fmt.Sprintf(format, args...))
}

// logSize renders a size the way the log wants it.
func logSize(s walk.Size) string {
	return fmt.Sprintf("%dx%d", s.Width, s.Height)
}

// logRect renders a rectangle the way the log wants it.
func logRect(r walk.Rectangle) string {
	return fmt.Sprintf("%dx%d at %d,%d", r.Width, r.Height, r.X, r.Y)
}

// logMachine records what the dialog is being laid out for, once, so the rest
// of the log can be read against it.
func logMachine(dlg *walk.Dialog, screen walk.Rectangle) {
	if !logging {
		return
	}

	logf("machine: %d threads over %d groups, %d skipped, %d threads per core, core groups %+v",
		cs.Threads, cs.Groups, cs.Skipped, cs.MaxThreadsPerCore, cs.CoreGroups)
	logf("screen: work area %s, dialog dpi %d", logRect(screen), dlg.DPI())
}

// logGrids records what the core grids look like right now.
func logGrids(when string, grids []*walk.Composite) {
	if !logging {
		return
	}

	if len(grids) == 0 {
		logf("%s: no core grids found, nothing to reflow", when)
		return
	}

	for i, grid := range grids {
		logf("%s: grid %d has %d boxes over %d columns, bounds %s, min %s",
			when, i, grid.Children().Len(), gridColumns(grid),
			logRect(grid.BoundsPixels()), logSize(grid.MinSizeHint()))
	}
}

// logDialog records the sizes that decide whether anything is cut off: what the
// content wants, what the dialog actually is, and how much of it the ScrollView
// is showing. A viewport shorter than the content is exactly the cut off.
func logDialog(when string, dlg *walk.Dialog, scroll *walk.ScrollView, body *walk.Composite) {
	if !logging {
		return
	}

	logf("%s: dialog %s, client %s", when,
		logSize(dlg.SizePixels()), logSize(dlg.ClientBoundsPixels().Size()))
	logf("%s: content wants %s, scroll viewport %s showing content %s",
		when, logSize(contentDialogSize(dlg, scroll)),
		logSize(scroll.ClientBoundsPixels().Size()), logSize(scroll.SizeHint()))
	logf("%s: body min %s, pinned to max %s, body bounds %s",
		when, logSize(body.MinSizeHint()), logSize(body.MaxSizePixels()),
		logRect(body.BoundsPixels()))
}
