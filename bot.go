package esammy

import (
	"bytes"
	"image"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"io/ioutil"
	"net/http"
	"runtime"
	"strings"

	"github.com/diamondburned/arikawa/api"
	"github.com/diamondburned/arikawa/bot"
	"github.com/diamondburned/arikawa/discord"
	"github.com/diamondburned/arikawa/gateway"
	"github.com/disintegration/imaging"
	"github.com/erebid/esammy/ff"
	"github.com/erebid/esammy/memegen"
	"github.com/pkg/errors"
)

type Bot struct {
	Ctx *bot.Context
	ff  ff.Throttler
}

func New() *Bot {
	ff := ff.NewThrottler(int64(runtime.GOMAXPROCS(-1) * 2))
	return &Bot{nil, ff}
}

func (bot *Bot) Meme(m *gateway.MessageCreateEvent, top, bottom string) (*api.SendMessageData, error) {
	b, gif, err := bot.findImage(m.Message)
	if err != nil {
		return nil, err
	}
	body, err := ioutil.ReadAll(b)
	b.Close()
	r := bytes.NewReader(body)
	var meme api.SendMessageFile
	if gif {
		w, h, err := probeGIF(r)
		if err != nil {
			return nil, err
		}
		r.Seek(0, 0)
		text := memegen.Meme(w, h, top, bottom)
		gif, err := bot.ff.OverlayGIF(r, text)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(gif)
		meme = api.SendMessageFile{"meme.gif", r}
	} else {
		img, _, err := image.Decode(r)
		if err != nil {
			return nil, err
		}
		buf := bytes.NewBuffer(nil)
		w, h := img.Bounds().Max.X, img.Bounds().Max.Y
		text := memegen.Meme(w, h, top, bottom)
		var drawer, drawable = img.(draw.Image)
		var _, paletted = img.(*image.Paletted)
		if !drawable || paletted {
			drawer = imaging.Clone(img)
		}
		draw.Draw(drawer, text.Bounds(), text, image.Point{0, 0}, draw.Over)
		png.Encode(buf, drawer)
		if err != nil {
			return nil, err
		}
		meme = api.SendMessageFile{"meme.png", buf}
	}
	return &api.SendMessageData{
		Files: []api.SendMessageFile{meme},
	}, nil
}

func (b *Bot) findImage(m discord.Message) (body io.ReadCloser, gif bool, err error) {
	body, gif = getMsgImage(m)
	if body != nil {
		return
	}
	msgs, err := b.Ctx.Messages(m.ChannelID)
	if err != nil {
		return
	}
	for _, m := range msgs {
		body, gif = getMsgImage(m)
		if body != nil {
			return
		}
	}
	return
}

func getMsgImage(m discord.Message) (body io.ReadCloser, gif bool) {
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
			return
		}
		switch mime := resp.Header.Get("Content-Type"); {
		case mime == "image/gif":
			gif = true
		case strings.HasPrefix(mime, "image"):
		default:
			resp.Body.Close()
			return
		}
		body = resp.Body
		return
	}
	return nil, false
}

func probeGIF(r io.Reader) (w, h int, err error) {
	c, err := gif.DecodeConfig(r)
	if err != nil {
		return 0, 0, errors.Wrap(err, "Failed to parse GIF")
	}
	return c.Width, c.Height, nil
}
