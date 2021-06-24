package discordbot

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/diamondburned/arikawa/v3/api"
	"github.com/diamondburned/arikawa/v3/bot"
	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/diamondburned/arikawa/v3/utils/sendpart"
	"github.com/disintegration/imaging"
	"github.com/pkg/errors"
	"go.samhza.com/esammy/memegen"
	"go.samhza.com/esammy/tenor"
	"go.samhza.com/esammy/vedit"
	ff "go.samhza.com/ffmpeg"
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
	msg, err := b.Ctx.SendMessage(m.ChannelID, "Pong!")
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

func (bot *Bot) Gif(m *gateway.MessageCreateEvent) error {
	media, err := bot.findMedia(m.Message)
	if err != nil {
		return err
	}
	if media.Type != mediaVideo {
		return errors.New("this isn't a video")
	}
	resp, err := http.Get(media.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	in, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return err
	}
	defer os.Remove(in.Name())
	_, err = io.Copy(in, resp.Body)
	in.Close()
	resp.Body.Close()
	if err != nil {
		return err
	}
	if _, err = in.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var v ff.Stream = ff.Video(ff.InputFile{File: in})
	v = ff.Filter(v, "fps=20")
	one, two := ff.Split(v)
	palette := ff.PaletteGen(two)
	v = ff.PaletteUse(one, palette)
	fcmd := new(ff.Cmd)
	out, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	out.Close()
	fcmd.AddFileOutput(out, []string{"-y", "-f", "gif"}, v)
	err = fcmd.Cmd().Run()
	if err != nil {
		return err
	}
	out, err = os.Open(out.Name())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = bot.Ctx.SendMessageComplex(m.ChannelID, api.SendMessageData{
		Files: []sendpart.File{{Name: "out.gif", Reader: out}},
	})
	return err
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
		defer r.Close()
		go func() {
			png.Encode(w, dest)
			w.Close()
		}()
		meme = sendpart.File{Name: "out.png", Reader: r}
	} else {
		tmp, err := os.CreateTemp("", "esammy.*")
		if err != nil {
			return errors.Wrap(err, "error creating temporary file")
		}
		defer os.Remove(tmp.Name())
		defer tmp.Close()
		_, err = io.Copy(tmp, b)
		if err != nil {
			return errors.Wrap(err, "error downloading input")
		}
		b.Close()
		if _, err = tmp.Seek(0, io.SeekStart); err != nil {
			return err
		}

		input := ff.InputFile{File: tmp}
		v, a := ff.Video(input), ff.Audio(input)

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
		if media.Type == mediaGIFV || media.Type == mediaGIF {
			v = ff.Filter(v, "fps=20")
			one, two := ff.Split(v)
			palette := ff.PaletteGen(two)
			v = ff.PaletteUse(one, palette)
		}
		out, err := os.CreateTemp("", "esammy.*")
		if err != nil {
			return err
		}
		defer os.Remove(out.Name())
		fcmd := &ff.Cmd{}
		var format string
		streams := []ff.Stream{v}
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
		defer out.Close()
		meme = sendpart.File{Name: "out." + format, Reader: out}

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
	case mediaImage, mediaGIF, mediaGIFV:
		itype = vedit.InputImage
	case mediaVideo:
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
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	_, err = io.Copy(tmp, resp.Body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if _, err = tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	out, err := vedit.Process(args, itype, tmp)
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	defer out.Close()
	_, err = bot.Ctx.SendMessageComplex(m.ChannelID, api.SendMessageData{
		Files: []sendpart.File{{Name: "out.mp4", Reader: out}},
	})
	return err
}

func (bot *Bot) Uncaption(m *gateway.MessageCreateEvent) error {
	media, err := bot.findMedia(m.Message)
	if err != nil {
		return err
	}
	resp, err := http.Get(media.URL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var newMinY int
	if media.Type == mediaImage {
		im, _, err := image.Decode(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		var found bool
		newMinY, found = detectCaption(im)
		if !found {
			return errors.New("couldn't find the caption")
		}
		b := im.Bounds()
		b.Min.Y = newMinY
		cropped := croppedImage{im, b}
		r, w := io.Pipe()
		defer r.Close()
		go func() {
			png.Encode(w, cropped)
			w.Close()
		}()
		_, err = bot.Ctx.SendMessageComplex(m.ChannelID, api.SendMessageData{
			Files: []sendpart.File{{Name: "out.png", Reader: r}},
		})
		return err
	}
	in, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return err
	}
	inputf := in.Name()
	defer os.Remove(inputf)
	defer in.Close()
	_, err = io.Copy(in, resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil
	}
	var firstFrame image.Image
	if media.Type == mediaGIF {
		_, err = in.Seek(0, io.SeekStart)
		if err != nil {
			return err
		}
		firstFrame, _, err = image.Decode(in)
		if err != nil {
			return err
		}
		in.Close()
	} else {
		in.Close()
		firstFrame, err = firstVidFrame(inputf)
		if err != nil {
			return err
		}
	}
	newMinY, found := detectCaption(firstFrame)
	if !found {
		return errors.New("couldn't find the caption")
	}
	var v ff.Stream = ff.Input{Name: inputf}
	b := firstFrame.Bounds()
	v = ff.Filter(v, "crop=y="+strconv.Itoa(newMinY)+
		":out_h="+strconv.Itoa(b.Max.Y-newMinY))
	var outformat string
	switch media.Type {
	case mediaGIFV, mediaGIF:
		v = ff.Filter(v, "fps=20")
		one, two := ff.Split(v)
		palette := ff.PaletteGen(two)
		v = ff.PaletteUse(one, palette)
		outformat = "gif"
	default:
		outformat = "mp4"
	}
	out, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	out.Close()
	fcmd := new(ff.Cmd)
	fcmd.AddFileOutput(out, []string{"-y", "-f", outformat}, v)
	err = fcmd.Cmd().Run()
	if err != nil {
		return err
	}
	out, err = os.Open(out.Name())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = bot.Ctx.SendMessageComplex(m.ChannelID, api.SendMessageData{
		Files: []sendpart.File{{Name: "out." + outformat, Reader: out}},
	})
	return err
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
		if brightness < 250 {
			if y == b.Min.Y {
				return 0, false
			}
			return y, true
		}
	}
	return 0, false
}

type croppedImage struct {
	image  image.Image
	bounds image.Rectangle
}

func (c croppedImage) ColorModel() color.Model { return c.image.ColorModel() }
func (c croppedImage) At(x, y int) color.Color { return c.image.At(x, y) }
func (c croppedImage) Bounds() image.Rectangle { return c.bounds }
