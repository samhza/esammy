package discordbot

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"os"
	"os/exec"

	"github.com/diamondburned/arikawa/v3/bot"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/disintegration/imaging"
	"github.com/ericpauley/go-quantize/quantize"
	"github.com/pkg/errors"
	"samhza.com/esammy/internal/gif"
	"samhza.com/esammy/memegen"
	"samhza.com/ffmpeg"
)

type compositeFunc func(int, int) (image.Image, image.Point, bool)

func (bot *Bot) Meme(m *gateway.MessageCreateEvent, args MemeArguments) error {
	return bot.composite(m.Message, func(w, h int) (image.Image, image.Point, bool) {
		m := image.NewRGBA(image.Rect(0, 0, w, h))
		memegen.Impact(m, args.Top, args.Bottom)
		return m, image.Point{}, false
	})
}

func (bot *Bot) Motivate(m *gateway.MessageCreateEvent, args MemeArguments) error {
	return bot.composite(m.Message, func(w, h int) (image.Image, image.Point, bool) {
		img, pt := memegen.Motivate(w, h, args.Top, args.Bottom)
		return img, pt, true
	})
}

func (bot *Bot) Caption(m *gateway.MessageCreateEvent, raw bot.RawArguments) error {
	return bot.composite(m.Message, func(w, h int) (image.Image, image.Point, bool) {
		img, pt := memegen.Caption(w, h, string(raw))
		return img, pt, true
	})
}

func (bot *Bot) composite(m discord.Message, imgfn compositeFunc) error {
	media, err := bot.findMedia(m)
	if err != nil {
		return err
	}
	resp, err := bot.httpClient.Get(media.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b := resp.Body
	switch media.Type {
	case mediaImage:
		img, _, err := image.Decode(b)
		b.Close()
		if err != nil {
			return err
		}
		width, height := img.Bounds().Max.X, img.Bounds().Max.Y
		overlay, pt, under := imgfn(width, height)
		var src image.Image
		var dest draw.Image
		if under {
			src = img
			dest = makeDrawable(overlay)
		} else {
			src = overlay
			dest = makeDrawable(img)
		}
		draw.Draw(dest, dest.Bounds(), src, pt, draw.Over)
		r, w := io.Pipe()
		defer r.Close()
		go func() {
			err := png.Encode(w, dest)
			w.CloseWithError(err)
		}()
		return bot.sendFile(m.ChannelID, m.ID, "png", r)
	case mediaGIF:
		r, w := io.Pipe()
		defer r.Close()
		go func() {
			err := compositeGIF(b, w, imgfn)
			w.CloseWithError(err)
		}()
		return bot.sendFile(m.ChannelID, m.ID, "gif", r)
	default:
		in, err := downloadInput(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		defer os.Remove(in.Name())
		defer in.Close()
		out, err := bot.createOutput(m.ID, "mp4")
		if err != nil {
			return err
		}
		defer out.Cleanup()
		var fmt string
		if media.Type == mediaGIFV {
			fmt = "gif"
		} else {
			fmt = "mp4"
		}
		err = bot.compositeVideo(in, out.File, imgfn, media.Width, media.Height, fmt)
		if err != nil {
			return err
		}
		return out.Send(bot.Ctx.Client, m.ChannelID)
	}
}

func (bot *Bot) compositeVideo(in, out *os.File,
	fn compositeFunc, w, h int, outfmt string) error {
	input := ffmpeg.InputFile{File: in}
	var v ffmpeg.Stream
	a := ffmpeg.Audio(input)

	img, pt, under := fn(w, h)
	pR, pW, err := os.Pipe()
	if err != nil {
		return err
	}
	defer pW.Close()
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	go func() {
		enc.Encode(pW, img)
		pW.Close()
	}()
	imginput := ffmpeg.InputFile{File: pR}
	if under {
		v = ffmpeg.Overlay(imginput, input, -pt.X, -pt.Y)
	} else {
		v = ffmpeg.Overlay(input, imginput, -pt.X, -pt.Y)
	}
	if outfmt == "gif" {
		v = ffmpeg.Filter(v, "fps=20")
		one, two := ffmpeg.Split(v)
		palette := ffmpeg.PaletteGen(two)
		v = ffmpeg.PaletteUse(one, palette)
	}
	fcmd := &ffmpeg.Cmd{}
	streams := []ffmpeg.Stream{v}
	if outfmt == "mp4" {
		streams = append(streams, a)
	}
	outopts := []string{"-f", outfmt, "-shortest"}
	fcmd.AddFileOutput(out, outopts, streams...)
	cmd := fcmd.Cmd()
	cmd.Args = append(cmd.Args, "-y", "-loglevel", "error", "-shortest")
	if outfmt == "gif" {
		cmd.Args = append(cmd.Args, "-gifflags", "-offsetting")
	}
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return fmt.Errorf("exit status %d: %s",
				exitError.ExitCode(), string(stderr.String()))
		}
		return err
	}
	return nil
}

func compositeGIF(rdr io.Reader, wtr io.Writer, fn compositeFunc) error {
	r := gif.NewReader(rdr)
	w := gif.NewWriter(wtr)
	cfg, err := r.ReadHeader()
	if err != nil {
		return err
	}
	inbounds := image.Rect(0, 0, cfg.Width, cfg.Height)
	img, pt, under := fn(cfg.Width, cfg.Height)
	outbounds := img.Bounds()
	cfg.Height = outbounds.Max.Y
	if err = w.WriteHeader(*cfg); err != nil {
		return err
	}

	const concurrency = 8
	type bruh struct {
		n       int
		in, out *image.Paletted
		m       draw.Image
	}
	in, out := make(chan bruh, concurrency), make(chan bruh, concurrency)
	defer close(in)
	for i := 0; i < concurrency; i++ {
		go func(i int) {
			defer fmt.Println("worker exit", i)
			for b := range in {
				if under {
					fmt.Println("KEK")
					draw.Draw(b.m, outbounds, img, image.Point{}, draw.Src)
					draw.Draw(b.m, outbounds, b.in, pt, draw.Over)
				} else {
					draw.Draw(b.m, outbounds, b.in, image.Point{}, draw.Src)
					draw.Draw(b.m, outbounds, img, pt, draw.Over)
				}
				b.out.Rect = outbounds
				b.out.Palette = append(quantize.MedianCutQuantizer{}.Quantize(make([]color.Color, 0, 255), b.m), color.RGBA{})
				draw.Draw(b.out, outbounds, b.m, image.Point{}, draw.Src)
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
				return err
			}
			if block == nil {
				readall = true
				break
			}
			frames[n] = *block
			var b bruh
			if firstBatch {
				in := image.NewPaletted(inbounds, nil)
				b = bruh{n, in, in, image.NewRGBA(outbounds)}
				if inbounds != outbounds {
					b.out = image.NewPaletted(outbounds, nil)
				}
			} else {
				b = <-scratch
				b.n = n
			}
			copyPaletted(b.in, block.Image)
			in <- b
		}
		firstBatch = false
		for j := 0; j < n; j++ {
			b := <-out
			frames[b.n].Image = b.out
			scratch <- b
		}
		for j := 0; j < n; j++ {
			err := w.WriteFrame(frames[j])
			if err != nil {
				return err
			}
		}
	}
	return w.Close()
}

func copyPaletted(dst, src *image.Paletted) {
	copy(dst.Pix, src.Pix)
	dst.Stride = src.Stride
	dst.Rect = src.Rect
	dst.Palette = make(color.Palette, len(src.Palette))
	copy(dst.Palette, src.Palette)
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
