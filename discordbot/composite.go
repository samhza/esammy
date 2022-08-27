package discordbot

import (
	"bytes"
	"fmt"
	"github.com/diamondburned/arikawa/v3/state"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path"
	"samhza.com/esammy/bot/option"
	"samhza.com/esammy/bot/plugin"
	"strings"

	"github.com/diamondburned/arikawa/v3/discord"
	"github.com/disintegration/imaging"
	"github.com/pkg/errors"
	"samhza.com/esammy/memegen"
	ff "samhza.com/ffmpeg"
)

type MemeArguments struct{}

var memeArgsForDiscord = option.Default{
	option.Option{
		Name:        "top",
		Type:        option.String{},
		Description: "The top text of the meme",
		Required:    false,
	},
	option.Option{
		Name:        "bottom",
		Type:        option.String{},
		Description: "The bottom text of the meme",
		Required:    false,
	},
}

func (m MemeArguments) Parse(s *state.State, ctx *plugin.Context) error {
	if ctx.IsInteraction() {
		err := memeArgsForDiscord.Parse(s, ctx)
		if err != nil {
			return err
		}
		_, ok1 := ctx.Options["top"]
		_, ok2 := ctx.Options["bottom"]
		if !ok1 && !ok2 {
			return errors.New("either top or bottom must be specified")
		}
		return nil
	}
	args := ctx.Source.(plugin.MessageSource).Content[ctx.OptsIndex:]
	if args == "" {
		return errors.New("you need some text for me to generate the image")
	}
	split := strings.SplitN(args, ",", 2)
	ctx.Options["top"] = strings.TrimSpace(split[0])
	if len(split) == 2 {
		ctx.Options["bottom"] = strings.TrimSpace(split[1])
	}
	return nil
}

func (m MemeArguments) ForDiscord() []discord.CommandOptionValue {
	return memeArgsForDiscord.ForDiscord()
}

func (bot *Bot) cmdMeme(s *state.State, ctx *plugin.Context) (any, error) {
	return bot.composite(s, ctx, func(w, h int) (image.Image, image.Point, bool) {
		m := image.NewRGBA(image.Rect(0, 0, w, h))
		var top, bot string
		if toparg, ok := ctx.Options["top"]; ok {
			top = toparg.(string)
		}
		if botarg, ok := ctx.Options["bottom"]; ok {
			bot = botarg.(string)
		}
		memegen.Impact(m, top, bot)
		return m, image.Point{}, false
	})
}

func (bot *Bot) cmdMotivate(s *state.State, ctx *plugin.Context) (any, error) {
	return bot.composite(s, ctx, func(w, h int) (image.Image, image.Point, bool) {
		var top, bot string
		if toparg, ok := ctx.Options["top"]; ok {
			top = toparg.(string)
		}
		if botarg, ok := ctx.Options["bottom"]; ok {
			bot = botarg.(string)
		}
		img, pt := memegen.Motivate(w, h, top, bot)
		return img, pt, true
	})
}

type WithMedia struct {
	Options plugin.Options
	Bot     *Bot
}

func (w WithMedia) Parse(s *state.State, ctx *plugin.Context) error {
	if err := w.Options.Parse(s, ctx); err != nil {
		return err
	}
	if ctx.IsInteraction() {
		opt := ctx.InteractionOpts.Find("attachment")
		if opt.Name != "" {
			var at discord.Attachment
			opt.Value.UnmarshalTo(&at)
			if at.Height != 0 {
				ext := path.Ext(at.Proxy)
				ctx.Options["media"] = Media{
					URL:  at.Proxy,
					Type: mediaTypeByExt(ext),
				}
				return nil
			}
		}
		opt = ctx.InteractionOpts.Find("url")
		if opt.Name != "" {
			var str string
			opt.Value.UnmarshalTo(&str)
			ctx.Options["media"] = Media{
				URL:  str,
				Type: mediaTypeByExt(path.Ext(str)),
			}
			return nil
		}
	} else {
		m := ctx.Source.(plugin.MessageSource).Message
		if media := w.Bot.getMsgMedia(m); media != nil {
			ctx.Options["media"] = *media
			return nil
		}
		if m.Type == discord.InlinedReplyMessage && m.ReferencedMessage != nil {
			if media := w.Bot.getMsgMedia(*m.ReferencedMessage); media != nil {
				ctx.Options["media"] = *media
				return nil
			}
		}
	}
	msgs, err := s.Messages(ctx.ChannelID, 20)
	if err != nil {
		return err
	}
	for _, m := range msgs {
		if media := w.Bot.getMsgMedia(m); media != nil {
			ctx.Options["media"] = *media
			return nil
		}
	}
	return errors.New("no media found")
}

