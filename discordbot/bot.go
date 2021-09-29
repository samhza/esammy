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
	"path"
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

	cfg Config

	httpClient *http.Client
	tenor      *tenor.Client
}

type Config struct {
	Tenor     string `toml:"tenor"`
	OutputDir string `toml:"output-dir"`
	OutputURL string `toml:"output-url"`
}

type compositeFunc func(int, int) (image.Image, image.Point, bool)

func New(client *http.Client, cfg Config) *Bot {
	b := Bot{Ctx: nil, httpClient: client, cfg: cfg}
	if cfg.Tenor != "" {
		b.tenor = tenor.NewClient(cfg.Tenor)
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
	_, err = b.Ctx.EditMessage(m.ChannelID, msg.ID, response)
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
	in, err := downloadInput(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	defer os.Remove(in.Name())
	defer in.Close()
	var v ff.Stream = ff.Video(ff.InputFile{File: in})
	v = ff.Filter(v, "fps=20")
	one, two := ff.Split(v)
	palette := ff.PaletteGen(two)
	v = ff.PaletteUse(one, palette)
	fcmd := new(ff.Cmd)
	out, err := bot.createOutput(m.ID, "gif")
	if err != nil {
		return err
	}
	defer out.Cleanup()
	fcmd.AddFileOutput(out.File, []string{"-y", "-f", "gif"}, v)
	err = fcmd.Cmd().Run()
	if err != nil {
		return err
	}
	return out.Send(bot.Ctx.Client, m.ChannelID)
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

		input := ff.InputFile{File: in}
		var v ff.Stream
		a := ff.Audio(input)

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
	err = vedit.Process(args, itype, in, out.File)
	if err != nil {
		return err
	}
	return out.Send(bot.Ctx.Client, m.ChannelID)
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
		return bot.sendFile(m.ChannelID, m.ID, "png", r)
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
	var instream ff.Stream = ff.Input{Name: inputf}
	v, a := ff.Video(instream), ff.Audio(instream)
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
	out, err := bot.createOutput(m.ID, outformat)
	if err != nil {
		return err
	}
	defer out.Cleanup()
	fcmd := new(ff.Cmd)
	fcmd.AddFileOutput(out.File, []string{"-y", "-f", outformat}, v, a)
	err = fcmd.Cmd().Run()
	if err != nil {
		return err
	}
	return out.Send(bot.Ctx.Client, m.ChannelID)
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

func downloadInput(body io.Reader) (*os.File, error) {
	in, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			in.Close()
			os.Remove(in.Name())
		}
	}()
	if _, err = io.Copy(in, body); err != nil {
		return nil, err
	}
	if _, err = in.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return in, nil
}

func (b *Bot) createOutput(id discord.MessageID, ext string) (*outputFile, error) {
	return createOutput(id, ext, b.cfg.OutputDir, b.cfg.OutputURL)
}

type outputFile struct {
	File    *os.File
	moveto  string
	moved   bool
	keep    bool
	baseurl string
}

func createOutput(id discord.MessageID, ext,
	dir, baseurl string) (*outputFile, error) {
	f, err := os.Create(id.String() + "." + ext)
	if err != nil {
		return nil, err
	}
	of := new(outputFile)
	of.baseurl = baseurl
	of.File = f
	if dir != "" {
		of.moveto = path.Join(dir, id.String()+"."+ext)
	}
	return of, err
}

func (s *outputFile) Send(ctx *api.Client, id discord.ChannelID) error {
	f := s.File
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	_, name := path.Split(f.Name())
	if stat.Size() <= 8000000 {
		_, err = ctx.SendMessageComplex(id, api.SendMessageData{
			Files: []sendpart.File{{Name: name, Reader: f}},
		})
		return err
	}
	if s.moveto == "" || s.baseurl == "" {
		return errors.New("file too large")
	}
	f.Close()
	err = os.Rename(f.Name(), s.moveto)
	if err != nil {
		return err
	}
	err = os.Chmod(s.moveto, 0644)
	if err != nil {
		return err
	}
	s.moved = true
	_, name = path.Split(s.moveto)
	_, err = ctx.SendMessage(id, s.baseurl+name)
	if err == nil {
		s.keep = true
	}
	return err
}

func (s *outputFile) Cleanup() {
	s.File.Close()
	if s.keep {
		return
	}
	if s.moved {
		os.Remove(s.moveto)
	} else {
		os.Remove(s.File.Name())
	}
}

func (b *Bot) sendFile(ch discord.ChannelID, mid discord.MessageID,
	ext string, src io.Reader) error {
	buf := new(bytes.Buffer) // TODO sync.Pool of buffers?
	lr := &io.LimitedReader{R: src, N: 8000000}
	_, err := buf.ReadFrom(lr)
	if err != nil {
		return err
	}
	if lr.N > 0 {
		_, err := b.Ctx.SendMessageComplex(ch, api.SendMessageData{
			Files: []sendpart.File{{Name: mid.String() + "." + ext, Reader: buf}},
		})
		return err
	}
	out, err := b.createOutput(mid, ext)
	if err != nil {
		return err
	}
	defer out.Cleanup()
	_, err = buf.WriteTo(out.File)
	if err != nil {
		return err
	}
	_, err = io.Copy(out.File, src)
	if err != nil {
		return err
	}
	return out.Send(b.Ctx.Client, ch)
}

type croppedImage struct {
	image  image.Image
	bounds image.Rectangle
}

func (c croppedImage) ColorModel() color.Model { return c.image.ColorModel() }
func (c croppedImage) At(x, y int) color.Color { return c.image.At(x, y) }
func (c croppedImage) Bounds() image.Rectangle { return c.bounds }
