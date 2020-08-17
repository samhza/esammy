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
	"time"

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
		return memegen.Meme(w, h, args.Top, args.Bottom),
			image.Point{}, false
	})
}

func (bot *Bot) Caption(m *gateway.MessageCreateEvent, raw bot.RawArguments) (*api.SendMessageData, error) {
	return bot.composite(m.Message, func(w, h int) (image.Image, image.Point, bool) {
		img, pt := memegen.Caption(w, h, string(raw))
		return img, pt, true
	})
}

func (bot *Bot) composite(m discord.Message, imgfn compositeFunc) (*api.SendMessageData, error) {
	now := time.Now()
	img := false
	outputformat := ""
	b, err := bot.findMedia(m,
		func(mime string) bool {
			if mime == "image/gif" {
				outputformat = "gif"
				return true
			}
			if mime == "video/mp4" {
				outputformat = "mp4"
				return true
			}
			if strings.HasPrefix(mime, "image") {
				img = true
				return true
			}
			return false
		},
	)
	if err != nil {
		return nil, err
	}
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
		opts := ff.ProcessOptions{}
		opts.Image, opts.Point, opts.Under = imgfn(size.Width, size.Height)
		gif, err := bot.ff.Process(tmp.Name(), outputformat, opts)
		if err != nil {
			return nil, err
		}
		r := bytes.NewReader(gif)
		meme = api.SendMessageFile{Name: "out." + outputformat, Reader: r}
	}
	fmt.Println(time.Since(now).Milliseconds())
	return &api.SendMessageData{
		Files: []api.SendMessageFile{meme},
	}, nil
}

func (b *Bot) findMedia(m discord.Message, checkType func(string) bool) (body io.ReadCloser, err error) {
	body = getMsgMedia(m, checkType)
	if body != nil {
		return
	}
	msgs, err := b.Ctx.Messages(m.ChannelID)
	if err != nil {
		return
	}
	for _, m := range msgs {
		body = getMsgMedia(m, checkType)
		if body != nil {
			return
		}
	}
	return
}

func getMsgMedia(m discord.Message, checkType func(string) bool) (body io.ReadCloser) {
	var urls []string
	for _, at := range m.Attachments {
		urls = append(urls, at.Proxy)
	}
	for _, em := range m.Embeds {
		if em.Image != nil {
			urls = append(urls, em.Image.Proxy)
		}
		if em.Thumbnail != nil {
			urls = append(urls, em.Thumbnail.Proxy)
		}
	}
	for _, url := range urls {
		resp, err := http.Get(url)
		if err != nil {
			continue
		}
		if !checkType(resp.Header.Get("Content-Type")) {
			resp.Body.Close()
			continue
		}
		body = resp.Body
		return
	}
	return nil
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
