package discordbot

import (
	"errors"
	"fmt"
	"github.com/diamondburned/arikawa/v3/state"
	"io"
	"net/http"
	"os"
	"samhza.com/esammy/bot/command"
	"samhza.com/esammy/bot/option"
	"samhza.com/esammy/bot/plugin"
	ff "samhza.com/ffmpeg"
	"strconv"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"samhza.com/esammy/bot"
	"samhza.com/esammy/tenor"
	"samhza.com/esammy/vedit"
)

type Bot struct {
	Router *bot.Router

	cfg Config

	httpClient *http.Client
	tenor      *tenor.Client
	s3         *minio.Client
}

type Config struct {
	Tenor       string `toml:"tenor"`
	OutputDir   string `toml:"output-dir"`
	OutputURL   string `toml:"output-url"`
	S3Endpoint  string `toml:"s3-endpoint"`
	S3KeyID     string `toml:"s3-key-id"`
	S3SecretKey string `toml:"s3-secret-key"`
	S3Bucket    string `toml:"s3-bucket"`
}

func New(client *http.Client, state *state.State, cfg Config) (*Bot, error) {
	b := Bot{httpClient: client, cfg: cfg}
	if cfg.Tenor != "" {
		b.tenor = tenor.NewClient(cfg.Tenor)
		b.tenor.Client = client
	}
	app, err := state.CurrentApplication()
	if err != nil {
		return nil, err
	}
	b.Router, err = bot.New(bot.Options{
		State: state,
		AppID: app.ID,
	})
	if err != nil {
		return nil, err
	}
	b.Router.AddCommand(command.New(b.cmdCaption, "caption", "caption",
		WithMedia{RawString{"text", "caption text"}, &b},
	))
	b.Router.AddCommand(command.New(b.cmdConcat, "concat", "concat",
		RawString{"videos", "space-seperated links to videos to concat"}))
	b.Router.AddCommand(command.New(b.cmdEdit, "edit", "edit",
		WithMedia{RawString{"edits", "edits to perform on video"}, &b},
	))
	b.Router.AddCommand(command.New(b.cmdGif, "gif", "gif",
		WithMedia{option.None{}, &b},
	))
	b.Router.AddCommand(command.New(b.cmdMeme, "meme", "meme",
		WithMedia{MemeArguments{}, &b},
	))
	b.Router.AddCommand(command.New(b.cmdMotivate, "motivate", "motivate",
		WithMedia{MemeArguments{}, &b},
	))
	b.Router.AddCommand(command.New(b.cmdUncaption, "uncaption", "uncaption",
		WithMedia{option.None{}, &b},
	))

	if cfg.S3Endpoint != "" {
		var err error
		b.s3, err = minio.New(cfg.S3Endpoint,
			&minio.Options{
				Creds:  credentials.NewStaticV4(cfg.S3KeyID, cfg.S3SecretKey, ""),
				Secure: true,
			})
		if err != nil {
			panic(err)
		}
	}
	return &b, nil
}

func (bot *Bot) cmdGif(s *state.State, ctx *plugin.Context) (any, error) {
	media := ctx.Options["media"].(Media)
	if media.Type != mediaVideo {
		return nil, errors.New("this isn't a video")
	}
	resp, err := http.Get(media.URL)
	if err != nil {
		return nil, err
	}
	done := bot.startWorking(ctx.Replier)
	defer done()
	in, err := downloadInput(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	defer os.Remove(in.Name())
	defer in.Close()
	var v = ff.Video(ff.InputFile{File: in})
	v = ff.Filter(v, "fps=20")
	one, two := ff.Split(v)
	palette := ff.PaletteGen(two)
	v = ff.PaletteUse(one, palette)
	fcmd := new(ff.Cmd)
	out, err := bot.createOutput(ctx.ID, "gif")
	if err != nil {
		return nil, err
	}
	fcmd.AddFileOutput(out.File, []string{"-y", "-f", "gif"}, v)
	err = fcmd.Cmd().Run()
	if err != nil {
		return nil, err
	}
	return nil, out.Send(ctx.Replier)
}

func (bot *Bot) cmdEdit(s *state.State, ctx *plugin.Context) (any, error) {
	media := ctx.Options["media"].(Media)
	edits := ctx.Options["edits"].(string)
	var args vedit.Arguments
	if err := args.Parse(edits); err != nil {
		return nil, err
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
		return nil, err
	}
	done := bot.startWorking(ctx.Replier)
	defer done()
	in, err := downloadInput(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	defer os.Remove(in.Name())
	defer in.Close()
	out, err := bot.createOutput(ctx.ID, "mp4")
	if err != nil {
		return nil, err
	}
	err = vedit.Process(args, itype, in, out.File)
	if err != nil {
		return nil, err
	}
	done()
	return nil, out.Send(ctx.Replier)
}

func (bot *Bot) cmdConcat(s *state.State, ctx *plugin.Context) (any, error) {
	videos := ctx.Options["videos"].(string)
	var cliplen []int
	args := strings.Split(videos, " ")
	for _, arg := range args {
		n, err := strconv.Atoi(arg)
		if err != nil {
			break
		}
		cliplen = append(cliplen, n)
	}
	fmt.Println(cliplen)
	clips := args[len(cliplen):]
	if len(clips) < 2 {
		return nil, errors.New("need at least 2 videos")
	}
	probed, err := ff.Probe(clips[0])
	width, height := -1, -1
	for _, stream := range probed.Streams {
		if stream.CodecType == ff.CodecTypeVideo {
			width = stream.Width
			height = stream.Height
			break
		}
	}
	done := bot.startWorking(ctx.Replier)
	defer done()
	out, err := bot.createOutput(ctx.ID, "mp4")
	if err != nil {
		return nil, err
	}
	var inputs []ff.Stream
	for i, arg := range clips {
		input := ff.Input{Name: arg}
		scaled := ff.Filter(ff.Video(input),
			fmt.Sprintf("scale=%d:%d", width, height),
		)
		if i+1 <= len(cliplen) {
			n := strconv.Itoa(cliplen[i])
			inputs = append(inputs,
				ff.Filter(scaled,
					"trim=duration="+n),
				ff.Filter(ff.Audio(input),
					"atrim=duration="+n))
		} else {
			inputs = append(inputs, scaled, ff.Audio(input))
		}
	}
	outs := ff.Concat(1, 1, inputs...)
	fcmd := new(ff.Cmd)
	if err != nil {
		return nil, err
	}
	fcmd.AddFileOutput(out.File, []string{"-y", "-f", "mp4"}, outs...)
	err = fcmd.Cmd().Run()
	if err != nil {
		return nil, err
	}
	done()
	return nil, out.Send(ctx.Replier)
}

func downloadInput(body io.Reader) (*os.File, error) {
	in, err := os.CreateTemp("", "esammy.*")
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(in, body); err != nil {
		in.Close()
		os.Remove(in.Name())
		return nil, err
	}
	if _, err = in.Seek(0, io.SeekStart); err != nil {
		in.Close()
		os.Remove(in.Name())
		return nil, err
	}
	return in, nil
}
