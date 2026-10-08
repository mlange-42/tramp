package ui

import "testing"

func TestDefaultStyleUsesGoMono(t *testing.T) {
	st := DefaultStyle()
	if st.Theme.Face != "Go Mono" {
		t.Errorf("unexpected default typeface %q", st.Theme.Face)
	}
	for _, f := range monoFaces() {
		if f.Font.Typeface != st.Theme.Face {
			t.Errorf("face %v has a different typeface than %q", f.Font, st.Theme.Face)
		}
	}
}
