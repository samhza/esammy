package memegen

import (
	"image"
	"strings"

	"github.com/golang/freetype/truetype"
	"github.com/samhza/gg"
	"git.sr.ht/~samhza/esammy/memegen/internal/assets"
)

var impactFont, captionFont, timesFont *truetype.Font

func init() {
	var err error
	if impactFont, err = truetype.Parse(assets.ImpactTTF); err != nil {
		panic(err)
	}
	if captionFont, err = truetype.Parse(assets.CaptionTTF); err != nil {
		panic(err)
	}
	if timesFont, err = truetype.Parse(assets.TimesTTF); err != nil {
		panic(err)
	}
}

// Impact generates the text for an Impact font meme.
// The returned image is meant to be used as an src for a call to draw.Draw with
// draw.Over as the op.
func Impact(w, h int, top, bot string) image.Image {
	dc := gg.NewContext(w, h)
	dc.Clear()
	face := truetype.NewFace(impactFont, &truetype.Options{Size: float64(h / 8)})
	dc.SetFontFace(face)
	drawOutlinedText(dc, top, float64(w/2), dc.FontHeight()*0.3, 0.5, 0, float64(w))
	drawOutlinedText(dc, bot, float64(w/2), float64(h)-dc.FontHeight()*0.3, 0.5, 1, float64(w))
	return dc.Image()
}

// Caption makes an iFunny-like caption meant to have another image overlayed
// onto it.
// The returned image and point are meant to be used as dest and sp for a
// call to draw.Draw with draw.Over as the op.
func Caption(w, h int, text string) (image.Image, image.Point) {
	padding := float64(h) / 24
	linespc := 1.2
	face := truetype.NewFace(captionFont, &truetype.Options{Size: float64(h / 10)})

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
	dc.DrawStringWrapped(text, float64(w)/2,
		rectH/2,
		0.5, 0.5,
		float64(w), linespc, gg.AlignCenter)

	return dc.Image(), image.Point{0, -int(rectH)}
}

// Motivate makes a "motivational meme" frame meant to have another image overlayed onto it.
// The returned image and point are meant to be used as dest and sp for a
// call to draw.Draw with draw.Over as the op.
func Motivate(w, h int, top, bot string) (image.Image, image.Point) {
	padding := int(float64(h) / 10)
	linespc := 1.2
	topFace := truetype.NewFace(timesFont, &truetype.Options{Size: float64(h / 8)})
	botFace := truetype.NewFace(timesFont, &truetype.Options{Size: float64(h / 10)})

	dc := gg.NewContext(0, 0)
	dc.SetFontFace(topFace)
	wrapped := dc.WordWrap(top, float64(w))
	_, topH := dc.MeasureMultilineString(strings.Join(wrapped, "\n"), linespc)
	dc.SetFontFace(botFace)
	wrapped = dc.WordWrap(top, float64(w))
	var botH float64
	if strings.TrimSpace(bot) != "" {
		_, botH = dc.MeasureMultilineString(strings.Join(wrapped, "\n"), linespc)
	}

	imgH := h + int(topH) + int(botH) + padding*3
	if botH != 0 {
		imgH += padding
	}
	if int(imgH)%2 != 0 {
		imgH = imgH + 1.0
	}
	imgW := w + padding*2

	dc = gg.NewContext(imgW, imgH)
	dc.SetRGB(0, 0, 0)
	dc.Clear()
	dc.SetRGB(1, 1, 1)
	dc.DrawRectangle(float64(padding)-2, float64(padding)-2, float64(w)+4, float64(h)+4)
	dc.Fill()
	dc.SetRGBA(0, 0, 0, 0)
	dc.DrawRectangle(float64(padding), float64(padding), float64(w), float64(h))
	dc.Fill()
	dc.SetRGB(1, 1, 1)
	dc.SetFontFace(topFace)
	dc.DrawStringWrapped(top, float64(imgW)/2,
		float64(padding*2+h)+topH/2,
		0.5, 0.5,
		float64(imgW), linespc, gg.AlignCenter)
	dc.SetFontFace(botFace)
	if botH != 0 {
		dc.DrawStringWrapped(bot, float64(imgW)/2,
			float64(padding*3+h)+topH+botH/2,
			0.5, 0.5,
			float64(imgW), linespc, gg.AlignCenter)
	}

	return dc.Image(), image.Point{-padding, -padding}
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
