package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"runtime"
	"runtime/debug"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/mlange-42/tramp/assets"
)

// Facts shown in the About window.
const (
	repoURL      = "https://github.com/mlange-42/tramp"
	contactEmail = "tramp@mlange.dev"
	copyright    = "© 2026 Martin Lange · MIT License"
)

// aboutWidth is the largest width of the About window.
const aboutWidth unit.Dp = 480

// aboutScrim darkens the window behind the About window.
var aboutScrim = color.NRGBA{A: 0x60}

// Version returns the version of TRAMP, like "v0.3.0", from the build information Go stamps into the binary:
// the tag of the commit it was built from, or a pseudo-version with the commit's time and hash.
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "development build"
	}
	return info.Main.Version
}

// aboutView is the About window, with the logo, the version and links, drawn above everything else.
type aboutView struct {
	open bool
	// img is the social preview image, decoded when first shown.
	img    paint.ImageOp
	loaded bool
	// The view itself is the event tag of the scrim, closing the window. panel is that of the window.
	panel   int
	close   iconButton
	repo    widget.Clickable
	mail    widget.Clickable
	sources widget.Clickable
}

// updateAbout handles closing the About window and its links.
func (a *App) updateAbout(gtx layout.Context) {
	v := &a.about
	if !v.open {
		return
	}
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: v, Kinds: pointer.Press}); !ok {
			break
		}
		v.open = false
	}
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: &v.panel, Kinds: pointer.Press}); !ok {
			break
		}
	}
	for {
		ev, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			v.open = false
		}
	}
	if v.close.Clicked(gtx) {
		v.open = false
	}
	if v.repo.Clicked(gtx) {
		openURL(repoURL)
	}
	if v.mail.Clicked(gtx) {
		openURL("mailto:" + contactEmail)
	}
	if v.sources.Clicked(gtx) {
		v.open = false
		a.sources.open = true
	}
}

// layoutAbout draws the About window centered in the window, if open.
func (a *App) layoutAbout(gtx layout.Context) {
	v, st := &a.about, a.style
	if !v.open {
		return
	}
	if !v.loaded {
		v.loaded = true
		img, err := png.Decode(bytes.NewReader(assets.Social))
		if err != nil {
			log.Printf("about image: %v", err)
		} else {
			v.img = paint.NewImageOp(img)
		}
	}

	size := gtx.Constraints.Max
	scrim := clip.Rect{Max: size}.Push(gtx.Ops)
	paint.ColorOp{Color: aboutScrim}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	event.Op(gtx.Ops, v)
	scrim.Pop()

	margin := gtx.Dp(st.GroupSpacing)
	w := max(0, min(gtx.Dp(aboutWidth), size.X-2*margin))
	pgtx := gtx
	pgtx.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, max(0, size.Y-2*margin))}
	macro := op.Record(gtx.Ops)
	dims := layoutPanel(pgtx, st, &v.panel, func(gtx layout.Context) layout.Dimensions {
		return a.layoutAboutContent(gtx)
	})
	call := macro.Stop()
	defer op.Offset(image.Pt((size.X-dims.Size.X)/2, max(margin, (size.Y-dims.Size.Y)/2))).Push(gtx.Ops).Pop()
	call.Add(gtx.Ops)
}

func (a *App) layoutAboutContent(gtx layout.Context) layout.Dimensions {
	v, st := &a.about, a.style
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Constraints.Max.X
			b := v.img.Size()
			if b.X == 0 {
				return layout.Dimensions{}
			}
			h := w * b.Y / b.X
			gtx.Constraints = layout.Exact(image.Pt(w, h))
			img := widget.Image{Src: v.img, Fit: widget.Contain, Scale: 1}
			dims := img.Layout(gtx)
			// The close button in the top right corner of the image.
			macro := op.Record(gtx.Ops)
			bd := v.close.LayoutFlat(gtx, st, iconClose, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xc0}, "Close (Esc)")
			call := macro.Stop()
			off := op.Offset(image.Pt(w-bd.Size.X-gtx.Dp(st.Spacing/2), gtx.Dp(st.Spacing/2))).Push(gtx.Ops)
			call.Add(gtx.Ops)
			off.Pop()
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(st.GroupSpacing).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				line := func(w layout.Widget) layout.FlexChild {
					return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Bottom: st.Spacing / 2}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Center.Layout(gtx, w)
						})
					})
				}
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					line(func(gtx layout.Context) layout.Dimensions {
						l := st.Label("Version " + Version())
						l.Font.Weight = font.Bold
						l.Alignment = text.Middle
						return l.Layout(gtx)
					}),
					line(func(gtx layout.Context) layout.Dimensions {
						l := st.SmallLabel("Built with " + runtime.Version())
						l.Color = st.HintFg
						return l.Layout(gtx)
					}),
					line(st.SmallLabel(copyright).Layout),
					layout.Rigid(layout.Spacer{Height: st.Spacing}.Layout),
					line(a.aboutLink(&v.repo, repoURL)),
					line(a.aboutLink(&v.mail, contactEmail)),
					line(a.aboutLink(&v.sources, "Data sources and credits")),
				)
			})
		}),
	)
}

// aboutLink returns a clickable text in the link color.
func (a *App) aboutLink(btn *widget.Clickable, label string) layout.Widget {
	st := a.style
	return func(gtx layout.Context) layout.Dimensions {
		return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			l := st.SmallLabel(label)
			l.Color = st.Theme.ContrastBg
			if btn.Hovered() {
				l.Color = st.Theme.Fg
			}
			pointer.CursorPointer.Add(gtx.Ops)
			return l.Layout(gtx)
		})
	}
}
