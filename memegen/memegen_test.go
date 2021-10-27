package memegen

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"testing"

	"github.com/disintegration/imaging"
)

func BenchmarkMeme(b *testing.B) {
	for n := 0; n < b.N; n++ {
		Impact(image.NewRGBA(image.Rect(0, 0, 1000, 1000)),
			"top text here, with wrapping if needed",
			"bottom text too, literal\nnew\nlines are respected")
	}
}

func BenchmarkMemeParallel(b *testing.B) {
	imgdata, err := os.ReadFile("in.png")
	if err != nil {
		b.Fatal(err)
	}
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			m, err := png.Decode(bytes.NewReader(imgdata))
			if err != nil {
				b.Fatal(p)
			}
			Impact(makeDrawable(m),
				"top text here, with wrapping if needed",
				"bottom text too, literal\nnew\nlines are respected")
			png.Encode(io.Discard, m)
		}
	})
}

func makeDrawable(img image.Image) draw.Image {
	var drawer, drawable = img.(draw.Image)
	var _, paletted = img.(*image.Paletted)
	if !drawable || paletted {
		return imaging.Clone(img)
	} else {
		return drawer
	}
}
