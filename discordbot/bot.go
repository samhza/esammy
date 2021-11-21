package discordbot

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/diamondburned/arikawa/v3/bot"
	"github.com/diamondburned/arikawa/v3/gateway"
	"github.com/pkg/errors"
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
	out, err := bot.createOutput(m.ID, "mp4")
	if err != nil {
		return err
	}
	defer out.Cleanup()
	err = vedit.Process(args, itype, in, out.File)
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
