package ui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/gps"
	"github.com/mlange-42/tramp/internal/settings"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

const (
	// gpsFollowZoom is the zoom level the map zooms in to, at least, when it starts following the position.
	gpsFollowZoom = 15
	// gpsMinSpeed is the speed in m/s above which the direction of movement is shown.
	gpsMinSpeed = 0.5
)

var (
	iconGPSOff       = mustIcon(icons.DeviceGPSOff)
	iconGPSSearching = mustIcon(icons.DeviceGPSNotFixed)
	iconGPSFixed     = mustIcon(icons.DeviceGPSFixed)
	iconGPSFollow    = mustIcon(icons.MapsMyLocation)
)

// gpsView shows the live position from a GPS receiver on the map, and follows it.
type gpsView struct {
	cfg settings.GPS
	// rec is the running receiver, or nil while the GPS is off.
	rec *gps.Receiver
	// prev is the last stopped receiver, which may still be closing its port.
	prev *gps.Receiver
	// status is the receiver status of the current frame.
	status gps.Status
	// following keeps the map centered on the position. It ends when the map is panned.
	following bool
	// zoomed is set once the map was zoomed in on the position after starting to follow it.
	zoomed bool
	btn    iconButton
}

func (a *App) startGPS() {
	g := &a.gps
	g.rec = gps.Start(gps.Config{Port: g.cfg.Port, Baud: g.cfg.Baud}, a.invalidate, g.prev)
	g.following, g.zoomed = true, false
}

// stopGPS stops the receiver, if running.
func (a *App) stopGPS() {
	g := &a.gps
	if g.rec == nil {
		return
	}
	g.rec.Stop()
	g.prev, g.rec = g.rec, nil
	g.following = false
	g.status = gps.Status{}
}

// updateGPS handles the GPS button, and keeps the map centered on the position while following it.
func (a *App) updateGPS(gtx layout.Context) {
	g := &a.gps
	if g.btn.Clicked(gtx) {
		switch {
		case g.rec == nil:
			a.startGPS()
		case g.following:
			a.stopGPS()
		default:
			g.following = true
		}
	}
	if a.mapView.Dragged() {
		g.following = false
	}
	if g.rec == nil {
		return
	}
	g.status = g.rec.Status()
	if !g.following || !g.status.HasFix {
		return
	}
	// Re-centering every frame also keeps the position centered when zooming with the mouse wheel.
	a.mapView.View.Center = geo.ToMercator(g.status.Fix.Pos)
	if !g.zoomed {
		g.zoomed = true
		if a.mapView.View.Zoom < gpsFollowZoom {
			a.mapView.SetZoom(gpsFollowZoom)
		}
	}
}

// gpsStatus returns the GPS state for the status bar, or an empty string while the GPS is off.
func (a *App) gpsStatus() string {
	g := &a.gps
	if g.rec == nil {
		return ""
	}
	s := g.status
	switch {
	case s.State == gps.Connected && s.Valid:
		f := s.Fix
		str := "GPS " + f.Pos.String()
		if f.Accuracy > 0 {
			str += fmt.Sprintf(" ±%.0f m", f.Accuracy)
		}
		if f.HasCourse {
			str += fmt.Sprintf(" %.1f km/h", f.Speed*3.6)
		}
		return str
	case s.State == gps.Connected:
		return fmt.Sprintf("GPS %s: waiting for fix", s.Port)
	case s.Port != "":
		return fmt.Sprintf("GPS: searching %s", s.Port)
	case errors.Is(s.Err, gps.ErrNotFound):
		return "GPS: no receiver found"
	case s.Err != nil:
		return "GPS: " + s.Err.Error()
	}
	return "GPS: searching"
}

