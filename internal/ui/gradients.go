package ui

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"slices"
)

// GradientsFile is the name of the gradients configuration file in the settings directory.
const GradientsFile = "gradients.yaml"

// GradientConfig are the available color gradients, as stored in [GradientsFile].
type GradientConfig struct {
	Gradients []GradientDef `yaml:"gradients"`
}

// GradientDef is a color gradient as stored in [GradientsFile].
type GradientDef struct {
	// Name is shown in the gradient drop-down. It must be unique.
	Name string `yaml:"name"`
	// Colors are at least two equally spaced colors, like "#e01010", from low to high values.
	Colors []string `yaml:"colors,flow"`
}

// DefaultGradients returns the built-in gradients.
func DefaultGradients() GradientConfig {
	return GradientConfig{Gradients: []GradientDef{
		{Name: "Turbo", Colors: []string{"#30123b", "#4145ab", "#4675ed", "#39a2fc", "#1bcfd4", "#24eca6", "#61fc6c",
			"#a4fc3b", "#d1e834", "#f3c63a", "#fe9b2d", "#f36315", "#d93806", "#b11901", "#7a0403"}},
		{Name: "Viridis", Colors: []string{"#440154", "#472d7b", "#3b528b", "#2c728e", "#21918c", "#28ae80", "#5ec962", "#addc30", "#fde725"}},
		{Name: "Plasma", Colors: []string{"#0d0887", "#46039f", "#7201a8", "#9c179e", "#bd3786", "#d8576b", "#ed7953", "#fb9f3a", "#fdca26", "#f0f921"}},
		{Name: "Green–Red", Colors: []string{"#1a9850", "#91cf60", "#d9ef8b", "#fee08b", "#fc8d59", "#d73027"}},
		// Diverging, with a gray instead of a white center, which would vanish on bright maps.
		{Name: "Blue–Red", Colors: []string{"#2166ac", "#4393c3", "#92c5de", "#bdbdbd", "#f4a582", "#d6604d", "#b2182b"}},
	}}
}

// Parse returns the valid gradients, and an error for each invalid one.
func (c *GradientConfig) Parse() ([]Gradient, error) {
	var grads []Gradient
	var errs []error
	for i, d := range c.Gradients {
		g, err := d.parse()
		if err == nil && slices.ContainsFunc(grads, func(o Gradient) bool { return o.Name == g.Name }) {
			err = fmt.Errorf("duplicate name %q", g.Name)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("gradient %d: %w", i+1, err))
			continue
		}
		grads = append(grads, g)
	}
	return grads, errors.Join(errs...)
}

func (d *GradientDef) parse() (Gradient, error) {
	if d.Name == "" {
		return Gradient{}, errors.New("missing name")
	}
	if len(d.Colors) < 2 {
		return Gradient{}, fmt.Errorf("%q: needs at least two colors", d.Name)
	}
	g := Gradient{Name: d.Name, Stops: make([]color.NRGBA, len(d.Colors))}
	for i, s := range d.Colors {
		c, err := parseColor(s)
		if err != nil {
			return Gradient{}, fmt.Errorf("%q: %w", d.Name, err)
		}
		g.Stops[i] = c
	}
	return g, nil
}

// builtinGradients are the parsed built-in gradients.
var builtinGradients = func() []Gradient {
	cfg := DefaultGradients()
	g, err := cfg.Parse()
	if err != nil {
		panic(err)
	}
	return g
}()

// Gradient is a color gradient, given by equally spaced color stops.
type Gradient struct {
	Name  string
	Stops []color.NRGBA
}

// At returns the color at position t in [0, 1], interpolated linearly between stops.
func (g *Gradient) At(t float64) color.NRGBA {
	t = math.Max(0, math.Min(1, t))
	n := len(g.Stops) - 1
	if n <= 0 {
		return g.Stops[0]
	}
	i := min(int(t*float64(n)), n-1)
	f := t*float64(n) - float64(i)
	a, b := g.Stops[i], g.Stops[i+1]
	mix := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*f)) }
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: mix(a.A, b.A)}
}

// Bins returns n colors at the centers of n equal parts of the gradient.
func (g *Gradient) Bins(n int) []color.NRGBA {
	cols := make([]color.NRGBA, n)
	for i := range cols {
		cols[i] = g.At((float64(i) + 0.5) / float64(n))
	}
	return cols
}
