package memegen

import (
	_ "embed"
	"image"
	"image/draw"
	"strings"
	"unicode"

	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"samhza.com/gg"
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

func Impact(m draw.Image, top, bot string) {
	b := m.Bounds()
	w, h := b.Max.X-b.Min.X, b.Max.Y-b.Min.Y
	face := truetype.NewFace(impactFont, &truetype.Options{Size: float64(h / 8)})
	dr := &font.Drawer{
		Face: face,
		Dst:  m,
	}
	var text []string
	var y int
	fn := func(xoff, yoff int) {
		drawStringsCentered(dr, w, xoff, y+yoff, 0, text)
	}
	n := h / 160
	draw := func() {
		dr.Src = image.Black
		fn(-n, -n)
		fn(-n, +n)
		fn(n, -n)
		fn(n, n)
		dr.Src = image.White
		fn(0, 0)
	}

	text = wrap(dr, w, top)
	y = h / 32
	draw()

	text = wrap(dr, w, bot)
	_, texth := measure(dr, text)
	y = h - texth - h/32
	draw()
}

func measure(dr *font.Drawer, lines []string) (w, h int) {
	var fixw fixed.Int26_6
	faceh := dr.Face.Metrics().Height.Ceil()
	for _, line := range lines {
		fixw += dr.MeasureString(line)
		h += faceh
	}
	w = fixw.Ceil()
	return
}

func drawStringsCentered(dr *font.Drawer, w, x, y, pad int, lines []string) {
	faceh := dr.Face.Metrics().Height.Ceil()
	for _, str := range lines {
		drawStringCentered(dr, w, x, y, str)
		y += faceh + pad
	}
}

func drawStringCentered(dr *font.Drawer, w, x, y int, str string) {
	dr.Dot = fixed.Point26_6{(fixed.I(w)-dr.MeasureString(str))/2 + fixed.I(x), dr.Face.Metrics().Height + fixed.I(y)}
	dr.DrawString(str)
}

func wrap(dr *font.Drawer, width int, str string) []string {
	var result []string
	for _, line := range strings.Split(str, "\n") {
		fields := splitOnSpace(line)
		if len(fields)%2 == 1 {
			fields = append(fields, "")
		}
		x := ""
		for i := 0; i < len(fields); i += 2 {
			w := dr.MeasureString(x + fields[i])
			if w.Ceil() > width {
				if x == "" {
					result = append(result, fields[i])
					x = ""
					continue
				} else {
					result = append(result, x)
					x = ""
				}
			}
			x += fields[i] + fields[i+1]
		}
		if x != "" {
			result = append(result, x)
		}
	}
	for i, line := range result {
		result[i] = strings.TrimSpace(line)
	}
	return result
}

func splitOnSpace(x string) []string {
	var result []string
	pi := 0
	ps := false
	for i, c := range x {
		s := unicode.IsSpace(c)
		if s != ps && i > 0 {
			result = append(result, x[pi:i])
			pi = i
		}
		ps = s
	}
	result = append(result, x[pi:])
	return result
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
