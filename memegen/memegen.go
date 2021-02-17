package memegen

import (
	_ "embed"
	"image"
	"strings"

	"github.com/golang/freetype/truetype"
	"github.com/samhza/gg"
	"golang.org/x/image/font"
)

var impactFont, captionFont, timesFont *truetype.Font

//go:embed assets/impact.ttf
var impactTTF []byte

//go:embed assets/caption.ttf
var captionTTF []byte

//go:embed assets/times.ttf
var timesTTF []byte

func init() {
	var err error
	if impactFont, err = truetype.Parse(impactTTF); err != nil {
		panic(err)
	}
	if captionFont, err = truetype.Parse(captionTTF); err != nil {
		panic(err)
	}
	if timesFont, err = truetype.Parse(timesTTF); err != nil {
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
	limitWrappedTextHeight(dc, impactFont, top, float64(w), float64(h)/2, 1.0)
	drawOutlinedText(dc, top, float64(w/2), dc.FontHeight()*0.3, 0.5, 0, float64(w), 1)
	face = truetype.NewFace(impactFont, &truetype.Options{Size: float64(h / 8)})
	dc.SetFontFace(face)
	limitWrappedTextHeight(dc, impactFont, bot, float64(w), float64(h)/2, 1.0)
	drawOutlinedText(dc, bot, float64(w/2), float64(h)-dc.FontHeight()*0.3, 0.5, 1, float64(w), 1)
	return dc.Image()
}

// Caption makes an iFunny-like caption meant to have another image overlayed
// onto it.
// The returned image and point are meant to be used as dest and sp for a
// call to draw.Draw with draw.Over as the op.
func Caption(w, h int, text string) (image.Image, image.Point) {
	linespc := 1.2

	dc := gg.NewContext(0, 0)
	face := truetype.NewFace(captionFont, &truetype.Options{Size: float64(w) / 10})
	dc.SetFontFace(face)
	sizedFace, textH := limitWrappedTextHeight(dc, captionFont, text, float64(w), float64(h)/2, 1.0)
	if sizedFace != nil {
		face = sizedFace
	}
	padding := dc.FontHeight() / 2
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

func drawOutlinedText(dc *gg.Context, s string,
	x, y, ax, ay, width float64, sp float64) {
	dc.SetRGB(0, 0, 0)
	n := dc.FontHeight() / 20
	w := width * .95
	dc.DrawStringWrapped(s, x+n, y+n, ax, ay, w, sp, gg.AlignCenter)
	dc.DrawStringWrapped(s, x-n, y-n, ax, ay, w, sp, gg.AlignCenter)
	dc.DrawStringWrapped(s, x+n, y-n, ax, ay, w, sp, gg.AlignCenter)
	dc.DrawStringWrapped(s, x-n, y+n, ax, ay, w, sp, gg.AlignCenter)
	dc.SetRGB(1, 1, 1)
	dc.DrawStringWrapped(s, x, y, ax, ay, w, sp, gg.AlignCenter)
}

func limitWrappedTextHeight(dc *gg.Context,
	fontf *truetype.Font, text string, w, desiredH, linespc float64) (face font.Face, height float64) {
	textH := func() float64 {
		wrapped := dc.WordWrap(text, w)
		withBreaks := strings.Join(wrapped, "\n")
		_, textH := dc.MeasureMultilineString(withBreaks, linespc)
		return textH
	}
	height = textH()
	for height > desiredH {
		face = truetype.NewFace(fontf, &truetype.Options{Size: dc.FontHeight() * 0.75})
		dc.SetFontFace(face)
		height = textH()
	}
	return
}
