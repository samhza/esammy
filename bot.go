package esammy

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"git.sr.ht/~samhza/esammy/memegen"
	"git.sr.ht/~samhza/esammy/tenor"
	"git.sr.ht/~samhza/esammy/vedit"
	"git.sr.ht/~samhza/esammy/vedit/ffmpeg"
	"github.com/diamondburned/arikawa/v2/api"
	"github.com/diamondburned/arikawa/v2/bot"
	"github.com/diamondburned/arikawa/v2/discord"
	"github.com/diamondburned/arikawa/v2/gateway"
	"github.com/diamondburned/arikawa/v2/utils/sendpart"
	"github.com/disintegration/imaging"
	"github.com/pkg/errors"
)

type Bot struct {
	Ctx *bot.Context

	httpClient *http.Client
	tenor      *tenor.Client
}

type compositeFunc func(int, int) (image.Image, image.Point, bool)

func New(client *http.Client, tenorkey string) *Bot {
	b := Bot{Ctx: nil, httpClient: client, tenor: nil}
	if tenorkey != "" {
		b.tenor = tenor.NewClient(tenorkey)
		b.tenor.Client = client
	}
	return &b
}

func (b *Bot) Ping(m *gateway.MessageCreateEvent) error {
	msg, err := b.Ctx.SendMessage(m.ChannelID, "Pong!", nil)
	if err != nil {
		return err
	}
	ping := msg.Timestamp.Time().Sub(m.Timestamp.Time())
	response := fmt.Sprintf("Pong! (Response time: `%s`)", ping)
	_, err = b.Ctx.EditMessage(m.ChannelID, msg.ID, response, nil, false)
	return err
}

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
	return bot.composite(m.Message, func(w, h int) (image.Image, image.Point, bool) {
		return memegen.Impact(w, h, args.Top, args.Bottom),
			image.Point{}, false
	})
}

func (bot *Bot) Caption(m *gateway.MessageCreateEvent, raw bot.RawArguments) error {
	return bot.composite(m.Message, func(w, h int) (image.Image, image.Point, bool) {
		img, pt := memegen.Caption(w, h, string(raw))
		return img, pt, true
	})
}

func (bot *Bot) Motivate(m *gateway.MessageCreateEvent, args MemeArguments) error {
	return bot.composite(m.Message, func(w, h int) (image.Image, image.Point, bool) {
		img, pt := memegen.Motivate(w, h, args.Top, args.Bottom)
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
	var meme sendpart.File
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
		go func() {
			png.Encode(w, dest)
			w.Close()
		}()
		meme = sendpart.File{Name: "out.png", Reader: r}
	} else {
		tmp, err := ioutil.TempFile("", "esammy.*")
		if err != nil {
			return errors.Wrap(err, "error creating temporary file")
		}
		defer os.Remove(tmp.Name())
		_, err = io.Copy(tmp, b)
		if err != nil {
			return errors.Wrap(err, "error downloading input")
		}
		tmp.Close()
		b.Close()

		input := ffmpeg.Input{Name: tmp.Name()}
		v, a := ffmpeg.Video(input), ffmpeg.Audio(input)

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
			one, two := ffmpeg.Split(v)
			palette := ffmpeg.PaletteGen(two)
			v = ffmpeg.PaletteUse(one, palette)
		}
		outfd, err := os.CreateTemp("", "esammy.*")
		if err != nil {
			return err
		}
		outfd.Close()
		out := outfd.Name()
		defer os.Remove(out)
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
		outopts := []string{"-f", format, "-shortest"}
		fcmd.AddFileOutput(out, outopts, streams...)
		cmd := fcmd.Cmd()
		cmd.Args = append(cmd.Args, "-y", "-loglevel", "error", "-shortest")
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
		outfd, err = os.Open(out)
		if err != nil {
			return err
		}
		defer outfd.Close()
		meme = sendpart.File{Name: "out." + format, Reader: outfd}

	}
	_, err = bot.Ctx.SendMessageComplex(m.ChannelID, api.SendMessageData{Files: []sendpart.File{meme}})
	return err
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

type editArguments vedit.Arguments

func (e *editArguments) CustomParse(arg string) error {
	return (*vedit.Arguments)(e).Parse(arg)
}

func (bot *Bot) Edit(m *gateway.MessageCreateEvent, cmd editArguments) error {
	args := (vedit.Arguments)(cmd)
	media, err := bot.findMedia(m.Message)
	if err != nil {
		return err
	}
	var itype vedit.InputType
	switch media.Type {
	case mediaImage, mediaGIF:
		itype = vedit.InputImage
	case mediaVideo, mediaGIFV:
		itype = vedit.InputVideo
	}
	resp, err := bot.httpClient.Get(media.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	tmp, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return err
	}
	defer tmp.Close()
	_, err = io.Copy(tmp, resp.Body)
	if err != nil {
		return err
	}
	tmp.Close()
	resp.Body.Close()
	outname, err := vedit.Process(args, itype, tmp.Name())
	if err != nil {
		return err
	}
	defer os.Remove(outname)
	out, err := os.Open(outname)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = bot.Ctx.SendMessageComplex(m.ChannelID, api.SendMessageData{
		Files: []sendpart.File{{"out.mp4", out}},
	})
	return err
}
