package main

import (
	"testing"

	"github.com/tailscale/walk"
)

func TestCapToSize(t *testing.T) {
	// Scrollbar thickness as GetSystemMetricsForDpi reports it at 96 dpi.
	const vScroll, hScroll = 17, 17

	// 1080p minus a taskbar at the bottom.
	available := walk.Size{Width: 1920, Height: 1040}

	tests := []struct {
		name      string
		size      walk.Size
		available walk.Size
		want      walk.Size
	}{
		{
			name:      "fits",
			size:      walk.Size{Width: 700, Height: 900},
			available: available,
			want:      walk.Size{Width: 700, Height: 900},
		},
		{
			name:      "exact fit is not capped",
			size:      walk.Size{Width: 1920, Height: 1040},
			available: available,
			want:      walk.Size{Width: 1920, Height: 1040},
		},
		{
			name:      "too tall reserves the vertical scrollbar",
			size:      walk.Size{Width: 700, Height: 2000},
			available: available,
			want:      walk.Size{Width: 700 + vScroll, Height: 1040},
		},
		{
			name:      "too wide reserves the horizontal scrollbar",
			size:      walk.Size{Width: 3000, Height: 500},
			available: available,
			want:      walk.Size{Width: 1920, Height: 500 + hScroll},
		},
		{
			name:      "too wide and too tall is capped to the work area",
			size:      walk.Size{Width: 3000, Height: 2000},
			available: available,
			want:      walk.Size{Width: 1920, Height: 1040},
		},
		{
			name:      "scrollbar reservation never pushes past the work area",
			size:      walk.Size{Width: 1915, Height: 2000},
			available: available,
			want:      walk.Size{Width: 1920, Height: 1040},
		},
		{
			name: "no monitor leaves the size alone",
			size: walk.Size{Width: 3000, Height: 2000},
			want: walk.Size{Width: 3000, Height: 2000},
			// available stays zero: GetMonitorInfo failed.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := capToSize(tt.size, tt.available, vScroll, hScroll); got != tt.want {
				t.Errorf("capToSize(%v, %v) = %v, want %v", tt.size, tt.available, got, tt.want)
			}
		})
	}
}

func TestCenterBounds(t *testing.T) {
	// 1080p with the taskbar at the bottom.
	area := walk.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040}

	tests := []struct {
		name string
		old  walk.Rectangle
		size walk.Size
		area walk.Rectangle
		want walk.Rectangle
	}{
		{
			name: "growing keeps the center",
			old:  walk.Rectangle{X: 810, Y: 470, Width: 300, Height: 100},
			size: walk.Size{Width: 500, Height: 300},
			area: area,
			want: walk.Rectangle{X: 710, Y: 370, Width: 500, Height: 300},
		},
		{
			name: "shrinking keeps the center",
			old:  walk.Rectangle{X: 710, Y: 370, Width: 500, Height: 300},
			size: walk.Size{Width: 300, Height: 100},
			area: area,
			want: walk.Rectangle{X: 810, Y: 470, Width: 300, Height: 100},
		},
		{
			name: "growing past the taskbar moves the dialog up",
			old:  walk.Rectangle{X: 800, Y: 900, Width: 300, Height: 100},
			size: walk.Size{Width: 300, Height: 400},
			area: area,
			want: walk.Rectangle{X: 800, Y: 640, Width: 300, Height: 400},
		},
		{
			name: "a dialog taller than the work area starts at its top",
			old:  walk.Rectangle{X: 800, Y: 500, Width: 300, Height: 100},
			size: walk.Size{Width: 300, Height: 1040},
			area: area,
			want: walk.Rectangle{X: 800, Y: 0, Width: 300, Height: 1040},
		},
		{
			name: "a dialog wider than the work area starts at its left",
			old:  walk.Rectangle{X: 800, Y: 500, Width: 300, Height: 100},
			size: walk.Size{Width: 2400, Height: 300},
			area: area,
			want: walk.Rectangle{X: 0, Y: 400, Width: 2400, Height: 300},
		},
		{
			name: "the work area offset of a second monitor is honored",
			old:  walk.Rectangle{X: 2400, Y: 300, Width: 300, Height: 100},
			size: walk.Size{Width: 800, Height: 1000},
			area: walk.Rectangle{X: 1920, Y: 0, Width: 1280, Height: 1000},
			want: walk.Rectangle{X: 2150, Y: 0, Width: 800, Height: 1000},
		},
		{
			name: "a taskbar at the top pushes the dialog down",
			old:  walk.Rectangle{X: 800, Y: 60, Width: 300, Height: 100},
			size: walk.Size{Width: 300, Height: 400},
			area: walk.Rectangle{X: 0, Y: 40, Width: 1920, Height: 1040},
			want: walk.Rectangle{X: 800, Y: 40, Width: 300, Height: 400},
		},
		{
			name: "no monitor only re-centers",
			old:  walk.Rectangle{X: 10, Y: 10, Width: 300, Height: 100},
			size: walk.Size{Width: 900, Height: 700},
			area: walk.Rectangle{},
			want: walk.Rectangle{X: -290, Y: -290, Width: 900, Height: 700},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := centerBounds(tt.old, tt.size, tt.area); got != tt.want {
				t.Errorf("centerBounds(%v, %v, %v) = %v, want %v", tt.old, tt.size, tt.area, got, tt.want)
			}
		})
	}
}

func TestClampInt(t *testing.T) {
	tests := []struct {
		value, lo, hi, want int
	}{
		{value: 5, lo: 0, hi: 10, want: 5},
		{value: -5, lo: 0, hi: 10, want: 0},
		{value: 15, lo: 0, hi: 10, want: 10},
		{value: 0, lo: 0, hi: 10, want: 0},
		{value: 10, lo: 0, hi: 10, want: 10},
		// A window larger than the work area gives hi < lo, and lo has to win
		// so the caption bar stays on screen.
		{value: 5, lo: 0, hi: -200, want: 0},
	}

	for _, tt := range tests {
		if got := clampInt(tt.value, tt.lo, tt.hi); got != tt.want {
			t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tt.value, tt.lo, tt.hi, got, tt.want)
		}
	}
}