// layoutGPSButton draws the round GPS button at the bottom right of the map.
func (a *App) layoutGPSButton(gtx layout.Context) {
	g := &a.gps
	st := a.style
	ic, col, tip := iconGPSOff, st.Theme.Fg, "Show the live position of a GPS receiver, connected by USB, Bluetooth or network"
	switch {
	case g.rec == nil:
	case g.following && g.status.Valid:
		ic, col, tip = iconGPSFollow, st.GPSColor, "Following the GPS position. Click to turn the GPS off"
	case g.following:
		ic, col, tip = iconGPSSearching, st.GPSColor, "Waiting for a GPS position. Click to turn the GPS off"
	case g.status.HasFix:
		ic, tip = iconGPSFixed, "Center the map on the GPS position, and follow it"
	default:
		ic, tip = iconGPSSearching, "Follow the GPS position once it is available"
	}
	g.btn.tip.above, g.btn.tip.right = true, true
	layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(st.Spacing*2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			avail := gtx.Constraints.Max.X
			dims := g.btn.btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				size := gtx.Dp(st.GPSButtonSize)
				box := image.Rectangle{Max: image.Pt(size, size)}
				shadow := box.Add(image.Pt(0, gtx.Dp(1))).Inset(-gtx.Dp(1))
				paint.FillShape(gtx.Ops, color.NRGBA{A: 0x40}, clip.Ellipse(shadow).Op(gtx.Ops))
				bg := st.PanelBg
				if g.btn.btn.Hovered() {
					bg = st.DragBg
				}
				paint.FillShape(gtx.Ops, bg, clip.Ellipse(box).Op(gtx.Ops))
				is := gtx.Dp(st.GPSIconSize)
				layoutIcon(gtx, ic, image.Pt((size-is)/2, (size-is)/2), is, col)
				defer clip.Ellipse(box).Push(gtx.Ops).Pop()
				pointer.CursorPointer.Add(gtx.Ops)
				return layout.Dimensions{Size: box.Max}
			})
			g.btn.tip.Layout(gtx, st, g.btn.btn.Hovered(), dims.Size, avail, tip)
			return dims
		})
	})
}

// layoutGPSMarker draws the GPS position on the map: a dot with a circle for the accuracy,
// and an arrow for the direction of movement.
func (a *App) layoutGPSMarker(gtx layout.Context) {
	g := &a.gps
	s := g.status
	if g.rec == nil || !s.HasFix {
		return
	}
	st := a.style
	v := &a.mapView.View
	f := s.Fix
	x, y := v.ToScreen(geo.ToMercator(f.Pos))
	c := f32.Pt(float32(x), float32(y))
	r := float32(gtx.Dp(st.GPSDotSize)) / 2
	ring := float32(max(1, gtx.Dp(2)))

	col := st.GPSColor
	if s.State != gps.Connected || !s.Valid {
		col = st.GPSStale
	}
	if f.Accuracy > 0 && s.Valid {
		// Web Mercator stretches distances by 1/cos(latitude).
		metersPerPx := v.Resolution() * math.Cos(f.Pos.Lat*math.Pi/180)
		ar := float32(f.Accuracy / metersPerPx)
		// Don't draw circles far larger than the view.
		limit := float32(v.Size.X + v.Size.Y)
		if ar > r+ring && ar < limit {
			paint.FillShape(gtx.Ops, st.GPSAccuracy, circle(c, ar).Op(gtx.Ops))
		}
	}
	if s.Valid && f.HasCourse && f.Speed > gpsMinSpeed {
		// An arrow head outside the ring, pointing in the direction of movement.
		rad := f.Course * math.Pi / 180
		dir := f32.Pt(float32(math.Sin(rad)), -float32(math.Cos(rad)))
		side := f32.Pt(-dir.Y, dir.X)
		base, tip, w := r+ring, (r+ring)*1.9, (r+ring)*0.75
		var p clip.Path
		p.Begin(gtx.Ops)
		p.MoveTo(c.Add(dir.Mul(tip)))
		p.LineTo(c.Add(dir.Mul(base * 0.6)).Add(side.Mul(w)))
		p.LineTo(c.Add(dir.Mul(base)))
		p.LineTo(c.Add(dir.Mul(base * 0.6)).Sub(side.Mul(w)))
		p.Close()
		paint.FillShape(gtx.Ops, col, clip.Outline{Path: p.End()}.Op())
	}
	paint.FillShape(gtx.Ops, color.NRGBA{A: 0x40}, circle(c.Add(f32.Pt(0, 1)), r+ring+1).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, st.HoverRing, circle(c, r+ring).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, col, circle(c, r).Op(gtx.Ops))
}

// circle returns a circle with center c and radius r.
func circle(c f32.Point, r float32) clip.Ellipse {
	fl := func(v float32) int { return int(math.Floor(float64(v))) }
	ce := func(v float32) int { return int(math.Ceil(float64(v))) }
	return clip.Ellipse(image.Rect(fl(c.X-r), fl(c.Y-r), ce(c.X+r), ce(c.Y+r)))
}
