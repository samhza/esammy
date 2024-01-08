package discordbot

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/diamondburned/arikawa/v3/utils/bot"
	"github.com/diamondburned/arikawa/v3/gateway"
	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"samhza.com/esammy/tenor"
	"samhza.com/esammy/vedit"
	ff "samhza.com/ffmpeg"
)

type Bot struct {
	Ctx *bot.Context

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

func New(client *http.Client, cfg Config) *Bot {
	b := Bot{Ctx: nil, httpClient: client, cfg: cfg}
	if cfg.Tenor != "" {
		b.tenor = tenor.NewClient(cfg.Tenor)
		b.tenor.Client = client
	}
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
	done := bot.startWorking(m.ChannelID, m.ID)
	defer done()
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
	out, err := bot.createOutput(m.ID, "gif", ".gif")
	if err != nil {
		return err
	}
	fcmd.AddFileOutput(out.File, []string{"-y", "-f", "gif"}, v)
	err = fcmd.Cmd().Run()
	if err != nil {
		return err
	}
	return out.Send(bot.Ctx.Client, m.ChannelID)
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
	done := bot.startWorking(m.ChannelID, m.ID)
	defer done()
	in, err := downloadInput(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	defer os.Remove(in.Name())
	defer in.Close()
	out, err := bot.createOutput(m.ID, "edit", ".mp4")
	if err != nil {
		return err
	}
	err = vedit.Process(args, itype, in, out.File)
	if err != nil {
		return err
	}
	done()
	return out.Send(bot.Ctx.Client, m.ChannelID)
}

func (bot *Bot) Concat(m *gateway.MessageCreateEvent, args ...string) error {
	var cliplen []int
	for _, arg := range args {
		n, err := strconv.Atoi(arg)
		if err != nil {
			break
		}
		cliplen = append(cliplen, n)
	}
	fmt.Println(cliplen)
	clips := args[len(cliplen):]
	for _, att := range m.Attachments {
		clips = append(clips, att.Proxy)
	}
	if len(clips) < 2 {
		return errors.New("need at least 2 videos")
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
	done := bot.startWorking(m.ChannelID, m.ID)
	defer done()
	out, err := bot.createOutput(m.ID, "combined", ".mp4")
	if err != nil {
		return err
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
		return err
	}
	fcmd.AddFileOutput(out.File, []string{"-y", "-f", "mp4"}, outs...)
	err = fcmd.Cmd().Run()
	if err != nil {
		return err
	}
	done()
	return out.Send(bot.Ctx.Client, m.ChannelID)
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
