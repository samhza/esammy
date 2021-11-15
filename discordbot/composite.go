package discordbot

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/diamondburned/arikawa/v3/bot"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/disintegration/imaging"
	"github.com/pkg/errors"
	"go.samhza.com/esammy/memegen"
	"go.samhza.com/ffmpeg"
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
	if media.Type == mediaImage {
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
			png.Encode(w, dest)
			w.Close()
		}()
		return bot.sendFile(m.ChannelID, m.ID, "png", r)
	} else {
		in, err := downloadInput(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		defer os.Remove(in.Name())
		defer in.Close()

		input := ffmpeg.InputFile{File: in}
		var v ffmpeg.Stream
		a := ffmpeg.Audio(input)

		img, pt, under := imgfn(media.Width, media.Height)
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
		if media.Type == mediaGIFV || media.Type == mediaGIF {
			v = ffmpeg.Filter(v, "fps=20")
			one, two := ffmpeg.Split(v)
			palette := ffmpeg.PaletteGen(two)
			v = ffmpeg.PaletteUse(one, palette)
		}
		fcmd := &ffmpeg.Cmd{}
		var format string
		streams := []ffmpeg.Stream{v}
		switch media.Type {
		case mediaVideo:
			format = "mp4"
			streams = append(streams, a)
		case mediaGIFV, mediaGIF:
			format = "gif"
		}
		out, err := bot.createOutput(m.ID, format)
		if err != nil {
			return err
		}
		defer out.Cleanup()
		outopts := []string{"-f", format, "-shortest"}
		fcmd.AddFileOutput(out.File, outopts, streams...)
		cmd := fcmd.Cmd()
		cmd.Args = append(cmd.Args, "-y", "-loglevel", "error", "-shortest")
		stderr := &bytes.Buffer{}
		cmd.Stderr = stderr
		log.Println(cmd)
		err = cmd.Run()
		if err != nil {
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				return fmt.Errorf("exit status %d: %s",
					exitError.ExitCode(), string(stderr.String()))
			}
			return err
		}
		return out.Send(bot.Ctx.Client, m.ChannelID)
	}
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
