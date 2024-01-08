package discordbot

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/diamondburned/arikawa/v3/gateway"
	ff "samhza.com/ffmpeg"
)

func (bot *Bot) Uncaption(m *gateway.MessageCreateEvent) error {
	media, err := bot.findMedia(m.Message)
	if err != nil {
		return err
	}
	resp, err := http.Get(media.URL)
	if err != nil {
		return err
	}
	done := bot.startWorking(m.ChannelID, m.ID)
	defer done()
	defer resp.Body.Close()
	var r io.ReadCloser
	var ext string
	switch media.Type {
	case mediaImage:
		r, err = bot.uncaptionImage(resp.Body)
		ext = ".png"
	case mediaGIF:
		r, err = bot.uncaptionGif(resp.Body)
		ext = ".gif"
	default:
		body := resp.Body
		out, err := bot.uncaptionVideo(body, media, m.ID)
		if err != nil {
			return err
		}
		done()
		return out.Send(bot.Ctx.Client, m.ChannelID)
	}
	if err != nil {
		return err
	}
	done()
	defer r.Close()
	return bot.sendFile(m.ChannelID, m.ID, "uncaption", ext, r)
}

func (bot *Bot) uncaptionVideo(body io.ReadCloser, media *Media, mid discord.MessageID) (*outputFile, error) {
	in, err := downloadInput(body)
	body.Close()
	if err != nil {
		return nil, err
	}
	defer os.Remove(in.Name())
	defer in.Close()
	firstFrame, err := firstFrame(in.Name())
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
	out, err := bot.createOutput(mid, "uncaption", "."+outfmt)
	if err != nil {
		return nil, err
	}
	fcmd := new(ff.Cmd)
	fcmd.AddFileOutput(out.File, []string{"-y", "-f", outfmt}, streams...)
	err = fcmd.Cmd().Run()
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (*Bot) uncaptionGif(body io.ReadCloser) (io.ReadCloser, error) {
	dgif, err := gif.DecodeAll(body)
	body.Close()
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
	return r, nil
}

func (*Bot) uncaptionImage(resp io.ReadCloser) (io.ReadCloser, error) {
	im, _, err := image.Decode(resp)
	resp.Close()
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
	go func() {
		png.Encode(w, cropped)
		w.Close()
	}()
	return r, nil
}

func firstFrame(filename string) (image.Image, error) {
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