func (w WithMedia) ForDiscord() []discord.CommandOptionValue {
	return append(w.Options.ForDiscord(), []discord.CommandOptionValue{
		&discord.AttachmentOption{
			OptionName:  "attachment",
			Description: "The attachment to use",
		},
		&discord.StringOption{
			OptionName:  "url",
			Description: "The url to use",
		},
	}...)
}

type RawString struct {
	Name        string
	Description string
}

func (r RawString) Parse(s *state.State, ctx *plugin.Context) error {
	if ctx.IsInteraction() {
		opt := ctx.InteractionOpts.Find(r.Name)
		if opt.Name == "" {
			return errors.New("RawString: option not found")
		}
		var str string
		opt.Value.UnmarshalTo(&str)
		ctx.Options[r.Name] = str
	} else {
		str := ctx.Source.(plugin.MessageSource).Content[ctx.OptsIndex:]
		if str == "" {
			return errors.New("RawString: no content")
		}
		ctx.Options[r.Name] = str
	}
	return nil
}

func (r RawString) ForDiscord() []discord.CommandOptionValue {
	return []discord.CommandOptionValue{
		&discord.StringOption{
			OptionName:  r.Name,
			Description: r.Description,
			Required:    true,
		},
	}
}

func (bot *Bot) cmdCaption(s *state.State, ctx *plugin.Context) (any, error) {
	return bot.composite(s, ctx, func(w, h int) (image.Image, image.Point, bool) {
		img, pt := memegen.Caption(w, h, ctx.Options["text"].(string))
		return img, pt, true
	})
}

type compositeFunc func(int, int) (image.Image, image.Point, bool)

func (bot *Bot) composite(s *state.State, ctx *plugin.Context, imgfn compositeFunc) (any, error) {
	media := ctx.Options["media"].(Media)
	resp, err := bot.httpClient.Get(media.URL)
	if err != nil {
		return nil, err
	}
	done := bot.startWorking(ctx.Replier)
	defer done()
	defer resp.Body.Close()
	b := resp.Body
	if media.Type == mediaImage {
		img, _, err := image.Decode(b)
		b.Close()
		if err != nil {
			return nil, err
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
		return nil, bot.sendFile(s, ctx, "png", r)
	} else {
		in, err := downloadInput(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		inName := in.Name()
		in.Close()
		defer os.Remove(in.Name())
		var width, height int
		probed, err := ff.Probe(inName)
		if err != nil {
			return nil, err
		}
		for _, stream := range probed.Streams {
			if stream.CodecType == ff.CodecTypeVideo {
				width = stream.Width
				height = stream.Height
			}
		}

		input := ff.Input{Name: inName}
		var v ff.Stream

		img, pt, under := imgfn(width, height)
		pR, pW, err := os.Pipe()
		if err != nil {
			return nil, err
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
				return nil, err
			}
			if _, err = in.Seek(0, 0); err != nil {
				return nil, err
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
		out, err := bot.createOutput(ctx.ID, format)
		if err != nil {
			return nil, err
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
				return nil, fmt.Errorf("exit status %d: %s",
					exitError.ExitCode(), stderr.String())
			}
			return nil, err
		}
		done()
		return nil, out.Send(ctx.Replier)
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
