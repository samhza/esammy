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
	"runtime"
	"strings"

	"github.com/diamondburned/arikawa/api"
	"github.com/diamondburned/arikawa/bot"
	"github.com/diamondburned/arikawa/discord"
	"github.com/diamondburned/arikawa/gateway"
	"github.com/disintegration/imaging"
	"github.com/pkg/errors"
	"github.com/samhza/esammy/ff"
	"github.com/samhza/esammy/memegen"
)

type Bot struct {
	Ctx *bot.Context
	ff  ff.Throttler
}

type compositeFunc func(int, int) (image.Image, image.Point, bool)

func New() *Bot {
	ff := ff.NewThrottler(int64(runtime.GOMAXPROCS(-1) * 2))
	return &Bot{nil, ff}
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

func (bot *Bot) Meme(m *gateway.MessageCreateEvent, args MemeArguments) (*api.SendMessageData, error) {
	return bot.composite(m.Message, func(w, h int) (image.Image, image.Point, bool) {
		return memegen.Impact(w, h, args.Top, args.Bottom),
			image.Point{}, false
	})
}

func (bot *Bot) Caption(m *gateway.MessageCreateEvent, raw bot.RawArguments) (*api.SendMessageData, error) {
	return bot.composite(m.Message, func(w, h int) (image.Image, image.Point, bool) {
		img, pt := memegen.Caption(w, h, string(raw))
		return img, pt, true
	})
}

func (bot *Bot) Motivate(m *gateway.MessageCreateEvent, args MemeArguments) (*api.SendMessageData, error) {
	return bot.composite(m.Message, func(w, h int) (image.Image, image.Point, bool) {
		img, pt := memegen.Motivate(w, h, args.Top, args.Bottom)
		return img, pt, true
	})
}

func (bot *Bot) Speed(m *gateway.MessageCreateEvent, speed ...float64) (*api.SendMessageData, error) {
	outputformat := ""
	media, err := bot.findMedia(m.Message)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(media.URL)
	if err != nil {
		return nil, err
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "image/gif" {
		outputformat = "gif"
	} else if strings.HasPrefix(mime, "video") {
		if media.GIFV {
			outputformat = "gif"
		} else {
			outputformat = "mp4"
		}
	} else {
		resp.Body.Close()
		return nil, errors.New("unsupported file type")
	}
	b := resp.Body
	tmp, err := ioutil.TempFile("", "esammy.*")
	if err != nil {
		return nil, errors.Wrap(err, "error creating temporary file")
	}
	defer os.Remove(tmp.Name())
	io.Copy(tmp, b)
	b.Close()
	setpts := 0.5
	if len(speed) != 0 {
		setpts = 1.0 / speed[0]
	}
	gif, err := bot.ff.Speed(tmp.Name(), outputformat, setpts)
	if err != nil {
		return nil, err
	}
	r := bytes.NewReader(gif)
	meme := api.SendMessageFile{Name: "out." + outputformat, Reader: r}
	return &api.SendMessageData{
		Files: []api.SendMessageFile{meme},
	}, nil
}

func (bot *Bot) composite(m discord.Message, imgfn compositeFunc) (*api.SendMessageData, error) {
	img := false
	outputformat := ""
	media, err := bot.findMedia(m)
	if err != nil {
		return nil, err
	}
	resp, err := http.Get(media.URL)
	if err != nil {
		return nil, err
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "image/gif" {
		outputformat = "gif"
	} else if strings.HasPrefix(mime, "image") {
		img = true
	} else if strings.HasPrefix(mime, "video") {
		if media.GIFV {
			outputformat = "gif"
		} else {
			outputformat = "mp4"
		}
	} else {
		resp.Body.Close()
		return nil, errors.New("unsupported file type")
	}
	b := resp.Body
	var meme api.SendMessageFile
	if img {
		img, _, err := image.Decode(b)
		b.Close()
		if err != nil {
			return nil, err
		}
		buf := bytes.NewBuffer(nil)
		w, h := img.Bounds().Max.X, img.Bounds().Max.Y
		overlay, pt, under := imgfn(w, h)
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
		png.Encode(buf, dest)
		if err != nil {
			return nil, err
		}
		meme = api.SendMessageFile{Name: "out.png", Reader: buf}
	} else {
		tmp, err := ioutil.TempFile("", "esammy.*")
		if err != nil {
			return nil, errors.Wrap(err, "error creating temporary file")
		}
		defer os.Remove(tmp.Name())
		io.Copy(tmp, b)
		b.Close()
		size, _, err := bot.ff.Probe(tmp.Name())
		if err != nil {
			return nil, errors.Wrap(err, "error probing input")
		}
		img, pt, under := imgfn(size.Width, size.Height)
		gif, err := bot.ff.Composite(tmp.Name(), outputformat, img, under, pt)
		if err != nil {
			return nil, err
		}
		r := bytes.NewReader(gif)
		meme = api.SendMessageFile{Name: "out." + outputformat, Reader: r}
	}
	return &api.SendMessageData{
		Files: []api.SendMessageFile{meme},
	}, nil
}

func probeGIF(path string) (w, h int, err error) {
	size, _, err := ff.Probe(path)
	if err != nil {
		return 0, 0, errors.Wrap(err, "Failed to parse GIF")
	}
	return size.Width, size.Height, nil
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
