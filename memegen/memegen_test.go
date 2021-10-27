package memegen

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/carbocation/go-quantize/quantize"
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

func BenchmarkMemeGIF(b *testing.B) {
	p, err := os.ReadFile("in.gif")
	if err != nil {
		b.Fatal(err)
	}
	buf := new(bytes.Buffer)
	for n := 0; n < b.N; n++ {
		g, err := gif.DecodeAll(bytes.NewReader(p))
		if err != nil {
			b.Fatal(p)
		}
		textm := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
		Impact(textm, "HELLO", "bottom text...")
		bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
		var wg sync.WaitGroup
		wg.Add(len(g.Image))
		type bruh struct {
			k int
			i *image.Paletted
		}
		out := make(chan bruh)
		for i, frame := range g.Image {
			go func(i int, frame image.Image) {
				m := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
				draw.Draw(m, bounds, frame, image.Point{}, draw.Src)
				draw.Draw(m, bounds, textm, image.Point{}, draw.Over)
				p := image.NewPaletted(bounds, quantize.MedianCutQuantizer{}.Quantize(make([]color.Color, 0, 256), m))
				draw.Draw(p, bounds, m, image.Point{}, draw.Src)
				out <- bruh{i, p}
				wg.Done()
			}(i, frame)
		}
		frames := make([]*image.Paletted, len(g.Image))
		sorted := make(chan struct{})
		go func() {
			for b := range out {
				frames[b.k] = b.i
			}
			sorted <- struct{}{}
		}()
		wg.Wait()
		close(out)
		<-sorted
		g.Image = frames
		err = gif.EncodeAll(buf, g)
		if err != nil {
			b.Fatal(p)
		}
	}
	os.WriteFile("out.gif", buf.Bytes(), 0666)
}

/*

}
for n := 0; n < b.N; n++ {
	g, err := gif.DecodeAll(bytes.NewReader(p))
	if err != nil {
		b.Fatal(p)
	}
	textm := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
	Impact(textm, "HELLO", "bottom text...")
	m := image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
	bounds := image.Rect(0, 0, g.Config.Width, g.Config.Height)
	for i, frame := range g.Image {
		draw.Draw(m, bounds, frame, image.Point{}, draw.Src)
		draw.Draw(m, bounds, textm, image.Point{}, draw.Over)
		g.Image[i] = image.NewPaletted(bounds, quantize.MedianCutQuantizer{}.QuantizeMultiple(make([]color.Color, 0, 256), m))
		draw.Draw(g.Image[i], bounds, m, image.Point{}, draw.Src)
	}
	err = gif.EncodeAll(io.Discard, g)
	if err != nil {
		b.Fatal(p)
	}
}
*/

func bail(err error) {
	if err != nil {
		panic(err)
	}
}
