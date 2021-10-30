package memegen

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"testing"
	"time"

	"github.com/carbocation/go-quantize/quantize"
	"github.com/disintegration/imaging"
	"samhza.com/esammy/internal/gif"
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
		r := gif.NewReader(bytes.NewReader(p))
		w := gif.NewWriter(buf)
		cfg, err := r.ReadHeader()
		if err != nil {
			b.Fatal(err)
		}
		if err = w.WriteHeader(*cfg); err != nil {
			b.Fatal(err)
		}
		bounds := image.Rect(0, 0, cfg.Width, cfg.Height)
		textm := image.NewRGBA(bounds)
		Impact(textm, "HELLO", "bottom text...")

		const concurrency = 8
		type bruh struct {
			n  int
			pm *image.Paletted
			m  draw.Image
		}
		in, out := make(chan bruh, concurrency), make(chan bruh, concurrency)
		for i := 0; i < concurrency; i++ {
			go func(i int) {
				defer fmt.Println("worker exit", i)
				for b := range in {
					draw.Draw(b.m, bounds, b.pm, image.Point{}, draw.Src)
					draw.Draw(b.m, bounds, textm, image.Point{}, draw.Over)
					b.pm.Palette = quantize.MedianCutQuantizer{}.Quantize(make([]color.Color, 0, 256), b.m)
					draw.Draw(b.pm, bounds, b.m, image.Point{}, draw.Src)
					out <- b
				}
			}(i)
		}
		firstBatch := true
		scratch := make(chan bruh, concurrency)

		frames := make([]gif.ImageBlock, concurrency)
		readall := false
		for {
			if readall {
				break
			}
			var n int
			for n = 0; n < concurrency; n++ {
				block, err := r.ReadImage()
				if err != nil {
					b.Fatal(err)
				}
				if block == nil {
					readall = true
					close(in)
					break
				}
				frames[n] = *block
				var b bruh
				if firstBatch {
					b = bruh{n, image.NewPaletted(bounds, nil), image.NewRGBA(bounds)}
				} else {
					b = <-scratch
					b.n = n
				}
				copyPaletted(b.pm, block.Image)
				in <- b
			}
			firstBatch = false
			for j := 0; j < n; j++ {
				b := <-out
				frames[b.n].Image = b.pm
				scratch <- b
			}
			for j := 0; j < n; j++ {
				err := w.WriteFrame(frames[j])
				if err != nil {
					b.Fatal(err)
				}
			}
		}

		err = w.Close()
		if err != nil {
			b.Fatal(err)
		}
	}
	time.Sleep(2 * time.Second)
	os.WriteFile("out.gif", buf.Bytes(), 0666)
}

func copyPaletted(dst, src *image.Paletted) {
	copy(dst.Pix, src.Pix)
	dst.Stride = src.Stride
	dst.Rect = src.Rect
	dst.Palette = make(color.Palette, len(src.Palette))
	copy(dst.Palette, src.Palette)
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
