package discordbot

import (
	"bytes"
	"errors"
	"github.com/diamondburned/arikawa/v3/state"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"os"
	"samhza.com/esammy/bot/plugin"
	"strconv"

	ff "samhza.com/ffmpeg"
)

// TODO: split Uncaption into multiple functions

func (bot *Bot) cmdUncaption(s *state.State, ctx *plugin.Context) (any, error) {
	media := ctx.Options["media"].(Media)
	resp, err := http.Get(media.URL)
	if err != nil {
		return nil, err
	}
	done := bot.startWorking(ctx.Replier)
	defer done()
	defer resp.Body.Close()
	switch media.Type {
	case mediaImage:
		im, _, err := image.Decode(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		newMinY, found := detectCaption(im)
		if !found {
			return nil, errors.New("couldn't find the caption")
		}
		b := im.Bounds()
		b.Min.Y = newMinY
		cropped := cropImage(im, b)
		r, w := io.Pipe()
		defer r.Close()
		go func() {
			png.Encode(w, cropped)
			w.Close()
		}()
		done()
		return nil, bot.sendFile(s, ctx, "png", r)
	case mediaGIF:
		dgif, err := gif.DecodeAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		newMinY, found := detectCaption(dgif.Image[0])
		if !found {
			return nil, errors.New("couldn't find the caption")
		}
		bounds := dgif.Image[0].Rect
		bounds.Min.Y = newMinY
		for i, img := range dgif.Image {
			dgif.Image[i] = img.SubImage(bounds).(*image.Paletted)
			dgif.Image[i].Rect.Min.Y = 0
			dgif.Image[i].Rect.Max.Y -= newMinY
		}
		dgif.Config.Height -= newMinY
		r, w := io.Pipe()
		go func() {
			w.CloseWithError(gif.EncodeAll(w, dgif))
		}()
		done()
		return nil, bot.sendFile(s, ctx, "gif", r)
	}
	in, err := downloadInput(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	defer os.Remove(in.Name())
	defer in.Close()
	firstFrame, err := firstVidFrame(in.Name())
	if err != nil {
		return nil, err
	}
	newMinY, found := detectCaption(firstFrame)
	if !found {
		return nil, errors.New("couldn't find the caption")
	}
	var instream ff.Stream = ff.InputFile{File: in}

	probed, err := ff.Probe(in.Name())
	if err != nil {
		return nil, err
	}
	var hasAudio bool
	for _, stream := range probed.Streams {
		if stream.CodecType == ff.CodecTypeAudio {
			hasAudio = true
		}
	}
	var v ff.Stream = ff.Video(instream)

	b := firstFrame.Bounds()
	v = ff.Filter(v, "crop=y="+strconv.Itoa(newMinY)+
		":out_h="+strconv.Itoa(b.Max.Y-newMinY))
	var outfmt string
	streams := []ff.Stream{v}
	if media.Type == mediaGIFV {
		v = ff.Filter(v, "fps=20")
		one, two := ff.Split(v)
		palette := ff.PaletteGen(two)
		v = ff.PaletteUse(one, palette)
		outfmt = "gif"
	} else {
		outfmt = "mp4"
		if hasAudio {
			streams = append(streams, ff.Audio(instream))
		}
	}
	out, err := bot.createOutput(ctx.ID, outfmt)
	if err != nil {
		return nil, err
	}
	fcmd := new(ff.Cmd)
	fcmd.AddFileOutput(out.File, []string{"-y", "-f", outfmt}, streams...)
	err = fcmd.Cmd().Run()
	if err != nil {
		return nil, err
	}
	done()
	return nil, out.Send(ctx.Replier)
}

func firstVidFrame(filename string) (image.Image, error) {
	var v ff.Stream = ff.Input{Name: filename}
	v = ff.Filter(v, "select=eq(n\\,0)")
	cmd := new(ff.Cmd)
	cmd.AddOutput("-", []string{"-f", "mjpeg"}, v)
	out, err := cmd.Cmd().Output()
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	return img, err
}

func detectCaption(m image.Image) (newMinY int, found bool) {
	b := m.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		brightness := color.GrayModel.Convert(m.At(b.Min.X, y)).(color.Gray).Y
		if brightness < 248 {
			if y == b.Min.Y {
				return 0, false
			}
			return y, true
		}
	}
	return 0, false
}

func cropImage(im image.Image, b image.Rectangle) image.Image {
	sub, ok := im.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if ok {
		return sub.SubImage(b)
	}
	return croppedImage{im, b}
}

type croppedImage struct {
	image  image.Image
	bounds image.Rectangle
}

func (c croppedImage) ColorModel() color.Model { return c.image.ColorModel() }
func (c croppedImage) At(x, y int) color.Color { return c.image.At(x, y) }
func (c croppedImage) Bounds() image.Rectangle { return c.bounds }
