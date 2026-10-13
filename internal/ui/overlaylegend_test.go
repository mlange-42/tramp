package ui

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mlange-42/tramp/internal/wms"
)

// waitLegend waits for the legend image of an overlay to load.
func waitLegend(a *App, i int) {
	for a.overlays[i].legend.loading {
		a.runUITasks()
		time.Sleep(time.Millisecond)
	}
}

func TestOverlayLegend(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 30, 60))); err != nil {
		t.Fatal(err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/legend":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(buf.Bytes())
		case "/caps":
			_, _ = w.Write([]byte(`<WMS_Capabilities><Capability><Layer>
<Layer><Name>A</Name><Style><LegendURL><OnlineResource href="` + srv.URL + `/legend"/></LegendURL></Style></Layer>
<Layer><Name>B</Name><Style><LegendURL><OnlineResource href="` + srv.URL + `/broken"/></LegendURL></Style></Layer>
</Layer></Capability></WMS_Capabilities>`))
		default:
			http.Error(w, "nope", http.StatusNotFound)
		}
	}))
	defer srv.Close()
	a := newApp(func() {}, Options{
		Maps: []Map{{Layer: wms.Layer{Name: "None", Type: wms.TypeNone}}},
		Overlays: []Overlay{
			{Layer: wms.Layer{Name: "A", URL: srv.URL, Capabilities: srv.URL + "/caps", Layers: "A"}, Refresh: time.Hour},
			{Layer: wms.Layer{Name: "B", URL: srv.URL, Capabilities: srv.URL + "/caps", Layers: "B"}, Refresh: time.Hour},
		},
	})
	defer a.closeTiles()
	for i := range a.overlays {
		a.setOverlay(i, true)
		waitOverlayTime(a, i)
	}
	la, lb := &a.overlays[0].legend, &a.overlays[1].legend
	if la.url != srv.URL+"/legend" || !la.available() || la.size != (image.Point{}) {
		t.Fatalf("unexpected legend %+v", la)
	}

	// The image is loaded when first shown.
	a.toggleLegend(0)
	waitLegend(a, 0)
	if !la.shown || la.size != image.Pt(30, 60) {
		t.Fatalf("legend not loaded: %+v", la)
	}
	// Only one legend is shown, and a broken one is not offered again.
	a.toggleLegend(1)
	waitLegend(a, 1)
	if la.shown || !lb.shown || lb.available() {
		t.Errorf("unexpected legends %+v %+v", la, lb)
	}
	a.toggleLegend(0)
	a.toggleLegend(0)
	if la.shown || lb.shown {
		t.Error("legend should be hidden after a second click")
	}

	// A new address drops the image.
	la.setURL(srv.URL + "/other")
	if la.size != (image.Point{}) || !la.available() {
		t.Errorf("unexpected legend after new address %+v", la)
	}
	la.setURL("")
	if la.available() {
		t.Error("legend without address should not be available")
	}
}
