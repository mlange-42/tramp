package ui

import (
	"slices"
	"testing"

	"github.com/mlange-42/tramp/internal/geo"
	"github.com/mlange-42/tramp/internal/track"
)

// zigRoute returns a route going north, then right, then left, with the given names.
func zigRoute(names ...string) track.Route {
	pos := []geo.LonLat{{Lon: 12, Lat: 51}, {Lon: 12, Lat: 51.01}, {Lon: 12.015, Lat: 51.01}, {Lon: 12.015, Lat: 51.02}, {Lon: 12.015, Lat: 51.03}}
	r := track.Route{}
	for i, p := range pos {
		w := track.Waypoint{Pos: p}
		if i < len(names) {
			w.Name = names[i]
		}
		r.Points = append(r.Points, w)
	}
	return r
}

func routeNames(r *track.Route) []string {
	var n []string
	for _, p := range r.Points {
		n = append(n, p.Name)
	}
	return n
}

func TestAutoRouteNames(t *testing.T) {
	d := &track.File{
		Waypoints: []track.Waypoint{{Name: "Hut", Pos: geo.LonLat{Lon: 12.015, Lat: 51.02}}},
		// The fourth point is linked to the waypoint, the third has a name of its own.
		Routes: []track.Route{zigRoute("", "", "Bridge", "Hut", "")},
	}
	names, changed := autoRouteNames(d, 0, 22.5)
	if want := []string{"01 Start", "02 R", "Bridge", "Hut", "05 End"}; !changed || !slices.Equal(names, want) {
		t.Fatalf("expected %v, got %v", want, names)
	}
	nameRoutePoints(d, 0, 22.5)
	if !autoNamed(&d.Routes[0]) {
		t.Error("expected the route named automatically")
	}
	if _, changed := autoRouteNames(d, 0, 22.5); changed {
		t.Error("expected no change when naming again")
	}

	// A second route gets unique names.
	d.Routes = append(d.Routes, zigRoute())
	nameRoutePoints(d, 1, 22.5)
	if want := []string{"01 Start (2)", "02 R (2)", "03 L", "04", "05 End (2)"}; !slices.Equal(routeNames(&d.Routes[1]), want) {
		t.Errorf("expected %v, got %v", want, routeNames(&d.Routes[1]))
	}
	// Other routes are not named automatically.
	d.Routes = append(d.Routes, zigRoute("A", "04"))
	updateAutoNames(d, 22.5)
	if routeNames(&d.Routes[2])[1] != "04" {
		t.Error("expected a route without generated start or end unchanged")
	}
}

func TestAutoNamesOnChange(t *testing.T) {
	a, f := editApp(t, testPlanGPX)
	a.setEditing(f)
	f.edit.slightAngle = 22.5
	f.change(func(d *track.File) { d.Routes[0] = zigRoute() })
	a.props.fields[routeNameField].bound = false
	a.editor.sel, a.editor.hasSel = vertex{route: 0, point: 0}, true
	a.nameRoute()
	if want := []string{"01 Start", "02 R", "03 L", "04", "05 End"}; !slices.Equal(routeNames(&f.data.Routes[0]), want) {
		t.Fatalf("expected %v, got %v", want, routeNames(&f.data.Routes[0]))
	}
	// Deleting a point renumbers the others, and updates the turns.
	f.change(func(d *track.File) { d.Routes[0].Points = slices.Delete(d.Routes[0].Points, 1, 2) })
	if want := []string{"01 Start", "02 SL", "03", "04 End"}; !slices.Equal(routeNames(&f.data.Routes[0]), want) {
		t.Errorf("expected %v after deleting, got %v", want, routeNames(&f.data.Routes[0]))
	}
	// Appending a point names it, and the old end becomes a normal point. A custom name is kept.
	f.change(func(d *track.File) {
		d.Routes[0].Points[1].Name = "Bridge"
		d.Routes[0].Points = append(d.Routes[0].Points, track.Waypoint{Pos: geo.LonLat{Lon: 12.03, Lat: 51.03}})
	})
	if want := []string{"01 Start", "Bridge", "03", "04 R", "05 End"}; !slices.Equal(routeNames(&f.data.Routes[0]), want) {
		t.Errorf("expected %v after appending, got %v", want, routeNames(&f.data.Routes[0]))
	}
	// Naming is undone in one step with the change.
	f.undo()
	if want := []string{"01 Start", "02 SL", "03", "04 End"}; !slices.Equal(routeNames(&f.data.Routes[0]), want) {
		t.Errorf("expected %v after undo, got %v", want, routeNames(&f.data.Routes[0]))
	}
}
