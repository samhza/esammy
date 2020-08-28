package memegen

import (
	"image"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/samhza/esammy/memegen/assets"
)

var impactFont *truetype.Font
var captionFont *truetype.Font

func init() {
	var err error
	if impactFont, err = truetype.Parse(assets.ImpactTTF); err != nil {
		panic(err)
	}
	if captionFont, err = truetype.Parse(assets.CaptionTTF); err != nil {
		panic(err)
	}
}

// Meme generates the text for an Impact font meme
func Meme(w, h int, top, bot string) image.Image {
	dc := gg.NewContext(w, h)
	dc.Clear()
	face := truetype.NewFace(impactFont, &truetype.Options{Size: float64(h / 12)})
	dc.SetFontFace(face)
	drawOutlinedText(dc, top, float64(w/2), dc.FontHeight()*0.3, 0.5, 0, float64(w))
	drawOutlinedText(dc, bot, float64(w/2), float64(h)-dc.FontHeight()*0.3, 0.5, 1, float64(w))
	return dc.Image()
}

func Caption(w, h int, text string) (image.Image, image.Point) {
	padding := float64(h) / 24
	linespc := 1.2
	face := truetype.NewFace(captionFont, &truetype.Options{Size: float64(h / 8)})

	dc := gg.NewContext(0, 0)
	dc.SetFontFace(face)
	wrapped := dc.WordWrap(text, float64(w))
	_, textH := dc.MeasureMultilineString(strings.Join(wrapped, "\n"), linespc)
	rectH := textH + padding*2
	if int(rectH)%2 != 0 {
		rectH = rectH + 1.0
	}

	dc = gg.NewContext(w, int(rectH)+h)
	dc.SetFontFace(face)
	dc.SetRGB(1, 1, 1)
	dc.DrawRectangle(0, 0, float64(w), rectH)
	dc.Fill()
	dc.SetRGB(0, 0, 0)
	dc.DrawStringWrapped(text, float64(w)/2, rectH/2, 0.5, 0.5, float64(w), linespc, gg.AlignCenter)

	return dc.Image(), image.Point{0, -int(rectH)}
}

func drawOutlinedText(dc *gg.Context, s string, x, y, ax, ay, width float64) {
	dc.SetRGB(0, 0, 0)
	n := width / 300
	const sp float64 = 1.2 // line spacing
	w := width * .95
	dc.DrawStringWrapped(s, x+n, y+n, ax, ay, w, sp, gg.AlignCenter)
	dc.DrawStringWrapped(s, x-n, y-n, ax, ay, w, sp, gg.AlignCenter)
	dc.DrawStringWrapped(s, x+n, y-n, ax, ay, w, sp, gg.AlignCenter)
	dc.DrawStringWrapped(s, x-n, y+n, ax, ay, w, sp, gg.AlignCenter)
	dc.SetRGB(1, 1, 1)
	dc.DrawStringWrapped(s, x, y, ax, ay, w, sp, gg.AlignCenter)
}
