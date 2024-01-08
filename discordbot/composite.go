package discordbot

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/diamondburned/arikawa/v3/bot"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/disintegration/imaging"
	"github.com/pkg/errors"
	"samhza.com/esammy/memegen"
	ff "samhza.com/ffmpeg"
)

type MemeArguments struct {
	Top,
	Bottom string
}

func (m *MemeArguments) CustomParse(args string) error {
	if args == "" {
		return errors.New("you need some text for me to generate the image")
	}
	split := strings.SplitN(args, ",", 2)
	m.Top = strings.TrimSpace(split[0])
	if len(split) == 2 {
		m.Bottom = strings.TrimSpace(split[1])
	}
	return nil
}

func (bot *Bot) Meme(m *gateway.MessageCreateEvent, args MemeArguments) error {
	return bot.composite(m.Message, "meme", func(w, h int) (image.Image, image.Point, bool) {
		m := image.NewRGBA(image.Rect(0, 0, w, h))
		memegen.Impact(m, args.Top, args.Bottom)
		return m, image.Point{}, false
	})
}

func (bot *Bot) Motivate(m *gateway.MessageCreateEvent, args MemeArguments) error {
	return bot.composite(m.Message, "motivate", func(w, h int) (image.Image, image.Point, bool) {
		img, pt := memegen.Motivate(w, h, args.Top, args.Bottom)
		return img, pt, true
	})
}

func (bot *Bot) Caption(m *gateway.MessageCreateEvent, raw bot.RawArguments) error {
	return bot.composite(m.Message, "caption", func(w, h int) (image.Image, image.Point, bool) {
		img, pt := memegen.Caption(w, h, string(raw))
		return img, pt, true
	})
}

type compositeFunc func(int, int) (image.Image, image.Point, bool)

func (bot *Bot) composite(m discord.Message, name string, imgfn compositeFunc) error {
	media, err := bot.findMedia(m)
	if err != nil {
		return err
	}
	resp, err := bot.httpClient.Get(media.URL)
	if err != nil {
		return err
	}
	done := bot.startWorking(m.ChannelID, m.ID)
	defer done()
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
		done()
		return bot.sendFile(m.ChannelID, m.ID, name, ".png", r)
	} else {
		in, err := downloadInput(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		defer os.Remove(in.Name())
		defer in.Close()

		input := ff.InputFile{File: in}
		var v ff.Stream

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
		imginput := ff.InputFile{File: pR}
		if under {
			v = ff.Overlay(imginput, input, -pt.X, -pt.Y)
		} else {
			v = ff.Overlay(input, imginput, -pt.X, -pt.Y)
		}
		if media.Type == mediaGIFV {
			v = ff.Filter(v, "fps=20")
		}
		if media.Type == mediaGIFV || media.Type == mediaGIF {
			v = ff.Filter(v, "fps=20")
			one, two := ff.Split(v)
			palette := ff.PaletteGen(two)
			v = ff.PaletteUse(one, palette)
		}
		fcmd := &ff.Cmd{}
		var format string
		streams := []ff.Stream{v}
		switch media.Type {
		case mediaVideo:
			format = "mp4"
			probed, err := ff.ProbeReader(in)
			if err != nil {
				return err
			}
			if _, err = in.Seek(0, 0); err != nil {
				return err
			}
			var hasAudio bool
			for _, stream := range probed.Streams {
				if stream.CodecType == ff.CodecTypeAudio {
					hasAudio = true
					break
				}
			}
			if hasAudio {
				streams = append(streams, ff.Audio(input))
			}
		case mediaGIFV, mediaGIF:
			format = "gif"
		}
		out, err := bot.createOutput(m.ID, name, "."+format)
		if err != nil {
			return err
		}
		outopts := []string{"-f", format, "-shortest"}
		fcmd.AddFileOutput(out.File, outopts, streams...)
		cmd := fcmd.Cmd()
		cmd.Args = append(cmd.Args, "-y", "-loglevel", "error", "-shortest")
		if media.Type == mediaGIF {
			cmd.Args = append(cmd.Args, "-vsync", "0")
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
		done()
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
