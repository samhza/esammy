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

	"github.com/diamondburned/arikawa/v3/gateway"
	ff "go.samhza.com/ffmpeg"
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
	defer resp.Body.Close()
	switch media.Type {
	case mediaImage:
		im, _, err := image.Decode(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		newMinY, found := detectCaption(im)
		if !found {
			return errors.New("couldn't find the caption")
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
		return bot.sendFile(m.ChannelID, m.ID, "png", r)
	case mediaGIF:
		dgif, err := gif.DecodeAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		newMinY, found := detectCaption(dgif.Image[0])
		if !found {
			return errors.New("couldn't find the caption")
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
		return bot.sendFile(m.ChannelID, m.ID, "gif", r)

	}
	in, err := downloadInput(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	defer os.Remove(in.Name())
	defer in.Close()
	firstFrame, err := firstVidFrame(in.Name())
	if err != nil {
		return err
	}
	newMinY, found := detectCaption(firstFrame)
	if !found {
		return errors.New("couldn't find the caption")
	}
	var instream ff.Stream = ff.InputFile{File: in}

	probed, err := ff.Probe(in.Name())
	if err != nil {
		return err
	}
	var hasAudio bool
	var inDur string
	for _, stream := range probed.Streams {
		if stream.CodecType == ff.CodecTypeVideo {
			inDur = stream.Duration
		} else {
			hasAudio = true
		}
	}
	var v, a ff.Stream
	v = ff.Video(instream)

	b := firstFrame.Bounds()
	var outfmt string
	if media.Type == mediaGIFV {
		v = ff.Filter(v, "fps=20")
		one, two := ff.Split(v)
		palette := ff.PaletteGen(two)
		v = ff.PaletteUse(one, palette)
		outfmt = "gif"
	} else {
		outfmt = "mp4"
		if hasAudio {
			a = ff.Audio(instream)
		} else {
			a = ff.Filter(ff.ANullSrc,
				"atrim=duration="+inDur)
		}
	}
	v = ff.Filter(v, "crop=y="+strconv.Itoa(newMinY)+
		":out_h="+strconv.Itoa(b.Max.Y-newMinY))
	out, err := bot.createOutput(m.ID, outfmt)
	if err != nil {
		return err
	}
	defer out.Cleanup()
	fcmd := new(ff.Cmd)
	fcmd.AddFileOutput(out.File, []string{"-y", "-f", outfmt}, v, a)
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
