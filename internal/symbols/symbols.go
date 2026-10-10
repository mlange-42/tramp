// Package symbols maps waypoint symbol names to icons.
//
// GPX does not define symbol names: the sym element holds the name of the symbol as shown by the device.
// The names used by Garmin devices are the de facto standard, and understood by most applications.
// This package knows a selection of them, relevant outdoors, and common aliases.
// Names are matched ignoring case and everything but letters and digits.
//
// The icons are from Maki (https://github.com/mapbox/maki), CC0, converted to IconVG by gen/main.go.
package symbols

//go:generate go run ./gen

import (
	"fmt"
	"image/color"
	"strings"
	"unicode"

	"gioui.org/widget"
)

// Symbol is a known waypoint symbol.
type Symbol struct {
	// Name is the canonical name, as used by Garmin devices.
	Name string
	// Category is the group the symbol is listed in, like "Outdoors".
	Category string
	// Aliases are other names that refer to the symbol.
	Aliases []string
	// Icon is drawn in the marker of the point. Nil for symbols drawn as plain dots, like "Waypoint".
	Icon *widget.Icon
	// Color is the marker color of colored symbols like "Flag, Blue", or zero for the color of the item.
	Color color.NRGBA
}

// Matches reports whether the name or an alias of the symbol contains the query,
// ignoring case and everything but letters and digits.
func (s *Symbol) Matches(query string) bool {
	q := key(query)
	if strings.Contains(key(s.Name), q) {
		return true
	}
	for _, a := range s.Aliases {
		if strings.Contains(key(a), q) {
			return true
		}
	}
	return false
}

// Unknown is drawn for symbol names that are not known.
var Unknown = &Symbol{Icon: mustIcon("marker")}

// Lookup returns the symbol with the given name or alias, or false if the name is not known.
func Lookup(name string) (*Symbol, bool) {
	s, ok := byKey[key(name)]
	return s, ok
}

// All returns the known symbols, in catalog order, grouped by category.
func All() []*Symbol {
	return all
}

// key normalizes a symbol name for matching: lower case, with only letters and digits.
func key(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, name)
}

var (
	blue  = color.NRGBA{R: 0x1f, G: 0x6f, B: 0xd1, A: 0xff}
	green = color.NRGBA{R: 0x2a, G: 0x9d, B: 0x3a, A: 0xff}
	red   = color.NRGBA{R: 0xd6, G: 0x28, B: 0x28, A: 0xff}
)

// def is a catalog entry: a canonical name, the Maki icon, and aliases.
type def struct {
	name    string
	icon    string
	color   color.NRGBA
	aliases []string
}

func d(name, icon string, aliases ...string) def {
	return def{name: name, icon: icon, aliases: aliases}
}

// category is a group of catalog entries.
type category struct {
	name string
	defs []def
}

