// +build ignore

package main

import "github.com/fogleman/gg"

const S = 1024

func main() {
	dc := gg.NewContext(S, S)
	dc.SetRGB(1, 1, 1)
	dc.Clear()
	if err := dc.LoadFontFace("./impact.ttf", S/10); err != nil {
		panic(err)
	}
	top := "top text top text top text ayy"
	bot := "bottom text :)"
	drawOutlinedText(dc, top, S/2, dc.FontHeight()*0.3, 0.5, 0, S)
	drawOutlinedText(dc, bot, S/2, S-dc.FontHeight()*0.3, 0.5, 1, S)
	dc.SavePNG("out.png")
}

func drawOutlinedText(dc *gg.Context, s string, x, y, ax, ay, width float64) {
	dc.SetRGB(0, 0, 0)
	n := width / 500
	const sp float64 = 1.2 // line spacing
	w := width * .95
	dc.DrawStringWrapped(s, x+n, y+n, ax, ay, w, sp, gg.AlignCenter)
	dc.DrawStringWrapped(s, x-n, y-n, ax, ay, w, sp, gg.AlignCenter)
	dc.DrawStringWrapped(s, x+n, y-n, ax, ay, w, sp, gg.AlignCenter)
	dc.DrawStringWrapped(s, x-n, y+n, ax, ay, w, sp, gg.AlignCenter)
	dc.SetRGB(1, 1, 1)
	dc.DrawStringWrapped(s, x, y, ax, ay, w, sp, gg.AlignCenter)
}
