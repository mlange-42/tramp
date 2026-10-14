package ui

import "testing"

func TestDefaultStyleFaces(t *testing.T) {
	st := DefaultStyle()
	if st.Theme.Face != "Go" {
		t.Errorf("unexpected default typeface %q", st.Theme.Face)
	}
	if st.MonoFace != "Go Mono" {
		t.Errorf("unexpected monospaced typeface %q", st.MonoFace)
	}
	if l := st.SmallMonoLabel("x"); l.Font.Typeface != st.MonoFace {
		t.Errorf("mono label has typeface %q", l.Font.Typeface)
	}
}
