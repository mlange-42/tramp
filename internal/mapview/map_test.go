package mapview

import (
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"
)

func TestDoubleClick(t *testing.T) {
	var m Map
	click := func(ms int, x float32) bool {
		m.click(pointer.Event{Time: time.Duration(ms) * time.Millisecond, Position: f32.Pt(x, 0)})
		return m.DoubleClicked()
	}
	if click(1000, 0) {
		t.Error("single click detected as double click")
	}
	if !click(1200, 2) {
		t.Error("double click not detected")
	}
	// A third click starts a new double click.
	if click(1300, 2) {
		t.Error("third click detected as double click")
	}
	if click(2000, 2) {
		t.Error("slow clicks detected as double click")
	}
	if click(2100, 20) {
		t.Error("distant clicks detected as double click")
	}
}