// catalog are the known symbols. Canonical names are Garmin's where Garmin has the symbol.
var catalog = []category{
	{"", []def{
		d("Waypoint", "", "Dot", "Default"),
	}},
	{"Outdoors", []def{
		d("Summit", "mountain", "Peak", "Mountain", "Top"),
		d("Scenic Area", "viewpoint", "Viewpoint", "View", "Vista", "Lookout"),
		d("Trail Head", "shoe", "Start"),
		d("Campground", "campsite", "Camping", "Campsite", "Camp", "Tent"),
		d("Lodging", "lodging", "Lodge", "Hotel", "Hut", "Alpine Hut", "Hostel", "Accommodation"),
		d("Shelter", "shelter"),
		d("Picnic Area", "picnic-site", "Picnic"),
		d("Drinking Water", "drinking-water", "Water", "Water Source", "Spring", "Fountain"),
		d("Restroom", "toilet", "Toilet", "Toilets", "WC"),
		d("Danger Area", "danger", "Danger", "Warning", "Caution", "Hazard", "Skull and Crossbones"),
		d("Information", "information", "Info"),
		d("Gate", "gate"),
		d("Bridge", "bridge"),
		d("Tunnel", "tunnel"),
		d("Dam", "dam"),
		d("Waterfall", "waterfall"),
		d("Beach", "beach"),
		d("Swimming Area", "swimming", "Swimming", "Swim"),
		d("Ski Resort", "skiing", "Skiing", "Ski"),
		d("Park", "park"),
		d("Forest", "park-alt1", "Tree", "Woods"),
	}},
	{"Food and services", []def{
		d("Restaurant", "restaurant", "Food", "Dining"),
		d("Fast Food", "fast-food"),
		d("Cafe", "cafe", "Coffee", "Coffee Shop"),
		d("Bar", "bar", "Pub", "Beer"),
		d("Shopping Center", "shop", "Shop", "Shopping"),
		d("Convenience Store", "convenience"),
		d("Medical Facility", "hospital", "Hospital", "First Aid", "Doctor"),
		d("Police Station", "police", "Police"),
		d("Post Office", "post", "Post"),
		d("Bank", "bank", "ATM"),
		d("Telephone", "telephone", "Phone"),
	}},
	{"Transport", []def{
		d("Parking Area", "parking", "Parking", "Car Park"),
		d("Gas Station", "fuel", "Fuel", "Petrol"),
		d("Car", "car"),
		d("Car Rental", "car-rental"),
		d("Car Repair", "car-repair", "Garage"),
		d("Bike Trail", "bicycle", "Bicycle", "Bike"),
		d("Ground Transportation", "bus", "Bus", "Bus Stop"),
		d("Train", "rail", "Railway", "Train Station", "Station"),
		d("Airport", "airport"),
		d("Boat Ramp", "slipway", "Slipway"),
		d("Marina", "harbor", "Harbor", "Harbour", "Anchor"),
	}},
	{"Places", []def{
		d("City (Large)", "city", "City"),
		d("City (Medium)", "town", "Town"),
		d("City (Small)", "village", "Village"),
		d("Residence", "home", "Home", "House"),
		d("Building", "building"),
		d("Church", "place-of-worship", "Place of Worship", "Chapel"),
		d("Cemetery", "cemetery"),
		d("Museum", "museum"),
		d("School", "school"),
		d("Tall Tower", "communications-tower", "Tower"),
		d("Light", "lighthouse", "Lighthouse"),
		d("Golf Course", "golf", "Golf"),
		d("Stadium", "stadium"),
		d("Zoo", "zoo"),
		d("Amusement Park", "amusement-park"),
		d("Movie Theater", "cinema", "Cinema"),
		d("Live Theater", "theatre", "Theater", "Theatre"),
	}},
	{"Markers", markers()},
}

// markers returns the flag and pin, and Garmin's colored shapes.
func markers() []def {
	defs := []def{d("Flag", "embassy"), d("Pin", "marker")}
	for _, shape := range []struct{ name, icon string }{{"Flag", "embassy"}, {"Pin", "marker"}, {"Block", "square"}, {"Diamond", "diamond"}} {
		for _, c := range []struct {
			name string
			col  color.NRGBA
		}{{"Blue", blue}, {"Green", green}, {"Red", red}} {
			defs = append(defs, def{name: shape.name + ", " + c.name, icon: shape.icon, color: c.col})
		}
	}
	return defs
}

var (
	all   []*Symbol
	byKey = map[string]*Symbol{}
)

func init() {
	for _, c := range catalog {
		for _, df := range c.defs {
			s := &Symbol{Name: df.name, Category: c.name, Aliases: df.aliases, Color: df.color}
			if df.icon != "" {
				s.Icon = mustIcon(df.icon)
			}
			all = append(all, s)
			for _, n := range append([]string{df.name}, df.aliases...) {
				k := key(n)
				if _, ok := byKey[k]; ok {
					panic(fmt.Sprintf("duplicate symbol name %q", n))
				}
				byKey[k] = s
			}
		}
	}
}

// icons caches the icons by Maki name, as several symbols share icons.
var icons = map[string]*widget.Icon{}

func mustIcon(name string) *widget.Icon {
	if ic, ok := icons[name]; ok {
		return ic
	}
	data, ok := makiIcons[name]
	if !ok {
		panic(fmt.Sprintf("unknown icon %q", name))
	}
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic(err)
	}
	icons[name] = ic
	return ic
}
