package memegen

import (
	"image"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
)

var impactFont *truetype.Font

func init() {
	var err error
	if impactFont, err = truetype.Parse(impactTTF); err != nil {
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
